# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.25.0

/**
 * A key/value store with per-key TTL behind one `Store` value and one verb set
 * (set / get / delete / touch / incrWindow), so code written against it runs
 * over any backend. This is the shared backend layer under the `session` and
 * `ratelimit` modules. The verb set mirrors `memcache`, so a fixed-window
 * counter (atomic incr + TTL) works identically on every backend.
 *
 * This module holds the **local** backends, over the `kv` library, and runs on
 * both binaries: `inProcessStore` keeps state in memory (reset each run),
 * `fileStore` persists it to a file (survives across `jennifer run`
 * invocations; handy for development and CLI tools with no server). The
 * distributed backends, a `memcache` or `redis` connection shared across
 * processes, are built by the client modules, `memcache.kvStore(mc)` and
 * `redis.kvStore(rc)`, which need the default binary (`net`); they return the
 * same `Store`, so `session` and `ratelimit` take either.
 *
 * `Store` is a sum type (enum): a `Local` variant carries its `kv.Store`
 * handle, a `Remote` variant the `Backend` that `memcache` or `redis` fills in
 * (the connection and one function value per verb, which run in that module).
 * This module imports no network module: `use net;` names only the
 * `net.Conn` type, so it loads on `jennifer-tiny` too.
 * @module kvstore
 */
use kv;
use net;

/**
 * A remote backend, as `memcache.kvStore` and `redis.kvStore` build it: the connection and the function
 * value that carries out each verb on it. Not built by hand; the function
 * values are called with the `Backend` itself first, then the verb's
 * arguments.
 * @field name {string} the backend: "memcache" or "redis"
 * @field conn {net.Conn} the open connection
 * @field timeoutMs {int} the connection's per-reply timeout in milliseconds
 * @field set {func} `(Backend, key, value, ttlSec)`
 * @field get {func} `(Backend, key) -> string`
 * @field delete {func} `(Backend, key) -> bool`
 * @field touch {func} `(Backend, key, ttlSec) -> bool`
 * @field incrWindow {func} `(Backend, key, ttlSec) -> int`
 */
export def struct Backend {
    name as string,
    conn as net.Conn,
    timeoutMs as int,
    set as func,
    get as func,
    delete as func,
    touch as func,
    incrWindow as func
};

/**
 * A store bound to one backend. Value-semantic; the handle a variant wraps (a
 * `kv.Store` or a `net.Conn`) is shared across copies. Build with
 * `inProcessStore` / `fileStore`, or `memcache.kvStore` /
 * `redis.kvStore`.
 */
export def enum Store {
    Local{local as kv.Store},
    Remote{backend as Backend}
};

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

/**
 * A store over a remote backend. `memcache.kvStore` and `redis.kvStore` call
 * this; other code has no need to.
 * @param backend {Backend} the backend, with every verb set
 * @return {Store} a store dispatching to the backend
 */
export func remoteStore(backend as Backend) {
    return Store.Remote{backend: $backend};
}

# --- uniform key/value ops (exported) ------------------------------

# checkTtl refuses a negative ttlSec. The backends disagree on one (memcached
# expires the key at once, redis and kv would keep it), so no backend sees it.
func checkTtl(fnName as string, ttlSec as int) {
    if ($ttlSec < 0) {
        throw Error{
            kind: "kvstore",
            message: "kvstore." + $fnName + ": ttlSec must be >= 0 (0 = no expiry), got {$ttlSec}"
        };
    }
    return null;
}

/**
 * Store `value` at `key` with a `ttlSec`-second expiry (0 = no expiry), replacing
 * any existing value.
 * @param store {Store} the backend store
 * @param key {string} the key to write
 * @param value {string} the value to store
 * @param ttlSec {int} the expiry in seconds (0 = no expiry)
 * @throws {Error} kind "kvstore" for a negative ttlSec
 */
export func set(store as Store, key as string, value as string, ttlSec as int) {
    checkTtl("set", $ttlSec);
    match ($store) {
        when Local(p) { kv.set($p.local, $key, $value, $ttlSec); }
        when Remote(r) { $r.backend.set($r.backend, $key, $value, $ttlSec); }
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
        when Local(p) { return kv.get($p.local, $key); }
        when Remote(r) { return $r.backend.get($r.backend, $key); }
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
        when Local(p) { return kv.delete($p.local, $key); }
        when Remote(r) { return $r.backend.delete($r.backend, $key); }
    }
    return false;
}

/**
 * Re-arm `key`'s expiry to `ttlSec` seconds; returns whether it existed.
 * @param store {Store} the backend store
 * @param key {string} the key to re-arm
 * @param ttlSec {int} the new expiry in seconds (0 = no expiry)
 * @return {bool} whether the key existed
 * @throws {Error} kind "kvstore" for a negative ttlSec
 */
export func touch(store as Store, key as string, ttlSec as int) {
    checkTtl("touch", $ttlSec);
    match ($store) {
        when Local(p) { return kv.touch($p.local, $key, $ttlSec); }
        when Remote(r) { return $r.backend.touch($r.backend, $key, $ttlSec); }
    }
    return false;
}

/**
 * Atomically increment the counter at `key` and return the new value, creating it
 * (at 1, with a `ttlSec`-second expiry) on the first hit. The atomic-counter-plus-TTL
 * primitive a fixed-window rate limiter is built from - portable across every
 * backend (kv `add` then `incr`; see `memcache.kvStore` and `redis.kvStore`).
 * @param store {Store} the store
 * @param key {string} the counter key
 * @param ttlSec {int} the TTL to arm on the first hit (seconds)
 * @return {int} the counter's new value
 * @throws {Error} kind "kvstore" for a negative ttlSec
 */
export func incrWindow(store as Store, key as string, ttlSec as int) {
    checkTtl("incrWindow", $ttlSec);
    match ($store) {
        when Local(p) {
            # kv.incr creates an absent key at 0 but without a TTL, so arm the
            # window first: add is a no-op when the counter exists, and incr
            # keeps its expiry. A counter that expired between the two starts
            # again at 1 and is re-armed.
            kv.add($p.local, $key, "0", $ttlSec);
            def n as int init kv.incr($p.local, $key, 1);
            if ($n == 1 and $ttlSec > 0) {
                kv.touch($p.local, $key, $ttlSec);
            }
            return $n;
        }
        when Remote(r) { return $r.backend.incrWindow($r.backend, $key, $ttlSec); }
    }
    return 0;
}
