# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# soap_test.j - white-box tests for soap.j. Run with:
#
#     jennifer test modules/soap_test.j
#
# The overlay splices soap.j in first, so it reaches the private helpers by
# bare identifier. The protocol layer is tested on its own (envelopes and
# faults of both versions, against .NET- and Java-style replies); the client
# runs against an `httpd` server on a loopback port that answers canned
# replies in order and hands back what it received; `handle` is tested
# directly and end to end behind `httpd`.
use httpd;
use task;
use testing;
use strings;
use convert;
use time;
use net;
use hash;
use encoding;

# A canned reply.
def struct Canned {
    status as int,
    headers as map of string to string,
    body as string
};

# One request as the fake server saw it.
def struct Seen {
    method as string,
    headers as map of string to string,
    body as string
};

# A running fake server.
def struct Fake {
    srv as httpd.Server,
    url as string,
    t as task of list of Seen
};

# The request headers the tests look at.
def const WATCHED as list of string init ["SOAPAction", "Content-Type", "Authorization", "X-Extra"];

def const TEMPURI as string init "http://tempuri.org/";

def const ROLE_NONE as string init "http://www.w3.org/2003/05/soap-envelope/role/none";

func serve(srv as httpd.Server, replies as list of Canned) {
    def seen as list of Seen init [];
    for (def r in $replies) {
        def req as httpd.Request init httpd.accept($srv);
        def headers as map of string to string init {};
        for (def name in WATCHED) {
            def value as string init httpd.header($req, $name);
            if ($value != "") {
                $headers[$name] = $value;
            }
        }
        $seen[] = Seen{
            method: httpd.method($req),
            headers: $headers,
            body: encoding.decode("utf-8", httpd.body($req))
        };
        for (def name in maps.keys($r.headers)) {
            httpd.setHeader($req, $name, $r.headers[$name]);
        }
        httpd.respond($req, $r.status, $r.body);
    }
    return $seen;
}

func fake(replies as list of Canned) {
    def srv as httpd.Server init httpd.listen("127.0.0.1:0");
    def t as task of list of Seen init spawn {
        return serve($srv, $replies);
    };
    return Fake{srv: $srv, url: "http://" + httpd.address($srv) + "/svc.asmx", t: $t};
}

func finish(f as Fake) {
    def seen as list of Seen init task.wait($f.t);
    httpd.shutdown($f.srv);
    return $seen;
}

func canned(status as int, body as string) {
    return Canned{
        status: $status,
        headers: {"Content-Type": "text/xml; charset=utf-8"},
        body: $body
    };
}

func missed() {
    throw Error{kind: "test", message: "the call did not fail"};
}

func messageOf(e as SoapError) {
    testing.assertEqual($e.kind, ERROR_KIND);
    return $e.message;
}

func addOp() {
    return operation(TEMPURI, "Add", {"intA": "2", "intB": "3"}, true);
}

# What an ASMX service answers `Add`.
def const ADD_REPLY as string init '<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"
  xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <soap:Body>
    <AddResponse xmlns="http://tempuri.org/"><AddResult>5</AddResult></AddResponse>
  </soap:Body>
</soap:Envelope>';

# A .NET SOAP 1.1 fault: prefixed faultcode, text detail.
def const DOTNET_FAULT as string init '<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/">
  <soap:Body>
    <soap:Fault>
      <faultcode>soap:Client</faultcode>
      <faultstring>Server was unable to read request. ---&gt; bad intA</faultstring>
      <detail />
    </soap:Fault>
  </soap:Body>
</soap:Envelope>';

# A SOAP 1.2 fault with a subcode, a role and an element detail.
def const FAULT12 as string init '<env:Envelope xmlns:env="http://www.w3.org/2003/05/soap-envelope"
  xmlns:rpc="http://www.w3.org/2003/05/soap-rpc">
  <env:Body>
    <env:Fault>
      <env:Code>
        <env:Value>env:Sender</env:Value>
        <env:Subcode><env:Value>rpc:BadArguments</env:Value></env:Subcode>
      </env:Code>
      <env:Reason><env:Text xml:lang="en-US">Processing error</env:Text></env:Reason>
      <env:Role>http://example.org/gateway</env:Role>
      <env:Detail>
        <e:myFaultDetails xmlns:e="http://example.org/faults">42</e:myFaultDetails>
      </env:Detail>
    </env:Fault>
  </env:Body>
</env:Envelope>';

# --- protocol layer --------------------------------------------------------

func testEnvelopeRoundTrip11() {
    def block as xml.Value init xml.setText(xml.elementNS("urn:auth", "a:Token"), "t-1");
    def env as Envelope init parse(envelope("1.1", [$block], [addOp()]));
    testing.assertEqual($env.version, "1.1");
    testing.assertEqual(len($env.headers), 1);
    testing.assertEqual(xml.namespace($env.headers[0]), "urn:auth");
    testing.assertEqual(xml.text($env.headers[0]), "t-1");
    testing.assertEqual(len($env.body), 1);
    testing.assertEqual(xml.localName($env.body[0]), "Add");
    testing.assertEqual(xml.namespace($env.body[0]), TEMPURI);
    testing.assertEqual(values($env.body[0]), {"intA": "2", "intB": "3"});
    testing.assertFalse(isFault($env));
}

func testEnvelopeRoundTrip12() {
    def text as string init envelope("1.2", [], [operation("urn:x", "Ping", {}, false)]);
    testing.assertContains($text, SOAP12_NS);
    testing.assertFalse(strings.contains($text, "Header"));
    def env as Envelope init parse($text);
    testing.assertEqual($env.version, "1.2");
    testing.assertEqual(len($env.headers), 0);
    testing.assertEqual(xml.localName($env.body[0]), "Ping");
}

func testEnvelopeUnknownVersion() {
    try {
        envelope("1.3", [], []);
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "1.1 or 1.2");
    }
}

func testParseRefusesDoctype() {
    def texts as list of string init [
        "<!DOCTYPE x [<!ENTITY a \"b\">]><soap:Envelope xmlns:soap=\"" + SOAP11_NS +
            "\"><soap:Body/></soap:Envelope>",
        "<?xml version=\"1.0\"?>\n<!-- c --><!DOCTYPE soap:Envelope><soap:Envelope xmlns:soap=\"" +
            SOAP11_NS + "\"><soap:Body/></soap:Envelope>",
        # the first "<!DOCTYPE" is in a comment, the real one follows it
        "<!-- <!DOCTYPE --><!DOCTYPE x [<!ENTITY a \"b\">]><soap:Envelope xmlns:soap=\"" +
            SOAP11_NS + "\"><soap:Body/></soap:Envelope>"
    ];
    for (def t in $texts) {
        try {
            parse($t);
            missed();
        } catch (e) {
            testing.assertContains(messageOf($e), "DOCTYPE");
        }
    }
}

func testParseDoctypeTextInBodyIsData() {
    def text as string init "<soap:Envelope xmlns:soap=\"" + SOAP11_NS +
        "\"><soap:Body><m:Echo xmlns:m=\"urn:x\"><![CDATA[<!DOCTYPE html>]]></m:Echo>" +
        "</soap:Body></soap:Envelope>";
    def env as Envelope init parse($text);
    testing.assertEqual(xml.text($env.body[0]), "<!DOCTYPE html>");
}

func testParseRefusesNonEnvelopes() {
    def cases as map of string to string init {
        "not XML": "[json]",
        "not a SOAP 1.1 or 1.2 Envelope": "<Envelope xmlns=\"urn:other\"><Body/></Envelope>",
        "exactly one Body, got 0": "<s:Envelope xmlns:s=\"" + SOAP11_NS +
            "\"><s:Header/></s:Envelope>",
        "exactly one Body, got 2": "<s:Envelope xmlns:s=\"" + SOAP11_NS +
            "\"><s:Body/><s:Body/></s:Envelope>",
        "unexpected element": "<s:Envelope xmlns:s=\"" + SOAP12_NS + "\"><s:Body/><x/></s:Envelope>"
    };
    for (def want in $cases) {
        try {
            parse($cases[$want]);
            missed();
        } catch (e) {
            testing.assertContains(messageOf($e), $want);
        }
    }
}

func testParse11AllowsElementsAfterBody() {
    def env as Envelope init parse("<s:Envelope xmlns:s=\"" + SOAP11_NS +
        "\"><s:Body/><x/></s:Envelope>");
    testing.assertEqual(len($env.body), 0);
}

func testParseFaultDotNet() {
    def env as Envelope init parse(DOTNET_FAULT);
    testing.assertTrue(isFault($env));
    def f as Fault init parseFault($env);
    testing.assertEqual($f.code, "Client");
    testing.assertEqual($f.reason, "Server was unable to read request. ---> bad intA");
    testing.assertEqual($f.detail, "");
}

func testParseFault11AppCode() {
    def text as string init "<s:Envelope xmlns:s=\"" + SOAP11_NS + "\"><s:Body><s:Fault>" +
        "<faultcode xmlns:q=\"urn:q\">q:Busy</faultcode><faultstring>later</faultstring>" +
        "<faultactor>urn:gw</faultactor><detail><q:info xmlns:q=\"urn:q\">x</q:info></detail>" +
        "</s:Fault></s:Body></s:Envelope>";
    def f as Fault init parseFault(parse($text));
    testing.assertEqual($f.code, "\{urn:q\}Busy");
    testing.assertEqual($f.actor, "urn:gw");
    testing.assertEqual($f.detail, "<q:info xmlns:q=\"urn:q\">x</q:info>");
}

func testParseFault11UnboundPrefixKept() {
    def text as string init "<s:Envelope xmlns:s=\"" + SOAP11_NS + "\"><s:Body><s:Fault>" +
        "<faultcode>zz:Odd</faultcode><faultstring>r</faultstring></s:Fault></s:Body></s:Envelope>";
    testing.assertEqual(parseFault(parse($text)).code, "zz:Odd");
}

func testParseFault12() {
    def f as Fault init parseFault(parse(FAULT12));
    testing.assertEqual($f.code, "Sender");
    testing.assertEqual($f.subcode, "\{http://www.w3.org/2003/05/soap-rpc\}BadArguments");
    testing.assertEqual($f.reason, "Processing error");
    testing.assertEqual($f.actor, "http://example.org/gateway");
    testing.assertContains($f.detail, "myFaultDetails");
}

func testParseFaultNotAFault() {
    try {
        parseFault(parse(ADD_REPLY));
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "not a fault");
    }
}

func testFaultEnvelopeMapsCodes() {
    def f as Fault init Fault{code: "Sender", subcode: "", reason: "bad", actor: "", detail: ""};
    testing.assertContains(faultEnvelope("1.1", $f), "<faultcode>soap:Client</faultcode>");
    $f.code = "Server";
    testing.assertContains(faultEnvelope("1.2", $f), "<soap:Value>soap:Receiver</soap:Value>");
}

func testFaultEnvelopeRoundTrip() {
    def f as Fault init Fault{
        code: "\{urn:app\}Busy",
        subcode: "",
        reason: "try <later> & again",
        actor: "urn:me",
        detail: "<e:why xmlns:e=\"urn:e\">x</e:why>"
    };
    testing.assertEqual(parseFault(parse(faultEnvelope("1.1", $f))), $f);
    $f.code = "Receiver";
    $f.subcode = "\{urn:app\}Busy";
    testing.assertEqual(parseFault(parse(faultEnvelope("1.2", $f))), $f);
    $f.detail = "plain words";
    testing.assertEqual(parseFault(parse(faultEnvelope("1.2", $f))).detail, "plain words");
}

func testFaultElementRefusesBadCode() {
    for (def code in ["", "a b", "\{\}x", "\{urn:x\}", "\{urn:x\}1a"]) {
        try {
            faultElement("1.1", Fault{code: $code, subcode: "", reason: "", actor: "", detail: ""});
            missed();
        } catch (e) {
            testing.assertContains(messageOf($e), "fault code");
        }
    }
}

func testOperationUnqualified() {
    def params as map of string to string init {"id": "7", "kind": "a<b"};
    def text as string init xml.encode(operation("urn:x", "Get", $params, false));
    testing.assertEqual($text, "<m:Get xmlns:m=\"urn:x\"><id>7</id><kind>a&lt;b</kind></m:Get>");
}

func testOperationRefusesBadNames() {
    for (def name in ["", "a:b", "1x", "a b"]) {
        try {
            operation("urn:x", $name, {}, false);
            missed();
        } catch (e) {
            testing.assertContains(messageOf($e), "XML name");
        }
        try {
            operation("urn:x", "Op", {$name: "v"}, false);
            missed();
        } catch (e) {
            testing.assertContains(messageOf($e), "XML name");
        }
    }
}

func testValuesNestedAndRepeated() {
    def node as xml.Value init xml.decode("<r><a>1</a><a>2</a><b><c>x</c></b><d/></r>");
    testing.assertEqual(values($node), {"a": "1", "b": "<c>x</c>", "d": ""});
}

func testContentType() {
    testing.assertEqual(contentType("1.1", "urn:a"), "text/xml; charset=utf-8");
    testing.assertEqual(contentType("1.2", ""), "application/soap+xml; charset=utf-8");
    testing.assertEqual(
        contentType("1.2", "urn:a"),
        "application/soap+xml; charset=utf-8; action=\"urn:a\"");
}

func testRequestAction() {
    testing.assertEqual(
        requestAction("text/xml", "\"http://tempuri.org/Add\""),
        "http://tempuri.org/Add");
    testing.assertEqual(requestAction("text/xml", "urn:bare"), "urn:bare");
    testing.assertEqual(
        requestAction("application/soap+xml; action=\"urn:a\"; charset=utf-8", ""),
        "urn:a");
    testing.assertEqual(requestAction("application/soap+xml;Action=urn:b", ""), "urn:b");
    testing.assertEqual(requestAction("text/xml", "\"\""), "");
    testing.assertEqual(requestAction("", ""), "");
}

# --- client ----------------------------------------------------------------

func testConnectRefusesBadUrls() {
    for (def url in ["ftp://h/x", "https://u:p@h/x", "https://h/x#f", "h/x", "https:///x"]) {
        try {
            connect($url);
            missed();
        } catch (e) {
            testing.assertContains(messageOf($e), "endpoint");
        }
    }
    testing.assertEqual(connect(" https://h:8443/svc?x=1 ").endpoint, "https://h:8443/svc?x=1");
}

func testClientOptions() {
    def c as Client init connect("https://h/svc");
    testing.assertEqual($c.version, "1.1");
    $c = withVersion($c, "1.2");
    $c = withTimeout(withMaxBytes($c, 0), -5);
    testing.assertEqual($c.timeoutMs, DEFAULT_TIMEOUT_MS);
    testing.assertEqual($c.maxBytes, DEFAULT_MAX_BYTES);
    $c = withToken(withSkipVerify($c), "abc");
    testing.assertTrue($c.tls.skipVerify);
    testing.assertEqual($c.headers["Authorization"], "Bearer abc");
    for (def name in ["Content-Type", "SOAPAction", "Host", "content-length"]) {
        try {
            withHeader($c, $name, "x");
            missed();
        } catch (e) {
            testing.assertContains(messageOf($e), "sets");
        }
    }
    try {
        withHeader($c, "X-A", "a\r\nX-B: b");
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "one-line");
    }
    try {
        withVersion($c, "2.0");
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "1.1 or 1.2");
    }
}

func testCall11() {
    def f as Fake init fake([canned(200, ADD_REPLY)]);
    def c as Client init withHeader(withBasicAuth(connect($f.url), "u", "p"), "X-Extra", "1");
    $c = withSoapHeader($c, xml.setText(xml.elementNS("urn:auth", "a:Token"), "t"));
    def result as xml.Value init call($c, "http://tempuri.org/Add", addOp());
    testing.assertEqual(values($result)["AddResult"], "5");
    def seen as list of Seen init finish($f);
    testing.assertEqual($seen[0].method, "POST");
    testing.assertEqual($seen[0].headers["SOAPAction"], "\"http://tempuri.org/Add\"");
    testing.assertEqual($seen[0].headers["Content-Type"], "text/xml; charset=utf-8");
    testing.assertEqual($seen[0].headers["Authorization"], http.basic("u", "p"));
    testing.assertEqual($seen[0].headers["X-Extra"], "1");
    def sent as Envelope init parse($seen[0].body);
    testing.assertEqual(xml.text($sent.headers[0]), "t");
    testing.assertEqual(values($sent.body[0]), {"intA": "2", "intB": "3"});
}

func testCall12CarriesActionInContentType() {
    def reply as string init envelope("1.2", [], [xml.elementNS("urn:x", "m:PingResponse")]);
    def f as Fake init fake([canned(200, $reply)]);
    def c as Client init withVersion(connect($f.url), "1.2");
    testing.assertEqual(
        xml.localName(call($c, "urn:x#Ping", operation("urn:x", "Ping", {}, false))),
        "PingResponse");
    def seen as list of Seen init finish($f);
    testing.assertFalse(maps.has($seen[0].headers, "SOAPAction"));
    testing.assertEqual(
        $seen[0].headers["Content-Type"],
        "application/soap+xml; charset=utf-8; action=\"urn:x#Ping\"");
}

func testCallFaultThrows() {
    def f as Fake init fake([canned(500, DOTNET_FAULT), canned(500, DOTNET_FAULT)]);
    def c as Client init connect($f.url);
    try {
        call($c, "", addOp());
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "fault Client: Server was unable to read request");
    }
    def r as Reply init send($c, "", addOp());
    testing.assertTrue($r.isFault);
    testing.assertEqual($r.status, 500);
    testing.assertEqual($r.fault.code, "Client");
    finish($f);
}

func testCallFaultMessageIsPlain() {
    def evil as Fault init Fault{
        code: "Server",
        subcode: "",
        reason: "a\u001b[2Jb\nc",
        actor: "",
        detail: ""
    };
    def f as Fake init fake([canned(500, faultEnvelope("1.1", $evil))]);
    try {
        call(connect($f.url), "", addOp());
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "fault Server: a?[2Jb?c");
    }
    finish($f);
}

func testSendNonSoapError() {
    def f as Fake init fake([
        Canned{
            status: 503,
            headers: {"Content-Type": "text/html"},
            body: "<html><body>down</body></html>"
        },
        Canned{status: 502, headers: {}, body: ""},
        Canned{status: 200, headers: {}, body: "<html/>"}
    ]);
    def c as Client init connect($f.url);
    def wants as list of string init [
        "HTTP 503 Service Unavailable without a SOAP reply",
        "HTTP 502 Bad Gateway",
        "not a SOAP 1.1 or 1.2 Envelope"
    ];
    for (def want in $wants) {
        try {
            send($c, "", addOp());
            missed();
        } catch (e) {
            testing.assertContains(messageOf($e), $want);
        }
    }
    finish($f);
}

func testSendOneWay() {
    def f as Fake init fake([Canned{status: 202, headers: {}, body: ""}]);
    def r as Reply init send(connect($f.url), "urn:notify", addOp());
    testing.assertEqual($r.status, 202);
    testing.assertFalse($r.isFault);
    testing.assertEqual(len($r.envelope.body), 0);
    finish($f);
}

func testSendVersionMismatchReply() {
    def f as Fake init fake([canned(200, ADD_REPLY)]);
    try {
        send(withVersion(connect($f.url), "1.2"), "", addOp());
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "SOAP 1.2 call got a SOAP 1.1 reply");
    }
    finish($f);
}

func testSendRespectsMaxBytes() {
    def big as string init envelope(
        "1.1",
        [],
        [xml.setText(xml.elementNS("urn:x", "m:R"), strings.replicate("x", 4096))]);
    def f as Fake init fake([canned(200, $big)]);
    try {
        send(withMaxBytes(connect($f.url), 1024), "", addOp());
        missed();
    } catch (e) {
        # a reply over maxBytes is the generic kind "limit", not a SoapError
        testing.assertEqual($e.kind, "limit");
        testing.assertContains($e.message, "soap: POST");
    }
    finish($f);
}

/**
 * Accept one request and answer it as told: "silent" waits without a word
 * until the client gives up, "short" promises more body than it sends and
 * closes.
 *
 * @param l {net.Listener} the listener
 * @param how {string} "silent" or "short"
 * @return {int} 0
 */
func misbehave(l as net.Listener, how as string) {
    def conn as net.Conn init net.accept($l);
    net.readBytes($conn, 65536);
    if ($how == "silent") {
        time.sleep(time.fromMilliseconds(1500));
    } else {
        net.writeBytes(
            $conn,
            encoding.encode(
                "utf-8",
                "HTTP/1.1 200 OK\r\nContent-Type: text/xml\r\nContent-Length: 1000\r\n\r\n<soap"));
    }
    net.close($conn);
    return 0;
}

# A deadline and a connection closed under the call keep their generic kinds,
# as "limit" does, so a caller can tell them from a SOAP failure.
func testSendKeepsTimeoutAndClosed() {
    for (def c in [["silent", "timeout"], ["short", "closed"]]) {
        def l as net.Listener init net.listen("127.0.0.1:0");
        def how as string init $c[0];
        def t as task of int init spawn {
            return misbehave($l, $how);
        };
        try {
            send(
                withTimeout(connect("http://" + net.address($l) + "/svc?key=s3cr3t"), 300),
                "",
                addOp());
            missed();
        } catch (e) {
            testing.assertEqual($e.kind, $c[1]);
            testing.assertContains($e.message, "soap: POST ");
            # the endpoint's query may carry a key: it stays out of the message
            testing.assertFalse(strings.contains($e.message, "s3cr3t"));
        }
        task.wait($t);
        net.close($l);
    }
}

# A socket failure (a refused connection, a reset before the reply) is http's
# net.NetError (kind "net"), passed on as it is, not a SoapError.
func testSendPassesASocketFailureOn() {
    def l as net.Listener init net.listen("127.0.0.1:0");
    def addr as string init net.address($l);
    def t as task of null init spawn {
        def conn as net.Conn init net.accept($l);
        net.readBytes($conn, 4096);
        net.abort($conn);
    };
    try {
        send(connect("http://" + $addr + "/svc"), "", addOp());
        missed();
    } catch (e) {
        testing.assertEqual($e.kind, "net");
        testing.assertEqual($e.reason, "reset");
    }
    task.wait($t);
    net.close($l);
    try {
        send(connect("http://" + $addr + "/svc"), "", addOp());
        missed();
    } catch (e) {
        testing.assertEqual($e.kind, "net");
        testing.assertEqual($e.reason, "refused");
    }
}

# A reply past xml's nesting cap is kind "limit", not a SoapError, and so is a
# fault detail past it.
func testDecodingCapIsALimit() {
    def deep as string init strings.replicate("<a>", 1001);
    # a non-2xx reply past the caps is a limit too, not "HTTP 500 without a
    # SOAP reply"
    def f as Fake init fake([canned(200, $deep), canned(500, $deep)]);
    for (def i in 0..2) {
        try {
            send(connect($f.url), "", addOp());
            missed();
        } catch (e) {
            testing.assertEqual($e.kind, "limit");
        }
    }
    finish($f);
    try {
        parse($deep);
        missed();
    } catch (e) {
        testing.assertEqual($e.kind, "limit");
    }
    try {
        faultElement(
            "1.1",
            Fault{code: "Server", subcode: "", reason: "r", actor: "", detail: $deep});
        missed();
    } catch (e) {
        testing.assertEqual($e.kind, "limit");
    }
}

# A 2xx reply that is not an envelope keeps its HTTP status.
func testSendNonEnvelopeKeepsStatus() {
    def f as Fake init fake([canned(200, "<html>not soap</html>"), canned(202, "not xml at all")]);
    for (def want in [200, 202]) {
        try {
            send(connect($f.url), "", addOp());
            missed();
        } catch (e) {
            testing.assertEqual($e.kind, ERROR_KIND);
            testing.assertEqual($e.status, $want);
            testing.assertTrue(strings.startsWith($e.message, "soap: "));
            testing.assertFalse(strings.startsWith($e.message, "soap: soap: "));
        }
    }
    finish($f);
}

func testSendRefusesBadAction() {
    for (def action in ["a\"b", "a\\b", "a\nb"]) {
        try {
            send(connect("http://127.0.0.1:9/x"), $action, addOp());
            missed();
        } catch (e) {
            testing.assertContains(messageOf($e), "SOAPAction");
        }
    }
}

# --- server ----------------------------------------------------------------

func addHandler(req as Request) {
    if (xml.localName($req.body) != "Add") {
        senderFault("unknown operation " + xml.localName($req.body));
    }
    def args as map of string to string init values($req.body);
    def sum as int init convert.toInt($args["intA"]) + convert.toInt($args["intB"]);
    def out as xml.Value init xml.elementNS(TEMPURI, "m:AddResponse");
    return xml.append(
        $out,
        xml.setText(xml.elementNS(TEMPURI, "m:AddResult"), convert.toString($sum)));
}

func brokenHandler(req as Request) {
    throw Error{kind: "sql", message: "password=hunter2 at db.internal"};
}

func testHandleSuccess() {
    def resp as Response init handle(
        envelope("1.1", [], [addOp()]),
        "http://tempuri.org/Add",
        addHandler);
    testing.assertEqual($resp.status, 200);
    testing.assertEqual($resp.contentType, "text/xml; charset=utf-8");
    def env as Envelope init parse($resp.body);
    testing.assertEqual(values($env.body[0])["AddResult"], "5");
    $resp = handle(envelope("1.2", [], [addOp()]), "", addHandler);
    testing.assertEqual($resp.contentType, "application/soap+xml; charset=utf-8");
    testing.assertEqual(parse($resp.body).version, "1.2");
}

func testHandleSenderFault() {
    def op as xml.Value init operation(TEMPURI, "Sub", {}, true);
    def resp as Response init handle(envelope("1.1", [], [$op]), "", addHandler);
    testing.assertEqual($resp.status, 500);
    def f as Fault init parseFault(parse($resp.body));
    testing.assertEqual($f.code, "Client");
    testing.assertEqual($f.reason, "unknown operation Sub");
    $resp = handle(envelope("1.2", [], [$op]), "", addHandler);
    testing.assertEqual($resp.status, 400);
    testing.assertEqual(parseFault(parse($resp.body)).code, "Sender");
}

func testHandleHidesInternalErrors() {
    def resp as Response init handle(envelope("1.2", [], [addOp()]), "", brokenHandler);
    testing.assertEqual($resp.status, 500);
    def f as Fault init parseFault(parse($resp.body));
    testing.assertEqual($f.code, "Receiver");
    testing.assertEqual($f.reason, "internal error");
    testing.assertFalse(strings.contains($resp.body, "hunter2"));
}

func testHandleMalformed() {
    def cases as map of string to string init {
        "Client": "not xml at all",
        "VersionMismatch": "<s:Envelope xmlns:s=\"urn:soap-0.9\"><s:Body/></s:Envelope>"
    };
    for (def code in $cases) {
        def resp as Response init handle($cases[$code], "", addHandler);
        testing.assertEqual($resp.status, 500);
        def env as Envelope init parse($resp.body);
        testing.assertEqual($env.version, "1.1");
        testing.assertEqual(parseFault($env).code, $code);
    }
    def empty as Response init handle(envelope("1.2", [], []), "", addHandler);
    testing.assertEqual($empty.status, 400);
    testing.assertContains(parseFault(parse($empty.body)).reason, "exactly one element");
    def dtd as Response init handle("<!DOCTYPE x><x/>", "", addHandler);
    testing.assertContains(parseFault(parse($dtd.body)).reason, "DOCTYPE");
}

func testHandleMustUnderstand() {
    def block as xml.Value init xml.setText(xml.elementNS("urn:auth", "a:Token"), "t");
    $block = xml.setAttrNS($block, SOAP12_NS, "soap:mustUnderstand", "true");
    def text as string init envelope("1.2", [$block], [addOp()]);
    def resp as Response init handle($text, "", addHandler);
    testing.assertEqual($resp.status, 500);
    testing.assertEqual(parseFault(parse($resp.body)).code, "MustUnderstand");
    $resp = handleWith($text, "", addHandler, ["\{urn:auth\}Token"]);
    testing.assertEqual($resp.status, 200);
    # A block for role "none" is never processed, so it need not be understood.
    $block = xml.setAttrNS($block, SOAP12_NS, "soap:role", ROLE_NONE);
    testing.assertEqual(handle(envelope("1.2", [$block], [addOp()]), "", addHandler).status, 200);
    # mustUnderstand="0" is optional.
    def optional as xml.Value init xml.setAttrNS(
        xml.elementNS("urn:auth", "a:Hint"),
        SOAP11_NS,
        "soap:mustUnderstand",
        "0");
    testing.assertEqual(
        handle(envelope("1.1", [$optional], [addOp()]), "", addHandler).status,
        200);
}

func serveOne(srv as httpd.Server) {
    def req as httpd.Request init httpd.accept($srv);
    def action as string init requestAction(
        httpd.header($req, "Content-Type"),
        httpd.header($req, "SOAPAction"));
    def resp as Response init handle(
        encoding.decode("utf-8", httpd.body($req)),
        $action,
        addHandler);
    httpd.setHeader($req, "Content-Type", $resp.contentType);
    httpd.respond($req, $resp.status, $resp.body);
    return $action;
}

func testEndToEnd() {
    def srv as httpd.Server init httpd.listen("127.0.0.1:0");
    def t as task of string init spawn {
        return serveOne($srv);
    };
    def c as Client init withVersion(connect("http://" + httpd.address($srv) + "/"), "1.2");
    def result as xml.Value init call($c, "http://tempuri.org/Add", addOp());
    testing.assertEqual(values($result)["AddResult"], "5");
    testing.assertEqual(task.wait($t), "http://tempuri.org/Add");
    httpd.shutdown($srv);
}

# --- review fixes ----------------------------------------------------------

func codeText(version as string, code as string, subcode as string) {
    def f as Fault init Fault{code: $code, subcode: $subcode, reason: "r", actor: "", detail: ""};
    return faultEnvelope($version, $f);
}

func testFaultCodes11() {
    testing.assertContains(
        codeText("1.1", "Client.Authentication", ""),
        "<faultcode>soap:Client.Authentication</faultcode>");
    testing.assertContains(
        codeText("1.1", "Sender.Auth", ""),
        "<faultcode>soap:Client.Auth</faultcode>");
    # An application code is not put in the envelope namespace.
    def busy as string init codeText("1.1", "Busy", "");
    testing.assertContains($busy, "<faultcode>Busy</faultcode>");
    testing.assertEqual(parseFault(parse($busy)).code, "Busy");
    testing.assertEqual(
        parseFault(parse(codeText("1.1", "Client.Authentication", ""))).code,
        "Client.Authentication");
}

func testFaultCodes12() {
    for (def code in ["Busy", "\{urn:app\}Busy", "Client.Auth"]) {
        try {
            codeText("1.2", $code, "");
            missed();
        } catch (e) {
            testing.assertContains(messageOf($e), "subcode");
        }
    }
    def text as string init codeText("1.2", "Client", "BadArgs");
    testing.assertContains($text, "<soap:Subcode><soap:Value>BadArgs</soap:Value>");
    def f as Fault init parseFault(parse($text));
    testing.assertEqual($f.code, "Sender");
    testing.assertEqual($f.subcode, "BadArgs");
}

func testFaultTextStaysWellFormed() {
    for (def version in ["1.1", "1.2"]) {
        def f as Fault init Fault{
            code: "Server",
            subcode: "",
            reason: "a\u0001b",
            actor: "x\u0008",
            detail: "d\u000b"
        };
        def back as Fault init parseFault(parse(faultEnvelope($version, $f)));
        testing.assertEqual($back.reason, "a?b");
        testing.assertEqual($back.actor, "x?");
        testing.assertEqual($back.detail, "d?");
    }
    def r as Response init handle("x", "", addHandler);
    testing.assertEqual($r.status, 500);
}

func controlHandler(req as Request) {
    senderFault("bad\u0001input");
}

func testSenderFaultReasonSanitised() {
    def resp as Response init handle(envelope("1.1", [], [addOp()]), "", controlHandler);
    testing.assertEqual($resp.status, 500);
    testing.assertEqual(parseFault(parse($resp.body)).reason, "bad?input");
}

func testOperationGuards() {
    try {
        operation("", "Op", {}, false);
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "namespace");
    }
    try {
        operation("urn:x", "Op", {"a": "x\u0001"}, false);
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "XML 1.0");
    }
    # Tab, newline and carriage return are allowed.
    testing.assertEqual(values(operation("urn:x", "Op", {"a": "1\t2\n3"}, false))["a"], "1\t2\n3");
}

func testClientGuards() {
    try {
        connect("https://h/a\u0001b");
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "endpoint");
    }
    for (def block in [xml.element("Token"), xml.children(xml.decode("<a>t<b/></a>"))[0]]) {
        try {
            withSoapHeader(connect("https://h/"), $block);
            missed();
        } catch (e) {
            testing.assertContains(messageOf($e), "namespace");
        }
    }
    try {
        contentType("1.2", "a\"; x=\"y");
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "SOAPAction");
    }
}

func testTransportErrorHidesQuery() {
    try {
        send(withTimeout(connect("http://127.0.0.1:1/svc?apikey=s3cret"), 2000), "", addOp());
        missed();
    } catch (e) {
        # a refused connection is http's net.NetError, which names the host
        # and port only
        testing.assertEqual($e.kind, "net");
        testing.assertFalse(strings.contains($e.message, "s3cret"));
    }
}

func mustText(version as string, attr as string, value as string) {
    def ns as string init namespaceOf($version);
    def block as xml.Value init xml.setText(xml.elementNS("urn:auth", "a:Token"), "t");
    $block = xml.setAttrNS($block, $ns, "soap:mustUnderstand", "1");
    if ($attr != "") {
        $block = xml.setAttrNS($block, $ns, "soap:" + $attr, $value);
    }
    return handle(envelope($version, [$block], [addOp()]), "", addHandler).status;
}

func testMustUnderstandTargeting() {
    testing.assertEqual(mustText("1.1", "", ""), 500);
    testing.assertEqual(
        mustText("1.1", "actor", "http://schemas.xmlsoap.org/soap/actor/next"),
        500);
    testing.assertEqual(mustText("1.1", "actor", "urn:some-gateway"), 200);
    testing.assertEqual(mustText("1.2", "role", ROLE_BASE + "next"), 500);
    testing.assertEqual(mustText("1.2", "role", ROLE_BASE + "ultimateReceiver"), 500);
    testing.assertEqual(mustText("1.2", "role", ROLE_NONE), 200);
    testing.assertEqual(mustText("1.2", "role", "urn:some-gateway"), 200);
}

# --- HTTP Digest -------------------------------------------------------------

func md5Hex(text as string) {
    return encoding.toText("hex", hash.compute("md5", encoding.encode("utf-8", $text)));
}

# The value of one name="value" (or name=value) parameter of a Digest header.
func digestField(auth as string, name as string) {
    def m as regex.Match init regex.find(',? ' + $name + '="?([^",]*)"?', $auth);
    if ($m.start < 0) {
        return "";
    }
    return $m.groups[0];
}

func testDigestAuthReplacesOtherAuth() {
    def c as Client init withDigestAuth(
        withBasicAuth(connect("https://h/svc"), "u", "p"),
        "cam",
        "pw");
    testing.assertFalse(maps.has($c.headers, "Authorization"));
    testing.assertEqual($c.digest.user, "cam");
    $c = withToken($c, "t");
    testing.assertEqual($c.digest.user, "");
    testing.assertEqual($c.headers["Authorization"], "Bearer t");
    for (def bad in ["", "a\nb"]) {
        try {
            withDigestAuth($c, $bad, "pw");
            missed();
        } catch (e) {
            testing.assertContains(messageOf($e), "Digest user name");
        }
    }
}

func testCallWithDigestAuth() {
    def challenge as string init 'Digest realm="onvif", qop="auth", nonce="abc123", algorithm=MD5';
    def f as Fake init fake([
        Canned{status: 401, headers: {"WWW-Authenticate": $challenge}, body: ""},
        canned(200, ADD_REPLY)
    ]);
    def c as Client init withDigestAuth(connect($f.url), "admin", "secret");
    testing.assertEqual(values(call($c, "http://tempuri.org/Add", addOp()))["AddResult"], "5");
    def seen as list of Seen init finish($f);
    testing.assertEqual(len($seen), 2);
    testing.assertFalse(maps.has($seen[0].headers, "Authorization"));
    # Both requests carry the envelope.
    testing.assertEqual($seen[0].body, $seen[1].body);
    def auth as string init $seen[1].headers["Authorization"];
    testing.assertTrue(strings.startsWith($auth, "Digest "));
    testing.assertEqual(digestField($auth, "uri"), "/svc.asmx");
    # Recomputed here, independently of the http module (RFC 7616, MD5, qop=auth).
    def ha1 as string init md5Hex("admin:onvif:secret");
    def ha2 as string init md5Hex("POST:/svc.asmx");
    def want as string init md5Hex(
        $ha1 + ":abc123:" + digestField($auth, "nc") + ":" + digestField($auth, "cnonce") +
            ":auth:" + $ha2);
    testing.assertEqual(digestField($auth, "response"), $want);
}

func testDigestAuthWrongPasswordIsAnHttpError() {
    def challenge as string init 'Digest realm="onvif", qop="auth", nonce="n"';
    def f as Fake init fake([
        Canned{status: 401, headers: {"WWW-Authenticate": $challenge}, body: ""},
        Canned{status: 401, headers: {"WWW-Authenticate": $challenge}, body: ""}
    ]);
    try {
        call(withDigestAuth(connect($f.url), "admin", "wrong"), "", addOp());
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "HTTP 401");
    }
    testing.assertEqual(len(finish($f)), 2);
}

# A fault from call is a SoapError with the status and the parsed fault; an
# HTTP failure without a SOAP reply keeps its status.
func testCallFaultIsASoapError() {
    def f as Fake init fake([canned(500, DOTNET_FAULT)]);
    try {
        call(connect($f.url), "", addOp());
        missed();
    } catch (e) {
        testing.assertEqual($e.kind, "soap");
        testing.assertTrue($e.isFault);
        testing.assertEqual($e.status, 500);
        testing.assertEqual($e.fault.code, "Client");
        testing.assertContains($e.fault.reason, "Server was unable to read request");
    }
    finish($f);
    def g as Fake init fake([canned(502, "")]);
    try {
        call(connect($g.url), "", addOp());
        missed();
    } catch (e) {
        testing.assertFalse($e.isFault);
        testing.assertEqual($e.status, 502);
        testing.assertEqual($e.fault.code, "");
    }
    finish($g);
}

# An Authorization header to a non-loopback http:// endpoint is refused before
# any connection (192.0.2.1 is TEST-NET, nothing answers there); loopback is
# allowed (testCall11), and so is a client that opts in.
func testCredentialsNeedHttps() {
    def plain as Client init withTimeout(connect("http://192.0.2.1/svc"), 2000);
    for (def c in [
        withBasicAuth($plain, "u", "p"),
        withToken($plain, "t"),
        withHeader($plain, "authorization", "x"),
        withHeader($plain, "Proxy-Authorization", "x"),
        # Digest sends no reusable secret, but its hash can be cracked offline
        withDigestAuth($plain, "u", "p")
    ]) {
        try {
            send($c, "", addOp());
            missed();
        } catch (e) {
            testing.assertEqual($e.kind, "soap");
            def se as SoapError init $e;
            testing.assertEqual($se.status, 0);
            testing.assertFalse($se.isFault);
        }
    }
    def lan as Client init withToken(connect("http://host.invalid/svc"), "t");
    $lan.allowInsecureHttp = true;
    try {
        send($lan, "", addOp());
        missed();
    } catch (e) {
        # the name does not resolve; what matters is that it was tried
        testing.assertEqual($e.kind, "net");
    }
}

# --- WSDL -----------------------------------------------------------------

# An ASMX-style description: document/literal, a qualified inline schema,
# SOAP 1.1 and 1.2 ports and an HTTP GET port that is not SOAP. ADDRESS is
# replaced with the endpoint.
def const CALC_WSDL as string init '<?xml version="1.0" encoding="utf-8"?>
<wsdl:definitions name="Calculator" targetNamespace="http://tempuri.org/"
    xmlns:wsdl="http://schemas.xmlsoap.org/wsdl/"
    xmlns:soap="http://schemas.xmlsoap.org/wsdl/soap/"
    xmlns:soap12="http://schemas.xmlsoap.org/wsdl/soap12/"
    xmlns:http="http://schemas.xmlsoap.org/wsdl/http/"
    xmlns:s="http://www.w3.org/2001/XMLSchema"
    xmlns:tns="http://tempuri.org/">
  <wsdl:documentation>Adds two numbers.</wsdl:documentation>
  <wsdl:types>
    <s:schema elementFormDefault="qualified" targetNamespace="http://tempuri.org/">
      <s:element name="Add"/>
      <s:element name="AddResponse"/>
    </s:schema>
  </wsdl:types>
  <wsdl:message name="AddSoapIn"><wsdl:part name="parameters" element="tns:Add"/></wsdl:message>
  <wsdl:message name="AddSoapOut">
    <wsdl:part name="parameters" element="tns:AddResponse"/>
  </wsdl:message>
  <wsdl:message name="AddHttpGetIn"><wsdl:part name="intA" type="s:string"/></wsdl:message>
  <wsdl:portType name="CalculatorSoap">
    <wsdl:operation name="Add">
      <wsdl:input message="tns:AddSoapIn"/>
      <wsdl:output message="tns:AddSoapOut"/>
    </wsdl:operation>
  </wsdl:portType>
  <wsdl:portType name="CalculatorHttpGet">
    <wsdl:operation name="Add"><wsdl:input message="tns:AddHttpGetIn"/></wsdl:operation>
  </wsdl:portType>
  <wsdl:binding name="CalculatorSoap" type="tns:CalculatorSoap">
    <soap:binding transport="http://schemas.xmlsoap.org/soap/http"/>
    <wsdl:operation name="Add">
      <soap:operation soapAction="http://tempuri.org/Add" style="document"/>
      <wsdl:input><soap:body use="literal"/></wsdl:input>
      <wsdl:output><soap:body use="literal"/></wsdl:output>
    </wsdl:operation>
  </wsdl:binding>
  <wsdl:binding name="CalculatorSoap12" type="tns:CalculatorSoap">
    <soap12:binding transport="http://schemas.xmlsoap.org/soap/http"/>
    <wsdl:operation name="Add">
      <soap12:operation soapAction="http://tempuri.org/Add" style="document"/>
      <wsdl:input><soap12:body use="literal"/></wsdl:input>
      <wsdl:output><soap12:body use="literal"/></wsdl:output>
    </wsdl:operation>
  </wsdl:binding>
  <wsdl:binding name="CalculatorHttpGet" type="tns:CalculatorHttpGet">
    <http:binding verb="GET"/>
    <wsdl:operation name="Add"><http:operation location="/Add"/></wsdl:operation>
  </wsdl:binding>
  <wsdl:service name="Calculator">
    <wsdl:port name="CalculatorSoap" binding="tns:CalculatorSoap">
      <soap:address location="ADDRESS"/>
    </wsdl:port>
    <wsdl:port name="CalculatorSoap12" binding="tns:CalculatorSoap12">
      <soap12:address location="https://example.com/calc12.asmx"/>
    </wsdl:port>
    <wsdl:port name="CalculatorHttpGet" binding="tns:CalculatorHttpGet">
      <http:address location="https://example.com/calc.asmx"/>
    </wsdl:port>
  </wsdl:service>
</wsdl:definitions>';

# An rpc/literal SOAP 1.2 description with a default namespace and no prefix
# on its references.
def const RPC_WSDL as string init '<definitions xmlns="http://schemas.xmlsoap.org/wsdl/"
    xmlns:soap12="http://schemas.xmlsoap.org/wsdl/soap12/"
    xmlns:xsd="http://www.w3.org/2001/XMLSchema"
    xmlns:t="urn:stock" targetNamespace="urn:stock">
  <message name="QuoteIn"><part name="symbol" type="xsd:string"/></message>
  <message name="QuoteOut"><part name="price" type="xsd:decimal"/></message>
  <message name="Ping"/>
  <portType name="Stock">
    <operation name="Quote"><input message="t:QuoteIn"/><output message="t:QuoteOut"/></operation>
    <operation name="Notify"><input message="t:Ping"/></operation>
  </portType>
  <binding name="StockBinding" type="t:Stock">
    <soap12:binding style="rpc" transport="http://schemas.xmlsoap.org/soap/http"/>
    <operation name="Quote">
      <soap12:operation soapAction="urn:stock#Quote"/>
      <input><soap12:body use="literal" namespace="urn:stock:rpc"/></input>
      <output><soap12:body use="literal" namespace="urn:stock:rpc"/></output>
    </operation>
    <operation name="Notify">
      <soap12:operation style="document"/>
      <input><soap12:body use="literal" namespace="urn:ignored"/></input>
    </operation>
  </binding>
  <service name="StockService">
    <port name="StockPort" binding="t:StockBinding">
      <soap12:address location="https://example.com/stock"/>
    </port>
  </service>
</definitions>';

# A service whose binding, port type and messages sit in an imported document.
def const IMPORTING_WSDL as string init '<w:definitions xmlns:w="http://schemas.xmlsoap.org/wsdl/"
    xmlns:soap="http://schemas.xmlsoap.org/wsdl/soap/"
    xmlns:b="urn:vbox" targetNamespace="urn:vbox:service">
  <w:import namespace="urn:vbox" location="vboxweb.wsdl"/>
  <w:service name="vboxService">
    <w:port name="vboxServicePort" binding="b:vboxBinding">
      <soap:address location="http://localhost:18083/"/>
    </w:port>
  </w:service>
</w:definitions>';

def const IMPORTED_WSDL as string init '<w:definitions xmlns:w="http://schemas.xmlsoap.org/wsdl/"
    xmlns:soap="http://schemas.xmlsoap.org/wsdl/soap/"
    xmlns:v="urn:vbox" targetNamespace="urn:vbox">
  <w:import namespace="urn:vbox:service" location="vboxwebService.wsdl"/>
  <w:message name="logonRequest">
    <w:part name="parameters" element="v:IWebsessionManager_logon"/>
  </w:message>
  <w:message name="logonResponse">
    <w:part name="parameters" element="v:IWebsessionManager_logonResponse"/>
  </w:message>
  <w:portType name="vboxPortType">
    <w:operation name="IWebsessionManager_logon">
      <w:input message="v:logonRequest"/>
      <w:output message="v:logonResponse"/>
    </w:operation>
  </w:portType>
  <w:binding name="vboxBinding" type="v:vboxPortType">
    <soap:binding style="document" transport="http://schemas.xmlsoap.org/soap/http"/>
    <w:operation name="IWebsessionManager_logon">
      <soap:operation soapAction=""/>
      <w:input><soap:body use="literal"/></w:input>
      <w:output><soap:body use="literal"/></w:output>
    </w:operation>
  </w:binding>
</w:definitions>';

# wsdlFailure is the kind and message a WSDL read fails with, "ok" when it
# does not.
func wsdlFailure(text as string, imports as map of string to string) {
    try {
        parseWsdlWith($text, WsdlOptions{imports: $imports, maxDocuments: 0});
    } catch (e) {
        return $e.kind + ": " + $e.message;
    }
    return "ok";
}

func testParseWsdlDocumentLiteral() {
    def w as Wsdl init parseWsdl(CALC_WSDL);
    testing.assertEqual($w.name, "Calculator");
    testing.assertEqual($w.targetNamespace, TEMPURI);
    testing.assertEqual(len($w.ports), 2);
    testing.assertEqual($w.ports[0].service, "Calculator");
    testing.assertEqual($w.ports[0].name, "CalculatorSoap");
    testing.assertEqual($w.ports[0].version, "1.1");
    testing.assertEqual($w.ports[1].version, "1.2");
    testing.assertEqual($w.ports[1].address, "https://example.com/calc12.asmx");
    testing.assertEqual(len($w.bindings), 2);
    def op as WsdlOperation init $w.bindings[0].operations[0];
    testing.assertEqual($op.name, "Add");
    testing.assertEqual($op.action, "http://tempuri.org/Add");
    testing.assertEqual($op.style, "document");
    testing.assertEqual($op.bodyUse, "literal");
    testing.assertEqual($op.namespace, "");
    testing.assertEqual($op.input[0].element, "Add");
    testing.assertEqual($op.input[0].elementNamespace, TEMPURI);
    testing.assertTrue($op.input[0].qualified);
    testing.assertEqual($op.output[0].element, "AddResponse");
}

func testWsdlCallPicksAPort() {
    def w as Wsdl init parseWsdl(CALC_WSDL);
    def first as WsdlCall init wsdlCall($w, "Add");
    testing.assertEqual($first.address, "ADDRESS");
    testing.assertEqual($first.version, "1.1");
    def at12 as WsdlCall init wsdlCallAt($w, "CalculatorSoap12", "Add");
    testing.assertEqual($at12.version, "1.2");
    testing.assertEqual($at12.operation.action, "http://tempuri.org/Add");
    def kind as string init "";
    try {
        wsdlCallAt($w, "CalculatorHttpGet", "Add");
    } catch (e) {
        $kind = $e.kind;
        testing.assertContains($e.message, "no SOAP port named");
    }
    testing.assertEqual($kind, "soap");
    try {
        wsdlCall($w, "Subtract");
        missed();
    } catch (e) {
        testing.assertEqual($e.kind, "soap");
        testing.assertContains($e.message, "offers the operation \"Subtract\"");
    }
}

func testWsdlRequestDocumentLiteral() {
    def target as WsdlCall init wsdlCall(parseWsdl(CALC_WSDL), "Add");
    def body as xml.Value init wsdlRequest($target, {"intA": "2", "intB": "3"});
    testing.assertEqual(xml.encode($body), xml.encode(addOp()));
}

func testParseWsdlRpc() {
    def w as Wsdl init parseWsdl(RPC_WSDL);
    testing.assertEqual($w.bindings[0].style, "rpc");
    def quote as WsdlCall init wsdlCall($w, "Quote");
    testing.assertEqual($quote.version, "1.2");
    testing.assertEqual($quote.operation.style, "rpc");
    testing.assertEqual($quote.operation.namespace, "urn:stock:rpc");
    testing.assertEqual($quote.operation.input[0].typeName, "string");
    testing.assertEqual(
        $quote.operation.input[0].typeNamespace,
        "http://www.w3.org/2001/XMLSchema");
    def body as xml.Value init wsdlRequest($quote, {"symbol": "ACME"});
    testing.assertEqual(xml.localName($body), "Quote");
    testing.assertEqual(xml.namespace($body), "urn:stock:rpc");
    testing.assertEqual(values($body), {"symbol": "ACME"});
    # a document operation in an rpc binding: no wrapper namespace, one-way
    def notify as WsdlOperation init wsdlCall($w, "Notify").operation;
    testing.assertEqual($notify.style, "document");
    testing.assertEqual($notify.namespace, "");
    testing.assertEqual($notify.input, []);
    testing.assertEqual($notify.output, []);
    testing.assertContains(wsdlRequestFailure(wsdlCall($w, "Notify")), "is not one schema element");
}

# wsdlRequestFailure is the message wsdlRequest fails with.
func wsdlRequestFailure(target as WsdlCall) {
    try {
        wsdlRequest($target, {});
    } catch (e) {
        return $e.kind + ": " + $e.message;
    }
    return "ok";
}

func testParseWsdlWithImports() {
    def imports as map of string to string init {
        "vboxweb.wsdl": IMPORTED_WSDL,
        "vboxwebService.wsdl": IMPORTING_WSDL
    };
    def w as Wsdl init parseWsdlWith(
        IMPORTING_WSDL,
        WsdlOptions{imports: $imports, maxDocuments: 0});
    testing.assertEqual($w.targetNamespace, "urn:vbox:service");
    def target as WsdlCall init wsdlCall($w, "IWebsessionManager_logon");
    testing.assertEqual($target.address, "http://localhost:18083/");
    testing.assertEqual($target.operation.action, "");
    testing.assertEqual($target.operation.input[0].elementNamespace, "urn:vbox");
    testing.assertFalse($target.operation.input[0].qualified);
    testing.assertContains(
        wsdlFailure(IMPORTING_WSDL, {}),
        "soap: soap: the WSDL imports \"vboxweb.wsdl\"");
}

func testParseWsdlRefusals() {
    def cases as list of list of string init [
        ['<definitions xmlns="http://www.w3.org/ns/wsdl"/>', "WSDL 2.0 is not supported"],
        ['<definitions/>', "not a WSDL 1.1 definitions element"],
        ['<!DOCTYPE d [<!ENTITY x "y">]><d/>', "must not carry a DOCTYPE"],
        ['<w:definitions', "the WSDL is not XML"],
        [
            strings.replace(
                CALC_WSDL,
                'binding="tns:CalculatorSoap12"',
                'binding="q:CalculatorSoap12"'),
            "uses an undeclared prefix"
        ],
        [
            strings.replace(CALC_WSDL, 'binding="tns:CalculatorSoap12"', 'binding="tns:Nope"'),
            "names the undefined binding"
        ],
        [
            strings.replace(CALC_WSDL, 'message="tns:AddSoapOut"', 'message="tns:Nope"'),
            "names the undefined message"
        ],
        [
            strings.replace(
                CALC_WSDL,
                '<wsdl:message name="AddHttpGetIn">',
                '<wsdl:message name="AddSoapIn">'),
            "defines message"
        ],
        [
            strings.replace(CALC_WSDL, 'style="document"/>', 'style="literal"/>'),
            "style is document or rpc"
        ],
        [
            strings.replace(CALC_WSDL, 'element="tns:Add"/>', 'element="tns:Add" type="s:int"/>'),
            "names exactly one of element and type"
        ],
        [
            strings.replace(
                RPC_WSDL,
                '<operation name="Notify"><input',
                '<operation name="Quote"><input'),
            "overloading is not supported"
        ],
        [
            strings.replace(
                RPC_WSDL,
                '<operation name="Notify"><input message="t:Ping"/></operation>',
                '<operation name="Gone"><input message="t:Ping"/></operation>'),
            "is not in its port type"
        ],
        [
            strings.replace(CALC_WSDL, '<soap:address location="ADDRESS"/>', '<soap:address/>'),
            "has no location attribute"
        ],
        [
            strings.replace(
                CALC_WSDL,
                '<http:address location="https://example.com/calc.asmx"/>',
                '<soap:address location="https://example.com/calc.asmx"/>'),
            "binding CalculatorHttpGet is not SOAP"
        ]
    ];
    for (def c in $cases) {
        def got as string init wsdlFailure($c[0], {});
        testing.assertEqual(strings.split($got, ":")[0], "soap");
        testing.assertContains($got, $c[1]);
    }
}

func testWsdlRequestRefusesEncoded() {
    def encoded as string init strings.replace(
        RPC_WSDL,
        '<input><soap12:body use="literal" namespace="urn:stock:rpc"/></input>',
        '<input><soap12:body use="encoded" namespace="urn:stock:rpc"/></input>');
    def target as WsdlCall init wsdlCall(parseWsdl($encoded), "Quote");
    testing.assertEqual($target.operation.bodyUse, "encoded");
    testing.assertContains(wsdlRequestFailure($target), "uses SOAP encoding");
}

func testParseWsdlImportCap() {
    def imports as map of string to string init {};
    for (def i in 0..MAX_WSDL_DOCUMENTS + 1) {
        def next as string init "d" + convert.toString($i + 1);
        $imports["d" + convert.toString($i)] = '<definitions xmlns="' + WSDL_NS + '">' +
            '<import location="' + $next + '"/></definitions>';
    }
    def got as string init wsdlFailure($imports["d0"], $imports);
    testing.assertEqual(strings.split($got, ":")[0], "limit");
    # maxDocuments lowers the cap; 0 keeps it, a value outside 0..32 is misuse.
    def three as map of string to string init {
        "d1": $imports["d1"],
        "d2": $imports["d2"],
        "d3": $imports["d3"]
    };
    def kinds as list of string init [];
    for (def m in [2, -1, MAX_WSDL_DOCUMENTS + 1]) {
        try {
            parseWsdlWith($imports["d0"], WsdlOptions{imports: $three, maxDocuments: $m});
            $kinds[] = "ok";
        } catch (e) {
            $kinds[] = $e.kind;
        }
    }
    testing.assertEqual($kinds, ["limit", "soap", "soap"]);
}

func testWsdlEndToEnd() {
    def f as Fake init fake([canned(200, ADD_REPLY)]);
    def w as Wsdl init parseWsdl(strings.replace(CALC_WSDL, "ADDRESS", $f.url));
    def target as WsdlCall init wsdlCall($w, "Add");
    def result as xml.Value init call(
        wsdlConnect($target),
        $target.operation.action,
        wsdlRequest($target, {"intA": "2", "intB": "3"}));
    testing.assertEqual(values($result)["AddResult"], "5");
    def seen as list of Seen init finish($f);
    testing.assertEqual($seen[0].headers["SOAPAction"], "\"http://tempuri.org/Add\"");
    testing.assertEqual(values(parse($seen[0].body).body[0]), {"intA": "2", "intB": "3"});
}

# A description without a service, as ONVIF's device WSDLs are.
func testWsdlCallToAnAddress() {
    def bindingOnly as string init strings.replace(
        RPC_WSDL,
        '<service name="StockService">',
        '<service name="StockService"><documentation>no ports</documentation>');
    def w as Wsdl init parseWsdl(CALC_WSDL);
    def target as WsdlCall init wsdlCallTo($w, "http://10.0.0.5/onvif/device_service", "Add");
    testing.assertEqual($target.address, "http://10.0.0.5/onvif/device_service");
    testing.assertEqual($target.version, "1.1");
    testing.assertEqual($target.operation.action, "http://tempuri.org/Add");
    def stock as Wsdl init parseWsdl($bindingOnly);
    testing.assertEqual(wsdlCallTo($stock, "https://x/", "Quote").version, "1.2");
    try {
        wsdlCallTo($stock, "https://x/", "Sell");
        missed();
    } catch (e) {
        testing.assertEqual($e.kind, "soap");
        testing.assertContains($e.message, "no SOAP binding of the WSDL has the operation");
    }
}

# --- strictness and guards ---------------------------------------------------

func stringHandler(req as Request) {
    throw "oops"; # lint-disable: L104
}

# A handler may throw any value: one that is not an Error is a Server fault
# too, not a "runtime" error out of handle.
func testHandleNonErrorThrow() {
    def resp as Response init handle(envelope("1.1", [], [addOp()]), "", stringHandler);
    testing.assertEqual($resp.status, 500);
    def f as Fault init parseFault(parse($resp.body));
    testing.assertEqual($f.code, "Server");
    testing.assertEqual($f.reason, "internal error");
}

# envelopeOf wraps Body content and Envelope content around a 1.1 envelope.
func envelopeOf(inEnvelope as string, inBody as string) {
    return "<s:Envelope xmlns:s=\"" + SOAP11_NS + "\">" + $inEnvelope + "<s:Body>" + $inBody +
        "</s:Body></s:Envelope>";
}

func testParseRefusesStrayContent() {
    def fault as string init "<s:Fault><faultcode>s:Server</faultcode>" +
        "<faultstring>boom</faultstring></s:Fault>";
    def cases as map of string to string init {
        "a Fault is the only element": envelopeOf("", $fault + "<x/>"),
        "the Body holds text": envelopeOf("", "junk<x/>"),
        "the Envelope holds text": envelopeOf("junk", "<x/>"),
        "a header block is namespace-qualified": "<s:Envelope xmlns:s=\"" + SOAP12_NS +
            "\"><s:Header><h/></s:Header><s:Body><x/></s:Body></s:Envelope>",
        "a Header is the Envelope's first element": "<s:Envelope xmlns:s=\"" + SOAP11_NS +
            "\"><s:Body><x/></s:Body><s:Header/></s:Envelope>"
    };
    for (def want in $cases) {
        try {
            parse($cases[$want]);
            missed();
        } catch (e) {
            testing.assertContains(messageOf($e), $want);
        }
    }
    # A 1.1 Header after the Body would skip the mustUnderstand check.
    def late as string init "<s:Envelope xmlns:s=\"" + SOAP11_NS + "\"><s:Body>" +
        xml.encode(addOp()) + "</s:Body><s:Header><a:h xmlns:a=\"urn:a\" " +
        "s:mustUnderstand=\"1\"/></s:Header></s:Envelope>";
    def resp as Response init handle($late, "", addHandler);
    testing.assertEqual(parseFault(parse($resp.body)).code, "Client");
    # SOAP 1.1 requires qualified header blocks too
    def bare as string init "<s:Envelope xmlns:s=\"" + SOAP11_NS + "\"><s:Header><h/></s:Header>" +
        "<s:Body>" + xml.encode(addOp()) + "</s:Body></s:Envelope>";
    testing.assertEqual(parseFault(parse(handle($bare, "", addHandler).body)).code, "Client");
}

func testMustUnderstandValues() {
    # 1.2 takes xs:boolean, 1.1 only 0 and 1; anything else is the sender's error
    testing.assertEqual(mustValue("1.2", "yes"), "Sender");
    testing.assertEqual(mustValue("1.1", "true"), "Client");
    testing.assertEqual(mustValue("1.2", "true"), "MustUnderstand");
    testing.assertEqual(mustValue("1.2", "false"), "");
    testing.assertEqual(mustValue("1.1", " 0 "), "");
}

# mustValue is the fault code handle answers a block with mustUnderstand set to
# `value` with, "" for none.
func mustValue(version as string, value as string) {
    def ns as string init namespaceOf($version);
    def block as xml.Value init xml.setAttrNS(
        xml.elementNS("urn:auth", "a:Token"),
        $ns,
        "soap:mustUnderstand",
        $value);
    def resp as Response init handle(envelope($version, [$block], [addOp()]), "", addHandler);
    if ($resp.status == 200) {
        return "";
    }
    return parseFault(parse($resp.body)).code;
}

func testAuthHeadersReplaceAnyCase() {
    def c as Client init withHeader(connect("https://h/"), "authorization", "Basic old");
    $c = withBasicAuth($c, "u", "p");
    testing.assertEqual(maps.keys($c.headers), ["Authorization"]);
    $c = withHeader($c, "AUTHORIZATION", "x");
    $c = withToken($c, "t");
    testing.assertEqual($c.headers, {"Authorization": "Bearer t"});
    $c = withHeader(withHeader($c, "x-extra", "1"), "X-Extra", "2");
    testing.assertEqual($c.headers["X-Extra"], "2");
    testing.assertFalse(maps.has($c.headers, "x-extra"));
}

func testReservedNamespacesRefused() {
    for (def ns in [XML_NS, XMLNS_NS, "urn:a\u0001", ""]) {
        try {
            operation($ns, "Op", {}, false);
            missed();
        } catch (e) {
            testing.assertContains(messageOf($e), "namespace URI");
        }
    }
    for (def ns in [XML_NS, XMLNS_NS, "urn:a\u0001"]) {
        def f as Fault init Fault{
            code: "\{" + $ns + "\}Busy",
            subcode: "",
            reason: "",
            actor: "",
            detail: ""
        };
        try {
            faultElement("1.1", $f);
            missed();
        } catch (e) {
            testing.assertContains(messageOf($e), "namespace URI");
        }
        $f.code = "Sender";
        $f.subcode = "\{" + $ns + "\}Busy";
        try {
            faultElement("1.2", $f);
            missed();
        } catch (e) {
            testing.assertContains(messageOf($e), "namespace URI");
        }
    }
    def rpc as string init strings.replace(
        RPC_WSDL,
        'namespace="urn:stock:rpc"/></input>',
        'namespace="' + XMLNS_NS + '"/></input>');
    testing.assertContains(
        wsdlRequestFailure(wsdlCall(parseWsdl($rpc), "Quote")),
        "soap: soap: an operation's namespace");
}

func testFaultDetailMixedContent() {
    def f as Fault init Fault{
        code: "Server",
        subcode: "",
        reason: "r",
        actor: "",
        detail: "<a xmlns=\"urn:d\">1</a> and more"
    };
    for (def version in ["1.1", "1.2"]) {
        if ($version == "1.2") {
            $f.code = "Receiver";
        }
        def text as string init faultEnvelope($version, $f);
        testing.assertContains($text, "</a> and more</");
        # read back, mixed content is the whole detail element
        def got as string init parseFault(parse($text)).detail;
        testing.assertContains($got, "<a xmlns=\"urn:d\">1</a> and more");
        testing.assertTrue(strings.startsWith($got, "<"));
    }
    # Content stands on its own: the wrapper's soap: prefix is not lent to it.
    $f.detail = "<soap:x/>";
    try {
        faultElement("1.2", $f);
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "is XML");
    }
}

func testCredentialsAllowedWithOptIn() {
    def c as Client init withDigestAuth(connect("http://host.invalid/svc"), "u", "p");
    $c.allowInsecureHttp = true;
    try {
        send(withTimeout($c, 2000), "", addOp());
        missed();
    } catch (e) {
        # the name does not resolve; what matters is that it was tried
        testing.assertEqual($e.kind, "net");
    }
}

func testRequestActionQuotedParameters() {
    def ct as string init "application/soap+xml; x=\"a;action=evil\"; action=\"urn:good\"";
    testing.assertEqual(requestAction($ct, ""), "urn:good");
    testing.assertEqual(requestAction("application/soap+xml; x=\"a;action=evil\"", ""), "");
    testing.assertEqual(
        requestAction("application/soap+xml; action=\"urn:a\\\"b\"", ""),
        "urn:a\"b");
    testing.assertEqual(
        requestAction("application/soap+xml; charset=utf-8;action=urn:c", ""),
        "urn:c");
    # parameters that do not parse carry no action
    testing.assertEqual(requestAction("application/soap+xml; junk; action=urn:d", ""), "");
}

# --- WSDL strictness ---------------------------------------------------------

func testWsdlPortVersionMatchesBinding() {
    def mismatch as string init strings.replace(
        CALC_WSDL,
        '<soap12:address location="https://example.com/calc12.asmx"/>',
        '<soap:address location="https://example.com/calc12.asmx"/>');
    testing.assertContains(
        wsdlFailure($mismatch, {}),
        "has a SOAP 1.1 address but binding CalculatorSoap12 is SOAP 1.2");
}

func testWsdlBindingRefusals() {
    def two as string init strings.replace(
        CALC_WSDL,
        '<soap:binding transport="http://schemas.xmlsoap.org/soap/http"/>',
        '<soap:binding transport="http://schemas.xmlsoap.org/soap/http"/>' +
            '<soap12:binding transport="http://schemas.xmlsoap.org/soap/http"/>');
    testing.assertContains(wsdlFailure($two, {}), "has two SOAP binding elements");
    def none as string init strings.replace(
        CALC_WSDL,
        '<soap:binding transport="http://schemas.xmlsoap.org/soap/http"/>',
        '<soap:binding/>');
    testing.assertContains(wsdlFailure($none, {}), "without transport");
    def unknown as string init strings.replace(
        CALC_WSDL,
        '<wsdl:input><soap:body use="literal"/></wsdl:input>',
        '<wsdl:input><soap:body use="literal" parts="nope"/></wsdl:input>');
    testing.assertContains(wsdlFailure($unknown, {}), "names the part \"nope\"");
}

# A binding over another transport (JMS) is left out with its ports.
func testWsdlOtherTransportLeftOut() {
    def jms as string init strings.replace(
        CALC_WSDL,
        '<soap12:binding transport="http://schemas.xmlsoap.org/soap/http"/>',
        '<soap12:binding transport="http://www.w3.org/2010/soapjms/"/>');
    def w as Wsdl init parseWsdl($jms);
    testing.assertEqual(len($w.bindings), 1);
    testing.assertEqual(len($w.ports), 1);
    testing.assertEqual($w.ports[0].name, "CalculatorSoap");
}

# soap:body parts picks the Body's parts; soap:header names the header blocks.
def const HEADER_WSDL as string init '<definitions xmlns="http://schemas.xmlsoap.org/wsdl/"
    xmlns:soap="http://schemas.xmlsoap.org/wsdl/soap/" xmlns:t="urn:t" targetNamespace="urn:t">
  <message name="In">
    <part name="session" element="t:Session"/>
    <part name="body" element="t:Op"/>
  </message>
  <message name="Out"><part name="result" element="t:OpResponse"/></message>
  <message name="Trace"><part name="id" element="t:TraceId"/></message>
  <portType name="PT">
    <operation name="Op"><input message="t:In"/><output message="t:Out"/></operation>
  </portType>
  <binding name="B" type="t:PT">
    <soap:binding style="document" transport="http://schemas.xmlsoap.org/soap/http"/>
    <operation name="Op">
      <soap:operation soapAction="urn:t#Op"/>
      <input>
        <soap:header message="t:In" part="session" use="literal"/>
        <soap:body parts="body" use="literal"/>
      </input>
      <output>
        <soap:header message="t:Trace" part="id" use="literal"/>
        <soap:body use="literal"/>
      </output>
    </operation>
  </binding>
  <service name="S">
    <port name="P" binding="t:B"><soap:address location="https://example.com/t"/></port>
  </service>
</definitions>';

func testWsdlBodyAndHeaderParts() {
    def op as WsdlOperation init wsdlCall(parseWsdl(HEADER_WSDL), "Op").operation;
    testing.assertEqual(len($op.input), 1);
    testing.assertEqual($op.input[0].element, "Op");
    testing.assertEqual(len($op.inputHeaders), 1);
    testing.assertEqual($op.inputHeaders[0].element, "Session");
    testing.assertEqual($op.output[0].element, "OpResponse");
    testing.assertEqual($op.outputHeaders[0].element, "TraceId");
    # with the header part out of the Body, the request is one element again
    def body as xml.Value init wsdlRequest(wsdlCall(parseWsdl(HEADER_WSDL), "Op"), {});
    testing.assertEqual(xml.localName($body), "Op");
    def badHeader as string init strings.replace(HEADER_WSDL, 'part="id"', 'part="nope"');
    testing.assertContains(wsdlFailure($badHeader, {}), "names the part \"nope\"");
}

func testWsdlRequestElementInNoNamespace() {
    # prefixed, so an unprefixed QName is in no namespace
    def nons as string init '<w:definitions xmlns:w="' + WSDL_NS + '" ' +
        'xmlns:soap="http://schemas.xmlsoap.org/wsdl/soap/" xmlns:t="urn:t" ' +
        'targetNamespace="urn:t"><w:message name="In"><w:part name="p" element="Op"/>' +
        '</w:message><w:portType name="PT"><w:operation name="Op"><w:input message="t:In"/>' +
        '</w:operation></w:portType><w:binding name="B" type="t:PT">' +
        '<soap:binding transport="http://schemas.xmlsoap.org/soap/http"/>' +
        '<w:operation name="Op"><w:input><soap:body use="literal"/></w:input></w:operation>' +
        '</w:binding></w:definitions>';
    testing.assertContains(
        wsdlRequestFailure(wsdlCallTo(parseWsdl($nons), "https://h/", "Op")),
        "is in no namespace");
}

# An import back to the root, which has no location of its own, is read once.
func testWsdlImportBackToRoot() {
    def root as string init strings.replace(
        HEADER_WSDL,
        '<message name="In">',
        '<import namespace="urn:u" location="b.wsdl"/><message name="In">');
    def b as string init '<definitions xmlns="' + WSDL_NS + '" targetNamespace="urn:u">' +
        '<import namespace="urn:t" location="root.wsdl"/></definitions>';
    def w as Wsdl init parseWsdlWith(
        $root,
        WsdlOptions{imports: {"b.wsdl": $b, "root.wsdl": $root}, maxDocuments: 0});
    testing.assertEqual(len($w.ports), 1);
    testing.assertEqual(len($w.bindings), 1);
}

# Many ports on one binding: a missing operation searches that binding once.
func testWsdlManyPortsOneBinding() {
    def ports as list of string init [];
    for (def i in 0..3) {
        $ports[] = '<port name="P' + convert.toString($i) + '" binding="t:B">' +
            '<soap:address location="https://example.com/' + convert.toString($i) + '"/></port>';
    }
    def text as string init strings.replace(
        HEADER_WSDL,
        '<port name="P" binding="t:B"><soap:address location="https://example.com/t"/></port>',
        strings.join($ports, ""));
    def w as Wsdl init parseWsdl($text);
    testing.assertEqual(wsdlCallAt($w, "P2", "Op").address, "https://example.com/2");
    try {
        wsdlCall($w, "Missing");
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "offers the operation \"Missing\"");
    }
}
