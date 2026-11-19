# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.26.0

/**
 * JSON-RPC 2.0 (https://www.jsonrpc.org/specification) over HTTP: a **client**
 * that calls remote methods, and a transport-agnostic **server** `handle` that
 * dispatches an incoming request to the entry program's functions **by name**, via
 * `meta.callMain` - the method name arrives on the wire, so it is resolved as a
 * runtime string, not a `func` value. Built on `json` for the wire format and
 * `http` for the client transport, so it needs the default `jennifer` binary.
 *
 * `params` and a call's result are `json.Value`s: the caller builds params with
 * the `json` write API (`json.list` / `json.map` + `json.append` / `json.set`)
 * and reads the result with the `json` accessors. Any client-side failure - a
 * JSON-RPC error reply, a transport error (malformed HTTP), or a malformed
 * reply - surfaces as a single catchable `jsonrpc.RpcError` (kind
 * `"jsonrpc"`), whose `reason`, `code` and `data` say which. The generic kinds
 * pass as they are: a socket failure (refused, reset, DNS) stays `http`'s
 * `net.NetError` (kind `"net"`), a reached cap kind `"limit"`, a deadline
 * `"timeout"`, a reply cut short `"closed"`, a cancelled task `"cancelled"`.
 * A client carrying an `Authorization` header refuses an `http://` endpoint on
 * a non-loopback host before connecting, unless its `allowInsecureHttp` field
 * is set (plain HTTP on a trusted LAN only).
 *
 * **Server security.** `handle` dispatches each request's `method` to a
 * *top-level* `func` of that name in the entry program, so **every** top-level
 * function taking one `json.Value` parameter is remotely reachable - there is no
 * separate route registry. Name RPC handlers deliberately (a shared prefix, a
 * dedicated dispatch file) and do not co-locate a `handle`-served program with
 * privileged one-argument helpers. Authentication is the transport's job: gate
 * on a header / token before calling `handle`. A handler that throws yields a
 * generic `-32603` reply (the thrown message is *not* put on the wire), so raise
 * errors freely without leaking internals.
 *
 * @module jsonrpc
 * @example
 * use json;
 * import "jsonrpc.j" as jsonrpc;
 * def c as jsonrpc.Client init jsonrpc.client("https://api.example.com/rpc");
 * def args as json.Value init json.append(json.append(json.list(), "", 2), "", 3);
 * def sum as json.Value init jsonrpc.call($c, "add", $args);   # -> 5
 */
use json;
use strings;
use lists;
use meta;
import "./http.j" as http;

/**
 * A failed call or notification. Every error of kind `"jsonrpc"` is one, so its
 * fields are safe to read once the kind matches.
 * @field kind {string} always `"jsonrpc"`
 * @field message {string} `jsonrpc error <code>: <message>` for an error reply, else what went wrong
 * @field reason {string} `"rpc"` (the server answered with an error object), `"transport"` (a malformed HTTP answer), `"protocol"` (an answer that is not a JSON-RPC 2.0 response object: not JSON, a `jsonrpc` other than `"2.0"`, neither or both of result and error, a malformed error object, or another id), `"http"` (a notification answered with a non-2xx status) or `"insecure"` (an `Authorization` header refused over plain `http://`; nothing was sent)
 * @field status {int} the HTTP status of the answer; 0 for `"transport"` and `"insecure"`
 * @field code {int} the error object's `code` (such as -32601, method not found); 0 unless `reason` is `"rpc"` or the object had none
 * @field data {json.Value} the error object's `data` member; JSON `null` when it has none or `reason` is not `"rpc"`
 * @example
 * try {
 *     jsonrpc.call($c, "add", $args);
 * } catch (e) {
 *     if ($e.kind == "jsonrpc" and $e.reason == "rpc" and $e.code == -32601) {
 *         io.printf("the server has no add\n");
 *     } else {
 *         throw $e;
 *     }
 * }
 */
export def struct RpcError {
    kind as string,
    message as string,
    reason as string,
    status as int,
    code as int,
    data as json.Value
};

# rpcError is the RpcError of a failure that carries no error object.
func rpcError(reason as string, message as string, status as int) {
    return RpcError{
        kind: "jsonrpc",
        message: $message,
        reason: $reason,
        status: $status,
        code: 0,
        data: json.decode("null")
    };
}

# The protocol version string every request and response carries.
def const VERSION as string init "2.0";

# The reserved error codes (JSON-RPC 2.0 section 5.1). Application errors use the
# -32000..-32099 "server error" range or any code outside the reserved block.
# INVALID_PARAMS is part of the reserved set for handlers / callers to reference;
# the server maps a thrown handler error to INTERNAL_ERROR (it cannot tell an
# invalid-params failure from any other), so it never emits INVALID_PARAMS itself.
/** Invalid JSON was received (-32700). */
export def const PARSE_ERROR as int init -32700;
/** The JSON sent is not a valid request object (-32600). */
export def const INVALID_REQUEST as int init -32600;
/** The method does not exist or is not available (-32601). */
export def const METHOD_NOT_FOUND as int init -32601;
/** Invalid method parameters (-32602). */
export def const INVALID_PARAMS as int init -32602;
/** Internal JSON-RPC error (-32603); also what a throwing handler is answered with. */
export def const INTERNAL_ERROR as int init -32603;

# A JSON null value, reused for the `id` of an error reply that has no reliable
# request id (the write API has no scalar constructor, so decode the literal
# once at load instead of per reply).
def const NULL_VALUE as json.Value init json.decode("null");

/**
 * A client bound to a JSON-RPC HTTP endpoint. Value-semantic; build with
 * `client` / `clientWith`.
 * @field endpoint {string} the JSON-RPC endpoint URL (`http://` or `https://`)
 * @field headers {map of string to string} extra request headers (auth, ...)
 * @field allowInsecureHttp {bool} allow an `Authorization` header over plain `http://` to a non-loopback host (default false)
 */
export def struct Client {
    endpoint as string,
    headers as map of string to string,
    allowInsecureHttp as bool
};

/**
 * Build a client for a JSON-RPC endpoint.
 * @param endpoint {string} the endpoint URL
 * @return {Client} a configured client
 */
export func client(endpoint as string) {
    def h as map of string to string init {};
    return Client{endpoint: $endpoint, headers: $h, allowInsecureHttp: false};
}

/**
 * Build a client with extra request headers (e.g. `Authorization`).
 * @param endpoint {string} the endpoint URL
 * @param headers {map of string to string} headers sent with every request
 * @return {Client} a configured client
 */
export func clientWith(endpoint as string, headers as map of string to string) {
    return Client{endpoint: $endpoint, headers: $headers, allowInsecureHttp: false};
}

# --- request building (private, pure) ------------------------------

# buildEnvelope renders the shared head of a request or notification (the
# `jsonrpc` / `method` / `params` members, in that order). `params` is grafted
# verbatim - the whole positional array or named object.
func buildEnvelope(method as string, params as json.Value) {
    def req as json.Value init json.map();
    $req = json.set($req, "/jsonrpc", VERSION);
    $req = json.set($req, "/method", $method);
    $req = json.set($req, "/params", $params);
    return $req;
}

# buildRequest renders a request that expects a reply (carries an `id`).
func buildRequest(method as string, params as json.Value, id as int) {
    return json.encode(json.set(buildEnvelope($method, $params), "/id", $id));
}

# buildNotification renders a notification: no `id`, so the server sends no reply.
func buildNotification(method as string, params as json.Value) {
    return json.encode(buildEnvelope($method, $params));
}

# --- client (exported) ---------------------------------------------

# MAX_BATCH caps the requests one batch may carry: each runs a handler, so an
# unbounded batch lets one request body trigger any number of calls.
def const MAX_BATCH as int init 1000;

# The single request id the synchronous client sends and correlates on.
def const REQUEST_ID as int init 1;

/**
 * Call a remote method and return its result. Any failure - a JSON-RPC error
 * reply, a transport error (malformed HTTP), or a reply that is not a
 * JSON-RPC 2.0 response (`jsonrpc` not `"2.0"`, neither or both of `result`
 * and `error`, an error that is not an object with an int `code` and a string
 * `message`, or an id that does not match the request) - throws a
 * catchable `RpcError` (kind `"jsonrpc"`); a socket failure is a `net.NetError`
 * (kind `"net"`), a reached cap kind `"limit"`, a deadline `"timeout"`, a
 * reply cut short `"closed"`.
 * @param client {Client} the endpoint client
 * @param method {string} the method name
 * @param params {json.Value} the params (a `json.list` array or a `json.map`
 *   object; pass `json.list()` for a method that takes no arguments)
 * @return {json.Value} the `result` member of the reply
 */
export func call(client as Client, method as string, params as json.Value) {
    def resp as http.Response init httpPost($client, buildRequest($method, $params, REQUEST_ID));
    return parseResult($resp.body, $resp.status, REQUEST_ID);
}

/**
 * Send a notification: a request with no `id`, for which the server returns no
 * reply. A delivered notification says nothing about whether the method ran, but
 * a failed request still throws as `call` does: a deadline is kind
 * `"timeout"`, and a socket failure (refused, DNS) a `net.NetError` (kind
 * `"net"`), which means the request never reached the server. A non-2xx
 * answer is an `RpcError`: reason `"rpc"` when its body is a JSON-RPC error
 * object, else `"http"`.
 * @param client {Client} the endpoint client
 * @param method {string} the method name
 * @param params {json.Value} the params (see `call`)
 */
export func notify(client as Client, method as string, params as json.Value) {
    def resp as http.Response init httpPost($client, buildNotification($method, $params));
    if ($resp.status < 200 or $resp.status >= 300) {
        # A server that refuses the notification may still say why in a
        # JSON-RPC error object; anything else is reported by its status.
        def reply as json.Value;
        try {
            $reply = json.decode($resp.body);
        } catch (e) {
            if ($e.kind == "limit") {
                throw $e;
            }
            throw rpcError(
                "http",
                "jsonrpc: notification answered HTTP {$resp.status}",
                $resp.status);
        }
        if (json.typeOf($reply, "") == "map" and json.typeOf($reply, "/error") == "map" and
            errorObjectProblem($reply) == "") {
            throw errorReply($reply, $resp.status);
        }
        throw rpcError("http", "jsonrpc: notification answered HTTP {$resp.status}", $resp.status);
    }
    return;
}

# httpPost sends the request body and normalizes a transport-level failure
# (a malformed response) into the module's own RpcError (kind "jsonrpc"). The
# generic kinds go on as they are: a reached cap "limit", a deadline
# "timeout", a reply cut short "closed", a cancelled task "cancelled", and a
# socket failure (refused, reset, DNS, TLS), http's net.NetError, kind "net".
func httpPost(client as Client, body as string) {
    # Refuse to send credentials in the clear, before any connection.
    if (not $client.allowInsecureHttp and hasCredentials($client.headers) and
        http.isCleartextRemote($client.endpoint)) {
        throw rpcError(
            "insecure",
            "jsonrpc: credentials need an https:// endpoint (http:// only with allowInsecureHttp)",
            0);
    }
    try {
        return http.post($client.endpoint, "application/json", $body, $client.headers);
    } catch (e) {
        if ($e.kind == "net" or $e.kind == "limit" or $e.kind == "timeout" or
            $e.kind == "closed" or $e.kind == "cancelled") {
            throw $e;
        }
        throw rpcError("transport", "jsonrpc: transport error: " + $e.message, 0);
    }
}

# hasCredentials reports whether the headers carry an Authorization or
# Proxy-Authorization header, in any case.
func hasCredentials(headers as map of string to string) {
    for (def k in $headers) {
        def lower as string init strings.lower($k);
        if ($lower == "authorization" or $lower == "proxy-authorization") {
            return true;
        }
    }
    return false;
}

# errorObjectProblem checks a reply's error member (JSON-RPC 2.0 section 5.1):
# an object with an int `code` and a string `message` (`data` is free). It
# returns "" when it is one, or what is wrong.
func errorObjectProblem(reply as json.Value) {
    if (json.typeOf($reply, "/error") != "map") {
        return "the error member is not an object";
    }
    if (json.typeOf($reply, "/error/code") != "int") {
        return "the error object has no integer code";
    }
    if (json.typeOf($reply, "/error/message") != "string") {
        return "the error object has no string message";
    }
    return "";
}

# errorReply is the RpcError of a well-formed error object.
func errorReply(reply as json.Value, status as int) {
    def code as int init json.asInt($reply, "/error/code");
    def msg as string init json.asString($reply, "/error/message");
    def data as json.Value init json.decode("null");
    if (json.has($reply, "/error/data")) {
        $data = json.get($reply, "/error/data");
    }
    return RpcError{
        kind: "jsonrpc",
        message: "jsonrpc error {$code}: {$msg}",
        reason: "rpc",
        status: $status,
        code: $code,
        data: $data
    };
}

# parseResult pulls the result out of a reply body, or throws: an RpcError
# with reason "rpc" for a well-formed error object, "protocol" for anything
# that is not a JSON-RPC 2.0 response to this request (not JSON, not an
# object, a `jsonrpc` other than "2.0", neither or both of result and error,
# a malformed error object, or an `id` that does not echo the request's; an
# error reply may carry a null id, which the server sends when it could not
# read the request's).
func parseResult(body as string, status as int, expectId as int) {
    def reply as json.Value;
    try {
        $reply = json.decode($body);
    } catch (e) {
        # a decoding cap stays kind "limit"
        if ($e.kind == "limit") {
            throw $e;
        }
        throw rpcError("protocol", "jsonrpc: HTTP {$status}, non-JSON reply", $status);
    }
    if (json.typeOf($reply, "") != "map") {
        throw rpcError("protocol", "jsonrpc: reply is not a JSON object", $status);
    }
    if (json.typeOf($reply, "/jsonrpc") != "string" or json.asString($reply, "/jsonrpc") != VERSION) {
        throw rpcError("protocol", "jsonrpc: reply is not JSON-RPC 2.0", $status);
    }
    def hasResult as bool init json.has($reply, "/result");
    def hasError as bool init json.has($reply, "/error");
    if ($hasResult and $hasError) {
        throw rpcError("protocol", "jsonrpc: reply carries both result and error", $status);
    }
    if (not $hasResult and not $hasError) {
        throw rpcError("protocol", "jsonrpc: reply carries neither result nor error", $status);
    }
    def idType as string init json.typeOf($reply, "/id");
    def idMatches as bool init $idType == "int" and json.asInt($reply, "/id") == $expectId;
    if ($hasError) {
        if (not $idMatches and $idType != "null") {
            throw rpcError("protocol", "jsonrpc: reply id does not match the request", $status);
        }
        def problem as string init errorObjectProblem($reply);
        if ($problem != "") {
            throw rpcError("protocol", "jsonrpc: malformed error reply: " + $problem, $status);
        }
        throw errorReply($reply, $status);
    }
    if (not $idMatches) {
        throw rpcError("protocol", "jsonrpc: reply id does not match the request", $status);
    }
    return json.get($reply, "/result");
}

/**
 * Read a JSON-RPC 2.0 response body the way `call` does, for a transport
 * other than this module's own HTTP POST (such as `mcp`'s, which may carry the
 * response in an event stream).
 * @param body {string} the response JSON
 * @param status {int} the HTTP status it came with (0 for none), carried into an error
 * @param id {int} the id of the request it answers
 * @return {json.Value} the `result` member
 * @throws {RpcError} kind "jsonrpc": reason "rpc" for an error object, "protocol" for a body that is not a JSON-RPC 2.0 response to `id`
 *   (kind "limit" for one past `json`'s decoding caps)
 */
export func parseReply(body as string, status as int, id as int) {
    return parseResult($body, $status, $id);
}

# --- server (exported) ---------------------------------------------

/**
 * Dispatch a JSON-RPC request body and return the reply body (transport-
 * agnostic: wire it to `httpd` / `net` however you serve). Each request's
 * `method` names a top-level function `func NAME(params as json.Value)` in the
 * program that imported this module; it is called with the request's params and
 * must return a `json.Value` **or a scalar** (int / float / string / bool /
 * null) as its result. A missing function is a -32601 reply, a thrown error a
 * generic -32603 reply. A request that is not a JSON-RPC 2.0 request object
 * (not an object, a `jsonrpc` other than `"2.0"`, a `method` that is not a
 * string, `params` that are not an array or an object, an `id` that is not a
 * string, a number or null) is a -32600 reply, with a null id when the id
 * itself is the problem. A single request yields a single reply; a notification
 * (no `id`) yields no reply (`""`); a batch (a JSON array) yields an array reply
 * with the notification entries omitted (`""` when the batch is all
 * notifications). A batch of more than 1000 requests is refused with one -32600
 * reply.
 *
 * **Security.** Every top-level one-`json.Value`-argument function is reachable by
 * name (there is no route allow-list); the module-level doc explains how to
 * scope handlers and where authentication belongs. A thrown handler error's
 * message is *not* echoed to the client (only the generic -32603 text is), so
 * internal detail does not leak.
 * @param requestBody {string} the raw request JSON
 * @return {string} the reply JSON, or `""` when no reply is owed
 */
export func handle(requestBody as string) {
    def req as json.Value;
    try {
        $req = json.decode($requestBody);
    } catch (e) {
        return encodeError(NULL_VALUE, PARSE_ERROR, "Parse error");
    }
    if (json.typeOf($req, "") == "list") {
        return handleBatch($req);
    }
    return handleOne($req);
}

# handleOne dispatches a single request object and returns its reply (or "" for
# a notification).
func handleOne(req as json.Value) {
    if (json.typeOf($req, "") != "map") {
        return encodeError(NULL_VALUE, INVALID_REQUEST, "Invalid Request");
    }
    def hasId as bool init json.has($req, "/id");
    # An id is a string, a number or null (section 4); any other cannot be
    # echoed, so the request is invalid and answered with a null id.
    if ($hasId and not lists.contains(["string", "int", "float", "null"], json.typeOf($req, "/id"))) {
        return encodeError(NULL_VALUE, INVALID_REQUEST, "Invalid Request");
    }
    # The version must be exactly "2.0", the method a string, and params, when
    # present, a structured value (an array or an object).
    def paramsType as string init json.typeOf($req, "/params");
    if (json.typeOf($req, "/jsonrpc") != "string" or json.asString($req, "/jsonrpc") != VERSION or
        json.typeOf($req, "/method") != "string" or
        ($paramsType != "missing" and $paramsType != "list" and $paramsType != "map")) {
        return replyFor($hasId, $req, INVALID_REQUEST, "Invalid Request", true);
    }
    def method as string init json.asString($req, "/method");
    def params as json.Value init json.list();
    if (json.has($req, "/params")) {
        $params = json.get($req, "/params");
    }
    if (not meta.definedMain($method)) {
        return replyFor($hasId, $req, METHOD_NOT_FOUND, "Method not found", false);
    }
    try {
        # Invoke the handler exactly once, capturing its json.Value-or-scalar
        # return through json.set (which accepts either). A notification is fully
        # served here - it ran, and owes no reply - so return before encoding one.
        def held as json.Value init json.set(json.map(), "/v", meta.callMain($method, $params));
        if (not $hasId) {
            return "";
        }
        def resp as json.Value init json.map();
        $resp = json.set($resp, "/jsonrpc", VERSION);
        $resp = json.set($resp, "/result", json.get($held, "/v"));
        $resp = json.set($resp, "/id", idValue($req));
        return json.encode($resp);
    } catch (e) {
        # A handler error becomes a generic internal-error reply; the thrown
        # message stays server-side (never on the wire) so nothing leaks.
        return replyFor($hasId, $req, INTERNAL_ERROR, "Internal error", false);
    }
}

# handleBatch dispatches an array of requests, dropping notification (empty)
# replies and returning "" when nothing is owed.
func handleBatch(reqs as json.Value) {
    def n as int init json.length($reqs, "");
    if ($n == 0) {
        return encodeError(NULL_VALUE, INVALID_REQUEST, "Invalid Request");
    }
    if ($n > MAX_BATCH) {
        return encodeError(NULL_VALUE, INVALID_REQUEST, "Invalid Request: batch too large");
    }
    def replies as list of string init [];
    def i as int init 0;
    while ($i < $n) {
        def one as string init handleOne(json.get($reqs, "/{$i}"));
        if (len($one) > 0) {
            $replies[] = $one;
        }
        $i = $i + 1;
    }
    if (len($replies) == 0) {
        return "";
    }
    return "[" + strings.join($replies, ",") + "]";
}

# replyFor builds an error reply, or "" when this request is a notification (no
# id) and `always` is false. A malformed request has no reliable id, so its error
# is always reported (`always` true) with a JSON null id.
func replyFor(hasId as bool, req as json.Value, code as int, message as string, always as bool) {
    if (not $hasId and not $always) {
        return "";
    }
    return encodeError(idValue($req), $code, $message);
}

# idValue returns the request's `id` as a json.Value to echo back, or JSON null
# when it has none.
func idValue(req as json.Value) {
    if (json.typeOf($req, "") == "map" and json.has($req, "/id")) {
        return json.get($req, "/id");
    }
    return NULL_VALUE;
}

# encodeError renders an error reply with the given id value; `message` is
# JSON-escaped by `json.encode`, so a message is always wire-safe.
func encodeError(idValue as json.Value, code as int, message as string) {
    def resp as json.Value init json.map();
    $resp = json.set($resp, "/jsonrpc", VERSION);
    $resp = json.set($resp, "/error", json.map());
    $resp = json.set($resp, "/error/code", $code);
    $resp = json.set($resp, "/error/message", $message);
    $resp = json.set($resp, "/id", $idValue);
    return json.encode($resp);
}
