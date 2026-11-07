// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package iolib

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"unicode/utf8"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// inputMu guards the shared stdin reader state below. The stdin reader is one
// shared resource, so two spawned tasks reading concurrently (io.readLine /
// readBytes / readChars / readLines / eof) must serialise or they tear the
// buffer and the eof flag. Holding it across the (blocking) read serialises
// readers, which is the intended behaviour for one shared stream. None of the
// guarded functions calls another, so a plain mutex cannot self-deadlock.
var inputMu sync.Mutex

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

// maxLineBytes caps one readLine line (terminator included) and the whole
// input readLines slurps, so stdin without newlines, or an endless stream, is
// a catchable error rather than an unbounded buffer.
const maxLineBytes = limits.MaxAllocBytes

// maxLines caps how many lines readLines returns: each costs about 1 KB of
// heap at the peak (its Value plus list growth), so a stream of empty lines
// would otherwise cost ~1000x its size. A quarter of the range cap is ~1 GiB.
const maxLines = limits.MaxRangeElements / 4

// maxReadBytes caps readBytes' count, like fs.readBytes($f, n).
const maxReadBytes = limits.MaxAllocBytes

// maxReadChars caps readChars' count: at up to 4 bytes per character the
// result stays within limits.MaxAllocBytes.
const maxReadChars = limits.MaxAllocBytes / 4

// readCappedLine reads through the next '\n', at most maxLineBytes bytes.
func readCappedLine(r *bufio.Reader) (string, error) {
	b, err := limits.ReadDelim(r, '\n', maxLineBytes)
	return string(b), err
}

// stripLineEnding removes the trailing line terminator, treating `\r` as part
// of it only when it precedes an `\n` (matching Go's bufio.ScanLines). A bare
// trailing `\r` on an unterminated final line is data, not a terminator, so it
// is preserved rather than silently dropped.
func stripLineEnding(line string) string {
	if strings.HasSuffix(line, "\n") {
		line = line[:len(line)-1]
		line = strings.TrimSuffix(line, "\r")
	}
	return line
}

// readLine reads one line from stdin and returns it with the trailing
// newline (`\r\n` or `\n`) stripped. With one string argument the prompt
// is written to stdout first. Calling at end-of-input is a positioned
// runtime error - callers should check `eof()` first.
//
// A final line without a trailing newline is returned on the call that
// reaches it; the subsequent call errors. A line that is not valid UTF-8 is
// consumed and reported (byte offset within the line); read binary input with
// readBytes.
func readLine(ctx interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if ctx.InREPL {
		return interpreter.Null(), fmt.Errorf("io.readLine: stdin is owned by the REPL editor")
	}
	if len(args) > 1 {
		return interpreter.Null(), fmt.Errorf("io.readLine: takes 0 or 1 argument, got %d", len(args))
	}
	if len(args) == 1 {
		if args[0].Kind != interpreter.KindString {
			return interpreter.Null(), fmt.Errorf("io.readLine: prompt must be string, got %s", args[0].Kind)
		}
		if _, err := io.WriteString(ctx.Out, args[0].Str); err != nil {
			return interpreter.Null(), fmt.Errorf("io.readLine: writing prompt: %v", err)
		}
	}
	inputMu.Lock()
	defer inputMu.Unlock()
	r := getReader(ctx.In)
	if r == nil {
		return interpreter.Null(), fmt.Errorf("io.readLine: no input source")
	}
	if eofState {
		return interpreter.Null(), fmt.Errorf("io.readLine: end of input")
	}
	line, err := readCappedLine(r)
	if errors.Is(err, limits.ErrTooLarge) {
		return interpreter.Null(), fmt.Errorf("io.readLine: line exceeds the %d-byte limit", maxLineBytes)
	}
	if err != nil && err != io.EOF {
		return interpreter.Null(), fmt.Errorf("io.readLine: %v", err)
	}
	if err == io.EOF {
		eofState = true
		if line == "" {
			return interpreter.Null(), fmt.Errorf("io.readLine: end of input")
		}
	}
	if err := limits.CheckUTF8("io.readLine", line); err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(stripLineEnding(line)), nil
}

// readBytes reads exactly `n` bytes from stdin and returns them as a
// `bytes` value. If EOF is hit before `n` bytes are available, the
// partial result is returned and `eof()` becomes true on the next call.
// `n` must be a non-negative int, at most maxReadBytes.
func readBytes(ctx interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if ctx.InREPL {
		return interpreter.Null(), fmt.Errorf("io.readBytes: stdin is owned by the REPL editor")
	}
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("io.readBytes: takes 1 argument, got %d", len(args))
	}
	if args[0].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("io.readBytes: count must be int, got %s", args[0].Kind)
	}
	n := args[0].Int
	if n < 0 {
		return interpreter.Null(), fmt.Errorf("io.readBytes: count must be non-negative, got %d", n)
	}
	if n > maxReadBytes {
		return interpreter.Null(), fmt.Errorf("io.readBytes: count %d exceeds the %d-byte per-call limit", n, maxReadBytes)
	}
	inputMu.Lock()
	defer inputMu.Unlock()
	r := getReader(ctx.In)
	if r == nil {
		return interpreter.Null(), fmt.Errorf("io.readBytes: no input source")
	}
	if eofState {
		return interpreter.BytesVal(nil), nil
	}
	// ReadN grows its buffer with the data, so a large n on a short stream
	// allocates only what arrives.
	out, err := limits.ReadN(r, n, nil)
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		eofState = true
	} else if err != nil {
		return interpreter.Null(), fmt.Errorf("io.readBytes: %v", err)
	}
	return interpreter.BytesVal(out), nil
}

// readChars reads exactly `n` Unicode code points from stdin's UTF-8
// stream and returns them as a string. An invalid byte (also a partial rune
// at EOF) is an error naming its offset within this call's read; the bytes up
// to it are consumed. `eof()` becomes true once the stream is exhausted.
func readChars(ctx interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if ctx.InREPL {
		return interpreter.Null(), fmt.Errorf("io.readChars: stdin is owned by the REPL editor")
	}
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("io.readChars: takes 1 argument, got %d", len(args))
	}
	if args[0].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("io.readChars: count must be int, got %s", args[0].Kind)
	}
	n := args[0].Int
	if n < 0 {
		return interpreter.Null(), fmt.Errorf("io.readChars: count must be non-negative, got %d", n)
	}
	if n > maxReadChars {
		return interpreter.Null(), fmt.Errorf("io.readChars: count %d exceeds the %d-character per-call limit", n, maxReadChars)
	}
	inputMu.Lock()
	defer inputMu.Unlock()
	r := getReader(ctx.In)
	if r == nil {
		return interpreter.Null(), fmt.Errorf("io.readChars: no input source")
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
			return interpreter.Null(), fmt.Errorf("io.readChars: %v", err)
		}
		// U+FFFD with size 1 is an invalid byte, not a genuine U+FFFD (size 3).
		// readChars reads text; surface malformed input rather than silently
		// substituting the replacement character.
		if ch == utf8.RuneError && size == 1 {
			return interpreter.Null(), fmt.Errorf("io.readChars: invalid UTF-8 at byte %d", b.Len())
		}
		b.WriteRune(ch)
	}
	return interpreter.StringVal(b.String()), nil
}

// readLines reads every remaining line from stdin and returns them as a
// `list of string`, each with its trailing newline stripped - the slurp
// counterpart to the streaming `readLine()` + `eof()` loop, for tiny filter
// scripts over bounded input. Splitting is OS-independent, exactly like
// readLine: it breaks on `\n` and strips a trailing `\r`, so LF and CRLF input
// read identically on any platform. Empty stdin yields an empty list; a blank
// line is preserved as ""; a final line without a trailing newline is included.
// It reads the whole input into memory, so use the `readLine()` loop for large
// or unbounded streams. Shares stdin state with `readLine`/`readBytes`, so it
// returns only the lines not already consumed. Input that is not valid UTF-8
// is an error naming the byte offset within this call's read and the line.
func readLines(ctx interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if ctx.InREPL {
		return interpreter.Null(), fmt.Errorf("io.readLines: stdin is owned by the REPL editor")
	}
	if len(args) != 0 {
		return interpreter.Null(), fmt.Errorf("io.readLines: takes no arguments, got %d", len(args))
	}
	inputMu.Lock()
	defer inputMu.Unlock()
	r := getReader(ctx.In)
	if r == nil {
		return interpreter.Null(), fmt.Errorf("io.readLines: no input source")
	}
	var out []interpreter.Value
	total := 0
	for !eofState {
		line, err := readCappedLine(r)
		if errors.Is(err, limits.ErrTooLarge) {
			return interpreter.Null(), fmt.Errorf("io.readLines: a line exceeds the %d-byte limit", maxLineBytes)
		}
		if err != nil && err != io.EOF {
			return interpreter.Null(), fmt.Errorf("io.readLines: %v", err)
		}
		total += len(line)
		if total > maxLineBytes {
			return interpreter.Null(), fmt.Errorf("io.readLines: input exceeds the %d-byte limit; use a readLine() loop", maxLineBytes)
		}
		if err == io.EOF {
			eofState = true
			if line == "" {
				break
			}
		}
		if len(out) >= maxLines {
			return interpreter.Null(), fmt.Errorf("io.readLines: input exceeds the %d-line limit; use a readLine() loop", maxLines)
		}
		if off := limits.InvalidUTF8At(line); off >= 0 {
			return interpreter.Null(), fmt.Errorf("io.readLines: invalid UTF-8 at byte %d (line %d)", total-len(line)+off, len(out)+1)
		}
		out = append(out, interpreter.StringVal(stripLineEnding(line)))
	}
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeString), out), nil
}

// eofFn reports whether the next `readLine()` would error. Implementation
// peeks one byte through the buffered reader; the byte stays in the
// buffer for the next read. Once true, eofState is sticky for the rest
// of the run.
func eofFn(ctx interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if ctx.InREPL {
		return interpreter.Null(), fmt.Errorf("io.eof: stdin is owned by the REPL editor")
	}
	if len(args) != 0 {
		return interpreter.Null(), fmt.Errorf("io.eof: takes no arguments, got %d", len(args))
	}
	inputMu.Lock()
	defer inputMu.Unlock()
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
		return interpreter.Null(), fmt.Errorf("io.eof: %v", err)
	}
	return interpreter.BoolVal(false), nil
}
