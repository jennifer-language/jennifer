#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

/**
 * A self-contained SOAP demo: it starts a small calculator service on a local
 * port with `httpd` and `soap.handle` (so the example runs with no network),
 * then uses the `soap` client to call an operation over SOAP 1.1 and 1.2, read
 * a fault in full with `soap.send`, and catch the fault `soap.call` throws as a
 * `soap.SoapError`, whose fields carry the HTTP status and the parsed fault.
 * Against a real service you would `soap.connect` its endpoint URL and skip
 * the local server.
 * @module soap_demo
 */
use io;
use convert;
use encoding;
use httpd;
use task;
use xml;
import "../../modules/soap.j" as soap;

def const NS as string init "http://tempuri.org/";

/**
 * The calculator's handler: Add and Divide on two integers. A division by zero
 * or an unknown operation is the caller's fault, so it answers with a Client
 * (1.1) / Sender (1.2) fault.
 * @param req {soap.Request} the decoded request; `body` is the operation element
 * @return {xml.Value} the response element
 */
func calculator(req as soap.Request) {
    def op as string init xml.localName($req.body);
    def args as map of string to string init soap.values($req.body);
    def a as int init convert.toInt($args["intA"]);
    def b as int init convert.toInt($args["intB"]);
    def n as int init 0;
    if ($op == "Add") {
        $n = $a + $b;
    } elseif ($op == "Divide") {
        if ($b == 0) {
            soap.senderFault("division by zero");
        }
        $n = $a // $b;
    } else {
        soap.senderFault("unknown operation " + $op);
    }
    def out as xml.Value init xml.elementNS(NS, "m:" + $op + "Response");
    return xml.append($out, xml.setText(xml.element($op + "Result"), convert.toString($n)));
}

/**
 * Serve SOAP requests until the server is shut down.
 * @param srv {httpd.Server} the listening server
 */
func serve(srv as httpd.Server) {
    while (true) {
        try {
            def req as httpd.Request init httpd.accept($srv);
            def action as string init soap.requestAction(
                httpd.header($req, "Content-Type"),
                httpd.header($req, "SOAPAction"));
            def resp as soap.Response init soap.handle(
                encoding.decode("utf-8", httpd.body($req)),
                $action,
                calculator);
            httpd.setHeader($req, "Content-Type", $resp.contentType);
            httpd.respond($req, $resp.status, $resp.body);
        } catch (acceptErr) {
            return;
        }
    }
}

def srv as httpd.Server init httpd.listen("127.0.0.1:0");
def server as task of null init spawn {
    serve($srv);
};
def c as soap.Client init soap.connect("http://" + httpd.address($srv) + "/Calculator.asmx");

# The envelope a call sends, built by the protocol layer.
def add as xml.Value init soap.operation(NS, "Add", {"intA": "2", "intB": "3"}, true);
io.printf("=== request envelope (SOAP 1.1) ===\n%s\n\n", soap.envelope("1.1", [], [$add]));

# A call returns the reply's first Body child; values reads its children.
def result as xml.Value init soap.call($c, NS + "Add", $add);
io.printf("Add(2, 3) = %s\n", soap.values($result)["AddResult"]);

# The same service over SOAP 1.2: the reply uses the request's version.
def c12 as soap.Client init soap.withVersion($c, "1.2");
def div as xml.Value init soap.operation(NS, "Divide", {"intA": "17", "intB": "5"}, true);
io.printf(
    "Divide(17, 5) = %s (SOAP 1.2)\n",
    soap.values(soap.call($c12, NS + "Divide", $div))["DivideResult"]);

# send returns the reply, a fault included, without throwing.
def byZero as xml.Value init soap.operation(NS, "Divide", {"intA": "1", "intB": "0"}, true);
def reply as soap.Reply init soap.send($c, NS + "Divide", $byZero);
io.printf(
    "\nsend: HTTP %d, fault %v, code %s, reason %s\n",
    $reply.status,
    $reply.isFault,
    $reply.fault.code,
    $reply.fault.reason);

# call throws a fault as a soap.SoapError: read its fields, never its message.
try {
    soap.call($c12, NS + "Divide", $byZero);
    io.printf("unexpected success\n");
} catch (e) {
    if ($e.kind == soap.ERROR_KIND and $e.isFault) {
        io.printf("call: caught %s (HTTP %d)\n", $e.message, $e.status);
        io.printf("  fault code %s, reason %s\n", $e.fault.code, $e.fault.reason);
    } else {
        throw $e;
    }
}

# The handler refuses an operation it does not know with a fault too.
def mul as xml.Value init soap.operation(NS, "Multiply", {"intA": "6", "intB": "7"}, true);
try {
    soap.call($c, NS + "Multiply", $mul);
} catch (e) {
    if ($e.kind == soap.ERROR_KIND and $e.isFault) {
        io.printf("call: %s fault: %s\n", $e.fault.code, $e.fault.reason);
    } else {
        throw $e;
    }
}

httpd.shutdown($srv);
task.wait($server);
