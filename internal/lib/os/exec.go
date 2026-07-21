// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package oslib

import (
	"bytes"
	"fmt"
	stdos "os"
	"os/exec"
	"runtime"
	"sync"
	"syscall"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

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

func (c *capBuffer) String() string {
	if c.truncated {
		return c.buf.String() + "\n[output truncated at 16 MiB]"
	}
	return c.buf.String()
}

type processState struct {
	cmd      *exec.Cmd
	stdout   *capBuffer
	stderr   *capBuffer
	done     chan struct{} // closed when cmd.Wait() returns
	exitCode int64
	waitErr  error
	// outStr / errStr hold the captured output once the process is reaped;
	// the reaper drains the buffers into these and drops the *bytes.Buffer so
	// a terminated handle no longer pins a live growable buffer for the
	// program's life. os.wait reads these (idempotent, unchanged result).
	outStr string
	errStr string
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
// `os.wait` (waiting on a `spawn`'d handle).
func makeResult(exitCode int64, stdout, stderr string) interpreter.Value {
	return interpreter.NamespacedStructVal("os", "Result", []interpreter.StructField{
		{Name: "exitCode", Value: interpreter.IntVal(exitCode)},
		{Name: "stdout", Value: interpreter.StringVal(stdout)},
		{Name: "stderr", Value: interpreter.StringVal(stderr)},
	})
}

// makeProcess constructs an `os.Process{pid}` Value.
func makeProcess(pid int64) interpreter.Value {
	return interpreter.NamespacedStructVal("os", "Process", []interpreter.StructField{
		{Name: "pid", Value: interpreter.IntVal(pid)},
	})
}

// extractPid pulls the pid field out of an `os.Process` value. A
// non-Process value is a typed runtime error; missing field is an
// internal error (the struct was built outside this library).
func extractPid(fnName string, v interpreter.Value) (int64, error) {
	if v.Kind != interpreter.KindStruct || v.StructNS != "os" || v.StructName != "Process" {
		return 0, fmt.Errorf("%s: argument must be an os.Process, got %s", fnName, v.Kind)
	}
	for _, f := range v.Fields {
		if f.Name == "pid" {
			if f.Value.Kind != interpreter.KindInt {
				return 0, fmt.Errorf("%s: os.Process.pid is not int (got %s)", fnName, f.Value.Kind)
			}
			return f.Value.Int, nil
		}
	}
	return 0, fmt.Errorf("%s: os.Process has no pid field", fnName)
}

// runFn implements `os.run(argv) -> os.Result`. Blocking: runs the
// command to completion and returns the captured streams and exit
// code as a single result struct. A non-zero exit code is NOT an
// error - the caller branches on `$result.exitCode`. Boundary
// failures (program not found, not executable, fork/exec failure)
// are typed runtime errors.
func runFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if !execSupported() {
		return interpreter.Null(), execUnsupportedErr("os.run")
	}
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("os.run expects 1 argument (argv list), got %d", len(args))
	}
	argv, err := argvFromList("os.run", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	var outBuf, errBuf capBuffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		// ExitError means the process ran and reported non-zero exit;
		// that's a result, not a boundary error.
		if exitErr, ok := err.(*exec.ExitError); ok {
			return makeResult(int64(exitErr.ExitCode()), outBuf.String(), errBuf.String()), nil
		}
		return interpreter.Null(), fmt.Errorf("os.run: %v", err)
	}
	return makeResult(int64(cmd.ProcessState.ExitCode()), outBuf.String(), errBuf.String()), nil
}

// spawnFn implements `os.spawn(argv) -> os.Process`. Non-blocking:
// starts the child and returns immediately with a handle. A
// background goroutine calls cmd.Wait() and records the result;
// `os.wait` / `os.poll` consult the recorded state.
func spawnFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if !execSupported() {
		return interpreter.Null(), execUnsupportedErr("os.spawn")
	}
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("os.spawn expects 1 argument (argv list), got %d", len(args))
	}
	argv, err := argvFromList("os.spawn", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	state := &processState{
		cmd:    cmd,
		stdout: &capBuffer{},
		stderr: &capBuffer{},
		done:   make(chan struct{}),
	}
	cmd.Stdout = state.stdout
	cmd.Stderr = state.stderr
	if err := cmd.Start(); err != nil {
		return interpreter.Null(), fmt.Errorf("os.spawn: %v", err)
	}
	handlesMu.Lock()
	nextID++
	id := nextID
	handles[id] = state
	handlesMu.Unlock()
	go func() {
		err := cmd.Wait()
		handlesMu.Lock()
		state.waitErr = err
		if cmd.ProcessState != nil {
			state.exitCode = int64(cmd.ProcessState.ExitCode())
		}
		// Drain the buffers into strings and drop them (idempotent os.wait
		// returns these). Ordered before close(done) under the lock, so a
		// waiter that unblocks on <-done sees the drained strings.
		state.outStr = state.stdout.String()
		state.errStr = state.stderr.String()
		state.stdout = nil
		state.stderr = nil
		close(state.done)
		handlesMu.Unlock()
	}()
	return makeProcess(id), nil
}

// waitFn implements `os.wait(p) -> os.Result`. Blocks until the
// process terminates, then returns the captured streams and exit
// code. Subsequent calls on the same handle return the same result
// (idempotent).
func waitFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("os.wait expects 1 argument (process handle), got %d", len(args))
	}
	pid, err := extractPid("os.wait", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	handlesMu.Lock()
	state, ok := handles[pid]
	handlesMu.Unlock()
	if !ok {
		return interpreter.Null(), fmt.Errorf("os.wait: unknown process handle (pid %d)", pid)
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
			return interpreter.Null(), fmt.Errorf("os.wait: process did not complete cleanly: %v", werr)
		}
	}
	// The reaper has drained the buffers into outStr / errStr by the time
	// done is closed, so read those (stdout / stderr are now nil). The handle
	// stays in the registry so os.wait / os.poll remain idempotent; a
	// long-running spawner drops handles it is done with via os.release.
	return makeResult(state.exitCode, state.outStr, state.errStr), nil
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
	pid, err := extractPid("os.release", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	handlesMu.Lock()
	defer handlesMu.Unlock()
	state, ok := handles[pid]
	if !ok {
		return interpreter.BoolVal(false), nil
	}
	select {
	case <-state.done:
		delete(handles, pid)
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
	pid, err := extractPid("os.poll", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	handlesMu.Lock()
	state, ok := handles[pid]
	handlesMu.Unlock()
	if !ok {
		return interpreter.Null(), fmt.Errorf("os.poll: unknown process handle (pid %d)", pid)
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
	pid, err := extractPid("os.kill", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	handlesMu.Lock()
	state, ok := handles[pid]
	handlesMu.Unlock()
	if !ok {
		return interpreter.Null(), fmt.Errorf("os.kill: unknown process handle (pid %d)", pid)
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
