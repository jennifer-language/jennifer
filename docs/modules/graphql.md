# `graphql` - a thin GraphQL client

Import with `import "graphql.j" as graphql;`. A small GraphQL client over
[`http`](http.md) / [`rest`](rest.md): point a `Client` at one endpoint, then
`query` POSTs `{"query": ..., "variables": ...}` and returns the decoded JSON
response as a `json.Value`. **Default `jennifer` binary only** (net-backed).

```jennifer
import "graphql.j" as graphql;
use json;
use io;

def gql as graphql.Client init graphql.bearer(
    graphql.client("https://api.github.com/graphql"), $token);

def vars as json.Value init json.set(json.map(), "/login", "octocat");
def resp as json.Value init graphql.query($gql,
    "query($login:String!){ user(login:$login){ name } }", $vars);

io.printf("%s\n", json.asString($resp, "/data/user/name"));
```

The query is an **opaque string** you supply - GraphQL syntax is the server's
job, not this module's - and a mutation is just a query string, so there is no
separate verb. `variables` is a `json.Value` object (use an empty `json.map()`
when the operation takes none).

Runnable: [`examples/modules/graphql_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/graphql_demo.j).

## The client

`graphql.Client` wraps a [`rest.Client`](rest.md), so it carries the endpoint
URL, per-request headers, and TLS options. It is value-semantic: each builder
returns a **new** client, nothing is mutated.

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `graphql.client(endpoint)` | `Client` | A client for one endpoint URL. The URL is POSTed to verbatim - pass the complete GraphQL URL (e.g. `https://host/graphql`); no path is appended. |
| `graphql.bearer(c, token)` | `Client` | Add `Authorization: Bearer <token>`. |
| `graphql.basic(c, user, pass)` | `Client` | Add HTTP Basic `Authorization`. |
| `graphql.header(c, name, value)` | `Client` | Set an arbitrary request header. |
| `graphql.withCA(c, pem)` | `Client` | Trust a private-CA / self-signed certificate (PEM); verification stays on, against this CA. |
| `graphql.insecure(c)` | `Client` | Skip TLS verification - a trusted network or a local test server only. |

Builders compose left to right - each takes a `Client` and returns one:

```jennifer
def gql as graphql.Client init graphql.header(
    graphql.bearer(graphql.client("https://host/graphql"), $token),
    "X-Api-Version", "2");
```

## Running a query

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `graphql.query(c, query, variables)` | `json.Value` | POST the operation; raise on GraphQL / HTTP errors; return the full decoded response (result under `/data`). |
| `graphql.queryNamed(c, query, variables, operationName)` | `json.Value` | `query`, selecting one operation from a multi-operation document by name. |
| `graphql.tryQuery(c, query, variables)` | `json.Value` | Like `query` but does **not** raise on GraphQL errors - returns the envelope so you inspect `/errors` yourself. Still raises on a non-2xx HTTP status. |
| `graphql.tryQueryNamed(c, query, variables, operationName)` | `json.Value` | `tryQuery` with an `operationName`. |
| `graphql.hasErrors(resp)` | `bool` | Whether a returned envelope carries a non-empty `errors` array. |
| `graphql.errorMessages(resp)` | `string` | The envelope's error `message`s, joined with `; `. |

```jennifer
def resp as json.Value init graphql.query($gql, "{ viewer { login } }", json.map());
def login as string init json.asString($resp, "/data/viewer/login");
```

The response is the whole GraphQL envelope, so read the payload under `/data`
(and `/data/<field>/...`) with the [`json`](../libraries/json.md) accessors.

`variables` is a `json.Value` object - pass an empty `json.map()` for an
operation that takes none.

### Named operations

When a document defines more than one named operation, pass the one to run as
`operationName` (some servers require it in that case):

```jennifer
def q as string init "query Me { viewer { login } } query Repos { viewer { repositories { totalCount } } }";
def resp as json.Value init graphql.queryNamed($gql, $q, json.map(), "Repos");
```

A single-operation document needs no name - use plain `query` / `tryQuery`.

## Errors

GraphQL has an unusual error convention that this client handles for you: an
**execution error is an HTTP 200 with a top-level `errors` array**, not a non-2xx
status. `query` inspects the payload rather than trusting the status line and
raises a positioned `Error` (kind `"graphql"`) in two cases:

- **GraphQL errors** - the response carries a non-empty `errors` array. The
  error's `message` is `graphql: ` followed by every entry's `message`, joined
  with `; `. (A response with partial `data` *and* `errors` still raises - the
  errors are not silently dropped.)
- **HTTP failure** - a non-2xx status (a transport, auth, or server problem). The
  message carries the status code and the response body.

A successful return therefore has no `errors`, so `/data` is present and
complete. Catch with `try` / `catch`:

```jennifer
try {
    def resp as json.Value init graphql.query($gql, $q, $vars);
    # ... use $resp ...
} catch (e) {
    io.eprintf("query failed: %s\n", $e.message);   # e.kind == "graphql"
}
```

### Handling GraphQL errors yourself

The thrown `Error` carries only the joined message text. When you need the
**structured** error - to branch on an error `code`, read partial `data`, or see
`path` / `locations` - use `tryQuery`, which returns the raw envelope instead of
raising on GraphQL errors (it still raises on a non-2xx HTTP status, where there
is no envelope):

```jennifer
def resp as json.Value init graphql.tryQuery($gql, $q, $vars);
if (graphql.hasErrors($resp)) {
    def code as string init json.asString($resp, "/errors/0/extensions/code");
    if ($code == "RATE_LIMITED") {
        # ... back off and retry ...
    } else {
        io.eprintf("graphql: %s\n", graphql.errorMessages($resp));
    }
} else {
    def login as string init json.asString($resp, "/data/viewer/login");
    # ...
}
```

Everything under `/errors` is reachable with the ordinary `json` accessors, since
the envelope is a plain `json.Value`.

## TLS to a self-signed host

Because the client wraps `rest`, the TLS builders reach a homelab / private-CA
endpoint (Hasura, a self-hosted API):

```jennifer
def pem as bytes init fs.readBytes("ca.pem");
def gql as graphql.Client init graphql.withCA(
    graphql.client("https://hasura.home.lan/v1/graphql"), $pem);
```

Use `graphql.insecure(c)` only on a trusted network or against a local test
server - it accepts any certificate.

## See also

- [rest.md](rest.md) - the REST layer the client wraps (auth + TLS options).
- [http.md](http.md) - the underlying HTTP/1.1 client.
- [json.md](../libraries/json.md) - reading the response and building `variables`.
- [modules/index.md](index.md) - the module catalog and import rules.
