// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build unix

package fslib_test

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// A handle that cannot seek (a FIFO) throws on fs.seek and fs.tell; reading
// it still works afterwards.
func TestSeekTellFifo(t *testing.T) {
	pipe := filepath.Join(t.TempDir(), "p")
	if err := syscall.Mkfifo(pipe, 0o600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		w, err := os.OpenFile(pipe, os.O_WRONLY, 0)
		if err != nil {
			return
		}
		_, _ = w.WriteString("data\n")
		_ = w.Close()
	}()
	out, err := runProg(t, fmt.Sprintf(`
		use io;
		use fs;
		def f as fs.File init fs.open(%q, "read");
		try { fs.seek($f, 0); } catch (e) { io.printf("seek %%s\n", $e.kind); }
		try { fs.tell($f); } catch (e) { io.printf("tell %%s\n", $e.kind); }
		io.printf("%%s\n", fs.readLine($f));
		fs.close($f);
	`, pipe))
	<-done
	if err != nil {
		t.Fatal(err)
	}
	if want := "seek runtime\ntell runtime\ndata\n"; out != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
}
