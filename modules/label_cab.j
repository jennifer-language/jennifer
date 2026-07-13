# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
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
# Grammar used here, from the cab JScript Programming Manual (edition 05/2025),
# cross-checked against a real cabLabel-generated job. All values millimetres;
# no spaces between comma-separated parameters; no space after the final `;`;
# one line per command (LF):
#   m m                              measurement in millimetres
#   J                                job start (clear the image buffer)
#   S {ptype;}xo,yo,ho,dy,wd         label size: offsets, height, pitch (height+gap), width
#   T x,y,r,font,size;text           text (font 3 = Swiss 721, 5 = Bold, 596 = Monospace)
#   B x,y,r,type,size;data           barcode (UPPERCASE type = human-readable line)
#   G x,y,r;R:w,h,hD,vD              rectangle (hD/vD = horizontal/vertical line thickness)
#   A quantity                       print n copies
# Barcode `size` by type: code128 / EAN-13 = height,ne; 2 of 5 Interleaved =
# height,ne,ratio; QR = moduleSize (all mm; ne = narrow element).

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
    def size as string init "," + convert.toString($f.h) + "," + convert.toString(BARCODE_NARROW);
    if ($f.barcodeType == "itf" or $f.barcodeType == "code39") {
        $size = $size + "," + convert.toString(ITF_RATIO);
    }
    return $head + $size + ";" + $f.data;
}

# cabField renders one field as a cab JScript command.
func cabField(f as Field) {
    def x as string init convert.toString($f.x);
    def y as string init convert.toString($f.y);
    if ($f.kind == "text") {
        return "T " + $x + "," + $y + ",0,3," + convert.toString($f.h) + ";" + cabEscape($f.data);
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

# renderCab renders a whole label as a cab JScript job. The `S` line uses gap 0
# (dy = height); adjust the pitch to your media if labels have a gap between them.
func renderCab(label as Label) {
    def out as string init "m m\nJ\n";
    $out = $out + "S 0,0," + convert.toString($label.height) + "," +
        convert.toString($label.height) + "," + convert.toString($label.width) + "\n";
    for (def f in $label.fields) {
        $out = $out + cabField($f) + "\n";
    }
    return $out + "A " + convert.toString($label.quantity) + "\n";
}
