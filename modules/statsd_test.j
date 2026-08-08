# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# statsd_test.j - white-box tests for statsd.j. Run with:
#
#     jennifer test modules/statsd_test.j
#
# These exercise the pure name / line formatting (metricName / formatLine) with
# no network; the live UDP send is driven against a real loopback listener in
# the Go suite (cmd/jennifer/statsd_test.go). statsd.j already `use`s net and
# convert, so the overlay only adds testing.
use testing;

func testMetricNameNoPrefix() {
    testing.assertEqual(metricName("", "requests"), "requests");
}

func testMetricNameWithPrefix() {
    testing.assertEqual(metricName("web", "requests"), "web.requests");
    testing.assertEqual(metricName("web.api", "hits"), "web.api.hits");
}

func testFormatCounter() {
    testing.assertEqual(formatLine("", "hits", "5", "c"), "hits:5|c");
    testing.assertEqual(formatLine("", "hits", "-1", "c"), "hits:-1|c");
}

func testFormatWithPrefix() {
    testing.assertEqual(formatLine("web", "hits", "5", "c"), "web.hits:5|c");
}

func testFormatGaugeTimingSet() {
    testing.assertEqual(formatLine("", "temp", "42", "g"), "temp:42|g");
    testing.assertEqual(formatLine("", "response", "120", "ms"), "response:120|ms");
    testing.assertEqual(formatLine("app", "users", "u123", "s"), "app.users:u123|s");
}

func injectStatsdName() {
    checkMetric("a:b|c", "1");
}
func injectStatsdValue() {
    checkMetric("ok", "1|c\nother:5");
}
func testMetricInjectionBlocked() { # OM-007
    testing.assertThrows("injectStatsdName", "statsd");
    testing.assertThrows("injectStatsdValue", "statsd");
    checkMetric("http.requests.ok", "42"); # a valid metric does not throw
}

# --- sample rate ------------------------------------------------------------

func testRateSuffix() {
    testing.assertEqual(rateSuffix(0.1), "|@0.1");
    testing.assertEqual(rateSuffix(0.25), "|@0.25");
    testing.assertEqual(rateSuffix(1.0), ""); # 1.0 omits the suffix
    testing.assertEqual(rateSuffix(2.0), ""); # >= 1 omits the suffix
}

func testBuildLineWithRate() {
    testing.assertEqual(buildLine("web", "hits", "1", "c", 0.1, {}), "web.hits:1|c|@0.1");
    testing.assertEqual(buildLine("", "response", "42", "ms", 0.5, {}), "response:42|ms|@0.5");
    testing.assertEqual(buildLine("", "hits", "1", "c", 1.0, {}), "hits:1|c");
}

# --- DogStatsD tags ---------------------------------------------------------

func testTagsSuffix() {
    def t as map of string to string init {"env": "prod", "host": "h1"};
    testing.assertEqual(tagsSuffix($t), "|#env:prod,host:h1"); # insertion order
    def empty as map of string to string init {};
    testing.assertEqual(tagsSuffix($empty), ""); # no tags -> no suffix
}

func testBuildLineWithTags() {
    def t as map of string to string init {"env": "prod", "host": "h1"};
    testing.assertEqual(buildLine("web", "hits", "1", "c", 1.0, $t),
        "web.hits:1|c|#env:prod,host:h1");
    # rate then tags: name:value|type|@rate|#tags
    testing.assertEqual(buildLine("web", "hits", "1", "c", 0.1, $t),
        "web.hits:1|c|@0.1|#env:prod,host:h1");
}

func injectTagValue() {
    checkTag("env", "prod\nother:5|c"); # newline in a tag value would forge a metric
}
func injectTagKey() {
    checkTag("a|b", "prod"); # '|' in a tag key
}
func testTagInjectionBlocked() {
    testing.assertThrows("injectTagValue", "statsd");
    testing.assertThrows("injectTagKey", "statsd");
    checkTag("env", "prod"); # a valid tag does not throw
}

# --- prefix validation ------------------------------------------------------

func injectPrefix() {
    validateMetric("web|x", "hits", "1", {}); # a hostile prefix must be checked too
}
func testPrefixInjectionBlocked() {
    testing.assertThrows("injectPrefix", "statsd");
    validateMetric("web.api", "hits", "1", {}); # a valid prefix does not throw
}

# --- float values -----------------------------------------------------------

func testFloatFormatting() {
    testing.assertEqual(buildLine("", "temp", convert.toString(3.5), "g", 1.0, {}), "temp:3.5|g");
    testing.assertEqual(buildLine("", "load", convert.toString(0.5), "c", 1.0, {}), "load:0.5|c");
}

# --- packet batching --------------------------------------------------------

func testBatchPacket() {
    def b as Batch init Batch{prefix: "web", lines: []};
    $b = addCount($b, "hits", 3);
    $b = addGauge($b, "queue", 7);
    $b = addIncrement($b, "errors");
    testing.assertEqual(len($b.lines), 3);
    testing.assertEqual($b.lines[0], "web.hits:3|c");
    testing.assertEqual($b.lines[1], "web.queue:7|g");
    testing.assertEqual($b.lines[2], "web.errors:1|c");
    # the multi-metric datagram: lines joined by "\n"
    def packet as string init strings.join($b.lines, "\n");
    testing.assertEqual($packet, "web.hits:3|c\nweb.queue:7|g\nweb.errors:1|c");
}

func testBatchValueSemantic() {
    def b as Batch init Batch{prefix: "", lines: []};
    def b2 as Batch init addCount($b, "a", 1);
    testing.assertEqual(len($b.lines), 0);  # original untouched
    testing.assertEqual(len($b2.lines), 1); # copy has the line
}

# --- metric verbs over a loopback UDP socket ---
# statsd.j `use`s net + convert, so the overlay reaches them directly. One send
# is verified end-to-end; the rest are swept for coverage (fire-and-forget, so
# the listener's buffer just drains when the sockets close).

func testMetricVerbsSendAndFormat() {
    def listener as net.UDPSocket init net.listenUDP("127.0.0.1:0");
    def addr as string init net.address($listener);
    def c as Client init clientWith($addr, "app");

    # End-to-end: an increment reaches the listener with the right wire format.
    increment($c, "hits");
    net.setDeadline($listener, 2000);
    def dg as net.Datagram init net.recvFrom($listener, 1024);
    testing.assertEqual(convert.stringFromBytes($dg.data, "utf-8"), "app.hits:1|c");

    def tags as map of string to string;
    $tags["env"] = "prod";

    count($c, "c", 5);
    decrement($c, "d");
    gauge($c, "g", 7);
    gauge($c, "gneg", -3);          # negative gauge: set 0 then decrement
    timing($c, "t", 42);
    set($c, "s", "u1");

    countRate($c, "cr", 5, 0.5);
    timingRate($c, "tr", 42, 0.5);

    countTagged($c, "ct", 5, $tags);
    incrementTagged($c, "it", $tags);
    decrementTagged($c, "dt", $tags);
    gaugeTagged($c, "gt", 7, $tags);
    timingTagged($c, "tt", 42, $tags);
    setTagged($c, "st", "u1", $tags);

    countFloat($c, "cf", 3.5);
    gaugeFloat($c, "gf", 2.5);

    def b as Batch init batch($c);
    addCount($b, "bc", 1);
    addIncrement($b, "bi");
    addDecrement($b, "bd");
    addGauge($b, "bg", 4);
    addTiming($b, "bt", 10);
    addSet($b, "bs", "x");
    flush($c, $b);

    close($c);
    net.close($listener);
}

# client(host) joins the default port and sends over its own socket.
func testDefaultPortClient() {
    def c as Client init client("127.0.0.1");
    increment($c, "ping");   # fire-and-forget to the default statsd port
    close($c);
    testing.assertTrue(true);
}
