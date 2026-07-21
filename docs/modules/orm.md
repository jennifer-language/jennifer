# `orm` - a relational mapper over `sql`

Import with `import "orm.j" as orm;`. A minimal relational mapper over the
[`sql`](../libraries/sql.md) library. It is **Data Mapper, not Active Record**:
Jennifer structs are value-semantic and carry no methods, and a module holds no
state, so a row cannot `save()` itself. You pass a record and a `Schema` to
repository functions - `orm.insert` / `find` / `update` / `delete`, and
`orm.all` over a query.

There is no reflection, so you declare the table mapping once as an `orm.Schema`,
which also carries the SQL **dialect** (`"mysql"` or `"postgres"`) - a backend
selector on one module, not parallel modules.

```jennifer
import "orm.j" as orm;

def users as orm.Schema init orm.column(orm.column(
    orm.schema("users", "id", "postgres"), "id", "int"), "name", "string");

def ada as map of string to string init {};
$ada["id"] = "1";
$ada["name"] = "ada";
orm.insert($conn, $users, $ada);                       # INSERT INTO users ...
def row as map of string to string init orm.find($conn, $users, "1");
```

Runnable: [`examples/modules/orm_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/orm_demo.j).

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
| `orm.schema(table, primaryKey, dialect)` | `Schema` | Start a schema; `dialect` is `"mysql"` or `"postgres"`. |
| `orm.column(schema, name, kind)` | `Schema` | Append a column; `kind` is `int` / `string` / `float` / `bool` / `bytes`. |
| `orm.createTable(schema)` | `string` | The `CREATE TABLE` DDL for the dialect (a convenience emitter, not a migration tool). |

## The query builder (functional, pure)

Like the [`json`](../libraries/json.md) write surface, each step returns a fresh
`orm.Query` - no method chaining (values have no methods):

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `orm.from(schema)` | `Query` | Select all rows of the schema's table. |
| `orm.where(query, col, op, value)` | `Query` | Add a `col op value` condition (AND-joined); `value` binds as a parameter. |
| `orm.orderBy(query, col, dir)` | `Query` | Add an `ORDER BY` term (`"asc"` / `"desc"`). |
| `orm.limit(query, n)` / `orm.offset(query, n)` | `Query` | Add `LIMIT` / `OFFSET`. |
| `orm.join(query, table, leftCol, rightCol)` | `Query` | Add an `INNER JOIN`. |
| `orm.toSql(query)` | `Rendered` | Render to parameterized SQL: `Rendered{sql, params}`. |

`toSql` spells placeholders per dialect - `?` for MySQL, `$1` / `$2` … for
Postgres - and **values only ever reach the query through those placeholders**,
so the injection safety is inherited from `sql`. The whole builder is pure, so it
is fully testable without a database.

```jennifer
def q as orm.Query init orm.limit(
    orm.where(orm.from($users), "age", ">=", "18"), 25);
def r as orm.Rendered init orm.toSql($q);
# r.sql    = SELECT * FROM users WHERE age >= $1 LIMIT 25
# r.params = ["18"]
```

## CRUD

Each takes a `sql.Connection` (or a `sql.Tx`), the schema, and a record or key:

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `orm.insert(conn, schema, record)` | `sql.Result` | Insert the record's present columns. |
| `orm.find(conn, schema, id)` | `map of string to string` | The row with that primary-key value (throws if none). |
| `orm.update(conn, schema, record)` | `sql.Result` | Update the non-key columns, matched by the record's primary key. |
| `orm.delete(conn, schema, id)` | `sql.Result` | Delete by primary-key value. |
| `orm.all(conn, query)` | `list of map of string to string` | Every row matching a built query. |

Transactions come straight from `sql`: pass a `sql.Tx` (from `sql.begin`) as the
`conn`, and `sql.commit` / `rollback` it.

```jennifer
def tx as sql.Tx init sql.begin($conn);
errdefer sql.rollback($tx);
orm.insert($tx, $users, $ada);
orm.insert($tx, $users, $bob);
sql.commit($tx);
```

## Scope

First release: the repository CRUD and the query builder above. **Out of v1**:
relations beyond a plain `join` (has-many / belongs-to eager loading wants object
identity Jennifer does not have), full schema migrations, and a typed-struct row
form (gated on map-to-struct conversion). Needs `sql`, so the **default
`jennifer`** binary.
