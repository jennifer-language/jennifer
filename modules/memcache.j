# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
# pragma-jennifer-version: >=0.24.0
# pragma-jennifer-capability: net

/**
 * A client for a memcached server, speaking its classic text protocol over the
 * `net` system library. Store with an expiration (`set` / `add`), read (`get`),
 * remove (`delete`), count atomically (`incr` / `decr`), and re-arm a key's
 * expiry (`touch`). memcached is a volatile cache - keys expire on their
 * `exptime` and the server evicts under memory pressure - so it suits sessions,
 * rate limits, and derived data, not a system of record. `exptime` is seconds
 * (0 = never expire, until evicted). A protocol error (`ERROR` / `CLIENT_ERROR`
 * / `SERVER_ERROR`) throws a catchable `Error` (kind "memcache"). The value
 * block is framed over bytes by the server's byte count, so a value whose byte
 * length differs from its rune length (any non-ASCII UTF-8 text) round-trips
 * byte-exact. Needs the default `jennifer` binary (uses `net`).
 * @module memcache
 * @example
 * def mc as memcache.Session init memcache.connect(memcache.Options{host: "127.0.0.1", port: 11211});
 * memcache.set($mc, "greeting", "hello", 60);
 * io.printf("%s\n", memcache.get($mc, "greeting"));
 * memcache.quit($mc);
 */
use net;
use binary;
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

# Cap on a server-declared value length (64 MiB, the tree-wide house rule): a
# hostile or desynchronised server declaring a huge `VALUE ... <bytes>` would
# otherwise drive `fillBytes` to OOM the (recover-less) interpreter.
def const MAX_VALUE_BYTES as int init 67108864;

/**
 * An open memcached connection.
 * @field conn {net.Conn} the underlying socket
 * @field timeout {int} per-read idle timeout in milliseconds (0 disables it)
 */
export def struct Session {
    conn as net.Conn,
    timeout as int
};

/**
 * One value read by `gets`: its string value, the server's CAS token (an opaque
 * version counter), and whether the key was found. Pass `cas` to `cas` for a
 * check-and-set that only stores if no one else changed the value meanwhile.
 * @field value {string} the value ("" when not found)
 * @field cas {int} the CAS token to hand to `cas` (0 when not found)
 * @field found {bool} whether the key was present
 */
export def struct Item {
    value as string,
    cas as int,
    found as bool
};

# One CRLF-terminated protocol line plus the still-buffered remainder. `rest`
# is `bytes`: a value block is framed by a byte count and the remainder after a
# line can hold the start of one, so the buffer stays bytes and a payload is
# decoded to a string only once fully extracted.
def struct Line {
    text as string,
    rest as bytes
};

# One raw value block parsed from a get / gets reply: the key, the value as bytes
# (decoded to a string only by the string-returning verbs), and the CAS token
# (0 unless the command was `gets`).
def struct RawItem {
    key as string,
    value as bytes,
    cas as int
};

# --- wire helpers (private) ----------------------------------------

func writeCmd(session as Session, text as string) {
    net.writeBytes($session.conn, convert.bytesFromString($text, "utf-8"));
}

# emptyBytes returns a fresh empty bytes value (a zero-length starting buffer).
func emptyBytes() {
    def e as bytes;
    return $e;
}

# byteSlice returns buf[start:end] as a fresh bytes value.
func byteSlice(buf as bytes, start as int, end as int) {
    return binary.slice($buf, $start, $end);
}

# recvLine reads one CRLF-terminated line, returning it (without the CRLF, as a
# string - protocol lines are ASCII) and the raw byte remainder after it.
func recvLine(session as Session, buf as bytes) {
    def b as bytes init $buf;
    def scanFrom as int init 0;
    while (true) {
        # Scan for CRLF by indexing $b in place, resuming near the buffer end
        # (1-byte overlap for a straddling CRLF). Passing the whole growing
        # buffer to a helper each pass would deep-copy it (value semantics), so
        # a server sending a very long line would be O(n^2).
        def blen as int init len($b);
        def nl as int init -1;
        def si as int init $scanFrom;
        while ($si + 1 < $blen and $nl < 0) {
            if ($b[$si] == 13 and $b[$si + 1] == 10) {
                $nl = $si;
            }
            $si = $si + 1;
        }
        if ($nl >= 0) {
            return Line{
                text: convert.stringFromBytes(byteSlice($b, 0, $nl), "utf-8"),
                rest: byteSlice($b, $nl + 2, len($b))
            };
        }
        if ($session.timeout > 0) {
            net.setDeadline($session.conn, $session.timeout);
        }
        def chunk as bytes init net.readBytes($session.conn, 1024);
        if (len($chunk) == 0) {
            return Line{text: convert.stringFromBytes($b, "utf-8"), rest: emptyBytes()};
        }
        def j as int init 0;
        while ($j < len($chunk)) {
            $b[] = $chunk[$j];
            $j = $j + 1;
        }
        $scanFrom = $blen - 1;
        if ($scanFrom < 0) {
            $scanFrom = 0;
        }
    }
    return Line{text: "", rest: emptyBytes()};
}

# fillBytes reads until the buffer holds at least `n` bytes, framing over the
# raw byte stream (`n` is a byte count).
func fillBytes(session as Session, buf as bytes, n as int) {
    def b as bytes init $buf;
    while (len($b) < $n) {
        if ($session.timeout > 0) {
            net.setDeadline($session.conn, $session.timeout);
        }
        def chunk as bytes init net.readBytes($session.conn, 1024);
        if (len($chunk) == 0) {
            return $b;
        }
        def j as int init 0;
        while ($j < len($chunk)) {
            $b[] = $chunk[$j];
            $j = $j + 1;
        }
    }
    return $b;
}

# fail throws a catchable memcache error.
func fail(message as string) {
    throw Error{kind: "memcache", message: $message, file: "", line: 0, col: 0};
}

# checkValueLen rejects a server-declared VALUE length outside [0, MAX_VALUE_BYTES]
# before it is used to size a read.
func checkValueLen(n as int) {
    if ($n < 0 or $n > MAX_VALUE_BYTES) {
        fail("VALUE length out of range: " + convert.toString($n));
    }
    return;
}

# checkKey rejects a key that would corrupt the text protocol. memcached keys may
# not be empty, exceed 250 bytes, or contain a space, control byte (<= 0x20), or
# DEL (0x7f); a CR/LF in particular would inject extra command lines the server
# executes (OM-001), so this is validated in the module - once - rather than left
# to every caller (session ids, rate-limit keys, and app data all flow here).
func checkKey(key as string) {
    def raw as bytes init convert.bytesFromString($key, "utf-8");
    if (len($raw) == 0) {
        fail("key must not be empty");
    }
    if (len($raw) > 250) {
        fail("key exceeds the 250-byte memcached limit");
    }
    def i as int init 0;
    while ($i < len($raw)) {
        def b as int init $raw[$i];
        if ($b <= 32 or $b == 127) {
            fail("key contains an illegal byte (space, control, or DEL) at position " +
                convert.toString($i));
        }
        $i = $i + 1;
    }
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
    checkKey($key);
    writeCmd($session, storeHeader($verb, $key, $value, $exptime) + "\r\n" + $value + "\r\n");
    def resp as Line init recvLine($session, emptyBytes());
    checkError($resp.text);
    return $resp.text;
}

# storeBytes runs a storage command whose value is raw `bytes`, so a binary value
# reaches the wire byte-for-byte. `extra` is "" for set / add or " <casId>" for
# cas (the trailing field on the storage line). Returns the reply line.
func storeBytes(
    session as Session,
    verb as string,
    key as string,
    value as bytes,
    exptime as int,
    extra as string) {
    checkKey($key);
    def header as string init $verb + " " + $key + " 0 " + convert.toString($exptime) + " " +
        convert.toString(len($value)) + $extra;
    def out as bytes init convert.bytesFromString($header + "\r\n", "utf-8");
    $out = binary.concat($out, $value);
    $out = binary.concat($out, convert.bytesFromString("\r\n", "utf-8"));
    net.writeBytes($session.conn, $out);
    def resp as Line init recvLine($session, emptyBytes());
    checkError($resp.text);
    return $resp.text;
}

# fetchValues runs a `get` / `gets` for one or more keys and returns the value
# blocks parsed byte-exact (each `RawItem` keeps its value as bytes and, for
# `gets`, its CAS token). The reply is zero or more `VALUE <key> <flags> <bytes>
# [<cas>]\r\n<data>\r\n` blocks terminated by an `END` line; a missing key simply
# has no block. The value is framed by its byte count, so a binary value round-
# trips exactly and a value containing CRLF is not mis-split.
func fetchValues(session as Session, verb as string, keys as list of string, withCas as bool) {
    def cmd as string init $verb;
    for (def k in $keys) {
        checkKey($k);
        $cmd = $cmd + " " + $k;
    }
    writeCmd($session, $cmd + "\r\n");
    def items as list of RawItem init [];
    def minParts as int init 4;
    if ($withCas) {
        $minParts = 5;
    }
    def line as Line init recvLine($session, emptyBytes());
    while (true) {
        checkError($line.text);
        if (strings.startsWith($line.text, "END")) {
            return $items;
        }
        def parts as list of string init strings.split($line.text, " ");
        if (len($parts) < $minParts) {
            fail("malformed VALUE reply: " + $line.text);
        }
        def nbytes as int init convert.toInt($parts[3]);
        checkValueLen($nbytes);
        def cas as int init 0;
        if ($withCas) {
            $cas = convert.toInt($parts[4]);
        }
        # Read the value plus its trailing CRLF, framed by the byte count.
        def data as bytes init fillBytes($session, $line.rest, $nbytes + 2);
        $items[] = RawItem{key: $parts[1], value: byteSlice($data, 0, $nbytes), cas: $cas};
        # The next line (another VALUE or END) starts after the value + CRLF.
        $line = recvLine($session, byteSlice($data, $nbytes + 2, len($data)));
    }
    return $items;
}

# --- commands (exported) -------------------------------------------

/**
 * Open a session to the memcached server.
 * @param opts {Options} the connection settings
 * @return {Session} the open session
 */
export func connect(opts as Options) {
    def addr as string init $opts.host + ":" + convert.toString($opts.port);
    return Session{conn: net.connect($addr, DEFAULT_TIMEOUT_MS), timeout: DEFAULT_TIMEOUT_MS};
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
 * Store a raw `bytes` value at `key`, byte-for-byte (the binary counterpart to
 * `set`). A serialized blob, a compressed payload, or an image round-trips
 * exactly; read it back with `getBytes`, not `get` (the string reader throws on a
 * non-UTF-8 value).
 * @param session {Session} the open session
 * @param key {string} the key to write
 * @param value {bytes} the raw value to store
 * @param exptime {int} the TTL in seconds (0 = never expire, until evicted)
 * @throws {Error} kind "memcache" on a non-STORED reply
 */
export func setBytes(session as Session, key as string, value as bytes, exptime as int) {
    def r as string init storeBytes($session, "set", $key, $value, $exptime, "");
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
    match ($r) {
        when "STORED" { return true; }
        when "NOT_STORED" { return false; }
        else { fail($r); }
    }
}

/**
 * Return the string value of `key`, or "" when the key is absent / expired.
 * @param session {Session} the open session
 * @param key {string} the key to read
 * @return {string} the value, or "" when absent / expired
 */
export func get(session as Session, key as string) {
    def items as list of RawItem init fetchValues($session, "get", [$key], false);
    if (len($items) == 0) {
        return "";
    }
    return convert.stringFromBytes($items[0].value, "utf-8");
}

/**
 * Return the raw `bytes` value of `key`, or empty `bytes` when absent / expired
 * (the binary counterpart to `get`). It never UTF-8-decodes, so a value stored
 * with `setBytes` round-trips exactly.
 * @param session {Session} the open session
 * @param key {string} the key to read
 * @return {bytes} the value, or empty bytes when absent / expired
 */
export func getBytes(session as Session, key as string) {
    def items as list of RawItem init fetchValues($session, "get", [$key], false);
    if (len($items) == 0) {
        return emptyBytes();
    }
    return $items[0].value;
}

/**
 * Fetch several keys in one round-trip (multi-key `get`) as a `map of string to
 * string`; a missing / expired key is simply absent from the map, so `maps.has`
 * distinguishes it. Cheaper than N separate `get`s.
 * @param session {Session} the open session
 * @param keys {list of string} the keys to read
 * @return {map of string to string} the found key -> value pairs
 */
export func getMulti(session as Session, keys as list of string) {
    def out as map of string to string init {};
    for (def item in fetchValues($session, "get", $keys, false)) {
        $out[$item.key] = convert.stringFromBytes($item.value, "utf-8");
    }
    return $out;
}

/**
 * Read `key` with its CAS token (`gets`), for a check-and-set update. The
 * returned `Item` carries the `value`, the `cas` token, and whether the key was
 * `found`; pass its `cas` to `cas` so the store only succeeds if no one else
 * changed the value meanwhile.
 * @param session {Session} the open session
 * @param key {string} the key to read
 * @return {Item} the value, CAS token, and found flag
 */
export func gets(session as Session, key as string) {
    def items as list of RawItem init fetchValues($session, "gets", [$key], true);
    if (len($items) == 0) {
        return Item{value: "", cas: 0, found: false};
    }
    return Item{
        value: convert.stringFromBytes($items[0].value, "utf-8"),
        cas: $items[0].cas,
        found: true
    };
}

/**
 * Check-and-set: store `value` at `key` only if its CAS token still matches
 * `casId` (from a prior `gets`) - i.e. only if no one else changed it meanwhile.
 * The optimistic-concurrency primitive: `gets`, compute a new value, `cas`, and
 * retry on `"exists"`. Returns `"stored"` (success), `"exists"` (someone else
 * changed it - retry), or `"not_found"` (the key is gone / expired).
 * @param session {Session} the open session
 * @param key {string} the key to write
 * @param value {string} the new value
 * @param exptime {int} the TTL in seconds (0 = never expire, until evicted)
 * @param casId {int} the CAS token from a prior `gets`
 * @return {string} "stored", "exists", or "not_found"
 * @throws {Error} kind "memcache" on an unexpected reply
 */
export func cas(session as Session, key as string, value as string, exptime as int, casId as int) {
    def r as string init storeBytes(
        $session,
        "cas",
        $key,
        convert.bytesFromString($value, "utf-8"),
        $exptime,
        " " + convert.toString($casId));
    match ($r) {
        when "STORED" { return "stored"; }
        when "EXISTS" { return "exists"; }
        when "NOT_FOUND" { return "not_found"; }
        else { fail($r); }
    }
}

/**
 * Remove `key`.
 * @param session {Session} the open session
 * @param key {string} the key to remove
 * @return {bool} whether the key existed
 * @throws {Error} kind "memcache" on an unexpected reply
 */
export func delete(session as Session, key as string) {
    checkKey($key);
    writeCmd($session, "delete " + $key + "\r\n");
    def r as Line init recvLine($session, emptyBytes());
    checkError($r.text);
    match ($r.text) {
        when "DELETED" { return true; }
        when "NOT_FOUND" { return false; }
        else { fail($r.text); }
    }
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
    checkKey($key);
    writeCmd($session, "touch " + $key + " " + convert.toString($exptime) + "\r\n");
    def r as Line init recvLine($session, emptyBytes());
    checkError($r.text);
    match ($r.text) {
        when "TOUCHED" { return true; }
        when "NOT_FOUND" { return false; }
        else { fail($r.text); }
    }
}

# counter runs incr / decr and returns the new value, or -1 when the key is
# absent (memcached will not create it).
func counter(session as Session, verb as string, key as string, delta as int) {
    checkKey($key);
    writeCmd($session, $verb + " " + $key + " " + convert.toString($delta) + "\r\n");
    def r as Line init recvLine($session, emptyBytes());
    checkError($r.text);
    match ($r.text) {
        when "NOT_FOUND" { return -1; }
        else { return convert.toInt($r.text); }
    }
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
    # The socket is shut even when the quit write throws (a dead server must
    # not leak the fd).
    defer net.close($session.conn);
    writeCmd($session, "quit\r\n");
}
