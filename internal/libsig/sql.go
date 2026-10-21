// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("sql", `
open(string, string) sql.Connection
close(sql.Connection) null
setQueryTimeout(int) null
# params: placeholder values, or one list spread into the parameter sequence
query(sql.Connection|sql.Tx, string, int|float|string|bool|bytes|null|list of any...) sql.Rows
exec(sql.Connection|sql.Tx, string, int|float|string|bool|bytes|null|list of any...) sql.Result
next(sql.Rows) bool
columns(sql.Rows) list of string
asInt(sql.Rows, int|string) int
asFloat(sql.Rows, int|string) float
asString(sql.Rows, int|string) string
asBool(sql.Rows, int|string) bool
asBytes(sql.Rows, int|string) bytes
isNull(sql.Rows, int|string) bool
closeRows(sql.Rows) null
begin(sql.Connection) sql.Tx
commit(sql.Tx) null
rollback(sql.Tx) null
prepare(sql.Connection, string) sql.Statement
queryStmt(sql.Statement, int|float|string|bool|bytes|null|list of any...) sql.Rows
execStmt(sql.Statement, int|float|string|bool|bytes|null|list of any...) sql.Result
closeStmt(sql.Statement) null
`)
}
