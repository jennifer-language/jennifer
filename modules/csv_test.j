# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# csv_test.j - white-box tests for csv.j. Run with:
#
#     jennifer test modules/csv_test.j
#
# The overlay splices csv.j in front of this file, so the tests reach its
# private helpers (needsQuote, quoteField) by bare identifier as well as its
# exported surface.
use testing;

func testParseBasic() {
    def rows as list of list of string init parse("a,b,c\n1,2,3");
    testing.assertEqual(len($rows), 2);
    testing.assertEqual(len($rows[0]), 3);
    testing.assertEqual($rows[0][0], "a");
    testing.assertEqual($rows[0][2], "c");
    testing.assertEqual($rows[1][1], "2");
}

func testParseQuotedComma() {
    def rows as list of list of string init parse("\"Smith, J\",42");
    testing.assertEqual(len($rows[0]), 2);
    testing.assertEqual($rows[0][0], "Smith, J");
    testing.assertEqual($rows[0][1], "42");
}

func testParseDoubledQuote() {
    def rows as list of list of string init parse("\"he said \"\"hi\"\"\"");
    testing.assertEqual($rows[0][0], "he said \"hi\"");
}

func testParseEmbeddedNewline() {
    def rows as list of list of string init parse("a,\"line1\nline2\"\nb,c");
    testing.assertEqual(len($rows), 2);
    testing.assertEqual($rows[0][1], "line1\nline2");
    testing.assertEqual($rows[1][0], "b");
}

func testParseCRLF() {
    def rows as list of list of string init parse("a,b\r\nc,d\r\n");
    testing.assertEqual(len($rows), 2);
    testing.assertEqual($rows[0][1], "b");
    testing.assertEqual($rows[1][0], "c");
}

func testParseEdges() {
    testing.assertEqual(len(parse("")), 0); # empty input, no rows
    testing.assertEqual(len(parse("a,b\n")), 1); # trailing newline, no extra row
    def e as list of list of string init parse("\"\",x,");
    testing.assertEqual(len($e[0]), 3); # empty quoted, value, trailing empty
    testing.assertEqual($e[0][0], "");
    testing.assertEqual($e[0][1], "x");
    testing.assertEqual($e[0][2], "");
}

func testFormatQuoting() {
    def rows as list of list of string init [];
    $rows[] = ["plain", "has,comma", "has\"quote", "has\nnewline"];
    def out as string init format($rows);
    testing.assertEqual($out, "plain,\"has,comma\",\"has\"\"quote\",\"has\nnewline\"");
}

func testRoundTrip() {
    def src as string init "name,note\n\"Smith, J\",\"a \"\"quote\"\" and, comma\"\nAda,plain";
    def rows as list of list of string init parse($src);
    def back as list of list of string init parse(format($rows));
    testing.assertEqual(len($back), len($rows));
    testing.assertEqual($back[1][0], "Smith, J");
    testing.assertEqual($back[1][1], "a \"quote\" and, comma");
    testing.assertEqual($back[2][1], "plain");
}

func testTSVDelimiter() {
    def rows as list of list of string init parseWith("a\tb\tc", "\t");
    testing.assertEqual(len($rows[0]), 3);
    testing.assertEqual($rows[0][1], "b");
    # A tab inside a field forces quoting under a tab delimiter.
    def out as string init formatWith($rows, "\t");
    testing.assertEqual($out, "a\tb\tc");
}

func testToRecords() {
    def rows as list of list of string init parse("name,age\nAda,36\nGrace,45");
    def recs as list of map of string to string init toRecords($rows);
    testing.assertEqual(len($recs), 2);
    testing.assertEqual($recs[0]["name"], "Ada");
    testing.assertEqual($recs[0]["age"], "36");
    testing.assertEqual($recs[1]["name"], "Grace");
}

func testToRecordsShortRow() {
    # A row shorter than the header fills the missing field with "".
    def rows as list of list of string init parse("a,b,c\n1,2");
    def recs as list of map of string to string init toRecords($rows);
    testing.assertEqual($recs[0]["a"], "1");
    testing.assertEqual($recs[0]["b"], "2");
    testing.assertEqual($recs[0]["c"], "");
}

func testToRecordsEmpty() {
    testing.assertEqual(len(toRecords([])), 0);
}

func testFromRecords() {
    def recs as list of map of string to string init [];
    def one as map of string to string init {};
    $one["name"] = "Ada";
    $one["age"] = "36";
    $recs[] = $one;
    def rows as list of list of string init fromRecords(["name", "age"], $recs);
    testing.assertEqual(len($rows), 2);
    testing.assertEqual($rows[0][0], "name"); # header row
    testing.assertEqual($rows[1][0], "Ada");
    testing.assertEqual($rows[1][1], "36");
}

func testFromRecordsMissingKey() {
    # A record missing a header column writes "" in that position.
    def recs as list of map of string to string init [];
    def one as map of string to string init {};
    $one["name"] = "Ada";
    $recs[] = $one;
    def rows as list of list of string init fromRecords(["name", "age"], $recs);
    testing.assertEqual($rows[1][0], "Ada");
    testing.assertEqual($rows[1][1], "");
}

func testRecordsRoundTrip() {
    def rows as list of list of string init parse("name,age\nAda,36\nGrace,45");
    def recs as list of map of string to string init toRecords($rows);
    def back as list of list of string init fromRecords(["name", "age"], $recs);
    testing.assertEqual(format($back), format($rows));
}

# White-box: private quoting helpers reached by bare identifier.
func testPrivateNeedsQuote() {
    testing.assertFalse(needsQuote("plain", ","));
    testing.assertTrue(needsQuote("a,b", ","));
    testing.assertTrue(needsQuote("a\"b", ","));
    testing.assertTrue(needsQuote("a\nb", ","));
    testing.assertTrue(needsQuote("a\rb", ","));
    testing.assertFalse(needsQuote("a,b", "\t")); # comma is not the tab delimiter
}

func testPrivateQuoteField() {
    testing.assertEqual(quoteField("plain", ","), "plain");
    testing.assertEqual(quoteField("a,b", ","), "\"a,b\"");
    testing.assertEqual(quoteField("a\"b", ","), "\"a\"\"b\"");
}

# --- formula-injection mitigation (CWE-1236) ------------------------

# White-box: the private sanitizeField neutraliser.
func testPrivateSanitizeField() {
    testing.assertEqual(sanitizeField("=SUM(A1)"), "'=SUM(A1)");
    testing.assertEqual(sanitizeField("+1"), "'+1");
    testing.assertEqual(sanitizeField("-1"), "'-1");
    testing.assertEqual(sanitizeField("@ref"), "'@ref");
    testing.assertEqual(sanitizeField("\tx"), "'\tx"); # leading tab
    testing.assertEqual(sanitizeField("\rx"), "'\rx"); # leading CR
    testing.assertEqual(sanitizeField("plain"), "plain"); # normal untouched
    testing.assertEqual(sanitizeField(""), ""); # empty untouched
    testing.assertEqual(sanitizeField("a=b"), "a=b"); # only the FIRST char matters
}

func testFormatSafeNeutralises() {
    def rows as list of list of string init [];
    $rows[] = ["=1+2", "+cmd", "-2", "@ref", "\tinj", "normal"];
    def back as list of list of string init parse(formatSafe($rows));
    testing.assertEqual($back[0][0], "'=1+2");
    testing.assertEqual($back[0][1], "'+cmd");
    testing.assertEqual($back[0][2], "'-2");
    testing.assertEqual($back[0][3], "'@ref");
    testing.assertEqual($back[0][4], "'\tinj");
    testing.assertEqual($back[0][5], "normal"); # left alone
}

func testFormatSafeWithDelimiter() {
    def rows as list of list of string init [];
    $rows[] = ["=danger", "ok"];
    def out as string init formatSafeWith($rows, "\t");
    testing.assertEqual($out, "'=danger\tok");
}

# --- dialects -------------------------------------------------------

func testDialectDefaults() {
    def d as Dialect init dialect(";");
    testing.assertEqual($d.delimiter, ";");
    testing.assertEqual($d.quote, "\"");
    testing.assertEqual($d.comment, "");
    testing.assertFalse($d.trim);
}

func testParseDialectSemicolonCommentTrim() {
    def d as Dialect init Dialect{delimiter: ";", quote: "\"", comment: "#", trim: true};
    def text as string init "# a header comment\n a ; b ; c \nx;y;z\n# trailing comment";
    def rows as list of list of string init parseDialect($text, $d);
    testing.assertEqual(len($rows), 2);
    testing.assertEqual($rows[0][0], "a"); # unquoted field trimmed
    testing.assertEqual($rows[0][1], "b");
    testing.assertEqual($rows[0][2], "c");
    testing.assertEqual($rows[1][0], "x");
    testing.assertEqual($rows[1][2], "z");
}

func testParseDialectQuotedKeepsWhitespace() {
    # A quoted field keeps its whitespace even under trim=true.
    def d as Dialect init Dialect{delimiter: ";", quote: "\"", comment: "", trim: true};
    def rows as list of list of string init parseDialect("\" keep \"; drop ", $d);
    testing.assertEqual($rows[0][0], " keep ");
    testing.assertEqual($rows[0][1], "drop");
}

func testFormatDialect() {
    def rows as list of list of string init [];
    $rows[] = ["a;b", "plain"];
    def d as Dialect init dialect(";");
    # The field carrying the delimiter is quoted with the dialect quote char.
    testing.assertEqual(formatDialect($rows, $d), "\"a;b\";plain");
}

# --- streaming reader / writer round-trip ---------------------------

func testStreamCsvRoundTrip() {
    def path as string init fs.makeTempFile("", "csv-", ".csv");
    def wf as fs.File init fs.open($path, "write");
    def w as Writer init writer($wf);
    writeRow($w, ["name", "note"]);
    writeRow($w, ["Smith, J", "hi"]); # embedded comma forces quoting
    writeRow($w, ["Ada", "plain"]);
    closeWriter($w);

    def rf as fs.File init fs.open($path, "read");
    def r as Reader init reader($rf);
    def rows as list of list of string init [];
    while (not readerEof($r)) {
        def row as list of string init readRow($r);
        if (len($row) > 0) {
            $rows[] = $row;
        }
    }
    closeReader($r);
    fs.remove($path);

    testing.assertEqual(len($rows), 3);
    testing.assertEqual($rows[0][0], "name");
    testing.assertEqual($rows[1][0], "Smith, J"); # quoted comma survived streaming
    testing.assertEqual($rows[1][1], "hi");
    testing.assertEqual($rows[2][1], "plain");
}

func testStreamCsvQuotedNewlineSpansLines() {
    # A quoted field with an embedded newline spans two physical lines; readRow
    # must read the whole record.
    def path as string init fs.makeTempFile("", "csv-", ".csv");
    fs.writeString($path, "a,\"x\ny\"\nb,c\n");

    def rf as fs.File init fs.open($path, "read");
    def r as Reader init reader($rf);
    def first as list of string init readRow($r);
    def second as list of string init readRow($r);
    closeReader($r);
    fs.remove($path);

    testing.assertEqual(len($first), 2);
    testing.assertEqual($first[1], "x\ny");
    testing.assertEqual($second[0], "b");
    testing.assertEqual($second[1], "c");
}
