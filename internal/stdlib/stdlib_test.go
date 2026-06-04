// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package stdlib

import (
	"bytes"
	"testing"

	"github.com/mplx/jennifer-lang/internal/interpreter"
)

func TestInstallRegistersPrintf(t *testing.T) {
	in := interpreter.New()
	Install(in)
	if _, ok := in.Builtins["printf"]; !ok {
		t.Fatal("printf not registered after Install")
	}
}

func TestPrintfWritesInt(t *testing.T) {
	in := interpreter.New()
	Install(in)
	var buf bytes.Buffer
	if _, err := in.Builtins["printf"](&buf, []interpreter.Value{interpreter.IntVal(42)}); err != nil {
		t.Fatalf("err: %v", err)
	}
	if buf.String() != "42" {
		t.Errorf("got %q, want %q", buf.String(), "42")
	}
}

func TestPrintfRejectsBadArity(t *testing.T) {
	in := interpreter.New()
	Install(in)
	var buf bytes.Buffer
	if _, err := in.Builtins["printf"](&buf, nil); err == nil {
		t.Error("expected error for 0 args")
	}
	if _, err := in.Builtins["printf"](&buf, []interpreter.Value{interpreter.IntVal(1), interpreter.IntVal(2)}); err == nil {
		t.Error("expected error for 2 args")
	}
}
