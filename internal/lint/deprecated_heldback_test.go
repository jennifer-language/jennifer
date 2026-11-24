// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package lint_test

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/lint"
)

// TestDeprecatedImagesRgba: L301 reports images.rgba (M29.48), naming the
// images.Color literal; images.rgb, images.hex and the literal are not
// reported.
func TestDeprecatedImagesRgba(t *testing.T) {
	cfg := lint.DefaultConfig()
	cases := []struct {
		name string
		src  string
		want int
		frag string
	}{
		{"rgba", "use images;\ndef c as images.Color init images.rgba(1, 2, 3, 4);", 1,
			"`images.rgba` is deprecated and removed in 0.29.0; use `images.Color{r: ..., g: ..., b: ..., a: ...}`"},
		{"rgba alias", "use images as im;\ndef c as im.Color init im.rgba(1, 2, 3, 4);", 1, "`im.rgba` is deprecated"},
		{"rgb", "use images;\ndef c as images.Color init images.rgb(1, 2, 3);", 0, ""},
		{"hex", "use images;\ndef c as images.Color init images.hex(\"#01020304\");", 0, ""},
		{"literal", "use images;\ndef c as images.Color init images.Color{r: 1, g: 2, b: 3, a: 4};", 0, ""},
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
