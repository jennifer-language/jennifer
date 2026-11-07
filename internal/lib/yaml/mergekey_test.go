// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package yamllib

import (
	"testing"

	"jennifer-lang.dev/jennifer/internal/lib/doctree"
)

// A literal `<<` key is quoted on encode, so it decodes back as a string key
// rather than a merge key (which would replace it with the inner map's keys).
func TestEncodeQuotesMergeKeyLiteral(t *testing.T) {
	inner := doctree.Map([]string{"a"}, []node{doctree.Int(1)})
	m := doctree.Map([]string{"<<", "b"}, []node{inner, doctree.Int(2)})
	for _, flow := range []bool{false, true} {
		out, err := encodeNode(m, flow)
		if err != nil {
			t.Fatal(err)
		}
		back, err := decodeYaml(out)
		if err != nil {
			t.Fatalf("decode %q: %v", out, err)
		}
		if !back.Equal(m) {
			t.Errorf("flow %v: %q decodes to a different document", flow, out)
		}
	}
}
