# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
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

func testOpenStringReadsInMemory() {
    def db as DB init openString("{\"count\": 2, \"users\": {\"1\": {\"name\": \"ada\"}}}");
    testing.assertEqual(json.asInt(get($db, "/count")), 2);
    testing.assertEqual(json.asString(get($db, "/users/1/name")), "ada");
    testing.assertEqual(length($db, "/users"), 1);
    # whitespace-only text is an empty document, like open() on a missing file
    testing.assertEqual(length(openString("   "), ""), 0);
}

func testOpenStringIsValueSemantic() {
    def db as DB init openString("{\"count\": 2}");
    def db2 as DB init set($db, "/count", json.decode("5"));
    testing.assertEqual(json.asInt(get($db2, "/count")), 5);
    testing.assertEqual(json.asInt(get($db, "/count")), 2);   # original unchanged
}

# saveReadOnly is a helper for testReadOnlySaveThrows (not a test itself).
func saveReadOnly() {
    save(openString("{}"));
}
func testReadOnlySaveThrows() {
    testing.assertThrows("saveReadOnly", "flatdb");
}

func testSaveAsPromotesReadOnlyToWritable() {
    def p as string init os.tempDir() + "/flatdb_saveas_" + uuid.v4() + ".json";
    def ro as DB init openString("{\"n\": 1}");
    def w as DB init saveAs($ro, $p);
    testing.assertTrue(fs.exists($p));
    testing.assertEqual($w.path, $p);            # returned handle bound to the new path
    testing.assertEqual(length($ro, ""), 1);     # original untouched (value semantics)
    def w2 as DB init set($w, "/n", json.decode("2"));
    save($w2);                                   # writable now (has a path)
    testing.assertEqual(json.asInt(get(open($p), "/n")), 2);
    fs.remove($p);
}

func testSaveAsForksIndependently() {
    def pa as string init os.tempDir() + "/flatdb_a_" + uuid.v4() + ".json";
    def pb as string init os.tempDir() + "/flatdb_b_" + uuid.v4() + ".json";
    def a as DB init set(open($pa), "/who", json.decode("\"a\""));
    save($a);
    def b as DB init saveAs($a, $pb);            # copy to a new file
    save(set($b, "/who", json.decode("\"b\"")));
    testing.assertEqual(json.asString(get(open($pa), "/who")), "a");   # original file unchanged
    testing.assertEqual(json.asString(get(open($pb), "/who")), "b");
    fs.remove($pa);
    fs.remove($pb);
}

# saveAsEmpty is a helper for testSaveAsEmptyPathThrows (not a test itself).
func saveAsEmpty() {
    def x as DB init saveAs(openString("{}"), "");
}
func testSaveAsEmptyPathThrows() {
    testing.assertThrows("saveAsEmpty", "flatdb");
}

# OM-014: save must not silently widen a tightened store's permissions, and a
# brand-new file defaults to 0600 (not fs.writeString's 0644).
func testSavePreservesMode() {
    def path as string init os.tempDir() + "/flatdb_mode_scratch.json";
    def db as DB init open($path);
    $db = set($db, "/k", json.decode("1"));
    save($db);                                  # first write of a new file
    def st1 as fs.Stat init fs.stat($path);
    testing.assertEqual($st1.mode, 0o600);
    # A later save preserves the operator's chosen mode instead of resetting to 0644.
    fs.chmod($path, 0o640);
    $db = set($db, "/k", json.decode("2"));
    save($db);
    def st2 as fs.Stat init fs.stat($path);
    testing.assertEqual($st2.mode, 0o640);
    fs.remove($path);
}
