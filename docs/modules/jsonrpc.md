# `jsonrpc` - JSON-RPC 2.0 client and server

Import with `import "jsonrpc.j" as jsonrpc;`. A [JSON-RPC
2.0](https://www.jsonrpc.org/specification) **client** that calls remote methods
over HTTP, and a transport-agnostic **server** `handle` that dispatches an
incoming request to your program's methods by name. Built on
[`json`](../libraries/json.md) for the wire format and
[`http`](http.md) for the client transport, so it needs the default `jennifer`
binary.

`params` and a call's result are [`json.Value`](../libraries/json.md)s: you build
params with the `json` write API and read the result with the `json` accessors.
Every client-side failure - a JSON-RPC error reply, a transport error (connection
refused / timeout / malformed HTTP), or a malformed reply - surfaces as a single
catchable `Error{kind: "jsonrpc"}`, so one `catch` covers them all.

```jennifer
import "jsonrpc.j" as jsonrpc;
use json;

def c as jsonrpc.Client init jsonrpc.client("https://api.example.com/rpc");

# positional params: [2, 3]
def args as json.Value init json.list();
$args = json.append($args, "", 2);
$args = json.append($args, "", 3);

def sum as json.Value init jsonrpc.call($c, "add", $args);   # -> 5
```

Runnable: [`examples/modules/jsonrpc_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/jsonrpc_demo.j).

## Client

A `Client` is an endpoint plus optional headers; every call POSTs a request and
reads the reply.

```jennifer
def struct jsonrpc.Client { endpoint as string, headers as map of string to string };
```

| Call | Returns | |
| ---- | ------- | - |
| `jsonrpc.client(endpoint)` | `Client` | bind to a JSON-RPC endpoint URL |
| `jsonrpc.clientWith(endpoint, headers)` | `Client` | with extra request headers (e.g. `Authorization`) |
| `jsonrpc.call(client, method, params)` | `json.Value` | call `method` and return its `result`; throws `Error{kind: "jsonrpc"}` on an error reply, a transport error, or a malformed reply (no `result`/`error`, or a mismatched `id`) |
| `jsonrpc.notify(client, method, params)` | | send a notification (no `id`, no reply); returns nothing, but a transport error still throws `Error{kind: "jsonrpc"}` (the request never reached the server) |

`params` is a `json.Value`: a `json.list` for positional arguments or a
`json.map` for named ones (pass `json.list()` for a method that takes none). An
`https://` endpoint uses TLS automatically.

```jennifer
# named params, and reading a structured result
def p as json.Value init json.set(json.map(), "/city", "berlin");
def w as json.Value init jsonrpc.call($c, "weather", $p);
io.printf("%d C\n", json.asInt($w, "/tempC"));
```

## Server

`jsonrpc.handle(requestBody)` turns a raw request into a reply body - it does the
whole JSON-RPC protocol (single request, notification, batch, and every reserved
error code) and leaves the transport to you: read the body off
[`httpd`](../libraries/httpd.md) or [`net`](../libraries/net.md), pass it to
`handle`, write the returned string back.

Each request's `method` names a **top-level method** `func NAME(params as
json.Value)` in the program that imported the module (dispatched by name via
`meta.callMain`, the same mechanism the [`web`](web.md) module uses). The handler
is called with the request's params and returns a `json.Value` **or a scalar**
(int / float / string / bool / null) as its result.

| Call | Returns | |
| ---- | ------- | - |
| `jsonrpc.handle(requestBody)` | `string` | the reply JSON, or `""` when none is owed (a notification, or an all-notification batch) |

```jennifer
# in the program that serves RPC:
func add(params as json.Value) {
    return json.asInt($params, "/0") + json.asInt($params, "/1");
}

# ... in your accept loop, given the request body:
def reply as string init jsonrpc.handle($body);
if (len($reply) > 0) {
    httpd.respond($req, 200, $reply);   # a notification returns "" - send 204
}
```

A missing method is a `-32601` (method not found) reply; a handler that **throws**
becomes a generic `-32603` (internal error) reply - the thrown message stays
server-side and is **not** put on the wire, so you can raise detailed errors
without leaking internals. An unparseable body is `-32700` (parse error). A
handler must take exactly one `json.Value` parameter.

> **Security - the whole top-level namespace is exposed.** `handle` resolves a
> request's `method` against *every* top-level `func` in the program (via
> `meta.callMain`); any one that takes a single `json.Value` argument is remotely
> callable. There is no route allow-list like [`web`](web.md)'s. So:
>
> - Name RPC handlers deliberately - a shared prefix, or a dedicated dispatch
>   file - and do not co-locate a `handle`-served program with privileged
>   one-argument helpers.
> - Authenticate at the transport: check a header / token **before** calling
>   `handle`; the module does no authentication of its own.
> - A batch is processed entry by entry with no size limit of its own (the
>   transport's body cap bounds it); cap the request size upstream if untrusted.

## Error codes

Constants for the reserved codes (JSON-RPC 2.0 section 5.1); application errors
use the `-32000..-32099` server-error range or any code outside the reserved
block.

| Constant | Code | |
| -------- | ---- | - |
| `jsonrpc.PARSE_ERROR` | `-32700` | invalid JSON received |
| `jsonrpc.INVALID_REQUEST` | `-32600` | not a valid request object |
| `jsonrpc.METHOD_NOT_FOUND` | `-32601` | no such method |
| `jsonrpc.INVALID_PARAMS` | `-32602` | invalid method parameters |
| `jsonrpc.INTERNAL_ERROR` | `-32603` | internal JSON-RPC error |

## Scope

- **HTTP transport.** The client speaks JSON-RPC over HTTP POST (the common
  case); `handle` is transport-agnostic, so a raw-TCP / newline-framed server is
  a matter of wiring it to `net`.
- **Client is single-call.** `call` / `notify` send one request; the server
  `handle` accepts a batch (a JSON array) on the receiving side. A client-side
  batch builder is a possible later add.
- **Handler result shape.** A handler returns a `json.Value` or a scalar; a raw
  Jennifer `list` / `map` / struct is not coerced - build a `json.Value` result
  explicitly. A custom per-handler error code (beyond the generic `-32603`) is a
  later add; today a thrown error maps to internal-error with a fixed message.
- **Client correlation.** `call` sends a fixed request `id` and rejects a reply
  whose `id` does not echo it. It is a synchronous single-call client, so there
  is no in-flight pipelining to correlate across.

## See also

- [json.md](../libraries/json.md) - the value type params and results are built
  from and read with.
- [http.md](http.md) - the client transport (`https://` via TLS).
- [rest.md](rest.md) / [web.md](web.md) - the REST client and web framework, the
  other `http`-backed request/response modules.
- [modules/index.md](index.md) - the module catalog and import rules.
