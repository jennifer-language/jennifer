# `mdpdf` - render Markdown to a laid-out PDF

Import with `import "mdpdf.j" as mdpdf;`. Turns a Markdown document into a
paginated PDF ("write markup, get a PDF"): it parses with the
[`markdown`](markdown.md) module, then flows the block tree onto pages with the
[`pdf`](pdf.md) layout primitives (`measureText` / `wrapText` / `text` / `line`).
Pure Jennifer over `markdown` + `pdf`; runs on either binary.

```jennifer
use fs;
import "mdpdf.j" as mdpdf;

def out as bytes init mdpdf.render("# Title\n\nA **bold** paragraph.\n");
fs.writeBytes("out.pdf", $out);
```

Runnable: [`examples/modules/mdpdf_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/mdpdf_demo.j).

## Surface

| Call                          | Returns   | Notes                                                                 |
| ----------------------------- | --------- | --------------------------------------------------------------------- |
| `mdpdf.render(md)`            | `bytes`   | Parse and render a Markdown string with the default options.          |
| `mdpdf.renderWith(md, opts)`  | `bytes`   | Render a Markdown string with custom `Options`.                       |
| `mdpdf.renderTree(doc, opts)` | `bytes`   | Render a parsed (or transformed) `markdown.Node` tree.                |
| `mdpdf.defaults()`            | `Options` | The default options (US Letter, 72-pt margins, Helvetica / Courier).  |

`render` / `renderWith` are wrappers over `renderTree(markdown.parse(md), ...)`, so
to inspect or transform a document first, parse it with `markdown`, edit the tree,
and hand it to `renderTree`:

```jennifer
import "markdown.j" as markdown;
import "mdpdf.j" as mdpdf;

def tree as markdown.Node init markdown.parse($md);
# ... walk / rewrite the tree with the markdown reader ...
def out as bytes init mdpdf.renderTree($tree, mdpdf.defaults());
```

## Options

`Options` is a plain value struct; call `defaults()` and tweak the fields you want
(value semantics - the copy is independent).

| Field                         | Default            | Meaning                                    |
| ----------------------------- | ------------------ | ------------------------------------------ |
| `pageWidth` / `pageHeight`    | `612` / `792`      | Page size in points (A4 is `595` / `842`). |
| `margin`                      | `72`               | Margin on every side, in points.           |
| `bodyFont`                    | `"Helvetica"`      | Body text font (a standard-14 name).       |
| `boldFont` / `italicFont`     | `-Bold` / `-Oblique` | Fonts for `**strong**` / `*emphasis*`.   |
| `monoFont`                    | `"Courier"`        | Inline and block code.                     |
| `headingFont`                 | `"Helvetica-Bold"` | Headings.                                  |
| `bodySize`                    | `11`               | Body point size; heading sizes derive from the level. |

```jennifer
def a4 as mdpdf.Options init mdpdf.defaults();
$a4.pageWidth = 595;
$a4.pageHeight = 842;
$a4.margin = 54;
def out as bytes init mdpdf.renderWith($md, $a4);
```

## What is rendered

Every block the `markdown` reader produces is placed and paginated - a new page
starts when the next line would cross the bottom margin.

| Block          | Layout                                                                 |
| -------------- | --------------------------------------------------------------------- |
| Heading        | Sized bold text (h1 22pt down to h6 body size).                       |
| Paragraph      | Word-wrapped, with per-run fonts: `**bold**`, `*italic*`, `` `code` ``. |
| List           | Indented `-` / `N.` markers; nested lists indent further.             |
| Table (GFM)    | A ruled grid, bold header row, per-column alignment (`:--` / `--:` / `:-:`). |
| Fenced code    | A monospaced block, indented.                                         |
| Blockquote     | Its inner blocks, indented from the body.                            |
| Image          | The alt text in brackets (`[alt]`); the source is not fetched.        |

## Notes

- **Fonts are the standard-14 PDF base fonts**, so text is best kept to the Latin-1
  range. Embedding a Unicode TrueType font (for other scripts) is a future
  refinement; the underlying [`pdf`](pdf.md) module already supports embedded fonts
  via `loadFont` / `textUnicode`.
- **Inline styling is per word.** A run's font is chosen per inline node, so emphasis
  that splits a word (`a**b**c`) renders the fragments as separate space-joined
  words - keep emphasis to whole words.
- **A pathological document is bounded by `markdown`'s own parse caps** (nesting
  depth and node budget), which raise a catchable `"markdown"` error.

## See also

- [`markdown`](markdown.md) - the Markdown parser and document tree this renders.
- [`pdf`](pdf.md) - the PDF layout primitives underneath.
