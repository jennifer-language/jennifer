// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package oslib

import (
	"bytes"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// bytesField returns a field of an os.BytesResult value.
func bytesField(t *testing.T, v interpreter.Value, name string) interpreter.Value {
	t.Helper()
	if v.Kind != interpreter.KindStruct || v.StructNS != "os" || v.StructName != "BytesResult" {
		t.Fatalf("want os.BytesResult, got %s %s.%s", v.Kind, v.StructNS, v.StructName)
	}
	for _, f := range v.Fields {
		if f.Name == name {
			return f.Value
		}
	}
	t.Fatalf("os.BytesResult has no field %s", name)
	return interpreter.Null()
}

// TestRunBytesBinaryOutput: os.runBytes returns stdout and stderr as bytes,
// NUL and 0xFF included, with the exit code; os.run refuses the same output.
func TestRunBytesBinaryOutput(t *testing.T) {
	skipIfNotLinux(t)
	in := []byte{0x00, 0xff, 'a', 0x00, 0xfe, 0x80}
	r, err := runBytesFn(interpreter.BuiltinCtx{}, []interpreter.Value{
		stringList("/bin/sh", "-c", `cat; printf '\377\000x' >&2; exit 4`), interpreter.BytesVal(in)})
	if err != nil {
		t.Fatal(err)
	}
	if got := bytesField(t, r, "stdout"); got.Kind != interpreter.KindBytes || !bytes.Equal(got.Bytes, in) {
		t.Errorf("stdout: %v %q", got.Kind, got.Bytes)
	}
	if got := bytesField(t, r, "stderr"); !bytes.Equal(got.Bytes, []byte{0xff, 0x00, 'x'}) {
		t.Errorf("stderr: %q", got.Bytes)
	}
	if got := bytesField(t, r, "exitCode"); got.Int != 4 {
		t.Errorf("exitCode: %d", got.Int)
	}
	if got := bytesField(t, r, "truncated"); got.Bool {
		t.Error("truncated set on short output")
	}
	if _, err := runFn(interpreter.BuiltinCtx{}, []interpreter.Value{
		stringList("/bin/cat"), interpreter.BytesVal(in)}); err == nil {
		t.Error("os.run accepted binary output")
	}
}

// TestWaitBytesBinaryOutput: os.waitBytes returns a spawned child's captured
// streams as bytes; os.wait on the same handle still applies its UTF-8 check,
// and waitBytes stays idempotent.
func TestWaitBytesBinaryOutput(t *testing.T) {
	skipIfNotLinux(t)
	p, err := spawnFn(interpreter.BuiltinCtx{}, []interpreter.Value{
		stringList("/bin/sh", "-c", `printf 'a\000\377b'; printf '\376' >&2`)})
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFn(interpreter.BuiltinCtx{}, []interpreter.Value{p})
	for i := 0; i < 2; i++ {
		r, err := waitBytesFn(interpreter.BuiltinCtx{}, []interpreter.Value{p})
		if err != nil {
			t.Fatal(err)
		}
		if got := bytesField(t, r, "stdout"); !bytes.Equal(got.Bytes, []byte{'a', 0x00, 0xff, 'b'}) {
			t.Errorf("stdout: %q", got.Bytes)
		}
		if got := bytesField(t, r, "stderr"); !bytes.Equal(got.Bytes, []byte{0xfe}) {
			t.Errorf("stderr: %q", got.Bytes)
		}
	}
	if _, err := waitFn(interpreter.BuiltinCtx{}, []interpreter.Value{p}); err == nil ||
		err.Error() != "os.wait: stdout: invalid UTF-8 at byte 2" {
		t.Errorf("os.wait: %v", err)
	}
	if _, err := waitBytesFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(1)}); err == nil ||
		!strings.HasPrefix(err.Error(), "os.waitBytes: ") {
		t.Errorf("waitBytes bad arg: %v", err)
	}
}

// TestRunBytesCap: the 16 MiB per-stream cap applies; the bytes form keeps
// exactly maxCaptureBytes bytes, adds no marker and sets truncated, while
// os.wait on the same output still appends the text marker.
func TestRunBytesCap(t *testing.T) {
	skipIfNotLinux(t)
	script := `head -c 17000000 /dev/zero`
	r, err := runBytesFn(interpreter.BuiltinCtx{}, []interpreter.Value{stringList("/bin/sh", "-c", script)})
	if err != nil {
		t.Fatal(err)
	}
	out := bytesField(t, r, "stdout").Bytes
	if len(out) != maxCaptureBytes || bytes.IndexByte(out, 'o') >= 0 {
		t.Errorf("stdout: %d bytes, want %d zero bytes", len(out), maxCaptureBytes)
	}
	if !bytesField(t, r, "truncated").Bool {
		t.Error("truncated not set")
	}
	p, err := spawnFn(interpreter.BuiltinCtx{}, []interpreter.Value{stringList("/bin/sh", "-c", `head -c 17000000 /dev/zero | tr '\000' a`)})
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFn(interpreter.BuiltinCtx{}, []interpreter.Value{p})
	wb, err := waitBytesFn(interpreter.BuiltinCtx{}, []interpreter.Value{p})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(bytesField(t, wb, "stdout").Bytes); n != maxCaptureBytes || !bytesField(t, wb, "truncated").Bool {
		t.Errorf("waitBytes: %d bytes, truncated=%v", n, bytesField(t, wb, "truncated").Bool)
	}
	w, err := waitFn(interpreter.BuiltinCtx{}, []interpreter.Value{p})
	if err != nil {
		t.Fatal(err)
	}
	if s := w.Fields[1].Value.Str; !strings.HasSuffix(s, "a\n[output truncated at 16 MiB]") {
		t.Errorf("os.wait text marker missing: %q", s[len(s)-40:])
	}
}
