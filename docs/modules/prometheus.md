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
| `prometheus.Metric`                  | `name`, `help`, `type` (`MetricType.Counter` / `MetricType.Gauge`), `samples`. |
| `prometheus.Sample`                  | `labels` (map), `value` (float) - one rendered line.                 |
| `prometheus.counter(name, help)`     | A new counter metric; throws on an invalid name.                      |
| `prometheus.gauge(name, help)`       | A new gauge metric; throws on an invalid name.                        |
| `prometheus.observe(metric, labels, value)` | Record a sample (upsert by label set); throws on an invalid label name. |
| `prometheus.render(metrics)`         | Render a `list of Metric` as the text exposition format.              |

`observe` **upserts**: a sample with an equal label set is replaced (last write
wins), so re-observing the same series updates its value rather than duplicating
the line. `render` sorts label keys, so output is deterministic regardless of
the order labels were inserted.

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
label-key sorting, and the upsert - is unit-tested in the overlay
(`modules/prometheus_test.j`), alongside the result parser against canned
vector / matrix / scalar / error responses. The networked `query` /
`queryRange` path is covered end to end against an in-process fake Prometheus in
the Go test suite (`TestPrometheusQuery`), which also proves the PromQL URL
encoding round-trips.

## Out of scope

- **`counter` and `gauge` only.** `histogram` and `summary` (buckets /
  quantiles, the `_bucket` / `_sum` / `_count` child series) are a documented
  follow-on.
- **No registry / auto-collection.** The caller holds and assembles the metric
  set; there is no global default registry or process/Go collectors.
- **Query results are read-only values.** No PromQL building or evaluation - the
  server does that.

## See also

- [http.md](http.md) - the client transport the retrieval half builds on.
- [json.md](../libraries/json.md) - the query-response decoder.
- [modules/index.md](index.md) - the module catalog and import rules.
