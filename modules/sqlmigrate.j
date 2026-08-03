# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

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
 * statements plus its tracking-table write run inside **one** `sql` transaction,
 * so a failed statement rolls the whole step back and leaves `schema_migrations`
 * consistent. It takes a raw `sql.Connection` (not an `orm.Session`) for that
 * reason.
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
 * import "sqlmigrate.j" as mig;
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

# The tracking table: dialect-agnostic (VARCHAR PK + TEXT are accepted by both
# MySQL and PostgreSQL), so the runner never needs a dialect.
def const TABLE as string init "schema_migrations";

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
    throw Error{kind: "sqlmigrate", message: "sqlmigrate: " + $message, file: "", line: 0, col: 0};
}

# validVersion allowlists a migration version ([A-Za-z0-9._-]); the version is
# interpolated into the tracking-table SQL, so it must be a safe token.
func validVersion(s as string) {
    def raw as bytes init convert.bytesFromString($s, "utf-8");
    if (len($raw) == 0) {
        return false;
    }
    for (def i as int init 0; $i < len($raw); $i = $i + 1) {
        def b as int init $raw[$i];
        def ok as bool init ($b >= 48 and $b <= 57) or ($b >= 65 and $b <= 90) or
            ($b >= 97 and $b <= 122) or $b == 46 or $b == 95 or $b == 45;
        if (not $ok) {
            return false;
        }
    }
    return true;
}

# escapeLiteral renders s as the body of a single-quoted SQL literal (each `'`
# doubled). A backslash or control character is rejected, so the description can
# never break out of its quotes in either dialect (MySQL backslash-escapes,
# Postgres does not).
func escapeLiteral(s as string) {
    def raw as bytes init convert.bytesFromString($s, "utf-8");
    for (def i as int init 0; $i < len($raw); $i = $i + 1) {
        if ($raw[$i] == 92) {
            fail("a migration description may not contain a backslash");
        }
        if ($raw[$i] < 32) {
            fail("a migration description may not contain a control character");
        }
    }
    return strings.replace($s, "'", "''");
}

# strLess compares two strings byte-wise (Jennifer's `<` is numeric-only), for
# lexical migration ordering.
func strLess(a as string, b as string) {
    def ba as bytes init convert.bytesFromString($a, "utf-8");
    def bb as bytes init convert.bytesFromString($b, "utf-8");
    def n as int init len($ba);
    if (len($bb) < $n) {
        $n = len($bb);
    }
    for (def i as int init 0; $i < $n; $i = $i + 1) {
        if ($ba[$i] < $bb[$i]) {
            return true;
        }
        if ($ba[$i] > $bb[$i]) {
            return false;
        }
    }
    return len($ba) < len($bb);
}

# sortMigrations returns the migrations ordered by version ascending (a small
# insertion sort; migration lists are short).
func sortMigrations(migrations as list of Migration) {
    def out as list of Migration init [];
    for (def m in $migrations) {
        def next as list of Migration init [];
        def inserted as bool init false;
        for (def x in $out) {
            if ((not $inserted) and strLess($m.version, $x.version)) {
                $next[] = $m;
                $inserted = true;
            }
            $next[] = $x;
        }
        if (not $inserted) {
            $next[] = $m;
        }
        $out = $next;
    }
    return $out;
}

# ensureTable creates the tracking table if absent.
func ensureTable(conn as sql.Connection) {
    def noParams as list of string init [];
    sql.exec($conn,
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
    if (not validVersion($m.version)) {
        fail("migration version must be [A-Za-z0-9._-], got: " + $m.version);
    }
    def noParams as list of string init [];
    def tx as sql.Tx init sql.begin($conn);
    errdefer sql.rollback($tx);
    for (def stmt in $m.up) {
        sql.exec($tx, $stmt, $noParams);
    }
    sql.exec($tx,
        "INSERT INTO " + TABLE + " (version, description) VALUES ('" + $m.version + "', '" +
            escapeLiteral($m.description) + "')",
        $noParams);
    sql.commit($tx);
}

# rollbackOne runs one migration's `down` statements and un-records it, in a
# single transaction.
func rollbackOne(conn as sql.Connection, m as Migration) {
    if (not validVersion($m.version)) {
        fail("migration version must be [A-Za-z0-9._-], got: " + $m.version);
    }
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
 * if absent). Idempotent: already-applied versions are skipped.
 * @param conn {sql.Connection} the open connection
 * @param migrations {list of Migration} the full migration set (any order)
 * @return {int} the number of migrations applied this run
 */
export func migrate(conn as sql.Connection, migrations as list of Migration) {
    ensureTable($conn);
    def applied as map of string to string init appliedVersions($conn);
    def sorted as list of Migration init sortMigrations($migrations);
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
 * and removing its tracking-table row.
 * @param conn {sql.Connection} the open connection
 * @param migrations {list of Migration} the full migration set
 * @param steps {int} how many applied migrations to reverse
 * @return {int} the number of migrations rolled back
 */
export func rollbackMigrations(conn as sql.Connection, migrations as list of Migration, steps as int) {
    ensureTable($conn);
    def applied as map of string to string init appliedVersions($conn);
    def sorted as list of Migration init sortMigrations($migrations);
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
