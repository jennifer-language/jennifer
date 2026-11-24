// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package lint_test

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/lint"
)

// TestDeprecatedSQL: L301 reports the deprecated `sql` driver aliases written
// as a literal and `sql.setQueryTimeout` (M29.36), and leaves the current
// forms alone.
func TestDeprecatedSQL(t *testing.T) {
	cfg := lint.DefaultConfig()
	cases := []struct {
		name string
		src  string
		want int
		frag string
	}{
		{"open mariadb", "use sql;\ndef c as sql.Connection init sql.open(\"mariadb\", \"u:p@/db\");", 1,
			"the `sql.open` driver \"mariadb\" is deprecated and removed in 0.29.0; use \"mysql\""},
		{"openWith postgresql", "use sql;\ndef c as sql.Connection init sql.openWith(\"postgresql\", \"host=/tmp\", sql.Options{});", 1,
			"the `sql.openWith` driver \"postgresql\" is deprecated and removed in 0.29.0; use \"postgres\""},
		{"aliased", "use sql as db;\ndef c as db.Connection init db.open(\"mariadb\", \"u:p@/db\");", 1, "the `db.open` driver \"mariadb\""},
		{"setQueryTimeout", "use sql;\nsql.setQueryTimeout(0);", 1,
			"`sql.setQueryTimeout` is deprecated and removed in 0.29.0; use `sql.openWith(driver, dsn, sql.Options{queryTimeoutMs: ...})`"},
		{"open mysql", "use sql;\ndef c as sql.Connection init sql.open(\"mysql\", \"u:p@/db\");", 0, ""},
		{"openWith postgres", "use sql;\ndef c as sql.Connection init sql.openWith(\"postgres\", \"host=/tmp\", sql.Options{queryTimeoutMs: 5000});", 0, ""},
		{"driver from a variable", "use sql;\ndef d as string init \"mariadb\";\ndef c as sql.Connection init sql.open($d, \"u:p@/db\");", 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			diags := lintSrcResolved(t, c.src, only("L301"), cfg)
			if got := countID(diags, "L301"); got != c.want {
				t.Fatalf("L301 count = %d, want %d: %v", got, c.want, diags)
			}
			for _, d := range diags {
				if d.ID == "L301" && !strings.Contains(d.Message, c.frag) {
					t.Errorf("L301 message %q lacks %q", d.Message, c.frag)
				}
			}
		})
	}
}
