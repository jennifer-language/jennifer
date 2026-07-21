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
| `oauth.isExpired(token)`                      | Whether the token is past its expiry (30s skew buffer).          |
| `oauth.google(clientId, clientSecret, scope)` | A `Config` with Google's endpoints.                              |
| `oauth.microsoft(tenant, clientId, clientSecret, scope)` | A `Config` with a Microsoft 365 / Entra tenant's endpoints. |
| `oauth.save(path, token)` / `oauth.load(path)` | Persist / reload a token as JSON (via `fs`). `save` tightens the file to owner-only `0600`. |

## Flows

Three grants ship - the ones that need only `http` + `json`:

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

## Out of scope (later, dependency-gated)

- **Authorization Code + PKCE** - needs a local redirect server (`httpd`) to
  catch the callback and a crypto-grade random source for the PKCE verifier;
  lands with those.
- **Service-account JWT assertion** (Google) - an RSA-signed client assertion,
  so it waits on the `crypto` library.

## See also

- [sasl.md](sasl.md) - XOAUTH2, the use-a-token half.
- [http.md](http.md) / [json.md](../libraries/json.md) - what `oauth` composes over.
- [modules/index.md](index.md) - the module catalog and import rules.
