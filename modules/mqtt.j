# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * An MQTT 3.1.1 publish/subscribe client over the `net` system library - the
 * same "protocol clients are modules, `net` is the transport" line the other
 * network clients follow. MQTT packets are a 1-byte fixed header, a variable
 * remaining-length integer, then a length-prefixed payload, all built and
 * parsed here with Jennifer's bitwise operators and `bytes`. QoS 0 and QoS 1
 * publish / subscribe (a synchronous PUBACK handshake), retained messages, a
 * Last-Will set in the CONNECT, and `reconnect` for session resumption, on top
 * of a single-threaded `poll` with timeout (via `net.setDeadline`) so one flow
 * can wait for a packet and send keepalives without a spawned reader, plus
 * blocking `receive`, `ping`, and `disconnect`. QoS 2 and MQTT 5 properties are
 * out of scope. Needs the default `jennifer` binary (uses `net`).
 * @module mqtt
 * @example
 * import "transport.j" as transport;
 * def c as mqtt.Client init mqtt.connect(mqtt.Options{host: "127.0.0.1", port: 1883, clientId: "demo", keepalive: 30, security: transport.Security.None, username: "", password: ""});
 * $c = mqtt.subscribe($c, "sensors/temp");
 * mqtt.publish($c, "sensors/temp", "21.5");
 * def m as mqtt.Message init mqtt.receive($c);
 * mqtt.disconnect($c);
 */
use net;
use binary;
use convert;
use strings;
use lists;
import "./transport.j" as transport;

# --- types ------------------------------------------------------------------

/**
 * Connection settings for mqtt.connect.
 * @field host {string} the broker host
 * @field port {int} the broker port (1883 plaintext, 8883 TLS by convention)
 * @field clientId {string} the client identifier the broker sees
 * @field keepalive {int} the keepalive interval in seconds (0 disables)
 * @field security {transport.Security} `transport.Security.None` (plaintext) or `.Tls` (mqtts); `.Starttls` is rejected (MQTT has no in-band upgrade)
 * @field username {string} the CONNECT username ("" to omit)
 * @field password {string} the CONNECT password ("" to omit)
 */
export def struct Options {
    host as string,
    port as int,
    clientId as string,
    keepalive as int,
    security as transport.Security,
    username as string,
    password as string
};

/**
 * A Last-Will-and-Testament message the broker publishes on this client's
 * behalf if the connection drops without a clean DISCONNECT. An empty `topic`
 * means "no will" (the CONNECT sets no will flags).
 * @field topic {string} the will topic ("" disables the will)
 * @field payload {bytes} the will message bytes
 * @field qos {int} the will QoS (0 or 1)
 * @field retain {bool} whether the broker retains the will message
 */
export def struct Will {
    topic as string,
    payload as bytes,
    qos as int,
    retain as bool
};

/**
 * One tracked subscription, remembered on the Client so `reconnect` can restore
 * the session's routes.
 * @field topic {string} the subscribed topic filter
 * @field qos {int} the QoS the broker granted (0x80 means it was rejected)
 */
export def struct Subscription {
    topic as string,
    qos as int
};

/**
 * An open MQTT connection. Beyond the socket it carries the settings needed to
 * re-establish the session (`opts`, `will`, `cleanSession`) and the list of
 * subscriptions `subscribe` / `subscribeQos1` have tracked, so `reconnect` can
 * re-dial and re-subscribe. Value-semantic: copies share the socket handle but
 * each carries its own settings and subscription list.
 * @field conn {net.Conn} the underlying socket
 * @field opts {Options} the settings this client connected with
 * @field will {Will} the Last-Will registered in the CONNECT ("" topic = none)
 * @field cleanSession {bool} the clean-session flag (false resumes the session)
 * @field subs {list of Subscription} the tracked subscriptions for reconnect
 */
export def struct Client {
    conn as net.Conn,
    opts as Options,
    will as Will,
    cleanSession as bool,
    subs as list of Subscription
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
    return binary.slice($src, $start, $end);
}

# putBytesField appends a 2-byte big-endian length prefix and then the raw
# bytes (MQTT's binary-data and UTF-8-string wire shape) to b and returns b.
func putBytesField(b as bytes, raw as bytes) {
    def n as int init len($raw);
    $b[] = ($n >> 8) & 0xff;
    $b[] = $n & 0xff;
    return appendBytes($b, $raw);
}

# putString appends an MQTT UTF-8 string (2-byte big-endian length prefix, then
# the bytes) to b and returns b.
func putString(b as bytes, s as string) {
    return putBytesField($b, convert.bytesFromString($s, "utf-8"));
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
    return DecodedLen{value: $value, size: $i - $start};
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

# noWill returns the "no Last-Will" sentinel (an empty topic).
func noWill() {
    return Will{topic: "", payload: emptyBytes(), qos: 0, retain: false};
}

# buildConnectFull builds the CONNECT packet (type 1) for the given options,
# Last-Will, and clean-session flag. A `will` with an empty topic sets no will
# flags; `cleanSession` false asks the broker to resume a persistent session.
func buildConnectFull(opts as Options, will as Will, cleanSession as bool) {
    def vh as bytes;
    $vh = putString($vh, "MQTT");
    $vh[] = 4;
    def flags as int init 0;
    if ($cleanSession) {
        $flags = $flags | 0x02;
    }
    def hasWill as bool init len($will.topic) > 0;
    if ($hasWill) {
        $flags = $flags | 0x04;
        $flags = $flags | (($will.qos & 0x03) << 3);
        if ($will.retain) {
            $flags = $flags | 0x20;
        }
    }
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
    if ($hasWill) {
        $pl = putString($pl, $will.topic);
        $pl = putBytesField($pl, $will.payload);
    }
    if (len($opts.username) > 0) {
        $pl = putString($pl, $opts.username);
    }
    if (len($opts.password) > 0) {
        $pl = putString($pl, $opts.password);
    }
    return frame(0x10, $vh, $pl);
}

# buildConnect builds a basic CONNECT (clean session, no will) for the options.
func buildConnect(opts as Options) {
    return buildConnectFull($opts, noWill(), true);
}

# buildPublish builds a PUBLISH packet (type 3). The flags nibble carries DUP
# (bit 3), the QoS (bits 2-1), and retain (bit 0); a QoS>0 packet inserts the
# 2-byte packet identifier after the topic.
func buildPublish(
    topic as string,
    payload as bytes,
    qos as int,
    packetId as int,
    dup as bool,
    retain as bool) {
    def header as int init 0x30;
    if ($dup) {
        $header = $header | 0x08;
    }
    $header = $header | (($qos & 0x03) << 1);
    if ($retain) {
        $header = $header | 0x01;
    }
    def vh as bytes;
    $vh = putString($vh, $topic);
    if ($qos > 0) {
        $vh[] = ($packetId >> 8) & 0xff;
        $vh[] = $packetId & 0xff;
    }
    return frame($header, $vh, $payload);
}

# buildPuback builds a PUBACK packet (type 4): a 2-byte packet identifier and no
# payload. Sent to acknowledge a received QoS-1 PUBLISH.
func buildPuback(packetId as int) {
    def vh as bytes;
    $vh[] = ($packetId >> 8) & 0xff;
    $vh[] = $packetId & 0xff;
    def pl as bytes;
    return frame(0x40, $vh, $pl);
}

# parsePuback returns the packet identifier a PUBACK acknowledges.
func parsePuback(pkt as Packet) {
    return ($pkt.body[0] << 8) | $pkt.body[1];
}

# parseSubackQos returns the granted QoS the broker reported in a single-topic
# SUBACK (0, 1, or 2), or 0x80 when it rejected the subscription. The granted
# byte follows the 2-byte packet identifier.
func parseSubackQos(pkt as Packet) {
    return $pkt.body[2];
}

# publishPacketId returns the packet identifier of a QoS>0 PUBLISH (the 2-byte
# value after the topic), or -1 for a QoS-0 PUBLISH (which carries none).
func publishPacketId(pkt as Packet) {
    def qos as int init ($pkt.flags >> 1) & 0x03;
    if ($qos == 0) {
        return -1;
    }
    def body as bytes init $pkt.body;
    def tlen as int init (($body[0] << 8) | $body[1]);
    def idx as int init 2 + $tlen;
    return ($body[$idx] << 8) | $body[$idx + 1];
}

# parsePublish turns a PUBLISH packet's body into a Message. A QoS>0 packet
# carries a 2-byte packet identifier after the topic, which is skipped here (the
# PUBACK is sent by the receive / poll loop, which reads the id directly).
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
    return Message{topic: $topic, payload: $payload};
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
        throw Error{
            kind: "mqtt",
            message: "mqtt: packet declares " + convert.toString($n) + " bytes, over the " +
                convert.toString(MAX_PACKET_BYTES) + "-byte limit",
            file: "",
            line: 0,
            col: 0
        };
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
            throw Error{
                kind: "mqtt",
                message: "mqtt: connection closed mid-packet",
                file: "",
                line: 0,
                col: 0
            };
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
    return Packet{typ: $typ, flags: $flags, body: $body};
}

# ackIfQos1 sends the PUBACK for an incoming PUBLISH when it is QoS 1 (a QoS-0
# PUBLISH needs none); a no-op otherwise.
func ackIfQos1(client as Client, pkt as Packet) {
    def pid as int init publishPacketId($pkt);
    if ($pid >= 0) {
        net.writeBytes($client.conn, buildPuback($pid));
    }
    return null;
}

# The handshake read timeout (ms), so a broker that accepts but never sends the
# CONNACK / SUBACK fails instead of blocking forever. Cleared after the ack so
# `poll` / `receive` keep managing their own deadlines.
def const HANDSHAKE_TIMEOUT_MS as int init 30000;

# QoS-1 PUBACK wait. publishQos1 is synchronous (only one message is ever in
# flight, because it blocks for the PUBACK), so the packet identifier is a fixed
# non-zero value and an unacknowledged send is retried as a DUP up to
# PUBACK_RETRIES times, PUBACK_TIMEOUT_MS apart.
def const QOS1_PACKET_ID as int init 1;
def const PUBACK_TIMEOUT_MS as int init 5000;
def const PUBACK_RETRIES as int init 5;

# --- connection lifecycle (exported) ----------------------------------------

/**
 * Open a connection, send CONNECT, and check the CONNACK return code. Uses a
 * clean session and no Last-Will; for a will or session resumption use
 * `connectWith`.
 * @param opts {Options} the connection settings
 * @return {Client} the open client
 * @throws {Error} kind "mqtt" when the broker refuses the connection
 */
export func connect(opts as Options) {
    return connectWith($opts, noWill(), true);
}

/**
 * Open a connection with an explicit Last-Will and clean-session flag. A `will`
 * whose topic is "" registers no will; `cleanSession` false asks the broker to
 * resume a persistent session keyed by `opts.clientId` (retained subscriptions
 * and queued QoS-1 messages).
 * @param opts {Options} the connection settings
 * @param will {Will} the Last-Will to register ("" topic disables it)
 * @param cleanSession {bool} false to resume a persistent session
 * @return {Client} the open client
 * @throws {Error} kind "mqtt" when the broker refuses the connection
 */
export func connectWith(opts as Options, will as Will, cleanSession as bool) {
    def addr as string init $opts.host + ":" + convert.toString($opts.port);
    def conn as net.Conn;
    match ($opts.security) {
        when Tls { $conn = net.connectTLS($addr, HANDSHAKE_TIMEOUT_MS); }
        when None { $conn = net.connect($addr, HANDSHAKE_TIMEOUT_MS); }
        when Starttls {
            throw Error{kind: "mqtt", message: "mqtt: STARTTLS is not supported; use transport.Security.Tls (mqtts) or .None", file: "", line: 0, col: 0};
        }
    }
    # A refused / malformed CONNACK must not leak the socket; on success the
    # caller owns the open client.
    errdefer net.close($conn);
    net.writeBytes($conn, buildConnectFull($opts, $will, $cleanSession));
    net.setDeadline($conn, HANDSHAKE_TIMEOUT_MS);
    def h as bytes init readN($conn, 1);
    def pkt as Packet init readPacketBody($conn, $h[0]);
    net.setDeadline($conn, 0);
    if (not ($pkt.typ == 2)) {
        throw Error{
            kind: "mqtt",
            message: "mqtt: expected CONNACK, got packet type " + convert.toString($pkt.typ),
            file: "",
            line: 0,
            col: 0
        };
    }
    def code as int init $pkt.body[1];
    if (not ($code == 0)) {
        throw Error{
            kind: "mqtt",
            message: "mqtt: connection refused, CONNACK code " + convert.toString($code),
            file: "",
            line: 0,
            col: 0
        };
    }
    def subs as list of Subscription init [];
    return Client{conn: $conn, opts: $opts, will: $will, cleanSession: $cleanSession, subs: $subs};
}

/**
 * Re-dial the broker and re-CONNECT with this client's stored settings, then
 * re-subscribe every tracked subscription. Returns a fresh Client the caller
 * reassigns (value semantics - the old handle is best-effort closed).
 *
 * Session resumption: when the client connected with `cleanSession` false, the
 * broker keeps the session (subscriptions and queued QoS-1 messages) across the
 * drop and reports it in the CONNACK; the re-subscribe here is idempotent and
 * also covers the clean-session case, where the broker discarded the session.
 * @param client {Client} the (typically disconnected) client to re-establish
 * @return {Client} a freshly connected client with the tracked routes restored
 * @throws {Error} kind "mqtt" when the re-connect or a re-subscribe fails
 */
export func reconnect(client as Client) {
    def opts as Options init $client.opts;
    def will as Will init $client.will;
    def clean as bool init $client.cleanSession;
    def subs as list of Subscription init $client.subs;
    # Best-effort close of the old (likely dead) socket so its fd is not leaked.
    closeQuietly($client.conn);
    def out as Client init connectWith($opts, $will, $clean);
    for (def s in $subs) {
        if ($s.qos > 0) {
            $out = subscribeQos1($out, $s.topic);
        } else {
            $out = subscribe($out, $s.topic);
        }
    }
    return $out;
}

# closeQuietly closes a socket, swallowing an error (the peer may already be
# gone). Used by reconnect on the stale handle.
func closeQuietly(conn as net.Conn) {
    try {
        net.close($conn);
    } catch (e) {
        return null;
    }
    return null;
}

/**
 * Publish a raw byte payload to a topic at QoS 0 (fire and forget).
 * @param client {Client} the open client
 * @param topic {string} the topic to publish to
 * @param payload {bytes} the message bytes
 */
export func publishBytes(client as Client, topic as string, payload as bytes) {
    net.writeBytes($client.conn, buildPublish($topic, $payload, 0, 0, false, false));
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
 * Publish a raw byte payload at QoS 0 with an explicit retain flag. A retained
 * message is stored by the broker and delivered to every future subscriber of
 * the topic; publishing an empty payload with retain true clears it.
 * @param client {Client} the open client
 * @param topic {string} the topic to publish to
 * @param payload {bytes} the message bytes
 * @param retain {bool} whether the broker retains the message
 */
export func publishBytesRetain(client as Client, topic as string, payload as bytes, retain as bool) {
    net.writeBytes($client.conn, buildPublish($topic, $payload, 0, 0, false, $retain));
    return null;
}

/**
 * Publish a text message at QoS 0 with an explicit retain flag (UTF-8 encoded).
 * @param client {Client} the open client
 * @param topic {string} the topic to publish to
 * @param message {string} the message text
 * @param retain {bool} whether the broker retains the message
 */
export func publishRetain(client as Client, topic as string, message as string, retain as bool) {
    return publishBytesRetain($client, $topic, convert.bytesFromString($message, "utf-8"), $retain);
}

/**
 * Publish a raw byte payload at QoS 1 (at-least-once) and block until the
 * matching PUBACK. The PUBLISH carries a packet identifier; an unacknowledged
 * send is retried as a DUP (the duplicate-delivery flag set) up to a fixed
 * number of attempts, so at-least-once delivery is honored across a slow or
 * flapping link. Because the call blocks for the PUBACK, only one message is
 * ever in flight, so a fixed non-zero packet identifier is safe. A duplicate
 * the broker already delivered is de-duplicated by the packet id on its side.
 * @param client {Client} the open client
 * @param topic {string} the topic to publish to
 * @param payload {bytes} the message bytes
 * @param retain {bool} whether the broker retains the message
 * @throws {Error} kind "mqtt" when no PUBACK arrives within the retry budget
 */
export func publishQos1(client as Client, topic as string, payload as bytes, retain as bool) {
    def pid as int init QOS1_PACKET_ID;
    net.writeBytes($client.conn, buildPublish($topic, $payload, 1, $pid, false, $retain));
    def attempt as int init 1;
    while ($attempt <= PUBACK_RETRIES) {
        if (awaitPuback($client, $pid, PUBACK_TIMEOUT_MS)) {
            return null;
        }
        $attempt = $attempt + 1;
        if ($attempt <= PUBACK_RETRIES) {
            # Re-send as a duplicate (DUP set) with the same packet id.
            net.writeBytes($client.conn, buildPublish($topic, $payload, 1, $pid, true, $retain));
        }
    }
    throw Error{
        kind: "mqtt",
        message: "mqtt: no PUBACK for packet id " + convert.toString($pid) + " after " +
            convert.toString(PUBACK_RETRIES) + " attempts",
        file: "",
        line: 0,
        col: 0
    };
}

# awaitPuback waits up to timeoutMs for the PUBACK matching pid, returning true
# on a match and false on timeout. An incoming QoS-1 PUBLISH seen while waiting
# is acknowledged so the broker stays satisfied; other control packets (a
# PINGRESP) are consumed. Non-timeout I/O errors propagate.
func awaitPuback(client as Client, pid as int, timeoutMs as int) {
    net.setDeadline($client.conn, $timeoutMs);
    def result as bool init false;
    def waiting as bool init true;
    try {
        while ($waiting) {
            def h as bytes init readN($client.conn, 1);
            def pkt as Packet init readPacketBody($client.conn, $h[0]);
            if ($pkt.typ == 4) {
                if (parsePuback($pkt) == $pid) {
                    $result = true;
                }
                $waiting = false;
            } elseif ($pkt.typ == 3) {
                ackIfQos1($client, $pkt);
            }
        }
    } catch (e) {
        net.setDeadline($client.conn, 0);
        if (strings.contains($e.message, "timed out")) {
            return false;
        }
        throw $e;
    }
    net.setDeadline($client.conn, 0);
    return $result;
}

/**
 * Subscribe to a topic filter at QoS 0, wait for the SUBACK, and track the
 * subscription on the returned Client (so `reconnect` can restore it). Reassign
 * the result: `$c = mqtt.subscribe($c, "topic");`.
 * @param client {Client} the open client
 * @param topic {string} the topic filter (may contain `+` / `#` wildcards)
 * @return {Client} the client with the subscription tracked
 * @throws {Error} kind "mqtt" when the broker rejects the subscription
 */
export func subscribe(client as Client, topic as string) {
    return subscribeAt($client, $topic, 0);
}

/**
 * Subscribe to a topic filter requesting QoS 1, wait for the SUBACK, and track
 * the subscription (with the QoS the broker granted) on the returned Client.
 * With QoS 1 the broker delivers each matching message as a QoS-1 PUBLISH,
 * which `receive` / `poll` acknowledge with a PUBACK. Reassign the result:
 * `$c = mqtt.subscribeQos1($c, "topic");`.
 * @param client {Client} the open client
 * @param topic {string} the topic filter (may contain `+` / `#` wildcards)
 * @return {Client} the client with the subscription tracked
 * @throws {Error} kind "mqtt" when the broker rejects the subscription
 */
export func subscribeQos1(client as Client, topic as string) {
    return subscribeAt($client, $topic, 1);
}

# subscribeAt sends a SUBSCRIBE requesting the given QoS, waits for the SUBACK,
# and returns the client with the subscription (topic + granted QoS) tracked.
func subscribeAt(client as Client, topic as string, qos as int) {
    def vh as bytes;
    $vh[] = 0x00;
    $vh[] = 0x01;
    def pl as bytes;
    $pl = putString($pl, $topic);
    $pl[] = $qos & 0x03;
    net.writeBytes($client.conn, frame(0x82, $vh, $pl));
    net.setDeadline($client.conn, HANDSHAKE_TIMEOUT_MS);
    def h as bytes init readN($client.conn, 1);
    def pkt as Packet init readPacketBody($client.conn, $h[0]);
    net.setDeadline($client.conn, 0);
    if (not ($pkt.typ == 9)) {
        throw Error{
            kind: "mqtt",
            message: "mqtt: expected SUBACK, got packet type " + convert.toString($pkt.typ),
            file: "",
            line: 0,
            col: 0
        };
    }
    def granted as int init parseSubackQos($pkt);
    if ($granted == 0x80) {
        throw Error{
            kind: "mqtt",
            message: "mqtt: subscribe rejected for topic " + $topic,
            file: "",
            line: 0,
            col: 0
        };
    }
    def updated as Client init $client;
    $updated.subs = lists.push($client.subs, Subscription{topic: $topic, qos: $granted});
    return $updated;
}

/**
 * Block until the next application message arrives, returning it. A QoS-1
 * PUBLISH is acknowledged with a PUBACK before it is returned. Non-PUBLISH
 * control packets (e.g. a PINGRESP) are consumed and skipped.
 * @param client {Client} the open client
 * @return {Message} the next received message
 */
export func receive(client as Client) {
    net.setDeadline($client.conn, 0);
    def msg as Message init Message{topic: "", payload: emptyBytes()};
    def waiting as bool init true;
    while ($waiting) {
        def h as bytes init readN($client.conn, 1);
        def pkt as Packet init readPacketBody($client.conn, $h[0]);
        if ($pkt.typ == 3) {
            ackIfQos1($client, $pkt);
            $msg = parsePublish($pkt);
            $waiting = false;
        }
    }
    return $msg;
}

/**
 * Poll for a message, waiting up to timeoutMs milliseconds. Returns a list of
 * zero or one Message: empty when nothing arrived in the window (the caller can
 * then `ping` and loop), one Message when a PUBLISH was received. A QoS-1
 * PUBLISH is acknowledged with a PUBACK before it is returned. Non-PUBLISH
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
        ackIfQos1($client, $pkt);
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
