# `statsd` - StatsD metrics client

Import with `import "statsd.j" as statsd;`. Emit `metric:value|type` lines to a
StatsD / Datadog / Telegraf agent over UDP. This is the **push** counterpart to
a pull-based scrape: it is **fire-and-forget** (UDP, no reply, no error when no
agent is listening), so a metric costs one datagram and never blocks the
program. Needs the default `jennifer` binary (`net`).

```jennifer
import "statsd.j" as statsd;

def c as statsd.Client init statsd.clientWith("127.0.0.1:8125", "web");
statsd.increment($c, "requests");        # web.requests:1|c
statsd.timing($c, "response", 42);       # web.response:42|ms
statsd.gauge($c, "queue.depth", 7);      # web.queue.depth:7|g
statsd.close($c);
```

Runnable: [`examples/modules/statsd_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/statsd_demo.j).

## Client

```jennifer
def struct statsd.Client {
    socket as net.UDPSocket,   # the sending socket
    address as string,         # the agent "host:port"
    prefix as string           # a metric-name namespace ("" for none)
};
```

A `Client` bundles the sending socket, the agent address, and an optional
metric-name **prefix**. Value-copies share the underlying socket (the usual
handle carve-out to value semantics), so copying a `Client` is safe and cheap.
The prefix is joined to every metric name with a `.` separator, so prefix `web`
and metric `hits` send `web.hits`; an empty prefix sends the bare name.

| Call | Returns | |
| ---- | ------- | - |
| `statsd.client(host)` | `Client` | connect to `host:8125` (the default port), no prefix |
| `statsd.clientWith(address, prefix)` | `Client` | connect to a full `host:port` with a metric-name prefix |
| `statsd.close(c)` | | close the sending socket |

## Metrics

| Call | Wire line | Type |
| ---- | --------- | ---- |
| `statsd.count(c, name, value)` | `name:value\|c` | counter delta (`value` may be negative) |
| `statsd.increment(c, name)` | `name:1\|c` | counter +1 |
| `statsd.decrement(c, name)` | `name:-1\|c` | counter -1 |
| `statsd.gauge(c, name, value)` | `name:value\|g` | absolute gauge |
| `statsd.timing(c, name, ms)` | `name:ms\|ms` | timer, milliseconds |
| `statsd.set(c, name, value)` | `name:value\|s` | unique-member set (agent counts distinct values) |

All six are fire-and-forget: they format one line, send one datagram, and
return. `count` / `increment` / `decrement` adjust a counter; `gauge` sets an
absolute value; `timing` records a duration the agent aggregates into
percentiles; `set` records a distinct member (e.g. a user id) the agent counts
uniquely. The base verbs take integer counter / gauge values and string `set`
members; the extensions below add sample rates, tags, float values, and
batching.

## Sample rates

A `|@rate` suffix tells the agent this metric was emitted from only a fraction
of the events, so it scales the received value back up by `1/rate`. Emit it from
a `*Rate` verb; a `rate` of `1.0` (or any value `>= 1`) samples everything and
sends no suffix.

| Call | Wire line |
| ---- | --------- |
| `statsd.countRate(c, name, value, rate)` | `name:value\|c\|@rate` |
| `statsd.timingRate(c, name, ms, rate)` | `name:ms\|ms\|@rate` |

```jennifer
statsd.countRate($c, "hits", 1, 0.1);       # web.hits:1|c|@0.1
statsd.timingRate($c, "response", 42, 0.5); # web.response:42|ms|@0.5
```

The rate is formatted compactly (`0.1`, not `0.100000`).

## Tags (DogStatsD)

The Datadog `|#key:value,key2:value2` extension attaches dimensions to a metric.
Pass a `map of string to string` (rendered in insertion order) to a `*Tagged`
verb; the tag suffix comes last on the line, after any sample rate.

| Call | Wire line |
| ---- | --------- |
| `statsd.countTagged(c, name, value, tags)` | `name:value\|c\|#k:v,...` |
| `statsd.incrementTagged(c, name, tags)` | `name:1\|c\|#k:v,...` |
| `statsd.decrementTagged(c, name, tags)` | `name:-1\|c\|#k:v,...` |
| `statsd.gaugeTagged(c, name, value, tags)` | `name:value\|g\|#k:v,...` |
| `statsd.timingTagged(c, name, ms, tags)` | `name:ms\|ms\|#k:v,...` |
| `statsd.setTagged(c, name, value, tags)` | `name:value\|s\|#k:v,...` |

```jennifer
def tags as map of string to string init {"env": "prod", "host": "h1"};
statsd.countTagged($c, "hits", 1, $tags);  # web.hits:1|c|#env:prod,host:h1
```

Tag keys and values are control-character validated on the wire line just like
the metric name, so a tag carrying a newline, `|`, or `,` (which would forge an
extra metric or tag) is a positioned `statsd` error.

## Float values

StatsD also accepts fractional counter / gauge values (e.g. a load average).

| Call | Wire line |
| ---- | --------- |
| `statsd.countFloat(c, name, value)` | `name:value\|c` |
| `statsd.gaugeFloat(c, name, value)` | `name:value\|g` |

```jennifer
statsd.gaugeFloat($c, "load", 3.5);        # web.load:3.5|g
```

The float is formatted compactly (no trailing padding).

## Batching

`Batch` packs several metrics into **one** UDP datagram (the StatsD
multi-metric packet, lines separated by `\n`). It is a value-semantic builder:
each `add*` verb returns a fresh `Batch` with the line appended, and `flush`
sends the whole packet through a client. The prefix is captured from the client
at `batch` time.

```jennifer
def b as statsd.Batch init statsd.batch($c);
$b = statsd.addCount($b, "hits", 3);    # web.hits:3|c
$b = statsd.addGauge($b, "queue", 7);   # web.queue:7|g
$b = statsd.addIncrement($b, "errors"); # web.errors:1|c
statsd.flush($c, $b);                   # one datagram, three lines
```

| Call | Returns | |
| ---- | ------- | - |
| `statsd.batch(c)` | `Batch` | start an empty accumulator, capturing the client prefix |
| `statsd.addCount(b, name, value)` | `Batch` | append `name:value\|c` |
| `statsd.addIncrement(b, name)` | `Batch` | append `name:1\|c` |
| `statsd.addDecrement(b, name)` | `Batch` | append `name:-1\|c` |
| `statsd.addGauge(b, name, value)` | `Batch` | append `name:value\|g` |
| `statsd.addTiming(b, name, ms)` | `Batch` | append `name:ms\|ms` |
| `statsd.addSet(b, name, value)` | `Batch` | append `name:value\|s` |
| `statsd.flush(c, b)` | | send all lines in one datagram (an empty batch sends nothing) |

Each line is individually validated as it is added, so a batched packet can no
more forge an extra metric than a single-metric send.

## Scope

- **Fire-and-forget only.** UDP means a lost or unheard datagram is silent by
  design - there is no delivery confirmation and no error when the agent is
  down. Use it for metrics, not for data you must not lose.
- **One datagram per verb (except `Batch`).** The single-metric verbs each send
  their own datagram; use a `Batch` to coalesce several metrics into one packet.

## See also

- [net.md](../libraries/net.md) - the UDP surface (`listenUDP` / `sendTo`) the
  client is built on.
- [modules/index.md](index.md) - the module catalog and import rules.
