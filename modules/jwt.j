# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

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
use maps;
use convert;

# The algorithms this module accepts, by JOSE `alg` name.
def const SUPPORTED as list of string init [
    "HS256",
    "HS384",
    "HS512",
    "RS256",
    "RS384",
    "RS512",
    "ES256",
    "ES384",
    "ES512",
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
        throw Error{
            kind: "value",
            message: "jwt: non-canonical base64url segment",
            file: "",
            line: 0,
            col: 0
        };
    }
    return $out;
}

# ---- algorithm dispatch ----

func requireAlg(alg as string) {
    if (not lists.contains(SUPPORTED, $alg)) {
        throw Error{
            kind: "value",
            message: "jwt: unsupported algorithm " + $alg,
            file: "",
            line: 0,
            col: 0
        };
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
    match ($fam) {
        when "hmac" { return hash.hmac($key, $input, algHash($alg)); }
        when "rsa" { return crypto.rsaSign($key, $input, algHash($alg)); }
        when "ecdsa" { return crypto.ecdsaSign($key, $input, algHash($alg)); }
        else { return crypto.sign($key, $input); }
    }
}

# checkSig verifies a signature over the signing input. HMAC uses a constant-time
# compare so a bad MAC leaks nothing through timing.
func checkSig(alg as string, input as bytes, sig as bytes, key as bytes) {
    def fam as string init family($alg);
    match ($fam) {
        when "hmac" { return crypto.hmacEqual(hash.hmac($key, $input, algHash($alg)), $sig); }
        when "rsa" { return crypto.rsaVerify($key, $input, $sig, algHash($alg)); }
        when "ecdsa" { return crypto.ecdsaVerify($key, $input, $sig, algHash($alg)); }
        else { return crypto.verify($key, $input, $sig); }
    }
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
    return verifyLeeway($token, $key, $alg, 0);
}

/**
 * Verify a JWT exactly like `verify`, but widen the `exp` / `nbf` time-claim
 * checks by `leeway` seconds on each side, tolerating a small clock difference
 * between the token's issuer and this verifier. A token is accepted while
 * `now < exp + leeway` and `now >= nbf - leeway`; the signature, the header
 * algorithm, and the `crit` check are enforced exactly as in `verify`. `leeway`
 * must be non-negative. `verify(token, key, alg)` is `verifyLeeway(token, key,
 * alg, 0)`.
 * @param token {string} the compact JWT
 * @param key {bytes} the verification key (HMAC secret / PEM public / Ed25519 public)
 * @param alg {string} the algorithm the caller requires (e.g. "HS256", "RS256")
 * @param leeway {int} clock-skew tolerance in seconds (must be >= 0)
 * @return {json.Value} the verified claims
 * @throws {Error} on a malformed token, an algorithm mismatch, a bad signature, an out-of-window token, or a negative leeway
 */
export func verifyLeeway(token as string, key as bytes, alg as string, leeway as int) {
    if ($leeway < 0) {
        throw Error{
            kind: "value",
            message: "jwt.verifyLeeway: leeway must be non-negative",
            file: "",
            line: 0,
            col: 0
        };
    }
    requireAlg($alg);
    def parts as list of string init strings.split($token, ".");
    if (len($parts) != 3) {
        throw Error{
            kind: "value",
            message: "jwt.verify: malformed token (want three dot-separated segments)",
            file: "",
            line: 0,
            col: 0
        };
    }
    def head as json.Value init json.decode(convert.stringFromBytes(
        decodeSegment($parts[0]),
        "utf-8"));
    if (not json.has($head, "/alg") or json.asString($head, "/alg") != $alg) {
        throw Error{
            kind: "value",
            message: "jwt.verify: token algorithm does not match the expected " + $alg,
            file: "",
            line: 0,
            col: 0
        };
    }
    # RFC 7515 4.1.11: a verifier must reject a token carrying a `crit` (critical
    # header extensions) member it does not understand. This module understands
    # none, so any `crit` is a refusal - never silently ignore a critical header.
    if (json.has($head, "/crit")) {
        throw Error{
            kind: "value",
            message: "jwt.verify: token has an unsupported \"crit\" header",
            file: "",
            line: 0,
            col: 0
        };
    }
    def signingInput as string init $parts[0] + "." + $parts[1];
    def sig as bytes init decodeSegment($parts[2]);
    if (not checkSig($alg, convert.bytesFromString($signingInput, "utf-8"), $sig, $key)) {
        throw Error{
            kind: "value",
            message: "jwt.verify: signature verification failed",
            file: "",
            line: 0,
            col: 0
        };
    }
    def claims as json.Value init json.decode(convert.stringFromBytes(
        decodeSegment($parts[1]),
        "utf-8"));
    def now as int init time.unix(time.now());
    # Widen each bound by `leeway`: still-expired only past `exp + leeway`, and
    # not-yet-valid only before `nbf - leeway`. The `leeway` is moved to the `now`
    # side (`now - leeway >= exp`, `now + leeway < nbf`) rather than added to the
    # token-supplied `exp` / `nbf`, so a hostile far-future `exp` (near int64 max)
    # cannot trigger an integer-overflow error instead of a clean rejection - the
    # arithmetic is on `now` (~1.7e9) and the caller's small non-negative leeway.
    if (json.has($claims, "/exp") and $now - $leeway >= json.asInt($claims, "/exp")) {
        throw Error{
            kind: "value",
            message: "jwt.verify: token has expired",
            file: "",
            line: 0,
            col: 0
        };
    }
    if (json.has($claims, "/nbf") and $now + $leeway < json.asInt($claims, "/nbf")) {
        throw Error{
            kind: "value",
            message: "jwt.verify: token is not yet valid",
            file: "",
            line: 0,
            col: 0
        };
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
            throw Error{
                kind: "value",
                message: "jwt.verifyWith: issuer (iss) does not match the expected " + $policy.iss,
                file: "",
                line: 0,
                col: 0
            };
        }
    }
    if ($policy.aud != "") {
        if (not audienceMatches($claims, $policy.aud)) {
            throw Error{
                kind: "value",
                message: "jwt.verifyWith: audience (aud) does not include the expected " +
                    $policy.aud,
                file: "",
                line: 0,
                col: 0
            };
        }
    }
    return $claims;
}

/**
 * Verify a JWT, selecting the verification key by the header's `kid` (key id).
 * Reads `kid` from the (unverified) header, looks it up in `keysByKid`, and
 * verifies with the matching value - a shared secret for HS\*, or, for RS\* /
 * ES\*, either a PEM public key **or a JWK** (a JSON object with a `"kty"`
 * member, converted to a PEM via `crypto.jwkToPem`). A token with no `kid`
 * header, or a `kid` absent from the map, is a `jwt` error (never a silent skip).
 * The map value is **text** (decoded UTF-8), so it holds an HS\* secret, PEM, or
 * JWK - a raw binary Ed25519 key (from `crypto.signKeypair`) is not
 * text-representable, so `EdDSA` is not supported on this kid-based path; pass its
 * key to `verify` directly.
 *
 * `alg` is **still required** and enforced against the header, exactly as in
 * `verify`: selecting the key by `kid` does not select the algorithm, so this
 * keeps the algorithm-confusion protection (an attacker cannot swap in `HS256`
 * and have your RSA public PEM read as an HMAC secret). Pin `alg` to what the
 * issuer uses.
 *
 * For a raw **JWKS** (a JSON set of `{"kty","kid","n","e"|"x","y"}` keys), use
 * `verifyJwks`, which resolves the `kid` and converts the JWK to a key via
 * `crypto.jwkToPem`. This `keysByKid` form stays useful for a `kid -> secret`
 * (HMAC) map, or when you have already resolved keys to PEM out of band.
 * @param token {string} the compact JWT
 * @param keysByKid {map of string to string} `kid` -> HMAC secret, PEM key text, or a JWK (RS\* / ES\*)
 * @param alg {string} the algorithm the caller requires (e.g. "HS256", "RS256")
 * @return {json.Value} the verified claims
 * @throws {Error} on a missing / unknown `kid`, or any `verify` failure
 */
export func verifyWithKeys(token as string, keysByKid as map of string to string, alg as string) {
    def head as json.Value init header($token);
    if (not json.has($head, "/kid")) {
        throw Error{
            kind: "value",
            message: "jwt.verifyWithKeys: token header has no \"kid\"",
            file: "",
            line: 0,
            col: 0
        };
    }
    def kid as string init json.asString($head, "/kid");
    if (not maps.has($keysByKid, $kid)) {
        throw Error{
            kind: "value",
            message: "jwt.verifyWithKeys: no key registered for kid " + $kid,
            file: "",
            line: 0,
            col: 0
        };
    }
    def key as bytes init convert.bytesFromString(keyMaterialFor($alg, $keysByKid[$kid]), "utf-8");
    return verify($token, $key, $alg);
}

# keyMaterialFor turns a kid-map value into the key text verify() needs. For an
# asymmetric alg (RS* / ES*), a value that is a JWK (a JSON object carrying a
# "kty" member) is converted to a PEM public key via crypto.jwkToPem, so the map
# may hold JWKs directly rather than pre-converted PEMs; a PEM passes through
# unchanged. For an HMAC alg the value is the shared secret, always verbatim.
func keyMaterialFor(alg as string, value as string) {
    if (strings.startsWith($alg, "HS")) {
        return $value;
    }
    def t as string init strings.trim($value);
    if (strings.startsWith($t, "{") and strings.contains($t, "\"kty\"")) {
        return crypto.jwkToPem($t);
    }
    return $value;
}

/**
 * Verify a JWT against a **JWKS** (a JSON Web Key Set), selecting the key by the
 * header's `kid`. Reads `kid` from the (unverified) header, finds the matching
 * JWK in the set's `keys` array, converts it to a PEM public key with
 * `crypto.jwkToPem`, and verifies. For asymmetric algorithms only (RS\* / ES\*):
 * a JWKS carries public keys, so an HMAC `alg` is a `jwt` error (use
 * `verifyWithKeys` with the shared secret instead).
 *
 * `alg` is **required** and enforced against the header, exactly as `verify` /
 * `verifyWithKeys` - selecting the key by `kid` does not select the algorithm, so
 * the algorithm-confusion protection stays. Needs the default `jennifer` binary
 * (`crypto.jwkToPem` is net-of-`crypto/x509`, off the TinyGo build).
 * @param token {string} the compact JWT
 * @param jwksJson {string} the JWKS JSON (`{"keys":[{...}, ...]}`)
 * @param alg {string} the algorithm the caller requires (e.g. "RS256", "ES256")
 * @return {json.Value} the verified claims
 * @throws {Error} on a missing / unknown `kid`, an HMAC `alg`, a malformed JWK, or any verify failure
 */
export func verifyJwks(token as string, jwksJson as string, alg as string) {
    requireAlg($alg);
    if (strings.startsWith($alg, "HS")) {
        throw Error{
            kind: "value",
            message: "jwt.verifyJwks: an HMAC algorithm (" + $alg +
                ") has no JWKS public key; use verifyWithKeys with the shared secret",
            file: "",
            line: 0,
            col: 0
        };
    }
    def head as json.Value init header($token);
    if (not json.has($head, "/kid")) {
        throw Error{
            kind: "value",
            message: "jwt.verifyJwks: token header has no \"kid\"",
            file: "",
            line: 0,
            col: 0
        };
    }
    def kid as string init json.asString($head, "/kid");
    def set as json.Value init json.decode($jwksJson);
    if (not json.has($set, "/keys") or not (json.typeOf($set, "/keys") == "list")) {
        throw Error{
            kind: "value",
            message: "jwt.verifyJwks: JWKS has no \"keys\" array",
            file: "",
            line: 0,
            col: 0
        };
    }
    def n as int init json.length($set, "/keys");
    def i as int init 0;
    while ($i < $n) {
        def kp as string init "/keys/" + convert.toString($i);
        if (json.has($set, $kp + "/kid") and json.asString($set, $kp + "/kid") == $kid) {
            def jwk as json.Value init json.get($set, $kp);
            def pem as string init crypto.jwkToPem(json.encode($jwk));
            return verify($token, convert.bytesFromString($pem, "utf-8"), $alg);
        }
        $i = $i + 1;
    }
    throw Error{
        kind: "value",
        message: "jwt.verifyJwks: no key in the JWKS matches kid " + $kid,
        file: "",
        line: 0,
        col: 0
    };
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
        throw Error{
            kind: "value",
            message: "jwt: malformed token (want three dot-separated segments)",
            file: "",
            line: 0,
            col: 0
        };
    }
    return $parts[$i];
}
