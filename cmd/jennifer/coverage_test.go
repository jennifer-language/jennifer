// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/parser"
	"jennifer-lang.dev/jennifer/internal/profile"
)

// renderCoverage over a fully-hit program is 100%, over an unhit one is 0%, and
// the JSON form parses with a total equal to the statement count.
func TestCoverageRenderFullAndEmpty(t *testing.T) {
	prog, err := parser.Parse(`func a() { return 1; }
func testA() { def x as int init a(); }`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	all, _ := statementPositions(prog)
	if len(all) == 0 {
		t.Fatal("no statement positions enumerated")
	}
	full := map[profile.Position]int64{}
	for p := range all {
		full[p] = 1
	}
	if out, _ := renderCoverage(prog, full, "text"); !strings.Contains(out, "100.0%") {
		t.Errorf("full coverage should be 100%%:\n%s", out)
	}
	if out, _ := renderCoverage(prog, map[profile.Position]int64{}, "text"); !strings.Contains(out, "0.0%") {
		t.Errorf("empty coverage should be 0%%:\n%s", out)
	}
	j, err := renderCoverage(prog, full, "json")
	if err != nil {
		t.Fatalf("json render: %v", err)
	}
	var parsed struct {
		Covered, Total int
		Percent        float64
		Files          []struct {
			Uncovered []profile.Position
		}
	}
	if err := json.Unmarshal([]byte(j), &parsed); err != nil {
		t.Fatalf("json does not parse: %v\n%s", err, j)
	}
	if parsed.Total != len(all) || parsed.Covered != len(all) || parsed.Percent != 100 {
		t.Errorf("json totals wrong: %+v (want total %d)", parsed, len(all))
	}
}

// captureStdout runs f with os.Stdout redirected to a pipe and returns what it
// wrote.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		done <- buf.String()
	}()
	f()
	w.Close()
	os.Stdout = old
	return <-done
}

// End to end: `jennifer test --coverage` over a file whose tests exercise some
// but not all methods reports below 100% and names the unexecuted positions,
// and the plain run emits no coverage section.
func TestCoverageCLIPartial(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cov_test.j")
	if err := os.WriteFile(path, []byte(`use testing;
func add(a as int, b as int) { return $a + $b; }
func unused() { return 9; }
func testAdd() { testing.assertEqual(add(2, 3), 5); }`), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() { runTest([]string{"--coverage", path}) })
	if !strings.Contains(out, "Coverage (statements):") {
		t.Errorf("missing coverage section:\n%s", out)
	}
	if !strings.Contains(out, "uncovered:") {
		t.Errorf("missing uncovered list (unused() never ran):\n%s", out)
	}
	if strings.Contains(out, "100.0%") {
		t.Errorf("coverage should be below 100%% (unused not called):\n%s", out)
	}

	// Plain run: no coverage section, byte-for-byte the normal report.
	plain := captureStdout(t, func() { runTest([]string{path}) })
	if strings.Contains(plain, "Coverage") {
		t.Errorf("plain run must not emit coverage:\n%s", plain)
	}
}

// `--coverage=json` puts parseable JSON on stdout (the human report moves to
// stderr).
func TestCoverageCLIJSONOwnsStdout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "j_test.j")
	if err := os.WriteFile(path, []byte(`use testing;
func add(a as int, b as int) { return $a + $b; }
func testAdd() { testing.assertEqual(add(2, 3), 5); }`), 0o644); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() { runTest([]string{"--coverage=json", path}) })
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("stdout is not pure JSON: %v\n%s", err, out)
	}
	if _, ok := parsed["files"]; !ok {
		t.Errorf("json missing files key: %s", out)
	}
}
