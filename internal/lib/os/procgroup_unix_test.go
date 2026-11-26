// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build unix && !tinygo

package oslib

import (
	"errors"
	stdos "os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// M29.57: os.SpawnOptions.group and os.signal.

var groupOpt = interpreter.StructField{Name: "group", Value: interpreter.BoolVal(true)}

// waitForFile polls until path exists and returns its trimmed content.
func waitForFile(t *testing.T, path string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := stdos.ReadFile(path); err == nil && strings.HasSuffix(string(b), "\n") {
			return strings.TrimSpace(string(b))
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s did not appear", path)
	return ""
}

// alive reports whether pid is a live process (a zombie counts as dead).
func alive(pid int) bool {
	if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
		return false
	}
	b, err := stdos.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	// The state follows the parenthesised command name.
	s := string(b)
	if i := strings.LastIndexByte(s, ')'); i >= 0 && i+2 < len(s) {
		return s[i+2] != 'Z'
	}
	return true
}

// awaitDead polls until pid is gone, failing after 5 s.
func awaitDead(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !alive(pid) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("pid %d still alive", pid)
}

// spawnBackground starts `sh -c 'sleep 30 & echo $! > F; <tail>'` and returns
// the process and the background sleep's pid. The sleep is killed at cleanup.
func spawnBackground(t *testing.T, group bool, tail string) (interpreter.Value, int) {
	t.Helper()
	pidFile := filepath.Join(t.TempDir(), "pid")
	var fields []interpreter.StructField
	if group {
		fields = append(fields, groupOpt)
	}
	p, err := spawnWithFn(interpreter.BuiltinCtx{}, []interpreter.Value{
		stringList("sh", "-c", "sleep 30 & echo $! > "+pidFile+"; "+tail),
		spawnOptsWith(fields...),
	})
	if err != nil {
		t.Fatal(err)
	}
	bg, err := strconv.Atoi(waitForFile(t, pidFile))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(bg, syscall.SIGKILL) })
	return p, bg
}

func sendSig(t *testing.T, p interpreter.Value, name string) {
	t.Helper()
	if _, err := signalFn(interpreter.BuiltinCtx{}, []interpreter.Value{p, interpreter.StringVal(name)}); err != nil {
		t.Fatalf("os.signal(%q): %v", name, err)
	}
}

// TestSignalGroupKillsBackgroundChild: with group, "kill" reaches the shell's
// background child too; without, only the shell dies.
func TestSignalGroupKillsBackgroundChild(t *testing.T) {
	skipIfNotLinux(t)
	ctx := interpreter.BuiltinCtx{}
	p, bg := spawnBackground(t, true, "wait")
	sendSig(t, p, "kill")
	if _, err := waitFn(ctx, []interpreter.Value{p}); err != nil {
		t.Fatal(err)
	}
	awaitDead(t, bg)

	p, bg = spawnBackground(t, false, "wait")
	sendSig(t, p, "kill")
	if _, err := waitFn(ctx, []interpreter.Value{p}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if !alive(bg) {
		t.Fatal("without group the background child died too")
	}
}

// TestSignalGroupAfterLeaderExit: the leader exits at once, its background
// child keeps the group; os.signal still reaches it, and once the group is
// empty a further signal is no error.
func TestSignalGroupAfterLeaderExit(t *testing.T) {
	skipIfNotLinux(t)
	ctx := interpreter.BuiltinCtx{}
	p, bg := spawnBackground(t, true, "exit 0")
	if _, err := waitFn(ctx, []interpreter.Value{p}); err != nil {
		t.Fatal(err)
	}
	if !alive(bg) {
		t.Fatal("background child gone before the signal")
	}
	sendSig(t, p, "kill")
	awaitDead(t, bg)
	sendSig(t, p, "term")
}

// TestSignalKillAfterIgnoredTerm: a child that ignores SIGTERM survives "term"
// and is stopped by "kill".
func TestSignalKillAfterIgnoredTerm(t *testing.T) {
	skipIfNotLinux(t)
	ctx := interpreter.BuiltinCtx{}
	ready := filepath.Join(t.TempDir(), "ready")
	p, err := spawnWithFn(ctx, []interpreter.Value{
		stringList("sh", "-c", `trap "" TERM; echo > `+ready+`; exec sleep 30`),
		spawnOptsWith(),
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForFile(t, ready)
	sendSig(t, p, "term")
	if _, err := waitTimeoutFn(ctx, []interpreter.Value{p, interpreter.IntVal(200)}); kindOf(err) != "timeout" {
		t.Fatalf("after term: %v (want the child still running)", err)
	}
	sendSig(t, p, "kill")
	if _, err := waitTimeoutFn(ctx, []interpreter.Value{p, interpreter.IntVal(5000)}); err != nil {
		t.Fatalf("after kill: %v", err)
	}
	// An exited child: nothing is sent and that is no error.
	sendSig(t, p, "kill")
}

func TestSignalNames(t *testing.T) {
	skipIfNotLinux(t)
	ctx := interpreter.BuiltinCtx{}
	for _, name := range sendSignalNames {
		p, err := spawnWithFn(ctx, []interpreter.Value{stringList("sleep", "30"), spawnOptsWith()})
		if err != nil {
			t.Fatal(err)
		}
		sendSig(t, p, name)
		if name != "kill" {
			sendSig(t, p, "kill")
		}
		if _, err := waitFn(ctx, []interpreter.Value{p}); err != nil {
			t.Fatal(err)
		}
	}
	p, err := spawnWithFn(ctx, []interpreter.Value{stringList("true"), spawnOptsWith()})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = waitFn(ctx, []interpreter.Value{p})
	for _, c := range []struct {
		args []interpreter.Value
		want string
	}{
		{[]interpreter.Value{p, interpreter.StringVal("TERM")}, `os.signal: unknown signal "TERM"; known: "term", "kill", "int", "hup", "quit", "usr1", "usr2"`},
		{[]interpreter.Value{p, interpreter.StringVal("sigterm")}, `os.signal: unknown signal "sigterm"`},
		{[]interpreter.Value{p, interpreter.IntVal(9)}, "os.signal: signal name must be string, got int"},
		{[]interpreter.Value{p}, "os.signal expects 2 arguments"},
		{[]interpreter.Value{interpreter.StringVal("x"), interpreter.StringVal("kill")}, "os.signal: argument must be an os.Process"},
	} {
		_, err := signalFn(ctx, c.args)
		if err == nil || !strings.HasPrefix(err.Error(), c.want) {
			t.Errorf("err %v, want prefix %q", err, c.want)
		}
	}
}
