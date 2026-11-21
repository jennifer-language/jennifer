// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package httpdlib

import (
	"errors"
	"testing"

	"jennifer-lang.dev/jennifer/internal/limits"
)

// TestServerCapIsKindLimit: opening a server past the open-server cap is kind
// "limit" (M29.1), for plaintext and TLS alike. The registry is filled with
// placeholder entries, so no 4096 sockets are opened.
func TestServerCapIsKindLimit(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	serversMu.Lock()
	for i := int64(1); i <= maxServers; i++ {
		servers[-i] = &serverState{closing: make(chan struct{}), killed: make(chan struct{})}
	}
	serversMu.Unlock()
	var ce *limits.CapError
	if _, err := openServer("httpd.listen", "127.0.0.1:0", serverLimits{}); !errors.As(err, &ce) {
		t.Errorf("httpd.listen at the cap: %T %v, want a limits.CapError", err, err)
	}
	if _, err := openServerTLS("httpd.listenTLS", "127.0.0.1:0", nil, nil, serverLimits{}); !errors.As(err, &ce) {
		t.Errorf("httpd.listenTLS at the cap: %T %v, want a limits.CapError", err, err)
	}
}
