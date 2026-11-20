// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build unix

package fslib

import (
	"io/fs"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

// fakeInfo is a file's FileInfo with a chosen owner and mode, for the checks
// a test cannot set up without root: another owner, a read-only mount.
type fakeInfo struct {
	uid  uint32
	mode fs.FileMode
}

func (f fakeInfo) Name() string       { return "key" }
func (f fakeInfo) Size() int64        { return 1 }
func (f fakeInfo) Mode() fs.FileMode  { return f.mode }
func (f fakeInfo) ModTime() time.Time { return time.Time{} }
func (f fakeInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fakeInfo) Sys() any           { return &syscall.Stat_t{Uid: f.uid} }

// TestSecretOwner: on a writable filesystem a secret owned by another user
// is refused; the effective user and root pass.
func TestSecretOwner(t *testing.T) {
	me := uint32(os.Geteuid())
	if p := secretProblem("fs.readSecret", "/k", fakeInfo{me, 0o600}, false); p != "" {
		t.Errorf("own file refused: %s", p)
	}
	if p := secretProblem("fs.readSecret", "/k", fakeInfo{0, 0o600}, false); p != "" {
		t.Errorf("root's file refused: %s", p)
	}
	other := me + 4242
	if p := secretProblem("fs.readSecret", "/k", fakeInfo{other, 0o600}, false); !strings.Contains(p, "is owned by uid") {
		t.Errorf("another user's file: %q", p)
	}
}

// TestSecretOnReadOnlyMount: what container runtimes mount passes (Swarm's
// 0444 root, Kubernetes' 0644, a Compose file owned by the host user), the
// same files on a writable filesystem do not, and a file group or others
// could write is refused even on a read-only mount.
func TestSecretOnReadOnlyMount(t *testing.T) {
	other := uint32(os.Geteuid()) + 4242
	for _, f := range []fakeInfo{{0, 0o444}, {0, 0o644}, {0, 0o400}, {other, 0o600}, {other, 0o640}} {
		if p := secretProblem("fs.readSecret", "/run/secrets/k", f, true); p != "" {
			t.Errorf("read-only mount, uid %d mode %04o refused: %s", f.uid, f.mode, p)
		}
		if p := secretProblem("fs.readSecret", "/run/secrets/k", f, false); p == "" && (f.mode&0o077 != 0 || f.uid == other) {
			t.Errorf("writable fs, uid %d mode %04o accepted", f.uid, f.mode)
		}
	}
	for _, mode := range []fs.FileMode{0o666, 0o620, 0o602, 0o464} {
		p := secretProblem("fs.readSecret", "/run/secrets/k", fakeInfo{0, mode}, true)
		if !strings.Contains(p, "writable by group or others") {
			t.Errorf("read-only mount, mode %04o: %q", mode, p)
		}
	}
	if p := secretProblem("fs.readSecret", "/run/secrets", fakeInfo{0, fs.ModeDir | 0o555}, true); !strings.Contains(p, "not a regular file") {
		t.Errorf("directory on a read-only mount: %q", p)
	}
}
