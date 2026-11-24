// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build linux && !tinygo

package spilib

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lib/devio"
)

// TestOpenWith (M29.49): spi.openWith applies mode, speed and word size at
// open (zero fields take the defaults), and transfer sends the word size. A
// regular file stands in for the device and a recorder for the ioctls.
func TestOpenWith(t *testing.T) {
	ResetForTest()
	var modes, bitss []uint8
	var speeds []uint32
	var xferBits []uint8
	old := ioctl
	ioctl = func(_ int, req uintptr, p unsafe.Pointer) error {
		switch req {
		case spiWrMode:
			modes = append(modes, *(*uint8)(p))
		case spiWrBits:
			bitss = append(bitss, *(*uint8)(p))
		case spiWrMaxSpeedHz:
			speeds = append(speeds, *(*uint32)(p))
		case spiMessage1():
			xferBits = append(xferBits, (*spiIocTransfer)(p).bitsPerWord)
		}
		return nil
	}
	t.Cleanup(func() { ioctl = old })
	path := filepath.Join(t.TempDir(), "spidev")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := interpreter.BuiltinCtx{}
	opts := func(fields ...interpreter.StructField) Value {
		return interpreter.NamespacedStructVal(LibraryName, "Options", fields)
	}
	iv := interpreter.IntVal
	dflt, err := openWithFn(ctx, []Value{interpreter.StringVal(path), opts()})
	if err != nil {
		t.Fatal(err)
	}
	d16, err := openWithFn(ctx, []Value{interpreter.StringVal(path), opts(
		interpreter.StructField{Name: "mode", Value: iv(3)},
		interpreter.StructField{Name: "speedHz", Value: iv(8000000)},
		interpreter.StructField{Name: "bitsPerWord", Value: iv(16)})})
	if err != nil {
		t.Fatal(err)
	}
	if string(modes) != "\x00\x03" || string(bitss) != "\x08\x10" || len(speeds) != 2 || speeds[0] != 500000 || speeds[1] != 8000000 {
		t.Fatalf("configured modes %v bits %v speeds %v", modes, bitss, speeds)
	}
	if _, err := transferFn(ctx, []Value{d16, interpreter.BytesVal([]byte{1, 2})}); err != nil {
		t.Fatal(err)
	}
	if _, err := transferFn(ctx, []Value{dflt, interpreter.BytesVal([]byte{1})}); err != nil {
		t.Fatal(err)
	}
	if string(xferBits) != "\x10\x08" {
		t.Errorf("transfer bits per word %v, want [16 8]", xferBits)
	}
	if _, err := transferFn(ctx, []Value{d16, interpreter.BytesVal([]byte{1, 2, 3})}); err == nil || !strings.Contains(err.Error(), "whole number of words") {
		t.Errorf("3 bytes at 16 bits per word: %v", err)
	}
	for _, c := range []struct {
		field string
		v     int64
		want  string
	}{
		{"mode", 4, "mode must be 0..3"},
		{"speedHz", -1, "speedHz must be in"},
		{"bitsPerWord", 33, "bitsPerWord must be 1..32"},
	} {
		_, err := openWithFn(ctx, []Value{interpreter.StringVal(path), opts(interpreter.StructField{Name: c.field, Value: iv(c.v)})})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s %d: %v", c.field, c.v, err)
		}
	}
}

// TestClosedDevice (M29.46): any call on a closed device is kind "closed"; a
// second close does nothing.
func TestClosedDevice(t *testing.T) {
	ResetForTest()
	f, err := os.Create(filepath.Join(t.TempDir(), "spidev"))
	if err != nil {
		t.Fatal(err)
	}
	devsMu.Lock()
	nextID++
	id := nextID
	devs[id] = &devState{f: f, speedHz: 500000, bufsiz: 4096}
	devsMu.Unlock()
	h := devio.Handle(LibraryName, "Device", id)
	ctx := interpreter.BuiltinCtx{}
	for i := 0; i < 2; i++ {
		if _, err := closeFn(ctx, []Value{h}); err != nil {
			t.Fatalf("close %d: %v", i+1, err)
		}
	}
	_, err = transferFn(ctx, []Value{h, interpreter.BytesVal([]byte{1})})
	if kind, _, _, _, _ := interpreter.ClassifyError(err); kind != "closed" {
		t.Errorf("transfer after close: kind %q (%v)", kind, err)
	}
	_, err = configureFn(ctx, []Value{h, interpreter.IntVal(0), interpreter.IntVal(1000)})
	if kind, _, _, _, _ := interpreter.ClassifyError(err); kind != "closed" {
		t.Errorf("configure after close: kind %q (%v)", kind, err)
	}
}
