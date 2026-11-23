# `webapi` - a JSON-API conventions layer over `web`

`webapi` sits on top of the bundled [`web`](web.md) module and supplies the
pieces every JSON API otherwise re-implements by hand: a uniform error envelope,
request validation (reusing [`validate`](validate.md)), versioned route mounting,
pluggable bearer auth and rate limiting, content negotiation, and pagination.
`web` stays the HTTP framework; `webapi` carries the JSON-API opinions, so an
application can take one without the other.

```jennifer
use json;
import "web.j" as web;
import "webapi.j" as webapi;
import "validate.j" as validate;

# How a token is checked stays the application's business.
func verifyToken(token as string) {
    if ($token == "admin-token") {
        return webapi.Identity{ok: true, subject: "u-admin", display: "admin", scopes: ["publish"]};
    }
    def anonymous as webapi.Identity;
    return $anonymous;
}

func getDeck(ctx as web.Context) {
    webapi.sendJson($ctx, 200, json.set(json.map(), "/deck", web.param($ctx, "name")));
}

func publish(ctx as web.Context) {
    def form as map of string to string init webapi.validated($api, $ctx);
    webapi.sendJson($ctx, 201, json.set(json.map(), "/published", $form["tag"]));
}

def app as web.App init web.new();
def api as webapi.Api init webapi.new();
$api = webapi.mount($api, 1, "/v1");
$api = webapi.alias($api, 1);
$api = webapi.authenticator($api, verifyToken);
$api = webapi.get($api, "/deck/:name", getDeck, webapi.public());
$api = webapi.post($api, "/publish", publish, webapi.Spec{
    summary: "publish a deck version",
    auth: webapi.Auth.Bearer, scopes: ["publish"],
    rules: {"tag": [validate.required(), validate.maxLen(16)]},
    rateLimit: 30, produces: webapi.Produces.Json
});

func apiGuard(ctx as web.Context) { return webapi.guard($api, $ctx); }
$app = webapi.install($api, $app, apiGuard);
web.run($app, "127.0.0.1:8080");
```

Runnable: [`examples/modules/webapi_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/webapi_demo.j).

## The builder

`webapi.new()` returns a value-semantic `Api` you finish before the server runs;
every builder returns a fresh `Api`.

| Call | Does |
| ---- | ---- |
| `webapi.mount(a, version, path)` | serve the routes under `path` (labelled `version`) |
| `webapi.alias(a, version)` | additionally serve `version` at the bare root |
| `webapi.deprecate(a, version, sunset)` | mark a version deprecated, with its sunset date (`Sunset` header) |
| `webapi.deprecatedSince(a, version, since)` | mark a version deprecated as of a date (`Deprecation` header) |
| `webapi.authenticator(a, fn)` | the token-verifier `func` value |
| `webapi.limiter(a, fn)` | the rate-limit `func` value (returns a `ratelimit.Result`) |
| `webapi.limitKey(a, fn)` | how an unauthenticated request is keyed for rate limiting (default: the client host, IPv6 by /64) |
| `webapi.get/post/put/patch/delete(a, pattern, handler, spec)` | register a route (`handler` a `func` value) with a `Spec` |
| `webapi.feature(a, name)` | label the last route for the discovery document |
| `webapi.install(a, app, guard)` | register every route (once per mount) with the guard `func` value as its handler |

`webapi.public()` returns a zero `Spec` - an unauthenticated JSON route with no
scopes, rules, or rate limit.

### The `Spec`

```jennifer
export def struct Spec {
    summary as string,
    auth as Auth,                                     # Auth.None (public) or Auth.Bearer
    scopes as list of string,                         # permissions the identity needs
    rules as map of string to list of validate.Rule,  # request validation
    rateLimit as int,                                 # per identity per window; 0 = none
    produces as Produces                              # Produces.Json / Html / Negotiate
};
```

`Auth` and `Produces` are enums, not strings, so a `match` on them is
exhaustiveness-checked and a typo is a compile error.

## The guard, and why the shim

Routes, the authenticator, the limiter, and the guard are all `func` values,
called through their home interpreter so they resolve their own imports and can
build a `webapi.Identity` across the module boundary. The one thing a `func` value
still cannot do is **capture** the built `Api` (Jennifer has no closures yet), so
`install` registers a `func` value you supply as every route's `web` handler,
backed by a one-line entry-program shim that binds the `Api`:

```jennifer
func apiGuard(ctx as web.Context) { return webapi.guard($api, $ctx); }
```

The guard runs after every `web.before` middleware (registered before or after
`install`). Per request it matches the route, then, in this order:

1. sets `Deprecation` / `Sunset` for a deprecated version (see below);
2. authenticates (the authenticator runs once; `webapi.identity` reuses it);
3. rate-limits (`429`) - before any check that rejects, so every request counts;
4. answers a `Bearer` route without a token `401`;
5. checks `limit` / `offset` (`400`), the identity (`401`), the scopes (`403`)
   and the `rules` (`422`);
6. runs the route's handler.

A failure is answered with the error envelope. Limiting before the checks means
a flood of bad tokens or invalid bodies (say, at a login route) is limited like
any other traffic; the authenticator comes first only because the limit key
needs the identity.

A throw from the handler - or from the authenticator, limiter or key function -
is answered `500` with `{"error": "internal server error"}` (nothing of the
error itself is sent), unless the handler had already answered. The guard then
rethrows it, so `web` writes it to stderr and calls the `web.onError` hook
(`webapi.onError`) with it; the hook observes, it cannot change the response.

Wired by hand as a `web.before` middleware (without `install`), the guard only
checks: it answers and returns false on a failure, true otherwise, and a request
matching no API route passes through untouched.

The authenticator and limiter signatures:

```text
func verifyToken(token as string) -> webapi.Identity        # ok:false = reject
func countHit(key as string, limit as int) -> ratelimit.Result
```

## Rate limiting

A route with `Spec.rateLimit > 0` calls the limiter with a key and the route's
limit, for every request of the route (also those the guard then rejects). The limiter returns a [`ratelimit.Result`](ratelimit.md); when it is not
`allowed`, the guard answers `429` with `Retry-After` set from its `retryAfter`.
The usual limiter is `ratelimit.check` over a limiter built for the route's
limit:

```jennifer
import "ratelimit.j" as ratelimit;
import "kvstore.j" as kvstore;

def store as kvstore.Store init kvstore.inProcessStore();

func countHit(key as string, limit as int) {
    return ratelimit.check(ratelimit.slidingWindow($store, $limit, 60), $key);
}
```

The key is chosen in this order:

1. **The identity's subject**, when the request is authenticated.
2. **The `limitKey` function's result**, when one is set.
3. **The client host** otherwise: `web.remoteHost`, never the `host:port`
   address (a client opens a new port per connection, which would give it a
   fresh bucket every time). An IPv6 client is keyed by its /64, since one host
   routinely holds a whole /64; an IPv4-mapped address counts as IPv4.

**Behind a reverse proxy** every request comes from the proxy, so the default
key puts all clients in one bucket. Supply a key function that reads the
forwarding header your proxy sets - and trust it only from the proxy, because
any client can send `X-Forwarded-For` and would otherwise pick its own bucket:

```jennifer
use strings;

func clientKey(ctx as web.Context) {
    if (web.remoteHost($ctx) == "10.0.0.5") {             # the proxy
        def fwd as list of string init strings.split(web.header($ctx, "X-Forwarded-For"), ",");
        return strings.trim($fwd[len($fwd) - 1]);         # the address the proxy saw
    }
    return web.remoteHost($ctx);
}

$api = webapi.limitKey($api, clientKey);
```

The `Spec` evaluation itself is a **pure** function you can test without a
server: `webapi.evaluate(spec, identity, data) -> Decision`.

## Error envelopes

Every failure uses one shape, so a client parses one thing:

```json
{ "error": "no such deck" }
{ "error": "invalid request", "failures": [{ "field": "tag", "rule": "required", "message": "is required" }] }
```

| Call | Does |
| ---- | ---- |
| `webapi.fail(ctx, status, message)` | send the envelope at a status |
| `webapi.failWith(ctx, status, message, failures)` | with `validate.Failure` detail |
| `webapi.notFound(ctx, message)` | `404` |
| `webapi.denied(ctx, message)` | `403` |
| `webapi.unauthorized(ctx, message)` | `401` with a `WWW-Authenticate` header |
| `webapi.onError(app, fn)` | the `web.onError` hook: called with a handler's error after the guard has answered it `500` in the envelope |

## Request data

The guard validates a route's `rules` before the handler runs (a failure
short-circuits to `422`), so a handler never has to ask whether its input is
sane. Data is read from the query, then the JSON body's top-level scalars, then
the form body, and the first source holding a field wins. Read checked fields
through `webapi.validated` only: a request can carry the same name in several
places (`?tag=ok` plus a long `tag` in the body), and `web.bodyJson` or
`web.form` would hand the handler the copy that was never checked.

A HEAD request is guarded by the spec of the GET route that web serves it with.

| Call | Does |
| ---- | ---- |
| `webapi.queryData(ctx, fields)` | named query parameters as a string map |
| `webapi.formData(ctx)` | the form body as a string map |
| `webapi.jsonData(ctx, fields)` | named top-level JSON fields, flattened to strings |
| `webapi.validated(a, ctx)` | inside a handler: the checked data for this request |
| `webapi.identity(a, ctx)` | inside a handler: the caller the guard authenticated (no second authenticator call) |

A nested or array-valued body is the handler's own business, read with
`web.bodyJson`.

## Negotiation, pagination, discovery

| Call | Does |
| ---- | ---- |
| `webapi.wants(ctx)` | `"json"` or `"html"` from `Accept` (JSON on a tie or no match) |
| `webapi.negotiate(ctx, offered)` | the best of the `offered` media types for `Accept`, or `""` when none is acceptable |
| `webapi.sendJson(ctx, status, value)` | send a `json.Value` |
| `webapi.page(ctx, defaultLimit, maxLimit)` | parse `offset`/`limit` -> `Page`, `limit` lowered to `maxLimit` |
| `webapi.sendPage(ctx, items, page, total)` | a `{items, offset, limit, total}` response |
| `webapi.discovery(a, registry, specVersion)` | a discovery `json.Value` derived from the route table |

`negotiate` follows RFC 9110: each offered type takes the `q` of the most
specific matching range (`text/html` over `text/*` over `*/*`, default `q=1`),
`q=0` excludes a type, the highest positive `q` wins, and a tie goes to the type
listed first. A range whose `q` is not an RFC 9110 qvalue (0 to 1 with at most
three decimals, so not `q=2`, `q=-1`, `q=0.1234` or `q=bogus`) is ignored, as is
a range without a `/`. Only the first 64 ranges are read, and the first 16
`;` pieces of each. An absent `Accept` header picks the first offered type. So
`Accept: application/json, text/html;q=0.1` gets JSON from `wants`, and a
browser's `text/html,...,*/*;q=0.8` gets HTML. Answer `406` yourself when
`negotiate` returns `""` and the handler cannot fall back.

`page` lowers `limit` to `maxLimit` so a client cannot ask for the whole table;
an absent `limit` is `defaultLimit`, an absent `offset` 0. `limit` and `offset`
are webapi's paging parameters, so `guard` checks them on every API route: a
value that is not a whole number of at most 18 digits (`abc`, `-1`, `+5`, a
20-digit number) or a `limit` of 0 is answered with a `400` envelope listing
the bad fields (`{"error": "invalid paging parameters", "failures": [...]}`)
before the handler runs, instead of becoming the default. On a route the guard
does not cover, `page` throws `Error{kind: "webapi"}` for such a value.
`discovery` builds its `apis[]` and `features[]` from what is actually
registered, so the advertised surface cannot drift from the routes that exist.

## Deprecated versions

```jennifer
$api = webapi.deprecatedSince($api, 1, "2026-01-01");
$api = webapi.deprecate($api, 1, "2027-06-30");
```

Every response of a route served under a mount of that version then carries

```text
Deprecation: @1767225600
Sunset: Wed, 30 Jun 2027 00:00:00 GMT
```

`Deprecation` is RFC 9745's structured-field date (a future date announces a
coming deprecation), `Sunset` RFC 8594's HTTP-date; each is sent only when its
date is set. A date is `YYYY-MM-DD` (midnight UTC) or an RFC 3339 timestamp;
anything else, or a sunset before the deprecation date, throws
`Error{kind: "webapi"}` while the `Api` is built. The discovery document lists
the version as `deprecated` with its `deprecatedSince` and `sunset`.

## Placement

- Depends on `web`, `validate`, `ratelimit`, `ipnet`, `json`, `strings`,
  `lists`, `maps`, `convert`, `regex`, `time`, `httpd` - all bundled, so no new
  external dependency.
- Needs the default `jennifer` binary (`web` needs `net`); it carries
  `# pragma-jennifer-capability: net` and is unavailable under `jennifer-tiny`.

See also: [`web`](web.md), [`validate`](validate.md), [`json`](../libraries/json.md).

## API reference

Every exported function, struct and constant of `webapi`, with its signature, generated
from the module's doc comments: [the `webapi` API reference](../api/webapi.md).
