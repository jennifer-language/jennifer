# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.28.0

/**
 * An ACME (RFC 8555) client: obtain and renew TLS certificates from Let's
 * Encrypt and compatible CAs. It drives the full flow - account registration, a
 * new order, HTTP-01 or DNS-01 challenge, CSR finalize, and certificate download
 * - over `http` + `json`, with every request a JWS signed by the account key
 * (`RS256` for an RSA key; `ES256` / `ES384` / `ES512` for an EC key, by curve).
 * Keys and the CSR come from the `crypto` library, so this needs the default
 * `jennifer` binary.
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
 *
 * A CA's refusal or a malformed answer throws an `acme.AcmeError` (kind
 * `"acme"`); a socket failure, a timeout or a reply over `http`'s cap goes on as
 * `http` threw it (kind `"net"`, `"timeout"`, `"limit"`, ...). A CA's error
 * answer carries its RFC 7807 problem document: the `problemType`
 * (`urn:ietf:params:acme:error:rateLimited`, `...:unauthorized`, ...), the
 * `detail`, the HTTP `status`, the whole `problem` (with `subproblems`), and
 * `retryAfter`, the seconds a rate-limited client must wait. An answer that is
 * not the JSON object RFC 8555 defines (not JSON, a required member missing or
 * of the wrong type, an unknown status) is an `AcmeError` too.
 *
 * Every request goes over `https://` (RFC 8555 6.1): a directory or endpoint
 * URL with `http://` to a non-loopback host throws an `AcmeError` before
 * anything is sent, unless the client comes from `connectWith` with
 * `allowInsecureHttp` set (a test CA on a trusted network only). `http://` to
 * `localhost`, `127.0.0.0/8` or `[::1]` is always allowed.
 * @module acme
 * @example
 * use crypto;
 * import "acme.j" as acme;
 *
 * def key as bytes init crypto.keypair("p256").private;
 * def client as acme.Client init acme.connect(
 *     "https://acme-staging-v02.api.letsencrypt.org/directory", $key);
 * $client = acme.register($client, "you@example.com");
 * def order as acme.Order init acme.order($client, ["example.com"]);
 */
import "./http.j" as http;
use json;
use crypto;
use hash;
use encoding;
use convert;
use regex;
use strings;
use lists;
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
 * @field alg {string} the JWS algorithm for the key: `RS256` for RSA; `ES256` / `ES384` / `ES512` for EC, by curve
 * @field kid {string} the account URL (empty until `register`)
 * @field allowInsecureHttp {bool} allow `http://` to a non-loopback host (set by `connectWith`)
 */
export def struct Client {
    directory as string,
    newNonce as string,
    newAccount as string,
    newOrder as string,
    accountKey as bytes,
    alg as string,
    kid as string,
    allowInsecureHttp as bool
};

/**
 * Options for `connectWith`.
 * @field allowInsecureHttp {bool} allow `http://` to a non-loopback test CA (default false)
 */
export def struct Options {
    allowInsecureHttp as bool
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
    return strings.replace(encoding.toText("base64-url", $b), "=", "");
}
# hexQuad renders n as four lowercase hex digits (for a \uXXXX JSON escape).
func hexQuad(n as int) {
    def digits as string init "0123456789abcdef";
    def out as string init "";
    for (def shift as int init 12; $shift >= 0; $shift = $shift - 4) {
        def nibble as int init ($n >> $shift) & 15;
        $out = $out + $digits[$nibble..$nibble + 1];
    }
    return $out;
}

# jsonEsc escapes a string for embedding in a hand-built JSON literal, per
# RFC 8259: the two structural characters plus every control character
# (< 0x20), which are otherwise invalid in a JSON string and could break the
# JWS protected-header JSON (e.g. a hostile CA nonce carrying a quote or newline).
func jsonEsc(s as string) {
    def out as list of string init [];
    for (def c in strings.chars($s)) {
        def cp as int init convert.toCodepoint($c);
        if ($c == "\\") {
            $out[] = "\\\\";
        } elseif ($c == "\"") {
            $out[] = "\\\"";
        } elseif ($cp == 10) {
            $out[] = "\\n";
        } elseif ($cp == 13) {
            $out[] = "\\r";
        } elseif ($cp == 9) {
            $out[] = "\\t";
        } elseif ($cp == 8) {
            $out[] = "\\b";
        } elseif ($cp == 12) {
            $out[] = "\\f";
        } elseif ($cp < 32) {
            $out[] = "\\u" + hexQuad($cp);
        } else {
            $out[] = $c;
        }
    }
    return strings.join($out, "");
}

# ---- JWS request signing ----

# fetchNonce gets a fresh anti-replay nonce (a HEAD to the newNonce endpoint).
func fetchNonce(client as Client) {
    requireHttps($client.newNonce, $client.allowInsecureHttp);
    def resp as http.Response init http.head($client.newNonce, {});
    def nonce as string init http.header($resp, "Replay-Nonce");
    if (len($nonce) == 0) {
        throw acmeError("acme: no Replay-Nonce from " + $client.newNonce, 0);
    }
    # RFC 8555 nonces are base64url tokens. Reject anything else outright, so a
    # hostile or buggy CA cannot feed a crafted value toward the JWS header (the
    # header also JSON-escapes it - defence in depth).
    if (not isNonceSafe($nonce)) {
        throw acmeError("acme: malformed Replay-Nonce (not base64url)", 0);
    }
    return $nonce;
}

# isNonceSafe reports whether every character of s is in the base64url alphabet
# (A-Z a-z 0-9 - _), i.e. the shape of an ACME nonce / JOSE token.
func isNonceSafe(s as string) {
    if (len($s) == 0) {
        return false;
    }
    return regex.matches('\A[A-Za-z0-9_-]+\z', $s);
}

# signAlgo maps a JOSE alg to its crypto.sign algorithm name. The suffix is the
# hash width, so ES384 -> "ecdsa-sha384", RS512 -> "rsa-pkcs1-sha512", and so on.
func signAlgo(alg as string) {
    def family as string init "ecdsa-";
    if (strings.startsWith($alg, "RS")) {
        $family = "rsa-pkcs1-";
    }
    if (strings.endsWith($alg, "384")) {
        return $family + "sha384";
    }
    if (strings.endsWith($alg, "512")) {
        return $family + "sha512";
    }
    return $family + "sha256";
}

func signInput(client as Client, input as bytes) {
    return crypto.sign(signAlgo($client.alg), $input, $client.accountKey);
}

# jws POSTs a signed request to url. `payload` is the JSON body ("" for the
# POST-as-GET form). `useJwk` embeds the account public JWK (account creation);
# otherwise the account `kid` is used. Returns the http.Response.
func jws(client as Client, url as string, payload as string, useJwk as bool) {
    requireHttps($url, $client.allowInsecureHttp);
    def nonce as string init fetchNonce($client);
    def protected as string init '{"alg":"' + $client.alg + "\",\"nonce\":\"" + jsonEsc($nonce) +
        "\",\"url\":\"" + jsonEsc($url) + "\"";
    if ($useJwk) {
        $protected = $protected + ",\"jwk\":" + crypto.jwkPublic($client.accountKey);
    } else {
        $protected = $protected + ",\"kid\":\"" + jsonEsc($client.kid) + "\"";
    }
    $protected = $protected + '}';
    def head as string init encodeSeg(encoding.encode("utf-8", $protected));
    def body as string init encodeSeg(encoding.encode("utf-8", $payload));
    def signingInput as string init $head + "." + $body;
    def sig as bytes init signInput($client, encoding.encode("utf-8", $signingInput));
    def jwsBody as string init '{"protected":"' + $head + "\",\"payload\":\"" + $body +
        "\",\"signature\":\"" + encodeSeg($sig) + '"}';
    return http.post($url, "application/jose+json", $jwsBody, {});
}

# PROBLEM_MAX caps how much of a CA's error body an exception quotes.
def const PROBLEM_MAX as int init 1024;

/**
 * A failed ACME operation. Every error of kind `"acme"` is one, so its fields
 * are safe to read once the kind matches.
 * @field kind {string} always `"acme"`
 * @field message {string} what failed, with the status and the start of the CA's answer
 * @field status {int} the HTTP status of the CA's answer; 0 when there was none
 * @field problemType {string} the problem document's `type` (`urn:ietf:params:acme:error:rateLimited`, `...:badCSR`, `...:unauthorized`, ...), "" when the answer had none
 * @field detail {string} the problem document's `detail`, "" when none
 * @field retryAfter {int} the `Retry-After` header in seconds (a rate limit, or a pending order); 0 when absent or given as a date
 * @field problem {json.Value} the whole problem document (with `subproblems` and `identifier`); JSON `null` when the answer was not one
 * @example
 * use io;
 * use strings;
 *
 * try {
 *     $order = acme.order($client, ["example.com"]);
 * } catch (e) {
 *     if ($e.kind == "acme" and strings.endsWith($e.problemType, ":rateLimited")) {
 *         io.printf("rate limited; retry in %d s\n", $e.retryAfter);
 *     } else {
 *         throw $e;
 *     }
 * }
 */
export def struct AcmeError {
    kind as string,
    message as string,
    status as int,
    problemType as string,
    detail as string,
    retryAfter as int,
    problem as json.Value
};

# acmeError is the AcmeError of a failure without a problem document.
func acmeError(message as string, status as int) {
    return AcmeError{
        kind: "acme",
        message: $message,
        status: $status,
        problemType: "",
        detail: "",
        retryAfter: 0,
        problem: json.decode("null")
    };
}

# problemError is the AcmeError of a CA's error answer: its problem document
# (when the body is one) and its Retry-After seconds.
func problemError(message as string, resp as http.Response) {
    def err as AcmeError init acmeError($message, $resp.status);
    try {
        def doc as json.Value init json.decode($resp.body);
        if (json.typeOf($doc, "") == "map") {
            $err.problem = $doc;
            if (json.typeOf($doc, "/type") == "string") {
                $err.problemType = json.asString($doc, "/type");
            }
            if (json.typeOf($doc, "/detail") == "string") {
                $err.detail = json.asString($doc, "/detail");
            }
        }
    } catch (e) {
        $err.problem = json.decode("null");
    }
    for (def k in $resp.headers) {
        if (strings.lower($k) == "retry-after") {
            def ra as string init strings.trim($resp.headers[$k]);
            if (regex.matches('^[0-9]{1,9}$', $ra)) {
                $err.retryAfter = convert.toInt($ra);
            }
        }
    }
    return $err;
}

# requireHttps refuses a URL that would carry an ACME request in the clear:
# RFC 8555 6.1 requires HTTPS, so `http://` is allowed only to a loopback host
# or with the client's explicit opt-in. The message leaves the URL out.
func requireHttps(url as string, allowInsecureHttp as bool) {
    if (not $allowInsecureHttp and http.isCleartextRemote($url)) {
        throw acmeError(
            "acme: refusing plain http:// to a non-loopback host (RFC 8555 requires https; " +
                "Options.allowInsecureHttp allows it for a test CA)",
            0);
    }
}

# decodeAnswer parses a CA's JSON answer: one that is not a JSON object is an
# AcmeError naming `what`; json's decoding caps stay kind "limit".
func decodeAnswer(body as string, what as string, status as int) {
    def v as json.Value;
    try {
        $v = json.decode($body);
    } catch (e) {
        if ($e.kind == "limit") {
            throw $e;
        }
        throw acmeError("acme: the " + $what + " is not JSON", $status);
    }
    if (json.typeOf($v, "") != "map") {
        throw acmeError("acme: the " + $what + " is not a JSON object", $status);
    }
    return $v;
}

# member reads a required non-empty string member of a CA's answer.
func member(v as json.Value, ptr as string, what as string, status as int) {
    if (json.typeOf($v, $ptr) != "string" or json.asString($v, $ptr) == "") {
        throw acmeError("acme: the " + $what + " has no string " + $ptr, $status);
    }
    return json.asString($v, $ptr);
}

# oneOf checks a status member against the values RFC 8555 7.1.6 defines.
func oneOf(value as string, allowed as list of string, what as string, status as int) {
    if (not lists.contains($allowed, $value)) {
        throw acmeError(
            "acme: the " + $what + " has an unknown status " + jsonQuote($value),
            $status);
    }
    return $value;
}

# jsonQuote shows a CA-supplied value in a message, quoted and escaped, cut
# to 64 characters.
func jsonQuote(s as string) {
    def shown as string init $s;
    if (len($shown) > 64) {
        $shown = $shown[..64] + "...";
    }
    return '"' + jsonEsc($shown) + '"';
}

# BAD_NONCE is the problem type of an expired or unknown nonce.
def const BAD_NONCE as string init "urn:ietf:params:acme:error:badNonce";

# jwsOk is jws plus an error check: an ACME error response (>= 400) carries an
# RFC 7807 problem document, which is surfaced as a catchable error. A
# `badNonce` problem is retried once with a fresh nonce (RFC 8555 6.5): CAs
# expire nonces at will, so a client is expected to retry it.
func jwsOk(client as Client, url as string, payload as string, useJwk as bool) {
    def resp as http.Response init jws($client, $url, $payload, $useJwk);
    if ($resp.status == 400 and strings.contains($resp.body, BAD_NONCE)) {
        $resp = jws($client, $url, $payload, $useJwk);
    }
    if ($resp.status >= 400) {
        def problem as string init $resp.body;
        if (len($problem) > PROBLEM_MAX) {
            $problem = $problem[..PROBLEM_MAX] + "...";
        }
        throw problemError(
            "acme: request to " + $url + " failed ({$resp.status}): " + $problem,
            $resp);
    }
    return $resp;
}

# ---- account + directory ----

# algForKey picks the JWS algorithm for an account key from its public JWK:
# RSA -> RS256; EC by curve -> ES256 (P-256) / ES384 (P-384) / ES512 (P-521).
# The curve *must* drive the ES alg - JOSE binds ES256 to P-256, ES384 to P-384,
# ES512 to P-521, so a P-384 key signed as "ES256" is a malformed JWS the CA
# rejects. An unrecognised key type is an error rather than a silent wrong guess.
func algForKey(accountKey as bytes) {
    def jwk as string init crypto.jwkPublic($accountKey);
    if (strings.contains($jwk, "\"kty\":\"RSA\"")) {
        return "RS256";
    }
    if (strings.contains($jwk, "\"crv\":\"P-256\"")) {
        return "ES256";
    }
    if (strings.contains($jwk, "\"crv\":\"P-384\"")) {
        return "ES384";
    }
    if (strings.contains($jwk, "\"crv\":\"P-521\"")) {
        return "ES512";
    }
    throw acmeError("acme: unsupported account key type (want RSA or EC P-256/P-384/P-521)", 0);
}

/**
 * Build a client from a CA directory URL and an account private key (PEM
 * `bytes`, RSA or EC - the `private` half of `crypto.keypair("rsa2048")` or
 * `crypto.keypair("p256")`). Fetches the directory to learn the CA's endpoints.
 * Does not yet register.
 * @param directoryUrl {string} the CA's ACME directory URL
 * @param accountKey {bytes} the account private key, PEM-encoded
 * @return {Client} the configured client (kid empty until `register`)
 * @throws {AcmeError} on a refused or malformed directory, or `http://` to a non-loopback host
 * (a socket failure is http's error, kind "net")
 */
export func connect(directoryUrl as string, accountKey as bytes) {
    def opts as Options;
    return connectWith($directoryUrl, $accountKey, $opts);
}

/**
 * `connect` with options: `allowInsecureHttp` lets the client talk plain
 * `http://` to a non-loopback test CA (RFC 8555 requires https; never set it
 * for a real CA - anyone on the path could then redirect the requests).
 * @param directoryUrl {string} the CA's ACME directory URL
 * @param accountKey {bytes} the account private key, PEM-encoded
 * @param options {Options} the connection options
 * @return {Client} the configured client (kid empty until `register`)
 * @throws {AcmeError} as `connect`
 * @example
 *   use crypto;
 *   import "acme.j" as acme;
 *
 *   def key as bytes init crypto.keypair("p256").private;
 *   def client as acme.Client init acme.connectWith(
 *       "http://ca.lab.internal:4001/directory", $key,
 *       acme.Options{allowInsecureHttp: true});
 */
export func connectWith(directoryUrl as string, accountKey as bytes, options as Options) {
    requireHttps($directoryUrl, $options.allowInsecureHttp);
    def alg as string init algForKey($accountKey);
    def resp as http.Response init http.get($directoryUrl, {});
    if ($resp.status != 200) {
        throw problemError("acme.connect: directory fetch failed ({$resp.status})", $resp);
    }
    def dir as json.Value init decodeAnswer($resp.body, "directory", $resp.status);
    def out as Client init Client{
        directory: $directoryUrl,
        newNonce: member($dir, "/newNonce", "directory", $resp.status),
        newAccount: member($dir, "/newAccount", "directory", $resp.status),
        newOrder: member($dir, "/newOrder", "directory", $resp.status),
        accountKey: $accountKey,
        alg: $alg,
        kid: "",
        allowInsecureHttp: $options.allowInsecureHttp
    };
    for (def u in [$out.newNonce, $out.newAccount, $out.newOrder]) {
        requireHttps($u, $out.allowInsecureHttp);
    }
    return $out;
}

/**
 * Register (or look up) the account with the CA and return a client carrying the
 * account URL (`kid`). Agrees to the CA's terms of service. Idempotent: the CA
 * returns the existing account for a known key.
 * @param client {Client} a client from `connect`
 * @param email {string} a contact email (empty for none)
 * @return {Client} a client with `kid` set
 * @throws {AcmeError} on a registration error
 */
export func register(client as Client, email as string) {
    def payload as string init '{"termsOfServiceAgreed":true';
    if (len($email) > 0) {
        $payload = $payload + ",\"contact\":[\"mailto:" + jsonEsc($email) + "\"]";
    }
    $payload = $payload + '}';
    def resp as http.Response init jwsOk($client, $client.newAccount, $payload, true);
    def kid as string init http.header($resp, "Location");
    if (len($kid) == 0) {
        throw acmeError("acme.register: no account Location header", 0);
    }
    def out as Client init $client;
    $out.kid = $kid;
    return $out;
}

# ---- orders + authorizations ----

# parseOrder builds an Order from a response body + its order URL.
# Every member RFC 8555 7.1.3 requires must be there with its type; an
# unknown status is refused, so a poll loop never waits on a value it cannot
# recognise.
func parseOrder(url as string, resp as http.Response) {
    def v as json.Value init decodeAnswer($resp.body, "order", $resp.status);
    def status as string init oneOf(
        member($v, "/status", "order", $resp.status),
        ["pending", "ready", "processing", "valid", "invalid"],
        "order",
        $resp.status);
    if (json.typeOf($v, "/authorizations") != "list") {
        throw acmeError("acme: the order has no /authorizations list", $resp.status);
    }
    def authz as list of string init [];
    def n as int init json.length($v, "/authorizations");
    for (def i in 0..$n) {
        $authz[] = member($v, "/authorizations/{$i}", "order", $resp.status);
    }
    def cert as string init "";
    if (json.has($v, "/certificate")) {
        $cert = member($v, "/certificate", "order", $resp.status);
    }
    return Order{
        url: $url,
        status: $status,
        authorizations: $authz,
        finalize: member($v, "/finalize", "order", $resp.status),
        certificate: $cert
    };
}

/**
 * Create an order for one or more domains. The returned order is `pending` with
 * one authorization URL per domain.
 * @param client {Client} a registered client
 * @param domains {list of string} the domains to certify (the first is the CN)
 * @return {Order} the new order
 * @throws {AcmeError} on an order error, or a response without the order URL
 */
export func order(client as Client, domains as list of string) {
    def ids as list of string init [];
    for (def d in $domains) {
        $ids[] = '{"type":"dns","value":"' + jsonEsc($d) + '"}';
    }
    def payload as string init '{"identifiers":[' + strings.join($ids, ",") + ']}';
    def resp as http.Response init jwsOk($client, $client.newOrder, $payload, false);
    def url as string init http.header($resp, "Location");
    if (len($url) == 0) {
        throw acmeError("acme.order: no order Location header", 0);
    }
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
    return parseAuthorization($resp.body, $resp.status);
}

# parseAuthorization reads an authorization object. A challenge token must be
# base64url (RFC 8555 8.3): the caller serves it as a file name under
# `/.well-known/acme-challenge/`, so a `/` or `..` from the CA is refused. The
# members RFC 8555 7.1.4 / 8 require must be there with their types, and the
# statuses must be ones it defines.
func parseAuthorization(body as string, status as int) {
    def v as json.Value init decodeAnswer($body, "authorization", $status);
    if (json.typeOf($v, "/challenges") != "list") {
        throw acmeError("acme: the authorization has no /challenges list", $status);
    }
    def challenges as list of Challenge init [];
    def n as int init json.length($v, "/challenges");
    for (def i in 0..$n) {
        def base as string init "/challenges/{$i}";
        if (json.typeOf($v, $base) != "map") {
            throw acmeError("acme: the authorization's challenge {$i} is not an object", $status);
        }
        def token as string init "";
        if (json.typeOf($v, $base + "/token") == "string") {
            $token = json.asString($v, $base + "/token");
        }
        if (not isNonceSafe($token)) {
            throw acmeError("acme.authorization: challenge token is not base64url", $status);
        }
        $challenges[] = Challenge{
            kind: member($v, $base + "/type", "authorization", $status),
            url: member($v, $base + "/url", "authorization", $status),
            token: $token,
            status: oneOf(
                member($v, $base + "/status", "authorization", $status),
                ["pending", "processing", "valid", "invalid"],
                "challenge",
                $status)
        };
    }
    return Authorization{
        domain: member($v, "/identifier/value", "authorization", $status),
        status: oneOf(
            member($v, "/status", "authorization", $status),
            ["pending", "valid", "invalid", "deactivated", "expired", "revoked"],
            "authorization",
            $status),
        challenges: $challenges
    };
}

# failedAuthorization is the AcmeError of an authorization that settled other
# than `valid`. RFC 8555 8 puts the reason in the failed challenge's `error`
# problem document, which becomes the error's problem.
func failedAuthorization(body as string, a as Authorization) {
    def err as AcmeError init acmeError(
        "acme.pollAuthorization: the authorization for " + jsonQuote($a.domain) + " is " +
            $a.status,
        0);
    def v as json.Value init json.decode($body);
    def n as int init json.length($v, "/challenges");
    for (def i in 0..$n) {
        def ptr as string init "/challenges/{$i}/error";
        if (json.typeOf($v, $ptr) == "map") {
            $err.problem = json.get($v, $ptr);
            if (json.typeOf($v, $ptr + "/type") == "string") {
                $err.problemType = json.asString($v, $ptr + "/type");
            }
            if (json.typeOf($v, $ptr + "/detail") == "string") {
                $err.detail = json.asString($v, $ptr + "/detail");
                $err.message = $err.message + ": " + $err.detail;
            }
            return $err;
        }
    }
    return $err;
}

/**
 * The challenge of a given type within an authorization (`"http-01"` /
 * `"dns-01"`).
 * @param kind {string} the challenge type to select
 * @param authz {Authorization} an authorization from `authorization`
 * @return {Challenge} the matching challenge
 * @throws {AcmeError} when no challenge of that type is offered
 */
export func challenge(kind as string, authz as Authorization) {
    for (def c in $authz.challenges) {
        if ($c.kind == $kind) {
            return $c;
        }
    }
    throw acmeError("acme.challenge: no " + $kind + " challenge for " + $authz.domain, 0);
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
    def thumb as string init encodeSeg(hash.compute("sha256", encoding.encode("utf-8", $jwk)));
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
    return encodeSeg(hash.compute("sha256", encoding.encode("utf-8", $ka)));
}

# ---- accept + poll + finalize + download ----

/**
 * Tell the CA a challenge response is in place (a POST of `{}` to the challenge
 * URL). Do this only after publishing the HTTP-01 / DNS-01 response.
 * @param client {Client} a registered client
 * @param challengeUrl {string} the challenge URL (from `challenge`)
 * @return {null} nothing
 * @throws {AcmeError} on an accept error
 */
export func accept(client as Client, challengeUrl as string) {
    jwsOk($client, $challengeUrl, '{}', false);
    return;
}

/**
 * Poll an authorization until it leaves `pending`, waiting `intervalMs`
 * between attempts up to `maxTries`. It returns once the authorization is
 * `valid`; one that settles otherwise (`invalid`, `deactivated`, `expired`,
 * `revoked`) throws, with the failed challenge's `error` problem document
 * (why the CA could not validate) as the error's `problem`, `problemType` and
 * `detail`.
 * @param client {Client} a registered client
 * @param authzUrl {string} the authorization URL
 * @param intervalMs {int} milliseconds between polls
 * @param maxTries {int} maximum number of polls
 * @return {Authorization} the `valid` authorization
 * @throws {AcmeError} if it settles other than `valid`, or is still pending after `maxTries`
 */
export func pollAuthorization(
    client as Client,
    authzUrl as string,
    intervalMs as int,
    maxTries as int) {
    for (def i as int init 0; $i < $maxTries; $i = $i + 1) {
        def resp as http.Response init jwsOk($client, $authzUrl, "", false);
        def a as Authorization init parseAuthorization($resp.body, $resp.status);
        if ($a.status == "valid") {
            return $a;
        }
        if ($a.status != "pending") {
            throw failedAuthorization($resp.body, $a);
        }
        time.sleep(time.fromMilliseconds($intervalMs));
    }
    throw acmeError("acme.pollAuthorization: still pending after {$maxTries} tries", 0);
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
 * @throws {AcmeError} on a finalize error or if issuance does not complete
 */
export func finalize(
    client as Client,
    order as Order,
    csrDer as bytes,
    intervalMs as int,
    maxTries as int) {
    def payload as string init '{"csr":"' + encodeSeg($csrDer) + '"}';
    jwsOk($client, $order.finalize, $payload, false);
    for (def i as int init 0; $i < $maxTries; $i = $i + 1) {
        def o as Order init fetchOrder($client, $order.url);
        if ($o.status == "valid") {
            return $o;
        }
        if ($o.status == "invalid") {
            throw acmeError("acme.finalize: order became invalid", 0);
        }
        time.sleep(time.fromMilliseconds($intervalMs));
    }
    throw acmeError("acme.finalize: certificate not issued after {$maxTries} tries", 0);
}

/**
 * Download the issued certificate chain (PEM) for a valid order.
 * @param client {Client} a registered client
 * @param order {Order} a `valid` order (with a `certificate` URL)
 * @return {string} the PEM certificate chain (leaf first)
 * @throws {AcmeError} when the order has no certificate yet
 */
export func downloadCertificate(client as Client, order as Order) {
    if (len($order.certificate) == 0) {
        throw acmeError("acme.downloadCertificate: order has no certificate URL yet", 0);
    }
    def resp as http.Response init jwsOk($client, $order.certificate, "", false);
    return $resp.body;
}
