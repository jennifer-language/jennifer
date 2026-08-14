# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
# pragma-jennifer-version: >=0.25.0

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
use math;
use json;
import "./http.j" as http;
import "./uri.j" as uri;

# --- exposition types -------------------------------------------------------

/**
 * One recorded series of a metric, keyed by its label set. For a `Counter` or
 * `Gauge` only `value` is used. For a `Histogram`, `count` / `sum` accumulate
 * and `buckets` holds the cumulative per-bucket counts (parallel to the owning
 * `Metric.buckets`). For a `Summary`, `count` / `sum` accumulate and
 * `observations` holds the raw observed values (quantiles are computed at
 * render time). An optional millisecond `timestamp` is appended after the value
 * when `hasTimestamp` is set.
 * @field labels {map of string to string} the label name/value pairs ({} for none)
 * @field value {float} the sample value (Counter / Gauge)
 * @field count {float} the observation count (Histogram / Summary)
 * @field sum {float} the observation sum (Histogram / Summary)
 * @field buckets {list of float} cumulative per-bucket counts (Histogram; parallel to Metric.buckets)
 * @field observations {list of float} the raw observed values (Summary; source for quantiles)
 * @field timestamp {int} an explicit millisecond timestamp, appended after the value when set
 * @field hasTimestamp {bool} whether `timestamp` is present and should be rendered
 */
export def struct Sample {
    labels as map of string to string,
    value as float,
    count as float,
    sum as float,
    buckets as list of float,
    observations as list of float,
    timestamp as int,
    hasTimestamp as bool
};

/**
 * A Prometheus metric type: `Counter` (a monotonically increasing total),
 * `Gauge` (a value that can go up and down), `Histogram` (bucketed
 * observations with `_bucket` / `_sum` / `_count` child series), or `Summary`
 * (quantile observations with `{quantile="..."}` / `_sum` / `_count` series).
 */
export def enum MetricType { Counter, Gauge, Histogram, Summary };

/**
 * A metric family: a name, help text, a type, its samples, and (for a
 * `Histogram` / `Summary`) its bucket bounds / quantiles.
 * @field name {string} the metric name (`[a-zA-Z_:][a-zA-Z0-9_:]*`)
 * @field help {string} the HELP text (empty to omit the HELP line)
 * @field type {MetricType} the metric type
 * @field samples {list of Sample} the recorded series
 * @field buckets {list of float} the histogram upper bounds (`le`), ascending; empty for other types
 * @field quantiles {list of float} the summary quantiles in `[0, 1]`, ascending; empty for other types
 */
export def struct Metric {
    name as string,
    help as string,
    type as MetricType,
    samples as list of Sample,
    buckets as list of float,
    quantiles as list of float
};

# typeName renders a metric type to its Prometheus text-exposition token. A
# MetricType parameter, so the `match` is exhaustiveness-checked: a new metric
# type must be given a token here before it compiles.
func typeName(t as MetricType) {
    match ($t) {
        when Counter { return "counter"; }
        when Gauge { return "gauge"; }
        when Histogram { return "histogram"; }
        when Summary { return "summary"; }
    }
}

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
        throw Error{
            kind: "prometheus",
            message: "prometheus: invalid metric name: " + $name,
            file: "",
            line: 0,
            col: 0
        };
    }
    def s as list of Sample init [];
    def bk as list of float init [];
    def qs as list of float init [];
    return Metric{name: $name, help: $help, type: MetricType.Counter, samples: $s, buckets: $bk, quantiles: $qs};
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
        throw Error{
            kind: "prometheus",
            message: "prometheus: invalid metric name: " + $name,
            file: "",
            line: 0,
            col: 0
        };
    }
    def s as list of Sample init [];
    def bk as list of float init [];
    def qs as list of float init [];
    return Metric{name: $name, help: $help, type: MetricType.Gauge, samples: $s, buckets: $bk, quantiles: $qs};
}

/**
 * Build an empty histogram metric. Buckets are the cumulative upper bounds
 * (`le`); they are sorted ascending, and an `+Inf` bucket is always rendered in
 * addition to the given bounds. Throws on an invalid metric name.
 * @param name {string} the metric name (`[a-zA-Z_:][a-zA-Z0-9_:]*`)
 * @param help {string} the HELP text (empty to omit the HELP line)
 * @param buckets {list of float} the cumulative upper bounds (`le`); sorted ascending internally
 * @return {Metric} the new histogram metric
 * @throws {Error} kind "prometheus" when the name is invalid
 */
export func histogram(name as string, help as string, buckets as list of float) {
    if (not isValidName($name)) {
        throw Error{
            kind: "prometheus",
            message: "prometheus: invalid metric name: " + $name,
            file: "",
            line: 0,
            col: 0
        };
    }
    def s as list of Sample init [];
    def bk as list of float init lists.sort($buckets);
    def qs as list of float init [];
    return Metric{name: $name, help: $help, type: MetricType.Histogram, samples: $s, buckets: $bk, quantiles: $qs};
}

/**
 * Build an empty summary metric. Quantiles are values in `[0, 1]`; they are
 * sorted ascending and reported as `{quantile="..."}` child series, computed
 * from the observed values at render time. Throws on an invalid metric name or
 * an out-of-range quantile.
 * @param name {string} the metric name (`[a-zA-Z_:][a-zA-Z0-9_:]*`)
 * @param help {string} the HELP text (empty to omit the HELP line)
 * @param quantiles {list of float} the reported quantiles, each in `[0, 1]`; sorted ascending internally
 * @return {Metric} the new summary metric
 * @throws {Error} kind "prometheus" when the name is invalid or a quantile is out of range
 */
export func summary(name as string, help as string, quantiles as list of float) {
    if (not isValidName($name)) {
        throw Error{
            kind: "prometheus",
            message: "prometheus: invalid metric name: " + $name,
            file: "",
            line: 0,
            col: 0
        };
    }
    for (def q in $quantiles) {
        if ($q < 0.0 or $q > 1.0) {
            throw Error{
                kind: "prometheus",
                message: "prometheus: quantile out of range [0,1]: " + convert.toString($q),
                file: "",
                line: 0,
                col: 0
            };
        }
    }
    def s as list of Sample init [];
    def bk as list of float init [];
    def qs as list of float init lists.sort($quantiles);
    return Metric{name: $name, help: $help, type: MetricType.Summary, samples: $s, buckets: $bk, quantiles: $qs};
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

# freshSample builds a zeroed Sample for a label set, with `nbuckets` bucket
# slots pre-sized to 0.0 (for a histogram; 0 for the other types).
func freshSample(labels as map of string to string, nbuckets as int) {
    def bk as list of float init [];
    def i as int init 0;
    while ($i < $nbuckets) {
        $bk[] = 0.0;
        $i = $i + 1;
    }
    def obs as list of float init [];
    return Sample{
        labels: $labels,
        value: 0.0,
        count: 0.0,
        sum: 0.0,
        buckets: $bk,
        observations: $obs,
        timestamp: 0,
        hasTimestamp: false
    };
}

# record is the shared observe core: it validates label names, finds (or
# appends) the sample for the label set, and updates it per metric type -
# Counter / Gauge upsert the value (last write wins); Histogram / Summary
# accumulate. An optional millisecond timestamp is stamped onto the sample.
func record(metric as Metric, labels as map of string to string, value as float, hasTs as bool, ts as int) {
    for (def k in $labels) {
        if (not isValidLabelName($k)) {
            throw Error{
                kind: "prometheus",
                message: "prometheus: invalid label name: " + $k,
                file: "",
                line: 0,
                col: 0
            };
        }
    }
    def out as Metric init $metric;
    def idx as int init -1;
    def i as int init 0;
    while ($i < len($out.samples)) {
        if (labelsEqual($out.samples[$i].labels, $labels)) {
            $idx = $i;
            break; # a label set appears once; stop scanning
        }
        $i = $i + 1;
    }
    if ($idx == -1) {
        $out.samples = lists.push($out.samples, freshSample($labels, len($out.buckets)));
        $idx = len($out.samples) - 1;
    }
    match ($out.type) {
        when Counter {
            $out.samples[$idx].value = $value;
        }
        when Gauge {
            $out.samples[$idx].value = $value;
        }
        when Histogram {
            $out.samples[$idx].count = $out.samples[$idx].count + 1.0;
            $out.samples[$idx].sum = $out.samples[$idx].sum + $value;
            def j as int init 0;
            while ($j < len($out.buckets)) {
                # cumulative: an observation lands in every bucket whose upper
                # bound (`le`) is >= the value.
                if ($out.buckets[$j] >= $value) {
                    $out.samples[$idx].buckets[$j] = $out.samples[$idx].buckets[$j] + 1.0;
                }
                $j = $j + 1;
            }
        }
        when Summary {
            $out.samples[$idx].count = $out.samples[$idx].count + 1.0;
            $out.samples[$idx].sum = $out.samples[$idx].sum + $value;
            $out.samples[$idx].observations = lists.push($out.samples[$idx].observations, $value);
        }
    }
    $out.samples[$idx].hasTimestamp = $hasTs;
    $out.samples[$idx].timestamp = $ts;
    return $out;
}

/**
 * Record an observation for a label set, returning a new Metric
 * (value-semantic). For a `Counter` / `Gauge`, a sample with an equal label set
 * is replaced (last write wins). For a `Histogram` / `Summary`, the observation
 * accumulates: the count and sum grow, histogram buckets increment, and summary
 * observations are retained for quantile computation. Throws on an invalid
 * label name.
 * @param metric {Metric} the metric to extend
 * @param labels {map of string to string} the sample's label set ({} for none)
 * @param value {float} the observed value
 * @return {Metric} a new Metric with the observation recorded
 * @throws {Error} kind "prometheus" when a label name is invalid
 */
export func observe(metric as Metric, labels as map of string to string, value as float) {
    return record($metric, $labels, $value, false, 0);
}

/**
 * Record an observation carrying an explicit millisecond timestamp, returning a
 * new Metric (value-semantic). Same accumulation rules as `observe`; the
 * timestamp is appended after the value in the rendered exposition
 * (`metric value timestamp`). Throws on an invalid label name.
 * @param metric {Metric} the metric to extend
 * @param labels {map of string to string} the sample's label set ({} for none)
 * @param value {float} the observed value
 * @param timestampMs {int} the sample timestamp in milliseconds since the Unix epoch
 * @return {Metric} a new Metric with the timestamped observation recorded
 * @throws {Error} kind "prometheus" when a label name is invalid
 */
export func observeAt(metric as Metric, labels as map of string to string, value as float, timestampMs as int) {
    return record($metric, $labels, $value, true, $timestampMs);
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
    return '{' + strings.join($parts, ",") + '}';
}

# mergeLabel returns a copy of a label set with one extra key set (value
# semantics; the input map is not mutated). Used to fold `le` / `quantile`
# into a sample's own labels so they sort with the rest.
func mergeLabel(labels as map of string to string, key as string, val as string) {
    def out as map of string to string init $labels;
    $out[$key] = $val;
    return $out;
}

# sampleTsSuffix renders " <timestamp>" when the sample carries one, else "".
func sampleTsSuffix(s as Sample) {
    if ($s.hasTimestamp) {
        return " " + convert.toString($s.timestamp);
    }
    return "";
}

# quantileOf returns the q-quantile of an ascending-sorted list using the
# nearest-rank method (rank = ceil(q * n), clamped to [1, n]). Empty input
# yields 0.0.
func quantileOf(sorted as list of float, q as float) {
    def n as int init len($sorted);
    if ($n == 0) {
        return 0.0;
    }
    def rank as int init math.ceil($q * convert.toFloat($n));
    if ($rank < 1) {
        $rank = 1;
    }
    if ($rank > $n) {
        $rank = $n;
    }
    return $sorted[$rank - 1];
}

# simpleLines renders the sample lines of a Counter / Gauge metric.
func simpleLines(m as Metric) {
    def lines as list of string init [];
    for (def s in $m.samples) {
        $lines[] = $m.name + renderLabels($s.labels) + " " + convert.toString($s.value) + sampleTsSuffix($s);
    }
    return $lines;
}

# histogramLines renders the `_bucket` (cumulative, `+Inf` last), `_sum`, and
# `_count` child series of a Histogram metric.
func histogramLines(m as Metric) {
    def lines as list of string init [];
    for (def s in $m.samples) {
        def ts as string init sampleTsSuffix($s);
        def j as int init 0;
        while ($j < len($m.buckets)) {
            def le as map of string to string init mergeLabel($s.labels, "le", convert.toString($m.buckets[$j]));
            $lines[] = $m.name + "_bucket" + renderLabels($le) + " " + convert.toString($s.buckets[$j]) + $ts;
            $j = $j + 1;
        }
        def inf as map of string to string init mergeLabel($s.labels, "le", "+Inf");
        $lines[] = $m.name + "_bucket" + renderLabels($inf) + " " + convert.toString($s.count) + $ts;
        $lines[] = $m.name + "_sum" + renderLabels($s.labels) + " " + convert.toString($s.sum) + $ts;
        $lines[] = $m.name + "_count" + renderLabels($s.labels) + " " + convert.toString($s.count) + $ts;
    }
    return $lines;
}

# summaryLines renders the `{quantile="..."}`, `_sum`, and `_count` child series
# of a Summary metric. Quantiles are computed from the retained observations.
func summaryLines(m as Metric) {
    def lines as list of string init [];
    for (def s in $m.samples) {
        def ts as string init sampleTsSuffix($s);
        def sorted as list of float init lists.sort($s.observations);
        for (def q in $m.quantiles) {
            def ql as map of string to string init mergeLabel($s.labels, "quantile", convert.toString($q));
            $lines[] = $m.name + renderLabels($ql) + " " + convert.toString(quantileOf($sorted, $q)) + $ts;
        }
        $lines[] = $m.name + "_sum" + renderLabels($s.labels) + " " + convert.toString($s.sum) + $ts;
        $lines[] = $m.name + "_count" + renderLabels($s.labels) + " " + convert.toString($s.count) + $ts;
    }
    return $lines;
}

# metricLines renders the sample lines of one metric, dispatched by type. The
# `match` over the metric type is exhaustiveness-checked, so a new metric type
# must be given a renderer here before it compiles.
func metricLines(m as Metric) {
    def lines as list of string init [];
    match ($m.type) {
        when Counter { $lines = simpleLines($m); }
        when Gauge { $lines = simpleLines($m); }
        when Histogram { $lines = histogramLines($m); }
        when Summary { $lines = summaryLines($m); }
    }
    return $lines;
}

/**
 * Render a list of metrics as the Prometheus text exposition format. A
 * `Histogram` emits `_bucket{le="..."}` (cumulative, ascending, `+Inf` last),
 * `_sum`, and `_count` series; a `Summary` emits `{quantile="..."}`, `_sum`,
 * and `_count` series. A sample carrying a timestamp appends it after the value.
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
        $lines[] = "# TYPE " + $m.name + " " + typeName($m.type);
        for (def ln in metricLines($m)) {
            $lines[] = $ln;
        }
    }
    if (len($lines) == 0) {
        return "";
    }
    return strings.join($lines, "\n") + "\n";
}

/**
 * Build the Pushgateway URL path for a job and a set of grouping labels:
 * `/metrics/job/<job>/<k1>/<v1>/...` with the job name and label values
 * percent-encoded and the grouping keys sorted for a deterministic path. The
 * caller POSTs the `render` output to `base + pushgatewayPath(job, grouping)`.
 * A pure string helper - no network. Throws on an invalid grouping label name.
 * @param job {string} the Pushgateway job name
 * @param grouping {map of string to string} additional grouping labels ({} for none)
 * @return {string} the Pushgateway path segment (leading slash, no trailing slash)
 * @throws {Error} kind "prometheus" when a grouping label name is invalid
 */
export func pushgatewayPath(job as string, grouping as map of string to string) {
    def parts as list of string init ["/metrics/job/" + uri.encode($job)];
    def keys as list of string init lists.sort(maps.keys($grouping));
    for (def k in $keys) {
        if (not isValidLabelName($k)) {
            throw Error{
                kind: "prometheus",
                message: "prometheus: invalid label name: " + $k,
                file: "",
                line: 0,
                col: 0
            };
        }
        $parts[] = "/" + $k + "/" + uri.encode($grouping[$k]);
    }
    return strings.join($parts, "");
}

# --- retrieval (exported; needs the default binary via http) ----------------

# joinBase joins a base URL and an API path with exactly one slash between them.
func joinBase(base as string, path as string) {
    if (strings.endsWith($base, "/")) {
        return strings.substring($base, 0, len($base) - 1) + $path;
    }
    return $base + $path;
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
    return Point{timestamp: $ts, value: $val};
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
        throw Error{kind: "prometheus", message: "prometheus: " + $msg, file: "", line: 0, col: 0};
    }
    def rtype as string init json.asString($node, "/data/resultType");
    def series as list of Series init [];
    if ($rtype == "scalar" or $rtype == "string") {
        def pt as Point init parsePoint($node, "/data/result");
        def empty as map of string to string init {};
        def one as list of Point init [$pt];
        $series[] = Series{metric: $empty, values: $one};
        return Result{resultType: $rtype, series: $series};
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
        $series[] = Series{metric: $labels, values: $pts};
        $i = $i + 1;
    }
    return Result{resultType: $rtype, series: $series};
}

# decodeBody decodes a Prometheus HTTP response, mapping a non-JSON body (a 502
# HTML page or an auth portal) to a prometheus-kind error, not a raw json one.
func decodeBody(resp as http.Response) {
    def node as json.Value;
    try {
        $node = json.decode($resp.body);
    } catch (e) {
        throw Error{
            kind: "prometheus",
            message: "prometheus: non-JSON response (HTTP " + convert.toString($resp.status) + ")",
            file: "",
            line: 0,
            col: 0
        };
    }
    return $node;
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
    def url as string init joinBase($base, "/api/v1/query") + "?query=" + uri.encode($promql);
    def resp as http.Response init http.get($url, {});
    return parseResult(decodeBody($resp));
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
export func queryRange(
    base as string,
    promql as string,
    start as string,
    end as string,
    step as string) {
    def url as string init joinBase($base, "/api/v1/query_range") + "?query=" + uri.encode($promql) +
        "&start=" + uri.encode($start) + "&end=" + uri.encode($end) + "&step=" + uri.encode($step);
    def resp as http.Response init http.get($url, {});
    return parseResult(decodeBody($resp));
}
