# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# html_test.j - white-box tests for html.j. Run with:
#
#     jennifer test modules/html_test.j
#
# The overlay splices html.j in front of this file, so the tests reach
# its private helpers (escapeAttr, isVoid, renderAttrs) and the private VOID
# table by bare identifier, alongside its exported surface.
use testing;

func testEscapeText() {
    testing.assertEqual(escape("a < b & c > d"), "a &lt; b &amp; c &gt; d");
    testing.assertEqual(escape("&amp;"), "&amp;amp;"); # & escaped first, no re-escape
    testing.assertEqual(escape("plain"), "plain");
}

func testTextNodeEscapes() {
    testing.assertEqual(render(text("x & <y>")), "x &amp; &lt;y&gt;");
}

func testRawIsVerbatim() {
    testing.assertEqual(render(raw("<b>bold</b> & co")), "<b>bold</b> & co");
}

func testElementBasic() {
    def kids as list of Node init [];
    $kids[] = text("hi");
    testing.assertEqual(render(element("p", [], $kids)), "<p>hi</p>");
    testing.assertEqual(render(element("br", [], [])), "<br>"); # void
}

func testAttributes() {
    def attrs as list of Attr init [];
    $attrs[] = attr("class", "box");
    $attrs[] = attr("title", "a \"b\" <c>");
    def out as string init render(element("div", $attrs, []));
    testing.assertEqual($out, "<div class=\"box\" title=\"a &quot;b&quot; &lt;c&gt;\"></div>");
}

func testBoolAttrBareName() {
    def attrs as list of Attr init [];
    $attrs[] = boolAttr("disabled");
    testing.assertEqual(render(element("input", $attrs, [])), "<input disabled>");
    # The Attr carries the boolean flag and no value.
    testing.assertTrue(boolAttr("checked").boolean);
    testing.assertEqual(boolAttr("checked").value, "");
    # A normal attribute is unaffected and still renders name="value".
    testing.assertFalse(attr("id", "x").boolean);
    testing.assertEqual(renderAttrs([attr("id", "x")]), " id=\"x\"");
}

func testBoolAttrMixed() {
    # A boolean and a normal attribute on one element render side by side.
    def attrs as list of Attr init [];
    $attrs[] = attr("type", "checkbox");
    $attrs[] = boolAttr("checked");
    $attrs[] = boolAttr("required");
    $attrs[] = attr("name", "agree");
    testing.assertEqual(
        render(element("input", $attrs, [])),
        "<input type=\"checkbox\" checked required name=\"agree\">");
}

func injectBoolAttrName() {
    boolAttr("x onclick=alert(1)");
}
func testBoolAttrNameValidated() {
    # An invalid boolean-attribute name still throws, like attr.
    testing.assertThrows("injectBoolAttrName", "html");
    testing.assertEqual(boolAttr("data-x2").name, "data-x2");
}

func testNestedTree() {
    def a as list of Node init [];
    $a[] = text("one");
    def b as list of Node init [];
    $b[] = text("two");
    def items as list of Node init [];
    $items[] = element("li", [], $a);
    $items[] = element("li", [], $b);
    testing.assertEqual(render(element("ul", [], $items)), "<ul><li>one</li><li>two</li></ul>");
}

func testVoidDropsChildren() {
    # A void element renders no closing tag and ignores any children given.
    def kids as list of Node init [];
    $kids[] = text("ignored");
    def attrs as list of Attr init [];
    $attrs[] = attr("src", "a.png");
    testing.assertEqual(render(element("img", $attrs, $kids)), "<img src=\"a.png\">");
}

func testRenderAllFragment() {
    def frag as list of Node init [];
    $frag[] = raw("<!-- c -->");
    $frag[] = text("x>y");
    $frag[] = element("b", [], []);
    testing.assertEqual(renderAll($frag), "<!-- c -->x&gt;y<b></b>");
    testing.assertEqual(renderAll([]), "");
}

# White-box: private helpers reached by bare identifier.
func testPrivateEscapeAttr() {
    testing.assertEqual(escapeAttr("a\"b<c>&d"), "a&quot;b&lt;c&gt;&amp;d");
}

func testPrivateIsVoid() {
    testing.assertTrue(isVoid("br"));
    testing.assertTrue(isVoid("IMG")); # case-insensitive
    testing.assertTrue(isVoid("hr"));
    testing.assertFalse(isVoid("div"));
    testing.assertFalse(isVoid("span"));
}

func testPrivateRenderAttrs() {
    def attrs as list of Attr init [];
    $attrs[] = attr("id", "main");
    $attrs[] = attr("data", "x&y");
    testing.assertEqual(renderAttrs($attrs), " id=\"main\" data=\"x&amp;y\"");
    testing.assertEqual(renderAttrs([]), "");
}

# --- OM-011: tag / attribute name validation + safeUrl -----------------------

func injectAttrName() {
    attr("x onclick=alert(1)", "y");
}
func injectTagName() {
    element("a href=x", [], []);
}
func injectTagSpace() {
    element("bad tag", [], []);
}
func testNameInjectionBlocked() {
    testing.assertThrows("injectAttrName", "html");
    testing.assertThrows("injectTagName", "html");
    testing.assertThrows("injectTagSpace", "html");
    # Legal names still build: letters, a digit, a hyphen.
    testing.assertEqual(attr("data-x2", "v").name, "data-x2");
    testing.assertEqual(element("h1", [], []).tag, "h1");
}

func testSafeUrl() {
    testing.assertEqual(safeUrl("https://example.com/a"), "https://example.com/a");
    testing.assertEqual(safeUrl("http://x"), "http://x");
    testing.assertEqual(safeUrl("mailto:a@b.c"), "mailto:a@b.c");
    testing.assertEqual(safeUrl("/relative/path"), "/relative/path");
    testing.assertEqual(safeUrl("page.html"), "page.html");
    # Dangerous schemes fold to "#", including ones hidden behind whitespace.
    testing.assertEqual(safeUrl("javascript:alert(1)"), "#");
    testing.assertEqual(safeUrl("java\tscript:alert(1)"), "#");
    testing.assertEqual(safeUrl("  javascript:alert(1)"), "#");
    testing.assertEqual(safeUrl("data:text/html,<script>"), "#");
    # A non-ASCII (multibyte) URL must not error - rune-indexed, returned unchanged
    # for an allowed/relative reference.
    testing.assertEqual(safeUrl("http://exämple.com/x"), "http://exämple.com/x");
    testing.assertEqual(safeUrl("/café/menü"), "/café/menü");
}

# --- parser (reader) ---

func testParseBasic() {
    def doc as Node init parse("<ul class=fruit><li>apples</li><li>figs</li></ul>");
    def ul as Node init get($doc, "ul");
    testing.assertEqual($ul.tag, "ul");
    testing.assertEqual(attrOf($ul, "class"), "fruit");
    def items as list of Node init findAll($doc, "ul/li");
    testing.assertEqual(len($items), 2);
    testing.assertEqual($items[0].children[0].text, "apples");
    testing.assertEqual($items[1].children[0].text, "figs");
}

func testParseSelectors() {
    def doc as Node init parse("<div><p>a</p><p>b</p><p>c</p></div>");
    testing.assertTrue(has($doc, "div/p"));
    testing.assertEqual(len(findAll($doc, "div/p")), 3);
    testing.assertEqual(len(findAll($doc, "div/*")), 3);
    testing.assertEqual(get($doc, "div/p[2]").children[0].text, "b");
    testing.assertFalse(has($doc, "div/span"));
    testing.assertEqual(get($doc, "div/span").tag, "");   # no match -> empty node
}

func testParseAttrs() {
    def doc as Node init parse('<a href="/x" data-id=42 disabled>hi</a>');
    def a as Node init get($doc, "a");
    testing.assertEqual(attrOf($a, "href"), "/x");
    testing.assertEqual(attrOf($a, "data-id"), "42");   # unquoted value
    testing.assertTrue(hasAttr($a, "disabled"));        # boolean attribute
    testing.assertFalse(hasAttr($a, "nope"));
    testing.assertEqual(attrOf($a, "nope"), "");
}

func testParseRoundTrip() {
    def src as string init "<p>Hello <a href=\"/x\">link</a> &amp; more</p>";
    testing.assertEqual(render(get(parse($src), "p")), $src);
}

func testParseTolerant() {
    # void elements, unquoted attr, comment skipped, mismatched </i> ignored + <b> auto-closed
    def doc as Node init parse("<div><br><img src=a.png><!-- c --><b>x</i></div>");
    testing.assertEqual(render(get($doc, "div")), "<div><br><img src=\"a.png\"><b>x</b></div>");
}

func testParseScriptRaw() {
    def doc as Node init parse("<script>if (a < b) x();</script>");
    testing.assertEqual(get($doc, "script").children[0].text, "if (a < b) x();");
}

func testParseDoctype() {
    def doc as Node init parse("<!DOCTYPE html><html><body>hi</body></html>");
    testing.assertTrue(has($doc, "html/body"));
    testing.assertEqual(get($doc, "html/body").children[0].text, "hi");
}
