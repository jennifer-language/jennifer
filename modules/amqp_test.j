# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
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
