// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package version

import "testing"

func withVersion(v string, fn func()) {
	saved := Version
	Version = v
	defer func() { Version = saved }()
	fn()
}

func TestBelow(t *testing.T) {
	cases := []struct {
		build, max string
		want       bool
	}{
		{"0.25.9", "0.26.0", true},
		{"0.26.0", "0.26.0", false},
		{"0.27.1", "0.26.0", false},
		{"0.9.0", "0.10.0", true},
		{"dev", "99.0.0", false},
		{"0.25.0-dev+131.afd9207", "0.26.0", false},
	}
	for _, c := range cases {
		withVersion(c.build, func() {
			got, err := Below(c.max)
			if err != nil || got != c.want {
				t.Errorf("build %s Below(%s) = %v, %v; want %v", c.build, c.max, got, err, c.want)
			}
		})
	}
	if _, err := Below("0.26"); err == nil {
		t.Error("Below(\"0.26\") should be an error")
	}
}

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.25.0", "0.26.0", -1},
		{"0.26.0", "0.26.0", 0},
		{"1.0.0", "0.99.99", 1},
		{"0.10.0", "0.9.0", 1},
		{"0.25.0-dev+1.abc", "0.25.0", 0},
	}
	for _, c := range cases {
		if got, err := Compare(c.a, c.b); err != nil || got != c.want {
			t.Errorf("Compare(%s, %s) = %d, %v; want %d", c.a, c.b, got, err, c.want)
		}
	}
}
