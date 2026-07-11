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

func testEncodeCommand() {
    testing.assertEqual(encodeCommand(["SET", "key", "value"]),
        "*3\r\n$3\r\nSET\r\n$3\r\nkey\r\n$5\r\nvalue\r\n");
    testing.assertEqual(encodeCommand(["PING"]), "*1\r\n$4\r\nPING\r\n");
}

func testParseSimpleString() {
    def pr as ParseResult init parseComplete("+OK\r\n");
    testing.assertTrue($pr.complete);
    testing.assertEqual($pr.reply.kind, "string");
    testing.assertEqual($pr.reply.str, "OK");
    testing.assertEqual($pr.rest, "");
}

func testParseError() {
    def pr as ParseResult init parseComplete("-ERR unknown command\r\n");
    testing.assertEqual($pr.reply.kind, "error");
    testing.assertEqual($pr.reply.str, "ERR unknown command");
}

func testParseInteger() {
    def pr as ParseResult init parseComplete(":42\r\n");
    testing.assertEqual($pr.reply.kind, "int");
    testing.assertEqual($pr.reply.num, 42);
}

func testParseBulkString() {
    def pr as ParseResult init parseComplete("$5\r\nhello\r\n");
    testing.assertEqual($pr.reply.kind, "string");
    testing.assertEqual($pr.reply.str, "hello");
    testing.assertEqual($pr.rest, "");
}

func testParseNilBulk() {
    def pr as ParseResult init parseComplete("$-1\r\n");
    testing.assertEqual($pr.reply.kind, "nil");
}

func testParseArray() {
    def pr as ParseResult init parseComplete("*2\r\n$3\r\nfoo\r\n$3\r\nbar\r\n");
    testing.assertEqual($pr.reply.kind, "array");
    testing.assertEqual(len($pr.reply.items), 2);
    testing.assertEqual($pr.reply.items[0].str, "foo");
    testing.assertEqual($pr.reply.items[1].str, "bar");
}

func testParseMixedArray() {
    def pr as ParseResult init parseComplete("*2\r\n:1\r\n$3\r\nfoo\r\n");
    testing.assertEqual($pr.reply.items[0].kind, "int");
    testing.assertEqual($pr.reply.items[0].num, 1);
    testing.assertEqual($pr.reply.items[1].str, "foo");
}

func testParseIncomplete() {
    testing.assertFalse(parseComplete("+OK").complete);          # no CRLF yet
    testing.assertFalse(parseComplete("$5\r\nhel").complete);    # short bulk
    testing.assertFalse(parseComplete("*2\r\n$3\r\nfoo\r\n").complete);   # missing element
}

func testParseLeavesRest() {
    # A reply followed by the start of the next one leaves the remainder.
    def pr as ParseResult init parseComplete(":7\r\n+NEXT\r\n");
    testing.assertEqual($pr.reply.num, 7);
    testing.assertEqual($pr.rest, "+NEXT\r\n");
}
