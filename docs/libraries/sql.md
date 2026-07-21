# sql

A relational-database client over Go's `database/sql`, shipping the two
client-server engines: **MySQL / MariaDB** and **PostgreSQL** (both pure-Go
drivers). SQLite (the one embedded engine) is deliberately not here. **Default
`jennifer` binary only** - `jennifer-tiny` returns a friendly error (the drivers
are not compiled there, and it has no network stack anyway).

Values bind **only through placeholders**. String interpolation (the
SQL-injection vector) is never how a value reaches a query. The placeholder
spelling is the engine's own - `?` for MySQL / MariaDB, `$1`..`$n` for
Postgres; the SQL text goes to the driver verbatim, so write the dialect you
are connecting to.

```jennifer
use sql;
use io;

def db as sql.Connection init sql.open("postgres", "postgres://user:pw@host/app");
defer sql.close($db);

def result as sql.Result init sql.exec($db, "insert into t(v) values ($1)", "hello");
io.printf("inserted, %d row(s)\n", $result.affected);

def rows as sql.Rows init sql.query($db, "select id, name from t where id > $1", 0);
while (sql.next($rows)) {
    io.printf("%d %s\n", sql.asInt($rows, "id"), sql.asString($rows, "name"));
}
```

## Connections

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `sql.open(driver, dsn)` | `sql.Connection` | `driver`: `"mysql"` / `"mariadb"` or `"postgres"` / `"postgresql"`. Pings, so a bad DSN / unreachable server errors here. |
| `sql.close(conn)` | `null` | Closes the connection pool. |

The DSN format is the driver's:

- MySQL / MariaDB: `"user:pw@tcp(host:3306)/dbname"`
- Postgres: `"postgres://user:pw@host:5432/dbname"` (`?sslmode=disable` for a
  plaintext local server)

## Query and exec

`query` / `exec` take a **Connection or a Tx** as the first argument, the SQL
next, then the placeholder values.

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `sql.query(target, sql, params...)` | `sql.Rows` | A row cursor. |
| `sql.exec(target, sql, params...)` | `sql.Result` | `sql.Result{affected, lastId}` - read the fields. A field the driver cannot report is `-1` (`lastId` on Postgres, e.g.; `affected` on the rare driver without row counts). |

Parameters bind by type: `int` / `float` / `string` / `bool` / `bytes` / `null`.
A struct is a positioned error - build the value first.

A **single `list` argument is spread** into the parameter sequence:
`sql.exec($conn, $sql, $params)` binds each element of `$params` in turn, so a
query whose parameter count is only known at runtime (the pattern the
[`orm`](../modules/orm.md) module builds on) works without a fixed call-site
arity. The plain variadic form (`sql.exec($conn, $sql, a, b, c)`) is unchanged;
a list mixed with other arguments, or a list element that is itself a list, is
still a placeholder-binding error.

## Reading rows

The cursor is pull-based; `next` advances and scans, then the typed accessors read
a column of the current row by **name (string) or 0-based index (int)**.

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `sql.next(rows)` | `bool` | Advance to the next row; `false` at the end, which closes the cursor and releases the handle (a later `sql.next` on it is an error). |
| `sql.columns(rows)` | `list of string` | Column names. |
| `sql.asInt(rows, col)` / `.asFloat` / `.asString` / `.asBool` / `.asBytes` | typed | The current row's column, coerced. A `NULL` column is an error (check `sql.isNull` first); `asString` also stringifies a datetime (RFC 3339). |
| `sql.isNull(rows, col)` | `bool` | Whether the column is SQL `NULL`. |
| `sql.closeRows(rows)` | `null` | Close a cursor early (before exhaustion). A no-op on an already-released cursor, so `defer sql.closeRows($rows);` is always safe - whether or not the loop ran to the end. |

## Transactions

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `sql.begin(conn)` | `sql.Tx` | Pass the `Tx` as the `target` of `query` / `exec`. |
| `sql.commit(tx)` / `sql.rollback(tx)` | `null` | Ends the transaction; the handle is then unusable. `errdefer sql.rollback($tx);` pairs well with a `commit` on the success path. |

## Prepared statements

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `sql.prepare(conn, sql)` | `sql.Statement` | Prepare once, run many. |
| `sql.queryStmt(stmt, params...)` | `sql.Rows` | |
| `sql.execStmt(stmt, params...)` | `sql.Result` | |
| `sql.closeStmt(stmt)` | `null` | |

Blocking; compose with `spawn`. Handles use the integer-registry pattern - close
them (a `defer` right after acquisition is the idiom).

## See also

[net](net.md), [fs](fs.md), [json](json.md), [toml](toml.md).
