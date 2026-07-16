# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

/**
 * An InfluxDB (1.x) client over `http`: write measurements as line-protocol
 * points and run InfluxQL queries, getting back a parsed result. The push
 * counterpart to a scrape - `write` sends points to `/write`, `query` runs an
 * InfluxQL statement against `/query` and parses the tabular JSON into `Series`.
 *
 * A `Point` is built with value-semantic builders (`point` / `tag` / `field` /
 * `intField` / `stringField` / `boolField` / `at`), each returning a fresh
 * `Point`, then rendered to a line-protocol line by `line` (or sent by
 * `write`). Field types are carried as pre-rendered fragments, so one point can
 * mix float / int / string / bool fields despite Jennifer's homogeneous maps.
 * Needs the default `jennifer` binary (`http` over `net`); a failed request
 * throws `Error{kind: "influxdb"}`.
 * @module influxdb
 * @example
 * import "influxdb.j" as influxdb;
 * def db as influxdb.Client init influxdb.client("http://localhost:8086", "metrics");
 * def p as influxdb.Point init influxdb.field(
 *     influxdb.tag(influxdb.point("cpu"), "host", "server01"), "value", 0.64);
 * influxdb.write($db, [$p]);
 * def r as influxdb.Result init influxdb.query($db, "SELECT last(\"value\") FROM cpu");
 */
use json;
use strings;
use convert;
use lists;
use time;
use encoding;
import "./http.j" as http;

# The default InfluxDB 1.x HTTP endpoint.
def const DEFAULT_URL as string init "http://localhost:8086";

/**
 * A client: the base URL, the target database, and optional Basic-auth
 * credentials ("" for none).
 * @field url {string} the base URL (e.g. "http://localhost:8086")
 * @field db {string} the database name
 * @field user {string} the username ("" for no auth)
 * @field password {string} the password
 */
export def struct Client {
    url as string,
    db as string,
    user as string,
    password as string
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
    throw Error{ kind: "influxdb", message: "influxdb: " + $msg, file: "", line: 0, col: 0 };
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
 * Open a client with HTTP Basic-auth credentials.
 * @param url {string} the base URL
 * @param db {string} the database name
 * @param user {string} the username
 * @param password {string} the password
 * @return {Client} a ready client
 */
export func clientWith(url as string, db as string, user as string, password as string) {
    return Client{ url: $url, db: $db, user: $user, password: $password };
}

# --- line-protocol escaping (private) ---------------------------------------

# escapeChars backslash-escapes every character of `s` that appears in
# `specials`.
func escapeChars(s as string, specials as string) {
    def out as string init "";
    def cs as list of string init strings.chars($s);
    for (def ch in $cs) {
        if (strings.contains($specials, $ch)) {
            $out = $out + "\\" + $ch;
        } else {
            $out = $out + $ch;
        }
    }
    return $out;
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
    return Point{ measurement: $measurement, tags: $tags, fields: $fields, timestamp: 0, timed: false };
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
        fail("point \"" + $p.measurement + "\" contains a newline in a measurement / tag / field value (not representable in line protocol)");
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

# hexByte renders one byte as two uppercase hex digits.
func hexByte(b as int) {
    def digits as string init "0123456789ABCDEF";
    return strings.substring($digits, $b // 16, $b // 16 + 1) +
        strings.substring($digits, $b % 16, $b % 16 + 1);
}

# urlEncode percent-encodes a URL query component (unreserved bytes stay).
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

# authHeaders builds the Basic-auth header map for a client (empty when the
# client carries no username).
func authHeaders(c as Client) {
    def h as map of string to string init {};
    if (len($c.user) > 0) {
        def creds as string init $c.user + ":" + $c.password;
        def enc as string init encoding.toText(convert.bytesFromString($creds, "utf-8"), "base64");
        $h["Authorization"] = "Basic " + $enc;
    }
    return $h;
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
        } catch (e) {
            # body is not JSON; fall through to the status message
        }
    }
    return "request failed with status " + convert.toString($resp.status);
}

# --- write (exported) -------------------------------------------------------

/**
 * Write points to the client's database (line protocol, nanosecond precision).
 * @param c {Client} the client
 * @param points {list of Point} the points to write
 * @throws {Error} kind "influxdb" on a non-2xx response
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
    def url as string init joinBase($c.url, "/write") + "?db=" + urlEncode($c.db) + "&precision=ns";
    def resp as http.Response init http.post($url, "text/plain; charset=utf-8", $body, authHeaders($c));
    if ($resp.status >= 300) {
        fail(errorFrom($resp));
    }
    return null;
}

# --- query (exported) -------------------------------------------------------

# cellString stringifies one JSON scalar cell (null -> "").
func cellString(node as json.Value, ptr as string) {
    def t as string init json.typeOf($node, $ptr);
    if ($t == "null") {
        return "";
    }
    if ($t == "string") {
        return json.asString($node, $ptr);
    }
    if ($t == "bool") {
        if (json.asBool($node, $ptr)) {
            return "true";
        }
        return "false";
    }
    if ($t == "int") {
        return convert.toString(json.asInt($node, $ptr));
    }
    if ($t == "float") {
        return convert.toString(json.asFloat($node, $ptr));
    }
    return "";
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
    return Series{ name: $name, tags: $tags, columns: $columns, values: $values };
}

# parseQuery turns a decoded `/query` response into a Result, throwing on a
# per-statement error.
func parseQuery(node as json.Value) {
    def series as list of Series init [];
    if (not json.has($node, "/results")) {
        return Result{ series: $series };
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
    return Result{ series: $series };
}

/**
 * Run an InfluxQL statement against the client's database and parse the result.
 * @param c {Client} the client
 * @param influxql {string} the InfluxQL statement (e.g. `SELECT * FROM cpu`)
 * @return {Result} the parsed series
 * @throws {Error} kind "influxdb" on a request failure or a query error
 */
export func query(c as Client, influxql as string) {
    def url as string init joinBase($c.url, "/query") + "?db=" + urlEncode($c.db) + "&q=" + urlEncode($influxql);
    def resp as http.Response init http.post($url, "application/x-www-form-urlencoded", "", authHeaders($c));
    if ($resp.status >= 300) {
        fail(errorFrom($resp));
    }
    return parseQuery(json.decode($resp.body));
}
