# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# docblock_test.j - white-box tests for docblock.j. Run with:
#
#     jennifer test modules/docblock_test.j
#
# The overlay splices docblock.j in first, so these tests reach both its public
# surface (parse) and its private helpers (declNames, cleanLine) by bare
# identifier. docblock.j already `use`s regex / strings / lists, so the overlay
# adds testing.
use testing;

# --- private helpers --------------------------------------------------------

func testDeclNames() {
    def names as list of string init declNames("a as int, b as string");
    testing.assertEqual(len($names), 2);
    testing.assertEqual($names[0], "a");
    testing.assertEqual($names[1], "b");
    testing.assertEqual(len(declNames("")), 0);
}

func testCleanLine() {
    testing.assertEqual(cleanLine(" * hello "), "hello");
    testing.assertEqual(cleanLine("plain"), "plain");
}

# --- constructs -------------------------------------------------------------

func testModulePreamble() {
    def doc as FileDoc init parse("/**\n * The greet lib.\n * @module greet\n * @author edv\n * @version 2.0\n */\nuse io;");
    testing.assertEqual($doc.module.summary, "The greet lib.");
    testing.assertEqual($doc.module.author, "edv");
    testing.assertEqual($doc.module.version, "2.0");
}

# A wrapped @param / @return description (continuation lines) is captured, not
# truncated to its first line.
func testParamContinuationLines() {
    def doc as FileDoc init parse("/**\n * F.\n * @param name {string} who to greet,\n * spanning two lines\n * @return {string} the greeting\n * also wrapped\n */\nexport func f(name as string) { return \"x\"; }");
    def fn as FuncDoc init $doc.funcs[0];
    testing.assertEqual($fn.params[0].name, "name");
    testing.assertContains($fn.params[0].description, "spanning two lines");
    testing.assertContains($fn.returns.description, "also wrapped");
}

func testExportedFuncWithParams() {
    def doc as FileDoc init parse("/**\n * Greet.\n * @param name {string} who\n * @return {string} greeting\n */\nexport func greet(name as string) { return \"x\"; }");
    testing.assertEqual(len($doc.funcs), 1);
    def f as FuncDoc init $doc.funcs[0];
    testing.assertEqual($f.name, "greet");
    testing.assertTrue($f.exported);
    testing.assertEqual($f.summary, "Greet.");
    testing.assertEqual(len($f.params), 1);
    testing.assertEqual($f.params[0].name, "name");
    testing.assertEqual($f.params[0].type, "string");
    testing.assertEqual($f.returns.type, "string");
    testing.assertEqual(len($doc.diagnostics), 0);
}

func testPrivateFuncNotExported() {
    def doc as FileDoc init parse("/** helper */\nfunc helper() { return; }");
    testing.assertEqual(len($doc.funcs), 1);
    testing.assertFalse($doc.funcs[0].exported);
}

func testStructFields() {
    def doc as FileDoc init parse("/**\n * A point.\n * @field x {int} the x\n * @field y {int} the y\n */\nexport def struct Point { x as int, y as int };");
    testing.assertEqual(len($doc.structs), 1);
    def s as StructDoc init $doc.structs[0];
    testing.assertEqual($s.name, "Point");
    testing.assertTrue($s.exported);
    testing.assertEqual(len($s.fields), 2);
    testing.assertEqual($s.fields[1].name, "y");
    testing.assertEqual($s.fields[1].type, "int");
}

func testConst() {
    def doc as FileDoc init parse("/** max retries */\ndef const MAX_RETRIES as int init 5;");
    testing.assertEqual(len($doc.consts), 1);
    testing.assertEqual($doc.consts[0].name, "MAX_RETRIES");
    testing.assertEqual($doc.consts[0].type, "int");
    testing.assertFalse($doc.consts[0].exported);
}

# --- diagnostics ------------------------------------------------------------

func testBadParamDiagnostic() {
    def doc as FileDoc init parse("/**\n * F.\n * @param bogus {int} nope\n */\nfunc f(real as int) { return; }");
    # bogus is not a parameter -> one diag; real is undocumented -> one diag
    testing.assertEqual(len($doc.diagnostics), 2);
}

func testUndocumentedParamDiagnostic() {
    def doc as FileDoc init parse("/** F */\nfunc f(x as int) { return; }");
    testing.assertEqual(len($doc.diagnostics), 1);
    testing.assertContains($doc.diagnostics[0].message, "has no @param");
}

func testOrphanReported() {
    def doc as FileDoc init parse("/** orphan */\n");
    testing.assertEqual(len($doc.diagnostics), 1);
    testing.assertContains($doc.diagnostics[0].message, "orphaned");
}

# --- scanner edge cases -----------------------------------------------------

func testDocStartInStringIgnored() {
    def doc as FileDoc init parse("/** real */\nexport func f() { return \"/** fake */\"; }");
    testing.assertEqual(len($doc.funcs), 1);
    testing.assertEqual($doc.funcs[0].summary, "real");
    testing.assertEqual(len($doc.diagnostics), 0);
}

func testNestedBlockCommentInBody() {
    def doc as FileDoc init parse("/**\n * summary\n * nested /* c */ inside\n */\nexport func g() { return; }");
    testing.assertEqual(len($doc.funcs), 1);
    testing.assertEqual($doc.funcs[0].name, "g");
    testing.assertEqual($doc.funcs[0].summary, "summary");
}

func testPlainBlockCommentInvisible() {
    def doc as FileDoc init parse("/* just a plain comment */\nfunc h() { return; }");
    # a plain /* */ is not a doc comment: no docs, so h is undocumented (absent)
    testing.assertEqual(len($doc.funcs), 0);
    testing.assertEqual(len($doc.diagnostics), 0);
}
