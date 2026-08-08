# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

# White-box tests for modules/snmp.j. Run: jennifer test modules/snmp_test.j
#
# These exercise the pure, network-free message codec - build a request or a
# simulated response with encodeMessage, decode it with decodeMessage, and
# assert the fields and every value type round-trip. The live send/recv is
# driven against a loopback UDP server in the Go suite (cmd/jennifer/snmp_test.go).

use testing;

# Decode a GET request and confirm its shape: version, community, context-0 PDU,
# and each binding value encoded as NULL.
func testBuildGetRequest() {
    def vbs as list of Varbind init oidsToVarbinds(["1.3.6.1.2.1.1.1.0", "1.3.6.1.2.1.1.5.0"]);
    def wire as bytes init encodeMessage(VERSION2C, "public", PDU_GET, 42, 0, 0, $vbs, true);

    def tree as asn1.Value init asn1.decode($wire);
    testing.assertEqual(asn1.asInt($tree, "/0"), VERSION2C);
    testing.assertEqual(asn1.asString($tree, "/1"), "public");

    def pdu as asn1.Value init asn1.get($tree, "/2");
    testing.assertEqual(asn1.tagClass($pdu), "context");
    testing.assertEqual(asn1.tagNumber($pdu), PDU_GET);
    testing.assertEqual(asn1.asInt($pdu, "/0"), 42);

    # Two bindings, each SEQUENCE { oid, NULL }.
    def vblist as asn1.Value init asn1.get($pdu, "/3");
    testing.assertEqual(asn1.length($vblist), 2);
    testing.assertEqual(asn1.asOid($vblist, "/0/0"), "1.3.6.1.2.1.1.1.0");
    testing.assertTrue(asn1.isNull($vblist, "/0/1"));
}

# Round-trip a simulated Response through decodeMessage and confirm the fields.
func testDecodeResponse() {
    def vbs as list of Varbind init [
        Varbind{oid: "1.3.6.1.2.1.1.3.0", type: "timeTicks", value: "", number: 7832124},
        Varbind{oid: "1.3.6.1.2.1.1.1.0", type: "octetString", value: "example agent", number: 0}
    ];
    def wire as bytes init encodeMessage(VERSION1, "public", PDU_RESPONSE, 99, 0, 0, $vbs, false);

    def resp as Message init decodeMessage($wire);
    testing.assertEqual($resp.requestId, 99);
    testing.assertEqual($resp.errorStatus, 0);
    testing.assertEqual(len($resp.varbinds), 2);
    testing.assertEqual($resp.varbinds[0].type, "timeTicks");
    testing.assertEqual($resp.varbinds[0].number, 7832124);
    testing.assertEqual($resp.varbinds[1].type, "octetString");
    testing.assertEqual($resp.varbinds[1].value, "example agent");
}

# Every value type survives an encode -> decode round-trip with the right type,
# string rendering, and numeric value.
func testValueTypesRoundTrip() {
    def vbs as list of Varbind init [
        Varbind{oid: "1.1", type: "integer", value: "-5", number: -5},
        Varbind{oid: "1.2", type: "octetString", value: "hello", number: 0},
        Varbind{oid: "1.3", type: "oid", value: "1.3.6.1.4.1.1602", number: 0},
        Varbind{oid: "1.4", type: "null", value: "", number: 0},
        Varbind{oid: "1.5", type: "counter32", value: "", number: 4000000000},
        Varbind{oid: "1.6", type: "gauge32", value: "", number: 12345},
        Varbind{oid: "1.7", type: "timeTicks", value: "", number: 100},
        Varbind{oid: "1.8", type: "ipAddress", value: "192.0.2.10", number: 0}
    ];
    def wire as bytes init encodeMessage(VERSION2C, "c", PDU_RESPONSE, 1, 0, 0, $vbs, false);
    def resp as Message init decodeMessage($wire);
    def got as list of Varbind init $resp.varbinds;

    testing.assertEqual($got[0].type, "integer");
    testing.assertEqual($got[0].number, -5);
    testing.assertEqual($got[1].type, "octetString");
    testing.assertEqual($got[1].value, "hello");
    testing.assertEqual($got[2].type, "oid");
    testing.assertEqual($got[2].value, "1.3.6.1.4.1.1602");
    testing.assertEqual($got[3].type, "null");
    testing.assertEqual($got[4].type, "counter32");
    testing.assertEqual($got[4].number, 4000000000);
    testing.assertEqual($got[4].value, "4000000000");
    testing.assertEqual($got[5].type, "gauge32");
    testing.assertEqual($got[5].number, 12345);
    testing.assertEqual($got[6].type, "timeTicks");
    testing.assertEqual($got[6].number, 100);
    testing.assertEqual($got[7].type, "ipAddress");
    testing.assertEqual($got[7].value, "192.0.2.10");
}

# An octet string that is not printable comes back as a hex rendering.
func testNonPrintableOctetString() {
    def raw as bytes;
    $raw[] = 0x00;
    $raw[] = 0x1b;
    $raw[] = 0xff;
    # Encode a real binary octet string directly, bypassing the string value path.
    def wire as bytes init encodeMessage(VERSION2C, "c", PDU_RESPONSE, 1, 0, 0, [
        Varbind{oid: "1.1", type: "octetString", value: convert.stringFromBytes(convert.bytesFromString("AB", "utf-8"), "utf-8"), number: 0}
    ], false);
    def resp as Message init decodeMessage($wire);
    testing.assertEqual($resp.varbinds[0].value, "AB");
    # Direct check of the hex fallback on binary content.
    testing.assertEqual(renderOctets($raw), "0x001bff");
    testing.assertTrue(isPrintable(convert.bytesFromString("router-01", "utf-8")));
    testing.assertTrue(not isPrintable($raw));
}

# The error-status field is carried through decode (request would throw on it).
func testErrorStatusDecoded() {
    def vbs as list of Varbind init oidsToVarbinds(["1.3.6.1.2.1.1.1.0"]);
    def wire as bytes init encodeMessage(VERSION1, "public", PDU_RESPONSE, 7, 2, 1, $vbs, true);
    def resp as Message init decodeMessage($wire);
    testing.assertEqual($resp.errorStatus, 2);
    testing.assertEqual($resp.errorIndex, 1);
}

# SNMPv2c exception variants come back as their named types.
func testExceptionVariants() {
    # Build a message with an explicit context-1 exception in place of the value.
    def excSeq as asn1.Value init asn1.sequence([
        asn1.sequence([asn1.oid("1.1"), asn1.retag("context", 1, asn1.null())])
    ]);
    def pdu as asn1.Value init asn1.retag("context", PDU_RESPONSE, asn1.sequence([
        asn1.integer(1), asn1.integer(0), asn1.integer(0), $excSeq
    ]));
    def msg as bytes init asn1.encode(asn1.sequence([
        asn1.integer(VERSION2C), asn1.octetString(convert.bytesFromString("c", "utf-8")), $pdu
    ]));
    def resp as Message init decodeMessage($msg);
    testing.assertEqual($resp.varbinds[0].type, "noSuchInstance");
}

func testIpAndSubtreeHelpers() {
    testing.assertEqual(renderIp(parseIp("10.0.2.15")), "10.0.2.15");
    testing.assertTrue(inSubtree("1.3.6.1.2.1.1.1.0", "1.3.6.1.2.1.1"));
    testing.assertTrue(inSubtree("1.3.6.1.2.1.1", "1.3.6.1.2.1.1"));
    testing.assertTrue(not inSubtree("1.3.6.1.2.1.2.1.0", "1.3.6.1.2.1.1"));
    testing.assertTrue(not inSubtree("1.3.6.1.2.1.10.0", "1.3.6.1.2.1.1"));
}

# Malformed input and a bad IP are catchable snmp errors.
func injectGarbage() {
    def bad as bytes;
    $bad[] = 0x30;
    $bad[] = 0x01;
    $bad[] = 0x02;
    decodeMessage($bad);
}

func injectBadIp() {
    parseIp("1.2.3");
}

func injectBadValueType() {
    encodeValue(Varbind{oid: "1.1", type: "nope", value: "", number: 0});
}

func testErrorsAreCatchable() {
    testing.assertThrows("injectGarbage", "snmp");
    testing.assertThrows("injectBadIp", "snmp");
    testing.assertThrows("injectBadValueType", "snmp");
}

# --- agent (server) tests: drive handleRequest with encoded requests ---

# The OID comparator orders by numeric component, not lexically as strings.
func testCompareOid() {
    testing.assertEqual(compareOid("1.3.6.1.2.1.1.2.0", "1.3.6.1.2.1.1.10.0"), -1);
    testing.assertEqual(compareOid("1.3.6.1.2.1.1.10.0", "1.3.6.1.2.1.1.2.0"), 1);
    testing.assertEqual(compareOid("1.3.6.1", "1.3.6.1"), 0);
    testing.assertEqual(compareOid("1.3.6", "1.3.6.1"), -1);
    testing.assertEqual(compareOid("1.3.6.1", "1.3.6"), 1);
}

func testAgentGet() {
    def ag as Agent init agent("public", VERSION2C, [
        stringVar("1.3.6.1.2.1.1.1.0", "example agent"),
        varbind("1.3.6.1.2.1.1.3.0", "timeTicks", "", 4200)
    ]);
    # An existing OID comes back with its value and the echoed request-id.
    def req as bytes init encodeMessage(VERSION2C, "public", PDU_GET, 7, 0, 0, oidsToVarbinds(["1.3.6.1.2.1.1.1.0"]), true);
    def r as ServeResult init handleRequest($ag.bindings, $ag, $req);
    def resp as Message init decodeMessage($r.reply);
    testing.assertEqual($resp.requestId, 7);
    testing.assertEqual($resp.varbinds[0].value, "example agent");
    # A missing OID is a v2c noSuchObject exception.
    def miss as bytes init encodeMessage(VERSION2C, "public", PDU_GET, 8, 0, 0, oidsToVarbinds(["1.3.6.1.2.1.99.0"]), true);
    def rm as ServeResult init handleRequest($ag.bindings, $ag, $miss);
    def respm as Message init decodeMessage($rm.reply);
    testing.assertEqual($respm.varbinds[0].type, "noSuchObject");
}

func testAgentGetNext() {
    def ag as Agent init agent("public", VERSION2C, [
        stringVar("1.3.6.1.2.1.1.1.0", "a"),
        stringVar("1.3.6.1.2.1.1.2.0", "b"),
        stringVar("1.3.6.1.2.1.1.10.0", "c")
    ]);
    # GETNEXT from ...1.2.0 must return ...1.10.0 (10 follows 2 numerically).
    def req as bytes init encodeMessage(VERSION2C, "public", PDU_GETNEXT, 1, 0, 0, oidsToVarbinds(["1.3.6.1.2.1.1.2.0"]), true);
    def r as ServeResult init handleRequest($ag.bindings, $ag, $req);
    def resp as Message init decodeMessage($r.reply);
    testing.assertEqual($resp.varbinds[0].oid, "1.3.6.1.2.1.1.10.0");
    # GETNEXT from the subtree root returns the first entry.
    def root as bytes init encodeMessage(VERSION2C, "public", PDU_GETNEXT, 2, 0, 0, oidsToVarbinds(["1.3.6.1.2.1.1"]), true);
    def rr as ServeResult init handleRequest($ag.bindings, $ag, $root);
    def respr as Message init decodeMessage($rr.reply);
    testing.assertEqual($respr.varbinds[0].oid, "1.3.6.1.2.1.1.1.0");
    # GETNEXT past the last entry is endOfMibView.
    def endReq as bytes init encodeMessage(VERSION2C, "public", PDU_GETNEXT, 3, 0, 0, oidsToVarbinds(["1.3.6.1.2.1.1.10.0"]), true);
    def re as ServeResult init handleRequest($ag.bindings, $ag, $endReq);
    def respe as Message init decodeMessage($re.reply);
    testing.assertEqual($respe.varbinds[0].type, "endOfMibView");
}

func testAgentSet() {
    def ag as Agent init agent("public", VERSION2C, [intVar("1.3.6.1.2.1.1.7.0", 72)]);
    # SET updates the MIB; the returned ServeResult carries the new MIB.
    def setReq as bytes init encodeMessage(VERSION2C, "public", PDU_SET, 1, 0, 0, [intVar("1.3.6.1.2.1.1.7.0", 100)], false);
    def r as ServeResult init handleRequest($ag.bindings, $ag, $setReq);
    testing.assertEqual(decodeMessage($r.reply).errorStatus, 0);
    # A follow-up GET against the updated MIB reflects the new value.
    def getReq as bytes init encodeMessage(VERSION2C, "public", PDU_GET, 2, 0, 0, oidsToVarbinds(["1.3.6.1.2.1.1.7.0"]), true);
    def r2 as ServeResult init handleRequest($r.mib, $ag, $getReq);
    def resp2 as Message init decodeMessage($r2.reply);
    testing.assertEqual($resp2.varbinds[0].number, 100);
    # SET on an unknown OID errors and leaves the MIB unchanged.
    def bad as bytes init encodeMessage(VERSION2C, "public", PDU_SET, 3, 0, 0, [intVar("1.3.6.1.2.1.99.0", 1)], false);
    def r3 as ServeResult init handleRequest($ag.bindings, $ag, $bad);
    def resp3 as Message init decodeMessage($r3.reply);
    testing.assertEqual($resp3.errorStatus, 2);
}

func testAgentWrongCommunityDropped() {
    def ag as Agent init agent("public", VERSION2C, [stringVar("1.1", "x")]);
    def req as bytes init encodeMessage(VERSION2C, "private", PDU_GET, 1, 0, 0, oidsToVarbinds(["1.1"]), true);
    def r as ServeResult init handleRequest($ag.bindings, $ag, $req);
    testing.assertEqual(len($r.reply), 0);
}

# The agent serves versions up to its configured one: a v1 agent drops a v2c
# request, a v2c agent accepts v1.
func testAgentVersionGate() {
    def v1agent as Agent init agent("public", VERSION1, [stringVar("1.1", "x")]);
    def req2c as bytes init encodeMessage(VERSION2C, "public", PDU_GET, 1, 0, 0, oidsToVarbinds(["1.1"]), true);
    def dropped as ServeResult init handleRequest($v1agent.bindings, $v1agent, $req2c);
    testing.assertEqual(len($dropped.reply), 0);

    def req1 as bytes init encodeMessage(VERSION1, "public", PDU_GET, 2, 0, 0, oidsToVarbinds(["1.1"]), true);
    def served as ServeResult init handleRequest($v1agent.bindings, $v1agent, $req1);
    testing.assertTrue(len($served.reply) > 0);

    def v2agent as Agent init agent("public", VERSION2C, [stringVar("1.1", "x")]);
    def acceptsV1 as ServeResult init handleRequest($v2agent.bindings, $v2agent, $req1);
    testing.assertTrue(len($acceptsV1.reply) > 0);
}

# A Counter64 (or any unsigned type) beyond int64 is rendered as inspectable hex
# with number 0, never a silently-wrong negative number.
func testUnsignedBeyondInt64() {
    # 8 octets of 0xFF = 2^64 - 1: does not fit a signed int64.
    def maxc as bytes;
    def k as int init 0;
    while ($k < 8) {
        $maxc[] = 0xff;
        $k = $k + 1;
    }
    def elem as asn1.Value init asn1.retag("application", 6, asn1.octetString($maxc));
    def vb as Varbind init decodeValue("1.1", $elem);
    testing.assertEqual($vb.type, "counter64");
    testing.assertEqual($vb.value, "0xffffffffffffffff");
    testing.assertEqual($vb.number, 0);

    # 9 octets (a leading 0x00 for the unsigned 2^63 range) also falls back.
    def big9 as bytes;
    $big9[] = 0x00;
    $big9[] = 0x80;
    def z as int init 0;
    while ($z < 7) {
        $big9[] = 0x00;
        $z = $z + 1;
    }
    def elem9 as asn1.Value init asn1.retag("application", 6, asn1.octetString($big9));
    def vb9 as Varbind init decodeValue("1.2", $elem9);
    testing.assertEqual($vb9.number, 0);
    testing.assertTrue(strings.startsWith($vb9.value, "0x"));

    # A value within int64 range still decodes to a decimal number.
    def small as bytes;
    $small[] = 0x01;
    $small[] = 0x00;
    def elemS as asn1.Value init asn1.retag("application", 1, asn1.octetString($small));
    def vbS as Varbind init decodeValue("1.3", $elemS);
    testing.assertEqual($vbS.type, "counter32");
    testing.assertEqual($vbS.number, 256);
}

use task;

# --- full client <-> agent round-trip over a loopback UDP socket ---
# snmp.j `use`s net + channel; the agent is served in a spawned task on a
# pre-bound socket (no bind race), driving the client's get/getNext/walk/set.

func testClientAgentRoundTrip() {
    def ag as Agent init agent("public", VERSION2C, [
        stringVar("1.3.6.1.2.1.1.1.0", "sysdescr"),
        stringVar("1.3.6.1.2.1.1.2.0", "middle"),
        stringVar("1.3.6.1.2.1.1.10.0", "last")
    ]);
    def socket as net.UDPSocket init net.listenUDP("127.0.0.1:0");
    def addr as string init net.address($socket);
    def stop as channel of bool init channel.make(1);
    def server as task of null init spawn {
        serveOn($ag, $socket, $stop);
    };

    def c as Client init clientWith($addr, "public", VERSION2C, 2000, 1);

    def got as list of Varbind init get($c, ["1.3.6.1.2.1.1.1.0"]);
    testing.assertEqual($got[0].value, "sysdescr");

    def nxt as list of Varbind init getNext($c, ["1.3.6.1.2.1.1.1.0"]);
    testing.assertEqual($nxt[0].oid, "1.3.6.1.2.1.1.2.0");

    def all as list of Varbind init walk($c, "1.3.6.1.2.1.1");
    testing.assertEqual(len($all), 3);

    def sr as list of Varbind init set($c, [stringVar("1.3.6.1.2.1.1.1.0", "updated")]);
    testing.assertEqual($sr[0].value, "updated");

    channel.send($stop, true);
    task.wait($server);
    net.close($socket);
}

# client() convenience + oidVar builder.
func testClientAndOidVarConstructors() {
    def c as Client init client("127.0.0.1", "public");
    testing.assertTrue(strings.contains($c.address, "127.0.0.1"));   # default host:port
    def ov as Varbind init oidVar("1.3.6.1.2.1.1.2.0", "1.3.6.1.4.1.8072");
    testing.assertEqual($ov.type, "oid");
    testing.assertEqual($ov.value, "1.3.6.1.4.1.8072");
}
