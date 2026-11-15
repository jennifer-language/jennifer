// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package interpreter

// stackTooDeep is jennifer-tiny's stack-bytes guard (stackguard_tiny.go). Go's
// goroutine stacks grow, so the default binary relies on MaxCallDepth alone.
func stackTooDeep(*CallChain) bool { return false }
