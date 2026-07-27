# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
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

# ---- read cap (DoS from an attacker-declared packet length) ----
func testPacketCapRejectsOversized() {
    testing.assertThrows("overPacketCap", "mqtt");
}
func overPacketCap() { capPacket(MAX_PACKET_BYTES + 1); }
func testPacketCapAllowsAtLimit() {
    capPacket(MAX_PACKET_BYTES);
    testing.assertTrue(true);
}

# ---- QoS-1 PUBLISH framing (packet id + DUP / retain flags) ----
func testBuildPublishQosZeroNoFlags() {
    # QoS 0, no dup / retain: header 0x30, topic "a", payload "hi".
    def got as bytes init buildPublish("a", bytesOf([104, 105]), 0, 0, false, false);
    testing.assertEqual($got, bytesOf([48, 5, 0, 1, 97, 104, 105]));
}
func testBuildPublishQosOneWithPacketIdAndFlags() {
    # QoS 1 + DUP + retain: header 0x30|0x08|0x02|0x01 = 0x3B (59); the 2-byte
    # packet id (5) follows the topic; payload "hi".
    def got as bytes init buildPublish("a", bytesOf([104, 105]), 1, 5, true, true);
    testing.assertEqual($got, bytesOf([59, 7, 0, 1, 97, 0, 5, 104, 105]));
}
func testBuildPublishRetainOnlyFlag() {
    # QoS 0, retain set (no DUP, no packet id): header 0x30|0x01 = 0x31 (49).
    def got as bytes init buildPublish("a", bytesOf([104, 105]), 0, 0, false, true);
    testing.assertEqual($got, bytesOf([49, 5, 0, 1, 97, 104, 105]));
}
func testPublishPacketIdExtraction() {
    # QoS 1 PUBLISH body: topic "a" + packet id 5 + payload "hi".
    def q1 as Packet init Packet{ typ: 3, flags: 0x02, body: bytesOf([0, 1, 97, 0, 5, 104, 105]) };
    testing.assertEqual(publishPacketId($q1), 5);
    # QoS 0 PUBLISH carries no packet id: -1.
    def q0 as Packet init Packet{ typ: 3, flags: 0x00, body: bytesOf([0, 1, 97, 104, 105]) };
    testing.assertEqual(publishPacketId($q0), -1);
}

# ---- PUBACK build / parse ----
func testBuildPuback() {
    testing.assertEqual(buildPuback(5), bytesOf([64, 2, 0, 5]));
    # 258 = 0x0102 exercises the big-endian split.
    testing.assertEqual(buildPuback(258), bytesOf([64, 2, 1, 2]));
}
func testParsePuback() {
    def a as Packet init Packet{ typ: 4, flags: 0, body: bytesOf([0, 5]) };
    testing.assertEqual(parsePuback($a), 5);
    def b as Packet init Packet{ typ: 4, flags: 0, body: bytesOf([1, 2]) };
    testing.assertEqual(parsePuback($b), 258);
}
func testPubackRoundTrip() {
    # A built PUBACK's body (after the [type][remlen] fixed header) parses back
    # to the same packet id.
    def full as bytes init buildPuback(4242);
    def body as bytes init bytesOf([$full[2], $full[3]]);
    def pkt as Packet init Packet{ typ: 4, flags: 0, body: $body };
    testing.assertEqual(parsePuback($pkt), 4242);
}

# ---- CONNECT with Last-Will + clean-session flags ----
func testBuildConnectFullWithWill() {
    def opts as Options init Options{ host: "h", port: 1883, clientId: "a", keepalive: 60, security: "none", username: "", password: "" };
    def will as Will init Will{ topic: "w", payload: bytesOf([98, 121, 101]), qos: 1, retain: true };
    # clean-session false, will flag 0x04, will-QoS 1 (0x08), will-retain 0x20
    # => flags 0x2C (44). Payload: clientId "a", will topic "w", will payload "bye".
    def want as bytes init bytesOf([16, 21, 0, 4, 77, 81, 84, 84, 4, 44, 0, 60, 0, 1, 97, 0, 1, 119, 0, 3, 98, 121, 101]);
    testing.assertEqual(buildConnectFull($opts, $will, false), $want);
}
func testBuildConnectFullCleanSessionFlag() {
    def opts as Options init Options{ host: "h", port: 1883, clientId: "a", keepalive: 0, security: "none", username: "", password: "" };
    def none as Will init Will{ topic: "", payload: bytesOf([]), qos: 0, retain: false };
    # No will, clean-session true => flags byte (offset 9) is 0x02.
    testing.assertEqual(buildConnectFull($opts, $none, true)[9], 0x02);
    # No will, clean-session false => flags byte is 0x00.
    testing.assertEqual(buildConnectFull($opts, $none, false)[9], 0x00);
}
func testBuildConnectFullWillFlagsCombine() {
    def opts as Options init Options{ host: "h", port: 1883, clientId: "a", keepalive: 0, security: "none", username: "", password: "" };
    def will as Will init Will{ topic: "w", payload: bytesOf([1]), qos: 1, retain: true };
    # clean-session true (0x02) + will (0x04) + will-QoS 1 (0x08) + retain (0x20)
    # => 0x2E (46).
    testing.assertEqual(buildConnectFull($opts, $will, true)[9], 0x2E);
}

# ---- SUBACK granted-QoS parse ----
func testParseSubackQosGranted() {
    def s0 as Packet init Packet{ typ: 9, flags: 0, body: bytesOf([0, 1, 0]) };
    testing.assertEqual(parseSubackQos($s0), 0);
    def s1 as Packet init Packet{ typ: 9, flags: 0, body: bytesOf([0, 1, 1]) };
    testing.assertEqual(parseSubackQos($s1), 1);
    # 0x80 signals the broker rejected the subscription.
    def fail as Packet init Packet{ typ: 9, flags: 0, body: bytesOf([0, 1, 128]) };
    testing.assertEqual(parseSubackQos($fail), 128);
}
