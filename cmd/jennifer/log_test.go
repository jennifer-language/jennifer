// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLogFileSink drives the log module's file sink: it appends records to a
// file, and level filtering drops records below the logger's level. The Go test
// reads the file back and checks structure.
func TestLogFileSink(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")
	logMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "log.j"))
	if err != nil {
		t.Fatal(err)
	}
	prog := fmt.Sprintf(`import %q as log;
def lg as log.Logger init log.toFile("info", "logfmt", %q);
def f as map of string to string init {"user": "ada", "id": "42"};
def empty as map of string to string init {};
log.info($lg, "hello world", $f);
log.debug($lg, "should be dropped", $empty);
log.error($lg, "boom", $empty);`, logMod, logFile)
	progPath := filepath.Join(dir, "run.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("log file program failed with code %d", code)
	}

	body, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	out := string(body)
	for _, want := range []string{
		`level=info msg="hello world" user=ada id=42`,
		`level=error msg=boom`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log file missing %q; got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "should be dropped") {
		t.Errorf("debug record was not filtered out below info level:\n%s", out)
	}
	if lines := strings.Count(strings.TrimSpace(out), "\n"); lines != 1 { // 2 records -> 1 newline between
		t.Errorf("expected 2 records, got %d lines:\n%s", lines+1, out)
	}
}

// TestLogSyslog drives the RFC 5424 syslog sink against a fake UDP server and
// checks the datagram framing (priority, version, app, message, fields).
func TestLogSyslog(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	addr := pc.LocalAddr().String()

	logMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "log.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`import %q as log;
def lg as log.Logger init log.toSyslog("info", %q, "myapp");
def f as map of string to string init {"k": "v"};
log.warn($lg, "hi there", $f);`, logMod, addr)
	progPath := filepath.Join(dir, "run.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	got := make(chan string, 1)
	go func() {
		buf := make([]byte, 2048)
		n, _, _ := pc.ReadFrom(buf)
		got <- string(buf[:n])
	}()

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("log syslog program failed with code %d", code)
	}

	select {
	case line := <-got:
		// PRI = facility 1 * 8 + severity 4 (warning) = 12; version 1.
		if !strings.HasPrefix(line, "<12>1 ") {
			t.Errorf("syslog datagram bad prefix: %q", line)
		}
		if !strings.Contains(line, " myapp - - - hi there k=v") {
			t.Errorf("syslog datagram missing app / msg / fields: %q", line)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no syslog datagram received")
	}
}
