// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package termlib

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	"jennifer-lang.dev/jennifer/internal/stdinbuf"
)

// ioCall calls an `io` builtin with the given context.
func ioCall(t *testing.T, name string, ctx interpreter.BuiltinCtx, args ...Value) (Value, error) {
	t.Helper()
	in := interpreter.New()
	iolib.Install(in)
	return in.LookupNamespacedBuiltin("io", name)(ctx, args)
}

// M28.38: a term read that timed out leaves its read in flight; the byte it
// collects is the next one io.readLine sees, in order, and term reads after
// the line continue where it ended. Run under -race: only the shared layer
// touches the input.
func TestTimedOutKeyThenReadLine(t *testing.T) {
	ResetForTest()
	r, w := io.Pipe()
	defer w.Close()
	ctx := interpreter.BuiltinCtx{In: r, Out: &bytes.Buffer{}}
	if got := readTimeout(t, ctx, 20); got != gotTimeout {
		t.Fatalf("idle timed read = %d, want timeout", got)
	}
	go func() { _, _ = w.Write([]byte("hello\nZ")) }()
	v, err := ioCall(t, "readLine", ctx)
	if err != nil || v.Str != "hello" {
		t.Fatalf("io.readLine after a timed-out key read = %v, %v; want \"hello\"", v, err)
	}
	if got := readTimeout(t, ctx, 2000); got != 'Z' {
		t.Errorf("term read after the line = %d, want 'Z'", got)
	}
	// io.readBytes and term reads interleave byte for byte.
	go func() { _, _ = w.Write([]byte("abc")) }()
	if v, err := readByteFn(ctx, nil); err != nil || v.Int != 'a' {
		t.Fatalf("readByte = %v, %v; want 'a'", v, err)
	}
	if v, err := ioCall(t, "readBytes", ctx, interpreter.IntVal(1)); err != nil || string(v.Bytes) != "b" {
		t.Fatalf("io.readBytes(1) = %v, %v; want b", v, err)
	}
	if v, err := ioCall(t, "readChars", ctx, interpreter.IntVal(1)); err != nil || v.Str != "c" {
		t.Fatalf("io.readChars(1) = %v, %v; want c", v, err)
	}
}

// io.eof peeks a byte; a term read then takes that same byte.
func TestEofPeekThenKey(t *testing.T) {
	ResetForTest()
	ctx := interpreter.BuiltinCtx{In: strings.NewReader("q"), Out: &bytes.Buffer{}}
	if v, err := ioCall(t, "eof", ctx); err != nil || v.Bool {
		t.Fatalf("io.eof = %v, %v; want false", v, err)
	}
	if v, err := readByteFn(ctx, nil); err != nil || v.Int != 'q' {
		t.Fatalf("readByte after io.eof = %v, %v; want 'q'", v, err)
	}
	if v, err := ioCall(t, "eof", ctx); err != nil || !v.Bool {
		t.Fatalf("io.eof at end = %v, %v; want true", v, err)
	}
}

// End of input reached by one library is end of input for the other, whichever
// sees it first.
func TestEOFReachesBothReaders(t *testing.T) {
	ResetForTest()
	r, w := io.Pipe()
	ctx := interpreter.BuiltinCtx{In: r, Out: &bytes.Buffer{}}
	if got := readTimeout(t, ctx, 10); got != gotTimeout {
		t.Fatalf("idle timed read = %d, want timeout", got)
	}
	_ = w.Close()
	if v, err := ioCall(t, "eof", ctx); err != nil || !v.Bool {
		t.Errorf("io.eof after close = %v, %v; want true", v, err)
	}
	if _, err := ioCall(t, "readLine", ctx); err == nil || !strings.Contains(err.Error(), "end of input") {
		t.Errorf("io.readLine after close: %v", err)
	}
	if got := readTimeout(t, ctx, 2000); got != gotClosed {
		t.Errorf("term timed read at EOF = %d, want closed", got)
	}
	if got := readOne(t, ctx); got != gotClosed {
		t.Errorf("term.readByte at EOF = %d, want closed", got)
	}

	// The other order: term sees the end first, io agrees.
	stdinbuf.ResetForTest()
	ctx = interpreter.BuiltinCtx{In: strings.NewReader(""), Out: &bytes.Buffer{}}
	if got := readOne(t, ctx); got != gotClosed {
		t.Errorf("term.readByte on empty input = %d", got)
	}
	if v, err := ioCall(t, "readLines", ctx); err != nil || len(v.List) != 0 {
		t.Errorf("io.readLines after term EOF = %v, %v; want []", v, err)
	}
}

// A closed stdin is an error for both libraries, not end of input.
func TestClosedStdinReachesBothReaders(t *testing.T) {
	ResetForTest()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	_ = r.Close()
	ctx := interpreter.BuiltinCtx{In: r, Out: &bytes.Buffer{}}
	if _, err := readByteTimeoutFn(ctx, []Value{interpreter.IntVal(1000)}); err == nil || errKind(err) != "" || !strings.Contains(err.Error(), "closed") {
		t.Errorf("term.readByteTimeout on closed stdin: %v", err)
	}
	if _, err := ioCall(t, "readLine", ctx); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Errorf("io.readLine on closed stdin: %v", err)
	}
	if _, err := readByteFn(ctx, nil); err == nil {
		t.Error("term.readByte on closed stdin should error")
	}
}
