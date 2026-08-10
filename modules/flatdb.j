# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
# pragma-jennifer-version: >=0.24.0

/**
 * A file-backed JSON document store. Load a JSON file once into a
 * value-semantic handle, query and edit it in memory through JSON Pointer, and
 * write it back with a crash-atomic whole-file replace (temp file + rename). It
 * is deliberately NOT a database engine: crash-atomic snapshotting of small
 * data. Atomicity is whole-file (temp + rename); there is no isolation (one
 * process, reload-the-whole-file, no concurrent transactions) and durability is
 * only as strong as the OS's buffering of the rename. For a real database, use
 * a client over `net` (e.g. `redis`), not this. It is a thin file-lifecycle +
 * ergonomics layer over the `json` write surface, not a re-implementation of
 * it. Runs on both binaries (pure `json` + `fs`, no network).
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

/**
 * The value the caller holds: the file path plus the decoded document. A module
 * holds no mutable state and `spawn` deep-copies scope, so a store cannot be a
 * shared open connection - it is a value. Mutating verbs return a fresh DB;
 * `save` is the only side effect.
 * @field path {string} the backing file path (empty for a read-only DB from openString)
 * @field data {json.Value} the decoded in-memory document
 */
export def struct DB {
    path as string,
    data as json.Value
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
    return DB{path: $path, data: $doc};
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
    return DB{path: "", data: $doc};
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
    return DB{path: $db.path, data: json.set($db.data, $pointer, $value)};
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
    return DB{path: $db.path, data: json.append($db.data, $pointer, $value)};
}

/**
 * Drop the key or element at pointer, returning a new DB.
 * @param db {DB} the store to edit
 * @param pointer {string} the JSON Pointer to remove
 * @return {DB} a new store with the node removed
 * @throws {Error} when pointer does not resolve
 */
export func remove(db as DB, pointer as string) {
    return DB{path: $db.path, data: json.remove($db.data, $pointer)};
}

# --- persistence -----------------------------------------------------------

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
 * rename). A read-only DB (from `openString`, no backing file) has nowhere to
 * write - use `saveAs` to give it a path.
 * @param db {DB} the store to persist
 * @throws {Error} kind "flatdb" if the DB is read-only (no backing file), or on a
 *   filesystem write or rename failure
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
    writeAtomic($db.path, $db.data);
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
    return DB{path: $path, data: $db.data};
}
