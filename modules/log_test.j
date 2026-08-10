# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
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
    testing.assertEqual(
        render($lg, "info", "hello", fields(), fixed()),
        "2026-01-02T03:04:05Z INFO hello user=ada id=42");
}

func testRenderLogfmt() {
    def lg as Logger init new("info", "logfmt");
    testing.assertEqual(
        render($lg, "warn", "disk low", fields(), fixed()),
        "time=2026-01-02T03:04:05Z level=warn msg=\"disk low\" user=ada id=42");
}

func testRenderJson() {
    def lg as Logger init new("info", "json");
    testing.assertEqual(
        render($lg, "error", "boom", none(), fixed()),
        '{"time":"2026-01-02T03:04:05Z","level":"error","msg":"boom"}');
    testing.assertEqual(
        render($lg, "info", "hi", fields(), fixed()),
        '{"time":"2026-01-02T03:04:05Z","level":"info","msg":"hi","user":"ada","id":"42"}');
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

# mergeFields overlays extra over base; a per-call key wins on collision.
func testMergeFields() {
    def m as map of string to string init mergeFields({"user": "ada"}, {"user": "bob", "x": "1"});
    testing.assertEqual($m["user"], "bob");
    testing.assertEqual($m["x"], "1");
    # base is preserved when extra is empty
    def b as map of string to string init mergeFields({"a": "1"}, {});
    testing.assertEqual($b["a"], "1");
}

# with stores persistent context fields on the returned logger.
func testWithStoresFields() {
    def lg as Logger init with(new("info", "text"), fields());
    testing.assertEqual($lg.fields["user"], "ada");
    testing.assertEqual($lg.fields["id"], "42");
    # the parent is unchanged (value semantics)
    testing.assertEqual(len(new("info", "text").fields), 0);
}

# with composes: a second with merges on top of the carried fields.
func testWithComposes() {
    def a as Logger init with(new("info", "text"), {"a": "1"});
    def b as Logger init with($a, {"b": "2"});
    testing.assertEqual($b.fields["a"], "1");
    testing.assertEqual($b.fields["b"], "2");
    # a later with of the same key wins
    def c as Logger init with($b, {"a": "9"});
    testing.assertEqual($c.fields["a"], "9");
}

# fatal ranks above every level, so it clears any threshold and always logs.
func testFatalAlwaysLogs() {
    testing.assertTrue(shouldLog(new("error", "text"), "fatal"));
    testing.assertTrue(shouldLog(new("info", "text"), "fatal"));
    testing.assertTrue(shouldLog(new("debug", "text"), "fatal"));
}

# fatal maps to RFC 5424 severity critical (2), just above error (3).
func testFatalSeverity() {
    testing.assertEqual(syslogSeverity("fatal"), 2);
    testing.assertEqual(syslogSeverity("error"), 3);
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

# --- emit paths via a temp file + the stdout/stderr sinks ---
# log.j `use`s fs / strings / io, so the overlay reaches them directly. fatal()
# is skipped here (it calls exit 1).

func testFileLoggerWritesLevels() {
    def p as string init fs.makeTempFile("", "log-", ".log");
    def lg as Logger init toFile("debug", "text", $p);
    def empty as map of string to string;
    debug($lg, "tracing", $empty);
    info($lg, "hello", $empty);
    warn($lg, "careful", $empty);
    error($lg, "boom", $empty);
    def content as string init fs.readString($p);
    fs.remove($p);
    testing.assertTrue(strings.contains($content, "hello"));
    testing.assertTrue(strings.contains($content, "boom"));
}

func testLevelFilteringDropsBelowThreshold() {
    def p as string init fs.makeTempFile("", "log-lvl-", ".log");
    def lg as Logger init toFile("error", "text", $p);
    def empty as map of string to string;
    info($lg, "should-drop", $empty);   # below the error threshold
    error($lg, "should-keep", $empty);
    def content as string init fs.readString($p);
    fs.remove($p);
    testing.assertFalse(strings.contains($content, "should-drop"));
    testing.assertTrue(strings.contains($content, "should-keep"));
}

func testChildLoggerAddsFields() {
    def p as string init fs.makeTempFile("", "log-child-", ".log");
    def base as Logger init toFile("info", "logfmt", $p);
    def fields as map of string to string;
    $fields["req"] = "42";
    def child as Logger init with($base, $fields);
    def empty as map of string to string;
    info($child, "handled", $empty);
    def content as string init fs.readString($p);
    fs.remove($p);
    testing.assertTrue(strings.contains($content, "req"));
    testing.assertTrue(strings.contains($content, "42"));
}

func testStdoutAndStderrSinks() {
    def empty as map of string to string;
    info(new("info", "text"), "to stdout", $empty);       # io.printf branch
    info(toStderr("info", "text"), "to stderr", $empty);  # io.eprintf branch
    testing.assertTrue(true);
}
