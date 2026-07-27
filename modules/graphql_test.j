# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# graphql_test.j - white-box tests for graphql.j's pure helpers. Run with:
#
#     jennifer test modules/graphql_test.j
#
# The overlay splices graphql.j in first, so these tests reach its private
# helpers (buildRequest, hasGraphqlErrors, errorMessages) by bare identifier. The
# live request path (query -> HTTP -> decode -> data / raise) needs a server and
# is covered in the Go suite (TestGraphql). graphql.j already `use`s json /
# convert / strings, so the overlay only adds testing.
use testing;

func testBuildRequest() {
    def vars as json.Value init json.set(json.map(), "/id", 5);
    def req as json.Value init buildRequest("{ x }", $vars, "");
    testing.assertEqual(json.encode($req), "{\"query\":\"{ x }\",\"variables\":{\"id\":5}}");
}

func testBuildRequestEmptyVariables() {
    def req as json.Value init buildRequest("query { me { id } }", json.map(), "");
    testing.assertEqual(json.encode($req), "{\"query\":\"query { me { id } }\",\"variables\":{}}");
}

func testBuildRequestOmitsEmptyOperationName() {
    # An empty operationName is left out of the body entirely.
    def req as json.Value init buildRequest("{ x }", json.map(), "");
    testing.assertFalse(json.has($req, "/operationName"));
}

func testBuildRequestWithOperationName() {
    def req as json.Value init buildRequest("query A { x } query B { y }", json.map(), "B");
    testing.assertEqual(json.asString($req, "/operationName"), "B");
}

func testHasGraphqlErrorsTrue() {
    def resp as json.Value init json.decode("{\"errors\":[{\"message\":\"boom\"}]}");
    testing.assertTrue(hasErrors($resp));
}

func testHasGraphqlErrorsFalseOnData() {
    def resp as json.Value init json.decode("{\"data\":{\"x\":1}}");
    testing.assertFalse(hasErrors($resp));
}

func testHasGraphqlErrorsFalseOnEmptyArray() {
    # A well-formed response never carries an empty errors array, but a buggy
    # server might; an empty array is not an error signal.
    def resp as json.Value init json.decode("{\"errors\":[]}");
    testing.assertFalse(hasErrors($resp));
}

func testHasGraphqlErrorsWithPartialData() {
    # GraphQL allows data + errors together; the errors still signal a failure.
    def resp as json.Value init json.decode("{\"data\":{\"x\":null},\"errors\":[{\"message\":\"x failed\"}]}");
    testing.assertTrue(hasErrors($resp));
}

func testErrorMessages() {
    def resp as json.Value init json.decode("{\"errors\":[{\"message\":\"a\"},{\"message\":\"b\"}]}");
    testing.assertEqual(errorMessages($resp), "a; b");
}

func testErrorMessagesSingle() {
    def resp as json.Value init json.decode("{\"errors\":[{\"message\":\"only one\"}]}");
    testing.assertEqual(errorMessages($resp), "only one");
}

func testErrorMessagesMissingMessage() {
    # An error entry without a `message` field falls back to a placeholder.
    def resp as json.Value init json.decode("{\"errors\":[{\"code\":42}]}");
    testing.assertEqual(errorMessages($resp), "(no message)");
}
