// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
)

// TestLabelSend drives the label module's `send` (the :9100 raw-print path)
// against an in-process fake printer: the .j program builds a label, renders
// ZPL, and sends it; the fake printer reads the stream off the socket and the
// test asserts the exact bytes arrived. Proves the emit stage over real `net`.
func TestLabelSend(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	got := make(chan string, 1)
	go func() {
		conn, aErr := ln.Accept()
		if aErr != nil {
			got <- ""
			return
		}
		defer conn.Close()
		b, _ := io.ReadAll(conn)
		got <- string(b)
	}()

	labelMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "label.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use io;
import %q as label;
def l as label.Label init label.new(50.0, 30.0);
$l = label.text($l, 5.0, 5.0, label.TextOptions{height: 4.0}, "HELLO");
def zpl as string init label.render($l, label.Device{dialect: "zpl", dpi: 203});
label.send("127.0.0.1", %d, $zpl);`, labelMod, port)
	progPath := filepath.Join(dir, "print.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("label send program failed with code %d", code)
	}

	want := "^XA\n^FO40,40^A0N,32,32^FH^FDHELLO^FS\n^PQ1\n^XZ\n"
	if out := <-got; out != want {
		t.Errorf("printer received %q, want %q", out, want)
	}
}
