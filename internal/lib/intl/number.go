// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package intllib

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// maxFractionDigits bounds the digits after the decimal separator: enough for
// any currency or measurement, small enough that a caller cannot ask for a
// megabyte of zeros.
const maxFractionDigits = 20

// numberStyle is how one locale writes a number: its separators, the size of
// the first and the following digit groups, the fewest integer digits that get
// a separator at all (CLDR minimumGroupingDigits: 2 means "1234" stays
// ungrouped), and its minus sign.
type numberStyle struct {
	decimal   string
	group     string
	primary   int
	secondary int
	minGroup  int
	minus     string
}

const (
	nbsp      = " " // no-break space
	nnbsp     = " " // narrow no-break space
	apos      = "’" // right single quotation mark, the Swiss group separator
	minusSign = "−" // MINUS SIGN, used by some Nordic locales
)

// numberStyles is the locale table, from the CLDR number symbols (latn digits).
// A regional locale missing here falls back to its language ("de-DE" -> "de").
var numberStyles = map[string]numberStyle{
	"en":    {".", ",", 3, 3, 1, "-"},
	"en-IN": {".", ",", 3, 2, 1, "-"},
	"en-CH": {".", apos, 3, 3, 1, "-"},
	"en-ZA": {",", nbsp, 3, 3, 1, "-"},
	"hi":    {".", ",", 3, 2, 1, "-"},
	"de":    {",", ".", 3, 3, 1, "-"},
	"de-AT": {",", nbsp, 3, 3, 1, "-"},
	"de-CH": {".", apos, 3, 3, 1, "-"},
	"de-LI": {".", apos, 3, 3, 1, "-"},
	"fr":    {",", nnbsp, 3, 3, 1, "-"},
	"fr-CA": {",", nbsp, 3, 3, 1, "-"},
	"fr-CH": {",", nnbsp, 3, 3, 1, "-"},
	"it":    {",", ".", 3, 3, 1, "-"},
	"it-CH": {".", apos, 3, 3, 1, "-"},
	"es":    {",", ".", 3, 3, 2, "-"},
	"es-MX": {".", ",", 3, 3, 1, "-"},
	"es-US": {".", ",", 3, 3, 1, "-"},
	"pt":    {",", ".", 3, 3, 1, "-"},
	"pt-PT": {",", nbsp, 3, 3, 2, "-"},
	"nl":    {",", ".", 3, 3, 1, "-"},
	"da":    {",", ".", 3, 3, 1, "-"},
	"sv":    {",", nbsp, 3, 3, 1, minusSign},
	"nb":    {",", nbsp, 3, 3, 1, minusSign},
	"no":    {",", nbsp, 3, 3, 1, minusSign},
	"fi":    {",", nbsp, 3, 3, 1, minusSign},
	"pl":    {",", nbsp, 3, 3, 2, "-"},
	"cs":    {",", nbsp, 3, 3, 1, "-"},
	"sk":    {",", nbsp, 3, 3, 1, "-"},
	"hu":    {",", nbsp, 3, 3, 1, "-"},
	"ru":    {",", nbsp, 3, 3, 1, "-"},
	"uk":    {",", nbsp, 3, 3, 1, "-"},
	"tr":    {",", ".", 3, 3, 1, "-"},
	"ja":    {".", ",", 3, 3, 1, "-"},
	"zh":    {".", ",", 3, 3, 1, "-"},
	"ko":    {".", ",", 3, 3, 1, "-"},
}

// styleFor finds the style of a normalized locale: the exact tag, else its
// language. A locale whose language is not in the table is an error, not a
// silent "en".
func styleFor(tag string) (numberStyle, bool) {
	if s, ok := numberStyles[tag]; ok {
		return s, true
	}
	s, ok := numberStyles[baseLang(tag)]
	return s, ok
}

// formatNumberFn implements intl.formatNumber(locale, value, digits): value
// (int, float or decimal.Decimal) with exactly `digits` fraction digits,
// rounded half to even, grouped and separated the way `locale` writes numbers.
// Every error is a misuse, kind "runtime".
func formatNumberFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	fail := func(format string, a ...any) (interpreter.Value, error) {
		return interpreter.Null(), fmt.Errorf("intl.formatNumber: "+format, a...)
	}
	if len(args) != 3 {
		return fail("expects 3 arguments (locale, value, digits), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return fail("locale must be string, got %s", args[0].Kind)
	}
	tag, err := normalizeLocale("intl.formatNumber", args[0].Str())
	if err != nil {
		return interpreter.Null(), err
	}
	style, ok := styleFor(tag)
	if !ok {
		return fail("no number format for locale %q", args[0].Str())
	}
	if args[2].Kind != interpreter.KindInt {
		return fail("digits must be int, got %s", args[2].Kind)
	}
	digits := args[2].Int()
	if digits < 0 || digits > maxFractionDigits {
		return fail("digits must be 0 to %d, got %d", maxFractionDigits, digits)
	}
	text, err := plainDecimal(args[1])
	if err != nil {
		return fail("%v", err)
	}
	neg, whole, frac, err := roundHalfEven(text, int(digits))
	if err != nil {
		return fail("%v", err)
	}
	out := group(whole, style)
	if digits > 0 {
		out += style.decimal + frac
	}
	if neg {
		out = style.minus + out
	}
	return interpreter.StringVal(out), nil
}

// plainDecimal writes a number as a plain decimal string ("-1234.5"): an int
// exactly, a float as its shortest exact decimal form, a decimal.Decimal as
// its own digits.
func plainDecimal(v interpreter.Value) (string, error) {
	switch v.Kind {
	case interpreter.KindInt:
		return strconv.FormatInt(v.Int(), 10), nil
	case interpreter.KindFloat:
		if math.IsNaN(v.Float()) || math.IsInf(v.Float(), 0) {
			return "", fmt.Errorf("value is not a finite number")
		}
		return strconv.FormatFloat(v.Float(), 'f', -1, 64), nil
	}
	if n, ok := v.AsNativeObject("decimal", "Decimal"); ok {
		return n.DisplayNative(), nil
	}
	return "", fmt.Errorf("value must be int, float or decimal.Decimal, got %s", v.Kind)
}

// roundHalfEven rounds a plain decimal string to `digits` fraction digits,
// half to even, and returns its sign, integer digits and fraction digits. A
// value that rounds to zero loses its minus sign.
func roundHalfEven(text string, digits int) (bool, string, string, error) {
	neg := strings.HasPrefix(text, "-")
	text = strings.TrimPrefix(text, "-")
	whole, frac, _ := strings.Cut(text, ".")
	if whole == "" || strings.Trim(whole+frac, "0123456789") != "" {
		return false, "", "", fmt.Errorf("value %q is not a decimal number", text)
	}
	for len(frac) < digits {
		frac += "0"
	}
	keep, rest := frac[:digits], frac[digits:]
	n, _ := new(big.Int).SetString(whole+keep, 10)
	if rest != "" {
		half := "5" + strings.Repeat("0", len(rest)-1)
		switch c := strings.Compare(padLeft(rest, len(half)), half); {
		case c > 0:
			n.Add(n, big.NewInt(1))
		case c == 0 && n.Bit(0) == 1:
			n.Add(n, big.NewInt(1))
		}
	}
	s := n.String()
	if n.Sign() == 0 {
		neg = false
	}
	if len(s) <= digits {
		s = strings.Repeat("0", digits-len(s)+1) + s
	}
	return neg, s[:len(s)-digits], s[len(s)-digits:], nil
}

func padLeft(s string, n int) string {
	for len(s) < n {
		s = "0" + s
	}
	return s
}

// group inserts the locale's group separator into the integer digits: the
// first group from the right holds `primary` digits, the others `secondary`,
// and a number with fewer than primary + minGroup digits is not grouped.
func group(whole string, s numberStyle) string {
	if len(whole) < s.primary+s.minGroup {
		return whole
	}
	end := len(whole) - s.primary
	parts := []string{whole[end:]}
	for end > 0 {
		start := end - s.secondary
		if start < 0 {
			start = 0
		}
		parts = append([]string{whole[start:end]}, parts...)
		end = start
	}
	return strings.Join(parts, s.group)
}
