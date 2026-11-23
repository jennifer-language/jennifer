// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// Formatting and parsing. `time.format` / `time.parse` use a
// strftime-style layout: `%Y-%m-%d %H:%M:%S` rather than Go's
// reference-time spelling, since the strftime codes are widely
// familiar (C / Python / shell / many languages). `time.iso` and
// `time.fromIso` round-trip RFC 3339 strings without needing a layout
// argument since that's the most common case.

package timelib

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	stdtime "time"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// formatFn implements `time.format($t, layout)`.
func formatFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("time.format expects 2 arguments (time, layout), got %d", len(args))
	}
	nanos, offset, err := extractTime("time.format", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("time.format: layout must be string, got %s", args[1].Kind)
	}
	out, err := strftimeFormat(goTimeFrom(nanos, offset), offset, args[1].Str)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("time.format: %w", err)
	}
	return interpreter.StringVal(out), nil
}

// parseFn implements `time.parse(s, layout)`.
func parseFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("time.parse expects 2 arguments (string, layout), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("time.parse: input must be string, got %s", args[0].Kind)
	}
	if args[1].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("time.parse: layout must be string, got %s", args[1].Kind)
	}
	t, err := strftimeParse(args[1].Str, args[0].Str)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("time.parse: %w", err)
	}
	v, err := makeTime(t)
	return v, dataerr.Wrap("time", err)
}

// isoFn implements `time.iso($t)`. Returns an RFC 3339 string with
// fractional seconds when the nanosecond part is non-zero.
func isoFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("time.iso expects 1 argument, got %d", len(args))
	}
	nanos, offset, err := extractTime("time.iso", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(formatISO(goTimeFrom(nanos, offset), offset)), nil
}

// fromIsoFn implements `time.fromIso(s)`. Accepts RFC 3339 (`T`
// separator, `Z` or `+HH:MM` zone, optional fractional seconds).
func fromIsoFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("time.fromIso expects 1 argument, got %d", len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("time.fromIso: input must be string, got %s", args[0].Kind)
	}
	t, err := stdtime.Parse(stdtime.RFC3339Nano, args[0].Str)
	if err != nil {
		return interpreter.Null(), dataerr.New("time", "time.fromIso: %v", err)
	}
	v, err := makeTime(t)
	return v, dataerr.Wrap("time", err)
}

// formatISO produces an RFC 3339 string. Includes the fractional
// second only when non-zero, so simple cases stay tidy.
func formatISO(t stdtime.Time, offset int64) string {
	base := t.Format("2006-01-02T15:04:05")
	frac := ""
	if t.Nanosecond() != 0 {
		// Up to 9 digits; trim trailing zeros for a tidy display.
		raw := fmt.Sprintf("%09d", t.Nanosecond())
		raw = strings.TrimRight(raw, "0")
		frac = "." + raw
	}
	return base + frac + formatOffsetISO(offset)
}

// strftimeFormat walks `layout`, emitting strftime verbs and passing
// every other rune through unchanged. The supported verb set is the
// v1 subset documented in docs/libraries/time.md.
func strftimeFormat(t stdtime.Time, offset int64, layout string) (string, error) {
	var b strings.Builder
	i := 0
	for i < len(layout) {
		c := layout[i]
		if c != '%' {
			b.WriteByte(c)
			i++
			continue
		}
		if i+1 >= len(layout) {
			return "", fmt.Errorf("trailing %% at position %d", i)
		}
		verb := layout[i+1]
		if verb == '%' {
			b.WriteByte('%')
			i += 2
			continue
		}
		if !isFormatVerb(verb) {
			return "", fmt.Errorf("unknown format verb %%%c at position %d", verb, i)
		}
		pad, next, err := readCodeModifiers(layout, verb, i+2)
		if err != nil {
			return "", err
		}
		i = next
		switch verb {
		case 'Y':
			writePadded(&b, t.Year(), pad)
		case 'm':
			writePadded(&b, int(t.Month()), pad)
		case 'd':
			writePadded(&b, t.Day(), pad)
		case 'H':
			writePadded(&b, t.Hour(), pad)
		case 'I':
			h := t.Hour() % 12
			if h == 0 {
				h = 12
			}
			writePadded(&b, h, pad)
		case 'p':
			if t.Hour() < 12 {
				b.WriteString("AM")
			} else {
				b.WriteString("PM")
			}
		case 'M':
			writePadded(&b, t.Minute(), pad)
		case 'S':
			writePadded(&b, t.Second(), pad)
		case 'z':
			b.WriteString(formatOffsetStrftime(offset))
		case 'a':
			b.WriteString(t.Weekday().String()[:3])
		case 'A':
			b.WriteString(t.Weekday().String())
		case 'b':
			b.WriteString(t.Month().String()[:3])
		case 'B':
			b.WriteString(t.Month().String())
		case 'j':
			writePadded(&b, t.YearDay(), pad)
		case 'u':
			wd := int(t.Weekday())
			if wd == 0 {
				wd = 7
			}
			b.WriteByte(byte('0' + wd))
		}
	}
	return b.String(), nil
}

// isFormatVerb reports whether c is a code time.format knows (`%%` aside).
func isFormatVerb(c byte) bool {
	return strings.IndexByte("YmdHIpMSzaAbBju", c) >= 0
}

// codeWidth is the default zero-padded width of a numeric code, or 0 for a
// code that takes no modifiers (the names, %p, %z, %u).
func codeWidth(verb byte) int {
	switch verb {
	case 'Y':
		return 4
	case 'j':
		return 3
	case 'm', 'd', 'H', 'I', 'M', 'S':
		return 2
	}
	return 0
}

// maxCodePad caps a code's pad, like io.printf's field cap: a layout never
// needs a wider number, and the pad is allocated as zeros.
const maxCodePad = 4096

// readCodeModifiers reads the printf-style modifier list after a layout code,
// starting at pos (just past the code), and returns the code's pad and the
// position after the list. It follows io.printf's rules: a `|` right after a
// code always starts a modifier, `||` is a literal `|` (the first is consumed
// here, the second is left as layout text), a bare value ends at the first
// byte outside [A-Za-z0-9_], and an unknown modifier, or one the code does
// not take, is an error. The numeric codes take `pad` (the minimum width,
// zero-filled; 0 prints the number unpadded); every other code takes none.
func readCodeModifiers(layout string, verb byte, pos int) (int, int, error) {
	pad := codeWidth(verb)
	seen := false
	for pos < len(layout) && layout[pos] == '|' {
		if pos+1 < len(layout) && layout[pos+1] == '|' {
			pos++
			break
		}
		bar := pos
		pos++
		start := pos
		for pos < len(layout) && layout[pos] >= 'a' && layout[pos] <= 'z' {
			pos++
		}
		key := layout[start:pos]
		if key == "" {
			return 0, pos, fmt.Errorf("expected modifier key after `|` at position %d", bar)
		}
		if pos >= len(layout) || layout[pos] != '=' {
			return 0, pos, fmt.Errorf("%%%c: modifier %q is missing `=`", verb, key)
		}
		pos++
		start = pos
		if pos+1 < len(layout) && layout[pos] == '-' && isDigit(layout[pos+1]) {
			pos++
		}
		for pos < len(layout) && isWordByte(layout[pos]) {
			pos++
		}
		value := layout[start:pos]
		if value == "" {
			return 0, pos, fmt.Errorf("%%%c: modifier %q: empty value after `=`", verb, key)
		}
		if codeWidth(verb) == 0 {
			return 0, pos, fmt.Errorf("%%%c takes no modifiers (got %q)", verb, key)
		}
		if key != "pad" {
			return 0, pos, fmt.Errorf("%%%c: modifier %q not valid in a time layout (only pad)", verb, key)
		}
		if seen {
			return 0, pos, fmt.Errorf("%%%c: modifier \"pad\" specified twice", verb)
		}
		seen = true
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil && !errors.Is(err, strconv.ErrRange) {
			return 0, pos, fmt.Errorf("%%%c: pad=%q is not an integer", verb, value)
		}
		if n < 0 {
			return 0, pos, fmt.Errorf("%%%c: pad must be >= 0", verb)
		}
		if n > maxCodePad {
			return 0, pos, limits.Capf("%%%c: pad=%s exceeds the maximum %d", verb, value, maxCodePad)
		}
		pad = int(n)
	}
	return pad, pos, nil
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isWordByte(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

// writePadded writes n zero-padded to at least width characters, the sign
// counted in the width (as `%0*d` does).
func writePadded(b *strings.Builder, n, width int) {
	digits := strconv.Itoa(n)
	sign := ""
	if n < 0 {
		sign, digits = "-", digits[1:]
	}
	b.WriteString(sign)
	for k := len(sign) + len(digits); k < width; k++ {
		b.WriteByte('0')
	}
	b.WriteString(digits)
}

// monthNames and weekdayNames hold the English names strftime parsing
// expects. Matching is case-insensitive (the lowercase form is
// canonical for comparison).
var monthNames = []string{
	"january", "february", "march", "april", "may", "june",
	"july", "august", "september", "october", "november", "december",
}
var weekdayNames = []string{
	// ISO order, Monday=1, but the array is 0-indexed so weekdayNames[0]="monday".
	"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday",
}

// strftimeParse walks `layout` and `input` in lockstep, filling in
// year/month/day/hour/minute/second/nanos/offset as verbs are
// encountered. Missing parts default to year=1970 month=1 day=1, all
// the time-of-day at zero, offset = 0 (UTC). On any mismatch the
// caller sees a positioned error of kind "time"; a malformed layout is a
// plain error.
func strftimeParse(layout, input string) (stdtime.Time, error) {
	var (
		year   = 1970
		month  = 1
		day    = 1
		hour   = 0
		minute = 0
		second = 0
		nano   = 0
		offset = 0
		// 12-hour clock state: %I fills hour12 (1..12), %p fills pmMark
		// (0 = AM, 1 = PM); both -1 until seen. Combined into `hour` after the
		// walk so the two verbs compose regardless of order.
		hour12 = -1
		pmMark = -1
	)

	li, si := 0, 0
	for li < len(layout) {
		c := layout[li]
		if c != '%' {
			// Literal match. Whitespace in the layout matches one
			// whitespace byte in the input to keep parsing simple.
			if si >= len(input) || input[si] != c {
				return stdtime.Time{}, badInput(
					"layout literal %q at position %d does not match input position %d",
					string(c), li, si,
				)
			}
			li++
			si++
			continue
		}
		if li+1 >= len(layout) {
			return stdtime.Time{}, fmt.Errorf("trailing %% in layout at position %d", li)
		}
		verb := layout[li+1]
		if verb == '%' {
			if si >= len(input) || input[si] != '%' {
				return stdtime.Time{}, badInput("expected literal %% at position %d", si)
			}
			si++
			li += 2
			continue
		}
		if !isParseVerb(verb) {
			return stdtime.Time{}, fmt.Errorf("unknown / unparseable verb %%%c at position %d", verb, li)
		}
		pad, next, err := readCodeModifiers(layout, verb, li+2)
		if err != nil {
			return stdtime.Time{}, err
		}
		li = next
		// A numeric code reads at least pad digits (one when pad=0) and at most
		// its default width or pad, whichever is larger: exactly the default
		// width when pad is unset, so every layout reads what it formats.
		minDigits, maxDigits := max(pad, 1), max(pad, codeWidth(verb))
		switch verb {
		case 'Y':
			n, consumed, err := readDigits(input, si, minDigits, maxDigits)
			if err != nil {
				return stdtime.Time{}, badInput("%%Y: %v", err)
			}
			year = n
			si += consumed
		case 'm':
			n, consumed, err := readDigits(input, si, minDigits, maxDigits)
			if err != nil {
				return stdtime.Time{}, badInput("%%m: %v", err)
			}
			if n < 1 || n > 12 {
				return stdtime.Time{}, badInput("%%m: month %d out of range 1..12", n)
			}
			month = n
			si += consumed
		case 'd':
			n, consumed, err := readDigits(input, si, minDigits, maxDigits)
			if err != nil {
				return stdtime.Time{}, badInput("%%d: %v", err)
			}
			if n < 1 || n > 31 {
				return stdtime.Time{}, badInput("%%d: day %d out of range 1..31", n)
			}
			day = n
			si += consumed
		case 'H':
			n, consumed, err := readDigits(input, si, minDigits, maxDigits)
			if err != nil {
				return stdtime.Time{}, badInput("%%H: %v", err)
			}
			if n < 0 || n > 23 {
				return stdtime.Time{}, badInput("%%H: hour %d out of range 0..23", n)
			}
			hour = n
			si += consumed
		case 'I':
			n, consumed, err := readDigits(input, si, minDigits, maxDigits)
			if err != nil {
				return stdtime.Time{}, badInput("%%I: %v", err)
			}
			if n < 1 || n > 12 {
				return stdtime.Time{}, badInput("%%I: hour %d out of range 1..12", n)
			}
			hour12 = n
			si += consumed
		case 'p':
			if si+2 > len(input) {
				return stdtime.Time{}, badInput("%%p: need AM or PM at position %d", si)
			}
			switch strings.ToUpper(input[si : si+2]) {
			case "AM":
				pmMark = 0
			case "PM":
				pmMark = 1
			default:
				return stdtime.Time{}, badInput("%%p: %q is not AM or PM", input[si:si+2])
			}
			si += 2
		case 'M':
			n, consumed, err := readDigits(input, si, minDigits, maxDigits)
			if err != nil {
				return stdtime.Time{}, badInput("%%M: %v", err)
			}
			if n < 0 || n > 59 {
				return stdtime.Time{}, badInput("%%M: minute %d out of range 0..59", n)
			}
			minute = n
			si += consumed
		case 'S':
			n, consumed, err := readDigits(input, si, minDigits, maxDigits)
			if err != nil {
				return stdtime.Time{}, badInput("%%S: %v", err)
			}
			if n < 0 || n > 59 {
				return stdtime.Time{}, badInput("%%S: second %d out of range 0..59", n)
			}
			second = n
			si += consumed
		case 'z':
			off, consumed, err := readZoneShort(input, si)
			if err != nil {
				return stdtime.Time{}, badInput("%%z: %v", err)
			}
			offset = off
			si += consumed
		case 'a', 'A':
			name, consumed, err := readName(input, si)
			if err != nil {
				return stdtime.Time{}, badInput("%%%c: %v", verb, err)
			}
			lc := strings.ToLower(name)
			matched := false
			for _, full := range weekdayNames {
				if (verb == 'A' && lc == full) || (verb == 'a' && lc == full[:3]) {
					matched = true
					break
				}
			}
			if !matched {
				return stdtime.Time{}, badInput("%%%c: %q is not an English weekday name", verb, name)
			}
			si += consumed
		case 'b', 'B':
			name, consumed, err := readName(input, si)
			if err != nil {
				return stdtime.Time{}, badInput("%%%c: %v", verb, err)
			}
			lc := strings.ToLower(name)
			matchedIdx := -1
			for idx, full := range monthNames {
				if (verb == 'B' && lc == full) || (verb == 'b' && lc == full[:3]) {
					matchedIdx = idx
					break
				}
			}
			if matchedIdx < 0 {
				return stdtime.Time{}, badInput("%%%c: %q is not an English month name", verb, name)
			}
			month = matchedIdx + 1
			si += consumed
		}
	}
	if si != len(input) {
		return stdtime.Time{}, badInput("trailing input %q after layout consumed", input[si:])
	}

	// Fold the 12-hour clock into `hour`: %I gives 1..12 (12 is midnight/noon,
	// i.e. 0 within its half), and %p selects the half (no %p means AM). Applied
	// after the walk so %I and %p compose in either order; it overrides a %H in
	// the same layout (mixing the two is a caller error).
	if hour12 >= 0 {
		h := hour12 % 12
		if pmMark == 1 {
			h += 12
		}
		hour = h
	}

	loc := stdtime.FixedZone("", offset)
	t := stdtime.Date(year, stdtime.Month(month), day, hour, minute, second, nano, loc)
	// time.Date silently normalizes an out-of-range day (Feb 31 -> Mar 3);
	// reject a date whose Y/M/D changed so an impossible date is an error, not
	// a silent shift.
	if t.Year() != year || int(t.Month()) != month || t.Day() != day {
		return stdtime.Time{}, badInput("impossible date %04d-%02d-%02d", year, month, day)
	}
	return t, nil
}

// badInput is an input string that does not match the layout or is no valid
// date (kind "time"); an error in the layout itself stays a plain error.
func badInput(format string, a ...any) error {
	return dataerr.New("time", format, a...)
}

// isParseVerb reports whether c is a code time.parse reads (`%%` aside).
func isParseVerb(c byte) bool {
	return strings.IndexByte("YmdHIpMSzaAbB", c) >= 0
}

// readDigits consumes between lo and hi decimal digits (as many as there are,
// up to hi) from input at pos. With lo == hi it is readFixedDigits.
func readDigits(input string, pos, lo, hi int) (int, int, error) {
	if lo == hi {
		return readFixedDigits(input, pos, lo)
	}
	end := pos
	for end < len(input) && end-pos < hi && isDigit(input[end]) {
		end++
	}
	if end-pos < lo {
		if end < len(input) {
			return 0, 0, fmt.Errorf("need %d to %d digits at position %d, found non-digit %q", lo, hi, pos, string(input[end]))
		}
		return 0, 0, fmt.Errorf("need %d to %d digits at position %d, only %d available", lo, hi, pos, end-pos)
	}
	val := 0
	for _, c := range []byte(input[pos:end]) {
		val = val*10 + int(c-'0')
	}
	return val, end - pos, nil
}

// readFixedDigits consumes exactly `n` decimal digits from input
// starting at `pos`. Returns the integer value, bytes consumed, and an
// error if fewer than `n` digits are available or the slice contains
// non-digits.
func readFixedDigits(input string, pos, n int) (int, int, error) {
	if pos+n > len(input) {
		return 0, 0, fmt.Errorf("need %d digits at position %d, only %d available", n, pos, len(input)-pos)
	}
	val := 0
	for i := 0; i < n; i++ {
		c := input[pos+i]
		if c < '0' || c > '9' {
			return 0, 0, fmt.Errorf("non-digit %q at position %d", string(c), pos+i)
		}
		val = val*10 + int(c-'0')
	}
	return val, n, nil
}

// readZoneShort consumes a `%z`-shaped zone marker: either `Z` (UTC)
// or `+HHMM` / `-HHMM`. Returns offset in seconds and bytes consumed.
func readZoneShort(input string, pos int) (int, int, error) {
	if pos >= len(input) {
		return 0, 0, fmt.Errorf("zone marker missing at position %d", pos)
	}
	if input[pos] == 'Z' {
		return 0, 1, nil
	}
	if pos+5 > len(input) {
		return 0, 0, fmt.Errorf("need 5 chars (+HHMM) at position %d, only %d available", pos, len(input)-pos)
	}
	sign := input[pos]
	if sign != '+' && sign != '-' {
		return 0, 0, fmt.Errorf("zone marker must start with + or -, got %q", string(sign))
	}
	hh, _, err := readFixedDigits(input, pos+1, 2)
	if err != nil {
		return 0, 0, err
	}
	mm, _, err := readFixedDigits(input, pos+3, 2)
	if err != nil {
		return 0, 0, err
	}
	if mm >= 60 {
		return 0, 0, fmt.Errorf("zone offset minutes %02d out of range (must be < 60)", mm)
	}
	off := hh*3600 + mm*60
	if sign == '-' {
		off = -off
	}
	// Enforce the same bound as time.zone so `%z` can't parse an offset the
	// zone constructor would reject (e.g. `+9999`).
	if off > maxOffsetSeconds || off < -maxOffsetSeconds {
		return 0, 0, fmt.Errorf("zone offset %c%02d%02d exceeds the +/-26h limit", sign, hh, mm)
	}
	return off, 5, nil
}

// readName consumes a contiguous run of ASCII letters from `pos`.
// Used by %a / %A / %b / %B parsing.
func readName(input string, pos int) (string, int, error) {
	if pos >= len(input) {
		return "", 0, fmt.Errorf("name expected at position %d, end of input", pos)
	}
	end := pos
	for end < len(input) {
		c := input[end]
		if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') {
			break
		}
		end++
	}
	if end == pos {
		return "", 0, fmt.Errorf("name expected at position %d, found %q", pos, string(input[pos]))
	}
	return input[pos:end], end - pos, nil
}
