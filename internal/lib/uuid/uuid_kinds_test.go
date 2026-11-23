// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package uuidlib

import (
	"testing"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// M29.21: a malformed UUID string is kind "uuid"; misuse stays "runtime" ("").
func TestErrorKinds(t *testing.T) {
	str := interpreter.StringVal
	cases := []struct {
		name string
		call func() error
		kind string
	}{
		{"parse malformed", func() error {
			_, err := parseFn(interpreter.BuiltinCtx{}, []interpreter.Value{str("not-a-uuid")})
			return err
		}, "uuid"},
		{"parse bad hex", func() error {
			_, err := parseFn(interpreter.BuiltinCtx{}, []interpreter.Value{str("zzzzzzzz-0000-0000-0000-000000000000")})
			return err
		}, "uuid"},
		{"version malformed", func() error { _, err := versionFn(interpreter.BuiltinCtx{}, []interpreter.Value{str("")}); return err }, "uuid"},
		{"parse not a string", func() error {
			_, err := parseFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(1)})
			return err
		}, ""},
		{"isValid no args", func() error { _, err := isValidFn(interpreter.BuiltinCtx{}, nil); return err }, ""},
		{"v4 with args", func() error { _, err := v4Fn(interpreter.BuiltinCtx{}, []interpreter.Value{str("x")}); return err }, ""},
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
