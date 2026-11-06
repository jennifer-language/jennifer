# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.25.0

/**
 * Server-side sessions over a selectable key/value backend. A session is a
 * `json.Value` held under a `sess:ID` key with a sliding TTL, so it expires on
 * its own when idle. The store is a `kvstore.Store` - back it with `memcache` or
 * `redis` (distributed, shared across processes) or the in-process `kv` library
 * (single process, no server), all behind one API. Session values are a
 * `json.Value`, so a session holds structured data (nested objects, arrays,
 * numbers), not just flat strings. The JSON is stored base64-wrapped, so any
 * value is binary-safe. Session IDs are UUID v4 from the `crypto` library's
 * crypto-grade random source (122 unguessable bits), safe to hand a client as
 * the session's bearer token. Sessions are volatile (a cache of soft state, not a
 * store of record - memcache / kv evict, so treat expiry as expected).
 * @module session
 * @example
 * def store as kvstore.Store init kvstore.redisStore($rc);   # or memcacheStore / inProcessStore
 * def id as string init session.create($store, 1800);        # 30-minute session
 * def data as json.Value init session.load($store, $id);
 * $data = json.set($data, "/user", "ada");
 * session.save($store, $id, $data, 1800);
 */
use convert;
use json;
use uuid;
use regex;
use encoding;
import "./kvstore.j" as kvstore;

# The key namespace for sessions in the store.
def const PREFIX as string init "sess:";

# --- helpers (private) ---------------------------------------------

# checkId rejects a session id that could break out of the store key and inject
# backend protocol commands. Session ids arrive from a client cookie, so an id
# with a space, CR, LF, or any character outside `[A-Za-z0-9-]` (or longer than
# the 250-byte key limit) is refused before it reaches the wire.
func checkId(id as string) {
    def n as int init len($id);
    if ($n == 0 or $n > 250) {
        throw Error{kind: "session", message: "invalid session id: must be 1 to 250 characters"};
    }
    if (not regex.matches('^[A-Za-z0-9-]+$', $id)) {
        throw Error{
            kind: "session",
            message: "invalid session id: only letters, digits, and '-' are allowed"
        };
    }
}

# checkTtl rejects a ttl below 1: kvstore reads <= 0 as "no expiry", which would
# make the session immortal.
func checkTtl(ttl as int) {
    if ($ttl < 1) {
        throw Error{kind: "session", message: "session: ttl must be >= 1 second"};
    }
}

func storeKey(id as string) {
    checkId($id);
    return PREFIX + $id;
}

# encodeData serializes a session value to a storable ASCII blob (base64 of the
# JSON), so the store only ever holds ASCII and any value round-trips.
func encodeData(data as json.Value) {
    def js as string init json.encode($data);
    return encoding.toText(convert.bytesFromString($js, "utf-8"), "base64");
}

# decodeData parses a stored blob back into a session value (an empty object when
# the blob is empty, i.e. the session was absent or expired).
func decodeData(blob as string) {
    if (len($blob) == 0) {
        return json.map();
    }
    try {
        return json.decode(convert.stringFromBytes(encoding.fromText($blob, "base64"), "utf-8"));
    } catch (e) {
        throw Error{kind: "session", message: "session: stored data is corrupt"};
    }
}

# --- session lifecycle (exported) ----------------------------------

/**
 * Mint a new session ID and store an empty session with a ttl-second expiry.
 * @param store {kvstore.Store} the backend store
 * @param ttl {int} the session lifetime in seconds (>= 1)
 * @return {string} the new session ID
 * @throws {Error} kind "session" if ttl is < 1
 */
export func create(store as kvstore.Store, ttl as int) {
    checkTtl($ttl);
    def id as string init uuid.v4();
    kvstore.set($store, storeKey($id), encodeData(json.map()), $ttl);
    return $id;
}

/**
 * Return the session's data as a `json.Value`, or an empty object when the
 * session is absent or expired.
 * @param store {kvstore.Store} the backend store
 * @param id {string} the session ID
 * @return {json.Value} the session data (an empty object if absent or expired)
 */
export func load(store as kvstore.Store, id as string) {
    return decodeData(kvstore.get($store, storeKey($id)));
}

/**
 * Whether the session exists (was created and has not expired or been
 * destroyed). Check it before `save` on an id from a request: `save` stores
 * any well-formed id, so saving a client-chosen one would adopt it.
 * @param store {kvstore.Store} the backend store
 * @param id {string} the session ID
 * @return {bool} true when the session exists
 */
export func exists(store as kvstore.Store, id as string) {
    return len(kvstore.get($store, storeKey($id))) > 0;
}

/**
 * Move a session to a fresh ID and destroy the old one; call it when the
 * privilege level changes (login, logout, sudo) so an ID known before the
 * change is worthless after it. An absent session moves as an empty one.
 * @param store {kvstore.Store} the backend store
 * @param id {string} the current session ID
 * @param ttl {int} the new session's lifetime in seconds (>= 1)
 * @return {string} the new session ID
 * @throws {Error} kind "session" if ttl is < 1
 */
export func rotate(store as kvstore.Store, id as string, ttl as int) {
    checkTtl($ttl);
    def blob as string init kvstore.get($store, storeKey($id));
    if (len($blob) == 0) {
        $blob = encodeData(json.map());
    }
    def fresh as string init uuid.v4();
    kvstore.set($store, storeKey($fresh), $blob, $ttl);
    kvstore.delete($store, storeKey($id));
    return $fresh;
}

/**
 * Write the session's data and re-arm its expiry to ttl seconds.
 * @param store {kvstore.Store} the backend store
 * @param id {string} the session ID
 * @param data {json.Value} the session data to store
 * @param ttl {int} the session lifetime in seconds (>= 1)
 * @throws {Error} kind "session" if ttl is < 1
 */
export func save(store as kvstore.Store, id as string, data as json.Value, ttl as int) {
    checkTtl($ttl);
    kvstore.set($store, storeKey($id), encodeData($data), $ttl);
}

/**
 * Re-arm the session's expiry without rewriting its data.
 * @param store {kvstore.Store} the backend store
 * @param id {string} the session ID
 * @param ttl {int} the new lifetime in seconds (>= 1)
 * @return {bool} true when the session still existed
 * @throws {Error} kind "session" if ttl is < 1
 */
export func touch(store as kvstore.Store, id as string, ttl as int) {
    checkTtl($ttl);
    return kvstore.touch($store, storeKey($id), $ttl);
}

/**
 * Remove the session.
 * @param store {kvstore.Store} the backend store
 * @param id {string} the session ID
 * @return {bool} true when the session existed
 */
export func destroy(store as kvstore.Store, id as string) {
    return kvstore.delete($store, storeKey($id));
}
