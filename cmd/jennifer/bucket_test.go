// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sigV4Expected re-derives the SigV4 signature for a received request the way an
// S3 server would, from the exact host / date / path / body on the wire. It is
// an independent implementation of the module's signing, so a match proves the
// module signed what it actually sent (host coupling included), end to end.
func sigV4Expected(r *http.Request, body []byte, region, secret string) string {
	hsh := func(key []byte, msg string) []byte {
		m := hmac.New(sha256.New, key)
		m.Write([]byte(msg))
		return m.Sum(nil)
	}
	amzDate := r.Header.Get("x-amz-date")
	shortDate := amzDate[:8]
	payloadHash := r.Header.Get("x-amz-content-sha256")
	canonicalHeaders := "host:" + r.Host + "\nx-amz-content-sha256:" + payloadHash + "\nx-amz-date:" + amzDate + "\n"
	signed := "host;x-amz-content-sha256;x-amz-date"
	canonicalRequest := r.Method + "\n" + r.URL.EscapedPath() + "\n" + r.URL.RawQuery + "\n" +
		canonicalHeaders + "\n" + signed + "\n" + payloadHash
	creqHash := sha256.Sum256([]byte(canonicalRequest))
	scope := shortDate + "/" + region + "/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(creqHash[:])
	kSign := hsh(hsh(hsh(hsh([]byte("AWS4"+secret), shortDate), region), "s3"), "aws4_request")
	return hex.EncodeToString(hsh(kSign, stringToSign))
}

// TestBucketRequests drives the bucket module's get / put / delete / listObjects
// against an S3-shaped server that re-derives and checks the SigV4 signature over
// the wire (a 403 on mismatch), also confirming x-amz-content-sha256 equals the
// body's hash. A signing or transport bug throws in the .j program and fails
// loadForTest.
func TestBucketRequests(t *testing.T) {
	const region = "us-east-1"
	const accessKey = "AKIDEXAMPLE"
	const secret = "test-secret-key"
	const objectBody = "the object body"

	check := func(w http.ResponseWriter, r *http.Request) bool {
		body, _ := io.ReadAll(r.Body)
		sum := sha256.Sum256(body)
		if r.Header.Get("x-amz-content-sha256") != hex.EncodeToString(sum[:]) {
			http.Error(w, "payload-hash", http.StatusBadRequest)
			return false
		}
		auth := r.Header.Get("Authorization")
		want := "Signature=" + sigV4Expected(r, body, region, secret)
		if !strings.Contains(auth, want) {
			http.Error(w, "signature", http.StatusForbidden)
			return false
		}
		return true
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/mybucket/hello.txt", func(w http.ResponseWriter, r *http.Request) {
		if !check(w, r) {
			return
		}
		switch r.Method {
		case http.MethodPut:
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			fmt.Fprint(w, objectBody)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		}
	})
	mux.HandleFunc("/mybucket", func(w http.ResponseWriter, r *http.Request) {
		if !check(w, r) {
			return
		}
		fmt.Fprint(w, `<?xml version="1.0"?><ListBucketResult><Contents><Key>hello.txt</Key></Contents><Contents><Key>docs/readme.md</Key></Contents></ListBucketResult>`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	bucketMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "bucket.j"))
	if err != nil {
		t.Fatal(err)
	}
	httpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "http.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
import %q as bucket;
import %q as http;
def c as bucket.Client init bucket.connect(%q, %q, %q, %q);
def pu as http.Response init bucket.put($c, "mybucket", "hello.txt", %q);
testing.assertEqual($pu.status, 200);
def g as http.Response init bucket.get($c, "mybucket", "hello.txt");
testing.assertEqual($g.status, 200);
testing.assertEqual($g.body, %q);
def d as http.Response init bucket.delete($c, "mybucket", "hello.txt");
testing.assertEqual($d.status, 204);
def l as http.Response init bucket.listObjects($c, "mybucket");
testing.assertEqual($l.status, 200);
def keys as list of string init bucket.objectKeys($l.body);
testing.assertEqual(len($keys), 2);
testing.assertEqual($keys[0], "hello.txt");
testing.assertEqual($keys[1], "docs/readme.md");`,
		bucketMod, httpMod, srv.URL, region, accessKey, secret, objectBody, objectBody)
	progPath := filepath.Join(dir, "bucket.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("bucket program failed with code %d", code)
	}
}
