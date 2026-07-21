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

// TestPrometheusQuery drives the prometheus module's retrieval half against an
// in-process fake Prometheus HTTP API: an instant `query` (vector) and a range
// `queryRange` (matrix). Each handler rejects a wrong `query` param, so the
// test also proves the module's URL encoding round-trips the PromQL (the range
// query carries `()[]`). A mismatch throws in the .j program and fails
// loadForTest, so this runs the real HTTP + JSON path in CI with no Prometheus.
func TestPrometheusQuery(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/query", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("query") != "up" {
			http.Error(w, "bad query", http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"__name__":"up","job":"api"},"value":[1700000000,"1"]}]}}`)
	})
	mux.HandleFunc("/api/v1/query_range", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("query") != "rate(x[1m])" {
			http.Error(w, "bad query", http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"job":"api"},"values":[[1700000000,"1"],[1700000015,"2"]]}]}}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	promMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "prometheus.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
import %q as prometheus;
def r as prometheus.Result init prometheus.query(%q, "up");
testing.assertEqual($r.resultType, "vector");
testing.assertEqual(len($r.series), 1);
testing.assertEqual($r.series[0].metric["job"], "api");
testing.assertEqual(len($r.series[0].values), 1);
testing.assertEqual($r.series[0].values[0].value, 1.0);
def rr as prometheus.Result init prometheus.queryRange(%q, "rate(x[1m])", "1700000000", "1700000060", "15s");
testing.assertEqual($rr.resultType, "matrix");
testing.assertEqual(len($rr.series[0].values), 2);
testing.assertEqual($rr.series[0].values[1].value, 2.0);`, promMod, srv.URL, srv.URL)
	progPath := filepath.Join(dir, "query.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("prometheus query program failed with code %d", code)
	}
}
