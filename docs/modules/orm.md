# `orm` - a relational mapper over `sql`

Import with `import "orm.j" as orm;`. A minimal relational mapper over the
[`sql`](../libraries/sql.md) library. It is **Data Mapper, not Active Record**:
Jennifer structs are value-semantic and carry no methods, and a module holds no
state, so a row cannot `save()` itself. You pass a record and a `Schema` to
repository functions - `orm.insert` / `find` / `update` / `delete`, and
`orm.all` over a query.

There is no reflection, so you declare the table mapping once as an `orm.Schema`,
which also carries the SQL **dialect** (`orm.Dialect.Mysql` or
`orm.Dialect.Postgres`) - a backend selector on one module, not parallel modules.
The dialect and each column's kind are closed **enums** (`orm.Dialect`,
`orm.ColumnKind`), so an invalid value is a compile-time error, not a runtime one.

Every persistence / query-executing function takes an `orm.Session` (from
`orm.session(conn)` or `orm.transaction(tx)`) as its first argument, so the same
call runs standalone or inside a transaction.

```jennifer
import "orm.j" as orm;

def users as orm.Schema init orm.column(orm.column(
    orm.schema("users", "id", orm.Dialect.Postgres), "id", orm.ColumnKind.Int), "name", orm.ColumnKind.String);

def sess as orm.Session init orm.session($conn);       # auto-committing session
def ada as map of string to string init {};
$ada["id"] = "1";
$ada["name"] = "ada";
orm.insert($sess, $users, $ada);                       # INSERT INTO users ...
def row as map of string to string init orm.find($sess, $users, "1");
```

Runnable: [`examples/modules/orm_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/orm_demo.j).
Schema migrations live in the separate
[`sqlmigrate`](sqlmigrate.md) module (its `up` / `down` are DDL strings built by
the DDL helpers below).

## Records are `map of string to string`

Both the input to `insert` / `update` and the result of `find` / `all` are a
`map of string to string` keyed by column name. This is the row form that needs
no map-to-struct conversion; a typed-struct form waits on that language feature.
The database coerces the string values to the column types, and only the columns
**present** in the record are written (so an auto-generated key is simply
omitted).

## The schema

Declared once with `orm.schema` + `orm.column` (value-semantic - each `column`
returns a fresh schema):

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `orm.schema(table, primaryKey, dialect)` | `Schema` | Start a schema; `dialect` is `orm.Dialect.Mysql` or `orm.Dialect.Postgres`. |
| `orm.column(schema, name, kind)` | `Schema` | Append a column; `kind` is `orm.ColumnKind.Int` / `String` / `Float` / `Bool` / `Bytes`. |
| `orm.createTable(schema)` | `string` | The `CREATE TABLE` DDL for the dialect, including each column's attributes and the primary key. |

### Column attributes

A column is **nullable by default** (SQL's own default). The fluent setters
decorate the **most-recently-added** column and return a fresh `Schema`, so they
chain onto `orm.column`:

| Call | Effect on the last column |
| ---- | ------------------------- |
| `orm.notNull(schema)` | `NOT NULL` |
| `orm.unique(schema)` | `UNIQUE` |
| `orm.autoIncrement(schema)` | `SERIAL` (Postgres) / `AUTO_INCREMENT` (MySQL) |
| `orm.withDefault(schema, value)` | `DEFAULT value` (numeric / bool bare, string quoted+escaped) |

A `DEFAULT` value is rendered by the column kind and can never inject DDL: an
`Int` / `Float` default is validated numeric; a `Bool` default accepts
`true`/`false`/`1`/`0`; a `String` / `Bytes` default is a quoted, escaped SQL
literal (a backslash or control character is rejected).

```jennifer
def accounts as orm.Schema init orm.schema("accounts", "id", orm.Dialect.Postgres);
$accounts = orm.autoIncrement(orm.column($accounts, "id", orm.ColumnKind.Int));
$accounts = orm.notNull(orm.unique(orm.column($accounts, "email", orm.ColumnKind.String)));
$accounts = orm.withDefault(orm.column($accounts, "active", orm.ColumnKind.Bool), "true");
# CREATE TABLE accounts (id SERIAL, email TEXT NOT NULL UNIQUE, active BOOLEAN DEFAULT TRUE, PRIMARY KEY (id))
```

### DDL builders

Standalone DDL emitters, each returning one statement string. Feed them to the
[`sqlmigrate`](sqlmigrate.md) runner's `up` / `down`, or run them directly.
Identifiers are allowlist-checked like the query builder.

| Call | Statement |
| ---- | --------- |
| `orm.dropTable(table)` | `DROP TABLE table` |
| `orm.addColumn(table, name, kind, dialect)` | `ALTER TABLE ... ADD COLUMN name type` |
| `orm.dropColumn(table, name)` | `ALTER TABLE ... DROP COLUMN name` |
| `orm.renameColumn(table, from, to)` | `ALTER TABLE ... RENAME COLUMN from TO to` |
| `orm.createIndex(name, table, columns, isUnique)` | `CREATE [UNIQUE] INDEX name ON table (...)` |
| `orm.dropIndex(name, table, dialect)` | `DROP INDEX name [ON table]` (dialect-spelled) |
| `orm.addForeignKey(table, name, column, refTable, refColumn)` | `ALTER TABLE ... ADD CONSTRAINT ... FOREIGN KEY ...` |
| `orm.dropForeignKey(table, name, dialect)` | `ALTER TABLE ... DROP CONSTRAINT/FOREIGN KEY name` |

## The query builder (functional, pure)

Like the [`json`](../libraries/json.md) write surface, each step returns a fresh
`orm.Query` - no method chaining (values have no methods):

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `orm.from(schema)` | `Query` | Select all rows of the schema's table. |
| `orm.select(query, cols)` | `Query` | Project only `cols` (a `list of string`) instead of `SELECT *`. |
| `orm.count(query, alias)` | `Query` | Add `COUNT(*) AS alias` to the projection. |
| `orm.aggregate(query, fn, col, alias)` | `Query` | Add `fn(col) AS alias`; `fn` is `COUNT` / `SUM` / `AVG` / `MIN` / `MAX`. |
| `orm.where(query, col, op, value)` | `Query` | A `col op value` condition, AND-joined; `value` binds as a parameter. |
| `orm.orWhere(query, col, op, value)` | `Query` | Like `where`, but OR-joined to the previous condition. |
| `orm.whereIn(query, col, values)` / `orm.orWhereIn` / `orm.whereNotIn` | `Query` | `col IN (...)` / `NOT IN`, one placeholder bound per value (non-empty). |
| `orm.whereNull(query, col)` / `orm.whereNotNull(query, col)` | `Query` | `col IS NULL` / `IS NOT NULL` (no bound value). |
| `orm.whereBetween(query, col, lo, hi)` | `Query` | `col BETWEEN lo AND hi` (both bounds bound). |
| `orm.distinct(query)` | `Query` | Render `SELECT DISTINCT`. |
| `orm.page(query, pageNum, pageSize)` | `Query` | 1-based `LIMIT` / `OFFSET` sugar (page 1 starts at offset 0). |
| `orm.join` / `orm.leftJoin` / `orm.rightJoin` `(query, table, leftCol, rightCol)` | `Query` | An `INNER` / `LEFT` / `RIGHT JOIN`. |
| `orm.groupBy(query, cols)` | `Query` | Add a `GROUP BY`. |
| `orm.having(query, fn, col, op, value)` / `orm.orHaving` | `Query` | A `HAVING fn(col) op value` condition over an aggregate. |
| `orm.orderBy(query, col, dir)` | `Query` | Add an `ORDER BY` term (`"asc"` / `"desc"`). |
| `orm.limit(query, n)` / `orm.offset(query, n)` | `Query` | Add `LIMIT` / `OFFSET`. |
| `orm.toSql(query)` | `Rendered` | Render to parameterized SQL: `Rendered{sql, params}`. |

`toSql` spells placeholders per dialect - `?` for MySQL, `$1` / `$2` … for
Postgres - and **values only ever reach the query through those placeholders**,
so the injection safety is inherited from `sql`. The whole builder is pure, so it
is fully testable without a database.

Note the SQL precedence of `orWhere`: SQL binds `AND` tighter than `OR`, so
`where(...)` then `orWhere(...)` reads `a AND b OR c` as `(a AND b) OR c`.

```jennifer
# projection + aggregate + GROUP BY + HAVING
def q as orm.Query init orm.having(
    orm.groupBy(orm.count(orm.select(orm.from($users), ["age"]), "n"), ["age"]),
    "COUNT", "*", ">", "5");
def r as orm.Rendered init orm.toSql($q);
# r.sql    = SELECT age, COUNT(*) AS n FROM users GROUP BY age HAVING COUNT(*) > $1
# r.params = ["5"]

# OR + IN
def q2 as orm.Query init orm.whereIn(
    orm.orWhere(orm.where(orm.from($users), "age", ">=", "18"), "name", "=", "ada"),
    "id", ["1", "2", "3"]);
# SELECT * FROM users WHERE age >= $1 OR name = $2 AND id IN ($3, $4, $5)
```

### Render-time validation

Identifiers, operators, aggregate functions, join kinds, and sort directions are
validated against fixed allowlists **twice**: once in the builder (an early,
friendly error) and again inside `toSql` / `createTable` / the CRUD builders. So
even a hand-built `orm.Query{...}` or `orm.Schema{...}` struct literal that
skipped the builder is re-checked before it renders - an injected column name
throws `Error{kind: "orm"}` at render time rather than reaching the database.

## Relations

Declare associations once on the schema; they are metadata (a `list of Relation`
on the `Schema`) that `joinRelation` turns into the right `JOIN`. Each builder
returns a fresh schema, so they chain like `column`:

| Call | Kind | Foreign key is on ... |
| ---- | ---- | --------------------- |
| `orm.belongsTo(schema, name, target, foreignKey)` | belongs-to | **this** table (references the target's `id`) |
| `orm.hasOne(schema, name, target, foreignKey)` | has-one | the **target** table (references this PK) |
| `orm.hasMany(schema, name, target, foreignKey)` | has-many | the **target** table (references this PK) |
| `orm.manyToMany(schema, name, target, joinTable, localFk, targetFk)` | many-to-many | a **join table** linking the two |

`orm.joinRelation(query, schema, relationName)` emits the `INNER JOIN` a relation
implies (two joins for a many-to-many, through its join table), built over the
`join` primitive - so the key columns are always correct and identifier-checked:

```jennifer
def authors as orm.Schema init orm.hasMany(
    orm.schema("authors", "id", orm.Dialect.Postgres), "posts", "posts", "authorId");
orm.toSql(orm.joinRelation(orm.from($authors), $authors, "posts")).sql;
# SELECT * FROM authors INNER JOIN posts ON posts.authorId = authors.id
```

A `belongsTo` and a `manyToMany` reference the target's primary key by the `id`
convention.

## Eager loading (N+1 elimination)

Mark a declared relation with `orm.with`, then `orm.load` fetches the base rows
and their relations in a **fixed 1 + R queries** (R = relations requested): the
base query once, then **one** batched `WHERE fk IN (...)` per relation - never one
query per row.

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `orm.with(query, relationName)` | `Query` | Mark a relation to eager-load (ignored by `toSql`; consumed by `load`). |
| `orm.load(session, schema, query)` | `Result` | Run the base query + one batched query per marked relation. |
| `orm.rows(result)` | `list of map of string to string` | The base rows. |
| `orm.related(result, row, name)` | `list of map of string to string` | A row's children for a has-many / many-to-many (empty list if none). |
| `orm.relatedOne(result, row, name)` | `map of string to string` | A row's single related row for a belongs-to / has-one, or `{}` if none. |

```jennifer
def r as orm.Result init orm.load($sess, $authors, orm.with(orm.from($authors), "posts"));
for (def author in orm.rows($r)) {
    def posts as list of map of string to string init orm.related($r, $author, "posts");
    # 2 queries total (authors + posts WHERE authorId IN (...)), regardless of author count
}
```

`related`/`relatedOne` are pure lookups into the `Result` (no query). One level of
nesting is supported; relations-of-relations is a follow-on. Eager loading needs
the relation's key column present in the base projection (the default `SELECT *`
includes it). A very large parent set is handled by splitting each relation's
`WHERE fk IN (...)` batch so the placeholder count never exceeds the
per-statement limit (so the query count is `1 + R` for the usual case, growing
only when a key set exceeds the limit - never an over-limit statement). For a
many-to-many, a child row keeps the joined join-table columns (so pivot payload
survives); if the target and join table share a column name, that name collapses
in the row map (join tables conventionally use distinct FK names, so this is
rare).

## Sessions and CRUD

CRUD runs through an `orm.Session`, the unit of work that wraps either a
connection (auto-committing) or a transaction:

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `orm.session(conn)` | `Session` | An auto-committing session over a `sql.Connection`. |
| `orm.transaction(tx)` | `Session` | A session over a `sql.Tx`, so CRUD runs inside a caller's transaction. |

Each repository function takes a `Session` first, then the schema and a record or
key:

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `orm.insert(session, schema, record)` | `sql.Result` | Insert the record's present columns. |
| `orm.find(session, schema, id)` | `map of string to string` | The row with that primary-key value (throws if none). |
| `orm.update(session, schema, record)` | `sql.Result` | Update the non-key columns, matched by the record's primary key. |
| `orm.delete(session, schema, id)` | `sql.Result` | Delete by primary-key value. |
| `orm.all(session, query)` | `list of map of string to string` | Every row matching a built query. |

Transactions come straight from `sql` - `sql.begin` / `commit` / `rollback` - and
`orm.transaction` bridges the `Tx` into a `Session`, so the same CRUD calls run
inside the transaction:

```jennifer
def sess as orm.Session init orm.session($conn);   # auto-commit
orm.insert($sess, $users, $ada);

def tx as sql.Tx init sql.begin($conn);            # or a transaction
errdefer sql.rollback($tx);
def txn as orm.Session init orm.transaction($tx);
orm.insert($txn, $users, $bob);
sql.commit($tx);
```

## Bulk and conditional writes

Beyond single-row CRUD, the write path covers upsert, batch insert, generated
keys, and bulk conditional mutations - all through a `Session`:

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `orm.upsert(session, schema, record, conflictCols)` | `sql.Result` | Insert, or update the non-conflict columns on a unique/PK conflict (Postgres `ON CONFLICT`, MySQL `ON DUPLICATE KEY`). |
| `orm.insertMany(session, schema, records)` | `int` | One multi-row `INSERT` (records must set the same columns); returns the count inserted. |
| `orm.insertReturning(session, schema, record)` | `string` | Insert and return the generated primary-key value. |
| `orm.updateWhere(session, schema, assignments, query)` | `sql.Result` | Bulk-update the rows matching the query's `WHERE`. |
| `orm.deleteWhere(session, schema, query)` | `sql.Result` | Bulk-delete the rows matching the query's `WHERE`. |
| `orm.save(session, schema, record)` | `sql.Result` | Insert when the record has no primary key, else update. |

`updateWhere` and `deleteWhere` **refuse a query with no `WHERE`**, so a missing
condition can never update or delete every row (use a raw `sql.exec` to truncate
deliberately). `insertMany` never silently caps: a batch whose placeholder count
would exceed the per-statement limit (60000) is split across several `INSERT`s -
wrap it in `orm.transaction` for all-or-nothing. Every value binds through a
placeholder.

```jennifer
orm.upsert($sess, $users, $ada, ["email"]);          # insert, or update on an email conflict
def id as string init orm.insertReturning($sess, $users, $bob);
def bump as map of string to string init {};
$bump["active"] = "false";
orm.updateWhere($sess, $users, $bump, orm.where(orm.from($users), "age", "<", "13"));
```

## Finders

Everyday read helpers over a `Session` and a query:

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `orm.first(session, query)` | `map of string to string` | The first matching row (adds `LIMIT 1`), or `{}` if none. |
| `orm.exists(session, query)` | `bool` | Whether any row matches. |
| `orm.findBy(session, schema, col, value)` | `map of string to string` | The first row where `col = value`, or `{}`. |
| `orm.pluck(session, query, col)` | `list of string` | One column's value for each matching row. |

```jennifer
def ada as map of string to string init orm.findBy($sess, $users, "name", "ada");
def anyAdmins as bool init orm.exists($sess, orm.where(orm.from($users), "role", "=", "admin"));
def names as list of string init orm.pluck($sess, orm.from($users), "name");
```

A raw-SQL `whereRaw` escape hatch was **rejected** - it is the one thing that
could not keep orm's "no injection even from a hand-built query" guarantee (an
arbitrary SQL fragment can't be allowlist-checked); genuinely bespoke SQL belongs
in the [`sql`](../libraries/sql.md) library. See
[rejected.md](../technical/rejected.md).

## Scope

The repository CRUD, the write path, finders, the query builder, column-attribute
DDL, and the standalone DDL builders above. Schema migrations are a separate concern in the
[`sqlmigrate`](sqlmigrate.md) module (which consumes the DDL builders' strings).
**Out of scope here**: relations beyond a plain `join` (has-many / belongs-to
eager loading wants object identity Jennifer does not have) and a typed-struct row
form (gated on map-to-struct conversion). Needs `sql`, so the **default
`jennifer`** binary.
