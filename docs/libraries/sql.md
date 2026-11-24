# sql

A relational-database client over Go's `database/sql`, shipping the two
client-server engines: **MySQL/MariaDB/Galera** and **PostgreSQL** (both pure-Go
drivers). SQLite (embedded) is not included. **Default `jennifer` binary
only**: `jennifer-tiny` raises an error (the drivers are not compiled there,
and it has no network stack).

Values bind **only through placeholders**, never by string interpolation (the
SQL-injection vector). The placeholder spelling is the engine's own: `?` for
MySQL/MariaDB/Galera, `$1`..`$n` for Postgres; the SQL text goes to the driver
verbatim, so write the dialect of the server.

```jennifer
use sql;
use io;

def db as sql.Connection init sql.open("postgres", "postgres://user:pw@db.example.com/app");
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
| `sql.open(driver, dsn)` | `sql.Connection` | `sql.openWith(driver, dsn, sql.Options{})`. |
| `sql.openWith(driver, dsn, options)` | `sql.Connection` | `driver`: `"mysql"` (MySQL, MariaDB, Galera Cluster) or `"postgres"`. Applies the TLS rules below, then pings, so a bad DSN or an unreachable server fails here. The ping is bounded by the query timeout. |
| `sql.close(conn)` | `null` | Closes the connection pool and the statements prepared on it. A second `close` does nothing. |

| `sql.Options` field | Type | Default (zero value) |
| --- | --- | --- |
| `allowInsecure` | `bool` | `false`: a DSN that weakens TLS is refused. `true` accepts it |
| `queryTimeoutMs` | `int` | `0`: the 30 s default. The connection's client query deadline (below); `math.MAX_INT` is none, a negative value throws `"runtime"` |

The DSN format is the driver's:

| Driver | TCP | Unix socket |
| --- | --- | --- |
| `"mysql"` | `"user:pw@tcp(db.example.com:3306)/dbname?tls=true"` | `"user:pw@unix(/run/mysqld/mysqld.sock)/dbname"` |
| `"postgres"` | `"postgres://user:pw@db.example.com:5432/dbname?sslmode=verify-full"` or `"host=db.example.com user=u password=pw dbname=d sslmode=verify-full"` | `"host=/run/postgresql user=u dbname=d"` |

### TLS

A connection to another host uses verified TLS: the server's certificate
must chain to a system CA and name the host.

| DSN names | MySQL (`tls=`) | Postgres (`sslmode=`) |
| --- | --- | --- |
| nothing | `tls=true` is added | `sslmode=verify-full` is added |
| a verifying mode | `true` | `verify-full`; `verify-ca` with `sslrootcert` (a CA file, or `system`) |
| any other mode | `false`, `skip-verify`, `preferred`: refused unless `allowInsecure` | `disable`, `allow`, `prefer`, `require`, an empty `sslmode=`, `verify-ca` without `sslrootcert`: refused unless `allowInsecure` |
| a plaintext fallback | `allowFallbackToPlaintext=true`: refused unless `allowInsecure` | (part of `allow` / `prefer`) |

`verify-ca` checks the certificate chain but not the host name, so it is
accepted only against a pinned CA: without `sslrootcert` (or
`PGSSLROOTCERT`) any certificate a public CA issued would pass.

Exempt, with the DSN used as written: a Unix socket and a loopback host
(`localhost`, 127.0.0.0/8, `::1`), where the traffic never leaves the host.
`localhost` is not resolved: it dials 127.0.0.1, then `::1`, so neither a
missing hosts entry nor a DNS answer can send the exempt login elsewhere.
A container's service name (`db:5432`) is another host. Where the DSN names
no host the driver's default applies (MySQL `127.0.0.1:3306`, Postgres
`PGHOST` or the local socket). A refusal is `"runtime"` and names the fix:

```text
sql.open: sslmode=disable sends the password and the data in plaintext (host db.example.com); use sslmode=verify-full, or sql.openWith(driver, dsn, sql.Options{allowInsecure: true}) to accept that
```

No error carries the DSN's password: a driver's text that quotes the DSN is
redacted before it is raised.

## Query and exec

`query` / `exec` take a **Connection or a Tx** as the first argument, the SQL
next, then the placeholder values.

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `sql.query(target, sql, params...)` | `sql.Rows` | A row cursor. |
| `sql.exec(target, sql, params...)` | `sql.Result` | `sql.Result{affected, lastId}` - read the fields. A field the driver cannot report is `-1` (`lastId` on Postgres, e.g.; `affected` on the rare driver without row counts). |

Parameters bind by type: `int` / `float` / `string` / `bool` / `bytes` / `null`.
A struct is a positioned error - build the value first.

Every `query` / `exec` (and the prepared-statement variants) runs under a
**client-side deadline** (default **30 s**) that also bounds the pool-connection
acquisition it may block on, so a leaked cursor pinning a pool connection cannot
make a later call block forever. The same deadline governs the cursor's
`sql.next` reads, so a long streaming loop over a big table can hit it
mid-iteration. `sql.open`'s ping, and the connection acquisition of `sql.begin`
and `sql.prepare`, are bounded by the same timeout; a transaction itself has no
deadline. The deadline is per connection, `sql.Options.queryTimeoutMs`; for a
long batch read open the connection with a larger value or `math.MAX_INT`
(none). When it passes the call throws `"timeout"`.

Each handle registry (connections, cursors, statements, transactions) is
bounded (4096 each), so a handle leaked in a loop raises a catchable "too many
open" error of kind `"limit"`; pair `open` / `query` / `prepare` / `begin` with
their close call (a `defer` is the idiom).

A **single `list` argument is spread** into the parameter sequence:
`sql.exec($conn, $sql, $params)` binds each element of `$params` in turn, so a
query whose parameter count is only known at runtime (the pattern the
[`orm`](../modules/orm.md) module builds on) works without a fixed call-site
arity. The plain variadic form (`sql.exec($conn, $sql, a, b, c)`) still works;
a list mixed with other arguments, or a list element that is itself a list, is
a placeholder-binding error.

## Reading rows

The cursor is pull-based; `next` advances and scans, then the typed accessors read
a column of the current row by **name (string) or 0-based index (int)**.

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `sql.next(rows)` | `bool` | Advance to the next row; `false` at the end, which closes the cursor and releases the handle (a later `sql.next` on it throws `"closed"`). |
| `sql.columns(rows)` | `list of string` | Column names. |
| `sql.asInt(rows, col)` / `.asFloat` / `.asString` / `.asBool` / `.asBytes` | typed | The current row's column, coerced. A `NULL` column is an error (check `sql.isNull` first). `asInt` accepts MySQL `BIGINT UNSIGNED` up to the int range and a float only when it is a whole number in range. |
| `sql.isNull(rows, col)` | `bool` | Whether the column is SQL `NULL`. |
| `sql.closeRows(rows)` | `null` | Close a cursor early (before exhaustion). A no-op on an already-released cursor, so `defer sql.closeRows($rows);` is always safe. |

| Accessor | Accepts | Throws `"sql"` for |
| --- | --- | --- |
| `asString` | text that is valid UTF-8; numbers, bools and datetimes (RFC 3339) as text | text that is not UTF-8, naming the column and the byte offset; read it with `asBytes` |
| `asFloat` | a finite number | NaN, `Infinity`, `-Infinity` |
| `asBool` | a bool, or an integer column holding 0 or 1 (MySQL `BOOL` is `TINYINT(1)`) | any other integer |

## Transactions

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `sql.begin(conn)` | `sql.Tx` | Pass the `Tx` as the `target` of `query` / `exec`. |
| `sql.commit(tx)` / `sql.rollback(tx)` | `null` | Ends the transaction; a later `query` / `exec` / `commit` on the handle throws `"closed"`. `rollback` of an ended transaction does nothing, so `defer sql.rollback($tx);` pairs with a `commit` on the success path. |

## Prepared statements

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `sql.prepare(conn, sql)` | `sql.Statement` | Prepare once, run many. |
| `sql.queryStmt(stmt, params...)` | `sql.Rows` | |
| `sql.execStmt(stmt, params...)` | `sql.Result` | |
| `sql.closeStmt(stmt)` | `null` | A second `closeStmt` does nothing. |

Blocking; compose with `spawn`. Handles use the integer-registry pattern - close
them (a `defer` right after acquisition is the idiom).

## Error handling

### Errors

| Kind | Raised for |
|---|---|
| `"sql"` | always a `sql.SqlError`: an error the server reports (a constraint, a syntax error, a deadlock, a refused login), and column data an accessor cannot return (above) |
| `"timeout"` | the client query deadline passed (`queryTimeoutMs`): in a query, an exec, a `sql.next` read, the ping of `open`, or waiting for a pool connection |
| `"net"` | the network failed (refused, reset, unreachable, DNS, a certificate that does not verify); always a [`net.NetError`](net.md#errors) |
| `"limit"` | 4096 open connections, cursors, statements or transactions (each counted on its own) |
| `"closed"` | a closed connection, statement or cursor (exhausted or `closeRows`), a statement whose connection is closed, a committed or rolled-back transaction (`sql.query: sql.Connection id 1 is closed`); `close`, `closeRows`, `closeStmt` and `rollback` on such a handle do nothing |
| `"runtime"` | misuse: a DSN refused by the TLS rules, a DSN the driver cannot parse, an unknown driver, a negative `queryTimeoutMs`, a parameter that cannot bind, a `NULL` column read without `sql.isNull`, a column read as a type it does not hold |

### `sql.SqlError`

A server error is a `sql.SqlError`, so a handler tests the SQLSTATE rather
than the message:

```text
def struct sql.SqlError {
    kind as string,      # "sql"
    message as string,   # the server's text: "sql.exec: ERROR: duplicate key value ... (SQLSTATE 23505)"
    code as int,         # the MySQL error number (1062); 0 on Postgres and for bad column data
    sqlState as string   # the five-character SQLSTATE ("23505", "23000"); "" if none
};
```

```jennifer
try {
    sql.exec($db, "insert into users(email) values ($1)", $email);
} catch (e) {
    if ($e.kind != "sql") {
        throw $e;
    }
    def se as sql.SqlError init $e;
    if ($se.sqlState != "23505") {
        throw $e;
    }
    io.printf("already registered\n");
}
```

## Deprecated

Each still works and is removed in 0.29.0; `jennifer lint` reports it (L301).

| Deprecated | Use instead |
| --- | --- |
| `sql.setQueryTimeout(ms)` | `sql.openWith(driver, dsn, sql.Options{queryTimeoutMs: ms})` |
| driver `"mariadb"` | `"mysql"` |
| driver `"postgresql"` | `"postgres"` |

`sql.setQueryTimeout(ms)` sets the process-wide deadline of every connection
opened with `queryTimeoutMs` 0, before or after the call; `0` (or negative)
disables it. A connection's own `queryTimeoutMs` wins over it.

## See also

[net](net.md), [fs](fs.md), [json](json.md), [toml](toml.md).

## Academy talks

| Talk | What it shows |
| ---- | ------------- |
| [A production web API in Jennifer](https://academy.jennifer-lang.dev/talks/mplx/web-api/) | transactions, row locks and idempotent POST on PostgreSQL |
| [A web app in Jennifer from scratch](https://academy.jennifer-lang.dev/talks/mplx/web-app/) | reading and writing MariaDB rows |
| [From PHP to Jennifer](https://academy.jennifer-lang.dev/talks/mplx/from-php/) | database access without PDO, queries and rows |
