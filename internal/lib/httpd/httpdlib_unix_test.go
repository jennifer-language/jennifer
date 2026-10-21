// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo && unix

package httpdlib

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// A unix socket is never reachable under a looser mode than 0660, even with a
// umask of 0: it appears at its path already tightened. A watcher polls the
// path through repeated listen / shutdown cycles and records any mode it sees.
func TestUnixSocketNeverWorldConnectable(t *testing.T) {
	ResetForTest()
	old := syscall.Umask(0)
	defer syscall.Umask(old)
	dir := t.TempDir()
	sock := filepath.Join(dir, "httpd.sock")

	var loose atomic.Uint32
	var stop atomic.Bool
	watched := make(chan struct{})
	go func() {
		defer close(watched)
		for !stop.Load() {
			if fi, err := os.Lstat(sock); err == nil && fi.Mode().Perm()&0o007 != 0 {
				loose.Store(uint32(fi.Mode().Perm()))
			}
		}
	}()
	for i := 0; i < 200; i++ {
		srv, err := listenFn(noCtx, []Value{interpreter.StringVal("unix:" + sock)})
		if err != nil {
			stop.Store(true)
			t.Fatalf("listen %d: %v", i, err)
		}
		_, _ = shutdownFn(noCtx, []Value{srv})
	}
	stop.Store(true)
	<-watched
	if m := loose.Load(); m != 0 {
		t.Errorf("socket was visible with mode %o before it was tightened to 0660", m)
	}

	// The published socket is 0660, shutdown removes it, and no private
	// directory is left behind.
	srv, err := listenFn(noCtx, []Value{interpreter.StringVal("unix:" + sock)})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	if fi, err := os.Lstat(sock); err != nil || fi.Mode().Perm() != 0o660 {
		t.Errorf("socket mode = %v, %v; want 0660", fi.Mode().Perm(), err)
	}
	// A second server cannot take over a live socket.
	if _, err := listenFn(noCtx, []Value{interpreter.StringVal("unix:" + sock)}); err == nil || !strings.Contains(err.Error(), "address already in use") {
		t.Errorf("second listen on a live socket: err = %v", err)
	}
	_, _ = shutdownFn(noCtx, []Value{srv})
	if _, err := os.Lstat(sock); !os.IsNotExist(err) {
		t.Errorf("socket left behind after shutdown: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		t.Errorf("leftover entry %q in the socket directory", e.Name())
	}
}
