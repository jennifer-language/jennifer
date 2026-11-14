# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# ntp_test.j - white-box tests for ntp.j. Run with:
#
#     jennifer test modules/ntp_test.j
#
# These exercise the pure packet codec (appendUint / readUint / buildRequest /
# readTimestamp) with no network; the live query is driven against a fake UDP
# server in the Go suite (cmd/jennifer/ntp_test.go). ntp.j already `use`s time,
# so the overlay only adds testing.
use testing;

func testAppendReadUintRoundTrip() {
    def buf as bytes;
    $buf = appendUint($buf, 0xdeadbeef);
    testing.assertEqual(readUint($buf, 0), 0xdeadbeef);
    $buf = appendUint($buf, 0);
    $buf = appendUint($buf, 4294967295); # 0xffffffff
    testing.assertEqual(readUint($buf, 4), 0);
    testing.assertEqual(readUint($buf, 8), 4294967295);
}

func testBuildRequestShape() {
    def req as bytes init buildRequest(time.fromUnix(1000000000));
    testing.assertEqual(len($req), 48);
    testing.assertEqual($req[0], 0x23); # LI=0, VN=4, Mode=3 client
    testing.assertEqual($req[1], 0); # stratum
    testing.assertEqual($req[2], 0); # poll
}

func testBuildRequestTransmitRoundTrip() {
    def t as time.Time init time.fromUnix(1700000000);
    def req as bytes init buildRequest($t);
    # the transmit timestamp (bytes 40..47) decodes back to the same instant
    testing.assertEqual(time.unix(readTimestamp($req, 40)), 1700000000);
}

func testReadTimestampEpoch() {
    # NTP seconds == the 1900->1970 offset means Unix time zero.
    def buf as bytes;
    $buf = appendUint($buf, NTP_UNIX_OFFSET);
    $buf = appendUint($buf, 0);
    testing.assertEqual(time.unix(readTimestamp($buf, 0)), 0);
}

func testReadTimestampKnown() {
    def buf as bytes;
    $buf = appendUint($buf, NTP_UNIX_OFFSET + 1700000000);
    $buf = appendUint($buf, 0);
    testing.assertEqual(time.unix(readTimestamp($buf, 0)), 1700000000);
}

func testTimestampFractionRoundTripsToMillis() {
    # Half a second of fraction survives the pack / unpack to millisecond scale.
    def t as time.Time init time.fromUnixMillis(1700000000500);
    def buf as bytes;
    $buf = appendUint($buf, time.unix($t) + NTP_UNIX_OFFSET);
    $buf = appendUint($buf, ((time.unixNanos($t) % 1000000000) << 32) // 1000000000);
    testing.assertEqual(time.unixMillis(readTimestamp($buf, 0)), 1700000000500);
}

# reply builds a server reply to `req`: mode 4, stratum `stratum`, leap `li`,
# the request's transmit timestamp echoed as the originate timestamp.
func reply(req as bytes, li as int, stratum as int) {
    def r as bytes;
    $r[] = ($li << 6) | (4 << 3) | 4;
    $r[] = $stratum;
    for (def i in 2..12) {
        $r[] = 0;
    }
    for (def c in [82, 65, 84, 69]) { # "RATE"
        $r[] = $c;
    }
    for (def i in 16..24) {
        $r[] = 0;
    }
    $r = binary.concat($r, binary.slice($req, 40, 48));
    $r = binary.concat($r, binary.slice($req, 40, 48)); # receive
    return binary.concat($r, binary.slice($req, 40, 48)); # transmit
}

func kissOfDeath() {
    def req as bytes init buildRequest(time.now());
    checkReply(reply($req, 0, 0), $req, "x");
}
func unsynchronised() {
    def req as bytes init buildRequest(time.now());
    checkReply(reply($req, 3, 2), $req, "x");
}
func wrongOriginate() {
    def req as bytes init buildRequest(time.now());
    checkReply(reply(buildRequest(time.fromUnix(1)), 0, 2), $req, "x");
}
# A reply that does not answer this request, a kiss-of-death and an
# unsynchronised server are refused; a well-formed reply passes.
func testReplyChecks() {
    testing.assertThrows("kissOfDeath", "ntp");
    testing.assertThrows("unsynchronised", "ntp");
    testing.assertThrows("wrongOriginate", "ntp");
    def req as bytes init buildRequest(time.now());
    checkReply(reply($req, 0, 2), $req, "x");
}

# Timestamps past the 2036 rollover (era 1) decode forward, not back to 1900.
func testEraRollover() {
    def b as bytes;
    for (def v in [0, 0, 0, 1, 0, 0, 0, 0]) {
        $b[] = $v;
    }
    testing.assertEqual(time.unix(readTimestamp($b, 0)), 4294967296 + 1 - 2208988800);
}

# A Kiss-o'-Death is an NtpError with its code; another failure has none.
func testKissOfDeathCarriesItsCode() {
    try {
        kissOfDeath();
        throw Error{kind: "missed", message: "a kiss-of-death"};
    } catch (e) {
        testing.assertEqual($e.kind, "ntp");
        testing.assertEqual($e.kissCode, "RATE");
        testing.assertEqual($e.message, "server x sent a Kiss-o'-Death (RATE)");
    }
    try {
        unsynchronised();
        throw Error{kind: "missed", message: "an unsynchronised server"};
    } catch (e) {
        testing.assertEqual($e.kissCode, "");
    }
}
