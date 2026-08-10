# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Model Context Protocol (https://modelcontextprotocol.io) - the stateless
 * JSON-RPC 2.0 surface an LLM host and an MCP peer speak. This module is both a
 * **server** (expose tools / resources / prompts to a host) and a **client**
 * (call a remote MCP server) over either HTTP or a launched stdio subprocess. It
 * is JSON-RPC 2.0 on the wire, so the HTTP client is a thin layer over `jsonrpc`
 * (which is over `http` + `json`) and the server is a purpose-built router
 * (`handle`) that dispatches only to
 * *registered* items - unlike `jsonrpc.handle`'s open name dispatch, MCP is an
 * allow-list.
 *
 * A `Server` is built value-semantically: `server(name, version)` then
 * `addTool` / `addResource` / `addPrompt`, each returning a fresh `Server`. Each
 * registered item names a top-level `func NAME(arg as json.Value)` in the entry
 * program, reached by name via `meta.callMain` (the same mechanism the `web`
 * module uses). `handle(server, requestBody) -> replyBody` is the transport-
 * agnostic dispatcher; `serveStdio(server)` runs the primary stdio transport
 * (newline-delimited JSON-RPC on the program's own stdin/stdout).
 *
 * **Scope: the stateless protocol only.** No SSE, no sessions. The stdio server
 * (`serveStdio`), the HTTP client (`connect`, over `jsonrpc`), and the stdio
 * subprocess client (`connectStdio`, over `os.run`) are all implemented; the
 * stdio client's exchange is one-shot (launch, handshake + op, read replies) since
 * there is no persistent child pipe. Because it uses `jsonrpc` / `http` / `os.run`,
 * this module needs the default `jennifer` binary.
 *
 * **Security.** A tool call is dispatched only when the tool name is in the
 * registry; an unknown tool is a tool error, not an arbitrary-name dispatch. A
 * handler that throws yields a generic error to the peer - the thrown message is
 * never put on the wire, matching `jsonrpc`'s posture.
 *
 * @module mcp
 * @example
 * import "mcp.j" as mcp;
 * use json;
 * func echo(args as json.Value) { return json.asString($args, "/text"); }
 * def sch as json.Value init mcp.property(mcp.schema(), "text", "string", "echo it", true);
 * def base as mcp.Server init mcp.server("demo", "1.0.0");
 * def srv as mcp.Server init mcp.addTool($base, "echo", "echo text", $sch, "echo");
 * def reply as string init mcp.handle($srv, "{\"jsonrpc\":\"2.0\",\"method\":\"ping\",\"id\":1}");
 */
use json;
use io;
use strings;
use convert;
use meta;
use os;
import "./jsonrpc.j" as jsonrpc;

# The MCP protocol revision this module implements, sent in the `initialize`
# handshake by both server and client.
def const PROTOCOL_VERSION as string init "2025-06-18";

# Identity the client reports in its `initialize` request's `clientInfo`.
def const CLIENT_NAME as string init "jennifer-mcp";
def const CLIENT_VERSION as string init "0.1.0";

# The JSON-RPC reserved error codes this router emits (JSON-RPC 2.0 section 5.1).
# A protocol-method miss is METHOD_NOT_FOUND; a bad request body is PARSE_ERROR;
# a missing resource / prompt is INVALID_PARAMS; a handler that throws is
# INTERNAL_ERROR (its message stays server-side).
def const PARSE_ERROR as int init -32700;
def const INVALID_REQUEST as int init -32600;
def const METHOD_NOT_FOUND as int init -32601;
def const INVALID_PARAMS as int init -32602;
def const INTERNAL_ERROR as int init -32603;

# A JSON null value, the `id` echoed on an error reply that has no reliable
# request id (the json write API has no scalar constructor, so decode it once).
def const NULL_VALUE as json.Value init json.decode("null");

# --- registry structs (exported) -----------------------------------

/**
 * A tool the server exposes. Its `handler` names a top-level `func NAME(args as
 * json.Value)` in the entry program that runs when the tool is called.
 * @field name {string} the tool's unique name (the `tools/call` key)
 * @field description {string} a human / model readable description
 * @field handler {string} the top-level method name to dispatch the call to
 * @field inputSchema {json.Value} the JSON Schema for the tool's arguments
 */
export def struct Tool {
    name as string,
    description as string,
    handler as string,
    inputSchema as json.Value
};

/**
 * A resource the server exposes. Its `handler` names a top-level `func
 * NAME(uri as json.Value)` returning the resource's text.
 * @field uri {string} the resource's unique URI (the `resources/read` key)
 * @field name {string} a short display name
 * @field description {string} a human / model readable description
 * @field mimeType {string} the content type of the resource's text
 * @field handler {string} the top-level method name that produces the content
 */
export def struct Resource {
    uri as string,
    name as string,
    description as string,
    mimeType as string,
    handler as string
};

/**
 * One argument a prompt declares, so a host can collect it before `prompts/get`.
 * Build with `promptArg`.
 * @field name {string} the argument's name
 * @field description {string} a human / model readable description
 * @field required {bool} whether the host must supply the argument
 */
export def struct PromptArg {
    name as string,
    description as string,
    required as bool
};

/**
 * A prompt template the server exposes. Its `handler` names a top-level `func
 * NAME(arguments as json.Value)` returning the prompt messages as a json.Value;
 * `arguments` declares what a host should collect (surfaced by `prompts/list`).
 * @field name {string} the prompt's unique name (the `prompts/get` key)
 * @field description {string} a human / model readable description
 * @field arguments {list of PromptArg} the declared arguments (may be empty)
 * @field handler {string} the top-level method name that builds the messages
 */
export def struct Prompt {
    name as string,
    description as string,
    arguments as list of PromptArg,
    handler as string
};

/**
 * An MCP server: an identity plus registries of tools, resources, and prompts.
 * Value-semantic; build with `server` and the `add*` functions.
 * @field name {string} the server name reported in `initialize`
 * @field version {string} the server version reported in `initialize`
 * @field tools {list of Tool} the registered tools
 * @field resources {list of Resource} the registered resources
 * @field prompts {list of Prompt} the registered prompts
 */
export def struct Server {
    name as string,
    version as string,
    tools as list of Tool,
    resources as list of Resource,
    prompts as list of Prompt
};

# --- builders (exported) -------------------------------------------

/**
 * Build an empty server with the given identity.
 * @param name {string} the server name
 * @param version {string} the server version
 * @return {Server} a server with no registered items
 */
export func server(name as string, version as string) {
    def t as list of Tool init [];
    def r as list of Resource init [];
    def p as list of Prompt init [];
    return Server{name: $name, version: $version, tools: $t, resources: $r, prompts: $p};
}

/**
 * Register a tool, returning a fresh server (value-semantic).
 * @param server {Server} the server to extend
 * @param name {string} the tool's unique name
 * @param description {string} a description of the tool
 * @param inputSchema {json.Value} the JSON Schema for the tool's arguments
 * @param handler {string} the top-level method name the call dispatches to
 * @return {Server} a server with the tool added
 */
export func addTool(
    server as Server, name as string, description as string,
    inputSchema as json.Value, handler as string) {
    def s as Server init $server;
    def t as Tool init Tool{
        name: $name, description: $description,
        handler: $handler, inputSchema: $inputSchema};
    def ts as list of Tool init $s.tools;
    $ts[] = $t;
    $s.tools = $ts;
    return $s;
}

/**
 * Register a resource, returning a fresh server (value-semantic).
 * @param server {Server} the server to extend
 * @param uri {string} the resource's unique URI
 * @param name {string} a short display name
 * @param description {string} a description of the resource
 * @param mimeType {string} the content type of the resource's text
 * @param handler {string} the top-level method name that produces the content
 * @return {Server} a server with the resource added
 */
export func addResource(
    server as Server, uri as string, name as string, description as string,
    mimeType as string, handler as string) {
    def s as Server init $server;
    def r as Resource init Resource{
        uri: $uri, name: $name, description: $description,
        mimeType: $mimeType, handler: $handler};
    def rs as list of Resource init $s.resources;
    $rs[] = $r;
    $s.resources = $rs;
    return $s;
}

/**
 * Register a prompt, returning a fresh server (value-semantic). `arguments`
 * declares the arguments a host should collect (build each with `promptArg`; pass
 * `[]` for a prompt that takes none).
 * @param server {Server} the server to extend
 * @param name {string} the prompt's unique name
 * @param description {string} a description of the prompt
 * @param arguments {list of PromptArg} the declared arguments (may be empty)
 * @param handler {string} the top-level method name that builds the messages
 * @return {Server} a server with the prompt added
 */
export func addPrompt(
    server as Server, name as string, description as string,
    arguments as list of PromptArg, handler as string) {
    def s as Server init $server;
    def p as Prompt init Prompt{
        name: $name, description: $description,
        arguments: $arguments, handler: $handler};
    def ps as list of Prompt init $s.prompts;
    $ps[] = $p;
    $s.prompts = $ps;
    return $s;
}

/**
 * Build one prompt argument declaration for `addPrompt`.
 * @param name {string} the argument's name
 * @param description {string} a description of the argument
 * @param required {bool} whether the host must supply it
 * @return {PromptArg} the argument declaration
 */
export func promptArg(name as string, description as string, required as bool) {
    return PromptArg{name: $name, description: $description, required: $required};
}

# --- schema builder (exported) -------------------------------------

/**
 * Build an empty JSON Schema object skeleton (`type: object`) for a tool's
 * `inputSchema`. Add fields with `property`.
 * @return {json.Value} a `{"type":"object","properties":{},"required":[]}` skeleton
 */
export func schema() {
    return json.decode('{"type":"object","properties":{},"required":[]}');
}

/**
 * Add a property to a schema, returning a fresh schema. Sets
 * `properties/<name> = {type, description}` and appends `name` to `required`
 * when `required` is true.
 * @param schema {json.Value} the schema to extend
 * @param name {string} the property name
 * @param jsonType {string} the JSON Schema type (`"string"`, `"integer"`, ...)
 * @param description {string} a description of the property
 * @param required {bool} whether the property is required
 * @return {json.Value} the extended schema
 */
export func property(
    schema as json.Value, name as string, jsonType as string,
    description as string, required as bool) {
    def s as json.Value init $schema;
    def prop as json.Value init json.map();
    $prop = json.set($prop, "/type", $jsonType);
    $prop = json.set($prop, "/description", $description);
    $s = json.set($s, "/properties/" + $name, $prop);
    if ($required) {
        $s = json.append($s, "/required", $name);
    }
    return $s;
}

# --- server dispatch (exported) ------------------------------------

/**
 * Dispatch a single JSON-RPC request body against the server and return the
 * reply body (transport-agnostic: wire it to `httpd` / `net` / stdio). A
 * notification (no `id`, or any `notifications/` method) yields `""`. A parse
 * failure is a `-32700` reply; an unknown protocol method is `-32601`. A
 * `tools/call` to an unknown tool, or a tool handler that throws, is a
 * successful reply carrying a tool result with `isError: true` (the thrown
 * message is never put on the wire).
 * @param server {Server} the server whose registries route the request
 * @param requestBody {string} the raw request JSON
 * @return {string} the reply JSON, or `""` when no reply is owed
 */
export func handle(server as Server, requestBody as string) {
    def req as json.Value;
    try {
        $req = json.decode($requestBody);
    } catch (e) {
        return encodeError(NULL_VALUE, PARSE_ERROR, "Parse error");
    }
    if (json.typeOf($req, "") != "map" or not json.has($req, "/method") or
        json.typeOf($req, "/method") != "string") {
        return encodeError(idValue($req), INVALID_REQUEST, "Invalid Request");
    }
    def method as string init json.asString($req, "/method");
    # A notification (any notifications/ method, or a request with no id) owes
    # no reply.
    if (strings.startsWith($method, "notifications/") or not json.has($req, "/id")) {
        return "";
    }
    def params as json.Value init json.map();
    if (json.has($req, "/params")) {
        $params = json.get($req, "/params");
    }
    # Defensive net: the request is untrusted, so a shape the router did not
    # anticipate must become an error reply, never an uncaught throw that would
    # crash the transport loop.
    try {
        return route($server, $method, $params, idValue($req));
    } catch (e) {
        return encodeError(idValue($req), INTERNAL_ERROR, "Internal error");
    }
}

# route dispatches a resolved (method, params, id) to the matching handler and
# returns the encoded reply. Split out of `handle` so the router is one place.
func route(server as Server, method as string, params as json.Value, id as json.Value) {
    match ($method) {
        when "initialize" {
            return encodeResult($id, initializeResult($server));
        }
        when "ping" {
            return encodeResult($id, json.map());
        }
        when "tools/list" {
            return encodeResult($id, toolsListResult($server));
        }
        when "tools/call" {
            return encodeResult($id, toolsCallResult($server, $params));
        }
        when "resources/list" {
            return encodeResult($id, resourcesListResult($server));
        }
        when "resources/read" {
            return resourcesReadReply($server, $params, $id);
        }
        when "prompts/list" {
            return encodeResult($id, promptsListResult($server));
        }
        when "prompts/get" {
            return promptsGetReply($server, $params, $id);
        }
        else {
            return encodeError($id, METHOD_NOT_FOUND, "Method not found");
        }
    }
}

/**
 * Run the stdio MCP server: read newline-delimited JSON-RPC requests from the
 * program's own stdin, dispatch each through `handle`, and write each non-empty
 * reply to stdout. Blank lines are skipped; the loop ends at EOF. This is the
 * primary MCP transport (an LLM host launches the program and speaks to it over
 * stdin/stdout).
 * @param server {Server} the server whose registries route each request
 */
export func serveStdio(server as Server) {
    while (not io.eof()) {
        def line as string init io.readLine();
        if (len(strings.trim($line)) > 0) {
            def reply as string init handle($server, $line);
            if (len($reply) > 0) {
                io.printf("%s\n", $reply);
            }
        }
    }
    return;
}

# --- server result builders (private) ------------------------------

# initializeResult builds the `initialize` result: the protocol version, the
# three always-present capability objects, and the server identity.
func initializeResult(server as Server) {
    def res as json.Value init json.map();
    $res = json.set($res, "/protocolVersion", PROTOCOL_VERSION);
    def caps as json.Value init json.map();
    $caps = json.set($caps, "/tools", json.map());
    $caps = json.set($caps, "/resources", json.map());
    $caps = json.set($caps, "/prompts", json.map());
    $res = json.set($res, "/capabilities", $caps);
    def info as json.Value init json.map();
    $info = json.set($info, "/name", $server.name);
    $info = json.set($info, "/version", $server.version);
    $res = json.set($res, "/serverInfo", $info);
    return $res;
}

# toolsListResult builds the `tools/list` result from the registry.
func toolsListResult(server as Server) {
    def arr as json.Value init json.list();
    for (def t in $server.tools) {
        def item as json.Value init json.map();
        $item = json.set($item, "/name", $t.name);
        $item = json.set($item, "/description", $t.description);
        $item = json.set($item, "/inputSchema", $t.inputSchema);
        $arr = json.append($arr, "", $item);
    }
    def res as json.Value init json.map();
    $res = json.set($res, "/tools", $arr);
    return $res;
}

# toolsCallResult runs a registered tool and wraps its return into a tool
# result. An unknown tool, or a handler that throws, is a tool error (isError
# true) - never an arbitrary-name dispatch, and never a leaked thrown message.
func toolsCallResult(server as Server, params as json.Value) {
    def name as string init stringParam($params, "/name");
    def arguments as json.Value init json.map();
    if (json.has($params, "/arguments")) {
        $arguments = json.get($params, "/arguments");
    }
    def idx as int init findTool($server, $name);
    if ($idx < 0) {
        return toolResult("Unknown tool: " + $name, true);
    }
    def tool as Tool init $server.tools[$idx];
    def text as string;
    try {
        def ret as json.Value init json.map();
        $ret = json.set($ret, "/v", meta.callMain($tool.handler, $arguments));
        $text = valueText($ret, "/v");
    } catch (e) {
        return toolResult("Tool execution failed", true);
    }
    return toolResult($text, false);
}

# resourcesListResult builds the `resources/list` result from the registry.
func resourcesListResult(server as Server) {
    def arr as json.Value init json.list();
    for (def r in $server.resources) {
        def item as json.Value init json.map();
        $item = json.set($item, "/uri", $r.uri);
        $item = json.set($item, "/name", $r.name);
        $item = json.set($item, "/description", $r.description);
        $item = json.set($item, "/mimeType", $r.mimeType);
        $arr = json.append($arr, "", $item);
    }
    def res as json.Value init json.map();
    $res = json.set($res, "/resources", $arr);
    return $res;
}

# resourcesReadReply resolves the requested uri, runs its handler, and returns
# the encoded reply (a -32602 error when the uri is unregistered, a -32603 when
# the handler throws).
func resourcesReadReply(server as Server, params as json.Value, id as json.Value) {
    def uri as string init stringParam($params, "/uri");
    def idx as int init findResource($server, $uri);
    if ($idx < 0) {
        return encodeError($id, INVALID_PARAMS, "Resource not found");
    }
    def resource as Resource init $server.resources[$idx];
    try {
        def ret as json.Value init json.map();
        $ret = json.set($ret, "/v", meta.callMain($resource.handler, uriValue($uri)));
        def text as string init valueText($ret, "/v");
        def item as json.Value init json.map();
        $item = json.set($item, "/uri", $uri);
        $item = json.set($item, "/mimeType", $resource.mimeType);
        $item = json.set($item, "/text", $text);
        def contents as json.Value init json.list();
        $contents = json.append($contents, "", $item);
        def res as json.Value init json.map();
        $res = json.set($res, "/contents", $contents);
        return encodeResult($id, $res);
    } catch (e) {
        return encodeError($id, INTERNAL_ERROR, "Internal error");
    }
}

# promptsListResult builds the `prompts/list` result from the registry.
func promptsListResult(server as Server) {
    def arr as json.Value init json.list();
    for (def p in $server.prompts) {
        def item as json.Value init json.map();
        $item = json.set($item, "/name", $p.name);
        $item = json.set($item, "/description", $p.description);
        if (len($p.arguments) > 0) {
            def args as json.Value init json.list();
            for (def a in $p.arguments) {
                def ad as json.Value init json.map();
                $ad = json.set($ad, "/name", $a.name);
                $ad = json.set($ad, "/description", $a.description);
                $ad = json.set($ad, "/required", $a.required);
                $args = json.append($args, "", $ad);
            }
            $item = json.set($item, "/arguments", $args);
        }
        $arr = json.append($arr, "", $item);
    }
    def res as json.Value init json.map();
    $res = json.set($res, "/prompts", $arr);
    return $res;
}

# promptsGetReply resolves the requested prompt, runs its handler for the
# messages, and returns the encoded reply (a -32602 error when the name is
# unregistered, a -32603 when the handler throws).
func promptsGetReply(server as Server, params as json.Value, id as json.Value) {
    def name as string init stringParam($params, "/name");
    def idx as int init findPrompt($server, $name);
    if ($idx < 0) {
        return encodeError($id, INVALID_PARAMS, "Prompt not found");
    }
    def prompt as Prompt init $server.prompts[$idx];
    def arguments as json.Value init json.map();
    if (json.has($params, "/arguments")) {
        $arguments = json.get($params, "/arguments");
    }
    try {
        def messages as json.Value init meta.callMain($prompt.handler, $arguments);
        def res as json.Value init json.map();
        $res = json.set($res, "/description", $prompt.description);
        $res = json.set($res, "/messages", $messages);
        return encodeResult($id, $res);
    } catch (e) {
        return encodeError($id, INTERNAL_ERROR, "Internal error");
    }
}

# toolResult wraps text into the MCP tool-result shape
# ({content:[{type:"text", text}], isError}).
func toolResult(text as string, isError as bool) {
    def item as json.Value init json.map();
    $item = json.set($item, "/type", "text");
    $item = json.set($item, "/text", $text);
    def content as json.Value init json.list();
    $content = json.append($content, "", $item);
    def res as json.Value init json.map();
    $res = json.set($res, "/content", $content);
    $res = json.set($res, "/isError", $isError);
    return $res;
}

# valueText renders a handler's json.Value-or-scalar return at `ptr` as text: a
# string is used verbatim, anything else is JSON-encoded.
func valueText(held as json.Value, ptr as string) {
    if (json.typeOf($held, $ptr) == "string") {
        return json.asString($held, $ptr);
    }
    return json.encode(json.get($held, $ptr));
}

# uriValue wraps a uri string into a json.Value scalar to hand to a resource
# handler (the json write API has no direct scalar constructor).
func uriValue(uri as string) {
    return json.get(json.set(json.map(), "/u", $uri), "/u");
}

# stringParam reads a string param at `ptr`, returning "" when it is absent OR
# not a string. The request is untrusted, so a non-string where a string is
# expected must be a clean protocol error, never a bare `json.asString` throw -
# which, being uncaught in the dispatch path, would otherwise crash the whole
# server (e.g. the serveStdio loop) on one malformed request.
func stringParam(params as json.Value, ptr as string) {
    if (json.has($params, $ptr) and json.typeOf($params, $ptr) == "string") {
        return json.asString($params, $ptr);
    }
    return "";
}

# findTool / findResource / findPrompt return a registry index by name/uri, or
# -1 when absent - the allow-list lookups that keep dispatch to registered items.
func findTool(server as Server, name as string) {
    def i as int init 0;
    while ($i < len($server.tools)) {
        if ($server.tools[$i].name == $name) {
            return $i;
        }
        $i = $i + 1;
    }
    return -1;
}

func findResource(server as Server, uri as string) {
    def i as int init 0;
    while ($i < len($server.resources)) {
        if ($server.resources[$i].uri == $uri) {
            return $i;
        }
        $i = $i + 1;
    }
    return -1;
}

func findPrompt(server as Server, name as string) {
    def i as int init 0;
    while ($i < len($server.prompts)) {
        if ($server.prompts[$i].name == $name) {
            return $i;
        }
        $i = $i + 1;
    }
    return -1;
}

# idValue returns the request's `id` as a json.Value to echo, or JSON null.
func idValue(req as json.Value) {
    if (json.typeOf($req, "") == "map" and json.has($req, "/id")) {
        return json.get($req, "/id");
    }
    return NULL_VALUE;
}

# encodeResult renders a success reply ({jsonrpc, result, id}).
func encodeResult(id as json.Value, result as json.Value) {
    def resp as json.Value init json.map();
    $resp = json.set($resp, "/jsonrpc", "2.0");
    $resp = json.set($resp, "/result", $result);
    $resp = json.set($resp, "/id", $id);
    return json.encode($resp);
}

# encodeError renders an error reply ({jsonrpc, error:{code, message}, id}).
func encodeError(id as json.Value, code as int, message as string) {
    def resp as json.Value init json.map();
    $resp = json.set($resp, "/jsonrpc", "2.0");
    $resp = json.set($resp, "/error", json.map());
    $resp = json.set($resp, "/error/code", $code);
    $resp = json.set($resp, "/error/message", $message);
    $resp = json.set($resp, "/id", $id);
    return json.encode($resp);
}

# --- client (exported) ---------------------------------------------

/**
 * An MCP client. Over HTTP (`connect` / `connectWith`) it wraps a
 * `jsonrpc.Client`; over stdio (`connectStdio`) it holds the server's `argv` and
 * drives it as a subprocess. The same call surface (`initialize` / `listTools` /
 * `callTool` / ...) works for both transports.
 * @field rpc {jsonrpc.Client} the underlying JSON-RPC HTTP client (HTTP transport)
 * @field argv {list of string} the stdio server command line (stdio transport)
 * @field isStdio {bool} true for the stdio transport, false for HTTP
 */
export def struct Client {
    rpc as jsonrpc.Client,
    argv as list of string,
    isStdio as bool
};

/**
 * Connect to a remote MCP server over HTTP.
 * @param endpoint {string} the MCP endpoint URL (`http://` or `https://`)
 * @return {Client} a configured client
 */
export func connect(endpoint as string) {
    def noArgv as list of string init [];
    return Client{rpc: jsonrpc.client($endpoint), argv: $noArgv, isStdio: false};
}

/**
 * Connect to a remote MCP server with extra request headers (auth, ...).
 * @param endpoint {string} the MCP endpoint URL
 * @param headers {map of string to string} headers sent with every request
 * @return {Client} a configured client
 */
export func connectWith(endpoint as string, headers as map of string to string) {
    def noArgv as list of string init [];
    return Client{rpc: jsonrpc.clientWith($endpoint, $headers), argv: $noArgv, isStdio: false};
}

/**
 * Connect to an MCP server launched as a subprocess over **stdio** - the primary
 * MCP transport (a host runs the server and speaks newline-delimited JSON-RPC on
 * its stdin/stdout). `argv` is the server command line (program first). Because
 * the exchange is one-shot (`os.run`, no persistent pipe), each call launches the
 * server, sends the `initialize` handshake plus the operation as newline-delimited
 * JSON on stdin, closes stdin, and reads the replies from stdout - so a stateless
 * server sees a complete short session per call. Needs the default binary
 * (`os.run`).
 * @param argv {list of string} the server command line (`["python", "srv.py"]`, ...)
 * @return {Client} a configured stdio client
 */
export func connectStdio(argv as list of string) {
    return Client{rpc: jsonrpc.client(""), argv: $argv, isStdio: true};
}

/**
 * Perform the `initialize` handshake and return the server's result (its
 * protocol version, capabilities, and serverInfo). Per the MCP lifecycle, this
 * also sends the required `notifications/initialized` once the handshake
 * succeeds, so the session is ready for further requests. Throws `Error{kind:
 * "jsonrpc"}` on any transport or protocol failure.
 * @param client {Client} the MCP client
 * @return {json.Value} the server's `initialize` result
 */
export func initialize(client as Client) {
    def params as json.Value init initParams();
    if ($client.isStdio) {
        # the stdio exchange already carries the handshake (initialize + the
        # initialized notification) in one launch
        return stdioCall($client.argv, "initialize", $params);
    }
    def result as json.Value init jsonrpc.call($client.rpc, "initialize", $params);
    jsonrpc.notify($client.rpc, "notifications/initialized", json.map());
    return $result;
}

/**
 * List the remote server's tools. Returns the `tools` array from the
 * `tools/list` result.
 * @param client {Client} the MCP client
 * @return {json.Value} the array of tool descriptors
 */
export func listTools(client as Client) {
    def res as json.Value init mcpCall($client, "tools/list", json.map());
    return json.get($res, "/tools");
}

/**
 * Call a remote tool by name. Returns the whole tool result (a `content` array
 * plus an `isError` flag), so the caller can check `isError`.
 * @param client {Client} the MCP client
 * @param name {string} the tool name
 * @param arguments {json.Value} the tool arguments object
 * @return {json.Value} the `tools/call` result
 */
export func callTool(client as Client, name as string, arguments as json.Value) {
    def params as json.Value init json.map();
    $params = json.set($params, "/name", $name);
    $params = json.set($params, "/arguments", $arguments);
    return mcpCall($client, "tools/call", $params);
}

/**
 * List the remote server's resources. Returns the `resources` array from the
 * `resources/list` result.
 * @param client {Client} the MCP client
 * @return {json.Value} the array of resource descriptors
 */
export func listResources(client as Client) {
    def res as json.Value init mcpCall($client, "resources/list", json.map());
    return json.get($res, "/resources");
}

/**
 * Read a remote resource by URI. Returns the whole `resources/read` result (a
 * `contents` array).
 * @param client {Client} the MCP client
 * @param uri {string} the resource URI
 * @return {json.Value} the `resources/read` result
 */
export func readResource(client as Client, uri as string) {
    def params as json.Value init json.set(json.map(), "/uri", $uri);
    return mcpCall($client, "resources/read", $params);
}

/**
 * List the remote server's prompts. Returns the `prompts` array from the
 * `prompts/list` result.
 * @param client {Client} the MCP client
 * @return {json.Value} the array of prompt descriptors
 */
export func listPrompts(client as Client) {
    def res as json.Value init mcpCall($client, "prompts/list", json.map());
    return json.get($res, "/prompts");
}

/**
 * Get a remote prompt by name. Returns the whole `prompts/get` result (a
 * description plus the messages array).
 * @param client {Client} the MCP client
 * @param name {string} the prompt name
 * @param arguments {json.Value} the prompt arguments object
 * @return {json.Value} the `prompts/get` result
 */
export func getPrompt(client as Client, name as string, arguments as json.Value) {
    def params as json.Value init json.map();
    $params = json.set($params, "/name", $name);
    $params = json.set($params, "/arguments", $arguments);
    return mcpCall($client, "prompts/get", $params);
}

# --- client transport dispatch (private) ---------------------------

# initParams builds the client's `initialize` params (protocol version, empty
# capabilities, clientInfo). Shared by the HTTP and stdio handshakes.
func initParams() {
    def params as json.Value init json.map();
    $params = json.set($params, "/protocolVersion", PROTOCOL_VERSION);
    $params = json.set($params, "/capabilities", json.map());
    def info as json.Value init json.map();
    $info = json.set($info, "/name", CLIENT_NAME);
    $info = json.set($info, "/version", CLIENT_VERSION);
    $params = json.set($params, "/clientInfo", $info);
    return $params;
}

# mcpCall dispatches one request/response to the client's transport: HTTP through
# jsonrpc, or a one-shot stdio subprocess exchange. Returns the reply's result (a
# json.Value), or throws Error{kind: "mcp"} / Error{kind: "jsonrpc"} on failure.
func mcpCall(client as Client, method as string, params as json.Value) {
    if ($client.isStdio) {
        return stdioCall($client.argv, $method, $params);
    }
    return jsonrpc.call($client.rpc, $method, $params);
}

# stdioRequest / stdioNotification render one JSON-RPC request / notification as a
# single line for the stdio wire.
func stdioRequest(method as string, params as json.Value, id as int) {
    def r as json.Value init json.map();
    $r = json.set($r, "/jsonrpc", "2.0");
    $r = json.set($r, "/method", $method);
    $r = json.set($r, "/params", $params);
    $r = json.set($r, "/id", $id);
    return json.encode($r);
}

func stdioNotification(method as string) {
    def r as json.Value init json.map();
    $r = json.set($r, "/jsonrpc", "2.0");
    $r = json.set($r, "/method", $method);
    return json.encode($r);
}

# stdioCall runs one MCP operation over a freshly launched stdio server: it writes
# the `initialize` handshake (initialize + the initialized notification) plus the
# operation as newline-delimited JSON on the child's stdin, closes it, and reads
# the operation's reply from stdout. `initialize` itself needs no extra op line,
# so its own reply (id 1) is returned. Deadlock-free via os.run (input buffered,
# output captured). A launch failure or non-zero exit throws Error{kind: "mcp"}.
func stdioCall(argv as list of string, method as string, params as json.Value) {
    def lines as list of string init [];
    def opId as int init 2;
    if ($method == "initialize") {
        $opId = 1;
        $lines[] = stdioRequest("initialize", $params, 1);
        $lines[] = stdioNotification("notifications/initialized");
    } else {
        $lines[] = stdioRequest("initialize", initParams(), 1);
        $lines[] = stdioNotification("notifications/initialized");
        $lines[] = stdioRequest($method, $params, 2);
    }
    def blob as string init strings.join($lines, "\n") + "\n";
    def res as os.Result;
    try {
        $res = os.run($argv, $blob);
    } catch (e) {
        throw Error{
            kind: "mcp",
            message: "mcp: cannot run the stdio server: " + $e.message,
            file: "", line: 0, col: 0};
    }
    if ($res.exitCode != 0) {
        throw Error{
            kind: "mcp",
            message: "mcp: stdio server exited with code " + convert.toString($res.exitCode),
            file: "", line: 0, col: 0};
    }
    return stdioReply($res.stdout, $opId);
}

# stdioReply scans the server's newline-delimited stdout for the reply whose `id`
# matches `opId`, returning its `result` (or throwing on a JSON-RPC error reply).
# Non-JSON lines are skipped defensively; a missing reply throws Error{kind:"mcp"}.
func stdioReply(stdout as string, opId as int) {
    for (def line in strings.split($stdout, "\n")) {
        def t as string init strings.trim($line);
        if (len($t) == 0) {
            continue;
        }
        def reply as json.Value init NULL_VALUE;
        def ok as bool init true;
        try {
            $reply = json.decode($t);
        } catch (e) {
            $ok = false;
        }
        if ($ok and json.has($reply, "/id") and json.typeOf($reply, "/id") == "int" and
            json.asInt($reply, "/id") == $opId) {
            if (json.has($reply, "/error")) {
                def code as int init json.asInt($reply, "/error/code");
                def msg as string init json.asString($reply, "/error/message");
                throw Error{
                    kind: "mcp",
                    message: "mcp error " + convert.toString($code) + ": " + $msg,
                    file: "", line: 0, col: 0};
            }
            return json.get($reply, "/result");
        }
    }
    throw Error{
        kind: "mcp",
        message: "mcp: no reply for request id " + convert.toString($opId) +
            " from the stdio server",
        file: "", line: 0, col: 0};
}
