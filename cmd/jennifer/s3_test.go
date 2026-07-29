// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

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

// signedHeaderList pulls the "SignedHeaders=a;b;c" list out of an Authorization
// header value, so the verifier rebuilds the canonical headers from exactly the
// set the module signed (host + content-type + x-amz-* metadata + copy-source).
func signedHeaderList(auth string) []string {
	const marker = "SignedHeaders="
	i := strings.Index(auth, marker)
	if i < 0 {
		return nil
	}
	rest := auth[i+len(marker):]
	if c := strings.Index(rest, ","); c >= 0 {
		rest = rest[:c]
	}
	return strings.Split(rest, ";")
}

// sigV4Expected re-derives the SigV4 signature for a received request the way an
// S3 server would, from the exact host / date / path / query / body / signed
// headers on the wire. It is an independent implementation of the module's
// signing, so a match proves the module signed what it actually sent (host
// coupling, metadata, copy-source, and query included), end to end.
func sigV4Expected(r *http.Request, region, secret string) string {
	hsh := func(key []byte, msg string) []byte {
		m := hmac.New(sha256.New, key)
		m.Write([]byte(msg))
		return m.Sum(nil)
	}
	amzDate := r.Header.Get("x-amz-date")
	shortDate := amzDate[:8]
	payloadHash := r.Header.Get("x-amz-content-sha256")
	// SigV4 canonical header values collapse each run of spaces to one.
	collapse := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	signed := signedHeaderList(r.Header.Get("Authorization"))
	var ch strings.Builder
	for _, n := range signed {
		if n == "host" {
			ch.WriteString("host:" + r.Host + "\n")
		} else {
			ch.WriteString(n + ":" + collapse(r.Header.Get(n)) + "\n")
		}
	}
	canonicalRequest := r.Method + "\n" + r.URL.EscapedPath() + "\n" + r.URL.RawQuery + "\n" +
		ch.String() + "\n" + strings.Join(signed, ";") + "\n" + payloadHash
	creqHash := sha256.Sum256([]byte(canonicalRequest))
	scope := shortDate + "/" + region + "/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(creqHash[:])
	kSign := hsh(hsh(hsh(hsh([]byte("AWS4"+secret), shortDate), region), "s3"), "aws4_request")
	return hex.EncodeToString(hsh(kSign, stringToSign))
}

// TestS3Requests drives the s3 module's get / put / delete / listObjects plus the
// M23.6 additions (putBytes / getBytes / putWith metadata / head / copy /
// multipart) against an S3-shaped server that re-derives and checks the SigV4
// signature over the wire (a 403 on mismatch), also confirming
// x-amz-content-sha256 equals the body's hash. A signing or transport bug throws
// in the .j program and fails loadForTest.
func TestS3Requests(t *testing.T) {
	const region = "us-east-1"
	const accessKey = "AKIDEXAMPLE"
	const secret = "test-secret-key"
	const objectBody = "the object body"

	check := func(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
		body, _ := io.ReadAll(r.Body)
		sum := sha256.Sum256(body)
		if r.Header.Get("x-amz-content-sha256") != hex.EncodeToString(sum[:]) {
			http.Error(w, "payload-hash", http.StatusBadRequest)
			return nil, false
		}
		auth := r.Header.Get("Authorization")
		want := "Signature=" + sigV4Expected(r, region, secret)
		if !strings.Contains(auth, want) {
			http.Error(w, "signature", http.StatusForbidden)
			return nil, false
		}
		return body, true
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/mybucket/hello.txt", func(w http.ResponseWriter, r *http.Request) {
		body, ok := check(w, r)
		if !ok {
			return
		}
		switch r.Method {
		case http.MethodPut:
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			fmt.Fprint(w, objectBody)
		case http.MethodHead:
			// Real S3 returns the object's stored metadata on HEAD; the mock is
			// stateless, so it returns a fixed value the .j side reads back.
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("x-amz-meta-team", "eng")
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		}
		_ = body
	})
	mux.HandleFunc("/mybucket/bin.dat", func(w http.ResponseWriter, r *http.Request) {
		body, ok := check(w, r)
		if !ok {
			return
		}
		switch r.Method {
		case http.MethodPut:
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			// Return the exact bytes that were PUT so the .j side proves the
			// byte-body round-trip (including a non-UTF-8 byte).
			w.Write([]byte{0xff, 0x00, 0x41})
		}
		_ = body
	})
	mux.HandleFunc("/mybucket/copy.txt", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := check(w, r); !ok {
			return
		}
		if r.Header.Get("x-amz-copy-source") == "" {
			http.Error(w, "no copy source", http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `<CopyObjectResult><ETag>"copied"</ETag></CopyObjectResult>`)
	})
	mux.HandleFunc("/mybucket/big.txt", func(w http.ResponseWriter, r *http.Request) {
		body, ok := check(w, r)
		if !ok {
			return
		}
		q := r.URL.Query()
		// The upload id carries `/` and `+` so the test proves the module's
		// percent-encoded query is preserved on the wire (signature stays valid)
		// and the id decodes back intact for dispatch.
		switch {
		case r.Method == http.MethodPost && q.Has("uploads"):
			fmt.Fprint(w, `<InitiateMultipartUploadResult><UploadId>UP/LOAD+123</UploadId></InitiateMultipartUploadResult>`)
		case r.Method == http.MethodPut && q.Get("partNumber") != "":
			if q.Get("uploadId") != "UP/LOAD+123" {
				http.Error(w, "bad upload id", http.StatusBadRequest)
				return
			}
			w.Header().Set("ETag", `"part-etag-`+q.Get("partNumber")+`"`)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && q.Get("uploadId") == "UP/LOAD+123":
			if !strings.Contains(string(body), "<ETag>") {
				http.Error(w, "no parts", http.StatusBadRequest)
				return
			}
			fmt.Fprint(w, `<CompleteMultipartUploadResult><ETag>"final"</ETag></CompleteMultipartUploadResult>`)
		case r.Method == http.MethodDelete && q.Get("uploadId") == "UP/LOAD+123":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected", http.StatusBadRequest)
		}
	})
	mux.HandleFunc("/mybucket", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := check(w, r); !ok {
			return
		}
		fmt.Fprint(w, `<?xml version="1.0"?><ListBucketResult><Contents><Key>hello.txt</Key></Contents><Contents><Key>docs/readme.md</Key></Contents></ListBucketResult>`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	s3Mod, err := filepath.Abs(filepath.Join("..", "..", "modules", "s3.j"))
	if err != nil {
		t.Fatal(err)
	}
	httpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "http.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
import %q as s3;
import %q as http;
def c as s3.Client init s3.connect(%q, %q, %q, %q);

# get / put / delete / list
def pu as http.Response init s3.put($c, "mybucket", "hello.txt", %q);
testing.assertEqual($pu.status, 200);
def g as http.Response init s3.get($c, "mybucket", "hello.txt");
testing.assertEqual($g.status, 200);
testing.assertEqual($g.body, %q);
def d as http.Response init s3.delete($c, "mybucket", "hello.txt");
testing.assertEqual($d.status, 204);
def l as http.Response init s3.listObjects($c, "mybucket");
testing.assertEqual($l.status, 200);
testing.assertEqual(len(s3.objectKeys($l.body)), 2);

# putWith (content type + metadata, both signed; the value has internal double
# spaces to exercise the SigV4 canonical space-collapse rule)
def pw as http.Response init s3.putWith($c, "mybucket", "hello.txt", "hi", "text/plain", {"team": "eng  west"});
testing.assertEqual($pw.status, 200);

# HEAD reads metadata back
def h as http.Response init s3.head($c, "mybucket", "hello.txt");
testing.assertEqual($h.status, 200);
testing.assertEqual(http.header($h, "x-amz-meta-team"), "eng");

# byte body round-trip (a non-UTF-8 byte survives)
def raw as bytes;
$raw[] = 255;
$raw[] = 0;
$raw[] = 65;
def pb as http.Response init s3.putBytes($c, "mybucket", "bin.dat", $raw);
testing.assertEqual($pb.status, 200);
def gb as http.BytesResponse init s3.getBytes($c, "mybucket", "bin.dat");
testing.assertEqual($gb.status, 200);
testing.assertEqual(len($gb.body), 3);
testing.assertEqual($gb.body[0], 255);
testing.assertEqual($gb.body[2], 65);

# copy (x-amz-copy-source signed)
def cp as http.Response init s3.copy($c, "mybucket", "hello.txt", "mybucket", "copy.txt");
testing.assertEqual($cp.status, 200);

# multipart upload
def uid as string init s3.createMultipartUpload($c, "mybucket", "big.txt", "application/octet-stream");
testing.assertEqual($uid, "UP/LOAD+123");
def part1 as bytes;
$part1[] = 1;
def e1 as string init s3.uploadPart($c, "mybucket", "big.txt", $uid, 1, $part1);
testing.assertEqual($e1, "\"part-etag-1\"");
def etags as list of string init [$e1];
def comp as http.Response init s3.completeMultipartUpload($c, "mybucket", "big.txt", $uid, $etags);
testing.assertEqual($comp.status, 200);
def ab as http.Response init s3.abortMultipartUpload($c, "mybucket", "big.txt", $uid);
testing.assertEqual($ab.status, 204);`,
		s3Mod, httpMod, srv.URL, region, accessKey, secret, objectBody, objectBody)
	progPath := filepath.Join(dir, "s3.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("s3 program failed with code %d", code)
	}
}
