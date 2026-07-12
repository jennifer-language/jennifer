# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# flatdb.j - a file-backed JSON document store. Load a JSON file once into a
# value-semantic handle, query and edit it in memory through JSON Pointer, and
# write it back with a crash-atomic whole-file replace (temp file + rename).
#
# It is deliberately NOT a database engine. Honestly: crash-atomic snapshotting
# of small data. Atomicity is whole-file (temp + rename); there is no isolation
# (one process, reload-the-whole-file, no concurrent transactions) and
# durability is only as strong as the OS's buffering of the rename. For a real
# database, use a client over `net` (e.g. `redis`), not this. It is a thin
# file-lifecycle + ergonomics layer over the `json` write surface, not a
# re-implementation of it.
#
# Runs on both binaries (pure `json` + `fs`, no network).

use json;
use fs;

# DB is the value the caller holds: the file path plus the decoded document.
# A module holds no mutable state and `spawn` deep-copies scope, so a store
# cannot be a shared open connection - it is a value. Mutating verbs return a
# fresh DB; `save` is the only side effect. (A library type - `json.Value` - in
# an exported field is fine; the referential-closure rule concerns only module
# structs.)
export def struct DB {
    path as string,
    data as json.Value
};

# open loads the JSON document at path into a DB. A missing file yields an
# empty document (an empty object), so open never fails on a first run.
export func open(path as string) {
    def doc as json.Value init json.map();
    if (fs.exists($path)) {
        def text as string init fs.readString($path);
        $doc = json.decode($text);
    }
    return DB{ path: $path, data: $doc };
}

# --- readers (do not change the DB) ----------------------------------------

# get returns the sub-document at pointer (the whole document for "").
export func get(db as DB, pointer as string) {
    return json.get($db.data, $pointer);
}

# has reports whether pointer resolves to an existing node.
export func has(db as DB, pointer as string) {
    return json.has($db.data, $pointer);
}

# keys lists the keys of the object at pointer, in document order.
export func keys(db as DB, pointer as string) {
    return json.keys($db.data, $pointer);
}

# length is the element count of a list, or entry count of an object, at pointer.
export func length(db as DB, pointer as string) {
    return json.length($db.data, $pointer);
}

# --- writers (return a fresh DB; call save to persist) ---------------------

# set writes value at pointer (upsert an object key / replace a list index),
# returning a new DB. Strict: intermediate containers must already exist. value
# is any JSON value - a `json.Value` (build scalars with `json.decode`, objects
# and lists with `json.map` / `json.list`).
export func set(db as DB, pointer as string, value as json.Value) {
    return DB{ path: $db.path, data: json.set($db.data, $pointer, $value) };
}

# append pushes value onto the list addressed by pointer, returning a new DB.
# The list must already exist (create it first with `set($db, ptr, json.list())`).
export func append(db as DB, pointer as string, value as json.Value) {
    return DB{ path: $db.path, data: json.append($db.data, $pointer, $value) };
}

# remove drops the key or element at pointer, returning a new DB.
export func remove(db as DB, pointer as string) {
    return DB{ path: $db.path, data: json.remove($db.data, $pointer) };
}

# --- persistence -----------------------------------------------------------

# save writes the document back to its file with a crash-atomic replace: it
# writes a sibling temp file and renames it over the target, so a reader ever
# sees the whole old file or the whole new one, never a torn write. An
# interrupted save leaves the original intact (only a stray temp file remains).
export func save(db as DB) {
    def text as string init json.encode($db.data);
    def tmp as string init $db.path + ".tmp";
    fs.writeString($tmp, $text);
    fs.rename($tmp, $db.path);
}
