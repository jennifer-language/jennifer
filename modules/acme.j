# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

/**
 * An ACME (RFC 8555) client: obtain and renew TLS certificates from Let's
 * Encrypt and compatible CAs. It drives the full flow - account registration, a
 * new order, HTTP-01 or DNS-01 challenge, CSR finalize, and certificate download
 * - over `http` + `json`, with every request a JWS signed by the account key
 * (`RS256` for an RSA key, `ES256` for an EC key). Keys and the CSR come from the
 * `crypto` library, so this needs the default `jennifer` binary.
 *
 * The building blocks are separate calls, not one `obtain()`, because proving
 * domain control is the caller's job: between `order` and `finalize` you must
 * publish the challenge response - serve `keyAuthorization` at
 * `/.well-known/acme-challenge/<token>` (HTTP-01), or set the `dnsRecord` TXT at
 * `_acme-challenge.<domain>` (DNS-01) - then `accept` the challenge. See the
 * demo for the orchestration.
 *
 * Test against a CA's **staging** endpoint first (Let's Encrypt staging issues
 * untrusted certs with far higher rate limits).
 * @module acme
 * @example
 * import "acme.j" as acme;
 * def key as bytes init crypto.ecGenerateKey("p256");
 * def client as acme.Client init acme.connect(
 *     "https://acme-staging-v02.api.letsencrypt.org/directory", $key);
 * def client as acme.Client init acme.register($client, "you@example.com");
 * def order as acme.Order init acme.order($client, ["example.com"]);
 */
import "./http.j" as http;
use json;
use crypto;
use hash;
use encoding;
use convert;
use strings;
use time;

# ---- data ----

/**
 * An ACME client: the CA's directory endpoints plus the account key and (once
 * registered) the account URL used as the JWS `kid`. Value-semantic; `register`
 * returns a copy with `kid` filled in.
 * @field directory {string} the directory URL the client was built from
 * @field newNonce {string} the CA's new-nonce endpoint
 * @field newAccount {string} the CA's new-account endpoint
 * @field newOrder {string} the CA's new-order endpoint
 * @field accountKey {bytes} the account private key, PEM-encoded
 * @field alg {string} the JWS algorithm for the key (`RS256` or `ES256`)
 * @field kid {string} the account URL (empty until `register`)
 */
export def struct Client {
    directory as string,
    newNonce as string,
    newAccount as string,
    newOrder as string,
    accountKey as bytes,
    alg as string,
    kid as string
};

/**
 * An ACME order for one or more identifiers (domains).
 * @field url {string} the order URL (poll this for status)
 * @field status {string} `pending` / `ready` / `processing` / `valid` / `invalid`
 * @field authorizations {list of string} the per-identifier authorization URLs
 * @field finalize {string} the finalize URL (POST the CSR here)
 * @field certificate {string} the certificate URL (empty until issued)
 */
export def struct Order {
    url as string,
    status as string,
    authorizations as list of string,
    finalize as string,
    certificate as string
};

/**
 * A domain-control authorization: the challenges the CA offers for it.
 * @field domain {string} the identifier being authorized
 * @field status {string} `pending` / `valid` / `invalid`
 * @field challenges {list of Challenge} the offered challenges
 */
export def struct Authorization {
    domain as string,
    status as string,
    challenges as list of Challenge
};

/**
 * One challenge within an authorization.
 * @field kind {string} the challenge type (`http-01` / `dns-01` / `tls-alpn-01`)
 * @field url {string} the challenge URL (POST to it to accept / tell the CA it is ready)
 * @field token {string} the challenge token
 * @field status {string} `pending` / `valid` / `invalid`
 */
export def struct Challenge {
    kind as string,
    url as string,
    token as string,
    status as string
};

# ---- base64url (unpadded) + JSON string escaping ----

func encodeSeg(b as bytes) {
    return strings.replace(encoding.toText($b, "base64-url"), "=", "");
}
# jsonEsc escapes a string for embedding in a hand-built JSON literal.
func jsonEsc(s as string) {
    def out as string init strings.replace($s, "\\", "\\\\");
    return strings.replace($out, "\"", "\\\"");
}

# ---- JWS request signing ----

# fetchNonce gets a fresh anti-replay nonce (a HEAD to the newNonce endpoint).
func fetchNonce(client as Client) {
    def resp as http.Response init http.head($client.newNonce, {});
    def nonce as string init http.header($resp, "Replay-Nonce");
    if (len($nonce) == 0) {
        throw Error{kind: "acme", message: "acme: no Replay-Nonce from " + $client.newNonce, file: "", line: 0, col: 0};
    }
    return $nonce;
}

func signInput(client as Client, input as bytes) {
    if ($client.alg == "RS256") {
        return crypto.rsaSign($client.accountKey, $input, "sha256");
    }
    return crypto.ecdsaSign($client.accountKey, $input, "sha256");
}

# jws POSTs a signed request to url. `payload` is the JSON body ("" for the
# POST-as-GET form). `useJwk` embeds the account public JWK (account creation);
# otherwise the account `kid` is used. Returns the http.Response.
func jws(client as Client, url as string, payload as string, useJwk as bool) {
    def nonce as string init fetchNonce($client);
    def protected as string init "{\"alg\":\"" + $client.alg + "\",\"nonce\":\"" + $nonce +
        "\",\"url\":\"" + jsonEsc($url) + "\"";
    if ($useJwk) {
        $protected = $protected + ",\"jwk\":" + crypto.jwkPublic($client.accountKey);
    } else {
        $protected = $protected + ",\"kid\":\"" + jsonEsc($client.kid) + "\"";
    }
    $protected = $protected + "}";
    def head as string init encodeSeg(convert.bytesFromString($protected, "utf-8"));
    def body as string init encodeSeg(convert.bytesFromString($payload, "utf-8"));
    def signingInput as string init $head + "." + $body;
    def sig as bytes init signInput($client, convert.bytesFromString($signingInput, "utf-8"));
    def jwsBody as string init "{\"protected\":\"" + $head + "\",\"payload\":\"" + $body +
        "\",\"signature\":\"" + encodeSeg($sig) + "\"}";
    return http.post($url, "application/jose+json", $jwsBody, {});
}

# jwsOk is jws plus an error check: an ACME error response (>= 400) carries an
# RFC 7807 problem document, which is surfaced as a catchable error.
func jwsOk(client as Client, url as string, payload as string, useJwk as bool) {
    def resp as http.Response init jws($client, $url, $payload, $useJwk);
    if ($resp.status >= 400) {
        throw Error{kind: "acme", message: "acme: request to " + $url + " failed (" +
            convert.toString($resp.status) + "): " + $resp.body, file: "", line: 0, col: 0};
    }
    return $resp;
}

# ---- account + directory ----

/**
 * Build a client from a CA directory URL and an account private key (PEM
 * `bytes`, RSA or EC - from `crypto.rsaGenerateKey` / `crypto.ecGenerateKey`).
 * Fetches the directory to learn the CA's endpoints. Does not yet register.
 * @param directoryUrl {string} the CA's ACME directory URL
 * @param accountKey {bytes} the account private key, PEM-encoded
 * @return {Client} the configured client (kid empty until `register`)
 * @throws {Error} on a transport error or a directory missing required endpoints
 */
export func connect(directoryUrl as string, accountKey as bytes) {
    def resp as http.Response init http.get($directoryUrl, {});
    if ($resp.status != 200) {
        throw Error{kind: "acme", message: "acme.connect: directory fetch failed (" + convert.toString($resp.status) + ")", file: "", line: 0, col: 0};
    }
    def dir as json.Value init json.decode($resp.body);
    def jwk as string init crypto.jwkPublic($accountKey);
    def alg as string init "ES256";
    if (strings.contains($jwk, "\"kty\":\"RSA\"")) {
        $alg = "RS256";
    }
    return Client{
        directory: $directoryUrl,
        newNonce: json.asString($dir, "/newNonce"),
        newAccount: json.asString($dir, "/newAccount"),
        newOrder: json.asString($dir, "/newOrder"),
        accountKey: $accountKey,
        alg: $alg,
        kid: ""
    };
}

/**
 * Register (or look up) the account with the CA and return a client carrying the
 * account URL (`kid`). Agrees to the CA's terms of service. Idempotent: the CA
 * returns the existing account for a known key.
 * @param client {Client} a client from `connect`
 * @param email {string} a contact email (empty for none)
 * @return {Client} a client with `kid` set
 * @throws {Error} on a registration error
 */
export func register(client as Client, email as string) {
    def payload as string init "{\"termsOfServiceAgreed\":true";
    if (len($email) > 0) {
        $payload = $payload + ",\"contact\":[\"mailto:" + jsonEsc($email) + "\"]";
    }
    $payload = $payload + "}";
    def resp as http.Response init jwsOk($client, $client.newAccount, $payload, true);
    def kid as string init http.header($resp, "Location");
    if (len($kid) == 0) {
        throw Error{kind: "acme", message: "acme.register: no account Location header", file: "", line: 0, col: 0};
    }
    def out as Client init $client;
    $out.kid = $kid;
    return $out;
}

# ---- orders + authorizations ----

# parseOrder builds an Order from a response body + its order URL.
func parseOrder(url as string, resp as http.Response) {
    def v as json.Value init json.decode($resp.body);
    def authz as list of string init [];
    def n as int init json.length($v, "/authorizations");
    for (def i as int init 0; $i < $n; $i = $i + 1) {
        $authz[] = json.asString($v, "/authorizations/" + convert.toString($i));
    }
    def cert as string init "";
    if (json.has($v, "/certificate")) {
        $cert = json.asString($v, "/certificate");
    }
    return Order{
        url: $url,
        status: json.asString($v, "/status"),
        authorizations: $authz,
        finalize: json.asString($v, "/finalize"),
        certificate: $cert
    };
}

/**
 * Create an order for one or more domains. The returned order is `pending` with
 * one authorization URL per domain.
 * @param client {Client} a registered client
 * @param domains {list of string} the domains to certify (the first is the CN)
 * @return {Order} the new order
 * @throws {Error} on an order error
 */
export func order(client as Client, domains as list of string) {
    def ids as string init "";
    for (def i as int init 0; $i < len($domains); $i = $i + 1) {
        if ($i > 0) {
            $ids = $ids + ",";
        }
        $ids = $ids + "{\"type\":\"dns\",\"value\":\"" + jsonEsc($domains[$i]) + "\"}";
    }
    def payload as string init "{\"identifiers\":[" + $ids + "]}";
    def resp as http.Response init jwsOk($client, $client.newOrder, $payload, false);
    def url as string init http.header($resp, "Location");
    return parseOrder($url, $resp);
}

/**
 * Fetch an order's current state by URL (a POST-as-GET). Use it to poll after
 * `finalize`.
 * @param client {Client} a registered client
 * @param orderUrl {string} the order URL
 * @return {Order} the order's current state
 */
export func fetchOrder(client as Client, orderUrl as string) {
    def resp as http.Response init jwsOk($client, $orderUrl, "", false);
    return parseOrder($orderUrl, $resp);
}

/**
 * Fetch an authorization: its domain, status, and the challenges the CA offers.
 * @param client {Client} a registered client
 * @param authzUrl {string} an authorization URL (from `order`)
 * @return {Authorization} the authorization
 */
export func authorization(client as Client, authzUrl as string) {
    def resp as http.Response init jwsOk($client, $authzUrl, "", false);
    def v as json.Value init json.decode($resp.body);
    def challenges as list of Challenge init [];
    def n as int init json.length($v, "/challenges");
    for (def i as int init 0; $i < $n; $i = $i + 1) {
        def base as string init "/challenges/" + convert.toString($i);
        $challenges[] = Challenge{
            kind: json.asString($v, $base + "/type"),
            url: json.asString($v, $base + "/url"),
            token: json.asString($v, $base + "/token"),
            status: json.asString($v, $base + "/status")
        };
    }
    return Authorization{
        domain: json.asString($v, "/identifier/value"),
        status: json.asString($v, "/status"),
        challenges: $challenges
    };
}

/**
 * The challenge of a given type within an authorization (`"http-01"` /
 * `"dns-01"`).
 * @param authz {Authorization} an authorization from `authorization`
 * @param kind {string} the challenge type to select
 * @return {Challenge} the matching challenge
 * @throws {Error} when no challenge of that type is offered
 */
export func challenge(authz as Authorization, kind as string) {
    for (def i as int init 0; $i < len($authz.challenges); $i = $i + 1) {
        if ($authz.challenges[$i].kind == $kind) {
            return $authz.challenges[$i];
        }
    }
    throw Error{kind: "acme", message: "acme.challenge: no " + $kind + " challenge for " + $authz.domain, file: "", line: 0, col: 0};
}

# ---- challenge responses (pure) ----

/**
 * The key authorization for a challenge token: `token.thumbprint`, where the
 * thumbprint is the RFC 7638 SHA-256 of the account key's JWK (base64url,
 * unpadded). This is the exact body an **HTTP-01** challenge serves at
 * `/.well-known/acme-challenge/<token>`.
 * @param client {Client} the client (its account key defines the thumbprint)
 * @param token {string} the challenge token
 * @return {string} the key authorization
 */
export func keyAuthorization(client as Client, token as string) {
    def jwk as string init crypto.jwkPublic($client.accountKey);
    def thumb as string init encodeSeg(hash.compute(convert.bytesFromString($jwk, "utf-8"), "sha256"));
    return $token + "." + $thumb;
}

/**
 * The TXT record value for a **DNS-01** challenge: the base64url (unpadded)
 * SHA-256 of the key authorization. Publish it at `_acme-challenge.<domain>`.
 * @param client {Client} the client
 * @param token {string} the challenge token
 * @return {string} the `_acme-challenge` TXT value
 */
export func dnsRecord(client as Client, token as string) {
    def ka as string init keyAuthorization($client, $token);
    return encodeSeg(hash.compute(convert.bytesFromString($ka, "utf-8"), "sha256"));
}

# ---- accept + poll + finalize + download ----

/**
 * Tell the CA a challenge response is in place (a POST of `{}` to the challenge
 * URL). Do this only after publishing the HTTP-01 / DNS-01 response.
 * @param client {Client} a registered client
 * @param challengeUrl {string} the challenge URL (from `challenge`)
 * @return {null} nothing
 * @throws {Error} on an accept error
 */
export func accept(client as Client, challengeUrl as string) {
    jwsOk($client, $challengeUrl, "{}", false);
    return;
}

/**
 * Poll an authorization until it leaves `pending` (becomes `valid` or
 * `invalid`), waiting `intervalMs` between attempts up to `maxTries`.
 * @param client {Client} a registered client
 * @param authzUrl {string} the authorization URL
 * @param intervalMs {int} milliseconds between polls
 * @param maxTries {int} maximum number of polls
 * @return {Authorization} the settled authorization
 * @throws {Error} if it is still pending after `maxTries`
 */
export func pollAuthorization(client as Client, authzUrl as string, intervalMs as int, maxTries as int) {
    for (def i as int init 0; $i < $maxTries; $i = $i + 1) {
        def a as Authorization init authorization($client, $authzUrl);
        if ($a.status != "pending") {
            return $a;
        }
        time.sleep(time.fromMilliseconds($intervalMs));
    }
    throw Error{kind: "acme", message: "acme.pollAuthorization: still pending after " + convert.toString($maxTries) + " tries", file: "", line: 0, col: 0};
}

/**
 * Finalize an order by submitting a CSR, then poll the order until it is `valid`
 * (the certificate is ready) or `invalid`. `csrDer` is a DER PKCS#10 request
 * (`crypto.csr(certKey, domains)`) for the same domains as the order.
 * @param client {Client} a registered client
 * @param order {Order} the (ready) order
 * @param csrDer {bytes} the DER CSR
 * @param intervalMs {int} milliseconds between status polls
 * @param maxTries {int} maximum number of polls
 * @return {Order} the finalized order (`certificate` set when valid)
 * @throws {Error} on a finalize error or if issuance does not complete
 */
export func finalize(client as Client, order as Order, csrDer as bytes, intervalMs as int, maxTries as int) {
    def payload as string init "{\"csr\":\"" + encodeSeg($csrDer) + "\"}";
    jwsOk($client, $order.finalize, $payload, false);
    for (def i as int init 0; $i < $maxTries; $i = $i + 1) {
        def o as Order init fetchOrder($client, $order.url);
        if ($o.status == "valid") {
            return $o;
        }
        if ($o.status == "invalid") {
            throw Error{kind: "acme", message: "acme.finalize: order became invalid", file: "", line: 0, col: 0};
        }
        time.sleep(time.fromMilliseconds($intervalMs));
    }
    throw Error{kind: "acme", message: "acme.finalize: certificate not issued after " + convert.toString($maxTries) + " tries", file: "", line: 0, col: 0};
}

/**
 * Download the issued certificate chain (PEM) for a valid order.
 * @param client {Client} a registered client
 * @param order {Order} a `valid` order (with a `certificate` URL)
 * @return {string} the PEM certificate chain (leaf first)
 * @throws {Error} when the order has no certificate yet
 */
export func downloadCertificate(client as Client, order as Order) {
    if (len($order.certificate) == 0) {
        throw Error{kind: "acme", message: "acme.downloadCertificate: order has no certificate URL yet", file: "", line: 0, col: 0};
    }
    def resp as http.Response init jwsOk($client, $order.certificate, "", false);
    return $resp.body;
}
