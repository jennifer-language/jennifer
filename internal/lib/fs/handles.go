// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package fslib

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/mplx/jennifer-lang/internal/interpreter"
)

// maxHandleRead caps a single caller-requested fs.readBytes so a huge `n` cannot
// force a multi-gigabyte up-front allocation. Read in chunks for more.
const maxHandleRead = 256 << 20

// handleState holds one live file's read/write state. Line-oriented
// reads go through a bufio.Reader; writes go direct. `mode` records
// the open mode so read ops on a write-mode handle (and vice versa)
// error at the boundary rather than crashing at the syscall.
type handleState struct {
	f      *os.File
	reader *bufio.Reader // populated for read/append; nil for write-only
	mode   string        // "read" | "write" | "append"
	sticky bool          // sticky EOF flag; matches io.eof semantics
	path   string        // for error messages
}

// The registry: integer id -> live state. Guarded by handlesMu so
// concurrent spawn tasks can share the map safely (though sharing a
// handle across spawns is discouraged; each spawn should open its own).
var (
	handlesMu sync.Mutex
	handles   = map[int64]*handleState{}
	nextID    int64
)

// ResetForTest lets tests wipe the handle registry between runs so
// leaked handles from an earlier test don't leak ids into the next.
// Exported so the `_test` package can drive it; not part of the
// user-facing library surface.
func ResetForTest() {
	handlesMu.Lock()
	defer handlesMu.Unlock()
	for _, h := range handles {
		if h != nil && h.f != nil {
			_ = h.f.Close()
		}
	}
	handles = map[int64]*handleState{}
	nextID = 0
}

func installHandles(in *interpreter.Interpreter) {
	in.RegisterNamespaced(LibraryName, "open", openFn)
	in.RegisterNamespaced(LibraryName, "close", closeFn)
	in.RegisterNamespaced(LibraryName, "readLine", readLineFn)
	in.RegisterNamespaced(LibraryName, "readChars", readCharsFn)
	in.RegisterNamespaced(LibraryName, "readBytes", readBytesDispatchFn)
	in.RegisterNamespaced(LibraryName, "writeString", writeHandleStringFn)
	in.RegisterNamespaced(LibraryName, "writeBytes", writeHandleBytesFn)
	in.RegisterNamespaced(LibraryName, "eof", eofFn)
}

// makeFile builds the Jennifer-side `fs.File{id}` value.
func makeFile(id int64) Value {
	return interpreter.NamespacedStructVal(LibraryName, "File", []interpreter.StructField{
		{Name: "id", Value: interpreter.IntVal(id)},
	})
}

// extractFileID pulls the integer id out of an `fs.File{...}` value.
// Every handle-consuming builtin routes through this to keep the
// boundary errors uniform.
func extractFileID(fnName string, v Value) (int64, error) {
	if v.Kind != interpreter.KindStruct || v.StructNS != LibraryName || v.StructName != "File" {
		return 0, fmt.Errorf("%s: argument must be an fs.File, got %s", fnName, v.Kind)
	}
	for _, f := range v.Fields {
		if f.Name == "id" {
			if f.Value.Kind != interpreter.KindInt {
				return 0, fmt.Errorf("%s: fs.File.id is not int (got %s)", fnName, f.Value.Kind)
			}
			return f.Value.Int, nil
		}
	}
	return 0, fmt.Errorf("%s: fs.File has no id field", fnName)
}

// resolveHandle returns the live state for an id, or a positioned
// error if the id is unknown (typical cause: use after close).
func resolveHandle(fnName string, id int64) (*handleState, error) {
	handlesMu.Lock()
	defer handlesMu.Unlock()
	h, ok := handles[id]
	if !ok {
		return nil, fmt.Errorf("%s: fs.File id %d is not open (already closed, or never opened)", fnName, id)
	}
	return h, nil
}

// -------- open / close --------

func openFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("fs.open expects 2 arguments (path, mode), got %d", len(args))
	}
	path, err := takeStringArg("fs.open", args, 0, "path")
	if err != nil {
		return interpreter.Null(), err
	}
	mode, err := takeStringArg("fs.open", args, 1, "mode")
	if err != nil {
		return interpreter.Null(), err
	}
	var flag int
	switch mode {
	case "read":
		flag = os.O_RDONLY
	case "write":
		flag = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	case "append":
		flag = os.O_WRONLY | os.O_CREATE | os.O_APPEND
	default:
		return interpreter.Null(), fmt.Errorf(`fs.open: unknown mode %q; known: "read", "write", "append"`, mode)
	}
	f, opErr := os.OpenFile(path, flag, 0o644)
	if opErr != nil {
		return interpreter.Null(), fmt.Errorf("fs.open: %s: %v", path, opErr)
	}
	state := &handleState{
		f:    f,
		mode: mode,
		path: path,
	}
	if mode == "read" {
		state.reader = bufio.NewReader(f)
	}
	handlesMu.Lock()
	nextID++
	id := nextID
	handles[id] = state
	handlesMu.Unlock()
	return makeFile(id), nil
}

func closeFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("fs.close expects 1 argument (fs.File), got %d", len(args))
	}
	id, err := extractFileID("fs.close", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	handlesMu.Lock()
	h, ok := handles[id]
	if !ok {
		handlesMu.Unlock()
		return interpreter.Null(), fmt.Errorf("fs.close: fs.File id %d is not open (already closed?)", id)
	}
	delete(handles, id)
	handlesMu.Unlock()
	if h.f != nil {
		if cerr := h.f.Close(); cerr != nil {
			return interpreter.Null(), fmt.Errorf("fs.close: %s: %v", h.path, cerr)
		}
	}
	return interpreter.Null(), nil
}

// -------- reads --------

// requireReadMode is the boundary check for the read-side handle ops.
// Write and append modes are rejected; the friendly message points at
// the corrective open call.
func requireReadMode(fnName string, h *handleState) error {
	if h.mode != "read" {
		return fmt.Errorf(`%s: fs.File %q was opened in mode %q; open with mode "read" to read`, fnName, h.path, h.mode)
	}
	return nil
}

func readLineFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("fs.readLine expects 1 argument (fs.File), got %d", len(args))
	}
	id, err := extractFileID("fs.readLine", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	h, err := resolveHandle("fs.readLine", id)
	if err != nil {
		return interpreter.Null(), err
	}
	if err := requireReadMode("fs.readLine", h); err != nil {
		return interpreter.Null(), err
	}
	if h.sticky {
		return interpreter.Null(), fmt.Errorf("fs.readLine: %s: end of input", h.path)
	}
	line, rerr := h.reader.ReadString('\n')
	if rerr != nil && !errors.Is(rerr, io.EOF) {
		return interpreter.Null(), fmt.Errorf("fs.readLine: %s: %v", h.path, rerr)
	}
	if errors.Is(rerr, io.EOF) {
		h.sticky = true
		if line == "" {
			return interpreter.Null(), fmt.Errorf("fs.readLine: %s: end of input", h.path)
		}
		// Trailing unterminated line: return normally; next call errors.
		return interpreter.StringVal(stripTrailingNewline(line)), nil
	}
	return interpreter.StringVal(stripTrailingNewline(line)), nil
}

// stripTrailingNewline handles both LF and CRLF terminators.
func stripTrailingNewline(s string) string {
	s = strings.TrimSuffix(s, "\n")
	s = strings.TrimSuffix(s, "\r")
	return s
}

func readCharsFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("fs.readChars expects 2 arguments (fs.File, n), got %d", len(args))
	}
	id, err := extractFileID("fs.readChars", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("fs.readChars: n must be int, got %s", args[1].Kind)
	}
	n := args[1].Int
	if n < 0 {
		return interpreter.Null(), fmt.Errorf("fs.readChars: n must be non-negative, got %d", n)
	}
	h, err := resolveHandle("fs.readChars", id)
	if err != nil {
		return interpreter.Null(), err
	}
	if err := requireReadMode("fs.readChars", h); err != nil {
		return interpreter.Null(), err
	}
	var sb strings.Builder
	for i := int64(0); i < n; i++ {
		r, size, rerr := h.reader.ReadRune()
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				h.sticky = true
				break
			}
			return interpreter.Null(), fmt.Errorf("fs.readChars: %s: %v", h.path, rerr)
		}
		// U+FFFD with size 1 is an invalid byte; a genuine U+FFFD in the file
		// decodes with size 3 and is valid content.
		if r == utf8.RuneError && size == 1 {
			return interpreter.Null(), fmt.Errorf("fs.readChars: %s: not valid UTF-8", h.path)
		}
		sb.WriteRune(r)
	}
	return interpreter.StringVal(sb.String()), nil
}

// readBytesDispatchFn routes `fs.readBytes` to either the one-shot
// whole-file read (fslib.go: readBytesFn) or the handle-based partial
// read below, depending on the argument shape. Arity-1 with a string
// arg is the whole-file form; arity-2 with an fs.File then an int is
// the handle form. Anything else surfaces at the boundary.
func readBytesDispatchFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) == 1 {
		return readBytesFn(ctx, args)
	}
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("fs.readBytes expects (path) for whole file or (fs.File, n) for partial read; got %d arguments", len(args))
	}
	id, err := extractFileID("fs.readBytes", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("fs.readBytes: n must be int, got %s", args[1].Kind)
	}
	n := args[1].Int
	if n < 0 {
		return interpreter.Null(), fmt.Errorf("fs.readBytes: n must be non-negative, got %d", n)
	}
	if n > maxHandleRead {
		return interpreter.Null(), fmt.Errorf("fs.readBytes: %d exceeds the %d-byte per-call limit", n, maxHandleRead)
	}
	h, err := resolveHandle("fs.readBytes", id)
	if err != nil {
		return interpreter.Null(), err
	}
	if err := requireReadMode("fs.readBytes", h); err != nil {
		return interpreter.Null(), err
	}
	buf := make([]byte, n)
	read, rerr := io.ReadFull(h.reader, buf)
	if rerr != nil {
		if errors.Is(rerr, io.EOF) || errors.Is(rerr, io.ErrUnexpectedEOF) {
			h.sticky = true
			return interpreter.BytesVal(buf[:read]), nil
		}
		return interpreter.Null(), fmt.Errorf("fs.readBytes: %s: %v", h.path, rerr)
	}
	return interpreter.BytesVal(buf), nil
}

// -------- writes --------

// requireWriteMode is the boundary check for the write-side handle ops.
func requireWriteMode(fnName string, h *handleState) error {
	if h.mode != "write" && h.mode != "append" {
		return fmt.Errorf(`%s: fs.File %q was opened in mode %q; open with mode "write" or "append" to write`, fnName, h.path, h.mode)
	}
	return nil
}

func writeHandleStringFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("fs.writeString expects 2 arguments (fs.File or path, content), got %d", len(args))
	}
	// Disambiguate: the one-shot form takes (path, content); the
	// handle form takes (fs.File, content). Route by the first arg's
	// kind so both stay under the same name at the call site.
	if args[0].Kind == interpreter.KindString {
		return writeStringFn(interpreter.BuiltinCtx{}, args)
	}
	id, err := extractFileID("fs.writeString", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("fs.writeString: content must be string, got %s", args[1].Kind)
	}
	h, err := resolveHandle("fs.writeString", id)
	if err != nil {
		return interpreter.Null(), err
	}
	if err := requireWriteMode("fs.writeString", h); err != nil {
		return interpreter.Null(), err
	}
	if _, werr := h.f.WriteString(args[1].Str); werr != nil {
		return interpreter.Null(), fmt.Errorf("fs.writeString: %s: %v", h.path, werr)
	}
	return interpreter.Null(), nil
}

func writeHandleBytesFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("fs.writeBytes expects 2 arguments (fs.File or path, content), got %d", len(args))
	}
	if args[0].Kind == interpreter.KindString {
		return writeBytesFn(interpreter.BuiltinCtx{}, args)
	}
	id, err := extractFileID("fs.writeBytes", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("fs.writeBytes: content must be bytes, got %s", args[1].Kind)
	}
	h, err := resolveHandle("fs.writeBytes", id)
	if err != nil {
		return interpreter.Null(), err
	}
	if err := requireWriteMode("fs.writeBytes", h); err != nil {
		return interpreter.Null(), err
	}
	if _, werr := h.f.Write(args[1].Bytes); werr != nil {
		return interpreter.Null(), fmt.Errorf("fs.writeBytes: %s: %v", h.path, werr)
	}
	return interpreter.Null(), nil
}

func eofFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("fs.eof expects 1 argument (fs.File), got %d", len(args))
	}
	id, err := extractFileID("fs.eof", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	h, err := resolveHandle("fs.eof", id)
	if err != nil {
		return interpreter.Null(), err
	}
	if h.sticky {
		return interpreter.BoolVal(true), nil
	}
	// Peek without consuming to see whether the next read would find
	// anything. When the previous read completed exactly at a newline
	// boundary the sticky flag is still false, but the underlying
	// stream has nothing left; the canonical `while (not fs.eof($f))`
	// loop needs this to be visible so it terminates.
	if h.reader != nil {
		if _, peekErr := h.reader.Peek(1); errors.Is(peekErr, io.EOF) {
			h.sticky = true
			return interpreter.BoolVal(true), nil
		}
	}
	return interpreter.BoolVal(false), nil
}
