// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package oslib

import (
	"strings"
	"testing"
	"unicode/utf8"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// TestRunWaitInvalidUTF8: captured stdout / stderr that is not valid UTF-8
// is an error naming the function, the stream and the byte offset. Before the
// fix the raw bytes came back as a string.
func TestRunWaitInvalidUTF8(t *testing.T) {
	skipIfNotLinux(t)
	_, err := runFn(interpreter.BuiltinCtx{}, []interpreter.Value{
		stringList("/bin/cat"), interpreter.BytesVal([]byte("ab\xffc"))})
	if err == nil || err.Error() != "os.run: stdout: invalid UTF-8 at byte 2" {
		t.Fatalf("os.run stdout: %v", err)
	}
	_, err = runFn(interpreter.BuiltinCtx{}, []interpreter.Value{
		stringList("/bin/sh", "-c", `printf 'x\376' >&2; exit 3`)})
	if err == nil || err.Error() != "os.run: stderr: invalid UTF-8 at byte 1" {
		t.Fatalf("os.run stderr: %v", err)
	}
	p, err := spawnFn(interpreter.BuiltinCtx{}, []interpreter.Value{stringList("/bin/sh", "-c", `printf '\303'`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := waitFn(interpreter.BuiltinCtx{}, []interpreter.Value{p}); err == nil ||
		err.Error() != "os.wait: stdout: invalid UTF-8 at byte 0" {
		t.Fatalf("os.wait: %v", err)
	}
	_, _ = releaseFn(interpreter.BuiltinCtx{}, []interpreter.Value{p})
}

// TestCapBufferKeepsRunes: the 16 MiB cap drops a rune it splits instead of
// leaving a partial sequence (which the UTF-8 check would then refuse).
func TestCapBufferKeepsRunes(t *testing.T) {
	var c capBuffer
	_, _ = c.Write([]byte(strings.Repeat("a", maxCaptureBytes-1)))
	_, _ = c.Write([]byte("€tail")) // 3-byte rune split after its first byte
	s := c.String()
	if !utf8.ValidString(s) || !strings.HasSuffix(s, "a\n[output truncated at 16 MiB]") {
		t.Fatalf("truncated output: valid=%v suffix=%q", utf8.ValidString(s), s[len(s)-40:])
	}
	var d capBuffer
	_, _ = d.Write([]byte(strings.Repeat("a", maxCaptureBytes-3) + "€x"))
	if s := d.String(); !strings.HasSuffix(s, "a€\n[output truncated at 16 MiB]") {
		t.Fatalf("whole rune at the cap dropped: %q", s[len(s)-40:])
	}
}
