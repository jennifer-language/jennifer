// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package devio

import "testing"

func TestReadSize(t *testing.T) {
	if _, err := ReadSize("x.read", -1); err == nil {
		t.Error("negative length should error")
	}
	if _, err := ReadSize("x.read", MaxRead+1); err == nil {
		t.Error("over-cap length should error")
	}
	if sz, err := ReadSize("x.read", 64); err != nil || sz != 64 {
		t.Errorf("valid length: sz=%d err=%v", sz, err)
	}
	if sz, err := ReadSize("x.read", MaxRead); err != nil || sz != MaxRead {
		t.Errorf("boundary length: sz=%d err=%v", sz, err)
	}
}
