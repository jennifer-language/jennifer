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

// A .j program driving the oauth client against a mock OAuth2 provider asserts
// the buildable-now grants: client-credentials yields a token with an expiry,
// the device flow (deviceStart -> deviceWait) returns the approved token, refresh
// trades a refresh token for a new access token and preserves the refresh token
// when the server omits it, a token-endpoint error surfaces as a catchable
// Error, and the resulting access token feeds sasl's XOAUTH2 encoder. A mismatch
// throws and fails loadForTest.
func TestOauthFlows(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/device", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"device_code":"dc","user_code":"WXYZ-1234",`+
			`"verification_uri":"https://example.com/device","interval":1,"expires_in":900}`)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		switch r.FormValue("grant_type") {
		case "client_credentials":
			fmt.Fprint(w, `{"access_token":"cc-token","token_type":"Bearer","expires_in":3600}`)
		case "refresh_token":
			if r.FormValue("refresh_token") == "valid-refresh" {
				// omit refresh_token in the reply to exercise client-side preservation
				fmt.Fprint(w, `{"access_token":"refreshed-token","token_type":"Bearer","expires_in":3600}`)
			} else {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, `{"error":"invalid_grant","error_description":"bad refresh token"}`)
			}
		case "urn:ietf:params:oauth:grant-type:device_code":
			fmt.Fprint(w, `{"access_token":"device-token","token_type":"Bearer",`+
				`"expires_in":3600,"refresh_token":"dev-refresh"}`)
		default:
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"unsupported_grant_type"}`)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	oauthMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "oauth.j"))
	if err != nil {
		t.Fatal(err)
	}
	saslMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "sasl.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
import %q as oauth;
import %q as sasl;
def cfg as oauth.Config init oauth.Config{tokenUrl: %q, deviceUrl: %q, clientId: "id", clientSecret: "secret", scope: "mail"};
def cc as oauth.Token init oauth.clientCredentials($cfg);
testing.assertEqual($cc.accessToken, "cc-token");
testing.assertTrue($cc.expiresAt > 0);
def dev as oauth.DeviceAuth init oauth.deviceStart($cfg);
testing.assertEqual($dev.userCode, "WXYZ-1234");
testing.assertContains($dev.verificationUri, "device");
def tok as oauth.Token init oauth.deviceWait($cfg, $dev);
testing.assertEqual($tok.accessToken, "device-token");
testing.assertEqual($tok.refreshToken, "dev-refresh");
def refreshed as oauth.Token init oauth.refresh($cfg, "valid-refresh");
testing.assertEqual($refreshed.accessToken, "refreshed-token");
testing.assertEqual($refreshed.refreshToken, "valid-refresh");
def threw as bool init false;
try {
    oauth.refresh($cfg, "bad-refresh");
} catch (e) {
    $threw = true;
    testing.assertContains($e.message, "invalid_grant");
}
testing.assertTrue($threw);
def xoauth as string init sasl.bearer("me@example.com", $tok.accessToken);
testing.assertTrue(len($xoauth) > 0);`, oauthMod, saslMod, srv.URL+"/token", srv.URL+"/device")
	progPath := filepath.Join(dir, "flows.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("oauth flows program failed with code %d", code)
	}
}
