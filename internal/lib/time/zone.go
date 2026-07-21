// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Zone handling: a `time.Zone` struct with an
// integer offset (seconds east of UTC) and a display name, plus the
// `time.UTC` constant, `time.local()` reader, and `time.inZone($t, $z)`
// shifter. IANA zone names and daylight-saving transitions are
// deliberately not part of the core library; the `timezones.j`
// module supplies that data as ordinary Jennifer source.

package timelib

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// maxOffsetSeconds caps |offset| at 26 hours to keep the value in a
// plausible UTC-offset range. Real zones span -12:00 to +14:00; the
// extra margin tolerates any historical or non-IANA experiment a user
// chooses to model. Anything wider is almost certainly a bug.
const maxOffsetSeconds = 26 * 60 * 60

// makeZone constructs a `time.Zone{offset, name}` Value.
func makeZone(offset int64, name string) interpreter.Value {
	return interpreter.NamespacedStructVal(LibraryName, "Zone", []interpreter.StructField{
		{Name: "offset", Value: interpreter.IntVal(offset)},
		{Name: "name", Value: interpreter.StringVal(name)},
	})
}

// extractZone unpacks a `time.Zone` argument into (offset, name).
func extractZone(fnName string, v interpreter.Value) (int64, string, error) {
	if v.Kind != interpreter.KindStruct || v.StructNS != LibraryName || v.StructName != "Zone" {
		return 0, "", fmt.Errorf("%s: argument must be a time.Zone, got %s", fnName, v.Kind)
	}
	var offset int64
	var name string
	var sawOffset, sawName bool
	for _, f := range v.Fields {
		switch f.Name {
		case "offset":
			if f.Value.Kind != interpreter.KindInt {
				return 0, "", fmt.Errorf("%s: time.Zone.offset is not int (got %s)", fnName, f.Value.Kind)
			}
			offset = f.Value.Int
			sawOffset = true
		case "name":
			if f.Value.Kind != interpreter.KindString {
				return 0, "", fmt.Errorf("%s: time.Zone.name is not string (got %s)", fnName, f.Value.Kind)
			}
			name = f.Value.Str
			sawName = true
		}
	}
	if !sawOffset || !sawName {
		return 0, "", fmt.Errorf("%s: time.Zone missing offset/name field", fnName)
	}
	return offset, name, nil
}

// zoneFn implements `time.zone(offset, name)`. Validates the offset
// against the soft maxOffsetSeconds cap; name is free-form (empty
// string is allowed and triggers a synthesized "+HHMM" / "Z" display
// in formatters).
func zoneFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("time.zone expects 2 arguments (offset, name), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("time.zone: offset must be int, got %s", args[0].Kind)
	}
	if args[1].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("time.zone: name must be string, got %s", args[1].Kind)
	}
	offset := args[0].Int
	if offset > maxOffsetSeconds || offset < -maxOffsetSeconds {
		return interpreter.Null(), fmt.Errorf(
			"time.zone: offset %d is outside +/-%d seconds (~26h cap)",
			offset, maxOffsetSeconds,
		)
	}
	return makeZone(offset, args[1].Str), nil
}

// inZoneFn implements `time.inZone($t, $z)`. The UTC instant is
// preserved; the stored offset is replaced so calendar accessors and
// formatters render the same moment in `$z`'s wall-clock.
func inZoneFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("time.inZone expects 2 arguments (time, zone), got %d", len(args))
	}
	nanos, _, err := extractTime("time.inZone", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	offset, _, err := extractZone("time.inZone", args[1])
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.NamespacedStructVal(LibraryName, "Time", []interpreter.StructField{
		{Name: "nanos", Value: interpreter.IntVal(nanos)},
		{Name: "offset", Value: interpreter.IntVal(offset)},
	}), nil
}

// localFn implements `time.local()`. Reads the host's current zone
// (its name + offset) and returns a `time.Zone`. The name comes from
// the OS-supplied zone abbreviation (e.g. "CET", "EDT", "UTC"); when
// the OS gives an empty name we fall back to the numeric offset.
func localFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 0 {
		return interpreter.Null(), fmt.Errorf("time.local expects 0 arguments, got %d", len(args))
	}
	name, off := nowFunc().Zone()
	if name == "" {
		name = formatOffsetStrftime(int64(off))
	}
	return makeZone(int64(off), name), nil
}

// formatOffsetStrftime renders a numeric offset in POSIX strftime
// `%z` style: always `+HHMM` / `-HHMM`, including `+0000` for UTC.
// Used by the strftime `%z` verb and as the fallback display name
// when the host supplies no zone abbreviation.
func formatOffsetStrftime(offset int64) string {
	sign := byte('+')
	if offset < 0 {
		sign = '-'
		offset = -offset
	}
	hours := offset / 3600
	minutes := (offset % 3600) / 60
	return fmt.Sprintf("%c%02d%02d", sign, hours, minutes)
}

// formatOffsetISO renders the same offset in ISO 8601 colon form:
// "+01:00", "-05:30", "Z".
func formatOffsetISO(offset int64) string {
	if offset == 0 {
		return "Z"
	}
	sign := byte('+')
	if offset < 0 {
		sign = '-'
		offset = -offset
	}
	hours := offset / 3600
	minutes := (offset % 3600) / 60
	return fmt.Sprintf("%c%02d:%02d", sign, hours, minutes)
}

// utcConst is the canonical `time.UTC` Zone value. Registered as a
// namespaced constant during Install.
var utcConst = makeZone(0, "UTC")
