// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package stringslib

import (
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func TestFold(t *testing.T) {
	in := interpreter.New()
	Install(in)
	fold := in.LookupNamespacedBuiltin("strings", "fold")

	cases := []struct{ in, want string }{
		{"Österreich", "Osterreich"},
		{"café", "cafe"},
		{"naïve", "naive"},
		{"Straße", "Strasse"},
		{"Zürich", "Zurich"},
		{"El Niño", "El Nino"},
		{"Æsir", "AEsir"},
		{"œuvre", "oeuvre"},
		{"Þór", "Thor"},
		{"STRAẞE", "STRASSE"},                      // capital sharp-s folds like its lowercase ß
		{"ẞ", "SS"},                                // U+1E9E alone
		{"café", "cafe"},                          // NFD "café" (e + combining acute) -> combining mark dropped
		{"à́̂", "a"},                              // stacked NFD combining marks all stripped
		{"́", ""},                                  // bare combining mark folds to empty
		{"Hello, World! 123", "Hello, World! 123"}, // ASCII passes through
		{"", ""},
		{"日本語", "日本語"}, // non-Latin passes through unchanged
	}
	for _, c := range cases {
		v, err := fold(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal(c.in)})
		if err != nil {
			t.Errorf("fold(%q): err %v", c.in, err)
			continue
		}
		if v.Kind != interpreter.KindString || v.Str != c.want {
			t.Errorf("fold(%q) = %q, want %q", c.in, v.Str, c.want)
		}
	}

	if _, err := fold(interpreter.BuiltinCtx{}, nil); err == nil {
		t.Error("fold() with no args should error")
	}
	if _, err := fold(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(5)}); err == nil {
		t.Error("fold(int) should error")
	}
}

// TestFoldCanonicalEquivalence pins the point of the combining-mark stripping:
// the precomposed (NFC) and decomposed (NFD) spellings of the same accented
// word must fold to one key, and the two sharp-s cases must agree too.
func TestFoldCanonicalEquivalence(t *testing.T) {
	in := interpreter.New()
	Install(in)
	fold := in.LookupNamespacedBuiltin("strings", "fold")

	foldOf := func(s string) string {
		v, err := fold(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal(s)})
		if err != nil {
			t.Fatalf("fold(%q): %v", s, err)
		}
		return v.Str
	}

	pairs := []struct{ nfc, nfd string }{
		{"café", "café"},   // e + combining acute
		{"naïve", "naïve"}, // i + diaeresis
	}
	for _, p := range pairs {
		if got1, got2 := foldOf(p.nfc), foldOf(p.nfd); got1 != got2 {
			t.Errorf("fold NFC %q = %q, fold NFD %q = %q - want equal", p.nfc, got1, p.nfd, got2)
		}
	}

	// The documented lower(fold(s)) key recipe now buckets both sharp-s spellings
	// together (F-string-1): STRAẞE and STRASSE land on one key.
	lower := in.LookupNamespacedBuiltin("strings", "lower")
	key := func(s string) string {
		f, _ := fold(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal(s)})
		l, _ := lower(interpreter.BuiltinCtx{}, []interpreter.Value{f})
		return l.Str
	}
	if a, b := key("STRAẞE"), key("STRASSE"); a != b {
		t.Errorf("key(STRAẞE) = %q, key(STRASSE) = %q - want equal", a, b)
	}
}
