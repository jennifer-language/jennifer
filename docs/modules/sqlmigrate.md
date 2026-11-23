# `sqlmigrate` - version-tracked schema migrations over `sql`

Import with `import "sqlmigrate.j" as mig;`. A version-tracked schema-migration
runner over the [`sql`](../libraries/sql.md) library: it applies ordered,
reversible DDL migrations and records what ran in a `schema_migrations` tracking
table, so each migration runs **exactly once** and can be rolled back.

It is **decoupled from the [`orm`](orm.md) mapper**. A migration's
`up` / `down` are plain `list of string` DDL statements, so you build them with
`orm`'s DDL helpers (`orm.createTable` / `orm.dropTable` / `orm.addColumn` / ...)
**or** hand-written SQL - the runner never touches a `Schema` or a `Query`, so
it serves any `sql` program, ORM or not.

```jennifer
use sql;
import "sqlmigrate.j" as mig;

def conn as sql.Connection init sql.open("postgres", "postgres://app:secret@localhost/app");
defer sql.close($conn);
def m1 as mig.Migration init mig.Migration{
    version: "001",
    description: "create users",
    up: ["CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)"],
    down: ["DROP TABLE users"]
};
def applied as int init mig.migrate($conn, [$m1]);   # 1 the first run, 0 after
```

Runnable: [`examples/modules/sqlmigrate_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/sqlmigrate_demo.j).

## The `Migration` struct

| Field | Type | Meaning |
| ----- | ---- | ------- |
| `version` | `string` | The ordering key (`[A-Za-z0-9._-]`). |
| `description` | `string` | A human-readable label, recorded in the tracking table. |
| `up` | `list of string` | The forward DDL statements. |
| `down` | `list of string` | The reverse DDL statements. |

Migrations are ordered by `version` **lexically**, so zero-pad numeric versions
(`"001"`, `"002"`, ...) or use a sortable timestamp (`"20260101120000"`).

## The runner

Each function takes a raw `sql.Connection` (not an `orm.Session`) because the
runner owns transaction control: each migration's `up` (or `down`) statements
**plus** its tracking-table write run inside **one** `sql` transaction. How much
that transaction covers depends on the server (see
[Transactions and DDL](#transactions-and-ddl)). The whole list is checked
before anything runs: an invalid or duplicate
version, or a description with a backslash or control character, throws kind
`sqlmigrate` with no migration applied.

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `mig.migrate(conn, migrations)` | `int` | Apply every pending migration in version order; returns how many ran. Idempotent - applied versions are skipped. |
| `mig.rollbackMigrations(conn, migrations, steps)` | `int` | Reverse the `steps` most-recently-applied migrations (newest first); returns how many ran. |
| `mig.migrationStatus(conn, migrations)` | `list of MigrationStatus` | The applied / pending state of every migration, in version order. |

`MigrationStatus` is `{version as string, description as string, applied as
bool}`. The tracking table (`version VARCHAR(255) PRIMARY KEY, description TEXT`)
is created on first use and is accepted verbatim by both MySQL and PostgreSQL, so
the runner needs no dialect.

```jennifer
import "orm.j" as orm;

# Build the DDL with orm's helpers, or hand-write it - the runner runs strings.
def m2 as mig.Migration init mig.Migration{
    version: "002",
    description: "add unique email",
    up: [
        orm.addColumn("users", "email", orm.ColumnKind.String, orm.Dialect.Postgres),
        orm.createIndex("idxUsersEmail", "users", ["email"], true)
    ],
    down: [
        orm.dropIndex("idxUsersEmail", "users", orm.Dialect.Postgres),
        orm.dropColumn("users", "email")
    ]
};
def plan as list of mig.Migration init [$m1, $m2];
mig.migrate($conn, $plan);                  # applies 001 then 002
mig.rollbackMigrations($conn, $plan, 1);    # reverses 002
```

## Transactions and DDL

- **PostgreSQL** has transactional DDL: a statement that fails rolls the whole
  step back, its earlier DDL included, and nothing is recorded.
- **MySQL / MariaDB** commit implicitly before and after every DDL statement
  (`CREATE`, `ALTER`, `DROP`, `RENAME`, ...), so the transaction cannot undo
  them. A step that fails after some of its DDL ran **keeps that DDL** and stays
  unrecorded as pending, and the next `migrate` runs the step again from its
  first statement (which then fails, e.g. `Table 'x' already exists`) until the
  schema is repaired by hand. Keep **one DDL statement per migration** there, or
  write each step so it can be re-run (`CREATE TABLE IF NOT EXISTS`, `DROP ...
  IF EXISTS`).

## Concurrent runs

`migrate` and `rollbackMigrations` hold an advisory lock for the whole run, so
two instances starting at once (several replicas of a service, a deploy hook
racing a manual run) apply each migration once: the second waits, then sees the
steps the first recorded and skips them. The lock is per database:
`pg_try_advisory_xact_lock` on PostgreSQL (keyed on
`hashtext(current_database() || '.sqlmigrate')`), `GET_LOCK` on MySQL / MariaDB
(named `SHA1(CONCAT('sqlmigrate.', COALESCE(DATABASE(), '')))`). It is held on a connection
of its own, taken from the pool for the run and freed when the run ends,
successfully or not; a process that dies loses its session and with it the
lock. A run that cannot get the lock within **60 seconds** throws
`Error{kind: "timeout"}` with nothing applied. `migrationStatus` only reads and
does not take the lock.

## Safety

- **Version and description are injection-safe.** The `version` is allowlisted
  (`[A-Za-z0-9._-]`) and the `description` is escaped as a quoted SQL literal
  (backslash / control characters rejected), both before reaching the
  tracking-table SQL. The `up` / `down` DDL strings are the caller's own (built by
  `orm`'s allowlist-checked helpers or hand-written); the runner executes them
  verbatim.
- **Atomic per migration on PostgreSQL.** A migration that fails partway leaves
  nothing recorded and nothing half-applied. On MySQL / MariaDB only the
  tracking-table write is atomic; DDL that already ran stays (see above).

Needs `sql`, so the **default `jennifer`** binary.

## Academy talks

| Talk | What it shows |
| ---- | ------------- |
| [A production web API in Jennifer](https://academy.jennifer-lang.dev/talks/mplx/web-api/) | migrations run once at deploy under a lock |
| [A web app in Jennifer from scratch](https://academy.jennifer-lang.dev/talks/mplx/web-app/) | database migrations on start-up |

## API reference

Every exported function, struct and constant of `sqlmigrate`, with its signature, generated
from the module's doc comments: [the `sqlmigrate` API reference](../api/sqlmigrate.md).
