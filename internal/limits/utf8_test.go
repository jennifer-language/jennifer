// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package limits

import "testing"

func TestInvalidUTF8At(t *testing.T) {
	cases := []struct {
		s    string
		want int
	}{
		{"", -1}, {"abc", -1}, {"héllo €", -1}, {"�", -1},
		{"\xff", 0}, {"ab\xffcd", 2}, {"é\xc3", 2}, {"a\xed\xa0\x80", 1}, // truncated rune, surrogate
		{"a\xc0\xafb", 1}, // overlong
	}
	for _, c := range cases {
		if got := InvalidUTF8At(c.s); got != c.want {
			t.Errorf("InvalidUTF8At(%q) = %d, want %d", c.s, got, c.want)
		}
	}
	if err := CheckUTF8("io.readLine", "ok\x80"); err == nil || err.Error() != "io.readLine: invalid UTF-8 at byte 2" {
		t.Fatalf("CheckUTF8: %v", err)
	}
	if err := CheckUTF8("x", "ok"); err != nil {
		t.Fatal(err)
	}
}
