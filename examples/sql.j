# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

# The `sql` library: a relational client over MySQL / MariaDB and PostgreSQL.
# Guarded so this demo runs anywhere (it reports "no database" when no server is
# reachable); against a real server the same calls do the query. Values ONLY
# bind through placeholders - never string interpolation - so injection is not
# expressible. A localhost DSN is used as written; a DSN naming another host gets
# verified TLS (sslmode=verify-full / tls=true), and one that weakens it is
# refused unless opened with sql.openWith(..., sql.Options{allowInsecure: true}).
use sql;
use io;

try {
    def db as sql.Connection init sql.open("postgres", "postgres://user:pw@localhost/app");
    defer sql.close($db); # closed however this block exits
    
    # exec with a bound parameter ($1 for Postgres, ? for MySQL).
    def res as sql.Result init sql.exec($db, "insert into t(v) values ($1)", "hello");
    io.printf("inserted %d row(s)\n", $res.affected);

    # a prepared statement: parse once on the server, run many with fresh
    # parameters. `defer` closes it however this block exits.
    def stmt as sql.Statement init sql.prepare($db, "insert into t(v) values ($1)");
    defer sql.closeStmt($stmt);
    sql.execStmt($stmt, "world");
    sql.execStmt($stmt, "again");

    # query + pull cursor + typed accessors.
    def rows as sql.Rows init sql.query($db, "select id, name from t where id > $1", 0);
    while (sql.next($rows)) {
        io.printf("%d %s\n", sql.asInt($rows, "id"), sql.asString($rows, "name"));
    }
} catch (e) {
    # No server is kind "net" (a net.NetError: reason "refused", "dns", ...);
    # an error the server reports is kind "sql" (a sql.SqlError carrying the
    # SQLSTATE); a passed query deadline is kind "timeout".
    io.printf("no database\n");
}
