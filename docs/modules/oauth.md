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
use httpd;
use io;
import "oauth.j" as oauth;
import "sasl.j" as sasl;

def cfg as oauth.Config init oauth.discover("https://accounts.google.com",
    "client-id", "client-secret", "https://mail.google.com/");
def srv as httpd.Server init httpd.listen("127.0.0.1:0");
def req as oauth.AuthRequest init oauth.authorize($cfg,
    "http://" + httpd.address($srv) + "/callback");
io.printf("open %s\n", $req.url);                           # the user signs in
def tok as oauth.Token init oauth.exchangeCode($cfg, $req, oauth.awaitCode($req, $srv));
httpd.shutdown($srv);
# use the token, e.g. for IMAP: sasl.bearer("me@gmail.com", $tok.accessToken)
```

Google allows Gmail only through the browser sign-in: its device flow accepts
a short list of scopes (sign-in, profile, some Drive and YouTube). Microsoft
allows IMAP and SMTP through both.

Runnable: [`examples/modules/oauth_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/oauth_demo.j).

## Surface

| Call / type                                   | Notes                                                            |
| --------------------------------------------- | ---------------------------------------------------------------- |
| `oauth.Config`                                | `tokenUrl`, `deviceUrl`, `authorizeUrl`, `clientId`, `clientSecret`, `scope` (`deviceUrl` / `authorizeUrl` `""` when unused). |
| `oauth.Token`                                 | `accessToken`, `tokenType`, `refreshToken`, `scope`, `expiresAt` (Unix seconds; 0 = unknown), `idToken` (OpenID Connect; `""` if none), `extra` (the reply's other top-level string / number / boolean fields, as text). |
| `oauth.DeviceAuth`                            | `deviceCode`, `userCode`, `verificationUri`, `interval`, `expiresAt`. |
| `oauth.AuthRequest`                           | `url`, `state`, `verifier`, `redirectUri`: one authorization-code sign-in in progress. |
| `oauth.DevicePoll`                            | `state` (`"ok"`, `"pending"`, `"slow_down"`, `"failed"`), `token`, `errorCode`, `description`, `status`: one device-flow poll. |
| `oauth.Discovery`                             | `config`, `userinfoUrl`, `metadata` (the whole document, a `json.Value`). |
| `oauth.discover(issuer, clientId, clientSecret, scope)` | Read the provider's metadata (OpenID Connect Discovery, then RFC 8414) -> `Config`. |
| `oauth.discoverMetadata(issuer, clientId, clientSecret, scope)` | `discover`, also returning the UserInfo endpoint and the document -> `Discovery`. |
| `oauth.clientCredentials(config)`             | The Client Credentials grant (a service as itself, with its secret) -> `Token`. |
| `oauth.clientAssertion(alg, clientId, audience, key, kid)` | A signed client assertion (RFC 7523 section 2.2, `private_key_jwt`) -> `string`. |
| `oauth.clientCredentialsJwt(config, assertion)` | The Client Credentials grant, authenticated by the assertion instead of the secret -> `Token`. |
| `oauth.authorize(config, redirectUri)`        | Start an authorization-code sign-in with PKCE -> `AuthRequest` (open its `url` in a browser). |
| `oauth.authorizeWith(config, redirectUri, extra)` | `authorize` with more URL parameters (`access_type`, `prompt`, `nonce`, `login_hint`, ...). |
| `oauth.awaitCode(req, server)`                | Catch the redirect on a loopback `httpd` server -> the code. |
| `oauth.codeFromRedirect(req, redirectedUrl)`  | The code from a redirected URL, its `state` checked. |
| `oauth.exchangeCode(config, req, code)`       | Trade the code (with the PKCE verifier) for a `Token`. |
| `oauth.exchangeCodeWith(config, code, verifier, redirectUri)` | `exchangeCode` with a verifier and redirect made elsewhere (a relaying server). |
| `oauth.refresh(config, refreshToken)`         | Trade a refresh token for a new `Token` (keeps the refresh token when the server omits it). |
| `oauth.deviceStart(config)`                   | Begin the Device Authorization grant -> `DeviceAuth` (show the user the URL + code). |
| `oauth.deviceWait(config, deviceAuth)`        | Poll until the user approves -> `Token` (backs off on `slow_down`). |
| `oauth.devicePoll(config, deviceAuth)`        | Poll once, without waiting -> `DevicePoll`. |
| `oauth.jwtAssertion(alg, issuer, subject, audience, lifetime, extra, key, kid)` | Build a signed RFC 7523 assertion (`iss` / `sub` / `aud` / `iat` / `exp` plus the `extra` claims map) -> `string`. |
| `oauth.jwtBearer(config, assertion)`          | The JWT Bearer grant (RFC 7523): trade the assertion for a `Token`. |
| `oauth.isExpired(token)`                      | Whether the token is past its expiry (30s skew buffer).          |
| `oauth.save(path, token)` / `oauth.load(path)` | Persist / reload a token as JSON (via `fs`). `save` writes an owner-only `0600` file and renames it into place. |

## Flows

Five grants ship:

- **Client Credentials** - `clientCredentials(config)`: a service authenticates
  as itself (no user), for machine-to-machine APIs. `clientCredentialsJwt`
  does the same with a signed client assertion instead of a secret.
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
- **Authorization Code with PKCE** - `authorize`, then `awaitCode` (or
  `codeFromRedirect`), then `exchangeCode`: the browser sign-in, for providers
  and scopes that offer no device flow.

Every grant posts the client secret, a refresh token or a code, so the token
and device endpoints must be https; plain http is accepted only on this
machine (`127.0.0.1`, `localhost`, `[::1]`), for a test server. A `Config`
built by hand is checked when it is used. A reply may be at most 1 MiB, a read
may stall at most 15 s, and a request takes at most 60 s in all; a breach is an
`oauth` error, and the abandoned request's socket is closed, so a hostile or
broken endpoint can neither fill memory nor hold the program or a connection.
`deviceWait` never sleeps past the device code's expiry.

## Relaying a sign-in

A server that signs its own clients in through a provider (a package registry
whose command-line client logs in with GitHub, say) holds no sign-in from
start to end: each step arrives as a separate request. Three calls fit that:

- `oauth.devicePoll(config, deviceAuth)` makes **one** poll and returns a
  `DevicePoll` instead of waiting: `state` is `"ok"` (with `token`),
  `"pending"`, `"slow_down"`, or `"failed"` with the provider's `errorCode`
  (`access_denied`, `expired_token`, ...) and the HTTP `status`, so the server
  can tell a refusal from a provider fault (GitHub answers an already used
  device code with an HTML 500 page: `errorCode` `""`, `status` 500). Keeping
  to `interval` and `expiresAt` is the caller's job. A `DeviceAuth` rebuilt
  from its fields works as well as one from `deviceStart`.
- `oauth.exchangeCodeWith(config, code, verifier, redirectUri)` trades a code
  its client caught, with the client's PKCE verifier (43 to 128 characters,
  RFC 7636) and redirect URI.
- `oauth.discoverMetadata(issuer, clientId, clientSecret, scope)` is
  `discover` returning a `Discovery`: the `Config`, the OpenID Connect
  `userinfoUrl` (checked like the other endpoints; `""` when not named), and
  the whole `metadata` document. Keep it from startup to skip the fetch on
  each sign-in.

## Discovery

`oauth.discover(issuer, clientId, clientSecret, scope)` fetches the provider's
metadata document and returns a `Config` with its token, device and
authorization endpoints. It tries the OpenID Connect location
(`<issuer>/.well-known/openid-configuration`), then RFC 8414's
(`/.well-known/oauth-authorization-server` before the issuer's path). The
document must name exactly the issuer asked for, and the issuer and every
endpoint must be https; plain http is accepted only on this machine
(`127.0.0.1`, `localhost`, `[::1]`), for a test server. Most providers publish
one - Google (`https://accounts.google.com`), Microsoft Entra ID for one
tenant (`https://login.microsoftonline.com/<tenant-id>/v2.0`), Okta, Auth0,
Keycloak, Oracle Identity Domains, AWS Cognito - so a preset is rarely needed.
Entra ID's multi-tenant `common` metadata names a placeholder issuer
(`.../{tenantid}/v2.0`), which `discover` refuses; use the
`@jennifer/oauthext` preset for it.

## Client assertions

A confidential client can prove who it is with a JWT signed by its own key
instead of a shared secret (RFC 7523 section 2.2; OpenID Connect calls it
`private_key_jwt`). `oauth.clientAssertion(alg, clientId, audience, key, kid)`
builds it: `iss` and `sub` the client id, `aud` the token endpoint, a random
`jti`, valid for five minutes. `oauth.clientCredentialsJwt(config, assertion)`
sends it with `client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer`;
the client secret is not sent. Make a new assertion for each request.

```jennifer
def a as string init oauth.clientAssertion("RS256", $cfg.clientId, $cfg.tokenUrl,
    fs.readBytes("client-key.pem"), "key-1");
def tok as oauth.Token init oauth.clientCredentialsJwt($cfg, $a);
```

## Authorization Code with PKCE

The sign-in most providers offer to programs running on the user's machine:
the user signs in in their browser, and the provider redirects the browser back
to the program with a one-time code. `oauth.authorize(config, redirectUri)`
makes a random `state` and a PKCE code verifier (RFC 7636, S256) and returns an
`AuthRequest` whose `url` the user opens. RFC 8252 advises a loopback redirect
(`http://127.0.0.1:<port>/<path>`, registered with the provider); listen there
with [`httpd`](../libraries/httpd.md) and let `oauth.awaitCode(req, server)`
catch the redirect. It answers the browser with a short page, ignores requests
for another path or with another `state`, and returns the code, or raises the
provider's error (`authorization refused: access_denied: ...`). A program that
receives the redirect another way passes the URL to
`oauth.codeFromRedirect(req, url)`. `oauth.exchangeCode(config, req, code)`
then trades the code, with the verifier, for a `Token`.

```jennifer
def srv as httpd.Server init httpd.listen("127.0.0.1:8765");
def req as oauth.AuthRequest init oauth.authorize($cfg, "http://127.0.0.1:8765/callback");
io.printf("open %s\n", $req.url);
def tok as oauth.Token init oauth.exchangeCode($cfg, $req, oauth.awaitCode($req, $srv));
httpd.shutdown($srv);
```

`awaitCode` blocks until the redirect arrives; run it in a task and
`task.waitTimeout` to bound the wait, and `httpd.shutdown` the server on a
timeout so the waiting task ends too.

The checks: the authorization URL must be https (plain http only on this
machine); the redirect URI https, http on this machine (RFC 8252 section 7.3),
or a private-use scheme named like a reverse domain (`com.example.app:/cb`,
section 7.1), with no fragment. A redirect whose `state` differs, or that
repeats a parameter (RFC 6749 section 3.1), is refused; `codeFromRedirect` also
refuses a URL that goes elsewhere than the redirect URI. `awaitCode` answers
such requests and keeps waiting, however many arrive, so a stray or forged
request cannot end the sign-in. To give up, shut the server down from the task
that waits with `task.waitTimeout`. A loopback redirect URI's path may not
hold a `%`-escape: `awaitCode` compares it with the decoded path the browser
asks for, so `/caf%C3%A9` would never match (and `%2F` has no single decoded
form).

`oauth.authorizeWith(config, redirectUri, extra)` adds parameters to the
authorization URL: OpenID Connect's `nonce`, `login_hint` and `prompt`, or a
provider's own. Google issues a refresh token only when asked for offline
access:

```jennifer
def req as oauth.AuthRequest init oauth.authorizeWith($cfg, $redirect,
    {"access_type": "offline", "prompt": "consent"});
```

The extras may not replace the parameters `authorize` sets itself.

## Token fields

Besides the standard fields, a `Token` keeps the OpenID Connect `id_token` as
`idToken` (verify it with [`jwt.verifyJwks`](jwt.md) before trusting its
claims) and the reply's other top-level string, number and boolean fields in
`extra`, as text: Salesforce's `instance_url`, Microsoft's `ext_expires_in`.
`save` / `load` keep both; a file saved before 0.26.0 loads with them empty.

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
def a as string init oauth.jwtAssertion("RS256", json.asString($key, "/client_email"),
    "", $tokenUrl, 3600,
    $extra,
    convert.bytesFromString("utf-8", json.asString($key, "/private_key")), json.asString($key, "/private_key_id"));
def cfg as oauth.Config init oauth.Config{tokenUrl: $tokenUrl, deviceUrl: "",
    authorizeUrl: "", clientId: "", clientSecret: "", scope: ""};
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
token is a bearer credential, so `save` writes it to a new owner-only `0600`
file beside `path`, flushes it to the disk, and renames it over `path`, the
file-permission model `gh` / `aws` use for their token stores. A crash or a
power loss mid-write leaves the old file or the new one, and a symbolic link at
`path` is replaced, not followed.

## Feeding mail auth

OAuth2's other half is presenting the token. For mail, that is SASL XOAUTH2:
pass `token.accessToken` to [`sasl.bearer`](sasl.md), and the result drives an
SMTP / IMAP `AUTHENTICATE XOAUTH2`. So `oauth` (get the token) + `sasl` (use it)
+ the mail clients compose into modern Google / Microsoft 365 mail access.

## Errors

A token-endpoint error (`{"error":"...","error_description":"..."}`) throws a
catchable `Error` (kind `"oauth"`) with the code and description, control
characters as `?`. So does a reply the module cannot trust: not JSON (the
message gives the HTTP status), another status than 200 without an `error`, no
`access_token` or an empty one, no `token_type` or an empty one (RFC 6749
section 5.1 requires it), a field of the wrong type (`"expires_in":"3599"`,
`"scope":null`), an `expires_in` that is not a whole number from 1 s to ten
years, or a reply over 1 MiB. A device-authorization reply is checked the same
way, and also refuses a `user_code` over 64 characters or with a control
character, a verification URI that is not https, an `interval` over ten
minutes and an `expires_in` over a day. A grant whose endpoint is not https
(or http on this machine) is refused before anything is sent. A request that
gets no usable reply - a refused or reset connection, a DNS or TLS failure, a
malformed status line, headers over 1 MiB, a stall or the 60 s limit - is an
`oauth` error too (`the request to <url> failed: ...`), so one `catch` on kind
`"oauth"` covers every way a grant can fail. A non-terminal device-poll status
(`authorization_pending`, `slow_down`) is handled internally by `deviceWait`,
not surfaced; a poll answered by an HTML 5xx page is retried, any other reply
without JSON ends `deviceWait` (`the reply is not JSON (HTTP 200)`) instead of
polling on until the code expires. `load` refuses a missing or unreadable
file, and a saved token with an empty access token or type or a negative
expiry.

## Out of scope

- **Verifying ID tokens** - `idToken` is kept as received; check it with
  [`jwt.verifyJwks`](jwt.md) against the provider's `jwks_uri`.
- **Anything provider-specific** - endpoint presets, Google's
  service-account key files, a provider's claim conventions. They are in the
  [`@jennifer/oauthext`](https://jennifer-language.github.io/deck-oauthext/)
  deck (`oauthext.google`, `oauthext.microsoft`,
  `oauthext.googleServiceAccount`), so a new provider is a deck change, not an
  interpreter one.

## Provider presets moved out (0.26.0)

`oauth.google` and `oauth.microsoft` were removed in 0.26.0. Use
`oauthext.google` / `oauthext.microsoft` from the
[`@jennifer/oauthext`](https://jennifer-language.github.io/deck-oauthext/)
deck, which take the same parameters (`jvc add @jennifer/oauthext`, then
change the prefix), or `oauth.discover` with the provider's issuer URL.

`Config` gained `authorizeUrl` and `Token` gained `idToken` and `extra` in
0.26.0. A struct literal must name every field, so code that builds a `Config`
or `Token` by hand adds them (`authorizeUrl: ""`, `idToken: ""`, `extra: {}`).

## See also

- [sasl.md](sasl.md) - XOAUTH2, the use-a-token half.
- [jwt.md](jwt.md) - signs the JWT Bearer assertion.
- [http.md](http.md) / [json.md](../libraries/json.md) - what `oauth` composes over.
- [modules/index.md](index.md) - the module catalog and import rules.
