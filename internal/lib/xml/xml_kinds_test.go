// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package xmllib

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// M29.21: malformed input is kind "xml", a cap stays "limit", and misuse
// (wrong arguments, a bad namespace map or name the program wrote, a lookup
// that finds nothing) stays "runtime" ("").
func TestErrorKinds(t *testing.T) {
	in := interpreter.New()
	Install(in)
	str := interpreter.StringVal
	callErr := func(name string, args ...interpreter.Value) func() error {
		return func() error {
			_, err := in.LookupNamespacedBuiltin("xml", name)(interpreter.BuiltinCtx{}, args)
			return err
		}
	}
	doc, err := in.LookupNamespacedBuiltin("xml", "decode")(interpreter.BuiltinCtx{}, []interpreter.Value{str("<a/>")})
	if err != nil {
		t.Fatal(err)
	}
	strT := parser.PrimitiveType(parser.TypeString)
	badNS := interpreter.MapVal(strT, strT, []interpreter.MapEntry{{Key: str("xmlns"), Value: str("urn:x")}})
	deep := strings.Repeat("<a>", limits.MaxNestingDepth+2)
	cases := []struct {
		name string
		call func() error
		kind string
	}{
		{"unclosed", callErr("decode", str("<a>")), "xml"},
		{"mismatched tag", callErr("decode", str("<a></b>")), "xml"},
		{"trailing", callErr("decode", str("<a/><b/>")), "xml"},
		{"duplicate attribute", callErr("decode", str(`<a x="1" x="2"/>`)), "xml"},
		{"unknown entity", callErr("decode", str("<a>&nope;</a>")), "xml"},
		{"undeclared prefix", callErr("decode", str("<p:a/>")), "xml"},
		{"invalid UTF-8", callErr("decode", str("<a>\xff</a>")), "xml"},
		{"empty", callErr("decode", str("")), "xml"},
		{"depth cap", callErr("decode", str(deep)), "limit"},
		{"not a string", callErr("decode", interpreter.IntVal(1)), ""},
		{"bad namespace map", callErr("decode", str("<a/>"), badNS), ""},
		{"bad element name", callErr("element", str("1a")), ""},
		{"no such attribute", callErr("attr", doc, str("x")), ""},
		{"no matching path", callErr("get", doc, str("b")), ""},
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
