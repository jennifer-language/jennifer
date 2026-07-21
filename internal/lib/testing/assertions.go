// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package testinglib

import (
	"fmt"
	"strings"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// The assertion vocabulary. Each reduces to Value.Equal / Kind dispatch at the
// Go level (native speed - no per-call interpreter overhead) and, on failure,
// throws the canonical Error{kind: "assertion"} via interpreter.RaiseError,
// anchored at the assertion's call site (from BuiltinCtx). testing.run catches
// and classifies it exactly like a user `throw Error{kind: "assertion", ...}`.

// assertFail builds the assertion-failure error at the call site.
func assertFail(ctx interpreter.BuiltinCtx, msg string) error {
	return interpreter.RaiseError("assertion", msg, ctx.File, ctx.Line, ctx.Col)
}

// assertEqualFn: deep structural equality via Value.Equal.
func assertEqualFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("testing.assertEqual expects 2 arguments (actual, expected), got %d", len(args))
	}
	if !args[0].Equal(args[1]) {
		return interpreter.Null(), assertFail(ctx,
			fmt.Sprintf("assertEqual: %s != %s", args[0].Display(), args[1].Display()))
	}
	return interpreter.Null(), nil
}

// assertNotEqualFn: the negation.
func assertNotEqualFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("testing.assertNotEqual expects 2 arguments (actual, expected), got %d", len(args))
	}
	if args[0].Equal(args[1]) {
		return interpreter.Null(), assertFail(ctx,
			fmt.Sprintf("assertNotEqual: both are %s", args[0].Display()))
	}
	return interpreter.Null(), nil
}

// assertTrueFn requires a bool and throws on false.
func assertTrueFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	b, err := boolArg("testing.assertTrue", args)
	if err != nil {
		return interpreter.Null(), err
	}
	if !b {
		return interpreter.Null(), assertFail(ctx, "assertTrue: condition is false")
	}
	return interpreter.Null(), nil
}

// assertFalseFn requires a bool and throws on true.
func assertFalseFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	b, err := boolArg("testing.assertFalse", args)
	if err != nil {
		return interpreter.Null(), err
	}
	if b {
		return interpreter.Null(), assertFail(ctx, "assertFalse: condition is true")
	}
	return interpreter.Null(), nil
}

// boolArg enforces the single-bool-argument shape of assertTrue / assertFalse.
// A non-bool is a usage error (a runtime type mismatch), not an assertion
// failure.
func boolArg(fnName string, args []Value) (bool, error) {
	if len(args) != 1 {
		return false, fmt.Errorf("%s expects 1 argument (condition), got %d", fnName, len(args))
	}
	if args[0].Kind != interpreter.KindBool {
		return false, fmt.Errorf("%s: condition must be bool, got %s", fnName, args[0].Kind)
	}
	return args[0].Bool, nil
}

// assertContainsFn dispatches on the haystack's kind: substring for a string,
// element membership (Value.Equal) for a list, key membership for a map.
func assertContainsFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("testing.assertContains expects 2 arguments (haystack, needle), got %d", len(args))
	}
	haystack, needle := args[0], args[1]
	switch haystack.Kind {
	case interpreter.KindString:
		if needle.Kind != interpreter.KindString {
			return interpreter.Null(), fmt.Errorf("testing.assertContains: needle for a string must be string, got %s", needle.Kind)
		}
		if !strings.Contains(haystack.Str, needle.Str) {
			return interpreter.Null(), assertFail(ctx,
				fmt.Sprintf("assertContains: %q does not contain %q", haystack.Str, needle.Str))
		}
	case interpreter.KindList:
		for _, el := range haystack.List {
			if el.Equal(needle) {
				return interpreter.Null(), nil
			}
		}
		return interpreter.Null(), assertFail(ctx,
			fmt.Sprintf("assertContains: list does not contain %s", needle.Display()))
	case interpreter.KindMap:
		for _, e := range haystack.Map {
			if e.Key.Equal(needle) {
				return interpreter.Null(), nil
			}
		}
		return interpreter.Null(), assertFail(ctx,
			fmt.Sprintf("assertContains: map has no key %s", needle.Display()))
	default:
		return interpreter.Null(), fmt.Errorf("testing.assertContains: haystack must be string, list, or map, got %s", haystack.Kind)
	}
	return interpreter.Null(), nil
}

// makeAssertThrowsFn calls the named zero-arg method and asserts it throws an
// Error whose `kind` matches. Wraps testing.run's try/catch shape (the
// CallByName + ClassifyError path) rather than duplicating it.
func makeAssertThrowsFn(in *interpreter.Interpreter) interpreter.Builtin {
	return func(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
		if len(args) != 2 {
			return interpreter.Null(), fmt.Errorf("testing.assertThrows expects 2 arguments (name, kind), got %d", len(args))
		}
		name, err := takeStringArg("testing.assertThrows", args, 0, "name")
		if err != nil {
			return interpreter.Null(), err
		}
		wantKind, err := takeStringArg("testing.assertThrows", args, 1, "kind")
		if err != nil {
			return interpreter.Null(), err
		}
		_, callErr := in.CallByName(name)
		if callErr == nil {
			return interpreter.Null(), assertFail(ctx,
				fmt.Sprintf("assertThrows: %q did not throw", name))
		}
		gotKind, _, _, _, _ := interpreter.ClassifyError(callErr)
		if gotKind != wantKind {
			return interpreter.Null(), assertFail(ctx,
				fmt.Sprintf("assertThrows: %q threw kind %q, expected %q", name, gotKind, wantKind))
		}
		return interpreter.Null(), nil
	}
}
