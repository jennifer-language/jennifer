# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# imap_test.j - white-box tests for imap.j's pure protocol helpers. Run with:
#
#     jennifer test modules/imap_test.j
#
# The overlay splices imap.j in front of this file, so the tests reach its
# private helpers (literalLength, extractLiteral, parseExists, parseSearch,
# isTagged, quoteArg, expectTaggedOK) by bare identifier. The
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
    expectTaggedOK("JEN OK completed", "JEN"); # no throw
    testing.assertTrue(true);
}

# ---- read cap (DoS from an oversized literal / unterminated response) ----
func testResponseCapRejectsOversized() {
    testing.assertThrows("overRespCap", "imap");
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
    command($c, "STORE 1 +FLAGS.SILENT (x)\r\nA1 LOGOUT\r\n(y)");
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
    def mb as Folder init parseListLine("* LIST (\\HasNoChildren \\Marked) \"/\" \"INBOX\"");
    testing.assertEqual($mb.name, "INBOX");
    testing.assertEqual($mb.delimiter, "/");
    testing.assertEqual(len($mb.flags), 2);
    testing.assertEqual($mb.flags[0], "\\HasNoChildren");
    # A \Noselect parent with a NIL delimiter and an atom name.
    def nil as Folder init parseListLine("* LIST (\\Noselect) NIL Archive");
    testing.assertEqual($nil.name, "Archive");
    testing.assertEqual($nil.delimiter, "");
    testing.assertEqual($nil.flags[0], "\\Noselect");
}

func testParseList() {
    def resp as string init "* LIST (\\HasNoChildren) \"/\" \"INBOX\"\r\n* LIST (\\HasChildren) \"/\" \"Archive\"\r\n* LIST (\\HasNoChildren) \"/\" \"Archive/2024\"\r\nJEN OK LIST completed\r\n";
    def boxes as list of Folder init parseList($resp);
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

# extractLiteral: the {N} is a BYTE count, so a multi-byte body must come back
# byte-exact, not over-read by (bytes - runes) into the trailing `)`/CRLF/tag.
# "HÉLLO" is 6 bytes / 5 runes -> marker {6}. (assertEqual, not assertContains.)
func testExtractLiteralMultibyte() {
    testing.assertEqual(extractLiteral("* 1 FETCH (BODY[] {6}\r\nHÉLLO)\r\nJEN OK\r\n"), "HÉLLO");
    # An ASCII body still round-trips exactly.
    testing.assertEqual(extractLiteral("* 1 FETCH (BODY[] {5}\r\nplain)\r\nJEN OK\r\n"), "plain");
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
