# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
# pragma-jennifer-version: >=0.25.0

/**
 * An InfluxDB client over `http` for both major API generations: write
 * measurements as line-protocol points and run queries, getting back a parsed
 * result. The push counterpart to a scrape.
 *
 * Two selectable backends under one `Client` (stance 1: one module, one
 * version discriminator). A **1.x** client (`client` / `clientWith`) writes to
 * `/write?db=...` with optional Basic auth and queries InfluxQL over `/query`
 * (`query`). A **2.x / 3.x** client (`client2`) writes to
 * `/api/v2/write?org=...&bucket=...` with `Authorization: Token <token>` auth
 * and queries Flux over `/api/v2/query` (`queryFlux`). `write` dispatches on the
 * client's `version`, so the same call serves both; the line protocol is
 * identical across versions, so `line` is reused verbatim.
 *
 * A `Point` is built with value-semantic builders (`point` / `tag` / `field` /
 * `intField` / `stringField` / `boolField` / `at`), each returning a fresh
 * `Point`, then rendered to a line-protocol line by `line` (or sent by
 * `write`). Field types are carried as pre-rendered fragments, so one point can
 * mix float / int / string / bool fields despite Jennifer's homogeneous maps.
 * Needs the default `jennifer` binary (`http` over `net`); a failed request
 * throws `Error{kind: "influxdb"}` (a 2.x client's token is redacted from it).
 * @module influxdb
 * @example
 * import "influxdb.j" as influxdb;
 * def db as influxdb.Client init influxdb.client("http://localhost:8086", "metrics");
 * def p as influxdb.Point init influxdb.field(
 *     influxdb.tag(influxdb.point("cpu"), "host", "server01"), "value", 0.64);
 * influxdb.write($db, [$p]);
 * def r as influxdb.Result init influxdb.query($db, "SELECT last(\"value\") FROM cpu");
 * # or a 2.x client:
 * def db2 as influxdb.Client init influxdb.client2("http://localhost:8086", "myorg", "mybucket", "mytoken");
 * influxdb.write($db2, [$p]);
 * def csv as string init influxdb.queryFlux($db2, "from(bucket:\"mybucket\") |> range(start:-1h)");
 */
use json;
use strings;
use convert;
use lists;
use time;
use encoding;
import "./http.j" as http;
import "./uri.j" as uri;

# The default InfluxDB HTTP endpoint (shared by 1.x and 2.x).
def const DEFAULT_URL as string init "http://localhost:8086";

/**
 * The API generation a `Client` targets: `influxdb.Version.V1` (InfluxDB 1.x -
 * `/write?db=...` + InfluxQL, optional Basic auth) or `influxdb.Version.V2`
 * (InfluxDB 2.x / 3.x - `/api/v2/write?org=...&bucket=...` + Flux, token auth).
 * The backend selector `write` dispatches on; the zero value is `V1`.
 */
export def enum Version { V1, V2 };

/**
 * A client for either API generation. A 1.x client carries `db` and optional
 * Basic-auth `user` / `password`; a 2.x client carries `org` / `bucket` and a
 * `token`. `version` selects which set is used (unused fields stay "").
 * @field url {string} the base URL (e.g. "http://localhost:8086")
 * @field version {Version} the API generation (V1 or V2)
 * @field db {string} the 1.x database name ("" for a 2.x client)
 * @field user {string} the 1.x Basic-auth username ("" for no auth)
 * @field password {string} the 1.x Basic-auth password
 * @field org {string} the 2.x organization ("" for a 1.x client)
 * @field bucket {string} the 2.x bucket ("" for a 1.x client)
 * @field token {string} the 2.x API token ("" for a 1.x client)
 */
export def struct Client {
    url as string,
    version as Version,
    db as string,
    user as string,
    password as string,
    org as string,
    bucket as string,
    token as string
};

/**
 * A line-protocol point under construction. Tags and fields are held as
 * pre-rendered, escaped `key=value` fragments so a point can mix field types.
 * @field measurement {string} the measurement name
 * @field tags {list of string} escaped `key=value` tag fragments
 * @field fields {list of string} rendered `key=value` field fragments
 * @field timestamp {int} the timestamp in nanoseconds (when `timed`)
 * @field timed {bool} whether an explicit timestamp is set
 */
export def struct Point {
    measurement as string,
    tags as list of string,
    fields as list of string,
    timestamp as int,
    timed as bool
};

/**
 * One result series: its measurement name, its GROUP BY tag set, the column
 * names, and the rows (each cell stringified).
 * @field name {string} the measurement name
 * @field tags {map of string to string} the series tag set ({} if none)
 * @field columns {list of string} the column names (e.g. ["time", "value"])
 * @field values {list of list of string} the rows, one stringified cell per column
 */
export def struct Series {
    name as string,
    tags as map of string to string,
    columns as list of string,
    values as list of list of string
};

/**
 * A parsed query result: the flattened series across every statement.
 * @field series {list of Series} the result series
 */
export def struct Result {
    series as list of Series
};

func fail(msg as string) {
    throw Error{kind: "influxdb", message: "influxdb: " + $msg, file: "", line: 0, col: 0};
}

# --- clients (exported) -----------------------------------------------------

/**
 * Open a client to a database with no authentication.
 * @param url {string} the base URL (e.g. "http://localhost:8086")
 * @param db {string} the database name
 * @return {Client} a ready client
 */
export func client(url as string, db as string) {
    return clientWith($url, $db, "", "");
}

/**
 * Open a 1.x client with HTTP Basic-auth credentials.
 * @param url {string} the base URL
 * @param db {string} the database name
 * @param user {string} the username
 * @param password {string} the password
 * @return {Client} a ready client
 */
export func clientWith(url as string, db as string, user as string, password as string) {
    return Client{
        url: $url,
        version: Version.V1,
        db: $db,
        user: $user,
        password: $password,
        org: "",
        bucket: "",
        token: ""
    };
}

/**
 * Open an InfluxDB 2.x / 3.x client: token auth, addressing data by
 * organization + bucket. `write` posts to `/api/v2/write` and `queryFlux` runs
 * Flux over `/api/v2/query`.
 * @param url {string} the base URL (e.g. "http://localhost:8086")
 * @param org {string} the organization
 * @param bucket {string} the target bucket
 * @param token {string} the API token (sent as `Authorization: Token <token>`)
 * @return {Client} a ready 2.x client
 */
export func client2(url as string, org as string, bucket as string, token as string) {
    return Client{
        url: $url,
        version: Version.V2,
        db: "",
        user: "",
        password: "",
        org: $org,
        bucket: $bucket,
        token: $token
    };
}

# --- line-protocol escaping (private) ---------------------------------------

# escapeChars backslash-escapes every character of `s` that appears in
# `specials`. Characters collect and join once: growing a string with `+` per
# char is O(N^2) over a long value (a string field value can be a paragraph).
func escapeChars(s as string, specials as string) {
    def out as list of string init [];
    def cs as list of string init strings.chars($s);
    for (def ch in $cs) {
        if (strings.contains($specials, $ch)) {
            $out[] = "\\" + $ch;
        } else {
            $out[] = $ch;
        }
    }
    return strings.join($out, "");
}

# escapeMeasurement escapes a measurement name (comma and space, not equals).
func escapeMeasurement(s as string) {
    return escapeChars($s, ", ");
}

# escapeKey escapes a tag key / tag value / field key (comma, space, equals).
func escapeKey(s as string) {
    return escapeChars($s, ", =");
}

# escapeStringField renders a string field value: double-quoted, with `"` and
# `\` backslash-escaped.
func escapeStringField(s as string) {
    return "\"" + escapeChars($s, "\"\\") + "\"";
}

# --- point builders (exported) ----------------------------------------------

/**
 * Start a point for a measurement (no tags or fields yet).
 * @param measurement {string} the measurement name
 * @return {Point} a fresh point
 */
export func point(measurement as string) {
    def tags as list of string init [];
    def fields as list of string init [];
    return Point{
        measurement: $measurement,
        tags: $tags,
        fields: $fields,
        timestamp: 0,
        timed: false
    };
}

/**
 * Add a tag (indexed string metadata). Returns a fresh point.
 * @param p {Point} the point
 * @param key {string} the tag key
 * @param value {string} the tag value
 * @return {Point} a point with the tag added
 */
export func tag(p as Point, key as string, value as string) {
    def out as Point init $p;
    $out.tags = lists.push($out.tags, escapeKey($key) + "=" + escapeKey($value));
    return $out;
}

/**
 * Add a float field. Returns a fresh point.
 * @param p {Point} the point
 * @param key {string} the field key
 * @param value {float} the field value
 * @return {Point} a point with the field added
 */
export func field(p as Point, key as string, value as float) {
    def out as Point init $p;
    $out.fields = lists.push($out.fields, escapeKey($key) + "=" + convert.toString($value));
    return $out;
}

/**
 * Add an integer field (rendered with the line-protocol `i` suffix). Returns a
 * fresh point.
 * @param p {Point} the point
 * @param key {string} the field key
 * @param value {int} the field value
 * @return {Point} a point with the field added
 */
export func intField(p as Point, key as string, value as int) {
    def out as Point init $p;
    $out.fields = lists.push($out.fields, escapeKey($key) + "=" + convert.toString($value) + "i");
    return $out;
}

/**
 * Add a string field (double-quoted, escaped). Returns a fresh point.
 * @param p {Point} the point
 * @param key {string} the field key
 * @param value {string} the field value
 * @return {Point} a point with the field added
 */
export func stringField(p as Point, key as string, value as string) {
    def out as Point init $p;
    $out.fields = lists.push($out.fields, escapeKey($key) + "=" + escapeStringField($value));
    return $out;
}

/**
 * Add a boolean field. Returns a fresh point.
 * @param p {Point} the point
 * @param key {string} the field key
 * @param value {bool} the field value
 * @return {Point} a point with the field added
 */
export func boolField(p as Point, key as string, value as bool) {
    def out as Point init $p;
    def rendered as string init "false";
    if ($value) {
        $rendered = "true";
    }
    $out.fields = lists.push($out.fields, escapeKey($key) + "=" + $rendered);
    return $out;
}

/**
 * Set an explicit timestamp in nanoseconds since the Unix epoch. Returns a
 * fresh point.
 * @param p {Point} the point
 * @param unixNanos {int} the timestamp in nanoseconds
 * @return {Point} a timestamped point
 */
export func at(p as Point, unixNanos as int) {
    def out as Point init $p;
    $out.timestamp = $unixNanos;
    $out.timed = true;
    return $out;
}

/**
 * Set the timestamp from a `time.Time`. Returns a fresh point.
 * @param p {Point} the point
 * @param t {time.Time} the timestamp
 * @return {Point} a timestamped point
 */
export func atTime(p as Point, t as time.Time) {
    return at($p, time.unixNanos($t));
}

/**
 * Render a point to one line-protocol line.
 * @param p {Point} the point
 * @return {string} the line-protocol line
 * @throws {Error} kind "influxdb" if the point has no fields
 */
export func line(p as Point) {
    if (len($p.fields) == 0) {
        fail("point \"" + $p.measurement + "\" has no fields");
    }
    def out as string init escapeMeasurement($p.measurement);
    if (len($p.tags) > 0) {
        $out = $out + "," + strings.join($p.tags, ",");
    }
    $out = $out + " " + strings.join($p.fields, ",");
    if ($p.timed) {
        $out = $out + " " + convert.toString($p.timestamp);
    }
    # A newline anywhere in the rendered point splits its line and corrupts the
    # whole batch (the newline is the point separator, and line protocol has no
    # escape for it). Reject the point so it fails alone instead of silently
    # breaking every point in the write.
    if (strings.contains($out, "\n") or strings.contains($out, "\r")) {
        fail("point \"" + $p.measurement +
            "\" contains a newline in a measurement / tag / field value (not representable in line protocol)");
    }
    return $out;
}

# --- HTTP plumbing (private) ------------------------------------------------

# joinBase joins a base URL and a path with exactly one slash between them.
func joinBase(base as string, path as string) {
    if (strings.endsWith($base, "/")) {
        return strings.substring($base, 0, len($base) - 1) + $path;
    }
    return $base + $path;
}



# A built HTTP request: the URL, the Content-Type, the header map, and the
# body. The pure output of the request builders (buildWrite / buildQuery /
# buildFlux), so a test can assert the endpoint / params / auth / body without a
# live server.
def struct Req {
    url as string,
    contentType as string,
    headers as map of string to string,
    body as string
};

# authHeaders builds the auth header map for a client: Basic auth from
# user/password for a 1.x client (empty when no username), a `Token` header for
# a 2.x client (empty when no token).
func authHeaders(c as Client) {
    def h as map of string to string init {};
    match ($c.version) {
        when V1 {
            if (len($c.user) > 0) {
                def creds as string init $c.user + ":" + $c.password;
                def enc as string init encoding.toText(convert.bytesFromString($creds, "utf-8"), "base64");
                $h["Authorization"] = "Basic " + $enc;
            }
        }
        when V2 {
            if (len($c.token) > 0) {
                $h["Authorization"] = "Token " + $c.token;
            }
        }
    }
    return $h;
}

# redact scrubs a 2.x client's token out of an error message so a leaked
# request line or echoed header can never expose the secret. No-op for a 1.x
# client (its password never rides in a URL, so it does not leak).
func redact(c as Client, msg as string) {
    if (len($c.token) > 0) {
        return strings.replace($msg, $c.token, "<redacted>");
    }
    return $msg;
}

# buildWrite builds the write request for a client and a rendered line-protocol
# body: `/write?db=...` for a 1.x client, `/api/v2/write?org=...&bucket=...` for
# a 2.x client, both at nanosecond precision.
func buildWrite(c as Client, body as string) {
    def ct as string init "text/plain; charset=utf-8";
    match ($c.version) {
        when V1 {
            def url as string init joinBase($c.url, "/write") + "?db=" +
                uri.encode($c.db) + "&precision=ns";
            return Req{url: $url, contentType: $ct, headers: authHeaders($c), body: $body};
        }
        when V2 {
            def url as string init joinBase($c.url, "/api/v2/write") + "?org=" +
                uri.encode($c.org) + "&bucket=" + uri.encode($c.bucket) + "&precision=ns";
            return Req{url: $url, contentType: $ct, headers: authHeaders($c), body: $body};
        }
    }
}

# buildQuery builds the 1.x InfluxQL request (`/query?db=...&q=...`, empty body).
func buildQuery(c as Client, influxql as string) {
    def url as string init joinBase($c.url, "/query") + "?db=" + uri.encode($c.db) +
        "&q=" + uri.encode($influxql);
    return Req{
        url: $url,
        contentType: "application/x-www-form-urlencoded",
        headers: authHeaders($c),
        body: ""
    };
}

# buildFlux builds the 2.x Flux request (`/api/v2/query?org=...`, the Flux
# script as the body under `application/vnd.flux`).
func buildFlux(c as Client, flux as string) {
    def url as string init joinBase($c.url, "/api/v2/query") + "?org=" + uri.encode($c.org);
    return Req{
        url: $url,
        contentType: "application/vnd.flux",
        headers: authHeaders($c),
        body: $flux
    };
}

# errorFrom extracts a server error message from a failed response, falling
# back to the status code when the body is not a JSON `{"error": ...}`.
func errorFrom(resp as http.Response) {
    if (len($resp.body) > 0) {
        try {
            def n as json.Value init json.decode($resp.body);
            if (json.has($n, "/error")) {
                return json.asString($n, "/error");
            }
        } catch (e) { # lint-disable: L103
            # Intentionally empty: a non-JSON error body falls through to the
            # status message below. Bound to keep the swallow deliberate.
        }
    }
    return "request failed with status " + convert.toString($resp.status);
}

# --- write (exported) -------------------------------------------------------

/**
 * Write points to the client's target (line protocol, nanosecond precision).
 * Dispatches on the client's `version`: a 1.x client posts to `/write?db=...`,
 * a 2.x client to `/api/v2/write?org=...&bucket=...`.
 * @param c {Client} the client
 * @param points {list of Point} the points to write
 * @throws {Error} kind "influxdb" on a non-2xx response (2.x token redacted)
 */
export func write(c as Client, points as list of Point) {
    if (len($points) == 0) {
        return null;
    }
    # Collect the lines and join once: an accumulating `+` over the batch would
    # be O(N^2) in the body size, and batch writes are the hot path.
    def lines as list of string init [];
    for (def p in $points) {
        $lines[] = line($p);
    }
    def body as string init strings.join($lines, "\n");
    def req as Req init buildWrite($c, $body);
    def resp as http.Response init http.post(
        $req.url,
        $req.contentType,
        $req.body,
        $req.headers);
    if ($resp.status >= 300) {
        fail(redact($c, errorFrom($resp)));
    }
    return null;
}

# --- query (exported) -------------------------------------------------------

# cellString stringifies one JSON scalar cell (null -> "").
func cellString(node as json.Value, ptr as string) {
    def t as string init json.typeOf($node, $ptr);
    match ($t) {
        when "string" { return json.asString($node, $ptr); }
        when "bool" {
            if (json.asBool($node, $ptr)) {
                return "true";
            }
            return "false";
        }
        when "int" { return convert.toString(json.asInt($node, $ptr)); }
        when "float" { return convert.toString(json.asFloat($node, $ptr)); }
        else { return ""; } # null and anything else -> ""
    }
}

# parseSeries reads one series object at `base` into a Series.
func parseSeries(node as json.Value, base as string) {
    def name as string init "";
    if (json.has($node, $base + "/name")) {
        $name = json.asString($node, $base + "/name");
    }
    def tags as map of string to string init {};
    if (json.has($node, $base + "/tags")) {
        def keys as list of string init json.keys($node, $base + "/tags");
        for (def k in $keys) {
            $tags[$k] = json.asString($node, $base + "/tags/" + $k);
        }
    }
    def columns as list of string init [];
    if (json.has($node, $base + "/columns")) {
        def nc as int init json.length($node, $base + "/columns");
        def c as int init 0;
        while ($c < $nc) {
            $columns[] = json.asString($node, $base + "/columns/" + convert.toString($c));
            $c = $c + 1;
        }
    }
    def values as list of list of string init [];
    if (json.has($node, $base + "/values")) {
        def nrows as int init json.length($node, $base + "/values");
        def r as int init 0;
        while ($r < $nrows) {
            def rowptr as string init $base + "/values/" + convert.toString($r);
            def ncols as int init json.length($node, $rowptr);
            def row as list of string init [];
            def cc as int init 0;
            while ($cc < $ncols) {
                $row[] = cellString($node, $rowptr + "/" + convert.toString($cc));
                $cc = $cc + 1;
            }
            $values[] = $row;
            $r = $r + 1;
        }
    }
    return Series{name: $name, tags: $tags, columns: $columns, values: $values};
}

# parseQuery turns a decoded `/query` response into a Result, throwing on a
# per-statement error.
func parseQuery(node as json.Value) {
    def series as list of Series init [];
    if (not json.has($node, "/results")) {
        return Result{series: $series};
    }
    def nres as int init json.length($node, "/results");
    def i as int init 0;
    while ($i < $nres) {
        def rbase as string init "/results/" + convert.toString($i);
        if (json.has($node, $rbase + "/error")) {
            fail(json.asString($node, $rbase + "/error"));
        }
        if (json.has($node, $rbase + "/series")) {
            def nser as int init json.length($node, $rbase + "/series");
            def j as int init 0;
            while ($j < $nser) {
                $series[] = parseSeries($node, $rbase + "/series/" + convert.toString($j));
                $j = $j + 1;
            }
        }
        $i = $i + 1;
    }
    return Result{series: $series};
}

/**
 * Run an InfluxQL statement against a 1.x client's database and parse the
 * result. (The 2.x query language is Flux - see `queryFlux`.)
 * @param c {Client} the client
 * @param influxql {string} the InfluxQL statement (e.g. `SELECT * FROM cpu`)
 * @return {Result} the parsed series
 * @throws {Error} kind "influxdb" on a request failure or a query error
 */
export func query(c as Client, influxql as string) {
    def req as Req init buildQuery($c, $influxql);
    def resp as http.Response init http.post(
        $req.url,
        $req.contentType,
        $req.body,
        $req.headers);
    if ($resp.status >= 300) {
        fail(redact($c, errorFrom($resp)));
    }
    def node as json.Value;
    try {
        $node = json.decode($resp.body);
    } catch (e) {
        fail(redact($c, "non-JSON response from the query endpoint"));
    }
    return parseQuery($node);
}

/**
 * Run a Flux query against a 2.x client's organization and return the raw
 * response body: InfluxDB answers a Flux query with annotated CSV (RFC 4180
 * with `#datatype` / `#group` / `#default` header rows), not JSON, so this
 * returns it verbatim - parse it with the `csv` module. Posts the script to
 * `/api/v2/query?org=...` under `application/vnd.flux`.
 * @param c {Client} a 2.x client (from `client2`)
 * @param flux {string} the Flux script
 * @return {string} the annotated-CSV response body
 * @throws {Error} kind "influxdb" on a non-2xx response (token redacted)
 */
export func queryFlux(c as Client, flux as string) {
    def req as Req init buildFlux($c, $flux);
    def resp as http.Response init http.post(
        $req.url,
        $req.contentType,
        $req.body,
        $req.headers);
    if ($resp.status >= 300) {
        fail(redact($c, errorFrom($resp)));
    }
    return $resp.body;
}
