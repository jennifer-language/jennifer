// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Formatting and parsing. `time.format` / `time.parse` use a
// strftime-style layout: `%Y-%m-%d %H:%M:%S` rather than Go's
// reference-time spelling, since the strftime codes are widely
// familiar (C / Python / shell / many languages). `time.iso` and
// `time.fromIso` round-trip RFC 3339 strings without needing a layout
// argument since that's the most common case.

package timelib

import (
	"fmt"
	"strings"
	stdtime "time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
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
		return interpreter.Null(), fmt.Errorf("time.format: %v", err)
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
		return interpreter.Null(), fmt.Errorf("time.parse: %v", err)
	}
	return makeTime(t)
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
		return interpreter.Null(), fmt.Errorf("time.fromIso: %v", err)
	}
	return makeTime(t)
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
	for i := 0; i < len(layout); i++ {
		c := layout[i]
		if c != '%' {
			b.WriteByte(c)
			continue
		}
		if i+1 >= len(layout) {
			return "", fmt.Errorf("trailing %% at position %d", i)
		}
		verb := layout[i+1]
		switch verb {
		case 'Y':
			fmt.Fprintf(&b, "%04d", t.Year())
		case 'm':
			fmt.Fprintf(&b, "%02d", t.Month())
		case 'd':
			fmt.Fprintf(&b, "%02d", t.Day())
		case 'H':
			fmt.Fprintf(&b, "%02d", t.Hour())
		case 'M':
			fmt.Fprintf(&b, "%02d", t.Minute())
		case 'S':
			fmt.Fprintf(&b, "%02d", t.Second())
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
			fmt.Fprintf(&b, "%03d", t.YearDay())
		case 'u':
			wd := int(t.Weekday())
			if wd == 0 {
				wd = 7
			}
			b.WriteByte(byte('0' + wd))
		case '%':
			b.WriteByte('%')
		default:
			return "", fmt.Errorf("unknown format verb %%%c at position %d", verb, i)
		}
		i++ // skip the verb byte
	}
	return b.String(), nil
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
// caller sees a positioned runtime error.
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
	)

	li, si := 0, 0
	for li < len(layout) {
		c := layout[li]
		if c != '%' {
			// Literal match. Whitespace in the layout matches one
			// whitespace byte in the input to keep parsing simple.
			if si >= len(input) || input[si] != c {
				return stdtime.Time{}, fmt.Errorf(
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
		switch verb {
		case 'Y':
			n, consumed, err := readFixedDigits(input, si, 4)
			if err != nil {
				return stdtime.Time{}, fmt.Errorf("%%Y: %v", err)
			}
			year = n
			si += consumed
		case 'm':
			n, consumed, err := readFixedDigits(input, si, 2)
			if err != nil {
				return stdtime.Time{}, fmt.Errorf("%%m: %v", err)
			}
			if n < 1 || n > 12 {
				return stdtime.Time{}, fmt.Errorf("%%m: month %d out of range 1..12", n)
			}
			month = n
			si += consumed
		case 'd':
			n, consumed, err := readFixedDigits(input, si, 2)
			if err != nil {
				return stdtime.Time{}, fmt.Errorf("%%d: %v", err)
			}
			if n < 1 || n > 31 {
				return stdtime.Time{}, fmt.Errorf("%%d: day %d out of range 1..31", n)
			}
			day = n
			si += consumed
		case 'H':
			n, consumed, err := readFixedDigits(input, si, 2)
			if err != nil {
				return stdtime.Time{}, fmt.Errorf("%%H: %v", err)
			}
			if n < 0 || n > 23 {
				return stdtime.Time{}, fmt.Errorf("%%H: hour %d out of range 0..23", n)
			}
			hour = n
			si += consumed
		case 'M':
			n, consumed, err := readFixedDigits(input, si, 2)
			if err != nil {
				return stdtime.Time{}, fmt.Errorf("%%M: %v", err)
			}
			if n < 0 || n > 59 {
				return stdtime.Time{}, fmt.Errorf("%%M: minute %d out of range 0..59", n)
			}
			minute = n
			si += consumed
		case 'S':
			n, consumed, err := readFixedDigits(input, si, 2)
			if err != nil {
				return stdtime.Time{}, fmt.Errorf("%%S: %v", err)
			}
			if n < 0 || n > 59 {
				return stdtime.Time{}, fmt.Errorf("%%S: second %d out of range 0..59", n)
			}
			second = n
			si += consumed
		case 'z':
			off, consumed, err := readZoneShort(input, si)
			if err != nil {
				return stdtime.Time{}, fmt.Errorf("%%z: %v", err)
			}
			offset = off
			si += consumed
		case 'a', 'A':
			name, consumed, err := readName(input, si)
			if err != nil {
				return stdtime.Time{}, fmt.Errorf("%%%c: %v", verb, err)
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
				return stdtime.Time{}, fmt.Errorf("%%%c: %q is not an English weekday name", verb, name)
			}
			si += consumed
		case 'b', 'B':
			name, consumed, err := readName(input, si)
			if err != nil {
				return stdtime.Time{}, fmt.Errorf("%%%c: %v", verb, err)
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
				return stdtime.Time{}, fmt.Errorf("%%%c: %q is not an English month name", verb, name)
			}
			month = matchedIdx + 1
			si += consumed
		case '%':
			if si >= len(input) || input[si] != '%' {
				return stdtime.Time{}, fmt.Errorf("expected literal %% at position %d", si)
			}
			si++
		default:
			return stdtime.Time{}, fmt.Errorf("unknown / unparseable verb %%%c at position %d", verb, li)
		}
		li += 2
	}
	if si != len(input) {
		return stdtime.Time{}, fmt.Errorf("trailing input %q after layout consumed", input[si:])
	}

	loc := stdtime.FixedZone("", offset)
	t := stdtime.Date(year, stdtime.Month(month), day, hour, minute, second, nano, loc)
	// time.Date silently normalizes an out-of-range day (Feb 31 -> Mar 3);
	// reject a date whose Y/M/D changed so an impossible date is an error, not
	// a silent shift.
	if t.Year() != year || int(t.Month()) != month || t.Day() != day {
		return stdtime.Time{}, fmt.Errorf("impossible date %04d-%02d-%02d", year, month, day)
	}
	return t, nil
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
