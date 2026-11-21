// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package timelib

import (
	"errors"
	"strings"
	"testing"
	stdtime "time"

	"jennifer-lang.dev/jennifer/internal/limits"
)

// TestFormatPadModifier: the numeric codes take printf's `pad` modifier
// (M29.24): pad=0 prints the number unpadded, a pad above the default widens
// it with zeros, and no modifier keeps the default width.
func TestFormatPadModifier(t *testing.T) {
	single := stdtime.Date(2027, 1, 2, 3, 4, 5, 0, stdtime.UTC)      // every field one digit
	double := stdtime.Date(2024, 12, 25, 23, 58, 59, 0, stdtime.UTC) // every field two digits
	cases := []struct {
		t            stdtime.Time
		layout, want string
	}{
		{single, "%Y|pad=0 %m|pad=0 %d|pad=0 %H|pad=0 %I|pad=0 %M|pad=0 %S|pad=0 %j|pad=0", "2027 1 2 3 3 4 5 2"},
		{single, "%Y %m %d %H %I %M %S %j", "2027 01 02 03 03 04 05 002"},
		{double, "%Y|pad=0 %m|pad=0 %d|pad=0 %H|pad=0 %I|pad=0 %M|pad=0 %S|pad=0 %j|pad=0", "2024 12 25 23 11 58 59 360"},
		{double, "%m|pad=1 %d|pad=1 %j|pad=2", "12 25 360"},
		// pad above the default widens with zeros; between 0 and the default
		// it is a smaller minimum width.
		{single, "%d|pad=4 %Y|pad=6 %j|pad=5", "0002 002027 00002"},
		{stdtime.Date(5, 3, 1, 0, 0, 0, 0, stdtime.UTC), "%Y||%Y|pad=0||%Y|pad=2", "0005|5|05"},
		// %j on the first and the last day of a leap year.
		{stdtime.Date(2024, 1, 1, 0, 0, 0, 0, stdtime.UTC), "%j|pad=0", "1"},
		{stdtime.Date(2024, 12, 31, 0, 0, 0, 0, stdtime.UTC), "%j|pad=0", "366"},
		// The US long date; `,` ends the bare value.
		{stdtime.Date(2027, 11, 5, 0, 0, 0, 0, stdtime.UTC), "%A, %B %d|pad=0, %Y", "Friday, November 5, 2027"},
		// `||` after a code is a literal `|`; after other text `|` is plain text.
		{single, "%d||%m", "02|01"},
		{single, "%d|pad=0||%m", "2|01"},
		{single, "a|b %%|x", "a|b %|x"},
		{single, "%A||", "Saturday|"},
	}
	for _, tc := range cases {
		got, err := strftimeFormat(tc.t, 0, tc.layout)
		if err != nil {
			t.Errorf("layout %q: %v", tc.layout, err)
			continue
		}
		if got != tc.want {
			t.Errorf("layout %q: got %q, want %q", tc.layout, got, tc.want)
		}
	}
}

// TestPadModifierRefused: a modifier on a code that takes none, a modifier
// other than pad, and malformed lists are errors in format and parse alike.
func TestPadModifierRefused(t *testing.T) {
	cases := []struct{ layout, want string }{
		{"%A|pad=0", "takes no modifiers"},
		{"%B|pad=2", "takes no modifiers"},
		{"%p|pad=0", "takes no modifiers"},
		{"%z|pad=0", "takes no modifiers"},
		{"%d|fill=0", `modifier "fill" not valid`},
		{"%d|bogus=1", `modifier "bogus" not valid`},
		{"%d|pad=0|pad=1", "specified twice"},
		{"%d|pad=-1", "pad must be >= 0"},
		{"%d|pad=x", "not an integer"},
		{"%d|pad", "missing `=`"},
		{"%d|pad=", "empty value"},
		{"%d|", "expected modifier key"},
		{"%d|-", "expected modifier key"},
	}
	instant := stdtime.Date(2027, 1, 2, 3, 4, 5, 0, stdtime.UTC)
	for _, tc := range cases {
		if _, err := strftimeFormat(instant, 0, tc.layout); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("format %q: got %v, want %q", tc.layout, err, tc.want)
		}
		if _, err := strftimeParse(tc.layout, "Saturday"); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("parse %q: got %v, want %q", tc.layout, err, tc.want)
		}
	}
}

// TestPadCapIsLimit: a pad past the cap is a reached cap, kind "limit".
func TestPadCapIsLimit(t *testing.T) {
	var ce *limits.CapError
	_, err := strftimeFormat(stdtime.Unix(0, 0).UTC(), 0, "%d|pad=4097")
	if !errors.As(err, &ce) {
		t.Fatalf("format: want a limits.CapError, got %T %v", err, err)
	}
	_, err = strftimeParse("%d|pad=99999999999999999999", "1")
	if !errors.As(err, &ce) {
		t.Fatalf("parse: want a limits.CapError, got %T %v", err, err)
	}
	got := runProg(t, `
		use io;
		use time;
		try {
			io.printf("%s", time.format(time.fromUnix(0), "%d|pad=5000"));
		} catch (e) {
			io.printf("%s", $e.kind);
		}
	`)
	if got != "limit" {
		t.Errorf("kind through time.format: got %q", got)
	}
}

// TestParsePadModifier: time.parse reads what time.format writes. pad=0 reads
// one digit up to the code's default width; a pad above the default reads
// exactly that many digits.
func TestParsePadModifier(t *testing.T) {
	instants := []stdtime.Time{
		stdtime.Date(2027, 1, 2, 3, 4, 5, 0, stdtime.UTC),
		stdtime.Date(2024, 12, 25, 23, 58, 59, 0, stdtime.UTC),
		stdtime.Date(2027, 11, 5, 12, 0, 9, 0, stdtime.UTC),
		stdtime.Date(5, 3, 1, 0, 0, 0, 0, stdtime.UTC),
	}
	layouts := []string{
		"%Y|pad=0-%m|pad=0-%d|pad=0 %H|pad=0:%M|pad=0:%S|pad=0",
		"%A, %B %d|pad=0, %Y|pad=0 %I|pad=0:%M:%S|pad=0 %p",
		"%Y|pad=6/%m|pad=3/%d|pad=1 %H|pad=1||%M|pad=4||%S",
		"%Y-%m-%d %H:%M:%S",
	}
	for _, layout := range layouts {
		for _, want := range instants {
			s, err := strftimeFormat(want, 0, layout)
			if err != nil {
				t.Fatalf("format %q: %v", layout, err)
			}
			got, err := strftimeParse(layout, s)
			if err != nil {
				t.Errorf("parse %q with %q: %v", s, layout, err)
				continue
			}
			if !got.Equal(want) {
				t.Errorf("parse %q with %q: got %v, want %v", s, layout, got, want)
			}
		}
	}
}

// TestParsePadDigitCounts: pad=0 takes 1..default digits (greedy), the default
// stays exactly the default width, and pad above it is exact.
func TestParsePadDigitCounts(t *testing.T) {
	ok := []struct{ layout, input string }{
		{"%d|pad=0.%m|pad=0", "5.11"},
		{"%d|pad=0.%m|pad=0", "05.1"},
		{"%d|pad=0%m", "511"}, // greedy: day 51 is out of range
		{"%Y|pad=0", "7"},
		{"%Y|pad=0", "2027"},
		{"%d|pad=3", "005"},
	}
	for i, tc := range ok {
		_, err := strftimeParse(tc.layout, tc.input)
		if (err == nil) != (i != 2) {
			t.Errorf("%q on %q: %v", tc.layout, tc.input, err)
		}
	}
	bad := []struct{ layout, input string }{
		{"%d", "5"},           // the default is exact
		{"%d|pad=0", "x"},     // no digit
		{"%d|pad=0", ""},      // nothing
		{"%Y|pad=0", "20271"}, // a fifth digit is trailing input
		{"%d|pad=3", "05"},    // pad above the default is exact
	}
	for _, tc := range bad {
		if _, err := strftimeParse(tc.layout, tc.input); err == nil {
			t.Errorf("%q on %q: accepted", tc.layout, tc.input)
		}
	}
}
