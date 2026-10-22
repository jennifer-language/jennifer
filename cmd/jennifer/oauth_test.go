// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package main

import (
	"crypto"
	crand "crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

// TestOauthJwtBearer drives the RFC 7523 grant end to end with RS256: the .j
// program builds an assertion with oauth.jwtAssertion from a PEM key minted
// here, and the mock token endpoint checks what a real provider checks - the
// header alg / kid, the RSA signature, and the claims - before issuing a token.
// An assertion signed with a different key is refused, which the program must
// see as a catchable oauth Error.
func TestOauthJwtBearer(t *testing.T) {
	dir := t.TempDir()
	good, err := rsa.GenerateKey(crand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	other, err := rsa.GenerateKey(crand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	writePEM(t, filepath.Join(dir, "good.pem"), "PRIVATE KEY", mustMarshalPKCS8(t, good))
	writePEM(t, filepath.Join(dir, "other.pem"), "PRIVATE KEY", mustMarshalPKCS8(t, other))

	var srvURL string
	check := func(r *http.Request) error {
		if r.FormValue("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
			return fmt.Errorf("grant_type %q", r.FormValue("grant_type"))
		}
		if r.FormValue("scope") != "read" {
			return fmt.Errorf("scope %q", r.FormValue("scope"))
		}
		parts := strings.Split(r.FormValue("assertion"), ".")
		if len(parts) != 3 {
			return fmt.Errorf("assertion has %d parts", len(parts))
		}
		var head struct{ Alg, Kid, Typ string }
		var claims map[string]any
		if err := decodeJWTPart(parts[0], &head); err != nil {
			return err
		}
		if err := decodeJWTPart(parts[1], &claims); err != nil {
			return err
		}
		if head.Alg != "RS256" || head.Kid != "key-7" || head.Typ != "JWT" {
			return fmt.Errorf("header %+v", head)
		}
		sig, err := base64.RawURLEncoding.DecodeString(parts[2])
		if err != nil {
			return err
		}
		sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
		if err := rsa.VerifyPKCS1v15(&good.PublicKey, crypto.SHA256, sum[:], sig); err != nil {
			return err
		}
		iat, _ := claims["iat"].(float64)
		exp, _ := claims["exp"].(float64)
		if claims["iss"] != "svc@example.com" || claims["aud"] != srvURL+"/token" ||
			claims["jti"] != "once" || exp-iat != 600 {
			return fmt.Errorf("claims %v", claims)
		}
		if _, ok := claims["sub"]; ok {
			return fmt.Errorf("empty subject was sent: %v", claims)
		}
		return nil
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		if err := check(r); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error":"invalid_grant","error_description":%q}`, err.Error())
			return
		}
		fmt.Fprint(w, `{"access_token":"sa-token","token_type":"Bearer","expires_in":3600}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	srvURL = srv.URL

	oauthMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "oauth.j"))
	if err != nil {
		t.Fatal(err)
	}
	prog := fmt.Sprintf(`use testing;
use json;
use fs;
import %q as oauth;
def tokenUrl as string init %q;
def cfg as oauth.Config init oauth.Config{tokenUrl: $tokenUrl, deviceUrl: "", clientId: "", clientSecret: "", scope: "read"};
def extra as json.Value init json.set(json.map(), "/jti", "once");
def good as bytes init fs.readBytes(%q);
def a as string init oauth.jwtAssertion("svc@example.com", "", $tokenUrl, 600, $extra, $good, "RS256", "key-7");
def tok as oauth.Token init oauth.jwtBearer($cfg, $a);
testing.assertEqual($tok.accessToken, "sa-token");
testing.assertTrue($tok.expiresAt > 0);
def other as bytes init fs.readBytes(%q);
def bad as string init oauth.jwtAssertion("svc@example.com", "", $tokenUrl, 600, $extra, $other, "RS256", "key-7");
def threw as bool init false;
try {
    oauth.jwtBearer($cfg, $bad);
} catch (e) {
    $threw = true;
    testing.assertEqual($e.kind, "oauth");
    testing.assertContains($e.message, "invalid_grant");
}
testing.assertTrue($threw);`, oauthMod, srv.URL+"/token",
		filepath.Join(dir, "good.pem"), filepath.Join(dir, "other.pem"))
	progPath := filepath.Join(dir, "bearer.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("oauth jwt-bearer program failed with code %d", code)
	}
}

// decodeJWTPart base64url-decodes one compact-JWT segment into v.
func decodeJWTPart(seg string, v any) error {
	raw, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}
