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
func usersSchema(dialect as string) {
    return column(column(column(
        schema("users", "id", $dialect), "id", "int"), "name", "string"), "age", "int");
}

func testSchemaAndColumns() {
    def s as Schema init usersSchema("postgres");
    testing.assertEqual($s.table, "users");
    testing.assertEqual($s.primaryKey, "id");
    testing.assertEqual(len($s.columns), 3);
    testing.assertEqual($s.columns[1].name, "name");
    testing.assertEqual($s.columns[1].kind, "string");
}

func testDialectRejected() {
    testing.assertThrows("badDialect", "orm");
}
func badDialect() { schema("t", "id", "sqlite"); }

func testSelectAll() {
    testing.assertEqual(toSql(from(usersSchema("mysql"))).sql, "SELECT * FROM users");
}

func testWherePostgresPlaceholders() {
    def q as Query init where(where(from(usersSchema("postgres")), "age", ">", "18"), "name", "=", "ada");
    def r as Rendered init toSql($q);
    testing.assertEqual($r.sql, "SELECT * FROM users WHERE age > $1 AND name = $2");
    testing.assertEqual(len($r.params), 2);
    testing.assertEqual($r.params[0], "18");
    testing.assertEqual($r.params[1], "ada");
}

func testWhereMysqlPlaceholders() {
    def q as Query init where(where(from(usersSchema("mysql")), "age", ">", "18"), "name", "=", "ada");
    testing.assertEqual(toSql($q).sql, "SELECT * FROM users WHERE age > ? AND name = ?");
}

func testOrderLimitOffset() {
    def q as Query init offset(limit(orderBy(orderBy(from(usersSchema("postgres")),
        "age", "desc"), "name", "asc"), 10), 20);
    testing.assertEqual(toSql($q).sql,
        "SELECT * FROM users ORDER BY age DESC, name ASC LIMIT 10 OFFSET 20");
}

func testJoin() {
    def q as Query init join(from(usersSchema("mysql")), "orders", "users.id", "orders.userId");
    testing.assertEqual(toSql($q).sql,
        "SELECT * FROM users INNER JOIN orders ON users.id = orders.userId");
}

func testWhereAfterJoinNumbersPlaceholders() {
    # A join has no params, so the first WHERE placeholder is still $1.
    def q as Query init where(join(from(usersSchema("postgres")), "orders", "users.id", "orders.userId"),
        "age", ">", "21");
    testing.assertEqual(toSql($q).sql,
        "SELECT * FROM users INNER JOIN orders ON users.id = orders.userId WHERE age > $1");
}

func testBuilderIsNonMutating() {
    def base as Query init from(usersSchema("mysql"));
    def withWhere as Query init where($base, "id", "=", "1");
    testing.assertEqual(len($base.wheres), 0);        # original untouched
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
    def r as Rendered init buildInsert(usersSchema("postgres"), aRecord());
    testing.assertEqual($r.sql, "INSERT INTO users (name, age) VALUES ($1, $2)");
    testing.assertEqual(len($r.params), 2);
    testing.assertEqual($r.params[0], "ada");
}

func testBuildInsertMysql() {
    testing.assertEqual(buildInsert(usersSchema("mysql"), aRecord()).sql,
        "INSERT INTO users (name, age) VALUES (?, ?)");
}

func testBuildUpdate() {
    def rec as map of string to string init aRecord();
    $rec["id"] = "7";
    def r as Rendered init buildUpdate(usersSchema("postgres"), $rec);
    # Non-key columns SET, matched by the key last.
    testing.assertEqual($r.sql, "UPDATE users SET name = $1, age = $2 WHERE id = $3");
    testing.assertEqual($r.params[2], "7");           # the key value binds last
}

func testBuildUpdateRequiresKey() {
    testing.assertThrows("updateNoKey", "orm");
}
func updateNoKey() { buildUpdate(usersSchema("mysql"), aRecord()); }   # no "id"

func testBuildByKey() {
    def find as Rendered init buildByKey("SELECT *", usersSchema("mysql"), "3");
    testing.assertEqual($find.sql, "SELECT * FROM users WHERE id = ?");
    testing.assertEqual($find.params[0], "3");
    def del as Rendered init buildByKey("DELETE", usersSchema("postgres"), "3");
    testing.assertEqual($del.sql, "DELETE FROM users WHERE id = $1");
}

func testCreateTableDialects() {
    testing.assertEqual(createTable(usersSchema("postgres")),
        "CREATE TABLE users (id INTEGER, name TEXT, age INTEGER, PRIMARY KEY (id))");
    def s as Schema init column(schema("blobs", "id", "mysql"), "data", "bytes");
    testing.assertContains(createTable($s), "data BLOB");
}
