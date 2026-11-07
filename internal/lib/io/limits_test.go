// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package iolib

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

type zeros struct{}

func (zeros) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func stdinCtx(r io.Reader) interpreter.BuiltinCtx {
	return interpreter.BuiltinCtx{Out: &bytes.Buffer{}, In: r}
}

// TestReadLineCap: a line past maxLineBytes on stdin is a catchable error.
// The input is one byte over the cap, so the pre-fix code returned it.
func TestReadLineCap(t *testing.T) {
	resetInputForTest()
	in := interpreter.New()
	Install(in)
	_, err := callReadLine(in, stdinCtx(io.LimitReader(zeros{}, limits.MaxAllocBytes+1)), nil)
	if err == nil || !strings.Contains(err.Error(), "readLine: line exceeds") {
		t.Fatalf("got %v", err)
	}
}

// TestReadLinesCaps: readLines refuses a total input past maxLineBytes, and
// more than maxLines lines.
func TestReadLinesCaps(t *testing.T) {
	resetInputForTest()
	in := interpreter.New()
	Install(in)
	line := strings.Repeat("x", 1<<20-1) + "\n"
	src := io.MultiReader(strings.NewReader(strings.Repeat(line, limits.MaxAllocBytes>>20)), strings.NewReader("y\n"))
	_, err := callReadLines(in, stdinCtx(src))
	if err == nil || !strings.Contains(err.Error(), "readLines: input exceeds") || !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("bytes: got %v", err)
	}
	resetInputForTest()
	_, err = callReadLines(in, stdinCtx(strings.NewReader(strings.Repeat("\n", limits.MaxRangeElements/4+1))))
	if err == nil || !strings.Contains(err.Error(), "line limit") {
		t.Fatalf("lines: got %v", err)
	}
	resetInputForTest()
	v, err := callReadLines(in, stdinCtx(strings.NewReader(strings.Repeat("\n", 1000))))
	if err != nil || len(v.List) != 1000 {
		t.Fatalf("1000 lines: %v", err)
	}
}

// TestReadCharsCap: a count past maxReadChars is refused before reading.
func TestReadCharsCap(t *testing.T) {
	resetInputForTest()
	in := interpreter.New()
	Install(in)
	fn := in.LookupNamespacedBuiltin("io", "readChars")
	_, err := fn(stdinCtx(strings.NewReader("abc")), []interpreter.Value{interpreter.IntVal(9000000000000)})
	if err == nil || !strings.Contains(err.Error(), "readChars: count 9000000000000 exceeds") {
		t.Fatalf("got %v", err)
	}
	v, err := fn(stdinCtx(strings.NewReader("abc")), []interpreter.Value{interpreter.IntVal(limits.MaxAllocBytes / 4)})
	if err != nil || v.Str != "abc" {
		t.Fatalf("at cap: %q, %v", v.Str, err)
	}
}
