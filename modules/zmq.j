# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.26.0
# pragma-jennifer-capability: net

/**
 * A ZeroMQ peer over the `net` system library: ZMTP 3.1 (RFC 37), the wire
 * protocol libzmq, pyzmq, JeroMQ and the other ZeroMQ stacks speak over TCP, so a
 * Jennifer program can talk to them. Socket types PAIR, PUB / SUB, REQ / REP,
 * DEALER / ROUTER and PUSH / PULL, multipart messages as `list of bytes`, and the
 * NULL (no security) and PLAIN (username / password, sent in clear text)
 * mechanisms. CURVE encryption is not supported.
 *
 * Unlike libzmq, a socket is **one connection to one peer**, and nothing happens
 * in the background: `connect` dials a peer, `listen` + `accept` take peers one
 * at a time (spawn a task per accepted socket to serve several), and the program
 * reads with `receive` or `poll` (with a timeout). There is no automatic
 * reconnect and no send queue: a `send` writes to the connection, and fails if
 * it is gone. Needs the default `jennifer` binary (uses `net`).
 * @module zmq
 * @example
 * import "zmq.j" as zmq;
 * def s as zmq.Socket init zmq.connect("REQ", "127.0.0.1:5555");
 * zmq.sendText($s, "hello");
 * def m as zmq.Message init zmq.receive($s);
 * zmq.close($s);
 */
use net;
use binary;
use convert;
use strings;
use crypto;
use maps;

# --- types ------------------------------------------------------------------

/**
 * Connection settings for `connectWith` / `listenWith`. `defaultOptions()`
 * gives the NULL mechanism, no identity, a 5 s timeout and a 64 MiB message cap;
 * a zero numeric field also takes its default.
 * @field mechanism {string} the security mechanism: "NULL" (or "") or "PLAIN"; both peers must use the same one
 * @field username {string} PLAIN: the username a client sends, or the one a listener accepts
 * @field password {string} PLAIN: the password a client sends, or the one a listener accepts
 * @field identity {bytes} the routing id a ROUTER peer sees for this REQ / DEALER / ROUTER socket (empty: the ROUTER picks one); at most 255 bytes, not starting with a zero byte
 * @field timeoutMs {int} how long to wait for a connection and its handshake, and for the rest of a message once `poll` has seen its start, in milliseconds (0 = 5000)
 * @field maxMessageBytes {int} the largest message accepted from the peer, all frames together (0 = 64 MiB); a larger one throws `Error{kind: "limit"}`
 */
export def struct Options {
    mechanism as string,
    username as string,
    password as string,
    identity as bytes,
    timeoutMs as int,
    maxMessageBytes as int
};

/**
 * An open connection to one peer, after the ZMTP handshake. Value-semantic,
 * but every copy shares the one connection.
 * @field conn {net.Conn} the TCP connection
 * @field kind {string} this socket's type ("REQ", "SUB", ...)
 * @field peerKind {string} the peer's socket type, from its handshake
 * @field peerIdentity {bytes} the peer's routing id: the identity it sent, or for a ROUTER one generated when it sent none
 * @field peerMinor {int} the peer's ZMTP minor version (0 or 1; decides how subscriptions are sent)
 * @field timeoutMs {int} the timeout from the options, in milliseconds
 * @field maxMessageBytes {int} the message size cap from the options
 * @field subscriptions {list of bytes} a SUB socket's topic prefixes (see `subscribe`)
 */
export def struct Socket {
    conn as net.Conn,
    kind as string,
    peerKind as string,
    peerIdentity as bytes,
    peerMinor as int,
    timeoutMs as int,
    maxMessageBytes as int,
    subscriptions as list of bytes
};

/**
 * A listening socket from `listen` / `listenWith`; `accept` takes one peer from it.
 * @field listener {net.Listener} the TCP listener
 * @field kind {string} the socket type every accepted socket gets
 * @field opts {Options} the options every accepted socket gets
 */
export def struct Listener {
    listener as net.Listener,
    kind as string,
    opts as Options
};

/**
 * One received message. `frames` is the message body. `envelope` is what a REP
 * or ROUTER socket needs to answer it with `reply`: the routing frames a REP
 * socket stripped, or for a ROUTER the peer's routing id. It is empty for the
 * other socket types.
 * @field frames {list of bytes} the message frames, in order
 * @field envelope {list of bytes} the reply envelope (REP / ROUTER only)
 */
export def struct Message {
    frames as list of bytes,
    envelope as list of bytes
};

# One wire frame: the flags byte and the body. Private.
def struct Frame {
    flags as int,
    body as bytes
};

# One decoded command frame: its name and its data. Private.
def struct Command {
    name as string,
    data as bytes
};

# What a handshake learned about the peer. Private.
def struct PeerInfo {
    kind as string,
    identity as bytes,
    minor as int
};

# --- constants --------------------------------------------------------------

def const FLAG_MORE as int init 1;
def const FLAG_LONG as int init 2;
def const FLAG_COMMAND as int init 4;
def const DEFAULT_TIMEOUT_MS as int init 5000;
def const DEFAULT_MAX_MESSAGE_BYTES as int init 67108864;
# The most frames one message may carry, so a peer cannot pile up empty frames.
def const MAX_FRAMES as int init 65536;

# The peer types each socket type may talk to (RFC 37, "Socket semantics").
def const PEERS as map of string to list of string init {
    "PAIR": ["PAIR"],
    "PUB": ["SUB", "XSUB"],
    "SUB": ["PUB", "XPUB"],
    "REQ": ["REP", "ROUTER"],
    "REP": ["REQ", "DEALER"],
    "DEALER": ["REP", "DEALER", "ROUTER"],
    "ROUTER": ["REQ", "DEALER", "ROUTER"],
    "PULL": ["PUSH"],
    "PUSH": ["PULL"]
};

# --- small helpers -------------------------------------------------------------

func fail(msg as string) {
    throw Error{kind: "zmq", message: "zmq: " + $msg};
}

func utf8(s as string) {
    return convert.bytesFromString($s, "utf-8");
}

func emptyBytes() {
    def b as bytes;
    return $b;
}

# beBytes encodes n as `width` big-endian bytes.
func beBytes(n as int, width as int) {
    def out as bytes;
    def shift as int init ($width - 1) * 8;
    while ($shift >= 0) {
        $out[] = ($n >> $shift) & 0xff;
        $shift = $shift - 8;
    }
    return $out;
}

# beInt decodes big-endian bytes.
func beInt(b as bytes) {
    def n as int init 0;
    for (def i in 0..len($b)) {
        $n = ($n << 8) | $b[$i];
    }
    return $n;
}

func isKnownKind(kind as string) {
    for (def k in PEERS) {
        if ($k == $kind) {
            return true;
        }
    }
    return false;
}

func checkKind(kind as string) {
    if (not isKnownKind($kind)) {
        fail("unknown socket type '" + $kind +
            "' (want PAIR, PUB, SUB, REQ, REP, DEALER, ROUTER, PULL or PUSH)");
    }
}

# normalized fills in the defaults and checks the options.
func normalized(opts as Options) {
    def o as Options init $opts;
    if ($o.mechanism == "") {
        $o.mechanism = "NULL";
    }
    if ($o.mechanism != "NULL" and $o.mechanism != "PLAIN") {
        fail("unsupported mechanism '" + $o.mechanism + "' (want NULL or PLAIN)");
    }
    if ($o.timeoutMs < 0 or $o.maxMessageBytes < 0) {
        fail("timeoutMs and maxMessageBytes must not be negative");
    }
    if ($o.timeoutMs == 0) {
        $o.timeoutMs = DEFAULT_TIMEOUT_MS;
    }
    if ($o.maxMessageBytes == 0) {
        $o.maxMessageBytes = DEFAULT_MAX_MESSAGE_BYTES;
    }
    if (len($o.identity) > 255 or (len($o.identity) > 0 and $o.identity[0] == 0)) {
        fail("identity must be at most 255 bytes and must not start with a zero byte");
    }
    if (len(utf8($o.username)) > 255 or len(utf8($o.password)) > 255) {
        fail("PLAIN username and password must each be at most 255 bytes");
    }
    # An empty password (an unset config value, typically) would make a PLAIN
    # listener accept anyone who sends empty credentials.
    if ($o.mechanism == "PLAIN" and $o.password == "") {
        fail("PLAIN needs a non-empty password");
    }
    return $o;
}

# --- wire encoding ----------------------------------------------------------------

# encodeFrame builds one frame: flags, a 1- or 8-byte length, then the body.
func encodeFrame(flags as int, body as bytes) {
    def n as int init len($body);
    def out as bytes;
    if ($n > 255) {
        $out[] = $flags | FLAG_LONG;
        $out = binary.concat($out, beBytes($n, 8));
    } else {
        $out[] = $flags;
        $out[] = $n;
    }
    return binary.concat($out, $body);
}

# encodeCommand builds a command frame: a name-length byte, the name, the data.
func encodeCommand(name as string, data as bytes) {
    def body as bytes;
    def nameBytes as bytes init utf8($name);
    $body[] = len($nameBytes);
    return encodeFrame(FLAG_COMMAND, binary.concat(binary.concat($body, $nameBytes), $data));
}

# encodeMessage builds the frames of one message, MORE set on all but the last.
func encodeMessage(frames as list of bytes) {
    def parts as list of bytes init [];
    def last as int init len($frames) - 1;
    for (def i in 0..len($frames)) {
        def flags as int init 0;
        if ($i < $last) {
            $flags = FLAG_MORE;
        }
        $parts[] = encodeFrame($flags, $frames[$i]);
    }
    return binary.join($parts);
}

# metadata encodes handshake properties: name length, name, 4-byte value
# length, value.
func metadata(kind as string, identity as bytes) {
    def out as bytes;
    def names as list of string init ["Socket-Type"];
    def values as list of bytes init [utf8($kind)];
    if (len($identity) > 0) {
        $names[] = "Identity";
        $values[] = $identity;
    }
    for (def i in 0..len($names)) {
        def n as bytes init utf8($names[$i]);
        $out[] = len($n);
        $out = binary.concat(binary.concat($out, $n), beBytes(len($values[$i]), 4));
        $out = binary.concat($out, $values[$i]);
    }
    return $out;
}

# parseMetadata reads handshake properties into a map keyed by lower-case
# name (names are case-insensitive).
func parseMetadata(data as bytes) {
    def out as map of string to bytes init {};
    def i as int init 0;
    def n as int init len($data);
    while ($i < $n) {
        def nl as int init $data[$i];
        if ($i + 1 + $nl + 4 > $n) {
            fail("malformed handshake metadata");
        }
        def name as string init strings.lower(
            convert.stringFromBytes($data[$i + 1..$i + 1 + $nl], "utf-8"));
        $i = $i + 1 + $nl;
        def vl as int init beInt($data[$i..$i + 4]);
        $i = $i + 4;
        if ($i + $vl > $n) {
            fail("malformed handshake metadata");
        }
        $out[$name] = $data[$i..$i + $vl];
        $i = $i + $vl;
    }
    return $out;
}

# greeting builds the 64-byte ZMTP 3.1 greeting.
func greeting(mechanism as string, asServer as bool) {
    def out as bytes;
    $out[] = 0xff;
    for (def i in 0..8) {
        $out[] = 0;
    }
    $out[] = 0x7f;
    $out[] = 3;
    $out[] = 1;
    def mech as bytes init utf8($mechanism);
    for (def i in 0..20) {
        if ($i < len($mech)) {
            $out[] = $mech[$i];
        } else {
            $out[] = 0;
        }
    }
    if ($asServer) {
        $out[] = 1;
    } else {
        $out[] = 0;
    }
    for (def i in 0..31) {
        $out[] = 0;
    }
    return $out;
}

# --- wire decoding ----------------------------------------------------------------

# readFrameAfter reads the rest of a frame whose flags byte has been read.
# `budget` is how many body bytes the message may still take.
func readFrameAfter(conn as net.Conn, flags as int, budget as int) {
    def size as int init 0;
    if (($flags & FLAG_LONG) != 0) {
        $size = beInt(net.readN($conn, 8));
        if ($size < 0) {
            fail("frame length out of range");
        }
    } else {
        $size = net.readN($conn, 1)[0];
    }
    if ($size > $budget) {
        throw Error{
            kind: "limit",
            message: "zmq: a message from the peer is larger than maxMessageBytes"
        };
    }
    def body as bytes;
    if ($size > 0) {
        $body = net.readN($conn, $size);
    }
    return Frame{flags: $flags, body: $body};
}

func readFrame(conn as net.Conn, budget as int) {
    return readFrameAfter($conn, net.readN($conn, 1)[0], $budget);
}

func parseCommand(f as Frame) {
    if (($f.flags & FLAG_COMMAND) == 0) {
        fail("expected a command frame during the handshake");
    }
    def b as bytes init $f.body;
    # libzmq 4.3 writes the ERROR it sends after a failed ZAP check as
    # 0x5e "RROR" <reason>: the name-length byte and the "E" merged into one,
    # one byte short. Read that as the ERROR it is, so the reason gets through.
    if (len($b) >= 5 and $b[0] == 0x5e and convert.stringFromBytes($b[1..5], "utf-8") == "RROR") {
        return Command{name: "ERROR", data: $b[5..]};
    }
    if (len($b) < 1 or 1 + $b[0] > len($b)) {
        fail("malformed command frame");
    }
    def nl as int init $b[0];
    return Command{name: convert.stringFromBytes($b[1..1 + $nl], "utf-8"), data: $b[1 + $nl..]};
}

# expectCommand reads the next command, turning a peer ERROR into a zmq error.
func expectCommand(conn as net.Conn, want as string) {
    def c as Command init parseCommand(readFrame($conn, 65536));
    if ($c.name == "ERROR") {
        fail("peer refused the handshake: " + errorReason($c.data));
    }
    if ($c.name != $want) {
        fail("expected " + $want + " from the peer, got " + $c.name);
    }
    return $c;
}

func errorReason(data as bytes) {
    if (len($data) < 1 or 1 + $data[0] > len($data)) {
        return "(no reason given)";
    }
    return convert.stringFromBytes($data[1..1 + $data[0]], "utf-8");
}

func sendError(conn as net.Conn, reason as string) {
    def body as bytes;
    def r as bytes init utf8($reason);
    $body[] = len($r);
    try {
        net.writeBytes($conn, encodeCommand("ERROR", binary.concat($body, $r)));
    } catch (e) { # lint-disable: L103
        # the peer may already be gone; the handshake fails either way
    }
}

# --- handshake --------------------------------------------------------------

# peerFrom checks the peer's READY / INITIATE metadata against our socket type.
func peerFrom(conn as net.Conn, kind as string, data as bytes, minor as int) {
    def props as map of string to bytes init parseMetadata($data);
    if (not maps.has($props, "socket-type")) {
        sendError($conn, "missing Socket-Type");
        fail("the peer sent no socket type");
    }
    def peerKind as string init convert.stringFromBytes($props["socket-type"], "utf-8");
    def allowed as bool init false;
    for (def k in PEERS[$kind]) {
        if ($k == $peerKind) {
            $allowed = true;
        }
    }
    if (not $allowed) {
        sendError($conn, "invalid socket type");
        fail("a " + $kind + " socket cannot talk to a " + $peerKind + " peer");
    }
    def identity as bytes;
    if (maps.has($props, "identity")) {
        $identity = $props["identity"];
    }
    if ($kind == "ROUTER" and len($identity) == 0) {
        # libzmq's form for a generated routing id: a zero byte, then 4 bytes
        $identity[] = 0;
        $identity = binary.concat($identity, crypto.randBytes(4));
    }
    return PeerInfo{kind: $peerKind, identity: $identity, minor: $minor};
}

# handshake runs the ZMTP greeting and the mechanism's commands on a fresh
# connection. asServer is true on the accepting side (the PLAIN server).
func handshake(conn as net.Conn, kind as string, opts as Options, asServer as bool) {
    net.setReadDeadline($conn, $opts.timeoutMs);
    net.writeBytes($conn, greeting($opts.mechanism, $asServer));
    def g as bytes init net.readN($conn, 64);
    if ($g[0] != 0xff or $g[9] != 0x7f) {
        fail("the peer is not speaking ZMTP (bad greeting signature)");
    }
    if ($g[10] < 3) {
        fail("the peer speaks ZMTP " + convert.toString($g[10]) + ".x; ZMTP 3 is required");
    }
    def minor as int init $g[11];
    if ($g[10] > 3) {
        $minor = 1;
    }
    def mechEnd as int init 12;
    while ($mechEnd < 32 and $g[$mechEnd] != 0) {
        $mechEnd = $mechEnd + 1;
    }
    def peerMech as string init convert.stringFromBytes($g[12..$mechEnd], "utf-8");
    if ($peerMech != $opts.mechanism) {
        fail("the peer uses the " + $peerMech + " mechanism, this socket " + $opts.mechanism);
    }
    def ours as bytes init metadata($kind, $opts.identity);
    def info as PeerInfo;
    if ($opts.mechanism == "NULL") {
        net.writeBytes($conn, encodeCommand("READY", $ours));
        $info = peerFrom($conn, $kind, expectCommand($conn, "READY").data, $minor);
    } elseif ($asServer) {
        $info = plainServer($conn, $kind, $opts, $ours, $minor);
    } else {
        # The connecting side is the PLAIN client. The greeting's as-server byte is
        # not checked: libzmq leaves it 0 on a PLAIN server too.
        $info = plainClient($conn, $kind, $opts, $ours, $minor);
    }
    net.setReadDeadline($conn, 0);
    return Socket{
        conn: $conn,
        kind: $kind,
        peerKind: $info.kind,
        peerIdentity: $info.identity,
        peerMinor: $info.minor,
        timeoutMs: $opts.timeoutMs,
        maxMessageBytes: $opts.maxMessageBytes,
        subscriptions: []
    };
}

func plainClient(conn as net.Conn, kind as string, opts as Options, ours as bytes, minor as int) {
    def hello as bytes;
    def u as bytes init utf8($opts.username);
    def p as bytes init utf8($opts.password);
    $hello[] = len($u);
    $hello = binary.concat($hello, $u);
    $hello[] = len($p);
    $hello = binary.concat($hello, $p);
    net.writeBytes($conn, encodeCommand("HELLO", $hello));
    expectCommand($conn, "WELCOME");
    net.writeBytes($conn, encodeCommand("INITIATE", $ours));
    return peerFrom($conn, $kind, expectCommand($conn, "READY").data, $minor);
}

func plainServer(conn as net.Conn, kind as string, opts as Options, ours as bytes, minor as int) {
    def hello as bytes init expectCommand($conn, "HELLO").data;
    def ok as bool init false;
    if (len($hello) >= 1 and 1 + $hello[0] < len($hello)) {
        def ul as int init $hello[0];
        def u as bytes init $hello[1..1 + $ul];
        def pl as int init $hello[1 + $ul];
        if (2 + $ul + $pl == len($hello)) {
            def p as bytes init $hello[2 + $ul..];
            # both compared in constant time, and both always compared
            def userOk as bool init crypto.hmacEqual($u, utf8($opts.username));
            def passOk as bool init crypto.hmacEqual($p, utf8($opts.password));
            $ok = $userOk and $passOk;
        }
    }
    if (not $ok) {
        sendError($conn, "invalid username or password");
        fail("PLAIN authentication failed");
    }
    net.writeBytes($conn, encodeCommand("WELCOME", emptyBytes()));
    def info as PeerInfo init peerFrom($conn, $kind, expectCommand($conn, "INITIATE").data, $minor);
    net.writeBytes($conn, encodeCommand("READY", $ours));
    return $info;
}

# --- connecting -----------------------------------------------------------------

/**
 * The default options: NULL mechanism, no identity, a 5 s timeout and a 64 MiB
 * message cap. Set fields on the result for PLAIN or an identity.
 * @return {Options} the defaults
 */
export func defaultOptions() {
    def none as bytes;
    return Options{
        mechanism: "NULL",
        username: "",
        password: "",
        identity: $none,
        timeoutMs: DEFAULT_TIMEOUT_MS,
        maxMessageBytes: DEFAULT_MAX_MESSAGE_BYTES
    };
}

/**
 * Connect a socket of type `kind` to a peer listening at `address` and run the
 * handshake, with the default options.
 * @param kind {string} the socket type: "PAIR", "PUB", "SUB", "REQ", "REP", "DEALER", "ROUTER", "PULL" or "PUSH"
 * @param address {string} the peer, "host:port" (no "tcp://" prefix)
 * @return {Socket} the connected socket
 * @throws {Error} kind "zmq" for an unknown type, a handshake failure or an incompatible peer; a net error (kind "timeout", "closed", "runtime") if the connection fails
 */
export func connect(kind as string, address as string) {
    return connectWith($kind, $address, defaultOptions());
}

/**
 * Connect like `connect`, with options (a PLAIN client, an identity, a timeout).
 * @param kind {string} the socket type
 * @param address {string} the peer, "host:port"
 * @param opts {Options} the options
 * @return {Socket} the connected socket
 * @throws {Error} kind "zmq" for an unknown type, bad options, a refused or failed handshake; a net error if the connection fails
 */
export func connectWith(kind as string, address as string, opts as Options) {
    checkKind($kind);
    def o as Options init normalized($opts);
    def conn as net.Conn init net.connect(stripScheme($address), $o.timeoutMs);
    errdefer net.close($conn);
    return handshake($conn, $kind, $o, false);
}

# stripScheme accepts a libzmq-style "tcp://host:port" too.
func stripScheme(address as string) {
    if (strings.startsWith($address, "tcp://")) {
        return $address[6..];
    }
    if (strings.contains($address, "://")) {
        fail("only TCP is supported, not '" + $address + "'");
    }
    return $address;
}

/**
 * Listen for peers at `address` with the default options. Each `accept` takes
 * one peer and returns a socket of type `kind` for it.
 * @param kind {string} the socket type of every accepted socket
 * @param address {string} where to listen, "host:port" (":0" picks a free port)
 * @return {Listener} the listener
 */
export func listen(kind as string, address as string) {
    return listenWith($kind, $address, defaultOptions());
}

/**
 * Listen like `listen`, with options. For PLAIN, the listener is the server:
 * it accepts a peer only if it sends `opts.username` and `opts.password`.
 * @param kind {string} the socket type of every accepted socket
 * @param address {string} where to listen, "host:port"
 * @param opts {Options} the options every accepted socket gets
 * @return {Listener} the listener
 */
export func listenWith(kind as string, address as string, opts as Options) {
    checkKind($kind);
    def o as Options init normalized($opts);
    return Listener{listener: net.listen(stripScheme($address)), kind: $kind, opts: $o};
}

/**
 * Wait for the next peer to connect, run the handshake, and return a socket for
 * it. A peer whose handshake fails (wrong mechanism, bad PLAIN credentials,
 * incompatible socket type) is disconnected and the call throws; call `accept`
 * again for the next one.
 * @param l {Listener} the listener
 * @return {Socket} the socket for the new peer
 * @throws {Error} kind "zmq" if the peer's handshake fails
 */
export func accept(l as Listener) {
    def conn as net.Conn init net.accept($l.listener);
    errdefer net.close($conn);
    return handshake($conn, $l.kind, $l.opts, true);
}

/**
 * The address a listener is bound to, "host:port" - the real port after
 * listening on ":0".
 * @param l {Listener} the listener
 * @return {string} the bound address
 */
export func address(l as Listener) {
    return net.address($l.listener);
}

/**
 * Close a socket's connection.
 * @param s {Socket} the socket
 */
export func close(s as Socket) {
    net.close($s.conn);
    return null;
}

/**
 * Stop listening.
 * @param l {Listener} the listener
 */
export func closeListener(l as Listener) {
    net.close($l.listener);
    return null;
}

# --- sending ------------------------------------------------------------------

/**
 * Send a message, one frame per list element. A REQ socket adds the empty
 * delimiter frame REP peers expect. A ROUTER socket takes the peer's routing id
 * as the first frame (as libzmq's ROUTER does) and does not send it. REP sockets
 * answer with `reply`; SUB and PULL sockets cannot send.
 * @param s {Socket} the socket
 * @param frames {list of bytes} the message frames (at least one)
 * @throws {Error} kind "zmq" if the socket type cannot send, the message is empty, or (ROUTER) the routing id is not the peer's
 */
export func send(s as Socket, frames as list of bytes) {
    if (len($frames) == 0) {
        fail("a message needs at least one frame");
    }
    def wire as list of bytes init $frames;
    match ($s.kind) {
        when "SUB", "PULL" { fail("a " + $s.kind + " socket cannot send"); }
        when "REP" { fail("a REP socket answers with zmq.reply, not zmq.send"); }
        when "REQ" { $wire = [emptyBytes()];
        for (def f in $frames) {
            $wire[] = $f;
        }
        }
        when "ROUTER" {
            if ($frames[0] != $s.peerIdentity) {
                fail("the routing id is not this socket's peer");
            }
            if (len($frames) < 2) {
                fail("a ROUTER message needs the routing id and at least one frame");
            }
            $wire = $frames[1..];
        }
    }
    net.writeBytes($s.conn, encodeMessage($wire));
    return null;
}

/**
 * Send a one-frame text message (UTF-8), with `send`'s rules.
 * @param s {Socket} the socket
 * @param text {string} the message
 */
export func sendText(s as Socket, text as string) {
    return send($s, [utf8($text)]);
}

/**
 * Answer a request received on a REP or ROUTER socket. On REP the request's
 * envelope (the routing frames and the empty delimiter) goes back in front of
 * `frames`, so the reply reaches its requester through any ROUTER in between.
 * On ROUTER the request's routing id selects the peer and `frames` are sent as
 * they are - include the empty delimiter frame yourself when the peer is a REQ.
 * @param s {Socket} the REP or ROUTER socket
 * @param request {Message} the request being answered
 * @param frames {list of bytes} the reply frames (at least one)
 */
export func reply(s as Socket, request as Message, frames as list of bytes) {
    if (len($frames) == 0) {
        fail("a message needs at least one frame");
    }
    def wire as list of bytes init [];
    if ($s.kind == "REP") {
        for (def f in $request.envelope) {
            $wire[] = $f;
        }
        $wire[] = emptyBytes();
    } elseif ($s.kind == "ROUTER") {
        if (len($request.envelope) != 1 or $request.envelope[0] != $s.peerIdentity) {
            fail("the request did not come from this socket's peer");
        }
    } else {
        fail("zmq.reply needs a REP or ROUTER socket, not " + $s.kind);
    }
    for (def f in $frames) {
        $wire[] = $f;
    }
    net.writeBytes($s.conn, encodeMessage($wire));
    return null;
}

/**
 * A copy of a SUB socket that receives messages whose first frame starts with
 * `prefix` ("" subscribes to everything). The subscription is sent to the
 * publisher, which may then filter at its end; `receive` filters too. A SUB
 * socket with no subscription receives nothing.
 * @param s {Socket} the SUB socket
 * @param prefix {string} the topic prefix
 * @return {Socket} the socket with the subscription added
 */
export func subscribe(s as Socket, prefix as string) {
    sendSubscription($s, utf8($prefix), true);
    $s.subscriptions[] = utf8($prefix);
    return $s;
}

/**
 * A copy of a SUB socket with one subscription to `prefix` removed (the
 * publisher is told). Removing a prefix that was never subscribed does nothing.
 * @param s {Socket} the SUB socket
 * @param prefix {string} the topic prefix
 * @return {Socket} the socket with the subscription removed
 */
export func unsubscribe(s as Socket, prefix as string) {
    if ($s.kind != "SUB") {
        fail("only a SUB socket subscribes, not " + $s.kind);
    }
    def p as bytes init utf8($prefix);
    def kept as list of bytes init [];
    def removed as bool init false;
    for (def sub in $s.subscriptions) {
        if (not $removed and $sub == $p) {
            $removed = true;
        } else {
            $kept[] = $sub;
        }
    }
    if ($removed) {
        sendSubscription($s, $p, false);
    }
    $s.subscriptions = $kept;
    return $s;
}

# sendSubscription tells the publisher: a SUBSCRIBE / CANCEL command to a ZMTP
# 3.1 peer, a message starting with byte 1 / 0 to a ZMTP 3.0 one.
func sendSubscription(s as Socket, prefix as bytes, on as bool) {
    if ($s.kind != "SUB") {
        fail("only a SUB socket subscribes, not " + $s.kind);
    }
    if ($s.peerMinor >= 1) {
        def name as string init "CANCEL";
        if ($on) {
            $name = "SUBSCRIBE";
        }
        net.writeBytes($s.conn, encodeCommand($name, $prefix));
    } else {
        def body as bytes;
        if ($on) {
            $body[] = 1;
        } else {
            $body[] = 0;
        }
        net.writeBytes($s.conn, encodeFrame(0, binary.concat($body, $prefix)));
    }
    return null;
}

# --- receiving ----------------------------------------------------------------

# readRest reads the rest of a message whose first frame's flags byte is in
# hand. A command between messages is answered (PING) or rejected (ERROR) and
# skipped; it returns an empty list when only a command came.
func readRest(s as Socket, firstFlags as int) {
    def frames as list of bytes init [];
    def budget as int init $s.maxMessageBytes;
    def flags as int init $firstFlags;
    while (true) {
        def f as Frame init readFrameAfter($s.conn, $flags, $budget);
        if (($f.flags & FLAG_COMMAND) != 0) {
            handleCommand($s, parseCommand($f));
            if (len($frames) == 0) {
                return $frames;
            }
        } else {
            $frames[] = $f.body;
            $budget = $budget - len($f.body);
            if (len($frames) > MAX_FRAMES) {
                throw Error{
                    kind: "limit",
                    message: "zmq: a message from the peer has too many frames"
                };
            }
            if (($f.flags & FLAG_MORE) == 0) {
                return $frames;
            }
        }
        $flags = net.readN($s.conn, 1)[0];
    }
    return $frames;
}

func handleCommand(s as Socket, c as Command) {
    match ($c.name) {
        when "PING" {
            # PING: a 2-byte TTL, then a context the PONG echoes back
            def ctx as bytes;
            if (len($c.data) > 2) {
                $ctx = $c.data[2..];
            }
            net.writeBytes($s.conn, encodeCommand("PONG", $ctx));
        }
        when "ERROR" { fail("the peer reported an error: " + errorReason($c.data)); }
    }
    return null;
}

# deliver applies the socket type's receive rules to a raw message: a list of
# zero or one Message (empty when the message is dropped).
func deliver(s as Socket, frames as list of bytes) {
    def out as list of Message init [];
    def none as list of bytes init [];
    match ($s.kind) {
        when "PUB", "PUSH" {
            # these never receive; a peer's data (stray subscriptions) is dropped
            return $out;
        }
        when "REQ" {
            if (len($frames) < 2 or len($frames[0]) != 0) {
                return $out; # not a reply envelope: dropped, as libzmq does
            }
            $out[] = Message{frames: $frames[1..], envelope: $none};
        }
        when "REP" {
            def d as int init -1;
            for (def i in 0..len($frames)) {
                if ($d < 0 and len($frames[$i]) == 0) {
                    $d = $i;
                }
            }
            if ($d < 0 or $d == len($frames) - 1) {
                return $out; # no delimiter, or no body: dropped
            }
            $out[] = Message{frames: $frames[$d + 1..], envelope: $frames[..$d]};
        }
        when "ROUTER" { $out[] = Message{frames: $frames, envelope: [$s.peerIdentity]}; }
        when "SUB" {
            for (def p in $s.subscriptions) {
                if (len($out) == 0 and binary.startsWith($frames[0], $p)) {
                    $out[] = Message{frames: $frames, envelope: $none};
                }
            }
        }
        else { $out[] = Message{frames: $frames, envelope: $none}; }
    }
    return $out;
}

/**
 * Block until the next message arrives and return it. Messages the socket type
 * drops - a SUB message no subscription matches, a malformed REQ / REP envelope -
 * are skipped. A PING from the peer is answered on the way.
 * @param s {Socket} the socket
 * @return {Message} the next message
 * @throws {Error} kind "zmq" if the peer reports an error or the socket type cannot receive; kind "limit" for a message over maxMessageBytes; kind "closed" when the peer disconnects
 */
export func receive(s as Socket) {
    if ($s.kind == "PUB" or $s.kind == "PUSH") {
        fail("a " + $s.kind + " socket cannot receive");
    }
    net.setReadDeadline($s.conn, 0);
    while (true) {
        def frames as list of bytes init readRest($s, net.readN($s.conn, 1)[0]);
        if (len($frames) > 0) {
            def got as list of Message init deliver($s, $frames);
            if (len($got) > 0) {
                return $got[0];
            }
        }
    }
    return Message{frames: [], envelope: []};
}

/**
 * Wait up to `timeoutMs` for a message. Returns a list of zero or one Message:
 * empty when nothing arrived in time (or only a command or a dropped message
 * did), one Message otherwise. Once a message has started arriving, the rest of
 * it must come within the socket's `timeoutMs` option.
 * @param s {Socket} the socket
 * @param timeoutMs {int} how long to wait, in milliseconds
 * @return {list of Message} zero or one message
 */
export func poll(s as Socket, timeoutMs as int) {
    if ($s.kind == "PUB" or $s.kind == "PUSH") {
        fail("a " + $s.kind + " socket cannot receive");
    }
    def none as list of Message init [];
    net.setReadDeadline($s.conn, $timeoutMs);
    def first as int init 0;
    try {
        $first = net.readN($s.conn, 1)[0];
    } catch (e) {
        net.setReadDeadline($s.conn, 0);
        if ($e.kind == "timeout") {
            return $none;
        }
        throw $e;
    }
    net.setReadDeadline($s.conn, $s.timeoutMs);
    def frames as list of bytes init readRest($s, $first);
    net.setReadDeadline($s.conn, 0);
    if (len($frames) == 0) {
        return $none;
    }
    return deliver($s, $frames);
}

/**
 * The first frame of a message as UTF-8 text.
 * @param m {Message} the message
 * @return {string} the text ("" for a message with no frames)
 */
export func text(m as Message) {
    if (len($m.frames) == 0) {
        return "";
    }
    return convert.stringFromBytes($m.frames[0], "utf-8");
}
