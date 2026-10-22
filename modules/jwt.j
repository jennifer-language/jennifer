# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.25.0

/**
 * JSON Web Tokens (RFC 7519): sign, verify, and decode compact JWTs. Claims are
 * a `json.Value` object; the token is the usual `header.payload.signature` of
 * base64url segments. Ten algorithms across four families: HMAC
 * (`HS256` / `HS384` / `HS512`, over a shared secret), RSA PKCS#1 v1.5
 * (`RS256` / `RS384` / `RS512`), ECDSA (`ES256` / `ES384` / `ES512`), and
 * Ed25519 (`EdDSA`). The key is always `bytes`: an HMAC secret (at least as
 * long as the hash: 32 / 48 / 64 bytes, RFC 7518 3.2), a PEM-encoded RSA / EC
 * key, or a raw Ed25519 key from `crypto.signKeypair`.
 *
 * `verify` takes the algorithm you *expect* and rejects a token whose header
 * disagrees - this closes the classic JWT algorithm-confusion attack (a forged
 * `HS256` token verified with an RSA public key as the HMAC secret). It also
 * enforces the `exp` (expiry) and `nbf` (not-before) time claims when present.
 * Every rejection is an `Error` of kind "value".
 *
 * RS* / ES* need the `crypto` library's RSA / ECDSA surface, which is on the
 * default `jennifer` binary; HS* and `EdDSA` run on both binaries.
 * @module jwt
 * @example
 * use json;
 * use crypto;
 * import "jwt.j" as jwt;
 * def claims as json.Value init json.decode('{"sub":"ada","exp":9999999999}');
 * def secret as bytes init crypto.randBytes(32);
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

/**
 * The application-level checks the `...With` verifiers enforce on top of the
 * cryptographic verification. `exp` is optional in RFC 7519, so plain `verify`
 * accepts a token without one; `requireExp: true` refuses such a token, so a
 * token minted by a bug or a leaked key cannot stay valid forever.
 * @field iss {string} the expected issuer ("" skips the check)
 * @field aud {string} the expected audience, in a string or array `aud` ("" skips the check)
 * @field requireExp {bool} refuse a token that has no `exp` claim
 * @field leeway {int} clock-skew seconds for `exp` / `nbf`, as `verifyLeeway` (>= 0)
 */
export def struct Policy {
    iss as string,
    aud as string,
    requireExp as bool,
    leeway as int
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
        throw Error{kind: "value", message: "jwt: non-canonical base64url segment"};
    }
    return $out;
}

# ---- algorithm dispatch ----

func requireAlg(alg as string) {
    if (not lists.contains(SUPPORTED, $alg)) {
        throw Error{kind: "value", message: "jwt: unsupported algorithm " + $alg};
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

# requireHmacKey enforces RFC 7518 3.2: an HS* key must be at least as long as
# the hash output (32 / 48 / 64 bytes). A short key - an empty secret from a
# missing environment variable above all - would make tokens forgeable, so it is
# refused on both the signing and the verifying side.
func requireHmacKey(alg as string, key as bytes) {
    def need as int init 32;
    if ($alg == "HS384") {
        $need = 48;
    } elseif ($alg == "HS512") {
        $need = 64;
    }
    if (len($key) < $need) {
        throw Error{
            kind: "value",
            message: "jwt: " + $alg + " needs a key of at least " + convert.toString($need) +
                " bytes (RFC 7518 3.2), got " + convert.toString(len($key))
        };
    }
}

# decodeObject decodes one token segment as a JSON object. RFC 7519 requires
# both the header and the claims set to be objects; any other JSON value is a
# malformed token, not something to hand back.
func decodeObject(fnName as string, segment as string, what as string) {
    def v as json.Value init json.decode(convert.stringFromBytes(decodeSegment($segment), "utf-8"));
    if (json.typeOf($v) != "map") {
        throw Error{
            kind: "value",
            message: $fnName + ": the token " + $what + " is not a JSON object"
        };
    }
    return $v;
}

# stringMember reads an optional string member of a decoded object: "" when it
# is absent, a value error when it is present with another type (so a hostile
# `"alg": 1` or `"kid": []` is a clean rejection, not a runtime error).
func stringMember(fnName as string, node as json.Value, name as string) {
    if (not json.has($node, "/" + $name)) {
        return "";
    }
    if (json.typeOf($node, "/" + $name) != "string") {
        throw Error{kind: "value", message: $fnName + ": \"" + $name + "\" is not a string"};
    }
    return json.asString($node, "/" + $name);
}

# timeClaimPassed reports whether `now` has reached the NumericDate claim `name`
# (RFC 7519 2: a JSON number, possibly fractional). An int compares directly; a
# float compares exactly against the int `now` (Jennifer's mixed comparison is
# exact), so a fractional `exp` such as 1700000000.5 is honoured instead of
# erroring. Any other type is a malformed token.
func timeClaimPassed(claims as json.Value, name as string, now as int) {
    def kind as string init json.typeOf($claims, "/" + $name);
    if ($kind == "int") {
        return $now >= json.asInt($claims, "/" + $name);
    }
    if ($kind == "float") {
        return $now >= json.asFloat($claims, "/" + $name);
    }
    throw Error{kind: "value", message: "jwt.verify: \"" + $name + "\" is not a NumericDate"};
}

# requireJwkUsable rejects a JWK the issuer did not publish for this use: a
# `use` other than "sig", an `alg` other than the expected one, or a `kty` of
# the wrong family (RFC 8725 3.1 - a key is bound to one algorithm). Members
# that are absent are not checked.
func requireJwkUsable(fnName as string, jwk as json.Value, alg as string) {
    def keyUse as string init stringMember($fnName, $jwk, "use");
    if ($keyUse != "" and $keyUse != "sig") {
        throw Error{
            kind: "value",
            message: $fnName + ": the JWK is for use \"" + $keyUse + "\", not signatures"
        };
    }
    def jwkAlg as string init stringMember($fnName, $jwk, "alg");
    if ($jwkAlg != "" and $jwkAlg != $alg) {
        throw Error{
            kind: "value",
            message: $fnName + ": the JWK is for " + $jwkAlg + ", not " + $alg
        };
    }
    def kty as string init stringMember($fnName, $jwk, "kty");
    def want as string init "RSA";
    if (family($alg) == "ecdsa") {
        $want = "EC";
    }
    if ($kty != $want) {
        throw Error{
            kind: "value",
            message: $fnName + ": " + $alg + " needs a JWK of kty \"" + $want + "\", got \"" +
                $kty + "\""
        };
    }
}

# requireJwksAlg refuses the algorithms a JWKS cannot serve: HMAC (a JWKS
# publishes public keys) and EdDSA (crypto.jwkToPem converts RSA / EC keys).
func requireJwksAlg(fnName as string, alg as string) {
    requireAlg($alg);
    if (strings.startsWith($alg, "HS")) {
        throw Error{
            kind: "value",
            message: $fnName + ": an HMAC algorithm (" + $alg +
                ") has no JWKS public key; use verifyWithKeys with the shared secret"
        };
    }
    if ($alg == "EdDSA") {
        throw Error{
            kind: "value",
            message: $fnName + ": EdDSA keys are not supported from a JWKS; pass the key to verify"
        };
    }
}

# computeSig produces the signature over the signing input for the algorithm.
func computeSig(alg as string, input as bytes, key as bytes) {
    def fam as string init family($alg);
    if ($fam == "hmac") {
        requireHmacKey($alg, $key);
    }
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
    if ($fam == "hmac") {
        requireHmacKey($alg, $key);
    }
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
    return signHeader($claims, $key, $alg, '{"alg":"' + $alg + '","typ":"JWT"}');
}

/**
 * Sign `claims` like `sign`, adding a `kid` (key id) to the header so the
 * verifier can pick the key: `{"alg": alg, "kid": kid, "typ": "JWT"}`. An empty
 * `kid` is left out, which makes this the same as `sign`.
 * @param claims {json.Value} the claims object (the token payload)
 * @param key {bytes} the signing key (HMAC secret / PEM / Ed25519 private)
 * @param alg {string} the JOSE algorithm (e.g. "HS256", "RS256", "ES256", "EdDSA")
 * @param kid {string} the key id for the header ("" for none)
 * @return {string} the signed `header.payload.signature` token
 * @throws {Error} on an unsupported algorithm or a key the algorithm rejects
 */
export func signWithKid(claims as json.Value, key as bytes, alg as string, kid as string) {
    requireAlg($alg);
    if ($kid == "") {
        return sign($claims, $key, $alg);
    }
    # json.encode quotes and escapes the caller-supplied kid.
    return signHeader(
        $claims,
        $key,
        $alg,
        '{"alg":"' + $alg + '","kid":' + json.encode($kid) + ',"typ":"JWT"}');
}

# signHeader signs claims under an already-rendered header JSON; the alg has
# been validated by the caller.
func signHeader(claims as json.Value, key as bytes, alg as string, headerJson as string) {
    if (json.typeOf($claims) != "map") {
        throw Error{kind: "value", message: "jwt.sign: claims must be a JSON object"};
    }
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
        throw Error{kind: "value", message: "jwt.verifyLeeway: leeway must be non-negative"};
    }
    requireAlg($alg);
    def parts as list of string init strings.split($token, ".");
    if (len($parts) != 3) {
        throw Error{
            kind: "value",
            message: "jwt.verify: malformed token (want three dot-separated segments)"
        };
    }
    def head as json.Value init decodeObject("jwt.verify", $parts[0], "header");
    if (stringMember("jwt.verify", $head, "alg") != $alg) {
        throw Error{
            kind: "value",
            message: "jwt.verify: token algorithm does not match the expected " + $alg
        };
    }
    # RFC 7515 4.1.11: a verifier must reject a token carrying a `crit` (critical
    # header extensions) member it does not understand. This module understands
    # none, so any `crit` is a refusal - never silently ignore a critical header.
    if (json.has($head, "/crit")) {
        throw Error{kind: "value", message: "jwt.verify: token has an unsupported \"crit\" header"};
    }
    def signingInput as string init $parts[0] + "." + $parts[1];
    def sig as bytes init decodeSegment($parts[2]);
    if (not checkSig($alg, convert.bytesFromString($signingInput, "utf-8"), $sig, $key)) {
        throw Error{kind: "value", message: "jwt.verify: signature verification failed"};
    }
    def claims as json.Value init decodeObject("jwt.verify", $parts[1], "claims set");
    def now as int init time.unix(time.now());
    # Widen each bound by `leeway`: still-expired only past `exp + leeway`, and
    # not-yet-valid only before `nbf - leeway`. The `leeway` is moved to the `now`
    # side (`now - leeway >= exp`, `now + leeway < nbf`) rather than added to the
    # token-supplied `exp` / `nbf`, so a hostile far-future `exp` (near int64 max)
    # cannot trigger an integer-overflow error instead of a clean rejection - the
    # arithmetic is on `now` (~1.7e9) and the caller's small non-negative leeway.
    if (json.has($claims, "/exp") and timeClaimPassed($claims, "exp", $now - $leeway)) {
        throw Error{kind: "value", message: "jwt.verify: token has expired"};
    }
    if (json.has($claims, "/nbf") and not timeClaimPassed($claims, "nbf", $now + $leeway)) {
        throw Error{kind: "value", message: "jwt.verify: token is not yet valid"};
    }
    return $claims;
}

# audienceMatches reports whether the token's `aud` claim admits `want`. Per RFC
# 7519 4.1.3 `aud` is either a single string or an array of strings; a missing
# claim never matches, and a non-string array element is skipped, never matched.
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
        for (def i in 0..$n) {
            if (json.typeOf($claims, "/aud/{$i}") == "string" and
                json.asString($claims, "/aud/{$i}") == $want) {
                return true;
            }
        }
    }
    return false;
}

/**
 * Verify a JWT like `verify`, then additionally enforce the application-level
 * claim policy: when `policy.iss` is non-empty the token's `iss` must equal it,
 * when `policy.aud` is non-empty the token's `aud` (a string or an array of
 * strings) must include it, and when `policy.requireExp` is true the token must
 * carry an `exp` claim. The signature is checked exactly as in `verify`, and
 * `exp` / `nbf` as in `verifyLeeway` with `policy.leeway`; issuer, audience, and
 * a mandatory expiry are application policy the library cannot assume, hence the
 * explicit opt-in.
 * @param token {string} the compact JWT
 * @param key {bytes} the verification key (HMAC secret / PEM public / Ed25519 public)
 * @param alg {string} the algorithm the caller requires (e.g. "HS256", "RS256")
 * @param policy {Policy} the expected issuer / audience, the `exp` requirement, and the leeway
 * @return {json.Value} the verified claims
 * @throws {Error} on any `verify` failure, a mismatched issuer / audience, or a
 *   missing `exp` the policy requires
 */
export func verifyWith(token as string, key as bytes, alg as string, policy as Policy) {
    def claims as json.Value init verifyLeeway($token, $key, $alg, $policy.leeway);
    return applyPolicy("jwt.verifyWith", $claims, $policy);
}

# applyPolicy enforces a Policy on already-verified claims - a required `exp`,
# the expected issuer, the expected audience - for every `...With` verifier, so
# the key-set forms check exactly what verifyWith checks. fnName prefixes the
# errors.
func applyPolicy(fnName as string, claims as json.Value, policy as Policy) {
    if ($policy.requireExp and not json.has($claims, "/exp")) {
        throw Error{
            kind: "value",
            message: $fnName + ": the token has no expiry (exp), which the policy requires"
        };
    }
    if ($policy.iss != "") {
        if (not json.has($claims, "/iss") or json.typeOf($claims, "/iss") != "string" or
            json.asString($claims, "/iss") != $policy.iss) {
            throw Error{
                kind: "value",
                message: $fnName + ": issuer (iss) does not match the expected " + $policy.iss
            };
        }
    }
    if ($policy.aud != "") {
        if (not audienceMatches($claims, $policy.aud)) {
            throw Error{
                kind: "value",
                message: $fnName + ": audience (aud) does not include the expected " + $policy.aud
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
 * member, checked for its `use` / `alg` / `kty` as in `verifyJwks` and converted
 * to a PEM via `crypto.jwkToPem`, which needs the default `jennifer` binary). A
 * token with no `kid` header, or a `kid` absent from the map, is an `Error` of
 * kind "value" (never a silent skip).
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
 * `crypto.jwkToPem` - or convert the set once with `jwksKeys` and pass the
 * result here, so a verifier handling many tokens does not re-parse the set per
 * token. This `keysByKid` form is also the one for a `kid -> secret` (HMAC) map.
 * @param token {string} the compact JWT
 * @param keysByKid {map of string to string} `kid` -> HMAC secret, PEM key text, or a JWK (RS\* / ES\*)
 * @param alg {string} the algorithm the caller requires (e.g. "HS256", "RS256")
 * @return {json.Value} the verified claims
 * @throws {Error} on a missing / unknown `kid`, an unusable JWK, or any `verify` failure
 */
export func verifyWithKeys(token as string, keysByKid as map of string to string, alg as string) {
    return keysVerify("jwt.verifyWithKeys", $token, $keysByKid, $alg, 0);
}

# keysVerify is verifyWithKeys with a leeway, shared with verifyWithKeysWith.
func keysVerify(
    fnName as string,
    token as string,
    keysByKid as map of string to string,
    alg as string,
    leeway as int) {
    def kid as string init headerKid($fnName, $token);
    if (not maps.has($keysByKid, $kid)) {
        throw Error{kind: "value", message: $fnName + ": no key registered for kid " + $kid};
    }
    def material as string init keyMaterialFor($fnName, $alg, $keysByKid[$kid]);
    return verifyLeeway($token, convert.bytesFromString($material, "utf-8"), $alg, $leeway);
}

# headerKid reads the (unverified) header's `kid`, which must be present and a
# string.
func headerKid(fnName as string, token as string) {
    def head as json.Value init decodeObject($fnName, segment($token, 0), "header");
    def kid as string init stringMember($fnName, $head, "kid");
    if ($kid == "") {
        throw Error{kind: "value", message: $fnName + ": token header has no \"kid\""};
    }
    return $kid;
}

# keyMaterialFor turns a kid-map value into the key text verify() needs. For an
# asymmetric alg (RS* / ES*), a value that is a JWK (a JSON object carrying a
# "kty" member) is checked for its use / alg / kty and converted to a PEM public
# key via crypto.jwkToPem, so the map may hold JWKs directly rather than
# pre-converted PEMs; a PEM passes through unchanged. For an HMAC alg the value
# is the shared secret, always verbatim.
func keyMaterialFor(fnName as string, alg as string, value as string) {
    if (strings.startsWith($alg, "HS")) {
        return $value;
    }
    def t as string init strings.trim($value);
    if (strings.startsWith($t, '{') and strings.contains($t, "\"kty\"")) {
        def jwk as json.Value init json.decode($t);
        requireJwkUsable($fnName, $jwk, $alg);
        return crypto.jwkToPem($t);
    }
    return $value;
}

/**
 * `verifyWithKeys` plus the claim policy `verifyWith` enforces: the key is chosen
 * by the header's `kid` from `keysByKid`, the signature and `exp` / `nbf` are
 * checked, then the `Policy` - a required `exp`, the expected issuer, the
 * expected audience. Use this whenever the token comes from an issuer that serves
 * more than one audience, or may omit `exp`.
 * @param token {string} the compact JWT
 * @param keysByKid {map of string to string} `kid` -> HMAC secret, PEM key text, or a JWK (RS\* / ES\*)
 * @param alg {string} the algorithm the caller requires (e.g. "HS256", "RS256")
 * @param policy {Policy} the expected issuer / audience, the `exp` requirement, and the leeway
 * @return {json.Value} the verified claims
 * @throws {Error} on anything `verifyWithKeys` rejects, a mismatched issuer /
 *   audience, or a missing `exp` the policy requires
 */
export func verifyWithKeysWith(
    token as string,
    keysByKid as map of string to string,
    alg as string,
    policy as Policy) {
    def fnName as string init "jwt.verifyWithKeysWith";
    def claims as json.Value init keysVerify($fnName, $token, $keysByKid, $alg, $policy.leeway);
    return applyPolicy($fnName, $claims, $policy);
}

/**
 * Verify a JWT against a **JWKS** (a JSON Web Key Set), selecting the key by the
 * header's `kid`. Reads `kid` from the (unverified) header, finds the matching
 * JWK in the set's `keys` array, converts it to a PEM public key with
 * `crypto.jwkToPem`, and verifies. The JWK must be fit for the job: a `use`
 * other than "sig", an `alg` other than the expected one, or a `kty` of the
 * wrong family is refused (RFC 8725 3.1). For RS\* / ES\* only: a JWKS carries
 * public keys, so an HMAC `alg` is refused (use `verifyWithKeys` with the shared
 * secret), and so is `EdDSA` (pass its key to `verify`).
 *
 * `alg` is **required** and enforced against the header, exactly as `verify` /
 * `verifyWithKeys` - selecting the key by `kid` does not select the algorithm, so
 * the algorithm-confusion protection stays. This call parses the set and
 * converts the key on every call; to verify many tokens against one set,
 * convert it once with `jwksKeys` and use `verifyWithKeys`. Needs the default
 * `jennifer` binary (`crypto.jwkToPem` uses Go's `crypto/x509`, which is not in
 * the TinyGo build).
 * @param token {string} the compact JWT
 * @param jwksJson {string} the JWKS JSON (`{"keys":[{...}, ...]}`)
 * @param alg {string} the algorithm the caller requires (e.g. "RS256", "ES256")
 * @return {json.Value} the verified claims
 * @throws {Error} on a missing / unknown `kid`, an HMAC or EdDSA `alg`, an unusable or malformed JWK, or any verify failure
 */
export func verifyJwks(token as string, jwksJson as string, alg as string) {
    return jwksVerify("jwt.verifyJwks", $token, $jwksJson, $alg, 0);
}

# jwksVerify is verifyJwks with a leeway, shared with verifyJwksWith.
func jwksVerify(
    fnName as string,
    token as string,
    jwksJson as string,
    alg as string,
    leeway as int) {
    requireJwksAlg($fnName, $alg);
    def kid as string init headerKid($fnName, $token);
    def set as json.Value init jwksSet($fnName, $jwksJson);
    def n as int init json.length($set, "/keys");
    for (def i in 0..$n) {
        def jwk as json.Value init json.get($set, "/keys/{$i}");
        if (json.typeOf($jwk) == "map" and jwkKid($jwk) == $kid) {
            requireJwkUsable($fnName, $jwk, $alg);
            def pem as string init crypto.jwkToPem(json.encode($jwk));
            return verifyLeeway($token, convert.bytesFromString($pem, "utf-8"), $alg, $leeway);
        }
    }
    throw Error{kind: "value", message: $fnName + ": no key in the JWKS matches kid " + $kid};
}

# jwksSet decodes a JWKS and checks it has a `keys` array.
func jwksSet(fnName as string, jwksJson as string) {
    def set as json.Value init json.decode($jwksJson);
    if (json.typeOf($set) != "map" or not json.has($set, "/keys") or
        json.typeOf($set, "/keys") != "list") {
        throw Error{kind: "value", message: $fnName + ": JWKS has no \"keys\" array"};
    }
    return $set;
}

# jwkKid returns a JWK's `kid`, or "" when it has none or it is not a string (such
# a key can never be selected).
func jwkKid(jwk as json.Value) {
    if (json.has($jwk, "/kid") and json.typeOf($jwk, "/kid") == "string") {
        return json.asString($jwk, "/kid");
    }
    return "";
}

/**
 * Convert a JWKS once into the `kid -> PEM` map `verifyWithKeys` /
 * `verifyWithKeysWith` take, keeping only the keys usable for `alg`: a key with
 * a `kid`, a `use` of "sig" (or none), an `alg` of `alg` (or none), and the
 * `kty` of its family. Fetch and convert the set when it changes, then verify
 * each token against the map; this skips the per-token parse and conversion
 * `verifyJwks` does. For RS\* / ES\* only, and needs the default `jennifer`
 * binary, as `verifyJwks`.
 * @param jwksJson {string} the JWKS JSON (`{"keys":[{...}, ...]}`)
 * @param alg {string} the algorithm the keys will verify (e.g. "RS256", "ES256")
 * @return {map of string to string} `kid` -> PEM public key, for the usable keys
 * @throws {Error} on an HMAC or EdDSA `alg`, a set with no `keys` array, or a malformed usable key
 */
export func jwksKeys(jwksJson as string, alg as string) {
    def fnName as string init "jwt.jwksKeys";
    requireJwksAlg($fnName, $alg);
    def set as json.Value init jwksSet($fnName, $jwksJson);
    def out as map of string to string init {};
    def n as int init json.length($set, "/keys");
    for (def i in 0..$n) {
        def jwk as json.Value init json.get($set, "/keys/{$i}");
        if (json.typeOf($jwk) == "map" and jwkKid($jwk) != "" and jwkUsable($fnName, $jwk, $alg)) {
            $out[jwkKid($jwk)] = crypto.jwkToPem(json.encode($jwk));
        }
    }
    return $out;
}

# jwkUsable is requireJwkUsable as a predicate, for filtering a set: a key the
# issuer published for another algorithm or use is left out, not an error.
func jwkUsable(fnName as string, jwk as json.Value, alg as string) {
    try {
        requireJwkUsable($fnName, $jwk, $alg);
    } catch (e) {
        return false;
    }
    return true;
}

/**
 * `verifyJwks` plus the claim policy `verifyWith` enforces. This is the form to
 * use for an OIDC issuer's tokens (CI identity tokens, login ID tokens): the key
 * comes from the issuer's JWKS by `kid`, then the `Policy` checks the issuer, the
 * audience - without which a token minted for another service would be accepted
 * by yours - and, with `requireExp`, that the token expires at all.
 * @param token {string} the compact JWT
 * @param jwksJson {string} the JWKS JSON (`{"keys":[{...}, ...]}`)
 * @param alg {string} the algorithm the caller requires (e.g. "RS256", "ES256")
 * @param policy {Policy} the expected issuer / audience, the `exp` requirement, and the leeway
 * @return {json.Value} the verified claims
 * @throws {Error} on anything `verifyJwks` rejects, a mismatched issuer /
 *   audience, or a missing `exp` the policy requires
 */
export func verifyJwksWith(token as string, jwksJson as string, alg as string, policy as Policy) {
    def fnName as string init "jwt.verifyJwksWith";
    def claims as json.Value init jwksVerify($fnName, $token, $jwksJson, $alg, $policy.leeway);
    return applyPolicy($fnName, $claims, $policy);
}

/**
 * Decode a JWT's claims **without verifying** the signature or time claims - for
 * inspecting a token you do not trust yet (e.g. to read its `kid` / issuer
 * before fetching a key). Never trust these claims for authorization; use
 * `verify` for that.
 * @param token {string} the compact JWT
 * @return {json.Value} the payload claims, unverified
 * @throws {Error} on a malformed token (including claims that are not a JSON object)
 */
export func decode(token as string) {
    return decodeObject("jwt.decode", segment($token, 1), "claims set");
}

/**
 * Read a JWT's header (also without verifying) - useful for the `alg` and `kid`
 * fields when selecting a verification key.
 * @param token {string} the compact JWT
 * @return {json.Value} the token header
 * @throws {Error} on a malformed token (including a header that is not a JSON object)
 */
export func header(token as string) {
    return decodeObject("jwt.header", segment($token, 0), "header");
}

# segment returns the i-th dot-separated part of a token, erroring if the token
# is not three segments.
func segment(token as string, i as int) {
    def parts as list of string init strings.split($token, ".");
    if (len($parts) != 3) {
        throw Error{
            kind: "value",
            message: "jwt: malformed token (want three dot-separated segments)"
        };
    }
    return $parts[$i];
}
