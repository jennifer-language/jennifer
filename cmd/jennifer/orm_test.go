// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestOrmCRUD drives the orm module's live CRUD against a real database. It is
// DB-service-gated: it runs only when ORM_TEST_DRIVER (`postgres` / `mysql`) and
// ORM_TEST_DSN are set, and skips otherwise, so CI without a database still
// passes (the pure query-builder surface is covered by modules/orm_test.j). To
// run it locally, point it at a throwaway database - e.g.
//
//	ORM_TEST_DRIVER=postgres \
//	ORM_TEST_DSN='postgres://user:pass@localhost:5432/test?sslmode=disable' \
//	go test ./cmd/jennifer/ -run TestOrmCRUD
func TestOrmCRUD(t *testing.T) {
	driver := os.Getenv("ORM_TEST_DRIVER")
	dsn := os.Getenv("ORM_TEST_DSN")
	if driver == "" || dsn == "" {
		t.Skip("set ORM_TEST_DRIVER and ORM_TEST_DSN to run the orm CRUD integration test")
	}
	dialect := "postgres"
	if driver == "mysql" || driver == "mariadb" {
		dialect = "mysql"
	}

	ormMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "orm.j"))
	if err != nil {
		t.Fatal(err)
	}
	prog := fmt.Sprintf(`use testing;
use sql;
import %q as orm;

def conn as sql.Connection init sql.open(%q, %q);
defer sql.close($conn);

# A schema; a fresh table (drop first so the test is repeatable).
def s as orm.Schema init orm.column(orm.column(
    orm.schema("orm_users", "id", %q), "id", "int"), "name", "string");
sql.exec($conn, "DROP TABLE IF EXISTS orm_users");
sql.exec($conn, orm.createTable($s));

# insert two rows (explicit ids), find one back.
def ada as map of string to string init {};
$ada["id"] = "1";
$ada["name"] = "ada";
orm.insert($conn, $s, $ada);
def bob as map of string to string init {};
$bob["id"] = "2";
$bob["name"] = "bob";
orm.insert($conn, $s, $bob);

def got as map of string to string init orm.find($conn, $s, "1");
testing.assertEqual($got["name"], "ada");

# update, then re-find.
def edit as map of string to string init {};
$edit["id"] = "1";
$edit["name"] = "alice";
orm.update($conn, $s, $edit);
testing.assertEqual(orm.find($conn, $s, "1")["name"], "alice");

# query: everyone ordered by id -> two rows.
def rows as list of map of string to string init orm.all($conn,
    orm.orderBy(orm.from($s), "id", "asc"));
testing.assertEqual(len($rows), 2);
testing.assertEqual($rows[0]["name"], "alice");
testing.assertEqual($rows[1]["name"], "bob");

# a filtered query.
def named as list of map of string to string init orm.all($conn,
    orm.where(orm.from($s), "name", "=", "bob"));
testing.assertEqual(len($named), 1);

# delete, then the count drops.
orm.delete($conn, $s, "2");
testing.assertEqual(len(orm.all($conn, orm.from($s))), 1);
sql.exec($conn, "DROP TABLE IF EXISTS orm_users");`, ormMod, driver, dsn, dialect)

	dir := t.TempDir()
	progPath := filepath.Join(dir, "orm.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("orm CRUD program failed with code %d", code)
	}
}
