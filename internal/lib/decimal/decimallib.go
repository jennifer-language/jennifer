// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// Package decimallib is the `decimal` library: exact base-10 arithmetic over an
// opaque `decimal.Decimal` (a math/big coefficient and a scale), for money and
// anything else binary floats round wrongly. add / sub / mul are exact; div
// either is exact or rounds to a scale the caller names; round rounds by a named
// mode (half-even by default). `==` compares values (1.0 == 1.00) while the
// scale is kept for display. Go standard library only, TinyGo-clean.
package decimallib

import (
	"errors"
	"fmt"
	"math/big"
	"strconv"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// LibraryName is the namespace prefix (`decimal.`) and the `use` name.
const LibraryName = "decimal"

type Value = interpreter.Value

// decValue is the native payload of a decimal.Decimal.
type decValue struct{ d dec }

func (v *decValue) EqualNative(other interpreter.NativeObject) bool {
	o, ok := other.(*decValue)
	return ok && cmp(v.d, o.d) == 0
}

func (v *decValue) DisplayNative() string { return v.d.String() }

func wrap(d dec) Value {
	return interpreter.NativeObjectVal(LibraryName, "Decimal", &decValue{d: d})
}

// Install registers the decimal surface.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespacedObject(LibraryName, "Decimal", nil)
	// An uninitialised `def d as decimal.Decimal;` is 0.
	in.SetNativeObjectZero(LibraryName, "Decimal", &decValue{d: fromInt(0)})

	reg := func(name string, fn interpreter.Builtin) { in.RegisterNamespaced(LibraryName, name, fn) }
	reg("from", fromFn)
	reg("add", binary("add", add))
	reg("sub", binary("sub", sub))
	reg("mul", binary("mul", mul))
	reg("div", divFn)
	reg("neg", unary("neg", func(d dec) (dec, error) { return dec{coef: new(big.Int).Neg(d.coef), scale: d.scale}, nil }))
	reg("abs", unary("abs", func(d dec) (dec, error) { return dec{coef: new(big.Int).Abs(d.coef), scale: d.scale}, nil }))
	reg("normalize", unary("normalize", func(d dec) (dec, error) { return normalize(d), nil }))
	reg("round", roundFn)
	reg("pow", powFn)
	reg("sum", sumFn)
	reg("compare", compareFn)
	reg("sign", info("sign", func(d dec) Value { return interpreter.IntVal(int64(d.coef.Sign())) }))
	reg("scale", info("scale", func(d dec) Value { return interpreter.IntVal(int64(d.scale)) }))
	reg("isInteger", info("isInteger", func(d dec) Value { return interpreter.BoolVal(isInteger(d)) }))
	reg("toString", info("toString", func(d dec) Value { return interpreter.StringVal(d.String()) }))
	reg("toInt", toIntFn)
	reg("toFloat", toFloatFn)
}

// --- errors -----------------------------------------------------------------

// raise turns an arithmetic failure into the catchable error a caller sees:
// kind "limit" for a result past maxDigits, kind "decimal" for the rest.
func raise(ctx interpreter.BuiltinCtx, fn string, err error) error {
	kind, msg := "decimal", err.Error()
	switch {
	case errors.Is(err, errLimit):
		kind, msg = "limit", fmt.Sprintf("the result would have more than %d digits", maxDigits)
	case errors.Is(err, errInexact):
		msg = "the quotient has no finite decimal form; pass a scale (and a rounding mode)"
	}
	return interpreter.RaiseError(kind, "decimal."+fn+": "+msg, ctx.File, ctx.Line, ctx.Col)
}

// --- arguments --------------------------------------------------------------

func describe(v Value) string {
	if v.Kind == interpreter.KindObject || v.Kind == interpreter.KindStruct {
		return v.StructNS + "." + v.StructName
	}
	return v.Kind.String()
}

// operand reads a Decimal, an int, or a decimal string. A float is not taken
// here: decimal.from makes that conversion explicit.
func operand(ctx interpreter.BuiltinCtx, fn string, v Value) (dec, error) {
	if n, ok := v.AsNativeObject(LibraryName, "Decimal"); ok {
		return n.(*decValue).d, nil
	}
	switch v.Kind {
	case interpreter.KindInt:
		return fromInt(v.Int), nil
	case interpreter.KindString:
		d, err := parse(v.Str)
		if err != nil {
			return dec{}, raise(ctx, fn, err)
		}
		return d, nil
	case interpreter.KindFloat:
		return dec{}, fmt.Errorf("decimal.%s: a float operand must go through decimal.from first", fn)
	}
	return dec{}, fmt.Errorf("decimal.%s: operand must be a decimal.Decimal, int or string, got %s", fn, describe(v))
}

func modeArg(fn string, v Value) (mode, error) {
	if v.Kind != interpreter.KindString {
		return 0, fmt.Errorf("decimal.%s: rounding mode must be a string, got %s", fn, describe(v))
	}
	m, ok := modeNames[v.Str]
	if !ok {
		return 0, fmt.Errorf("decimal.%s: unknown rounding mode %q; known: %s", fn, v.Str, modeList)
	}
	return m, nil
}

func scaleArg(fn string, v Value) (int, error) {
	if v.Kind != interpreter.KindInt {
		return 0, fmt.Errorf("decimal.%s: scale must be int, got %s", fn, describe(v))
	}
	if v.Int < 0 || v.Int > maxDigits {
		return 0, fmt.Errorf("decimal.%s: scale must be in 0..%d, got %d", fn, maxDigits, v.Int)
	}
	return int(v.Int), nil
}

// --- builtins ---------------------------------------------------------------

// fromFn implements `decimal.from(x)` for a string, an int or a float.
func fromFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("decimal.from expects 1 argument (string, int or float), got %d", len(args))
	}
	if args[0].Kind == interpreter.KindFloat {
		d, err := fromFloat(args[0].Float)
		if err != nil {
			return interpreter.Null(), raise(ctx, "from", err)
		}
		return wrap(d), nil
	}
	d, err := operand(ctx, "from", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	return wrap(d), nil
}

func binary(fn string, op func(a, b dec) (dec, error)) interpreter.Builtin {
	return func(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
		if len(args) != 2 {
			return interpreter.Null(), fmt.Errorf("decimal.%s expects 2 arguments, got %d", fn, len(args))
		}
		a, err := operand(ctx, fn, args[0])
		if err != nil {
			return interpreter.Null(), err
		}
		b, err := operand(ctx, fn, args[1])
		if err != nil {
			return interpreter.Null(), err
		}
		r, err := op(a, b)
		if err != nil {
			return interpreter.Null(), raise(ctx, fn, err)
		}
		return wrap(r), nil
	}
}

func unary(fn string, op func(d dec) (dec, error)) interpreter.Builtin {
	return func(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
		if len(args) != 1 {
			return interpreter.Null(), fmt.Errorf("decimal.%s expects 1 argument, got %d", fn, len(args))
		}
		d, err := operand(ctx, fn, args[0])
		if err != nil {
			return interpreter.Null(), err
		}
		r, err := op(d)
		if err != nil {
			return interpreter.Null(), raise(ctx, fn, err)
		}
		return wrap(r), nil
	}
}

func info(fn string, get func(d dec) Value) interpreter.Builtin {
	return func(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
		if len(args) != 1 {
			return interpreter.Null(), fmt.Errorf("decimal.%s expects 1 argument, got %d", fn, len(args))
		}
		d, err := operand(ctx, fn, args[0])
		if err != nil {
			return interpreter.Null(), err
		}
		return get(d), nil
	}
}

// divFn implements `decimal.div(a, b[, scale[, mode]])`.
func divFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) < 2 || len(args) > 4 {
		return interpreter.Null(), fmt.Errorf("decimal.div expects 2 to 4 arguments (a, b[, scale[, mode]]), got %d", len(args))
	}
	a, err := operand(ctx, "div", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	b, err := operand(ctx, "div", args[1])
	if err != nil {
		return interpreter.Null(), err
	}
	scale, m := -1, halfEven
	if len(args) >= 3 {
		if scale, err = scaleArg("div", args[2]); err != nil {
			return interpreter.Null(), err
		}
	}
	if len(args) == 4 {
		if m, err = modeArg("div", args[3]); err != nil {
			return interpreter.Null(), err
		}
	}
	r, err := div(a, b, scale, m)
	if err != nil {
		return interpreter.Null(), raise(ctx, "div", err)
	}
	return wrap(r), nil
}

// roundFn implements `decimal.round(d, scale[, mode])`.
func roundFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 && len(args) != 3 {
		return interpreter.Null(), fmt.Errorf("decimal.round expects 2 or 3 arguments (d, scale[, mode]), got %d", len(args))
	}
	d, err := operand(ctx, "round", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	scale, err := scaleArg("round", args[1])
	if err != nil {
		return interpreter.Null(), err
	}
	m := halfEven
	if len(args) == 3 {
		if m, err = modeArg("round", args[2]); err != nil {
			return interpreter.Null(), err
		}
	}
	r, err := roundTo(d, scale, m)
	if err != nil {
		return interpreter.Null(), raise(ctx, "round", err)
	}
	return wrap(r), nil
}

// powFn implements `decimal.pow(d, n)` for an int n >= 0.
func powFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("decimal.pow expects 2 arguments (d, n), got %d", len(args))
	}
	d, err := operand(ctx, "pow", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindInt || args[1].Int < 0 {
		return interpreter.Null(), fmt.Errorf("decimal.pow: the exponent must be an int >= 0")
	}
	if args[1].Int > maxDigits*4 {
		return interpreter.Null(), raise(ctx, "pow", errLimit)
	}
	r, err := pow(d, int(args[1].Int))
	if err != nil {
		return interpreter.Null(), raise(ctx, "pow", err)
	}
	return wrap(r), nil
}

// sumFn implements `decimal.sum(xs)` over a list of operands; the empty sum is 0.
func sumFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 || args[0].Kind != interpreter.KindList {
		return interpreter.Null(), fmt.Errorf("decimal.sum expects 1 argument, a list of decimals")
	}
	total := fromInt(0)
	for _, v := range args[0].List {
		d, err := operand(ctx, "sum", v)
		if err != nil {
			return interpreter.Null(), err
		}
		if total, err = add(total, d); err != nil {
			return interpreter.Null(), raise(ctx, "sum", err)
		}
	}
	return wrap(total), nil
}

// compareFn implements `decimal.compare(a, b) -> -1 | 0 | 1`.
func compareFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("decimal.compare expects 2 arguments, got %d", len(args))
	}
	a, err := operand(ctx, "compare", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	b, err := operand(ctx, "compare", args[1])
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.IntVal(int64(cmp(a, b))), nil
}

// toIntFn implements `decimal.toInt(d)`: only an integral value that fits int.
func toIntFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("decimal.toInt expects 1 argument, got %d", len(args))
	}
	d, err := operand(ctx, "toInt", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if !isInteger(d) {
		return interpreter.Null(), raise(ctx, "toInt", fmt.Errorf("%s has a fractional part; round it first", d))
	}
	n := new(big.Int).Quo(d.coef, pow10(d.scale))
	if !n.IsInt64() {
		return interpreter.Null(), raise(ctx, "toInt", fmt.Errorf("%s does not fit an int", d))
	}
	return interpreter.IntVal(n.Int64()), nil
}

// toFloatFn implements `decimal.toFloat(d)`: the nearest float.
func toFloatFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("decimal.toFloat expects 1 argument, got %d", len(args))
	}
	d, err := operand(ctx, "toFloat", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	f, err := strconv.ParseFloat(d.String(), 64)
	if err != nil {
		return interpreter.Null(), raise(ctx, "toFloat", fmt.Errorf("%s is out of float range", d))
	}
	return interpreter.FloatVal(f), nil
}
