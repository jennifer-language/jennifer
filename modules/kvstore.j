# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
# pragma-jennifer-version: >=0.24.0

/**
 * A backend selector for a key/value store with per-key TTL: one `Store` value
 * is one of three backends - a `memcache` connection, a `redis` connection, or
 * the in-process `kv` library - and dispatches a uniform set / get / delete /
 * touch / incrWindow surface to whichever was chosen. This is the shared backend
 * layer under the `session` and `ratelimit` modules, so each works over any of
 * the three without duplicating the dispatch. The verb set mirrors `memcache`,
 * so a fixed-window counter (atomic incr + TTL) works identically on all three.
 *
 * `Store` is a sum type (enum), not a tagged struct: each variant carries only
 * its own backend handle, and the dispatch is a `match` the compiler checks for
 * exhaustiveness - there is no invalid "kind says memcache but the handle is
 * empty" state, and a new backend cannot be silently forgotten in one verb.
 *
 * The `memcache` / `redis` backends are distributed (state lives on the server,
 * shared across processes); the **local** backend (the `kv` library) keeps state
 * in this process - either in memory (`inProcessStore`, reset each run) or
 * persisted to a file (`fileStore`, survives across `jennifer run` invocations -
 * handy for development and CLI tools with no server). Pick a distributed backend
 * when more than one process must see the same state.
 * @module kvstore
 */
use kv;
use convert;
import "./memcache.j" as memcache;
import "./redis.j" as redis;

/**
 * A store bound to one backend (a sum type over the three). Value-semantic; the
 * live connection / handle a variant wraps is shared across copies (a `net.Conn`
 * or a `kv.Store` handle). Build with `memcacheStore` / `redisStore` /
 * `inProcessStore` / `fileStore`.
 */
export def enum Store {
    Memcache{mc as memcache.Session},
    Redis{rc as redis.Session},
    Local{local as kv.Store}
};

/**
 * A store backed by a memcache connection (distributed).
 * @param mc {memcache.Session} the memcache connection
 * @return {Store} a memcache-backed store
 */
export func memcacheStore(mc as memcache.Session) {
    return Store.Memcache{mc: $mc};
}

/**
 * A store backed by a redis connection (distributed).
 * @param rc {redis.Session} the redis connection
 * @return {Store} a redis-backed store
 */
export func redisStore(rc as redis.Session) {
    return Store.Redis{rc: $rc};
}

/**
 * A store backed by the in-process `kv` library (this process only, no server).
 * Opens a fresh `kv.Store`; keep the returned `Store` for the program's lifetime.
 * @return {Store} an in-process store
 */
export func inProcessStore() {
    return Store.Local{local: kv.open()};
}

/**
 * A store backed by the in-process `kv` library, **persisted to `path`**: its
 * contents load on open and survive across `jennifer run` invocations (unlike
 * `inProcessStore`, which resets each run). No server; single-process (safe for
 * `spawn` within one process, but not for concurrent separate processes on one
 * file - use a distributed backend for that). Practical for development and
 * persistent CLI state.
 * @param path {string} the file to persist the store to
 * @return {Store} a file-backed local store
 */
export func fileStore(path as string) {
    return Store.Local{local: kv.openFile($path)};
}

# --- uniform key/value ops (exported) ------------------------------

/**
 * Store `value` at `key` with a `ttl`-second expiry (0 = no expiry), replacing
 * any existing value.
 * @param store {Store} the backend store
 * @param key {string} the key to write
 * @param value {string} the value to store
 * @param ttl {int} the expiry in seconds (0 = no expiry)
 */
export func set(store as Store, key as string, value as string, ttl as int) {
    match ($store) {
        when Memcache(m) { memcache.set($m.mc, $key, $value, $ttl); }
        when Local(p) { kv.set($p.local, $key, $value, $ttl); }
        when Redis(r) {
            if ($ttl > 0) {
                redis.command($r.rc, ["SET", $key, $value, "EX", convert.toString($ttl)]);
            } else {
                redis.command($r.rc, ["SET", $key, $value]);
            }
        }
    }
    return;
}

/**
 * Return the value at `key`, or "" when absent / expired.
 * @param store {Store} the backend store
 * @param key {string} the key to read
 * @return {string} the value, or "" when absent / expired
 */
export func get(store as Store, key as string) {
    match ($store) {
        when Memcache(m) { return memcache.get($m.mc, $key); }
        when Local(p) { return kv.get($p.local, $key); }
        when Redis(r) { return redis.get($r.rc, $key); }
    }
    return "";
}

/**
 * Remove `key`; returns whether it existed.
 * @param store {Store} the backend store
 * @param key {string} the key to remove
 * @return {bool} whether the key existed
 */
export func delete(store as Store, key as string) {
    match ($store) {
        when Memcache(m) { return memcache.delete($m.mc, $key); }
        when Local(p) { return kv.delete($p.local, $key); }
        when Redis(r) { return redis.del($r.rc, $key) > 0; }
    }
    return false;
}

/**
 * Re-arm `key`'s expiry to `ttl` seconds; returns whether it existed.
 * @param store {Store} the backend store
 * @param key {string} the key to re-arm
 * @param ttl {int} the new expiry in seconds (0 = no expiry)
 * @return {bool} whether the key existed
 */
export func touch(store as Store, key as string, ttl as int) {
    match ($store) {
        when Memcache(m) { return memcache.touch($m.mc, $key, $ttl); }
        when Local(p) { return kv.touch($p.local, $key, $ttl); }
        when Redis(r) {
            if ($ttl > 0) {
                return redis.command($r.rc, ["EXPIRE", $key, convert.toString($ttl)]).num > 0;
            }
            # ttl <= 0 means "no expiry" (uniform with memcache / kv). Use PERSIST,
            # not EXPIRE: redis treats a non-positive EXPIRE as an immediate
            # delete, which would diverge from the other backends. Report whether
            # the key existed, as the other backends do.
            def present as bool init redis.command($r.rc, ["EXISTS", $key]).num > 0;
            if ($present) {
                redis.command($r.rc, ["PERSIST", $key]);
            }
            return $present;
        }
    }
    return false;
}

/**
 * Atomically increment the counter at `key` and return the new value, creating it
 * (at 1, with a `ttl`-second expiry) on the first hit. The atomic-counter-plus-TTL
 * primitive a fixed-window rate limiter is built from - portable across all three
 * backends (redis `INCR` + `EXPIRE`; memcache / kv `incr` else `add`).
 * @param store {Store} the store
 * @param key {string} the counter key
 * @param ttl {int} the TTL to arm on the first hit (seconds)
 * @return {int} the counter's new value
 */
export func incrWindow(store as Store, key as string, ttl as int) {
    match ($store) {
        when Redis(r) {
            # redis INCR creates the key (at 1) when absent, so arm the TTL on the
            # first hit and the window resets on its own.
            def n as int init redis.command($r.rc, ["INCR", $key]).num;
            if ($n == 1) {
                redis.command($r.rc, ["EXPIRE", $key, convert.toString($ttl)]);
            }
            return $n;
        }
        when Memcache(m) {
            # memcache incr does not create; incr-else-add arms the window TTL.
            def n as int init memcache.incr($m.mc, $key, 1);
            if ($n == -1) {
                if (memcache.add($m.mc, $key, "1", $ttl)) {
                    return 1;
                }
                return memcache.incr($m.mc, $key, 1);
            }
            return $n;
        }
        when Local(p) {
            def n as int init kv.incr($p.local, $key, 1);
            if ($n == -1) {
                if (kv.add($p.local, $key, "1", $ttl)) {
                    return 1;
                }
                return kv.incr($p.local, $key, 1);
            }
            return $n;
        }
    }
    return 0;
}
