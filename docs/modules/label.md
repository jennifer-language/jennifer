# `label` - industrial label printing

Import with `import "label.j" as label;`. Describe and print labels for
industrial label printers. One module covers every printer: the printer
language is a selectable backend (a `Device` dialect). A three-stage pipeline
keeps the stages independent:

1. **build** a device-independent `Label` in millimetres,
2. **render** it to a chosen dialect string, and
3. **emit** that string anywhere.

Build and render are pure text and run on both binaries; only `send` (the
`:9100` convenience) uses `net`, so it needs the default `jennifer` binary.

```jennifer
import "label.j" as label;

def l as label.Label init label.new(50.0, 30.0);          # 50 x 30 mm
def t as label.TextOptions; $t.height = 4.0;
$l = label.text($l, 5.0, 5.0, $t, "HELLO");
def o as label.BarcodeOptions;                            # zero-value = defaults
$l = label.barcode($l, 5.0, 15.0, "code128", $o, "12345678");
def zpl as string init label.render($l, label.zpl(203));
# label.send("192.168.1.50", 9100, $zpl);                 # to a printer's raw port
```

Runnable: [`examples/modules/label_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/label_demo.j).

## Stage 1 - build (device-independent, millimetres)

Every builder is value-semantic and returns a new `Label`, so a label is
assembled by reassignment. Coordinates and sizes are millimetres, not device
dots.

| Call / type                                       | Notes                                             |
| ------------------------------------------------- | ------------------------------------------------- |
| `label.Label`                                     | `width`, `height` (mm), `quantity`, `fields`.     |
| `label.Field`                                     | one placed field (`kind` "text"/"barcode"/"box"/"image"). |
| `label.TextOptions`                               | `height` (mm), `points` (pt, wins over height), `rotation` (0/90/180/270), `bold`. |
| `label.new(width, height)`                        | A new empty label of that size in mm (quantity 1). |
| `label.text(label, x, y, opts, content)`          | Place text at (x, y); `opts` is a `TextOptions`.  |
| `label.barcode(label, x, y, type, opts, data)`    | Place a barcode (symbologies + `opts` below).     |
| `label.box(label, x, y, w, h, thickness)`         | Place a rectangular outline (all mm).             |
| `label.image(label, x, y, name)`                  | Place a pre-stored image by name (native size).   |
| `label.quantity(label, n)`                        | Set the number of copies.                         |

`TextOptions` is zero-value-friendly (`def t as label.TextOptions;`): an
unrotated, non-bold field sized by `height` mm. Set `points` for a point-sized
font (it wins over `height`), `rotation` to turn it (degrees counter-clockwise),
and `bold` for a bold face. Both dialects honour rotation and point size (cab via the `r` / `ptN` fields, ZPL via the field-orientation letter
and a dots conversion).

Barcode `type` is a **linear** symbology - `"code128"`, `"ean13"`, `"ean8"`,
`"itf"` (Interleaved 2 of 5), `"code39"`, `"gs1-128"` - or a **2D** symbology -
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
| `moduleWidth` (float, mm) | A linear code's narrow-element width; `0` uses the dialect default (cab writes it as `ne`; ZPL uses its default `^BY` module width). |
| `ratio` (float) | The wide:narrow bar ratio for a ratio-based code (Interleaved 2 of 5 / Code 39); `0` uses the default (3). |

**ITF** (Interleaved 2 of 5, the standard shipping-carton symbology) is
numeric-only and even-length because the encoding pairs digits: `label.barcode`
rejects non-numeric ITF data (a catchable `Error`, kind `"label"`) and pads
odd-length data with a leading zero (so a 13-digit body becomes ITF-14). An
unknown barcode `type` also throws.

## Stage 2 - render (to a dialect)

`label.render(label, device)` returns the command stream for the device's
dialect as a plain string. Build the `device` with a constructor rather than a
raw literal:

| Constructor | Notes |
| ----------- | ----- |
| `label.zpl(dpi)` | A ZPL target at the given printer resolution. |
| `label.cab()` | A cab target with default print-setup. |
| `label.cabWith(setup)` | A cab target carrying an explicit `label.CabSetup`. |

The resolution converts millimetres to dots for raster dialects; millimetre-
native dialects ignore it. An unknown dialect throws (kind `"label"`).

- **`"zpl"` - Zebra Programming Language.** The dominant, public label
  language; cab Squix printers accept it too, so this one dialect drives most
  hardware. Emits `^XA` / `^FO` / `^A0` / `^FD` / `^FS`, `^BY` / `^BC` (with
  `^BE` for EAN-13, `^B8` for EAN-8, `^B2` for ITF, `^BQ` for QR), `^GB`, `^PQ`,
  `^XZ`, converting millimetres to dots at the target `dpi`. The `^A0`
  orientation letter carries text rotation; a point size converts to dots. Text
  is escaped via `^FH` hex for the ZPL command characters (`^`, `~`, `_`) and
  non-ASCII bytes.
- **`"cab"` - cab JScript.** The native language of cab printers,
  millimetre-native (it ignores `dpi`). Emits `m` / `J` / `H` / `O` / `S` / `T`
  / `B` / `G` / `A` per the cab JScript Programming Manual (edition 05/2025):
  `T x,y,r,font,size;text` (`r` = rotation, font 3 = Swiss 721 / 5 = Bold, size =
  `ptN` or mm), `G x,y,r;R:w,h,hD,vD` for a box, and `B x,y,r,type,size;data`
  where an **uppercase** type name prints the human-readable line and a
  **lowercase** one suppresses it, with size `height,ne` for Code 128 / EAN-13 /
  EAN-8, `height,ne,ratio` for Interleaved 2 of 5, and a single module size for
  QR.

### cab print-setup (`CabSetup`)

The `J` (job name), `H` (heat/speed), `O` (orientation), and `S` (label sensor +
geometry) lines are printer/media setup with no ZPL equivalent, so they live in a
cab-only `label.CabSetup` passed via `label.cabWith(setup)`. Every field is
optional; the encoder emits a command only when the matching field is set, and a
zero-value setup (`label.cab()`) emits a bare `J`, no `H`/`O`, and an `S` line
derived from the label size. ZPL ignores the struct entirely.

| Field | cab line |
| ----- | -------- |
| `jobName` (string) | `J <name>` (bare `J` when empty). |
| `heat` (int), `speed` (int), `mode` (string) | `H <heat>,<speed>,<mode>` (omitted when all are zero/empty). |
| `orientation` (string) | `O <orientation>` (e.g. `"R"`; omitted when empty). |
| `sensor` (string) | the `S` photocell/sensor type (e.g. `"l1"` = die-cut with gap -> `S l1;...`). |
| `xOffset`, `yOffset` (float, mm) | the `S` horizontal / vertical origin offset. |
| `height` (float, mm) | the `S` label height (transport direction). **`width` 0 derives the whole `S` line from the label size.** |
| `pitch` (float, mm) | the `S` label pitch = label height + the gap between labels. |
| `width` (float, mm) | the `S` label width. |
| `columnPitch` (float, mm), `columns` (int) | multi-up dies: the horizontal column pitch and label count; appended only when `columns > 1`. |

The `S` line is `S [sensor;]xo,yo,ho,dy,wd[,dx,col]`. So a 4-up die of 17 x 12 mm
die-cut labels with a 3 mm gap (pitch 15) at a 20 mm column pitch is:

```jennifer
def setup as label.CabSetup;
$setup.jobName = "Shipping";
$setup.heat = 100; $setup.speed = 5; $setup.mode = "T,R0";
$setup.orientation = "R";
$setup.sensor = "l1";
$setup.height = 12.0; $setup.pitch = 15.0; $setup.width = 17.0;
$setup.columnPitch = 20.0; $setup.columns = 4;   # -> S l1;0.0,0.0,12.0,15.0,17.0,20.0,4
def job as string init label.render($l, label.cabWith($setup));
```

> **cab dialect note.** The encoder follows the cab JScript Programming Manual
> (edition 05/2025) and emits the manual's canonical forms - full barcode names
> rather than the deprecated one-letter short codes. The derived `S` label-size
> line uses gap 0 (`dy = height`); set the `CabSetup` `height` / `pitch` /
> `width` (and `columnPitch` / `columns` for a multi-up die) to match your media.

## Stage 3 - emit (transport-agnostic)

The rendered string is yours to deliver: write it to a `*.prom`-style spool file
or a USB device node with `fs`, store it, or send it over the network. The
module ships one convenience for the common case:

| Call                              | Notes                                                     |
| --------------------------------- | --------------------------------------------------------- |
| `label.send(host, port, rendered)` | Open a TCP connection and write the stream (raw `:9100`). |

Because emit is separate from render, a label can be printed, saved, or tested
without a printer attached.

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
- **Text rotation, point sizes, and a bold face** are covered by `TextOptions`;
  barcode size, narrow-element width, check digit, 2D error level, and the
  human-readable line by `BarcodeOptions`; cab print-setup by `CabSetup`. Full
  font selection beyond regular/bold is still a follow-on. The long-tail
  symbologies (Aztec, MaxiCode, PDF417, the GS1 DataBar family, ...) are added
  the same way when needed.
- **Brother ESC/P** is raster/bitmap, not a field command language, so it does
  not fit this vector-field model and is not a planned dialect.

## See also

- [net.md](../libraries/net.md) - the transport `send` uses.
- [fs.md](../libraries/fs.md) - for spooling a rendered label to a file / device.
- [modules/index.md](index.md) - the module catalog and import rules.
