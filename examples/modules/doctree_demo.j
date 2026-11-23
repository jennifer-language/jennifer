#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

/**
 * The doctree module (modules/doctree.j): build a small document tree by hand
 * (headings, paragraphs with inline markup and a link, a bullet list, a table
 * and a code block), walk it with the selector accessors, and render it as
 * HTML, XHTML, terminal text and a paginated PDF with a page-number footer.
 * The PDF goes to the system temp directory, never the current one.
 * @module doctree_demo
 */
use io;
use fs;
use os;
use path;
import "../../modules/doctree.j" as doctree;
import "../../modules/pdf.j" as pdf;

/**
 * A node of the given kind with its text and children; every other field
 * empty. The literal names all fields, so the demo also runs against a
 * `modules/` that is not the installed one.
 * @param kind {string} the node kind
 * @param text {string} a leaf's text
 * @param children {list of doctree.Node} the child nodes
 * @return {doctree.Node} the node
 */
func node(kind as string, text as string, children as list of doctree.Node) {
    def attrs as map of string to string init {};
    return doctree.Node{
        kind: $kind,
        level: 0,
        text: $text,
        lang: "",
        ordered: false,
        url: "",
        title: "",
        align: "",
        attrs: $attrs,
        children: $children
    };
}

/**
 * A run of plain text.
 * @param s {string} the text
 * @return {doctree.Node} a "text" node
 */
func txt(s as string) {
    return node("text", $s, []);
}

/**
 * A heading.
 * @param level {int} the heading level, 1-6
 * @param s {string} the heading text
 * @return {doctree.Node} a "heading" node
 */
func heading(level as int, s as string) {
    def h as doctree.Node init node("heading", "", [txt($s)]);
    $h.level = $level;
    return $h;
}

/**
 * A table cell holding plain text.
 * @param s {string} the cell text
 * @param align {string} "left", "right", "center" or ""
 * @return {doctree.Node} a "cell" node
 */
func cell(s as string, align as string) {
    def c as doctree.Node init node("cell", "", [txt($s)]);
    $c.align = $align;
    return $c;
}

/**
 * A table row of a name, a size and a note.
 * @param name {string} the first column
 * @param size {string} the second column, right-aligned
 * @param note {string} the third column
 * @return {doctree.Node} a "row" node
 */
func row(name as string, size as string, note as string) {
    return node("row", "", [cell($name, "left"), cell($size, "right"), cell($note, "left")]);
}

# Inline markup: strong, emphasis, inline code and a link.
def link as doctree.Node init node("link", "", [txt("the guide")]);
$link.url = "https://www.jennifer-lang.dev/docs/modules/doctree.html";
def intro as doctree.Node init node(
    "paragraph",
    "",
    [
        txt("A "),
        node("strong", "", [txt("doctree")]),
        txt(" is built by a reader or "),
        node("emphasis", "", [txt("by hand")]),
        txt(", then rendered with "),
        node("codespan", "render", []),
        txt(". See "),
        $link,
        txt(".")
    ]);

# A bullet list; an item holds inline children.
def steps as doctree.Node init node(
    "list",
    "",
    [
        node("item", "", [txt("build the tree")]),
        node("item", "", [txt("walk it with "), node("codespan", "findAll", [])]),
        node("item", "", [txt("render it")])
    ]);

# A table: the first row is the header (attr "headerRows" defaults to 1), the
# title is its caption.
def table as doctree.Node init node(
    "table",
    "",
    [
        row("Format", "Calls", "Output"),
        row("html", "1", "compact markup"),
        row("ansi", "1", "terminal text"),
        row("pdf", "2", "paginated bytes")
    ]);
$table.title = "What each renderer gives";

# A fenced code block with its language.
def code as doctree.Node init node(
    "code",
    "def doc as doctree.Node init markdown.parse($src);\nio.printf(\"%s\\n\", doctree.render(\"html\", $doc));",
    []);
$code.lang = "jennifer";

def doc as doctree.Node init node(
    "document",
    "",
    [
        heading(1, "Document trees"),
        $intro,
        heading(2, "Steps"),
        $steps,
        heading(2, "Renderers"),
        $table,
        heading(2, "From Markdown"),
        $code
    ]);

# Walk the tree. Selector steps match direct children: "*" is any kind.
io.printf("=== outline ===\n");
for (def h in doctree.findAll($doc, "heading")) {
    io.printf("  H%d: %s\n", doctree.level($h), doctree.text($h));
}
def spans as list of string init [];
for (def s in doctree.findAll($doc, "*/*/codespan")) {
    $spans[] = doctree.text($s);
}
for (def s in doctree.findAll($doc, "paragraph/codespan")) {
    $spans[] = doctree.text($s);
}
io.printf("  code spans: %v\n", $spans);
def a as doctree.Node init doctree.get($doc, "paragraph/link");
io.printf("  link: %s -> %s\n", doctree.text($a), doctree.attr($a, "href"));
io.printf(
    "  table rows: %d, code language: %s\n",
    len(doctree.findAll($doc, "table/row")),
    doctree.attr(doctree.get($doc, "code"), "lang"));

# HTML: compact, with every text and link target escaped.
io.printf("\n=== HTML ===\n%s\n", doctree.render("html", $doc));

# XHTML writes a cell's alignment as a style, as EPUB 3 needs.
def xhtml as string init doctree.render("xhtml", doctree.get($doc, "table"));
io.printf("\n=== XHTML (the table only) ===\n%s\n", $xhtml);

# Terminal text: styled on a TTY, plain when piped.
io.printf("\n=== ANSI ===\n%s\n", doctree.render("ansi", $doc));

# PDF: tweak the defaults, lay the tree out to a pdf.Document, add a footer with
# page numbers, then serialize.
def opts as doctree.PdfOptions init doctree.pdfDefaults();
$opts.title = "Document trees";
$opts.bookmarkLevel = 2;
$opts.tableHeaderFill = doctree.gray(232);
$opts.codeFill = doctree.gray(244);
$opts.headingStyles = [
    doctree.headingStyle(doctree.rgb(220, 230, 245)),
    doctree.headingStyle(doctree.noFill())
];
def laidOut as pdf.Document init doctree.renderPdfDoc($doc, $opts);
def footer as pdf.PageLabel init pdf.pageLabel();
$footer.right = "%page%/%pages%";
def pdfBytes as bytes init pdf.render(pdf.setFooter($laidOut, $footer));
def pdfPath as string init path.join(os.tempDir(), "doctree_demo.pdf");
fs.writeBytes($pdfPath, $pdfBytes);
io.printf("\n=== PDF ===\nwrote %d bytes to %s\n", len($pdfBytes), $pdfPath);
