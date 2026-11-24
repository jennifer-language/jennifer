// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package tomllib

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/dataerr"
)

// M29.40: TOML 1.0 requires an error for an integer it cannot hold
// losslessly; it is a data error naming the fix.
func TestBigIntegerRefused(t *testing.T) {
	_, err := decodeToml("id = 12345678901234567891\n")
	if dataerr.KindOf(err) != "toml" || !strings.Contains(err.Error(), `integer "12345678901234567891" is out of range for a 64-bit signed TOML integer; quote it as a string`) {
		t.Errorf("%v (kind %q)", err, dataerr.KindOf(err))
	}
	if _, err := decodeToml("id = 9223372036854775807\n"); err != nil {
		t.Errorf("int64 max: %v", err)
	}
}

// A huge integer's error message shows a bounded prefix and the digit count,
// not every digit.
func TestBigIntegerMessageBounded(t *testing.T) {
	_, err := decodeToml("id = " + strings.Repeat("9", 100_000) + "\n")
	if dataerr.KindOf(err) != "toml" || len(err.Error()) > 300 || !strings.Contains(err.Error(), "(100000 digits)") {
		t.Errorf("%.400v", err)
	}
}
