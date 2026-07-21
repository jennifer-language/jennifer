# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Encode and decode JSON with the `json` library.
 * @module json
 */

use io;
use json;

# Encode a nested value (structs/maps -> object, list -> array).
io.printf("%s\n", json.encode({"name": "jen", "nums": [1, 2, 3], "ok": true}));

# Pretty-print with 2-space indent.
io.printf("%s\n", json.encodePretty({"a": 1, "b": [true, null]}));

# Decode yields an opaque json.Value; the accessors address it by JSON Pointer
# (RFC 6901), the same paths the write surface uses. as* pulls a leaf out.
def doc as json.Value init json.decode("{\"x\": 7, \"y\": 8}");
io.printf("x+y = %d\n", json.asInt($doc, "/x") + json.asInt($doc, "/y"));

# Build and edit a document with the non-mutating write verbs (each returns a
# fresh json.Value, so rebind). Start from an explicit empty container.
def cfg as json.Value init json.map();
$cfg = json.set($cfg, "/name", "jen");
$cfg = json.set($cfg, "/tags", json.list());
$cfg = json.append($cfg, "/tags", "cli");
$cfg = json.append($cfg, "/tags", "lib");
io.printf("built = %s\n", json.encode($cfg));

# Every node reports its type (list / map, not array / object); numbers decode
# to int when integral else float.
io.printf("%s %s\n", json.typeOf(json.decode("42")), json.typeOf(json.decode("4.2")));

# Rebuild a typed struct explicitly from the decoded value (no auto-coercion).
/**
 * A 2D point rebuilt from a decoded JSON object.
 * @field x {int} the x coordinate
 * @field y {int} the y coordinate
 */
def struct Point { x as int, y as int };
def d as json.Value init json.decode("{\"x\": 1, \"y\": 2}");
def p as Point init Point{ x: json.asInt($d, "/x"), y: json.asInt($d, "/y") };
io.printf("point = %s\n", json.encode($p));
