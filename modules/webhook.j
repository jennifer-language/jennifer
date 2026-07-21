# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

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
 * def sig as string init webhook.sign("{\"event\":\"ping\"}", "topsecret");
 * def ok as bool init webhook.verify("{\"event\":\"ping\"}", $sig, "topsecret");
 */
use hash;
use crypto;
use encoding;
use convert;
use strings;
import "./http.j" as http;

# The GitHub-convention signature header carrying the sha256= HMAC.
def const HEADER as string init "X-Hub-Signature-256";

# --- sign / verify (pure) ---------------------------------------------------

# hexMac is the lowercase-hex HMAC-SHA256 of payload keyed by secret.
func hexMac(payload as string, secret as string) {
    def mac as bytes init hash.hmac(convert.bytesFromString($secret, "utf-8"),
        convert.bytesFromString($payload, "utf-8"), "sha256");
    return encoding.toText($mac, "hex");
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
# timing, how many leading characters matched. Delegates to crypto.hmacEqual
# (Go's vetted subtle.ConstantTimeCompare) over the UTF-8 bytes.
func equalConstantTime(a as string, b as string) {
    return crypto.hmacEqual(convert.bytesFromString($a, "utf-8"),
        convert.bytesFromString($b, "utf-8"));
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
    return equalConstantTime(sign($payload, $secret), strings.lower($signature));
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
