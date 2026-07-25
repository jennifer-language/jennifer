# `rest` - an ergonomic REST client

Import with `import "rest.j" as rest;`. A REST convenience layer over the
[`http`](http.md) client and [`json`](../libraries/json.md): hold a
value-semantic `Client` (base URL + default headers) and call JSON-aware verbs.
It is **pure composition** - base-URL joining, query strings, `Content-Type`,
and auth headers are string / map work; the transport (verbs, TLS, framing) is
`http`'s and the bodies are `json`'s. Because it builds on `http` (which uses
`net`), this module needs the default **`jennifer`** binary.

> **On `jennifer-tiny`:** "needs the default `jennifer` binary" refers to the
> **stock** tiny build, which ships without a network driver - not a TinyGo
> limitation. A `jennifer-tiny` rebuilt with a network stack runs this module
> too; see the
> [note on `net` and TinyGo](../technical/tinygo.md#net-on-tinygo-is-a-build-choice-not-a-hard-limit).

```jennifer
import "rest.j" as rest;
use json;

def api as rest.Client init rest.withHeader(rest.client("https://api.example.com"),
    "Authorization", rest.bearer("my-token"));

def user as json.Value init rest.getJson($api, "/users/1", {});
def created as rest.Response init rest.postJson($api, "/users",
    json.decode("{\"name\":\"ada\"}"));
io.printf("created -> %d\n", $created.status);
```

Runnable: [`examples/modules/rest_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/rest_demo.j).

## Surface

The `Client` is value-semantic: pass it to each call, auth lives in its
`headers`. Every verb returns a `rest.Response` (`status`, lowercased `headers`,
`body`), except the `*Json` reads which decode the body to a `json.Value`.

| Call / type                                   | Notes                                                            |
| --------------------------------------------- | ---------------------------------------------------------------- |
| `rest.Client`                                 | `baseUrl`, default `headers`, and `tls` options sent with every request. |
| `rest.Response`                               | `status`, `headers`, `body`.                                     |
| `rest.client(baseUrl)`                        | Build a client: no default headers, full TLS verification.       |
| `rest.get(c, path, query)`                    | GET; `query` is a `map of string to string` ({} for none).       |
| `rest.delete(c, path, query)`                 | DELETE.                                                          |
| `rest.post(c, path, contentType, body)`       | POST with a raw body.                                            |
| `rest.put(c, path, contentType, body)`        | PUT with a raw body.                                             |
| `rest.patch(c, path, contentType, body)`      | PATCH with a raw body.                                           |
| `rest.getJson(c, path, query)`                | GET, decode the body -> `json.Value`.                            |
| `rest.postJson(c, path, body)`                | POST a `json.Value` (encodes, sets `Content-Type`); -> `Response`. |
| `rest.putJson(c, path, body)`                 | PUT a `json.Value`.                                              |
| `rest.patchJson(c, path, body)`               | PATCH a `json.Value`.                                            |
| `rest.bearer(token)`                          | An `Authorization` value: `Bearer <token>`.                      |
| `rest.basic(user, pass)`                      | An `Authorization` value: `Basic <base64(user:pass)>`.           |
| `rest.withHeader(c, name, value)`             | A copy of the client with one default header set.                |
| `rest.withCA(c, pem)`                         | A copy trusting a private-CA / self-signed PEM cert (`bytes`) for `https://`. The safer TLS opt-out: the server is still authenticated. |
| `rest.insecure(c)`                            | A copy that skips TLS certificate verification (accepts any cert). Disables authentication - trusted-LAN endpoints only; prefer `withCA`. |

## URLs, queries, and auth

- **Base-URL joining** puts exactly one slash between `baseUrl` and `path`, so
  `"https://api"` + `"/users"` and `"https://api/"` + `"users"` both give
  `https://api/users` - no double slashes.
- **Query strings** are built from a `map of string to string` and
  percent-encoded (`{"q": "a b"}` -> `?q=a%20b`).
- **Auth** is a header: set `Client.headers["Authorization"]` to `rest.bearer(token)`
  or `rest.basic(user, pass)` when building the client, or add it later with
  `rest.withHeader`. Basic base64-encodes `user:pass` through `encoding`.

## TLS (self-signed / private CA)

Every `https://` request full-verifies the server certificate by default. A LAN
appliance (Proxmox, Synology, an internal service) usually ships a self-signed
cert, which that default rejects. Two per-client opt-outs, both returning a copy
(value semantics, like `withHeader`):

- **`rest.withCA(client, pem)`** trusts a specific PEM certificate (`bytes`, e.g.
  from `fs.readBytes`) in addition to the system roots. **Preferred** - the
  server is still authenticated, just against the appliance's own CA.
- **`rest.insecure(client)`** accepts *any* certificate. This disables server
  authentication and exposes the connection to a man-in-the-middle; use it only
  for a trusted LAN endpoint you cannot give a proper CA.

```jennifer
use fs;
def ca as bytes init fs.readBytes("appliance-ca.pem");
def api as rest.Client init rest.withCA(rest.client("https://192.168.1.10"), $ca);
# or, last resort on a trusted LAN:
def loose as rest.Client init rest.insecure(rest.client("https://192.168.1.10"));
```

## Errors

A non-2xx **status is a value, not a crash**: a 404 or 500 comes back as a
normal `Response` with that `status` for you to branch on. `getJson` will,
however, `throw` if the body is not valid JSON (e.g. an HTML error page) - guard
with `get` + a status check when a server may return non-JSON errors.

## Out of scope

- **No cookie jar / stateful session.** A `Client` is a value threaded per call
  (a module has no mutable state); a stateful session that remembers cookies
  belongs on the `http` side (a system library may hold stateful handles; a
  module may not) and is deferred there.
- **No retry / backoff, no pagination helper, no auto-redirect** (redirects come
  back as their 3xx `Response`, from `http`).

## See also

- [http.md](http.md) - the client `rest` composes over.
- [json.md](../libraries/json.md) - the `json.Value` request / response bodies.
- [modules/index.md](index.md) - the module catalog and import rules.
