# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.25.0
# pragma-jennifer-capability: net

/**
 * A JSON-API conventions layer over the bundled `web` module. `web` is the HTTP
 * framework - routing, `:param` captures, middleware, cookies, sessions, CORS,
 * ETag; `webapi` adds the pieces every JSON API re-implements by hand on top of
 * it: a uniform error envelope, request validation (reusing `validate`),
 * versioned route mounting, pluggable bearer auth and rate limiting (the app
 * supplies a verifier and a counter as `func` values, so this module depends on
 * no `jwt` / store), content negotiation, and pagination.
 *
 * It is a value-semantic builder finished once, before the server runs:
 * `webapi.new()` -> add mounts, an authenticator, routes with a `Spec` ->
 * `webapi.install($api, $app, apiGuard)`. Routes and the authenticator / limiter
 * are `func` values; the one small guard shim in the entry program exists only to
 * bind the built `Api` into `web`'s middleware chain (Jennifer has no closures
 * yet, so the shim captures `$api` through a top-level binding):
 *
 *     func apiGuard(ctx as web.Context) { return webapi.guard($api, $ctx); }
 *
 * The `Spec`-evaluation core (`webapi.evaluate`) is a pure function of
 * `(Spec, Identity, data)`, testable without a server. Needs the default
 * `jennifer` binary (`web` needs `net`); unavailable under `jennifer-tiny`.
 * @module webapi
 * @example
 * import "web.j" as web;
 * import "webapi.j" as webapi;
 * def app as web.App init web.new();
 * def api as webapi.Api init webapi.new();
 * $api = webapi.mount($api, 1, "/v1");
 * $api = webapi.get($api, "/deck", getDeck, webapi.public());
 * func apiGuard(ctx as web.Context) { return webapi.guard($api, $ctx); }
 * $app = webapi.install($api, $app, apiGuard);
 * web.run($app, ":8080");
 */
use strings;
use lists;
use maps;
use convert;
use json;
use regex;
import "./web.j" as web;
import "./validate.j" as validate;
import "./ratelimit.j" as ratelimit;
import "./ipnet.j" as ipnet;

# --- types ------------------------------------------------------------------

/**
 * How a route authenticates. `None` is public (no credential); `Bearer` requires
 * an `Authorization: Bearer <token>` the authenticator accepts.
 */
export def enum Auth {
    None,
    Bearer
};

/**
 * What a route produces, driving content negotiation. `Json` always answers
 * JSON, `Html` always HTML, `Negotiate` picks from the request's `Accept`.
 */
export def enum Produces {
    Json,
    Html,
    Negotiate
};

/**
 * The metadata attached to a route: what it needs and what it makes. A zero
 * `Spec` (from `webapi.public()`) is an unauthenticated JSON route with no rules.
 * @field summary {string} a one-line human description (used in discovery)
 * @field auth {Auth} the authentication requirement
 * @field scopes {list of string} the permissions the identity must hold
 * @field rules {map of string to list of validate.Rule} per-field request validation
 * @field rateLimit {int} allowed requests per identity per window (0 = unlimited)
 * @field produces {Produces} the response content type policy
 */
export def struct Spec {
    summary as string,
    auth as Auth,
    scopes as list of string,
    rules as map of string to list of validate.Rule,
    rateLimit as int,
    produces as Produces
};

/**
 * The result of authenticating a request. `ok` is false when the credential was
 * absent or rejected; the other fields describe the caller when it is true.
 * @field ok {bool} whether the credential was accepted
 * @field subject {string} a stable identifier for the caller
 * @field display {string} a human-readable label
 * @field scopes {list of string} the permissions this caller holds
 */
export def struct Identity {
    ok as bool,
    subject as string,
    display as string,
    scopes as list of string
};

/**
 * A page window parsed from a request's `offset` / `limit` query parameters,
 * clamped by `webapi.page`.
 * @field offset {int} the zero-based start index
 * @field limit {int} the page size
 */
export def struct Page {
    offset as int,
    limit as int
};

/**
 * One registered route. Exported only because `Api.routes` exposes a list of
 * them; programs build these through `webapi.get` / `post` / ..., never directly.
 * @field method {string} the HTTP method
 * @field pattern {string} the un-prefixed route pattern
 * @field handler {func} the entry-program handler func value
 * @field feature {string} the discovery feature label (defaults to "METHOD /pattern")
 * @field spec {Spec} the route's specification
 */
export def struct RouteDef {
    method as string,
    pattern as string,
    handler as func,
    feature as string,
    spec as Spec
};

/**
 * One version mount point. Exported only because `Api.mounts` exposes a list of
 * them; programs build these through `webapi.mount` / `alias`, never directly.
 * @field version {int} the version label
 * @field path {string} the base path it serves under ("" = root)
 * @field sunset {string} the deprecation date ("" = not deprecated)
 */
export def struct Mount {
    version as int,
    path as string,
    sunset as string
};

/**
 * The built API description: its routes, mount points, and the authenticator /
 * limiter `func` values. Value-semantic; every builder returns a fresh `Api`. The
 * authenticator / limiter are entry-program `func` values, called through their
 * home interpreter so they resolve their own imports and can construct a
 * `webapi.Identity` across the module boundary.
 * @field routes {list of RouteDef} the registered routes
 * @field mounts {list of Mount} the version mount points
 * @field authenticator {func} `func(token as string) -> Identity` (only when hasAuthenticator)
 * @field hasAuthenticator {bool} whether an authenticator is set (else every route is public)
 * @field limiter {func} `func(key as string, limit as int) -> ratelimit.Result` (only when hasLimiter)
 * @field hasLimiter {bool} whether a rate limiter is set (else no limiting)
 * @field limitKey {func} `func(ctx as web.Context) -> string`, the key of an unauthenticated request (only when hasLimitKey)
 * @field hasLimitKey {bool} whether a key function is set (else the client host, IPv6 by /64)
 */
export def struct Api {
    routes as list of RouteDef,
    mounts as list of Mount,
    authenticator as func,
    hasAuthenticator as bool,
    limiter as func,
    hasLimiter as bool,
    limitKey as func,
    hasLimitKey as bool
};

/**
 * The outcome of evaluating a `Spec` against a request: proceed, or halt with the
 * status + message (and any validation failures) to answer. The return of the
 * pure `webapi.evaluate`.
 * @field proceed {bool} true if the request may run the handler
 * @field status {int} the HTTP status to answer when not proceeding
 * @field message {string} the error message to answer
 * @field failures {list of validate.Failure} the field-level failures (validation only)
 */
export def struct Decision {
    proceed as bool,
    status as int,
    message as string,
    failures as list of validate.Failure
};

# --- construction -----------------------------------------------------------

/**
 * A fresh, empty API builder.
 * @return {Api} an API with no routes, mounts, or auth
 */
export func new() {
    def routes as list of RouteDef init [];
    def mounts as list of Mount init [];
    def noHandler as func;
    return Api{
        routes: $routes,
        mounts: $mounts,
        authenticator: $noHandler,
        hasAuthenticator: false,
        limiter: $noHandler,
        hasLimiter: false,
        limitKey: $noHandler,
        hasLimitKey: false
    };
}

/**
 * A zero `Spec`: a public JSON route with no scopes, rules, or rate limit. Keeps
 * an unauthenticated route a one-liner: `webapi.get($a, "/x", h, webapi.public())`.
 * @return {Spec} the zero specification
 */
export func public() {
    def spec as Spec;
    return $spec;
}

# --- builder ----------------------------------------------------------------

/**
 * Serve this version's routes under `path`, returning a new Api. A version may be
 * mounted at several paths (`mount` then `alias`); each is served by `install`.
 * @param a {Api} the builder to extend
 * @param version {int} the version label (surfaced in discovery)
 * @param path {string} the base path to mount under
 * @return {Api} a new Api with the mount added
 */
export func mount(a as Api, version as int, path as string) {
    def out as Api init $a;
    $out.mounts[] = Mount{version: $version, path: $path, sunset: ""};
    return $out;
}

/**
 * Additionally serve `version` at the bare root, so `/v1/deck` is also reachable
 * as `/deck`.
 * @param a {Api} the builder to extend
 * @param version {int} the version to also mount at the root
 * @return {Api} a new Api with the root alias added
 */
export func alias(a as Api, version as int) {
    def out as Api init $a;
    $out.mounts[] = Mount{version: $version, path: "", sunset: ""};
    return $out;
}

/**
 * Mark every mount of `version` deprecated with a sunset date, surfaced in the
 * discovery document.
 * @param a {Api} the builder to extend
 * @param version {int} the version to deprecate
 * @param sunset {string} the sunset date (e.g. an ISO date)
 * @return {Api} a new Api with the version marked deprecated
 */
export func deprecate(a as Api, version as int, sunset as string) {
    def out as Api init $a;
    def marked as list of Mount init [];
    for (def m in $out.mounts) {
        if ($m.version == $version) {
            $marked[] = Mount{version: $m.version, path: $m.path, sunset: $sunset};
        } else {
            $marked[] = $m;
        }
    }
    $out.mounts = $marked;
    return $out;
}

/**
 * Set the authenticator: an entry-program `func(token as string) ->
 * webapi.Identity` value. The module calls it for a `Bearer` route; it never
 * learns how a token is verified.
 * @param a {Api} the builder to extend
 * @param handler {func} the verifier func value
 * @return {Api} a new Api with the authenticator set
 */
export func authenticator(a as Api, handler as func) {
    def out as Api init $a;
    $out.authenticator = $handler;
    $out.hasAuthenticator = true;
    return $out;
}

/**
 * Set the rate limiter: an entry-program `func(key as string, limit as int) ->
 * ratelimit.Result` value - usually `ratelimit.check` over a limiter built for
 * `limit`. A denied request is answered `429` with `Retry-After` from the
 * result's `retryAfter`. The key is the identity's subject when authenticated,
 * else the `limitKey` function's result, else the client host (an IPv6 client by
 * its /64, since one host routinely holds a whole /64).
 * @param a {Api} the builder to extend
 * @param handler {func} the limiter func value
 * @return {Api} a new Api with the limiter set
 */
export func limiter(a as Api, handler as func) {
    def out as Api init $a;
    $out.limiter = $handler;
    $out.hasLimiter = true;
    return $out;
}

/**
 * Set how an unauthenticated request is keyed for rate limiting: an
 * entry-program `func(ctx as web.Context) -> string` value. The default is the
 * client host (IPv6 by /64). Behind a reverse proxy every request comes from the
 * proxy, so supply a function that reads the forwarding header the proxy sets -
 * and trust it only from the proxy: any client can send `X-Forwarded-For`, so
 * honouring it unconditionally lets a client pick its own bucket. Authenticated
 * requests stay keyed on the identity's subject.
 * @param a {Api} the builder to extend
 * @param handler {func} the key func value
 * @return {Api} a new Api with the key function set
 */
export func limitKey(a as Api, handler as func) {
    def out as Api init $a;
    $out.limitKey = $handler;
    $out.hasLimitKey = true;
    return $out;
}

# addRoute is the shared registrar: it defaults the discovery feature label to
# "METHOD /pattern" (a func value has no name to borrow) and appends. An
# undefined handler is already a parse-time error at the call site (a bare name
# that is not a top-level function), so no runtime existence check is needed.
func addRoute(a as Api, method as string, pattern as string, handler as func, spec as Spec) {
    def out as Api init $a;
    $out.routes[] = RouteDef{
        method: $method,
        pattern: $pattern,
        handler: $handler,
        feature: $method + " " + $pattern,
        spec: $spec
    };
    return $out;
}

/**
 * Register a GET route with a `Spec`.
 * @param a {Api} the builder to extend
 * @param pattern {string} the route pattern (`:param` captures allowed)
 * @param handler {func} the entry-program handler func value
 * @param spec {Spec} the route's specification
 * @return {Api} a new Api with the route added
 */
export func get(a as Api, pattern as string, handler as func, spec as Spec) {
    return addRoute($a, "GET", $pattern, $handler, $spec);
}

/**
 * Register a POST route with a `Spec`.
 * @param a {Api} the builder to extend
 * @param pattern {string} the route pattern
 * @param handler {func} the entry-program handler func value
 * @param spec {Spec} the route's specification
 * @return {Api} a new Api with the route added
 */
export func post(a as Api, pattern as string, handler as func, spec as Spec) {
    return addRoute($a, "POST", $pattern, $handler, $spec);
}

/**
 * Register a PUT route with a `Spec`.
 * @param a {Api} the builder to extend
 * @param pattern {string} the route pattern
 * @param handler {func} the entry-program handler func value
 * @param spec {Spec} the route's specification
 * @return {Api} a new Api with the route added
 */
export func put(a as Api, pattern as string, handler as func, spec as Spec) {
    return addRoute($a, "PUT", $pattern, $handler, $spec);
}

/**
 * Register a PATCH route with a `Spec`.
 * @param a {Api} the builder to extend
 * @param pattern {string} the route pattern
 * @param handler {func} the entry-program handler func value
 * @param spec {Spec} the route's specification
 * @return {Api} a new Api with the route added
 */
export func patch(a as Api, pattern as string, handler as func, spec as Spec) {
    return addRoute($a, "PATCH", $pattern, $handler, $spec);
}

/**
 * Register a DELETE route with a `Spec`.
 * @param a {Api} the builder to extend
 * @param pattern {string} the route pattern
 * @param handler {func} the entry-program handler func value
 * @param spec {Spec} the route's specification
 * @return {Api} a new Api with the route added
 */
export func delete(a as Api, pattern as string, handler as func, spec as Spec) {
    return addRoute($a, "DELETE", $pattern, $handler, $spec);
}

/**
 * Set an explicit discovery feature label for the most recently added route
 * (defaults to "METHOD /pattern"), so the discovery document reads meaningfully.
 * @param a {Api} the builder whose last route to label
 * @param feature {string} the feature name
 * @return {Api} a new Api with the last route's feature set
 * @throws {Error} kind "webapi" when there is no route to label
 */
export func feature(a as Api, feature as string) {
    if (len($a.routes) == 0) {
        apiFail("webapi.feature: no route to label yet");
    }
    def out as Api init $a;
    def last as int init len($out.routes) - 1;
    def r as RouteDef init $out.routes[$last];
    $out.routes[$last] = RouteDef{
        method: $r.method,
        pattern: $r.pattern,
        handler: $r.handler,
        feature: $feature,
        spec: $r.spec
    };
    return $out;
}

/**
 * Register every route on the `web.App`, once per mount path (via `web.mount`),
 * and wire the `Spec`-enforcing guard as a `before` middleware. `guard` must be an
 * entry-program `func` value that calls `webapi.guard($api, $ctx)`; the shim is
 * only needed to bind the `Api` (Jennifer has no closures yet for the guard to
 * capture it directly).
 * @param a {Api} the built API
 * @param app {web.App} the web router to register onto
 * @param guard {func} the entry-program guard shim func value
 * @return {web.App} a new App with the routes and guard installed
 */
export func install(a as Api, app as web.App, guard as func) {
    def out as web.App init $app;
    # Build a sub-router holding every route once, then mount it under each path.
    def sub as web.App init web.new();
    for (def r in $a.routes) {
        $sub = web.route($sub, $r.method, $r.pattern, $r.handler);
    }
    for (def m in $a.mounts) {
        $out = web.mount($out, $m.path, $sub);
    }
    $out = web.before($out, $guard);
    return $out;
}

# --- routing / matching (private) -------------------------------------------

func apiFail(msg as string) {
    throw Error{kind: "webapi", message: $msg};
}

# pathSegs splits a path into non-empty segments.
func pathSegs(p as string) {
    def out as list of string init [];
    for (def s in strings.split($p, "/")) {
        if (not ($s == "")) {
            $out[] = $s;
        }
    }
    return $out;
}

# One matched route: whether it matched, its Spec, feature, and captured params.
def struct Matched {
    found as bool,
    spec as Spec,
    feature as string,
    params as map of string to string
};

# matchSegs matches pattern segments against path segments, capturing `:name`
# and a trailing `*name` wildcard (mirroring web's matcher). Returns the params
# and whether it matched.
func matchSegs(pat as list of string, segs as list of string) {
    def params as map of string to string init {};
    def wildKey as string init "";
    def fixed as int init len($pat);
    if (len($pat) > 0 and strings.startsWith($pat[len($pat) - 1], "*")) {
        def last as string init $pat[len($pat) - 1];
        $wildKey = $last[1..];
        $fixed = len($pat) - 1;
    }
    if (len($wildKey) > 0) {
        if (len($segs) < $fixed) {
            return {"__nomatch": ""};
        }
    } elseif (not (len($pat) == len($segs))) {
        return {"__nomatch": ""};
    }
    for (def i in 0..$fixed) {
        def ps as string init $pat[$i];
        if (strings.startsWith($ps, ":")) {
            $params[$ps[1..]] = $segs[$i];
        } elseif (not ($ps == $segs[$i])) {
            return {"__nomatch": ""};
        }
    }
    if (len($wildKey) > 0) {
        $params[$wildKey] = strings.join($segs[$fixed..], "/");
    }
    return $params;
}

# findRoute locates the webapi route serving `method` + `path` (trying every
# route under every mount prefix), returning its Spec / feature / params.
func findRoute(a as Api, method as string, path as string) {
    def segs as list of string init pathSegs($path);
    for (def r in $a.routes) {
        if ($r.method == $method) {
            for (def m in $a.mounts) {
                def full as string init web.joinRoute($m.path, $r.pattern);
                def params as map of string to string init matchSegs(pathSegs($full), $segs);
                if (not maps.has($params, "__nomatch")) {
                    return Matched{
                        found: true,
                        spec: $r.spec,
                        feature: $r.feature,
                        params: $params
                    };
                }
            }
        }
    }
    def empty as Spec;
    def noParams as map of string to string init {};
    return Matched{found: false, spec: $empty, feature: "", params: $noParams};
}

# --- request data extraction ------------------------------------------------

# jsonScalars flattens a JSON body's top-level scalar fields into a string map;
# a nested / array field is skipped (read it with web.bodyJson). A non-JSON body
# yields an empty map.
func jsonScalars(ctx as web.Context) {
    def out as map of string to string init {};
    def doc as json.Value init emptyJson();
    try {
        $doc = web.bodyJson($ctx);
    } catch (e) { # lint-disable: L103
        return $out;
    }
    if (not (json.typeOf($doc, "") == "map")) {
        return $out;
    }
    for (def k in json.keys($doc, "")) {
        def ptr as string init "/" + $k;
        def t as string init json.typeOf($doc, $ptr);
        if ($t == "string") {
            $out[$k] = json.asString($doc, $ptr);
        } elseif ($t == "int") {
            $out[$k] = convert.toString(json.asInt($doc, $ptr));
        } elseif ($t == "float") {
            $out[$k] = convert.toString(json.asFloat($doc, $ptr));
        } elseif ($t == "bool") {
            $out[$k] = convert.toString(json.asBool($doc, $ptr));
        }
    }
    return $out;
}

func emptyJson() {
    return json.map();
}

# requestData collects the named `fields` from the request into one string map -
# the surface `validate` and the handler's `webapi.validated` see. A field is
# taken from the query first, then the JSON body's top-level scalars, then the
# form body. Only the named fields are read, so a body parsed both ways never
# leaks a spurious key.
func requestData(ctx as web.Context, fields as list of string) {
    def out as map of string to string init {};
    if (len($fields) == 0) {
        return $out;
    }
    def form as map of string to string init web.form($ctx);
    def jsonB as map of string to string init jsonScalars($ctx);
    for (def f in $fields) {
        def q as string init web.query($ctx, $f);
        if (not ($q == "")) {
            $out[$f] = $q;
        } elseif (maps.has($jsonB, $f)) {
            $out[$f] = $jsonB[$f];
        } elseif (maps.has($form, $f)) {
            $out[$f] = $form[$f];
        }
    }
    return $out;
}

/**
 * The named query parameters as a string map (absent names omitted).
 * @param ctx {web.Context} the request context
 * @param fields {list of string} the parameter names to read
 * @return {map of string to string} the present query values
 */
export func queryData(ctx as web.Context, fields as list of string) {
    def out as map of string to string init {};
    for (def f in $fields) {
        def v as string init web.query($ctx, $f);
        if (not ($v == "")) {
            $out[$f] = $v;
        }
    }
    return $out;
}

/**
 * The form body (`application/x-www-form-urlencoded`) as a string map.
 * @param ctx {web.Context} the request context
 * @return {map of string to string} the form fields
 */
export func formData(ctx as web.Context) {
    return web.form($ctx);
}

/**
 * The named top-level JSON body fields, flattened to strings (a nested or
 * array-valued field is omitted; read those with `web.bodyJson`).
 * @param ctx {web.Context} the request context
 * @param fields {list of string} the JSON keys to read
 * @return {map of string to string} the present scalar fields
 */
export func jsonData(ctx as web.Context, fields as list of string) {
    def all as map of string to string init jsonScalars($ctx);
    def out as map of string to string init {};
    for (def f in $fields) {
        if (maps.has($all, $f)) {
            $out[$f] = $all[$f];
        }
    }
    return $out;
}

/**
 * Inside a handler: the request data the route's rules were checked against
 * (query + body scalars). The guard already validated it, so a handler that got
 * this far can trust it. Takes the `Api` because Jennifer has no per-request
 * state to stash it in.
 * @param a {Api} the built API
 * @param ctx {web.Context} the request context
 * @return {map of string to string} the request data
 */
export func validated(a as Api, ctx as web.Context) {
    def m as Matched init findRoute($a, web.method($ctx), web.path($ctx));
    return requestData($ctx, maps.keys($m.spec.rules));
}

/**
 * Inside a handler: the authenticated identity for this request (re-derived via
 * the authenticator). Returns a zero `Identity` (`ok: false`) for a public route
 * or an absent credential.
 * @param a {Api} the built API
 * @param ctx {web.Context} the request context
 * @return {Identity} the caller's identity
 */
export func identity(a as Api, ctx as web.Context) {
    def none as Identity;
    if (not $a.hasAuthenticator) {
        return $none;
    }
    def token as string init web.bearerToken($ctx);
    if ($token == "") {
        return $none;
    }
    def auth as func init $a.authenticator;
    return $auth($token);
}

# --- Spec evaluation (pure) -------------------------------------------------

/**
 * The pure core: decide whether a request bearing `identity` and `data` may
 * proceed under `spec`, without touching the engine. Returns proceed=true, or
 * proceed=false with the status / message (and validation failures) to answer.
 * Order: auth (401) -> scopes (403) -> validation (422). Rate limiting is applied
 * by the guard (it is stateful), not here.
 * @param spec {Spec} the route specification
 * @param identity {Identity} the authenticated caller (ok=false when none)
 * @param data {map of string to string} the request data to validate
 * @return {Decision} proceed, or a status + message to answer
 */
export func evaluate(spec as Spec, identity as Identity, data as map of string to string) {
    def none as list of validate.Failure init [];
    match ($spec.auth) {
        when Bearer {
            if (not $identity.ok) {
                return Decision{
                    proceed: false,
                    status: 401,
                    message: "unauthorized",
                    failures: $none
                };
            }
        }
        when None {
        }
    }
    for (def need in $spec.scopes) {
        if (not lists.contains($identity.scopes, $need)) {
            return Decision{
                proceed: false,
                status: 403,
                message: "missing scope: " + $need,
                failures: $none
            };
        }
    }
    if (len($spec.rules) > 0) {
        def failures as list of validate.Failure init validate.check($data, $spec.rules);
        if (len($failures) > 0) {
            return Decision{
                proceed: false,
                status: 422,
                message: "invalid request",
                failures: $failures
            };
        }
    }
    return Decision{proceed: true, status: 200, message: "", failures: $none};
}

# --- the guard (engine-facing) ----------------------------------------------

/**
 * The `Spec`-enforcing middleware body. Wire it from an entry-program shim named
 * in `install`: `func apiGuard(ctx) { return webapi.guard($api, $ctx); }`. It
 * matches the request to its route, authenticates, checks scopes, validates, and
 * rate-limits, answering the request and returning false on any failure. A
 * request that matches no API route passes through (returns true).
 * @param a {Api} the built API
 * @param ctx {web.Context} the request context
 * @return {bool} true to proceed to the handler, false when it has answered
 */
export func guard(a as Api, ctx as web.Context) {
    def m as Matched init findRoute($a, web.method($ctx), web.path($ctx));
    if (not $m.found) {
        return true;
    }
    def who as Identity init identity($a, $ctx);
    # A Bearer route with an absent token is a 401 with a challenge, before the
    # pure evaluation (which cannot emit the WWW-Authenticate header).
    match ($m.spec.auth) {
        when Bearer {
            if (web.bearerToken($ctx) == "") {
                unauthorized($ctx, "missing bearer token");
                return false;
            }
        }
        when None {
        }
    }
    def data as map of string to string init requestData($ctx, maps.keys($m.spec.rules));
    def d as Decision init evaluate($m.spec, $who, $data);
    if (not $d.proceed) {
        if ($d.status == 401) {
            unauthorized($ctx, $d.message);
        } elseif (len($d.failures) > 0) {
            failWith($ctx, $d.status, $d.message, $d.failures);
        } else {
            fail($ctx, $d.status, $d.message);
        }
        return false;
    }
    if ($m.spec.rateLimit > 0 and $a.hasLimiter) {
        def lim as func init $a.limiter;
        def r as ratelimit.Result init $lim(limitKeyFor($a, $ctx, $who), $m.spec.rateLimit);
        if (not $r.allowed) {
            if ($r.retryAfter > 0) {
                web.setHeader($ctx, "Retry-After", convert.toString($r.retryAfter));
            }
            fail($ctx, 429, "rate limit exceeded");
            return false;
        }
    }
    return true;
}

# limitKeyFor picks the rate-limit key: the identity subject when authenticated,
# else the app's key function, else the client host - so an unauthenticated flood
# is bounded per client, not per connection (the port changes every connection).
func limitKeyFor(a as Api, ctx as web.Context, who as Identity) {
    if ($who.ok and $who.subject != "") {
        return $who.subject;
    }
    if ($a.hasLimitKey) {
        def keyOf as func init $a.limitKey;
        return $keyOf($ctx);
    }
    return addressKey(web.remoteHost($ctx));
}

# addressKey turns a client host into a rate-limit key: an IPv4 address as is
# (an IPv4-mapped IPv6 address unmapped first, so it is not lumped into ::/64),
# an IPv6 address as its /64 network. A host ipnet cannot parse (a zoned
# link-local address) is its own key.
func addressKey(host as string) {
    try {
        def addr as ipnet.Address init ipnet.unmap(ipnet.parseAddress($host));
        if (ipnet.version($addr) == 6) {
            return ipnet.networkString(ipnet.parse(ipnet.toString($addr) + "/64"));
        }
        return ipnet.toString($addr);
    } catch (e) {
        return $host;
    }
}

# --- error envelopes --------------------------------------------------------

/**
 * Send the uniform error envelope `{"error": message}` at `status`.
 * @param ctx {web.Context} the request context
 * @param status {int} the HTTP status
 * @param message {string} the error message
 */
export func fail(ctx as web.Context, status as int, message as string) {
    def env as json.Value init json.set(json.map(), "/error", $message);
    web.sendJson($ctx, $status, $env);
    return;
}

/**
 * Send the error envelope with field-level `failures` detail:
 * `{"error": message, "failures": [{field, rule, message}, ...]}`.
 * @param ctx {web.Context} the request context
 * @param status {int} the HTTP status
 * @param message {string} the error message
 * @param failures {list of validate.Failure} the per-field failures
 */
export func failWith(
    ctx as web.Context,
    status as int,
    message as string,
    failures as list of validate.Failure) {
    def env as json.Value init json.set(json.map(), "/error", $message);
    $env = json.set($env, "/failures", json.list());
    for (def f in $failures) {
        def item as json.Value init json.set(json.map(), "/field", $f.field);
        $item = json.set($item, "/rule", $f.rule);
        $item = json.set($item, "/message", $f.message);
        $env = json.append($env, "/failures", $item);
    }
    web.sendJson($ctx, $status, $env);
    return;
}

/**
 * Send a `404` error envelope.
 * @param ctx {web.Context} the request context
 * @param message {string} the error message
 */
export func notFound(ctx as web.Context, message as string) {
    fail($ctx, 404, $message);
    return;
}

/**
 * Send a `403` error envelope.
 * @param ctx {web.Context} the request context
 * @param message {string} the error message
 */
export func denied(ctx as web.Context, message as string) {
    fail($ctx, 403, $message);
    return;
}

/**
 * Send a `401` error envelope with a `WWW-Authenticate: Bearer` header.
 * @param ctx {web.Context} the request context
 * @param message {string} the error message
 */
export func unauthorized(ctx as web.Context, message as string) {
    web.setHeader($ctx, "WWW-Authenticate", "Bearer");
    fail($ctx, 401, $message);
    return;
}

# --- content negotiation + pagination ---------------------------------------

/**
 * Register a `web.onError` envelope handler on `app`, so an uncaught throw
 * becomes a `500` in the same shape rather than a bare engine error. Pair with
 * `install`; the original error is still logged by `web`.
 * @param app {web.App} the router to extend
 * @param handler {func} an entry-program `func(e as Error)` value that calls `webapi`
 * @return {web.App} a new App with the error handler set
 */
export func onError(app as web.App, handler as func) {
    return web.onError($app, $handler);
}

/**
 * What the caller wants from the `Accept` header: `"json"` or `"html"`, by
 * content negotiation (see `negotiate`) between `application/json` and
 * `text/html`. JSON wins a tie, and is the answer when the header is absent,
 * `* / *`, or accepts neither type.
 * @param ctx {web.Context} the request context
 * @return {string} "json" or "html"
 */
export func wants(ctx as web.Context) {
    if (negotiate($ctx, ["application/json", "text/html"]) == "text/html") {
        return "html";
    }
    return "json";
}

/**
 * Pick the media type the caller prefers from those the handler can send, by
 * the request's `Accept` header (RFC 9110 content negotiation). Each offered
 * type takes the quality (`q`, default 1) of the most specific range that
 * matches it (an exact `type/subtype` over a `type` wildcard over the full
 * wildcard); the highest positive
 * quality wins, and a tie goes to the type listed first in `offered`. An absent
 * header accepts everything, so the first offered type wins.
 * @param ctx {web.Context} the request context
 * @param offered {list of string} the media types the handler can produce, in order of preference
 * @return {string} the chosen type from `offered`, or "" when the caller accepts none of them
 */
export func negotiate(ctx as web.Context, offered as list of string) {
    return bestMediaType(web.header($ctx, "Accept"), $offered);
}

/**
 * The `negotiate` decision over a raw `Accept` header value.
 * @param accept {string} the header value ("" when absent)
 * @param offered {list of string} the candidate media types, in preference order
 * @return {string} the chosen type, or "" when none is acceptable
 */
func bestMediaType(accept as string, offered as list of string) {
    if (strings.trim($accept) == "") {
        if (len($offered) == 0) {
            return "";
        }
        return $offered[0];
    }
    def ranges as list of AcceptRange init parseAccept($accept);
    def best as string init "";
    def bestQ as float init 0.0;
    for (def t in $offered) {
        def q as float init acceptQuality($ranges, strings.lower($t));
        if ($q > $bestQ) {
            $best = $t;
            $bestQ = $q;
        }
    }
    return $best;
}

# One media range from an Accept header: `type/subtype` (either part may be
# "*"), its quality, and how specific it is (2 exact, 1 type/*, 0 */*).
def struct AcceptRange {
    range as string,
    q as float,
    specificity as int
};

/**
 * Parse an `Accept` header into its media ranges. Parameters other than `q` are
 * ignored; a malformed `q` counts as 1 and a range without a "/" is skipped.
 * @param accept {string} the header value
 * @return {list of AcceptRange} the ranges, lower-cased
 */
func parseAccept(accept as string) {
    def out as list of AcceptRange init [];
    for (def part in strings.split($accept, ",")) {
        def params as list of string init strings.split($part, ";");
        def range as string init strings.lower(strings.trim($params[0]));
        def slash as int init strings.indexOf($range, "/");
        if ($slash < 0) {
            continue;
        }
        def q as float init 1.0;
        for (def p in $params[1..]) {
            def kv as string init strings.lower(strings.trim($p));
            if (strings.startsWith($kv, "q=")) {
                $q = parseQuality($kv[2..]);
            }
        }
        def specificity as int init 2;
        if ($range == "*/*") {
            $specificity = 0;
        } elseif ($range[$slash + 1..] == "*") {
            $specificity = 1;
        }
        $out[] = AcceptRange{range: $range, q: $q, specificity: $specificity};
    }
    return $out;
}

/**
 * Read a `q` parameter value; a malformed one counts as 1 (the default).
 * @param text {string} the text after "q="
 * @return {float} the quality
 */
func parseQuality(text as string) {
    try {
        return convert.toFloat(strings.trim($text));
    } catch (e) {
        return 1.0;
    }
}

/**
 * The quality the parsed ranges give one media type: that of the most specific
 * matching range, or 0 when none matches.
 * @param ranges {list of AcceptRange} the parsed header
 * @param mediaType {string} a lower-case `type/subtype`
 * @return {float} the quality in [0, 1]
 */
func acceptQuality(ranges as list of AcceptRange, mediaType as string) {
    def slash as int init strings.indexOf($mediaType, "/");
    def major as string init $mediaType;
    if ($slash >= 0) {
        $major = $mediaType[..$slash];
    }
    def q as float init 0.0;
    def found as int init -1;
    for (def r in $ranges) {
        def wildcard as bool init $r.range == "*/*" or $r.range == $major + "/*";
        def hit as bool init $wildcard or $r.range == $mediaType;
        if ($hit and $r.specificity > $found) {
            $q = $r.q;
            $found = $r.specificity;
        }
    }
    return $q;
}

/**
 * Send an envelope-consistent JSON response.
 * @param ctx {web.Context} the request context
 * @param status {int} the HTTP status
 * @param value {json.Value} the response body
 */
export func sendJson(ctx as web.Context, status as int, value as json.Value) {
    web.sendJson($ctx, $status, $value);
    return;
}

/**
 * Parse and clamp the `offset` / `limit` query parameters into a `Page`. `limit`
 * defaults to `defaultLimit` and is clamped to `[1, maxLimit]`; `offset` is
 * clamped to `>= 0`, so a client cannot ask for the whole table or a bad window.
 * @param ctx {web.Context} the request context
 * @param defaultLimit {int} the limit when none is supplied
 * @param maxLimit {int} the largest allowed limit
 * @return {Page} the clamped window
 */
export func page(ctx as web.Context, defaultLimit as int, maxLimit as int) {
    def limit as int init $defaultLimit;
    def lq as string init web.query($ctx, "limit");
    if (not ($lq == "") and isDigits($lq)) {
        $limit = convert.toInt($lq);
    }
    if ($limit < 1) {
        $limit = 1;
    }
    if ($limit > $maxLimit) {
        $limit = $maxLimit;
    }
    def offset as int init 0;
    def oq as string init web.query($ctx, "offset");
    if (not ($oq == "") and isDigits($oq)) {
        $offset = convert.toInt($oq);
    }
    if ($offset < 0) {
        $offset = 0;
    }
    return Page{offset: $offset, limit: $limit};
}

func isDigits(s as string) {
    # Cap the length so a value accepted here always fits in int64: a 19-digit
    # all-digit string can exceed it, and page() would then call convert.toInt on
    # it and throw kind "runtime" - turning an oversized `limit` / `offset` query
    # into a 500 instead of clamping. An over-long value is treated as absent (the
    # default limit / a zero offset), which page() then clamps as usual.
    if (len($s) > 18) {
        return false;
    }
    return regex.matches('^[0-9]+$', $s);
}

/**
 * Send a paged list response: `{"items": [...], "offset", "limit", "total"}`.
 * `items` must already be a `json.Value` list (the page slice the handler built).
 * @param ctx {web.Context} the request context
 * @param items {json.Value} the page's items, as a JSON list
 * @param p {Page} the page window
 * @param total {int} the total item count across all pages
 */
export func sendPage(ctx as web.Context, items as json.Value, p as Page, total as int) {
    def env as json.Value init json.set(json.map(), "/items", $items);
    $env = json.set($env, "/offset", $p.offset);
    $env = json.set($env, "/limit", $p.limit);
    $env = json.set($env, "/total", $total);
    web.sendJson($ctx, 200, $env);
    return;
}

# --- discovery --------------------------------------------------------------

/**
 * Build the discovery document from the route table, so the advertised versions
 * and features cannot drift from what is actually served:
 * `{"registry", "spec", "apis": [{version, basePath, deprecated, sunset}],
 * "features": [name, ...]}`. `features` is the set of route feature labels.
 * @param a {Api} the built API
 * @param registry {string} the registry / service name to advertise
 * @param specVersion {string} the spec version string to advertise
 * @return {json.Value} the discovery document
 */
export func discovery(a as Api, registry as string, specVersion as string) {
    def doc as json.Value init json.set(json.map(), "/registry", $registry);
    $doc = json.set($doc, "/spec", $specVersion);
    $doc = json.set($doc, "/apis", json.list());
    for (def m in $a.mounts) {
        def base as string init $m.path;
        if ($base == "") {
            $base = "/";
        }
        def entry as json.Value init json.set(json.map(), "/version", $m.version);
        $entry = json.set($entry, "/basePath", $base);
        $entry = json.set($entry, "/deprecated", not ($m.sunset == ""));
        if (not ($m.sunset == "")) {
            $entry = json.set($entry, "/sunset", $m.sunset);
        }
        $doc = json.append($doc, "/apis", $entry);
    }
    $doc = json.set($doc, "/features", json.list());
    def seen as map of string to string init {};
    for (def r in $a.routes) {
        if (not maps.has($seen, $r.feature)) {
            $seen[$r.feature] = "";
            $doc = json.append($doc, "/features", $r.feature);
        }
    }
    return $doc;
}
