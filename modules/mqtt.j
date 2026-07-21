# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * An MQTT 3.1.1 publish/subscribe client over the `net` system library - the
 * same "protocol clients are modules, `net` is the transport" line the other
 * network clients follow. MQTT packets are a 1-byte fixed header, a variable
 * remaining-length integer, then a length-prefixed payload, all built and
 * parsed here with Jennifer's bitwise operators and `bytes`. Basics-first: QoS
 * 0 publish / subscribe, a single-threaded `poll` with timeout (via
 * `net.setDeadline`) so one flow can wait for a packet and send keepalives
 * without a spawned reader, plus blocking `receive`, `ping`, and `disconnect`.
 * QoS 1/2, retained messages, the will, and reconnect are out of the basics.
 * Needs the default `jennifer` binary (uses `net`).
 * @module mqtt
 * @example
 * def c as mqtt.Client init mqtt.connect(mqtt.Options{host: "127.0.0.1", port: 1883, clientId: "demo", keepalive: 30, security: "none", username: "", password: ""});
 * mqtt.subscribe($c, "sensors/temp");
 * mqtt.publish($c, "sensors/temp", "21.5");
 * def m as mqtt.Message init mqtt.receive($c);
 * mqtt.disconnect($c);
 */
use net;
use convert;
use strings;

# --- types ------------------------------------------------------------------

/**
 * Connection settings for mqtt.connect.
 * @field host {string} the broker host
 * @field port {int} the broker port (1883 plaintext, 8883 TLS by convention)
 * @field clientId {string} the client identifier the broker sees
 * @field keepalive {int} the keepalive interval in seconds (0 disables)
 * @field security {string} "none" (plaintext) or "tls" (mqtts)
 * @field username {string} the CONNECT username ("" to omit)
 * @field password {string} the CONNECT password ("" to omit)
 */
export def struct Options {
    host as string,
    port as int,
    clientId as string,
    keepalive as int,
    security as string,
    username as string,
    password as string
};

/**
 * An open MQTT connection.
 * @field conn {net.Conn} the underlying socket
 */
export def struct Client {
    conn as net.Conn
};

/**
 * One received application message.
 * @field topic {string} the topic it was published to
 * @field payload {bytes} the raw message bytes (convert to text as needed)
 */
export def struct Message {
    topic as string,
    payload as bytes
};

# One decoded control packet: the type nibble, the flags nibble, and the
# variable-header-plus-payload body. Private (never in an exported signature).
def struct Packet {
    typ as int,
    flags as int,
    body as bytes
};

# The result of decoding a remaining-length varint: its value and how many
# bytes it occupied. Private.
def struct DecodedLen {
    value as int,
    size as int
};

# --- byte building (pure helpers) -------------------------------------------

# appendBytes copies every byte of src onto dst and returns dst.
func appendBytes(dst as bytes, src as bytes) {
    def i as int init 0;
    while ($i < len($src)) {
        $dst[] = $src[$i];
        $i = $i + 1;
    }
    return $dst;
}

# sliceBytes returns src[start:end] as a fresh bytes value.
func sliceBytes(src as bytes, start as int, end as int) {
    def out as bytes;
    def i as int init $start;
    while ($i < $end) {
        $out[] = $src[$i];
        $i = $i + 1;
    }
    return $out;
}

# putString appends an MQTT UTF-8 string (2-byte big-endian length prefix, then
# the bytes) to b and returns b.
func putString(b as bytes, s as string) {
    def raw as bytes init convert.bytesFromString($s, "utf-8");
    def n as int init len($raw);
    $b[] = ($n >> 8) & 0xff;
    $b[] = $n & 0xff;
    return appendBytes($b, $raw);
}

# encodeRemLen encodes a remaining-length as MQTT's 1-to-4-byte varint (7 bits
# per byte, high bit = continuation).
func encodeRemLen(n as int) {
    def out as bytes;
    def x as int init $n;
    repeat {
        def enc as int init $x & 0x7f;
        $x = $x >> 7;
        if ($x > 0) {
            $enc = $enc | 0x80;
        }
        $out[] = $enc;
    } until ($x == 0);
    return $out;
}

# decodeRemLen decodes a remaining-length varint from buf starting at `start`.
func decodeRemLen(buf as bytes, start as int) {
    def mult as int init 1;
    def value as int init 0;
    def i as int init $start;
    def more as bool init true;
    repeat {
        def b as int init $buf[$i];
        $value = $value + ($b & 0x7f) * $mult;
        $mult = $mult * 128;
        $i = $i + 1;
        if (($b & 0x80) == 0) {
            $more = false;
        }
    } until (not $more);
    return DecodedLen{ value: $value, size: $i - $start };
}

# frame assembles a control packet: the fixed-header byte, the encoded
# remaining length, then the variable header and payload.
func frame(header as int, vh as bytes, pl as bytes) {
    def out as bytes;
    $out[] = $header;
    def total as int init len($vh) + len($pl);
    $out = appendBytes($out, encodeRemLen($total));
    $out = appendBytes($out, $vh);
    $out = appendBytes($out, $pl);
    return $out;
}

# buildConnect builds the CONNECT packet (type 1) for the given options.
func buildConnect(opts as Options) {
    def vh as bytes;
    $vh = putString($vh, "MQTT");
    $vh[] = 4;
    def flags as int init 0x02;
    if (len($opts.username) > 0) {
        $flags = $flags | 0x80;
    }
    if (len($opts.password) > 0) {
        $flags = $flags | 0x40;
    }
    $vh[] = $flags;
    $vh[] = ($opts.keepalive >> 8) & 0xff;
    $vh[] = $opts.keepalive & 0xff;
    def pl as bytes;
    $pl = putString($pl, $opts.clientId);
    if (len($opts.username) > 0) {
        $pl = putString($pl, $opts.username);
    }
    if (len($opts.password) > 0) {
        $pl = putString($pl, $opts.password);
    }
    return frame(0x10, $vh, $pl);
}

# parsePublish turns a PUBLISH packet's body into a Message. QoS>0 packets carry
# a 2-byte packet identifier after the topic, which basics-QoS-0 skips over
# (there is no PUBACK path yet).
func parsePublish(pkt as Packet) {
    def body as bytes init $pkt.body;
    def tlen as int init (($body[0] << 8) | $body[1]);
    def topic as string init convert.stringFromBytes(sliceBytes($body, 2, 2 + $tlen), "utf-8");
    def idx as int init 2 + $tlen;
    def qos as int init ($pkt.flags >> 1) & 0x03;
    if ($qos > 0) {
        $idx = $idx + 2;
    }
    def payload as bytes init sliceBytes($body, $idx, len($body));
    return Message{ topic: $topic, payload: $payload };
}

# --- socket reading ---------------------------------------------------------

# MAX_PACKET_BYTES caps a single control packet's body. The remaining-length
# varint is attacker-declarable (up to 256 MiB), so a malicious / compromised
# broker could force an unbounded allocation; a larger declared length fails the
# read with a catchable error instead.
def const MAX_PACKET_BYTES as int init 67108864;

# capPacket throws when a declared packet length is over the cap.
func capPacket(n as int) {
    if ($n > MAX_PACKET_BYTES) {
        throw Error{ kind: "mqtt", message: "mqtt: packet declares " + convert.toString($n) + " bytes, over the " + convert.toString(MAX_PACKET_BYTES) + "-byte limit", file: "", line: 0, col: 0 };
    }
    return;
}

# readN reads exactly n bytes from conn, looping over the "up to n" net.readBytes
# until the count is met. A closed connection mid-packet is a catchable error.
func readN(conn as net.Conn, n as int) {
    capPacket($n);
    # net.readN reads the whole frame in one Go loop, not a per-byte interpreted
    # accumulation. A peer that closes before n bytes is re-tagged as the mqtt
    # mid-packet error; a timeout / other I/O error propagates unchanged.
    try {
        return net.readN($conn, $n);
    } catch (e) {
        if (strings.contains($e.message, "closed after")) {
            throw Error{ kind: "mqtt", message: "mqtt: connection closed mid-packet", file: "", line: 0, col: 0 };
        }
        throw $e;
    }
}

# readRemLen reads a remaining-length varint one byte at a time off the socket,
# then decodes it.
func readRemLen(conn as net.Conn) {
    def buf as bytes;
    def more as bool init true;
    repeat {
        def one as bytes init readN($conn, 1);
        def b as int init $one[0];
        $buf[] = $b;
        if (($b & 0x80) == 0) {
            $more = false;
        }
    } until (not $more);
    return decodeRemLen($buf, 0).value;
}

# readPacketBody reads the remaining length and body for an already-consumed
# fixed-header byte and returns the decoded Packet.
func readPacketBody(conn as net.Conn, hb as int) {
    def typ as int init ($hb >> 4) & 0x0f;
    def flags as int init $hb & 0x0f;
    def rem as int init readRemLen($conn);
    def body as bytes;
    if ($rem > 0) {
        $body = readN($conn, $rem);
    }
    return Packet{ typ: $typ, flags: $flags, body: $body };
}

# The handshake read timeout (ms), so a broker that accepts but never sends the
# CONNACK / SUBACK fails instead of blocking forever. Cleared after the ack so
# `poll` / `receive` keep managing their own deadlines.
def const HANDSHAKE_TIMEOUT_MS as int init 30000;

# --- connection lifecycle (exported) ----------------------------------------

/**
 * Open a connection, send CONNECT, and check the CONNACK return code.
 * @param opts {Options} the connection settings
 * @return {Client} the open client
 * @throws {Error} kind "mqtt" when the broker refuses the connection
 */
export func connect(opts as Options) {
    def addr as string init $opts.host + ":" + convert.toString($opts.port);
    def conn as net.Conn;
    if ($opts.security == "tls") {
        $conn = net.connectTLS($addr, HANDSHAKE_TIMEOUT_MS);
    } else {
        $conn = net.connect($addr, HANDSHAKE_TIMEOUT_MS);
    }
    # A refused / malformed CONNACK must not leak the socket; on success the
    # caller owns the open client.
    errdefer net.close($conn);
    net.writeBytes($conn, buildConnect($opts));
    net.setDeadline($conn, HANDSHAKE_TIMEOUT_MS);
    def h as bytes init readN($conn, 1);
    def pkt as Packet init readPacketBody($conn, $h[0]);
    net.setDeadline($conn, 0);
    if (not ($pkt.typ == 2)) {
        throw Error{ kind: "mqtt", message: "mqtt: expected CONNACK, got packet type " + convert.toString($pkt.typ), file: "", line: 0, col: 0 };
    }
    def code as int init $pkt.body[1];
    if (not ($code == 0)) {
        throw Error{ kind: "mqtt", message: "mqtt: connection refused, CONNACK code " + convert.toString($code), file: "", line: 0, col: 0 };
    }
    return Client{ conn: $conn };
}

/**
 * Publish a raw byte payload to a topic at QoS 0 (fire and forget).
 * @param client {Client} the open client
 * @param topic {string} the topic to publish to
 * @param payload {bytes} the message bytes
 */
export func publishBytes(client as Client, topic as string, payload as bytes) {
    def vh as bytes;
    $vh = putString($vh, $topic);
    net.writeBytes($client.conn, frame(0x30, $vh, $payload));
    return null;
}

/**
 * Publish a text message to a topic at QoS 0 (UTF-8 encoded).
 * @param client {Client} the open client
 * @param topic {string} the topic to publish to
 * @param message {string} the message text
 */
export func publish(client as Client, topic as string, message as string) {
    return publishBytes($client, $topic, convert.bytesFromString($message, "utf-8"));
}

/**
 * Subscribe to a topic filter at QoS 0 and wait for the SUBACK.
 * @param client {Client} the open client
 * @param topic {string} the topic filter (may contain `+` / `#` wildcards)
 * @throws {Error} kind "mqtt" when the broker rejects the subscription
 */
export func subscribe(client as Client, topic as string) {
    def vh as bytes;
    $vh[] = 0x00;
    $vh[] = 0x01;
    def pl as bytes;
    $pl = putString($pl, $topic);
    $pl[] = 0x00;
    net.writeBytes($client.conn, frame(0x82, $vh, $pl));
    net.setDeadline($client.conn, HANDSHAKE_TIMEOUT_MS);
    def h as bytes init readN($client.conn, 1);
    def pkt as Packet init readPacketBody($client.conn, $h[0]);
    net.setDeadline($client.conn, 0);
    if (not ($pkt.typ == 9)) {
        throw Error{ kind: "mqtt", message: "mqtt: expected SUBACK, got packet type " + convert.toString($pkt.typ), file: "", line: 0, col: 0 };
    }
    if ($pkt.body[2] == 0x80) {
        throw Error{ kind: "mqtt", message: "mqtt: subscribe rejected for topic " + $topic, file: "", line: 0, col: 0 };
    }
    return null;
}

/**
 * Block until the next application message arrives, returning it. Non-PUBLISH
 * control packets (e.g. a PINGRESP) are consumed and skipped.
 * @param client {Client} the open client
 * @return {Message} the next received message
 */
export func receive(client as Client) {
    net.setDeadline($client.conn, 0);
    def msg as Message init Message{ topic: "", payload: emptyBytes() };
    def waiting as bool init true;
    while ($waiting) {
        def h as bytes init readN($client.conn, 1);
        def pkt as Packet init readPacketBody($client.conn, $h[0]);
        if ($pkt.typ == 3) {
            $msg = parsePublish($pkt);
            $waiting = false;
        }
    }
    return $msg;
}

/**
 * Poll for a message, waiting up to timeoutMs milliseconds. Returns a list of
 * zero or one Message: empty when nothing arrived in the window (the caller can
 * then `ping` and loop), one Message when a PUBLISH was received. Non-PUBLISH
 * control packets are consumed and reported as an empty poll.
 * @param client {Client} the open client
 * @param timeoutMs {int} how long to wait for the next packet, in milliseconds
 * @return {list of Message} zero or one received message
 */
export func poll(client as Client, timeoutMs as int) {
    def out as list of Message init [];
    net.setDeadline($client.conn, $timeoutMs);
    def hb as int init 0;
    def gotByte as bool init true;
    try {
        def h as bytes init readN($client.conn, 1);
        $hb = $h[0];
    } catch (err) {
        net.setDeadline($client.conn, 0);
        if (strings.contains($err.message, "timed out")) {
            $gotByte = false;
        } else {
            throw $err;
        }
    }
    if (not $gotByte) {
        return $out;
    }
    net.setDeadline($client.conn, 0);
    def pkt as Packet init readPacketBody($client.conn, $hb);
    if ($pkt.typ == 3) {
        $out[] = parsePublish($pkt);
    }
    return $out;
}

/**
 * Send a PINGREQ keepalive (fire and forget). The matching PINGRESP is consumed
 * by the next `poll` / `receive`.
 * @param client {Client} the open client
 */
export func ping(client as Client) {
    def f as bytes;
    $f[] = 0xc0;
    $f[] = 0x00;
    net.writeBytes($client.conn, $f);
    return null;
}

/**
 * Send DISCONNECT and close the connection.
 * @param client {Client} the open client
 */
export func disconnect(client as Client) {
    # The socket is shut even when the DISCONNECT write throws (a dead broker
    # must not leak the fd).
    defer net.close($client.conn);
    def f as bytes;
    $f[] = 0xe0;
    $f[] = 0x00;
    net.writeBytes($client.conn, $f);
    return null;
}

# emptyBytes returns a fresh empty bytes value (the zero payload for a
# not-yet-populated Message).
func emptyBytes() {
    def b as bytes;
    return $b;
}
