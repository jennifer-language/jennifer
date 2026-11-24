// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package asn1lib

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
	"jennifer-lang.dev/jennifer/internal/parser"
)

func asn1Opts(depth, nodes int64) interpreter.Value {
	return interpreter.NamespacedStructVal(LibraryName, "DecodeOptions", []interpreter.StructField{
		{Name: "maxDepth", Value: interpreter.IntVal(depth)},
		{Name: "maxNodes", Value: interpreter.IntVal(nodes)},
	})
}

// M29.47: decodeWith lowers the caps (a reached one is "limit"), 0 keeps the
// default, a value above the default or a negative one is "runtime".
func TestDecodeWithCaps(t *testing.T) {
	// SEQUENCE { SEQUENCE { INTEGER 1 }, INTEGER 2 }: 4 elements, 2 levels
	// below the top.
	inner := mustBuild(t, sequenceFn, interpreter.ListVal(parser.Type{}, []interpreter.Value{mustBuild(t, integerFn, interpreter.IntVal(1))}))
	outer := mustBuild(t, sequenceFn, interpreter.ListVal(parser.Type{}, []interpreter.Value{inner, mustBuild(t, integerFn, interpreter.IntVal(2))}))
	data := interpreter.BytesVal(der(t, outer))
	plain := decode(t, data.Bytes())
	for _, o := range []interpreter.Value{asn1Opts(0, 0), asn1Opts(2, 4)} {
		got, err := call(decodeWithFn, data, o)
		if err != nil || !got.Equal(plain) {
			t.Errorf("options %v: %v, want decode's value", o, err)
		}
	}
	for name, o := range map[string]interpreter.Value{
		"depth": asn1Opts(1, 0),
		"nodes": asn1Opts(0, 3),
	} {
		_, err := call(decodeWithFn, data, o)
		if k := dataerr.KindOf(err); k != "limit" {
			t.Errorf("%s cap: kind %q, want limit (%v)", name, k, err)
		}
	}
	for name, o := range map[string]interpreter.Value{
		"depth above the default": asn1Opts(int64(limits.MaxNestingDepth)+1, 0),
		"nodes above the default": asn1Opts(0, int64(maxNodes)+1),
		"negative depth":          asn1Opts(-1, 0),
		"negative nodes":          asn1Opts(0, -1),
	} {
		_, err := call(decodeWithFn, data, o)
		if err == nil || dataerr.KindOf(err) != "" {
			t.Errorf("%s: %v, want a runtime error", name, err)
		}
	}
	if _, err := call(decodeWithFn, interpreter.StringVal("x"), asn1Opts(0, 0)); err == nil {
		t.Error("a string for data should be refused")
	}
	// Malformed input stays kind "asn1", named by the call.
	_, err := call(decodeWithFn, interpreter.BytesVal(append(data.Bytes(), 0)), asn1Opts(0, 0))
	if dataerr.KindOf(err) != "asn1" || !strings.HasPrefix(err.Error(), "asn1.decodeWith:") {
		t.Errorf("trailing byte: %v", err)
	}
}
