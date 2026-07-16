# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

/**
 * A Prometheus metrics module in two halves. **Exposition** builds a metric set
 * and renders the Prometheus text format (`# HELP` / `# TYPE` / sample lines) -
 * pure text over `strings` / `maps` / `lists` / `convert`, transport-agnostic
 * (write the string to a `*.prom` file for the node_exporter textfile
 * collector, POST it to a Pushgateway, or serve it from a `/metrics` handler).
 * **Retrieval** is a read client for Prometheus's HTTP query API (`query` /
 * `queryRange`) over the `http` module + `json`. The exposition half runs on
 * both binaries; the query half needs the default `jennifer` binary (it uses
 * `net` through `http`).
 * @module prometheus
 * @example
 * def m as prometheus.Metric init prometheus.counter("http_requests_total", "Total HTTP requests");
 * $m = prometheus.observe($m, {"method": "get", "code": "200"}, 42.0);
 * io.printf("%s", prometheus.render([$m]));
 */
use strings;
use convert;
use maps;
use lists;
use json;
import "./http.j" as http;

# --- exposition types -------------------------------------------------------

/**
 * One sample line of a metric: a label set and a float value.
 * @field labels {map of string to string} the label name/value pairs ({} for none)
 * @field value {float} the sample value
 */
export def struct Sample {
    labels as map of string to string,
    value as float
};

/**
 * A metric family: a name, help text, a type, and its samples.
 * @field name {string} the metric name (`[a-zA-Z_:][a-zA-Z0-9_:]*`)
 * @field help {string} the HELP text (empty to omit the HELP line)
 * @field type {string} the metric type ("counter" or "gauge")
 * @field samples {list of Sample} the sample lines
 */
export def struct Metric {
    name as string,
    help as string,
    type as string,
    samples as list of Sample
};

# --- retrieval types --------------------------------------------------------

/**
 * One (timestamp, value) point of a query result series.
 * @field timestamp {float} the sample time as a Unix timestamp (seconds)
 * @field value {float} the sample value
 */
export def struct Point {
    timestamp as float,
    value as float
};

/**
 * One result series: its label set and its points (one point for an instant
 * vector, many for a range matrix).
 * @field metric {map of string to string} the series label set
 * @field values {list of Point} the series points
 */
export def struct Series {
    metric as map of string to string,
    values as list of Point
};

/**
 * A parsed query result set.
 * @field resultType {string} "vector", "matrix", "scalar", or "string"
 * @field series {list of Series} the result series
 */
export def struct Result {
    resultType as string,
    series as list of Series
};

# --- name validation (pure, byte-level) -------------------------------------

# isNameHeadByte reports whether b may start a metric name: A-Z a-z _ :
func isNameHeadByte(b as int) {
    return ($b >= 65 and $b <= 90) or ($b >= 97 and $b <= 122) or $b == 95 or $b == 58;
}

# isNameTailByte reports whether b may continue a metric name (head plus 0-9).
func isNameTailByte(b as int) {
    return isNameHeadByte($b) or ($b >= 48 and $b <= 57);
}

# isLabelHeadByte reports whether b may start a label name: A-Z a-z _ (no colon).
func isLabelHeadByte(b as int) {
    return ($b >= 65 and $b <= 90) or ($b >= 97 and $b <= 122) or $b == 95;
}

# isLabelTailByte reports whether b may continue a label name (head plus 0-9).
func isLabelTailByte(b as int) {
    return isLabelHeadByte($b) or ($b >= 48 and $b <= 57);
}

# isValidName validates a metric name against `[a-zA-Z_:][a-zA-Z0-9_:]*`.
func isValidName(s as string) {
    def raw as bytes init convert.bytesFromString($s, "utf-8");
    if (len($raw) == 0) {
        return false;
    }
    if (not isNameHeadByte($raw[0])) {
        return false;
    }
    def i as int init 1;
    while ($i < len($raw)) {
        if (not isNameTailByte($raw[$i])) {
            return false;
        }
        $i = $i + 1;
    }
    return true;
}

# isValidLabelName validates a label name against `[a-zA-Z_][a-zA-Z0-9_]*`.
func isValidLabelName(s as string) {
    def raw as bytes init convert.bytesFromString($s, "utf-8");
    if (len($raw) == 0) {
        return false;
    }
    if (not isLabelHeadByte($raw[0])) {
        return false;
    }
    def i as int init 1;
    while ($i < len($raw)) {
        if (not isLabelTailByte($raw[$i])) {
            return false;
        }
        $i = $i + 1;
    }
    return true;
}

# --- escaping (pure) --------------------------------------------------------

# escapeLabelValue escapes a label value: backslash, double-quote, and newline.
func escapeLabelValue(s as string) {
    def out as string init strings.replace($s, "\\", "\\\\");
    $out = strings.replace($out, "\"", "\\\"");
    $out = strings.replace($out, "\n", "\\n");
    return $out;
}

# escapeHelp escapes HELP text: backslash and newline (quotes are not escaped).
func escapeHelp(s as string) {
    def out as string init strings.replace($s, "\\", "\\\\");
    $out = strings.replace($out, "\n", "\\n");
    return $out;
}

# --- exposition builders (exported) -----------------------------------------

/**
 * Build an empty counter metric. Throws on an invalid metric name.
 * @param name {string} the metric name (`[a-zA-Z_:][a-zA-Z0-9_:]*`)
 * @param help {string} the HELP text (empty to omit the HELP line)
 * @return {Metric} the new counter metric
 * @throws {Error} kind "prometheus" when the name is invalid
 */
export func counter(name as string, help as string) {
    if (not isValidName($name)) {
        throw Error{ kind: "prometheus", message: "prometheus: invalid metric name: " + $name, file: "", line: 0, col: 0 };
    }
    def s as list of Sample init [];
    return Metric{ name: $name, help: $help, type: "counter", samples: $s };
}

/**
 * Build an empty gauge metric. Throws on an invalid metric name.
 * @param name {string} the metric name (`[a-zA-Z_:][a-zA-Z0-9_:]*`)
 * @param help {string} the HELP text (empty to omit the HELP line)
 * @return {Metric} the new gauge metric
 * @throws {Error} kind "prometheus" when the name is invalid
 */
export func gauge(name as string, help as string) {
    if (not isValidName($name)) {
        throw Error{ kind: "prometheus", message: "prometheus: invalid metric name: " + $name, file: "", line: 0, col: 0 };
    }
    def s as list of Sample init [];
    return Metric{ name: $name, help: $help, type: "gauge", samples: $s };
}

# labelsEqual reports whether two label sets have identical keys and values.
func labelsEqual(a as map of string to string, b as map of string to string) {
    if (not (len($a) == len($b))) {
        return false;
    }
    for (def k in $a) {
        if (not maps.has($b, $k)) {
            return false;
        }
        if (not ($a[$k] == $b[$k])) {
            return false;
        }
    }
    return true;
}

/**
 * Record a sample for a label set, returning a new Metric (value-semantic). A
 * sample with an equal label set is replaced (last write wins); otherwise the
 * sample is appended. Throws on an invalid label name.
 * @param metric {Metric} the metric to extend
 * @param labels {map of string to string} the sample's label set ({} for none)
 * @param value {float} the sample value
 * @return {Metric} a new Metric with the sample recorded
 * @throws {Error} kind "prometheus" when a label name is invalid
 */
export func observe(metric as Metric, labels as map of string to string, value as float) {
    for (def k in $labels) {
        if (not isValidLabelName($k)) {
            throw Error{ kind: "prometheus", message: "prometheus: invalid label name: " + $k, file: "", line: 0, col: 0 };
        }
    }
    def out as Metric init $metric;
    def replaced as bool init false;
    def i as int init 0;
    while ($i < len($out.samples)) {
        if (labelsEqual($out.samples[$i].labels, $labels)) {
            $out.samples[$i].value = $value;
            $replaced = true;
            break;   # a label set appears once; stop scanning
        }
        $i = $i + 1;
    }
    if (not $replaced) {
        $out.samples = lists.push($out.samples, Sample{ labels: $labels, value: $value });
    }
    return $out;
}

# renderLabels renders a label set as `{k="v",...}` (sorted keys) or "" if empty.
func renderLabels(labels as map of string to string) {
    if (len($labels) == 0) {
        return "";
    }
    def keys as list of string init lists.sort(maps.keys($labels));
    def parts as list of string init [];
    for (def k in $keys) {
        $parts[] = $k + "=\"" + escapeLabelValue($labels[$k]) + "\"";
    }
    return "{" + strings.join($parts, ",") + "}";
}

/**
 * Render a list of metrics as the Prometheus text exposition format.
 * @param metrics {list of Metric} the metrics to render
 * @return {string} the exposition text (one trailing newline per line)
 */
export func render(metrics as list of Metric) {
    # Collect lines and join once: an accumulating `+` over thousands of
    # samples is O(N^2) in the output, and /metrics is scraped repeatedly.
    def lines as list of string init [];
    for (def m in $metrics) {
        if (len($m.help) > 0) {
            $lines[] = "# HELP " + $m.name + " " + escapeHelp($m.help);
        }
        $lines[] = "# TYPE " + $m.name + " " + $m.type;
        for (def s in $m.samples) {
            $lines[] = $m.name + renderLabels($s.labels) + " " + convert.toString($s.value);
        }
    }
    if (len($lines) == 0) {
        return "";
    }
    return strings.join($lines, "\n") + "\n";
}

# --- retrieval (exported; needs the default binary via http) ----------------

# joinBase joins a base URL and an API path with exactly one slash between them.
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

# parseLabels reads the object at `pointer` into a string/string map.
func parseLabels(node as json.Value, pointer as string) {
    def out as map of string to string init {};
    def keys as list of string init json.keys($node, $pointer);
    for (def k in $keys) {
        $out[$k] = json.asString($node, $pointer + "/" + $k);
    }
    return $out;
}

# parsePoint reads a `[timestamp, "value"]` pair at `pointer` into a Point. The
# timestamp is a JSON number; the value is a JSON string (Prometheus's wire form).
func parsePoint(node as json.Value, pointer as string) {
    def ts as float init json.asFloat($node, $pointer + "/0");
    def val as float init convert.toFloat(json.asString($node, $pointer + "/1"));
    return Point{ timestamp: $ts, value: $val };
}

# parseResult turns a decoded `/api/v1/query*` response into a Result. Throws
# when the response status is not "success".
func parseResult(node as json.Value) {
    def status as string init json.asString($node, "/status");
    if (not ($status == "success")) {
        def msg as string init "query failed";
        if (json.has($node, "/error")) {
            $msg = json.asString($node, "/error");
        }
        throw Error{ kind: "prometheus", message: "prometheus: " + $msg, file: "", line: 0, col: 0 };
    }
    def rtype as string init json.asString($node, "/data/resultType");
    def series as list of Series init [];
    if ($rtype == "scalar" or $rtype == "string") {
        def pt as Point init parsePoint($node, "/data/result");
        def empty as map of string to string init {};
        def one as list of Point init [$pt];
        $series[] = Series{ metric: $empty, values: $one };
        return Result{ resultType: $rtype, series: $series };
    }
    def n as int init json.length($node, "/data/result");
    def i as int init 0;
    while ($i < $n) {
        def base as string init "/data/result/" + convert.toString($i);
        def labels as map of string to string init parseLabels($node, $base + "/metric");
        def pts as list of Point init [];
        if ($rtype == "vector") {
            $pts[] = parsePoint($node, $base + "/value");
        } else {
            def m as int init json.length($node, $base + "/values");
            def j as int init 0;
            while ($j < $m) {
                $pts[] = parsePoint($node, $base + "/values/" + convert.toString($j));
                $j = $j + 1;
            }
        }
        $series[] = Series{ metric: $labels, values: $pts };
        $i = $i + 1;
    }
    return Result{ resultType: $rtype, series: $series };
}

/**
 * Run an instant query against `base` (a Prometheus server URL) via
 * `/api/v1/query`, returning the parsed result set.
 * @param base {string} the Prometheus base URL (e.g. "http://localhost:9090")
 * @param promql {string} the PromQL expression
 * @return {Result} the parsed result set
 * @throws {Error} kind "prometheus" when the server reports a query error
 */
export func query(base as string, promql as string) {
    def url as string init joinBase($base, "/api/v1/query") + "?query=" + urlEncode($promql);
    def resp as http.Response init http.get($url, {});
    return parseResult(json.decode($resp.body));
}

/**
 * Run a range query against `base` via `/api/v1/query_range`, returning the
 * parsed result matrix. `start` / `end` are RFC 3339 or Unix-timestamp strings;
 * `step` is a duration ("15s") or a seconds string.
 * @param base {string} the Prometheus base URL
 * @param promql {string} the PromQL expression
 * @param start {string} the range start (RFC 3339 or Unix timestamp)
 * @param end {string} the range end (RFC 3339 or Unix timestamp)
 * @param step {string} the resolution step (duration or seconds)
 * @return {Result} the parsed result matrix
 * @throws {Error} kind "prometheus" when the server reports a query error
 */
export func queryRange(base as string, promql as string, start as string, end as string, step as string) {
    def url as string init joinBase($base, "/api/v1/query_range") + "?query=" + urlEncode($promql) +
        "&start=" + urlEncode($start) + "&end=" + urlEncode($end) + "&step=" + urlEncode($step);
    def resp as http.Response init http.get($url, {});
    return parseResult(json.decode($resp.body));
}
