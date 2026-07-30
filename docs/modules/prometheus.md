# `prometheus` - metrics exposition and query

Import with `import "prometheus.j" as prometheus;`. A **Prometheus** module in
two halves. **Exposition** builds a metric set and renders the Prometheus text
format - pure text over `strings` / `maps` / `lists` / `convert`, and
transport-agnostic: write the string to a `*.prom` file for the node_exporter
textfile collector, POST it to a Pushgateway, or serve it from a `/metrics`
handler. **Retrieval** is a read client for Prometheus's HTTP query API, built on
the [`http`](http.md) module + [`json`](../libraries/json.md).

The exposition half runs on **both** binaries. The query half uses `net` through
`http`, so it needs the default **`jennifer`** binary; on `jennifer-tiny` the
exposition functions still work, and only `query` / `queryRange` surface the
no-network error.

```jennifer
import "prometheus.j" as prometheus;

def m as prometheus.Metric init prometheus.counter("http_requests_total",
    "Total HTTP requests");
$m = prometheus.observe($m, {"method": "get", "code": "200"}, 42.0);
io.printf("%s", prometheus.render([$m]));
# # HELP http_requests_total Total HTTP requests
# # TYPE http_requests_total counter
# http_requests_total{code="200",method="get"} 42.0
```

Runnable: [`examples/modules/prometheus_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/prometheus_demo.j).

## Exposition

Build metrics, record samples, render the text format. Every builder is
value-semantic (returns a new `Metric`), so a metric set is assembled by
reassignment.

| Call / type                          | Notes                                                                 |
| ------------------------------------ | --------------------------------------------------------------------- |
| `prometheus.Metric`                  | `name`, `help`, `type` (`Counter` / `Gauge` / `Histogram` / `Summary`), `samples`, `buckets`, `quantiles`. |
| `prometheus.Sample`                  | `labels` (map), `value` (float), plus `count` / `sum` / `buckets` / `observations` / `timestamp` / `hasTimestamp` for the aggregate types. |
| `prometheus.counter(name, help)`     | A new counter metric; throws on an invalid name.                      |
| `prometheus.gauge(name, help)`       | A new gauge metric; throws on an invalid name.                        |
| `prometheus.histogram(name, help, buckets)` | A new histogram; `buckets` is a `list of float` of upper bounds (`le`), sorted ascending. |
| `prometheus.summary(name, help, quantiles)` | A new summary; `quantiles` is a `list of float` in `[0, 1]`, sorted ascending. |
| `prometheus.observe(metric, labels, value)` | Record an observation; throws on an invalid label name.          |
| `prometheus.observeAt(metric, labels, value, timestampMs)` | As `observe`, plus an explicit millisecond timestamp. |
| `prometheus.render(metrics)`         | Render a `list of Metric` as the text exposition format.              |
| `prometheus.pushgatewayPath(job, grouping)` | Build the `/metrics/job/<job>/<k>/<v>/...` Pushgateway path (pure string). |

For a `Counter` / `Gauge`, `observe` **upserts**: a sample with an equal label
set is replaced (last write wins), so re-observing the same series updates its
value rather than duplicating the line. For a `Histogram` / `Summary`, `observe`
**accumulates** (see below). `render` sorts label keys, so output is
deterministic regardless of the order labels were inserted.

### Histograms and summaries

A **histogram** buckets observations by an upper bound (`le`). Each `observe`
increments the count, adds to the sum, and increments every cumulative bucket
whose bound is `>=` the value; `render` emits, for a metric named `x`:
`x_bucket{le="..."}` (cumulative counts, ascending, `le="+Inf"` last), `x_sum`,
and `x_count`, under a `# TYPE x histogram` header.

```jennifer
def h as prometheus.Metric init prometheus.histogram(
    "http_request_duration_seconds", "Request latency", [0.1, 0.5, 1.0]);
$h = prometheus.observe($h, {"method": "get"}, 0.3);
$h = prometheus.observe($h, {"method": "get"}, 0.05);
$h = prometheus.observe($h, {"method": "get"}, 2.0);
io.printf("%s", prometheus.render([$h]));
# # TYPE http_request_duration_seconds histogram
# http_request_duration_seconds_bucket{le="0.1",method="get"} 1.0
# http_request_duration_seconds_bucket{le="0.5",method="get"} 2.0
# http_request_duration_seconds_bucket{le="1.0",method="get"} 2.0
# http_request_duration_seconds_bucket{le="+Inf",method="get"} 3.0
# http_request_duration_seconds_sum{method="get"} 2.35
# http_request_duration_seconds_count{method="get"} 3.0
```

A **summary** reports quantiles. Each `observe` increments the count, adds to
the sum, and retains the observed value; `render` emits `x{quantile="..."}`
(computed from the retained observations by the nearest-rank method,
`rank = ceil(q * n)`), `x_sum`, and `x_count`, under `# TYPE x summary`. Each
label set accumulates its own count / sum / buckets / observations
independently.

### Timestamps

`observeAt(metric, labels, value, timestampMs)` records a sample carrying an
explicit millisecond Unix timestamp; `render` appends it after the value
(`metric value timestamp`), the optional third field the exposition format
allows. Plain `observe` records no timestamp.

### Pushing to a Pushgateway

`pushgatewayPath(job, grouping)` is a pure string helper that builds the
Pushgateway URL path from a job name and a `map of string to string` of grouping
labels: `/metrics/job/<job>/<k1>/<v1>/...`, with the job and values
percent-encoded and grouping keys sorted for a deterministic path. POST the
`render` output to `base + pushgatewayPath(job, grouping)` via the
[`http`](http.md) module. An invalid grouping label name throws.

### Strictness

The format's rules are enforced:

- A metric name must match `[a-zA-Z_:][a-zA-Z0-9_:]*`; a label name must match
  `[a-zA-Z_][a-zA-Z0-9_]*`. A violation throws a catchable `Error` (kind
  `"prometheus"`).
- Label values escape `\`, `"`, and newline; `# HELP` text escapes `\` and
  newline. An empty `help` omits the `# HELP` line.

### Getting the text to Prometheus

`render` returns a plain string; delivery is your choice:

- **Textfile collector** - write it to a `*.prom` file (via `fs`) in
  node_exporter's textfile directory.
- **Pushgateway** - POST it with the [`http`](http.md) module.
- **Scrape endpoint** - serve it from a `/metrics` handler (e.g. the
  [`web`](web.md) framework over `httpd`).

## Retrieval

A read client for the HTTP query API. Both return a `Result`.

| Call / type                                       | Notes                                             |
| ------------------------------------------------- | ------------------------------------------------- |
| `prometheus.Result`                               | `resultType` + `series` (a `list of Series`).     |
| `prometheus.Series`                               | `metric` (label map) + `values` (a `list of Point`). |
| `prometheus.Point`                                | `timestamp` (float, Unix seconds) + `value` (float). |
| `prometheus.query(base, promql)`                  | Instant query (`/api/v1/query`) -> `Result`.      |
| `prometheus.queryRange(base, promql, start, end, step)` | Range query (`/api/v1/query_range`) -> `Result`. |

`base` is the server URL (e.g. `"http://localhost:9090"`). `start` / `end` are
RFC 3339 or Unix-timestamp strings; `step` is a duration (`"15s"`) or a seconds
string. An instant query returns a `"vector"` (one `Point` per series); a range
query returns a `"matrix"` (many `Point`s per series). A server-reported query
error throws an `Error` (kind `"prometheus"`).

```jennifer
def r as prometheus.Result init prometheus.query("http://localhost:9090", "up");
for (def s in $r.series) {
    io.printf("%s = %f\n", $s.metric["instance"], $s.values[0].value);
}
```

## Testing

The pure exposition logic - name / label validation, value and HELP escaping,
label-key sorting, the counter/gauge upsert, histogram bucket accumulation
(with pinned `_bucket` / `_sum` / `_count` output), summary quantiles,
timestamps, and the Pushgateway path - is unit-tested in the overlay
(`modules/prometheus_test.j`), alongside the result parser against canned
vector / matrix / scalar / error responses. The networked `query` /
`queryRange` path is covered end to end against an in-process fake Prometheus in
the Go test suite (`TestPrometheusQuery`), which also proves the PromQL URL
encoding round-trips.

## Out of scope

- **No registry / auto-collection.** The caller holds and assembles the metric
  set; there is no global default registry or process/Go collectors.
- **Summary quantiles are exact over the retained observations**, not the
  streaming, bounded-error estimators the reference clients use - simpler and
  more accurate, at the cost of keeping every observed value per series.
- **Query results are read-only values.** No PromQL building or evaluation - the
  server does that.

## See also

- [http.md](http.md) - the client transport the retrieval half builds on.
- [json.md](../libraries/json.md) - the query-response decoder.
- [modules/index.md](index.md) - the module catalog and import rules.
