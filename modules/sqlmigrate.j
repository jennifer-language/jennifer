# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.25.0
# pragma-jennifer-capability: sql

/**
 * A version-tracked schema-migration runner over the [`sql`](../libraries/sql.md)
 * library. It applies ordered, reversible DDL migrations and records what ran in
 * a `schema_migrations` tracking table, so each migration runs **exactly once**
 * and can be rolled back.
 *
 * It is deliberately **decoupled from the [`orm`](orm.md) mapper**: a migration's
 * `up` / `down` are plain `list of string` DDL statements, so you build them with
 * `orm`'s DDL helpers (`orm.createTable` / `orm.dropTable` / `orm.addColumn` /
 * ...) **or** hand-written SQL - the runner never touches a `Schema` or a
 * `Query`. That keeps it usable for any `sql` program, ORM or not.
 *
 * The runner owns transaction control: each migration's `up` (or `down`)
 * statements plus its tracking-table write run inside **one** `sql` transaction.
 * On PostgreSQL (transactional DDL) a failed statement rolls the whole step back.
 * On MySQL / MariaDB every DDL statement commits implicitly, so a step that fails
 * after some of its DDL ran keeps that DDL and stays unrecorded: keep one DDL
 * statement per migration there. It takes a raw `sql.Connection` (not an
 * `orm.Session`) for that reason.
 *
 * `migrate` and `rollbackMigrations` hold a database-wide advisory lock for the
 * whole run (`pg_try_advisory_xact_lock` / `GET_LOCK`), so two instances
 * starting at once apply each migration once; a run that cannot get the lock
 * within 60 seconds throws kind `timeout`.
 *
 * Migrations are ordered by `version` **lexically**, so zero-pad numeric
 * versions (`"001"`, `"002"`, ...) or use a sortable timestamp
 * (`"20260101120000"`). The tracking table (`version VARCHAR(255) PRIMARY KEY,
 * description TEXT`) is accepted verbatim by both MySQL and PostgreSQL, so the
 * runner needs no dialect.
 *
 * Needs `sql`, so the default `jennifer` binary.
 * @module sqlmigrate
 * @example
 * use sql;
 * import "sqlmigrate.j" as mig;
 * def conn as sql.Connection init sql.open("postgres", "postgres://app:secret@localhost/app");
 * def m1 as mig.Migration init mig.Migration{
 *     version: "001",
 *     description: "create users",
 *     up: ["CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)"],
 *     down: ["DROP TABLE users"]
 * };
 * def applied as int init mig.migrate($conn, [$m1]);   # 1 the first run, 0 after
 */
use sql;
use maps;
use strings;
use convert;
use regex;
use lists;
use time;

# The tracking table: dialect-agnostic (VARCHAR PK + TEXT are accepted by both
# MySQL and PostgreSQL), so the runner never needs a dialect.
def const TABLE as string init "schema_migrations";

# The migration lock is polled (a non-blocking try every LOCK_POLL_MS) so a wait
# never runs into the sql query timeout; LOCK_TRIES * LOCK_POLL_MS is the 60 s
# limit before a run gives up with kind "timeout".
def const LOCK_POLL_MS as int init 100;
def const LOCK_TRIES as int init 600;

# The lock keys: per database, so runs against different databases on one
# server never wait for each other. MySQL lock names are capped at 64
# characters, hence the SHA1.
def const PG_LOCK_KEY as string init "hashtext(current_database() || '.sqlmigrate')";
def const MYSQL_LOCK_NAME as string init "SHA1(CONCAT('sqlmigrate.', COALESCE(DATABASE(), '')))";

/**
 * One schema migration: an ordered `up` and reverse `down` list of DDL
 * statements (build them with `orm`'s DDL helpers or hand-written SQL),
 * identified by `version`. Runs are ordered by `version` **lexically**, so
 * zero-pad numeric versions (`"001"`, `"002"`, ...) or use a sortable timestamp.
 * @field version {string} the ordering key ([A-Za-z0-9._-])
 * @field description {string} a human-readable label (recorded in the tracking table)
 * @field up {list of string} the forward DDL statements
 * @field down {list of string} the reverse DDL statements
 */
export def struct Migration {
    version as string,
    description as string,
    up as list of string,
    down as list of string
};

/**
 * The applied / pending state of one migration, from `sqlmigrate.migrationStatus`.
 * @field version {string} the migration version
 * @field description {string} its description
 * @field applied {bool} whether it has been applied
 */
export def struct MigrationStatus {
    version as string,
    description as string,
    applied as bool
};

func fail(message as string) {
    throw Error{kind: "sqlmigrate", message: "sqlmigrate: " + $message};
}

# validVersion allowlists a migration version ([A-Za-z0-9._-]); the version is
# interpolated into the tracking-table SQL, so it must be a safe token. One RE2
# match does the whole check in Go instead of a per-byte loop.
func validVersion(s as string) {
    return regex.matches('^[A-Za-z0-9._-]+$', $s);
}

# escapeLiteral renders s as the body of a single-quoted SQL literal (each `'`
# doubled). A backslash or control character is rejected, so the description can
# never break out of its quotes in either dialect (MySQL backslash-escapes,
# Postgres does not).
func escapeLiteral(s as string) {
    def raw as bytes init convert.bytesFromString("utf-8", $s);
    for (def i in 0..len($raw)) {
        if ($raw[$i] == 92) {
            fail("a migration description may not contain a backslash");
        }
        if ($raw[$i] < 32) {
            fail("a migration description may not contain a control character");
        }
    }
    return strings.replace($s, "'", "''");
}

# versionKey extracts a migration's sort key for `lists.sortBy`.
func versionKey(m as Migration) {
    return $m.version;
}

# sortMigrations checks every version, then returns the migrations ordered by
# version ascending (lexical, so "10" sorts after "9" - zero-pad numeric
# versions). Checking the whole list first means a bad or duplicate version
# fails before any migration runs, not halfway through.
func sortMigrations(migrations as list of Migration) {
    def seen as map of string to bool init {};
    for (def m in $migrations) {
        if (not validVersion($m.version)) {
            fail("migration version must be [A-Za-z0-9._-], got: " + $m.version);
        }
        if (maps.has($seen, $m.version)) {
            fail("duplicate migration version: " + $m.version);
        }
        $seen[$m.version] = true;
        escapeLiteral($m.description);
    }
    return lists.sortBy($migrations, versionKey);
}

# isPostgres tells the two supported servers apart (version() exists in both;
# only PostgreSQL's starts with "PostgreSQL").
func isPostgres(conn as sql.Connection) {
    def noParams as list of string init [];
    def rows as sql.Rows init sql.query($conn, "SELECT version() AS v", $noParams);
    defer sql.closeRows($rows);
    sql.next($rows);
    return strings.startsWith(sql.asString($rows, "v"), "PostgreSQL");
}

# tryLock makes one non-blocking attempt at the migration lock on lock's
# connection and reports whether it got it.
func tryLock(lock as sql.Tx, postgres as bool) {
    def noParams as list of string init [];
    def stmt as string init "SELECT GET_LOCK(" + MYSQL_LOCK_NAME + ", 0) AS got";
    if ($postgres) {
        $stmt = "SELECT CASE WHEN pg_try_advisory_xact_lock(" + PG_LOCK_KEY +
            ") THEN 1 ELSE 0 END AS got";
    }
    def rows as sql.Rows init sql.query($lock, $stmt, $noParams);
    defer sql.closeRows($rows);
    if (not sql.next($rows) or sql.isNull($rows, "got")) {
        fail("could not take the migration lock");
    }
    return sql.asInt($rows, "got") == 1;
}

# acquireLock takes the migration lock and returns the transaction that pins
# its connection (the pool would otherwise hand later statements to another
# session). Postgres releases the transaction-level lock at rollback; MySQL's
# GET_LOCK outlives transactions, so releaseLock frees it explicitly. Waits up
# to LOCK_TRIES polls, then throws kind "timeout".
func acquireLock(conn as sql.Connection, postgres as bool) {
    def lock as sql.Tx init sql.begin($conn);
    errdefer sql.rollback($lock);
    for (def i in 0..LOCK_TRIES) {
        if (tryLock($lock, $postgres)) {
            return $lock;
        }
        time.sleep(time.fromMilliseconds(LOCK_POLL_MS));
    }
    throw Error{
        kind: "timeout",
        message: "sqlmigrate: another migration run held the lock for over 60 seconds"
    };
}

# releaseLock frees the migration lock and the connection pinned for it.
func releaseLock(lock as sql.Tx, postgres as bool) {
    defer sql.rollback($lock);
    if (not $postgres) {
        def noParams as list of string init [];
        sql.exec($lock, "DO RELEASE_LOCK(" + MYSQL_LOCK_NAME + ")", $noParams);
    }
}

# ensureTable creates the tracking table if absent.
func ensureTable(conn as sql.Connection) {
    def noParams as list of string init [];
    sql.exec(
        $conn,
        "CREATE TABLE IF NOT EXISTS " + TABLE +
            " (version VARCHAR(255) PRIMARY KEY, description TEXT)",
        $noParams);
}

# appliedVersions reads the set of already-applied versions (value is a marker).
func appliedVersions(conn as sql.Connection) {
    def out as map of string to string init {};
    def noParams as list of string init [];
    def rows as sql.Rows init sql.query($conn, "SELECT version FROM " + TABLE, $noParams);
    defer sql.closeRows($rows);
    repeat {
        if (not sql.next($rows)) {
            break;
        }
        $out[sql.asString($rows, "version")] = "1";
    } until (false);
    return $out;
}

# applyMigration runs one migration's `up` statements and records it, all inside
# a single transaction (rolled back on any error).
func applyMigration(conn as sql.Connection, m as Migration) {
    def noParams as list of string init [];
    def tx as sql.Tx init sql.begin($conn);
    errdefer sql.rollback($tx);
    for (def stmt in $m.up) {
        sql.exec($tx, $stmt, $noParams);
    }
    sql.exec(
        $tx,
        "INSERT INTO " + TABLE + " (version, description) VALUES ('" + $m.version + "', '" +
            escapeLiteral($m.description) + "')",
        $noParams);
    sql.commit($tx);
}

# rollbackOne runs one migration's `down` statements and un-records it, in a
# single transaction.
func rollbackOne(conn as sql.Connection, m as Migration) {
    def noParams as list of string init [];
    def tx as sql.Tx init sql.begin($conn);
    errdefer sql.rollback($tx);
    for (def stmt in $m.down) {
        sql.exec($tx, $stmt, $noParams);
    }
    sql.exec($tx, "DELETE FROM " + TABLE + " WHERE version = '" + $m.version + "'", $noParams);
    sql.commit($tx);
}

/**
 * Apply every pending migration in `version` order, each inside its own
 * transaction, recording it in the `schema_migrations` tracking table (created
 * if absent). Idempotent: already-applied versions are skipped. Holds the
 * migration lock for the whole run, so concurrent runs apply each migration
 * once; throws kind `timeout` when another run holds it for over 60 seconds.
 * @param conn {sql.Connection} the open connection
 * @param migrations {list of Migration} the full migration set (any order)
 * @return {int} the number of migrations applied this run
 */
export func migrate(conn as sql.Connection, migrations as list of Migration) {
    def sorted as list of Migration init sortMigrations($migrations);
    def postgres as bool init isPostgres($conn);
    def lock as sql.Tx init acquireLock($conn, $postgres);
    defer releaseLock($lock, $postgres);
    ensureTable($conn);
    def applied as map of string to string init appliedVersions($conn);
    def count as int init 0;
    for (def m in $sorted) {
        if (maps.has($applied, $m.version)) {
            continue;
        }
        applyMigration($conn, $m);
        $count = $count + 1;
    }
    return $count;
}

/**
 * Roll back the `steps` most-recently-applied migrations (by version order),
 * running each one's `down` statements newest-first inside its own transaction
 * and removing its tracking-table row. Holds the migration lock like `migrate`.
 * @param conn {sql.Connection} the open connection
 * @param migrations {list of Migration} the full migration set
 * @param steps {int} how many applied migrations to reverse
 * @return {int} the number of migrations rolled back
 */
export func rollbackMigrations(
    conn as sql.Connection,
    migrations as list of Migration,
    steps as int) {
    def sorted as list of Migration init sortMigrations($migrations);
    def postgres as bool init isPostgres($conn);
    def lock as sql.Tx init acquireLock($conn, $postgres);
    defer releaseLock($lock, $postgres);
    ensureTable($conn);
    def applied as map of string to string init appliedVersions($conn);
    def count as int init 0;
    for (def i as int init len($sorted) - 1; $i >= 0; $i = $i - 1) {
        if ($count >= $steps) {
            break;
        }
        def m as Migration init $sorted[$i];
        if (maps.has($applied, $m.version)) {
            rollbackOne($conn, $m);
            $count = $count + 1;
        }
    }
    return $count;
}

/**
 * The applied / pending state of every migration, in `version` order.
 * @param conn {sql.Connection} the open connection
 * @param migrations {list of Migration} the full migration set
 * @return {list of MigrationStatus} one entry per migration, in version order
 */
export func migrationStatus(conn as sql.Connection, migrations as list of Migration) {
    ensureTable($conn);
    def applied as map of string to string init appliedVersions($conn);
    def sorted as list of Migration init sortMigrations($migrations);
    def out as list of MigrationStatus init [];
    for (def m in $sorted) {
        $out[] = MigrationStatus{
            version: $m.version,
            description: $m.description,
            applied: maps.has($applied, $m.version)
        };
    }
    return $out;
}
