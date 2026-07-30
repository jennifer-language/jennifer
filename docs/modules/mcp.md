# `mcp` - Model Context Protocol server and client

Import with `import "mcp.j" as mcp;`. A [Model Context
Protocol](https://modelcontextprotocol.io) implementation: a **server** that
exposes tools / resources / prompts to an LLM host, and an HTTP **client** that
calls a remote MCP server. MCP is [JSON-RPC
2.0](https://www.jsonrpc.org/specification) on the wire, so the client is a thin
layer over [`jsonrpc`](jsonrpc.md) (which is over [`http`](http.md) +
[`json`](../libraries/json.md)) and the server is a purpose-built router. Because
it uses `jsonrpc` / `http`, this module needs the default `jennifer` binary.

This module targets the **stateless** protocol only - no SSE, no sessions. The
stdio server (the primary transport) and the HTTP client are implemented; the
stdio *subprocess* client is deferred (see [Scope](#scope)).

```jennifer
import "mcp.j" as mcp;
use json;

func echo(args as json.Value) {
    return json.asString($args, "/text");
}

def sch as json.Value init mcp.property(mcp.schema(), "text", "string", "text to echo", true);
def srv as mcp.Server init mcp.addTool(mcp.server("demo", "1.0.0"), "echo", "echo text", $sch, "echo");

# answer one request (transport-agnostic):
def reply as string init mcp.handle($srv, "{\"jsonrpc\":\"2.0\",\"method\":\"ping\",\"id\":1}");
```

Runnable: [`examples/modules/mcp_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/mcp_demo.j).

## Building a server

A `Server` is an identity plus registries of tools, resources, and prompts.
Build it value-semantically: each builder returns a fresh `Server`.

```jennifer
def struct mcp.Server {
    name as string, version as string,
    tools as list of mcp.Tool,
    resources as list of mcp.Resource,
    prompts as list of mcp.Prompt
};
```

| Call | Returns | |
| ---- | ------- | - |
| `mcp.server(name, version)` | `Server` | an empty server with the given identity |
| `mcp.addTool(server, name, description, inputSchema, handler)` | `Server` | register a tool; `inputSchema` is a `json.Value` JSON Schema, `handler` a top-level method name |
| `mcp.addResource(server, uri, name, description, mimeType, handler)` | `Server` | register a resource keyed by `uri` |
| `mcp.addPrompt(server, name, description, arguments, handler)` | `Server` | register a prompt template; `arguments` is a `list of PromptArg` (declared to the host, may be `[]`) |
| `mcp.promptArg(name, description, required)` | `PromptArg` | one prompt-argument declaration for `addPrompt` |

Each item's `handler` names a **top-level method** in the program that imported
the module, dispatched by name via `meta.callMain` (the same mechanism the
[`web`](web.md) module uses):

- a **tool** handler is `func NAME(arguments as json.Value)` and returns a
  `json.Value` **or a scalar** (a string is used as the text content verbatim;
  anything else is JSON-encoded into the text content);
- a **resource** handler is `func NAME(uri as json.Value)` and returns the
  resource's text;
- a **prompt** handler is `func NAME(arguments as json.Value)` and returns the
  prompt messages as a `json.Value` array. Each message is
  `{role, content}` where **`content` is a content block**
  `{"type": "text", "text": ...}` - not a bare string (MCP requires the block
  form, and a host rejects a plain string).

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
body - it routes the whole stateless MCP surface and leaves the transport to
you. A notification (a request with no `id`, or any `notifications/` method)
owes no reply and returns `""`.

| Call | Returns | |
| ---- | ------- | - |
| `mcp.handle(server, requestBody)` | `string` | the reply JSON, or `""` when none is owed |
| `mcp.serveStdio(server)` | | run the stdio transport: read newline-delimited JSON-RPC from the program's own stdin, dispatch each through `handle`, write each non-empty reply to stdout, until EOF |

`serveStdio` is the primary MCP transport - an LLM host launches the program and
speaks JSON-RPC over its stdin/stdout:

```jennifer
import "mcp.j" as mcp;
# ... build $srv ...
mcp.serveStdio($srv);   # blocks, reading stdin until EOF
```

For HTTP, wire `handle` to [`httpd`](../libraries/httpd.md) yourself:

```jennifer
def req as httpd.Request init httpd.accept($hsrv);
def body as string init convert.stringFromBytes(httpd.body($req), "utf-8");
httpd.respond($req, 200, mcp.handle($srv, $body));
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
error); an unknown resource / prompt is `-32602` (invalid params).

> **Security - dispatch is an allow-list.** Unlike [`jsonrpc`](jsonrpc.md)'s
> `handle`, which dispatches a request's `method` to any top-level `func` of that
> name, `mcp.handle` only ever invokes a handler **registered** in the server:
>
> - A `tools/call` looks the tool up by name in the registry; an unregistered
>   name is a tool-result error (`isError: true`), never a dispatch to an
>   arbitrary top-level method. The same holds for resources (by `uri`) and
>   prompts (by `name`).
> - A tool handler that **throws** yields a tool-result error with a generic
>   message; the thrown message stays server-side and is **not** put on the wire,
>   so you can raise detailed errors without leaking internals.
> - Authenticate at the transport (check a header / token before calling
>   `handle`); the module does no authentication of its own.

## Client (HTTP)

`mcp.connect` binds a client to a remote MCP endpoint over HTTP (it wraps a
[`jsonrpc.Client`](jsonrpc.md)). Every call POSTs a JSON-RPC request; any failure
- a JSON-RPC error reply, a transport error, or a malformed reply - throws a
catchable `Error{kind: "jsonrpc"}`.

```jennifer
def struct mcp.Client { rpc as jsonrpc.Client };
```

| Call | Returns | |
| ---- | ------- | - |
| `mcp.connect(endpoint)` | `Client` | bind to an MCP endpoint URL |
| `mcp.connectWith(endpoint, headers)` | `Client` | with extra request headers (auth, ...) |
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

- **Stateless protocol only.** No SSE, no session lifecycle. `handle` treats each
  request independently; `notifications/initialized` (and any other
  `notifications/` message) is accepted and answered with no reply.
- **Transports.** The **stdio server** (`serveStdio`) and a **transport-agnostic
  `handle`** (wire to `httpd` / `net`) are implemented on the server side; the
  **HTTP client** (`connect` + the call verbs, over `jsonrpc`) is implemented on
  the client side.
- **The stdio subprocess client is deferred.** A Jennifer program cannot launch
  an MCP server as a subprocess and talk to it bidirectionally: `os.run` /
  `os.spawn` have no interactive stdin pipe, so there is no way to write a
  request line to a child's stdin and read its reply. It is a follow-on that
  needs an `os` subprocess-stdin primitive (a pipe handle to a running child).
  Use the HTTP client, or run `serveStdio` when *you* are the server a host
  launches.
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
- [web.md](web.md) - the web framework, the other `meta.callMain`-dispatched
  request/response module.
- [modules/index.md](index.md) - the module catalog and import rules.
