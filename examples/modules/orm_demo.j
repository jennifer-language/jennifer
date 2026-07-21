# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

# orm_demo.j - the orm module's query builder and SQL generation, which are pure
# (no database). The CRUD calls need a live connection, so `runCrud` is defined
# for you to run against your own database. Run it:
#
#     jennifer run examples/modules/orm_demo.j

use io;
import "../../modules/orm.j" as orm;

# Declare the table mapping once (no reflection). The dialect - "postgres" here -
# selects the placeholder and DDL spelling.
def users as orm.Schema init orm.column(orm.column(orm.column(
    orm.schema("users", "id", "postgres"), "id", "int"), "name", "string"), "age", "int");

io.printf("DDL:\n  %s\n\n", orm.createTable($users));

# A functional query: composed with fresh handles, never mutated.
def q as orm.Query init orm.limit(
    orm.orderBy(
        orm.where(orm.where(orm.from($users), "age", ">=", "18"), "name", "LIKE", "a%"),
        "age", "desc"),
    25);
def rendered as orm.Rendered init orm.toSql($q);
io.printf("query:\n  %s\n  params = %d (values bind through placeholders)\n\n",
    $rendered.sql, len($rendered.params));

# The same schema in MySQL renders `?` placeholders instead of `$1` / `$2`.
def mysqlUsers as orm.Schema init orm.column(
    orm.schema("users", "id", "mysql"), "name", "string");
io.printf("mysql:\n  %s\n", orm.toSql(orm.where(orm.from($mysqlUsers), "name", "=", "ada")).sql);

# runCrud shows the Data-Mapper CRUD shape against a live connection: you pass a
# record (a `map of string to string`) and the schema to the repository
# functions. Value-semantic records - no `save()` on the row itself.
func runCrud(conn as sql.Connection, s as orm.Schema) {
    def ada as map of string to string init {};
    $ada["id"] = "1";
    $ada["name"] = "ada";
    $ada["age"] = "36";
    orm.insert($conn, $s, $ada);                          # INSERT

    def found as map of string to string init orm.find($conn, $s, "1");   # SELECT by id
    io.printf("found: %s\n", $found["name"]);

    $found["age"] = "37";
    orm.update($conn, $s, $found);                        # UPDATE by primary key

    def adults as list of map of string to string init orm.all($conn,
        orm.where(orm.from($s), "age", ">=", "18"));      # SELECT with a filter
    io.printf("adults: %d\n", len($adults));

    orm.delete($conn, $s, "1");                           # DELETE by id
}

use sql;

io.printf("\n(runCrud is defined; call it with a live sql.Connection)\n");
