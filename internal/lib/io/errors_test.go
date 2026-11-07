// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package iolib

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// TestFieldValueWidthIndependent: a field integer beyond int64 is "exceeds",
// not "not an integer", and a huge negative is rejected as negative rather
// than wrapping when narrowed to a 32-bit int.
func TestFieldValueWidthIndependent(t *testing.T) {
	in := interpreter.New()
	Install(in)
	cases := []struct{ format, want string }{
		{"%s|pad=99999999999999999999", "io.sprintf: 99999999999999999999 exceeds the maximum field value 4096"},
		{"%s|pad=100000000000", "io.sprintf: 100000000000 exceeds the maximum field value 4096"},
		{"%s|pad=-99999999999", "io.sprintf: pad must be >= 0"},
		{"%s|max=-99999999999999999999", "io.sprintf: max must be >= 0"},
		{"%s|pad=1x", `io.sprintf: "1x" is not an integer`},
	}
	for _, c := range cases {
		_, err := callSprintf(in, []interpreter.Value{interpreter.StringVal(c.format), interpreter.StringVal("x")})
		if err == nil || !strings.HasPrefix(err.Error(), c.want) {
			t.Errorf("sprintf(%q): err %v, want prefix %q", c.format, err, c.want)
		}
	}
}

// TestFormatErrorsNameFunction: every printf-family error names the function.
func TestFormatErrorsNameFunction(t *testing.T) {
	in := interpreter.New()
	Install(in)
	var sink strings.Builder
	if _, err := callPrintf(in, &sink, nil); err == nil || err.Error() != "io.printf: expects at least 1 argument" {
		t.Errorf("printf(): %v", err)
	}
	if _, err := callEprintf(in, &sink, []interpreter.Value{interpreter.StringVal("%q"), interpreter.IntVal(1)}); err == nil || err.Error() != "io.eprintf: unknown format verb `%q`" {
		t.Errorf("eprintf: %v", err)
	}
	if _, err := callSprintf(in, []interpreter.Value{interpreter.IntVal(1), interpreter.IntVal(2)}); err == nil || !strings.HasPrefix(err.Error(), "io.sprintf: with multiple arguments") {
		t.Errorf("sprintf: %v", err)
	}
}

// TestInputErrorsNameFunction: stdin errors carry the io. prefix.
func TestInputErrorsNameFunction(t *testing.T) {
	in := interpreter.New()
	Install(in)
	resetInputForTest()
	ctx := stdinCtx(strings.NewReader(""))
	if _, err := in.LookupNamespacedBuiltin("io", "readBytes")(ctx, []interpreter.Value{interpreter.StringVal("x")}); err == nil || err.Error() != "io.readBytes: count must be int, got string" {
		t.Errorf("readBytes: %v", err)
	}
	if _, err := callReadLine(in, ctx, nil); err == nil || err.Error() != "io.readLine: end of input" {
		t.Errorf("readLine: %v", err)
	}
	if _, err := callEof(in, interpreter.BuiltinCtx{InREPL: true}); err == nil || err.Error() != "io.eof: stdin is owned by the REPL editor" {
		t.Errorf("eof: %v", err)
	}
}

// TestReadBytesCountCap: n over maxReadBytes is an error before any read, like
// fs.readBytes($f, n); the pre-fix code read the stream up to n.
func TestReadBytesCountCap(t *testing.T) {
	in := interpreter.New()
	Install(in)
	resetInputForTest()
	ctx := stdinCtx(strings.NewReader("abc"))
	readBytes := in.LookupNamespacedBuiltin("io", "readBytes")
	_, err := readBytes(ctx, []interpreter.Value{interpreter.IntVal(maxReadBytes + 1)})
	if err == nil || !strings.HasPrefix(err.Error(), "io.readBytes: count ") || !strings.Contains(err.Error(), "per-call limit") {
		t.Fatalf("err %v", err)
	}
	v, err := readBytes(ctx, []interpreter.Value{interpreter.IntVal(maxReadBytes)})
	if err != nil || string(v.Bytes) != "abc" {
		t.Fatalf("readBytes(max) = %q, %v", v.Bytes, err)
	}
}

// TestParseFormatSpecNoAlloc: the duplicate-modifier set does not allocate.
func TestParseFormatSpecNoAlloc(t *testing.T) {
	src := "%d|pad=6|sign=always"
	if _, _, err := parseFormatSpec('d', src, 2); err != nil {
		t.Fatal(err)
	}
	if _, _, err := parseFormatSpec('d', "%d|pad=6|pad=7", 2); err == nil || !strings.Contains(err.Error(), `modifier "pad" specified twice`) {
		t.Fatalf("duplicate: %v", err)
	}
	if n := testing.AllocsPerRun(100, func() { _, _, _ = parseFormatSpec('d', src, 2) }); n != 0 {
		t.Errorf("parseFormatSpec: %v allocs/op, want 0", n)
	}
}
