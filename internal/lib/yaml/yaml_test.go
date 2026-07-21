// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package yamllib

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

var noCtx = interpreter.BuiltinCtx{}

func mustDecode(t *testing.T, src string) interpreter.Value {
	t.Helper()
	tree, err := decodeYaml(src)
	if err != nil {
		t.Fatalf("decode %q: %v", src, err)
	}
	return wrap(tree)
}

// get resolves a JSON pointer against a wrapped yaml.Value and returns the
// wrapped sub-node.
func get(t *testing.T, rv interpreter.Value, ptr string) interpreter.Value {
	t.Helper()
	v, err := getFn(noCtx, []interpreter.Value{rv, interpreter.StringVal(ptr)})
	if err != nil {
		t.Fatalf("get %q: %v", ptr, err)
	}
	return v
}

func strAt(t *testing.T, rv interpreter.Value, ptr string) string {
	t.Helper()
	v, err := asStringFn(noCtx, []interpreter.Value{get(t, rv, ptr)})
	if err != nil {
		t.Fatalf("asString %q: %v", ptr, err)
	}
	return v.Str
}

func intAt(t *testing.T, rv interpreter.Value, ptr string) int64 {
	t.Helper()
	v, err := asIntFn(noCtx, []interpreter.Value{get(t, rv, ptr)})
	if err != nil {
		t.Fatalf("asInt %q: %v", ptr, err)
	}
	return v.Int
}

func typeAt(t *testing.T, rv interpreter.Value, ptr string) string {
	t.Helper()
	v, err := typeOfFn(noCtx, []interpreter.Value{rv, interpreter.StringVal(ptr)})
	if err != nil {
		t.Fatalf("typeOf %q: %v", ptr, err)
	}
	return v.Str
}

func TestDecodeScalars(t *testing.T) {
	rv := mustDecode(t, "s: hello\ni: 42\nhex: 0xff\nf: 3.5\nb: true\nn: ~\nq: \"42\"\nwhen: 2001-12-15T02:59:43Z\n")
	if got := typeAt(t, rv, "/s"); got != "string" {
		t.Errorf("s type = %q", got)
	}
	if got := strAt(t, rv, "/s"); got != "hello" {
		t.Errorf("s = %q", got)
	}
	if got := intAt(t, rv, "/i"); got != 42 {
		t.Errorf("i = %d", got)
	}
	if got := intAt(t, rv, "/hex"); got != 255 {
		t.Errorf("hex = %d", got)
	}
	if got := typeAt(t, rv, "/f"); got != "float" {
		t.Errorf("f type = %q", got)
	}
	fv, _ := asFloatFn(noCtx, []interpreter.Value{get(t, rv, "/f")})
	if fv.Float != 3.5 {
		t.Errorf("f = %v", fv.Float)
	}
	bv, _ := asBoolFn(noCtx, []interpreter.Value{get(t, rv, "/b")})
	if !bv.Bool {
		t.Errorf("b = %v", bv.Bool)
	}
	if got := typeAt(t, rv, "/n"); got != "null" {
		t.Errorf("n type = %q", got)
	}
	nv, _ := isNullFn(noCtx, []interpreter.Value{get(t, rv, "/n")})
	if !nv.Bool {
		t.Error("n should be null")
	}
	// A quoted "42" stays a string (implicit typing is overridden by quotes).
	if got := typeAt(t, rv, "/q"); got != "string" {
		t.Errorf("q type = %q, want string", got)
	}
	// A timestamp scalar is reported as datetime.
	if got := typeAt(t, rv, "/when"); got != "datetime" {
		t.Errorf("when type = %q, want datetime", got)
	}
}

func TestDecodeNesting(t *testing.T) {
	rv := mustDecode(t, "a:\n  b:\n    - x\n    - y\n    - z\nm:\n  k: v\n")
	if got := typeAt(t, rv, "/a/b"); got != "list" {
		t.Errorf("a/b type = %q", got)
	}
	lv, _ := lengthFn(noCtx, []interpreter.Value{get(t, rv, "/a/b")})
	if lv.Int != 3 {
		t.Errorf("a/b length = %d", lv.Int)
	}
	if got := strAt(t, rv, "/a/b/2"); got != "z" {
		t.Errorf("a/b/2 = %q", got)
	}
	if got := strAt(t, rv, "/m/k"); got != "v" {
		t.Errorf("m/k = %q", got)
	}
	// keys preserves insertion order.
	kv, _ := keysFn(noCtx, []interpreter.Value{rv})
	if len(kv.List) != 2 || kv.List[0].Str != "a" || kv.List[1].Str != "m" {
		t.Errorf("keys = %v, want [a m]", kv.List)
	}
	// has.
	if h, _ := hasFn(noCtx, []interpreter.Value{rv, interpreter.StringVal("/m/k")}); !h.Bool {
		t.Error("has /m/k should be true")
	}
	if h, _ := hasFn(noCtx, []interpreter.Value{rv, interpreter.StringVal("/m/nope")}); h.Bool {
		t.Error("has /m/nope should be false")
	}
}

func TestMultiDocument(t *testing.T) {
	docs, err := decodeAllYaml("one: 1\n---\ntwo: 2\n---\nthree: 3\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 3 {
		t.Fatalf("decodeAll = %d docs, want 3", len(docs))
	}
	if got := intAt(t, wrap(docs[1]), "/two"); got != 2 {
		t.Errorf("doc1 two = %d", got)
	}
	// decode of a multi-document stream is an error pointing at decodeAll.
	if _, err := decodeYaml("a: 1\n---\nb: 2\n"); err == nil || !strings.Contains(err.Error(), "decodeAll") {
		t.Errorf("multi-doc decode error = %v, want a decodeAll hint", err)
	}
	// empty stream decodes to null.
	v, err := decodeYaml("")
	if err != nil || v.Kind != interpreter.KindNull {
		t.Errorf("empty decode = (%v, %v), want (null, nil)", v.Kind, err)
	}
}

func TestAnchorsAndMerge(t *testing.T) {
	// Alias resolves by value.
	rv := mustDecode(t, "base: &b [1, 2, 3]\ncopy: *b\n")
	if got := intAt(t, rv, "/copy/2"); got != 3 {
		t.Errorf("copy/2 = %d", got)
	}
	// Merge key: explicit key wins, others merged in.
	mv := mustDecode(t, "def: &d\n  timeout: 30\n  retries: 3\nprod:\n  <<: *d\n  timeout: 60\n")
	if got := intAt(t, mv, "/prod/timeout"); got != 60 {
		t.Errorf("prod/timeout = %d, want 60 (own wins)", got)
	}
	if got := intAt(t, mv, "/prod/retries"); got != 3 {
		t.Errorf("prod/retries = %d, want 3 (merged)", got)
	}
	// Sequence merge: earlier source wins.
	sv := mustDecode(t, "one: &a {x: 1, y: 1}\ntwo: &c {y: 2, z: 2}\nboth:\n  <<: [*a, *c]\n")
	if got := intAt(t, sv, "/both/x"); got != 1 {
		t.Errorf("both/x = %d", got)
	}
	if got := intAt(t, sv, "/both/y"); got != 1 {
		t.Errorf("both/y = %d, want 1 (first source wins)", got)
	}
	if got := intAt(t, sv, "/both/z"); got != 2 {
		t.Errorf("both/z = %d", got)
	}
}

func TestEncodeFlowBlock(t *testing.T) {
	rv := mustDecode(t, "name: hi\nitems:\n  - 1\n  - 2\n")
	flow, err := encodeYaml(rv, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(flow, "{name: hi") || !strings.Contains(flow, "[1, 2]") {
		t.Errorf("flow encode = %q, want compact flow style", flow)
	}
	block, err := encodeYaml(rv, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(block, "name: hi\n") || !strings.Contains(block, "- 1\n") {
		t.Errorf("block encode = %q, want block style", block)
	}
	// Round-trip through block style preserves values.
	back, err := decodeYaml(block)
	if err != nil {
		t.Fatal(err)
	}
	if got := intAt(t, wrap(back), "/items/1"); got != 2 {
		t.Errorf("round-trip items/1 = %d", got)
	}
}

func TestWriteSurface(t *testing.T) {
	empty, _ := mapFn(noCtx, nil)
	// set a scalar
	step, err := setFn(noCtx, []interpreter.Value{empty, interpreter.StringVal("/host"), interpreter.StringVal("localhost")})
	if err != nil {
		t.Fatal(err)
	}
	// nested list grown by append
	lst, _ := listFn(noCtx, nil)
	step, _ = setFn(noCtx, []interpreter.Value{step, interpreter.StringVal("/ports"), lst})
	step, _ = appendFn(noCtx, []interpreter.Value{step, interpreter.StringVal("/ports"), interpreter.IntVal(80)})
	step, err = appendFn(noCtx, []interpreter.Value{step, interpreter.StringVal("/ports"), interpreter.IntVal(443)})
	if err != nil {
		t.Fatal(err)
	}
	if got := strAt(t, step, "/host"); got != "localhost" {
		t.Errorf("host = %q", got)
	}
	if got := intAt(t, step, "/ports/1"); got != 443 {
		t.Errorf("ports/1 = %d", got)
	}
	// original empty map is untouched (non-mutating).
	lv, _ := lengthFn(noCtx, []interpreter.Value{empty})
	if lv.Int != 0 {
		t.Errorf("empty was mutated: %d keys", lv.Int)
	}
	// insert at index, remove, move.
	step, _ = insertFn(noCtx, []interpreter.Value{step, interpreter.StringVal("/ports/0"), interpreter.IntVal(22)})
	if got := intAt(t, step, "/ports/0"); got != 22 {
		t.Errorf("after insert ports/0 = %d", got)
	}
	step, _ = removeFn(noCtx, []interpreter.Value{step, interpreter.StringVal("/ports/0")})
	if got := intAt(t, step, "/ports/0"); got != 80 {
		t.Errorf("after remove ports/0 = %d", got)
	}
	step, err = moveFn(noCtx, []interpreter.Value{step, interpreter.StringVal("/host"), interpreter.StringVal("/hostname")})
	if err != nil {
		t.Fatal(err)
	}
	if got := strAt(t, step, "/hostname"); got != "localhost" {
		t.Errorf("after move hostname = %q", got)
	}
	if h, _ := hasFn(noCtx, []interpreter.Value{step, interpreter.StringVal("/host")}); h.Bool {
		t.Error("host should be gone after move")
	}
}

func TestDatetimeAccessor(t *testing.T) {
	rv := mustDecode(t, "when: 2001-12-15T02:59:43Z\n")
	dv, err := isDatetimeFn(noCtx, []interpreter.Value{rv, interpreter.StringVal("/when")})
	if err != nil || !dv.Bool {
		t.Fatalf("isDatetime = (%v, %v)", dv.Bool, err)
	}
	tv, err := asDatetimeFn(noCtx, []interpreter.Value{get(t, rv, "/when")})
	if err != nil {
		t.Fatal(err)
	}
	if tv.Kind != interpreter.KindStruct || tv.StructNS != "time" || tv.StructName != "Time" {
		t.Fatalf("asDatetime = %s %s.%s, want a time.Time", tv.Kind, tv.StructNS, tv.StructName)
	}
	var nanos int64
	for _, f := range tv.Fields {
		if f.Name == "nanos" {
			nanos = f.Value.Int
		}
	}
	// 2001-12-15T02:59:43Z in Unix nanoseconds.
	if nanos != 1008385183000000000 {
		t.Errorf("nanos = %d", nanos)
	}
}

func TestBytesBinary(t *testing.T) {
	// An explicit !!binary scalar decodes to bytes.
	rv := mustDecode(t, "data: !!binary aGk=\n")
	if got := typeAt(t, rv, "/data"); got != "bytes" {
		t.Errorf("!!binary type = %q, want bytes", got)
	}
	bnode := get(t, rv, "/data")
	inner, _ := bnode.AsObject(LibraryName, "Value")
	if inner.Kind != interpreter.KindBytes || string(inner.Bytes) != "hi" {
		t.Fatalf("decoded !!binary = %s %q", inner.Kind, inner.Bytes)
	}
	// A raw bytes value encodes to a !!binary scalar and round-trips.
	m := mapVal([]interpreter.MapEntry{{Key: interpreter.StringVal("blob"), Value: interpreter.BytesVal([]byte("hi"))}})
	text, err := encodeYaml(m, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "!!binary") || !strings.Contains(text, "aGk=") {
		t.Errorf("encoded bytes = %q, want a !!binary aGk= scalar", text)
	}
	back, err := decodeYaml(text)
	if err != nil {
		t.Fatal(err)
	}
	rb := get(t, wrap(back), "/blob")
	ib, _ := rb.AsObject(LibraryName, "Value")
	if ib.Kind != interpreter.KindBytes || string(ib.Bytes) != "hi" {
		t.Errorf("round-trip bytes = %s %q", ib.Kind, ib.Bytes)
	}
}

func TestDecodeErrors(t *testing.T) {
	cases := []string{
		"a: [1, 2\n",      // malformed flow sequence
		"a: 1\n b: 2\n",   // bad indentation
		"? [a, b]\n: c\n", // complex (non-scalar) mapping key
	}
	for _, src := range cases {
		if _, err := decodeYaml(src); err == nil {
			t.Errorf("decode %q: expected an error", src)
		}
	}
}

func TestDepthGuardRejectsDeepNesting(t *testing.T) {
	// yaml.v3 is a recursive-descent parser; deeply-nested input overflows a
	// fixed goroutine stack (a fatal, uncatchable crash under jennifer-tiny).
	// guardDepth must turn every nesting style into a catchable error before the
	// parse runs.
	deep := func(open, close string, times int) string {
		return "a: " + strings.Repeat(open, times) + strings.Repeat(close, times)
	}
	attacks := map[string]string{
		"flow-sequence": deep("[", "]", maxParseDepth+50),
		"flow-mapping":  "a: " + strings.Repeat("{a: ", maxParseDepth+50) + "1" + strings.Repeat("}", maxParseDepth+50),
		"compact-dash":  strings.Repeat("- ", maxParseDepth+50) + "x",
	}
	// block-indent attack: one deeper key per line.
	var block strings.Builder
	for i := 0; i < maxParseDepth+50; i++ {
		block.WriteString(strings.Repeat(" ", i))
		block.WriteString("a:\n")
	}
	attacks["block-indent"] = block.String()

	for name, src := range attacks {
		if _, err := decodeYaml(src); err == nil || !strings.Contains(err.Error(), "too deep") {
			t.Errorf("%s: expected a depth error, got %v", name, err)
		}
	}

	// A document just under the cap decodes fine (no over-eager rejection).
	if _, err := decodeYaml(deep("[", "]", maxParseDepth-5)); err != nil {
		t.Errorf("just-under-cap flow rejected: %v", err)
	}
	// A quoted scalar full of brackets is not structure and must not be counted.
	braces := "x: \"" + strings.Repeat("[", maxParseDepth+50) + "\"\n"
	if _, err := decodeYaml(braces); err != nil {
		t.Errorf("bracket-heavy quoted string wrongly rejected: %v", err)
	}
}

func TestNodeTypeVocabulary(t *testing.T) {
	rv := mustDecode(t, "list:\n  - 1\nmap:\n  k: v\n")
	if got := typeAt(t, rv, "/list"); got != "list" {
		t.Errorf("sequence reported as %q, want list", got)
	}
	if got := typeAt(t, rv, "/map"); got != "map" {
		t.Errorf("mapping reported as %q, want map", got)
	}
}

// guardDepth must reject deep nesting of every structural kind, protecting
// yaml.v3's parser stack, while a block scalar's increasingly-indented content
// (literal text, not nesting) must NOT be mistaken for depth.
func TestGuardDepth(t *testing.T) {
	deep := []string{
		strings.Repeat("[", maxParseDepth+50) + "x" + strings.Repeat("]", maxParseDepth+50), // flow
		strings.Repeat("- ", maxParseDepth+50) + "x",                                        // compact seq
		buildIndentedNesting(maxParseDepth + 50),                                            // block mapping
	}
	for _, src := range deep {
		if _, err := decodeYaml(src); err == nil {
			t.Errorf("deep nesting should be rejected: %.40q...", src)
		}
	}
	// A block scalar whose content indentation increases is valid YAML with no
	// real nesting; it must decode, not be rejected as "too deep".
	var sb strings.Builder
	sb.WriteString("text: |\n")
	for i := 0; i < maxParseDepth*3; i++ {
		sb.WriteString("  ")
		sb.WriteString(strings.Repeat(" ", i))
		sb.WriteString("x\n")
	}
	if _, err := decodeYaml(sb.String()); err != nil {
		t.Errorf("a deeply-indented block scalar is valid and must decode: %v", err)
	}
	// A plain scalar ending in '|' is NOT a block scalar, so deep flow after it
	// must still be caught (no bypass).
	bypass := "key: value|\n  " + strings.Repeat("[", maxParseDepth+50) + "x"
	if _, err := decodeYaml(bypass); err == nil {
		t.Error("deep flow after a non-block-scalar line must still be rejected")
	}
}

func buildIndentedNesting(depth int) string {
	var sb strings.Builder
	for i := 0; i < depth; i++ {
		sb.WriteString(strings.Repeat(" ", i))
		sb.WriteString("k:\n")
	}
	sb.WriteString(strings.Repeat(" ", depth))
	sb.WriteString("v: x")
	return sb.String()
}

// An alias bomb expands exponentially only when followed by value; the node
// budget must turn it into a catchable error, and a cyclic alias must hit the
// recursion cap - neither may OOM or hang.
func TestAliasBombAndCycle(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("l0: &a0 [x,x,x,x,x,x,x,x,x]\n")
	for i := 1; i <= 22; i++ {
		sb.WriteString("l")
		sb.WriteString(itoa(i))
		sb.WriteString(": &a")
		sb.WriteString(itoa(i))
		sb.WriteString(" [")
		for j := 0; j < 9; j++ {
			if j > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString("*a")
			sb.WriteString(itoa(i - 1))
		}
		sb.WriteString("]\n")
	}
	if _, err := decodeYaml(sb.String()); err == nil {
		t.Error("alias bomb should be rejected by the node budget")
	}
	if _, err := decodeYaml("a: &x\n  self: *x\n"); err == nil {
		t.Error("cyclic alias should be rejected by the depth cap")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// Duplicate mapping keys are rejected (as json / toml / xml decode do).
func TestDuplicateKeyRejected(t *testing.T) {
	if _, err := decodeYaml("a: 1\nb: 2\na: 3"); err == nil {
		t.Error("duplicate mapping key should be rejected")
	}
	// A merge key supplying an already-present key is fine (own key wins).
	if _, err := decodeYaml("base: &b {x: 1}\nd:\n  x: 2\n  <<: *b"); err != nil {
		t.Errorf("merge overlapping an explicit key should be allowed: %v", err)
	}
}

// A string whose text resolves to another type must round-trip as a string:
// the encoder tags it !!str so yaml.v3 quotes it.
func TestEncodeQuotesTypeAmbiguousStrings(t *testing.T) {
	for _, s := range []string{"true", "false", "null", "~", "123", "1.5"} {
		rv := mustDecode(t, "v: \""+s+"\"")
		enc, err := encodeFn([]interpreter.Value{rv}, false)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		back, err := decodeYaml(enc.Str)
		if err != nil {
			t.Fatalf("re-decode %q: %v", enc.Str, err)
		}
		if got := typeAt(t, wrap(back), "/v"); got != "string" {
			t.Errorf("%q round-tripped to %s, want string (encoded: %s)", s, got, enc.Str)
		}
	}
}

// An integer too large for int64 keeps its exact digits (as a string), not a
// lossy float; genuine floats and in-range ints are unaffected.
func TestHugeIntKeepsExactDigits(t *testing.T) {
	rv := mustDecode(t, "big: 99999999999999999999999999\nneg: -12345678901234567890123")
	if got := typeAt(t, rv, "/big"); got != "string" {
		t.Errorf("huge int type = %s, want string", got)
	}
	if got := strAt(t, rv, "/big"); got != "99999999999999999999999999" {
		t.Errorf("huge int lost digits: %q", got)
	}
	if got := typeAt(t, rv, "/neg"); got != "string" {
		t.Errorf("huge negative int type = %s, want string", got)
	}
	ok := mustDecode(t, "f: 3.14\nsci: 1e6\nn: 42")
	if got := typeAt(t, ok, "/f"); got != "float" {
		t.Errorf("3.14 type = %s, want float", got)
	}
	if got := typeAt(t, ok, "/n"); got != "int" {
		t.Errorf("42 type = %s, want int", got)
	}
}
