// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// A .j program drives the flatdb module through its acceptance criteria: open
// of a missing path yields an empty store; set / get / remove round-trip
// through a JSON Pointer; save then a fresh open returns the same data; and a
// stray temp file (an interrupted save that wrote the temp but never renamed)
// leaves the original intact. A mismatch throws and fails loadForTest.
func TestFlatdbStore(t *testing.T) {
	flatdbMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "flatdb.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "store.json")
	prog := fmt.Sprintf(`use testing;
use json;
use fs;
import %q as flatdb;

def path as string init %q;

# open of a missing path -> an empty store
def db as flatdb.DB init flatdb.open($path);
testing.assertEqual(flatdb.length($db, ""), 0);

# set / get round-trip through a JSON Pointer
$db = flatdb.set($db, "/name", json.decode("\"ada\""));
testing.assertEqual(json.asString(flatdb.get($db, "/name")), "ada");

# save, then a fresh open returns the same data
flatdb.save($db);
def reopened as flatdb.DB init flatdb.open($path);
testing.assertEqual(json.asString(flatdb.get($reopened, "/name")), "ada");

# remove drops the key (in memory)
$reopened = flatdb.remove($reopened, "/name");
testing.assertFalse(flatdb.has($reopened, "/name"));

# an interrupted save (a stray temp file, no rename) must not corrupt the file
fs.writeString($path + ".tmp", "GARBAGE not json");
def stillGood as flatdb.DB init flatdb.open($path);
testing.assertEqual(json.asString(flatdb.get($stillGood, "/name")), "ada");`, flatdbMod, dbPath)

	progPath := filepath.Join(dir, "prog.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("flatdb program failed with code %d", code)
	}
}
