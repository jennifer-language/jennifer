// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package timelib implements Jennifer's `time` library: the
// core type set, Unix conversions, and arithmetic. Formatting, parsing,
// and the fixed-offset Zone type live in format.go and zone.go.
//
// Two namespaced structs anchor the library:
//
//   - `time.Time { nanos as int, offset as int }` - an instant on the
//     wall-clock timeline. `nanos` is the UTC nanosecond count since the
//     Unix epoch; `offset` is the seconds east of UTC the instant is
//     interpreted in when calendar accessors run. Fields are private API;
//     users interact through the function set.
//   - `time.Duration { nanos as int }` - a signed span of nanoseconds.
//
// The Go package is named timelib to avoid colliding with Go's standard
// `time` package, which this implementation depends on.
package timelib

import (
	"fmt"
	stdtime "time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// LibraryName is the Jennifer name programs `use` to enable these names,
// and doubles as the namespace prefix.
const LibraryName = "time"

// nowFunc is the wall-clock source. Tests swap it to get deterministic
// output without rewriting the constructor surface.
var nowFunc = stdtime.Now

// sleepFunc is the pause primitive `time.sleep` calls. Defaults to the
// real Go sleep; tests swap it to a no-op (or to a clock-advancing
// stub) so the test suite doesn't actually block. Parallels nowFunc.
var sleepFunc = stdtime.Sleep

// Install registers the `time` library: two structs and the
// function set (constructors, accessors, arithmetic, comparison).
func Install(in *interpreter.Interpreter) {
	intT := parser.PrimitiveType(parser.TypeInt)
	strT := parser.PrimitiveType(parser.TypeString)
	in.RegisterNamespacedStruct(LibraryName, "Time", []parser.StructField{
		{Name: "nanos", Type: intT},
		{Name: "offset", Type: intT},
	})
	in.RegisterNamespacedStruct(LibraryName, "Duration", []parser.StructField{
		{Name: "nanos", Type: intT},
	})
	in.RegisterNamespacedStruct(LibraryName, "Zone", []parser.StructField{
		{Name: "offset", Type: intT},
		{Name: "name", Type: strT},
	})

	in.RegisterNamespaced(LibraryName, "now", nowFn)
	in.RegisterNamespaced(LibraryName, "utc", utcFn)
	in.RegisterNamespaced(LibraryName, "fromUnix", fromUnixFn)
	in.RegisterNamespaced(LibraryName, "fromUnixMillis", fromUnixMillisFn)
	in.RegisterNamespaced(LibraryName, "fromUnixNanos", fromUnixNanosFn)

	in.RegisterNamespaced(LibraryName, "unix", unixFn)
	in.RegisterNamespaced(LibraryName, "unixMillis", unixMillisFn)
	in.RegisterNamespaced(LibraryName, "unixNanos", unixNanosFn)

	in.RegisterNamespaced(LibraryName, "year", yearFn)
	in.RegisterNamespaced(LibraryName, "month", monthFn)
	in.RegisterNamespaced(LibraryName, "day", dayFn)
	in.RegisterNamespaced(LibraryName, "hour", hourFn)
	in.RegisterNamespaced(LibraryName, "minute", minuteFn)
	in.RegisterNamespaced(LibraryName, "second", secondFn)
	in.RegisterNamespaced(LibraryName, "nanosecond", nanosecondFn)
	in.RegisterNamespaced(LibraryName, "weekday", weekdayFn)

	in.RegisterNamespaced(LibraryName, "fromSeconds", fromSecondsFn)
	in.RegisterNamespaced(LibraryName, "fromMilliseconds", fromMillisecondsFn)
	in.RegisterNamespaced(LibraryName, "fromMinutes", fromMinutesFn)
	in.RegisterNamespaced(LibraryName, "fromHours", fromHoursFn)

	in.RegisterNamespaced(LibraryName, "seconds", secondsFn)
	in.RegisterNamespaced(LibraryName, "milliseconds", millisecondsFn)
	in.RegisterNamespaced(LibraryName, "minutes", minutesFn)
	in.RegisterNamespaced(LibraryName, "hours", hoursFn)

	in.RegisterNamespaced(LibraryName, "add", addFn)
	in.RegisterNamespaced(LibraryName, "sub", subFn)
	in.RegisterNamespaced(LibraryName, "before", beforeFn)
	in.RegisterNamespaced(LibraryName, "after", afterFn)
	in.RegisterNamespaced(LibraryName, "equal", equalFn)
	in.RegisterNamespaced(LibraryName, "sleep", sleepFn)

	// Zones, format/parse, ISO round-trip.
	in.RegisterNamespaced(LibraryName, "zone", zoneFn)
	in.RegisterNamespaced(LibraryName, "inZone", inZoneFn)
	in.RegisterNamespaced(LibraryName, "local", localFn)
	in.RegisterNamespacedConst(LibraryName, "UTC", utcConst)

	// `time.PROGRAM_START` captures the moment the time library was
	// installed. In the CLI / REPL that's the start of the user's
	// program run, before the source file is even read (see main.go
	// and repl.go, which call every library's Install up front).
	// Captured through `nowFunc` so tests that freeze the clock pick
	// up the frozen instant.
	in.RegisterNamespacedConst(LibraryName, "PROGRAM_START", makeTimeUnchecked(nowFunc()))

	in.RegisterNamespaced(LibraryName, "format", formatFn)
	in.RegisterNamespaced(LibraryName, "parse", parseFn)
	in.RegisterNamespaced(LibraryName, "iso", isoFn)
	in.RegisterNamespaced(LibraryName, "fromIso", fromIsoFn)
}

// The instant range a Time can represent: UnixNano is stored in an int64, so
// an instant before ~1678 or after ~2262 has no representation and UnixNano
// would silently wrap to a wrong date. Boundaries are the times at the int64
// nanosecond extremes.
var (
	minRepresentable = stdtime.Unix(0, -1<<63)
	maxRepresentable = stdtime.Unix(0, 1<<63-1)
)

// makeTime constructs a `time.Time{...}` Value from a Go time.Time, rejecting
// an instant outside the representable int64-nanosecond range. Use it wherever
// the instant derives from user input (parse, fromUnix, arithmetic).
func makeTime(t stdtime.Time) (interpreter.Value, error) {
	if t.Before(minRepresentable) || t.After(maxRepresentable) {
		return interpreter.Null(), fmt.Errorf("time %s is outside the representable range (roughly year 1678 to 2262)", t.UTC().Format("2006-01-02T15:04:05Z"))
	}
	return makeTimeUnchecked(t), nil
}

// makeTimeUnchecked skips the range check for instants known to be in range
// (the wall clock, and a nanosecond count that is itself the stored value).
func makeTimeUnchecked(t stdtime.Time) interpreter.Value {
	_, offset := t.Zone()
	return interpreter.NamespacedStructVal(LibraryName, "Time", []interpreter.StructField{
		{Name: "nanos", Value: interpreter.IntVal(t.UnixNano())},
		{Name: "offset", Value: interpreter.IntVal(int64(offset))},
	})
}

// makeDuration constructs a `time.Duration{nanos}` Value.
func makeDuration(nanos int64) interpreter.Value {
	return interpreter.NamespacedStructVal(LibraryName, "Duration", []interpreter.StructField{
		{Name: "nanos", Value: interpreter.IntVal(nanos)},
	})
}

// extractTime pulls (nanos, offset) out of a `time.Time` Value, with
// boundary type checking. Mismatched argument kind / wrong struct
// shape are plain Go errors; the interpreter positions them at the
// call site.
func extractTime(fnName string, v interpreter.Value) (int64, int64, error) {
	if v.Kind != interpreter.KindStruct || v.StructNS != LibraryName || v.StructName != "Time" {
		return 0, 0, fmt.Errorf("%s: argument must be a time.Time, got %s", fnName, v.Kind)
	}
	var nanos, offset int64
	var sawNanos, sawOffset bool
	for _, f := range v.Fields {
		switch f.Name {
		case "nanos":
			if f.Value.Kind != interpreter.KindInt {
				return 0, 0, fmt.Errorf("%s: time.Time.nanos is not int (got %s)", fnName, f.Value.Kind)
			}
			nanos = f.Value.Int
			sawNanos = true
		case "offset":
			if f.Value.Kind != interpreter.KindInt {
				return 0, 0, fmt.Errorf("%s: time.Time.offset is not int (got %s)", fnName, f.Value.Kind)
			}
			offset = f.Value.Int
			sawOffset = true
		}
	}
	if !sawNanos || !sawOffset {
		return 0, 0, fmt.Errorf("%s: time.Time missing nanos/offset field", fnName)
	}
	return nanos, offset, nil
}

// extractDuration pulls nanos out of a `time.Duration` Value.
func extractDuration(fnName string, v interpreter.Value) (int64, error) {
	if v.Kind != interpreter.KindStruct || v.StructNS != LibraryName || v.StructName != "Duration" {
		return 0, fmt.Errorf("%s: argument must be a time.Duration, got %s", fnName, v.Kind)
	}
	for _, f := range v.Fields {
		if f.Name == "nanos" {
			if f.Value.Kind != interpreter.KindInt {
				return 0, fmt.Errorf("%s: time.Duration.nanos is not int (got %s)", fnName, f.Value.Kind)
			}
			return f.Value.Int, nil
		}
	}
	return 0, fmt.Errorf("%s: time.Duration has no nanos field", fnName)
}

// goTimeFrom reconstructs a Go time.Time from the stored (nanos, offset)
// pair so calendar accessors can lean on Go's calendar math.
func goTimeFrom(nanos, offset int64) stdtime.Time {
	loc := stdtime.FixedZone("", int(offset))
	return stdtime.Unix(0, nanos).In(loc)
}

// ----- constructors ---------------------------------------------------

func nowFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 0 {
		return interpreter.Null(), fmt.Errorf("time.now expects 0 arguments, got %d", len(args))
	}
	return makeTimeUnchecked(nowFunc()), nil
}

func utcFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 0 {
		return interpreter.Null(), fmt.Errorf("time.utc expects 0 arguments, got %d", len(args))
	}
	return makeTimeUnchecked(nowFunc().UTC()), nil
}

func intArg(fnName, paramName string, args []interpreter.Value) (int64, error) {
	if len(args) != 1 {
		return 0, fmt.Errorf("%s expects 1 argument (%s), got %d", fnName, paramName, len(args))
	}
	if args[0].Kind != interpreter.KindInt {
		return 0, fmt.Errorf("%s: %s must be int, got %s", fnName, paramName, args[0].Kind)
	}
	return args[0].Int, nil
}

func fromUnixFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	seconds, err := intArg("time.fromUnix", "seconds", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return makeTime(stdtime.Unix(seconds, 0).UTC())
}

func fromUnixMillisFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	ms, err := intArg("time.fromUnixMillis", "milliseconds", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return makeTime(stdtime.UnixMilli(ms).UTC())
}

func fromUnixNanosFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	ns, err := intArg("time.fromUnixNanos", "nanoseconds", args)
	if err != nil {
		return interpreter.Null(), err
	}
	// A nanosecond count is exactly the stored value, always in range.
	return makeTimeUnchecked(stdtime.Unix(0, ns).UTC()), nil
}

// ----- unix accessors -------------------------------------------------

func unixFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("time.unix expects 1 argument, got %d", len(args))
	}
	nanos, _, err := extractTime("time.unix", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	// Unix seconds floor toward negative infinity; Go integer division
	// truncates toward zero, so a pre-epoch instant would be off by one.
	// stdtime.Unix(0, nanos).Unix() floors correctly.
	return interpreter.IntVal(stdtime.Unix(0, nanos).Unix()), nil
}

func unixMillisFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("time.unixMillis expects 1 argument, got %d", len(args))
	}
	nanos, _, err := extractTime("time.unixMillis", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	// Floor toward negative infinity (see time.unix) for pre-epoch instants.
	return interpreter.IntVal(stdtime.Unix(0, nanos).UnixMilli()), nil
}

func unixNanosFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("time.unixNanos expects 1 argument, got %d", len(args))
	}
	nanos, _, err := extractTime("time.unixNanos", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.IntVal(nanos), nil
}

// ----- calendar accessors --------------------------------------------

func calendarPart(fnName string, args []interpreter.Value, pick func(stdtime.Time) int64) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("%s expects 1 argument, got %d", fnName, len(args))
	}
	nanos, offset, err := extractTime(fnName, args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.IntVal(pick(goTimeFrom(nanos, offset))), nil
}

func yearFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return calendarPart("time.year", args, func(t stdtime.Time) int64 { return int64(t.Year()) })
}

func monthFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return calendarPart("time.month", args, func(t stdtime.Time) int64 { return int64(t.Month()) })
}

func dayFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return calendarPart("time.day", args, func(t stdtime.Time) int64 { return int64(t.Day()) })
}

func hourFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return calendarPart("time.hour", args, func(t stdtime.Time) int64 { return int64(t.Hour()) })
}

func minuteFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return calendarPart("time.minute", args, func(t stdtime.Time) int64 { return int64(t.Minute()) })
}

func secondFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return calendarPart("time.second", args, func(t stdtime.Time) int64 { return int64(t.Second()) })
}

func nanosecondFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return calendarPart("time.nanosecond", args, func(t stdtime.Time) int64 { return int64(t.Nanosecond()) })
}

// weekdayFn returns the ISO 8601 weekday: Monday=1 ... Sunday=7. Go's
// own Weekday() is Sunday=0 ... Saturday=6; we remap so the API stays
// consistent with Jennifer's 1-based calendar accessors (month 1-12,
// day 1-31).
func weekdayFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return calendarPart("time.weekday", args, func(t stdtime.Time) int64 {
		// Go: Sunday=0, Monday=1, ..., Saturday=6.
		// ISO: Monday=1, Tuesday=2, ..., Sunday=7.
		wd := int64(t.Weekday())
		if wd == 0 {
			return 7
		}
		return wd
	})
}

// ----- duration constructors -----------------------------------------

func fromSecondsFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	n, err := intArg("time.fromSeconds", "seconds", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return makeDuration(n * int64(stdtime.Second)), nil
}

func fromMillisecondsFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	n, err := intArg("time.fromMilliseconds", "milliseconds", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return makeDuration(n * int64(stdtime.Millisecond)), nil
}

func fromMinutesFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	n, err := intArg("time.fromMinutes", "minutes", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return makeDuration(n * int64(stdtime.Minute)), nil
}

func fromHoursFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	n, err := intArg("time.fromHours", "hours", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return makeDuration(n * int64(stdtime.Hour)), nil
}

// ----- duration accessors --------------------------------------------

func durationAs(fnName string, args []interpreter.Value, unit int64) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("%s expects 1 argument, got %d", fnName, len(args))
	}
	nanos, err := extractDuration(fnName, args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.IntVal(nanos / unit), nil
}

func secondsFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return durationAs("time.seconds", args, int64(stdtime.Second))
}

func millisecondsFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return durationAs("time.milliseconds", args, int64(stdtime.Millisecond))
}

func minutesFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return durationAs("time.minutes", args, int64(stdtime.Minute))
}

func hoursFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return durationAs("time.hours", args, int64(stdtime.Hour))
}

// ----- arithmetic ----------------------------------------------------

func addFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("time.add expects 2 arguments (time, duration), got %d", len(args))
	}
	nanos, offset, err := extractTime("time.add", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	dn, err := extractDuration("time.add", args[1])
	if err != nil {
		return interpreter.Null(), err
	}
	// Detect int64 wrap in the nanosecond sum before it lands an instant in a
	// wrong era (a wrapped value could otherwise pass the range check).
	sum := nanos + dn
	if (nanos > 0 && dn > 0 && sum < 0) || (nanos < 0 && dn < 0 && sum >= 0) {
		return interpreter.Null(), fmt.Errorf("time.add: result overflows the representable range")
	}
	loc := stdtime.FixedZone("", int(offset))
	result := stdtime.Unix(0, sum).In(loc)
	return makeTime(result)
}

func subFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("time.sub expects 2 arguments (time, time), got %d", len(args))
	}
	an, _, err := extractTime("time.sub", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	bn, _, err := extractTime("time.sub", args[1])
	if err != nil {
		return interpreter.Null(), err
	}
	return makeDuration(an - bn), nil
}

func cmpFn(fnName string, args []interpreter.Value, pick func(a, b int64) bool) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("%s expects 2 arguments (time, time), got %d", fnName, len(args))
	}
	an, _, err := extractTime(fnName, args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	bn, _, err := extractTime(fnName, args[1])
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.BoolVal(pick(an, bn)), nil
}

func beforeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return cmpFn("time.before", args, func(a, b int64) bool { return a < b })
}

func afterFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return cmpFn("time.after", args, func(a, b int64) bool { return a > b })
}

func equalFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return cmpFn("time.equal", args, func(a, b int64) bool { return a == b })
}

// sleepFn implements `time.sleep($d)`. Blocks the current goroutine
// for the requested duration; a negative or zero `Duration` returns
// immediately (matches Go's `time.Sleep`, sparing callers a check on
// every "computed delay might be <= 0" path). Returns null - callers
// who want the wake-up instant call `time.now()` afterward.
func sleepFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("time.sleep expects 1 argument (duration), got %d", len(args))
	}
	nanos, err := extractDuration("time.sleep", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	sleepFunc(stdtime.Duration(nanos))
	return interpreter.Null(), nil
}
