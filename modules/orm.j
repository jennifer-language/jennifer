# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

/**
 * A minimal relational mapper over the [`sql`](../libraries/sql.md) library. It
 * is **Data Mapper, not Active Record** - Jennifer structs are value-semantic
 * and carry no methods, and a module holds no state, so a row cannot `save()`
 * itself. Instead you pass a record and a `Schema` to repository functions:
 * `orm.insert` / `find` / `update` / `delete`, and `orm.all` over a query.
 *
 * There is no reflection, so you declare the table mapping once as an
 * `orm.Schema` (built with `orm.schema` + `orm.column`), which also carries the
 * SQL **dialect** (`"mysql"` or `"postgres"`) - a backend selector on one module,
 * not parallel modules. The query builder is functional (like the `json` write
 * surface): `orm.where(orm.from($schema), "age", ">", "18")` returns a fresh
 * `orm.Query`, rendered to **parameterized** SQL by `orm.toSql`. Values bind
 * only through placeholders (injection safety inherited from `sql`).
 *
 * A record - both the input to `insert` / `update` and the result of `find` /
 * `all` - is a `map of string to string` keyed by column name (the row form that
 * needs no map-to-struct conversion; a typed-struct form waits on that language
 * feature). The database coerces the string values to the column types.
 *
 * Needs `sql`, so the default `jennifer` binary.
 * @module orm
 * @example
 * import "orm.j" as orm;
 * def s as orm.Schema init orm.column(orm.column(
 *     orm.schema("users", "id", "postgres"), "id", "int"), "name", "string");
 * def q as orm.Query init orm.limit(orm.where(orm.from($s), "name", "=", "ada"), 10);
 * def sql as orm.Rendered init orm.toSql($q);       # SELECT * FROM users WHERE name = $1 LIMIT 10
 */
use sql;
use strings;
use convert;
use maps;

# ---- schema ----

/**
 * One column in a schema: its name and value kind (informational for the
 * string-row form; the guide for a future typed form).
 * @field name {string} the column name
 * @field kind {string} the value kind (`int` / `string` / `float` / `bool` / `bytes`)
 */
export def struct Column {
    name as string,
    kind as string
};

/**
 * A table mapping: the table name, its columns, the primary-key column, and the
 * SQL dialect. Value-semantic; `column` returns a fresh schema.
 * @field table {string} the table name
 * @field columns {list of Column} the columns
 * @field primaryKey {string} the primary-key column name
 * @field dialect {string} `"mysql"` or `"postgres"` (placeholder + DDL spelling)
 */
export def struct Schema {
    table as string,
    columns as list of Column,
    primaryKey as string,
    dialect as string
};

/**
 * A rendered, parameterized statement: the SQL text and the ordered bind values.
 * @field sql {string} the SQL with dialect placeholders
 * @field params {list of string} the bind values, in placeholder order
 */
export def struct Rendered {
    sql as string,
    params as list of string
};

/**
 * A single WHERE condition within a `Query`; its bound value lives in the
 * query's `params` list (positionally). Built by `orm.where`, not directly.
 * @field column {string} the column
 * @field op {string} the comparison operator
 */
export def struct Condition {
    column as string,
    op as string
};

/**
 * A composable, non-mutating SELECT query. Build it with `from` / `where` /
 * `orderBy` / `limit` / `offset` / `join`, then render with `toSql`.
 * @field table {string} the base table
 * @field dialect {string} the SQL dialect
 * @field wheres {list of Condition} the WHERE conditions (AND-joined)
 * @field params {list of string} the bind values for the conditions
 * @field joins {list of string} rendered JOIN clauses
 * @field orders {list of string} rendered ORDER BY terms
 * @field hasLimit {bool} whether a LIMIT is set
 * @field limitN {int} the LIMIT value
 * @field hasOffset {bool} whether an OFFSET is set
 * @field offsetN {int} the OFFSET value
 */
export def struct Query {
    table as string,
    dialect as string,
    wheres as list of Condition,
    params as list of string,
    joins as list of string,
    orders as list of string,
    hasLimit as bool,
    limitN as int,
    hasOffset as bool,
    offsetN as int
};

func checkDialect(dialect as string) {
    if ($dialect != "mysql" and $dialect != "postgres") {
        throw Error{kind: "orm", message: "orm: dialect must be \"mysql\" or \"postgres\", got " + $dialect, file: "", line: 0, col: 0};
    }
}

/**
 * Start a schema for a table with its primary-key column and dialect. Add
 * columns with `orm.column`.
 * @param table {string} the table name
 * @param primaryKey {string} the primary-key column
 * @param dialect {string} `"mysql"` or `"postgres"`
 * @return {Schema} the schema (no columns yet)
 * @throws {Error} on an unknown dialect
 */
export func schema(table as string, primaryKey as string, dialect as string) {
    checkDialect($dialect);
    return Schema{table: $table, columns: [], primaryKey: $primaryKey, dialect: $dialect};
}

/**
 * A copy of `s` with a column appended.
 * @param s {Schema} the source schema
 * @param name {string} the column name
 * @param kind {string} the value kind (`int` / `string` / `float` / `bool` / `bytes`)
 * @return {Schema} the extended schema
 */
export func column(s as Schema, name as string, kind as string) {
    def out as Schema init $s;
    def cols as list of Column init $out.columns;
    $cols[] = Column{name: $name, kind: $kind};
    $out.columns = $cols;
    return $out;
}

# ---- placeholder / dialect helpers ----

# ph renders the n-th placeholder for the dialect: `?` (mysql) or `$n` (postgres).
func ph(dialect as string, n as int) {
    if ($dialect == "postgres") {
        return "$" + convert.toString($n);
    }
    return "?";
}

# sqlType maps a column kind to a dialect SQL type for createTable.
func sqlType(kind as string, dialect as string) {
    if ($kind == "int") {
        return "INTEGER";
    }
    if ($kind == "float") {
        if ($dialect == "postgres") {
            return "DOUBLE PRECISION";
        }
        return "DOUBLE";
    }
    if ($kind == "bool") {
        return "BOOLEAN";
    }
    if ($kind == "bytes") {
        if ($dialect == "postgres") {
            return "BYTEA";
        }
        return "BLOB";
    }
    return "TEXT";
}

# ---- query builder (pure) ----

/**
 * A base query selecting all rows of the schema's table.
 * @param s {Schema} the schema
 * @return {Query} the base query
 */
export func from(s as Schema) {
    return Query{table: $s.table, dialect: $s.dialect, wheres: [], params: [],
        joins: [], orders: [], hasLimit: false, limitN: 0, hasOffset: false, offsetN: 0};
}

/**
 * A copy of `q` with a `column op value` condition added (AND-joined). The value
 * binds as a parameter.
 * @param q {Query} the source query
 * @param col {string} the column
 * @param op {string} the operator (`=`, `>`, `<`, `>=`, `<=`, `!=`, `LIKE`, ...)
 * @param value {string} the value to bind
 * @return {Query} the extended query
 */
export func where(q as Query, col as string, op as string, value as string) {
    def out as Query init $q;
    def ws as list of Condition init $out.wheres;
    $ws[] = Condition{column: $col, op: $op};
    $out.wheres = $ws;
    def ps as list of string init $out.params;
    $ps[] = $value;
    $out.params = $ps;
    return $out;
}

/**
 * A copy of `q` with an ORDER BY term added.
 * @param q {Query} the source query
 * @param col {string} the column
 * @param dir {string} `"asc"` or `"desc"`
 * @return {Query} the extended query
 */
export func orderBy(q as Query, col as string, dir as string) {
    def out as Query init $q;
    def os as list of string init $out.orders;
    def word as string init "ASC";
    if (strings.lower($dir) == "desc") {
        $word = "DESC";
    }
    $os[] = $col + " " + $word;
    $out.orders = $os;
    return $out;
}

/**
 * A copy of `q` with a LIMIT.
 * @param q {Query} the source query
 * @param n {int} the row limit
 * @return {Query} the extended query
 */
export func limit(q as Query, n as int) {
    def out as Query init $q;
    $out.hasLimit = true;
    $out.limitN = $n;
    return $out;
}

/**
 * A copy of `q` with an OFFSET.
 * @param q {Query} the source query
 * @param n {int} the row offset
 * @return {Query} the extended query
 */
export func offset(q as Query, n as int) {
    def out as Query init $q;
    $out.hasOffset = true;
    $out.offsetN = $n;
    return $out;
}

/**
 * A copy of `q` with an INNER JOIN.
 * @param q {Query} the source query
 * @param table {string} the table to join
 * @param leftCol {string} the left join column (`table.col`)
 * @param rightCol {string} the right join column
 * @return {Query} the extended query
 */
export func join(q as Query, table as string, leftCol as string, rightCol as string) {
    def out as Query init $q;
    def js as list of string init $out.joins;
    $js[] = "INNER JOIN " + $table + " ON " + $leftCol + " = " + $rightCol;
    $out.joins = $js;
    return $out;
}

/**
 * Render a query to parameterized SQL for its dialect. Pure - the whole
 * query-builder surface is testable without a database.
 * @param q {Query} the query
 * @return {Rendered} the SQL text and ordered bind values
 */
export func toSql(q as Query) {
    def stmt as string init "SELECT * FROM " + $q.table;
    for (def i as int init 0; $i < len($q.joins); $i = $i + 1) {
        $stmt = $stmt + " " + $q.joins[$i];
    }
    def n as int init 1;
    if (len($q.wheres) > 0) {
        $stmt = $stmt + " WHERE ";
        for (def i as int init 0; $i < len($q.wheres); $i = $i + 1) {
            if ($i > 0) {
                $stmt = $stmt + " AND ";
            }
            $stmt = $stmt + $q.wheres[$i].column + " " + $q.wheres[$i].op + " " + ph($q.dialect, $n);
            $n = $n + 1;
        }
    }
    if (len($q.orders) > 0) {
        $stmt = $stmt + " ORDER BY " + strings.join($q.orders, ", ");
    }
    if ($q.hasLimit) {
        $stmt = $stmt + " LIMIT " + convert.toString($q.limitN);
    }
    if ($q.hasOffset) {
        $stmt = $stmt + " OFFSET " + convert.toString($q.offsetN);
    }
    return Rendered{sql: $stmt, params: $q.params};
}

/**
 * The CREATE TABLE DDL for a schema, in its dialect. A convenience DDL emitter -
 * a schema-migration tool is out of scope.
 * @param s {Schema} the schema
 * @return {string} the CREATE TABLE statement
 */
export func createTable(s as Schema) {
    def cols as string init "";
    for (def i as int init 0; $i < len($s.columns); $i = $i + 1) {
        if ($i > 0) {
            $cols = $cols + ", ";
        }
        $cols = $cols + $s.columns[$i].name + " " + sqlType($s.columns[$i].kind, $s.dialect);
    }
    return "CREATE TABLE " + $s.table + " (" + $cols + ", PRIMARY KEY (" + $s.primaryKey + "))";
}

# ---- statement builders (pure, for CRUD) ----

# buildInsert renders an INSERT for the columns present in the record.
func buildInsert(s as Schema, record as map of string to string) {
    def cols as string init "";
    def phs as string init "";
    def params as list of string init [];
    def n as int init 1;
    for (def i as int init 0; $i < len($s.columns); $i = $i + 1) {
        def name as string init $s.columns[$i].name;
        if (maps.has($record, $name)) {
            if (len($params) > 0) {
                $cols = $cols + ", ";
                $phs = $phs + ", ";
            }
            $cols = $cols + $name;
            $phs = $phs + ph($s.dialect, $n);
            $n = $n + 1;
            $params[] = $record[$name];
        }
    }
    return Rendered{sql: "INSERT INTO " + $s.table + " (" + $cols + ") VALUES (" + $phs + ")", params: $params};
}

# buildUpdate renders an UPDATE of every non-key present column, matched by key.
func buildUpdate(s as Schema, record as map of string to string) {
    if (not maps.has($record, $s.primaryKey)) {
        throw Error{kind: "orm", message: "orm.update: record has no primary key (" + $s.primaryKey + ")", file: "", line: 0, col: 0};
    }
    def sets as string init "";
    def params as list of string init [];
    def n as int init 1;
    for (def i as int init 0; $i < len($s.columns); $i = $i + 1) {
        def name as string init $s.columns[$i].name;
        if ($name != $s.primaryKey and maps.has($record, $name)) {
            if (len($params) > 0) {
                $sets = $sets + ", ";
            }
            $sets = $sets + $name + " = " + ph($s.dialect, $n);
            $n = $n + 1;
            $params[] = $record[$name];
        }
    }
    def stmt as string init "UPDATE " + $s.table + " SET " + $sets + " WHERE " + $s.primaryKey + " = " + ph($s.dialect, $n);
    $params[] = $record[$s.primaryKey];
    return Rendered{sql: $stmt, params: $params};
}

# buildByKey renders a `verb ... WHERE pk = ph` for find (SELECT) / delete.
func buildByKey(verb as string, s as Schema, id as string) {
    def params as list of string init [];
    $params[] = $id;
    def stmt as string init $verb + " FROM " + $s.table + " WHERE " + $s.primaryKey + " = " + ph($s.dialect, 1);
    return Rendered{sql: $stmt, params: $params};
}

# ---- row mapping ----

# mapRow reads the current cursor row into a map of column name to string value
# (a NULL column becomes the empty string).
func mapRow(rows as sql.Rows) {
    def out as map of string to string init {};
    def cols as list of string init sql.columns($rows);
    for (def i as int init 0; $i < len($cols); $i = $i + 1) {
        if (sql.isNull($rows, $cols[$i])) {
            $out[$cols[$i]] = "";
        } else {
            $out[$cols[$i]] = sql.asString($rows, $cols[$i]);
        }
    }
    return $out;
}

# ---- CRUD ----

/**
 * Insert a record. Only the columns present in `record` are written (so an
 * auto-generated primary key is simply omitted).
 * @param conn {sql.Connection} the connection (or a `sql.Tx`)
 * @param s {Schema} the schema
 * @param record {map of string to string} column name -> value
 * @return {sql.Result} the affected-rows / last-insert-id result
 */
export func insert(conn as sql.Connection, s as Schema, record as map of string to string) {
    def r as Rendered init buildInsert($s, $record);
    return sql.exec($conn, $r.sql, $r.params);
}

/**
 * Find a single row by primary-key value.
 * @param conn {sql.Connection} the connection (or a `sql.Tx`)
 * @param s {Schema} the schema
 * @param id {string} the primary-key value
 * @return {map of string to string} the row (column name -> value)
 * @throws {Error} when no row has that key
 */
export func find(conn as sql.Connection, s as Schema, id as string) {
    def r as Rendered init buildByKey("SELECT *", $s, $id);
    def rows as sql.Rows init sql.query($conn, $r.sql, $r.params);
    if (not sql.next($rows)) {
        throw Error{kind: "orm", message: "orm.find: no " + $s.table + " with " + $s.primaryKey + " = " + $id, file: "", line: 0, col: 0};
    }
    def row as map of string to string init mapRow($rows);
    sql.closeRows($rows);
    return $row;
}

/**
 * Update a record, matched by its primary-key value (which the record must
 * carry). Every other present column is written.
 * @param conn {sql.Connection} the connection (or a `sql.Tx`)
 * @param s {Schema} the schema
 * @param record {map of string to string} the row, including the primary key
 * @return {sql.Result} the affected-rows result
 * @throws {Error} when the record has no primary-key value
 */
export func update(conn as sql.Connection, s as Schema, record as map of string to string) {
    def r as Rendered init buildUpdate($s, $record);
    return sql.exec($conn, $r.sql, $r.params);
}

/**
 * Delete a row by primary-key value.
 * @param conn {sql.Connection} the connection (or a `sql.Tx`)
 * @param s {Schema} the schema
 * @param id {string} the primary-key value
 * @return {sql.Result} the affected-rows result
 */
export func delete(conn as sql.Connection, s as Schema, id as string) {
    def r as Rendered init buildByKey("DELETE", $s, $id);
    return sql.exec($conn, $r.sql, $r.params);
}

/**
 * Run a query and return every matching row.
 * @param conn {sql.Connection} the connection (or a `sql.Tx`)
 * @param q {Query} the query (from the builder)
 * @return {list of map of string to string} the rows
 */
export func all(conn as sql.Connection, q as Query) {
    def r as Rendered init toSql($q);
    def rows as sql.Rows init sql.query($conn, $r.sql, $r.params);
    def out as list of map of string to string init [];
    repeat {
        if (not sql.next($rows)) {
            break;
        }
        $out[] = mapRow($rows);
    } until (false);
    return $out;
}
