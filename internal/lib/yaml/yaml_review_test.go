// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package yamllib

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/lib/doctree"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// A value whose text does not fit its explicit tag is an error, not a string.
func TestTaggedScalarMustFitItsTag(t *testing.T) {
	for _, src := range []string{
		"a: !!binary '@@@'", "a: !!float abc", "a: !!int abc", "a: !!bool maybe",
		"a: !!null x", "a: !!timestamp nope", "a: !!map [1]", "a: !!seq {b: 1}",
		"a: !!str {b: 1}", "a: !!int [1]", "a: !!map x",
	} {
		if _, err := decodeYaml(src); err == nil {
			t.Errorf("decode(%q): want an error", src)
		}
	}
	// What still decodes: well-formed tagged values, custom tags (their
	// scalar is the string), the !!set / !!omap collection tags.
	for src, want := range map[string]string{
		"a: !!int '42'":                 "int",
		"a: !!float 1":                  "float",
		"a: !!bool 'true'":              "bool",
		"a: !!null ''":                  "null",
		"a: !!binary aGk=":              "bytes",
		"a: !!timestamp 2001-12-14":     "datetime",
		"a: !Ref thing":                 "string",
		"a: !!str 12":                   "string",
		"a: !!set {x, y}":               "map",
		"a: !!omap [{x: 1}]":            "list",
		"a: !!int 99999999999999999999": "string",
	} {
		v, err := decodeYaml(src)
		if err != nil {
			t.Errorf("decode(%q): %v", src, err)
			continue
		}
		if got := v.Elem(0).TypeName(); got != want {
			t.Errorf("decode(%q) is a %s, want %s", src, got, want)
		}
	}
}

// yaml.v3 reads these the YAML 1.1 way; YAML 1.2 reads them differently or
// as strings. Each is an error that names the fix; quoting gives the string.
func TestYAML11NumbersAreErrors(t *testing.T) {
	for _, s := range []string{"012", "-012", "00", "01234", "0b11", "-0b11", "1_000", "1_0.5", "0X1F", "0O17", "+0x1F", "-0o17"} {
		_, err := decodeYaml("a: " + s)
		if err == nil || !strings.Contains(err.Error(), "YAML 1.1") {
			t.Errorf("decode(a: %s): err = %v, want the YAML 1.1 error", s, err)
		}
		v, err := decodeYaml("a: '" + s + "'")
		if err != nil || v.Elem(0).Str() != s {
			t.Errorf("quoted %s: %v", s, err)
		}
	}
	for s, want := range map[string]string{
		"0": "int", "-0": "int", "+12": "int", "0x1F": "int", "0o17": "int", "12": "int",
		"0.5": "float", "012.5": "float", "1e3": "float", ".5": "float", "-.inf": "float",
	} {
		v, err := decodeYaml("a: " + s)
		if err != nil {
			t.Errorf("decode(a: %s): %v", s, err)
			continue
		}
		if got := v.Elem(0).TypeName(); got != want {
			t.Errorf("decode(a: %s) is a %s, want %s", s, got, want)
		}
	}
	// The encoder quotes such strings, so they round-trip.
	for _, s := range []string{"012", "0b11", "1_000"} {
		out, err := encodeNode(doctree.Map([]string{"a"}, []doctree.Node{doctree.String(s)}), false)
		if err != nil {
			t.Fatal(err)
		}
		v, err := decodeYaml(out)
		if err != nil || v.Elem(0).Str() != s {
			t.Errorf("string %q encoded as %q: %v", s, out, err)
		}
	}
}

// An alias shares its anchor's conversion: a block of defaults reused many
// times converts once, while the node budget still counts every expansion.
func TestAliasesShareTheirAnchor(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("defaults: &d {a: 1, b: [1, 2, 3], c: {x: y}}\nuses:\n")
	for i := 0; i < 1000; i++ {
		sb.WriteString("  - *d\n")
	}
	v, err := decodeYaml(sb.String())
	if err != nil {
		t.Fatal(err)
	}
	uses := v.Elem(1)
	if uses.Len() != 1000 || !uses.Elem(0).Equal(v.Elem(0)) || !uses.Elem(999).Equal(v.Elem(0)) {
		t.Fatalf("aliases decode wrong: %d uses", uses.Len())
	}
	// The budget still sees the expansion.
	saved := maxNodes
	maxNodes = 5000
	defer func() { maxNodes = saved }()
	if _, err := decodeYaml(sb.String()); err == nil || !strings.Contains(err.Error(), "too many nodes") {
		t.Errorf("1000 aliases of a 9-node anchor under a 5000 budget: err = %v", err)
	}
}

// Nesting built through aliases is held to the same per-binary limit as any
// other decode, though the text itself is shallow.
func TestAliasNestingHeldToTheLimit(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("a0: &a0 [x]\n")
	for i := 1; i <= limits.MaxNestingDepth+5; i++ {
		sb.WriteString("a" + itoa(i) + ": &a" + itoa(i) + " [*a" + itoa(i-1) + "]\n")
	}
	if _, err := decodeYaml(sb.String()); err == nil || !strings.Contains(err.Error(), "nesting too deep") {
		t.Errorf("alias-built nesting past %d levels: err = %v", limits.MaxNestingDepth, err)
	}
}

func TestEncodeFloatForm(t *testing.T) {
	for f, want := range map[float64]string{1e6: "1000000.0", 123456789: "123456789.0", 1e-7: "1e-7", 1e21: "1e+21", 2.5: "2.5"} {
		if got := formatYamlFloat(f); got != want {
			t.Errorf("formatYamlFloat(%v) = %s, want %s", f, got, want)
		}
	}
}
