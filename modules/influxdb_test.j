# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# influxdb_test.j - white-box tests for influxdb.j. Run with:
#
#     jennifer test modules/influxdb_test.j
#
# These exercise the pure line-protocol rendering / escaping and the query-JSON
# parsing with no network; the live write / query over http is driven against a
# fake HTTP server in the Go suite (cmd/jennifer/influxdb_test.go). influxdb.j
# already `use`s json / strings / convert / lists / time / encoding, so the
# overlay only adds testing.
use testing;

func testLineBasic() {
    def p as Point init field(point("cpu"), "value", 0.5);
    testing.assertEqual(line($p), "cpu value=0.5");
}

# A newline in a tag value (or any component) must be rejected: it would split
# the point's line and corrupt the whole batch.
func lineWithNewlineTag() {
    def p as Point init field(tag(point("cpu"), "host", "a\nb"), "value", 1.0);
    line($p);
}
func testLineRejectsNewline() {
    testing.assertThrows("lineWithNewlineTag", "influxdb");
}

func testLineTagsSorted() {
    def p as Point init point("cpu");
    $p = tag($p, "host", "a");
    $p = field($p, "value", 1.0);
    testing.assertEqual(line($p), "cpu,host=a value=1.0");
}

func testLineFieldTypes() {
    def p as Point init point("m");
    $p = intField($p, "n", 5);
    $p = boolField($p, "up", true);
    $p = boolField($p, "down", false);
    $p = stringField($p, "s", "hi");
    testing.assertEqual(line($p), "m n=5i,up=true,down=false,s=\"hi\"");
}

func testLineTimestamp() {
    def p as Point init at(field(point("m"), "v", 2.0), 1434055562000000000);
    testing.assertEqual(line($p), "m v=2.0 1434055562000000000");
}

func testLineEscaping() {
    def p as Point init point("cpu load");
    $p = tag($p, "region", "us west");
    $p = tag($p, "host", "srv,1");
    $p = field($p, "a=b", 1.0);
    testing.assertEqual(line($p), "cpu\\ load,region=us\\ west,host=srv\\,1 a\\=b=1.0");
}

func testStringFieldEscaping() {
    def p as Point init stringField(point("m"), "note", "he said \"hi\"\\done");
    testing.assertEqual(line($p), "m note=\"he said \\\"hi\\\"\\\\done\"");
}

func testLineNoFieldsThrows() {
    def threw as bool init false;
    try {
        line(point("m"));
    } catch (e) {
        $threw = true;
        testing.assertEqual($e.kind, "influxdb");
    }
    testing.assertTrue($threw);
}

func testEscapeHelpers() {
    testing.assertEqual(escapeMeasurement("a b,c"), "a\\ b\\,c");
    testing.assertEqual(escapeKey("a b,c=d"), "a\\ b\\,c\\=d");
    testing.assertEqual(escapeStringField("x"), "\"x\"");
}

func testCellString() {
    def n as json.Value init json.decode("{\"s\":\"txt\",\"i\":42,\"f\":3.5,\"b\":true,\"z\":null}");
    testing.assertEqual(cellString($n, "/s"), "txt");
    testing.assertEqual(cellString($n, "/i"), "42");
    testing.assertEqual(cellString($n, "/f"), "3.5");
    testing.assertEqual(cellString($n, "/b"), "true");
    testing.assertEqual(cellString($n, "/z"), "");
}

func testParseQuery() {
    def body as string init "{\"results\":[{\"statement_id\":0,\"series\":[{\"name\":\"cpu\",\"tags\":{\"host\":\"a\"},\"columns\":[\"time\",\"value\"],\"values\":[[\"2021-01-01T00:00:00Z\",0.5],[\"2021-01-01T00:01:00Z\",0.7]]}]}]}";
    def r as Result init parseQuery(json.decode($body));
    testing.assertEqual(len($r.series), 1);
    testing.assertEqual($r.series[0].name, "cpu");
    testing.assertEqual($r.series[0].tags["host"], "a");
    testing.assertEqual(len($r.series[0].columns), 2);
    testing.assertEqual($r.series[0].columns[1], "value");
    testing.assertEqual(len($r.series[0].values), 2);
    testing.assertEqual($r.series[0].values[0][0], "2021-01-01T00:00:00Z");
    testing.assertEqual($r.series[0].values[1][1], "0.7");
}

func testParseQueryError() {
    def body as string init "{\"results\":[{\"statement_id\":0,\"error\":\"database not found: nope\"}]}";
    def threw as bool init false;
    try {
        parseQuery(json.decode($body));
    } catch (e) {
        $threw = true;
        testing.assertEqual($e.kind, "influxdb");
    }
    testing.assertTrue($threw);
}

func testParseQueryEmpty() {
    # a statement with no series (e.g. a write-style statement) yields no series
    def r as Result init parseQuery(json.decode("{\"results\":[{\"statement_id\":0}]}"));
    testing.assertEqual(len($r.series), 0);
}
