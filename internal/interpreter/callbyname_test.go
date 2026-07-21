// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

func TestCallByNameWith(t *testing.T) {
	prog, err := parser.Parse(`func add(a as int, b as int) { return $a + $b; }`)
	if err != nil {
		t.Fatal(err)
	}
	in := interpreter.New()
	if err := in.Run(prog); err != nil {
		t.Fatal(err)
	}

	t.Run("binds args", func(t *testing.T) {
		v, err := in.CallByNameWith("add", interpreter.IntVal(2), interpreter.IntVal(3))
		if err != nil {
			t.Fatalf("call: %v", err)
		}
		if v.Kind != interpreter.KindInt || v.Int != 5 {
			t.Fatalf("got %+v, want int 5", v)
		}
	})
	t.Run("arity mismatch errors", func(t *testing.T) {
		if _, err := in.CallByNameWith("add", interpreter.IntVal(1)); err == nil {
			t.Error("expected an arity error")
		}
	})
	t.Run("type mismatch errors", func(t *testing.T) {
		if _, err := in.CallByNameWith("add", interpreter.IntVal(1), interpreter.StringVal("x")); err == nil {
			t.Error("expected a declared-type error")
		}
	})
	t.Run("unknown method errors", func(t *testing.T) {
		if _, err := in.CallByNameWith("nope"); err == nil {
			t.Error("expected an unknown-method error")
		}
	})
}

func TestRaiseErrorIsCatchable(t *testing.T) {
	// A canonical Error built by RaiseError classifies by its kind.
	err := interpreter.RaiseError("assertion", "boom", "f.j", 3, 5)
	kind, msg, file, line, col := interpreter.ClassifyError(err)
	if kind != "assertion" || msg != "boom" || file != "f.j" || line != 3 || col != 5 {
		t.Fatalf("ClassifyError = %q %q %q %d %d", kind, msg, file, line, col)
	}
}
