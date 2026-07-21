// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"bytes"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// newWidgetInterp registers a synthetic `widgets` namespace carrying a
// `Point` struct (two int fields) and a `Box` struct (two Point
// fields). The two-level nesting exercises both flat
// library-provided structs and library-struct-containing-
// library-struct field chains.
func newWidgetInterp() (*interpreter.Interpreter, *bytes.Buffer) {
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	in.RegisterNamespacedStruct("widgets", "Point", []parser.StructField{
		{Name: "x", Type: parser.PrimitiveType(parser.TypeInt)},
		{Name: "y", Type: parser.PrimitiveType(parser.TypeInt)},
	})
	in.RegisterNamespacedStruct("widgets", "Box", []parser.StructField{
		{Name: "topLeft", Type: parser.NamespacedStructType("widgets", "Point")},
		{Name: "bottomRight", Type: parser.NamespacedStructType("widgets", "Point")},
	})
	return in, &buf
}

func runWidget(t *testing.T, src string) (string, error) {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		return "", err
	}
	in, buf := newWidgetInterp()
	if err := in.Run(prog); err != nil {
		return buf.String(), err
	}
	return buf.String(), nil
}

func TestNamespacedStructDeclarationAndLiteral(t *testing.T) {
	out, err := runWidget(t, `
use io;
use widgets;
def p as widgets.Point init widgets.Point{ x: 3, y: 4 };
io.printf("%v\n", $p);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "widgets.Point{x: 3, y: 4}\n" {
		t.Errorf("got %q", out)
	}
}

func TestNamespacedStructFieldReadWrite(t *testing.T) {
	out, err := runWidget(t, `
use io;
use widgets;
def p as widgets.Point init widgets.Point{ x: 1, y: 2 };
io.printf("read: %d %d\n", $p.x, $p.y);
$p.x = 99;
io.printf("after write: %v\n", $p);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := "read: 1 2\nafter write: widgets.Point{x: 99, y: 2}\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestNamespacedStructZeroInit(t *testing.T) {
	out, err := runWidget(t, `
use io;
use widgets;
def p as widgets.Point;
io.printf("%v\n", $p);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "widgets.Point{x: 0, y: 0}\n" {
		t.Errorf("got %q", out)
	}
}

func TestNamespacedStructNestedZeroInit(t *testing.T) {
	out, err := runWidget(t, `
use io;
use widgets;
def b as widgets.Box;
io.printf("%v\n", $b);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := "widgets.Box{topLeft: widgets.Point{x: 0, y: 0}, bottomRight: widgets.Point{x: 0, y: 0}}\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestNamespacedStructAliasedPrefix(t *testing.T) {
	// `use widgets as w;` makes `w.Point` resolve to the canonical
	// widgets.Point - both in type position and in literal position.
	// Display uses the canonical name.
	out, err := runWidget(t, `
use io;
use widgets as w;
def p as w.Point init w.Point{ x: 5, y: 6 };
io.printf("%v\n", $p);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "widgets.Point{x: 5, y: 6}\n" {
		t.Errorf("got %q", out)
	}
}

func TestNamespacedStructChainedLvalue(t *testing.T) {
	// `$b.topLeft.x = 99;` walks the lvalue chain into a nested
	// library-provided struct.
	out, err := runWidget(t, `
use io;
use widgets;
def b as widgets.Box init widgets.Box{
    topLeft:     widgets.Point{ x: 0, y: 0 },
    bottomRight: widgets.Point{ x: 10, y: 20 }
};
$b.topLeft.x = 99;
io.printf("%v\n", $b);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := "widgets.Box{topLeft: widgets.Point{x: 99, y: 0}, bottomRight: widgets.Point{x: 10, y: 20}}\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestNamespacedStructValueSemantics(t *testing.T) {
	out, err := runWidget(t, `
use io;
use widgets;
def p as widgets.Point init widgets.Point{ x: 1, y: 2 };
def q as widgets.Point init $p;
$q.x = 99;
io.printf("p=%v q=%v\n", $p, $q);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := "p=widgets.Point{x: 1, y: 2} q=widgets.Point{x: 99, y: 2}\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestNamespacedStructUnknownNameErrors(t *testing.T) {
	_, err := runWidget(t, `
use widgets;
def x as widgets.Widget init widgets.Widget{ a: 1 };
`)
	if err == nil || !strings.Contains(err.Error(), "unknown struct type widgets.Widget") {
		t.Fatalf("got %v", err)
	}
}

func TestNamespacedStructUnknownNamespaceErrors(t *testing.T) {
	_, err := runWidget(t, `
def x as gadgets.Thing;
`)
	if err == nil || !strings.Contains(err.Error(), "unknown namespace") {
		t.Fatalf("got %v", err)
	}
}

func TestNamespacedStructWithoutUseErrors(t *testing.T) {
	_, err := runWidget(t, `
def x as widgets.Point;
`)
	if err == nil || !strings.Contains(err.Error(), "use widgets") {
		t.Fatalf("got %v", err)
	}
}

func TestNamespacedStructDistinctFromBareSameName(t *testing.T) {
	// A library-provided `widgets.Point` is a *different* type from a
	// user-defined bare `Point`. Cross-assigning should fail.
	_, err := runWidget(t, `
use widgets;
def struct Point { x as int, y as int };
def bare as Point init Point{ x: 1, y: 2 };
def named as widgets.Point init $bare;
`)
	if err == nil || !strings.Contains(err.Error(), "cannot initialize") {
		t.Fatalf("got %v", err)
	}
}

func TestNamespacedStructMissingFieldAtLiteral(t *testing.T) {
	_, err := runWidget(t, `
use widgets;
def p as widgets.Point init widgets.Point{ x: 1 };
`)
	if err == nil || !strings.Contains(err.Error(), `missing field "y"`) {
		t.Fatalf("got %v", err)
	}
}

func TestNamespacedStructUnknownFieldAtLiteral(t *testing.T) {
	_, err := runWidget(t, `
use widgets;
def p as widgets.Point init widgets.Point{ x: 1, y: 2, z: 3 };
`)
	if err == nil || !strings.Contains(err.Error(), `unknown field "z"`) {
		t.Fatalf("got %v", err)
	}
}
