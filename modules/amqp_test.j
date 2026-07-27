# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
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
use testing;
use encoding;

# hex renders a bytes value as lowercase hex.
func hex(b as bytes) {
    return encoding.toText($b, "hex");
}

# fromHex builds a bytes value from a hex string.
func fromHex(s as string) {
    return encoding.fromText($s, "hex");
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
    testing.assertEqual(hex($t),
        "000000180c782d71756575652d74797065530000000671756f72756d");
    # An empty map encodes as an empty table.
    def m as map of string to string init {};
    def empty as bytes;
    testing.assertEqual(hex(putStringTable($empty, $m)), "00000000");
}

func testShortStrTruncatedNul() {
    # SASL PLAIN response NUL user NUL pass -> 00 75 00 70
    def raw as bytes init convert.bytesFromString(saslPlain("u", "p"), "utf-8");
    testing.assertEqual(hex($raw), "00750070");
}

func testReadIntegers() {
    testing.assertEqual(readShort(fromHex("1234"), 0), 0x1234);
    testing.assertEqual(readLong(fromHex("01020304"), 0), 0x01020304);
    testing.assertEqual(readLongLong(fromHex("0000000000000102"), 0), 258);
    # offset read
    testing.assertEqual(readShort(fromHex("aaaa1234"), 2), 0x1234);
}

func testReadShortStr() {
    testing.assertEqual(readShortStr(fromHex("026869"), 0), "hi");
    testing.assertEqual(readShortStr(fromHex("ffff026869"), 2), "hi");
}

func testByteLen() {
    testing.assertEqual(byteLen("hi"), 2);
    testing.assertEqual(byteLen(""), 0);
    # a 2-byte UTF-8 character
    testing.assertEqual(byteLen(convert.stringFromBytes(fromHex("c3a9"), "utf-8")), 2);
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
func overFrameCap() { checkFrameSize(MAX_FRAME_BYTES + 1); }
func negFrameCap() { checkFrameSize(-1); }
func testFrameSizeCap() {
    testing.assertThrows("overFrameCap", "amqp");
    testing.assertThrows("negFrameCap", "amqp");
    checkFrameSize(MAX_FRAME_BYTES);   # exactly at the limit does not throw
    checkFrameSize(0);
}

# --- new method encoders / decoders -----------------------------------------

# encodeExchangeDeclare: reserved(0) | "logs" | "fanout" | durable bit | empty table.
func testEncodeExchangeDeclare() {
    testing.assertEqual(hex(encodeExchangeDeclare("logs", "fanout", true)),
        "0000046c6f67730666616e6f75740200000000");
    # non-durable clears the flag octet.
    testing.assertEqual(hex(encodeExchangeDeclare("logs", "fanout", false)),
        "0000046c6f67730666616e6f75740000000000");
}

# encodeQueueBind: reserved(0) | "jobs" | "logs" | "rk" | no-wait(0) | empty table.
func testEncodeQueueBind() {
    testing.assertEqual(hex(encodeQueueBind("jobs", "logs", "rk")),
        "0000046a6f6273046c6f677302726b0000000000");
}

# encodeBasicConsume: reserved(0) | "jobs" | "ctag" | flags | empty table.
func testEncodeBasicConsume() {
    # autoAck false -> flags octet 0x00.
    testing.assertEqual(hex(encodeBasicConsume("jobs", "ctag", false)),
        "0000046a6f627304637461670000000000");
    # autoAck true -> no-ack bit (0x02).
    testing.assertEqual(hex(encodeBasicConsume("jobs", "ctag", true)),
        "0000046a6f627304637461670200000000");
    # An empty consumer tag asks the broker to name it (single 0x00 length byte).
    testing.assertEqual(hex(encodeBasicConsume("q", "", false)),
        "00000171000000000000");
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
    def a as Properties init Properties{ contentType: "text/plain", persistent: true, correlationId: "", replyTo: "" };
    testing.assertEqual(hex(encodeProperties($a)),
        "90000a746578742f706c61696e02");
    # correlation-id + reply-to -> flags 0x0600, "abc", "q1" (correlation before reply).
    def b as Properties init Properties{ contentType: "", persistent: false, correlationId: "abc", replyTo: "q1" };
    testing.assertEqual(hex(encodeProperties($b)),
        "060003616263027131");
}

# decodeDeliverMethod parses Basic.Deliver args into a Delivery (empty body).
func testDecodeDeliverMethod() {
    def args as bytes;
    $args = putShortStr($args, "ctag");
    $args = putLongLong($args, 42);
    $args = putOctet($args, 1);          # redelivered
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
    $args = putShortStr($args, "");      # broker-named consumer
    $args = putLongLong($args, 9);
    $args = putOctet($args, 0);          # not redelivered
    $args = putShortStr($args, "");      # default exchange
    $args = putShortStr($args, "jobs");
    # content header: class | weight | body-size | property-flags.
    def hdr as bytes;
    $hdr = putShort($hdr, CLS_BASIC);
    $hdr = putShort($hdr, 0);
    $hdr = putLongLong($hdr, 5);
    $hdr = putShort($hdr, 0);
    testing.assertEqual(decodeContentBodySize($hdr), 5);
    def body as bytes init convert.bytesFromString("hello", "utf-8");
    def d as Delivery init deliveryFrom($args, $body);
    testing.assertEqual($d.deliveryTag, 9);
    testing.assertEqual($d.redelivered, false);
    testing.assertEqual($d.exchange, "");
    testing.assertEqual($d.routingKey, "jobs");
    testing.assertEqual(hex($d.body), hex($body));
}
