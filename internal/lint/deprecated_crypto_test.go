// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package lint_test

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/lint"
)

// TestDeprecatedCrypto: L301 reports a literal "rsa-sha*" algorithm in
// crypto.sign / verify and a password whose static type is string in
// hashPassword / verifyPassword (M29.51). A non-literal algorithm and a bytes
// password are not reported, and the argument-type check (L110) accepts the
// deprecated string password.
func TestDeprecatedCrypto(t *testing.T) {
	cfg := lint.DefaultConfig()
	const head = "use crypto;\nuse encoding;\ndef m as bytes init encoding.encode(\"utf-8\", \"m\");\ndef k as bytes init $m;\ndef h as string init \"$argon2id$\";\n"
	cases := []struct {
		name string
		src  string
		want int
		frag string
	}{
		{"sign rsa-sha256", head + "def s as bytes init crypto.sign(\"rsa-sha256\", $m, $k);", 1,
			"`crypto.sign(\"rsa-sha256\", ...)` is deprecated and removed in 0.29.0; use \"rsa-pkcs1-sha256\""},
		{"verify rsa-sha512", head + "def ok as bool init crypto.verify(\"rsa-sha512\", $m, $m, $k);", 1, "use \"rsa-pkcs1-sha512\""},
		{"aliased sign rsa-sha384", "use crypto as c;\ndef b as bytes;\ndef s as bytes init c.sign(\"rsa-sha384\", $b, $b);", 1, "`c.sign(\"rsa-sha384\", ...)`"},
		{"sign rsa-pkcs1-sha256", head + "def s as bytes init crypto.sign(\"rsa-pkcs1-sha256\", $m, $k);", 0, ""},
		{"sign rsa-pss-sha256", head + "def s as bytes init crypto.sign(\"rsa-pss-sha256\", $m, $k);", 0, ""},
		{"non-literal algorithm", head + "def a as string init \"rsa-sha256\";\ndef s as bytes init crypto.sign($a, $m, $k);", 0, ""},
		{"hashPassword string literal", head + "def x as string init crypto.hashPassword(\"argon2id\", \"pw\");", 1,
			"a `string` password to `crypto.hashPassword` is deprecated and removed in 0.29.0; pass `bytes`: `encoding.encode(\"utf-8\", password)`"},
		{"hashPassword string variable", head + "def pw as string init \"pw\";\ndef x as string init crypto.hashPassword(\"argon2id\", $pw);", 1, "`crypto.hashPassword`"},
		{"hashPassword bytes", head + "def x as string init crypto.hashPassword(\"argon2id\", $m);", 0, ""},
		{"hashPassword encoded", head + "def x as string init crypto.hashPassword(\"argon2id\", encoding.encode(\"utf-8\", \"pw\"));", 0, ""},
		{"verifyPassword string", head + "def ok as bool init crypto.verifyPassword(\"pw\", $h);", 1, "`crypto.verifyPassword`"},
		{"verifyPassword bytes", head + "def ok as bool init crypto.verifyPassword($m, $h);", 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			diags := lintSrcResolved(t, c.src, only("L301", "L109", "L110"), cfg)
			if got := countID(diags, "L301"); got != c.want {
				t.Errorf("L301 count = %d, want %d: %v", got, c.want, diags)
			}
			if n := countID(diags, "L109") + countID(diags, "L110"); n != 0 {
				t.Errorf("L109/L110 also reported: %v", diags)
			}
			for _, d := range diags {
				if d.ID == "L301" && !strings.Contains(d.Message, c.frag) {
					t.Errorf("L301 message %q lacks %q", d.Message, c.frag)
				}
			}
		})
	}
}
