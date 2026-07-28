# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# orm_test.j - white-box tests for orm.j. Run with:
#
#     jennifer test modules/orm_test.j
#
# Covers the entire query-builder-to-SQL surface (pure string generation) for
# both dialects, offline. Live CRUD against a real database is a separate,
# DB-gated integration test (cmd/jennifer/orm_test.go), not the unit overlay.
use testing;
use strings;

# A three-column schema in a chosen dialect.
func usersSchema(dialect as Dialect) {
    return column(
        column(column(schema("users", "id", $dialect), "id", ColumnKind.Int), "name", ColumnKind.String),
        "age",
        ColumnKind.Int);
}

func testSchemaAndColumns() {
    def s as Schema init usersSchema(Dialect.Postgres);
    testing.assertEqual($s.table, "users");
    testing.assertEqual($s.primaryKey, "id");
    testing.assertEqual(len($s.columns), 3);
    testing.assertEqual($s.columns[1].name, "name");
    testing.assertEqual($s.columns[1].kind, ColumnKind.String);
}

# An invalid dialect is now unrepresentable: `Dialect` is a closed enum, so a
# bad value like "sqlite" is a compile-time error, not a runtime throw - which is
# exactly why the old testDialectRejected / badDialect pair was removed.

func testSelectAll() {
    testing.assertEqual(toSql(from(usersSchema(Dialect.Mysql))).sql, "SELECT * FROM users");
}

func testWherePostgresPlaceholders() {
    def q as Query init where(
        where(from(usersSchema(Dialect.Postgres)), "age", ">", "18"),
        "name",
        "=",
        "ada");
    def r as Rendered init toSql($q);
    testing.assertEqual($r.sql, "SELECT * FROM users WHERE age > $1 AND name = $2");
    testing.assertEqual(len($r.params), 2);
    testing.assertEqual($r.params[0], "18");
    testing.assertEqual($r.params[1], "ada");
}

func testWhereMysqlPlaceholders() {
    def q as Query init where(
        where(from(usersSchema(Dialect.Mysql)), "age", ">", "18"),
        "name",
        "=",
        "ada");
    testing.assertEqual(toSql($q).sql, "SELECT * FROM users WHERE age > ? AND name = ?");
}

func testOrderLimitOffset() {
    def q as Query init offset(
        limit(orderBy(orderBy(from(usersSchema(Dialect.Postgres)), "age", "desc"), "name", "asc"), 10),
        20);
    testing.assertEqual(
        toSql($q).sql,
        "SELECT * FROM users ORDER BY age DESC, name ASC LIMIT 10 OFFSET 20");
}

func testJoin() {
    def q as Query init join(from(usersSchema(Dialect.Mysql)), "orders", "users.id", "orders.userId");
    testing.assertEqual(
        toSql($q).sql,
        "SELECT * FROM users INNER JOIN orders ON users.id = orders.userId");
}

func testWhereAfterJoinNumbersPlaceholders() {
    # A join has no params, so the first WHERE placeholder is still $1.
    def q as Query init where(
        join(from(usersSchema(Dialect.Postgres)), "orders", "users.id", "orders.userId"),
        "age",
        ">",
        "21");
    testing.assertEqual(
        toSql($q).sql,
        "SELECT * FROM users INNER JOIN orders ON users.id = orders.userId WHERE age > $1");
}

func testBuilderIsNonMutating() {
    def base as Query init from(usersSchema(Dialect.Mysql));
    def withWhere as Query init where($base, "id", "=", "1");
    testing.assertEqual(len($base.wheres), 0); # original untouched
    testing.assertEqual(len($withWhere.wheres), 1);
}

# ---- statement builders (private, pure) ----

func aRecord() {
    def r as map of string to string init {};
    $r["name"] = "ada";
    $r["age"] = "36";
    return $r;
}

func testBuildInsertOmitsAbsentColumns() {
    # The record has no "id" (auto-generated), so it is left out.
    def r as Rendered init buildInsert(usersSchema(Dialect.Postgres), aRecord());
    testing.assertEqual($r.sql, "INSERT INTO users (name, age) VALUES ($1, $2)");
    testing.assertEqual(len($r.params), 2);
    testing.assertEqual($r.params[0], "ada");
}

func testBuildInsertMysql() {
    testing.assertEqual(
        buildInsert(usersSchema(Dialect.Mysql), aRecord()).sql,
        "INSERT INTO users (name, age) VALUES (?, ?)");
}

func testBuildUpdate() {
    def rec as map of string to string init aRecord();
    $rec["id"] = "7";
    def r as Rendered init buildUpdate(usersSchema(Dialect.Postgres), $rec);
    # Non-key columns SET, matched by the key last.
    testing.assertEqual($r.sql, "UPDATE users SET name = $1, age = $2 WHERE id = $3");
    testing.assertEqual($r.params[2], "7"); # the key value binds last
}

func testBuildUpdateRequiresKey() {
    testing.assertThrows("updateNoKey", "orm");
}
func updateNoKey() {
    buildUpdate(usersSchema(Dialect.Mysql), aRecord());
} # no "id"

func testBuildByKey() {
    def find as Rendered init buildByKey("SELECT *", usersSchema(Dialect.Mysql), "3");
    testing.assertEqual($find.sql, "SELECT * FROM users WHERE id = ?");
    testing.assertEqual($find.params[0], "3");
    def del as Rendered init buildByKey("DELETE", usersSchema(Dialect.Postgres), "3");
    testing.assertEqual($del.sql, "DELETE FROM users WHERE id = $1");
}

func testCreateTableDialects() {
    testing.assertEqual(
        createTable(usersSchema(Dialect.Postgres)),
        "CREATE TABLE users (id INTEGER, name TEXT, age INTEGER, PRIMARY KEY (id))");
    def s as Schema init column(schema("blobs", "id", Dialect.Mysql), "data", ColumnKind.Bytes);
    testing.assertContains(createTable($s), "data BLOB");
}

# OM-002: identifiers and operators reach the SQL text unparameterized, so they
# are validated at build time. Helpers for assertThrows (called by name).
func injectOp() {
    where(from(usersSchema(Dialect.Postgres)), "id", "= 1 OR 1=1 --", "7");
}
func injectCol() {
    where(from(usersSchema(Dialect.Postgres)), "id; DROP TABLE x", "=", "7");
}
func injectOrder() {
    orderBy(from(usersSchema(Dialect.Postgres)), "1; DROP TABLE users", "asc");
}
func injectTable() {
    schema("t; DROP TABLE x", "id", Dialect.Mysql);
}
func injectJoin() {
    join(from(usersSchema(Dialect.Postgres)), "other x", "a.b", "c.d");
}

func testIdentifierAndOperatorInjectionBlocked() {
    testing.assertThrows("injectOp", "orm");
    testing.assertThrows("injectCol", "orm");
    testing.assertThrows("injectOrder", "orm");
    testing.assertThrows("injectTable", "orm");
    testing.assertThrows("injectJoin", "orm");
    # a qualified `table.col` identifier and an allowlisted operator still render
    def q as Query init where(from(usersSchema(Dialect.Postgres)), "users.name", "like", "%a%");
    testing.assertContains(toSql($q).sql, "users.name LIKE");
}
