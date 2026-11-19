# `mcp` - Model Context Protocol server and client

Import with `import "mcp.j" as mcp;`. A [Model Context
Protocol](https://modelcontextprotocol.io) implementation: a **server** that
exposes tools / resources / prompts to an LLM host, and a **client** that
calls a remote MCP server over HTTP or a launched stdio subprocess. MCP is
[JSON-RPC 2.0](https://www.jsonrpc.org/specification) on the wire, so the HTTP
client reads its replies with [`jsonrpc`](jsonrpc.md) (over [`http`](http.md) +
[`json`](../libraries/json.md)) and the server is a purpose-built router.
Because it uses `http` and `os.spawn`, this module needs the default
`jennifer` binary.

This module targets the stateless protocol only (no sessions). The stdio
server, the HTTP client, and the stdio subprocess client are implemented (see
[Scope](#scope)).

```jennifer
import "mcp.j" as mcp;
use json;

func echo(args as json.Value) {
    return json.asString($args, "/text");
}

def sch as json.Value init mcp.property(mcp.schema(), "text", "string", "text to echo", true);
def srv as mcp.Server init mcp.addTool(mcp.server("demo", "1.0.0"), "echo", "echo text", $sch, echo);

# answer one request (transport-agnostic):
def reply as string init mcp.handle($srv, '{"jsonrpc":"2.0","method":"ping","id":1}');
```

Runnable: [`examples/modules/mcp_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/mcp_demo.j).

## Building a server

A `Server` is an identity plus registries of tools, resources, and prompts.
Build it value-semantically: each builder returns a fresh `Server`.

```text
struct mcp.Server {
    name as string, version as string,
    tools as list of mcp.Tool,
    resources as list of mcp.Resource,
    prompts as list of mcp.Prompt
}
```

| Call | Returns | |
| ---- | ------- | - |
| `mcp.server(name, version)` | `Server` | an empty server with the given identity |
| `mcp.addTool(server, name, description, inputSchema, handler)` | `Server` | register a tool; `inputSchema` is a `json.Value` JSON Schema, `handler` a `func` value |
| `mcp.addResource(server, uri, name, description, mimeType, handler)` | `Server` | register a resource keyed by `uri` |
| `mcp.addPrompt(server, name, description, arguments, handler)` | `Server` | register a prompt template; `arguments` is a `list of PromptArg` (declared to the host, may be `[]`) |
| `mcp.promptArg(name, description, required)` | `PromptArg` | one prompt-argument declaration for `addPrompt` |

Each item's `handler` is a `func` value: a top-level function in the program
that imported the module, passed by its bare name and called through its home
interpreter, so it resolves its own imports and builds its `json.Value` result in
the entry program's context (the same mechanism the [`web`](web.md) module uses):

- a **tool** handler is `func NAME(arguments as json.Value)` and returns a
  `json.Value` **or a scalar** (a string is used as the text content verbatim;
  anything else is JSON-encoded into the text content);
- a **resource** handler is `func NAME(uri as json.Value)` and returns the
  resource's text;
- a **prompt** handler is `func NAME(arguments as json.Value)` and returns the
  prompt messages as a `json.Value` array. Each message is
  `{role, content}` where `content` is a content block
  `{"type": "text", "text": ...}`; MCP requires the block form, and a host
  rejects a plain string.

A prompt's `arguments` (built with `promptArg`) are surfaced by `prompts/list`,
so a host can collect them before calling `prompts/get`. Pass `[]` for a prompt
that takes none.

### Declaring a tool's input schema

A tool declares a JSON-Schema object describing its arguments. Build it with the
two schema helpers (a small convenience over the `json` write API):

| Call | Returns | |
| ---- | ------- | - |
| `mcp.schema()` | `json.Value` | a `{"type":"object","properties":{},"required":[]}` skeleton |
| `mcp.property(schema, name, jsonType, description, required)` | `json.Value` | add `properties/<name> = {type, description}`; append `name` to `required` when `required` is true |

```jennifer
def sch as json.Value init mcp.property(
    mcp.property(mcp.schema(), "a", "integer", "first addend", true),
    "b", "integer", "second addend", true);
```

## Serving

`mcp.handle(server, requestBody)` turns one raw JSON-RPC request into a reply
body: it routes the whole stateless MCP surface and leaves the transport to
you. A notification (a request with no `id`, or any `notifications/` method)
owes no reply and returns `""`.

| Call | Returns | |
| ---- | ------- | - |
| `mcp.handle(server, requestBody)` | `string` | the reply JSON, or `""` when none is owed |
| `mcp.serveStdio(server)` | | run the stdio transport: read newline-delimited JSON-RPC from the program's own stdin, dispatch each through `handle`, write each non-empty reply to stdout, until EOF |

`serveStdio` is the primary MCP transport: an LLM host launches the program and
speaks JSON-RPC over its stdin/stdout:

```jennifer
import "mcp.j" as mcp;
# ... build $srv ...
mcp.serveStdio($srv);   # blocks, reading stdin until EOF
```

A line that is not JSON, not UTF-8, or over `io.readLine`'s cap (256 MiB) is
answered with one `-32700` parse error and the loop goes on. The rest of an
over-cap line is skipped up to its newline, never read as the next request.

For HTTP, wire `handle` to [`httpd`](../libraries/httpd.md) yourself:

```jennifer
def req as httpd.Request init httpd.accept($hsrv);
def body as string init convert.stringFromBytes("utf-8", httpd.body($req));
def reply as string init mcp.handle($srv, $body);
if ($reply == "") {
    httpd.respond($req, 202, "");   # a notification: accepted, no body
} else {
    httpd.setHeader($req, "Content-Type", "application/json");
    httpd.respond($req, 200, $reply);
}
```

### Routed methods

| Method | Result |
| ------ | ------ |
| `initialize` | `{protocolVersion, capabilities: {tools, resources, prompts}, serverInfo: {name, version}}` |
| `ping` | `{}` |
| `notifications/*` | no reply (`""`) |
| `tools/list` | `{tools: [{name, description, inputSchema}]}` |
| `tools/call` | `{content: [{type: "text", text}], isError}` (params `{name, arguments}`) |
| `resources/list` | `{resources: [{uri, name, description, mimeType}]}` |
| `resources/read` | `{contents: [{uri, mimeType, text}]}` (params `{uri}`) |
| `prompts/list` | `{prompts: [{name, description, arguments?}]}` (`arguments` present when the prompt declares any) |
| `prompts/get` | `{description, messages}` (params `{name, arguments}`) |

The protocol version reported is `2025-06-18`. An unknown protocol method is a
`-32601` (method not found) reply; an unparseable body is `-32700` (parse
error); an unknown resource / prompt is `-32602` (invalid params). A request
that is not a JSON-RPC 2.0 request - not an object, a `jsonrpc` member other
than `"2.0"` (or none), a `method` that is not a string, or an `id` that is
not a string or an integer (MCP ids are never `null`) - is `-32600` (invalid
request), with a `null` id when the id is the problem. `params` that are not an
object are `-32602`.

> **Security - dispatch is an allow-list.** Unlike [`jsonrpc`](jsonrpc.md)'s
> `handle`, which dispatches a request's `method` to any top-level `func` of that
> name, `mcp.handle` only invokes a handler registered in the server:
>
> - A `tools/call` looks the tool up by name in the registry; an unregistered
>   name is a tool-result error (`isError: true`), never a dispatch to an
>   arbitrary top-level function. The same holds for resources (by `uri`) and
>   prompts (by `name`).
> - A tool handler that throws yields a tool-result error with a generic
>   message; the thrown message stays server-side and is not put on the wire,
>   so detailed errors do not leak internals.
> - Authenticate at the transport (check a header / token before calling
>   `handle`); the module does no authentication of its own.

## Client (HTTP)

`mcp.connect` binds a client to a remote MCP endpoint over HTTP (it keeps the
endpoint and headers in a [`jsonrpc.Client`](jsonrpc.md)). Every call POSTs a
JSON-RPC request, as the Streamable HTTP transport of MCP `2025-06-18` asks:
with `Accept: application/json, text/event-stream` (unless you set your own
`Accept`), and on every request after `initialize` with `MCP-Protocol-Version:
2025-06-18`. The server may answer with one JSON object or with an event
stream carrying the response (requests and notifications it sends first are
passed over). Any failure - a JSON-RPC error reply, a transport error, or a
reply that is not a JSON-RPC 2.0 response - throws a catchable
[`jsonrpc.RpcError`](jsonrpc.md) (kind `"jsonrpc"`) with the reply's `code`
and `data`; a socket failure is a `net.NetError` (kind `"net"`).

A client whose headers carry `Authorization` or `Proxy-Authorization` refuses
an `http://` endpoint on a host other than loopback (`127.0.0.0/8`, `[::1]`,
`localhost`): the call throws a `jsonrpc.RpcError` with reason `"insecure"`
before connecting. For plain HTTP on a trusted LAN, set `allowInsecureHttp`:

```jennifer
def lan as mcp.Client init mcp.connectWith(
    "http://mcp.home.lan/mcp", {"Authorization": "Bearer " + $token});
$lan.allowInsecureHttp = true;
```

Over stdio a failure is an `mcp.McpError` (kind `"mcp"`): `reason` is `"rpc"`
(an error reply, with its `code` and `data`), `"launch"` (the server could not
be run), `"exit"` (it exited non-zero; `exitCode` holds the code),
`"protocol"` (its stdout carried a line that is not a JSON-RPC 2.0 message -
the stdio transport allows nothing else there, so log to stderr - or a
malformed reply) or `"noreply"` (no reply for the request). Every error of
kind `"mcp"` is one. A server that has not replied within the client's
`stdioTimeoutMs` (default 60 s) is stopped and the call throws kind
`"timeout"`; stdout past 16 MiB, or a reply line past `json`'s decoding caps,
is kind `"limit"`.

```text
struct mcp.Client {
    rpc as jsonrpc.Client,       # the HTTP endpoint and headers
    argv as list of string,      # the stdio server command line
    isStdio as bool,
    allowInsecureHttp as bool,   # credentials over plain http:// to another host
    stdioTimeoutMs as int        # one stdio exchange, launch to reply; 0 = 60 s
}
```

| Call | Returns | |
| ---- | ------- | - |
| `mcp.connect(endpoint)` | `Client` | bind to an MCP endpoint URL (HTTP) |
| `mcp.connectWith(endpoint, headers)` | `Client` | HTTP, with extra request headers (auth, ...) |
| `mcp.connectStdio(argv)` | `Client` | launch an MCP server subprocess and talk to it over stdio (needs a POSIX `/bin/sh`) |
| `mcp.initialize(client)` | `json.Value` | run the `initialize` handshake (and send the required `notifications/initialized` on success); returns the server's result |
| `mcp.listTools(client)` | `json.Value` | the `tools` array |
| `mcp.callTool(client, name, arguments)` | `json.Value` | call a tool; returns the whole result (check `isError`) |
| `mcp.listResources(client)` | `json.Value` | the `resources` array |
| `mcp.readResource(client, uri)` | `json.Value` | read a resource; returns the whole `resources/read` result |
| `mcp.listPrompts(client)` | `json.Value` | the `prompts` array |
| `mcp.getPrompt(client, name, arguments)` | `json.Value` | get a prompt; returns the whole `prompts/get` result |

```jennifer
def c as mcp.Client init mcp.connect("http://127.0.0.1:8080/");
def info as json.Value init mcp.initialize($c);

def args as json.Value init json.set(json.map(), "/text", "hello");
def out as json.Value init mcp.callTool($c, "echo", $args);
io.printf("isError=%t text=%s\n",
    json.asBool($out, "/isError"), json.asString($out, "/content/0/text"));
```

## Scope

- **Stateless protocol only.** No session lifecycle: the client sends no
  `Mcp-Session-Id`, so a server that requires a session refuses its calls after
  `initialize`. `handle` treats each request independently;
  `notifications/initialized` (and any other `notifications/` message) is
  accepted and answered with no reply. The HTTP client reads a POST's event
  stream whole, once the server has closed it after the response; the
  standalone GET stream for server-initiated messages needs an HTTP response
  read as it arrives, which `http` does not do today. Streaming `http`
  responses and an `sse` module are planned (see the
  [milestones](../milestones.md)), and that transport builds on them.
- **Transports.** Server side: the stdio server (`serveStdio`) and the
  transport-agnostic `handle` (wire to `httpd` / `net`). Client side: the HTTP
  client (`connect` + the call verbs) and the stdio subprocess client
  (`connectStdio`, over `os.spawn`).
- **The stdio client is one-shot.** Because `os` has no persistent child pipe,
  `connectStdio` relaunches the server per call: it writes the `initialize`
  handshake plus the one operation as newline-delimited JSON for the child's
  stdin, which `/bin/sh` feeds it from a temporary file (`os.spawn` takes no
  stdin of its own), and reads stdout as it arrives (correlating by `id`). Once
  the reply is in, a server that does not exit on stdin's end gets 2 s, then
  SIGTERM and 2 s more, then SIGKILL - the shutdown the MCP stdio transport
  describes; only the server's own process is signalled, not children it
  started. A stateless server sees a complete short session each call; a
  persistent, interleaved session (many calls over one launched process) would
  need the streaming-pipe primitive noted in `os` as a follow-on.
  `connectStdio` needs the default binary (`os.spawn`).
- **Handler result shape.** A tool handler returns a `json.Value` or a scalar; a
  raw Jennifer `list` / `map` / struct is not coerced - build a `json.Value`
  result explicitly. Structured (non-text) tool content and resource
  subscriptions are later adds.

## See also

- [jsonrpc.md](jsonrpc.md) - the JSON-RPC 2.0 client / server MCP is built on.
- [json.md](../libraries/json.md) - the value type params and results are built
  from and read with.
- [httpd.md](../libraries/httpd.md) / [http.md](http.md) - the server engine and
  client transport.
- [web.md](web.md) - the web framework, the other `func`-value-dispatched
  request/response module.
- [modules/index.md](index.md) - the module catalog and import rules.
