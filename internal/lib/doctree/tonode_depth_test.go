// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package doctree

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// nestValue wraps leaf in depth containers, cycling list / map / struct.
func nestValue(depth int, leaf interpreter.Value) interpreter.Value {
	v := leaf
	str := parser.PrimitiveType(parser.TypeString)
	for i := 0; i < depth; i++ {
		switch i % 3 {
		case 0:
			v = interpreter.ListVal(str, []interpreter.Value{v})
		case 1:
			v = interpreter.MapVal(str, str, []interpreter.MapEntry{{Key: interpreter.StringVal("k"), Value: v}})
		default:
			v = interpreter.NamespacedStructVal("", "N", []interpreter.StructField{{Name: "kids", Value: v}})
		}
	}
	return v
}

// ToNode bounds its recursion at limits.MaxNestingDepth with a catchable
// error (yaml.encode / toml.encode / json.list of a deep struct overflowed
// the Go stack, and corrupted the value on jennifer-tiny). A wrapped node
// counts with the containers around it; a time.Time leaf is a scalar.
func TestToNodeBoundsDepth(t *testing.T) {
	max := limits.MaxNestingDepth
	tm := interpreter.NamespacedStructVal("time", "Time", []interpreter.StructField{
		{Name: "nanos", Value: interpreter.IntVal(0)},
		{Name: "offset", Value: interpreter.IntVal(0)},
	})
	for _, leaf := range []interpreter.Value{interpreter.IntVal(1), tm} {
		n, err := testLib.ToNode(nestValue(max, leaf), false)
		if err != nil || n.Height() != max {
			t.Errorf("depth %d: height %d, %v", max, n.Height(), err)
		}
		_, err = testLib.ToNode(nestValue(max+1, leaf), false)
		if err == nil || !strings.Contains(err.Error(), "value nests deeper than the") {
			t.Errorf("depth %d: err = %v, want a nesting error", max+1, err)
		}
	}
	inner, err := testLib.ToNode(nestValue(max-2, interpreter.IntVal(1)), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testLib.ToNode(nestValue(2, testLib.Wrap(inner)), false); err != nil {
		t.Errorf("wrapped node at the limit: %v", err)
	}
	if _, err := testLib.ToNode(nestValue(3, testLib.Wrap(inner)), false); err == nil {
		t.Error("wrapped node past the limit converted")
	}
}
