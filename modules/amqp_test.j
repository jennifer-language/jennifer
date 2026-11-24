# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# amqp_test.j - white-box tests for amqp.j. Run with:
#
#     jennifer test modules/amqp_test.j
#
# These exercise the pure AMQP integer / string / table encoding and decoding
# with no network; the full handshake + declare / publish / get / ack round trip
# is driven against a fake AMQP broker in the Go suite (cmd/jennifer/amqp_test.go).
# amqp.j already `use`s net and convert, so the overlay adds testing (and
# encoding, for byte-exact hex assertions).
use convert;
use testing;
use encoding;
use strings;
use task;

# hex renders a bytes value as lowercase hex.
func hex(b as bytes) {
    return encoding.toText("hex", $b);
}

# fromHex builds a bytes value from a hex string.
func fromHex(s as string) {
    return encoding.fromText("hex", $s);
}

func testPutIntegers() {
    def e as bytes;
    testing.assertEqual(hex(putShort($e, 0x1234)), "1234");
    def f as bytes;
    testing.assertEqual(hex(putLong($f, 0x01020304)), "01020304");
    def g as bytes;
    testing.assertEqual(hex(putLongLong($g, 258)), "0000000000000102");
    def h as bytes;
    testing.assertEqual(hex(putOctet($h, 0xab)), "ab");
}

func testPutStrings() {
    def e as bytes;
    testing.assertEqual(hex(putShortStr($e, "hi")), "026869");
    def f as bytes;
    testing.assertEqual(hex(putLongStr($f, "hi")), "000000026869");
    def g as bytes;
    testing.assertEqual(hex(putEmptyTable($g)), "00000000");
}

# putStringTable encodes an AMQP field-table of long-string values - the wire
# form behind a quorum queue's {"x-queue-type": "quorum"} argument.
func testStringTable() {
    def e as bytes;
    def t as bytes init putStringTable($e, {"x-queue-type": "quorum"});
    # length(24) | key "x-queue-type" (0c + 12 bytes) | 'S'(53) | longstr "quorum" (len 6 + 6 bytes)
    testing.assertEqual(hex($t), "000000180c782d71756575652d74797065530000000671756f72756d");
    # An empty map encodes as an empty table.
    def m as map of string to string init {};
    def empty as bytes;
    testing.assertEqual(hex(putStringTable($empty, $m)), "00000000");
}

func testShortStrTruncatedNul() {
    # SASL PLAIN response NUL user NUL pass -> 00 75 00 70
    def raw as bytes init encoding.encode("utf-8", saslPlain("u", "p"));
    testing.assertEqual(hex($raw), "00750070");
}

func testReadIntegers() {
    testing.assertEqual(readShort(fromHex("1234"), 0), 0x1234);
    testing.assertEqual(readLong(fromHex("01020304"), 0), 0x01020304);
    testing.assertEqual(readLongLong(fromHex("0000000000000102"), 0), 258);
    # offset read
    testing.assertEqual(readShort(fromHex("aaaa1234"), 2), 0x1234);
    # high bits read unsigned
    testing.assertEqual(readShort(fromHex("ffff"), 0), 65535);
    testing.assertEqual(readLong(fromHex("ffffffff"), 0), 4294967295);
    testing.assertEqual(readLongLong(fromHex("7fffffffffffffff"), 0), 9223372036854775807);
}

# A 64-bit field above 2^63 - 1 throws; shifting it into an int wrapped it to
# a negative number.
func testReadLongLongRefusesTheTopBit() {
    def failed as bool init false;
    try {
        readLongLong(fromHex("ffffffffffffffff"), 0);
    } catch (e) {
        $failed = true;
        testing.assertContains($e.message, "exceeds the int range");
    }
    testing.assertTrue($failed);
}

func testReadShortStr() {
    testing.assertEqual(readShortStr(fromHex("026869"), 0), "hi");
    testing.assertEqual(readShortStr(fromHex("ffff026869"), 2), "hi");
}

func testByteLen() {
    testing.assertEqual(byteLen("hi"), 2);
    testing.assertEqual(byteLen(""), 0);
    # a 2-byte UTF-8 character
    testing.assertEqual(byteLen(encoding.decode("utf-8", fromHex("c3a9"))), 2);
}

func testRoundTripShortStrOffset() {
    # Build "exch" then "rk" as consecutive short-strings, decode both by
    # advancing the offset with byteLen (the Get-Ok parsing pattern).
    def buf as bytes;
    $buf = putShortStr($buf, "exch");
    $buf = putShortStr($buf, "rk");
    def a as string init readShortStr($buf, 0);
    testing.assertEqual($a, "exch");
    def b as string init readShortStr($buf, 1 + byteLen($a));
    testing.assertEqual($b, "rk");
}

# OM-004: a broker-declared frame size beyond the cap is rejected before it
# sizes a read.
func overFrameCap() {
    checkFrameSize(MAX_FRAME_BYTES + 1, 0);
}
func negFrameCap() {
    checkFrameSize(-1, 0);
}
func testFrameSizeCap() {
    testing.assertThrows("overFrameCap", "limit");
    testing.assertThrows("negFrameCap", "amqp");
    checkFrameSize(MAX_FRAME_BYTES, 0); # exactly at the limit does not throw
    checkFrameSize(0, 0);
}

# --- new method encoders / decoders -----------------------------------------

# encodeExchangeDeclare: reserved(0) | "logs" | "fanout" | durable bit | empty table.
func testEncodeExchangeDeclare() {
    testing.assertEqual(
        hex(encodeExchangeDeclare("logs", "fanout", true)),
        "0000046c6f67730666616e6f75740200000000");
    # non-durable clears the flag octet.
    testing.assertEqual(
        hex(encodeExchangeDeclare("logs", "fanout", false)),
        "0000046c6f67730666616e6f75740000000000");
}

# encodeQueueBind: reserved(0) | "jobs" | "logs" | "rk" | no-wait(0) | empty table.
func testEncodeQueueBind() {
    testing.assertEqual(
        hex(encodeQueueBind("jobs", "logs", "rk")),
        "0000046a6f6273046c6f677302726b0000000000");
}

# encodeBasicConsume: reserved(0) | "jobs" | "ctag" | flags | empty table.
func testEncodeBasicConsume() {
    # autoAck false -> flags octet 0x00.
    testing.assertEqual(
        hex(encodeBasicConsume("jobs", "ctag", false)),
        "0000046a6f627304637461670000000000");
    # autoAck true -> no-ack bit (0x02).
    testing.assertEqual(
        hex(encodeBasicConsume("jobs", "ctag", true)),
        "0000046a6f627304637461670200000000");
    # An empty consumer tag asks the broker to name it (single 0x00 length byte).
    testing.assertEqual(hex(encodeBasicConsume("q", "", false)), "00000171000000000000");
}

# encodeBasicNack: delivery-tag(u64) | flags(multiple, requeue).
func testEncodeBasicNack() {
    # multiple=false, requeue=true -> flags 0x02.
    testing.assertEqual(hex(encodeBasicNack(5, false, true)), "000000000000000502");
    # multiple=true, requeue=false -> flags 0x01.
    testing.assertEqual(hex(encodeBasicNack(7, true, false)), "000000000000000701");
    # both -> flags 0x03.
    testing.assertEqual(hex(encodeBasicNack(1, true, true)), "000000000000000103");
}

# encodeConfirmSelect: a single no-wait bit (0x00).
func testEncodeConfirmSelect() {
    testing.assertEqual(hex(encodeConfirmSelect()), "00");
}

# encodeProperties: property-flags word then each present value, MSB flag first.
func testEncodeProperties() {
    # A zero Properties -> flags 0x0000, no values.
    def zero as Properties;
    testing.assertEqual(hex(encodeProperties($zero)), "0000");
    # content-type + persistent -> flags 0x9000, "text/plain", delivery-mode 2.
    def a as Properties init Properties{
        contentType: "text/plain",
        persistent: true,
        correlationId: "",
        replyTo: ""
    };
    testing.assertEqual(hex(encodeProperties($a)), "90000a746578742f706c61696e02");
    # correlation-id + reply-to -> flags 0x0600, "abc", "q1" (correlation before reply).
    def b as Properties init Properties{
        contentType: "",
        persistent: false,
        correlationId: "abc",
        replyTo: "q1"
    };
    testing.assertEqual(hex(encodeProperties($b)), "060003616263027131");
}

# decodeDeliverMethod parses Basic.Deliver args into a Delivery (empty body).
func testDecodeDeliverMethod() {
    def args as bytes;
    $args = putShortStr($args, "ctag");
    $args = putLongLong($args, 42);
    $args = putOctet($args, 1); # redelivered
    $args = putShortStr($args, "logs");
    $args = putShortStr($args, "rk");
    def d as Delivery init decodeDeliverMethod($args);
    testing.assertEqual($d.consumerTag, "ctag");
    testing.assertEqual($d.deliveryTag, 42);
    testing.assertEqual($d.redelivered, true);
    testing.assertEqual($d.exchange, "logs");
    testing.assertEqual($d.routingKey, "rk");
    testing.assertEqual(len($d.body), 0);
}

# A method frame + content header + body frame(s) assemble into one Delivery:
# decode the body size from the header payload, then attach the collected body.
func testDeliveryFromContentAndBody() {
    def args as bytes;
    $args = putShortStr($args, ""); # broker-named consumer
    $args = putLongLong($args, 9);
    $args = putOctet($args, 0); # not redelivered
    $args = putShortStr($args, ""); # default exchange
    $args = putShortStr($args, "jobs");
    # content header: class | weight | body-size | property-flags.
    def hdr as bytes;
    $hdr = putShort($hdr, CLS_BASIC);
    $hdr = putShort($hdr, 0);
    $hdr = putLongLong($hdr, 5);
    $hdr = putShort($hdr, 0);
    testing.assertEqual(decodeContentBodySize($hdr), 5);
    def body as bytes init encoding.encode("utf-8", "hello");
    def d as Delivery init deliveryFrom($args, $body);
    testing.assertEqual($d.deliveryTag, 9);
    testing.assertEqual($d.redelivered, false);
    testing.assertEqual($d.exchange, "");
    testing.assertEqual($d.routingKey, "jobs");
    testing.assertEqual(hex($d.body), hex($body));
}

# A broker Close is an AmqpError with the scope, the reply code and text, and
# the method it refused; any other failure has none of them.
func testABrokerCloseIsAnAmqpError() {
    def e as bytes;
    def args as bytes init putShort(
        putShort(
            putShortStr(putShort($e, 406), "PRECONDITION_FAILED - inequivalent arg 'type'"),
            40),
        10);
    try {
        brokerClosed(CLS_CHANNEL, $args);
        throw Error{kind: "missed", message: "a channel close"};
    } catch (err) {
        testing.assertEqual($err.kind, "amqp");
        testing.assertEqual($err.scope, "channel");
        testing.assertEqual($err.replyCode, 406);
        testing.assertEqual($err.replyText, "PRECONDITION_FAILED - inequivalent arg 'type'");
        testing.assertEqual($err.classId, 40);
        testing.assertEqual($err.methodId, 10);
        testing.assertEqual(
            $err.message,
            "amqp: broker closed the channel: 406 PRECONDITION_FAILED - inequivalent arg 'type'");
    }
    try {
        brokerClosed(CLS_CONNECTION, putShortStr(putShort($e, 320), "CONNECTION_FORCED"));
        throw Error{kind: "missed", message: "a connection close"};
    } catch (err) {
        testing.assertEqual($err.scope, "connection");
        testing.assertEqual($err.replyCode, 320);
        testing.assertEqual($err.classId, 0);
    }
    try {
        checkFrameSize(-1, 0);
        throw Error{kind: "missed", message: "a bad frame size"};
    } catch (err) {
        testing.assertEqual($err.kind, "amqp");
        testing.assertEqual($err.scope, "");
        testing.assertEqual($err.replyCode, 0);
    }
}

# A broker that resets the connection mid-frame is a net.NetError (kind
# "net"), passed on as it is, not re-tagged as an AmqpError.
#
# The server aborts only after reading a byte the client sent once connected,
# so the reset cannot reach the client while `net.connect` is still running
# (on a busy machine an abort right after `accept` sometimes did, and connect
# itself failed with "connection reset by peer").
func testAResetIsANetError() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        def conn as net.Conn init net.accept($listener);
        net.setReadDeadline($conn, 2000);
        net.readN($conn, 1);
        net.abort($conn);
    };
    def c as net.Conn init net.connect(net.address($listener));
    net.writeBytes($c, encoding.encode("utf-8", "x"));
    try {
        readN($c, 8, 2000);
        throw Error{kind: "missed", message: "a reset"};
    } catch (e) {
        testing.assertEqual($e.kind, "net");
        testing.assertEqual($e.reason, "reset");
    }
    task.wait($server);
    net.close($c);
    net.close($listener);
}

# A broker that closes the connection mid-frame is kind "closed", passed on as
# net threw it, not re-tagged as an AmqpError.
func testAPeerCloseMidFrameIsClosed() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        def conn as net.Conn init net.accept($listener);
        net.writeBytes($conn, encoding.encode("utf-8", "AB"));
        net.close($conn);
    };
    def c as net.Conn init net.connect(net.address($listener));
    try {
        readN($c, 8, 2000);
        throw Error{kind: "missed", message: "a close mid-frame"};
    } catch (e) {
        testing.assertEqual($e.kind, "closed");
        testing.assertContains($e.message, "2 of 8 bytes");
    }
    task.wait($server);
    net.close($c);
    net.close($listener);
}

# A body split into many tiny and empty frames, across the BODY_BATCH joins,
# assembles in order.
func testBodyFromManySmallFrames() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def want as bytes;
    for (def i in 0..2500) {
        $want[] = $i & 0xff;
    }
    def server as task of null init spawn {
        def conn as net.Conn init net.accept($listener);
        def e as bytes;
        # content header: class, weight, body size, no properties
        writeFrame(
            $conn,
            FRAME_HEADER,
            1,
            putShort(putLongLong(putShort(putShort($e, 60), 0), 2500), 0));
        for (def i in 0..2500) {
            writeFrame($conn, FRAME_BODY, 1, $want[$i..$i + 1]);
            writeFrame($conn, FRAME_BODY, 1, $e);
        }
        net.setReadDeadline($conn, 5000);
        net.readN($conn, 1);
        net.close($conn);
    };
    def c as net.Conn init net.connect(net.address($listener));
    def body as bytes init readMessageBody(newConn($c, 0));
    net.writeBytes($c, encoding.encode("utf-8", "x"));
    task.wait($server);
    net.close($c);
    net.close($listener);
    testing.assertEqual(hex($body), hex($want));
}

# --- prefetch, mandatory publishing, auth failure, close ----------------------

# text renders a string as hex (for building expected wire bytes).
func textHex(s as string) {
    return hex(encoding.encode("utf-8", $s));
}

# writeContent writes a Basic content header and one body frame (the fake
# broker's half of a returned or delivered message).
func writeContent(conn as net.Conn, body as bytes) {
    def e as bytes;
    writeFrame(
        $conn,
        FRAME_HEADER,
        1,
        putShort(putLongLong(putShort(putShort($e, CLS_BASIC), 0), len($body)), 0));
    writeFrame($conn, FRAME_BODY, 1, $body);
}

# confirmArgs builds Basic.Ack args: delivery-tag, multiple = false.
func confirmArgs(tag as int) {
    def e as bytes;
    return putOctet(putLongLong($e, $tag), 0);
}

# readClientMethod is the fake broker's readMethod: a client's Connection.Close
# is a request to answer, not an error to throw.
func readClientMethod(conn as net.Conn) {
    def f as Frame init readFrame($conn, 5000, 0);
    testing.assertEqual($f.ftype, FRAME_METHOD);
    return Method{
        classId: readShort($f.payload, 0),
        methodId: readShort($f.payload, 2),
        args: sliceBytes($f.payload, 4, len($f.payload))
    };
}

# serveClose reads the client's Connection.Close and answers Close-Ok.
func serveClose(conn as net.Conn) {
    def m as Method init readClientMethod($conn);
    testing.assertEqual($m.classId, CLS_CONNECTION);
    testing.assertEqual($m.methodId, CONN_CLOSE);
    def e as bytes;
    writeMethod($conn, 0, CLS_CONNECTION, CONN_CLOSEOK, $e);
}

# serveConfirmSelect reads the client's Confirm.Select and answers Select-Ok.
func serveConfirmSelect(conn as net.Conn) {
    def m as Method init readClientMethod($conn);
    testing.assertEqual($m.classId, CLS_CONFIRM);
    testing.assertEqual($m.methodId, CONFIRM_SELECT);
    def e as bytes;
    writeMethod($conn, 1, CLS_CONFIRM, CONFIRM_SELECTOK, $e);
}

# readPublishes reads n publishes (method, content header, body) off the wire.
func readPublishes(conn as net.Conn, n as int) {
    for (def i in 0..$n) {
        readClientMethod($conn);
        readMessageBody(newConn($conn, 0));
    }
}

# multipleArgs builds Basic.Ack / Basic.Nack args with the multiple bit set.
func multipleArgs(tag as int) {
    def e as bytes;
    return putOctet(putLongLong($e, $tag), 1);
}

# encodeBasicQos: prefetch-size 0 | prefetch-count | global = false.
func testEncodeBasicQos() {
    testing.assertEqual(hex(encodeBasicQos(1)), "00000000000100");
    testing.assertEqual(hex(encodeBasicQos(65535)), "00000000ffff00");
}

# encodeBasicPublish: reserved | exchange | routing-key | mandatory bit.
func testEncodeBasicPublish() {
    testing.assertEqual(hex(encodeBasicPublish("", "q", false)), "0000000171" + "00");
    testing.assertEqual(hex(encodeBasicPublish("", "q", true)), "0000000171" + "01");
}

# Start-Ok's client properties advertise authentication_failure_close, so
# RabbitMQ answers bad credentials with Connection.Close 403.
func testEncodeClientProperties() {
    testing.assertEqual(
        hex(encodeClientProperties()),
        "00000031" + "0c" + textHex("capabilities") + "46" + "0000001f" + "1c" +
            textHex("authentication_failure_close") + "7401");
}

# qos sends Basic.Qos with the prefetch count and waits for Qos-Ok; a count
# outside the 16-bit field is refused before anything is sent.
func testQosSendsBasicQos() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of string init spawn {
        def conn as net.Conn init net.accept($listener);
        def m as Method init readMethod($conn, 5000, 0);
        def e as bytes;
        writeMethod($conn, 1, CLS_BASIC, B_QOSOK, $e);
        net.setReadDeadline($conn, 5000);
        net.readN($conn, 1);
        net.close($conn);
        return "{$m.classId}.{$m.methodId} " + hex($m.args);
    };
    def c as Conn init newConn(net.connect(net.address($listener)), 0);
    for (def bad in [-1, 65536]) {
        try {
            qos($c, $bad);
            throw Error{kind: "missed", message: "a prefetch count out of range"};
        } catch (err) {
            testing.assertEqual($err.kind, "amqp");
            testing.assertContains($err.message, "outside 0..65535");
        }
    }
    qos($c, 1);
    net.writeBytes($c.socket, encoding.encode("utf-8", "x"));
    testing.assertEqual(task.wait($server), "60.10 00000000000100");
    net.close($c.socket);
    net.close($listener);
}

# A bad login: a broker that is told authentication_failure_close answers
# Connection.Close 403 ACCESS_REFUSED, which connect throws as an AmqpError.
# Like RabbitMQ, the fake broker drops the socket for a client that does not
# advertise the capability (kind "closed").
func testBadCredentialsAreAccessRefused() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        def conn as net.Conn init net.accept($listener);
        net.setReadDeadline($conn, 5000);
        net.readN($conn, 8); # protocol header
        def start as bytes;
        $start = putOctet(putOctet($start, 0), 9); # version 0-9
        $start = putEmptyTable($start); # server properties
        $start = putLongStr($start, "PLAIN");
        $start = putLongStr($start, "en_US");
        writeMethod($conn, 0, CLS_CONNECTION, CONN_START, $start);
        def startOk as Frame init readFrame($conn, 5000, 0);
        def capability as string init textHex("authentication_failure_close") + "7401";
        if (strings.contains(hex($startOk.payload), $capability)) {
            def args as bytes;
            $args = putShort($args, 403);
            $args = putShortStr(
                $args,
                "ACCESS_REFUSED - Login was refused using authentication mechanism PLAIN.");
            $args = putShort(putShort($args, 0), 0);
            writeMethod($conn, 0, CLS_CONNECTION, CONN_CLOSE, $args);
        }
        net.close($conn);
    };
    def addr as string init net.address($listener);
    def port as int init convert.toInt($addr[strings.lastIndexOf($addr, ":") + 1..len($addr)]);
    try {
        connect(withPort(options("127.0.0.1", "guest", "wrong"), $port));
        throw Error{kind: "missed", message: "a refused login"};
    } catch (err) {
        testing.assertEqual($err.kind, "amqp");
        testing.assertEqual($err.scope, "connection");
        testing.assertEqual($err.replyCode, 403);
        testing.assertContains($err.replyText, "ACCESS_REFUSED");
    }
    task.wait($server);
    net.close($listener);
}

# A mandatory publish no queue takes comes back as Basic.Return before its
# confirm; waitConfirm reads both and throws the return as an AmqpError with
# scope "return", leaving the channel in step for the next publish.
func testMandatoryReturnThrowsFromWaitConfirm() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of string init spawn {
        def conn as net.Conn init net.accept($listener);
        serveConfirmSelect($conn);
        def publish as Method init readMethod($conn, 5000, 0);
        def body as bytes init readMessageBody(newConn($conn, 0));
        def ret as bytes;
        $ret = putShort($ret, 312);
        $ret = putShortStr($ret, "NO_ROUTE");
        $ret = putShortStr($ret, "orders");
        $ret = putShortStr($ret, "order.lost");
        writeMethod($conn, 1, CLS_BASIC, B_RETURN, $ret);
        writeContent($conn, $body);
        writeMethod($conn, 1, CLS_BASIC, B_ACK, confirmArgs(1));
        # the next, routable publish
        readMethod($conn, 5000, 0);
        readMessageBody(newConn($conn, 0));
        writeMethod($conn, 1, CLS_BASIC, B_ACK, confirmArgs(2));
        serveClose($conn);
        net.close($conn);
        return hex($publish.args);
    };
    def c as Conn init newConn(net.connect(net.address($listener)), 0);
    confirmSelect($c);
    def zero as Properties;
    def body as bytes init encoding.encode("utf-8", "hello");
    publishMandatory($c, "orders", "order.lost", $body, $zero);
    try {
        waitConfirm($c);
        throw Error{kind: "missed", message: "a returned message"};
    } catch (err) {
        testing.assertEqual($err.kind, "amqp");
        testing.assertEqual($err.scope, "return");
        testing.assertEqual($err.replyCode, 312);
        testing.assertEqual($err.replyText, "NO_ROUTE");
        testing.assertEqual($err.classId, CLS_BASIC);
        testing.assertEqual($err.methodId, B_PUBLISH);
        testing.assertEqual(
            $err.message,
            "amqp: broker returned the message published to exchange 'orders' with routing " +
                "key 'order.lost': 312 NO_ROUTE");
    }
    publish($c, "", "jobs", $body);
    testing.assertTrue(waitConfirm($c));
    close($c);
    # the publish carried the mandatory bit
    testing.assertEqual(
        task.wait($server),
        "0000" + "06" + textHex("orders") + "0a" + textHex("order.lost") + "01");
    net.close($listener);
}

# close reads past what the broker sent before it saw Connection.Close - an
# unread confirm, a returned message, an in-flight delivery - to Close-Ok.
# A skipped waitConfirm made close throw "expected Connection.Close-Ok".
func testCloseDrainsPendingFrames() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        def conn as net.Conn init net.accept($listener);
        def m as Method init readClientMethod($conn);
        testing.assertEqual($m.methodId, CONN_CLOSE);
        def body as bytes init encoding.encode("utf-8", "late");
        writeMethod($conn, 1, CLS_BASIC, B_ACK, confirmArgs(1));
        writeMethod($conn, 1, CLS_BASIC, B_NACK, confirmArgs(2));
        def ret as bytes;
        $ret = putShortStr(putShort($ret, 312), "NO_ROUTE");
        $ret = putShortStr(putShortStr($ret, ""), "nowhere");
        writeMethod($conn, 1, CLS_BASIC, B_RETURN, $ret);
        writeContent($conn, $body);
        def deliver as bytes;
        $deliver = putOctet(putLongLong(putShortStr($deliver, "ctag"), 7), 0);
        $deliver = putShortStr(putShortStr($deliver, ""), "jobs");
        writeMethod($conn, 1, CLS_BASIC, B_DELIVER, $deliver);
        writeContent($conn, $body);
        def e as bytes;
        writeMethod($conn, 0, CLS_CONNECTION, CONN_CLOSEOK, $e);
        net.close($conn);
    };
    close(newConn(net.connect(net.address($listener)), 0));
    task.wait($server);
    net.close($listener);
}

# close still refuses a method that cannot be pending: a reply nobody asked for.
func testCloseRefusesAStrayReply() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        def conn as net.Conn init net.accept($listener);
        readClientMethod($conn);
        def ok as bytes;
        $ok = putLong(putLong(putShortStr($ok, "jobs"), 0), 0);
        writeMethod($conn, 1, CLS_QUEUE, Q_DECLAREOK, $ok);
        def e as bytes;
        writeMethod($conn, 0, CLS_CONNECTION, CONN_CLOSEOK, $e);
        net.close($conn);
    };
    try {
        close(newConn(net.connect(net.address($listener)), 0));
        throw Error{kind: "missed", message: "a stray reply"};
    } catch (err) {
        testing.assertEqual($err.kind, "amqp");
        testing.assertEqual(
            $err.message,
            "amqp: expected Connection.Close-Ok, got class 50 method 11");
    }
    task.wait($server);
    net.close($listener);
}

# A call that reads a reply names a skipped waitConfirm when a confirm arrives
# in its place.
func testAPendingConfirmIsNamed() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        def conn as net.Conn init net.accept($listener);
        readMethod($conn, 5000, 0);
        writeMethod($conn, 1, CLS_BASIC, B_ACK, confirmArgs(1));
        net.setReadDeadline($conn, 5000);
        net.readN($conn, 1);
        net.close($conn);
    };
    def c as Conn init newConn(net.connect(net.address($listener)), 0);
    try {
        declareQueue($c, "jobs", true);
        throw Error{kind: "missed", message: "a pending confirm"};
    } catch (err) {
        testing.assertEqual($err.kind, "amqp");
        testing.assertContains($err.message, "call waitConfirm once per publish");
    }
    net.writeBytes($c.socket, encoding.encode("utf-8", "x"));
    task.wait($server);
    net.close($c.socket);
    net.close($listener);
}

# One Basic.Ack or Basic.Nack with `multiple` set confirms every publish up to
# its tag; waitConfirm still reports one publish per call. A Basic.Ack
# multiple=true for 3 publishes answers three waitConfirm calls with true, a
# Nack multiple=true covering the next 2 answers two with false. Counting the
# Ack as one confirm returned the Nack for publish 2 and timed out at 4.
func testMultipleConfirmsCoverSeveralPublishes() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        def conn as net.Conn init net.accept($listener);
        serveConfirmSelect($conn);
        readPublishes($conn, 5);
        writeMethod($conn, 1, CLS_BASIC, B_ACK, multipleArgs(3));
        writeMethod($conn, 1, CLS_BASIC, B_NACK, multipleArgs(5));
        # 6 nacked on its own, then a multiple ack up to 8 that covers it; 7 is
        # a mandatory publish the broker returns before the ack
        readPublishes($conn, 3);
        writeMethod($conn, 1, CLS_BASIC, B_NACK, confirmArgs(6));
        def ret as bytes;
        $ret = putShortStr(putShort($ret, 312), "NO_ROUTE");
        $ret = putShortStr(putShortStr($ret, "orders"), "order.lost");
        writeMethod($conn, 1, CLS_BASIC, B_RETURN, $ret);
        writeContent($conn, encoding.encode("utf-8", "m7"));
        writeMethod($conn, 1, CLS_BASIC, B_ACK, multipleArgs(8));
        serveClose($conn);
        net.close($conn);
    };
    def c as Conn init newConn(net.connect(net.address($listener)), 0);
    confirmSelect($c);
    def zero as Properties;
    for (def i in 0..5) {
        publishText($c, "", "jobs", "m{$i + 1}");
    }
    def got as list of bool init [];
    for (def i in 0..5) {
        $got[] = waitConfirm($c);
    }
    testing.assertEqual($got, [true, true, true, false, false]);
    publishText($c, "", "jobs", "m6");
    publishMandatory($c, "orders", "order.lost", encoding.encode("utf-8", "m7"), $zero);
    publishMandatory($c, "orders", "order.created", encoding.encode("utf-8", "m8"), $zero);
    testing.assertFalse(waitConfirm($c));
    try {
        waitConfirm($c);
        throw Error{kind: "missed", message: "a returned message"};
    } catch (err) {
        testing.assertEqual($err.scope, "return");
        testing.assertEqual($err.replyCode, 312);
    }
    testing.assertTrue(waitConfirm($c));
    try {
        waitConfirm($c);
        throw Error{kind: "missed", message: "no publish outstanding"};
    } catch (err) {
        testing.assertEqual($err.kind, "amqp");
        testing.assertContains($err.message, "no publish awaiting a confirm");
    }
    close($c);
    task.wait($server);
    net.close($listener);
}

# A confirm for a tag never published, or for one already confirmed, is a
# protocol error.
func testNoteConfirmIsStrict() {
    def single as map of int to bool init {};
    def mandatory as list of Sent init [];
    def returned as list of Returned init [];
    def st as ConfirmState init ConfirmState{
        on: true,
        next: 4,
        waited: 0,
        covered: 0,
        single: $single,
        mandatory: $mandatory,
        head: 0,
        returned: $returned
    };
    for (def tag in [0, 4]) {
        try {
            noteConfirm($st, $tag, false, true);
            throw Error{kind: "missed", message: "an unknown tag"};
        } catch (err) {
            testing.assertEqual($err.kind, "amqp");
            testing.assertContains($err.message, "but 3 publishes were sent");
        }
    }
    $st = noteConfirm($st, 1, false, false);
    $st = noteConfirm($st, 2, true, true);
    testing.assertEqual($st.covered, 2);
    testing.assertFalse($st.single[1]);
    for (def tag in [1, 2]) {
        try {
            noteConfirm($st, $tag, false, true);
            throw Error{kind: "missed", message: "a repeated tag"};
        } catch (err) {
            testing.assertContains($err.message, "delivery tag {$tag} twice");
        }
    }
}

# waitConfirm before confirmSelect is an error, not a 30 s wait for a reply.
func testWaitConfirmNeedsConfirmSelect() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def c as Conn init newConn(net.connect(net.address($listener)), 0);
    try {
        waitConfirm($c);
        throw Error{kind: "missed", message: "no confirm mode"};
    } catch (err) {
        testing.assertEqual($err.kind, "amqp");
        testing.assertEqual($err.message, "amqp: waitConfirm before confirmSelect");
    }
    net.close($c.socket);
    net.close($listener);
}

# A frame over the negotiated frame-max is kind "limit": with frame-max 4096 a
# payload may hold 4088 bytes, and the broker's 5000-byte Declare-Ok is refused.
func testFrameOverNegotiatedMaxIsALimit() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        def conn as net.Conn init net.accept($listener);
        net.setReadDeadline($conn, 5000);
        def h as bytes init net.readN($conn, 7);
        net.readN($conn, readLong($h, 3) + 1);
        writeMethod($conn, 1, CLS_EXCHANGE, EX_DECLAREOK, binary.make(4996, 0));
        net.close($conn);
    };
    def c as Conn init newConn(net.connect(net.address($listener)), 4096);
    def kind as string init "";
    try {
        declareExchange($c, "logs", "fanout");
    } catch (err) {
        $kind = $err.kind;
    }
    testing.assertEqual($kind, "limit");
    task.wait($server);
    net.close($c.socket);
    net.close($listener);
    checkFrameSize(4088, 4096);
    checkFrameSize(5000, 0);
}
