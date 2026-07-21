// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package iolib

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// callPrintf and callSprintf invoke the registered namespaced builtins.
// Every io builtin lives under the `io.` namespace, so we
// go through the interpreter's namespaced lookup helper rather than
// the flat Builtins map.
func callPrintf(in *interpreter.Interpreter, out io.Writer, args []interpreter.Value) (interpreter.Value, error) {
	return in.LookupNamespacedBuiltin("io", "printf")(interpreter.BuiltinCtx{Out: out}, args)
}

func callSprintf(in *interpreter.Interpreter, args []interpreter.Value) (interpreter.Value, error) {
	return in.LookupNamespacedBuiltin("io", "sprintf")(interpreter.BuiltinCtx{}, args)
}

func callEprintf(in *interpreter.Interpreter, errW io.Writer, args []interpreter.Value) (interpreter.Value, error) {
	return in.LookupNamespacedBuiltin("io", "eprintf")(interpreter.BuiltinCtx{Err: errW}, args)
}

func TestInstallRegistersBuiltins(t *testing.T) {
	in := interpreter.New()
	Install(in)
	for _, name := range []string{"printf", "eprintf", "sprintf"} {
		if in.LookupNamespacedBuiltin("io", name) == nil {
			t.Fatalf("io.%s not registered after Install", name)
		}
	}
}

// TestEprintfWritesToErr confirms eprintf formats like printf but targets the
// error stream (ctx.Err), not stdout (ctx.Out).
func TestEprintfWritesToErr(t *testing.T) {
	in := interpreter.New()
	Install(in)
	var errBuf bytes.Buffer
	if _, err := callEprintf(in, &errBuf, []interpreter.Value{
		interpreter.StringVal("n=%d\n"), interpreter.IntVal(7),
	}); err != nil {
		t.Fatalf("io.eprintf: %v", err)
	}
	if errBuf.String() != "n=7\n" {
		t.Errorf("io.eprintf wrote %q, want %q", errBuf.String(), "n=7\n")
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
		if _, err := callPrintf(in, &buf, []interpreter.Value{c.v}); err != nil {
			t.Errorf("io.printf(%v): %v", c.v, err)
			continue
		}
		if buf.String() != c.want {
			t.Errorf("io.printf(%v): got %q, want %q", c.v, buf.String(), c.want)
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
		if _, err := callPrintf(in, &buf, c.args); err != nil {
			t.Errorf("io.printf(%v): %v", c.args, err)
			continue
		}
		if buf.String() != c.want {
			t.Errorf("io.printf(%v): got %q, want %q", c.args, buf.String(), c.want)
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
		_, err := callPrintf(in, &buf, c.args)
		if err == nil {
			t.Errorf("io.printf(%v): expected error, got nil", c.args)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("io.printf(%v): error %q does not contain %q", c.args, err.Error(), c.want)
		}
	}
}

func TestSprintfReturnsString(t *testing.T) {
	in := interpreter.New()
	Install(in)
	v, err := callSprintf(in, []interpreter.Value{interpreter.StringVal("%d+%d=%d"), interpreter.IntVal(1), interpreter.IntVal(2), interpreter.IntVal(3)})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if v.Kind != interpreter.KindString || v.Str != "1+2=3" {
		t.Errorf("got %s(%q), want String(%q)", v.Kind, v.Str, "1+2=3")
	}
}

// vals is a tiny constructor helper to keep the modifier tables readable.
// Returns a []interpreter.Value built from a format string + variadic args.
func vals(format string, args ...interpreter.Value) []interpreter.Value {
	return append([]interpreter.Value{interpreter.StringVal(format)}, args...)
}

func TestPrintfStringModifiers(t *testing.T) {
	in := interpreter.New()
	Install(in)
	cases := []struct {
		name string
		args []interpreter.Value
		want string
	}{
		{"pad left default", vals("[%s|pad=5]", interpreter.StringVal("ab")), "[ab   ]"},
		{"pad right", vals("[%s|pad=5|align=right]", interpreter.StringVal("ab")), "[   ab]"},
		{"pad shorter than value is a no-op", vals("[%s|pad=2]", interpreter.StringVal("abcd")), "[abcd]"},
		{"max truncates", vals("[%s|max=3]", interpreter.StringVal("abcdef")), "[abc]"},
		{"max then pad", vals("[%s|max=3|pad=6|align=right]", interpreter.StringVal("abcdef")), "[   abc]"},
		{"max=0 yields empty", vals("[%s|max=0]", interpreter.StringVal("anything")), "[]"},
		{"mode=quote wraps and escapes", vals("%s|mode=quote", interpreter.StringVal("a\nb\"c")), `"a\nb\"c"`},
		{"mode=escape shows escapes without quoting", vals("%s|mode=escape", interpreter.StringVal("a\tb")), `a\tb`},
		{"rune-aware pad", vals("[%s|pad=4]", interpreter.StringVal("héllo")), "[héllo]"},
		{"rune-aware truncate", vals("%s|max=3", interpreter.StringVal("héllo")), "hél"},
		{"|| escapes a literal pipe after verb", vals("%s||fill", interpreter.StringVal("hi")), "hi|fill"},
		{"|| after modifiers", vals("%s|pad=4||fill", interpreter.StringVal("hi")), "hi  |fill"},
		{"|| at end of format", vals("%s||", interpreter.StringVal("hi")), "hi|"},
		{"docs example: literal | only escaped when touching verb", vals("a|b %s||c|d\n", interpreter.StringVal("X")), "a|b X|c|d\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			if _, err := callPrintf(in, &buf, c.args); err != nil {
				t.Fatalf("err: %v", err)
			}
			if buf.String() != c.want {
				t.Errorf("got %q, want %q", buf.String(), c.want)
			}
		})
	}
}

func TestPrintfIntModifiers(t *testing.T) {
	in := interpreter.New()
	Install(in)
	cases := []struct {
		name string
		args []interpreter.Value
		want string
	}{
		{"base=2", vals("%d|base=2", interpreter.IntVal(5)), "101"},
		{"base=8", vals("%d|base=8", interpreter.IntVal(8)), "10"},
		{"base=16", vals("%d|base=16", interpreter.IntVal(255)), "ff"},
		{"base=16 negative", vals("%d|base=16", interpreter.IntVal(-255)), "-ff"},
		{"sign=always positive", vals("%d|sign=always", interpreter.IntVal(7)), "+7"},
		{"sign=always zero", vals("%d|sign=always", interpreter.IntVal(0)), "+0"},
		{"sign=space positive", vals("[%d|sign=space]", interpreter.IntVal(7)), "[ 7]"},
		{"sign=space negative still has -", vals("[%d|sign=space]", interpreter.IntVal(-3)), "[-3]"},
		{"sign=always plus base=16", vals("%d|base=16|sign=always", interpreter.IntVal(255)), "+ff"},
		{"group + sep", vals("%d|group=3|sep=,", interpreter.IntVal(1234567)), "1,234,567"},
		{"group + sep underscore", vals("%d|base=16|group=4|sep=_", interpreter.IntVal(3735928559)), "dead_beef"},
		{"pad right (default)", vals("[%d|pad=5]", interpreter.IntVal(42)), "[   42]"},
		{"pad left", vals("[%d|pad=5|align=left]", interpreter.IntVal(42)), "[42   ]"},
		{"fill=0", vals("%d|pad=5|fill=0", interpreter.IntVal(42)), "00042"},
		{"fill=0 with negative", vals("%d|pad=5|fill=0", interpreter.IntVal(-42)), "-0042"},
		{"fill=0 with sign=always", vals("%d|pad=5|fill=0|sign=always", interpreter.IntVal(42)), "+0042"},
		{"fill=0 with group is columnar", vals("%d|pad=10|fill=0|group=3|sep=,", interpreter.IntVal(1234)), "00,001,234"},
		{"fill=0 with group negative", vals("%d|pad=10|fill=0|group=3|sep=,", interpreter.IntVal(-1234)), "-0,001,234"},
		{"pad short of value is no-op", vals("[%d|pad=2]", interpreter.IntVal(12345)), "[12345]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			if _, err := callPrintf(in, &buf, c.args); err != nil {
				t.Fatalf("err: %v", err)
			}
			if buf.String() != c.want {
				t.Errorf("got %q, want %q", buf.String(), c.want)
			}
		})
	}
}

func TestPrintfFloatModifiers(t *testing.T) {
	in := interpreter.New()
	Install(in)
	cases := []struct {
		name string
		args []interpreter.Value
		want string
	}{
		{"prec=2", vals("%f|prec=2", interpreter.FloatVal(3.14159)), "3.14"},
		{"prec=0", vals("%f|prec=0", interpreter.FloatVal(3.7)), "4"},
		{"trim strips trailing zeros", vals("%f|prec=4|trim=true", interpreter.FloatVal(3.14)), "3.14"},
		{"trim strips trailing zeros and dot", vals("%f|prec=4|trim=true", interpreter.FloatVal(3.0)), "3"},
		{"sci=true", vals("%f|sci=true|prec=4", interpreter.FloatVal(1234.5)), "1.2345e+03"},
		{"sci=true tiny", vals("%f|sci=true|prec=2", interpreter.FloatVal(0.00123)), "1.23e-03"},
		{"sci=true trim", vals("%f|sci=true|prec=4|trim=true", interpreter.FloatVal(1234.5)), "1.2345e+03"},
		{"sign=always positive", vals("%f|prec=1|sign=always", interpreter.FloatVal(3.14)), "+3.1"},
		{"sign=space positive", vals("[%f|prec=1|sign=space]", interpreter.FloatVal(3.14)), "[ 3.1]"},
		{"pad right", vals("[%f|prec=2|pad=8]", interpreter.FloatVal(3.14)), "[    3.14]"},
		{"pad left", vals("[%f|prec=2|pad=8|align=left]", interpreter.FloatVal(3.14)), "[3.14    ]"},
		{"negative", vals("%f|prec=2", interpreter.FloatVal(-3.14)), "-3.14"},
		{"no prec uses default display form", vals("%f", interpreter.FloatVal(3.5)), "3.5"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			if _, err := callPrintf(in, &buf, c.args); err != nil {
				t.Fatalf("err: %v", err)
			}
			if buf.String() != c.want {
				t.Errorf("got %q, want %q", buf.String(), c.want)
			}
		})
	}
}

func TestPrintfBoolModifiers(t *testing.T) {
	in := interpreter.New()
	Install(in)
	cases := []struct {
		name string
		args []interpreter.Value
		want string
	}{
		{"default lower", vals("%t", interpreter.BoolVal(true)), "true"},
		{"case=upper", vals("%t|case=upper", interpreter.BoolVal(true)), "TRUE"},
		{"case=upper false", vals("%t|case=upper", interpreter.BoolVal(false)), "FALSE"},
		{"case=title", vals("%t|case=title", interpreter.BoolVal(true)), "True"},
		{"case=lower (default explicit)", vals("%t|case=lower", interpreter.BoolVal(false)), "false"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			if _, err := callPrintf(in, &buf, c.args); err != nil {
				t.Fatalf("err: %v", err)
			}
			if buf.String() != c.want {
				t.Errorf("got %q, want %q", buf.String(), c.want)
			}
		})
	}
}

func TestPrintfNullModifiers(t *testing.T) {
	in := interpreter.New()
	Install(in)
	cases := []struct {
		name string
		args []interpreter.Value
		want string
	}{
		{"null=empty %s", vals("[%s|null=empty]", interpreter.Null()), "[]"},
		{"null=null %d", vals("%d|null=null", interpreter.Null()), "null"},
		{"null=literal %f", vals(`%f|null=literal("N/A")`, interpreter.Null()), "N/A"},
		{"null=literal with escapes", vals(`%s|null=literal("\t\n")`, interpreter.Null()), "\t\n"},
		{"null wins over mode=quote", vals(`%s|mode=quote|null=literal("X")`, interpreter.Null()), "X"},
		{"layout still applies to null literal", vals(`[%s|null=literal("?")|pad=5|align=right]`, interpreter.Null()), "[    ?]"},
		{"non-null value uses verb render with null= set", vals(`%s|null=null`, interpreter.StringVal("hi")), "hi"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			if _, err := callPrintf(in, &buf, c.args); err != nil {
				t.Fatalf("err: %v", err)
			}
			if buf.String() != c.want {
				t.Errorf("got %q, want %q", buf.String(), c.want)
			}
		})
	}
}

func TestPrintfModifierErrors(t *testing.T) {
	in := interpreter.New()
	Install(in)
	cases := []struct {
		name string
		args []interpreter.Value
		want string // substring
	}{
		{"unknown key on %s", vals("%s|base=2", interpreter.StringVal("hi")), `not valid for verb`},
		{"unknown key on %d", vals("%d|case=upper", interpreter.IntVal(1)), `not valid for verb`},
		{"unknown key on %t", vals("%t|pad=4", interpreter.BoolVal(true)), `not valid for verb`},
		{"%v takes no modifiers", vals("%v|pad=4", interpreter.IntVal(1)), "no modifiers"},
		{"bad base value", vals("%d|base=3", interpreter.IntVal(1)), `base="3"`},
		{"bad align value", vals("%s|align=middle", interpreter.StringVal("x")), `align="middle"`},
		{"bad sign value", vals("%d|sign=plus", interpreter.IntVal(1)), `sign="plus"`},
		{"bad case value", vals("%t|case=snake", interpreter.BoolVal(true)), `case="snake"`},
		{"group without sep", vals("%d|group=3", interpreter.IntVal(1000)), "must be specified together"},
		{"sep without group", vals("%d|sep=,", interpreter.IntVal(1000)), "must be specified together"},
		{"duplicate key", vals("%d|pad=4|pad=8", interpreter.IntVal(1)), "specified twice"},
		{"bad fill", vals("%d|fill=x", interpreter.IntVal(1)), `fill="x"`},
		{"bad null mode", vals("%s|null=foo", interpreter.Null()), `null="foo"`},
		{"null=literal missing quote", vals("%s|null=literal(x)", interpreter.Null()), "needs a double-quoted string"},
		{"null=literal unclosed", vals(`%s|null=literal("abc`, interpreter.Null()), "missing closing"},
		{"missing equals", vals("%s|pad", interpreter.StringVal("x")), "missing `=`"},
		{"empty value", vals("%s|pad=", interpreter.StringVal("x")), "empty value"},
		{"non-numeric pad", vals("%s|pad=abc", interpreter.StringVal("x")), "not an integer"},
		{"non-null type still mismatches", vals("%d", interpreter.StringVal("x")), "requires int"},
		{"null still mismatches without null=", vals("%d", interpreter.Null()), "requires int"},
		{"fill=0 with align=left is rejected", vals("%d|pad=5|fill=0|align=left", interpreter.IntVal(1)), "fill=0"},
		// additions
		{"align=center on %d is rejected", vals("%d|pad=5|align=center", interpreter.IntVal(1)), "only valid on %s"},
		{"align=center on %f is rejected", vals("%f|pad=5|align=center", interpreter.FloatVal(1.0)), "only valid on %s"},
		{"null=skip on %s is rejected", vals("%s|null=skip", interpreter.Null()), "only valid on %a"},
		{"unknown %a modifier", vals("%a|junk=true", listOfInts(1, 2)), `unknown modifier "junk"`},
		{"%a on a non-aggregate is rejected", vals("%a", interpreter.IntVal(42)), "requires list or map"},
		{"unterminated quoted value", vals(`%a|sep="abc`, listOfInts(1, 2)), "unterminated quoted modifier value"},
		{"unknown escape in quoted value", vals(`%a|sep="\q"`, listOfInts(1, 2)), "unknown escape"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			_, err := callPrintf(in, &buf, c.args)
			if err == nil {
				t.Fatalf("expected error, got nil; output=%q", buf.String())
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not contain %q", err.Error(), c.want)
			}
		})
	}
}

// listOfInts returns a Jennifer list value containing the given ints,
// used by tests that need an aggregate input for `%a`.
func listOfInts(xs ...int64) interpreter.Value {
	list := make([]interpreter.Value, 0, len(xs))
	for _, x := range xs {
		list = append(list, interpreter.IntVal(x))
	}
	v := interpreter.Value{Kind: interpreter.KindList, List: list}
	return v
}

// TestAlignCenterAndAggregate exercises the printf additions:
// `%s|align=center` and the `%a` aggregate verb with its modifiers.
func TestAlignCenterAndAggregate(t *testing.T) {
	in := interpreter.New()
	Install(in)
	cases := []struct {
		name string
		args []interpreter.Value
		want string
	}{
		// align=center distributes padding evenly; odd leftover goes
		// to the right.
		{"center even width", vals("%s|pad=6|align=center", interpreter.StringVal("hi")), "  hi  "},
		{"center odd leftover", vals("%s|pad=7|align=center", interpreter.StringVal("hi")), "  hi   "},
		{"center on already-wide", vals("%s|pad=3|align=center", interpreter.StringVal("hello")), "hello"},

		// %a list defaults match Jennifer's literal syntax.
		{"list default", vals("%a", listOfInts(1, 2, 3)), "[1, 2, 3]"},
		{"list custom sep", vals(`%a|sep=" | "`, listOfInts(1, 2, 3)), "[1 | 2 | 3]"},
		{"list custom brackets", vals(`%a|open="<"|close=">"`, listOfInts(1, 2, 3)), "<1, 2, 3>"},

		// Depth collapses nested aggregates to "[...]" / "{...}".
		{"depth=0 collapses top", vals("%a|depth=0", listOfInts(1, 2)), "[...]"},

		// Empty list still rounds-trips.
		{"empty list", vals("%a", listOfInts()), "[]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			if _, err := callPrintf(in, &buf, c.args); err != nil {
				t.Fatalf("err: %v", err)
			}
			if buf.String() != c.want {
				t.Errorf("got %q, want %q", buf.String(), c.want)
			}
		})
	}
}
