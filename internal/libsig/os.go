// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("os", `
getEnv(string) string
setEnv(string, string) null
hasFlag(string) bool
flag(string) string
run(list of string, string|bytes?) os.Result
spawn(list of string, map of any to any?) os.Process
wait(os.Process) os.Result
poll(os.Process) bool
kill(os.Process) null
release(os.Process) bool
isTerminal(string) bool
cwd() string
homeDir() string
tempDir() string
catchSignal(string) null
gotSignal(string) bool
`)
}
