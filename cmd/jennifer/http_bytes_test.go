// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestHttpGetBytesDownloadsBinary proves the http module's byte path downloads a
// genuinely-binary body (a gzip stream, not valid UTF-8) intact - matching sha256
// and unpacking back - while the text path (http.get) refuses the same body.
func TestHttpGetBytesDownloadsBinary(t *testing.T) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write([]byte("the quick brown fox jumps over the lazy dog, packed into a gzip stream")); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	payload := buf.Bytes()
	sum := sha256.Sum256(payload)
	sumHex := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	httpMod, _ := filepath.Abs(filepath.Join("..", "..", "modules", "http.j"))
	prog := fmt.Sprintf(`use testing;
use hash;
use encoding;
use convert;
use compress;
import %q as http;

# byte-safe download: body arrives intact
def r as http.BytesResponse init http.getBytes(%q, {});
testing.assertEqual($r.status, 200);
testing.assertEqual(len($r.body), %d);
testing.assertEqual(encoding.toText(hash.compute($r.body, "sha256"), "hex"), %q);
testing.assertEqual($r.body[0], 31);    # gzip magic 0x1f
testing.assertEqual($r.body[1], 139);   # gzip magic 0x8b
def back as bytes init compress.unpack($r.body, "gzip");
testing.assertContains(convert.stringFromBytes($back, "utf-8"), "quick brown fox");

# the text path refuses the same non-UTF-8 body (why the byte path exists)
def threw as bool init false;
try { def x as http.Response init http.get(%q, {}); } catch (e) { $threw = true; }
testing.assertEqual($threw, true);`,
		httpMod, srv.URL, len(payload), sumHex, srv.URL)

	dir := t.TempDir()
	progPath := filepath.Join(dir, "getbytes.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("http getBytes binary-download program failed with code %d", code)
	}
}
