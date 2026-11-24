// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package tomllib

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// tomlCall runs a registered toml builtin.
func tomlCall(t *testing.T, name string, args ...interpreter.Value) (interpreter.Value, error) {
	t.Helper()
	in := interpreter.New()
	Install(in)
	b := in.LookupNamespacedBuiltin("toml", name)
	if b == nil {
		t.Fatalf("toml.%s is not registered", name)
	}
	return b(interpreter.BuiltinCtx{}, args)
}

func mustToml(t *testing.T, name string, args ...interpreter.Value) interpreter.Value {
	t.Helper()
	v, err := tomlCall(t, name, args...)
	if err != nil {
		t.Fatalf("toml.%s: %v", name, err)
	}
	return v
}

func decodeOpts(depth, nodes int64) interpreter.Value {
	return interpreter.NamespacedStructVal(LibraryName, "DecodeOptions", []interpreter.StructField{
		{Name: "maxDepth", Value: interpreter.IntVal(depth)},
		{Name: "maxNodes", Value: interpreter.IntVal(nodes)},
	})
}

// M29.47: decodeWith lowers the caps (a reached one is "limit"), 0 keeps the
// default, a value above the default or a negative one is "runtime".
func TestDecodeWithCaps(t *testing.T) {
	s := interpreter.StringVal
	doc := "a = [[1, 2], [3]]\nb = { c = 1 }\n"
	plain := mustToml(t, "decode", s(doc))
	if got := mustToml(t, "decodeWith", s(doc), decodeOpts(0, 0)); !got.Equal(plain) {
		t.Errorf("zero options = %v, want %v", got, plain)
	}
	mustToml(t, "decodeWith", s(doc), decodeOpts(2, 8))
	for name, o := range map[string]interpreter.Value{
		"depth": decodeOpts(1, 0),
		"nodes": decodeOpts(0, 5),
	} {
		_, err := tomlCall(t, "decodeWith", s(doc), o)
		if k := dataerr.KindOf(err); k != "limit" {
			t.Errorf("%s cap: kind %q, want limit (%v)", name, k, err)
		}
		if err != nil && !strings.HasPrefix(err.Error(), "toml.decodeWith: ") {
			t.Errorf("%s cap: message %q", name, err)
		}
	}
	// The header path counts against the depth cap too.
	if _, err := tomlCall(t, "decodeWith", s("[a.b.c]\nx = 1\n"), decodeOpts(2, 0)); dataerr.KindOf(err) != "limit" {
		t.Errorf("header depth: %v, want limit", err)
	}
	for name, o := range map[string]interpreter.Value{
		"depth above the default": decodeOpts(int64(maxNestingDepth)+1, 0),
		"nodes above the default": decodeOpts(0, int64(maxDecodedNodes)+1),
		"negative depth":          decodeOpts(-1, 0),
		"negative nodes":          decodeOpts(0, -1),
	} {
		_, err := tomlCall(t, "decodeWith", s(doc), o)
		if err == nil || dataerr.KindOf(err) != "" {
			t.Errorf("%s: %v, want a runtime error", name, err)
		}
	}
}

// M29.47: typeOf names the local forms, asLocal reads each, asDatetime reads
// only the offset form, and encode writes every form back unchanged.
func TestLocalDatetimes(t *testing.T) {
	s := interpreter.StringVal
	src := "odt = 2024-01-02T03:04:05.5+02:00\n" +
		"ldt = 2024-01-02T03:04:05.123456789\n" +
		"ld = 2024-01-02\n" +
		"lt = 03:04:05.25\n"
	doc := mustToml(t, "decode", s(src))
	for ptr, want := range map[string]string{"/odt": "datetime", "/ldt": "local-datetime", "/ld": "local-date", "/lt": "local-time"} {
		if got := mustToml(t, "typeOf", doc, s(ptr)).Str(); got != want {
			t.Errorf("typeOf(%s) = %q, want %q", ptr, got, want)
		}
		if got := mustToml(t, "isDatetime", doc, s(ptr)).Bool(); got != (want == "datetime") {
			t.Errorf("isDatetime(%s) = %v", ptr, got)
		}
	}
	fields := func(v interpreter.Value) map[string]int64 {
		out := map[string]int64{}
		for _, f := range v.Fields() {
			out[f.Name] = f.Value.Int()
		}
		return out
	}
	for ptr, want := range map[string]map[string]int64{
		"/ldt": {"year": 2024, "month": 1, "day": 2, "hour": 3, "minute": 4, "second": 5, "nanosecond": 123456789},
		"/ld":  {"year": 2024, "month": 1, "day": 2, "hour": 0, "minute": 0, "second": 0, "nanosecond": 0},
		"/lt":  {"year": 0, "month": 0, "day": 0, "hour": 3, "minute": 4, "second": 5, "nanosecond": 250000000},
	} {
		v := mustToml(t, "asLocal", doc, s(ptr))
		if v.StructNS() != "toml" || v.StructName() != "Local" {
			t.Fatalf("asLocal(%s) = %s.%s", ptr, v.StructNS(), v.StructName())
		}
		got := fields(v)
		for k, w := range want {
			if got[k] != w {
				t.Errorf("asLocal(%s).%s = %d, want %d", ptr, k, got[k], w)
			}
		}
		_, err := tomlCall(t, "asDatetime", doc, s(ptr))
		if err == nil || dataerr.KindOf(err) != "" || !strings.Contains(err.Error(), "toml.asLocal") {
			t.Errorf("asDatetime(%s) = %v, want a runtime error naming toml.asLocal", ptr, err)
		}
	}
	// asLocal on the sub-node itself (no pointer) reads the same.
	if got := fields(mustToml(t, "asLocal", mustToml(t, "get", doc, s("/ld"))))["day"]; got != 2 {
		t.Errorf("asLocal(get(/ld)).day = %d", got)
	}
	// The offset form is unchanged: a time.Time at its offset.
	odt := mustToml(t, "asDatetime", doc, s("/odt"))
	if f := fields(odt); f["nanos"] != 1704157445500000000 || f["offset"] != 7200 {
		t.Errorf("asDatetime(/odt) = %v", f)
	}
	if _, err := tomlCall(t, "asLocal", doc, s("/odt")); err == nil || !strings.Contains(err.Error(), "toml.asDatetime") {
		t.Errorf("asLocal(/odt) = %v, want an error naming asDatetime", err)
	}
	if _, err := tomlCall(t, "asLocal", mustToml(t, "decode", s("x = 1")), s("/x")); err == nil {
		t.Error("asLocal on an int should error")
	}
	// encode round-trips every form, and so does a set of a local node.
	out := mustToml(t, "encode", doc).Str()
	for _, text := range []string{"2024-01-02T03:04:05.5+02:00", "2024-01-02T03:04:05.123456789", "ld = 2024-01-02\n", "lt = 03:04:05.25\n"} {
		if !strings.Contains(out, text) {
			t.Errorf("encode lost %q:\n%s", text, out)
		}
	}
	if back := mustToml(t, "decode", s(out)); !back.Equal(doc) {
		t.Errorf("decode(encode(doc)) differs:\n%s", out)
	}
	moved := mustToml(t, "set", doc, s("/copy"), mustToml(t, "get", doc, s("/lt")))
	if got := mustToml(t, "typeOf", moved, s("/copy")).Str(); got != "local-time" {
		t.Errorf("typeOf after set = %q", got)
	}
}

// M29.47: toml.decode's messages read like json.decode's: the function, the
// text, then line and column; at the end of the text, "unexpected end of
// input". The kind stays "toml".
func TestDecodeMessageShape(t *testing.T) {
	cases := []struct{ src, want string }{
		{"a = 1\nb = @\n", "toml.decode: unexpected value starting with \"@\" at line 2, column 5"},
		{"a = 1\nb = ", "toml.decode: unexpected end of input at line 2, column 5"},
		{"a", "toml.decode: unexpected end of input at line 1, column 2"},
		{"[t", "toml.decode: unexpected end of input at line 1, column 3"},
		{"x = { a = 1", "toml.decode: unexpected end of input at line 1, column 12"},
		{"a = 1\na = 2\n", "toml.decode: duplicate key \"a\" at line 2"},
		{"s = \"open", "toml.decode: unterminated string at line 1, column 10"},
	}
	for _, c := range cases {
		_, err := tomlCall(t, "decode", interpreter.StringVal(c.src))
		if err == nil {
			t.Errorf("%q: no error", c.src)
			continue
		}
		if !strings.HasPrefix(err.Error(), c.want) {
			t.Errorf("%q: %q, want %q", c.src, err, c.want)
		}
		if k := dataerr.KindOf(err); k != "toml" {
			t.Errorf("%q: kind %q, want toml", c.src, k)
		}
	}
	_, err := tomlCall(t, "decodeWith", interpreter.StringVal("a = "), decodeOpts(0, 0))
	if err == nil || err.Error() != "toml.decodeWith: unexpected end of input at line 1, column 5" {
		t.Errorf("decodeWith message: %v", err)
	}
}
