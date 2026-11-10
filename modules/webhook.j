# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.25.0

/**
 * Sign and verify HMAC-signed webhooks - the GitHub-style `X-Hub-Signature-256`
 * convention. A sender computes `sha256=<hex>`, the hex HMAC-SHA256 of the exact
 * request body keyed by a shared secret, and sends it in a header; a receiver
 * recomputes it and compares, confirming the delivery is authentic and
 * untampered. `sign` / `verify` are pure and run on **both** binaries; `send`
 * POSTs a payload with the signature header and needs the default binary (`net`
 * via `http`).
 * @module webhook
 * @example
 * def sig as string init webhook.sign('{"event":"ping"}', "topsecret");
 * def ok as bool init webhook.verify('{"event":"ping"}', $sig, "topsecret");
 */
use hash;
use encoding;
use convert;
use strings;
use math;
import "./http.j" as http;

# The GitHub-convention signature header carrying the sha256= HMAC.
def const HEADER as string init "X-Hub-Signature-256";

# --- sign / verify (pure) ---------------------------------------------------

# checkSecret refuses an empty secret: a key of "" (an unset env variable,
# typically) makes every signature computable by anyone, and verify would pass.
func checkSecret(secret as string) {
    if ($secret == "") {
        throw Error{kind: "webhook", message: "webhook: the signing secret is empty"};
    }
}

# hexMac is the lowercase-hex HMAC-SHA256 of payload keyed by secret.
func hexMac(payload as string, secret as string) {
    checkSecret($secret);
    def mac as bytes init hash.hmac(
        "sha256",
        convert.bytesFromString("utf-8", $payload),
        convert.bytesFromString("utf-8", $secret));
    return encoding.toText("hex", $mac);
}

/**
 * Sign a payload: `sha256=` followed by the hex HMAC-SHA256 of the payload keyed
 * by the shared secret - the value a receiver checks in `X-Hub-Signature-256`.
 * @param payload {string} the exact request body
 * @param secret {string} the shared secret
 * @return {string} the signature, e.g. "sha256=757107ea..."
 */
export func sign(payload as string, secret as string) {
    return "sha256=" + hexMac($payload, $secret);
}

# equalConstantTime compares two strings for equality without leaking, through
# timing, how many leading characters matched. Delegates to hash.equal
# (Go's vetted subtle.ConstantTimeCompare) over the UTF-8 bytes.
func equalConstantTime(a as string, b as string) {
    return hash.equal(convert.bytesFromString("utf-8", $a), convert.bytesFromString("utf-8", $b));
}

/**
 * Verify a signature against a payload and secret, with a constant-time compare.
 * @param payload {string} the exact request body received
 * @param signature {string} the received signature, e.g. "sha256=..."
 * @param secret {string} the shared secret
 * @return {bool} true if the signature is valid
 */
export func verify(payload as string, signature as string, secret as string) {
    # Normalize case before the compare: the hex digest and the `sha256=`
    # prefix are case-insensitive, so a signature sent as `SHA256=...` or with
    # uppercase hex is still valid. sign() always emits lowercase.
    def sig as string init strings.lower($signature);
    # Bail before the HMAC: a valid signature is always exactly "sha256=" (7)
    # plus a 64-char hex digest, so any other length is malformed and never
    # pays for a keyed hash it cannot match.
    if (len($sig) != 71) {
        return false;
    }
    return equalConstantTime(sign($payload, $secret), $sig);
}

# --- timestamped, replay-protected schemes (pure) ---------------------------

# macText is the encoded HMAC of base keyed by secret: algo picks the digest
# ("sha1" / "sha256"), enc picks the text encoding ("hex" / "base64"). The
# composable core the Stripe / Slack / generic schemes below all build on.
func macText(secret as string, base as string, algo as string, enc as string) {
    checkSecret($secret);
    def mac as bytes init hash.hmac(
        $algo,
        convert.bytesFromString("utf-8", $base),
        convert.bytesFromString("utf-8", $secret));
    return encoding.toText($enc, $mac);
}

# fresh is the replay defence: the delivery is fresh only if its timestamp is
# within toleranceSeconds of now (in either direction), so a captured request
# replayed after the window is rejected even with a still-valid signature.
# The timestamp comes from the request, so it is range-checked first: outside
# 0..MAX_TIMESTAMP (year ~36812) it is stale, and the subtraction cannot overflow.
def const MAX_TIMESTAMP as int init 1099511627776;

func fresh(now as int, timestamp as int, tolerance as int) {
    if ($timestamp < 0 or $timestamp > MAX_TIMESTAMP or $now < 0 or $now > MAX_TIMESTAMP) {
        return false;
    }
    return math.abs($now - $timestamp) <= $tolerance;
}

# parseIntOr returns convert.toInt(s), or fallback when s is not a valid int
# (a malformed timestamp in an attacker-controlled header must not throw).
func parseIntOr(s as string, fallback as int) {
    def out as int init $fallback;
    try {
        $out = convert.toInt($s);
    } catch (e) {
        return $fallback;
    }
    return $out;
}

/**
 * Sign a payload the Stripe way: the signed base string is
 * `<timestamp> + "." + <body>`, HMAC-SHA256, hex-encoded, and the returned
 * header value is `t=<timestamp>,v1=<hexsig>` (the `Stripe-Signature` shape).
 * @param secret {string} the endpoint signing secret
 * @param body {string} the exact request body
 * @param timestamp {int} the unix timestamp (seconds) to stamp and sign
 * @return {string} the header value, e.g. "t=1492774800,v1=5257a8..."
 */
export func stripeSign(secret as string, body as string, timestamp as int) {
    def ts as string init convert.toString($timestamp);
    def sig as string init macText($secret, $ts + "." + $body, "sha256", "hex");
    return "t=" + $ts + ",v1=" + $sig;
}

/**
 * Verify a Stripe `t=...,v1=...` signature header against a body and secret.
 * Parses the timestamp and the (possibly several) `v1=` signatures, recomputes
 * the HMAC-SHA256 over `<t>.<body>`, and constant-time compares. Returns true
 * only if a signature matches AND the timestamp is within toleranceSeconds of
 * `now` (replay protection).
 * @param secret {string} the endpoint signing secret
 * @param body {string} the exact request body received
 * @param header {string} the received `t=...,v1=...` header value
 * @param toleranceSeconds {int} the freshness window in seconds (e.g. 300)
 * @param now {int} the current unix time (seconds)
 * @return {bool} true if a signature is valid and the timestamp is fresh
 */
export func stripeVerify(
    secret as string,
    body as string,
    header as string,
    toleranceSeconds as int,
    now as int) {
    def haveT as bool init false;
    def tstr as string init "";
    def sigs as list of string init [];
    def parts as list of string init strings.split($header, ",");
    for (def p in $parts) {
        def part as string init strings.trim($p);
        if (strings.startsWith($part, "t=")) {
            $tstr = $part[2..];
            $haveT = true;
        } elseif (strings.startsWith($part, "v1=")) {
            $sigs[] = $part[3..];
        }
    }
    if (not $haveT) {
        return false;
    }
    if (len($sigs) == 0) {
        return false;
    }
    # The MAC covers the timestamp string exactly as it appears in the header.
    def expected as string init macText($secret, $tstr + "." + $body, "sha256", "hex");
    def matched as bool init false;
    for (def s in $sigs) {
        # No early return: compare every candidate so timing does not reveal
        # which (or how many) signatures were present.
        if (equalConstantTime($expected, $s)) {
            $matched = true;
        }
    }
    def ts as int init parseIntOr($tstr, -1);
    return $matched and fresh($now, $ts, $toleranceSeconds);
}

/**
 * Sign a payload the Slack way: the signed base string is
 * `"v0:" + <timestamp> + ":" + <body>`, HMAC-SHA256, hex-encoded, and the
 * returned value is `v0=<hexsig>` (the `X-Slack-Signature` shape). The
 * timestamp travels separately in `X-Slack-Request-Timestamp`.
 * @param secret {string} the Slack signing secret
 * @param body {string} the exact request body
 * @param timestamp {int} the unix timestamp (seconds), the request timestamp
 * @return {string} the signature, e.g. "v0=a2114d57..."
 */
export func slackSign(secret as string, body as string, timestamp as int) {
    def base as string init "v0:{$timestamp}:{$body}";
    return "v0=" + macText($secret, $base, "sha256", "hex");
}

/**
 * Verify a Slack `v0=...` signature against a body, secret, and the request
 * timestamp (from `X-Slack-Request-Timestamp`). Recomputes the HMAC-SHA256
 * over `"v0:" + <timestamp> + ":" + <body>` and constant-time compares.
 * Returns true only if the signature matches AND the timestamp is within
 * toleranceSeconds of `now` (replay protection, e.g. 300s).
 * @param secret {string} the Slack signing secret
 * @param body {string} the exact request body received
 * @param timestamp {int} the request timestamp (seconds) from the header
 * @param signature {string} the received `v0=...` header value
 * @param toleranceSeconds {int} the freshness window in seconds (e.g. 300)
 * @param now {int} the current unix time (seconds)
 * @return {bool} true if the signature is valid and the timestamp is fresh
 */
export func slackVerify(
    secret as string,
    body as string,
    timestamp as int,
    signature as string,
    toleranceSeconds as int,
    now as int) {
    def expected as string init slackSign($secret, $body, $timestamp);
    def matched as bool init equalConstantTime($expected, strings.lower($signature));
    return $matched and fresh($now, $timestamp, $toleranceSeconds);
}

/**
 * Generic timestamped HMAC signature: sign `<timestamp> + "." + <body>` with a
 * caller-chosen digest and text encoding. Covers GitHub-style `sha1`/`sha256`
 * and hex/base64 schemes from one composable primitive.
 * @param algo {string} the HMAC digest: "sha1" or "sha256"
 * @param encoding {string} the text encoding: "hex" or "base64"
 * @param secret {string} the shared secret
 * @param body {string} the exact request body
 * @param timestamp {int} the unix timestamp (seconds) to stamp and sign
 * @return {string} the encoded signature (no scheme prefix)
 */
export func timestampedSign(
    algo as string,
    encoding as string,
    secret as string,
    body as string,
    timestamp as int) {
    def base as string init "{$timestamp}.{$body}";
    return macText($secret, $base, $algo, $encoding);
}

/**
 * Verify a generic timestamped HMAC signature (see `timestampedSign`) with a
 * constant-time compare and a replay-protecting freshness check. Returns true
 * only if the signature matches AND the timestamp is within toleranceSeconds of
 * `now`.
 * @param algo {string} the HMAC digest: "sha1" or "sha256"
 * @param encoding {string} the text encoding: "hex" or "base64"
 * @param secret {string} the shared secret
 * @param body {string} the exact request body received
 * @param timestamp {int} the request timestamp (seconds)
 * @param signature {string} the received signature (encoded, no prefix)
 * @param toleranceSeconds {int} the freshness window in seconds
 * @param now {int} the current unix time (seconds)
 * @return {bool} true if the signature is valid and the timestamp is fresh
 */
export func timestampedVerify(
    algo as string,
    encoding as string,
    secret as string,
    body as string,
    timestamp as int,
    signature as string,
    toleranceSeconds as int,
    now as int) {
    def expected as string init timestampedSign($algo, $encoding, $secret, $body, $timestamp);
    def matched as bool init equalConstantTime($expected, $signature);
    return $matched and fresh($now, $timestamp, $toleranceSeconds);
}

# --- send (needs the default binary) ----------------------------------------

/**
 * POST a payload to a webhook URL with the `X-Hub-Signature-256` header set,
 * returning the receiver's HTTP response. The body is sent as
 * `application/json` (the common webhook content type). Inspecting the result
 * needs `import "http.j"` for the `http.Response` type.
 * @param url {string} the receiver URL
 * @param payload {string} the request body
 * @param secret {string} the shared secret
 * @return {http.Response} the receiver's response (status / headers / body)
 * @throws {Error} on a network failure (a positioned `http` / `net` error)
 */
export func send(url as string, payload as string, secret as string) {
    def headers as map of string to string init {};
    $headers[HEADER] = sign($payload, $secret);
    return http.post($url, "application/json", $payload, $headers);
}
