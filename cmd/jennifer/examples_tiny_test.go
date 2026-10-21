// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// tinyExampleSkips lists the golden examples whose output legitimately differs
// under jennifer-tiny, with the reason. Everything else must match byte for byte.
var tinyExampleSkips = map[string]string{
	"net.j":      "needs the network stack, which jennifer-tiny does not include",
	"showcase.j": "its net section prints the jennifer-tiny fallback line instead of the echo reply",
}

// TestExamplesTinyBinary runs every golden example through a built jennifer-tiny
// binary and compares its stdout with examples/expected/, the same goldens
// TestExamples checks in-process. TinyGo's test runner cannot run this package,
// so the binary is built separately (`make build-tinygo`) and named by
// JENNIFER_TINY_BIN; without it the test skips, so a plain `go test ./...` needs
// no TinyGo. CI sets it, which is what catches a TinyGo-only divergence (an
// unimplemented runtime feature, a libm last-ulp difference in a golden).
func TestExamplesTinyBinary(t *testing.T) {
	bin := os.Getenv("JENNIFER_TINY_BIN")
	if bin == "" {
		t.Skip("JENNIFER_TINY_BIN not set; build jennifer-tiny and point it there to run the goldens under TinyGo")
	}
	bin, err := filepath.Abs(bin)
	if err != nil {
		t.Fatalf("resolve JENNIFER_TINY_BIN: %v", err)
	}
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("JENNIFER_TINY_BIN=%s: %v", bin, err)
	}
	root, err := filepath.Abs(filepath.Join("..", "..", "examples"))
	if err != nil {
		t.Fatalf("locate examples: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read examples dir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".j") {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			if reason, ok := tinyExampleSkips[name]; ok {
				t.Skip(reason)
			}
			expected, err := os.ReadFile(filepath.Join(root, "expected", strings.TrimSuffix(name, ".j")+".txt"))
			if err != nil {
				t.Skipf("no expected file: %v", err)
			}
			var stdout, stderr bytes.Buffer
			cmd := exec.Command(bin, "run", filepath.Join(root, name))
			cmd.Dir = root
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("jennifer-tiny run %s: %v\n--- stderr ---\n%s", name, err, stderr.String())
			}
			if stdout.String() != string(expected) {
				t.Errorf("%s output mismatch under jennifer-tiny:\n--- got ---\n%s\n--- want ---\n%s", name, stdout.String(), string(expected))
			}
		})
	}
}
