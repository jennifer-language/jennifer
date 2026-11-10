# `markdown` - read and render CommonMark

Import with `import "markdown.j" as markdown;`. Reads
[CommonMark](https://commonmark.org) Markdown - plus the GitHub extensions
(tables, task lists, strikethrough, footnotes, alerts) and a few others - into
a [`doctree`](doctree.md) document, and renders it to HTML, XHTML, styled
terminal text or PDF through doctree's renderers. It also writes Markdown text
(the authoring helpers). Pure Jennifer, using CommonMark's own block and inline
algorithms. Runs on either binary.

```jennifer
use io;
import "markdown.j" as markdown;

io.printf("%s\n", markdown.toHtml("# Hi\n\nA **bold** word."));
# <h1>Hi</h1><p>A <strong>bold</strong> word.</p>

io.printf("%s\n", markdown.toAnsi("- one\n- two"));   # styled on a TTY
```

Runnable: [`examples/modules/markdown_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/markdown_demo.j).

## Surface

Reading and rendering (Markdown text in):

| Call                  | Returns  | Notes                                                            |
| --------------------- | -------- | --------------------------------------------------------------- |
| `markdown.parse(md)`  | `doctree.Node` | Parse to the document root (`typeOf` `"document"`); walk, transform and render it with [`doctree`](doctree.md). |
| `markdown.toHtml(md)` | `string` | Render to HTML: block elements concatenated, no indentation. Safe by default: raw HTML in the source is escaped. |
| `markdown.toHtmlWith(md, opts)` | `string` | `toHtml` with an `HtmlOptions`; set `allowRawHtml: true` to pass raw HTML blocks through (trusted input only). |
| `markdown.toXhtml(md)` | `string` | Render to well-formed XHTML (void elements self-close, boolean attributes expand) - an XHTML / EPUB content document. Raw HTML is escaped, as `toHtml`. |
| `markdown.toAnsi(md)` | `string` | Render to terminal text with `ansi` styling (self-suppressing). |
| `markdown.toPdf(md)`  | `bytes`  | Lay the document out to a paginated PDF.                         |
| `markdown.toPdfWith(md, opts)` | `bytes` | `toPdf` with a `doctree.PdfOptions` (page size, margins, fonts, metadata, bookmarks, images). |

The `to*` calls are shortcuts: `toHtml(md)` is `doctree.render("html",
markdown.parse(md))`, `toPdfWith(md, opts)` is `doctree.renderPdf(markdown.parse(md), opts)`.
Reach for `parse` and the [`doctree`](doctree.md) calls to inspect or transform a
document before rendering it, or for a running header / footer
(`doctree.renderPdfDoc`).

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

**CommonMark 0.31.2.** The parser passes 644 of the specification's 652
examples (98.8%, compared structurally with the reference output). The eight it
does not are deliberate: five autolinks or links to a URL scheme outside the
[`html.safeUrl`](html.md) allowlist (`irc:`, `localhost:`, a made-up scheme)
render with `href="#"`; two put soft line breaks inside a raw `<pre>`, where a
soft break here is a space; one uses HTML5-only entity names (`&HilbertSpace;`).

Blocks:

| Block | Syntax | Node |
| --- | --- | --- |
| Heading | `# H` ... `###### H` (closing `#`s allowed), or text underlined with `===` / `---` | `heading` |
| Paragraph | text lines; a line may continue it lazily inside a quote or item | `paragraph` |
| Block quote | `> ...`, holding any blocks | `quote` |
| List | `-` / `*` / `+`, `1.` / `1)` (the first number is kept as `attr "start"`); items hold any blocks | `list` / `item` |
| Code | fenced with ```` ``` ```` or `~~~` (the info string's first word is `lang`, the whole of it `attr "info"`), or indented four spaces | `code` |
| Thematic break | `---`, `***`, `___` (spaces allowed) | `thematic_break` |
| HTML block | the seven CommonMark kinds (`<div>`, `<!-- -->`, `<script>`, ...) | `html_block` |
| Link reference definition | `[label]: url "title"` anywhere in the document | (used by links) |

A list is **tight** unless blank lines separate its items or the blocks inside
one: a tight list's items hold their text directly (`<li>a</li>`), a loose
one's hold paragraphs (`<li><p>a</p></li>`), as in CommonMark.

Inlines:

| Inline | Syntax | Node | ANSI |
| --- | --- | --- | --- |
| Emphasis / strong | `*a*` / `_a_`, `**a**` / `__a__` (CommonMark's delimiter rules) | `emphasis` / `strong` | italic / bold |
| Code span | `` `a` ``, ``` `` a`b `` ``` | `codespan` | cyan |
| Link | `[text](url "title")`, `[text][label]`, `[text][]`, `[label]` | `link` | underline + ` (url)` |
| Autolink | `<https://x>`, `<a@b.com>` (`mailto:` added) | `link` | underline + ` (url)` |
| Image | `![alt](url "title")` and the reference forms | `image` | `[image] alt (url)` |
| Hard break | two spaces or a `\` at the end of a line | `line_break` | a newline |
| Raw HTML | `<span>`, `<!-- -->`, ... | `html_inline` | as written |
| Escapes and entities | `\*`, `&amp;`, `&ouml;`, `&#35;` | `text` | - |

A soft line break (a plain newline in a paragraph) becomes a space. Named
entities are the HTML 4 set (Latin-1, Greek, math and typographic names) plus
`&apos;`; an unknown name stays as written.

Extensions:

| Extension | Syntax | Node |
| --- | --- | --- |
| Table (GFM) | `\| a \| b \|` over a `\| --- \| :-: \|` delimiter row | `table` / `row` / `cell` |
| Task list (GFM) | `- [ ] todo` / `- [x] done` | `item` with `task` / `checked` attrs |
| Strikethrough (GFM) | `~~text~~` | `strikethrough` |
| Footnote (GFM) | `text[^1]` and a `[^1]: note` definition (continued four spaces in) | `footnote_ref`, `footnote` |
| Alert (GitHub) | `> [!NOTE]` opening a quote | `admonition` (below) |
| Highlight | `==text==` | `highlight` |
| Sub / superscript | `~x~` / `^x^` (no spaces inside) | `subscript` / `superscript` |
| Definition list | a term line, then `: definition` lines (continued indented) | `definition_list` / `def_term` / `def_desc` |
| Attribute list | `## H {#id .class}`, `[t](u){.button target="_blank"}`, `![a](u){width="20"}` | `attrs` |

**Footnotes** are numbered in the order they are first referenced; a second
reference to the same note gets an id of its own (`fnref-1-2`), and every note
links back to its first reference. The referenced notes are appended to the
document in that order, as `footnote` nodes; a note never referenced is
dropped, and a reference to a note that does not exist stays as text.

**Attribute lists.** A `{#id .class key="value"}` trailing a heading, or
immediately after a link or image, sets that node's attributes - a stable
heading anchor (`## Title {#my-anchor}` -> `<h2 id="my-anchor">`), extra
classes, a `target="_blank"` link, an image `width`. HTML/XHTML emit them;
ANSI/PDF ignore them. In a `.j` string literal a cooked `"...{...}"` treats
`{...}` as interpolation, so write Markdown that contains attribute lists (or
any literal brace) as a raw `'...'` string, or read it from a file.

A link's `href`, an autolink's target, and an image's `src` all pass through the
same scheme allowlist ([`html.safeUrl`](html.md)), so `javascript:` and other
script schemes render as `#`.

### Admonitions

A blockquote that opens with an alert marker is an `admonition` node rather than
a `quote`:

```markdown
> [!NOTE]
> Useful to know, and not in the way of the sentence you were reading.

> [!WARNING] Mind the gap
> A marker can carry its own title.
```

Five kinds - `NOTE`, `TIP`, `IMPORTANT`, `WARNING`, `CAUTION` - spelled in any
case. `attr(node, "kind")` is the kind in lower case, `attr(node, "title")` the
custom title or `""`, and the marker itself is gone: it is structure rather than
content, so `text(node)` is the body alone. Everything else the quotation holds -
paragraphs, lists, code, a nested admonition - is its children.

A sixth word, or a marker that is not the whole of what precedes the title, is a
quotation: `> [!NOTES]` and `> [!NOTE]: as it happens` both parse as `quote`.
A renderer unaware of the syntax also reads every admonition as a quote, which is
why this syntax was chosen over `:::note`: it degrades to plain Markdown.

The title is plain text, not inline Markdown, and only the parser can see it: by
the time the block exists the line break after the marker has been folded into a
space, and `> [!NOTE] Mind the gap` is then indistinguishable from a marker with
a body on the next line.

`toHtml` writes the conventional class names, so a stylesheet written for another
generator's callouts already matches (this module ships none). `toAnsi` prints
the label in bold above the quotation. In a PDF the label is a bold paragraph at
the head of the panel; `PdfOptions.admonitionLabels` localizes the labels:

```jennifer
$o.admonitionLabels = {"note": "Hinweis", "warning": "Warnung"};
```

A kind missing from the map uses its own name; a custom title overrides both.

## HTML output and safety

**Safe by default for untrusted Markdown.** Raw HTML in the source - a block
(`<div>...`) or inline (`a <span>b</span>`) - is escaped, so rendering a README,
a comment, or any publisher-supplied Markdown cannot inject a `<script>` or an
event-handler attribute (`<img onerror=...>`, a stored-XSS vector); it renders
as visible text. Everything the renderer builds (text, link URLs, image
attributes) is escaped too - see [doctree's HTML output](doctree.md#html-output-and-safety).

For trusted Markdown where raw HTML is intended (your own docs using
`<details>`, `<div align>`, `<kbd>`, badges), opt back in to verbatim
passthrough of both kinds:

```jennifer
def opts as markdown.HtmlOptions;
$opts.allowRawHtml = true;
markdown.toHtmlWith($trustedMd, $opts);
```

`allowRawHtml` defaults to false, so `toHtml(md)` and `toHtmlWith` with the
zero options are identical. Markup inside a raw tag is never parsed:
`<a title="*x*">` keeps its asterisks either way.

## Reading the document tree

`markdown.parse(md)` returns the document as a [`doctree.Node`](doctree.md) tree,
so it can be inspected or transformed (pull the headings for a table of contents,
rewrite links, lint) and then rendered:

```jennifer
import "markdown.j" as markdown;
import "doctree.j" as doctree;

def doc as doctree.Node init markdown.parse("# Title\n\nSee [docs](https://example.com).\n");
for (def h in doctree.findAll($doc, "heading")) {
    io.printf("H{doctree.level($h)}: {doctree.text($h)}\n");   # H1: Title
}
io.printf("{doctree.render('html', $doc)}\n");
```

Markdown builds the block kinds `"document"`, `"heading"`, `"paragraph"`,
`"code"`, `"list"`, `"item"`, `"table"`, `"row"`, `"cell"`, `"quote"`,
`"admonition"`, `"definition_list"`, `"def_term"`, `"def_desc"`,
`"thematic_break"`, `"html_block"`, `"page_break"`, and the inline kinds `"text"`,
`"codespan"`, `"strong"`, `"emphasis"`, `"strikethrough"`, `"highlight"`,
`"subscript"`, `"superscript"`, `"link"`, `"image"`. The parse is hardened with a
nesting-depth cap and a total-node budget, so a pathological document is a
catchable `"markdown"` error rather than an unbounded parse.

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

`tablePretty` reformats the source of every table in a document so its
columns line up, and leaves every non-table line exactly as written:

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

- URL schemes outside [`html.safeUrl`](html.md)'s allowlist, deliberately: such
  a link renders with `href="#"`.
- Destinations are not percent-encoded in the output (`[a](<b c>)` gives
  `href="b c"`).
- Bare URLs in prose are not linkified (GFM's autolink extension); write
  `<https://...>`.
- HTML5's full named-entity table: names outside HTML 4 (`&HilbertSpace;`)
  stay as written.
- A code block's text has no final newline (`<pre><code>x</code></pre>`).
- Tabs inside a line are kept as they are; only the indentation's tabs count as
  4-column stops.

## PDF output

`toPdf` / `toPdfWith` lay the document out through doctree's PDF renderer:
`PdfOptions` (from `doctree.pdfDefaults()`) sets the page, fonts, backgrounds,
metadata, bookmarks and images, and a lone `<!-- pagebreak -->` comment in the
source forces a page break. [doctree.md](doctree.md#pdf-output) has the options.

```jennifer
import "markdown.j" as markdown;
import "doctree.j" as doctree;

def o as doctree.PdfOptions init doctree.pdfDefaults();
$o.title = "My Report";
$o.bookmarkLevel = 2;
def out as bytes init markdown.toPdfWith($md, $o);
```

## See also

- [doctree.md](doctree.md) - the document tree `parse` returns, its accessors
  and renderers.
- [html.md](html.md) - the HTML backend `toHtml` renders through.
- [ansi.md](ansi.md) - the terminal styling `toAnsi` renders through.
- [modules/index.md](index.md) - the module catalog and import rules.
