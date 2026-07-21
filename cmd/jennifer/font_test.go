// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestFontModule drives the font module through the real import + file path:
// font.open reads the committed TrueType fixture (modules/testdata/font_fixture.ttf,
// regenerable with scripts/gen-font-fixture.py) and the .j program asserts its
// units-per-em, family, advances, and glyph outlines (a straight-line glyph, a
// quadratic-curve glyph, and a composite glyph). A mismatch throws and fails
// loadForTest.
func TestFontModule(t *testing.T) {
	fontMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "font.j"))
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := filepath.Abs(filepath.Join("..", "..", "modules", "testdata", "font_fixture.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	prog := fmt.Sprintf(`use testing;
import %q as font;

def f as font.Font init font.open(%q);
testing.assertEqual(font.unitsPerEm($f), 1000);
testing.assertEqual(font.name($f), "JenFixture");
testing.assertEqual(font.advance($f, 65), 600);
testing.assertEqual(font.advance($f, 67), 700);

# straight-line glyph, quadratic glyph, and composite glyph.
testing.assertEqual(font.glyphPath($f, 65), "M 100 0 L 500 0 L 300 700 L 100 0 Z");
testing.assertEqual(font.glyphPath($f, 66), "M 100 0 L 100 600 Q 450 600 450 300 Q 450 0 100 0 Z");
testing.assertEqual(font.glyphPath($f, 67), "M 300 0 L 700 0 L 500 700 L 300 0 Z");

def gl as font.Glyph init font.glyph($f, 65);
testing.assertEqual(len($gl.contours), 1);
testing.assertEqual($gl.xMax, 500);`, fontMod, fixture)

	dir := t.TempDir()
	progPath := filepath.Join(dir, "font.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("font program failed with code %d", code)
	}
}
