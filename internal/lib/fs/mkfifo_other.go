// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !unix

package fslib

import "errors"

func mkfifoAt(string, uint32) error {
	return errors.New("named pipes are not supported on this platform")
}
