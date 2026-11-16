# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
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
use task;

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
    testing.assertTrue(dotTerminated(".\r\n")); # empty body
    testing.assertTrue(dotTerminated("body\r\n.\r\n"));
    testing.assertFalse(dotTerminated("body\r\nmore")); # not yet complete
}

func testParseSizes() {
    def szs as list of int init parseSizes("1 20\r\n2 14\r\n10 300");
    testing.assertEqual(len($szs), 3);
    testing.assertEqual($szs[0], 20);
    testing.assertEqual($szs[1], 14);
    testing.assertEqual($szs[2], 300);
}

func testParseUidl() {
    def ids as list of MessageId init parseUidl("1 whqtswO00WBw418f9t5JxYwZ\r\n2 QhdPYR:00WBw1Ph7x7");
    testing.assertEqual(len($ids), 2);
    testing.assertEqual($ids[0].number, 1);
    testing.assertEqual($ids[0].id, "whqtswO00WBw418f9t5JxYwZ");
    testing.assertEqual($ids[1].number, 2);
    testing.assertEqual($ids[1].id, "QhdPYR:00WBw1Ph7x7");
}

func testApopTimestamp() {
    testing.assertEqual(
        apopTimestamp("+OK POP3 ready <1896.697170952@dbc.mtview.ca.us>"),
        "<1896.697170952@dbc.mtview.ca.us>");
    testing.assertEqual(apopTimestamp("+OK no timestamp here"), "");
}

func testApopDigest() {
    # RFC 1939 worked example: <1896.697170952@dbc.mtview.ca.us> + "tanstaaf".
    testing.assertEqual(
        apopDigest("<1896.697170952@dbc.mtview.ca.us>", "tanstaaf"),
        "c4c9334bac560ecc979e58001b3e22fb");
}

# ---- read cap (DoS from an unterminated / oversized response) ----
func testResponseCapRejectsOversized() {
    testing.assertThrows("overRespCap", "limit");
}
func overRespCap() {
    capResponse(MAX_RESPONSE_BYTES + 1);
}
func testResponseCapAllowsAtLimit() {
    capResponse(MAX_RESPONSE_BYTES);
    testing.assertTrue(true);
}

func injectPop() {
    rejectControl("USER a\r\nPASS x", "USER");
}
func testPopInjectionBlocked() { # OM-006
    testing.assertThrows("injectPop", "pop");
}

# An -ERR reply is a PopError with the command, the response code and the
# text; another failure has none, and every failure is kind "pop3".
func testErrRepliesArePopErrors() {
    def e as PopError init replyError("-ERR [IN-USE] mailbox locked by another session", "PASS");
    testing.assertEqual($e.kind, "pop3");
    testing.assertEqual($e.command, "PASS");
    testing.assertEqual($e.responseCode, "IN-USE");
    testing.assertEqual($e.text, "mailbox locked by another session");
    testing.assertEqual($e.message, "PASS: -ERR [IN-USE] mailbox locked by another session");
    $e = replyError("-ERR [SYS/TEMP] try later", "RETR");
    testing.assertEqual($e.responseCode, "SYS/TEMP");
    $e = replyError("-ERR no such message", "RETR");
    testing.assertEqual($e.responseCode, "");
    testing.assertEqual($e.text, "no such message");
    testing.assertThrows("injectedUser", "pop");
    testing.assertThrows("hugeResponse", "limit");
}

func injectedUser() {
    rejectControl("me\r\nDELE 1", "user");
}

func hugeResponse() {
    capResponse(MAX_RESPONSE_BYTES + 1);
}

# A server that closes mid-response is kind "closed", not a PopError.
func testAPeerCloseIsClosed() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        def conn as net.Conn init net.accept($listener);
        net.writeBytes($conn, convert.bytesFromString("utf-8", "+OK 1 octets\r\npartial"));
        net.close($conn);
    };
    def c as net.Conn init net.connect(net.address($listener));
    try {
        readMultiline($c, "RETR");
        throw Error{kind: "missed", message: "a close mid-response"};
    } catch (e) {
        testing.assertEqual($e.kind, "closed");
    }
    task.wait($server);
    net.close($c);
    net.close($listener);
}

# CAPA: a "-ERR" answer is no SASL mechanism; a server closing mid-answer is
# kind "closed", not taken for a server without CAPA.
func testCapaErrIsNoMechanisms() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        def conn as net.Conn init net.accept($listener);
        net.readBytes($conn, 512);
        net.writeBytes($conn, convert.bytesFromString("utf-8", "-ERR no CAPA\r\n"));
        net.readBytes($conn, 512);
        net.writeBytes($conn, convert.bytesFromString("utf-8", "+OK\r\nSASL SCRAM-SHA-256\r\n"));
        net.close($conn);
    };
    def c as net.Conn init net.connect(net.address($listener));
    testing.assertEqual(len(popCapaMechs($c)), 0);
    try {
        popCapaMechs($c);
        throw Error{kind: "missed", message: "a close mid-CAPA"};
    } catch (e) {
        testing.assertEqual($e.kind, "closed");
    }
    task.wait($server);
    net.close($c);
    net.close($listener);
}

# scramFirstFor answers a base64 client-first with a base64 server-first whose
# nonce extends the client's (salt "salt", 4096 iterations).
func scramFirstFor(clientFirst as string) {
    def cf as string init convert.stringFromBytes(
        "utf-8",
        encoding.fromText("base64", strings.trim($clientFirst)));
    def nonce as string init $cf[strings.indexOf($cf, ",r=") + 3..];
    return encoding.toText(
        "base64",
        convert.bytesFromString("utf-8", "r=" + $nonce + "srv,s=c2FsdA==,i=4096"));
}

# A "+OK" straight after the client-final skips the server-final: the server
# never proved it knows the password, so the session is refused.
func testScramWithoutServerFinalIsRefused() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        def conn as net.Conn init net.accept($listener);
        def line as string init convert.stringFromBytes("utf-8", net.readBytes($conn, 512));
        def parts as list of string init strings.split(strings.trim($line), " ");
        net.writeBytes(
            $conn,
            convert.bytesFromString("utf-8", "+ " + scramFirstFor($parts[2]) + "\r\n"));
        net.readBytes($conn, 512);
        net.writeBytes($conn, convert.bytesFromString("utf-8", "+OK logged in\r\n"));
        net.readBytes($conn, 512);
        net.close($conn);
    };
    def c as net.Conn init net.connect(net.address($listener));
    def opts as Options init Options{
        host: "",
        port: 0,
        security: transport.Security.None,
        user: "u",
        pass: "p",
        auth: "scram-sha-256"
    };
    try {
        scramAuth($c, $opts, "scram-sha-256");
        throw Error{kind: "missed", message: "a SCRAM success without server-final"};
    } catch (e) {
        testing.assertEqual($e.kind, "pop3");
    }
    net.close($c);
    task.wait($server);
    net.close($listener);
}
