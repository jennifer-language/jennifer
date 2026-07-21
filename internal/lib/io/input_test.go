// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package iolib

import (
	"bytes"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// callReadLine and callEof invoke the registered namespaced builtins
// with a fully-populated BuiltinCtx. Each test calls resetInputForTest
// first so the package-level buffered reader doesn't leak state across
// cases. The io library is namespaced, so the lookup goes
// through the interpreter's namespaced lookup helper.
func callReadLine(in *interpreter.Interpreter, ctx interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return in.LookupNamespacedBuiltin("io", "readLine")(ctx, args)
}

func callEof(in *interpreter.Interpreter, ctx interpreter.BuiltinCtx) (interpreter.Value, error) {
	return in.LookupNamespacedBuiltin("io", "eof")(ctx, nil)
}

func TestReadLineSingleLine(t *testing.T) {
	resetInputForTest()
	in := interpreter.New()
	Install(in)
	var out bytes.Buffer
	ctx := interpreter.BuiltinCtx{Out: &out, In: strings.NewReader("hello\n")}

	v, err := callReadLine(in, ctx, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if v.Kind != interpreter.KindString || v.Str != "hello" {
		t.Errorf("got %s(%q), want String(%q)", v.Kind, v.Str, "hello")
	}
}

func TestReadLineStripsCRLF(t *testing.T) {
	resetInputForTest()
	in := interpreter.New()
	Install(in)
	ctx := interpreter.BuiltinCtx{Out: &bytes.Buffer{}, In: strings.NewReader("hello\r\n")}

	v, err := callReadLine(in, ctx, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if v.Str != "hello" {
		t.Errorf("got %q, want %q", v.Str, "hello")
	}
}

func TestReadLineFinalUnterminatedLine(t *testing.T) {
	resetInputForTest()
	in := interpreter.New()
	Install(in)
	ctx := interpreter.BuiltinCtx{Out: &bytes.Buffer{}, In: strings.NewReader("a\nb")}

	v, err := callReadLine(in, ctx, nil)
	if err != nil || v.Str != "a" {
		t.Fatalf("first line: v=%q err=%v", v.Str, err)
	}
	v, err = callReadLine(in, ctx, nil)
	if err != nil {
		t.Fatalf("unterminated final line: %v", err)
	}
	if v.Str != "b" {
		t.Errorf("got %q, want %q", v.Str, "b")
	}
	_, err = callReadLine(in, ctx, nil)
	if err == nil || !strings.Contains(err.Error(), "end of input") {
		t.Errorf("expected EOF error on call past last line, got %v", err)
	}
}

func TestReadLineEOFOnEmpty(t *testing.T) {
	resetInputForTest()
	in := interpreter.New()
	Install(in)
	ctx := interpreter.BuiltinCtx{Out: &bytes.Buffer{}, In: strings.NewReader("")}

	_, err := callReadLine(in, ctx, nil)
	if err == nil || !strings.Contains(err.Error(), "end of input") {
		t.Errorf("expected EOF error, got %v", err)
	}
}

func TestReadLineWithPrompt(t *testing.T) {
	resetInputForTest()
	in := interpreter.New()
	Install(in)
	var out bytes.Buffer
	ctx := interpreter.BuiltinCtx{Out: &out, In: strings.NewReader("answer\n")}

	v, err := callReadLine(in, ctx, []interpreter.Value{interpreter.StringVal("name: ")})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if v.Str != "answer" {
		t.Errorf("got %q", v.Str)
	}
	if out.String() != "name: " {
		t.Errorf("prompt got %q, want %q", out.String(), "name: ")
	}
}

func TestReadLinePromptOnPipedInputStillWritten(t *testing.T) {
	// Behavior spec: prompt is unconditional (no isatty branch).
	resetInputForTest()
	in := interpreter.New()
	Install(in)
	var out bytes.Buffer
	ctx := interpreter.BuiltinCtx{Out: &out, In: strings.NewReader("X\n")}

	_, _ = callReadLine(in, ctx, []interpreter.Value{interpreter.StringVal("> ")})
	if out.String() != "> " {
		t.Errorf("prompt not written: got %q", out.String())
	}
}

func TestEofEmptyInput(t *testing.T) {
	resetInputForTest()
	in := interpreter.New()
	Install(in)
	ctx := interpreter.BuiltinCtx{Out: &bytes.Buffer{}, In: strings.NewReader("")}

	v, err := callEof(in, ctx)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if v.Kind != interpreter.KindBool || !v.Bool {
		t.Errorf("eof on empty input: got %s(%v), want Bool(true)", v.Kind, v.Bool)
	}
}

func TestEofBeforeReads(t *testing.T) {
	resetInputForTest()
	in := interpreter.New()
	Install(in)
	ctx := interpreter.BuiltinCtx{Out: &bytes.Buffer{}, In: strings.NewReader("one\n")}

	v, err := callEof(in, ctx)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if v.Bool {
		t.Errorf("eof should be false with data buffered")
	}
}

func TestEofAfterAllConsumed(t *testing.T) {
	resetInputForTest()
	in := interpreter.New()
	Install(in)
	ctx := interpreter.BuiltinCtx{Out: &bytes.Buffer{}, In: strings.NewReader("one\n")}

	if _, err := callReadLine(in, ctx, nil); err != nil {
		t.Fatalf("read: %v", err)
	}
	v, err := callEof(in, ctx)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !v.Bool {
		t.Errorf("eof should be true after last line consumed")
	}
}

func TestEofStickyOnceTrue(t *testing.T) {
	resetInputForTest()
	in := interpreter.New()
	Install(in)
	ctx := interpreter.BuiltinCtx{Out: &bytes.Buffer{}, In: strings.NewReader("")}

	_, _ = callEof(in, ctx)
	v, err := callEof(in, ctx)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !v.Bool {
		t.Errorf("eof should stay true on repeat calls")
	}
}

func TestReadEofLoop(t *testing.T) {
	// End-to-end shape: while (not io.eof()) { io.readLine() } over three lines.
	resetInputForTest()
	in := interpreter.New()
	Install(in)
	ctx := interpreter.BuiltinCtx{Out: &bytes.Buffer{}, In: strings.NewReader("a\nb\nc\n")}

	var got []string
	for {
		v, err := callEof(in, ctx)
		if err != nil {
			t.Fatalf("eof: %v", err)
		}
		if v.Bool {
			break
		}
		line, err := callReadLine(in, ctx, nil)
		if err != nil {
			t.Fatalf("readLine: %v", err)
		}
		got = append(got, line.Str)
	}
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("got %v, want [a b c]", got)
	}
}

func TestReadLineInREPLRefuses(t *testing.T) {
	resetInputForTest()
	in := interpreter.New()
	Install(in)
	ctx := interpreter.BuiltinCtx{Out: &bytes.Buffer{}, In: strings.NewReader("x\n"), InREPL: true}

	_, err := callReadLine(in, ctx, nil)
	if err == nil || !strings.Contains(err.Error(), "REPL") {
		t.Errorf("expected REPL refusal, got %v", err)
	}
	_, err = callEof(in, ctx)
	if err == nil || !strings.Contains(err.Error(), "REPL") {
		t.Errorf("expected REPL refusal for eof, got %v", err)
	}
}

func TestReadLineArgumentErrors(t *testing.T) {
	resetInputForTest()
	in := interpreter.New()
	Install(in)
	ctx := interpreter.BuiltinCtx{Out: &bytes.Buffer{}, In: strings.NewReader("x\n")}

	cases := []struct {
		name string
		args []interpreter.Value
		want string
	}{
		{"too many args", []interpreter.Value{interpreter.StringVal("a"), interpreter.StringVal("b")}, "0 or 1 argument"},
		{"non-string prompt", []interpreter.Value{interpreter.IntVal(1)}, "prompt must be string"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resetInputForTest()
			_, err := callReadLine(in, ctx, c.args)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("got %v, want substring %q", err, c.want)
			}
		})
	}
}

func TestEofArgumentError(t *testing.T) {
	resetInputForTest()
	in := interpreter.New()
	Install(in)
	ctx := interpreter.BuiltinCtx{Out: &bytes.Buffer{}, In: strings.NewReader("")}

	_, err := in.LookupNamespacedBuiltin("io", "eof")(ctx, []interpreter.Value{interpreter.IntVal(1)})
	if err == nil || !strings.Contains(err.Error(), "no arguments") {
		t.Errorf("got %v", err)
	}
}

// readBytes with a huge requested count on a short stream must return only the
// available bytes without attempting a giant up-front allocation.
func TestReadBytesHugeCountShortStream(t *testing.T) {
	resetInputForTest()
	in := interpreter.New()
	Install(in)
	var out bytes.Buffer
	ctx := interpreter.BuiltinCtx{Out: &out, In: strings.NewReader("hello")}
	readBytesFn := in.LookupNamespacedBuiltin("io", "readBytes")
	// A trillion-byte request against a 5-byte stream must not OOM.
	v, err := readBytesFn(ctx, []interpreter.Value{interpreter.IntVal(1000000000000)})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if v.Kind != interpreter.KindBytes || string(v.Bytes) != "hello" {
		t.Errorf("got %s(%q), want the 5 available bytes", v.Kind, string(v.Bytes))
	}
}
