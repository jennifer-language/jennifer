# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# mqtt_test.j - white-box tests for mqtt.j's pure packet-building helpers. Run:
#
#     jennifer test modules/mqtt_test.j
#
# The overlay splices mqtt.j in front of this file, so the tests reach its
# private encoders / decoders (encodeRemLen, decodeRemLen, putString, frame,
# buildConnect, parsePublish) by bare identifier. The networked connect /
# publish / subscribe / poll round-trip runs against an in-process MQTT-broker
# fake in the Go suite (TestMqttPubSub).
use testing;

# bytesOf builds a bytes value from a list of int byte values (test convenience).
func bytesOf(xs as list of int) {
    def b as bytes;
    for (def x in $xs) {
        $b[] = $x;
    }
    return $b;
}

func testEncodeRemLenBoundaries() {
    testing.assertEqual(encodeRemLen(0), bytesOf([0]));
    testing.assertEqual(encodeRemLen(127), bytesOf([127]));
    testing.assertEqual(encodeRemLen(128), bytesOf([0x80, 1]));
    testing.assertEqual(encodeRemLen(16383), bytesOf([0xff, 0x7f]));
    testing.assertEqual(encodeRemLen(16384), bytesOf([0x80, 0x80, 1]));
}

func testRemLenRoundTrip() {
    def cases as list of int init [0, 1, 127, 128, 8192, 16383, 16384, 2097151];
    for (def n in $cases) {
        def enc as bytes init encodeRemLen($n);
        def dec as DecodedLen init decodeRemLen($enc, 0);
        testing.assertEqual($dec.value, $n);
        testing.assertEqual($dec.size, len($enc));
    }
}

func testPutStringPrefix() {
    def b as bytes;
    testing.assertEqual(putString($b, "MQTT"), bytesOf([0, 4, 77, 81, 84, 84]));
}

func testBuildConnectBytes() {
    def opts as Options init Options{ host: "h", port: 1883, clientId: "a", keepalive: 60, security: "none", username: "", password: "" };
    # [type 0x10][remlen 13][vh: "MQTT" level 4 flags 0x02 keepalive 60][pl: "a"]
    def want as bytes init bytesOf([16, 13, 0, 4, 77, 81, 84, 84, 4, 2, 0, 60, 0, 1, 97]);
    testing.assertEqual(buildConnect($opts), $want);
}

func testBuildConnectSetsCredentialFlags() {
    def opts as Options init Options{ host: "h", port: 1883, clientId: "a", keepalive: 0, security: "none", username: "u", password: "p" };
    def packet as bytes init buildConnect($opts);
    # flags byte sits after the 6-byte "MQTT" string and the 1-byte level, at
    # offset 2 (header+remlen) + 6 + 1 = 9: clean-session|username|password.
    testing.assertEqual($packet[9], 0x02 | 0x80 | 0x40);
}

func testFrameAssembly() {
    def vh as bytes;
    $vh = putString($vh, "a");
    def frame as bytes init frame(0x30, $vh, bytesOf([1, 2]));
    testing.assertEqual($frame, bytesOf([48, 5, 0, 1, 97, 1, 2]));
}

func testParsePublishQosZero() {
    # body = topic "t/x" (length-prefixed) + payload "hi"
    def body as bytes init bytesOf([0, 3, 116, 47, 120, 104, 105]);
    def pkt as Packet init Packet{ typ: 3, flags: 0, body: $body };
    def m as Message init parsePublish($pkt);
    testing.assertEqual($m.topic, "t/x");
    testing.assertEqual(convert.stringFromBytes($m.payload, "utf-8"), "hi");
}

func testParsePublishSkipsPacketIdAtQosOne() {
    # QoS 1 PUBLISH: flags 0x02, body = topic "a" + packet-id (2 bytes) + "hi"
    def body as bytes init bytesOf([0, 1, 97, 0, 5, 104, 105]);
    def pkt as Packet init Packet{ typ: 3, flags: 0x02, body: $body };
    def m as Message init parsePublish($pkt);
    testing.assertEqual($m.topic, "a");
    testing.assertEqual(convert.stringFromBytes($m.payload, "utf-8"), "hi");
}
