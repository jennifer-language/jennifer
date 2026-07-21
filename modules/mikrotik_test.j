# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
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
    def w as list of string init buildWords("/interface/print", $none);
    testing.assertEqual(len($w), 1);
    testing.assertEqual($w[0], "/interface/print");

    def attrs as map of string to string init {};
    $attrs["address"] = "1.2.3.4/24";
    def withAttr as list of string init buildWords("/ip/address/add", $attrs);
    testing.assertEqual(len($withAttr), 2);
    testing.assertEqual($withAttr[1], "=address=1.2.3.4/24");
}

func testChallengeResponseShape() {
    # "00" + 32 hex chars of the MD5 digest = 34 characters, all lowercase hex.
    def r as string init challengeResponse("secret", "abcdef0123456789abcdef0123456789");
    testing.assertEqual(len($r), 34);
    testing.assertTrue(strings.startsWith($r, "00"));
}
