# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

# sqlmigrate_demo.j - build a version-tracked migration plan whose up/down steps
# are plain DDL strings (from orm's DDL helpers or hand-written), print it, and
# define runMigrations for you to run against your own database. Run it:
#
#     jennifer run examples/modules/sqlmigrate_demo.j

use io;
import "../../modules/orm.j" as orm;
import "../../modules/sqlmigrate.j" as mig;
use sql;

# Migration 001 builds its DDL with orm's schema + DDL helpers.
def users as orm.Schema init orm.schema("users", "id", orm.Dialect.Postgres);
$users = orm.autoIncrement(orm.column($users, "id", orm.ColumnKind.Int));
$users = orm.notNull(orm.column($users, "name", orm.ColumnKind.String));
def m1 as mig.Migration init mig.Migration{
    version: "001",
    description: "create users",
    up: [orm.createTable($users)],
    down: [orm.dropTable("users")]
};

# Migration 002 mixes DDL helpers - the runner just executes the strings, so a
# hand-written statement works exactly the same.
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

io.printf("migration plan (%d):\n", len($plan));
for (def m in $plan) {
    io.printf("  [%s] %s\n", $m.version, $m.description);
    for (def stmt in $m.up) {
        io.printf("      up:   %s\n", $stmt);
    }
    for (def stmt in $m.down) {
        io.printf("      down: %s\n", $stmt);
    }
}

# runMigrations applies the plan against a live connection. migrate is idempotent
# and records each version in schema_migrations, so re-running applies only what
# is new; rollbackMigrations reverses the newest N steps.
func runMigrations(conn as sql.Connection) {
    io.printf("applied %d migration(s)\n", mig.migrate($conn, $plan));
    for (def st in mig.migrationStatus($conn, $plan)) {
        io.printf("  %s applied=%t\n", $st.version, $st.applied);
    }
    # undo the most recent step
    mig.rollbackMigrations($conn, $plan, 1);
}

io.printf("\n(runMigrations is defined; call it with a live sql.Connection)\n");
