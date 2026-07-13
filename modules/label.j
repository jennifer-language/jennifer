# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

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
 * $l = label.text($l, 5.0, 5.0, label.TextOptions{height: 4.0}, "HELLO");
 * def o as label.BarcodeOptions;
 * $l = label.barcode($l, 5.0, 15.0, "code128", $o, "12345678");
 * io.printf("%s", label.render($l, label.Device{dialect: "zpl", dpi: 203}));
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
    hideText as bool
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
 * Options for a text field.
 * @field height {float} the font height in millimetres
 */
export def struct TextOptions {
    height as float
};

/**
 * The render target: which dialect to emit and, for raster dialects, the
 * printer resolution.
 * @field dialect {string} "zpl" or "cab"
 * @field dpi {int} the printer dots-per-inch (used by "zpl"; ignored by "cab")
 */
export def struct Device {
    dialect as string,
    dpi as int
};

/**
 * Optional refinements for a barcode. A zero-value struct
 * (`def o as label.BarcodeOptions;`) means no options: default size, no added
 * check digit, default error correction, human-readable line shown.
 * @field height {float} bar height (linear) or module size (2D) in millimetres; 0 uses the default
 * @field checkDigit {string} append an auto-computed check digit: "" (none), "mod10", "mod11", "mod16", "mod36", or "mod43"
 * @field errorLevel {string} 2D error-correction level: "" (default), "L", "M", "Q", or "H"
 * @field hideText {bool} true suppresses a linear barcode's human-readable line
 */
export def struct BarcodeOptions {
    height as float,
    checkDigit as string,
    errorLevel as string,
    hideText as bool
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
    def f as Field init Field{ kind: "text", x: $x, y: $y, w: 0.0, h: $opts.height, thickness: 0.0, barcodeType: "", data: $content, checkDigit: "", errorLevel: "", hideText: false };
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
 * "code128", "ean13", "itf" (Interleaved 2 of 5), "code39", "gs1-128" - or a 2D
 * symbology - "datamatrix", "qr". `opts` refines it (size, check digit, 2D error
 * level, human-readable line); pass a zero-value `BarcodeOptions` for the
 * defaults. ITF is numeric-only and even-length (its digits are paired):
 * non-numeric data is rejected and odd-length data is padded with a leading zero
 * (so a 13-digit body becomes ITF-14). GS1-128 data uses the parenthesised
 * Application Identifier form, e.g. `(00)3006...`.
 * @param label {Label} the label to extend
 * @param x {float} the x origin in millimetres
 * @param y {float} the y origin in millimetres
 * @param type {string} the barcode symbology ("code128"/"ean13"/"itf"/"code39"/"gs1-128"/"datamatrix"/"qr")
 * @param opts {BarcodeOptions} size / check-digit / error-level / text refinements
 * @param data {string} the barcode data
 * @return {Label} a new Label with the barcode field added
 * @throws {Error} kind "label" for an unknown type or invalid ITF data
 */
export func barcode(label as Label, x as float, y as float, type as string, opts as BarcodeOptions, data as string) {
    if (not ($type == "code128" or $type == "ean13" or $type == "itf" or $type == "code39" or
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
    def f as Field init Field{ kind: "barcode", x: $x, y: $y, w: 0.0, h: $size, thickness: 0.0, barcodeType: $type, data: $d, checkDigit: $opts.checkDigit, errorLevel: $opts.errorLevel, hideText: $opts.hideText };
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
    def f as Field init Field{ kind: "image", x: $x, y: $y, w: 0.0, h: 0.0, thickness: 0.0, barcodeType: "", data: $name, checkDigit: "", errorLevel: "", hideText: false };
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
    def f as Field init Field{ kind: "box", x: $x, y: $y, w: $w, h: $h, thickness: $thickness, barcodeType: "", data: "", checkDigit: "", errorLevel: "", hideText: false };
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
        return renderCab($label);
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
    net.writeBytes($conn, convert.bytesFromString($rendered, "utf-8"));
    net.close($conn);
    return null;
}
