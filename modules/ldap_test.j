# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# ldap_test.j - white-box tests for ldap.j's pure protocol logic. Run with:
#
#     jennifer test modules/ldap_test.j
#
# The overlay splices ldap.j in front of this file, so the tests reach its
# private helpers (parseFilter, evalFilter, normDn, inScope, compareStr,
# verifyPassword, the BER encoders/decoders, the kv-backed directory) by bare
# identifier. The networked client / server round-trip is verified end to end in
# the Go suite (TestLdapDirectory).
use testing;

func sampleEntry() {
    return entry(
        "uid=alice,ou=people,dc=example,dc=org",
        {
            "objectClass": ["inetOrgPerson", "person"],
            "uid": ["alice"],
            "cn": ["Alice Alpha"],
            "mail": ["alice@example.org"]
        });
}

# --- RFC 4515 filter parsing (each variant maps to its context tag number) ---

func testFilterParseTags() {
    testing.assertEqual(asn1.tagNumber(parseFilter("(objectClass=*)")), 7);
    testing.assertEqual(asn1.tagNumber(parseFilter("(uid=alice)")), 3);
    testing.assertEqual(asn1.tagNumber(parseFilter("(cn=a*b*c)")), 4);
    testing.assertEqual(asn1.tagNumber(parseFilter("(&(a=1)(b=2))")), 0);
    testing.assertEqual(asn1.tagNumber(parseFilter("(|(a=1)(b=2))")), 1);
    testing.assertEqual(asn1.tagNumber(parseFilter("(!(a=1))")), 2);
    testing.assertEqual(asn1.tagNumber(parseFilter("(n>=5)")), 5);
    testing.assertEqual(asn1.tagNumber(parseFilter("(n<=5)")), 6);
}

func testFilterParseNested() {
    def f as asn1.Value init parseFilter("(&(objectClass=person)(|(uid=a)(uid=b)))");
    testing.assertEqual(asn1.tagNumber($f), 0);
    testing.assertEqual(asn1.length($f), 2);
    testing.assertEqual(asn1.tagNumber(asn1.get($f, "/1")), 1);
    testing.assertEqual(asn1.length(asn1.get($f, "/1")), 2);
}

# --- filter evaluation against an entry ---

func testEvalEquality() {
    def e as Entry init sampleEntry();
    testing.assertEqual(evalFilter(parseFilter("(uid=alice)"), $e), true);
    testing.assertEqual(evalFilter(parseFilter("(uid=bob)"), $e), false);
    testing.assertEqual(evalFilter(parseFilter("(UID=ALICE)"), $e), true);
    testing.assertEqual(evalFilter(parseFilter("(objectClass=person)"), $e), true);
}

func testEvalPresence() {
    def e as Entry init sampleEntry();
    testing.assertEqual(evalFilter(parseFilter("(mail=*)"), $e), true);
    testing.assertEqual(evalFilter(parseFilter("(telephoneNumber=*)"), $e), false);
}

func testEvalSubstrings() {
    def e as Entry init sampleEntry();
    testing.assertEqual(evalFilter(parseFilter("(cn=Alice*)"), $e), true);
    testing.assertEqual(evalFilter(parseFilter("(cn=*Alpha)"), $e), true);
    testing.assertEqual(evalFilter(parseFilter("(cn=*ce Al*)"), $e), true);
    testing.assertEqual(evalFilter(parseFilter("(mail=al*@*.org)"), $e), true);
    testing.assertEqual(evalFilter(parseFilter("(cn=Bob*)"), $e), false);
}

func testEvalBoolean() {
    def e as Entry init sampleEntry();
    testing.assertEqual(evalFilter(parseFilter("(&(uid=alice)(mail=*))"), $e), true);
    testing.assertEqual(evalFilter(parseFilter("(&(uid=alice)(uid=bob))"), $e), false);
    testing.assertEqual(evalFilter(parseFilter("(|(uid=bob)(uid=alice))"), $e), true);
    testing.assertEqual(evalFilter(parseFilter("(!(uid=bob))"), $e), true);
    testing.assertEqual(evalFilter(parseFilter("(!(uid=alice))"), $e), false);
}

# --- DN normalisation, scope, ordering ---

func testNormDn() {
    testing.assertEqual(normDn("UID=Alice, OU=People, DC=X"), "uid=alice,ou=people,dc=x");
    testing.assertEqual(normDn("cn=x,dc=y"), "cn=x,dc=y");
}

func testInScope() {
    def base as string init "ou=people,dc=example,dc=org";
    testing.assertEqual(inScope($base, $base, SCOPE_BASE), true);
    testing.assertEqual(inScope("uid=a," + $base, $base, SCOPE_BASE), false);
    testing.assertEqual(inScope("uid=a," + $base, $base, SCOPE_SUB), true);
    testing.assertEqual(inScope("uid=a," + $base, $base, SCOPE_ONE), true);
    testing.assertEqual(inScope("uid=a,ou=sub," + $base, $base, SCOPE_ONE), false);
    testing.assertEqual(inScope("uid=a,dc=other", $base, SCOPE_SUB), false);
}

func testCompareStr() {
    testing.assertEqual(compareStr("a", "b"), -1);
    testing.assertEqual(compareStr("b", "a"), 1);
    testing.assertEqual(compareStr("abc", "abc"), 0);
    testing.assertEqual(compareStr("ab", "abc"), -1);
}

# --- userPassword schemes ---

func testPasswordSchemes() {
    for (def scheme in ["plain", "sha", "sha256", "ssha", "ssha256", "pbkdf2", "pbkdf2-sha512"]) {
        def stored as string init password("s3cret!", $scheme);
        testing.assertEqual(verifyPassword($stored, "s3cret!"), true);
        testing.assertEqual(verifyPassword($stored, "wrong"), false);
    }
}

func testPbkdf2Format() {
    def stored as string init password("hunter2", "pbkdf2");
    testing.assertEqual(strings.startsWith($stored, '{PBKDF2-SHA256}$100000$'), true);
    # four $-separated fields after the prefix: "", iters, salt, hash
    def parts as list of string init strings.split(
        strings.substring($stored, 15, len($stored)),
        "$");
    testing.assertEqual(len($parts), 4);
    testing.assertEqual($parts[1], "100000");
    def s512 as string init password("hunter2", "pbkdf2-sha512");
    testing.assertEqual(strings.startsWith($s512, '{PBKDF2-SHA512}$'), true);
}

func testPasswordEmptyRejected() {
    testing.assertEqual(verifyPassword("", "anything"), false);
}

# A malformed stored value must verify false and never throw - otherwise the
# error unwinds out of the server's bind handler (audit F1/F3).
func testMalformedPasswordNoThrow() {
    testing.assertEqual(verifyPassword('{PBKDF2-SHA256}$notanumber$AA==$AA==', "x"), false);
    testing.assertEqual(verifyPassword('{SSHA}@@@notbase64', "x"), false);
    testing.assertEqual(verifyPassword('{SSHA256}@@@notbase64', "x"), false);
    # iterations far over the honoured cap -> reject rather than stall
    testing.assertEqual(
        verifyPassword('{PBKDF2-SHA256}$999999999999$QUFBQQ==$QUFBQQ==', "x"),
        false);
    # a valid pbkdf2 value still verifies
    def good as string init password("s3cret", "pbkdf2");
    testing.assertEqual(verifyPassword($good, "s3cret"), true);
    testing.assertEqual(verifyPassword($good, "nope"), false);
}

# --- entry accessors ---

func testAccessors() {
    def e as Entry init sampleEntry();
    testing.assertEqual(firstValue($e, "mail"), "alice@example.org");
    testing.assertEqual(firstValue($e, "MAIL"), "alice@example.org");
    testing.assertEqual(firstValue($e, "absent"), "");
    testing.assertEqual(len(values($e, "objectClass")), 2);
}

# --- mutable kv-backed directory (no socket) ---

func testDirectoryMutation() {
    def d as Directory init directory([sampleEntry()]);
    testing.assertEqual(len(listEntries($d)), 1);
    testing.assertEqual(hasEntry($d, "uid=alice,ou=people,dc=example,dc=org"), true);

    addEntry($d, entry("uid=bob,ou=people,dc=example,dc=org", {"uid": ["bob"]}));
    testing.assertEqual(len(listEntries($d)), 2);

    setAttribute($d, "uid=bob,ou=people,dc=example,dc=org", "mail", ["bob@example.org"]);
    testing.assertEqual(
        firstValue(getEntry($d, "uid=bob,ou=people,dc=example,dc=org"), "mail"),
        "bob@example.org");

    # addEntry replaces an entry with the same DN rather than duplicating it
    addEntry($d, entry("uid=bob,ou=people,dc=example,dc=org", {"uid": ["bobby"]}));
    testing.assertEqual(len(listEntries($d)), 2);
    testing.assertEqual(
        firstValue(getEntry($d, "uid=bob,ou=people,dc=example,dc=org"), "uid"),
        "bobby");

    deleteEntry($d, "uid=alice,ou=people,dc=example,dc=org");
    testing.assertEqual(len(listEntries($d)), 1);
    testing.assertEqual(hasEntry($d, "uid=alice,ou=people,dc=example,dc=org"), false);
}

func testGroupHelper() {
    def g as Entry init group("cn=admins,ou=groups,dc=x", ["uid=a,dc=x", "uid=b,dc=x"]);
    testing.assertEqual(firstValue($g, "objectClass"), "groupOfNames");
    testing.assertEqual(len(values($g, "member")), 2);
}

# --- BER message round-trips (encode then decode with the same codec) ---

func testBindRequestRoundtrip() {
    def raw as bytes init encodeMessage(42, encodeBindRequest("cn=admin,dc=x", "pw"), []);
    def msg as asn1.Value init asn1.decode($raw);
    testing.assertEqual(asn1.asInt($msg, "/0"), 42);
    def op as asn1.Value init asn1.get($msg, "/1");
    testing.assertEqual(asn1.tagClass($op), "application");
    testing.assertEqual(asn1.tagNumber($op), 0);
    testing.assertEqual(asn1.asInt($op, "/0"), 3);
    testing.assertEqual(asn1.asString($op, "/1"), "cn=admin,dc=x");
}

func testBindResponseRoundtrip() {
    def msg as asn1.Value init asn1.decode(encodeBindResponse(
        7,
        INVALID_CREDENTIALS,
        "",
        "bad creds"));
    def r as Result init parseResult(asn1.get($msg, "/1"));
    testing.assertEqual($r.code, 49);
    testing.assertEqual($r.message, "bad creds");
}

func testSearchEntryRoundtrip() {
    def raw as bytes init encodeSearchEntry(9, sampleEntry(), ["uid", "mail"]);
    def msg as asn1.Value init asn1.decode($raw);
    def e2 as Entry init parseEntry(asn1.get($msg, "/1"));
    testing.assertEqual($e2.dn, "uid=alice,ou=people,dc=example,dc=org");
    testing.assertEqual(firstValue($e2, "uid"), "alice");
    testing.assertEqual(firstValue($e2, "mail"), "alice@example.org");
    # cn / objectClass were not requested, so they are not projected
    testing.assertEqual(len(values($e2, "cn")), 0);
}

# --- write-request encoders (round-tripped through the BER codec) ---

func testAddRequestEncoding() {
    def op as asn1.Value init encodeAddRequest(
        "uid=x,dc=y",
        {"objectClass": ["top", "person"], "cn": ["X"]});
    testing.assertEqual(asn1.tagClass($op), "application");
    testing.assertEqual(asn1.tagNumber($op), 8);
    testing.assertEqual(asn1.asString($op, "/0"), "uid=x,dc=y");
    # first attribute: type + SET OF values
    testing.assertEqual(asn1.asString($op, "/1/0/0"), "objectClass");
    testing.assertEqual(asn1.length(asn1.get($op, "/1/0/1")), 2);
}

func testModifyRequestEncoding() {
    def op as asn1.Value init encodeModifyRequest(
        "uid=x,dc=y",
        [change(MOD_REPLACE, "mail", ["x@y.org"])]);
    testing.assertEqual(asn1.tagNumber($op), 6);
    testing.assertEqual(asn1.asString($op, "/0"), "uid=x,dc=y");
    testing.assertEqual(asn1.asInt($op, "/1/0/0"), MOD_REPLACE);
    testing.assertEqual(asn1.asString($op, "/1/0/1/0"), "mail");
    testing.assertEqual(asn1.asString($op, "/1/0/1/1/0"), "x@y.org");
}

func testDeleteRequestEncoding() {
    def op as asn1.Value init encodeDeleteRequest("uid=x,dc=y");
    testing.assertEqual(asn1.tagClass($op), "application");
    testing.assertEqual(asn1.tagNumber($op), 10);
    testing.assertEqual(primStr($op), "uid=x,dc=y");
}

func testModifyDnEncoding() {
    def op as asn1.Value init encodeModifyDnRequest("uid=x,dc=y", "uid=z", true, "ou=new,dc=y");
    testing.assertEqual(asn1.tagNumber($op), 12);
    testing.assertEqual(asn1.asString($op, "/0"), "uid=x,dc=y");
    testing.assertEqual(asn1.asString($op, "/1"), "uid=z");
    testing.assertEqual(asn1.asBool($op, "/2"), true);
    testing.assertEqual(primStr(asn1.get($op, "/3")), "ou=new,dc=y");
}

func testPasswordModifyEncoding() {
    def op as asn1.Value init encodePasswordModifyRequest("uid=x,dc=y", "old", "new");
    testing.assertEqual(asn1.tagNumber($op), 23);
    testing.assertEqual(primStr(asn1.get($op, "/0")), PASSWD_MODIFY_OID);
    def reqValue as asn1.Value init asn1.decode(asn1.asBytes($op, "/1"));
    testing.assertEqual(primStr(asn1.get($reqValue, "/0")), "uid=x,dc=y");
    testing.assertEqual(primStr(asn1.get($reqValue, "/1")), "old");
    testing.assertEqual(primStr(asn1.get($reqValue, "/2")), "new");
}

# --- binary attribute values come back base64-encoded, never throwing ---

func testBinaryValueBase64() {
    def guid as bytes;
    $guid[] = 0xff;
    $guid[] = 0x00;
    $guid[] = 0xab;
    def node as asn1.Value init asn1.set([asn1.octetString($guid)]);
    testing.assertEqual(valueString($node, "/0"), encoding.toText($guid, "base64"));

    def utf8 as asn1.Value init asn1.set([
        asn1.octetString(convert.bytesFromString("hello", "utf-8"))
    ]);
    testing.assertEqual(valueString($utf8, "/0"), "hello");
}

func testSearchRequestRoundtrip() {
    def op as asn1.Value init encodeSearchRequest(
        "dc=x",
        SCOPE_SUB,
        parseFilter("(uid=alice)"),
        ["cn"]);
    def raw as bytes init encodeMessage(1, $op, []);
    def msg as asn1.Value init asn1.decode($raw);
    def got as asn1.Value init asn1.get($msg, "/1");
    testing.assertEqual(asn1.tagNumber($got), 3);
    testing.assertEqual(asn1.asString($got, "/0"), "dc=x");
    testing.assertEqual(asn1.asInt($got, "/1"), SCOPE_SUB);
    testing.assertEqual(asn1.tagNumber(asn1.get($got, "/6")), 3);
}

use task;

# --- full client <-> server round-trip over a loopback TCP socket ---
# ldap.j `use`s net + imports transport; the read-only directory server runs in
# a spawned task on a pre-bound listener (no bind race), and a real client
# connects, binds, and searches it.

func testClientServerRoundTrip() {
    def dir as Directory init directory([
        entry("uid=alice,ou=people,dc=example,dc=org", {
            "objectClass": ["person"], "uid": ["alice"], "cn": ["Alice"], "mail": ["alice@example.org"]
        }),
        entry("uid=bob,ou=people,dc=example,dc=org", {
            "objectClass": ["person"], "uid": ["bob"], "cn": ["Bob"]
        })
    ]);
    def listener as net.Listener init listen("127.0.0.1:0");
    def addr as string init net.address($listener);
    def server as task of null init spawn {
        serveOn($dir, $listener);
    };

    def conn as Conn init connect($addr, transport.Security.None);
    def br as Result init bind($conn, "", "");   # anonymous simple bind
    testing.assertEqual($br.code, SUCCESS);

    # A single-entry match by uid, with an attribute read.
    def found as list of Entry init search($conn, "dc=example,dc=org", SCOPE_SUB, parseFilter("(uid=alice)"), []);
    testing.assertEqual(len($found), 1);
    testing.assertEqual(firstValue($found[0], "mail"), "alice@example.org");

    # A filter matching both entries.
    def persons as list of Entry init search($conn, "dc=example,dc=org", SCOPE_SUB, parseFilter("(objectClass=person)"), []);
    testing.assertEqual(len($persons), 2);

    # A filter matching nothing.
    def none as list of Entry init search($conn, "dc=example,dc=org", SCOPE_SUB, parseFilter("(uid=carol)"), []);
    testing.assertEqual(len($none), 0);

    unbind($conn);
    net.close($listener);
    task.wait($server);
}

# Binding as a user with a hashed userPassword exercises the server's password
# verification, plus the INVALID_CREDENTIALS path on a wrong password.
func testPasswordBindRoundTrip() {
    def dir as Directory init directory([
        entry("uid=carol,ou=people,dc=example,dc=org", {
            "objectClass": ["person"], "uid": ["carol"],
            "userPassword": [password("s3cret", "ssha256")]
        })
    ]);
    def listener as net.Listener init listen("127.0.0.1:0");
    def addr as string init net.address($listener);
    def server as task of null init spawn {
        serveOn($dir, $listener);
    };

    def conn as Conn init connect($addr, transport.Security.None);
    def ok as Result init bind($conn, "uid=carol,ou=people,dc=example,dc=org", "s3cret");
    testing.assertEqual($ok.code, SUCCESS);
    def bad as Result init bind($conn, "uid=carol,ou=people,dc=example,dc=org", "wrong");
    testing.assertTrue($bad.code != SUCCESS);

    unbind($conn);
    net.close($listener);
    task.wait($server);
}
