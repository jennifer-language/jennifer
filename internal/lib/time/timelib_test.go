// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package timelib

import (
	"bytes"
	"strings"
	"testing"
	stdtime "time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// frozenAt swaps the package clock to return a fixed Time for the
// duration of the test, restoring the previous source on return.
func frozenAt(t *testing.T, instant stdtime.Time) {
	t.Helper()
	prev := nowFunc
	nowFunc = func() stdtime.Time { return instant }
	t.Cleanup(func() { nowFunc = prev })
}

// runProg lexes, parses, and runs `src` against a fresh interpreter
// with `io` + `time` installed, returning stdout.
func runProg(t *testing.T, src string) string {
	t.Helper()
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	Install(in)
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := in.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	return buf.String()
}

// expectErr lexes, parses, and runs `src`. Fails the test if no error,
// otherwise returns the error message.
func expectErr(t *testing.T, src string) string {
	t.Helper()
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	Install(in)
	prog, err := parser.Parse(src)
	if err != nil {
		return err.Error()
	}
	err = in.Run(prog)
	if err == nil {
		t.Fatalf("expected error, got output %q", buf.String())
	}
	return err.Error()
}

// TestNowReturnsFrozenInstant: with the clock frozen at a known wall
// time, `time.now()` round-trips through `time.unix` to the same
// number of seconds.
func TestNowReturnsFrozenInstant(t *testing.T) {
	frozenAt(t, stdtime.Unix(1718452800, 0).UTC()) // 2024-06-15T12:00:00Z
	got := runProg(t, `
		use io;
		use time;
		def t as time.Time init time.now();
		io.printf("%d", time.unix($t));
	`)
	if got != "1718452800" {
		t.Errorf("time.unix(now) = %q, want 1718452800", got)
	}
}

// TestFromUnixRoundTrip: fromUnix -> unix returns the same int.
func TestFromUnixRoundTrip(t *testing.T) {
	got := runProg(t, `
		use io;
		use time;
		def t as time.Time init time.fromUnix(1577836800);
		io.printf("%d %d", time.unix($t), time.unixMillis($t));
	`)
	want := "1577836800 1577836800000"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// An instant outside the representable int64-nanosecond range must be rejected
// rather than silently wrapping to a wrong era. `time.parse("9999-...")` used to
// read back as ~1816.
func TestOutOfRangeInstantRejected(t *testing.T) {
	msg := expectErr(t, `use time; def t as time.Time init time.parse("9999-12-31", "%Y-%m-%d");`)
	if !strings.Contains(msg, "representable range") {
		t.Errorf("far-future parse: got %q", msg)
	}
	// A huge fromUnix value overflows UnixNano too.
	msg = expectErr(t, `use time; def t as time.Time init time.fromUnix(99999999999);`)
	if !strings.Contains(msg, "representable range") {
		t.Errorf("far-future fromUnix: got %q", msg)
	}
	// A normal date still parses.
	got := runProg(t, `use io; use time; def t as time.Time init time.parse("2024-06-15", "%Y-%m-%d"); io.printf("%d", time.year($t));`)
	if got != "2024" {
		t.Errorf("normal parse: got %q, want 2024", got)
	}
}

// Unix seconds / millis floor toward negative infinity, so a pre-epoch instant
// is not off by one (Go integer division truncates toward zero).
func TestPreEpochUnixFloors(t *testing.T) {
	// -1.5 seconds: floored Unix seconds is -2 (not -1), millis is -1500.
	got := runProg(t, `use io; use time;
def t as time.Time init time.fromUnixNanos(0 - 1500000000);
io.printf("%d %d", time.unix($t), time.unixMillis($t));`)
	if got != "-2 -1500" {
		t.Errorf("pre-epoch floor: got %q, want %q", got, "-2 -1500")
	}
}

// TestCalendarAccessors: known epoch + 1 second has year=1970, month=1,
// day=1, hour=0, minute=0, second=1, weekday=4 (Thursday is ISO 4).
func TestCalendarAccessors(t *testing.T) {
	got := runProg(t, `
		use io;
		use time;
		def t as time.Time init time.fromUnix(1);
		io.printf("%d %d %d %d %d %d %d",
			time.year($t), time.month($t), time.day($t),
			time.hour($t), time.minute($t), time.second($t),
			time.weekday($t));
	`)
	want := "1970 1 1 0 0 1 4"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestWeekdaySunday: 1970-01-04 was a Sunday; ISO weekday must be 7
// (Go's Weekday returns 0 for Sunday; the library remaps).
func TestWeekdaySunday(t *testing.T) {
	got := runProg(t, `
		use io;
		use time;
		def t as time.Time init time.fromUnix(259200);
		io.printf("%d", time.weekday($t));
	`)
	if got != "7" {
		t.Errorf("Sunday weekday = %q, want 7", got)
	}
}

// TestNanosecondAccessor: fromUnixNanos pins the fractional second.
func TestNanosecondAccessor(t *testing.T) {
	got := runProg(t, `
		use io;
		use time;
		def t as time.Time init time.fromUnixNanos(1500000123);
		io.printf("%d %d", time.unix($t), time.nanosecond($t));
	`)
	want := "1 500000123"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestDurationConstructorsAndAccessors: each constructor/accessor pair
// preserves the value at its native unit.
func TestDurationConstructorsAndAccessors(t *testing.T) {
	got := runProg(t, `
		use io;
		use time;
		def s as time.Duration init time.fromSeconds(90);
		def ms as time.Duration init time.fromMilliseconds(1500);
		def m as time.Duration init time.fromMinutes(2);
		def h as time.Duration init time.fromHours(3);
		io.printf("%d %d %d %d %d %d",
			time.seconds($s), time.milliseconds($s),
			time.milliseconds($ms),
			time.minutes($m), time.seconds($m),
			time.hours($h));
	`)
	want := "90 90000 1500 2 120 3"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestAddSubtract: add(time, duration) and sub(timeA, timeB) compose.
func TestAddSubtract(t *testing.T) {
	got := runProg(t, `
		use io;
		use time;
		def a as time.Time init time.fromUnix(1000);
		def step as time.Duration init time.fromSeconds(300);
		def b as time.Time init time.add($a, $step);
		def diff as time.Duration init time.sub($b, $a);
		io.printf("%d %d", time.unix($b), time.seconds($diff));
	`)
	want := "1300 300"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestNegativeDuration: sub(earlier, later) is negative.
func TestNegativeDuration(t *testing.T) {
	got := runProg(t, `
		use io;
		use time;
		def a as time.Time init time.fromUnix(1000);
		def b as time.Time init time.fromUnix(700);
		def diff as time.Duration init time.sub($a, $b);
		def neg as time.Duration init time.sub($b, $a);
		io.printf("%d %d", time.seconds($diff), time.seconds($neg));
	`)
	want := "300 -300"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestComparisons: before / after / equal honour Unix-instant order.
func TestComparisons(t *testing.T) {
	got := runProg(t, `
		use io;
		use time;
		def a as time.Time init time.fromUnix(1000);
		def b as time.Time init time.fromUnix(2000);
		def c as time.Time init time.fromUnix(1000);
		io.printf("%t %t %t %t %t %t",
			time.before($a, $b),
			time.after($a, $b),
			time.before($b, $a),
			time.equal($a, $c),
			time.equal($a, $b),
			time.after($b, $a));
	`)
	want := "true false false true false true"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestUtcCarriesZeroOffset: time.utc()'s stored offset is 0 even when
// the host's local zone isn't UTC.
func TestUtcCarriesZeroOffset(t *testing.T) {
	frozenAt(t, stdtime.Unix(1718452800, 0).In(stdtime.FixedZone("CET", 3600)))
	got := runProg(t, `
		use io;
		use time;
		def t as time.Time init time.utc();
		io.printf("%d", $t.offset);
	`)
	if got != "0" {
		t.Errorf("time.utc offset = %q, want 0", got)
	}
}

// TestArgCountMismatches: each constructor with the wrong arg count
// returns a positioned runtime error mentioning the function name.
func TestArgCountMismatches(t *testing.T) {
	cases := []struct {
		src     string
		wantSub string
	}{
		{`use time; def t as time.Time init time.now(0);`, "time.now expects 0 arguments"},
		{`use time; def t as time.Time init time.utc(0);`, "time.utc expects 0 arguments"},
		{`use time; def t as time.Time init time.fromUnix();`, "time.fromUnix expects 1 argument"},
		{`use time; def t as time.Time init time.fromUnix(1, 2);`, "time.fromUnix expects 1 argument"},
	}
	for _, tc := range cases {
		err := expectErr(t, tc.src)
		if !strings.Contains(err, tc.wantSub) {
			t.Errorf("for %q\n got: %v\nwant substring: %q", tc.src, err, tc.wantSub)
		}
	}
}

// TestWrongStructType: passing a Duration where a Time is expected (and
// vice versa) is rejected at the call boundary.
func TestWrongStructType(t *testing.T) {
	// Note: Jennifer's MatchesDeclared also catches the mismatch at the
	// def-site, so we pass a Time into time.seconds (which wants
	// Duration) via a less-typed intermediate.
	err := expectErr(t, `
		use time;
		def t as time.Time init time.fromUnix(1);
		def n as int init time.seconds($t);
	`)
	if !strings.Contains(err, "time.seconds") || !strings.Contains(err, "Duration") {
		t.Errorf("err = %v; want mention of time.seconds + Duration", err)
	}
}

// TestNonIntArgumentToConstructor: float to a fromUnix* constructor is
// rejected as int-only.
func TestNonIntArgumentToConstructor(t *testing.T) {
	err := expectErr(t, `
		use time;
		def t as time.Time init time.fromUnix(1.5);
	`)
	if !strings.Contains(err, "time.fromUnix") || !strings.Contains(err, "int") {
		t.Errorf("err = %v; want mention of time.fromUnix + int", err)
	}
}
