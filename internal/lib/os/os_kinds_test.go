// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package oslib

import (
	"testing"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// M29.21: a child's captured output that is not valid UTF-8 is kind "os"; a
// program that cannot start and misuse stay "runtime" ("").
func TestErrorKinds(t *testing.T) {
	skipIfNotLinux(t)
	ctx := interpreter.BuiltinCtx{}
	cases := []struct {
		name string
		args []interpreter.Value
		kind string
	}{
		{"stdout", []interpreter.Value{stringList("/bin/cat"), interpreter.BytesVal([]byte("ab\xffc"))}, "os"},
		{"stderr", []interpreter.Value{stringList("/bin/sh", "-c", `printf '\376' >&2`)}, "os"},
		{"not found", []interpreter.Value{stringList("/nonexistent/jennifer-test")}, ""},
		{"empty argv", []interpreter.Value{stringList()}, ""},
	}
	for _, c := range cases {
		_, err := runFn(ctx, c.args)
		if err == nil {
			t.Errorf("%s: no error", c.name)
			continue
		}
		if got := dataerr.KindOf(err); got != c.kind {
			t.Errorf("%s: %v: kind %q, want %q", c.name, err, got, c.kind)
		}
	}
	p, err := spawnFn(ctx, []interpreter.Value{stringList("/bin/sh", "-c", `printf '\303'`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := waitFn(ctx, []interpreter.Value{p}); dataerr.KindOf(err) != "os" {
		t.Errorf("os.wait: %v: kind %q, want \"os\"", err, dataerr.KindOf(err))
	}
	_, _ = releaseFn(ctx, []interpreter.Value{p})
}
