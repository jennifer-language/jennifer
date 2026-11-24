// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package lint_test

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/lint"
)

// TestDeprecatedWaitShapes: L301 reports the optional-argument shapes 0.28.0
// deprecates for options structs (M29.45) and leaves their current forms alone.
func TestDeprecatedWaitShapes(t *testing.T) {
	cfg := lint.DefaultConfig()
	cases := []struct {
		name string
		src  string
		want int
		frag string
	}{
		{"fs.watch with an interval", "use fs;\ndef w as fs.Watcher init fs.watch(\".\", 100);", 1,
			"`fs.watch(path, intervalMs)` is deprecated and removed in 0.29.0; use `fs.watchWith(path, fs.WatchOptions{intervalMs: ...})`"},
		{"aliased fs.watch with an interval", "use fs as f;\ndef w as f.Watcher init f.watch(\".\", 100);", 1, "`f.watchWith(path, f.WatchOptions{"},
		{"fs.watch", "use fs;\ndef w as fs.Watcher init fs.watch(\".\");", 0, ""},
		{"fs.watchWith", "use fs;\ndef w as fs.Watcher init fs.watchWith(\".\", fs.WatchOptions{intervalMs: 100});", 0, ""},
		{"os.spawn with options", "use os;\ndef p as os.Process init os.spawn([\"true\"], {\"stdout\": \"inherit\"});", 1,
			"`os.spawn(argv, options)` is deprecated and removed in 0.29.0; use `os.spawnWith(argv, os.SpawnOptions{stdout: ..., stderr: ...})`"},
		{"os.spawn", "use os;\ndef p as os.Process init os.spawn([\"true\"]);", 0, ""},
		{"os.spawnWith", "use os;\ndef p as os.Process init os.spawnWith([\"true\"], os.SpawnOptions{stdout: \"inherit\"});", 0, ""},
		{"os.waitTimeout", "use os;\ndef p as os.Process init os.spawn([\"true\"]);\ndef r as os.Result init os.waitTimeout($p, 100);", 0, ""},
		{"os.run is untouched", "use os;\ndef r as os.Result init os.run([\"true\"], \"in\");", 0, ""},
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
