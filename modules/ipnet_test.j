# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# ipnet_test.j - white-box tests for ipnet.j. Run with:
#
#     jennifer test modules/ipnet_test.j
#
# The overlay splices ipnet.j in front, so the tests reach its private helpers
# (parseFour, parseSix, hexGroup, applyMask, bytesEqual) by bare identifier as
# well as the exported surface. ipnet.j already `use`s strings / convert, so the
# overlay only adds testing.
use testing;

# roundTrip: parse an address and render it back to canonical form.
func canon(s as string) {
    return toString(parseAddress($s));
}

# --- IPv4 -------------------------------------------------------------------

func testParseFour() {
    def a as Address init parseAddress("192.168.1.1");
    testing.assertEqual($a.version, 4);
    testing.assertEqual(len($a.octets), 4);
    testing.assertEqual($a.octets[0], 192);
    testing.assertEqual($a.octets[3], 1);
    testing.assertEqual(toString($a), "192.168.1.1");
}

func testParseFourBounds() {
    testing.assertEqual(canon("0.0.0.0"), "0.0.0.0");
    testing.assertEqual(canon("255.255.255.255"), "255.255.255.255");
}

# --- IPv6 canonical formatting (RFC 5952) -----------------------------------

func testParseSixCanonical() {
    testing.assertEqual(canon("2001:0db8:0000:0000:0000:0000:0000:0001"), "2001:db8::1");
    testing.assertEqual(canon("2001:db8::1"), "2001:db8::1");
    testing.assertEqual(canon("::1"), "::1");
    testing.assertEqual(canon("::"), "::");
    testing.assertEqual(canon("fe80::1ff:fe23:4567:890a"), "fe80::1ff:fe23:4567:890a");
}

func testParseSixLeftmostLongestRun() {
    # Two equal-length zero runs -> compress the leftmost.
    testing.assertEqual(canon("2001:db8:0:0:1:0:0:1"), "2001:db8::1:0:0:1");
    # A single zero group is not compressed (needs a run of >= 2).
    testing.assertEqual(canon("2001:db8:0:1:1:1:1:1"), "2001:db8:0:1:1:1:1:1");
}

func testParseSixEmbeddedFour() {
    def a as Address init parseAddress("::ffff:192.168.1.1");
    testing.assertEqual($a.version, 6);
    testing.assertEqual(len($a.octets), 16);
    # last four bytes are the embedded IPv4
    testing.assertEqual($a.octets[12], 192);
    testing.assertEqual($a.octets[15], 1);
    testing.assertEqual($a.octets[10], 255);
    testing.assertEqual($a.octets[11], 255);
}

func testParseSixExpandsToSixteenBytes() {
    def a as Address init parseAddress("2001:db8::");
    testing.assertEqual(len($a.octets), 16);
    testing.assertEqual($a.octets[0], 32);   # 0x20
    testing.assertEqual($a.octets[1], 1);    # 0x01
    testing.assertEqual($a.octets[2], 13);   # 0x0d
    testing.assertEqual($a.octets[3], 184);  # 0xb8
    testing.assertEqual($a.octets[4], 0);
}

# --- CIDR -------------------------------------------------------------------

func testParseNetworkZeroesHostBits() {
    def n as Network init parse("192.168.1.42/24");
    testing.assertEqual($n.prefix, 24);
    testing.assertEqual(networkString($n), "192.168.1.0/24");   # .42 host bits dropped
}

func testNetmaskFour() {
    testing.assertEqual(toString(netmask(parse("10.0.0.0/8"))), "255.0.0.0");
    testing.assertEqual(toString(netmask(parse("192.168.1.0/24"))), "255.255.255.0");
    testing.assertEqual(toString(netmask(parse("203.0.113.128/26"))), "255.255.255.192");
    testing.assertEqual(toString(netmask(parse("1.2.3.4/32"))), "255.255.255.255");
}

func testBroadcastFour() {
    testing.assertEqual(toString(broadcast(parse("192.168.1.0/24"))), "192.168.1.255");
    testing.assertEqual(toString(broadcast(parse("10.0.0.0/8"))), "10.255.255.255");
    testing.assertEqual(toString(broadcast(parse("203.0.113.128/26"))), "203.0.113.191");
}

func testNetmaskSix() {
    testing.assertEqual(toString(netmask(parse("2001:db8::/32"))), "ffff:ffff::");
    testing.assertEqual(toString(netmask(parse("2001:db8:abcd:1200::/56"))), "ffff:ffff:ffff:ff00::");
}

func testBroadcastSix() {
    testing.assertEqual(toString(broadcast(parse("2001:db8::/32"))),
        "2001:db8:ffff:ffff:ffff:ffff:ffff:ffff");
}

# --- membership -------------------------------------------------------------

func testContainsFour() {
    def n as Network init parse("192.168.1.0/24");
    testing.assertTrue(contains($n, parseAddress("192.168.1.42")));
    testing.assertTrue(contains($n, parseAddress("192.168.1.0")));
    testing.assertTrue(contains($n, parseAddress("192.168.1.255")));
    testing.assertFalse(contains($n, parseAddress("192.168.2.1")));
}

func testContainsBoundary() {
    def n as Network init parse("203.0.113.128/26");
    testing.assertTrue(contains($n, parseAddress("203.0.113.128")));
    testing.assertTrue(contains($n, parseAddress("203.0.113.191")));
    testing.assertFalse(contains($n, parseAddress("203.0.113.192")));
    testing.assertFalse(contains($n, parseAddress("203.0.113.127")));
}

func testContainsSix() {
    def n as Network init parse("2001:db8::/32");
    testing.assertTrue(contains($n, parseAddress("2001:db8:abcd::1")));
    testing.assertFalse(contains($n, parseAddress("2001:db9::1")));
}

func testContainsCrossVersionFalse() {
    testing.assertFalse(contains(parse("192.168.1.0/24"), parseAddress("2001:db8::1")));
    testing.assertFalse(contains(parse("2001:db8::/32"), parseAddress("192.168.1.1")));
}

func testEqualAndVersion() {
    testing.assertTrue(equal(parseAddress("10.0.0.1"), parseAddress("10.0.0.1")));
    testing.assertFalse(equal(parseAddress("10.0.0.1"), parseAddress("10.0.0.2")));
    # same bits, different notation still equal
    testing.assertTrue(equal(parseAddress("2001:db8::1"), parseAddress("2001:0db8:0:0:0:0:0:1")));
    testing.assertFalse(equal(parseAddress("10.0.0.1"), parseAddress("::1")));   # version differs
    testing.assertEqual(version(parseAddress("10.0.0.1")), 4);
    testing.assertEqual(version(parseAddress("::1")), 6);
}

# --- private helpers --------------------------------------------------------

func testHexGroup() {
    testing.assertEqual(hexGroup(0), "0");
    testing.assertEqual(hexGroup(1), "1");
    testing.assertEqual(hexGroup(255), "ff");
    testing.assertEqual(hexGroup(0xdb8), "db8");
    testing.assertEqual(hexGroup(0xffff), "ffff");
}

func testApplyMask() {
    def masked as bytes init applyMask(parseAddress("192.168.1.200").octets, 24);
    testing.assertEqual($masked[3], 0);
    testing.assertEqual($masked[2], 1);
    testing.assertTrue(bytesEqual($masked, parseAddress("192.168.1.0").octets));
}

# --- errors -----------------------------------------------------------------

func caughtIpnet(bad as string) {
    def threw as bool init false;
    try {
        parseAddress($bad);
    } catch (e) {
        $threw = true;
    }
    return $threw;
}

func testParseErrors() {
    testing.assertTrue(caughtIpnet("999.1.1.1"));       # octet out of range
    testing.assertTrue(caughtIpnet("1.2.3"));           # too few octets
    testing.assertTrue(caughtIpnet("2001:db8::1::2"));  # multiple ::
    testing.assertTrue(caughtIpnet("2001:db8:zz::1"));  # bad hex
    testing.assertTrue(caughtIpnet("hello"));           # not an IP
}

func testPrefixOutOfRangeThrows() {
    def threw as bool init false;
    try {
        parse("10.0.0.0/40");
    } catch (e) {
        $threw = true;
    }
    testing.assertTrue($threw);
}
