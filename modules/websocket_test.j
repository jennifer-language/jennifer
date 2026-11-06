# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# websocket_test.j - white-box tests for websocket.j. Run with:
#
#     jennifer test modules/websocket_test.j
#
# These exercise the pure handshake-accept computation, URL parsing, and frame
# encoding / masking with no network; the live handshake + send / receive round
# trip is driven against a minimal WebSocket server in the Go suite
# (cmd/jennifer/websocket_test.go). websocket.j already `use`s net / strings /
# convert / hash / encoding / math, so the overlay only adds testing. The
# handshake options and the frame checks run against a raw `net` server below.
use testing;
use task;

# hex renders a bytes value as lowercase hex for byte-exact assertions.
func hex(b as bytes) {
    return encoding.toText($b, "hex");
}

func testAcceptRfcVector() {
    # The canonical RFC 6455 section 1.3 example.
    testing.assertEqual(acceptFor("dGhlIHNhbXBsZSBub25jZQ=="), "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=");
}

func testParseUrlPlain() {
    def t as Target init parseUrl("ws://example.com/chat");
    testing.assertTrue(not $t.secure);
    testing.assertEqual($t.host, "example.com");
    testing.assertEqual($t.port, 80);
    testing.assertEqual($t.path, "/chat");
}

func testParseUrlSecureWithPort() {
    def t as Target init parseUrl("wss://host.local:8443/a/b");
    testing.assertTrue($t.secure);
    testing.assertEqual($t.host, "host.local");
    testing.assertEqual($t.port, 8443);
    testing.assertEqual($t.path, "/a/b");
}

func testParseUrlNoPathDefaults() {
    def t as Target init parseUrl("ws://h:9001");
    testing.assertEqual($t.port, 9001);
    testing.assertEqual($t.path, "/");
}

func testEncodeSmallTextFrame() {
    # "Hi" masked with 01 02 03 04: FIN|text=0x81, MASK|len2=0x82, mask, then
    # 0x48^01=0x49, 0x69^02=0x6b.
    def mask as bytes init encoding.fromText("01020304", "hex");
    def frame as bytes init encodeFrameMasked(
        OP_TEXT,
        convert.bytesFromString("Hi", "utf-8"),
        $mask);
    testing.assertEqual(hex($frame), "818201020304496b");
}

func testEncodeMediumFrameLength() {
    # A 200-byte payload uses the 16-bit length form: 0x80|126, then 0x00 0xC8.
    def payload as bytes;
    def i as int init 0;
    while ($i < 200) {
        $payload[] = 65;
        $i = $i + 1;
    }
    def mask as bytes init encoding.fromText("00000000", "hex");
    def frame as bytes init encodeFrameMasked(OP_BINARY, $payload, $mask);
    # header: 82 (FIN|binary), FE (MASK|126), 00 C8 (200), then 4 mask bytes
    testing.assertEqual($frame[0], 0x82);
    testing.assertEqual($frame[1], 0xfe);
    testing.assertEqual($frame[2], 0x00);
    testing.assertEqual($frame[3], 0xc8);
    # zero mask leaves payload unchanged: first data byte after the 4-byte mask
    testing.assertEqual($frame[8], 65);
    testing.assertEqual(len($frame), 2 + 2 + 4 + 200);
}

func testEncodeCloseFrameOpcode() {
    def mask as bytes init encoding.fromText("00000000", "hex");
    def empty as bytes;
    def frame as bytes init encodeFrameMasked(OP_CLOSE, $empty, $mask);
    # 88 = FIN|close, 80 = MASK|len0, then 4 mask bytes
    testing.assertEqual(hex($frame), "888000000000");
}

func testMakeKeyLength() {
    # 16 random bytes -> 24-char base64 (ends with "==").
    def key as string init makeKey();
    testing.assertEqual(len($key), 24);
    testing.assertTrue(strings.endsWith($key, "=="));
}

# ---- CRLF injection in the handshake URL ----

func injectUrl() {
    parseUrl("ws://example.com/path\r\nX-Injected: 1");
}
func testUrlRejectsCrlf() {
    testing.assertThrows("injectUrl", "websocket");
}
func testCleanUrlParsed() {
    def t as Target init parseUrl("ws://example.com/chat");
    testing.assertEqual($t.host, "example.com");
    testing.assertEqual($t.path, "/chat");
}

# --- URL parsing ---

func testParseUrlForms() {
    def v6 as Target init parseUrl("ws://[::1]:8080/x");
    testing.assertEqual($v6.host, "[::1]");
    testing.assertEqual($v6.port, 8080);
    def q as Target init parseUrl("wss://h?token=1#frag");
    testing.assertEqual($q.host, "h");
    testing.assertEqual($q.path, "/?token=1");
    testing.assertEqual(parseUrl("ws://h/p#f").path, "/p");
}

func urlNoHost() {
    parseUrl("ws:///x");
}

func urlBadPort() {
    parseUrl("ws://h:abc/");
}

func urlPortRange() {
    parseUrl("ws://h:70000/");
}

func urlCredentials() {
    parseUrl("ws://user:pw@h/");
}

func urlOpenBracket() {
    parseUrl("ws://[::1/");
}

func urlSpace() {
    parseUrl("ws://h/a b");
}

func testParseUrlRefuses() {
    for (def f in [
        "urlNoHost",
        "urlBadPort",
        "urlPortRange",
        "urlCredentials",
        "urlOpenBracket",
        "urlSpace"
    ]) {
        testing.assertThrows($f, "websocket");
    }
}

# --- option checks ---

func optionsWith(headers as map of string to string, protocols as list of string) {
    def o as Options init defaultOptions();
    $o.headers = $headers;
    $o.protocols = $protocols;
    return $o;
}

func optOwnedHeader() {
    checkOptions(optionsWith({"Sec-WebSocket-Extensions": "permessage-deflate"}, []));
}

func optHostHeader() {
    checkOptions(optionsWith({"HOST": "evil"}, []));
}

func optBadName() {
    checkOptions(optionsWith({"X Key": "v"}, []));
}

func optCrlfValue() {
    checkOptions(optionsWith({"X-Key": "v\r\nX-Injected: 1"}, []));
}

func optBadProtocol() {
    checkOptions(optionsWith({}, ["graphql ws"]));
}

func testCheckOptions() {
    checkOptions(optionsWith(
        {"x-api-key": "k\tv", "Authorization": "Bearer t"},
        ["graphql-transport-ws"]));
    for (def f in [
        "optOwnedHeader",
        "optHostHeader",
        "optBadName",
        "optCrlfValue",
        "optBadProtocol"
    ]) {
        testing.assertThrows($f, "websocket");
    }
    testing.assertTrue(hasToken("keep-alive, Upgrade", "upgrade"));
    testing.assertFalse(hasToken("keep-alive", "upgrade"));
}

# --- a raw server for the handshake and frame checks ---

# serverFrame builds one unmasked frame, as a server sends it.
func serverFrame(first as int, payload as bytes) {
    def f as bytes;
    $f[] = $first;
    def n as int init len($payload);
    if ($n < 126) {
        $f[] = $n;
    } else {
        $f[] = 126;
        $f[] = ($n >> 8) & 0xff;
        $f[] = $n & 0xff;
    }
    return binary.concat($f, $payload);
}

# fakeServer answers one handshake with `respHeaders` after the accept key,
# then sends `frame`, or, when `frame` is empty, the request it got as a text
# message. It then waits for the client to hang up.
func fakeServer(listener as net.Listener, respHeaders as string, frame as bytes) {
    try {
        def srv as net.Conn init net.accept($listener);
        def got as bytes;
        while (not binary.contains($got, convert.bytesFromString("\r\n\r\n", "utf-8"))) {
            def more as bytes init net.readBytes($srv, 4096);
            if (len($more) == 0) {
                net.close($srv);
                return;
            }
            $got = binary.concat($got, $more);
        }
        def req as string init convert.stringFromBytes($got, "utf-8");
        def key as string init headerValue(strings.split($req, "\r\n"), "sec-websocket-key");
        def head as string init "HTTP/1.1 101 Switching Protocols\r\nSec-WebSocket-Accept: " +
            acceptFor($key) + "\r\n" + $respHeaders + "\r\n";
        net.writeBytes($srv, convert.bytesFromString($head, "utf-8"));
        if (len($frame) > 0) {
            net.writeBytes($srv, $frame);
        } else {
            net.writeBytes($srv, serverFrame(0x81, $got));
        }
        net.readBytes($srv, 1);
        net.close($srv);
    } catch (e) { # lint-disable: L103
        # the client hung up first
    }
}

def const UPGRADE as string init "Upgrade: websocket\r\nConnection: Upgrade\r\n";

# scenario connects to a fakeServer and returns the received message's text,
# or, with receiveIt false, the negotiated subprotocol.
func scenario(respHeaders as string, frame as bytes, o as Options, receiveIt as bool) {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        fakeServer($listener, $respHeaders, $frame);
    };
    defer task.wait($server);
    defer net.close($listener);
    def c as Conn init connectOptions("ws://" + net.address($listener) + "/live", $o);
    defer net.close($c.socket);
    if ($receiveIt) {
        return receive($c).text;
    }
    return $c.protocol;
}

func noFrame() {
    def b as bytes;
    return $b;
}

func testHandshakeSendsOptions() {
    def o as Options init optionsWith({"x-api-key": "k1"}, ["graphql-transport-ws", "graphql-ws"]);
    def req as string init scenario(
        UPGRADE + "Sec-WebSocket-Protocol: graphql-transport-ws\r\n",
        noFrame(),
        $o,
        true);
    testing.assertContains($req, "GET /live HTTP/1.1\r\n");
    testing.assertContains($req, "x-api-key: k1\r\n");
    testing.assertContains($req, "Sec-WebSocket-Protocol: graphql-transport-ws, graphql-ws\r\n");
    testing.assertEqual(
        scenario(
            UPGRADE + "Sec-WebSocket-Protocol: graphql-transport-ws\r\n",
            noFrame(),
            $o,
            false),
        "graphql-transport-ws");
    testing.assertEqual(scenario(UPGRADE, noFrame(), $o, false), "");
}

func unofferedProtocol() {
    scenario(
        UPGRADE + "Sec-WebSocket-Protocol: mqtt\r\n",
        noFrame(),
        optionsWith({}, ["graphql-transport-ws"]),
        false);
}

func unofferedExtension() {
    scenario(
        UPGRADE + "Sec-WebSocket-Extensions: permessage-deflate\r\n",
        noFrame(),
        defaultOptions(),
        false);
}

func noUpgradeHeader() {
    scenario("Connection: Upgrade\r\n", noFrame(), defaultOptions(), false);
}

func reservedBits() {
    # RSV1 (compression) set on a text frame
    scenario(
        UPGRADE,
        serverFrame(0xc1, convert.bytesFromString("x", "utf-8")),
        defaultOptions(),
        true);
}

func hugeLength() {
    # a 64-bit length with the top bit set
    scenario(UPGRADE, encoding.fromText("817fff00000000000001", "hex"), defaultOptions(), true);
}

# The 64-bit length form: 127, then 8 bytes. A short payload in that form is
# legal (RFC 6455 asks for minimal encoding but receivers accept it).
func testReceiveSixtyFourBitLength() {
    def frame as bytes init encoding.fromText("817f0000000000000003616263", "hex");
    testing.assertEqual(scenario(UPGRADE, $frame, defaultOptions(), true), "abc");
}

func overCapLength() {
    # high four bytes clear, 2^32 - 1 declared: past the 64 MiB cap
    scenario(UPGRADE, encoding.fromText("817f00000000ffffffff", "hex"), defaultOptions(), true);
}

func testSixtyFourBitLengthOverTheCap() {
    testing.assertThrows("overCapLength", "websocket");
}

func badUtf8() {
    scenario(UPGRADE, serverFrame(0x81, encoding.fromText("c328", "hex")), defaultOptions(), true);
}

func maskedFrame() {
    scenario(UPGRADE, encoding.fromText("818100000000" + "78", "hex"), defaultOptions(), true);
}

func unknownOpcode() {
    scenario(
        UPGRADE,
        serverFrame(0x83, convert.bytesFromString("x", "utf-8")),
        defaultOptions(),
        true);
}

func fragmentedPing() {
    scenario(
        UPGRADE,
        serverFrame(0x09, convert.bytesFromString("x", "utf-8")),
        defaultOptions(),
        true);
}

func longPing() {
    scenario(UPGRADE, serverFrame(0x89, binary.make(126, 0)), defaultOptions(), true);
}

func strayContinuation() {
    scenario(
        UPGRADE,
        serverFrame(0x80, convert.bytesFromString("x", "utf-8")),
        defaultOptions(),
        true);
}

func messageInsideMessage() {
    def two as bytes init binary.concat(
        serverFrame(0x01, convert.bytesFromString("a", "utf-8")),
        serverFrame(0x81, convert.bytesFromString("b", "utf-8")));
    scenario(UPGRADE, $two, defaultOptions(), true);
}

# a ping between two fragments is answered and the message still assembles
func testFragmentsAroundPing() {
    def frames as bytes init binary.join([
        serverFrame(0x01, convert.bytesFromString("he", "utf-8")),
        serverFrame(0x89, convert.bytesFromString("p", "utf-8")),
        serverFrame(0x80, convert.bytesFromString("llo", "utf-8"))
    ]);
    testing.assertEqual(scenario(UPGRADE, $frames, defaultOptions(), true), "hello");
}

func testHandshakeAndFramesRefused() {
    for (def f in [
        "unofferedProtocol",
        "unofferedExtension",
        "noUpgradeHeader",
        "reservedBits",
        "hugeLength",
        "badUtf8",
        "maskedFrame",
        "unknownOpcode",
        "fragmentedPing",
        "longPing",
        "strayContinuation",
        "messageInsideMessage"
    ]) {
        testing.assertThrows($f, "websocket");
    }
}
