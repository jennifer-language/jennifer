// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package main

import (
	"crypto"
	crand "crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// runOauthProgram writes a .j program that imports oauth.j as `oauth` and
// runs it as a test, failing t when an assertion in it fails.
func runOauthProgram(t *testing.T, name, body string) {
	t.Helper()
	oauthMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "oauth.j"))
	if err != nil {
		t.Fatal(err)
	}
	prog := fmt.Sprintf("use testing;\nuse json;\nuse fs;\nuse httpd;\nuse maps;\nuse task;\nimport %q as oauth;\n", oauthMod) + body
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(path); code != testExitPass {
		t.Fatalf("%s failed with code %d", name, code)
	}
}

// TestOauthDiscover: discover reads the OpenID Connect location, falls back
// to RFC 8414's for an issuer with a path, refuses a document naming another
// issuer, and the Config it returns drives a grant whose reply keeps the ID
// token and the non-standard fields.
func TestOauthDiscover(t *testing.T) {
	var base string
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"issuer":%q,"token_endpoint":%q,"authorization_endpoint":%q,`+
			`"device_authorization_endpoint":%q,"jwks_uri":"x"}`,
			base, base+"/token", base+"/authorize", base+"/device")
	})
	// only RFC 8414's location for the issuer <base>/tenant
	mux.HandleFunc("/.well-known/oauth-authorization-server/tenant", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"issuer":%q,"token_endpoint":%q}`, base+"/tenant", base+"/tenant/token")
	})
	mux.HandleFunc("/liar/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"issuer":"https://elsewhere.example","token_endpoint":%q}`, base+"/token")
	})
	// a login part makes a loopback-looking URL go elsewhere
	mux.HandleFunc("/login/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"issuer":%q,"token_endpoint":"http://[::1]@evil.example/token"}`, base+"/login")
	})
	mux.HandleFunc("/plain/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"issuer":%q,"token_endpoint":"http://idp.example/token"}`, base+"/plain")
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Header.Get("Accept") != "application/json" || r.FormValue("grant_type") != "client_credentials" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"invalid_request"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"at","token_type":"Bearer","expires_in":60,"id_token":"h.p.s",`+
			`"instance_url":"https://eu1.example.com","ext_expires_in":120}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	base = srv.URL

	runOauthProgram(t, "discover.j", fmt.Sprintf(`
def base as string init %q;
def cfg as oauth.Config init oauth.discover($base, "id", "secret", "s");
testing.assertEqual($cfg.tokenUrl, $base + "/token");
testing.assertEqual($cfg.authorizeUrl, $base + "/authorize");
testing.assertEqual($cfg.deviceUrl, $base + "/device");
testing.assertEqual($cfg.clientId, "id");
def tok as oauth.Token init oauth.clientCredentials($cfg);
testing.assertEqual($tok.accessToken, "at");
testing.assertEqual($tok.idToken, "h.p.s");
testing.assertEqual($tok.extra, {"instance_url": "https://eu1.example.com", "ext_expires_in": "120"});
def t2 as oauth.Config init oauth.discover($base + "/tenant", "id", "", "");
testing.assertEqual($t2.tokenUrl, $base + "/tenant/token");
testing.assertEqual($t2.deviceUrl, "");
testing.assertEqual($t2.authorizeUrl, "");
def cases as map of string to string init {
    "/liar": "does not name the issuer",
    "/plain": "token_endpoint must be an https URL",
    "/login": "token_endpoint must be an https URL",
    "/none": "discover: no metadata at "
};
for (def path in maps.keys($cases)) {
    def msg as string init "";
    try {
        oauth.discover($base + $path, "id", "", "");
    } catch (e) {
        $msg = $e.kind + ": " + $e.message;
    }
    testing.assertContains($msg, "oauth: discover: ");
    testing.assertContains($msg, $cases[$path]);
}
def msg as string init "";
try {
    oauth.discover("http://idp.example", "id", "", "");
} catch (e) {
    $msg = $e.message;
}
testing.assertEqual($msg, "discover: the issuer must be an https URL without a query, got http://idp.example");
for (def bad in [$base + "/?x=1", $base + "/#f", "http://localhost:1@idp.example"]) {
    $msg = "";
    try {
        oauth.discover($bad, "id", "", "");
    } catch (e) {
        $msg = $e.message;
    }
    testing.assertContains($msg, "discover: the issuer must be an https URL without a query");
}
`, base))
}

// TestOauthClientAssertion: clientCredentialsJwt authenticates with an RS256
// client assertion the provider verifies, and never sends the secret.
func TestOauthClientAssertion(t *testing.T) {
	dir := t.TempDir()
	key, err := rsa.GenerateKey(crand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(dir, "client.pem")
	writePEM(t, keyPath, "PRIVATE KEY", mustMarshalPKCS8(t, key))
	var tokenURL string
	check := func(r *http.Request) error {
		if r.FormValue("grant_type") != "client_credentials" || r.FormValue("client_id") != "app-1" ||
			r.FormValue("scope") != "api" || r.FormValue("client_secret") != "" ||
			r.FormValue("client_assertion_type") != "urn:ietf:params:oauth:client-assertion-type:jwt-bearer" {
			return fmt.Errorf("form %v", r.Form)
		}
		parts := strings.Split(r.FormValue("client_assertion"), ".")
		if len(parts) != 3 {
			return fmt.Errorf("assertion has %d parts", len(parts))
		}
		var head struct{ Alg, Kid string }
		var claims map[string]any
		if err := decodeJWTPart(parts[0], &head); err != nil {
			return err
		}
		if err := decodeJWTPart(parts[1], &claims); err != nil {
			return err
		}
		sig, err := base64.RawURLEncoding.DecodeString(parts[2])
		if err != nil {
			return err
		}
		sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
		if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, sum[:], sig); err != nil {
			return err
		}
		iat, _ := claims["iat"].(float64)
		exp, _ := claims["exp"].(float64)
		jti, _ := claims["jti"].(string)
		if head.Alg != "RS256" || head.Kid != "k1" || claims["iss"] != "app-1" ||
			claims["sub"] != "app-1" || claims["aud"] != tokenURL || exp-iat != 300 || len(jti) < 16 {
			return fmt.Errorf("header %+v claims %v", head, claims)
		}
		return nil
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		if err := check(r); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error":"invalid_client","error_description":%q}`, err.Error())
			return
		}
		fmt.Fprint(w, `{"access_token":"cc","token_type":"Bearer","expires_in":60}`)
	}))
	defer srv.Close()
	tokenURL = srv.URL + "/token"

	runOauthProgram(t, "assertion.j", fmt.Sprintf(`
def cfg as oauth.Config init oauth.Config{tokenUrl: %q, deviceUrl: "", authorizeUrl: "",
    clientId: "app-1", clientSecret: "must-not-be-sent", scope: "api"};
def a as string init oauth.clientAssertion("RS256", "app-1", $cfg.tokenUrl, fs.readBytes(%q), "k1");
def tok as oauth.Token init oauth.clientCredentialsJwt($cfg, $a);
testing.assertEqual($tok.accessToken, "cc");
`, tokenURL, keyPath))
}

// TestOauthAuthorizationCode drives the browser sign-in: the provider's
// authorize endpoint plays the browser and follows the redirect to the
// program's loopback server (after twenty stray requests with another state
// or path, which must be ignored), and the token endpoint checks the PKCE verifier against
// the challenge before issuing a token. A refused sign-in (access_denied)
// reaches the program as an oauth Error.
func TestOauthAuthorizationCode(t *testing.T) {
	// the browser's requests to the program's loopback server, keep-alive on
	// as in a real browser: awaitCode closes the connection itself
	browser := &http.Client{Transport: &http.Transport{}}
	var mu sync.Mutex
	challenges := map[string]string{} // code -> challenge
	redirects := map[string]string{}  // code -> redirect_uri
	mux := http.NewServeMux()
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("response_type") != "code" || q.Get("client_id") != "app" ||
			q.Get("code_challenge_method") != "S256" || q.Get("scope") != "openid" {
			http.Error(w, "bad request "+r.URL.RawQuery, http.StatusBadRequest)
			return
		}
		redirect := q.Get("redirect_uri")
		// stray requests first (more than any request limit would allow):
		// wrong state, another path, then the real redirect
		for i := 0; i < 20; i++ {
			target := redirect + "?code=evil&state=forged"
			if i%2 == 1 {
				target = strings.TrimSuffix(redirect, "/callback") + "/other"
			}
			if resp, err := browser.Get(target); err == nil {
				resp.Body.Close()
			}
		}
		v := url.Values{"state": {q.Get("state")}}
		if q.Get("login_hint") == "no" {
			v.Set("error", "access_denied")
			v.Set("error_description", "User said no")
		} else {
			code := "code-" + q.Get("state")[:6]
			mu.Lock()
			challenges[code] = q.Get("code_challenge")
			redirects[code] = redirect
			mu.Unlock()
			v.Set("code", code)
		}
		resp, err := browser.Get(redirect + "?" + v.Encode())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		resp.Body.Close()
		fmt.Fprint(w, "redirected")
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		code := r.FormValue("code")
		mu.Lock()
		challenge, redirect := challenges[code], redirects[code]
		mu.Unlock()
		sum := sha256.Sum256([]byte(r.FormValue("code_verifier")))
		if r.FormValue("grant_type") != "authorization_code" || challenge == "" ||
			base64.RawURLEncoding.EncodeToString(sum[:]) != challenge ||
			r.FormValue("redirect_uri") != redirect || r.FormValue("client_id") != "app" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error":"invalid_grant","error_description":%q}`, r.Form.Encode())
			return
		}
		fmt.Fprint(w, `{"access_token":"user-token","token_type":"Bearer","expires_in":60,`+
			`"refresh_token":"rt","id_token":"h.p.s"}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	runOauthProgram(t, "authcode.j", fmt.Sprintf(`
import %q as http;
func browse(url as string) {
    return http.get($url, {}).status;
}
func signIn(cfg as oauth.Config, hint as string) {
    def srv as httpd.Server init httpd.listen("127.0.0.1:0");
    defer httpd.shutdown($srv);
    def req as oauth.AuthRequest init oauth.authorize($cfg, "http://" + httpd.address($srv) + "/callback");
    def b as task of int init spawn {
        return browse($req.url + $hint);
    };
    def code as string init "";
    try {
        $code = oauth.awaitCode($req, $srv);
    } catch (e) {
        task.waitTimeout($b, 10000);
        throw $e;
    }
    testing.assertEqual(task.waitTimeout($b, 10000), 200);
    return oauth.exchangeCode($cfg, $req, $code);
}
def cfg as oauth.Config init oauth.Config{tokenUrl: %q, deviceUrl: "", authorizeUrl: %q,
    clientId: "app", clientSecret: "", scope: "openid"};
def tok as oauth.Token init signIn($cfg, "");
testing.assertEqual($tok.accessToken, "user-token");
testing.assertEqual($tok.refreshToken, "rt");
testing.assertEqual($tok.idToken, "h.p.s");
def msg as string init "";
try {
    signIn($cfg, "&login_hint=no");
} catch (e) {
    $msg = $e.kind + ": " + $e.message;
}
testing.assertEqual($msg, "oauth: authorization refused: access_denied: User said no");
`, mustAbs(t, filepath.Join("..", "..", "modules", "http.j")), srv.URL+"/token", srv.URL+"/authorize"))
}

// TestOauthReplyBounds: a token reply over 1 MiB is refused as an oauth
// error before it is decoded (a hostile endpoint cannot fill memory), and a
// normal reply still works.
func TestOauthReplyBounds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/big" {
			fmt.Fprintf(w, `{"access_token":"at","token_type":"Bearer","pad":%q}`, strings.Repeat("x", 2<<20))
			return
		}
		fmt.Fprint(w, `{"access_token":"at","token_type":"Bearer"}`)
	}))
	defer srv.Close()
	runOauthProgram(t, "bounds.j", fmt.Sprintf(`
def cfg as oauth.Config init oauth.Config{tokenUrl: %q, deviceUrl: "", authorizeUrl: "",
    clientId: "id", clientSecret: "s", scope: ""};
testing.assertEqual(oauth.clientCredentials($cfg).accessToken, "at");
$cfg.tokenUrl = $cfg.tokenUrl + "big";
def msg as string init "";
try {
    oauth.clientCredentials($cfg);
} catch (e) {
    $msg = $e.kind + ": " + $e.message;
}
testing.assertEqual($msg, "oauth: the reply from " + $cfg.tokenUrl + " is over 1048576 bytes");
`, srv.URL+"/"))
}

// mustAbs is filepath.Abs, failing t on an error.
func mustAbs(t *testing.T, p string) string {
	t.Helper()
	a, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// TestOauthRelayHelpers: the calls a server relaying sign-ins for its own
// clients needs. devicePoll makes one poll and reports the provider's answer
// (pending, a refusal, GitHub's HTML 500 for a spent code) instead of
// waiting; exchangeCodeWith trades a code with a verifier made elsewhere;
// discoverMetadata names the UserInfo endpoint and keeps the document. A
// connection that cannot be made is an oauth error.
func TestOauthRelayHelpers(t *testing.T) {
	var base string
	verifier := strings.Repeat("v", 43)
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"issuer":%q,"token_endpoint":%q,"userinfo_endpoint":%q,"jwks_uri":"https://keys.example/"}`,
			base, base+"/token", base+"/userinfo")
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		if r.FormValue("grant_type") == "authorization_code" {
			if r.FormValue("code_verifier") != verifier || r.FormValue("code") != "c1" ||
				r.FormValue("redirect_uri") != "http://127.0.0.1:9/cb" {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprintf(w, `{"error":"invalid_grant","error_description":%q}`, r.Form.Encode())
				return
			}
			fmt.Fprint(w, `{"access_token":"code-token","token_type":"Bearer"}`)
			return
		}
		switch r.FormValue("device_code") {
		case "pending":
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"authorization_pending"}`)
		case "denied":
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"access_denied","error_description":"User said no"}`)
		case "spent":
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `<html>Internal Server Error</html>`)
		default:
			fmt.Fprint(w, `{"access_token":"device-token","token_type":"Bearer"}`)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	base = srv.URL
	// a port nothing listens on
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	dead := "http://" + ln.Addr().String() + "/token"
	ln.Close()

	runOauthProgram(t, "relay.j", fmt.Sprintf(`
def base as string init %q;
def d as oauth.Discovery init oauth.discoverMetadata($base, "id", "", "openid");
testing.assertEqual($d.config.tokenUrl, $base + "/token");
testing.assertEqual($d.userinfoUrl, $base + "/userinfo");
testing.assertEqual(json.asString($d.metadata, "/jwks_uri"), "https://keys.example/");
def cfg as oauth.Config init $d.config;
func poll(cfg as oauth.Config, code as string) {
    return oauth.devicePoll($cfg, oauth.DeviceAuth{deviceCode: $code, userCode: "", verificationUri: "",
        interval: 5, expiresAt: 0});
}
testing.assertEqual(poll($cfg, "pending").state, "pending");
def no as oauth.DevicePoll init poll($cfg, "denied");
testing.assertEqual([$no.state, $no.errorCode, $no.description], ["failed", "access_denied", "User said no"]);
def spent as oauth.DevicePoll init poll($cfg, "spent");
testing.assertEqual([$spent.state, $spent.errorCode], ["failed", ""]);
testing.assertEqual($spent.status, 500);
def ok as oauth.DevicePoll init poll($cfg, "ok");
testing.assertEqual($ok.token.accessToken, "device-token");
def tok as oauth.Token init oauth.exchangeCodeWith($cfg, "c1", %q, "http://127.0.0.1:9/cb");
testing.assertEqual($tok.accessToken, "code-token");
def msg as string init "";
try {
    oauth.exchangeCodeWith($cfg, "wrong", %q, "http://127.0.0.1:9/cb");
} catch (e) {
    $msg = $e.kind + ": " + $e.message;
}
testing.assertContains($msg, "oauth: invalid_grant");
$cfg.tokenUrl = %q;
$msg = "";
try {
    oauth.clientCredentials($cfg);
} catch (e) {
    $msg = $e.kind + ": " + $e.message;
}
testing.assertContains($msg, "oauth: the request to " + $cfg.tokenUrl + " failed: ");
`, base, verifier, verifier, dead))
}
