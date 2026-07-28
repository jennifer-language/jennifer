#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Server-side sessions over a selectable backend with the session module. This
 * demo uses the in-process backend (kvstore.inProcessStore), so it runs with no
 * server; swap in kvstore.redisStore($rc) / memcacheStore($mc) / fileStore(path)
 * for a distributed or persistent store.
 * @module session_demo
 */
use io;
use json;
import "../../modules/session.j" as session;
import "../../modules/kvstore.j" as kvstore;

def store as kvstore.Store init kvstore.inProcessStore();

# Start a session (30 minutes) and populate it with structured data.
def id as string init session.create($store, 1800);
io.printf("created session %s\n", $id);

def data as json.Value init session.load($store, $id);
$data = json.set($data, "/user", "ada");
$data = json.set($data, "/name", "José"); # non-ASCII survives (base64-wrapped)
$data = json.set($data, "/prefs", json.map());
$data = json.set($data, "/prefs/theme", "dark"); # nested - richer than a flat map
session.save($store, $id, $data, 1800);

# A later request loads it back.
def back as json.Value init session.load($store, $id);
io.printf("user  -> %s\n", json.asString($back, "/user"));
io.printf("name  -> %s\n", json.asString($back, "/name"));
io.printf("theme -> %s\n", json.asString($back, "/prefs/theme"));

# Keep it alive without rewriting, then end it.
io.printf("touch  -> %t\n", session.touch($store, $id, 1800));
io.printf("destroy-> %t\n", session.destroy($store, $id));
io.printf("gone   -> %d entries\n", json.length(session.load($store, $id), ""));
