# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# mqtt_test.j - white-box tests for mqtt.j's pure packet-building helpers. Run:
#
#     jennifer test modules/mqtt_test.j
#
# The overlay splices mqtt.j in front of this file, so the tests reach its
# private encoders / decoders (encodeRemLen, decodeRemLen, putString, frame,
# buildConnect, parsePublish) by bare identifier. The networked connect /
# publish / subscribe / poll round-trip runs against an in-process MQTT-broker
# fake in the Go suite (TestMqttPubSub); the tests at the end run small spawned
# brokers here for the packets that arrive while a call waits for an ack.
use encoding;
use testing;
use task;
use channel;

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

# A malformed remaining-length with a 5th continuation byte (all high bits set)
# is rejected typed, not looped / overflowed.
func testRemLenTooLongRejected() {
    def bad as bytes init bytesOf([0xff, 0xff, 0xff, 0xff, 0xff]);
    def threw as bool init false;
    def kind as string init "";
    try {
        decodeRemLen($bad, 0);
    } catch (e) {
        $threw = true;
        $kind = $e.kind;
    }
    testing.assertTrue($threw);
    testing.assertEqual($kind, "mqtt");
}

func testPutStringPrefix() {
    def b as bytes;
    testing.assertEqual(putString($b, "MQTT"), bytesOf([0, 4, 77, 81, 84, 84]));
}

func testBuildConnectBytes() {
    def opts as Options init Options{
        host: "h",
        port: 1883,
        clientId: "a",
        keepalive: 60,
        security: transport.Security.None,
        username: "",
        password: ""
    };
    # [type 0x10][remlen 13][vh: "MQTT" level 4 flags 0x02 keepalive 60][pl: "a"]
    def want as bytes init bytesOf([16, 13, 0, 4, 77, 81, 84, 84, 4, 2, 0, 60, 0, 1, 97]);
    testing.assertEqual(buildConnect($opts), $want);
}

func testBuildConnectSetsCredentialFlags() {
    def opts as Options init Options{
        host: "h",
        port: 1883,
        clientId: "a",
        keepalive: 0,
        security: transport.Security.None,
        username: "u",
        password: "p"
    };
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
    def pkt as Packet init Packet{typ: 3, flags: 0, body: $body};
    def m as Message init parsePublish($pkt);
    testing.assertEqual($m.topic, "t/x");
    testing.assertEqual(encoding.decode("utf-8", $m.payload), "hi");
}

func testParsePublishSkipsPacketIdAtQosOne() {
    # QoS 1 PUBLISH: flags 0x02, body = topic "a" + packet-id (2 bytes) + "hi"
    def body as bytes init bytesOf([0, 1, 97, 0, 5, 104, 105]);
    def pkt as Packet init Packet{typ: 3, flags: 0x02, body: $body};
    def m as Message init parsePublish($pkt);
    testing.assertEqual($m.topic, "a");
    testing.assertEqual(encoding.decode("utf-8", $m.payload), "hi");
}

# ---- read cap (DoS from an attacker-declared packet length) ----
func testPacketCapRejectsOversized() {
    testing.assertThrows("overPacketCap", "limit");
}
func overPacketCap() {
    capPacket(MAX_PACKET_BYTES + 1);
}
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
    def q1 as Packet init Packet{typ: 3, flags: 0x02, body: bytesOf([0, 1, 97, 0, 5, 104, 105])};
    testing.assertEqual(publishPacketId($q1), 5);
    # QoS 0 PUBLISH carries no packet id: -1.
    def q0 as Packet init Packet{typ: 3, flags: 0x00, body: bytesOf([0, 1, 97, 104, 105])};
    testing.assertEqual(publishPacketId($q0), -1);
    # A packet id above 255 reads its high byte too.
    def big as Packet init Packet{typ: 3, flags: 0x02, body: bytesOf([0, 1, 97, 0xAB, 0xCD])};
    testing.assertEqual(publishPacketId($big), 0xABCD);
}

# ---- PUBACK build / parse ----
func testBuildPuback() {
    testing.assertEqual(buildPuback(5), bytesOf([64, 2, 0, 5]));
    # 258 = 0x0102 exercises the big-endian split.
    testing.assertEqual(buildPuback(258), bytesOf([64, 2, 1, 2]));
}
func testParsePuback() {
    def a as Packet init Packet{typ: 4, flags: 0, body: bytesOf([0, 5])};
    testing.assertEqual(parsePuback($a), 5);
    def b as Packet init Packet{typ: 4, flags: 0, body: bytesOf([1, 2])};
    testing.assertEqual(parsePuback($b), 258);
    def c as Packet init Packet{typ: 4, flags: 0, body: bytesOf([0xAB, 0xCD])};
    testing.assertEqual(parsePuback($c), 0xABCD);
}
func testPubackRoundTrip() {
    # A built PUBACK's body (after the [type][remlen] fixed header) parses back
    # to the same packet id.
    def full as bytes init buildPuback(4242);
    def body as bytes init bytesOf([$full[2], $full[3]]);
    def pkt as Packet init Packet{typ: 4, flags: 0, body: $body};
    testing.assertEqual(parsePuback($pkt), 4242);
}

# ---- CONNECT with Last-Will + clean-session flags ----
func testBuildConnectFullWithWill() {
    def opts as Options init Options{
        host: "h",
        port: 1883,
        clientId: "a",
        keepalive: 60,
        security: transport.Security.None,
        username: "",
        password: ""
    };
    def will as Will init Will{topic: "w", payload: bytesOf([98, 121, 101]), qos: 1, retain: true};
    # clean-session false, will flag 0x04, will-QoS 1 (0x08), will-retain 0x20
    # => flags 0x2C (44). Payload: clientId "a", will topic "w", will payload "bye".
    def want as bytes init bytesOf([
        16,
        21,
        0,
        4,
        77,
        81,
        84,
        84,
        4,
        44,
        0,
        60,
        0,
        1,
        97,
        0,
        1,
        119,
        0,
        3,
        98,
        121,
        101
    ]);
    testing.assertEqual(buildConnectFull($opts, $will, false), $want);
}
func testBuildConnectFullCleanSessionFlag() {
    def opts as Options init Options{
        host: "h",
        port: 1883,
        clientId: "a",
        keepalive: 0,
        security: transport.Security.None,
        username: "",
        password: ""
    };
    def none as Will init Will{topic: "", payload: bytesOf([]), qos: 0, retain: false};
    # No will, clean-session true => flags byte (offset 9) is 0x02.
    testing.assertEqual(buildConnectFull($opts, $none, true)[9], 0x02);
    # No will, clean-session false => flags byte is 0x00.
    testing.assertEqual(buildConnectFull($opts, $none, false)[9], 0x00);
}
func testBuildConnectFullWillFlagsCombine() {
    def opts as Options init Options{
        host: "h",
        port: 1883,
        clientId: "a",
        keepalive: 0,
        security: transport.Security.None,
        username: "",
        password: ""
    };
    def will as Will init Will{topic: "w", payload: bytesOf([1]), qos: 1, retain: true};
    # clean-session true (0x02) + will (0x04) + will-QoS 1 (0x08) + retain (0x20)
    # => 0x2E (46).
    testing.assertEqual(buildConnectFull($opts, $will, true)[9], 0x2E);
}

# ---- SUBACK granted-QoS parse ----
func testParseSubackQosGranted() {
    def s0 as Packet init Packet{typ: 9, flags: 0, body: bytesOf([0, 1, 0])};
    testing.assertEqual(parseSubackQos($s0), 0);
    def s1 as Packet init Packet{typ: 9, flags: 0, body: bytesOf([0, 1, 1])};
    testing.assertEqual(parseSubackQos($s1), 1);
    # 0x80 signals the broker rejected the subscription.
    def fail as Packet init Packet{typ: 9, flags: 0, body: bytesOf([0, 1, 128])};
    testing.assertEqual(parseSubackQos($fail), 128);
}

func publishWildcard() {
    def e as bytes;
    return buildPublish("a/+/b", $e, 0, 0, false, false);
}
func longTopic() {
    def e as bytes;
    return buildPublish(strings.replicate("t", 70000), $e, 0, 0, false, false);
}
# A publish topic with a wildcard, or a field too long for its 2-byte length
# prefix, is refused instead of desyncing the stream.
func testPublishTopicGuards() {
    testing.assertThrows("publishWildcard", "mqtt");
    testing.assertThrows("longTopic", "mqtt");
}

# A refused CONNACK is an MqttError with the return code, named in the message;
# any other failure has code 0.
func testARefusedConnackIsAnMqttError() {
    def e as MqttError init connackRefusal(5);
    testing.assertEqual($e.kind, "mqtt");
    testing.assertEqual($e.connackCode, 5);
    testing.assertEqual($e.message, "mqtt: connection refused, CONNACK code 5 (not authorized)");
    testing.assertEqual(
        connackRefusal(4).message,
        "mqtt: connection refused, CONNACK code 4 (bad user name or password)");
    testing.assertEqual(connackRefusal(9).message, "mqtt: connection refused, CONNACK code 9");
    try {
        throw mqttError("mqtt: malformed remaining-length (over 4 bytes)");
    } catch (err) {
        testing.assertEqual($err.kind, "mqtt");
        testing.assertEqual($err.connackCode, 0);
        testing.assertEqual($err.topic, "");
    }
}

# A broker that closes the connection mid-packet is kind "closed", passed on
# as net threw it, not re-tagged as an MqttError.
func testAPeerCloseMidPacketIsClosed() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        def conn as net.Conn init net.accept($listener);
        net.writeBytes($conn, encoding.encode("utf-8", "AB"));
        net.close($conn);
    };
    def c as net.Conn init net.connect(net.address($listener));
    try {
        readN($c, 8);
        throw Error{kind: "missed", message: "a close mid-packet"};
    } catch (e) {
        testing.assertEqual($e.kind, "closed");
    }
    task.wait($server);
    net.close($c);
    net.close($listener);
}

# byteList builds bytes from a list of byte values.
func byteList(values as list of int) {
    def out as bytes;
    for (def v in $values) {
        $out[] = $v;
    }
    return $out;
}

# A broker packet too short for its fields is an MqttError, not a runtime
# slice / read error: a topic length past the body, a missing packet id, a
# QoS of 3, and short PUBACK / SUBACK bodies.
func shortTopic() {
    parsePublish(Packet{typ: 3, flags: 0, body: byteList([0, 50, 0x61])});
}
func missingPacketId() {
    publishPacketId(Packet{typ: 3, flags: 2, body: byteList([0, 1, 0x61])});
}
func qosThree() {
    parsePublish(Packet{typ: 3, flags: 6, body: byteList([0, 1, 0x61, 0, 1])});
}
func emptyPublish() {
    parsePublish(Packet{typ: 3, flags: 0, body: binary.make(0, 0)});
}
func shortPuback() {
    parsePuback(Packet{typ: 4, flags: 0, body: byteList([1])});
}
func shortSuback() {
    parseSubackQos(Packet{typ: 9, flags: 0, body: byteList([0, 1])});
}
func testMalformedPacketsAreMqttErrors() {
    testing.assertThrows("shortTopic", "mqtt");
    testing.assertThrows("missingPacketId", "mqtt");
    testing.assertThrows("qosThree", "mqtt");
    testing.assertThrows("emptyPublish", "mqtt");
    testing.assertThrows("shortPuback", "mqtt");
    testing.assertThrows("shortSuback", "mqtt");
    # a well-formed QoS-1 PUBLISH still parses
    def ok as Packet init Packet{typ: 3, flags: 2, body: byteList([0, 1, 0x61, 0, 7, 0x7a])};
    testing.assertEqual(publishPacketId($ok), 7);
    testing.assertEqual(parsePublish($ok).topic, "a");
}

# A remaining length that keeps its continuation bit past four bytes is
# refused after the fifth byte, not read for as long as the broker sends.
func testEndlessRemainingLengthIsRefused() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        def conn as net.Conn init net.accept($listener);
        net.writeBytes($conn, binary.make(4096, 0xff));
        net.close($conn);
    };
    def c as net.Conn init net.connect(net.address($listener));
    try {
        readRemLen($c);
        throw Error{kind: "missed", message: "an endless remaining length"};
    } catch (e) {
        testing.assertEqual($e.kind, "mqtt");
    }
    task.wait($server);
    net.close($c);
    net.close($listener);
}

# ---- PUBLISH packets that arrive while a call waits for an ack ----

# fakeOptions points a client at a fake broker's listener.
func fakeOptions(listener as net.Listener) {
    def addr as string init net.address($listener);
    def parts as list of string init strings.split($addr, ":");
    return Options{
        host: "127.0.0.1",
        port: convert.toInt($parts[len($parts) - 1]),
        clientId: "t",
        keepalive: 0,
        security: transport.Security.None,
        username: "",
        password: ""
    };
}

# brokerRead reads the next client packet and checks its type.
func brokerRead(conn as net.Conn, typ as int) {
    def h as bytes init readN($conn, 1);
    def pkt as Packet init readPacketBody($conn, $h[0]);
    testing.assertEqual($pkt.typ, $typ);
    return $pkt;
}

# brokerConnack answers the client's CONNECT.
func brokerConnack(conn as net.Conn) {
    brokerRead($conn, 1);
    net.writeBytes($conn, bytesOf([0x20, 2, 0, 0]));
    return null;
}

# A broker that sends a PUBLISH (and a PINGRESP) before the SUBACK, as one does
# for a client that already has subscriptions: subscribe queues the message
# instead of throwing "expected SUBACK", and receive returns it before the one
# that follows on the socket.
func brokerPublishBeforeSuback(listener as net.Listener) {
    def conn as net.Conn init net.accept($listener);
    defer net.close($conn);
    brokerConnack($conn);
    brokerRead($conn, 8);
    net.writeBytes(
        $conn,
        buildPublish("a", encoding.encode("utf-8", "early"), 0, 0, false, false));
    net.writeBytes($conn, bytesOf([0xd0, 0]));
    net.writeBytes($conn, bytesOf([0x90, 3, 0, 1, 0]));
    net.writeBytes(
        $conn,
        buildPublish("a", encoding.encode("utf-8", "late"), 0, 0, false, false));
    brokerRead($conn, 14);
    return null;
}
func testSubscribeKeepsAnEarlierPublish() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        return brokerPublishBeforeSuback($listener);
    };
    def c as Client init connect(fakeOptions($listener));
    $c = subscribe($c, "b");
    testing.assertEqual(len($c.subs), 1);
    def first as Message init receive($c);
    testing.assertEqual($first.topic, "a");
    testing.assertEqual(encoding.decode("utf-8", $first.payload), "early");
    def second as Message init receive($c);
    testing.assertEqual(encoding.decode("utf-8", $second.payload), "late");
    disconnect($c);
    task.wait($server);
    net.close($listener);
}

# A broker that delivers a QoS-1 PUBLISH before the PUBACK of the client's own
# QoS-1 publish: publishQos1 acknowledges and queues it, poll returns it.
# Returns the packet id the client acknowledged.
func brokerPublishBeforePuback(listener as net.Listener) {
    def conn as net.Conn init net.accept($listener);
    defer net.close($conn);
    brokerConnack($conn);
    brokerRead($conn, 3);
    net.writeBytes(
        $conn,
        buildPublish("cmd", encoding.encode("utf-8", "mid"), 1, 9, false, false));
    net.writeBytes($conn, buildPuback(QOS1_PACKET_ID));
    def ack as Packet init brokerRead($conn, 4);
    brokerRead($conn, 14);
    return parsePuback($ack);
}
func testPublishQos1KeepsAPublishBeforeItsPuback() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of int init spawn {
        return brokerPublishBeforePuback($listener);
    };
    def c as Client init connect(fakeOptions($listener));
    publishQos1($c, "cmd", encoding.encode("utf-8", "go"), false);
    def got as list of Message init poll($c, 1000);
    testing.assertEqual(len($got), 1);
    testing.assertEqual($got[0].topic, "cmd");
    testing.assertEqual(encoding.decode("utf-8", $got[0].payload), "mid");
    testing.assertEqual(len(poll($c, 50)), 0);
    disconnect($c);
    testing.assertEqual(task.wait($server), 9);
    net.close($listener);
}

# Messages still queued when the connection drops move to the reconnected
# client, ahead of anything the re-subscribe queues.
func brokerTwoSessions(listener as net.Listener) {
    def first as net.Conn init net.accept($listener);
    brokerConnack($first);
    brokerRead($first, 8);
    net.writeBytes(
        $first,
        buildPublish("a", encoding.encode("utf-8", "one"), 0, 0, false, false));
    net.writeBytes($first, bytesOf([0x90, 3, 0, 1, 0]));
    net.close($first);
    def second as net.Conn init net.accept($listener);
    defer net.close($second);
    brokerConnack($second);
    brokerRead($second, 8);
    net.writeBytes(
        $second,
        buildPublish("a", encoding.encode("utf-8", "two"), 0, 0, false, false));
    net.writeBytes($second, bytesOf([0x90, 3, 0, 1, 0]));
    brokerRead($second, 14);
    return null;
}
func testReconnectKeepsQueuedMessages() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        return brokerTwoSessions($listener);
    };
    def c as Client init connect(fakeOptions($listener));
    $c = subscribe($c, "a");
    $c = reconnect($c);
    testing.assertEqual(encoding.decode("utf-8", receive($c).payload), "one");
    testing.assertEqual(encoding.decode("utf-8", receive($c).payload), "two");
    disconnect($c);
    task.wait($server);
    net.close($listener);
}

# A full queue throws kind "limit" before acknowledging the message.
func overfullQueue() {
    def pending as channel of Message init channel.make(MAX_PENDING);
    def c as Client;
    $c.pending = $pending;
    def pkt as Packet init Packet{typ: 3, flags: 0, body: bytesOf([0, 1, 97])};
    def i as int init 0;
    while ($i <= MAX_PENDING) {
        queuePublish($c, $pkt);
        $i = $i + 1;
    }
}
func testAFullQueueIsALimit() {
    testing.assertThrows("overfullQueue", "limit");
}
