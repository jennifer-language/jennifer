# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Describe and print labels for industrial label printers. One module, one way
 * to describe a label, with the printer language as a **selectable backend** (a
 * `Device` dialect), not a module per printer. A three-stage pipeline keeps the
 * stages independent: **build** a device-independent `Label` in millimetres,
 * **render** it to a chosen dialect string, then **emit** that string anywhere
 * (a file, a database, or the thin `send` convenience over `net` to a printer's
 * `:9100` raw port). Dialects: `"zpl"` (Zebra Programming Language, raster -
 * needs the target `dpi`) and `"cab"` (cab JScript, millimetre-native); each
 * dialect encoder lives in its own file (`label_zpl.j` / `label_cab.j`) spliced
 * in via `include`, so a new dialect is a new file plus a branch in `render`.
 * Build and render are pure and run on both binaries; only `send` needs the
 * default `jennifer` binary.
 * @module label
 * @example
 * def l as label.Label init label.new(50.0, 30.0);
 * def t as label.TextOptions; $t.height = 4.0;
 * $l = label.text($l, 5.0, 5.0, $t, "HELLO");
 * def o as label.BarcodeOptions;
 * $l = label.barcode($l, 5.0, 15.0, "code128", $o, "12345678");
 * io.printf("%s", label.render($l, label.zpl(203)));
 */
use lists;
use math;
use strings;
use convert;
use net;

# Default linear-barcode height and 2D-code module size, in millimetres (the
# barcode builder takes no size; a follow-on may add a BarcodeOptions). For a 2D
# symbology (qr / datamatrix) the field's `h` carries the module size instead of
# a bar height.
def const DEFAULT_BARCODE_HEIGHT as float init 15.0;
def const DEFAULT_MODULE_SIZE as float init 1.0;

# --- types ------------------------------------------------------------------

/**
 * One field on a label. `kind` selects which attributes matter: "text" uses
 * `h` as the font height and `data` as the content; "barcode" uses
 * `barcodeType`, `h` (bar height, or module size for a 2D code), and `data`;
 * "box" uses `w`, `h`, `thickness`; "image" uses `data` as the stored name.
 * @field kind {string} "text", "barcode", "box", or "image"
 * @field x {float} the x origin in millimetres
 * @field y {float} the y origin in millimetres
 * @field w {float} the box width in millimetres (0 otherwise)
 * @field h {float} box height / barcode height (or 2D module size) / text font height, in millimetres
 * @field thickness {float} the box line thickness in millimetres (0 otherwise)
 * @field barcodeType {string} the symbology for a barcode field (empty otherwise)
 * @field data {string} the text content, barcode data, or image name
 * @field checkDigit {string} a barcode's auto-computed check digit ("" | "mod10" | ...)
 * @field errorLevel {string} a 2D barcode's error-correction level ("" | "L" | "M" | "Q" | "H")
 * @field hideText {bool} suppress a linear barcode's human-readable line
 * @field rotation {int} text rotation in degrees counter-clockwise (0, 90, 180, 270)
 * @field points {int} text font size in points; 0 means use `h` as a millimetre height
 * @field bold {bool} bold text face
 * @field moduleWidth {float} a linear barcode's narrow-element width in millimetres (0 = dialect default)
 * @field ratio {float} a ratio-based barcode's wide:narrow ratio (0 = dialect default)
 */
export def struct Field {
    kind as string,
    x as float,
    y as float,
    w as float,
    h as float,
    thickness as float,
    barcodeType as string,
    data as string,
    checkDigit as string,
    errorLevel as string,
    hideText as bool,
    rotation as int,
    points as int,
    bold as bool,
    moduleWidth as float,
    ratio as float
};

/**
 * A device-independent label: its physical size in millimetres, the number of
 * copies to print, and its fields. Value-semantic - every builder returns a new
 * Label.
 * @field width {float} the label width in millimetres
 * @field height {float} the label height in millimetres
 * @field quantity {int} the number of copies to print
 * @field fields {list of Field} the placed fields
 */
export def struct Label {
    width as float,
    height as float,
    quantity as int,
    fields as list of Field
};

/**
 * Options for a text field. A zero-value struct
 * (`def o as label.TextOptions;`) means an unrotated, non-bold field sized by
 * `height`. Set `points` for a point-sized font (it wins over `height`).
 * @field height {float} the font height in millimetres (used when `points` is 0)
 * @field points {int} the font size in points; when > 0 it is used instead of `height`
 * @field rotation {int} rotation in degrees counter-clockwise: 0, 90, 180, or 270
 * @field bold {bool} true selects a bold face
 */
export def struct TextOptions {
    height as float,
    points as int,
    rotation as int,
    bold as bool
};

/**
 * cab-only print-setup that has no ZPL equivalent, carried on the render target
 * so it can be reproduced without leaking into the device-independent `Label`. A
 * zero-value struct emits none of it (a bare `J`, no `H`/`O` line, and an `S`
 * line derived from the label size). Every field is optional; the cab encoder
 * emits a command only when the corresponding field is set. ZPL ignores it all.
 * @field jobName {string} the `J` job name; "" emits a bare `J`
 * @field heat {int} the `H` heat/contrast level
 * @field speed {int} the `H` print speed
 * @field mode {string} the trailing `H` tokens verbatim, e.g. "T,R0"; "" and heat/speed 0 omit the `H` line
 * @field orientation {string} the `O` orientation token, e.g. "R"; "" omits the `O` line
 * @field sensor {string} the `S` photocell/sensor type, e.g. "l1" (die-cut labels with gap); "" emits no prefix
 * @field xOffset {float} the `S` horizontal origin offset in millimetres
 * @field yOffset {float} the `S` vertical origin offset in millimetres
 * @field height {float} the `S` label height in millimetres (transport direction); 0 derives the whole `S` line from the label size
 * @field pitch {float} the `S` label pitch in millimetres (label height + gap between labels)
 * @field width {float} the `S` label width in millimetres
 * @field columnPitch {float} the `S` horizontal distance to the next column in millimetres (multi-up dies)
 * @field columns {int} the `S` number of labels across; <= 1 emits a single-up geometry
 */
export def struct CabSetup {
    jobName as string,
    heat as int,
    speed as int,
    mode as string,
    orientation as string,
    sensor as string,
    xOffset as float,
    yOffset as float,
    height as float,
    pitch as float,
    width as float,
    columnPitch as float,
    columns as int
};

/**
 * The render target: which dialect to emit and, for raster dialects, the
 * printer resolution. Build one with `label.zpl(dpi)`, `label.cab()`, or
 * `label.cabWith(setup)` rather than a raw literal.
 * @field dialect {string} "zpl" or "cab"
 * @field dpi {int} the printer dots-per-inch (used by "zpl"; ignored by "cab")
 * @field cab {CabSetup} cab-only print-setup (ignored by "zpl")
 */
export def struct Device {
    dialect as string,
    dpi as int,
    cab as CabSetup
};

# noCab returns a zero-value CabSetup (emit no cab-specific setup).
func noCab() {
    return CabSetup{ jobName: "", heat: 0, speed: 0, mode: "", orientation: "", sensor: "", xOffset: 0.0, yOffset: 0.0, height: 0.0, pitch: 0.0, width: 0.0, columnPitch: 0.0, columns: 0 };
}

/**
 * Build a ZPL render target for a printer of the given resolution.
 * @param dpi {int} the printer dots-per-inch
 * @return {Device} a ZPL device
 */
export func zpl(dpi as int) {
    return Device{ dialect: "zpl", dpi: $dpi, cab: noCab() };
}

/**
 * Build a cab JScript render target with default print-setup.
 * @return {Device} a cab device
 */
export func cab() {
    return Device{ dialect: "cab", dpi: 0, cab: noCab() };
}

/**
 * Build a cab JScript render target carrying explicit cab print-setup.
 * @param setup {CabSetup} the cab-only print-setup (job name, heat, orientation, sensor, geometry)
 * @return {Device} a cab device
 */
export func cabWith(setup as CabSetup) {
    return Device{ dialect: "cab", dpi: 0, cab: $setup };
}

/**
 * Optional refinements for a barcode. A zero-value struct
 * (`def o as label.BarcodeOptions;`) means no options: default size, no added
 * check digit, default error correction, human-readable line shown.
 * @field height {float} bar height (linear) or module size (2D) in millimetres; 0 uses the default
 * @field checkDigit {string} append an auto-computed check digit: "" (none), "mod10", "mod11", "mod16", "mod36", or "mod43"
 * @field errorLevel {string} 2D error-correction level: "" (default), "L", "M", "Q", or "H"
 * @field hideText {bool} true suppresses a linear barcode's human-readable line
 * @field moduleWidth {float} a linear barcode's narrow-element width in millimetres; 0 uses the dialect default (cab; zpl uses its own default module width)
 * @field ratio {float} the wide:narrow bar ratio for a ratio-based code (Interleaved 2 of 5 / Code 39); 0 uses the default (3)
 */
export def struct BarcodeOptions {
    height as float,
    checkDigit as string,
    errorLevel as string,
    hideText as bool,
    moduleWidth as float,
    ratio as float
};

# --- build (exported) -------------------------------------------------------

/**
 * Start a new, empty label of the given size in millimetres (quantity 1).
 * @param width {float} the label width in millimetres
 * @param height {float} the label height in millimetres
 * @return {Label} a fresh, empty label
 */
export func new(width as float, height as float) {
    def fs as list of Field init [];
    return Label{ width: $width, height: $height, quantity: 1, fields: $fs };
}

/**
 * Place a text field, returning a new Label.
 * @param label {Label} the label to extend
 * @param x {float} the x origin in millimetres
 * @param y {float} the y origin in millimetres
 * @param opts {TextOptions} the text options (font height)
 * @param content {string} the text to print
 * @return {Label} a new Label with the text field added
 */
export func text(label as Label, x as float, y as float, opts as TextOptions, content as string) {
    def f as Field init Field{ kind: "text", x: $x, y: $y, w: 0.0, h: $opts.height, thickness: 0.0, barcodeType: "", data: $content, checkDigit: "", errorLevel: "", hideText: false, rotation: $opts.rotation, points: $opts.points, bold: $opts.bold, moduleWidth: 0.0, ratio: 0.0 };
    def out as Label init $label;
    $out.fields = lists.push($out.fields, $f);
    return $out;
}

# isDigits reports whether s is a non-empty run of ASCII digits.
func isDigits(s as string) {
    def raw as bytes init convert.bytesFromString($s, "utf-8");
    if (len($raw) == 0) {
        return false;
    }
    def i as int init 0;
    while ($i < len($raw)) {
        if ($raw[$i] < 48 or $raw[$i] > 57) {
            return false;
        }
        $i = $i + 1;
    }
    return true;
}

/**
 * Place a barcode field, returning a new Label. `type` is a linear symbology -
 * "code128", "ean13", "ean8", "itf" (Interleaved 2 of 5), "code39", "gs1-128" -
 * or a 2D symbology - "datamatrix", "qr". `opts` refines it (size, check digit, 2D error
 * level, human-readable line); pass a zero-value `BarcodeOptions` for the
 * defaults. ITF is numeric-only and even-length (its digits are paired):
 * non-numeric data is rejected and odd-length data is padded with a leading zero
 * (so a 13-digit body becomes ITF-14). GS1-128 data uses the parenthesised
 * Application Identifier form, e.g. `(00)3006...`.
 * @param label {Label} the label to extend
 * @param x {float} the x origin in millimetres
 * @param y {float} the y origin in millimetres
 * @param type {string} the barcode symbology ("code128"/"ean13"/"ean8"/"itf"/"code39"/"gs1-128"/"datamatrix"/"qr")
 * @param opts {BarcodeOptions} size / check-digit / error-level / text refinements
 * @param data {string} the barcode data
 * @return {Label} a new Label with the barcode field added
 * @throws {Error} kind "label" for an unknown type or invalid ITF data
 */
export func barcode(label as Label, x as float, y as float, type as string, opts as BarcodeOptions, data as string) {
    if (not ($type == "code128" or $type == "ean13" or $type == "ean8" or $type == "itf" or $type == "code39" or
        $type == "gs1-128" or $type == "datamatrix" or $type == "qr")) {
        throw Error{ kind: "label", message: "label: unknown barcode type: " + $type, file: "", line: 0, col: 0 };
    }
    def d as string init $data;
    if ($type == "itf") {
        if (not isDigits($d)) {
            throw Error{ kind: "label", message: "label: ITF barcode data must be numeric: " + $d, file: "", line: 0, col: 0 };
        }
        if (len($d) % 2 == 1) {
            $d = "0" + $d;
        }
    }
    # A 2D symbology carries a module size in `h`; a linear one a bar height. An
    # explicit opts.height (mm) overrides the default.
    def size as float init DEFAULT_BARCODE_HEIGHT;
    if ($type == "qr" or $type == "datamatrix") {
        $size = DEFAULT_MODULE_SIZE;
    }
    if ($opts.height > 0.0) {
        $size = $opts.height;
    }
    def f as Field init Field{ kind: "barcode", x: $x, y: $y, w: 0.0, h: $size, thickness: 0.0, barcodeType: $type, data: $d, checkDigit: $opts.checkDigit, errorLevel: $opts.errorLevel, hideText: $opts.hideText, rotation: 0, points: 0, bold: false, moduleWidth: $opts.moduleWidth, ratio: $opts.ratio };
    def out as Label init $label;
    $out.fields = lists.push($out.fields, $f);
    return $out;
}

/**
 * Place an image referenced by name, returning a new Label. The image must be
 * pre-stored on the printer (cab: the `images/` folder; ZPL: a stored graphic);
 * `name` is the stored name in that dialect's convention. Printed at native
 * size. (Embedding a bitmap in the job is a planned follow-on.)
 * @param label {Label} the label to extend
 * @param x {float} the x origin in millimetres
 * @param y {float} the y origin in millimetres
 * @param name {string} the stored image name
 * @return {Label} a new Label with the image field added
 */
export func image(label as Label, x as float, y as float, name as string) {
    def f as Field init Field{ kind: "image", x: $x, y: $y, w: 0.0, h: 0.0, thickness: 0.0, barcodeType: "", data: $name, checkDigit: "", errorLevel: "", hideText: false, rotation: 0, points: 0, bold: false, moduleWidth: 0.0, ratio: 0.0 };
    def out as Label init $label;
    $out.fields = lists.push($out.fields, $f);
    return $out;
}

/**
 * Place a rectangular box (outline), returning a new Label.
 * @param label {Label} the label to extend
 * @param x {float} the x origin in millimetres
 * @param y {float} the y origin in millimetres
 * @param w {float} the box width in millimetres
 * @param h {float} the box height in millimetres
 * @param thickness {float} the line thickness in millimetres
 * @return {Label} a new Label with the box added
 */
export func box(label as Label, x as float, y as float, w as float, h as float, thickness as float) {
    def f as Field init Field{ kind: "box", x: $x, y: $y, w: $w, h: $h, thickness: $thickness, barcodeType: "", data: "", checkDigit: "", errorLevel: "", hideText: false, rotation: 0, points: 0, bold: false, moduleWidth: 0.0, ratio: 0.0 };
    def out as Label init $label;
    $out.fields = lists.push($out.fields, $f);
    return $out;
}

/**
 * Set the number of copies to print, returning a new Label.
 * @param label {Label} the label to update
 * @param n {int} the number of copies
 * @return {Label} a new Label with the quantity set
 */
export func quantity(label as Label, n as int) {
    def out as Label init $label;
    $out.quantity = $n;
    return $out;
}

# --- render (exported) ------------------------------------------------------

/**
 * Render a label to a dialect command string.
 * @param label {Label} the label to render
 * @param device {Device} the target dialect (and dpi for raster dialects)
 * @return {string} the printer command stream
 * @throws {Error} kind "label" for an unknown dialect
 */
export func render(label as Label, device as Device) {
    if ($device.dialect == "zpl") {
        return renderZpl($label, $device.dpi);
    }
    if ($device.dialect == "cab") {
        return renderCab($label, $device.cab);
    }
    throw Error{ kind: "label", message: "label: unknown dialect: " + $device.dialect, file: "", line: 0, col: 0 };
}

# --- dialect encoders (each in its own file, spliced in here) ---------------
#
# A new printer language is a new file plus a branch in `render` above, with no
# change to the build API. The included files declare no `use` of their own -
# they rely on this file's imports and structs.
include "label_zpl.j";
include "label_cab.j";

# --- emit (exported; needs the default binary via net) ----------------------

/**
 * Send a rendered command stream to a printer's raw `:9100` port over TCP.
 * @param host {string} the printer host or IP
 * @param port {int} the raw print port (usually 9100)
 * @param rendered {string} the rendered command stream from `render`
 * @throws {Error} on a network failure (a positioned `net` error)
 */
export func send(host as string, port as int, rendered as string) {
    def conn as net.Conn init net.connect($host + ":" + convert.toString($port));
    defer net.close($conn);              # closed even when the write throws
    net.writeBytes($conn, convert.bytesFromString($rendered, "utf-8"));
    return null;
}
