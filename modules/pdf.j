# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
# pragma-jennifer-version: >=0.24.0

# A hand-rolled PDF writer: its render and PNG-decode methods legitimately run
# past the L201 statement-count limit. Every other lint check stays active.
# lint-disable-file: L201

/**
 * Generate PDF documents - text, lines, rectangles - the way `html` /
 * `label` generate their formats. Build a `Document` of `Page`s with
 * value-semantic builders, then `render()` writes the PDF object / xref structure
 * by hand (no stdlib PDF) as `bytes`. Content streams are FlateDecode-compressed
 * via `compress`. Two font paths: the standard-14 Type1 fonts (Helvetica / Times
 * / Courier families + Symbol / ZapfDingbats) via `text`, and **embedded
 * TrueType fonts** via `loadFont` / `addFont` / `textUnicode` - a Type0 /
 * CIDFontType2 composite (Identity-H) with an embedded `FontFile2` and a
 * ToUnicode map, so any script the font covers (accented Latin, Greek, Cyrillic,
 * CJK, ...) renders and stays selectable. Raster images embed as image XObjects
 * via `loadImage` / `addImage` / `drawImage` - PNG (greyscale / RGB / palette,
 * plus 8-bit alpha as a soft mask) and JPEG (DCTDecode). Text layout -
 * `measureText` width measurement (standard-14 AFM metrics + embedded-font
 * advances), `wrapText` word-wrap, and `textBlock` wrapped / aligned paragraphs
 * (left / right / center / justify) - flows text into a column. Object numbers
 * are assigned dynamically. Pure Jennifer (over the `font` module); both
 * binaries.
 *
 * Coordinates are in PDF points (1/72 inch), origin bottom-left, y upward.
 * Common page sizes: US Letter 612 x 792, A4 595 x 842. Colours are 0-255 RGB.
 * @module pdf
 * @example
 * import "pdf.j" as pdf;
 * def p as pdf.Page init pdf.page(612, 792);
 * $p = pdf.text($p, 72, 720, "Helvetica", 24, "Hello, PDF");
 * def doc as pdf.Document init pdf.addPage(pdf.document(), $p);
 * def out as bytes init pdf.render($doc);   # write to a file with fs.writeBytes
 */
use strings;
use lists;
use maps;
use convert;
use compress;
use binary;
use math;
use time;
use encoding;
import "./font.j" as font;
include "./pdf_afm.inc.j";

/**
 * A loaded, embeddable font: a resource name (referenced in `textUnicode`) and
 * the parsed `font.Font`. Register it in a document with `addFont`.
 * @field name {string} the resource name (e.g. "Body")
 * @field f {font.Font} the parsed font
 */
export def struct LoadedFont {
    name as string,
    f as font.Font
};

/**
 * A record of one glyph used by embedded text: which font, its glyph id, and the
 * Unicode codepoint it came from - collected so `render` can build the font's
 * width array and ToUnicode map.
 * @field font {string} the font resource name
 * @field gid {int} the glyph id
 * @field cp {int} the source Unicode codepoint
 */
export def struct GlyphUse {
    font as string,
    gid as int,
    cp as int
};

/**
 * A loaded raster image (from `loadImage`), ready to embed as a PDF image
 * XObject and draw with `drawImage`. Register it in a document with `addImage`.
 * @field name {string} the resource name (referenced in `drawImage`)
 * @field width {int} the pixel width
 * @field height {int} the pixel height
 * @field bits {int} bits per colour component
 * @field colorSpace {string} the PDF colour-space token (e.g. "/DeviceRGB")
 * @field filter {string} the stream filter ("DCTDecode" for JPEG, "FlateDecode" for PNG)
 * @field predictor {int} 15 when the FlateDecode stream carries PNG predictors, else 0
 * @field colors {int} colour components per pixel (for the predictor DecodeParms)
 * @field decode {string} an optional /Decode array token (Adobe CMYK JPEG), else ""
 * @field data {bytes} the image stream bytes
 * @field smask {bytes} the soft-mask (alpha) stream, empty when opaque
 * @field hasSmask {bool} whether the image carries an alpha soft mask
 */
export def struct Image {
    name as string,
    width as int,
    height as int,
    bits as int,
    colorSpace as string,
    filter as string,
    predictor as int,
    colors as int,
    decode as string,
    data as bytes,
    smask as bytes,
    hasSmask as bool
};

/**
 * One outline (bookmark) entry: a title that jumps to a position on a page, plus
 * the heading level that nests it under the outline tree. Built with `bookmark`.
 * @field title {string} the bookmark label
 * @field page {int} the 0-based page index the bookmark points at
 * @field y {int} the destination y coordinate in PDF points (origin bottom-left)
 * @field level {int} the nesting level (1 = top; a level-2 entry nests under the last level-1)
 */
export def struct OutlineEntry {
    title as string,
    page as int,
    y as int,
    level as int
};

/**
 * A running header or footer, drawn on every page at render time. Its three text
 * slots are left- / centre- / right-aligned (alignment measured with the font's
 * own metrics), and each may contain the placeholders `%page%` (the 1-based page
 * number) and `%pages%` (the total page count), filled in per page - so a footer
 * `"sample.pdf"` / `""` / `"%page%/%pages%"` reads `sample.pdf ... 13/108`. The
 * placeholders are percent-delimited (not braces) so they do not collide with
 * Jennifer's own `{expr}` string interpolation. `margin` is the distance from the
 * page edge to the text on every side. With `border` on, a thin rule is drawn under
 * a header / over a footer.
 * @field left {string} left-aligned slot (may use %page% / %pages%)
 * @field center {string} centre-aligned slot
 * @field right {string} right-aligned slot
 * @field font {string} a standard-14 font name
 * @field size {int} the point size
 * @field margin {int} distance from the page edge, in points
 * @field border {bool} draw a separating rule
 * @field red {int} text / rule colour red 0-255
 * @field green {int} text / rule colour green 0-255
 * @field blue {int} text / rule colour blue 0-255
 */
export def struct PageLabel {
    left as string,
    center as string,
    right as string,
    font as string,
    size as int,
    margin as int,
    border as bool,
    red as int,
    green as int,
    blue as int
};

/**
 * A PDF document: an ordered list of pages, the document metadata (the PDF Info
 * dictionary, keyed by PDF key name - "Title", "Author", ...), any embedded
 * fonts registered with `addFont`, any outline (bookmark) entries, and an optional
 * running header / footer drawn on every page at render time.
 * @field pages {list of Page} the document's pages
 * @field info {map of string to string} the Info-dictionary metadata
 * @field embedded {list of LoadedFont} the embedded fonts
 * @field images {list of Image} the embedded raster images
 * @field outline {list of OutlineEntry} the outline / bookmark entries, in document order
 * @field header {PageLabel} the running header (drawn when headerOn)
 * @field footer {PageLabel} the running footer (drawn when footerOn)
 * @field headerOn {bool} whether a header has been set
 * @field footerOn {bool} whether a footer has been set
 */
export def struct Document {
    pages as list of Page,
    info as map of string to string,
    embedded as list of LoadedFont,
    images as list of Image,
    outline as list of OutlineEntry,
    header as PageLabel,
    footer as PageLabel,
    headerOn as bool,
    footerOn as bool
};

/**
 * A single page: its size (points) and its accumulated content-stream operators.
 * @field width {int} the page width in points
 * @field height {int} the page height in points
 * @field content {string} the content-stream operators built so far
 * @field fonts {list of string} the distinct standard-14 fonts referenced on this page
 * @field glyphUses {list of GlyphUse} embedded-font glyphs drawn on this page
 */
export def struct Page {
    width as int,
    height as int,
    content as string,
    fonts as list of string,
    glyphUses as list of GlyphUse
};

func fail(msg as string) {
    throw Error{kind: "pdf", message: $msg, file: "", line: 0, col: 0};
}

# checkName validates a font / image resource name: a letter, then letters or
# digits (the documented contract). Resource names are written into the PDF
# content stream and the /Font / /XObject resource dictionaries UNESCAPED, so
# this is the injection guard - an unchecked name like `X>> /OpenAction <<...`
# would otherwise break out of the dictionary and inject PDF structure.
func checkName(name as string, what as string) {
    def raw as bytes init convert.bytesFromString($name, "utf-8");
    def ok as bool init len($raw) > 0;
    def i as int init 0;
    while ($i < len($raw)) {
        def b as int init $raw[$i];
        def alpha as bool init ($b >= 65 and $b <= 90) or ($b >= 97 and $b <= 122);
        def digit as bool init $b >= 48 and $b <= 57;
        if ($i == 0) {
            if (not $alpha) {
                $ok = false;
            }
        } elseif (not ($alpha or $digit)) {
            $ok = false;
        }
        $i = $i + 1;
    }
    if (not $ok) {
        fail($what + " name must be a letter then letters/digits (e.g. \"Body\"), got: " + $name);
    }
}

# standardFonts lists the 14 base fonts every PDF viewer provides.
func standardFonts() {
    return [
        "Helvetica",
        "Helvetica-Bold",
        "Helvetica-Oblique",
        "Helvetica-BoldOblique",
        "Times-Roman",
        "Times-Bold",
        "Times-Italic",
        "Times-BoldItalic",
        "Courier",
        "Courier-Bold",
        "Courier-Oblique",
        "Courier-BoldOblique",
        "Symbol",
        "ZapfDingbats"
    ];
}

# --- builders (exported) ----------------------------------------------------

/**
 * An empty document. The `Producer` metadata defaults to "Jennifer pdf";
 * everything else is unset (no `CreationDate` is stamped, so output stays
 * deterministic - set one explicitly with `info` + `pdfDate` if you want it).
 * @return {Document} the document
 */
export func document() {
    def meta as map of string to string init {"Producer": "Jennifer pdf"};
    def noFonts as list of LoadedFont init [];
    def noImages as list of Image init [];
    def noOutline as list of OutlineEntry init [];
    def blank as PageLabel init pageLabel();
    return Document{
        pages: [],
        info: $meta,
        embedded: $noFonts,
        images: $noImages,
        outline: $noOutline,
        header: $blank,
        footer: $blank,
        headerOn: false,
        footerOn: false
    };
}

/**
 * A blank page label (empty slots, Helvetica 9pt, 36-point margin, no border,
 * black). Copy it and set slots / options, then attach with `setHeader` /
 * `setFooter`.
 * @return {PageLabel} the default label
 */
export func pageLabel() {
    return PageLabel{
        left: "",
        center: "",
        right: "",
        font: "Helvetica",
        size: 9,
        margin: 36,
        border: false,
        red: 0,
        green: 0,
        blue: 0
    };
}

/**
 * Attach a running header, drawn on every page when the document renders.
 * @param doc {Document} the document
 * @param label {PageLabel} the header spec
 * @return {Document} the document with the header set
 */
export func setHeader(doc as Document, label as PageLabel) {
    def d as Document init $doc;
    $d.header = $label;
    $d.headerOn = true;
    return $d;
}

/**
 * Attach a running footer, drawn on every page when the document renders.
 * @param doc {Document} the document
 * @param label {PageLabel} the footer spec
 * @return {Document} the document with the footer set
 */
export func setFooter(doc as Document, label as PageLabel) {
    def d as Document init $doc;
    $d.footer = $label;
    $d.footerOn = true;
    return $d;
}

/**
 * The total number of pages added to the document (the value `{pages}` expands to).
 * @param doc {Document} the document
 * @return {int} the page count
 */
export func getTotalPages(doc as Document) {
    return len($doc.pages);
}

/**
 * The 1-based number of the page currently being built - the next page to be
 * added, i.e. `getTotalPages(doc) + 1`. Use it while filling a page (before
 * `addPage`) to label it.
 * @param doc {Document} the document
 * @return {int} the current page number
 */
export func getCurrentPageNr(doc as Document) {
    return len($doc.pages) + 1;
}

/**
 * A copy of the document with one outline (bookmark) entry appended
 * (value-semantic). Entries are nested by `level` in the order added: a level-2
 * entry becomes a child of the most recent level-1, and so on. The destination
 * scrolls page `page` (0-based) so that `y` (PDF points, origin bottom-left) is
 * at the top of the view. With any outline present, the viewer opens showing the
 * bookmark panel.
 * @param doc {Document} the document
 * @param page {int} the 0-based index of the destination page
 * @param y {int} the destination y in PDF points
 * @param title {string} the bookmark label
 * @param level {int} the nesting level (>= 1)
 * @return {Document} a copy with the entry appended
 */
export func bookmark(doc as Document, page as int, y as int, title as string, level as int) {
    if ($level < 1) {
        fail("bookmark: level must be >= 1");
    }
    def out as Document init $doc;
    $out.outline = lists.push($out.outline, OutlineEntry{title: $title, page: $page, y: $y, level: $level});
    return $out;
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
    return "D:" + $base + strings.substring($z, 0, 1) + strings.substring($z, 1, 3) + "'" +
        strings.substring($z, 3, 5) + "'";
}

/**
 * A blank page of the given size in points (e.g. `page(612, 792)` for Letter,
 * `page(595, 842)` for A4).
 * @param width {int} the page width in points
 * @param height {int} the page height in points
 * @return {Page} the page
 */
export func page(width as int, height as int) {
    def noGlyphs as list of GlyphUse init [];
    return Page{width: $width, height: $height, content: "", fonts: [], glyphUses: $noGlyphs};
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
        if ($b <= 32 or $b >= 127 or $b == 35 or $b == 47 or $b == 40 or $b == 41 or $b == 60 or
            $b == 62 or $b == 91 or $b == 93 or $b == 123 or $b == 125 or $b == 37) {
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
    return convert.toString(($b >> 6) & 7) + convert.toString(($b >> 3) & 7) +
        convert.toString($b & 7);
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
 * @throws {Error} kind "pdf" if the font is not a standard-14 name
 */
export func text(pg as Page, x as int, y as int, font as string, size as int, str as string) {
    if (not lists.contains(standardFonts(), $font)) {
        fail("unknown font '" + $font + "' (use a standard-14 base font)");
    }
    if (not lists.contains($pg.fonts, $font)) {
        $pg.fonts = lists.push($pg.fonts, $font);
    }
    $pg.content = $pg.content + "BT\n/" + $font + " " + convert.toString($size) + " Tf\n" +
        convert.toString($x) + " " + convert.toString($y) + " Td\n(" + escapeString($str) +
        ") Tj\nET\n";
    return $pg;
}

/**
 * Load an embeddable font from its bytes, under a resource name used to select it
 * in `textUnicode`. The font is embedded (and its Unicode text made selectable)
 * when the document renders.
 * @param name {string} the resource name (e.g. "Body"; letters / digits)
 * @param data {bytes} the TrueType (.ttf) font file
 * @return {LoadedFont} the loaded font
 */
export func loadFont(name as string, data as bytes) {
    checkName($name, "font");
    def parsed as font.Font init font.parse($data);
    if (font.isCff($parsed)) {
        fail("loadFont: only a TrueType (glyf) font can be embedded; '" + $name +
            "' is a CFF / OpenType-PostScript font");
    }
    return LoadedFont{name: $name, f: $parsed};
}

/**
 * A copy of the document with an embedded font registered, so `render` writes its
 * font program and dictionaries.
 * @param doc {Document} the document
 * @param lf {LoadedFont} the loaded font (from `loadFont`)
 * @return {Document} a fresh document with the font registered
 */
export func addFont(doc as Document, lf as LoadedFont) {
    def out as Document init $doc;
    $out.embedded = lists.push($out.embedded, $lf);
    return $out;
}

# hexGid renders a glyph id as four uppercase hex digits (a 2-byte Identity-H
# character code). A glyph id past 0xFFFF cannot be represented in the 2-byte
# code and must not silently truncate, so it is rejected.
func hexGid(gid as int) {
    if ($gid < 0 or $gid > 0xFFFF) {
        fail("textUnicode: glyph id " + convert.toString($gid) +
            " is outside the 2-byte Identity-H range (font has too many glyphs to embed this way)");
    }
    def b as bytes;
    $b[] = ($gid >> 8) & 0xff;
    $b[] = $gid & 0xff;
    return strings.upper(encoding.toText($b, "hex"));
}

/**
 * Draw Unicode text at (x, y) in an embedded font (from `loadFont` + `addFont`)
 * at the given point size. Each character maps through the font's cmap to a
 * glyph, so any script the font covers - including CJK - renders and stays
 * selectable / copyable in a viewer.
 * @param pg {Page} the page
 * @param x {int} the x position (points from the left)
 * @param y {int} the y position (points from the bottom)
 * @param lf {LoadedFont} the embedded font
 * @param size {int} the font size in points
 * @param str {string} the text to draw
 * @return {Page} a fresh page with the text added
 */
export func textUnicode(pg as Page, x as int, y as int, lf as LoadedFont, size as int, str as string) {
    def cps as list of int init [];
    for (def ch in strings.chars($str)) {
        $cps[] = convert.toCodepoint($ch);
    }
    def gids as list of int init font.glyphIds($lf.f, $cps);   # one font copy, not one per char
    def hex as string init "";
    def i as int init 0;
    while ($i < len($cps)) {
        $hex = $hex + hexGid($gids[$i]);
        $pg.glyphUses = lists.push($pg.glyphUses, GlyphUse{font: $lf.name, gid: $gids[$i], cp: $cps[$i]});
        $i = $i + 1;
    }
    $pg.content = $pg.content + "BT\n/" + $lf.name + " " + convert.toString($size) + " Tf\n" +
        convert.toString($x) + " " + convert.toString($y) + " Td\n<" + $hex + "> Tj\nET\n";
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
    $pg.content = $pg.content + $r + " " + $g + " " + $b + " rg\n" + $r + " " + $g + " " + $b +
        " RG\n";
    return $pg;
}

# beU16 reads a big-endian unsigned 16-bit integer at off.
func beU16(b as bytes, off as int) {
    return ($b[$off] << 8) | $b[$off + 1];
}

# beU32 reads a big-endian unsigned 32-bit integer at off.
func beU32(b as bytes, off as int) {
    return ($b[$off] << 24) | ($b[$off + 1] << 16) | ($b[$off + 2] << 8) | $b[$off + 3];
}

# paeth is the PNG Paeth predictor: pick a (left), b (above), or c (upper-left).
func paeth(a as int, b as int, c as int) {
    def p as int init $a + $b - $c;
    def pa as int init $p - $a;
    if ($pa < 0) {
        $pa = 0 - $pa;
    }
    def pb as int init $p - $b;
    if ($pb < 0) {
        $pb = 0 - $pb;
    }
    def pc as int init $p - $c;
    if ($pc < 0) {
        $pc = 0 - $pc;
    }
    if ($pa <= $pb and $pa <= $pc) {
        return $a;
    }
    if ($pb <= $pc) {
        return $b;
    }
    return $c;
}

# pngUnfilter reverses the per-scanline PNG filters (None / Sub / Up / Average /
# Paeth) of an inflated 8-bit-per-channel image, returning the raw pixel bytes
# (filter bytes stripped). `bpp` is bytes-per-pixel (channels, since 8-bit).
func pngUnfilter(raw as bytes, width as int, height as int, bpp as int) {
    def rowBytes as int init $width * $bpp;
    def out as bytes;
    def y as int init 0;
    def inPos as int init 0;
    while ($y < $height) {
        def ft as int init $raw[$inPos];
        $inPos = $inPos + 1;
        def rowStart as int init $y * $rowBytes;
        def x as int init 0;
        while ($x < $rowBytes) {
            def cur as int init $raw[$inPos + $x];
            def a as int init 0;
            if ($x >= $bpp) {
                $a = $out[$rowStart + $x - $bpp];
            }
            def b as int init 0;
            if ($y > 0) {
                $b = $out[$rowStart - $rowBytes + $x];
            }
            def c as int init 0;
            if ($x >= $bpp and $y > 0) {
                $c = $out[$rowStart - $rowBytes + $x - $bpp];
            }
            def val as int init $cur;
            if ($ft == 1) {
                $val = ($cur + $a) & 0xff;
            } elseif ($ft == 2) {
                $val = ($cur + $b) & 0xff;
            } elseif ($ft == 3) {
                $val = ($cur + (($a + $b) // 2)) & 0xff;
            } elseif ($ft == 4) {
                $val = ($cur + paeth($a, $b, $c)) & 0xff;
            }
            $out[] = $val;
            $x = $x + 1;
        }
        $inPos = $inPos + $rowBytes;
        $y = $y + 1;
    }
    return $out;
}

# parseJpeg reads a baseline / progressive JPEG's frame header for dimensions,
# component count, and precision, and wraps the file bytes as a DCTDecode image.
func parseJpeg(name as string, data as bytes) {
    def n as int init len($data);
    def i as int init 2;
    def width as int init 0;
    def height as int init 0;
    def comps as int init 0;
    def bits as int init 8;
    def adobe as bool init false;
    while ($i + 1 < $n) {
        if ($data[$i] != 0xFF) {
            $i = $i + 1;
        } else {
            def marker as int init $data[$i + 1];
            if ($marker == 0xD8 or $marker == 0xD9 or ($marker >= 0xD0 and $marker <= 0xD7) or
                $marker == 0x01 or $marker == 0xFF) {
                $i = $i + 2;
            } else {
                def seg as int init beU16($data, $i + 2);
                if (($marker >= 0xC0 and $marker <= 0xCF) and $marker != 0xC4 and $marker != 0xC8 and
                    $marker != 0xCC) {
                    $bits = $data[$i + 4];
                    $height = beU16($data, $i + 5);
                    $width = beU16($data, $i + 7);
                    $comps = $data[$i + 9];
                    $i = $n;
                } else {
                    if ($marker == 0xEE) {
                        $adobe = true;
                    }
                    $i = $i + 2 + $seg;
                }
            }
        }
    }
    if ($width == 0 or $height == 0 or $comps == 0) {
        fail("loadImage: '" + $name + "' is not a supported JPEG (no frame header found)");
    }
    def cs as string init "/DeviceRGB";
    def dec as string init "";
    if ($comps == 1) {
        $cs = "/DeviceGray";
    } elseif ($comps == 3) {
        $cs = "/DeviceRGB";
    } elseif ($comps == 4) {
        $cs = "/DeviceCMYK";
        if ($adobe) {
            $dec = " /Decode [1 0 1 0 1 0 1 0]";
        }
    } else {
        fail("loadImage: '" + $name + "' JPEG has unsupported component count " +
            convert.toString($comps));
    }
    def none as bytes;
    return Image{name: $name, width: $width, height: $height, bits: $bits, colorSpace: $cs,
        filter: "DCTDecode", predictor: 0, colors: $comps, decode: $dec, data: $data,
        smask: $none, hasSmask: false};
}

# parsePng reads a non-interlaced PNG. Opaque colour types (grey / RGB / palette)
# embed the raw zlib IDAT with a FlateDecode PNG predictor (no pixel decode);
# alpha colour types (grey+alpha / RGBA, 8-bit) are inflated, de-filtered, and
# split into a colour stream plus a grey soft-mask (SMask) stream.
func parsePng(name as string, data as bytes) {
    def n as int init len($data);
    def i as int init 8;
    def width as int init 0;
    def height as int init 0;
    def bitDepth as int init 0;
    def colorType as int init 0;
    def interlace as int init 0;
    def haveIhdr as bool init false;
    def idat as bytes;
    def plte as bytes;
    while ($i + 8 <= $n) {
        def clen as int init beU32($data, $i);
        def ctype as string init convert.fromCodepoint($data[$i + 4]) +
            convert.fromCodepoint($data[$i + 5]) + convert.fromCodepoint($data[$i + 6]) +
            convert.fromCodepoint($data[$i + 7]);
        def body as int init $i + 8;
        # a chunk whose declared length runs past the buffer is truncated / hostile
        if ($clen < 0 or $body + $clen > $n) {
            fail("loadImage: '" + $name + "' has a truncated or oversized PNG chunk");
        }
        if ($ctype == "IHDR" and $clen < 13) {
            fail("loadImage: '" + $name + "' has a malformed PNG IHDR chunk");
        }
        if ($ctype == "IHDR") {
            $width = beU32($data, $body);
            $height = beU32($data, $body + 4);
            $bitDepth = $data[$body + 8];
            $colorType = $data[$body + 9];
            $interlace = $data[$body + 12];
            $haveIhdr = true;
        } elseif ($ctype == "PLTE") {
            $plte = $data[$body..$body + $clen];
        } elseif ($ctype == "IDAT") {
            $idat = binary.concat($idat, $data[$body..$body + $clen]);
        } elseif ($ctype == "IEND") {
            $i = $n;
        }
        if ($i < $n) {
            $i = $body + $clen + 4;
        }
    }
    if (not $haveIhdr) {
        fail("loadImage: '" + $name + "' has no PNG IHDR chunk");
    }
    if ($interlace != 0) {
        fail("loadImage: '" + $name + "' is an interlaced PNG (unsupported)");
    }
    if (len($idat) == 0) {
        fail("loadImage: '" + $name + "' has no PNG image data");
    }
    def none as bytes;
    if ($colorType == 0) {
        return Image{name: $name, width: $width, height: $height, bits: $bitDepth,
            colorSpace: "/DeviceGray", filter: "FlateDecode", predictor: 15, colors: 1,
            decode: "", data: $idat, smask: $none, hasSmask: false};
    }
    if ($colorType == 2) {
        return Image{name: $name, width: $width, height: $height, bits: $bitDepth,
            colorSpace: "/DeviceRGB", filter: "FlateDecode", predictor: 15, colors: 3,
            decode: "", data: $idat, smask: $none, hasSmask: false};
    }
    if ($colorType == 3) {
        if (len($plte) < 3) {
            fail("loadImage: '" + $name + "' is a palette PNG with no usable PLTE chunk");
        }
        def hival as int init (len($plte) // 3) - 1;
        def cs as string init "[/Indexed /DeviceRGB " + convert.toString($hival) + " <" +
            encoding.toText($plte, "hex") + ">]";
        return Image{name: $name, width: $width, height: $height, bits: $bitDepth,
            colorSpace: $cs, filter: "FlateDecode", predictor: 15, colors: 1, decode: "",
            data: $idat, smask: $none, hasSmask: false};
    }
    if ($colorType == 4 or $colorType == 6) {
        if ($bitDepth != 8) {
            fail("loadImage: '" + $name + "' is an alpha PNG that is not 8-bit (unsupported)");
        }
        def ch as int init 2;
        if ($colorType == 6) {
            $ch = 4;
        }
        def raw as bytes init compress.unpack($idat, "zlib");
        # the inflated data must hold height * (1 filter byte + width*bpp) bytes;
        # reject a dimension / data mismatch cleanly before de-filtering
        if (len($raw) < $height * ($width * $ch + 1)) {
            fail("loadImage: '" + $name + "' PNG pixel data is shorter than its dimensions imply");
        }
        def unf as bytes init pngUnfilter($raw, $width, $height, $ch);
        def colorCh as int init $ch - 1;
        def colorBytes as bytes;
        def alphaBytes as bytes;
        def total as int init $width * $height;
        def px as int init 0;
        while ($px < $total) {
            def bo as int init $px * $ch;
            def c as int init 0;
            while ($c < $colorCh) {
                $colorBytes[] = $unf[$bo + $c];
                $c = $c + 1;
            }
            $alphaBytes[] = $unf[$bo + $colorCh];
            $px = $px + 1;
        }
        def cs as string init "/DeviceRGB";
        if ($colorType == 4) {
            $cs = "/DeviceGray";
        }
        return Image{name: $name, width: $width, height: $height, bits: 8, colorSpace: $cs,
            filter: "FlateDecode", predictor: 0, colors: $colorCh, decode: "",
            data: compress.pack($colorBytes, "zlib"), smask: compress.pack($alphaBytes, "zlib"),
            hasSmask: true};
    }
    fail("loadImage: '" + $name + "' has an unsupported PNG colour type " +
        convert.toString($colorType));
}

/**
 * Load a raster image (PNG or JPEG) from its bytes, under a resource name used to
 * select it in `drawImage`. The format is detected from the file signature. PNG:
 * non-interlaced greyscale / RGB / palette (embedded directly with a FlateDecode
 * predictor) and 8-bit greyscale+alpha / RGBA (decoded to a colour stream plus an
 * alpha soft mask). JPEG: baseline / progressive greyscale / RGB / CMYK, embedded
 * as-is via DCTDecode.
 * @param name {string} the resource name (e.g. "Logo"; letters / digits)
 * @param data {bytes} the PNG or JPEG file
 * @return {Image} the loaded image
 * @throws {Error} kind "pdf" if the data is not a supported PNG / JPEG
 */
export func loadImage(name as string, data as bytes) {
    checkName($name, "image");
    if (len($data) < 12) {
        fail("loadImage: '" + $name + "' is too short to be an image");
    }
    if ($data[0] == 0xFF and $data[1] == 0xD8 and $data[2] == 0xFF) {
        return parseJpeg($name, $data);
    }
    if ($data[0] == 0x89 and $data[1] == 0x50 and $data[2] == 0x4E and $data[3] == 0x47 and
        $data[4] == 0x0D and $data[5] == 0x0A and $data[6] == 0x1A and $data[7] == 0x0A) {
        return parsePng($name, $data);
    }
    fail("loadImage: '" + $name + "' is not a PNG or JPEG");
}

/**
 * A copy of the document with an embedded image registered, so `render` writes its
 * image XObject. Draw it on a page with `drawImage`.
 * @param doc {Document} the document
 * @param img {Image} the loaded image (from `loadImage`)
 * @return {Document} a fresh document with the image registered
 */
export func addImage(doc as Document, img as Image) {
    def out as Document init $doc;
    $out.images = lists.push($out.images, $img);
    return $out;
}

/**
 * Draw a registered image scaled into the rectangle at (x, y) of the given width
 * and height (points). The image keeps no aspect ratio of its own - it fills the
 * box - so pass a width / height in the image's own proportion to avoid stretching.
 * `addImage` the image into the document first so the resource name resolves.
 * @param pg {Page} the page
 * @param img {Image} the image (from `loadImage`)
 * @param x {int} the lower-left x in points
 * @param y {int} the lower-left y in points
 * @param width {int} the drawn width in points
 * @param height {int} the drawn height in points
 * @return {Page} a fresh page with the image drawn
 */
export func drawImage(pg as Page, img as Image, x as int, y as int, width as int, height as int) {
    $pg.content = $pg.content + "q\n" + convert.toString($width) + " 0 0 " +
        convert.toString($height) + " " + convert.toString($x) + " " + convert.toString($y) +
        " cm\n/" + $img.name + " Do\nQ\n";
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

# --- text layout (exported) -------------------------------------------------

# measureEm sums a standard-14 font's glyph advances (1000-em units) over a
# WinAnsi-encoded string. Courier is monospaced (600 per byte); Symbol /
# ZapfDingbats have no layout metrics and throw.
func measureEm(font as string, str as string) {
    if (not lists.contains(standardFonts(), $font)) {
        fail("measureText: unknown font '" + $font + "' (use a standard-14 base font)");
    }
    def raw as bytes init encoding.encode($str, "windows-1252");
    def table as list of int init afmWidths($font);
    if (len($table) == 0) {
        if (strings.startsWith($font, "Courier")) {
            return len($raw) * 600;
        }
        fail("measureText: font '" + $font + "' has no layout metrics (Symbol / ZapfDingbats)");
    }
    def total as int init 0;
    def i as int init 0;
    while ($i < len($raw)) {
        $total = $total + $table[$raw[$i]];
        $i = $i + 1;
    }
    return $total;
}

/**
 * Measure the rendered width (in points) of a string in a standard-14 font at a
 * point size, using the Adobe Core-14 AFM metrics. A character outside WinAnsi,
 * or a Symbol / ZapfDingbats font, throws.
 * @param font {string} a standard-14 base font name
 * @param size {int} the font size in points
 * @param str {string} the text to measure
 * @return {float} the width in points
 * @throws {Error} kind "pdf" for an unknown / metric-less font
 */
export func measureText(font as string, size as int, str as string) {
    return (measureEm($font, $str) * $size) / 1000;
}

/**
 * Measure the rendered width (in points) of a string in an embedded font at a
 * point size, using the font's own glyph advances.
 * @param lf {LoadedFont} the embedded font
 * @param size {int} the font size in points
 * @param str {string} the text to measure
 * @return {float} the width in points
 */
export func measureTextUnicode(lf as LoadedFont, size as int, str as string) {
    def upem as int init font.unitsPerEm($lf.f);
    def cps as list of int init [];
    for (def ch in strings.chars($str)) {
        $cps[] = convert.toCodepoint($ch);
    }
    # two whole-font copies for the batch, not two per character
    def advs as list of int init font.advances($lf.f, font.glyphIds($lf.f, $cps));
    def total as int init 0;
    for (def a in $advs) {
        $total = $total + $a;
    }
    return ($total * $size) / $upem;
}

# nonEmptySplit splits on spaces, dropping empty tokens (collapsing whitespace).
func nonEmptySplit(seg as string) {
    def out as list of string init [];
    for (def w in strings.split($seg, " ")) {
        if ($w != "") {
            $out[] = $w;
        }
    }
    return $out;
}

# countWords counts space-separated non-empty tokens in a line.
func countWords(line as string) {
    return len(nonEmptySplit($line));
}

# packSegment greedily packs words (with their point widths and the space width)
# into lines no wider than maxWidth, returning the lines (single-spaced). A word
# wider than maxWidth lands alone on its line (overflow).
func packSegment(words as list of string, wordW as list of float, spaceW as float, maxWidth as int) {
    def lines as list of string init [];
    def cur as string init "";
    def curW as float init 0.0;
    def i as int init 0;
    while ($i < len($words)) {
        if ($cur == "") {
            $cur = $words[$i];
            $curW = $wordW[$i];
        } else {
            def withSpace as float init $curW + $spaceW + $wordW[$i];
            if ($withSpace <= $maxWidth) {
                $cur = $cur + " " + $words[$i];
                $curW = $withSpace;
            } else {
                $lines[] = $cur;
                $cur = $words[$i];
                $curW = $wordW[$i];
            }
        }
        $i = $i + 1;
    }
    $lines[] = $cur;
    return $lines;
}

# wrapSegment wraps one paragraph (no newlines) of standard-14 text to maxWidth,
# always returning at least one line ([""] for an empty paragraph).
func wrapSegment(font as string, size as int, seg as string, maxWidth as int) {
    def words as list of string init nonEmptySplit($seg);
    if (len($words) == 0) {
        return [""];
    }
    def wordW as list of float init [];
    for (def w in $words) {
        $wordW[] = measureText($font, $size, $w);
    }
    return packSegment($words, $wordW, measureText($font, $size, " "), $maxWidth);
}

# wrapSegmentUnicode is wrapSegment for an embedded font.
func wrapSegmentUnicode(lf as LoadedFont, size as int, seg as string, maxWidth as int) {
    def words as list of string init nonEmptySplit($seg);
    if (len($words) == 0) {
        return [""];
    }
    def wordW as list of float init [];
    for (def w in $words) {
        $wordW[] = measureTextUnicode($lf, $size, $w);
    }
    return packSegment($words, $wordW, measureTextUnicode($lf, $size, " "), $maxWidth);
}

# isBreakChar reports whether a hard fold may break right after this character:
# a space or an identifier / URL seam, so a forced break lands at a readable spot.
func isBreakChar(c as string) {
    return $c == " " or $c == "/" or $c == "." or $c == "-" or $c == "_";
}

/**
 * Hard-fold one line so every piece fits `maxWidth` (points). Breaks at the last
 * space or seam punctuation before the overflow, or mid-token when there is none
 * (a bare URL, a slash-joined identifier). Unlike `wrapText`, it does not reflow
 * on every space - it only breaks where a line would otherwise run past the box -
 * so it is the right tool for a code line or an unbreakable token that must stay
 * on the page. A single character wider than `maxWidth` is emitted alone.
 * @param font {string} the standard-14 font
 * @param size {int} the point size
 * @param text {string} the line to fold (no embedded newlines)
 * @param maxWidth {int} the target width in points
 * @return {list of string} the fitted pieces (at least one)
 */
export func foldLine(font as string, size as int, text as string, maxWidth as int) {
    def chars as list of string init strings.chars($text);
    def out as list of string init [];
    def cur as string init "";
    def curW as float init 0.0;
    def brk as int init -1;
    def limit as float init convert.toFloat($maxWidth);
    for (def i in 0..len($chars)) {
        def cw as float init measureText($font, $size, $chars[$i]);
        if ($cur != "" and $curW + $cw > $limit) {
            if ($brk >= 0) {
                $out[] = strings.substring($cur, 0, $brk + 1);
                $cur = strings.substring($cur, $brk + 1, len($cur));
                $curW = measureText($font, $size, $cur);
            } else {
                $out[] = $cur;
                $cur = "";
                $curW = 0.0;
            }
            $brk = -1;
        }
        $cur = $cur + $chars[$i];
        $curW = $curW + $cw;
        if (isBreakChar($chars[$i])) {
            $brk = len($cur) - 1;
        }
    }
    if ($cur != "") {
        $out[] = $cur;
    }
    if (len($out) == 0) {
        $out[] = "";
    }
    return $out;
}

/**
 * Return `s` with every character the standard-14 fonts cannot encode (outside
 * WinAnsi / windows-1252) replaced by `replacement` ("" drops it). A clean string
 * (the common case) is returned unchanged after a single whole-string check, so
 * the per-character path runs only when there is something to replace. This lets
 * a caller keep one out-of-range glyph from aborting a whole render.
 * @param s {string} the text
 * @param replacement {string} the substitute for an un-encodable character
 * @return {string} the WinAnsi-safe text
 */
export func toWinAnsi(s as string, replacement as string) {
    def clean as bool init true;
    try {
        encoding.encode($s, "windows-1252");
    } catch (e) {
        $clean = false;
    }
    if ($clean) {
        return $s;
    }
    def out as list of string init [];
    for (def ch in strings.chars($s)) {
        def ok as bool init true;
        try {
            encoding.encode($ch, "windows-1252");
        } catch (e) {
            $ok = false;
        }
        if ($ok) {
            $out[] = $ch;
        } else {
            $out[] = $replacement;
        }
    }
    return strings.join($out, "");
}

/**
 * Word-wrap a standard-14 string to a maximum line width (points), returning the
 * lines. Existing newlines are honoured as hard breaks; runs of spaces collapse.
 * A word-wrapped line still wider than the box (an unbreakable token) is
 * hard-folded with `foldLine`, so no output line runs past `maxWidth`.
 * @param font {string} a standard-14 base font name
 * @param size {int} the font size in points
 * @param str {string} the text to wrap
 * @param maxWidth {int} the maximum line width in points
 * @return {list of string} the wrapped lines
 */
export func wrapText(font as string, size as int, str as string, maxWidth as int) {
    def out as list of string init [];
    for (def seg in strings.split($str, "\n")) {
        for (def ln in wrapSegment($font, $size, $seg, $maxWidth)) {
            # A word-wrapped line still wider than the box holds an unbreakable
            # token (no space to break on); hard-fold it so it cannot run off the
            # page (fixes over-wide table cells and any other wrapText caller).
            if (measureText($font, $size, $ln) > convert.toFloat($maxWidth)) {
                for (def piece in foldLine($font, $size, $ln, $maxWidth)) {
                    $out[] = $piece;
                }
            } else {
                $out[] = $ln;
            }
        }
    }
    return $out;
}

/**
 * Word-wrap an embedded-font string to a maximum line width (points).
 * @param lf {LoadedFont} the embedded font
 * @param size {int} the font size in points
 * @param str {string} the text to wrap
 * @param maxWidth {int} the maximum line width in points
 * @return {list of string} the wrapped lines
 */
export func wrapTextUnicode(lf as LoadedFont, size as int, str as string, maxWidth as int) {
    def out as list of string init [];
    for (def seg in strings.split($str, "\n")) {
        for (def ln in wrapSegmentUnicode($lf, $size, $seg, $maxWidth)) {
            $out[] = $ln;
        }
    }
    return $out;
}

func validAlign(align as string) {
    return $align == "left" or $align == "right" or $align == "center" or $align == "justify";
}

# alignStart returns the left x of a line of width lineW inside an x..x+width box.
func alignStart(x as int, width as int, lineW as float, align as string) {
    if ($align == "right") {
        return $x + math.round($width - $lineW);
    }
    if ($align == "center") {
        return $x + math.round(($width - $lineW) / 2);
    }
    return $x;
}

# drawJustifiedStd draws one standard-14 line justified across `width` by padding
# the inter-word gaps evenly (word positions computed, each word placed by `text`).
func drawJustifiedStd(pg as Page, x as int, baseline as int, width as int, font as string, size as int, line as string) {
    def words as list of string init nonEmptySplit($line);
    def n as int init len($words);
    def natural as float init measureText($font, $size, $line);
    def gap as float init ($width - $natural) / (0 + $n - 1);
    def spaceW as float init measureText($font, $size, " ");
    def cx as float init convert.toFloat($x);
    def k as int init 0;
    while ($k < $n) {
        $pg = text($pg, math.round($cx), $baseline, $font, $size, $words[$k]);
        $cx = $cx + measureText($font, $size, $words[$k]) + $spaceW + $gap;
        $k = $k + 1;
    }
    return $pg;
}

# drawJustifiedUni is drawJustifiedStd for an embedded font.
func drawJustifiedUni(pg as Page, x as int, baseline as int, width as int, lf as LoadedFont, size as int, line as string) {
    def words as list of string init nonEmptySplit($line);
    def n as int init len($words);
    def natural as float init measureTextUnicode($lf, $size, $line);
    def gap as float init ($width - $natural) / (0 + $n - 1);
    def spaceW as float init measureTextUnicode($lf, $size, " ");
    def cx as float init convert.toFloat($x);
    def k as int init 0;
    while ($k < $n) {
        $pg = textUnicode($pg, math.round($cx), $baseline, $lf, $size, $words[$k]);
        $cx = $cx + measureTextUnicode($lf, $size, $words[$k]) + $spaceW + $gap;
        $k = $k + 1;
    }
    return $pg;
}

/**
 * Flow standard-14 text into a column: word-wrap `str` to `width` points and draw
 * each line, the first line's baseline at (x, y) and each subsequent line
 * `leading` points lower. `align` is "left" / "right" / "center" / "justify"
 * (justify pads inter-word gaps on every line but the last of each paragraph).
 * The drawn block is `len(wrapText(...)) * leading` points tall.
 * @param pg {Page} the page
 * @param x {int} the column's left x in points
 * @param y {int} the first line's baseline y in points
 * @param width {int} the column width in points
 * @param font {string} a standard-14 base font name
 * @param size {int} the font size in points
 * @param leading {int} the line-to-line spacing in points
 * @param str {string} the text (newlines are hard breaks)
 * @param align {string} "left" / "right" / "center" / "justify"
 * @return {Page} a fresh page with the text block drawn
 * @throws {Error} kind "pdf" for an unknown align or font
 */
export func textBlock(pg as Page, x as int, y as int, width as int, font as string, size as int, leading as int, str as string, align as string) {
    if (not validAlign($align)) {
        fail("textBlock: unknown alignment '" + $align + "' (left / right / center / justify)");
    }
    def lineIdx as int init 0;
    for (def seg in strings.split($str, "\n")) {
        def plines as list of string init wrapSegment($font, $size, $seg, $width);
        def j as int init 0;
        while ($j < len($plines)) {
            def line as string init $plines[$j];
            def baseline as int init $y - $lineIdx * $leading;
            if ($align == "justify" and $j < len($plines) - 1 and countWords($line) > 1) {
                $pg = drawJustifiedStd($pg, $x, $baseline, $width, $font, $size, $line);
            } elseif ($line != "") {
                $pg = text($pg, alignStart($x, $width, measureText($font, $size, $line), $align),
                    $baseline, $font, $size, $line);
            }
            $lineIdx = $lineIdx + 1;
            $j = $j + 1;
        }
    }
    return $pg;
}

/**
 * Flow embedded-font (Unicode) text into a column - `textBlock` for a font from
 * `loadFont` / `addFont`. Same wrapping / alignment / leading behaviour.
 * @param pg {Page} the page
 * @param x {int} the column's left x in points
 * @param y {int} the first line's baseline y in points
 * @param width {int} the column width in points
 * @param lf {LoadedFont} the embedded font
 * @param size {int} the font size in points
 * @param leading {int} the line-to-line spacing in points
 * @param str {string} the text (newlines are hard breaks)
 * @param align {string} "left" / "right" / "center" / "justify"
 * @return {Page} a fresh page with the text block drawn
 * @throws {Error} kind "pdf" for an unknown align
 */
export func textBlockUnicode(pg as Page, x as int, y as int, width as int, lf as LoadedFont, size as int, leading as int, str as string, align as string) {
    if (not validAlign($align)) {
        fail("textBlockUnicode: unknown alignment '" + $align + "' (left / right / center / justify)");
    }
    def lineIdx as int init 0;
    for (def seg in strings.split($str, "\n")) {
        def plines as list of string init wrapSegmentUnicode($lf, $size, $seg, $width);
        def j as int init 0;
        while ($j < len($plines)) {
            def line as string init $plines[$j];
            def baseline as int init $y - $lineIdx * $leading;
            if ($align == "justify" and $j < len($plines) - 1 and countWords($line) > 1) {
                $pg = drawJustifiedUni($pg, $x, $baseline, $width, $lf, $size, $line);
            } elseif ($line != "") {
                $pg = textUnicode($pg, alignStart($x, $width, measureTextUnicode($lf, $size, $line), $align),
                    $baseline, $lf, $size, $line);
            }
            $lineIdx = $lineIdx + 1;
            $j = $j + 1;
        }
    }
    return $pg;
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

# --- embedded-font helpers (private) ----------------------------------------

# scaleMetric converts a value in font units to the 1000-unit em PDF text space.
func scaleMetric(v as int, upem as int) {
    def q as int init $v * 1000;
    if ($q >= 0) {
        return ($q + $upem // 2) // $upem;
    }
    return 0 - ((0 - $q + $upem // 2) // $upem);
}

# aggregateGlyphs collects every glyph an embedded font drew across all pages,
# returning a map from glyph id to a representative source codepoint. It takes the
# pages (not the whole Document) so it does not copy every embedded font's bytes
# on each call.
func aggregateGlyphs(pages as list of Page, fontName as string) {
    def m as map of int to int init {};
    for (def pg in $pages) {
        for (def gu in $pg.glyphUses) {
            if ($gu.font == $fontName and not maps.has($m, $gu.gid)) {
                $m[$gu.gid] = $gu.cp;
            }
        }
    }
    return $m;
}

# sortedGids returns a glyph-map's ids in ascending order.
func sortedGids(m as map of int to int) {
    def ks as list of int init [];
    for (def k in $m) {
        $ks[] = $k;
    }
    return lists.sort($ks);
}

# buildW renders a CIDFontType2 `W` widths array for the used glyphs (each in
# 1000-unit em space).
func buildW(fe as font.Font, gids as list of int, upem as int) {
    def advs as list of int init font.advances($fe, $gids);   # one font copy, not one per glyph
    def parts as list of string init [];
    def i as int init 0;
    while ($i < len($gids)) {
        def w as int init scaleMetric($advs[$i], $upem);
        $parts[] = convert.toString($gids[$i]) + " [" + convert.toString($w) + "]";
        $i = $i + 1;
    }
    return strings.join($parts, " ");
}

# toUnicodeHex renders a codepoint as UTF-16BE hex (a surrogate pair above the BMP).
func toUnicodeHex(cp as int) {
    def b as bytes;
    if ($cp < 0x10000) {
        $b[] = ($cp >> 8) & 0xff;
        $b[] = $cp & 0xff;
    } else {
        def v as int init $cp - 0x10000;
        def hi as int init 0xD800 + (($v >> 10) & 0x3FF);
        def lo as int init 0xDC00 + ($v & 0x3FF);
        $b[] = ($hi >> 8) & 0xff;
        $b[] = $hi & 0xff;
        $b[] = ($lo >> 8) & 0xff;
        $b[] = $lo & 0xff;
    }
    return strings.upper(encoding.toText($b, "hex"));
}

# buildToUnicode builds the ToUnicode CMap stream body mapping each glyph id to
# its Unicode value, so a viewer can extract / copy the embedded text.
func buildToUnicode(gids as list of int, m as map of int to int) {
    def out as string init "/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n" +
        "/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n" +
        "/CMapName /Adobe-Identity-UCS def\n/CMapType 2 def\n" +
        "1 begincodespacerange <0000> <FFFF> endcodespacerange\n";
    def i as int init 0;
    def n as int init len($gids);
    while ($i < $n) {
        def cnt as int init $n - $i;
        if ($cnt > 100) {
            $cnt = 100;
        }
        $out = $out + convert.toString($cnt) + " beginbfchar\n";
        def j as int init 0;
        while ($j < $cnt) {
            def gid as int init $gids[$i + $j];
            $out = $out + "<" + hexGid($gid) + "> <" + toUnicodeHex($m[$gid]) + ">\n";
            $j = $j + 1;
        }
        $out = $out + "endbfchar\n";
        $i = $i + $cnt;
    }
    return $out + "endcmap\nCMapName currentdict /CMap defineresource pop\nend\nend\n";
}

# escapePdfString escapes a string for a PDF literal `(...)` string: backslash,
# and the two parentheses (kept ASCII / Latin-1, matching the standard-14 scope).
func escapePdfString(s as string) {
    def out as string init strings.replace($s, "\\", "\\\\");
    $out = strings.replace($out, "(", "\\(");
    $out = strings.replace($out, ")", "\\)");
    return $out;
}

# outlineField renders `/Key N 0 R` when N is a real object number, else "".
func outlineField(key as string, obj as int) {
    if ($obj > 0) {
        return " /" + $key + " " + convert.toString($obj) + " 0 R";
    }
    return "";
}

# buildOutlineObjects turns the flat, level-ordered outline entries into the PDF
# object bodies (the /Outlines root first, then one item per entry, in object-
# number order). Nesting is derived from `level`: an entry's parent is the nearest
# preceding entry of a smaller level; siblings share a parent; /Count is the run of
# following deeper entries. Returns the object strings ready to emit.
func buildOutlineObjects(outline as list of OutlineEntry, rootNum as int, itemNums as list of int, pageNum as list of int) {
    def n as int init len($outline);
    def numPages as int init len($pageNum);
    # parent index per entry (-1 = directly under the root)
    def parentIdx as list of int init [];
    def i as int init 0;
    while ($i < $n) {
        def par as int init -1;
        def j as int init $i - 1;
        while ($j >= 0) {
            if ($outline[$j].level < $outline[$i].level) {
                $par = $j;
                break;
            }
            $j = $j - 1;
        }
        $parentIdx[] = $par;
        $i = $i + 1;
    }
    # root children (parent == -1): first / last object numbers
    def rootFirst as int init 0;
    def rootLast as int init 0;
    $i = 0;
    while ($i < $n) {
        if ($parentIdx[$i] == -1) {
            if ($rootFirst == 0) {
                $rootFirst = $itemNums[$i];
            }
            $rootLast = $itemNums[$i];
        }
        $i = $i + 1;
    }
    def objs as list of string init [];
    $objs[] = convert.toString($rootNum) + " 0 obj\n<< /Type /Outlines" +
        outlineField("First", $rootFirst) + outlineField("Last", $rootLast) +
        " /Count " + convert.toString($n) + " >>\nendobj\n";
    # each item
    $i = 0;
    while ($i < $n) {
        def par as int init $rootNum;
        if ($parentIdx[$i] != -1) {
            $par = $itemNums[$parentIdx[$i]];
        }
        # previous / next sibling (same parent), first / last child, descendant count
        def prevSib as int init 0;
        def nextSib as int init 0;
        def firstCh as int init 0;
        def lastCh as int init 0;
        def cnt as int init 0;
        def j as int init 0;
        while ($j < $n) {
            if ($j != $i and $parentIdx[$j] == $parentIdx[$i]) {
                if ($j < $i) {
                    $prevSib = $itemNums[$j];
                } elseif ($nextSib == 0) {
                    $nextSib = $itemNums[$j];
                }
            }
            if ($parentIdx[$j] == $i) {
                if ($firstCh == 0) {
                    $firstCh = $itemNums[$j];
                }
                $lastCh = $itemNums[$j];
            }
            $j = $j + 1;
        }
        def k as int init $i + 1;
        while ($k < $n and $outline[$k].level > $outline[$i].level) {
            $cnt = $cnt + 1;
            $k = $k + 1;
        }
        def pageIdx as int init $outline[$i].page;
        if ($pageIdx < 0) {
            $pageIdx = 0;
        }
        if ($pageIdx >= $numPages) {
            $pageIdx = $numPages - 1;
        }
        def countField as string init "";
        if ($cnt > 0) {
            $countField = " /Count " + convert.toString($cnt);
        }
        $objs[] = convert.toString($itemNums[$i]) + " 0 obj\n<< /Title (" + escapePdfString($outline[$i].title) +
            ") /Parent " + convert.toString($par) + " 0 R" +
            outlineField("Prev", $prevSib) + outlineField("Next", $nextSib) +
            outlineField("First", $firstCh) + outlineField("Last", $lastCh) + $countField +
            " /Dest [" + convert.toString($pageNum[$pageIdx]) + " 0 R /XYZ null " + convert.toString($outline[$i].y) + " null]" +
            " >>\nendobj\n";
        $i = $i + 1;
    }
    return $objs;
}

# substPage fills the %page% / %pages% placeholders of a header / footer slot.
# The tokens are percent-delimited (like `intl`), not brace-delimited, so they do
# not collide with Jennifer's own `{expr}` string interpolation.
func substPage(s as string, pageNr as int, total as int) {
    def out as string init strings.replace($s, "%page%", convert.toString($pageNr));
    return strings.replace($out, "%pages%", convert.toString($total));
}

# drawLabel draws one header (isHeader) or footer onto a page: the left / centre /
# right slots (alignment measured with the font metrics), the colour, and an
# optional separating rule. Placeholders are already resolved for (pageNr, total).
func drawLabel(pg as Page, label as PageLabel, pageNr as int, total as int, isHeader as bool) {
    def baseline as int init $label.margin;
    if ($isHeader) {
        $baseline = $pg.height - $label.margin;
    }
    def left as string init substPage($label.left, $pageNr, $total);
    def center as string init substPage($label.center, $pageNr, $total);
    def right as string init substPage($label.right, $pageNr, $total);
    # Set the label colour explicitly (the body content may have left a different
    # fill / stroke colour), then restore black afterwards.
    $pg = color($pg, $label.red, $label.green, $label.blue);
    if (len($left) > 0) {
        $pg = text($pg, $label.margin, $baseline, $label.font, $label.size, $left);
    }
    if (len($center) > 0) {
        def cw as int init math.round(measureText($label.font, $label.size, $center));
        $pg = text($pg, ($pg.width - $cw) // 2, $baseline, $label.font, $label.size, $center);
    }
    if (len($right) > 0) {
        def rw as int init math.round(measureText($label.font, $label.size, $right));
        $pg = text($pg, $pg.width - $label.margin - $rw, $baseline, $label.font, $label.size, $right);
    }
    if ($label.border) {
        # A rule below a header, above a footer.
        def ly as int init $baseline + $label.size;
        if ($isHeader) {
            $ly = $baseline - $label.size // 2;
        }
        $pg = line($pg, $label.margin, $ly, $pg.width - $label.margin, $ly);
    }
    $pg = color($pg, 0, 0, 0);
    return $pg;
}

# applyHeadersFooters bakes the running header / footer onto every page just before
# serialization, once the total page count is known. Drawing through `text` / `line`
# registers the label font in each page's resources, so the later font collection
# sees it. A no-op when neither is set.
func applyHeadersFooters(doc as Document) {
    if (not $doc.headerOn and not $doc.footerOn) {
        return $doc;
    }
    def total as int init len($doc.pages);
    def out as list of Page init [];
    def p as int init 0;
    while ($p < $total) {
        def pg as Page init $doc.pages[$p];
        if ($doc.headerOn) {
            $pg = drawLabel($pg, $doc.header, $p + 1, $total, true);
        }
        if ($doc.footerOn) {
            $pg = drawLabel($pg, $doc.footer, $p + 1, $total, false);
        }
        $out[] = $pg;
        $p = $p + 1;
    }
    $doc.pages = $out;
    return $doc;
}

/**
 * Render the document to PDF bytes (PDF 1.7). Any running header / footer is drawn
 * onto every page first. Content streams are FlateDecode-compressed; standard-14
 * fonts become shared Type1 objects and each embedded font a Type0 / CIDFontType2
 * with an embedded FontFile2 and a ToUnicode map. Registered images become image
 * XObjects (with a soft-mask XObject when they carry alpha). An outline (bookmarks)
 * is emitted when the document has any, with the catalog set to open the bookmark
 * panel. Objects are numbered dynamically, so any mix of pages, fonts, images, and
 * resources references correctly.
 * @param doc {Document} the document to render
 * @return {bytes} the PDF file contents
 */
export func render(doc as Document) {
    $doc = applyHeadersFooters($doc);
    def numPages as int init len($doc.pages);
    def fontNames as list of string init collectFonts($doc);
    def numFonts as int init len($fontNames);
    def numEmbed as int init len($doc.embedded);
    def numImages as int init len($doc.images);
    def hasInfo as bool init len($doc.info) > 0;

    # --- assign object numbers: catalog, pages, per-page (dict + content),
    # standard-14 fonts, embedded fonts (5 objects each), info ---
    def next as int init 3;
    def pageNum as list of int init [];
    def contentNum as list of int init [];
    def p as int init 0;
    while ($p < $numPages) {
        $pageNum[] = $next; $next = $next + 1;
        $contentNum[] = $next; $next = $next + 1;
        $p = $p + 1;
    }
    def std14Base as int init $next;
    $next = $next + $numFonts;
    # per embedded font, in emission order: FontFile2, FontDescriptor,
    # CIDFontType2, Type0, ToUnicode.
    def embFile as list of int init [];
    def embDesc as list of int init [];
    def embCid as list of int init [];
    def embType0 as list of int init [];
    def embToUni as list of int init [];
    def e as int init 0;
    while ($e < $numEmbed) {
        $embFile[] = $next; $next = $next + 1;
        $embDesc[] = $next; $next = $next + 1;
        $embCid[] = $next; $next = $next + 1;
        $embType0[] = $next; $next = $next + 1;
        $embToUni[] = $next; $next = $next + 1;
        $e = $e + 1;
    }
    # per image: the image XObject, plus a soft-mask XObject when it has alpha.
    def imgNum as list of int init [];
    def smaskNum as list of int init [];
    def im as int init 0;
    while ($im < $numImages) {
        $imgNum[] = $next; $next = $next + 1;
        if ($doc.images[$im].hasSmask) {
            $smaskNum[] = $next; $next = $next + 1;
        } else {
            $smaskNum[] = 0;
        }
        $im = $im + 1;
    }
    def infoNum as int init 0;
    if ($hasInfo) {
        $infoNum = $next; $next = $next + 1;
    }
    # outline: the /Outlines root, then one object per entry (emitted last)
    def hasOutline as bool init len($doc.outline) > 0;
    def outlineRoot as int init 0;
    def outlineItems as list of int init [];
    if ($hasOutline) {
        $outlineRoot = $next; $next = $next + 1;
        def oi as int init 0;
        while ($oi < len($doc.outline)) {
            $outlineItems[] = $next; $next = $next + 1;
            $oi = $oi + 1;
        }
    }
    def totalObjs as int init $next - 1;

    def buf as bytes;
    def offsets as list of int init [];

    # header + binary marker comment (four high bytes)
    $buf = appendStr($buf, "%PDF-1.7\n");
    $buf[] = 0x25; $buf[] = 0xE2; $buf[] = 0xE3; $buf[] = 0xCF; $buf[] = 0xD3; $buf[] = 0x0A;

    # obj 1: catalog
    $offsets[] = len($buf);
    def catExtra as string init "";
    if ($hasOutline) {
        $catExtra = " /Outlines " + convert.toString($outlineRoot) + " 0 R /PageMode /UseOutlines";
    }
    $buf = appendStr($buf, "1 0 obj\n<< /Type /Catalog /Pages 2 0 R" + $catExtra + " >>\nendobj\n");

    # obj 2: page tree
    def kids as string init "";
    $p = 0;
    while ($p < $numPages) {
        if ($p > 0) {
            $kids = $kids + " ";
        }
        $kids = $kids + convert.toString($pageNum[$p]) + " 0 R";
        $p = $p + 1;
    }
    $offsets[] = len($buf);
    $buf = appendStr(
        $buf,
        "2 0 obj\n<< /Type /Pages /Kids [" + $kids + "] /Count " + convert.toString($numPages) +
            " >>\nendobj\n");

    # per page: the page dict and its (compressed) content stream
    $p = 0;
    while ($p < $numPages) {
        def pg as Page init $doc.pages[$p];
        def fontDict as string init "";
        for (def fn in $pg.fonts) {
            $fontDict = $fontDict + "/" + $fn + " " +
                convert.toString(fontObjNum($fontNames, $fn, $std14Base)) + " 0 R ";
        }
        # every embedded font is listed on every page (harmless if unused there)
        def ei as int init 0;
        while ($ei < $numEmbed) {
            $fontDict = $fontDict + "/" + $doc.embedded[$ei].name + " " +
                convert.toString($embType0[$ei]) + " 0 R ";
            $ei = $ei + 1;
        }
        def res as string init "/Font << " + $fontDict + ">>";
        if ($numImages > 0) {
            def xobjDict as string init "";
            def xi as int init 0;
            while ($xi < $numImages) {
                $xobjDict = $xobjDict + "/" + $doc.images[$xi].name + " " +
                    convert.toString($imgNum[$xi]) + " 0 R ";
                $xi = $xi + 1;
            }
            $res = $res + " /XObject << " + $xobjDict + ">>";
        }
        $offsets[] = len($buf);
        $buf = appendStr(
            $buf,
            convert.toString($pageNum[$p]) + " 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 " +
                convert.toString($pg.width) + " " + convert.toString($pg.height) +
                "] /Resources << " + $res + " >> /Contents " + convert.toString($contentNum[$p]) +
                " 0 R >>\nendobj\n");

        def comp as bytes init compress.pack(convert.bytesFromString($pg.content, "utf-8"), "zlib");
        $offsets[] = len($buf);
        $buf = appendStr(
            $buf,
            convert.toString($contentNum[$p]) + " 0 obj\n<< /Length " + convert.toString(len($comp)) +
                " /Filter /FlateDecode >>\nstream\n");
        $buf = appendBytes($buf, $comp);
        $buf = appendStr($buf, "\nendstream\nendobj\n");
        $p = $p + 1;
    }

    # standard-14 font objects (shared)
    def f as int init 0;
    while ($f < $numFonts) {
        $offsets[] = len($buf);
        # Symbol and ZapfDingbats carry their own built-in encodings; forcing
        # /WinAnsiEncoding on them remaps their glyphs, so omit /Encoding there.
        def enc as string init " /Encoding /WinAnsiEncoding";
        if ($fontNames[$f] == "Symbol" or $fontNames[$f] == "ZapfDingbats") {
            $enc = "";
        }
        $buf = appendStr(
            $buf,
            convert.toString($std14Base + $f) +
                " 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /" +
                $fontNames[$f] + $enc + " >>\nendobj\n");
        $f = $f + 1;
    }

    # embedded fonts: FontFile2, FontDescriptor, CIDFontType2, Type0, ToUnicode
    $e = 0;
    while ($e < $numEmbed) {
        def lf as LoadedFont init $doc.embedded[$e];
        def fe as font.Font init $lf.f;
        def upem as int init font.unitsPerEm($fe);
        def base as string init pdfName(font.name($fe));
        def gmap as map of int to int init aggregateGlyphs($doc.pages, $lf.name);
        def gids as list of int init sortedGids($gmap);

        # FontFile2 (the raw font program, FlateDecode-compressed)
        def raw as bytes init font.data($fe);
        def fcomp as bytes init compress.pack($raw, "zlib");
        $offsets[] = len($buf);
        $buf = appendStr(
            $buf,
            convert.toString($embFile[$e]) + " 0 obj\n<< /Length " + convert.toString(len($fcomp)) +
                " /Length1 " + convert.toString(len($raw)) + " /Filter /FlateDecode >>\nstream\n");
        $buf = appendBytes($buf, $fcomp);
        $buf = appendStr($buf, "\nendstream\nendobj\n");

        # FontDescriptor
        def bb as list of int init font.bbox($fe);
        def cap as int init font.capHeight($fe);
        if ($cap == 0) {
            $cap = font.ascender($fe);
        }
        $offsets[] = len($buf);
        $buf = appendStr(
            $buf,
            convert.toString($embDesc[$e]) + " 0 obj\n<< /Type /FontDescriptor /FontName /" + $base +
                " /Flags 4 /FontBBox [" + convert.toString(scaleMetric($bb[0], $upem)) + " " +
                convert.toString(scaleMetric($bb[1], $upem)) + " " +
                convert.toString(scaleMetric($bb[2], $upem)) + " " +
                convert.toString(scaleMetric($bb[3], $upem)) +
                "] /ItalicAngle 0 /Ascent " + convert.toString(scaleMetric(font.ascender($fe), $upem)) +
                " /Descent " + convert.toString(scaleMetric(font.descender($fe), $upem)) +
                " /CapHeight " + convert.toString(scaleMetric($cap, $upem)) +
                " /StemV 80 /FontFile2 " + convert.toString($embFile[$e]) + " 0 R >>\nendobj\n");

        # CIDFontType2 (descendant)
        $offsets[] = len($buf);
        $buf = appendStr(
            $buf,
            convert.toString($embCid[$e]) +
                " 0 obj\n<< /Type /Font /Subtype /CIDFontType2 /BaseFont /" + $base +
                " /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >>" +
                " /FontDescriptor " + convert.toString($embDesc[$e]) +
                " 0 R /CIDToGIDMap /Identity /DW 1000 /W [" + buildW($fe, $gids, $upem) +
                "] >>\nendobj\n");

        # Type0 (composite)
        $offsets[] = len($buf);
        $buf = appendStr(
            $buf,
            convert.toString($embType0[$e]) +
                " 0 obj\n<< /Type /Font /Subtype /Type0 /BaseFont /" + $base +
                " /Encoding /Identity-H /DescendantFonts [" + convert.toString($embCid[$e]) +
                " 0 R] /ToUnicode " + convert.toString($embToUni[$e]) + " 0 R >>\nendobj\n");

        # ToUnicode CMap
        def cmap as bytes init convert.bytesFromString(buildToUnicode($gids, $gmap), "utf-8");
        $offsets[] = len($buf);
        $buf = appendStr(
            $buf,
            convert.toString($embToUni[$e]) + " 0 obj\n<< /Length " + convert.toString(len($cmap)) +
                " >>\nstream\n");
        $buf = appendBytes($buf, $cmap);
        $buf = appendStr($buf, "\nendstream\nendobj\n");
        $e = $e + 1;
    }

    # image XObjects: each image, plus a soft-mask XObject when it has alpha
    $im = 0;
    while ($im < $numImages) {
        def img as Image init $doc.images[$im];
        def smaskRef as string init "";
        if ($img.hasSmask) {
            $smaskRef = " /SMask " + convert.toString($smaskNum[$im]) + " 0 R";
        }
        def dp as string init "";
        if ($img.predictor == 15) {
            $dp = " /DecodeParms << /Predictor 15 /Colors " + convert.toString($img.colors) +
                " /BitsPerComponent " + convert.toString($img.bits) + " /Columns " +
                convert.toString($img.width) + " >>";
        }
        $offsets[] = len($buf);
        $buf = appendStr(
            $buf,
            convert.toString($imgNum[$im]) +
                " 0 obj\n<< /Type /XObject /Subtype /Image /Width " + convert.toString($img.width) +
                " /Height " + convert.toString($img.height) + " /ColorSpace " + $img.colorSpace +
                " /BitsPerComponent " + convert.toString($img.bits) + " /Filter /" + $img.filter +
                $dp + $img.decode + $smaskRef + " /Length " + convert.toString(len($img.data)) +
                " >>\nstream\n");
        $buf = appendBytes($buf, $img.data);
        $buf = appendStr($buf, "\nendstream\nendobj\n");

        if ($img.hasSmask) {
            $offsets[] = len($buf);
            $buf = appendStr(
                $buf,
                convert.toString($smaskNum[$im]) +
                    " 0 obj\n<< /Type /XObject /Subtype /Image /Width " +
                    convert.toString($img.width) + " /Height " + convert.toString($img.height) +
                    " /ColorSpace /DeviceGray /BitsPerComponent 8 /Filter /FlateDecode /Length " +
                    convert.toString(len($img.smask)) + " >>\nstream\n");
            $buf = appendBytes($buf, $img.smask);
            $buf = appendStr($buf, "\nendstream\nendobj\n");
        }
        $im = $im + 1;
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

    # outline (bookmark) objects: the /Outlines root, then one per entry, in
    # object-number order (so the offsets stay in step).
    if ($hasOutline) {
        def outObjs as list of string init buildOutlineObjects($doc.outline, $outlineRoot, $outlineItems, $pageNum);
        for (def obj in $outObjs) {
            $offsets[] = len($buf);
            $buf = appendStr($buf, $obj);
        }
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
    $buf = appendStr(
        $buf,
        "trailer\n<< /Size " + convert.toString($totalObjs + 1) + " /Root 1 0 R" + $trailerInfo +
            " >>\nstartxref\n" + convert.toString($xrefOffset) + "\n%%EOF\n");
    return $buf;
}
