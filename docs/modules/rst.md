# `rst` - read and render reStructuredText

Import with `import "rst.j" as rst;`. Reads reStructuredText (rST) into a
[`doctree`](doctree.md) document - the same tree [`markdown.parse`](markdown.md)
builds - so one set of renderers serves both formats: HTML, XHTML, styled
terminal text and PDF. Parsing is lenient: malformed markup is kept as text and
never throws; `check` reports what was wrong. Pure Jennifer, no file I/O
(nothing in a document can read from the host). Runs on either binary.

```jennifer
use io;
import "rst.j" as rst;
import "doctree.j" as doctree;

def src as string init "Guide\n=====\n\nSome *emphasis*, ``code`` and a link_.\n\n.. _link: https://example.org\n";
io.printf("%s\n", rst.toHtml($src));

def doc as doctree.Node init rst.parse($src);
io.printf("%s\n", rst.title($doc));                 # Guide
for (def d in rst.check($src)) {
    io.printf("%s line %d: %s\n", $d.severity, $d.line, $d.message);
}
```

The `doctree` import is there only because the script names `doctree.Node`.
Import aliases are not transitive: `rst` imports `doctree` for itself, but that
does not give your file the name. A script that only calls `rst.*`
(`rst.toHtml(src)`) needs just the `rst` import. Add the `doctree` one as soon
as you write `doctree.` yourself, to type a tree or to walk or render it.

## Surface

| Call | Returns | Notes |
| --- | --- | --- |
| `rst.parse(src)` | `doctree.Node` | Parse to the document root (`typeOf` `"document"`); walk, transform and render it with [`doctree`](doctree.md). |
| `rst.parseWith(src, opts)` | `doctree.Node` | `parse` with an `Options`. |
| `rst.check(src)` | `list of Diagnostic` | The problems in the document, in source order; empty when it is clean. |
| `rst.toHtml(src)` | `string` | Render to HTML. Safe by default: everything is escaped and link targets go through doctree's URL filter. |
| `rst.toHtmlWith(src, opts)` | `string` | `toHtml` with an `Options` (`allowRawHtml`, heading levels, strictness). |
| `rst.toXhtml(src)` | `string` | Well-formed XHTML (void elements self-close). |
| `rst.toAnsi(src)` | `string` | Terminal text with `ansi` styling (self-suppressing). |
| `rst.title(doc)` | `string` | The promoted document title's text, or `""`. |
| `rst.docinfo(doc)` | `map of string to string` | The bibliographic fields (`:Author:`, `:Version:`, ...) keyed by lowercased name. |

For PDF, render the tree: `doctree.renderPdf(rst.parse(src), doctree.pdfDefaults())`.

`Options` (the zero value is the default):

| Field | Type | Meaning |
| --- | --- | --- |
| `strict` | `bool` | Throw the first warning or error as an `"rst"` error instead of recovering. |
| `allowRawHtml` | `bool` | Pass raw HTML through when rendering (trusted input only). |
| `initialHeadingLevel` | `int` | The HTML level of the top heading (1-6; 0 means 1); deeper sections follow, capped at 6. |
| `noTitlePromotion` | `bool` | Keep a lone top-level title as an ordinary section heading. |

A `Diagnostic` has `severity` (`"warning"`: recovered from, likely unintended;
`"error"`: markup that could not be read as written), `line` (1-based) and
`message`.

## Supported reStructuredText

Blocks:

- **Section titles**, underlined or over- and underlined. Levels come from the
  order the adornment styles first appear, as in docutils. A lone top-level title
  is promoted to the document title (`class="title"`), and a lone subtitle under
  it; `noTitlePromotion` turns that off. Each section gets an `id` slug
  (duplicates are numbered). Nesting past six levels renders at level 6, with a
  warning.
- **Transitions** (`----`), **paragraphs**, **block quotes** (nesting) with
  `-- attribution`, **line blocks** (`| line`), **doctest blocks** (`>>>`).
- **Literal blocks** after `::` (the `Paragraph::`, `Paragraph ::` and lone `::`
  forms), indented or quoted.
- **Lists**: bullet (`-`, `*`, `+`, ...); enumerated - arabic, alphabetic and
  roman, in the `1.` / `1)` / `(1)` forms, with a start value and auto-numbered
  `#.`; definition lists with `: classifiers`; field lists; option lists.
- **Docinfo**: a field list at the top (after any title) becomes the
  bibliographic `docinfo`; its `dedication` and `abstract` fields become topics.
- **Tables**: grid tables, with cells spanning rows and columns and a `+===+`
  line ending the header rows; simple tables, with multi-line rows (a line whose
  first column is blank continues the row), a `-` underline grouping a row's
  columns into spanning cells, and a border line ending the header rows. A
  cell holds any body elements (lists, code, nested tables); a cell that is one
  paragraph renders without the `<p>`. A table that cannot be read (ragged
  lines, no bottom border, text in a column margin) is kept as a literal block
  and reported.
- **Comments** (`.. text`) are dropped.

Directives:

| Directive | Becomes |
| --- | --- |
| `attention`, `caution`, `danger`, `error`, `hint`, `important`, `note`, `tip`, `warning` | an `admonition` node, `lang` the kind: `<div class="admonition admonition-note">` |
| `admonition:: Title` | an admonition with that title, its class from the title (`admonition-my-title`) |
| `code` / `code-block` / `sourcecode` `[:: lang]` | a `code` block, `lang` lowercased (`class="language-python"`). Sphinx's options (`linenos`, `emphasize-lines`, `caption`, ...) are accepted and ignored |
| `math` | each blank-line-separated block a `code` block with class `math` (TeX kept as written, for a client-side renderer) |
| `image:: uri` | a paragraph holding the image; `alt` (default: the URI), `width`, `height`, `align` (an `align-*` class), `target` (a URI or `name_`, which links it) |
| `figure:: uri` | a `figure`: the image, the first content paragraph as `caption`, the rest as `legend` (content opening with an empty `..` comment has no caption); `figclass`, `figwidth`, `align` |
| `topic:: Title` / `sidebar:: Title` | a `topic` (a sidebar has class `sidebar`) |
| `rubric:: text` | a `rubric` |
| `epigraph` / `highlights` / `pull-quote` | a block quote, with attribution, classed by the directive name |
| `container:: classes` / `compound` | a `container` with those classes (`compound`) |
| `class:: names` | the classes go to each block of its content, or with no content to the next element |
| `raw:: html` | an `html_block`: escaped unless `Options.allowRawHtml`; other formats are dropped |
| `contents:: [Title]` | a `topic` (class `contents`) with a nested list of links to the sections; `depth`, `local` (only the current section's subsections). Top level only |
| `list-table:: [Title]` | a table from a two-level bullet list; `header-rows`, `stub-columns` (class `stub`), `align`; the title is the `<caption>` |
| `csv-table:: [Title]` | a table from inline CSV, parsed by [`csv`](csv.md); `header` (CSV for the header row), `header-rows`, `delim` (one character, `tab`, `space`), `quote`, `keepspace`, `stub-columns`, `align`. Short rows are padded |
| substitution `replace` / `unicode` / `image` | see references below |

Every directive takes `:class:` (added to the node's classes) and `:name:`
(an internal target: the node gets that `id` and `name_` links to it). An
unknown option is reported and ignored. A directive missing its required
argument or content is kept as a literal block and reported.

Inline: `*emphasis*`, `**strong**`, ` ``literal`` `, interpreted text with
roles (`:emphasis:`, `:strong:`, `:literal:`, `:code:`, `:math:`, `:sub:`,
`:sup:`, `:title-reference:` - the default -, `:abbreviation:`, `:pep:`,
`:rfc:`), inline targets (`` _`name` ``) and backslash escapes, under rST's
rules for where markup may start and end. An unknown role stays as text.

References and targets:

- Named (`name_`, `` `a phrase`_ ``; case- and space-insensitive) and anonymous
  (`name__`, paired with `__ uri` targets in order) references.
- An embedded URI (`` `text <https://...>`_ ``) or alias (`` `text <name_>`_ ``).
- External (`.. _name: uri`), internal (`.. _name:` - gives the next element an
  `id`) and indirect (`.. _a: b_`) targets; section titles are implicit targets.
- Standalone URLs and email addresses.
- Footnotes - manual `[1]_`, auto-numbered `[#]_` / `[#label]_`, symbol `[*]_` -
  and citations (`[CIT2002]_`), with links back from each note.
- Substitutions (`|name|`, `|name|_`) defined with the `replace`, `unicode` or
  `image` directives.

A reference to an unknown target stays as its source text and is reported by
`check`; so are duplicate targets, mismatched anonymous references and
undefined or circular substitutions. A `javascript:` (or other unsafe) target
is filtered: the link renders with `href="#"`.

## Not supported

- **Other directives**: `parsed-literal`, `line-block`, `table`, `sectnum`,
  `header` / `footer`, `meta`, `title`, `default-role`, `role` and all Sphinx
  extensions (`toctree`, `autodoc`, ...). An unknown directive is kept as a
  literal block of its source, so nothing silently disappears, and `check`
  reports it as an error.
- **File access of any kind**, by design: `include` and the `:file:` / `:url:`
  options of `raw` and `csv-table` are refused with an error.
- Custom roles (`.. role::`) and `default-role`.
- `contents` inside a nested body (a quote, a list item), section numbering and
  backlinks from the table of contents.
- `csv-table` with `:delim: tab`: tabs in the source are expanded to spaces
  before the table is read (as docutils does), so the content has no tabs left
  to split on. Use another delimiter.
- Wide (CJK) characters in grid and simple tables: columns are counted in
  characters, not display cells.

## Limits

A document that nests too deep, holds too many nodes, or has a table of more
than 20,000 cells is a catchable `"rst"` error, as is the first problem under
`Options{strict: true}`.

## See also

- [doctree.md](doctree.md) - the document tree `parse` returns, its accessors
  and renderers (including PDF).
- [markdown.md](markdown.md) - the Markdown reader for the same tree.
- [modules/index.md](index.md) - the module catalog and import rules.
