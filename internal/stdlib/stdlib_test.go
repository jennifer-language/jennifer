// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package stdlib

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mplx/jennifer-lang/internal/interpreter"
)

func TestInstallRegistersBuiltins(t *testing.T) {
	in := interpreter.New()
	Install(in)
	if _, ok := in.Builtins["printf"]; !ok {
		t.Fatal("printf not registered after Install")
	}
	if _, ok := in.Builtins["sprintf"]; !ok {
		t.Fatal("sprintf not registered after Install")
	}
}

func TestPrintfSingleArgDisplay(t *testing.T) {
	in := interpreter.New()
	Install(in)
	cases := []struct {
		v    interpreter.Value
		want string
	}{
		{interpreter.IntVal(42), "42"},
		{interpreter.StringVal("hi"), "hi"},
		{interpreter.BoolVal(true), "true"},
		{interpreter.FloatVal(3.5), "3.5"},
		{interpreter.Null(), "null"},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		if _, err := in.Builtins["printf"](&buf, []interpreter.Value{c.v}); err != nil {
			t.Errorf("printf(%v): %v", c.v, err)
			continue
		}
		if buf.String() != c.want {
			t.Errorf("printf(%v): got %q, want %q", c.v, buf.String(), c.want)
		}
	}
}

func TestPrintfFormatString(t *testing.T) {
	in := interpreter.New()
	Install(in)
	cases := []struct {
		args []interpreter.Value
		want string
	}{
		{[]interpreter.Value{interpreter.StringVal("%d items"), interpreter.IntVal(3)}, "3 items"},
		{[]interpreter.Value{interpreter.StringVal("pi is %f"), interpreter.FloatVal(3.14)}, "pi is 3.14"},
		{[]interpreter.Value{interpreter.StringVal("%s = %d"), interpreter.StringVal("answer"), interpreter.IntVal(42)}, "answer = 42"},
		{[]interpreter.Value{interpreter.StringVal("flag=%t"), interpreter.BoolVal(true)}, "flag=true"},
		{[]interpreter.Value{interpreter.StringVal("any=%v"), interpreter.IntVal(7)}, "any=7"},
		{[]interpreter.Value{interpreter.StringVal("50%% done")}, "50% done"},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		if _, err := in.Builtins["printf"](&buf, c.args); err != nil {
			t.Errorf("printf(%v): %v", c.args, err)
			continue
		}
		if buf.String() != c.want {
			t.Errorf("printf(%v): got %q, want %q", c.args, buf.String(), c.want)
		}
	}
}

func TestPrintfFormatErrors(t *testing.T) {
	in := interpreter.New()
	Install(in)
	cases := []struct {
		args []interpreter.Value
		want string // substring of error
	}{
		{nil, "at least 1 argument"},
		{[]interpreter.Value{interpreter.IntVal(1), interpreter.IntVal(2)}, "first argument must be a string"},
		{[]interpreter.Value{interpreter.StringVal("%d"), interpreter.StringVal("nope")}, "`%d` requires int"},
		{[]interpreter.Value{interpreter.StringVal("%s"), interpreter.IntVal(1)}, "`%s` requires string"},
		{[]interpreter.Value{interpreter.StringVal("%d")}, "not enough arguments"},
		{[]interpreter.Value{interpreter.StringVal("hi"), interpreter.IntVal(1)}, "too many arguments"},
		{[]interpreter.Value{interpreter.StringVal("%")}, "dangling"},
		{[]interpreter.Value{interpreter.StringVal("%q"), interpreter.IntVal(1)}, "unknown format verb"},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		_, err := in.Builtins["printf"](&buf, c.args)
		if err == nil {
			t.Errorf("printf(%v): expected error, got nil", c.args)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("printf(%v): error %q does not contain %q", c.args, err.Error(), c.want)
		}
	}
}

func TestSprintfReturnsString(t *testing.T) {
	in := interpreter.New()
	Install(in)
	v, err := in.Builtins["sprintf"](nil, []interpreter.Value{interpreter.StringVal("%d+%d=%d"), interpreter.IntVal(1), interpreter.IntVal(2), interpreter.IntVal(3)})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if v.Kind != interpreter.KindString || v.Str != "1+2=3" {
		t.Errorf("got %s(%q), want String(%q)", v.Kind, v.Str, "1+2=3")
	}
}
