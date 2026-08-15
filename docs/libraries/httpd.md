# `httpd` - HTTP server engine

Enable with `use httpd;`. An HTTP/1.1 server engine wrapping Go's `net/http`,
so keep-alive, chunked transfer, TLS (and HTTP/2 over TLS), request timeouts,
and graceful shutdown come from the battle-tested Go stack rather than being
re-implemented in the interpreter. It is the server counterpart to the `net`
client primitives and the [`http`](../modules/http.md) client module.

**Default binary only.** Like `net`, `httpd` needs a network stack, so it runs
on the standard `jennifer` build; on `jennifer-tiny` every call returns a
friendly error (TinyGo ships no netdev driver). See
[technical/tinygo.md](../technical/tinygo.md).

## The pull loop

You cannot hand Go's `net/http` a Jennifer request-handler callback: the
interpreter is not re-entered from Go's handler goroutines. Instead the engine
accepts and parses requests concurrently on Go's
side and hands them to your program **one at a time**: `httpd.accept` blocks
for the next request, and `httpd.respond` answers it.

```jennifer
use httpd;

def srv as httpd.Server init httpd.listen("127.0.0.1:8080");
while (true) {
    def req as httpd.Request init httpd.accept($srv);
    httpd.respond($req, 200, "hello\n");
}
```

The two concurrency worlds stay cleanly separate: **Go owns the I/O
concurrency** (accepting, parsing, keep-alive), and your program stays a simple
serial loop. When you *want* per-request parallelism, opt into it with your own
`spawn` - several `spawn`ed workers can each call `httpd.accept` on the same
server handle to form a worker pool, since the handle's state is shared:

```jennifer
use httpd;
use task;

def srv as httpd.Server init httpd.listen("127.0.0.1:8080");
def workers as list of task of null init [];
for (def i in lists.range(0, 4)) {
    $workers[] = spawn {
        while (true) {
            def req as httpd.Request init httpd.accept($srv);
            httpd.respond($req, 200, "handled by a worker\n");
        }
    };
}
```

## Surface

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `httpd.listen(addr)` | `httpd.Server` | Start listening. `"127.0.0.1:8080"` (TCP), `":0"` (ephemeral TCP port), or `"unix:/run/app.sock"` (a Unix domain socket). |
| `httpd.listenTLS(addr, cert, key)` | `httpd.Server` | HTTPS; `cert` / `key` are PEM `bytes`. HTTP/2 negotiated automatically. |
| `httpd.address(srv)` | `string` | The actual bound address (resolve `":0"` to the chosen port). |
| `httpd.accept(srv)` | `httpd.Request` | Block for the next request. Errors once the server is shut down. |
| `httpd.method(req)` | `string` | `"GET"`, `"POST"`, ... |
| `httpd.path(req)` | `string` | URL path, e.g. `/users/42`. |
| `httpd.query(req, name)` | `string` | Query parameter (`""` if absent). |
| `httpd.header(req, name)` | `string` | Request header (`""` if absent; case-insensitive name). |
| `httpd.body(req)` | `bytes` | The request body (buffered; a body over the 10 MiB cap is answered 413 by the engine). |
| `httpd.remoteAddr(req)` | `string` | Client `host:port`. |
| `httpd.setHeader(req, name, value)` | `null` | Set a response header (before `respond`). |
| `httpd.respond(req, status, body)` | `null` | Send the response; `body` is a `string` or `bytes`. |
| `httpd.serveFile(req, path)` | `null` | Answer with a file (content type, range requests handled by `net/http`). |
| `httpd.serveDir(req, root)` | `null` | Answer with the file under `root` matching the request path (`..` cannot escape `root`). |
| `httpd.shutdown(srv)` | `null` | Graceful drain: stop accepting, unblock parked `accept` calls, finish in-flight requests. |

Each request must be answered **exactly once** - a second `respond` /
`serveFile` / `serveDir` on the same request, or a `setHeader` after the
answer, is an error.

## Handles

`httpd.Server` and `httpd.Request` are `{id as int}` handles into a Go-side
registry (the same pattern as `fs`, `net`, `os.Process`): value-semantic to
copy, but every copy refers to the same underlying server / request. That is
what lets a copied `Server` handle inside a `spawn` worker pull from the same
accept queue.

## A tiny JSON API

Everything the engine hands you is a value, so the rest of the standard library
composes normally - here, `json` for the response body:

```jennifer
use httpd;
use json;

def srv as httpd.Server init httpd.listen(":8080");
while (true) {
    def req as httpd.Request init httpd.accept($srv);
    def out as json.Value init json.map();
    $out = json.set($out, "/method", httpd.method($req));
    $out = json.set($out, "/path", httpd.path($req));
    httpd.setHeader($req, "Content-Type", "application/json");
    httpd.respond($req, 200, json.encode($out));
}
```

## Static files

```jennifer
use httpd;
def srv as httpd.Server init httpd.listen(":8080");
while (true) {
    def req as httpd.Request init httpd.accept($srv);
    httpd.serveDir($req, "./public");
}
```

`serveDir` cleans the request path so a `../` cannot climb above `root`, rejects
a request path containing a backslash (`400`), and re-verifies the joined path is
still under `root` before serving it (`404` otherwise); `serveFile` answers with
one specific file regardless of the request path.

**Symlinks are followed.** Both verbs open the resolved path directly (Go's
`http.ServeFile` behaviour), so a symbolic link *inside* `root` that points
outside it exposes its target. If the served tree can contain links created by
another user or an upload feature, resolve and containment-check the path
yourself before serving, or serve from a directory you fully control.

## Graceful shutdown

`httpd.shutdown` closes the listener, wakes any workers blocked in
`httpd.accept` (they get an error so their loops can exit), and lets in-flight
requests finish before returning. A typical server installs a signal handler
(via `os`) that calls `shutdown`, or shuts down after a sentinel request.

## Behind a reverse proxy (nginx)

In production an `httpd` / `web` app usually sits behind nginx, which
terminates TLS, serves static assets, buffers slow clients, and can load
balance. nginx speaks plain HTTP to the app over either a **TCP port** or a
**Unix domain socket** - `httpd.listen` supports both.

**TCP port.** The app listens on a local port; nginx proxies to it:

```jennifer
def srv as httpd.Server init httpd.listen("127.0.0.1:8080");
```

```nginx
server {
    listen 443 ssl;
    server_name app.example;
    location /static/ { root /srv/app; }        # nginx serves assets directly
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $remote_addr;
    }
}
```

**Unix domain socket.** No TCP port; nginx proxies over a socket file (cleaner
permissions, a touch less overhead). The `unix:` prefix selects it, and a
graceful `httpd.shutdown` unlinks the socket on the way out. If a socket file
lingers from a prior crash, `httpd.listen` clears it only after confirming it
is stale (nothing is listening) - it never deletes a socket a live server is
still using.

`httpd.listenTLS` floors the negotiated protocol at TLS 1.2 (TLS 1.0 / 1.1 are
deprecated by RFC 8996).

```jennifer
def srv as httpd.Server init httpd.listen("unix:/run/app/app.sock");
```

```nginx
upstream app { server unix:/run/app/app.sock; }

server {
    listen 443 ssl;
    server_name app.example;
    location / {
        proxy_pass http://app;
        proxy_set_header Host $host;
    }
}
```

Each process handles one request at a time (the pull loop is serial per accept
loop - see Scope and limits), so for concurrency and multi-core use run several
app processes on distinct ports or sockets behind one nginx `upstream {}` block.

## Scope and limits

- **HTTP/1.1** over plaintext; **HTTP/2** is negotiated automatically over TLS
  by `net/http`.
- The request body is buffered with a **10 MiB cap**; a body over the cap is
  rejected with **413 Request Entity Too Large** before it reaches the program
  (never silently truncated - a truncated body would defeat body-signature
  checks). A configurable limit is a planned follow-up.
- **Admission control.** At most 256 requests buffer a body / stay in flight at
  once; further connections wait for a slot, so buffered memory is bounded
  (~slots x 10 MiB) rather than growing with the connection count.
- **Must respond.** Every accepted request must be answered with
  `httpd.respond` (or `serveFile` / `serveDir`). A request left unanswered -
  e.g. the program threw between `accept` and `respond` - is answered **500**
  by the engine after a 60-second safety timeout, so the handler goroutine and
  client connection don't leak.
- **Routing, path parameters, middleware, cookies, and sessions** are not in
  the engine - they belong to the [`web`](../modules/web.md) framework module
  built on top of it, which does name-based handler dispatch itself (the engine
  never calls back into the interpreter). `web` owns the session **id cookie**;
  the session **store** stays with the app, so the engine and `web` both stay
  storage-agnostic.

## See also

- [`http`](../modules/http.md) - the HTTP/1.1 *client* module.
- [`net`](net.md) - the lower-level TCP / TLS / UDP primitives.
- [`json`](json.md) / [`toml`](toml.md) - encode / decode request and response
  bodies.
- [technical/tinygo.md](../technical/tinygo.md) - why `httpd` is
  default-binary-only.
