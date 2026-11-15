// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package main

// canSpawnSelf reports whether this build can start a copy of itself as a
// subprocess (`jennifer test --isolated`). TinyGo has no os/exec.
const canSpawnSelf = true
