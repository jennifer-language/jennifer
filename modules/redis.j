# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.28.0
# pragma-jennifer-capability: net

/**
 * A Redis client speaking RESP2 (the REdis Serialization Protocol) over the
 * `net` system library. Commands go out as RESP arrays of bulk strings; replies
 * (`+OK`, `-ERR`, `:int`, `$bulk`, `*array`) parse back into a `Reply`. Typed
 * per-command helpers (`get` / `set` / `incr` / `keys` / ...) keep the common
 * path fully typed; `command` is the generic escape hatch for anything else. A
 * `-ERR` reply throws a catchable `redis.RedisError` (kind "redis") whose `code`
 * is the reply's first word (`ERR`, `WRONGTYPE`, `NOAUTH`, `MOVED`, ...). The reply parser
 * frames over bytes and counts bulk-string lengths in bytes, so a value whose
 * byte length differs from its rune length (any non-ASCII UTF-8 text) is read
 * byte-exact. Needs the default `jennifer` binary (uses `net`).
 * @module redis
 * @example
 * import "transport.j" as transport;
 * def db as redis.Session init redis.connect(redis.Options{host: "127.0.0.1", port: 6379, security: transport.Security.None, user: "", password: "", db: 0});
 * redis.set($db, "greeting", "hello");
 * io.printf("%s\n", redis.get($db, "greeting"));
 * redis.quit($db);
 */
use encoding;
use net;
use binary;
use channel;
use crypto;
use lists;
use strings;
use convert;
import "./transport.j" as transport;

/**
 * Connection settings.
 * @field host {string} the server host
 * @field port {int} the server port
 * @field security {transport.Security} `transport.Security.None` (plaintext) or `.Tls` (rediss); `.Starttls` is rejected (Redis has no in-band upgrade)
 * @field user {string} the AUTH username ("" for password-only or no auth)
 * @field password {string} the AUTH password; "" skips AUTH
 * @field db {int} the database to SELECT (0 is the default)
 */
export def struct Options {
    host as string,
    port as int,
    security as transport.Security,
    user as string,
    password as string,
    db as int
};

# The default per-read idle timeout (ms), so a hung server fails instead of
# blocking forever. `connect` sets `Session.timeoutMs`; override it or use 0 to disable.
def const DEFAULT_TIMEOUT_MS as int init 30000;

# MAX_REPLY_BYTES caps a single accumulated reply. A malicious / compromised
# server can declare an enormous bulk-string length (or simply never terminate a
# reply), which would grow the read buffer without bound; this fails the read
# with a catchable error instead.
def const MAX_REPLY_BYTES as int init 67108864;

# capReply throws when an accumulated reply has grown past the cap.
/**
 * A failed Redis command. Every error of kind `"redis"` is one, so its fields
 * are safe to read once the kind matches.
 * @field kind {string} always `"redis"`
 * @field message {string} the server's error reply (`WRONGTYPE Operation against a key ...`), or what went wrong
 * @field code {string} the error reply's first word, its error code (`ERR`, `WRONGTYPE`, `NOAUTH`, `WRONGPASS`, `MOVED`, `LOADING`, `READONLY`, ...); "" for a failure that is no server error reply
 * @example
 * try {
 *     redis.command($s, ["INCR", "page:views"]);
 * } catch (e) {
 *     if ($e.kind == "redis" and $e.code == "WRONGTYPE") {
 *         io.printf("page:views is not a counter\n");
 *     } else {
 *         throw $e;
 *     }
 * }
 */
export def struct RedisError {
    kind as string,
    message as string,
    code as string
};

# redisError is the RedisError of a failure that is no server error reply.
func redisError(message as string) {
    return RedisError{kind: "redis", message: $message, code: ""};
}

# replyError is the RedisError of a server error reply: its first word, when it
# is an upper-case code, is the code.
func replyError(text as string) {
    def word as string init strings.split(strings.trim($text), " ")[0];
    def code as string init "";
    if ($word != "" and strings.upper($word) == $word and strings.lower($word) != $word) {
        $code = $word;
    }
    return RedisError{kind: "redis", message: $text, code: $code};
}

func capReply(n as int) {
    if ($n > MAX_REPLY_BYTES) {
        throw Error{
            kind: "limit",
            message: "redis: reply exceeds the {MAX_REPLY_BYTES}-byte limit"
        };
    }
    return;
}

/**
 * A pushed pub/sub message (from `receiveMessage`).
 * @field kind {string} "message" (a plain channel push) or "pmessage" (a
 *   pattern push)
 * @field channel {string} the channel the message was published to
 * @field pattern {string} the subscribed glob pattern ("" for a plain message)
 * @field payload {string} the message body
 */
export def struct Message {
    kind as string,
    channel as string,
    pattern as string,
    payload as string
};

# PubSub is a subscribed connection's read state, held in a one-slot channel so
# every copy of the Session (one in a spawned receive loop, one that subscribes)
# shares it, and so whoever holds it is the one reader of the socket: the bytes
# read past the last reply, the pushed messages read while waiting for a
# (un)subscribe to be confirmed (in arrival order, for `receiveMessage`), and
# the barrier tokens read on behalf of another waiter.
def struct PubSub {
    buf as bytes,
    queue as list of Message,
    tokens as list of string
};

/**
 * An open Redis connection. Build it with `connect`.
 * @field conn {net.Conn} the underlying socket
 * @field timeoutMs {int} per-read idle timeout in milliseconds (0 disables it)
 * @field pubsub {channel of PubSub} the shared pub/sub read state (internal)
 */
export def struct Session {
    conn as net.Conn,
    timeoutMs as int,
    pubsub as channel of PubSub
};

/**
 * A parsed RESP reply.
 * @field kind {string} "string" (simple or bulk), "error", "int", "nil", or "array"
 * @field str {string} the string / error text
 * @field num {int} the integer value
 * @field items {list of Reply} an array reply's elements
 */
export def struct Reply {
    kind as string,
    str as string,
    num as int,
    items as list of Reply
};

/**
 * One step of a `scan` cursor walk.
 * @field cursor {int} the cursor to pass to the next `scan` (0 ends the walk)
 * @field keys {list of string} the keys returned by this step
 */
export def struct ScanResult {
    cursor as int,
    keys as list of string
};

# One parse step's result: the value, the byte offset in the buffer just past
# what was consumed, and whether the buffer held a complete value. A cursor
# (`pos`) rather than a sliced remainder keeps parsing an N-element array O(N)
# instead of O(N^2) - each step would otherwise copy the whole rest of the
# buffer. RESP bulk-string lengths are byte counts, so framing is byte-indexed
# and a payload is decoded to a string only after the full run is in hand.
def struct ParseResult {
    reply as Reply,
    pos as int,
    complete as bool
};

# ReadValue carries one decoded reply plus any bytes the socket read past it
# (overshoot from the last chunk), so a pipelined batch keeps the leftover for the
# next reply instead of dropping it.
def struct ReadValue {
    reply as Reply,
    rest as bytes
};

# --- reply constructors (private) ----------------------------------

func replyStr(kind as string, s as string) {
    return Reply{kind: $kind, str: $s, num: 0, items: []};
}

func replyInt(n as int) {
    return Reply{kind: "int", str: "", num: $n, items: []};
}

func replyNil() {
    return Reply{kind: "nil", str: "", num: 0, items: []};
}

func replyArray(items as list of Reply) {
    return Reply{kind: "array", str: "", num: 0, items: $items};
}

func done(reply as Reply, pos as int) {
    return ParseResult{reply: $reply, pos: $pos, complete: true};
}

# CRLF terminates every RESP header line.
def const CRLF as bytes init encoding.encode("utf-8", "\r\n");
def const LF as bytes init encoding.encode("utf-8", "\n");

func incomplete() {
    return ParseResult{reply: replyNil(), pos: 0, complete: false};
}

# --- RESP encode / decode (private, unit-tested) -------------------

# encodeCommand renders a command's arguments as a RESP array of bulk strings.
# Bulk lengths are byte counts. Each argument's piece is appended to a list and
# joined once, so a command with many arguments (a wide MSET / RPUSH / SADD)
# stays linear instead of re-copying the growing accumulator per argument.
func encodeCommand(args as list of string) {
    def parts as list of string init [];
    $parts[] = "*" + convert.toString(len($args)) + "\r\n";
    for (def arg in $args) {
        def blen as int init len(encoding.encode("utf-8", $arg));
        $parts[] = "${$blen}\r\n" + $arg + "\r\n";
    }
    return strings.join($parts, "");
}

# parseBulkAt parses a `$`-length bulk string in `buf` starting at `pos` (just
# past its length header), or reports incomplete. The bulk length is a byte
# count, so framing is byte-indexed and the payload is decoded to a string only
# after the full run is in hand.
func parseBulkAt(payload as string, buf as bytes, pos as int) {
    def n as int init convert.toInt($payload);
    if ($n < 0) {
        return done(replyNil(), $pos);
    }
    if (len($buf) - $pos < $n + 2) {
        return incomplete();
    }
    def data as bytes init $buf[$pos..$pos + $n];
    return done(replyStr("string", encoding.decode("utf-8", $data)), $pos + $n + 2);
}

# parseArrayAt parses a `*`-count array in `buf` starting at `pos`, recursing
# per element and carrying an integer cursor (never slicing the remainder), so
# an N-element array parses in O(N), not O(N^2).
func parseArrayAt(payload as string, buf as bytes, pos as int) {
    def count as int init convert.toInt($payload);
    if ($count < 0) {
        return done(replyNil(), $pos);
    }
    def items as list of Reply init [];
    def cur as int init $pos;
    def i as int init 0;
    while ($i < $count) {
        def pr as ParseResult init parseAt($buf, $cur);
        if (not $pr.complete) {
            return incomplete();
        }
        $items[] = $pr.reply;
        $cur = $pr.pos;
        $i = $i + 1;
    }
    return done(replyArray($items), $cur);
}

# parseAt parses one RESP value in `buf` starting at byte offset `pos`.
# `complete` is false when the buffer does not yet hold the whole value. The
# control framing (type byte, `\r\n`, length headers) is ASCII; only the
# bulk-string payloads carry arbitrary bytes, and those are sliced once.
func parseAt(buf as bytes, pos as int) {
    def nl as int init binary.indexOf($buf, CRLF, $pos);
    if ($nl < 0) {
        return incomplete();
    }
    def typ as int init $buf[$pos];
    def payload as string init encoding.decode("utf-8", $buf[$pos + 1..$nl]);
    def after as int init $nl + 2;
    match ($typ) {
        when 43 { # '+'
            return done(replyStr("string", $payload), $after);
        }
        when 45 { # '-'
            return done(replyStr("error", $payload), $after);
        }
        when 58 { # ':'
            return done(replyInt(convert.toInt($payload)), $after);
        }
        when 36 { # '$'
            return parseBulkAt($payload, $buf, $after);
        }
        when 42 { # '*'
            return parseArrayAt($payload, $buf, $after);
        }
        else {
            # Unknown type byte: surface the whole line as a string.
            def line as string init encoding.decode("utf-8", $buf[$pos..$nl]);
            return done(replyStr("string", $line), $after);
        }
    }
}

# parseComplete parses one RESP value from the front of `buf` (offset 0).
func parseComplete(buf as bytes) {
    return parseAt($buf, 0);
}

# subscribeArgs builds the argument list for a (P)SUBSCRIBE / (P)UNSUBSCRIBE
# command: the verb followed by each channel / pattern. Pure, so the overlay can
# check the encoded wire form without a server.
func subscribeArgs(verb as string, names as list of string) {
    def args as list of string init [$verb];
    for (def name in $names) {
        $args[] = $name;
    }
    return $args;
}

# publishArgs builds the argument list for a PUBLISH command.
func publishArgs(channel as string, message as string) {
    return ["PUBLISH", $channel, $message];
}

# encodePipeline renders several commands into one back-to-back RESP payload, so
# a pipeline is a single write followed by N reads (one network round trip).
# Command payloads are appended in place and joined once, so a long pipeline
# stays linear instead of re-copying the accumulated payload per command.
func encodePipeline(commands as list of list of string) {
    def parts as list of string init [];
    for (def cmd in $commands) {
        $parts[] = encodeCommand($cmd);
    }
    return strings.join($parts, "");
}

# emptyBytes returns a fresh zero-length bytes value.
func emptyBytes() {
    def e as bytes;
    return $e;
}

# encodeCommandBytes renders a command whose arguments are raw `bytes` as a RESP
# array of bulk strings, so a binary value (a serialized blob, a compressed
# payload) reaches the wire byte-for-byte instead of through a UTF-8 string. The
# control framing (`*`, `$`, length headers, CRLF) is ASCII; only the argument
# payloads carry arbitrary bytes.
func encodeCommandBytes(args as list of bytes) {
    def parts as list of bytes init [];
    $parts[] = encoding.encode("utf-8", "*" + convert.toString(len($args)) + "\r\n");
    for (def arg in $args) {
        def hdr as string init "$" + convert.toString(len($arg)) + "\r\n";
        $parts[] = encoding.encode("utf-8", $hdr);
        $parts[] = $arg;
        $parts[] = CRLF;
    }
    return binary.join($parts);
}

# readBulkReply reads one RESP reply expecting a bulk string (a GET-style reply)
# and returns its payload as raw `bytes` - the byte-exact counterpart to the
# string reader, which decodes as UTF-8 and would throw on a binary value. A nil
# (`$-1`) reply yields empty bytes; an error (`-`) reply throws; the bulk length
# is a byte count, so the payload (which may itself contain CRLF) is framed by
# count, not by scanning.
func readBulkReply(conn as net.Conn, timeoutMs as int) {
    # Read until the reply's header line (the first CRLF) is in hand. The header
    # is short, so this settles in the first read; any payload bytes read
    # alongside it are the "overshoot" carried into the framed read below.
    # Chunks collect in a list and join only once a chunk carries an LF (the
    # header's CRLF may straddle two reads, so the joined head is what is
    # searched).
    def parts as list of bytes;
    def total as int init 0;
    def head as bytes;
    def nl as int init -1;
    while ($nl < 0) {
        # 0 clears a deadline an earlier timed read left armed (else a blocking
        # read could hit it).
        net.setReadDeadline($conn, $timeoutMs);
        def chunk as bytes init net.readBytes($conn, 4096);
        if (len($chunk) == 0) {
            throw Error{
                kind: "closed",
                message: "redis: connection closed before a complete reply"
            };
        }
        $parts[] = $chunk;
        $total = $total + len($chunk);
        capReply($total);
        if (binary.indexOf($chunk, LF) >= 0) {
            $head = binary.join($parts);
            $nl = binary.indexOf($head, CRLF);
        }
    }

    def typ as int init $head[0];
    def header as string init encoding.decode("utf-8", $head[1..$nl]);
    if ($typ == 45) { # '-' error
        throw replyError($header);
    }
    if ($typ != 36) {
        # A '+' simple string or ':' integer reply: return the line's payload
        # bytes (unusual for a value read, but never decode-throws).
        return $head[1..$nl];
    }

    # '$' bulk string: framed by a byte count, so the payload (which may itself
    # contain CRLF) is read by count, not by scanning.
    def n as int init convert.toInt($header);
    if ($n < 0) {
        return emptyBytes(); # nil
    }
    def start as int init $nl + 2;
    capReply($start + $n + 2);

    # Payload bytes already read alongside the header, then the exact remainder
    # in one framed read - so a large value is O(value), not the O(value^2) a
    # concat-per-chunk accumulation would cost.
    def have as bytes init $head[$start..];
    def need as int init $n + 2 - len($have);
    if ($need > 0) {
        $have = binary.concat($have, readExact($conn, $need, $timeoutMs));
    }
    return $have[0..$n];
}

# readExact reads exactly `n` bytes in one native call; `timeoutMs` is the idle
# timeout, re-armed per underlying read, so it bounds a stall rather than the
# whole value (0 waits without limit).
func readExact(conn as net.Conn, n as int, timeoutMs as int) {
    net.setReadDeadline($conn, 0);
    net.setIdleTimeout($conn, $timeoutMs);
    defer net.setIdleTimeout($conn, 0);
    return net.readN($conn, $n);
}

# messageFromReply classifies a pushed pub/sub array into a Message. A `message`
# push is `[ "message", channel, payload ]`; a `pmessage` push is
# `[ "pmessage", pattern, channel, payload ]`; a (un)subscribe confirmation is
# `[ verb, channel, count ]` and keeps its verb as `kind` with an empty payload.
# A non-array or short reply yields an empty-kind Message. Pure and total, so the
# overlay exercises the framing without a socket.
func messageFromReply(reply as Reply) {
    if ($reply.kind != "array" or len($reply.items) < 3) {
        return Message{kind: "", channel: "", pattern: "", payload: ""};
    }
    def tag as string init $reply.items[0].str;
    if ($tag == "message") {
        return Message{
            kind: "message",
            channel: $reply.items[1].str,
            pattern: "",
            payload: $reply.items[2].str
        };
    }
    if ($tag == "pmessage" and len($reply.items) >= 4) {
        return Message{
            kind: "pmessage",
            channel: $reply.items[2].str,
            pattern: $reply.items[1].str,
            payload: $reply.items[3].str
        };
    }
    return Message{kind: $tag, channel: $reply.items[1].str, pattern: "", payload: ""};
}

# scanResultFromReply reads a SCAN reply - a two-element array of the next
# cursor (a bulk string) and an array of keys - into a ScanResult. Pure.
func scanResultFromReply(reply as Reply) {
    def keys as list of string init [];
    if ($reply.kind == "array" and len($reply.items) >= 2) {
        for (def item in $reply.items[1].items) {
            $keys[] = $item.str;
        }
        return ScanResult{cursor: convert.toInt($reply.items[0].str), keys: $keys};
    }
    return ScanResult{cursor: 0, keys: $keys};
}

# --- net dialogue (private) ----------------------------------------

# readOneValue reads one complete RESP reply, decoding it incrementally as bytes
# arrive so the accumulated buffer is never re-parsed from the start (the old
# reader called parseComplete from offset 0 after every chunk, making an
# N-element reply O(N^2)). It parses with a flat value buffer plus an array stack:
# each scalar / bulk is produced once and pushed, and a `*` header opens a frame
# recording where its elements begin; when a frame's count is met, its elements
# are lifted out of the buffer in one window into a `replyArray` and that array is
# itself produced (cascading for nested arrays). `pos` only advances past a fully
# decoded value, so a short buffer re-reads just the current value's header, not
# the whole reply. Returns the reply plus any overshoot bytes read past it, so a
# pipelined caller keeps the leftover. `pending` seeds the buffer with a prior
# call's overshoot.
#
# The buffer only ever holds the value being decoded: a refill first drops the
# bytes before `pos` (already decoded into `output`), counting them in `used`
# for the reply cap, and a bulk string whose payload is not yet in hand is read
# with one framed `net.readN` of exactly the missing bytes. So no byte is copied
# more than a constant number of times and a large reply reads in O(size), not
# the O(size^2) of appending each socket chunk onto the whole reply.
func readOneValue(conn as net.Conn, pending as bytes, timeoutMs as int) {
    def buf as bytes init $pending;
    def pos as int init 0;
    def used as int init 0;
    def output as list of Reply init [];
    def outLen as int init 0;
    def arrBase as list of int init [];
    def arrRemain as list of int init [];
    def asp as int init 0;
    def result as Reply;
    def haveResult as bool init false;
    while (not $haveResult) {
        def nl as int init binary.indexOf($buf, CRLF, $pos);
        if ($nl < 0) {
            $used = $used + $pos;
            $buf = refillReply($conn, $buf[$pos..], $timeoutMs, $used);
            $pos = 0;
            continue;
        }
        def typ as int init $buf[$pos];
        def payload as string init encoding.decode("utf-8", $buf[$pos + 1..$nl]);
        def after as int init $nl + 2;
        def v as Reply;
        def haveV as bool init false;
        if ($typ == 42) { # '*' array header
            def count as int init convert.toInt($payload);
            $pos = $after;
            if ($count < 0) {
                $v = replyNil();
                $haveV = true;
            } elseif ($count == 0) {
                $v = replyArray([]);
                $haveV = true;
            } else {
                if ($asp < len($arrBase)) {
                    $arrBase[$asp] = $outLen;
                    $arrRemain[$asp] = $count;
                } else {
                    $arrBase[] = $outLen;
                    $arrRemain[] = $count;
                }
                $asp = $asp + 1;
            }
        } elseif ($typ == 36) { # '$' bulk string
            def n as int init convert.toInt($payload);
            if ($n < 0) {
                $pos = $after;
                $v = replyNil();
                $haveV = true;
            } elseif (len($buf) - $after < $n + 2) {
                capReply($used + $after + $n + 2);
                def more as bytes init readExact($conn, $n + 2 - (len($buf) - $after), $timeoutMs);
                $used = $used + $pos;
                $buf = binary.concat($buf[$pos..], $more);
                $pos = 0;
                # retry this value now that its payload is in hand
            } else {
                def data as bytes init $buf[$after..$after + $n];
                $v = replyStr("string", encoding.decode("utf-8", $data));
                $pos = $after + $n + 2;
                $haveV = true;
            }
        } elseif ($typ == 43) { # '+' simple string
            $v = replyStr("string", $payload);
            $pos = $after;
            $haveV = true;
        } elseif ($typ == 45) { # '-' error
            $v = replyStr("error", $payload);
            $pos = $after;
            $haveV = true;
        } elseif ($typ == 58) { # ':' integer
            $v = replyInt(convert.toInt($payload));
            $pos = $after;
            $haveV = true;
        } else {
            $v = replyStr("string", encoding.decode("utf-8", $buf[$pos..$nl]));
            $pos = $after;
            $haveV = true;
        }
        if ($haveV) {
            # Produce `v`, folding it into the open array (if any); a completed
            # array becomes a produced value in turn, cascading outward.
            def cur as Reply init $v;
            def settling as bool init true;
            while ($settling) {
                $settling = false;
                if ($asp == 0) {
                    $result = $cur;
                    $haveResult = true;
                } else {
                    if ($outLen < len($output)) {
                        $output[$outLen] = $cur;
                    } else {
                        $output[] = $cur;
                    }
                    $outLen = $outLen + 1;
                    $arrRemain[$asp - 1] = $arrRemain[$asp - 1] - 1;
                    if ($arrRemain[$asp - 1] == 0) {
                        def base as int init $arrBase[$asp - 1];
                        def items as list of Reply init [];
                        def k as int init $base;
                        while ($k < $outLen) {
                            $items[] = $output[$k];
                            $k = $k + 1;
                        }
                        $outLen = $base;
                        $asp = $asp - 1;
                        $cur = replyArray($items);
                        $settling = true;
                    }
                }
            }
        }
    }
    return ReadValue{reply: $result, rest: $buf[$pos..]};
}

# READ_CHUNK is the most one socket read takes while looking for a header line.
def const READ_CHUNK as int init 65536;

# refillReply returns `buf` (the undecoded tail of a reply) with socket reads
# appended until one of them carries an LF, so the header line being looked for
# can be complete. The reads collect in a list and join once, so a long line is
# no quadratic re-copy either. `used` is the number of this reply's bytes
# already decoded and dropped, so the 64 MiB cap counts the whole reply. Throws
# kind "closed" on a closed connection.
func refillReply(conn as net.Conn, buf as bytes, timeoutMs as int, used as int) {
    def parts as list of bytes init [$buf];
    def total as int init $used + len($buf);
    while (true) {
        # 0 clears a deadline an earlier timed read left armed (else a blocking
        # read could hit it).
        net.setReadDeadline($conn, $timeoutMs);
        def chunk as bytes init net.readBytes($conn, READ_CHUNK);
        if (len($chunk) == 0) {
            throw Error{
                kind: "closed",
                message: "redis: connection closed before a complete reply"
            };
        }
        $parts[] = $chunk;
        $total = $total + len($chunk);
        capReply($total);
        if (binary.indexOf($chunk, LF) >= 0) {
            return binary.join($parts);
        }
    }
    return $buf;
}

# readReply reads one complete RESP reply. Over-read past the reply is discarded
# (a single-command caller sends one command and reads one reply).
func readReply(conn as net.Conn, timeoutMs as int) {
    return readOneValue($conn, emptyBytes(), $timeoutMs).reply;
}

# readReplies reads exactly `count` complete replies, threading any bytes read
# past one reply into the next (a pipelined batch coalesced into one TCP segment),
# so the leftover is never dropped.
func readReplies(conn as net.Conn, timeoutMs as int, count as int) {
    def replies as list of Reply init [];
    def pending as bytes init emptyBytes();
    while (len($replies) < $count) {
        def rv as ReadValue init readOneValue($conn, $pending, $timeoutMs);
        $replies[] = $rv.reply;
        $pending = $rv.rest;
    }
    return $replies;
}

# connectOptions bounds the connect (and the TLS handshake) by DEFAULT_TIMEOUT_MS.
func connectOptions(tls as bool) {
    return net.ConnectOptions{timeoutMs: DEFAULT_TIMEOUT_MS, tls: $tls};
}

func dial(opts as Options) {
    def addr as string init $opts.host + ":{$opts.port}";
    match ($opts.security) {
        when Tls { return net.connectWith($addr, connectOptions(true)); }
        when None { return net.connectWith($addr, connectOptions(false)); }
        when Starttls {
            throw redisError("redis: STARTTLS is not supported; use transport.Security.Tls (rediss) or .None");
        }
    }
}

# --- commands (exported) -------------------------------------------

/**
 * Send one command (its arguments) and return the reply.
 * @param session {Session} the open session
 * @param args {list of string} the command name and its arguments
 * @return {Reply} the parsed reply
 * @throws {RedisError} kind "redis" on a `-ERR` reply
 */
export func command(session as Session, args as list of string) {
    net.writeBytes($session.conn, encoding.encode("utf-8", encodeCommand($args)));
    def reply as Reply init readReply($session.conn, $session.timeoutMs);
    if ($reply.kind == "error") {
        throw replyError($reply.str);
    }
    return $reply;
}

/**
 * Open a session, authenticating and selecting a database when set.
 * @param opts {Options} the connection settings
 * @return {Session} the open session
 */
export func connect(opts as Options) {
    def session as Session init newSession(dial($opts), DEFAULT_TIMEOUT_MS);
    # A refused AUTH / SELECT must not leak the socket; on success the caller
    # owns the open session.
    errdefer net.close($session.conn);
    if (len($opts.password) > 0) {
        def auth as list of string init ["AUTH"];
        if (len($opts.user) > 0) {
            $auth[] = $opts.user;
        }
        $auth[] = $opts.password;
        command($session, $auth);
    }
    if ($opts.db > 0) {
        command($session, ["SELECT", convert.toString($opts.db)]);
    }
    return $session;
}

/**
 * Return the string value of `key`, or "" when the key is missing.
 * @param session {Session} the open session
 * @param key {string} the key to read
 * @return {string} the value, or "" when missing
 */
export func get(session as Session, key as string) {
    return command($session, ["GET", $key]).str;
}

/**
 * Store `value` at `key`.
 * @param session {Session} the open session
 * @param key {string} the key to write
 * @param value {string} the value to store
 */
export func set(session as Session, key as string, value as string) {
    command($session, ["SET", $key, $value]);
}

/**
 * Store a raw `bytes` value at `key`, byte-for-byte. Unlike `set` (whose string
 * value is UTF-8-encoded onto the wire), this stores arbitrary binary - a
 * serialized blob, a compressed payload, an image - which `getBytes` reads back
 * exactly. Read it with `getBytes`, not `get` (the string reader throws on a
 * non-UTF-8 value).
 * @param session {Session} the open session
 * @param key {string} the key to write
 * @param value {bytes} the raw value to store
 * @throws {RedisError} kind "redis" on a `-ERR` reply
 */
export func setBytes(session as Session, key as string, value as bytes) {
    def args as list of bytes init [
        encoding.encode("utf-8", "SET"),
        encoding.encode("utf-8", $key),
        $value
    ];
    net.writeBytes($session.conn, encodeCommandBytes($args));
    def reply as Reply init readReply($session.conn, $session.timeoutMs);
    if ($reply.kind == "error") {
        throw replyError($reply.str);
    }
    return;
}

/**
 * Return the raw `bytes` value of `key`, or empty `bytes` when the key is
 * missing (use `exists` to tell an empty value from a missing key). The
 * byte-exact counterpart to `get`: it never UTF-8-decodes, so a binary value
 * stored with `setBytes` round-trips exactly.
 * @param session {Session} the open session
 * @param key {string} the key to read
 * @return {bytes} the value, or empty bytes when missing
 * @throws {RedisError} kind "redis" on a `-ERR` reply
 */
export func getBytes(session as Session, key as string) {
    net.writeBytes($session.conn, encoding.encode("utf-8", encodeCommand(["GET", $key])));
    return readBulkReply($session.conn, $session.timeoutMs);
}

/**
 * Delete `key` and return the number of keys removed (0 or 1).
 * @param session {Session} the open session
 * @param key {string} the key to delete
 * @return {int} the number of keys removed (0 or 1)
 */
export func del(session as Session, key as string) {
    return command($session, ["DEL", $key]).num;
}

/**
 * Report whether `key` is present.
 * @param session {Session} the open session
 * @param key {string} the key to test
 * @return {bool} whether the key exists
 */
export func exists(session as Session, key as string) {
    return command($session, ["EXISTS", $key]).num > 0;
}

/**
 * Atomically increment `key` and return the new value.
 * @param session {Session} the open session
 * @param key {string} the counter key
 * @return {int} the new value
 */
export func incr(session as Session, key as string) {
    return command($session, ["INCR", $key]).num;
}

/**
 * Atomically decrement `key` and return the new value.
 * @param session {Session} the open session
 * @param key {string} the counter key
 * @return {int} the new value
 */
export func decr(session as Session, key as string) {
    return command($session, ["DECR", $key]).num;
}

/**
 * Return the keys matching a glob `pattern` (e.g. "*", "user:*").
 * @param session {Session} the open session
 * @param pattern {string} the glob pattern
 * @return {list of string} the matching keys
 */
export func keys(session as Session, pattern as string) {
    def out as list of string init [];
    for (def item in command($session, ["KEYS", $pattern]).items) {
        $out[] = $item.str;
    }
    return $out;
}

# --- typed hash / list / set helpers (exported) --------------------
# Thin, string-valued wrappers over `command` for the common Redis container
# types. For a binary field / element / member, build the command yourself with
# `command` + `encodeCommandBytes` (the `setBytes` pattern), or store the blob at
# a plain key with `setBytes`.

# arrayToStrings reads an array Reply's bulk items into a list of string.
func arrayToStrings(reply as Reply) {
    def out as list of string init [];
    for (def item in $reply.items) {
        $out[] = $item.str;
    }
    return $out;
}

/**
 * Set hash `key`'s `field` to `value` (HSET); returns the number of fields newly
 * created (0 when it already existed and was updated).
 * @param session {Session} the open session
 * @param key {string} the hash key
 * @param field {string} the field name
 * @param value {string} the field value
 * @return {int} the number of new fields (0 or 1)
 */
export func hset(session as Session, key as string, field as string, value as string) {
    return command($session, ["HSET", $key, $field, $value]).num;
}

/**
 * Return hash `key`'s `field` (HGET), or "" when the field or key is missing.
 * @param session {Session} the open session
 * @param key {string} the hash key
 * @param field {string} the field name
 * @return {string} the value, or "" when missing
 */
export func hget(session as Session, key as string, field as string) {
    return command($session, ["HGET", $key, $field]).str;
}

/**
 * Return every field and value of hash `key` (HGETALL) as a `map of string to
 * string` (empty when the key is missing).
 * @param session {Session} the open session
 * @param key {string} the hash key
 * @return {map of string to string} the field -> value pairs
 */
export func hgetAll(session as Session, key as string) {
    def out as map of string to string init {};
    def items as list of Reply init command($session, ["HGETALL", $key]).items;
    def i as int init 0;
    # HGETALL is a flat array [field, value, field, value, ...].
    while ($i + 1 < len($items)) {
        $out[$items[$i].str] = $items[$i + 1].str;
        $i = $i + 2;
    }
    return $out;
}

/**
 * Delete `field` from hash `key` (HDEL); returns the number removed (0 or 1).
 * @param session {Session} the open session
 * @param key {string} the hash key
 * @param field {string} the field to delete
 * @return {int} the number of fields removed
 */
export func hdel(session as Session, key as string, field as string) {
    return command($session, ["HDEL", $key, $field]).num;
}

/**
 * Prepend `value` to list `key` (LPUSH); returns the list's new length.
 * @param session {Session} the open session
 * @param key {string} the list key
 * @param value {string} the value to prepend
 * @return {int} the new list length
 */
export func lpush(session as Session, key as string, value as string) {
    return command($session, ["LPUSH", $key, $value]).num;
}

/**
 * Append `value` to list `key` (RPUSH); returns the list's new length.
 * @param session {Session} the open session
 * @param key {string} the list key
 * @param value {string} the value to append
 * @return {int} the new list length
 */
export func rpush(session as Session, key as string, value as string) {
    return command($session, ["RPUSH", $key, $value]).num;
}

/**
 * Return the elements of list `key` from `start` to `stop` inclusive (LRANGE;
 * negative indices count from the end, so `0, -1` is the whole list).
 * @param session {Session} the open session
 * @param key {string} the list key
 * @param start {int} the start index
 * @param stop {int} the stop index (inclusive)
 * @return {list of string} the elements in range
 */
export func lrange(session as Session, key as string, start as int, stop as int) {
    return arrayToStrings(command(
        $session,
        ["LRANGE", $key, convert.toString($start), convert.toString($stop)]));
}

/**
 * Return the length of list `key` (LLEN); 0 when the key is missing.
 * @param session {Session} the open session
 * @param key {string} the list key
 * @return {int} the list length
 */
export func llen(session as Session, key as string) {
    return command($session, ["LLEN", $key]).num;
}

/**
 * Remove and return the first element of list `key` (LPOP), or "" when empty.
 * @param session {Session} the open session
 * @param key {string} the list key
 * @return {string} the popped element, or "" when the list is empty
 */
export func lpop(session as Session, key as string) {
    return command($session, ["LPOP", $key]).str;
}

/**
 * Add `member` to set `key` (SADD); returns the number newly added (0 or 1).
 * @param session {Session} the open session
 * @param key {string} the set key
 * @param member {string} the member to add
 * @return {int} the number of members added
 */
export func sadd(session as Session, key as string, member as string) {
    return command($session, ["SADD", $key, $member]).num;
}

/**
 * Remove `member` from set `key` (SREM); returns the number removed (0 or 1).
 * @param session {Session} the open session
 * @param key {string} the set key
 * @param member {string} the member to remove
 * @return {int} the number of members removed
 */
export func srem(session as Session, key as string, member as string) {
    return command($session, ["SREM", $key, $member]).num;
}

/**
 * Return every member of set `key` (SMEMBERS) as a list (empty when missing;
 * order is unspecified).
 * @param session {Session} the open session
 * @param key {string} the set key
 * @return {list of string} the members
 */
export func smembers(session as Session, key as string) {
    return arrayToStrings(command($session, ["SMEMBERS", $key]));
}

/**
 * Report whether `member` is in set `key` (SISMEMBER).
 * @param session {Session} the open session
 * @param key {string} the set key
 * @param member {string} the member to test
 * @return {bool} whether the member is present
 */
export func sismember(session as Session, key as string, member as string) {
    return command($session, ["SISMEMBER", $key, $member]).num > 0;
}

/**
 * Return the number of members in set `key` (SCARD); 0 when missing.
 * @param session {Session} the open session
 * @param key {string} the set key
 * @return {int} the member count
 */
export func scard(session as Session, key as string) {
    return command($session, ["SCARD", $key]).num;
}

/**
 * Return the server's PONG (a liveness check).
 * @param session {Session} the open session
 * @return {string} the server's reply ("PONG")
 */
export func ping(session as Session) {
    return command($session, ["PING"]).str;
}

/**
 * End the session and close the connection.
 * @param session {Session} the open session
 */
export func quit(session as Session) {
    # The socket is shut even when the QUIT dialogue throws (a dead server
    # must not leak the fd).
    defer net.close($session.conn);
    command($session, ["QUIT"]);
}

# --- pub/sub (exported) --------------------------------------------

# Every (un)subscribe call ends its write with `PING <token>`, a barrier: Redis
# answers a connection's commands in order, so once the PING's reply is read,
# every confirmation of the call has arrived. A token is this prefix plus a
# random number, so a waiter tells its own PING reply from another's.
def const TOKEN_PREFIX as string init "jennifer-redis-";

# barrierToken is a fresh PING argument for one (un)subscribe call.
func barrierToken() {
    return TOKEN_PREFIX + convert.toString(crypto.randInt(0, 9007199254740991));
}

# tokenFromReply returns the barrier token a PING reply carries: `["pong",
# token]` on a subscribed connection, the bulk `token` once it has left
# subscribed mode. Any other reply yields "". Pure.
func tokenFromReply(reply as Reply) {
    if ($reply.kind == "array" and len($reply.items) == 2 and $reply.items[0].str == "pong") {
        return $reply.items[1].str;
    }
    if ($reply.kind == "string" and strings.startsWith($reply.str, TOKEN_PREFIX)) {
        return $reply.str;
    }
    return "";
}

# newSession wraps an open socket as a Session with an empty pub/sub state.
func newSession(conn as net.Conn, timeoutMs as int) {
    def pubsub as channel of PubSub init channel.make(1);
    channel.send($pubsub, PubSub{buf: emptyBytes(), queue: [], tokens: []});
    return Session{conn: $conn, timeoutMs: $timeoutMs, pubsub: $pubsub};
}

# readPushed reads the next reply of a subscribed connection, starting with the
# bytes `st.buf` holds, and returns it with the bytes read past it (the new
# `st.buf`). The caller holds the state; on a failed read it is handed back
# before the error propagates, so a timed-out receive leaves the session usable.
func readPushed(session as Session, st as PubSub) {
    try {
        return readOneValue($session.conn, $st.buf, $session.timeoutMs);
    } catch (e) {
        channel.send($session.pubsub, $st);
        throw $e;
    }
}

# subscribeCommand sends a (P)SUBSCRIBE / (P)UNSUBSCRIBE and a barrier PING in
# one write and returns once the PING's reply is read, so the server has acted
# on the command. A message pushed meanwhile is queued for `receiveMessage`, in
# order; a barrier reply another caller waits for is left for it. A server error
# reply throws once the barrier is reached, so the connection stays in step.
func subscribeCommand(session as Session, verb as string, names as list of string) {
    def token as string init barrierToken();
    def wire as string init encodePipeline([subscribeArgs($verb, $names), ["PING", $token]]);
    net.writeBytes($session.conn, encoding.encode("utf-8", $wire));
    def failed as bool init false;
    def failure as string init "";
    while (true) {
        def st as PubSub init channel.recv($session.pubsub);
        def at as int init lists.indexOf($st.tokens, $token);
        if ($at >= 0) {
            # Another reader took this call's barrier reply off the socket.
            $st.tokens = lists.concat($st.tokens[..$at], $st.tokens[$at + 1..]);
            channel.send($session.pubsub, $st);
            break;
        }
        def rv as ReadValue init readPushed($session, $st);
        $st.buf = $rv.rest;
        def reply as Reply init $rv.reply;
        if ($reply.kind == "error") {
            if (not $failed) {
                $failed = true;
                $failure = $reply.str;
            }
        } else {
            def msg as Message init messageFromReply($reply);
            def got as string init tokenFromReply($reply);
            if ($msg.kind == "message" or $msg.kind == "pmessage") {
                $st.queue[] = $msg;
            } elseif ($got == $token) {
                channel.send($session.pubsub, $st);
                break;
            } elseif ($got != "") {
                $st.tokens[] = $got;
            }
        }
        channel.send($session.pubsub, $st);
    }
    if ($failed) {
        throw replyError($failure);
    }
}

/**
 * Subscribe to one or more channels and wait until the server has confirmed
 * it, so a message published once this returns is delivered. After
 * subscribing, the connection is in subscribed mode and may only run
 * (P)SUBSCRIBE / (P)UNSUBSCRIBE / PING / QUIT until fully unsubscribed; read
 * pushes with `receiveMessage`. A message that arrives while this waits (on a
 * connection already subscribed elsewhere) is kept for `receiveMessage`.
 * @param session {Session} the open session
 * @param channels {list of string} the channels to subscribe to
 * @throws {RedisError} kind "redis" on a server error reply (an empty list is one)
 */
export func subscribe(session as Session, channels as list of string) {
    subscribeCommand($session, "SUBSCRIBE", $channels);
}

/**
 * Subscribe to one or more glob patterns (e.g. "news.*") and wait until the
 * server has confirmed it, as `subscribe` does.
 * @param session {Session} the open session
 * @param patterns {list of string} the patterns to subscribe to
 * @throws {RedisError} kind "redis" on a server error reply (an empty list is one)
 */
export func psubscribe(session as Session, patterns as list of string) {
    subscribeCommand($session, "PSUBSCRIBE", $patterns);
}

/**
 * Unsubscribe from the given channels, or from all channels when the list is
 * empty, and wait until the server has confirmed it: no message from those
 * channels arrives after this returns (one that arrived before stays queued for
 * `receiveMessage`).
 * @param session {Session} the open session
 * @param channels {list of string} the channels to leave ([] means all)
 * @throws {RedisError} kind "redis" on a server error reply
 */
export func unsubscribe(session as Session, channels as list of string) {
    subscribeCommand($session, "UNSUBSCRIBE", $channels);
}

/**
 * Unsubscribe from the given patterns, or from all patterns when the list is
 * empty, and wait until the server has confirmed it, as `unsubscribe` does.
 * @param session {Session} the open session
 * @param patterns {list of string} the patterns to leave ([] means all)
 * @throws {RedisError} kind "redis" on a server error reply
 */
export func punsubscribe(session as Session, patterns as list of string) {
    subscribeCommand($session, "PUNSUBSCRIBE", $patterns);
}

/**
 * Publish `message` to `channel` and return the number of subscribers that
 * received it. Runs on an ordinary (non-subscribed) connection.
 * @param session {Session} the open session
 * @param channel {string} the channel to publish to
 * @param message {string} the message body
 * @return {int} the number of subscribers that received the message
 */
export func publish(session as Session, channel as string, message as string) {
    return command($session, publishArgs($channel, $message)).num;
}

/**
 * Block until the next `message` / `pmessage` push arrives, skipping the
 * (un)subscribe confirmation frames. Messages come back in the order the server
 * sent them, including those read while a `subscribe` waited for its
 * confirmation and those that arrived in the same read as an earlier one. Wrap
 * the call in a `spawn` to receive concurrently with the rest of the program
 * (the session's copies share their read state, so another task may
 * `subscribe` / `unsubscribe` meanwhile); the per-read idle timeout
 * (`Session.timeoutMs`, milliseconds) bounds the wait and raises a catchable
 * error on a stall.
 * @param session {Session} the open, subscribed session
 * @return {Message} the next channel / pattern push
 * @throws {RedisError} kind "redis" on a server error reply; kind "closed" when the server closes the connection
 */
export func receiveMessage(session as Session) {
    while (true) {
        def st as PubSub init channel.recv($session.pubsub);
        if (len($st.queue) > 0) {
            def first as Message init $st.queue[0];
            $st.queue = $st.queue[1..];
            channel.send($session.pubsub, $st);
            return $first;
        }
        def rv as ReadValue init readPushed($session, $st);
        $st.buf = $rv.rest;
        def reply as Reply init $rv.reply;
        if ($reply.kind == "error") {
            channel.send($session.pubsub, $st);
            throw replyError($reply.str);
        }
        def msg as Message init messageFromReply($reply);
        if ($msg.kind == "message" or $msg.kind == "pmessage") {
            channel.send($session.pubsub, $st);
            return $msg;
        }
        def got as string init tokenFromReply($reply);
        if ($got != "") {
            $st.tokens[] = $got;
        }
        channel.send($session.pubsub, $st);
    }
    return Message{kind: "", channel: "", pattern: "", payload: ""};
}

# --- pipelining (exported) -----------------------------------------

/**
 * Send several commands in one write and read exactly one reply per command
 * (a single network round trip). Unlike `command`, a `-ERR` reply does not
 * throw - each command's outcome is returned in order, so inspect each `Reply`.
 * @param session {Session} the open session
 * @param commands {list of list of string} each command as an argument list
 * @return {list of Reply} one reply per command, in order
 */
export func pipeline(session as Session, commands as list of list of string) {
    net.writeBytes($session.conn, encoding.encode("utf-8", encodePipeline($commands)));
    # Read all N replies buffered - the server may return them coalesced in one
    # TCP segment, which a per-reply read would drop past the first.
    return readReplies($session.conn, $session.timeoutMs, len($commands));
}

# --- transactions (exported) ---------------------------------------

/**
 * Begin a transaction (MULTI). Commands issued after this are queued (each
 * replies "+QUEUED") until `exec` runs them atomically or `discard` drops them.
 * @param session {Session} the open session
 */
export func multi(session as Session) {
    command($session, ["MULTI"]);
}

/**
 * Execute the queued transaction (EXEC) and return one reply per queued
 * command, in order. An aborted transaction (a `nil` EXEC reply) yields an
 * empty list.
 * @param session {Session} the open session
 * @return {list of Reply} the queued commands' replies
 */
export func exec(session as Session) {
    def reply as Reply init command($session, ["EXEC"]);
    def out as list of Reply init [];
    if ($reply.kind == "array") {
        for (def item in $reply.items) {
            $out[] = $item;
        }
    }
    return $out;
}

/**
 * Discard the queued transaction (DISCARD).
 * @param session {Session} the open session
 */
export func discard(session as Session) {
    command($session, ["DISCARD"]);
}

# --- scan (exported) -----------------------------------------------

/**
 * Walk the keyspace one page at a time (SCAN), the production-safe iterator:
 * start with `cursor` 0 and repeat until the returned cursor is 0 again. Pass
 * `pattern` "" to skip the MATCH glob and `count` 0 to skip the COUNT hint.
 * Prefer this over `keys`, which blocks the server on a large keyspace.
 * @param session {Session} the open session
 * @param cursor {int} the cursor (0 to begin)
 * @param pattern {string} a glob to filter keys ("" for no filter)
 * @param count {int} a per-page size hint (0 for the server default)
 * @return {ScanResult} the next cursor and this page's keys
 */
export func scan(session as Session, cursor as int, pattern as string, count as int) {
    def args as list of string init ["SCAN", convert.toString($cursor)];
    if (len($pattern) > 0) {
        $args[] = "MATCH";
        $args[] = $pattern;
    }
    if ($count > 0) {
        $args[] = "COUNT";
        $args[] = convert.toString($count);
    }
    return scanResultFromReply(command($session, $args));
}
