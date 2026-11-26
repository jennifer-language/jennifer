// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package fslib_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSyncDir (M29.57): fs.syncDir flushes a directory after a rename into it;
// a regular file or a missing path is an error.
func TestSyncDir(t *testing.T) {
	dir := t.TempDir()
	tmp := filepath.Join(dir, "a.tmp")
	dst := filepath.Join(dir, "a")
	prog := fmt.Sprintf(`use fs; fs.writeString(%q, "x"); fs.rename(%q, %q); fs.syncDir(%q);`, tmp, tmp, dst, dir)
	if _, err := runProg(t, prog); err != nil {
		t.Fatalf("syncDir on a directory: %v", err)
	}
	if b, err := os.ReadFile(dst); err != nil || string(b) != "x" {
		t.Fatalf("renamed file: %q, %v", b, err)
	}
	for _, c := range []struct{ path, want string }{
		{dst, "fs.syncDir: " + dst + ": not a directory"},
		{filepath.Join(dir, "missing"), "fs.syncDir: " + filepath.Join(dir, "missing") + ": "},
	} {
		_, err := runProg(t, fmt.Sprintf(`use fs; fs.syncDir(%q);`, c.path))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("syncDir(%s): err %v, want %q", c.path, err, c.want)
		}
	}
}
