// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package yamllib

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lib/doctree"
)

// A plain integer of any length, and a hex / octal one past int64, decodes
// as an integer (never a string), as json's does.
func TestPlainBigIntegers(t *testing.T) {
	nines := strings.Repeat("9", 400)
	for src, want := range map[string]string{
		"x: " + nines:                         nines,
		"x: -" + nines:                        "-" + nines,
		"x: +" + nines:                        nines,
		"x: 123456789012345678901234567890":   "123456789012345678901234567890",
		"x: 0x1234567890123456789012345":      "90144042670598483612836897605",
		"x: 0xFFFFFFFFFFFFFFFF":               "18446744073709551615",
		"x: 0o7777777777777777777777777777":   "19342813113834066795298815",
		"x: !!int " + nines:                   nines,
		"x: !!int 0xffffffffffffffffffffffff": "79228162514264337593543950335",
	} {
		v, err := decodeYaml(src)
		if err != nil {
			t.Errorf("decode(%.60q): %v", src, err)
			continue
		}
		if n := v.Elem(0); n.Kind() != doctree.KindBigInt || n.Str() != want {
			t.Errorf("decode(%.60q) = %s %.60q, want int %.60q", src, n.TypeName(), n.Str(), want)
		}
	}
	// Still strings: quoted digits, text that is no YAML 1.2 number.
	for _, src := range []string{"x: '" + nines + "'", "x: 0o9", "x: 0xg1", "x: 1.2.3", "x: 12abc"} {
		v, err := decodeYaml(src)
		if err != nil || v.Elem(0).Kind() != doctree.KindString {
			t.Errorf("decode(%.60q): %v, want a string", src, err)
		}
	}
	// A float past float64's range is an error, plain or tagged, and so are
	// a long leading-zero integer and separators in one.
	for _, src := range []string{"x: 1e400", "x: -1.5e999", "x: .5e999", "x: !!float 1e400", "x: 0" + nines, "x: 1_" + nines, "x: !!float " + nines} {
		_, err := decodeYaml(src)
		if dataerr.KindOf(err) != "yaml" || len(err.Error()) > 300 {
			t.Errorf("decode(%.60q): %.300v, want a short yaml error", src, err)
		}
	}
	// A hex integer too long to convert cheaply is a reached cap.
	if _, err := decodeYaml("x: 0x" + strings.Repeat("f", maxRadixDigits+1)); dataerr.KindOf(err) != "limit" {
		t.Errorf("long hex: %v, want limit", err)
	}
}

// An explicit !!float is the author's: a float even when its text is an
// integer past int64.
func TestExplicitFloatTagIsFloat(t *testing.T) {
	v, err := decodeYaml("x: !!float 123456789012345678901234567890\ny: 123456789012345678901234567890")
	if err != nil {
		t.Fatal(err)
	}
	if v.Elem(0).Kind() != doctree.KindFloat || v.Elem(0).FloatVal() != 1.2345678901234568e29 {
		t.Errorf("x = %s", v.Elem(0).TypeName())
	}
	if v.Elem(1).Kind() != doctree.KindBigInt {
		t.Errorf("y = %s", v.Elem(1).TypeName())
	}
}

// A big integer encodes as plain untagged digits (YAML 1.2 !!int, as every
// parser reads it) and decodes back exactly, in both styles.
func TestBigIntegerEncodeRoundTrip(t *testing.T) {
	for _, digits := range []string{"123456789012345678901234567890", "-" + strings.Repeat("8", 400)} {
		doc := doctree.Map([]string{"x"}, []doctree.Node{doctree.BigInt(digits)})
		for _, flow := range []bool{true, false} {
			out, err := encodeNode(doc, flow)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(out, "!!") || strings.ContainsAny(out, `'"`) {
				t.Errorf("encode (flow %v) = %.80q, want plain digits", flow, out)
			}
			back, err := decodeYaml(out)
			if err != nil || !back.Equal(doc) {
				t.Errorf("round trip (flow %v): %v", flow, err)
			}
		}
	}
}

// A timestamp with a time part round-trips through flow style (yaml.encode)
// as a datetime, not a string; block style keeps it plain.
func TestDatetimeFlowRoundTrip(t *testing.T) {
	for _, ts := range []string{"2001-12-14T21:59:43.10-05:00", "2001-12-14T21:59:43Z", "2002-12-14"} {
		doc, err := decodeYaml("x: " + ts)
		if err != nil || doc.Elem(0).Kind() != doctree.KindDatetime {
			t.Fatalf("decode %q: %v", ts, err)
		}
		for _, flow := range []bool{true, false} {
			out, err := encodeNode(doc, flow)
			if err != nil {
				t.Fatal(err)
			}
			back, err := decodeYaml(out)
			if err != nil || back.Elem(0).Kind() != doctree.KindDatetime || back.Elem(0).Str() != ts {
				t.Errorf("flow %v: %q decodes to %v (%v)", flow, out, back.Elem(0).TypeName(), err)
			}
		}
	}
}

// decodeWith's messages, its caps' included, name decodeWith.
func TestDecodeWithMessagesNameIt(t *testing.T) {
	s := interpreter.StringVal
	for _, doc := range []string{"a:\n  b:\n    c: 1\n", "a: [1, 2, 3, 4, 5]\n", "a: 1\na: 2\n", "x: !!int abc\n", "x: 012\n"} {
		_, err := yamlCall(t, "decodeWith", s(doc), yamlOpts(2, 4))
		if err == nil || !strings.HasPrefix(err.Error(), "yaml.decodeWith: ") {
			t.Errorf("%q: %v", doc, err)
		}
	}
}
