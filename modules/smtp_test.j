# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
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

func testParseCode() {
    testing.assertEqual(parseCode("250 OK"), 250);
    testing.assertEqual(parseCode("354 Go ahead"), 354);
    testing.assertEqual(parseCode("ab"), 0);   # too short
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
    def bare as Options init Options{host: "h", port: 25, security: "none",
        clientName: "", user: "", pass: "", auth: "", allowInsecureAuth: false};
    testing.assertEqual(clientName($bare), "localhost");
    def named as Options init Options{host: "h", port: 25, security: "none",
        clientName: "me.example", user: "", pass: "", auth: "", allowInsecureAuth: false};
    testing.assertEqual(clientName($named), "me.example");
}


# ---- CRLF / control-character injection in addresses + EHLO name ----

func injectAddress() { asciiEnvelope("alice@example.com\r\nRCPT TO:<evil@x>"); }
func injectHelper() { rejectControl("host\r\nDATA", "test"); }

func testEnvelopeRejectsCrlf() {
    testing.assertThrows("injectAddress", "smtp");
}
func testRejectControlHelper() {
    testing.assertThrows("injectHelper", "smtp");
}
func testCleanEnvelopeAccepted() {
    testing.assertEqual(asciiEnvelope("alice@example.com"), "alice@example.com");
}

# ---- M21.9 hardening: STARTTLS advertise, envelope validation, cleartext auth ----

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
func envNoAt() { asciiEnvelope("nobody"); }

func testEnvelopeRejectsEmptyLocal() {
    testing.assertThrows("envEmptyLocal", "smtp");
}
func envEmptyLocal() { asciiEnvelope("@example.com"); }

func testEnvelopeRejectsEmptyDomain() {
    testing.assertThrows("envEmptyDomain", "smtp");
}
func envEmptyDomain() { asciiEnvelope("alice@"); }

func testAuthRefusedOverCleartext() {
    testing.assertThrows("cleartextAuth", "smtp");
}
func cleartextAuth() {
    def o as Options;
    $o.security = "none";
    $o.user = "me@example.com";
    $o.pass = "secret";
    $o.auth = "plain";
    # allowInsecureAuth defaults false; the refusal must throw before any I/O.
    authenticate(net.Conn{id: -1}, $o, "");
}

func testAuthForcedSkipsCleartextRefusal() {
    def o as Options;
    $o.security = "none";
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
