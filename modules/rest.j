# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

/**
 * An ergonomic REST layer over the `http` client and `json`. Hold a
 * value-semantic Client (base URL + default headers) and call JSON-aware verbs;
 * the module handles base-URL joining, query strings, `Content-Type`, and
 * Bearer / Basic auth headers. It is pure composition - no sockets, no TLS, no
 * parsing of its own - so all the transport lives in `http` (which uses `net`),
 * and this module needs the default `jennifer` binary. A 4xx / 5xx is a normal
 * Response (inspect `.status`), not a crash.
 * @module rest
 * @example
 * def api as rest.Client init rest.Client{baseUrl: "https://api.example.com",
 *     headers: {"Authorization": rest.bearer("my-token")}};
 * def user as json.Value init rest.getJson($api, "/users/1", {});
 */
use strings;
use convert;
use encoding;
use json;
import "./http.j" as http;

/**
 * A REST client: a base URL every path joins onto, and default headers sent
 * with every request (auth lives here). Value-semantic; thread it per call.
 * @field baseUrl {string} the base URL every path joins onto
 * @field headers {map of string to string} default headers sent with every request
 */
export def struct Client {
    baseUrl as string,
    headers as map of string to string
};

/**
 * A REST response: the status code, response headers, and the body text.
 * @field status {int} the HTTP status code
 * @field headers {map of string to string} the response headers (lowercased keys)
 * @field body {string} the response body text
 */
export def struct Response {
    status as int,
    headers as map of string to string,
    body as string
};

# --- pure helpers (private + exported) -----------------------------

# hexByte renders one byte as two uppercase hex digits.
func hexByte(b as int) {
    def digits as string init "0123456789ABCDEF";
    return strings.substring($digits, $b // 16, $b // 16 + 1) +
        strings.substring($digits, $b % 16, $b % 16 + 1);
}

# urlEncode percent-encodes a string for a URL (query) component: unreserved
# bytes (A-Z / a-z / 0-9 / - _ . ~) stay, every other byte becomes `%XX`.
func urlEncode(s as string) {
    def raw as bytes init convert.bytesFromString($s, "utf-8");
    def out as string init "";
    def i as int init 0;
    while ($i < len($raw)) {
        def b as int init $raw[$i];
        def unreserved as bool init ($b >= 65 and $b <= 90) or ($b >= 97 and $b <= 122);
        $unreserved = $unreserved or ($b >= 48 and $b <= 57);
        $unreserved = $unreserved or $b == 45 or $b == 95 or $b == 46 or $b == 126;
        if ($unreserved) {
            $out = $out + convert.fromCodepoint($b);
        } else {
            $out = $out + "%" + hexByte($b);
        }
        $i = $i + 1;
    }
    return $out;
}

# joinUrl joins a base URL and a path with exactly one slash between them.
func joinUrl(baseUrl as string, path as string) {
    def base as string init $baseUrl;
    if (strings.endsWith($base, "/")) {
        $base = strings.substring($base, 0, len($base) - 1);
    }
    if (strings.startsWith($path, "/")) {
        return $base + $path;
    }
    return $base + "/" + $path;
}

# queryString builds a "?k=v&..." query from a param map (percent-encoded), or
# "" when the map is empty.
func queryString(params as map of string to string) {
    if (len($params) == 0) {
        return "";
    }
    def out as string init "?";
    def first as bool init true;
    for (def k in $params) {
        if (not $first) {
            $out = $out + "&";
        }
        $out = $out + urlEncode($k) + "=" + urlEncode($params[$k]);
        $first = false;
    }
    return $out;
}

/**
 * Build an `Authorization` value for a Bearer token.
 * @param token {string} the bearer token
 * @return {string} the "Bearer <token>" header value
 */
export func bearer(token as string) {
    return "Bearer " + $token;
}

/**
 * Build an `Authorization` value for HTTP Basic auth (base64 of "user:password").
 * @param user {string} the username
 * @param pass {string} the password
 * @return {string} the "Basic <base64>" header value
 */
export func basic(user as string, pass as string) {
    def creds as bytes init convert.bytesFromString($user + ":" + $pass, "utf-8");
    return "Basic " + encoding.toText($creds, "base64");
}

/**
 * Return a copy of the client with one default header set.
 * @param c {Client} the client to copy
 * @param name {string} the header name
 * @param value {string} the header value
 * @return {Client} a new client with the header set
 */
export func withHeader(c as Client, name as string, value as string) {
    def nc as Client init $c;
    $nc.headers[$name] = $value;
    return $nc;
}

# --- request core (private) ----------------------------------------

# send runs one request through `http`, joining the URL and merging the client's
# default headers, and wraps the reply as a rest Response.
func send(c as Client, method as string, path as string,
    query as map of string to string, contentType as string, body as string) {
    def url as string init joinUrl($c.baseUrl, $path) + queryString($query);
    def headers as map of string to string init $c.headers;
    if (len($contentType) > 0) {
        $headers["Content-Type"] = $contentType;
    }
    def r as http.Response init http.request($method, $url, $headers, $body);
    return Response{status: $r.status, headers: $r.headers, body: $r.body};
}

# --- verbs (exported) ----------------------------------------------

/**
 * Issue a GET with an optional query map ({} for none).
 * @param c {Client} the client
 * @param path {string} the request path, joined onto the base URL
 * @param query {map of string to string} query parameters ({} for none)
 * @return {Response} the response
 */
export func get(c as Client, path as string, query as map of string to string) {
    return send($c, "GET", $path, $query, "", "");
}

/**
 * Issue a DELETE with an optional query map.
 * @param c {Client} the client
 * @param path {string} the request path, joined onto the base URL
 * @param query {map of string to string} query parameters ({} for none)
 * @return {Response} the response
 */
export func delete(c as Client, path as string, query as map of string to string) {
    return send($c, "DELETE", $path, $query, "", "");
}

/**
 * Issue a POST with a content type and body.
 * @param c {Client} the client
 * @param path {string} the request path, joined onto the base URL
 * @param contentType {string} the `Content-Type` header value
 * @param body {string} the request body
 * @return {Response} the response
 */
export func post(c as Client, path as string, contentType as string, body as string) {
    return send($c, "POST", $path, {}, $contentType, $body);
}

/**
 * Issue a PUT with a content type and body.
 * @param c {Client} the client
 * @param path {string} the request path, joined onto the base URL
 * @param contentType {string} the `Content-Type` header value
 * @param body {string} the request body
 * @return {Response} the response
 */
export func put(c as Client, path as string, contentType as string, body as string) {
    return send($c, "PUT", $path, {}, $contentType, $body);
}

/**
 * Issue a PATCH with a content type and body.
 * @param c {Client} the client
 * @param path {string} the request path, joined onto the base URL
 * @param contentType {string} the `Content-Type` header value
 * @param body {string} the request body
 * @return {Response} the response
 */
export func patch(c as Client, path as string, contentType as string, body as string) {
    return send($c, "PATCH", $path, {}, $contentType, $body);
}

# --- JSON verbs (exported) -----------------------------------------

/**
 * Issue a GET and decode the response body as JSON.
 * @param c {Client} the client
 * @param path {string} the request path, joined onto the base URL
 * @param query {map of string to string} query parameters ({} for none)
 * @return {json.Value} the decoded response body
 */
export func getJson(c as Client, path as string, query as map of string to string) {
    def r as Response init get($c, $path, $query);
    # Check the status before decoding: a 4xx / 5xx often returns an HTML error
    # page, which would otherwise throw a generic JSON-parse error and lose the
    # status and body. Surface a typed rest error carrying both.
    if ($r.status < 200 or $r.status >= 300) {
        throw Error{kind: "rest", message: "rest.getJson: HTTP " + convert.toString($r.status) + ": " + $r.body, file: "", line: 0, col: 0};
    }
    return json.decode($r.body);
}

/**
 * Issue a POST with a JSON body; returns the Response (inspect status).
 * @param c {Client} the client
 * @param path {string} the request path, joined onto the base URL
 * @param body {json.Value} the JSON request body
 * @return {Response} the response
 */
export func postJson(c as Client, path as string, body as json.Value) {
    return send($c, "POST", $path, {}, "application/json", json.encode($body));
}

/**
 * Issue a PUT with a JSON body.
 * @param c {Client} the client
 * @param path {string} the request path, joined onto the base URL
 * @param body {json.Value} the JSON request body
 * @return {Response} the response
 */
export func putJson(c as Client, path as string, body as json.Value) {
    return send($c, "PUT", $path, {}, "application/json", json.encode($body));
}

/**
 * Issue a PATCH with a JSON body.
 * @param c {Client} the client
 * @param path {string} the request path, joined onto the base URL
 * @param body {json.Value} the JSON request body
 * @return {Response} the response
 */
export func patchJson(c as Client, path as string, body as json.Value) {
    return send($c, "PATCH", $path, {}, "application/json", json.encode($body));
}
