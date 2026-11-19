// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestSqlmigrateRunner drives the sqlmigrate module's live runner against a real
// database. Like TestOrmCRUD it is DB-service-gated: it runs only when
// ORM_TEST_DRIVER (`postgres` / `mysql`) and ORM_TEST_DSN are set (the same
// throwaway database), and skips otherwise, so CI without a database still
// passes (the pure helpers are covered by modules/sqlmigrate_test.j). To run it:
//
//	ORM_TEST_DRIVER=postgres \
//	ORM_TEST_DSN='postgres://user:pass@localhost:5432/test?sslmode=disable' \
//	go test ./cmd/jennifer/ -run TestSqlmigrateRunner
func TestSqlmigrateRunner(t *testing.T) {
	driver := os.Getenv("ORM_TEST_DRIVER")
	dsn := os.Getenv("ORM_TEST_DSN")
	if driver == "" || dsn == "" {
		t.Skip("set ORM_TEST_DRIVER and ORM_TEST_DSN to run the sqlmigrate integration test")
	}

	migMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "sqlmigrate.j"))
	if err != nil {
		t.Fatal(err)
	}
	prog := fmt.Sprintf(`use testing;
use sql;
import %q as mig;

def conn as sql.Connection init sql.open(%q, %q);
defer sql.close($conn);

# Fresh slate (drop first so the test is repeatable).
sql.exec($conn, "DROP TABLE IF EXISTS mig_widgets");
sql.exec($conn, "DROP TABLE IF EXISTS schema_migrations");

def m1 as mig.Migration init mig.Migration{
    version: "001",
    description: "create widgets",
    up: ["CREATE TABLE mig_widgets (id INTEGER PRIMARY KEY, name TEXT)"],
    down: ["DROP TABLE mig_widgets"]
};
def m2 as mig.Migration init mig.Migration{
    version: "002",
    description: "add color",
    up: ["ALTER TABLE mig_widgets ADD COLUMN color TEXT"],
    down: ["ALTER TABLE mig_widgets DROP COLUMN color"]
};

# Unsorted input is applied in version order; the second run is idempotent.
testing.assertEqual(mig.migrate($conn, [$m2, $m1]), 2);
testing.assertEqual(mig.migrate($conn, [$m1, $m2]), 0);

# Both show applied, in version order.
def st as list of mig.MigrationStatus init mig.migrationStatus($conn, [$m1, $m2]);
testing.assertEqual(len($st), 2);
testing.assertEqual($st[0].version, "001");
testing.assertEqual($st[0].applied, true);
testing.assertEqual($st[1].applied, true);

# The 002 up added the color column, so an insert using it succeeds.
sql.exec($conn, "INSERT INTO mig_widgets (id, name, color) VALUES (1, 'a', 'red')");
def rows as sql.Rows init sql.query($conn, "SELECT color FROM mig_widgets WHERE id = 1");
testing.assertEqual(sql.next($rows), true);
testing.assertEqual(sql.asString($rows, "color"), "red");
sql.closeRows($rows);

# Roll back one step (002) -> only 001 remains applied.
testing.assertEqual(mig.rollbackMigrations($conn, [$m1, $m2], 1), 1);
def st2 as list of mig.MigrationStatus init mig.migrationStatus($conn, [$m1, $m2]);
testing.assertEqual($st2[0].applied, true);
testing.assertEqual($st2[1].applied, false);

# Re-apply the one pending migration, then roll everything back.
testing.assertEqual(mig.migrate($conn, [$m1, $m2]), 1);
testing.assertEqual(mig.rollbackMigrations($conn, [$m1, $m2], 2), 2);
testing.assertEqual(mig.migrationStatus($conn, [$m1, $m2])[0].applied, false);

sql.exec($conn, "DROP TABLE IF EXISTS mig_widgets");
sql.exec($conn, "DROP TABLE IF EXISTS schema_migrations");`, migMod, driver, dsn)

	dir := t.TempDir()
	progPath := filepath.Join(dir, "mig.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("sqlmigrate program failed with code %d", code)
	}
}

// TestSqlmigrateConcurrentRuns starts two migrate runs at once on separate
// connections. The migration lock makes the second wait and then skip the
// already-applied step; without it both create the table and one fails. Gated
// like TestSqlmigrateRunner.
func TestSqlmigrateConcurrentRuns(t *testing.T) {
	driver := os.Getenv("ORM_TEST_DRIVER")
	dsn := os.Getenv("ORM_TEST_DSN")
	if driver == "" || dsn == "" {
		t.Skip("set ORM_TEST_DRIVER and ORM_TEST_DSN to run the sqlmigrate integration test")
	}
	// A slow step widens the race window.
	pause := "DO SLEEP(1)"
	if driver == "postgres" || driver == "postgresql" {
		pause = "SELECT pg_sleep(1)"
	}

	migMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "sqlmigrate.j"))
	if err != nil {
		t.Fatal(err)
	}
	prog := fmt.Sprintf(`use testing;
use sql;
use task;
import %q as mig;

def driver as string init %q;
def dsn as string init %q;
def conn as sql.Connection init sql.open($driver, $dsn);
defer sql.close($conn);
sql.exec($conn, "DROP TABLE IF EXISTS mig_race");
sql.exec($conn, "DROP TABLE IF EXISTS schema_migrations");

func runOnce(driver as string, dsn as string) {
    def c as sql.Connection init sql.open($driver, $dsn);
    defer sql.close($c);
    def m as mig.Migration init mig.Migration{
        version: "001",
        description: "race",
        up: ["CREATE TABLE mig_race (id INTEGER)", %q],
        down: ["DROP TABLE mig_race"]
    };
    return mig.migrate($c, [$m]);
}

def a as task of int init spawn { return runOnce($driver, $dsn); };
def b as task of int init spawn { return runOnce($driver, $dsn); };
testing.assertEqual(task.wait($a) + task.wait($b), 1);

sql.exec($conn, "DROP TABLE IF EXISTS mig_race");
sql.exec($conn, "DROP TABLE IF EXISTS schema_migrations");`, migMod, driver, dsn, pause)

	dir := t.TempDir()
	progPath := filepath.Join(dir, "race.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("concurrent sqlmigrate program failed with code %d", code)
	}
}
