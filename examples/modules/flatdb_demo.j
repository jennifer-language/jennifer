#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# flatdb_demo.j - a file-backed JSON store with the flatdb module.
#
# Models the "benchmark history" use case: append a self-describing record per
# run to a store, save it with a crash-atomic replace, then reopen and query.
# Self-contained (writes to a temp file it cleans up), so it needs no external
# service and runs on either binary.
#
#     jennifer run examples/modules/flatdb_demo.j
use io;
use json;
use fs;
use os;
import "../../modules/flatdb.j" as flatdb;

def path as string init os.tempDir() + "/flatdb_demo.json";
if (fs.exists($path)) {
    fs.remove($path);
}

# Open (empty on first run) and start a runs list.
def db as flatdb.DB init flatdb.open($path);
$db = flatdb.set($db, "/runs", json.list());

# Append two benchmark-like records.
def recA as json.Value init json.map();
$recA = json.set($recA, "/cpu", "Ryzen 5 7600X3D");
$recA = json.set($recA, "/ms", 118);
$db = flatdb.append($db, "/runs", $recA);

def recB as json.Value init json.map();
$recB = json.set($recB, "/cpu", "Apple M2");
$recB = json.set($recB, "/ms", 205);
$db = flatdb.append($db, "/runs", $recB);

# Persist with an atomic whole-file replace.
flatdb.save($db);

# Reopen from disk and query through JSON Pointer.
def store as flatdb.DB init flatdb.open($path);
io.printf("records: %d\n", flatdb.length($store, "/runs"));
io.printf("first:   %s (%d ms)\n",
    json.asString(flatdb.get($store, "/runs/0/cpu")),
    json.asInt(flatdb.get($store, "/runs/0/ms")));
io.printf("second:  %s (%d ms)\n",
    json.asString(flatdb.get($store, "/runs/1/cpu")),
    json.asInt(flatdb.get($store, "/runs/1/ms")));

fs.remove($path);
io.printf("done\n");
