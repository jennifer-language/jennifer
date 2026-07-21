# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# log_test.j - white-box tests for log.j. Run with:
#
#     jennifer test modules/log_test.j
#
# The overlay splices log.j in first, so these tests reach its private helpers
# (render, quoteIfNeeded, shouldLog, syslogLine) with a fixed timestamp for
# determinism. The sinks (stdout / stderr / file / syslog) are exercised in the
# Go suite (TestLog). log.j already `use`s io / fs / net / json / strings /
# convert / time / os, so the overlay only adds testing.
use testing;

func fixed() {
    return time.fromIso("2026-01-02T03:04:05Z");
}

func fields() {
    def f as map of string to string init {"user": "ada", "id": "42"};
    return $f;
}

func none() {
    def f as map of string to string init {};
    return $f;
}

func testRenderText() {
    def lg as Logger init new("info", "text");
    testing.assertEqual(render($lg, "info", "hello", fields(), fixed()),
        "2026-01-02T03:04:05Z INFO hello user=ada id=42");
}

func testRenderLogfmt() {
    def lg as Logger init new("info", "logfmt");
    testing.assertEqual(render($lg, "warn", "disk low", fields(), fixed()),
        "time=2026-01-02T03:04:05Z level=warn msg=\"disk low\" user=ada id=42");
}

func testRenderJson() {
    def lg as Logger init new("info", "json");
    testing.assertEqual(render($lg, "error", "boom", none(), fixed()),
        "{\"time\":\"2026-01-02T03:04:05Z\",\"level\":\"error\",\"msg\":\"boom\"}");
    testing.assertEqual(render($lg, "info", "hi", fields(), fixed()),
        "{\"time\":\"2026-01-02T03:04:05Z\",\"level\":\"info\",\"msg\":\"hi\",\"user\":\"ada\",\"id\":\"42\"}");
}

func testQuoteIfNeeded() {
    testing.assertEqual(quoteIfNeeded("plain"), "plain");
    testing.assertEqual(quoteIfNeeded("a b"), "\"a b\"");
    testing.assertEqual(quoteIfNeeded("k=v"), "\"k=v\"");
    testing.assertEqual(quoteIfNeeded("say \"hi\""), "\"say \\\"hi\\\"\"");
}

# A backslash must be escaped (and escaped first, so it does not double up with
# the quote escape), an embedded newline must be encoded (not left to forge a
# second record), and an empty value must be quoted so the field stays present.
func testQuoteEscapesBackslashAndNewline() {
    testing.assertEqual(quoteIfNeeded("a\\b"), "\"a\\\\b\"");
    testing.assertEqual(quoteIfNeeded("c:\\path\""), "\"c:\\\\path\\\"\"");
    testing.assertEqual(quoteIfNeeded("line1\nline2"), "\"line1\\nline2\"");
    testing.assertEqual(quoteIfNeeded(""), "\"\"");
}

# A field value carrying a newline must not split the rendered record into two
# lines (classic log injection).
func testRenderTextNoInjection() {
    def fields as map of string to string init {"user": "eve\ninjected=evil"};
    def line as string init renderText("info", "hello", $fields, "T");
    testing.assertFalse(strings.contains($line, "\n"));
    # a message newline is neutralised too
    def m as string init renderText("info", "one\ntwo", {}, "T");
    testing.assertFalse(strings.contains($m, "\n"));
}

func testLevelFiltering() {
    def lg as Logger init new("warn", "text");
    testing.assertFalse(shouldLog($lg, "debug"));
    testing.assertFalse(shouldLog($lg, "info"));
    testing.assertTrue(shouldLog($lg, "warn"));
    testing.assertTrue(shouldLog($lg, "error"));
    # a debug logger passes everything
    testing.assertTrue(shouldLog(new("debug", "text"), "debug"));
}

func testSyslogLine() {
    def lg as Logger init toSyslog("info", "localhost:514", "myapp");
    def line as string init syslogLine($lg, "error", "boom", fields(), fixed());
    # PRI = facility 1 * 8 + severity 3 (err) = 11; RFC 5424 structure after the host.
    testing.assertTrue(strings.startsWith($line, "<11>1 2026-01-02T03:04:05Z "));
    testing.assertTrue(strings.contains($line, " myapp - - - boom user=ada id=42"));
    # warn -> severity 4 -> PRI 12
    testing.assertTrue(strings.startsWith(syslogLine($lg, "warn", "x", none(), fixed()), "<12>1 "));
}

func testConstructors() {
    testing.assertEqual(new("info", "text").sink, "stdout");
    testing.assertEqual(toStderr("info", "text").sink, "stderr");
    def fl as Logger init toFile("info", "json", "/tmp/x.log");
    testing.assertEqual($fl.sink, "file");
    testing.assertEqual($fl.target, "/tmp/x.log");
    def sl as Logger init toSyslog("info", "h:514", "app");
    testing.assertEqual($sl.sink, "syslog");
    testing.assertEqual($sl.target, "h:514");
    testing.assertEqual($sl.app, "app");
}
