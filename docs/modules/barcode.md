# `barcode` - barcode / QR code generation

Import with `import "barcode.j" as barcode;`. Generate scannable codes as
**images** - the complement to [`label`](label.md), which emits printer-native
barcode *commands*. `encode(data, symbology, opts)` builds a device-independent
`Symbol` (a module matrix for 2D, bar widths for 1D), and the renderers turn it
into SVG, PNG, terminal art, or the raw matrix. Pure `.j` over `compress` (zlib)
+ `crc` (CRC-32) + `encoding` + the bitwise operators - no image library; runs
on both binaries.

```jennifer
import "barcode.j" as barcode;

def opts as barcode.Options init barcode.defaults();
def qr as barcode.Symbol init barcode.encode("https://example.com", "qr", $opts);
def svg as string init barcode.svg($qr, $opts);   # embed in HTML / email
def png as bytes init barcode.png($qr, $opts);     # a monochrome PNG
```

Runnable: [`examples/modules/barcode_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/barcode_demo.j).

## Encoding

`barcode.encode(data, symbology, opts) -> Symbol`. Symbologies:

| Symbology | Kind | Notes |
| --------- | ---- | ----- |
| `qr` | 2D | Reed-Solomon over GF(256), EC levels L/M/Q/H (`opts.ecLevel`), automatic version selection 1-40, data-mask scoring; numeric / alphanumeric / byte mode chosen for compactness |
| `datamatrix` | 2D | ECC200, square symbols 10x10 to 26x26, ASCII encodation (digit-pair packing), Reed-Solomon over GF(0x12d) |
| `code128` | 1D | Code set B (ASCII 32-126), auto checksum |
| `code93` | 1D | uppercase + digits + `-. $/+%`, two check characters (C, K) |
| `code39` | 1D | uppercase + digits + `-. $/+%`, `*` start/stop |
| `ean13` | 1D | 12 or 13 digits (check digit computed if omitted) |
| `ean8` | 1D | 7 or 8 digits |
| `upca` | 1D | 11 or 12 digits (UPC-A; an EAN-13 with a leading 0) |
| `upce` | 1D | 6 / 7 / 8 digits (UPC-E, zero-compressed) |
| `itf` | 1D | Interleaved 2 of 5, even digit count |
| `gs1-128` | 1D | Code 128 with a leading FNC1 and parenthesised Application Identifiers, e.g. `(01)09501101020917(10)ABC123` |

```jennifer
def enum barcode.SymbolKind { Matrix, Linear };
def struct barcode.Symbol {
    kind as SymbolKind,                # SymbolKind.Matrix (2D) or SymbolKind.Linear (1D)
    size as int,                       # matrix dimension (2D; 0 for 1D)
    matrix as list of list of bool,    # the 2D module grid (true = dark)
    bars as list of int,               # 1D bar/space run widths, starting with a bar
    text as string                     # the encoded data
};
```

## Rendering

```jennifer
def struct barcode.Options {
    scale as int, height as int, quiet as int,
    ecLevel as string, foreground as string, background as string,
    humanReadable as bool   # render the data as a text line under a 1D SVG (on by default)
};
```

`barcode.defaults()` gives scale 8, quiet 4, EC level M, black on white, and a
human-readable text line under 1D SVGs (`opts.humanReadable = false` to omit it).
Scale 8 (8 px per module) keeps a code comfortably scannable by a phone camera
off a screen - a smaller render can defeat autofocus or pick up screen moire;
lower `opts.scale` for a more compact image.

| Call | Returns | |
| ---- | ------- | - |
| `barcode.svg(symbol, opts)` | `string` | resolution-independent SVG (embeds in HTML / email) |
| `barcode.png(symbol, opts)` | `bytes` | a monochrome (grayscale) PNG, hand-encoded over `compress` + `crc` |
| `barcode.terminal(symbol)` | `string` | Unicode half-block art for the CLI / REPL (2D only), wrapped in a 4-module quiet zone so it stays camera-scannable |
| `barcode.matrix(symbol)` | `list of list of bool` | the raw 2D cells (e.g. to feed `label.image`) |

`opts.scale` is pixels (PNG) or units (SVG) per module / narrow bar; `opts.quiet`
is the mandatory light border in modules; `opts.height` is the bar height for 1D
codes; `opts.foreground` / `background` are the SVG / PNG colours.

## Verification

Correctness is pinned two ways: the overlay
(`modules/barcode_test.j`) checks the Reed-Solomon against the canonical QR
vector, the format / version BCH against known values, mode codewords (including
the QR spec's numeric worked example), the DataMatrix 10x10 matrix against
`zint`, and 1D bar patterns against independent reference encoders; and the Go
suite (`cmd/jennifer/barcode_test.go`) renders PNGs, decodes them with the
standard library (proving the hand-rolled PNG is byte-correct), and - where
`zbarimg` is available - **optically scans** them to confirm they read.

## Scope

- **QR versions 1-40**, all four EC levels, with numeric / alphanumeric / byte
  mode chosen for compactness. A byte-mode payload with non-ASCII bytes gets an
  ECI(26) UTF-8 declaration, so it decodes as UTF-8 in a strict reader. Kanji
  mode, other ECI assignments, and structured-append are not covered. Large
  versions (v30+) are slow to render (the mask-penalty scoring runs in the
  tree-walking interpreter), not incorrect.
- **DataMatrix ECC200 square symbols 10x10 to 26x26** (single data region), ASCII
  encodation. Rectangular symbols, larger sizes (region interleaving), and the
  C40 / Text / X12 / EDIFACT / Base256 encodations are follow-ons; a letter-heavy
  payload is valid but larger than an optimal-encodation encoder would produce.
- **The GF(256) / Reed-Solomon math** lives in a private `barcode_ecc.inc.j`
  (`include`d, parameterised by primitive polynomial and generator base, so QR
  (0x11d) and DataMatrix (0x12d) share it).
- **No general image library** - the only raster need is a monochrome bitmap,
  which the PNG encoder covers directly.
- **Aztec / PDF417 / MaxiCode** are not included.

## See also

- [label.md](label.md) - printer-native barcode *commands* (the other half).
- [compress.md](../libraries/compress.md) / [crc.md](../libraries/crc.md) - the
  PNG encoder's zlib and CRC-32.
- [modules/index.md](index.md) - the module catalog and import rules.
