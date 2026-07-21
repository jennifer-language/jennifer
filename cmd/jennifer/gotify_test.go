// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// A .j program driving the gotify module against a fake Gotify server asserts
// the push contract: a POST to /message with the X-Gotify-Key header and a
// form-encoded title / message / priority (the server parses and echoes them,
// so the encoding round-trips), a 200 on the right token, and a 401 surfaced as
// a value (not a crash) on a bad one. A mismatch throws and fails loadForTest.
func TestGotifyPush(t *testing.T) {
	const token = "secret-token"
	mux := http.NewServeMux()
	mux.HandleFunc("/message", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("X-Gotify-Key") != token {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"title":%q,"message":%q,"priority":%s}`,
			r.FormValue("title"), r.FormValue("message"), r.FormValue("priority"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	gotifyMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "gotify.j"))
	if err != nil {
		t.Fatal(err)
	}
	httpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "http.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
import %q as gotify;
import %q as http;
def cfg as gotify.Config init gotify.Config{url: %q, token: %q};
def r as http.Response init gotify.push($cfg, "Deploy", "build 1234 & up", 5);
testing.assertEqual($r.status, 200);
testing.assertContains($r.body, "Deploy");
testing.assertContains($r.body, "build 1234 & up");
testing.assertContains($r.body, "\"priority\":5");
def bad as gotify.Config init gotify.Config{url: %q, token: "wrong"};
def br as http.Response init gotify.push($bad, "X", "Y", 1);
testing.assertEqual($br.status, 401);`, gotifyMod, httpMod, srv.URL, token, srv.URL)
	progPath := filepath.Join(dir, "push.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("gotify push program failed with code %d", code)
	}
}
