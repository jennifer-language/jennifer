# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
# pragma-jennifer-version: >=0.25.0
# pragma-jennifer-capability: net

/**
 * A Redis client speaking RESP2 (the REdis Serialization Protocol) over the
 * `net` system library. Commands go out as RESP arrays of bulk strings; replies
 * (`+OK`, `-ERR`, `:int`, `$bulk`, `*array`) parse back into a `Reply`. Typed
 * per-command helpers (`get` / `set` / `incr` / `keys` / ...) keep the common
 * path fully typed; `command` is the generic escape hatch for anything else. A
 * `-ERR` reply throws a catchable `Error` (kind "redis"). The reply parser
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
use net;
use binary;
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
# blocking forever. `connect` sets `Session.timeout`; override it or use 0 to disable.
def const DEFAULT_TIMEOUT_MS as int init 30000;

# MAX_REPLY_BYTES caps a single accumulated reply. A malicious / compromised
# server can declare an enormous bulk-string length (or simply never terminate a
# reply), which would grow the read buffer without bound; this fails the read
# with a catchable error instead.
def const MAX_REPLY_BYTES as int init 67108864;

# capReply throws when an accumulated reply has grown past the cap.
func capReply(n as int) {
    if ($n > MAX_REPLY_BYTES) {
        throw Error{
            kind: "redis",
            message: "redis: reply exceeds the " + convert.toString(MAX_REPLY_BYTES) + "-byte limit",
            file: "",
            line: 0,
            col: 0
        };
    }
    return;
}

/**
 * An open Redis connection.
 * @field conn {net.Conn} the underlying socket
 * @field timeout {int} per-read idle timeout in milliseconds (0 disables it)
 */
export def struct Session {
    conn as net.Conn,
    timeout as int
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

# byteSlice returns buf[start:end] as a fresh bytes value.
func byteSlice(buf as bytes, start as int, end as int) {
    return binary.slice($buf, $start, $end);
}

# crlfIndex returns the index of the CR of the first CRLF at or after `from`,
# or -1 if none is present yet.
func crlfIndex(buf as bytes, from as int) {
    def i as int init $from;
    def n as int init len($buf);
    while ($i + 1 < $n) {
        if ($buf[$i] == 13 and $buf[$i + 1] == 10) {
            return $i;
        }
        $i = $i + 1;
    }
    return -1;
}

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
        def blen as int init len(convert.bytesFromString($arg, "utf-8"));
        $parts[] = "$" + convert.toString($blen) + "\r\n" + $arg + "\r\n";
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
    def data as bytes init byteSlice($buf, $pos, $pos + $n);
    return done(replyStr("string", convert.stringFromBytes($data, "utf-8")), $pos + $n + 2);
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
    def nl as int init crlfIndex($buf, $pos);
    if ($nl < 0) {
        return incomplete();
    }
    def typ as int init $buf[$pos];
    def payload as string init convert.stringFromBytes(byteSlice($buf, $pos + 1, $nl), "utf-8");
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
            def line as string init convert.stringFromBytes(byteSlice($buf, $pos, $nl), "utf-8");
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
    def crlf as bytes init convert.bytesFromString("\r\n", "utf-8");
    def parts as list of bytes init [];
    $parts[] = convert.bytesFromString(
        "*" + convert.toString(len($args)) + "\r\n", "utf-8");
    for (def arg in $args) {
        def hdr as string init "$" + convert.toString(len($arg)) + "\r\n";
        $parts[] = convert.bytesFromString($hdr, "utf-8");
        $parts[] = $arg;
        $parts[] = $crlf;
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
    def head as bytes;
    def nl as int init -1;
    while ($nl < 0) {
        if ($timeoutMs > 0) {
            net.setDeadline($conn, $timeoutMs);
        }
        def chunk as bytes init net.readBytes($conn, 4096);
        if (len($chunk) == 0) {
            throw Error{
                kind: "redis",
                message: "redis: connection closed before a complete reply",
                file: "",
                line: 0,
                col: 0
            };
        }
        $head = binary.concat($head, $chunk);
        capReply(len($head));
        $nl = crlfIndex($head, 0);
    }

    def typ as int init $head[0];
    def header as string init convert.stringFromBytes(byteSlice($head, 1, $nl), "utf-8");
    if ($typ == 45) { # '-' error
        throw Error{kind: "redis", message: $header, file: "", line: 0, col: 0};
    }
    if ($typ != 36) {
        # A '+' simple string or ':' integer reply: return the line's payload
        # bytes (unusual for a value read, but never decode-throws).
        return byteSlice($head, 1, $nl);
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
    def have as bytes init byteSlice($head, $start, len($head));
    def need as int init $n + 2 - len($have);
    if ($need > 0) {
        def more as bytes;
        if ($timeoutMs > 0) {
            $more = net.readN($conn, $need, $timeoutMs);
        } else {
            $more = net.readN($conn, $need);
        }
        $have = binary.concat($have, $more);
    }
    return byteSlice($have, 0, $n);
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

# readReply reads bytes until a complete RESP reply has arrived, then returns it.
# `timeoutMs` re-arms a read deadline before each read (0 disables it).
func readReply(conn as net.Conn, timeoutMs as int) {
    def buf as bytes;
    while (true) {
        def pr as ParseResult init parseComplete($buf);
        if ($pr.complete) {
            return $pr.reply;
        }
        if ($timeoutMs > 0) {
            net.setDeadline($conn, $timeoutMs);
        }
        def chunk as bytes init net.readBytes($conn, 1024);
        if (len($chunk) == 0) {
            return $pr.reply;
        }
        # Append the raw chunk into the byte buffer (never round-trip through a
        # string mid-stream: a chunk boundary can fall inside a multi-byte
        # sequence, and stringFromBytes on a partial rune would corrupt it).
        def j as int init 0;
        while ($j < len($chunk)) {
            $buf[] = $chunk[$j];
            $j = $j + 1;
        }
        capReply(len($buf));
    }
    return replyNil();
}

# readReplies reads exactly `count` complete replies, buffering across socket
# reads so that replies the server coalesced into one TCP segment are all parsed.
# A single `readReply` per reply drops any bytes past the first complete reply,
# which silently loses the rest of a pipelined batch (and blocks the next read).
# `ParseResult.pos` gives the bytes one reply consumed, so the leftover is kept
# and re-parsed. Over-read is the only hazard here - the next request has not been
# sent - so buffering within this one call is sufficient.
func readReplies(conn as net.Conn, timeoutMs as int, count as int) {
    def replies as list of Reply init [];
    def buf as bytes;
    while (len($replies) < $count) {
        def pr as ParseResult init parseComplete($buf);
        if ($pr.complete) {
            $replies[] = $pr.reply;
            $buf = $buf[$pr.pos..];
            continue;
        }
        if ($timeoutMs > 0) {
            net.setDeadline($conn, $timeoutMs);
        }
        def chunk as bytes init net.readBytes($conn, 4096);
        if (len($chunk) == 0) {
            throw Error{
                kind: "redis",
                message: "redis: connection closed after " + convert.toString(len($replies)) +
                    " of " + convert.toString($count) + " replies",
                file: "",
                line: 0,
                col: 0
            };
        }
        def k as int init 0;
        while ($k < len($chunk)) {
            $buf[] = $chunk[$k];
            $k = $k + 1;
        }
        capReply(len($buf));
    }
    return $replies;
}

func dial(opts as Options) {
    def addr as string init $opts.host + ":" + convert.toString($opts.port);
    match ($opts.security) {
        when Tls { return net.connectTLS($addr, DEFAULT_TIMEOUT_MS); }
        when None { return net.connect($addr, DEFAULT_TIMEOUT_MS); }
        when Starttls {
            throw Error{kind: "redis", message: "redis: STARTTLS is not supported; use transport.Security.Tls (rediss) or .None", file: "", line: 0, col: 0};
        }
    }
}

# --- commands (exported) -------------------------------------------

/**
 * Send one command (its arguments) and return the reply.
 * @param session {Session} the open session
 * @param args {list of string} the command name and its arguments
 * @return {Reply} the parsed reply
 * @throws {Error} kind "redis" on a `-ERR` reply
 */
export func command(session as Session, args as list of string) {
    net.writeBytes($session.conn, convert.bytesFromString(encodeCommand($args), "utf-8"));
    def reply as Reply init readReply($session.conn, $session.timeout);
    if ($reply.kind == "error") {
        throw Error{kind: "redis", message: $reply.str, file: "", line: 0, col: 0};
    }
    return $reply;
}

/**
 * Open a session, authenticating and selecting a database when set.
 * @param opts {Options} the connection settings
 * @return {Session} the open session
 */
export func connect(opts as Options) {
    def session as Session init Session{conn: dial($opts), timeout: DEFAULT_TIMEOUT_MS};
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
 * @throws {Error} kind "redis" on a `-ERR` reply
 */
export func setBytes(session as Session, key as string, value as bytes) {
    def args as list of bytes init [
        convert.bytesFromString("SET", "utf-8"),
        convert.bytesFromString($key, "utf-8"),
        $value
    ];
    net.writeBytes($session.conn, encodeCommandBytes($args));
    def reply as Reply init readReply($session.conn, $session.timeout);
    if ($reply.kind == "error") {
        throw Error{kind: "redis", message: $reply.str, file: "", line: 0, col: 0};
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
 * @throws {Error} kind "redis" on a `-ERR` reply
 */
export func getBytes(session as Session, key as string) {
    net.writeBytes($session.conn, convert.bytesFromString(encodeCommand(["GET", $key]), "utf-8"));
    return readBulkReply($session.conn, $session.timeout);
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

# writeCommand sends a command without reading its reply - the shape a
# subscribed connection needs, since (un)subscribe confirmations arrive
# interleaved with pushes and are drained by `receiveMessage`.
func writeCommand(session as Session, args as list of string) {
    net.writeBytes($session.conn, convert.bytesFromString(encodeCommand($args), "utf-8"));
}

/**
 * Subscribe to one or more channels. After subscribing, the connection is in
 * subscribed mode and may only run (P)SUBSCRIBE / (P)UNSUBSCRIBE / PING / QUIT
 * until fully unsubscribed; read pushes with `receiveMessage`.
 * @param session {Session} the open session
 * @param channels {list of string} the channels to subscribe to
 */
export func subscribe(session as Session, channels as list of string) {
    writeCommand($session, subscribeArgs("SUBSCRIBE", $channels));
}

/**
 * Subscribe to one or more glob patterns (e.g. "news.*").
 * @param session {Session} the open session
 * @param patterns {list of string} the patterns to subscribe to
 */
export func psubscribe(session as Session, patterns as list of string) {
    writeCommand($session, subscribeArgs("PSUBSCRIBE", $patterns));
}

/**
 * Unsubscribe from the given channels, or from all channels when the list is
 * empty.
 * @param session {Session} the open session
 * @param channels {list of string} the channels to leave ([] means all)
 */
export func unsubscribe(session as Session, channels as list of string) {
    writeCommand($session, subscribeArgs("UNSUBSCRIBE", $channels));
}

/**
 * Unsubscribe from the given patterns, or from all patterns when the list is
 * empty.
 * @param session {Session} the open session
 * @param patterns {list of string} the patterns to leave ([] means all)
 */
export func punsubscribe(session as Session, patterns as list of string) {
    writeCommand($session, subscribeArgs("PUNSUBSCRIBE", $patterns));
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
 * (un)subscribe confirmation frames. Wrap the call in a `spawn` to receive
 * concurrently with the rest of the program; the per-read idle timeout
 * (`Session.timeout`, milliseconds) bounds the wait and raises a catchable
 * error on a stall.
 * @param session {Session} the open, subscribed session
 * @return {Message} the next channel / pattern push
 * @throws {Error} kind "redis" on a server error reply or a closed connection
 */
export func receiveMessage(session as Session) {
    # Buffer across reads and parse each reply out of the buffer, so a subscribe
    # confirmation and the first pushed message coalesced into one TCP segment are
    # both handled (a per-reply read would drop the message with the leftover and
    # then block). NOTE: a message coalesced *after* the one returned - rapid
    # consecutive pushes landing in a single read - can still be lost across calls,
    # since a value-semantic Session cannot retain the leftover buffer; a buffered
    # `net` reader is the follow-up for that tail case.
    def buf as bytes;
    while (true) {
        def pr as ParseResult init parseComplete($buf);
        if ($pr.complete) {
            $buf = $buf[$pr.pos..];
            def reply as Reply init $pr.reply;
            if ($reply.kind == "error") {
                throw Error{kind: "redis", message: $reply.str, file: "", line: 0, col: 0};
            }
            def msg as Message init messageFromReply($reply);
            if ($msg.kind == "message" or $msg.kind == "pmessage") {
                return $msg;
            }
            continue;
        }
        if ($session.timeout > 0) {
            net.setDeadline($session.conn, $session.timeout);
        }
        def chunk as bytes init net.readBytes($session.conn, 4096);
        if (len($chunk) == 0) {
            throw Error{
                kind: "redis",
                message: "redis: connection closed while awaiting a message",
                file: "",
                line: 0,
                col: 0
            };
        }
        def k as int init 0;
        while ($k < len($chunk)) {
            $buf[] = $chunk[$k];
            $k = $k + 1;
        }
        capReply(len($buf));
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
    net.writeBytes($session.conn, convert.bytesFromString(encodePipeline($commands), "utf-8"));
    # Read all N replies buffered - the server may return them coalesced in one
    # TCP segment, which a per-reply read would drop past the first.
    return readReplies($session.conn, $session.timeout, len($commands));
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
