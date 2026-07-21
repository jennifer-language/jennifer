# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

/**
 * An AMQP 0-9-1 client over `net` for RabbitMQ and compatible brokers. `connect`
 * runs the connection + channel handshake (protocol header, `Connection.Start` /
 * `Start-Ok` with SASL PLAIN auth, `Tune` / `Tune-Ok`, `Open` / `Open-Ok`,
 * `Channel.Open`); `declareQueue` declares a classic queue and
 * `declareQuorumQueue` a replicated quorum queue; `publish` sends a message
 * (method + content-header + body frames); `get` pulls the next message with
 * `Basic.Get` (a synchronous pull, no async delivery loop); `ack` acknowledges
 * it; `close` shuts the connection down cleanly.
 *
 * The binary frame and method encoding is built by hand from `bytes` and the
 * bitwise operators - the largest protocol module here. Needs the default
 * `jennifer` binary (`net`); a protocol error or dropped connection throws
 * `Error{kind: "amqp"}`. Uses one channel (1); heartbeats are disabled.
 * @module amqp
 * @example
 * import "amqp.j" as amqp;
 * def c as amqp.Conn init amqp.connect(amqp.options("localhost", "guest", "guest"));
 * amqp.declareQueue($c, "jobs", true);
 * amqp.publishText($c, "", "jobs", "hello");
 * def m as amqp.Message init amqp.get($c, "jobs", false);
 * if (not $m.empty) { amqp.ack($c, $m.deliveryTag); }
 * amqp.close($c);
 */
use net;
use convert;

# Frame types and the frame-end sentinel.
def const FRAME_METHOD as int init 1;
def const FRAME_HEADER as int init 2;
def const FRAME_BODY as int init 3;
def const FRAME_END as int init 206;   # 0xCE

# The single channel this client uses.
def const CHANNEL as int init 1;

# The connection-establishment timeout (ms): a slow / unreachable broker fails
# instead of blocking the dial forever.
def const CONNECT_TIMEOUT_MS as int init 30000;

# Class ids.
def const CLS_CONNECTION as int init 10;
def const CLS_CHANNEL as int init 20;
def const CLS_QUEUE as int init 50;
def const CLS_BASIC as int init 60;

# Method ids (per class).
def const CONN_START as int init 10;
def const CONN_STARTOK as int init 11;
def const CONN_TUNE as int init 30;
def const CONN_TUNEOK as int init 31;
def const CONN_OPEN as int init 40;
def const CONN_OPENOK as int init 41;
def const CONN_CLOSE as int init 50;
def const CONN_CLOSEOK as int init 51;
def const CH_OPEN as int init 10;
def const CH_OPENOK as int init 11;
def const Q_DECLARE as int init 10;
def const Q_DECLAREOK as int init 11;
def const B_PUBLISH as int init 40;
def const B_GET as int init 70;
def const B_GETOK as int init 71;
def const B_GETEMPTY as int init 72;
def const B_ACK as int init 80;

# The durable flag bit for Queue.Declare.
def const Q_DURABLE as int init 2;

/**
 * Connection options.
 * @field host {string} the broker host
 * @field port {int} the broker port (5672 by default)
 * @field user {string} the username
 * @field password {string} the password
 * @field vhost {string} the virtual host ("/" by default)
 * @field security {string} "none" (plaintext AMQP, the default) or "tls" (AMQPS - TLS on connect, verifying the broker certificate)
 */
export def struct Options {
    host as string,
    port as int,
    user as string,
    password as string,
    vhost as string,
    security as string
};

/**
 * An open connection (single channel).
 * @field socket {net.Conn} the underlying connection
 * @field channel {int} the channel number (always 1)
 * @field frameMax {int} the negotiated maximum frame size (bytes)
 */
export def struct Conn {
    socket as net.Conn,
    channel as int,
    frameMax as int
};

/**
 * Queue metadata returned by declareQueue.
 * @field name {string} the queue name (the server's, for a server-named queue)
 * @field messageCount {int} the number of ready messages
 * @field consumerCount {int} the number of consumers
 */
export def struct QueueInfo {
    name as string,
    messageCount as int,
    consumerCount as int
};

/**
 * A message pulled with get.
 * @field empty {bool} true when the queue had no message (the other fields are zero)
 * @field deliveryTag {int} the delivery tag (pass to ack)
 * @field exchange {string} the exchange the message came from
 * @field routingKey {string} the routing key
 * @field body {bytes} the message body
 */
export def struct Message {
    empty as bool,
    deliveryTag as int,
    exchange as string,
    routingKey as string,
    body as bytes
};

# A decoded method frame (private).
def struct Method {
    classId as int,
    methodId as int,
    args as bytes
};

# A decoded frame (private).
def struct Frame {
    ftype as int,
    channel as int,
    payload as bytes
};

func fail(msg as string) {
    throw Error{ kind: "amqp", message: "amqp: " + $msg, file: "", line: 0, col: 0 };
}

func emptyBytes() {
    def b as bytes;
    return $b;
}

# --- options (exported) -----------------------------------------------------

/**
 * Default options for a broker: port 5672, vhost "/".
 * @param host {string} the broker host
 * @param user {string} the username
 * @param password {string} the password
 * @return {Options} the options
 */
export func options(host as string, user as string, password as string) {
    return Options{ host: $host, port: 5672, user: $user, password: $password, vhost: "/", security: "none" };
}

/**
 * Copy options with a different port.
 * @param o {Options} the options
 * @param port {int} the port
 * @return {Options} a fresh options
 */
export func withPort(o as Options, port as int) {
    def out as Options init $o;
    $out.port = $port;
    return $out;
}

/**
 * Copy options with a different virtual host.
 * @param o {Options} the options
 * @param vhost {string} the virtual host
 * @return {Options} a fresh options
 */
export func withVhost(o as Options, vhost as string) {
    def out as Options init $o;
    $out.vhost = $vhost;
    return $out;
}

# --- byte helpers (private) -------------------------------------------------

func appendBytes(dst as bytes, src as bytes) {
    def i as int init 0;
    while ($i < len($src)) {
        $dst[] = $src[$i];
        $i = $i + 1;
    }
    return $dst;
}

func sliceBytes(src as bytes, start as int, end as int) {
    def out as bytes;
    def i as int init $start;
    while ($i < $end) {
        $out[] = $src[$i];
        $i = $i + 1;
    }
    return $out;
}

func putOctet(b as bytes, v as int) {
    $b[] = $v & 0xff;
    return $b;
}

func putShort(b as bytes, v as int) {
    $b[] = ($v >> 8) & 0xff;
    $b[] = $v & 0xff;
    return $b;
}

func putLong(b as bytes, v as int) {
    $b[] = ($v >> 24) & 0xff;
    $b[] = ($v >> 16) & 0xff;
    $b[] = ($v >> 8) & 0xff;
    $b[] = $v & 0xff;
    return $b;
}

func putLongLong(b as bytes, v as int) {
    def s as int init 56;
    while ($s >= 0) {
        $b[] = ($v >> $s) & 0xff;
        $s = $s - 8;
    }
    return $b;
}

func putShortStr(b as bytes, s as string) {
    def raw as bytes init convert.bytesFromString($s, "utf-8");
    def out as bytes init putOctet($b, len($raw));
    return appendBytes($out, $raw);
}

func putLongStr(b as bytes, s as string) {
    def raw as bytes init convert.bytesFromString($s, "utf-8");
    def out as bytes init putLong($b, len($raw));
    return appendBytes($out, $raw);
}

# putEmptyTable writes an empty AMQP field-table (a 4-byte zero length).
func putEmptyTable(b as bytes) {
    return putLong($b, 0);
}

# putStringTable writes an AMQP field-table whose every value is a long-string
# ('S' field-value type), preceded by the 4-byte table length. Used for queue
# arguments such as {"x-queue-type": "quorum"}.
func putStringTable(b as bytes, entries as map of string to string) {
    def body as bytes;
    for (def key in $entries) {
        $body = putShortStr($body, $key);      # field name (shortstr)
        $body = putOctet($body, 83);           # 'S' = long-string value type
        $body = putLongStr($body, $entries[$key]);
    }
    def out as bytes init putLong($b, len($body));
    return appendBytes($out, $body);
}

func readShort(buf as bytes, off as int) {
    return ($buf[$off] << 8) | $buf[$off + 1];
}

func readLong(buf as bytes, off as int) {
    return ($buf[$off] << 24) | ($buf[$off + 1] << 16) | ($buf[$off + 2] << 8) | $buf[$off + 3];
}

func readLongLong(buf as bytes, off as int) {
    def v as int init 0;
    def i as int init 0;
    while ($i < 8) {
        $v = ($v << 8) | $buf[$off + $i];
        $i = $i + 1;
    }
    return $v;
}

# readShortStr decodes the short-string (1-byte length prefix) at off.
func readShortStr(buf as bytes, off as int) {
    def n as int init $buf[$off];
    return convert.stringFromBytes(sliceBytes($buf, $off + 1, $off + 1 + $n), "utf-8");
}

# byteLen is the UTF-8 byte length of s (how far a short-string field advances).
func byteLen(s as string) {
    return len(convert.bytesFromString($s, "utf-8"));
}

# --- frame I/O (private) ----------------------------------------------------

func readN(socket as net.Conn, n as int) {
    def out as bytes;
    while (len($out) < $n) {
        def chunk as bytes init net.readBytes($socket, $n - len($out));
        if (len($chunk) == 0) {
            fail("connection closed mid-frame");
        }
        # Append into `out` in place: `out = appendBytes(out, chunk)` copies the
        # whole growing buffer per read (O(N^2) over the accumulation).
        def k as int init 0;
        while ($k < len($chunk)) {
            $out[] = $chunk[$k];
            $k = $k + 1;
        }
    }
    return $out;
}

# writeFrame writes one framed unit (type + channel + size + payload + 0xCE).
func writeFrame(socket as net.Conn, ftype as int, channel as int, payload as bytes) {
    def f as bytes;
    $f = putOctet($f, $ftype);
    $f = putShort($f, $channel);
    $f = putLong($f, len($payload));
    $f = appendBytes($f, $payload);
    $f = putOctet($f, FRAME_END);
    net.writeBytes($socket, $f);
}

# writeMethod writes a method frame (class + method + args).
func writeMethod(socket as net.Conn, channel as int, classId as int, methodId as int, args as bytes) {
    def p as bytes;
    $p = putShort($p, $classId);
    $p = putShort($p, $methodId);
    $p = appendBytes($p, $args);
    writeFrame($socket, FRAME_METHOD, $channel, $p);
}

# readFrame reads one framed unit off the socket.
func readFrame(socket as net.Conn) {
    def h as bytes init readN($socket, 7);
    def ftype as int init $h[0];
    def channel as int init readShort($h, 1);
    def size as int init readLong($h, 3);
    def payload as bytes;
    if ($size > 0) {
        $payload = readN($socket, $size);
    }
    def end as bytes init readN($socket, 1);
    if (not ($end[0] == FRAME_END)) {
        fail("bad frame terminator");
    }
    return Frame{ ftype: $ftype, channel: $channel, payload: $payload };
}

# readMethod reads a method frame and splits out its class / method / args.
func readMethod(socket as net.Conn) {
    def f as Frame init readFrame($socket);
    if (not ($f.ftype == FRAME_METHOD)) {
        fail("expected a method frame");
    }
    def classId as int init readShort($f.payload, 0);
    def methodId as int init readShort($f.payload, 2);
    return Method{ classId: $classId, methodId: $methodId, args: sliceBytes($f.payload, 4, len($f.payload)) };
}

# expectMethod reads a method frame and asserts its class / method.
func expectMethod(socket as net.Conn, classId as int, methodId as int, what as string) {
    def m as Method init readMethod($socket);
    if (not ($m.classId == $classId and $m.methodId == $methodId)) {
        fail("expected " + $what + ", got class " + convert.toString($m.classId) + " method " + convert.toString($m.methodId));
    }
    return $m;
}

# --- handshake (private) ----------------------------------------------------

# saslPlain builds the SASL PLAIN response: NUL user NUL password.
func saslPlain(user as string, password as string) {
    return "\0" + $user + "\0" + $password;
}

func handshake(socket as net.Conn, opts as Options) {
    # protocol header: "AMQP" 0 0 9 1
    def hdr as bytes;
    $hdr = appendBytes($hdr, convert.bytesFromString("AMQP", "utf-8"));
    $hdr = putOctet($hdr, 0);
    $hdr = putOctet($hdr, 0);
    $hdr = putOctet($hdr, 9);
    $hdr = putOctet($hdr, 1);
    net.writeBytes($socket, $hdr);

    # Connection.Start (we ignore the server properties / mechanisms).
    expectMethod($socket, CLS_CONNECTION, CONN_START, "Connection.Start");

    # Connection.Start-Ok: client-properties(table) mechanism response locale.
    def sok as bytes;
    $sok = putEmptyTable($sok);
    $sok = putShortStr($sok, "PLAIN");
    $sok = putLongStr($sok, saslPlain($opts.user, $opts.password));
    $sok = putShortStr($sok, "en_US");
    writeMethod($socket, 0, CLS_CONNECTION, CONN_STARTOK, $sok);

    # Connection.Tune: echo channel-max / frame-max, disable heartbeat.
    def tune as Method init expectMethod($socket, CLS_CONNECTION, CONN_TUNE, "Connection.Tune");
    def channelMax as int init readShort($tune.args, 0);
    def frameMax as int init readLong($tune.args, 2);
    def tok as bytes;
    $tok = putShort($tok, $channelMax);
    $tok = putLong($tok, $frameMax);
    $tok = putShort($tok, 0);
    writeMethod($socket, 0, CLS_CONNECTION, CONN_TUNEOK, $tok);

    # Connection.Open: virtual-host, reserved shortstr, reserved bit.
    def open as bytes;
    $open = putShortStr($open, $opts.vhost);
    $open = putShortStr($open, "");
    $open = putOctet($open, 0);
    writeMethod($socket, 0, CLS_CONNECTION, CONN_OPEN, $open);
    expectMethod($socket, CLS_CONNECTION, CONN_OPENOK, "Connection.Open-Ok");

    # Channel.Open on channel 1 (reserved shortstr).
    def cho as bytes;
    $cho = putShortStr($cho, "");
    writeMethod($socket, CHANNEL, CLS_CHANNEL, CH_OPEN, $cho);
    expectMethod($socket, CLS_CHANNEL, CH_OPENOK, "Channel.Open-Ok");
    # Return the negotiated frame-max so publish can split large bodies (0 = no
    # limit).
    return $frameMax;
}

# --- API (exported) ---------------------------------------------------------

# dial opens the transport per opts.security: plaintext AMQP, or AMQPS (TLS on
# connect, broker certificate verified). A connection timeout bounds the dial.
func dial(opts as Options) {
    def addr as string init $opts.host + ":" + convert.toString($opts.port);
    if ($opts.security == "tls") {
        return net.connectTLS($addr, CONNECT_TIMEOUT_MS);
    }
    return net.connect($addr, CONNECT_TIMEOUT_MS);
}

/**
 * Connect to a broker and open a channel.
 * @param opts {Options} the connection options
 * @return {Conn} the open connection
 * @throws {Error} kind "amqp" on a handshake failure
 */
export func connect(opts as Options) {
    def socket as net.Conn init dial($opts);
    # A failed AMQP handshake must not leak the socket; on success the caller
    # owns the open connection.
    errdefer net.close($socket);
    def frameMax as int init handshake($socket, $opts);
    return Conn{ socket: $socket, channel: CHANNEL, frameMax: $frameMax };
}

/**
 * Declare a queue (idempotent). `durable` survives a broker restart.
 * @param c {Conn} the connection
 * @param name {string} the queue name ("" for a server-generated name)
 * @param durable {bool} whether the queue is durable
 * @return {QueueInfo} the queue name and message / consumer counts
 * @throws {Error} kind "amqp" on failure
 */
export func declareQueue(c as Conn, name as string, durable as bool) {
    def flags as int init 0;
    if ($durable) {
        $flags = $flags | Q_DURABLE;
    }
    def empty as bytes;
    return declareQueueImpl($c, $name, $flags, putEmptyTable($empty));
}

/**
 * Declare a quorum queue: a Raft-replicated queue type for high availability and
 * data safety. Sets the `x-queue-type=quorum` argument. Quorum queues are always
 * durable (there is no durable flag) and cannot be server-named, so `name` must
 * be non-empty. Re-declaring must use the same type, so do not also declare the
 * same name as a classic queue.
 * @param c {Conn} the connection
 * @param name {string} the queue name (non-empty)
 * @return {QueueInfo} the queue name and message / consumer counts
 * @throws {Error} kind "amqp" on an empty name or a declare failure
 */
export func declareQuorumQueue(c as Conn, name as string) {
    if ($name == "") {
        throw Error{ kind: "amqp", message: "declareQuorumQueue: a quorum queue must have a non-empty name (server-named quorum queues are not allowed)", file: "", line: 0, col: 0 };
    }
    def empty as bytes;
    def table as bytes init putStringTable($empty, {"x-queue-type": "quorum"});
    return declareQueueImpl($c, $name, Q_DURABLE, $table);
}

# declareQueueImpl issues Queue.Declare with pre-computed flags and a fully
# encoded arguments field-table, then parses Declare-Ok into a QueueInfo. Shared
# by declareQueue (empty arguments) and declareQuorumQueue (x-queue-type).
func declareQueueImpl(c as Conn, name as string, flags as int, table as bytes) {
    def args as bytes;
    $args = putShort($args, 0);            # reserved
    $args = putShortStr($args, $name);
    $args = putOctet($args, $flags);
    $args = appendBytes($args, $table);    # arguments field-table
    writeMethod($c.socket, $c.channel, CLS_QUEUE, Q_DECLARE, $args);
    def ok as Method init expectMethod($c.socket, CLS_QUEUE, Q_DECLAREOK, "Queue.Declare-Ok");
    def qname as string init readShortStr($ok.args, 0);
    def off as int init 1 + byteLen($qname);
    def messageCount as int init readLong($ok.args, $off);
    def consumerCount as int init readLong($ok.args, $off + 4);
    return QueueInfo{ name: $qname, messageCount: $messageCount, consumerCount: $consumerCount };
}

/**
 * Publish a message body to an exchange with a routing key. Use exchange "" for
 * the default exchange (routing key = queue name).
 * @param c {Conn} the connection
 * @param exchange {string} the exchange name ("" for the default)
 * @param routingKey {string} the routing key
 * @param body {bytes} the message body
 * @throws {Error} kind "amqp" on failure
 */
export func publish(c as Conn, exchange as string, routingKey as string, body as bytes) {
    def args as bytes;
    $args = putShort($args, 0);                # reserved
    $args = putShortStr($args, $exchange);
    $args = putShortStr($args, $routingKey);
    $args = putOctet($args, 0);                 # mandatory / immediate bits
    writeMethod($c.socket, $c.channel, CLS_BASIC, B_PUBLISH, $args);

    # content header: class, weight, body-size, property-flags (none).
    def hdr as bytes;
    $hdr = putShort($hdr, CLS_BASIC);
    $hdr = putShort($hdr, 0);
    $hdr = putLongLong($hdr, len($body));
    $hdr = putShort($hdr, 0);
    writeFrame($c.socket, FRAME_HEADER, $c.channel, $hdr);

    if (len($body) > 0) {
        # Split the body into frames no larger than the negotiated frame-max: a
        # single FRAME_BODY over the broker's limit (RabbitMQ default 131072)
        # triggers a connection-level error and drops the connection. Each
        # frame carries 8 octets of overhead (1 type + 2 channel + 4 size + 1
        # end), so the payload budget is frameMax - 8. A frameMax of 0 means no
        # limit.
        def maxPayload as int init 0;
        if ($c.frameMax > 8) {
            $maxPayload = $c.frameMax - 8;
        }
        if ($maxPayload <= 0 or len($body) <= $maxPayload) {
            writeFrame($c.socket, FRAME_BODY, $c.channel, $body);
        } else {
            def off as int init 0;
            def total as int init len($body);
            while ($off < $total) {
                def stop as int init $off + $maxPayload;
                if ($stop > $total) {
                    $stop = $total;
                }
                writeFrame($c.socket, FRAME_BODY, $c.channel, sliceBytes($body, $off, $stop));
                $off = $stop;
            }
        }
    }
}

/**
 * Publish a text message (UTF-8). Convenience over publish.
 * @param c {Conn} the connection
 * @param exchange {string} the exchange name ("" for the default)
 * @param routingKey {string} the routing key
 * @param text {string} the message text
 * @throws {Error} kind "amqp" on failure
 */
export func publishText(c as Conn, exchange as string, routingKey as string, text as string) {
    publish($c, $exchange, $routingKey, convert.bytesFromString($text, "utf-8"));
}

/**
 * Pull the next message from a queue with Basic.Get. When `autoAck` is false,
 * ack the returned message with `ack`.
 * @param c {Conn} the connection
 * @param queue {string} the queue name
 * @param autoAck {bool} whether the broker should auto-acknowledge
 * @return {Message} the message (its `empty` field is true when none was ready)
 * @throws {Error} kind "amqp" on failure
 */
export func get(c as Conn, queue as string, autoAck as bool) {
    def args as bytes;
    $args = putShort($args, 0);            # reserved
    $args = putShortStr($args, $queue);
    def noAck as int init 0;
    if ($autoAck) {
        $noAck = 1;
    }
    $args = putOctet($args, $noAck);
    writeMethod($c.socket, $c.channel, CLS_BASIC, B_GET, $args);

    def m as Method init readMethod($c.socket);
    if ($m.classId == CLS_BASIC and $m.methodId == B_GETEMPTY) {
        return Message{ empty: true, deliveryTag: 0, exchange: "", routingKey: "", body: emptyBytes() };
    }
    if (not ($m.classId == CLS_BASIC and $m.methodId == B_GETOK)) {
        fail("unexpected reply to Basic.Get");
    }
    # Get-Ok: delivery-tag(u64) redelivered(bit) exchange(shortstr) routing-key(shortstr) message-count(u32)
    def deliveryTag as int init readLongLong($m.args, 0);
    def off as int init 9;   # 8 (delivery-tag) + 1 (redelivered bit)
    def exchange as string init readShortStr($m.args, $off);
    $off = $off + 1 + byteLen($exchange);
    def routingKey as string init readShortStr($m.args, $off);

    # content header frame carries the body size.
    def hf as Frame init readFrame($c.socket);
    def bodySize as int init readLongLong($hf.payload, 4);

    # body frames until the whole body is collected.
    def body as bytes;
    while (len($body) < $bodySize) {
        def bf as Frame init readFrame($c.socket);
        # Append in place to keep multi-frame body assembly O(N), not O(N^2).
        def k as int init 0;
        while ($k < len($bf.payload)) {
            $body[] = $bf.payload[$k];
            $k = $k + 1;
        }
    }
    return Message{ empty: false, deliveryTag: $deliveryTag, exchange: $exchange, routingKey: $routingKey, body: $body };
}

/**
 * Acknowledge a delivered message by its tag.
 * @param c {Conn} the connection
 * @param deliveryTag {int} the delivery tag from a got Message
 */
export func ack(c as Conn, deliveryTag as int) {
    def args as bytes;
    $args = putLongLong($args, $deliveryTag);
    $args = putOctet($args, 0);   # multiple = false
    writeMethod($c.socket, $c.channel, CLS_BASIC, B_ACK, $args);
}

/**
 * Close the connection cleanly (Connection.Close) and shut the socket.
 * @param c {Conn} the connection
 */
export func close(c as Conn) {
    # The socket is shut even when the polite Connection.Close dialogue throws
    # (a dead broker must not leak the fd).
    defer net.close($c.socket);
    def args as bytes;
    $args = putShort($args, 200);          # reply code: OK
    $args = putShortStr($args, "");      # reply text
    $args = putShort($args, 0);            # class-id
    $args = putShort($args, 0);            # method-id
    writeMethod($c.socket, 0, CLS_CONNECTION, CONN_CLOSE, $args);
    expectMethod($c.socket, CLS_CONNECTION, CONN_CLOSEOK, "Connection.Close-Ok");
}
