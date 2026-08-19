# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
# pragma-jennifer-version: >=0.25.0
# pragma-jennifer-capability: net

/**
 * A MikroTik **MAC-Telnet** client - a Layer-2 console to a RouterOS device by
 * MAC address, with no IP configured on either side (what Winbox's "Neighbors"
 * console does). The transport is UDP broadcast on port 20561 with the source
 * and destination MAC carried in the packet payload, so it reaches a router that
 * has no address yet - the way to set the first IP on a fresh or just-reset
 * device before switching to the IP-based `mikrotik` API.
 *
 * `connect` runs the full session-start + authentication handshake and returns a
 * logged-in `Session`; `send` writes a console command, `recv` reads whatever
 * output is available, and `close` ends the session. It drives the router's text
 * CLI (send `/ip address add ...\r\n`, read the echo), not the binary API.
 *
 * Authentication auto-detects the router's generation from the salt it offers:
 * legacy 16-byte-salt **MD5** (RouterOS < 6.43) or the modern 49-byte **EC-SRP**
 * (Curve25519; RouterOS 6.43+ and all v7, including factory-fresh devices), via
 * `crypto.mtwei*`. The local interface's MAC is read from `/sys/class/net`.
 *
 * Needs the default `jennifer` binary (`net`, and broadcast UDP is Linux-only).
 * Over `net` (SO_BROADCAST + UDP) + `crypto` (EC-SRP) + `hash` (MD5) + `binary` +
 * `encoding` + `fs` + `kv` (per-session mutable counters).
 *
 * SECURITY: MAC-Telnet is an unauthenticated-transport, cleartext L2 protocol -
 * the password proof is hashed but the session itself is not encrypted, and any
 * host on the same segment can see the traffic. Use it only on a trusted wire to
 * bootstrap a device, then move to `mikrotik.optionsTLS` (api-ssl) over IP.
 * @module mactelnet
 * @example
 * import "mactelnet.j" as mactelnet;
 * def s as mactelnet.Session init mactelnet.connect("eth0", "e4:8d:8c:11:22:33", "admin", "");
 * mactelnet.send($s, "/ip address add address=192.168.88.10/24 interface=ether1\r\n");
 * def out as string init mactelnet.recv($s, 1500);
 * mactelnet.close($s);
 */
use net;
use crypto;
use hash;
use binary;
use convert;
use strings;
use encoding;
use fs;
use kv;
use os;

# --- protocol constants (private) -------------------------------------------

def const MT_PORT as int init 20561;

# Packet types (header byte 1).
def const PTYPE_SESSIONSTART as int init 0;
def const PTYPE_DATA as int init 1;
def const PTYPE_ACK as int init 2;
def const PTYPE_END as int init 255;

# Control-packet types (inside a DATA payload).
def const CPTYPE_BEGINAUTH as int init 0;
def const CPTYPE_PASSSALT as int init 1;
def const CPTYPE_PASSWORD as int init 2;
def const CPTYPE_USERNAME as int init 3;
def const CPTYPE_TERM_TYPE as int init 4;
def const CPTYPE_TERM_WIDTH as int init 5;
def const CPTYPE_TERM_HEIGHT as int init 6;
def const CPTYPE_END_AUTH as int init 9;

# The 22-byte MAC-Telnet header length, and the largest datagram we read.
def const HEADER_LEN as int init 22;
def const MAX_DATAGRAM as int init 1600;

# Retransmit schedule (ms) for a reliable send: resend until an ACK arrives, then
# give up. Bounded so an unreachable or wrong MAC fails instead of hanging.
def const POLL_MS as int init 120;

func retransmitMs() {
    return [150, 250, 500, 900, 1500];
}

func fail(msg as string) {
    throw Error{kind: "mactelnet", message: "mactelnet: " + $msg, file: "", line: 0, col: 0};
}

# --- structs ----------------------------------------------------------------

/**
 * An open MAC-Telnet console session. `sock` is the broadcast UDP socket;
 * `state` is a per-session key/value store holding the mutable send/receive
 * counters and the pending-output buffer (a plain struct field could not persist
 * a mutation across a module call, so the counters live in shared `kv` state);
 * `srcmac` / `dstmac` are the 6-byte local and router MACs; `seskey` is the
 * random session id.
 * @field sock {net.UDPSocket} the broadcast UDP socket
 * @field state {kv.Store} per-session counters and output buffer
 * @field srcmac {bytes} local interface MAC (6 bytes)
 * @field dstmac {bytes} router MAC (6 bytes)
 * @field seskey {int} the 16-bit session key
 */
export def struct Session {
    sock as net.UDPSocket,
    state as kv.Store,
    srcmac as bytes,
    dstmac as bytes,
    seskey as int
};

# One decoded server DATA packet's salient contents (private): collected plain
# terminal text, an auth salt if a PASSSALT control was present, and the auth /
# end flags.
def struct Frame {
    plain as string,
    salt as bytes,
    endAuth as bool,
    ended as bool
};

# A parsed packet header (private): the fields the client dispatches on.
def struct Header {
    ptype as int,
    seskey as int,
    counter as int
};

# --- little byte helpers (private) ------------------------------------------

func emptyBytes() {
    def b as bytes;
    return $b;
}

func utf8(s as string) {
    return convert.bytesFromString($s, "utf-8");
}

func u16be(n as int) {
    def b as bytes;
    $b[] = ($n >> 8) & 0xff;
    $b[] = $n & 0xff;
    return $b;
}

func u16le(n as int) {
    def b as bytes;
    $b[] = $n & 0xff;
    $b[] = ($n >> 8) & 0xff;
    return $b;
}

func u32be(n as int) {
    def b as bytes;
    $b[] = ($n >> 24) & 0xff;
    $b[] = ($n >> 16) & 0xff;
    $b[] = ($n >> 8) & 0xff;
    $b[] = $n & 0xff;
    return $b;
}

func readU16be(buf as bytes, off as int) {
    return ($buf[$off] << 8) | $buf[$off + 1];
}

func readU32be(buf as bytes, off as int) {
    return ($buf[$off] << 24) | ($buf[$off + 1] << 16) | ($buf[$off + 2] << 8) | $buf[$off + 3];
}

# --- MAC address (private, plus exported convenience) -----------------------

/**
 * Parse a MAC address string ("aa:bb:cc:dd:ee:ff", "-" separators or none also
 * accepted) into its 6 raw bytes.
 * @param s {string} the MAC address text
 * @return {bytes} the 6-byte address
 */
export func parseMac(s as string) {
    def clean as string init strings.replace($s, ":", "");
    $clean = strings.replace($clean, "-", "");
    $clean = strings.lower(strings.trim($clean));
    if (len($clean) != 12) {
        fail("invalid MAC address: " + $s);
    }
    def b as bytes init encoding.fromText($clean, "hex");
    if (len($b) != 6) {
        fail("invalid MAC address: " + $s);
    }
    return $b;
}

/**
 * Format 6 raw MAC bytes as a colon-separated lower-case string.
 * @param b {bytes} the 6-byte address
 * @return {string} "aa:bb:cc:dd:ee:ff"
 */
export func formatMac(b as bytes) {
    if (len($b) != 6) {
        fail("a MAC address is 6 bytes, got " + convert.toString(len($b)));
    }
    def parts as list of string init [];
    for (def i in 0..6) {
        def h as string init encoding.toText(binary.slice($b, $i, $i + 1), "hex");
        $parts[] = $h;
    }
    return strings.join($parts, ":");
}

# localMac reads an interface's hardware address from Linux sysfs.
func localMac(iface as string) {
    def p as string init "/sys/class/net/" + $iface + "/address";
    if (not fs.exists($p)) {
        fail("no such network interface: " + $iface + " (looked at " + $p + ")");
    }
    return parseMac(strings.trim(fs.readString($p)));
}

# --- packet build / parse (private) -----------------------------------------

# buildHeader builds the 22-byte client header. Client layout: session key at
# offset 14 (big-endian), client type {00 15} at 16, counter at 18 (big-endian).
func buildHeader(ptype as int, src as bytes, dst as bytes, seskey as int, counter as int) {
    def h as bytes;
    $h[] = 1;
    $h[] = $ptype;
    $h = binary.concat($h, $src);
    $h = binary.concat($h, $dst);
    $h = binary.concat($h, u16be($seskey));
    def ct as bytes;
    $ct[] = 0x00;
    $ct[] = 0x15;
    $h = binary.concat($h, $ct);
    $h = binary.concat($h, u32be($counter));
    return $h;
}

# parseHeader reads a server->client packet header. Server layout puts the
# session key at offset 16 (the client/server swap of key and client-type).
func parseHeader(pkt as bytes) {
    return Header{ptype: $pkt[1], seskey: readU16be($pkt, 16), counter: readU32be($pkt, 18)};
}

# controlBlock frames one control packet: magic 56 34 12 ff, type, 4-byte
# big-endian length, then the data.
func controlBlock(cptype as int, data as bytes) {
    def b as bytes;
    $b[] = 0x56;
    $b[] = 0x34;
    $b[] = 0x12;
    $b[] = 0xff;
    $b[] = $cptype;
    $b = binary.concat($b, u32be(len($data)));
    $b = binary.concat($b, $data);
    return $b;
}

func matchMagic(buf as bytes, off as int) {
    return $buf[$off] == 0x56 and $buf[$off + 1] == 0x34 and $buf[$off + 2] == 0x12 and
        $buf[$off + 3] == 0xff;
}

# md5Password builds the legacy password control payload:
# 0x00 followed by MD5(0x00 + password + salt). 17 bytes.
func md5Password(password as string, salt as bytes) {
    def m as bytes;
    $m[] = 0;
    $m = binary.concat($m, utf8($password));
    $m = binary.concat($m, $salt);
    def d as bytes init hash.compute($m, "md5");
    def r as bytes;
    $r[] = 0;
    $r = binary.concat($r, $d);
    return $r;
}

# loginKey is the PASSSALT payload the client sends first: username, a null
# byte, then the 33-byte EC-SRP public key.
func loginKey(user as string, pubkey as bytes) {
    def k as bytes init utf8($user);
    def z as bytes;
    $z[] = 0;
    $k = binary.concat($k, $z);
    $k = binary.concat($k, $pubkey);
    return $k;
}

# --- session state accessors (private) --------------------------------------

func getInt(state as kv.Store, key as string) {
    return convert.toInt(kv.get($state, $key));
}

func setInt(state as kv.Store, key as string, n as int) {
    kv.set($state, $key, convert.toString($n), 0);
    return;
}

func appendBuf(state as kv.Store, plain as string) {
    if (len($plain) > 0) {
        kv.set($state, "buf", kv.get($state, "buf") + $plain, 0);
    }
    return;
}

func drainBuf(state as kv.Store) {
    def b as string init kv.get($state, "buf");
    kv.set($state, "buf", "", 0);
    return $b;
}

# --- send / receive core (private) ------------------------------------------

func sendRaw(s as Session, packet as bytes) {
    net.sendTo($s.sock, kv.get($s.state, "target"), $packet);
    return;
}

# broadcastTarget is the "host:port" every packet is sent to. It is the limited
# broadcast address by default; JENNIFER_MACTELNET_TARGET overrides it (a unicast
# loopback address for the test harness, which has no broadcast segment).
func broadcastTarget() {
    def t as string init os.getEnv("JENNIFER_MACTELNET_TARGET");
    if (len($t) == 0) {
        return "255.255.255.255:" + convert.toString(MT_PORT);
    }
    return $t;
}

# recvPacket waits up to timeoutMs for a datagram. Returns the raw bytes, or an
# empty bytes on timeout. The read deadline is cleared on every exit path (0
# clears it) so it never leaks onto the next send - net.setDeadline arms the
# write side too, and a stale deadline would fail the following sendTo.
func recvPacket(s as Session, timeoutMs as int) {
    defer net.setDeadline($s.sock, 0);
    net.setDeadline($s.sock, $timeoutMs);
    try {
        def dg as net.Datagram init net.recvFrom($s.sock, MAX_DATAGRAM);
        return $dg.data;
    } catch (e) {
        return emptyBytes();
    }
}

# ackFor sends an ACK for a received DATA packet: counter = the received counter
# plus the received payload length, no body.
func ackFor(s as Session, pkt as bytes) {
    def h as Header init parseHeader($pkt);
    def payloadLen as int init len($pkt) - HEADER_LEN;
    sendRaw($s, buildHeader(PTYPE_ACK, $s.srcmac, $s.dstmac, $s.seskey, $h.counter + $payloadLen));
    return;
}

# processData acknowledges a server DATA packet, drops duplicates by the receive
# counter, walks its control blocks, buffers any plain terminal text, and returns
# a Frame describing what it found.
func processData(s as Session, pkt as bytes) {
    def h as Header init parseHeader($pkt);
    ackFor($s, $pkt);

    def inc as int init getInt($s.state, "in");
    def isNew as bool init ($h.counter > $inc) or (($inc - $h.counter) > 65535);
    if (not $isNew) {
        return Frame{plain: "", salt: emptyBytes(), endAuth: false, ended: false};
    }
    setInt($s.state, "in", $h.counter);

    def payload as bytes init binary.slice($pkt, HEADER_LEN, len($pkt));
    def off as int init 0;
    def plain as string init "";
    def salt as bytes init emptyBytes();
    def endAuth as bool init false;
    while ($off < len($payload)) {
        if ($off + 9 <= len($payload) and matchMagic($payload, $off)) {
            def cptype as int init $payload[$off + 4];
            def clen as int init readU32be($payload, $off + 5);
            def dstart as int init $off + 9;
            if ($dstart + $clen > len($payload)) {
                $clen = len($payload) - $dstart;
            }
            def cdata as bytes init binary.slice($payload, $dstart, $dstart + $clen);
            if ($cptype == CPTYPE_PASSSALT) {
                $salt = $cdata;
            } elseif ($cptype == CPTYPE_END_AUTH) {
                $endAuth = true;
            }
            $off = $dstart + $clen;
        } else {
            $plain = $plain +
                convert.stringFromBytes(binary.slice($payload, $off, len($payload)), "utf-8");
            $off = len($payload);
        }
    }
    appendBuf($s.state, $plain);
    return Frame{plain: $plain, salt: $salt, endAuth: $endAuth, ended: false};
}

# markEnd records that the server closed the session.
func markEnd(s as Session) {
    kv.set($s.state, "open", "0", 0);
    return Frame{plain: "", salt: emptyBytes(), endAuth: false, ended: true};
}

# dispatch handles one received packet, appending any resulting Frame to frames
# and returning whether it was the ACK we were waiting for.
func dispatch(s as Session, pkt as bytes, frames as list of Frame) {
    def h as Header init parseHeader($pkt);
    if ($h.seskey != $s.seskey) {
        return frames;
    }
    if ($h.ptype == PTYPE_DATA) {
        $frames[] = processData($s, $pkt);
    } elseif ($h.ptype == PTYPE_END) {
        $frames[] = markEnd($s);
    }
    return $frames;
}

# sendReliable sends a packet and resends it on the retransmit schedule until an
# ACK for this session arrives, draining (and collecting) any DATA / END packets
# that turn up while waiting. Returns the collected Frames; throws on timeout.
func sendReliable(s as Session, packet as bytes) {
    def frames as list of Frame init [];
    for (def interval in retransmitMs()) {
        sendRaw($s, $packet);
        def pkt as bytes init recvPacket($s, $interval);
        while (len($pkt) > 0) {
            def h as Header init parseHeader($pkt);
            if ($h.seskey == $s.seskey and $h.ptype == PTYPE_ACK) {
                return $frames;
            }
            $frames = dispatch($s, $pkt, $frames);
            $pkt = recvPacket($s, POLL_MS);
        }
    }
    fail("timed out waiting for the router (wrong MAC, or mac-server off on this port?)");
}

# pump receives for up to timeoutMs, ACKing and collecting server DATA / END
# packets, and returns the collected Frames (no send).
func pump(s as Session, timeoutMs as int) {
    def frames as list of Frame init [];
    def pkt as bytes init recvPacket($s, $timeoutMs);
    while (len($pkt) > 0) {
        $frames = dispatch($s, $pkt, $frames);
        $pkt = recvPacket($s, POLL_MS);
    }
    return $frames;
}

# sendData sends a DATA packet (header + payload) reliably and advances the send
# counter by the payload length. Returns the Frames seen while waiting.
func sendData(s as Session, payload as bytes) {
    def out as int init getInt($s.state, "out");
    def pkt as bytes init binary.concat(
        buildHeader(PTYPE_DATA, $s.srcmac, $s.dstmac, $s.seskey, $out),
        $payload);
    def frames as list of Frame init sendReliable($s, $pkt);
    setInt($s.state, "out", $out + len($payload));
    return $frames;
}

# findSalt returns the first auth salt among frames, or empty bytes.
func findSalt(frames as list of Frame) {
    for (def f in $frames) {
        if (len($f.salt) > 0) {
            return $f.salt;
        }
    }
    return emptyBytes();
}

func anyEndAuth(frames as list of Frame) {
    for (def f in $frames) {
        if ($f.endAuth) {
            return true;
        }
    }
    return false;
}

func anyEnded(frames as list of Frame) {
    for (def f in $frames) {
        if ($f.ended) {
            return true;
        }
    }
    return false;
}

func closeQuiet(s as Session) {
    try {
        net.close($s.sock);
    } catch (e) { # lint-disable: L103
    }
    try {
        kv.close($s.state);
    } catch (e) { # lint-disable: L103
    }
    return;
}

# --- public API -------------------------------------------------------------

/**
 * Open a MAC-Telnet console to a router by MAC address and log in. Runs the full
 * session-start + authentication handshake (auto-detecting MD5 vs EC-SRP from
 * the router's salt) and returns a logged-in `Session`.
 *
 * `iface` is the local interface to reach the router on (its MAC is read from
 * `/sys/class/net/<iface>/address`); `mac` is the router's MAC. A blank password
 * is valid (a factory-default `admin`). Throws `Error{kind: "mactelnet"}` on an
 * unreachable router or a failed login.
 * @param iface {string} local network interface name (e.g. "eth0")
 * @param mac {string} the router's MAC address
 * @param user {string} the login user (e.g. "admin")
 * @param password {string} the login password ("" for none)
 * @return {Session} a logged-in session
 */
export func connect(iface as string, mac as string, user as string, password as string) {
    def src as bytes init localMac($iface);
    def dst as bytes init parseMac($mac);
    def seskey as int init crypto.randInt(1, 65534);
    def kp as crypto.Keypair init crypto.mtweiKeygen();

    def store as kv.Store init kv.open();
    kv.set($store, "out", "0", 0);
    kv.set($store, "in", "-1", 0);
    kv.set($store, "buf", "", 0);
    kv.set($store, "open", "1", 0);
    kv.set($store, "target", broadcastTarget(), 0);

    def sock as net.UDPSocket init net.listenUDP("0.0.0.0:0");
    net.setBroadcast($sock, true);
    def s as Session init Session{
        sock: $sock,
        state: $store,
        srcmac: $src,
        dstmac: $dst,
        seskey: $seskey
    };

    # 1. Start the session (wait for the router's ACK - proof of reachability).
    sendReliable($s, buildHeader(PTYPE_SESSIONSTART, $src, $dst, $seskey, 0));

    # 2. Begin auth and offer our EC-SRP public key. The router replies with a
    #    salt whose length selects the algorithm.
    def auth1 as bytes init binary.concat(
        controlBlock(CPTYPE_BEGINAUTH, emptyBytes()),
        controlBlock(CPTYPE_PASSSALT, loginKey($user, $kp.public)));
    def frames as list of Frame init sendData($s, $auth1);
    def salt as bytes init findSalt($frames);
    def tries as int init 0;
    while (len($salt) == 0 and $tries < 20) {
        $salt = findSalt(pump($s, 400));
        $tries = $tries + 1;
    }
    if (len($salt) == 0) {
        closeQuiet($s);
        fail("no auth salt from the router (is mac-server enabled on this interface?)");
    }

    # 3. Build the password proof for the offered algorithm.
    def pwctl as bytes;
    if (len($salt) == 16) {
        $pwctl = controlBlock(CPTYPE_PASSWORD, md5Password($password, $salt));
    } elseif (len($salt) == 49) {
        def serverKey as bytes init binary.slice($salt, 0, 33);
        def salt16 as bytes init binary.slice($salt, 33, 49);
        def validator as bytes init crypto.mtweiId($user, $password, $salt16);
        def resp as bytes init crypto.mtweiClientKey(
            $kp.private,
            $serverKey,
            $kp.public,
            $validator);
        $pwctl = controlBlock(CPTYPE_PASSWORD, $resp);
    } else {
        closeQuiet($s);
        fail("unexpected salt length " + convert.toString(len($salt)) + " (want 16 or 49)");
    }

    # 4. Send credentials + terminal parameters, then wait for end-of-auth.
    def auth2 as bytes init $pwctl;
    $auth2 = binary.concat($auth2, controlBlock(CPTYPE_USERNAME, utf8($user)));
    $auth2 = binary.concat($auth2, controlBlock(CPTYPE_TERM_TYPE, utf8("vt100")));
    $auth2 = binary.concat($auth2, controlBlock(CPTYPE_TERM_WIDTH, u16le(80)));
    $auth2 = binary.concat($auth2, controlBlock(CPTYPE_TERM_HEIGHT, u16le(24)));
    def frames2 as list of Frame init sendData($s, $auth2);

    def ok as bool init anyEndAuth($frames2);
    def ended as bool init anyEnded($frames2);
    def tries2 as int init 0;
    while (not $ok and not $ended and $tries2 < 20) {
        def more as list of Frame init pump($s, 500);
        if (anyEndAuth($more)) {
            $ok = true;
        }
        if (anyEnded($more)) {
            $ended = true;
        }
        $tries2 = $tries2 + 1;
    }
    if (not $ok) {
        def msg as string init strings.trim(drainBuf($s.state));
        closeQuiet($s);
        if (len($msg) == 0) {
            $msg = "login failed (wrong username or password?)";
        }
        fail("login failed: " + $msg);
    }
    return $s;
}

/**
 * Send a console command (or any raw input) to the router. Include a trailing
 * "\r\n" to submit a command line. Output is not returned here; read it with
 * `recv`.
 * @param s {Session} the session
 * @param text {string} the text to send
 * @return {null} nothing
 */
export func send(s as Session, text as string) {
    sendData($s, utf8($text));
    return;
}

/**
 * Read whatever console output has arrived, waiting up to `timeoutMs` for the
 * first byte. Returns the accumulated text (possibly ""); call it again to read
 * more. ANSI escape sequences from the RouterOS CLI are passed through verbatim.
 * @param s {Session} the session
 * @param timeoutMs {int} how long to wait for output, in milliseconds
 * @return {string} the console output received
 */
export func recv(s as Session, timeoutMs as int) {
    pump($s, $timeoutMs);
    return drainBuf($s.state);
}

/**
 * Whether the router has closed the session (an END packet was seen).
 * @param s {Session} the session
 * @return {bool} true once the session has ended
 */
export func closed(s as Session) {
    return kv.get($s.state, "open") == "0";
}

/**
 * End the session: tell the router, then release the socket and session state.
 * @param s {Session} the session
 * @return {null} nothing
 */
export func close(s as Session) {
    try {
        sendRaw($s, buildHeader(PTYPE_END, $s.srcmac, $s.dstmac, $s.seskey, 0));
    } catch (e) { # lint-disable: L103
    }
    closeQuiet($s);
    return;
}
