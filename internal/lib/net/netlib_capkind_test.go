// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package netlib

import (
	"errors"
	stdnet "net"
	"strconv"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// fillRegistries makes every handle registry look full (placeholder entries,
// no sockets), so the next connect / accept / listen / listenUDP reaches its
// cap without opening 4096 descriptors.
func fillRegistries(t *testing.T) {
	t.Helper()
	ResetForTest()
	t.Cleanup(ResetForTest)
	connsMu.Lock()
	listenersMu.Lock()
	udpsMu.Lock()
	for i := int64(1); i <= maxNetHandles; i++ {
		conns[-i] = nil
		listeners[-i] = nil
		udps[-i] = nil
	}
	udpsMu.Unlock()
	listenersMu.Unlock()
	connsMu.Unlock()
}

func wantCapError(t *testing.T, what string, err error) {
	t.Helper()
	var ce *limits.CapError
	if !errors.As(err, &ce) {
		t.Errorf("%s at the handle cap: %T %v, want a limits.CapError (kind \"limit\")", what, err, err)
	}
}

// TestHandleCapsAreKindLimit: the connection, listener and UDP-socket caps are
// kind "limit" (M29.1), like the fs / hash / compress handle caps.
func TestHandleCapsAreKindLimit(t *testing.T) {
	ln, err := stdnet.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	addr := interpreter.StringVal(ln.Addr().String())

	fillRegistries(t)
	_, err = connectFn(ctx(), []Value{addr})
	wantCapError(t, "net.connect", err)

	_, err = listenFn(ctx(), []Value{interpreter.StringVal("127.0.0.1:0")})
	wantCapError(t, "net.listen", err)

	_, err = listenUDPFn(ctx(), []Value{interpreter.StringVal("127.0.0.1:0")})
	wantCapError(t, "net.listenUDP", err)
}

// TestAcceptCapIsKindLimit: an accept past the connection cap is kind "limit".
func TestAcceptCapIsKindLimit(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	l, err := listenFn(ctx(), []Value{interpreter.StringVal("127.0.0.1:0")})
	if err != nil {
		t.Fatal(err)
	}
	a, err := addressFn(ctx(), []Value{l})
	if err != nil {
		t.Fatal(err)
	}
	c, err := stdnet.Dial("tcp", a.Str())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	connsMu.Lock()
	for i := int64(1); i <= maxNetHandles; i++ {
		conns[-i] = nil
	}
	connsMu.Unlock()
	_, err = acceptFn(ctx(), []Value{l})
	wantCapError(t, "net.accept", err)
}

// TestConnectTLSCapIsKindLimit: connectTLS shares the connection cap.
func TestConnectTLSCapIsKindLimit(t *testing.T) {
	cert, pool, _ := genCert(t)
	port := tlsEchoServer(t, cert)
	trust(t, pool)
	fillRegistries(t)
	_, err := connectTLSFn(ctx(), []Value{interpreter.StringVal("localhost:" + strconv.Itoa(port))})
	wantCapError(t, "net.connectTLS", err)
}
