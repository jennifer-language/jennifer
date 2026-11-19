# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# jsonrpc_test.j - white-box tests for jsonrpc.j. Run with:
#
#     jennifer test modules/jsonrpc_test.j
#
# The overlay splices jsonrpc.j in first, so these tests reach its private
# request builder / reply parser / error encoder by bare identifier. The server
# `handle` dispatches by name via meta.callMain to the functions below, which the
# overlay is the entry program for; the live HTTP client round-trip is verified
# against an in-process server in the Go suite (TestJsonrpcClient).
use testing;
use json;
use net;
use task;
use convert;
use time;

# --- handler functions `handle` dispatches to (named in the requests) --------

func add(params as json.Value) {
    return json.asInt($params, "/0") + json.asInt($params, "/1");
}

func greet(params as json.Value) {
    return "hi " + json.asString($params, "/name");
}

func boom(params as json.Value) {
    throw Error{kind: "demo", message: "kaboom"};
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
        '{"jsonrpc":"2.0","method":"add","params":[2,3],"id":1}');
}

func testBuildNotification() {
    # a notification omits the id member
    testing.assertEqual(
        buildNotification("ping", json.list()),
        '{"jsonrpc":"2.0","method":"ping","params":[]}');
}

# --- reply parsing -----------------------------------------------------------

func testParseResultOk() {
    def r as json.Value init parseResult('{"jsonrpc":"2.0","result":42,"id":1}', 200, 1);
    testing.assertEqual(json.asInt($r, ""), 42);
}

func testParseResultError() {
    testing.assertThrows("parseError", "jsonrpc");
}

func parseError() {
    parseResult('{"jsonrpc":"2.0","error":{"code":-32601,"message":"nope"},"id":1}', 200, 1);
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
    parseResult('{"jsonrpc":"2.0","id":1}', 200, 1);
}

func testParseResultIdMismatch() {
    # a reply whose id does not echo the request's is rejected
    testing.assertThrows("parseIdMismatch", "jsonrpc");
}

func parseIdMismatch() {
    parseResult('{"jsonrpc":"2.0","result":42,"id":9}', 200, 1);
}

# --- server dispatch ---------------------------------------------------------

func testHandleCall() {
    testing.assertEqual(
        handle('{"jsonrpc":"2.0","method":"add","params":[2,3],"id":1}'),
        '{"jsonrpc":"2.0","result":5,"id":1}');
}

func testHandleNamedAndStringId() {
    testing.assertEqual(
        handle('{"jsonrpc":"2.0","method":"greet","params":{"name":"x"},"id":"a"}'),
        '{"jsonrpc":"2.0","result":"hi x","id":"a"}');
}

func testHandleMethodNotFound() {
    testing.assertEqual(
        handle('{"jsonrpc":"2.0","method":"nope","id":7}'),
        '{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not found"},"id":7}');
}

func testHandleThrowIsInternalError() {
    # the thrown handler message ("kaboom") stays server-side; the wire reply
    # carries only the generic internal-error text
    testing.assertEqual(
        handle('{"jsonrpc":"2.0","method":"boom","id":3}'),
        '{"jsonrpc":"2.0","error":{"code":-32603,"message":"Internal error"},"id":3}');
}

func testHandleNotificationNoReply() {
    testing.assertEqual(handle('{"jsonrpc":"2.0","method":"add","params":[1,1]}'), "");
}

func testHandleParseError() {
    testing.assertEqual(
        handle('{bad'),
        '{"jsonrpc":"2.0","error":{"code":-32700,"message":"Parse error"},"id":null}');
}

func testHandleInvalidRequest() {
    # a request with no method member
    testing.assertEqual(
        handle('{"jsonrpc":"2.0","id":1}'),
        '{"jsonrpc":"2.0","error":{"code":-32600,"message":"Invalid Request"},"id":1}');
}

func testHandleBatch() {
    # two calls and a notification -> two replies, notification omitted
    testing.assertEqual(
        handle('[{"jsonrpc":"2.0","method":"add","params":[1,2],"id":1},{"jsonrpc":"2.0","method":"add","params":[3,4]},{"jsonrpc":"2.0","method":"add","params":[5,6],"id":2}]'),
        '[{"jsonrpc":"2.0","result":3,"id":1},{"jsonrpc":"2.0","result":11,"id":2}]');
}

func testHandleAllNotificationBatchNoReply() {
    testing.assertEqual(
        handle('[{"jsonrpc":"2.0","method":"add","params":[1,1]},{"jsonrpc":"2.0","method":"add","params":[2,2]}]'),
        "");
}

func testHandleEmptyBatch() {
    testing.assertEqual(
        handle("[]"),
        '{"jsonrpc":"2.0","error":{"code":-32600,"message":"Invalid Request"},"id":null}');
}

func testConstants() {
    testing.assertEqual(VERSION, "2.0");
    testing.assertEqual(METHOD_NOT_FOUND, -32601);
    testing.assertEqual(PARSE_ERROR, -32700);
}

# A reply with a result but no id cannot be matched to the request.
func testParseResultMissingId() {
    testing.assertThrows("parseMissingId", "jsonrpc");
}

func parseMissingId() {
    parseResult('{"jsonrpc":"2.0","result":42}', 200, 1);
}

# A batch over MAX_BATCH requests is refused whole, before any handler runs.
func testBatchCapped() {
    def parts as list of string init [];
    for (def i in 0..1001) {
        $parts[] = '{"jsonrpc":"2.0","method":"nope","id":1}';
    }
    def reply as string init handle("[" + strings.join($parts, ",") + "]");
    testing.assertContains($reply, "batch too large");
    testing.assertContains($reply, "-32600");
}

# A malformed error object in a reply still surfaces as a jsonrpc error.
func malformedErrorReply() {
    return parseResult('{"jsonrpc":"2.0","error":{"code":"x"},"id":1}', 200, 1);
}
func testMalformedErrorReply() {
    testing.assertThrows("malformedErrorReply", "jsonrpc");
}

# Every client failure is an RpcError: the reason, the HTTP status, and for an
# error reply its code and data.
func testFailuresAreRpcErrors() {
    try {
        parseResult(
            '{"jsonrpc":"2.0","error":{"code":-32602,"message":"bad","data":{"field":"x"}},"id":1}',
            200,
            1);
        throw Error{kind: "missed", message: "an error reply"};
    } catch (e) {
        testing.assertEqual($e.kind, "jsonrpc");
        testing.assertEqual($e.reason, "rpc");
        testing.assertEqual($e.code, -32602);
        testing.assertEqual($e.status, 200);
        testing.assertEqual(json.asString($e.data, "/field"), "x");
        testing.assertEqual($e.message, "jsonrpc error -32602: bad");
    }
    try {
        parseResult('{"jsonrpc":"2.0","error":{"code":-32601,"message":"nope"},"id":1}', 200, 1);
        throw Error{kind: "missed", message: "an error reply without data"};
    } catch (e) {
        testing.assertEqual($e.code, -32601);
        testing.assertTrue(json.isNull($e.data, ""));
    }
    try {
        parseResult("<html>502</html>", 502, 1);
        throw Error{kind: "missed", message: "a non-JSON reply"};
    } catch (e) {
        testing.assertEqual($e.reason, "protocol");
        testing.assertEqual($e.status, 502);
        testing.assertEqual($e.code, 0);
    }
    try {
        parseResult(strings.repeat("[", 1001), 200, 1);
        throw Error{kind: "missed", message: "a decoding cap"};
    } catch (e) {
        testing.assertEqual($e.kind, "limit");
    }
}

# A refused connection is http's net.NetError (kind "net"), passed on as it
# is, not an RpcError.
func testARefusedConnectionIsANetError() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def addr as string init net.address($listener);
    net.close($listener);
    try {
        httpPost(client("http://" + $addr + "/rpc"), '{}');
        throw Error{kind: "missed", message: "a refused connection"};
    } catch (e) {
        testing.assertEqual($e.kind, "net");
        testing.assertEqual($e.reason, "refused");
    }
}

# Accept one request and answer it as told: "short" promises more body than
# it sends and closes, "silent" says nothing until the client is gone.
func misbehave(l as net.Listener, how as string) {
    def conn as net.Conn init net.accept($l);
    net.readBytes($conn, 65536);
    if ($how == "short") {
        net.writeBytes(
            $conn,
            convert.bytesFromString(
                "utf-8",
                "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 100\r\n\r\n\{"));
    } else {
        time.sleep(time.fromMilliseconds(1500));
    }
    net.close($conn);
    return 0;
}

# A reply cut short is kind "closed" and a cancelled call kind "cancelled",
# rethrown as http threw them, not an RpcError with reason "transport". (A
# deadline, "timeout", takes the same path; http.post waits 30 s for one.)
func testGenericKindsPassOn() {
    def l as net.Listener init net.listen("127.0.0.1:0");
    def addr as string init net.address($l);
    def t as task of int init spawn {
        return misbehave($l, "short");
    };
    try {
        call(client("http://" + $addr + "/rpc"), "add", json.list());
        throw Error{kind: "missed", message: "a reply cut short"};
    } catch (e) {
        testing.assertEqual($e.kind, "closed");
    }
    task.wait($t);
    $t = spawn {
        return misbehave($l, "silent");
    };
    def c as task of json.Value init spawn {
        return call(client("http://" + $addr + "/rpc"), "add", json.list());
    };
    time.sleep(time.fromMilliseconds(200));
    task.cancel($c);
    try {
        task.wait($c);
        throw Error{kind: "missed", message: "a cancelled call"};
    } catch (e) {
        testing.assertEqual($e.kind, "cancelled");
    }
    task.wait($t);
    net.close($l);
}

# --- strict reply parsing ----------------------------------------------------

# reasonOf parses a reply and returns the RpcError's reason ("" if none).
func reasonOf(body as string) {
    try {
        parseResult($body, 200, 1);
    } catch (e) {
        testing.assertEqual($e.kind, "jsonrpc");
        def re as RpcError init $e;
        return $re.reason;
    }
    return "";
}

# A reply that is not a JSON-RPC 2.0 response to this request is a protocol
# error, not a result and not an error reply.
func testParseResultIsStrict() {
    for (def bad in [
        '{"result":5,"id":1}',
        '{"jsonrpc":"1.0","result":5,"id":1}',
        '{"jsonrpc":2,"result":5,"id":1}',
        '{"jsonrpc":"2.0","error":"boom","id":1}',
        '{"jsonrpc":"2.0","error":{"code":-1},"id":1}',
        '{"jsonrpc":"2.0","error":{"code":-1.5,"message":"x"},"id":1}',
        '{"jsonrpc":"2.0","error":{"code":-1,"message":"x"},"id":99}',
        '{"jsonrpc":"2.0","error":{"code":-1,"message":"x"}}',
        '{"jsonrpc":"2.0","result":5,"error":{"code":1,"message":"x"},"id":1}',
        '[{"jsonrpc":"2.0","result":5,"id":1}]',
        '5',
        '{"jsonrpc":"2.0","result":5,"id":"1"}'
    ]) {
        testing.assertEqual(reasonOf($bad), "protocol");
    }
    testing.assertEqual(reasonOf('{"jsonrpc":"2.0","result":5,"id":1}'), "");
    testing.assertEqual(reasonOf('{"jsonrpc":"2.0","result":null,"id":1}'), "");
    # an error the server could not tie to the request carries a null id
    testing.assertEqual(
        reasonOf('{"jsonrpc":"2.0","error":{"code":-32700,"message":"Parse error"},"id":null}'),
        "rpc");
    testing.assertEqual(
        reasonOf('{"jsonrpc":"2.0","error":{"code":-1,"message":"x"},"id":1}'),
        "rpc");
}

# --- notify reads the status -------------------------------------------------

# answer accepts one request on `l` and replies with status, type and body.
func answer(l as net.Listener, status as int, ctype as string, body as string) {
    def conn as net.Conn init net.accept($l);
    net.readBytes($conn, 65536);
    net.writeBytes(
        $conn,
        convert.bytesFromString(
            "utf-8",
            "HTTP/1.1 " + convert.toString($status) + " X\r\nContent-Type: " + $ctype +
                "\r\nContent-Length: " + convert.toString(len($body)) +
                "\r\nConnection: close\r\n\r\n" + $body));
    net.close($conn);
    return 0;
}

# notifyReason sends a notification to a server answering status / body and
# returns the RpcError's reason ("" when notify returned).
func notifyReason(status as int, ctype as string, body as string) {
    def l as net.Listener init net.listen("127.0.0.1:0");
    def t as task of int init spawn {
        return answer($l, $status, $ctype, $body);
    };
    def reason as string init "";
    try {
        notify(client("http://" + net.address($l) + "/rpc"), "ping", json.list());
    } catch (e) {
        testing.assertEqual($e.kind, "jsonrpc");
        def re as RpcError init $e;
        testing.assertEqual($re.status, $status);
        $reason = $re.reason;
    }
    task.wait($t);
    net.close($l);
    return $reason;
}

func testNotifyChecksTheStatus() {
    testing.assertEqual(notifyReason(204, "text/plain", ""), "");
    testing.assertEqual(notifyReason(200, "application/json", ""), "");
    testing.assertEqual(notifyReason(404, "text/html", "<h1>err</h1>"), "http");
    testing.assertEqual(notifyReason(500, "text/html", "<h1>err</h1>"), "http");
    testing.assertEqual(notifyReason(401, "text/html", "<h1>err</h1>"), "http");
    testing.assertEqual(
        notifyReason(
            500,
            "application/json",
            '{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not found"},"id":null}'),
        "rpc");
}

# --- the server refuses requests that are not JSON-RPC 2.0 -------------------

func testHandleRefusesInvalidRequests() {
    def invalid as string init '{"jsonrpc":"2.0","error":{"code":-32600,"message":"Invalid Request"},"id":';
    testing.assertEqual(handle('{"method":"add","params":[1,2],"id":1}'), $invalid + '1}');
    testing.assertEqual(
        handle('{"jsonrpc":"1.0","method":"add","params":[1,2],"id":1}'),
        $invalid + '1}');
    testing.assertEqual(
        handle('{"jsonrpc":"2.0","method":"add","params":5,"id":1}'),
        $invalid + '1}');
    testing.assertEqual(
        handle('{"jsonrpc":"2.0","method":"add","params":"x","id":1}'),
        $invalid + '1}');
    testing.assertEqual(
        handle('{"jsonrpc":"2.0","method":"add","params":[1,2],"id":{"a":1}}'),
        $invalid + 'null}');
    testing.assertEqual(
        handle('{"jsonrpc":"2.0","method":"add","params":[1,2],"id":[1]}'),
        $invalid + 'null}');
    testing.assertEqual(
        handle('{"jsonrpc":"2.0","method":"add","params":[1,2],"id":true}'),
        $invalid + 'null}');
    # an invalid notification is still answered: it is not a valid request
    testing.assertEqual(
        handle('{"jsonrpc":"1.0","method":"add","params":[1,2]}'),
        $invalid + 'null}');
    # what stays valid: no params, a null or string id
    testing.assertEqual(
        handle('{"jsonrpc":"2.0","method":"nope","id":null}'),
        '{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not found"},"id":null}');
    testing.assertEqual(
        handle('{"jsonrpc":"2.0","method":"greet","params":{"name":"y"},"id":"k"}'),
        '{"jsonrpc":"2.0","result":"hi y","id":"k"}');
}

# --- credentials over plain http ---------------------------------------------

# An Authorization header to a non-loopback http:// endpoint is refused before
# any connection (192.0.2.1 is TEST-NET, nothing answers there).
func testCredentialsNeedHttps() {
    def c as Client init clientWith("http://192.0.2.1/rpc", {"authorization": "Bearer t"});
    for (def how in ["call", "notify"]) {
        try {
            if ($how == "call") {
                call($c, "add", json.list());
            } else {
                notify($c, "add", json.list());
            }
            throw Error{kind: "missed", message: "a cleartext refusal"};
        } catch (e) {
            testing.assertEqual($e.kind, "jsonrpc");
            def re as RpcError init $e;
            testing.assertEqual($re.reason, "insecure");
            testing.assertEqual($re.status, 0);
        }
    }
    testing.assertFalse(hasCredentials({"X-Trace": "1"}));
    testing.assertTrue(hasCredentials({"Proxy-Authorization": "x"}));
}

# Loopback http, and allowInsecureHttp, pass the check.
func testCredentialsOverLoopbackOrOptIn() {
    def l as net.Listener init net.listen("127.0.0.1:0");
    def t as task of int init spawn {
        return answer($l, 200, "application/json", '{"jsonrpc":"2.0","result":5,"id":1}');
    };
    def c as Client init clientWith(
        "http://" + net.address($l) + "/rpc",
        {"Authorization": "Bearer t"});
    testing.assertEqual(json.asInt(call($c, "add", posParams()), ""), 5);
    task.wait($t);
    net.close($l);
    def lan as Client init clientWith("http://host.invalid/rpc", {"Authorization": "Bearer t"});
    $lan.allowInsecureHttp = true;
    try {
        call($lan, "add", json.list());
    } catch (e) {
        # the name does not resolve; what matters is that it was tried
        testing.assertNotEqual($e.kind, "jsonrpc");
    }
}
