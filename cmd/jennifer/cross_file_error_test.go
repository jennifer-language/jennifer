// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lexer"
	"jennifer-lang.dev/jennifer/internal/lib/convert"
	"jennifer-lang.dev/jennifer/internal/lib/io"
	listslib "jennifer-lang.dev/jennifer/internal/lib/lists"
	mapslib "jennifer-lang.dev/jennifer/internal/lib/maps"
	"jennifer-lang.dev/jennifer/internal/lib/math"
	oslib "jennifer-lang.dev/jennifer/internal/lib/os"
	stringslib "jennifer-lang.dev/jennifer/internal/lib/strings"
	"jennifer-lang.dev/jennifer/internal/parser"
	"jennifer-lang.dev/jennifer/internal/preproc"
)

// positionedErr mirrors the CLI's positioned interface so the test can
// assert the same surface the CLI relies on.
type positionedErr interface {
	Position() (file string, line, col int)
}

// TestCrossFileRuntimeError ensures a runtime error raised inside an
// imported `.j` file reports the imported file's path - not the importer's.
func TestCrossFileRuntimeError(t *testing.T) {
	dir := t.TempDir()
	libPath := filepath.Join(dir, "boom.j")
	mainPath := filepath.Join(dir, "main.j")

	// boom.j divides by zero - the runtime error should originate here.
	libSrc := "use io;\nfunc boom() {\n    io.printf(1 / 0);\n}\n"
	mainSrc := "include \"boom.j\";\nboom();\n"

	if err := os.WriteFile(libPath, []byte(libSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainPath, []byte(mainSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	err := runPipeline(mainPath, mainSrc)
	if err == nil {
		t.Fatal("expected runtime error, got nil")
	}

	p, ok := err.(positionedErr)
	if !ok {
		t.Fatalf("error %T does not implement Position()", err)
	}
	file, line, _ := p.Position()
	if !strings.HasSuffix(file, "boom.j") {
		t.Errorf("expected error to point at boom.j, got file=%q", file)
	}
	if line != 3 {
		t.Errorf("expected error at boom.j line 3, got line=%d", line)
	}
}

// TestCrossFileParseError ensures a parse error inside an imported file
// reports the imported file's path.
func TestCrossFileParseError(t *testing.T) {
	dir := t.TempDir()
	libPath := filepath.Join(dir, "broken.j")
	mainPath := filepath.Join(dir, "main.j")

	// broken.j is syntactically invalid (missing semicolon + truncated stmt).
	libSrc := "func broken() {\n    $x = \n}\n"
	mainSrc := "include \"broken.j\";\nbroken();\n"

	if err := os.WriteFile(libPath, []byte(libSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainPath, []byte(mainSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	err := runPipeline(mainPath, mainSrc)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}

	p, ok := err.(positionedErr)
	if !ok {
		t.Fatalf("error %T does not implement Position()", err)
	}
	file, _, _ := p.Position()
	if !strings.HasSuffix(file, "broken.j") {
		t.Errorf("expected error to point at broken.j, got file=%q", file)
	}
}

// TestIncludeDiamondDuplicate pins the report's include finding: a shared file
// spliced twice (a diamond) fails at PARSE time with a message that names the
// splice, instead of a runtime "defined more than once" that only fires after
// output has printed. Struct / func duplicates used to reach this only at
// runtime; now the resolver catches every declaration kind uniformly.
func TestIncludeDiamondDuplicate(t *testing.T) {
	dir := t.TempDir()
	libPath := filepath.Join(dir, "lib.j")
	mainPath := filepath.Join(dir, "main.j")
	// A header of a pure type + function - the most natural thing to factor into
	// a shared include, and exactly the case that used to slip to runtime.
	libSrc := "def struct P { x as int };\nfunc twice(n as int) { return $n * 2; }\n"
	mainSrc := "include \"lib.j\";\ninclude \"lib.j\";\nuse io;\nio.printf(\"%d\\n\", twice(21));\n"
	if err := os.WriteFile(libPath, []byte(libSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainPath, []byte(mainSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	err := runPipeline(mainPath, mainSrc)
	if err == nil {
		t.Fatal("expected a duplicate-declaration error, got nil")
	}
	// Parse phase, not runtime: the resolver's error is a *parser.ParseError, so it
	// is raised before any top-level statement (and any output) runs.
	if _, ok := err.(*parser.ParseError); !ok {
		t.Fatalf("expected a *parser.ParseError (parse phase), got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "defined more than once") {
		t.Fatalf("want `defined more than once`, got %v", err)
	}
	if !strings.Contains(err.Error(), "spliced more than once by `include`") {
		t.Fatalf("want the include-splice hint, got %v", err)
	}
	if p, ok := err.(positionedErr); ok {
		if file, _, _ := p.Position(); !strings.HasSuffix(file, "lib.j") {
			t.Errorf("expected error at the spliced lib.j, got %q", file)
		}
	}
}

// runPipeline mimics the CLI's lex+preproc+parse+run sequence and returns
// the first error encountered (or nil on success).
func runPipeline(mainPath, src string) error {
	absPath, _ := filepath.Abs(mainPath)
	baseDir := filepath.Dir(absPath)

	tokens, err := lexer.TokenizeWithFile(src, absPath)
	if err != nil {
		return err
	}
	tokens, err = preproc.Process(tokens, baseDir, absPath)
	if err != nil {
		return err
	}
	prog, err := parser.ParseTokens(tokens)
	if err != nil {
		return err
	}
	in := interpreter.New()
	iolib.Install(in)
	convert.Install(in)
	mathlib.Install(in)
	stringslib.Install(in)
	listslib.Install(in)
	mapslib.Install(in)
	oslib.Install(in)
	return in.Run(prog)
}
