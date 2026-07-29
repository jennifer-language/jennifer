# `pdfwriter` - generate simple PDF documents

Import with `import "pdfwriter.j" as pdf;`. Build a `Document` of `Page`s with
value-semantic builders - text, lines, rectangles - then `render()` writes the
PDF object / xref structure by hand (no stdlib PDF) as `bytes`, the way
[`htmlwriter`](htmlwriter.md) / [`label`](label.md) generate their formats.
Content streams are FlateDecode-compressed via [`compress`](../libraries/compress.md).
Pure Jennifer; runs on **both** binaries.

```jennifer
import "pdfwriter.j" as pdf;
use fs;

def p as pdf.Page init pdf.page(612, 792);
$p = pdf.text($p, 72, 720, "Helvetica", 24, "Hello, PDF");
def doc as pdf.Document init pdf.addPage(pdf.document(), $p);
fs.writeBytes("out.pdf", pdf.render($doc));
```

Runnable: [`examples/modules/pdfwriter_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/pdfwriter_demo.j).

## Coordinates and units

Coordinates are in **PDF points** (1/72 inch), with the origin at the
**bottom-left** and y increasing **upward**. All coordinates and sizes are
integers. Common page sizes: US Letter `612 x 792`, A4 `595 x 842`. Colours are
`0-255` RGB integers.

## Building

Every builder is **value-semantic** - it returns a fresh copy and never mutates
its argument, so you thread them (`$p = pdf.text($p, ...)`).

| Call | Returns | |
| ---- | ------- | - |
| `pdf.document()` | `Document` | an empty document |
| `pdf.page(width, height)` | `Page` | a blank page of the given size |
| `pdf.text(pg, x, y, font, size, str)` | `Page` | draw text in a standard-14 font at (x, y) |
| `pdf.loadFont(name, ttfBytes)` | `LoadedFont` | load an embeddable TrueType font under a resource name |
| `pdf.addFont(doc, lf)` | `Document` | register an embedded font in the document |
| `pdf.textUnicode(pg, x, y, lf, size, str)` | `Page` | draw Unicode text in an embedded font at (x, y) |
| `pdf.loadImage(name, imgBytes)` | `Image` | load a PNG / JPEG under a resource name |
| `pdf.addImage(doc, img)` | `Document` | register an embedded image in the document |
| `pdf.drawImage(pg, img, x, y, width, height)` | `Page` | draw an image into the box at (x, y) |
| `pdf.measureText(font, size, str)` | `float` | width of `str` in points (standard-14) |
| `pdf.measureTextUnicode(lf, size, str)` | `float` | width of `str` in points (embedded font) |
| `pdf.wrapText(font, size, str, maxWidth)` | `list of string` | word-wrap to `maxWidth` points (standard-14) |
| `pdf.wrapTextUnicode(lf, size, str, maxWidth)` | `list of string` | word-wrap to `maxWidth` points (embedded font) |
| `pdf.textBlock(pg, x, y, width, font, size, leading, str, align)` | `Page` | flow wrapped, aligned text into a column (standard-14) |
| `pdf.textBlockUnicode(pg, x, y, width, lf, size, leading, str, align)` | `Page` | flow wrapped, aligned text into a column (embedded font) |
| `pdf.line(pg, fromX, fromY, toX, toY)` | `Page` | draw a stroked line |
| `pdf.rect(pg, x, y, width, height, filled)` | `Page` | draw a rectangle (fill or stroke) |
| `pdf.color(pg, red, green, blue)` | `Page` | set fill + stroke colour for what follows |
| `pdf.addPage(doc, pg)` | `Document` | append a page |
| `pdf.render(doc)` | `bytes` | the finished PDF |

`color` sets the drawing colour for **subsequent** operations on that page (both
fill and stroke), so order matters: set the colour, then draw. `rect`'s `filled`
flag fills the rectangle when `true`, otherwise strokes its outline.

## Fonts

`text` takes one of the **standard-14** base fonts every PDF viewer provides;
any other name throws `Error{kind: "pdfwriter"}`:

```
Helvetica  Helvetica-Bold  Helvetica-Oblique  Helvetica-BoldOblique
Times-Roman  Times-Bold  Times-Italic  Times-BoldItalic
Courier  Courier-Bold  Courier-Oblique  Courier-BoldOblique
Symbol  ZapfDingbats
```

Each distinct font used becomes one shared Type1 font object
(`WinAnsiEncoding`). Text is escaped for the PDF literal-string syntax (`\`, `(`,
`)`, and line breaks), so any ASCII / Latin-1 string is safe to pass.

### Embedded TrueType fonts (Unicode)

For text beyond Latin-1 - accented text, Greek, Cyrillic, CJK, ... - embed a
TrueType (`.ttf`) font and draw with `textUnicode`:

```jennifer
def body as pdf.LoadedFont init pdf.loadFont("Body", fs.readBytes("/path/NotoSans.ttf"));
def doc as pdf.Document init pdf.addFont(pdf.document(), $body);
def p as pdf.Page init pdf.textUnicode(pdf.page(595, 842), 50, 780, $body, 18, "Héllo, 日本語");
$doc = pdf.addPage($doc, $p);
```

The font is embedded as a **Type0 / CIDFontType2** composite (Identity-H
encoding, `FontFile2` font program) over the [`font`](font.md) module: each
character maps through the font's cmap to a glyph, so any script the font covers
renders. A **ToUnicode** map is written too, so the text stays selectable and
copyable in a viewer. The whole font file is embedded (glyph subsetting, for
smaller files, is a follow-on). `loadFont` accepts a TrueType (`glyf`) font; a
CFF / OpenType-PostScript (`.otf`) font throws (its `FontFile3` embedding is a
follow-on).

## Images

Embed a PNG or JPEG as an image XObject and draw it scaled into a box:

```jennifer
def logo as pdf.Image init pdf.loadImage("Logo", fs.readBytes("/path/logo.png"));
def doc as pdf.Document init pdf.addImage(pdf.document(), $logo);
def p as pdf.Page init pdf.drawImage(pdf.page(595, 842), $logo, 50, 700, 120, 90);
$doc = pdf.addPage($doc, $p);
```

`loadImage` detects the format from the file signature and reads the pixel
geometry / colour space. `addImage` registers the image on the document (so the
resource is written once and can be reused across pages); `drawImage` places it,
**scaled to fill** the `width` x `height` box in points - pass a box in the
image's own proportion to avoid stretching. The origin is the box's lower-left,
like everything else.

Supported inputs:

| Format | Handling |
| ------ | -------- |
| **JPEG** (baseline / progressive) | embedded as-is via `DCTDecode`; greyscale / RGB / CMYK (an Adobe CMYK file gets an inverting `/Decode`) |
| **PNG** greyscale / RGB / palette | embedded directly - the raw `zlib` image data with a `FlateDecode` PNG predictor, so no pixel decode happens (1 / 2 / 4 / 8 / 16-bit) |
| **PNG** greyscale+alpha / RGBA (8-bit) | decoded (inflate + de-filter) and split into a colour stream plus an 8-bit greyscale **soft mask** (`/SMask`), so transparency renders |

The image and its soft mask round-trip pixel-exact (the JPEG is re-embedded
losslessly). **Not** supported: interlaced (Adam7) PNG, 16-bit alpha, and
palette `tRNS` transparency - each throws `Error{kind: "pdfwriter"}` with a
reason. `addImage` the image before you `drawImage` it, so the `/name` resolves
(the same rule as `addFont` / `textUnicode`).

## Text layout

`text` / `textUnicode` place a single string at exact coordinates. To flow a
paragraph into a **column** - wrapped, aligned, multi-line - use the layout
functions, which sit on top of accurate width measurement.

**Measure.** `measureText(font, size, str)` returns the rendered width of a
string in points, using the Adobe Core-14 AFM metrics for the standard-14 fonts;
`measureTextUnicode(lf, size, str)` uses an embedded font's own glyph advances.

**Wrap.** `wrapText(font, size, str, maxWidth)` greedily word-wraps to a maximum
line width (points) and returns the lines. Existing newlines in `str` are honoured
as hard breaks (so a `\n\n` leaves a blank line), and runs of spaces collapse. A
single word wider than `maxWidth` lands alone on its line (it overflows rather
than being broken mid-word). `wrapTextUnicode` is the embedded-font form.

**Flow.** `textBlock(pg, x, y, width, font, size, leading, str, align)` wraps
`str` to `width` and draws each line - the first line's baseline at `(x, y)`, each
next line `leading` points lower:

```jennifer
def body as string init "Jennifer flows wrapped, justified paragraphs into a "
    + "column using accurate standard-14 width metrics.";
$p = pdf.textBlock($p, 72, 720, 240, "Times-Roman", 11, 15, $body, "justify");
# the block is len(pdf.wrapText("Times-Roman", 11, $body, 240)) * 15 points tall
```

`align` is one of:

| `align` | Effect |
| ------- | ------ |
| `"left"` | lines start at `x` (the default look) |
| `"right"` | lines end at `x + width` |
| `"center"` | lines centred in the column |
| `"justify"` | inter-word gaps padded so each line fills `width` exactly - **except** the last line of each paragraph, which stays left-aligned |

`textBlockUnicode` is the same for an embedded font. The block does not clip or
paginate; a column height is `len(wrapText(...)) * leading`, so you decide where
it ends and when to start a new page. Measuring a Symbol / ZapfDingbats font, or a
character outside WinAnsi, throws `Error{kind: "pdfwriter"}`.

## Metadata

Set document metadata - the PDF Info dictionary shown in a viewer's "Document
Properties" - with `pdf.info(doc, key, value)`. `key` is a PDF Info key:

| Key | |
| --- | - |
| `Title` / `Author` / `Subject` / `Keywords` | the descriptive fields |
| `Creator` | the app that authored the source |
| `Producer` | the app that wrote the PDF (defaults to `"Jennifer pdfwriter"`) |
| `CreationDate` / `ModDate` | PDF date strings (see `pdfDate` below) |

```jennifer
def doc as pdf.Document init pdf.document();
$doc = pdf.info($doc, "Title", "Q3 Report");
$doc = pdf.info($doc, "Author", "Ada Lovelace");
$doc = pdf.info($doc, "Keywords", "report, finance, q3");
```

`document()` presets `Producer` to `"Jennifer pdfwriter"`; every other field is
unset until you set it. Any custom key works too. Dates use the PDF date syntax,
which `pdf.pdfDate(t)` builds from a `time.Time`:

```jennifer
use time;
$doc = pdf.info($doc, "CreationDate", pdf.pdfDate(time.utc()));   # D:20260714160000+00'00'
```

## Rendering

`render(doc)` produces a complete PDF 1.7 file as `bytes`: a catalog, a page
tree, one page dict + one FlateDecode-compressed content stream per page, the
shared font objects, the image XObjects (each with a soft-mask XObject when it
carries alpha), an Info dictionary when any metadata is set, a cross-reference
table with correct byte offsets, and the trailer. Object numbers are assigned
dynamically, so any mix of pages, fonts, and images cross-references correctly.
Write it with
`fs.writeBytes`, return it from an `httpd` handler, or attach it via `mime`. It
validates clean under `qpdf --check`.

**Byte-identical output.** The same document always renders to the **exact same
bytes** - on either binary, run to run. This is deliberate: `pdfwriter` never
auto-stamps a `CreationDate` or any other timestamp (you opt into one explicitly
via `info` + `pdfDate`), so nothing varies with wall-clock time. That makes the
output safe to assert against a golden file in an automated test, and
reproducible for content-addressed builds.

## Scope

- **Text, lines, rectangles.** The standard-14 fonts, solid fills / strokes, and
  RGB colour. No curves / paths beyond rectangles, no clipping, no transparency.
- **Embedded TrueType fonts** (Type0 / CIDFontType2, Unicode + ToUnicode) via
  `loadFont` / `textUnicode`. Glyph subsetting (smaller files) and CFF `.otf`
  embedding are follow-ons.
- **Raster images** (PNG greyscale / RGB / palette / alpha, JPEG) via
  `loadImage` / `drawImage`. Interlaced PNG, 16-bit alpha, and palette `tRNS`
  are follow-ons.
- **Text layout** - width measurement, word-wrap, and left / right / center /
  justify column flow via `measureText` / `wrapText` / `textBlock`. No automatic
  pagination or clipping (you place each block); tab stops, hyphenation, and
  mixed inline runs are out of scope.
- **A writer, not a reader.** It generates PDFs; it does not parse them.

## See also

- [compress.md](../libraries/compress.md) - the FlateDecode (`zlib`) streams.
- [htmlwriter.md](htmlwriter.md) / [label.md](label.md) - the sibling
  format-generation modules.
- [fs.md](../libraries/fs.md) - `writeBytes` to save the rendered PDF.
- [modules/index.md](index.md) - the module catalog and import rules.
