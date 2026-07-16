# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

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
 * def db as redis.Session init redis.connect(redis.Options{host: "127.0.0.1", port: 6379, security: "none", user: "", password: "", db: 0});
 * redis.set($db, "greeting", "hello");
 * io.printf("%s\n", redis.get($db, "greeting"));
 * redis.quit($db);
 */
use net;
use strings;
use convert;

/**
 * Connection settings.
 * @field host {string} the server host
 * @field port {int} the server port
 * @field security {string} "none" (plaintext) or "tls" (rediss)
 * @field user {string} the AUTH username ("" for password-only or no auth)
 * @field password {string} the AUTH password; "" skips AUTH
 * @field db {int} the database to SELECT (0 is the default)
 */
export def struct Options {
    host as string,
    port as int,
    security as string,
    user as string,
    password as string,
    db as int
};

# The default per-read idle timeout (ms), so a hung server fails instead of
# blocking forever. `connect` sets `Session.timeout`; override it or use 0 to disable.
def const DEFAULT_TIMEOUT_MS as int init 30000;

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
    def out as bytes;
    def i as int init $start;
    while ($i < $end) {
        $out[] = $buf[$i];
        $i = $i + 1;
    }
    return $out;
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
# Bulk lengths are byte counts.
func encodeCommand(args as list of string) {
    def out as string init "*" + convert.toString(len($args)) + "\r\n";
    for (def arg in $args) {
        def blen as int init len(convert.bytesFromString($arg, "utf-8"));
        $out = $out + "$" + convert.toString($blen) + "\r\n" + $arg + "\r\n";
    }
    return $out;
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
    if ($typ == 43) {          # '+'
        return done(replyStr("string", $payload), $after);
    }
    if ($typ == 45) {          # '-'
        return done(replyStr("error", $payload), $after);
    }
    if ($typ == 58) {          # ':'
        return done(replyInt(convert.toInt($payload)), $after);
    }
    if ($typ == 36) {          # '$'
        return parseBulkAt($payload, $buf, $after);
    }
    if ($typ == 42) {          # '*'
        return parseArrayAt($payload, $buf, $after);
    }
    # Unknown type byte: surface the whole line as a string.
    def line as string init convert.stringFromBytes(byteSlice($buf, $pos, $nl), "utf-8");
    return done(replyStr("string", $line), $after);
}

# parseComplete parses one RESP value from the front of `buf` (offset 0).
func parseComplete(buf as bytes) {
    return parseAt($buf, 0);
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
    }
    return replyNil();
}

func dial(opts as Options) {
    def addr as string init $opts.host + ":" + convert.toString($opts.port);
    if ($opts.security == "tls") {
        return net.connectTLS($addr);
    }
    return net.connect($addr);
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
    command($session, ["QUIT"]);
    net.close($session.conn);
}
