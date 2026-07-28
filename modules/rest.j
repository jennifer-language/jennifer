# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * An ergonomic REST layer over the `http` client and `json`. Hold a
 * value-semantic Client (base URL + default headers + an `http.Options` request
 * policy) and call JSON-aware verbs; the module handles base-URL joining, query
 * strings, `Content-Type`, and Bearer / Basic auth headers. It is pure
 * composition - no sockets, no parsing of its own - so all the transport lives in
 * `http` (which uses `net`), and this module needs the default `jennifer` binary.
 * A 4xx / 5xx is a normal Response (inspect `.status`), not a crash. Build a
 * client with `rest.client`; layer on TLS (`rest.withCA` / `rest.insecure`), a
 * per-request timeout (`rest.withTimeout`), redirect-following
 * (`rest.withRedirects`), and retries (`rest.withRetries`) - each inherited from
 * `http.send`. For paginated collections, `rest.paginate` (Link header) and
 * `rest.paginateCursor` (cursor field) walk every page.
 * @module rest
 * @example
 * def api as rest.Client init rest.withHeader(rest.client("https://api.example.com"),
 *     "Authorization", rest.bearer("my-token"));
 * def user as json.Value init rest.getJson($api, "/users/1", {});
 */
use strings;
use convert;
use encoding;
use maps;
use json;
import "./http.j" as http;

/**
 * A REST client: a base URL every path joins onto, default headers sent with
 * every request (auth lives here), and the `http.Options` request policy
 * (per-request timeout, body cap, redirect-following, retry / backoff, and TLS)
 * applied to every call. Value-semantic; thread it per call.
 * @field baseUrl {string} the base URL every path joins onto
 * @field headers {map of string to string} default headers sent with every request
 * @field options {http.Options} the request policy (timeout, cap, redirects, retries, tls); zero value = defaults, full TLS verification, no redirects / retries
 */
export def struct Client {
    baseUrl as string,
    headers as map of string to string,
    options as http.Options
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

# joinUrl joins a base URL and a path with exactly one slash between them. An
# already-absolute path (a full http(s):// URL, e.g. a Link-header "next") is
# returned as-is, so pagination can feed back a server-supplied absolute URL.
func joinUrl(baseUrl as string, path as string) {
    if (strings.startsWith($path, "http://") or strings.startsWith($path, "https://")) {
        return $path;
    }
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

/**
 * Build a Client for a base URL, with no default headers and full TLS
 * verification. Layer auth / headers on with `withHeader` and TLS relaxation
 * with `withCA` / `insecure`.
 * @param baseUrl {string} the base URL every path joins onto
 * @return {Client} a new client
 */
export func client(baseUrl as string) {
    def o as http.Options; # zero value: default timeout / cap, verify TLS, no redirects / retries
    return Client{baseUrl: $baseUrl, headers: {}, options: $o};
}

/**
 * Return a copy of the client that trusts a private-CA / self-signed PEM
 * certificate (in addition to the system roots) for every `https://` request.
 * The safer alternative to `insecure`: the server is still authenticated.
 * @param c {Client} the client to copy
 * @param pem {bytes} a PEM certificate to trust
 * @return {Client} a new client pinned to the CA
 */
export func withCA(c as Client, pem as bytes) {
    def nc as Client init $c;
    $nc.options.tls.caCert = $pem;
    return $nc;
}

/**
 * Return a copy of the client that skips TLS certificate verification for every
 * `https://` request. This disables server authentication and exposes the
 * connection to a man-in-the-middle - use only for a trusted LAN endpoint you
 * cannot give a proper CA; prefer `withCA`.
 * @param c {Client} the client to copy
 * @return {Client} a new client that accepts any certificate
 */
export func insecure(c as Client) {
    def nc as Client init $c;
    $nc.options.tls.skipVerify = true;
    return $nc;
}

/**
 * Return a copy of the client with a per-request idle timeout (milliseconds; 0 =
 * the 30s default). Bounds each read, so a hung server fails rather than blocking.
 * @param c {Client} the client to copy
 * @param timeoutMs {int} the per-read idle timeout in milliseconds
 * @return {Client} a new client with the timeout set
 */
export func withTimeout(c as Client, timeoutMs as int) {
    def nc as Client init $c;
    $nc.options.timeoutMs = $timeoutMs;
    return $nc;
}

/**
 * Return a copy of the client that follows up to `maxRedirects` 3xx redirects
 * (0 = none; the 3xx is returned as-is). Inherits `http.send`'s redirect rules.
 * @param c {Client} the client to copy
 * @param maxRedirects {int} the redirect hop limit
 * @return {Client} a new client that follows redirects
 */
export func withRedirects(c as Client, maxRedirects as int) {
    def nc as Client init $c;
    $nc.options.maxRedirects = $maxRedirects;
    return $nc;
}

/**
 * Return a copy of the client that retries a 429 / 5xx up to `maxRetries` times
 * with exponential backoff (honouring a numeric `Retry-After`). Use
 * `rest.withBackoff` for a non-default base backoff.
 * @param c {Client} the client to copy
 * @param maxRetries {int} the retry limit
 * @return {Client} a new client that retries
 */
export func withRetries(c as Client, maxRetries as int) {
    def nc as Client init $c;
    $nc.options.maxRetries = $maxRetries;
    return $nc;
}

/**
 * Return a copy of the client with a base retry backoff in milliseconds (doubled
 * each attempt; 0 = the 250ms default). Pairs with `rest.withRetries`.
 * @param c {Client} the client to copy
 * @param backoffMs {int} the base backoff in milliseconds
 * @return {Client} a new client with the backoff set
 */
export func withBackoff(c as Client, backoffMs as int) {
    def nc as Client init $c;
    $nc.options.backoffMs = $backoffMs;
    return $nc;
}

# --- request core (private) ----------------------------------------

# send runs one request through `http`, joining the URL and merging the client's
# default headers, and wraps the reply as a rest Response.
func send(
    c as Client,
    method as string,
    path as string,
    query as map of string to string,
    contentType as string,
    body as string) {
    def url as string init joinUrl($c.baseUrl, $path) + queryString($query);
    def headers as map of string to string init $c.headers;
    if (len($contentType) > 0) {
        $headers["Content-Type"] = $contentType;
    }
    # Route through http.send so the client's whole policy applies: per-request
    # timeout / body cap, redirect-following, retry / backoff, and TLS. A zero
    # Options is a plain one-shot request, exactly as before.
    def r as http.Response init http.send($method, $url, $headers, $body, $c.options);
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
        throw Error{
            kind: "rest",
            message: "rest.getJson: HTTP " + convert.toString($r.status) + ": " + $r.body,
            file: "",
            line: 0,
            col: 0
        };
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

# --- pagination (exported) -----------------------------------------

# parseNextLink returns the URL of the `rel="next"` entry in an RFC 8288 Link
# header (`<url>; rel="next", <url2>; rel="prev"`), or "" if there is none.
func parseNextLink(linkHeader as string) {
    if (len($linkHeader) == 0) {
        return "";
    }
    for (def part in strings.split($linkHeader, ",")) {
        def seg as string init strings.trim($part);
        def low as string init strings.lower($seg);
        if (strings.contains($low, "rel=\"next\"") or strings.contains($low, "rel=next")) {
            def lt as int init strings.indexOf($seg, "<");
            def gt as int init strings.indexOf($seg, ">");
            if ($lt >= 0 and $gt > $lt) {
                return strings.substring($seg, $lt + 1, $gt);
            }
        }
    }
    return "";
}

# decodeOk decodes a page body as JSON, throwing a typed rest error on a non-2xx
# status (as getJson does) so a pagination walk fails loudly, not on a parse.
func decodeOk(fn as string, r as Response) {
    if ($r.status < 200 or $r.status >= 300) {
        throw Error{
            kind: "rest",
            message: $fn + ": HTTP " + convert.toString($r.status) + ": " + $r.body,
            file: "",
            line: 0,
            col: 0
        };
    }
    return json.decode($r.body);
}

/**
 * Walk a **Link-header** paginated collection (GitHub / GitLab style), following
 * each response's `Link: <url>; rel="next"` until it is absent or `maxPages` is
 * reached, and return the list of decoded page bodies. The first request uses
 * `path` + `query`; each `next` URL is followed verbatim (it carries its own
 * query). `maxPages` bounds the walk so a mis-behaving server cannot loop
 * forever.
 * @param c {Client} the client
 * @param path {string} the first page's path, joined onto the base URL
 * @param query {map of string to string} query parameters for the first page ({} for none)
 * @param maxPages {int} the maximum number of pages to fetch
 * @return {list of json.Value} the decoded body of each page, in order
 * @throws {Error} kind "rest" on a non-2xx page
 */
export func paginate(c as Client, path as string, query as map of string to string, maxPages as int) {
    def pages as list of json.Value init [];
    def nextPath as string init $path;
    def nextQuery as map of string to string init $query;
    def count as int init 0;
    while (len($nextPath) > 0 and $count < $maxPages) {
        def r as Response init get($c, $nextPath, $nextQuery);
        $pages[] = decodeOk("rest.paginate", $r);
        $count = $count + 1;
        def link as string init "";
        if (maps.has($r.headers, "link")) {
            $link = $r.headers["link"];
        }
        $nextPath = parseNextLink($link);
        $nextQuery = {}; # the next URL carries its own query
    }
    return $pages;
}

/**
 * Walk a **cursor** paginated collection: fetch a page, read the next cursor
 * from the response body at `cursorPointer` (a JSON Pointer, e.g.
 * "/meta/next_cursor"), and re-request with that cursor set as the
 * `cursorParam` query parameter, until the cursor is absent / null / empty or
 * `maxPages` is reached. Returns the list of decoded page bodies. The cursor may
 * be a JSON string or integer.
 * @param c {Client} the client
 * @param path {string} the collection path, joined onto the base URL
 * @param query {map of string to string} the initial query parameters ({} for none)
 * @param cursorPointer {string} a JSON Pointer to the next cursor in each page body
 * @param cursorParam {string} the query-parameter name to send the cursor as
 * @param maxPages {int} the maximum number of pages to fetch
 * @return {list of json.Value} the decoded body of each page, in order
 * @throws {Error} kind "rest" on a non-2xx page
 */
export func paginateCursor(
    c as Client,
    path as string,
    query as map of string to string,
    cursorPointer as string,
    cursorParam as string,
    maxPages as int) {
    def pages as list of json.Value init [];
    def q as map of string to string init $query;
    def count as int init 0;
    while ($count < $maxPages) {
        def r as Response init get($c, $path, $q);
        def doc as json.Value init decodeOk("rest.paginateCursor", $r);
        $pages[] = $doc;
        $count = $count + 1;
        if (not json.has($doc, $cursorPointer)) {
            return $pages;
        }
        def ctype as string init json.typeOf($doc, $cursorPointer);
        def cursor as string init "";
        if ($ctype == "string") {
            $cursor = json.asString($doc, $cursorPointer);
        } elseif ($ctype == "int") {
            $cursor = convert.toString(json.asInt($doc, $cursorPointer));
        } else {
            return $pages; # null or non-scalar cursor: end of the walk
        }
        if (len($cursor) == 0) {
            return $pages;
        }
        $q[$cursorParam] = $cursor;
    }
    return $pages;
}
