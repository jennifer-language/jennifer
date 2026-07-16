# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# semver_test.j - white-box tests for semver.j. Run with:
#
#     jennifer test modules/semver_test.j
#
# The overlay splices semver.j in first, so these tests reach its private
# helpers (isNum, sign, compareStr, compareIdent) by bare identifier as well
# as its exported surface.
use testing;
use meta;

# cmp compares two version strings, for terse ordering assertions.
func cmp(a as string, b as string) {
    return compare(parse($a), parse($b));
}

func testParseFields() {
    def v as Version init parse("1.2.3-rc.1+build.5");
    testing.assertEqual($v.major, 1);
    testing.assertEqual($v.minor, 2);
    testing.assertEqual($v.patch, 3);
    testing.assertEqual($v.prerelease, "rc.1");
    testing.assertEqual($v.build, "build.5");
}

func testRoundTrip() {
    testing.assertEqual(toString(parse("1.2.3")), "1.2.3");
    testing.assertEqual(toString(parse("1.0.0-alpha.1")), "1.0.0-alpha.1");
    testing.assertEqual(toString(parse("2.0.0+build.7")), "2.0.0+build.7");
    testing.assertEqual(toString(parse("1.2.3-rc.1+b.2")), "1.2.3-rc.1+b.2");
}

func testValidRejects() {
    testing.assertFalse(isValid("1.2.3.4"));   # four segments
    testing.assertFalse(isValid("1.2"));        # too few
    testing.assertFalse(isValid("01.0.0"));     # leading zero in core
    testing.assertFalse(isValid("1.0.0-"));     # empty prerelease
    testing.assertFalse(isValid("1.0.0+"));     # empty build
    testing.assertFalse(isValid("1.0.0-01"));   # leading zero in numeric prerelease
    testing.assertFalse(isValid("1.0.0-a..b")); # empty prerelease field
    testing.assertFalse(isValid(""));
    testing.assertTrue(isValid("0.0.0"));
    testing.assertTrue(isValid("1.0.0-alpha-beta"));  # hyphen inside an identifier
}

func testParseThrowsOnInvalid() {
    testing.assertThrows("parseBad", "value");
}
func parseBad() {
    return parse("not-a-version");
}

func testCoreOrdering() {
    testing.assertEqual(cmp("1.0.0-alpha", "1.0.0"), -1);
    testing.assertEqual(cmp("1.0.0", "1.0.1"), -1);
    testing.assertEqual(cmp("1.0.1", "1.1.0"), -1);
    testing.assertEqual(cmp("1.1.0", "2.0.0"), -1);
    testing.assertEqual(cmp("2.0.0", "1.1.0"), 1);
    testing.assertEqual(cmp("1.0.0", "1.0.0"), 0);
}

# The precedence example from semver.org, item 11.
func testPrereleasePrecedence() {
    testing.assertEqual(cmp("1.0.0-alpha", "1.0.0-alpha.1"), -1);
    testing.assertEqual(cmp("1.0.0-alpha.1", "1.0.0-alpha.beta"), -1);
    testing.assertEqual(cmp("1.0.0-alpha.beta", "1.0.0-beta"), -1);
    testing.assertEqual(cmp("1.0.0-beta", "1.0.0-beta.2"), -1);
    testing.assertEqual(cmp("1.0.0-beta.2", "1.0.0-beta.11"), -1);   # numeric, not lexical
    testing.assertEqual(cmp("1.0.0-beta.11", "1.0.0-rc.1"), -1);
    testing.assertEqual(cmp("1.0.0-rc.1", "1.0.0"), -1);
}

func testBuildIgnored() {
    testing.assertTrue(eq(parse("1.0.0+a"), parse("1.0.0+b")));
    testing.assertEqual(cmp("1.0.0+build.1", "1.0.0+build.2"), 0);
}

func testClassify() {
    testing.assertTrue(isStable(parse("1.0.0")));
    testing.assertFalse(isStable(parse("0.5.0")));       # 0.y.z is unstable
    testing.assertFalse(isStable(parse("1.0.0-rc")));
    testing.assertTrue(isPrerelease(parse("1.0.0-rc")));
    testing.assertFalse(isPrerelease(parse("1.0.0")));
}

func testIncrement() {
    testing.assertEqual(toString(incMajor(parse("1.2.3-rc+b"))), "2.0.0");
    testing.assertEqual(toString(incMinor(parse("1.2.3"))), "1.3.0");
    testing.assertEqual(toString(incPatch(parse("1.2.3"))), "1.2.4");
}

func testSort() {
    def vs as list of Version init [];
    $vs[] = parse("2.0.0");
    $vs[] = parse("1.0.0-alpha");
    $vs[] = parse("1.0.1");
    $vs[] = parse("1.0.0");
    $vs[] = parse("1.1.0");
    def out as list of Version init sort($vs);
    testing.assertEqual(toString($out[0]), "1.0.0-alpha");
    testing.assertEqual(toString($out[1]), "1.0.0");
    testing.assertEqual(toString($out[2]), "1.0.1");
    testing.assertEqual(toString($out[3]), "1.1.0");
    testing.assertEqual(toString($out[4]), "2.0.0");
}

# White-box: private helpers reached by bare identifier.
func testPrivateHelpers() {
    testing.assertTrue(isNum("42"));
    testing.assertFalse(isNum("4a"));
    testing.assertEqual(sign(-5), -1);
    testing.assertEqual(sign(0), 0);
    testing.assertEqual(sign(9), 1);
    testing.assertEqual(compareStr("alpha", "beta"), -1);
    testing.assertEqual(compareIdent("1", "beta"), -1);   # numeric ranks below alphanumeric
    testing.assertEqual(compareIdent("2", "11"), -1);     # numeric compared numerically
}

func testParsesInterpreterVersion() {
    # A real build stamps meta.VERSION with a semver ("0.17.0-dev+<n>.<sha>");
    # a plain `go test` build (no version codegen) uses the placeholder "dev",
    # which is correctly not semver. Assert validity only for a real version.
    if (not (meta.VERSION == "dev")) {
        testing.assertTrue(isValid(meta.VERSION));
    }
}

# --- comparison helpers + diff + rsort ------------------------------

func testCompareHelpers() {
    testing.assertTrue(gte(parse("2.0.0"), parse("2.0.0")));
    testing.assertTrue(gte(parse("2.0.1"), parse("2.0.0")));
    testing.assertFalse(gte(parse("1.9.9"), parse("2.0.0")));
    testing.assertTrue(lte(parse("2.0.0"), parse("2.0.0")));
    testing.assertTrue(lte(parse("1.0.0"), parse("2.0.0")));
    testing.assertTrue(neq(parse("1.0.0"), parse("1.0.1")));
    testing.assertFalse(neq(parse("1.0.0"), parse("1.0.0+build")));   # build ignored
}

func testDiff() {
    testing.assertEqual(diff(parse("1.2.3"), parse("2.0.0")), "major");
    testing.assertEqual(diff(parse("1.2.3"), parse("1.3.0")), "minor");
    testing.assertEqual(diff(parse("1.2.3"), parse("1.2.4")), "patch");
    testing.assertEqual(diff(parse("1.2.3"), parse("1.2.3-rc.1")), "prerelease");
    testing.assertEqual(diff(parse("1.2.3"), parse("1.2.3")), "");
    testing.assertEqual(diff(parse("1.2.3"), parse("1.2.3+build")), "");   # build ignored
}

func testRsort() {
    def vs as list of Version init [];
    $vs[] = parse("1.0.0");
    $vs[] = parse("2.0.0");
    $vs[] = parse("1.5.0");
    def out as list of Version init rsort($vs);
    testing.assertEqual(toString($out[0]), "2.0.0");
    testing.assertEqual(toString($out[1]), "1.5.0");
    testing.assertEqual(toString($out[2]), "1.0.0");
}

# --- ranges: single constraints (from the old constraint module) ----

func testWildcardMatchesAnyRelease() {
    testing.assertTrue(satisfies("1.0.0", "*"));
    testing.assertTrue(satisfies("0.0.1", ""));
    testing.assertTrue(satisfies("9.9.9", "any"));
    testing.assertFalse(satisfies("not-a-version", "*"));
    testing.assertFalse(satisfies("1.0.0-rc.1", "*"));   # prerelease excluded
}

func testExactAndCaret() {
    testing.assertTrue(satisfies("1.2.3", "1.2.3"));
    testing.assertTrue(satisfies("1.2.3", "=1.2.3"));
    testing.assertFalse(satisfies("1.2.4", "1.2.3"));
    testing.assertTrue(satisfies("1.4.9", "^1.2.0"));
    testing.assertFalse(satisfies("2.0.0", "^1.2.0"));
    testing.assertTrue(satisfies("0.2.9", "^0.2.3"));
    testing.assertFalse(satisfies("0.3.0", "^0.2.3"));
    testing.assertFalse(satisfies("0.0.4", "^0.0.3"));
    testing.assertTrue(satisfies("0.0.9", "^0.0"));
    testing.assertFalse(satisfies("0.1.0", "^0.0"));
}

func testTildeAndComparators() {
    testing.assertTrue(satisfies("1.2.9", "~1.2.3"));
    testing.assertFalse(satisfies("1.3.0", "~1.2.3"));
    testing.assertTrue(satisfies("1.5.0", "~1"));
    testing.assertFalse(satisfies("2.0.0", "~1"));
    testing.assertTrue(satisfies("2.0.0", ">=1.2.3"));
    testing.assertFalse(satisfies("1.2.2", ">=1.2.3"));
    testing.assertTrue(satisfies("1.2.4", ">1.2.3"));
    testing.assertFalse(satisfies("1.2.3", ">1.2.3"));
    testing.assertFalse(satisfies("1.2.4", "<=1.2.3"));
    testing.assertTrue(satisfies("1.2.2", "<1.2.3"));
}

func testPrereleaseGate() {
    testing.assertFalse(satisfies("2.0.0-rc.1", "^1.2.0"));
    testing.assertFalse(satisfies("1.3.0-beta", "~1.2.0"));
    testing.assertTrue(satisfies("1.2.3-rc.1", "=1.2.3-rc.1"));
    # a prerelease matches a range that pins a prerelease at the same tuple
    testing.assertTrue(satisfies("1.2.3-rc.2", ">=1.2.3-rc.1 <1.3.0"));
    # but not one pinned at a different tuple
    testing.assertFalse(satisfies("1.4.0-rc.1", ">=1.2.3-rc.1 <2.0.0"));
}

func testInvalidVersionNeverMatches() {
    testing.assertFalse(satisfies("1.2", "^1.0.0"));
    testing.assertFalse(satisfies("", "*"));
}

# A caret / tilde range whose operand carries a prerelease keeps that
# prerelease as the inclusive lower bound (npm semantics), rather than
# rounding up to the bare release and excluding it.
func testCaretTildePrereleaseLowerBound() {
    testing.assertTrue(satisfies("1.2.3-rc.2", "^1.2.3-rc.1"));
    testing.assertTrue(satisfies("1.2.3-rc.1", "^1.2.3-rc.1"));
    testing.assertFalse(satisfies("1.2.3-rc.0", "^1.2.3-rc.1"));   # below the pinned prerelease
    testing.assertTrue(satisfies("1.2.4", "^1.2.3-rc.1"));         # a later release still matches
    testing.assertTrue(satisfies("1.2.3-rc.2", "~1.2.3-rc.1"));
    testing.assertFalse(satisfies("1.3.0-rc.1", "~1.2.3-rc.1"));   # outside the tilde patch window
}

func testMinVersionCaretPrerelease() {
    testing.assertEqual(minVersion("^1.2.3-rc.1"), "1.2.3-rc.1");
    testing.assertEqual(minVersion("~1.2.3-rc.1"), "1.2.3-rc.1");
}

# --- ranges: compound (new) -----------------------------------------

func testCompoundAnd() {
    testing.assertTrue(satisfies("1.5.0", ">=1.2.0 <2.0.0"));
    testing.assertFalse(satisfies("2.0.0", ">=1.2.0 <2.0.0"));
    testing.assertFalse(satisfies("1.1.0", ">=1.2.0 <2.0.0"));
    testing.assertTrue(satisfies("1.9.0", ">=1.2.0,<2.0.0"));   # comma also = AND
}

func testCompoundOr() {
    testing.assertTrue(satisfies("1.4.0", "^1.0.0 || ^3.0.0"));
    testing.assertTrue(satisfies("3.4.0", "^1.0.0 || ^3.0.0"));
    testing.assertFalse(satisfies("2.0.0", "^1.0.0 || ^3.0.0"));
}

func testHyphenRanges() {
    testing.assertTrue(satisfies("2.0.0", "1.2.3 - 2.3.4"));
    testing.assertTrue(satisfies("2.3.4", "1.2.3 - 2.3.4"));   # inclusive upper
    testing.assertFalse(satisfies("2.3.5", "1.2.3 - 2.3.4"));
    testing.assertFalse(satisfies("1.2.2", "1.2.3 - 2.3.4"));
    testing.assertTrue(satisfies("2.3.9", "1.2 - 2.3"));       # partial upper -> <2.4.0
    testing.assertFalse(satisfies("2.4.0", "1.2 - 2.3"));
}

func testXRangesAndBarePartials() {
    testing.assertTrue(satisfies("1.9.9", "1.x"));
    testing.assertFalse(satisfies("2.0.0", "1.x"));
    testing.assertTrue(satisfies("1.2.7", "1.2.*"));
    testing.assertFalse(satisfies("1.3.0", "1.2.X"));
    testing.assertTrue(satisfies("1.2.7", "1.2"));            # bare 2-part = 1.2.x
    testing.assertFalse(satisfies("1.3.0", "1.2"));
    testing.assertTrue(satisfies("1.9.0", "1"));              # bare 1-part = 1.x
}

# --- ranges: selection + validation (new) ---------------------------

func testMaxSatisfying() {
    def vers as list of string init ["1.0.0", "1.2.0", "1.4.3", "2.0.0"];
    testing.assertEqual(maxSatisfying($vers, "^1.2.0"), "1.4.3");
    testing.assertEqual(maxSatisfying($vers, ">=1.0.0"), "2.0.0");
    testing.assertEqual(maxSatisfying($vers, "~1.0.0"), "1.0.0");
    testing.assertEqual(maxSatisfying($vers, "^9.0.0"), "");
    def messy as list of string init ["bad", "1.2.0", "also-bad", "1.3.0"];
    testing.assertEqual(maxSatisfying($messy, "^1.0.0"), "1.3.0");   # invalids skipped
}

func testMinSatisfying() {
    def vers as list of string init ["1.0.0", "1.2.0", "1.4.3", "2.0.0"];
    testing.assertEqual(minSatisfying($vers, "^1.2.0"), "1.2.0");
    testing.assertEqual(minSatisfying($vers, ">=1.0.0"), "1.0.0");
    testing.assertEqual(minSatisfying($vers, "^9.0.0"), "");
}

# Whitespace after a comparator operator must not turn ">= 1.2.3" into an
# exact match (the operator and operand rejoin).
func testWhitespaceAfterOperator() {
    testing.assertTrue(satisfies("1.5.0", ">= 1.2.3"));
    testing.assertFalse(satisfies("1.1.0", ">= 1.2.3"));
    testing.assertTrue(satisfies("1.2.0", "<= 2.0.0"));
    testing.assertTrue(satisfies("1.9.9", "^ 1.2.0"));
    testing.assertTrue(satisfies("1.5.0", ">= 1.2.0 < 2.0.0"));
    testing.assertFalse(satisfies("2.1.0", ">= 1.2.0 < 2.0.0"));
}

func testValidRange() {
    testing.assertTrue(validRange("*"));
    testing.assertTrue(validRange(""));
    testing.assertTrue(validRange("^1.2.0"));
    testing.assertTrue(validRange(">=1.0.0 <2.0.0"));
    testing.assertTrue(validRange("^1.0.0 || ~2.3.0"));
    testing.assertTrue(validRange("1.2.3 - 2.3.4"));
    testing.assertTrue(validRange("1.x"));
    testing.assertFalse(validRange("garbage!!"));
    testing.assertFalse(validRange(">=notaversion"));
}

# --- ranges: private machinery --------------------------------------

func testRangeHelpers() {
    testing.assertTrue(isWild("*"));
    testing.assertTrue(isWild("x"));
    testing.assertFalse(isWild("1.0.0"));
    def c as Core init parseCore("1.2.x");
    testing.assertEqual($c.major, 1);
    testing.assertEqual($c.minor, 2);
    testing.assertEqual($c.count, 2);
    testing.assertEqual(toString(caretUpper(parseCore("1.2.3"))), "2.0.0");
    testing.assertEqual(toString(tildeUpper(parseCore("1.2"))), "1.3.0");
    testing.assertEqual(len(tokenize(">=1.2.0  <2.0.0")), 2);
    testing.assertEqual(operandOf(">=1.2.0"), "1.2.0");
}

# --- lenient input: coerce / clean ----------------------------------

func testCoerce() {
    testing.assertEqual(coerce("v1.2.3"), "1.2.3");
    testing.assertEqual(coerce("1.2"), "1.2.0");
    testing.assertEqual(coerce("2"), "2.0.0");
    testing.assertEqual(coerce("release-2.3"), "2.3.0");
    testing.assertEqual(coerce("v01.02.03"), "1.2.3");   # leading zeros normalised
    testing.assertEqual(coerce("latest"), "");
}

func testClean() {
    testing.assertEqual(clean("v1.2.3"), "1.2.3");
    testing.assertEqual(clean("  =1.2.3  "), "1.2.3");
    testing.assertEqual(clean("1.2.3+build"), "1.2.3+build");
    testing.assertEqual(clean("1.2"), "");               # strict: not a full version
    testing.assertEqual(clean("garbage"), "");
}

# --- range algebra: minVersion / intersects / subset / gtr ----------

func testMinVersion() {
    testing.assertEqual(minVersion("^1.2.0"), "1.2.0");
    testing.assertEqual(minVersion(">=1.2.3"), "1.2.3");
    testing.assertEqual(minVersion(">1.2.3"), "1.2.4");
    testing.assertEqual(minVersion("<2.0.0"), "0.0.0");
    testing.assertEqual(minVersion("^2.0.0 || ^0.5.0"), "0.5.0");
    testing.assertEqual(minVersion("garbage"), "");
}

func testIntersects() {
    testing.assertTrue(intersects("^1.2.0", ">=1.5.0"));
    testing.assertFalse(intersects("^1.2.0", "^2.0.0"));
    testing.assertTrue(intersects(">=1.5.0 <1.8.0", "^1.2.0"));
    testing.assertFalse(intersects("^1.0.0", "^2.0.0 || ^3.0.0"));
    testing.assertTrue(intersects("^1.0.0 || ^3.0.0", "^3.2.0"));
    testing.assertFalse(intersects("^1.0.0", "garbage"));
}

func testSubset() {
    testing.assertTrue(subset("^1.5.0", "^1.0.0"));
    testing.assertFalse(subset("^1.0.0", "^1.5.0"));
    testing.assertTrue(subset("~1.2.3", "^1.0.0"));
    testing.assertTrue(subset(">=1.2.0 <1.5.0", "^1.0.0"));
    testing.assertTrue(subset("^1.0.0 || ^2.0.0", ">=1.0.0 <3.0.0"));
    testing.assertFalse(subset("^1.0.0 || ^3.0.0", "^1.0.0"));   # the ^3 clause is not covered
}

func testGtrLtrOutside() {
    testing.assertTrue(gtr("2.0.0", "^1.2.0"));
    testing.assertFalse(gtr("1.5.0", "^1.2.0"));
    testing.assertFalse(gtr("9.9.9", ">=1.0.0"));               # unbounded above
    testing.assertTrue(ltr("0.9.0", "^1.2.0"));
    testing.assertFalse(ltr("1.5.0", "^1.2.0"));
    testing.assertTrue(outside("2.0.0", "^1.2.0"));
    testing.assertTrue(outside("0.1.0", "^1.2.0"));
    testing.assertFalse(outside("2.5.0", "^1.0.0 || ^3.0.0"));  # interior gap is not outside
}

# --- range algebra: prerelease-precise ------------------------------

func testMinVersionPrerelease() {
    testing.assertEqual(minVersion(">=1.2.3-rc.1"), "1.2.3-rc.1");
    testing.assertEqual(minVersion(">=1.2.3-rc.1 <2.0.0"), "1.2.3-rc.1");
    testing.assertEqual(minVersion(">1.2.3-rc.1"), "1.2.3");   # excluded prerelease -> its release
}

func testIntersectsPrerelease() {
    testing.assertTrue(intersects(">=1.2.3-rc.1 <1.2.3", ">=1.2.3-rc.2 <1.2.3"));   # same tuple
    testing.assertFalse(intersects(">=1.2.3-rc.1 <1.2.3", ">=1.5.0-rc.1 <1.5.0"));  # different tuple
    testing.assertFalse(intersects(">=1.2.3-rc.1 <1.2.3", "^1.0.0"));               # pre-only vs release
    testing.assertTrue(intersects(">=1.2.3-rc.1 <2.0.0", "^1.5.0"));                # shares releases
}

func testSubsetPrerelease() {
    testing.assertTrue(subset(">=1.2.3-rc.1 <1.3.0", ">=1.2.3-rc.1 <1.3.0"));
    testing.assertFalse(subset(">=1.2.3-rc.1 <2.0.0", "^1.2.3"));                   # caret admits no prereleases
    testing.assertTrue(subset(">=1.2.3-rc.5 <1.5.0", ">=1.2.3-rc.1 <2.0.0"));       # outer pins the tuple
}

func testLtrPrerelease() {
    testing.assertTrue(ltr("1.2.3-rc.0", ">=1.2.3-rc.1 <2.0.0"));
    testing.assertFalse(ltr("1.2.3-rc.2", ">=1.2.3-rc.1 <2.0.0"));
}

# --- simplifyRange --------------------------------------------------

func testSimplifyRange() {
    def vers as list of string init ["1.0.0", "1.1.0", "1.2.0", "1.3.0", "2.0.0", "2.1.0"];
    testing.assertEqual(simplifyRange($vers, ">=1.0.0 <=1.0.0 || >=1.1.0 <=1.3.0"), ">=1.0.0 <=1.3.0");
    testing.assertEqual(simplifyRange($vers, ">=1.0.0"), "*");
    testing.assertEqual(simplifyRange($vers, "^9.0.0"), "<0.0.0-0");
    testing.assertEqual(simplifyRange($vers, "^1.0.0"), "^1.0.0");   # original kept when shorter
    testing.assertEqual(simplifyRange($vers, "1.0.0 || 2.1.0"), "1.0.0 || 2.1.0");
}

# simplifyRange must not drop prerelease versions the original matched (the "*"
# shortcut excludes prereleases): the produced range still matches them.
func testSimplifyRangePreservesPrereleases() {
    def vers as list of string init ["1.2.3-rc.1"];
    def out as string init simplifyRange($vers, ">=1.2.3-rc.1 <1.3.0");
    testing.assertFalse($out == "*");                 # "*" would match nothing here
    testing.assertTrue(satisfies("1.2.3-rc.1", $out)); # the produced range keeps it
}
