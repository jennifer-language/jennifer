# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
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
    testing.assertEqual(encodeCommand(["SET", "key", "value"]),
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
    testing.assertFalse(parseComplete(b("+OK")).complete);          # no CRLF yet
    testing.assertFalse(parseComplete(b("$5\r\nhel")).complete);    # short bulk
    testing.assertFalse(parseComplete(b("*2\r\n$3\r\nfoo\r\n")).complete);   # missing element
}

func testParseLeavesRest() {
    # A reply followed by the start of the next one leaves the remainder.
    def pr as ParseResult init parseComplete(b(":7\r\n+NEXT\r\n"));
    testing.assertEqual($pr.reply.num, 7);
    testing.assertEqual(reststr(":7\r\n+NEXT\r\n", $pr), "+NEXT\r\n");
}
