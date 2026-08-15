# `webapi` - a JSON-API conventions layer over `web`

`webapi` sits on top of the bundled [`web`](web.md) module and supplies the
pieces every JSON API otherwise re-implements by hand: a uniform error envelope,
request validation (reusing [`validate`](validate.md)), versioned route mounting,
pluggable bearer auth and rate limiting, content negotiation, and pagination.
`web` stays the HTTP framework; `webapi` carries the JSON-API opinions, so an
application can take one without the other.

```jennifer
import "web.j" as web;
import "webapi.j" as webapi;

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
web.run($app, ":8080");
```

Runnable: [`examples/modules/webapi_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/webapi_demo.j).

## The builder

`webapi.new()` returns a value-semantic `Api` you finish before the server runs;
every builder returns a fresh `Api`.

| Call | Does |
| ---- | ---- |
| `webapi.mount(a, version, path)` | serve the routes under `path` (labelled `version`) |
| `webapi.alias(a, version)` | additionally serve `version` at the bare root |
| `webapi.deprecate(a, version, sunset)` | mark a version deprecated with a date |
| `webapi.authenticator(a, fn)` | the token-verifier `func` value |
| `webapi.limiter(a, fn)` | the rate-limit counter `func` value |
| `webapi.get/post/put/patch/delete(a, pattern, handler, spec)` | register a route (`handler` a `func` value) with a `Spec` |
| `webapi.feature(a, name)` | label the last route for the discovery document |
| `webapi.install(a, app, guard)` | register every route (once per mount) + wire the guard `func` value |

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
`install` wires the enforcement as a `before` middleware whose `func` value you
supply, backed by a one-line entry-program shim that binds the `Api`:

```jennifer
func apiGuard(ctx as web.Context) { return webapi.guard($api, $ctx); }
```

The guard, per request, matches the route, authenticates, checks scopes,
validates the body, and rate-limits - answering the request and returning false
on any failure, or true to run the handler. A request matching no API route
passes through untouched.

The authenticator and limiter are entry-program `func` values, called through
their home interpreter (so the authenticator can construct and return a
`webapi.Identity`). Their signatures:

```jennifer
func verifyToken(token as string) -> webapi.Identity        # ok:false = reject
func countHit(key as string, limit as int) -> bool           # true = allowed
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
| `webapi.onError(app, name)` | register a `500`-envelope error handler |

## Request data

The guard validates a route's `rules` before the handler runs (a failure
short-circuits to `422`), so a handler never has to ask whether its input is
sane. Data is read from the query, then the JSON body's top-level scalars, then
the form body.

| Call | Does |
| ---- | ---- |
| `webapi.queryData(ctx, fields)` | named query parameters as a string map |
| `webapi.formData(ctx)` | the form body as a string map |
| `webapi.jsonData(ctx, fields)` | named top-level JSON fields, flattened to strings |
| `webapi.validated(a, ctx)` | inside a handler: the checked data for this request |
| `webapi.identity(a, ctx)` | inside a handler: the authenticated caller |

A nested or array-valued body is the handler's own business, read with
`web.bodyJson`.

## Negotiation, pagination, discovery

| Call | Does |
| ---- | ---- |
| `webapi.wants(ctx)` | `"json"` or `"html"` from `Accept` |
| `webapi.sendJson(ctx, status, value)` | send a `json.Value` |
| `webapi.page(ctx, defaultLimit, maxLimit)` | parse + clamp `offset`/`limit` -> `Page` |
| `webapi.sendPage(ctx, items, page, total)` | a `{items, offset, limit, total}` response |
| `webapi.discovery(a, registry, specVersion)` | a discovery `json.Value` derived from the route table |

`page` clamps `limit` to `maxLimit` so a client cannot ask for the whole table.
`discovery` builds its `apis[]` and `features[]` from what is actually
registered, so the advertised surface cannot drift from the routes that exist.

## Placement

- Depends on `web`, `validate`, `json`, `strings`, `lists`, `maps`, `convert`,
  `meta` - all bundled, so no new external dependency.
- Needs the default `jennifer` binary (`web` needs `net`); it carries
  `# pragma-jennifer-capability: net` and is unavailable under `jennifer-tiny`.

See also: [`web`](web.md), [`validate`](validate.md), [`json`](../libraries/json.md).
