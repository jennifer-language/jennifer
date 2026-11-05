# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# myst_test.j - white-box tests for myst.j. Run with:
#
#     jennifer test modules/myst_test.j
#
# The overlay splices myst.j (and its included myst_directives.inc.j) in front
# of this file, so the tests reach private helpers (preprocess, inlineMath,
# readDirective, ...) by bare identifier, next to the exported surface. MyST
# sources are raw '...' strings: their braces must not interpolate.
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

func opts() {
    def o as Options;
    return $o;
}

# --- the pre-pass ---

func testColonFenceBecomesBacktickFence() {
    def out as string init preprocess(':::{note} Title
body
:::', 1, opts());
    testing.assertContains($out, '```{note} Title');
    testing.assertContains($out, "body");
}

func testCommentsBreaksAndFrontMatter() {
    def src as string init '---
title: Doc
---
% a comment
One.

+++

Two.';
    testing.assertEqual(h($src), "<p>One.</p><hr><p>Two.</p>");
    testing.assertEqual(frontMatter($src), "title: Doc");
    testing.assertEqual(frontMatter("no front matter"), "");
}

func testInlineMathRules() {
    testing.assertEqual(inlineMath('a $x^2$ b'), 'a {math}`x^2` b');
    testing.assertEqual(inlineMath('costs $5 and $10'), 'costs $5 and $10');
    testing.assertEqual(inlineMath('`$x$` stays'), '`$x$` stays');
    testing.assertEqual(inlineMath('escaped \$x\$'), 'escaped \$x\$');
    testing.assertEqual(inlineMath('tick $a`b$'), 'tick {math}``a`b``');
}

func testDollarMathCanBeOff() {
    def o as Options init opts();
    $o.noDollarMath = true;
    testing.assertEqual(toHtmlWith('a $x$ b', $o), "<p>a $x$ b</p>");
}

func testColonFenceCanBeOff() {
    def o as Options init opts();
    $o.noColonFence = true;
    testing.assertContains(toHtmlWith(':::{note}
x
:::', $o), '<p>:::{note} x :::</p>');
}

# --- directives ---

func testAdmonitions() {
    testing.assertEqual(
        h('```{note}
Mind the *gap*.
```'),
        "<div class=\"admonition admonition-note\"><p class=\"admonition-title\">Note</p>" +
            "<p>Mind the <em>gap</em>.</p></div>");
    for (def kind in ADMONITIONS) {
        testing.assertContains(h('```{' + $kind + '}
x
```'), "admonition-" + $kind);
    }
    testing.assertEqual(
        h('```{admonition} My title
:class: dropdown

Body.
```'),
        "<div class=\"admonition admonition-my-title dropdown\"><p class=\"admonition-title\">My title</p>" +
            "<p>Body.</p></div>");
    # A specific admonition's argument starts its content.
    testing.assertContains(h('```{tip} Do this.
```'), "<p>Do this.</p>");
}

func testNestedColonFences() {
    testing.assertEqual(
        h('::::{important}
Outer.

:::{tip}
Inner *tip*.
:::
::::'),
        "<div class=\"admonition admonition-important\"><p class=\"admonition-title\">Important</p><p>Outer.</p>" +
            "<div class=\"admonition admonition-tip\"><p class=\"admonition-title\">Tip</p><p>Inner <em>tip</em>.</p>" +
            "</div></div>");
}

func testYamlOptionsBlock() {
    def d as Directive init readDirective(markdown.parse('```{figure} a.png
---
name: fig
alt: "Alt text"
---
Caption.
```').children[0]);
    testing.assertEqual($d.options["name"], "fig");
    testing.assertEqual($d.options["alt"], "Alt text");
    testing.assertEqual(trimmedContent($d.content), ["Caption."]);
}

func testCodeAndMath() {
    testing.assertEqual(
        h('```{code-block} Python
:linenos:

def f():
    return 1
```'),
        "<pre><code class=\"language-python\">def f():\n    return 1</code></pre>");
    testing.assertEqual(
        h('$$
x = 1
$$ (eq-one)

See {eq}`eq-one`.'),
        "<pre id=\"eq-one\" class=\"math\"><code>x = 1</code></pre><p>See <a href=\"#eq-one\">(eq-one)</a>.</p>");
    testing.assertEqual(h('Inline $a^2$.'), "<p>Inline <code class=\"math\">a^2</code>.</p>");
}

func testImageAndFigure() {
    testing.assertEqual(
        h('```{image} pic.png
:alt: A picture
:width: 100
:align: center
```'),
        "<p><img src=\"pic.png\" alt=\"A picture\" class=\"align-center\" width=\"100\"></p>");
    testing.assertEqual(
        h('```{figure} img.png
:name: fig-one

The *caption*.

Legend.
```

See {numref}`fig-one`.'),
        "<figure id=\"fig-one\"><img src=\"img.png\" alt=\"img.png\"><figcaption><p>The <em>caption</em>.</p>" +
            "<div class=\"legend\"><p>Legend.</p></div></figcaption></figure><p>See <a href=\"#fig-one\">The caption.</a>.</p>");
}

func testTables() {
    testing.assertEqual(
        h('```{list-table} Frozen
:header-rows: 1

* - Treat
  - Qty
* - Albatross
  - 2.99
```'),
        "<table><caption>Frozen</caption><thead><tr><th>Treat</th><th>Qty</th></tr></thead>" +
            "<tbody><tr><td>Albatross</td><td>2.99</td></tr></tbody></table>");
    testing.assertEqual(
        h('```{table} Caption
:name: tbl

| a | b |
|---|---|
| 1 | 2 |
```

{ref}`tbl`'),
        "<table id=\"tbl\"><caption>Caption</caption><thead><tr><th>a</th><th>b</th></tr></thead>" +
            "<tbody><tr><td>1</td><td>2</td></tr></tbody></table><p><a href=\"#tbl\">Caption</a></p>");
    testing.assertEqual(
        h('```{csv-table} Prices
:header: "Treat", "Price"

"Crunchy, Frog", *1.49*
```'),
        "<table><caption>Prices</caption><thead><tr><th>Treat</th><th>Price</th></tr></thead>" +
            "<tbody><tr><td>Crunchy, Frog</td><td><em>1.49</em></td></tr></tbody></table>");
}

func testListTableMustBeUniform() {
    testing.assertEqual(
        diagsOf('```{list-table}
* - a
  - b
* - c
```'),
        [
            "error 1: The \"list-table\" directive needs a uniform two-level bullet list. " +
                "Its rows have different numbers of cells."
        ]);
}

func testEvalRst() {
    testing.assertEqual(
        h('```{eval-rst}
.. note:: From *rST*.
```'),
        "<div class=\"admonition admonition-note\"><p class=\"admonition-title\">Note</p><p>From <em>rST</em>.</p></div>");
    testing.assertEqual(
        diagsOf('Intro.

```{eval-rst}
see nowhere_
```'),
        ["error 4: eval-rst: Unknown target name: \"nowhere\"."]);
}

func testRawIsGated() {
    def src as string init '```{raw} html
<b>hi</b>
```';
    testing.assertEqual(h($src), "&lt;b&gt;hi&lt;/b&gt;");
    def o as Options init opts();
    $o.allowRawHtml = true;
    testing.assertEqual(toHtmlWith($src, $o), "<b>hi</b>");
}

func testContents() {
    testing.assertEqual(
        h('# Doc

```{contents}
:depth: 1
```

## A

## B'),
        "<h1 id=\"doc\">Doc</h1><aside class=\"topic contents\"><p class=\"topic-title\">Contents</p>" +
            "<ul><li><a href=\"#doc\">Doc</a></li></ul></aside><h2 id=\"a\">A</h2><h2 id=\"b\">B</h2>");
}

func testUnknownDirectivesAndFilesAreReported() {
    testing.assertEqual(
        h('```{frobnicate}
x
```'),
        '<pre class="literal-block"><code>```{frobnicate}' + "\nx\n```</code></pre>");
    testing.assertEqual(
        diagsOf('Text.

```{frobnicate}
x
```'),
        ["error 3: Unknown directive type \"frobnicate\"."]);
    testing.assertEqual(
        diagsOf('```{include} ../README.md
```'),
        ["error 1: The \"include\" directive is not supported (myst reads no files)."]);
    testing.assertEqual(
        diagsOf('```{note}
:bogus: 1

x
```'),
        ["error 1: Unknown option \"bogus\" for the \"note\" directive; ignored."]);
}

# --- roles, targets and references ---

func testRoles() {
    testing.assertEqual(
        h('H{sub}`2`O, mc{sup}`2`, {abbr}`CSS (Cascading Style Sheets)`, {kbd}`Ctrl+C`, {pep}`8`, {rfc}`2119`.'),
        "<p>H<sub>2</sub>O, mc<sup>2</sup>, <abbr title=\"Cascading Style Sheets\">CSS</abbr>, " +
            "<code class=\"kbd\">Ctrl+C</code>, <a href=\"https://peps.python.org/pep-0008/\">PEP 8</a>, " +
            "<a href=\"https://datatracker.ietf.org/doc/html/rfc2119\">RFC 2119</a>.</p>");
    testing.assertEqual(
        h('{emphasis}`a` {strong}`b` {literal}`c`'),
        "<p><em>a</em> <strong>b</strong> <code>c</code></p>");
}

func testUnknownRoleIsKeptAndReported() {
    # Kept as its source text, backticks and all.
    testing.assertEqual(h('x {bogus}`y`'), '<p>x {bogus}`y`</p>');
    testing.assertEqual(
        diagsOf('x {bogus}`y`'),
        ["warning 1: Unknown role \"bogus\"; kept as text."]);
}

func testTargetsAndReferences() {
    def src as string init '(sec-intro)=
# Introduction

See {ref}`sec-intro`, {ref}`the intro <sec-intro>` and [a link](#sec-intro).';
    testing.assertEqual(
        h($src),
        "<h1 id=\"sec-intro\">Introduction</h1><p>See <a href=\"#sec-intro\">Introduction</a>, " +
            "<a href=\"#sec-intro\">the intro</a> and <a href=\"#sec-intro\">a link</a>.</p>");
    testing.assertEqual(diagsOf($src), []);
}

func testUnknownReferenceTarget() {
    def src as string init 'Para.

See {ref}`nope`.';
    testing.assertEqual(h($src), "<p>Para.</p><p>See nope.</p>");
    testing.assertEqual(diagsOf($src), ['warning 3: Unknown target "nope" in a {ref} reference.']);
}

func testDocRoleLinks() {
    testing.assertEqual(
        h('{doc}`Guide <guide/intro>`'),
        "<p><a href=\"guide/intro\">Guide</a></p>");
}

# --- everything markdown reads still works ---

func testMarkdownExtensionsPassThrough() {
    testing.assertContains(
        h('Text[^1].

[^1]: The *note*.'),
        "<aside id=\"fn-1\" class=\"footnote\"");
    testing.assertEqual(h('Term
: Definition'), "<dl><dt>Term</dt><dd>Definition</dd></dl>");
    testing.assertEqual(
        h('| a |
| - |
| 1 |'),
        "<table><thead><tr><th>a</th></tr></thead><tbody><tr><td>1</td></tr></tbody></table>");
}

func testDirectiveInsideListItem() {
    testing.assertEqual(
        h('- item

  ```{note}
  inside
  ```'),
        "<ul><li><p>item</p><div class=\"admonition admonition-note\"><p class=\"admonition-title\">Note</p>" +
            "<p>inside</p></div></li></ul>");
}

# --- strictness, limits, other renderers ---

func strictFails() {
    def o as Options init opts();
    $o.strict = true;
    parseWith('x {bogus}`y`', $o);
}

func testStrictThrows() {
    testing.assertThrows("strictFails", "myst");
}

func deepDirectives() {
    def src as string init "";
    for (def k in 0..60) {
        $src = $src + strings.repeat(":", 70 - $k) + '{note}' + "\n";
    }
    $src = $src + "x\n";
    for (def k in 0..60) {
        $src = $src + strings.repeat(":", 11 + $k) + "\n";
    }
    return parse($src);
}

func testDeepNestingIsAMystError() {
    testing.assertThrows("deepDirectives", "myst");
}

func testAnsiAndPdf() {
    def src as string init '```{note}
n
```

| a | b |
|---|---|
| 1 | 2 |';
    testing.assertContains(toAnsi($src), "Note");
    testing.assertTrue(len(doctree.renderPdf(parse($src), doctree.pdfDefaults())) > 500);
    testing.assertEqual(toXhtml('---
title: x
---
a\
b'), "<p>a<br />b</p>");
}
