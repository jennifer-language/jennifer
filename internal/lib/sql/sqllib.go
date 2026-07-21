// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package sqllib implements Jennifer's `sql` library: a relational-database
// client over Go's `database/sql`, shipping the two client-server engines
// MySQL / MariaDB (`go-sql-driver/mysql`) and PostgreSQL (`jackc/pgx`). Both are
// pure-Go drivers. SQLite (the one embedded engine) is deliberately out - it
// needs a multi-MB cgo-free-but-huge dependency and cannot build under TinyGo.
//
// These are the first heavyweight third-party dependencies in the library layer,
// a conscious exception to the dependency-free discipline - see
// docs/technical/design-decisions.md. The mature drivers absorb a decade of
// protocol long-tail (auth plugins, charsets, NULL semantics, SCRAM) a
// hand-rolled `.j` client would re-derive one edge case at a time.
//
// Build-tag split like `net` / `httpd`: sqllib_std.go (`!tinygo`) imports
// `database/sql` + the drivers; sqllib_tiny.go (`tinygo`) registers the surface
// and returns a friendly "not available on this build" error from every verb, so
// TinyGo never compiles the driver trees.
//
// Values bind ONLY through placeholders, so string interpolation - the SQL
// injection vector - is never how a value reaches a query. The placeholder
// spelling is the engine's own (`?` for MySQL, `$1`..`$n` for Postgres); the
// SQL text is passed to the driver verbatim, nothing translates between
// dialects. Handles use the integer-registry pattern from `fs` / `net`.
package sqllib

import (
	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// LibraryName is the namespace prefix and `use` name.
const LibraryName = "sql"

// Value keeps builtin signatures short.
type Value = interpreter.Value

func idField() []parser.StructField {
	return []parser.StructField{{Name: "id", Type: parser.PrimitiveType(parser.TypeInt)}}
}

// Install registers the sql surface. The engine-driving verbs are provided by
// the build-tag-selected file (sqllib_std.go or sqllib_tiny.go).
func Install(in *interpreter.Interpreter) {
	// Handle structs (integer registry ids).
	in.RegisterNamespacedStruct(LibraryName, "Connection", idField())
	in.RegisterNamespacedStruct(LibraryName, "Rows", idField())
	in.RegisterNamespacedStruct(LibraryName, "Tx", idField())
	in.RegisterNamespacedStruct(LibraryName, "Statement", idField())
	// A result is a plain value struct (no live handle): read its fields.
	in.RegisterNamespacedStruct(LibraryName, "Result", []parser.StructField{
		{Name: "affected", Type: parser.PrimitiveType(parser.TypeInt)},
		{Name: "lastId", Type: parser.PrimitiveType(parser.TypeInt)},
	})

	// Connection lifecycle.
	in.RegisterNamespaced(LibraryName, "open", openFn)
	in.RegisterNamespaced(LibraryName, "close", closeFn)
	// Query / exec (first arg: a Connection or a Tx).
	in.RegisterNamespaced(LibraryName, "query", queryFn)
	in.RegisterNamespaced(LibraryName, "exec", execFn)
	// Row cursor.
	in.RegisterNamespaced(LibraryName, "next", nextFn)
	in.RegisterNamespaced(LibraryName, "columns", columnsFn)
	in.RegisterNamespaced(LibraryName, "asInt", asIntFn)
	in.RegisterNamespaced(LibraryName, "asFloat", asFloatFn)
	in.RegisterNamespaced(LibraryName, "asString", asStringFn)
	in.RegisterNamespaced(LibraryName, "asBool", asBoolFn)
	in.RegisterNamespaced(LibraryName, "asBytes", asBytesFn)
	in.RegisterNamespaced(LibraryName, "isNull", isNullFn)
	in.RegisterNamespaced(LibraryName, "closeRows", closeRowsFn)
	// Transactions.
	in.RegisterNamespaced(LibraryName, "begin", beginFn)
	in.RegisterNamespaced(LibraryName, "commit", commitFn)
	in.RegisterNamespaced(LibraryName, "rollback", rollbackFn)
	// Prepared statements.
	in.RegisterNamespaced(LibraryName, "prepare", prepareFn)
	in.RegisterNamespaced(LibraryName, "queryStmt", queryStmtFn)
	in.RegisterNamespaced(LibraryName, "execStmt", execStmtFn)
	in.RegisterNamespaced(LibraryName, "closeStmt", closeStmtFn)
}
