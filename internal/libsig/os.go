// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	addDataKind("os", "os")
	add("os", `
getEnv(string) string
getEnvBytes(string) bytes
argsBytes() list of bytes
setEnv(string, string) null
unsetEnv(string) null
hasEnv(string) bool
hasFlag(string) bool
flag(string) string
run(list of string, string|bytes?) os.Result
runBytes(list of string, string|bytes?) os.BytesResult
# The options map of spawn is a deprecated shape (lint L301, removed in
# 0.29.0); the signature keeps accepting it until then.
spawn(list of string, map of any to any?) os.Process
spawnWith(list of string, os.SpawnOptions) os.Process
wait(os.Process) os.Result
waitBytes(os.Process) os.BytesResult
waitTimeout(os.Process, int) os.Result
waitBytesTimeout(os.Process, int) os.BytesResult
poll(os.Process) bool
kill(os.Process) null
signal(os.Process, string) null
release(os.Process) bool
isTerminal(string) bool
cwd() string
homeDir() string
tempDir() string
catchSignal(string) null
gotSignal(string) bool
`)
	deprecate("os", "hasFlag", "the `args` module: `args.boolFlag`, then `args.asBool`", "0.29.0")
	deprecate("os", "flag", "the `args` module: `args.flag`, then `args.asString`", "0.29.0")
}
