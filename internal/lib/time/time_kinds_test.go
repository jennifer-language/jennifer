// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package timelib

import (
	"testing"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// M29.21: an input string that does not parse is kind "time", a reached cap
// stays "limit", and misuse (a malformed layout, wrong argument types, an
// out-of-range program value) stays "runtime" ("").
func TestErrorKinds(t *testing.T) {
	str := interpreter.StringVal
	ctx := interpreter.BuiltinCtx{}
	parse := func(s, layout string) func() error {
		return func() error { _, err := parseFn(ctx, []interpreter.Value{str(s), str(layout)}); return err }
	}
	fromIso := func(s string) func() error {
		return func() error { _, err := fromIsoFn(ctx, []interpreter.Value{str(s)}); return err }
	}
	cases := []struct {
		name string
		call func() error
		kind string
	}{
		{"literal mismatch", parse("2024/06/15", "%Y-%m-%d"), "time"},
		{"short digits", parse("24-06-15", "%Y-%m-%d"), "time"},
		{"month range", parse("2024-13-01", "%Y-%m-%d"), "time"},
		{"day range", parse("2024-06-32", "%Y-%m-%d"), "time"},
		{"hour range", parse("24", "%H"), "time"},
		{"am/pm", parse("07 XM", "%I %p"), "time"},
		{"am/pm short", parse("07 ", "%I %p"), "time"},
		{"zone", parse("x0100", "%z"), "time"},
		{"zone range", parse("+2700", "%z"), "time"},
		{"weekday", parse("Mondai", "%A"), "time"},
		{"month name", parse("Foo", "%b"), "time"},
		{"literal percent", parse("x", "%%"), "time"},
		{"trailing input", parse("2024x", "%Y"), "time"},
		{"impossible date", parse("2023-02-29", "%Y-%m-%d"), "time"},
		{"out of range", parse("9999", "%Y"), "time"},
		{"iso malformed", fromIso("2024-06-15 12:00"), "time"},
		{"iso out of range", fromIso("9999-01-01T00:00:00Z"), "time"},
		{"pad cap", parse("1", "%Y|pad=5000"), "limit"},
		{"unknown verb", parse("x", "%Q"), ""},
		{"trailing % in layout", parse("x", "%"), ""},
		{"bad modifier", parse("1", "%Y|foo=1"), ""},
		{"parse not a string", func() error {
			_, err := parseFn(ctx, []interpreter.Value{interpreter.IntVal(1), str("%Y")})
			return err
		}, ""},
		{"iso no args", func() error { _, err := fromIsoFn(ctx, nil); return err }, ""},
		{"fromUnix overflow", func() error {
			_, err := fromUnixFn(ctx, []interpreter.Value{interpreter.IntVal(1 << 62)})
			return err
		}, ""},
	}
	for _, c := range cases {
		err := c.call()
		if err == nil {
			t.Errorf("%s: no error", c.name)
			continue
		}
		if got := dataerr.KindOf(err); got != c.kind {
			t.Errorf("%s: %v: kind %q, want %q", c.name, err, got, c.kind)
		}
	}
}
