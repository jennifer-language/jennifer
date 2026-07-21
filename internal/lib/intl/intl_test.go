// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package intllib

import (
	"strconv"
	"strings"
	"sync"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

var noCtx = interpreter.BuiltinCtx{}

func newCat() *catalogs { return &catalogs{byLang: map[string]map[string]string{}} }

// catalog builds a `map of string to string` Value from alternating k, v pairs.
func catalog(kv ...string) interpreter.Value {
	entries := make([]interpreter.MapEntry, 0, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		entries = append(entries, interpreter.MapEntry{
			Key:   interpreter.StringVal(kv[i]),
			Value: interpreter.StringVal(kv[i+1]),
		})
	}
	return interpreter.Value{Kind: interpreter.KindMap, Map: entries}
}

func mustLoad(t *testing.T, c *catalogs, lang string, cat interpreter.Value) {
	t.Helper()
	if _, err := c.loadFn(noCtx, []interpreter.Value{interpreter.StringVal(lang), cat}); err != nil {
		t.Fatalf("load %s: %v", lang, err)
	}
}

func tr(t *testing.T, c *catalogs, args ...interpreter.Value) string {
	t.Helper()
	v, err := c.trFn(noCtx, args)
	if err != nil {
		t.Fatalf("tr: %v", err)
	}
	return v.Str
}

func TestLoadAndTranslate(t *testing.T) {
	c := newCat()
	mustLoad(t, c, "en", catalog("bye", "Goodbye"))
	// No locale set yet: falls through to the default (first-loaded) language.
	if got := tr(t, c, interpreter.StringVal("bye")); got != "Goodbye" {
		t.Errorf("default tr = %q", got)
	}
	// locale() reflects setLocale.
	if _, err := c.setLocaleFn(noCtx, []interpreter.Value{interpreter.StringVal("en")}); err != nil {
		t.Fatal(err)
	}
	lv, _ := c.localeFn(noCtx, nil)
	if lv.Str != "en" {
		t.Errorf("locale = %q", lv.Str)
	}
}

func TestFallbackChain(t *testing.T) {
	c := newCat()
	mustLoad(t, c, "en", catalog("greeting", "Hello", "bye", "Goodbye", "only", "en-only"))
	mustLoad(t, c, "de", catalog("greeting", "Hallo", "bye", "Tschuess"))
	mustLoad(t, c, "de-AT", catalog("greeting", "Servus"))

	c.setLocaleFn(noCtx, []interpreter.Value{interpreter.StringVal("de-AT")})

	cases := map[string]string{
		"greeting": "Servus",   // exact de-AT
		"bye":      "Tschuess", // base language de
		"only":     "en-only",  // default language en
		"missing":  "missing",  // the key itself
	}
	for key, want := range cases {
		if got := tr(t, c, interpreter.StringVal(key)); got != want {
			t.Errorf("tr(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestInterpolation(t *testing.T) {
	c := newCat()
	mustLoad(t, c, "en", catalog(
		"greet", "Hello, {name}!",
		"count", "You have {n} items",
		"lit", "raw {missing} and {{brace}}",
	))
	params := func(kv ...interpreter.Value) interpreter.Value {
		entries := make([]interpreter.MapEntry, 0, len(kv)/2)
		for i := 0; i+1 < len(kv); i += 2 {
			entries = append(entries, interpreter.MapEntry{Key: kv[i], Value: kv[i+1]})
		}
		return interpreter.Value{Kind: interpreter.KindMap, Map: entries}
	}

	if got := tr(t, c, interpreter.StringVal("greet"),
		params(interpreter.StringVal("name"), interpreter.StringVal("World"))); got != "Hello, World!" {
		t.Errorf("string interp = %q", got)
	}
	// A non-string param is rendered to its display form.
	if got := tr(t, c, interpreter.StringVal("count"),
		params(interpreter.StringVal("n"), interpreter.IntVal(5))); got != "You have 5 items" {
		t.Errorf("int interp = %q", got)
	}
	// A missing placeholder stays literal; {{ }} escape to single braces.
	if got := tr(t, c, interpreter.StringVal("lit"), params()); got != "raw {missing} and {brace}" {
		t.Errorf("literal/escape = %q", got)
	}
}

func TestLoadErrors(t *testing.T) {
	c := newCat()
	bad := [][]interpreter.Value{
		{interpreter.StringVal("en")},                             // arity
		{interpreter.IntVal(1), catalog()},                        // lang not string
		{interpreter.StringVal(""), catalog()},                    // empty lang
		{interpreter.StringVal("en"), interpreter.StringVal("x")}, // catalog not map
	}
	for i, args := range bad {
		if _, err := c.loadFn(noCtx, args); err == nil {
			t.Errorf("load case %d: expected an error", i)
		}
	}
	// A non-string catalog value is rejected.
	nonStr := interpreter.Value{Kind: interpreter.KindMap, Map: []interpreter.MapEntry{
		{Key: interpreter.StringVal("k"), Value: interpreter.IntVal(1)},
	}}
	if _, err := c.loadFn(noCtx, []interpreter.Value{interpreter.StringVal("en"), nonStr}); err == nil {
		t.Error("load with non-string catalog value should error")
	}
}

func TestTrErrors(t *testing.T) {
	c := newCat()
	if _, err := c.trFn(noCtx, nil); err == nil {
		t.Error("tr with no args should error")
	}
	if _, err := c.trFn(noCtx, []interpreter.Value{interpreter.IntVal(1)}); err == nil {
		t.Error("tr with non-string key should error")
	}
	if _, err := c.trFn(noCtx, []interpreter.Value{interpreter.StringVal("k"), interpreter.StringVal("x")}); err == nil {
		t.Error("tr with non-map params should error")
	}
}

func TestBaseLang(t *testing.T) {
	cases := map[string]string{"de-AT": "de", "en_US": "en", "de": "de", "": ""}
	for in, want := range cases {
		if got := baseLang(in); got != want {
			t.Errorf("baseLang(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestConcurrentAccessRace hammers every entry point from many goroutines at
// once; run with -race it is the actual proof the RWMutex makes the shared
// catalog state safe for `spawn`ed access.
func TestConcurrentAccessRace(t *testing.T) {
	c := newCat()
	mustLoad(t, c, "en", catalog("k", "hello {who}"))
	var wg sync.WaitGroup
	for g := 0; g < 64; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			lang := "l" + strconv.Itoa(g%4)
			params := catalog("who", "world")
			for i := 0; i < 300; i++ {
				c.loadFn(noCtx, []interpreter.Value{interpreter.StringVal(lang), catalog("k", "x")})
				c.setLocaleFn(noCtx, []interpreter.Value{interpreter.StringVal("en")})
				c.localeFn(noCtx, nil)
				c.trFn(noCtx, []interpreter.Value{interpreter.StringVal("k")})
				c.trFn(noCtx, []interpreter.Value{interpreter.StringVal("k"), params})
			}
		}(g)
	}
	wg.Wait()
}

// TestInterpolationNoReexpansion is the core hang / OOM safety property:
// interpolation is single-pass, so a substituted value is NEVER re-scanned. A
// template of N placeholders expands to exactly N copies (linear, not
// exponential), and a param whose value itself contains a placeholder - even a
// self-reference - is inert rather than an infinite loop.
func TestInterpolationNoReexpansion(t *testing.T) {
	const n = 20000
	c := newCat()
	c.loadFn(noCtx, []interpreter.Value{interpreter.StringVal("en"),
		catalog("t", strings.Repeat("{x}", n), "self", "{x}")})

	// Substituted value contains {x} and {t}: kept verbatim, not re-expanded.
	out := tr(t, c, interpreter.StringVal("t"), catalog("x", "{x}{t}"))
	if want := n * len("{x}{t}"); len(out) != want {
		t.Fatalf("expansion = %d bytes, want %d (single-pass, no re-expansion)", len(out), want)
	}

	// A self-referential param terminates and is substituted exactly once.
	if got := tr(t, c, interpreter.StringVal("self"), catalog("x", "{x}")); got != "{x}" {
		t.Errorf("self-ref = %q, want %q (one substitution, no loop)", got, "{x}")
	}
}

// TestInterpolationOutputCap covers the untrusted-catalog output limit: a
// template that amplifies past maxTranslationBytes errors (before the oversized
// string is built), while an ordinary translation is untouched.
func TestInterpolationOutputCap(t *testing.T) {
	c := newCat()

	// Small template, huge amplification: 200k copies of a 1 KiB param.
	c.loadFn(noCtx, []interpreter.Value{interpreter.StringVal("en"),
		catalog("amp", strings.Repeat("{x}", 200000))})
	_, err := c.trFn(noCtx, []interpreter.Value{interpreter.StringVal("amp"),
		catalog("x", strings.Repeat("A", 1024))})
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("amplified translation error = %v, want an output-limit error", err)
	}

	// A single oversized param is rejected before it is copied in.
	c.loadFn(noCtx, []interpreter.Value{interpreter.StringVal("en"), catalog("one", "{x}")})
	if _, err := c.trFn(noCtx, []interpreter.Value{interpreter.StringVal("one"),
		catalog("x", strings.Repeat("B", maxTranslationBytes+1))}); err == nil {
		t.Error("oversized single param should hit the cap")
	}

	// An oversized literal template (no substitution) is also capped.
	c.loadFn(noCtx, []interpreter.Value{interpreter.StringVal("en"),
		catalog("big", strings.Repeat("x", maxTranslationBytes+1))})
	if _, err := c.trFn(noCtx, []interpreter.Value{interpreter.StringVal("big")}); err == nil {
		t.Error("oversized literal template should hit the cap")
	}

	// A normal translation just under the limit is unaffected.
	if got := tr(t, c, interpreter.StringVal("one"),
		catalog("x", strings.Repeat("C", 1000))); len(got) != 1000 {
		t.Errorf("normal translation length = %d, want 1000 (cap must not fire)", len(got))
	}
}

// TestInterpolationLinearScan guards against a quadratic scan on adversarial
// brace patterns (unclosed braces, dense braces): each must complete in a single
// linear pass.
func TestInterpolationLinearScan(t *testing.T) {
	c := newCat()
	patterns := []string{
		strings.Repeat("{", 100000),       // all opens, never closed
		strings.Repeat("{}", 50000),       // empty placeholders
		strings.Repeat("{{", 50000),       // all escapes
		"{" + strings.Repeat("a", 100000), // one huge unterminated name
		strings.Repeat("{a", 50000) + "}", // many opens, one distant close
	}
	for i, p := range patterns {
		c.loadFn(noCtx, []interpreter.Value{interpreter.StringVal("en"), catalog("p", p)})
		// Must simply return (no panic, no hang); value content is not asserted.
		_ = tr(t, c, interpreter.StringVal("p"), catalog("z", "Z"))
		_ = i
	}
}

func TestLoadMergesAndDefaultStable(t *testing.T) {
	c := newCat()
	mustLoad(t, c, "en", catalog("a", "A"))
	mustLoad(t, c, "de", catalog("a", "DE-A")) // second load must not change the default
	mustLoad(t, c, "en", catalog("b", "B"))    // merge into en, keep a
	c.setLocaleFn(noCtx, []interpreter.Value{interpreter.StringVal("fr")})
	// fr has no catalog -> default en.
	if got := tr(t, c, interpreter.StringVal("a")); got != "A" {
		t.Errorf("merged/default a = %q", got)
	}
	if got := tr(t, c, interpreter.StringVal("b")); got != "B" {
		t.Errorf("merged b = %q", got)
	}
}
