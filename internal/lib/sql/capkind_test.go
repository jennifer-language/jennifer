// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package sqllib

import (
	"errors"
	"testing"

	"jennifer-lang.dev/jennifer/internal/limits"
)

// TestHandleCapsAreKindLimit: a query, queryStmt, begin or prepare past its
// registry's cap is kind "limit" (M29.1). The registries are filled with
// placeholder entries. (sql.open's cap needs a live server to ping first.)
func TestHandleCapsAreKindLimit(t *testing.T) {
	conn := mockConnValue(t)
	t.Cleanup(ResetForTest)
	stmt, err := prepareFn(ctx(), []Value{conn, sv("SELECT * FROM t")})
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	mu.Lock()
	for i := int64(1); i <= maxHandles; i++ {
		rows[-i] = nil
		txs[-i] = nil
		stmts[-i] = nil
	}
	mu.Unlock()

	var ce *limits.CapError
	if _, err := queryFn(ctx(), []Value{conn, sv("SELECT * FROM t")}); !errors.As(err, &ce) {
		t.Errorf("sql.query at the cap: %T %v, want a limits.CapError", err, err)
	}
	if _, err := queryStmtFn(ctx(), []Value{stmt}); !errors.As(err, &ce) {
		t.Errorf("sql.queryStmt at the cap: %T %v, want a limits.CapError", err, err)
	}
	if _, err := beginFn(ctx(), []Value{conn}); !errors.As(err, &ce) {
		t.Errorf("sql.begin at the cap: %T %v, want a limits.CapError", err, err)
	}
	if _, err := prepareFn(ctx(), []Value{conn, sv("SELECT 1")}); !errors.As(err, &ce) {
		t.Errorf("sql.prepare at the cap: %T %v, want a limits.CapError", err, err)
	}
}
