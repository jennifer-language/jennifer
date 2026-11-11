# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.26.0

/**
 * A file-backed JSON document store. Load a JSON file once into a
 * value-semantic handle, query and edit it in memory through JSON Pointer, and
 * write it back with a crash-atomic whole-file replace (temp file + rename). It
 * is deliberately NOT a database engine: crash-atomic snapshotting of small
 * data. Atomicity is whole-file (temp + rename); the data is fsynced before the
 * rename, the directory entry is not. For a real database, use a client over `net`
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
use hash;
use encoding;

# Advisory-lock tuning for `lock` / `update`: how long to wait for a contended
# lock, how often to retry while waiting, and when a lock left by a crashed
# writer is considered stale and broken.
def const LOCK_WAIT_MS as int init 5000;
def const LOCK_RETRY_MS as int init 25;
def const LOCK_STALE_SECONDS as int init 120;
# BREAK_STALE_SECONDS ages out a `.break` file left by a breaker that crashed
# mid-break. A break takes milliseconds, so anything this old is abandoned.
def const BREAK_STALE_SECONDS as int init 30;

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
 * @field version {string} an opaque token of the file's content at open (a SHA-256
 *   of the text), so `save` can detect a concurrent write (empty when there was
 *   no backing file)
 */
export def struct DB {
    path as string,
    data as json.Value,
    version as string
};

/**
 * How `saveWith` writes. Start from `saveDefaults()` and change the fields you
 * need, so a field added later does not break your code.
 * @field pretty {bool} indent the JSON (`json.encodePretty`) for a file people
 *   read or edit by hand; false writes it compact
 * @field backup {bool} before replacing the file, keep its previous content as
 *   `<path>.bak`, itself written crash-atomically with the store's permissions
 */
export def struct SaveOptions {
    pretty as bool,
    backup as bool
};

/**
 * The options `save` uses: compact JSON, no backup.
 * @return {SaveOptions} the defaults
 */
export func saveDefaults() {
    return SaveOptions{pretty: false, backup: false};
}

/**
 * Load the JSON document at path into a DB. A missing file yields an empty
 * document (an empty object), so open never fails on a first run.
 * @param path {string} the backing file path
 * @return {DB} the loaded store
 */
export func open(path as string) {
    def text as string init "";
    def found as bool init fs.exists($path);
    if ($found) {
        $text = fs.readString($path);
    }
    return loaded($path, $found, $text);
}

/**
 * `open` with a size cap: refuse a file larger than `maxBytes` before decoding
 * it, for a store whose size another party controls (an upload directory, a
 * shared volume). At most `maxBytes + 1` bytes are ever read, however large the
 * file is or grows while being read. A missing file is an empty store, as for
 * `open`.
 * @param path {string} the backing file path
 * @param maxBytes {int} the largest file accepted, in bytes (> 0)
 * @return {DB} the loaded store
 * @throws {Error} kind "flatdb" if `maxBytes` is not positive or the file is
 *   larger than `maxBytes`; the `fs` / `json` errors `open` throws
 */
export func openWith(path as string, maxBytes as int) {
    if ($maxBytes <= 0) {
        throw Error{
            kind: "flatdb",
            message: "flatdb.openWith: maxBytes must be > 0, got {$maxBytes}"
        };
    }
    if (not fs.exists($path)) {
        return loaded($path, false, "");
    }
    # One bounded read rather than a stat followed by a whole-file read: the
    # file may grow between the two.
    def f as fs.File init fs.open($path, "read");
    defer fs.close($f);
    def data as bytes init fs.readBytes($f, $maxBytes + 1);
    if (len($data) > $maxBytes) {
        throw Error{
            kind: "flatdb",
            message: "flatdb.openWith: " + $path + " is larger than {$maxBytes} bytes"
        };
    }
    return loaded($path, true, convert.stringFromBytes("utf-8", $data));
}

# loaded builds the DB for a file's text (`found` false: no file, an empty store
# with no version).
func loaded(path as string, found as bool, text as string) {
    def doc as json.Value init json.map();
    def version as string init "";
    if ($found) {
        # The token is taken from the very text decoded, never from a second
        # look at the file: one taken after the read would vouch for a write
        # that landed in between, and `save` would then overwrite it silently.
        $version = tokenOf($text);
        # A whitespace-only (or zero-byte) file - e.g. `touch state.json` before
        # the first save - is treated like a missing file, so open never fails
        # on a first run.
        if (len(strings.trim($text)) > 0) {
            $doc = json.decode($text);
        }
    }
    return DB{path: $path, data: $doc, version: $version};
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
 * Return the sub-document at pointer, or `fallback` when the pointer does not
 * resolve - the `has` + `get` pair in one call. Only a missing node falls back:
 * a malformed pointer (one not starting with `/`) still throws.
 * @param db {DB} the store to read
 * @param pointer {string} the JSON Pointer
 * @param fallback {json.Value} what to return when nothing is at pointer
 * @return {json.Value} the node at pointer, or fallback
 * @throws {Error} when pointer is malformed
 */
export func getOr(db as DB, pointer as string, fallback as json.Value) {
    if (not json.has($db.data, $pointer)) {
        return $fallback;
    }
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
 * @param value {json.Value} the value to write (build scalars with `json.decode`,
 *   objects and lists with `json.map` / `json.list`)
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

# tokenOf is the version token of a document's text: its SHA-256, in hex.
func tokenOf(text as string) {
    return encoding.toText("hex", hash.compute("sha256", convert.bytesFromString("utf-8", $text)));
}

# isRegular reports whether path exists and is a regular file.
func isRegular(path as string) {
    return fs.exists($path) and fs.isFile($path);
}

# modeFor is the permission a store file is written with: the existing file's,
# else 0600 (OM-014).
func modeFor(path as string) {
    if (isRegular($path)) {
        return fs.stat($path).mode;
    }
    return 0o600;
}

# encodeFor renders the document as `save` writes it.
func encodeFor(data as json.Value, opts as SaveOptions) {
    if ($opts.pretty) {
        return json.encodePretty($data);
    }
    return json.encode($data);
}

# writeAtomic writes `text` and replaces `path` crash-atomically: it writes a
# sibling temp file and renames it over the target, so a reader ever sees the
# whole old file or the whole new one, never a torn write; an interrupted write
# leaves any existing target intact (only a stray temp file remains). The temp
# name is uniquified - a fixed `.tmp` sibling would let two concurrent writes
# share one path, so one could rename while the other is mid-write and publish a
# torn file, defeating the guarantee.
func writeAtomic(path as string, text as string, mode as int) {
    def tmp as string init $path + ".tmp." + uuid.v4();
    # The temp is created empty and chmod-ed before any data is written, so a
    # store tightened to 0600 (tokens, credentials) is never readable under a
    # wider mode, not even briefly.
    fs.writeNew($tmp, "");
    # On any failure below (a full disk, a cross-device rename), remove the temp
    # so no stray copy of the document stays behind; the failure propagates.
    errdefer removeQuietly($tmp);
    fs.chmod($tmp, $mode);
    def f as fs.File init fs.open($tmp, "write");
    fs.writeString($f, $text);
    # Flush the data before the rename: otherwise a crash can leave the renamed
    # file empty on filesystems that reorder metadata and data writes.
    fs.sync($f);
    fs.close($f);
    fs.rename($tmp, $path);
    return null;
}

# removeQuietly is a best-effort remove for error-path cleanup: it never throws,
# so the failure that triggered the cleanup is the one that propagates.
func removeQuietly(path as string) {
    try {
        fs.remove($path);
    } catch (err) { # lint-disable: L103
        # best-effort cleanup; the original failure is what matters
    }
    return null;
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
    return saveWith($db, saveDefaults());
}

/**
 * `save` with options: indented JSON (`pretty`) and a backup of the previous
 * content (`backup`, kept as `<path>.bak`). The conflict check is the same as
 * `save`'s, and the backup is exactly the content it checked, written before the
 * store is replaced; a first save, with no previous file, writes none.
 * @param db {DB} the store to persist
 * @param opts {SaveOptions} how to write (from `saveDefaults()`)
 * @return {DB} a fresh DB bound to the written file
 * @throws {Error} kind "flatdb" as `save`
 */
export func saveWith(db as DB, opts as SaveOptions) {
    if (len($db.path) == 0) {
        throw Error{
            kind: "flatdb",
            message: "flatdb.save: this DB is read-only (loaded with openString and has " +
                "no backing file); use flatdb.saveAs(db, path)"
        };
    }
    # Read once: the same text is both what the conflict check vouches for and
    # what the backup keeps.
    def exists as bool init isRegular($db.path);
    def previous as string init "";
    def current as string init "";
    if ($exists) {
        $previous = fs.readString($db.path);
        $current = tokenOf($previous);
    }
    if (not ($current == $db.version)) {
        throw Error{
            kind: "flatdb",
            message: "flatdb.save: the document at " + $db.path +
                " changed since it was opened. If you are saving this handle again, thread " +
                "the returned DB (`$db = flatdb.save($db);`); if a concurrent writer got " +
                "there first, re-open and retry, or use flatdb.update for a locked " +
                "read-modify-write"
        };
    }
    def mode as int init modeFor($db.path);
    if ($opts.backup and $exists) {
        # The store's mode, not an older backup's: a store tightened to 0600
        # must not leave its previous content readable under a wider one.
        writeAtomic($db.path + ".bak", $previous, $mode);
    }
    def text as string init encodeFor($db.data, $opts);
    writeAtomic($db.path, $text, $mode);
    return DB{path: $db.path, data: $db.data, version: tokenOf($text)};
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
        throw Error{kind: "flatdb", message: "flatdb.saveAs: path must not be empty"};
    }
    def text as string init json.encode($db.data);
    writeAtomic($path, $text, modeFor($path));
    return DB{path: $path, data: $db.data, version: tokenOf($text)};
}

# --- advisory locking ------------------------------------------------------

# holderStamp extracts the unix-seconds a holder id was stamped with
# ("<uuid> <seconds>"), or 0 when it cannot be parsed.
func holderStamp(holder as string) {
    def sp as int init strings.indexOf($holder, " ");
    if ($sp < 0) {
        return 0;
    }
    def tail as string init strings.trim($holder[$sp + 1..]);
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

# isStale reports whether the holder id ("<uuid> <seconds>") read from the file
# at `path` is older than maxAge seconds. A holder with no readable stamp ages by
# the file's mtime instead: a writer that crashed between creating its lock and
# writing its id leaves an empty file, and if that were never stale it would wedge
# the store for good.
func isStale(path as string, holder as string, maxAge as int) {
    def stamp as int init holderStamp($holder);
    if ($stamp == 0) {
        try {
            $stamp = fs.stat($path).mtimeNanos // 1000000000;
        } catch (err) { # lint-disable: L103
            return false; # gone already: nothing to break
        }
    }
    return time.unix(time.now()) - $stamp > $maxAge;
}

# removeIfHolder removes the file at path only if it still holds `expected`. The
# file is first renamed to a unique scratch name (so two removers can never both
# act on one file), then the moved content is checked: if a different holder had
# replaced it in the meantime, it is put back with an exclusive create rather
# than deleted. Returns whether the expected holder was removed.
func removeIfHolder(path as string, expected as string) {
    def scratch as string init $path + ".stale." + uuid.v4();
    try {
        fs.rename($path, $scratch);
    } catch (err) { # lint-disable: L103
        return false; # already gone, or another remover won the rename
    }
    def moved as string init "";
    try {
        $moved = fs.readString($scratch);
    } catch (err) { # lint-disable: L103
        $moved = "";
    }
    if ($moved != $expected) {
        try {
            fs.writeNew($path, $moved);
        } catch (err) { # lint-disable: L103
            # the path was taken again meanwhile; the moved holder cannot be restored
        }
        fs.remove($scratch);
        return false;
    }
    fs.remove($scratch);
    return true;
}

# releaseBreak removes this breaker's `.break` file, and only if it still holds
# this breaker's id (a leaked-break cleanup may have replaced it).
func releaseBreak(breakPath as string, me as string) {
    try {
        if (fs.readString($breakPath) == $me) {
            fs.remove($breakPath);
        }
    } catch (err) { # lint-disable: L103
        # already gone
    }
    return null;
}

# breakIfStale removes a lock whose holder timestamp is older than
# LOCK_STALE_SECONDS, so a lock leaked by a crashed writer does not wedge the store
# forever. The unlocked staleness read here is only a cheap pre-check; the break
# itself happens in breakHolder, under the exclusive `.break` file.
func breakIfStale(lockPath as string) {
    def holder as string init "";
    try {
        $holder = fs.readString($lockPath);
    } catch (err) { # lint-disable: L103
        return null;
    }
    if (isStale($lockPath, $holder, LOCK_STALE_SECONDS)) {
        breakHolder($lockPath, $holder);
    }
    return null;
}

# breakHolder removes the lock at lockPath if - and only if - it is still held by
# `judged`, the holder a waiter found stale. Waiters that judged the same stale
# holder must not each remove "whatever is at lockPath now": the first one's
# removal lets a writer take a fresh lock, which a second breaker would then
# delete, leaving two writers. So breakers serialize on an exclusive `.break`
# file (fs.writeNew) and, holding it, re-read the lock and remove it only while it
# still holds the judged holder. A waiter that finds `.break` taken leaves the
# break to its holder (clearing a `.break` abandoned by a crashed breaker) and
# simply polls again. Returns whether the judged holder was removed.
func breakHolder(lockPath as string, judged as string) {
    def breakPath as string init $lockPath + ".break";
    def me as string init uuid.v4() + " " + convert.toString(time.unix(time.now()));
    try {
        fs.writeNew($breakPath, $me);
    } catch (err) { # lint-disable: L103
        def other as string init "";
        try {
            $other = fs.readString($breakPath);
        } catch (err2) { # lint-disable: L103
            return false;
        }
        if (isStale($breakPath, $other, BREAK_STALE_SECONDS)) {
            removeIfHolder($breakPath, $other);
        }
        return false;
    }
    defer releaseBreak($breakPath, $me);
    def current as string init "";
    try {
        $current = fs.readString($lockPath);
    } catch (err) { # lint-disable: L103
        return false; # already released or broken
    }
    if ($current != $judged) {
        return false; # replaced since it was judged: the new holder is not stale
    }
    return removeIfHolder($lockPath, $judged);
}

# lockFileExists reports whether a lock file is visibly present. A path that
# cannot even be checked (a directory the process may not search) counts as
# absent: there is no holder to wait for there either.
func lockFileExists(lockPath as string) {
    try {
        return fs.exists($lockPath);
    } catch (err) {
        return false;
    }
}

# isSymlink reports whether path is a symbolic link (dangling or not).
func isSymlink(path as string) {
    try {
        fs.readlink($path);
        return true;
    } catch (err) {
        return false;
    }
}

# createWithoutHolder handles an exclusive create that failed. If a lock file is
# present, someone holds the lock: return false and let the caller wait. If none
# is, the create failed for another reason (a missing directory, a read-only
# mount, a directory owned by another user), and waiting cannot help - so retry
# once at once (a holder may have released between the create and the check),
# and if that fails with still no holder, throw the create's own error, which
# names the real cause, instead of reporting contention after the full wait.
func createWithoutHolder(lockPath as string, holder as string) {
    if (lockFileExists($lockPath)) {
        return false;
    }
    try {
        fs.writeNew($lockPath, $holder);
        return true;
    } catch (again) {
        if (lockFileExists($lockPath)) {
            return false; # taken by another writer in between: ordinary contention
        }
        # A dangling symlink is not a holder: fs.exists follows it (false) while
        # the exclusive create refuses the name. Say so rather than "file exists".
        def why as string init $again.message;
        if (isSymlink($lockPath)) {
            $why = "the path is a dangling symbolic link (a lock file is never a symlink); " +
                "remove it";
        }
        throw Error{
            kind: "flatdb",
            message: "flatdb.lock: cannot create the lock file " + $lockPath + ": " + $why
        };
    }
}

/**
 * Take the advisory write lock for the store backed by `dbPath`, blocking (with a
 * short poll) until it is free or `LOCK_WAIT_MS` elapses. Built on `fs.writeNew`
 * (an atomic exclusive create), so it coordinates **across processes**, not just
 * `spawn`ed tasks. A lock leaked by a crashed writer is broken after
 * `LOCK_STALE_SECONDS`. Pair with `unlock` - or, better, use `update`, which
 * brackets the whole read-modify-write and releases on every path.
 * `LOCK_WAIT_MS` is 5000 (5 s); `lockWith` takes a different bound.
 * @param dbPath {string} the store's backing file path
 * @return {Lock} the held lock (pass to unlock)
 * @throws {Error} kind "flatdb" if the lock cannot be taken within LOCK_WAIT_MS, or
 *   at once if the lock file cannot be created at all (no holder exists: a
 *   missing directory, a read-only mount, no write permission)
 */
export func lock(dbPath as string) {
    return lockWith($dbPath, LOCK_WAIT_MS);
}

/**
 * `lock` with the caller's own wait bound: block (with a short poll) for at most
 * `waitMs` milliseconds for a contended lock. `0` tries once and fails at once if
 * the lock is held; a long bound lets a writer queue behind a slow one.
 * @param dbPath {string} the store's backing file path
 * @param waitMs {int} how long to wait for a held lock, in milliseconds (>= 0)
 * @return {Lock} the held lock (pass to unlock)
 * @throws {Error} kind "flatdb" if `waitMs` is negative, if the lock cannot be
 *   taken within `waitMs`, or at once if the lock file cannot be created at all
 */
export func lockWith(dbPath as string, waitMs as int) {
    if ($waitMs < 0) {
        throw Error{kind: "flatdb", message: "flatdb.lockWith: waitMs must be >= 0, got {$waitMs}"};
    }
    def lockPath as string init $dbPath + ".lock";
    def id as string init uuid.v4();
    def holder as string init "";
    # A deadline on the clock, not a sum of sleeps: the time spent creating,
    # reading and breaking lock files counts against the caller's bound too.
    # Elapsed time is a duration truncated to whole ms, so the wait is never
    # cut short (whole-ms wall-clock stamps could end it up to 1ms early).
    def start as time.Time init time.now();
    while (true) {
        # Stamped at each attempt, not once up front: a holder that waited out a
        # long bound would otherwise take the lock with a stamp already near
        # LOCK_STALE_SECONDS, and lose it to a stale-break while still working.
        $holder = $id + " " + convert.toString(time.unix(time.now()));
        def took as bool init false;
        try {
            fs.writeNew($lockPath, $holder);
            $took = true;
        } catch (err) {
            $took = createWithoutHolder($lockPath, $holder);
            if (not $took) {
                breakIfStale($lockPath);
            }
        }
        if ($took) {
            return Lock{path: $lockPath, holder: $holder};
        }
        def left as int init $waitMs - time.milliseconds(time.sub(time.now(), $start));
        if ($left <= 0) {
            break;
        }
        def step as int init LOCK_RETRY_MS;
        if ($left < $step) {
            $step = $left;
        }
        time.sleep(time.fromMilliseconds($step));
    }
    throw Error{
        kind: "flatdb",
        message: "flatdb.lock: could not acquire the write lock at " + $lockPath +
            " within {$waitMs}ms (holder: " + peekHolder($lockPath) + ")"
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
    # Check-and-remove in one step (rename aside, then compare): a read followed
    # by a remove would delete a lock that a stale-break handed to another writer
    # in between.
    removeIfHolder($lk.path, $lk.holder);
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
 *
 * The lock is advisory and has no lease renewal: a lock older than
 * `LOCK_STALE_SECONDS` (120 s) is taken for a crashed writer's and broken, so
 * `transform` must finish well inside that, and should not do slow I/O.
 * @param path {string} the store's backing file path
 * @param transform {func} func(db as DB) -> DB, the edit to apply under the lock
 * @return {DB} the saved DB (its version is the freshly-written file's)
 * @throws {Error} kind "flatdb" if the lock cannot be taken, if `transform`
 *   returns a DB bound to another file, or on a write failure
 */
export func update(path as string, transform as func) {
    return updateWith($path, LOCK_WAIT_MS, $transform);
}

/**
 * `update` with the caller's own lock wait bound, as `lockWith` takes it.
 * @param path {string} the store's backing file path
 * @param waitMs {int} how long to wait for a held lock, in milliseconds (>= 0)
 * @param transform {func} func(db as DB) -> DB, the edit to apply under the lock
 * @return {DB} the saved DB (its version is the freshly-written file's)
 * @throws {Error} kind "flatdb" as `update`, and if `waitMs` is negative
 */
export func updateWith(path as string, waitMs as int, transform as func) {
    def lk as Lock init lockWith($path, $waitMs);
    defer unlock($lk);
    def db as DB init open($path);
    def edited as DB init $transform($db);
    # The lock covers `path` only. A transform that returned a DB from `saveAs`
    # or `openString` would have it write another file unlocked, or none.
    if ($edited.path != $path) {
        throw Error{
            kind: "flatdb",
            message: "flatdb.update: the transform returned a DB bound to \"" + $edited.path +
                "\", not \"" + $path + "\"; return the DB it was given, edited"
        };
    }
    return save($edited);
}
