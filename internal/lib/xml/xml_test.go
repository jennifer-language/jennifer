// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package xmllib

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

var noCtx = interpreter.BuiltinCtx{}

func mustDecode(t *testing.T, src string) interpreter.Value {
	t.Helper()
	root, err := decodeXML(src)
	if err != nil {
		t.Fatalf("decode %q: %v", src, err)
	}
	return wrap(root)
}

func strGetter(t *testing.T) func(interpreter.Value, error) string {
	return func(v interpreter.Value, err error) string {
		t.Helper()
		if err != nil {
			t.Fatalf("accessor error: %v", err)
		}
		if v.Kind != interpreter.KindString {
			t.Fatalf("expected string, got %s", v.Kind)
		}
		return v.Str
	}
}

func TestDecodeAndAccess(t *testing.T) {
	str := strGetter(t)
	rv := mustDecode(t, `<library><book id="1" lang="en"><title>Go</title></book><book id="2"><title>XML</title></book></library>`)

	if got := str(tagFn(noCtx, []interpreter.Value{rv})); got != "library" {
		t.Errorf("root tag = %q", got)
	}
	all, err := findAllFn(noCtx, []interpreter.Value{rv, interpreter.StringVal("book")})
	if err != nil || len(all.List) != 2 {
		t.Fatalf("findAll book = %d elems, %v", len(all.List), err)
	}
	// book[2] via index path, then its attr and title text.
	bk, err := getFn(noCtx, []interpreter.Value{rv, interpreter.StringVal("book[2]")})
	if err != nil {
		t.Fatal(err)
	}
	if got := str(attrFn(noCtx, []interpreter.Value{bk, interpreter.StringVal("id")})); got != "2" {
		t.Errorf("book[2] id = %q", got)
	}
	title, _ := getFn(noCtx, []interpreter.Value{bk, interpreter.StringVal("title")})
	if got := str(textFn(noCtx, []interpreter.Value{title})); got != "XML" {
		t.Errorf("book[2] title = %q", got)
	}
	// has / children / attrs.
	if h, _ := hasFn(noCtx, []interpreter.Value{rv, interpreter.StringVal("book/title")}); !h.Bool {
		t.Error("has book/title should be true")
	}
	if h, _ := hasFn(noCtx, []interpreter.Value{rv, interpreter.StringVal("book/author")}); h.Bool {
		t.Error("has book/author should be false")
	}
	kids, _ := childrenFn(noCtx, []interpreter.Value{rv})
	if len(kids.List) != 2 {
		t.Errorf("children = %d, want 2", len(kids.List))
	}
	attrs, _ := attrsFn(noCtx, []interpreter.Value{mustGet(t, rv, "book[1]")})
	if len(attrs.List) != 2 || attrs.List[0].Str != "id" || attrs.List[1].Str != "lang" {
		t.Errorf("book[1] attrs = %v, want [id lang]", attrs.List)
	}
}

func mustGet(t *testing.T, rv interpreter.Value, path string) interpreter.Value {
	t.Helper()
	v, err := getFn(noCtx, []interpreter.Value{rv, interpreter.StringVal(path)})
	if err != nil {
		t.Fatalf("get %q: %v", path, err)
	}
	return v
}

func TestEntitiesAndCDATA(t *testing.T) {
	str := strGetter(t)
	rv := mustDecode(t, `<r a="x &lt; y &amp; z"><t>1 &lt; 2 &#65; &#x42;</t><c><![CDATA[raw <&> text]]></c></r>`)
	if got := str(attrFn(noCtx, []interpreter.Value{rv, interpreter.StringVal("a")})); got != "x < y & z" {
		t.Errorf("attr entities = %q", got)
	}
	if got := str(textFn(noCtx, []interpreter.Value{mustGet(t, rv, "t")})); got != "1 < 2 A B" {
		t.Errorf("text entities = %q", got)
	}
	if got := str(textFn(noCtx, []interpreter.Value{mustGet(t, rv, "c")})); got != "raw <&> text" {
		t.Errorf("CDATA text = %q", got)
	}
	// An unknown entity is a decode error.
	if _, err := decodeXML(`<r>&nope;</r>`); err == nil {
		t.Error("unknown entity should error")
	}
}

func TestSelfClosingAndMixed(t *testing.T) {
	str := strGetter(t)
	rv := mustDecode(t, `<p>Hello <b>world</b><br/>!</p>`)
	// text() concatenates the direct text children only.
	if got := str(textFn(noCtx, []interpreter.Value{rv})); got != "Hello !" {
		t.Errorf("mixed text = %q", got)
	}
	kids, _ := childrenFn(noCtx, []interpreter.Value{rv})
	if len(kids.List) != 2 { // <b> and <br/>
		t.Errorf("mixed element children = %d, want 2", len(kids.List))
	}
	// self-closing <br/> has no children.
	brk, _ := childrenFn(noCtx, []interpreter.Value{kids.List[1]})
	if len(brk.List) != 0 {
		t.Errorf("<br/> children = %d, want 0", len(brk.List))
	}
}

func TestRoundTrip(t *testing.T) {
	// Canonical input (double-quoted attrs, entities, no comments) round-trips.
	cases := []string{
		`<a/>`,
		`<a x="1"/>`,
		`<a><b>t</b><b>u</b></a>`,
		`<p>a &amp; b &lt; c</p>`,
		`<ns:a xmlns:ns="urn:x"><ns:b>y</ns:b></ns:a>`,
	}
	for _, in := range cases {
		root, err := decodeXML(in)
		if err != nil {
			t.Fatalf("decode %q: %v", in, err)
		}
		var sb strings.Builder
		if err := encodeNode(&sb, root, false, 0); err != nil {
			t.Fatalf("encode %q: %v", in, err)
		}
		if sb.String() != in {
			t.Errorf("round-trip: %q -> %q", in, sb.String())
		}
	}
}

func TestDecodeErrors(t *testing.T) {
	bad := []string{
		`<a></b>`,              // mismatched close
		`<a>`,                  // unclosed
		`<a/><b/>`,             // two roots
		`<a/> trailing junk x`, // content after root (non-space)
		`<a x=1/>`,             // unquoted attr value
		`<a x="1" x="2"/>`,     // duplicate attribute
		`no root here`,         // no element
		``,                     // empty
		`<a>&#xZZ;</a>`,        // bad char reference
	}
	for _, in := range bad {
		if _, err := decodeXML(in); err == nil {
			t.Errorf("expected error decoding %q", in)
		}
	}
}

func TestBuildSurface(t *testing.T) {
	e, err := elementFn(noCtx, []interpreter.Value{interpreter.StringVal("note")})
	if err != nil {
		t.Fatal(err)
	}
	e, _ = setAttrFn(noCtx, []interpreter.Value{e, interpreter.StringVal("type"), interpreter.StringVal("info")})
	e, _ = setAttrFn(noCtx, []interpreter.Value{e, interpreter.StringVal("type"), interpreter.StringVal("warn")}) // update
	e, _ = setTextFn(noCtx, []interpreter.Value{e, interpreter.StringVal("hi <there> & you")})
	var sb strings.Builder
	inner, _ := asNode(e)
	encodeNode(&sb, inner, false, 0)
	if got := sb.String(); got != `<note type="warn">hi &lt;there&gt; &amp; you</note>` {
		t.Errorf("built = %q", got)
	}
	// append into a fresh parent; the child tree is not mutated.
	parent, _ := elementFn(noCtx, []interpreter.Value{interpreter.StringVal("outer")})
	parent, _ = appendFn(noCtx, []interpreter.Value{parent, e})
	pinner, _ := asNode(parent)
	var sb2 strings.Builder
	encodeNode(&sb2, pinner, false, 0)
	if !strings.HasPrefix(sb2.String(), "<outer><note") {
		t.Errorf("appended = %q", sb2.String())
	}
	// invalid names are rejected.
	if _, err := elementFn(noCtx, []interpreter.Value{interpreter.StringVal("bad name")}); err == nil {
		t.Error("element with a space should error")
	}
	if _, err := setAttrFn(noCtx, []interpreter.Value{parent, interpreter.StringVal("a=b"), interpreter.StringVal("x")}); err == nil {
		t.Error("attr name with '=' should error")
	}
}

func TestPathDialect(t *testing.T) {
	str := strGetter(t)
	rv := mustDecode(t, `<r><a><x>1</x><x>2</x></a><a><y>3</y></a></r>`)
	// wildcard: any child of r.
	all, _ := findAllFn(noCtx, []interpreter.Value{rv, interpreter.StringVal("*")})
	if len(all.List) != 2 {
		t.Errorf("* = %d, want 2", len(all.List))
	}
	// duplicated element children across siblings: a/x matches both x under the first a.
	xs, _ := findAllFn(noCtx, []interpreter.Value{rv, interpreter.StringVal("a/x")})
	if len(xs.List) != 2 {
		t.Errorf("a/x = %d, want 2", len(xs.List))
	}
	if got := str(textFn(noCtx, []interpreter.Value{mustGet(t, rv, "a[1]/x[2]")})); got != "2" {
		t.Errorf("a[1]/x[2] = %q", got)
	}
	// empty path returns the node itself.
	self := mustGet(t, rv, "")
	if got := str(tagFn(noCtx, []interpreter.Value{self})); got != "r" {
		t.Errorf("empty path self = %q", got)
	}
	// malformed path errors; no-match get errors; no-match has is false.
	if _, err := getFn(noCtx, []interpreter.Value{rv, interpreter.StringVal("a[0]")}); err == nil {
		t.Error("index 0 should error (1-based)")
	}
	if _, err := getFn(noCtx, []interpreter.Value{rv, interpreter.StringVal("nope")}); err == nil {
		t.Error("no-match get should error")
	}
}

// Deeply-nested input must fail with a catchable error, not overflow the Go
// stack (uncatchable, since the interpreter has no recover()).
func TestDecodeDepthCap(t *testing.T) {
	deep := strings.Repeat("<a>", maxDepth+50) + "x" + strings.Repeat("</a>", maxDepth+50)
	if _, err := decodeXML(deep); err == nil {
		t.Error("over-deep document should error")
	} else if !strings.Contains(err.Error(), "nesting exceeds") {
		t.Errorf("want a nesting-depth error, got %v", err)
	}
	// A document right at the cap still decodes.
	ok := strings.Repeat("<a>", maxDepth-1) + "x" + strings.Repeat("</a>", maxDepth-1)
	if _, err := decodeXML(ok); err != nil {
		t.Errorf("document within the cap should decode: %v", err)
	}
}

// Encoding a tree built deeper than the cap must error, not overflow the stack.
func TestEncodeDepthCap(t *testing.T) {
	deep := elementNode("a", nil, nil)
	for i := 0; i < maxDepth+50; i++ {
		deep = elementNode("a", nil, []node{deep})
	}
	var sb strings.Builder
	if err := encodeNode(&sb, deep, false, 0); err == nil {
		t.Error("encoding an over-deep tree should error")
	}
}

// A name may not contain characters that terminate other syntax; accepting them
// would let a malformed name round-trip through encode as un-escaped markup.
func TestDecodeRejectsInvalidNameChars(t *testing.T) {
	for _, src := range []string{`<a&b>x</a&b>`, `<a b&c="v"/>`, `<a"b/>`, `<a'b/>`} {
		if _, err := decodeXML(src); err == nil {
			t.Errorf("expected error decoding %q (invalid name char)", src)
		}
	}
}

// Numeric character references outside XML 1.0's Char production (e.g. NUL and
// other C0 controls) are rejected; tab/LF/CR and normal code points pass.
func TestCharRefValidation(t *testing.T) {
	for _, bad := range []string{`<r>&#0;</r>`, `<r>&#8;</r>`, `<r>&#xB;</r>`, `<r>&#xFFFE;</r>`} {
		if _, err := decodeXML(bad); err == nil {
			t.Errorf("expected error decoding illegal char ref %q", bad)
		}
	}
	for _, ok := range []string{`<r>&#9;</r>`, `<r>&#10;</r>`, `<r>&#65;</r>`, `<r>&#x41;</r>`} {
		if _, err := decodeXML(ok); err != nil {
			t.Errorf("valid char ref %q should decode: %v", ok, err)
		}
	}
}

// Attribute values normalize literal whitespace to a space, while the encoder
// escapes whitespace as character references so exact bytes survive a round-trip.
func TestAttrWhitespaceNormalization(t *testing.T) {
	get := strGetter(t)
	// Literal tab/newline in the source normalize to spaces.
	rv := mustDecode(t, "<x a=\"p\nq\tr\"/>")
	if got := get(attrFn(noCtx, []interpreter.Value{rv, interpreter.StringVal("a")})); got != "p q r" {
		t.Errorf("literal whitespace not normalized: %q", got)
	}
	// A built value with whitespace encodes to refs and round-trips exactly.
	el, _ := setAttrFn(noCtx, []interpreter.Value{wrap(elementNode("x", nil, nil)), interpreter.StringVal("a"), interpreter.StringVal("p\nq")})
	enc := get(encodeFn([]interpreter.Value{el}, false))
	if !strings.Contains(enc, "&#10;") {
		t.Errorf("newline should encode as a reference, got %q", enc)
	}
	back := mustDecode(t, enc)
	if got := get(attrFn(noCtx, []interpreter.Value{back, interpreter.StringVal("a")})); got != "p\nq" {
		t.Errorf("round-trip lost the newline: %q", got)
	}
}

func TestDecodeNodeBudget(t *testing.T) {
	saved := maxDecodedNodes
	maxDecodedNodes = 50
	defer func() { maxDecodedNodes = saved }()
	bomb := "<a>" + strings.Repeat("<b/>", 200) + "</a>"
	if _, err := decodeXML(bomb); err == nil || !strings.Contains(err.Error(), "decode bomb") {
		t.Errorf("over-budget document: expected a decode-bomb error, got %v", err)
	}
	if _, err := decodeXML("<a><b/></a>"); err != nil {
		t.Errorf("within-budget document should decode, got %v", err)
	}
}

// Text and attribute values with nothing to decode are taken as-is (the
// substring fast path); ones with references or whitespace still decode.
func TestDecodeFastAndSlowPaths(t *testing.T) {
	str := strGetter(t)
	rv := mustDecode(t, "<r a=\"plain\" b=\"x &amp; y\" c=\"p\tq\">one<i/>two &lt;3</r>")
	for name, want := range map[string]string{"a": "plain", "b": "x & y", "c": "p q"} {
		if got := str(attrFn(noCtx, []interpreter.Value{rv, interpreter.StringVal(name)})); got != want {
			t.Errorf("attr %s = %q, want %q", name, got, want)
		}
	}
	if got := str(textFn(noCtx, []interpreter.Value{rv})); got != "onetwo <3" {
		t.Errorf("text = %q", got)
	}
	if got := str(encodeFn([]interpreter.Value{rv}, false)); got != `<r a="plain" b="x &amp; y" c="p q">one<i/>two &lt;3</r>` {
		t.Errorf("re-encode = %q", got)
	}
}

// Equality compares content: attribute order does not matter, child order
// does, and a decoded document equals the same document built by hand.
func TestValueEquality(t *testing.T) {
	a := mustDecode(t, `<p x="1" y="2"><q>t</q></p>`)
	b := mustDecode(t, `<p y="2" x="1"><q>t</q></p>`)
	if !a.Equal(b) {
		t.Error("attribute order should not affect equality")
	}
	if a.Equal(mustDecode(t, `<p x="1" y="2"><q>u</q></p>`)) {
		t.Error("different text should not be equal")
	}
	if mustDecode(t, `<p><a/><b/></p>`).Equal(mustDecode(t, `<p><b/><a/></p>`)) {
		t.Error("child order should matter")
	}
	built, _ := elementFn(noCtx, []interpreter.Value{interpreter.StringVal("q")})
	built, _ = setTextFn(noCtx, []interpreter.Value{built, interpreter.StringVal("t")})
	if !built.Equal(mustGet(t, a, "q")) {
		t.Error("built element should equal the decoded one")
	}
}

// The build surface never mutates the element it was given, even when the
// result is appended to or re-attributed.
func TestBuildSurfaceSharesSafely(t *testing.T) {
	str := strGetter(t)
	base := mustDecode(t, `<e k="v"><c/></e>`)
	_, _ = setAttrFn(noCtx, []interpreter.Value{base, interpreter.StringVal("k"), interpreter.StringVal("w")})
	extra, _ := elementFn(noCtx, []interpreter.Value{interpreter.StringVal("d")})
	_, _ = appendFn(noCtx, []interpreter.Value{base, extra})
	_, _ = setTextFn(noCtx, []interpreter.Value{base, interpreter.StringVal("x")})
	if got := str(encodeFn([]interpreter.Value{base}, false)); got != `<e k="v"><c/></e>` {
		t.Errorf("source element changed: %q", got)
	}
}

// An element with a huge attribute list decodes and compares in linear time,
// and a duplicate is still found past the small-list scan.
func TestManyAttributes(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("<r")
	for i := 0; i < 80000; i++ {
		fmt.Fprintf(&sb, ` a%d=""`, i)
	}
	sb.WriteString("/>")
	start := time.Now()
	a := mustDecode(t, sb.String())
	b := mustDecode(t, sb.String())
	if !a.Equal(b) {
		t.Error("identical documents should be equal")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("decode + compare of 80k attributes took %v", d)
	}
	if _, err := decodeXML(`<r a0="" a1="" a2="" a3="" a4="" a5="" a6="" a7="" a8="" a9="" a3=""/>`); err == nil || !strings.Contains(err.Error(), "duplicate attribute") {
		t.Errorf("duplicate past the scan threshold: %v", err)
	}
}

// xml.append refuses to build a tree deeper than the nesting cap, so equality
// and encoding (one Go frame per level) stay bounded.
func TestAppendDepthCap(t *testing.T) {
	v, _ := elementFn(noCtx, []interpreter.Value{interpreter.StringVal("a")})
	var err error
	for i := 0; i < maxDepth+5 && err == nil; i++ {
		p, _ := elementFn(noCtx, []interpreter.Value{interpreter.StringVal("a")})
		var next interpreter.Value
		next, err = appendFn(noCtx, []interpreter.Value{p, v})
		if err == nil {
			v = next
		}
	}
	if err == nil || !strings.Contains(err.Error(), "nest deeper") {
		t.Errorf("deep append: %v", err)
	}
	n, _ := asNode(v)
	if n.height() != maxDepth {
		t.Errorf("deepest tree built has height %d, want %d", n.height(), maxDepth)
	}
}
