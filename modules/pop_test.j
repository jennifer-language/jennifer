# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# pop_test.j - white-box tests for pop.j's pure protocol helpers. Run with:
#
#     jennifer test modules/pop_test.j
#
# The overlay splices pop.j in front of this file, so the tests reach its
# private helpers (statusOK, parseStat, parseDotBody, parseList, parseUidl,
# dotTerminated, ...) by bare identifier. The networked session is not covered here
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
        auth: "scram-sha-256",
        allowInsecureAuth: false
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

# ---- strict replies: a malformed reply is a PopError, not a default ----

func bareStat() {
    parseStat("+OK");
}
func wordStat() {
    parseStat("+OK two 300");
}
func testParseStatIsStrict() {
    testing.assertThrows("bareStat", "pop3");
    testing.assertThrows("wordStat", "pop3");
    # RFC 1939 allows text after the size
    testing.assertEqual(parseStat("+OK 2 300 octets").count, 2);
}

func testStatusOKIsExact() {
    testing.assertTrue(statusOK("+OK"));
    testing.assertFalse(statusOK("+OKAY 1 2"));
}

func uidlNoSpace() {
    parseUidl("1 abc\r\ngarbage-line-no-space\r\n3 xyz");
}
func uidlWordNumber() {
    parseUidl("one abc");
}
func uidlBlank() {
    parseUidl("1 abc\r\n\r\n2 def");
}
func testParseUidlIsStrict() {
    testing.assertThrows("uidlNoSpace", "pop3");
    testing.assertThrows("uidlWordNumber", "pop3");
    testing.assertThrows("uidlBlank", "pop3");
    testing.assertEqual(len(parseUidl("")), 0);
}

func listWord() {
    parseList("1 big");
}
func testParseListKeepsNumbers() {
    # Message 2 is marked deleted: LIST skips it, and the numbers say so.
    def l as list of MessageSize init parseList("1 100\r\n3 300 extra info");
    testing.assertEqual(len($l), 2);
    testing.assertEqual($l[1].number, 3);
    testing.assertEqual($l[1].size, 300);
    testing.assertThrows("listWord", "pop3");
    testing.assertEqual(len(parseList("")), 0);
}

func uidlOneOther() {
    parseUidlOne("+OK 3 abc", 2);
}
func uidlOneBare() {
    parseUidlOne("+OK", 2);
}
func testParseUidlOneIsStrict() {
    testing.assertEqual(parseUidlOne("+OK 2 bbb222", 2), "bbb222");
    testing.assertThrows("uidlOneOther", "pop3");
    testing.assertThrows("uidlOneBare", "pop3");
}

# The body comes back byte for byte: lines end in CRLF only, a bare LF stays.
func testParseDotBodyKeepsBareLF() {
    testing.assertEqual(parseDotBody("a\nb\r\n.\r\n"), "a\nb");
    testing.assertEqual(parseDotBody("a\n.\r\nb\r\n.\r\n"), "a\n.\r\nb");
}

func noTerminator() {
    parseDotBody("line\r\n");
}
func testParseDotBodyWithoutTerminatorIsClosed() {
    testing.assertThrows("noTerminator", "closed");
}

# serveRaw accepts one connection, reads one line when asked, writes `reply`
# and closes.
func serveRaw(listener as net.Listener, readFirst as bool, reply as bytes) {
    def conn as net.Conn init net.accept($listener);
    if ($readFirst) {
        net.readBytes($conn, 512);
    }
    net.writeBytes($conn, $reply);
    net.close($conn);
    return;
}

# A connection that closes before a status line ends is kind "closed": a
# partial "+OK" is no reply, and nothing at all is not an empty -ERR.
func testReadLineEofIsClosed() {
    for (def reply in ["+OK", ""]) {
        def listener as net.Listener init net.listen("127.0.0.1:0");
        def server as task of null init spawn {
            serveRaw($listener, false, convert.bytesFromString("utf-8", $reply));
        };
        def c as net.Conn init net.connect(net.address($listener));
        def kind as string init "none";
        try {
            readLine($c);
        } catch (e) {
            $kind = $e.kind;
        }
        testing.assertEqual($kind, "closed");
        task.wait($server);
        net.close($c);
        net.close($listener);
    }
}

# An 8-bit (Latin-1) message is read byte-exact by retrieveBytes; the text
# form throws a PopError instead of a runtime error.
func testNonUtf8Body() {
    def wire as bytes init encoding.fromText("hex", "2b4f4b0d0a6162e963640d0a2e0d0a");
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        serveRaw($listener, true, $wire);
    };
    def s as Session init Session{conn: net.connect(net.address($listener))};
    testing.assertEqual(encoding.toText("hex", retrieveBytes($s, 1)), "6162e96364");
    task.wait($server);
    net.close($s.conn);
    def server2 as task of null init spawn {
        serveRaw($listener, true, $wire);
    };
    def s2 as Session init Session{conn: net.connect(net.address($listener))};
    def kind as string init "none";
    try {
        retrieve($s2, 1);
    } catch (e) {
        $kind = $e.kind;
    }
    testing.assertEqual($kind, "pop3");
    task.wait($server2);
    net.close($s2.conn);
    net.close($listener);
}

# USER / PASS and XOAUTH2 over .None are refused before anything is sent,
# unless allowInsecureAuth is set.
func testPlaintextCredentialsRefused() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        def conn as net.Conn init net.accept($listener);
        def got as string init convert.stringFromBytes("utf-8", net.readBytes($conn, 512));
        net.writeBytes($conn, convert.bytesFromString("utf-8", "+OK\r\n"));
        def got2 as string init convert.stringFromBytes("utf-8", net.readBytes($conn, 512));
        net.writeBytes($conn, convert.bytesFromString("utf-8", "+OK\r\n"));
        net.close($conn);
        testing.assertEqual($got + $got2, "USER u\r\nPASS p\r\n");
    };
    def c as net.Conn init net.connect(net.address($listener));
    def opts as Options init Options{
        host: "",
        port: 0,
        security: transport.Security.None,
        user: "u",
        pass: "p",
        auth: "",
        allowInsecureAuth: false
    };
    for (def mech in ["", "xoauth2"]) {
        $opts.auth = $mech;
        def kind as string init "none";
        try {
            authenticate($c, $opts, "+OK hi");
        } catch (e) {
            $kind = $e.kind;
        }
        testing.assertEqual($kind, "pop");
    }
    $opts.auth = "";
    $opts.allowInsecureAuth = true;
    authenticate($c, $opts, "+OK hi");
    task.wait($server);
    net.close($c);
    net.close($listener);
    # an encrypted connection needs no opt-in
    $opts.allowInsecureAuth = false;
    $opts.security = transport.Security.Tls;
    refusePlaintext($opts, "USER / PASS");
}
