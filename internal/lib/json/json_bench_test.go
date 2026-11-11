// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package jsonlib

import (
	"fmt"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// benchDoc is a document of n records mixing plain and escaped strings,
// HTML-sensitive text, ints, floats and nesting: the shapes of an API response
// or a log export.
func benchDoc(n int) string {
	var sb strings.Builder
	sb.WriteString(`{"items":[`)
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, `{"id":%d,"name":"user number %d","score":%d.25,`+
			`"bio":"line one\nline two \"quoted\" and a tab\there, more text after it",`+
			`"html":"<b>bold</b> & more","tags":["alpha","beta","gamma"],"ok":true,"none":null}`, i, i, i)
	}
	sb.WriteString(`]}`)
	return sb.String()
}

func BenchmarkDecode(b *testing.B) {
	text := interpreter.StringVal(benchDoc(2000))
	b.SetBytes(int64(len(text.Str)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := decodeFn([]interpreter.Value{text}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncode(b *testing.B) {
	text := benchDoc(2000)
	tree, err := decodeFn([]interpreter.Value{interpreter.StringVal(text)})
	if err != nil {
		b.Fatal(err)
	}
	v := wrap(tree)
	b.SetBytes(int64(len(text)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := encodeFn([]interpreter.Value{v}, false); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodePretty(b *testing.B) {
	tree, err := decodeFn([]interpreter.Value{interpreter.StringVal(benchDoc(2000))})
	if err != nil {
		b.Fatal(err)
	}
	v := wrap(tree)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := encodeFn([]interpreter.Value{v}, true); err != nil {
			b.Fatal(err)
		}
	}
}
