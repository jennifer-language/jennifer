# `font` - TrueType / OpenType font parsing

Import with `import "font.j" as font;`. A **pure-Jennifer** TrueType / OpenType
parser: read a `.ttf` / `.otf` from `bytes` and expose its metrics, character
map, and glyph outlines - no Go, no dependency, just the `bytes` type and the
bitwise operators for the big-endian tables, so it runs on **both binaries**.

Both outline backends ship: the **TrueType `glyf`** backend (simple + composite
glyphs, quadratic curves) and the **CFF / PostScript** backend for OpenType
`OTTO` fonts (a Type2 charstring interpreter with global / local subroutines and
CID-keyed FDArray / FDSelect, so CJK fonts outline too), detected on parse. It
parses `head`, `cmap` (formats 4 and 12), `maxp` / `hhea` / `hmtx`, `OS/2`
(vertical metrics), the legacy `kern` table, `loca` / `glyf` or `CFF `, and
`name`.

```jennifer
use io;
import "font.j" as font;

def f as font.Font init font.open("/usr/share/fonts/TTF/DejaVuSans.ttf");
io.printf("%s, %d upem\n", font.name($f), font.unitsPerEm($f));
io.printf("<path d=\"%s\"/>\n", font.glyphPath($f, 65));   # outline of 'A'
```

Runnable: [`examples/modules/font_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/font_demo.j) (a word rendered to an SVG).

## Surface

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `font.parse(b)` | `Font` | Parse a font from `bytes` (`.ttf` or `.otf`). |
| `font.open(path)` | `Font` | Read a font file and parse it. |
| `font.unitsPerEm(f)` | `int` | The em-square size - the coordinate scale of every metric and outline. |
| `font.name(f)` | `string` | The font family name. |
| `font.advance(f, codepoint)` | `int` | The horizontal advance width of a codepoint's glyph, in font units. |
| `font.kern(f, left, right)` | `int` | The `kern`-table pair adjustment between two codepoints (0 when absent). |
| `font.ascender(f)` | `int` | Typographic ascender (OS/2 `sTypoAscender`, else hhea). |
| `font.descender(f)` | `int` | Typographic descender (negative). |
| `font.lineGap(f)` | `int` | Typographic line gap (line height = ascender - descender + lineGap). |
| `font.capHeight(f)` | `int` | Cap height (OS/2 `sCapHeight`; 0 when unavailable). |
| `font.xHeight(f)` | `int` | x-height (OS/2 `sxHeight`; 0 when unavailable). |
| `font.glyphPath(f, codepoint)` | `string` | The glyph outline as an SVG path `d` string. |
| `font.glyph(f, codepoint)` | `Glyph` | The raw outline: contours of on / off-curve points, advance, and bounding box. |

A codepoint the font lacks maps to glyph 0 (`.notdef`).

## Coordinates and outlines

Everything is in **font units** - divide by `unitsPerEm` (typically 1000 or
2048) to get em fractions, then multiply by your point size. Fonts store **y
pointing up**, so flip y for screen rendering (an SVG `transform="scale(1,-1)"`
around the whole word does it).

`font.glyphPath` emits an SVG path: `M` / `L` for straight segments, `Q`
(quadratic Bezier) for TrueType curves, and `C` (cubic Bezier) for CFF curves -
each backend's native curve type. For the raw outline, `font.glyph` returns a
`Glyph` (whose points are always quadratic - a CFF cubic is approximated as two
quadratics there, while `glyphPath` keeps the exact cubic):

- `Glyph { advance, xMin, yMin, xMax, yMax, contours as list of Contour }`
- `Contour { points as list of Point }`
- `Point { x, y, onCurve }` - an off-curve point is a quadratic control point.

Composite glyphs (an accented letter built from a base plus a mark) are resolved
into a single set of contours, with each component's translation and scale
applied.

## Laying out a string

Walk the string, outline each glyph, and advance the pen by its advance width:

```jennifer
def x as int init 0;
for (def i as int init 0; $i < len($chars); $i = $i + 1) {
    def cp as int init charCode($chars[$i]);
    render(font.glyphPath($f, $cp), $x);              # your renderer, offset by x
    $x = $x + font.advance($f, $cp);
}
```

Pair kerning from the legacy `kern` table is available via `font.kern(f, left,
right)` (add it to the pen advance); vertical metrics come from `font.ascender` /
`descender` / `lineGap` / `capHeight` / `xHeight`.

```jennifer
$x = $x + font.advance($f, $prev) + font.kern($f, $prev, $cp);   # kerned advance
```

## Scope

**In:** TrueType `glyf` and OpenType `CFF ` outlines (including CID-keyed CJK
fonts, subroutines, and the flex operators), `cmap` formats 4 and 12, `hmtx`
advances, OS/2 vertical metrics, and legacy `kern`-table pair kerning. **Out:**
kerning / shaping from `GPOS` / `GSUB` (modern fonts put kerning there; the
legacy `kern` table is read), hinting, colour / emoji tables, `CFF2` and
variable-font axes.

A hostile font cannot hang the outliner: charstring subroutine recursion is
bounded (a runaway self-call raises a catchable `font` error, not an infinite
loop), the composite-glyph depth is capped, and every table loop is finite.

The tests parse tiny committed fixtures
(`modules/testdata/font_fixture.ttf` and `font_fixture_cff.otf`, regenerable with
`scripts/gen-font-fixture.py`); the CFF backend is cross-checked against
`fontTools` (pixel-exact outlines, IoU 1.0) over ~170 glyphs on real system
fonts, including a CID-keyed CJK font, and a hand-crafted recursive font pins the
guard.

## Performance

Parsing is cheap - even a multi-megabyte CJK CFF font opens in ~100 ms, because
`parse` only reads the table directory and headers. Glyph access scales with the
glyph, not the font: a single CharStrings / subr INDEX entry is read in O(1)
(never walking the whole 65k-glyph INDEX), the format-12 cmap is binary-searched,
and CFF subroutines are fetched on demand - so a CJK glyph outlines in a fraction
of a second. There is no need to cache or serialise a parsed `Font`: it is just
the raw file bytes plus a small table of offsets, so re-`open`ing the file is as
fast as reloading any cache would be.
