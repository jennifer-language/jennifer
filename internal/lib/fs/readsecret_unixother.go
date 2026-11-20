// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build unix && !linux

package fslib

import "os"

// mountReadOnly is not asked outside Linux, where container runtimes mount
// secrets; every file gets the strict owner-only check.
func mountReadOnly(_ *os.File) bool { return false }
