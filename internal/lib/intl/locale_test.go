// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package intllib

import (
	"bytes"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// TestNormalizeLocale: the one rule load, setLocale and formatNumber share.
func TestNormalizeLocale(t *testing.T) {
	good := map[string]string{
		"de":          "de",
		"DE":          "de",
		"de_at":       "de-AT",
		"De-aT":       "de-AT",
		"zh_hant_tw":  "zh-Hant-TW",
		"es-419":      "es-419",
		"en-US-POSIX": "en-US-posix",
		"sr-latn":     "sr-Latn",
	}
	for in, want := range good {
		got, err := normalizeLocale("intl.x", in)
		if err != nil || got != want {
			t.Errorf("normalizeLocale(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "d", "de-", "-de", "de--AT", "de.AT", "de AT", "1de", "de-toolongsubtag", "deutschland"} {
		if got, err := normalizeLocale("intl.x", in); err == nil {
			t.Errorf("normalizeLocale(%q) = %q, want an error", in, got)
		}
	}
}

// TestSetLocaleNormalizesAndRefusesUnknown: setLocale stores the normalized
// tag, accepts a locale whose language has a catalog, and refuses one with no
// catalog or a malformed tag; load uses the same rule, so "de_at" and "de-AT"
// are one catalog.
func TestSetLocaleNormalizesAndRefusesUnknown(t *testing.T) {
	c := newCat()
	mustLoad(t, c, "EN", catalog("k", "en"))
	mustLoad(t, c, "de_at", catalog("k", "de-AT"))
	set := func(tag string) error {
		_, err := c.setLocaleFn(noCtx, []interpreter.Value{interpreter.StringVal(tag)})
		return err
	}
	if err := set("DE-at"); err != nil {
		t.Fatal(err)
	}
	if lv, _ := c.localeFn(noCtx, nil); lv.Str() != "de-AT" {
		t.Errorf("locale = %q, want de-AT", lv.Str())
	}
	if got := tr(t, c, interpreter.StringVal("k")); got != "de-AT" {
		t.Errorf("tr = %q, want de-AT", got)
	}
	if err := set("en_GB"); err != nil {
		t.Errorf("en-GB (language en loaded): %v", err)
	}
	for _, tag := range []string{"fr", "de", "", "en."} {
		if err := set(tag); err == nil {
			t.Errorf("setLocale(%q) accepted", tag)
		}
	}
	if lv, _ := c.localeFn(noCtx, nil); lv.Str() != "en-GB" {
		t.Errorf("a refused setLocale changed the locale to %q", lv.Str())
	}
}

// TestMisuseKindRuntime: every misuse in intl is kind "runtime", caught
// through a program.
func TestMisuseKindRuntime(t *testing.T) {
	calls := []string{
		`intl.setLocale("fr")`,
		`intl.setLocale("de..AT")`,
		`intl.load("", {})`,
		`intl.formatNumber("xx", 1, 0)`,
		`intl.formatNumber("de_", 1, 0)`,
		`intl.formatNumber("en", 1, 21)`,
	}
	for _, call := range calls {
		in := interpreter.New()
		var buf bytes.Buffer
		in.Out = &buf
		iolib.Install(in)
		Install(in)
		prog, err := parser.Parse(`use io;
use intl;
intl.load("en", {"k": "v"});
try {
    ` + call + `;
    io.printf("no error");
} catch (e) {
    io.printf("%s", $e.kind);
}`)
		if err != nil {
			t.Fatalf("%s: parse: %v", call, err)
		}
		if err := in.Run(prog); err != nil {
			t.Fatalf("%s: run: %v", call, err)
		}
		if buf.String() != "runtime" {
			t.Errorf("%s: kind %q, want runtime", call, buf.String())
		}
	}
}
