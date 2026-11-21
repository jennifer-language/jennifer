# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# imap_test.j - white-box tests for imap.j's pure protocol helpers. Run with:
#
#     jennifer test modules/imap_test.j
#
# The overlay splices imap.j in front of this file, so the tests reach its
# private helpers (literalLength, fetchBody, parseExists, parseSearch,
# isTagged, quoteArg, expectTaggedOK) by bare identifier. The dialogue
# regressions run against a scripted in-process server (serveScript below); the
# networked session is verified end to end in the Go suite (TestImapReceive).
use testing;
use task;
use encoding;
use lists;
use binary;
use time;

func testLiteralLength() {
    testing.assertEqual(literalLength('* 1 FETCH (BODY[] {1234}'), 1234);
    testing.assertEqual(literalLength("* 2 EXISTS"), -1);
}

# bodyOf runs fetchBody over a response given as text.
func bodyOf(resp as string, uid as int) {
    return encoding.decode("utf-8", fetchBody(encoding.encode("utf-8", $resp), $uid));
}

func testFetchBody() {
    def resp as string init "* 1 FETCH (UID 7 BODY[] \{11\}\r\nHELLO WORLD)\r\nJEN OK\r\n";
    testing.assertEqual(bodyOf($resp, 7), "HELLO WORLD");
    # The UID may follow the literal.
    testing.assertEqual(bodyOf("* 1 FETCH (BODY[] \{2\}\r\nhi UID 7)\r\nJEN OK\r\n", 7), "hi");
    # A quoted or NIL body section.
    testing.assertEqual(bodyOf("* 1 FETCH (UID 7 BODY[] \"a\\\"b\")\r\nJEN OK\r\n", 7), "a\"b");
    testing.assertEqual(bodyOf("* 1 FETCH (UID 7 BODY[] NIL)\r\nJEN OK\r\n", 7), "");
}

# A FETCH of a UID that does not exist is answered OK with no FETCH response;
# fetchBody must not return "" for it.
func fetchMissingUid() {
    bodyOf("JEN OK UID FETCH completed\r\n", 999);
}
# A FETCH response for another message is not the one asked for.
func fetchOtherUid() {
    bodyOf("* 1 FETCH (UID 8 BODY[] \{2\}\r\nhi)\r\nJEN OK\r\n", 7);
}
func testFetchBodyNeedsTheUid() {
    testing.assertThrows("fetchMissingUid", "imap");
    testing.assertThrows("fetchOtherUid", "imap");
}

func testParseExists() {
    testing.assertEqual(parseExists("* 2 EXISTS\r\n* 0 RECENT\r\nJEN OK done\r\n"), 2);
}
func existsMissing() {
    parseExists("JEN OK nothing\r\n");
}
func testParseExistsMissing() {
    testing.assertThrows("existsMissing", "imap");
}

func testParseSearch() {
    def nums as list of int init parseSearch("* SEARCH 1 2 5\r\nJEN OK done\r\n");
    testing.assertEqual(len($nums), 3);
    testing.assertEqual($nums[0], 1);
    testing.assertEqual($nums[2], 5);
    testing.assertEqual(len(parseSearch("* SEARCH\r\nJEN OK done\r\n")), 0);
}
func searchMissing() {
    parseSearch("JEN OK done\r\n");
}
func searchMalformed() {
    parseSearch("* SEARCH 1 x2\r\nJEN OK done\r\n");
}
func testParseSearchStrict() {
    testing.assertThrows("searchMissing", "imap");
    testing.assertThrows("searchMalformed", "imap");
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
    expectTaggedOK("JEN OK completed", "JEN"); # no throw
    testing.assertTrue(true);
}

# ---- read cap (DoS from an oversized literal / unterminated response) ----
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

func injectImapCRLF() {
    quoteArg("user\r\nA1 LOGOUT");
}
# A raw-interpolated argument (a STORE flag list, as addFlags/removeFlags build)
# is caught by the command() choke-point guard, which runs before the socket
# write - so a zero Conn is fine here, rejectControl throws first.
func injectImapFlags() {
    def c as net.Conn;
    command(newSession($c), "STORE 1 +FLAGS.SILENT (x)\r\nA1 LOGOUT\r\n(y)");
}
func testImapInjectionBlocked() { # OM-006
    testing.assertThrows("injectImapCRLF", "imap");
    testing.assertThrows("injectImapFlags", "imap");
    quoteArg("normal.user@example.com"); # a valid arg does not throw
}

# --- criteria-based search (pure helpers) -----------------------------------

func testBuildSearchAll() {
    testing.assertEqual(buildSearchCommand(criteria()), "SEARCH ALL");
}

func testBuildSearchMultiField() {
    def c as Criteria;
    $c.subject = "hi";
    $c.unseen = true;
    $c.since = time.fromIso("2026-01-01T00:00:00Z"); # rendered internally
    $c.largerThan = 1000;
    testing.assertEqual(
        buildSearchCommand($c),
        "SEARCH SUBJECT \"hi\" SINCE 01-Jan-2026 UNSEEN LARGER 1000");
    # from / to / text / before / flags render too, in order.
    def d as Criteria;
    $d.from = "billing@x.com";
    $d.flagged = true;
    $d.before = time.fromIso("2026-03-01T00:00:00Z");
    testing.assertEqual(
        buildSearchCommand($d),
        "SEARCH FROM \"billing@x.com\" BEFORE 01-Mar-2026 FLAGGED");
    # A client-side-only criteria still needs a server search (defaults to ALL).
    def e as Criteria;
    $e.subjectRegex = "INV-[0-9]+";
    testing.assertEqual(buildSearchCommand($e), "SEARCH ALL");
    testing.assertTrue(hasClientFilter($e));
    testing.assertFalse(hasClientFilter($c));
}

# The date range comes from time.Time values rendered internally, so a zero-value
# date is simply omitted (no SINCE / BEFORE token) and there is no date string to
# validate or inject.
func testUnsetDateOmitted() {
    def c as Criteria;
    $c.subject = "x"; # since / before left at their zero-value time
    testing.assertEqual(buildSearchCommand($c), "SEARCH SUBJECT \"x\"");
}

# A search substring with a control character is rejected before the wire
# (quoteArg control-checks it).
func injectSearchSubject() {
    def c as Criteria;
    $c.subject = "x\r\nA1 LOGOUT";
    buildSearchCommand($c);
}
func testSearchInjectionBlocked() {
    testing.assertThrows("injectSearchSubject", "imap");
}

func testBodyStructureAttachmentHeuristic() {
    def withAtt as string init "* 1 FETCH (BODYSTRUCTURE ((\"text\" \"plain\" ...)(\"application\" \"pdf\" (\"name\" \"a.pdf\") NIL NIL \"base64\" 1234)(\"attachment\" (\"filename\" \"a.pdf\")) ...))";
    testing.assertTrue(bodyStructureShowsAttachment($withAtt));
    def plain as string init "* 1 FETCH (BODYSTRUCTURE (\"text\" \"plain\" (\"charset\" \"utf-8\") NIL NIL \"7bit\" 42 3))";
    testing.assertFalse(bodyStructureShowsAttachment($plain));
    # Case-insensitive (some servers upper-case the disposition).
    testing.assertTrue(bodyStructureShowsAttachment("(\"ATTACHMENT\" NIL)"));
}

# --- sub-day (time-of-day) date refinement ----------------------------------

func testHasTimeOfDay() {
    testing.assertFalse(hasTimeOfDay(time.fromIso("2026-01-01T00:00:00Z"))); # midnight
    testing.assertTrue(hasTimeOfDay(time.fromIso("2026-01-01T14:30:00Z")));
    testing.assertTrue(hasTimeOfDay(time.fromIso("2026-01-01T00:00:01Z"))); # one second in
}

# A midnight `before` is exact (server BEFORE that day); a `before` with a
# time-of-day widens the server date to the next day so the client pass can keep
# that day's earlier messages.
func testBeforeWidening() {
    def midnight as Criteria;
    $midnight.before = time.fromIso("2026-01-20T00:00:00Z");
    testing.assertEqual(buildSearchCommand($midnight), "SEARCH BEFORE 20-Jan-2026");
    def timed as Criteria;
    $timed.before = time.fromIso("2026-01-20T10:00:00Z");
    testing.assertEqual(buildSearchCommand($timed), "SEARCH BEFORE 21-Jan-2026"); # widened
    # `since` is never widened (inclusive on its own day is already a superset).
    def s as Criteria;
    $s.since = time.fromIso("2026-01-15T14:30:00Z");
    testing.assertEqual(buildSearchCommand($s), "SEARCH SINCE 15-Jan-2026");
}

# A time-of-day on since/before turns on the client-side pass; a midnight bound
# stays pure server-side.
func testRefinePredicates() {
    def timed as Criteria;
    $timed.since = time.fromIso("2026-01-15T14:30:00Z");
    testing.assertTrue(refineSinceNeeded($timed));
    testing.assertTrue(hasClientFilter($timed)); # forces the client pass
    def midnight as Criteria;
    $midnight.since = time.fromIso("2026-01-15T00:00:00Z");
    testing.assertFalse(refineSinceNeeded($midnight));
    testing.assertFalse(hasClientFilter($midnight)); # pure server-side
    def unset as Criteria;
    testing.assertFalse(refineSinceNeeded($unset));
    testing.assertFalse(refineBeforeNeeded($unset));
}

func testParseInternalDate() {
    def id as time.Time init parseInternalDate("* 1 FETCH (INTERNALDATE \"15-Jan-2026 14:30:00 +0100\")");
    testing.assertEqual(time.format($id, "%Y-%m-%dT%H:%M:%S%z"), "2026-01-15T14:30:00+0100");
    # A single-digit day is space-padded on the wire; it still parses.
    def sp as time.Time init parseInternalDate("* 2 FETCH (INTERNALDATE \" 5-Jan-2026 09:00:00 +0000\")");
    testing.assertEqual(time.format($sp, "%Y-%m-%dT%H:%M:%S%z"), "2026-01-05T09:00:00+0000");
}
func missingInternalDate() {
    parseInternalDate("* 1 FETCH (FLAGS ())");
}
func testParseInternalDateMissing() {
    testing.assertThrows("missingInternalDate", "imap");
}

# An inverted date range (since after before) is a catchable error, not a silent
# empty result; since == before (an empty range) is allowed.
func invertedRange() {
    def c as Criteria;
    $c.since = time.fromIso("2026-03-01T00:00:00Z");
    $c.before = time.fromIso("2026-01-01T00:00:00Z");
    buildSearchCommand($c);
}
func testInvertedRangeRejected() {
    testing.assertThrows("invertedRange", "imap");
    # since <= before is fine (a valid, possibly-empty range).
    def ok as Criteria;
    $ok.since = time.fromIso("2026-01-01T00:00:00Z");
    $ok.before = time.fromIso("2026-03-01T00:00:00Z");
    testing.assertEqual(buildSearchCommand($ok), "SEARCH SINCE 01-Jan-2026 BEFORE 01-Mar-2026");
}

# --- LIST / STATUS parsing (pure helpers) -----------------------------------

func testParseListLine() {
    def mb as Folder init parseListLine("* LIST (\\HasNoChildren \\Marked) \"/\" \"INBOX\"", []);
    testing.assertEqual($mb.name, "INBOX");
    testing.assertEqual($mb.delimiter, "/");
    testing.assertEqual(len($mb.flags), 2);
    testing.assertEqual($mb.flags[0], "\\HasNoChildren");
    # A \Noselect parent with a NIL delimiter and an atom name.
    def nil as Folder init parseListLine("* LIST (\\Noselect) NIL Archive", []);
    testing.assertEqual($nil.name, "Archive");
    testing.assertEqual($nil.delimiter, "");
    testing.assertEqual($nil.flags[0], "\\Noselect");
}

func testParseList() {
    def resp as string init "* LIST (\\HasNoChildren) \"/\" \"INBOX\"\r\n* LIST (\\HasChildren) \"/\" \"Archive\"\r\n* LIST (\\HasNoChildren) \"/\" \"Archive/2024\"\r\nJEN OK LIST completed\r\n";
    def boxes as list of Folder init parseList(encoding.encode("utf-8", $resp));
    testing.assertEqual(len($boxes), 3);
    testing.assertEqual($boxes[0].name, "INBOX");
    testing.assertEqual($boxes[2].name, "Archive/2024");
}

func testParseStatus() {
    def s as Status init parseStatus("* STATUS \"INBOX\" (MESSAGES 231 RECENT 0 UNSEEN 5 UIDNEXT 4321 UIDVALIDITY 1)\r\nJEN OK STATUS completed\r\n");
    testing.assertEqual($s.messages, 231);
    testing.assertEqual($s.unseen, 5);
    testing.assertEqual($s.recent, 0);
    testing.assertEqual($s.uidnext, 4321);
    testing.assertEqual($s.uidvalidity, 1);
    # Items may arrive in any order; only requested keys set, the rest stay 0.
    def partial as Status init parseStatus("* STATUS \"Sent\" (UNSEEN 2 MESSAGES 40)\r\nJEN OK done\r\n");
    testing.assertEqual($partial.messages, 40);
    testing.assertEqual($partial.unseen, 2);
    testing.assertEqual($partial.uidnext, 0);
}

# --- regression tests for the review findings -------------------------------

# fetchBody: the {N} is a BYTE count, so a multi-byte body must come back
# byte-exact, not over-read by (bytes - runes) into the trailing `)`/CRLF/tag.
# "HÉLLO" is 6 bytes / 5 runes -> marker {6}. (assertEqual, not assertContains.)
func testFetchBodyMultibyte() {
    testing.assertEqual(
        bodyOf("* 1 FETCH (UID 1 BODY[] \{6\}\r\nHÉLLO)\r\nJEN OK\r\n", 1),
        "HÉLLO");
    # An ASCII body still round-trips exactly.
    testing.assertEqual(
        bodyOf("* 1 FETCH (UID 1 BODY[] \{5\}\r\nplain)\r\nJEN OK\r\n", 1),
        "plain");
}

# parseStatus: a folder name containing a '(' must not be mistaken for the item
# list (the items are the trailing paren group).
func testParseStatusParenInName() {
    def s as Status init parseStatus("* STATUS \"My(Folder\" (MESSAGES 7 UNSEEN 2)\r\nJEN OK\r\n");
    testing.assertEqual($s.messages, 7);
    testing.assertEqual($s.unseen, 2);
}

# --- IDLE / server-push parsing (pure helpers) ------------------------------

# parseNotification turns an untagged push line into a typed Notification; a
# non-push line (tagged completion, "* SEARCH", anything else) is the empty
# sentinel. The live IDLE dialogue is Go-tested against the imap mock.
func testParseNotification() {
    def exists as Notification init parseNotification("* 5 EXISTS");
    testing.assertEqual($exists.kind, "exists");
    testing.assertEqual($exists.number, 5);
    def expunge as Notification init parseNotification("* 2 EXPUNGE");
    testing.assertEqual($expunge.kind, "expunge");
    testing.assertEqual($expunge.number, 2);
    def recent as Notification init parseNotification("* 3 RECENT");
    testing.assertEqual($recent.kind, "recent");
    testing.assertEqual($recent.number, 3);
    # A lower-case keyword still parses (kind is normalized to lower).
    testing.assertEqual(parseNotification("* 9 exists").kind, "exists");
}

# A line that is not a modeled push becomes the empty sentinel (kind "").
func testParseNotificationSentinel() {
    testing.assertEqual(parseNotification("JEN OK IDLE completed").kind, "");
    testing.assertEqual(parseNotification("* SEARCH 1 2 5").kind, "");
    testing.assertEqual(parseNotification("* 4 FETCH (FLAGS (\\Seen))").kind, "");
    def none as Notification init noNotification();
    testing.assertEqual($none.kind, "");
    testing.assertEqual($none.number, 0);
}

# isContinuation recognizes the "+ idling" (or bare "+") IDLE continuation.
func testIsContinuation() {
    testing.assertTrue(isContinuation("+ idling"));
    testing.assertTrue(isContinuation("+"));
    testing.assertTrue(isContinuation("+ OK still here"));
    testing.assertFalse(isContinuation("* 5 EXISTS"));
    testing.assertFalse(isContinuation("JEN OK completed"));
}

# hasCapability detects a whole-token capability, case-insensitively, in a
# CAPABILITY response (the gate supportsIdle uses).
func testHasCapability() {
    def resp as string init "* CAPABILITY IMAP4rev1 IDLE STARTTLS AUTH=PLAIN\r\nJEN OK CAPABILITY completed\r\n";
    testing.assertTrue(hasCapability($resp, "IDLE"));
    testing.assertTrue(hasCapability($resp, "idle")); # case-insensitive
    testing.assertTrue(hasCapability($resp, "STARTTLS"));
    testing.assertFalse(hasCapability($resp, "MOVE"));
    # A capability name that is only a substring of a token does not match.
    testing.assertFalse(hasCapability($resp, "IDL"));
}

# A NO / BAD completion is an ImapError with the status, the response code and
# the text; another failure has none.
func testCompletionsAreImapErrors() {
    def e as ImapError init completionError(
        "a1 NO [AUTHENTICATIONFAILED] Invalid credentials",
        "a1");
    testing.assertEqual($e.kind, "imap");
    testing.assertEqual($e.status, "NO");
    testing.assertEqual($e.responseCode, "AUTHENTICATIONFAILED");
    testing.assertEqual($e.text, "Invalid credentials");
    testing.assertEqual($e.message, "a1 NO [AUTHENTICATIONFAILED] Invalid credentials");
    $e = completionError("a1 NO [TRYCREATE \"Archive\"] no such mailbox", "a1");
    testing.assertEqual($e.responseCode, "TRYCREATE");
    testing.assertEqual($e.text, "no such mailbox");
    $e = completionError("a1 BAD command unknown", "a1");
    testing.assertEqual($e.status, "BAD");
    testing.assertEqual($e.responseCode, "");
    testing.assertEqual($e.text, "command unknown");
    try {
        expectTaggedOK("a1 NO [OVERQUOTA] over quota", "a1");
        throw Error{kind: "missed", message: "a NO"};
    } catch (err) {
        testing.assertEqual($err.responseCode, "OVERQUOTA");
    }
    testing.assertEqual(imapError("x").status, "");
}

# A server that closes before the tagged completion is kind "closed", not an
# ImapError.
func testAPeerCloseIsClosed() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        def conn as net.Conn init net.accept($listener);
        net.writeBytes($conn, encoding.encode("utf-8", "* 1 FETCH (BODY[] \{10\}\r\nAB"));
        net.close($conn);
    };
    def c as net.Conn init net.connect(net.address($listener));
    try {
        readResponse(newSession($c), "A1", false);
        throw Error{kind: "missed", message: "a close mid-literal"};
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
    def cf as string init encoding.decode(
        "utf-8",
        encoding.fromText("base64", strings.trim($clientFirst)));
    def nonce as string init $cf[strings.indexOf($cf, ",r=") + 3..];
    return encoding.toText(
        "base64",
        encoding.encode("utf-8", "r=" + $nonce + "srv,s=c2FsdA==,i=4096"));
}

# A tagged OK straight after the client-final skips the server-final: the
# server never proved it knows the password, so the session is refused.
func testScramWithoutServerFinalIsRefused() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        def conn as net.Conn init net.accept($listener);
        net.readBytes($conn, 512);
        net.writeBytes($conn, encoding.encode("utf-8", "+ \r\n"));
        def line as string init encoding.decode("utf-8", net.readBytes($conn, 512));
        net.writeBytes($conn, encoding.encode("utf-8", "+ " + scramFirstFor($line) + "\r\n"));
        net.readBytes($conn, 512);
        net.writeBytes($conn, encoding.encode("utf-8", TAG + " OK AUTHENTICATE completed\r\n"));
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
        allowInsecureAuth: true
    };
    try {
        scramAuth(newSession($c), $opts, "scram-sha-256");
        throw Error{kind: "missed", message: "a SCRAM success without server-final"};
    } catch (e) {
        testing.assertEqual($e.kind, "imap");
    }
    net.close($c);
    task.wait($server);
    net.close($listener);
}

# --- scripted dialogues ------------------------------------------------------

# clientLine reads one line the client sent, CRLF stripped ("" at EOF).
func clientLine(conn as net.Conn) {
    def parts as list of bytes init [];
    while (true) {
        def c as bytes init net.readBytes($conn, 1);
        if (len($c) == 0 or $c == encoding.encode("utf-8", "\n")) {
            break;
        }
        $parts[] = $c;
    }
    return strings.trim(encoding.decode("utf-8", binary.join($parts)));
}

# serveScript accepts one client and plays `script`: "<" reads one client line
# (collected into the result), "!close" hangs up, "!pause" waits 100 ms, "hex:"
# sends raw bytes, anything else is sent as is. At the end it waits for the
# client to hang up.
func serveScript(listener as net.Listener, script as list of string) {
    def conn as net.Conn init net.accept($listener);
    def got as list of string init [];
    for (def st in $script) {
        if ($st == "<") {
            $got[] = clientLine($conn);
        } elseif ($st == "!close") {
            net.close($conn);
            return $got;
        } elseif ($st == "!pause") {
            time.sleep(time.fromMilliseconds(100));
        } elseif (strings.startsWith($st, "hex:")) {
            net.writeBytes($conn, encoding.fromText("hex", $st[4..]));
        } else {
            net.writeBytes($conn, encoding.encode("utf-8", $st));
        }
    }
    try {
        net.readAll($conn);
    } catch (e) {
        testing.assertTrue(true);
    }
    net.close($conn);
    return $got;
}

# Dialogue is a scripted server and the session connected to it (LOGIN done).
def struct Dialogue {
    listener as net.Listener,
    server as task of list of string,
    session as Session
};

# dialogue starts a scripted server whose script follows the greeting and the
# LOGIN completion, and connects to it.
func dialogue(script as list of string) {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def full as list of string init lists.concat(["* OK hi\r\n", "<", "JEN OK done\r\n"], $script);
    def server as task of list of string init spawn {
        return serveScript($listener, $full);
    };
    def port as int init convert.toInt(strings.split(net.address($listener), ":")[1]);
    def s as Session init connect(Options{
        host: "127.0.0.1",
        port: $port,
        security: transport.Security.None,
        user: "me",
        pass: "secret",
        auth: "",
        allowInsecureAuth: true
    });
    return Dialogue{listener: $listener, server: $server, session: $s};
}

# finish hangs up and returns the client lines the server read.
func finish(d as Dialogue) {
    net.close($d.session.conn);
    def got as list of string init task.wait($d.server);
    net.close($d.listener);
    return $got;
}

# Two pushes in one read, and a push in the same read as "+ idling": none is
# lost (the 512-byte line reader dropped every byte after the first line).
func testIdleKeepsPushesReadTogether() {
    def d as Dialogue init dialogue([
        "<",
        "+ idling\r\n* 4 EXISTS\r\n",
        "!pause",
        "* 5 EXISTS\r\n* 1 RECENT\r\n"
    ]);
    idle($d.session);
    testing.assertEqual(pollNotification($d.session, 2000).number, 4);
    testing.assertEqual(pollNotification($d.session, 2000).number, 5);
    def r as Notification init pollNotification($d.session, 2000);
    testing.assertEqual($r.kind, "recent");
    testing.assertEqual($r.number, 1);
    finish($d);
}

# A push the server sends before "+ idling" is kept for the first poll.
func testIdleKeepsPushBeforeContinuation() {
    def d as Dialogue init dialogue(["<", "* 9 EXISTS\r\n+ idling\r\n"]);
    idle($d.session);
    testing.assertEqual(pollNotification($d.session, 2000).number, 9);
    finish($d);
}

# done() with a push and the tagged completion in one read returns at once
# (it waited 30 s, then threw a runtime slice error).
func testDoneReadsCompletionAfterPush() {
    def d as Dialogue init dialogue([
        "<",
        "+ idling\r\n",
        "<",
        "* 6 EXISTS\r\nJEN OK IDLE terminated\r\n"
    ]);
    idle($d.session);
    def start as time.Time init time.now();
    done($d.session);
    testing.assertTrue(time.milliseconds(time.sub(time.now(), $start)) < 5000);
    testing.assertEqual(finish($d)[2], "DONE");
}

# A response's completion and the next response's data in one read: the second
# command still gets its own data.
func testResponseLeavesTheRestBuffered() {
    def d as Dialogue init dialogue([
        "<",
        "* 3 EXISTS\r\nJEN OK selected\r\n* SEARCH 4 5\r\n",
        "<",
        "JEN OK searched\r\n"
    ]);
    testing.assertEqual(selectFolder($d.session, "INBOX"), 3);
    testing.assertEqual(search($d.session, criteria()), [4, 5]);
    finish($d);
}

# A peer that closes during IDLE is kind "closed", not an endless sentinel.
func idleOnClosed() {
    def d as Dialogue init dialogue(["<", "+ idling\r\n", "!close"]);
    idle($d.session);
    try {
        pollNotification($d.session, 2000);
    } catch (e) {
        net.close($d.listener);
        throw $e;
    }
}
func testIdleClosedPeerIsClosed() {
    testing.assertThrows("idleOnClosed", "closed");
}

# fetch / flags of a UID the server does not report throw instead of "".
func fetchUnknown() {
    def d as Dialogue init dialogue(["<", "JEN OK UID FETCH completed\r\n"]);
    try {
        fetch($d.session, 999);
    } catch (e) {
        finish($d);
        throw $e;
    }
}
func flagsUnknown() {
    def d as Dialogue init dialogue(["<", "* 1 FETCH (FLAGS (\\Seen))\r\nJEN OK done\r\n"]);
    try {
        flags($d.session, 999);
    } catch (e) {
        finish($d);
        throw $e;
    }
}
func testUnknownUidThrows() {
    testing.assertThrows("fetchUnknown", "imap");
    testing.assertThrows("flagsUnknown", "imap");
}

# flags() reads the FETCH carrying the UID, not an unsolicited one before it.
func testFlagsMatchTheUid() {
    def d as Dialogue init dialogue([
        "<",
        "* 1 FETCH (FLAGS (\\Seen \\Deleted))\r\n* 7 FETCH (UID 42 FLAGS (\\Flagged))\r\nJEN OK\r\n"
    ]);
    testing.assertEqual(flags($d.session, 42), "\\Flagged");
    finish($d);
}

# An 8-bit (Latin-1) body: fetchBytes returns it byte-exact, fetch throws an
# ImapError naming fetchBytes (it was a runtime error).
func latin1Fetch() {
    def d as Dialogue init dialogue([
        "<",
        "* 1 FETCH (UID 1 BODY[] \{5\}\r\n",
        "hex:6162e96364",
        ")\r\nJEN OK\r\n"
    ]);
    try {
        fetch($d.session, 1);
    } catch (e) {
        finish($d);
        throw $e;
    }
}
func testEightBitBody() {
    def d as Dialogue init dialogue([
        "<",
        "* 1 FETCH (UID 1 BODY[] \{5\}\r\n",
        "hex:6162e96364",
        ")\r\nJEN OK\r\n"
    ]);
    testing.assertEqual(encoding.toText("hex", fetchBytes($d.session, 1)), "6162e96364");
    finish($d);
    testing.assertThrows("latin1Fetch", "imap");
}

# A LIST name sent as a literal is the literal's content (it was "7").
func testListLiteralName() {
    def d as Dialogue init dialogue([
        "<",
        "* LIST () \"/\" \{7\}\r\nfoo\"bar\r\n* LIST () \"/\" INBOX\r\nJEN OK\r\n"
    ]);
    def boxes as list of Folder init folders($d.session, "*");
    testing.assertEqual($boxes[0].name, "foo\"bar");
    testing.assertEqual($boxes[1].name, "INBOX");
    finish($d);
}
func malformedList() {
    parseListLine("* LIST garbage", []);
}
func missingStatus() {
    parseStatus("JEN OK STATUS completed\r\n");
}
func malformedStatus() {
    parseStatus("* STATUS INBOX (MESSAGES x)\r\nJEN OK\r\n");
}
func testListAndStatusStrict() {
    testing.assertThrows("malformedList", "imap");
    testing.assertThrows("missingStatus", "imap");
    testing.assertThrows("malformedStatus", "imap");
}

# CRAM-MD5 with an untagged CAPABILITY before the tagged OK (it was read as
# the completion and thrown). CRAM never sends the password, so it needs no
# allowInsecureAuth over Security.None.
func testCramSkipsUntagged() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of list of string init spawn {
        return serveScript(
            $listener,
            [
                "* OK hi\r\n",
                "<",
                "+ PDE4OTYuNjk3MTcwOTUyQHBvc3RvZmZpY2UucmVzdG9uLm1jaS5uZXQ+\r\n",
                "<",
                "* CAPABILITY IMAP4rev1 IDLE\r\nJEN OK authenticated\r\n"
            ]);
    };
    def port as int init convert.toInt(strings.split(net.address($listener), ":")[1]);
    def s as Session init connect(Options{
        host: "127.0.0.1",
        port: $port,
        security: transport.Security.None,
        user: "me",
        pass: "secret",
        auth: "cram",
        allowInsecureAuth: false
    });
    net.close($s.conn);
    testing.assertEqual(task.wait($server)[0], "JEN AUTHENTICATE CRAM-MD5");
    net.close($listener);
}

# A peer closing mid-SASL is kind "closed" (it was a runtime slice error).
func cramOnClosed() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of list of string init spawn {
        return serveScript($listener, ["* OK hi\r\n", "<", "!close"]);
    };
    def port as int init convert.toInt(strings.split(net.address($listener), ":")[1]);
    try {
        connect(Options{
            host: "127.0.0.1",
            port: $port,
            security: transport.Security.None,
            user: "me",
            pass: "secret",
            auth: "cram",
            allowInsecureAuth: true
        });
    } catch (e) {
        task.wait($server);
        net.close($listener);
        throw $e;
    }
}
func testSaslClosedPeerIsClosed() {
    testing.assertThrows("cramOnClosed", "closed");
}

# Over Security.None, LOGIN / XOAUTH2 are not sent unless allowInsecureAuth:
# the server sees no command at all.
func plainAuth(mech as string) {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of list of string init spawn {
        return serveScript($listener, ["* OK hi\r\n"]);
    };
    def port as int init convert.toInt(strings.split(net.address($listener), ":")[1]);
    try {
        connect(Options{
            host: "127.0.0.1",
            port: $port,
            security: transport.Security.None,
            user: "me",
            pass: "secret",
            auth: $mech,
            allowInsecureAuth: false
        });
    } catch (e) {
        testing.assertEqual(len(task.wait($server)), 0);
        net.close($listener);
        throw $e;
    }
}
func plainLogin() {
    plainAuth("");
}
func plainXoauth2() {
    plainAuth("xoauth2");
}
func testPlaintextAuthRefused() {
    testing.assertThrows("plainLogin", "imap");
    testing.assertThrows("plainXoauth2", "imap");
}

# Bytes behind the STARTTLS reply are refused (they would be read as if they
# had come over TLS).
func starttlsInjection() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of list of string init spawn {
        return serveScript($listener, ["* OK hi\r\n", "<", "JEN OK begin TLS\r\n* injected\r\n"]);
    };
    def port as int init convert.toInt(strings.split(net.address($listener), ":")[1]);
    try {
        connect(Options{
            host: "127.0.0.1",
            port: $port,
            security: transport.Security.Starttls,
            user: "me",
            pass: "secret",
            auth: "",
            allowInsecureAuth: false
        });
    } catch (e) {
        task.wait($server);
        net.close($listener);
        throw $e;
    }
}
func testStarttlsInjectionRefused() {
    testing.assertThrows("starttlsInjection", "imap");
}

# APPEND skips any number of untagged lines before "+", and a tagged OK before
# the literal is an error (it was read as success).
func testAppendSkipsUntagged() {
    def script as list of string init ["<"];
    for (def i in 0..40) {
        $script[] = "* {$i} EXISTS\r\n";
    }
    $script[] = "+ go ahead\r\n";
    $script[] = "<";
    $script[] = "JEN OK APPEND completed\r\n";
    def d as Dialogue init dialogue($script);
    append($d.session, "Sent", "hi");
    def got as list of string init finish($d);
    testing.assertEqual($got[2], "hi");
}
func appendOkEarly() {
    def d as Dialogue init dialogue(["<", "JEN OK APPEND completed\r\n"]);
    try {
        append($d.session, "Sent", "hi");
    } catch (e) {
        finish($d);
        throw $e;
    }
}
func testAppendOkBeforeLiteralIsError() {
    testing.assertThrows("appendOkEarly", "imap");
}
