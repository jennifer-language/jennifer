// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo

package netlib

import (
	"errors"
	"fmt"
	stdnet "net"
	"os"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// TestFailureClassifiesSocketErrors pins failure's mapping of a Go socket error
// to the Error a program catches: a deadline is kind "timeout", a socket closed
// under the call (net.ErrClosed, however deeply wrapped) is kind "closed", and
// anything else stays an unclassified runtime error with the OS text.
func TestFailureClassifiesSocketErrors(t *testing.T) {
	ctx := interpreter.BuiltinCtx{File: "f.j", Line: 3, Col: 5}
	cases := []struct {
		err       error
		kind, msg string
	}{
		{&stdnet.OpError{Op: "read", Err: os.ErrDeadlineExceeded}, "timeout", "net.readBytes: read timed out"},
		{fmt.Errorf("accept tcp 127.0.0.1:1: %w", stdnet.ErrClosed), "closed", "net.readBytes: read on a closed socket"},
		{&stdnet.OpError{Op: "read", Err: stdnet.ErrClosed}, "closed", "net.readBytes: read on a closed socket"},
		{errors.New("connection reset by peer"), "", "net.readBytes: connection reset by peer"},
	}
	for _, c := range cases {
		got := failure(ctx, "net.readBytes", "read", c.err)
		var sig *interpreter.ErrorSignal
		if !errors.As(got, &sig) {
			if c.kind != "" {
				t.Errorf("%v: got %T %q, want a %q Error", c.err, got, got, c.kind)
			} else if got.Error() != c.msg {
				t.Errorf("%v: got %q, want %q", c.err, got, c.msg)
			}
			continue
		}
		var kind, msg string
		for _, f := range sig.Value.Fields {
			switch f.Name {
			case "kind":
				kind = f.Value.Str
			case "message":
				msg = f.Value.Str
			}
		}
		if kind != c.kind || msg != c.msg {
			t.Errorf("%v: got kind %q message %q, want %q %q", c.err, kind, msg, c.kind, c.msg)
		}
	}
}
