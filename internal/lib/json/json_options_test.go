// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package jsonlib

import (
	"math"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// opts builds a json.EncodeOptions value as a program would.
func opts(indent int64, sortKeys, rawHtml, canonical bool) interpreter.Value {
	return interpreter.NamespacedStructVal(LibraryName, "EncodeOptions", []interpreter.StructField{
		{Name: "indent", Value: interpreter.IntVal(indent)},
		{Name: "sortKeys", Value: interpreter.BoolVal(sortKeys)},
		{Name: "rawHtml", Value: interpreter.BoolVal(rawHtml)},
		{Name: "canonical", Value: interpreter.BoolVal(canonical)},
	})
}

func encWith(t *testing.T, v, o interpreter.Value) string {
	t.Helper()
	out, err := encodeWithFn([]interpreter.Value{v, o})
	if err != nil {
		t.Fatalf("encodeWith: %v", err)
	}
	return out.Str
}

// sample mixes a Jennifer map (unsorted keys), a struct, a json.Value, HTML
// text and numbers.
func sample(t *testing.T) interpreter.Value {
	d := doc(t, `{"z": 1, "b": [3, 2], "a": {"y": true, "x": null}}`)
	st := interpreter.StructVal("P", []interpreter.StructField{
		{Name: "name", Value: interpreter.StringVal("<a&b>" + "\u2028")},
		{Name: "age", Value: interpreter.FloatVal(36)},
	})
	return interpreter.MapVal(parser.Type{}, parser.Type{}, []interpreter.MapEntry{
		{Key: interpreter.StringVal("m"), Value: d},
		{Key: interpreter.StringVal("k"), Value: st},
		{Key: interpreter.StringVal("c"), Value: interpreter.IntVal(7)},
	})
}

// The zero options are json.encode exactly, and indent 2 is encodePretty.
func TestEncodeWithDefaults(t *testing.T) {
	v := sample(t)
	if got, want := encWith(t, v, opts(0, false, false, false)), enc(t, v); got != want {
		t.Errorf("EncodeOptions{} = %s, encode = %s", got, want)
	}
	pretty, err := encodeFn([]interpreter.Value{v}, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := encWith(t, v, opts(2, false, false, false)); got != pretty.Str {
		t.Errorf("indent 2 = %s\nencodePretty = %s", got, pretty.Str)
	}
}

func TestEncodeWithIndent(t *testing.T) {
	v := doc(t, `{"a": [1, {}], "b": []}`)
	want := "{\n    \"a\": [\n        1,\n        {}\n    ],\n    \"b\": []\n}"
	if got := encWith(t, v, opts(4, false, false, false)); got != want {
		t.Errorf("indent 4 =\n%s\nwant\n%s", got, want)
	}
}

func TestEncodeWithSortKeys(t *testing.T) {
	got := encWith(t, sample(t), opts(0, true, false, false))
	want := `{"c":7,"k":{"age":36.0,"name":"` + `<a&b>` + `\u2028"},"m":{"a":{"x":null,"y":true},"b":[3,2],"z":1}}`
	want = strings.NewReplacer(`<`, `\`+"u003c", `>`, `\`+"u003e", `&`, `\`+"u0026").Replace(want)
	if got != want {
		t.Errorf("sortKeys =\n%s\nwant\n%s", got, want)
	}
	// Lists keep their order; an enum payload sorts like any object.
	e := interpreter.EnumVal("", "", "S", "C", []interpreter.StructField{
		{Name: "r", Value: interpreter.IntVal(1)}, {Name: "a", Value: interpreter.IntVal(2)},
	})
	if got := encWith(t, e, opts(0, true, false, false)); got != `{"C":{"a":2,"r":1}}` {
		t.Errorf("sorted enum = %s", got)
	}
}

func TestEncodeWithRawHtml(t *testing.T) {
	v := interpreter.StringVal("<a&b>" + "\u2028" + "\u2029" + "\x01\"")
	want := "\"<a&b>" + "\u2028\u2029" + `\` + "u0001" + `\""`
	if got := encWith(t, v, opts(0, false, true, false)); got != want {
		t.Errorf("rawHtml = %q, want %q", got, want)
	}
}

// RFC 8785: sorted keys (UTF-16 order), no whitespace, minimal escapes,
// ECMAScript numbers.
func TestEncodeWithCanonical(t *testing.T) {
	v := interpreter.MapVal(parser.Type{}, parser.Type{}, []interpreter.MapEntry{
		{Key: interpreter.StringVal("b"), Value: interpreter.IntVal(1)},
		{Key: interpreter.StringVal("a"), Value: interpreter.ListVal(parser.Type{}, []interpreter.Value{
			interpreter.FloatVal(1), interpreter.FloatVal(math.Copysign(0, -1)), interpreter.FloatVal(1e21),
			interpreter.FloatVal(1e-7), interpreter.FloatVal(0.000001), interpreter.FloatVal(2.5),
			interpreter.IntVal(1 << 53), interpreter.IntVal(-(1 << 53)),
			interpreter.StringVal("<" + "\u2028" + ">"),
		})},
	})
	want := `{"a":[1,0,1e+21,1e-7,0.000001,2.5,9007199254740992,-9007199254740992,"<` + "\u2028" + `>"],"b":1}`
	if got := encWith(t, v, opts(0, false, false, true)); got != want {
		t.Errorf("canonical =\n%s\nwant\n%s", got, want)
	}
	for _, n := range []int64{1<<53 + 1, -(1<<53 + 1), math.MaxInt64} {
		if _, err := encodeWithFn([]interpreter.Value{interpreter.IntVal(n), opts(0, false, false, true)}); err == nil ||
			!strings.Contains(err.Error(), "2^53") {
			t.Errorf("canonical %d: err = %v, want a 2^53 error", n, err)
		}
	}
}

// The RFC 8785 section 3.2.3 sorting example: UTF-16 code-unit order, which
// puts U+1F600 (surrogates D83D DE00) before U+FB33.
func TestCanonicalKeyOrder(t *testing.T) {
	keys := []string{"\u20ac", "\r", "\ufb33", "1", "\U0001f600", "\u0080", "\u00f6"}
	entries := make([]interpreter.MapEntry, len(keys))
	for i, k := range keys {
		entries[i] = interpreter.MapEntry{Key: interpreter.StringVal(k), Value: interpreter.IntVal(int64(i))}
	}
	got := encWith(t, interpreter.MapVal(parser.Type{}, parser.Type{}, entries), opts(0, false, false, true))
	want := `{"\r":1,"1":3,"` + "\u0080" + `":5,"` + "\u00f6" + `":6,"` + "\u20ac" + `":0,"` + "\U0001f600" + `":4,"` + "\ufb33" + `":2}`
	if got != want {
		t.Errorf("canonical key order =\n%q\nwant\n%q", got, want)
	}
}

func TestEncodeWithErrors(t *testing.T) {
	cases := []struct {
		o    interpreter.Value
		want string
	}{
		{opts(-1, false, false, false), "indent must be 0"},
		{opts(17, false, false, false), "indent must be 0"},
		{opts(2, false, false, true), "canonical output has no indentation"},
		{interpreter.IntVal(2), "must be a json.EncodeOptions"},
	}
	for _, c := range cases {
		if _, err := encodeWithFn([]interpreter.Value{interpreter.IntVal(1), c.o}); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("err = %v, want %q", err, c.want)
		}
	}
	if _, err := encodeWithFn([]interpreter.Value{interpreter.IntVal(1)}); err == nil {
		t.Error("one argument: want an arity error")
	}
}

// RFC 7386 Appendix A, every example.
func TestMergePatchRFCExamples(t *testing.T) {
	cases := [][3]string{
		{`{"a":"b"}`, `{"a":"c"}`, `{"a":"c"}`},
		{`{"a":"b"}`, `{"b":"c"}`, `{"a":"b","b":"c"}`},
		{`{"a":"b"}`, `{"a":null}`, `{}`},
		{`{"a":"b","b":"c"}`, `{"a":null}`, `{"b":"c"}`},
		{`{"a":["b"]}`, `{"a":"c"}`, `{"a":"c"}`},
		{`{"a":"c"}`, `{"a":["b"]}`, `{"a":["b"]}`},
		{`{"a":{"b":"c"}}`, `{"a":{"b":"d","c":null}}`, `{"a":{"b":"d"}}`},
		{`{"a":[{"b":"c"}]}`, `{"a":[1]}`, `{"a":[1]}`},
		{`["a","b"]`, `["c","d"]`, `["c","d"]`},
		{`{"a":"b"}`, `["c"]`, `["c"]`},
		{`{"a":"foo"}`, `null`, `null`},
		{`{"a":"foo"}`, `"bar"`, `"bar"`},
		{`{"e":null}`, `{"a":1}`, `{"e":null,"a":1}`},
		{`[1,2]`, `{"a":"b","c":null}`, `{"a":"b"}`},
		{`{}`, `{"a":{"bb":{"ccc":null}}}`, `{"a":{"bb":{}}}`},
	}
	for _, c := range cases {
		out, err := mergePatchFn([]interpreter.Value{doc(t, c[0]), doc(t, c[1])})
		if err != nil {
			t.Errorf("mergePatch(%s, %s): %v", c[0], c[1], err)
			continue
		}
		if got := jstr(t, out); got != c[2] {
			t.Errorf("mergePatch(%s, %s) = %s, want %s", c[0], c[1], got, c[2])
		}
	}
}

// A patch may be a plain Jennifer map; the target is not changed; removing
// from a large (indexed) object keeps document order and lookups right.
func TestMergePatchValues(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("{")
	for i := 0; i < 20; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`"k` + string(rune('a'+i)) + `":` + string(rune('0'+i%10)))
	}
	sb.WriteString("}")
	target := doc(t, sb.String())
	patch := interpreter.MapVal(parser.Type{}, parser.Type{}, []interpreter.MapEntry{
		{Key: interpreter.StringVal("kc"), Value: interpreter.Null()},
		{Key: interpreter.StringVal("kq"), Value: interpreter.Null()},
		{Key: interpreter.StringVal("ka"), Value: interpreter.StringVal("x")},
		{Key: interpreter.StringVal("new"), Value: interpreter.IntVal(1)},
	})
	before := jstr(t, target)
	out, err := mergePatchFn([]interpreter.Value{target, patch})
	if err != nil {
		t.Fatal(err)
	}
	if jstr(t, target) != before {
		t.Error("mergePatch changed its target")
	}
	got := jstr(t, out)
	if !strings.HasPrefix(got, `{"ka":"x","kb":1,"kd":3,`) || !strings.HasSuffix(got, `"kt":9,"new":1}`) ||
		strings.Contains(got, `"kc"`) || strings.Contains(got, `"kq"`) {
		t.Errorf("merged = %s", got)
	}
	for _, k := range []string{"/kb", "/kt", "/kr", "/new"} {
		if !call(t, hasFn, out, ptr(k)).Bool {
			t.Errorf("has(%s) = false after merge", k)
		}
	}
	if _, err := mergePatchFn([]interpreter.Value{target}); err == nil {
		t.Error("one argument: want an arity error")
	}
}
