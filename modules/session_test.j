# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# session_test.j - white-box tests for session.j's pure helpers. Run with:
#
#     jennifer test modules/session_test.j
#
# The overlay splices session.j in front of this file, so the tests reach its
# private key builder and base64+JSON encode / decode by bare identifier. The
# networked lifecycle (create / load / save / touch / destroy) is verified over
# each backend in the Go suite (TestSessionLifecycle).
use testing;
use json;
use strings;
use convert;

func testStoreKey() {
    testing.assertEqual(storeKey("abc"), "sess:abc");
}

func testRoundTrip() {
    def src as json.Value init json.map();
    $src = json.set($src, "/user", "ada");
    $src = json.set($src, "/n", 42);
    def back as json.Value init decodeData(encodeData($src));
    testing.assertEqual(json.asString($back, "/user"), "ada");
    testing.assertEqual(json.asInt($back, "/n"), 42);
}

func testRoundTripUnicode() {
    # base64-wrapping is what makes a non-ASCII value survive the store round-trip.
    def src as json.Value init json.set(json.map(), "/name", "José");
    def back as json.Value init decodeData(encodeData($src));
    testing.assertEqual(json.asString($back, "/name"), "José");
}

func testEncodedBlobIsAscii() {
    # the stored blob (base64) has equal byte and rune length: pure ASCII.
    def src as json.Value init json.set(json.map(), "/name", "José");
    def blob as string init encodeData($src);
    testing.assertEqual(len(convert.bytesFromString($blob, "utf-8")), len($blob));
}

func testDecodeEmptyBlob() {
    # an absent / expired session reads back as an empty object.
    testing.assertEqual(json.length(decodeData(""), ""), 0);
}

func testRoundTripEmptyObject() {
    testing.assertEqual(json.length(decodeData(encodeData(json.map())), ""), 0);
}

# A session id arrives from a client cookie; one carrying a space, CRLF, or any
# character outside [A-Za-z0-9-] must be rejected before it is embedded in a
# store key (otherwise it injects protocol commands).
func idWithSpace() {
    storeKey("abc def");
}
func idWithCRLF() {
    storeKey("abc\r\nset injected 0 0 3\r\nx");
}
func idWithColon() {
    storeKey("a:b");
}
func idEmpty() {
    storeKey("");
}
func idTooLong() {
    storeKey(strings.repeat("a", 251));
}

func testRejectsUnsafeIds() {
    testing.assertThrows("idWithSpace", "session");
    testing.assertThrows("idWithCRLF", "session");
    testing.assertThrows("idWithColon", "session");
    testing.assertThrows("idEmpty", "session");
    testing.assertThrows("idTooLong", "session");
}

func testAcceptsSafeIds() {
    # A UUID v4 (the minted shape) and a plain alnum-dash id are accepted.
    testing.assertEqual(storeKey("a1B2-c3D4"), "sess:a1B2-c3D4");
}

# The full lifecycle over the in-process backend (no server): create / load /
# save structured json.Value / touch / destroy.
func testLifecycleInProcess() {
    def st as kvstore.Store init kvstore.inProcessStore();
    def id as string init create($st, 60);
    testing.assertEqual(json.length(load($st, $id), ""), 0); # a fresh session is empty
    def d as json.Value init load($st, $id);
    $d = json.set($d, "/user", "ada");
    $d = json.set($d, "/prefs", json.map());
    $d = json.set($d, "/prefs/theme", "dark");
    save($st, $id, $d, 60);
    def back as json.Value init load($st, $id);
    testing.assertEqual(json.asString($back, "/user"), "ada");
    testing.assertEqual(json.asString($back, "/prefs/theme"), "dark"); # nested, richer than a flat map
    testing.assertTrue(touch($st, $id, 120));
    testing.assertFalse(touch($st, "no-such-session", 60));
    testing.assertTrue(destroy($st, $id));
    testing.assertFalse(destroy($st, $id));
    testing.assertEqual(json.length(load($st, $id), ""), 0); # gone
}
