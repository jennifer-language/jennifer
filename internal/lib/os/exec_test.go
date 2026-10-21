// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package oslib

import (
	"fmt"
	stdos "os"
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

// resultStdout pulls stdout + exit code out of an os.Result value.
func resultStdout(v interpreter.Value) (string, int64) {
	var out string
	var code int64
	for _, f := range v.Fields {
		switch f.Name {
		case "stdout":
			out = f.Value.Str
		case "exitCode":
			code = f.Value.Int
		}
	}
	return out, code
}

func TestRunFeedsStringStdin(t *testing.T) {
	skipIfNotLinux(t)
	v, err := runFn(interpreter.BuiltinCtx{}, []interpreter.Value{
		stringList("/bin/cat"), interpreter.StringVal("hello\nworld\n")})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	out, code := resultStdout(v)
	if out != "hello\nworld\n" {
		t.Errorf("stdout = %q, want the fed stdin echoed back", out)
	}
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
}

func TestRunFeedsBytesStdin(t *testing.T) {
	skipIfNotLinux(t)
	v, err := runFn(interpreter.BuiltinCtx{}, []interpreter.Value{
		stringList("/usr/bin/tr", "a-z", "A-Z"), interpreter.BytesVal([]byte("shout"))})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out, _ := resultStdout(v); out != "SHOUT" {
		t.Errorf("stdout = %q, want SHOUT (bytes stdin transformed)", out)
	}
}

func TestRunEmptyStdin(t *testing.T) {
	skipIfNotLinux(t)
	v, err := runFn(interpreter.BuiltinCtx{}, []interpreter.Value{
		stringList("/usr/bin/wc", "-c"), interpreter.StringVal("")})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out, _ := resultStdout(v); strings.TrimSpace(out) != "0" {
		t.Errorf("wc -c of empty stdin = %q, want 0", out)
	}
}

func TestRunLargeStdinCountedAndOutputCapped(t *testing.T) {
	skipIfNotLinux(t)
	// 20 MiB of stdin: it is fed in full (wc counts every byte) and, when echoed
	// back by cat, the OUTPUT stays capped at 16 MiB - large input does not hang
	// or blow the output contract.
	big := strings.Repeat("A", 20<<20)
	v, err := runFn(interpreter.BuiltinCtx{}, []interpreter.Value{
		stringList("/usr/bin/wc", "-c"), interpreter.StringVal(big)})
	if err != nil {
		t.Fatalf("wc err: %v", err)
	}
	if out, _ := resultStdout(v); strings.TrimSpace(out) != "20971520" {
		t.Errorf("wc -c of 20 MiB stdin = %q, want 20971520 (whole input fed)", out)
	}
	v2, err := runFn(interpreter.BuiltinCtx{}, []interpreter.Value{
		stringList("/bin/cat"), interpreter.StringVal(big)})
	if err != nil {
		t.Fatalf("cat err: %v", err)
	}
	out2, _ := resultStdout(v2)
	if !strings.Contains(out2, "[output truncated at 16 MiB]") {
		t.Errorf("echoing 20 MiB stdin should cap output at 16 MiB; got %d bytes", len(out2))
	}
}

func TestRunStdinWrongTypeErrors(t *testing.T) {
	skipIfNotLinux(t)
	_, err := runFn(interpreter.BuiltinCtx{}, []interpreter.Value{
		stringList("/bin/cat"), interpreter.IntVal(5)})
	if err == nil {
		t.Fatal("expected an error for a non-string/bytes stdin")
	}
	if !strings.Contains(err.Error(), "stdin must be a string or bytes") {
		t.Errorf("error = %v, want a stdin-type message", err)
	}
}

func TestRunTooManyArgsErrors(t *testing.T) {
	skipIfNotLinux(t)
	_, err := runFn(interpreter.BuiltinCtx{}, []interpreter.Value{
		stringList("/bin/cat"), interpreter.StringVal("x"), interpreter.StringVal("y")})
	if err == nil {
		t.Fatal("expected an error for 3 args")
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
	id1, _ := extractHandle("t", p1)
	// Let the first child fully terminate so the OS is free to recycle its pid.
	r1, err := waitFn(interpreter.BuiltinCtx{}, []interpreter.Value{p1})
	if err != nil {
		t.Fatalf("wait1 err: %v", err)
	}
	p2, err := spawnFn(interpreter.BuiltinCtx{}, []interpreter.Value{stringList("/bin/echo", "two")})
	if err != nil {
		t.Fatalf("spawn2 err: %v", err)
	}
	id2, _ := extractHandle("t", p2)
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
	p := makeProcess(9999999, 9999999)
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

// TestRunReturnsPromptlyWhenChildBackgrounds pins os.run must not block
// until a backgrounded grandchild exits. `sh` forks a 30s sleep that inherits
// the stdout pipe, then exits 0; run must return within the WaitDelay grace
// (~0.5s), not after 30s, with the process's own output and exit code.
func TestRunReturnsPromptlyWhenChildBackgrounds(t *testing.T) {
	skipIfNotLinux(t)
	start := time.Now()
	v, err := runFn(interpreter.BuiltinCtx{}, []interpreter.Value{
		stringList("/bin/sh", "-c", "sleep 30 & echo hi"),
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if elapsed > 10*time.Second {
		t.Fatalf("os.run blocked on the backgrounded child: took %v", elapsed)
	}
	out, code := resultStdout(v)
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out, "hi") {
		t.Errorf("stdout = %q, want to contain %q", out, "hi")
	}
}

// optsMap builds a `map of string to string` Value for os.spawn options.
func optsMap(kv ...string) interpreter.Value {
	var entries []interpreter.MapEntry
	for i := 0; i+1 < len(kv); i += 2 {
		entries = append(entries, interpreter.MapEntry{
			Key:   interpreter.StringVal(kv[i]),
			Value: interpreter.StringVal(kv[i+1]),
		})
	}
	return interpreter.Value{Kind: interpreter.KindMap, Map: entries}
}

func fieldInt(v interpreter.Value, name string) (int64, bool) {
	for _, f := range v.Fields {
		if f.Name == name {
			return f.Value.Int, f.Value.Kind == interpreter.KindInt
		}
	}
	return 0, false
}

func resultField(v interpreter.Value, name string) string {
	for _, f := range v.Fields {
		if f.Name == name {
			return f.Value.Str
		}
	}
	return ""
}

// os.Process carries a small monotonic handle AND the real OS pid; the two are
// distinct and the pid names a live process.
func TestSpawnExposesRealPidAndHandle(t *testing.T) {
	skipIfNotLinux(t)
	p, err := spawnFn(interpreter.BuiltinCtx{}, []interpreter.Value{stringList("sleep", "2")})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	handle, ok := fieldInt(p, "handle")
	if !ok || handle <= 0 {
		t.Fatalf("handle field missing/invalid: %d (ok=%v)", handle, ok)
	}
	pid, ok := fieldInt(p, "pid")
	if !ok || pid <= 1 {
		t.Fatalf("pid field missing/invalid: %d (ok=%v)", pid, ok)
	}
	if pid == handle {
		t.Errorf("pid (%d) must be the real OS pid, not the handle (%d)", pid, handle)
	}
	if _, serr := stdos.Stat(fmt.Sprintf("/proc/%d", pid)); serr != nil {
		t.Errorf("reported pid %d has no /proc entry: %v", pid, serr)
	}
	_, _ = killFn(interpreter.BuiltinCtx{}, []interpreter.Value{p})
	_, _ = waitFn(interpreter.BuiltinCtx{}, []interpreter.Value{p})
}

// The stdout / stderr options route each stream: capture (default, returned by
// os.wait), discard (dropped), and a file path (written to the file).
func TestSpawnStreamModes(t *testing.T) {
	skipIfNotLinux(t)
	bctx := interpreter.BuiltinCtx{}
	prog := func() interpreter.Value { return stringList("sh", "-c", "echo OUT; echo ERR >&2") }

	// capture (default): os.wait returns both streams.
	p, _ := spawnFn(bctx, []interpreter.Value{prog()})
	r, err := waitFn(bctx, []interpreter.Value{p})
	if err != nil {
		t.Fatalf("wait(capture): %v", err)
	}
	if !strings.Contains(resultField(r, "stdout"), "OUT") || !strings.Contains(resultField(r, "stderr"), "ERR") {
		t.Errorf("capture lost output: out=%q err=%q", resultField(r, "stdout"), resultField(r, "stderr"))
	}

	// discard: os.wait returns empty for the discarded streams.
	p, _ = spawnFn(bctx, []interpreter.Value{prog(), optsMap("stdout", "discard", "stderr", "discard")})
	r, _ = waitFn(bctx, []interpreter.Value{p})
	if resultField(r, "stdout") != "" || resultField(r, "stderr") != "" {
		t.Errorf("discard should drop both: out=%q err=%q", resultField(r, "stdout"), resultField(r, "stderr"))
	}

	// file: the stream is written to the file; os.wait's field is empty.
	logPath := t.TempDir() + "/err.log"
	p, _ = spawnFn(bctx, []interpreter.Value{prog(), optsMap("stderr", logPath)})
	r, _ = waitFn(bctx, []interpreter.Value{p})
	if resultField(r, "stderr") != "" {
		t.Errorf("file-redirected stderr should be empty in the Result, got %q", resultField(r, "stderr"))
	}
	body, rerr := stdos.ReadFile(logPath)
	if rerr != nil || !strings.Contains(string(body), "ERR") {
		t.Errorf("redirect file: err=%v contents=%q", rerr, string(body))
	}
}

// Bad options surface a catchable error rather than a crash.
func TestSpawnBadOptions(t *testing.T) {
	skipIfNotLinux(t)
	bctx := interpreter.BuiltinCtx{}
	// options that are not a map
	if _, err := spawnFn(bctx, []interpreter.Value{stringList("true"), interpreter.IntVal(1)}); err == nil {
		t.Error("non-map options should error")
	}
	// a redirect path that cannot be created
	if _, err := spawnFn(bctx, []interpreter.Value{stringList("true"), optsMap("stdout", "/no/such/dir/xyz.log")}); err == nil {
		t.Error("an unwritable redirect path should error")
	}
}
