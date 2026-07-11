# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# redis.j - a Redis client speaking RESP2 (the REdis Serialization Protocol)
# over the `net` system library. Commands go out as RESP arrays of bulk
# strings; replies (`+OK`, `-ERR`, `:int`, `$bulk`, `*array`) parse back into a
# `Reply`. Typed per-command helpers (`get` / `set` / `incr` / `keys` / ...)
# keep the common path fully typed; `command` is the generic escape hatch for
# anything else. Because it uses `net`, this module needs the default
# `jennifer` binary.
#
#     import "redis.j" as redis;
#     def db as redis.Session init redis.connect(redis.Options{host: "127.0.0.1",
#         port: 6379, security: "none", user: "", password: "", db: 0});
#     redis.set($db, "greeting", "hello");
#     io.printf("%s\n", redis.get($db, "greeting"));      # hello
#     redis.quit($db);
#
# A `-ERR` reply throws a catchable `Error` (kind "redis"). Bulk strings are
# read as UTF-8 text: byte-exact for ASCII / UTF-8 values, but a binary value
# whose byte length differs from its rune length is not yet byte-exact.
use net;
use strings;
use convert;

# Connection settings. `security` is "none" (plaintext) or "tls" (rediss).
# `password` "" skips AUTH; `db` selects a database (0 is the default).
export def struct Options {
    host as string,
    port as int,
    security as string,
    user as string,
    password as string,
    db as int
};

export def struct Session {
    conn as net.Conn
};

# A parsed RESP reply. `kind` is "string" (simple or bulk), "error", "int",
# "nil", or "array"; `str` holds a string / error, `num` an integer, `items`
# an array's elements.
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
func readReply(conn as net.Conn) {
    def buf as string init "";
    while (true) {
        def pr as ParseResult init parseComplete($buf);
        if ($pr.complete) {
            return $pr.reply;
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

# command sends one command (its arguments) and returns the reply. A `-ERR`
# reply throws a catchable `Error` (kind "redis").
export func command(session as Session, args as list of string) {
    net.writeBytes($session.conn, convert.bytesFromString(encodeCommand($args), "utf-8"));
    def reply as Reply init readReply($session.conn);
    if ($reply.kind == "error") {
        throw Error{kind: "redis", message: $reply.str, file: "", line: 0, col: 0};
    }
    return $reply;
}

# connect opens a session, authenticating and selecting a database when set.
export func connect(opts as Options) {
    def session as Session init Session{conn: dial($opts)};
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

# get returns the string value of `key`, or "" when the key is missing.
export func get(session as Session, key as string) {
    return command($session, ["GET", $key]).str;
}

# set stores `value` at `key`.
export func set(session as Session, key as string, value as string) {
    command($session, ["SET", $key, $value]);
}

# del deletes `key` and returns the number of keys removed (0 or 1).
export func del(session as Session, key as string) {
    return command($session, ["DEL", $key]).num;
}

# exists reports whether `key` is present.
export func exists(session as Session, key as string) {
    return command($session, ["EXISTS", $key]).num > 0;
}

# incr atomically increments `key` and returns the new value.
export func incr(session as Session, key as string) {
    return command($session, ["INCR", $key]).num;
}

# decr atomically decrements `key` and returns the new value.
export func decr(session as Session, key as string) {
    return command($session, ["DECR", $key]).num;
}

# keys returns the keys matching a glob `pattern` (e.g. "*", "user:*").
export func keys(session as Session, pattern as string) {
    def out as list of string init [];
    for (def item in command($session, ["KEYS", $pattern]).items) {
        $out[] = $item.str;
    }
    return $out;
}

# ping returns the server's PONG (a liveness check).
export func ping(session as Session) {
    return command($session, ["PING"]).str;
}

# quit ends the session and closes the connection.
export func quit(session as Session) {
    command($session, ["QUIT"]);
    net.close($session.conn);
}
