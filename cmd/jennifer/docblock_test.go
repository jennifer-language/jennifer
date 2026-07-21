// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// A .j program parses a sample source that mixes a module preamble, an exported
// function, an exported struct, a constant, a function whose doc mismatches its
// signature, and an orphaned comment - then asserts the whole FileDoc: each
// construct's fields and `exported` flag, and that the mismatches / orphan
// surface as diagnostics. The sample is written to a temp file and read back so
// the .j source needs no double escaping.
func TestDocblockParse(t *testing.T) {
	docblockMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "docblock.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()

	sample := `/**
 * A demo module.
 * @module demo
 * @author edv
 * @version 1.0
 */

/**
 * Add two numbers.
 * A longer description of add.
 * @param a {int} the first addend
 * @param b {int} the second addend
 * @return {int} the sum
 */
export func add(a as int, b as int) { return $a + $b; }

/**
 * A point in the plane.
 * @field x {int} the x coordinate
 * @field y {int} the y coordinate
 */
export def struct Point { x as int, y as int };

/** The retry ceiling. */
def const MAX as int init 10;

/**
 * A function whose docs drifted.
 * @param wrong {int} names no real parameter
 */
func drifted(real as int) { return; }

/** an orphan comment before nothing */
`
	samplePath := filepath.Join(dir, "sample.j")
	if err := os.WriteFile(samplePath, []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}

	prog := fmt.Sprintf(`use testing;
use fs;
import %q as docblock;

def src as string init fs.readString(%q);
def doc as docblock.FileDoc init docblock.parse($src);

# module preamble
testing.assertEqual($doc.module.summary, "A demo module.");
testing.assertEqual($doc.module.author, "edv");
testing.assertEqual($doc.module.version, "1.0");

# functions: add (documented, exported) and drifted (private)
testing.assertEqual(len($doc.funcs), 2);
testing.assertEqual($doc.funcs[0].name, "add");
testing.assertTrue($doc.funcs[0].exported);
testing.assertEqual($doc.funcs[0].summary, "Add two numbers.");
testing.assertContains($doc.funcs[0].description, "longer description");
testing.assertEqual(len($doc.funcs[0].params), 2);
testing.assertEqual($doc.funcs[0].params[1].name, "b");
testing.assertEqual($doc.funcs[0].returns.type, "int");
testing.assertFalse($doc.funcs[1].exported);

# struct + const
testing.assertEqual(len($doc.structs), 1);
testing.assertTrue($doc.structs[0].exported);
testing.assertEqual(len($doc.structs[0].fields), 2);
testing.assertEqual(len($doc.consts), 1);
testing.assertEqual($doc.consts[0].name, "MAX");
testing.assertEqual($doc.consts[0].type, "int");
testing.assertFalse($doc.consts[0].exported);

# diagnostics: bogus @param + undocumented real param + orphan = 3
testing.assertEqual(len($doc.diagnostics), 3);`, docblockMod, samplePath)

	progPath := filepath.Join(dir, "check.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("docblock program failed with code %d", code)
	}
}
