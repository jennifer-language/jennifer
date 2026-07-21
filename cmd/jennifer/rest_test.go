// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// A .j program driving the rest module against an in-memory CRUD server asserts
// the whole round-trip: postJson create (201, echoed body), getJson read
// (decoded field), putJson / patchJson update (200), delete (204), a read after
// delete (404 as a value), a query-string GET (the param round-trips), and a
// wrong Bearer token (401 as a value). A mismatch throws and fails loadForTest.
func TestRestCrud(t *testing.T) {
	const token = "Bearer test-token"
	var stored string
	var exists bool
	auth := func(r *http.Request) bool { return r.Header.Get("Authorization") == token }

	mux := http.NewServeMux()
	mux.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
		if !auth(r) {
			http.Error(w, "", http.StatusUnauthorized)
			return
		}
		b, _ := io.ReadAll(r.Body)
		stored, exists = string(b), true
		w.WriteHeader(http.StatusCreated)
		w.Write(b)
	})
	mux.HandleFunc("/users/1", func(w http.ResponseWriter, r *http.Request) {
		if !auth(r) {
			http.Error(w, "", http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodGet:
			if !exists {
				http.Error(w, "", http.StatusNotFound)
				return
			}
			fmt.Fprint(w, stored)
		case http.MethodPut, http.MethodPatch:
			b, _ := io.ReadAll(r.Body)
			stored = string(b)
			fmt.Fprint(w, stored)
		case http.MethodDelete:
			exists = false
			w.WriteHeader(http.StatusNoContent)
		}
	})
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		if !auth(r) {
			http.Error(w, "", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"q":%q}`, r.URL.Query().Get("q"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	restMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "rest.j"))
	if err != nil {
		t.Fatal(err)
	}
	httpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "http.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
use json;
import %q as rest;
import %q as http;
def api as rest.Client init rest.Client{baseUrl: %q, headers: {"Authorization": rest.bearer("test-token")}};
def created as rest.Response init rest.postJson($api, "/users", json.decode("{\"name\":\"ada\"}"));
testing.assertEqual($created.status, 201);
testing.assertContains($created.body, "ada");
def user as json.Value init rest.getJson($api, "/users/1", {});
testing.assertEqual(json.asString($user, "/name"), "ada");
def updated as rest.Response init rest.putJson($api, "/users/1", json.decode("{\"name\":\"ada lovelace\"}"));
testing.assertEqual($updated.status, 200);
def reread as json.Value init rest.getJson($api, "/users/1", {});
testing.assertEqual(json.asString($reread, "/name"), "ada lovelace");
def patched as rest.Response init rest.patchJson($api, "/users/1", json.decode("{\"name\":\"AL\"}"));
testing.assertEqual($patched.status, 200);
def del as rest.Response init rest.delete($api, "/users/1", {});
testing.assertEqual($del.status, 204);
def gone as rest.Response init rest.get($api, "/users/1", {});
testing.assertEqual($gone.status, 404);
def found as json.Value init rest.getJson($api, "/search", {"q": "ada lovelace"});
testing.assertEqual(json.asString($found, "/q"), "ada lovelace");
def bad as rest.Client init rest.Client{baseUrl: %q, headers: {"Authorization": rest.bearer("wrong")}};
def unauth as rest.Response init rest.get($bad, "/search", {});
testing.assertEqual($unauth.status, 401);`, restMod, httpMod, srv.URL, srv.URL)
	progPath := filepath.Join(dir, "crud.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("rest crud program failed with code %d", code)
	}
}
