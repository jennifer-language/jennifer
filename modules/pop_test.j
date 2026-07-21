# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# pop_test.j - white-box tests for pop.j's pure protocol helpers. Run with:
#
#     jennifer test modules/pop_test.j
#
# The overlay splices pop.j in front of this file, so the tests reach its
# private helpers (statusOK, parseStat, parseDotBody, parseSizes,
# dotTerminated) by bare identifier. The networked session is not covered here
# (it needs a live server); it is verified end to end against an in-process
# POP3 server in the Go suite (TestPop3Receive).
use testing;

func testStatusOK() {
    testing.assertTrue(statusOK("+OK 2 34"));
    testing.assertFalse(statusOK("-ERR no such message"));
}

func testParseStat() {
    def st as Stat init parseStat("+OK 3 320");
    testing.assertEqual($st.count, 3);
    testing.assertEqual($st.size, 320);
}

func testParseDotBodyBasic() {
    testing.assertEqual(parseDotBody("line1\r\nline2\r\n.\r\n"), "line1\r\nline2");
}

func testParseDotBodyEmpty() {
    testing.assertEqual(parseDotBody(".\r\n"), "");
}

func testParseDotBodyUnstuffs() {
    # A body line sent as "..dotted" un-stuffs to ".dotted".
    testing.assertEqual(parseDotBody("..dotted\r\n.\r\n"), ".dotted");
}

func testDotTerminated() {
    testing.assertTrue(dotTerminated(".\r\n"));           # empty body
    testing.assertTrue(dotTerminated("body\r\n.\r\n"));
    testing.assertFalse(dotTerminated("body\r\nmore"));   # not yet complete
}

func testParseSizes() {
    def szs as list of int init parseSizes("1 20\r\n2 14\r\n10 300");
    testing.assertEqual(len($szs), 3);
    testing.assertEqual($szs[0], 20);
    testing.assertEqual($szs[1], 14);
    testing.assertEqual($szs[2], 300);
}


func testApopTimestamp() {
    testing.assertEqual(apopTimestamp("+OK POP3 ready <1896.697170952@dbc.mtview.ca.us>"),
        "<1896.697170952@dbc.mtview.ca.us>");
    testing.assertEqual(apopTimestamp("+OK no timestamp here"), "");
}

func testApopDigest() {
    # RFC 1939 worked example: <1896.697170952@dbc.mtview.ca.us> + "tanstaaf".
    testing.assertEqual(apopDigest("<1896.697170952@dbc.mtview.ca.us>", "tanstaaf"),
        "c4c9334bac560ecc979e58001b3e22fb");
}

# ---- read cap (DoS from an unterminated / oversized response) ----
func testResponseCapRejectsOversized() {
    testing.assertThrows("overRespCap", "pop");
}
func overRespCap() { capResponse(MAX_RESPONSE_BYTES + 1); }
func testResponseCapAllowsAtLimit() {
    capResponse(MAX_RESPONSE_BYTES);
    testing.assertTrue(true);
}
