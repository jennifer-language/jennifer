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
use testing;

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
    def long as string init strings.repeat("x", 500);
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
