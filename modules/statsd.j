# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

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
 * (`s`). Needs the default `jennifer` binary (`net`). Sample rates and
 * Datadog tags are not in this version.
 * @module statsd
 * @example
 * import "statsd.j" as statsd;
 * def c as statsd.Client init statsd.clientWith("127.0.0.1:8125", "web");
 * statsd.increment($c, "requests");        # web.requests:1|c
 * statsd.timing($c, "response", 42);       # web.response:42|ms
 * statsd.gauge($c, "queue.depth", 7);      # web.queue.depth:7|g
 * statsd.close($c);
 */
use net;
use convert;

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

# emit sends one metric datagram to the client's agent (fire-and-forget).
func emit(c as Client, name as string, value as string, kind as string) {
    def line as string init formatLine($c.prefix, $name, $value, $kind);
    net.sendTo($c.socket, $c.address, convert.bytesFromString($line, "utf-8"));
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
    return Client{ socket: $sock, address: $address, prefix: $prefix };
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

/**
 * Close the client's sending socket.
 * @param c {Client} the client
 */
export func close(c as Client) {
    net.close($c.socket);
}
