# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# dotenv_test.j - white-box tests for dotenv.j's parser. Run with:
#
#     jennifer test modules/dotenv_test.j
#
# The overlay splices dotenv.j in first, so these tests reach its private helpers
# (parseValue, unquoteDouble, unquoteSingle, stripInlineComment, unescape) and the
# exported parse by bare identifier. `read` / `load` (file + environment) are
# verified in the Go suite (TestDotenv). dotenv.j already `use`s fs / strings /
# os, so the overlay only adds testing.
use testing;

func testBasic() {
    def m as map of string to string init parse("A=1\nB=two");
    testing.assertEqual($m["A"], "1");
    testing.assertEqual($m["B"], "two");
    testing.assertEqual(len($m), 2);
}

func testCommentsAndBlanks() {
    def m as map of string to string init parse("# comment\n\nA=1\n   \n# again\nB=2");
    testing.assertEqual(len($m), 2);
    testing.assertEqual($m["A"], "1");
    testing.assertEqual($m["B"], "2");
}

func testExportPrefix() {
    def m as map of string to string init parse("export A=1\nexport   B=2");
    testing.assertEqual($m["A"], "1");
    testing.assertEqual($m["B"], "2");
}

func testInlineComment() {
    def m as map of string to string init parse("A=value # trailing\nB=plain");
    testing.assertEqual($m["A"], "value");
    testing.assertEqual($m["B"], "plain");
}

func testSingleQuotesLiteral() {
    def m as map of string to string init parse("A='hello world'\nB='keep # hash'\nC='a\\nb'");
    testing.assertEqual($m["A"], "hello world");
    testing.assertEqual($m["B"], "keep # hash");   # no inline-comment strip inside quotes
    testing.assertEqual($m["C"], "a\\nb");          # single quotes are literal: backslash-n, not newline
}

func testDoubleQuotesEscapes() {
    def m as map of string to string init parse("A=\"hello world\"\nB=\"line1\\nline2\"\nC=\"tab\\there\"");
    testing.assertEqual($m["A"], "hello world");
    testing.assertEqual($m["B"], "line1\nline2");   # \n expands to a newline
    testing.assertEqual($m["C"], "tab\there");
}

func testValueWithEquals() {
    def m as map of string to string init parse("URL=key=val&x=y");
    testing.assertEqual($m["URL"], "key=val&x=y");
}

func testEmptyValues() {
    def m as map of string to string init parse("EMPTY=\nQUOTED=\"\"");
    testing.assertEqual($m["EMPTY"], "");
    testing.assertEqual($m["QUOTED"], "");
}

func testNoEqualsSkipped() {
    def m as map of string to string init parse("JUSTAKEY\n=noKey\nA=1");
    testing.assertEqual(len($m), 1);
    testing.assertEqual($m["A"], "1");
}

func testDuplicateLaterWins() {
    def m as map of string to string init parse("A=1\nA=2");
    testing.assertEqual($m["A"], "2");
}

func testCrlf() {
    def m as map of string to string init parse("A=1\r\nB=2\r\n");
    testing.assertEqual($m["A"], "1");
    testing.assertEqual($m["B"], "2");
}

func testHelpers() {
    testing.assertEqual(unescape("n"), "\n");
    testing.assertEqual(unescape("t"), "\t");
    testing.assertEqual(unescape("x"), "x");                # unknown escape -> literal
    testing.assertEqual(stripInlineComment("val # c"), "val");
    testing.assertEqual(stripInlineComment("val"), "val");
    testing.assertEqual(unquoteSingle("'abc'"), "abc");
    testing.assertEqual(unquoteDouble("\"a\\tb\""), "a\tb");
    testing.assertEqual(parseValue("  bare  "), "bare");
}
