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

// TestInfluxdbWriteAndQuery drives the influxdb module against an in-process
// fake InfluxDB 1.x HTTP API. The /write handler asserts the db / precision
// params, the Basic-auth header, and the exact line-protocol body the module
// renders; the /query handler asserts the InfluxQL round-trips through the URL
// encoding and returns a tabular result the module parses. A mismatch makes a
// handler answer non-2xx (or the .j asserts fail), so this runs the real
// http + json path in CI with no InfluxDB.
func TestInfluxdbWriteAndQuery(t *testing.T) {
	const wantAuth = "Basic dXNlcjpwYXNz" // base64("user:pass")
	const wantLine = "cpu,host=srv1 value=0.64"

	mux := http.NewServeMux()
	mux.HandleFunc("/write", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != wantAuth ||
			r.URL.Query().Get("db") != "metrics" || r.URL.Query().Get("precision") != "ns" {
			http.Error(w, `{"error":"bad write request"}`, http.StatusBadRequest)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != wantLine {
			http.Error(w, fmt.Sprintf(`{"error":"bad line: %s"}`, body), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/query", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "SELECT value FROM cpu" {
			http.Error(w, `{"error":"bad query"}`, http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `{"results":[{"statement_id":0,"series":[{"name":"cpu","columns":["time","value"],"values":[["2021-01-01T00:00:00Z",0.64]]}]}]}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	influxMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "influxdb.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
import %q as influxdb;
def c as influxdb.Client init influxdb.clientWith(%q, "metrics", "user", "pass");
def p as influxdb.Point init influxdb.field(influxdb.tag(influxdb.point("cpu"), "host", "srv1"), "value", 0.64);
influxdb.write($c, [$p]);
def r as influxdb.Result init influxdb.query($c, "SELECT value FROM cpu");
testing.assertEqual(len($r.series), 1);
testing.assertEqual($r.series[0].name, "cpu");
testing.assertEqual($r.series[0].values[0][0], "2021-01-01T00:00:00Z");
testing.assertEqual($r.series[0].values[0][1], "0.64");`, influxMod, srv.URL)
	progPath := filepath.Join(dir, "influx.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("influxdb write/query program failed with code %d", code)
	}
}
