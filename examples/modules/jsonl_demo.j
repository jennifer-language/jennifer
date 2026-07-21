#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The jsonl module (modules/jsonl.j): encode records as JSON Lines, decode them
 * back, write / append to a file, and stream a large file one record at a time.
 * Run: jennifer run examples/modules/jsonl_demo.j
 * @module jsonl_demo
 */
use io;
use json;
use os;
import "../../modules/jsonl.j" as jsonl;

# Build a few records as json.Values (any top-level JSON type is a valid line).
def rows as list of json.Value init [];
$rows[] = json.decode("{\"event\": \"login\", \"user\": \"ada\"}");
$rows[] = json.decode("{\"event\": \"click\", \"user\": \"ada\", \"x\": 42}");
$rows[] = json.decode("{\"event\": \"logout\", \"user\": \"ada\"}");

def text as string init jsonl.encode($rows);
io.printf("=== JSONL ===\n%s\n", $text);

# Decode back and read a field from each record by JSON Pointer.
def parsed as list of json.Value init jsonl.decode($text);
io.printf("=== decoded %d records ===\n", len($parsed));
for (def rec in $parsed) {
    io.printf("- %s\n", json.asString(json.get($rec, "/event")));
}

# Write to a file, append one more, then stream it back a record at a time.
def path as string init os.tempDir() + "/jsonl_demo.jsonl";
jsonl.writeFile($path, $rows);
jsonl.appendFile($path, [json.decode("{\"event\": \"purchase\", \"user\": \"ada\", \"amount\": 9}")]);

io.printf("=== streaming %s ===\n", $path);
def reader as jsonl.Reader init jsonl.openReader($path);
def count as int init 0;
while (true) {
    def rec as jsonl.Record init jsonl.readRecord($reader);
    if ($rec.done) {
        break;
    }
    $count = $count + 1;
    io.printf("  record %d: %s\n", $count, json.encode($rec.value));
}
jsonl.closeReader($reader);
io.printf("streamed %d records total\n", $count);
