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
| `httpd.listenWith(addr, opts)` | `httpd.Server` | Like `listen`, with per-server limits from an `httpd.Options` (see [Tuning the body / concurrency limits](#tuning-the-body--concurrency-limits)). |
| `httpd.listenTLS(addr, cert, key)` | `httpd.Server` | HTTPS; `cert` / `key` are PEM `bytes`. HTTP/2 negotiated automatically. |
| `httpd.listenTLSWith(addr, cert, key, opts)` | `httpd.Server` | Like `listenTLS`, with an `httpd.Options` trailing the cert/key. |
| `httpd.setMaxBufferBudget(bytes)` | `null` | Move the process-wide memory-guard ceiling (default 4 GiB) that `listenWith` enforces; raise it for a big-RAM host, lower it for a small one (floored at 10 MiB). |
| `httpd.setMaxBufferBudgetFromRAM(fraction)` | `int` | Opt-in, cgroup-aware: set the budget to `fraction` (`0 < f <= 1`) of the detected machine limit (min of host RAM and any cgroup limit); returns the bytes it set. Errors on a host where the limit can't be determined. |
| `httpd.address(srv)` | `string` | The actual bound address (resolve `":0"` to the chosen port). |
| `httpd.accept(srv)` | `httpd.Request` | Block for the next request. Errors once the server is shut down. |
| `httpd.method(req)` | `string` | `"GET"`, `"POST"`, ... |
| `httpd.path(req)` | `string` | URL path, e.g. `/users/42`. |
| `httpd.query(req, name)` | `string` | Query parameter (`""` if absent). |
| `httpd.header(req, name)` | `string` | Request header (`""` if absent; case-insensitive name). |
| `httpd.body(req)` | `bytes` | The request body (buffered; a body over the 10 MiB cap is answered 413 by the engine). |
| `httpd.remoteAddr(req)` | `string` | Client `host:port`. |
| `httpd.requestValue(req, key)` | `string` | Read a request-scoped note (`""` if unset). Per-request scratch space, not a header and never sent to the client. |
| `httpd.setRequestValue(req, key, value)` | `null` | Store a request-scoped note - a place to memoize a value computed at most once per request (see below). |
| `httpd.setHeader(req, name, value)` | `null` | Set a response header (before `respond`). |
| `httpd.etag(req, tag)` | `bool` | Set the `ETag` validator and honour a conditional GET: answers `304` and returns `true` (stop) when the request's `If-None-Match` matches `tag`, else returns `false` (send the full body). |
| `httpd.respond(req, status, body)` | `null` | Send the response; `body` is a `string` or `bytes`. |
| `httpd.serveFile(req, path)` | `null` | Answer with a file (content type, range requests handled by `net/http`). |
| `httpd.serveDir(req, root)` | `null` | Answer with the file under `root` matching the request path (`..` cannot escape `root`). |
| `httpd.serveFileEtag(req, path)` | `null` | Like `serveFile`, plus a content-hash `ETag` (SHA-256, cached by mtime+size) so a revalidating client gets a `304`. |
| `httpd.serveDirEtag(req, root)` | `null` | Like `serveDir`, plus the same content-hash `ETag`. |
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

## Request-scoped notes

`httpd.setRequestValue(req, key, value)` and `httpd.requestValue(req, key)` are a
small per-request string map - scratch space that lives as long as the request
and is never sent to the client. Use it to memoize a value that must be computed
at most once per request, even when several call sites (or a helper reached from
many of them) would otherwise each recompute it. An unset key reads back `""`, so
a caller storing a non-empty value can treat `""` as "not computed yet".

The framework layer uses this to make `web.csrfToken` idempotent: the first call
in a request mints the token and stashes it here; later calls read it back rather
than minting a second token (which would reset the `csrf` cookie and invalidate
every form already rendered on the page). The store is guarded by the request's
mutex, so it is safe when the request is handed to a `spawn`.

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

`serveFile` / `serveDir` already send `Last-Modified` and honour
`If-Modified-Since` (Go's `http.ServeContent`), so a browser revalidates a static
file for free. They do **not** add an `ETag` - an mtime is a per-file-copy
validator, so two replicas serving identical bytes disagree. When you want a
validator that is stable across replicas, use **`serveFileEtag` /
`serveDirEtag`**: they behave exactly like the plain verbs but also set a
strong `ETag` that is the SHA-256 of the file content, so an unchanged file
answers `304` regardless of which replica serves it. The digest is cached per
`(path, mtime, size)`, so a hot file is hashed once, not on every hit; an edit
(new bytes, or a changed size / mtime) recomputes it. The first request for a
large cold file pays a full read to hash it - reach for the cached verbs when
cross-replica revalidation is worth that, and keep the plain verbs otherwise.

## Conditional requests (ETags)

`httpd.etag($req, tag)` sets an `ETag` validator and handles a conditional GET in
one call, so a bare-engine app does not re-parse `If-None-Match` itself (the
comma-list, `*`, and the weak-validator `W/` prefix a cache or a gzip proxy may
add are all handled). It sets the `ETag` response header to `tag` (quoted) and:

- returns `true` when the request's `If-None-Match` matches - it has already
  answered `304 Not Modified` (with the `ETag`, no body); the handler should
  **stop**;
- returns `false` otherwise - the handler sends the full response as usual.

```jennifer
use httpd;
use hash;
use convert;
use encoding;
def srv as httpd.Server init httpd.listen(":8080");
while (true) {
    def req as httpd.Request init httpd.accept($srv);
    def page as string init render();                 # your content
    def tag as string init encoding.toText(
        hash.compute(convert.bytesFromString($page, "utf-8"), "sha256"), "hex");
    if (not httpd.etag($req, $tag)) {                 # 304 already sent when true
        httpd.respond($req, 200, $page);
    }
}
```

`tag` is your choice of validator - a content hash (as above), a row version, a
build id - so the engine hashes nothing on your behalf. A content hash is stable
across replicas (unlike an mtime); a strong hash of the exact bytes is the safe
default. `web.etag($ctx, tag)` is the framework wrapper over this same engine
call. For **static files**, `serveFileEtag` / `serveDirEtag` (above) do this
for you - a cached content-hash `ETag` without your handler computing anything.

## Response headers and the `Server` line

The engine adds **no `Server:` header**. Go's `net/http` sends `Date` and the
framing headers (`Content-Length` / `Transfer-Encoding`) but never advertises the
server software, and `httpd` adds nothing on top - so a response carries no
`Server: ...` fingerprint (no stack name, no version a scanner can match a CVE
against). This is a deliberate default; you opt *in* to advertising a server, you
do not opt out.

To send a `Server` header (or any custom header), set it **before** `respond`, on
that one request:

```jennifer
func handle(req as httpd.Request) {
    httpd.setHeader($req, "Server", "jennifer");
    httpd.respond($req, 200, "hi\n");
}
```

The pull loop has no middleware of its own, so to stamp a header onto **every**
response use the [`web`](../modules/web.md) framework's `web.before` middleware,
which runs before each handler:

```jennifer
func stampServer(ctx as web.Context) {
    web.setHeader($ctx, "Server", "jennifer");
    return true;
}
$app = web.before($app, stampServer);
```

(`web.setHeader` is the framework wrapper over `httpd.setHeader`; a bare `httpd`
program repeats the per-request call, or factors it into a small helper the
handlers call.)

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

## Tuning the body / concurrency limits

`httpd.listen` / `listenTLS` open a server with two safety limits: a **10 MiB**
request-body cap and a **256** in-flight-request ceiling. They exist to bound
worst-case memory - the engine buffers each body fully into RAM before your
program sees it, so the worst case is `maxInFlight x maxBodyBytes` (`256 x 10
MiB = 2.56 GiB` at the defaults). Raising the body cap for everyone therefore
also raises that ceiling; the two knobs let you spend a fixed memory budget the
way your traffic needs it.

`httpd.listenWith(addr, opts)` (and `httpd.listenTLSWith(addr, cert, key, opts)`)
open a server with an explicit `httpd.Options`:

| Field | Meaning |
| ----- | ------- |
| `maxBodyBytes as int` | Per-request body cap in bytes. `0` selects the 10 MiB default. |
| `maxInFlight as int` | Concurrent in-flight requests. `0` selects the default 256. |

A `0` field takes that limit's default, so you can set just one:

```jennifer
use httpd;

# A dedicated upload endpoint on a low-concurrency server: 100 MiB bodies, but
# only 40 at once (100 MiB x 40 = 4 GiB worst case).
def srv as httpd.Server init httpd.listenWith(":8080",
    httpd.Options{ maxBodyBytes: 100 * 1024 * 1024, maxInFlight: 40 });
```

**The memory guard.** `listenWith` validates the pair at listen time and refuses
a combination whose worst-case buffered memory (`maxInFlight x maxBodyBytes`)
exceeds the process **memory budget**, or a `maxInFlight` over 65536, or a
negative value. The budget starts at **4 GiB** - a typo-catcher sized for a
general host (it stops a slip like `maxBodyBytes: 1 GiB` at the default 256
concurrency from silently arming a ~256 GiB worst case), **not** a hardware
limit. The rejection names both values, and what to do:

```
httpd.listenWith: maxInFlight (256) x maxBodyBytes (104857600) of worst-case
buffered memory exceeds the 4096 MiB budget; lower one of them, or raise the
budget with httpd.setMaxBufferBudget if the host has the RAM
```

**`httpd.setMaxBufferBudget(bytes)`** moves that ceiling to match the actual box,
since only you know its RAM. On an 8 GiB VPS, raise it once at startup and the
larger configuration is allowed; on a small container, lower it to a tighter
policy (floored at one default body, 10 MiB, so it can never reject everything):

```jennifer
use httpd;
httpd.setMaxBufferBudget(6 * 1024 * 1024 * 1024); # this 8 GiB VPS can spend 6 GiB
def srv as httpd.Server init httpd.listenWith(":8080",
    httpd.Options{ maxBodyBytes: 500 * 1024 * 1024, maxInFlight: 12 });
```

The budget is **process-wide**, not per-server: several servers in one process
share the machine's RAM, so size it for their combined worst case (the guard
checks each server's own product against it, it does not sum them for you).

**Sizing to the machine automatically.** `httpd.setMaxBufferBudgetFromRAM(fraction)`
sets the budget to a fraction of the **detected** machine limit and returns the
bytes it chose (for logging):

```jennifer
use io;
def budget as int init httpd.setMaxBufferBudgetFromRAM(0.5); # half the box
io.printf("upload budget: {$budget} bytes\n");
```

It is **opt-in** and **cgroup-aware** by design. The detected limit is the
minimum of the host's `MemTotal` and any cgroup memory limit found by walking
this process's cgroup to the root - so inside a container it uses the
**container's** limit, not the host's RAM (auto-sizing off host RAM in a
container is the classic over-commit that gets a process OOM-killed, which is an
uncatchable `SIGKILL`, not a 413). Because it only acts when you call it, a plain
`httpd.listen` is never affected; and on a host where no limit can be determined
(a non-Linux box) it **errors** rather than guessing, pointing you at the
explicit `setMaxBufferBudget`. The result is floored at one default body (10 MiB)
and, since `fraction <= 1`, never exceeds the detected limit. You still choose the
fraction - leave headroom for the interpreter heap, response buffers, the page
cache, and anything else on the box.

Because the body is fully buffered, none of this is a streaming-upload path:
genuinely large uploads (video, backups, multi-GB files) want a reverse proxy or
object storage in front, not a bigger buffer - see below.

For the [`web`](../modules/web.md) framework, open the tuned server yourself and
hand it to `web.serveOn(app, srv)` instead of `web.run(app, addr)` (which uses
the defaults):

```jennifer
def srv as httpd.Server init httpd.listenWith(":8080",
    httpd.Options{ maxBodyBytes: 50 * 1024 * 1024, maxInFlight: 64 });
web.serveOn(app, srv);
```

## Scope and limits

- **HTTP/1.1** over plaintext; **HTTP/2** is negotiated automatically over TLS
  by `net/http`.
- The request body is buffered with a **10 MiB cap** by default; a body over the
  cap is rejected with **413 Request Entity Too Large** before it reaches the
  program (never silently truncated - a truncated body would defeat body-signature
  checks). The cap is per-server settable - see
  [Tuning the body / concurrency limits](#tuning-the-body--concurrency-limits).
- **Admission control.** At most 256 requests (by default) buffer a body / stay
  in flight at once; further connections wait for a slot, so buffered memory is
  bounded (`slots x maxBody`) rather than growing with the connection count. The
  concurrency slot count is per-server settable too.
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
