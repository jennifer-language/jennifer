# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

/**
 * The rst module (modules/rst.j): read a reStructuredText document - sections,
 * a table, directives, references and a footnote - render it to HTML and styled
 * terminal text, report its problems, and walk and lay out its tree with the
 * doctree module.
 * @module rst_demo
 */
use io;
use fs;
use os;
use path;
import "../../modules/rst.j" as rst;
import "../../modules/doctree.j" as doctree;

def lines as list of string init [
    "==============",
    "Release notes",
    "==============",
    "",
    ":Version: 1.2",
    ":Author: The team",
    "",
    ".. contents::",
    "",
    "What changed",
    "============",
    "",
    "The parser is *faster* and the docs moved to the site_.",
    "Details are in the changelog [#]_.",
    "",
    ".. _site: https://example.org/docs",
    "",
    ".. note:: Upgrading needs no code changes.",
    "",
    "+-----------+---------+",
    "| Area      | Change  |",
    "+===========+=========+",
    "| parser    | 2x      |",
    "+-----------+---------+",
    "| renderer  | new PDF |",
    "+-----------+---------+",
    "",
    "Example",
    "=======",
    "",
    ".. code-block:: python",
    "",
    "   print(\"hello\")",
    "",
    ".. [#] See CHANGES.rst.",
    ""
];
def src as string init "";
for (def l in $lines) {
    $src = $src + $l + "\n";
}

io.printf("=== HTML ===\n%s\n\n", rst.toHtml($src));
io.printf("=== ANSI ===\n%s\n\n", rst.toAnsi($src));

# The tree: the promoted title, the bibliographic fields, every section.
def doc as doctree.Node init rst.parse($src);
io.printf("title: %s\n", rst.title($doc));
def info as map of string to string init rst.docinfo($doc);
io.printf("version: %s, author: %s\n", $info["version"], $info["author"]);
for (def h in doctree.findAll($doc, "heading")) {
    io.printf("  H%d: %s\n", doctree.level($h), doctree.text($h));
}

# Parsing is lenient: problems come back from check, and strict mode throws.
def broken as string init "See nowhere_.\n\n.. frobnicate:: x\n";
for (def d in rst.check($broken)) {
    io.printf("%s, line %d: %s\n", $d.severity, $d.line, $d.message);
}
try {
    def strict as rst.Options;
    $strict.strict = true;
    rst.parseWith($broken, $strict);
} catch (e) {
    io.printf("strict: %s\n", $e.message);
}

# The same tree as a PDF, through doctree's renderer.
def pdfBytes as bytes init doctree.renderPdf($doc, doctree.pdfDefaults());
def pdfPath as string init path.join(os.tempDir(), "rst_demo.pdf");
fs.writeBytes($pdfPath, $pdfBytes);
io.printf("\n=== PDF ===\nwrote %d bytes to %s\n", len($pdfBytes), $pdfPath);
