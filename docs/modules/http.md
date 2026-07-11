# `http` - an HTTP/1.1 client

Import with `import "http.j" as http;`. An **HTTP/1.1 client** over the `net`
system library: build a request (method, URL, headers, body), send it, and read
the response back into a `Response` (status, headers, body). `http://` connects
in the clear; `https://` connects with TLS (`net.connectTLS`). Because it uses
`net`, this module needs the default **`jennifer`** binary.

> **On `jennifer-tiny`:** "needs the default `jennifer` binary" refers to the
> **stock** tiny build, which ships without a network driver - not a TinyGo
> limitation. A `jennifer-tiny` rebuilt with a network stack runs this module
> too; see the
> [note on `net` and TinyGo](../technical/tinygo.md#net-on-tinygo-is-a-build-choice-not-a-hard-limit).

```jennifer
import "http.j" as http;

def r as http.Response init http.get("http://example.com/", {});
io.printf("status %d\n%s\n", $r.status, $r.body);

def sent as http.Response init http.post("https://api.example.com/items",
    "application/json", "{\"name\":\"ada\"}", {"Authorization": "Bearer xyz"});
```

Runnable: [`examples/modules/http_demo.j`](https://github.com/mplx/jennifer-lang/blob/main/examples/modules/http_demo.j).

## Surface

`headers` is a `map of string to string` (pass `{}` for none); a `body` is a
string (`""` for none).

| Call / type                                 | Notes                                                              |
| ------------------------------------------- | ------------------------------------------------------------------ |
| `http.Response`                             | `status` (int), `statusText`, `headers` (lowercased keys), `body`. |
| `http.request(method, url, headers, body)`  | The general request; returns a `Response`.                         |
| `http.get(url, headers)`                    | GET.                                                               |
| `http.post(url, contentType, body, headers)`| POST; sets `Content-Type`.                                         |
| `http.put(url, contentType, body, headers)` | PUT; sets `Content-Type`.                                          |
| `http.patch(url, contentType, body, headers)`| PATCH (partial update); sets `Content-Type`.                      |
| `http.delete(url, headers)`                 | DELETE.                                                            |
| `http.head(url, headers)`                   | HEAD (status + headers, no body).                                  |
| `http.options(url, headers)`                | OPTIONS (capability probe; read the `Allow` header).              |
| `http.header(resp, name)`                   | Read a response header case-insensitively, or `""` if absent.      |

The shortcuts are thin wrappers over `request`, which is **method-agnostic** -
it sends whatever method string you pass. So a method without a shortcut still
works: `http.request("TRACE", url, {}, "")` and the like. The one method that is
**not** supported is `CONNECT`: it is the HTTP tunneling primitive (after a
`200` the socket becomes a raw bidirectional tunnel), which needs a connection
hand-off this request/response-then-close client does not do.

## URLs and headers

A URL is parsed into scheme / host / port / path: `http://` defaults to port 80,
`https://` to 443, an explicit `:port` overrides, and the path (with any query
string) defaults to `/`. The `Host` header is set automatically (with the port
when non-default), along with `Connection: close` and a default `User-Agent`
(overridable by supplying your own).

Response header names are **lowercased** (HTTP header names are
case-insensitive), so `$r.headers["content-type"]` works regardless of how the
server cased it; `http.header($r, "Content-Type")` does the case-folding for you.

## Response body and framing

The client reads the whole response (it sends `Connection: close`, so the server
closes when done) and decodes the body, handling both framings:

- **Content-Length** - the body is taken as exactly that many bytes.
- **Transfer-Encoding: chunked** - the chunks are decoded and concatenated.

The body is returned as **text** (UTF-8). A JSON / HTML / XML body round-trips
exactly (the whole body is decoded as one unit, so it is byte-exact); a binary
body (an image, a gzip stream) is not decodable to a string and raises an error
- a `bytes` body accessor is a planned follow-on.

## Errors

A malformed response, a body that is not valid UTF-8, or a network failure
raises a positioned error (a `throw`n `Error` for a malformed response, kind
`"http"`); wrap a request in `try` / `catch` to handle a down server. A non-2xx
**status is not an error** - a 404 or 500 comes back as a normal `Response` with
that `status`, for the caller to branch on.

## Out of scope

- **Redirects are returned, not followed.** A 3xx comes back with its
  `Location` header; follow it yourself (auto-follow with a hop limit is a later
  add).
- **One request per connection.** No keep-alive, no connection pool.
- **No cookie jar, no automatic decompression, no multipart builder.** Set the
  headers and body you need directly.
- **Text bodies.** Binary responses need the planned `bytes` accessor.

## See also

- [net.md](../libraries/net.md) - the transport (and its TLS) `http` builds on.
- [json.md](../libraries/json.md) - encode / decode JSON request and response
  bodies.
- [modules/index.md](index.md) - the module catalog and import rules.
