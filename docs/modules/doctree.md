# `doctree` - a document tree, rendered to HTML, XHTML, ANSI and PDF

Import with `import "doctree.j" as doctree;`. A `doctree.Node` is one element of
a document (a heading, a paragraph, a list, a table, a link, a run of text)
with its children. Readers build these trees ([`markdown.parse`](markdown.md),
[`rst.parse`](rst.md), [`myst.parse`](myst.md)); a program can also build one
by hand. Either way the same renderers turn it into HTML (through [`html`](html.md), which handles
escaping), XHTML, styled terminal text (through [`ansi`](ansi.md)) or a
paginated PDF (through [`pdf`](pdf.md)), and the same accessors walk it. Pure
Jennifer; runs on either binary.

```jennifer
use io;
import "markdown.j" as markdown;
import "doctree.j" as doctree;

def doc as doctree.Node init markdown.parse("# Hi\n\nA **bold** word.");
io.printf("%s\n", doctree.render("html", $doc));
# <h1>Hi</h1><p>A <strong>bold</strong> word.</p>
io.printf("%s\n", doctree.render("ansi", $doc));   # styled on a TTY
```

Runnable: [`examples/modules/doctree_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/doctree_demo.j).

## Surface

Rendering:

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `doctree.render(format, doc)` | `string` | `format` is `"html"` / `"xhtml"` / `"ansi"`. A `"document"` node renders its children; any other node renders as one block. An unknown format is a catchable `"doctree"` error. |
| `doctree.renderHtml(doc, opts)` | `string` | HTML with an `HtmlOptions`: `allowRawHtml` passes `html_block` nodes through (trusted input only), `xhtml` writes XHTML. |
| `doctree.renderPdf(doc, opts)` | `bytes` | Lay the tree out to a paginated PDF. |
| `doctree.renderPdfDoc(doc, opts)` | `pdf.Document` | Lay the tree out to a `pdf.Document` (not yet serialized), so document-level work (a running header / footer, extra metadata) can happen before `pdf.render`. |
| `doctree.layoutPdf(doc, opts)` | `PdfResult` | `renderPdfDoc` plus the list of characters that could not be drawn (`missing`, a `list of MissingGlyph`); see [Fonts](#fonts-standard-14-and-embedded-truetype). |
| `doctree.pdfDefaults()` | `PdfOptions` | US Letter, 54-pt margins, Helvetica / Courier; copy it and tweak the fields. |
| `doctree.gray(level)` / `doctree.rgb(r, g, b)` / `doctree.noFill()` | `Fill` | A background colour (or none) for headings, table headers, code and quotes. |
| `doctree.headingStyle(fill)` | `HeadingStyle` | One heading level's style, for `PdfOptions.headingStyles`. |
| `doctree.pageBreak()` | `Node` | A page-break node; `renderPdf` starts the next content on a fresh page, HTML and ANSI ignore it. |

Reading (walked like [`xml`](../libraries/xml.md) / [`html`](html.md)):

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `doctree.typeOf(node)` | `string` | The node kind (`"heading"`, `"link"`, `"text"`, ...). |
| `doctree.children(node)` | `list of Node` | The node's direct children. |
| `doctree.text(node)` | `string` | A leaf's text, else its descendants' text concatenated. |
| `doctree.level(node)` | `int` | A heading's level (1-6); 0 otherwise. |
| `doctree.attr(node, name)` | `string` | `"href"` (or `"url"`) / `"title"` / `"lang"` (or `"language"`) / `"kind"` (an admonition's) / `"align"` / `"ordered"` / `"level"`; a code block's `"info"`; a task item's `"task"` / `"checked"`; any key in `attrs` (`"id"`, `"class"`, ...); else `""`. |
| `doctree.get(node, sel)` | `Node` | First node matching a `/`-separated selector, or an empty node. |
| `doctree.findAll(node, sel)` | `list of Node` | Every node matching the selector. |
| `doctree.has(node, sel)` | `bool` | Whether any node matches. |

## The `Node`

| Field | Type | Meaning |
| ----- | ---- | ------- |
| `kind` | `string` | the node kind |
| `level` | `int` | a heading's level (1-6) |
| `text` | `string` | a leaf's text; an image's alt |
| `lang` | `string` | a code block's language; an admonition's kind |
| `ordered` | `bool` | an ordered list |
| `url` | `string` | a link / image target |
| `title` | `string` | a link / image title; a topic's or admonition's title |
| `align` | `string` | a table cell's alignment |
| `attrs` | `map of string to string` | `id`, `class` and kind-specific attributes |
| `children` | `list of Node` | the child nodes |

Block kinds: `"document"`, `"heading"`, `"paragraph"`, `"code"`, `"list"` /
`"item"`, `"table"` / `"row"` / `"cell"`, `"quote"`, `"admonition"`,
`"definition_list"` / `"def_term"` / `"def_desc"`, `"thematic_break"`,
`"html_block"`, `"page_break"`. Inline kinds: `"text"`, `"codespan"`,
`"strong"`, `"emphasis"`, `"strikethrough"`, `"highlight"`, `"subscript"`,
`"superscript"`, `"link"`, `"image"`, `"line_break"` (a hard break: `<br>`, a
newline in ANSI, a line end in PDF), `"html_inline"` (raw inline HTML in
`text`). A literal of the struct may leave fields out (it is a module that
ships with Jennifer), so a hand-built node names only what it uses:

```jennifer
def para as doctree.Node init doctree.Node{
    kind: "paragraph",
    children: [doctree.Node{kind: "text", text: "Built by hand."}]
};
def doc as doctree.Node init doctree.Node{kind: "document", children: [$para]};
```

### More kinds

Beyond the kinds `markdown.parse` builds, the renderers take these, which
[`rst`](rst.md) builds and a hand-built tree may use:

| Kind | Holds | HTML |
| ---- | ----- | ---- |
| `line_block` | `line` nodes (inline children) and nested `line_block`s | `<div class="line-block">` of `<div class="line">`, an empty line a `<br>` |
| `footnote` / `citation` | blocks; `attr "label"`, optional `attr "backref"` | `<aside class="footnote" role="doc-footnote">` with a `[label]` that links back |
| `figure` | an `image`, a `caption` (inline), a `legend` (blocks) | `<figure>` + `<figcaption>` |
| `topic` | blocks; the `title` field | `<aside class="topic">` with a `topic-title` paragraph |
| `rubric` | inline | `<p class="rubric">` |
| `container` | blocks | `<div>` |
| `footnote_ref` / `citation_ref` | the label as `text`, the target as `url` (`#id`) | `<a class="footnote-reference">[1]</a>` |
| `span` / `cite` / `abbr` | inline (`abbr`'s expansion in `title`) | `<span>` / `<cite>` / `<abbr title>` |

**Blocks inside containers.** An `item`, `def_desc`, `cell` or footnote may hold
blocks (paragraphs, code, nested lists, tables) as well as inline nodes. HTML
renders each child as what it is. ANSI and PDF lay the blocks out one under
another in the item's column, and a run of inline children becomes a paragraph.

**Attributes on any node.** In a hand-built tree, `attrs` `id` / `class` (and the
other names the attribute-list allowlist admits) reach the HTML of every block
and of `codespan`, a `class` merged with the renderer's own
(`class="admonition admonition-note extra"`). Event handlers and `style` are
dropped, as for an attribute list.

**Lists and tables.** An ordered `list` takes `attr "start"` (a number) and
`attr "enumtype"` (`1`, `a`, `A`, `i`, `I`): HTML emits `<ol start type>`, and
ANSI / PDF number the items that way (`iii.`, `iv.`). A `table`'s
`attr "headerRows"` sets how many leading rows are the header (default 1, `0`
for none, which drops the `<thead>`). A table's `title` is its caption: a
`<caption>` in HTML, a bold line above the table in ANSI and PDF. A `cell`'s
`colspan` / `rowspan` reach the HTML as written; ANSI and PDF draw fixed columns, so they keep a spanning
cell's text in its first slot and leave the slots it covers empty.

## Reading and transforming a tree

`markdown.parse(md)` and `rst.parse(src)` return the document as a tree of
`Node`s, walked with the same accessors as [`xml`](../libraries/xml.md) /
[`html`](html.md), so a document can be inspected or transformed (pull the
headings for a table of contents, rewrite links, lint) and then rendered. A
node's `typeOf` is one of the kinds [above](#the-node). `get` / `findAll` /
`has` take a `/`-separated selector whose steps are kind names, `*` (any kind), or `name[k]` (the k-th such child, 1-based),
matching direct children. Any other bracket in a step (`paragraph[0]`,
`paragraph[x]`, `paragraph[1`) is a kind `"doctree"` error.

```jennifer
use io;
import "markdown.j" as markdown;
import "doctree.j" as doctree;

def doc as doctree.Node init markdown.parse("# Title\n\nSee [docs](https://example.com).\n");

# Walk the headings for an outline.
for (def h in doctree.findAll($doc, "heading")) {
    io.printf("H{doctree.level($h)}: {doctree.text($h)}\n");   # H1: Title
}

# Pull a link's target.
def a as doctree.Node init doctree.get($doc, "paragraph/link");
io.printf("{doctree.text($a)} -> {doctree.attr($a, 'href')}\n");   # docs -> https://example.com

# Render straight from the (possibly transformed) tree.
io.printf("{doctree.render('html', $doc)}\n");
```

`attr` reads a node's string attributes: `"href"` / `"title"` (a link or image),
`"lang"` (a fenced code block's language), `"align"` (a table cell), `"ordered"`
(`"true"` / `"false"`, a list), and `"level"` (a heading). `render(format, doc)`
renders a `"document"` node's children (or any single node as one block); it and
the parse are hardened with a nesting-depth cap and a total-node budget, so a
pathological document is a catchable kind `"limit"` error rather than an
unbounded parse.

## HTML output and safety

`render("html", doc)` builds an [`html`](html.md) node tree and renders it, so
all text and every link target are escaped (`&`, `<`, `>` in text and code,
`&`/`"` in an `href`) and the markup is always well-formed:

```jennifer
doctree.render("html", markdown.parse("[t](http://x/?a=1&b=2) and <b> & `x<y`"));
# <p><a href="http://x/?a=1&amp;b=2">t</a> and &lt;b&gt; &amp; <code>x&lt;y</code></p>
```

**Safe by default for untrusted input.** An `html_block` or `html_inline` node
(raw HTML from the source: Markdown's raw HTML, rST's `raw` directive) is
escaped, so rendering a README, a comment, or any publisher-supplied document cannot inject a
`<script>` or an event-handler attribute (`<img onerror=...>`, a stored-XSS
vector); it renders as visible text. Link and image URLs go through an
allow-list (`javascript:` and other unsafe schemes become `#`), and attribute
names outside a small allow-list (`id`, `class`, `title`, `width`, `data-*`,
`aria-*`, ...) are dropped, so event handlers and `style` never reach the page.

For trusted input where raw HTML is intended, opt back in to verbatim passthrough
(of both kinds):

```jennifer
doctree.renderHtml($doc, doctree.HtmlOptions{allowRawHtml: true});
```

`HtmlOptions{xhtml: true}` writes XHTML, as `render("xhtml", doc)` does. XHTML
writes a table cell's alignment as `style="text-align: ..."` (EPUB 3 has no
`align` on `th` / `td`); HTML keeps `align`.

Output is compact (no newlines between block elements), which diffs and
round-trips predictably; wrap it in your own formatter if you need indented
source. A code block's content is escaped but never interpreted.

## ANSI output

`render("ansi", doc)` renders for a terminal: headings and `**bold**` in bold, `*italic*`
in italic, inline code in cyan, links underlined with their URL in
parentheses, list items with `- ` / `N. ` markers, and code indented and
dimmed. Styling comes from the `ansi` module, which suppresses itself when
stdout is not a terminal (or `NO_COLOR` is set) and is forced on by
`FORCE_COLOR`, so piped output is plain text. Control characters
in the source text (all C0 but tab and newline, DEL, C1) become U+FFFD, so
untrusted Markdown cannot emit its own escape sequences;
`ansi.strip(doctree.render("ansi", $doc))` removes the styling.

## PDF output

`renderPdf(doc, opts)` lays a tree out to a paginated PDF over [`pdf`](pdf.md)'s
layout primitives (`pdf` pulls in `font`, so importing `doctree`, and with it
`markdown` or `rst`, carries that weight even for an HTML-only program; see
[design-decisions.md](../technical/design-decisions.md)). `markdown.toPdf(md)` /
`markdown.toPdfWith(md, opts)` are the Markdown-text shortcuts. Text a font
cannot draw (outside WinAnsi for a standard-14 font, missing from an embedded
face) is replaced per the `unencodable` option (below) rather than aborting the
render, and `layoutPdf` lists every replaced character. `PdfOptions` comes from
`doctree.pdfDefaults()` (US Letter 612 x 792 pt, 54-pt margins, Helvetica 11 pt /
Courier, `tablePad` 4); copy it and tweak the fields (`pageWidth` /
`pageHeight` / `margin` / `bodyFont` / `boldFont` / `italicFont` / `monoFont` /
`headingFont` / `bodySize` / `tablePad`). A level-1 heading starts a new page,
and `tablePad` tunes cell density. Each quote, list or description
level indents the text column; nesting deep enough to leave it under 72 pt is a
kind `"limit"` error rather than text drawn past the page edge. Table columns
are sized to their widest words; when those do not fit, every column keeps at
least 12 pt of text width (plus its padding) and a longer word overflows its
cell, and a table with too many columns for even that is a `"limit"` error.

**Backgrounds.** `tableHeaderFill` (a `Fill`) shades a table's header row, and
`headingStyles` (a `list of HeadingStyle`, index 0 = h1, 1 = h2, ...) shades each
heading level. Build a colour with `doctree.gray(level)` / `doctree.rgb(r, g, b)`
(or `doctree.noFill()` for none) and a heading style with
`doctree.headingStyle(fill)`:

```jennifer
def o as doctree.PdfOptions init doctree.pdfDefaults();
$o.tableHeaderFill = doctree.gray(232);
$o.headingStyles = [doctree.headingStyle(doctree.gray(205)), doctree.headingStyle(doctree.gray(225))];
def out as bytes init markdown.toPdfWith($md, $o);
```

**Document metadata + bookmarks.** Set `title` / `author` / `subject` /
`keywords` on `PdfOptions` to write the PDF Info dictionary (shown in a viewer's
Document Properties). Set `bookmarkLevel` to add a navigation outline: every
heading up to that level becomes a bookmark, nested by heading level:
`bookmarkLevel = 2` bookmarks all `#` and `##` headings, `1` only `#`, `0` (the
default) none.

```jennifer
def o as doctree.PdfOptions init doctree.pdfDefaults();
$o.title = "My Report";
$o.author = "Ada Lovelace";
$o.bookmarkLevel = 2;
def out as bytes init markdown.toPdfWith($md, $o);
```

**Block panels + un-encodable text.** `codeFill` / `codeBorder` (each a `Fill`,
off by default) draw a background and / or border behind every code block, and
`quoteFill` / `quoteRule` do the same behind a blockquote (`quoteRule` is the
vertical bar down its left edge), the way `tableHeaderFill` shades a table
header. A long code line or an unbreakable table token is folded to the column
instead of running off the page. A character the role's font cannot draw (outside
WinAnsi for a standard-14 font, missing from every embedded face of the role) is
replaced by `unencodable` (default `"?"`, `""` drops it) so one stray glyph
never fails the whole document; `layoutPdf` reports each one. The substitution is for what is *drawn*: a
bookmark is read by the viewer rather than set in a font, so it keeps the
heading whole in every script, and the Info dictionary keeps its values whole
too.

**More document metadata.** Alongside `title` / `author` / `subject` /
`keywords`, set `creator` (the application that produced the document) and
`producer` on `PdfOptions`; each is written to the PDF Info dictionary when
non-empty (an empty `producer` keeps `pdf`'s own `"Jennifer pdf"` default).

**Page breaks.** A level-1 heading starts a fresh page, but a document can force
a break anywhere with `doctree.pageBreak()`, a node placed in a tree handed to
`doctree.renderPdf`, or, in Markdown source, a lone `<!-- pagebreak -->` comment.
`toHtml` / `toAnsi` ignore it.

```jennifer
def o as doctree.PdfOptions init doctree.pdfDefaults();
$o.codeFill = doctree.gray(240);
$o.codeBorder = doctree.gray(150);
$o.quoteFill = doctree.gray(244);
$o.quoteRule = doctree.gray(150);
$o.creator = "Grimoire";
def out as bytes init markdown.toPdfWith($md, $o);
```

**Images.** By default `![alt](url)` renders as its `[alt]` text in the PDF (the
same fallback `toAnsi` uses), so nothing is lost when no picture is supplied. To
draw the actual picture, load its bytes with [`pdf.loadImage`](pdf.md) and put it
in `PdfOptions.images`, a `map of string to pdf.Image` keyed by the image URL
exactly as it appears in the node (the URL as written in the source). `doctree`
never touches the filesystem: the caller loads the bytes (and so resolves relative paths).

```jennifer
import "markdown.j" as markdown;
import "doctree.j" as doctree;
import "pdf.j" as pdf;
use fs;
def o as doctree.PdfOptions init doctree.pdfDefaults();
$o.images["figure.png"] = pdf.loadImage("Figure", fs.readBytes("figure.png"));
def out as bytes init markdown.toPdfWith("# Report\n\n![a diagram](figure.png)\n", $o);
```

Only a paragraph that is a single image (a picture on its own line) is drawn;
an image inline in a sentence keeps its `[alt]` run, and an image in a list item,
quote, or table cell keeps the fallback too. A drawn picture is scaled to keep its
aspect ratio, never wider than the text column and never taller than one page;
`imageDpi` (default 96) sets the pixel-to-point scale, so a screenshot wider than
the column shrinks to fit. Each image's resource `name` (the first `loadImage`
argument) must be unique across the map; a duplicate is a kind `"doctree"`
error. Registration order follows the map's insertion order (a language
guarantee), so output stays byte-identical run to run.

**Running headers / footers (page numbers).** `toPdf` / `renderPdf` return
`bytes`, but `pdf`'s running header / footer take a `pdf.Document`. Use
`doctree.renderPdfDoc` to get the laid-out document, attach the footer with
[`pdf.setFooter`](pdf.md) (its `%page%` / `%pages%` placeholders fill in per page,
which needs the total, known only once the whole book is laid out), then
serialize with `pdf.render`:

```jennifer
import "markdown.j" as markdown;
import "doctree.j" as doctree;
import "pdf.j" as pdf;
def doc as pdf.Document init doctree.renderPdfDoc(markdown.parse($md), doctree.pdfDefaults());
def lbl as pdf.PageLabel init pdf.pageLabel();
$lbl.right = "%page%/%pages%";
def out as bytes init pdf.render(pdf.setFooter($doc, $lbl));
```

### Fonts: standard-14 and embedded TrueType

Each of the five font roles (body, bold, italic, mono, heading) draws in one of
three ways:

| The role has | It draws | It measures with |
| ------------ | -------- | ---------------- |
| nothing set (the default) | its standard-14 font (`bodyFont`, ...), WinAnsi only; output is byte-identical to a render without the face fields | the Adobe AFM metrics |
| a face (`bodyFace`, `boldFace`, `italicFace`, `monoFace`, `headingFace`) | that face, then each face of its fallback list for a character the face lacks | each face's own advance widths |
| only fallbacks (`bodyFallbacks`, ...) | its standard-14 font for what WinAnsi covers, the fallbacks for the rest | the AFM metrics and the fallbacks' advances |

A face is a `pdf.LoadedFont` from `pdf.loadFont(name, ttfBytes)`: a TrueType
(`glyf`) font, which `pdf` embeds whole as a Type0 font with a ToUnicode map, so
the text stays selectable. `doctree` ships no font; the caller reads the
`.ttf` bytes. The tested choice is DejaVu (Sans, Sans Bold, Sans Oblique, Sans
Mono), which covers Latin, Greek, Cyrillic, arrows and many symbols but no CJK;
add a CJK TrueType face as a fallback for that. Line breaking, code folding and
table columns use the drawing font's widths, so they stay exact, and in a face a
run of CJK ideographs may break between any two of them. The same face may serve
several roles (it is embedded once), and only faces some page drew from are
embedded. Each face needs its own resource name, not a standard-14 font's
(`Helvetica`, `Courier`, `Symbol`, `ZapfDingbats`); two different fonts under
one name are a kind `"doctree"` error.

```jennifer
import "markdown.j" as markdown;
import "doctree.j" as doctree;
import "pdf.j" as pdf;
use fs;
use io;
def o as doctree.PdfOptions init doctree.pdfDefaults();
$o.bodyFace = pdf.loadFont("Sans", fs.readBytes("/usr/share/fonts/TTF/DejaVuSans.ttf"));
$o.boldFace = pdf.loadFont("SansBold", fs.readBytes("/usr/share/fonts/TTF/DejaVuSans-Bold.ttf"));
$o.italicFace = pdf.loadFont("SansIt", fs.readBytes("/usr/share/fonts/TTF/DejaVuSans-Oblique.ttf"));
$o.monoFace = pdf.loadFont("Mono", fs.readBytes("/usr/share/fonts/TTF/DejaVuSansMono.ttf"));
$o.headingFace = $o.boldFace;
def res as doctree.PdfResult init doctree.layoutPdf(markdown.parse($md), $o);
for (def m in $res.missing) {
    io.printf("page %d: cannot draw %s in '%s'\n", $m.pageNr, $m.char, $m.context);
}
fs.writeBytes("book.pdf", pdf.render($res.doc));
```

**What could not be drawn.** A character no font of its role has is drawn as
`unencodable` (default `"?"`; only the part of it the role can draw) and
reported, never dropped silently. `layoutPdf` returns a `PdfResult`: `doc`, the
`pdf.Document`, and `missing`, one `MissingGlyph` per such character in layout
order:

| Field | Meaning |
| ----- | ------- |
| `char` / `codepoint` | the character and its code point |
| `role` | `"body"`, `"bold"`, `"italic"`, `"mono"` or `"heading"` |
| `pageNr` | the 1-based page it falls on |
| `y` | the baseline of its line (a table row's first line), in points from the page bottom |
| `context` | the word, code line, heading or table cell it sits in, as written |

With `unencodable = ""` a word made only of such characters is dropped from the
line but still reported: its `context` joins it to the next word drawn in the
paragraph (else the last one), and `pageNr` / `y` are that word's line; a
paragraph with nothing drawable keeps one empty line for its report.

The report is a list in the result rather than a callback in `PdfOptions`: the
options stay plain data, the caller decides whether a missing glyph is a warning
or a failure (and can group the entries by page), and no function value is
called from inside the layout, where it would make every layout call copy its
arguments for a program with a mutable top-level variable. `renderPdf` and
`renderPdfDoc` lay out the same way and drop the list. Bookmarks and the
metadata fields are PDF text strings, written as UTF-16 when not ASCII, so they
keep every script whatever font the page uses.

## See also

- [markdown.md](markdown.md) - the Markdown reader (and `toHtml` / `toAnsi` /
  `toPdf` shortcuts from Markdown text).
- [rst.md](rst.md) - the reStructuredText reader.
- [html.md](html.md), [ansi.md](ansi.md), [pdf.md](pdf.md) - the backends.
- [modules/index.md](index.md) - the module catalog and import rules.

## Academy talks

| Talk | What it shows |
| ---- | ------------- |
| [Inside a document pipeline: doctree, MyST, reST and fonts](https://academy.jennifer-lang.dev/talks/mplx/doctree/) | the document tree, selectors and transforms |

## API reference

Every exported function, struct and constant of `doctree`, with its signature, generated
from the module's doc comments: [the `doctree` API reference](../api/doctree.md).
