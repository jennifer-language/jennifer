// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package yamllib

import (
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// treeDepth is the collection nesting yaml.v3 built: how deep its recursive
// parser went. An alias is a leaf to the parser.
func treeDepth(n *yaml.Node) int {
	best := 0
	for _, c := range n.Content {
		if d := treeDepth(c); d > best {
			best = d
		}
	}
	if n.Kind == yaml.MappingNode || n.Kind == yaml.SequenceNode {
		best++
	}
	return best
}

// parsedDepth parses src with yaml.v3 (every document) and returns the
// deepest nesting, or ok=false when yaml.v3 rejects it.
func parsedDepth(src string) (int, bool) {
	dec := yaml.NewDecoder(strings.NewReader(src))
	best := 0
	for {
		var doc yaml.Node
		if err := dec.Decode(&doc); err != nil {
			return best, err.Error() == "EOF"
		}
		if d := treeDepth(&doc); d > best {
			best = d
		}
	}
}

func checkBound(t *testing.T, src string) {
	t.Helper()
	if !utf8.ValidString(src) {
		// yaml.v3 may read it as UTF-16; the guard refuses it instead.
		if guardDepth(src) == nil {
			t.Fatalf("guard passed invalid UTF-8 %q", src)
		}
		return
	}
	got, ok := parsedDepth(src)
	if !ok {
		return
	}
	if est := scanDepth(src, 1<<30); est < got {
		t.Fatalf("scanDepth = %d, yaml.v3 nests %d, for %q", est, got, src)
	}
}

// Documents the old quote-skipping guard under-counted: a quote inside a
// plain scalar hid the nesting after it.
func TestGuardSeesPastQuotesInPlainScalars(t *testing.T) {
	deep := strings.Repeat("[", 300) + strings.Repeat("]", 300)
	for _, src := range []string{
		"a: it's\nb: " + deep + "\nc: x'\n",
		"a: say \"hi\nb: " + deep + "\nc: \"\n",
		"[it's, " + deep + "]",
		"[x - 'y, " + deep + "]",
		"[a\n 'b, " + deep + "]",
		"a: rock 'n roll\nb: " + deep + "\nc: '\n",
		"a: x, 'y\nb: " + deep + "\nc: '\n",
		"k: !!str \"x, '\"\nb: " + deep + "\nc: '\n",
	} {
		if err := guardDepth(src); err == nil {
			if d, ok := parsedDepth(src); ok {
				t.Errorf("guard passed a document yaml.v3 nests %d deep: %.60q", d, src)
			} else {
				// yaml.v3 rejects it, but only after parsing the nesting
				// before the error; the bound still has to see it.
				t.Errorf("guard passed %.60q", src)
			}
		}
	}
	// yaml.v3's parser skips the `]` right after a `?` in a flow sequence, so
	// it nests one level per `[?],` while the brackets stay two deep.
	for _, src := range []string{
		"a: " + strings.Repeat("[?],", 300) + strings.Repeat("]", 300),
		strings.Repeat("[[?]:", 100) + strings.Repeat("]", 200),
		strings.Repeat("[?, : ", 100) + strings.Repeat("]", 100),
	} {
		if guardDepth(src) == nil {
			t.Errorf("guard passed %.40q", src)
		}
	}
	// Block nesting through indentation, compact sequences and keys.
	var sb strings.Builder
	for i := 0; i < 200; i++ {
		sb.WriteString(strings.Repeat(" ", i) + "k:\n")
	}
	sb.WriteString(strings.Repeat(" ", 200) + "v\n")
	if guardDepth(sb.String()) == nil {
		t.Error("guard passed 200 nested block mappings")
	}
	if guardDepth(strings.Repeat("- ", 200)+"x\n") == nil {
		t.Error("guard passed 200 compact sequences")
	}
}

// Ordinary documents, including the shapes that confused the old guard,
// stay under the limit.
func TestGuardAcceptsOrdinaryDocuments(t *testing.T) {
	docs := []string{
		"a: it's [not] a list\nb: [1, 2]\n",
		"pattern: \"[a-z\"\nother: '[[['\n" + strings.Repeat("re: \"^[a-z\"\n", 1),
		"text: |\n  - - - - [[[[ {{{{\n  more\nnext: 1\n",
		"# [[[[[[[[\na: 1 # {{{{{{\n",
	}
	for _, src := range docs {
		if err := guardDepth(src); err != nil {
			t.Errorf("guard rejected %q: %v", src, err)
		}
		checkBound(t, src)
	}
	// Many strings with unbalanced brackets do not add up.
	var sb strings.Builder
	for i := 0; i < 500; i++ {
		sb.WriteString("k" + strings.Repeat("x", i%7) + string(rune('a'+i%26)) + string(rune('a'+i/26%26)) + ": \"[a-z\"\n")
	}
	if err := guardDepth(sb.String()); err != nil {
		t.Errorf("guard rejected 500 regex strings: %v", err)
	}
}

// The bound holds against yaml.v3 itself over random documents built from
// the characters that decide YAML structure.
func TestScanDepthBoundsParser(t *testing.T) {
	alphabet := []string{
		"[", "]", "{", "}", ",", " ", "  ", "    ", "\n", "\n", "\n  ", "\n    ", "- ", "-", "? ", "?",
		": ", ":", "a", "b c", "x'y", "'", "\"", "''", "\\", "#", " #x", "|", ">", "|2-", ">+",
		"&x ", "*x", "!!str ", "!t ", "!<u> ", "---\n", "...\n", "\t", "\r\n", "\r", "é", "\u2028",
		"%YAML 1.2\n", "k: ", "- - ", "? - ", "[a, [b]]", "{k: [v]}", "\"[\"", "'{'",
	}
	n := 300000
	if testing.Short() {
		n = 20000
	}
	r := rand.New(rand.NewSource(1))
	var sb strings.Builder
	for k := 0; k < n; k++ {
		sb.Reset()
		for j := r.Intn(30) + 1; j > 0; j-- {
			sb.WriteString(alphabet[r.Intn(len(alphabet))])
		}
		checkBound(t, sb.String())
	}
}

func FuzzScanDepthBoundsParser(f *testing.F) {
	for _, s := range []string{"a: [b, {c: d}]", "- - x\n  - y", "a:\n- b\n- c: [d]", "k: |\n  x\n[y]", "? [a]\n: b"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		if len(src) > 4096 {
			return
		}
		checkBound(t, src)
	})
}
