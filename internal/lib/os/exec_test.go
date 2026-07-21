// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package oslib

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// stringList builds a Jennifer list-of-string Value (with no element
// type stamping; the exec helpers only inspect Kind / Str).
func stringList(elems ...string) interpreter.Value {
	list := make([]interpreter.Value, len(elems))
	for i, e := range elems {
		list[i] = interpreter.StringVal(e)
	}
	return interpreter.Value{Kind: interpreter.KindList, List: list}
}

func skipIfNotLinux(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("os.run / spawn tests use /bin/sh; skipped on non-Linux until cross-platform support lands")
	}
}

func TestRunCapturesStdoutAndExitZero(t *testing.T) {
	skipIfNotLinux(t)
	v, err := runFn(interpreter.BuiltinCtx{}, []interpreter.Value{stringList("/bin/echo", "hello")})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if v.Kind != interpreter.KindStruct || v.StructNS != "os" || v.StructName != "Result" {
		t.Fatalf("not os.Result: %+v", v)
	}
	for _, f := range v.Fields {
		switch f.Name {
		case "exitCode":
			if f.Value.Int != 0 {
				t.Errorf("exit code = %d, want 0", f.Value.Int)
			}
		case "stdout":
			if !strings.Contains(f.Value.Str, "hello") {
				t.Errorf("stdout = %q", f.Value.Str)
			}
		case "stderr":
			if f.Value.Str != "" {
				t.Errorf("stderr = %q, want empty", f.Value.Str)
			}
		}
	}
}

func TestRunNonZeroExitIsValue(t *testing.T) {
	// Non-zero exit codes are values, NOT errors. The caller branches
	// on $result.exitCode.
	skipIfNotLinux(t)
	v, err := runFn(interpreter.BuiltinCtx{}, []interpreter.Value{stringList("/bin/sh", "-c", "exit 7")})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	for _, f := range v.Fields {
		if f.Name == "exitCode" && f.Value.Int != 7 {
			t.Errorf("exit code = %d, want 7", f.Value.Int)
		}
	}
}

func TestRunSeparatesStdoutFromStderr(t *testing.T) {
	skipIfNotLinux(t)
	v, err := runFn(interpreter.BuiltinCtx{}, []interpreter.Value{
		stringList("/bin/sh", "-c", "echo out; echo err 1>&2"),
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	for _, f := range v.Fields {
		switch f.Name {
		case "stdout":
			if !strings.Contains(f.Value.Str, "out") || strings.Contains(f.Value.Str, "err") {
				t.Errorf("stdout = %q", f.Value.Str)
			}
		case "stderr":
			if !strings.Contains(f.Value.Str, "err") || strings.Contains(f.Value.Str, "out") {
				t.Errorf("stderr = %q", f.Value.Str)
			}
		}
	}
}

func TestRunUnknownProgramIsRuntimeError(t *testing.T) {
	skipIfNotLinux(t)
	_, err := runFn(interpreter.BuiltinCtx{}, []interpreter.Value{stringList("/no/such/binary/anywhere")})
	if err == nil {
		t.Fatal("expected boundary error, got nil")
	}
	if !strings.Contains(err.Error(), "os.run") {
		t.Errorf("error doesn't mention os.run: %v", err)
	}
}

func TestRunEmptyArgvErrors(t *testing.T) {
	_, err := runFn(interpreter.BuiltinCtx{}, []interpreter.Value{stringList()})
	if err == nil || !strings.Contains(err.Error(), "at least one element") {
		t.Fatalf("got %v", err)
	}
}

func TestSpawnWaitRoundTrip(t *testing.T) {
	skipIfNotLinux(t)
	p, err := spawnFn(interpreter.BuiltinCtx{}, []interpreter.Value{
		stringList("/bin/sh", "-c", "echo spawned; exit 0"),
	})
	if err != nil {
		t.Fatalf("spawn err: %v", err)
	}
	if p.StructNS != "os" || p.StructName != "Process" {
		t.Fatalf("not os.Process: %+v", p)
	}
	r, err := waitFn(interpreter.BuiltinCtx{}, []interpreter.Value{p})
	if err != nil {
		t.Fatalf("wait err: %v", err)
	}
	for _, f := range r.Fields {
		switch f.Name {
		case "exitCode":
			if f.Value.Int != 0 {
				t.Errorf("exit = %d", f.Value.Int)
			}
		case "stdout":
			if !strings.Contains(f.Value.Str, "spawned") {
				t.Errorf("stdout = %q", f.Value.Str)
			}
		}
	}
}

// Handles are keyed by a monotonic internal id, not the OS pid, so two spawned
// processes never share a handle even if the OS recycles a pid between them.
// Each handle waits its own child and returns that child's output - no overwrite.
func TestSpawnHandlesAreDistinctAndOwnTheirProcess(t *testing.T) {
	skipIfNotLinux(t)
	p1, err := spawnFn(interpreter.BuiltinCtx{}, []interpreter.Value{stringList("/bin/echo", "one")})
	if err != nil {
		t.Fatalf("spawn1 err: %v", err)
	}
	id1, _ := extractPid("t", p1)
	// Let the first child fully terminate so the OS is free to recycle its pid.
	r1, err := waitFn(interpreter.BuiltinCtx{}, []interpreter.Value{p1})
	if err != nil {
		t.Fatalf("wait1 err: %v", err)
	}
	p2, err := spawnFn(interpreter.BuiltinCtx{}, []interpreter.Value{stringList("/bin/echo", "two")})
	if err != nil {
		t.Fatalf("spawn2 err: %v", err)
	}
	id2, _ := extractPid("t", p2)
	if id1 == id2 {
		t.Fatalf("handles collided: id1=%d id2=%d", id1, id2)
	}
	// The first handle still resolves to the first child's result, and the
	// second to the second child's - neither overwrote the other.
	r1b, err := waitFn(interpreter.BuiltinCtx{}, []interpreter.Value{p1})
	if err != nil {
		t.Fatalf("wait1b err: %v", err)
	}
	r2, err := waitFn(interpreter.BuiltinCtx{}, []interpreter.Value{p2})
	if err != nil {
		t.Fatalf("wait2 err: %v", err)
	}
	if !r1.Equal(r1b) {
		t.Errorf("first handle not stable after a second spawn: %+v vs %+v", r1, r1b)
	}
	stdoutOf := func(r interpreter.Value) string {
		for _, f := range r.Fields {
			if f.Name == "stdout" {
				return f.Value.Str
			}
		}
		return ""
	}
	if !strings.Contains(stdoutOf(r1), "one") {
		t.Errorf("handle 1 stdout = %q, want 'one'", stdoutOf(r1))
	}
	if !strings.Contains(stdoutOf(r2), "two") {
		t.Errorf("handle 2 stdout = %q, want 'two'", stdoutOf(r2))
	}
}

func TestWaitIsIdempotent(t *testing.T) {
	skipIfNotLinux(t)
	p, err := spawnFn(interpreter.BuiltinCtx{}, []interpreter.Value{stringList("/bin/echo", "x")})
	if err != nil {
		t.Fatalf("spawn err: %v", err)
	}
	r1, err := waitFn(interpreter.BuiltinCtx{}, []interpreter.Value{p})
	if err != nil {
		t.Fatalf("wait1 err: %v", err)
	}
	r2, err := waitFn(interpreter.BuiltinCtx{}, []interpreter.Value{p})
	if err != nil {
		t.Fatalf("wait2 err: %v", err)
	}
	if !r1.Equal(r2) {
		t.Errorf("non-idempotent: r1=%+v r2=%+v", r1, r2)
	}
}

// os.release drops a finished handle from the registry so a long-running
// spawner does not leak. A live handle can't be released; a released handle is
// gone (a following wait errors).
func TestReleaseDropsFinishedHandle(t *testing.T) {
	skipIfNotLinux(t)
	p, err := spawnFn(interpreter.BuiltinCtx{}, []interpreter.Value{stringList("/bin/echo", "x")})
	if err != nil {
		t.Fatalf("spawn err: %v", err)
	}
	if _, err := waitFn(interpreter.BuiltinCtx{}, []interpreter.Value{p}); err != nil {
		t.Fatalf("wait err: %v", err)
	}
	rel, err := releaseFn(interpreter.BuiltinCtx{}, []interpreter.Value{p})
	if err != nil || rel.Kind != interpreter.KindBool || !rel.Bool {
		t.Fatalf("release = %+v, err %v; want true", rel, err)
	}
	// The handle is gone now; wait on it errors.
	if _, err := waitFn(interpreter.BuiltinCtx{}, []interpreter.Value{p}); err == nil {
		t.Error("wait on a released handle should error")
	}
	// Releasing an unknown handle just returns false.
	if rel, err := releaseFn(interpreter.BuiltinCtx{}, []interpreter.Value{p}); err != nil || rel.Bool {
		t.Errorf("release of a released handle = %+v, err %v; want false", rel, err)
	}
}

// Releasing a still-running process is an error (wait or kill it first).
func TestReleaseLiveProcessErrors(t *testing.T) {
	skipIfNotLinux(t)
	p, err := spawnFn(interpreter.BuiltinCtx{}, []interpreter.Value{stringList("/bin/sh", "-c", "sleep 2")})
	if err != nil {
		t.Fatalf("spawn err: %v", err)
	}
	if _, err := releaseFn(interpreter.BuiltinCtx{}, []interpreter.Value{p}); err == nil {
		t.Error("release of a live process should error")
	}
	_, _ = killFn(interpreter.BuiltinCtx{}, []interpreter.Value{p})
	_, _ = waitFn(interpreter.BuiltinCtx{}, []interpreter.Value{p})
}

func TestPollBeforeAndAfterExit(t *testing.T) {
	skipIfNotLinux(t)
	p, err := spawnFn(interpreter.BuiltinCtx{}, []interpreter.Value{
		stringList("/bin/sh", "-c", "sleep 0.1"),
	})
	if err != nil {
		t.Fatalf("spawn err: %v", err)
	}
	// Immediate poll: should usually be false (race-tolerant).
	v, err := pollFn(interpreter.BuiltinCtx{}, []interpreter.Value{p})
	if err != nil {
		t.Fatalf("poll err: %v", err)
	}
	if v.Bool {
		t.Logf("poll returned true immediately - scheduling raced; the process completed before we polled")
	}
	if _, err := waitFn(interpreter.BuiltinCtx{}, []interpreter.Value{p}); err != nil {
		t.Fatalf("wait err: %v", err)
	}
	v, err = pollFn(interpreter.BuiltinCtx{}, []interpreter.Value{p})
	if err != nil {
		t.Fatalf("poll-after err: %v", err)
	}
	if !v.Bool {
		t.Error("poll after wait should be true")
	}
}

func TestKillTerminatesProcess(t *testing.T) {
	skipIfNotLinux(t)
	p, err := spawnFn(interpreter.BuiltinCtx{}, []interpreter.Value{
		stringList("/bin/sleep", "30"),
	})
	if err != nil {
		t.Fatalf("spawn err: %v", err)
	}
	if _, err := killFn(interpreter.BuiltinCtx{}, []interpreter.Value{p}); err != nil {
		t.Fatalf("kill err: %v", err)
	}
	// Wait should return quickly now.
	done := make(chan struct{})
	go func() {
		waitFn(interpreter.BuiltinCtx{}, []interpreter.Value{p})
		close(done)
	}()
	select {
	case <-done:
		// ok
	case <-time.After(5 * time.Second):
		t.Fatal("wait did not return after kill")
	}
}

func TestWaitOnUnknownHandleErrors(t *testing.T) {
	// Construct a synthetic Process with an unknown pid.
	p := makeProcess(9999999)
	_, err := waitFn(interpreter.BuiltinCtx{}, []interpreter.Value{p})
	if err == nil || !strings.Contains(err.Error(), "unknown process handle") {
		t.Errorf("got %v", err)
	}
}

func TestRunRejectsNonStringArgv(t *testing.T) {
	bad := interpreter.Value{Kind: interpreter.KindList, List: []interpreter.Value{
		interpreter.StringVal("/bin/echo"),
		interpreter.IntVal(42),
	}}
	_, err := runFn(interpreter.BuiltinCtx{}, []interpreter.Value{bad})
	if err == nil || !strings.Contains(err.Error(), "argv[1]") {
		t.Errorf("got %v", err)
	}
}
