# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.26.0

/**
 * A thin GraphQL client over `http` / `rest`. Point a `graphql.Client` at one
 * endpoint URL, then `graphql.query(client, query, variables)` POSTs
 * `{"query": ..., "variables": ...}` and returns the decoded JSON response as a
 * `json.Value` (the result is under `/data`).
 *
 * The query is an **opaque string** the caller supplies - GraphQL syntax is the
 * server's job, not this module's - and a mutation is just a query string, so no
 * separate verb is needed. The one GraphQL-specific rule this client gets right:
 * a GraphQL execution error is an **HTTP 200 with a top-level `errors` array**,
 * not a non-2xx status, so `query` inspects the payload and raises a
 * `graphql.GraphqlError` (kind `"graphql"`) carrying the server's messages
 * rather than trusting the status line. A non-2xx response carrying such an
 * envelope (a 4xx for an invalid query, under the GraphQL-over-HTTP media type)
 * is treated the same; any other non-2xx is raised with the status and the
 * start of the body. Every error of kind `"graphql"` is a `GraphqlError`:
 * `reason` says which case it is, and `status`, `codes` (each error's
 * `extensions.code`) and `errors` (the server's array) carry the details.
 *
 * For a document that defines several named operations, `queryNamed` /
 * `tryQueryNamed` add the `operationName` selector. To handle GraphQL errors
 * yourself instead of catching a throw - branching on an error `code`, reading
 * partial data - `tryQuery` / `tryQueryNamed` return the raw envelope (both
 * `/data` and `/errors`) and raise only on an HTTP failure with no envelope;
 * `graphql.hasErrors` and `graphql.errorMessages` inspect it.
 *
 * The `Client` wraps a `rest.Client`, so it carries the endpoint URL, per-request
 * headers, and (via `rest`'s `http.TlsOptions`) TLS settings for a self-signed or
 * private-CA host. Default `jennifer` binary only (net-backed via `http` /
 * `rest`).
 * @module graphql
 * @example
 * def gql as graphql.Client init graphql.bearer(graphql.client("https://api.example.com/graphql"), $token);
 * def vars as json.Value init json.set(json.map(), "/login", "octocat");
 * def resp as json.Value init graphql.query($gql, "query($login:String!){user(login:$login){name}}", $vars);
 * io.printf("%s\n", json.asString($resp, "/data/user/name"));
 */
use json;
use strings;
use regex;
import "./http.j" as http;
import "./rest.j" as rest;

/**
 * A failed GraphQL operation. Every error of kind `"graphql"` is one, so its
 * fields are safe to read once the kind matches.
 * @field kind {string} always `"graphql"`
 * @field message {string} the server's messages (each with its code in brackets), or the HTTP status and the start of the body
 * @field reason {string} `"graphql"` (the server answered with an `errors` array, at any status), `"http"` (a non-2xx without one), or `"json"` (a 2xx whose body is not JSON)
 * @field status {int} the HTTP status
 * @field codes {list of string} each error's `extensions.code` (such as `UNAUTHENTICATED`), "" for one without; empty unless `reason` is `"graphql"`
 * @field errors {json.Value} the server's `errors` array, with `message`, `path`, `locations` and `extensions`; JSON `null` unless `reason` is `"graphql"`
 * @example
 * try {
 *     def r as json.Value init graphql.query($gql, $q, json.map());
 * } catch (e) {
 *     if ($e.kind == "graphql" and lists.contains($e.codes, "UNAUTHENTICATED")) {
 *         io.printf("log in again\n");
 *     } else {
 *         throw $e;
 *     }
 * }
 */
export def struct GraphqlError {
    kind as string,
    message as string,
    reason as string,
    status as int,
    codes as list of string,
    errors as json.Value
};

/**
 * A GraphQL client: an endpoint URL plus headers and TLS options, carried in a
 * wrapped `rest.Client`. Build one with `graphql.client`, then layer auth / TLS
 * with the builders (each returns a new `Client`; value semantics, no mutation).
 * @field rest {rest.Client} the underlying REST client (endpoint, headers, TLS)
 */
export def struct Client {
    rest as rest.Client
};

/**
 * A GraphQL client for one endpoint URL. The full endpoint is POSTed to verbatim
 * (no path is appended), so pass the complete GraphQL URL (e.g.
 * `https://host/graphql`).
 * @param endpoint {string} the GraphQL endpoint URL
 * @return {Client} a client with no auth and default (verifying) TLS
 */
export func client(endpoint as string) {
    return Client{rest: rest.client($endpoint)};
}

/**
 * Return a copy of the client that sends `Authorization: Bearer <token>`.
 * @param c {Client} the client
 * @param token {string} the bearer token
 * @return {Client} a new client with the header set
 */
export func bearer(c as Client, token as string) {
    def nc as Client init $c;
    $nc.rest = rest.withHeader($nc.rest, "Authorization", rest.bearer($token));
    return $nc;
}

/**
 * Return a copy of the client that sends HTTP Basic `Authorization`.
 * @param c {Client} the client
 * @param user {string} the username
 * @param pass {string} the password
 * @return {Client} a new client with the header set
 */
export func basic(c as Client, user as string, pass as string) {
    def nc as Client init $c;
    $nc.rest = rest.withHeader($nc.rest, "Authorization", rest.basic($user, $pass));
    return $nc;
}

/**
 * Return a copy of the client with an arbitrary request header set.
 * @param c {Client} the client
 * @param name {string} the header name
 * @param value {string} the header value
 * @return {Client} a new client with the header set
 */
export func header(c as Client, name as string, value as string) {
    def nc as Client init $c;
    $nc.rest = rest.withHeader($nc.rest, $name, $value);
    return $nc;
}

/**
 * Return a copy of the client that trusts a private-CA / self-signed certificate
 * (PEM). Certificate verification stays on, against this CA.
 * @param c {Client} the client
 * @param pem {bytes} the CA certificate in PEM form
 * @return {Client} a new client with the CA trusted
 */
export func withCA(c as Client, pem as bytes) {
    def nc as Client init $c;
    $nc.rest = rest.withCA($nc.rest, $pem);
    return $nc;
}

/**
 * Return a copy of the client that skips TLS certificate verification. Insecure -
 * for a trusted network or a local test server only.
 * @param c {Client} the client
 * @return {Client} a new client that accepts any server certificate
 */
export func insecure(c as Client) {
    def nc as Client init $c;
    $nc.rest = rest.insecure($nc.rest);
    return $nc;
}

/**
 * Return a copy of the client with a per-request idle timeout in milliseconds
 * (0 = the 30 s default). It bounds each read, so a hung server fails rather
 * than blocking.
 * @param c {Client} the client
 * @param timeoutMs {int} the per-read idle timeout in milliseconds
 * @return {Client} a new client with the timeout set
 */
export func withTimeout(c as Client, timeoutMs as int) {
    def nc as Client init $c;
    $nc.rest = rest.withTimeout($nc.rest, $timeoutMs);
    return $nc;
}

/**
 * Return a copy of the client that retries a **query** answered 429 or 5xx up
 * to `maxRetries` times, with exponential backoff honouring a numeric
 * `Retry-After`. A mutation or subscription is never retried: every GraphQL
 * operation is a POST, and repeating a write the server may have done is not
 * safe. A document counts as a query when it starts with `{` or `query`, or
 * when `operationName` names a `query` operation in it; one that starts with
 * a fragment definition is not retried.
 * @param c {Client} the client
 * @param maxRetries {int} the retry limit for queries
 * @return {Client} a new client that retries queries
 */
export func withRetries(c as Client, maxRetries as int) {
    def nc as Client init $c;
    $nc.rest = rest.withRetries($nc.rest, $maxRetries);
    return $nc;
}

/**
 * Return a copy of the client with a base retry backoff in milliseconds
 * (doubled each attempt; 0 = the 250 ms default). Pairs with `withRetries`.
 * @param c {Client} the client
 * @param backoffMs {int} the base backoff in milliseconds
 * @return {Client} a new client with the backoff set
 */
export func withBackoff(c as Client, backoffMs as int) {
    def nc as Client init $c;
    $nc.rest = rest.withBackoff($nc.rest, $backoffMs);
    return $nc;
}

# IGNORED matches what may precede the first definition of a document:
# whitespace, commas (insignificant in GraphQL), a byte-order mark and
# `#` comments.
def const IGNORED as string init '^(?:[\s,\x{FEFF}]|#[^\n]*)*';

# NOISE matches what may hold text that looks like a definition but is not
# one: a block string, a string, a comment. One alternation, so the leftmost
# wins and a "#" inside a string or a quote inside a comment is read right.
def const NOISE as string init '"""(?s:.*?)"""|"(?:[^"\\\n]|\\.)*"|#[^\n]*';

# isQuery reports whether the operation a request runs is a query, the only
# kind that is safe to send twice. With an operationName it finds that
# operation's definition, once strings and comments are blanked out; without
# one, it looks at how the document begins (a bare selection set is a query).
# Anything it cannot tell is not a query.
func isQuery(query as string, operationName as string) {
    if (len($operationName) > 0) {
        if (not regex.matches('^[_A-Za-z][_0-9A-Za-z]*$', $operationName)) {
            return false;
        }
        def bare as string init regex.replace(NOISE, $query, " ");
        def found as list of regex.Match init regex.findAll(
            '(?:^|[^_0-9A-Za-z])(query|mutation|subscription)\s+' + $operationName +
                '(?:[^_0-9A-Za-z]|$)',
            $bare);
        # Exactly one definition by that name, and it is a query.
        return len($found) == 1 and $found[0].groups[0] == "query";
    }
    return regex.matches(IGNORED + '(?:\{|query(?:[^_0-9A-Za-z]|$))', $query);
}

# buildRequest assembles the GraphQL request body as a json.Value: `{"query":
# ..., "variables": ...}`, plus `"operationName"` only when a non-empty name is
# given (needed when the document defines several named operations). variables is
# any json.Value (an empty object for none).
func buildRequest(query as string, variables as json.Value, operationName as string) {
    def req as json.Value init json.map();
    $req = json.set($req, "/query", $query);
    $req = json.set($req, "/variables", $variables);
    if (len($operationName) > 0) {
        $req = json.set($req, "/operationName", $operationName);
    }
    return $req;
}

/**
 * Report whether a decoded response carries a non-empty top-level `errors` array
 * - the GraphQL error signal, which the spec delivers on an HTTP 200. Use this on
 * the envelope returned by `tryQuery` to branch without a `try` / `catch`.
 * @param resp {json.Value} a decoded GraphQL response
 * @return {bool} true if the response reports one or more GraphQL errors
 */
export func hasErrors(resp as json.Value) {
    return json.typeOf($resp, "/errors") == "list" and json.length($resp, "/errors") > 0;
}

/**
 * Join the `message` field of each entry in a response's `errors` array into one
 * `"; "`-separated string. The `GraphqlError` that `query` raises carries the same
 * messages, each followed by its `extensions.code` in brackets when it has one
 * (see `errorCodes`). For structured error data - `extensions.code`, `path`, `locations` - read the
 * `errors` array of the envelope directly with the `json` accessors, e.g.
 * `json.asString($resp, "/errors/0/extensions/code")`.
 * @param resp {json.Value} a decoded GraphQL response with an `errors` array
 * @return {string} the joined error messages
 */
export func errorMessages(resp as json.Value) {
    def n as int init json.length($resp, "/errors");
    def msgs as list of string init [];
    def i as int init 0;
    while ($i < $n) {
        def mp as string init "/errors/{$i}/message";
        if (json.typeOf($resp, $mp) == "string") {
            $msgs[] = json.asString($resp, $mp);
        } else {
            $msgs[] = "(no message)";
        }
        $i = $i + 1;
    }
    return strings.join($msgs, "; ");
}

/**
 * Return each error's `extensions.code` (the machine-readable reason a server
 * gives, such as `UNAUTHENTICATED` or `FORBIDDEN`), in order, with "" for an
 * error that has none. Empty when the response has no `errors` array.
 * @param resp {json.Value} a decoded GraphQL response
 * @return {list of string} the codes, one per error
 */
export func errorCodes(resp as json.Value) {
    def codes as list of string init [];
    if (json.typeOf($resp, "/errors") != "list") {
        return $codes;
    }
    def n as int init json.length($resp, "/errors");
    def i as int init 0;
    while ($i < $n) {
        def cp as string init "/errors/{$i}/extensions/code";
        if (json.typeOf($resp, $cp) == "string") {
            $codes[] = json.asString($resp, $cp);
        } else {
            $codes[] = "";
        }
        $i = $i + 1;
    }
    return $codes;
}

# describeErrors is the text of a raised error: each message, followed by its
# code in brackets when it has one ("denied [FORBIDDEN]").
func describeErrors(resp as json.Value) {
    def n as int init json.length($resp, "/errors");
    def codes as list of string init errorCodes($resp);
    def parts as list of string init [];
    def i as int init 0;
    while ($i < $n) {
        def mp as string init "/errors/{$i}/message";
        def text as string init "(no message)";
        if (json.typeOf($resp, $mp) == "string") {
            $text = json.asString($resp, $mp);
        }
        if ($codes[$i] != "") {
            $text = $text + " [" + $codes[$i] + "]";
        }
        $parts[] = $text;
        $i = $i + 1;
    }
    def text as string init flatten(strings.join($parts, "; "));
    if (len($text) > MAX_ERRORS_TEXT) {
        return $text[..MAX_ERRORS_TEXT] + "...";
    }
    return $text;
}

# MAX_SNIPPET bounds how much of a non-GraphQL error body goes into a message;
# MAX_ERRORS_TEXT bounds the joined `errors` messages of a raised error.
def const MAX_SNIPPET as int init 200;
def const MAX_ERRORS_TEXT as int init 2000;

# flatten turns runs of control characters into one space, so text from the
# server cannot forge extra lines in a log.
func flatten(text as string) {
    return strings.trim(regex.replace('[\x00-\x1f\x7f]+', $text, " "));
}

# snippet shortens an error body for a message: flattened, and cut past
# MAX_SNIPPET characters, so a proxy's whole HTML page does not land in the
# error.
func snippet(body as string) {
    def flat as string init flatten($body);
    if (len($flat) > MAX_SNIPPET) {
        return $flat[..MAX_SNIPPET] + "...";
    }
    return $flat;
}

# envelopeOf decodes a body as a GraphQL envelope: a JSON object with a
# non-empty `errors` list. It reports false for anything else.
func envelopeOf(body as string) {
    try {
        def v as json.Value init json.decode($body);
        return json.typeOf($v, "") == "map" and hasErrors($v);
    } catch (e) {
        return false;
    }
}

# run posts one operation and decodes the response. It always raises on an HTTP
# failure (a non-2xx status: a transport / auth / server problem, with no GraphQL
# envelope to return). raiseOnErrors additionally raises on a GraphQL `errors`
# array (the `query` behaviour); when false, the envelope is returned as-is (the
# `tryQuery` behaviour) so the caller inspects the errors itself.
func run(
    c as Client,
    query as string,
    variables as json.Value,
    operationName as string,
    raiseOnErrors as bool) {
    def req as json.Value init buildRequest($query, $variables, $operationName);
    def headers as map of string to string init $c.rest.headers;
    $headers["Content-Type"] = "application/json";
    # The GraphQL-over-HTTP media type first; a server that knows it answers a
    # bad request with a 4xx and a normal errors envelope.
    if (not hasHeader($headers, "accept")) {
        $headers["Accept"] = "application/graphql-response+json, application/json";
    }
    # Only a query may be sent twice; see withRetries.
    def options as http.Options init $c.rest.options;
    if (not isQuery($query, $operationName)) {
        $options.maxRetries = 0;
    }
    # POST to the endpoint URL verbatim (no path joining), through the same
    # policy-aware send rest uses - so the wrapped client's TLS (withCA /
    # insecure) and the timeout / retry policy apply here too.
    def r as http.Response init http.send(
        "POST",
        $c.rest.baseUrl,
        $headers,
        json.encode($req),
        $options);
    if ($r.status < 200 or $r.status >= 300) {
        # A 4xx carrying a GraphQL envelope (an invalid query, a refused key)
        # is a GraphQL answer: tryQuery returns it, query raises its messages.
        if (envelopeOf($r.body)) {
            def env as json.Value init json.decode($r.body);
            if (not $raiseOnErrors) {
                return $env;
            }
            throw errorsOf("graphql: HTTP {$r.status}: " + describeErrors($env), $r.status, $env);
        }
        throw failure("http", "graphql: HTTP {$r.status}: " + snippet($r.body), $r.status);
    }
    # A 2xx body that is not JSON (e.g. an HTML error page from a proxy) would
    # raise an untyped json error; wrap it so the module kind stays "graphql".
    def resp as json.Value;
    try {
        $resp = json.decode($r.body);
    } catch (e) {
        throw failure("json", "graphql: response body is not valid JSON", $r.status);
    }
    if ($raiseOnErrors and hasErrors($resp)) {
        throw errorsOf("graphql: " + describeErrors($resp), $r.status, $resp);
    }
    return $resp;
}

# failure is the GraphqlError of an answer without an errors array.
func failure(reason as string, message as string, status as int) {
    return GraphqlError{
        kind: "graphql",
        message: $message,
        reason: $reason,
        status: $status,
        codes: [],
        errors: json.decode("null")
    };
}

# errorsOf is the GraphqlError of an answer with an errors array.
func errorsOf(message as string, status as int, env as json.Value) {
    return GraphqlError{
        kind: "graphql",
        message: $message,
        reason: "graphql",
        status: $status,
        codes: errorCodes($env),
        errors: json.get($env, "/errors")
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

/**
 * Run a GraphQL operation (query or mutation) and return the decoded JSON
 * response as a `json.Value`; the result is at `/data`. The `variables` is a
 * `json.Value` object (use an empty `json.map()` when the operation takes none).
 *
 * Raises a `GraphqlError` (kind `"graphql"`) when the server reports GraphQL
 * execution errors (an HTTP 200 with a top-level `errors` array - the messages
 * are collected into the error), or when the request fails at the HTTP level (a
 * non-2xx status without an envelope, carrying the status and the start of the
 * body). A successful return therefore
 * has no `errors`, so `/data` is present and complete. The error's `codes` and
 * `errors` fields carry each error's code and the server's array; to read
 * partial data alongside the errors, use `tryQuery`.
 * @param c {Client} the client
 * @param query {string} the GraphQL query or mutation document
 * @param variables {json.Value} the variables object (empty object for none)
 * @return {json.Value} the full decoded response (data under `/data`)
 * @throws {GraphqlError} kind "graphql" on GraphQL errors, a non-2xx HTTP status or a non-JSON body
 */
export func query(c as Client, query as string, variables as json.Value) {
    return run($c, $query, $variables, "", true);
}

/**
 * Like `query`, but for a document that defines several named operations: the
 * `operationName` selects which one to run (some servers require it when more
 * than one is present).
 * @param c {Client} the client
 * @param query {string} the GraphQL document
 * @param variables {json.Value} the variables object (empty object for none)
 * @param operationName {string} the name of the operation to execute
 * @return {json.Value} the full decoded response (data under `/data`)
 * @throws {GraphqlError} kind "graphql" on GraphQL errors, a non-2xx HTTP status or a non-JSON body
 */
export func queryNamed(
    c as Client,
    query as string,
    variables as json.Value,
    operationName as string) {
    return run($c, $query, $variables, $operationName, true);
}

/**
 * Run an operation and return the decoded response **without raising on GraphQL
 * errors** - the envelope comes back with both `/data` (possibly partial) and
 * `/errors`, so you can inspect the errors yourself (`graphql.hasErrors($resp)`,
 * `graphql.errorMessages($resp)`, or `json.asString($resp, "/errors/0/extensions/code")`
 * for structured data). A non-2xx response with an `errors` envelope is returned
 * too; one without (a proxy page, a 502) still raises, since there is nothing to
 * return.
 * @param c {Client} the client
 * @param query {string} the GraphQL query or mutation document
 * @param variables {json.Value} the variables object (empty object for none)
 * @return {json.Value} the full decoded response (data under `/data`, errors under `/errors`)
 * @throws {GraphqlError} kind "graphql" only on a non-2xx HTTP status without an errors array, or a non-JSON body
 */
export func tryQuery(c as Client, query as string, variables as json.Value) {
    return run($c, $query, $variables, "", false);
}

/**
 * `tryQuery` (no raise on GraphQL errors) for a document with several named
 * operations, selected by `operationName`.
 * @param c {Client} the client
 * @param query {string} the GraphQL document
 * @param variables {json.Value} the variables object (empty object for none)
 * @param operationName {string} the name of the operation to execute
 * @return {json.Value} the full decoded response (data under `/data`, errors under `/errors`)
 * @throws {GraphqlError} kind "graphql" only on a non-2xx HTTP status without an errors array, or a non-JSON body
 */
export func tryQueryNamed(
    c as Client,
    query as string,
    variables as json.Value,
    operationName as string) {
    return run($c, $query, $variables, $operationName, false);
}
