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

There is no WSDL processing: take the target namespace, the operation and
parameter names, the SOAPAction and the SOAP version from the service's WSDL
or its documentation.

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
| `soap.withCaCert(c, pem)` | `Client` | trust a certificate (PEM bytes) |
| `soap.withInsecure(c)` | `Client` | accept any certificate |
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

## Server

`soap.handle(body, action, handler)` turns a request body into a
`Response{status, contentType, body}` for any transport. `handler` is a
function `func(req as soap.Request)` that returns the result element.
`Request` is `{version, action, headers, body}`, where `body` is the operation
element. The reply uses the request's SOAP version.

```jennifer
use convert;
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
        convert.stringFromBytes("utf-8", httpd.body($req)), $action, add);
    httpd.setHeader($req, "Content-Type", $resp.contentType);
    httpd.respond($req, $resp.status, $resp.body);
}
```

| Function | Returns | Does |
|---|---|---|
| `soap.handle(body, action, handler)` | `Response` | dispatch one request; no header block is understood |
| `soap.handleWith(body, action, handler, understood)` | `Response` | the same; `understood` lists the header blocks the handler processes, by Clark name (`{ns}local`) |
| `soap.requestAction(contentType, soapAction)` | `string` | a request's SOAPAction: the `SOAPAction` header, else the 1.2 `action` parameter, unquoted |
| `soap.senderFault(reason)` | | throw from a handler: answer with a Client / Sender fault |

`handle` answers the protocol errors itself:

| Request | Fault | Status |
|---|---|---|
| malformed XML, a DOCTYPE, no or several Bodies, a Body without exactly one element | `Client` (1.1) / `Sender` (1.2) | 500 / 400 |
| an `Envelope` in another namespace | `VersionMismatch` (as 1.1) | 500 |
| a header block for this node with `mustUnderstand="1"` / `"true"` that is not in `understood` | `MustUnderstand` | 500 |
| the handler calls `senderFault(reason)` | `Client` / `Sender` with `reason` | 500 / 400 |
| the handler throws anything else | `Server` / `Receiver`, reason `internal error` | 500 |

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
| `soap.parse(text)` | `Envelope` | decode an envelope of either version: `{version, headers, body}` |
| `soap.isFault(env)` | `bool` | whether its Body holds one `Fault` |
| `soap.parseFault(env)` | `Fault` | read the fault of either version |
| `soap.faultElement(version, fault)` | `xml.Value` | a `Fault` element |
| `soap.faultEnvelope(version, fault)` | `string` | a fault envelope |
| `soap.contentType(version, action)` | `string` | the `Content-Type` of a message |

`Fault` is `{code, subcode, reason, actor, detail}` for both versions:

- `code`: a code of the envelope namespace without its prefix:
  `VersionMismatch`, `MustUnderstand`, `Client` / `Server` (1.1, also dotted:
  `Client.Authentication`), `Sender` / `Receiver` / `DataEncodingUnknown`
  (1.2). It is resolved against the namespace bindings, so `soap:Client` reads
  as `Client` whatever the prefix. A 1.1 code in another namespace reads as
  `{ns}local`, one in no namespace as written (`Busy`).
- `subcode`: the first 1.2 `Subcode` value, as `{ns}local` or a bare name;
  "" for 1.1.
- `reason`: `faultstring` (1.1) or the first `Reason/Text` (1.2).
- `actor`: `faultactor` (1.1) or `Role` (1.2).
- `detail`: the detail's content, as XML when it holds elements, else as text.

When writing a fault:

- `Client` / `Sender` and `Server` / `Receiver` are mapped to the target
  version.
- A 1.2 `code` must be one of the five standard codes. An application code
  goes in `subcode`, which is written unqualified or with its `{ns}`.
- A 1.1 code that is not standard is written without the envelope prefix.
- A `detail` that starts with `<` is parsed as XML.
- Characters XML 1.0 does not allow are written as `?` in `reason`, `actor`
  and a text `detail`.

`faultEnvelope` and `parseFault` are inverses: a fault written and read
again comes back equal.

The constants `SOAP11_NS` and `SOAP12_NS` are the envelope namespaces.

## Errors and safety

Every failure is `Error{kind: "soap"}`: `soap: HTTP 503 Service Unavailable
without a SOAP reply`, `soap: fault Client: ...`. Strings from the server
are cut to 200 characters and stripped of control characters in messages,
so a fault reason cannot inject terminal escapes.

- A message with a DOCTYPE is refused, as both SOAP versions require.
  Entities beyond the predefined five are never resolved, so a reply can
  neither read a local file nor expand into gigabytes.
- The reply body is capped (16 MiB by default, `withMaxBytes`). A decoded
  reply takes up to about 15 times its size in memory.
- Redirects are not followed and nothing is retried: a call may not be
  idempotent.
- `operation` refuses an empty namespace and a value holding a character XML
  1.0 does not allow.
- `withSoapHeader` takes a namespaced element only.
- `connect` refuses credentials, a fragment and control characters in the
  URL; use `withBasicAuth`, `withToken` or `withDigestAuth` (the last one set
  wins). Digest answers only the endpoint itself; a redirect is not followed
  for the challenge. A transport error names the endpoint without its
  query, which may carry a key.
- `withHeader` refuses the headers the client sets itself (`Content-Type`,
  `SOAPAction`) and those that frame a request (`Host`, `Content-Length`,
  ...). A SOAPAction with a quote, a backslash or a control character is
  refused.
- `handle` hides the handler's internal errors behind a generic `Server` /
  `Receiver` fault.

Not covered: WSDL, SOAP encoding (`soap-enc` arrays and multi-ref values),
attachments (SwA, MTOM), and WS-* processing. WS-Security headers can be
built with `xml` and sent with `withSoapHeader`.
