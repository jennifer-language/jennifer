// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

// How a driver error reaches Jennifer (M29.36): a server error is a
// sql.SqlError (kind "sql") carrying the MySQL error number and the SQLSTATE,
// the client query deadline is kind "timeout", and a transport failure
// (refused, reset, unreachable, DNS, TLS) is a net.NetError (kind "net"),
// classified as the net library does. Anything else stays "runtime".

package sqllib

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	stdnet "net"
	"syscall"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"

	"jennifer-lang.dev/jennifer/internal/dnsconf"
	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// SqlErrorKind is the kind of every sql.SqlError: a server error, and bad
// data read from a column (invalid UTF-8, a NaN float, a bool that is not
// 0 / 1).
const SqlErrorKind = "sql"

// newSqlError throws a sql.SqlError anchored at the call site. code is the
// MySQL error number (0 on Postgres and for a data error), sqlState the
// five-character SQLSTATE ("" when the server sent none).
func newSqlError(ctx interpreter.BuiltinCtx, message string, code int64, sqlState string) error {
	v := interpreter.NamespacedStructVal(LibraryName, "SqlError", []interpreter.StructField{
		{Name: "kind", Value: interpreter.StringVal(SqlErrorKind)},
		{Name: "message", Value: interpreter.StringVal(message)},
		{Name: "code", Value: interpreter.IntVal(code)},
		{Name: "sqlState", Value: interpreter.StringVal(sqlState)},
	})
	return &interpreter.ErrorSignal{Value: v, File: ctx.File, Line: ctx.Line, Col: ctx.Col}
}

// newNetError throws a net.NetError, the struct the net library throws for
// a transport failure; sent / replied are false (no protocol exchange facts).
func newNetError(ctx interpreter.BuiltinCtx, op, reason, message string) error {
	v := interpreter.NamespacedStructVal("net", "NetError", []interpreter.StructField{
		{Name: "kind", Value: interpreter.StringVal("net")},
		{Name: "message", Value: interpreter.StringVal(message)},
		{Name: "op", Value: interpreter.StringVal(op)},
		{Name: "reason", Value: interpreter.StringVal(reason)},
		{Name: "sent", Value: interpreter.BoolVal(false)},
		{Name: "replied", Value: interpreter.BoolVal(false)},
	})
	return &interpreter.ErrorSignal{Value: v, File: ctx.File, Line: ctx.Line, Col: ctx.Col}
}

// serverError extracts the error number and SQLSTATE of a server error.
func serverError(err error) (code int64, sqlState string, ok bool) {
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		if me.SQLState != [5]byte{} {
			sqlState = string(me.SQLState[:])
		}
		return int64(me.Number), sqlState, true
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return 0, pe.Code, true
	}
	return 0, "", false
}

// isTimeout reports a deadline: the client query timeout (the context), or
// a driver / socket timeout.
func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || pgconn.Timeout(err) {
		return true
	}
	var ne stdnet.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// transportFailure classifies a transport failure as the net library does
// (op: connect / handshake / read / write; reason: refused / reset /
// unreachable / dns / tls / other). ok is false for an error that is not one.
// defaultOp names the step when the error does not (connect for sql.open).
func transportFailure(err error, defaultOp string) (op, reason string, ok bool) {
	op = defaultOp
	var opErr *stdnet.OpError
	if errors.As(err, &opErr) {
		switch opErr.Op {
		case "dial":
			op = "connect"
		case "read", "write":
			op = opErr.Op
		}
		ok = true
	}
	var connErr *pgconn.ConnectError
	if errors.As(err, &connErr) {
		op, ok = "connect", true
	}
	var dnsErr *stdnet.DNSError
	var errno syscall.Errno
	switch {
	case errors.As(err, &dnsErr):
		return op, "dns", true
	case isTLSError(err) || errors.Is(err, mysql.ErrNoTLS):
		return "handshake", "tls", true
	case errors.As(err, &errno) && errnoReason(errno) != "":
		return op, errnoReason(errno), true
	case errors.Is(err, driver.ErrBadConn), errors.Is(err, mysql.ErrInvalidConn),
		errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, io.EOF), errors.Is(err, pgconn.ErrConnClosed):
		// The server or the network tore the connection down.
		return op, "reset", true
	}
	return op, "other", ok
}

// isTLSError reports a certificate verification or TLS protocol error (the
// net library's test).
func isTLSError(err error) bool {
	var certErr *tls.CertificateVerificationError
	var authErr x509.UnknownAuthorityError
	var hostErr x509.HostnameError
	var invalidErr x509.CertificateInvalidError
	var recordErr tls.RecordHeaderError
	var alertErr tls.AlertError
	var opErr *stdnet.OpError
	switch {
	case errors.As(err, &certErr), errors.As(err, &authErr), errors.As(err, &hostErr),
		errors.As(err, &invalidErr), errors.As(err, &recordErr), errors.As(err, &alertErr):
		return true
	case errors.As(err, &opErr) && opErr.Op == "remote error":
		return true
	}
	return false
}

// dbFailure turns a driver error from fn into the error Jennifer raises.
// limit is the client query timeout in force (0 = none), named in a timeout's
// message; defaultOp the transport step for an error that names none.
func dbFailure(ctx interpreter.BuiltinCtx, fn, defaultOp string, err error, limit time.Duration) error {
	msg := fmt.Sprintf("%s: %s", fn, redactDSN(err.Error()))
	if code, state, ok := serverError(err); ok {
		return newSqlError(ctx, msg, code, state)
	}
	if isTimeout(err) {
		if errors.Is(err, context.DeadlineExceeded) && limit > 0 {
			msg = fmt.Sprintf("%s: the client query timeout (%s) passed; raise it with sql.Options.queryTimeoutMs", fn, limit)
		}
		return interpreter.RaiseError("timeout", msg, ctx.File, ctx.Line, ctx.Col)
	}
	if op, reason, ok := transportFailure(err, defaultOp); ok {
		if reason == "dns" {
			// Name the resolver files when none is configured (M29.5).
			msg = dnsconf.Setup().Explain(msg)
		}
		return newNetError(ctx, op, reason, msg)
	}
	return errors.New(msg)
}
