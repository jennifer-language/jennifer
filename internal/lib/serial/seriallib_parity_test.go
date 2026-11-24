// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build linux && !tinygo

package seriallib

import (
	"bytes"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// TestParityDecoder: the PARMRK marks decode across read boundaries; a read
// meeting a marked byte reports its offset once, the next read returns the
// bytes before it, and reading goes on behind the dropped byte. A pty cannot
// produce a parity error, so the marks are fed directly.
func TestParityDecoder(t *testing.T) {
	var d parityDecoder
	d.feed([]byte{'a', 'b', 0xFF})
	d.feed([]byte{0x00})
	d.feed([]byte{'X', 'c', 0xFF, 0xFF, 'd'})
	type step struct {
		n     int
		out   string
		errAt int
	}
	for i, s := range []step{
		{10, "", 2},
		{10, "ab", -1},
		{1, "c", -1},
		{10, "\xffd", -1},
	} {
		out, errAt, ok := d.take(s.n)
		if !ok || string(out) != s.out || errAt != s.errAt {
			t.Fatalf("step %d: take(%d) = %q, %d, %v; want %q, %d", i, s.n, out, errAt, ok, s.out, s.errAt)
		}
	}
	if _, _, ok := d.take(10); ok {
		t.Fatal("decoder should be empty")
	}
	// An error at offset 0, then one directly behind it.
	d.feed([]byte{0xFF, 0x00, 'X', 0xFF, 0x00, 'Y', 'z'})
	for i, s := range []step{{5, "", 0}, {5, "", 0}, {5, "z", -1}} {
		out, errAt, _ := d.take(s.n)
		if string(out) != s.out || errAt != s.errAt {
			t.Fatalf("step %d: take = %q, %d; want %q, %d", i, out, errAt, s.out, s.errAt)
		}
	}
	// A read for fewer bytes than precede the error does not meet it.
	d.feed([]byte{'a', 'b', 0xFF, 0x00, 'X'})
	if out, errAt, _ := d.take(2); string(out) != "ab" || errAt != -1 {
		t.Fatalf("take(2) = %q, %d", out, errAt)
	}
	if _, errAt, _ := d.take(2); errAt != 0 {
		t.Fatalf("then take(2) errAt = %d, want 0", errAt)
	}
}

// TestParityPortReadsMarkedFF: with parity on, the line discipline doubles a
// received 0xFF (PARMRK); serial.read returns it once.
func TestParityPortReadsMarkedFF(t *testing.T) {
	ResetForTest()
	master, slave := ptyPair(t)
	p := openWith(t, slave,
		field("baud", interpreter.IntVal(9600)),
		field("parity", interpreter.StringVal("even")),
		field("shared", interpreter.BoolVal(true)))
	want := []byte{'a', 0xFF, 'b', 0x00, 0xFF}
	if _, err := master.Write(want); err != nil {
		t.Fatal(err)
	}
	if _, err := setReadDeadlineFn(interpreter.BuiltinCtx{}, []Value{p, interpreter.IntVal(2000)}); err != nil {
		t.Fatal(err)
	}
	var got []byte
	for len(got) < len(want) {
		v, err := readFn(interpreter.BuiltinCtx{}, []Value{p, interpreter.IntVal(16)})
		if err != nil {
			t.Fatalf("read: %v (got %q so far)", err, got)
		}
		got = append(got, v.Bytes...)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("read %q, want %q", got, want)
	}
}

// TestClosedPort (M29.46): any call on a closed port is kind "closed"; a
// second close does nothing.
func TestClosedPort(t *testing.T) {
	ResetForTest()
	_, slave := ptyPair(t)
	p := openWith(t, slave, field("shared", interpreter.BoolVal(true)))
	ctx := interpreter.BuiltinCtx{}
	if _, err := closeFn(ctx, []Value{p}); err != nil {
		t.Fatal(err)
	}
	if _, err := closeFn(ctx, []Value{p}); err != nil {
		t.Fatalf("second close: %v", err)
	}
	for name, call := range map[string]func() error{
		"read":  func() error { _, err := readFn(ctx, []Value{p, interpreter.IntVal(1)}); return err },
		"write": func() error { _, err := writeFn(ctx, []Value{p, interpreter.BytesVal([]byte("x"))}); return err },
		"flush": func() error { _, err := flushFn(ctx, []Value{p}); return err },
		"setReadDeadline": func() error {
			_, err := setReadDeadlineFn(ctx, []Value{p, interpreter.IntVal(1)})
			return err
		},
	} {
		err := call()
		kind, msg, _, _, _ := interpreter.ClassifyError(err)
		if kind != "closed" || !strings.Contains(msg, "not open") {
			t.Errorf("%s after close: kind %q, %v", name, kind, err)
		}
	}
}
