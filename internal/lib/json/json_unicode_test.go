// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package jsonlib

import (
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// TestDecodeUnicodeEscapes covers the JSON string `\u` escape path
// (parseUnicodeEscape + readHex4): a BMP escape, a surrogate pair, and the
// error branches. The backslash is built from a rune value so an editor / tool
// that normalises a literal `\u` sequence cannot defeat the test.
func TestDecodeUnicodeEscapes(t *testing.T) {
	bs := string(rune(0x5C)) // one backslash, no literal escape in the source

	dec := func(jsonText string) (interpreter.Value, error) {
		raw, err := decodeFn([]interpreter.Value{interpreter.StringVal(jsonText)})
		if err != nil {
			return interpreter.Null(), err
		}
		return interpreter.ObjectVal(LibraryName, "Value", raw), nil // wrap as json.Value
	}
	asStr := func(node interpreter.Value, ptr string) string {
		t.Helper()
		v, err := asStringFn(interpreter.BuiltinCtx{}, []interpreter.Value{node, interpreter.StringVal(ptr)})
		if err != nil {
			t.Fatalf("asString(%q): %v", ptr, err)
		}
		return v.Str
	}

	// BMP escape: {"k": "é"} -> é (U+00E9).
	node, err := dec(`{"k": "` + bs + `u00e9"}`)
	if err != nil {
		t.Fatalf("decode BMP: %v", err)
	}
	if got := asStr(node, "/k"); got != string(rune(0x00E9)) {
		t.Errorf("\\u00e9 decoded to %q, want U+00E9", got)
	}

	// Surrogate pair: {"k": "😀"} -> U+1F600 (an emoji).
	node2, err := dec(`{"k": "` + bs + `ud83d` + bs + `ude00"}`)
	if err != nil {
		t.Fatalf("decode surrogate pair: %v", err)
	}
	if got := asStr(node2, "/k"); got != string(rune(0x1F600)) {
		t.Errorf("surrogate pair decoded to %q, want U+1F600", got)
	}

	// Error branches of readHex4 / parseUnicodeEscape.
	bad := map[string]string{
		"incomplete \\u":     `{"k": "` + bs + `u12"}`,                  // fewer than 4 hex digits
		"non-hex \\u":        `{"k": "` + bs + `uZZZZ"}`,                // not hex
		"unpaired surrogate": `{"k": "` + bs + `ud83d"}`,                // high surrogate, no low
		"invalid pair":       `{"k": "` + bs + `ud83d` + bs + `u0041"}`, // low is not a low surrogate
	}
	for name, txt := range bad {
		if _, err := dec(txt); err == nil {
			t.Errorf("%s: expected a decode error, got nil", name)
		}
	}
}
