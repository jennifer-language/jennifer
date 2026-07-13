# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# label_zpl.j - the ZPL (Zebra Programming Language) encoder for the `label`
# module. This file is spliced into label.j via `include` and is not a
# standalone module: it declares no `use` of its own and relies on label.j's
# imports (math / strings / convert) and its Field / Label structs.
#
# ZPL II reference: https://www.zebra.com (public ZPL II Programming Guide).

# mmToDots converts millimetres to printer dots at the given dpi (rounded).
func mmToDots(mm as float, dpi as int) {
    return math.round($mm * $dpi / 25.4);
}

# hexByte renders one byte as two uppercase hex digits.
func hexByte(b as int) {
    def digits as string init "0123456789ABCDEF";
    return strings.substring($digits, $b // 16, $b // 16 + 1) +
        strings.substring($digits, $b % 16, $b % 16 + 1);
}

# zplEscape hex-escapes the ZPL command characters (^ ~ _) and any non-printable
# or non-ASCII byte as `_XX`, to be used after a `^FH` field-hex indicator.
func zplEscape(s as string) {
    def raw as bytes init convert.bytesFromString($s, "utf-8");
    def out as string init "";
    def i as int init 0;
    while ($i < len($raw)) {
        def b as int init $raw[$i];
        if ($b == 94 or $b == 126 or $b == 95 or $b < 32 or $b > 126) {
            $out = $out + "_" + hexByte($b);
        } else {
            $out = $out + convert.fromCodepoint($b);
        }
        $i = $i + 1;
    }
    return $out;
}

# zplBarcode renders the barcode command body (after the ^FO origin). `h` is the
# bar height in dots for linear codes, or the module size in dots for 2D codes.
# Options: `hideText` toggles the human-readable interpretation line, `checkDigit`
# turns on a symbology's native check digit (Code 39 / ITF; other codes carry it
# in the data), `errorLevel` sets the QR error-correction level.
func zplBarcode(f as Field, h as int) {
    def hs as string init convert.toString($h);
    def btype as string init $f.barcodeType;
    def data as string init $f.data;
    def hri as string init "Y";
    if ($f.hideText) {
        $hri = "N";
    }
    def chk as string init "N";
    if (not ($f.checkDigit == "")) {
        $chk = "Y";
    }
    if ($btype == "code128") {
        return "^BY2^BCN," + $hs + "," + $hri + ",N,N^FD" + $data + "^FS";
    }
    if ($btype == "ean13") {
        return "^BEN," + $hs + "," + $hri + ",N^FD" + $data + "^FS";
    }
    if ($btype == "itf") {
        return "^BY2^B2N," + $hs + "," + $hri + ",N," + $chk + "^FD" + $data + "^FS";
    }
    if ($btype == "code39") {
        return "^BY2^B3N," + $chk + "," + $hs + "," + $hri + ",N^FD" + $data + "^FS";
    }
    if ($btype == "gs1-128") {
        # ^BC mode D = UCC/EAN (GS1-128); parenthesised AI data is parsed.
        return "^BY2^BCN," + $hs + "," + $hri + ",N,N,D^FD" + $data + "^FS";
    }
    if ($btype == "datamatrix") {
        # ^BX: module height in dots, quality 200 = ECC200.
        return "^BXN," + $hs + ",200^FD" + $data + "^FS";
    }
    def lvl as string init "M";
    if (not ($f.errorLevel == "")) {
        $lvl = $f.errorLevel;
    }
    return "^BQN,2,5^FD" + $lvl + "A," + $data + "^FS";
}

# zplField renders one field as a ZPL command sequence.
func zplField(f as Field, dpi as int) {
    def origin as string init "^FO" + convert.toString(mmToDots($f.x, $dpi)) + "," + convert.toString(mmToDots($f.y, $dpi));
    if ($f.kind == "text") {
        def h as string init convert.toString(mmToDots($f.h, $dpi));
        return $origin + "^A0N," + $h + "," + $h + "^FH^FD" + zplEscape($f.data) + "^FS";
    }
    if ($f.kind == "box") {
        return $origin + "^GB" + convert.toString(mmToDots($f.w, $dpi)) + "," +
            convert.toString(mmToDots($f.h, $dpi)) + "," +
            convert.toString(mmToDots($f.thickness, $dpi)) + "^FS";
    }
    if ($f.kind == "image") {
        # Recall a stored graphic by name at native size.
        return $origin + "^XG" + $f.data + ",1,1^FS";
    }
    return $origin + zplBarcode($f, mmToDots($f.h, $dpi));
}

# renderZpl renders a whole label as a ZPL command stream.
func renderZpl(label as Label, dpi as int) {
    if ($dpi <= 0) {
        throw Error{ kind: "label", message: "label: zpl render needs a positive dpi", file: "", line: 0, col: 0 };
    }
    def out as string init "^XA\n";
    for (def f in $label.fields) {
        $out = $out + zplField($f, $dpi) + "\n";
    }
    return $out + "^PQ" + convert.toString($label.quantity) + "\n^XZ\n";
}
