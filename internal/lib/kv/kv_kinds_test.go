// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package kv

import (
	"os"
	"path/filepath"
	"testing"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// M29.21: a store file found damaged on kv.openFile is kind "kv"; an
// unreadable file and every misuse stay "runtime" ("").
func TestErrorKinds(t *testing.T) {
	dir := t.TempDir()
	file := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	unterminated := file("unterminated.kv", "a2V5 dmFs 0")
	fields := file("fields.kv", "a2V5 dmFs\n")
	base64 := file("base64.kv", "!!! dmFs 0\n")
	expiry := file("expiry.kv", "a2V5 dmFs -1\n")
	r := &registry{stores: map[int64]*store{}}
	s := call(t, r.openFn)
	cases := []struct {
		name string
		fn   interpreter.Builtin
		args []interpreter.Value
		kind string
	}{
		{"unterminated", r.openFileFn, []interpreter.Value{str(unterminated)}, "kv"},
		{"field count", r.openFileFn, []interpreter.Value{str(fields)}, "kv"},
		{"base64", r.openFileFn, []interpreter.Value{str(base64)}, "kv"},
		{"negative expiry", r.openFileFn, []interpreter.Value{str(expiry)}, "kv"},
		{"a directory", r.openFileFn, []interpreter.Value{str(dir)}, ""},
		{"path type", r.openFileFn, []interpreter.Value{iv(1)}, ""},
		{"negative ttl", r.setFn, []interpreter.Value{s, str("k"), str("v"), iv(-1)}, ""},
		{"incr text", r.incrFn, []interpreter.Value{s, str("k"), iv(1)}, ""},
	}
	call(t, r.setFn, s, str("k"), str("text"), iv(0))
	for _, c := range cases {
		_, err := c.fn(ctx, c.args)
		if err == nil {
			t.Errorf("%s: no error", c.name)
			continue
		}
		if got := dataerr.KindOf(err); got != c.kind {
			t.Errorf("%s: %v: kind %q, want %q", c.name, err, got, c.kind)
		}
	}
}
