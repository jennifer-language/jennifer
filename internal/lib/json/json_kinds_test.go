// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package jsonlib

import (
	"math"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// M29.21: malformed input is kind "json", a cap stays "limit", and misuse
// (wrong argument types, unencodable program values) stays "runtime" ("").
func TestErrorKinds(t *testing.T) {
	str := interpreter.StringVal
	deep := strings.Repeat("[", limits.MaxNestingDepth+2)
	cases := []struct {
		name string
		call func() error
		kind string
	}{
		{"syntax", func() error { _, err := decodeFn([]interpreter.Value{str(`{"a":}`)}); return err }, "json"},
		{"trailing", func() error { _, err := decodeFn([]interpreter.Value{str(`1 2`)}); return err }, "json"},
		{"bad escape", func() error { _, err := decodeFn([]interpreter.Value{str(`"\uZZZZ"`)}); return err }, "json"},
		{"empty", func() error { _, err := decodeFn([]interpreter.Value{str(``)}); return err }, "json"},
		{"unterminated", func() error { _, err := decodeFn([]interpreter.Value{str(`"abc`)}); return err }, "json"},
		{"depth cap", func() error { _, err := decodeFn([]interpreter.Value{str(deep)}); return err }, "limit"},
		{"not a string", func() error { _, err := decodeFn([]interpreter.Value{interpreter.IntVal(1)}); return err }, ""},
		{"encode NaN", func() error {
			_, err := encodeFn([]interpreter.Value{interpreter.FloatVal(math.NaN())}, false)
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
