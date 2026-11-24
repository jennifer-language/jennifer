// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package intllib

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func formatNum(t *testing.T, locale string, v interpreter.Value, digits int64) string {
	t.Helper()
	out, err := formatNumberFn(noCtx, []interpreter.Value{
		interpreter.StringVal(locale), v, interpreter.IntVal(digits)})
	if err != nil {
		t.Fatalf("formatNumber(%q, %s, %d): %v", locale, v.Display(), digits, err)
	}
	return out.Str()
}

// TestFormatNumberLocales checks each kind of rule against CLDR's own output:
// separators, the Swiss apostrophe, French narrow no-break spaces, Indian 3;2
// grouping, minimum grouping digits and the Unicode minus sign.
func TestFormatNumberLocales(t *testing.T) {
	big := interpreter.FloatVal(1234567.891)
	cases := []struct {
		locale string
		v      interpreter.Value
		digits int64
		want   string
	}{
		{"en", big, 2, "1,234,567.89"},
		{"en-US", big, 2, "1,234,567.89"},
		{"de", big, 2, "1.234.567,89"},
		{"de-DE", big, 2, "1.234.567,89"},
		{"de-AT", big, 2, "1 234 567,89"},
		{"de-CH", big, 2, "1’234’567.89"},
		{"fr", big, 2, "1 234 567,89"},
		{"fr-CA", big, 2, "1 234 567,89"},
		{"en-IN", interpreter.IntVal(123456789), 0, "12,34,56,789"},
		{"hi", interpreter.IntVal(1000), 0, "1,000"},
		{"es", interpreter.IntVal(1234), 0, "1234"},
		{"es", interpreter.IntVal(12345), 0, "12.345"},
		{"pl", interpreter.IntVal(1234), 0, "1234"},
		{"pt-PT", interpreter.IntVal(12345), 0, "12 345"},
		{"sv", interpreter.IntVal(-1234), 0, "−1 234"},
		{"de", interpreter.IntVal(-5), 2, "-5,00"},
		{"en", interpreter.IntVal(999), 0, "999"},
		{"de_ch", interpreter.IntVal(1000), 0, "1’1000"[:0] + "1’000"},
	}
	for _, c := range cases {
		if got := formatNum(t, c.locale, c.v, c.digits); got != c.want {
			t.Errorf("formatNumber(%q, %s, %d) = %q, want %q", c.locale, c.v.Display(), c.digits, got, c.want)
		}
	}
}

// TestFormatNumberRoundsHalfEven: ties go to the even digit, as ICU does, and a
// value that rounds to zero has no minus sign.
func TestFormatNumberRoundsHalfEven(t *testing.T) {
	cases := []struct {
		v      interpreter.Value
		digits int64
		want   string
	}{
		{interpreter.FloatVal(2.5), 0, "2"},
		{interpreter.FloatVal(3.5), 0, "4"},
		{interpreter.FloatVal(0.125), 2, "0.12"},
		{interpreter.FloatVal(0.135), 2, "0.14"},
		{interpreter.FloatVal(-0.004), 2, "0.00"},
		{interpreter.FloatVal(999.996), 2, "1,000.00"},
		{interpreter.IntVal(7), 3, "7.000"},
		{interpreter.FloatVal(0.1), 20, "0.10000000000000000000"},
	}
	for _, c := range cases {
		if got := formatNum(t, "en", c.v, c.digits); got != c.want {
			t.Errorf("formatNumber(en, %s, %d) = %q, want %q", c.v.Display(), c.digits, got, c.want)
		}
	}
}

// TestFormatNumberErrors: an unknown language, a malformed locale, bad digits
// or a value that is no number throw; a regional locale falls back to its
// language.
func TestFormatNumberErrors(t *testing.T) {
	if got := formatNum(t, "de-BE", interpreter.IntVal(1234), 0); got != "1.234" {
		t.Errorf("de-BE should fall back to de, got %q", got)
	}
	bad := [][]interpreter.Value{
		{interpreter.StringVal("xx"), interpreter.IntVal(1), interpreter.IntVal(0)},
		{interpreter.StringVal(""), interpreter.IntVal(1), interpreter.IntVal(0)},
		{interpreter.StringVal("de-"), interpreter.IntVal(1), interpreter.IntVal(0)},
		{interpreter.StringVal("de.CH"), interpreter.IntVal(1), interpreter.IntVal(0)},
		{interpreter.StringVal("en"), interpreter.IntVal(1), interpreter.IntVal(-1)},
		{interpreter.StringVal("en"), interpreter.IntVal(1), interpreter.IntVal(21)},
		{interpreter.StringVal("en"), interpreter.StringVal("12"), interpreter.IntVal(0)},
		{interpreter.StringVal("en"), interpreter.IntVal(1)},
	}
	for _, args := range bad {
		_, err := formatNumberFn(noCtx, args)
		if err == nil || !strings.Contains(err.Error(), "intl.formatNumber") {
			t.Errorf("formatNumber(%v) = %v, want an intl.formatNumber error", args, err)
		}
	}
}
