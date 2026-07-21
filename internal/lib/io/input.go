// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package iolib

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// Package-level stdin state. The interpreter is single-instance per
// process today, so this is safe. If multi-interpreter ever lands, move
// these onto the Interpreter struct (or pass through BuiltinCtx).
//
// The buffered reader has to persist across calls because `eof()` may
// Peek a byte that a subsequent `readLine()` needs to see; throwing
// the reader away each call would drop the byte. `eofState` is sticky
// once true so the EOF error message is consistent across repeated
// calls past end-of-input.
var (
	cachedSrc io.Reader
	bufIn     *bufio.Reader
	eofState  bool
)

// resetInputForTest clears the cached reader and EOF flag. Exported via
// the test-only helper in input_test.go.
func resetInputForTest() {
	cachedSrc = nil
	bufIn = nil
	eofState = false
}

// getReader returns the buffered wrapper for `in`, rebuilding it if the
// underlying source has changed since the last call. A change of source
// also clears the sticky EOF flag - tests substitute new stdin contents
// per case.
func getReader(in io.Reader) *bufio.Reader {
	if in == nil {
		return nil
	}
	if in != cachedSrc {
		cachedSrc = in
		bufIn = bufio.NewReader(in)
		eofState = false
	}
	return bufIn
}

// readLine reads one line from stdin and returns it with the trailing
// newline (`\r\n` or `\n`) stripped. With one string argument the prompt
// is written to stdout first. Calling at end-of-input is a positioned
// runtime error - callers should check `eof()` first.
//
// A final line without a trailing newline is returned on the call that
// reaches it; the subsequent call errors.
func readLine(ctx interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if ctx.InREPL {
		return interpreter.Null(), fmt.Errorf("readLine: stdin is owned by the REPL editor")
	}
	if len(args) > 1 {
		return interpreter.Null(), fmt.Errorf("`readLine` takes 0 or 1 argument, got %d", len(args))
	}
	if len(args) == 1 {
		if args[0].Kind != interpreter.KindString {
			return interpreter.Null(), fmt.Errorf("`readLine` prompt must be string, got %s", args[0].Kind)
		}
		if _, err := io.WriteString(ctx.Out, args[0].Str); err != nil {
			return interpreter.Null(), fmt.Errorf("readLine: writing prompt: %v", err)
		}
	}
	r := getReader(ctx.In)
	if r == nil {
		return interpreter.Null(), fmt.Errorf("readLine: no input source")
	}
	if eofState {
		return interpreter.Null(), fmt.Errorf("readLine: end of input")
	}
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return interpreter.Null(), fmt.Errorf("readLine: %v", err)
	}
	if err == io.EOF {
		eofState = true
		if line == "" {
			return interpreter.Null(), fmt.Errorf("readLine: end of input")
		}
	}
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	return interpreter.StringVal(line), nil
}

// readBytes reads exactly `n` bytes from stdin and returns them as a
// `bytes` value. If EOF is hit before `n` bytes are available, the
// partial result is returned and `eof()` becomes true on the next call.
// `n` must be a non-negative int.
func readBytes(ctx interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if ctx.InREPL {
		return interpreter.Null(), fmt.Errorf("readBytes: stdin is owned by the REPL editor")
	}
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("`readBytes` takes 1 argument, got %d", len(args))
	}
	if args[0].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("`readBytes` count must be int, got %s", args[0].Kind)
	}
	n := args[0].Int
	if n < 0 {
		return interpreter.Null(), fmt.Errorf("`readBytes` count must be non-negative, got %d", n)
	}
	r := getReader(ctx.In)
	if r == nil {
		return interpreter.Null(), fmt.Errorf("readBytes: no input source")
	}
	if eofState {
		return interpreter.BytesVal(nil), nil
	}
	// Read in bounded chunks so a huge requested count on a short stream does
	// not pre-allocate gigabytes: the buffer grows only to what the stream
	// actually delivers, capped at n. (A bare make([]byte, n) with an
	// arbitrary n OOMs, and narrows n to a 32-bit int under TinyGo.)
	const chunkSize = 1 << 16 // 64 KiB
	var out []byte
	remaining := n
	for remaining > 0 {
		want := int64(chunkSize)
		if remaining < want {
			want = remaining
		}
		buf := make([]byte, want)
		got, err := io.ReadFull(r, buf)
		out = append(out, buf[:got]...)
		remaining -= int64(got)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			eofState = true
			break
		}
		if err != nil {
			return interpreter.Null(), fmt.Errorf("readBytes: %v", err)
		}
	}
	return interpreter.BytesVal(out), nil
}

// readChars reads exactly `n` Unicode code points from stdin's UTF-8
// stream and returns them as a string. Partial-rune EOF errors are
// surfaced; `eof()` becomes true once the stream is exhausted.
func readChars(ctx interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if ctx.InREPL {
		return interpreter.Null(), fmt.Errorf("readChars: stdin is owned by the REPL editor")
	}
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("`readChars` takes 1 argument, got %d", len(args))
	}
	if args[0].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("`readChars` count must be int, got %s", args[0].Kind)
	}
	n := args[0].Int
	if n < 0 {
		return interpreter.Null(), fmt.Errorf("`readChars` count must be non-negative, got %d", n)
	}
	r := getReader(ctx.In)
	if r == nil {
		return interpreter.Null(), fmt.Errorf("readChars: no input source")
	}
	if eofState {
		return interpreter.StringVal(""), nil
	}
	var b strings.Builder
	for i := int64(0); i < n; i++ {
		ch, size, err := r.ReadRune()
		if err == io.EOF {
			eofState = true
			break
		}
		if err != nil {
			return interpreter.Null(), fmt.Errorf("readChars: %v", err)
		}
		// U+FFFD with size 1 is an invalid byte, not a genuine U+FFFD (size 3).
		// readChars reads text; surface malformed input rather than silently
		// substituting the replacement character.
		if ch == utf8.RuneError && size == 1 {
			return interpreter.Null(), fmt.Errorf("readChars: input is not valid UTF-8")
		}
		b.WriteRune(ch)
	}
	return interpreter.StringVal(b.String()), nil
}

// eofFn reports whether the next `readLine()` would error. Implementation
// peeks one byte through the buffered reader; the byte stays in the
// buffer for the next read. Once true, eofState is sticky for the rest
// of the run.
func eofFn(ctx interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if ctx.InREPL {
		return interpreter.Null(), fmt.Errorf("eof: stdin is owned by the REPL editor")
	}
	if len(args) != 0 {
		return interpreter.Null(), fmt.Errorf("`eof` takes no arguments, got %d", len(args))
	}
	if eofState {
		return interpreter.BoolVal(true), nil
	}
	r := getReader(ctx.In)
	if r == nil {
		return interpreter.BoolVal(true), nil
	}
	_, err := r.Peek(1)
	if err == io.EOF {
		eofState = true
		return interpreter.BoolVal(true), nil
	}
	if err != nil {
		return interpreter.Null(), fmt.Errorf("eof: %v", err)
	}
	return interpreter.BoolVal(false), nil
}
