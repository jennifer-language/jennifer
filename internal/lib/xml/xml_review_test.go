// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package xmllib

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func encodeString(t *testing.T, n node) string {
	t.Helper()
	var sb strings.Builder
	if err := encodeNode(&sb, n, false, 0, &outScope{}); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return sb.String()
}

// Documents that are not well-formed XML 1.0 are decode errors.
func TestDecodeRejectsMalformed(t *testing.T) {
	cases := map[string]string{
		"<1a/>":                      "not a valid XML name",
		"<-a/>":                      "not a valid XML name",
		"<a!b/>":                     "not a valid XML name",
		"<a b!c='1'/>":               "not a valid XML name",
		"<a>&#+65;</a>":              "invalid character reference",
		"<a>&#x+41;</a>":             "invalid character reference",
		"<a>&#X41;</a>":              "invalid character reference",
		"<a>&#x;</a>":                "invalid character reference",
		"<a><!-- x -- y --></a>":     "'--' is not allowed",
		"<a><!-- x ---></a>":         "'--' is not allowed",
		"<a>x]]>y</a>":               "']]>' is not allowed",
		"<a/><?xml version='1.0'?>":  "only allowed at the very start",
		" <?xml version='1.0'?><a/>": "only allowed at the very start",
		"<a><?XML x?></a>":           "only allowed at the very start",
		"<a><?1x?></a>":              "not a valid name",
		"<a>\xff</a>":                "invalid UTF-8",
	}
	for src, want := range cases {
		if _, err := decodeXML(src); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("decode(%q): err = %v, want %q", src, err, want)
		}
	}
	for _, src := range []string{
		"<?xml version='1.0'?><a/>", "\xef\xbb\xbf<?xml version='1.0'?><a/>", "\xef\xbb\xbf<a/>",
		"<a><!-- x - y --></a>", "<a>x]]&gt;y</a>", "<a>&#65;&#x41;&#0065;</a>", "<a.b _c='1' d-e='2'/>",
		"<a><?pi data?></a>", "<a><![CDATA[x]]]]><![CDATA[>y]]></a>",
	} {
		if _, err := decodeXML(src); err != nil {
			t.Errorf("decode(%q): %v", src, err)
		}
	}
}

// Line ends are normalized in text and CDATA (XML 1.0, 2.11); a CR the
// document wrote as &#13; survives, and encode writes a CR back that way.
func TestLineEndNormalization(t *testing.T) {
	n, err := decodeXML("<a>x\r\ny\rz&#13;<![CDATA[p\r\nq]]></a>")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range n.el.children {
		got = append(got, c.text)
	}
	if strings.Join(got, "|") != "x\ny\nz\r|p\nq" {
		t.Errorf("text = %q", got)
	}
	out := encodeString(t, n)
	back, err := decodeXML(out)
	if err != nil {
		t.Fatal(err)
	}
	joined := func(n node) string {
		var sb strings.Builder
		for _, c := range n.el.children {
			sb.WriteString(c.text)
		}
		return sb.String()
	}
	if joined(back) != joined(n) {
		t.Errorf("round trip of %q: %q, want %q", out, joined(back), joined(n))
	}
}

// setAttr on a namespace-aware element resolves a prefixed name through the
// prefixes in scope, refuses an undeclared one, and records a declaration in
// scope - so encode never writes a prefix nothing declares.
func TestSetAttrResolvesPrefixes(t *testing.T) {
	n, err := decodeXML("<a xmlns:p='urn:p'/>")
	if err != nil {
		t.Fatal(err)
	}
	set := func(v interpreter.Value, name, value string) (interpreter.Value, error) {
		return setAttrFn(interpreter.BuiltinCtx{}, []interpreter.Value{v, sv(name), sv(value)})
	}
	if _, err := set(wrap(n), "q:x", "1"); err == nil || !strings.Contains(err.Error(), "setAttrNS") {
		t.Errorf("undeclared prefix: err = %v", err)
	}
	v, err := set(wrap(n), "p:x", "1")
	if err != nil {
		t.Fatal(err)
	}
	out, _ := asNode(v)
	if a, _ := out.el.find("p:x"); a.ns != "urn:p" {
		t.Errorf("p:x namespace = %q, want urn:p", a.ns)
	}
	// Declare a prefix, then use it.
	v, err = set(wrap(n), "xmlns:q", "urn:q")
	if err != nil {
		t.Fatal(err)
	}
	if v, err = set(v, "q:y", "2"); err != nil {
		t.Fatalf("prefix declared by setAttr: %v", err)
	}
	out, _ = asNode(v)
	text := encodeString(t, out)
	if _, err := decodeXML(text); err != nil {
		t.Errorf("encoded %s does not decode: %v", text, err)
	}
	for _, bad := range [][2]string{{"xmlns:xmlns", "u"}, {"xmlns:xml", "urn:x"}, {"xmlns:p", ""}, {"xmlns:p", xmlNamespace}} {
		if _, err := set(wrap(n), bad[0], bad[1]); err == nil {
			t.Errorf("setAttr(%s=%q): want an error", bad[0], bad[1])
		}
	}
	// A namespace-unaware element (xml.element) keeps verbatim names.
	plain := elementNode("a", nil, nil)
	if _, err := set(wrap(plain), "q:x", "1"); err != nil {
		t.Errorf("unaware element: %v", err)
	}
}
