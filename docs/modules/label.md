# `label` - industrial label printing

Import with `import "label.j" as label;`. Describe and print labels for
industrial label printers. **One** module, one way to describe a label, with the
printer language as a **selectable backend** (a `Device` dialect) rather than a
module per printer. A deliberate **three-stage pipeline** keeps the stages
independent:

1. **build** a device-independent `Label` in millimetres,
2. **render** it to a chosen dialect string, and
3. **emit** that string anywhere.

Build and render are pure text and run on **both** binaries; only `send` (the
`:9100` convenience) uses `net`, so it needs the default **`jennifer`** binary.

```jennifer
import "label.j" as label;

def l as label.Label init label.new(50.0, 30.0);          # 50 x 30 mm
$l = label.text($l, 5.0, 5.0, label.TextOptions{height: 4.0}, "HELLO");
$l = label.barcode($l, 5.0, 15.0, "code128", "12345678");
def zpl as string init label.render($l, label.Device{dialect: "zpl", dpi: 203});
# label.send("192.168.1.50", 9100, $zpl);                 # to a printer's raw port
```

Runnable: [`examples/modules/label_demo.j`](https://github.com/mplx/jennifer-lang/blob/main/examples/modules/label_demo.j).

## Stage 1 - build (device-independent, millimetres)

Every builder is value-semantic and returns a new `Label`, so a label is
assembled by reassignment. Coordinates and sizes are **millimetres** - never
device dots.

| Call / type                                       | Notes                                             |
| ------------------------------------------------- | ------------------------------------------------- |
| `label.Label`                                     | `width`, `height` (mm), `quantity`, `fields`.     |
| `label.Field`                                     | one placed field (`kind` "text"/"barcode"/"box"). |
| `label.TextOptions`                               | `height` - the font height in mm.                 |
| `label.Device`                                    | `dialect` ("zpl"/"cab") + `dpi` (for raster).     |
| `label.new(width, height)`                        | A new empty label of that size in mm (quantity 1). |
| `label.text(label, x, y, opts, content)`          | Place text at (x, y) with `opts.height` mm font.  |
| `label.barcode(label, x, y, type, opts, data)`    | Place a barcode (symbologies + `opts` below).     |
| `label.box(label, x, y, w, h, thickness)`         | Place a rectangular outline (all mm).             |
| `label.image(label, x, y, name)`                  | Place a pre-stored image by name (native size).   |
| `label.quantity(label, n)`                        | Set the number of copies.                         |

Barcode `type` is a **linear** symbology - `"code128"`, `"ean13"`, `"itf"`
(Interleaved 2 of 5), `"code39"`, `"gs1-128"` - or a **2D** symbology -
`"datamatrix"`, `"qr"`. GS1-128 data uses the parenthesised Application
Identifier form (`(00)3006...`). `label.image` references an image already
stored on the printer (cab: the `images/` folder; ZPL: a stored graphic);
`name` is the stored name in that dialect's convention.

`opts` is a `label.BarcodeOptions` refining the barcode; a zero-value struct
(`def o as label.BarcodeOptions;`) means the defaults:

| Field | Effect |
| ----- | ------ |
| `height` (float, mm) | Bar height (linear) or module size (2D); `0` uses the default (15 mm / 1 mm). |
| `checkDigit` (string) | Append an auto-computed check digit: `"mod10"`, `"mod11"`, `"mod16"`, `"mod36"`, `"mod43"` (`""` = none). On cab this is `+MODxx`; on ZPL it toggles a symbology's native check (Code 39 / ITF) - Code 128 / EAN / GS1 carry the check digit in the data. |
| `errorLevel` (string) | 2D error-correction level `"L"`/`"M"`/`"Q"`/`"H"` (`""` = default). |
| `hideText` (bool) | `true` suppresses a linear code's human-readable line. |

**ITF** (Interleaved 2 of 5, the standard shipping-carton symbology) is
numeric-only and even-length because the encoding pairs digits: `label.barcode`
rejects non-numeric ITF data (a catchable `Error`, kind `"label"`) and pads
odd-length data with a leading zero (so a 13-digit body becomes ITF-14). An
unknown barcode `type` also throws.

## Stage 2 - render (to a dialect)

`label.render(label, device)` returns the command stream for the device's
dialect as a plain string. `Device.dpi` is the printer resolution used to
convert millimetres to dots for raster dialects; millimetre-native dialects
ignore it. An unknown dialect throws (kind `"label"`).

- **`"zpl"` - Zebra Programming Language.** The dominant, public label
  language; cab Squix printers accept it too, so this one dialect drives most
  hardware. Emits `^XA` / `^FO` / `^A0` / `^FD` / `^FS`, `^BY` / `^BC` (with
  `^BE` for EAN-13, `^B2` for ITF, `^BQ` for QR), `^GB`, `^PQ`, `^XZ`,
  converting millimetres to dots at the target `dpi`. Text is escaped via `^FH`
  hex for the ZPL command characters (`^`, `~`, `_`) and non-ASCII bytes.
- **`"cab"` - cab JScript.** The native language of cab printers,
  millimetre-native (it ignores `dpi`). Emits `m` / `J` / `S` / `T` / `B` / `G`
  / `A` per the cab JScript Programming Manual (edition 05/2025):
  `T x,y,r,font,size;text` (font 3 = Swiss 721), `G x,y,r;R:w,h,hD,vD` for a box,
  and `B x,y,r,type,size;data` where an uppercase type name prints the
  human-readable line and the size is `height,ne` for Code 128 / EAN-13,
  `height,ne,ratio` for Interleaved 2 of 5, and a single module size for QR.

> **cab dialect note.** The encoder follows the cab JScript Programming Manual
> (edition 05/2025), cross-checked against a real cabLabel-generated job. The
> `S` label-size line uses gap 0 (`dy = height`); adjust it to your media if the
> stock has a gap between labels. Pending final verification on cab Apollo A4+ /
> Squix hardware.

## Stage 3 - emit (transport-agnostic)

The rendered string is yours to deliver: write it to a `*.prom`-style spool file
or a USB device node with `fs`, store it, or send it over the network. The
module ships one convenience for the common case:

| Call                              | Notes                                                     |
| --------------------------------- | --------------------------------------------------------- |
| `label.send(host, port, rendered)` | Open a TCP connection and write the stream (raw `:9100`). |

Keeping emit separate from render is what makes the same label printable,
saveable, and testable without a printer attached.

## Testing

The pure logic - the millimetre-to-dots conversion, ZPL hex escaping, ITF
validation / padding, and both dialects' exact command output for a sample
label - is unit-tested in the overlay (`modules/label_test.j`). The `send`
`:9100` path is covered against an in-process fake printer in the Go test suite
(`TestLabelSend`).

## Out of scope

- **Two dialects** (`zpl`, `cab`). Adding another is a new encoder plus a
  dialect string, with no change to the build API.
- **Images are by reference only.** `label.image` recalls an image already
  stored on the printer; embedding a bitmap in the job (converting a PNG to the
  dialect raster) is a planned follow-on.
- **No text rotation, single font.** Text rotation and font selection are
  documented follow-ons - the build API extends without breaking. Barcode size,
  check digit, 2D error level, and human-readable line are covered by
  `BarcodeOptions`. The long-tail symbologies (Aztec, MaxiCode, PDF417, the GS1
  DataBar family, ...) are added the same way when needed.
- **Brother ESC/P** is raster/bitmap, not a field command language, so it does
  not fit this vector-field model and is not a planned dialect.

## See also

- [net.md](../libraries/net.md) - the transport `send` uses.
- [fs.md](../libraries/fs.md) - for spooling a rendered label to a file / device.
- [modules/index.md](index.md) - the module catalog and import rules.
