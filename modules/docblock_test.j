# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
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
    def doc as FileDoc init parse("/**\n * F.\n * @param name \{string\} who to greet,\n * spanning two lines\n * @return \{string\} the greeting\n * also wrapped\n */\nexport func f(name as string) \{ return \"x\"; \}");
    def fn as FuncDoc init $doc.funcs[0];
    testing.assertEqual($fn.params[0].name, "name");
    testing.assertContains($fn.params[0].description, "spanning two lines");
    testing.assertContains($fn.returns.description, "also wrapped");
}

func testExportedFuncWithParams() {
    def doc as FileDoc init parse("/**\n * Greet.\n * @param name \{string\} who\n * @return \{string\} greeting\n */\nexport func greet(name as string) \{ return \"x\"; \}");
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
    def doc as FileDoc init parse("/** helper */\nfunc helper() \{ return; \}");
    testing.assertEqual(len($doc.funcs), 1);
    testing.assertFalse($doc.funcs[0].exported);
}

func testStructFields() {
    def doc as FileDoc init parse("/**\n * A point.\n * @field x \{int\} the x\n * @field y \{int\} the y\n */\nexport def struct Point \{ x as int, y as int \};");
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

# An enum (sum type) is a documentable construct: its doc parses to an EnumDoc,
# no orphaned-comment diagnostic, and payload-carrying variants are fine (only
# the name is read).
func testEnumRecognized() {
    def doc as FileDoc init parse("/**\n * A shape.\n * A longer note.\n */\nexport def enum Shape \{ Circle\{r as int\}, Square, Empty \};");
    testing.assertEqual(len($doc.enums), 1);
    testing.assertEqual(len($doc.diagnostics), 0);
    def e as EnumDoc init $doc.enums[0];
    testing.assertEqual($e.name, "Shape");
    testing.assertTrue($e.exported);
    testing.assertEqual($e.summary, "A shape.");
}

# A private enum is recognized too, and marked unexported.
func testEnumUnexported() {
    def doc as FileDoc init parse("/** private tag */\ndef enum Tag \{ A, B \};");
    testing.assertEqual(len($doc.enums), 1);
    testing.assertFalse($doc.enums[0].exported);
}

# An enum documents its variants in prose, so a stray @field / @return usually
# means a struct's doc drifted onto the enum below it - flagged as a diagnostic.
func testEnumWithFieldTagsWarns() {
    def doc as FileDoc init parse("/**\n * Mislabelled.\n * @field x \{int\} a field\n */\nexport def enum Bad \{ A, B \};");
    testing.assertEqual(len($doc.enums), 1);
    testing.assertEqual(len($doc.diagnostics), 1);
    testing.assertTrue(strings.contains($doc.diagnostics[0].message, "mis-attached"));
}

# --- digit-inclusive identifiers --------------------------------------------

# A func name with an interior/trailing digit (uuid.v4-style) parses.
func testFuncNameWithDigit() {
    def doc as FileDoc init parse("/**\n * Make a v4 UUID.\n * @return \{string\} the uuid\n */\nexport func v4() \{ return \"x\"; \}");
    testing.assertEqual(len($doc.funcs), 1);
    testing.assertEqual($doc.funcs[0].name, "v4");
    testing.assertTrue($doc.funcs[0].exported);
}

# A @param name with a trailing digit binds to a real parameter (no diagnostic).
func testParamNameWithDigit() {
    def doc as FileDoc init parse("/**\n * F.\n * @param x2 \{int\} second x\n */\nfunc f(x2 as int) \{ return; \}");
    testing.assertEqual(len($doc.funcs), 1);
    testing.assertEqual($doc.funcs[0].params[0].name, "x2");
    testing.assertEqual($doc.funcs[0].params[0].type, "int");
    testing.assertEqual(len($doc.diagnostics), 0);
}

# A struct field name with a trailing digit (md5) parses and matches (no diag).
func testStructFieldWithDigit() {
    def doc as FileDoc init parse("/**\n * Hashes.\n * @field md5 \{string\} the md5 hex\n */\nexport def struct Hashes \{ md5 as string \};");
    testing.assertEqual(len($doc.structs), 1);
    testing.assertEqual($doc.structs[0].fields[0].name, "md5");
    testing.assertEqual(len($doc.diagnostics), 0);
}

# A constant name with an in-chunk digit (SHA256) parses.
func testConstNameWithDigit() {
    def doc as FileDoc init parse("/** sha256 block size */\ndef const SHA256 as int init 64;");
    testing.assertEqual(len($doc.consts), 1);
    testing.assertEqual($doc.consts[0].name, "SHA256");
    testing.assertEqual($doc.consts[0].type, "int");
}

# A multi-chunk constant with a digit chunk (SCRAM_SHA256) parses.
func testConstNameMultiChunkDigit() {
    def doc as FileDoc init parse("/** scram mechanism */\nexport def const SCRAM_SHA256 as string init \"x\";");
    testing.assertEqual(len($doc.consts), 1);
    testing.assertEqual($doc.consts[0].name, "SCRAM_SHA256");
    testing.assertTrue($doc.consts[0].exported);
}

# A digit-initial @param name is not a legal identifier: parseParam falls back
# to firstWord (no {type} capture), so the type stays empty.
func testDigitInitialParamNotIdent() {
    def p as ParamDoc init parseParam('2x {int} bogus');
    testing.assertEqual($p.type, "");
}

# --- diagnostics ------------------------------------------------------------

func testBadParamDiagnostic() {
    def doc as FileDoc init parse("/**\n * F.\n * @param bogus \{int\} nope\n */\nfunc f(real as int) \{ return; \}");
    # bogus is not a parameter -> one diag; real is undocumented -> one diag
    testing.assertEqual(len($doc.diagnostics), 2);
}

func testUndocumentedParamDiagnostic() {
    def doc as FileDoc init parse("/** F */\nfunc f(x as int) \{ return; \}");
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
    def doc as FileDoc init parse("/** real */\nexport func f() \{ return \"/** fake */\"; \}");
    testing.assertEqual(len($doc.funcs), 1);
    testing.assertEqual($doc.funcs[0].summary, "real");
    testing.assertEqual(len($doc.diagnostics), 0);
}

func testNestedBlockCommentInBody() {
    def doc as FileDoc init parse("/**\n * summary\n * nested /* c */ inside\n */\nexport func g() \{ return; \}");
    testing.assertEqual(len($doc.funcs), 1);
    testing.assertEqual($doc.funcs[0].name, "g");
    testing.assertEqual($doc.funcs[0].summary, "summary");
}

func testPlainBlockCommentInvisible() {
    def doc as FileDoc init parse("/* just a plain comment */\nfunc h() \{ return; \}");
    # a plain /* */ is not a doc comment: no docs, so h is undocumented (absent)
    testing.assertEqual(len($doc.funcs), 0);
    testing.assertEqual(len($doc.diagnostics), 0);
}
