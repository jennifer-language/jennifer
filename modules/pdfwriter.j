# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Generate simple PDF documents - text, lines, rectangles - the way
 * `htmlwriter` / `label` generate their formats. Build a `Document` of `Page`s
 * with value-semantic builders, then `render()` writes the PDF object / xref
 * structure by hand (no stdlib PDF) as `bytes`. Content streams are
 * FlateDecode-compressed via `compress`. The standard-14 Type1 fonts (Helvetica
 * / Times / Courier families + Symbol / ZapfDingbats); embedded fonts and images
 * are follow-ons. Pure Jennifer; both binaries.
 *
 * Coordinates are in PDF points (1/72 inch), origin bottom-left, y upward.
 * Common page sizes: US Letter 612 x 792, A4 595 x 842. Colours are 0-255 RGB.
 * @module pdfwriter
 * @example
 * import "pdfwriter.j" as pdf;
 * def p as pdf.Page init pdf.page(612, 792);
 * $p = pdf.text($p, 72, 720, "Helvetica", 24, "Hello, PDF");
 * def doc as pdf.Document init pdf.addPage(pdf.document(), $p);
 * def out as bytes init pdf.render($doc);   # write to a file with fs.writeBytes
 */
use strings;
use lists;
use convert;
use compress;
use time;
use encoding;

/**
 * A PDF document: an ordered list of pages and the document metadata
 * (the PDF Info dictionary, keyed by PDF key name - "Title", "Author", ...).
 * @field pages {list of Page} the document's pages
 * @field info {map of string to string} the Info-dictionary metadata
 */
export def struct Document {
    pages as list of Page,
    info as map of string to string
};

/**
 * A single page: its size (points) and its accumulated content-stream operators.
 * @field width {int} the page width in points
 * @field height {int} the page height in points
 * @field content {string} the content-stream operators built so far
 * @field fonts {list of string} the distinct fonts referenced on this page
 */
export def struct Page {
    width as int,
    height as int,
    content as string,
    fonts as list of string
};

func fail(msg as string) {
    throw Error{ kind: "pdfwriter", message: $msg, file: "", line: 0, col: 0 };
}

# standardFonts lists the 14 base fonts every PDF viewer provides.
func standardFonts() {
    return ["Helvetica", "Helvetica-Bold", "Helvetica-Oblique", "Helvetica-BoldOblique",
        "Times-Roman", "Times-Bold", "Times-Italic", "Times-BoldItalic",
        "Courier", "Courier-Bold", "Courier-Oblique", "Courier-BoldOblique",
        "Symbol", "ZapfDingbats"];
}

# --- builders (exported) ----------------------------------------------------

/**
 * An empty document. The `Producer` metadata defaults to "Jennifer pdfwriter";
 * everything else is unset (no `CreationDate` is stamped, so output stays
 * deterministic - set one explicitly with `info` + `pdfDate` if you want it).
 * @return {Document} the document
 */
export func document() {
    def meta as map of string to string init {"Producer": "Jennifer pdfwriter"};
    return Document{ pages: [], info: $meta };
}

/**
 * A copy of the document with one metadata field set (value-semantic). `key` is
 * a PDF Info key - "Title", "Author", "Subject", "Keywords", "Creator",
 * "Producer", "CreationDate", "ModDate" (or any custom key).
 * @param doc {Document} the document
 * @param key {string} the Info-dictionary key
 * @param value {string} the value
 * @return {Document} a fresh document with the metadata set
 */
export func info(doc as Document, key as string, value as string) {
    $doc.info[$key] = $value;
    return $doc;
}

/**
 * Format an instant as a PDF date string (`D:YYYYMMDDHHmmSS+HH'mm'`), for use as
 * a `CreationDate` / `ModDate` value. Passing a time is explicit, so it does not
 * make `render` non-deterministic on its own.
 * @param t {time.Time} the instant
 * @return {string} the PDF date string
 */
export func pdfDate(t as time.Time) {
    def base as string init time.format($t, "%Y%m%d%H%M%S");
    def z as string init time.format($t, "%z");
    return "D:" + $base + strings.substring($z, 0, 1) + strings.substring($z, 1, 3) + "'" + strings.substring($z, 3, 5) + "'";
}

/**
 * A blank page of the given size in points (e.g. `page(612, 792)` for Letter,
 * `page(595, 842)` for A4).
 * @param width {int} the page width in points
 * @param height {int} the page height in points
 * @return {Page} the page
 */
export func page(width as int, height as int) {
    return Page{ width: $width, height: $height, content: "", fonts: [] };
}

# hexByte renders a byte as two uppercase hex digits.
func hexByte(b as int) {
    def one as bytes;
    $one[] = $b;
    return strings.upper(encoding.toText($one, "hex"));
}

# pdfName escapes a dictionary key as a PDF name token: any byte outside the
# regular-character set (whitespace, delimiters, `#`) becomes `#xx`, so a key
# with a space / `/` / `(` can't produce a malformed dictionary.
func pdfName(key as string) {
    def raw as bytes init convert.bytesFromString($key, "utf-8");
    def out as list of string init [];
    def i as int init 0;
    while ($i < len($raw)) {
        def b as int init $raw[$i];
        if ($b <= 32 or $b >= 127 or $b == 35 or $b == 47 or $b == 40 or $b == 41 or $b == 60 or $b == 62 or $b == 91 or $b == 93 or $b == 123 or $b == 125 or $b == 37) {
            $out[] = "#" + hexByte($b);
        } else {
            $out[] = convert.fromCodepoint($b);
        }
        $i = $i + 1;
    }
    return strings.join($out, "");
}

# utfSixteenBe encodes a string as UTF-16BE with a leading BOM (surrogate pairs
# for supplementary code points) - the PDF text-string form for non-Latin text.
func utfSixteenBe(s as string) {
    def out as bytes;
    $out[] = 0xFE;
    $out[] = 0xFF;
    for (def ch in strings.chars($s)) {
        def cp as int init convert.toCodepoint($ch);
        if ($cp < 0x10000) {
            $out[] = ($cp >> 8) & 0xff;
            $out[] = $cp & 0xff;
        } else {
            def v as int init $cp - 0x10000;
            def hi as int init 0xD800 + (($v >> 10) & 0x3FF);
            def lo as int init 0xDC00 + ($v & 0x3FF);
            $out[] = ($hi >> 8) & 0xff;
            $out[] = $hi & 0xff;
            $out[] = ($lo >> 8) & 0xff;
            $out[] = $lo & 0xff;
        }
    }
    return $out;
}

# infoValue renders a document-info value: a WinAnsi literal `(...)` for ASCII
# text, else a UTF-16BE hex string `<FEFF...>` so non-Latin metadata isn't
# emitted as raw UTF-8 mojibake.
func infoValue(v as string) {
    if (encoding.isAscii(convert.bytesFromString($v, "utf-8"))) {
        return "(" + escapeString($v) + ")";
    }
    return "<" + encoding.toText(utfSixteenBe($v), "hex") + ">";
}

# octalTriple renders a byte as three octal digits for a PDF `\ddd` escape.
func octalTriple(b as int) {
    return convert.toString(($b >> 6) & 7) + convert.toString(($b >> 3) & 7) + convert.toString($b & 7);
}

# escapeString transcodes text to WinAnsi (windows-1252) bytes to match the
# font's /WinAnsiEncoding, then PDF-escapes it: high / control bytes become
# octal `\ddd` runs (a raw UTF-8 emission would render as mojibake). A character
# outside WinAnsi throws (from encoding.encode).
func escapeString(s as string) {
    def raw as bytes init encoding.encode($s, "windows-1252");
    def out as list of string init [];
    def i as int init 0;
    while ($i < len($raw)) {
        def b as int init $raw[$i];
        if ($b == 92) {
            $out[] = "\\\\";
        } elseif ($b == 40) {
            $out[] = "\\(";
        } elseif ($b == 41) {
            $out[] = "\\)";
        } elseif ($b == 13) {
            $out[] = "\\r";
        } elseif ($b == 10) {
            $out[] = "\\n";
        } elseif ($b < 32 or $b >= 127) {
            $out[] = "\\" + octalTriple($b);
        } else {
            $out[] = convert.fromCodepoint($b);
        }
        $i = $i + 1;
    }
    return strings.join($out, "");
}

/**
 * Draw a line of text at (x, y) in the given standard-14 font and point size.
 * @param pg {Page} the page
 * @param x {int} the x position (points from the left)
 * @param y {int} the y position (points from the bottom)
 * @param font {string} a standard-14 base font name (e.g. "Helvetica")
 * @param size {int} the font size in points
 * @param str {string} the text to draw
 * @return {Page} a fresh page with the text added
 * @throws {Error} kind "pdfwriter" if the font is not a standard-14 name
 */
export func text(pg as Page, x as int, y as int, font as string, size as int, str as string) {
    if (not lists.contains(standardFonts(), $font)) {
        fail("unknown font '" + $font + "' (use a standard-14 base font)");
    }
    if (not lists.contains($pg.fonts, $font)) {
        $pg.fonts = lists.push($pg.fonts, $font);
    }
    $pg.content = $pg.content + "BT\n/" + $font + " " + convert.toString($size) + " Tf\n" +
        convert.toString($x) + " " + convert.toString($y) + " Td\n(" + escapeString($str) + ") Tj\nET\n";
    return $pg;
}

/**
 * Draw a straight line from (fromX, fromY) to (toX, toY).
 * @param pg {Page} the page
 * @param fromX {int} the start x
 * @param fromY {int} the start y
 * @param toX {int} the end x
 * @param toY {int} the end y
 * @return {Page} a fresh page with the line added
 */
export func line(pg as Page, fromX as int, fromY as int, toX as int, toY as int) {
    $pg.content = $pg.content + convert.toString($fromX) + " " + convert.toString($fromY) + " m\n" +
        convert.toString($toX) + " " + convert.toString($toY) + " l\nS\n";
    return $pg;
}

/**
 * Draw a rectangle at (x, y) of the given size. `filled` fills it; otherwise it
 * is stroked (outline only).
 * @param pg {Page} the page
 * @param x {int} the lower-left x
 * @param y {int} the lower-left y
 * @param width {int} the width in points
 * @param height {int} the height in points
 * @param filled {bool} true to fill, false to stroke
 * @return {Page} a fresh page with the rectangle added
 */
export func rect(pg as Page, x as int, y as int, width as int, height as int, filled as bool) {
    def op as string init "S";
    if ($filled) {
        $op = "f";
    }
    $pg.content = $pg.content + convert.toString($x) + " " + convert.toString($y) + " " +
        convert.toString($width) + " " + convert.toString($height) + " re\n" + $op + "\n";
    return $pg;
}

# colorComp formats a 0-255 component as a PDF 0..1 number.
func colorComp(v as int) {
    def s as string init convert.toString($v / 255);
    if (strings.contains($s, ".")) {
        while (strings.endsWith($s, "0")) {
            $s = strings.substring($s, 0, len($s) - 1);
        }
        if (strings.endsWith($s, ".")) {
            $s = strings.substring($s, 0, len($s) - 1);
        }
    }
    return $s;
}

/**
 * Set the fill and stroke colour (0-255 RGB) for subsequent drawing on the page.
 * @param pg {Page} the page
 * @param red {int} the red component 0-255
 * @param green {int} the green component 0-255
 * @param blue {int} the blue component 0-255
 * @return {Page} a fresh page with the colour set
 */
export func color(pg as Page, red as int, green as int, blue as int) {
    def r as string init colorComp($red);
    def g as string init colorComp($green);
    def b as string init colorComp($blue);
    $pg.content = $pg.content + $r + " " + $g + " " + $b + " rg\n" + $r + " " + $g + " " + $b + " RG\n";
    return $pg;
}

/**
 * A copy of the document with a page appended.
 * @param doc {Document} the document
 * @param pg {Page} the page to add
 * @return {Document} a fresh document with the page appended
 */
export func addPage(doc as Document, pg as Page) {
    $doc.pages = lists.push($doc.pages, $pg);
    return $doc;
}

# --- render (exported) ------------------------------------------------------

# appendStr appends a string's UTF-8 bytes to a buffer.
func appendStr(buf as bytes, s as string) {
    def chunk as bytes init convert.bytesFromString($s, "utf-8");
    def i as int init 0;
    def n as int init len($chunk);
    while ($i < $n) {
        $buf[] = $chunk[$i];
        $i = $i + 1;
    }
    return $buf;
}

# appendBytes appends a raw byte chunk to a buffer.
func appendBytes(buf as bytes, chunk as bytes) {
    def i as int init 0;
    def n as int init len($chunk);
    while ($i < $n) {
        $buf[] = $chunk[$i];
        $i = $i + 1;
    }
    return $buf;
}

# zeroPad left-pads a number with zeros to a fixed width (for xref offsets).
func zeroPad(value as int, width as int) {
    def s as string init convert.toString($value);
    while (len($s) < $width) {
        $s = "0" + $s;
    }
    return $s;
}

# collectFonts returns the distinct fonts used across all pages, in first-use order.
func collectFonts(doc as Document) {
    def names as list of string init [];
    for (def pg in $doc.pages) {
        for (def f in $pg.fonts) {
            if (not lists.contains($names, $f)) {
                $names[] = $f;
            }
        }
    }
    return $names;
}

# fontObjNum maps a font name to its object number.
func fontObjNum(fontNames as list of string, name as string, base as int) {
    def i as int init 0;
    while ($i < len($fontNames)) {
        if ($fontNames[$i] == $name) {
            return $base + $i;
        }
        $i = $i + 1;
    }
    return $base;
}

/**
 * Render the document to PDF bytes (PDF 1.7). Content streams are
 * FlateDecode-compressed; every page's fonts become shared Type1 font objects.
 * @param doc {Document} the document to render
 * @return {bytes} the PDF file contents
 */
export func render(doc as Document) {
    def numPages as int init len($doc.pages);
    def fontNames as list of string init collectFonts($doc);
    def numFonts as int init len($fontNames);
    def fontBase as int init 3 + 2 * $numPages;
    def baseObjs as int init 2 + 2 * $numPages + $numFonts;
    def hasInfo as bool init len($doc.info) > 0;
    def infoNum as int init $baseObjs + 1;
    def totalObjs as int init $baseObjs;
    if ($hasInfo) {
        $totalObjs = $baseObjs + 1;
    }

    def buf as bytes;
    def offsets as list of int init [];

    # header + binary marker comment (four high bytes)
    $buf = appendStr($buf, "%PDF-1.7\n");
    $buf[] = 0x25;
    $buf[] = 0xE2;
    $buf[] = 0xE3;
    $buf[] = 0xCF;
    $buf[] = 0xD3;
    $buf[] = 0x0A;

    # obj 1: catalog
    $offsets[] = len($buf);
    $buf = appendStr($buf, "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n");

    # obj 2: page tree
    def kids as string init "";
    def p as int init 0;
    while ($p < $numPages) {
        if ($p > 0) {
            $kids = $kids + " ";
        }
        $kids = $kids + convert.toString(3 + 2 * $p) + " 0 R";
        $p = $p + 1;
    }
    $offsets[] = len($buf);
    $buf = appendStr($buf, "2 0 obj\n<< /Type /Pages /Kids [" + $kids + "] /Count " + convert.toString($numPages) + " >>\nendobj\n");

    # per page: the page dict and its (compressed) content stream
    $p = 0;
    while ($p < $numPages) {
        def pg as Page init $doc.pages[$p];
        def contentNum as int init 4 + 2 * $p;
        def fontDict as string init "";
        for (def fn in $pg.fonts) {
            $fontDict = $fontDict + "/" + $fn + " " + convert.toString(fontObjNum($fontNames, $fn, $fontBase)) + " 0 R ";
        }
        $offsets[] = len($buf);
        $buf = appendStr($buf, convert.toString(3 + 2 * $p) + " 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 " +
            convert.toString($pg.width) + " " + convert.toString($pg.height) + "] /Resources << /Font << " + $fontDict +
            ">> >> /Contents " + convert.toString($contentNum) + " 0 R >>\nendobj\n");

        def comp as bytes init compress.pack(convert.bytesFromString($pg.content, "utf-8"), "zlib");
        $offsets[] = len($buf);
        $buf = appendStr($buf, convert.toString($contentNum) + " 0 obj\n<< /Length " + convert.toString(len($comp)) + " /Filter /FlateDecode >>\nstream\n");
        $buf = appendBytes($buf, $comp);
        $buf = appendStr($buf, "\nendstream\nendobj\n");
        $p = $p + 1;
    }

    # font objects (shared)
    def f as int init 0;
    while ($f < $numFonts) {
        $offsets[] = len($buf);
        # Symbol and ZapfDingbats carry their own built-in encodings; forcing
        # /WinAnsiEncoding on them remaps their glyphs, so omit /Encoding there.
        def enc as string init " /Encoding /WinAnsiEncoding";
        if ($fontNames[$f] == "Symbol" or $fontNames[$f] == "ZapfDingbats") {
            $enc = "";
        }
        $buf = appendStr($buf, convert.toString($fontBase + $f) + " 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /" +
            $fontNames[$f] + $enc + " >>\nendobj\n");
        $f = $f + 1;
    }

    # info dictionary (document metadata), when any field is set
    if ($hasInfo) {
        def dict as string init "";
        for (def key in $doc.info) {
            $dict = $dict + "/" + pdfName($key) + " " + infoValue($doc.info[$key]) + " ";
        }
        $offsets[] = len($buf);
        $buf = appendStr($buf, convert.toString($infoNum) + " 0 obj\n<< " + $dict + ">>\nendobj\n");
    }

    # cross-reference table
    def xrefOffset as int init len($buf);
    $buf = appendStr($buf, "xref\n0 " + convert.toString($totalObjs + 1) + "\n");
    $buf = appendStr($buf, "0000000000 65535 f \n");
    def k as int init 0;
    while ($k < len($offsets)) {
        $buf = appendStr($buf, zeroPad($offsets[$k], 10) + " 00000 n \n");
        $k = $k + 1;
    }

    # trailer
    def trailerInfo as string init "";
    if ($hasInfo) {
        $trailerInfo = " /Info " + convert.toString($infoNum) + " 0 R";
    }
    $buf = appendStr($buf, "trailer\n<< /Size " + convert.toString($totalObjs + 1) + " /Root 1 0 R" + $trailerInfo +
        " >>\nstartxref\n" + convert.toString($xrefOffset) + "\n%%EOF\n");
    return $buf;
}
