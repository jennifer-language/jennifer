// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mplx/jennifer-lang/internal/interpreter"
	"github.com/mplx/jennifer-lang/internal/lexer"
	"github.com/mplx/jennifer-lang/internal/lib/convert"
	"github.com/mplx/jennifer-lang/internal/lib/crc"
	"github.com/mplx/jennifer-lang/internal/lib/encoding"
	"github.com/mplx/jennifer-lang/internal/lib/hash"
	"github.com/mplx/jennifer-lang/internal/lib/io"
	"github.com/mplx/jennifer-lang/internal/lib/lists"
	"github.com/mplx/jennifer-lang/internal/lib/maps"
	"github.com/mplx/jennifer-lang/internal/lib/math"
	"github.com/mplx/jennifer-lang/internal/lib/meta"
	"github.com/mplx/jennifer-lang/internal/lib/os"
	"github.com/mplx/jennifer-lang/internal/lib/strings"
	"github.com/mplx/jennifer-lang/internal/lib/task"
	"github.com/mplx/jennifer-lang/internal/lib/time"
	"github.com/mplx/jennifer-lang/internal/parser"
	"github.com/mplx/jennifer-lang/internal/preproc"
)

// TestExamples runs every *.j file under ../../examples/ and asserts its stdout
// matches the corresponding file under ../../examples/expected/.
// Files without a matching expected/ file are skipped (lets us add WIP examples).
func TestExamples(t *testing.T) {
	// Locate the examples directory relative to this test file.
	// `go test` runs with cwd = package dir, so we walk up to repo root.
	root, err := filepath.Abs(filepath.Join("..", "..", "examples"))
	if err != nil {
		t.Fatalf("locate examples: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read examples dir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".j") {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			srcPath := filepath.Join(root, name)
			expectedPath := filepath.Join(root, "expected", strings.TrimSuffix(name, ".j")+".txt")
			expected, err := os.ReadFile(expectedPath)
			if err != nil {
				t.Skipf("no expected file at %s: %v", expectedPath, err)
				return
			}
			src, err := os.ReadFile(srcPath)
			if err != nil {
				t.Fatalf("read %s: %v", srcPath, err)
			}
			absPath, _ := filepath.Abs(srcPath)
			tokens, err := lexer.TokenizeWithFile(string(src), absPath)
			if err != nil {
				t.Fatalf("lex %s: %v", name, err)
			}
			tokens, err = preproc.Process(tokens, filepath.Dir(absPath), absPath)
			if err != nil {
				t.Fatalf("preproc %s: %v", name, err)
			}
			prog, err := parser.ParseTokens(tokens)
			if err != nil {
				t.Fatalf("parse %s: %v", name, err)
			}
			in := interpreter.New()
			var buf bytes.Buffer
			in.Out = &buf
			iolib.Install(in)
			convert.Install(in)
			mathlib.Install(in)
			stringslib.Install(in)
			listslib.Install(in)
			mapslib.Install(in)
			oslib.Install(in)
			metalib.Install(in)
			timelib.Install(in)
			hashlib.Install(in)
			crclib.Install(in)
			encodinglib.Install(in)
			tasklib.Install(in)
			if err := in.Run(prog); err != nil {
				t.Fatalf("run %s: %v", name, err)
			}
			if buf.String() != string(expected) {
				t.Errorf("%s output mismatch:\n--- got ---\n%s\n--- want ---\n%s", name, buf.String(), string(expected))
			}
		})
	}
}
