// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build linux && !tinygo

package seriallib_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lib/convert"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	seriallib "jennifer-lang.dev/jennifer/internal/lib/serial"
	tasklib "jennifer-lang.dev/jennifer/internal/lib/task"
	timelib "jennifer-lang.dev/jennifer/internal/lib/time"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// runSerialTasks is runSerial with the task and time libraries, for tests that
// spawn readers and cancel them.
func runSerialTasks(t *testing.T, src string) (string, error) {
	t.Helper()
	seriallib.ResetForTest()
	prog, err := parser.Parse(src)
	if err != nil {
		return "", err
	}
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	convert.Install(in)
	tasklib.Install(in)
	timelib.Install(in)
	seriallib.Install(in)
	rerr := in.Run(prog)
	return buf.String(), rerr
}

// TestReadDeadline: a read past serial.setReadDeadline is a catchable
// Error{kind: "timeout"} that returns on time; the deadline stays until
// cleared with 0, and a read after clearing gets the bytes that came.
func TestReadDeadline(t *testing.T) {
	master, slave := openPTY(t)
	defer master.Close()
	go func() {
		time.Sleep(600 * time.Millisecond)
		_, _ = master.Write([]byte("Z"))
	}()
	start := time.Now()
	out, err := runSerialTasks(t, fmt.Sprintf(`use serial; use io; use convert; use time;
def p as serial.Port init serial.open(%q, 9600);
serial.setReadDeadline($p, 150);
def t0 as time.Time init time.now();
try { serial.read($p, 4); } catch (e) { io.printf("%%s %%s;", $e.kind, $e.message); }
def took as int init time.milliseconds(time.sub(time.now(), $t0));
io.printf("%%v;", $took < 1000);
try { serial.read($p, 4); } catch (e) { io.printf("%%s;", $e.kind); }
serial.setReadDeadline($p, 0);
io.printf("[%%s]", convert.stringFromBytes("utf-8", serial.read($p, 4)));
serial.close($p);`, slave))
	if err != nil {
		t.Fatalf("run: %v (out=%q)", err, out)
	}
	want := "timeout serial.read: read timed out;true;timeout;[Z]"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("took %v", time.Since(start))
	}
}

// TestDeadlineArguments: a negative ms is refused; a write deadline does not
// touch reads and the write still goes out.
func TestDeadlineArguments(t *testing.T) {
	master, slave := openPTY(t)
	defer master.Close()
	out, err := runSerialTasks(t, fmt.Sprintf(`use serial; use io; use convert;
def p as serial.Port init serial.open(%q, 9600);
try { serial.setReadDeadline($p, -1); } catch (e) { io.printf("%%s;", $e.message); }
serial.setWriteDeadline($p, 1000);
io.printf("%%d;", serial.write($p, convert.bytesFromString("utf-8", "ok")));
serial.setWriteDeadline($p, 0);
serial.close($p);
try { serial.setReadDeadline($p, 10); } catch (e) { io.printf("closed handle"); }`, slave))
	if err != nil {
		t.Fatalf("run: %v (out=%q)", err, out)
	}
	if !strings.Contains(out, "serial.setReadDeadline: ms must be 0 (clear) or positive, got -1;") ||
		!strings.Contains(out, "2;") || !strings.HasSuffix(out, "closed handle") {
		t.Errorf("got %q", out)
	}
}

// TestCancelWakesRead: task.cancel of a task blocked in serial.read wakes it
// with "task cancelled" at once, and the port keeps working (the user's read
// deadline is restored, none here).
func TestCancelWakesRead(t *testing.T) {
	master, slave := openPTY(t)
	defer master.Close()
	go func() {
		time.Sleep(900 * time.Millisecond)
		_, _ = master.Write([]byte("A"))
	}()
	out, err := runSerialTasks(t, fmt.Sprintf(`use serial; use io; use convert; use task; use time;
def p as serial.Port init serial.open(%q, 9600);
def reader as task of string init spawn {
    try { serial.read($p, 4); } catch (e) { return $e.message; }
    return "read returned";
};
time.sleep(time.fromMilliseconds(100));
def t0 as time.Time init time.now();
task.cancel($reader);
io.printf("%%s;", task.wait($reader));
io.printf("%%v;", time.milliseconds(time.sub(time.now(), $t0)) < 500);
io.printf("[%%s]", convert.stringFromBytes("utf-8", serial.read($p, 4)));
serial.close($p);`, slave))
	if err != nil {
		t.Fatalf("run: %v (out=%q)", err, out)
	}
	if out != "task cancelled;true;[A]" {
		t.Errorf("got %q, want %q", out, "task cancelled;true;[A]")
	}
}

// TestCloseIsKindClosed: a read blocked when another task closes the port
// fails with Error{kind: "closed"}.
func TestCloseIsKindClosed(t *testing.T) {
	master, slave := openPTY(t)
	defer master.Close()
	out, err := runSerialTasks(t, fmt.Sprintf(`use serial; use io; use task; use time;
def p as serial.Port init serial.open(%q, 9600);
def reader as task of string init spawn {
    try { serial.read($p, 4); } catch (e) { return $e.kind + " " + $e.message; }
    return "read returned";
};
time.sleep(time.fromMilliseconds(100));
serial.close($p);
io.printf("%%s", task.wait($reader));`, slave))
	if err != nil {
		t.Fatalf("run: %v (out=%q)", err, out)
	}
	if out != "closed serial.read: read on a closed port" {
		t.Errorf("got %q", out)
	}
}
