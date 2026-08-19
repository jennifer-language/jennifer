# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# mactelnet_test.j - white-box tests for mactelnet.j. Run with:
#
#     jennifer test modules/mactelnet_test.j
#
# These exercise the pure wire codec (header build/parse, control-block framing,
# the byte helpers), the MD5 password proof, the EC-SRP client wiring, and MAC
# parsing - all with no network. The live handshake (session start, auth, the
# counter/ACK loop) is driven against a fake MAC-Telnet server in the Go suite
# (cmd/jennifer/mactelnet_test.go). mactelnet.j already `use`s net / crypto /
# hash / binary / convert / strings / encoding / fs / kv / os, so the overlay
# only adds testing.
use testing;

func hex(b as bytes) {
    return encoding.toText($b, "hex");
}

func zeros(n as int) {
    def b as bytes;
    for (def i in 0..$n) {
        $b[] = 0;
    }
    return $b;
}

func testByteHelpers() {
    testing.assertEqual(hex(u16be(0xabcd)), "abcd");
    testing.assertEqual(hex(u16le(0xabcd)), "cdab");
    testing.assertEqual(hex(u32be(0x01020304)), "01020304");
    testing.assertEqual(readU16be(u16be(0x1234), 0), 0x1234);
    testing.assertEqual(readU32be(u32be(0x0a0b0c0d), 0), 0x0a0b0c0d);
}

func testParseMac() {
    testing.assertEqual(hex(parseMac("aa:bb:cc:dd:ee:ff")), "aabbccddeeff");
    testing.assertEqual(hex(parseMac("AA:BB:CC:DD:EE:FF")), "aabbccddeeff");
    testing.assertEqual(hex(parseMac("aabb-ccdd-eeff")), "aabbccddeeff");
    testing.assertEqual(hex(parseMac("aabbccddeeff")), "aabbccddeeff");
    testing.assertEqual(formatMac(parseMac("aa:bb:cc:dd:ee:ff")), "aa:bb:cc:dd:ee:ff");
}

func badMac() {
    parseMac("aa:bb:cc");
}

func testParseMacRejectsBadLength() {
    testing.assertThrows("badMac", "mactelnet");
}

func testBuildHeaderClientLayout() {
    def src as bytes init parseMac("01:02:03:04:05:06");
    def dst as bytes init parseMac("aa:bb:cc:dd:ee:ff");
    def h as bytes init buildHeader(PTYPE_DATA, $src, $dst, 0xabcd, 0x01020304);
    # ver=01 ptype=01 src dst seskey(BE)=abcd clienttype=0015 counter(BE)=01020304
    testing.assertEqual(len($h), 22);
    testing.assertEqual(hex($h), "0101010203040506aabbccddeeffabcd001501020304");
}

func testParseHeaderServerLayout() {
    # Server->client packets carry the session key at offset 16 (client type at
    # 14), the mirror of the client layout buildHeader emits.
    def sp as bytes;
    $sp[] = 1;
    $sp[] = PTYPE_ACK;
    $sp = binary.concat($sp, parseMac("010203040506"));
    $sp = binary.concat($sp, parseMac("0a0b0c0d0e0f"));
    $sp = binary.concat($sp, u16be(0x0015));
    $sp = binary.concat($sp, u16be(0xbeef));
    $sp = binary.concat($sp, u32be(0x00000064));
    def hd as Header init parseHeader($sp);
    testing.assertEqual($hd.ptype, PTYPE_ACK);
    testing.assertEqual($hd.seskey, 0xbeef);
    testing.assertEqual($hd.counter, 0x64);
}

func testControlBlockFraming() {
    testing.assertEqual(hex(controlBlock(CPTYPE_BEGINAUTH, emptyBytes())), "563412ff0000000000");
    def two as bytes;
    $two[] = 0xaa;
    $two[] = 0xbb;
    testing.assertEqual(hex(controlBlock(CPTYPE_PASSWORD, $two)), "563412ff0200000002aabb");
}

func testMatchMagic() {
    def yes as bytes;
    $yes[] = 0x56;
    $yes[] = 0x34;
    $yes[] = 0x12;
    $yes[] = 0xff;
    $yes[] = 0x99;
    testing.assertEqual(matchMagic($yes, 0), true);
    def no as bytes;
    $no[] = 0x00;
    $no[] = 0x34;
    $no[] = 0x12;
    $no[] = 0xff;
    testing.assertEqual(matchMagic($no, 0), false);
}

func testMd5Password() {
    def salt as bytes init zeros(16);
    def pw as bytes init md5Password("test", $salt);
    # 0x00 followed by a 16-byte digest.
    testing.assertEqual(len($pw), 17);
    testing.assertEqual($pw[0], 0);
    # Cross-check the exact preimage: MD5(0x00 + password + salt).
    def pre as bytes;
    $pre[] = 0;
    $pre = binary.concat($pre, utf8("test"));
    $pre = binary.concat($pre, $salt);
    def md as bytes init hash.compute($pre, "md5");
    testing.assertEqual(hex(binary.slice($pw, 1, 17)), hex($md));
}

func testLoginKey() {
    def kp as crypto.Keypair init crypto.mtweiKeygen();
    def lk as bytes init loginKey("ab", $kp.public);
    # "ab" + 0x00 + 33-byte public key.
    testing.assertEqual(len($lk), 36);
    testing.assertEqual($lk[0], 0x61);
    testing.assertEqual($lk[1], 0x62);
    testing.assertEqual($lk[2], 0);
    testing.assertEqual(hex(binary.slice($lk, 3, 36)), hex($kp.public));
}

func testEcSrpWiring() {
    def kp as crypto.Keypair init crypto.mtweiKeygen();
    testing.assertEqual(len($kp.public), 33);
    testing.assertEqual(len($kp.private), 32);
    def salt16 as bytes init zeros(16);
    def val as bytes init crypto.mtweiId("admin", "pw", $salt16);
    testing.assertEqual(len($val), 32);
    # A second keypair's public key is a valid on-curve server key for wiring.
    def sk as crypto.Keypair init crypto.mtweiKeygen();
    def resp as bytes init crypto.mtweiClientKey($kp.private, $sk.public, $kp.public, $val);
    testing.assertEqual(len($resp), 32);
}
