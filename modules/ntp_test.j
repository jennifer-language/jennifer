# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
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
    $buf = appendUint($buf, 4294967295);   # 0xffffffff
    testing.assertEqual(readUint($buf, 4), 0);
    testing.assertEqual(readUint($buf, 8), 4294967295);
}

func testBuildRequestShape() {
    def req as bytes init buildRequest(time.fromUnix(1000000000));
    testing.assertEqual(len($req), 48);
    testing.assertEqual($req[0], 0x23);   # LI=0, VN=4, Mode=3 client
    testing.assertEqual($req[1], 0);      # stratum
    testing.assertEqual($req[2], 0);      # poll
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
