# `jsonrpc` - JSON-RPC 2.0 client and server

Import with `import "jsonrpc.j" as jsonrpc;`. A [JSON-RPC
2.0](https://www.jsonrpc.org/specification) client that calls remote methods
over HTTP, and a transport-agnostic server `handle` that dispatches an
incoming request to your program's functions by name. Built on
[`json`](../libraries/json.md) for the wire format and
[`http`](http.md) for the client transport, so it needs the default `jennifer`
binary.

`params` and a call's result are [`json.Value`](../libraries/json.md)s: you build
params with the `json` write API and read the result with the `json` accessors.
Every client-side failure - a JSON-RPC error reply, a transport error
(malformed HTTP), or a malformed reply - surfaces as one catchable
`jsonrpc.RpcError` (kind `"jsonrpc"`). The generic kinds are passed on as `http`
or `json` threw them: a reply over `http`'s body cap or past `json`'s decoding
caps is `"limit"`, a deadline `"timeout"`, a reply cut short `"closed"`, a
cancelled task `"cancelled"`, and a socket failure (refused, reset, DNS, TLS)
`http`'s `net.NetError` (kind `"net"`):

| Field | Meaning |
|---|---|
| `kind` | always `"jsonrpc"` |
| `message` | `jsonrpc error <code>: <message>` for an error reply, else what went wrong |
| `reason` | `"rpc"` (an error object), `"transport"` (a malformed HTTP answer), `"protocol"` (not a JSON-RPC 2.0 response: not JSON, `jsonrpc` not `"2.0"`, neither or both of result and error, a malformed error object, or another id), `"http"` (a notification answered with a non-2xx status) or `"insecure"` (credentials refused over plain `http://`; nothing was sent) |
| `status` | the HTTP status of the answer; 0 for `"transport"` and `"insecure"` |
| `code` | the error object's `code` (`-32601` method not found, ...); 0 otherwise |
| `data` | the error object's `data` member; JSON `null` when there is none |

```jennifer
try {
    jsonrpc.call($c, "add", $args);
} catch (e) {
    if ($e.kind == "jsonrpc" and $e.reason == "rpc" and $e.code == -32601) {
        io.printf("the server has no add\n");
    } else {
        throw $e;
    }
}
```

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

```text
struct jsonrpc.Client {
    endpoint as string,
    headers as map of string to string,
    allowInsecureHttp as bool
}
```

| Call | Returns | |
| ---- | ------- | - |
| `jsonrpc.client(endpoint)` | `Client` | bind to a JSON-RPC endpoint URL |
| `jsonrpc.clientWith(endpoint, headers)` | `Client` | with extra request headers (e.g. `Authorization`) |
| `jsonrpc.call(client, method, params)` | `json.Value` | call `method` and return its `result`; throws `RpcError` (kind `"jsonrpc"`) on an error reply, a transport error, or a reply that is not a JSON-RPC 2.0 response (below) |
| `jsonrpc.notify(client, method, params)` | | send a notification (no `id`, no reply); returns nothing on a 2xx, but a failed request still throws as `call` does (a `net.NetError` means the request never reached the server), and a non-2xx answer is an `RpcError` with reason `"rpc"` when it carries a JSON-RPC error object, else `"http"` |
| `jsonrpc.parseReply(body, status, id)` | `json.Value` | read a response body the way `call` does, for another transport (an event stream, a socket); `status` is carried into the error (0 for none) |

`call` is strict about the reply: it must be a JSON object with `"jsonrpc":
"2.0"` and exactly one of `result` and `error`; an `error` must be an object
with an integer `code` and a string `message`; and the `id` must echo the
request's (an error reply may carry `null`, which a server sends when it could
not read the request's id). Anything else is reason `"protocol"`.

A client whose headers carry `Authorization` or `Proxy-Authorization` refuses
an `http://` endpoint on a host other than loopback (`127.0.0.0/8`, `[::1]`,
`localhost`) with reason `"insecure"`, before connecting. For plain HTTP on a
trusted LAN, set `allowInsecureHttp`:

```jennifer
def lan as jsonrpc.Client init jsonrpc.clientWith(
    "http://rpc.home.lan/rpc", {"Authorization": "Bearer " + $token});
$lan.allowInsecureHttp = true;
```

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

`jsonrpc.handle(requestBody)` turns a raw request into a reply body. It handles
the whole JSON-RPC protocol (single request, notification, batch, and every reserved
error code) and leaves the transport to you: read the body off
[`httpd`](../libraries/httpd.md) or [`net`](../libraries/net.md), pass it to
`handle`, write the returned string back.

Each request's `method` names a top-level function `func NAME(params as
json.Value)` in the program that imported the module, dispatched by name via
`meta.callMain` (the name arrives on the wire as a runtime string, not a `func`
value). The handler is called with the request's params and returns a
`json.Value` or a scalar (int / float / string / bool / null) as its result.

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

A missing method is a `-32601` (method not found) reply; a handler that throws
becomes a generic `-32603` (internal error) reply. The thrown message stays
server-side and is not put on the wire, so detailed errors do not leak
internals. An unparseable body is `-32700` (parse error). A request that is
not a JSON-RPC 2.0 request object is `-32600` (invalid request): not an
object, a `jsonrpc` member other than `"2.0"` (or none), a `method` that is not
a string, `params` that are neither an array nor an object, or an `id` that is
not a string, a number or `null` (that reply's `id` is `null`). Such a request
is answered even without an `id`. A handler must take exactly one `json.Value`
parameter.

> **Security - the whole top-level namespace is exposed.** `handle` resolves a
> request's `method` against *every* top-level `func` in the program (via
> `meta.callMain`); any one that takes a single `json.Value` argument is remotely
> callable. There is no route allow-list like [`web`](web.md)'s. So:
>
> - Name RPC handlers distinctly (a shared prefix, or a dedicated dispatch
>   file) and do not co-locate a `handle`-served program with privileged
>   one-argument helpers.
> - Authenticate at the transport: check a header / token **before** calling
>   `handle`; the module does no authentication of its own.
> - A batch is processed entry by entry and holds at most 1000 requests; a
>   larger one is refused whole with a single `-32600` reply, before any handler
>   runs. The transport's body cap still bounds the request size.

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
  case); `handle` is transport-agnostic, so a raw-TCP / newline-framed server
  only needs wiring to `net`.
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
