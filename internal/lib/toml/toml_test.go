// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package tomllib

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// get walks a decoded tree by JSON Pointer, failing the test on any error.
func get(t *testing.T, tree interpreter.Value, ptr string) interpreter.Value {
	t.Helper()
	tokens, err := parsePointer("get", ptr)
	if err != nil {
		t.Fatalf("bad pointer %q: %v", ptr, err)
	}
	v, err := walkPointer("get", tree, tokens)
	if err != nil {
		t.Fatalf("walk %q: %v", ptr, err)
	}
	return v
}

func TestDecodeScalars(t *testing.T) {
	src := `
title = "Jennifer"
literal = 'C:\temp'
count = 42
hex = 0xff
oct = 0o755
bin = 0b1010
neg = -17
big = 1_000_000
ratio = 3.14
sci = 1e3
posinf = inf
notnum = nan
yes = true
no = false
`
	tree, err := decodeToml(src)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v := get(t, tree, "/title"); v.Kind != interpreter.KindString || v.Str != "Jennifer" {
		t.Errorf("title = %+v", v)
	}
	if v := get(t, tree, "/literal"); v.Str != `C:\temp` {
		t.Errorf("literal = %q", v.Str)
	}
	if v := get(t, tree, "/count"); v.Kind != interpreter.KindInt || v.Int != 42 {
		t.Errorf("count = %+v", v)
	}
	if v := get(t, tree, "/hex"); v.Int != 255 {
		t.Errorf("hex = %+v", v)
	}
	if v := get(t, tree, "/oct"); v.Int != 493 {
		t.Errorf("oct = %+v", v)
	}
	if v := get(t, tree, "/bin"); v.Int != 10 {
		t.Errorf("bin = %+v", v)
	}
	if v := get(t, tree, "/neg"); v.Int != -17 {
		t.Errorf("neg = %+v", v)
	}
	if v := get(t, tree, "/big"); v.Int != 1000000 {
		t.Errorf("big = %+v", v)
	}
	if v := get(t, tree, "/ratio"); v.Kind != interpreter.KindFloat || v.Float != 3.14 {
		t.Errorf("ratio = %+v", v)
	}
	if v := get(t, tree, "/sci"); v.Kind != interpreter.KindFloat || v.Float != 1000 {
		t.Errorf("sci = %+v", v)
	}
	if v := get(t, tree, "/yes"); v.Kind != interpreter.KindBool || !v.Bool {
		t.Errorf("yes = %+v", v)
	}
}

func TestDecodeTablesAndArrays(t *testing.T) {
	src := `
[server]
host = "localhost"
ports = [8000, 8001, 8002]

[server.tls]
enabled = true

[[fruit]]
name = "apple"

[[fruit]]
name = "banana"
`
	tree, err := decodeToml(src)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v := get(t, tree, "/server/host"); v.Str != "localhost" {
		t.Errorf("host = %q", v.Str)
	}
	if v := get(t, tree, "/server/ports/2"); v.Int != 8002 {
		t.Errorf("ports[2] = %+v", v)
	}
	if v := get(t, tree, "/server/tls/enabled"); !v.Bool {
		t.Errorf("tls.enabled = %+v", v)
	}
	if v := get(t, tree, "/fruit/0/name"); v.Str != "apple" {
		t.Errorf("fruit[0] = %q", v.Str)
	}
	if v := get(t, tree, "/fruit/1/name"); v.Str != "banana" {
		t.Errorf("fruit[1] = %q", v.Str)
	}
}

func TestDecodeDottedKeysAndInlineTable(t *testing.T) {
	src := `
a.b.c = 1
point = { x = 1, y = 2 }
nested = { deep.value = 3 }
`
	tree, err := decodeToml(src)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v := get(t, tree, "/a/b/c"); v.Int != 1 {
		t.Errorf("a.b.c = %+v", v)
	}
	if v := get(t, tree, "/point/x"); v.Int != 1 {
		t.Errorf("point.x = %+v", v)
	}
	if v := get(t, tree, "/nested/deep/value"); v.Int != 3 {
		t.Errorf("nested.deep.value = %+v", v)
	}
}

func TestDecodeStrings(t *testing.T) {
	src := `
esc = "tab\there\nnewline"
uni = "é"
multi = """
first
second"""
mlit = '''raw\nnot-escaped'''
`
	tree, err := decodeToml(src)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v := get(t, tree, "/esc"); v.Str != "tab\there\nnewline" {
		t.Errorf("esc = %q", v.Str)
	}
	if v := get(t, tree, "/uni"); v.Str != "é" {
		t.Errorf("uni = %q", v.Str)
	}
	if v := get(t, tree, "/multi"); v.Str != "first\nsecond" {
		t.Errorf("multi = %q", v.Str)
	}
	if v := get(t, tree, "/mlit"); v.Str != `raw\nnot-escaped` {
		t.Errorf("mlit = %q", v.Str)
	}
}

func TestDecodeDatetimes(t *testing.T) {
	src := `
odt = 1979-05-27T07:32:00Z
odt2 = 1979-05-27T00:32:00-07:00
ldt = 1979-05-27T07:32:00
ld = 1979-05-27
lt = 07:32:00
spaced = 1979-05-27 07:32:00Z
`
	tree, err := decodeToml(src)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	cases := []struct {
		ptr, wantForm string
	}{
		{"/odt", formOffsetDatetime},
		{"/odt2", formOffsetDatetime},
		{"/ldt", formLocalDatetime},
		{"/ld", formLocalDate},
		{"/lt", formLocalTime},
		{"/spaced", formOffsetDatetime},
	}
	for _, c := range cases {
		v := get(t, tree, c.ptr)
		_, form, ok := isDatetimeNode(v)
		if !ok {
			t.Errorf("%s: not a datetime node (%+v)", c.ptr, v)
			continue
		}
		if form != c.wantForm {
			t.Errorf("%s: form = %q, want %q", c.ptr, form, c.wantForm)
		}
		if _, err := parseDatetimeText(mustText(v), form); err != nil {
			t.Errorf("%s: parseDatetimeText: %v", c.ptr, err)
		}
	}
	// The space separator is normalised to 'T'.
	if txt := mustText(get(t, tree, "/spaced")); txt != "1979-05-27T07:32:00Z" {
		t.Errorf("spaced normalised = %q", txt)
	}
}

func mustText(v interpreter.Value) string {
	text, _, _ := isDatetimeNode(v)
	return text
}

func TestEncodeRoundTrip(t *testing.T) {
	src := `title = "Jennifer"
created = 1979-05-27T07:32:00Z

[server]
host = "localhost"
ports = [8000, 8001]

[[fruit]]
name = "apple"

[[fruit]]
name = "banana"
`
	tree, err := decodeToml(src)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	out, err := encodeToml(tree, false)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	// Re-decode and compare a few reachable values (encode reorders tables, so a
	// text-equality check would be brittle; structural equality is the contract).
	tree2, err := decodeToml(out)
	if err != nil {
		t.Fatalf("re-decode:\n%s\nerr: %v", out, err)
	}
	if v := get(t, tree2, "/server/ports/1"); v.Int != 8001 {
		t.Errorf("round-trip ports[1] = %+v", v)
	}
	if v := get(t, tree2, "/fruit/1/name"); v.Str != "banana" {
		t.Errorf("round-trip fruit[1] = %q", v.Str)
	}
	if txt := mustText(get(t, tree2, "/created")); txt != "1979-05-27T07:32:00Z" {
		t.Errorf("round-trip created = %q", txt)
	}
}

func TestEncodeFloatSpecials(t *testing.T) {
	tree := mapVal([]interpreter.MapEntry{
		{Key: interpreter.StringVal("whole"), Value: interpreter.FloatVal(2)},
		{Key: interpreter.StringVal("pinf"), Value: interpreter.FloatVal(inf(false))},
		{Key: interpreter.StringVal("ninf"), Value: interpreter.FloatVal(inf(true))},
		{Key: interpreter.StringVal("bad"), Value: interpreter.FloatVal(nan())},
	})
	out, err := encodeToml(tree, false)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	for _, want := range []string{"whole = 2.0", "pinf = inf", "ninf = -inf", "bad = nan"} {
		if !strings.Contains(out, want) {
			t.Errorf("encode missing %q in:\n%s", want, out)
		}
	}
}

func TestEncodeRejectsNullAndNonTableRoot(t *testing.T) {
	if _, err := encodeToml(interpreter.Null(), false); err == nil {
		t.Error("expected error encoding a null root")
	}
	if _, err := encodeToml(listVal(nil), false); err == nil {
		t.Error("expected error encoding a list root")
	}
	withNull := mapVal([]interpreter.MapEntry{
		{Key: interpreter.StringVal("x"), Value: interpreter.Null()},
	})
	if _, err := encodeToml(withNull, false); err == nil {
		t.Error("expected error encoding a null value")
	}
}

func TestDecodeErrors(t *testing.T) {
	bad := []string{
		"key = ",
		"= 1",
		"key = \"unterminated",
		"[unclosed",
		"a = 1\na = 2",
		"key = @nope",
		// TOML 1.0 integers are 64-bit signed; overflow is a decode error,
		// not a silent lossy-float downgrade.
		"big = 9223372036854775808",           // MaxInt64 + 1
		"big = 99999999999999999999999999999", // far past int64
		"big = -9223372036854775809",          // MinInt64 - 1
	}
	for _, src := range bad {
		if _, err := decodeToml(src); err == nil {
			t.Errorf("expected error decoding %q", src)
		}
	}
	// The boundaries themselves still decode.
	for _, src := range []string{"n = 9223372036854775807", "n = -9223372036854775808"} {
		if _, err := decodeToml(src); err != nil {
			t.Errorf("boundary %q should decode, got %v", src, err)
		}
	}
}

// Nesting beyond the decoder's depth cap must surface as a normal, catchable
// decode error. Unbounded recursion exhausts the Go stack, which is fatal and
// uncatchable - a DoS wherever untrusted TOML is decoded.
func TestDecodeDepthCap(t *testing.T) {
	deepArr := "x = " + strings.Repeat("[", maxNestingDepth+1) + strings.Repeat("]", maxNestingDepth+1)
	if _, err := decodeToml(deepArr); err == nil || !strings.Contains(err.Error(), "nesting") {
		t.Errorf("deep array: expected nesting-depth error, got %v", err)
	}
	deepTbl := "x = " + strings.Repeat("{a = ", maxNestingDepth+1) + "1" + strings.Repeat("}", maxNestingDepth+1)
	if _, err := decodeToml(deepTbl); err == nil || !strings.Contains(err.Error(), "nesting") {
		t.Errorf("deep inline table: expected nesting-depth error, got %v", err)
	}
	// A dotted key recurses per segment in inlineAssign and builds a
	// per-segment-deep tree from headers/key-values; cap it the same way.
	deepKey := "x = {" + strings.Repeat("a.", maxNestingDepth+1) + "b = 1}"
	if _, err := decodeToml(deepKey); err == nil || !strings.Contains(err.Error(), "nesting") {
		t.Errorf("deep dotted key: expected nesting-depth error, got %v", err)
	}
	// A document exactly at the cap still decodes.
	okArr := "x = " + strings.Repeat("[", maxNestingDepth) + strings.Repeat("]", maxNestingDepth)
	if _, err := decodeToml(okArr); err != nil {
		t.Errorf("depth-%d document should decode, got %v", maxNestingDepth, err)
	}
	// Truncated deep input (openers only, the cheap-to-build attack shape)
	// must error normally instead of crashing the process.
	if _, err := decodeToml("x = " + strings.Repeat("[", 5_000_000)); err == nil {
		t.Error("truncated deep input: expected error, got nil")
	}
}

// TestDecodeTruncatedDatetimes verifies a short date-time token errors instead
// of slicing past the buffer (a crash on untrusted input).
func TestDecodeTruncatedDatetimes(t *testing.T) {
	for _, src := range []string{
		"k = 2020-",
		"k = 12:3",
		"k = 2020-01-01T00:00:00+",
		"k = 2020-01-01T",
	} {
		if _, err := decodeToml(src); err == nil {
			t.Errorf("expected error decoding %q, got nil", src)
		}
	}
}

// Redefinition state must be scoped per array-of-tables element: every
// [[header]] opens a fresh element namespace, so a sub-table or inline-table
// key that already appeared under an earlier element is legal again.
func TestDecodeArrayOfTablesElementScoping(t *testing.T) {
	// The TOML 1.0 spec's own fruit example.
	fruit := `
[[fruit]]
name = "apple"
[fruit.physical]
color = "red"
[[fruit.variety]]
name = "red delicious"
[[fruit]]
name = "banana"
[fruit.physical]
color = "yellow"
[[fruit.variety]]
name = "plantain"
`
	tree, err := decodeToml(fruit)
	if err != nil {
		t.Fatalf("spec fruit example must decode, got: %v", err)
	}
	if v := get(t, tree, "/fruit/1/physical/color"); v.Str != "yellow" {
		t.Errorf("fruit[1].physical.color = %q", v.Str)
	}
	if v := get(t, tree, "/fruit/1/variety/0/name"); v.Str != "plantain" {
		t.Errorf("fruit[1].variety[0].name = %q", v.Str)
	}

	// Inline tables under the same key across elements are independent too.
	srv := `
[[srv]]
net = { ip = "1.1.1.1" }
[[srv]]
net = { ip = "2.2.2.2" }
`
	tree, err = decodeToml(srv)
	if err != nil {
		t.Fatalf("per-element inline tables must decode, got: %v", err)
	}
	if v := get(t, tree, "/srv/1/net/ip"); v.Str != "2.2.2.2" {
		t.Errorf("srv[1].net.ip = %q", v.Str)
	}

	// Within ONE element the redefinition rules still bite.
	bad := []string{
		"[[f]]\n[f.p]\nx = 1\n[f.p]\ny = 2",            // same sub-table twice in one element
		"[[f]]\nnet = { ip = \"1\" }\nnet.ip2 = \"2\"", // extend own inline table
		"[[f]]\n[f.p]\nx = 1\n[[f.p]]",                 // sub-table redefined as array
	}
	for _, src := range bad {
		if _, err := decodeToml(src); err == nil {
			t.Errorf("expected error decoding %q", src)
		}
	}
}

// A [header] or [[header]] may not reach THROUGH a closed inline table
// either - not just name one exactly.
func TestDecodeHeaderThroughInlineTable(t *testing.T) {
	bad := []string{
		"a = { x = 1 }\n[a.b]\ny = 2",
		"a = { x = 1 }\n[[a.b]]\ny = 2",
	}
	for _, src := range bad {
		if _, err := decodeToml(src); err == nil {
			t.Errorf("expected error decoding %q", src)
		}
	}
}

// Underscore validation must use the token's own base: in a decimal number
// `e` is an exponent marker, not a digit, so `1_e5` is a MUST-error (it used
// to slip through the hex digit class and decode as 100000).
func TestDecodeUnderscoreBaseAware(t *testing.T) {
	bad := []string{"x = 1_e5", "x = 1e_5", "x = 1.5_e2", "x = 0b1_2", "x = 0o1_8"}
	for _, src := range bad {
		if _, err := decodeToml(src); err == nil {
			t.Errorf("expected error decoding %q", src)
		}
	}
	good := map[string]interface{}{
		"x = 1_000":       int64(1000),
		"x = 1e5_0":       float64(1e50),
		"x = 0xdead_beef": int64(0xdeadbeef),
		"x = 0b1_0":       int64(2),
	}
	for src, want := range good {
		tree, err := decodeToml(src)
		if err != nil {
			t.Errorf("%q should decode, got %v", src, err)
			continue
		}
		v := get(t, tree, "/x")
		switch w := want.(type) {
		case int64:
			if v.Int != w {
				t.Errorf("%q = %d, want %d", src, v.Int, w)
			}
		case float64:
			if v.Float != w {
				t.Errorf("%q = %g, want %g", src, v.Float, w)
			}
		}
	}
}
