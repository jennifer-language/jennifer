// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package xmllib

import (
	"testing"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

func xmlOpts(depth, nodes int64, ns interpreter.Value) interpreter.Value {
	fields := []interpreter.StructField{
		{Name: "maxDepth", Value: interpreter.IntVal(depth)},
		{Name: "maxNodes", Value: interpreter.IntVal(nodes)},
	}
	if ns.Kind != interpreter.KindNull {
		fields = append(fields, interpreter.StructField{Name: "namespaces", Value: ns})
	}
	return interpreter.NamespacedStructVal(LibraryName, "DecodeOptions", fields)
}

func prefixes(kv ...string) interpreter.Value {
	var es []interpreter.MapEntry
	for i := 0; i+1 < len(kv); i += 2 {
		es = append(es, interpreter.MapEntry{Key: sval(kv[i]), Value: sval(kv[i+1])})
	}
	st := parser.PrimitiveType(parser.TypeString)
	return interpreter.MapVal(st, st, es)
}

// M29.47: decodeWith lowers the caps (a reached one is "limit"), 0 keeps the
// default, a value above the default or a negative one is "runtime".
func TestDecodeWithCaps(t *testing.T) {
	doc := sval(`<a><b><c/></b><d x="1">t</d></a>`)
	plain, err := decodeFn([]interpreter.Value{doc})
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range []interpreter.Value{xmlOpts(0, 0, interpreter.Null()), xmlOpts(3, 6, prefixes())} {
		got, err := decodeWithFn([]interpreter.Value{doc, o})
		if err != nil || !got.Equal(plain) {
			t.Errorf("options %v: %v, want decode's value", o, err)
		}
	}
	for name, o := range map[string]interpreter.Value{
		"depth": xmlOpts(2, 0, interpreter.Null()),
		"nodes": xmlOpts(0, 5, interpreter.Null()),
	} {
		_, err := decodeWithFn([]interpreter.Value{doc, o})
		if k := dataerr.KindOf(err); k != "limit" {
			t.Errorf("%s cap: kind %q, want limit (%v)", name, k, err)
		}
	}
	for name, o := range map[string]interpreter.Value{
		"depth above the default": xmlOpts(int64(maxDepth)+1, 0, interpreter.Null()),
		"nodes above the default": xmlOpts(0, int64(maxDecodedNodes)+1, interpreter.Null()),
		"negative depth":          xmlOpts(-1, 0, interpreter.Null()),
		"negative nodes":          xmlOpts(0, -1, interpreter.Null()),
	} {
		_, err := decodeWithFn([]interpreter.Value{doc, o})
		if err == nil || dataerr.KindOf(err) != "" {
			t.Errorf("%s: %v, want a runtime error", name, err)
		}
	}
}

// M29.47: DecodeOptions.namespaces is the deprecated second argument of
// xml.decode: the same bindings in scope at the root, the same tree.
func TestDecodeWithNamespaces(t *testing.T) {
	frag := sval(`<gpx:wpt lat="1"><gpx:name>a</gpx:name><x:ext/></gpx:wpt>`)
	ns := prefixes("gpx", "urn:gpx", "x", "urn:x", "", "urn:d")
	old, err := decodeFn([]interpreter.Value{frag, ns})
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeWithFn([]interpreter.Value{frag, xmlOpts(0, 0, ns)})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(old) {
		t.Error("decodeWith{namespaces} differs from decode(s, ns)")
	}
	if a, b := call(t, encodeFnFor(false), got).Str(), call(t, encodeFnFor(false), old).Str(); a != b {
		t.Errorf("encodings differ:\n%s\n%s", a, b)
	}
	if got := call(t, namespaceFn, mustGet(t, got, "x:ext")).Str(); got != "urn:x" {
		t.Errorf("x:ext namespace = %q", got)
	}
	// Without the bindings the prefixes are undeclared.
	if _, err := decodeWithFn([]interpreter.Value{frag, xmlOpts(0, 0, interpreter.Null())}); dataerr.KindOf(err) != "xml" {
		t.Errorf("no namespaces: %v, want kind xml", err)
	}
	// A bad binding is misuse, as with the positional map.
	if _, err := decodeWithFn([]interpreter.Value{frag, xmlOpts(0, 0, prefixes("xmlns", "urn:y"))}); err == nil {
		t.Error("binding the xmlns prefix should be refused")
	}
}

// encodeFnFor adapts encodeFn to a Builtin.
func encodeFnFor(pretty bool) interpreter.Builtin {
	return func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		return encodeFn(args, pretty)
	}
}
