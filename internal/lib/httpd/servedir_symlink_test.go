// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo && unix

package httpdlib

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// M29.52: serveDir keeps a symlink's target inside root unless the server set
// httpd.Options.followSymlinks.
func TestServeDirSymlinks(t *testing.T) {
	ResetForTest()
	base := t.TempDir()
	root := filepath.Join(base, "root")
	outside := filepath.Join(base, "outside")
	write := func(p, s string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	link := func(target, name string) {
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root, "real.txt"), "inside")
	write(filepath.Join(root, "sub", "index.html"), "sub index")
	write(filepath.Join(outside, "secret.txt"), "secret")
	write(filepath.Join(outside, "index.html"), "outside index")
	write(filepath.Join(root, "leaky", "a.txt"), "a")
	link("real.txt", "rel.txt")                                    // relative, inside
	link(filepath.Join(root, "real.txt"), "abs.txt")               // absolute, inside
	link("sub", "subdir")                                          // directory, inside
	link("../outside/secret.txt", "out.txt")                       // relative, outside
	link(filepath.Join(outside, "secret.txt"), "absout.txt")       // absolute, outside
	link(outside, "outdir")                                        // directory, outside
	link(filepath.Join(outside, "index.html"), "leaky/index.html") // an index outside
	link("missing.txt", "dangling.txt")                            // dangling

	serve := func(opts Value, etag bool) string {
		srv, err := listenWithFn(noCtx, []Value{interpreter.StringVal("127.0.0.1:0"), opts})
		if err != nil {
			t.Fatalf("listenWith: %v", err)
		}
		t.Cleanup(func() { shutdownFn(noCtx, []Value{srv}) })
		serveLoop(srv, func(req Value) {
			if etag {
				_, _ = serveDirEtagFn(noCtx, []Value{req, interpreter.StringVal(root)})
			} else {
				_, _ = serveDirFn(noCtx, []Value{req, interpreter.StringVal(root)})
			}
		})
		addrV, _ := addressFn(noCtx, []Value{srv})
		return addrV.Str()
	}
	get := func(addr, p string) (int, string, string) {
		resp, err := http.Get("http://" + addr + p)
		if err != nil {
			t.Fatalf("GET %s: %v", p, err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b), resp.Header.Get("ETag")
	}
	type want struct {
		status int
		body   string
	}
	contained := map[string]want{
		"/real.txt":          {200, "inside"},
		"/rel.txt":           {200, "inside"},
		"/abs.txt":           {200, "inside"},
		"/subdir/":           {200, "sub index"},
		"/subdir/index.html": {301, ""},
		"/out.txt":           {404, ""},
		"/absout.txt":        {404, ""},
		"/outdir/secret.txt": {404, ""},
		"/outdir/":           {404, ""},
		"/leaky/":            {404, ""},
		"/leaky/index.html":  {404, ""},
		"/dangling.txt":      {404, ""},
		"/missing.txt":       {404, ""},
	}
	followed := map[string]want{
		"/real.txt":          {200, "inside"},
		"/abs.txt":           {200, "inside"},
		"/out.txt":           {200, "secret"},
		"/absout.txt":        {200, "secret"},
		"/outdir/secret.txt": {200, "secret"},
		"/outdir/":           {200, "outside index"},
		"/leaky/":            {200, "outside index"},
	}
	follow := interpreter.NamespacedStructVal(LibraryName, "Options", []interpreter.StructField{
		{Name: "followSymlinks", Value: interpreter.BoolVal(true)},
	})
	listing := interpreter.NamespacedStructVal(LibraryName, "Options", []interpreter.StructField{
		{Name: "dirListing", Value: interpreter.BoolVal(true)},
	})
	noRedirect := http.DefaultClient.CheckRedirect
	http.DefaultClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	defer func() { http.DefaultClient.CheckRedirect = noRedirect }()
	for _, tc := range []struct {
		name  string
		opts  Value
		etag  bool
		cases map[string]want
	}{
		{"default", optionsVal(0, 0), false, contained},
		{"default etag", optionsVal(0, 0), true, contained},
		{"followSymlinks", follow, false, followed},
		{"followSymlinks etag", follow, true, followed},
	} {
		addr := serve(tc.opts, tc.etag)
		for p, w := range tc.cases {
			status, body, etag := get(addr, p)
			if status != w.status || (w.body != "" && body != w.body) {
				t.Errorf("%s: GET %s = %d %q, want %d %q", tc.name, p, status, body, w.status, w.body)
			}
			if tc.etag && status == 200 && !strings.HasSuffix(p, "/") && etag != sha256Tag([]byte(w.body)) {
				t.Errorf("%s: GET %s ETag = %q, want the content's", tc.name, p, etag)
			}
			if status == 404 && etag != "" {
				t.Errorf("%s: GET %s: a 404 carries ETag %q", tc.name, p, etag)
			}
		}
	}
	// With dirListing, a directory whose index.html lies outside is listed, not
	// answered with the outside index; an outside directory is still not found.
	addr := serve(listing, false)
	if status, body, _ := get(addr, "/leaky/"); status != 200 || body == "outside index" {
		t.Errorf("dirListing: GET /leaky/ = %d %q, want a listing", status, body)
	}
	if status, _, _ := get(addr, "/outdir/"); status != 404 {
		t.Errorf("dirListing: GET /outdir/ = %d, want 404", status)
	}
}
