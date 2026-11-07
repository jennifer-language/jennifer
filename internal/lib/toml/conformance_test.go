// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package tomllib

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"jennifer-lang.dev/jennifer/internal/lib/doctree"
)

// The toml-test 1.0.0 invalid documents the decoder used to accept (the
// suite's file name in the comment), and valid neighbours that must still
// decode.
func TestDecodeRejectsTomlTestInvalid(t *testing.T) {
	for _, src := range []string{
		"[[tab.arr]]\n[tab]\narr.val1=1\n",                                                     // array/extend-defined-aot
		"[[a.b]]\n\n[a]\nb.y = 2\n",                                                            // table/append-with-dotted-keys-03
		"[a.b.c]\nz = 9\n\n[a]\nb.c.t = 1\n",                                                   // table/append-with-dotted-keys-01
		"[a.b.c.d]\nz = 9\n\n[a]\nb.c.d.k.t = 1\n",                                             // table/append-with-dotted-keys-02
		"[a.b.c]\nz = 9\n[[x]]\ny = 1\n[a]\nb.c.t = 1\n",                                       // table/append-with-dotted-keys-08
		"[fruit]\napple.color = \"red\"\n\n[fruit.apple]\n",                                    // table/duplicate-key-04, spec table-9-0
		"[fruit]\napple.taste.sweet = true\n[fruit.apple.taste]\n",                             // table/duplicate-key-05, spec table-9-1
		"[t1]\nt2.t3.v = 0\n[t1.t2]\n",                                                         // table/redefine-02
		"[t1]\nt2.t3.v = 0\n[t1.t2.t3]\n",                                                      // table/redefine-03
		"tab = { inner = { dog = \"best\" }, inner.cat = \"worst\" }",                          // inline-table/overwrite-08
		"tbl = { fruit = { apple.color = \"red\" }, fruit.apple.texture = { smooth = true } }", // inline-table/duplicate-key-03
		"a = 0x-1", "a = 0o+7", "a = 0b-1", // integer/invalid-hex-03
		"a = 0X1F", "a = 0O7", "a = 0B1", // integer/capital-*
		"d = 1985-06-18 17:04:07+12:60", // datetime/offset-overflow-minute
		"d = 1985-06-18 17:04:07+24:00",
		"# c\r\n\ra = 1",             // control/bare-cr
		"a = 1\r",                    // bare CR ending a statement
		"# a\rb\na = 1",              // control/comment-cr
		"# a\x7f\na = 1",             // control/comment-del
		"# a\x0c\na = 1",             // control/comment-ff
		"# a\x00\na = 1",             // control/comment-null
		"# a\x1f\na = 1",             // control/comment-us
		"a = [1, # \x01\n 2]",        // control character in an array comment
		"# U+D800 \xed\xa0\x80\na=1", // encoding/bad-codepoint
		"a = \"\xff\"",               // encoding/bad-utf8-in-string
	} {
		if _, err := decodeToml(src); err == nil {
			t.Errorf("accepted invalid %q", src)
		}
	}
	for _, src := range []string{
		"[fruit]\napple.color = \"red\"\n[fruit.apple.texture]\nsmooth = true\n", // spec: sub-table of a dotted table
		"[a.b.c]\nz = 9\n[a]\nx = 1\n",
		"[a]\nb.c = 1\nb.d = 2\n",
		"t = { a.b = 1, a.c = 2 }",
		"a = 0xDEADbeef\nb = 0o17\nc = 0b101",
		"d = 1985-06-18 17:04:07+23:59",
		"# tab\tok\r\na = 1\r\n",
		"[[x]]\na.b = 1\n[[x]]\na.b = 2\n",
	} {
		if _, err := decodeToml(src); err != nil {
			t.Errorf("rejected valid %q: %v", src, err)
		}
	}
}

// DEL is escaped on encode: the decoder rejects it raw, so the output must
// round-trip.
func TestEncodeEscapesDEL(t *testing.T) {
	m := doctree.Map([]string{"a"}, []node{doctree.String("x\x7fy")})
	out, err := encodeToml(lib.Wrap(m), false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `\u007f`) {
		t.Errorf("encode = %q, want a \\u007f escape", out)
	}
	back, err := decodeToml(out)
	if err != nil || !back.Equal(m) {
		t.Errorf("round-trip: %v", err)
	}
}

// A dotted key of k segments costs O(k) state lookups, not O(k^3): 1 MB of
// 990-segment keys under an array of tables decoded in ~8 s before.
func TestDottedKeyCostLinear(t *testing.T) {
	prefix := strings.Repeat("a.", 990)
	var sb strings.Builder
	sb.WriteString("[[x]]\n")
	for i := 0; sb.Len() < 1<<20; i++ {
		sb.WriteString(prefix)
		sb.WriteString("b")
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString(" = 1\n")
	}
	start := time.Now()
	if _, err := decodeToml(sb.String()); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("1 MB of 990-segment dotted keys took %v", d)
	}
}
