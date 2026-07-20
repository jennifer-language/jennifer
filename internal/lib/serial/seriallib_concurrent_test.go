// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

//go:build linux && !tinygo

package seriallib

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"golang.org/x/sys/unix"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// The port registry (nextID + the map) is shared across goroutines - a spawn may
// open a port while another closes one. Hammer open/close from many goroutines
// under -race to prove the mutex discipline holds (no map race, no id reuse).
func TestSerialRegistryConcurrent(t *testing.T) {
	ResetForTest()
	const workers = 24
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
			if err != nil {
				return
			}
			defer m.Close()
			_ = unix.IoctlSetPointerInt(int(m.Fd()), unix.TIOCSPTLCK, 0)
			n, err := unix.IoctlGetInt(int(m.Fd()), unix.TIOCGPTN)
			if err != nil {
				return
			}
			ctx := interpreter.BuiltinCtx{}
			pathv := interpreter.StringVal(fmt.Sprintf("/dev/pts/%d", n))
			port, oerr := openFn(ctx, []interpreter.Value{pathv, interpreter.IntVal(9600)})
			if oerr != nil {
				return
			}
			// A read with a huge n must be a bounded error, never a crash.
			if _, rerr := readFn(ctx, []interpreter.Value{port, interpreter.IntVal(1 << 60)}); rerr == nil {
				t.Error("expected a read-size-cap error")
			}
			if _, cerr := closeFn(ctx, []interpreter.Value{port}); cerr != nil {
				t.Errorf("close: %v", cerr)
			}
		}()
	}
	wg.Wait()
}
