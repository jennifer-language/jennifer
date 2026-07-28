# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

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
 * One column in a schema: its name and value kind (informational for the
 * string-row form; the guide for a future typed form).
 * @field name {string} the column name
 * @field kind {ColumnKind} the value kind
 */
export def struct Column {
    name as string,
    kind as ColumnKind
};

/**
 * A table mapping: the table name, its columns, the primary-key column, and the
 * SQL dialect. Value-semantic; `column` returns a fresh schema.
 * @field table {string} the table name
 * @field columns {list of Column} the columns
 * @field primaryKey {string} the primary-key column name
 * @field dialect {Dialect} the SQL dialect (placeholder + DDL spelling)
 */
export def struct Schema {
    table as string,
    columns as list of Column,
    primaryKey as string,
    dialect as Dialect
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
    offsetN as int
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
# IN / NOT IN (which `whereIn` adds). Used by validateQuery so a hand-built
# Condition literal is still checked.
func checkRenderedOp(op as string) {
    def norm as string init strings.upper(strings.trim($op));
    match ($norm) {
        when "=", "!=", "<>", "<", ">", "<=", ">=", "LIKE", "NOT LIKE", "IN", "NOT IN" {
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
    return Schema{table: $table, columns: [], primaryKey: $primaryKey, dialect: $dialect};
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
    $cols[] = Column{name: $name, kind: $kind};
    $out.columns = $cols;
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
        offsetN: 0
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
    def stmt as string init "SELECT " + renderSelects($q) + " FROM " + $q.table;
    for (def j in $q.joins) {
        $stmt = $stmt + " " + $j.kind + " JOIN " + $j.table + " ON " + $j.leftCol + " = " +
            $j.rightCol;
    }
    def n as int init 1;
    if (len($q.wheres) > 0) {
        $stmt = $stmt + " WHERE ";
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
            } else {
                $stmt = $stmt + $c.column + " " + $c.op + " " + ph($q.dialect, $n);
                $n = $n + 1;
            }
        }
    }
    if (len($q.groups) > 0) {
        $stmt = $stmt + " GROUP BY " + strings.join($q.groups, ", ");
    }
    if (len($q.havings) > 0) {
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

/**
 * The CREATE TABLE DDL for a schema, in its dialect. A convenience DDL emitter -
 * a schema-migration tool is out of scope.
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
        $cols = $cols + $s.columns[$i].name + " " + sqlType($s.columns[$i].kind, $s.dialect);
    }
    return "CREATE TABLE " + $s.table + " (" + $cols + ", PRIMARY KEY (" + $s.primaryKey + "))";
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
        throw Error{
            kind: "orm",
            message: "orm.find: no " + $s.table + " with " + $s.primaryKey + " = " + $id,
            file: "",
            line: 0,
            col: 0
        };
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
