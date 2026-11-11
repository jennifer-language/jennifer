// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package doctree

import (
	"fmt"
	"testing"
)

// wideMap is a map of n members, each a small object, as a store of records is.
func wideMap(n int) Node {
	keys := make([]string, n)
	elems := make([]Node, n)
	for i := range elems {
		keys[i] = fmt.Sprintf("k%d", i)
		elems[i] = Map([]string{"a", "b"}, []Node{Int(1), String("xxxxxxxxxxxxxxxx")})
	}
	return Map(keys, elems)
}

func benchSet(b *testing.B, n int, ptr string) {
	root := wideMap(n)
	tokens, err := ParsePointer("bench", ptr)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		root, err = Set("bench", root, tokens, Int(int64(i)))
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSetNested1k(b *testing.B)   { benchSet(b, 1_000, "/k0/a") }
func BenchmarkSetNested100k(b *testing.B) { benchSet(b, 100_000, "/k0/a") }
func BenchmarkSetTop100k(b *testing.B)    { benchSet(b, 100_000, "/k0") }
func BenchmarkAddTop100k(b *testing.B) {
	root := wideMap(100_000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Set("bench", root, []string{fmt.Sprintf("new%d", i)}, Int(1))
		if err != nil {
			b.Fatal(err)
		}
	}
}
