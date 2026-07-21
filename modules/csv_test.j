# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
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
    testing.assertEqual(len(parse("")), 0);            # empty input, no rows
    testing.assertEqual(len(parse("a,b\n")), 1);       # trailing newline, no extra row
    def e as list of list of string init parse("\"\",x,");
    testing.assertEqual(len($e[0]), 3);                # empty quoted, value, trailing empty
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
    testing.assertEqual($rows[0][0], "name");   # header row
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
    testing.assertFalse(needsQuote("a,b", "\t"));   # comma is not the tab delimiter
}

func testPrivateQuoteField() {
    testing.assertEqual(quoteField("plain", ","), "plain");
    testing.assertEqual(quoteField("a,b", ","), "\"a,b\"");
    testing.assertEqual(quoteField("a\"b", ","), "\"a\"\"b\"");
}
