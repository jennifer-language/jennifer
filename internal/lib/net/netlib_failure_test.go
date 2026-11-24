// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package netlib

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	stdnet "net"
	"os"
	"syscall"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// TestFailureClassifiesSocketErrors pins failure's mapping of a Go socket error
// to the Error a program catches: a deadline is kind "timeout", a socket closed
// under the call (net.ErrClosed, however deeply wrapped) is kind "closed", and
// any other socket error is a net.NetError (kind "net") with the OS text and a
// reason; a malformed address stays an unclassified runtime error.
func TestFailureClassifiesSocketErrors(t *testing.T) {
	ctx := interpreter.BuiltinCtx{File: "f.j", Line: 3, Col: 5}
	sysErr := func(op string, errno syscall.Errno) error {
		return &stdnet.OpError{Op: op, Net: "tcp", Err: os.NewSyscallError(op, errno)}
	}
	cases := []struct {
		op        string
		err       error
		kind, msg string
		reason    string
	}{
		{"read", &stdnet.OpError{Op: "read", Err: os.ErrDeadlineExceeded}, "timeout", "net.readBytes: read timed out", ""},
		{"read", fmt.Errorf("accept tcp 127.0.0.1:1: %w", stdnet.ErrClosed), "closed", "net.readBytes: read on a closed socket", ""},
		{"read", &stdnet.OpError{Op: "read", Err: stdnet.ErrClosed}, "closed", "net.readBytes: read on a closed socket", ""},
		{"read", errors.New("connection reset by peer"), "net", "net.readBytes: connection reset by peer", "other"},
		{"read", sysErr("read", syscall.ECONNRESET), "net", "net.readBytes: read tcp: read: connection reset by peer", "reset"},
		{"write", sysErr("write", syscall.EPIPE), "net", "net.readBytes: write tcp: write: broken pipe", "reset"},
		{"connect", sysErr("dial", syscall.ECONNREFUSED), "net", "net.readBytes: dial tcp: dial: connection refused", "refused"},
		{"connect", sysErr("dial", syscall.EHOSTUNREACH), "net", "net.readBytes: dial tcp: dial: no route to host", "unreachable"},
		{"connect", &stdnet.OpError{Op: "dial", Net: "tcp", Err: &stdnet.DNSError{Err: "no such host", Name: "x.invalid", IsNotFound: true}}, "net", "net.readBytes: dial tcp: lookup x.invalid: no such host", "dns"},
		{"handshake", &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}, "net", "net.readBytes: tls: failed to verify certificate: x509: certificate signed by unknown authority", "tls"},
		{"handshake", errors.New("EOF"), "net", "net.readBytes: EOF", "tls"},
		{"read", tls.RecordHeaderError{Msg: "first record does not look like a TLS handshake"}, "net", "net.readBytes: tls: first record does not look like a TLS handshake", "tls"},
		{"connect", &stdnet.OpError{Op: "dial", Net: "tcp", Err: &stdnet.AddrError{Err: "missing port in address", Addr: "x"}}, "", "net.readBytes: dial tcp: address x: missing port in address", ""},
	}
	for _, c := range cases {
		got := failure(ctx, "net.readBytes", c.op, c.err)
		var sig *interpreter.ErrorSignal
		if !errors.As(got, &sig) {
			if c.kind != "" {
				t.Errorf("%v: got %T %q, want a %q Error", c.err, got, got, c.kind)
			} else if got.Error() != c.msg {
				t.Errorf("%v: got %q, want %q", c.err, got, c.msg)
			}
			continue
		}
		var kind, msg, reason string
		for _, f := range sig.Value.Fields() {
			switch f.Name {
			case "kind":
				kind = f.Value.Str()
			case "message":
				msg = f.Value.Str()
			case "reason":
				reason = f.Value.Str()
			}
		}
		if kind != c.kind || msg != c.msg || reason != c.reason {
			t.Errorf("%v: got kind %q message %q reason %q, want %q %q %q", c.err, kind, msg, reason, c.kind, c.msg, c.reason)
		}
		if kind == "net" && (sig.Value.StructNS() != "net" || sig.Value.StructName() != "NetError") {
			t.Errorf("%v: thrown as %s.%s, want net.NetError", c.err, sig.Value.StructNS(), sig.Value.StructName())
		}
	}
}
