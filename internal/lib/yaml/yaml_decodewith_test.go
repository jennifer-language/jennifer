// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package yamllib

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

func yamlCall(t *testing.T, name string, args ...interpreter.Value) (interpreter.Value, error) {
	t.Helper()
	in := interpreter.New()
	Install(in)
	b := in.LookupNamespacedBuiltin("yaml", name)
	if b == nil {
		t.Fatalf("yaml.%s is not registered", name)
	}
	return b(interpreter.BuiltinCtx{}, args)
}

func mustYaml(t *testing.T, name string, args ...interpreter.Value) interpreter.Value {
	t.Helper()
	v, err := yamlCall(t, name, args...)
	if err != nil {
		t.Fatalf("yaml.%s: %v", name, err)
	}
	return v
}

func yamlOpts(depth, nodes int64) interpreter.Value {
	return interpreter.NamespacedStructVal(LibraryName, "DecodeOptions", []interpreter.StructField{
		{Name: "maxDepth", Value: interpreter.IntVal(depth)},
		{Name: "maxNodes", Value: interpreter.IntVal(nodes)},
	})
}

// M29.47: decodeWith lowers the caps (a reached one is "limit"), 0 keeps the
// default, a value above the default or a negative one is "runtime".
func TestDecodeWithCaps(t *testing.T) {
	s := interpreter.StringVal
	doc := "a:\n  b:\n    - 1\n    - 2\nc: 3\n"
	plain := mustYaml(t, "decode", s(doc))
	if got := mustYaml(t, "decodeWith", s(doc), yamlOpts(0, 0)); !got.Equal(plain) {
		t.Errorf("zero options = %v, want %v", got, plain)
	}
	mustYaml(t, "decodeWith", s(doc), yamlOpts(4, 7))
	for name, o := range map[string]interpreter.Value{
		"depth": yamlOpts(2, 0),
		"nodes": yamlOpts(0, 5),
	} {
		_, err := yamlCall(t, "decodeWith", s(doc), o)
		if k := dataerr.KindOf(err); k != "limit" {
			t.Errorf("%s cap: kind %q, want limit (%v)", name, k, err)
		}
	}
	// An alias expansion counts against a lowered node cap.
	bomb := "a: &a [1, 2, 3, 4]\nb: [*a, *a, *a]\n"
	mustYaml(t, "decode", s(bomb))
	if _, err := yamlCall(t, "decodeWith", s(bomb), yamlOpts(0, 10)); dataerr.KindOf(err) != "limit" {
		t.Errorf("alias expansion: %v, want limit", err)
	}
	for name, o := range map[string]interpreter.Value{
		"depth above the default": yamlOpts(int64(maxNestingDepth)+1, 0),
		"nodes above the default": yamlOpts(0, int64(maxNodes)+1),
		"negative depth":          yamlOpts(-1, 0),
		"negative nodes":          yamlOpts(0, -1),
	} {
		_, err := yamlCall(t, "decodeWith", s(doc), o)
		if err == nil || dataerr.KindOf(err) != "" {
			t.Errorf("%s: %v, want a runtime error", name, err)
		}
	}
	if _, err := yamlCall(t, "decodeWith", s(doc), s("x")); err == nil {
		t.Error("a string for options should be refused")
	}
}

// M29.47: asBytes reads a !!binary node; any other node is misuse.
func TestAsBytes(t *testing.T) {
	s := interpreter.StringVal
	doc := mustYaml(t, "decode", s("bin: !!binary aGVsbG8=\nblock: !!binary |\n  aGVs\n  bG8=\ntext: aGVsbG8=\n"))
	for _, ptr := range []string{"/bin", "/block"} {
		if got := mustYaml(t, "typeOf", doc, s(ptr)).Str; got != "bytes" {
			t.Errorf("typeOf(%s) = %q", ptr, got)
		}
		v := mustYaml(t, "asBytes", doc, s(ptr))
		if v.Kind != interpreter.KindBytes || string(v.Bytes) != "hello" {
			t.Errorf("asBytes(%s) = %v", ptr, v)
		}
	}
	if v := mustYaml(t, "asBytes", mustYaml(t, "get", doc, s("/bin"))); string(v.Bytes) != "hello" {
		t.Errorf("asBytes(get(/bin)) = %v", v)
	}
	_, err := yamlCall(t, "asBytes", doc, s("/text"))
	if err == nil || dataerr.KindOf(err) != "" || !strings.Contains(err.Error(), "node is a string") {
		t.Errorf("asBytes on a string: %v, want a runtime error", err)
	}
	// encode writes bytes as !!binary, which asBytes reads back.
	enc := mustYaml(t, "encode", interpreter.MapVal(parser.PrimitiveType(parser.TypeString), parser.PrimitiveType(parser.TypeBytes),
		[]interpreter.MapEntry{{Key: s("b"), Value: interpreter.BytesVal([]byte{0, 1, 255})}}))
	back := mustYaml(t, "asBytes", mustYaml(t, "decode", enc), s("/b"))
	if string(back.Bytes) != "\x00\x01\xff" {
		t.Errorf("round trip through %q = %v", enc.Str, back.Bytes)
	}
}
