// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

// Package listslib implements Jennifer's `lists` library (M9): the
// non-mutating manipulation helpers for `list of T` values. Every
// function returns a *new* list (or a primitive value); the input is
// never modified. Callers commit results with `$xs = lists.push($xs,
// x);` - matching Jennifer's value-semantics rule and the explicit
// "one way per thing" stance.
//
// All names are namespaced under the `lists.` prefix per M8's hybrid
// model. Verbs like `contains` / `concat` collide with `strings.contains`
// / `strings.concat` etc., and namespacing keeps the call-site
// unambiguous.
package listslib

import (
	"fmt"
	"sort"

	"github.com/mplx/jennifer-lang/internal/interpreter"
)

// LibraryName is the Jennifer name programs `use` to enable these
// functions, and doubles as the namespace prefix.
const LibraryName = "lists"

// Install registers every M9 lists builtin with the interpreter.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespaced(LibraryName, "push", pushFn)
	in.RegisterNamespaced(LibraryName, "pop", popFn)
	in.RegisterNamespaced(LibraryName, "first", firstFn)
	in.RegisterNamespaced(LibraryName, "last", lastFn)
	in.RegisterNamespaced(LibraryName, "head", headFn)
	in.RegisterNamespaced(LibraryName, "tail", tailFn)
	in.RegisterNamespaced(LibraryName, "reverse", reverseFn)
	in.RegisterNamespaced(LibraryName, "sort", sortFn)
	in.RegisterNamespaced(LibraryName, "contains", containsFn)
	in.RegisterNamespaced(LibraryName, "concat", concatFn)
	in.RegisterNamespaced(LibraryName, "slice", sliceFn)
}

func requireList(name string, v interpreter.Value, argpos string) error {
	if v.Kind != interpreter.KindList {
		return fmt.Errorf("lists.%s: %s must be a list, got %s", name, argpos, v.Kind)
	}
	return nil
}

func requireInt(name string, v interpreter.Value, argpos string) (int, error) {
	if v.Kind != interpreter.KindInt {
		return 0, fmt.Errorf("lists.%s: %s must be int, got %s", name, argpos, v.Kind)
	}
	return int(v.Int), nil
}

// pushFn returns a new list with `item` appended. The element-type
// check at use-site assignment will reject mismatches when the result
// is written back to a typed binding.
func pushFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("lists.push expects 2 arguments (list, item), got %d", len(args))
	}
	if err := requireList("push", args[0], "first argument"); err != nil {
		return interpreter.Null(), err
	}
	out := args[0].Copy()
	out.List = append(out.List, args[1].Copy())
	return out, nil
}

// popFn returns a new list with its last element removed. Empty input
// errors - "strict at boundaries".
func popFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("lists.pop expects 1 argument, got %d", len(args))
	}
	if err := requireList("pop", args[0], "argument"); err != nil {
		return interpreter.Null(), err
	}
	if len(args[0].List) == 0 {
		return interpreter.Null(), fmt.Errorf("lists.pop: cannot pop from an empty list")
	}
	out := args[0].Copy()
	out.List = out.List[:len(out.List)-1]
	return out, nil
}

// firstFn returns the element at index 0. Empty input errors.
func firstFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("lists.first expects 1 argument, got %d", len(args))
	}
	if err := requireList("first", args[0], "argument"); err != nil {
		return interpreter.Null(), err
	}
	if len(args[0].List) == 0 {
		return interpreter.Null(), fmt.Errorf("lists.first: list is empty")
	}
	return args[0].List[0].Copy(), nil
}

// lastFn returns the element at index len-1. Empty input errors.
func lastFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("lists.last expects 1 argument, got %d", len(args))
	}
	if err := requireList("last", args[0], "argument"); err != nil {
		return interpreter.Null(), err
	}
	if len(args[0].List) == 0 {
		return interpreter.Null(), fmt.Errorf("lists.last: list is empty")
	}
	return args[0].List[len(args[0].List)-1].Copy(), nil
}

// headFn returns the first `n` elements as a new list. `n` must be
// in `[0, len(xs)]`; out-of-range errors. Modeled on `head -n N` -
// callers wanting "just the first element" use lists.first.
func headFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("lists.head expects 2 arguments (list, n), got %d", len(args))
	}
	if err := requireList("head", args[0], "first argument"); err != nil {
		return interpreter.Null(), err
	}
	n, err := requireInt("head", args[1], "count")
	if err != nil {
		return interpreter.Null(), err
	}
	if n < 0 || n > len(args[0].List) {
		return interpreter.Null(), fmt.Errorf("lists.head: count %d out of range [0, %d]", n, len(args[0].List))
	}
	out := args[0].Copy()
	out.List = append(out.List[:0:0], args[0].List[:n]...)
	for i := range out.List {
		out.List[i] = args[0].List[i].Copy()
	}
	return out, nil
}

// tailFn returns the last `n` elements as a new list. Symmetric with
// headFn; for "everything except the first element" use
// `lists.tail($xs, len($xs) - 1)`.
func tailFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("lists.tail expects 2 arguments (list, n), got %d", len(args))
	}
	if err := requireList("tail", args[0], "first argument"); err != nil {
		return interpreter.Null(), err
	}
	n, err := requireInt("tail", args[1], "count")
	if err != nil {
		return interpreter.Null(), err
	}
	total := len(args[0].List)
	if n < 0 || n > total {
		return interpreter.Null(), fmt.Errorf("lists.tail: count %d out of range [0, %d]", n, total)
	}
	out := args[0].Copy()
	out.List = make([]interpreter.Value, n)
	for i := 0; i < n; i++ {
		out.List[i] = args[0].List[total-n+i].Copy()
	}
	return out, nil
}

// reverseFn returns a new list with elements in reverse order.
func reverseFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("lists.reverse expects 1 argument, got %d", len(args))
	}
	if err := requireList("reverse", args[0], "argument"); err != nil {
		return interpreter.Null(), err
	}
	src := args[0].List
	out := args[0].Copy()
	out.List = make([]interpreter.Value, len(src))
	for i, v := range src {
		out.List[len(src)-1-i] = v.Copy()
	}
	return out, nil
}

// sortFn returns a new list sorted in ascending order. The element
// kind decides comparison: int and float compare numerically (with
// mixed int/float allowed and promoted), string compares
// lexicographically, bool sorts false < true. Other element kinds,
// or a list mixing incompatible kinds, error.
func sortFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("lists.sort expects 1 argument, got %d", len(args))
	}
	if err := requireList("sort", args[0], "argument"); err != nil {
		return interpreter.Null(), err
	}
	out := args[0].Copy()
	if len(out.List) < 2 {
		return out, nil
	}
	if err := validateSortable(out.List); err != nil {
		return interpreter.Null(), err
	}
	sort.SliceStable(out.List, func(i, j int) bool {
		return less(out.List[i], out.List[j])
	})
	return out, nil
}

// validateSortable enforces that every element has a comparable kind
// and that mixed-kind lists are either all-numeric or all-same-kind.
// Catches the error up front rather than per-comparison.
func validateSortable(xs []interpreter.Value) error {
	allNumeric := true
	firstKind := xs[0].Kind
	for _, v := range xs {
		switch v.Kind {
		case interpreter.KindInt, interpreter.KindFloat:
			// numeric
		case interpreter.KindString, interpreter.KindBool:
			allNumeric = false
		default:
			return fmt.Errorf("lists.sort: cannot sort element of kind %s", v.Kind)
		}
		if v.Kind != firstKind {
			if !(allNumeric && (v.Kind == interpreter.KindInt || v.Kind == interpreter.KindFloat)) {
				return fmt.Errorf("lists.sort: cannot sort mixed-kind list (saw %s and %s)", firstKind, v.Kind)
			}
		}
	}
	return nil
}

func less(a, b interpreter.Value) bool {
	if a.Kind == interpreter.KindString {
		return a.Str < b.Str
	}
	if a.Kind == interpreter.KindBool {
		return !a.Bool && b.Bool
	}
	af, _ := a.AsFloat()
	bf, _ := b.AsFloat()
	return af < bf
}

// containsFn reports whether `item` appears in `xs` under structural
// equality. Argument order is (haystack, needle), matching
// `strings.contains` - PHP's `in_array(needle, haystack)` order is
// deliberately not adopted.
func containsFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("lists.contains expects 2 arguments (list, item), got %d", len(args))
	}
	if err := requireList("contains", args[0], "first argument"); err != nil {
		return interpreter.Null(), err
	}
	for _, v := range args[0].List {
		if v.Equal(args[1]) {
			return interpreter.BoolVal(true), nil
		}
	}
	return interpreter.BoolVal(false), nil
}

// concatFn returns a new list with `a`'s elements followed by `b`'s.
// The resulting element-type comes from `a` (the assignment at the
// call site will type-check it against the destination binding).
func concatFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("lists.concat expects 2 arguments, got %d", len(args))
	}
	if err := requireList("concat", args[0], "first argument"); err != nil {
		return interpreter.Null(), err
	}
	if err := requireList("concat", args[1], "second argument"); err != nil {
		return interpreter.Null(), err
	}
	out := args[0].Copy()
	out.List = make([]interpreter.Value, 0, len(args[0].List)+len(args[1].List))
	for _, v := range args[0].List {
		out.List = append(out.List, v.Copy())
	}
	for _, v := range args[1].List {
		out.List = append(out.List, v.Copy())
	}
	return out, nil
}

// sliceFn returns a sublist `[start, end)`. Two-arg call:
// `slice(xs, start)` extracts from `start` to the end - matches
// `strings.substring`'s optional-end shape.
func sliceFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 && len(args) != 3 {
		return interpreter.Null(), fmt.Errorf("lists.slice expects 2 or 3 arguments (list, start[, end]), got %d", len(args))
	}
	if err := requireList("slice", args[0], "first argument"); err != nil {
		return interpreter.Null(), err
	}
	start, err := requireInt("slice", args[1], "start")
	if err != nil {
		return interpreter.Null(), err
	}
	total := len(args[0].List)
	end := total
	if len(args) == 3 {
		end, err = requireInt("slice", args[2], "end")
		if err != nil {
			return interpreter.Null(), err
		}
	}
	if start < 0 || start > total {
		return interpreter.Null(), fmt.Errorf("lists.slice: start %d out of range [0, %d]", start, total)
	}
	if end < start || end > total {
		return interpreter.Null(), fmt.Errorf("lists.slice: end %d out of range [%d, %d]", end, start, total)
	}
	out := args[0].Copy()
	out.List = make([]interpreter.Value, end-start)
	for i := start; i < end; i++ {
		out.List[i-start] = args[0].List[i].Copy()
	}
	return out, nil
}
