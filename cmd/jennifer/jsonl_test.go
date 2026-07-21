// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestJsonlFileAndStreaming drives the jsonl module's fs-backed surface
// (writeFile / appendFile / readFile and the streaming Reader) against a real
// temp file. The in-memory encode / decode are covered by the overlay
// (modules/jsonl_test.j); this exercises the parts that need `fs`.
func TestJsonlFileAndStreaming(t *testing.T) {
	dir := t.TempDir()
	dataFile := filepath.Join(dir, "data.jsonl")
	blankFile := filepath.Join(dir, "blank.jsonl")
	jsonlMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "jsonl.j"))
	if err != nil {
		t.Fatal(err)
	}
	prog := fmt.Sprintf(`import %q as jsonl;
use json;
use testing;
use fs;
def const PATH as string init %q;
def const BLANKPATH as string init %q;

func testWriteReadAppend() {
    jsonl.writeFile(PATH, [json.decode("{\"a\":1}"), json.decode("{\"a\":2}")]);
    jsonl.appendFile(PATH, [json.decode("{\"a\":3}")]);
    def all as list of json.Value init jsonl.readFile(PATH);
    testing.assertEqual(len($all), 3);
    testing.assertEqual(json.asInt(json.get($all[0], "/a")), 1);
    testing.assertEqual(json.asInt(json.get($all[2], "/a")), 3);
}

func testStreaming() {
    def r as jsonl.Reader init jsonl.openReader(PATH);
    def n as int init 0;
    def sum as int init 0;
    while (true) {
        def rec as jsonl.Record init jsonl.readRecord($r);
        if ($rec.done) {
            break;
        }
        $n = $n + 1;
        $sum = $sum + json.asInt(json.get($rec.value, "/a"));
    }
    jsonl.closeReader($r);
    testing.assertEqual($n, 3);
    testing.assertEqual($sum, 6);
}

func testReadRecordDoneAtEnd() {
    def r as jsonl.Reader init jsonl.openReader(PATH);
    while (not jsonl.readRecord($r).done) {
    }
    # Past the end, readRecord keeps returning done (no throw, no phantom).
    testing.assertTrue(jsonl.readRecord($r).done);
    jsonl.closeReader($r);
}

# A file ending in extra blank lines must not over-run the last record: a
# hasMore-guarded loop would skip the trailing blanks then throw / phantom, but
# looping on .done stops cleanly with the right count.
func testTrailingBlankLines() {
    jsonl.writeFile(BLANKPATH, [json.decode("{\"a\":10}"), json.decode("{\"a\":20}")]);
    fs.appendString(BLANKPATH, "\n\n  \n");
    def r as jsonl.Reader init jsonl.openReader(BLANKPATH);
    def n as int init 0;
    def sum as int init 0;
    while (true) {
        def rec as jsonl.Record init jsonl.readRecord($r);
        if ($rec.done) {
            break;
        }
        $n = $n + 1;
        $sum = $sum + json.asInt(json.get($rec.value, "/a"));
    }
    jsonl.closeReader($r);
    testing.assertEqual($n, 2);
    testing.assertEqual($sum, 30);
}
`, jsonlMod, dataFile, blankFile)
	progPath := filepath.Join(dir, "prog.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	in, code := loadForTest(progPath)
	if in == nil || code != testExitPass {
		t.Fatalf("loadForTest failed: code %d", code)
	}
	// testWriteReadAppend must run first (it creates the file the others read).
	for _, name := range []string{"testWriteReadAppend", "testStreaming", "testReadRecordDoneAtEnd", "testTrailingBlankLines"} {
		if _, err := in.CallByName(name); err != nil {
			t.Errorf("%s failed: %v", name, err)
		}
	}

	// Independently confirm the file is exactly three JSON lines.
	body, err := os.ReadFile(dataFile)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(body), "\n"), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 JSONL lines, got %d: %q", len(lines), string(body))
	}
	for i, ln := range lines {
		if !strings.HasPrefix(ln, "{") || !strings.HasSuffix(ln, "}") {
			t.Errorf("line %d is not a compact JSON object: %q", i, ln)
		}
	}
}
