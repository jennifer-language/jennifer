// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package intllib implements the `intl` system library: message catalogs and
// locale-aware translation. It is a system library rather than a `.j` module for
// two independent reasons - it holds global mutable state (the loaded catalogs
// plus the current locale), which a declarations-only module cannot; and it
// keeps each catalog in a Go `map[string]string` for O(1) lookup, where a
// Jennifer `map` (a linear-scan `[]MapEntry`) would be O(n) per translation.
//
// Surface: `intl.load(lang, catalog)` ingests a `map of string to string`;
// `intl.setLocale(lang)` / `intl.locale()` get and set the current locale;
// `intl.tr(key)` translates in the current locale, and `intl.tr(key, params)`
// additionally fills `{name}` placeholders. A missing translation falls back
// current locale -> its base language -> the default (first-loaded) language ->
// the key itself, so a gap is always visible rather than silently blank.
package intllib

import (
	"fmt"
	"strings"
	"sync"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// LibraryName is the namespace prefix (`intl.load`, `intl.tr`, ...).
const LibraryName = "intl"

// maxTranslationBytes caps the size of one interpolated translation. A template
// from an untrusted catalog can repeat a placeholder arbitrarily many times, so a
// small template plus a large (or many-times-substituted) param could amplify
// into a huge string and drive the process toward OOM. Interpolation checks this
// limit incrementally, so it errors *before* materialising the oversized string
// rather than after. A genuine translation is a UI message far below 1 MiB;
// anything larger is a document a templating layer should build, not a catalog
// entry.
const maxTranslationBytes = 1 << 20 // 1 MiB

// catalogs holds the library's global mutable state. A sync.RWMutex guards every
// field so a `spawn`ed goroutine reading a translation cannot race a load or a
// locale change (the state lives in the library, not the deep-copied spawn
// scope, so it is genuinely shared).
type catalogs struct {
	mu      sync.RWMutex
	byLang  map[string]map[string]string // language -> (key -> template)
	locale  string                       // current locale, e.g. "de-AT"
	deflang string                       // default language (the first one loaded)
}

// Install registers the intl library on in. Each interpreter gets its own
// catalog state (closed over by the builtins), so nothing leaks between runs.
func Install(in *interpreter.Interpreter) {
	c := &catalogs{byLang: map[string]map[string]string{}}
	in.RegisterNamespaced(LibraryName, "load", c.loadFn)
	in.RegisterNamespaced(LibraryName, "setLocale", c.setLocaleFn)
	in.RegisterNamespaced(LibraryName, "locale", c.localeFn)
	in.RegisterNamespaced(LibraryName, "tr", c.trFn)
}

// loadFn implements intl.load(lang, catalog): merge a `map of string to string`
// into the catalog for lang. The first language loaded becomes the default.
func (c *catalogs) loadFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("intl.load expects 2 arguments (lang, catalog), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("intl.load: lang must be string, got %s", args[0].Kind)
	}
	lang := args[0].Str
	if lang == "" {
		return interpreter.Null(), fmt.Errorf("intl.load: lang must not be empty")
	}
	if args[1].Kind != interpreter.KindMap {
		return interpreter.Null(), fmt.Errorf("intl.load: catalog must be a map of string to string, got %s", args[1].Kind)
	}
	// Validate every entry up front (no allocation) so a bad catalog fails
	// before it touches shared state; the copy into the stored map then runs
	// straight from args, without a redundant intermediate map.
	for _, e := range args[1].Map {
		if e.Key.Kind != interpreter.KindString {
			return interpreter.Null(), fmt.Errorf("intl.load: catalog key must be string, got %s", e.Key.Kind)
		}
		if e.Value.Kind != interpreter.KindString {
			return interpreter.Null(), fmt.Errorf("intl.load: catalog value for %q must be string, got %s", e.Key.Str, e.Value.Kind)
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	m := c.byLang[lang]
	if m == nil {
		m = make(map[string]string, len(args[1].Map))
		c.byLang[lang] = m
	}
	for _, e := range args[1].Map {
		m[e.Key.Str] = e.Value.Str
	}
	if c.deflang == "" {
		c.deflang = lang
	}
	return interpreter.Null(), nil
}

// setLocaleFn implements intl.setLocale(lang).
func (c *catalogs) setLocaleFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("intl.setLocale expects 1 argument (lang), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("intl.setLocale: lang must be string, got %s", args[0].Kind)
	}
	c.mu.Lock()
	c.locale = args[0].Str
	c.mu.Unlock()
	return interpreter.Null(), nil
}

// localeFn implements intl.locale() -> string (the current locale, "" if unset).
func (c *catalogs) localeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 0 {
		return interpreter.Null(), fmt.Errorf("intl.locale expects 0 arguments, got %d", len(args))
	}
	c.mu.RLock()
	loc := c.locale
	c.mu.RUnlock()
	return interpreter.StringVal(loc), nil
}

// trFn implements intl.tr(key[, params]).
func (c *catalogs) trFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return interpreter.Null(), fmt.Errorf("intl.tr expects 1 or 2 arguments (key[, params]), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("intl.tr: key must be string, got %s", args[0].Kind)
	}

	var params map[string]string
	if len(args) == 2 {
		if args[1].Kind != interpreter.KindMap {
			return interpreter.Null(), fmt.Errorf("intl.tr: params must be a map, got %s", args[1].Kind)
		}
		params = make(map[string]string, len(args[1].Map))
		for _, e := range args[1].Map {
			if e.Key.Kind != interpreter.KindString {
				return interpreter.Null(), fmt.Errorf("intl.tr: param key must be string, got %s", e.Key.Kind)
			}
			// A non-string value is rendered to its display form, so a caller can
			// interpolate numbers without a manual convert.toString.
			params[e.Key.Str] = e.Value.Display()
		}
	}

	out, err := interpolate(c.lookup(args[0].Str), params)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(out), nil
}

// lookup resolves key through the fallback chain: current locale, its base
// language, the default (first-loaded) language, then the key itself.
func (c *catalogs) lookup(key string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if v, ok := c.get(c.locale, key); ok {
		return v
	}
	if base := baseLang(c.locale); base != c.locale {
		if v, ok := c.get(base, key); ok {
			return v
		}
	}
	if c.deflang != c.locale {
		if v, ok := c.get(c.deflang, key); ok {
			return v
		}
	}
	return key
}

// get reads one key from one language's catalog. The caller holds c.mu.
func (c *catalogs) get(lang, key string) (string, bool) {
	if lang == "" {
		return "", false
	}
	m := c.byLang[lang]
	if m == nil {
		return "", false
	}
	v, ok := m[key]
	return v, ok
}

// baseLang strips a locale's region, so "de-AT" / "de_AT" -> "de".
func baseLang(locale string) string {
	for i := 0; i < len(locale); i++ {
		if locale[i] == '-' || locale[i] == '_' {
			return locale[:i]
		}
	}
	return locale
}

// interpolate replaces each `{name}` placeholder with params[name]. A name with
// no matching param is left literal, so a missing value is visible; `{{` and
// `}}` are escapes for a literal brace. The output is bounded by
// maxTranslationBytes (checked incrementally): a template that amplifies past the
// limit errors before the oversized string is built.
func interpolate(s string, params map[string]string) (string, error) {
	if !strings.ContainsRune(s, '{') && !strings.ContainsRune(s, '}') {
		if len(s) > maxTranslationBytes {
			return "", errTooLarge()
		}
		return s, nil
	}
	var b strings.Builder
	// Pre-size to the template, but never beyond the output cap: a multi-MiB
	// template value from an untrusted catalog must not force a matching
	// allocation up front, since the interpolation loop errors once the running
	// output passes maxTranslationBytes anyway.
	b.Grow(min(len(s), maxTranslationBytes))
	for i := 0; i < len(s); {
		// Bounds the running output regardless of write kind, so accumulation from
		// repeated substitutions or long literal runs cannot exceed the cap.
		if b.Len() > maxTranslationBytes {
			return "", errTooLarge()
		}
		switch s[i] {
		case '{':
			if i+1 < len(s) && s[i+1] == '{' {
				b.WriteByte('{')
				i += 2
				continue
			}
			end := strings.IndexByte(s[i+1:], '}')
			if end < 0 {
				b.WriteString(s[i:])
				i = len(s)
				continue
			}
			name := s[i+1 : i+1+end]
			if v, ok := params[name]; ok {
				// Reject a single substitution that alone would overshoot, so one
				// oversized param cannot be copied into the builder at all.
				if b.Len()+len(v) > maxTranslationBytes {
					return "", errTooLarge()
				}
				b.WriteString(v)
			} else {
				b.WriteString(s[i : i+1+end+1]) // keep `{name}` literal
			}
			i += end + 2
		case '}':
			if i+1 < len(s) && s[i+1] == '}' {
				b.WriteByte('}')
				i += 2
				continue
			}
			b.WriteByte('}')
			i++
		default:
			b.WriteByte(s[i])
			i++
		}
	}
	if b.Len() > maxTranslationBytes {
		return "", errTooLarge()
	}
	return b.String(), nil
}

// errTooLarge is the output-cap error, shared by every check site.
func errTooLarge() error {
	return fmt.Errorf("intl.tr: translation exceeds the %d-byte output limit (a catalog template amplified too far)", maxTranslationBytes)
}
