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
	corelib "github.com/mplx/jennifer-lang/internal/lib/core"
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
	corelib.Install(in)
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
printf("%s\n", os.platform());
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// runtime.GOOS varies between hosts; just make sure something
	// non-empty printed and ended with a newline.
	if !strings.HasSuffix(out, "\n") || strings.TrimSpace(out) == "" {
		t.Errorf("got %q", out)
	}
}

func TestNamespacedConstResolves(t *testing.T) {
	out, err := runNS(t, `
use io;
use bio;
printf("%d\n", bio.STOPS);
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
printf("%s\n", bio.translate("ATG"));
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
printf("%s\n", b.translate("X"));
printf("%d\n", b.STOPS);
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
    printf("custom bio call\n");
}
bio();
printf("%s\n", b.translate("Y"));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "custom bio call\nAA-Y\n" {
		t.Errorf("got %q", out)
	}
}

func TestNamespaceAsRejectedForFlatLib(t *testing.T) {
	// `math` is flat-only; an `as` clause on it is meaningless.
	_, err := runNS(t, `use math as m;`)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "meaningless") {
		t.Errorf("err = %v", err)
	}
}

func TestNamespacedConstantInExpressions(t *testing.T) {
	out, err := runNS(t, `
use io;
use bio;
def total as int init bio.STOPS * 2 + 1;
printf("%d\n", $total);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "7\n" {
		t.Errorf("got %q", out)
	}
}

func TestOsJenniferConstants(t *testing.T) {
	out, err := runNS(t, `
use io;
use os;
printf("%s", os.JENNIFER_LF);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "\n" && out != "\r\n" {
		t.Errorf("os.JENNIFER_LF was %q (expected \\n or \\r\\n)", out)
	}
}
