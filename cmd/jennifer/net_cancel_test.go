// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestNetReadEndsOnCancel: task.cancel ends a read blocked on a peer that
// trickles a byte now and then (so the idle deadline never fires), for each
// read path - readBytes, readN (a Content-Length body) and readAll (a body to
// EOF) - and the task's socket is closed, not left to the peer.
func TestNetReadEndsOnCancel(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	var mu sync.Mutex
	closed := 0
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 4096)
				n, _ := c.Read(buf)
				head := "HTTP/1.1 200 OK\r\nContent-Length: 1000\r\n\r\n"
				if strings.Contains(string(buf[:n]), "/eof") {
					head = "HTTP/1.1 200 OK\r\nConnection: close\r\n\r\n"
				}
				if strings.Contains(string(buf[:n]), "/raw") {
					head = ""
				}
				c.Write([]byte(head))
				for i := 0; i < 200; i++ {
					time.Sleep(100 * time.Millisecond)
					if _, err := c.Write([]byte("x")); err != nil {
						mu.Lock()
						closed++
						mu.Unlock()
						return
					}
				}
			}(c)
		}
	}()
	httpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "http.j"))
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	prog := fmt.Sprintf(`use testing;
use task;
use time;
use net;
use convert;
import %q as http;

func fetch(url as string) {
    return http.requestWith("GET", $url, {}, "", 5000, 0).status;
}

func readRaw(addr as string) {
    def c as net.Conn init net.connect($addr, 5000);
    defer net.close($c);
    net.writeBytes($c, convert.bytesFromString("utf-8", "GET /raw HTTP/1.1\r\n\r\n"));
    while (true) {
        net.readBytes($c, 100000);
    }
    return 0;
}

func cancelled(t as task of int) {
    time.sleep(time.fromMilliseconds(400));
    task.cancel($t);
    def msg as string init "";
    try {
        task.waitTimeout($t, 2000);
    } catch (e) {
        $msg = $e.kind + ": " + $e.message;
    }
    return $msg;
}

for (def path in ["/length", "/eof"]) {
    def url as string init "http://%s" + $path;
    def t as task of int init spawn { return fetch($url); };
    testing.assertContains(cancelled($t), "task cancelled");
}
def addr as string init %q;
def r as task of int init spawn { return readRaw($addr); };
testing.assertContains(cancelled($r), "task cancelled");
`, httpMod, addr, addr)
	path := filepath.Join(t.TempDir(), "cancel.j")
	if err := os.WriteFile(path, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, code := loadForTest(path); code != testExitPass {
		t.Fatalf("cancel.j failed with code %d", code)
	}
	if d := time.Since(start); d > 6*time.Second {
		t.Errorf("the cancelled reads took %v", d)
	}
	// each closed socket shows at the peer as a failed write
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := closed
		mu.Unlock()
		if n == 3 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Errorf("%d of 3 connections closed after the cancel", closed)
}

// TestCancelReachesModuleCode: task.cancel stops a spawned task while it runs
// in an imported module's function - a sleep and a loop there - not only in
// the program's own code.
func TestCancelReachesModuleCode(t *testing.T) {
	dir := t.TempDir()
	mod := `use time;
export func nap() {
    time.sleep(time.fromSeconds(5));
    return 1;
}
export func spin() {
    def n as int init 0;
    while (true) {
        $n = $n + 1;
    }
    return $n;
}
`
	prog := `use testing;
use task;
use time;
import "./m.j" as m;
def a as task of int init spawn { return m.nap(); };
def b as task of int init spawn { return m.spin(); };
time.sleep(time.fromMilliseconds(100));
for (def t in [$a, $b]) {
    task.cancel($t);
    def msg as string init "";
    try {
        task.waitTimeout($t, 2000);
    } catch (e) {
        $msg = $e.message;
    }
    testing.assertContains($msg, "task cancelled");
}
`
	if err := os.WriteFile(filepath.Join(dir, "m.j"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "main.j")
	if err := os.WriteFile(path, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(path); code != testExitPass {
		t.Fatalf("main.j failed with code %d", code)
	}
}
