// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package main

import (
	"os"
	"os/exec"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/memlimit"
)

func TestMemoryLimitFor(t *testing.T) {
	if got := memoryLimitFor(10 << 30); got != 9<<30 {
		t.Errorf("memoryLimitFor(10 GiB) = %d, want %d", got, int64(9)<<30)
	}
}

// The interpreter sets a soft memory limit at startup unless GOMEMLIMIT is set.
// Checked in a child process, since the limit is process-wide: the child reads
// it back with debug.SetMemoryLimit(-1).
func TestStartupMemoryLimit(t *testing.T) {
	if os.Getenv("JENNIFER_MEMLIMIT_CHILD") == "1" {
		setMemoryLimit()
		os.Stdout.WriteString(strconv.FormatInt(debug.SetMemoryLimit(-1), 10))
		os.Exit(0)
	}
	run := func(env ...string) string {
		cmd := exec.Command(os.Args[0], "-test.run=^TestStartupMemoryLimit$")
		cmd.Env = append(os.Environ(), append([]string{"JENNIFER_MEMLIMIT_CHILD=1"}, env...)...)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("child: %v", err)
		}
		return strings.TrimSpace(string(out))
	}
	if got := run("GOMEMLIMIT=123MiB"); got != "128974848" {
		t.Errorf("with GOMEMLIMIT=123MiB the limit is %s, want it kept (128974848)", got)
	}
	limit, err := memlimit.Detect()
	if err != nil {
		t.Skipf("no machine memory limit detectable here: %v", err)
	}
	if got, want := run("GOMEMLIMIT="), strconv.FormatInt(memoryLimitFor(limit), 10); got != want {
		t.Errorf("startup limit is %s, want %s (90%% of %d)", got, want, limit)
	}
}
