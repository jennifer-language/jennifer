# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# dotenv_test.j - white-box tests for dotenv.j's parser. Run with:
#
#     jennifer test modules/dotenv_test.j
#
# The overlay splices dotenv.j in first, so these tests reach its private helpers
# (parseValue, unquoteDouble, unquoteSingle, stripInlineComment, unescape,
# interpolate, parseWithBase, closingDoubleIndex, validProfile) and the exported
# parse by bare identifier. The file + environment paths (read / load / the
# cascade loaders: readCascade / resolve / loadCascade / autoload) are verified in
# the Go suite (TestDotenv, TestDotenvCascade). dotenv.j already `use`s fs /
# strings / os / path / regex / maps, so the overlay only adds testing.
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
    testing.assertEqual($m["B"], "keep # hash"); # no inline-comment strip inside quotes
    testing.assertEqual($m["C"], "a\\nb"); # single quotes are literal: backslash-n, not newline
}

func testDoubleQuotesEscapes() {
    def m as map of string to string init parse("A=\"hello world\"\nB=\"line1\\nline2\"\nC=\"tab\\there\"");
    testing.assertEqual($m["A"], "hello world");
    testing.assertEqual($m["B"], "line1\nline2"); # \n expands to a newline
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
    testing.assertEqual(unescape("x"), "x"); # unknown escape -> literal
    testing.assertEqual(stripInlineComment("val # c"), "val");
    testing.assertEqual(stripInlineComment("val"), "val");
    testing.assertEqual(unquoteSingle("'abc'"), "abc");
    testing.assertEqual(unquoteDouble("\"a\\tb\""), "a\tb");
    testing.assertEqual(parseValue("  bare  "), "bare");
}

func testEnvNameValidation() { # OM-021
    testing.assertTrue(validEnvName("PATH_2"));
    testing.assertTrue(validEnvName("_x"));
    testing.assertFalse(validEnvName("2BAD"));
    testing.assertFalse(validEnvName("A=B"));
    testing.assertFalse(validEnvName(""));
}

# --- interpolation ----------------------------------------------------------

func testInterpolationBackward() {
    # A later value references an earlier key; the resolved value substitutes.
    def m as map of string to string init parse("HOST=example.com\nURL=http://$\{HOST\}/x");
    testing.assertEqual($m["URL"], "http://example.com/x");
}

func testInterpolationForwardIsEmpty() {
    # Backward-reference only: a reference to a not-yet-defined key yields "".
    def m as map of string to string init parse("URL=http://$\{HOST\}/x\nHOST=example.com");
    testing.assertEqual($m["URL"], "http:///x");
}

func testInterpolationInDoubleQuotes() {
    def m as map of string to string init parse("A=1\nB=\"v$\{A\}v\"");
    testing.assertEqual($m["B"], "v1v");
}

func testInterpolationNotInSingleQuotes() {
    def m as map of string to string init parse("A=1\nB='v$\{A\}v'");
    testing.assertEqual($m["B"], 'v${A}v'); # single quotes never interpolate
}

func testInterpolationInvalidRefIsLiteral() {
    # `${` with no valid env name is kept literal, not treated as an empty ref.
    def m as map of string to string init parse("A=$\{1BAD\}\nB=$\{no close");
    testing.assertEqual($m["A"], '${1BAD}');
    testing.assertEqual($m["B"], '${no close');
}

func testNoCommandSubstitution() {
    # `$(...)` and a lone `$` are plain characters - never executed or expanded.
    def m as map of string to string init parse("A=$(echo hi)\nB=cost is $5");
    testing.assertEqual($m["A"], "$(echo hi)");
    testing.assertEqual($m["B"], "cost is $5");
}

func testInterpolateHelper() {
    def acc as map of string to string init {"X": "9"};
    testing.assertEqual(interpolate('a${X}b', $acc), "a9b");
    testing.assertEqual(interpolate('none${MISSING}here', $acc), "nonehere"); # undefined -> ""
    testing.assertEqual(interpolate("plain", $acc), "plain");
}

# --- multi-line double-quoted values ----------------------------------------

func testMultilineDoubleQuoted() {
    def m as map of string to string init parse("A=\"line1\nline2\nline3\"\nB=after");
    testing.assertEqual($m["A"], "line1\nline2\nline3");
    testing.assertEqual($m["B"], "after"); # parsing resumes after the closing line
}

func testMultilineWithInterpolation() {
    def m as map of string to string init parse("H=host\nA=\"top\n$\{H\}\nbot\"");
    testing.assertEqual($m["A"], "top\nhost\nbot");
}

func testUnterminatedMultilineThrows() {
    testing.assertThrows("unterminatedBody", "dotenv");
}

func unterminatedBody() {
    parse("A=\"open\nstill open\n");
}

func testClosingDoubleIndex() {
    testing.assertEqual(closingDoubleIndex("\"abc\""), 4); # closing quote at index 4
    testing.assertEqual(closingDoubleIndex("\"no close"), -1); # not closed
    testing.assertEqual(closingDoubleIndex("\"a\\\"b\""), 5); # an escaped \" is not the close
}

# --- profile validation + cascade base ---------------------------------------

func testValidProfile() {
    testing.assertTrue(validProfile("production"));
    testing.assertTrue(validProfile("staging-2"));
    testing.assertTrue(validProfile("dev_1"));
    testing.assertFalse(validProfile(""));
    testing.assertFalse(validProfile("../../etc")); # traversal blocked
    testing.assertFalse(validProfile("a/b"));
    testing.assertFalse(validProfile("with space"));
}

func testParseWithBaseLaterWins() {
    def base as map of string to string init {"A": "1", "B": "2"};
    def m as map of string to string init parseWithBase("B=override\nC=3", $base);
    testing.assertEqual($m["A"], "1"); # inherited from base
    testing.assertEqual($m["B"], "override"); # this text overrides base
    testing.assertEqual($m["C"], "3"); # new key
}

func testParseWithBaseInterpolatesBase() {
    # A `${VAR}` resolves against an earlier cascade file's keys (the base map).
    def base as map of string to string init {"HOST": "h1"};
    def m as map of string to string init parseWithBase('URL=${HOST}/p', $base);
    testing.assertEqual($m["URL"], "h1/p");
}

# --- file-backed functions (read / load / cascade / autoload) via temp files ---
# dotenv.j `use`s fs / os / path / maps, so the overlay reaches them directly.

func testReadFromFile() {
    def p as string init fs.makeTempFile("", "env-", ".env");
    fs.writeString($p, "FOO=bar\nNUM=42\n# a comment\nEMPTY=\n");
    def m as map of string to string init read($p);
    fs.remove($p);
    testing.assertEqual($m["FOO"], "bar");
    testing.assertEqual($m["NUM"], "42");
    testing.assertEqual($m["EMPTY"], "");
}

func testLoadSetsProcessEnv() {
    def p as string init fs.makeTempFile("", "env-load-", ".env");
    fs.writeString($p, "DOTENV_OVERLAY_VAR=hello\n");
    load($p);
    fs.remove($p);
    testing.assertEqual(os.getEnv("DOTENV_OVERLAY_VAR"), "hello");
    os.setEnv("DOTENV_OVERLAY_VAR", "");
}

func testCascadeResolveAndLoad() {
    def dir as string init fs.makeTempDir("", "envdir-");
    fs.writeString(path.join($dir, ".env"), "A=base\nB=base\n");
    fs.writeString(path.join($dir, ".env.dev"), "B=dev\nC=dev\n");
    # resolve / readCascade: the profile file overlays the base.
    def m as map of string to string init resolve($dir, "dev");
    testing.assertEqual($m["A"], "base");
    testing.assertEqual($m["B"], "dev");
    testing.assertEqual($m["C"], "dev");
    def rc as map of string to string init readCascade($dir, "dev");
    testing.assertTrue(maps.has($rc, "A"));
    # loadCascade applies it to the environment.
    loadCascade($dir, "dev");
    testing.assertEqual(os.getEnv("C"), "dev");
    os.setEnv("C", "");
    fs.removeAll($dir);
}

func testAutoloadUsesEnvProfile() {
    def dir as string init fs.makeTempDir("", "envauto-");
    fs.writeString(path.join($dir, ".env"), "AUTO_A=1\n");
    os.setEnv("JENNIFER_ENV", "prod");
    fs.writeString(path.join($dir, ".env.prod"), "AUTO_B=2\n");
    autoload($dir);
    os.setEnv("JENNIFER_ENV", "");
    fs.removeAll($dir);
    testing.assertEqual(os.getEnv("AUTO_A"), "1");
    testing.assertEqual(os.getEnv("AUTO_B"), "2");
    os.setEnv("AUTO_A", "");
    os.setEnv("AUTO_B", "");
}

# An invalid cascade profile name is rejected.
func badProfile() {
    def dir as string init fs.makeTempDir("", "envbad-");
    return readCascade($dir, "has spaces!");
}
func testInvalidProfileThrows() {
    testing.assertThrows("badProfile", "dotenv");
}
