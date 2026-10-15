// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package jsonlib

import (
	"math/rand"
	"strconv"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lib/doctree"
)

// A large map keeps its sorted lookup index in step with its members through
// every write: keys added (in random order), replaced and removed must all stay
// findable, and removed ones must not.
func TestMapIndexMaintainedAcrossWrites(t *testing.T) {
	ctx := interpreter.BuiltinCtx{}
	d := wrap(doctree.Map(nil, nil))
	want := map[string]int64{}
	r := rand.New(rand.NewSource(1))
	for _, i := range r.Perm(200) {
		k := "k" + strconv.Itoa(i)
		var err error
		d, err = setFn(ctx, []interpreter.Value{d, interpreter.StringVal("/" + k), interpreter.IntVal(int64(i))})
		if err != nil {
			t.Fatal(err)
		}
		want[k] = int64(i)
	}
	for _, i := range r.Perm(200)[:80] {
		k := "k" + strconv.Itoa(i)
		var err error
		if i%2 == 0 {
			d, err = removeFn(ctx, []interpreter.Value{d, interpreter.StringVal("/" + k)})
			delete(want, k)
		} else {
			d, err = setFn(ctx, []interpreter.Value{d, interpreter.StringVal("/" + k), interpreter.IntVal(-1)})
			want[k] = -1
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	n, _ := asNode(d)
	if n.Len() != len(want) {
		t.Fatalf("length %d, want %d", n.Len(), len(want))
	}
	for i := 0; i < 200; i++ {
		k := "k" + strconv.Itoa(i)
		got, err := asIntFn(ctx, []interpreter.Value{d, interpreter.StringVal("/" + k)})
		w, present := want[k]
		switch {
		case present && (err != nil || got.Int != w):
			t.Errorf("%s: got %v (err %v), want %d", k, got, err, w)
		case !present && err == nil:
			t.Errorf("%s: removed key still found", k)
		}
	}
}

// Objects nested inside lists inside objects (and the reverse) decode with
// their own members: the shared key / element stacks must stay aligned.
func TestDecodeMixedNesting(t *testing.T) {
	src := `{"a": [1, {"x": [2, 3], "y": {"z": [4, {"w": 5}]}}, 6], "b": {"c": [], "d": {}}, "e": [[{"f": 7}]]}`
	n, err := decodeFn([]interpreter.Value{interpreter.StringVal(src)})
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	if err := encodeNode(&sb, n, false, 0); err != nil {
		t.Fatal(err)
	}
	if want := strings.ReplaceAll(src, " ", ""); sb.String() != want {
		t.Errorf("round-trip:\n got %s\nwant %s", sb.String(), want)
	}
}

// Escaped strings decode through the slow path; unescaped ones are slices of
// the input. Both must read back exactly.
func TestDecodeStringPaths(t *testing.T) {
	n, err := decodeFn([]interpreter.Value{interpreter.StringVal(`{"plain": "abc", "esc": "a\"b\\c\ndé", "": ""}`)})
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{"plain": "abc", "esc": "a\"b\\c\ndé", "": ""} {
		if got := n.Elem(n.Find(k)).Str(); got != want {
			t.Errorf("%q = %q, want %q", k, got, want)
		}
	}
}

// json.Value equality follows `==`: map member order does not matter, numbers
// compare exactly across int / float, and lists are ordered.
func TestNodeEquality(t *testing.T) {
	eq := func(a, b string) bool {
		x, err := decodeFn([]interpreter.Value{interpreter.StringVal(a)})
		if err != nil {
			t.Fatal(err)
		}
		y, err := decodeFn([]interpreter.Value{interpreter.StringVal(b)})
		if err != nil {
			t.Fatal(err)
		}
		return wrap(x).Equal(wrap(y))
	}
	cases := []struct {
		a, b string
		want bool
	}{
		{`{"a": 1, "b": [true, null]}`, `{"b": [true, null], "a": 1}`, true},
		{`[1, 2]`, `[2, 1]`, false},
		{`1`, `1.0`, true},
		{`1`, `1.5`, false},
		{`{"a": 1}`, `{"a": 1, "b": 2}`, false},
		{`"x"`, `"y"`, false},
		{`null`, `null`, true},
	}
	for _, c := range cases {
		if got := eq(c.a, c.b); got != c.want {
			t.Errorf("%s == %s: %t, want %t", c.a, c.b, got, c.want)
		}
	}
}

// A declared-but-unset json.Value is JSON null and equals a decoded null.
func TestZeroValueIsNull(t *testing.T) {
	zero := lib.Zero()
	decoded, err := decodeFn([]interpreter.Value{interpreter.StringVal("null")})
	if err != nil {
		t.Fatal(err)
	}
	if !zero.Equal(wrap(decoded)) {
		t.Error("the zero json.Value should equal json.decode(\"null\")")
	}
}
