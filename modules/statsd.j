# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
# pragma-jennifer-version: >=0.24.0
# pragma-jennifer-capability: net

/**
 * A StatsD client (over UDP): emit `metric:value|type` lines for a StatsD /
 * Datadog / Telegraf agent to aggregate. This is the push counterpart to a
 * pull-based scrape - it is fire-and-forget (UDP, no reply, no error on a
 * missing agent), so a metric costs one datagram and never blocks the program.
 *
 * A `Client` holds the sending socket, the agent address, and an optional
 * metric-name prefix (namespace). The verbs map to the StatsD types: `count`
 * / `increment` / `decrement` are counters (`c`), `gauge` a gauge (`g`),
 * `timing` a timer in milliseconds (`ms`), and `set` a unique-member set
 * (`s`). Needs the default `jennifer` binary (`net`).
 *
 * Optional StatsD / DogStatsD extensions are layered on top: a `|@rate`
 * sample-rate suffix (the `*Rate` verbs), `|#key:value` Datadog tags (the
 * `*Tagged` verbs, carrying a `map of string to string`), float-valued
 * counters / gauges (`countFloat` / `gaugeFloat`), and a `Batch` accumulator
 * that packs several metrics into one datagram (`batch` / `add*` / `flush`).
 * Every line - metric name, prefix, value, and tag keys / values - is
 * control-character validated before it reaches the wire, so untrusted request
 * data cannot forge extra metrics.
 * @module statsd
 * @example
 * import "statsd.j" as statsd;
 * def c as statsd.Client init statsd.clientWith("127.0.0.1:8125", "web");
 * statsd.increment($c, "requests");        # web.requests:1|c
 * statsd.timing($c, "response", 42);       # web.response:42|ms
 * statsd.gauge($c, "queue.depth", 7);      # web.queue.depth:7|g
 * def tags as map of string to string init {"env": "prod"};
 * statsd.countTagged($c, "hits", 1, $tags); # web.hits:1|c|#env:prod
 * statsd.countRate($c, "hits", 1, 0.1);     # web.hits:1|c|@0.1
 * statsd.close($c);
 */
use net;
use convert;
use strings;
use lists;

# The default StatsD UDP port used by the `client` convenience constructor.
def const DEFAULT_PORT as int init 8125;

/**
 * A StatsD client: a bound sending socket plus the agent address and an
 * optional metric-name prefix. Value-copies share the underlying socket (the
 * usual handle carve-out), so copying a `Client` is safe and cheap.
 * @field socket {net.UDPSocket} the sending socket
 * @field address {string} the agent "host:port"
 * @field prefix {string} a metric-name namespace ("" for none)
 */
export def struct Client {
    socket as net.UDPSocket,
    address as string,
    prefix as string
};

# --- name + line formatting (private, pure) ---------------------------------

# metricName joins an optional prefix to a metric name with a "." separator.
func metricName(prefix as string, name as string) {
    if (len($prefix) > 0) {
        return $prefix + "." + $name;
    }
    return $name;
}

# formatLine builds one StatsD wire line "[prefix.]name:value|type".
func formatLine(prefix as string, name as string, value as string, kind as string) {
    return metricName($prefix, $name) + ":" + $value + "|" + $kind;
}

# rateSuffix renders the StatsD "|@<rate>" sample-rate suffix. A rate of 1.0 (or
# any value >= 1) samples everything and emits no suffix. The rate is formatted
# compactly (convert.toString gives "0.1", not "0.100000").
func rateSuffix(rate as float) {
    if ($rate < 1.0) {
        return "|@" + convert.toString($rate);
    }
    return "";
}

# tagsSuffix renders the DogStatsD "|#k:v,k2:v2" tag suffix from a map (keys in
# insertion order). An empty map emits no suffix.
func tagsSuffix(tags as map of string to string) {
    if (len($tags) == 0) {
        return "";
    }
    def parts as list of string init [];
    for (def k in $tags) {
        $parts = lists.push($parts, $k + ":" + $tags[$k]);
    }
    return "|#" + strings.join($parts, ",");
}

# buildLine is the pure, network-free line builder: "[prefix.]name:value|type"
# plus an optional "|@rate" then "|#tags". Unit-tested directly by the overlay.
func buildLine(prefix as string, name as string, value as string, kind as string, rate as float, tags as map of string to string) {
    return formatLine($prefix, $name, $value, $kind) + rateSuffix($rate) + tagsSuffix($tags);
}

# checkName rejects a metric name (or prefix) with a character outside
# [A-Za-z0-9._-]. Reused for the client prefix so a hostile prefix cannot inject
# a separator onto the wire line.
func checkName(name as string) {
    def nb as bytes init convert.bytesFromString($name, "utf-8");
    def i as int init 0;
    while ($i < len($nb)) {
        def b as int init $nb[$i];
        def ok as bool init ($b >= 65 and $b <= 90) or ($b >= 97 and $b <= 122) or
            ($b >= 48 and $b <= 57) or $b == 46 or $b == 95 or $b == 45;
        if (not $ok) {
            throw Error{
                kind: "statsd",
                message: "metric name has an illegal character (allowed: letters, digits, and . _ -)",
                file: "",
                line: 0,
                col: 0
            };
        }
        $i = $i + 1;
    }
    return;
}

# checkValue rejects a value carrying a field / line separator (":", "|", "\n",
# "\r") that would forge extra metrics.
func checkValue(value as string) {
    def vb as bytes init convert.bytesFromString($value, "utf-8");
    def j as int init 0;
    while ($j < len($vb)) {
        def c as int init $vb[$j];
        if ($c == 58 or $c == 124 or $c == 10 or $c == 13) {
            throw Error{
                kind: "statsd",
                message: "metric value must not contain ':', '|', or a newline",
                file: "",
                line: 0,
                col: 0
            };
        }
        $j = $j + 1;
    }
    return;
}

# checkMetric rejects a name or value that would forge extra metrics. StatsD
# packs several metrics in one datagram separated by "\n", and ":" / "|" are the
# field separators, so a metric name outside [A-Za-z0-9._-] or a value carrying
# ":" "|" "\n" "\r" injects fabricated counters / gauges (OM-007). Both are the
# obvious place a program puts request data (`"http.ua." + $userAgent`).
func checkMetric(name as string, value as string) {
    checkName($name);
    checkValue($value);
    return;
}

# checkTagField rejects a control character, "|", or "," in a tag key or value
# (a colon too, when it is a key), so a tag cannot inject a newline / extra
# metric or an extra tag onto the wire line.
func checkTagField(field as string, isKey as bool) {
    def fb as bytes init convert.bytesFromString($field, "utf-8");
    def i as int init 0;
    while ($i < len($fb)) {
        def b as int init $fb[$i];
        def bad as bool init ($b < 32) or ($b == 127) or ($b == 124) or ($b == 44);
        if ($isKey and $b == 58) {
            $bad = true;
        }
        if ($bad) {
            throw Error{
                kind: "statsd",
                message: "tag key/value must not contain a control character, '|', ',', or (in a key) ':'",
                file: "",
                line: 0,
                col: 0
            };
        }
        $i = $i + 1;
    }
    return;
}

# checkTag validates one tag's key and value.
func checkTag(key as string, value as string) {
    checkTagField($key, true);
    checkTagField($value, false);
    return;
}

# validateMetric runs every wire-line check for one metric: the prefix and name
# (name charset), the value (no separators), and each tag key / value.
func validateMetric(prefix as string, name as string, value as string, tags as map of string to string) {
    if (len($prefix) > 0) {
        checkName($prefix);
    }
    checkMetric($name, $value);
    for (def k in $tags) {
        checkTag($k, $tags[$k]);
    }
    return;
}

# emitFull validates, builds, and sends one metric datagram with an optional
# sample rate and tag set (fire-and-forget).
func emitFull(c as Client, name as string, value as string, kind as string, rate as float, tags as map of string to string) {
    validateMetric($c.prefix, $name, $value, $tags);
    def line as string init buildLine($c.prefix, $name, $value, $kind, $rate, $tags);
    net.sendTo($c.socket, $c.address, convert.bytesFromString($line, "utf-8"));
    return;
}

# emit sends one plain metric datagram (no rate, no tags).
func emit(c as Client, name as string, value as string, kind as string) {
    emitFull($c, $name, $value, $kind, 1.0, {});
    return;
}

# --- constructors (exported) ------------------------------------------------

/**
 * Open a client to a StatsD agent on `host` at the default port (8125), with
 * no metric prefix.
 * @param host {string} the agent host (e.g. "127.0.0.1")
 * @return {Client} a ready-to-use client
 */
export func client(host as string) {
    return clientWith($host + ":" + convert.toString(DEFAULT_PORT), "");
}

/**
 * Open a client to a StatsD agent at a full `host:port` address, with a
 * metric-name prefix ("" for none). The prefix is joined to every metric name
 * with a "." (so prefix "web" and metric "hits" send "web.hits").
 * @param address {string} the agent "host:port"
 * @param prefix {string} a metric-name namespace ("" for none)
 * @return {Client} a ready-to-use client
 */
export func clientWith(address as string, prefix as string) {
    def sock as net.UDPSocket init net.listenUDP(":0");
    return Client{socket: $sock, address: $address, prefix: $prefix};
}

# --- metric verbs (exported) ------------------------------------------------

/**
 * Adjust a counter by `value` (may be negative). Sends "name:value|c".
 * @param c {Client} the client
 * @param name {string} the metric name
 * @param value {int} the counter delta
 */
export func count(c as Client, name as string, value as int) {
    emit($c, $name, convert.toString($value), "c");
}

/**
 * Increment a counter by 1. Sends "name:1|c".
 * @param c {Client} the client
 * @param name {string} the metric name
 */
export func increment(c as Client, name as string) {
    emit($c, $name, "1", "c");
}

/**
 * Decrement a counter by 1. Sends "name:-1|c".
 * @param c {Client} the client
 * @param name {string} the metric name
 */
export func decrement(c as Client, name as string) {
    emit($c, $name, "-1", "c");
}

/**
 * Set a gauge to an absolute value. Sends "name:value|g".
 * @param c {Client} the client
 * @param name {string} the metric name
 * @param value {int} the gauge value
 */
export func gauge(c as Client, name as string, value as int) {
    # StatsD reads a value with a leading sign as a *delta*, so a bare
    # "name:-5|g" would decrement the gauge by 5 rather than set it to -5. To
    # set an absolute negative value, first set the gauge to 0, then apply the
    # negative as a decrement.
    if ($value < 0) {
        emit($c, $name, "0", "g");
    }
    emit($c, $name, convert.toString($value), "g");
}

/**
 * Record a timing in milliseconds. Sends "name:ms|ms".
 * @param c {Client} the client
 * @param name {string} the metric name
 * @param ms {int} the duration in milliseconds
 */
export func timing(c as Client, name as string, ms as int) {
    emit($c, $name, convert.toString($ms), "ms");
}

/**
 * Record a unique member in a set (the agent counts distinct values). Sends
 * "name:value|s".
 * @param c {Client} the client
 * @param name {string} the metric name
 * @param value {string} the set member
 */
export func set(c as Client, name as string, value as string) {
    emit($c, $name, $value, "s");
}

# --- sample-rate verbs (exported) -------------------------------------------

/**
 * Adjust a counter by `value` at a sample `rate`. Sends "name:value|c|@rate"
 * (the "|@rate" suffix is omitted when `rate` >= 1). The agent scales the
 * received count back up by 1/rate, so a `rate` of 0.1 emitted from a tenth of
 * the events reconstructs the full total.
 * @param c {Client} the client
 * @param name {string} the metric name
 * @param value {int} the counter delta
 * @param rate {float} the sample rate in (0, 1]; >= 1 sends no suffix
 */
export func countRate(c as Client, name as string, value as int, rate as float) {
    emitFull($c, $name, convert.toString($value), "c", $rate, {});
}

/**
 * Record a timing in milliseconds at a sample `rate`. Sends "name:ms|ms|@rate"
 * (the "|@rate" suffix is omitted when `rate` >= 1).
 * @param c {Client} the client
 * @param name {string} the metric name
 * @param ms {int} the duration in milliseconds
 * @param rate {float} the sample rate in (0, 1]; >= 1 sends no suffix
 */
export func timingRate(c as Client, name as string, ms as int, rate as float) {
    emitFull($c, $name, convert.toString($ms), "ms", $rate, {});
}

# --- tagged (DogStatsD) verbs (exported) ------------------------------------

/**
 * Adjust a counter by `value`, carrying DogStatsD tags. Sends
 * "name:value|c|#k:v,...". Tag keys / values are control-character validated.
 * @param c {Client} the client
 * @param name {string} the metric name
 * @param value {int} the counter delta
 * @param tags {map of string to string} tags rendered as "|#k:v,..." (insertion order)
 */
export func countTagged(c as Client, name as string, value as int, tags as map of string to string) {
    emitFull($c, $name, convert.toString($value), "c", 1.0, $tags);
}

/**
 * Increment a counter by 1, carrying DogStatsD tags. Sends "name:1|c|#k:v,...".
 * @param c {Client} the client
 * @param name {string} the metric name
 * @param tags {map of string to string} tags rendered as "|#k:v,..." (insertion order)
 */
export func incrementTagged(c as Client, name as string, tags as map of string to string) {
    emitFull($c, $name, "1", "c", 1.0, $tags);
}

/**
 * Decrement a counter by 1, carrying DogStatsD tags. Sends "name:-1|c|#k:v,...".
 * @param c {Client} the client
 * @param name {string} the metric name
 * @param tags {map of string to string} tags rendered as "|#k:v,..." (insertion order)
 */
export func decrementTagged(c as Client, name as string, tags as map of string to string) {
    emitFull($c, $name, "-1", "c", 1.0, $tags);
}

/**
 * Set a gauge to an absolute value, carrying DogStatsD tags. Sends
 * "name:value|g|#k:v,...". A negative value is set as a 0-then-decrement pair
 * (see `gauge`), each line carrying the tags.
 * @param c {Client} the client
 * @param name {string} the metric name
 * @param value {int} the gauge value
 * @param tags {map of string to string} tags rendered as "|#k:v,..." (insertion order)
 */
export func gaugeTagged(c as Client, name as string, value as int, tags as map of string to string) {
    if ($value < 0) {
        emitFull($c, $name, "0", "g", 1.0, $tags);
    }
    emitFull($c, $name, convert.toString($value), "g", 1.0, $tags);
}

/**
 * Record a timing in milliseconds, carrying DogStatsD tags. Sends
 * "name:ms|ms|#k:v,...".
 * @param c {Client} the client
 * @param name {string} the metric name
 * @param ms {int} the duration in milliseconds
 * @param tags {map of string to string} tags rendered as "|#k:v,..." (insertion order)
 */
export func timingTagged(c as Client, name as string, ms as int, tags as map of string to string) {
    emitFull($c, $name, convert.toString($ms), "ms", 1.0, $tags);
}

/**
 * Record a unique set member, carrying DogStatsD tags. Sends
 * "name:value|s|#k:v,...".
 * @param c {Client} the client
 * @param name {string} the metric name
 * @param value {string} the set member
 * @param tags {map of string to string} tags rendered as "|#k:v,..." (insertion order)
 */
export func setTagged(c as Client, name as string, value as string, tags as map of string to string) {
    emitFull($c, $name, $value, "s", 1.0, $tags);
}

# --- float-valued verbs (exported) ------------------------------------------

/**
 * Adjust a counter by a fractional `value`. Sends "name:value|c" with the float
 * formatted compactly (e.g. "3.5", no trailing zeros beyond the canonical form).
 * @param c {Client} the client
 * @param name {string} the metric name
 * @param value {float} the counter delta
 */
export func countFloat(c as Client, name as string, value as float) {
    emitFull($c, $name, convert.toString($value), "c", 1.0, {});
}

/**
 * Set a gauge to a fractional absolute value (e.g. a load average). Sends
 * "name:value|g", the float formatted compactly. A negative value is set as a
 * 0-then-decrement pair (see `gauge`).
 * @param c {Client} the client
 * @param name {string} the metric name
 * @param value {float} the gauge value
 */
export func gaugeFloat(c as Client, name as string, value as float) {
    if ($value < 0.0) {
        emitFull($c, $name, "0", "g", 1.0, {});
    }
    emitFull($c, $name, convert.toString($value), "g", 1.0, {});
}

# --- packet batching (exported) ---------------------------------------------

/**
 * An accumulator of formatted metric lines to pack into one UDP datagram (the
 * StatsD multi-metric packet, lines separated by "\n"). Value-semantic: each
 * `add*` verb returns a fresh `Batch` with the line appended, leaving the input
 * unchanged. `flush` sends the whole packet through a client. The prefix is
 * captured from the client at `batch` time and applied to every line.
 * @field prefix {string} the metric-name namespace copied from the client
 * @field lines {list of string} the accumulated wire lines
 */
export def struct Batch {
    prefix as string,
    lines as list of string
};

/**
 * Start an empty `Batch`, capturing the client's metric-name prefix.
 * @param c {Client} the client whose prefix the batched lines inherit
 * @return {Batch} an empty accumulator
 */
export func batch(c as Client) {
    return Batch{prefix: $c.prefix, lines: []};
}

# addLine validates one metric and appends its formatted wire line to a fresh
# copy of the batch (value-semantic builder). Each line is individually checked.
func addLine(b as Batch, name as string, value as string, kind as string) {
    validateMetric($b.prefix, $name, $value, {});
    def line as string init buildLine($b.prefix, $name, $value, $kind, 1.0, {});
    def out as Batch init $b;
    $out.lines = lists.push($out.lines, $line);
    return $out;
}

/**
 * Append a counter delta to the batch. Returns a new `Batch`.
 * @param b {Batch} the accumulator
 * @param name {string} the metric name
 * @param value {int} the counter delta
 * @return {Batch} a new accumulator with "name:value|c" appended
 */
export func addCount(b as Batch, name as string, value as int) {
    return addLine($b, $name, convert.toString($value), "c");
}

/**
 * Append a counter +1 to the batch. Returns a new `Batch`.
 * @param b {Batch} the accumulator
 * @param name {string} the metric name
 * @return {Batch} a new accumulator with "name:1|c" appended
 */
export func addIncrement(b as Batch, name as string) {
    return addLine($b, $name, "1", "c");
}

/**
 * Append a counter -1 to the batch. Returns a new `Batch`.
 * @param b {Batch} the accumulator
 * @param name {string} the metric name
 * @return {Batch} a new accumulator with "name:-1|c" appended
 */
export func addDecrement(b as Batch, name as string) {
    return addLine($b, $name, "-1", "c");
}

/**
 * Append an absolute gauge to the batch. A negative value appends a
 * 0-then-decrement pair (see `gauge`). Returns a new `Batch`.
 * @param b {Batch} the accumulator
 * @param name {string} the metric name
 * @param value {int} the gauge value
 * @return {Batch} a new accumulator with "name:value|g" appended
 */
export func addGauge(b as Batch, name as string, value as int) {
    if ($value < 0) {
        def mid as Batch init addLine($b, $name, "0", "g");
        return addLine($mid, $name, convert.toString($value), "g");
    }
    return addLine($b, $name, convert.toString($value), "g");
}

/**
 * Append a timing (milliseconds) to the batch. Returns a new `Batch`.
 * @param b {Batch} the accumulator
 * @param name {string} the metric name
 * @param ms {int} the duration in milliseconds
 * @return {Batch} a new accumulator with "name:ms|ms" appended
 */
export func addTiming(b as Batch, name as string, ms as int) {
    return addLine($b, $name, convert.toString($ms), "ms");
}

/**
 * Append a unique set member to the batch. Returns a new `Batch`.
 * @param b {Batch} the accumulator
 * @param name {string} the metric name
 * @param value {string} the set member
 * @return {Batch} a new accumulator with "name:value|s" appended
 */
export func addSet(b as Batch, name as string, value as string) {
    return addLine($b, $name, $value, "s");
}

/**
 * Send every accumulated line in one UDP datagram (lines joined by "\n").
 * An empty batch sends nothing. Fire-and-forget, like the single-metric verbs.
 * @param c {Client} the client
 * @param b {Batch} the accumulator to flush
 */
export func flush(c as Client, b as Batch) {
    if (len($b.lines) == 0) {
        return;
    }
    def packet as string init strings.join($b.lines, "\n");
    net.sendTo($c.socket, $c.address, convert.bytesFromString($packet, "utf-8"));
}

/**
 * Close the client's sending socket.
 * @param c {Client} the client
 */
export func close(c as Client) {
    net.close($c.socket);
}
