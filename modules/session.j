# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Server-side sessions on memcached. A session is a `map of string to string`
 * held under a `sess:ID` key with a sliding TTL, so it expires on its own when
 * idle. The pieces: `memcache` (store + TTL), `uuid` (the session ID), and
 * `json` (encode the map). Built on the `memcache` module, so it needs the
 * default `jennifer` binary. Sessions are volatile (memcached evicts under
 * memory pressure), so treat them as a cache of soft state, not a store of
 * record. The JSON is stored base64-wrapped, so session values are binary-safe
 * (any UTF-8). Session IDs are UUID v4 drawn from the `crypto` library's
 * crypto-grade random source (122 unguessable bits), so the id is safe to hand
 * a client as the session's bearer token.
 * @module session
 * @example
 * def mc as memcache.Session init memcache.connect(memcache.Options{
 *     host: "127.0.0.1", port: 11211});
 * def id as string init session.create($mc, 1800);      # 30-minute session
 * def data as map of string to string init session.load($mc, $id);
 * $data["user"] = "ada";
 * session.save($mc, $id, $data, 1800);
 */
use strings;
use convert;
use json;
use uuid;
use encoding;
import "./memcache.j" as memcache;

# The key namespace for sessions in the cache.
def const PREFIX as string init "sess:";

# --- helpers (private) ---------------------------------------------

# checkId rejects a session id that could break out of the cache key and inject
# memcache protocol commands. Session ids arrive from a client cookie, so an id
# with a space, CR, LF, or any character outside `[A-Za-z0-9-]` (or longer than
# memcached's 250-byte key limit) is refused before it reaches the wire.
func checkId(id as string) {
    def n as int init len($id);
    if ($n == 0 or $n > 250) {
        throw Error{kind: "session", message: "invalid session id: must be 1 to 250 characters", file: "", line: 0, col: 0};
    }
    for (def c in strings.chars($id)) {
        def cp as int init convert.toCodepoint($c);
        def ok as bool init ($cp >= 97 and $cp <= 122) or ($cp >= 65 and $cp <= 90) or ($cp >= 48 and $cp <= 57) or $cp == 45;
        if (not $ok) {
            throw Error{kind: "session", message: "invalid session id: only letters, digits, and '-' are allowed", file: "", line: 0, col: 0};
        }
    }
}

func cacheKey(id as string) {
    checkId($id);
    return PREFIX + $id;
}

# pointer builds the RFC 6901 JSON Pointer for a session-data key, escaping
# `~` and `/` so keys carrying them still resolve.
func pointer(k as string) {
    def e as string init strings.replace($k, "~", "~0");
    return "/" + strings.replace($e, "/", "~1");
}

# encodeData serializes a session map to a storable ASCII blob (base64 of the
# JSON), so the cache only ever holds ASCII and any UTF-8 value round-trips.
func encodeData(data as map of string to string) {
    def js as string init json.encode($data);
    return encoding.toText(convert.bytesFromString($js, "utf-8"), "base64");
}

# decodeData parses a stored blob back into a session map (empty when the blob
# is empty, i.e. the session was absent or expired).
func decodeData(blob as string) {
    def out as map of string to string init {};
    if (len($blob) == 0) {
        return $out;
    }
    def js as string init convert.stringFromBytes(encoding.fromText($blob, "base64"), "utf-8");
    def node as json.Value init json.decode($js);
    for (def k in json.keys($node)) {
        $out[$k] = json.asString($node, pointer($k));
    }
    return $out;
}

# --- session lifecycle (exported) ----------------------------------

/**
 * Mint a new session ID and store an empty session with a ttl-second expiry.
 * @param mc {memcache.Session} the memcache connection
 * @param ttl {int} the session lifetime in seconds
 * @return {string} the new session ID
 */
export func create(mc as memcache.Session, ttl as int) {
    def id as string init uuid.generate("v4");
    def empty as map of string to string init {};
    memcache.set($mc, cacheKey($id), encodeData($empty), $ttl);
    return $id;
}

/**
 * Return the session's data map, or an empty map when the session is absent or
 * expired.
 * @param mc {memcache.Session} the memcache connection
 * @param id {string} the session ID
 * @return {map of string to string} the session data ({} if absent or expired)
 */
export func load(mc as memcache.Session, id as string) {
    return decodeData(memcache.get($mc, cacheKey($id)));
}

/**
 * Write the session's data map and re-arm its expiry to ttl seconds.
 * @param mc {memcache.Session} the memcache connection
 * @param id {string} the session ID
 * @param data {map of string to string} the session data to store
 * @param ttl {int} the session lifetime in seconds
 */
export func save(mc as memcache.Session, id as string,
    data as map of string to string, ttl as int) {
    memcache.set($mc, cacheKey($id), encodeData($data), $ttl);
}

/**
 * Re-arm the session's expiry without rewriting its data.
 * @param mc {memcache.Session} the memcache connection
 * @param id {string} the session ID
 * @param ttl {int} the new lifetime in seconds
 * @return {bool} true when the session still existed
 */
export func touch(mc as memcache.Session, id as string, ttl as int) {
    return memcache.touch($mc, cacheKey($id), $ttl);
}

/**
 * Remove the session.
 * @param mc {memcache.Session} the memcache connection
 * @param id {string} the session ID
 * @return {bool} true when the session existed
 */
export func destroy(mc as memcache.Session, id as string) {
    return memcache.delete($mc, cacheKey($id));
}
