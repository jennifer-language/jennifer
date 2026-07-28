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
| `qr` | 2D | Reed-Solomon over GF(256), EC levels L/M/Q/H (`opts.ecLevel`), automatic version selection 1-10, data-mask scoring, byte mode (any UTF-8) |
| `code128` | 1D | Code set B (ASCII 32-126), auto checksum |
| `code39` | 1D | uppercase + digits + `-. $/+%`, `*` start/stop |
| `ean13` | 1D | 12 or 13 digits (check digit computed if omitted) |
| `ean8` | 1D | 7 or 8 digits |
| `itf` | 1D | Interleaved 2 of 5, even digit count |

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
    ecLevel as string, foreground as string, background as string
};
```

`barcode.defaults()` gives scale 4, quiet 4, EC level M, black on white.

| Call | Returns | |
| ---- | ------- | - |
| `barcode.svg(symbol, opts)` | `string` | resolution-independent SVG (embeds in HTML / email) |
| `barcode.png(symbol, opts)` | `bytes` | a monochrome (grayscale) PNG, hand-encoded over `compress` + `crc` |
| `barcode.terminal(symbol)` | `string` | Unicode half-block art for the CLI / REPL (2D only) |
| `barcode.matrix(symbol)` | `list of list of bool` | the raw 2D cells (e.g. to feed `label.image`) |

`opts.scale` is pixels (PNG) or units (SVG) per module / narrow bar; `opts.quiet`
is the mandatory light border in modules; `opts.height` is the bar height for 1D
codes; `opts.foreground` / `background` are the SVG / PNG colours.

## Verification

Correctness is pinned two ways: the overlay
(`modules/barcode_test.j`) checks the Reed-Solomon against the canonical QR
vector, the format / version BCH against known values, byte-mode codewords, and
1D bar patterns; and the Go suite (`cmd/jennifer/barcode_test.go`) renders PNGs,
decodes them with the standard library (proving the hand-rolled PNG is
byte-correct), and - where `zbarimg` is available - **optically scans** them to
confirm they read.

## Scope

- **QR versions 1-10** (up to ~270 bytes at level L). Byte mode only (universal);
  numeric / alphanumeric compaction and versions 11-40 are follow-ons.
- **The GF(256) / Reed-Solomon math** lives in a private `barcode_ecc.j`
  (`include`d), isolated so it can be extracted into an `ecc` module if a second
  consumer (e.g. Data Matrix) ever appears.
- **No general image library** - the only raster need is a monochrome bitmap,
  which the PNG encoder covers directly.
- **Data Matrix / Aztec / PDF417** are not included (a Data Matrix would reuse
  `barcode_ecc.j` inside this module).

## See also

- [label.md](label.md) - printer-native barcode *commands* (the other half).
- [compress.md](../libraries/compress.md) / [crc.md](../libraries/crc.md) - the
  PNG encoder's zlib and CRC-32.
- [modules/index.md](index.md) - the module catalog and import rules.
