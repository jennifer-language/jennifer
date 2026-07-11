# `markdown` - render a Markdown subset to HTML and ANSI

Import with `import "markdown.j" as markdown;`. Renders a small CommonMark
subset to **HTML** (through the [`htmlwriter`](htmlwriter.md) module, so
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

Runnable: [`examples/modules/markdown_demo.j`](https://github.com/mplx/jennifer-lang/blob/main/examples/modules/markdown_demo.j).

## Surface

Rendering (Markdown in, HTML / terminal text out):

| Call                  | Returns  | Notes                                                            |
| --------------------- | -------- | --------------------------------------------------------------- |
| `markdown.toHtml(md)` | `string` | Render to HTML: block elements concatenated, no indentation.    |
| `markdown.toAnsi(md)` | `string` | Render to terminal text with `ansi` styling (self-suppressing). |

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

## Supported Markdown

A deliberately small [CommonMark](https://commonmark.org) subset:

| Block                | Syntax                          | HTML                        |
| -------------------- | ------------------------------- | --------------------------- |
| Heading (levels 1-6) | `# H` ... `###### H`            | `<h1>` ... `<h6>`           |
| Paragraph            | consecutive text lines          | `<p>` (lines joined by ` `) |
| Unordered list       | `- x` / `* x` / `+ x`           | `<ul><li>`                  |
| Ordered list         | `1. x`                          | `<ol><li>`                  |
| Fenced code block    | ` ``` ` ... ` ``` `             | `<pre><code>`               |

| Inline    | Syntax          | HTML                  | ANSI            |
| --------- | --------------- | --------------------- | --------------- |
| Bold      | `**text**`      | `<strong>`            | bold            |
| Italic    | `*text*`        | `<em>`                | italic          |
| Code      | `` `text` ``    | `<code>`              | cyan            |
| Link      | `[text](url)`   | `<a href="url">`      | underline + ` (url)` |

## HTML output

`toHtml` builds an [`htmlwriter`](htmlwriter.md) node tree and renders it, so
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

io.printf("%s\n", $doc);          # Markdown source
io.printf("%s\n", markdown.toHtml($doc));   # ... or rendered
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

Note the asymmetry: `table` emits GFM tables for other renderers (GitHub, a
docs site), but tables are **not** in the subset this module *reads*, so
`toHtml(markdown.table(...))` does not render them back to `<table>` - the
other authoring helpers round-trip, `table` does not.

## Not supported

This is a subset, chosen to stay small and TinyGo-clean:

- **Inline spans do not nest.** The content of `**...**`, `` `...` ``, and a
  link's text is taken as plain text, so `**a `b`**` does not render the
  inner code span.
- No blockquotes, thematic breaks (`---`), images, reference links,
  autolinks, HTML passthrough, tables, or setext (underlined) headings.
- No nested / indented lists; a list is a flat run of same-kind items.

For anything beyond this subset, render with an external tool. The module is
sized for READMEs, help text, and comment / docblock bodies, not
general-purpose CommonMark conformance.

## See also

- [htmlwriter.md](htmlwriter.md) - the HTML backend `toHtml` renders through.
- [ansi.md](ansi.md) - the terminal styling `toAnsi` renders through.
- [modules/index.md](index.md) - the module catalog and import rules.
