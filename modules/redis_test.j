# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
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
use task;
use time;
use channel;

# The RESP parser frames over bytes; these helpers keep the string-literal
# test inputs readable by converting at the call boundary.
func b(s as string) {
    return convert.bytesFromString("utf-8", $s);
}

# reststr decodes the unconsumed remainder: the parser now returns a byte
# cursor (`pos`), so slice the original input from there.
func reststr(orig as string, pr as ParseResult) {
    def buf as bytes init b($orig);
    return convert.stringFromBytes("utf-8", $buf[$pr.pos..]);
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
    testing.assertThrows("overReplyCap", "limit");
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
        convert.bytesFromString("utf-8", "SET"),
        convert.bytesFromString("utf-8", "k"),
        $val
    ]);
    def expected as bytes init convert.bytesFromString(
        "utf-8",
        "*3\r\n$3\r\nSET\r\n$1\r\nk\r\n$4\r\n");
    $expected = binary.concat($expected, $val);
    $expected = binary.concat($expected, convert.bytesFromString("utf-8", "\r\n"));
    testing.assertEqual($enc, $expected);
}

# PERFORMANCE/DOS: readReply must decode an N-element array reply in O(N), not
# re-parse the whole accumulated buffer after every socket chunk (the old reader
# was O(N^2) - tens of seconds for a 20k-element LRANGE / SMEMBERS / KEYS). A fake
# server sends a large array; the incremental reader decodes it promptly and
# correctly, including the overshoot-carrying path.
func testReadOneValueLargeArrayFast() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def addr as string init net.address($listener);
    def n as int init 20000;
    def server as task of null init spawn {
        def srv as net.Conn init net.accept($listener);
        def parts as list of string init ["*{$n}\r\n"];
        def i as int init 0;
        while ($i < $n) {
            $parts[] = "$1\r\na\r\n";
            $i = $i + 1;
        }
        net.writeBytes($srv, convert.bytesFromString("utf-8", strings.join($parts, "")));
        net.close($srv);
    };
    def conn as net.Conn init net.connect($addr, 2000);
    def rv as ReadValue init readOneValue($conn, emptyBytes(), 2000);
    testing.assertEqual($rv.reply.kind, "array");
    testing.assertEqual(len($rv.reply.items), $n);
    testing.assertEqual($rv.reply.items[0].str, "a");
    testing.assertEqual($rv.reply.items[$n - 1].str, "a");
    net.close($conn);
    net.close($listener);
    task.wait($server);
}

# A server error reply is a RedisError with its code; another failure has none.
func testErrorRepliesAreRedisErrors() {
    def e as RedisError init replyError("WRONGTYPE Operation against a key holding the wrong kind of value");
    testing.assertEqual($e.kind, "redis");
    testing.assertEqual($e.code, "WRONGTYPE");
    testing.assertEqual(
        $e.message,
        "WRONGTYPE Operation against a key holding the wrong kind of value");
    testing.assertEqual(replyError("ERR unknown command 'FOO'").code, "ERR");
    testing.assertEqual(replyError("MOVED 3999 127.0.0.1:6381").code, "MOVED");
    testing.assertEqual(replyError("something odd").code, "");
    testing.assertEqual(redisError("redis: connection closed").code, "");
}

# A server that closes mid-reply, or while a subscriber waits, is kind
# "closed", not a RedisError.
func testAPeerCloseIsClosed() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        for (def reply in ["$10\r\nAB", ""]) {
            def conn as net.Conn init net.accept($listener);
            if ($reply != "") {
                net.writeBytes($conn, convert.bytesFromString("utf-8", $reply));
            }
            net.close($conn);
        }
    };
    def c as net.Conn init net.connect(net.address($listener));
    try {
        readReply($c, 2000);
        throw Error{kind: "missed", message: "a close mid-reply"};
    } catch (e) {
        testing.assertEqual($e.kind, "closed");
    }
    net.close($c);
    def s as Session init newSession(net.connect(net.address($listener)), 2000);
    try {
        receiveMessage($s);
        throw Error{kind: "missed", message: "a close while subscribed"};
    } catch (e) {
        testing.assertEqual($e.kind, "closed");
    }
    net.close($s.conn);
    task.wait($server);
    net.close($listener);
}

# serveSlowly accepts one connection and answers each of `replies` after a
# 300 ms pause, one per command read.
func serveSlowly(listener as net.Listener, replies as list of string) {
    def srv as net.Conn init net.accept($listener);
    for (def r in $replies) {
        net.readBytes($srv, 4096);
        time.sleep(time.fromMilliseconds(300));
        net.writeBytes($srv, b($r));
    }
    net.readBytes($srv, 1);
    net.close($srv);
}

# A timeout of 0 blocks: it clears the deadline an earlier timed read left
# armed, which has passed by now (resque.reserveBlocking(s, queues, 0) timed
# out on it). Both the reply reader and the bulk reader.
func testZeroTimeoutClearsAnEarlierDeadline() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        serveSlowly($listener, ["+OK\r\n", ":7\r\n", "$3\r\nabc\r\n"]);
    };
    def s as Session init newSession(net.connect(net.address($listener), 2000), 1000);
    testing.assertEqual(command($s, ["PING"]).str, "OK"); # arms a 1 s deadline
    time.sleep(time.fromMilliseconds(800)); # with the 300 ms reply, it has passed
    $s.timeout = 0;
    testing.assertEqual(command($s, ["INCR", "k"]).num, 7);
    net.writeBytes($s.conn, b(encodeCommand(["GET", "k"])));
    net.setReadDeadline($s.conn, 50);
    time.sleep(time.fromMilliseconds(100));
    testing.assertEqual(readBulkReply($s.conn, 0), b("abc"));
    net.close($s.conn);
    task.wait($server);
    net.close($listener);
}

# ---- large replies ----

# serveBytes accepts one connection, writes `payload` to it, then waits for the
# client to close it.
func serveBytes(listener as net.Listener, payload as bytes) {
    def srv as net.Conn init net.accept($listener);
    net.writeBytes($srv, $payload);
    net.readBytes($srv, 1);
    net.close($srv);
}

# A multi-MiB bulk reply is read whole and byte-exact, and the reply after it,
# sent in the same write, stays in `rest` for the next read. Reading 8 MiB took
# seconds while each 4 KiB read was appended onto the whole buffer; now the
# payload's missing bytes are one framed read.
func testReadOneValueLargeBulk() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def size as int init 8 * 1024 * 1024;
    def value as bytes init binary.make($size, 120);
    $value[$size - 1] = 121;
    def wire as bytes init binary.join([b("${$size}\r\n"), $value, b("\r\n+NEXT\r\n")]);
    def server as task of null init spawn {
        serveBytes($listener, $wire);
    };
    def conn as net.Conn init net.connect(net.address($listener), 2000);
    def rv as ReadValue init readOneValue($conn, emptyBytes(), 5000);
    testing.assertEqual($rv.reply.kind, "string");
    testing.assertEqual(len($rv.reply.str), $size);
    testing.assertEqual($rv.reply.str[$size - 2..], "xy");
    def next as ReadValue init readOneValue($conn, $rv.rest, 5000);
    testing.assertEqual($next.reply.str, "NEXT");
    net.close($conn);
    task.wait($server);
    net.close($listener);
}

# A bulk string declaring more than the reply cap fails with kind "limit" as
# soon as its header is read, not after 64 MiB have been buffered (or, as here,
# a stalled server has run into the read timeout).
func testOversizedBulkFailsAtItsHeader() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        serveBytes($listener, b("*2\r\n$1\r\na\r\n$70000000\r\nabc"));
    };
    def conn as net.Conn init net.connect(net.address($listener), 2000);
    try {
        readOneValue($conn, emptyBytes(), 1500);
        throw Error{kind: "missed", message: "an oversized bulk string"};
    } catch (e) {
        testing.assertEqual($e.kind, "limit");
    }
    net.close($conn);
    task.wait($server);
    net.close($listener);
}

# ---- pub/sub confirmation ----

func testTokenFromReply() {
    def pong as Reply init replyArray([
        replyStr("string", "pong"),
        replyStr("string", "jennifer-redis-7")
    ]);
    testing.assertEqual(tokenFromReply($pong), "jennifer-redis-7");
    testing.assertEqual(tokenFromReply(replyStr("string", "jennifer-redis-7")), "jennifer-redis-7");
    testing.assertEqual(tokenFromReply(replyStr("string", "OK")), "");
    def confirm as ParseResult init parseComplete(b("*3\r\n$9\r\nsubscribe\r\n$4\r\nnews\r\n:1\r\n"));
    testing.assertEqual(tokenFromReply($confirm.reply), "");
}

# push3 renders a pushed `[verb, a, b]` array.
func push3(verb as string, a as string, c as string) {
    return encodeCommand([$verb, $a, $c]);
}

# A scripted subscribed connection: it reads the SUBSCRIBE and its barrier PING,
# signals `answered` before it replies, then sends a message pushed before the
# confirmation (on a connection already subscribed elsewhere), the confirmation,
# the PING's reply and a second message, all in one write.
func serveSubscribe(listener as net.Listener, answered as channel of bool) {
    def srv as net.Conn init net.accept($listener);
    def cmd as ReadValue init readOneValue($srv, emptyBytes(), 5000);
    def ping as ReadValue init readOneValue($srv, $cmd.rest, 5000);
    time.sleep(time.fromMilliseconds(200));
    channel.send($answered, true);
    def token as string init $ping.reply.items[1].str;
    def wire as string init push3("message", "news", "first") +
        "*3\r\n$9\r\nsubscribe\r\n$4\r\nnews\r\n:1\r\n" +
        "*2\r\n$4\r\npong\r\n${len($token)}\r\n{$token}\r\n" + push3("message", "news", "second");
    net.writeBytes($srv, b($wire));
    net.readBytes($srv, 1);
    net.close($srv);
}

# subscribe returns only once the server has confirmed it, and the messages
# read meanwhile or coalesced with the confirmation reach receiveMessage in
# order. subscribe used to return as soon as the command was written, so a
# publish right after it could reach no subscriber.
func testSubscribeWaitsForConfirmation() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def answered as channel of bool init channel.make(1);
    def server as task of null init spawn {
        serveSubscribe($listener, $answered);
    };
    def s as Session init newSession(net.connect(net.address($listener), 2000), 5000);
    subscribe($s, ["news"]);
    testing.assertEqual(channel.len($answered), 1);
    testing.assertEqual(receiveMessage($s).payload, "first");
    testing.assertEqual(receiveMessage($s).payload, "second");
    net.close($s.conn);
    task.wait($server);
    net.close($listener);
}

# A server error reply to a subscribe call throws once its barrier is read, so
# the connection stays in step for the next call.
func testSubscribeErrorReplyThrows() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        def srv as net.Conn init net.accept($listener);
        def cmd as ReadValue init readOneValue($srv, emptyBytes(), 5000);
        def ping as ReadValue init readOneValue($srv, $cmd.rest, 5000);
        def token as string init $ping.reply.items[1].str;
        net.writeBytes(
            $srv,
            b("-ERR wrong number of arguments for 'subscribe' command\r\n" +
                "${len($token)}\r\n{$token}\r\n"));
        net.readBytes($srv, 1);
        net.close($srv);
    };
    def s as Session init newSession(net.connect(net.address($listener), 2000), 5000);
    try {
        subscribe($s, []);
        throw Error{kind: "missed", message: "an error reply"};
    } catch (e) {
        testing.assertEqual($e.kind, "redis");
        testing.assertEqual($e.code, "ERR");
    }
    def st as PubSub init channel.recv($s.pubsub);
    testing.assertEqual(len($st.buf), 0);
    testing.assertEqual(len($st.tokens), 0);
    net.close($s.conn);
    task.wait($server);
    net.close($listener);
}

# A receive loop in another task holds the socket while a psubscribe waits: the
# receiver reads the barrier reply and hands it over, and still gets the
# message after it.
func testSubscribeWhileAnotherTaskReceives() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        def srv as net.Conn init net.accept($listener);
        def cmd as ReadValue init readOneValue($srv, emptyBytes(), 5000);
        def ping as ReadValue init readOneValue($srv, $cmd.rest, 5000);
        def token as string init $ping.reply.items[1].str;
        net.writeBytes(
            $srv,
            b("*3\r\n$10\r\npsubscribe\r\n$6\r\nnews.*\r\n:1\r\n" +
                "*2\r\n$4\r\npong\r\n${len($token)}\r\n{$token}\r\n"));
        time.sleep(time.fromMilliseconds(100));
        net.writeBytes($srv, b(encodeCommand(["pmessage", "news.*", "news.tech", "hi"])));
        net.readBytes($srv, 1);
        net.close($srv);
    };
    def s as Session init newSession(net.connect(net.address($listener), 2000), 5000);
    def receiver as task of string init spawn {
        def m as Message init receiveMessage($s);
        return "{$m.kind} {$m.pattern} {$m.channel} {$m.payload}";
    };
    time.sleep(time.fromMilliseconds(50));
    psubscribe($s, ["news.*"]);
    testing.assertEqual(task.wait($receiver), "pmessage news.* news.tech hi");
    net.close($s.conn);
    task.wait($server);
    net.close($listener);
}
