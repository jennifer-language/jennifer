# `markdown` - render a Markdown subset to HTML and ANSI

Import with `import "markdown.j" as markdown;`. Renders a small CommonMark
subset to **HTML** (through the [`html`](html.md) module, so
escaping is handled for you) and to **styled terminal text** (through the
[`ansi`](ansi.md) module). Pure Jennifer: line-oriented block parsing with a
small inline scanner. Runs on either binary.

```jennifer
use io;
import "markdown.j" as markdown;

io.printf("%s\n", markdown.toHtml("# Hi\n\nA **bold** word."));
# <h1>Hi</h1><p>A <strong>bold</strong> word.</p>

io.printf("%s\n", markdown.toAnsi("- one\n- two"));   # styled on a TTY
```

Runnable: [`examples/modules/markdown_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/markdown_demo.j).

## Surface

Rendering (Markdown in, HTML / terminal text / PDF out):

| Call                  | Returns  | Notes                                                            |
| --------------------- | -------- | --------------------------------------------------------------- |
| `markdown.toHtml(md)` | `string` | Render to HTML: block elements concatenated, no indentation.    |
| `markdown.toAnsi(md)` | `string` | Render to terminal text with `ansi` styling (self-suppressing). |
| `markdown.render(doc, format)` | `string` | Render a parsed (or hand-built) tree; `format` is `"html"` / `"ansi"`. |
| `markdown.toPdf(md)`  | `bytes`  | Lay the document out to a paginated PDF (through `pdf`).         |
| `markdown.toPdfWith(md, opts)` | `bytes` | `toPdf` with a custom `PdfOptions` (page size, margins, fonts, document metadata, bookmarks). |
| `markdown.renderPdf(doc, opts)` | `bytes` | Lay a parsed (or transformed) tree out to a PDF.               |
| `markdown.pageBreak()`        | `Node`   | A page-break node; `renderPdf` starts the next content on a fresh page. |

`toHtml` / `toAnsi` are thin wrappers over `render(parse(md), ...)`, and `toPdf` over
`renderPdf(parse(md), pdfDefaults())`, so build and parse share one document model.

Reading (surface the parse tree, walked like [`xml`](xml.md) / [`html`](html.md)):

| Call                          | Returns        | Notes                                                       |
| ----------------------------- | -------------- | ----------------------------------------------------------- |
| `markdown.parse(md)`          | `Node`         | Parse to the document root (`typeOf` `"document"`).         |
| `markdown.typeOf(node)`       | `string`       | The node kind (`"heading"`, `"link"`, `"text"`, ...).       |
| `markdown.children(node)`     | `list of Node` | The node's direct children.                                 |
| `markdown.text(node)`         | `string`       | A leaf's text, else its descendants' text concatenated.     |
| `markdown.level(node)`        | `int`          | A heading's level (1-6); 0 otherwise.                       |
| `markdown.attr(node, name)`   | `string`       | `"href"` / `"title"` / `"lang"` / `"align"` / `"ordered"` / `"level"`, or "". |
| `markdown.get(node, sel)`     | `Node`         | First node matching a `/`-separated selector, or an empty node. |
| `markdown.findAll(node, sel)` | `list of Node` | Every node matching the selector.                           |
| `markdown.has(node, sel)`     | `bool`         | Whether any node matches.                                   |

Authoring (build Markdown text - the inverse):

| Call                         | Returns  | Notes                                                     |
| ---------------------------- | -------- | --------------------------------------------------------- |
| `markdown.header(level, s)`  | `string` | ATX heading; `level` is `"h1"`..`"h6"` (throws otherwise). |
| `markdown.style(kind, s)`    | `string` | Inline emphasis; `kind` is `"bold"` / `"italic"` / `"code"`. |
| `markdown.link(text, url)`   | `string` | `[text](url)`.                                            |
| `markdown.bullets(items)`    | `string` | Unordered list, one `- item` per line.                    |
| `markdown.numbered(items)`   | `string` | Ordered list, `1. item` upward.                           |
| `markdown.codeBlock(text)`   | `string` | Fenced code block around verbatim text.                   |
| `markdown.table(headings, aligns, rows)` | `string` | GFM table from column headings, per-column alignment, and rows. |
| `markdown.tablePretty(md)`   | `string` | Reformat every table's source columns to line up; other lines untouched. |

## Supported Markdown

A deliberately small [CommonMark](https://commonmark.org) subset:

| Block                | Syntax                          | HTML                        |
| -------------------- | ------------------------------- | --------------------------- |
| Heading (levels 1-6) | `# H` ... `###### H`            | `<h1>` ... `<h6>`           |
| Paragraph            | consecutive text lines          | `<p>` (lines joined by ` `) |
| Unordered list       | `- x` / `* x` / `+ x`           | `<ul><li>`                  |
| Ordered list         | `1. x`                          | `<ol><li>`                  |
| Nested list          | indent a sub-list under an item | a child `<ul>` / `<ol>` inside the parent `<li>` |
| Blockquote           | `> x` (recursive: `> > y`)      | `<blockquote>` (inner text parsed as blocks) |
| Fenced code block    | ` ``` ` ... ` ``` `             | `<pre><code>`               |
| Indented code block  | four-space indent after a blank line | `<pre><code>`          |
| Thematic break       | `---` / `***` / `___` (3+, spaced ok) | `<hr>`                 |
| Raw HTML block       | a line opening with `<tag` / `</` / `<!--` to the next blank line | passed through verbatim |
| Table (GFM)          | `\| a \| b \|` + `\| --- \| --- \|` row | `<table>` (aligned terminal columns in ANSI) |

| Inline    | Syntax          | HTML                  | ANSI            |
| --------- | --------------- | --------------------- | --------------- |
| Bold      | `**text**`      | `<strong>`            | bold            |
| Italic    | `*text*`        | `<em>`                | italic          |
| Code      | `` `text` ``    | `<code>`              | cyan            |
| Link      | `[text](url)`   | `<a href="url">`      | underline + ` (url)` |
| Autolink  | `<https://x>` / `<a@b.com>` | `<a href="...">` (email gets `mailto:`) | underline + ` (url)` |
| Image     | `![alt](url)`   | `<img src alt>`       | `[image] alt (url)`  |

**Inline spans nest.** A link inside `**...**`, a `` `code` `` span or
`*emphasis*` inside a link label, and so on all parse to a nested tree, so
`**[label](x)**` renders `<strong><a href="x">label</a></strong>` rather than
dropping the link. A link label may contain a `]` inside a code span or a
balanced nested `[...]` pair (`[the `$xs[]` sugar](x)` parses correctly).

A **lazy continuation** line - a soft-wrapped line indented to an item's content
column - stays part of that item rather than splitting the list. A blank line
still ends the item. A nested list is a more-indented list under a parent item;
a blockquote's inner lines are parsed as blocks, so a quote can hold paragraphs,
lists, or nested quotes. A link's `href`, an autolink's target, and an image's
`src` all pass through the same scheme allowlist ([`html.safeUrl`](html.md)), so
`javascript:` and other script schemes render as `#`. A raw HTML block is emitted
verbatim by `toHtml` (it is the author's own markup) and dropped by `toAnsi` /
the PDF renderer, which have no HTML.

## HTML output

`toHtml` builds an [`html`](html.md) node tree and renders it, so
all text and every link target are correctly escaped - `&`, `<`, `>` in text
and code, and `&`/`"` in an `href` - and you cannot produce malformed markup:

```jennifer
markdown.toHtml("[t](http://x/?a=1&b=2) and <b> & `x<y`");
# <p><a href="http://x/?a=1&amp;b=2">t</a> and &lt;b&gt; &amp; <code>x&lt;y</code></p>
```

Output is compact (no newlines between block elements), which diffs and
round-trips predictably; wrap it in your own formatter if you need indented
source. A code block's content is escaped but never treated as Markdown.

## ANSI output

`toAnsi` renders for a terminal: headings and `**bold**` in bold, `*italic*`
in italic, inline code in cyan, links underlined with their URL in
parentheses, list items with `- ` / `N. ` markers, and fenced code indented
and dimmed. Styling comes from the `ansi` module, which **suppresses itself
when stdout is not a terminal** (or `NO_COLOR` is set) and is forced on by
`FORCE_COLOR` - so piping the output gives clean plain text, and
`ansi.strip(markdown.toAnsi(md))` gives it unconditionally.

## Reading the document tree

`markdown.parse(md)` returns the document as a tree of `Node`s, walked with the
same accessor vocabulary as [`xml`](xml.md) / [`html`](html.md) - so a document
can be inspected or transformed (pull the headings for a table of contents,
rewrite links, lint) and then rendered, rather than going straight to a string. A
node's `typeOf` is a block kind (`"document"`, `"heading"`, `"paragraph"`,
`"code"`, `"list"`, `"item"`, `"table"`, `"row"`, `"cell"`, `"quote"`) or an
inline kind (`"text"`, `"codespan"`, `"strong"`, `"emphasis"`, `"link"`,
`"image"`). `get` / `findAll` / `has` take a `/`-separated selector whose steps
are kind names, `*` (any kind), or `name[k]` (the k-th such child, 1-based),
matching direct children.

```jennifer
def doc as markdown.Node init markdown.parse("# Title\n\nSee [docs](https://example.com).\n");

# Walk the headings for an outline.
for (def h in markdown.findAll($doc, "heading")) {
    io.printf("H{markdown.level($h)}: {markdown.text($h)}\n");   # H1: Title
}

# Pull a link's target.
def a as markdown.Node init markdown.get($doc, "paragraph/link");
io.printf("{markdown.text($a)} -> {markdown.attr($a, 'href')}\n");   # docs -> https://example.com

# Render straight from the (possibly transformed) tree.
io.printf("{markdown.render($doc, 'html')}\n");
```

`attr` reads a node's string attributes: `"href"` / `"title"` (a link or image),
`"lang"` (a fenced code block's language), `"align"` (a table cell), `"ordered"`
(`"true"` / `"false"`, a list), and `"level"` (a heading). `render(doc, format)`
renders a `"document"` node's children (or any single node as one block); it and
the parse are hardened with a nesting-depth cap and a total-node budget, so a
pathological document is a catchable `"markdown"` error rather than an unbounded
parse.

## Authoring Markdown

The authoring helpers are the inverse of the renderer: they build Markdown
*text*, so a program can assemble a document (and, since it is Markdown,
round-trip it through `toHtml` / `toAnsi`):

```jennifer
use io;
import "markdown.j" as markdown;

def items as list of string init ["fast", "small", "strict"];
def doc as string init markdown.header("h1", "Jennifer") + "\n\n";
$doc = $doc + "It is " + markdown.style("bold", "great") + ". Features:\n\n";
$doc = $doc + markdown.bullets($items) + "\n\n";
$doc = $doc + "See " + markdown.link("the docs", "https://example/docs") + ".";

io.printf("%s\n", $doc);                  # Markdown source
io.printf("%s\n", markdown.toHtml($doc)); # ... or rendered
```

The text is inserted literally: a caller passing Markdown metacharacters
(a `*` or `` ` `` inside a heading, say) is responsible for escaping them.
`header` throws a catchable `value` error on a level outside `"h1"`..`"h6"`,
and `style` on a `kind` other than `"bold"` / `"italic"` / `"code"`.

### Tables

`table` turns tabular data into a [GFM](https://github.github.com/gfm/)
table in one call: column `headings`, per-column `aligns` (`"left"` /
`"right"` / `"center"` / `"none"`, or `[]` for all-default), and `rows` (each
a `list of string`):

```jennifer
def rows as list of list of string init [];
$rows[] = ["Ada", "95"];
$rows[] = ["Bo", "88"];
io.printf("%s\n", markdown.table(["Name", "Score"], ["left", "right"], $rows));
# | Name | Score |
# | :--- | ---: |
# | Ada | 95 |
# | Bo | 88 |
```

Columns follow `headings`: a short row is padded with empty cells and extra
cells are dropped, so every row is the same width. A `|` in a cell is escaped
to `\|` and a newline becomes a space, so cell content can't break the table.
An `align` value outside the four names throws a catchable `value` error.

The reader understands GFM tables too, so an authored table round-trips:
`toHtml(markdown.table(...))` renders a `<table>` (with per-column `align`),
and `toAnsi` renders aligned terminal columns. A parsed table needs a header
row, a delimiter row (`| --- | :--: |`), and its data rows; cell content is
inline-parsed (emphasis / code / links work in cells), and a table interrupts
an open paragraph.

`tablePretty` reformats the **source** of every table in a document so its
columns line up - the handcraft-then-prettify workflow, in one call - and
leaves every non-table line exactly as written:

```jennifer
def messy as string init "| Name | Score |\n|:-|-:|\n| Ada | 95 |";
io.printf("%s\n", markdown.tablePretty($messy));
# | Name | Score |
# | :--- | ----: |
# | Ada  |    95 |
```

Each column is padded to its widest cell (minimum three, so the delimiter
keeps its dashes), data cells follow the column's alignment, and an escaped
`\|` is preserved. It is idempotent: prettifying an already-pretty table is a
no-op.

## Not supported

This is a subset, chosen to stay small and TinyGo-clean:

- No reference links (`[text][ref]`), setext (underlined) headings, or
  hard-break-on-two-spaces.
- Autolinks cover `<scheme:...>` and `<user@host>`; bare (unbracketed) URLs in
  prose are not linkified.
- The raw-HTML block rule is CommonMark's "type 6" shape (a tag at the line
  start, to the next blank line); inline raw HTML in a paragraph is still escaped.

For anything beyond this subset, render with an external tool. The module is
sized for READMEs, help text, and comment / docblock bodies, not
general-purpose CommonMark conformance.

## Note

> `toPdf` folds PDF rendering into `markdown` as a third output beside
> `toHtml` / `toAnsi`, so it imports [`pdf`](pdf.md) (which pulls in `font`). That
> makes every `markdown` import a bit heavier even for a `toHtml`-only program - a
> deliberate trade for one unified rendering surface; see
> [design-decisions.md](../technical/design-decisions.md). Text outside the WinAnsi
> range the standard-14 fonts encode is replaced per the `unencodable` option
> (below) rather than aborting the render. `PdfOptions` comes from `markdown.pdfDefaults()`
> (US Letter, 54-pt margins, Helvetica / Courier); copy it and tweak the fields
> (`pageWidth` / `pageHeight` / `margin` / `bodyFont` / `boldFont` / `italicFont` /
> `monoFont` / `headingFont` / `bodySize` / `tablePad`). A level-1 heading starts a
> new page; table columns are sized to their content (a short column doesn't crowd a
> long one), and `tablePad` tunes cell density.
>
> **Backgrounds.** `tableHeaderFill` (a `Fill`) shades a table's header row, and
> `headingStyles` (a `list of HeadingStyle`, index 0 = h1, 1 = h2, ...) shades each
> heading level. Build a colour with `markdown.gray(level)` / `markdown.rgb(r, g, b)`
> (or `markdown.noFill()` for none) and a heading style with
> `markdown.headingStyle(fill)`:
>
> ```jennifer
> def o as markdown.PdfOptions init markdown.pdfDefaults();
> $o.tableHeaderFill = markdown.gray(232);
> $o.headingStyles = [markdown.headingStyle(markdown.gray(205)), markdown.headingStyle(markdown.gray(225))];
> def out as bytes init markdown.toPdfWith($md, $o);
> ```
>
> **Document metadata + bookmarks.** Set `title` / `author` / `subject` /
> `keywords` on `PdfOptions` to write the PDF Info dictionary (shown in a viewer's
> Document Properties). Set `bookmarkLevel` to add a navigation outline: every
> heading up to that level becomes a bookmark, nested by heading level -
> `bookmarkLevel = 2` bookmarks all `#` and `##` headings, `1` only `#`, `0` (the
> default) none.
>
> ```jennifer
> def o as markdown.PdfOptions init markdown.pdfDefaults();
> $o.title = "My Report";
> $o.author = "Ada Lovelace";
> $o.bookmarkLevel = 2;
> def out as bytes init markdown.toPdfWith($md, $o);
> ```
>
> **Block panels + un-encodable text.** `codeFill` / `codeBorder` (each a `Fill`,
> off by default) draw a background and / or border behind every code block, and
> `quoteFill` / `quoteRule` do the same behind a blockquote (`quoteRule` is the
> vertical bar down its left edge) - the way `tableHeaderFill` shades a table
> header. A long code line or an unbreakable table token is folded to the column
> instead of running off the page. A character the standard-14 fonts cannot encode
> is replaced by `unencodable` (default `"?"`, `""` drops it) so one stray glyph
> never fails the whole document.
>
> **More document metadata.** Alongside `title` / `author` / `subject` /
> `keywords`, set `creator` (the application that produced the document) and
> `producer` on `PdfOptions`; each is written to the PDF Info dictionary when
> non-empty (an empty `producer` keeps `pdf`'s own `"Jennifer pdf"` default).
>
> **Page breaks.** A level-1 heading starts a fresh page, but a document can force
> a break anywhere with `markdown.pageBreak()` - a node placed in a tree handed to
> `markdown.renderPdf`, or, in Markdown source, a lone `<!-- pagebreak -->` comment.
> `toHtml` / `toAnsi` ignore it.
>
> ```jennifer
> def o as markdown.PdfOptions init markdown.pdfDefaults();
> $o.codeFill = markdown.gray(240);
> $o.codeBorder = markdown.gray(150);
> $o.quoteFill = markdown.gray(244);
> $o.quoteRule = markdown.gray(150);
> $o.creator = "Grimoire";
> def out as bytes init markdown.toPdfWith($md, $o);
> ```

## See also

- [html.md](html.md) - the HTML backend `toHtml` renders through.
- [ansi.md](ansi.md) - the terminal styling `toAnsi` renders through.
- [modules/index.md](index.md) - the module catalog and import rules.
