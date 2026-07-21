// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import "testing"

// A generic collection - a fresh literal, which records no element type -
// must be validated entry by entry against the declared element type, at
// every binding boundary, so a mismatched or heterogeneous collection can't
// be relabeled as matching. (json.decode results are no longer generic
// collections: they come back as an opaque json.Value the accessors walk, so
// literals are the remaining generic source and stand in here.)

func TestGenericCollectionRejectedAtDefInit(t *testing.T) {
	cases := []string{
		`def x as map of string to string init {"a": 1};`,
		`def x as map of string to int init {"a": "s"};`,
		`def x as list of string init [1, 2];`,
		`def x as list of int init [1, "s"];`,                    // heterogeneous
		`def x as map of string to int init {"a": 1, "b": "s"};`, // heterogeneous
	}
	for _, src := range cases {
		if _, err := run(t, src); err == nil {
			t.Errorf("expected a type error for %q", src)
		}
	}
}

func TestGenericCollectionRejectedAtAssign(t *testing.T) {
	cases := []string{
		`def x as map of string to string; $x = {"a": 1};`,
		`def x as list of int; $x = ["s"];`,
		`def x as map of string to int init {"a": 1}; $x = {"b": "s"};`,
	}
	for _, src := range cases {
		if _, err := run(t, src); err == nil {
			t.Errorf("expected a type error for %q", src)
		}
	}
}

func TestHomogeneousCollectionBinds(t *testing.T) {
	cases := []string{
		`use io; def x as map of string to int init {"a": 1, "b": 2}; io.printf("%d", $x["a"]);`,
		`use io; def x as list of string init ["a", "b"]; io.printf("%s", $x[0]);`,
		`use io; def x as list of list of int init [[1], [2, 3]]; io.printf("%d", $x[1][1]);`,
		// empty collections stay assignable to any declared collection type
		`use io; def x as map of string to int init {}; def y as list of string init []; io.printf("ok");`,
		`use io; def x as list of int; $x = [1, 2, 3]; io.printf("%d", $x[2]);`,
	}
	for _, src := range cases {
		if _, err := run(t, src); err != nil {
			t.Errorf("expected a clean bind for %q, got %v", src, err)
		}
	}
}

// Elements stored via index-write and append must be stamped with the
// container's declared element / value type. A nested container stored
// unstamped carries no ElemTyp/ValTyp, so later writes into it would skip
// the declared-type check and a string could land inside a
// `list of list of int`.
func TestNestedStoreStampsElementType(t *testing.T) {
	cases := []string{
		// append a nested list, then write a mismatched element into it
		`def grid as list of list of int init [[1]]; $grid[] = [9]; $grid[1][0] = "oops";`,
		// index-write a nested list, then write a mismatched element into it
		`def grid as list of list of int init [[1], [2]]; $grid[1] = [9]; $grid[1][0] = "oops";`,
		// map value: store a nested list under a key, then poison it
		`def m as map of string to list of int init {"a": [1]}; $m["b"] = [9]; $m["b"][0] = "oops";`,
		// map value update of an existing key, then poison it
		`def m as map of string to list of int init {"a": [1]}; $m["a"] = [9]; $m["a"][0] = "oops";`,
		// nested map stored by index-write, then wrong value type inside
		`def mm as map of string to map of string to int init {}; $mm["x"] = {"k": 1}; $mm["x"]["k"] = "oops";`,
	}
	for _, src := range cases {
		if _, err := run(t, src); err == nil {
			t.Errorf("expected a type error for %q", src)
		}
	}
	// The stamped stores still accept well-typed writes.
	ok := []string{
		`use io; def grid as list of list of int init [[1]]; $grid[] = [9]; $grid[1][0] = 7; io.printf("%d", $grid[1][0]);`,
		`use io; def m as map of string to list of int init {}; $m["a"] = [9]; $m["a"][0] = 7; io.printf("%d", $m["a"][0]);`,
	}
	for _, src := range ok {
		if out, err := run(t, src); err != nil || out != "7" {
			t.Errorf("well-typed store failed for %q: out=%q err=%v", src, out, err)
		}
	}
}
