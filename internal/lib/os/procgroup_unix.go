// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build unix && !tinygo

// Unix process groups and signals for spawned children: os.SpawnOptions.group
// starts the child as the leader of a new process group (setpgid), and
// os.signal sends any of its signal names to the child or to that group. The
// TinyGo build has no os/exec (and its syscall has no Setpgid), so it uses
// procgroup_other.go; os.spawnWith and os.signal refuse before reaching it.
package oslib

import (
	"errors"
	"fmt"
	stdos "os"
	"os/exec"
	"syscall"
)

// setProcessGroup makes the child the leader of a new process group whose id
// is the child's pid.
func setProcessGroup(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return nil
}

// sendSignal maps an os.signal name to its signal.
func sendSignal(name string) (syscall.Signal, error) {
	switch name {
	case "term":
		return syscall.SIGTERM, nil
	case "kill":
		return syscall.SIGKILL, nil
	case "int":
		return syscall.SIGINT, nil
	case "hup":
		return syscall.SIGHUP, nil
	case "quit":
		return syscall.SIGQUIT, nil
	case "usr1":
		return syscall.SIGUSR1, nil
	case "usr2":
		return syscall.SIGUSR2, nil
	}
	return 0, fmt.Errorf("unknown signal %q", name)
}

// signalChild sends name to the child alone. os.Process.Signal does not signal
// a reaped pid (it reports os.ErrProcessDone), so a recycled pid is safe.
func signalChild(p *stdos.Process, name string) error {
	sig, err := sendSignal(name)
	if err != nil {
		return err
	}
	return p.Signal(sig)
}

// signalGroup sends name to every member of the process group pgid. ESRCH (no
// member left) is no error: the group has already gone.
func signalGroup(pgid int, name string) error {
	sig, err := sendSignal(name)
	if err != nil {
		return err
	}
	if pgid <= 0 {
		return fmt.Errorf("invalid process group %d", pgid)
	}
	if err := syscall.Kill(-pgid, sig); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}
