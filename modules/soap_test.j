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
            body: convert.stringFromBytes("utf-8", httpd.body($req))
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

func messageOf(e as Error) {
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
    $c = withToken(withInsecure($c), "abc");
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
        [xml.setText(xml.elementNS("urn:x", "m:R"), strings.repeat("x", 4096))]);
    def f as Fake init fake([canned(200, $big)]);
    try {
        send(withMaxBytes(connect($f.url), 1024), "", addOp());
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "POST");
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
        convert.stringFromBytes("utf-8", httpd.body($req)),
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
        def m as string init messageOf($e);
        testing.assertContains($m, "POST http://127.0.0.1:1/svc:");
        testing.assertFalse(strings.contains($m, "s3cret"));
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
    return encoding.toText("hex", hash.compute("md5", convert.bytesFromString("utf-8", $text)));
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
