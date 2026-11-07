// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package iolib

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// TestStdinInvalidUTF8: readLine, readLines and readChars refuse input that
// is not valid UTF-8, naming the function and the byte offset. Before the fix
// readLine and readLines returned the raw bytes as a string.
func TestStdinInvalidUTF8(t *testing.T) {
	in := interpreter.New()
	Install(in)

	resetInputForTest()
	ctx := stdinCtx(strings.NewReader("ok\nab\xffc\nnext\n"))
	if v, err := callReadLine(in, ctx, nil); err != nil || v.Str != "ok" {
		t.Fatalf("first line: %q %v", v.Str, err)
	}
	if _, err := callReadLine(in, ctx, nil); err == nil || err.Error() != "io.readLine: invalid UTF-8 at byte 2" {
		t.Fatalf("readLine: %v", err)
	}
	// The bad line is consumed; the next read continues after it.
	if v, err := callReadLine(in, ctx, nil); err != nil || v.Str != "next" {
		t.Fatalf("after bad line: %q %v", v.Str, err)
	}

	resetInputForTest()
	if _, err := callReadLines(in, stdinCtx(strings.NewReader("é\nab\n\xc3"))); err == nil ||
		err.Error() != "io.readLines: invalid UTF-8 at byte 6 (line 3)" {
		t.Fatalf("readLines: %v", err)
	}

	resetInputForTest()
	readChars := in.LookupNamespacedBuiltin("io", "readChars")
	if _, err := readChars(stdinCtx(strings.NewReader("hé\xffx")), []interpreter.Value{interpreter.IntVal(4)}); err == nil ||
		err.Error() != "io.readChars: invalid UTF-8 at byte 3" {
		t.Fatalf("readChars: %v", err)
	}

	// Valid multi-byte input and a genuine U+FFFD pass.
	resetInputForTest()
	if v, err := callReadLines(in, stdinCtx(strings.NewReader("é€\n�\n"))); err != nil || len(v.List) != 2 {
		t.Fatalf("valid input: %v", err)
	}
}
