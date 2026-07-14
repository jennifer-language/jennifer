# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

/**
 * A Redis client speaking RESP2 (the REdis Serialization Protocol) over the
 * `net` system library. Commands go out as RESP arrays of bulk strings; replies
 * (`+OK`, `-ERR`, `:int`, `$bulk`, `*array`) parse back into a `Reply`. Typed
 * per-command helpers (`get` / `set` / `incr` / `keys` / ...) keep the common
 * path fully typed; `command` is the generic escape hatch for anything else. A
 * `-ERR` reply throws a catchable `Error` (kind "redis"). Bulk strings are read
 * as UTF-8 text: byte-exact for ASCII / UTF-8 values, but a binary value whose
 * byte length differs from its rune length is not yet byte-exact. Needs the
 * default `jennifer` binary (uses `net`).
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

# One parse step's result: the value, the unconsumed buffer, and whether the
# buffer held a complete value.
def struct ParseResult {
    reply as Reply,
    rest as string,
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

func done(reply as Reply, rest as string) {
    return ParseResult{reply: $reply, rest: $rest, complete: true};
}

func incomplete() {
    return ParseResult{reply: replyNil(), rest: "", complete: false};
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

# parseBulk parses a `$`-length bulk string from `rest`, or reports incomplete.
func parseBulk(payload as string, rest as string) {
    def n as int init convert.toInt($payload);
    if ($n < 0) {
        return done(replyNil(), $rest);
    }
    if (len($rest) < $n + 2) {
        return incomplete();
    }
    def data as string init strings.substring($rest, 0, $n);
    return done(replyStr("string", $data), strings.substring($rest, $n + 2));
}

# parseArray parses a `*`-count array, recursing per element, from `rest`.
func parseArray(payload as string, rest as string) {
    def count as int init convert.toInt($payload);
    if ($count < 0) {
        return done(replyNil(), $rest);
    }
    def items as list of Reply init [];
    def cur as string init $rest;
    def i as int init 0;
    while ($i < $count) {
        def pr as ParseResult init parseComplete($cur);
        if (not $pr.complete) {
            return incomplete();
        }
        $items[] = $pr.reply;
        $cur = $pr.rest;
        $i = $i + 1;
    }
    return done(replyArray($items), $cur);
}

# parseComplete parses one RESP value from the front of `buf`. `complete` is
# false when `buf` does not yet hold the whole value.
func parseComplete(buf as string) {
    def nl as int init strings.indexOf($buf, "\r\n");
    if ($nl < 0) {
        return incomplete();
    }
    def line as string init strings.substring($buf, 0, $nl);
    def rest as string init strings.substring($buf, $nl + 2);
    def typ as string init strings.substring($line, 0, 1);
    def payload as string init strings.substring($line, 1, len($line));
    if ($typ == "+") {
        return done(replyStr("string", $payload), $rest);
    }
    if ($typ == "-") {
        return done(replyStr("error", $payload), $rest);
    }
    if ($typ == ":") {
        return done(replyInt(convert.toInt($payload)), $rest);
    }
    if ($typ == "$") {
        return parseBulk($payload, $rest);
    }
    if ($typ == "*") {
        return parseArray($payload, $rest);
    }
    return done(replyStr("string", $line), $rest);
}

# --- net dialogue (private) ----------------------------------------

# readReply reads bytes until a complete RESP reply has arrived, then returns it.
# `timeoutMs` re-arms a read deadline before each read (0 disables it).
func readReply(conn as net.Conn, timeoutMs as int) {
    def buf as string init "";
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
        $buf = $buf + convert.stringFromBytes($chunk, "utf-8");
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
