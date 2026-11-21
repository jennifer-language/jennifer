# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# graphql_test.j - white-box tests for graphql.j's pure helpers. Run with:
#
#     jennifer test modules/graphql_test.j
#
# The overlay splices graphql.j in first, so these tests reach its private
# helpers (buildRequest, hasErrors, errorMessages, isQuery, ...) by bare
# identifier. The
# live request path (query -> HTTP -> decode -> data / raise) needs a server and
# is covered in the Go suite (TestGraphql). graphql.j already `use`s json /
# convert / strings, so the overlay only adds testing.
use encoding;
use testing;
use net;
use task;
use convert;

func testBuildRequest() {
    def vars as json.Value init json.set(json.map(), "/id", 5);
    def req as json.Value init buildRequest('{ x }', $vars, "");
    testing.assertEqual(json.encode($req), '{"query":"{ x }","variables":{"id":5}}');
}

func testBuildRequestEmptyVariables() {
    def req as json.Value init buildRequest('query { me { id } }', json.map(), "");
    testing.assertEqual(json.encode($req), '{"query":"query { me { id } }","variables":{}}');
}

func testBuildRequestOmitsEmptyOperationName() {
    # An empty operationName is left out of the body entirely.
    def req as json.Value init buildRequest('{ x }', json.map(), "");
    testing.assertFalse(json.has($req, "/operationName"));
}

func testBuildRequestWithOperationName() {
    def req as json.Value init buildRequest('query A { x } query B { y }', json.map(), "B");
    testing.assertEqual(json.asString($req, "/operationName"), "B");
}

func testHasGraphqlErrorsTrue() {
    def resp as json.Value init json.decode('{"errors":[{"message":"boom"}]}');
    testing.assertTrue(hasErrors($resp));
}

func testHasGraphqlErrorsFalseOnData() {
    def resp as json.Value init json.decode('{"data":{"x":1}}');
    testing.assertFalse(hasErrors($resp));
}

func testHasGraphqlErrorsFalseOnEmptyArray() {
    # A well-formed response never carries an empty errors array, but a buggy
    # server might; an empty array is not an error signal.
    def resp as json.Value init json.decode('{"errors":[]}');
    testing.assertFalse(hasErrors($resp));
}

func testHasGraphqlErrorsWithPartialData() {
    # GraphQL allows data + errors together; the errors still signal a failure.
    def resp as json.Value init json.decode('{"data":{"x":null},"errors":[{"message":"x failed"}]}');
    testing.assertTrue(hasErrors($resp));
}

func testErrorMessages() {
    def resp as json.Value init json.decode('{"errors":[{"message":"a"},{"message":"b"}]}');
    testing.assertEqual(errorMessages($resp), "a; b");
}

func testErrorMessagesSingle() {
    def resp as json.Value init json.decode('{"errors":[{"message":"only one"}]}');
    testing.assertEqual(errorMessages($resp), "only one");
}

func testErrorMessagesMissingMessage() {
    # An error entry without a `message` field falls back to a placeholder.
    def resp as json.Value init json.decode('{"errors":[{"code":42}]}');
    testing.assertEqual(errorMessages($resp), "(no message)");
}

# An errors entry without a message still counts, with a placeholder.
func testErrorMessagesWithoutMessage() {
    def resp as json.Value init json.decode('{"errors":[{"message":"a"},{"path":["x"]}]}');
    testing.assertEqual(errorMessages($resp), "a; (no message)");
}

# --- which documents may be retried ---

func testIsQuery() {
    testing.assertTrue(isQuery('{ me { id } }', ""));
    testing.assertTrue(isQuery('query { me { id } }', ""));
    testing.assertTrue(isQuery('query Me($a: Int) { me { id } }', ""));
    testing.assertTrue(isQuery("  # list them\n , query\{ x \}", ""));
    testing.assertFalse(isQuery('mutation { stop }', ""));
    testing.assertFalse(isQuery('subscription { tick }', ""));
    testing.assertFalse(isQuery('queryX { x }', "")); # not the keyword
    testing.assertFalse(isQuery('fragment F on T { x } query { ...F }', ""));
    testing.assertFalse(isQuery('', ""));
    # a named operation in a document with several
    def doc as string init 'query Read { x } mutation Write { y }';
    testing.assertTrue(isQuery($doc, "Read"));
    testing.assertFalse(isQuery($doc, "Write"));
    testing.assertFalse(isQuery($doc, "Rea"));
    testing.assertFalse(isQuery($doc, "a|b")); # not a name: never a pattern
    # text that only looks like a definition: in a string, a block string, a comment
    testing.assertFalse(isQuery('mutation Write { note(text: "query Write") }', "Write"));
    testing.assertFalse(isQuery('mutation Write { note(text: """ query Write """) }', "Write"));
    testing.assertFalse(isQuery("# query Write was the old name\nmutation Write \{ x \}", "Write"));
    testing.assertTrue(isQuery('query Read { a(q: "mutation Read # \\"x") }', "Read"));
    testing.assertFalse(isQuery('query Twice { a } query Twice { b }', "Twice"));
}

# --- error codes, and what goes into a raised message ---

func testErrorCodes() {
    def resp as json.Value init json.decode('{"errors":[{"message":"a","extensions":{"code":"FORBIDDEN"}},{"message":"b"}]}');
    testing.assertEqual(errorCodes($resp), ["FORBIDDEN", ""]);
    testing.assertEqual(describeErrors($resp), "a [FORBIDDEN]; b");
    testing.assertEqual(errorCodes(json.decode('{"data":{}}')), []);
}

func testErrorTextIsFlattened() {
    def resp as json.Value init json.decode('{"errors":[{"message":"line one\nforged: line"}]}');
    testing.assertEqual(describeErrors($resp), "line one forged: line");
    testing.assertEqual(
        snippet("<html>\r\n<body>bad gateway</body>"),
        "<html> <body>bad gateway</body>");
    def long as string init strings.replicate("x", 500);
    testing.assertEqual(len(snippet($long)), MAX_SNIPPET + 3);
}

func testEnvelopeOf() {
    testing.assertTrue(envelopeOf('{"errors":[{"message":"bad query"}]}'));
    testing.assertFalse(envelopeOf('{"data":{}}'));
    testing.assertFalse(envelopeOf('<html>502</html>'));
    testing.assertFalse(envelopeOf('[1, 2]'));
}

func testHasHeader() {
    testing.assertTrue(hasHeader({"ACCEPT": "x"}, "accept"));
    testing.assertFalse(hasHeader({"Content-Type": "x"}, "accept"));
}

# the builders reach the wrapped rest client's options
func testPolicyBuilders() {
    def c as Client init withBackoff(
        withRetries(withTimeout(client("https://h/graphql"), 5000), 3),
        100);
    testing.assertEqual($c.rest.options.timeoutMs, 5000);
    testing.assertEqual($c.rest.options.maxRetries, 3);
    testing.assertEqual($c.rest.options.backoffMs, 100);
}

# serveCanned answers one request on `listener` with a 200 carrying `body`.
func serveCanned(listener as net.Listener, body as string) {
    def c as net.Conn init net.accept($listener);
    net.readBytes($c, 65536);
    net.writeBytes(
        $c,
        encoding.encode(
            "utf-8",
            "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: " +
                convert.toString(len($body)) + "\r\nConnection: close\r\n\r\n" + $body));
    net.close($c);
}

# A 2xx reply past json's nesting cap is kind "limit", not a GraphqlError.
func testADecodingCapIsALimit() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        serveCanned($listener, strings.replicate("[", 1001));
    };
    try {
        query(client("http://" + net.address($listener) + "/graphql"), '{ a }', json.map());
        throw Error{kind: "missed", message: "a decoding cap"};
    } catch (e) {
        testing.assertEqual($e.kind, "limit");
    }
    task.wait($server);
    net.close($listener);
}

# --- the response must be a GraphQL response ---

func testShapeProblem() {
    for (def ok in [
        '{"data":{"a":1}}',
        '{"data":null}',
        '{"data":null,"errors":[{"message":"x","path":["a"],"locations":[{"line":1,"column":2}]}]}',
        '{"errors":[{"message":"x","extensions":{"code":"C"}}]}'
    ]) {
        testing.assertEqual(shapeProblem(json.decode($ok)), "");
    }
    for (def bad in [
        '{}',
        '[1,2]',
        '"hello"',
        'null',
        '{"errors":"boom"}',
        '{"errors":{"message":"boom"}}',
        '{"errors":[]}',
        '{"data":{},"errors":[]}',
        '{"errors":[1]}',
        '{"errors":[{"code":42}]}',
        '{"errors":[{"message":7}]}',
        '{"errors":[{"message":"x","path":"a"}]}',
        '{"errors":[{"message":"x","extensions":[]}]}',
        '{"data":[1]}',
        '{"data":"x"}',
        '{"data":{},"extensions":1}'
    ]) {
        testing.assertNotEqual(shapeProblem(json.decode($bad)), "");
    }
    # a 4xx is only read as an envelope when it is a well-formed one
    testing.assertFalse(envelopeOf('{"errors":[{"code":42}]}'));
    testing.assertFalse(envelopeOf('{"errors":"boom"}'));
}

# askCanned runs a query against a one-shot server answering 200 + body and
# returns the GraphqlError's reason ("" when the query succeeded).
func askCanned(body as string, raise as bool) {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        serveCanned($listener, $body);
    };
    def reason as string init "";
    try {
        def c as Client init client("http://" + net.address($listener) + "/graphql");
        if ($raise) {
            query($c, '{ a }', json.map());
        } else {
            tryQuery($c, '{ a }', json.map());
        }
    } catch (e) {
        testing.assertEqual($e.kind, "graphql");
        def ge as GraphqlError init $e;
        $reason = $ge.reason;
    }
    task.wait($server);
    net.close($listener);
    return $reason;
}

# A 200 that is not a GraphQL response is an error, for query and tryQuery.
func testMalformedResponseIsAnError() {
    for (def body in ['{"errors":"boom"}', '{}', '[1,2]', 'null', '{"errors":[]}']) {
        testing.assertEqual(askCanned($body, true), "json");
        testing.assertEqual(askCanned($body, false), "json");
    }
    testing.assertEqual(askCanned('{"data":{"a":1}}', true), "");
    testing.assertEqual(askCanned('{"errors":[{"message":"no"}]}', true), "graphql");
    testing.assertEqual(askCanned('{"errors":[{"message":"no"}]}', false), "");
}

# --- credentials over plain http ---

# An Authorization header to a non-loopback http:// endpoint is refused before
# any connection (192.0.2.1 is TEST-NET: a connect would hang to the timeout).
func testCredentialsNeedHttps() {
    def plain as Client init withTimeout(client("http://192.0.2.1/graphql"), 2000);
    for (def c in [
        bearer($plain, "t"),
        basic($plain, "u", "p"),
        header($plain, "authorization", "x")
    ]) {
        try {
            query($c, '{ a }', json.map());
            throw Error{kind: "missed", message: "a cleartext refusal"};
        } catch (e) {
            testing.assertEqual($e.kind, "graphql");
            def ge as GraphqlError init $e;
            testing.assertEqual($ge.reason, "insecure");
            testing.assertEqual($ge.status, 0);
        }
    }
}

# Loopback http, and allowInsecureHttp, pass the check.
func testCredentialsOverLoopbackOrOptIn() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        serveCanned($listener, '{"data":{"a":1}}');
    };
    def c as Client init bearer(client("http://" + net.address($listener) + "/graphql"), "t");
    def r as json.Value init query($c, '{ a }', json.map());
    testing.assertEqual(json.asInt($r, "/data/a"), 1);
    task.wait($server);
    net.close($listener);
    def lan as Client init bearer(client("http://host.invalid/graphql"), "t");
    $lan.allowInsecureHttp = true;
    try {
        query($lan, '{ a }', json.map());
    } catch (e) {
        # the name does not resolve; what matters is that it was tried
        if ($e.kind == "graphql") {
            def ge as GraphqlError init $e;
            testing.assertNotEqual($ge.reason, "insecure");
        }
    }
}
