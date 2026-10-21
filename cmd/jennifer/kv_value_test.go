// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// kv.setValue / kv.getValue share one parsed document between concurrent
// readers while a writer replaces it: every read sees a whole version (never a
// torn one), and the last write wins. Run under -race in CI, this also pins that
// sharing the immutable tree across goroutines is race-free.
func TestKvValueSharedAcrossTasks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kvshare.j")
	src := `use json;
use kv;
use task;
use testing;
def cache as kv.Store init kv.open();
kv.setValue($cache, "doc", json.decode('{"rev": 0, "items": [1, 2, 3]}'), 0);
def readers as list of task of int;
for (def i in 0..8) {
    $readers[] = spawn {
        def seen as int init 0;
        for (def k in 0..200) {
            def doc as json.Value init kv.getValue($cache, "doc");
            testing.assertEqual(json.length($doc, "/items"), 3);
            $seen = $seen + 1;
        }
        return $seen;
    };
}
def writer as task of null init spawn {
    for (def k in 1..50) {
        kv.setValue($cache, "doc", json.set(kv.getValue($cache, "doc"), "/rev", json.decode("{$k}")), 0);
    }
    return;
};
task.wait($writer);
def total as int init 0;
for (def n in task.waitAll($readers)) {
    $total = $total + $n;
}
testing.assertEqual($total, 8 * 200);
testing.assertEqual(json.asInt(kv.getValue($cache, "doc"), "/rev"), 49);
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := runFile(path, nil, ""); code != 0 {
		t.Fatalf("program exited %d", code)
	}
}
