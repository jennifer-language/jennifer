// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package timelib

import (
	"strings"
	"testing"
	stdtime "time"
)

// TestFormatBasicVerbs covers the v1 strftime code set against a
// known instant: 2024-06-15T12:34:56Z, a Saturday in June.
func TestFormatBasicVerbs(t *testing.T) {
	instant := stdtime.Unix(1718454896, 0).UTC()
	cases := []struct {
		layout, want string
	}{
		{"%Y", "2024"},
		{"%m", "06"},
		{"%d", "15"},
		{"%H", "12"},
		{"%M", "34"},
		{"%S", "56"},
		{"%z", "+0000"},
		{"%a", "Sat"},
		{"%A", "Saturday"},
		{"%b", "Jun"},
		{"%B", "June"},
		{"%j", "167"},
		{"%u", "6"},
		{"%%", "%"},
		{"%Y-%m-%d", "2024-06-15"},
		{"%H:%M:%S", "12:34:56"},
		{"plain text %Y here", "plain text 2024 here"},
	}
	for _, tc := range cases {
		got, err := strftimeFormat(instant, 0, tc.layout)
		if err != nil {
			t.Errorf("layout %q: %v", tc.layout, err)
			continue
		}
		if got != tc.want {
			t.Errorf("layout %q: got %q, want %q", tc.layout, got, tc.want)
		}
	}
}

// TestFormatOffsetCET: %z for a +01:00 zone is +0100, not Z.
func TestFormatOffsetCET(t *testing.T) {
	instant := stdtime.Unix(1718454896, 0).In(stdtime.FixedZone("CET", 3600))
	got, err := strftimeFormat(instant, 3600, "%H %z")
	if err != nil {
		t.Fatal(err)
	}
	if got != "13 +0100" {
		t.Errorf("got %q, want %q", got, "13 +0100")
	}
}

// TestFormatUnknownVerb: an unrecognised verb is a positioned error.
func TestFormatUnknownVerb(t *testing.T) {
	_, err := strftimeFormat(stdtime.Unix(0, 0).UTC(), 0, "%Q")
	if err == nil {
		t.Fatal("expected error for unknown verb, got nil")
	}
	if !strings.Contains(err.Error(), "%Q") {
		t.Errorf("error doesn't mention verb: %v", err)
	}
}

// TestFormatTrailingPercent: a bare % at end is rejected.
func TestFormatTrailingPercent(t *testing.T) {
	_, err := strftimeFormat(stdtime.Unix(0, 0).UTC(), 0, "noon %")
	if err == nil {
		t.Fatal("expected trailing-%% error")
	}
}

// TestParseBasic: round-trip a printable date + time.
func TestParseBasic(t *testing.T) {
	parsed, err := strftimeParse("%Y-%m-%d %H:%M:%S", "2024-06-15 12:34:56")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Unix() != 1718454896 {
		t.Errorf("unix = %d, want 1718454896", parsed.Unix())
	}
}

// TestParseZone: `+0530` is parsed into a 5h30m offset.
func TestParseZone(t *testing.T) {
	parsed, err := strftimeParse("%Y-%m-%d %H:%M:%S %z", "2024-06-15 12:00:00 +0530")
	if err != nil {
		t.Fatal(err)
	}
	_, off := parsed.Zone()
	if off != 5*3600+30*60 {
		t.Errorf("offset = %d, want %d", off, 5*3600+30*60)
	}
}

// TestParseZoneZ: `Z` is accepted as offset 0 (lenient).
func TestParseZoneZ(t *testing.T) {
	parsed, err := strftimeParse("%Y-%m-%dT%H:%M:%S%z", "2024-06-15T12:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	_, off := parsed.Zone()
	if off != 0 {
		t.Errorf("offset = %d, want 0", off)
	}
}

// TestParseMonthNames: %B / %b match full and short forms,
// case-insensitively.
func TestParseMonthNames(t *testing.T) {
	cases := []struct {
		layout, input string
		wantMonth     int
	}{
		{"%B %d %Y", "June 15 2024", 6},
		{"%b %d %Y", "Jun 15 2024", 6},
		{"%B %d %Y", "JANUARY 01 2024", 1},
		{"%b %d %Y", "feb 29 2024", 2},
	}
	for _, tc := range cases {
		parsed, err := strftimeParse(tc.layout, tc.input)
		if err != nil {
			t.Errorf("%q against %q: %v", tc.input, tc.layout, err)
			continue
		}
		if int(parsed.Month()) != tc.wantMonth {
			t.Errorf("%q: month = %d, want %d", tc.input, parsed.Month(), tc.wantMonth)
		}
	}
}

// TestParseRejectsTrailingInput: leftover bytes after the layout is
// consumed are an error.
func TestParseRejectsTrailingInput(t *testing.T) {
	_, err := strftimeParse("%Y", "2024 hello")
	if err == nil {
		t.Fatal("expected trailing-input error")
	}
}

// TestParseRejectsRangeViolation: month 13 errors.
func TestParseRejectsRangeViolation(t *testing.T) {
	_, err := strftimeParse("%Y-%m-%d", "2024-13-01")
	if err == nil {
		t.Fatal("expected range-violation error")
	}
	if !strings.Contains(err.Error(), "month") {
		t.Errorf("error doesn't mention month: %v", err)
	}
}

// TestISOFormatRoundTrip: time.iso emits Z for UTC, +HH:MM otherwise.
func TestISOFormatRoundTrip(t *testing.T) {
	cases := []struct {
		nanos, offset int64
		want          string
	}{
		{1718454896 * 1_000_000_000, 0, "2024-06-15T12:34:56Z"},
		{1718454896 * 1_000_000_000, 3600, "2024-06-15T13:34:56+01:00"},
		{1718454896*1_000_000_000 + 500_000_000, 0, "2024-06-15T12:34:56.5Z"},
	}
	for _, tc := range cases {
		got := formatISO(goTimeFrom(tc.nanos, tc.offset), tc.offset)
		if got != tc.want {
			t.Errorf("nanos=%d offset=%d: got %q, want %q",
				tc.nanos, tc.offset, got, tc.want)
		}
	}
}

// TestEndToEndJenniferRoundTrip exercises format -> parse via the
// public Jennifer surface to confirm strftime layout cross-talk works
// inside the interpreter.
func TestEndToEndJenniferRoundTrip(t *testing.T) {
	got := runProg(t, `
		use io;
		use time;
		def t as time.Time init time.fromUnix(1718454896);
		def s as string init time.format($t, "%Y-%m-%dT%H:%M:%S%z");
		def back as time.Time init time.parse($s, "%Y-%m-%dT%H:%M:%S%z");
		io.printf("%s %d", $s, time.unix($back));
	`)
	want := "2024-06-15T12:34:56+0000 1718454896"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestZoneConstructorAndInZone: time.zone + time.inZone preserve the
// instant while shifting the wall-clock offset.
func TestZoneConstructorAndInZone(t *testing.T) {
	got := runProg(t, `
		use io;
		use time;
		def vienna as time.Zone init time.zone(3600, "CET");
		def t as time.Time init time.fromUnix(1718452800);
		def tv as time.Time init time.inZone($t, $vienna);
		io.printf("%d %d", time.unix($tv), $tv.offset);
	`)
	want := "1718452800 3600"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestZoneOffsetCapRejected: |offset| > 26h errors.
func TestZoneOffsetCapRejected(t *testing.T) {
	err := expectErr(t, `
		use time;
		def z as time.Zone init time.zone(200000, "weird");
	`)
	if !strings.Contains(err, "cap") && !strings.Contains(err, "outside") {
		t.Errorf("error doesn't mention the cap: %v", err)
	}
}

// TestUTCConstant: time.UTC is a Zone{0, "UTC"}; coexists with the
// time.utc() function in the same library because they live in
// separate namespace maps (NSConstants vs NSBuiltins).
func TestUTCConstant(t *testing.T) {
	got := runProg(t, `
		use io;
		use time;
		def z as time.Zone init time.UTC;
		io.printf("%s %d", $z.name, $z.offset);
	`)
	want := "UTC 0"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestUtcFunctionStillWorks: case-sensitivity means time.utc() (the
// function) and time.UTC (the constant) coexist.
func TestUtcFunctionStillWorks(t *testing.T) {
	frozenAt(t, stdtime.Unix(0, 0).UTC())
	got := runProg(t, `
		use io;
		use time;
		def t as time.Time init time.utc();
		io.printf("%d", time.unix($t));
	`)
	if got != "0" {
		t.Errorf("time.utc() = %q, want 0", got)
	}
}

// TestLocalReturnsZone: time.local() reads the frozen clock's zone.
func TestLocalReturnsZone(t *testing.T) {
	frozenAt(t, stdtime.Unix(0, 0).In(stdtime.FixedZone("XYZ", 7200)))
	got := runProg(t, `
		use io;
		use time;
		def z as time.Zone init time.local();
		io.printf("%s %d", $z.name, $z.offset);
	`)
	want := "XYZ 7200"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFromIsoFractional: fractional seconds survive the round trip.
func TestFromIsoFractional(t *testing.T) {
	got := runProg(t, `
		use io;
		use time;
		def t as time.Time init time.fromIso("2024-06-15T12:00:00.123456789Z");
		io.printf("%d", time.unixNanos($t));
	`)
	// 2024-06-15T12:00:00Z is 1718452800; nanos = 1718452800 * 1e9 + 123456789.
	want := "1718452800123456789"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFromIsoErrorPositioned: invalid ISO input returns a positioned
// runtime error mentioning the function name.
func TestFromIsoErrorPositioned(t *testing.T) {
	err := expectErr(t, `
		use time;
		def t as time.Time init time.fromIso("not an iso string");
	`)
	if !strings.Contains(err, "time.fromIso") {
		t.Errorf("error doesn't mention time.fromIso: %v", err)
	}
}

// TestProgramStartCapturedAtInstall: time.PROGRAM_START is the moment
// the time library was installed, captured through `nowFunc` so the
// test's frozen clock determines the value.
func TestProgramStartCapturedAtInstall(t *testing.T) {
	frozenAt(t, stdtime.Unix(1718452800, 0).UTC())
	got := runProg(t, `
		use io;
		use time;
		io.printf("%d", time.unix(time.PROGRAM_START));
	`)
	if got != "1718452800" {
		t.Errorf("time.PROGRAM_START unix = %q, want 1718452800", got)
	}
}

// TestProgramStartIsStable: multiple reads see the same instant
// (it's a constant; the clock moving on doesn't affect it).
func TestProgramStartIsStable(t *testing.T) {
	frozenAt(t, stdtime.Unix(1718452800, 0).UTC())
	got := runProg(t, `
		use io;
		use time;
		def a as time.Time init time.PROGRAM_START;
		def b as time.Time init time.PROGRAM_START;
		io.printf("%t", time.equal($a, $b));
	`)
	if got != "true" {
		t.Errorf("time.PROGRAM_START not stable across reads: %q", got)
	}
}

// TestSleepHonorsHook: tests swap `sleepFunc` so the suite doesn't
// actually pause. The hook captures the duration the call requested
// so we can assert it was forwarded correctly.
func TestSleepHonorsHook(t *testing.T) {
	var got stdtime.Duration
	prev := sleepFunc
	sleepFunc = func(d stdtime.Duration) { got = d }
	t.Cleanup(func() { sleepFunc = prev })

	out := runProg(t, `
		use time;
		time.sleep(time.fromMilliseconds(500));
	`)
	if out != "" {
		t.Errorf("expected no stdout output, got %q", out)
	}
	if got != 500*stdtime.Millisecond {
		t.Errorf("sleepFunc received %v, want %v", got, 500*stdtime.Millisecond)
	}
}

// TestSleepReturnsNull verifies the return type by binding it to
// `def r as null init time.sleep(...)`. A non-null return would
// fail Jennifer's declared-type check at the init site.
func TestSleepReturnsNull(t *testing.T) {
	prev := sleepFunc
	sleepFunc = func(_ stdtime.Duration) {}
	t.Cleanup(func() { sleepFunc = prev })

	got := runProg(t, `
		use io;
		use time;
		def r as null init time.sleep(time.fromMilliseconds(1));
		io.printf("after");
	`)
	if got != "after" {
		t.Errorf("program output = %q, want %q (and null return implied by `as null`)", got, "after")
	}
}

// TestSleepNegativeIsNoOp: negative durations forward through to the
// hook (which would no-op anyway under Go's time.Sleep), no error.
func TestSleepNegativeIsNoOp(t *testing.T) {
	called := false
	prev := sleepFunc
	sleepFunc = func(d stdtime.Duration) {
		called = true
		if d >= 0 {
			t.Errorf("expected negative duration, got %v", d)
		}
	}
	t.Cleanup(func() { sleepFunc = prev })

	runProg(t, `
		use time;
		time.sleep(time.sub(time.fromUnix(0), time.fromUnix(1)));
	`)
	if !called {
		t.Error("sleepFunc should have been called even with negative duration")
	}
}

// TestSleepRejectsNonDuration: passing a Time (or anything not a
// Duration) errors at the boundary.
func TestSleepRejectsNonDuration(t *testing.T) {
	err := expectErr(t, `
		use time;
		def t as time.Time init time.fromUnix(0);
		time.sleep($t);
	`)
	if !strings.Contains(err, "time.sleep") || !strings.Contains(err, "Duration") {
		t.Errorf("err lacks Duration / fn-name context: %v", err)
	}
}

// TestSleepArgCount enforces 1-arg arity.
func TestSleepArgCount(t *testing.T) {
	err := expectErr(t, `
		use time;
		time.sleep();
	`)
	if !strings.Contains(err, "time.sleep expects 1 argument") {
		t.Errorf("err lacks arity message: %v", err)
	}
}

// TestProgramStartAnchorsElapsed: composing time.PROGRAM_START with
// time.now() and time.sub produces the documented elapsed-time
// idiom. The clock advances 1500ms between Install and the now()
// call, so the elapsed Duration should report 1500.
func TestProgramStartAnchorsElapsed(t *testing.T) {
	base := stdtime.Unix(1718452800, 0).UTC()
	calls := 0
	prev := nowFunc
	nowFunc = func() stdtime.Time {
		// First call (during Install) returns base; later calls add 1.5 s.
		if calls == 0 {
			calls++
			return base
		}
		return base.Add(1500 * stdtime.Millisecond)
	}
	t.Cleanup(func() { nowFunc = prev })

	got := runProg(t, `
		use io;
		use time;
		def elapsed as time.Duration init time.sub(time.now(), time.PROGRAM_START);
		io.printf("%d", time.milliseconds($elapsed));
	`)
	if got != "1500" {
		t.Errorf("elapsed ms = %q, want 1500", got)
	}
}
