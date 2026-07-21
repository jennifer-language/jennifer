// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package stringslib implements Jennifer's `strings` library: text utilities
// like length, case conversion, search, trim, and substring extraction. All
// operations that involve indices or lengths work on Unicode code points
// (runes), not bytes - so `len("héllo")` is 5, not 6.
//
// The Go package is named stringslib to avoid colliding with Go's standard
// `strings` package, which this implementation depends on.
package stringslib

import (
	"fmt"
	"math"
	gostrings "strings"
	"unicode"
	"unicode/utf8"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// LibraryName is the Jennifer name programs `use` to enable these functions.
const LibraryName = "strings"

// Install registers strings library functions on an interpreter.
//
// **strings is namespaced.** Every name lives behind the `strings.`
// prefix - call as `strings.upper(s)`, `strings.contains(s, sub)`, etc.
// The library was previously flat; the hybrid model relocates collision-
// prone verbs (`contains`, `split`, `replace`, ...) under their domain
// library so they don't pollute the bare-name pool. Same rule that
// drove `lists`, `maps`, and `os` into namespaced form.
//
// `len` used to live here as the rune-count function for strings; it
// is now a language built-in keyword so the same name covers
// strings, lists, maps, and bytes with one polymorphic dispatch.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespaced(LibraryName, "upper", upperFn)
	in.RegisterNamespaced(LibraryName, "lower", lowerFn)
	in.RegisterNamespaced(LibraryName, "contains", containsFn)
	in.RegisterNamespaced(LibraryName, "startsWith", startsWithFn)
	in.RegisterNamespaced(LibraryName, "endsWith", endsWithFn)
	in.RegisterNamespaced(LibraryName, "indexOf", indexOfFn)
	in.RegisterNamespaced(LibraryName, "trim", trimFn)
	in.RegisterNamespaced(LibraryName, "trimLeft", trimLeftFn)
	in.RegisterNamespaced(LibraryName, "trimRight", trimRightFn)
	in.RegisterNamespaced(LibraryName, "replace", replaceFn)
	in.RegisterNamespaced(LibraryName, "repeat", repeatFn)
	in.RegisterNamespaced(LibraryName, "substring", substringFn)
	in.RegisterNamespaced(LibraryName, "split", splitFn)
	in.RegisterNamespaced(LibraryName, "chars", charsFn)
	in.RegisterNamespaced(LibraryName, "join", joinFn)
}

// ---- helpers ----

func arityN(name string, args []interpreter.Value, want int) error {
	if len(args) != want {
		return fmt.Errorf("%s expects %d argument(s), got %d", name, want, len(args))
	}
	return nil
}

// requireString returns the string value at args[i] or an error mentioning
// the position and function name.
func requireString(name string, args []interpreter.Value, i int) (string, error) {
	v := args[i]
	if v.Kind != interpreter.KindString {
		return "", fmt.Errorf("%s(): argument %d must be string, got %s", name, i+1, v.Kind)
	}
	return v.Str, nil
}

// requireInt returns the int value at args[i] or an error.
func requireInt(name string, args []interpreter.Value, i int) (int64, error) {
	v := args[i]
	if v.Kind != interpreter.KindInt {
		return 0, fmt.Errorf("%s(): argument %d must be int, got %s", name, i+1, v.Kind)
	}
	return v.Int, nil
}

// runeIndex converts a byte offset in s to a rune index (0-based).
// Returns -1 if byteOffset == -1.
func runeIndex(s string, byteOffset int) int {
	if byteOffset < 0 {
		return -1
	}
	return utf8.RuneCountInString(s[:byteOffset])
}

// byteOffsetForRune returns the byte position of the i'th rune in s. If i ==
// runeCount(s), returns len(s). Errors if i is out of range.
func byteOffsetForRune(s string, i int) (int, error) {
	if i < 0 {
		return 0, fmt.Errorf("negative index %d", i)
	}
	idx := 0
	for byteOff := 0; byteOff < len(s); idx++ {
		if idx == i {
			return byteOff, nil
		}
		_, size := utf8.DecodeRuneInString(s[byteOff:])
		byteOff += size
	}
	if idx == i {
		return len(s), nil
	}
	return 0, fmt.Errorf("index %d out of range (length %d)", i, idx)
}

// ---- functions ----

// upperFn returns s with all letters uppercased (Unicode-aware).
func upperFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityN("upper", args, 1); err != nil {
		return interpreter.Null(), err
	}
	s, err := requireString("upper", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(gostrings.ToUpper(s)), nil
}

// lowerFn returns s with all letters lowercased (Unicode-aware).
func lowerFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityN("lower", args, 1); err != nil {
		return interpreter.Null(), err
	}
	s, err := requireString("lower", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(gostrings.ToLower(s)), nil
}

// containsFn reports whether sub appears anywhere in s.
func containsFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityN("contains", args, 2); err != nil {
		return interpreter.Null(), err
	}
	s, err := requireString("contains", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	sub, err := requireString("contains", args, 1)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.BoolVal(gostrings.Contains(s, sub)), nil
}

// startsWithFn reports whether s begins with prefix.
func startsWithFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityN("startsWith", args, 2); err != nil {
		return interpreter.Null(), err
	}
	s, err := requireString("startsWith", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	prefix, err := requireString("startsWith", args, 1)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.BoolVal(gostrings.HasPrefix(s, prefix)), nil
}

// endsWithFn reports whether s ends with suffix.
func endsWithFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityN("endsWith", args, 2); err != nil {
		return interpreter.Null(), err
	}
	s, err := requireString("endsWith", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	suffix, err := requireString("endsWith", args, 1)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.BoolVal(gostrings.HasSuffix(s, suffix)), nil
}

// indexOfFn returns the rune index of the first occurrence of sub in s, or
// -1 if sub is not present.
func indexOfFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityN("indexOf", args, 2); err != nil {
		return interpreter.Null(), err
	}
	s, err := requireString("indexOf", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	sub, err := requireString("indexOf", args, 1)
	if err != nil {
		return interpreter.Null(), err
	}
	byteIdx := gostrings.Index(s, sub)
	return interpreter.IntVal(int64(runeIndex(s, byteIdx))), nil
}

// trimFn strips leading and trailing whitespace (Unicode-aware).
func trimFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityN("trim", args, 1); err != nil {
		return interpreter.Null(), err
	}
	s, err := requireString("trim", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(gostrings.TrimFunc(s, unicode.IsSpace)), nil
}

// trimLeftFn strips leading whitespace only.
func trimLeftFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityN("trimLeft", args, 1); err != nil {
		return interpreter.Null(), err
	}
	s, err := requireString("trimLeft", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(gostrings.TrimLeftFunc(s, unicode.IsSpace)), nil
}

// trimRightFn strips trailing whitespace only.
func trimRightFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityN("trimRight", args, 1); err != nil {
		return interpreter.Null(), err
	}
	s, err := requireString("trimRight", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(gostrings.TrimRightFunc(s, unicode.IsSpace)), nil
}

// replaceFn returns s with all occurrences of old replaced by new.
func replaceFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityN("replace", args, 3); err != nil {
		return interpreter.Null(), err
	}
	s, err := requireString("replace", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	old, err := requireString("replace", args, 1)
	if err != nil {
		return interpreter.Null(), err
	}
	new, err := requireString("replace", args, 2)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(gostrings.ReplaceAll(s, old, new)), nil
}

// repeatFn returns n copies of s concatenated. Negative n is an error.
func repeatFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityN("repeat", args, 2); err != nil {
		return interpreter.Null(), err
	}
	s, err := requireString("repeat", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	n, err := requireInt("repeat", args, 1)
	if err != nil {
		return interpreter.Null(), err
	}
	if n < 0 {
		return interpreter.Null(), fmt.Errorf("repeat(): negative count %d", n)
	}
	// Range-check before narrowing to int: on a 32-bit build (TinyGo) a count
	// above MaxInt would silently truncate (2^32 -> 0, returning "").
	if n > int64(math.MaxInt) {
		return interpreter.Null(), fmt.Errorf("repeat(): count %d exceeds the platform limit", n)
	}
	// Cap the result size with a catchable error rather than attempting a
	// multi-gigabyte allocation (Go's strings.Repeat would otherwise OOM/panic).
	if n > 0 && len(s) > 0 {
		total := int64(len(s)) * n
		if total/n != int64(len(s)) || total > maxRepeatBytes {
			return interpreter.Null(), fmt.Errorf("repeat(): result of %d bytes exceeds the %d-byte limit", total, int64(maxRepeatBytes))
		}
	}
	return interpreter.StringVal(gostrings.Repeat(s, int(n))), nil
}

// maxRepeatBytes caps strings.repeat output so an accidental huge count is a
// catchable error, not an out-of-memory crash. 1 GiB is far beyond any real
// string-building need while still stopping runaway allocations.
const maxRepeatBytes int64 = 1 << 30

// substringFn returns the slice of s from rune index `start` (inclusive) to
// rune index `end` (exclusive). The end argument is optional - omitting it
// extracts from `start` to the end of the string. Indices are rune-based to
// match `len` and `indexOf`. Out-of-range indices error explicitly (no
// silent clamping).
func substringFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 && len(args) != 3 {
		return interpreter.Null(), fmt.Errorf("substring expects 2 or 3 arguments, got %d", len(args))
	}
	s, err := requireString("substring", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	start, err := requireInt("substring", args, 1)
	if err != nil {
		return interpreter.Null(), err
	}
	runeLen := int64(utf8.RuneCountInString(s))
	end := runeLen
	if len(args) == 3 {
		end, err = requireInt("substring", args, 2)
		if err != nil {
			return interpreter.Null(), err
		}
	}
	if start < 0 {
		return interpreter.Null(), fmt.Errorf("substring(): start %d is negative", start)
	}
	// Resolve and validate start first so that `substring(s, 99)` (with the
	// 2-arg form, where we auto-fill end) reports the real problem - the
	// start is out of range - rather than confusingly saying "end before
	// start" against the auto-filled end.
	startByte, err := byteOffsetForRune(s, int(start))
	if err != nil {
		return interpreter.Null(), fmt.Errorf("substring(): start out of range (length %d)", runeLen)
	}
	if end < start {
		return interpreter.Null(), fmt.Errorf("substring(): end %d is before start %d", end, start)
	}
	endByte, err := byteOffsetForRune(s, int(end))
	if err != nil {
		return interpreter.Null(), fmt.Errorf("substring(): end out of range (length %d)", runeLen)
	}
	return interpreter.StringVal(s[startByte:endByte]), nil
}

// stringListType is the `list of string` declared type stamped onto
// results of split / chars / join's inverse. Built once and reused so we
// don't allocate a fresh Type tree per call.
var stringListType = parser.ListType(parser.PrimitiveType(parser.TypeString))

// splitFn returns the substrings of s separated by sep. A literal Go
// `strings.Split` with an empty separator splits on runes; we expose
// that via the separate `chars` builtin and require a non-empty
// separator here so the behavior is unambiguous to users.
func splitFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityN("split", args, 2); err != nil {
		return interpreter.Null(), err
	}
	s, err := requireString("split", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	sep, err := requireString("split", args, 1)
	if err != nil {
		return interpreter.Null(), err
	}
	if sep == "" {
		return interpreter.Null(), fmt.Errorf("split(): separator must be non-empty; use chars() to split into runes")
	}
	parts := gostrings.Split(s, sep)
	out := make([]interpreter.Value, len(parts))
	for i, p := range parts {
		out[i] = interpreter.StringVal(p)
	}
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeString), out), nil
}

// charsFn returns the runes of s as a list of single-rune strings.
// Returns an empty list for the empty string. Each entry is one Unicode
// code point - the same unit `len`, `indexOf`, and `substring` work in.
func charsFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityN("chars", args, 1); err != nil {
		return interpreter.Null(), err
	}
	s, err := requireString("chars", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	out := make([]interpreter.Value, 0, utf8.RuneCountInString(s))
	for _, r := range s {
		out = append(out, interpreter.StringVal(string(r)))
	}
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeString), out), nil
}

// joinFn concatenates the strings in `parts` with `sep` between them.
// `parts` must be a `list of string`; any non-string element is a
// positioned error rather than silent coercion.
func joinFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityN("join", args, 2); err != nil {
		return interpreter.Null(), err
	}
	parts := args[0]
	if parts.Kind != interpreter.KindList {
		return interpreter.Null(), fmt.Errorf("join(): first argument must be a list of string, got %s", parts.Kind)
	}
	sep, err := requireString("join", args, 1)
	if err != nil {
		return interpreter.Null(), err
	}
	strs := make([]string, len(parts.List))
	for i, e := range parts.List {
		if e.Kind != interpreter.KindString {
			return interpreter.Null(), fmt.Errorf("join(): element %d is %s, expected string", i, e.Kind)
		}
		strs[i] = e.Str
	}
	return interpreter.StringVal(gostrings.Join(strs, sep)), nil
}

// (the package-level `stringListType` keeps the type-tree alloc cost
// at zero for callers that build lists of strings inline.)
var _ = stringListType
