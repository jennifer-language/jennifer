// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build (!tinygo || tinygo_devtools) && !unix

package main

import "jennifer-lang.dev/jennifer/internal/interpreter"

// installProfileSignals is a no-op where SIGUSR1 does not exist (Windows): a
// profiled program there writes its report when it ends. Returns a no-op stop
// function.
func installProfileSignals(_ *interpreter.Interpreter, _ *profileReport, _ <-chan struct{}) func() {
	return func() {}
}
