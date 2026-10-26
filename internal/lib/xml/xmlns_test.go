// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package xmllib

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

const (
	kmlNS  = "http://www.opengis.net/kml/2.2"
	atomNS = "http://www.w3.org/2005/Atom"
	dcNS   = "http://purl.org/dc/elements/1.1/"
)

func sval(s string) interpreter.Value { return interpreter.StringVal(s) }

func call(t *testing.T, fn interpreter.Builtin, args ...interpreter.Value) interpreter.Value {
	t.Helper()
	v, err := fn(noCtx, args)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	return v
}

func nsMap(kv ...string) interpreter.Value {
	var entries []interpreter.MapEntry
	for i := 0; i < len(kv); i += 2 {
		entries = append(entries, interpreter.MapEntry{Key: sval(kv[i]), Value: sval(kv[i+1])})
	}
	str := parser.PrimitiveType(parser.TypeString)
	return interpreter.MapVal(str, str, entries)
}

func encoded(t *testing.T, v interpreter.Value) string {
	t.Helper()
	return call(t, func(_ interpreter.BuiltinCtx, a []interpreter.Value) (interpreter.Value, error) {
		return encodeFn(a, false)
	}, v).Str
}

// Names resolve against the declarations in scope: an unprefixed element takes
// the default namespace, an unprefixed attribute none, xmlns="" undeclares the
// default, a nested declaration shadows an outer one, and xml: is implicit.
func TestNamespaceResolution(t *testing.T) {
	doc := mustDecode(t, `<r xmlns="urn:d" xmlns:p="urn:p" a="1" p:b="2" xml:lang="en">`+
		`<c/><p:c/><e xmlns=""><f/></e><g xmlns:p="urn:q"><p:h/></g></r>`)
	cases := []struct{ path, ns, local string }{
		{"", "urn:d", "r"},
		{"c", "urn:d", "c"},
		{"p:c", "urn:p", "c"},
		{"e", "", "e"},
		{"e/f", "", "f"},
		{"g/p:h", "urn:q", "h"},
	}
	for _, c := range cases {
		n := mustGet(t, doc, c.path)
		if got := call(t, namespaceFn, n).Str; got != c.ns {
			t.Errorf("namespace(%q) = %q, want %q", c.path, got, c.ns)
		}
		if got := call(t, localNameFn, n).Str; got != c.local {
			t.Errorf("localName(%q) = %q, want %q", c.path, got, c.local)
		}
	}
	for _, c := range []struct{ uri, local, want string }{{"", "a", "1"}, {"urn:p", "b", "2"}, {xmlNamespace, "lang", "en"}} {
		if got := call(t, attrNSFn, doc, sval(c.uri), sval(c.local)).Str; got != c.want {
			t.Errorf("attrNS(%q, %q) = %q, want %q", c.uri, c.local, got, c.want)
		}
	}
	// An unprefixed attribute is not in the default namespace.
	if call(t, hasAttrNSFn, doc, sval("urn:d"), sval("a")).Bool {
		t.Error("unprefixed attribute a must not be in the default namespace")
	}
	if call(t, hasAttrNSFn, doc, sval("urn:p"), sval("a")).Bool {
		t.Error("hasAttrNS({urn:p}a) should be false")
	}
	// The qualified-name accessors are unchanged.
	if got := call(t, tagFn, mustGet(t, doc, "p:c")).Str; got != "p:c" {
		t.Errorf("tag = %q", got)
	}
}

// A sub-tree handle keeps the bindings in scope where it was decoded.
func TestLookupPrefixOnSubtree(t *testing.T) {
	doc := mustDecode(t, `<r xmlns="urn:d" xmlns:p="urn:p"><s xmlns:q="urn:q"><t/></s></r>`)
	sub := mustGet(t, doc, "s/t")
	for prefix, want := range map[string]string{"": "urn:d", "p": "urn:p", "q": "urn:q", "xml": xmlNamespace} {
		if got := call(t, lookupPrefixFn, sub, sval(prefix)).Str; got != want {
			t.Errorf("lookupPrefix(%q) = %q, want %q", prefix, got, want)
		}
	}
	if _, err := lookupPrefixFn(noCtx, []interpreter.Value{sub, sval("zz")}); err == nil {
		t.Error("an unbound prefix should error")
	}
	if got := call(t, lookupPrefixFn, mustDecode(t, `<a/>`), sval("")).Str; got != "" {
		t.Errorf("undeclared default = %q, want empty", got)
	}
}

// Documents that are well-formed XML but not namespace-well-formed are decode
// errors (Namespaces in XML 1.0, the namespace constraints).
func TestNamespaceDecodeErrors(t *testing.T) {
	cases := map[string]string{
		`<p:a/>`:                              "not declared",
		`<a p:x="1"/>`:                        "not declared",
		`<a xmlns:p=""/>`:                     "cannot be undeclared",
		`<a xmlns:xmlns="urn:x"/>`:            "must not be declared",
		`<a xmlns:xml="urn:x"/>`:              "can only be bound",
		`<a xmlns:p="` + xmlNamespace + `"/>`: "only the xml prefix",
		`<a xmlns="` + xmlnsNamespace + `"/>`: "no prefix may be bound",
		`<xmlns:a xmlns:p="urn:p"/>`:          "reserved xmlns prefix",
		`<a:b:c xmlns:a="urn:a"/>`:            "not a valid qualified name",
		`<a :x="1"/>`:                         "not a valid qualified name",
		`<a x:="1"/>`:                         "not a valid qualified name",
		`<a xmlns:p="urn:u" xmlns:q="urn:u" p:x="1" q:x="2"/>`: "same name",
		`<r xmlns:p="urn:p"><a/></r><!-- --><x/>`:              "after the root",
	}
	for src, want := range cases {
		_, err := decodeXML(src)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("decode %s: err = %v, want it to mention %q", src, err, want)
		}
	}
	// Legal: xml bound to its own namespace, a prefix redeclared to a new URI,
	// the default undeclared, and the same local name under two namespaces.
	for _, src := range []string{
		`<a xmlns:xml="` + xmlNamespace + `"/>`,
		`<a xmlns:p="urn:1"><b xmlns:p="urn:2"/></a>`,
		`<a xmlns="urn:d"><b xmlns=""/></a>`,
		`<a xmlns:p="urn:1" xmlns:q="urn:2" p:x="1" q:x="2"/>`,
	} {
		if _, err := decodeXML(src); err != nil {
			t.Errorf("decode %s: %v", src, err)
		}
	}
}

// One prefix-map path reads a default-namespace document and a prefixed one
// alike; without the map, steps match the names as written.
func TestPathPrefixMap(t *testing.T) {
	plain := mustDecode(t, `<kml xmlns="`+kmlNS+`"><Document><Placemark/><Placemark/></Document></kml>`)
	prefixed := mustDecode(t, `<k:kml xmlns:k="`+kmlNS+`"><k:Document><k:Placemark/><k:Placemark/></k:Document></k:kml>`)
	m := nsMap("g", kmlNS)
	for name, doc := range map[string]interpreter.Value{"default": plain, "prefixed": prefixed} {
		got := call(t, findAllFn, doc, sval("g:Document/g:Placemark"), m)
		if len(got.List) != 2 {
			t.Errorf("%s: g:Document/g:Placemark = %d, want 2", name, len(got.List))
		}
		if !call(t, hasFn, doc, sval("g:Document/g:Placemark[2]"), m).Bool {
			t.Errorf("%s: indexed step should match", name)
		}
		if n := len(call(t, findAllFn, doc, sval("*:Document/*:Placemark")).List); n != 2 {
			t.Errorf("%s: *:Document/*:Placemark = %d, want 2", name, n)
		}
	}
	// Verbatim matching without a map is unchanged.
	if n := len(call(t, findAllFn, plain, sval("Document/Placemark")).List); n != 2 {
		t.Errorf("verbatim on default-ns doc = %d, want 2", n)
	}
	if n := len(call(t, findAllFn, prefixed, sval("Document")).List); n != 0 {
		t.Errorf("verbatim unprefixed step on prefixed doc = %d, want 0", n)
	}
	// With a map, an unprefixed step is in no namespace unless "" is mapped.
	if call(t, hasFn, plain, sval("Document"), m).Bool {
		t.Error("unprefixed step with a map should not match a namespaced element")
	}
	if !call(t, hasFn, plain, sval("Document"), nsMap("", kmlNS)).Bool {
		t.Error(`a "" entry should give unprefixed steps that namespace`)
	}
	if _, err := findAllFn(noCtx, []interpreter.Value{plain, sval("x:Document"), m}); err == nil {
		t.Error("a prefix missing from the map should error")
	}
	if _, err := findAllFn(noCtx, []interpreter.Value{plain, sval("*:"), m}); err == nil {
		t.Error("*: with no local name should error")
	}
	if _, err := findAllFn(noCtx, []interpreter.Value{plain, sval("Document"), sval("not a map")}); err == nil {
		t.Error("a non-map namespace argument should error")
	}
}

// A decoded document round-trips byte for byte, and a sub-tree encoded on its
// own carries the declarations it needs (so it decodes again).
func TestEncodeNamespaces(t *testing.T) {
	src := `<r xmlns="urn:d" xmlns:p="urn:p"><p:a p:x="1"><b/></p:a><c xmlns=""><d/></c></r>`
	doc := mustDecode(t, src)
	if got := encoded(t, doc); got != src {
		t.Errorf("round-trip:\n got %s\nwant %s", got, src)
	}
	sub := encoded(t, mustGet(t, doc, "p:a"))
	// Each declaration lands on the element that needs it: p on <p:a>, the
	// default namespace on <b>.
	if want := `<p:a p:x="1" xmlns:p="urn:p"><b xmlns="urn:d"/></p:a>`; sub != want {
		t.Errorf("sub-tree:\n got %s\nwant %s", sub, want)
	}
	if _, err := decodeXML(sub); err != nil {
		t.Errorf("encoded sub-tree does not decode: %v", err)
	}
	// A no-namespace sub-tree under nothing needs no declaration.
	if got := encoded(t, mustGet(t, doc, "c")); got != `<c xmlns=""><d/></c>` {
		t.Errorf("no-namespace sub-tree = %s", got)
	}
}

// elementNS / setAttrNS build namespaced nodes; encode declares each prefix
// once, where it is first needed, and namespace-unaware elements stay verbatim.
func TestBuildNamespaces(t *testing.T) {
	feed := call(t, elementNSFn, sval(atomNS), sval("feed"))
	entry := call(t, elementNSFn, sval(atomNS), sval("entry"))
	creator := call(t, setTextFn, call(t, elementNSFn, sval(dcNS), sval("dc:creator")), sval("ann"))
	entry = call(t, appendFn, entry, creator)
	entry = call(t, setAttrNSFn, entry, sval(dcNS), sval("dc:id"), sval("7"))
	feed = call(t, appendFn, feed, entry)
	want := `<feed xmlns="` + atomNS + `"><entry dc:id="7" xmlns:dc="` + dcNS + `"><dc:creator>ann</dc:creator></entry></feed>`
	if got := encoded(t, feed); got != want {
		t.Errorf("built feed:\n got %s\nwant %s", got, want)
	}
	back := mustDecode(t, want)
	if got := call(t, attrNSFn, mustGet(t, back, "entry"), sval(dcNS), sval("id")).Str; got != "7" {
		t.Errorf("attrNS after round-trip = %q", got)
	}
	if got := encoded(t, back); got != want {
		t.Errorf("re-encoded feed changed:\n got %s\nwant %s", got, want)
	}
	// xml.element children of a hand-declared default stay in it (no xmlns="").
	plain := call(t, setAttrFn, call(t, elementFn, sval("feed")), sval("xmlns"), sval(atomNS))
	plain = call(t, appendFn, plain, call(t, elementFn, sval("entry")))
	if got := encoded(t, plain); got != `<feed xmlns="`+atomNS+`"><entry/></feed>` {
		t.Errorf("namespace-unaware build = %s", got)
	}
	// A no-namespace elementNS under a default namespace undeclares it.
	inner := call(t, appendFn, call(t, elementNSFn, sval(atomNS), sval("feed")), call(t, elementNSFn, sval(""), sval("x")))
	if got := encoded(t, inner); got != `<feed xmlns="`+atomNS+`"><x xmlns=""/></feed>` {
		t.Errorf("no-namespace child = %s", got)
	}
	// setAttrNS replaces the attribute with the same expanded name.
	el := call(t, setAttrNSFn, creator, sval(dcNS), sval("dc:x"), sval("1"))
	el = call(t, setAttrNSFn, el, sval(dcNS), sval("dc:x"), sval("2"))
	if got := call(t, attrNSFn, el, sval(dcNS), sval("x")).Str; got != "2" || len(call(t, attrsFn, el).List) != 1 {
		t.Errorf("setAttrNS replace: value %q, %d attrs", got, len(call(t, attrsFn, el).List))
	}
	for _, bad := range [][]string{{"", "p:a"}, {"urn:x", "xmlns:a"}, {"urn:x", "a:b:c"}, {"urn:x", "xml:a"}, {xmlNamespace, "a"}} {
		if _, err := elementNSFn(noCtx, []interpreter.Value{sval(bad[0]), sval(bad[1])}); err == nil {
			t.Errorf("elementNS(%q, %q) should be refused", bad[0], bad[1])
		}
	}
	if _, err := setAttrNSFn(noCtx, []interpreter.Value{creator, sval("urn:x"), sval("a"), sval("1")}); err == nil {
		t.Error("a namespaced attribute without a prefix should be refused")
	}
	// Two namespaces on one prefix of one element cannot be encoded.
	clash := call(t, setAttrNSFn, call(t, elementNSFn, sval("urn:1"), sval("p:a")), sval("urn:2"), sval("p:x"), sval("1"))
	if _, err := encodeFn([]interpreter.Value{clash}, false); err == nil {
		t.Error("prefix bound to two namespaces on one element should not encode")
	}
}

// Same qualified names in different namespaces are different elements, even
// with identical attributes and content.
func TestNamespaceEquality(t *testing.T) {
	a := mustGet(t, mustDecode(t, `<r xmlns:p="urn:1"><p:a/></r>`), "p:a")
	b := mustGet(t, mustDecode(t, `<r xmlns:p="urn:2"><p:a/></r>`), "p:a")
	c := mustGet(t, mustDecode(t, `<s xmlns:p="urn:1"><p:a/></s>`), "p:a")
	if a.Equal(b) {
		t.Error("p:a in urn:1 should differ from p:a in urn:2")
	}
	if !a.Equal(c) {
		t.Error("p:a in urn:1 from two documents should be equal")
	}
	// A namespace-unaware built element compares by its name as before.
	if !call(t, elementFn, sval("p:a")).Equal(call(t, elementFn, sval("p:a"))) {
		t.Error("built elements with the same name should be equal")
	}
}

// A fragment cut out of a larger document decodes against its root's bindings
// passed as a prefix map; encode then declares them on the fragment.
func TestDecodeFragmentWithPrefixMap(t *testing.T) {
	frag := `<gpx:wpt lat="1"><gpx:name>a</gpx:name><x:ext/></gpx:wpt>`
	if _, err := decodeXML(frag); err == nil {
		t.Fatal("an undeclared prefix should still be an error without a map")
	}
	seed, err := seedScope(map[string]string{"gpx": "urn:gpx", "x": "urn:x", "": "urn:d"})
	if err != nil {
		t.Fatal(err)
	}
	root, err := decodeXMLIn(frag, seed)
	if err != nil {
		t.Fatalf("decode with seed: %v", err)
	}
	v := wrap(root)
	if got := call(t, namespaceFn, mustGet(t, v, "x:ext")).Str; got != "urn:x" {
		t.Errorf("x:ext namespace = %q", got)
	}
	if got := call(t, lookupPrefixFn, v, sval("")).Str; got != "urn:d" {
		t.Errorf("seeded default = %q", got)
	}
	want := `<gpx:wpt lat="1" xmlns:gpx="urn:gpx"><gpx:name>a</gpx:name><x:ext xmlns:x="urn:x"/></gpx:wpt>`
	if got := encoded(t, v); got != want {
		t.Errorf("encoded fragment:\n got %s\nwant %s", got, want)
	}
	for _, bad := range []map[string]string{{"p": ""}, {"xmlns": "urn:x"}, {"xml": "urn:x"}, {"p": xmlNamespace}, {"a:b": "urn:x"}} {
		if _, err := seedScope(bad); err == nil {
			t.Errorf("seed %v should be refused", bad)
		}
	}
}
