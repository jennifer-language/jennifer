// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package metalib_test

import (
	"bytes"
	"runtime"
	"strings"
	"testing"

	"github.com/mplx/jennifer-lang/internal/interpreter"
	iolib "github.com/mplx/jennifer-lang/internal/lib/io"
	metalib "github.com/mplx/jennifer-lang/internal/lib/meta"
	"github.com/mplx/jennifer-lang/internal/parser"
	"github.com/mplx/jennifer-lang/internal/version"
)

func runOne(t *testing.T, src string) string {
	t.Helper()
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	metalib.Install(in)
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := in.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	return buf.String()
}

// TestVersionConstantMatchesPackage confirms the constant the user sees as
// meta.VERSION is the same string the rest of the binary uses.
func TestVersionConstantMatchesPackage(t *testing.T) {
	got := runOne(t, `use io; use meta; io.printf("%s", meta.VERSION);`)
	if got != version.Version {
		t.Errorf("meta.VERSION = %q, want %q", got, version.Version)
	}
}

// TestBuildConstantReportsCompiler confirms JENNIFER_BUILD is one of the
// expected tags. "gc" gets normalised to "go" (the user-facing toolchain
// name); "tinygo" passes through; any other value passes through too.
func TestBuildConstantReportsCompiler(t *testing.T) {
	got := runOne(t, `use io; use meta; io.printf("%s", meta.BUILD);`)
	var want string
	switch runtime.Compiler {
	case "gc":
		want = "go"
	default:
		want = runtime.Compiler
	}
	if got != want {
		t.Errorf("meta.BUILD = %q, want %q", got, want)
	}
}

// TestBareVersionNoLongerAvailable confirms bare JENNIFER_VERSION at
// use site is now an error since the constant moved to meta and was
// then renamed to meta.VERSION.
func TestBareVersionNoLongerAvailable(t *testing.T) {
	in := interpreter.New()
	iolib.Install(in)
	metalib.Install(in)
	src := `use io; io.printf("%s", JENNIFER_VERSION);`
	prog, err := parser.Parse(src)
	if err != nil {
		// Acceptable: parser rejects the bare reference outright.
		return
	}
	err = in.Run(prog)
	if err == nil {
		t.Fatal("expected error for bare JENNIFER_VERSION reference, got nil")
	}
	if !strings.Contains(err.Error(), "undefined") && !strings.Contains(err.Error(), "JENNIFER_VERSION") {
		t.Errorf("error doesn't mention the missing constant: %v", err)
	}
}
