// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build linux && !tinygo

package i2clib

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// struct i2c_msg is { __u16 addr, flags, len; __u8 *buf; }: 12 bytes with buf
// at 8 on 32-bit targets (arm, 386), 16 bytes on 64-bit ones; struct
// i2c_rdwr_ioctl_data is { struct i2c_msg *msgs; __u32 nmsgs; }: 8 / 16 bytes.
// These compile-time checks fail to build on any GOARCH where the Go mirrors
// drift (go vet type-checks them for GOARCH=arm / arm64 / 386 too).
const ptrSize = unsafe.Sizeof(uintptr(0))

var (
	_ [unsafe.Sizeof(i2cMsg{}) - (8 + ptrSize)]byte
	_ [(8 + ptrSize) - unsafe.Sizeof(i2cMsg{})]byte
	_ [unsafe.Offsetof(i2cMsg{}.buf) - 8]byte
	_ [8 - unsafe.Offsetof(i2cMsg{}.buf)]byte
	_ [unsafe.Offsetof(i2cMsg{}.len) - 4]byte
	_ [4 - unsafe.Offsetof(i2cMsg{}.len)]byte
	_ [unsafe.Sizeof(i2cRdwrData{}) - 2*ptrSize]byte
	_ [2*ptrSize - unsafe.Sizeof(i2cRdwrData{})]byte
	_ [unsafe.Offsetof(i2cRdwrData{}.nmsgs) - ptrSize]byte
	_ [ptrSize - unsafe.Offsetof(i2cRdwrData{}.nmsgs)]byte
)

// TestI2cMsgLayout states the sizes the running GOARCH must have.
func TestI2cMsgLayout(t *testing.T) {
	want := map[uintptr][2]uintptr{4: {12, 8}, 8: {16, 16}}[ptrSize]
	if got := [2]uintptr{unsafe.Sizeof(i2cMsg{}), unsafe.Sizeof(i2cRdwrData{})}; got != want {
		t.Errorf("sizeof(i2c_msg), sizeof(i2c_rdwr_ioctl_data) = %v, want %v", got, want)
	}
}

// fakeBus registers a bus whose fd (a regular file) the injected rdwr ignores.
func fakeBus(t *testing.T, fake func(fd int, d *i2cRdwrData) (int, error)) Value {
	t.Helper()
	ResetForTest()
	old := rdwr
	rdwr = fake
	t.Cleanup(func() { rdwr = old })
	f, err := os.Create(filepath.Join(t.TempDir(), "bus"))
	if err != nil {
		t.Fatal(err)
	}
	return addBus(f)
}

func errKind(err error) (string, string) {
	sig, ok := err.(*interpreter.ErrorSignal)
	if !ok {
		return "runtime", err.Error()
	}
	var kind, msg string
	for _, f := range sig.Value.Fields {
		switch f.Name {
		case "kind":
			kind = f.Value.Str
		case "message":
			msg = f.Value.Str
		}
	}
	return kind, msg
}

// TestWriteReadOneCombinedTransfer: writeRead sends a write message and a read
// message to the bus address in one I2C_RDWR (repeated START between them).
func TestWriteReadOneCombinedTransfer(t *testing.T) {
	calls := 0
	b := fakeBus(t, func(_ int, d *i2cRdwrData) (int, error) {
		calls++
		msgs := unsafe.Slice(d.msgs, d.nmsgs)
		if d.nmsgs != 2 {
			t.Fatalf("nmsgs = %d", d.nmsgs)
		}
		w, r := msgs[0], msgs[1]
		if w.addr != 0x76 || w.flags != 0 || string(unsafe.Slice(w.buf, w.len)) != "\xf7\x01" {
			t.Errorf("write message %+v", w)
		}
		if r.addr != 0x76 || r.flags != i2cMRd || r.len != 3 {
			t.Errorf("read message %+v", r)
		}
		copy(unsafe.Slice(r.buf, r.len), "abc")
		return 2, nil
	})
	v, err := writeReadFn(interpreter.BuiltinCtx{}, []Value{b, interpreter.BytesVal([]byte{0xf7, 0x01}), interpreter.IntVal(3)})
	if err != nil {
		t.Fatal(err)
	}
	if string(v.Bytes) != "abc" || calls != 1 {
		t.Errorf("got %q after %d calls", v.Bytes, calls)
	}
}

func TestWriteReadErrors(t *testing.T) {
	cases := []struct {
		name   string
		errno  error
		data   []byte
		n      int64
		kind   string
		substr string
	}{
		{"nack", unix.ENXIO, []byte{1}, 1, "runtime", "i2c.writeRead: address 0x76: no such device or address (no ACK"},
		{"nack remote", unix.EREMOTEIO, []byte{1}, 1, "runtime", "(no ACK: nothing at this address"},
		{"timeout", unix.ETIMEDOUT, []byte{1}, 1, "timeout", "i2c.writeRead: address 0x76: bus timed out"},
		{"no rdwr", unix.EOPNOTSUPP, []byte{1}, 1, "runtime", "does not support this transfer"},
		{"read too long", nil, []byte{1}, 8193, "limit", "at most 8192 bytes"},
		{"write too long", nil, make([]byte, 8193), 1, "limit", "at most 8192 bytes"},
		{"empty write", nil, nil, 1, "runtime", "data must not be empty"},
		{"zero read", nil, []byte{1}, 0, "runtime", "n must be at least 1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			called := false
			b := fakeBus(t, func(int, *i2cRdwrData) (int, error) {
				called = true
				return 0, c.errno
			})
			_, err := writeReadFn(interpreter.BuiltinCtx{}, []Value{b, interpreter.BytesVal(c.data), interpreter.IntVal(c.n)})
			if err == nil {
				t.Fatal("no error")
			}
			kind, msg := errKind(err)
			if kind != c.kind || !strings.Contains(msg, c.substr) {
				t.Errorf("got %s %q", kind, msg)
			}
			if c.errno == nil && called {
				t.Error("an invalid request reached the adapter")
			}
		})
	}
}

// TestOpenErrorHints: open names the path, and a busy address explains that a
// kernel driver owns it (I2C_SLAVE_FORCE is not used).
func TestOpenErrorHints(t *testing.T) {
	ResetForTest()
	_, err := openFn(interpreter.BuiltinCtx{}, []Value{interpreter.StringVal("/dev/null"), interpreter.IntVal(0x76)})
	if err == nil || !strings.Contains(err.Error(), "i2c.open: /dev/null: selecting address 0x76: inappropriate ioctl for device (not an i2c-dev device)") {
		t.Errorf("got %v", err)
	}
	if got := explain(unix.EBUSY); !strings.Contains(got, "a kernel driver has bound this address") {
		t.Errorf("EBUSY: %q", got)
	}
}
