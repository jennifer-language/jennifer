// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
)

// A .j program drives rest's pagination walkers and its inherited http policy
// against an in-process server: Link-header pagination (three pages, each
// pointing at the next via a Link: rel="next" absolute URL), cursor pagination
// (a next-cursor field in the body), and a retry (a 503 that then succeeds,
// proving rest.withRetries threads http.send's retry policy).
func TestRestPaginateAndPolicy(t *testing.T) {
	mux := http.NewServeMux()
	// Link-header pages: /items?page=N -> {"page":N}, with a rel="next" until 3.
	mux.HandleFunc("/items", func(w http.ResponseWriter, r *http.Request) {
		page := 1
		if p := r.URL.Query().Get("page"); p != "" {
			page, _ = strconv.Atoi(p)
		}
		if page < 3 {
			next := "http://" + r.Host + "/items?page=" + strconv.Itoa(page+1)
			w.Header().Set("Link", "<"+next+">; rel=\"next\"")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"page":%d}`, page)
	})
	// Cursor pages: /cursor?cursor=C -> {"page":N,"next":"cX"} ("" on the last).
	mux.HandleFunc("/cursor", func(w http.ResponseWriter, r *http.Request) {
		cursor := r.URL.Query().Get("cursor")
		page, next := 1, "c2"
		switch cursor {
		case "c2":
			page, next = 2, "c3"
		case "c3":
			page, next = 3, ""
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"page":%d,"next":%q}`, page, next)
	})
	// Flaky: first call 503, then 200 (proves retry).
	var mu sync.Mutex
	hits := 0
	mux.HandleFunc("/flaky", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		n := hits
		mu.Unlock()
		if n == 1 {
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	restMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "rest.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
use json;
import %q as rest;
def api as rest.Client init rest.client(%q);

# Link-header pagination: three pages, in order
def linkPages as list of json.Value init rest.paginate($api, "/items", {}, 10);
testing.assertEqual(len($linkPages), 3);
testing.assertEqual(json.asInt($linkPages[0], "/page"), 1);
testing.assertEqual(json.asInt($linkPages[2], "/page"), 3);

# cursor pagination: three pages via the "next" field
def curPages as list of json.Value init rest.paginateCursor($api, "/cursor", {}, "/next", "cursor", 10);
testing.assertEqual(len($curPages), 3);
testing.assertEqual(json.asInt($curPages[0], "/page"), 1);
testing.assertEqual(json.asInt($curPages[2], "/page"), 3);

# maxPages caps the walk
def capped as list of json.Value init rest.paginate($api, "/items", {}, 2);
testing.assertEqual(len($capped), 2);

# inherited retry policy: a 503-then-200 succeeds with withRetries
def flakyApi as rest.Client init rest.withBackoff(rest.withRetries($api, 2), 1);
def ok as json.Value init rest.getJson($flakyApi, "/flaky", {});
testing.assertEqual(json.asBool($ok, "/ok"), true);`, restMod, srv.URL)
	progPath := filepath.Join(dir, "paginate.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("rest pagination/policy program failed with code %d", code)
	}
}
