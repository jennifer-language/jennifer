// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package sqllib

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"database/sql/driver"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	stdnet "net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// errKind is the kind Jennifer raises for err: a thrown struct's kind field,
// else "runtime".
func errKind(err error) string {
	var es *interpreter.ErrorSignal
	if errors.As(err, &es) {
		return field(es.Value, "kind").Str()
	}
	if err == nil {
		return ""
	}
	return "runtime"
}

func field(v Value, name string) Value {
	for _, f := range v.Fields() {
		if f.Name == name {
			return f.Value
		}
	}
	return interpreter.Null()
}

func thrown(t *testing.T, err error) Value {
	t.Helper()
	var es *interpreter.ErrorSignal
	if !errors.As(err, &es) {
		t.Fatalf("want a thrown struct, got %T %v", err, err)
	}
	return es.Value
}

func options(allowInsecure bool, ms int64) Value {
	return interpreter.NamespacedStructVal(LibraryName, "Options", []interpreter.StructField{
		{Name: "allowInsecure", Value: interpreter.BoolVal(allowInsecure)},
		{Name: "queryTimeoutMs", Value: interpreter.IntVal(ms)},
	})
}

// TestPostgresTLSPolicy: both DSN forms, every sslmode, the missing
// parameter, the exempt hosts and allowInsecure.
func TestPostgresTLSPolicy(t *testing.T) {
	t.Setenv("PGHOST", "")
	t.Setenv("PGSSLMODE", "")
	t.Setenv("PGSSLROOTCERT", "")
	caFile := writeTestCA(t)
	const added = "\x00added"
	cases := []struct {
		dsn      string
		insecure bool
		want     string // the DSN opened, added (the input + sslmode=verify-full), or "refused"
	}{
		// URL form, remote host.
		{"postgres://u:secret@db.example.com/app", false, "postgres://u:secret@db.example.com/app?sslmode=verify-full"},
		{"postgresql://u:secret@db.example.com:5432/app?application_name=x", false, "postgresql://u:secret@db.example.com:5432/app?application_name=x&sslmode=verify-full"},
		{"postgres://u:secret@db.example.com/app?sslmode=verify-full", false, added[:0]},
		// verify-ca checks the chain only: it needs a pinned CA.
		{"postgres://u:secret@db.example.com/app?sslmode=verify-ca", false, "refused"},
		{"postgres://u:secret@db.example.com/app?sslmode=verify-ca&sslrootcert=" + caFile, false, added[:0]},
		{"postgres://u:secret@db.example.com/app?sslmode=verify-ca&sslrootcert=system", false, added[:0]},
		{"host=db.example.com user=u sslmode=verify-ca", false, "refused"},
		{"host=db.example.com user=u sslmode=verify-ca sslrootcert=" + caFile, false, added[:0]},
		{"host=db.example.com user=u sslmode=verify-ca", true, added[:0]},
		// An empty or unknown mode is not a verifying one (pgx reads "" as prefer).
		{"postgres://u:secret@db.example.com/app?sslmode=", false, "refused"},
		{"host=db.example.com user=u password=secret sslmode=", false, "refused"},
		{"host=db.example.com user=u password=secret sslmode=''", false, "refused"},
		{"postgres://u:secret@db.example.com/app?sslmode=", true, added[:0]},
		{"postgres://u:secret@db.example.com/app?sslmode=disable", false, "refused"},
		{"postgres://u:secret@db.example.com/app?sslmode=allow", false, "refused"},
		{"postgres://u:secret@db.example.com/app?sslmode=prefer", false, "refused"},
		{"postgres://u:secret@db.example.com/app?sslmode=require", false, "refused"},
		{"postgres://u:secret@db.example.com/app?sslmode=disable", true, added[:0]},
		{"postgres://u:secret@db.example.com/app?sslmode=require", true, added[:0]},
		// A container's service name is another host.
		{"postgres://u:secret@db:5432/app?sslmode=disable", false, "refused"},
		{"postgres://u:secret@db:5432/app", false, "postgres://u:secret@db:5432/app?sslmode=verify-full"},
		// Keyword form.
		{"host=db.example.com user=u password=secret dbname=app", false, added},
		{"host=db.example.com user=u password='se cret' sslmode=disable", false, "refused"},
		{"host=db.example.com user=u password=secret sslmode=prefer", false, "refused"},
		{"host=db.example.com user=u password=secret sslmode=require", false, "refused"},
		{"host=db.example.com user=u password=secret sslmode=verify-full", false, added[:0]},
		{"host=db.example.com user=u password=secret sslmode=allow", true, added[:0]},
		// Multi-host: one remote host is enough to need TLS.
		{"host=localhost,db.example.com user=u sslmode=disable", false, "refused"},
		// Loopback and Unix sockets are exempt and used as written.
		{"postgres://u:secret@localhost/app?sslmode=disable", false, added[:0]},
		{"postgres://u:secret@localhost/app", false, added[:0]},
		{"postgres://u:secret@127.0.0.2:5432/app?sslmode=disable", false, added[:0]},
		{"postgres://u:secret@[::1]:5432/app?sslmode=disable", false, added[:0]},
		{"host=127.0.0.1 user=u sslmode=disable", false, added[:0]},
		{"host=/var/run/postgresql user=u sslmode=disable", false, added[:0]},
		{"postgres:///app?host=/tmp&sslmode=disable", false, added[:0]},
	}
	for _, c := range cases {
		got, err := securePostgresDSN("sql.open", c.dsn, c.insecure)
		want := c.want
		switch want {
		case "":
			want = c.dsn
		case added:
			want = c.dsn + " sslmode=verify-full"
		}
		if want == "refused" {
			if err == nil {
				t.Errorf("%q: opened as %q, want refused", c.dsn, got)
				continue
			}
			msg := err.Error()
			if !strings.Contains(msg, "allowInsecure: true") || !strings.Contains(msg, "sslmode=verify-full") {
				t.Errorf("%q: refusal names no fix: %s", c.dsn, msg)
			}
			if strings.Contains(msg, "secret") || strings.Contains(msg, "se cret") {
				t.Errorf("%q: refusal leaks the password: %s", c.dsn, msg)
			}
			continue
		}
		if err != nil || got != want {
			t.Errorf("%q (allowInsecure %v): got %q, %v; want %q", c.dsn, c.insecure, got, err, want)
		}
	}
}

// writeTestCA writes a self-signed CA certificate (PEM) for sslrootcert.
func writeTestCA(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test CA"},
		NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(p, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestMySQLTLSPolicy: the TCP form with each tls value, the missing
// parameter, a Unix socket and loopback hosts.
func TestMySQLTLSPolicy(t *testing.T) {
	cases := []struct {
		dsn      string
		insecure bool
		want     string // the resulting tls parameter, or "refused"
	}{
		{"u:secret@tcp(db.example.com:3306)/app", false, "true"},
		{"u:secret@tcp(db:3306)/app", false, "true"},
		{"u:secret@tcp(db.example.com:3306)/app?tls=true", false, "true"},
		{"u:secret@tcp(db.example.com:3306)/app?tls=1", false, "true"},
		{"u:secret@tcp(db.example.com:3306)/app?tls=false", false, "refused"},
		{"u:secret@tcp(db.example.com:3306)/app?tls=0", false, "refused"},
		{"u:secret@tcp(db.example.com:3306)/app?tls=skip-verify", false, "refused"},
		{"u:secret@tcp(db.example.com:3306)/app?tls=preferred", false, "refused"},
		{"u:secret@tcp(db.example.com:3306)/app?tls=false", true, "false"},
		{"u:secret@tcp(db.example.com:3306)/app?tls=skip-verify", true, "skip-verify"},
		{"u:secret@tcp([2001:db8::1]:3306)/app?tls=false", false, "refused"},
		// A plaintext fallback undoes the verified TLS.
		{"u:secret@tcp(db.example.com:3306)/app?allowFallbackToPlaintext=true", false, "refused"},
		{"u:secret@tcp(db.example.com:3306)/app?tls=true&allowFallbackToPlaintext=true", false, "refused"},
		{"u:secret@tcp(db.example.com:3306)/app?allowFallbackToPlaintext=true&allowCleartextPasswords=true", false, "refused"},
		{"u:secret@tcp(db.example.com:3306)/app?allowFallbackToPlaintext=true", true, "true"},
		{"u:secret@tcp(db.example.com:3306)/app?allowCleartextPasswords=true", false, "true"},
		{"u:secret@tcp(localhost:3306)/app?allowFallbackToPlaintext=true", false, ""},
		// Exempt: used as written.
		{"u:secret@tcp(localhost:3306)/app", false, ""},
		{"u:secret@tcp(127.0.0.1:3306)/app?tls=false", false, "false"},
		{"u:secret@tcp([::1]:3306)/app?tls=preferred", false, "preferred"},
		{"u:secret@/app", false, ""}, // the driver's default, 127.0.0.1:3306
		{"u:secret@unix(/run/mysqld/mysqld.sock)/app", false, ""},
		{"u:secret@unix(/run/mysqld/mysqld.sock)/app?tls=false", false, "false"},
	}
	for _, c := range cases {
		cfg, err := secureMySQLConfig("sql.open", c.dsn, c.insecure)
		if c.want == "refused" {
			if err == nil {
				t.Errorf("%q: opened with tls=%q, want refused", c.dsn, cfg.TLSConfig)
				continue
			}
			msg := err.Error()
			if !strings.Contains(msg, "allowInsecure: true") || (!strings.Contains(msg, "tls=true") && !strings.Contains(msg, "allowFallbackToPlaintext")) {
				t.Errorf("%q: refusal names no fix: %s", c.dsn, msg)
			}
			if strings.Contains(msg, "secret") {
				t.Errorf("%q: refusal leaks the password: %s", c.dsn, msg)
			}
			continue
		}
		if err != nil || cfg.TLSConfig != c.want {
			t.Errorf("%q: got %v, %v; want tls=%q", c.dsn, cfg, err, c.want)
			continue
		}
		if _, ok := cfg.Logger.(*mysql.NopLogger); !ok {
			t.Errorf("%q: the driver logs to %T, want the silent logger", c.dsn, cfg.Logger)
		}
		// The connector builds a verifying TLS config naming the host.
		if c.want == "true" {
			if _, err := mysql.NewConnector(cfg); err != nil {
				t.Errorf("%q: connector: %v", c.dsn, err)
			}
		}
	}
}

// TestLocalhostIsNeverResolved: the exempt name `localhost` dials the
// loopback addresses without asking the resolver, so a DNS answer (or a
// search-domain wildcard) cannot receive the plaintext login.
func TestLocalhostIsNeverResolved(t *testing.T) {
	t.Setenv("PGHOST", "")
	resolved := []string{}
	next := func(_ context.Context, host string) ([]string, error) {
		resolved = append(resolved, host)
		return []string{"192.0.2.1"}, nil
	}
	look := loopbackLookup(next)
	for _, h := range []string{"localhost", "LocalHost.", "localhost."} {
		got, err := look(context.Background(), h)
		if err != nil || strings.Join(got, ",") != "127.0.0.1,::1" {
			t.Errorf("%q -> %v, %v; want the loopback addresses", h, got, err)
		}
	}
	if got, _ := look(context.Background(), "db.example.com"); strings.Join(got, ",") != "192.0.2.1" {
		t.Errorf("another name is not passed on: %v", got)
	}
	if strings.Join(resolved, ",") != "db.example.com" {
		t.Errorf("resolver asked for %v", resolved)
	}

	// Postgres: the connection config carries the wrapped lookup.
	pc, err := postgresConnConfig("postgres://u:secret@localhost/app?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := pc.LookupFunc(context.Background(), "localhost"); err != nil || strings.Join(got, ",") != "127.0.0.1,::1" {
		t.Errorf("postgres lookup of localhost -> %v, %v", got, err)
	}

	// MySQL: a localhost address dials the loopback listener directly.
	ln, err := stdnet.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("listen: %v", err)
	}
	defer ln.Close()
	_, port, _ := stdnet.SplitHostPort(ln.Addr().String())
	mc, err := secureMySQLConfig("sql.open", "u:secret@tcp(localhost:"+port+")/app", false)
	if err != nil || mc.DialFunc == nil {
		t.Fatalf("mysql localhost config: %v, dial func set %v", err, mc != nil && mc.DialFunc != nil)
	}
	c, err := mc.DialFunc(context.Background(), "tcp", mc.Addr)
	if err != nil {
		t.Fatalf("dial localhost: %v", err)
	}
	if ra := c.RemoteAddr().String(); ra != ln.Addr().String() {
		t.Errorf("dialled %s, want %s", ra, ln.Addr())
	}
	c.Close()
	if mc, _ := secureMySQLConfig("sql.open", "u:secret@tcp(127.0.0.1:3306)/app", false); mc.DialFunc != nil {
		t.Error("an IP literal needs no dial override")
	}
}

// TestDSNParseErrorsHideThePassword: a DSN the drivers cannot parse is a
// runtime error that does not quote the password.
func TestDSNParseErrorsHideThePassword(t *testing.T) {
	for _, c := range []struct{ driver, dsn string }{
		{"postgres", "postgres://u:s3cr3t@db.example.com:notaport/app"},
		{"postgres", "host=db.example.com password='s3cr3t"},
		{"mysql", "u:s3cr3t@tcp(db.example.com:3306"},
		{"mysql", "u:s3cr3t@tcp(db.example.com:3306)/app?tls=custom"},
	} {
		_, err := openFn(ctx(), []Value{sv(c.driver), sv(c.dsn)})
		if err == nil || errKind(err) != "runtime" {
			t.Errorf("%s %q: %v, want a runtime error", c.driver, c.dsn, err)
			continue
		}
		if strings.Contains(err.Error(), "s3cr3t") {
			t.Errorf("%s %q: the message leaks the password: %s", c.driver, c.dsn, err)
		}
	}
}

// TestOpenRefusesWeakTLS: sql.open refuses before it dials; openWith with
// allowInsecure gets past the policy (and fails to connect to the closed
// loopback port instead).
func TestOpenRefusesWeakTLS(t *testing.T) {
	_, err := openFn(ctx(), []Value{sv("postgres"), sv("postgres://u:secret@db.invalid/app?sslmode=disable")})
	if err == nil || errKind(err) != "runtime" || !strings.Contains(err.Error(), "sql.open: sslmode=disable") {
		t.Errorf("open: %v", err)
	}
	_, err = openWithFn(ctx(), []Value{sv("mysql"), sv("u:secret@tcp(db.invalid:3306)/app?tls=false"), options(false, 0)})
	if err == nil || !strings.Contains(err.Error(), "sql.openWith: tls=false") {
		t.Errorf("openWith: %v", err)
	}
	if _, err := openWithFn(ctx(), []Value{sv("mysql"), sv("u:p@/db"), sv("x")}); err == nil || !strings.Contains(err.Error(), "sql.Options") {
		t.Errorf("openWith with a string for options: %v", err)
	}
	if _, err := openFn(ctx(), []Value{sv("sqlite"), sv("x")}); err == nil || !strings.Contains(err.Error(), `use "mysql" or "postgres"`) {
		t.Errorf("unknown driver: %v", err)
	}
}

// TestTransportFailureIsNetError: a refused connection is a net.NetError
// with reason "refused", for both drivers, with the password redacted.
func TestTransportFailureIsNetError(t *testing.T) {
	ln, err := stdnet.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close() // a closed port: connect is refused
	for _, c := range []struct{ driver, dsn string }{
		{"mysql", "u:s3cr3t@tcp(" + addr + ")/db"},
		{"postgres", "postgres://u:s3cr3t@" + addr + "/db"},
		{"mariadb", "u:s3cr3t@tcp(" + addr + ")/db"},
		{"postgresql", "postgres://u:s3cr3t@" + addr + "/db"},
	} {
		_, err := openWithFn(ctx(), []Value{sv(c.driver), sv(c.dsn), options(false, 5000)})
		if errKind(err) != "net" {
			t.Errorf("%s: %v, want kind net", c.driver, err)
			continue
		}
		v := thrown(t, err)
		if v.StructNS() != "net" || v.StructName() != "NetError" {
			t.Errorf("%s: thrown %s.%s, want net.NetError", c.driver, v.StructNS(), v.StructName())
		}
		if r := field(v, "reason").Str(); r != "refused" {
			t.Errorf("%s: reason %q, want refused (%s)", c.driver, r, field(v, "message").Str())
		}
		if op := field(v, "op").Str(); op != "connect" {
			t.Errorf("%s: op %q, want connect", c.driver, op)
		}
		if strings.Contains(field(v, "message").Str(), "s3cr3t") {
			t.Errorf("%s: message leaks the password: %s", c.driver, field(v, "message").Str())
		}
	}
}

// TestQueryTimeoutRules: 0 follows the process-wide default (which the
// deprecated setQueryTimeout changes), a value wins over it, MaxInt is none,
// a negative value throws.
func TestQueryTimeoutRules(t *testing.T) {
	t.Cleanup(ResetForTest)
	ResetForTest()
	for _, c := range []struct {
		ms       int64
		ns       int64
		explicit bool
		err      bool
	}{
		{0, 0, false, false},
		{1500, int64(1500 * time.Millisecond), true, false},
		{math.MaxInt64, 0, true, false},
		{maxTimeoutMs + 1, maxTimeoutMs * int64(time.Millisecond), true, false},
		{-1, 0, false, true},
	} {
		ns, explicit, err := resolveQueryTimeout("sql.openWith", c.ms)
		if (err != nil) != c.err || ns != c.ns || explicit != c.explicit {
			t.Errorf("queryTimeoutMs %d: got %d %v %v", c.ms, ns, explicit, err)
		}
	}
	if _, err := openWithFn(ctx(), []Value{sv("mysql"), sv("u:p@/db"), options(false, -5)}); err == nil ||
		errKind(err) != "runtime" || !strings.Contains(err.Error(), "queryTimeoutMs must be >= 0") {
		t.Errorf("negative queryTimeoutMs: %v", err)
	}

	inherit := &connState{}
	own := &connState{timeoutNs: int64(2 * time.Second), explicit: true}
	none := &connState{explicit: true}
	if inherit.limit() != 30*time.Second || own.limit() != 2*time.Second || none.limit() != 0 {
		t.Errorf("defaults: %v %v %v", inherit.limit(), own.limit(), none.limit())
	}
	if _, err := setQueryTimeoutFn(ctx(), []Value{interpreter.IntVal(100)}); err != nil {
		t.Fatal(err)
	}
	if inherit.limit() != 100*time.Millisecond || own.limit() != 2*time.Second || none.limit() != 0 {
		t.Errorf("after setQueryTimeout(100): %v %v %v", inherit.limit(), own.limit(), none.limit())
	}
	if _, err := setQueryTimeoutFn(ctx(), []Value{interpreter.IntVal(0)}); err != nil {
		t.Fatal(err)
	}
	if inherit.limit() != 0 || own.limit() != 2*time.Second {
		t.Errorf("after setQueryTimeout(0): %v %v", inherit.limit(), own.limit())
	}
}

// TestDbFailureMapping: server errors become sql.SqlError with the error
// number and SQLSTATE, deadlines "timeout", transport failures net.NetError,
// anything else "runtime".
func TestDbFailureMapping(t *testing.T) {
	my := &mysql.MySQLError{Number: 1062, SQLState: [5]byte{'2', '3', '0', '0', '0'}, Message: "Duplicate entry '1' for key 'PRIMARY'"}
	pg := &pgconn.PgError{Severity: "ERROR", Code: "23505", Message: "duplicate key value violates unique constraint"}
	for _, c := range []struct {
		name  string
		err   error
		code  int64
		state string
	}{
		{"mysql", my, 1062, "23000"},
		{"mysql wrapped", fmt.Errorf("exec: %w", my), 1062, "23000"},
		{"mysql no state", &mysql.MySQLError{Number: 1045, Message: "Access denied"}, 1045, ""},
		{"postgres", pg, 0, "23505"},
	} {
		err := dbFailure(ctx(), "sql.exec", "read", c.err, time.Second)
		v := thrown(t, err)
		if v.StructNS() != "sql" || v.StructName() != "SqlError" || field(v, "kind").Str() != "sql" {
			t.Errorf("%s: thrown %s.%s kind %q", c.name, v.StructNS(), v.StructName(), field(v, "kind").Str())
		}
		if field(v, "code").Int() != c.code || field(v, "sqlState").Str() != c.state {
			t.Errorf("%s: code %d sqlState %q, want %d %q", c.name, field(v, "code").Int(), field(v, "sqlState").Str(), c.code, c.state)
		}
		if !strings.HasPrefix(field(v, "message").Str(), "sql.exec: ") {
			t.Errorf("%s: message %q", c.name, field(v, "message").Str())
		}
	}

	if k := errKind(dbFailure(ctx(), "sql.query", "read", context.DeadlineExceeded, time.Second)); k != "timeout" {
		t.Errorf("deadline: kind %q", k)
	}
	if err := dbFailure(ctx(), "sql.query", "read", context.DeadlineExceeded, time.Second); !strings.Contains(err.Error(), "queryTimeoutMs") {
		t.Errorf("deadline message names no fix: %v", err)
	}

	refused := &stdnet.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}
	for _, c := range []struct {
		name, op, reason string
		err              error
	}{
		{"refused", "connect", "refused", refused},
		{"dns", "connect", "dns", &stdnet.OpError{Op: "dial", Net: "tcp", Err: &stdnet.DNSError{Err: "no such host", Name: "db"}}},
		{"reset", "read", "reset", &stdnet.OpError{Op: "read", Net: "tcp", Err: os.NewSyscallError("read", syscall.ECONNRESET)}},
		{"bad conn", "read", "reset", driver.ErrBadConn},
		{"mysql invalid conn", "read", "reset", mysql.ErrInvalidConn},
		{"eof", "read", "reset", io.ErrUnexpectedEOF},
		{"no tls", "handshake", "tls", mysql.ErrNoTLS},
	} {
		err := dbFailure(ctx(), "sql.query", "read", c.err, time.Second)
		v := thrown(t, err)
		if v.StructNS() != "net" || v.StructName() != "NetError" || field(v, "kind").Str() != "net" {
			t.Errorf("%s: thrown %s.%s", c.name, v.StructNS(), v.StructName())
			continue
		}
		if field(v, "op").Str() != c.op || field(v, "reason").Str() != c.reason {
			t.Errorf("%s: op %q reason %q, want %q %q", c.name, field(v, "op").Str(), field(v, "reason").Str(), c.op, c.reason)
		}
	}

	for _, e := range []error{sql.ErrTxDone, errors.New("sql: converting argument")} {
		if k := errKind(dbFailure(ctx(), "sql.commit", "read", e, 0)); k != "runtime" {
			t.Errorf("%v: kind %q, want runtime", e, k)
		}
	}
}

// -------- accessor data errors --------

type dataDriver struct{}

func (dataDriver) Open(string) (driver.Conn, error) { return dataConn{}, nil }

type dataConn struct{}

func (dataConn) Prepare(string) (driver.Stmt, error) { return dataStmt{}, nil }
func (dataConn) Close() error                        { return nil }
func (dataConn) Begin() (driver.Tx, error)           { return mockTx{}, nil }

type dataStmt struct{}

func (dataStmt) Close() error                               { return nil }
func (dataStmt) NumInput() int                              { return -1 }
func (dataStmt) Exec([]driver.Value) (driver.Result, error) { return mockResult{}, nil }
func (dataStmt) Query([]driver.Value) (driver.Rows, error)  { return &dataRows{}, nil }

type dataRows struct{ done bool }

var dataCols = []string{"bad", "badstr", "ok", "inf", "ninf", "nanstr", "two", "one", "zero", "word", "big"}

func (*dataRows) Columns() []string { return dataCols }
func (*dataRows) Close() error      { return nil }
func (r *dataRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	copy(dest, []driver.Value{
		[]byte{'a', 'b', 0xff, 'c'}, "x\xc3", "hé�",
		math.Inf(1), math.Inf(-1), []byte("NaN"),
		int64(2), []byte("1"), "0", []byte("yes"), uint64(7),
	})
	return nil
}

var registerDataOnce sync.Once

func TestAccessorDataErrors(t *testing.T) {
	registerDataOnce.Do(func() { sql.Register("mockdata", dataDriver{}) })
	ResetForTest()
	t.Cleanup(ResetForTest)
	db, err := sql.Open("mockdata", "")
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	nid++
	id := nid
	conns[id] = &connState{db: db}
	mu.Unlock()
	rowsV, err := queryFn(ctx(), []Value{handle("Connection", id), sv("SELECT")})
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := nextFn(ctx(), []Value{rowsV}); err != nil || !ok.Bool() {
		t.Fatalf("next: %v", err)
	}
	sqlErr := func(name string, err error, want string) {
		t.Helper()
		if errKind(err) != "sql" {
			t.Errorf("%s: %v, want kind sql", name, err)
			return
		}
		v := thrown(t, err)
		if v.StructName() != "SqlError" || field(v, "code").Int() != 0 || field(v, "sqlState").Str() != "" {
			t.Errorf("%s: %s{code %d, sqlState %q}", name, v.StructName(), field(v, "code").Int(), field(v, "sqlState").Str())
		}
		if msg := field(v, "message").Str(); !strings.Contains(msg, want) {
			t.Errorf("%s: message %q, want it to contain %q", name, msg, want)
		}
	}
	_, err = asStringFn(ctx(), []Value{rowsV, sv("bad")})
	sqlErr("asString bad", err, `column "bad" is not valid UTF-8 (byte offset 2)`)
	_, err = asStringFn(ctx(), []Value{rowsV, interpreter.IntVal(1)})
	sqlErr("asString badstr", err, `column "badstr" is not valid UTF-8 (byte offset 1)`)
	assertStr(t, rowsV, sv("ok"), "hé�")
	if b, err := asBytesFn(ctx(), []Value{rowsV, sv("bad")}); err != nil || len(b.Bytes()) != 4 {
		t.Errorf("asBytes bad: %v %v", b, err)
	}

	for _, col := range []string{"inf", "ninf", "nanstr"} {
		_, err = asFloatFn(ctx(), []Value{rowsV, sv(col)})
		sqlErr("asFloat "+col, err, "not a finite number")
	}
	assertFloat(t, rowsV, sv("two"), 2)

	_, err = asBoolFn(ctx(), []Value{rowsV, sv("two")})
	sqlErr("asBool two", err, `column "two" holds 2; a bool is 0 or 1`)
	_, err = asBoolFn(ctx(), []Value{rowsV, sv("big")})
	sqlErr("asBool big", err, "a bool is 0 or 1")
	assertBool(t, rowsV, sv("one"), true)
	assertBool(t, rowsV, sv("zero"), false)
	if _, err := asBoolFn(ctx(), []Value{rowsV, sv("word")}); errKind(err) != "runtime" {
		t.Errorf("asBool word: %v, want a runtime error", err)
	}
}
