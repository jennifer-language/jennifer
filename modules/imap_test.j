# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# imap_test.j - white-box tests for imap.j's pure protocol helpers. Run with:
#
#     jennifer test modules/imap_test.j
#
# The overlay splices imap.j in front of this file, so the tests reach its
# private helpers (literalLength, extractLiteral, parseExists, parseSearch,
# isTagged, quoteArg, expectTaggedOK, requireAscii) by bare identifier. The
# networked session is verified end to end against an in-process IMAP server in
# the Go suite (TestImapReceive).
use testing;

func testLiteralLength() {
    testing.assertEqual(literalLength("* 1 FETCH (BODY[] {1234}"), 1234);
    testing.assertEqual(literalLength("* 2 EXISTS"), -1);
}

func testExtractLiteral() {
    def resp as string init "* 1 FETCH (BODY[] {11}\r\nHELLO WORLD)\r\nJEN OK\r\n";
    testing.assertEqual(extractLiteral($resp), "HELLO WORLD");
    testing.assertEqual(extractLiteral("JEN OK no literal\r\n"), "");
}

func testParseExists() {
    testing.assertEqual(parseExists("* 2 EXISTS\r\n* 0 RECENT\r\nJEN OK done\r\n"), 2);
    testing.assertEqual(parseExists("JEN OK nothing\r\n"), 0);
}

func testParseSearch() {
    def nums as list of int init parseSearch("* SEARCH 1 2 5\r\nJEN OK done\r\n");
    testing.assertEqual(len($nums), 3);
    testing.assertEqual($nums[0], 1);
    testing.assertEqual($nums[2], 5);
    testing.assertEqual(len(parseSearch("* SEARCH\r\nJEN OK done\r\n")), 0);
}

func testIsTagged() {
    testing.assertTrue(isTagged("JEN OK completed", "JEN"));
    testing.assertFalse(isTagged("* 1 EXISTS", "JEN"));
}

func testQuoteArg() {
    testing.assertEqual(quoteArg("simple"), "\"simple\"");
    # a quote and a backslash are escaped
    testing.assertEqual(quoteArg("a\"b\\c"), "\"a\\\"b\\\\c\"");
}

func testExpectTaggedOKThrows() {
    testing.assertThrows("taggedNo", "imap");
}
func taggedNo() {
    expectTaggedOK("JEN NO login failed", "JEN");
}

func testExpectTaggedOKPasses() {
    expectTaggedOK("JEN OK completed", "JEN");   # no throw
    testing.assertTrue(true);
}

func testRequireAsciiThrowsOnIdn() {
    testing.assertThrows("idnHost", "imap");
}
func idnHost() {
    requireAscii("münchen.de", "host");
}
