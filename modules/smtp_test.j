# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# smtp_test.j - white-box tests for smtp.j's pure protocol helpers. Run with:
#
#     jennifer test modules/smtp_test.j
#
# The overlay splices smtp.j in front of this file, so the tests reach its
# private helpers (parseCode, replyFinalCode, authPlain, dotStuff, crlf,
# clientName) by bare identifier. The networked `send` path is not covered
# here (it needs a live server); it is verified end to end against a local
# SMTP daemon, outside the offline suite.
use testing;
use task;
use encoding;
use time;
use binary;

func testParseCode() {
    testing.assertEqual(parseCode("250 OK"), 250);
    testing.assertEqual(parseCode("354 Go ahead"), 354);
    testing.assertEqual(parseCode("ab"), 0); # too short
}

func testReplyFinalCodeSingleLine() {
    testing.assertEqual(replyFinalCode("220 mail.example.com ready\r\n"), 220);
    testing.assertEqual(replyFinalCode("354 End data with <CR><LF>.<CR><LF>\r\n"), 354);
}

func testReplyFinalCodeMultiLine() {
    # Continuation lines use "250-"; only the final "250 " completes the reply.
    def r as string init "250-mail.example.com\r\n250-PIPELINING\r\n250 STARTTLS\r\n";
    testing.assertEqual(replyFinalCode($r), 250);
}

func testReplyFinalCodeIncomplete() {
    # Only a continuation line so far.
    testing.assertEqual(replyFinalCode("250-mail.example.com\r\n"), -1);
    testing.assertEqual(replyFinalCode(""), -1);
    testing.assertEqual(replyFinalCode("220 partial with no newline yet"), -1);
}

func testDotStuff() {
    testing.assertEqual(dotStuff(".leading"), "..leading");
    testing.assertEqual(dotStuff("normal\r\n.dot line\r\nend"), "normal\r\n..dot line\r\nend");
    testing.assertEqual(dotStuff("no dots here"), "no dots here");
}

func testCrlf() {
    testing.assertEqual(crlf("a\nb"), "a\r\nb");
    testing.assertEqual(crlf("a\r\nb"), "a\r\nb");
}

func testAsciiEnvelopeIdnDomain() {
    # An IDN domain is Punycode-encoded; the ASCII local part is kept.
    testing.assertEqual(asciiEnvelope("me@münchen.de"), "me@xn--mnchen-3ya.de");
    testing.assertEqual(asciiEnvelope("ok@example.com"), "ok@example.com");
}

func testAsciiEnvelopeRejectsNonAsciiLocal() {
    testing.assertThrows("idnLocal", "smtp");
}
func idnLocal() {
    return asciiEnvelope("kü@example.com");
}

func testClientNameDefault() {
    def bare as Options init Options{
        host: "h",
        port: 25,
        security: transport.Security.None,
        clientName: "",
        user: "",
        pass: "",
        auth: "",
        allowInsecureAuth: false
    };
    testing.assertEqual(clientName($bare), "localhost");
    def named as Options init Options{
        host: "h",
        port: 25,
        security: transport.Security.None,
        clientName: "me.example",
        user: "",
        pass: "",
        auth: "",
        allowInsecureAuth: false
    };
    testing.assertEqual(clientName($named), "me.example");
}

# ---- CRLF / control-character injection in addresses + EHLO name ----

func injectAddress() {
    asciiEnvelope("alice@example.com\r\nRCPT TO:<evil@x>");
}
func injectHelper() {
    rejectControl("host\r\nDATA", "test");
}

func testEnvelopeRejectsCrlf() {
    testing.assertThrows("injectAddress", "smtp");
}
func testRejectControlHelper() {
    testing.assertThrows("injectHelper", "smtp");
}
func testCleanEnvelopeAccepted() {
    testing.assertEqual(asciiEnvelope("alice@example.com"), "alice@example.com");
}

# ---- hardening: STARTTLS advertise, envelope validation, cleartext auth ----

func testEhloAdvertisesStartTls() {
    def caps as string init "250-mail.example.com\n250-STARTTLS\n250 AUTH PLAIN LOGIN";
    testing.assertTrue(ehloAdvertises($caps, "STARTTLS"));
    testing.assertTrue(ehloAdvertises($caps, "starttls"));
    testing.assertTrue(ehloAdvertises($caps, "AUTH"));
    testing.assertFalse(ehloAdvertises($caps, "PIPELINING"));
}

func testEhloStartTlsAbsent() {
    def caps as string init "250-mail.example.com\n250 AUTH PLAIN";
    testing.assertFalse(ehloAdvertises($caps, "STARTTLS"));
}

func testEnvelopeRejectsMissingAt() {
    testing.assertThrows("envNoAt", "smtp");
}
func envNoAt() {
    asciiEnvelope("nobody");
}

func testEnvelopeRejectsEmptyLocal() {
    testing.assertThrows("envEmptyLocal", "smtp");
}
func envEmptyLocal() {
    asciiEnvelope("@example.com");
}

func testEnvelopeRejectsEmptyDomain() {
    testing.assertThrows("envEmptyDomain", "smtp");
}
func envEmptyDomain() {
    asciiEnvelope("alice@");
}

func testAuthRefusedOverCleartext() {
    testing.assertThrows("cleartextAuth", "smtp");
}
func cleartextAuth() {
    def o as Options;
    $o.security = transport.Security.None;
    $o.user = "me@example.com";
    $o.pass = "secret";
    $o.auth = "plain";
    # allowInsecureAuth defaults false; the refusal must throw before any I/O.
    authenticate(net.Conn{id: -1}, $o, "");
}

func testAuthForcedSkipsCleartextRefusal() {
    def o as Options;
    $o.security = transport.Security.None;
    $o.user = "me@example.com";
    $o.pass = "secret";
    $o.auth = "plain";
    $o.allowInsecureAuth = true;
    def refused as bool init false;
    try {
        authenticate(net.Conn{id: -1}, $o, "");
    } catch (e) {
        # Still fails on the invalid conn, but NOT with the cleartext refusal.
        $refused = strings.contains($e.message, "refusing to send SASL");
    }
    testing.assertFalse($refused);
}

# sendOn on a closed session fails fast (before touching the wire).
func sendOnClosed() {
    def s as Session init Session{conn: net.Conn{id: -1}, open: false};
    sendOn($s, "from@example.com", ["to@example.com"], "hi");
}

func testSendOnClosedSessionThrows() {
    testing.assertThrows("sendOnClosed", "smtp");
}

# A bare CR or LF before "." cannot end DATA early on a lenient server: every
# line ending becomes CRLF and the "." line is stuffed.
func testSmugglingNormalized() {
    testing.assertEqual(dotStuff(crlf("a\r.\rb")), "a\r\n..\r\nb");
    testing.assertEqual(dotStuff(crlf("a\n.\r\nb")), "a\r\n..\r\nb");
}

# A rejected command is an SmtpError with the reply code, the enhanced status
# code, the command, and whether it is temporary; a local check has code 0.
func testRejectionsAreSmtpErrors() {
    try {
        fail(Reply{code: 550, text: "550 5.1.1 <x@example.com>: User unknown\r\n"}, "RCPT TO");
        throw Error{kind: "missed", message: "a 550"};
    } catch (e) {
        testing.assertEqual($e.kind, "smtp");
        testing.assertEqual($e.code, 550);
        testing.assertEqual($e.enhancedCode, "5.1.1");
        testing.assertEqual($e.command, "RCPT TO");
        testing.assertFalse($e.temporary);
        testing.assertEqual($e.message, "RCPT TO: 550 550 5.1.1 <x@example.com>: User unknown");
    }
    try {
        fail(Reply{code: 451, text: "451-Greylisted\r\n451 4.7.1 try again later\r\n"}, "DATA");
        throw Error{kind: "missed", message: "a 451"};
    } catch (e) {
        testing.assertEqual($e.code, 451);
        testing.assertEqual($e.enhancedCode, "4.7.1");
        testing.assertTrue($e.temporary);
    }
    try {
        fail(Reply{code: 554, text: "554 no thanks\r\n"}, "MAIL FROM");
        throw Error{kind: "missed", message: "a 554"};
    } catch (e) {
        testing.assertEqual($e.enhancedCode, "");
    }
    testing.assertThrows("controlInAddress", "smtp");
}

func controlInAddress() {
    rejectControl("a\nb@example.com", "address");
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

# A 235 straight after the client-final skips the server-final: the server
# never proved it knows the password, so the session is refused.
func testScramWithoutServerFinalIsRefused() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        def conn as net.Conn init net.accept($listener);
        def line as string init convert.stringFromBytes("utf-8", net.readBytes($conn, 512));
        def parts as list of string init strings.split(strings.trim($line), " ");
        net.writeBytes(
            $conn,
            convert.bytesFromString("utf-8", "334 " + scramFirstFor($parts[2]) + "\r\n"));
        net.readBytes($conn, 512);
        net.writeBytes($conn, convert.bytesFromString("utf-8", "235 2.7.0 accepted\r\n"));
        net.readBytes($conn, 512);
        net.close($conn);
    };
    def c as net.Conn init net.connect(net.address($listener));
    def opts as Options init Options{
        host: "",
        port: 0,
        security: transport.Security.None,
        clientName: "",
        user: "u",
        pass: "p",
        auth: "scram-sha-256",
        allowInsecureAuth: true
    };
    try {
        scramAuth($c, $opts, "scram-sha-256");
        throw Error{kind: "missed", message: "a SCRAM success without server-final"};
    } catch (e) {
        testing.assertEqual($e.kind, "smtp");
    }
    net.close($c);
    task.wait($server);
    net.close($listener);
}

# An address that closes its angle brackets, or carries a space, could append
# ESMTP parameters to MAIL FROM / RCPT TO (`a@b> NOTIFY=NEVER`): only the RFC
# 5321 grammar passes - a dot-string or quoted local part, an LDH domain or an
# address literal.
func testEnvelopeRejectsParameterInjection() {
    def bad as list of string init [
        "a@example.com> RET=HDRS",
        "a@example.com>",
        "a b@example.com",
        "a<b@example.com",
        "a@exa mple.com",
        "a..b@example.com",
        ".a@example.com",
        "\"open@example.com",
        "a@[192.0.2.1> X]",
        "a@-example.com"
    ];
    def wrong as list of string init [];
    for (def addr in $bad) {
        def kind as string init "none";
        try {
            asciiEnvelope($addr);
        } catch (e) {
            $kind = $e.kind;
        }
        if ($kind != "smtp") {
            $wrong[] = $kind + ": " + $addr;
        }
    }
    testing.assertEqual($wrong, []);
    testing.assertEqual(asciiEnvelope("\"a b<c>\"@example.com"), "\"a b<c>\"@example.com");
    testing.assertEqual(asciiEnvelope("a.b+tag@example.com"), "a.b+tag@example.com");
    testing.assertEqual(asciiEnvelope("a@[192.0.2.1]"), "a@[192.0.2.1]");
}

# replyServer answers one client read with each chunk of `chunks` in turn,
# pausing between them so each arrives as its own read, then closes.
func replyServer(listener as net.Listener, chunks as list of bytes) {
    def conn as net.Conn init net.accept($listener);
    for (def c in $chunks) {
        net.writeBytes($conn, $c);
        time.sleep(time.fromMilliseconds(100));
    }
    net.close($conn);
    return null;
}

func readReplyFrom(chunks as list of bytes) {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        return replyServer($listener, $chunks);
    };
    def c as net.Conn init net.connect(net.address($listener));
    defer net.close($listener);
    defer task.wait($server);
    defer net.close($c);
    return readReply($c);
}

# A UTF-8 character split across two reads is decoded once whole.
func testReplyUtf8SplitAcrossReads() {
    def text as bytes init convert.bytesFromString("utf-8", "250 grüße\r\n");
    def r as Reply init readReplyFrom([binary.slice($text, 0, 7), binary.slice($text, 7)]);
    testing.assertEqual($r.code, 250);
    testing.assertEqual($r.text, "250 grüße\r\n");
}

# A final line cut off by the server closing ("250 que", no CRLF) is not a
# reply: the outcome is unknown, so it throws "closed" instead of succeeding.
func testTruncatedReplyIsClosed() {
    def kind as string init "none";
    try {
        readReplyFrom([convert.bytesFromString("utf-8", "250 que")]);
    } catch (e) {
        $kind = $e.kind;
    }
    testing.assertEqual($kind, "closed");
    $kind = "none";
    try {
        readReplyFrom([]);
    } catch (e) {
        $kind = $e.kind;
    }
    testing.assertEqual($kind, "closed");
}
