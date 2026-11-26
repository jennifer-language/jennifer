# `soap` - SOAP 1.1 / 1.2 client and server

Import with `import "soap.j" as soap;`. **SOAP over HTTP**, versions 1.1 and
1.2: a client that calls an operation and returns its result element, and a
transport-agnostic server dispatcher. Payloads are `xml.Value` elements, built
with `soap.operation` or the [`xml`](../libraries/xml.md) builders. Over
`http` + `xml`; the client needs the default **`jennifer`** binary (`net` via
`http`).

```jennifer
use io;
use xml;
import "soap.j" as soap;

def c as soap.Client init soap.connect("https://example.com/Calculator.asmx");
def args as map of string to string init {"intA": "2", "intB": "3"};
def op as xml.Value init soap.operation("http://tempuri.org/", "Add", $args, true);
def result as xml.Value init soap.call($c, "http://tempuri.org/Add", $op);
io.printf("%s\n", soap.values($result)["AddResult"]);    # 5
```

[WSDL](#wsdl) 1.1 descriptions are read for their ports, bindings and
operations: the endpoint, the SOAP version, each operation's SOAPAction and
the element its request carries. The schema types are not read, so the
parameter names still come from the service's documentation.

Runnable: [`examples/modules/soap_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/soap_demo.j).

## Client

The `Client` is a value; the `with*` functions return a changed copy.

| Function | Returns | Does |
|---|---|---|
| `soap.connect(endpoint)` | `Client` | a client for an `http(s)://` URL (no credentials, fragment or control character in it); SOAP 1.1, 30 s idle timeout, 16 MiB response cap |
| `soap.withVersion(c, version)` | `Client` | `"1.1"` or `"1.2"` |
| `soap.withBasicAuth(c, user, password)` | `Client` | HTTP Basic |
| `soap.withToken(c, token)` | `Client` | `Authorization: Bearer` |
| `soap.withDigestAuth(c, user, password)` | `Client` | HTTP Digest (RFC 7616): each call answers the endpoint's `401` challenge, one extra round trip (`http.sendDigest`) |
| `soap.withHeader(c, name, value)` | `Client` | another HTTP header on every call |
| `soap.withSoapHeader(c, block)` | `Client` | a header block (an `xml.Value`) in every envelope, e.g. WS-Security |
| `soap.withCA(c, pem)` | `Client` | trust a certificate (PEM bytes) |
| `soap.withSkipVerify(c)` | `Client` | accept any certificate |
| `soap.withTimeout(c, ms)` | `Client` | the idle timeout; 0 for 30 s |
| `soap.withMaxBytes(c, n)` | `Client` | the response-body cap; 0 for 16 MiB |
| `soap.call(c, action, body)` | `xml.Value` | post `body` and return the reply's first Body child; a fault throws |
| `soap.send(c, action, body)` | `Reply` | post `body` and return the reply, a fault included |

`action` is the SOAPAction URI, `""` for none. SOAP 1.1 sends it as the
quoted `SOAPAction` header with `Content-Type: text/xml`; SOAP 1.2 sends it
as the `action` parameter of `Content-Type: application/soap+xml`.

`call` throws `soap: fault CODE: REASON` on a fault. To read the fault's
subcode, actor or detail, use `send`:

```jennifer
def r as soap.Reply init soap.send($c, "http://tempuri.org/Add", $op);
if ($r.isFault) {
    io.printf("%s: %s\n%s\n", $r.fault.code, $r.fault.reason, $r.fault.detail);
}
```

`Reply` is `{status, envelope, isFault, fault}`. A 2xx reply with no body (a
one-way operation, often `202 Accepted`) gives an envelope with an empty
`body`.

### Building and reading payloads

| Function | Returns | Does |
|---|---|---|
| `soap.operation(ns, name, params, qualified)` | `xml.Value` | `<m:name xmlns:m="ns">` with one text child per `params` entry, in order |
| `soap.values(node)` | `map of string to string` | an element's children by local name: the text, or the XML of a child that holds elements |

`qualified` puts the parameter elements in `ns` too, as a schema with
`elementFormDefault="qualified"` expects (.NET services). `false` leaves them
unqualified (the JAX-WS default). For nested or repeated parameters, build
the element with `xml.elementNS` / `xml.append`. Of repeated names `values`
keeps the first; read those with `xml.findAll`.

## WSDL

`parseWsdl` reads a WSDL 1.1 document; `parseWsdlWith` also takes the
documents it imports, in `WsdlOptions.imports`, a map from each `wsdl:import`
`location` to the text (nothing is fetched). Only SOAP 1.1 and 1.2 bindings over HTTP and their
ports are kept; HTTP GET / POST and MIME bindings, and SOAP over another
transport (JMS, ...), are left out.

```jennifer
use fs;
use io;
use xml;
import "soap.j" as soap;

def w as soap.Wsdl init soap.parseWsdl(fs.readString("checkVatService.wsdl"));
def target as soap.WsdlCall init soap.wsdlCall($w, "checkVat");
def body as xml.Value init soap.wsdlRequest(
    $target, {"countryCode": "DE", "vatNumber": "123456789"});
def result as xml.Value init soap.call(soap.wsdlConnect($target), $target.operation.action, $body);
io.printf("%s\n", soap.values($result)["valid"]);
```

| Function | Returns | Does |
|---|---|---|
| `soap.parseWsdl(text)` | `Wsdl` | a description that imports no other WSDL |
| `soap.parseWsdlWith(text, o)` | `Wsdl` | `o` a `WsdlOptions{imports, maxDocuments}`. `imports`: `location` -> document text; an import of an import is looked up in the same map, each location and each document text read once (an import back to the importing document is no error). `maxDocuments`: 0 is `MAX_WSDL_DOCUMENTS` (32), a value may only lower it; past it is `"limit"` |
| `soap.wsdlCall(w, name)` | `WsdlCall` | the operation at the first SOAP port that offers it |
| `soap.wsdlCallAt(w, port, name)` | `WsdlCall` | the operation at the named port |
| `soap.wsdlCallTo(w, address, name)` | `WsdlCall` | the operation from the first SOAP binding that has it, sent to `address`: for a description with no `service` (ONVIF's device WSDLs) or whose ports name another host |
| `soap.wsdlConnect(target)` | `Client` | `connect` to the address with the binding's SOAP version |
| `soap.wsdlRequest(target, params)` | `xml.Value` | the request body with one text child per parameter: document/literal the part's element (children qualified when an inline schema sets `elementFormDefault="qualified"`), rpc/literal a wrapper named after the operation in the `soap:body` namespace |

| Struct | Fields |
|---|---|
| `Wsdl` | `name`, `targetNamespace`, `ports`, `bindings` |
| `WsdlPort` | `service`, `name`, `address`, `version` (`1.1` / `1.2`), `binding`, `bindingNamespace` |
| `WsdlBinding` | `name`, `namespace`, `version`, `style`, `operations` |
| `WsdlOperation` | `name`, `action`, `style` (`document` / `rpc`), `bodyUse` (`literal` / `encoded`), `namespace` (rpc only), `input` / `output` (the Body's parts: the message's, or those `soap:body parts` names), `inputHeaders` / `outputHeaders` (the parts `soap:header` names) |
| `WsdlPart` | `name`, `elementNamespace` + `element` or `typeNamespace` + `typeName`, `qualified` |
| `WsdlCall` | `address`, `version`, `operation` |

The reader is strict: a DOCTYPE, a root other than WSDL 1.1 `definitions`
(WSDL 2.0 is refused by name), a missing required attribute, a QName with an
undeclared prefix, a name defined twice, an overloaded operation, a style
other than `document` / `rpc`, a binding with two SOAP binding elements or
none with a `transport`, a port whose binding is not SOAP or is of the other
SOAP version, a reference to an undefined message, port type, binding or
part, and an import missing from `imports` all throw `SoapError` kind
`"soap"`. `wsdlRequest` refuses an `encoded` operation (the module writes no
SOAP encoding), a document operation whose Body is not one element part, and
an element in no namespace; build those bodies with the `xml` builders. XML Schema imports (`xs:import`) are not followed.

## Server

`soap.handle(body, action, handler)` turns a request body into a
`Response{status, contentType, body}` for any transport. `handler` is a
function `func(req as soap.Request)` that returns the result element.
`Request` is `{version, action, headers, body}`, where `body` is the operation
element. The reply uses the request's SOAP version.

```jennifer
use convert;
use encoding;
use httpd;
use xml;
import "soap.j" as soap;

func add(req as soap.Request) {
    if (xml.localName($req.body) != "Add") {
        soap.senderFault("unknown operation");
    }
    def args as map of string to string init soap.values($req.body);
    def sum as int init convert.toInt($args["intA"]) + convert.toInt($args["intB"]);
    def out as xml.Value init xml.elementNS("http://tempuri.org/", "m:AddResponse");
    return xml.append($out, xml.setText(xml.element("AddResult"), convert.toString($sum)));
}

def srv as httpd.Server init httpd.listen("127.0.0.1:8080");
while (true) {
    def req as httpd.Request init httpd.accept($srv);
    def action as string init soap.requestAction(
        httpd.header($req, "Content-Type"), httpd.header($req, "SOAPAction"));
    def resp as soap.Response init soap.handle(
        encoding.decode("utf-8", httpd.body($req)), $action, add);
    httpd.setHeader($req, "Content-Type", $resp.contentType);
    httpd.respond($req, $resp.status, $resp.body);
}
```

| Function | Returns | Does |
|---|---|---|
| `soap.handle(body, action, handler)` | `Response` | dispatch one request; no header block is understood |
| `soap.handleWith(body, action, handler, understood)` | `Response` | the same; `understood` lists the header blocks the handler processes, by Clark name (`{ns}local`) |
| `soap.requestAction(contentType, soapAction)` | `string` | a request's SOAPAction: the `SOAPAction` header, else the 1.2 `action` parameter, unquoted (parameters are read in turn, so `action=` inside another one's quoted value is not taken) |
| `soap.senderFault(reason)` | | throw from a handler: answer with a Client / Sender fault |

`handle` answers the protocol errors itself:

| Request | Fault | Status |
|---|---|---|
| malformed XML, a DOCTYPE, no or several Bodies, a Header that is not the first element, a header block in no namespace, text directly in the Envelope or Body, a Body without exactly one element | `Client` (1.1) / `Sender` (1.2) | 500 / 400 |
| a `mustUnderstand` other than `1` / `0` (1.1) or `true` / `false` / `1` / `0` (1.2) | `Client` / `Sender` | 500 / 400 |
| an `Envelope` in another namespace | `VersionMismatch` (as 1.1) | 500 |
| a header block for this node with `mustUnderstand="1"` (or `"true"` in 1.2) that is not in `understood` | `MustUnderstand` | 500 |
| the handler calls `senderFault(reason)` | `Client` / `Sender` with `reason` | 500 / 400 |
| the handler throws anything else, a value that is not an `Error` included | `Server` / `Receiver`, reason `internal error` | 500 |

`handle` acts as the ultimate receiver: a block is for it when it has no
`actor` (1.1) / `role` (1.2), the 1.1 `next` actor, or the 1.2 `next` or
`ultimateReceiver` role. A block for another node, or the 1.2 `none` role, is
left alone.

The handler's own error message never reaches the caller; log it in the
handler. A handler that returns something other than an `xml.Value`, or has
the wrong parameters, also gives `internal error`. Cap the request body in
the transport (`httpd` has its own limit).

## The protocol layer

The parts that do not send anything are exported, for a server on another
transport or for a message stored or queued:

| Function | Returns | Does |
|---|---|---|
| `soap.envelope(version, headers, body)` | `string` | an envelope with an XML declaration; `headers` (header blocks) and `body` are `list of xml.Value` |
| `soap.parse(text)` | `Envelope` | decode an envelope of either version: `{version, headers, body}`; refuses a `Fault` with other elements beside it, a `Header` that is not first, a header block in no namespace, and text directly in the Envelope or Body |
| `soap.isFault(env)` | `bool` | whether its Body holds one `Fault` |
| `soap.parseFault(env)` | `Fault` | read the fault of either version |
| `soap.faultElement(version, fault)` | `xml.Value` | a `Fault` element |
| `soap.faultEnvelope(version, fault)` | `string` | a fault envelope |
| `soap.contentType(version, action)` | `string` | the `Content-Type` of a message |

`Fault` is `{code, subcode, reason, actor, detail}` for both versions:

| Field | 1.1 | 1.2 |
|---|---|---|
| `code` | `VersionMismatch`, `MustUnderstand`, `Client` / `Server` (also dotted: `Client.Authentication`); a code in another namespace reads as `{ns}local`, one in no namespace as written (`Busy`) | `VersionMismatch`, `MustUnderstand`, `Sender` / `Receiver` / `DataEncodingUnknown` |
| `subcode` | `""` | the first `Subcode` value, as `{ns}local` or a bare name |
| `reason` | `faultstring` | the first `Reason/Text` |
| `actor` | `faultactor` | `Role` |
| `detail` | the detail's content, as XML when it holds elements, else as text; for elements mixed with text, the whole detail element's XML | the same |

`code` is resolved against the namespace bindings and has no prefix, so
`soap:Client` reads as `Client` whatever the prefix.

When writing a fault:

- `Client` / `Sender` and `Server` / `Receiver` are mapped to the target
  version.
- A 1.2 `code` must be one of the five standard codes. An application code
  goes in `subcode`, which is written unqualified or with its `{ns}`.
- A 1.1 code that is not standard is written without the envelope prefix.
- A `detail` that starts with `<` is parsed as XML, text between its elements
  included. It must declare the prefixes it uses.
- Characters XML 1.0 does not allow are written as `?` in `reason`, `actor`
  and a text `detail`.

`faultEnvelope` and `parseFault` are inverses: a fault written and read
again comes back equal.

| Constant | Value |
|---|---|
| `soap.SOAP11_NS` | `http://schemas.xmlsoap.org/soap/envelope/` |
| `soap.SOAP12_NS` | `http://www.w3.org/2003/05/soap-envelope` |
| `soap.ERROR_KIND` | `"soap"`, the kind of every `SoapError` |
| `soap.FAULT_KIND` | `"soapFault"`, the kind `senderFault` throws |
| `soap.DEFAULT_MAX_BYTES` | `16777216`, a new client's response cap |

## Errors and safety

Every error of kind `"soap"` is a `soap.SoapError`: `soap: HTTP 503 Service
Unavailable without a SOAP reply`, `soap: fault Client: ...`. Its `status` is
the reply's HTTP status (0 when none was read); a fault from `call` sets
`isFault` and the parsed `fault` (code, subcode, reason, actor, detail), so a
handler reads the fault code without parsing the message. Strings from the
server are cut to 200 characters and stripped of control characters in
messages, so a fault reason cannot inject terminal escapes.

- A message with a DOCTYPE is refused, as both SOAP versions require.
  Entities beyond the predefined five are never resolved, so a reply can
  neither read a local file nor expand into gigabytes.
- The reply body is capped (16 MiB by default, `withMaxBytes`); a larger one
  is an `Error` of kind `"limit"` (`soap: POST <endpoint>: ...`), and so is
  a reply past `xml`'s decoding caps. A decoded reply takes up to about 15
  times its size in memory.
- A server that does not answer within the idle timeout is an `Error` of
  kind `"timeout"`, one that closes the connection under the call kind
  `"closed"`, both `soap: POST <endpoint>: ...`; a cancelled task stays
  `"cancelled"`, and a socket failure (refused, reset, DNS, TLS) stays
  `http`'s `net.NetError` (kind `"net"`). Every other transport failure is a
  `SoapError`.
- Redirects are not followed and nothing is retried: a call may not be
  idempotent.
- `operation` refuses an empty or reserved (`xml`, `xmlns`) namespace, one
  with a control character, and a value holding a character XML 1.0 does not
  allow; so does a fault code's `{ns}`.
- `withSoapHeader` takes a namespaced element only.
- `connect` refuses credentials, a fragment and control characters in the
  URL; use `withBasicAuth`, `withToken` or `withDigestAuth` (the last one set
  wins). Digest answers only the endpoint itself; a redirect is not followed
  for the challenge. A transport error names the endpoint without its
  query, which may carry a key.
- A client with credentials (`withBasicAuth`, `withToken`, `withDigestAuth`,
  an `Authorization` or `Proxy-Authorization` header from `withHeader`) throws a `SoapError` (status
  0) before connecting when the endpoint is `http://` on a host other than
  loopback (`127.0.0.0/8`, `[::1]`, `localhost`). For plain HTTP on a trusted
  LAN, set the client's `allowInsecureHttp` field
  (`$c.allowInsecureHttp = true;`). Digest sends no reusable secret, but a
  listener can crack its response offline, so it is refused too.
- `withHeader` replaces a header of the same name in any letter case, and
  `withBasicAuth` / `withToken` replace any `Authorization` header set before.
- `withHeader` refuses the headers the client sets itself (`Content-Type`,
  `SOAPAction`) and those that frame a request (`Host`, `Content-Length`,
  ...). A SOAPAction with a quote, a backslash or a control character is
  refused.
- `handle` hides the handler's internal errors behind a generic `Server` /
  `Receiver` fault.

Not covered: WSDL 2.0, reading XML Schema types, SOAP encoding (`soap-enc` arrays and multi-ref values),
attachments (SwA, MTOM), and WS-* processing. WS-Security headers can be
built with `xml` and sent with `withSoapHeader`.

## Academy talks

| Talk | What it shows |
| ---- | ------------- |
| [Invoicing with web services: VAT, SOAP and GraphQL](https://academy.jennifer-lang.dev/talks/mplx/invoicing/) | calling the VIES VAT check by hand, without the WSDL reader |

## API reference

Every exported function, struct and constant of `soap`, with its signature, generated
from the module's doc comments: [the `soap` API reference](../api/soap.md).
