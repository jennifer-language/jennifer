# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
# pragma-jennifer-version: >=0.24.0
# pragma-jennifer-capability: sql

/**
 * A minimal relational mapper over the [`sql`](../libraries/sql.md) library. It
 * is **Data Mapper, not Active Record** - Jennifer structs are value-semantic
 * and carry no methods, and a module holds no state, so a row cannot `save()`
 * itself. Instead you pass a record and a `Schema` to repository functions:
 * `orm.insert` / `find` / `update` / `delete`, and `orm.all` over a query.
 *
 * There is no reflection, so you declare the table mapping once as an
 * `orm.Schema` (built with `orm.schema` + `orm.column`), which also carries the
 * SQL **dialect** (`orm.Dialect.Mysql` or `orm.Dialect.Postgres`) - a backend
 * selector on one module, not parallel modules. The query builder is functional
 * (like the `json` write surface): each step returns a fresh `orm.Query`,
 * rendered to **parameterized** SQL by `orm.toSql`. The surface covers ordinary
 * queries: column projection (`select`) and aggregates (`count` / `aggregate`),
 * `where` / `orWhere` / `whereIn` (AND / OR / IN conditions), `join` / `leftJoin`
 * / `rightJoin`, `groupBy` + `having`, `orderBy` / `limit` / `offset`.
 *
 * Values bind only through placeholders (injection safety inherited from `sql`);
 * the tokens that cannot be parameterized - table / column identifiers,
 * comparison operators, aggregate functions, join kinds, sort directions - are
 * validated against fixed allowlists (identifiers must be bare SQL names). They
 * are checked both at **build** time (for an early, friendly error) and again at
 * **render** time inside `toSql` / `createTable` / the CRUD builders, so even a
 * hand-built `orm.Query` / `orm.Schema` struct literal that skipped the builder
 * cannot inject SQL.
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
 *     orm.schema("users", "id", orm.Dialect.Postgres), "id", orm.ColumnKind.Int), "name", orm.ColumnKind.String);
 * def q as orm.Query init orm.limit(orm.where(orm.from($s), "name", "=", "ada"), 10);
 * def sql as orm.Rendered init orm.toSql($q);       # SELECT * FROM users WHERE name = $1 LIMIT 10
 */
use sql;
use strings;
use convert;
use maps;

# The per-statement placeholder ceiling. PostgreSQL caps a statement at 65535
# parameters; 60000 stays safely under it for both dialects. `insertMany` splits a
# larger multi-row INSERT across statements, and eager loading splits a larger
# `WHERE fk IN (...)` batch the same way - so neither is ever silently capped.
def const MAX_QUERY_PARAMS as int init 60000;

# MAX_PAGE_SIZE caps a page's row limit so an attacker-controlled page size cannot
# turn a paged query into an unbounded full-table SELECT.
def const MAX_PAGE_SIZE as int init 10000;

# ---- schema ----

/**
 * The SQL dialect: the backend selector that governs placeholder syntax and DDL
 * spelling. `orm.Dialect.Mysql` or `orm.Dialect.Postgres`.
 */
export def enum Dialect { Mysql, Postgres };

/**
 * A column's value kind: the SQL type family `createTable` renders. One of
 * `orm.ColumnKind.Int` / `String` / `Float` / `Bool` / `Bytes`.
 */
export def enum ColumnKind { Int, String, Float, Bool, Bytes };

/**
 * One column in a schema: its name, value kind, and DDL attributes. The
 * attributes drive `createTable` / `addColumn` DDL; set them with the fluent
 * setters (`notNull` / `unique` / `withDefault` / `autoIncrement`), which
 * decorate the most-recently-added column. A column is **nullable by default**
 * (SQL's own default); `notNull` opts into `NOT NULL`.
 * @field name {string} the column name
 * @field kind {ColumnKind} the value kind
 * @field nullable {bool} whether the column allows NULL (true = no NOT NULL clause)
 * @field unique {bool} whether the column carries a UNIQUE constraint
 * @field hasDefault {bool} whether a DEFAULT is set (guards `default`)
 * @field default {string} the DEFAULT value (rendered by kind: numeric/bool bare, string quoted+escaped)
 * @field autoIncrement {bool} whether the column auto-increments (`SERIAL` / `AUTO_INCREMENT`)
 */
export def struct Column {
    name as string,
    kind as ColumnKind,
    nullable as bool,
    unique as bool,
    hasDefault as bool,
    default as string,
    autoIncrement as bool
};

/**
 * The kind of an association between two tables: `orm.RelationKind.BelongsTo`
 * (this table holds the foreign key), `HasOne` / `HasMany` (the target table
 * holds it), or `ManyToMany` (a join table links the two).
 */
export def enum RelationKind { BelongsTo, HasOne, HasMany, ManyToMany };

/**
 * A declared association from a schema to another table. Metadata only - built
 * by `orm.belongsTo` / `hasOne` / `hasMany` / `manyToMany`, read by
 * `orm.joinRelation` (and, later, eager loading). For a `BelongsTo`, `foreignKey`
 * is on **this** table and `localKey` is the target's key; for `HasOne` /
 * `HasMany`, `foreignKey` is on the **target** table and `localKey` is this
 * table's key. `ManyToMany` links through `through` (a join table) via its two
 * keys.
 * @field name {string} the relation's name (the lookup key for joinRelation / eager loading)
 * @field kind {RelationKind} the association kind
 * @field target {string} the target table
 * @field foreignKey {string} the foreign-key column (side depends on kind)
 * @field localKey {string} the referenced key column (the "one" side's key)
 * @field through {string} the join table for ManyToMany (else "")
 * @field throughLocalKey {string} the join-table column referencing this table (else "")
 * @field throughTargetKey {string} the join-table column referencing the target (else "")
 */
export def struct Relation {
    name as string,
    kind as RelationKind,
    target as string,
    foreignKey as string,
    localKey as string,
    through as string,
    throughLocalKey as string,
    throughTargetKey as string
};

/**
 * A table mapping: the table name, its columns, the primary-key column, the SQL
 * dialect, and any declared relations. Value-semantic; `column` returns a fresh
 * schema.
 * @field table {string} the table name
 * @field columns {list of Column} the columns
 * @field primaryKey {string} the primary-key column name
 * @field dialect {Dialect} the SQL dialect (placeholder + DDL spelling)
 * @field relations {list of Relation} the declared associations
 */
export def struct Schema {
    table as string,
    columns as list of Column,
    primaryKey as string,
    dialect as Dialect,
    relations as list of Relation
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

# ---- session (unit of work) ----

/**
 * A unit of work wrapping either a `sql.Connection` (auto-committing) or a
 * `sql.Tx` (inside a caller's transaction). Every persistence / query-executing
 * function takes a `Session` as its first argument, so the same call runs
 * standalone or inside a transaction. Value-semantic; build it with
 * `orm.session` or `orm.transaction`, not directly (only the handle selected by
 * `inTx` is ever touched).
 * @field conn {sql.Connection} the connection (used when `inTx` is false)
 * @field tx {sql.Tx} the transaction (used when `inTx` is true)
 * @field inTx {bool} which handle is live
 */
export def struct Session {
    conn as sql.Connection,
    tx as sql.Tx,
    inTx as bool
};

/**
 * A session that runs each statement directly on a connection (auto-commit).
 * @param conn {sql.Connection} the open connection
 * @return {Session} an auto-committing session
 */
export func session(conn as sql.Connection) {
    def s as Session;
    $s.conn = $conn;
    $s.inTx = false;
    return $s;
}

/**
 * A session that runs its statements inside a caller-managed transaction. The
 * caller owns `sql.begin` / `commit` / `rollback`; orm just executes through the
 * `Tx`. The idiom is `sql.begin` + `errdefer sql.rollback` + `orm.transaction` +
 * `sql.commit`.
 * @param tx {sql.Tx} the open transaction
 * @return {Session} a transaction-bound session
 */
export func transaction(tx as sql.Tx) {
    def s as Session;
    $s.tx = $tx;
    $s.inTx = true;
    return $s;
}

# sessExec / sessQuery dispatch one statement to the session's live handle. sql's
# query / exec accept a Connection or a Tx uniformly, but Jennifer's object types
# are strict (a Tx does not satisfy an `as sql.Connection` slot), so the branch
# lives here rather than in a single polymorphic call.
func sessExec(session as Session, stmt as string, params as list of string) {
    if ($session.inTx) {
        return sql.exec($session.tx, $stmt, $params);
    }
    return sql.exec($session.conn, $stmt, $params);
}

func sessQuery(session as Session, stmt as string, params as list of string) {
    if ($session.inTx) {
        return sql.query($session.tx, $stmt, $params);
    }
    return sql.query($session.conn, $stmt, $params);
}

/**
 * One item in a SELECT projection: a plain column (`func` "") or an aggregate
 * (`func` one of COUNT / SUM / AVG / MIN / MAX, rendered `func(column) AS alias`).
 * Built by `orm.select` / `orm.count` / `orm.aggregate`, not directly.
 * @field func {string} the aggregate function, or "" for a plain column
 * @field column {string} the column (or "*" for COUNT(*))
 * @field alias {string} the `AS` alias for an aggregate, or "" for a plain column
 */
export def struct SelectItem {
    func as string,
    column as string,
    alias as string
};

/**
 * One ORDER BY term. Built by `orm.orderBy`, not directly.
 * @field column {string} the column
 * @field dir {string} `"ASC"` or `"DESC"`
 */
export def struct Order {
    column as string,
    dir as string
};

/**
 * One JOIN clause: its kind and the `left = right` equality. Built by
 * `orm.join` / `orm.leftJoin` / `orm.rightJoin`, not directly.
 * @field kind {string} `"INNER"`, `"LEFT"`, or `"RIGHT"`
 * @field table {string} the joined table
 * @field leftCol {string} the left join column (`table.col`)
 * @field rightCol {string} the right join column
 */
export def struct Join {
    kind as string,
    table as string,
    leftCol as string,
    rightCol as string
};

/**
 * A single WHERE condition within a `Query`; its bound value(s) live in the
 * query's `params` list (positionally). Built by `orm.where` / `orWhere` /
 * `whereIn`, not directly.
 * @field column {string} the column
 * @field op {string} the comparison operator (or `IN` / `NOT IN`)
 * @field connector {string} `"AND"` or `"OR"` - how this joins the previous condition
 * @field valueCount {int} the number of placeholders this condition consumes (1, or N for `IN`)
 */
export def struct Condition {
    column as string,
    op as string,
    connector as string,
    valueCount as int
};

/**
 * A single HAVING condition over an aggregate, its value in `havingParams`.
 * Built by `orm.having`, not directly.
 * @field func {string} the aggregate function (COUNT / SUM / AVG / MIN / MAX)
 * @field column {string} the aggregated column (or "*" for COUNT(*))
 * @field op {string} the comparison operator
 * @field connector {string} `"AND"` or `"OR"` - how this joins the previous HAVING condition
 */
export def struct Having {
    func as string,
    column as string,
    op as string,
    connector as string
};

/**
 * A composable, non-mutating SELECT query. Build it with `from` / `select` /
 * `count` / `aggregate` / `where` / `orWhere` / `whereIn` / `join` / `leftJoin` /
 * `groupBy` / `having` / `orderBy` / `limit` / `offset`, then render with
 * `toSql`. Every identifier is re-validated at render time, so a hand-built
 * `Query` literal cannot bypass the injection guards.
 * @field table {string} the base table
 * @field dialect {Dialect} the SQL dialect
 * @field selects {list of SelectItem} the projection (empty = `SELECT *`)
 * @field wheres {list of Condition} the WHERE conditions
 * @field params {list of string} the bind values for the WHERE conditions
 * @field joins {list of Join} the JOIN clauses
 * @field groups {list of string} the GROUP BY columns
 * @field havings {list of Having} the HAVING conditions
 * @field havingParams {list of string} the bind values for the HAVING conditions
 * @field orders {list of Order} the ORDER BY terms
 * @field hasLimit {bool} whether a LIMIT is set
 * @field limitN {int} the LIMIT value
 * @field hasOffset {bool} whether an OFFSET is set
 * @field offsetN {int} the OFFSET value
 * @field withRelations {list of string} relation names to eager-load (consumed by `load`, ignored by `toSql`)
 * @field distinctSelect {bool} whether to render `SELECT DISTINCT`
 */
export def struct Query {
    table as string,
    dialect as Dialect,
    selects as list of SelectItem,
    wheres as list of Condition,
    params as list of string,
    joins as list of Join,
    groups as list of string,
    havings as list of Having,
    havingParams as list of string,
    orders as list of Order,
    hasLimit as bool,
    limitN as int,
    hasOffset as bool,
    offsetN as int,
    withRelations as list of string,
    distinctSelect as bool
};

func fail(message as string) {
    throw Error{kind: "orm", message: "orm: " + $message, file: "", line: 0, col: 0};
}

# validIdentSegment reports whether s is one bare SQL identifier: a letter or `_`
# followed by letters, digits, or `_`. ASCII only (identifiers are).
func validIdentSegment(s as string) {
    def raw as bytes init convert.bytesFromString($s, "utf-8");
    if (len($raw) == 0) {
        return false;
    }
    def i as int init 0;
    while ($i < len($raw)) {
        def b as int init $raw[$i];
        def alpha as bool init ($b >= 65 and $b <= 90) or ($b >= 97 and $b <= 122) or $b == 95;
        def digit as bool init $b >= 48 and $b <= 57;
        if ($i == 0) {
            if (not $alpha) {
                return false;
            }
        } else {
            if (not ($alpha or $digit)) {
                return false;
            }
        }
        $i = $i + 1;
    }
    return true;
}

# checkIdent validates a column / table identifier, optionally qualified as
# `table.col`, and throws otherwise. Identifiers reach the SQL text
# unparameterized - only `value`s bind through placeholders - so this is the
# injection guard for every non-value token (OM-002).
func checkIdent(name as string, what as string) {
    def parts as list of string init strings.split($name, ".");
    if (len($parts) < 1 or len($parts) > 2) {
        fail($what + " must be a bare or `table.col` identifier, got: " + $name);
    }
    for (def p in $parts) {
        if (not validIdentSegment($p)) {
            fail($what + " must be a SQL identifier (letter/`_`, then letters/digits/`_`), got: " +
                $name);
        }
    }
}

# checkOp maps a comparison operator through an allowlist to its canonical form,
# throwing on anything else - the operator is interpolated raw, so it must be a
# known operator, not caller-controlled free text (OM-002). IN / IS are omitted
# because they do not fit the module's single-bound-value-per-condition shape.
func checkOp(op as string) {
    def norm as string init strings.upper(strings.trim($op));
    match ($norm) {
        when "=", "!=", "<>", "<", ">", "<=", ">=", "LIKE", "NOT LIKE" {
            return $norm;
        }
        when "IN", "NOT IN" {
            fail("use orm.whereIn for an IN condition, not a single-value where");
        }
        else {
            fail("operator not allowed: \"" + $op + "\" (use = != <> < > <= >= LIKE or \"NOT LIKE\")");
        }
    }
}

# checkRenderedOp is the render-time operator allowlist: the single-value set plus
# IN / NOT IN (`whereIn`), IS NULL / IS NOT NULL (`whereNull` / `whereNotNull`), and
# BETWEEN (`whereBetween`). Used by validateQuery so a hand-built Condition literal
# is still checked.
func checkRenderedOp(op as string) {
    def norm as string init strings.upper(strings.trim($op));
    match ($norm) {
        when "=", "!=", "<>", "<", ">", "<=", ">=", "LIKE", "NOT LIKE", "IN", "NOT IN", "IS NULL", "IS NOT NULL", "BETWEEN" {
            return $norm;
        }
        else {
            fail("operator not allowed: \"" + $op + "\"");
        }
    }
}

# checkColRef validates a column reference that may be the aggregate wildcard "*"
# (COUNT(*)) or an ordinary bare / `table.col` identifier.
func checkColRef(name as string, what as string) {
    if ($name == "*") {
        return;
    }
    checkIdent($name, $what);
}

# checkAggFunc maps an aggregate function through its allowlist to canonical form.
# The function name reaches the SQL unparameterized, so it must be a known token.
func checkAggFunc(fn as string) {
    def u as string init strings.upper(strings.trim($fn));
    match ($u) {
        when "COUNT", "SUM", "AVG", "MIN", "MAX" {
            return $u;
        }
        else {
            fail("aggregate function not allowed: \"" + $fn + "\" (use COUNT SUM AVG MIN or MAX)");
        }
    }
}

# checkDir validates and canonicalizes an ORDER BY direction.
func checkDir(dir as string) {
    def u as string init strings.upper(strings.trim($dir));
    if ($u == "ASC" or $u == "DESC") {
        return $u;
    }
    fail("order direction must be ASC or DESC, got: " + $dir);
}

# checkJoinKind validates and canonicalizes a JOIN kind.
func checkJoinKind(kind as string) {
    def u as string init strings.upper(strings.trim($kind));
    match ($u) {
        when "INNER", "LEFT", "RIGHT" {
            return $u;
        }
        else {
            fail("join kind must be INNER LEFT or RIGHT, got: " + $kind);
        }
    }
}

# validateQuery re-checks every identifier / operator a Query renders. The
# constructors already validate, but a caller can hand-build a `Query{...}` /
# `Condition{...}` literal and skip them; these tokens reach the SQL text
# unparameterized, so toSql re-validates them here as the injection backstop.
func validateQuery(q as Query) {
    checkIdent($q.table, "table");
    for (def si in $q.selects) {
        if (not ($si.func == "")) {
            checkAggFunc($si.func);
            checkColRef($si.column, "aggregate column");
            checkIdent($si.alias, "aggregate alias");
        } else {
            checkIdent($si.column, "select column");
        }
    }
    for (def j in $q.joins) {
        checkJoinKind($j.kind);
        checkIdent($j.table, "join table");
        checkIdent($j.leftCol, "join column");
        checkIdent($j.rightCol, "join column");
    }
    for (def c in $q.wheres) {
        checkIdent($c.column, "where column");
        checkRenderedOp($c.op);
    }
    for (def g in $q.groups) {
        checkIdent($g, "group-by column");
    }
    for (def h in $q.havings) {
        checkAggFunc($h.func);
        checkColRef($h.column, "having column");
        checkRenderedOp($h.op);
    }
    for (def o in $q.orders) {
        checkIdent($o.column, "order-by column");
        checkDir($o.dir);
    }
}

# validateSchema re-checks a Schema's identifiers, so a raw `Schema{...}` literal
# cannot inject through createTable / insert / update / find / delete.
func validateSchema(s as Schema) {
    checkIdent($s.table, "table");
    checkIdent($s.primaryKey, "primary-key column");
    for (def c in $s.columns) {
        checkIdent($c.name, "column");
    }
}

/**
 * Start a schema for a table with its primary-key column and dialect. Add
 * columns with `orm.column`.
 * @param table {string} the table name
 * @param primaryKey {string} the primary-key column
 * @param dialect {Dialect} `orm.Dialect.Mysql` or `orm.Dialect.Postgres`
 * @return {Schema} the schema (no columns yet)
 */
export func schema(table as string, primaryKey as string, dialect as Dialect) {
    checkIdent($table, "table name");
    checkIdent($primaryKey, "primary-key column");
    return Schema{
        table: $table,
        columns: [],
        primaryKey: $primaryKey,
        dialect: $dialect,
        relations: []
    };
}

/**
 * A copy of `s` with a column appended.
 * @param s {Schema} the source schema
 * @param name {string} the column name
 * @param kind {ColumnKind} the value kind (`orm.ColumnKind.Int` / `String` / `Float` / `Bool` / `Bytes`)
 * @return {Schema} the extended schema
 */
export func column(s as Schema, name as string, kind as ColumnKind) {
    checkIdent($name, "column name");
    def out as Schema init $s;
    def cols as list of Column init $out.columns;
    $cols[] = Column{
        name: $name,
        kind: $kind,
        nullable: true,
        unique: false,
        hasDefault: false,
        default: "",
        autoIncrement: false
    };
    $out.columns = $cols;
    return $out;
}

# lastColumnIndex returns the index of the most-recently-added column, throwing
# if the schema has none (a fluent setter was called before any `column`).
func lastColumnIndex(s as Schema) {
    if (len($s.columns) == 0) {
        fail("no column to decorate; call orm.column first");
    }
    return len($s.columns) - 1;
}

/**
 * A copy of `s` marking its most-recently-added column `NOT NULL`.
 * @param s {Schema} the source schema
 * @return {Schema} the schema with the last column made non-nullable
 */
export func notNull(s as Schema) {
    def out as Schema init $s;
    def idx as int init lastColumnIndex($out);
    def c as Column init $out.columns[$idx];
    $c.nullable = false;
    $out.columns[$idx] = $c;
    return $out;
}

/**
 * A copy of `s` adding a `UNIQUE` constraint to its most-recently-added column.
 * @param s {Schema} the source schema
 * @return {Schema} the schema with the last column made unique
 */
export func unique(s as Schema) {
    def out as Schema init $s;
    def idx as int init lastColumnIndex($out);
    def c as Column init $out.columns[$idx];
    $c.unique = true;
    $out.columns[$idx] = $c;
    return $out;
}

/**
 * A copy of `s` marking its most-recently-added column auto-incrementing
 * (`SERIAL` on Postgres, `AUTO_INCREMENT` on MySQL).
 * @param s {Schema} the source schema
 * @return {Schema} the schema with the last column auto-incrementing
 */
export func autoIncrement(s as Schema) {
    def out as Schema init $s;
    def idx as int init lastColumnIndex($out);
    def c as Column init $out.columns[$idx];
    $c.autoIncrement = true;
    $out.columns[$idx] = $c;
    return $out;
}

/**
 * A copy of `s` giving its most-recently-added column a `DEFAULT`. The value is
 * rendered by the column kind: an `Int` / `Float` default is validated numeric
 * and rendered bare; a `Bool` default accepts `true`/`false`/`1`/`0`; a `String`
 * / `Bytes` default is rendered as a quoted, escaped SQL string literal
 * (backslashes and control characters are rejected). So no default value can
 * inject DDL.
 * @param s {Schema} the source schema
 * @param value {string} the default value (interpreted per the column kind)
 * @return {Schema} the schema with the last column defaulted
 */
export func withDefault(s as Schema, value as string) {
    def out as Schema init $s;
    def idx as int init lastColumnIndex($out);
    def c as Column init $out.columns[$idx];
    $c.hasDefault = true;
    $c.default = $value;
    $out.columns[$idx] = $c;
    return $out;
}

# ---- placeholder / dialect helpers ----

# ph renders the n-th placeholder for the dialect: `?` (mysql) or `$n` (postgres).
func ph(dialect as Dialect, n as int) {
    match ($dialect) {
        when Postgres { return "$" + convert.toString($n); }
        when Mysql { return "?"; }
    }
}

# sqlType maps a column kind to a dialect SQL type for createTable. Both the kind
# and the dialect are enum parameters, so each `match` is exhaustiveness-checked:
# a new ColumnKind or Dialect must be handled here before it compiles.
func sqlType(kind as ColumnKind, dialect as Dialect) {
    match ($kind) {
        when Int { return "INTEGER"; }
        when Bool { return "BOOLEAN"; }
        when String { return "TEXT"; }
        when Float {
            match ($dialect) {
                when Postgres { return "DOUBLE PRECISION"; }
                when Mysql { return "DOUBLE"; }
            }
        }
        when Bytes {
            match ($dialect) {
                when Postgres { return "BYTEA"; }
                when Mysql { return "BLOB"; }
            }
        }
    }
}

# ---- query builder (pure) ----

/**
 * A base query selecting all rows of the schema's table.
 * @param s {Schema} the schema
 * @return {Query} the base query
 */
export func from(s as Schema) {
    return Query{
        table: $s.table,
        dialect: $s.dialect,
        selects: [],
        wheres: [],
        params: [],
        joins: [],
        groups: [],
        havings: [],
        havingParams: [],
        orders: [],
        hasLimit: false,
        limitN: 0,
        hasOffset: false,
        offsetN: 0,
        withRelations: [],
        distinctSelect: false
    };
}

# ---- projection ----

/**
 * A copy of `q` projecting only the named columns (instead of `SELECT *`).
 * Additive: call again, or combine with `count` / `aggregate`.
 * @param q {Query} the source query
 * @param cols {list of string} the columns to project
 * @return {Query} the extended query
 */
export func select(q as Query, cols as list of string) {
    def out as Query init $q;
    def ss as list of SelectItem init $out.selects;
    for (def c in $cols) {
        checkIdent($c, "select column");
        $ss[] = SelectItem{func: "", column: $c, alias: ""};
    }
    $out.selects = $ss;
    return $out;
}

/**
 * A copy of `q` adding `COUNT(*) AS alias` to the projection.
 * @param q {Query} the source query
 * @param alias {string} the result-column alias
 * @return {Query} the extended query
 */
export func count(q as Query, alias as string) {
    return aggregate($q, "COUNT", "*", $alias);
}

/**
 * A copy of `q` adding `func(col) AS alias` to the projection. `func` is one of
 * `COUNT` / `SUM` / `AVG` / `MIN` / `MAX`; `col` may be `"*"` (for COUNT).
 * @param q {Query} the source query
 * @param fn {string} the aggregate function
 * @param col {string} the column (or `"*"`)
 * @param alias {string} the result-column alias
 * @return {Query} the extended query
 */
export func aggregate(q as Query, fn as string, col as string, alias as string) {
    def f as string init checkAggFunc($fn);
    checkColRef($col, "aggregate column");
    checkIdent($alias, "aggregate alias");
    def out as Query init $q;
    def ss as list of SelectItem init $out.selects;
    $ss[] = SelectItem{func: $f, column: $col, alias: $alias};
    $out.selects = $ss;
    return $out;
}

# ---- WHERE (AND / OR / IN) ----

# addWhere appends one single-value condition with the given connector.
func addWhere(q as Query, col as string, op as string, value as string, connector as string) {
    checkIdent($col, "column");
    def out as Query init $q;
    def ws as list of Condition init $out.wheres;
    $ws[] = Condition{column: $col, op: checkOp($op), connector: $connector, valueCount: 1};
    $out.wheres = $ws;
    def ps as list of string init $out.params;
    $ps[] = $value;
    $out.params = $ps;
    return $out;
}

# addIn appends one IN / NOT IN condition binding each value in `values`.
func addIn(q as Query, col as string, op as string, values as list of string, connector as string) {
    checkIdent($col, "column");
    if (len($values) == 0) {
        fail("an empty value list renders `IN ()`, which is invalid SQL");
    }
    def out as Query init $q;
    def ws as list of Condition init $out.wheres;
    $ws[] = Condition{column: $col, op: $op, connector: $connector, valueCount: len($values)};
    $out.wheres = $ws;
    def ps as list of string init $out.params;
    for (def v in $values) {
        $ps[] = $v;
    }
    $out.params = $ps;
    return $out;
}

/**
 * A copy of `q` with a `column op value` condition AND-joined to the rest. The
 * value binds as a parameter.
 * @param q {Query} the source query
 * @param col {string} the column
 * @param op {string} the operator (`=`, `>`, `<`, `>=`, `<=`, `!=`, `<>`, `LIKE`, `NOT LIKE`)
 * @param value {string} the value to bind
 * @return {Query} the extended query
 */
export func where(q as Query, col as string, op as string, value as string) {
    return addWhere($q, $col, $op, $value, "AND");
}

/**
 * Like `where`, but OR-joined to the previous condition. SQL binds `AND` tighter
 * than `OR`, so `where(...).orWhere(...)` reads `a AND b OR c` = `(a AND b) OR c`.
 * @param q {Query} the source query
 * @param col {string} the column
 * @param op {string} the operator
 * @param value {string} the value to bind
 * @return {Query} the extended query
 */
export func orWhere(q as Query, col as string, op as string, value as string) {
    return addWhere($q, $col, $op, $value, "OR");
}

/**
 * A copy of `q` with a `column IN (...)` condition AND-joined, one placeholder
 * bound per value.
 * @param q {Query} the source query
 * @param col {string} the column
 * @param values {list of string} the values (must be non-empty)
 * @return {Query} the extended query
 */
export func whereIn(q as Query, col as string, values as list of string) {
    return addIn($q, $col, "IN", $values, "AND");
}

/**
 * Like `whereIn`, but OR-joined to the previous condition.
 * @param q {Query} the source query
 * @param col {string} the column
 * @param values {list of string} the values (must be non-empty)
 * @return {Query} the extended query
 */
export func orWhereIn(q as Query, col as string, values as list of string) {
    return addIn($q, $col, "IN", $values, "OR");
}

/**
 * A copy of `q` with a `column NOT IN (...)` condition AND-joined.
 * @param q {Query} the source query
 * @param col {string} the column
 * @param values {list of string} the values (must be non-empty)
 * @return {Query} the extended query
 */
export func whereNotIn(q as Query, col as string, values as list of string) {
    return addIn($q, $col, "NOT IN", $values, "AND");
}

# addNullCond appends an `IS NULL` / `IS NOT NULL` condition (no bound value).
func addNullCond(q as Query, col as string, op as string, connector as string) {
    checkIdent($col, "column");
    def out as Query init $q;
    def ws as list of Condition init $out.wheres;
    $ws[] = Condition{column: $col, op: $op, connector: $connector, valueCount: 0};
    $out.wheres = $ws;
    return $out;
}

/**
 * A copy of `q` with a `column IS NULL` condition, AND-joined.
 * @param q {Query} the source query
 * @param col {string} the column
 * @return {Query} the extended query
 */
export func whereNull(q as Query, col as string) {
    return addNullCond($q, $col, "IS NULL", "AND");
}

/**
 * A copy of `q` with a `column IS NOT NULL` condition, AND-joined.
 * @param q {Query} the source query
 * @param col {string} the column
 * @return {Query} the extended query
 */
export func whereNotNull(q as Query, col as string) {
    return addNullCond($q, $col, "IS NOT NULL", "AND");
}

/**
 * A copy of `q` with a `column BETWEEN lo AND hi` condition, AND-joined. Both
 * bounds bind as parameters.
 * @param q {Query} the source query
 * @param col {string} the column
 * @param lo {string} the lower bound (inclusive)
 * @param hi {string} the upper bound (inclusive)
 * @return {Query} the extended query
 */
export func whereBetween(q as Query, col as string, lo as string, hi as string) {
    checkIdent($col, "column");
    def out as Query init $q;
    def ws as list of Condition init $out.wheres;
    $ws[] = Condition{column: $col, op: "BETWEEN", connector: "AND", valueCount: 2};
    $out.wheres = $ws;
    def ps as list of string init $out.params;
    $ps[] = $lo;
    $ps[] = $hi;
    $out.params = $ps;
    return $out;
}

# ---- GROUP BY / HAVING ----

/**
 * A copy of `q` grouping by the named columns.
 * @param q {Query} the source query
 * @param cols {list of string} the GROUP BY columns
 * @return {Query} the extended query
 */
export func groupBy(q as Query, cols as list of string) {
    def out as Query init $q;
    def gs as list of string init $out.groups;
    for (def c in $cols) {
        checkIdent($c, "group-by column");
        $gs[] = $c;
    }
    $out.groups = $gs;
    return $out;
}

# addHaving appends one HAVING condition over an aggregate.
func addHaving(q as Query, fn as string, col as string, op as string, value as string, connector as string) {
    def f as string init checkAggFunc($fn);
    checkColRef($col, "having column");
    def out as Query init $q;
    def hs as list of Having init $out.havings;
    $hs[] = Having{func: $f, column: $col, op: checkOp($op), connector: $connector};
    $out.havings = $hs;
    def ps as list of string init $out.havingParams;
    $ps[] = $value;
    $out.havingParams = $ps;
    return $out;
}

/**
 * A copy of `q` with a HAVING condition over an aggregate (`func(col) op value`),
 * AND-joined. HAVING filters groups, so it is used with `groupBy` / `aggregate`.
 * @param q {Query} the source query
 * @param fn {string} the aggregate function (COUNT / SUM / AVG / MIN / MAX)
 * @param col {string} the aggregated column (or `"*"`)
 * @param op {string} the operator
 * @param value {string} the value to bind
 * @return {Query} the extended query
 */
export func having(q as Query, fn as string, col as string, op as string, value as string) {
    return addHaving($q, $fn, $col, $op, $value, "AND");
}

/**
 * Like `having`, but OR-joined to the previous HAVING condition.
 * @param q {Query} the source query
 * @param fn {string} the aggregate function
 * @param col {string} the aggregated column (or `"*"`)
 * @param op {string} the operator
 * @param value {string} the value to bind
 * @return {Query} the extended query
 */
export func orHaving(q as Query, fn as string, col as string, op as string, value as string) {
    return addHaving($q, $fn, $col, $op, $value, "OR");
}

# ---- ORDER / LIMIT / OFFSET ----

/**
 * A copy of `q` with an ORDER BY term added.
 * @param q {Query} the source query
 * @param col {string} the column
 * @param dir {string} `"asc"` or `"desc"`
 * @return {Query} the extended query
 */
export func orderBy(q as Query, col as string, dir as string) {
    checkIdent($col, "order-by column");
    def out as Query init $q;
    def os as list of Order init $out.orders;
    $os[] = Order{column: $col, dir: checkDir($dir)};
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
 * A copy of `q` rendered as `SELECT DISTINCT ...`.
 * @param q {Query} the source query
 * @return {Query} the query with DISTINCT set
 */
export func distinct(q as Query) {
    def out as Query init $q;
    $out.distinctSelect = true;
    return $out;
}

/**
 * A copy of `q` with `LIMIT` / `OFFSET` set for a 1-based page: page `pageNum` of
 * `pageSize` rows (page 1 starts at offset 0).
 * @param q {Query} the source query
 * @param pageNum {int} the 1-based page number
 * @param pageSize {int} the rows per page
 * @return {Query} the paginated query
 */
export func page(q as Query, pageNum as int, pageSize as int) {
    if ($pageNum < 1) {
        fail("page number must be >= 1");
    }
    if ($pageSize < 1) {
        fail("page size must be >= 1");
    }
    if ($pageSize > MAX_PAGE_SIZE) {
        fail("page size exceeds the maximum of " + convert.toString(MAX_PAGE_SIZE));
    }
    return limit(offset($q, ($pageNum - 1) * $pageSize), $pageSize);
}

# ---- JOIN ----

# addJoin appends a typed join clause.
func addJoin(q as Query, kind as string, table as string, leftCol as string, rightCol as string) {
    checkIdent($table, "join table");
    checkIdent($leftCol, "join column");
    checkIdent($rightCol, "join column");
    def out as Query init $q;
    def js as list of Join init $out.joins;
    $js[] = Join{kind: $kind, table: $table, leftCol: $leftCol, rightCol: $rightCol};
    $out.joins = $js;
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
    return addJoin($q, "INNER", $table, $leftCol, $rightCol);
}

/**
 * A copy of `q` with a LEFT JOIN.
 * @param q {Query} the source query
 * @param table {string} the table to join
 * @param leftCol {string} the left join column (`table.col`)
 * @param rightCol {string} the right join column
 * @return {Query} the extended query
 */
export func leftJoin(q as Query, table as string, leftCol as string, rightCol as string) {
    return addJoin($q, "LEFT", $table, $leftCol, $rightCol);
}

/**
 * A copy of `q` with a RIGHT JOIN.
 * @param q {Query} the source query
 * @param table {string} the table to join
 * @param leftCol {string} the left join column (`table.col`)
 * @param rightCol {string} the right join column
 * @return {Query} the extended query
 */
export func rightJoin(q as Query, table as string, leftCol as string, rightCol as string) {
    return addJoin($q, "RIGHT", $table, $leftCol, $rightCol);
}

# ---- relations (associations) ----

# addRelation appends one relation to a copy of the schema, validating the
# identifiers that will reach SQL (via joinRelation / eager loading).
func addRelation(s as Schema, rel as Relation) {
    checkIdent($rel.name, "relation name");
    checkIdent($rel.target, "relation target table");
    checkIdent($rel.foreignKey, "relation foreign key");
    checkIdent($rel.localKey, "relation local key");
    if (not ($rel.through == "")) {
        checkIdent($rel.through, "relation join table");
        checkIdent($rel.throughLocalKey, "relation join-table local key");
        checkIdent($rel.throughTargetKey, "relation join-table target key");
    }
    def out as Schema init $s;
    def rels as list of Relation init $out.relations;
    $rels[] = $rel;
    $out.relations = $rels;
    return $out;
}

/**
 * Declare a **belongs-to** relation: **this** table holds `foreignKey`, which
 * references the target's primary key (`id` by convention). E.g. a post belongs
 * to an author via `posts.authorId -> authors.id`.
 * @param s {Schema} the source schema
 * @param name {string} the relation name
 * @param target {string} the target table
 * @param foreignKey {string} the foreign-key column on this table
 * @return {Schema} the schema with the relation added
 */
export func belongsTo(s as Schema, name as string, target as string, foreignKey as string) {
    return addRelation($s, Relation{
        name: $name,
        kind: RelationKind.BelongsTo,
        target: $target,
        foreignKey: $foreignKey,
        localKey: "id",
        through: "",
        throughLocalKey: "",
        throughTargetKey: ""
    });
}

/**
 * Declare a **has-one** relation: the **target** table holds `foreignKey`, which
 * references this table's primary key. E.g. a user has one profile via
 * `profiles.userId -> users.id`.
 * @param s {Schema} the source schema
 * @param name {string} the relation name
 * @param target {string} the target table
 * @param foreignKey {string} the foreign-key column on the target table
 * @return {Schema} the schema with the relation added
 */
export func hasOne(s as Schema, name as string, target as string, foreignKey as string) {
    return addRelation($s, Relation{
        name: $name,
        kind: RelationKind.HasOne,
        target: $target,
        foreignKey: $foreignKey,
        localKey: $s.primaryKey,
        through: "",
        throughLocalKey: "",
        throughTargetKey: ""
    });
}

/**
 * Declare a **has-many** relation: the **target** table holds `foreignKey`, which
 * references this table's primary key. E.g. an author has many posts via
 * `posts.authorId -> authors.id`.
 * @param s {Schema} the source schema
 * @param name {string} the relation name
 * @param target {string} the target table
 * @param foreignKey {string} the foreign-key column on the target table
 * @return {Schema} the schema with the relation added
 */
export func hasMany(s as Schema, name as string, target as string, foreignKey as string) {
    return addRelation($s, Relation{
        name: $name,
        kind: RelationKind.HasMany,
        target: $target,
        foreignKey: $foreignKey,
        localKey: $s.primaryKey,
        through: "",
        throughLocalKey: "",
        throughTargetKey: ""
    });
}

/**
 * Declare a **many-to-many** relation through a join table. E.g. a post has many
 * tags via `post_tags(postId, tagId)`: `manyToMany(s, "tags", "tags",
 * "post_tags", "postId", "tagId")`. The target's primary key is `id` by
 * convention, and this table's key is its primary key.
 * @param s {Schema} the source schema
 * @param name {string} the relation name
 * @param target {string} the target table
 * @param joinTable {string} the join (through) table
 * @param localFk {string} the join-table column referencing this table
 * @param targetFk {string} the join-table column referencing the target
 * @return {Schema} the schema with the relation added
 */
export func manyToMany(s as Schema, name as string, target as string, joinTable as string, localFk as string, targetFk as string) {
    return addRelation($s, Relation{
        name: $name,
        kind: RelationKind.ManyToMany,
        target: $target,
        foreignKey: "id",
        localKey: $s.primaryKey,
        through: $joinTable,
        throughLocalKey: $localFk,
        throughTargetKey: $targetFk
    });
}

# findRelation returns the named relation on the schema, throwing if absent.
func findRelation(s as Schema, name as string) {
    for (def r in $s.relations) {
        if ($r.name == $name) {
            return $r;
        }
    }
    fail("no relation named \"" + $name + "\" on " + $s.table);
    return $s.relations[0];
}

/**
 * A copy of `q` with the `JOIN`(s) that a declared relation implies - an
 * `INNER JOIN` on the correct key columns (two joins for a many-to-many, through
 * its join table). Built over the `join` primitive.
 * @param q {Query} the source query
 * @param s {Schema} the schema declaring the relation
 * @param relationName {string} the relation to join
 * @return {Query} the extended query
 */
export func joinRelation(q as Query, s as Schema, relationName as string) {
    def rel as Relation init findRelation($s, $relationName);
    match ($rel.kind) {
        when BelongsTo {
            return join($q, $rel.target, $s.table + "." + $rel.foreignKey,
                $rel.target + "." + $rel.localKey);
        }
        when HasOne {
            return joinChild($q, $s, $rel);
        }
        when HasMany {
            return joinChild($q, $s, $rel);
        }
        when ManyToMany {
            def q2 as Query init join($q, $rel.through,
                $rel.through + "." + $rel.throughLocalKey, $s.table + "." + $rel.localKey);
            return join($q2, $rel.target, $rel.through + "." + $rel.throughTargetKey,
                $rel.target + "." + $rel.foreignKey);
        }
    }
}

# joinChild joins a has-one / has-many target (FK on the target references this
# table's local key).
func joinChild(q as Query, s as Schema, rel as Relation) {
    return join($q, $rel.target, $rel.target + "." + $rel.foreignKey,
        $s.table + "." + $rel.localKey);
}

/**
 * A copy of `q` marking a declared relation to **eager-load** with `orm.load`.
 * Additive: call again for more relations. Does not change the base SQL (`toSql`
 * ignores it); `load` runs one extra batched query per marked relation.
 * @param q {Query} the source query
 * @param relationName {string} the relation to eager-load
 * @return {Query} the extended query
 */
export func with(q as Query, relationName as string) {
    def out as Query init $q;
    def ws as list of string init $out.withRelations;
    $ws[] = $relationName;
    $out.withRelations = $ws;
    return $out;
}

# ---- render ----

# renderSelects renders the projection list ("*" when the query projects nothing
# explicitly).
func renderSelects(q as Query) {
    if (len($q.selects) == 0) {
        return "*";
    }
    def items as list of string init [];
    for (def si in $q.selects) {
        if ($si.func == "") {
            $items[] = $si.column;
        } else {
            $items[] = $si.func + "(" + $si.column + ") AS " + $si.alias;
        }
    }
    return strings.join($items, ", ");
}

/**
 * Render a query to parameterized SQL for its dialect. Pure - the whole
 * query-builder surface is testable without a database. Every identifier and
 * operator is re-validated here (`validateQuery`), so a hand-built `Query`
 * literal that skipped the builder guards still cannot inject.
 * @param q {Query} the query
 * @return {Rendered} the SQL text and ordered bind values
 */
export func toSql(q as Query) {
    validateQuery($q);
    def prefix as string init "SELECT ";
    if ($q.distinctSelect) {
        $prefix = "SELECT DISTINCT ";
    }
    def stmt as string init $prefix + renderSelects($q) + " FROM " + $q.table;
    for (def j in $q.joins) {
        $stmt = $stmt + " " + $j.kind + " JOIN " + $j.table + " ON " + $j.leftCol + " = " +
            $j.rightCol;
    }
    $stmt = $stmt + renderWhereClause($q, 1);
    if (len($q.groups) > 0) {
        $stmt = $stmt + " GROUP BY " + strings.join($q.groups, ", ");
    }
    if (len($q.havings) > 0) {
        # HAVING placeholders continue after the WHERE ones.
        def n as int init 1 + len($q.params);
        $stmt = $stmt + " HAVING ";
        for (def i as int init 0; $i < len($q.havings); $i = $i + 1) {
            def h as Having init $q.havings[$i];
            if ($i > 0) {
                $stmt = $stmt + " " + $h.connector + " ";
            }
            $stmt = $stmt + $h.func + "(" + $h.column + ") " + $h.op + " " + ph($q.dialect, $n);
            $n = $n + 1;
        }
    }
    if (len($q.orders) > 0) {
        def terms as list of string init [];
        for (def o in $q.orders) {
            $terms[] = $o.column + " " + $o.dir;
        }
        $stmt = $stmt + " ORDER BY " + strings.join($terms, ", ");
    }
    if ($q.hasLimit) {
        $stmt = $stmt + " LIMIT " + convert.toString($q.limitN);
    }
    if ($q.hasOffset) {
        $stmt = $stmt + " OFFSET " + convert.toString($q.offsetN);
    }
    def allParams as list of string init [];
    for (def p in $q.params) {
        $allParams[] = $p;
    }
    for (def p in $q.havingParams) {
        $allParams[] = $p;
    }
    return Rendered{sql: $stmt, params: $allParams};
}

# ---- DDL rendering helpers ----

# isIntLiteral reports whether s is an optionally-signed integer literal (the
# guard for an Int-column DEFAULT, which reaches DDL text unparameterized).
func isIntLiteral(s as string) {
    def raw as bytes init convert.bytesFromString($s, "utf-8");
    if (len($raw) == 0) {
        return false;
    }
    def start as int init 0;
    if ($raw[0] == 43 or $raw[0] == 45) {
        $start = 1;
    }
    if ($start >= len($raw)) {
        return false;
    }
    for (def i as int init $start; $i < len($raw); $i = $i + 1) {
        if ($raw[$i] < 48 or $raw[$i] > 57) {
            return false;
        }
    }
    return true;
}

# isFloatLiteral reports whether s is an optionally-signed decimal number with at
# most one dot and at least one digit (the guard for a Float-column DEFAULT).
func isFloatLiteral(s as string) {
    def raw as bytes init convert.bytesFromString($s, "utf-8");
    if (len($raw) == 0) {
        return false;
    }
    def i as int init 0;
    if ($raw[0] == 43 or $raw[0] == 45) {
        $i = 1;
    }
    def digits as int init 0;
    def dots as int init 0;
    while ($i < len($raw)) {
        def b as int init $raw[$i];
        if ($b >= 48 and $b <= 57) {
            $digits = $digits + 1;
        } elseif ($b == 46) {
            $dots = $dots + 1;
            if ($dots > 1) {
                return false;
            }
        } else {
            return false;
        }
        $i = $i + 1;
    }
    return $digits > 0;
}

# escapeStringLiteral renders s as the body of a single-quoted SQL literal: each
# `'` is doubled. A backslash or control character is rejected, so the result is
# a safe literal in both dialects (MySQL backslash-escapes, Postgres does not) -
# a string DEFAULT can never break out of its quotes.
func escapeStringLiteral(s as string) {
    def raw as bytes init convert.bytesFromString($s, "utf-8");
    for (def i as int init 0; $i < len($raw); $i = $i + 1) {
        if ($raw[$i] == 92) {
            fail("a string DEFAULT may not contain a backslash");
        }
        if ($raw[$i] < 32) {
            fail("a string DEFAULT may not contain a control character");
        }
    }
    return strings.replace($s, "'", "''");
}

# renderDefault renders a DEFAULT value by column kind: numeric literals bare
# (validated), booleans as TRUE / FALSE, strings / bytes as an escaped quoted
# literal. Each path validates or escapes, so no default injects DDL.
func renderDefault(kind as ColumnKind, value as string) {
    match ($kind) {
        when Int {
            if (not isIntLiteral($value)) {
                fail("integer DEFAULT must be an integer literal, got: " + $value);
            }
            return $value;
        }
        when Float {
            if (not isFloatLiteral($value)) {
                fail("float DEFAULT must be a numeric literal, got: " + $value);
            }
            return $value;
        }
        when Bool {
            def v as string init strings.lower(strings.trim($value));
            match ($v) {
                when "true", "1" {
                    return "TRUE";
                }
                when "false", "0" {
                    return "FALSE";
                }
                else {
                    fail("boolean DEFAULT must be true/false/1/0, got: " + $value);
                }
            }
        }
        when String {
            return "'" + escapeStringLiteral($value) + "'";
        }
        when Bytes {
            return "'" + escapeStringLiteral($value) + "'";
        }
    }
}

# renderColumnDef renders one column's DDL fragment: `name TYPE [NOT NULL]
# [DEFAULT x] [UNIQUE]`, with auto-increment spelled per dialect (SERIAL replaces
# the type on Postgres; AUTO_INCREMENT follows it on MySQL).
func renderColumnDef(c as Column, dialect as Dialect) {
    def out as string init $c.name;
    if ($c.autoIncrement) {
        match ($dialect) {
            when Postgres {
                $out = $out + " SERIAL";
            }
            when Mysql {
                $out = $out + " " + sqlType($c.kind, $dialect) + " AUTO_INCREMENT";
            }
        }
    } else {
        $out = $out + " " + sqlType($c.kind, $dialect);
    }
    if (not $c.nullable) {
        $out = $out + " NOT NULL";
    }
    if ($c.hasDefault) {
        $out = $out + " DEFAULT " + renderDefault($c.kind, $c.default);
    }
    if ($c.unique) {
        $out = $out + " UNIQUE";
    }
    return $out;
}

/**
 * The CREATE TABLE DDL for a schema, in its dialect - including each column's
 * `NOT NULL` / `DEFAULT` / `UNIQUE` / auto-increment attributes and the primary
 * key. Pair it with the migration runner (`orm.migrate`) or run it directly.
 * @param s {Schema} the schema
 * @return {string} the CREATE TABLE statement
 */
export func createTable(s as Schema) {
    validateSchema($s);
    def cols as string init "";
    for (def i as int init 0; $i < len($s.columns); $i = $i + 1) {
        if ($i > 0) {
            $cols = $cols + ", ";
        }
        $cols = $cols + renderColumnDef($s.columns[$i], $s.dialect);
    }
    return "CREATE TABLE " + $s.table + " (" + $cols + ", PRIMARY KEY (" + $s.primaryKey + "))";
}

# ---- standalone DDL builders (for migrations) ----

/**
 * The `DROP TABLE` DDL for a table.
 * @param table {string} the table name
 * @return {string} the DROP TABLE statement
 */
export func dropTable(table as string) {
    checkIdent($table, "table name");
    return "DROP TABLE " + $table;
}

/**
 * The `ALTER TABLE ... ADD COLUMN` DDL for a new column (name + type, no
 * attributes; express attribute-rich columns in a `createTable` schema).
 * @param table {string} the table name
 * @param name {string} the new column name
 * @param kind {ColumnKind} the column value kind
 * @param dialect {Dialect} the SQL dialect (for the type spelling)
 * @return {string} the ALTER TABLE ADD COLUMN statement
 */
export func addColumn(table as string, name as string, kind as ColumnKind, dialect as Dialect) {
    checkIdent($table, "table name");
    checkIdent($name, "column name");
    return "ALTER TABLE " + $table + " ADD COLUMN " + $name + " " + sqlType($kind, $dialect);
}

/**
 * The `ALTER TABLE ... DROP COLUMN` DDL.
 * @param table {string} the table name
 * @param name {string} the column to drop
 * @return {string} the ALTER TABLE DROP COLUMN statement
 */
export func dropColumn(table as string, name as string) {
    checkIdent($table, "table name");
    checkIdent($name, "column name");
    return "ALTER TABLE " + $table + " DROP COLUMN " + $name;
}

/**
 * The `ALTER TABLE ... RENAME COLUMN` DDL (modern MySQL 8+ / MariaDB / Postgres
 * syntax).
 * @param table {string} the table name
 * @param fromName {string} the current column name
 * @param toName {string} the new column name
 * @return {string} the ALTER TABLE RENAME COLUMN statement
 */
export func renameColumn(table as string, fromName as string, toName as string) {
    checkIdent($table, "table name");
    checkIdent($fromName, "column name");
    checkIdent($toName, "column name");
    return "ALTER TABLE " + $table + " RENAME COLUMN " + $fromName + " TO " + $toName;
}

/**
 * The `CREATE [UNIQUE] INDEX` DDL over one or more columns.
 * @param name {string} the index name
 * @param table {string} the table name
 * @param columns {list of string} the indexed columns (non-empty)
 * @param isUnique {bool} whether the index is UNIQUE
 * @return {string} the CREATE INDEX statement
 */
export func createIndex(name as string, table as string, columns as list of string, isUnique as bool) {
    checkIdent($name, "index name");
    checkIdent($table, "table name");
    if (len($columns) == 0) {
        fail("createIndex needs at least one column");
    }
    def cols as list of string init [];
    for (def c in $columns) {
        checkIdent($c, "index column");
        $cols[] = $c;
    }
    def kw as string init "CREATE INDEX ";
    if ($isUnique) {
        $kw = "CREATE UNIQUE INDEX ";
    }
    return $kw + $name + " ON " + $table + " (" + strings.join($cols, ", ") + ")";
}

/**
 * The `DROP INDEX` DDL (Postgres drops by index name; MySQL needs the table).
 * @param name {string} the index name
 * @param table {string} the table the index is on
 * @param dialect {Dialect} the SQL dialect
 * @return {string} the DROP INDEX statement
 */
export func dropIndex(name as string, table as string, dialect as Dialect) {
    checkIdent($name, "index name");
    checkIdent($table, "table name");
    match ($dialect) {
        when Postgres {
            return "DROP INDEX " + $name;
        }
        when Mysql {
            return "DROP INDEX " + $name + " ON " + $table;
        }
    }
}

/**
 * The `ALTER TABLE ... ADD CONSTRAINT ... FOREIGN KEY` DDL.
 * @param table {string} the table carrying the foreign key
 * @param name {string} the constraint name
 * @param column {string} the local foreign-key column
 * @param refTable {string} the referenced table
 * @param refColumn {string} the referenced column
 * @return {string} the ALTER TABLE ADD CONSTRAINT statement
 */
export func addForeignKey(table as string, name as string, column as string, refTable as string, refColumn as string) {
    checkIdent($table, "table name");
    checkIdent($name, "constraint name");
    checkIdent($column, "foreign-key column");
    checkIdent($refTable, "referenced table");
    checkIdent($refColumn, "referenced column");
    return "ALTER TABLE " + $table + " ADD CONSTRAINT " + $name + " FOREIGN KEY (" + $column +
        ") REFERENCES " + $refTable + " (" + $refColumn + ")";
}

/**
 * The `ALTER TABLE ... DROP` foreign-key DDL (Postgres `DROP CONSTRAINT`; MySQL
 * `DROP FOREIGN KEY`).
 * @param table {string} the table name
 * @param name {string} the constraint name
 * @param dialect {Dialect} the SQL dialect
 * @return {string} the ALTER TABLE DROP statement
 */
export func dropForeignKey(table as string, name as string, dialect as Dialect) {
    checkIdent($table, "table name");
    checkIdent($name, "constraint name");
    match ($dialect) {
        when Postgres {
            return "ALTER TABLE " + $table + " DROP CONSTRAINT " + $name;
        }
        when Mysql {
            return "ALTER TABLE " + $table + " DROP FOREIGN KEY " + $name;
        }
    }
}

# ---- statement builders (pure, for CRUD) ----

# buildInsert renders an INSERT for the columns present in the record.
func buildInsert(s as Schema, record as map of string to string) {
    validateSchema($s);
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
    return Rendered{
        sql: "INSERT INTO " + $s.table + " (" + $cols + ") VALUES (" + $phs + ")",
        params: $params
    };
}

# buildUpdate renders an UPDATE of every non-key present column, matched by key.
func buildUpdate(s as Schema, record as map of string to string) {
    validateSchema($s);
    if (not maps.has($record, $s.primaryKey)) {
        throw Error{
            kind: "orm",
            message: "orm.update: record has no primary key (" + $s.primaryKey + ")",
            file: "",
            line: 0,
            col: 0
        };
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
    def stmt as string init "UPDATE " + $s.table + " SET " + $sets + " WHERE " + $s.primaryKey +
        " = " + ph($s.dialect, $n);
    $params[] = $record[$s.primaryKey];
    return Rendered{sql: $stmt, params: $params};
}

# buildByKey renders a `verb ... WHERE pk = ph` for find (SELECT) / delete.
func buildByKey(verb as string, s as Schema, id as string) {
    validateSchema($s);
    def params as list of string init [];
    $params[] = $id;
    def stmt as string init $verb + " FROM " + $s.table + " WHERE " + $s.primaryKey + " = " +
        ph($s.dialect, 1);
    return Rendered{sql: $stmt, params: $params};
}

# presentColumns returns the schema columns present in the record, in schema order.
func presentColumns(s as Schema, record as map of string to string) {
    def out as list of string init [];
    for (def i as int init 0; $i < len($s.columns); $i = $i + 1) {
        def name as string init $s.columns[$i].name;
        if (maps.has($record, $name)) {
            $out[] = $name;
        }
    }
    return $out;
}

# buildUpsert renders an insert-or-update: Postgres `ON CONFLICT (...) DO UPDATE`,
# MySQL `ON DUPLICATE KEY UPDATE`. The non-conflict present columns are updated
# (or DO NOTHING / a no-op when every present column is a conflict column).
func buildUpsert(s as Schema, record as map of string to string, conflictCols as list of string) {
    validateSchema($s);
    if (len($conflictCols) == 0) {
        fail("upsert needs at least one conflict column (the unique / primary key)");
    }
    def conflictSet as map of string to string init {};
    for (def c in $conflictCols) {
        checkIdent($c, "conflict column");
        $conflictSet[$c] = "1";
    }
    def cols as list of string init presentColumns($s, $record);
    if (len($cols) == 0) {
        fail("upsert: the record has no columns to insert");
    }
    def phs as list of string init [];
    def params as list of string init [];
    def n as int init 1;
    for (def c in $cols) {
        $phs[] = ph($s.dialect, $n);
        $n = $n + 1;
        $params[] = $record[$c];
    }
    def sets as list of string init [];
    for (def c in $cols) {
        if (not maps.has($conflictSet, $c)) {
            match ($s.dialect) {
                when Postgres {
                    $sets[] = $c + " = EXCLUDED." + $c;
                }
                when Mysql {
                    $sets[] = $c + " = VALUES(" + $c + ")";
                }
            }
        }
    }
    def head as string init "INSERT INTO " + $s.table + " (" + strings.join($cols, ", ") +
        ") VALUES (" + strings.join($phs, ", ") + ")";
    def stmt as string init "";
    match ($s.dialect) {
        when Postgres {
            def onConflict as string init " ON CONFLICT (" + strings.join($conflictCols, ", ") + ")";
            if (len($sets) == 0) {
                $stmt = $head + $onConflict + " DO NOTHING";
            } else {
                $stmt = $head + $onConflict + " DO UPDATE SET " + strings.join($sets, ", ");
            }
        }
        when Mysql {
            if (len($sets) == 0) {
                # every present column is a conflict column: a harmless no-op update.
                $stmt = $head + " ON DUPLICATE KEY UPDATE " + $conflictCols[0] + " = " +
                    $conflictCols[0];
            } else {
                $stmt = $head + " ON DUPLICATE KEY UPDATE " + strings.join($sets, ", ");
            }
        }
    }
    return Rendered{sql: $stmt, params: $params};
}

# renderWhereClause renders a query's WHERE conditions as a ` WHERE ...` fragment,
# numbering placeholders from startN ("" when there are no conditions). Shared by
# updateWhere / deleteWhere; the values are the query's own `params`.
func renderWhereClause(q as Query, startN as int) {
    if (len($q.wheres) == 0) {
        return "";
    }
    def stmt as string init " WHERE ";
    def n as int init $startN;
    for (def i as int init 0; $i < len($q.wheres); $i = $i + 1) {
        def c as Condition init $q.wheres[$i];
        if ($i > 0) {
            $stmt = $stmt + " " + $c.connector + " ";
        }
        if ($c.op == "IN" or $c.op == "NOT IN") {
            $stmt = $stmt + $c.column + " " + $c.op + " (";
            for (def k as int init 0; $k < $c.valueCount; $k = $k + 1) {
                if ($k > 0) {
                    $stmt = $stmt + ", ";
                }
                $stmt = $stmt + ph($q.dialect, $n);
                $n = $n + 1;
            }
            $stmt = $stmt + ")";
        } elseif ($c.op == "IS NULL" or $c.op == "IS NOT NULL") {
            $stmt = $stmt + $c.column + " " + $c.op; # no placeholder
        } elseif ($c.op == "BETWEEN") {
            $stmt = $stmt + $c.column + " BETWEEN " + ph($q.dialect, $n) + " AND " +
                ph($q.dialect, $n + 1);
            $n = $n + 2;
        } else {
            $stmt = $stmt + $c.column + " " + $c.op + " " + ph($q.dialect, $n);
            $n = $n + 1;
        }
    }
    return $stmt;
}

# validateConditions re-checks a query's table + WHERE identifiers / operators (the
# subset updateWhere / deleteWhere render), the injection backstop for those verbs.
func validateConditions(q as Query) {
    checkIdent($q.table, "table");
    for (def c in $q.wheres) {
        checkIdent($c.column, "where column");
        checkRenderedOp($c.op);
    }
}

# buildUpdateWhere renders a bulk `UPDATE t SET ... WHERE ...` from an assignments
# map and a query's WHERE (SET placeholders first, then WHERE placeholders).
func buildUpdateWhere(s as Schema, assignments as map of string to string, q as Query) {
    validateSchema($s);
    validateConditions($q);
    if (len($q.wheres) == 0) {
        fail("updateWhere needs a WHERE (refusing to update every row); add a condition to the query");
    }
    def keys as list of string init maps.keys($assignments);
    if (len($keys) == 0) {
        fail("updateWhere: no assignments");
    }
    def sets as list of string init [];
    def params as list of string init [];
    def n as int init 1;
    for (def k in $keys) {
        checkIdent($k, "assignment column");
        $sets[] = $k + " = " + ph($s.dialect, $n);
        $n = $n + 1;
        $params[] = $assignments[$k];
    }
    def stmt as string init "UPDATE " + $s.table + " SET " + strings.join($sets, ", ") +
        renderWhereClause($q, $n);
    for (def p in $q.params) {
        $params[] = $p;
    }
    return Rendered{sql: $stmt, params: $params};
}

# buildDeleteWhere renders a bulk `DELETE FROM t WHERE ...` from a query's WHERE.
func buildDeleteWhere(s as Schema, q as Query) {
    validateSchema($s);
    validateConditions($q);
    if (len($q.wheres) == 0) {
        fail("deleteWhere needs a WHERE (refusing to delete every row); add a condition to the query");
    }
    def stmt as string init "DELETE FROM " + $s.table + renderWhereClause($q, 1);
    def params as list of string init [];
    for (def p in $q.params) {
        $params[] = $p;
    }
    return Rendered{sql: $stmt, params: $params};
}

# buildInsertManyChunk renders one multi-row INSERT for a slice of records that all
# write the same `cols`.
func buildInsertManyChunk(s as Schema, cols as list of string, records as list of map of string to string) {
    def rowGroups as list of string init [];
    def params as list of string init [];
    def n as int init 1;
    for (def rec in $records) {
        def phs as list of string init [];
        for (def c in $cols) {
            if (not maps.has($rec, $c)) {
                fail("insertMany: every record must set the same columns; missing \"" + $c + "\"");
            }
            $phs[] = ph($s.dialect, $n);
            $n = $n + 1;
            $params[] = $rec[$c];
        }
        # Reject a record that sets *extra* schema columns beyond the first record's
        # set - otherwise those columns would be silently dropped.
        if (len(presentColumns($s, $rec)) != len($cols)) {
            fail("insertMany: every record must set the same columns (a later record sets extra columns)");
        }
        $rowGroups[] = "(" + strings.join($phs, ", ") + ")";
    }
    def stmt as string init "INSERT INTO " + $s.table + " (" + strings.join($cols, ", ") +
        ") VALUES " + strings.join($rowGroups, ", ");
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
 * @param session {Session} the session (`orm.session` or `orm.transaction`)
 * @param s {Schema} the schema
 * @param record {map of string to string} column name -> value
 * @return {sql.Result} the affected-rows / last-insert-id result
 */
export func insert(session as Session, s as Schema, record as map of string to string) {
    def r as Rendered init buildInsert($s, $record);
    return sessExec($session, $r.sql, $r.params);
}

/**
 * Find a single row by primary-key value.
 * @param session {Session} the session (`orm.session` or `orm.transaction`)
 * @param s {Schema} the schema
 * @param id {string} the primary-key value
 * @return {map of string to string} the row (column name -> value)
 * @throws {Error} when no row has that key
 */
export func find(session as Session, s as Schema, id as string) {
    def r as Rendered init buildByKey("SELECT *", $s, $id);
    def rows as sql.Rows init sessQuery($session, $r.sql, $r.params);
    defer sql.closeRows($rows);
    if (not sql.next($rows)) {
        throw Error{
            kind: "orm",
            message: "orm.find: no " + $s.table + " with " + $s.primaryKey + " = " + $id,
            file: "",
            line: 0,
            col: 0
        };
    }
    return mapRow($rows);
}

/**
 * Update a record, matched by its primary-key value (which the record must
 * carry). Every other present column is written.
 * @param session {Session} the session (`orm.session` or `orm.transaction`)
 * @param s {Schema} the schema
 * @param record {map of string to string} the row, including the primary key
 * @return {sql.Result} the affected-rows result
 * @throws {Error} when the record has no primary-key value
 */
export func update(session as Session, s as Schema, record as map of string to string) {
    def r as Rendered init buildUpdate($s, $record);
    return sessExec($session, $r.sql, $r.params);
}

/**
 * Delete a row by primary-key value.
 * @param session {Session} the session (`orm.session` or `orm.transaction`)
 * @param s {Schema} the schema
 * @param id {string} the primary-key value
 * @return {sql.Result} the affected-rows result
 */
export func delete(session as Session, s as Schema, id as string) {
    def r as Rendered init buildByKey("DELETE", $s, $id);
    return sessExec($session, $r.sql, $r.params);
}

/**
 * Run a query and return every matching row.
 * @param session {Session} the session (`orm.session` or `orm.transaction`)
 * @param q {Query} the query (from the builder)
 * @return {list of map of string to string} the rows
 */
export func all(session as Session, q as Query) {
    def r as Rendered init toSql($q);
    def rows as sql.Rows init sessQuery($session, $r.sql, $r.params);
    defer sql.closeRows($rows);
    def out as list of map of string to string init [];
    repeat {
        if (not sql.next($rows)) {
            break;
        }
        $out[] = mapRow($rows);
    } until (false);
    return $out;
}

# ---- eager loading (N+1 elimination) ----

/**
 * The loaded child rows for one eager-loaded relation: a parent-key -> child-rows
 * lookup, built once from a single batched query. Internal to a `Result`; read it
 * through `orm.related` / `orm.relatedOne`, not directly.
 * @field name {string} the relation name
 * @field parentKeyColumn {string} the base-row column whose value indexes the lookup
 * @field byParentKey {map of string to list of map of string to string} parent-key value -> its child rows
 */
export def struct RelationData {
    name as string,
    parentKeyColumn as string,
    byParentKey as map of string to list of map of string to string
};

/**
 * The result of `orm.load`: the base query's rows plus the eager-loaded relations.
 * Read the base rows with `orm.rows` and a row's related rows with `orm.related`
 * (has-many / many-to-many) or `orm.relatedOne` (belongs-to / has-one).
 * @field rows {list of map of string to string} the base query's rows
 * @field relations {list of RelationData} one entry per eager-loaded relation
 */
export def struct Result {
    rows as list of map of string to string,
    relations as list of RelationData
};

# baseKeyColumn is the base-row column that supplies the IN values (and indexes
# the lookup for the accessor).
func baseKeyColumn(rel as Relation) {
    match ($rel.kind) {
        when BelongsTo {
            return $rel.foreignKey;
        }
        when HasOne {
            return $rel.localKey;
        }
        when HasMany {
            return $rel.localKey;
        }
        when ManyToMany {
            return $rel.localKey;
        }
    }
}

# childKeyColumn is the fetched-child column to group by (equals a base row's
# baseKeyColumn value, by the foreign-key relationship).
func childKeyColumn(rel as Relation) {
    match ($rel.kind) {
        when BelongsTo {
            return $rel.localKey;
        }
        when HasOne {
            return $rel.foreignKey;
        }
        when HasMany {
            return $rel.foreignKey;
        }
        when ManyToMany {
            return $rel.throughLocalKey;
        }
    }
}

# distinctKeys collects the distinct non-empty values of a column across rows,
# in first-seen order (the IN-list for a batched child query).
func distinctKeys(rows as list of map of string to string, col as string) {
    def seen as map of string to string init {};
    def out as list of string init [];
    for (def r in $rows) {
        if (maps.has($r, $col)) {
            def k as string init $r[$col];
            if ($k != "" and not maps.has($seen, $k)) {
                $seen[$k] = "1";
                $out[] = $k;
            }
        }
    }
    return $out;
}

# groupRows indexes rows by a column value -> the list of rows with that value.
func groupRows(rows as list of map of string to string, col as string) {
    def out as map of string to list of map of string to string init {};
    for (def r in $rows) {
        def k as string init $r[$col];
        if (maps.has($out, $k)) {
            def cur as list of map of string to string init $out[$k];
            $cur[] = $r;
            $out[$k] = $cur;
        } else {
            def fresh as list of map of string to string init [$r];
            $out[$k] = $fresh;
        }
    }
    return $out;
}

# targetSchema builds a throwaway schema for a related table (for the query
# builder); only the table + dialect matter for a SELECT.
func targetSchema(table as string, key as string, dialect as Dialect) {
    return Schema{table: $table, columns: [], primaryKey: $key, dialect: $dialect, relations: []};
}

# chunkKeys splits keys into batches of at most `size`, so a batched `WHERE fk IN
# (...)` never exceeds the per-statement placeholder ceiling. An empty input
# yields no chunks (so no query runs); each chunk is non-empty.
func chunkKeys(keys as list of string, size as int) {
    def out as list of list of string init [];
    def i as int init 0;
    def total as int init len($keys);
    while ($i < $total) {
        def chunk as list of string init [];
        while ($i < $total and len($chunk) < $size) {
            $chunk[] = $keys[$i];
            $i = $i + 1;
        }
        $out[] = $chunk;
    }
    return $out;
}

# fetchChildren runs the batched query for one relation and returns the child
# rows, splitting a large key set across several `IN (...)` statements so the
# placeholder count never exceeds MAX_QUERY_PARAMS (no query when there are no
# parent keys). So eager loading stays bounded even over a huge parent set.
func fetchChildren(session as Session, schema as Schema, rel as Relation, keys as list of string) {
    def out as list of map of string to string init [];
    for (def chunk in chunkKeys($keys, MAX_QUERY_PARAMS)) {
        for (def row in fetchChildrenChunk($session, $schema, $rel, $chunk)) {
            $out[] = $row;
        }
    }
    return $out;
}

# fetchChildrenChunk runs one batched query for a single (non-empty, bounded) key
# chunk.
func fetchChildrenChunk(session as Session, schema as Schema, rel as Relation, keys as list of string) {
    match ($rel.kind) {
        when ManyToMany {
            # `SELECT *` across the join keeps every joined column (so join-table
            # pivot payload survives). Caveat: if the target and the join table
            # share a column name, that name collapses in the row map (last wins) -
            # join tables conventionally use distinct FK names, so this is rare.
            def target as Schema init targetSchema($rel.target, $rel.foreignKey, $schema.dialect);
            def joined as Query init join(from($target), $rel.through,
                $rel.through + "." + $rel.throughTargetKey, $rel.target + "." + $rel.foreignKey);
            def q as Query init whereIn($joined, $rel.through + "." + $rel.throughLocalKey, $keys);
            return all($session, $q);
        }
        when BelongsTo {
            return fetchInCol($session, $schema, $rel.target, $rel.localKey, $keys);
        }
        when HasOne {
            return fetchInCol($session, $schema, $rel.target, $rel.foreignKey, $keys);
        }
        when HasMany {
            return fetchInCol($session, $schema, $rel.target, $rel.foreignKey, $keys);
        }
    }
}

# fetchInCol runs `SELECT * FROM table WHERE col IN (keys)` (values parameterized).
func fetchInCol(session as Session, schema as Schema, table as string, col as string, keys as list of string) {
    def target as Schema init targetSchema($table, $col, $schema.dialect);
    def q as Query init whereIn(from($target), $col, $keys);
    return all($session, $q);
}

# loadRelation runs one relation's batched query and builds its parent-key lookup.
func loadRelation(session as Session, schema as Schema, rel as Relation, baseRows as list of map of string to string) {
    def baseCol as string init baseKeyColumn($rel);
    def childCol as string init childKeyColumn($rel);
    def keys as list of string init distinctKeys($baseRows, $baseCol);
    def children as list of map of string to string init fetchChildren($session, $schema, $rel, $keys);
    return RelationData{
        name: $rel.name,
        parentKeyColumn: $baseCol,
        byParentKey: groupRows($children, $childCol)
    };
}

# plannedQueryCount returns how many SQL statements `load` issues for the given
# base rows: 1 (base) + one per key-chunk of each eager-loaded relation (the same
# key + chunking logic `load` runs). That is 1 + R for the usual case where each
# relation's key set fits one batch, growing only when a set exceeds
# MAX_QUERY_PARAMS.
func plannedQueryCount(schema as Schema, q as Query, baseRows as list of map of string to string) {
    def n as int init 1;
    for (def relName in $q.withRelations) {
        def rel as Relation init findRelation($schema, $relName);
        def keys as list of string init distinctKeys($baseRows, baseKeyColumn($rel));
        $n = $n + len(chunkKeys($keys, MAX_QUERY_PARAMS));
    }
    return $n;
}

/**
 * Run a query and eager-load its `orm.with`-marked relations in a **fixed 1 + R**
 * queries (R = relations requested): the base query once, then **one** batched
 * `WHERE fk IN (...)` per relation - never one query per row. Returns a `Result`
 * walked by `orm.rows` / `related` / `relatedOne`.
 * @param session {Session} the session (`orm.session` or `orm.transaction`)
 * @param schema {Schema} the base schema (declares the relations)
 * @param q {Query} the query, with relations marked by `orm.with`
 * @return {Result} the base rows plus the eager-loaded relations
 */
export func load(session as Session, schema as Schema, q as Query) {
    def baseRows as list of map of string to string init all($session, $q);
    def rds as list of RelationData init [];
    for (def relName in $q.withRelations) {
        def rel as Relation init findRelation($schema, $relName);
        $rds[] = loadRelation($session, $schema, $rel, $baseRows);
    }
    return Result{rows: $baseRows, relations: $rds};
}

# findRelationData returns the loaded data for a relation, throwing if it was not
# eager-loaded.
func findRelationData(result as Result, name as string) {
    for (def rd in $result.relations) {
        if ($rd.name == $name) {
            return $rd;
        }
    }
    fail("relation \"" + $name + "\" was not eager-loaded (mark it with orm.with before orm.load)");
    return $result.relations[0];
}

/**
 * The base rows of a `Result` (the same rows `orm.all` would return).
 * @param result {Result} the load result
 * @return {list of map of string to string} the base rows
 */
export func rows(result as Result) {
    return $result.rows;
}

/**
 * The eager-loaded child rows of `row` for a has-many / many-to-many relation
 * (an empty list if the row has none). No query - a lookup into the `Result`.
 * @param result {Result} the load result
 * @param row {map of string to string} one base row (from `orm.rows`)
 * @param name {string} the relation name
 * @return {list of map of string to string} the related child rows
 */
export func related(result as Result, row as map of string to string, name as string) {
    def rd as RelationData init findRelationData($result, $name);
    if (not maps.has($row, $rd.parentKeyColumn)) {
        fail("row has no \"" + $rd.parentKeyColumn +
            "\" column (eager loading needs the relation key in the projection)");
    }
    def key as string init $row[$rd.parentKeyColumn];
    if (maps.has($rd.byParentKey, $key)) {
        return $rd.byParentKey[$key];
    }
    def empty as list of map of string to string init [];
    return $empty;
}

/**
 * The single eager-loaded row of `row` for a belongs-to / has-one relation, or an
 * **empty map** when there is none (a null / unmatched foreign key). No query.
 * @param result {Result} the load result
 * @param row {map of string to string} one base row (from `orm.rows`)
 * @param name {string} the relation name
 * @return {map of string to string} the related row, or `{}` if none
 */
export func relatedOne(result as Result, row as map of string to string, name as string) {
    def kids as list of map of string to string init related($result, $row, $name);
    if (len($kids) > 0) {
        return $kids[0];
    }
    def empty as map of string to string init {};
    return $empty;
}

# ---- write path (upsert / batch / save / returning) ----

/**
 * Insert `record`, or update the conflicting row if a unique / primary-key
 * conflict on `conflictCols` occurs (Postgres `ON CONFLICT ... DO UPDATE`, MySQL
 * `ON DUPLICATE KEY UPDATE`). The non-conflict present columns are updated.
 * @param session {Session} the session (`orm.session` or `orm.transaction`)
 * @param s {Schema} the schema
 * @param record {map of string to string} the row to insert / update
 * @param conflictCols {list of string} the conflict-target columns (unique / PK; non-empty)
 * @return {sql.Result} the affected-rows result
 */
export func upsert(session as Session, s as Schema, record as map of string to string, conflictCols as list of string) {
    def r as Rendered init buildUpsert($s, $record, $conflictCols);
    return sessExec($session, $r.sql, $r.params);
}

/**
 * Insert many records in one multi-row `INSERT`. Every record must set the same
 * columns (those present in the first). A batch whose placeholder count would
 * exceed the per-statement limit is split across several `INSERT`s (never
 * silently capped); wrap the call in `orm.transaction` for all-or-nothing.
 * @param session {Session} the session (`orm.session` or `orm.transaction`)
 * @param s {Schema} the schema
 * @param records {list of map of string to string} the rows to insert
 * @return {int} the number of records inserted
 */
export func insertMany(session as Session, s as Schema, records as list of map of string to string) {
    if (len($records) == 0) {
        return 0;
    }
    validateSchema($s);
    def cols as list of string init presentColumns($s, $records[0]);
    if (len($cols) == 0) {
        fail("insertMany: the first record has no columns to insert");
    }
    def maxRows as int init MAX_QUERY_PARAMS // len($cols);
    if ($maxRows < 1) {
        $maxRows = 1;
    }
    def total as int init len($records);
    def i as int init 0;
    while ($i < $total) {
        def chunk as list of map of string to string init [];
        while ($i < $total and len($chunk) < $maxRows) {
            $chunk[] = $records[$i];
            $i = $i + 1;
        }
        def r as Rendered init buildInsertManyChunk($s, $cols, $chunk);
        sessExec($session, $r.sql, $r.params);
    }
    return $total;
}

/**
 * Insert `record` and return the generated primary-key value (Postgres
 * `RETURNING pk`; MySQL `LAST_INSERT_ID` via the result's `lastId`).
 * @param session {Session} the session (`orm.session` or `orm.transaction`)
 * @param s {Schema} the schema
 * @param record {map of string to string} the row to insert
 * @return {string} the generated primary-key value
 */
export func insertReturning(session as Session, s as Schema, record as map of string to string) {
    def r as Rendered init buildInsert($s, $record);
    match ($s.dialect) {
        when Postgres {
            def stmt as string init $r.sql + " RETURNING " + $s.primaryKey;
            def rows as sql.Rows init sessQuery($session, $stmt, $r.params);
            defer sql.closeRows($rows);
            if (not sql.next($rows)) {
                fail("insertReturning: the insert returned no row");
            }
            return sql.asString($rows, $s.primaryKey);
        }
        when Mysql {
            def res as sql.Result init sessExec($session, $r.sql, $r.params);
            return convert.toString($res.lastId);
        }
    }
}

/**
 * Bulk-update every row matching a query's `WHERE`, setting `assignments`
 * (column -> value, bound as parameters). Refuses a query with no `WHERE`.
 * @param session {Session} the session (`orm.session` or `orm.transaction`)
 * @param s {Schema} the schema
 * @param assignments {map of string to string} the columns to set
 * @param q {Query} the query whose `WHERE` selects the rows
 * @return {sql.Result} the affected-rows result
 */
export func updateWhere(session as Session, s as Schema, assignments as map of string to string, q as Query) {
    def r as Rendered init buildUpdateWhere($s, $assignments, $q);
    return sessExec($session, $r.sql, $r.params);
}

/**
 * Bulk-delete every row matching a query's `WHERE`. Refuses a query with no
 * `WHERE` (use a raw `sql.exec` to truncate deliberately).
 * @param session {Session} the session (`orm.session` or `orm.transaction`)
 * @param s {Schema} the schema
 * @param q {Query} the query whose `WHERE` selects the rows
 * @return {sql.Result} the affected-rows result
 */
export func deleteWhere(session as Session, s as Schema, q as Query) {
    def r as Rendered init buildDeleteWhere($s, $q);
    return sessExec($session, $r.sql, $r.params);
}

/**
 * Insert `record` when it carries **no** primary key, else update it (matched by
 * the primary key). A Data-Mapper convenience - still no per-row method. (A new
 * row with an explicitly-assigned primary key should use `orm.insert`.)
 * @param session {Session} the session (`orm.session` or `orm.transaction`)
 * @param s {Schema} the schema
 * @param record {map of string to string} the row
 * @return {sql.Result} the affected-rows result
 */
export func save(session as Session, s as Schema, record as map of string to string) {
    if (maps.has($record, $s.primaryKey)) {
        return update($session, $s, $record);
    }
    return insert($session, $s, $record);
}

# ---- finders ----

/**
 * The first row a query matches (adds `LIMIT 1`), or an **empty map** when there
 * is none.
 * @param session {Session} the session (`orm.session` or `orm.transaction`)
 * @param q {Query} the query
 * @return {map of string to string} the first row, or `{}` if none
 */
export func first(session as Session, q as Query) {
    def matched as list of map of string to string init all($session, limit($q, 1));
    if (len($matched) > 0) {
        return $matched[0];
    }
    def empty as map of string to string init {};
    return $empty;
}

/**
 * Whether a query matches any row (adds `LIMIT 1`).
 * @param session {Session} the session (`orm.session` or `orm.transaction`)
 * @param q {Query} the query
 * @return {bool} true if at least one row matches
 */
export func exists(session as Session, q as Query) {
    return len(all($session, limit($q, 1))) > 0;
}

/**
 * The first row of the schema's table where `col = value`, or `{}` if none.
 * @param session {Session} the session (`orm.session` or `orm.transaction`)
 * @param s {Schema} the schema
 * @param col {string} the column to match
 * @param value {string} the value to match
 * @return {map of string to string} the matching row, or `{}` if none
 */
export func findBy(session as Session, s as Schema, col as string, value as string) {
    return first($session, where(from($s), $col, "=", $value));
}

/**
 * The values of one column across every row a query matches (the column must be
 * in the projection; the default `SELECT *` includes it).
 * @param session {Session} the session (`orm.session` or `orm.transaction`)
 * @param q {Query} the query
 * @param col {string} the column to pluck
 * @return {list of string} the column's value for each matching row
 */
export func pluck(session as Session, q as Query, col as string) {
    def matched as list of map of string to string init all($session, $q);
    def out as list of string init [];
    for (def r in $matched) {
        if (not maps.has($r, $col)) {
            fail("pluck: column \"" + $col + "\" is not in the result (add it to the projection)");
        }
        $out[] = $r[$col];
    }
    return $out;
}
