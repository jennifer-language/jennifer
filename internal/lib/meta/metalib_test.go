// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package metalib_test

import (
	"bytes"
	"testing"

	"github.com/mplx/jennifer-lang/internal/interpreter"
	iolib "github.com/mplx/jennifer-lang/internal/lib/io"
	metalib "github.com/mplx/jennifer-lang/internal/lib/meta"
	"github.com/mplx/jennifer-lang/internal/parser"
	"github.com/mplx/jennifer-lang/internal/version"
)

// TestVersionConstantMatchesPackage ensures the constant exposed to Jennifer
// programs is the same string the rest of the binary uses (CLI help, etc.).
// We don't pin the value - it's set by the build - we just check the wiring.
func TestVersionConstantMatchesPackage(t *testing.T) {
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	metalib.Install(in)

	src := `use io; use meta; printf("%s", VERSION);`
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := in.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := buf.String(); got != version.Version {
		t.Errorf("VERSION constant = %q, want %q", got, version.Version)
	}
}

// TestVersionRequiresUse confirms VERSION isn't auto-available. Without
// `use meta;` a bare `VERSION` should fall through to the usual
// undefined-constant runtime error.
func TestVersionRequiresUse(t *testing.T) {
	in := interpreter.New()
	iolib.Install(in)
	metalib.Install(in)

	src := `use io; printf("%s", VERSION);`
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := in.Run(prog); err == nil {
		t.Fatal("expected error referencing VERSION without `use meta;`, got nil")
	}
}
