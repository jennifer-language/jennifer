# `oauth` - a generic OAuth2 client

Import with `import "oauth.j" as oauth;`. The **get-a-token** half of OAuth2
(the *use-a-token* half is [`sasl`](sasl.md) XOAUTH2). It acquires and refreshes
access tokens against any OAuth2 token endpoint - not email-specific, any
OAuth2-protected API - over [`http`](http.md) + [`json`](../libraries/json.md).
Because it builds on `http` (which uses `net`), this module needs the default
**`jennifer`** binary.

> **On `jennifer-tiny`:** "needs the default `jennifer` binary" refers to the
> **stock** tiny build, which ships without a network driver - not a TinyGo
> limitation. A `jennifer-tiny` rebuilt with a network stack runs this module
> too; see the
> [note on `net` and TinyGo](../technical/tinygo.md#net-on-tinygo-is-a-build-choice-not-a-hard-limit).

```jennifer
import "oauth.j" as oauth;
import "sasl.j" as sasl;

def cfg as oauth.Config init oauth.google("client-id", "client-secret",
    "https://mail.google.com/");
def dev as oauth.DeviceAuth init oauth.deviceStart($cfg);
io.printf("visit %s and enter %s\n", $dev.verificationUri, $dev.userCode);
def tok as oauth.Token init oauth.deviceWait($cfg, $dev);   # blocks until approved
# use the token, e.g. for IMAP: sasl.bearer("me@gmail.com", $tok.accessToken)
```

Runnable: [`examples/modules/oauth_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/oauth_demo.j).

## Surface

| Call / type                                   | Notes                                                            |
| --------------------------------------------- | ---------------------------------------------------------------- |
| `oauth.Config`                                | `tokenUrl`, `deviceUrl`, `clientId`, `clientSecret`, `scope`.    |
| `oauth.Token`                                 | `accessToken`, `tokenType`, `refreshToken`, `scope`, `expiresAt` (Unix seconds; 0 = unknown). |
| `oauth.DeviceAuth`                            | `deviceCode`, `userCode`, `verificationUri`, `interval`, `expiresAt`. |
| `oauth.clientCredentials(config)`             | The Client Credentials grant (a service as itself) -> `Token`.   |
| `oauth.refresh(config, refreshToken)`         | Trade a refresh token for a new `Token` (keeps the refresh token when the server omits it). |
| `oauth.deviceStart(config)`                   | Begin the Device Authorization grant -> `DeviceAuth` (show the user the URL + code). |
| `oauth.deviceWait(config, deviceAuth)`        | Poll until the user approves -> `Token` (backs off on `slow_down`). |
| `oauth.jwtAssertion(issuer, subject, audience, lifetime, extra, key, alg, kid)` | Build a signed RFC 7523 assertion (`iss` / `sub` / `aud` / `iat` / `exp` plus the `extra` claims map) -> `string`. |
| `oauth.jwtBearer(config, assertion)`          | The JWT Bearer grant (RFC 7523): trade the assertion for a `Token`. |
| `oauth.isExpired(token)`                      | Whether the token is past its expiry (30s skew buffer).          |
| `oauth.google(clientId, clientSecret, scope)` | A `Config` with Google's endpoints.                              |
| `oauth.microsoft(tenant, clientId, clientSecret, scope)` | A `Config` with a Microsoft 365 / Entra tenant's endpoints. |
| `oauth.save(path, token)` / `oauth.load(path)` | Persist / reload a token as JSON (via `fs`). `save` tightens the file to owner-only `0600`. |

## Flows

Four grants ship:

- **Client Credentials** - `clientCredentials(config)`: a service authenticates
  as itself (no user), for machine-to-machine APIs.
- **Refresh Token** - `refresh(config, refreshToken)`: exchange a long-lived
  refresh token for a fresh access token. The reply often omits the refresh
  token; the module carries the old one forward so the returned `Token` always
  has one.
- **Device Authorization Grant** - `deviceStart` then `deviceWait`: the
  CLI-friendly flow. There is no local redirect server - you show the user a
  URL and a short code, they approve in a browser, and `deviceWait` polls the
  token endpoint (honouring the server's `interval` and `slow_down`) until it
  returns a token.
- **JWT Bearer** (RFC 7523) - `jwtAssertion` then `jwtBearer`: the client signs
  a short-lived JWT with its own private key and trades it for an access token.
  This is how **service accounts** authenticate (Google Cloud, Salesforce, Okta,
  and other RFC 7523 providers), with no client secret and no user.

## JWT Bearer (service accounts)

`oauth.jwtAssertion` builds the standard claim set - `iss` (the client or
service-account id), `sub` (the user acted for; `""` leaves it out), `aud`
(usually the token endpoint URL), `iat` = now and `exp` = now + `lifetime`
seconds - adds the provider's `extra` claims (a `json.Value` map: a `scope`, a
`jti`, ...; it may not set the five above), and signs it through
[`jwt.signWithKid`](jwt.md) with `alg` and an optional header `kid`.
`oauth.jwtBearer` posts it with
`grant_type=urn:ietf:params:oauth:grant-type:jwt-bearer`, plus `config.scope`
when set and the client id / secret only when the config has them (the signed
assertion usually authenticates the client by itself).

The module stays provider-neutral: what goes into the claims is the provider's
convention. For example, a Google service account puts its scopes in the
assertion and names its key in `kid`, with the values from its JSON key file:

```jennifer
def key as json.Value init json.decode(fs.readString("service-account.json"));
def tokenUrl as string init json.asString($key, "/token_uri");
def extra as json.Value init json.set(json.map(), "/scope",
    "https://www.googleapis.com/auth/cloud-platform");
def a as string init oauth.jwtAssertion(json.asString($key, "/client_email"), "",
    $tokenUrl, 3600, $extra,
    convert.bytesFromString(json.asString($key, "/private_key"), "utf-8"),
    "RS256", json.asString($key, "/private_key_id"));
def cfg as oauth.Config init oauth.Config{tokenUrl: $tokenUrl, deviceUrl: "",
    clientId: "", clientSecret: "", scope: ""};
def tok as oauth.Token init oauth.jwtBearer($cfg, $a);
```

An assertion is good for its `lifetime` only; mint a new one for each token
request rather than reusing it (providers bound the lifetime, often between a
few minutes and an hour). RS\* / ES\* signing needs the default `jennifer`
binary, as the whole module already does.

## Expiry and refresh

A `Token` carries `expiresAt` (a Unix timestamp, computed from the response's
`expires_in`). `oauth.isExpired($token)` reports whether it is past due (with a
30-second skew buffer), so a caller can refresh proactively:

```jennifer
if (oauth.isExpired($tok)) {
    $tok = oauth.refresh($cfg, $tok.refreshToken);
}
```

`oauth.save` / `oauth.load` persist a token to disk (JSON via `fs`) so a
long-running or restarted program keeps its refresh token. An access / refresh
token is a bearer credential, so `save` tightens the file to owner-only `0600`
after writing (via `fs.chmod`), the same file-permission model `gh` / `aws` use
for their token stores.

## Feeding mail auth

OAuth2's other half is presenting the token. For mail, that is SASL XOAUTH2:
pass `token.accessToken` to [`sasl.bearer`](sasl.md), and the result drives an
SMTP / IMAP `AUTHENTICATE XOAUTH2`. So `oauth` (get the token) + `sasl` (use it)
+ the mail clients compose into modern Google / Microsoft 365 mail access. The
provider presets (`google`, `microsoft`) exist to make that the headline case.

## Errors

A token-endpoint error (`{"error":"...","error_description":"..."}`) throws a
catchable `Error` (kind `"oauth"`) with the code and description. A non-terminal
device-poll status (`authorization_pending`, `slow_down`) is handled internally
by `deviceWait`, not surfaced.

## Out of scope

- **Authorization Code + PKCE** - not provided. The pieces exist (an
  [`httpd`](../libraries/httpd.md) redirect listener to catch the callback,
  `crypto.randBytes` for the PKCE verifier, `hash.compute` for the S256
  challenge), but the flow is not wired into this module.
- **Reading a provider's key file** (Google's service-account JSON, ...) - a
  provider format, not part of OAuth2; read it with `json` as in the example
  above.

## See also

- [sasl.md](sasl.md) - XOAUTH2, the use-a-token half.
- [jwt.md](jwt.md) - signs the JWT Bearer assertion.
- [http.md](http.md) / [json.md](../libraries/json.md) - what `oauth` composes over.
- [modules/index.md](index.md) - the module catalog and import rules.
