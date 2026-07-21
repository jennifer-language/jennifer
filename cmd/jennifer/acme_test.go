// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// A faithful-enough in-process ACME (RFC 8555) server. It exists to drive the
// acme.j module end to end AND to *cryptographically verify* every JWS the
// module sends: it reconstructs each signing input, parses the account public
// key out of the JWS `jwk` (or the stored account key by `kid`), and checks the
// signature under the algorithm the protected header claims. That makes it an
// independent oracle for crypto.jwkPublic + the module's JWS assembly - a wrong
// canonical JWK, a wrong hash, or an alg/curve mismatch all fail here.
type acmeServer struct {
	mu        sync.Mutex
	base      string
	nonces    map[string]bool           // issued & unused
	accounts  map[string]map[string]any // kid URL -> account public JWK
	token     string                    // the single challenge token
	authValid bool                      // challenge accepted -> authz valid
	orderDone bool                      // finalize done -> order valid
	csrDNS    []string                  // SANs pulled from the finalize CSR
	caKey     *ecdsa.PrivateKey
	caCert    *x509.Certificate
	thumb     string // base64url SHA-256 thumbprint of the account JWK
}

func newACMEServer(t *testing.T) (*acmeServer, *httptest.Server) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test ACME CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, _ := x509.ParseCertificate(caDER)
	s := &acmeServer{
		nonces:   map[string]bool{},
		accounts: map[string]map[string]any{},
		token:    "challenge-token-abc",
		caKey:    caKey,
		caCert:   caCert,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/directory", s.directory)
	mux.HandleFunc("/new-nonce", s.newNonce)
	mux.HandleFunc("/new-account", s.newAccount)
	mux.HandleFunc("/new-order", s.newOrder)
	mux.HandleFunc("/authz/1", s.authz)
	mux.HandleFunc("/chal/1", s.challenge)
	mux.HandleFunc("/finalize/1", s.finalize)
	mux.HandleFunc("/order/1", s.order)
	mux.HandleFunc("/cert/1", s.cert)
	srv := httptest.NewServer(mux)
	s.base = srv.URL
	return s, srv
}

func (s *acmeServer) issueNonce() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	n := base64.RawURLEncoding.EncodeToString(b[:])
	s.mu.Lock()
	s.nonces[n] = true
	s.mu.Unlock()
	return n
}

// jwsHeader is the decoded protected header we care about.
type jwsHeader struct {
	Alg   string         `json:"alg"`
	Nonce string         `json:"nonce"`
	URL   string         `json:"url"`
	Kid   string         `json:"kid"`
	JWK   map[string]any `json:"jwk"`
}

// verifyJWS decodes the flattened JWS body, checks the nonce was issued (and
// consumes it), verifies the signature against the jwk/kid public key under the
// header alg, and returns the decoded payload plus the account JWK.
func (s *acmeServer) verifyJWS(t *testing.T, w http.ResponseWriter, body []byte, wantURL string) ([]byte, map[string]any, bool) {
	t.Helper()
	var flat struct{ Protected, Payload, Signature string }
	if err := json.Unmarshal(body, &flat); err != nil {
		s.fail(w, "malformed JWS envelope")
		return nil, nil, false
	}
	hdrRaw, err := base64.RawURLEncoding.DecodeString(flat.Protected)
	if err != nil {
		s.fail(w, "protected not base64url")
		return nil, nil, false
	}
	var hdr jwsHeader
	if err := json.Unmarshal(hdrRaw, &hdr); err != nil {
		s.fail(w, "protected not JSON")
		return nil, nil, false
	}
	// Anti-replay: the nonce must have been issued and not yet used.
	s.mu.Lock()
	ok := s.nonces[hdr.Nonce]
	delete(s.nonces, hdr.Nonce)
	s.mu.Unlock()
	if !ok {
		w.Header().Set("Replay-Nonce", s.issueNonce())
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"type":"urn:ietf:params:acme:error:badNonce","detail":"unknown nonce"}`)
		return nil, nil, false
	}
	if hdr.URL != wantURL {
		s.fail(w, "url mismatch: got "+hdr.URL+" want "+wantURL)
		return nil, nil, false
	}
	// Resolve the account public key: from the embedded jwk, or by kid lookup.
	jwk := hdr.JWK
	if jwk == nil {
		s.mu.Lock()
		jwk = s.accounts[hdr.Kid]
		s.mu.Unlock()
		if jwk == nil {
			s.fail(w, "unknown kid "+hdr.Kid)
			return nil, nil, false
		}
	}
	payload, err := base64.RawURLEncoding.DecodeString(flat.Payload)
	if err != nil {
		s.fail(w, "payload not base64url")
		return nil, nil, false
	}
	sig, err := base64.RawURLEncoding.DecodeString(flat.Signature)
	if err != nil {
		s.fail(w, "signature not base64url")
		return nil, nil, false
	}
	signingInput := flat.Protected + "." + flat.Payload
	if !verifySig(hdr.Alg, jwk, []byte(signingInput), sig) {
		s.fail(w, "JWS signature verification FAILED for alg "+hdr.Alg)
		return nil, nil, false
	}
	w.Header().Set("Replay-Nonce", s.issueNonce())
	return payload, jwk, true
}

// verifySig is the independent JWS signature check over the four ACME algs.
func verifySig(alg string, jwk map[string]any, input, sig []byte) bool {
	digest, hsh := hashFor(alg, input)
	kty, _ := jwk["kty"].(string)
	switch {
	case strings.HasPrefix(alg, "RS") && kty == "RSA":
		n := b64uBig(jwk["n"])
		e := b64uBig(jwk["e"])
		pub := &rsa.PublicKey{N: n, E: int(e.Int64())}
		return rsa.VerifyPKCS1v15(pub, hsh, digest, sig) == nil
	case strings.HasPrefix(alg, "ES") && kty == "EC":
		crv, _ := jwk["crv"].(string)
		curve := curveFor(crv)
		if curve == nil {
			return false
		}
		// JOSE binds each ES alg to one curve: ES256<->P-256, etc. Enforce it,
		// so a P-384 key labelled ES256 (the pre-fix bug) is rejected here.
		if joseCurve(alg) != crv {
			return false
		}
		x := b64uBig(jwk["x"])
		y := b64uBig(jwk["y"])
		pub := &ecdsa.PublicKey{Curve: curve, X: x, Y: y}
		size := (curve.Params().BitSize + 7) / 8
		if len(sig) != 2*size {
			return false
		}
		r := new(big.Int).SetBytes(sig[:size])
		sv := new(big.Int).SetBytes(sig[size:])
		return ecdsa.Verify(pub, digest, r, sv)
	}
	return false
}

func hashFor(alg string, input []byte) ([]byte, crypto.Hash) {
	switch {
	case strings.HasSuffix(alg, "384"):
		h := sha512.Sum384(input)
		return h[:], crypto.SHA384
	case strings.HasSuffix(alg, "512"):
		h := sha512.Sum512(input)
		return h[:], crypto.SHA512
	default:
		h := sha256.Sum256(input)
		return h[:], crypto.SHA256
	}
}

func curveFor(crv string) elliptic.Curve {
	switch crv {
	case "P-256":
		return elliptic.P256()
	case "P-384":
		return elliptic.P384()
	case "P-521":
		return elliptic.P521()
	}
	return nil
}

func joseCurve(alg string) string {
	switch {
	case strings.HasSuffix(alg, "384"):
		return "P-384"
	case strings.HasSuffix(alg, "512"):
		return "P-521"
	default:
		return "P-256"
	}
}

func b64uBig(v any) *big.Int {
	s, _ := v.(string)
	b, _ := base64.RawURLEncoding.DecodeString(s)
	return new(big.Int).SetBytes(b)
}

func (s *acmeServer) fail(w http.ResponseWriter, detail string) {
	w.Header().Set("Replay-Nonce", s.issueNonce())
	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprintf(w, `{"type":"urn:ietf:params:acme:error:malformed","detail":%q}`, detail)
}

func (s *acmeServer) directory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"newNonce":%q,"newAccount":%q,"newOrder":%q}`,
		s.base+"/new-nonce", s.base+"/new-account", s.base+"/new-order")
}

func (s *acmeServer) newNonce(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Replay-Nonce", s.issueNonce())
	w.WriteHeader(http.StatusOK)
}

func (s *acmeServer) newAccount(w http.ResponseWriter, r *http.Request) {
	body := readBody(r)
	t := testingFromReq(r)
	_, jwk, ok := s.verifyJWS(t, w, body, s.base+"/new-account")
	if !ok {
		return
	}
	kid := s.base + "/account/1"
	// Compute the RFC 7638 thumbprint from the received jwk, independently.
	s.mu.Lock()
	s.accounts[kid] = jwk
	s.thumb = thumbprint(jwk)
	s.mu.Unlock()
	w.Header().Set("Location", kid)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprint(w, `{"status":"valid"}`)
}

func (s *acmeServer) newOrder(w http.ResponseWriter, r *http.Request) {
	t := testingFromReq(r)
	payload, _, ok := s.verifyJWS(t, w, readBody(r), s.base+"/new-order")
	if !ok {
		return
	}
	// echo back an order referencing our single authz.
	_ = payload
	w.Header().Set("Location", s.base+"/order/1")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, `{"status":"pending","authorizations":[%q],"finalize":%q}`,
		s.base+"/authz/1", s.base+"/finalize/1")
}

func (s *acmeServer) authz(w http.ResponseWriter, r *http.Request) {
	t := testingFromReq(r)
	if _, _, ok := s.verifyJWS(t, w, readBody(r), s.base+"/authz/1"); !ok {
		return
	}
	s.mu.Lock()
	status := "pending"
	if s.authValid {
		status = "valid"
	}
	tok := s.token
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"identifier":{"type":"dns","value":"example.com"},"status":%q,`+
		`"challenges":[{"type":"http-01","url":%q,"token":%q,"status":%q}]}`,
		status, s.base+"/chal/1", tok, status)
}

func (s *acmeServer) challenge(w http.ResponseWriter, r *http.Request) {
	t := testingFromReq(r)
	if _, _, ok := s.verifyJWS(t, w, readBody(r), s.base+"/chal/1"); !ok {
		return
	}
	s.mu.Lock()
	s.authValid = true
	tok := s.token
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"type":"http-01","url":%q,"token":%q,"status":"valid"}`, s.base+"/chal/1", tok)
}

func (s *acmeServer) finalize(w http.ResponseWriter, r *http.Request) {
	t := testingFromReq(r)
	payload, _, ok := s.verifyJWS(t, w, readBody(r), s.base+"/finalize/1")
	if !ok {
		return
	}
	var fin struct{ CSR string }
	_ = json.Unmarshal(payload, &fin)
	der, err := base64.RawURLEncoding.DecodeString(fin.CSR)
	if err != nil {
		s.fail(w, "csr not base64url")
		return
	}
	csr, err := x509.ParseCertificateRequest(der)
	if err != nil {
		s.fail(w, "csr will not parse: "+err.Error())
		return
	}
	if err := csr.CheckSignature(); err != nil {
		s.fail(w, "csr signature invalid: "+err.Error())
		return
	}
	s.mu.Lock()
	s.csrDNS = csr.DNSNames
	s.orderDone = true
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"processing","authorizations":[%q],"finalize":%q}`,
		s.base+"/authz/1", s.base+"/finalize/1")
}

func (s *acmeServer) order(w http.ResponseWriter, r *http.Request) {
	t := testingFromReq(r)
	if _, _, ok := s.verifyJWS(t, w, readBody(r), s.base+"/order/1"); !ok {
		return
	}
	s.mu.Lock()
	done := s.orderDone
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if done {
		fmt.Fprintf(w, `{"status":"valid","authorizations":[%q],"finalize":%q,"certificate":%q}`,
			s.base+"/authz/1", s.base+"/finalize/1", s.base+"/cert/1")
		return
	}
	fmt.Fprintf(w, `{"status":"pending","authorizations":[%q],"finalize":%q}`,
		s.base+"/authz/1", s.base+"/finalize/1")
}

func (s *acmeServer) cert(w http.ResponseWriter, r *http.Request) {
	t := testingFromReq(r)
	if _, _, ok := s.verifyJWS(t, w, readBody(r), s.base+"/cert/1"); !ok {
		return
	}
	// Issue a leaf cert over the CSR's SANs, signed by our test CA.
	leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	s.mu.Lock()
	dns := s.csrDNS
	s.mu.Unlock()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: dns[0]},
		DNSNames:     dns,
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, s.caCert, &leafKey.PublicKey, s.caKey)
	if err != nil {
		s.fail(w, "issue: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/pem-certificate-chain")
	pem.Encode(w, &pem.Block{Type: "CERTIFICATE", Bytes: der})
	pem.Encode(w, &pem.Block{Type: "CERTIFICATE", Bytes: s.caCert.Raw})
}

// thumbprint computes the RFC 7638 JWK SHA-256 thumbprint (base64url) from the
// received jwk map, rebuilding the canonical member subset independently.
func thumbprint(jwk map[string]any) string {
	kty, _ := jwk["kty"].(string)
	var canon string
	if kty == "RSA" {
		canon = fmt.Sprintf(`{"e":%q,"kty":"RSA","n":%q}`, jwk["e"], jwk["n"])
	} else {
		canon = fmt.Sprintf(`{"crv":%q,"kty":"EC","x":%q,"y":%q}`, jwk["crv"], jwk["x"], jwk["y"])
	}
	h := sha256.Sum256([]byte(canon))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// readBody / testingFromReq are tiny helpers; the server stashes *testing.T in a
// package var so handlers can t.Logf on failure without plumbing.
var curT struct {
	sync.Mutex
	t *testing.T
}

func readBody(r *http.Request) []byte {
	b := make([]byte, r.ContentLength)
	if r.ContentLength <= 0 {
		var all []byte
		buf := make([]byte, 512)
		for {
			n, err := r.Body.Read(buf)
			all = append(all, buf[:n]...)
			if err != nil {
				break
			}
		}
		return all
	}
	_, _ = r.Body.Read(b)
	return b
}

func testingFromReq(_ *http.Request) *testing.T {
	curT.Lock()
	defer curT.Unlock()
	return curT.t
}

// runACMEFlow drives the acme.j module all the way to a downloaded certificate
// against the fake CA, for one account-key kind. Returns the account thumbprint
// the server derived, and the keyAuthorization the module produced, so the test
// can assert they agree.
func runACMEFlow(t *testing.T, keyGen string) {
	curT.Lock()
	curT.t = t
	curT.Unlock()
	s, srv := newACMEServer(t)
	defer srv.Close()

	acmeMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "acme.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	kaPath := filepath.Join(dir, "keyauth.txt")
	prog := fmt.Sprintf(`use testing;
use crypto;
use strings;
use fs;
import %q as acme;

def key as bytes init %s;
def client as acme.Client init acme.connect(%q, $key);
$client = acme.register($client, "admin@example.com");
testing.assertContains($client.kid, "/account/1");

def order as acme.Order init acme.order($client, ["example.com"]);
testing.assertEqual($order.status, "pending");
testing.assertEqual(len($order.authorizations), 1);

def authz as acme.Authorization init acme.authorization($client, $order.authorizations[0]);
def ch as acme.Challenge init acme.challenge($authz, "http-01");
testing.assertEqual($ch.status, "pending");

# The HTTP-01 key authorization is token.thumbprint; write it out so the Go side
# can check it equals what the server independently derived from the JWS jwk.
def ka as string init acme.keyAuthorization($client, $ch.token);
testing.assertTrue(strings.startsWith($ka, $ch.token + "."));
fs.writeString(%q, $ka);

acme.accept($client, $ch.url);
def settled as acme.Authorization init acme.pollAuthorization($client, $order.authorizations[0], 1, 5);
testing.assertEqual($settled.status, "valid");

def certKey as bytes init crypto.ecGenerateKey("p256");
def csr as bytes init crypto.csr($certKey, ["example.com"]);
def issued as acme.Order init acme.finalize($client, $order, $csr, 1, 5);
testing.assertEqual($issued.status, "valid");

def chain as string init acme.downloadCertificate($client, $issued);
testing.assertContains($chain, "BEGIN CERTIFICATE");
`, acmeMod, keyGen, srv.URL+"/directory", kaPath)

	progPath := filepath.Join(dir, "flow.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("%s: acme flow failed with code %d", keyGen, code)
	}
	// The server verified every JWS signature already; now confirm the module's
	// HTTP-01 key authorization matches the thumbprint the server derived from
	// the JWS jwk - i.e. crypto.jwkPublic's canonical JWK agrees with an
	// independent RFC 7638 implementation.
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.csrDNS) != 1 || s.csrDNS[0] != "example.com" {
		t.Errorf("%s: server saw CSR SANs %v, want [example.com]", keyGen, s.csrDNS)
	}
	ka, err := os.ReadFile(kaPath)
	if err != nil {
		t.Fatalf("%s: reading key authorization: %v", keyGen, err)
	}
	wantKA := s.token + "." + s.thumb
	if string(ka) != wantKA {
		t.Errorf("%s: module keyAuthorization %q != server-derived %q", keyGen, ka, wantKA)
	}
}

// TestAcmeFullFlowES256 drives the whole flow with a P-256 account key (ES256).
func TestAcmeFullFlowES256(t *testing.T) {
	runACMEFlow(t, `crypto.ecGenerateKey("p256")`)
}

// TestAcmeFullFlowES384 drives it with a P-384 key. Before the alg fix the
// module labelled this JWS "ES256", which the server's curve-bound check
// rejects - so this test would fail. It is the regression guard for that bug.
func TestAcmeFullFlowES384(t *testing.T) {
	runACMEFlow(t, `crypto.ecGenerateKey("p384")`)
}

// TestAcmeFullFlowRS256 drives it with a 2048-bit RSA account key (RS256).
func TestAcmeFullFlowRS256(t *testing.T) {
	runACMEFlow(t, `crypto.rsaGenerateKey(2048)`)
}
