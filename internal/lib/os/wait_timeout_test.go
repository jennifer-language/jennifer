// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package oslib

import (
	"bytes"
	stdos "os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	tasklib "jennifer-lang.dev/jennifer/internal/lib/task"
	timelib "jennifer-lang.dev/jennifer/internal/lib/time"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// M29.45: os.waitTimeout / waitBytesTimeout bound os.wait; os.spawnWith takes
// the stream modes in an os.SpawnOptions struct.

// kindOf is the kind of a builtin's error: a KindedError's Kind field, ""
// (runtime) for a plain error.
func kindOf(err error) string {
	if err == nil {
		return "<nil>"
	}
	v := reflect.ValueOf(err)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() == reflect.Struct {
		if f := v.FieldByName("Kind"); f.IsValid() && f.Kind() == reflect.String {
			return f.String()
		}
	}
	return ""
}

func spawnOpts(stdout, stderr string) interpreter.Value {
	return interpreter.NamespacedStructVal(LibraryName, "SpawnOptions", []interpreter.StructField{
		{Name: "stdout", Value: interpreter.StringVal(stdout)},
		{Name: "stderr", Value: interpreter.StringVal(stderr)},
	})
}

// TestWaitTimeoutLeavesProcessRunning: a process still running at the
// deadline throws "timeout" and keeps running; a later wait returns its
// result, and ms 0 on an exited process returns it every time.
func TestWaitTimeoutLeavesProcessRunning(t *testing.T) {
	skipIfNotLinux(t)
	ctx := interpreter.BuiltinCtx{}
	p, err := spawnFn(ctx, []interpreter.Value{stringList("sh", "-c", "sleep 0.3; echo done")})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = waitTimeoutFn(ctx, []interpreter.Value{p, interpreter.IntVal(50)})
	if kindOf(err) != "timeout" || !strings.Contains(err.Error(), "os.waitTimeout: the process did not exit within 50 ms") {
		t.Fatalf("err = %v (kind %q), want a timeout", err, kindOf(err))
	}
	if took := time.Since(start); took < 40*time.Millisecond || took > 250*time.Millisecond {
		t.Errorf("took %v, want about 50 ms", took)
	}
	if _, err := waitBytesTimeoutFn(ctx, []interpreter.Value{p, interpreter.IntVal(0)}); kindOf(err) != "timeout" {
		t.Errorf("poll of a running process: err = %v, want a timeout", err)
	}
	if polled, _ := pollFn(ctx, []interpreter.Value{p}); polled.Bool {
		t.Error("process finished early; the test needs it running")
	}
	r, err := waitTimeoutFn(ctx, []interpreter.Value{p, interpreter.IntVal(10000)})
	if err != nil {
		t.Fatal(err)
	}
	if out := r.Fields[1].Value.Str; out != "done\n" {
		t.Errorf("stdout = %q", out)
	}
	for i := 0; i < 50; i++ {
		b, err := waitBytesTimeoutFn(ctx, []interpreter.Value{p, interpreter.IntVal(0)})
		if err != nil || string(b.Fields[1].Value.Bytes) != "done\n" {
			t.Fatalf("waitBytesTimeout(p, 0) after exit: %v, %v", b, err)
		}
	}
	if r, err := waitFn(ctx, []interpreter.Value{p}); err != nil || r.Fields[1].Value.Str != "done\n" {
		t.Errorf("os.wait after: %v, %v", r, err)
	}
}

// TestWaitTimeoutArguments: ms outside 0..max and wrong shapes are "runtime".
func TestWaitTimeoutArguments(t *testing.T) {
	skipIfNotLinux(t)
	ctx := interpreter.BuiltinCtx{}
	p, err := spawnFn(ctx, []interpreter.Value{stringList("true")})
	if err != nil {
		t.Fatal(err)
	}
	defer waitFn(ctx, []interpreter.Value{p})
	for _, c := range []struct {
		args []interpreter.Value
		frag string
	}{
		{[]interpreter.Value{p, interpreter.IntVal(-1)}, "ms must be 0 to"},
		{[]interpreter.Value{p, interpreter.IntVal(maxTimeoutMillis + 1)}, "ms must be 0 to"},
		{[]interpreter.Value{p, interpreter.StringVal("1")}, "ms must be int"},
		{[]interpreter.Value{p}, "expects 2 arguments"},
	} {
		for _, fn := range []interpreter.Builtin{waitTimeoutFn, waitBytesTimeoutFn} {
			_, err := fn(ctx, c.args)
			if err == nil || !strings.Contains(err.Error(), c.frag) || kindOf(err) != "" {
				t.Errorf("err = %v, want runtime %q", err, c.frag)
			}
		}
	}
}

// TestWaitTimeoutCancel: task.cancel wakes a task blocked in os.waitTimeout
// with "task cancelled"; the process keeps running and is killed after.
func TestWaitTimeoutCancel(t *testing.T) {
	skipIfNotLinux(t)
	prog, err := parser.Parse(`use io; use os; use task; use time;
def p as os.Process init os.spawn(["sleep", "30"]);
def t as task of string init spawn {
    try { def r as os.Result init os.waitTimeout($p, 3600000); } catch (e) { return $e.message; }
    return "returned";
};
time.sleep(time.fromMilliseconds(30));
task.cancel($t);
io.printf("%s;%t", task.wait($t), os.poll($p));
os.kill($p);
def r as os.Result init os.wait($p);`)
	if err != nil {
		t.Fatal(err)
	}
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	tasklib.Install(in)
	timelib.Install(in)
	Install(in)
	done := make(chan error, 1)
	go func() { done <- in.Run(prog) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("blocked")
	}
	if buf.String() != "task cancelled;false" {
		t.Errorf("out = %q", buf.String())
	}
}

// TestWaitTimeoutNoLeak: timed-out waits leave no goroutine behind.
func TestWaitTimeoutNoLeak(t *testing.T) {
	skipIfNotLinux(t)
	ctx := interpreter.BuiltinCtx{}
	p, err := spawnFn(ctx, []interpreter.Value{stringList("sleep", "30")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = killFn(ctx, []interpreter.Value{p})
		_, _ = waitFn(ctx, []interpreter.Value{p})
	}()
	runtime.GC()
	before := runtime.NumGoroutine()
	for i := 0; i < 30; i++ {
		if _, err := waitTimeoutFn(ctx, []interpreter.Value{p, interpreter.IntVal(1)}); kindOf(err) != "timeout" {
			t.Fatalf("err = %v", err)
		}
	}
	if n := runtime.NumGoroutine(); n > before {
		t.Errorf("goroutines: %d before, %d after", before, n)
	}
}

// TestSpawnWith: the zero SpawnOptions captures both streams; the modes are
// os.spawn's; a bad mode or field type is refused.
func TestSpawnWith(t *testing.T) {
	skipIfNotLinux(t)
	ctx := interpreter.BuiltinCtx{}
	p, err := spawnWithFn(ctx, []interpreter.Value{
		stringList("sh", "-c", "echo out; echo err >&2"),
		interpreter.NamespacedStructVal(LibraryName, "SpawnOptions", nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	r, err := waitFn(ctx, []interpreter.Value{p})
	if err != nil || r.Fields[1].Value.Str != "out\n" || r.Fields[2].Value.Str != "err\n" {
		t.Fatalf("zero options: %v, %v", r, err)
	}
	log := filepath.Join(t.TempDir(), "x.log")
	p, err = spawnWithFn(ctx, []interpreter.Value{stringList("sh", "-c", "echo out; echo err >&2"), spawnOpts(log, "discard")})
	if err != nil {
		t.Fatal(err)
	}
	r, err = waitFn(ctx, []interpreter.Value{p})
	if err != nil || r.Fields[1].Value.Str != "" || r.Fields[2].Value.Str != "" {
		t.Fatalf("file + discard: %v, %v", r, err)
	}
	if body, err := stdos.ReadFile(log); err != nil || string(body) != "out\n" {
		t.Errorf("log = %q, %v", body, err)
	}
	for _, c := range []struct {
		opts interpreter.Value
		want string
	}{
		{spawnOpts("inhert", ""), `os.spawnWith: unknown stdout mode "inhert"`},
		{spawnOpts("", "discrad"), `os.spawnWith: unknown stderr mode "discrad"`},
		{optsMap("stdout", "discard"), "os.spawnWith: options must be an os.SpawnOptions"},
	} {
		_, err := spawnWithFn(ctx, []interpreter.Value{stringList("true"), c.opts})
		if err == nil || !strings.HasPrefix(err.Error(), c.want) {
			t.Errorf("err %v, want prefix %q", err, c.want)
		}
	}
	if _, err := spawnWithFn(ctx, []interpreter.Value{stringList("true")}); err == nil || !strings.Contains(err.Error(), "expects 2 arguments") {
		t.Errorf("one argument: %v", err)
	}
}
