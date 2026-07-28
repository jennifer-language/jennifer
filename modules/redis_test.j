# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# redis_test.j - white-box tests for redis.j's pure RESP helpers. Run with:
#
#     jennifer test modules/redis_test.j
#
# The overlay splices redis.j in front of this file, so the tests reach its
# private RESP encoder / decoder (encodeCommand, parseComplete) by bare
# identifier. The networked session is verified end to end against an in-process
# RESP server in the Go suite (TestRedisCommands).
use testing;

# The RESP parser frames over bytes; these helpers keep the string-literal
# test inputs readable by converting at the call boundary.
func b(s as string) {
    return convert.bytesFromString($s, "utf-8");
}

# reststr decodes the unconsumed remainder: the parser now returns a byte
# cursor (`pos`), so slice the original input from there.
func reststr(orig as string, pr as ParseResult) {
    def buf as bytes init b($orig);
    return convert.stringFromBytes(byteSlice($buf, $pr.pos, len($buf)), "utf-8");
}

func testEncodeCommand() {
    testing.assertEqual(
        encodeCommand(["SET", "key", "value"]),
        "*3\r\n$3\r\nSET\r\n$3\r\nkey\r\n$5\r\nvalue\r\n");
    testing.assertEqual(encodeCommand(["PING"]), "*1\r\n$4\r\nPING\r\n");
}

func testParseSimpleString() {
    def pr as ParseResult init parseComplete(b("+OK\r\n"));
    testing.assertTrue($pr.complete);
    testing.assertEqual($pr.reply.kind, "string");
    testing.assertEqual($pr.reply.str, "OK");
    testing.assertEqual(reststr("+OK\r\n", $pr), "");
}

func testParseError() {
    def pr as ParseResult init parseComplete(b("-ERR unknown command\r\n"));
    testing.assertEqual($pr.reply.kind, "error");
    testing.assertEqual($pr.reply.str, "ERR unknown command");
}

func testParseInteger() {
    def pr as ParseResult init parseComplete(b(":42\r\n"));
    testing.assertEqual($pr.reply.kind, "int");
    testing.assertEqual($pr.reply.num, 42);
}

func testParseBulkString() {
    def pr as ParseResult init parseComplete(b("$5\r\nhello\r\n"));
    testing.assertEqual($pr.reply.kind, "string");
    testing.assertEqual($pr.reply.str, "hello");
    testing.assertEqual(reststr("$5\r\nhello\r\n", $pr), "");
}

# A bulk string whose byte length exceeds its rune count ("café" is 5 bytes,
# 4 runes) must frame on the byte count. A rune-indexed parser reads the reply
# as incomplete (hang) or slices the CRLF into the payload.
func testParseBulkMultibyte() {
    def pr as ParseResult init parseComplete(b("$5\r\ncafé\r\n"));
    testing.assertTrue($pr.complete);
    testing.assertEqual($pr.reply.str, "café");
    testing.assertEqual(reststr("$5\r\ncafé\r\n", $pr), "");
}

# A trailing reply after a multi-byte bulk still frames cleanly (the byte
# cursor lands exactly on the next reply's type byte).
func testParseBulkMultibyteLeavesRest() {
    def pr as ParseResult init parseComplete(b("$5\r\ncafé\r\n+NEXT\r\n"));
    testing.assertEqual($pr.reply.str, "café");
    testing.assertEqual(reststr("$5\r\ncafé\r\n+NEXT\r\n", $pr), "+NEXT\r\n");
}

func testParseNilBulk() {
    def pr as ParseResult init parseComplete(b("$-1\r\n"));
    testing.assertEqual($pr.reply.kind, "nil");
}

func testParseArray() {
    def pr as ParseResult init parseComplete(b("*2\r\n$3\r\nfoo\r\n$3\r\nbar\r\n"));
    testing.assertEqual($pr.reply.kind, "array");
    testing.assertEqual(len($pr.reply.items), 2);
    testing.assertEqual($pr.reply.items[0].str, "foo");
    testing.assertEqual($pr.reply.items[1].str, "bar");
}

func testParseMixedArray() {
    def pr as ParseResult init parseComplete(b("*2\r\n:1\r\n$3\r\nfoo\r\n"));
    testing.assertEqual($pr.reply.items[0].kind, "int");
    testing.assertEqual($pr.reply.items[0].num, 1);
    testing.assertEqual($pr.reply.items[1].str, "foo");
}

func testParseIncomplete() {
    testing.assertFalse(parseComplete(b("+OK")).complete); # no CRLF yet
    testing.assertFalse(parseComplete(b("$5\r\nhel")).complete); # short bulk
    testing.assertFalse(parseComplete(b("*2\r\n$3\r\nfoo\r\n")).complete); # missing element
}

func testParseLeavesRest() {
    # A reply followed by the start of the next one leaves the remainder.
    def pr as ParseResult init parseComplete(b(":7\r\n+NEXT\r\n"));
    testing.assertEqual($pr.reply.num, 7);
    testing.assertEqual(reststr(":7\r\n+NEXT\r\n", $pr), "+NEXT\r\n");
}

# ---- read cap (DoS from an oversized reply) ----
func testReplyCapRejectsOversized() {
    testing.assertThrows("overReplyCap", "redis");
}
func overReplyCap() {
    capReply(MAX_REPLY_BYTES + 1);
}
func testReplyCapAllowsAtLimit() {
    capReply(MAX_REPLY_BYTES);
    testing.assertTrue(true);
}

# ---- pub/sub command encoding ----
func testEncodeSubscribe() {
    testing.assertEqual(
        encodeCommand(subscribeArgs("SUBSCRIBE", ["news", "weather"])),
        "*3\r\n$9\r\nSUBSCRIBE\r\n$4\r\nnews\r\n$7\r\nweather\r\n");
    testing.assertEqual(
        encodeCommand(subscribeArgs("PSUBSCRIBE", ["news.*"])),
        "*2\r\n$10\r\nPSUBSCRIBE\r\n$6\r\nnews.*\r\n");
}

func testEncodeUnsubscribeAll() {
    # An empty channel list is UNSUBSCRIBE-from-all (just the verb).
    testing.assertEqual(
        encodeCommand(subscribeArgs("UNSUBSCRIBE", [])),
        "*1\r\n$11\r\nUNSUBSCRIBE\r\n");
}

func testEncodePublish() {
    testing.assertEqual(
        encodeCommand(publishArgs("news", "hello")),
        "*3\r\n$7\r\nPUBLISH\r\n$4\r\nnews\r\n$5\r\nhello\r\n");
}

# ---- pushed message parsing ----
func testMessageFromReply() {
    def pr as ParseResult init parseComplete(b("*3\r\n$7\r\nmessage\r\n$4\r\nnews\r\n$5\r\nhello\r\n"));
    def m as Message init messageFromReply($pr.reply);
    testing.assertEqual($m.kind, "message");
    testing.assertEqual($m.channel, "news");
    testing.assertEqual($m.pattern, "");
    testing.assertEqual($m.payload, "hello");
}

func testPmessageFromReply() {
    def pr as ParseResult init parseComplete(b("*4\r\n$8\r\npmessage\r\n$6\r\nnews.*\r\n$8\r\nnews.tec\r\n$2\r\nhi\r\n"));
    def m as Message init messageFromReply($pr.reply);
    testing.assertEqual($m.kind, "pmessage");
    testing.assertEqual($m.pattern, "news.*");
    testing.assertEqual($m.channel, "news.tec");
    testing.assertEqual($m.payload, "hi");
}

func testSubscribeConfirmationNotAMessage() {
    # A subscribe confirmation carries its verb as kind, so receiveMessage skips
    # it (kind is neither "message" nor "pmessage").
    def pr as ParseResult init parseComplete(b("*3\r\n$9\r\nsubscribe\r\n$4\r\nnews\r\n:1\r\n"));
    def m as Message init messageFromReply($pr.reply);
    testing.assertEqual($m.kind, "subscribe");
    testing.assertEqual($m.channel, "news");
}

func testMessageFromNonArray() {
    testing.assertEqual(messageFromReply(replyInt(7)).kind, "");
}

# ---- pipeline encoding ----
func testEncodePipeline() {
    testing.assertEqual(
        encodePipeline([["PING"], ["SET", "k", "v"]]),
        "*1\r\n$4\r\nPING\r\n*3\r\n$3\r\nSET\r\n$1\r\nk\r\n$1\r\nv\r\n");
    testing.assertEqual(encodePipeline([]), "");
}

# ---- SCAN reply parsing ----
func testScanResultFromReply() {
    # SCAN returns [ nextCursor(bulk), [ keys... ] ].
    def pr as ParseResult init parseComplete(b("*2\r\n$2\r\n17\r\n*2\r\n$4\r\nkey1\r\n$4\r\nkey2\r\n"));
    def sr as ScanResult init scanResultFromReply($pr.reply);
    testing.assertEqual($sr.cursor, 17);
    testing.assertEqual(len($sr.keys), 2);
    testing.assertEqual($sr.keys[0], "key1");
    testing.assertEqual($sr.keys[1], "key2");
}

func testScanResultTerminalCursor() {
    def pr as ParseResult init parseComplete(b("*2\r\n$1\r\n0\r\n*0\r\n"));
    def sr as ScanResult init scanResultFromReply($pr.reply);
    testing.assertEqual($sr.cursor, 0);
    testing.assertEqual(len($sr.keys), 0);
}

# ---- byte-exact command encoding ----
func testEncodeCommandBytesBinary() {
    # A value carrying NUL, CR, LF, and 0xFF must be framed by its byte count
    # (3 args, value length 4) and reach the wire verbatim.
    def val as bytes;
    $val[] = 0;
    $val[] = 13;
    $val[] = 10;
    $val[] = 255;
    def enc as bytes init encodeCommandBytes([
        convert.bytesFromString("SET", "utf-8"),
        convert.bytesFromString("k", "utf-8"),
        $val
    ]);
    def expected as bytes init convert.bytesFromString(
        "*3\r\n$3\r\nSET\r\n$1\r\nk\r\n$4\r\n",
        "utf-8");
    $expected = binary.concat($expected, $val);
    $expected = binary.concat($expected, convert.bytesFromString("\r\n", "utf-8"));
    testing.assertEqual($enc, $expected);
}
