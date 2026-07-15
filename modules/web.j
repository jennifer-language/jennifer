# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

/**
 * An ergonomic HTTP framework over the `httpd` server engine. Register routes
 * against handler methods by name, then `web.run` owns the accept loop, matches
 * each request, and dispatches to the handler. Since Jennifer has no
 * first-class functions, dispatch is by method name via `meta.callMain` (which
 * reaches the entry program's handlers across the module boundary); a handler
 * is `func name(ctx as web.Context) { ... }`. Routing supports `:param`
 * segments, a middleware chain, and a custom not-found handler. Response and
 * request helpers hang off the `web.Context` so a handler rarely reaches for
 * `httpd` directly. Needs the default `jennifer` binary (the `httpd` engine is
 * net-backed; the constrained `jennifer-tiny` has no network stack).
 * @module web
 * @example
 * def app as web.App init web.new();
 * $app = web.get($app, "/hello/:name", "hello");
 * web.run($app, ":8080");
 */

use httpd;
use meta;
use json;
use lists;
use maps;
use strings;
use convert;
use uuid;
use encoding;
use hash;
use compress;
import "./multipart.j" as formdata;

/**
 * One registered (method, pattern, handler-name) triple. Exported only to
 * satisfy the referential-closure rule (App exposes a list of Route); programs
 * never build one directly - they call web.get / web.route.
 * @field method {string} the HTTP method (e.g. "GET")
 * @field pattern {string} the route pattern, with optional `:param` segments
 * @field handler {string} the name of the handler method to dispatch to
 */
export def struct Route {
    method as string,
    pattern as string,
    handler as string
};

/**
 * The value-semantic router state: the routes, the middleware chain (handler
 * names run before each route), and an optional not-found handler name (empty =
 * the built-in 404). Every registrar returns a fresh App.
 * @field routes {list of Route} the registered routes
 * @field middleware {list of string} handler names run before each route
 * @field notFound {string} the not-found handler name (empty = built-in 404)
 * @field cors {CorsOptions} the CORS policy applied by the serve loop (zero-value = off)
 */
export def struct App {
    routes as list of Route,
    middleware as list of string,
    notFound as string,
    cors as CorsOptions
};

/**
 * A cross-origin (CORS) policy. When set on an App via `web.cors`, the serve
 * loop adds the `Access-Control-*` headers to every response and answers a
 * preflight `OPTIONS` request with `204`. A zero-value struct (empty
 * `allowOrigin`) leaves CORS off.
 * @field allowOrigin {string} the `Access-Control-Allow-Origin` value ("*" or an origin; "" = off)
 * @field allowMethods {string} the `Access-Control-Allow-Methods` value ("" omits it)
 * @field allowHeaders {string} the `Access-Control-Allow-Headers` value ("" omits it)
 * @field allowCredentials {bool} add `Access-Control-Allow-Credentials: true`
 * @field maxAge {int} the `Access-Control-Max-Age` in seconds (0 omits it)
 */
export def struct CorsOptions {
    allowOrigin as string,
    allowMethods as string,
    allowHeaders as string,
    allowCredentials as bool,
    maxAge as int
};

/**
 * What a handler receives: the underlying request handle plus the path
 * parameters captured from the matched route (`:id` -> params["id"]).
 * @field req {httpd.Request} the underlying request handle
 * @field params {map of string to string} the captured path parameters
 */
export def struct Context {
    req as httpd.Request,
    params as map of string to string
};

# Match is the internal result of routing a request. Private: it never appears
# in an exported signature.
def struct Match {
    found as bool,
    handler as string,
    params as map of string to string
};

# --- registration ----------------------------------------------------------

/**
 * Return an empty App with no routes or middleware.
 * @return {App} a fresh, empty router
 */
export func new() {
    def routes as list of Route init [];
    def mw as list of string init [];
    def noCors as CorsOptions;
    return App{ routes: $routes, middleware: $mw, notFound: "", cors: $noCors };
}

/**
 * Register a handler for a method + pattern, returning a new App.
 * @param app {App} the router to extend
 * @param method {string} the HTTP method to match
 * @param pattern {string} the route pattern, with optional `:param` segments
 * @param handler {string} the name of the handler method
 * @return {App} a new App with the route added
 * @throws {Error} kind "web" when the handler method is not defined
 */
export func route(app as App, method as string, pattern as string, handler as string) {
    if (not meta.definedMain($handler)) {
        throw Error{ kind: "web", message: "web.route: handler not defined: " + $handler, file: "", line: 0, col: 0 };
    }
    def r as Route init Route{ method: $method, pattern: $pattern, handler: $handler };
    def out as App init $app;
    $out.routes = lists.push($out.routes, $r);
    return $out;
}

/**
 * Register a GET handler for a pattern, returning a new App.
 * @param app {App} the router to extend
 * @param pattern {string} the route pattern, with optional `:param` segments
 * @param handler {string} the name of the handler method
 * @return {App} a new App with the route added
 */
export func get(app as App, pattern as string, handler as string) {
    return route($app, "GET", $pattern, $handler);
}

/**
 * Register a POST handler for a pattern, returning a new App.
 * @param app {App} the router to extend
 * @param pattern {string} the route pattern, with optional `:param` segments
 * @param handler {string} the name of the handler method
 * @return {App} a new App with the route added
 */
export func post(app as App, pattern as string, handler as string) {
    return route($app, "POST", $pattern, $handler);
}

/**
 * Register a PUT handler for a pattern, returning a new App.
 * @param app {App} the router to extend
 * @param pattern {string} the route pattern, with optional `:param` segments
 * @param handler {string} the name of the handler method
 * @return {App} a new App with the route added
 */
export func put(app as App, pattern as string, handler as string) {
    return route($app, "PUT", $pattern, $handler);
}

/**
 * Register a PATCH handler for a pattern, returning a new App.
 * @param app {App} the router to extend
 * @param pattern {string} the route pattern, with optional `:param` segments
 * @param handler {string} the name of the handler method
 * @return {App} a new App with the route added
 */
export func patch(app as App, pattern as string, handler as string) {
    return route($app, "PATCH", $pattern, $handler);
}

/**
 * Register a DELETE handler for a pattern, returning a new App.
 * @param app {App} the router to extend
 * @param pattern {string} the route pattern, with optional `:param` segments
 * @param handler {string} the name of the handler method
 * @return {App} a new App with the route added
 */
export func delete(app as App, pattern as string, handler as string) {
    return route($app, "DELETE", $pattern, $handler);
}

/**
 * Register a middleware handler, run before each route handler. A middleware is
 * `func name(ctx as web.Context) { ...; return true; }`: return true to continue
 * to the route handler, or respond and return false to halt.
 * @param app {App} the router to extend
 * @param handler {string} the name of the middleware method
 * @return {App} a new App with the middleware appended to the chain
 * @throws {Error} kind "web" when the middleware method is not defined
 */
export func before(app as App, handler as string) {
    if (not meta.definedMain($handler)) {
        throw Error{ kind: "web", message: "web.before: middleware not defined: " + $handler, file: "", line: 0, col: 0 };
    }
    def out as App init $app;
    $out.middleware = lists.push($out.middleware, $handler);
    return $out;
}

/**
 * Set a custom handler for unmatched requests (default: a plain 404).
 * @param app {App} the router to extend
 * @param handler {string} the name of the not-found handler method
 * @return {App} a new App with the not-found handler set
 * @throws {Error} kind "web" when the handler method is not defined
 */
export func notFound(app as App, handler as string) {
    if (not meta.definedMain($handler)) {
        throw Error{ kind: "web", message: "web.notFound: handler not defined: " + $handler, file: "", line: 0, col: 0 };
    }
    def out as App init $app;
    $out.notFound = $handler;
    return $out;
}

# --- request / response helpers (on a Context) ------------------------------

/**
 * Return the request's HTTP method.
 * @param ctx {Context} the request context
 * @return {string} the HTTP method (e.g. "GET")
 */
export func method(ctx as Context) { return httpd.method($ctx.req); }

/**
 * Return the request's URL path.
 * @param ctx {Context} the request context
 * @return {string} the request path
 */
export func path(ctx as Context) { return httpd.path($ctx.req); }

/**
 * Return a query-string parameter value.
 * @param ctx {Context} the request context
 * @param name {string} the query parameter name
 * @return {string} the parameter value, or "" if absent
 */
export func query(ctx as Context, name as string) { return httpd.query($ctx.req, $name); }

/**
 * Return a request header value.
 * @param ctx {Context} the request context
 * @param name {string} the header name
 * @return {string} the header value, or "" if absent
 */
export func header(ctx as Context, name as string) { return httpd.header($ctx.req, $name); }

/**
 * Return the raw request body as bytes (binary-safe). Use `web.form` /
 * `web.bodyJson` for the common structured bodies, or `convert.stringFromBytes`
 * for text.
 * @param ctx {Context} the request context
 * @return {bytes} the request body
 */
export func body(ctx as Context) { return httpd.body($ctx.req); }

/**
 * Return the client's network address (host:port).
 * @param ctx {Context} the request context
 * @return {string} the remote address
 */
export func remoteAddr(ctx as Context) { return httpd.remoteAddr($ctx.req); }

/**
 * Decode the request body as JSON. Errors (invalid JSON) propagate - catch them
 * in the handler or let the framework's 500 net answer.
 * @param ctx {Context} the request context
 * @return {json.Value} the decoded request body
 */
export func bodyJson(ctx as Context) {
    return json.decode(convert.stringFromBytes(httpd.body($ctx.req), "utf-8"));
}

# hexNibble decodes an ASCII hex digit (0-9 / A-F / a-f) to its value, or -1.
func hexNibble(b as int) {
    if ($b >= 48 and $b <= 57) {
        return $b - 48;
    }
    if ($b >= 65 and $b <= 70) {
        return $b - 55;
    }
    if ($b >= 97 and $b <= 102) {
        return $b - 87;
    }
    return -1;
}

# percentDecode decodes a URL-encoded component: `%XX` -> byte, `+` -> space.
func percentDecode(s as string) {
    def raw as bytes init convert.bytesFromString($s, "utf-8");
    def out as bytes;
    def i as int init 0;
    while ($i < len($raw)) {
        def b as int init $raw[$i];
        if ($b == 43) {
            $out[] = 32;
            $i = $i + 1;
        } elseif ($b == 37 and $i + 2 < len($raw)) {
            def hi as int init hexNibble($raw[$i + 1]);
            def lo as int init hexNibble($raw[$i + 2]);
            if ($hi >= 0 and $lo >= 0) {
                $out[] = $hi * 16 + $lo;
                $i = $i + 3;
            } else {
                $out[] = $b;
                $i = $i + 1;
            }
        } else {
            $out[] = $b;
            $i = $i + 1;
        }
    }
    return convert.stringFromBytes($out, "utf-8");
}

# parseForm parses an `application/x-www-form-urlencoded` body into a map.
func parseForm(bodytext as string) {
    def out as map of string to string init {};
    if (len($bodytext) == 0) {
        return $out;
    }
    for (def pair in strings.split($bodytext, "&")) {
        def eq as int init strings.indexOf($pair, "=");
        if ($eq >= 0) {
            $out[percentDecode(strings.substring($pair, 0, $eq))] =
                percentDecode(strings.substring($pair, $eq + 1, len($pair)));
        } elseif (len($pair) > 0) {
            $out[percentDecode($pair)] = "";
        }
    }
    return $out;
}

/**
 * Parse an `application/x-www-form-urlencoded` request body into a map of
 * decoded field names to values.
 * @param ctx {Context} the request context
 * @return {map of string to string} the form fields
 */
export func form(ctx as Context) {
    return parseForm(convert.stringFromBytes(httpd.body($ctx.req), "utf-8"));
}

/**
 * Return one form field's value, or "" when absent.
 * @param ctx {Context} the request context
 * @param name {string} the field name
 * @return {string} the field value, or "" when absent
 */
export func formValue(ctx as Context, name as string) {
    def fields as map of string to string init form($ctx);
    if (maps.has($fields, $name)) {
        return $fields[$name];
    }
    return "";
}

/**
 * Parse a `multipart/form-data` request body (a file-upload form) into its
 * parts, using the request's Content-Type header (with the boundary) and the
 * raw body. The completion of the body-parser family alongside `web.form`
 * (urlencoded) and `web.bodyJson` (JSON). Each `multipart.Part` is a field or a
 * file (`multipart.isFile` / `multipart.text` distinguish them); the handler
 * imports `multipart.j` to name the type. A missing boundary or malformed part
 * throws `Error{kind: "multipart"}`.
 * @param ctx {Context} the request context
 * @return {list of multipart.Part} the form parts (fields and files)
 */
export func multipartForm(ctx as Context) {
    return formdata.parse(httpd.header($ctx.req, "Content-Type"), httpd.body($ctx.req));
}

/**
 * Return a captured path parameter, or "" if the route had none by that name.
 * @param ctx {Context} the request context
 * @param name {string} the path parameter name
 * @return {string} the captured value, or "" if absent
 */
export func param(ctx as Context, name as string) {
    if (maps.has($ctx.params, $name)) {
        return $ctx.params[$name];
    }
    return "";
}

/**
 * Set a response header.
 * @param ctx {Context} the request context
 * @param name {string} the header name
 * @param value {string} the header value
 */
export func setHeader(ctx as Context, name as string, value as string) {
    httpd.setHeader($ctx.req, $name, $value);
}

/**
 * Answer the request with a status code and body.
 * @param ctx {Context} the request context
 * @param status {int} the HTTP status code
 * @param body {string} the response body
 */
export func respond(ctx as Context, status as int, body as string) {
    httpd.respond($ctx.req, $status, $body);
}

/**
 * Answer with a text/plain body.
 * @param ctx {Context} the request context
 * @param status {int} the HTTP status code
 * @param body {string} the response body
 */
export func text(ctx as Context, status as int, body as string) {
    httpd.setHeader($ctx.req, "Content-Type", "text/plain; charset=utf-8");
    httpd.respond($ctx.req, $status, $body);
}

/**
 * Answer with an application/json body encoded from a json.Value. (Named
 * sendJson, not json, because a method may not shadow the `json` namespace this
 * module imports.)
 * @param ctx {Context} the request context
 * @param status {int} the HTTP status code
 * @param doc {json.Value} the JSON document to encode
 */
export func sendJson(ctx as Context, status as int, doc as json.Value) {
    httpd.setHeader($ctx.req, "Content-Type", "application/json");
    httpd.respond($ctx.req, $status, json.encode($doc));
}

/**
 * Answer with a text/html body.
 * @param ctx {Context} the request context
 * @param status {int} the HTTP status code
 * @param body {string} the HTML body
 */
export func html(ctx as Context, status as int, body as string) {
    httpd.setHeader($ctx.req, "Content-Type", "text/html; charset=utf-8");
    httpd.respond($ctx.req, $status, $body);
}

/**
 * Redirect to `location` with the given status (301 / 302 / 303 / 307 / 308).
 * @param ctx {Context} the request context
 * @param status {int} the redirect status code
 * @param location {string} the target URL for the Location header
 */
export func redirect(ctx as Context, status as int, location as string) {
    httpd.setHeader($ctx.req, "Location", $location);
    httpd.respond($ctx.req, $status, "");
    return null;
}

/**
 * Answer with a file from disk.
 * @param ctx {Context} the request context
 * @param path {string} the filesystem path to serve
 */
export func serveFile(ctx as Context, path as string) {
    httpd.serveFile($ctx.req, $path);
}

/**
 * Serve static files from a directory root (path-safe; 404 for a missing file).
 * @param ctx {Context} the request context
 * @param root {string} the directory to serve from
 */
export func serveDir(ctx as Context, root as string) {
    httpd.serveDir($ctx.req, $root);
    return null;
}

# acceptsGzip reports whether an Accept-Encoding header names gzip. A simple
# substring test (the `q=0` disable form is not parsed).
func acceptsGzip(accept as string) {
    return strings.contains($accept, "gzip");
}

/**
 * Answer with a body, gzip-compressed when the client accepts it. Sets `Vary:
 * Accept-Encoding` always; if the request's `Accept-Encoding` names gzip, the
 * body is compressed and sent with `Content-Encoding: gzip`, otherwise it is
 * sent as-is. Set the `Content-Type` yourself (via `web.setHeader`) before
 * calling. Worth it for large text / JSON / HTML; skip it for already-compressed
 * payloads (images, archives).
 * @param ctx {Context} the request context
 * @param status {int} the HTTP status code
 * @param body {string} the response body
 */
export func sendGzip(ctx as Context, status as int, body as string) {
    httpd.setHeader($ctx.req, "Vary", "Accept-Encoding");
    if (acceptsGzip(httpd.header($ctx.req, "Accept-Encoding"))) {
        httpd.setHeader($ctx.req, "Content-Encoding", "gzip");
        httpd.respond($ctx.req, $status, compress.pack(convert.bytesFromString($body, "utf-8"), "gzip"));
        return null;
    }
    httpd.respond($ctx.req, $status, $body);
    return null;
}

# --- cookies ----------------------------------------------------------------

/**
 * Attributes for a Set-Cookie header. A zero-value struct is a session cookie
 * (no attributes); set the fields you need.
 * @field path {string} the Path attribute ("" omits it)
 * @field domain {string} the Domain attribute ("" omits it)
 * @field maxAge {int} the Max-Age in seconds; 0 omits it, a negative value expires the cookie now
 * @field httpOnly {bool} add HttpOnly (hide the cookie from JavaScript)
 * @field secure {bool} add Secure (send only over HTTPS)
 * @field sameSite {string} the SameSite attribute: "Lax", "Strict", "None", or "" to omit
 */
export def struct CookieOptions {
    path as string,
    domain as string,
    maxAge as int,
    httpOnly as bool,
    secure as bool,
    sameSite as string
};

# parseCookie finds `name` in a raw `Cookie` header ("k1=v1; k2=v2"), returning
# its value or "" when absent.
func parseCookie(header as string, name as string) {
    def parts as list of string init strings.split($header, ";");
    for (def p in $parts) {
        def kv as string init strings.trim($p);
        def eq as int init strings.indexOf($kv, "=");
        if ($eq > 0) {
            if (strings.substring($kv, 0, $eq) == $name) {
                return strings.substring($kv, $eq + 1, len($kv));
            }
        }
    }
    return "";
}

# formatSetCookie renders a Set-Cookie header value from name, value, and options.
func formatSetCookie(name as string, value as string, opts as CookieOptions) {
    def out as string init $name + "=" + $value;
    if (len($opts.path) > 0) {
        $out = $out + "; Path=" + $opts.path;
    }
    if (len($opts.domain) > 0) {
        $out = $out + "; Domain=" + $opts.domain;
    }
    if (not ($opts.maxAge == 0)) {
        $out = $out + "; Max-Age=" + convert.toString($opts.maxAge);
    }
    if ($opts.httpOnly) {
        $out = $out + "; HttpOnly";
    }
    if ($opts.secure) {
        $out = $out + "; Secure";
    }
    if (len($opts.sameSite) > 0) {
        $out = $out + "; SameSite=" + $opts.sameSite;
    }
    return $out;
}

/**
 * Return the value of the request cookie named `name`, or "" if absent.
 * @param ctx {Context} the request context
 * @param name {string} the cookie name
 * @return {string} the cookie value, or "" when absent
 */
export func cookie(ctx as Context, name as string) {
    return parseCookie(httpd.header($ctx.req, "Cookie"), $name);
}

/**
 * Set a response cookie with the given attributes (a `Set-Cookie` header).
 * @param ctx {Context} the request context
 * @param name {string} the cookie name
 * @param value {string} the cookie value
 * @param opts {CookieOptions} the cookie attributes (zero-value = a session cookie)
 */
export func setCookie(ctx as Context, name as string, value as string, opts as CookieOptions) {
    httpd.setHeader($ctx.req, "Set-Cookie", formatSetCookie($name, $value, $opts));
    return null;
}

# --- sessions ---------------------------------------------------------------

/**
 * Return the request's session id, minting a new one on first use. If the
 * `cookieName` cookie is present its value is returned; otherwise a fresh UUID
 * is generated and set as a `HttpOnly`, `SameSite=Lax`, path-`/` cookie. `web`
 * manages only the id cookie - the session data itself lives in a store the app
 * owns (e.g. the `session` module over `memcache`), so `web` forces no store or
 * network dependency. Call once per request and capture the returned id.
 * @param ctx {Context} the request context
 * @param cookieName {string} the session-id cookie name (e.g. "sid")
 * @return {string} the session id (existing or newly minted)
 */
export func sessionId(ctx as Context, cookieName as string) {
    def id as string init cookie($ctx, $cookieName);
    if (len($id) > 0) {
        return $id;
    }
    $id = uuid.generate("v4");
    def opts as CookieOptions;
    $opts.path = "/";
    $opts.httpOnly = true;
    $opts.sameSite = "Lax";
    setCookie($ctx, $cookieName, $id, $opts);
    return $id;
}

# --- routing internals ------------------------------------------------------

# splitPath breaks a URL path into non-empty segments ("/a/b/" -> ["a", "b"]).
func splitPath(p as string) {
    def raw as list of string init strings.split($p, "/");
    def out as list of string init [];
    for (def seg in $raw) {
        if (not ($seg == "")) {
            $out[] = $seg;
        }
    }
    return $out;
}

# matchMiss is the "no match" Match value.
func matchMiss() {
    def empty as map of string to string init {};
    return Match{ found: false, handler: "", params: $empty };
}

# matchPattern tries one route against the request's path segments. A `:name`
# segment captures one segment; a trailing `*name` segment is a wildcard that
# captures all remaining segments (joined by "/", possibly empty), so
# `/files/*path` matches `/files/a/b` with path = "a/b" and `/*path` matches any
# path. The wildcard is only special as the last pattern segment.
func matchPattern(r as Route, segs as list of string) {
    def pat as list of string init splitPath($r.pattern);
    def wildKey as string init "";
    def fixed as int init len($pat);
    if (len($pat) > 0 and strings.startsWith($pat[len($pat) - 1], "*")) {
        def last as string init $pat[len($pat) - 1];
        $wildKey = strings.substring($last, 1, len($last));
        $fixed = len($pat) - 1;
    }
    if (len($wildKey) > 0) {
        if (len($segs) < $fixed) {
            return matchMiss();
        }
    } elseif (not (len($pat) == len($segs))) {
        return matchMiss();
    }
    def params as map of string to string init {};
    def idx as int init 0;
    while ($idx < $fixed) {
        def ps as string init $pat[$idx];
        if (strings.startsWith($ps, ":")) {
            $params[strings.substring($ps, 1, len($ps))] = $segs[$idx];
        } elseif (not ($ps == $segs[$idx])) {
            return matchMiss();
        }
        $idx = $idx + 1;
    }
    if (len($wildKey) > 0) {
        def rest as string init "";
        def j as int init $fixed;
        while ($j < len($segs)) {
            if (len($rest) > 0) {
                $rest = $rest + "/";
            }
            $rest = $rest + $segs[$j];
            $j = $j + 1;
        }
        $params[$wildKey] = $rest;
    }
    return Match{ found: true, handler: $r.handler, params: $params };
}

# matchRoute finds the first route matching method + path, capturing any `:param`
# and trailing `*name` wildcard segments. Returns a Match with found=false when
# nothing matches.
func matchRoute(app as App, method as string, path as string) {
    def segs as list of string init splitPath($path);
    for (def r in $app.routes) {
        if ($r.method == $method) {
            def m as Match init matchPattern($r, $segs);
            if ($m.found) {
                return $m;
            }
        }
    }
    return matchMiss();
}

# runMiddleware runs the chain; returns false as soon as one middleware halts.
func runMiddleware(app as App, ctx as Context) {
    for (def mw in $app.middleware) {
        def keep as bool init meta.callMain($mw, $ctx);
        if (not $keep) {
            return false;
        }
    }
    return true;
}

# ensureAnswered guarantees a request is answered exactly once: httpd.respond
# errors on an already-answered request, which we swallow, so a handler that
# forgot to respond still gets a response instead of hanging the connection.
func ensureAnswered(req as httpd.Request, status as int, body as string) {
    try {
        httpd.respond($req, $status, $body);
    } catch (err) {
        # already answered - nothing to do.
    }
}

# dispatch runs the middleware chain then the handler, guaranteeing a response.
func dispatch(app as App, handler as string, ctx as Context, req as httpd.Request) {
    def cont as bool init runMiddleware($app, $ctx);
    if ($cont) {
        try {
            meta.callMain($handler, $ctx);
        } catch (err) {
            # handler failed - the safety net below turns it into a 500.
        }
    }
    ensureAnswered($req, 500, "500 internal server error\n");
}

# --- authentication ---------------------------------------------------------

/**
 * Parsed HTTP Basic credentials from the request. `present` is false when the
 * request carried no valid `Authorization: Basic` header.
 * @field user {string} the username
 * @field password {string} the password
 * @field present {bool} true when valid Basic credentials were supplied
 */
export def struct BasicCredentials {
    user as string,
    password as string,
    present as bool
};

# parseBasicAuth decodes an `Authorization: Basic base64(user:pass)` header value.
func parseBasicAuth(header as string) {
    def absent as BasicCredentials init BasicCredentials{ user: "", password: "", present: false };
    def sp as int init strings.indexOf($header, " ");
    if ($sp < 0) {
        return $absent;
    }
    if (not (strings.lower(strings.substring($header, 0, $sp)) == "basic")) {
        return $absent;
    }
    def raw as string init strings.trim(strings.substring($header, $sp + 1, len($header)));
    def decoded as string init "";
    try {
        $decoded = convert.stringFromBytes(encoding.fromText($raw, "base64"), "utf-8");
    } catch (err) {
        return $absent;
    }
    def colon as int init strings.indexOf($decoded, ":");
    if ($colon < 0) {
        return $absent;
    }
    return BasicCredentials{ user: strings.substring($decoded, 0, $colon), password: strings.substring($decoded, $colon + 1, len($decoded)), present: true };
}

# parseBearer extracts the token from an `Authorization: Bearer <token>` header
# value, or "" when the scheme is not Bearer.
func parseBearer(header as string) {
    def sp as int init strings.indexOf($header, " ");
    if ($sp < 0) {
        return "";
    }
    if (not (strings.lower(strings.substring($header, 0, $sp)) == "bearer")) {
        return "";
    }
    return strings.trim(strings.substring($header, $sp + 1, len($header)));
}

/**
 * Parse the request's HTTP Basic credentials. Checking them (against a user
 * store) and sending a `401` challenge are the app's - web only decodes the
 * header.
 * @param ctx {Context} the request context
 * @return {BasicCredentials} the decoded credentials (`present` false if absent / invalid)
 */
export func basicAuth(ctx as Context) {
    return parseBasicAuth(httpd.header($ctx.req, "Authorization"));
}

/**
 * Return the request's bearer token (the `<token>` in `Authorization: Bearer
 * <token>`), or "" when absent. Validate it yourself (an opaque lookup, or
 * `jwt.verify` once the `jwt` module lands).
 * @param ctx {Context} the request context
 * @return {string} the bearer token, or "" when absent
 */
export func bearerToken(ctx as Context) {
    return parseBearer(httpd.header($ctx.req, "Authorization"));
}

# --- CSRF -------------------------------------------------------------------
#
# Stateless, HMAC-signed double-submit tokens. `web` holds no secret or session
# state: the app supplies the secret (a stable per-deployment string) and opts in
# with a middleware. A token is `<random>.<hmac(secret, random)>`; it is minted
# into a cookie and echoed in the form (or the `X-CSRF-Token` header), and a
# request is accepted only when the submitted token equals the cookie and its
# signature verifies - so a forger without the secret cannot mint a valid one.

# csrfSign returns the hex HMAC-SHA256 of `rand` under `secret`.
func csrfSign(secret as string, rand as string) {
    def mac as bytes init hash.hmac(convert.bytesFromString($secret, "utf-8"), convert.bytesFromString($rand, "utf-8"), "sha256");
    return encoding.toText($mac, "hex");
}

# csrfValid reports whether a "<rand>.<sig>" token's signature verifies.
func csrfValid(secret as string, token as string) {
    def dot as int init strings.indexOf($token, ".");
    if ($dot < 0) {
        return false;
    }
    def rand as string init strings.substring($token, 0, $dot);
    def sig as string init strings.substring($token, $dot + 1, len($token));
    return $sig == csrfSign($secret, $rand);
}

/**
 * Mint a CSRF token, set it in the `csrf` cookie, and return it for embedding in
 * a form (a hidden `csrf` field) or handing to the client for an `X-CSRF-Token`
 * header. Call from the GET handler that renders the form.
 * @param ctx {Context} the request context
 * @param secret {string} the app's CSRF secret (stable per deployment)
 * @return {string} the token to embed in the form / send as a header
 */
export func csrfToken(ctx as Context, secret as string) {
    def rand as string init uuid.generate("v4");
    def token as string init $rand + "." + csrfSign($secret, $rand);
    def opts as CookieOptions;
    $opts.path = "/";
    $opts.httpOnly = true;
    $opts.sameSite = "Lax";
    setCookie($ctx, "csrf", $token, $opts);
    return $token;
}

/**
 * Validate the request's CSRF token: the submitted token (the `X-CSRF-Token`
 * header, else the `csrf` form field) must equal the `csrf` cookie and its
 * signature must verify. Guard unsafe methods (POST / PUT / PATCH / DELETE) with
 * a `web.before` middleware that calls this and rejects on false.
 * @param ctx {Context} the request context
 * @param secret {string} the app's CSRF secret
 * @return {bool} true when the request carries a valid token
 */
export func csrfCheck(ctx as Context, secret as string) {
    def submitted as string init header($ctx, "X-CSRF-Token");
    if ($submitted == "") {
        $submitted = formValue($ctx, "csrf");
    }
    if ($submitted == "") {
        return false;
    }
    if (not ($submitted == cookie($ctx, "csrf"))) {
        return false;
    }
    return csrfValid($secret, $submitted);
}

# --- caching ----------------------------------------------------------------

# etagMatches reports whether an If-None-Match header value matches the tag (in
# either its quoted or bare form) or the wildcard "*". A simple exact match: the
# RFC 7232 comma-list and weak-tag (W/) forms are not parsed.
func etagMatches(inm as string, quoted as string, tag as string) {
    return $inm == "*" or $inm == $quoted or $inm == $tag;
}

/**
 * Set an `ETag` and honour a conditional GET. Sets the `ETag` response header to
 * `tag` (quoted) and, if the request's `If-None-Match` matches, answers
 * `304 Not Modified` and returns true - the handler should then stop. Returns
 * false when the client has no matching cached copy, so the handler sends the
 * full body. `tag` is the app's choice of validator: a content hash (via
 * `hash`), a row version, an mtime - so `web` needs no hashing of its own.
 * @param ctx {Context} the request context
 * @param tag {string} the entity tag identifying this version of the response
 * @return {bool} true if a 304 was sent (stop), false to send the full response
 */
export func etag(ctx as Context, tag as string) {
    def quoted as string init "\"" + $tag + "\"";
    httpd.setHeader($ctx.req, "ETag", $quoted);
    if (etagMatches(httpd.header($ctx.req, "If-None-Match"), $quoted, $tag)) {
        httpd.respond($ctx.req, 304, "");
        return true;
    }
    return false;
}

# --- CORS -------------------------------------------------------------------

/**
 * Set the CORS policy for the whole app, returning a new App. When set (a
 * non-empty `opts.allowOrigin`), the serve loop adds the `Access-Control-*`
 * headers to every response and answers a preflight `OPTIONS` request with a
 * `204` before routing.
 * @param app {App} the router to configure
 * @param opts {CorsOptions} the CORS policy
 * @return {App} a new App with the policy set
 */
export func cors(app as App, opts as CorsOptions) {
    def out as App init $app;
    $out.cors = $opts;
    return $out;
}

# corsEnabled reports whether a CORS policy is set (a non-empty allow-origin).
func corsEnabled(app as App) {
    return not ($app.cors.allowOrigin == "");
}

# applyCors adds the configured Access-Control-* response headers.
func applyCors(ctx as Context, opts as CorsOptions) {
    httpd.setHeader($ctx.req, "Access-Control-Allow-Origin", $opts.allowOrigin);
    if (len($opts.allowMethods) > 0) {
        httpd.setHeader($ctx.req, "Access-Control-Allow-Methods", $opts.allowMethods);
    }
    if (len($opts.allowHeaders) > 0) {
        httpd.setHeader($ctx.req, "Access-Control-Allow-Headers", $opts.allowHeaders);
    }
    if ($opts.allowCredentials) {
        httpd.setHeader($ctx.req, "Access-Control-Allow-Credentials", "true");
    }
    if ($opts.maxAge > 0) {
        httpd.setHeader($ctx.req, "Access-Control-Max-Age", convert.toString($opts.maxAge));
    }
    return null;
}

# --- serving ----------------------------------------------------------------

/**
 * Serve on an already-listening httpd.Server, dispatching each request to its
 * matched handler. Blocks until the server is shut down (httpd.accept then
 * errors and the loop exits). Use this when you want to hold the server handle
 * yourself - e.g. to shut it down from another task, or to serve from a
 * `spawn`. `web.run` is the listen-and-serve convenience over it.
 * @param app {App} the router
 * @param srv {httpd.Server} the already-listening server handle
 */
export func serveOn(app as App, srv as httpd.Server) {
    def running as bool init true;
    while ($running) {
        try {
            def req as httpd.Request init httpd.accept($srv);
            def m as string init httpd.method($req);
            def p as string init httpd.path($req);
            def matched as Match init matchRoute($app, $m, $p);
            def ctx as Context init Context{ req: $req, params: $matched.params };
            if (corsEnabled($app)) {
                applyCors($ctx, $app.cors);
            }
            if (corsEnabled($app) and $m == "OPTIONS") {
                # Preflight: the CORS headers are set; answer without routing.
                httpd.respond($req, 204, "");
            } elseif ($matched.found) {
                dispatch($app, $matched.handler, $ctx, $req);
            } elseif (not ($app.notFound == "")) {
                dispatch($app, $app.notFound, $ctx, $req);
            } else {
                httpd.respond($req, 404, "404 not found\n");
            }
        } catch (err) {
            # httpd.accept errors once the server is shut down; end the loop.
            $running = false;
        }
    }
}

/**
 * Listen on addr and serve forever, dispatching each request to its matched
 * handler. Blocks; interrupt to stop.
 * @param app {App} the router
 * @param addr {string} the listen address (e.g. ":8080")
 */
export func run(app as App, addr as string) {
    def srv as httpd.Server init httpd.listen($addr);
    serveOn($app, $srv);
}
