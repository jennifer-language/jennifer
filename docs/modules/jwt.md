# `jwt` - JSON Web Tokens (RFC 7519)

Import with `import "jwt.j" as jwt;`. Sign, verify, and decode compact JWTs.
Claims are a [`json.Value`](../libraries/json.md) object; a token is the usual
`header.payload.signature` of base64url segments.

Ten algorithms across four families:

| Family | Algorithms                | Key (`bytes`)                                   |
| ------ | ------------------------- | ----------------------------------------------- |
| HMAC   | `HS256` / `HS384` / `HS512` | a shared secret of at least 32 / 48 / 64 bytes |
| RSA    | `RS256` / `RS384` / `RS512` | a PEM RSA key (PKCS#1 / PKCS#8 / PKIX)        |
| ECDSA  | `ES256` / `ES384` / `ES512` | a PEM EC key (SEC1 / PKCS#8 / PKIX)           |
| EdDSA  | `EdDSA`                   | a raw Ed25519 key from `crypto.signKeypair`     |

The key is always `bytes`: a secret, a PEM blob read from disk, or a raw
Ed25519 key, depending on the algorithm family.

```jennifer
use json;
use crypto;
import "jwt.j" as jwt;

def secret as bytes init crypto.randBytes(32);          # HS256 needs >= 32 bytes
def claims as json.Value init json.decode('{"sub":"ada","exp":9999999999}');

def token as string init jwt.sign($claims, $secret, "HS256");
def back as json.Value init jwt.verify($token, $secret, "HS256");   # throws if invalid
```

Runnable: [`examples/modules/jwt_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/jwt_demo.j).

## Surface

| Call                          | Returns      | Notes                                                                 |
| ----------------------------- | ------------ | --------------------------------------------------------------------- |
| `jwt.sign(claims, key, alg)`  | `string`     | Sign `claims` (a `json.Value`) into a compact JWT.                     |
| `jwt.signWithKid(claims, key, alg, kid)` | `string` | Like `sign`, with a `kid` (key id) in the header so the verifier can select the key; an empty `kid` is the same as `sign`. |
| `jwt.verify(token, key, alg)` | `json.Value` | Verify the signature, the header algorithm, and `exp` / `nbf`; return the claims. Throws on any failure. |
| `jwt.verifyLeeway(token, key, alg, leeway)` | `json.Value` | Like `verify`, but widen the `exp` / `nbf` checks by `leeway` seconds each way for clock skew. `verify` is `verifyLeeway(..., 0)`. |
| `jwt.Policy`                  | struct       | `{iss, aud, requireExp, leeway}`: the expected issuer and audience (`""` skips a check), whether `exp` is required, and the clock-skew leeway in seconds. |
| `jwt.verifyWith(token, key, alg, policy)` | `json.Value` | Like `verifyLeeway` with `policy.leeway`, then additionally enforce the `jwt.Policy` claim checks (issuer / audience / required `exp`). Throws on any failure. |
| `jwt.verifyWithKeys(token, keysByKid, alg)` | `json.Value` | Select the verification key by the header's `kid` from a `map of string to string` (secret, PEM text, or a JWK), then `verify`. |
| `jwt.verifyJwks(token, jwksJson, alg)` | `json.Value` | Verify against a **JWKS** (`{"keys":[...]}`): select the JWK by the header's `kid`, check it is meant for this `alg`, convert it with `crypto.jwkToPem`, then `verify`. RS\* / ES\* only; needs the default binary. |
| `jwt.jwksKeys(jwksJson, alg)` | `map of string to string` | Convert a JWKS once into `kid -> PEM` for the keys usable with `alg`, for `verifyWithKeys` - no per-token parse. RS\* / ES\* only; needs the default binary. |
| `jwt.verifyWithKeysWith(token, keysByKid, alg, policy)` | `json.Value` | `verifyWithKeys`, then the same `jwt.Policy` checks as `verifyWith`. |
| `jwt.verifyJwksWith(token, jwksJson, alg, policy)` | `json.Value` | `verifyJwks`, then the same `jwt.Policy` checks as `verifyWith` - the form to use for an OIDC issuer's tokens. |
| `jwt.decode(token)`           | `json.Value` | The payload claims **without verifying** - for inspection only.        |
| `jwt.header(token)`           | `json.Value` | The token header (its `alg` / `kid`), also without verifying.          |

## Verifying safely

`jwt.verify` takes the algorithm you expect as its third argument:

- **Algorithm confusion is rejected.** A token whose header `alg` does not equal
  the `alg` you pass is refused. This closes the classic attack where an
  attacker re-signs a token as `HS256` using your RSA *public* key as the HMAC
  secret: you asked for `RS256`, so the `HS256` token never gets that far. Never
  derive the verification algorithm from the token's own header.
- **`"none"` is not an algorithm.** It is not in the supported set, so a token
  claiming `alg: none` cannot be verified (only `decode`d).
- **HMAC keys must be long enough.** An `HS256` / `HS384` / `HS512` key shorter
  than the hash output (32 / 48 / 64 bytes) is refused by `sign` and by every
  verifier, as RFC 7518 3.2 requires. A short secret is guessable, and an empty
  one - an unset environment variable, say - would let anyone forge tokens; this
  way a misconfigured service rejects every token instead.
- **Header and claims must be JSON objects.** A token whose header or claims set
  is any other JSON value (an array, a string) is malformed, and `sign` refuses
  claims that are not an object.
- **Time claims are enforced.** When present, `exp` (expiry) rejects an expired
  token and `nbf` (not-before) rejects one that is not yet valid, both against
  the current time. `NumericDate` claims are Unix seconds, an integer or a
  fractional number (`1700000000.5`); any other type is malformed.
- **Clock skew is a `leeway`.** Issuer and verifier clocks rarely agree to the
  second, so `jwt.verifyLeeway($token, $key, $alg, $leeway)` widens each time
  check by `leeway` seconds: a token is accepted while `now < exp + leeway` and
  `now >= nbf - leeway`. `jwt.verify` is `verifyLeeway(..., 0)`, so zero
  tolerance is the default; use a small leeway (30-60s is typical) only where
  clock drift is real. `leeway` must be non-negative. The `...With` verifiers
  take it from `jwt.Policy.leeway`.
- **HMAC comparison is constant-time** (`crypto.hmacEqual`), so a wrong MAC
  leaks nothing through timing.
- **Segments must be canonical base64url.** A segment with stray `=` padding or
  non-zero trailing bits decodes to the same bytes as the canonical spelling -
  a *second token string* that verifies as the same token, which breaks
  anything keyed on the token string (replay caches, denylists). Such spellings
  are rejected as malformed, matching strict JWS implementations.
- **Every rejection is an `Error` of kind `"value"`**, whatever the cause - a
  bad signature, an expired token, a malformed segment, a member of the wrong
  type - so one `catch` that answers `401` covers them all.

- **Issuer / audience are application policy.** `jwt.verify` checks the
  signature, algorithm, and time claims; it does not check `iss` or `aud`,
  because the expected values are the application's, not the library's. Use
  `jwt.verifyWith($token, $key, $alg, jwt.Policy{iss: "https://issuer.example",
  aud: "my-api", requireExp: true, leeway: 30})` to additionally require them. An
  empty `iss` (or `aud`) skips that check; a non-empty one must match. `aud` may
  be a JSON string or an array of strings (RFC 7519), and the check passes when
  the expected audience is present.

- **A missing `exp` is accepted unless the policy requires it.** RFC 7519 makes
  `exp` optional, so `jwt.verify` accepts a token that never expires. A service
  issuing short-lived tokens should set `requireExp: true` on its `jwt.Policy`:
  `verifyWith` then refuses a token without `exp` (a `value` error), so one
  minted by a bug or a leaked key cannot stay valid forever.

`jwt.decode` and `jwt.header` do not verify anything. Use them only to read
a token you have not trusted yet (for example, to read `kid` before fetching the
matching key). Never authorize on their output.

## Building claims

Claims are a `json.Value`: decode a literal, or assemble one with the `json`
write surface:

```jennifer
use json;
use time;
use crypto;
import "jwt.j" as jwt;

def secret as bytes init crypto.randBytes(32);
def claims as json.Value init json.map();
$claims = json.set($claims, "/sub", "ada");
$claims = json.set($claims, "/role", "admin");
$claims = json.set($claims, "/exp", time.unix(time.now()) + 3600);   # one hour
def token as string init jwt.sign($claims, $secret, "HS256");
```

## Keys by family

- **HS\***: a `bytes` secret at least as long as the hash - 32 bytes for
  `HS256`, 48 for `HS384`, 64 for `HS512`; shorter keys are refused. Use a
  random one (`crypto.randBytes(32)`), not a password.
- **RS\* / ES\***: a PEM key as `bytes` - read it with `fs.readBytes("key.pem")`.
  Signing needs the private key, verifying needs the public key. Standard PEM
  encodings are accepted (RSA PKCS#1 or PKCS#8; EC SEC1 or PKCS#8; public keys in
  PKIX / `-----BEGIN PUBLIC KEY-----`).
- **EdDSA**: the `public` / `private` `bytes` from `crypto.signKeypair()`.

`jwt` does not generate RSA or EC keys; bring your own (from your identity
provider, or `openssl`). It generates Ed25519 keys through `crypto.signKeypair`.

## Selecting a key by `kid`

When an issuer rotates keys, each token's header carries a `kid` (key id) naming
which key signed it. `jwt.verifyWithKeys` reads that `kid`, looks it up in a
caller-supplied `map of string to string`, and verifies with the matching value
- an HMAC secret for HS\*, or PEM key text or a JWK for RS\* / ES\* (each is
decoded to `bytes` for you). A token with no `kid`, or a `kid` absent from the
map, is an error.

```jennifer
def keys as map of string to string init {
    "2024-key": "-----BEGIN PUBLIC KEY-----\n...\n-----END PUBLIC KEY-----\n",
    "2025-key": "-----BEGIN PUBLIC KEY-----\n...\n-----END PUBLIC KEY-----\n"
};
def claims as json.Value init jwt.verifyWithKeys($token, $keys, "RS256");
```

`alg` is still required and enforced against the header: selecting the key by
`kid` does not select the algorithm, so the algorithm-confusion protection
above stays in force. Pin `alg` to what the issuer uses.

For a raw JWKS endpoint (`{"keys":[{"kty":..,"kid":..,"n":..,"e":..}, ...]}`),
`jwt.verifyJwks` finds the JWK whose `kid` matches the header, converts it to a
PEM public key with [`crypto.jwkToPem`](../libraries/crypto.md), and verifies.
It is for RS\* / ES\* only, since a JWKS carries public keys: for HMAC use
`verifyWithKeys` with the shared secret, and for `EdDSA` pass the key to
`verify`.

```jennifer
def jwks as string init http.get("https://issuer.example.com/.well-known/jwks.json", {}).body;
def claims as json.Value init jwt.verifyJwks($token, $jwks, "RS256");
```

**A JWK must be meant for the job.** The selected key is refused when its `use`
is not `"sig"`, its `alg` is not the one you asked for, or its `kty` is the
wrong family (`RSA` for RS\*, `EC` for ES\*) - an issuer binds each key to one
algorithm (RFC 8725 3.1). Members the key does not carry are not checked. The
same rule applies to a JWK held in a `verifyWithKeys` map.

**Convert the set once when verifying many tokens.** `verifyJwks` parses the
JWKS and converts the key on every call. A service checking each request should
convert the set when it fetches it, with `jwt.jwksKeys`, and verify against the
resulting `kid -> PEM` map; keys unusable for `alg` are left out of the map:

```jennifer
def keys as map of string to string init jwt.jwksKeys($jwks, "RS256");
def claims as json.Value init jwt.verifyWithKeys($token, $keys, "RS256");
```

**Check the issuer and audience on key-set tokens.** `verifyWithKeys` and
`verifyJwks` verify the signature and the `exp` / `nbf` times, and nothing
more: they accept a token with no `exp` (it never expires), and a token minted
for any audience. A JWKS is what an OIDC issuer publishes - CI identity tokens,
login ID tokens - and that issuer serves many relying parties, so without an
audience check a token minted for another service replays against yours. Use the
`...With` forms, which apply the same `jwt.Policy` as `verifyWith`:

```jennifer
def policy as jwt.Policy init jwt.Policy{
    iss: "https://token.actions.githubusercontent.com",
    aud: "https://registry.example.com",
    requireExp: true,
    leeway: 30
};
def claims as json.Value init jwt.verifyJwksWith($token, $jwks, "RS256", $policy);
```

## JWT as web auth (`jwt_auth`)

There is no separate `jwt_auth` module; JWT authentication is this module used
as a [`web`](web.md) `before` middleware. Register a guard that takes the bearer
token (`web.bearerToken`), verifies it, and rejects on failure; it hands the
verified subject to the route handler as a request-scoped note
(`httpd.setRequestValue`, read back with `httpd.requestValue`):

```jennifer
use os;
use convert;
use json;
use httpd;
import "web.j" as web;
import "jwt.j" as jwt;

def const JWT_SECRET as bytes init convert.bytesFromString(os.getEnv("JWT_SECRET"), "utf-8");

func requireJwt(ctx as web.Context) {
    def token as string init web.bearerToken($ctx);
    if ($token == "") {
        web.respond($ctx, 401, "missing bearer token");
        return false;                         # stop the chain
    }
    try {
        def claims as json.Value init jwt.verify($token, JWT_SECRET, "HS256");
        httpd.setRequestValue($ctx.req, "user", json.asString($claims, "/sub"));
    } catch (e) {
        web.respond($ctx, 401, "invalid token");
        return false;
    }
    return true;                               # continue to the handler
}
```

If `JWT_SECRET` is unset or shorter than 32 bytes, every token is refused (the
key-length check), so a missing secret fails closed instead of opening the API.

## Platforms

HS\* (over `hash.hmac`) and `EdDSA` (over `crypto.sign` / `verify`) run on
both binaries. RS\* / ES\* need the `crypto` library's RSA / ECDSA surface,
which is on the default `jennifer` binary; on `jennifer-tiny` they raise a
friendly "not available" error (the same split as `net`).
