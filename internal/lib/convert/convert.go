// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package convert implements Jennifer's `convert` library: explicit value
// conversions between the primitive kinds, plus `typeOf` for runtime kind
// introspection.
package convert

import (
	"fmt"
	"math"
	"strconv"
	"unicode/utf8"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// LibraryName is the Jennifer name programs `use` to enable these functions.
const LibraryName = "convert"

// Install registers convert library functions on an interpreter. Every
// name is namespaced behind `convert.`. The four conversion
// callees are named `toInt`, `toFloat`, `toString`, `toBool` so they
// don't collide with the type keywords (`int`, `float`, ...); the
// `to`-prefixed verb also reads as English at the call site
// (`convert.toInt("42")`). `typeOf` stays as-is - it doesn't have a
// keyword collision and the name carries its own intent.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespaced(LibraryName, "toInt", toIntFn)
	in.RegisterNamespaced(LibraryName, "toFloat", toFloatFn)
	in.RegisterNamespaced(LibraryName, "toString", toStringFn)
	in.RegisterNamespaced(LibraryName, "toBool", toBoolFn)
	in.RegisterNamespaced(LibraryName, "typeOf", typeOfFn)
	in.RegisterNamespaced(LibraryName, "objectType", objectTypeFn)
	// bytes <-> string codecs. Two-argument shape (value, codec)
	// follows the `toInt(v)` / `toFloat(v)` style; codec selects the
	// encoding (only "utf-8" today). Further codecs ship with the
	// `encoding` library.
	in.RegisterNamespaced(LibraryName, "bytesFromString", bytesFromStringFn)
	in.RegisterNamespaced(LibraryName, "stringFromBytes", stringFromBytesFn)
	// Rune <-> code-point-integer, the inverse pair a Unicode algorithm
	// (Punycode, escaping) needs and that `strings.chars` (rune strings)
	// alone cannot give.
	in.RegisterNamespaced(LibraryName, "toCodepoint", toCodepointFn)
	in.RegisterNamespaced(LibraryName, "fromCodepoint", fromCodepointFn)
}

// toCodepointFn implements `convert.toCodepoint(char)`: the Unicode code point
// of a one-rune string, as an int. Errors unless the argument is exactly one
// code point (a grapheme cluster - base plus combining marks - is several code
// points and is rejected).
func toCodepointFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityOne("toCodepoint", args); err != nil {
		return interpreter.Null(), err
	}
	if args[0].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("toCodepoint: argument must be string, got %s", args[0].Kind)
	}
	s := args[0].Str
	if utf8.RuneCountInString(s) != 1 {
		return interpreter.Null(), fmt.Errorf("toCodepoint: argument must be exactly one code point, got %q", s)
	}
	r, _ := utf8.DecodeRuneInString(s)
	return interpreter.IntVal(int64(r)), nil
}

// fromCodepointFn implements `convert.fromCodepoint(n)`: the one-rune string
// for a Unicode code point (whole range, 1-4 UTF-8 bytes). Errors on a
// negative, out-of-range, or surrogate value.
func fromCodepointFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityOne("fromCodepoint", args); err != nil {
		return interpreter.Null(), err
	}
	if args[0].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("fromCodepoint: argument must be int, got %s", args[0].Kind)
	}
	n := args[0].Int
	if n < 0 || n > utf8.MaxRune || (n >= 0xD800 && n <= 0xDFFF) {
		return interpreter.Null(), fmt.Errorf("fromCodepoint: %d is not a valid Unicode code point", n)
	}
	return interpreter.StringVal(string(rune(n))), nil
}

// arityOne returns an error if args doesn't contain exactly one value.
func arityOne(name string, args []interpreter.Value) error {
	if len(args) != 1 {
		return fmt.Errorf("%s expects 1 argument, got %d", name, len(args))
	}
	return nil
}

// toIntFn implements `convert.toInt(v)`:
//   - int    -> identity
//   - float  -> truncate toward zero (Go's int64 cast)
//   - string -> strconv.ParseInt(base 10, 64-bit); error on bad input
//   - bool   -> true=1, false=0
//   - null   -> error
func toIntFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityOne("toInt", args); err != nil {
		return interpreter.Null(), err
	}
	v := args[0]
	switch v.Kind {
	case interpreter.KindInt:
		return v, nil
	case interpreter.KindFloat:
		// Truncate toward zero, but reject the values int64 cannot hold - a
		// bare int64(f) is platform-defined garbage for NaN / Inf / out of
		// range, which contradicts convert's canonical-only contract.
		f := v.Float
		if math.IsNaN(f) {
			return interpreter.Null(), fmt.Errorf("toInt(): value is not a number")
		}
		if math.IsInf(f, 0) || f >= 9223372036854775808.0 || f < -9223372036854775808.0 {
			return interpreter.Null(), fmt.Errorf("toInt(): %g does not fit in an int", f)
		}
		return interpreter.IntVal(int64(f)), nil
	case interpreter.KindString:
		n, err := strconv.ParseInt(v.Str, 10, 64)
		if err != nil {
			return interpreter.Null(), fmt.Errorf("toInt(%q): not a valid integer", v.Str)
		}
		return interpreter.IntVal(n), nil
	case interpreter.KindBool:
		if v.Bool {
			return interpreter.IntVal(1), nil
		}
		return interpreter.IntVal(0), nil
	}
	return interpreter.Null(), fmt.Errorf("toInt(): cannot convert %s to int", v.Kind)
}

// toFloatFn implements `convert.toFloat(v)`:
//   - int    -> convert
//   - float  -> identity
//   - string -> strconv.ParseFloat(64-bit); error on bad input
//   - bool   -> true=1.0, false=0.0
//   - null   -> error
func toFloatFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityOne("toFloat", args); err != nil {
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
			return interpreter.Null(), fmt.Errorf("toFloat(%q): not a valid float", v.Str)
		}
		// strconv.ParseFloat accepts "NaN" / "Inf" / "Infinity"; Jennifer's
		// float model forbids those values (the whole numeric stdlib assumes
		// they cannot exist), so reject them rather than smuggle one in.
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return interpreter.Null(), fmt.Errorf("toFloat(%q): NaN and Infinity are not representable", v.Str)
		}
		return interpreter.FloatVal(f), nil
	case interpreter.KindBool:
		if v.Bool {
			return interpreter.FloatVal(1.0), nil
		}
		return interpreter.FloatVal(0.0), nil
	}
	return interpreter.Null(), fmt.Errorf("toFloat(): cannot convert %s to float", v.Kind)
}

// toStringFn implements `convert.toString(v)`: returns the value's
// display form. Never fails (every kind has a defined Display).
func toStringFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityOne("toString", args); err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(args[0].Display()), nil
}

// toBoolFn implements `convert.toBool(v)` with strict canonical-only conversions:
//
//   - bool   -> identity
//   - int    -> 0 = false, 1 = true; any other int errors
//   - float  -> 0.0 = false, 1.0 = true; any other float errors
//   - string -> "true" = true, "false" = false; any other string errors
//   - null   -> always errors
//
// If you want "nonzero counts as true" semantics, write the comparison
// explicitly: `def b as bool init $x != 0;`.
func toBoolFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityOne("toBool", args); err != nil {
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
		return interpreter.Null(), fmt.Errorf("toBool(%d): only 0 and 1 are accepted (use `$x != 0` for truthiness)", v.Int)
	case interpreter.KindFloat:
		switch v.Float {
		case 0:
			return interpreter.BoolVal(false), nil
		case 1:
			return interpreter.BoolVal(true), nil
		}
		return interpreter.Null(), fmt.Errorf("toBool(%s): only 0.0 and 1.0 are accepted (use `$x != 0.0` for truthiness)", interpreter.DisplayFloat(v.Float))
	case interpreter.KindString:
		switch v.Str {
		case "true":
			return interpreter.BoolVal(true), nil
		case "false":
			return interpreter.BoolVal(false), nil
		}
		return interpreter.Null(), fmt.Errorf("toBool(%q): only \"true\" or \"false\" are accepted", v.Str)
	}
	return interpreter.Null(), fmt.Errorf("toBool(): cannot convert %s to bool", v.Kind)
}

// typeOfFn returns the runtime kind name of its argument as a string:
// "null", "int", "float", "string", or "bool". Useful for debugging and
// runtime introspection.
func typeOfFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityOne("typeOf", args); err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(args[0].Kind.String()), nil
}

// objectTypeFn implements `convert.objectType(v) -> string`: the specific
// registered name of an opaque object value (e.g. "json.Value"), where
// convert.typeOf only reports the generic "object". Errors on a non-object.
func objectTypeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityOne("objectType", args); err != nil {
		return interpreter.Null(), err
	}
	v := args[0]
	if v.Kind != interpreter.KindObject {
		return interpreter.Null(), fmt.Errorf("convert.objectType: argument must be an object (convert.typeOf == \"object\"), got %s", v.Kind)
	}
	return interpreter.StringVal(v.StructNS + "." + v.StructName), nil
}

// bytesFromStringFn implements `convert.bytesFromString(s, codec) -> bytes`.
// `codec` selects the encoding; only "utf-8" is supported today. The
// returned bytes are a fresh slice; modifying them does not affect the
// source string.
func bytesFromStringFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("bytesFromString expects 2 arguments, got %d", len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("bytesFromString: first argument must be string, got %s", args[0].Kind)
	}
	if args[1].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("bytesFromString: codec must be string, got %s", args[1].Kind)
	}
	codec := args[1].Str
	if codec != "utf-8" {
		return interpreter.Null(), fmt.Errorf("bytesFromString: codec %q is not supported (only \"utf-8\" today)", codec)
	}
	// Go strings are already UTF-8 internally - copy the bytes so the
	// caller's mutations can't reach back into shared interned-string
	// storage.
	src := args[0].Str
	out := make([]byte, len(src))
	copy(out, src)
	return interpreter.BytesVal(out), nil
}

// stringFromBytesFn implements `convert.stringFromBytes(b, codec) -> string`.
// `codec` selects the encoding; only "utf-8" is supported today. Invalid
// UTF-8 input is rejected (matches Jennifer's "strict at boundaries"
// stance - no silent replacement characters).
func stringFromBytesFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("stringFromBytes expects 2 arguments, got %d", len(args))
	}
	if args[0].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("stringFromBytes: first argument must be bytes, got %s", args[0].Kind)
	}
	if args[1].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("stringFromBytes: codec must be string, got %s", args[1].Kind)
	}
	codec := args[1].Str
	if codec != "utf-8" {
		return interpreter.Null(), fmt.Errorf("stringFromBytes: codec %q is not supported (only \"utf-8\" today)", codec)
	}
	src := args[0].Bytes
	if !utf8Valid(src) {
		return interpreter.Null(), fmt.Errorf("stringFromBytes: input is not valid UTF-8")
	}
	return interpreter.StringVal(string(src)), nil
}

// utf8Valid wraps Go's utf8.Valid with a one-byte fast path. Inlined to
// avoid pulling the `unicode/utf8` import into convert.go just for this.
func utf8Valid(b []byte) bool {
	for i := 0; i < len(b); {
		if b[i] < 0x80 {
			i++
			continue
		}
		size, ok := utf8DecodeSize(b[i:])
		if !ok {
			return false
		}
		i += size
	}
	return true
}

// utf8DecodeSize returns the rune length of the encoding starting at b[0],
// and ok=false if the bytes don't form a valid multi-byte sequence.
func utf8DecodeSize(b []byte) (int, bool) {
	if len(b) == 0 {
		return 0, false
	}
	c := b[0]
	switch {
	case c&0xE0 == 0xC0:
		if len(b) < 2 || b[1]&0xC0 != 0x80 || c&0x1E == 0 {
			return 0, false
		}
		return 2, true
	case c&0xF0 == 0xE0:
		if len(b) < 3 || b[1]&0xC0 != 0x80 || b[2]&0xC0 != 0x80 {
			return 0, false
		}
		// Reject overlongs and surrogates.
		if c == 0xE0 && b[1] < 0xA0 {
			return 0, false
		}
		if c == 0xED && b[1] >= 0xA0 {
			return 0, false
		}
		return 3, true
	case c&0xF8 == 0xF0:
		if len(b) < 4 || b[1]&0xC0 != 0x80 || b[2]&0xC0 != 0x80 || b[3]&0xC0 != 0x80 {
			return 0, false
		}
		if c == 0xF0 && b[1] < 0x90 {
			return 0, false
		}
		if c > 0xF4 || (c == 0xF4 && b[1] >= 0x90) {
			return 0, false
		}
		return 4, true
	}
	return 0, false
}
