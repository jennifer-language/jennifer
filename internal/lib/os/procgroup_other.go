// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !unix || tinygo

// Process groups and signals where there are no Unix signals (Windows), and
// for the TinyGo build, which has no os/exec (os.spawnWith and os.signal refuse
// there before reaching this file). os.SpawnOptions{group: true} is an error;
// os.signal delivers "term" and "kill", both as a hard stop (Process.Kill), as
// os.kill does on Windows; the other names are errors.
package oslib

import (
	"errors"
	"fmt"
	stdos "os"
	"os/exec"
	"runtime"
)

func setProcessGroup(*exec.Cmd) error {
	return fmt.Errorf("group is supported only on Unix (this is %s)", runtime.GOOS)
}

func signalChild(p *stdos.Process, name string) error {
	switch name {
	case "term", "kill":
		return p.Kill()
	}
	return fmt.Errorf("signal %q is not available on %s; only \"term\" and \"kill\" (both stop the process)", name, runtime.GOOS)
}

func signalGroup(int, string) error {
	return errors.New("process groups are supported only on Unix")
}
