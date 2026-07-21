// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package regexlib implements Jennifer's `regex` library:
// regular expressions over `string` using Go's `regexp` package
// (RE2 syntax). No backreferences, no lookahead/lookbehind - RE2
// is a documented subset of PCRE.
//
// The library takes patterns as strings (no explicit
// `regex.Pattern` handle) and maintains an internal LRU cache of
// compiled patterns so hot loops get compile-once behaviour
// without user bookkeeping. Cap: 128 entries.
//
// The Go package is named regexlib to avoid colliding with the
// standard `regexp` package it depends on.
package regexlib

import (
	"container/list"
	"fmt"
	"regexp"
	"sync"
	"unicode/utf8"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// LibraryName is the namespace prefix and `use` name.
const LibraryName = "regex"

// Value alias keeps signatures short.
type Value = interpreter.Value

// -------- Pattern cache --------

// cacheCap bounds the LRU. Chosen to comfortably cover real
// programs' distinct patterns without unbounded memory growth.
const cacheCap = 128

type cacheEntry struct {
	pattern string
	re      *regexp.Regexp
}

var (
	cacheMu  sync.Mutex
	cacheMap = map[string]*list.Element{}
	cacheLRU = list.New() // front = MRU, back = LRU
)

// resetCacheForTest clears the LRU between tests so eviction
// behaviour stays deterministic.
func resetCacheForTest() {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	cacheMap = map[string]*list.Element{}
	cacheLRU = list.New()
}

// compilePattern returns a compiled *regexp.Regexp for pattern,
// consulting and updating the LRU cache. Compile errors surface
// with the pattern quoted.
func compilePattern(fnName, pattern string) (*regexp.Regexp, error) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if elem, ok := cacheMap[pattern]; ok {
		cacheLRU.MoveToFront(elem)
		return elem.Value.(*cacheEntry).re, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("%s: invalid pattern %q: %v", fnName, pattern, err)
	}
	elem := cacheLRU.PushFront(&cacheEntry{pattern: pattern, re: re})
	cacheMap[pattern] = elem
	if cacheLRU.Len() > cacheCap {
		oldest := cacheLRU.Back()
		if oldest != nil {
			cacheLRU.Remove(oldest)
			delete(cacheMap, oldest.Value.(*cacheEntry).pattern)
		}
	}
	return re, nil
}

// -------- Install --------

func Install(in *interpreter.Interpreter) {
	in.RegisterNamespacedStruct(LibraryName, "Match", []parser.StructField{
		{Name: "text", Type: parser.PrimitiveType(parser.TypeString)},
		{Name: "start", Type: parser.PrimitiveType(parser.TypeInt)},
		{Name: "end", Type: parser.PrimitiveType(parser.TypeInt)},
		{Name: "groups", Type: parser.ListType(parser.PrimitiveType(parser.TypeString))},
		{Name: "groupsNamed", Type: parser.MapType(
			parser.PrimitiveType(parser.TypeString),
			parser.PrimitiveType(parser.TypeString),
		)},
	})

	in.RegisterNamespaced(LibraryName, "matches", matchesFn)
	in.RegisterNamespaced(LibraryName, "find", findFn)
	in.RegisterNamespaced(LibraryName, "findAll", findAllFn)
	in.RegisterNamespaced(LibraryName, "replace", replaceFn)
	in.RegisterNamespaced(LibraryName, "split", splitFn)
	in.RegisterNamespaced(LibraryName, "escape", escapeFn)
}

// takeStringArg is the boundary check for a positional string.
func takeStringArg(fnName string, args []Value, idx int, role string) (string, error) {
	if args[idx].Kind != interpreter.KindString {
		return "", fmt.Errorf("%s: %s must be string, got %s", fnName, role, args[idx].Kind)
	}
	return args[idx].Str, nil
}

// -------- byte -> rune index translation --------

// runeTracker converts ascending byte offsets in s to rune indices in
// amortized O(len(s)) total, by counting runes only over the slice since the
// last offset. A single tracker threaded through all matches of a findAll turns
// an O(N*M) rescan-from-start into O(N). Offsets within and across matches are
// ascending; a rare non-monotonic offset falls back to a full recount.
type runeTracker struct {
	s        string
	lastByte int
	lastRune int
}

func newRuneTracker(s string) *runeTracker { return &runeTracker{s: s} }

func (t *runeTracker) at(off int) int {
	if off <= 0 {
		return 0
	}
	if off < t.lastByte {
		// Non-monotonic (defensive): recount from the start.
		if off >= len(t.s) {
			return utf8.RuneCountInString(t.s)
		}
		return utf8.RuneCountInString(t.s[:off])
	}
	if off > len(t.s) {
		off = len(t.s)
	}
	t.lastRune += utf8.RuneCountInString(t.s[t.lastByte:off])
	t.lastByte = off
	return t.lastRune
}

// -------- Match construction --------

// buildMatch builds a regex.Match Value from a submatch slice
// (parallel to re.SubexpNames()) and the source string.
// submatchIndex is the [start_byte, end_byte, group1_start,
// group1_end, ...] slice from FindStringSubmatchIndex; can be
// nil for no match, in which case a sentinel is returned.
func buildMatch(re *regexp.Regexp, s string, submatch []string, indices []int, tr *runeTracker) Value {
	if submatch == nil || indices == nil {
		return sentinelMatch()
	}
	startByte := indices[0]
	endByte := indices[1]
	startRune := tr.at(startByte)
	endRune := tr.at(endByte)

	// Positional groups. Slot 0 in submatch is the full match;
	// callers see groups 1..N, so skip index 0.
	positional := make([]Value, 0, len(submatch)-1)
	for i := 1; i < len(submatch); i++ {
		positional = append(positional, interpreter.StringVal(submatch[i]))
	}

	// Named groups. SubexpNames() returns "" for unnamed slots
	// and the group name for named ones; slot 0 is always "".
	named := make([]interpreter.MapEntry, 0)
	names := re.SubexpNames()
	for i := 1; i < len(names); i++ {
		if names[i] == "" {
			continue
		}
		named = append(named, interpreter.MapEntry{
			Key:   interpreter.StringVal(names[i]),
			Value: interpreter.StringVal(submatch[i]),
		})
	}

	return interpreter.NamespacedStructVal(LibraryName, "Match", []interpreter.StructField{
		{Name: "text", Value: interpreter.StringVal(submatch[0])},
		{Name: "start", Value: interpreter.IntVal(int64(startRune))},
		{Name: "end", Value: interpreter.IntVal(int64(endRune))},
		{Name: "groups", Value: interpreter.ListVal(
			parser.PrimitiveType(parser.TypeString), positional,
		)},
		{Name: "groupsNamed", Value: interpreter.MapVal(
			parser.PrimitiveType(parser.TypeString),
			parser.PrimitiveType(parser.TypeString),
			named,
		)},
	})
}

// sentinelMatch is the "no match" return of regex.find. Distinct
// from a real match with text="" (an empty capture) because the
// sentinel has start=-1 / end=-1.
func sentinelMatch() Value {
	return interpreter.NamespacedStructVal(LibraryName, "Match", []interpreter.StructField{
		{Name: "text", Value: interpreter.StringVal("")},
		{Name: "start", Value: interpreter.IntVal(-1)},
		{Name: "end", Value: interpreter.IntVal(-1)},
		{Name: "groups", Value: interpreter.ListVal(
			parser.PrimitiveType(parser.TypeString), nil,
		)},
		{Name: "groupsNamed", Value: interpreter.MapVal(
			parser.PrimitiveType(parser.TypeString),
			parser.PrimitiveType(parser.TypeString),
			nil,
		)},
	})
}

// -------- Verbs --------

func matchesFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("regex.matches expects 2 arguments (pattern, s), got %d", len(args))
	}
	pattern, err := takeStringArg("regex.matches", args, 0, "pattern")
	if err != nil {
		return interpreter.Null(), err
	}
	s, err := takeStringArg("regex.matches", args, 1, "s")
	if err != nil {
		return interpreter.Null(), err
	}
	re, err := compilePattern("regex.matches", pattern)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.BoolVal(re.MatchString(s)), nil
}

func findFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("regex.find expects 2 arguments (pattern, s), got %d", len(args))
	}
	pattern, err := takeStringArg("regex.find", args, 0, "pattern")
	if err != nil {
		return interpreter.Null(), err
	}
	s, err := takeStringArg("regex.find", args, 1, "s")
	if err != nil {
		return interpreter.Null(), err
	}
	re, err := compilePattern("regex.find", pattern)
	if err != nil {
		return interpreter.Null(), err
	}
	// One execution: the submatch strings are slices of s at the returned
	// indices, so there is no need for a second FindStringSubmatch pass.
	indices := re.FindStringSubmatchIndex(s)
	return buildMatch(re, s, submatchFromIndex(s, indices), indices, newRuneTracker(s)), nil
}

// submatchFromIndex reconstructs the FindStringSubmatch string slice from a
// FindStringSubmatchIndex result (a non-participating group, index -1, is "").
func submatchFromIndex(s string, idx []int) []string {
	if idx == nil {
		return nil
	}
	out := make([]string, len(idx)/2)
	for i := range out {
		start, end := idx[2*i], idx[2*i+1]
		if start >= 0 && end >= 0 {
			out[i] = s[start:end]
		}
	}
	return out
}

func findAllFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("regex.findAll expects 2 arguments (pattern, s), got %d", len(args))
	}
	pattern, err := takeStringArg("regex.findAll", args, 0, "pattern")
	if err != nil {
		return interpreter.Null(), err
	}
	s, err := takeStringArg("regex.findAll", args, 1, "s")
	if err != nil {
		return interpreter.Null(), err
	}
	re, err := compilePattern("regex.findAll", pattern)
	if err != nil {
		return interpreter.Null(), err
	}
	allIdx := re.FindAllStringSubmatchIndex(s, -1)
	results := make([]Value, len(allIdx))
	// One tracker for the whole scan: match offsets are ascending, so byte->
	// rune conversion stays amortized O(len(s)) instead of O(matches*len(s)).
	tr := newRuneTracker(s)
	for i := range allIdx {
		results[i] = buildMatch(re, s, submatchFromIndex(s, allIdx[i]), allIdx[i], tr)
	}
	return interpreter.ListVal(
		parser.NamespacedStructType(LibraryName, "Match"),
		results,
	), nil
}

func replaceFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 3 {
		return interpreter.Null(), fmt.Errorf("regex.replace expects 3 arguments (pattern, s, replacement), got %d", len(args))
	}
	pattern, err := takeStringArg("regex.replace", args, 0, "pattern")
	if err != nil {
		return interpreter.Null(), err
	}
	s, err := takeStringArg("regex.replace", args, 1, "s")
	if err != nil {
		return interpreter.Null(), err
	}
	replacement, err := takeStringArg("regex.replace", args, 2, "replacement")
	if err != nil {
		return interpreter.Null(), err
	}
	re, err := compilePattern("regex.replace", pattern)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(re.ReplaceAllString(s, replacement)), nil
}

func splitFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("regex.split expects 2 arguments (pattern, s), got %d", len(args))
	}
	pattern, err := takeStringArg("regex.split", args, 0, "pattern")
	if err != nil {
		return interpreter.Null(), err
	}
	s, err := takeStringArg("regex.split", args, 1, "s")
	if err != nil {
		return interpreter.Null(), err
	}
	re, err := compilePattern("regex.split", pattern)
	if err != nil {
		return interpreter.Null(), err
	}
	parts := re.Split(s, -1)
	out := make([]Value, len(parts))
	for i, p := range parts {
		out[i] = interpreter.StringVal(p)
	}
	return interpreter.ListVal(
		parser.PrimitiveType(parser.TypeString), out,
	), nil
}

func escapeFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("regex.escape expects 1 argument (s), got %d", len(args))
	}
	s, err := takeStringArg("regex.escape", args, 0, "s")
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(regexp.QuoteMeta(s)), nil
}

// ResetForTest wipes the LRU cache. Exported so the _test package
// can drive it.
func ResetForTest() {
	resetCacheForTest()
}
