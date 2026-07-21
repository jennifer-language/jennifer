# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
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
