# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# websocket_test.j - white-box tests for websocket.j. Run with:
#
#     jennifer test modules/websocket_test.j
#
# These exercise the pure handshake-accept computation, URL parsing, and frame
# encoding / masking with no network; the live handshake + send / receive round
# trip is driven against a minimal WebSocket server in the Go suite
# (cmd/jennifer/websocket_test.go). websocket.j already `use`s net / strings /
# convert / hash / encoding / math, so the overlay only adds testing.
use testing;

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
    def frame as bytes init encodeFrameMasked(OP_TEXT, convert.bytesFromString("Hi", "utf-8"), $mask);
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

func injectUrl() { parseUrl("ws://example.com/path\r\nX-Injected: 1"); }
func testUrlRejectsCrlf() {
    testing.assertThrows("injectUrl", "websocket");
}
func testCleanUrlParsed() {
    def t as Target init parseUrl("ws://example.com/chat");
    testing.assertEqual($t.host, "example.com");
    testing.assertEqual($t.path, "/chat");
}
