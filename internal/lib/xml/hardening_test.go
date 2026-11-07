// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package xmllib

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// A `]` or `>` inside a quoted literal, comment or PI of the DOCTYPE does not
// end it: the root is the element libxml2 and minidom see.
func TestDoctypeSkipsLiteralsAndComments(t *testing.T) {
	str := strGetter(t)
	for _, src := range []string{
		`<!DOCTYPE r [<!ENTITY e "]><a role='admin'/><!--">]><b role="guest"/><!-- -->`,
		`<!DOCTYPE r [<!ENTITY e ']><a role="admin"/>'>]><b role="guest"/>`,
		`<!DOCTYPE r SYSTEM "x>]<a role='admin'/>"><b role="guest"/>`,
		`<!DOCTYPE r [<!-- ]><a role="admin"/> -->]><b role="guest"/>`,
		`<!DOCTYPE r [<?pi ]><a role="admin"/> ?>]><b role="guest"/>`,
	} {
		rv := mustDecode(t, src)
		if got := str(tagFn(noCtx, []interpreter.Value{rv})); got != "b" {
			t.Errorf("%s: root %q, want b", src, got)
			continue
		}
		if got := str(attrFn(noCtx, []interpreter.Value{rv, sval("role")})); got != "guest" {
			t.Errorf("%s: role %q, want guest", src, got)
		}
	}
	for _, src := range []string{`<!DOCTYPE r [<!ENTITY e "]>]><r/>`, `<!DOCTYPE r [<!-- ]><r/>`} {
		if _, err := decodeXML(src); err == nil || !strings.Contains(err.Error(), "unterminated") {
			t.Errorf("%s: err = %v, want unterminated", src, err)
		}
	}
}

// The build surface accepts only XML names, so a name can't encode as markup
// (`<!--/>` opens a comment) or as a qualified name decode rejects.
func TestBuildNamesMustBeXMLNames(t *testing.T) {
	el := func(name string) error {
		_, err := elementFn(noCtx, []interpreter.Value{sval(name)})
		return err
	}
	base := call(t, elementFn, sval("r"))
	setAttr := func(name string) error {
		_, err := setAttrFn(noCtx, []interpreter.Value{base, sval(name), sval("v")})
		return err
	}
	for _, bad := range []string{"!--", "?x", "![CDATA[", "a:b:c", ":a", "a:", "1a", "-a", ".a", "a b", "a;b", ""} {
		if el(bad) == nil {
			t.Errorf("xml.element(%q) accepted", bad)
		}
		if setAttr(bad) == nil {
			t.Errorf("xml.setAttr(%q) accepted", bad)
		}
	}
	for _, good := range []string{"a", "_x", "dc:title", "a.b-c1", "été", "xmlns:atom", "xml:lang"} {
		if err := el(good); err != nil {
			t.Errorf("xml.element(%q): %v", good, err)
		}
		if err := setAttr(good); err != nil {
			t.Errorf("xml.setAttr(%q): %v", good, err)
		}
	}
}

// Appending one value repeatedly doubles the expanded tree each round; past
// the node cap the append is refused, so encode and == stay bounded.
func TestAppendBoundsExpandedSize(t *testing.T) {
	saved := maxDecodedNodes
	maxDecodedNodes = 1000
	defer func() { maxDecodedNodes = saved }()
	v := call(t, elementFn, sval("e"))
	var err error
	for i := 0; i < 40 && err == nil; i++ {
		p := call(t, elementFn, sval("p"))
		p = call(t, appendFn, p, v)
		v, err = appendFn(noCtx, []interpreter.Value{p, v})
	}
	if err == nil || !strings.Contains(err.Error(), "expands to more than 1000 nodes") {
		t.Fatalf("doubling appends: err = %v", err)
	}
}

// A chain of appends reuses the child backing (amortised O(1) per append,
// not a copy of every child), and a second append to an older handle does
// not disturb the handle that claimed its spare capacity.
func TestAppendChainReusesBacking(t *testing.T) {
	kid := call(t, elementFn, sval("k"))
	v := call(t, elementFn, sval("p"))
	backings := map[*node]bool{}
	var mid interpreter.Value
	for i := 0; i < 2000; i++ {
		v = call(t, appendFn, v, kid)
		n, _ := asNode(v)
		backings[&n.el.children[0]] = true
		if i == 999 {
			mid = v
		}
	}
	if len(backings) > 40 {
		t.Errorf("2000 appends used %d backing arrays, want amortised growth", len(backings))
	}
	branch := call(t, appendFn, mid, call(t, setTextFn, call(t, elementFn, sval("x")), sval("b")))
	bn, _ := asNode(branch)
	vn, _ := asNode(v)
	mn, _ := asNode(mid)
	if len(bn.el.children) != 1001 || bn.el.children[1000].el.name != "x" {
		t.Errorf("branch: %d children, last %q", len(bn.el.children), bn.el.children[1000].el.name)
	}
	if len(mn.el.children) != 1000 || vn.el.children[1000].el.name != "k" || len(vn.el.children) != 2000 {
		t.Error("appending to an older handle changed another handle")
	}
	if vn.el.size != 2001 || bn.el.size != 1003 {
		t.Errorf("sizes %d / %d, want 2001 / 1003", vn.el.size, bn.el.size)
	}
}

// A raw character outside XML 1.0's Char production makes a document
// malformed wherever it stands; DEL and the characters around the excluded
// ranges are allowed.
func TestDecodeRefusesNonXMLChars(t *testing.T) {
	for _, src := range []string{
		"<a>x\x02y</a>",
		"<a k=\"\x01\"/>",
		"<a><!-- \x1f --></a>",
		"<a><![CDATA[\x00]]></a>",
		"<?pi \x0b?><a/>",
		"<!DOCTYPE a [\x0c]><a/>",
		"<a>￾</a>",
		"<a>￿</a>",
		"<a/>\x08",
	} {
		_, err := decodeXML(src)
		if err == nil || !strings.Contains(err.Error(), "is not allowed in XML") || !strings.Contains(err.Error(), "line 1") {
			t.Errorf("%q: err = %v, want a positioned refusal", src, err)
		}
	}
	for _, src := range []string{"<a>\t\n\r\x7f�\U00010000</a>", "<a k=\"퟿\"/>"} {
		if _, err := decodeXML(src); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
}

// The builders refuse what no escape can write, so encode stays well-formed;
// before, setText("x\x01y") encoded the raw byte.
func TestBuildRefusesNonXMLChars(t *testing.T) {
	base := call(t, elementFn, sval("r"))
	for _, bad := range []string{"\x00", "a\x01b", "\x1f", "￾", "￿"} {
		if _, err := setTextFn(noCtx, []interpreter.Value{base, sval(bad)}); err == nil || !strings.Contains(err.Error(), "XML does not allow") {
			t.Errorf("setText(%q): err = %v", bad, err)
		}
		if _, err := setAttrFn(noCtx, []interpreter.Value{base, sval("k"), sval(bad)}); err == nil {
			t.Errorf("setAttr(%q) accepted", bad)
		}
		if _, err := setAttrNSFn(noCtx, []interpreter.Value{base, sval("urn:x"), sval("p:k"), sval(bad)}); err == nil {
			t.Errorf("setAttrNS(%q) accepted", bad)
		}
		if _, err := elementNSFn(noCtx, []interpreter.Value{sval("urn:" + bad), sval("p:a")}); err == nil {
			t.Errorf("elementNS(uri %q) accepted", bad)
		}
	}
	// What the builders accept encodes to a document that decodes back equal.
	text := "tab\tlf\ncr\rdel\x7f�\U0001F600"
	el := call(t, setAttrFn, call(t, setTextFn, base, sval(text)), sval("k"), sval(text))
	out := call(t, encodeOnly, el)
	back := mustDecode(t, out.Str)
	str := strGetter(t)
	if got := str(textFn(noCtx, []interpreter.Value{back})); got != text {
		t.Errorf("text round trip: %q", got)
	}
	if got := str(attrFn(noCtx, []interpreter.Value{back, sval("k")})); got != text {
		t.Errorf("attr round trip: %q", got)
	}
}

func encodeOnly(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return encodeFn(args, false)
}
