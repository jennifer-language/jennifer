# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# mikrotik_test.j - white-box tests for mikrotik.j. Run with:
#
#     jennifer test modules/mikrotik_test.j
#
# These exercise the pure RouterOS word length codec, sentence field parsing,
# and command building with no network; the live login + talk exchange is driven
# against a fake API server in the Go suite (cmd/jennifer/mikrotik_test.go).
# mikrotik.j already `use`s net / strings / convert / lists / maps / hash /
# encoding, so the overlay only adds testing.
use testing;

func hex(b as bytes) {
    return encoding.toText($b, "hex");
}

func testEncodeLenForms() {
    testing.assertEqual(hex(encodeLen(5)), "05");
    testing.assertEqual(hex(encodeLen(0)), "00");
    testing.assertEqual(hex(encodeLen(127)), "7f");
    testing.assertEqual(hex(encodeLen(128)), "8080");
    testing.assertEqual(hex(encodeLen(16383)), "bfff");
    testing.assertEqual(hex(encodeLen(16384)), "c04000");
    testing.assertEqual(hex(encodeLen(2097152)), "e0200000");
}

func testLenPrefixSize() {
    testing.assertEqual(lenPrefixSize(0x05), 1);
    testing.assertEqual(lenPrefixSize(0x80), 2);
    testing.assertEqual(lenPrefixSize(0xc0), 3);
    testing.assertEqual(lenPrefixSize(0xe0), 4);
    testing.assertEqual(lenPrefixSize(0xf0), 5);
}

func testDecodeLenRoundTrip() {
    def cases as list of int init [0, 5, 127, 128, 500, 16383, 16384, 2097152, 300000];
    for (def n in $cases) {
        testing.assertEqual(decodeLen(encodeLen($n), 0), $n);
    }
}

func testDecodeLenAtOffset() {
    # a 0xaa filler byte then encodeLen(128) = 8080
    def buf as bytes init encoding.fromText("aa8080", "hex");
    testing.assertEqual(decodeLen($buf, 1), 128);
}

func testParseFields() {
    def sentence as list of string init ["!re", "=name=ether1", "=type=ether", "=running=true"];
    def f as map of string to string init parseFields($sentence);
    testing.assertEqual($f["name"], "ether1");
    testing.assertEqual($f["type"], "ether");
    testing.assertEqual($f["running"], "true");
}

func testParseFieldsValueWithEquals() {
    # a value containing "=" keeps everything after the first "=" (comment=a=b)
    def f as map of string to string init parseFields(["!re", "=comment=a=b=c"]);
    testing.assertEqual($f["comment"], "a=b=c");
}

func testBuildWords() {
    def none as map of string to string init {};
    def noq as list of string init [];
    def w as list of string init buildWords("/interface/print", $none, $noq);
    testing.assertEqual(len($w), 1);
    testing.assertEqual($w[0], "/interface/print");

    def attrs as map of string to string init {};
    $attrs["address"] = "1.2.3.4/24";
    def withAttr as list of string init buildWords("/ip/address/add", $attrs, $noq);
    testing.assertEqual(len($withAttr), 2);
    testing.assertEqual($withAttr[1], "=address=1.2.3.4/24");
}

func testBuildWordsQueries() {
    # Query words are appended verbatim, after the command (and after any attrs).
    def none as map of string to string init {};
    def q as list of string init ["?type=ether", "?disabled"];
    def w as list of string init buildWords("/interface/print", $none, $q);
    testing.assertEqual(len($w), 3);
    testing.assertEqual($w[0], "/interface/print");
    testing.assertEqual($w[1], "?type=ether");
    testing.assertEqual($w[2], "?disabled");

    # Attribute words come before query words.
    def attrs as map of string to string init {};
    $attrs["numbers"] = "0";
    def wq as list of string init buildWords("/interface/print", $attrs, ["?type=ether"]);
    testing.assertEqual(len($wq), 3);
    testing.assertEqual($wq[1], "=numbers=0");
    testing.assertEqual($wq[2], "?type=ether");
}

# badQueryWord feeds buildWords a query word without its leading "?" - it must throw.
func badQueryWord() {
    def none as map of string to string init {};
    def bad as list of string init ["type=ether"];
    buildWords("/interface/print", $none, $bad);
}

func testBuildWordsRejectsBadQuery() {
    testing.assertThrows("badQueryWord", "mikrotik");
}

func testChallengeResponseShape() {
    # "00" + 32 hex chars of the MD5 digest = 34 characters, all lowercase hex.
    def r as string init challengeResponse("secret", "abcdef0123456789abcdef0123456789");
    testing.assertEqual(len($r), 34);
    testing.assertTrue(strings.startsWith($r, "00"));
}

# OM-004: a server-declared word length beyond the cap is rejected before it
# sizes a read.
func overWordCap() {
    checkWordLen(MAX_WORD_BYTES + 1);
}
func negWordCap() {
    checkWordLen(-1);
}
func testWordLenCap() {
    testing.assertThrows("overWordCap", "mikrotik");
    testing.assertThrows("negWordCap", "mikrotik");
    checkWordLen(MAX_WORD_BYTES); # exactly at the limit does not throw
    checkWordLen(1);
}

# --- tagging + streaming (pure helpers) -------------------------------------

# A tagged command sentence carries a trailing `.tag=<id>` API word after the
# command (and any attribute words), so replies can be correlated.
func testBuildTaggedWords() {
    def none as map of string to string init {};
    def noq as list of string init [];
    def w as list of string init buildTaggedWords("/interface/monitor", $none, $noq, "7");
    testing.assertEqual(len($w), 2);
    testing.assertEqual($w[0], "/interface/monitor");
    testing.assertEqual($w[1], ".tag=7");

    # The tag word follows the attribute words.
    def attrs as map of string to string init {};
    $attrs["interface"] = "ether1";
    def wa as list of string init buildTaggedWords("/interface/monitor", $attrs, $noq, "abc");
    testing.assertEqual(len($wa), 3);
    testing.assertEqual($wa[0], "/interface/monitor");
    testing.assertEqual($wa[1], "=interface=ether1");
    testing.assertEqual($wa[2], ".tag=abc");
}

# parseTag reads the `.tag=` word back off a reply sentence; "" when absent.
func testParseTag() {
    def tagged as list of string init ["!re", ".tag=42", "=name=ether1", "=type=ether"];
    testing.assertEqual(parseTag($tagged), "42");

    def untagged as list of string init ["!re", "=name=ether1"];
    testing.assertEqual(parseTag($untagged), "");

    # `.tag` is distinct from the `=key=value` attribute words: parseFields does
    # not pick it up, and parseTag does not pick up a `=tag=` attribute.
    def f as map of string to string init parseFields($tagged);
    testing.assertTrue(not maps.has($f, "tag"));
    testing.assertEqual(parseTag(["!re", "=tag=99"]), "");
}

# buildCancelWords builds the `/cancel` sentence naming the tag to stop.
func testBuildCancelWords() {
    def w as list of string init buildCancelWords("42");
    testing.assertEqual(len($w), 2);
    testing.assertEqual($w[0], "/cancel");
    testing.assertEqual($w[1], "=tag=42");
}

# A tag round-trips: encode it into a sentence, read it back off the reply.
func testTagRoundTrip() {
    def none as map of string to string init {};
    def noq as list of string init [];
    def sent as list of string init buildTaggedWords("/ping", $none, $noq, "xyz");
    # The router echoes the tag on its reply; parseTag recovers it.
    def reply as list of string init ["!re", ".tag=xyz", "=host=1.2.3.4"];
    testing.assertEqual(parseTag($reply), parseTag($sent));
    testing.assertEqual(parseTag($sent), "xyz");
}

# --- connection options ------------------------------------------------------

func testOptionsPlainDefaults() {
    def o as Options init options("192.168.88.1", "admin", "secret");
    testing.assertEqual($o.host, "192.168.88.1");
    testing.assertEqual($o.user, "admin");
    testing.assertEqual($o.port, 8728);
    testing.assertFalse($o.tls);
}

# api-ssl moves the port and turns TLS on. `connect` skips certificate
# verification on that path (a router is dialled by IP, which no certificate's
# SAN covers); the Go suite proves that on a live socket - see
# TestMikrotikTLSSkipsVerification in cmd/jennifer/mikrotik_test.go.
func testOptionsTLSDefaults() {
    def o as Options init optionsTLS("192.168.88.1", "admin", "secret");
    testing.assertEqual($o.port, 8729);
    testing.assertTrue($o.tls);
}

func testWithPortKeepsTransport() {
    def secure as Options init withPort(optionsTLS("10.0.0.1", "admin", "pw"), 18729);
    testing.assertEqual($secure.port, 18729);
    testing.assertTrue($secure.tls);

    def plain as Options init withPort(options("10.0.0.1", "admin", "pw"), 18728);
    testing.assertEqual($plain.port, 18728);
    testing.assertFalse($plain.tls);
}
