# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# flatdb_test.j - white-box tests for flatdb.j. Run with:
#
#     jennifer test modules/flatdb_test.j
#
# The overlay splices flatdb.j in first, so these tests reach its exported
# surface by bare identifier. flatdb.j already `use`s json / fs, so the overlay
# adds testing (for assertions) and os (for a temp path).
use testing;
use os;

# tmpPath returns a scratch file path under the OS temp dir.
func tmpPath() {
    return os.tempDir() + "/flatdb_overlay_scratch.json";
}

func testOpenMissingIsEmpty() {
    def db as DB init open("/no/such/flatdb/path/missing.json");
    testing.assertEqual(length($db, ""), 0);
}

func testSetGetRoundTrip() {
    def db as DB init open("/no/such/flatdb/path/missing.json");
    $db = set($db, "/name", json.decode("\"ada\""));
    def v as json.Value init get($db, "/name");
    testing.assertEqual(json.asString($v), "ada");
    testing.assertTrue(has($db, "/name"));
}

func testRemove() {
    def db as DB init open("/no/such/flatdb/path/missing.json");
    $db = set($db, "/gone", json.decode("1"));
    $db = remove($db, "/gone");
    testing.assertFalse(has($db, "/gone"));
}

func testAppend() {
    def db as DB init open("/no/such/flatdb/path/missing.json");
    $db = set($db, "/runs", json.list());
    $db = append($db, "/runs", json.decode("{\"n\":1}"));
    $db = append($db, "/runs", json.decode("{\"n\":2}"));
    testing.assertEqual(length($db, "/runs"), 2);
    testing.assertEqual(json.asInt(get($db, "/runs/1/n")), 2);
}

func testSaveThenReopen() {
    def path as string init tmpPath();
    def db as DB init open($path);
    $db = set($db, "/count", json.decode("42"));
    save($db);
    def reloaded as DB init open($path);
    testing.assertEqual(json.asInt(get($reloaded, "/count")), 42);
    fs.remove($path);
}

# A whitespace-only / zero-byte file (e.g. `touch state.json` before the first
# save) opens like a missing file rather than throwing a JSON parse error.
func testOpenEmptyFileIsEmpty() {
    def path as string init tmpPath();
    fs.writeString($path, "");
    def db as DB init open($path);
    testing.assertFalse(has($db, "/anything"));
    fs.writeString($path, "  \n\t ");
    def dbBlank as DB init open($path);
    testing.assertFalse(has($dbBlank, "/anything"));
    fs.remove($path);
}

func testWritersAreImmutable() {
    def db as DB init open("/no/such/flatdb/path/missing.json");
    def grown as DB init set($db, "/x", json.decode("1"));
    testing.assertFalse(has($db, "/x"));       # original untouched
    testing.assertTrue(has($grown, "/x"));
}
