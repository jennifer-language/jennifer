# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# label_cab.j - the cab JScript encoder for the `label` module. This file is
# spliced into label.j via `include` and is not a standalone module: it declares
# no `use` of its own and relies on label.j's imports (strings / convert) and
# its Field / Label structs.
#
# cab JScript references (keep for extending cab support - QR/DataMatrix options,
# fonts, the S sensor type and gap, heat/speed, applicator, etc.):
#   Introduction:      https://www.cab.de/media/pushfile.cfm?file=3962
#   Programming manual: https://www.cab.de/media/pushfile.cfm?file=3047
#
# Grammar used here, from the cab JScript Programming Manual (edition 05/2025).
# The encoder emits the manual's canonical forms (full barcode names, not the
# deprecated one-letter short codes). All values millimetres; no spaces between
# comma-separated parameters; no space after the final `;`; one line per command
# (LF):
#   m m                              measurement in millimetres
#   J {name}                         job start (clear the image buffer); optional name
#   H heat,speed{,mode}              heat/contrast, print speed, trailing mode tokens
#   O orientation                    label orientation (e.g. R = reversed)
#   S {ptype;}xo,yo,ho,dy,wd{,dx}{,col}  label size: sensor type, x/y offset, label
#                                    height, pitch (height+gap), width, and - for
#                                    multi-up dies - the column pitch + column count
#   T x,y,r,font,size;text           text (r = rotation; font 3 = Swiss 721, 5 = Bold, 596 = Monospace; size = "ptN" or mm)
#   B x,y,r,type,size;data           barcode (UPPERCASE type = human-readable line, lowercase = suppressed)
#   G x,y,r;R:w,h,hD,vD              rectangle (hD/vD = horizontal/vertical line thickness)
#   A quantity                       print n copies
# Barcode `size` by type: code128 / EAN-13 / EAN-8 = height,ne; 2 of 5 Interleaved
# = height,ne,ratio; QR = moduleSize (all mm; ne = narrow element). The J/H/O/S
# print-setup lines come from a `label.CabSetup`; the H/O lines are cab-only (no
# ZPL equivalent).

def const BARCODE_NARROW as float init 0.3;   # narrow-element width (mm)
def const ITF_RATIO as int init 3;            # wide:narrow ratio for ratio-oriented linear codes

# cabBarcodeType maps a portable type to a cab symbology name (uppercase, so the
# human-readable line is printed for the linear codes).
func cabBarcodeType(btype as string) {
    if ($btype == "code128") {
        return "CODE128";
    }
    if ($btype == "ean13") {
        return "EAN13";
    }
    if ($btype == "ean8") {
        return "EAN8";
    }
    if ($btype == "itf") {
        return "2OF5INTERLEAVED";
    }
    if ($btype == "code39") {
        return "CODE39";
    }
    if ($btype == "gs1-128") {
        # cab's JScript command token for GS1-128 / UCC-128 is "EAN128".
        return "EAN128";
    }
    if ($btype == "datamatrix") {
        return "DATAMATRIX";
    }
    return "QRCODE";
}

# cabEscape flattens a newline to a space (a newline would terminate the command
# line; the data runs from the final `;` to the end of the line).
func cabEscape(s as string) {
    return strings.replace($s, "\n", " ");
}

# cabBarcodeName builds the cab type token with its options: lowercase suppresses
# the human-readable line, `+MODxx` appends a check digit, `+ELx` sets a 2D error
# level.
func cabBarcodeName(f as Field) {
    def name as string init cabBarcodeType($f.barcodeType);
    if ($f.hideText) {
        $name = strings.lower($name);
    }
    if (not ($f.checkDigit == "")) {
        $name = $name + "+" + strings.upper($f.checkDigit);
    }
    if (not ($f.errorLevel == "")) {
        $name = $name + "+EL" + $f.errorLevel;
    }
    return $name;
}

# cabBarcode renders the barcode `B` command for one field. A 2D code (QR /
# DataMatrix) takes a single module-size value (carried in `h`); the linear codes
# take height,ne, and the ratio-oriented ones (2 of 5, Code 39) also a ratio.
func cabBarcode(f as Field) {
    def head as string init "B " + convert.toString($f.x) + "," + convert.toString($f.y) +
        ",0," + cabBarcodeName($f);
    if ($f.barcodeType == "qr" or $f.barcodeType == "datamatrix") {
        return $head + "," + convert.toString($f.h) + ";" + $f.data;
    }
    def ne as float init BARCODE_NARROW;
    if ($f.moduleWidth > 0.0) {
        $ne = $f.moduleWidth;
    }
    def size as string init "," + convert.toString($f.h) + "," + convert.toString($ne);
    if ($f.barcodeType == "itf" or $f.barcodeType == "code39") {
        def ratioStr as string init convert.toString(ITF_RATIO);
        if ($f.ratio > 0.0) {
            $ratioStr = convert.toString($f.ratio);
        }
        $size = $size + "," + $ratioStr;
    }
    return $head + $size + ";" + $f.data;
}

# cabField renders one field as a cab JScript command.
func cabField(f as Field) {
    def x as string init convert.toString($f.x);
    def y as string init convert.toString($f.y);
    if ($f.kind == "text") {
        # font 3 = Swiss 721, 5 = Swiss 721 Bold; a point size is written "ptN",
        # otherwise the size is a millimetre height.
        def font as string init "3";
        if ($f.bold) {
            $font = "5";
        }
        def size as string init convert.toString($f.h);
        if ($f.points > 0) {
            $size = "pt" + convert.toString($f.points);
        }
        return "T " + $x + "," + $y + "," + convert.toString($f.rotation) + "," + $font + "," + $size + ";" + cabEscape($f.data);
    }
    if ($f.kind == "box") {
        return "G " + $x + "," + $y + ",0;R:" + convert.toString($f.w) + "," +
            convert.toString($f.h) + "," + convert.toString($f.thickness) + "," +
            convert.toString($f.thickness);
    }
    if ($f.kind == "image") {
        # Autoload a stored image (mag 1,1) from the printer's images/ folder.
        return "I " + $x + "," + $y + ",0,1,1,a;" + $f.data;
    }
    return cabBarcode($f);
}

# renderCab renders a whole label as a cab JScript job. `setup` carries the
# cab-only print-setup (job name, heat/speed, orientation, sensor, S geometry); a
# zero-value setup emits a bare `J`, no `H`/`O`, and an `S` line derived from the
# label size with gap 0 (dy = height - adjust the geometry if your media has a
# gap between labels).
func renderCab(label as Label, setup as CabSetup) {
    def out as string init "m m\nJ";
    if (not ($setup.jobName == "")) {
        $out = $out + " " + $setup.jobName;
    }
    $out = $out + "\n";
    # H (heat/speed): emitted only when something is set.
    if (not ($setup.mode == "") or not ($setup.heat == 0) or not ($setup.speed == 0)) {
        $out = $out + "H " + convert.toString($setup.heat) + "," + convert.toString($setup.speed);
        if (not ($setup.mode == "")) {
            $out = $out + "," + $setup.mode;
        }
        $out = $out + "\n";
    }
    # O (orientation): emitted only when set.
    if (not ($setup.orientation == "")) {
        $out = $out + "O " + $setup.orientation + "\n";
    }
    # S (label geometry): an explicit width means "use these dimensions"; width 0
    # derives a single-up, gap-0 geometry from the label size. The optional column
    # pitch + count are emitted only for a multi-up die (columns > 1).
    $out = $out + "S ";
    if (not ($setup.sensor == "")) {
        $out = $out + $setup.sensor + ";";
    }
    if ($setup.width == 0.0) {
        $out = $out + "0,0," + convert.toString($label.height) + "," +
            convert.toString($label.height) + "," + convert.toString($label.width);
    } else {
        $out = $out + convert.toString($setup.xOffset) + "," + convert.toString($setup.yOffset) +
            "," + convert.toString($setup.height) + "," + convert.toString($setup.pitch) +
            "," + convert.toString($setup.width);
        if ($setup.columns > 1) {
            $out = $out + "," + convert.toString($setup.columnPitch) + "," + convert.toString($setup.columns);
        }
    }
    $out = $out + "\n";
    for (def f in $label.fields) {
        $out = $out + cabField($f) + "\n";
    }
    return $out + "A " + convert.toString($label.quantity) + "\n";
}
