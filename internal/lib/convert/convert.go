// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

// Package convert implements Jennifer's `convert` library: explicit value
// conversions between the primitive kinds, plus `typeof` for runtime kind
// introspection.
package convert

import (
	"fmt"
	"io"
	"strconv"

	"github.com/mplx/jennifer-lang/internal/interpreter"
)

// LibraryName is the Jennifer name programs `use` to enable these functions.
const LibraryName = "convert"

// Install registers convert library functions on an interpreter.
func Install(in *interpreter.Interpreter) {
	in.Register(LibraryName, "int", intFn)
	in.Register(LibraryName, "float", floatFn)
	in.Register(LibraryName, "string", stringFn)
	in.Register(LibraryName, "bool", boolFn)
	in.Register(LibraryName, "typeof", typeofFn)
}

// arityOne returns an error if args doesn't contain exactly one value.
func arityOne(name string, args []interpreter.Value) error {
	if len(args) != 1 {
		return fmt.Errorf("%s expects 1 argument, got %d", name, len(args))
	}
	return nil
}

// intFn implements `int(v)`:
//   - int    -> identity
//   - float  -> truncate toward zero (Go's int64 cast)
//   - string -> strconv.ParseInt(base 10, 64-bit); error on bad input
//   - bool   -> true=1, false=0
//   - null   -> error
func intFn(_ io.Writer, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityOne("int", args); err != nil {
		return interpreter.Null(), err
	}
	v := args[0]
	switch v.Kind {
	case interpreter.KindInt:
		return v, nil
	case interpreter.KindFloat:
		return interpreter.IntVal(int64(v.Float)), nil
	case interpreter.KindString:
		n, err := strconv.ParseInt(v.Str, 10, 64)
		if err != nil {
			return interpreter.Null(), fmt.Errorf("int(%q): not a valid integer", v.Str)
		}
		return interpreter.IntVal(n), nil
	case interpreter.KindBool:
		if v.Bool {
			return interpreter.IntVal(1), nil
		}
		return interpreter.IntVal(0), nil
	}
	return interpreter.Null(), fmt.Errorf("int(): cannot convert %s to int", v.Kind)
}

// floatFn implements `float(v)`:
//   - int    -> convert
//   - float  -> identity
//   - string -> strconv.ParseFloat(64-bit); error on bad input
//   - bool   -> true=1.0, false=0.0
//   - null   -> error
func floatFn(_ io.Writer, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityOne("float", args); err != nil {
		return interpreter.Null(), err
	}
	v := args[0]
	switch v.Kind {
	case interpreter.KindInt:
		return interpreter.FloatVal(float64(v.Int)), nil
	case interpreter.KindFloat:
		return v, nil
	case interpreter.KindString:
		f, err := strconv.ParseFloat(v.Str, 64)
		if err != nil {
			return interpreter.Null(), fmt.Errorf("float(%q): not a valid float", v.Str)
		}
		return interpreter.FloatVal(f), nil
	case interpreter.KindBool:
		if v.Bool {
			return interpreter.FloatVal(1.0), nil
		}
		return interpreter.FloatVal(0.0), nil
	}
	return interpreter.Null(), fmt.Errorf("float(): cannot convert %s to float", v.Kind)
}

// stringFn implements `string(v)`: returns the value's display form. Never
// fails (every kind has a defined Display).
func stringFn(_ io.Writer, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityOne("string", args); err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(args[0].Display()), nil
}

// boolFn implements `bool(v)` with strict canonical-only conversions:
//
//   - bool   -> identity
//   - int    -> 0 = false, 1 = true; any other int errors
//   - float  -> 0.0 = false, 1.0 = true; any other float errors
//   - string -> "true" = true, "false" = false; any other string errors
//   - null   -> always errors
//
// If you want "nonzero counts as true" semantics, write the comparison
// explicitly: `def b as bool init $x != 0;`.
func boolFn(_ io.Writer, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityOne("bool", args); err != nil {
		return interpreter.Null(), err
	}
	v := args[0]
	switch v.Kind {
	case interpreter.KindBool:
		return v, nil
	case interpreter.KindInt:
		switch v.Int {
		case 0:
			return interpreter.BoolVal(false), nil
		case 1:
			return interpreter.BoolVal(true), nil
		}
		return interpreter.Null(), fmt.Errorf("bool(%d): only 0 and 1 are accepted (use `$x != 0` for truthiness)", v.Int)
	case interpreter.KindFloat:
		switch v.Float {
		case 0:
			return interpreter.BoolVal(false), nil
		case 1:
			return interpreter.BoolVal(true), nil
		}
		return interpreter.Null(), fmt.Errorf("bool(%s): only 0.0 and 1.0 are accepted (use `$x != 0.0` for truthiness)", interpreter.DisplayFloat(v.Float))
	case interpreter.KindString:
		switch v.Str {
		case "true":
			return interpreter.BoolVal(true), nil
		case "false":
			return interpreter.BoolVal(false), nil
		}
		return interpreter.Null(), fmt.Errorf("bool(%q): only \"true\" or \"false\" are accepted", v.Str)
	}
	return interpreter.Null(), fmt.Errorf("bool(): cannot convert %s to bool", v.Kind)
}

// typeofFn returns the runtime kind name of its argument as a string:
// "null", "int", "float", "string", or "bool". Useful for debugging and
// runtime introspection.
func typeofFn(_ io.Writer, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityOne("typeof", args); err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(args[0].Kind.String()), nil
}
