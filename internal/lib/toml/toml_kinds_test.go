// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package tomllib

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// M29.21: malformed input is kind "toml", a cap stays "limit", and misuse
// (wrong arguments, unencodable program values, accessors on a node of another
// type) stays "runtime" ("").
func TestErrorKinds(t *testing.T) {
	in := interpreter.New()
	Install(in)
	str := interpreter.StringVal
	callErr := func(name string, args ...interpreter.Value) func() error {
		return func() error {
			_, err := in.LookupNamespacedBuiltin("toml", name)(interpreter.BuiltinCtx{}, args)
			return err
		}
	}
	doc, err := in.LookupNamespacedBuiltin("toml", "decode")(interpreter.BuiltinCtx{}, []interpreter.Value{str("a = \"x\"\n")})
	if err != nil {
		t.Fatal(err)
	}
	deep := "a = " + strings.Repeat("[", limits.MaxNestingDepth+2)
	cases := []struct {
		name string
		call func() error
		kind string
	}{
		{"syntax", callErr("decode", str("a = ")), "toml"},
		{"duplicate key", callErr("decode", str("a = 1\na = 2\n")), "toml"},
		{"trailing", callErr("decode", str("a = 1 b\n")), "toml"},
		{"bad escape", callErr("decode", str(`a = "\q"`)), "toml"},
		{"bad date", callErr("decode", str("a = 2024-13-01\n")), "toml"},
		{"invalid UTF-8", callErr("decode", str("a = \"\xff\"\n")), "toml"},
		{"redefined table", callErr("decode", str("[t]\n[t]\n")), "toml"},
		{"depth cap", callErr("decode", str(deep)), "limit"},
		{"not a string", callErr("decode", interpreter.IntVal(1)), ""},
		{"encode non-table", callErr("encode", interpreter.IntVal(1)), ""},
		{"accessor on another type", callErr("asInt", doc, str("/a")), ""},
		{"missing key", callErr("get", doc, str("/b")), ""},
	}
	for _, c := range cases {
		err := c.call()
		if err == nil {
			t.Errorf("%s: no error", c.name)
			continue
		}
		if got := dataerr.KindOf(err); got != c.kind {
			t.Errorf("%s: %v: kind %q, want %q", c.name, err, got, c.kind)
		}
	}
}
