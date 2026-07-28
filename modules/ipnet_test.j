# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# ipnet_test.j - white-box tests for ipnet.j. Run with:
#
#     jennifer test modules/ipnet_test.j
#
# The overlay splices ipnet.j in front, so the tests reach its private helpers
# (parse4, parse6, hexGroup, applyMask, bytesEqual) by bare identifier as
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

# parse6 keeps the full 16-byte v4-mapped form (parseAddress folds it to v4 -
# see testUnmapV4Mapped - so this exercises the private parser directly).
func testParseSixEmbeddedFour() {
    def octets as bytes init parse6("::ffff:192.168.1.1");
    testing.assertEqual(len($octets), 16);
    # last four bytes are the embedded IPv4
    testing.assertEqual($octets[12], 192);
    testing.assertEqual($octets[15], 1);
    testing.assertEqual($octets[10], 255);
    testing.assertEqual($octets[11], 255);
}

func testParseSixExpandsToSixteenBytes() {
    def a as Address init parseAddress("2001:db8::");
    testing.assertEqual(len($a.octets), 16);
    testing.assertEqual($a.octets[0], 32); # 0x20
    testing.assertEqual($a.octets[1], 1); # 0x01
    testing.assertEqual($a.octets[2], 13); # 0x0d
    testing.assertEqual($a.octets[3], 184); # 0xb8
    testing.assertEqual($a.octets[4], 0);
}

# --- CIDR -------------------------------------------------------------------

func testParseNetworkZeroesHostBits() {
    def n as Network init parse("192.168.1.42/24");
    testing.assertEqual($n.prefix, 24);
    testing.assertEqual(networkString($n), "192.168.1.0/24"); # .42 host bits dropped
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
    testing.assertEqual(
        toString(netmask(parse("2001:db8:abcd:1200::/56"))),
        "ffff:ffff:ffff:ff00::");
}

func testBroadcastSix() {
    testing.assertEqual(
        toString(broadcast(parse("2001:db8::/32"))),
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
    testing.assertFalse(equal(parseAddress("10.0.0.1"), parseAddress("::1"))); # version differs
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
    testing.assertTrue(caughtIpnet("999.1.1.1")); # octet out of range
    testing.assertTrue(caughtIpnet("1.2.3")); # too few octets
    testing.assertTrue(caughtIpnet("2001:db8::1::2")); # multiple ::
    testing.assertTrue(caughtIpnet("2001:db8:zz::1")); # bad hex
    testing.assertTrue(caughtIpnet("hello")); # not an IP
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

# --- OM-010: IPv4-mapped IPv6 is folded to a v4 address ----------------------

func testUnmapV4Mapped() {
    # ::ffff:a.b.c.d parses straight to a v4 Address, so a v4 allow-list matches.
    def a as Address init parseAddress("::ffff:127.0.0.1");
    testing.assertEqual($a.version, 4);
    testing.assertEqual(len($a.octets), 4);
    testing.assertTrue(contains(parse("127.0.0.0/8"), $a));
    # unmap is idempotent on a plain v4 address and leaves real v6 alone.
    def v4 as Address init parseAddress("10.0.0.1");
    testing.assertEqual(unmap($v4).version, 4);
    # ::1 (loopback) and a normal v6 address must NOT be folded.
    testing.assertEqual(parseAddress("::1").version, 6);
    testing.assertEqual(parseAddress("2001:db8::1").version, 6);
    # The deprecated IPv4-compatible form is deliberately left as v6.
    testing.assertEqual(parseAddress("::1.2.3.4").version, 6);
}

# --- IPv4-mapped CIDR (regression) ------------------------------------------

# A v4-mapped IPv6 block with prefix >= 96 is accepted (prefix taken from the
# literal, max 128) and folded to its v4 equivalent, translating the prefix by
# the 96-bit offset. A shorter prefix stays a genuine v6 network.
func testMappedCidrFolds() {
    testing.assertEqual(networkString(parse("::ffff:0:0/96")), "0.0.0.0/0");
    testing.assertEqual(networkString(parse("::ffff:192.168.1.0/120")), "192.168.1.0/24");
    testing.assertEqual(networkString(parse("::ffff:10.0.0.0/104")), "10.0.0.0/8");
    def folded as Network init parse("::ffff:192.168.1.0/120");
    testing.assertEqual($folded.addr.version, 4);
    testing.assertEqual($folded.prefix, 24);
    # A v4-mapped address falls inside the folded v4 block.
    testing.assertTrue(contains(parse("::ffff:192.168.1.0/120"), parseAddress("192.168.1.5")));
}

func testMappedCidrShortPrefixStaysSix() {
    # prefix < 96 spans beyond ::ffff:0:0/96, so it stays a v6 network.
    def n as Network init parse("::ffff:0:0/64");
    testing.assertEqual($n.addr.version, 6);
    testing.assertEqual($n.prefix, 64);
    testing.assertEqual(networkString($n), "::/64");
}

# --- ordering / iteration ---------------------------------------------------

func testNextPrevCompare() {
    testing.assertEqual(toString(next(parseAddress("1.1.1.255"))), "1.1.2.0");
    testing.assertEqual(toString(prev(parseAddress("1.1.2.0"))), "1.1.1.255");
    testing.assertEqual(toString(next(parseAddress("2001:db8::ffff"))), "2001:db8::1:0");
    testing.assertEqual(compare(parseAddress("10.0.0.1"), parseAddress("10.0.0.2")), -1);
    testing.assertEqual(compare(parseAddress("10.0.0.2"), parseAddress("10.0.0.1")), 1);
    testing.assertEqual(compare(parseAddress("10.0.0.1"), parseAddress("10.0.0.1")), 0);
    # cross-version: v4 orders before v6
    testing.assertEqual(compare(parseAddress("255.255.255.255"), parseAddress("::")), -1);
}

func caughtNext(s as string) {
    def threw as bool init false;
    try {
        next(parseAddress($s));
    } catch (e) {
        $threw = true;
    }
    return $threw;
}

func testNextPrevOverflow() {
    testing.assertTrue(caughtNext("255.255.255.255")); # no successor
    def threw as bool init false;
    try {
        prev(parseAddress("0.0.0.0"));
    } catch (e) {
        $threw = true;
    }
    testing.assertTrue($threw); # no predecessor
}

# --- subnet math ------------------------------------------------------------

func testHostCount() {
    testing.assertEqual(hostCount(parse("192.168.1.0/24")), 256);
    testing.assertEqual(hostCount(parse("10.0.0.0/30")), 4);
    testing.assertEqual(hostCount(parse("1.2.3.4/32")), 1);
    testing.assertEqual(hostCount(parse("0.0.0.0/0")), 4294967296); # 2^32
    testing.assertEqual(hostCount(parse("2001:db8::/126")), 4);
}

func caughtHostCount(cidr as string) {
    def threw as bool init false;
    try {
        hostCount(parse($cidr));
    } catch (e) {
        $threw = true;
    }
    return $threw;
}

func testHostCountTooLarge() {
    testing.assertTrue(caughtHostCount("2001:db8::/60")); # 2^68 overflows int
}

func testFirstLastUsable() {
    def n as Network init parse("192.168.1.0/24");
    testing.assertEqual(toString(firstUsable($n)), "192.168.1.1");
    testing.assertEqual(toString(lastUsable($n)), "192.168.1.254");
    # /31 (RFC 3021): both addresses usable
    def p2p as Network init parse("10.0.0.0/31");
    testing.assertEqual(toString(firstUsable($p2p)), "10.0.0.0");
    testing.assertEqual(toString(lastUsable($p2p)), "10.0.0.1");
    # /32: single host
    def one as Network init parse("10.0.0.5/32");
    testing.assertEqual(toString(firstUsable($one)), "10.0.0.5");
    testing.assertEqual(toString(lastUsable($one)), "10.0.0.5");
    # IPv6 has no broadcast: the last address is usable
    def six as Network init parse("2001:db8::/126");
    testing.assertEqual(toString(firstUsable($six)), "2001:db8::1");
    testing.assertEqual(toString(lastUsable($six)), "2001:db8::3");
}

func testHosts() {
    def hs as list of Address init hosts(parse("192.168.1.4/30"));
    testing.assertEqual(len($hs), 2); # .5 and .6 (excludes .4 network, .7 broadcast)
    testing.assertEqual(toString($hs[0]), "192.168.1.5");
    testing.assertEqual(toString($hs[1]), "192.168.1.6");
    # /31 lists both addresses
    testing.assertEqual(len(hosts(parse("10.0.0.0/31"))), 2);
    # /32 lists the single host
    testing.assertEqual(len(hosts(parse("10.0.0.9/32"))), 1);
}

func caughtHosts(cidr as string) {
    def threw as bool init false;
    try {
        hosts(parse($cidr));
    } catch (e) {
        $threw = true;
    }
    return $threw;
}

func testHostsCapped() {
    testing.assertTrue(caughtHosts("10.0.0.0/8")); # 16M addresses -> refused
}

# --- split ------------------------------------------------------------------

func testSplit() {
    def subs as list of Network init split(parse("192.168.1.0/24"), 26);
    testing.assertEqual(len($subs), 4);
    testing.assertEqual(networkString($subs[0]), "192.168.1.0/26");
    testing.assertEqual(networkString($subs[1]), "192.168.1.64/26");
    testing.assertEqual(networkString($subs[2]), "192.168.1.128/26");
    testing.assertEqual(networkString($subs[3]), "192.168.1.192/26");
    # split across an octet boundary
    def wide as list of Network init split(parse("10.0.0.0/8"), 10);
    testing.assertEqual(len($wide), 4);
    testing.assertEqual(networkString($wide[1]), "10.64.0.0/10");
    testing.assertEqual(networkString($wide[3]), "10.192.0.0/10");
    # a split to the same prefix is a single subnet (identity)
    testing.assertEqual(len(split(parse("10.0.0.0/8"), 8)), 1);
}

func caughtSplit(cidr as string, np as int) {
    def threw as bool init false;
    try {
        split(parse($cidr), $np);
    } catch (e) {
        $threw = true;
    }
    return $threw;
}

func testSplitErrors() {
    testing.assertTrue(caughtSplit("10.0.0.0/8", 4)); # shorter than the network
    testing.assertTrue(caughtSplit("10.0.0.0/8", 40)); # beyond the v4 max
    testing.assertTrue(caughtSplit("10.0.0.0/8", 25)); # > 65536 subnets
}

# --- aggregation ------------------------------------------------------------

func testAggregateMergesSiblings() {
    def parts as list of Network init [
        parse("192.168.0.0/25"),
        parse("192.168.0.128/25")
    ];
    def agg as list of Network init aggregate($parts);
    testing.assertEqual(len($agg), 1);
    testing.assertEqual(networkString($agg[0]), "192.168.0.0/24");
}

func testAggregateDropsContained() {
    def parts as list of Network init [
        parse("10.0.0.0/8"),
        parse("10.1.0.0/16"),
        parse("10.2.3.0/24")
    ];
    def agg as list of Network init aggregate($parts);
    testing.assertEqual(len($agg), 1);
    testing.assertEqual(networkString($agg[0]), "10.0.0.0/8");
}

func testAggregateCascades() {
    # four /26s collapse all the way to one /24
    def parts as list of Network init [
        parse("192.168.1.0/26"),
        parse("192.168.1.64/26"),
        parse("192.168.1.128/26"),
        parse("192.168.1.192/26")
    ];
    def agg as list of Network init aggregate($parts);
    testing.assertEqual(len($agg), 1);
    testing.assertEqual(networkString($agg[0]), "192.168.1.0/24");
}

func testAggregateKeepsVersionsSeparate() {
    def parts as list of Network init [
        parse("192.168.0.0/25"),
        parse("192.168.0.128/25"),
        parse("2001:db8::/33"),
        parse("2001:db8:8000::/33")
    ];
    def agg as list of Network init aggregate($parts);
    testing.assertEqual(len($agg), 2);
    testing.assertEqual(networkString($agg[0]), "192.168.0.0/24");
    testing.assertEqual(networkString($agg[1]), "2001:db8::/32");
}

func testAggregateNonMergeable() {
    # not siblings (different parents) -> stay separate
    def parts as list of Network init [
        parse("192.168.0.0/24"),
        parse("192.168.2.0/24")
    ];
    testing.assertEqual(len(aggregate($parts)), 2);
}

# --- overlap / subnet -------------------------------------------------------

func testOverlaps() {
    testing.assertTrue(overlaps(parse("10.0.0.0/8"), parse("10.1.0.0/16")));
    testing.assertTrue(overlaps(parse("10.1.0.0/16"), parse("10.0.0.0/8")));
    testing.assertFalse(overlaps(parse("10.0.0.0/8"), parse("11.0.0.0/8")));
    testing.assertTrue(overlaps(parse("10.0.0.0/8"), parse("10.0.0.0/8"))); # equal
    testing.assertFalse(overlaps(parse("10.0.0.0/8"), parse("2001:db8::/32"))); # cross-version
}

func testSubnetOf() {
    testing.assertTrue(subnetOf(parse("10.1.0.0/16"), parse("10.0.0.0/8")));
    testing.assertTrue(subnetOf(parse("10.0.0.0/8"), parse("10.0.0.0/8"))); # equal is a subnet
    testing.assertFalse(subnetOf(parse("10.0.0.0/8"), parse("10.1.0.0/16"))); # parent is not a subnet of child
    testing.assertFalse(subnetOf(parse("11.0.0.0/8"), parse("10.0.0.0/8")));
    testing.assertFalse(subnetOf(parse("10.1.0.0/16"), parse("2001:db8::/32"))); # cross-version
}

# --- classification ---------------------------------------------------------

func testClassifyFour() {
    testing.assertTrue(isPrivate(parseAddress("10.0.0.1")));
    testing.assertTrue(isPrivate(parseAddress("172.16.5.5")));
    testing.assertTrue(isPrivate(parseAddress("192.168.1.1")));
    testing.assertFalse(isPrivate(parseAddress("172.32.0.1"))); # outside 172.16/12
    testing.assertTrue(isLoopback(parseAddress("127.0.0.1")));
    testing.assertTrue(isLinkLocal(parseAddress("169.254.1.1")));
    testing.assertTrue(isMulticast(parseAddress("224.0.0.1")));
    testing.assertTrue(isUnspecified(parseAddress("0.0.0.0")));
    testing.assertTrue(isGlobal(parseAddress("8.8.8.8")));
    testing.assertFalse(isGlobal(parseAddress("10.0.0.1")));
    testing.assertFalse(isGlobal(parseAddress("192.0.2.5"))); # documentation -> reserved
    testing.assertFalse(isGlobal(parseAddress("100.64.0.1"))); # CGNAT -> reserved
}

func testClassifySix() {
    testing.assertTrue(isLoopback(parseAddress("::1")));
    testing.assertTrue(isLinkLocal(parseAddress("fe80::1")));
    testing.assertTrue(isMulticast(parseAddress("ff02::1")));
    testing.assertTrue(isPrivate(parseAddress("fd00::1"))); # ULA fc00::/7
    testing.assertTrue(isUnspecified(parseAddress("::")));
    testing.assertTrue(isGlobal(parseAddress("2606:4700::1111")));
    testing.assertFalse(isGlobal(parseAddress("2001:db8::1"))); # documentation -> reserved
}

func testClassifyMappedFolds() {
    # a v4-mapped v6 address classifies as its v4 self
    testing.assertTrue(isPrivate(parseAddress("::ffff:10.0.0.1")));
    testing.assertTrue(isLoopback(parseAddress("::ffff:127.0.0.1")));
}

# scope() drives every predicate; it is a total classification, so a caller can
# match every arm. Exercised here over a method result (runtime pattern match).
func scopeName(a as Address) {
    match (scope($a)) {
        when Global { return "global"; }
        when Private { return "private"; }
        when Loopback { return "loopback"; }
        when LinkLocal { return "linklocal"; }
        when Multicast { return "multicast"; }
        when Unspecified { return "unspecified"; }
        when Reserved { return "reserved"; }
    }
    return "?";
}

func testScopeMatch() {
    testing.assertEqual(scopeName(parseAddress("8.8.8.8")), "global");
    testing.assertEqual(scopeName(parseAddress("10.0.0.1")), "private");
    testing.assertEqual(scopeName(parseAddress("127.0.0.1")), "loopback");
    testing.assertEqual(scopeName(parseAddress("0.0.0.0")), "unspecified");
    testing.assertEqual(scopeName(parseAddress("240.0.0.1")), "reserved");
    testing.assertEqual(scopeName(parseAddress("ff02::1")), "multicast");
}
