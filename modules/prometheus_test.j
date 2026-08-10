# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# prometheus_test.j - white-box tests for prometheus.j. Run with:
#
#     jennifer test modules/prometheus_test.j
#
# The overlay splices prometheus.j in front of this file, so the tests reach
# its private helpers (escapeLabelValue, isValidName, parseResult) by bare
# identifier. The networked query / queryRange path is covered end to end
# against an in-process fake Prometheus in the Go suite (TestPrometheusQuery).
use testing;

# --- exposition -------------------------------------------------------------

func testRenderCounterWithLabels() {
    def m as Metric init counter("http_requests_total", "Total HTTP requests");
    $m = observe($m, {"method": "get", "code": "200"}, 42.0);
    def want as string init "# HELP http_requests_total Total HTTP requests\n# TYPE http_requests_total counter\nhttp_requests_total\{code=\"200\",method=\"get\"\} 42.0\n";
    testing.assertEqual(render([$m]), $want);
}

func testRenderNoLabelsNoHelp() {
    def m as Metric init gauge("up", "");
    $m = observe($m, {}, 1.0);
    testing.assertEqual(render([$m]), "# TYPE up gauge\nup 1.0\n");
}

func testObserveUpsertReplacesSameLabelSet() {
    def m as Metric init gauge("temp", "");
    $m = observe($m, {"room": "kitchen"}, 20.0);
    $m = observe($m, {"room": "kitchen"}, 21.5);
    $m = observe($m, {"room": "hall"}, 18.0);
    testing.assertEqual(len($m.samples), 2);
    testing.assertEqual(
        render([$m]),
        "# TYPE temp gauge\ntemp\{room=\"kitchen\"\} 21.5\ntemp\{room=\"hall\"\} 18.0\n");
}

func testLabelKeysSortDeterministically() {
    def m as Metric init counter("c", "");
    $m = observe($m, {"z": "1", "a": "2"}, 5.0);
    testing.assertEqual(render([$m]), "# TYPE c counter\nc\{a=\"2\",z=\"1\"\} 5.0\n");
}

func testEscapeLabelValueAndHelp() {
    testing.assertEqual(escapeLabelValue("a\\b\"c"), "a\\\\b\\\"c");
    testing.assertEqual(escapeHelp("line\\one"), "line\\\\one");
}

func testValidName() {
    testing.assertTrue(isValidName("http_requests_total"));
    testing.assertTrue(isValidName("my:metric"));
    testing.assertFalse(isValidName("bad-name"));
    testing.assertFalse(isValidName("1leading"));
    testing.assertFalse(isValidName(""));
}

func badMetricName() {
    counter("bad-name", "");
}

func badLabelName() {
    def m as Metric init counter("ok", "");
    observe($m, {"bad-label": "x"}, 1.0);
}

func testInvalidNamesThrow() {
    testing.assertThrows("badMetricName", "prometheus");
    testing.assertThrows("badLabelName", "prometheus");
}

# --- histogram --------------------------------------------------------------

func testRenderHistogram() {
    def h as Metric init histogram("http_request_duration_seconds", "Request latency", [0.1, 0.5, 1.0]);
    $h = observe($h, {"method": "get"}, 0.3);
    $h = observe($h, {"method": "get"}, 0.05);
    $h = observe($h, {"method": "get"}, 2.0);
    def want as string init "# HELP http_request_duration_seconds Request latency\n# TYPE http_request_duration_seconds histogram\nhttp_request_duration_seconds_bucket\{le=\"0.1\",method=\"get\"\} 1.0\nhttp_request_duration_seconds_bucket\{le=\"0.5\",method=\"get\"\} 2.0\nhttp_request_duration_seconds_bucket\{le=\"1.0\",method=\"get\"\} 2.0\nhttp_request_duration_seconds_bucket\{le=\"+Inf\",method=\"get\"\} 3.0\nhttp_request_duration_seconds_sum\{method=\"get\"\} 2.35\nhttp_request_duration_seconds_count\{method=\"get\"\} 3.0\n";
    testing.assertEqual(render([$h]), $want);
}

func testHistogramBucketsSortAscending() {
    # Buckets given out of order are sorted ascending, and `+Inf` renders last.
    def h as Metric init histogram("lat", "", [1.0, 0.5, 0.1]);
    $h = observe($h, {}, 0.05);
    testing.assertEqual(
        render([$h]),
        "# TYPE lat histogram\nlat_bucket\{le=\"0.1\"\} 1.0\nlat_bucket\{le=\"0.5\"\} 1.0\nlat_bucket\{le=\"1.0\"\} 1.0\nlat_bucket\{le=\"+Inf\"\} 1.0\nlat_sum 0.05\nlat_count 1.0\n");
}

func testHistogramSeparatesLabelSets() {
    # Two label sets accumulate independently.
    def h as Metric init histogram("d", "", [1.0]);
    $h = observe($h, {"path": "a"}, 0.5);
    $h = observe($h, {"path": "b"}, 5.0);
    $h = observe($h, {"path": "a"}, 0.5);
    testing.assertEqual(len($h.samples), 2);
    testing.assertEqual($h.samples[0].count, 2.0);
    testing.assertEqual($h.samples[0].buckets[0], 2.0);
    testing.assertEqual($h.samples[1].count, 1.0);
    testing.assertEqual($h.samples[1].buckets[0], 0.0);
}

# --- summary ----------------------------------------------------------------

func testRenderSummary() {
    def s as Metric init summary("rpc_duration_seconds", "RPC latency", [0.5, 0.9, 0.99]);
    def i as int init 1;
    while ($i <= 10) {
        $s = observe($s, {}, convert.toFloat($i));
        $i = $i + 1;
    }
    def want as string init "# HELP rpc_duration_seconds RPC latency\n# TYPE rpc_duration_seconds summary\nrpc_duration_seconds\{quantile=\"0.5\"\} 5.0\nrpc_duration_seconds\{quantile=\"0.9\"\} 9.0\nrpc_duration_seconds\{quantile=\"0.99\"\} 10.0\nrpc_duration_seconds_sum 55.0\nrpc_duration_seconds_count 10.0\n";
    testing.assertEqual(render([$s]), $want);
}

func badQuantile() {
    summary("s", "", [1.5]);
}

func testSummaryQuantileOutOfRangeThrows() {
    testing.assertThrows("badQuantile", "prometheus");
}

# --- timestamps -------------------------------------------------------------

func testObserveAtAppendsTimestamp() {
    def g as Metric init gauge("up", "");
    $g = observeAt($g, {"job": "api"}, 1.0, 1700000000000);
    testing.assertEqual(render([$g]), "# TYPE up gauge\nup\{job=\"api\"\} 1.0 1700000000000\n");
}

func testObserveHasNoTimestamp() {
    def g as Metric init gauge("up", "");
    $g = observe($g, {}, 1.0);
    testing.assertEqual(render([$g]), "# TYPE up gauge\nup 1.0\n");
}

# --- pushgateway path -------------------------------------------------------

func testPushgatewayPath() {
    # Grouping keys are sorted; the job and label values are percent-encoded.
    testing.assertEqual(
        pushgatewayPath("my job", {"instance": "host:9090", "az": "eu-1"}),
        "/metrics/job/my%20job/az/eu-1/instance/host%3A9090");
}

func testPushgatewayPathNoGrouping() {
    testing.assertEqual(pushgatewayPath("batch", {}), "/metrics/job/batch");
}

func pushBadLabel() {
    pushgatewayPath("job", {"bad-label": "x"});
}

func testPushgatewayPathBadLabelThrows() {
    testing.assertThrows("pushBadLabel", "prometheus");
}

# --- retrieval parsing (canned responses) -----------------------------------

func testParseVectorResult() {
    def body as string init '{"status":"success","data":{"resultType":"vector","result":[{"metric":{"__name__":"up","job":"api"},"value":[1700000000.5,"1"]}]}}';
    def r as Result init parseResult(json.decode($body));
    testing.assertEqual($r.resultType, "vector");
    testing.assertEqual(len($r.series), 1);
    testing.assertEqual($r.series[0].metric["job"], "api");
    testing.assertEqual(len($r.series[0].values), 1);
    testing.assertEqual($r.series[0].values[0].value, 1.0);
    testing.assertEqual($r.series[0].values[0].timestamp, 1700000000.5);
}

func testParseMatrixResult() {
    def body as string init '{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"job":"api"},"values":[[1700000000,"1"],[1700000015,"2"]]}]}}';
    def r as Result init parseResult(json.decode($body));
    testing.assertEqual($r.resultType, "matrix");
    testing.assertEqual(len($r.series[0].values), 2);
    testing.assertEqual($r.series[0].values[1].value, 2.0);
}

func testParseScalarResult() {
    def body as string init '{"status":"success","data":{"resultType":"scalar","result":[1700000000,"7"]}}';
    def r as Result init parseResult(json.decode($body));
    testing.assertEqual($r.resultType, "scalar");
    testing.assertEqual(len($r.series), 1);
    testing.assertEqual($r.series[0].values[0].value, 7.0);
}

func parseErrorResponse() {
    def body as string init '{"status":"error","error":"bad_data: parse error"}';
    parseResult(json.decode($body));
}

func testParseErrorThrows() {
    testing.assertThrows("parseErrorResponse", "prometheus");
}
