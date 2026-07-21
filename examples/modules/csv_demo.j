# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The csv module (modules/csv.j): parse RFC 4180 text into rows, read it as named records, and format rows back to CSV.
 * Run: jennifer run examples/modules/csv_demo.j
 * @module csv_demo
 */
use io;
import "../../modules/csv.j" as csv;

# A field with an embedded comma, a doubled quote, and a newline.
def text as string init "name,role,note\n";
$text = $text + "\"Smith, J\",dev,\"said \"\"hi\"\"\"\n";
$text = $text + "Ada,lead,\"two\nlines\"";

def rows as list of list of string init csv.parse($text);
io.printf("parsed %d rows\n", len($rows));

def recs as list of map of string to string init csv.toRecords($rows);
io.printf("records:\n");
for (def rec in $recs) {
    io.printf("  %s (%s): %s\n", $rec["name"], $rec["role"], $rec["note"]);
}

# Round-trip back to CSV text.
io.printf("re-encoded first data row: %s\n", csv.format([$rows[1]]));

# The delimiter is configurable: the same code reads TSV.
def tsv as list of list of string init csv.parseWith("a\tb\tc", "\t");
io.printf("tsv fields: %d\n", len($tsv[0]));
