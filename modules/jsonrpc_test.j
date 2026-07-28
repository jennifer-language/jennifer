# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# jsonrpc_test.j - white-box tests for jsonrpc.j. Run with:
#
#     jennifer test modules/jsonrpc_test.j
#
# The overlay splices jsonrpc.j in first, so these tests reach its private
# request builder / reply parser / error encoder by bare identifier. The server
# `handle` dispatches by name via meta.callMain to the methods below, which the
# overlay is the entry program for; the live HTTP client round-trip is verified
# against an in-process server in the Go suite (TestJsonrpcClient).
use testing;
use json;

# --- handler methods `handle` dispatches to (named in the requests) ----------

func add(params as json.Value) {
    return json.asInt($params, "/0") + json.asInt($params, "/1");
}

func greet(params as json.Value) {
    return "hi " + json.asString($params, "/name");
}

func boom(params as json.Value) {
    throw Error{kind: "demo", message: "kaboom", file: "", line: 0, col: 0};
}

# --- helpers -----------------------------------------------------------------

func posParams() {
    def p as json.Value init json.list();
    $p = json.append($p, "", 2);
    $p = json.append($p, "", 3);
    return $p;
}

# --- request building --------------------------------------------------------

func testBuildRequest() {
    testing.assertEqual(
        buildRequest("add", posParams(), 1),
        "{\"jsonrpc\":\"2.0\",\"method\":\"add\",\"params\":[2,3],\"id\":1}");
}

func testBuildNotification() {
    # a notification omits the id member
    testing.assertEqual(
        buildNotification("ping", json.list()),
        "{\"jsonrpc\":\"2.0\",\"method\":\"ping\",\"params\":[]}");
}

# --- reply parsing -----------------------------------------------------------

func testParseResultOk() {
    def r as json.Value init parseResult("{\"jsonrpc\":\"2.0\",\"result\":42,\"id\":1}", 200, 1);
    testing.assertEqual(json.asInt($r, ""), 42);
}

func testParseResultError() {
    testing.assertThrows("parseError", "jsonrpc");
}

func parseError() {
    parseResult(
        "{\"jsonrpc\":\"2.0\",\"error\":{\"code\":-32601,\"message\":\"nope\"},\"id\":1}",
        200,
        1);
}

func testParseResultNonJson() {
    testing.assertThrows("parseGarbage", "jsonrpc");
}

func parseGarbage() {
    parseResult("<html>500</html>", 500, 1);
}

func testParseResultMissingResult() {
    # a reply with neither result nor error is a jsonrpc error, not a raw throw
    testing.assertThrows("parseMissingResult", "jsonrpc");
}

func parseMissingResult() {
    parseResult("{\"jsonrpc\":\"2.0\",\"id\":1}", 200, 1);
}

func testParseResultIdMismatch() {
    # a reply whose id does not echo the request's is rejected
    testing.assertThrows("parseIdMismatch", "jsonrpc");
}

func parseIdMismatch() {
    parseResult("{\"jsonrpc\":\"2.0\",\"result\":42,\"id\":9}", 200, 1);
}

# --- server dispatch ---------------------------------------------------------

func testHandleCall() {
    testing.assertEqual(
        handle("{\"jsonrpc\":\"2.0\",\"method\":\"add\",\"params\":[2,3],\"id\":1}"),
        "{\"jsonrpc\":\"2.0\",\"result\":5,\"id\":1}");
}

func testHandleNamedAndStringId() {
    testing.assertEqual(
        handle("{\"jsonrpc\":\"2.0\",\"method\":\"greet\",\"params\":{\"name\":\"x\"},\"id\":\"a\"}"),
        "{\"jsonrpc\":\"2.0\",\"result\":\"hi x\",\"id\":\"a\"}");
}

func testHandleMethodNotFound() {
    testing.assertEqual(
        handle("{\"jsonrpc\":\"2.0\",\"method\":\"nope\",\"id\":7}"),
        "{\"jsonrpc\":\"2.0\",\"error\":{\"code\":-32601,\"message\":\"Method not found\"},\"id\":7}");
}

func testHandleThrowIsInternalError() {
    # the thrown handler message ("kaboom") stays server-side; the wire reply
    # carries only the generic internal-error text
    testing.assertEqual(
        handle("{\"jsonrpc\":\"2.0\",\"method\":\"boom\",\"id\":3}"),
        "{\"jsonrpc\":\"2.0\",\"error\":{\"code\":-32603,\"message\":\"Internal error\"},\"id\":3}");
}

func testHandleNotificationNoReply() {
    testing.assertEqual(handle("{\"jsonrpc\":\"2.0\",\"method\":\"add\",\"params\":[1,1]}"), "");
}

func testHandleParseError() {
    testing.assertEqual(
        handle("{bad"),
        "{\"jsonrpc\":\"2.0\",\"error\":{\"code\":-32700,\"message\":\"Parse error\"},\"id\":null}");
}

func testHandleInvalidRequest() {
    # a request with no method member
    testing.assertEqual(
        handle("{\"jsonrpc\":\"2.0\",\"id\":1}"),
        "{\"jsonrpc\":\"2.0\",\"error\":{\"code\":-32600,\"message\":\"Invalid Request\"},\"id\":1}");
}

func testHandleBatch() {
    # two calls and a notification -> two replies, notification omitted
    testing.assertEqual(
        handle("[{\"jsonrpc\":\"2.0\",\"method\":\"add\",\"params\":[1,2],\"id\":1},{\"jsonrpc\":\"2.0\",\"method\":\"add\",\"params\":[3,4]},{\"jsonrpc\":\"2.0\",\"method\":\"add\",\"params\":[5,6],\"id\":2}]"),
        "[{\"jsonrpc\":\"2.0\",\"result\":3,\"id\":1},{\"jsonrpc\":\"2.0\",\"result\":11,\"id\":2}]");
}

func testHandleAllNotificationBatchNoReply() {
    testing.assertEqual(
        handle("[{\"jsonrpc\":\"2.0\",\"method\":\"add\",\"params\":[1,1]},{\"jsonrpc\":\"2.0\",\"method\":\"add\",\"params\":[2,2]}]"),
        "");
}

func testHandleEmptyBatch() {
    testing.assertEqual(
        handle("[]"),
        "{\"jsonrpc\":\"2.0\",\"error\":{\"code\":-32600,\"message\":\"Invalid Request\"},\"id\":null}");
}

func testConstants() {
    testing.assertEqual(VERSION, "2.0");
    testing.assertEqual(METHOD_NOT_FOUND, -32601);
    testing.assertEqual(PARSE_ERROR, -32700);
}
