# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

/**
 * A client for a memcached server, speaking its classic text protocol over the
 * `net` system library. Store with an expiration (`set` / `add`), read (`get`),
 * remove (`delete`), count atomically (`incr` / `decr`), and re-arm a key's
 * expiry (`touch`). memcached is a volatile cache - keys expire on their
 * `exptime` and the server evicts under memory pressure - so it suits sessions,
 * rate limits, and derived data, not a system of record. `exptime` is seconds
 * (0 = never expire, until evicted). A protocol error (`ERROR` / `CLIENT_ERROR`
 * / `SERVER_ERROR`) throws a catchable `Error` (kind "memcache"). Values are
 * read as UTF-8 text: byte-exact for ASCII / UTF-8 values (the common case); a
 * binary value whose byte length differs from its rune length is not yet
 * byte-exact. Needs the default `jennifer` binary (uses `net`).
 * @module memcache
 * @example
 * def mc as memcache.Session init memcache.connect(memcache.Options{host: "127.0.0.1", port: 11211});
 * memcache.set($mc, "greeting", "hello", 60);
 * io.printf("%s\n", memcache.get($mc, "greeting"));
 * memcache.quit($mc);
 */
use net;
use strings;
use convert;

/**
 * Connection settings (plaintext; memcached's text protocol has no auth / TLS).
 * @field host {string} the server host
 * @field port {int} the server port
 */
export def struct Options {
    host as string,
    port as int
};

# The default per-read idle timeout (ms), so a hung server fails instead of
# blocking forever. `connect` sets `Session.timeout`; override it or use 0 to disable.
def const DEFAULT_TIMEOUT_MS as int init 30000;

/**
 * An open memcached connection.
 * @field conn {net.Conn} the underlying socket
 * @field timeout {int} per-read idle timeout in milliseconds (0 disables it)
 */
export def struct Session {
    conn as net.Conn,
    timeout as int
};

# One CRLF-terminated protocol line plus the still-buffered remainder.
def struct Line {
    text as string,
    rest as string
};

# --- wire helpers (private) ----------------------------------------

func writeCmd(session as Session, text as string) {
    net.writeBytes($session.conn, convert.bytesFromString($text, "utf-8"));
}

# recvLine reads one CRLF-terminated line, returning it (without the CRLF) and
# the buffer left over after it.
func recvLine(session as Session, buf as string) {
    def b as string init $buf;
    while (true) {
        def nl as int init strings.indexOf($b, "\r\n");
        if ($nl >= 0) {
            return Line{text: strings.substring($b, 0, $nl),
                rest: strings.substring($b, $nl + 2)};
        }
        if ($session.timeout > 0) {
            net.setDeadline($session.conn, $session.timeout);
        }
        def chunk as bytes init net.readBytes($session.conn, 1024);
        if (len($chunk) == 0) {
            return Line{text: $b, rest: ""};
        }
        $b = $b + convert.stringFromBytes($chunk, "utf-8");
    }
    return Line{text: "", rest: ""};
}

# fillBytes reads until the buffer holds at least `n` bytes.
func fillBytes(session as Session, buf as string, n as int) {
    def b as string init $buf;
    while (len($b) < $n) {
        if ($session.timeout > 0) {
            net.setDeadline($session.conn, $session.timeout);
        }
        def chunk as bytes init net.readBytes($session.conn, 1024);
        if (len($chunk) == 0) {
            return $b;
        }
        $b = $b + convert.stringFromBytes($chunk, "utf-8");
    }
    return $b;
}

# fail throws a catchable memcache error.
func fail(message as string) {
    throw Error{kind: "memcache", message: $message, file: "", line: 0, col: 0};
}

# checkError throws on a protocol-error reply line.
func checkError(line as string) {
    if (strings.startsWith($line, "ERROR")) {
        fail($line);
    }
    if (strings.startsWith($line, "CLIENT_ERROR")) {
        fail($line);
    }
    if (strings.startsWith($line, "SERVER_ERROR")) {
        fail($line);
    }
}

# storeHeader builds a storage command's first line (`verb key flags exptime
# bytes`); flags are unused (0) and `bytes` is the value's UTF-8 byte length.
func storeHeader(verb as string, key as string, value as string, exptime as int) {
    def n as int init len(convert.bytesFromString($value, "utf-8"));
    return $verb + " " + $key + " 0 " + convert.toString($exptime) + " " + convert.toString($n);
}

# store runs a storage command and returns its reply line.
func store(session as Session, verb as string, key as string, value as string, exptime as int) {
    writeCmd($session, storeHeader($verb, $key, $value, $exptime) + "\r\n" + $value + "\r\n");
    def resp as Line init recvLine($session, "");
    checkError($resp.text);
    return $resp.text;
}

# --- commands (exported) -------------------------------------------

/**
 * Open a session to the memcached server.
 * @param opts {Options} the connection settings
 * @return {Session} the open session
 */
export func connect(opts as Options) {
    def addr as string init $opts.host + ":" + convert.toString($opts.port);
    return Session{conn: net.connect($addr), timeout: DEFAULT_TIMEOUT_MS};
}

/**
 * Store `value` at `key` with an `exptime`-second TTL, replacing any existing
 * value.
 * @param session {Session} the open session
 * @param key {string} the key to write
 * @param value {string} the value to store
 * @param exptime {int} the TTL in seconds (0 = never expire, until evicted)
 * @throws {Error} kind "memcache" on a non-STORED reply
 */
export func set(session as Session, key as string, value as string, exptime as int) {
    def r as string init store($session, "set", $key, $value, $exptime);
    if (not ($r == "STORED")) {
        fail($r);
    }
}

/**
 * Store `value` at `key` only if the key is absent. The atomic build block for
 * locks and "create if new".
 * @param session {Session} the open session
 * @param key {string} the key to write
 * @param value {string} the value to store
 * @param exptime {int} the TTL in seconds (0 = never expire, until evicted)
 * @return {bool} whether it was stored (false when the key already exists)
 * @throws {Error} kind "memcache" on an unexpected reply
 */
export func add(session as Session, key as string, value as string, exptime as int) {
    def r as string init store($session, "add", $key, $value, $exptime);
    if ($r == "STORED") {
        return true;
    }
    if ($r == "NOT_STORED") {
        return false;
    }
    fail($r);
}

/**
 * Return the string value of `key`, or "" when the key is absent / expired.
 * @param session {Session} the open session
 * @param key {string} the key to read
 * @return {string} the value, or "" when absent / expired
 */
export func get(session as Session, key as string) {
    writeCmd($session, "get " + $key + "\r\n");
    def first as Line init recvLine($session, "");
    checkError($first.text);
    if (strings.startsWith($first.text, "END")) {
        return "";
    }
    def parts as list of string init strings.split($first.text, " ");
    def nbytes as int init convert.toInt($parts[3]);
    def data as string init fillBytes($session, $first.rest, $nbytes + 2);
    def value as string init strings.substring($data, 0, $nbytes);
    # consume the value's trailing CRLF and the END line
    recvLine($session, strings.substring($data, $nbytes + 2));
    return $value;
}

/**
 * Remove `key`.
 * @param session {Session} the open session
 * @param key {string} the key to remove
 * @return {bool} whether the key existed
 * @throws {Error} kind "memcache" on an unexpected reply
 */
export func delete(session as Session, key as string) {
    writeCmd($session, "delete " + $key + "\r\n");
    def r as Line init recvLine($session, "");
    checkError($r.text);
    if ($r.text == "DELETED") {
        return true;
    }
    if ($r.text == "NOT_FOUND") {
        return false;
    }
    fail($r.text);
}

/**
 * Re-arm `key`'s expiry to `exptime` seconds.
 * @param session {Session} the open session
 * @param key {string} the key to re-arm
 * @param exptime {int} the new TTL in seconds (0 = never expire, until evicted)
 * @return {bool} whether the key existed
 * @throws {Error} kind "memcache" on an unexpected reply
 */
export func touch(session as Session, key as string, exptime as int) {
    writeCmd($session, "touch " + $key + " " + convert.toString($exptime) + "\r\n");
    def r as Line init recvLine($session, "");
    checkError($r.text);
    if ($r.text == "TOUCHED") {
        return true;
    }
    if ($r.text == "NOT_FOUND") {
        return false;
    }
    fail($r.text);
}

# counter runs incr / decr and returns the new value, or -1 when the key is
# absent (memcached will not create it).
func counter(session as Session, verb as string, key as string, delta as int) {
    writeCmd($session, $verb + " " + $key + " " + convert.toString($delta) + "\r\n");
    def r as Line init recvLine($session, "");
    checkError($r.text);
    if ($r.text == "NOT_FOUND") {
        return -1;
    }
    return convert.toInt($r.text);
}

/**
 * Atomically add `delta` to the counter at `key`.
 * @param session {Session} the open session
 * @param key {string} the counter key
 * @param delta {int} the amount to add
 * @return {int} the new value, or -1 when the key is absent
 */
export func incr(session as Session, key as string, delta as int) {
    return counter($session, "incr", $key, $delta);
}

/**
 * Atomically subtract `delta` from the counter at `key` (not below 0).
 * @param session {Session} the open session
 * @param key {string} the counter key
 * @param delta {int} the amount to subtract
 * @return {int} the new value, or -1 when the key is absent
 */
export func decr(session as Session, key as string, delta as int) {
    return counter($session, "decr", $key, $delta);
}

/**
 * End the session and close the connection.
 * @param session {Session} the open session
 */
export func quit(session as Session) {
    writeCmd($session, "quit\r\n");
    net.close($session.conn);
}
