# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
# pragma-jennifer-version: >=0.25.0

/**
 * A file-backed JSON document store. Load a JSON file once into a
 * value-semantic handle, query and edit it in memory through JSON Pointer, and
 * write it back with a crash-atomic whole-file replace (temp file + rename). It
 * is deliberately NOT a database engine: crash-atomic snapshotting of small
 * data. Atomicity is whole-file (temp + rename); durability is only as strong as
 * the OS's buffering of the rename. For a real database, use a client over `net`
 * (e.g. `redis`), not this. It is a thin file-lifecycle + ergonomics layer over
 * the `json` write surface, not a re-implementation of it. Runs on both binaries
 * (pure `json` + `fs`, no network).
 *
 * CONCURRENT WRITERS: crash-atomic is not the same as write-safe. Two writers
 * that each `open`, edit different parts, and `save` would otherwise silently
 * lose one update (last rename wins). Two guards address that: `save` **fails
 * loudly** (a catchable `flatdb` error) if the file changed since it was opened,
 * so a lost update can never be silent; and `update(path, transform)` brackets
 * the whole read-modify-write in a cross-process advisory lock, so concurrent
 * writers serialize and no update is lost. Prefer `update` when more than one
 * writer (separate processes, or `spawn`ed tasks) can touch one store.
 * @module flatdb
 * @example
 * import "flatdb.j" as flatdb;
 * def db as flatdb.DB init flatdb.open("state.json");
 * $db = flatdb.set($db, "/count", json.decode("1"));
 * flatdb.save($db);
 */
use json;
use fs;
use strings;
use uuid;
use time;
use convert;

# Advisory-lock tuning for `lock` / `update`: how long to wait for a contended
# lock, how often to retry while waiting, and when a lock left by a crashed
# writer is considered stale and broken.
def const LOCK_WAIT_MS as int init 5000;
def const LOCK_RETRY_MS as int init 25;
def const LOCK_STALE_SECONDS as int init 120;

/**
 * A held advisory lock, returned by `lock` and passed to `unlock`. Carries the
 * lock file path and this holder's unique id, so `unlock` removes only a lock
 * this holder still owns (never one a stale-break has handed to another writer).
 * @field path {string} the lock file path
 * @field holder {string} this holder's unique id ("<uuid> <unix-seconds>")
 */
export def struct Lock {
    path as string,
    holder as string
};

/**
 * The value the caller holds: the file path plus the decoded document. A module
 * holds no mutable state and `spawn` deep-copies scope, so a store cannot be a
 * shared open connection - it is a value. Mutating verbs return a fresh DB;
 * `save` is the only side effect.
 * @field path {string} the backing file path (empty for a read-only DB from openString)
 * @field data {json.Value} the decoded in-memory document
 * @field version {string} an opaque token of the file's on-disk state at open, so
 *   `save` can detect a concurrent write (empty when there was no backing file)
 */
export def struct DB {
    path as string,
    data as json.Value,
    version as string
};

/**
 * Load the JSON document at path into a DB. A missing file yields an empty
 * document (an empty object), so open never fails on a first run.
 * @param path {string} the backing file path
 * @return {DB} the loaded store
 */
export func open(path as string) {
    def doc as json.Value init json.map();
    if (fs.exists($path)) {
        def text as string init fs.readString($path);
        # A whitespace-only (or zero-byte) file - e.g. `touch state.json` before
        # the first save - is treated like a missing file, so open never fails
        # on a first run.
        if (len(strings.trim($text)) > 0) {
            $doc = json.decode($text);
        }
    }
    return DB{path: $path, data: $doc, version: versionOf($path)};
}

/**
 * Load a DB from an in-memory JSON string instead of a file - for a database
 * fetched over the network (`http.get(url, {}).body`), embedded in the program,
 * or built elsewhere. The returned DB has **no backing path, so it is read-only**:
 * every reader verb works and the mutating verbs still return a fresh in-memory
 * DB, but `save` throws (there is nowhere to write). Whitespace-only text yields
 * an empty document, matching `open` on a missing file. This keeps flatdb
 * transport-agnostic: it never imports `http` / `net`, so it stays `fs`-only and
 * builds on both binaries; the caller brings the bytes.
 * @param text {string} the JSON document
 * @return {DB} a read-only store
 */
export func openString(text as string) {
    def doc as json.Value init json.map();
    if (len(strings.trim($text)) > 0) {
        $doc = json.decode($text);
    }
    return DB{path: "", data: $doc, version: ""};
}

# --- readers (do not change the DB) ----------------------------------------

/**
 * Return the sub-document at pointer (the whole document for "").
 * @param db {DB} the store to read
 * @param pointer {string} the JSON Pointer
 * @return {json.Value} the node at pointer
 * @throws {Error} when pointer does not resolve
 */
export func get(db as DB, pointer as string) {
    return json.get($db.data, $pointer);
}

/**
 * Report whether pointer resolves to an existing node.
 * @param db {DB} the store to read
 * @param pointer {string} the JSON Pointer
 * @return {bool} true when the node exists
 */
export func has(db as DB, pointer as string) {
    return json.has($db.data, $pointer);
}

/**
 * List the keys of the object at pointer, in document order.
 * @param db {DB} the store to read
 * @param pointer {string} the JSON Pointer to an object
 * @return {list of string} the object's keys
 * @throws {Error} when pointer does not resolve to an object
 */
export func keys(db as DB, pointer as string) {
    return json.keys($db.data, $pointer);
}

/**
 * Return the element count of a list, or entry count of an object, at pointer.
 * @param db {DB} the store to read
 * @param pointer {string} the JSON Pointer to a list or object
 * @return {int} the element or entry count
 * @throws {Error} when pointer does not resolve to a list or object
 */
export func length(db as DB, pointer as string) {
    return json.length($db.data, $pointer);
}

# --- writers (return a fresh DB; call save to persist) ---------------------

/**
 * Write value at pointer (upsert an object key / replace a list index),
 * returning a new DB. Strict: intermediate containers must already exist.
 * @param db {DB} the store to edit
 * @param pointer {string} the JSON Pointer to write
 * @param value {json.Value} the value to write (build scalars with `json.decode`, objects and lists with `json.map` / `json.list`)
 * @return {DB} a new store with the write applied
 * @throws {Error} when an intermediate container does not exist
 */
export func set(db as DB, pointer as string, value as json.Value) {
    return DB{path: $db.path, data: json.set($db.data, $pointer, $value), version: $db.version};
}

/**
 * Push value onto the list addressed by pointer, returning a new DB. The list
 * must already exist (create it first with `set($db, ptr, json.list())`).
 * @param db {DB} the store to edit
 * @param pointer {string} the JSON Pointer to a list
 * @param value {json.Value} the value to append
 * @return {DB} a new store with the element appended
 * @throws {Error} when pointer does not resolve to an existing list
 */
export func append(db as DB, pointer as string, value as json.Value) {
    return DB{path: $db.path, data: json.append($db.data, $pointer, $value), version: $db.version};
}

/**
 * Drop the key or element at pointer, returning a new DB.
 * @param db {DB} the store to edit
 * @param pointer {string} the JSON Pointer to remove
 * @return {DB} a new store with the node removed
 * @throws {Error} when pointer does not resolve
 */
export func remove(db as DB, pointer as string) {
    return DB{path: $db.path, data: json.remove($db.data, $pointer), version: $db.version};
}

# --- persistence -----------------------------------------------------------

# versionOf returns an opaque token for the on-disk state of `path` (its mtime +
# size), used to detect a concurrent write between open and save. A path that does
# not exist (or is not a regular file) has the empty token "". Best-effort: it
# rests on the filesystem's mtime granularity, so two writes within one mtime tick
# that also produce an identically-sized document could slip past - use `update`
# for a hard, race-free guarantee.
func versionOf(path as string) {
    if (not (fs.exists($path) and fs.isFile($path))) {
        return "";
    }
    def st as fs.Stat init fs.stat($path);
    return convert.toString($st.mtimeNanos) + ":" + convert.toString($st.size);
}

# writeAtomic encodes `data` and replaces `path` crash-atomically: it writes a
# sibling temp file and renames it over the target, so a reader ever sees the
# whole old file or the whole new one, never a torn write; an interrupted write
# leaves any existing target intact (only a stray temp file remains). The temp
# name is uniquified - a fixed `.tmp` sibling would let two concurrent writes
# share one path, so one could rename while the other is mid-write and publish a
# torn file, defeating the guarantee.
func writeAtomic(path as string, data as json.Value) {
    def text as string init json.encode($data);
    def tmp as string init $path + ".tmp." + uuid.v4();
    fs.writeString($tmp, $text);
    # Preserve the target's existing permissions before the rename replaces it:
    # fs.writeString creates the temp at 0644, so a store the operator tightened
    # to 0600 (tokens, credentials, user records) must not silently come back
    # world-readable. A new file defaults to 0600, not 0644 (OM-014).
    def mode as int init 0o600;
    if (fs.exists($path) and fs.isFile($path)) {
        def st as fs.Stat init fs.stat($path);
        $mode = $st.mode;
    }
    fs.chmod($tmp, $mode);
    # If the rename fails (cross-device, permissions, full disk), remove the temp
    # so a complete readable copy of the document is not left behind under a stray
    # name; then re-raise the original failure.
    try {
        fs.rename($tmp, $path);
    } catch (err) {
        try {
            fs.remove($tmp);
        } catch (rmErr) { # lint-disable: L103
            # best-effort cleanup; the rename failure below is what matters
        }
        throw $err;
    }
}

/**
 * Write the document back to its own backing file, crash-atomically (temp +
 * rename), and return a fresh DB rebound to the just-written file. A read-only DB
 * (from `openString`, no backing file) has nowhere to write - use `saveAs`.
 *
 * **Conflict check:** if the file changed since this DB was opened (a concurrent
 * writer got there first), `save` throws a `flatdb` error instead of silently
 * clobbering their write. Re-open and retry, or - to serialize writers without a
 * possible conflict at all - use `update`. Because `save` returns the rebound DB,
 * a program that saves the same handle repeatedly should thread it through
 * (`$db = flatdb.save($db);`) so the next save checks against the file it wrote.
 * @param db {DB} the store to persist
 * @return {DB} a fresh DB bound to the written file (its version is the new file's)
 * @throws {Error} kind "flatdb" if the DB is read-only, if the document changed
 *   since it was opened, or on a filesystem write / rename failure
 */
export func save(db as DB) {
    if (len($db.path) == 0) {
        throw Error{
            kind: "flatdb",
            message: "flatdb.save: this DB is read-only (loaded with openString and has no backing file); use flatdb.saveAs(db, path)",
            file: "",
            line: 0,
            col: 0
        };
    }
    def current as string init versionOf($db.path);
    if (not ($current == $db.version)) {
        throw Error{
            kind: "flatdb",
            message: "flatdb.save: the document at " + $db.path +
                " changed since it was opened. If you are saving this handle again, thread the returned DB (`$db = flatdb.save($db);`); if a concurrent writer got there first, re-open and retry, or use flatdb.update for a locked read-modify-write",
            file: "",
            line: 0,
            col: 0
        };
    }
    writeAtomic($db.path, $db.data);
    return DB{path: $db.path, data: $db.data, version: versionOf($db.path)};
}

/**
 * Write the document to `path` (not necessarily its own) and return a **fresh DB
 * bound to that path** - the passed-in `db` is unchanged (value semantics, like
 * `set` / `append` / `remove`). Two uses: the first dump to disk of a read-only
 * `openString` DB, and copying / forking an on-disk store to a new file (a
 * snapshot or a new version). Which store is "current" for the next `save` is
 * whichever handle you keep - reassign (`$db = flatdb.saveAs($db, path);`) to
 * make the new file current, or hold both to keep the original too. Same
 * crash-atomic temp+rename as `save`.
 * @param db {DB} the store whose data to write
 * @param path {string} the destination file path
 * @return {DB} a new DB bound to `path` (its data equals `db`'s)
 * @throws {Error} kind "flatdb" if `path` is empty, or on a filesystem failure
 */
export func saveAs(db as DB, path as string) {
    if (len($path) == 0) {
        throw Error{
            kind: "flatdb",
            message: "flatdb.saveAs: path must not be empty",
            file: "",
            line: 0,
            col: 0
        };
    }
    writeAtomic($path, $db.data);
    return DB{path: $path, data: $db.data, version: versionOf($path)};
}

# --- advisory locking ------------------------------------------------------

# holderStamp extracts the unix-seconds a holder id was stamped with
# ("<uuid> <seconds>"), or 0 when it cannot be parsed.
func holderStamp(holder as string) {
    def sp as int init strings.indexOf($holder, " ");
    if ($sp < 0) {
        return 0;
    }
    def tail as string init strings.trim(strings.substring($holder, $sp + 1, len($holder)));
    def n as int init 0;
    try {
        $n = convert.toInt($tail);
    } catch (err) { # lint-disable: L103
        $n = 0;
    }
    return $n;
}

# peekHolder reads the current lock holder for a diagnostic, or "unknown".
func peekHolder(lockPath as string) {
    try {
        return fs.readString($lockPath);
    } catch (err) { # lint-disable: L103
        return "unknown";
    }
}

# breakIfStale removes a lock whose holder timestamp is older than
# LOCK_STALE_SECONDS, so a lock leaked by a crashed writer does not wedge the store
# forever. The break is atomic against other breakers: it renames the stale lock to
# a unique scratch name, and only the rename winner removes it - so two waiters can
# never both "break and re-take" the same lock.
func breakIfStale(lockPath as string) {
    def holder as string init "";
    try {
        $holder = fs.readString($lockPath);
    } catch (err) { # lint-disable: L103
        return null;
    }
    def stamp as int init holderStamp($holder);
    if ($stamp == 0) {
        return null;
    }
    if (time.unix(time.now()) - $stamp <= LOCK_STALE_SECONDS) {
        return null;
    }
    def scratch as string init $lockPath + ".stale." + uuid.v4();
    try {
        fs.rename($lockPath, $scratch);
        fs.remove($scratch);
    } catch (err) { # lint-disable: L103
        # lost the break race, or the lock changed under us - just retry
    }
    return null;
}

/**
 * Take the advisory write lock for the store backed by `dbPath`, blocking (with a
 * short poll) until it is free or `LOCK_WAIT_MS` elapses. Built on `fs.writeNew`
 * (an atomic exclusive create), so it coordinates **across processes**, not just
 * `spawn`ed tasks. A lock leaked by a crashed writer is broken after
 * `LOCK_STALE_SECONDS`. Pair with `unlock` - or, better, use `update`, which
 * brackets the whole read-modify-write and releases on every path.
 * @param dbPath {string} the store's backing file path
 * @return {Lock} the held lock (pass to unlock)
 * @throws {Error} kind "flatdb" if the lock cannot be taken within LOCK_WAIT_MS
 */
export func lock(dbPath as string) {
    def lockPath as string init $dbPath + ".lock";
    def holder as string init uuid.v4() + " " + convert.toString(time.unix(time.now()));
    def waited as int init 0;
    while ($waited <= LOCK_WAIT_MS) {
        def took as bool init false;
        try {
            fs.writeNew($lockPath, $holder);
            $took = true;
        } catch (err) { # lint-disable: L103
            breakIfStale($lockPath);
        }
        if ($took) {
            return Lock{path: $lockPath, holder: $holder};
        }
        time.sleep(time.fromMilliseconds(LOCK_RETRY_MS));
        $waited = $waited + LOCK_RETRY_MS;
    }
    throw Error{
        kind: "flatdb",
        message: "flatdb.lock: could not acquire the write lock at " + $lockPath +
            " within " + convert.toString(LOCK_WAIT_MS) + "ms (holder: " + peekHolder($lockPath) + ")",
        file: "",
        line: 0,
        col: 0
    };
}

/**
 * Release a lock taken by `lock`, but only if this holder still owns it: if a
 * stale-break already handed the lock to another writer, `unlock` does nothing (it
 * never removes a lock it no longer holds).
 * @param lk {Lock} the lock returned by lock
 * @return {null}
 */
export func unlock(lk as Lock) {
    def current as string init "";
    try {
        $current = fs.readString($lk.path);
    } catch (err) { # lint-disable: L103
        return null;
    }
    if ($current == $lk.holder) {
        try {
            fs.remove($lk.path);
        } catch (err) { # lint-disable: L103
            # best-effort; a concurrent stale-break may already have removed it
        }
    }
    return null;
}

/**
 * Run a locked read-modify-write against the store at `path`: take the write lock,
 * open the **current** document under it, apply `transform`, save, and release the
 * lock - on every exit path (via `defer`). This is the safe way to update a store
 * that concurrent writers (separate processes, or `spawn`ed tasks) may also write:
 * because the read and the write are one locked span, no update is lost and `save`
 * cannot conflict. `transform` is a `func` value `func(db as DB) -> DB` - edit the
 * DB it receives with `set` / `append` / `remove` and return the result.
 * @param path {string} the store's backing file path
 * @param transform {func} func(db as DB) -> DB, the edit to apply under the lock
 * @return {DB} the saved DB (its version is the freshly-written file's)
 * @throws {Error} kind "flatdb" if the lock cannot be taken, or on a write failure
 */
export func update(path as string, transform as func) {
    def lk as Lock init lock($path);
    defer unlock($lk);
    def db as DB init open($path);
    def edited as DB init $transform($db);
    return save($edited);
}
