# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

/**
 * JSON Web Tokens (RFC 7519): sign, verify, and decode compact JWTs. Claims are
 * a `json.Value` object; the token is the usual `header.payload.signature` of
 * base64url segments. Ten algorithms across four families: HMAC
 * (`HS256` / `HS384` / `HS512`, over a shared secret), RSA PKCS#1 v1.5
 * (`RS256` / `RS384` / `RS512`), ECDSA (`ES256` / `ES384` / `ES512`), and
 * Ed25519 (`EdDSA`). The key is always `bytes`: an HMAC secret, a PEM-encoded
 * RSA / EC key, or a raw Ed25519 key from `crypto.signKeypair`.
 *
 * `verify` takes the algorithm you *expect* and rejects a token whose header
 * disagrees - this closes the classic JWT algorithm-confusion attack (a forged
 * `HS256` token verified with an RSA public key as the HMAC secret). It also
 * enforces the `exp` (expiry) and `nbf` (not-before) time claims when present.
 *
 * RS* / ES* need the `crypto` library's RSA / ECDSA surface, which is on the
 * default `jennifer` binary; HS* and `EdDSA` run on both binaries.
 * @module jwt
 * @example
 * use convert;
 * import "jwt.j" as jwt;
 * def claims as json.Value init json.decode("{\"sub\":\"ada\",\"exp\":9999999999}");
 * def secret as bytes init convert.bytesFromString("topsecret", "utf-8");
 * def token as string init jwt.sign($claims, $secret, "HS256");
 * def back as json.Value init jwt.verify($token, $secret, "HS256");
 */
use json;
use hash;
use crypto;
use encoding;
use time;
use strings;
use lists;
use convert;

# The algorithms this module accepts, by JOSE `alg` name.
def const SUPPORTED as list of string init [
    "HS256", "HS384", "HS512",
    "RS256", "RS384", "RS512",
    "ES256", "ES384", "ES512",
    "EdDSA"
];

# Policy carries the application-level claim checks jwt.verifyWith enforces on
# top of the cryptographic verification: the expected issuer (`iss`) and
# audience (`aud`). An empty string means "do not check this claim".
export def struct Policy {
    iss as string,
    aud as string
};

# ---- base64url (unpadded, as JWT requires) ----

# encodeSegment encodes bytes as base64url with the trailing "=" padding removed.
func encodeSegment(b as bytes) {
    # "=" appears only as padding in base64url output (it is not in the
    # alphabet), so stripping every "=" leaves the unpadded form JWT wants.
    return strings.replace(encoding.toText($b, "base64-url"), "=", "");
}
# decodeSegment restores the padding a JWT segment omits, then decodes it.
# The segment must be *canonical* unpadded base64url: after decoding, the bytes
# must re-encode to exactly the input. A lenient decoder would also accept a
# segment with stray "=" padding or non-zero trailing bits - a second spelling
# of the same token, which breaks anything keyed on the token string (replay
# caches, denylists) and is rejected by strict JWS implementations.
func decodeSegment(s as string) {
    def padded as string init $s;
    def r as int init len($s) % 4;
    if ($r == 2) {
        $padded = $s + "==";
    } elseif ($r == 3) {
        $padded = $s + "=";
    }
    def out as bytes init encoding.fromText($padded, "base64-url");
    if (encodeSegment($out) != $s) {
        throw Error{kind: "value", message: "jwt: non-canonical base64url segment", file: "", line: 0, col: 0};
    }
    return $out;
}

# ---- algorithm dispatch ----

func requireAlg(alg as string) {
    if (not lists.contains(SUPPORTED, $alg)) {
        throw Error{kind: "value", message: "jwt: unsupported algorithm " + $alg, file: "", line: 0, col: 0};
    }
}

# algHash maps a JOSE alg to its hash-library name (HS/RS/ES families); EdDSA
# carries its own hash internally and returns "".
func algHash(alg as string) {
    if (strings.endsWith($alg, "256")) {
        return "sha256";
    }
    if (strings.endsWith($alg, "384")) {
        return "sha384";
    }
    if (strings.endsWith($alg, "512")) {
        return "sha512";
    }
    return "";
}

func family(alg as string) {
    if (strings.startsWith($alg, "HS")) {
        return "hmac";
    }
    if (strings.startsWith($alg, "RS")) {
        return "rsa";
    }
    if (strings.startsWith($alg, "ES")) {
        return "ecdsa";
    }
    return "eddsa";
}

# computeSig produces the signature over the signing input for the algorithm.
func computeSig(alg as string, input as bytes, key as bytes) {
    def fam as string init family($alg);
    if ($fam == "hmac") {
        return hash.hmac($key, $input, algHash($alg));
    }
    if ($fam == "rsa") {
        return crypto.rsaSign($key, $input, algHash($alg));
    }
    if ($fam == "ecdsa") {
        return crypto.ecdsaSign($key, $input, algHash($alg));
    }
    return crypto.sign($key, $input);
}

# checkSig verifies a signature over the signing input. HMAC uses a constant-time
# compare so a bad MAC leaks nothing through timing.
func checkSig(alg as string, input as bytes, sig as bytes, key as bytes) {
    def fam as string init family($alg);
    if ($fam == "hmac") {
        return crypto.hmacEqual(hash.hmac($key, $input, algHash($alg)), $sig);
    }
    if ($fam == "rsa") {
        return crypto.rsaVerify($key, $input, $sig, algHash($alg));
    }
    if ($fam == "ecdsa") {
        return crypto.ecdsaVerify($key, $input, $sig, algHash($alg));
    }
    return crypto.verify($key, $input, $sig);
}

# ---- public surface ----

/**
 * Sign `claims` into a compact JWT with the given algorithm. The header is
 * `{"alg": alg, "typ": "JWT"}`; the payload is the encoded claims. `key` is the
 * HMAC secret, a PEM RSA / EC private key, or an Ed25519 private key, by family.
 * @param claims {json.Value} the claims object (the token payload)
 * @param key {bytes} the signing key (HMAC secret / PEM / Ed25519 private)
 * @param alg {string} the JOSE algorithm (e.g. "HS256", "RS256", "ES256", "EdDSA")
 * @return {string} the signed `header.payload.signature` token
 * @throws {Error} on an unsupported algorithm or a key the algorithm rejects
 */
export func sign(claims as json.Value, key as bytes, alg as string) {
    requireAlg($alg);
    # The alg comes from the validated whitelist, so this hand-built header JSON
    # cannot carry an injected value.
    def headerJson as string init "{\"alg\":\"" + $alg + "\",\"typ\":\"JWT\"}";
    def head as string init encodeSegment(convert.bytesFromString($headerJson, "utf-8"));
    def payload as string init encodeSegment(convert.bytesFromString(json.encode($claims), "utf-8"));
    def signingInput as string init $head + "." + $payload;
    def sig as bytes init computeSig($alg, convert.bytesFromString($signingInput, "utf-8"), $key);
    return $signingInput + "." + encodeSegment($sig);
}

/**
 * Verify a JWT and return its claims. Checks that the token's header algorithm
 * equals `alg` (rejecting algorithm-confusion), that the signature is valid for
 * `key`, and - when present - that the `exp` (expiry) and `nbf` (not-before)
 * NumericDate claims allow the token now.
 * @param token {string} the compact JWT
 * @param key {bytes} the verification key (HMAC secret / PEM public / Ed25519 public)
 * @param alg {string} the algorithm the caller requires (e.g. "HS256", "RS256")
 * @return {json.Value} the verified claims
 * @throws {Error} on a malformed token, an algorithm mismatch, a bad signature, or an expired / not-yet-valid token
 */
export func verify(token as string, key as bytes, alg as string) {
    requireAlg($alg);
    def parts as list of string init strings.split($token, ".");
    if (len($parts) != 3) {
        throw Error{kind: "value", message: "jwt.verify: malformed token (want three dot-separated segments)", file: "", line: 0, col: 0};
    }
    def head as json.Value init json.decode(convert.stringFromBytes(decodeSegment($parts[0]), "utf-8"));
    if (not json.has($head, "/alg") or json.asString($head, "/alg") != $alg) {
        throw Error{kind: "value", message: "jwt.verify: token algorithm does not match the expected " + $alg, file: "", line: 0, col: 0};
    }
    # RFC 7515 4.1.11: a verifier must reject a token carrying a `crit` (critical
    # header extensions) member it does not understand. This module understands
    # none, so any `crit` is a refusal - never silently ignore a critical header.
    if (json.has($head, "/crit")) {
        throw Error{kind: "value", message: "jwt.verify: token has an unsupported \"crit\" header", file: "", line: 0, col: 0};
    }
    def signingInput as string init $parts[0] + "." + $parts[1];
    def sig as bytes init decodeSegment($parts[2]);
    if (not checkSig($alg, convert.bytesFromString($signingInput, "utf-8"), $sig, $key)) {
        throw Error{kind: "value", message: "jwt.verify: signature verification failed", file: "", line: 0, col: 0};
    }
    def claims as json.Value init json.decode(convert.stringFromBytes(decodeSegment($parts[1]), "utf-8"));
    def now as int init time.unix(time.now());
    if (json.has($claims, "/exp") and $now >= json.asInt($claims, "/exp")) {
        throw Error{kind: "value", message: "jwt.verify: token has expired", file: "", line: 0, col: 0};
    }
    if (json.has($claims, "/nbf") and $now < json.asInt($claims, "/nbf")) {
        throw Error{kind: "value", message: "jwt.verify: token is not yet valid", file: "", line: 0, col: 0};
    }
    return $claims;
}

# audienceMatches reports whether the token's `aud` claim admits `want`. Per RFC
# 7519 4.1.3 `aud` is either a single string or an array of strings; a missing
# claim never matches.
func audienceMatches(claims as json.Value, want as string) {
    if (not json.has($claims, "/aud")) {
        return false;
    }
    def kind as string init json.typeOf($claims, "/aud");
    if ($kind == "string") {
        return json.asString($claims, "/aud") == $want;
    }
    if ($kind == "list") {
        def n as int init json.length($claims, "/aud");
        for (def i in lists.range(0, $n)) {
            if (json.asString($claims, "/aud/" + convert.toString($i)) == $want) {
                return true;
            }
        }
    }
    return false;
}

/**
 * Verify a JWT like `verify`, then additionally enforce the application-level
 * claim policy: when `policy.iss` is non-empty the token's `iss` must equal it,
 * and when `policy.aud` is non-empty the token's `aud` (a string or an array of
 * strings) must include it. `exp` / `nbf` and the signature are checked exactly
 * as in `verify`; issuer and audience are application policy the library cannot
 * assume, hence the explicit opt-in.
 * @param token {string} the compact JWT
 * @param key {bytes} the verification key (HMAC secret / PEM public / Ed25519 public)
 * @param alg {string} the algorithm the caller requires (e.g. "HS256", "RS256")
 * @param policy {Policy} the expected issuer / audience (empty string skips a check)
 * @return {json.Value} the verified claims
 * @throws {Error} on any `verify` failure, or a mismatched issuer / audience
 */
export func verifyWith(token as string, key as bytes, alg as string, policy as Policy) {
    def claims as json.Value init verify($token, $key, $alg);
    if ($policy.iss != "") {
        if (not json.has($claims, "/iss") or json.asString($claims, "/iss") != $policy.iss) {
            throw Error{kind: "value", message: "jwt.verifyWith: issuer (iss) does not match the expected " + $policy.iss, file: "", line: 0, col: 0};
        }
    }
    if ($policy.aud != "") {
        if (not audienceMatches($claims, $policy.aud)) {
            throw Error{kind: "value", message: "jwt.verifyWith: audience (aud) does not include the expected " + $policy.aud, file: "", line: 0, col: 0};
        }
    }
    return $claims;
}

/**
 * Decode a JWT's claims **without verifying** the signature or time claims - for
 * inspecting a token you do not trust yet (e.g. to read its `kid` / issuer
 * before fetching a key). Never trust these claims for authorization; use
 * `verify` for that.
 * @param token {string} the compact JWT
 * @return {json.Value} the payload claims, unverified
 * @throws {Error} on a malformed token
 */
export func decode(token as string) {
    return json.decode(convert.stringFromBytes(decodeSegment(segment($token, 1)), "utf-8"));
}

/**
 * Read a JWT's header (also without verifying) - useful for the `alg` and `kid`
 * fields when selecting a verification key.
 * @param token {string} the compact JWT
 * @return {json.Value} the token header
 * @throws {Error} on a malformed token
 */
export func header(token as string) {
    return json.decode(convert.stringFromBytes(decodeSegment(segment($token, 0)), "utf-8"));
}

# segment returns the i-th dot-separated part of a token, erroring if the token
# is not three segments.
func segment(token as string, i as int) {
    def parts as list of string init strings.split($token, ".");
    if (len($parts) != 3) {
        throw Error{kind: "value", message: "jwt: malformed token (want three dot-separated segments)", file: "", line: 0, col: 0};
    }
    return $parts[$i];
}
