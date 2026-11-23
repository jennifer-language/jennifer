// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package dataerr

import (
	"errors"
	"fmt"
	"testing"

	"jennifer-lang.dev/jennifer/internal/limits"
)

func TestKinds(t *testing.T) {
	e := New("json", "bad %s", "token")
	if e.Error() != "bad token" || KindOf(e) != "json" {
		t.Errorf("New: %q kind %q", e, KindOf(e))
	}
	if KindOf(fmt.Errorf("json.decode: %w", e)) != "json" {
		t.Error("a %w wrapper lost the kind")
	}
	if w := Wrap("gzip", errors.New("corrupt")); w.Error() != "corrupt" || KindOf(w) != "gzip" {
		t.Errorf("Wrap: %q kind %q", w, KindOf(w))
	}
	// The generic kinds and an inner library's kind keep priority.
	if KindOf(Wrap("json", limits.Capf("too deep"))) != "limit" {
		t.Error("Wrap reclassified a cap")
	}
	if KindOf(Wrap("archive", fmt.Errorf("x: %w", New("compress", "bad")))) != "compress" {
		t.Error("Wrap reclassified an inner data error")
	}
	if Wrap("json", nil) != nil || KindOf(errors.New("x")) != "" {
		t.Error("nil / plain")
	}
}
