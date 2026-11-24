// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build linux && !tinygo

package spilib

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lib/devio"
)

// fakeDriver stands in for spidev: it answers SPI_IOC_MESSAGE(1) by echoing
// tx into rx with every byte inverted, refuses a message over its bufsiz with
// EMSGSIZE (as spidev_message does), and accepts the configuration ioctls.
type fakeDriver struct {
	mu       sync.Mutex
	bufsiz   int
	messages int
	speeds   []uint32
	hold     time.Duration // how long a transfer takes
}

func (d *fakeDriver) ioctl(_ int, req uintptr, p unsafe.Pointer) error {
	if req != spiMessage1() {
		return nil
	}
	x := (*spiIocTransfer)(p)
	d.mu.Lock()
	d.messages++
	d.speeds = append(d.speeds, x.speedHz)
	bufsiz := d.bufsiz
	d.mu.Unlock()
	if int(x.length) > bufsiz {
		return unix.EMSGSIZE
	}
	time.Sleep(d.hold)
	// The buffer addresses sit in u64 fields; the field's leading
	// pointer-sized word holds them on 64-bit and little-endian 32-bit targets.
	tx := unsafe.Slice(*(**byte)(unsafe.Pointer(&x.txBuf)), x.length)
	rx := unsafe.Slice(*(**byte)(unsafe.Pointer(&x.rxBuf)), x.length)
	for i := range tx {
		rx[i] = ^tx[i]
	}
	return nil
}

// withFake installs d for the test and registers a device (a regular file,
// whose fd the fake ignores) with the given recorded bufsiz.
func withFake(t *testing.T, d *fakeDriver, bufsiz int) Value {
	t.Helper()
	ResetForTest()
	old := ioctl
	ioctl = d.ioctl
	t.Cleanup(func() { ioctl = old })
	f, err := os.Create(filepath.Join(t.TempDir(), "spidev"))
	if err != nil {
		t.Fatal(err)
	}
	devsMu.Lock()
	nextID++
	id := nextID
	devs[id] = &devState{f: f, speedHz: 500000, bufsiz: bufsiz}
	devsMu.Unlock()
	return devio.Handle(LibraryName, "Device", id)
}

func errKind(err error) (string, string) {
	sig, ok := err.(*interpreter.ErrorSignal)
	if !ok {
		return "runtime", err.Error()
	}
	var kind, msg string
	for _, f := range sig.Value.Fields() {
		switch f.Name {
		case "kind":
			kind = f.Value.Str()
		case "message":
			msg = f.Value.Str()
		}
	}
	return kind, msg
}

// TestTransferOverBufsiz: a transfer over spidev's bufsiz used to reach the
// kernel and fail with a bare "message too long"; it is now kind "limit",
// naming bufsiz and how to raise it, and is not split (which would drop chip
// select mid-transfer). Exactly bufsiz bytes still goes out as one message.
func TestTransferOverBufsiz(t *testing.T) {
	d := &fakeDriver{bufsiz: 4096}
	h := withFake(t, d, 4096)
	v, err := transferFn(interpreter.BuiltinCtx{}, []Value{h, interpreter.BytesVal(make([]byte, 4096))})
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Bytes()) != 4096 || v.Bytes()[0] != 0xff {
		t.Fatalf("rx: %d bytes, first %#x", len(v.Bytes()), v.Bytes()[0])
	}
	_, err = transferFn(interpreter.BuiltinCtx{}, []Value{h, interpreter.BytesVal(make([]byte, 4097))})
	if err == nil {
		t.Fatal("4097 bytes: no error")
	}
	kind, msg := errKind(err)
	if kind != "limit" || !strings.Contains(msg, "4097 bytes exceed the spidev buffer of 4096 bytes") ||
		!strings.Contains(msg, "spidev.bufsiz") {
		t.Errorf("got %s %q", kind, msg)
	}
	if d.messages != 1 {
		t.Errorf("driver saw %d messages, want 1 (the over-size one must not be sent or split)", d.messages)
	}
}

// TestTransferEMSGSIZE: a driver buffer smaller than the recorded bufsiz (the
// parameter unreadable, a different driver) still surfaces as kind "limit".
func TestTransferEMSGSIZE(t *testing.T) {
	h := withFake(t, &fakeDriver{bufsiz: 16}, 4096)
	_, err := transferFn(interpreter.BuiltinCtx{}, []Value{h, interpreter.BytesVal(make([]byte, 17))})
	if kind, _ := errKind(err); kind != "limit" {
		t.Errorf("got %v", err)
	}
}

// TestSpidevBufsiz: the module parameter is read when present, else 4096.
func TestSpidevBufsiz(t *testing.T) {
	old := bufsizPath
	t.Cleanup(func() { bufsizPath = old })
	p := filepath.Join(t.TempDir(), "bufsiz")
	bufsizPath = p
	if got := spidevBufsiz(); got != 4096 {
		t.Errorf("missing parameter: %d", got)
	}
	if err := os.WriteFile(p, []byte("65536\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := spidevBufsiz(); got != 65536 {
		t.Errorf("65536: got %d", got)
	}
}

// TestCloseWaitsForTransfer: close used to release the fd while a transfer in
// another task was still in the ioctl, and configure wrote speedHz while a
// transfer read it (a data race under -race). Both now wait for the device.
func TestCloseWaitsForTransfer(t *testing.T) {
	d := &fakeDriver{bufsiz: 4096, hold: 100 * time.Millisecond}
	h := withFake(t, d, 4096)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = transferFn(interpreter.BuiltinCtx{}, []Value{h, interpreter.BytesVal([]byte{1, 2})})
	}()
	go func() {
		defer wg.Done()
		_, _ = configureFn(interpreter.BuiltinCtx{}, []Value{h, interpreter.IntVal(0), interpreter.IntVal(1000000)})
	}()
	time.Sleep(20 * time.Millisecond)
	start := time.Now()
	if _, err := closeFn(interpreter.BuiltinCtx{}, []Value{h}); err != nil {
		t.Fatal(err)
	}
	took := time.Since(start)
	wg.Wait()
	d.mu.Lock()
	n := d.messages
	d.mu.Unlock()
	if n == 1 && took < 50*time.Millisecond {
		t.Errorf("close returned after %v while a transfer was in flight", took)
	}
}
