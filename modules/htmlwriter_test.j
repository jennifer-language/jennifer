# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# htmlwriter_test.j - white-box tests for htmlwriter.j. Run with:
#
#     jennifer test modules/htmlwriter_test.j
#
# The overlay splices htmlwriter.j in front of this file, so the tests reach
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
    testing.assertThrows("injectBoolAttrName", "htmlwriter");
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
    testing.assertThrows("injectAttrName", "htmlwriter");
    testing.assertThrows("injectTagName", "htmlwriter");
    testing.assertThrows("injectTagSpace", "htmlwriter");
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
