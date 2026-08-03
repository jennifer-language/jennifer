# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

# orm_demo.j - the orm module's query builder and SQL generation, which are pure
# (no database). The CRUD calls need a live connection, so `runCrud` is defined
# for you to run against your own database. Run it:
#
#     jennifer run examples/modules/orm_demo.j

use io;
import "../../modules/orm.j" as orm;

# Declare the table mapping once (no reflection). The dialect - Postgres here -
# selects the placeholder and DDL spelling.
def users as orm.Schema init orm.column(
    orm.column(orm.column(orm.schema("users", "id", orm.Dialect.Postgres), "id", orm.ColumnKind.Int), "name", orm.ColumnKind.String),
    "age",
    orm.ColumnKind.Int);

io.printf("DDL:\n  %s\n\n", orm.createTable($users));

# Column attributes drive the DDL. Set them with the fluent setters, which
# decorate the most-recently-added column: an auto-incrementing key, a NOT NULL
# UNIQUE email, and a defaulted bool.
def accounts as orm.Schema init orm.schema("accounts", "id", orm.Dialect.Postgres);
$accounts = orm.autoIncrement(orm.column($accounts, "id", orm.ColumnKind.Int));
$accounts = orm.notNull(orm.unique(orm.column($accounts, "email", orm.ColumnKind.String)));
$accounts = orm.withDefault(orm.column($accounts, "active", orm.ColumnKind.Bool), "true");
io.printf("DDL (attributes):\n  %s\n\n", orm.createTable($accounts));

# A functional query: composed with fresh handles, never mutated.
def q as orm.Query init orm.limit(
    orm.orderBy(
        orm.where(orm.where(orm.from($users), "age", ">=", "18"), "name", "LIKE", "a%"),
        "age",
        "desc"),
    25);
def rendered as orm.Rendered init orm.toSql($q);
io.printf(
    "query:\n  %s\n  params = %d (values bind through placeholders)\n\n",
    $rendered.sql,
    len($rendered.params));

# The same schema in MySQL renders `?` placeholders instead of `$1` / `$2`.
def mysqlUsers as orm.Schema init orm.column(orm.schema("users", "id", orm.Dialect.Mysql), "name", orm.ColumnKind.String);
io.printf("mysql:\n  %s\n\n", orm.toSql(orm.where(orm.from($mysqlUsers), "name", "=", "ada")).sql);

# Column projection + aggregate + GROUP BY + HAVING: "age brackets with 5+ users".
def report as orm.Query init orm.having(
    orm.groupBy(orm.count(orm.select(orm.from($users), ["age"]), "n"), ["age"]),
    "COUNT", "*", ">", "5");
io.printf("report:\n  %s\n\n", orm.toSql($report).sql);

# OR + IN conditions, and a LEFT JOIN.
def filtered as orm.Query init orm.whereIn(
    orm.orWhere(orm.where(orm.from($users), "age", ">=", "18"), "name", "=", "ada"),
    "id",
    ["1", "2", "3"]);
io.printf("or + in:\n  %s\n\n", orm.toSql($filtered).sql);
io.printf("left join:\n  %s\n", orm.toSql(
    orm.leftJoin(orm.from($users), "orders", "users.id", "orders.userId")).sql);

# Relations: declare an association once on the schema, then `joinRelation` emits
# the correct JOIN from it (two joins for a many-to-many, through its join table).
def authors as orm.Schema init orm.manyToMany(
    orm.hasMany(orm.schema("authors", "id", orm.Dialect.Postgres), "posts", "posts", "authorId"),
    "tags", "tags", "authorTags", "authorId", "tagId");
io.printf("\nhas-many join:\n  %s\n", orm.toSql(
    orm.joinRelation(orm.from($authors), $authors, "posts")).sql);
io.printf("many-to-many join:\n  %s\n", orm.toSql(
    orm.joinRelation(orm.from($authors), $authors, "tags")).sql);

# runCrud shows the Data-Mapper CRUD shape against a live connection: you pass a
# record (a `map of string to string`) and the schema to the repository
# functions through an `orm.Session`. Value-semantic records - no `save()` on the
# row itself.
func runCrud(conn as sql.Connection, s as orm.Schema) {
    def sess as orm.Session init orm.session($conn); # auto-committing session
    def ada as map of string to string init {};
    $ada["id"] = "1";
    $ada["name"] = "ada";
    $ada["age"] = "36";
    orm.insert($sess, $s, $ada); # INSERT

    def found as map of string to string init orm.find($sess, $s, "1"); # SELECT by id
    io.printf("found: %s\n", $found["name"]);

    $found["age"] = "37";
    orm.update($sess, $s, $found); # UPDATE by primary key

    def adults as list of map of string to string init orm.all(
        $sess,
        orm.where(orm.from($s), "age", ">=", "18")); # SELECT with a filter
    io.printf("adults: %d\n", len($adults));

    # A second write inside a transaction: begin, wrap the Tx in orm.transaction,
    # commit (errdefer rolls back on any failure before the commit).
    def tx as sql.Tx init sql.begin($conn);
    errdefer sql.rollback($tx);
    def txn as orm.Session init orm.transaction($tx);
    def bob as map of string to string init {};
    $bob["id"] = "2";
    $bob["name"] = "bob";
    $bob["age"] = "41";
    orm.insert($txn, $s, $bob);
    sql.commit($tx);

    orm.delete($sess, $s, "1"); # DELETE by id
}

# runEager shows eager loading: declare a relation, mark it with `with`, and
# `load` fetches the parents and their children in a fixed 1 + R queries (here 2:
# authors, then posts WHERE authorId IN (...)) - never one query per author.
func runEager(conn as sql.Connection) {
    def sess as orm.Session init orm.session($conn);
    def authorSchema as orm.Schema init orm.hasMany(
        orm.column(orm.schema("authors", "id", orm.Dialect.Postgres), "id", orm.ColumnKind.Int),
        "posts", "posts", "authorId");
    def res as orm.Result init orm.load($sess, $authorSchema,
        orm.with(orm.from($authorSchema), "posts"));
    for (def author in orm.rows($res)) {
        def posts as list of map of string to string init orm.related($res, $author, "posts");
        io.printf("%s has %d posts\n", $author["name"], len($posts));
    }
}

# runWrites shows the write path: batch insert, upsert on a unique column, bulk
# conditional update / delete, and insertReturning for a generated key.
func runWrites(conn as sql.Connection) {
    def sess as orm.Session init orm.session($conn);
    def s as orm.Schema init orm.schema("items", "id", orm.Dialect.Postgres);
    $s = orm.autoIncrement(orm.column($s, "id", orm.ColumnKind.Int));
    $s = orm.unique(orm.notNull(orm.column($s, "sku", orm.ColumnKind.String)));
    $s = orm.column($s, "qty", orm.ColumnKind.Int);

    def a as map of string to string init {};
    $a["sku"] = "A";
    $a["qty"] = "1";
    def b as map of string to string init {};
    $b["sku"] = "B";
    $b["qty"] = "9";
    orm.insertMany($sess, $s, [$a, $b]); # one multi-row INSERT

    def a2 as map of string to string init {};
    $a2["sku"] = "A";
    $a2["qty"] = "10";
    orm.upsert($sess, $s, $a2, ["sku"]); # A already exists -> updated in place

    def zero as map of string to string init {};
    $zero["qty"] = "0";
    orm.updateWhere($sess, $s, $zero, orm.where(orm.from($s), "qty", ">", "5"));
    orm.deleteWhere($sess, $s, orm.where(orm.from($s), "qty", "=", "0"));

    def c as map of string to string init {};
    $c["sku"] = "C";
    $c["qty"] = "3";
    io.printf("new item id: %s\n", orm.insertReturning($sess, $s, $c));
}

use sql;

io.printf("\n(runCrud / runEager / runWrites are defined; call them with a live sql.Connection)\n");
io.printf("(schema migrations live in the sqlmigrate module - see sqlmigrate_demo.j)\n");
