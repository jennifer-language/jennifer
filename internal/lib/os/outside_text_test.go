// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package oslib

import (
	"bytes"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// TestGetEnvInvalidUTF8 (M29.50): a value that is not UTF-8 throws kind "os"
// naming the variable and the byte offset; getEnvBytes returns it raw, and
// an unset variable is empty bytes.
func TestGetEnvInvalidUTF8(t *testing.T) {
	t.Setenv("JENNIFER_T_BAD", "ab\xffc")
	_, err := getEnvFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal("JENNIFER_T_BAD")})
	if err == nil || dataerr.KindOf(err) != "os" ||
		err.Error() != "os.getEnv: JENNIFER_T_BAD: invalid UTF-8 at byte 2; use os.getEnvBytes" {
		t.Fatalf("getEnv: %v", err)
	}
	v, err := getEnvBytesFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal("JENNIFER_T_BAD")})
	if err != nil || v.Kind != interpreter.KindBytes || !bytes.Equal(v.Bytes, []byte("ab\xffc")) {
		t.Fatalf("getEnvBytes: %v %v", v, err)
	}
	v, err = getEnvBytesFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal("JENNIFER_T_UNSET_XYZ")})
	if err != nil || v.Kind != interpreter.KindBytes || len(v.Bytes) != 0 {
		t.Fatalf("getEnvBytes unset: %v %v", v, err)
	}
	if _, err := getEnvBytesFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(1)}); err == nil {
		t.Fatal("getEnvBytes(int) accepted")
	}
}

func runOS(t *testing.T, args []string, src string) (string, error) {
	t.Helper()
	prev := userArgs
	t.Cleanup(func() { userArgs = prev })
	SetUserArgs(args)
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	Install(in)
	err = in.Run(prog)
	return buf.String(), err
}

// TestArgsInvalidUTF8 (M29.50): os.ARGS throws on every read when an argument
// is not UTF-8 (kind "os", naming the index and offset), catchably; a
// program that never reads it runs; os.argsBytes returns the raw arguments.
func TestArgsInvalidUTF8(t *testing.T) {
	bad := []string{"prog.j", "ok", "x\xfey"}
	out, err := runOS(t, bad, `
use io; use os;
io.printf("start ");
try {
    def a as list of string init os.ARGS;
} catch (e) {
    io.printf("%s: %s ", $e.kind, $e.message);
}
def raw as list of bytes init os.argsBytes();
io.printf("%d %d %d", len($raw), len($raw[2]), $raw[2][1]);
`)
	if err != nil {
		t.Fatal(err)
	}
	if want := "start os: os.ARGS: argument 2: invalid UTF-8 at byte 1; use os.argsBytes() 3 3 254"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
	// Uncaught, inside a function (the read site, not Install, throws).
	_, err = runOS(t, bad, `
use os;
func f() { return len(os.ARGS); }
f();
`)
	if err == nil || !strings.Contains(err.Error(), "argument 2: invalid UTF-8 at byte 1") {
		t.Fatalf("uncaught read: %v", err)
	}
	out, err = runOS(t, []string{"prog.j", "é"}, `use io; use os; io.printf("%s %d", os.ARGS[1], len(os.argsBytes()[1]));`)
	if err != nil || out != "é 2" {
		t.Fatalf("valid args: %q %v", out, err)
	}
	// os.flag validates the value it returns.
	_, err = runOS(t, []string{"prog.j", "--name", "\xc3"}, `use os; os.flag("--name");`)
	if err == nil || !strings.Contains(err.Error(), "os.flag: argument 2: invalid UTF-8 at byte 0") {
		t.Fatalf("os.flag: %v", err)
	}
}
