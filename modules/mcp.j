# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.28.0
# pragma-jennifer-capability: exec

/**
 * Model Context Protocol (https://modelcontextprotocol.io) - the stateless
 * JSON-RPC 2.0 surface an LLM host and an MCP peer speak. This module is both a
 * **server** (expose tools / resources / prompts to a host) and a **client**
 * (call a remote MCP server) over either HTTP or a launched stdio subprocess. It
 * is JSON-RPC 2.0 on the wire, so the HTTP client posts over `http` and reads
 * replies with `jsonrpc.parseReply`, and the server is a purpose-built router
 * (`handle`) that dispatches only to
 * *registered* items - unlike `jsonrpc.handle`'s open name dispatch, MCP is an
 * allow-list.
 *
 * A `Server` is built value-semantically: `server(name, version)` then
 * `addTool` / `addResource` / `addPrompt`, each returning a fresh `Server`. Each
 * registered item is handled by a top-level `func NAME(arg as json.Value)` in the
 * entry program, passed as a `func` value and called through its home interpreter
 * (the same mechanism the `web` module uses), so it resolves its own imports and
 * builds its `json.Value` result in the entry program's context.
 * `handle(server, requestBody) -> replyBody` is the transport-
 * agnostic dispatcher; `serveStdio(server)` runs the primary stdio transport
 * (newline-delimited JSON-RPC on the program's own stdin/stdout).
 *
 * **Scope: the stateless protocol only.** No sessions, and no server-sent
 * events beyond a POST's own response stream. The stdio server (`serveStdio`),
 * the HTTP client (`connect`), and the stdio subprocess client (`connectStdio`,
 * over `os.spawn` and `/bin/sh`) are all implemented; the stdio client's
 * exchange is one-shot (launch, handshake + op, read replies) since there is no
 * persistent child pipe. Because it uses `http` / `os.spawn`, this module needs
 * the default `jennifer` binary.
 *
 * **Security.** A tool call is dispatched only when the tool name is in the
 * registry; an unknown tool is a tool error, not an arbitrary-name dispatch. A
 * handler that throws yields a generic error to the peer - the thrown message is
 * never put on the wire, matching `jsonrpc`'s posture.
 *
 * **Client errors.** Over HTTP a failure is the `jsonrpc` module's
 * `jsonrpc.RpcError` (kind `"jsonrpc"`); over stdio it is an `mcp.McpError`
 * (kind `"mcp"`). Both carry the server's JSON-RPC `code` and `data` for an
 * error reply, and a `reason` for the other failures. A socket failure over
 * HTTP is a `net.NetError` (kind `"net"`), a reached cap kind `"limit"`, a
 * stdio server that does not answer within the client's `stdioTimeoutMs` kind
 * `"timeout"`. A client carrying an `Authorization` header refuses an
 * `http://` endpoint on a non-loopback host before connecting, unless its
 * `allowInsecureHttp` field is set.
 *
 * @module mcp
 * @example
 * import "mcp.j" as mcp;
 * use json;
 * func echo(args as json.Value) { return json.asString($args, "/text"); }
 * def sch as json.Value init mcp.property(mcp.schema(), "text", "string", "echo it", true);
 * def base as mcp.Server init mcp.server("demo", "1.0.0");
 * def srv as mcp.Server init mcp.addTool($base, "echo", "echo text", $sch, echo);
 * def reply as string init mcp.handle($srv, '{"jsonrpc":"2.0","method":"ping","id":1}');
 */
use encoding;
use json;
use io;
use strings;
use os;
use fs;
use path;
use time;
use binary;
use convert;
import "./http.j" as http;
import "./jsonrpc.j" as jsonrpc;

/**
 * A failed exchange with a stdio MCP server. Every error of kind `"mcp"` is one,
 * so its fields are safe to read once the kind matches. (Over HTTP the client
 * throws `jsonrpc.RpcError`, kind `"jsonrpc"`.)
 * @field kind {string} always `"mcp"`
 * @field message {string} `mcp error <code>: <message>` for an error reply, else what went wrong
 * @field reason {string} `"rpc"` (the server answered with an error object), `"launch"` (the server could not be run), `"exit"` (it exited non-zero), `"protocol"` (its stdout carried something other than JSON-RPC 2.0 messages, or a malformed reply) or `"noreply"` (no reply for the request)
 * @field code {int} the error object's `code` (such as -32602, invalid params); 0 unless `reason` is `"rpc"` or the object had none
 * @field data {json.Value} the error object's `data` member; JSON `null` when it has none or `reason` is not `"rpc"`
 * @field exitCode {int} the server's exit code when `reason` is `"exit"`; 0 otherwise
 */
export def struct McpError {
    kind as string,
    message as string,
    reason as string,
    code as int,
    data as json.Value,
    exitCode as int
};

# mcpError is the McpError of a failure that carries no error object.
func mcpError(reason as string, message as string, exitCode as int) {
    return McpError{
        kind: "mcp",
        message: $message,
        reason: $reason,
        code: 0,
        data: json.decode("null"),
        exitCode: $exitCode
    };
}

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
 * A tool the server exposes. Its `handler` is a `func` value `func NAME(args as
 * json.Value)` in the entry program that runs when the tool is called.
 * @field name {string} the tool's unique name (the `tools/call` key)
 * @field description {string} a human / model readable description
 * @field handler {func} the handler func value to dispatch the call to
 * @field inputSchema {json.Value} the JSON Schema for the tool's arguments
 */
export def struct Tool {
    name as string,
    description as string,
    handler as func,
    inputSchema as json.Value
};

/**
 * A resource the server exposes. Its `handler` is a `func` value `func
 * NAME(uri as json.Value)` returning the resource's text.
 * @field uri {string} the resource's unique URI (the `resources/read` key)
 * @field name {string} a short display name
 * @field description {string} a human / model readable description
 * @field mimeType {string} the content type of the resource's text
 * @field handler {func} the handler func value that produces the content
 */
export def struct Resource {
    uri as string,
    name as string,
    description as string,
    mimeType as string,
    handler as func
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
 * A prompt template the server exposes. Its `handler` is a `func` value `func
 * NAME(arguments as json.Value)` returning the prompt messages as a json.Value;
 * `arguments` declares what a host should collect (surfaced by `prompts/list`).
 * @field name {string} the prompt's unique name (the `prompts/get` key)
 * @field description {string} a human / model readable description
 * @field arguments {list of PromptArg} the declared arguments (may be empty)
 * @field handler {func} the handler func value that builds the messages
 */
export def struct Prompt {
    name as string,
    description as string,
    arguments as list of PromptArg,
    handler as func
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
 * @param handler {func} the handler func value the call dispatches to
 * @return {Server} a server with the tool added
 */
export func addTool(
    server as Server,
    name as string,
    description as string,
    inputSchema as json.Value,
    handler as func) {
    def s as Server init $server;
    def t as Tool init Tool{
        name: $name,
        description: $description,
        handler: $handler,
        inputSchema: $inputSchema
    };
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
 * @param handler {func} the handler func value that produces the content
 * @return {Server} a server with the resource added
 */
export func addResource(
    server as Server,
    uri as string,
    name as string,
    description as string,
    mimeType as string,
    handler as func) {
    def s as Server init $server;
    def r as Resource init Resource{
        uri: $uri,
        name: $name,
        description: $description,
        mimeType: $mimeType,
        handler: $handler
    };
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
 * @param handler {func} the handler func value that builds the messages
 * @return {Server} a server with the prompt added
 */
export func addPrompt(
    server as Server,
    name as string,
    description as string,
    arguments as list of PromptArg,
    handler as func) {
    def s as Server init $server;
    def p as Prompt init Prompt{
        name: $name,
        description: $description,
        arguments: $arguments,
        handler: $handler
    };
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
    schema as json.Value,
    name as string,
    jsonType as string,
    description as string,
    required as bool) {
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
 * failure is a `-32700` reply; a request that is not a JSON-RPC 2.0 request
 * (not an object, a `jsonrpc` other than `"2.0"`, a non-string `method`, an
 * `id` that is not a string or an integer) is `-32600`, with a null id when
 * the id is the problem; `params` that are not an object are `-32602`; an
 * unknown protocol method is `-32601`. A
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
    if (json.typeOf($req, "") != "map") {
        return encodeError(NULL_VALUE, INVALID_REQUEST, "Invalid Request");
    }
    # MCP ids are a string or an integer (never null); any other cannot be
    # echoed, so the request is invalid and answered with a null id.
    def idType as string init json.typeOf($req, "/id");
    if ($idType != "missing" and $idType != "string" and $idType != "int") {
        return encodeError(NULL_VALUE, INVALID_REQUEST, "Invalid Request");
    }
    if (json.typeOf($req, "/jsonrpc") != "string" or json.asString($req, "/jsonrpc") != "2.0" or
        json.typeOf($req, "/method") != "string") {
        return encodeError(idValue($req), INVALID_REQUEST, "Invalid Request");
    }
    def method as string init json.asString($req, "/method");
    # A notification (any notifications/ method, or a request with no id) owes
    # no reply.
    if (strings.startsWith($method, "notifications/") or $idType == "missing") {
        return "";
    }
    # MCP params are an object.
    def params as json.Value init json.map();
    if (json.has($req, "/params")) {
        if (json.typeOf($req, "/params") != "map") {
            return encodeError(idValue($req), INVALID_PARAMS, "Invalid params");
        }
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
        when "initialize" { return encodeResult($id, initializeResult($server)); }
        when "ping" { return encodeResult($id, json.map()); }
        when "tools/list" { return encodeResult($id, toolsListResult($server)); }
        when "tools/call" { return encodeResult($id, toolsCallResult($server, $params)); }
        when "resources/list" { return encodeResult($id, resourcesListResult($server)); }
        when "resources/read" { return resourcesReadReply($server, $params, $id); }
        when "prompts/list" { return encodeResult($id, promptsListResult($server)); }
        when "prompts/get" { return promptsGetReply($server, $params, $id); }
        else { return encodeError($id, METHOD_NOT_FOUND, "Method not found"); }
    }
}

/**
 * Run the stdio MCP server: read newline-delimited JSON-RPC requests from the
 * program's own stdin, dispatch each through `handle`, and write each non-empty
 * reply to stdout. Blank lines are skipped; the loop ends at EOF. This is the
 * primary MCP transport (an LLM host launches the program and speaks to it over
 * stdin/stdout). A line that is not UTF-8, or one over `io.readLine`'s cap, is
 * answered with a `-32700` parse error, like one that is not JSON, and the loop
 * goes on; the rest of an over-cap line is skipped, not read as a request.
 * @param server {Server} the server whose registries route each request
 */
export func serveStdio(server as Server) {
    while (not io.eof()) {
        def line as string init "";
        try {
            $line = io.readLine();
        } catch (e) {
            # A line that is not UTF-8 or is over io's cap is a client's bad
            # request, not the end of the session: answer it as a parse error
            # and go on. A UTF-8 error consumed the line; a "limit" consumed
            # only the bytes up to the cap, so the rest of the line is skipped
            # first, or it would be read as the next request. A cancelled
            # task stops.
            if ($e.kind == "cancelled") {
                throw $e;
            }
            if ($e.kind == "limit") {
                skipRestOfLine();
            }
            io.printf("%s\n", encodeError(NULL_VALUE, PARSE_ERROR, "Parse error"));
            continue;
        }
        if (len(strings.trim($line)) > 0) {
            def reply as string init handle($server, $line);
            if (len($reply) > 0) {
                io.printf("%s\n", $reply);
            }
        }
    }
    return;
}

# skipRestOfLine discards stdin up to and including the next newline, after
# io.readLine refused a line over its cap. Each read is capped like any other
# line (io.readBytes cannot stop at the newline and would eat the next
# request). It returns at EOF too.
func skipRestOfLine() {
    while (not io.eof()) {
        try {
            io.readLine();
            return;
        } catch (e) {
            if ($e.kind == "cancelled") {
                throw $e;
            }
            if ($e.kind != "limit") {
                return;
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
        def h as func init $tool.handler;
        $ret = json.set($ret, "/v", $h($arguments));
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
        def h as func init $resource.handler;
        $ret = json.set($ret, "/v", $h(uriValue($uri)));
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
        def h as func init $prompt.handler;
        def messages as json.Value init $h($arguments);
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
 * @field allowInsecureHttp {bool} allow an `Authorization` header over plain `http://` to a non-loopback host (HTTP transport; default false)
 * @field stdioTimeoutMs {int} how long one stdio exchange may take, launch to reply, before the server is killed (0 = 60 s)
 */
export def struct Client {
    rpc as jsonrpc.Client,
    argv as list of string,
    isStdio as bool,
    allowInsecureHttp as bool,
    stdioTimeoutMs as int
};

/**
 * Connect to a remote MCP server over HTTP.
 * @param endpoint {string} the MCP endpoint URL (`http://` or `https://`)
 * @return {Client} a configured client
 */
export func connect(endpoint as string) {
    def noArgv as list of string init [];
    return Client{
        rpc: jsonrpc.client($endpoint),
        argv: $noArgv,
        isStdio: false,
        allowInsecureHttp: false,
        stdioTimeoutMs: 0
    };
}

/**
 * Connect to a remote MCP server with extra request headers (auth, ...).
 * @param endpoint {string} the MCP endpoint URL
 * @param headers {map of string to string} headers sent with every request
 * @return {Client} a configured client
 */
export func connectWith(endpoint as string, headers as map of string to string) {
    def noArgv as list of string init [];
    return Client{
        rpc: jsonrpc.clientWith($endpoint, $headers),
        argv: $noArgv,
        isStdio: false,
        allowInsecureHttp: false,
        stdioTimeoutMs: 0
    };
}

/**
 * Connect to an MCP server launched as a subprocess over **stdio** - the primary
 * MCP transport (a host runs the server and speaks newline-delimited JSON-RPC on
 * its stdin/stdout). `argv` is the server command line (program first). Because
 * the exchange is one-shot (no persistent pipe), each call launches the
 * server, sends the `initialize` handshake plus the operation as newline-delimited
 * JSON on stdin, closes stdin, and reads the replies from stdout - so a stateless
 * server sees a complete short session per call. Once the reply is in, a server
 * that does not exit by itself is stopped (SIGTERM, then SIGKILL, 2 s apart);
 * one that has not replied within `stdioTimeoutMs` (default 60 s) is stopped the
 * same way and the call throws kind `"timeout"`. Needs the default binary
 * (`os.spawn`) and a POSIX `/bin/sh`, which feeds the server its stdin.
 * @param argv {list of string} the server command line (`["python", "srv.py"]`, ...)
 * @return {Client} a configured stdio client
 */
export func connectStdio(argv as list of string) {
    return Client{
        rpc: jsonrpc.client(""),
        argv: $argv,
        isStdio: true,
        allowInsecureHttp: false,
        stdioTimeoutMs: 0
    };
}

/**
 * Perform the `initialize` handshake and return the server's result (its
 * protocol version, capabilities, and serverInfo). Per the MCP lifecycle, this
 * also sends the required `notifications/initialized` once the handshake
 * succeeds, so the session is ready for further requests. Throws a
 * `jsonrpc.RpcError` (HTTP) or an `McpError` (stdio) on any transport or
 * protocol failure.
 * @param client {Client} the MCP client
 * @return {json.Value} the server's `initialize` result
 */
export func initialize(client as Client) {
    def params as json.Value init initParams();
    if ($client.isStdio) {
        # the stdio exchange already carries the handshake (initialize + the
        # initialized notification) in one launch
        return stdioCall($client.argv, "initialize", $params, $client.stdioTimeoutMs);
    }
    def result as json.Value init httpCall($client, "initialize", $params);
    # The notification carries the negotiated headers too; jsonrpc.notify
    # checks its status (MCP answers 202).
    def rpc as jsonrpc.Client init $client.rpc;
    $rpc.headers = httpHeaders($client, "notifications/initialized");
    $rpc.allowInsecureHttp = $client.allowInsecureHttp;
    jsonrpc.notify($rpc, "notifications/initialized", json.map());
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
# json.Value), or throws an McpError (stdio) / a jsonrpc.RpcError (HTTP).
func mcpCall(client as Client, method as string, params as json.Value) {
    if ($client.isStdio) {
        return stdioCall($client.argv, $method, $params, $client.stdioTimeoutMs);
    }
    return httpCall($client, $method, $params);
}

# --- HTTP transport (private) ----------------------------------------

# The id of the one request an HTTP exchange carries.
def const HTTP_ID as int init 1;

# rpcFailure is the jsonrpc.RpcError of an HTTP failure without an error
# object, so the HTTP transport throws one error type throughout.
func rpcFailure(reason as string, message as string, status as int) {
    return jsonrpc.RpcError{
        kind: "jsonrpc",
        message: $message,
        reason: $reason,
        status: $status,
        code: 0,
        data: NULL_VALUE
    };
}

# hasHeader reports whether headers holds name (lowercase), in any case.
func hasHeader(headers as map of string to string, name as string) {
    for (def k in $headers) {
        if (strings.lower($k) == $name) {
            return true;
        }
    }
    return false;
}

# httpHeaders are the client's headers plus what the Streamable HTTP
# transport (MCP 2025-06-18) asks of a client: an Accept naming both JSON
# and an event stream, and after `initialize` the MCP-Protocol-Version.
func httpHeaders(client as Client, method as string) {
    def h as map of string to string init $client.rpc.headers;
    if (not hasHeader($h, "accept")) {
        $h["Accept"] = "application/json, text/event-stream";
    }
    if ($method != "initialize") {
        $h["MCP-Protocol-Version"] = PROTOCOL_VERSION;
    }
    return $h;
}

# httpCall posts one request and reads its response, which the server sends
# either as one JSON object or as an event stream carrying it. A failure is a
# jsonrpc.RpcError, except the generic kinds, which go on as http threw them.
func httpCall(client as Client, method as string, params as json.Value) {
    def headers as map of string to string init httpHeaders($client, $method);
    # Refuse to send credentials in the clear, before any connection.
    if (not $client.allowInsecureHttp and
        (hasHeader($headers, "authorization") or hasHeader($headers, "proxy-authorization")) and
        http.isCleartextRemote($client.rpc.endpoint)) {
        throw rpcFailure(
            "insecure",
            "mcp: credentials need an https:// endpoint (http:// only with allowInsecureHttp)",
            0);
    }
    def r as http.Response;
    try {
        $r = http.post(
            $client.rpc.endpoint,
            "application/json",
            stdioRequest($method, $params, HTTP_ID),
            $headers);
    } catch (e) {
        if ($e.kind == "net" or $e.kind == "limit" or $e.kind == "timeout" or
            $e.kind == "closed" or $e.kind == "cancelled") {
            throw $e;
        }
        throw rpcFailure("transport", "mcp: transport error: " + $e.message, 0);
    }
    def body as string init $r.body;
    def ctype as string init strings.lower(strings.trim(http.header($r, "content-type")));
    if (strings.startsWith($ctype, "text/event-stream")) {
        $body = sseResponse($r.body, $r.status);
    }
    return jsonrpc.parseReply($body, $r.status, HTTP_ID);
}

# sseResponse finds the JSON-RPC response in an event stream
# (the server closes the stream once it has sent it). Each event's `data`
# lines, joined with newlines, are one JSON-RPC message; requests and
# notifications the server sends before the response are passed over. Any
# other content is a protocol failure.
func sseResponse(body as string, status as int) {
    def text as string init strings.replace($body, "\r\n", "\n");
    $text = strings.replace($text, "\r", "\n");
    for (def event in strings.split($text, "\n\n")) {
        def data as list of string init eventData($event);
        if (len($data) == 0) {
            continue;
        }
        def message as string init strings.join($data, "\n");
        def m as json.Value;
        try {
            $m = json.decode($message);
        } catch (e) {
            if ($e.kind == "limit") {
                throw $e;
            }
            throw rpcFailure("protocol", "mcp: an event carries no JSON", $status);
        }
        if (json.typeOf($m, "") != "map") {
            throw rpcFailure("protocol", "mcp: an event carries no JSON-RPC message", $status);
        }
        if (not json.has($m, "/method")) {
            return $message;
        }
    }
    throw rpcFailure("protocol", "mcp: the event stream carries no response", $status);
}

# eventData returns the values of an event's `data:` fields, without the one
# space that may follow the colon.
func eventData(event as string) {
    def data as list of string init [];
    for (def line in strings.split($event, "\n")) {
        if (not strings.startsWith($line, "data:")) {
            continue;
        }
        def v as string init $line[5..];
        if (strings.startsWith($v, " ")) {
            $v = $v[1..];
        }
        $data[] = $v;
    }
    return $data;
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

# STDIO_TIMEOUT_MS bounds one stdio exchange, launch to reply, when the
# client sets no stdioTimeoutMs; EXIT_GRACE_MS is how long a server that has
# replied (or was sent SIGTERM) gets to exit before the next step; the stdout
# a server may write in one exchange is capped like os.run's capture.
def const STDIO_TIMEOUT_MS as int init 60000;
def const EXIT_GRACE_MS as int init 2000;
def const MAX_STDOUT_BYTES as int init 16777216;

# The shell line that runs the server (the arguments after the file) with its
# stdin read from the file: os.spawn has no stdin of its own to feed.
def const STDIN_SHIM as string init 'exec "$@" < "$0"';

# A stdio exchange's progress: how much of stdout is read, and the reply for
# the operation once one is found.
def struct StdioScan {
    consumed as int,
    found as bool,
    reply as json.Value
};

# A stopped server: its exit code, and whether it had to be killed.
def struct StdioExit {
    exitCode as int,
    killed as bool
};

# StdioLine is one stdout line read as a JSON-RPC message: `ours` when it is
# the reply the exchange waits for.
def struct StdioLine {
    ours as bool,
    reply as json.Value
};

# stdioCall runs one MCP operation over a freshly launched stdio server: it writes
# the `initialize` handshake (initialize + the initialized notification) plus the
# operation as newline-delimited JSON for the child's stdin, launches it with
# stdin read from that file and stdout written to another, and reads stdout as
# it arrives until the operation's reply is in. A server that then does not exit
# is stopped (SIGTERM, then SIGKILL); one that does not reply within timeoutMs
# is stopped and the call throws kind "timeout". `initialize` itself needs no
# extra op line, so its own reply (id 1) is returned. A launch failure,
# non-zero exit or a stdout that is not JSON-RPC throws an McpError.
func stdioCall(argv as list of string, method as string, params as json.Value, timeoutMs as int) {
    if (len($argv) == 0) {
        throw mcpError("launch", "mcp: the stdio server command line is empty", 0);
    }
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
    def limitMs as int init $timeoutMs;
    if ($limitMs <= 0) {
        $limitMs = STDIO_TIMEOUT_MS;
    }
    def dir as string init fs.makeTempDir("", "jennifer-mcp");
    defer fs.removeAll($dir);
    def inPath as string init path.join($dir, "stdin");
    def outPath as string init path.join($dir, "stdout");
    fs.writeString($inPath, strings.join($lines, "\n") + "\n");
    def cmd as list of string init ["/bin/sh", "-c", STDIN_SHIM, $inPath];
    for (def a in $argv) {
        $cmd[] = $a;
    }
    def p as os.Process;
    try {
        $p = os.spawnWith($cmd, os.SpawnOptions{stdout: $outPath, stderr: "discard"});
    } catch (e) {
        if ($e.kind == "cancelled" or $e.kind == "limit") {
            throw $e;
        }
        throw mcpError("launch", "mcp: cannot run the stdio server: " + $e.message, 0);
    }
    def scan as StdioScan;
    try {
        $scan = awaitReply($p, $outPath, $opId, $limitMs);
    } catch (e) {
        stopServer($p);
        throw $e;
    }
    def stopped as StdioExit init stopServer($p);
    if (not $stopped.killed and $stopped.exitCode != 0) {
        # The shell answers 127 / 126 when the server is not found or not
        # executable; with nothing on stdout that is a launch failure.
        if (($stopped.exitCode == 127 or $stopped.exitCode == 126) and $scan.consumed == 0 and
            fs.stat($outPath).size == 0) {
            throw mcpError("launch", "mcp: cannot run the stdio server " + $argv[0], 0);
        }
        throw mcpError(
            "exit",
            "mcp: stdio server exited with code {$stopped.exitCode}",
            $stopped.exitCode);
    }
    if (not $scan.found) {
        throw mcpError("noreply", "mcp: no reply for request id {$opId} from the stdio server", 0);
    }
    return replyResult($scan.reply);
}

# nowMs is the wall clock in milliseconds, for the exchange's deadlines.
func nowMs() {
    return time.unixMilliseconds(time.now());
}

# awaitReply reads the server's stdout file as it grows, until the reply to
# opId is in or the server has exited (its whole output read, a last line
# without a newline too). Past limitMs it throws kind "timeout"; past
# MAX_STDOUT_BYTES kind "limit". The caller stops the server either way.
func awaitReply(p as os.Process, outPath as string, opId as int, limitMs as int) {
    def scan as StdioScan init StdioScan{consumed: 0, found: false, reply: NULL_VALUE};
    def deadline as int init nowMs() + $limitMs;
    def pause as int init 2;
    while (true) {
        def exited as bool init os.poll($p);
        $scan = readLines($scan, $outPath, $opId, $exited);
        if ($scan.found or $exited) {
            return $scan;
        }
        if (nowMs() >= $deadline) {
            throw Error{
                kind: "timeout",
                message: "mcp: the stdio server did not reply within {$limitMs} ms"
            };
        }
        time.sleep(time.fromMilliseconds($pause));
        if ($pause < 50) {
            $pause = $pause * 2;
        }
    }
    return $scan;
}

# readLines reads the complete lines stdout gained since the last call (and
# the unterminated rest once the server has exited) and checks each.
func readLines(scan as StdioScan, outPath as string, opId as int, final as bool) {
    def size as int init fs.stat($outPath).size;
    if ($size > MAX_STDOUT_BYTES) {
        throw Error{kind: "limit", message: "mcp: the stdio server wrote more than 16 MiB"};
    }
    if ($size <= $scan.consumed) {
        return $scan;
    }
    def data as bytes init fs.readBytes($outPath);
    def nl as bytes init encoding.encode("utf-8", "\n");
    while (not $scan.found) {
        def end as int init binary.indexOf($data, $nl, $scan.consumed);
        def next as int init $end + 1;
        if ($end < 0) {
            if (not $final or $scan.consumed >= len($data)) {
                return $scan;
            }
            $end = len($data);
            $next = $end;
        }
        def text as string;
        try {
            $text = encoding.decode("utf-8", $data[$scan.consumed..$end]);
        } catch (e) {
            throw mcpError("protocol", "mcp: the stdio server wrote a line that is not UTF-8", 0);
        }
        $scan.consumed = $next;
        if (len(strings.trim($text)) > 0) {
            def line as StdioLine init stdioLine(strings.trim($text), $opId);
            if ($line.ours) {
                $scan.found = true;
                $scan.reply = $line.reply;
            }
        }
    }
    return $scan;
}

# stopServer ends the exchange with a server: it gets EXIT_GRACE_MS to exit by
# itself, then SIGTERM and as long again, then SIGKILL (the MCP stdio
# shutdown). It reaps and releases the handle and reports how it ended.
func stopServer(p as os.Process) {
    def killed as bool init false;
    if (not waitExit($p, EXIT_GRACE_MS)) {
        $killed = true;
        os.kill($p);
        if (not waitExit($p, EXIT_GRACE_MS)) {
            os.run(["kill", "-KILL", convert.toString($p.pid)]);
        }
    }
    def code as int init 0;
    try {
        $code = os.wait($p).exitCode;
    } catch (e) {
        # stdout and stderr go to a file and nowhere, so there is nothing to
        # decode; a server that died oddly still counts as stopped
        if ($e.kind == "cancelled") {
            throw $e;
        }
        $code = -1;
    }
    os.release($p);
    return StdioExit{exitCode: $code, killed: $killed};
}

# waitExit polls until the server has exited or ms have passed.
func waitExit(p as os.Process, ms as int) {
    def deadline as int init nowMs() + $ms;
    while (not os.poll($p)) {
        if (nowMs() >= $deadline) {
            return false;
        }
        time.sleep(time.fromMilliseconds(10));
    }
    return true;
}

# stdioLine checks one stdout line. The MCP stdio transport allows only MCP
# messages there, so a line that is not a JSON-RPC 2.0 message is a protocol
# failure. A request or notification from the server is passed over; a
# response must be well-formed and answer a request this exchange sent (the
# handshake, id 1, or the operation): the operation's reply is `ours`, and so
# is a failed handshake or an error the server could not tie to a request
# (null id), since the operation cannot have run.
func stdioLine(text as string, opId as int) {
    def m as json.Value;
    try {
        $m = json.decode($text);
    } catch (e) {
        # a reply past json's decoding caps is kind "limit"
        if ($e.kind == "limit") {
            throw $e;
        }
        throw mcpError("protocol", "mcp: the stdio server wrote a line that is not JSON", 0);
    }
    if (json.typeOf($m, "") != "map" or json.typeOf($m, "/jsonrpc") != "string" or
        json.asString($m, "/jsonrpc") != "2.0") {
        throw mcpError(
            "protocol",
            "mcp: the stdio server wrote a line that is not a JSON-RPC 2.0 message",
            0);
    }
    if (json.has($m, "/method")) {
        if (json.typeOf($m, "/method") != "string") {
            throw mcpError(
                "protocol",
                "mcp: the stdio server sent a message with a non-string method",
                0);
        }
        return StdioLine{ours: false, reply: NULL_VALUE};
    }
    def hasResult as bool init json.has($m, "/result");
    def hasError as bool init json.has($m, "/error");
    if ($hasResult == $hasError) {
        throw mcpError("protocol", "mcp: a reply carries neither or both of result and error", 0);
    }
    if ($hasError and
        (json.typeOf($m, "/error") != "map" or
        json.typeOf($m, "/error/code") != "int" or json.typeOf($m, "/error/message") != "string")) {
        throw mcpError(
            "protocol",
            "mcp: a reply's error is not an object with a code and a message",
            0);
    }
    def idType as string init json.typeOf($m, "/id");
    if ($idType == "null" and $hasError) {
        return StdioLine{ours: true, reply: $m};
    }
    if ($idType != "int") {
        throw mcpError("protocol", "mcp: a reply's id is not one this client sent", 0);
    }
    def id as int init json.asInt($m, "/id");
    if ($id == $opId or ($id == 1 and $hasError)) {
        return StdioLine{ours: true, reply: $m};
    }
    if ($id == 1) {
        return StdioLine{ours: false, reply: NULL_VALUE};
    }
    throw mcpError("protocol", "mcp: a reply's id is not one this client sent", 0);
}

# replyResult returns a checked reply's result, or throws its error object as
# an McpError with reason "rpc".
func replyResult(reply as json.Value) {
    if (json.has($reply, "/error")) {
        def code as int init json.asInt($reply, "/error/code");
        def msg as string init json.asString($reply, "/error/message");
        def data as json.Value init json.decode("null");
        if (json.has($reply, "/error/data")) {
            $data = json.get($reply, "/error/data");
        }
        throw McpError{
            kind: "mcp",
            message: "mcp error {$code}: {$msg}",
            reason: "rpc",
            code: $code,
            data: $data,
            exitCode: 0
        };
    }
    return json.get($reply, "/result");
}

# stdioReply scans a whole newline-delimited stdout for the reply to opId and
# returns its result, as the exchange does line by line: a line that is not a
# JSON-RPC message, or a malformed reply, is a protocol McpError, an error
# reply an "rpc" one, and no reply at all "noreply".
func stdioReply(stdout as string, opId as int) {
    for (def line in strings.split($stdout, "\n")) {
        def t as string init strings.trim($line);
        if (len($t) == 0) {
            continue;
        }
        def l as StdioLine init stdioLine($t, $opId);
        if ($l.ours) {
            return replyResult($l.reply);
        }
    }
    throw mcpError("noreply", "mcp: no reply for request id {$opId} from the stdio server", 0);
}
