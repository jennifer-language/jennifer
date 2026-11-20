// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build unix

package fslib

import (
	"fmt"
	"os"
	"syscall"
)

// openSecret opens path for fs.readSecret and checks the open handle. The
// open is non-blocking so a FIFO named as a secret is refused instead of
// waiting for a writer; for a regular file the flag changes nothing.
func openSecret(fn, path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("%s: %s: %s", fn, path, osErr(err, path))
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("%s: %s: %s", fn, path, osErr(err, path))
	}
	if refusal := secretProblem(fn, path, st, mountReadOnly(f)); refusal != "" {
		f.Close()
		return nil, &secretRefusal{msg: refusal}
	}
	return f, nil
}

// secretProblem says why a file may not hold a secret, or "" when it may.
//
// On a writable filesystem the file must be owner-only and owned by the
// effective user or root, as OpenSSH requires of a private key. On a
// read-only mount, the way container runtimes hand secrets in (Docker and
// Compose `/run/secrets`, Kubernetes secret volumes), nothing here can
// replace the file and the operator who made the mount chose who reads it:
// group and other may read and any owner is accepted, but a file group or
// others could write at its source is still refused.
func secretProblem(fn, path string, st os.FileInfo, readOnly bool) string {
	if !st.Mode().IsRegular() {
		return fmt.Sprintf("%s: %s is not a regular file", fn, path)
	}
	perm := st.Mode().Perm()
	if readOnly {
		if perm&0o022 != 0 {
			return fmt.Sprintf("%s: %s is writable by group or others (mode %04o), even on a read-only mount; make it 0400 or 0444 where it is mounted from", fn, path, perm)
		}
		return ""
	}
	if perm&0o077 != 0 {
		return fmt.Sprintf("%s: %s has group or other permissions (mode %04o); run chmod 600 %s", fn, path, perm, path)
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Sprintf("%s: %s: cannot read the file's owner", fn, path)
	}
	if euid := os.Geteuid(); int(sys.Uid) != euid && sys.Uid != 0 {
		return fmt.Sprintf("%s: %s is owned by uid %d, not by you (uid %d) or root", fn, path, sys.Uid, euid)
	}
	return ""
}
