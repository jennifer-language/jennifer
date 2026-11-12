// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package fslib

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// A write that fails after fs.writeNew's exclusive create (disk full, file
// size limit) removes the file, so no truncated file is left to block the
// next try; fs.writeString keeps its file as before.
func TestWriteNewRemovesAFailedFile(t *testing.T) {
	saved := writePayload
	t.Cleanup(func() { writePayload = saved })
	writePayload = func(f *os.File, payload []byte) error {
		f.Write(payload[:len(payload)/2])
		return errors.New("file too large")
	}
	dir := t.TempDir()
	fresh := filepath.Join(dir, "fresh")
	plain := filepath.Join(dir, "plain")
	_, err := writeNewFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal(fresh), interpreter.StringVal("0123456789")})
	if err == nil || err.Error() != "fs.writeNew: "+fresh+": file too large" {
		t.Fatalf("writeNew error = %v", err)
	}
	if _, serr := os.Lstat(fresh); !os.IsNotExist(serr) {
		t.Fatalf("writeNew left the file behind: %v", serr)
	}
	if _, err := writeStringFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal(plain), interpreter.StringVal("0123456789")}); err == nil {
		t.Fatal("writeString: expected the write error")
	}
	if _, serr := os.Lstat(plain); serr != nil {
		t.Fatalf("writeString's file is gone: %v", serr)
	}
}
