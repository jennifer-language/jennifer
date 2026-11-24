// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build tinygo

package main

// setMemoryLimit does nothing on TinyGo: its GC has no soft limit, and the
// build has its own memory guard.
func setMemoryLimit() {}
