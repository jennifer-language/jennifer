// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package termlib

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/term"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func TestReadByte(t *testing.T) {
	ctx := interpreter.BuiltinCtx{In: strings.NewReader("AB")}
	for _, want := range []int64{'A', 'B', gotClosed, gotClosed} {
		if got := readOne(t, ctx); got != want {
			t.Errorf("readByte = %d, want %d", got, want)
		}
	}
	// No input reader at all is the end of input too.
	if got := readOne(t, interpreter.BuiltinCtx{}); got != gotClosed {
		t.Errorf("readByte without input = %d, want closed", got)
	}
	if got := readTimeout(t, interpreter.BuiltinCtx{}, 0); got != gotClosed {
		t.Errorf("readByteTimeout without input = %d, want closed", got)
	}
}

// TestReadByteBulk drains a large stream one byte at a time. The shared stdin
// layer reads one bounded chunk ahead on demand, so memory is flat regardless of
// input size; this confirms it returns every byte in order and terminates
// cleanly with "closed" (no hang, no accumulation).
func TestReadByteBulk(t *testing.T) {
	const n = 100000
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = byte(i * 7)
	}
	ctx := interpreter.BuiltinCtx{In: strings.NewReader(string(buf))}
	for i := 0; i < n; i++ {
		v, err := readByteFn(ctx, nil)
		if err != nil {
			t.Fatalf("readByte[%d]: %v", i, err)
		}
		if v.Int() != int64(buf[i]) {
			t.Fatalf("readByte[%d] = %d, want %d", i, v.Int(), buf[i])
		}
	}
	// Exactly drained: the next read throws "closed", and keeps throwing it.
	for k := 0; k < 3; k++ {
		if got := readOne(t, ctx); got != gotClosed {
			t.Fatalf("post-drain read = %d, want closed", got)
		}
	}
}

func TestReadByteRefusedInREPL(t *testing.T) {
	ctx := interpreter.BuiltinCtx{In: strings.NewReader("A"), InREPL: true}
	if _, err := readByteFn(ctx, nil); err == nil {
		t.Error("readByte in the REPL should error")
	}
}

func TestReadByteArity(t *testing.T) {
	ctx := interpreter.BuiltinCtx{In: strings.NewReader("A")}
	if _, err := readByteFn(ctx, []Value{interpreter.StringVal("x")}); err == nil {
		t.Error("readByte with an argument should error")
	}
}

func TestMakeRawRefusedInREPL(t *testing.T) {
	ctx := interpreter.BuiltinCtx{InREPL: true}
	if _, err := makeRawFn(ctx, []Value{interpreter.StringVal("stdin")}); err == nil {
		t.Error("makeRaw in the REPL should error")
	}
}

func TestUnknownStream(t *testing.T) {
	for _, fn := range []func(interpreter.BuiltinCtx, []Value) (Value, error){makeRawFn, sizeFn} {
		_, err := fn(interpreter.BuiltinCtx{}, []Value{interpreter.StringVal("nope")})
		if err == nil || !strings.Contains(err.Error(), "unknown stream") {
			t.Errorf("expected an unknown-stream error, got %v", err)
		}
	}
}

func TestStreamArity(t *testing.T) {
	if _, err := sizeFn(interpreter.BuiltinCtx{}, nil); err == nil {
		t.Error("size with no arguments should error")
	}
	if _, err := makeRawFn(interpreter.BuiltinCtx{}, []Value{interpreter.IntVal(1)}); err == nil {
		t.Error("makeRaw with a non-string stream should error")
	}
}

func TestRestoreInvalid(t *testing.T) {
	ResetForTest()
	// A non-State value is rejected.
	if _, err := restoreFn(interpreter.BuiltinCtx{}, []Value{interpreter.IntVal(1)}); err == nil {
		t.Error("restore of a non-State value should error")
	}
	// A State already restored (or never live): a second restore does
	// nothing (M29.46).
	stale := interpreter.NamespacedStructVal(LibraryName, "State", []interpreter.StructField{
		{Name: "id", Value: interpreter.IntVal(999)},
	})
	if _, err := restoreFn(interpreter.BuiltinCtx{}, []Value{stale}); err != nil {
		t.Errorf("restore of a restored State: %v, want no error", err)
	}
}

// TestMakeRawNonTTY exercises the real MakeRaw failure path only when stdin is
// not an interactive terminal (CI, a pipe) - never on a developer's live
// terminal, which raw mode would disrupt.
func TestMakeRawNonTTY(t *testing.T) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		t.Skip("stdin is an interactive terminal; skipping to avoid disrupting it")
	}
	if _, err := makeRawFn(interpreter.BuiltinCtx{}, []Value{interpreter.StringVal("stdin")}); err == nil {
		t.Error("makeRaw on a non-terminal stdin should error")
	}
}

// TestSize accepts either outcome: on a real terminal it returns a well-formed
// term.Size (querying size does not change terminal state); off one it errors.
func TestSize(t *testing.T) {
	v, err := sizeFn(interpreter.BuiltinCtx{}, []Value{interpreter.StringVal("stdout")})
	if err != nil {
		if !strings.Contains(err.Error(), "not a terminal") {
			t.Errorf("size error = %v, want a not-a-terminal message", err)
		}
		return
	}
	if v.Kind != interpreter.KindStruct || v.StructName() != "Size" {
		t.Fatalf("size = %v, want a term.Size struct", v)
	}
	var haveRows, haveCols bool
	for _, f := range v.Fields() {
		switch f.Name {
		case "rows":
			haveRows = true
		case "cols":
			haveCols = true
		}
	}
	if !haveRows || !haveCols {
		t.Errorf("term.Size missing rows/cols: %v", v.Fields())
	}
}

// makeRaw is a stdin concept; stdout / stderr (valid streams for size) are
// refused so the terminal is not reconfigured through the wrong fd.
func TestMakeRawStdinOnly(t *testing.T) {
	for _, s := range []string{"stdout", "stderr"} {
		_, err := makeRawFn(interpreter.BuiltinCtx{}, []Value{interpreter.StringVal(s)})
		if err == nil || !strings.Contains(err.Error(), "stdin") {
			t.Errorf("makeRaw(%q) should be refused with a stdin hint, got %v", s, err)
		}
	}
}

// A leaked registry (makeRaw without restore) is bounded: past the cap makeRaw
// returns a catchable error rather than growing the map without limit. The cap
// is checked before term.MakeRaw, so this holds without a real terminal.
func TestMakeRawRegistryCap(t *testing.T) {
	ResetForTest()
	defer ResetForTest()
	statesMu.Lock()
	for i := 0; i < maxRawStates; i++ {
		states[i] = &rawEntry{}
	}
	statesMu.Unlock()
	_, err := makeRawFn(interpreter.BuiltinCtx{}, []Value{interpreter.StringVal("stdin")})
	if err == nil || !strings.Contains(err.Error(), "too many active raw-mode states") {
		t.Errorf("makeRaw past the cap should error, got %v", err)
	}
}

// RestoreAll (the CLI teardown net) empties the registry and never panics, even
// on an already-empty registry. It restores real raw-mode sessions in the CLI's
// deferred cleanup; here we assert the bookkeeping (a real term.Restore needs a
// TTY, exercised end-to-end elsewhere).
func TestRestoreAllClears(t *testing.T) {
	ResetForTest()
	RestoreAll() // empty registry: must not panic
	statesMu.Lock()
	states[1] = &rawEntry{} // a placeholder entry (nil prev -> skipped, not restored)
	states[2] = &rawEntry{}
	statesMu.Unlock()
	RestoreAll()
	statesMu.Lock()
	n := len(states)
	statesMu.Unlock()
	if n != 0 {
		t.Errorf("RestoreAll left %d entries, want 0", n)
	}
}

// A reader may legally return (0, nil) - which is NOT end-of-input; readByte
// must retry it and deliver the next byte, and throw "closed" only at a real EOF.
type stutterReader struct{ steps []string }

func (r *stutterReader) Read(p []byte) (int, error) {
	if len(r.steps) == 0 {
		return 0, io.EOF
	}
	s := r.steps[0]
	r.steps = r.steps[1:]
	if s == "" { // a (0, nil) "nothing happened" read
		return 0, nil
	}
	p[0] = s[0]
	return 1, nil
}

func TestReadByteRetriesEmptyRead(t *testing.T) {
	ctx := interpreter.BuiltinCtx{In: &stutterReader{steps: []string{"", "", "A"}}}
	v, err := readByteFn(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if v.Int() != int64('A') {
		t.Errorf("readByte after empty reads = %d, want %d ('A')", v.Int(), 'A')
	}
	// After the steps are exhausted, a real EOF throws "closed".
	if got := readOne(t, ctx); got != gotClosed {
		t.Errorf("readByte at EOF = %d, want closed", got)
	}
}

// gotTimeout and gotClosed stand for the "timeout" and "closed" errors in the
// read helpers' results, so a test reads as a sequence of outcomes.
const (
	gotTimeout = -2
	gotClosed  = -1
)

// outcome maps a term read's result to the byte, gotTimeout or gotClosed,
// failing the test on any other error.
func outcome(t *testing.T, what string, v Value, err error) int64 {
	t.Helper()
	switch {
	case err == nil:
		return v.Int()
	case errKind(err) == "timeout":
		return gotTimeout
	case errKind(err) == "closed":
		return gotClosed
	}
	t.Fatalf("%s: %v", what, err)
	return 0
}

// readTimeout calls term.readByteTimeout(ms).
func readTimeout(t *testing.T, ctx interpreter.BuiltinCtx, ms int64) int64 {
	t.Helper()
	v, err := readByteTimeoutFn(ctx, []Value{interpreter.IntVal(ms)})
	return outcome(t, fmt.Sprintf("readByteTimeout(%d)", ms), v, err)
}

// readOne calls term.readByte().
func readOne(t *testing.T, ctx interpreter.BuiltinCtx) int64 {
	t.Helper()
	v, err := readByteFn(ctx, nil)
	return outcome(t, "readByte", v, err)
}

// errKind is the kind of a builtin's error: a KindedError's Kind field, ""
// (runtime) for a plain error.
func errKind(err error) string {
	v := reflect.ValueOf(err)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() == reflect.Struct {
		if f := v.FieldByName("Kind"); f.IsValid() && f.Kind() == reflect.String {
			return f.String()
		}
	}
	return ""
}

// A timed read returns the byte when one arrives in time, throws "timeout" when
// none does, and
// a read that timed out stays in flight: the byte it eventually yields is the
// next one any term read returns, so nothing is lost or reordered.
func TestReadByteTimeout(t *testing.T) {
	ResetForTest()
	r, w := io.Pipe()
	defer w.Close()
	ctx := interpreter.BuiltinCtx{In: r}
	if got := readTimeout(t, ctx, 0); got != gotTimeout {
		t.Errorf("poll with nothing to read = %d, want timeout", got)
	}
	go func() { _, _ = w.Write([]byte{27}) }()
	if got := readTimeout(t, ctx, 2000); got != 27 {
		t.Errorf("byte in time = %d, want 27", got)
	}
	if got := readTimeout(t, ctx, 20); got != gotTimeout {
		t.Errorf("nothing in time = %d, want timeout", got)
	}
	// The timed-out read is still pending: the next write lands in it, and a
	// blocking readByte collects it.
	go func() { _, _ = w.Write([]byte("xy")) }()
	if v, err := readByteFn(ctx, nil); err != nil || v.Int() != 'x' {
		t.Errorf("readByte after a timeout = %v, %v; want 'x'", v, err)
	}
	if got := readTimeout(t, ctx, 2000); got != 'y' {
		t.Errorf("next byte = %d, want 'y'", got)
	}
	w.Close()
	if got := readTimeout(t, ctx, 2000); got != gotClosed {
		t.Errorf("at end of input = %d, want closed", got)
	}
}

func TestReadByteTimeoutArgs(t *testing.T) {
	ctx := interpreter.BuiltinCtx{In: strings.NewReader("")}
	for _, args := range [][]Value{nil, {interpreter.IntVal(-1)}, {interpreter.IntVal(maxTimeoutMs + 1)}, {interpreter.StringVal("5")}} {
		if _, err := readByteTimeoutFn(ctx, args); err == nil || errKind(err) != "" {
			t.Errorf("readByteTimeout(%v): %v, want a runtime error", args, err)
		}
	}
	if _, err := readByteTimeoutFn(interpreter.BuiltinCtx{InREPL: true}, []Value{interpreter.IntVal(1)}); err == nil {
		t.Error("readByteTimeout in the REPL should error")
	}
}
