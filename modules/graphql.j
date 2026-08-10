# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
# pragma-jennifer-version: >=0.24.0

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
 * not a non-2xx status, so `query` inspects the payload and raises a positioned
 * `Error` (kind `"graphql"`) carrying the server's messages rather than trusting
 * the status line. A genuine transport / auth failure (a non-2xx status) is also
 * raised as a `graphql` error with the status and body.
 *
 * For a document that defines several named operations, `queryNamed` /
 * `tryQueryNamed` add the `operationName` selector. To handle GraphQL errors
 * yourself instead of catching a throw - branching on an error `code`, reading
 * partial data - `tryQuery` / `tryQueryNamed` return the raw envelope (both
 * `/data` and `/errors`) and only raise on an HTTP-level failure;
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
use convert;
use strings;
import "./http.j" as http;
import "./rest.j" as rest;

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
    return json.has($resp, "/errors") and json.typeOf($resp, "/errors") == "list" and
        json.length($resp, "/errors") > 0;
}

/**
 * Join the `message` field of each entry in a response's `errors` array into one
 * `"; "`-separated string (this is the text `query` puts in the raised `Error`).
 * For structured error data - `extensions.code`, `path`, `locations` - read the
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
        def mp as string init "/errors/" + convert.toString($i) + "/message";
        if (json.has($resp, $mp) and json.typeOf($resp, $mp) == "string") {
            $msgs[] = json.asString($resp, $mp);
        } else {
            $msgs[] = "(no message)";
        }
        $i = $i + 1;
    }
    return strings.join($msgs, "; ");
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
    # POST to the endpoint URL verbatim (no path joining), through the same
    # policy-aware send rest uses - so the wrapped client's TLS (withCA /
    # insecure) and any redirect / retry / timeout policy apply here too.
    def r as http.Response init http.send(
        "POST",
        $c.rest.baseUrl,
        $headers,
        json.encode($req),
        $c.rest.options);
    if ($r.status < 200 or $r.status >= 300) {
        throw Error{
            kind: "graphql",
            message: "graphql: HTTP " + convert.toString($r.status) + ": " + $r.body,
            file: "",
            line: 0,
            col: 0
        };
    }
    # A 2xx body that is not JSON (e.g. an HTML error page from a proxy) would
    # raise an untyped json error; wrap it so the module kind stays "graphql".
    def resp as json.Value;
    try {
        $resp = json.decode($r.body);
    } catch (e) {
        throw Error{
            kind: "graphql",
            message: "graphql: response body is not valid JSON",
            file: "",
            line: 0,
            col: 0
        };
    }
    if ($raiseOnErrors and hasErrors($resp)) {
        throw Error{
            kind: "graphql",
            message: "graphql: " + errorMessages($resp),
            file: "",
            line: 0,
            col: 0
        };
    }
    return $resp;
}

/**
 * Run a GraphQL operation (query or mutation) and return the decoded JSON
 * response as a `json.Value`; the result is at `/data`. The `variables` is a
 * `json.Value` object (use an empty `json.map()` when the operation takes none).
 *
 * Raises a positioned `Error` (kind `"graphql"`) when the server reports GraphQL
 * execution errors (an HTTP 200 with a top-level `errors` array - the messages
 * are collected into the error), or when the request fails at the HTTP level (a
 * non-2xx status, carrying the status and body). A successful return therefore
 * has no `errors`, so `/data` is present and complete. To handle GraphQL errors
 * yourself (e.g. branch on an error `code`), use `tryQuery`.
 * @param c {Client} the client
 * @param query {string} the GraphQL query or mutation document
 * @param variables {json.Value} the variables object (empty object for none)
 * @return {json.Value} the full decoded response (data under `/data`)
 * @throws {Error} kind "graphql" on GraphQL errors or a non-2xx HTTP status
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
 * @throws {Error} kind "graphql" on GraphQL errors or a non-2xx HTTP status
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
 * for structured data). An HTTP-level failure (a non-2xx status) still raises,
 * since there is no GraphQL envelope to return.
 * @param c {Client} the client
 * @param query {string} the GraphQL query or mutation document
 * @param variables {json.Value} the variables object (empty object for none)
 * @return {json.Value} the full decoded response (data under `/data`, errors under `/errors`)
 * @throws {Error} kind "graphql" only on a non-2xx HTTP status
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
 * @throws {Error} kind "graphql" only on a non-2xx HTTP status
 */
export func tryQueryNamed(
    c as Client,
    query as string,
    variables as json.Value,
    operationName as string) {
    return run($c, $query, $variables, $operationName, false);
}
