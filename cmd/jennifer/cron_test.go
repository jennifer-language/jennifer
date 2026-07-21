// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestCronModule drives the cron module through the real import path: parse a
// weekday schedule, compute the next fire (deterministic, from a fixed instant),
// and test matches both ways. A mismatch throws in the .j program and fails
// loadForTest. Pure calculator, no clock dependence.
func TestCronModule(t *testing.T) {
	cronMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "cron.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
use time;
import %q as cron;
def s as cron.Schedule init cron.parse("30 9 * * 1-5");
def n as time.Time init cron.next($s, time.fromIso("2026-03-14T10:30:00+00:00"));
testing.assertEqual(time.iso($n), "2026-03-16T09:30:00Z");
testing.assertTrue(cron.matches($s, time.fromIso("2026-03-16T09:30:00+00:00")));
testing.assertFalse(cron.matches($s, time.fromIso("2026-03-15T09:30:00+00:00")));
def every as cron.Schedule init cron.parse("*/15 * * * *");
testing.assertEqual(time.iso(cron.next($every, time.fromIso("2026-03-14T10:31:00+00:00"))), "2026-03-14T10:45:00Z");`, cronMod)
	progPath := filepath.Join(dir, "cron.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("cron program failed with code %d", code)
	}
}
