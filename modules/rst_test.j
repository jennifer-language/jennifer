# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# rst_test.j - white-box tests for rst.j. Run with:
#
#     jennifer test modules/rst_test.j
#
# The overlay splices rst.j (and its included rst_inline.inc.j) in front of this
# file, so the tests reach private helpers (normalize, titleAt, scanEnum,
# inlineNodes, ...) by bare identifier, next to the exported surface. Most
# cases compare the rendered HTML, which pins both the tree and its lowering.
use testing;

func h(src as string) {
    return toHtml($src);
}

func diagsOf(src as string) {
    def out as list of string init [];
    for (def d in check($src)) {
        $out[] = "{$d.severity} {$d.line}: {$d.message}";
    }
    return $out;
}

# --- normalisation ---

func testNormalizeLineEndingsTabsAndTrailingSpace() {
    def lines as list of string init normalize("﻿a\r\nb\rc  \n\tx\n  \t");
    testing.assertEqual($lines, ["a", "b", "c", "        x", ""]);
}

func testNormalizeTabStops() {
    testing.assertEqual(expandTabs("ab\tc"), "ab      c");
}

# --- sections ---

func testSectionLevelsFollowStyleOrder() {
    def src as string init "One\n===\n\na\n\nTwo\n---\n\nb\n\nThree\n=====\n\nc\n";
    testing.assertEqual(
        h($src),
        "<h1 id=\"one\">One</h1><p>a</p><h2 id=\"two\">Two</h2><p>b</p>" +
            "<h1 id=\"three\">Three</h1><p>c</p>");
}

func testOverlineIsADistinctStyle() {
    def src as string init "===\nA\n===\n\nx\n\nB\n===\n\ny\n\nC\n===\n\nz\n";
    testing.assertEqual(
        h($src),
        "<h1 id=\"a\" class=\"title\">A</h1><p>x</p><h2 id=\"b\">B</h2><p>y</p>" +
            "<h2 id=\"c\">C</h2>" +
            "<p>z</p>");
}

func testLoneTitleAndSubtitleArePromoted() {
    def src as string init "=====\nTitle\n=====\n\nSub\n---\n\nSection\n=======\n\ntext\n";
    testing.assertEqual(
        h($src),
        "<h1 id=\"title\" class=\"title\">Title</h1><p id=\"sub\" class=\"subtitle\">Sub</p>" +
            "<h2 id=\"section\">Section</h2><p>text</p>");
    testing.assertEqual(title(parse($src)), "Title");
}

func testTitleNotPromotedWhenNotAlone() {
    def doc as doctree.Node init parse("A\n=\n\nx\n\nB\n=\n\ny\n");
    testing.assertEqual(title($doc), "");
    testing.assertEqual($doc.children[0].level, 1);
    testing.assertEqual($doc.children[2].level, 1);
}

func testNoTitlePromotionOption() {
    def opts as Options init defaultOptions();
    $opts.noTitlePromotion = true;
    testing.assertEqual(
        toHtmlWith("Title\n=====\n\nSub\n---\n", $opts),
        "<h1 id=\"title\">Title</h1><h2 id=\"sub\">Sub</h2>");
}

func testInitialHeadingLevel() {
    def opts as Options init defaultOptions();
    $opts.initialHeadingLevel = 3;
    $opts.noTitlePromotion = true;
    testing.assertEqual(
        toHtmlWith("A\n=\n\nB\n-\n", $opts),
        "<h3 id=\"a\">A</h3><h4 id=\"b\">B</h4>");
}

func testHeadingLevelsCapAtSix() {
    # Eight levels: after the title and subtitle are promoted, the last one
    # still lands at level 7 and is capped.
    def src as string init "a\n=\n\nb\n-\n\nc\n~\n\nd\n^\n\ne\n+\n\nf\n*\n\ng\n#\n\nh\n:\n";
    def opts as Options init defaultOptions();
    $opts.noTitlePromotion = true;
    def doc as doctree.Node init parseWith($src, $opts);
    testing.assertEqual($doc.children[6].level, 6);
    testing.assertContains(strings.join(diagsOf($src), "\n"), "deeper than 6");
}

func testDuplicateSectionIdsAreNumbered() {
    def doc as doctree.Node init parse("Intro\n=====\n\nIntro\n=====\n");
    testing.assertEqual($doc.children[0].attrs["id"], "intro");
    testing.assertEqual($doc.children[1].attrs["id"], "intro-2");
}

func testSlugOf() {
    testing.assertEqual(slugOf("Hello, World!"), "hello-world");
    testing.assertEqual(slugOf("1. Getting started"), "getting-started");
    testing.assertEqual(slugOf("???"), "section");
}

func testShortUnderline() {
    # Under 4 characters and shorter than the title: ordinary text.
    testing.assertEqual(h("Long title\n--\n"), "<p>Long title --</p>");
    # 4 or more: still a title, with a warning.
    def src as string init "Long title\n----\n";
    testing.assertEqual(h($src), "<h1 id=\"long-title\" class=\"title\">Long title</h1>");
    testing.assertEqual(diagsOf($src), ["warning 1: Title underline too short."]);
}

func testInconsistentTitleLevel() {
    def src as string init "A\n=\n\nB\n-\n\nC\n~\n\nD\n=\n\nE\n~\n";
    testing.assertEqual(diagsOf($src), ["error 13: Title level inconsistent."]);
}

func testSectionInsideQuoteIsReported() {
    def src as string init "para\n\n  Title\n  =====\n";
    testing.assertEqual(h($src), "<p>para</p><blockquote><p>Title</p></blockquote>");
    testing.assertEqual(diagsOf($src), ["error 3: Unexpected section title or transition."]);
}

func testTransition() {
    testing.assertEqual(h("a\n\n----\n\nb\n"), "<p>a</p><hr><p>b</p>");
}

# --- paragraphs and literal blocks ---

func testParagraphLinesJoin() {
    testing.assertEqual(h("one\ntwo\n\nthree\n"), "<p>one two</p><p>three</p>");
}

func testHtmlInTextIsEscaped() {
    testing.assertEqual(
        h("<script>x</script> & y\n"),
        "<p>&lt;script&gt;x&lt;/script&gt; &amp; y</p>");
}

func testLiteralBlockMarkerForms() {
    testing.assertEqual(
        h("Code::\n\n    x = 1\n      y\n"),
        "<p>Code:</p><pre class=\"literal-block\"><code>x = 1\n  y</code></pre>");
    testing.assertEqual(
        h("Code ::\n\n    x\n"),
        "<p>Code</p><pre class=\"literal-block\"><code>x</code></pre>");
    testing.assertEqual(h("::\n\n    x\n"), "<pre class=\"literal-block\"><code>x</code></pre>");
}

func testLiteralBlockKeepsMarkupAndBlankLines() {
    testing.assertEqual(
        h("a::\n\n    *not em*\n\n    <b>\n\nafter\n"),
        "<p>a:</p><pre class=\"literal-block\"><code>*not em*\n\n&lt;b&gt;</code></pre>" +
            "<p>after</p>");
}

func testQuotedLiteralBlock() {
    testing.assertEqual(
        h("Mail::\n\n> quoted\n> lines\n\nend\n"),
        "<p>Mail:</p><pre class=\"literal-block\"><code>&gt; quoted\n&gt; lines</code></pre>" +
            "<p>end</p>");
}

func testLiteralBlockMissing() {
    def src as string init "a::\n\nb\n";
    testing.assertEqual(h($src), "<p>a:</p><p>b</p>");
    testing.assertEqual(diagsOf($src), ["warning 3: Literal block expected; none found."]);
}

func testUnexpectedIndentation() {
    def src as string init "one\ntwo\n  three\n";
    testing.assertEqual(h($src), "<p>one two</p><blockquote><p>three</p></blockquote>");
    testing.assertContains(strings.join(diagsOf($src), "\n"), "error 3: Unexpected indentation.");
}

func testDoctestBlock() {
    testing.assertEqual(
        h(">>> 1 + 1\n2\n\nafter\n"),
        "<pre class=\"doctest-block\"><code class=\"language-pycon\">&gt;&gt;&gt; 1 + 1\n2" +
            "</code></pre>" +
            "<p>after</p>");
}

# --- block quotes ---

func testBlockQuoteNests() {
    testing.assertEqual(
        h("a\n\n  q1\n\n    q2\n"),
        "<p>a</p><blockquote><p>q1</p><blockquote><p>q2</p></blockquote></blockquote>");
}

func testBlockQuoteAttribution() {
    testing.assertEqual(
        h("  Wise words.\n\n  -- Somebody *famous*\n"),
        "<blockquote><p>Wise words.</p><p class=\"attribution\">—Somebody <em>famous</em></p>" +
            "</blockquote>");
}

func testAttributionSplitsQuotes() {
    def doc as doctree.Node init parse("  one\n\n  -- A\n\n  two\n\n  --- B\n");
    testing.assertEqual(len($doc.children), 2);
    testing.assertEqual(doctree.text($doc.children[1]), "two—B");
}

func testFourDashesIsNotAttribution() {
    testing.assertEqual(h("  a\n\n  ---- b\n"), "<blockquote><p>a</p><p>---- b</p></blockquote>");
}

# --- bullet lists ---

func testBulletListSimple() {
    testing.assertEqual(h("- a\n- b\n"), "<ul><li>a</li><li>b</li></ul>");
    testing.assertEqual(h("* a\n\n* b\n"), "<ul><li>a</li><li>b</li></ul>");
}

func testBulletListNestedAndContinued() {
    testing.assertEqual(
        h("- a\n  more\n\n  - b\n- c\n"),
        "<ul><li>a more<ul><li>b</li></ul></li><li>c</li></ul>");
}

func testBulletListItemWithBlocks() {
    testing.assertEqual(
        h("- para one\n\n  para two\n- x\n"),
        "<ul><li><p>para one</p><p>para two</p></li><li><p>x</p></li></ul>");
}

func testDifferentBulletStartsNewList() {
    def doc as doctree.Node init parse("- a\n\n+ b\n");
    testing.assertEqual(len($doc.children), 2);
}

func testBulletListEndsWithoutBlank() {
    def src as string init "- a\nb\n";
    testing.assertEqual(h($src), "<ul><li>a</li></ul><p>b</p>");
    testing.assertEqual(
        diagsOf($src),
        ["warning 2: Bullet list ends without a blank line; unexpected unindent."]);
}

func testBulletWithEmptyFirstLine() {
    testing.assertEqual(h("-\n  text\n"), "<ul><li>text</li></ul>");
}

func testDashWordIsNotBullet() {
    testing.assertEqual(h("-not a list\n"), "<p>-not a list</p>");
}

# --- enumerated lists ---

func testEnumeratedArabic() {
    testing.assertEqual(h("1. a\n2. b\n"), "<ol><li>a</li><li>b</li></ol>");
}

func testEnumeratedFormatsAndSequences() {
    testing.assertEqual(h("(a) x\n(b) y\n"), "<ol type=\"a\"><li>x</li><li>y</li></ol>");
    testing.assertEqual(h("A) x\nB) y\n"), "<ol type=\"A\"><li>x</li><li>y</li></ol>");
    testing.assertEqual(
        h("i. x\nii. y\niii. z\n"),
        "<ol type=\"i\"><li>x</li><li>y</li><li>z</li></ol>");
    testing.assertEqual(h("IV. x\nV. y\n"), "<ol start=\"4\" type=\"I\"><li>x</li><li>y</li></ol>");
}

func testEnumeratedStartValue() {
    testing.assertEqual(h("3. c\n4. d\n"), "<ol start=\"3\"><li>c</li><li>d</li></ol>");
}

func testAutoEnumerated() {
    testing.assertEqual(h("#. a\n#. b\n"), "<ol><li>a</li><li>b</li></ol>");
    testing.assertEqual(h("a. x\n#. y\n"), "<ol type=\"a\"><li>x</li><li>y</li></ol>");
}

func testEnumeratedSequenceBreak() {
    # "3." does not follow "1.", so the first line is not a list item either:
    # docutils reads both lines as one paragraph.
    testing.assertEqual(h("1. a\n3. b\n"), "<p>1. a 3. b</p>");
    # With a blank line between, they are two one-item lists.
    def doc as doctree.Node init parse("1. a\n\n3. b\n");
    testing.assertEqual(len($doc.children), 2);
    testing.assertEqual($doc.children[1].attrs["start"], "3");
}

func testEnumeratorNeedsValidSecondLine() {
    # A sentence starting with an initial is a paragraph, not a list.
    testing.assertEqual(
        h("A. Einstein was a really\nsmart dude.\n"),
        "<p>A. Einstein was a really smart dude.</p>");
}

func testEnumSequenceRules() {
    testing.assertEqual(enumSequence("i", ""), "lowerroman");
    testing.assertEqual(enumSequence("v", ""), "loweralpha");
    testing.assertEqual(enumSequence("v", "lowerroman"), "lowerroman");
    testing.assertEqual(enumSequence("ii", ""), "lowerroman");
    testing.assertEqual(enumSequence("12", ""), "arabic");
    testing.assertEqual(enumOrdinal("lowerroman", "xiv"), 14);
    testing.assertEqual(enumOrdinal("upperroman", "IIII"), -1);
    testing.assertEqual(enumText(4, "upperroman", "parens"), "(IV)");
}

# --- definition, field and option lists ---

func testDefinitionList() {
    testing.assertEqual(
        h("term\n    def one\n\nother\n    def two\n"),
        "<dl><dt>term</dt><dd>def one</dd><dt>other</dt><dd>def two</dd></dl>");
}

func testDefinitionListClassifiers() {
    testing.assertEqual(
        h("name : str : opt\n    the name\n"),
        "<dl><dt>name<span class=\"classifier-delimiter\">:</span> " +
            "<span class=\"classifier\">str</span><span class=\"classifier-delimiter\">:</span> " +
            "<span class=\"classifier\">opt</span></dt>" +
            "<dd>the name</dd></dl>");
}

func testDefinitionWithBlocksIsNotCompacted() {
    testing.assertEqual(
        h("t\n    p1\n\n    p2\n"),
        "<dl><dt>t</dt><dd><p>p1</p><p>p2</p></dd></dl>");
}

func testFieldList() {
    testing.assertEqual(
        h("text\n\n:one: first\n:two words: second\n   continued\n"),
        "<p>text</p><dl class=\"field-list\"><dt>one</dt><dd>first</dd><dt>two words</dt>" +
            "<dd>second continued</dd></dl>");
}

func testScanField() {
    testing.assertTrue(scanField(":a: b").ok);
    testing.assertEqual(scanField(":a\\: b: c").name, "a\\: b");
    testing.assertFalse(scanField(": a: b").ok);
    testing.assertFalse(scanField("::x").ok);
    testing.assertEqual(scanField(":name:   body").end, 9);
}

func testDocinfo() {
    def src as string init "Title\n=====\n\n:Author: Ada Lovelace\n:Version: 1.2\n" +
        ":Dedication: For Charles.\n\nBody.\n";
    def doc as doctree.Node init parse($src);
    def info as map of string to string init docinfo($doc);
    testing.assertEqual($info["author"], "Ada Lovelace");
    testing.assertEqual($info["version"], "1.2");
    testing.assertFalse(maps.has($info, "dedication"));
    testing.assertEqual(
        h($src),
        "<h1 id=\"title\" class=\"title\">Title</h1><dl class=\"docinfo\"><dt>Author</dt>" +
            "<dd>Ada Lovelace</dd><dt>Version</dt><dd>1.2</dd></dl>" +
            "<aside class=\"topic dedication\">" +
            "<p class=\"topic-title\">Dedication</p><p>For Charles.</p></aside><p>Body.</p>");
}

func testFieldListLaterIsNotDocinfo() {
    def doc as doctree.Node init parse("para\n\n:a: b\n");
    testing.assertEqual(len(docinfo($doc)), 0);
}

func testOptionList() {
    testing.assertEqual(
        h("-a            all\n--file=FILE   the file\n-v, --verbose\n    say more\n"),
        "<dl class=\"option-list\"><dt><code class=\"option\">-a</code></dt><dd>all</dd>" +
            "<dt><code class=\"option\">--file=FILE</code></dt><dd>the file</dd>" +
            "<dt><code class=\"option\">-v</code>, <code class=\"option\">--verbose</code></dt>" +
            "<dd>say more</dd></dl>");
}

func testOptionNeedsDescription() {
    testing.assertEqual(h("-1 is a number\n"), "<p>-1 is a number</p>");
    testing.assertEqual(h("--flag\n"), "<p>--flag</p>");
}

# --- line blocks ---

func testLineBlock() {
    testing.assertEqual(
        h("| one\n|   nested\n|\n| *two*\n  continued\n"),
        "<div class=\"line-block\"><div class=\"line\">one</div><div class=\"line-block\">" +
            "<div class=\"line\">nested</div><div class=\"line\"><br></div></div>" +
            "<div class=\"line\"><em>two</em> continued</div></div>");
}

# --- explicit markup ---

func testCommentsAreDropped() {
    testing.assertEqual(h(".. a comment\n   spanning lines\n\npara\n"), "<p>para</p>");
    testing.assertEqual(h("..\n\n  quoted\n"), "<blockquote><p>quoted</p></blockquote>");
}

func testUnknownDirectiveIsKeptAndReported() {
    def src as string init ".. frobnicate:: arg\n   body\n";
    testing.assertEqual(
        h($src),
        "<pre class=\"literal-block\"><code>.. frobnicate:: arg\n   body</code></pre>");
    testing.assertEqual(diagsOf($src), ["error 1: Unknown directive type \"frobnicate\"."]);
}

# --- inline markup ---

func testInlineBasics() {
    testing.assertEqual(
        h("*em* **strong** ``lit *x*`` `title`\n"),
        "<p><em>em</em> <strong>strong</strong> <code>lit *x*</code> <cite>title</cite></p>");
}

func testInlineStartRules() {
    # Not after a word character, not before whitespace, not quoted.
    testing.assertEqual(h("2*3*4 and a * b\n"), "<p>2*3*4 and a * b</p>");
    testing.assertEqual(h("\"*\" and (*)\n"), "<p>\"*\" and (*)</p>");
    testing.assertEqual(h("(*em*), \"*q*\"\n"), "<p>(<em>em</em>), \"<em>q</em>\"</p>");
}

func testInlineEndRules() {
    # The end-string must not follow whitespace and must be followed by
    # whitespace or punctuation.
    testing.assertEqual(h("*a *b*\n"), "<p><em>a *b</em></p>");
    testing.assertEqual(h("*a*b c*\n"), "<p><em>a*b c</em></p>");
}

func testInlineEscapes() {
    testing.assertEqual(h("\\*not em\\* and a\\ b\n"), "<p>*not em* and ab</p>");
    testing.assertEqual(h("*a\\*b*\n"), "<p><em>a*b</em></p>");
}

func testInlineUnterminated() {
    def src as string init "an **open strong\n";
    testing.assertEqual(h($src), "<p>an **open strong</p>");
    testing.assertEqual(
        diagsOf($src),
        ["warning 1: Inline strong start-string without end-string."]);
}

func testInlineAcrossLines() {
    testing.assertEqual(h("*em\nphasis*\n"), "<p><em>em phasis</em></p>");
}

func testInlineNodesWhiteBox() {
    def kids as list of doctree.Node init inlineNodes("a ``b`` c", 1);
    testing.assertEqual(len($kids), 3);
    testing.assertEqual($kids[1].kind, "codespan");
    testing.assertEqual($kids[1].text, "b");
}

# --- diagnostics, strict mode, limits ---

func testCleanDocumentHasNoDiagnostics() {
    testing.assertEqual(len(check("Title\n=====\n\n- a\n- b\n\nText *em*.\n")), 0);
}

func testDiagnosticsLeaveTheTree() {
    def doc as doctree.Node init parse("- a\nb\n");
    testing.assertEqual(len(doctree.findAll($doc, "rst_diag")), 0);
}

func strictFails() {
    def opts as Options init defaultOptions();
    $opts.strict = true;
    return parseWith("- a\nb\n", $opts);
}

func testStrictThrows() {
    testing.assertThrows("strictFails", "rst");
}

func deepQuotes() {
    def parts as list of string init [];
    for (def k in 0..150) {
        $parts[] = strings.repeat(" ", $k) + "x";
        $parts[] = "";
    }
    return parse(strings.join($parts, "\n"));
}

func testDeepNestingIsAnRstError() {
    testing.assertThrows("deepQuotes", "rst");
}

func deepLists() {
    def parts as list of string init [];
    for (def k in 0..150) {
        $parts[] = strings.repeat("  ", $k) + "- x";
        $parts[] = "";
    }
    return parse(strings.join($parts, "\n"));
}

func testDeepListsAreAnRstError() {
    testing.assertThrows("deepLists", "rst");
}

func testManyNodesStillParse() {
    def parts as list of string init [];
    for (def k in 0..2000) {
        $parts[] = "- item *{$k}*";
    }
    def doc as doctree.Node init parse(strings.join($parts, "\n"));
    testing.assertEqual(len($doc.children[0].children), 2000);
}

# --- other renderers ---

func testAnsiRendering() {
    def out as string init regex.replace(
        '\x1b\[[0-9;]*m',
        toAnsi("Title\n=====\n\n- a\n- b\n\n| x\n|  y\n"),
        "");
    testing.assertContains($out, "Title");
    testing.assertContains($out, "  - a");
    testing.assertContains($out, "x\n    y");
}

func testXhtmlRendering() {
    testing.assertEqual(toXhtml("a\n\n----\n\nb\n"), "<p>a</p><hr /><p>b</p>");
}

func testPdfRenderingOfTheTree() {
    def doc as doctree.Node init parse("Title\n=====\n\n- a\n- b\n\nterm\n    def\n\n| line\n");
    def out as bytes init doctree.renderPdf($doc, doctree.pdfDefaults());
    testing.assertTrue(len($out) > 500);
}

# --- hyperlink references and targets ---

func testNamedReferenceAndTarget() {
    testing.assertEqual(
        h("See Python_.\n\n.. _Python: https://www.python.org/\n"),
        "<p>See <a href=\"https://www.python.org/\">Python</a>.</p>");
}

func testPhraseReferenceIsCaseAndSpaceInsensitive() {
    testing.assertEqual(
        h("A `Big   Name`_ here.\n\n.. _big name: https://x.example/\n"),
        "<p>A <a href=\"https://x.example/\">Big   Name</a> here.</p>");
}

func testEmbeddedUriDefinesATarget() {
    testing.assertEqual(
        h("`Docs <https://d.example/>`_ and Docs_ again.\n"),
        "<p><a href=\"https://d.example/\">Docs</a> and " +
            "<a href=\"https://d.example/\">Docs</a> again.</p>");
    # With `__` it does not.
    testing.assertEqual(
        diagsOf("`Docs <https://d.example/>`__ and Docs_ again.\n"),
        ["error 1: Unknown target name: \"docs\"."]);
}

func testEmbeddedUriWithoutText() {
    testing.assertEqual(
        h("`<https://d.example/>`_\n"),
        "<p><a href=\"https://d.example/\">https://d.example/</a></p>");
}

func testEmbeddedAlias() {
    testing.assertEqual(
        h("`the site <home_>`_\n\n.. _home: https://h.example/\n"),
        "<p><a href=\"https://h.example/\">the site</a></p>");
}

func testIndirectTargetChain() {
    def src as string init "a_\n\n.. _a: b_\n.. _b: `c`_\n.. _c: https://c.example/\n";
    testing.assertEqual(h($src), "<p><a href=\"https://c.example/\">a</a></p>");
}

func testIndirectTargetLoopIsUnknown() {
    def src as string init "a_\n\n.. _a: b_\n.. _b: a_\n";
    testing.assertEqual(diagsOf($src), ["error 1: Unknown target name: \"a\"."]);
}

func testTargetUriMayWrap() {
    testing.assertEqual(
        h("x_\n\n.. _x: https://example.org/a/very/\n   long/path\n"),
        "<p><a href=\"https://example.org/a/very/long/path\">x</a></p>");
}

func testQuotedTargetNameWithColon() {
    testing.assertEqual(
        h("`a:b`_\n\n.. _`a:b`: https://ab.example/\n"),
        "<p><a href=\"https://ab.example/\">a:b</a></p>");
}

func testAnonymousReferencesPairInOrder() {
    def src as string init "one__ and `two`__\n\n__ https://1.example/\n\n" +
        ".. __: https://2.example/\n";
    testing.assertEqual(
        h($src),
        "<p><a href=\"https://1.example/\">one</a> and <a href=\"https://2.example/\">two</a></p>");
}

func testAnonymousMismatch() {
    testing.assertEqual(
        diagsOf("one__ and two__\n\n__ https://1.example/\n"),
        ["error 0: Anonymous hyperlink mismatch: 2 references but 1 targets."]);
}

func testSectionTitleIsAnImplicitTarget() {
    def src as string init "Intro\n=====\n\nx\n\nUsage\n=====\n\nSee Intro_ or `usage`_.\n";
    testing.assertContains(h($src), "<a href=\"#intro\">Intro</a> or <a href=\"#usage\">usage</a>");
}

func testInternalTargetGivesTheNextElementAnId() {
    testing.assertEqual(
        h("go_\n\n.. _go:\n\nThe place.\n"),
        "<p><a href=\"#go\">go</a></p><p id=\"go\">The place.</p>");
    # Before a section title, it points at the title's own id.
    testing.assertContains(
        h(".. _top:\n\nA Title\n=======\n\nBack to top_.\n"),
        "<a href=\"#a-title\">top</a>");
}

func testInternalTargetAtTheEnd() {
    testing.assertEqual(h("e_\n\n.. _e:\n"), "<p><a href=\"#e\">e</a></p><div id=\"e\"></div>");
}

func testInlineTarget() {
    testing.assertEqual(
        h("A _`key term` and key-term later: `key term`_.\n"),
        "<p>A <span id=\"key-term\" class=\"target\">key term</span> and key-term later: " +
            "<a href=\"#key-term\">key term</a>.</p>");
}

func testUnknownTarget() {
    def src as string init "nowhere_ here\n";
    testing.assertEqual(h($src), "<p>nowhere_ here</p>");
    testing.assertEqual(diagsOf($src), ["error 1: Unknown target name: \"nowhere\"."]);
}

func testDuplicateTarget() {
    def src as string init "x_\n\n.. _x: https://1.example/\n.. _x: https://2.example/\n";
    testing.assertEqual(diagsOf($src), ["warning 4: Duplicate explicit target name: \"x\"."]);
}

func testNameReferenceBoundaries() {
    # Not a reference: an underscore inside a word, or a name stuck to a word.
    testing.assertEqual(
        h("snake_case and __init__ and a_b_c\n"),
        "<p>snake_case and __init__ and a_b_c</p>");
    # A name may hold single internal punctuation.
    testing.assertEqual(
        h("see web-site_.\n\n.. _web-site: https://w.example/\n"),
        "<p>see <a href=\"https://w.example/\">web-site</a>.</p>");
    # An escaped underscore is literal.
    testing.assertEqual(h("name\\_ here\n"), "<p>name_ here</p>");
}

func testJavascriptTargetIsFiltered() {
    testing.assertEqual(h("x_\n\n.. _x: javascript:alert(1)\n"), "<p><a href=\"#\">x</a></p>");
    testing.assertEqual(h("`x <javascript:alert(1)>`_\n"), "<p><a href=\"#\">x</a></p>");
}

func testStandaloneLinks() {
    testing.assertEqual(
        h("Visit https://example.com/a?b=1. Mail ada@example.org, or mailto:b@example.org!\n"),
        "<p>Visit <a href=\"https://example.com/a?b=1\">https://example.com/a?b=1</a>. Mail " +
            "<a href=\"mailto:ada@example.org\">ada@example.org</a>, or " +
            "<a href=\"mailto:b@example.org\">mailto:b@example.org</a>!</p>");
    # Not inside a word, and not inside literals.
    testing.assertEqual(
        h("xhttps://a.example ``https://b.example``\n"),
        "<p>xhttps://a.example <code>https://b.example</code></p>");
}

# --- footnotes and citations ---

func testFootnoteKinds() {
    def src as string init "a [1]_ b [#]_ c [#n]_ d [*]_\n\n" +
        ".. [1] one\n.. [#] two\n.. [#n] three\n.. [*] four\n";
    def out as string init h($src);
    testing.assertContains(
        $out,
        "<a id=\"footnote-reference-1\" class=\"footnote-reference\" href=\"#footnote-1\" " +
            "role=\"doc-noteref\">[1]</a>");
    testing.assertContains($out, "href=\"#footnote-2\" role=\"doc-noteref\">[2]</a>");
    testing.assertContains($out, "href=\"#footnote-3\" role=\"doc-noteref\">[3]</a>");
    testing.assertContains($out, "href=\"#footnote-sym-1\" role=\"doc-noteref\">[*]</a>");
    testing.assertContains(
        $out,
        "<aside id=\"footnote-3\" class=\"footnote\" role=\"doc-footnote\"><span class=\"label\">" +
            "<a role=\"doc-backlink\" href=\"#footnote-reference-3\">[3]</a></span>" +
            "<p>three</p></aside>");
    testing.assertEqual(len(check($src)), 0);
}

func testAutoNumbersSkipManualOnes() {
    def src as string init "[#]_ [#]_\n\n.. [1] manual\n.. [#] auto a\n.. [#] auto b\n";
    def doc as doctree.Node init parse($src);
    def notes as list of doctree.Node init doctree.findAll($doc, "footnote");
    testing.assertEqual($notes[1].attrs["label"], "2");
    testing.assertEqual($notes[2].attrs["label"], "3");
}

func testSymbolFootnotesCycle() {
    testing.assertEqual(symbolLabel(0), "*");
    testing.assertEqual(symbolLabel(1), "\u2020");
    testing.assertEqual(symbolLabel(10), "**");
}

func testFootnoteWithoutReferenceHasNoBacklink() {
    testing.assertEqual(
        h(".. [1] lonely\n"),
        "<aside id=\"footnote-1\" class=\"footnote\" role=\"doc-footnote\">" +
            "<span class=\"label\">[1]</span>" +
            "<p>lonely</p></aside>");
}

func testUnknownFootnoteReference() {
    def src as string init "x [7]_\n";
    testing.assertEqual(h($src), "<p>x [7]_</p>");
    testing.assertEqual(diagsOf($src), ["error 1: Unknown footnote reference: \"[7]_\"."]);
}

func testCitation() {
    testing.assertEqual(
        h("See [CIT2002]_.\n\n.. [CIT2002] A book.\n"),
        "<p>See <a class=\"citation-reference\" href=\"#citation-cit2002\">[CIT2002]</a>.</p>" +
            "<aside id=\"citation-cit2002\" class=\"citation\">" +
            "<span class=\"label\">[CIT2002]</span>" +
            "<p>A book.</p></aside>");
    testing.assertEqual(diagsOf("See [Nope]_.\n"), ["error 1: Unknown target name: \"nope\"."]);
}

func testFootnoteBodyHoldsBlocks() {
    def doc as doctree.Node init parse(".. [1] First.\n\n   - a list\n   - inside\n");
    testing.assertEqual(len($doc.children[0].children), 2);
    testing.assertEqual($doc.children[0].children[1].kind, "list");
}

# --- substitutions ---

func testSubstitutionReplace() {
    testing.assertEqual(
        h("I |like| it.\n\n.. |like| replace:: *really* like\n"),
        "<p>I <em>really</em> like it.</p>");
}

func testSubstitutionCaseFallbackAndLink() {
    testing.assertEqual(
        h("|Site|_\n\n.. |site| replace:: the site\n.. _site: https://s.example/\n"),
        "<p><a href=\"https://s.example/\">the site</a></p>");
}

func testSubstitutionImage() {
    testing.assertEqual(
        h("A |logo| here.\n\n.. |logo| image:: logo.png\n   :alt: Logo\n   :width: 20\n" +
            "   :target: https://l.example/\n"),
        "<p>A <a href=\"https://l.example/\"><img src=\"logo.png\" alt=\"Logo\" width=\"20\"></a>" +
            " here.</p>");
}

func testSubstitutionUnicode() {
    testing.assertEqual(
        h("|c| 2026\n\n.. |c| unicode:: U+00A9 .. copyright sign\n"),
        "<p>\u00A9 2026</p>");
    testing.assertEqual(unicodeText("0x41 &#66; x43 U+1F600 word"), "ABC\U0001F600word");
}

func testUndefinedAndCircularSubstitutions() {
    testing.assertEqual(diagsOf("|x|\n"), ["error 1: Undefined substitution referenced: \"x\"."]);
    def src as string init "|a|\n\n.. |a| replace:: |b|\n.. |b| replace:: |a|\n";
    testing.assertContains(
        strings.join(diagsOf($src), "\n"),
        "Circular substitution definition referenced");
}

func testUnsupportedSubstitutionDirective() {
    testing.assertEqual(
        diagsOf(".. |d| date::\n"),
        ["error 1: Substitution definition \"d\" uses the unsupported directive \"date\"."]);
}

func testPipeInTextIsNotASubstitution() {
    testing.assertEqual(h("a | b and x|y|z\n"), "<p>a | b and x|y|z</p>");
}

# --- roles ---

func testRoles() {
    testing.assertEqual(
        h(":emphasis:`e` :strong:`s` :literal:`l` :code:`c` `H`:sub: :sup:`2` :title:`T`\n"),
        "<p><em>e</em> <strong>s</strong> <code>l</code> <code>c</code> <sub>H</sub> " +
            "<sup>2</sup> " +
            "<cite>T</cite></p>");
    testing.assertEqual(h(":math:`x^2`\n"), "<p><code class=\"math\">x^2</code></p>");
}

func testPepRfcAndAbbr() {
    testing.assertEqual(
        h(":pep:`8` :rfc:`2822` :abbr:`LIFO (last in, first out)`\n"),
        "<p><a href=\"https://peps.python.org/pep-0008/\">PEP 8</a> " +
            "<a href=\"https://datatracker.ietf.org/doc/html/rfc2822\">RFC 2822</a> " +
            "<abbr title=\"last in, first out\">LIFO</abbr></p>");
    testing.assertEqual(
        diagsOf(":pep:`eight`\n"),
        ["error 1: PEP number must be a number from 0 to 9999; \"eight\" is invalid."]);
}

func testUnknownRoleIsKeptAsText() {
    def src as string init "a :nope:`x` b\n";
    testing.assertEqual(h($src), "<p>a :nope:`x` b</p>");
    testing.assertEqual(diagsOf($src), ["error 1: Unknown interpreted text role \"nope\"."]);
}

func testRoleAndReferenceTogetherIsAnError() {
    testing.assertContains(strings.join(diagsOf(":emphasis:`x`_\n"), "\n"), "Mismatch");
}

func testColonsInProseAreNotRoles() {
    testing.assertEqual(
        h("Note: time 10:30, see http://x.example/a:b\n"),
        "<p>Note: time 10:30, see <a href=\"http://x.example/a:b\">http://x.example/a:b</a></p>");
}

# --- diagnostics come out in line order ---

func testDiagnosticsAreSorted() {
    def src as string init "nowhere_\n\n- a\nb\n\n|undef|\n";
    def ds as list of Diagnostic init check($src);
    testing.assertEqual(len($ds), 3);
    testing.assertTrue($ds[0].line <= $ds[1].line and $ds[1].line <= $ds[2].line);
}

func testEmptyFootnoteStillNumbered() {
    testing.assertContains(h("x [#]_\n\n.. [#]\n"), "href=\"#footnote-1\"");
}

# --- grid tables ---

func testGridTableHeaderAndRowspan() {
    def src as string init "+-----+-----+\n| a   | b   |\n+=====+=====+\n| 1   | 2   |\n|     |     |\n" +
        "| x   | - p |\n+-----+ - q |\n| w   |     |\n+-----+-----+\n";
    testing.assertEqual(
        h($src),
        "<table><thead><tr><th>a</th><th>b</th></tr></thead><tbody><tr><td><p>1</p><p>x</p></td>" +
            "<td rowspan=\"2\"><p>2</p><ul><li>p</li><li>q</li></ul></td></tr><tr><td>w</td></tr></tbody></table>");
    testing.assertEqual(diagsOf($src), []);
}

func testGridTableColspanWithoutHeader() {
    testing.assertEqual(
        h("+---+---+\n| span  |\n+---+---+\n| a | b |\n+---+---+\n"),
        "<table><tbody><tr><td colspan=\"2\">span</td></tr><tr><td>a</td><td>b</td></tr></tbody></table>");
}

func testGridTableTreeShape() {
    def t as doctree.Node init parse("+---+---+\n| a | b |\n+===+===+\n| c | d |\n+---+---+\n").children[0];
    testing.assertEqual($t.kind, "table");
    testing.assertEqual($t.attrs["headerRows"], "1");
    testing.assertEqual(len($t.children), 2);
    testing.assertEqual(doctree.text($t.children[1].children[1]), "d");
}

func testMalformedGridTablesAreKept() {
    def ragged as string init "+---+---+\n| a | b |\n+---+--+\n";
    testing.assertEqual(
        h($ragged),
        "<pre class=\"literal-block\"><code>+---+---+\n| a | b |\n+---+--+</code></pre>");
    testing.assertEqual(
        diagsOf($ragged),
        ["error 1: Malformed table: the lines are not all the same width."]);
    # A missing inner `|` is not an error: the row is one cell, and the table one
    # column, since no cell ends at the inner `+`.
    testing.assertEqual(
        h("+---+---+\n| a   b |\n+---+---+\n"),
        "<table><tbody><tr><td>a   b</td></tr></tbody></table>");
    # A box that never closes leaves the grid unfilled.
    def open as list of list of string init [
        strings.chars("+--+"),
        strings.chars("|  |"),
        strings.chars("+  +")
    ];
    testing.assertEqual(len(scanGrid($open)), 0);
    testing.assertEqual(
        diagsOf("+---+\n| a |\n| b |\n"),
        ["error 1: Malformed table: no bottom table border found."]);
}

func testGridTableInsideAListItem() {
    testing.assertEqual(
        h("- item\n\n  +---+\n  | x |\n  +---+\n"),
        "<ul><li><p>item</p><table><tbody><tr><td>x</td></tr></tbody></table></li></ul>");
}

# --- simple tables ---

func testSimpleTable() {
    def src as string init "=====  =====\nA      B\n=====  =====\n1      2\n       more\n3      4\n=====  =====\n";
    testing.assertEqual(
        h($src),
        "<table><thead><tr><th>A</th><th>B</th></tr></thead><tbody><tr><td>1</td><td>2 more</td></tr>" +
            "<tr><td>3</td><td>4</td></tr></tbody></table>");
}

func testSimpleTableWithoutHeaderAndLongLastColumn() {
    testing.assertEqual(
        h("===  ===\na    b runs on\nc    d\n===  ===\n"),
        "<table><tbody><tr><td>a</td><td>b runs on</td></tr><tr><td>c</td><td>d</td></tr></tbody></table>");
}

func testSimpleTableColumnSpans() {
    def src as string init "=====  =====  =====\n1      2      3\n------------  -----\nx      y      z\n=====  =====  =====\n";
    testing.assertEqual(
        h($src),
        "<table><tbody><tr><td colspan=\"2\">1      2</td><td>3</td></tr>" +
            "<tr><td>x</td><td>y</td><td>z</td></tr></tbody></table>");
}

func testMalformedSimpleTables() {
    testing.assertEqual(
        diagsOf("===  ===\nab   c\naxxxxx d\n===  ===\n"),
        ["error 1: Malformed table: text in a column margin."]);
    def open as string init "===  ===\na    b\n\nafter\n";
    testing.assertEqual(
        diagsOf($open),
        ["error 1: Malformed table: no bottom table border found."]);
    testing.assertContains(h($open), "<p>after</p>");
    testing.assertEqual(
        diagsOf("===  ===\na    b\n---\nc    d\n===  ===\n"),
        ["error 1: Malformed table: a column span underline does not match the columns."]);
}

func testTableIsATarget() {
    testing.assertEqual(
        h(".. _tbl:\n\n===  ===\na    b\n===  ===\n\nSee tbl_.\n"),
        "<table id=\"tbl\"><tbody><tr><td>a</td><td>b</td></tr></tbody></table><p>See <a href=\"#tbl\">tbl</a>.</p>");
}

# --- directives: admonitions and friends ---

func testAdmonitionDirectives() {
    testing.assertEqual(
        h(".. note:: Be *careful*.\n\n   Second para.\n"),
        "<div class=\"admonition admonition-note\"><p class=\"admonition-title\">Note</p>" +
            "<p>Be <em>careful</em>.</p><p>Second para.</p></div>");
    for (def kind in ADMONITIONS) {
        testing.assertContains(h(".. " + $kind + ":: x\n"), "admonition-" + $kind);
    }
    testing.assertEqual(
        h(".. admonition:: My Title\n\n   Body.\n"),
        "<div class=\"admonition admonition-my-title\"><p class=\"admonition-title\">My Title</p><p>Body.</p></div>");
}

func testDirectiveClassAndNameOptions() {
    testing.assertEqual(
        h(".. warning::\n   :class: Big Red\n   :name: my-warn\n\n   Watch out.\n\nSee my-warn_.\n"),
        "<div id=\"my-warn\" class=\"admonition admonition-warning big red\"><p class=\"admonition-title\">Warning</p>" +
            "<p>Watch out.</p></div><p>See <a href=\"#my-warn\">my-warn</a>.</p>");
}

func testDirectiveErrorsKeepTheSource() {
    testing.assertEqual(
        h(".. note::\n"),
        "<pre class=\"literal-block\"><code>.. note::</code></pre>");
    testing.assertEqual(
        diagsOf(".. note::\n"),
        ["error 1: The \"note\" admonition is empty; content is required."]);
    testing.assertEqual(
        diagsOf(".. admonition::\n\n   body\n"),
        ["error 1: The \"admonition\" directive needs a title."]);
    testing.assertEqual(
        diagsOf(".. note::\n   :bogus: 1\n\n   x\n"),
        ["error 1: Unknown option \"bogus\" for the \"note\" directive; ignored."]);
}

func testIncludeIsRefused() {
    def src as string init ".. include:: /etc/passwd\n";
    testing.assertEqual(
        h($src),
        "<pre class=\"literal-block\"><code>.. include:: /etc/passwd</code></pre>");
    testing.assertEqual(
        diagsOf($src),
        ["error 1: The \"include\" directive is not supported (rst reads no files)."]);
}

func testTopicSidebarRubric() {
    testing.assertEqual(
        h(".. topic:: A long\n   title here\n\n   Body.\n"),
        "<aside class=\"topic\"><p class=\"topic-title\">A long title here</p><p>Body.</p></aside>");
    testing.assertEqual(
        h(".. sidebar:: Side\n\n   Body.\n"),
        "<aside class=\"topic sidebar\"><p class=\"topic-title\">Side</p><p>Body.</p></aside>");
    testing.assertEqual(h(".. rubric:: A *rubric*\n"), "<p class=\"rubric\">A <em>rubric</em></p>");
}

func testQuoteDirectivesKeepTheAttribution() {
    testing.assertEqual(
        h(".. epigraph::\n\n   No matter where you go.\n\n   -- Buckaroo Banzai\n"),
        "<blockquote class=\"epigraph\"><p>No matter where you go.</p>" +
            "<p class=\"attribution\">—Buckaroo Banzai</p></blockquote>");
}

func testContainerAndClass() {
    testing.assertEqual(
        h(".. container:: custom Two\n\n   Inside.\n"),
        "<div class=\"custom two\"><p>Inside.</p></div>");
    testing.assertEqual(
        h(".. compound::\n\n   Text.\n"),
        "<div class=\"compound\"><p>Text.</p></div>");
    testing.assertEqual(
        h(".. class:: special\n\nThis one.\n\nNot this.\n"),
        "<p class=\"special\">This one.</p><p>Not this.</p>");
    testing.assertEqual(
        h(".. class:: k\n\n   One.\n\n   Two.\n"),
        "<p class=\"k\">One.</p><p class=\"k\">Two.</p>");
    testing.assertEqual(
        diagsOf("Para.\n\n.. class:: x\n"),
        ["warning 3: The \"class\" directive has no element after it to apply to."]);
}

# --- directives: literal content ---

func testCodeDirectives() {
    def src as string init ".. code-block:: Python\n   :linenos:\n\n   def f():\n       return 1\n";
    testing.assertEqual(
        h($src),
        "<pre><code class=\"language-python\">def f():\n    return 1</code></pre>");
    testing.assertEqual(diagsOf($src), []);
    testing.assertEqual(h(".. code::\n\n   plain\n"), "<pre><code>plain</code></pre>");
    testing.assertEqual(
        h(".. sourcecode:: c\n\n   x;\n"),
        "<pre><code class=\"language-c\">x;</code></pre>");
}

func testMathDirective() {
    testing.assertEqual(
        h(".. math::\n\n   a^2 + b^2 = c^2\n\n   x = y\n"),
        "<pre class=\"math\"><code>a^2 + b^2 = c^2</code></pre><pre class=\"math\"><code>x = y</code></pre>");
    testing.assertEqual(
        h(".. math:: e = mc^2\n"),
        "<pre class=\"math\"><code>e = mc^2</code></pre>");
}

func testRawDirectiveIsGated() {
    def src as string init ".. raw:: html\n\n   <b>hi</b>\n\n.. raw:: latex\n\n   \\foo\n";
    testing.assertEqual(h($src), "&lt;b&gt;hi&lt;/b&gt;");
    def o as Options;
    $o.allowRawHtml = true;
    testing.assertEqual(toHtmlWith($src, $o), "<b>hi</b>");
    testing.assertEqual(
        diagsOf(".. raw:: html\n   :file: x.html\n"),
        [
            "error 1: The \"raw\" directive's :file: and :url: options are not supported (rst reads no files)."
        ]);
}

# --- directives: images and figures ---

func testImageDirective() {
    testing.assertEqual(
        h(".. image:: pic.png\n   :alt: A picture\n   :width: 100\n   :align: center\n   :target: https://example.org\n"),
        "<p><a href=\"https://example.org\"><img src=\"pic.png\" alt=\"A picture\" class=\"align-center\" width=\"100\"></a></p>");
    testing.assertEqual(
        h(".. image:: long/\n   path.png\n"),
        "<p><img src=\"long/path.png\" alt=\"long/path.png\"></p>");
    testing.assertEqual(
        h(".. image:: javascript:alert(1)\n"),
        "<p><img src=\"#\" alt=\"javascript:alert(1)\"></p>");
}

func testFigureDirective() {
    testing.assertEqual(
        h(".. figure:: fig.png\n   :figclass: wide\n\n   The *caption*.\n\n   A legend.\n"),
        "<figure class=\"wide\"><img src=\"fig.png\" alt=\"fig.png\"><figcaption><p>The <em>caption</em>.</p>" +
            "<div class=\"legend\"><p>A legend.</p></div></figcaption></figure>");
    testing.assertEqual(
        h(".. figure:: f.png\n\n   ..\n\n   Legend only.\n"),
        "<figure><img src=\"f.png\" alt=\"f.png\"><figcaption><div class=\"legend\"><p>Legend only.</p></div></figcaption></figure>");
    testing.assertEqual(
        h(".. figure:: f.png\n   :name: fig1\n\n   Cap.\n\nSee fig1_.\n"),
        "<figure id=\"fig1\"><img src=\"f.png\" alt=\"f.png\"><figcaption><p>Cap.</p></figcaption></figure>" +
            "<p>See <a href=\"#fig1\">fig1</a>.</p>");
    testing.assertEqual(
        diagsOf(".. figure:: f.png\n\n   - not a caption\n"),
        ["error 1: A figure's caption must be a paragraph or an empty comment."]);
}

# --- directives: contents ---

func testContentsDirective() {
    testing.assertEqual(
        h("Title\n=====\n\n.. contents:: Table\n   :depth: 1\n\nOne\n---\n\nSub\n~~~\n\nTwo\n---\n"),
        "<h1 id=\"title\" class=\"title\">Title</h1><aside class=\"topic contents\"><p class=\"topic-title\">Table</p>" +
            "<ul><li><a href=\"#one\">One</a></li><li><a href=\"#two\">Two</a></li></ul></aside>" +
            "<h2 id=\"one\">One</h2><h3 id=\"sub\">Sub</h3><h2 id=\"two\">Two</h2>");
    testing.assertContains(
        h(".. contents::\n\nA\n=\n\nB\n-\n"),
        "<p class=\"topic-title\">Contents</p><ul><li><a href=\"#a\">A</a><ul><li><a href=\"#b\">B</a></li></ul></li></ul>");
}

func testLocalContents() {
    def src as string init "Doc\n===\n\nA\n---\n\n.. contents::\n   :local:\n\nA1\n~~\n\nA2\n~~\n\nB\n---\n\nB1\n~~\n";
    testing.assertContains(
        h($src),
        "<aside class=\"topic contents local\"><ul><li><a href=\"#a1\">A1</a></li><li><a href=\"#a2\">A2</a></li></ul></aside>");
}

func testNestedContentsIsReported() {
    testing.assertEqual(
        diagsOf(".. note::\n\n   .. contents::\n"),
        ["error 3: The \"contents\" directive is only supported at the top level of a document."]);
}

# --- directives: list and CSV tables ---

func testListTable() {
    def src as string init ".. list-table:: Frozen\n   :header-rows: 1\n   :stub-columns: 1\n\n" +
        "   * - Treat\n     - Qty\n   * - Albatross\n     - 2.99\n";
    testing.assertEqual(
        h($src),
        "<table><caption>Frozen</caption><thead><tr><th class=\"stub\">Treat</th><th>Qty</th></tr></thead>" +
            "<tbody><tr><td class=\"stub\">Albatross</td><td>2.99</td></tr></tbody></table>");
    testing.assertEqual(diagsOf($src), []);
}

func testListTableMustBeUniform() {
    testing.assertEqual(
        diagsOf(".. list-table::\n\n   * - a\n     - b\n   * - c\n"),
        [
            "error 1: The \"list-table\" directive needs a uniform two-level bullet list. " +
                "Its rows have different numbers of cells."
        ]);
    testing.assertEqual(
        diagsOf(".. list-table::\n\n   Just a paragraph.\n"),
        ["error 1: The \"list-table\" directive needs a uniform two-level bullet list."]);
    testing.assertEqual(
        diagsOf(".. list-table::\n   :header-rows: 5\n\n   * - a\n"),
        ["error 1: The \"list-table\" directive's :header-rows: is not a row count of the table."]);
}

func testCsvTable() {
    def src as string init ".. csv-table:: Prices\n   :header: \"Treat\", \"Price\"\n   :widths: 10, 5\n\n" +
        "   \"Albatross\", 2.99\n   \"Crunchy, Frog\", 1.49\n   *short*\n";
    testing.assertEqual(
        h($src),
        "<table><caption>Prices</caption><thead><tr><th>Treat</th><th>Price</th></tr></thead><tbody>" +
            "<tr><td>Albatross</td><td>2.99</td></tr><tr><td>Crunchy, Frog</td><td>1.49</td></tr>" +
            "<tr><td><em>short</em></td><td></td></tr></tbody></table>");
    testing.assertEqual(diagsOf($src), []);
    testing.assertEqual(
        h(".. csv-table::\n   :delim: ;\n   :header-rows: 1\n\n   a;b\n   1;2\n"),
        "<table><thead><tr><th>a</th><th>b</th></tr></thead><tbody><tr><td>1</td><td>2</td></tr></tbody></table>");
}

func testCsvTableReadsNoFiles() {
    testing.assertEqual(
        diagsOf(".. csv-table::\n   :file: x.csv\n"),
        [
            "error 1: The \"csv-table\" directive's :file: and :url: options are not supported (rst reads no files)."
        ]);
}

func hugeCsvTable() {
    return parse(".. csv-table::\n\n" + strings.repeat("   a,b\n", 10001));
}

func testHugeTableIsAnRstError() {
    testing.assertThrows("hugeCsvTable", "rst");
}

func testTablesAndDirectivesRenderToAnsiAndPdf() {
    def src as string init ".. list-table:: T\n\n   * - a\n     - b\n\n+---+---+\n| span  |\n+---+---+\n| a | b |\n+---+---+\n\n" +
        ".. figure:: x.png\n\n   Cap.\n\n.. note:: n\n\n.. math::\n\n   x^2\n";
    def ansiOut as string init toAnsi($src);
    testing.assertContains($ansiOut, "a | b");
    testing.assertContains($ansiOut, "Note");
    def pdfOut as bytes init doctree.renderPdf(parse($src), doctree.pdfDefaults());
    testing.assertTrue(len($pdfOut) > 500);
}
