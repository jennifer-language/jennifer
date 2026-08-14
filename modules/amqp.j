# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
# pragma-jennifer-version: >=0.25.0
# pragma-jennifer-capability: net

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
use binary;
use convert;
import "./transport.j" as transport;

# Frame types and the frame-end sentinel.
def const FRAME_METHOD as int init 1;
def const FRAME_HEADER as int init 2;
def const FRAME_BODY as int init 3;
def const FRAME_END as int init 206; # 0xCE

# Upper bound on a broker-declared frame size (64 MiB, the tree-wide house rule).
# `readFrame` takes a 32-bit size straight off the wire; without this ceiling a
# hostile or MITM'd broker declaring ~4 GiB would drive `readN` to OOM the
# recover-less interpreter.
def const MAX_FRAME_BYTES as int init 67108864;

# The single channel this client uses.
def const CHANNEL as int init 1;

# The connection-establishment timeout (ms): a slow / unreachable broker fails
# instead of blocking the dial forever.
def const CONNECT_TIMEOUT_MS as int init 30000;

# Class ids.
def const CLS_CONNECTION as int init 10;
def const CLS_CHANNEL as int init 20;
def const CLS_EXCHANGE as int init 40;
def const CLS_QUEUE as int init 50;
def const CLS_BASIC as int init 60;
def const CLS_CONFIRM as int init 85;

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
def const EX_DECLARE as int init 10;
def const EX_DECLAREOK as int init 11;
def const Q_DECLARE as int init 10;
def const Q_DECLAREOK as int init 11;
def const Q_BIND as int init 20;
def const Q_BINDOK as int init 21;
def const B_CONSUME as int init 20;
def const B_CONSUMEOK as int init 21;
def const B_CANCEL as int init 30;
def const B_CANCELOK as int init 31;
def const B_PUBLISH as int init 40;
def const B_DELIVER as int init 60;
def const B_GET as int init 70;
def const B_GETOK as int init 71;
def const B_GETEMPTY as int init 72;
def const B_ACK as int init 80;
def const B_NACK as int init 120;
def const CONFIRM_SELECT as int init 10;
def const CONFIRM_SELECTOK as int init 11;

# The durable flag bit for Queue.Declare (and Exchange.Declare).
def const Q_DURABLE as int init 2;

# The no-ack flag bit (bit 1) for Basic.Consume.
def const B_NOACK as int init 2;

# Content-header property-flag bit masks (Basic properties), most-significant
# bit first: content-type, delivery-mode, correlation-id, reply-to.
def const PROP_CONTENT_TYPE as int init 0x8000; # bit 15
def const PROP_DELIVERY_MODE as int init 0x1000; # bit 12
def const PROP_CORRELATION_ID as int init 0x0400; # bit 10
def const PROP_REPLY_TO as int init 0x0200; # bit 9

# delivery-mode 2 = persistent (survives a broker restart when the queue is durable).
def const DELIVERY_PERSISTENT as int init 2;

/**
 * Connection options.
 * @field host {string} the broker host
 * @field port {int} the broker port (5672 by default)
 * @field user {string} the username
 * @field password {string} the password
 * @field vhost {string} the virtual host ("/" by default)
 * @field security {transport.Security} `transport.Security.None` (plaintext AMQP, the default) or `.Tls` (AMQPS - TLS on connect, verifying the broker certificate); `.Starttls` is rejected (AMQP has no in-band upgrade)
 */
export def struct Options {
    host as string,
    port as int,
    user as string,
    password as string,
    vhost as string,
    security as transport.Security
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

/**
 * Message properties carried on a publish (all optional; a "" string or a false
 * flag omits that property from the wire, keeping the content header minimal).
 * @field contentType {string} the MIME content-type (e.g. "application/json"); "" omits it
 * @field persistent {bool} true sets delivery-mode 2 (persistent); false omits it (transient)
 * @field correlationId {string} the correlation id (request / reply matching); "" omits it
 * @field replyTo {string} the reply-to queue / routing key; "" omits it
 */
export def struct Properties {
    contentType as string,
    persistent as bool,
    correlationId as string,
    replyTo as string
};

/**
 * A message pushed by the broker via Basic.Deliver (returned by receiveDelivery).
 * @field consumerTag {string} the consumer tag the delivery is for
 * @field deliveryTag {int} the delivery tag (pass to ack / nack)
 * @field redelivered {bool} true if the broker has delivered this message before
 * @field exchange {string} the exchange the message came from
 * @field routingKey {string} the routing key
 * @field body {bytes} the message body
 */
export def struct Delivery {
    consumerTag as string,
    deliveryTag as int,
    redelivered as bool,
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
    throw Error{kind: "amqp", message: "amqp: " + $msg, file: "", line: 0, col: 0};
}

# checkFrameSize rejects a broker-declared frame size outside [0, MAX_FRAME_BYTES]
# before it is used to size a read.
func checkFrameSize(n as int) {
    if ($n < 0 or $n > MAX_FRAME_BYTES) {
        fail("frame size out of range: " + convert.toString($n));
    }
    return;
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
    return Options{
        host: $host,
        port: 5672,
        user: $user,
        password: $password,
        vhost: "/",
        security: transport.Security.None
    };
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
    return binary.concat($dst, $src);
}

func sliceBytes(src as bytes, start as int, end as int) {
    return binary.slice($src, $start, $end);
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
        $body = putShortStr($body, $key); # field name (shortstr)
        $body = putOctet($body, 83); # 'S' = long-string value type
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

# --- method encoders / decoders (pure, socket-free) -------------------------
#
# These build or parse the *arguments* portion of a method frame (everything
# after class-id + method-id), or a content header's property section. Kept pure
# so the overlay can assert them byte-exactly without a broker; the socket-facing
# API functions below wrap them with writeMethod / expectMethod.

# encodeExchangeDeclare builds Exchange.Declare args: reserved, exchange, type,
# flag octet (durable), empty arguments table.
func encodeExchangeDeclare(name as string, exType as string, durable as bool) {
    def args as bytes;
    $args = putShort($args, 0); # reserved-1
    $args = putShortStr($args, $name);
    $args = putShortStr($args, $exType);
    def flags as int init 0;
    if ($durable) {
        $flags = $flags | Q_DURABLE; # durable bit (bit 1)
    }
    $args = putOctet($args, $flags); # passive|durable|auto-delete|internal|no-wait
    return putEmptyTable($args);
}

# encodeQueueBind builds Queue.Bind args: reserved, queue, exchange, routing-key,
# no-wait bit, empty arguments table.
func encodeQueueBind(queue as string, exchange as string, routingKey as string) {
    def args as bytes;
    $args = putShort($args, 0); # reserved-1
    $args = putShortStr($args, $queue);
    $args = putShortStr($args, $exchange);
    $args = putShortStr($args, $routingKey);
    $args = putOctet($args, 0); # no-wait = false
    return putEmptyTable($args);
}

# encodeBasicConsume builds Basic.Consume args: reserved, queue, consumer-tag,
# flag octet (no-ack when autoAck), empty arguments table. An empty consumer-tag
# asks the broker to generate one (returned in Consume-Ok).
func encodeBasicConsume(queue as string, consumerTag as string, autoAck as bool) {
    def args as bytes;
    $args = putShort($args, 0); # reserved-1
    $args = putShortStr($args, $queue);
    $args = putShortStr($args, $consumerTag);
    def flags as int init 0;
    if ($autoAck) {
        $flags = $flags | B_NOACK; # no-ack bit (bit 1)
    }
    $args = putOctet($args, $flags); # no-local|no-ack|exclusive|no-wait
    return putEmptyTable($args);
}

# encodeBasicCancel builds Basic.Cancel args: consumer-tag, no-wait bit.
func encodeBasicCancel(consumerTag as string) {
    def args as bytes;
    $args = putShortStr($args, $consumerTag);
    $args = putOctet($args, 0); # no-wait = false
    return $args;
}

# encodeBasicNack builds Basic.Nack args: delivery-tag, flag octet (multiple,
# requeue).
func encodeBasicNack(deliveryTag as int, multiple as bool, requeue as bool) {
    def args as bytes;
    $args = putLongLong($args, $deliveryTag);
    def flags as int init 0;
    if ($multiple) {
        $flags = $flags | 1; # multiple bit (bit 0)
    }
    if ($requeue) {
        $flags = $flags | 2; # requeue bit (bit 1)
    }
    return putOctet($args, $flags);
}

# encodeConfirmSelect builds Confirm.Select args: a single no-wait bit.
func encodeConfirmSelect() {
    def args as bytes;
    return putOctet($args, 0); # no-wait = false
}

# encodeProperties builds a content header's property section: the 16-bit
# property-flags word followed by each present property value, most-significant
# flag bit first (content-type, delivery-mode, correlation-id, reply-to). An
# unset field ("" / false) is omitted from both the flags and the value list, so
# a zero Properties encodes as the two-byte "0000" (no properties).
func encodeProperties(props as Properties) {
    def flags as int init 0;
    if ($props.contentType != "") {
        $flags = $flags | PROP_CONTENT_TYPE;
    }
    if ($props.persistent) {
        $flags = $flags | PROP_DELIVERY_MODE;
    }
    if ($props.correlationId != "") {
        $flags = $flags | PROP_CORRELATION_ID;
    }
    if ($props.replyTo != "") {
        $flags = $flags | PROP_REPLY_TO;
    }
    def out as bytes;
    $out = putShort($out, $flags);
    if ($props.contentType != "") {
        $out = putShortStr($out, $props.contentType);
    }
    if ($props.persistent) {
        $out = putOctet($out, DELIVERY_PERSISTENT);
    }
    if ($props.correlationId != "") {
        $out = putShortStr($out, $props.correlationId);
    }
    if ($props.replyTo != "") {
        $out = putShortStr($out, $props.replyTo);
    }
    return $out;
}

# decodeContentBodySize reads the 64-bit body-size from a content-header frame
# payload (class(2) weight(2) body-size(8) property-flags ...).
func decodeContentBodySize(payload as bytes) {
    return readLongLong($payload, 4);
}

# decodeDeliverMethod parses Basic.Deliver args (consumer-tag, delivery-tag,
# redelivered bit, exchange, routing-key) into a Delivery with an empty body;
# the body is filled from the following content-header + body frames.
func decodeDeliverMethod(args as bytes) {
    def consumerTag as string init readShortStr($args, 0);
    def off as int init 1 + byteLen($consumerTag);
    def deliveryTag as int init readLongLong($args, $off);
    $off = $off + 8;
    def redelivered as bool init ($args[$off] & 1) == 1;
    $off = $off + 1;
    def exchange as string init readShortStr($args, $off);
    $off = $off + 1 + byteLen($exchange);
    def routingKey as string init readShortStr($args, $off);
    def body as bytes;
    return Delivery{
        consumerTag: $consumerTag,
        deliveryTag: $deliveryTag,
        redelivered: $redelivered,
        exchange: $exchange,
        routingKey: $routingKey,
        body: $body
    };
}

# deliveryFrom decodes Basic.Deliver args and attaches an already-assembled body
# - the pure combination of a method frame + content header + body frame(s).
func deliveryFrom(args as bytes, body as bytes) {
    def d as Delivery init decodeDeliverMethod($args);
    $d.body = $body;
    return $d;
}

# --- frame I/O (private) ----------------------------------------------------

func readN(socket as net.Conn, n as int) {
    # Collect the chunks and join once (binary.join, a native Go copy): a per-byte
    # `.j` append runs one interpreted iteration per byte over a large frame, and
    # `out = appendBytes(out, chunk)` would recopy the whole growing buffer each
    # read (O(N^2)).
    def parts as list of bytes init [];
    def got as int init 0;
    while ($got < $n) {
        def chunk as bytes init net.readBytes($socket, $n - $got);
        if (len($chunk) == 0) {
            fail("connection closed mid-frame");
        }
        $parts[] = $chunk;
        $got = $got + len($chunk);
    }
    return binary.join($parts);
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
    checkFrameSize($size);
    def payload as bytes;
    if ($size > 0) {
        $payload = readN($socket, $size);
    }
    def end as bytes init readN($socket, 1);
    if (not ($end[0] == FRAME_END)) {
        fail("bad frame terminator");
    }
    return Frame{ftype: $ftype, channel: $channel, payload: $payload};
}

# readMethod reads a method frame and splits out its class / method / args.
func readMethod(socket as net.Conn) {
    def f as Frame init readFrame($socket);
    if (not ($f.ftype == FRAME_METHOD)) {
        fail("expected a method frame");
    }
    def classId as int init readShort($f.payload, 0);
    def methodId as int init readShort($f.payload, 2);
    return Method{
        classId: $classId,
        methodId: $methodId,
        args: sliceBytes($f.payload, 4, len($f.payload))
    };
}

# expectMethod reads a method frame and asserts its class / method.
func expectMethod(socket as net.Conn, classId as int, methodId as int, what as string) {
    def m as Method init readMethod($socket);
    if (not ($m.classId == $classId and $m.methodId == $methodId)) {
        fail("expected " + $what + ", got class " + convert.toString($m.classId) + " method " +
            convert.toString($m.methodId));
    }
    return $m;
}

# readMessageBody reads a content-header frame (for the body size) and the
# following body frame(s), assembling the whole body. Shared by get (Basic.Get-Ok)
# and receiveDelivery (Basic.Deliver).
func readMessageBody(c as Conn) {
    def hf as Frame init readFrame($c.socket);
    def bodySize as int init decodeContentBodySize($hf.payload);
    # A broker-declared body size is untrusted: cap it before assembling, so a
    # hostile / desynced broker cannot drive an unbounded allocation. The
    # per-frame MAX_FRAME_BYTES check does not bound the aggregate body.
    transport.checkReceiveSize($bodySize, "amqp message body");
    # Collect each frame's payload and join once (binary.join, O(N) native) rather
    # than feeding every body byte through the interpreter.
    def parts as list of bytes init [];
    def got as int init 0;
    while ($got < $bodySize) {
        def bf as Frame init readFrame($c.socket);
        $parts[] = $bf.payload;
        $got = $got + len($bf.payload);
    }
    return binary.join($parts);
}

# writeContentAndBody writes a Basic content-header frame (class, weight,
# body-size, property section) then the body split into frames no larger than the
# negotiated frame-max. `propBytes` is the encoded property-flags + property list
# (a two-byte "0000" for no properties). Shared by publish / publishWith.
func writeContentAndBody(c as Conn, body as bytes, propBytes as bytes) {
    def hdr as bytes;
    $hdr = putShort($hdr, CLS_BASIC);
    $hdr = putShort($hdr, 0); # weight (always 0)
    $hdr = putLongLong($hdr, len($body));
    $hdr = appendBytes($hdr, $propBytes); # property-flags + property list
    writeFrame($c.socket, FRAME_HEADER, $c.channel, $hdr);

    if (len($body) > 0) {
        # A single FRAME_BODY over the broker's limit (RabbitMQ default 131072)
        # triggers a connection-level error and drops the connection. Each frame
        # carries 8 octets of overhead (1 type + 2 channel + 4 size + 1 end), so
        # the payload budget is frameMax - 8. A frameMax of 0 means no limit.
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
    match ($opts.security) {
        when Tls { return net.connectTLS($addr, CONNECT_TIMEOUT_MS); }
        when None { return net.connect($addr, CONNECT_TIMEOUT_MS); }
        when Starttls {
            fail("STARTTLS is not supported; use transport.Security.Tls (AMQPS) or .None");
        }
    }
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
    return Conn{socket: $socket, channel: CHANNEL, frameMax: $frameMax};
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
        throw Error{
            kind: "amqp",
            message: "declareQuorumQueue: a quorum queue must have a non-empty name (server-named quorum queues are not allowed)",
            file: "",
            line: 0,
            col: 0
        };
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
    $args = putShort($args, 0); # reserved
    $args = putShortStr($args, $name);
    $args = putOctet($args, $flags);
    $args = appendBytes($args, $table); # arguments field-table
    writeMethod($c.socket, $c.channel, CLS_QUEUE, Q_DECLARE, $args);
    def ok as Method init expectMethod($c.socket, CLS_QUEUE, Q_DECLAREOK, "Queue.Declare-Ok");
    def qname as string init readShortStr($ok.args, 0);
    def off as int init 1 + byteLen($qname);
    def messageCount as int init readLong($ok.args, $off);
    def consumerCount as int init readLong($ok.args, $off + 4);
    return QueueInfo{name: $qname, messageCount: $messageCount, consumerCount: $consumerCount};
}

/**
 * Declare an exchange (idempotent, durable). `exType` is "direct", "topic", or
 * "fanout".
 * @param c {Conn} the connection
 * @param name {string} the exchange name
 * @param exType {string} the exchange type ("direct" / "topic" / "fanout")
 * @throws {Error} kind "amqp" on failure
 */
export func declareExchange(c as Conn, name as string, exType as string) {
    def args as bytes init encodeExchangeDeclare($name, $exType, true);
    writeMethod($c.socket, $c.channel, CLS_EXCHANGE, EX_DECLARE, $args);
    expectMethod($c.socket, CLS_EXCHANGE, EX_DECLAREOK, "Exchange.Declare-Ok");
}

/**
 * Bind a queue to an exchange with a routing key. For a "fanout" exchange the
 * routing key is ignored; for "direct" it is matched exactly; for "topic" it is
 * a pattern (`*` / `#`).
 * @param c {Conn} the connection
 * @param queue {string} the queue name
 * @param exchange {string} the exchange name
 * @param routingKey {string} the binding routing key / pattern
 * @throws {Error} kind "amqp" on failure
 */
export func bindQueue(c as Conn, queue as string, exchange as string, routingKey as string) {
    def args as bytes init encodeQueueBind($queue, $exchange, $routingKey);
    writeMethod($c.socket, $c.channel, CLS_QUEUE, Q_BIND, $args);
    expectMethod($c.socket, CLS_QUEUE, Q_BINDOK, "Queue.Bind-Ok");
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
    def zero as Properties;
    publishWith($c, $exchange, $routingKey, $body, $zero);
}

/**
 * Publish a message body carrying message properties (content-type, persistence,
 * correlation-id, reply-to). Otherwise identical to publish. Build the property
 * set with `Properties{...}`; an unset field ("" / false) is omitted from the
 * wire.
 * @param c {Conn} the connection
 * @param exchange {string} the exchange name ("" for the default)
 * @param routingKey {string} the routing key
 * @param body {bytes} the message body
 * @param props {Properties} the message properties
 * @throws {Error} kind "amqp" on failure
 */
export func publishWith(
    c as Conn,
    exchange as string,
    routingKey as string,
    body as bytes,
    props as Properties) {
    def args as bytes;
    $args = putShort($args, 0); # reserved
    $args = putShortStr($args, $exchange);
    $args = putShortStr($args, $routingKey);
    $args = putOctet($args, 0); # mandatory / immediate bits
    writeMethod($c.socket, $c.channel, CLS_BASIC, B_PUBLISH, $args);
    writeContentAndBody($c, $body, encodeProperties($props));
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
 * Put the channel into publisher-confirm mode (Confirm.Select). After this, each
 * publish is confirmed by the broker; call waitConfirm once per publish to block
 * for that confirmation. Idempotent.
 * @param c {Conn} the connection
 * @throws {Error} kind "amqp" on failure
 */
export func confirmSelect(c as Conn) {
    def args as bytes init encodeConfirmSelect();
    writeMethod($c.socket, $c.channel, CLS_CONFIRM, CONFIRM_SELECT, $args);
    expectMethod($c.socket, CLS_CONFIRM, CONFIRM_SELECTOK, "Confirm.Select-Ok");
}

/**
 * Block for the broker's confirmation of the next outstanding publish (only valid
 * after confirmSelect). Returns true on Basic.Ack (the broker took
 * responsibility for the message) and false on Basic.Nack (the broker could not).
 * @param c {Conn} the connection
 * @return {bool} true when confirmed, false when nacked
 * @throws {Error} kind "amqp" on an unexpected reply
 */
export func waitConfirm(c as Conn) {
    def m as Method init readMethod($c.socket);
    if ($m.classId == CLS_BASIC and $m.methodId == B_ACK) {
        return true;
    }
    if ($m.classId == CLS_BASIC and $m.methodId == B_NACK) {
        return false;
    }
    fail("expected a publisher confirm (Basic.Ack / Basic.Nack), got class " +
        convert.toString($m.classId) + " method " + convert.toString($m.methodId));
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
    $args = putShort($args, 0); # reserved
    $args = putShortStr($args, $queue);
    def noAck as int init 0;
    if ($autoAck) {
        $noAck = 1;
    }
    $args = putOctet($args, $noAck);
    writeMethod($c.socket, $c.channel, CLS_BASIC, B_GET, $args);

    def m as Method init readMethod($c.socket);
    if ($m.classId == CLS_BASIC and $m.methodId == B_GETEMPTY) {
        return Message{
            empty: true,
            deliveryTag: 0,
            exchange: "",
            routingKey: "",
            body: emptyBytes()
        };
    }
    if (not ($m.classId == CLS_BASIC and $m.methodId == B_GETOK)) {
        fail("unexpected reply to Basic.Get");
    }
    # Get-Ok: delivery-tag(u64) redelivered(bit) exchange(shortstr) routing-key(shortstr) message-count(u32)
    def deliveryTag as int init readLongLong($m.args, 0);
    def off as int init 9; # 8 (delivery-tag) + 1 (redelivered bit)
    def exchange as string init readShortStr($m.args, $off);
    $off = $off + 1 + byteLen($exchange);
    def routingKey as string init readShortStr($m.args, $off);

    # content header frame + body frame(s) carry the message body.
    def body as bytes init readMessageBody($c);
    return Message{
        empty: false,
        deliveryTag: $deliveryTag,
        exchange: $exchange,
        routingKey: $routingKey,
        body: $body
    };
}

/**
 * Acknowledge a delivered message by its tag.
 * @param c {Conn} the connection
 * @param deliveryTag {int} the delivery tag from a got Message
 */
export func ack(c as Conn, deliveryTag as int) {
    def args as bytes;
    $args = putLongLong($args, $deliveryTag);
    $args = putOctet($args, 0); # multiple = false
    writeMethod($c.socket, $c.channel, CLS_BASIC, B_ACK, $args);
}

/**
 * Negatively acknowledge a delivered message (Basic.Nack). With `requeue` true
 * the broker re-queues the message for redelivery; with false it is dropped (or
 * dead-lettered if configured). Nacks a single message (not "multiple").
 * @param c {Conn} the connection
 * @param deliveryTag {int} the delivery tag from a Message / Delivery
 * @param requeue {bool} whether the broker should re-queue the message
 */
export func nack(c as Conn, deliveryTag as int, requeue as bool) {
    def args as bytes init encodeBasicNack($deliveryTag, false, $requeue);
    writeMethod($c.socket, $c.channel, CLS_BASIC, B_NACK, $args);
}

/**
 * Start a server-pushed subscription with Basic.Consume and return the consumer
 * tag (the broker generates one). Follow with receiveDelivery in a loop - wrap it
 * in a `spawn` so the pushed deliveries do not block the rest of the program.
 * When `autoAck` is false, ack (or nack) each Delivery by its deliveryTag.
 * @param c {Conn} the connection
 * @param queue {string} the queue name
 * @param autoAck {bool} whether the broker should auto-acknowledge each delivery
 * @return {string} the consumer tag (pass to cancelConsume)
 * @throws {Error} kind "amqp" on failure
 */
export func consume(c as Conn, queue as string, autoAck as bool) {
    def args as bytes init encodeBasicConsume($queue, "", $autoAck);
    writeMethod($c.socket, $c.channel, CLS_BASIC, B_CONSUME, $args);
    def ok as Method init expectMethod($c.socket, CLS_BASIC, B_CONSUMEOK, "Basic.Consume-Ok");
    return readShortStr($ok.args, 0);
}

/**
 * Block for the next server-pushed message (Basic.Deliver + content header + body
 * frame(s)) and return it as a Delivery. Only valid after consume. This is the
 * cooperative receive loop: there is no callback - the app calls receiveDelivery
 * repeatedly, typically from inside its own `spawn`, and acts on each Delivery.
 * @param c {Conn} the connection
 * @return {Delivery} the next delivered message
 * @throws {Error} kind "amqp" on a non-delivery frame or a dropped connection
 */
export func receiveDelivery(c as Conn) {
    def m as Method init readMethod($c.socket);
    if (not ($m.classId == CLS_BASIC and $m.methodId == B_DELIVER)) {
        fail("expected Basic.Deliver, got class " + convert.toString($m.classId) + " method " +
            convert.toString($m.methodId));
    }
    def body as bytes init readMessageBody($c);
    return deliveryFrom($m.args, $body);
}

/**
 * Cancel a subscription started with consume (Basic.Cancel). After this the
 * broker sends no further deliveries for that consumer tag.
 * @param c {Conn} the connection
 * @param consumerTag {string} the consumer tag returned by consume
 * @throws {Error} kind "amqp" on failure
 */
export func cancelConsume(c as Conn, consumerTag as string) {
    def args as bytes init encodeBasicCancel($consumerTag);
    writeMethod($c.socket, $c.channel, CLS_BASIC, B_CANCEL, $args);
    expectMethod($c.socket, CLS_BASIC, B_CANCELOK, "Basic.Cancel-Ok");
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
    $args = putShort($args, 200); # reply code: OK
    $args = putShortStr($args, ""); # reply text
    $args = putShort($args, 0); # class-id
    $args = putShort($args, 0); # method-id
    writeMethod($c.socket, 0, CLS_CONNECTION, CONN_CLOSE, $args);
    expectMethod($c.socket, CLS_CONNECTION, CONN_CLOSEOK, "Connection.Close-Ok");
}
