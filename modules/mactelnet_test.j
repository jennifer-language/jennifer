# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# mactelnet_test.j - white-box tests for mactelnet.j. Run with:
#
#     jennifer test modules/mactelnet_test.j
#
# These exercise the pure wire codec (header build/parse, control-block framing,
# the byte helpers), the MD5 password proof, the EC-SRP client wiring, and MAC
# parsing - all with no network. The live handshake (session start, auth, the
# counter/ACK loop) is driven against a fake MAC-Telnet server in the Go suite
# (cmd/jennifer/mactelnet_test.go); the refusal of an MD5-only router, the
# deadlines under a flood and the console buffer cap are tested here against
# loopback sockets. mactelnet.j already `use`s net / crypto / hash / binary /
# convert / strings / encoding / fs / kv / os / time, so the overlay adds
# testing and task.
use testing;
use task;
use time;

func hex(b as bytes) {
    return encoding.toText("hex", $b);
}

func zeros(n as int) {
    return binary.make($n);
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
    $sp = binary.join([
        $sp,
        parseMac("010203040506"),
        parseMac("0a0b0c0d0e0f"),
        u16be(0x0015),
        u16be(0xbeef),
        u32be(0x00000064)
    ]);
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
    def zero as bytes;
    $zero[] = 0;
    def md as bytes init hash.compute("md5", binary.join([$zero, utf8("test"), $salt]));
    testing.assertEqual(hex($pw[1..17]), hex($md));
}

func testLoginKey() {
    def kp as crypto.Keypair init crypto.mtweiKeygen();
    def lk as bytes init loginKey("ab", $kp.public);
    # "ab" + 0x00 + 33-byte public key.
    testing.assertEqual(len($lk), 36);
    testing.assertEqual($lk[0], 0x61);
    testing.assertEqual($lk[1], 0x62);
    testing.assertEqual($lk[2], 0);
    testing.assertEqual(hex($lk[3..36]), hex($kp.public));
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

# SECURITY: a datagram shorter than a MACtelnet header from any LAN peer must be
# dropped inside recvPacket (not returned, not thrown), so a single runt cannot
# abort the session with an out-of-bounds error in parseHeader. The next
# full-length datagram is still delivered, and a runt with nothing behind it
# times out to empty rather than throwing.
func testRecvPacketDropsRunt() {
    def recvSock as net.UDPSocket init net.listenUDP("127.0.0.1:0");
    def addr as string init net.address($recvSock);
    def sender as net.UDPSocket init net.listenUDP("127.0.0.1:0");
    def sess as Session init Session{
        sock: $recvSock,
        state: kv.open(),
        srcmac: emptyBytes(),
        dstmac: emptyBytes(),
        seskey: 0
    };
    def runt as bytes init convert.bytesFromString("utf-8", "hello");
    def full as bytes init binary.make(HEADER_LEN);
    net.sendTo($sender, $addr, $runt);
    net.sendTo($sender, $addr, $full);
    testing.assertEqual(len(recvPacket($sess, 2000)), HEADER_LEN); # runt skipped, full returned
    net.sendTo($sender, $addr, $runt);
    testing.assertEqual(len(recvPacket($sess, 300)), 0); # lone runt -> timeout, no throw
    net.close($recvSock);
    net.close($sender);
}

# --- MD5 refusal, deadlines and the console buffer -----------------------------

def const ROUTER_MAC as string init "aa:bb:cc:dd:ee:ff";

# routerHeader builds a router's 22-byte header: the router MAC as the source,
# client type at 14, the session key at 16.
func routerHeader(ptype as int, seskey as int, counter as int) {
    def lead as bytes;
    $lead[] = 1;
    $lead[] = $ptype;
    return binary.join([
        $lead,
        parseMac(ROUTER_MAC),
        binary.make(6),
        u16be(0x0015),
        u16be($seskey),
        u32be($counter)
    ]);
}

# hasControl reports whether a DATA payload carries a control block of `cptype`.
func hasControl(payload as bytes, cptype as int) {
    def off as int init 0;
    while ($off + 9 <= len($payload) and matchMagic($payload, $off)) {
        if ($payload[$off + 4] == $cptype) {
            return true;
        }
        $off = $off + 9 + readU32be($payload, $off + 5);
    }
    return false;
}

# md5Router plays a pre-6.43 router on `sock`: it offers a 16-byte salt and
# ends authentication once a password proof arrives. It returns whether one
# did, after the client ends the session or two quiet seconds.
func md5Router(sock as net.UDPSocket) {
    def proof as bool init false;
    def out as int init 0;
    while (true) {
        net.setReadDeadline($sock, 2000);
        def dg as net.Datagram;
        try {
            $dg = net.recvFrom($sock, MAX_DATAGRAM);
        } catch (e) {
            if ($e.kind == "timeout") {
                return $proof;
            }
            throw $e;
        }
        def pkt as bytes init $dg.data;
        def seskey as int init readU16be($pkt, 14);
        if ($pkt[1] == PTYPE_SESSIONSTART) {
            net.sendTo($sock, $dg.peer, routerHeader(PTYPE_ACK, $seskey, 0));
        } elseif ($pkt[1] == PTYPE_END) {
            return $proof;
        } elseif ($pkt[1] == PTYPE_DATA) {
            def payload as bytes init $pkt[HEADER_LEN..];
            net.sendTo(
                $sock,
                $dg.peer,
                routerHeader(PTYPE_ACK, $seskey, readU32be($pkt, 18) + len($payload)));
            def body as bytes;
            if (hasControl($payload, CPTYPE_BEGINAUTH)) {
                $body = controlBlock(CPTYPE_PASSSALT, utf8("mactelnet16bsalt"));
            } elseif (hasControl($payload, CPTYPE_PASSWORD)) {
                $proof = true;
                $body = controlBlock(CPTYPE_END_AUTH, emptyBytes());
            }
            if (len($body) > 0) {
                net.sendTo(
                    $sock,
                    $dg.peer,
                    binary.concat(routerHeader(PTYPE_DATA, $seskey, $out), $body));
                $out = $out + len($body);
            }
        }
    }
}

# md5Login logs in to md5Router as `connect` (allowMd5 false) or through
# connectWith with allowLegacyMd5, and returns the error kind ("ok" for a
# login) and whether the router received a password proof.
func md5Login(allowMd5 as bool) {
    def sock as net.UDPSocket init net.listenUDP("127.0.0.1:0");
    defer net.close($sock);
    os.setEnv("JENNIFER_MACTELNET_TARGET", net.address($sock));
    defer os.unsetEnv("JENNIFER_MACTELNET_TARGET");
    def peer as task of bool init spawn {
        return md5Router($sock);
    };
    def kind as string init "ok";
    try {
        def s as Session;
        if ($allowMd5) {
            def o as Options init options("lo", ROUTER_MAC, "admin", "pw");
            $o.allowLegacyMd5 = true;
            $s = connectWith($o);
        } else {
            $s = connect("lo", ROUTER_MAC, "admin", "pw");
        }
        close($s);
    } catch (e) {
        $kind = $e.kind;
        testing.assertContains($e.message, "allowLegacyMd5");
    }
    def proof as bool init task.wait($peer);
    return "{$kind} proof={$proof}";
}

# A router that offers only the MD5 login is refused before any password proof
# is sent: the proof would let it, or anyone watching, guess the password
# offline. allowLegacyMd5 lets it through.
func testAnMd5OnlyRouterIsRefusedByDefault() {
    testing.assertEqual(md5Login(false), "mactelnet proof=false");
}

func testAllowLegacyMd5LogsIn() {
    testing.assertFalse(options("lo", ROUTER_MAC, "admin", "pw").allowLegacyMd5);
    testing.assertEqual(md5Login(true), "ok proof=true");
}

# looseSession is a session on a loopback socket that sends to `target`, with
# the counters of a fresh login.
func looseSession(target as string) {
    def store as kv.Store init kv.open();
    kv.set($store, "out", "0", 0);
    kv.set($store, "in", "-1", 0);
    kv.set($store, "buf", "", 0);
    kv.set($store, "open", "1", 0);
    kv.set($store, "target", $target, 0);
    return Session{
        sock: net.listenUDP("127.0.0.1:0"),
        state: $store,
        srcmac: binary.make(6),
        dstmac: parseMac(ROUTER_MAC),
        seskey: 1234
    };
}

# flood sends a header-sized datagram that belongs to no session to `addr`
# every 20 ms, until `stop` holds "1" or eight seconds have passed.
func flood(addr as string, stop as kv.Store) {
    def sender as net.UDPSocket init net.listenUDP("127.0.0.1:0");
    defer net.close($sender);
    def junk as bytes init binary.make(32);
    $junk[0] = 1;
    for (def i in 0..400) {
        if (kv.get($stop, "stop") == "1") {
            return null;
        }
        net.sendTo($sender, $addr, $junk);
        time.sleep(time.fromMilliseconds(20));
    }
    return null;
}

func clockMs() {
    return time.unixMillis(time.now());
}

# Datagrams for no session restart no wait: a send under a flood still times
# out on its schedule (about 3.3 seconds), and recv returns at its deadline.
func testAFloodCannotStretchTheWaits() {
    def sink as net.UDPSocket init net.listenUDP("127.0.0.1:0");
    defer net.close($sink);
    def s as Session init looseSession(net.address($sink));
    defer net.close($s.sock);
    def stop as kv.Store init kv.open();
    kv.set($stop, "stop", "0", 0);
    def flooder as task of null init spawn {
        return flood(net.address($s.sock), $stop);
    };
    def t0 as int init clockMs();
    try {
        sendReliable($s, buildHeader(PTYPE_SESSIONSTART, $s.srcmac, $s.dstmac, $s.seskey, 0));
        throw Error{kind: "missed", message: "a timeout"};
    } catch (e) {
        testing.assertEqual($e.kind, "timeout");
    }
    def sendMs as int init clockMs() - $t0;
    $t0 = clockMs();
    testing.assertEqual(recv($s, 300), "");
    def recvMs as int init clockMs() - $t0;
    kv.set($stop, "stop", "1", 0);
    task.wait($flooder);
    testing.assertTrue($sendMs < 5000);
    testing.assertTrue($recvMs < 1000);
}

# Console output nobody reads stops at 1 Mi characters (MAX_CONSOLE_CHARS): the
# packet that would pass it is kind "limit", neither acknowledged nor counted,
# so the router resends it, and it is taken once recv has drained the buffer.
func testUnreadConsoleOutputIsCapped() {
    def sink as net.UDPSocket init net.listenUDP("127.0.0.1:0");
    defer net.close($sink);
    def s as Session init looseSession(net.address($sink));
    defer net.close($s.sock);
    kv.set($s.state, "buf", strings.replicate("x", 1048574), 0);
    def pkt as bytes init binary.concat(routerHeader(PTYPE_DATA, 1234, 0), utf8("hello"));
    try {
        processData($s, $pkt);
        throw Error{kind: "missed", message: "a full buffer"};
    } catch (e) {
        testing.assertEqual($e.kind, "limit");
    }
    testing.assertEqual(getInt($s.state, "in"), -1);
    net.setReadDeadline($sink, 200);
    try {
        net.recvFrom($sink, MAX_DATAGRAM);
        throw Error{kind: "missed", message: "no ACK"};
    } catch (e) {
        testing.assertEqual($e.kind, "timeout");
    }
    testing.assertEqual(len(recv($s, 0)), 1048574);
    processData($s, $pkt);
    testing.assertEqual(recv($s, 0), "hello");
    testing.assertEqual(getInt($s.state, "in"), 0);
    testing.assertEqual(MAX_CONSOLE_CHARS, 1048576);
}
