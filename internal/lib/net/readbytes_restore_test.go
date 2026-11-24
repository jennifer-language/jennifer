// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package netlib

import (
	"bufio"
	stdnet "net"
	"sync/atomic"
	"testing"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// lockCheckConn records every SetReadDeadline made while the conn's readMu
// is free: such a call can overwrite the deadline another reader armed.
type lockCheckConn struct {
	stdnet.Conn
	s        *connState
	unlocked atomic.Int32
}

func (c *lockCheckConn) SetReadDeadline(t time.Time) error {
	if c.s.readMu.TryLock() {
		c.unlocked.Add(1)
		c.s.readMu.Unlock()
	}
	return c.Conn.SetReadDeadline(t)
}

// TestReadBytesRestoresIdleDeadlineUnderLock: readBytes with an idle timeout
// restores the setReadDeadline deadline before it releases readMu, so a
// second task's read cannot have its idle (or cancel) deadline overwritten.
func TestReadBytesRestoresIdleDeadlineUnderLock(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	a, b := stdnet.Pipe()
	defer a.Close()
	defer b.Close()
	s := &connState{idle: time.Second}
	lc := &lockCheckConn{Conn: a, s: s}
	s.c, s.r = lc, bufio.NewReader(lc)
	connsMu.Lock()
	conns[1] = s
	connsMu.Unlock()
	go func() { _, _ = b.Write([]byte("x")) }()
	v, err := readBytesFn(interpreter.BuiltinCtx{}, []Value{makeConn(1), interpreter.IntVal(4)})
	if err != nil || string(v.Bytes()) != "x" {
		t.Fatalf("readBytes: %v, %v", v, err)
	}
	if n := lc.unlocked.Load(); n != 0 {
		t.Errorf("%d read deadline change(s) made after readMu was released", n)
	}
}
