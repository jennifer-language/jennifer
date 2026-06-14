// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package interpreter_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mplx/jennifer-lang/internal/interpreter"
	"github.com/mplx/jennifer-lang/internal/lib/convert"
	iolib "github.com/mplx/jennifer-lang/internal/lib/io"
	listslib "github.com/mplx/jennifer-lang/internal/lib/lists"
	mapslib "github.com/mplx/jennifer-lang/internal/lib/maps"
	mathlib "github.com/mplx/jennifer-lang/internal/lib/math"
	oslib "github.com/mplx/jennifer-lang/internal/lib/os"
	stringslib "github.com/mplx/jennifer-lang/internal/lib/strings"
	"github.com/mplx/jennifer-lang/internal/parser"
)

// newNSInterp wires the standard libs plus a synthetic `bio` namespace
// registered through the public RegisterNamespaced API. The synthetic
// namespace lets us exercise paths the shipping `os` slice doesn't cover
// (calls with several args, returns of different kinds, multiple
// constants in one namespace) without a stub library package on disk.
func newNSInterp() (*interpreter.Interpreter, *bytes.Buffer) {
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
	in.RegisterNamespaced("bio", "translate", func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		if len(args) != 1 || args[0].Kind != interpreter.KindString {
			return interpreter.Null(), nil
		}
		return interpreter.StringVal("AA-" + args[0].Str), nil
	})
	in.RegisterNamespacedConst("bio", "STOPS", interpreter.IntVal(3))
	return in, &buf
}

func runNS(t *testing.T, src string) (string, error) {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		return "", err
	}
	in, buf := newNSInterp()
	if err := in.Run(prog); err != nil {
		return buf.String(), err
	}
	return buf.String(), nil
}

func TestNamespaceBuiltinResolves(t *testing.T) {
	out, err := runNS(t, `
use io;
use os;
io.printf("%s\n", os.getEnv("PATH"));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// PATH varies between hosts; just make sure something printed and
	// ended with a newline (an empty PATH still produces just "\n").
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("got %q", out)
	}
}

func TestNamespacedConstResolves(t *testing.T) {
	out, err := runNS(t, `
use io;
use bio;
io.printf("%d\n", bio.STOPS);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "3\n" {
		t.Errorf("got %q", out)
	}
}

func TestNamespacedCallWithArg(t *testing.T) {
	out, err := runNS(t, `
use io;
use bio;
io.printf("%s\n", bio.translate("ATG"));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "AA-ATG\n" {
		t.Errorf("got %q", out)
	}
}

func TestUnknownNamespaceErrors(t *testing.T) {
	_, err := runNS(t, `bio.translate("X");`)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "requires `use bio;`") {
		t.Errorf("err = %v", err)
	}
}

func TestUnknownNamespaceCompletelyUnknown(t *testing.T) {
	_, err := runNS(t, `dragons.fly();`)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown namespace") {
		t.Errorf("err = %v", err)
	}
}

func TestUnknownNamespacedFunctionErrors(t *testing.T) {
	_, err := runNS(t, `use bio; bio.transmute("X");`)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), `unknown function "transmute"`) {
		t.Errorf("err = %v", err)
	}
}

func TestAliasMakesPrefixUsable(t *testing.T) {
	out, err := runNS(t, `
use io;
use bio as b;
io.printf("%s\n", b.translate("X"));
io.printf("%d\n", b.STOPS);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "AA-X\n3\n" {
		t.Errorf("got %q", out)
	}
}

func TestAliasShadowsCanonicalName(t *testing.T) {
	_, err := runNS(t, `
use bio as b;
bio.translate("X");
`)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), `did you mean ` + "`b`") {
		t.Errorf("err = %v", err)
	}
}

func TestUserMethodCannotShadowImportedNamespace(t *testing.T) {
	_, err := runNS(t, `
use bio;
func bio() {}
`)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "shadows imported namespace") {
		t.Errorf("err = %v", err)
	}
}

func TestUserMethodMayUseCanonicalNameAfterAlias(t *testing.T) {
	// `use bio as b;` reserves `b` as a prefix but frees `bio` for
	// user-method use - matches Python's import-as semantics.
	out, err := runNS(t, `
use io;
use bio as b;
func bio() {
    io.printf("custom bio call\n");
}
bio();
io.printf("%s\n", b.translate("Y"));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "custom bio call\nAA-Y\n" {
		t.Errorf("got %q", out)
	}
}

func TestMathAliasingWorks(t *testing.T) {
	// After M10 every library is namespaced (including math), so
	// `use math as m;` is a valid rename. The pre-M10 "flat-only,
	// alias meaningless" rejection rule was removed in M10.
	out, err := runNS(t, `
use io;
use math as m;
io.printf("%f\n", m.PI);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.HasPrefix(out, "3.14") {
		t.Errorf("got %q", out)
	}
}

func TestNamespacedConstantInExpressions(t *testing.T) {
	out, err := runNS(t, `
use io;
use bio;
def total as int init bio.STOPS * 2 + 1;
io.printf("%d\n", $total);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "7\n" {
		t.Errorf("got %q", out)
	}
}

func TestOsEOLConstant(t *testing.T) {
	out, err := runNS(t, `
use io;
use os;
io.printf("%s", os.EOL);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "\n" && out != "\r\n" {
		t.Errorf("os.EOL was %q (expected \\n or \\r\\n)", out)
	}
}

// ---- Globals collision and alias-meaningless tests (M10 hardening) ----

// twoGlobalsInterp registers two synthetic libraries `alfa` and `beta`,
// each publishing a `VER` constant via RegisterGlobalConst, plus a
// globals-only `solo` library with no namespaced names. Tests use this
// to exercise the M10 cross-library-collision and alias-meaningless
// rules without shipping bogus libraries in the standard set.
func twoGlobalsInterp() *interpreter.Interpreter {
	in, _ := newNSInterp()
	in.RegisterGlobalConst("alfa", "VER", interpreter.StringVal("alfa-1"))
	in.RegisterGlobalConst("beta", "VER", interpreter.StringVal("beta-2"))
	in.RegisterGlobalConst("solo", "ONLY", interpreter.StringVal("solo-only"))
	return in
}

func runWithExtraGlobals(t *testing.T, src string) (string, error) {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		return "", err
	}
	in := twoGlobalsInterp()
	var buf bytes.Buffer
	in.Out = &buf
	if err := in.Run(prog); err != nil {
		return buf.String(), err
	}
	return buf.String(), nil
}

// Single-library import of a globals-publishing lib resolves the global
// to that library's value. Other libraries' globals stay invisible.
func TestSingleGlobalsLibResolves(t *testing.T) {
	out, err := runWithExtraGlobals(t, `
use io;
use alfa;
io.printf("%s\n", VER);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "alfa-1\n" {
		t.Errorf("got %q, want %q", out, "alfa-1\n")
	}
}

// Symmetric: importing `beta` alone resolves `VER` to beta's value.
// This is the regression check for the pre-fix bug where late-registered
// libraries silently overwrote earlier ones in the resolution map.
func TestOtherSingleGlobalsLibResolves(t *testing.T) {
	out, err := runWithExtraGlobals(t, `
use io;
use beta;
io.printf("%s\n", VER);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "beta-2\n" {
		t.Errorf("got %q, want %q", out, "beta-2\n")
	}
}

// Importing both libraries that publish the same global is the
// collision case from the M10 hardening. The second `use` errors.
func TestTwoLibsPublishingSameGlobalCollides(t *testing.T) {
	_, err := runWithExtraGlobals(t, `
use io;
use alfa;
use beta;
`)
	if err == nil {
		t.Fatal("expected collision error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "collides") || !strings.Contains(msg, `"VER"`) {
		t.Errorf("error should mention collision and the global name, got: %v", err)
	}
	// Both library names should appear so the user can pick which to drop.
	if !strings.Contains(msg, "alfa") || !strings.Contains(msg, "beta") {
		t.Errorf("error should name both libraries, got: %v", err)
	}
}

// Aliasing a globals-only library is meaningless - there is no
// namespace prefix to rename. Reject upfront at the `use` statement.
func TestAliasOfGlobalsOnlyLibraryIsMeaningless(t *testing.T) {
	_, err := runWithExtraGlobals(t, `
use solo as foo;
`)
	if err == nil {
		t.Fatal("expected alias-meaningless error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "no namespaced names") || !strings.Contains(msg, "meaningless") {
		t.Errorf("error should mention no-namespaced-names + meaningless, got: %v", err)
	}
}

// A globals-only library imported without `as ALIAS` works fine; the
// global is reachable as a bare name, and no namespace prefix is
// reserved.
func TestGlobalsOnlyLibraryWithoutAliasWorks(t *testing.T) {
	out, err := runWithExtraGlobals(t, `
use io;
use solo;
io.printf("%s\n", ONLY);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "solo-only\n" {
		t.Errorf("got %q", out)
	}
}
