# `myst` - read and render MyST Markdown

Import with `import "myst.j" as myst;`. Reads [MyST](https://mystmd.org)
(Markedly Structured Text) - CommonMark plus the directives and roles of
reStructuredText, as Sphinx and Jupyter Book write it - into a
[`doctree`](doctree.md) document, the same tree [`markdown`](markdown.md) and
[`rst`](rst.md) build, so one set of renderers serves all three: HTML, XHTML,
styled terminal text and PDF. Parsing is lenient: an unknown directive or role
is kept and reported by `check`. Pure Jennifer, no file I/O. Runs on either
binary.

````jennifer
use io;
import "myst.j" as myst;

def src as string init '(intro)=
# Introduction

```{note}
Water is H{sub}`2`O; see {ref}`intro`.
```
';
io.printf("%s\n", myst.toHtml($src));
for (def d in myst.check($src)) {
    io.printf("%s line %d: %s\n", $d.severity, $d.line, $d.message);
}
````

Write MyST in a `.j` string as a raw `'...'` literal: a cooked `"..."` reads
`{note}` and `{sub}` as interpolation. Import `doctree` too only when your code
names `doctree.` itself (to type a tree or walk / render it).

## Surface

| Call | Returns | Notes |
| --- | --- | --- |
| `myst.parse(src)` | `doctree.Node` | Parse to the document root (`typeOf` `"document"`). |
| `myst.parseWith(src, opts)` | `doctree.Node` | `parse` with an `Options`. |
| `myst.check(src)` | `list of Diagnostic` | The problems in the document, in source order; empty when it is clean. |
| `myst.toHtml(src)` | `string` | HTML; raw HTML is escaped and link targets filtered. |
| `myst.toHtmlWith(src, opts)` | `string` | `toHtml` with an `Options`. |
| `myst.toXhtml(src)` / `myst.toAnsi(src)` | `string` | XHTML; styled terminal text. |
| `myst.frontMatter(src)` | `string` | The YAML between the opening `---` lines, or `""`; decode it with `yaml`. |

For PDF, render the tree: `doctree.renderPdf(myst.parse(src), doctree.pdfDefaults())`.

`Options` (the zero value is the default):

| Field | Meaning |
| --- | --- |
| `strict` | Throw the first problem as a `"myst"` error instead of recovering. |
| `allowRawHtml` | Pass raw HTML (and `{raw} html`) through (trusted input only). |
| `noDollarMath` | Read `$...$` / `$$...$$` as text, not math. |
| `noColonFence` | Read `:::` lines as text, not directive fences. |

A `Diagnostic` has `severity` (`"warning"` or `"error"`), `line` (1-based; 0
when not known) and `message`.

## Syntax

Everything [`markdown`](markdown.md) reads - CommonMark, GFM tables, task
lists, strikethrough and footnotes, definition lists, alerts, attribute lists -
plus:

| Syntax | Meaning |
| --- | --- |
| ```` ```{name} argument ```` ... ```` ``` ```` | A directive. Options follow as `:key: value` lines, or as a `---` block of `key: value` lines; then the content. |
| `:::{name} argument` ... `:::` | The same, in a colon fence (nest with more colons outside: `::::{note}` around `:::{tip}`). |
| ``{name}`content` `` | A role. |
| `(label)=` | A target: the next block (a heading, a paragraph, a figure, ...) gets the id `label`. |
| ``{ref}`label` ``, ``{ref}`text <label>` `` | A link to a target, showing the target's title (a heading's text, a figure's caption, a table's title) unless text is given. `{numref}` and `{eq}` work alike (`{eq}` shows `(label)`); `[text](#label)` works too. |
| ``{doc}`path` `` | A link to another document, as written. |
| `% comment` | A comment line, dropped. |
| `+++` | A block break: a thematic break. |
| `---` ... `---` at the top | Front matter, kept out of the document (see `frontMatter`). |
| `$x^2$`, `$$ ... $$ (label)` | Dollar math: inline math and a math block (label optional). |

Inline `$...$` follows the dollarmath rules, so prices stay text: an opening
`$` may not be followed by a space, nor a closing one preceded by a space or
followed by a digit (`$5 and $10` is not math).

## Directives

| Directive | Becomes |
| --- | --- |
| `note`, `tip`, `hint`, `important`, `warning`, `caution`, `attention`, `danger`, `error`, `seealso` | an `admonition` (a first-line argument starts the content) |
| `admonition` `Title` | an admonition with that title (class `admonition-<slug>`) |
| `code`, `code-block`, `sourcecode`, `code-cell` `[lang]` | a `code` block; Sphinx's options (`linenos`, `emphasize-lines`, `caption`, ...) are accepted and ignored |
| `math` | each blank-line-separated block a `code` block with class `math`; `:label:` makes it a target for `{eq}` |
| `image` `uri` | a paragraph holding the image (`alt`, `width`, `height`, `align`, `target`) |
| `figure` `uri` | a `figure`: the image, the first paragraph as `caption`, the rest as `legend` (`figclass`, `figwidth`, `align`) |
| `topic` / `sidebar` `Title` | a `topic` |
| `rubric` `text` | a `rubric` |
| `epigraph`, `highlights`, `pull-quote` | a quote classed by the name |
| `container` `[classes]` | a `container` |
| `table` `[Caption]` | the Markdown table in its content, with the caption |
| `list-table` `[Caption]` | a table from a two-level bullet list (`header-rows`) |
| `csv-table` `[Caption]` | a table from inline CSV (`header`, `header-rows`, `delim`) |
| `raw` `html` | an `html_block`, escaped unless `allowRawHtml`; other formats are dropped |
| `contents` / `toc` `[Title]` | a table of contents of the headings (`depth`, `local`); top level only |
| `eval-rst` | its content read as reStructuredText by [`rst`](rst.md); rST's problems are reported at their lines |

Every directive takes `:class:` and `:name:` (a target, as `(label)=`). An
unknown option is reported and ignored; a directive missing its argument or
content, an unknown directive, `include` and `literalinclude` are kept as a
literal block of their source and reported.

## Roles

| Role | Becomes |
| --- | --- |
| `emphasis`, `strong` | emphasis / strong |
| `sub` / `subscript`, `sup` / `superscript` | sub / superscript |
| `literal`, `code`, `samp`, `file`, `command` | a code span |
| `kbd`, `math` | a code span with class `kbd` / `math` |
| `abbr` | `` `CSS (Cascading Style Sheets)` `` -> `<abbr title>` |
| `guilabel`, `menuselection` | a `span` with that class |
| `pep`, `rfc` | a link to the PEP / RFC |
| `ref`, `numref`, `eq`, `doc`, `download`, `term` | a cross-reference (see Syntax) |

An unknown role is kept as its source text and reported.

## Not supported

- Sphinx's project-level features: `toctree`, `autodoc`, cross-document
  references (a `{ref}` reaches labels in the same document), substitutions,
  numbered figures and equations (`{numref}` shows the title, `{eq}` the label).
- Reading files of any kind, by design.
- A directive's content does not see the document's link reference definitions
  or footnote definitions outside it; footnotes inside a directive are numbered
  on their own.
- Inline math spanning lines; `amsmath` environments outside `{math}`.

## See also

- [doctree.md](doctree.md) - the document tree `parse` returns, its accessors and
  renderers (including PDF).
- [markdown.md](markdown.md) - the CommonMark reader underneath.
- [rst.md](rst.md) - the reStructuredText reader `eval-rst` uses.
- [modules/index.md](index.md) - the module catalog and import rules.
