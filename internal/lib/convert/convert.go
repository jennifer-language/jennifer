// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// Package convert implements Jennifer's `convert` library: explicit value
// conversions between the primitive kinds, plus `typeOf` for runtime kind
// introspection.
package convert

import (
	"fmt"
	"math"
	"strconv"
	"unicode/utf8"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// LibraryName is the Jennifer name programs `use` to enable these functions.
const LibraryName = "convert"

// dataKind is the kind of a string that does not parse as the requested
// number or bool (M29.21): text from input, found bad. Converting a program
// value (a NaN float, an int other than 0 / 1) stays "runtime".
const dataKind = "convert"

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
	// bytes <-> string codecs. Two-argument shape (codec, value):
	// the codec name comes first, like every name-taking library
	// function; it selects the encoding (only "utf-8" today). Further codecs ship with the
	// `encoding` library.
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
		return interpreter.Null(), fmt.Errorf("convert.toCodepoint: argument must be string, got %s", args[0].Kind)
	}
	s := args[0].Str()
	if utf8.RuneCountInString(s) != 1 {
		return interpreter.Null(), fmt.Errorf("convert.toCodepoint: argument must be exactly one code point, got %q", s)
	}
	// A lone invalid byte counts as one rune and decodes to U+FFFD; it is not
	// a code point of the input.
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError && size == 1 {
		return interpreter.Null(), fmt.Errorf("convert.toCodepoint: argument is not valid UTF-8 (byte 0x%02X)", s[0])
	}
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
		return interpreter.Null(), fmt.Errorf("convert.fromCodepoint: argument must be int, got %s", args[0].Kind)
	}
	n := args[0].Int()
	if n < 0 || n > utf8.MaxRune || (n >= 0xD800 && n <= 0xDFFF) {
		return interpreter.Null(), fmt.Errorf("convert.fromCodepoint: %d is not a valid Unicode code point", n)
	}
	return interpreter.StringVal(string(rune(n))), nil
}

// arityOne returns an error if args doesn't contain exactly one value.
func arityOne(name string, args []interpreter.Value) error {
	if len(args) != 1 {
		return fmt.Errorf("convert.%s expects 1 argument, got %d", name, len(args))
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
	if len(args) != 1 && len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("convert.toInt expects 1 or 2 arguments (value[, radix]), got %d", len(args))
	}
	v := args[0]
	// An optional radix parses a STRING's digits in a chosen base - a hex numeric
	// character reference, an octal permission mask, a binary flag word. The base
	// is one of 2, 8, 10, 16 (the bases Jennifer's int literals use), applied to
	// bare digits with no `0x` / `0o` / `0b` prefix (strip the prefix first). A
	// radix is meaningless for a non-string value, so passing one there is a misuse
	// rather than a silent no-op.
	if len(args) == 2 {
		if v.Kind != interpreter.KindString {
			return interpreter.Null(), fmt.Errorf("convert.toInt: a radix is only valid when converting a string, not %s", v.Kind)
		}
		if args[1].Kind != interpreter.KindInt {
			return interpreter.Null(), fmt.Errorf("convert.toInt: radix must be int, got %s", args[1].Kind)
		}
		radix := args[1].Int()
		switch radix {
		case 2, 8, 10, 16:
		default:
			return interpreter.Null(), fmt.Errorf("convert.toInt: radix must be 2, 8, 10, or 16, got %d", radix)
		}
		n, err := strconv.ParseInt(v.Str(), int(radix), 64)
		if err != nil {
			// A 0x / 0o / 0b prefix is never accepted under an explicit radix (its
			// letter is not a digit in the base), so the parse fails rather than
			// misreads - point the caller at the fix instead of a bare "invalid".
			if hasBasePrefix(v.Str()) {
				return interpreter.Null(), dataerr.New(dataKind, "convert.toInt: %q (radix %d): a 0x / 0o / 0b prefix is not accepted; pass the bare digits", v.Str(), radix)
			}
			return interpreter.Null(), dataerr.New(dataKind, "convert.toInt: %q is not a valid base-%d integer", v.Str(), radix)
		}
		return interpreter.IntVal(n), nil
	}
	switch v.Kind {
	case interpreter.KindInt:
		return v, nil
	case interpreter.KindFloat:
		// Truncate toward zero, but reject the values int64 cannot hold - a
		// bare int64(f) is platform-defined garbage for NaN / Inf / out of
		// range, which contradicts convert's canonical-only contract.
		f := v.Float()
		if math.IsNaN(f) {
			return interpreter.Null(), fmt.Errorf("convert.toInt: value is not a number")
		}
		if math.IsInf(f, 0) || f >= 9223372036854775808.0 || f < -9223372036854775808.0 {
			return interpreter.Null(), fmt.Errorf("convert.toInt: %g does not fit in an int", f)
		}
		return interpreter.IntVal(int64(f)), nil
	case interpreter.KindString:
		n, err := strconv.ParseInt(v.Str(), 10, 64)
		if err != nil {
			// The one-arg form is decimal only, so a 0x / 0o / 0b number lands here;
			// point at the radix form rather than a bare "invalid".
			if hasBasePrefix(v.Str()) {
				return interpreter.Null(), dataerr.New(dataKind, "convert.toInt: %q is not a decimal integer; for a 0x / 0o / 0b number use convert.toInt(s, radix) with the bare digits", v.Str())
			}
			return interpreter.Null(), dataerr.New(dataKind, "convert.toInt: %q is not a valid integer", v.Str())
		}
		return interpreter.IntVal(n), nil
	case interpreter.KindBool:
		if v.Bool() {
			return interpreter.IntVal(1), nil
		}
		return interpreter.IntVal(0), nil
	}
	return interpreter.Null(), fmt.Errorf("convert.toInt: cannot convert %s to int", v.Kind)
}

// hasBasePrefix reports whether s, after an optional leading + or -, begins with a
// 0x / 0o / 0b base prefix - the shape toInt's explicit-radix form rejects, so its
// error can say why.
func hasBasePrefix(s string) bool {
	if len(s) > 0 && (s[0] == '+' || s[0] == '-') {
		s = s[1:]
	}
	if len(s) < 2 || s[0] != '0' {
		return false
	}
	switch s[1] {
	case 'x', 'X', 'o', 'O', 'b', 'B':
		return true
	}
	return false
}

// isDecimalFloat reports whether s is a decimal float:
// [+-]? (digits [. digits*] | . digits) ([eE] [+-]? digits)?
func isDecimalFloat(s string) bool {
	i := 0
	digits := func() int {
		start := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		return i - start
	}
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	mant := digits()
	if i < len(s) && s[i] == '.' {
		i++
		mant += digits()
	}
	if mant == 0 {
		return false
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		if digits() == 0 {
			return false
		}
	}
	return i == len(s)
}

// toFloatFn implements `convert.toFloat(v)`:
//   - int    -> convert
//   - float  -> identity
//   - string -> decimal float (see isDecimalFloat); error on bad input
//   - bool   -> true=1.0, false=0.0
//   - null   -> error
func toFloatFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityOne("toFloat", args); err != nil {
		return interpreter.Null(), err
	}
	v := args[0]
	switch v.Kind {
	case interpreter.KindInt:
		return interpreter.FloatVal(float64(v.Int())), nil
	case interpreter.KindFloat:
		return v, nil
	case interpreter.KindString:
		// strconv.ParseFloat also takes Go literal forms ("1_000", "0x1p4") and
		// "NaN" / "Inf" / "Infinity"; accept only the decimal form, as toInt
		// accepts only decimal digits. Jennifer's float model forbids NaN and
		// Infinity (the numeric stdlib assumes they cannot exist).
		if !isDecimalFloat(v.Str()) {
			return interpreter.Null(), dataerr.New(dataKind, "convert.toFloat: %q is not a valid float", v.Str())
		}
		f, err := strconv.ParseFloat(v.Str(), 64)
		if err != nil {
			return interpreter.Null(), dataerr.New(dataKind, "convert.toFloat: %q is not a valid float", v.Str())
		}
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return interpreter.Null(), dataerr.New(dataKind, "convert.toFloat: %q: NaN and Infinity are not representable", v.Str())
		}
		return interpreter.FloatVal(f), nil
	case interpreter.KindBool:
		if v.Bool() {
			return interpreter.FloatVal(1.0), nil
		}
		return interpreter.FloatVal(0.0), nil
	}
	return interpreter.Null(), fmt.Errorf("convert.toFloat: cannot convert %s to float", v.Kind)
}

// toStringFn implements `convert.toString(v[, radix])`. One argument returns the
// value's display form (never fails; every kind has a defined Display). A second
// argument renders an int's digits in a chosen base - the print half of
// toInt(s, radix), so a value round-trips through either direction. The base is
// one of 2, 8, 10, 16 (toInt's set), produces bare lowercase digits with no 0x /
// 0o / 0b prefix, and is valid only for an int (a radix on any other kind is a
// misuse, mirroring toInt's string-only radix).
func toStringFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) == 2 {
		// The radix form was removed in 0.28.0 (M29.35): digits in a base are
		// a presentation, which printf owns.
		return interpreter.Null(), fmt.Errorf("convert.toString expects 1 argument (value), got 2; for the digits of an int in a base, write io.sprintf(\"%%d|base=16\", n)")
	}
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("convert.toString expects 1 argument (value), got %d", len(args))
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
		switch v.Int() {
		case 0:
			return interpreter.BoolVal(false), nil
		case 1:
			return interpreter.BoolVal(true), nil
		}
		return interpreter.Null(), fmt.Errorf("convert.toBool: %d: only 0 and 1 are accepted (use `$x != 0` for truthiness)", v.Int())
	case interpreter.KindFloat:
		switch v.Float() {
		case 0:
			return interpreter.BoolVal(false), nil
		case 1:
			return interpreter.BoolVal(true), nil
		}
		return interpreter.Null(), fmt.Errorf("convert.toBool: %s: only 0.0 and 1.0 are accepted (use `$x != 0.0` for truthiness)", interpreter.DisplayFloat(v.Float()))
	case interpreter.KindString:
		switch v.Str() {
		case "true":
			return interpreter.BoolVal(true), nil
		case "false":
			return interpreter.BoolVal(false), nil
		}
		return interpreter.Null(), dataerr.New(dataKind, "convert.toBool: %q: only \"true\" or \"false\" are accepted", v.Str())
	}
	return interpreter.Null(), fmt.Errorf("convert.toBool: cannot convert %s to bool", v.Kind)
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
	return interpreter.StringVal(v.StructNS() + "." + v.StructName()), nil
}

// utf8Valid reports whether b is valid UTF-8, rejecting overlong encodings,
// surrogates, and code points above U+10FFFF, with a one-byte ASCII fast path.
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
