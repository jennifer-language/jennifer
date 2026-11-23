// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package yamllib

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// M29.21: malformed input is kind "yaml", a cap stays "limit", and misuse
// (wrong arguments, unencodable program values, accessors on a node of another
// type) stays "runtime" ("").
func TestErrorKinds(t *testing.T) {
	in := interpreter.New()
	Install(in)
	str := interpreter.StringVal
	callErr := func(name string, args ...interpreter.Value) func() error {
		return func() error {
			_, err := in.LookupNamespacedBuiltin("yaml", name)(interpreter.BuiltinCtx{}, args)
			return err
		}
	}
	doc, err := in.LookupNamespacedBuiltin("yaml", "decode")(interpreter.BuiltinCtx{}, []interpreter.Value{str("a: x\n")})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		call func() error
		kind string
	}{
		{"syntax", callErr("decode", str("a: [1, 2\n")), "yaml"},
		{"syntax in decodeAll", callErr("decodeAll", str("a: [1, 2\n")), "yaml"},
		{"duplicate key", callErr("decode", str("a: 1\na: 2\n")), "yaml"},
		{"several documents", callErr("decode", str("a: 1\n---\nb: 2\n")), "yaml"},
		{"undefined alias", callErr("decode", str("a: *nope\n")), "yaml"},
		{"bad tagged scalar", callErr("decode", str("a: !!int abc\n")), "yaml"},
		{"YAML 1.1 number", callErr("decode", str("a: 012\n")), "yaml"},
		{"merge of a scalar", callErr("decode", str("b: 1\na:\n  <<: 1\n")), "yaml"},
		{"invalid UTF-8", callErr("decode", str("a: \xff\n")), "yaml"},
		{"depth cap", callErr("decode", str(strings.Repeat("[", 200))), "limit"},
		{"not a string", callErr("decode", interpreter.IntVal(1)), ""},
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
