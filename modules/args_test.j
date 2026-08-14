# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# White-box tests for modules/args.j. Run: jennifer test modules/args_test.j
#
# The overlay splices args.j in front of this file, so the tests reach both its
# exported surface (parser / flag / parse / asString / ...) and its private
# helpers (findLong, validateValue, isIntStr, isShortFlag, ...) by bare
# identifier.

use testing;

# A parser exercising most argument kinds, reused across tests.
func demo() {
    def p as Parser init parser("demo", "a demo tool");
    $p = flag($p, "name", "n", "world", "who");
    $p = intFlag($p, "count", "c", 1, "how many");
    $p = floatFlag($p, "scale", "s", 1.5, "scale factor");
    $p = boolFlag($p, "loud", "l", "shout");
    $p = countFlag($p, "verbose", "v", "verbosity");
    $p = listFlag($p, "define", "D", "defines");
    $p = flag($p, "mode", "m", "debug", "build mode");
    $p = choices($p, ["debug", "release"]);
    return $p;
}

# --- flags: values, shorts, bundling, count, append ---

func testLongAndShortValues() {
    def r as Result init parse(demo(), ["prog", "--name", "Ada", "-c", "3"]);
    testing.assertEqual(asString($r, "name"), "Ada");
    testing.assertEqual(asInt($r, "count"), 3);
    testing.assertTrue(has($r, "name"));
}

func testInlineEquals() {
    def r as Result init parse(demo(), ["prog", "--name=Bo", "--count=7"]);
    testing.assertEqual(asString($r, "name"), "Bo");
    testing.assertEqual(asInt($r, "count"), 7);
}

func testShortBundlingCountAndBool() {
    def r as Result init parse(demo(), ["prog", "-lvv"]);
    testing.assertTrue(asBool($r, "loud"));
    testing.assertEqual(count($r, "verbose"), 2);
}

func testShortValueGlued() {
    def r as Result init parse(demo(), ["prog", "-nAda"]); # -n glued value
    testing.assertEqual(asString($r, "name"), "Ada");
}

func testLongCountAndBool() {
    def r as Result init parse(demo(), ["prog", "--verbose", "--verbose", "--loud"]);
    testing.assertEqual(count($r, "verbose"), 2);
    testing.assertTrue(asBool($r, "loud"));
}

func testUsageShowsRequired() {
    def p as Parser init required(flag(parser("t", ""), "key", "k", "", "the key"));
    testing.assertContains(usage($p), "(required)");
}

func testFloatFlag() {
    def r as Result init parse(demo(), ["prog", "-s", "2.5"]);
    testing.assertEqual(asFloat($r, "scale"), 2.5);
}

func testAppendList() {
    def r as Result init parse(demo(), ["prog", "-D", "A=1", "--define=B=2"]);
    def d as list of string init asList($r, "define");
    testing.assertEqual(len($d), 2);
    testing.assertEqual($d[0], "A=1");
    testing.assertEqual($d[1], "B=2");
}

func testDefaultsApplied() {
    def r as Result init parse(demo(), ["prog"]);
    testing.assertEqual(asString($r, "name"), "world");
    testing.assertEqual(asInt($r, "count"), 1);
    testing.assertEqual(asFloat($r, "scale"), 1.5);
    testing.assertFalse(asBool($r, "loud"));
    testing.assertEqual(count($r, "verbose"), 0);
    testing.assertEqual(len(asList($r, "define")), 0);
    testing.assertFalse(has($r, "name"));
}

func testEndOfFlagsMarker() {
    def p as Parser init positional(parser("t", ""), "x", "");
    def r as Result init parse($p, ["prog", "--", "--looks-like-flag"]);
    testing.assertEqual(asString($r, "x"), "--looks-like-flag");
}

func testNegativeNumberPositional() {
    def p as Parser init positional(parser("t", ""), "x", "");
    def r as Result init parse($p, ["prog", "-5"]);
    testing.assertEqual(asString($r, "x"), "-5");
}

# --- positionals + nargs ---

func testSinglePositional() {
    def p as Parser init positional(parser("t", ""), "target", "");
    def r as Result init parse($p, ["prog", "app"]);
    testing.assertEqual(asString($r, "target"), "app");
}

func testOptionalPositional() {
    def p as Parser init positionalOpt(parser("t", ""), "target", "auto", "");
    testing.assertEqual(asString(parse($p, ["prog"]), "target"), "auto");
    testing.assertEqual(asString(parse($p, ["prog", "x"]), "target"), "x");
}

func testVariadicStar() {
    def p as Parser init positionalList(parser("t", ""), "files", "");
    testing.assertEqual(len(asList(parse($p, ["prog"]), "files")), 0);
    testing.assertEqual(len(asList(parse($p, ["prog", "a", "b", "c"]), "files")), 3);
}

func testVariadicPlus() {
    def p as Parser init positionalList1(parser("t", ""), "files", "");
    testing.assertEqual(len(asList(parse($p, ["prog", "a"]), "files")), 1);
}

func testExactN() {
    def p as Parser init positionalN(parser("t", ""), "pair", 2, "");
    def r as Result init parse($p, ["prog", "a", "b"]);
    testing.assertEqual(len(asList($r, "pair")), 2);
    testing.assertTrue(has($r, "pair"));
}

func testMixedFlagAndPositionals() {
    def p as Parser init parser("t", "");
    $p = boolFlag($p, "v", "v", "");
    $p = positional($p, "cmd", "");
    $p = positionalList($p, "rest", "");
    def r as Result init parse($p, ["prog", "-v", "run", "a", "b"]);
    testing.assertTrue(asBool($r, "v"));
    testing.assertEqual(asString($r, "cmd"), "run");
    testing.assertEqual(len(asList($r, "rest")), 2);
}

# --- required + choices ---

func testRequiredFlagSupplied() {
    def p as Parser init required(flag(parser("t", ""), "key", "k", "", ""));
    testing.assertEqual(asString(parse($p, ["prog", "--key", "v"]), "key"), "v");
}

func testChoicesAccepted() {
    def r as Result init parse(demo(), ["prog", "-m", "release"]);
    testing.assertEqual(asString($r, "mode"), "release");
}

# --- subcommands ---

func testSubcommand() {
    def sub as Parser init positional(parser("add", "add"), "url", "");
    def top as Parser init command(boolFlag(parser("git", ""), "quiet", "q", ""), "add", "add a remote", $sub);
    def r as Result init parse($top, ["prog", "-q", "add", "http://x"]);
    testing.assertEqual($r.command, "add");
    testing.assertEqual(asString($r, "url"), "http://x");
    testing.assertTrue(asBool($r, "quiet"));
}

# --- help / version ---

func testHelpLong() {
    def r as Result init parse(demo(), ["prog", "--help"]);
    testing.assertTrue($r.done);
    testing.assertContains($r.helpText, "Usage: demo");
}

func testHelpShort() {
    def r as Result init parse(demo(), ["prog", "-h"]);
    testing.assertTrue($r.done);
}

func testVersion() {
    def p as Parser init version(parser("t", ""), "1.2.3");
    def r as Result init parse($p, ["prog", "--version"]);
    testing.assertTrue($r.done);
    testing.assertEqual($r.helpText, "1.2.3");
}

func testUsageContent() {
    def sub as Parser init parser("go", "run it");
    def p as Parser init command(demo(), "go", "run it", $sub);
    $p = version($p, "9");
    $p = positional($p, "target", "the target");
    def u as string init usage($p);
    testing.assertContains($u, "Options:");
    testing.assertContains($u, "--name");
    testing.assertContains($u, "one of: debug, release");
    testing.assertContains($u, "Commands:");
    testing.assertContains($u, "Positional arguments:");
    testing.assertContains($u, "--version");
}

func testUsageNargsForms() {
    def p as Parser init positionalOpt(parser("t", ""), "a", "", "");
    $p = positionalList1($p, "b", "");
    def u as string init usage($p);
    testing.assertContains($u, "[a]");
    testing.assertContains($u, "b [b ...]");
}

# --- private helpers ---

func testIsIntStr() {
    testing.assertTrue(isIntStr("42"));
    testing.assertFalse(isIntStr(""));
    testing.assertFalse(isIntStr("4a"));
}

func testIsShortFlag() {
    testing.assertTrue(isShortFlag("-x"));
    testing.assertFalse(isShortFlag("--x"));
    testing.assertFalse(isShortFlag("-5"));
    testing.assertFalse(isShortFlag("x"));
    testing.assertFalse(isShortFlag("-"));
}

func testIsLongFlag() {
    testing.assertTrue(isLongFlag("--x"));
    testing.assertFalse(isLongFlag("-x"));
    testing.assertFalse(isLongFlag("--"));
}

func testFindLookups() {
    def p as Parser init demo();
    testing.assertTrue(findLong($p, "name") >= 0);
    testing.assertEqual(findLong($p, "nope"), -1);
    testing.assertTrue(findShort($p, "n") >= 0);
    testing.assertEqual(findShort($p, "z"), -1);
    testing.assertEqual(findCommand($p, "x"), -1);
}

func testArgLabel() {
    def fa as Arg init Arg{name: "name", short: "n", kind: "flag", typ: "string", action: "store",
        fallback: "", hasDefault: false, required: false, nargs: "", choices: [], help: ""};
    def pa as Arg init Arg{name: "target", short: "", kind: "positional", typ: "string", action: "store",
        fallback: "", hasDefault: false, required: false, nargs: "", choices: [], help: ""};
    testing.assertEqual(argLabel($fa), "--name");
    testing.assertEqual(argLabel($pa), "target");
}

# --- error cases ---

func errBadChoice() {
    return parse(demo(), ["prog", "-m", "nope"]);
}
func errBadInt() {
    def p as Parser init intFlag(parser("t", ""), "n", "n", 0, "");
    return parse($p, ["prog", "--n", "abc"]);
}
func errBadFloat() {
    def p as Parser init floatFlag(parser("t", ""), "f", "f", 0.0, "");
    return parse($p, ["prog", "--f", "xx"]);
}
func errMissingPositional() {
    return parse(positional(parser("t", ""), "x", ""), ["prog"]);
}
func errMissingRequiredFlag() {
    return parse(required(flag(parser("t", ""), "k", "k", "", "")), ["prog"]);
}
func errMissingPlus() {
    return parse(positionalList1(parser("t", ""), "x", ""), ["prog"]);
}
func errExactNShort() {
    return parse(positionalN(parser("t", ""), "p", 2, ""), ["prog", "a"]);
}
func errUnknownLong() {
    return parse(demo(), ["prog", "--bogus", "x"]);
}
func errUnknownShort() {
    return parse(demo(), ["prog", "-Z"]);
}
func errUnknownCommand() {
    def sub as Parser init parser("a", "");
    return parse(command(parser("t", ""), "a", "", $sub), ["prog", "b"]);
}
func errMissingValueLong() {
    return parse(flag(parser("t", ""), "k", "k", "d", ""), ["prog", "--k"]);
}
func errMissingValueShort() {
    return parse(flag(parser("t", ""), "k", "k", "d", ""), ["prog", "-k"]);
}
func errExtraPositional() {
    return parse(positional(parser("t", ""), "x", ""), ["prog", "a", "b"]);
}
func errRequiredBeforeAdd() {
    return required(parser("t", ""));
}
func errChoicesBeforeAdd() {
    return choices(parser("t", ""), ["a"]);
}

func testErrorsThrow() {
    testing.assertThrows("errBadChoice", "args");
    testing.assertThrows("errBadInt", "args");
    testing.assertThrows("errBadFloat", "args");
    testing.assertThrows("errMissingPositional", "args");
    testing.assertThrows("errMissingRequiredFlag", "args");
    testing.assertThrows("errMissingPlus", "args");
    testing.assertThrows("errExactNShort", "args");
    testing.assertThrows("errUnknownLong", "args");
    testing.assertThrows("errUnknownShort", "args");
    testing.assertThrows("errUnknownCommand", "args");
    testing.assertThrows("errMissingValueLong", "args");
    testing.assertThrows("errMissingValueShort", "args");
    testing.assertThrows("errExtraPositional", "args");
    testing.assertThrows("errRequiredBeforeAdd", "args");
    testing.assertThrows("errChoicesBeforeAdd", "args");
}

# --- accessors on absent keys ---

func testAccessorsAbsent() {
    def r as Result init parse(parser("t", ""), ["prog"]);
    testing.assertEqual(asString($r, "nope"), "");
    testing.assertEqual(asInt($r, "nope"), 0);
    testing.assertEqual(asFloat($r, "nope"), 0.0);
    testing.assertFalse(asBool($r, "nope"));
    testing.assertEqual(len(asList($r, "nope")), 0);
    testing.assertEqual(count($r, "nope"), 0);
    testing.assertFalse(has($r, "nope"));
}

# --- subcommand dispatch ---

func cmdAdd(r as Result) { return "added:" + asString($r, "name"); }
func cmdRemove(r as Result) { return "removed"; }

func twoCmdParser() {
    def p as Parser init parser("tool", "");
    def addSub as Parser init parser("add", "");
    $addSub = positional($addSub, "name", "");
    $p = command($p, "add", "", $addSub);
    def rmSub as Parser init parser("remove", "");
    $p = command($p, "remove", "", $rmSub);
    return $p;
}

func testDispatchRoutesToHandler() {
    def r as Result init parse(twoCmdParser(), ["tool", "add", "gadget"]);
    def h as map of string to func init {"add": cmdAdd, "remove": cmdRemove};
    testing.assertEqual(dispatch($r, $h), "added:gadget");
}

func testDispatchRoutesSecond() {
    def r as Result init parse(twoCmdParser(), ["tool", "remove"]);
    def h as map of string to func init {"add": cmdAdd, "remove": cmdRemove};
    testing.assertEqual(dispatch($r, $h), "removed");
}

func testDispatchDoneReturnsNull() {
    def r as Result init parse(twoCmdParser(), ["tool", "--help"]);
    def h as map of string to func init {"add": cmdAdd};
    testing.assertEqual(convert.typeOf(dispatch($r, $h)), "null");
}

func errDispatchNoHandler() {
    def r as Result init parse(twoCmdParser(), ["tool", "add", "x"]);
    def h as map of string to func init {"remove": cmdRemove};
    dispatch($r, $h);
}

func errDispatchNoCommand() {
    def r as Result init parse(parser("tool", ""), ["tool"]);
    def h as map of string to func init {};
    dispatch($r, $h);
}

func testDispatchErrors() {
    testing.assertThrows("errDispatchNoHandler", "args");
    testing.assertThrows("errDispatchNoCommand", "args");
}
