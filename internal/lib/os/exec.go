// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package oslib

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	stdos "os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// procWaitDelay bounds how long cmd.Wait waits for a child's stdout / stderr
// pipes to close after the child process itself has exited. Without it, Wait
// blocks until every descendant that inherited the pipes exits - so os.wait
// after os.kill, and os.run of a command that backgrounds a process, hang for as
// long as a grandchild lives (forever for a daemon). The grace is generous
// enough never to truncate a normal command's own output (which flushes at
// exit), and short enough to bound the pathological case; when it fires the
// process has already reported its exit code, so the result stays valid.
const procWaitDelay = 500 * time.Millisecond

// execSupported reports whether the host's Go runtime actually
// supports `os/exec`. TinyGo (today) compiles `os/exec` but the
// underlying syscall layer panics or errors with cryptic messages
// like "files setting not implemented". The pre-check turns that
// into a friendly Jennifer-level error pointing at the build choice.
func execSupported() bool {
	return runtime.Compiler != "tinygo"
}

func execUnsupportedErr(fnName string) error {
	return fmt.Errorf("%s: external-program execution is not supported by `jennifer-tiny` (TinyGo build; compiler=%s); use the default `jennifer` binary (`make build` produces both, or `make build-go` for just the Go binary)", fnName, runtime.Compiler)
}

// processState holds the Go-side state for an `os.spawn`'d child.
// The Jennifer-visible `os.Process` value only carries the pid; the
// rest of the bookkeeping lives in this struct and is reached
// through the handles map keyed by pid.
// maxCaptureBytes caps the output captured per stream from a child process, so
// a verbose child can't grow the interpreter heap by its entire (possibly
// unbounded) output. Past the cap, further bytes are discarded and a marker is
// appended - the child is never blocked (Write always reports a full write).
const maxCaptureBytes = 16 << 20 // 16 MiB per stream

// capBuffer is a bytes.Buffer that stops storing past maxCaptureBytes and flags
// the truncation, so String() can append a marker.
type capBuffer struct {
	buf       bytes.Buffer
	truncated bool
}

func (c *capBuffer) Write(p []byte) (int, error) {
	room := maxCaptureBytes - c.buf.Len()
	if room <= 0 {
		c.truncated = true
		return len(p), nil // discard but don't block the child
	}
	if len(p) > room {
		c.buf.Write(p[:room])
		c.truncated = true
		return len(p), nil
	}
	return c.buf.Write(p)
}

func (c *capBuffer) String() string { return capturedText(c.buf.String(), c.truncated) }

// capturedText is the text form of a captured stream: when the cap truncated
// it, a rune the cap split is dropped (so valid UTF-8 output stays valid) and
// a marker is appended. The bytes form (os.runBytes / waitBytes) is raw as-is.
func capturedText(raw string, truncated bool) string {
	if !truncated {
		return raw
	}
	start := len(raw) - 1
	for start > 0 && start > len(raw)-utf8.UTFMax && !utf8.RuneStart(raw[start]) {
		start--
	}
	if start >= 0 && !utf8.FullRuneInString(raw[start:]) {
		raw = raw[:start]
	}
	return raw + "\n[output truncated at 16 MiB]"
}

type processState struct {
	cmd *exec.Cmd
	// stdout / stderr hold the capture buffer for a stream only when that stream
	// is captured (the default); they are nil for an inherited / discarded /
	// file-redirected stream, in which case os.wait's corresponding field is "".
	stdout *capBuffer
	stderr *capBuffer
	// outFile / errFile are the open files for a `path`-redirected stream; the
	// reaper closes them when the child exits.
	outFile  *stdos.File
	errFile  *stdos.File
	done     chan struct{} // closed when cmd.Wait() returns
	exitCode int64
	waitErr  error
	// outRaw / errRaw hold the captured bytes once the process is reaped, and
	// outTrunc / errTrunc whether the cap cut them; the reaper drains the
	// buffers into these and drops the *bytes.Buffer so a terminated handle no
	// longer pins a live growable buffer for the program's life. os.wait (text)
	// and os.waitBytes read these (idempotent, unchanged result).
	outRaw   string
	errRaw   string
	outTrunc bool
	errTrunc bool
}

var (
	handlesMu sync.Mutex
	// handles is keyed by a monotonic internal id, NOT the OS pid. Keying by
	// pid was a correctness bug: once the reaper Wait()s a child the OS can
	// recycle its pid, so a later os.spawn would overwrite the entry and
	// os.wait / poll / kill on the old handle would hit the wrong process. A
	// never-reused id makes each os.Process refer to exactly its own child for
	// the handle's whole life. (Matches net / fs / httpd, which also hand out
	// opaque integer handles into a registry.)
	handles = map[int64]*processState{}
	nextID  int64
)

// maxProcHandles bounds the live child-process registry so a script that leaks
// os.Process handles (spawns without os.wait / os.release) surfaces a catchable
// error instead of forking children without limit.
const maxProcHandles = 4096

// argvFromList unwraps a Jennifer `list of string` into a Go []string.
// Empty list and non-list values produce a typed error tagged with
// the caller's function name (`os.run` / `os.spawn`).
func argvFromList(fnName string, v interpreter.Value) ([]string, error) {
	if v.Kind != interpreter.KindList {
		return nil, fmt.Errorf("%s: argv must be a list of string, got %s", fnName, v.Kind)
	}
	if len(v.List) == 0 {
		return nil, fmt.Errorf("%s: argv must contain at least one element (the program name)", fnName)
	}
	out := make([]string, len(v.List))
	for i, elem := range v.List {
		if elem.Kind != interpreter.KindString {
			return nil, fmt.Errorf("%s: argv[%d] must be string, got %s", fnName, i, elem.Kind)
		}
		out[i] = elem.Str
	}
	return out, nil
}

// makeResult constructs an `os.Result{...}` Value from the captured
// streams and exit code. Used by both `os.run` (synchronous path) and
// `os.wait` (waiting on a `spawn`'d handle). Captured output that is not valid
// UTF-8 is an error naming fnName, the stream and the byte offset; binary
// output is read with os.runBytes / waitBytes.
func makeResult(fnName string, exitCode int64, stdout, stderr string) (interpreter.Value, error) {
	if err := limits.CheckUTF8(fnName+": stdout", stdout); err != nil {
		return interpreter.Null(), err
	}
	if err := limits.CheckUTF8(fnName+": stderr", stderr); err != nil {
		return interpreter.Null(), err
	}
	return interpreter.NamespacedStructVal("os", "Result", []interpreter.StructField{
		{Name: "exitCode", Value: interpreter.IntVal(exitCode)},
		{Name: "stdout", Value: interpreter.StringVal(stdout)},
		{Name: "stderr", Value: interpreter.StringVal(stderr)},
	}), nil
}

// makeBytesResult constructs an `os.BytesResult{exitCode, stdout, stderr,
// truncated}` Value for os.runBytes / waitBytes: the captured bytes as
// written, no UTF-8 check and no truncation marker; truncated reports that the
// 16 MiB cap cut either stream.
func makeBytesResult(exitCode int64, stdout, stderr string, truncated bool) interpreter.Value {
	return interpreter.NamespacedStructVal("os", "BytesResult", []interpreter.StructField{
		{Name: "exitCode", Value: interpreter.IntVal(exitCode)},
		{Name: "stdout", Value: interpreter.BytesVal([]byte(stdout))},
		{Name: "stderr", Value: interpreter.BytesVal([]byte(stderr))},
		{Name: "truncated", Value: interpreter.BoolVal(truncated)},
	})
}

// captureResult builds the result of fnName from captured streams: an
// os.BytesResult when asBytes, else an os.Result (UTF-8 checked, marked when
// truncated).
func captureResult(fnName string, asBytes bool, exitCode int64, out, errOut string, outTrunc, errTrunc bool) (interpreter.Value, error) {
	if asBytes {
		return makeBytesResult(exitCode, out, errOut, outTrunc || errTrunc), nil
	}
	return makeResult(fnName, exitCode, capturedText(out, outTrunc), capturedText(errOut, errTrunc))
}

// makeProcess constructs an `os.Process{handle, pid}` Value. `handle` is the
// internal registry id (a monotonic, never-reused key for os.wait / poll / kill /
// release); `pid` is the real OS process id (for logging, `ps`, `/proc`, or an
// out-of-band signal). They are distinct on purpose: the OS recycles a pid once
// the child is reaped, so the registry must key on the stable handle, not the pid.
func makeProcess(handle, pid int64) interpreter.Value {
	return interpreter.NamespacedStructVal("os", "Process", []interpreter.StructField{
		{Name: "handle", Value: interpreter.IntVal(handle)},
		{Name: "pid", Value: interpreter.IntVal(pid)},
	})
}

// extractHandle pulls the registry handle out of an `os.Process` value, which
// os.wait / poll / kill / release use to find the child. A non-Process value is a
// typed runtime error; a missing field is an internal error (the struct was built
// outside this library). The OS pid field is informational only and is not used
// here (the handle, not the recyclable pid, identifies the child).
func extractHandle(fnName string, v interpreter.Value) (int64, error) {
	if v.Kind != interpreter.KindStruct || v.StructNS != "os" || v.StructName != "Process" {
		return 0, fmt.Errorf("%s: argument must be an os.Process, got %s", fnName, v.Kind)
	}
	for _, f := range v.Fields {
		if f.Name == "handle" {
			if f.Value.Kind != interpreter.KindInt {
				return 0, fmt.Errorf("%s: os.Process.handle is not int (got %s)", fnName, f.Value.Kind)
			}
			return f.Value.Int, nil
		}
	}
	return 0, fmt.Errorf("%s: os.Process has no handle field", fnName)
}

// stdinBytes extracts the optional `stdin` argument to os.run: a string or
// bytes value, fed to the child's standard input (which is then closed, so the
// child sees EOF). Any other kind is a typed runtime error.
func stdinBytes(fnName string, v interpreter.Value) ([]byte, error) {
	switch v.Kind {
	case interpreter.KindString:
		return []byte(v.Str), nil
	case interpreter.KindBytes:
		return v.Bytes, nil
	default:
		return nil, fmt.Errorf("%s: stdin must be a string or bytes, got %s", fnName, v.Kind)
	}
}

// runFn implements `os.run(argv[, stdin]) -> os.Result`. Blocking: runs the
// command to completion and returns the captured streams and exit code as a
// single result struct. An optional second argument (a string or bytes) is
// written to the child's stdin, then closed. A non-zero exit code is NOT an
// error - the caller branches on `$result.exitCode`. Boundary failures (program
// not found, not executable, fork/exec failure) are typed runtime errors.
//
// Deadlock-free by construction: the whole stdin is buffered up front (a
// bytes.Reader) and the output is drained into the capped buffers as the child
// runs, so neither side blocks on a full pipe. Output stays capped at 16 MiB per
// stream (capBuffer).
func runFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return runCommand("os.run", args, false)
}

// runBytesFn implements `os.runBytes(argv[, stdin]) -> os.BytesResult`: os.run
// with stdout and stderr returned as bytes (binary output, no UTF-8 check).
func runBytesFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return runCommand("os.runBytes", args, true)
}

// runCommand is os.run (asBytes false) and os.runBytes (asBytes true).
func runCommand(fnName string, args []interpreter.Value, asBytes bool) (interpreter.Value, error) {
	if !execSupported() {
		return interpreter.Null(), execUnsupportedErr(fnName)
	}
	if len(args) < 1 || len(args) > 2 {
		return interpreter.Null(), fmt.Errorf(
			"%s expects 1 argument (argv list) or 2 (argv, stdin), got %d", fnName, len(args))
	}
	argv, err := argvFromList(fnName, args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	if len(args) == 2 {
		in, serr := stdinBytes(fnName, args[1])
		if serr != nil {
			return interpreter.Null(), serr
		}
		cmd.Stdin = bytes.NewReader(in)
	}
	var outBuf, errBuf capBuffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	cmd.WaitDelay = procWaitDelay
	result := func(code int) (interpreter.Value, error) {
		return captureResult(fnName, asBytes, int64(code), outBuf.buf.String(), errBuf.buf.String(), outBuf.truncated, errBuf.truncated)
	}
	if err := cmd.Run(); err != nil {
		// ExitError means the process ran and reported non-zero exit;
		// that's a result, not a boundary error.
		if exitErr, ok := err.(*exec.ExitError); ok {
			return result(exitErr.ExitCode())
		}
		// WaitDelay fired: the process exited and reported a code, but a
		// backgrounded descendant kept the pipes open past the grace. The
		// process's own result is valid - return it rather than a boundary error.
		if cmd.ProcessState != nil && errors.Is(err, exec.ErrWaitDelay) {
			return result(cmd.ProcessState.ExitCode())
		}
		return interpreter.Null(), fmt.Errorf("%s: %v", fnName, err)
	}
	return result(cmd.ProcessState.ExitCode())
}

// spawnOptions reads os.spawn's options map: only the keys "stdout" and
// "stderr", each with a string value. A missing key is "" (capture).
func spawnOptions(fn string, opts interpreter.Value) (stdout, stderr string, err error) {
	if opts.Kind != interpreter.KindMap {
		return "", "", nil
	}
	for _, e := range opts.Map {
		if e.Key.Kind != interpreter.KindString {
			return "", "", fmt.Errorf("%s: option keys must be strings, got %s", fn, e.Key.Kind)
		}
		if e.Key.Str != "stdout" && e.Key.Str != "stderr" {
			return "", "", fmt.Errorf("%s: unknown option %q; known: stdout, stderr", fn, e.Key.Str)
		}
		if e.Value.Kind != interpreter.KindString {
			return "", "", fmt.Errorf("%s: option %q must be a string, got %s", fn, e.Key.Str, e.Value.Kind)
		}
		if err := checkSpawnMode(fn, e.Key.Str, e.Value.Str); err != nil {
			return "", "", err
		}
		if e.Key.Str == "stdout" {
			stdout = e.Value.Str
		} else {
			stderr = e.Value.Str
		}
	}
	return stdout, stderr, nil
}

// checkSpawnMode accepts the stream keywords and a file path (isSpawnPath).
func checkSpawnMode(fn, which, mode string) error {
	switch mode {
	case "", "capture", "inherit", "discard":
		return nil
	}
	if !isSpawnPath(mode) {
		return fmt.Errorf("%s: unknown %s mode %q; known: capture, inherit, discard, or a file path containing / (./%s)", fn, which, mode, mode)
	}
	return nil
}

// isSpawnPath reports whether a stream mode names a file: it must contain a
// path separator ("./out.log", "/tmp/out"), so a mistyped keyword such as
// "inhert" is an error instead of a file in the working directory.
func isSpawnPath(mode string) bool {
	return strings.ContainsRune(mode, '/') || strings.ContainsRune(mode, filepath.Separator)
}

// resolveSpawnStream wires one child stream per its mode: "" / "capture" -> an
// in-memory capture buffer (returned later by os.wait), "inherit" -> the parent's
// own stream (live on the terminal / the parent's redirect), "discard" ->
// dropped, a file path (checked by checkSpawnMode) -> that file (created,
// truncated). Returns the writer to hand exec.Cmd
// plus the capture buffer or the open file to record on the state (at most one
// of the two is non-nil).
func resolveSpawnStream(fn, which, mode string, inherit *stdos.File) (io.Writer, *capBuffer, *stdos.File, error) {
	switch mode {
	case "", "capture":
		b := &capBuffer{}
		return b, b, nil, nil
	case "inherit":
		return inherit, nil, nil, nil
	case "discard":
		return io.Discard, nil, nil, nil
	}
	f, err := stdos.OpenFile(mode, stdos.O_WRONLY|stdos.O_CREATE|stdos.O_TRUNC, 0644)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%s: cannot open %s redirect file %q: %v", fn, which, mode, err)
	}
	return f, nil, f, nil
}

// sameSpawnFile reports whether path names the file f is open on, so stdout
// and stderr redirected to one file share a single descriptor (one offset)
// instead of truncating and overwriting each other.
func sameSpawnFile(f *stdos.File, path string) bool {
	if f == nil || !isSpawnPath(path) {
		return false
	}
	a, err := f.Stat()
	if err != nil {
		return false
	}
	b, err := stdos.Stat(path)
	return err == nil && stdos.SameFile(a, b)
}

// spawnFn implements `os.spawn(argv[, options]) -> os.Process`. Non-blocking:
// starts the child and returns immediately with a handle. A background goroutine
// calls cmd.Wait() and records the result; `os.wait` / `os.poll` consult it.
// `options` is a `map of string to string` with optional "stdout" / "stderr"
// keys, each "capture" (default; returned by os.wait), "inherit" (the parent's
// stream, so a child's diagnostics appear live), "discard", or a file path
// containing a separator. Any other key or mode is an error.
func spawnFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if !execSupported() {
		return interpreter.Null(), execUnsupportedErr("os.spawn")
	}
	if len(args) < 1 || len(args) > 2 {
		return interpreter.Null(), fmt.Errorf("os.spawn expects 1 or 2 arguments (argv list[, options map]), got %d", len(args))
	}
	argv, err := argvFromList("os.spawn", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	var opts interpreter.Value
	if len(args) == 2 {
		if args[1].Kind != interpreter.KindMap {
			return interpreter.Null(), fmt.Errorf("os.spawn: options must be a map of string to string, got %s", args[1].Kind)
		}
		opts = args[1]
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.WaitDelay = procWaitDelay
	state := &processState{cmd: cmd, done: make(chan struct{})}

	// Resolve each stream's destination (default: capture). On a file-open error
	// close any file already opened for the other stream, so no fd leaks.
	outMode, errMode, err := spawnOptions("os.spawn", opts)
	if err != nil {
		return interpreter.Null(), err
	}
	outW, outBuf, outFile, err := resolveSpawnStream("os.spawn", "stdout", outMode, stdos.Stdout)
	if err != nil {
		return interpreter.Null(), err
	}
	var errW io.Writer
	var errBuf *capBuffer
	var errFile *stdos.File
	if sameSpawnFile(outFile, errMode) {
		// One descriptor for both: exec passes the same *os.File as fd 1 and 2.
		errW = outW
	} else {
		errW, errBuf, errFile, err = resolveSpawnStream("os.spawn", "stderr", errMode, stdos.Stderr)
		if err != nil {
			if outFile != nil {
				_ = outFile.Close()
			}
			return interpreter.Null(), err
		}
	}
	cmd.Stdout, cmd.Stderr = outW, errW
	state.stdout, state.stderr = outBuf, errBuf
	state.outFile, state.errFile = outFile, errFile

	// Bound the live-process registry before starting the child, so a script that
	// leaks os.Process handles (spawns without os.wait / os.release) fails with a
	// catchable error instead of forking without limit. Checked before Start so no
	// orphan child is left behind on overflow.
	handlesMu.Lock()
	tooMany := len(handles) >= maxProcHandles
	handlesMu.Unlock()
	if tooMany {
		closeSpawnFiles(outFile, errFile)
		return interpreter.Null(), fmt.Errorf("os.spawn: too many live child processes (limit %d); each os.spawn needs a matching os.wait or os.release", maxProcHandles)
	}
	if err := cmd.Start(); err != nil {
		closeSpawnFiles(outFile, errFile)
		return interpreter.Null(), fmt.Errorf("os.spawn: %v", err)
	}
	// The real OS pid, captured now (cmd.Process is set once Start succeeds). It is
	// informational only; os.wait / poll / kill / release use the registry handle.
	pid := int64(cmd.Process.Pid)
	handlesMu.Lock()
	nextID++
	id := nextID
	handles[id] = state
	handlesMu.Unlock()
	go func() {
		err := cmd.Wait()
		handlesMu.Lock()
		if cmd.ProcessState != nil {
			state.exitCode = int64(cmd.ProcessState.ExitCode())
			// WaitDelay fired because a descendant held the pipes open past the
			// grace; the process itself exited and reported a code, so this is a
			// clean result, not a wait failure (os.wait must not report it as one).
			if errors.Is(err, exec.ErrWaitDelay) {
				err = nil
			}
		}
		state.waitErr = err
		// Drain any capture buffers into strings and drop them (idempotent
		// os.wait / waitBytes return these); close any redirect files. A nil buffer means that stream
		// was inherited / discarded / file-redirected, so its os.wait field is "".
		// Ordered before close(done) under the lock, so a waiter that unblocks on
		// <-done sees the drained strings.
		if state.stdout != nil {
			state.outRaw, state.outTrunc = state.stdout.buf.String(), state.stdout.truncated
			state.stdout = nil
		}
		if state.stderr != nil {
			state.errRaw, state.errTrunc = state.stderr.buf.String(), state.stderr.truncated
			state.stderr = nil
		}
		closeSpawnFiles(state.outFile, state.errFile)
		state.outFile = nil
		state.errFile = nil
		close(state.done)
		handlesMu.Unlock()
	}()
	return makeProcess(id, pid), nil
}

// closeSpawnFiles closes any non-nil redirect files (ignoring errors - the child
// is done or never started, and a close error on a log file is not worth failing
// the call over).
func closeSpawnFiles(files ...*stdos.File) {
	for _, f := range files {
		if f != nil {
			_ = f.Close()
		}
	}
}

// waitFn implements `os.wait(p) -> os.Result`. Blocks until the
// process terminates, then returns the captured streams and exit
// code. Subsequent calls on the same handle return the same result
// (idempotent).
func waitFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return waitProcess("os.wait", args, false)
}

// waitBytesFn implements `os.waitBytes(p) -> os.BytesResult`: os.wait with
// the captured streams as bytes. Either form may read the same handle.
func waitBytesFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return waitProcess("os.waitBytes", args, true)
}

// waitProcess is os.wait (asBytes false) and os.waitBytes (asBytes true).
func waitProcess(fnName string, args []interpreter.Value, asBytes bool) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("%s expects 1 argument (process handle), got %d", fnName, len(args))
	}
	handle, err := extractHandle(fnName, args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	handlesMu.Lock()
	state, ok := handles[handle]
	handlesMu.Unlock()
	if !ok {
		return interpreter.Null(), fmt.Errorf("%s: unknown process handle (%d)", fnName, handle)
	}
	<-state.done
	// A non-ExitError from cmd.Wait (a stdout/stderr pipe-copy failure, say)
	// means the process didn't cleanly report an exit code - surfacing exit 0
	// with partial output would be reporting success where there was none.
	handlesMu.Lock()
	werr := state.waitErr
	handlesMu.Unlock()
	if werr != nil {
		if _, isExit := werr.(*exec.ExitError); !isExit {
			return interpreter.Null(), fmt.Errorf("%s: process did not complete cleanly: %v", fnName, werr)
		}
	}
	// The reaper has drained the buffers into outRaw / errRaw by the time
	// done is closed, so read those (stdout / stderr are now nil). The handle
	// stays in the registry so os.wait / os.poll remain idempotent; a
	// long-running spawner drops handles it is done with via os.release.
	return captureResult(fnName, asBytes, state.exitCode, state.outRaw, state.errRaw, state.outTrunc, state.errTrunc)
}

// releaseFn implements `os.release(p) -> bool`. Drops the process handle from
// the registry, freeing its captured output. The process must have finished
// (os.wait / os.poll true); releasing a live handle is an error. Returns
// whether the handle existed. A server spawning a child per job calls this once
// it has read the result, so the registry does not grow without bound.
func releaseFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("os.release expects 1 argument (process handle), got %d", len(args))
	}
	handle, err := extractHandle("os.release", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	handlesMu.Lock()
	defer handlesMu.Unlock()
	state, ok := handles[handle]
	if !ok {
		return interpreter.BoolVal(false), nil
	}
	select {
	case <-state.done:
		delete(handles, handle)
		return interpreter.BoolVal(true), nil
	default:
		return interpreter.Null(), fmt.Errorf("os.release: process is still running (wait or kill it first)")
	}
}

// pollFn implements `os.poll(p) -> bool`. Pure predicate, no side
// effects: true if and only if a following `os.wait` would return
// immediately.
func pollFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("os.poll expects 1 argument (process handle), got %d", len(args))
	}
	handle, err := extractHandle("os.poll", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	handlesMu.Lock()
	state, ok := handles[handle]
	handlesMu.Unlock()
	if !ok {
		return interpreter.Null(), fmt.Errorf("os.poll: unknown process handle (%d)", handle)
	}
	select {
	case <-state.done:
		return interpreter.BoolVal(true), nil
	default:
		return interpreter.BoolVal(false), nil
	}
}

// killFn implements `os.kill(p)`. Sends SIGTERM to the child. A
// subsequent `os.wait` returns whatever the OS reports for the
// terminated process. Signal variants beyond SIGTERM are
// deliberately out of scope - users who need them reach for a
// future enhancement.
func killFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("os.kill expects 1 argument (process handle), got %d", len(args))
	}
	handle, err := extractHandle("os.kill", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	handlesMu.Lock()
	state, ok := handles[handle]
	handlesMu.Unlock()
	if !ok {
		return interpreter.Null(), fmt.Errorf("os.kill: unknown process handle (%d)", handle)
	}
	if state.cmd.Process == nil {
		return interpreter.Null(), fmt.Errorf("os.kill: process has no live OS handle")
	}
	if err := terminateProcess(state.cmd.Process); err != nil {
		// "process already finished" is a benign race - the wait
		// goroutine just hasn't recorded yet. Surface as a no-op.
		if err != stdos.ErrProcessDone {
			return interpreter.Null(), fmt.Errorf("os.kill: %v", err)
		}
	}
	return interpreter.Null(), nil
}

// terminateProcess asks a child to stop, portably. On Unix it sends SIGTERM;
// on Windows, which supports only Kill/Interrupt through Process.Signal, it
// falls back to Process.Kill so os.kill isn't a silent no-op.
func terminateProcess(p *stdos.Process) error {
	if runtime.GOOS == "windows" {
		return p.Kill()
	}
	return p.Signal(syscall.SIGTERM)
}

// parser import for the package - used in oslib.go's Install for
// struct field types. Re-exported here so go build doesn't drop the
// import when oslib.go is the only consumer.
var _ = parser.PrimitiveType
