# `jwt` - JSON Web Tokens (RFC 7519)

Import with `import "jwt.j" as jwt;`. Sign, verify, and decode compact JWTs.
Claims are a [`json.Value`](../libraries/json.md) object; a token is the usual
`header.payload.signature` of base64url segments.

Ten algorithms across four families:

| Family | Algorithms                | Key (`bytes`)                                   |
| ------ | ------------------------- | ----------------------------------------------- |
| HMAC   | `HS256` / `HS384` / `HS512` | a shared secret                               |
| RSA    | `RS256` / `RS384` / `RS512` | a PEM RSA key (PKCS#1 / PKCS#8 / PKIX)        |
| ECDSA  | `ES256` / `ES384` / `ES512` | a PEM EC key (SEC1 / PKCS#8 / PKIX)           |
| EdDSA  | `EdDSA`                   | a raw Ed25519 key from `crypto.signKeypair`     |

The key is **always `bytes`** - a secret, a PEM blob read from disk, or a raw
Ed25519 key - and which one is decided by the algorithm family.

```jennifer
use convert;
import "jwt.j" as jwt;

def secret as bytes init convert.bytesFromString("topsecret", "utf-8");
def claims as json.Value init json.decode("{\"sub\":\"ada\",\"exp\":9999999999}");

def token as string init jwt.sign($claims, $secret, "HS256");
def back as json.Value init jwt.verify($token, $secret, "HS256");   # throws if invalid
```

Runnable: [`examples/modules/jwt_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/jwt_demo.j).

## Surface

| Call                          | Returns      | Notes                                                                 |
| ----------------------------- | ------------ | --------------------------------------------------------------------- |
| `jwt.sign(claims, key, alg)`  | `string`     | Sign `claims` (a `json.Value`) into a compact JWT.                     |
| `jwt.verify(token, key, alg)` | `json.Value` | Verify the signature, the header algorithm, and `exp` / `nbf`; return the claims. Throws on any failure. |
| `jwt.verifyLeeway(token, key, alg, leeway)` | `json.Value` | Like `verify`, but widen the `exp` / `nbf` checks by `leeway` seconds each way for clock skew. `verify` is `verifyLeeway(..., 0)`. |
| `jwt.verifyWith(token, key, alg, policy)` | `json.Value` | Like `verify`, then additionally enforce the `jwt.Policy` claim checks (issuer / audience). Throws on any failure. |
| `jwt.verifyWithKeys(token, keysByKid, alg)` | `json.Value` | Select the verification key by the header's `kid` from a `map of string to string` (secret or PEM text), then `verify`. |
| `jwt.verifyJwks(token, jwksJson, alg)` | `json.Value` | Verify against a **JWKS** (`{"keys":[...]}`): select the JWK by the header's `kid`, convert it to a key with `crypto.jwkToPem`, then `verify`. Asymmetric algs (RS\* / ES\*) only; needs the default binary. |
| `jwt.decode(token)`           | `json.Value` | The payload claims **without verifying** - for inspection only.        |
| `jwt.header(token)`           | `json.Value` | The token header (its `alg` / `kid`), also without verifying.          |

## Verifying safely

`jwt.verify` takes the algorithm you **expect** as its third argument, and this
is deliberate - it is the difference between a safe verifier and a vulnerable
one:

- **Algorithm confusion is rejected.** A token whose header `alg` does not equal
  the `alg` you pass is refused. This closes the classic attack where an
  attacker re-signs a token as `HS256` using your RSA *public* key as the HMAC
  secret: you asked for `RS256`, so the `HS256` token never gets that far. Never
  derive the verification algorithm from the token's own header.
- **`"none"` is not an algorithm.** It is not in the supported set, so a token
  claiming `alg: none` cannot be verified (only `decode`d).
- **Time claims are enforced.** When present, `exp` (expiry) rejects an expired
  token and `nbf` (not-before) rejects one that is not yet valid, both against
  the current time. `NumericDate` claims are Unix seconds.
- **Clock skew is a `leeway`.** Issuer and verifier clocks rarely agree to the
  second, so `jwt.verifyLeeway($token, $key, $alg, $leeway)` widens each time
  check by `leeway` seconds: a token is accepted while `now < exp + leeway` and
  `now >= nbf - leeway`. `jwt.verify` is exactly `verifyLeeway(..., 0)`, so the
  strict, zero-tolerance behaviour stays the default; opt into a small leeway
  (30-60s is typical) only where clock drift is real. `leeway` must be
  non-negative.
- **HMAC comparison is constant-time** (`crypto.hmacEqual`), so a wrong MAC
  leaks nothing through timing.
- **Segments must be canonical base64url.** A segment with stray `=` padding or
  non-zero trailing bits decodes to the same bytes as the canonical spelling -
  a *second token string* that verifies as the same token, which breaks
  anything keyed on the token string (replay caches, denylists). Such spellings
  are rejected as malformed, matching strict JWS implementations.

- **Issuer / audience are application policy.** `jwt.verify` checks the
  signature, algorithm, and time claims; it does not check `iss` or `aud`,
  because the expected values are the application's, not the library's. Use
  `jwt.verifyWith($token, $key, $alg, jwt.Policy{iss: "https://issuer.example",
  aud: "my-api"})` to additionally require them. An empty `iss` (or `aud`) skips
  that check; a non-empty one must match. `aud` may be a JSON string or an array
  of strings (RFC 7519), and the check passes when the expected audience is
  present.

`jwt.decode` and `jwt.header` do **not** verify anything - use them only to read
a token you have not trusted yet (for example, to read `kid` before fetching the
matching key). Never authorize on their output.

## Building claims

Claims are a `json.Value`, so build them however you build JSON - decode a
literal, or assemble one with the `json` write surface:

```jennifer
use json;
use time;
import "jwt.j" as jwt;

def exp as int init time.unix(time.now()) + 3600;      # one hour
def claims as json.Value init json.decode(
    "{\"sub\":\"ada\",\"role\":\"admin\",\"exp\":" + convert.toString($exp) + "}");
def token as string init jwt.sign($claims, $secret, "HS256");
```

## Keys by family

- **HS\***: any `bytes` secret. Use a long, random secret (`crypto.randBytes(32)`).
- **RS\* / ES\***: a PEM key as `bytes` - read it with `fs.readBytes("key.pem")`.
  Signing needs the private key, verifying needs the public key. Standard PEM
  encodings are accepted (RSA PKCS#1 or PKCS#8; EC SEC1 or PKCS#8; public keys in
  PKIX / `-----BEGIN PUBLIC KEY-----`).
- **EdDSA**: the `public` / `private` `bytes` from `crypto.signKeypair()`.

`jwt` does not generate RSA or EC keys - bring your own (from your identity
provider, or `openssl`). It generates Ed25519 keys through `crypto.signKeypair`.

## Selecting a key by `kid`

When an issuer rotates keys, each token's header carries a `kid` (key id) naming
which key signed it. `jwt.verifyWithKeys` reads that `kid`, looks it up in a
caller-supplied `map of string to string`, and verifies with the matching value
- an HMAC secret for HS\*, or PEM key text for RS\* / ES\* (each is decoded to
`bytes` for you). A token with no `kid`, or a `kid` absent from the map, is an
error - never a silent skip.

```jennifer
def keys as map of string to string init {
    "2024-key": "-----BEGIN PUBLIC KEY-----\n...\n-----END PUBLIC KEY-----\n",
    "2025-key": "-----BEGIN PUBLIC KEY-----\n...\n-----END PUBLIC KEY-----\n"
};
def claims as json.Value init jwt.verifyWithKeys($token, $keys, "RS256");
```

`alg` is still required and enforced against the header: selecting the key by
`kid` does **not** select the algorithm, so the algorithm-confusion protection
above stays in force. Pin `alg` to what the issuer uses.

For a raw **JWKS** endpoint (`{"keys":[{"kty":..,"kid":..,"n":..,"e":..}, ...]}`),
`jwt.verifyJwks` does the resolution for you: it finds the JWK whose `kid` matches
the header, converts it to a PEM public key with
[`crypto.jwkToPem`](../libraries/crypto.md), and verifies - no manual JWK-to-key
step. It is for asymmetric algorithms (RS\* / ES\*) only, since a JWKS carries
public keys; for HMAC use `verifyWithKeys` with the shared secret.

```jennifer
def jwks as string init http.get("https://issuer.example.com/.well-known/jwks.json", {}).body;
def claims as json.Value init jwt.verifyJwks($token, $jwks, "RS256");
```

**JWKS.** A raw JWKS endpoint publishes keys as JSON Web Keys
(`{"kty","kid","n","e"}` for RSA, `{"kty","kid","crv","x","y"}` for EC), not
PEM. `jwt` does not consume a JWKS directly: turning a JWK's modulus/exponent
(or EC point) into a verification key requires ASN.1 DER encoding that the
`crypto` surface does not expose, and hand-rolled DER in `.j` would be fragile
and a poor fit for security-sensitive code. Resolve the JWKS to a `kid -> PEM`
(or `kid -> secret`) map out of band - for example with your identity provider's
tooling or `openssl` - and pass that map to `verifyWithKeys`. A `crypto.jwkToPem`
helper that would let `jwt` ingest a JWKS in-process is a noted follow-up.

## JWT as web auth (`jwt_auth`)

There is no separate `jwt_auth` module - JWT authentication is this module used
as a [`web`](web.md) `before` middleware. Register a guard that pulls the bearer
token from the `Authorization` header, verifies it, and rejects on failure:

```jennifer
func requireJwt(ctx as web.Context) {
    def auth as string init web.header($ctx, "Authorization");
    if (not strings.startsWith($auth, "Bearer ")) {
        web.respond($ctx, 401, "missing bearer token");
        return false;                         # stop the chain
    }
    def token as string init strings.substring($auth, 7, len($auth));
    try {
        def claims as json.Value init jwt.verify($token, JWT_SECRET, "HS256");
        web.set($ctx, "user", json.asString($claims, "/sub"));
    } catch (e) {
        web.respond($ctx, 401, "invalid token");
        return false;
    }
    return true;                               # continue to the handler
}
```

## Platforms

HS\* (over `hash.hmac`) and `EdDSA` (over `crypto.sign` / `verify`) run on
**both binaries**. RS\* / ES\* need the `crypto` library's RSA / ECDSA surface,
which is on the default `jennifer` binary; on `jennifer-tiny` they raise a
friendly "not available" error (the same split as `net`).
