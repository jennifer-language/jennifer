# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
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
use task;

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
    $db = append($db, "/runs", json.decode('{"n":1}'));
    $db = append($db, "/runs", json.decode('{"n":2}'));
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
    testing.assertFalse(has($db, "/x")); # original untouched
    testing.assertTrue(has($grown, "/x"));
}

func testOpenStringReadsInMemory() {
    def db as DB init openString('{"count": 2, "users": {"1": {"name": "ada"}}}');
    testing.assertEqual(json.asInt(get($db, "/count")), 2);
    testing.assertEqual(json.asString(get($db, "/users/1/name")), "ada");
    testing.assertEqual(length($db, "/users"), 1);
    # whitespace-only text is an empty document, like open() on a missing file
    testing.assertEqual(length(openString("   "), ""), 0);
}

func testOpenStringIsValueSemantic() {
    def db as DB init openString('{"count": 2}');
    def db2 as DB init set($db, "/count", json.decode("5"));
    testing.assertEqual(json.asInt(get($db2, "/count")), 5);
    testing.assertEqual(json.asInt(get($db, "/count")), 2); # original unchanged
}

# saveReadOnly is a helper for testReadOnlySaveThrows (not a test itself).
func saveReadOnly() {
    save(openString('{}'));
}
func testReadOnlySaveThrows() {
    testing.assertThrows("saveReadOnly", "flatdb");
}

func testSaveAsPromotesReadOnlyToWritable() {
    def p as string init os.tempDir() + "/flatdb_saveas_" + uuid.v4() + ".json";
    def ro as DB init openString('{"n": 1}');
    def w as DB init saveAs($ro, $p);
    testing.assertTrue(fs.exists($p));
    testing.assertEqual($w.path, $p); # returned handle bound to the new path
    testing.assertEqual(length($ro, ""), 1); # original untouched (value semantics)
    def w2 as DB init set($w, "/n", json.decode("2"));
    save($w2); # writable now (has a path)
    testing.assertEqual(json.asInt(get(open($p), "/n")), 2);
    fs.remove($p);
}

func testSaveAsForksIndependently() {
    def pa as string init os.tempDir() + "/flatdb_a_" + uuid.v4() + ".json";
    def pb as string init os.tempDir() + "/flatdb_b_" + uuid.v4() + ".json";
    def a as DB init set(open($pa), "/who", json.decode("\"a\""));
    save($a);
    def b as DB init saveAs($a, $pb); # copy to a new file
    save(set($b, "/who", json.decode("\"b\"")));
    testing.assertEqual(json.asString(get(open($pa), "/who")), "a"); # original file unchanged
    testing.assertEqual(json.asString(get(open($pb), "/who")), "b");
    fs.remove($pa);
    fs.remove($pb);
}

# saveAsEmpty is a helper for testSaveAsEmptyPathThrows (not a test itself).
func saveAsEmpty() {
    saveAs(openString('{}'), "");
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
    $db = save($db); # first write of a new file; thread the rebound DB for the next save
    def st1 as fs.Stat init fs.stat($path);
    testing.assertEqual($st1.mode, 0o600);
    # A later save preserves the operator's chosen mode instead of resetting to 0644.
    fs.chmod($path, 0o640);
    $db = set($db, "/k", json.decode("2"));
    $db = save($db);
    def st2 as fs.Stat init fs.stat($path);
    testing.assertEqual($st2.mode, 0o640);
    fs.remove($path);
}

# --- concurrency: version-conflict + locked update -------------------------

# testSaveConflictThrows: two writers open the same file at the same version; the
# first save commits, the second must fail loudly instead of silently clobbering.
func testSaveConflictThrows() {
    def path as string init os.tempDir() + "/flatdb_conflict_" + uuid.v4() + ".json";
    save(set(open($path), "/n", json.decode("0")));
    def w1 as DB init set(open($path), "/a", json.decode("1"));
    def w2 as DB init set(open($path), "/b", json.decode("2"));
    save($w1);
    def threw as bool init false;
    try {
        save($w2);
    } catch (e) {
        $threw = true;
        testing.assertEqual($e.kind, "flatdb");
    }
    testing.assertTrue($threw);
    fs.remove($path);
}

# incrementCount is the transform for the concurrency test: read the counter (0 if
# absent) and write it back +1. Under `update` this whole read-modify-write is
# locked, so concurrent runs cannot lose an increment.
func incrementCount(db as DB) {
    def n as int init 0;
    if (has($db, "/count")) {
        $n = json.asInt(get($db, "/count"));
    }
    return set($db, "/count", json.decode(convert.toString($n + 1)));
}

# testUpdateSerializesConcurrentWriters: 10 spawned workers each run a locked
# read-modify-write increment of one counter. Without the lock this loses writes
# (last save wins); with `update` every increment survives -> exactly 10.
func testUpdateSerializesConcurrentWriters() {
    def path as string init os.tempDir() + "/flatdb_update_" + uuid.v4() + ".json";
    save(set(open($path), "/count", json.decode("0")));
    def ts as list of task of null init [];
    def i as int init 0;
    while ($i < 10) {
        $ts[] = spawn {
            update($path, incrementCount);
            return;
        };
        $i = $i + 1;
    }
    for (def t in $ts) {
        task.wait($t);
    }
    testing.assertEqual(json.asInt(get(open($path), "/count")), 10);
    fs.remove($path);
    try {
        fs.remove($path + ".lock");
    } catch (e) { # lint-disable: L103
    }
}

# testLockUnlockRoundTrip: lock is exclusive while held (a second writeNew on the
# lock path fails) and free again after unlock.
func testLockUnlockRoundTrip() {
    def path as string init os.tempDir() + "/flatdb_lock_" + uuid.v4() + ".json";
    def lk as Lock init lock($path);
    def stillHeld as bool init false;
    try {
        fs.writeNew($path + ".lock", "intruder");
    } catch (e) {
        $stillHeld = true; # the lock file exists, so the intruder create failed
    }
    testing.assertTrue($stillHeld);
    unlock($lk);
    testing.assertFalse(fs.exists($path + ".lock")); # released
}

# A lock file that cannot be created at all (here: its directory does not exist,
# or a path component is a file) has no holder to wait for, so lock fails at
# once with the create's own error rather than after LOCK_WAIT_MS as contention.
func testUncreatableLockFailsFast() {
    def plainFile as string init os.tempDir() + "/flatdb_notadir_" + uuid.v4();
    fs.writeString($plainFile, "x");
    for (def path in ["/no/such/flatdb/dir/db.json", $plainFile + "/db.json"]) {
        def t0 as time.Time init time.now();
        def msg as string init "";
        try {
            lock($path);
        } catch (e) {
            $msg = $e.message;
        }
        testing.assertTrue(strings.contains($msg, "cannot create the lock file"));
        testing.assertFalse(strings.contains($msg, "holder: unknown"));
        testing.assertTrue(time.milliseconds(time.sub(time.now(), $t0)) < LOCK_WAIT_MS);
    }
    fs.remove($plainFile);
}

func testDanglingSymlinkLockPathIsNamed() {
    def path as string init lockScratch();
    fs.symlink("/no/such/flatdb/target", $path + ".lock");
    def msg as string init "";
    try {
        lock($path);
    } catch (e) {
        $msg = $e.message;
    }
    fs.remove($path + ".lock");
    testing.assertTrue(strings.contains($msg, "dangling symbolic link"));
}

# lockScratch returns a fresh store path whose lock files start absent.
func lockScratch() {
    return os.tempDir() + "/flatdb_break_" + uuid.v4() + ".json";
}

# holderAged builds a holder id stamped `age` seconds in the past.
func holderAged(age as int) {
    return uuid.v4() + " " + convert.toString(time.unix(time.now()) - $age);
}

func testStaleLockIsBroken() {
    def path as string init lockScratch();
    fs.writeNew($path + ".lock", holderAged(LOCK_STALE_SECONDS + 60));
    breakIfStale($path + ".lock");
    testing.assertFalse(fs.exists($path + ".lock"));
    testing.assertFalse(fs.exists($path + ".lock.break")); # the break lock is released
    unlock(lock($path)); # and the store can be locked again
}

func testFreshLockIsNotBroken() {
    def path as string init lockScratch();
    def holder as string init holderAged(0);
    fs.writeNew($path + ".lock", $holder);
    breakIfStale($path + ".lock");
    testing.assertEqual(fs.readString($path + ".lock"), $holder);
    fs.remove($path + ".lock");
}

# The race from the report, made deterministic: waiter A judged holder H0 stale,
# but before A breaks, waiter B broke H0 and took the lock as H1. A must not
# remove H1.
func testBreakRefusesAHolderThatReplacedTheJudgedOne() {
    def path as string init lockScratch();
    def stale as string init holderAged(LOCK_STALE_SECONDS + 60);
    def live as string init holderAged(0);
    fs.writeNew($path + ".lock", $live);
    testing.assertFalse(breakHolder($path + ".lock", $stale));
    testing.assertEqual(fs.readString($path + ".lock"), $live);
    fs.remove($path + ".lock");
}

# While another breaker holds `.break`, a second breaker leaves the lock alone.
func testActiveBreakerExcludesOthers() {
    def path as string init lockScratch();
    def stale as string init holderAged(LOCK_STALE_SECONDS + 60);
    fs.writeNew($path + ".lock", $stale);
    fs.writeNew($path + ".lock.break", holderAged(0));
    testing.assertFalse(breakHolder($path + ".lock", $stale));
    testing.assertEqual(fs.readString($path + ".lock"), $stale);
    fs.remove($path + ".lock");
    fs.remove($path + ".lock.break");
}

# A `.break` abandoned by a crashed breaker is cleared, so breaking resumes.
func testAbandonedBreakIsCleared() {
    def path as string init lockScratch();
    def stale as string init holderAged(LOCK_STALE_SECONDS + 60);
    fs.writeNew($path + ".lock", $stale);
    fs.writeNew($path + ".lock.break", holderAged(BREAK_STALE_SECONDS + 60));
    testing.assertFalse(breakHolder($path + ".lock", $stale)); # clears the old .break
    testing.assertFalse(fs.exists($path + ".lock.break"));
    testing.assertTrue(breakHolder($path + ".lock", $stale)); # then breaks the lock
    testing.assertFalse(fs.exists($path + ".lock"));
}

# removeIfHolder never deletes a different holder: it puts the file back.
func testRemoveIfHolderRestoresAMismatch() {
    def path as string init lockScratch();
    fs.writeNew($path, "B");
    testing.assertFalse(removeIfHolder($path, "A"));
    testing.assertEqual(fs.readString($path), "B");
    testing.assertTrue(removeIfHolder($path, "B"));
    testing.assertFalse(fs.exists($path));
}
