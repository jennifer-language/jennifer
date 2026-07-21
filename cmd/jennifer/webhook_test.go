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
	"testing"
)

// TestWebhookSend drives the webhook module's networked send against an
// in-process receiver that re-signs the body it got and rejects a wrong
// signature. So a 200 back proves the module signed the exact payload with the
// right HMAC-SHA256 in the X-Hub-Signature-256 header, over the real HTTP path.
// A mismatch makes the server 400, the .j assertion throws, and loadForTest
// fails.
func TestWebhookSend(t *testing.T) {
	const secret = "topsecret"
	const payload = "hello, webhook!"
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	mux := http.NewServeMux()
	mux.HandleFunc("/hook", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != payload {
			http.Error(w, "body", http.StatusBadRequest)
			return
		}
		if r.Header.Get("X-Hub-Signature-256") != want {
			http.Error(w, "signature", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"received":true}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	webhookMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "webhook.j"))
	if err != nil {
		t.Fatal(err)
	}
	httpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "http.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
import %q as webhook;
import %q as http;
def r as http.Response init webhook.send(%q, %q, %q);
testing.assertEqual($r.status, 200);
testing.assertContains($r.body, "received");`,
		webhookMod, httpMod, srv.URL+"/hook", payload, secret)
	progPath := filepath.Join(dir, "send.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("webhook send program failed with code %d", code)
	}
}
