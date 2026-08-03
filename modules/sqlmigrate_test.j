# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# sqlmigrate_test.j - white-box tests for sqlmigrate.j. Run with:
#
#     jennifer test modules/sqlmigrate_test.j
#
# Covers the pure helpers (version validation, lexical ordering, sorting,
# literal escaping) offline. The live runner (migrate / rollbackMigrations /
# migrationStatus) is a separate, DB-gated integration test
# (cmd/jennifer/sqlmigrate_test.go), not the unit overlay.
use testing;

func testValidVersion() {
    testing.assertEqual(validVersion("001"), true);
    testing.assertEqual(validVersion("2026.01.01-init"), true);
    testing.assertEqual(validVersion("v1_2_3"), true);
    testing.assertEqual(validVersion(""), false);
    testing.assertEqual(validVersion("bad;drop"), false);
    testing.assertEqual(validVersion("has space"), false);
    testing.assertEqual(validVersion("quote'd"), false);
}

func testStrLess() {
    testing.assertEqual(strLess("001", "002"), true);
    testing.assertEqual(strLess("002", "001"), false);
    testing.assertEqual(strLess("001", "001"), false);
    testing.assertEqual(strLess("a", "ab"), true);
    # lexical, not numeric: "10" sorts before "9" (documented - zero-pad versions)
    testing.assertEqual(strLess("10", "9"), true);
}

func testSortMigrations() {
    def a as Migration init Migration{version: "002", description: "b", up: [], down: []};
    def b as Migration init Migration{version: "001", description: "a", up: [], down: []};
    def c as Migration init Migration{version: "010", description: "c", up: [], down: []};
    def sorted as list of Migration init sortMigrations([$a, $b, $c]);
    testing.assertEqual($sorted[0].version, "001");
    testing.assertEqual($sorted[1].version, "002");
    testing.assertEqual($sorted[2].version, "010");
    # sorting is stable / non-mutating: the input is untouched.
    testing.assertEqual($a.version, "002");
}

func testEscapeLiteral() {
    testing.assertEqual(escapeLiteral("plain"), "plain");
    testing.assertEqual(escapeLiteral("it's ok"), "it''s ok");
    testing.assertEqual(escapeLiteral("a''b"), "a''''b");
}

func escapeBackslash() {
    escapeLiteral("a\\b");
}
func escapeControl() {
    escapeLiteral("a\nb");
}
func testEscapeLiteralRejectsUnsafe() {
    testing.assertThrows("escapeBackslash", "sqlmigrate");
    testing.assertThrows("escapeControl", "sqlmigrate");
}

func testMigrationStatusShape() {
    def st as MigrationStatus init MigrationStatus{version: "001", description: "x", applied: true};
    testing.assertEqual($st.applied, true);
    testing.assertEqual($st.version, "001");
}
