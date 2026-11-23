// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package netlib

import (
	"context"
	stdnet "net"
	"testing"
	"time"
)

// SetDefaultTimeoutForTest lowers the default connect / handshake / lookup
// deadline for one test, so the default is tested without waiting 30 s.
func SetDefaultTimeoutForTest(t *testing.T, d time.Duration) {
	old := defaultTimeout
	defaultTimeout = d
	t.Cleanup(func() { defaultTimeout = old })
}

// SetStalledResolverForTest makes every DNS query of the lookups wait until
// its deadline, so a lookup timeout is tested without a slow name server.
func SetStalledResolverForTest(t *testing.T) {
	old := resolver
	resolver = &stdnet.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (stdnet.Conn, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	t.Cleanup(func() { resolver = old })
}
