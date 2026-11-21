# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
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
use encoding;
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
        def stored as string init password($scheme, "s3cret!");
        testing.assertEqual(verifyPassword($stored, "s3cret!"), true);
        testing.assertEqual(verifyPassword($stored, "wrong"), false);
    }
}

func testPbkdf2Format() {
    def stored as string init password("pbkdf2", "hunter2");
    testing.assertEqual(strings.startsWith($stored, '{PBKDF2-SHA256}$100000$'), true);
    # four $-separated fields after the prefix: "", iters, salt, hash
    def parts as list of string init strings.split($stored[15..], "$");
    testing.assertEqual(len($parts), 4);
    testing.assertEqual($parts[1], "100000");
    def s512 as string init password("pbkdf2-sha512", "hunter2");
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
    def good as string init password("pbkdf2", "s3cret");
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
    testing.assertEqual(valueString($node, "/0"), encoding.toText("base64", $guid));

    def utf8 as asn1.Value init asn1.set([asn1.octetString(encoding.encode("utf-8", "hello"))]);
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
        entry(
            "uid=alice,ou=people,dc=example,dc=org",
            {
                "objectClass": ["person"],
                "uid": ["alice"],
                "cn": ["Alice"],
                "mail": ["alice@example.org"]
            }),
        entry(
            "uid=bob,ou=people,dc=example,dc=org",
            {"objectClass": ["person"], "uid": ["bob"], "cn": ["Bob"]})
    ]);
    def listener as net.Listener init listen("127.0.0.1:0");
    def addr as string init net.address($listener);
    def server as task of null init spawn {
        serveOn($dir, $listener);
    };

    def conn as Conn init connect($addr, transport.Security.None);
    def br as Result init bind($conn, "", ""); # anonymous simple bind
    testing.assertEqual($br.code, SUCCESS);

    # A single-entry match by uid, with an attribute read.
    def found as list of Entry init search(
        $conn,
        "dc=example,dc=org",
        SCOPE_SUB,
        parseFilter("(uid=alice)"),
        []);
    testing.assertEqual(len($found), 1);
    testing.assertEqual(firstValue($found[0], "mail"), "alice@example.org");

    # A filter matching both entries.
    def persons as list of Entry init search(
        $conn,
        "dc=example,dc=org",
        SCOPE_SUB,
        parseFilter("(objectClass=person)"),
        []);
    testing.assertEqual(len($persons), 2);

    # A filter matching nothing.
    def none as list of Entry init search(
        $conn,
        "dc=example,dc=org",
        SCOPE_SUB,
        parseFilter("(uid=carol)"),
        []);
    testing.assertEqual(len($none), 0);

    unbind($conn);
    net.close($listener);
    task.wait($server);
}

# Binding as a user with a hashed userPassword exercises the server's password
# verification, plus the INVALID_CREDENTIALS path on a wrong password.
func testPasswordBindRoundTrip() {
    def dir as Directory init directory([
        entry(
            "uid=carol,ou=people,dc=example,dc=org",
            {
                "objectClass": ["person"],
                "uid": ["carol"],
                "userPassword": [password("ssha256", "s3cret")]
            })
    ]);
    def listener as net.Listener init listen("127.0.0.1:0");
    def addr as string init net.address($listener);
    def server as task of null init spawn {
        serveOn($dir, $listener);
    };

    def conn as Conn init connect($addr, transport.Security.None);
    $conn.allowInsecureAuth = true; # loopback test server, no TLS
    def ok as Result init bind($conn, "uid=carol,ou=people,dc=example,dc=org", "s3cret");
    testing.assertEqual($ok.code, SUCCESS);
    def bad as Result init bind($conn, "uid=carol,ou=people,dc=example,dc=org", "wrong");
    testing.assertTrue($bad.code != SUCCESS);

    unbind($conn);
    net.close($listener);
    task.wait($server);
}

# A DN with an empty password is an RFC 4513 unauthenticated bind: refused
# before it is sent. An anonymous bind (both empty) still goes through.
func testUnauthenticatedBindRefused() {
    def dir as Directory init directory([]);
    def listener as net.Listener init listen("127.0.0.1:0");
    def addr as string init net.address($listener);
    def server as task of null init spawn {
        serveOn($dir, $listener);
    };
    def conn as Conn init connect($addr, transport.Security.None);
    try {
        bind($conn, "uid=carol,ou=people,dc=example,dc=org", "");
        throw Error{kind: "missed", message: "an unauthenticated bind"};
    } catch (e) {
        testing.assertEqual($e.kind, "ldap");
        testing.assertEqual($e.code, 0);
    }
    testing.assertEqual(bind($conn, "", "").code, SUCCESS);
    unbind($conn);
    net.close($listener);
    task.wait($server);
}

# RFC 4515 escapes: escapeFilterValue round-trips through parseFilter, and an
# escaped star is a literal, not a wildcard.
func testFilterEscapes() {
    def v as string init "a*b(c)\\d";
    def esc as string init escapeFilterValue($v);
    testing.assertEqual($esc, "a\\2ab\\28c\\29\\5cd");
    testing.assertEqual(
        asn1.encode(parseFilter("(cn=" + $esc + ")")),
        asn1.encode(equals("cn", $v)));
    testing.assertEqual(
        asn1.encode(parseFilter("(cn=x\\2a*)")),
        asn1.encode(substrings("cn", "x*", [], "")));
}

func deepFilter() {
    return parseFilter(strings.replicate("(&", 100) + "(a=1)" + strings.replicate(")", 100));
}
func badEscape() {
    return parseFilter("(cn=a\\zz)");
}
func testFilterLimits() {
    testing.assertThrows("deepFilter", "ldap");
    testing.assertThrows("badEscape", "ldap");
    # 64 levels still parse.
    parseFilter(strings.replicate("(&", 63) + "(a=1)" + strings.replicate(")", 63));
}

# A failure the server answered carries its result code, matched DN and
# diagnostic; a local one has code 0.
func testFailuresAreLdapErrors() {
    try {
        failResult(
            "search failed",
            Result{code: 32, matchedDn: "dc=example,dc=org", message: "no such object"});
        throw Error{kind: "missed", message: "a failed search"};
    } catch (e) {
        testing.assertEqual($e.kind, "ldap");
        testing.assertEqual($e.code, 32);
        testing.assertEqual($e.matchedDn, "dc=example,dc=org");
        testing.assertEqual($e.diagnostic, "no such object");
        testing.assertEqual($e.message, "ldap: search failed: code 32 (no such object)");
    }
    try {
        parseFilter("(uid=ada");
        throw Error{kind: "missed", message: "a malformed filter"};
    } catch (e) {
        testing.assertEqual($e.kind, "ldap");
        testing.assertEqual($e.code, 0);
    }
    testing.assertThrows("malformedFilter", "ldap");
}

func malformedFilter() {
    parseFilter("uid=ada)");
}

# --- userPassword is never matched, filters are three-valued ---

func secretEntry() {
    return entry(
        "uid=bob,ou=people,dc=example,dc=org",
        {"uid": ["bob"], "userPassword": ["hunter2"]});
}

# A filter item on userPassword is UNDEFINED, so no probe tells anything about
# the stored value: not equality, substrings, presence, ordering, nor a NOT
# around them (NOT UNDEFINED stays UNDEFINED).
func testUserPasswordIsNeverMatched() {
    def e as Entry init secretEntry();
    for (def f in [
        "(userPassword=hunter2)",
        "(userPassword=h*)",
        "(userPassword=*)",
        "(userPassword>=a)",
        "(userPassword<=z)",
        "(userPassword~=hunter2)",
        "(USERPASSWORD=hunter2)",
        "(userPassword;binary=hunter2)",
        "(2.5.4.35=hunter2)",
        "(!(userPassword=wrong))",
        "(!(userPassword=hunter2))",
        "(&(uid=bob)(userPassword=*))"
    ]) {
        testing.assertFalse(evalFilter(parseFilter($f), $e));
    }
    # an OR still matches on its other branch; an AND with a FALSE branch is FALSE
    testing.assertTrue(evalFilter(parseFilter("(|(uid=bob)(userPassword=x))"), $e));
    testing.assertTrue(evalFilter(parseFilter("(!(&(uid=carol)(userPassword=x)))"), $e));
}

# --- strict filter strings ---

func filterTrailingGarbage() {
    return parseFilter("(uid=alice)GARBAGE(");
}
func filterTwoFilters() {
    return parseFilter("(uid=a)(uid=b)");
}
func filterOpenParenInValue() {
    return parseFilter("(cn=a(b)");
}
func filterNoAttribute() {
    return parseFilter("(=x)");
}
func testFilterMustBeWhole() {
    testing.assertThrows("filterTrailingGarbage", "ldap");
    testing.assertThrows("filterTwoFilters", "ldap");
    testing.assertThrows("filterOpenParenInValue", "ldap");
    testing.assertThrows("filterNoAttribute", "ldap");
    testing.assertEqual(asn1.tagNumber(parseFilter("(cn=a\\28b)")), 3);
}

# --- the directory is safe for concurrent edits ---

func testConcurrentDirectoryEdits() {
    def dir as Directory init directory([entry("uid=x,dc=x", {"uid": ["x"]})]);
    def ts as list of task of null;
    for (def w in 0..4) {
        $ts[] = spawn {
            for (def i in 0..25) {
                addEntry($dir, entry("uid=u{$w}x{$i},dc=x", {"uid": ["u{$w}x{$i}"]}));
                setAttribute($dir, "uid=x,dc=x", "a{$w}x{$i}", ["v"]);
            }
            return;
        };
    }
    for (def t in $ts) {
        task.wait($t);
    }
    testing.assertEqual(len(listEntries($dir)), 101);
    testing.assertEqual(len(getEntry($dir, "uid=x,dc=x").attributes), 101);
}

# --- client: credentials, malformed replies, read-only server ---

func startServer(dir as Directory, listener as net.Listener) {
    serveOn($dir, $listener);
}

# A password is not sent over Security.None unless allowInsecureAuth is set;
# an anonymous bind still is.
func testInsecureBindRefused() {
    def dir as Directory init directory([
        entry("uid=carol,dc=x", {"uid": ["carol"], "userPassword": [password("plain", "pw")]})
    ]);
    def listener as net.Listener init listen("127.0.0.1:0");
    def server as task of null init spawn {
        startServer($dir, $listener);
    };
    def conn as Conn init connect(net.address($listener), transport.Security.None);
    testing.assertFalse($conn.allowInsecureAuth);
    testing.assertTrue($conn.security == transport.Security.None);
    for (def step in ["bind", "passwordModify"]) {
        def msg as string init "";
        try {
            if ($step == "bind") {
                bind($conn, "uid=carol,dc=x", "pw");
            } else {
                passwordModify($conn, "", "pw", "new");
            }
            throw Error{kind: "missed", message: "a password over plaintext"};
        } catch (e) {
            testing.assertEqual($e.kind, "ldap");
            $msg = $e.message;
        }
        testing.assertContains($msg, "allowInsecureAuth");
    }
    testing.assertEqual(bind($conn, "", "").code, SUCCESS);
    $conn.allowInsecureAuth = true;
    testing.assertEqual(bind($conn, "uid=carol,dc=x", "pw").code, SUCCESS);
    unbind($conn);
    net.close($listener);
    task.wait($server);
}

# serveJunk answers each request on one connection with `reply`.
func serveJunk(listener as net.Listener, reply as bytes, count as int) {
    def s as net.Conn init net.accept($listener);
    for (def i in 0..$count) {
        net.readBytes($s, 4096);
        net.writeBytes($s, $reply);
    }
    net.readBytes($s, 1);
    net.close($s);
}

# A reply that frames correctly but is no LDAPMessage is an LdapError, not an
# asn1 runtime error, for bind, a write and a search.
func testMalformedReplyIsLdapError() {
    def junk as bytes;
    for (def x in [0x30, 3, 0x01, 1, 0xff]) { # SEQUENCE { BOOLEAN }
        $junk[] = $x;
    }
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def server as task of null init spawn {
        serveJunk($listener, $junk, 3);
    };
    def conn as Conn init connect(net.address($listener), transport.Security.None);
    $conn.allowInsecureAuth = true;
    for (def step in ["bind", "delete", "search"]) {
        def kind as string init "";
        try {
            if ($step == "bind") {
                bind($conn, "cn=x", "pw");
            } elseif ($step == "delete") {
                delete($conn, "cn=x");
            } else {
                search($conn, "", SCOPE_SUB, parseFilter("(cn=*)"), []);
            }
        } catch (e) {
            $kind = $e.kind;
        }
        testing.assertEqual($kind, "ldap");
    }
    close($conn);
    task.wait($server);
    net.close($listener);
}

# The server is read-only on the wire: add / modify / delete / modifyDN get
# unwillingToPerform (53), and the connection stays usable.
func testWriteOpsAreRefusedNotDropped() {
    def dir as Directory init directory([entry("uid=a,dc=x", {"uid": ["a"]})]);
    def listener as net.Listener init listen("127.0.0.1:0");
    def server as task of null init spawn {
        startServer($dir, $listener);
    };
    def conn as Conn init connect(net.address($listener), transport.Security.None);
    testing.assertEqual(add($conn, "uid=b,dc=x", {"uid": ["b"]}).code, 53);
    testing.assertEqual(modify($conn, "uid=a,dc=x", [change(MOD_REPLACE, "uid", ["z"])]).code, 53);
    testing.assertEqual(delete($conn, "uid=a,dc=x").code, 53);
    testing.assertEqual(modifyDn($conn, "uid=a,dc=x", "uid=c", true, "").code, 53);
    testing.assertEqual(len(search($conn, "dc=x", SCOPE_SUB, parseFilter("(uid=a)"), [])), 1);
    unbind($conn);
    net.close($listener);
    task.wait($server);
}

# A search's replies are capped as a whole (transport.MAX_RECEIVED_BYTES for
# search / searchPaged; a small cap here): past it, kind "limit".
func testSearchBytesAreCapped() {
    def entries as list of Entry;
    for (def i in 0..20) {
        $entries[] = entry(
            "uid=u{$i},dc=x",
            {"uid": ["u{$i}"], "cn": [strings.replicate("x", 100)]});
    }
    def dir as Directory init directory($entries);
    def listener as net.Listener init listen("127.0.0.1:0");
    def server as task of null init spawn {
        startServer($dir, $listener);
    };
    def conn as Conn init connect(net.address($listener), transport.Security.None);
    def id as int init nextId();
    sendOp($conn, $id, encodeSearchRequest("dc=x", SCOPE_SUB, parseFilter("(uid=*)"), []), []);
    def kind as string init "";
    try {
        readEntries($conn, $id, 0, 1000);
    } catch (e) {
        $kind = $e.kind;
    }
    testing.assertEqual($kind, "limit");
    close($conn);
    def again as Conn init connect(net.address($listener), transport.Security.None);
    testing.assertEqual(len(search($again, "dc=x", SCOPE_SUB, parseFilter("(uid=*)"), [])), 20);
    unbind($again);
    net.close($listener);
    task.wait($server);
}

# --- TLS options reach net (connectWith / startTlsWith) ---
# net has no TLS listener for a .j test, so these prove the options are passed
# on: a caCert that is not a PEM certificate makes net refuse the handshake setup,
# which only happens when the options reach net.connectTLS / net.startTLS.

def const NOT_A_PEM as string init "not a certificate";

func badCaOptions() {
    def t as net.TLSOptions;
    $t.caCert = encoding.encode("utf-8", NOT_A_PEM);
    return $t;
}

# tlsSetupFailed reports whether `err` is net's refusal of the caCert.
func tlsSetupFailed(err as string) {
    return strings.contains($err, "caCert");
}

func testConnectWithPassesCaCert() {
    # Nothing listens: the invalid caCert is refused before any dial.
    def err as string init "";
    try {
        connectWith("127.0.0.1:1", transport.Security.Tls, badCaOptions());
    } catch (e) {
        $err = $e.message;
    }
    testing.assertTrue(tlsSetupFailed($err));
}

# fakeStartTls answers one StartTLS request with success and closes.
func fakeStartTls(listener as net.Listener) {
    def h as net.Conn init net.accept($listener);
    def c as Conn init Conn{
        handle: $h,
        timeoutMs: 5000,
        security: transport.Security.None,
        allowInsecureAuth: false
    };
    def req as asn1.Value init readMessage($c);
    net.writeBytes($h, encodeExtResponse(asn1.asInt($req, "/0"), SUCCESS));
    net.close($h);
}

func testStartTlsWithPassesCaCert() {
    def listener as net.Listener init listen("127.0.0.1:0");
    def addr as string init net.address($listener);
    def server as task of null init spawn {
        fakeStartTls($listener);
    };
    def err as string init "";
    try {
        connectWith($addr, transport.Security.Starttls, badCaOptions());
    } catch (e) {
        $err = $e.message;
    }
    # Closing the listener first ends a server still parked in accept (the
    # client failed before connecting), so the wait cannot hang.
    net.close($listener);
    try {
        task.wait($server);
    } catch (e) { # lint-disable: L103
        # the server's own failure (accept on the closed listener) is not under test
    }
    testing.assertTrue(tlsSetupFailed($err));
}
