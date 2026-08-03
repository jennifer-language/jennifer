// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

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
	dialect := "orm.Dialect.Postgres"
	if driver == "mysql" || driver == "mariadb" {
		dialect = "orm.Dialect.Mysql"
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

# Fresh slate (drop first so the test is repeatable).
sql.exec($conn, "DROP TABLE IF EXISTS orm_users");

# A schema with column attributes: NOT NULL columns and a defaulted bool. Its
# createTable DDL exercises the attribute rendering live.
def s as orm.Schema init orm.schema("orm_users", "id", %s);
$s = orm.notNull(orm.column($s, "id", orm.ColumnKind.Int));
$s = orm.notNull(orm.column($s, "name", orm.ColumnKind.String));
$s = orm.withDefault(orm.column($s, "active", orm.ColumnKind.Bool), "true");
sql.exec($conn, orm.createTable($s));

# CRUD through an auto-committing session.
def sess as orm.Session init orm.session($conn);
def ada as map of string to string init {};
$ada["id"] = "1";
$ada["name"] = "ada";
orm.insert($sess, $s, $ada);   # active omitted -> DEFAULT TRUE applies

# Insert a second row inside a caller-managed transaction.
def tx as sql.Tx init sql.begin($conn);
def txn as orm.Session init orm.transaction($tx);
def bob as map of string to string init {};
$bob["id"] = "2";
$bob["name"] = "bob";
orm.insert($txn, $s, $bob);
sql.commit($tx);

testing.assertEqual(orm.find($sess, $s, "1")["name"], "ada");

# update, then re-find.
def edit as map of string to string init {};
$edit["id"] = "1";
$edit["name"] = "alice";
orm.update($sess, $s, $edit);
testing.assertEqual(orm.find($sess, $s, "1")["name"], "alice");

# query: everyone ordered by id -> two rows.
def rows as list of map of string to string init orm.all($sess,
    orm.orderBy(orm.from($s), "id", "asc"));
testing.assertEqual(len($rows), 2);
testing.assertEqual($rows[0]["name"], "alice");
testing.assertEqual($rows[1]["name"], "bob");

# a filtered query.
testing.assertEqual(len(orm.all($sess, orm.where(orm.from($s), "name", "=", "bob"))), 1);

# finders: first / exists / findBy / pluck.
testing.assertEqual(orm.first($sess, orm.orderBy(orm.from($s), "id", "asc"))["name"], "alice");
testing.assertEqual(orm.exists($sess, orm.where(orm.from($s), "name", "=", "bob")), true);
testing.assertEqual(orm.exists($sess, orm.where(orm.from($s), "name", "=", "zzz")), false);
testing.assertEqual(orm.findBy($sess, $s, "name", "bob")["id"], "2");
testing.assertEqual(len(orm.pluck($sess, orm.orderBy(orm.from($s), "id", "asc"), "name")), 2);

# delete, then the count drops.
orm.delete($sess, $s, "2");
testing.assertEqual(len(orm.all($sess, orm.from($s))), 1);
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

// TestOrmEagerLoad drives orm.load's eager loading (has-many + belongs-to)
// against a real database, verifying the related rows resolve correctly. Same
// DB-service gate as TestOrmCRUD. The 1 + R query-count contract is pinned
// separately (offline) by plannedQueryCount in modules/orm_test.j.
func TestOrmEagerLoad(t *testing.T) {
	driver := os.Getenv("ORM_TEST_DRIVER")
	dsn := os.Getenv("ORM_TEST_DSN")
	if driver == "" || dsn == "" {
		t.Skip("set ORM_TEST_DRIVER and ORM_TEST_DSN to run the orm eager-load integration test")
	}
	dialect := "orm.Dialect.Postgres"
	if driver == "mysql" || driver == "mariadb" {
		dialect = "orm.Dialect.Mysql"
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
def sess as orm.Session init orm.session($conn);

sql.exec($conn, "DROP TABLE IF EXISTS el_posts");
sql.exec($conn, "DROP TABLE IF EXISTS el_authors");

# authors hasMany posts; posts belongsTo author.
def authors as orm.Schema init orm.hasMany(orm.column(orm.column(
    orm.schema("el_authors", "id", %[4]s), "id", orm.ColumnKind.Int), "name", orm.ColumnKind.String),
    "posts", "el_posts", "authorId");
def posts as orm.Schema init orm.belongsTo(orm.column(orm.column(orm.column(
    orm.schema("el_posts", "id", %[4]s), "id", orm.ColumnKind.Int), "authorId", orm.ColumnKind.Int),
    "title", orm.ColumnKind.String), "author", "el_authors", "authorId");

sql.exec($conn, orm.createTable($authors));
sql.exec($conn, orm.createTable($posts));

# 2 authors, 3 posts (ada:2, bob:1).
def a as map of string to string init {};
$a["id"] = "1"; $a["name"] = "ada"; orm.insert($sess, $authors, $a);
$a["id"] = "2"; $a["name"] = "bob"; orm.insert($sess, $authors, $a);
def p as map of string to string init {};
$p["id"] = "10"; $p["authorId"] = "1"; $p["title"] = "hello"; orm.insert($sess, $posts, $p);
$p["id"] = "11"; $p["authorId"] = "1"; $p["title"] = "world"; orm.insert($sess, $posts, $p);
$p["id"] = "12"; $p["authorId"] = "2"; $p["title"] = "hi"; orm.insert($sess, $posts, $p);

# has-many: 2 queries total (authors + posts WHERE authorId IN (...)).
def res as orm.Result init orm.load($sess, $authors,
    orm.with(orm.orderBy(orm.from($authors), "id", "asc"), "posts"));
testing.assertEqual(len(orm.rows($res)), 2);
testing.assertEqual(len(orm.related($res, orm.rows($res)[0], "posts")), 2);   # ada
testing.assertEqual(len(orm.related($res, orm.rows($res)[1], "posts")), 1);   # bob

# belongs-to: each post's author resolves.
def res2 as orm.Result init orm.load($sess, $posts,
    orm.with(orm.orderBy(orm.from($posts), "id", "asc"), "author"));
testing.assertEqual(len(orm.rows($res2)), 3);
testing.assertEqual(orm.relatedOne($res2, orm.rows($res2)[0], "author")["name"], "ada");
testing.assertEqual(orm.relatedOne($res2, orm.rows($res2)[2], "author")["name"], "bob");

sql.exec($conn, "DROP TABLE IF EXISTS el_posts");
sql.exec($conn, "DROP TABLE IF EXISTS el_authors");`, ormMod, driver, dsn, dialect)

	dir := t.TempDir()
	progPath := filepath.Join(dir, "eager.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("orm eager-load program failed with code %d", code)
	}
}

// TestOrmWritePath drives the M23.15.4 write verbs (insertMany, insertReturning,
// upsert, updateWhere, deleteWhere, save) against a real database. Same DB gate.
func TestOrmWritePath(t *testing.T) {
	driver := os.Getenv("ORM_TEST_DRIVER")
	dsn := os.Getenv("ORM_TEST_DSN")
	if driver == "" || dsn == "" {
		t.Skip("set ORM_TEST_DRIVER and ORM_TEST_DSN to run the orm write-path integration test")
	}
	dialect := "orm.Dialect.Postgres"
	if driver == "mysql" || driver == "mariadb" {
		dialect = "orm.Dialect.Mysql"
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
def sess as orm.Session init orm.session($conn);

sql.exec($conn, "DROP TABLE IF EXISTS wp_items");

# id auto-increments (for insertReturning); name is unique (for upsert conflict).
def s as orm.Schema init orm.schema("wp_items", "id", %[4]s);
$s = orm.autoIncrement(orm.column($s, "id", orm.ColumnKind.Int));
$s = orm.unique(orm.notNull(orm.column($s, "name", orm.ColumnKind.String)));
$s = orm.column($s, "qty", orm.ColumnKind.Int);
sql.exec($conn, orm.createTable($s));

# insertMany: one multi-row INSERT.
def a as map of string to string init {};
$a["name"] = "a"; $a["qty"] = "1";
def b as map of string to string init {};
$b["name"] = "b"; $b["qty"] = "2";
def c as map of string to string init {};
$c["name"] = "c"; $c["qty"] = "3";
testing.assertEqual(orm.insertMany($sess, $s, [$a, $b, $c]), 3);
testing.assertEqual(len(orm.all($sess, orm.from($s))), 3);

# insertReturning: a new row yields its generated id.
def d as map of string to string init {};
$d["name"] = "d"; $d["qty"] = "4";
def newId as string init orm.insertReturning($sess, $s, $d);
testing.assertEqual(len(orm.all($sess, orm.where(orm.from($s), "id", "=", $newId))), 1);

# upsert on the unique name: "a" is updated in place, not duplicated.
def a2 as map of string to string init {};
$a2["name"] = "a"; $a2["qty"] = "99";
orm.upsert($sess, $s, $a2, ["name"]);
testing.assertEqual(orm.all($sess, orm.where(orm.from($s), "name", "=", "a"))[0]["qty"], "99");
testing.assertEqual(len(orm.all($sess, orm.from($s))), 4); # still 4 (a updated)

# updateWhere: set b's qty via a WHERE.
def bump as map of string to string init {};
$bump["qty"] = "0";
orm.updateWhere($sess, $s, $bump, orm.where(orm.from($s), "name", "=", "b"));
testing.assertEqual(orm.all($sess, orm.where(orm.from($s), "name", "=", "b"))[0]["qty"], "0");

# deleteWhere: drop c.
orm.deleteWhere($sess, $s, orm.where(orm.from($s), "name", "=", "c"));
testing.assertEqual(len(orm.all($sess, orm.where(orm.from($s), "name", "=", "c"))), 0);

# save: no primary key -> insert; then with the key -> update.
def e as map of string to string init {};
$e["name"] = "e"; $e["qty"] = "5";
orm.save($sess, $s, $e);
def erow as map of string to string init orm.all($sess, orm.where(orm.from($s), "name", "=", "e"))[0];
$erow["qty"] = "50";
orm.save($sess, $s, $erow);
testing.assertEqual(orm.all($sess, orm.where(orm.from($s), "name", "=", "e"))[0]["qty"], "50");

sql.exec($conn, "DROP TABLE IF EXISTS wp_items");`, ormMod, driver, dsn, dialect)

	dir := t.TempDir()
	progPath := filepath.Join(dir, "writepath.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("orm write-path program failed with code %d", code)
	}
}
