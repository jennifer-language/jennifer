# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.28.0

/**
 * SOAP 1.1 and 1.2 over HTTP: a client that calls an operation and returns its
 * result element, and a transport-agnostic server dispatcher.
 *
 * The **protocol layer** (`envelope`, `parse`, `Fault`, `parseFault`,
 * `faultEnvelope`) knows nothing about sockets. The **client** posts
 * envelopes over `http`; the **server** side, `handle`, turns a request body
 * into a reply body for any transport, such as an `httpd` loop. Payloads are
 * `xml.Value` elements: build them with `operation` or the `xml` builders and
 * read them with `values` or the `xml` accessors. `parseWsdl` reads a WSDL 1.1
 * description's ports, bindings and operations (SOAPAction, style, message
 * parts), and `wsdlCall` / `wsdlConnect` / `wsdlRequest` turn one operation into
 * a client and a request body; schema types are not read. Needs the default
 * `jennifer` binary (`net` via `http`).
 *
 * @module soap
 * @example
 * def c as soap.Client init soap.connect("https://example.com/Calculator.asmx");
 * def args as map of string to string init {"intA": "2", "intB": "3"};
 * def op as xml.Value init soap.operation("http://tempuri.org/", "Add", $args, true);
 * def result as xml.Value init soap.call($c, "http://tempuri.org/Add", $op);
 * io.printf("%s\n", soap.values($result)["AddResult"]);
 */
use convert;
use encoding;
use lists;
use maps;
use regex;
use strings;
use xml;
import "./http.j" as http;

/** The `kind` of every error the module throws. */
export def const ERROR_KIND as string init "soap";

/**
 * The `kind` of an error a `handle` handler throws (see `senderFault`) to
 * answer with a Client (1.1) / Sender (1.2) fault carrying its message.
 */
export def const FAULT_KIND as string init "soapFault";

/** The SOAP 1.1 envelope namespace. */
export def const SOAP11_NS as string init "http://schemas.xmlsoap.org/soap/envelope/";

/** The SOAP 1.2 envelope namespace. */
export def const SOAP12_NS as string init "http://www.w3.org/2003/05/soap-envelope";

/** The response-body cap a Client starts with: 16 MiB. */
export def const DEFAULT_MAX_BYTES as int init 16777216;

# The idle timeout a Client starts with, and what `withTimeout(c, 0)` restores.
def const DEFAULT_TIMEOUT_MS as int init 30000;

# The SOAP 1.2 roles `handle` acts in.
def const ROLE_BASE as string init "http://www.w3.org/2003/05/soap-envelope/role/";
def const ROLE_NEXT as string init ROLE_BASE + "next";
def const ROLE_ULTIMATE as string init ROLE_BASE + "ultimateReceiver";

# The SOAP 1.1 actor of the next node, which `handle` is.
def const ACTOR_NEXT as string init "http://schemas.xmlsoap.org/soap/actor/next";

# The fault codes in the envelope namespace.
def const CODES11 as list of string init ["VersionMismatch", "MustUnderstand", "Client", "Server"];
def const CODES12 as list of string init [
    "VersionMismatch",
    "MustUnderstand",
    "DataEncodingUnknown",
    "Sender",
    "Receiver"
];

# Characters XML 1.0 does not allow, which `xml.setText` / `xml.setAttr` refuse.
def const XML_INVALID_RE as string init '[\x00-\x08\x0b\x0c\x0e-\x1f\x{FFFE}\x{FFFF}]';

# A control character, refused in URLs, header values and SOAPActions.
def const CONTROL_RE as string init '[\x00-\x1f\x7f]';

# How much of a server's or caller's string an error message repeats.
def const SHOWN as int init 200;

# An XML name without a colon (NCName, ASCII subset).
def const NCNAME_RE as string init '^[A-Za-z_][A-Za-z0-9._-]*$';

# A DOCTYPE before the root element. SOAP messages must not carry one, and
# `xml.decode` drops it silently.
def const DOCTYPE_RE as string init '(?s)^\x{FEFF}?\s*(<\?.*?\?>\s*|<!--.*?-->\s*)*<!DOCTYPE';

# One `; name=value` parameter of a Content-Type, at the start of the rest of
# the header: a token name, then a token or a quoted string (RFC 9110), which
# may hold `;` and backslash escapes.
def const PARAM_RE as string init '^;\s*([A-Za-z0-9!#$%&*+.^_`|~-]+)\s*=\s*' +
    '("(?:[^"\\]|\\.)*"|[^;\s"]*)\s*';

# The XML and XMLNS namespaces, which no element or QName may be put in.
def const XML_NS as string init "http://www.w3.org/XML/1998/namespace";
def const XMLNS_NS as string init "http://www.w3.org/2000/xmlns/";

# Headers the client sets itself, which `withHeader` refuses.
def const OWN_HEADERS as list of string init [
    "host",
    "content-length",
    "transfer-encoding",
    "connection",
    "upgrade",
    "te",
    "trailer",
    "content-type",
    "soapaction"
];

/**
 * A SOAP fault, as `parseFault` reads it and `faultEnvelope` writes it, for
 * either version.
 *
 * @field code {string} a code of the envelope namespace without prefix:
 *   `VersionMismatch`, `MustUnderstand`, `Client` / `Server` (1.1, also dotted
 *   as `Client.Auth`), `Sender` / `Receiver` / `DataEncodingUnknown` (1.2).
 *   Otherwise (1.1 only) a code in another namespace as `{ns}local`, or one
 *   in no namespace as written
 * @field subcode {string} the first 1.2 `Subcode` value, as `{ns}local` or a
 *   bare name; "" for none and for 1.1
 * @field reason {string} `faultstring` (1.1) or the first `Reason/Text` (1.2)
 * @field actor {string} `faultactor` (1.1) or `Role` (1.2); "" for none
 * @field detail {string} the detail's content: its XML when it holds elements
 *   (text mixed in written in place), else its text; "" for none. Read from a
 *   detail that mixes text with elements, it is the whole detail element
 */
export def struct Fault {
    code as string,
    subcode as string,
    reason as string,
    actor as string,
    detail as string
};

/**
 * A decoded envelope.
 *
 * @field version {string} `1.1` or `1.2`
 * @field headers {list of xml.Value} the header blocks (the `Header`'s
 *   children)
 * @field body {list of xml.Value} the `Body`'s children: the operation or
 *   result element, or one `Fault`
 */
export def struct Envelope {
    version as string,
    headers as list of xml.Value,
    body as list of xml.Value
};

/**
 * The outcome of `send`.
 *
 * @field status {int} the HTTP status
 * @field envelope {Envelope} the reply; an empty body for a bodiless 2xx
 *   (a one-way operation)
 * @field isFault {bool} whether the reply is a fault
 * @field fault {Fault} the fault when `isFault`
 */
export def struct Reply {
    status as int,
    envelope as Envelope,
    isFault as bool,
    fault as Fault
};

/**
 * The user and password a `Client` answers an HTTP Digest challenge with.
 *
 * @field user {string} the user name; "" for no Digest auth
 * @field password {string} the password
 */
export def struct DigestCredentials {
    user as string,
    password as string
};

/**
 * A SOAP endpoint and how to reach it. Value-semantic: the `with*` functions
 * return a changed copy.
 *
 * @field endpoint {string} the service URL
 * @field version {string} `1.1` or `1.2`
 * @field headers {map of string to string} HTTP headers sent with every call
 * @field soapHeaders {list of xml.Value} header blocks sent with every call
 * @field tls {http.TlsOptions} certificate checking
 * @field timeoutMs {int} the idle timeout of each call
 * @field maxBytes {int} the response-body cap
 * @field digest {DigestCredentials} HTTP Digest credentials (`withDigestAuth`);
 *   an empty user for none
 * @field allowInsecureHttp {bool} allow credentials (Basic, bearer, Digest,
 *   an `Authorization` header from `withHeader`) over plain `http://` to a
 *   non-loopback host (default false)
 */
export def struct Client {
    endpoint as string,
    version as string,
    headers as map of string to string,
    soapHeaders as list of xml.Value,
    tls as http.TlsOptions,
    timeoutMs as int,
    maxBytes as int,
    digest as DigestCredentials,
    allowInsecureHttp as bool
};

/**
 * A request as `handle` passes it to the handler.
 *
 * @field version {string} `1.1` or `1.2`
 * @field action {string} the SOAPAction, as given to `handle`
 * @field headers {list of xml.Value} the header blocks
 * @field body {xml.Value} the operation element (the `Body`'s one child)
 */
export def struct Request {
    version as string,
    action as string,
    headers as list of xml.Value,
    body as xml.Value
};

/**
 * What `handle` answers: send it as the HTTP response.
 *
 * @field status {int} 200, or 400 / 500 for a fault
 * @field contentType {string} the `Content-Type` header
 * @field body {string} the envelope
 */
export def struct Response {
    status as int,
    contentType as string,
    body as string
};

/**
 * A failed SOAP operation. Every error of kind `"soap"` is one, so its fields
 * are safe to read once the kind matches. `call` throws it on a fault, with the
 * parsed `fault`; `send` returns a fault instead of throwing.
 * @field kind {string} always `"soap"` (`ERROR_KIND`)
 * @field message {string} `soap: ` and what failed (`soap: fault CODE: REASON` for a fault)
 * @field status {int} the HTTP status of the reply; 0 when there was none (a malformed message, a local check, a transport failure)
 * @field isFault {bool} whether the server answered with a SOAP fault
 * @field fault {Fault} the fault when `isFault`; empty fields otherwise
 * @example
 * try {
 *     def result as xml.Value init soap.call($c, $action, $op);
 * } catch (e) {
 *     if ($e.kind == "soap" and $e.isFault and $e.fault.code == "Client") {
 *         io.printf("the request was refused: %s\n", $e.fault.reason);
 *     } else {
 *         throw $e;
 *     }
 * }
 */
export def struct SoapError {
    kind as string,
    message as string,
    status as int,
    isFault as bool,
    fault as Fault
};

func fail(message as string) {
    failStatus($message, 0);
}

# failStatus throws the SoapError of a failed reply, keeping its HTTP status.
func failStatus(message as string, status as int) {
    throw SoapError{
        kind: ERROR_KIND,
        message: "soap: " + $message,
        status: $status,
        isFault: false,
        fault: Fault{code: "", subcode: "", reason: "", actor: "", detail: ""}
    };
}

# plain cuts a string for an error message and replaces anything but printable
# ASCII, so a message stays one line and carries no terminal escape.
func plain(text as string) {
    def cut as string init $text;
    def more as string init "";
    if (len($text) > SHOWN) {
        $cut = $text[0..SHOWN];
        $more = "...";
    }
    return regex.replace("[^ -~]", $cut, "?") + $more;
}

func shown(text as string) {
    return "\"" + plain($text) + "\"";
}

# xmlSafe replaces the characters XML 1.0 does not allow, for text the module
# writes on its own behalf (fault reasons), so a reply stays well-formed.
func xmlSafe(text as string) {
    return regex.replace(XML_INVALID_RE, $text, "?");
}

# checkAction refuses a SOAPAction that would break out of its quotes.
func checkAction(action as string) {
    if (strings.contains($action, "\"") or strings.contains($action, "\\") or
        regex.matches(CONTROL_RE, $action)) {
        fail("a SOAPAction holds no quote, backslash or control character, got " + shown($action));
    }
}

# checkNamespace refuses a namespace URI an element or QName cannot be put in:
# empty, holding a control character, or the reserved XML / XMLNS namespace.
func checkNamespace(ns as string, what as string) {
    if ($ns == "" or regex.matches(CONTROL_RE, $ns) or regex.matches(XML_INVALID_RE, $ns) or
        $ns == XML_NS or $ns == XMLNS_NS) {
        fail($what + " is a non-empty namespace URI without control characters, other than " +
            "the reserved XML ones, got " + shown($ns));
    }
}

# --- protocol layer --------------------------------------------------------

/**
 * The envelope namespace of a version.
 *
 * @param version {string} `1.1` or `1.2`
 * @return {string} the namespace URI
 */
func namespaceOf(version as string) {
    if ($version == "1.1") {
        return SOAP11_NS;
    }
    if ($version == "1.2") {
        return SOAP12_NS;
    }
    fail("the version is 1.1 or 1.2, got " + shown($version));
    return "";
}

/**
 * The version of an envelope namespace.
 *
 * @param ns {string} the namespace URI
 * @return {string} `1.1`, `1.2`, or "" for another namespace
 */
func versionOf(ns as string) {
    if ($ns == SOAP11_NS) {
        return "1.1";
    }
    if ($ns == SOAP12_NS) {
        return "1.2";
    }
    return "";
}

/**
 * The `Content-Type` of a message.
 *
 * @param version {string} `1.1` or `1.2`
 * @param action {string} the SOAPAction; carried here only for 1.2, "" for
 *   none
 * @return {string} `text/xml; charset=utf-8` (1.1) or
 *   `application/soap+xml; charset=utf-8[; action="..."]` (1.2)
 * @throws {SoapError} kind `"soap"` on an unknown version, or an action with a
 *   quote, backslash or control character
 */
export func contentType(version as string, action as string) {
    checkAction($action);
    if (namespaceOf($version) == SOAP11_NS) {
        return "text/xml; charset=utf-8";
    }
    if ($action == "") {
        return "application/soap+xml; charset=utf-8";
    }
    return "application/soap+xml; charset=utf-8; action=\"" + $action + "\"";
}

/**
 * The SOAPAction of a received request: the `SOAPAction` header (1.1), else
 * the `action` parameter of the `Content-Type` (1.2), unquoted.
 *
 * @param contentType {string} the `Content-Type` header; "" when absent
 * @param soapAction {string} the `SOAPAction` header; "" when absent
 * @return {string} the action; "" for none
 */
export func requestAction(contentType as string, soapAction as string) {
    def a as string init strings.trim($soapAction);
    if ($a == "") {
        return actionParam($contentType);
    }
    if (len($a) >= 2 and strings.startsWith($a, "\"") and strings.endsWith($a, "\"")) {
        $a = $a[1..len($a) - 1];
    }
    return $a;
}

# actionParam is the `action` parameter of a Content-Type, unquoted; "" when
# there is none or the parameters do not parse. Parameters are read one after
# the other, so an `action=` inside another one's quoted value is not taken.
func actionParam(contentType as string) {
    def semi as int init strings.indexOf($contentType, ";");
    if ($semi < 0) {
        return "";
    }
    def rest as string init strings.trim($contentType[$semi..]);
    while ($rest != "") {
        def m as regex.Match init regex.find(PARAM_RE, $rest);
        if ($m.start < 0) {
            return "";
        }
        if (strings.lower($m.groups[0]) == "action") {
            def v as string init $m.groups[1];
            if (strings.startsWith($v, "\"")) {
                $v = regex.replace('\\(.)', $v[1..len($v) - 1], '$1');
            }
            return $v;
        }
        $rest = $rest[$m.end..];
    }
    return "";
}

/**
 * Write an envelope.
 *
 * @param version {string} `1.1` or `1.2`
 * @param headers {list of xml.Value} header blocks; [] for no `Header`
 * @param body {list of xml.Value} the `Body`'s children, usually one
 * @return {string} the document, with an XML declaration
 * @throws {SoapError} kind `"soap"` on an unknown version
 */
export func envelope(version as string, headers as list of xml.Value, body as list of xml.Value) {
    def ns as string init namespaceOf($version);
    def env as xml.Value init xml.elementNS($ns, "soap:Envelope");
    if (len($headers) > 0) {
        def h as xml.Value init xml.elementNS($ns, "soap:Header");
        for (def block in $headers) {
            $h = xml.append($h, $block);
        }
        $env = xml.append($env, $h);
    }
    def b as xml.Value init xml.elementNS($ns, "soap:Body");
    for (def part in $body) {
        $b = xml.append($b, $part);
    }
    $env = xml.append($env, $b);
    return "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n" + xml.encode($env);
}

/**
 * Decode a document into its root element, refusing a DTD.
 *
 * @param text {string} the document
 * @return {xml.Value} the root
 */
func decodeRoot(text as string) {
    # Most messages hold no "<!DOCTYPE" and skip the regex. It runs over the
    # whole text, not up to the first "<!DOCTYPE": that one may sit in a
    # comment in front of the real DOCTYPE. It is anchored and stops where the
    # prolog does.
    if (strings.contains($text, "<!DOCTYPE") and regex.matches(DOCTYPE_RE, $text)) {
        fail("a SOAP message must not carry a DOCTYPE");
    }
    def root as xml.Value;
    try {
        $root = xml.decode($text);
    } catch (e) {
        # a decoding cap stays kind "limit"
        if ($e.kind == "limit") {
            throw $e;
        }
        fail("the message is not XML: " + plain($e.message));
    }
    return $root;
}

/**
 * The Envelope of a decoded root in a known envelope namespace.
 *
 * @param root {xml.Value} the root, an `Envelope` element
 * @param version {string} its version
 * @return {Envelope} the envelope
 */
func fromRoot(root as xml.Value, version as string) {
    def ns as string init namespaceOf($version);
    def headers as list of xml.Value init [];
    def body as list of xml.Value init [];
    def bodies as int init 0;
    def seen as int init 0;
    if (strings.trim(xml.text($root)) != "") {
        fail("the Envelope holds text outside its Header and Body");
    }
    for (def child in xml.children($root)) {
        $seen = $seen + 1;
        def local as string init xml.localName($child);
        def mine as bool init xml.namespace($child) == $ns;
        if ($mine and $local == "Header") {
            # A Header after the Body would escape the mustUnderstand check.
            if ($seen != 1) {
                fail("a Header is the Envelope's first element");
            }
            $headers = headerBlocks($child);
        } elseif ($mine and $local == "Body") {
            $bodies = $bodies + 1;
            if (strings.trim(xml.text($child)) != "") {
                fail("the Body holds text outside its elements");
            }
            $body = xml.children($child);
        } elseif ($version == "1.2" or $bodies == 0) {
            # Only a first Header may precede the Body; 1.1 allows elements
            # after it.
            fail("unexpected element " + shown(xml.tag($child)) + " in the Envelope");
        }
    }
    if ($bodies != 1) {
        fail("an Envelope holds exactly one Body, got " + convert.toString($bodies));
    }
    if (len($body) > 1) {
        for (def b in $body) {
            if (xml.localName($b) == "Fault" and xml.namespace($b) == $ns) {
                fail("a Fault is the only element of its Body");
            }
        }
    }
    return Envelope{version: $version, headers: $headers, body: $body};
}

# headerBlocks is a Header's blocks, which both versions require to be
# namespace-qualified.
func headerBlocks(header as xml.Value) {
    def blocks as list of xml.Value init xml.children($header);
    for (def block in $blocks) {
        if (xml.namespace($block) == "") {
            fail("a header block is namespace-qualified, got " + shown(xml.tag($block)));
        }
    }
    return $blocks;
}

/**
 * Decode an envelope of either version.
 *
 * @param text {string} the document
 * @return {Envelope} the envelope
 * @throws {SoapError} kind `"soap"` on malformed XML, a DOCTYPE, a root that is
 *   not a 1.1 or 1.2 `Envelope`, a missing or repeated `Body`, a `Header` that
 *   is not the first element or holds a block in no namespace, text directly in
 *   the Envelope or Body, or a `Fault` with other elements beside it
 * @throws {Error} kind `"limit"` for a document past `xml`'s decoding caps
 */
export func parse(text as string) {
    def root as xml.Value init decodeRoot($text);
    def version as string init versionOf(xml.namespace($root));
    if (xml.localName($root) != "Envelope" or $version == "") {
        fail("the root is not a SOAP 1.1 or 1.2 Envelope but " + shown(xml.tag($root)) +
            " in " + shown(xml.namespace($root)));
    }
    return fromRoot($root, $version);
}

/**
 * Whether an envelope is a fault.
 *
 * @param env {Envelope} the envelope
 * @return {bool} whether its Body holds one `Fault`
 */
export func isFault(env as Envelope) {
    if (len($env.body) != 1) {
        return false;
    }
    def f as xml.Value init $env.body[0];
    return xml.localName($f) == "Fault" and xml.namespace($f) == namespaceOf($env.version);
}

/**
 * A QName in element content resolved against the element's bindings: the
 * local name for one in the envelope namespace or in none, else Clark
 * notation. An unbound prefix keeps the text as written.
 *
 * @param node {xml.Value} the element holding the QName
 * @param envNs {string} the envelope namespace
 * @return {string} e.g. `Client` or `{urn:app}Busy`
 */
func qnameOf(node as xml.Value, envNs as string) {
    def text as string init strings.trim(xml.text($node));
    def prefix as string init "";
    def local as string init $text;
    def colon as int init strings.indexOf($text, ":");
    if ($colon >= 0) {
        $prefix = $text[0..$colon];
        $local = $text[$colon + 1..];
    }
    def uri as string init "";
    try {
        $uri = xml.lookupPrefix($node, $prefix);
    } catch (e) {
        return $text;
    }
    if ($uri == "" or $uri == $envNs) {
        return $local;
    }
    return "\{" + $uri + "\}" + $local;
}

/**
 * The content of a detail element: its children's XML, else its text. Text
 * mixed in with elements cannot be placed between their XML, so for mixed
 * content it is the whole element's XML.
 *
 * @param node {xml.Value} the `detail` / `Detail` element
 * @return {string} the content
 */
func detailOf(node as xml.Value) {
    def kids as list of xml.Value init xml.children($node);
    if (len($kids) == 0) {
        return strings.trim(xml.text($node));
    }
    if (strings.trim(xml.text($node)) != "") {
        return xml.encode($node);
    }
    def parts as list of string init [];
    for (def k in $kids) {
        $parts[] = xml.encode($k);
    }
    return strings.join($parts, "");
}

/**
 * Read the fault of an envelope.
 *
 * @param env {Envelope} an envelope for which `isFault` holds
 * @return {Fault} the fault
 * @throws {SoapError} kind `"soap"` when the envelope is no fault, or a 1.2 fault
 *   lacks its `Code`
 */
export func parseFault(env as Envelope) {
    if (not isFault($env)) {
        fail("the envelope is not a fault");
    }
    def f as xml.Value init $env.body[0];
    def out as Fault init Fault{code: "", subcode: "", reason: "", actor: "", detail: ""};
    if ($env.version == "1.1") {
        # The 1.1 fault children are unqualified; `*:` also reads the
        # qualified ones some servers send.
        if (xml.has($f, "*:faultcode")) {
            $out.code = qnameOf(xml.get($f, "*:faultcode"), SOAP11_NS);
        }
        if (xml.has($f, "*:faultstring")) {
            $out.reason = strings.trim(xml.text(xml.get($f, "*:faultstring")));
        }
        if (xml.has($f, "*:faultactor")) {
            $out.actor = strings.trim(xml.text(xml.get($f, "*:faultactor")));
        }
        if (xml.has($f, "*:detail")) {
            $out.detail = detailOf(xml.get($f, "*:detail"));
        }
        return $out;
    }
    def ns as map of string to string init {"e": SOAP12_NS};
    if (not xml.has($f, "e:Code/e:Value", $ns)) {
        fail("a SOAP 1.2 Fault has a Code/Value");
    }
    $out.code = qnameOf(xml.get($f, "e:Code/e:Value", $ns), SOAP12_NS);
    if (xml.has($f, "e:Code/e:Subcode/e:Value", $ns)) {
        $out.subcode = qnameOf(xml.get($f, "e:Code/e:Subcode/e:Value", $ns), SOAP12_NS);
    }
    if (xml.has($f, "e:Reason/e:Text", $ns)) {
        $out.reason = strings.trim(xml.text(xml.get($f, "e:Reason/e:Text", $ns)));
    }
    if (xml.has($f, "e:Role", $ns)) {
        $out.actor = strings.trim(xml.text(xml.get($f, "e:Role", $ns)));
    }
    if (xml.has($f, "e:Detail", $ns)) {
        $out.detail = detailOf(xml.get($f, "e:Detail", $ns));
    }
    return $out;
}

/**
 * The generic code under its name in a version: `Client` <-> `Sender`,
 * `Server` <-> `Receiver`; other codes unchanged.
 *
 * @param code {string} a code without namespace
 * @param version {string} the target version
 * @return {string} the code
 */
func mapCode(code as string, version as string) {
    if ($version == "1.1") {
        if ($code == "Sender") {
            return "Client";
        }
        if ($code == "Receiver") {
            return "Server";
        }
    } elseif ($code == "Client") {
        return "Sender";
    } elseif ($code == "Server") {
        return "Receiver";
    }
    return $code;
}

/**
 * Write a fault code into `node` as a QName.
 *
 * - `{ns}local` gets a prefix declared on `node`.
 * - A 1.1 `faultcode` whose first dotted part is a standard code
 *   (`Client.Auth`) goes in the envelope namespace, with `Sender` / `Receiver`
 *   mapped to `Client` / `Server`; another name stays unqualified.
 * - A 1.2 `Code/Value` must be a standard code (`Client` / `Server` are
 *   mapped); an application code belongs in the subcode.
 * - A subcode name stays unqualified.
 *
 * @param node {xml.Value} the element that will hold the code
 * @param code {string} the code, as `Fault.code`
 * @param version {string} the target version
 * @param top {bool} the fault code itself rather than a 1.2 subcode
 * @return {xml.Value} the element with the code set
 */
func withCode(node as xml.Value, code as string, version as string, top as bool) {
    def standard as bool init $top and $version == "1.2";
    if (strings.startsWith($code, "\{")) {
        def close as int init strings.indexOf($code, "\}");
        if ($close < 2 or not regex.matches(NCNAME_RE, $code[$close + 1..])) {
            fail("a fault code is a name or \{namespace\}local, got " + shown($code));
        }
        if ($standard) {
            fail("a SOAP 1.2 fault code is one of " + strings.join(CODES12, ", ") +
                "; put " + shown($code) + " in the subcode");
        }
        def uri as string init $code[1..$close];
        checkNamespace($uri, "a fault code's namespace");
        return xml.setText(xml.setAttr($node, "xmlns:app", $uri), "app:" + $code[$close + 1..]);
    }
    if (not regex.matches(NCNAME_RE, $code)) {
        fail("a fault code is a name or \{namespace\}local, got " + shown($code));
    }
    if (not $top) {
        return xml.setText($node, $code);
    }
    if ($standard) {
        def c as string init mapCode($code, $version);
        if (not lists.contains(CODES12, $c)) {
            fail("a SOAP 1.2 fault code is one of " + strings.join(CODES12, ", ") +
                "; put " + shown($code) + " in the subcode");
        }
        return xml.setText($node, "soap:" + $c);
    }
    def parts as list of string init strings.split($code, ".");
    $parts[0] = mapCode($parts[0], $version);
    if (not lists.contains(CODES11, $parts[0])) {
        return xml.setText($node, $code);
    }
    return xml.setText($node, "soap:" + strings.join($parts, "."));
}

/**
 * A detail element filled from `Fault.detail`: XML content is decoded as the
 * element's content, text mixed in included; anything else becomes text.
 *
 * @param ns {string} the element's namespace; "" for the 1.1 `detail`
 * @param detail {string} the content
 * @return {xml.Value} the element
 */
func detailElement(ns as string, detail as string) {
    if (not strings.startsWith(strings.trim($detail), "<")) {
        if ($ns == "") {
            return xml.setText(xml.element("detail"), xmlSafe($detail));
        }
        return xml.setText(xml.elementNS($ns, "soap:Detail"), xmlSafe($detail));
    }
    def out as xml.Value;
    try {
        # The content stands on its own: decoded in a bare wrapper first, so a
        # `soap:` prefix it does not declare is not taken from the real one.
        xml.decode("<d>" + $detail + "</d>");
        if ($ns == "") {
            $out = xml.decode("<detail>" + $detail + "</detail>");
        } else {
            $out = xml.decode("<soap:Detail xmlns:soap=\"" + $ns + "\">" + $detail +
                "</soap:Detail>");
        }
    } catch (e) {
        if ($e.kind == "limit") {
            throw $e;
        }
        fail("a fault detail that starts with < is XML: " + plain($e.message));
    }
    return $out;
}

/**
 * The `Fault` element of a version.
 *
 * @param version {string} `1.1` or `1.2`
 * @param f {Fault} the fault; `code` is required, `reason` should be set.
 *   Characters XML 1.0 does not allow are written as `?`
 * @return {xml.Value} the element, for an envelope's Body
 * @throws {SoapError} kind `"soap"` on an unknown version, a malformed code
 *   (one in a reserved namespace or a namespace with a control character
 *   included), a 1.2 code that is not a standard one, or a detail starting
 *   with `<` that is not XML (kind `"limit"` for one past `xml`'s decoding caps)
 */
export func faultElement(version as string, f as Fault) {
    def ns as string init namespaceOf($version);
    def out as xml.Value init xml.elementNS($ns, "soap:Fault");
    if ($version == "1.1") {
        $out = xml.append($out, withCode(xml.element("faultcode"), $f.code, $version, true));
        $out = xml.append($out, xml.setText(xml.element("faultstring"), xmlSafe($f.reason)));
        if ($f.actor != "") {
            $out = xml.append($out, xml.setText(xml.element("faultactor"), xmlSafe($f.actor)));
        }
        if ($f.detail != "") {
            $out = xml.append($out, detailElement("", $f.detail));
        }
        return $out;
    }
    def code as xml.Value init xml.elementNS($ns, "soap:Code");
    $code = xml.append($code, withCode(xml.elementNS($ns, "soap:Value"), $f.code, $version, true));
    if ($f.subcode != "") {
        def sub as xml.Value init xml.elementNS($ns, "soap:Subcode");
        $sub = xml.append(
            $sub,
            withCode(xml.elementNS($ns, "soap:Value"), $f.subcode, $version, false));
        $code = xml.append($code, $sub);
    }
    $out = xml.append($out, $code);
    def text as xml.Value init xml.setAttr(xml.elementNS($ns, "soap:Text"), "xml:lang", "en");
    def reason as xml.Value init xml.append(
        xml.elementNS($ns, "soap:Reason"),
        xml.setText($text, xmlSafe($f.reason)));
    $out = xml.append($out, $reason);
    if ($f.actor != "") {
        $out = xml.append($out, xml.setText(xml.elementNS($ns, "soap:Role"), xmlSafe($f.actor)));
    }
    if ($f.detail != "") {
        $out = xml.append($out, detailElement($ns, $f.detail));
    }
    return $out;
}

/**
 * Write a fault envelope.
 *
 * @param version {string} `1.1` or `1.2`
 * @param f {Fault} the fault
 * @return {string} the document
 * @throws {SoapError} kind `"soap"` as `faultElement`
 */
export func faultEnvelope(version as string, f as Fault) {
    return envelope($version, [], [faultElement($version, $f)]);
}

/**
 * An operation element with one text child per parameter, the shape of a
 * document/literal wrapped or RPC request.
 *
 * @param ns {string} the operation's namespace, e.g. `http://tempuri.org/`
 * @param name {string} the operation, e.g. `Add`
 * @param params {map of string to string} child name to text, in order
 * @param qualified {bool} put the children in `ns` too (the schema's
 *   `elementFormDefault="qualified"`, as .NET services expect); false leaves
 *   them unqualified (the JAX-WS default)
 * @return {xml.Value} the element, e.g. `<m:Add xmlns:m="..."><m:intA>2</m:intA>...`
 * @throws {SoapError} kind `"soap"` on an empty or reserved (`xml`, `xmlns`)
 *   namespace or one with a control character, a name that is not an XML name,
 *   or a value with a character XML 1.0 does not allow
 */
export func operation(
    ns as string,
    name as string,
    params as map of string to string,
    qualified as bool) {
    checkNamespace($ns, "an operation's namespace");
    if (not regex.matches(NCNAME_RE, $name)) {
        fail("an operation name is an XML name, got " + shown($name));
    }
    def op as xml.Value init xml.elementNS($ns, "m:" + $name);
    for (def key in $params) {
        if (not regex.matches(NCNAME_RE, $key)) {
            fail("a parameter name is an XML name, got " + shown($key));
        }
        if (regex.matches(XML_INVALID_RE, $params[$key])) {
            fail("parameter " + $key + " holds a character XML 1.0 does not allow");
        }
        def child as xml.Value init xml.element($key);
        if ($qualified) {
            $child = xml.elementNS($ns, "m:" + $key);
        }
        $op = xml.append($op, xml.setText($child, $params[$key]));
    }
    return $op;
}

/**
 * The children of an element by local name: each one's text, or its XML when
 * it holds elements. Of repeated names the first wins; read a repeated or
 * nested result with the `xml` accessors.
 *
 * @param node {xml.Value} e.g. the result element `call` returns
 * @return {map of string to string} local name to content
 */
export func values(node as xml.Value) {
    def out as map of string to string init {};
    for (def k in xml.children($node)) {
        def local as string init xml.localName($k);
        if (not maps.has($out, $local)) {
            if (xml.has($k, "*")) {
                $out[$local] = detailOf($k);
            } else {
                $out[$local] = xml.text($k);
            }
        }
    }
    return $out;
}

# --- client ----------------------------------------------------------------

/**
 * A client for a SOAP endpoint.
 *
 * @param endpoint {string} the `http://` or `https://` service URL
 * @return {Client} SOAP 1.1, 30 s idle timeout, 16 MiB response cap, full
 *   certificate checking
 * @throws {SoapError} kind `"soap"` on a URL that is not http(s), or carries
 *   credentials (use `withBasicAuth`), a fragment or a control character
 */
export func connect(endpoint as string) {
    def u as string init strings.trim($endpoint);
    def shape as bool init regex.matches('^https?://[^/\s?#@]+(/[^\s#]*)?$', $u);
    if (not $shape or regex.matches(CONTROL_RE, $u)) {
        fail("an endpoint is http(s)://host[/path] without credentials or fragment, got " +
            shown($endpoint));
    }
    return Client{
        endpoint: $u,
        version: "1.1",
        headers: {},
        soapHeaders: [],
        tls: http.TlsOptions{caCert: encoding.encode("utf-8", ""), skipVerify: false},
        timeoutMs: DEFAULT_TIMEOUT_MS,
        maxBytes: DEFAULT_MAX_BYTES,
        digest: DigestCredentials{user: "", password: ""},
        allowInsecureHttp: false
    };
}

/**
 * Speak SOAP 1.1 or 1.2.
 *
 * @param c {Client} the client
 * @param version {string} `1.1` or `1.2`
 * @return {Client} the changed copy
 * @throws {SoapError} kind `"soap"` on another version
 */
export func withVersion(c as Client, version as string) {
    namespaceOf($version);
    $c.version = $version;
    return $c;
}

/**
 * Send an HTTP header with every call, replacing one of the same name in any
 * letter case.
 *
 * @param c {Client} the client
 * @param name {string} the header name
 * @param value {string} its value
 * @return {Client} the changed copy
 * @throws {SoapError} kind `"soap"` on a bad name, a value with a control
 *   character, or a header the client sets itself (`Content-Type`,
 *   `SOAPAction`, `Host`, `Content-Length`, ...)
 */
export func withHeader(c as Client, name as string, value as string) {
    if (not regex.matches("^[A-Za-z0-9-]+$", $name) or regex.matches(CONTROL_RE, $value)) {
        fail("a header is a token name and a one-line value, got " + shown($name));
    }
    if (lists.contains(OWN_HEADERS, strings.lower($name))) {
        fail("the client sets " + $name + " itself");
    }
    $c.headers = withoutHeader($c.headers, $name);
    $c.headers[$name] = $value;
    return $c;
}

# withoutHeader is `headers` without `name` in any letter case, so a header set
# again replaces the one before rather than going out beside it.
func withoutHeader(headers as map of string to string, name as string) {
    def lower as string init strings.lower($name);
    def kept as map of string to string init {};
    for (def h in $headers) {
        if (strings.lower($h) != $lower) {
            $kept[$h] = $headers[$h];
        }
    }
    return $kept;
}

/**
 * Authenticate with a user name and password (HTTP Basic). Over `http://`
 * they would travel readable, so a call to an `http://` endpoint on a
 * non-loopback host throws unless the client's `allowInsecureHttp` is set.
 *
 * @param c {Client} the client
 * @param user {string} the user name, without `:`
 * @param password {string} the password
 * @return {Client} the changed copy
 * @throws {SoapError} kind `"soap"` on a `:` in the name or a control character
 */
export func withBasicAuth(c as Client, user as string, password as string) {
    if (strings.contains($user, ":") or regex.matches(CONTROL_RE, $user + $password)) {
        fail("a Basic user name holds no :, and neither holds control characters");
    }
    $c.headers = withoutHeader($c.headers, "Authorization");
    $c.headers["Authorization"] = http.basic($user, $password);
    $c.digest = DigestCredentials{user: "", password: ""};
    return $c;
}

/**
 * Authenticate with a bearer token.
 *
 * @param c {Client} the client
 * @param token {string} the token
 * @return {Client} the changed copy
 * @throws {SoapError} kind `"soap"` on an empty token or one with a space or
 *   control character
 */
export func withToken(c as Client, token as string) {
    if ($token == "" or regex.matches('[\s\x00-\x1f\x7f]', $token)) {
        fail("a token is not empty and holds no spaces or control characters");
    }
    $c.headers = withoutHeader($c.headers, "Authorization");
    $c.headers["Authorization"] = "Bearer " + $token;
    $c.digest = DigestCredentials{user: "", password: ""};
    return $c;
}

/**
 * Authenticate with HTTP Digest (RFC 7616): each call goes out without
 * credentials and answers the endpoint's `401` challenge (`http.sendDigest`),
 * so it costs one extra round trip. Replaces Basic or bearer auth set before;
 * `withBasicAuth` / `withToken` replace it in turn. A listener can crack the
 * response offline, so over `http://` it is refused as Basic is (see
 * `withBasicAuth`).
 *
 * @param c {Client} the client
 * @param user {string} the user name
 * @param password {string} the password
 * @return {Client} the changed copy
 * @throws {SoapError} kind `"soap"` on an empty user name or a control character
 *   in it
 */
export func withDigestAuth(c as Client, user as string, password as string) {
    if ($user == "" or regex.matches(CONTROL_RE, $user)) {
        fail("a Digest user name is not empty and holds no control characters");
    }
    $c.headers = withoutHeader($c.headers, "Authorization");
    $c.digest = DigestCredentials{user: $user, password: $password};
    return $c;
}

/**
 * Send a header block with every call, such as a WS-Security or session
 * header. Add a `mustUnderstand` attribute yourself where the service needs
 * one.
 *
 * @param c {Client} the client
 * @param block {xml.Value} the header block, a namespaced element
 * @return {Client} the changed copy
 * @throws {SoapError} kind `"soap"` on a text node or an element without
 *   namespace, which SOAP does not allow as a header block
 */
export func withSoapHeader(c as Client, block as xml.Value) {
    if (xml.typeOf($block) != "element" or xml.namespace($block) == "") {
        fail("a header block is an element in a namespace");
    }
    $c.soapHeaders[] = $block;
    return $c;
}

/**
 * Trust a certificate (PEM) instead of the system roots.
 *
 * @param c {Client} the client
 * @param pem {bytes} the certificate
 * @return {Client} the changed copy
 */
export func withCA(c as Client, pem as bytes) {
    $c.tls.caCert = $pem;
    $c.tls.skipVerify = false;
    return $c;
}

/**
 * Accept any certificate. The connection stays encrypted but is no longer
 * authenticated; prefer `withCA`.
 *
 * @param c {Client} the client
 * @return {Client} the changed copy
 */
export func withSkipVerify(c as Client) {
    $c.tls.skipVerify = true;
    return $c;
}

/**
 * Set the idle timeout of each call.
 *
 * @param c {Client} the client
 * @param ms {int} milliseconds; 0 or less for the default, 30 s
 * @return {Client} the changed copy
 */
export func withTimeout(c as Client, ms as int) {
    $c.timeoutMs = $ms;
    if ($ms <= 0) {
        $c.timeoutMs = DEFAULT_TIMEOUT_MS;
    }
    return $c;
}

/**
 * Set the response-body cap. A decoded reply takes up to about 15 times its
 * size in memory.
 *
 * @param c {Client} the client
 * @param n {int} bytes; 0 or less for the default, 16 MiB
 * @return {Client} the changed copy
 */
export func withMaxBytes(c as Client, n as int) {
    $c.maxBytes = $n;
    if ($n <= 0) {
        $c.maxBytes = DEFAULT_MAX_BYTES;
    }
    return $c;
}

/**
 * Post an envelope and decode the reply, a fault included.
 *
 * @param c {Client} the client
 * @param action {string} the SOAPAction URI from the WSDL; "" for none
 * @param body {xml.Value} the operation element
 * @return {Reply} the reply; `isFault` and `fault` report a fault
 * @throws {Error} kind `"limit"` for a reply over `maxBytes` or past `xml`'s
 *   decoding caps, `"timeout"` when the server does not answer in time,
 *   `"closed"` when it closes the connection under the call (`soap: POST
 *   <endpoint>: ...`)
 * @throws {net.NetError} kind `"net"` on a socket failure (refused, reset, DNS,
 *   TLS), as `http` throws it
 * @throws {SoapError} kind `"soap"` on another transport error, a reply that is not an
 *   envelope (with the HTTP status when it is not 2xx), a non-fault reply in
 *   the other SOAP version, a non-2xx status without a fault, or (before
 *   connecting, status 0) credentials (an `Authorization` header or Digest)
 *   for an `http://` endpoint on a non-loopback host without
 *   `allowInsecureHttp`
 */
export func send(c as Client, action as string, body as xml.Value) {
    # Refuse to send credentials in the clear, before any connection. Digest
    # sends no reusable secret, but a listener can still crack its hash
    # offline, so it counts too.
    if (not $c.allowInsecureHttp and (hasCredentials($c.headers) or $c.digest.user != "") and
        http.isCleartextRemote($c.endpoint)) {
        fail("credentials need an https:// endpoint (http:// only with allowInsecureHttp)");
    }
    def headers as map of string to string init $c.headers;
    $headers["Content-Type"] = contentType($c.version, $action);
    if ($c.version == "1.1") {
        $headers["SOAPAction"] = "\"" + $action + "\"";
    }
    def o as http.Options;
    $o.tls = $c.tls;
    $o.timeoutMs = $c.timeoutMs;
    $o.maxBytes = $c.maxBytes;
    def request as string init envelope($c.version, $c.soapHeaders, [$body]);
    def r as http.Response;
    try {
        if ($c.digest.user == "") {
            $r = http.send("POST", $c.endpoint, $headers, $request, $o);
        } else {
            $r = http.sendDigest(
                "POST",
                $c.endpoint,
                $headers,
                $request,
                $o,
                $c.digest.user,
                $c.digest.password);
        }
    } catch (e) {
        # Not the query: it may carry a key.
        def why as string init "POST " + plain(strings.split($c.endpoint, "?")[0]) + ": " +
            plain($e.message);
        # generic kinds stay generic: a reached cap "limit", a deadline
        # "timeout", a connection closed under the call "closed", with the
        # request in front; a cancelled task and a socket failure (http's
        # net.NetError) go on as they are
        if ($e.kind == "cancelled" or $e.kind == "net") {
            throw $e;
        }
        if ($e.kind == "limit" or $e.kind == "timeout" or $e.kind == "closed") {
            throw Error{kind: $e.kind, message: "soap: " + $why};
        }
        fail($why);
    }
    def ok as bool init $r.status >= 200 and $r.status < 300;
    if (strings.trim($r.body) == "") {
        if (not $ok) {
            failStatus(
                "HTTP " + convert.toString($r.status) + " " + plain($r.statusText),
                $r.status);
        }
        return Reply{
            status: $r.status,
            envelope: Envelope{version: $c.version, headers: [], body: []},
            isFault: false,
            fault: Fault{code: "", subcode: "", reason: "", actor: "", detail: ""}
        };
    }
    def env as Envelope;
    try {
        $env = parse($r.body);
    } catch (e) {
        # a generic kind (a decoding cap is "limit") goes on as it is, at any
        # status
        if ($e.kind != ERROR_KIND) {
            throw $e;
        }
        if (not $ok) {
            failStatus(
                "HTTP " + convert.toString($r.status) + " " + plain($r.statusText) +
                    " without a SOAP reply",
                $r.status);
        }
        # a 2xx that is not an envelope keeps its status
        def why as string init $e.message;
        if (strings.startsWith($why, "soap: ")) {
            $why = $why[6..];
        }
        failStatus($why, $r.status);
    }
    if (isFault($env)) {
        return Reply{status: $r.status, envelope: $env, isFault: true, fault: parseFault($env)};
    }
    if ($env.version != $c.version) {
        failStatus(
            "a SOAP " + $c.version + " call got a SOAP " + $env.version + " reply",
            $r.status);
    }
    if (not $ok) {
        failStatus(
            "HTTP " + convert.toString($r.status) + " " + plain($r.statusText) + " without a fault",
            $r.status);
    }
    return Reply{
        status: $r.status,
        envelope: $env,
        isFault: false,
        fault: Fault{code: "", subcode: "", reason: "", actor: "", detail: ""}
    };
}

# hasCredentials reports whether the headers carry an Authorization or
# Proxy-Authorization header, in any case.
func hasCredentials(headers as map of string to string) {
    for (def name in $headers) {
        def lower as string init strings.lower($name);
        if ($lower == "authorization" or $lower == "proxy-authorization") {
            return true;
        }
    }
    return false;
}

/**
 * Call an operation and return its result element.
 *
 * @param c {Client} the client
 * @param action {string} the SOAPAction URI from the WSDL; "" for none
 * @param body {xml.Value} the operation element (see `operation`)
 * @return {xml.Value} the reply's first Body child, e.g. `AddResponse`
 * @throws {SoapError} kind `"soap"` on a fault (`soap: fault CODE: REASON`,
 *   with `isFault` and the parsed `fault`), an empty reply Body, or as `send`
 */
export func call(c as Client, action as string, body as xml.Value) {
    def r as Reply init send($c, $action, $body);
    if ($r.isFault) {
        throw SoapError{
            kind: ERROR_KIND,
            message: "soap: fault " + plain($r.fault.code) + ": " + plain($r.fault.reason),
            status: $r.status,
            isFault: true,
            fault: $r.fault
        };
    }
    if (len($r.envelope.body) == 0) {
        failStatus("the reply Body is empty", $r.status);
    }
    return $r.envelope.body[0];
}

# --- server ----------------------------------------------------------------

/**
 * Throw from a `handle` handler to answer with a Client (1.1) / Sender (1.2)
 * fault: the request was wrong, and `reason` says how. Any other error a
 * handler throws becomes a Server / Receiver fault with a generic reason, so
 * internal messages do not reach the caller.
 *
 * @param reason {string} the fault reason
 */
export func senderFault(reason as string) {
    throw Error{kind: FAULT_KIND, message: $reason};
}

/**
 * A fault as the server's answer.
 *
 * @param version {string} the request's version
 * @param code {string} the fault code
 * @param reason {string} the reason
 * @return {Response} the response
 */
func faultResponse(version as string, code as string, reason as string) {
    def status as int init 500;
    if ($version == "1.2" and ($code == "Sender" or $code == "Client")) {
        $status = 400;
    }
    def f as Fault init Fault{code: $code, subcode: "", reason: $reason, actor: "", detail: ""};
    return Response{
        status: $status,
        contentType: contentType($version, ""),
        body: faultEnvelope($version, $f)
    };
}

/**
 * Whether a header block must be understood here: `mustUnderstand` is `1`
 * (or, in 1.2, `true`), and the block targets this node, the ultimate receiver. That is a
 * block without `actor` (1.1) / `role` (1.2), or with the `next` actor, or
 * the `next` or `ultimateReceiver` role. A block for another node is left
 * alone.
 *
 * @param block {xml.Value} the header block
 * @param ns {string} the envelope namespace
 * @return {bool} whether it must be understood
 */
func mustUnderstand(block as xml.Value, ns as string) {
    if (not xml.hasAttrNS($block, $ns, "mustUnderstand")) {
        return false;
    }
    def flag as string init strings.trim(xml.attrNS($block, $ns, "mustUnderstand"));
    if ($flag == "0" or ($ns == SOAP12_NS and $flag == "false")) {
        return false;
    }
    if ($flag != "1" and ($ns == SOAP11_NS or $flag != "true")) {
        # anything else would turn a required block into an optional one
        def allowed as string init "1 or 0";
        if ($ns == SOAP12_NS) {
            $allowed = "true, false, 1 or 0";
        }
        fail("mustUnderstand is " + $allowed + ", got " + shown($flag));
    }
    if ($ns == SOAP11_NS) {
        return not xml.hasAttrNS($block, $ns, "actor") or
            strings.trim(xml.attrNS($block, $ns, "actor")) == ACTOR_NEXT;
    }
    if (not xml.hasAttrNS($block, $ns, "role")) {
        return true;
    }
    def role as string init strings.trim(xml.attrNS($block, $ns, "role"));
    return $role == ROLE_NEXT or $role == ROLE_ULTIMATE;
}

/**
 * Answer a request body with `handler`, as `handleWith` with no header blocks
 * understood.
 *
 * @param body {string} the HTTP request body
 * @param action {string} the SOAPAction (see `requestAction`)
 * @param handler {func} `func(req as Request)` returning the result element
 * @return {Response} the response to send
 */
export func handle(body as string, action as string, handler as func) {
    return handleWith($body, $action, $handler, []);
}

/**
 * Answer a request body with `handler`, transport-agnostic, as the ultimate
 * receiver. Malformed XML or a Body without exactly one element is a Client /
 * Sender fault; an envelope in another namespace is a VersionMismatch fault;
 * a header block for this node (see `mustUnderstand`) with `mustUnderstand`
 * set whose Clark name is not in `understood` is a MustUnderstand fault. The
 * handler's result is sent in the request's version; for its errors see
 * `senderFault`. Cap the request body in the transport.
 *
 * @param body {string} the HTTP request body
 * @param action {string} the SOAPAction (see `requestAction`)
 * @param handler {func} `func(req as Request)` returning the result element
 *   (`xml.Value`)
 * @param understood {list of string} the header blocks the handler processes,
 *   by Clark name (`{ns}local`)
 * @return {Response} the response to send; status 200, or 400 / 500 for a
 *   fault
 */
export func handleWith(
    body as string,
    action as string,
    handler as func,
    understood as list of string) {
    def root as xml.Value;
    try {
        $root = decodeRoot($body);
    } catch (e) {
        return faultResponse("1.1", "Client", $e.message);
    }
    def version as string init versionOf(xml.namespace($root));
    if (xml.localName($root) != "Envelope") {
        return faultResponse("1.1", "Client", "soap: the root is not an Envelope");
    }
    if ($version == "") {
        return faultResponse("1.1", "VersionMismatch", "soap: unsupported envelope namespace");
    }
    def env as Envelope;
    try {
        $env = fromRoot($root, $version);
    } catch (e) {
        return faultResponse($version, "Client", $e.message);
    }
    def ns as string init namespaceOf($version);
    for (def block in $env.headers) {
        def name as string init "\{" + xml.namespace($block) + "\}" + xml.localName($block);
        def must as bool init false;
        try {
            $must = mustUnderstand($block, $ns);
        } catch (e) {
            return faultResponse($version, "Client", $e.message);
        }
        if ($must and not lists.contains($understood, $name)) {
            def why as string init "soap: header " + plain($name) + " not understood";
            return faultResponse($version, "MustUnderstand", $why);
        }
    }
    if (len($env.body) != 1) {
        return faultResponse($version, "Client", "soap: the Body holds exactly one element");
    }
    def req as Request init Request{
        version: $version,
        action: $action,
        headers: $env.headers,
        body: $env.body[0]
    };
    def result as xml.Value;
    try {
        $result = $handler($req);
    } catch (e) {
        # A handler may throw any value; only senderFault's Error carries a
        # reason for the caller. Reading `kind` of a string would throw.
        def err as Error;
        try {
            $err = $e;
        } catch (other) {
            return faultResponse($version, "Server", "internal error");
        }
        if ($err.kind == FAULT_KIND) {
            return faultResponse($version, "Client", $err.message);
        }
        return faultResponse($version, "Server", "internal error");
    }
    return Response{
        status: 200,
        contentType: contentType($version, ""),
        body: envelope($version, [], [$result])
    };
}

# --- WSDL -----------------------------------------------------------------

/** The WSDL 1.1 namespace. */
export def const WSDL_NS as string init "http://schemas.xmlsoap.org/wsdl/";

# The SOAP 1.1 and 1.2 binding extensions of WSDL 1.1, WSDL 2.0 (refused) and
# XML Schema (read for `elementFormDefault`).
def const WSDL_SOAP11_NS as string init "http://schemas.xmlsoap.org/wsdl/soap/";
def const WSDL_SOAP12_NS as string init "http://schemas.xmlsoap.org/wsdl/soap12/";
def const WSDL20_NS as string init "http://www.w3.org/ns/wsdl";
def const XSD_NS as string init "http://www.w3.org/2001/XMLSchema";

# The `transport` URIs of SOAP over HTTP: WSDL 1.1's, and the SOAP 1.2 HTTP
# binding's that some SOAP 1.2 bindings name.
def const HTTP_TRANSPORTS as list of string init [
    "http://schemas.xmlsoap.org/soap/http",
    "http://schemas.xmlsoap.org/soap/http/",
    "http://www.w3.org/2003/05/soap/bindings/HTTP/"
];

/** How many documents one `parseWsdlWith` reads: the WSDL and its imports. */
export def const MAX_WSDL_DOCUMENTS as int init 32;

/**
 * One part of a WSDL message: the schema element (document style) or type
 * (rpc style) it carries.
 *
 * @field name {string} the part's name
 * @field elementNamespace {string} the namespace of `element`; "" with a type
 * @field element {string} the element's local name; "" when the part names a type
 * @field typeNamespace {string} the namespace of `typeName`; "" with an element
 * @field typeName {string} the type's local name; "" when the part names an element
 * @field qualified {bool} an inline schema for `elementNamespace` sets
 *   `elementFormDefault="qualified"` (the `qualified` argument of `operation`)
 */
export def struct WsdlPart {
    name as string,
    elementNamespace as string,
    element as string,
    typeNamespace as string,
    typeName as string,
    qualified as bool
};

/**
 * An operation of a SOAP binding, with the parts of its messages.
 *
 * @field name {string} the operation's name
 * @field action {string} the SOAPAction; "" when the binding sets none
 * @field style {string} `document` or `rpc`
 * @field bodyUse {string} `literal` or `encoded` (the input's `soap:body use`)
 * @field namespace {string} the rpc wrapper's namespace (`soap:body
 *   namespace`); "" for document style
 * @field input {list of WsdlPart} the parts of the request Body: the input
 *   message's parts, or those its `soap:body parts` names
 * @field output {list of WsdlPart} the parts of the response Body, as `input`;
 *   [] when one-way
 * @field inputHeaders {list of WsdlPart} the request's header blocks
 *   (`soap:header`), in order
 * @field outputHeaders {list of WsdlPart} the response's header blocks
 */
export def struct WsdlOperation {
    name as string,
    action as string,
    style as string,
    bodyUse as string,
    namespace as string,
    input as list of WsdlPart,
    output as list of WsdlPart,
    inputHeaders as list of WsdlPart,
    outputHeaders as list of WsdlPart
};

/**
 * A SOAP binding: the operations of one port type over SOAP 1.1 or 1.2.
 *
 * @field name {string} the binding's name
 * @field namespace {string} the target namespace it is defined in
 * @field version {string} `1.1` or `1.2`
 * @field style {string} the binding's default style, `document` or `rpc`
 * @field operations {list of WsdlOperation} in document order
 */
export def struct WsdlBinding {
    name as string,
    namespace as string,
    version as string,
    style as string,
    operations as list of WsdlOperation
};

/**
 * A port of a service: an endpoint address over a SOAP binding.
 *
 * @field service {string} the service it belongs to
 * @field name {string} the port's name
 * @field address {string} the endpoint URL (`soap:address location`)
 * @field version {string} `1.1` or `1.2`
 * @field binding {string} the binding's name
 * @field bindingNamespace {string} the binding's namespace
 */
export def struct WsdlPort {
    service as string,
    name as string,
    address as string,
    version as string,
    binding as string,
    bindingNamespace as string
};

/**
 * A WSDL 1.1 description with its imports: the SOAP ports and bindings.
 * Bindings and ports of other kinds (HTTP GET/POST, MIME) are left out.
 *
 * @field name {string} the definitions' `name`; "" when unset
 * @field targetNamespace {string} the definitions' target namespace
 * @field ports {list of WsdlPort} every SOAP port, in document order
 * @field bindings {list of WsdlBinding} every SOAP binding
 */
export def struct Wsdl {
    name as string,
    targetNamespace as string,
    ports as list of WsdlPort,
    bindings as list of WsdlBinding
};

/**
 * One operation at one endpoint: what `wsdlConnect`, `wsdlRequest` and `call`
 * need.
 *
 * @field address {string} the endpoint URL
 * @field version {string} `1.1` or `1.2`
 * @field operation {WsdlOperation} the operation
 */
export def struct WsdlCall {
    address as string,
    version as string,
    operation as WsdlOperation
};

# PortOp is one port type operation: the keys of its input and output messages.
def struct PortOp {
    input as string,
    output as string
};

# RawIO is the SOAP side of a binding operation's input or output: the
# `soap:body` (`restricted` when it names its `parts`) and each `soap:header`
# as a message key and a part name.
def struct RawIO {
    bodyUse as string,
    namespace as string,
    restricted as bool,
    parts as list of string,
    headerMessages as list of string,
    headerParts as list of string
};

# RawOp is a binding operation before its messages are looked up.
def struct RawOp {
    name as string,
    action as string,
    style as string,
    input as RawIO,
    output as RawIO
};

# RawBinding is a binding before its port type is looked up; version "" marks
# one that is not SOAP, "other" SOAP over a transport other than HTTP.
def struct RawBinding {
    name as string,
    namespace as string,
    portType as string,
    version as string,
    style as string,
    ops as list of RawOp
};

# WsdlDoc is what one document defines; messages, port types and operations
# are keyed by `{namespace}name`.
def struct WsdlDoc {
    name as string,
    targetNamespace as string,
    imports as list of string,
    messages as map of string to list of WsdlPart,
    portTypes as map of string to bool,
    portOps as map of string to PortOp,
    bindings as list of RawBinding,
    ports as list of WsdlPort,
    qualified as map of string to bool
};

# WsdlParts is the merged messages and schema settings, read while the
# bindings are resolved (a struct, so it is passed without a copy).
def struct WsdlParts {
    messages as map of string to list of WsdlPart,
    qualified as map of string to bool
};

# clark is the `{namespace}local` key of a name.
func clark(ns as string, local as string) {
    return '{' + $ns + '}' + $local;
}

# wsdlRoot decodes a WSDL document into its root element, refusing a DTD.
func wsdlRoot(text as string) {
    if (strings.contains($text, "<!DOCTYPE") and regex.matches(DOCTYPE_RE, $text)) {
        fail("a WSDL document must not carry a DOCTYPE");
    }
    def root as xml.Value;
    try {
        $root = xml.decode($text);
    } catch (e) {
        if ($e.kind == "limit") {
            throw $e;
        }
        fail("the WSDL is not XML: " + plain($e.message));
    }
    return $root;
}

# optAttr is an attribute's value, "" when it is absent.
func optAttr(node as xml.Value, name as string) {
    if (xml.hasAttr($node, $name)) {
        return xml.attr($node, $name);
    }
    return "";
}

# wsdlAttr is a required attribute's value.
func wsdlAttr(node as xml.Value, name as string) {
    if (not xml.hasAttr($node, $name)) {
        fail("WSDL " + xml.localName($node) + " has no " + $name + " attribute");
    }
    return xml.attr($node, $name);
}

# wsdlName is an element's `name`, which must be an XML name.
func wsdlName(node as xml.Value) {
    def name as string init wsdlAttr($node, "name");
    if (not regex.matches(NCNAME_RE, $name)) {
        fail("WSDL " + xml.localName($node) + " name is not an XML name: " + shown($name));
    }
    return $name;
}

# wsdlQName resolves a QName attribute value against the namespaces in scope
# at `node` (an unprefixed name takes the default namespace): [namespace, local].
func wsdlQName(node as xml.Value, value as string, what as string) {
    def parts as list of string init strings.split($value, ":");
    if (len($parts) > 2) {
        fail($what + " is not a QName: " + shown($value));
    }
    def prefix as string init "";
    def local as string init $parts[0];
    if (len($parts) == 2) {
        $prefix = $parts[0];
        $local = $parts[1];
        if (not regex.matches(NCNAME_RE, $prefix)) {
            fail($what + " is not a QName: " + shown($value));
        }
    }
    if (not regex.matches(NCNAME_RE, $local)) {
        fail($what + " is not a QName: " + shown($value));
    }
    def ns as string init "";
    try {
        $ns = xml.lookupPrefix($node, $prefix);
    } catch (e) {
        fail($what + " " + shown($value) + " uses an undeclared prefix");
    }
    return [$ns, $local];
}

# wsdlIn reports whether an element is `local` in namespace `ns`.
func wsdlIn(node as xml.Value, ns as string, local as string) {
    return xml.namespace($node) == $ns and xml.localName($node) == $local;
}

# messageParts reads the parts of a WSDL message.
func messageParts(el as xml.Value) {
    def out as list of WsdlPart init [];
    def names as map of string to bool init {};
    for (def p in xml.children($el)) {
        if (not wsdlIn($p, WSDL_NS, "part")) {
            continue;
        }
        def part as WsdlPart init WsdlPart{
            name: wsdlName($p),
            elementNamespace: "",
            element: "",
            typeNamespace: "",
            typeName: "",
            qualified: false
        };
        if (maps.has($names, $part.name)) {
            fail("WSDL message " + wsdlName($el) + " has two parts named " + $part.name);
        }
        $names[$part.name] = true;
        def hasElement as bool init xml.hasAttr($p, "element");
        if ($hasElement == xml.hasAttr($p, "type")) {
            fail("WSDL part " + $part.name + " names exactly one of element and type");
        }
        if ($hasElement) {
            def q as list of string init wsdlQName($p, xml.attr($p, "element"), "part element");
            $part.elementNamespace = $q[0];
            $part.element = $q[1];
        } else {
            def q as list of string init wsdlQName($p, xml.attr($p, "type"), "part type");
            $part.typeNamespace = $q[0];
            $part.typeName = $q[1];
        }
        $out[] = $part;
    }
    return $out;
}

# checkStyle refuses a style other than document and rpc.
func checkStyle(style as string) {
    if ($style != "document" and $style != "rpc") {
        fail("a WSDL style is document or rpc, got " + shown($style));
    }
    return $style;
}

# readBinding reads a binding; one with no SOAP binding element has version "".
func readBinding(el as xml.Value, tns as string) {
    def type as list of string init wsdlQName($el, wsdlAttr($el, "type"), "binding type");
    def out as RawBinding init RawBinding{
        name: wsdlName($el),
        namespace: $tns,
        portType: clark($type[0], $type[1]),
        version: "",
        style: "document",
        ops: []
    };
    def soapNs as string init "";
    def overHttp as bool init false;
    for (def c in xml.children($el)) {
        if (wsdlIn($c, WSDL_SOAP11_NS, "binding") or wsdlIn($c, WSDL_SOAP12_NS, "binding")) {
            if ($soapNs != "") {
                fail("WSDL binding " + $out.name + " has two SOAP binding elements");
            }
            $soapNs = xml.namespace($c);
            $out.version = "1.1";
            if ($soapNs == WSDL_SOAP12_NS) {
                $out.version = "1.2";
            }
            if (xml.hasAttr($c, "style")) {
                $out.style = checkStyle(xml.attr($c, "style"));
            }
            if (not xml.hasAttr($c, "transport")) {
                fail("WSDL binding " + $out.name + " has a SOAP binding element without transport");
            }
            $overHttp = lists.contains(HTTP_TRANSPORTS, xml.attr($c, "transport"));
        }
    }
    if ($out.version == "") {
        return $out;
    }
    if (not $overHttp) {
        # SOAP over JMS, SMTP, ...: not callable here, left out with its ports
        $out.version = "other";
        return $out;
    }
    def names as map of string to bool init {};
    for (def c in xml.children($el)) {
        if (not wsdlIn($c, WSDL_NS, "operation")) {
            continue;
        }
        def op as RawOp init readBindingOp($c, $soapNs, $out.style);
        if (maps.has($names, $op.name)) {
            fail("WSDL binding " + $out.name + " declares operation " + $op.name +
                " twice (overloading is not supported)");
        }
        $names[$op.name] = true;
        $out.ops[] = $op;
    }
    return $out;
}

# readIO reads the `soap:body` and `soap:header` elements of a binding
# operation's input or output.
func readIO(io as xml.Value, soapNs as string) {
    def out as RawIO init RawIO{
        bodyUse: "literal",
        namespace: "",
        restricted: false,
        parts: [],
        headerMessages: [],
        headerParts: []
    };
    def bodies as int init 0;
    for (def b in xml.children($io)) {
        if (wsdlIn($b, $soapNs, "header")) {
            def m as list of string init wsdlQName(
                $b,
                wsdlAttr($b, "message"),
                "soap:header message");
            $out.headerMessages[] = clark($m[0], $m[1]);
            $out.headerParts[] = wsdlAttr($b, "part");
            continue;
        }
        if (not wsdlIn($b, $soapNs, "body")) {
            continue;
        }
        $bodies = $bodies + 1;
        if ($bodies > 1) {
            fail("a WSDL operation input or output has one soap:body");
        }
        $out.bodyUse = optAttr($b, "use");
        if ($out.bodyUse == "") {
            $out.bodyUse = "literal";
        }
        if ($out.bodyUse != "literal" and $out.bodyUse != "encoded") {
            fail("a WSDL soap:body use is literal or encoded, got " + shown($out.bodyUse));
        }
        $out.namespace = optAttr($b, "namespace");
        if (xml.hasAttr($b, "parts")) {
            $out.restricted = true;
            def names as string init strings.trim(xml.attr($b, "parts"));
            if ($names != "") {
                $out.parts = regex.split('\s+', $names);
            }
        }
    }
    return $out;
}

# readBindingOp reads one operation of a SOAP binding.
func readBindingOp(c as xml.Value, soapNs as string, style as string) {
    def none as RawIO init RawIO{
        bodyUse: "literal",
        namespace: "",
        restricted: false,
        parts: [],
        headerMessages: [],
        headerParts: []
    };
    def op as RawOp init RawOp{
        name: wsdlName($c),
        action: "",
        style: $style,
        input: $none,
        output: $none
    };
    for (def x in xml.children($c)) {
        if (wsdlIn($x, $soapNs, "operation")) {
            $op.action = optAttr($x, "soapAction");
            if (xml.hasAttr($x, "style")) {
                $op.style = checkStyle(xml.attr($x, "style"));
            }
        } elseif (wsdlIn($x, WSDL_NS, "input")) {
            $op.input = readIO($x, $soapNs);
        } elseif (wsdlIn($x, WSDL_NS, "output")) {
            $op.output = readIO($x, $soapNs);
        }
    }
    if ($op.style == "document") {
        $op.input.namespace = "";
    }
    return $op;
}

# readPorts reads the SOAP ports of a service; a port without a SOAP address
# (an HTTP or MIME binding) is left out.
func readPorts(el as xml.Value) {
    def service as string init wsdlName($el);
    def out as list of WsdlPort init [];
    for (def p in xml.children($el)) {
        if (not wsdlIn($p, WSDL_NS, "port")) {
            continue;
        }
        def port as WsdlPort init WsdlPort{
            service: $service,
            name: wsdlName($p),
            address: "",
            version: "",
            binding: "",
            bindingNamespace: ""
        };
        def b as list of string init wsdlQName($p, wsdlAttr($p, "binding"), "port binding");
        $port.bindingNamespace = $b[0];
        $port.binding = $b[1];
        for (def a in xml.children($p)) {
            if (wsdlIn($a, WSDL_SOAP11_NS, "address")) {
                $port.version = "1.1";
                $port.address = wsdlAttr($a, "location");
            } elseif (wsdlIn($a, WSDL_SOAP12_NS, "address")) {
                $port.version = "1.2";
                $port.address = wsdlAttr($a, "location");
            }
        }
        if ($port.version != "") {
            $out[] = $port;
        }
    }
    return $out;
}

# portOpOf reads the input and output message keys of a port type operation.
func portOpOf(op as xml.Value) {
    def po as PortOp init PortOp{input: "", output: ""};
    for (def io in xml.children($op)) {
        if (not wsdlIn($io, WSDL_NS, "input") and not wsdlIn($io, WSDL_NS, "output")) {
            continue;
        }
        def q as list of string init wsdlQName($io, wsdlAttr($io, "message"), "message");
        if (xml.localName($io) == "input") {
            $po.input = clark($q[0], $q[1]);
        } else {
            $po.output = clark($q[0], $q[1]);
        }
    }
    return $po;
}

# readPortType reads a port type's operations into `{ns}portType#op` keys.
func readPortType(el as xml.Value, key as string) {
    def out as map of string to PortOp init {};
    for (def op in xml.children($el)) {
        if (not wsdlIn($op, WSDL_NS, "operation")) {
            continue;
        }
        def name as string init wsdlName($op);
        if (maps.has($out, $key + "#" + $name)) {
            fail("WSDL port type " + wsdlName($el) + " declares operation " + $name +
                " twice (overloading is not supported)");
        }
        $out[$key + "#" + $name] = portOpOf($op);
    }
    return $out;
}

# qualifiedSchemas is the target namespaces of the inline schemas that set
# `elementFormDefault="qualified"`.
func qualifiedSchemas(types as xml.Value) {
    def out as list of string init [];
    for (def s in xml.children($types)) {
        if (wsdlIn($s, XSD_NS, "schema") and optAttr($s, "elementFormDefault") == "qualified") {
            $out[] = optAttr($s, "targetNamespace");
        }
    }
    return $out;
}

# readWsdlDoc reads one WSDL 1.1 document.
func readWsdlDoc(root as xml.Value) {
    if (xml.namespace($root) == WSDL20_NS) {
        fail("WSDL 2.0 is not supported, only WSDL 1.1");
    }
    if (not wsdlIn($root, WSDL_NS, "definitions")) {
        fail("the root is not a WSDL 1.1 definitions element but " + shown(xml.tag($root)) +
            " in " + shown(xml.namespace($root)));
    }
    def tns as string init optAttr($root, "targetNamespace");
    def imports as list of string init [];
    def messages as map of string to list of WsdlPart init {};
    def portTypes as map of string to bool init {};
    def portOps as map of string to PortOp init {};
    def bindings as list of RawBinding init [];
    def ports as list of WsdlPort init [];
    def qualified as map of string to bool init {};
    for (def el in xml.children($root)) {
        if (xml.namespace($el) != WSDL_NS) {
            continue;
        }
        def local as string init xml.localName($el);
        if ($local == "import") {
            $imports[] = wsdlAttr($el, "location");
        } elseif ($local == "types") {
            for (def ns in qualifiedSchemas($el)) {
                $qualified[$ns] = true;
            }
        } elseif ($local == "message") {
            def key as string init clark($tns, wsdlName($el));
            if (maps.has($messages, $key)) {
                fail("the WSDL defines message " + wsdlName($el) + " twice");
            }
            $messages[$key] = messageParts($el);
        } elseif ($local == "portType") {
            def key as string init clark($tns, wsdlName($el));
            if (maps.has($portTypes, $key)) {
                fail("the WSDL defines port type " + wsdlName($el) + " twice");
            }
            $portTypes[$key] = true;
            def ops as map of string to PortOp init readPortType($el, $key);
            for (def k in $ops) {
                $portOps[$k] = $ops[$k];
            }
        } elseif ($local == "binding") {
            $bindings[] = readBinding($el, $tns);
        } elseif ($local == "service") {
            for (def p in readPorts($el)) {
                $ports[] = $p;
            }
        }
    }
    return WsdlDoc{
        name: optAttr($root, "name"),
        targetNamespace: $tns,
        imports: $imports,
        messages: $messages,
        portTypes: $portTypes,
        portOps: $portOps,
        bindings: $bindings,
        ports: $ports,
        qualified: $qualified
    };
}

# partsOf looks up a message's parts and marks the qualified ones; "" (no
# message) has none.
func partsOf(all as WsdlParts, key as string, what as string) {
    def out as list of WsdlPart init [];
    if ($key == "") {
        return $out;
    }
    if (not maps.has($all.messages, $key)) {
        fail($what + " names the undefined message " + shown($key));
    }
    for (def p in $all.messages[$key]) {
        def part as WsdlPart init $p;
        if ($part.element != "" and maps.has($all.qualified, $part.elementNamespace)) {
            $part.qualified = true;
        }
        $out[] = $part;
    }
    return $out;
}

# bodyParts is the parts of a message that go in the Body: all of them, or
# those the `soap:body parts` names, in that order.
func bodyParts(parts as list of WsdlPart, io as RawIO, what as string) {
    if (not $io.restricted) {
        return $parts;
    }
    def out as list of WsdlPart init [];
    for (def name in $io.parts) {
        $out[] = $parts[partIndex($parts, $name, $what)];
    }
    return $out;
}

# headerParts is the parts each `soap:header` names.
func headerParts(all as WsdlParts, io as RawIO, what as string) {
    def out as list of WsdlPart init [];
    for (def i in 0..len($io.headerMessages)) {
        def parts as list of WsdlPart init partsOf($all, $io.headerMessages[$i], $what);
        $out[] = $parts[partIndex($parts, $io.headerParts[$i], $what)];
    }
    return $out;
}

# partIndex is the index of the part named `name`.
func partIndex(parts as list of WsdlPart, name as string, what as string) {
    for (def i in 0..len($parts)) {
        if ($parts[$i].name == $name) {
            return $i;
        }
    }
    fail($what + " names the part " + shown($name) + ", which its message does not have");
    return -1;
}

# assemble merges the documents and resolves every reference.
func assemble(docs as list of WsdlDoc) {
    def all as WsdlParts init WsdlParts{messages: {}, qualified: {}};
    def portTypes as map of string to bool init {};
    def portOps as map of string to PortOp init {};
    def raw as list of RawBinding init [];
    def ports as list of WsdlPort init [];
    for (def d in $docs) {
        for (def k in $d.messages) {
            if (maps.has($all.messages, $k)) {
                fail("the WSDL defines message " + shown($k) + " twice");
            }
            $all.messages[$k] = $d.messages[$k];
        }
        for (def k in $d.qualified) {
            $all.qualified[$k] = true;
        }
        for (def k in $d.portTypes) {
            if (maps.has($portTypes, $k)) {
                fail("the WSDL defines port type " + shown($k) + " twice");
            }
            $portTypes[$k] = true;
        }
        for (def k in $d.portOps) {
            $portOps[$k] = $d.portOps[$k];
        }
        for (def b in $d.bindings) {
            $raw[] = $b;
        }
        for (def p in $d.ports) {
            $ports[] = $p;
        }
    }
    def bindings as list of WsdlBinding init [];
    # binding key to its version: "1.1", "1.2", "" (not SOAP) or "other"
    def known as map of string to string init {};
    for (def b in $raw) {
        def key as string init clark($b.namespace, $b.name);
        if (maps.has($known, $key)) {
            fail("the WSDL defines binding " + shown($key) + " twice");
        }
        $known[$key] = $b.version;
        if ($b.version == "" or $b.version == "other") {
            continue;
        }
        if (not maps.has($portTypes, $b.portType)) {
            fail("WSDL binding " + $b.name + " names the undefined port type " +
                shown($b.portType));
        }
        def ops as list of WsdlOperation init [];
        for (def o in $b.ops) {
            def pkey as string init $b.portType + "#" + $o.name;
            if (not maps.has($portOps, $pkey)) {
                fail("WSDL binding " + $b.name + " operation " + $o.name +
                    " is not in its port type");
            }
            def what as string init "WSDL operation " + $o.name;
            $ops[] = WsdlOperation{
                name: $o.name,
                action: $o.action,
                style: $o.style,
                bodyUse: $o.input.bodyUse,
                namespace: $o.input.namespace,
                input: bodyParts(partsOf($all, $portOps[$pkey].input, $what), $o.input, $what),
                output: bodyParts(partsOf($all, $portOps[$pkey].output, $what), $o.output, $what),
                inputHeaders: headerParts($all, $o.input, $what),
                outputHeaders: headerParts($all, $o.output, $what)
            };
        }
        $bindings[] = WsdlBinding{
            name: $b.name,
            namespace: $b.namespace,
            version: $b.version,
            style: $b.style,
            operations: $ops
        };
    }
    def kept as list of WsdlPort init [];
    for (def p in $ports) {
        def key as string init clark($p.bindingNamespace, $p.binding);
        if (not maps.has($known, $key)) {
            fail("WSDL port " + $p.name + " names the undefined binding " + shown($key));
        }
        def version as string init $known[$key];
        if ($version == "") {
            fail("WSDL port " + $p.name + " has a SOAP address but binding " + $p.binding +
                " is not SOAP");
        }
        if ($version == "other") {
            continue;
        }
        if ($version != $p.version) {
            fail("WSDL port " + $p.name + " has a SOAP " + $p.version + " address but binding " +
                $p.binding + " is SOAP " + $version);
        }
        $kept[] = $p;
    }
    return Wsdl{
        name: $docs[0].name,
        targetNamespace: $docs[0].targetNamespace,
        ports: $kept,
        bindings: $bindings
    };
}

/**
 * Read a WSDL 1.1 description that imports no other WSDL document.
 *
 * @param text {string} the WSDL document
 * @return {Wsdl} its SOAP ports and bindings
 * @throws {SoapError} kind `"soap"` as `parseWsdlWith`, and for any `wsdl:import`
 * @throws {Error} kind `"limit"` for a document past `xml`'s decoding caps
 */
export func parseWsdl(text as string) {
    return parseWsdlWith($text, {});
}

/**
 * Read a WSDL 1.1 description and the WSDL documents it imports. Nothing is
 * fetched: `imports` maps each `wsdl:import` `location` to that document's
 * text, read by the caller. An import of an import is looked up in the same
 * map; each location, and each document text, is read once, so an import back
 * to the importing document is no error. XML Schema imports (`xs:import`) are not
 * followed: the parts name their elements and types, not their contents.
 *
 * @param text {string} the WSDL document
 * @param imports {map of string to string} location to document text
 * @return {Wsdl} the SOAP ports and bindings of all the documents
 * @throws {SoapError} kind `"soap"` on malformed XML, a DOCTYPE, a root that is
 *   not WSDL 1.1 `definitions` (WSDL 2.0 is refused by name), a missing or
 *   malformed required attribute, an undeclared prefix, a name defined twice,
 *   an overloaded operation, a reference to an undefined message, port type or
 *   binding, a style other than document or rpc, or an import not in `imports`
 * @throws {Error} kind `"limit"` past `MAX_WSDL_DOCUMENTS` documents or `xml`'s
 *   decoding caps
 */
export func parseWsdlWith(text as string, imports as map of string to string) {
    def docs as list of WsdlDoc init [readWsdlDoc(wsdlRoot($text))];
    def seen as map of string to bool init {};
    # The texts read, so a document imported back (the root has no location of
    # its own) or under a second location is read once.
    def texts as map of string to bool init {};
    $texts[$text] = true;
    def i as int init 0;
    while ($i < len($docs)) {
        for (def location in $docs[$i].imports) {
            if (maps.has($seen, $location)) {
                continue;
            }
            $seen[$location] = true;
            if (not maps.has($imports, $location)) {
                fail("the WSDL imports " + shown($location) + "; pass that document in " +
                    "parseWsdlWith's imports");
            }
            if (maps.has($texts, $imports[$location])) {
                continue;
            }
            $texts[$imports[$location]] = true;
            if (len($docs) >= MAX_WSDL_DOCUMENTS) {
                throw Error{
                    kind: "limit",
                    message: "soap: a WSDL reads at most " + convert.toString(MAX_WSDL_DOCUMENTS) +
                        " documents"
                };
            }
            $docs[] = readWsdlDoc(wsdlRoot($imports[$location]));
        }
        $i = $i + 1;
    }
    return assemble($docs);
}

/**
 * The first SOAP port that offers an operation, with the operation.
 *
 * @param w {Wsdl} the description
 * @param name {string} the operation's name
 * @return {WsdlCall} its address, version and operation
 * @throws {SoapError} kind `"soap"` when no SOAP port offers it
 */
export func wsdlCall(w as Wsdl, name as string) {
    return wsdlCallAt($w, "", $name);
}

/**
 * An operation at a named port (the first port of that name, in any service);
 * "" for the first SOAP port that offers it.
 *
 * @param w {Wsdl} the description
 * @param port {string} the port's name, or ""
 * @param name {string} the operation's name
 * @return {WsdlCall} its address, version and operation
 * @throws {SoapError} kind `"soap"` on an unknown port, or when no matching port
 *   offers the operation
 */
export func wsdlCallAt(w as Wsdl, port as string, name as string) {
    # Each binding is looked up once by key and searched once, however many
    # ports share it: a lookup is linear in the description's size.
    def index as map of string to int init {};
    for (def i in 0..len($w.bindings)) {
        $index[clark($w.bindings[$i].namespace, $w.bindings[$i].name)] = $i;
    }
    def searched as map of int to bool init {};
    def found as bool init false;
    for (def p in $w.ports) {
        if ($port != "" and $p.name != $port) {
            continue;
        }
        $found = true;
        def key as string init clark($p.bindingNamespace, $p.binding);
        if (not maps.has($index, $key)) {
            fail("WSDL port " + $p.name + " names the undefined binding " + shown($key));
        }
        def b as int init $index[$key];
        if (maps.has($searched, $b)) {
            continue;
        }
        $searched[$b] = true;
        def o as int init operationIndex($w, $b, $name);
        if ($o >= 0) {
            return WsdlCall{
                address: $p.address,
                version: $p.version,
                operation: $w.bindings[$b].operations[$o]
            };
        }
    }
    if ($port != "" and not $found) {
        fail("the WSDL has no SOAP port named " + shown($port));
    }
    fail("no SOAP port of the WSDL offers the operation " + shown($name));
    return WsdlCall{address: "", version: "", operation: emptyOperation()};
}

/**
 * An operation from the first SOAP binding that has it, sent to `address`: for
 * a description with no `service` (ONVIF's device WSDLs, whose address comes
 * from discovery) or whose ports name another host.
 *
 * @param w {Wsdl} the description
 * @param address {string} the endpoint URL
 * @param name {string} the operation's name
 * @return {WsdlCall} the address, the binding's version and the operation
 * @throws {SoapError} kind `"soap"` when no SOAP binding has the operation
 */
export func wsdlCallTo(w as Wsdl, address as string, name as string) {
    for (def b in 0..len($w.bindings)) {
        def o as int init operationIndex($w, $b, $name);
        if ($o >= 0) {
            return WsdlCall{
                address: $address,
                version: $w.bindings[$b].version,
                operation: $w.bindings[$b].operations[$o]
            };
        }
    }
    fail("no SOAP binding of the WSDL has the operation " + shown($name));
    return WsdlCall{address: "", version: "", operation: emptyOperation()};
}

# operationIndex is the index of an operation in binding `b`, -1 when absent.
func operationIndex(w as Wsdl, b as int, name as string) {
    for (def i in 0..len($w.bindings[$b].operations)) {
        if ($w.bindings[$b].operations[$i].name == $name) {
            return $i;
        }
    }
    return -1;
}

# emptyOperation is the zero WsdlOperation, for a return after `fail`.
func emptyOperation() {
    return WsdlOperation{
        name: "",
        action: "",
        style: "",
        bodyUse: "",
        namespace: "",
        input: [],
        output: [],
        inputHeaders: [],
        outputHeaders: []
    };
}

/**
 * A client for an operation's endpoint, speaking its SOAP version.
 *
 * @param target {WsdlCall} the operation, from `wsdlCall`
 * @return {Client} `connect` to the address, with `withVersion`
 * @throws {SoapError} kind `"soap"` as `connect`
 */
export func wsdlConnect(target as WsdlCall) {
    return withVersion(connect($target.address), $target.version);
}

/**
 * The request body of an operation with one text child per parameter: for
 * document/literal the part's element (children qualified as its schema
 * says), for rpc/literal a wrapper named after the operation in the
 * `soap:body` namespace. Build any other shape with the `xml` builders.
 *
 * @param target {WsdlCall} the operation, from `wsdlCall`
 * @param params {map of string to string} child name to text, in order
 * @return {xml.Value} the body element, for `call` with the operation's action
 * @throws {SoapError} kind `"soap"` for an encoded operation, an rpc operation
 *   without a namespace, a document operation whose input is not one element
 *   part, or a parameter `operation` refuses
 */
export func wsdlRequest(target as WsdlCall, params as map of string to string) {
    def o as WsdlOperation init $target.operation;
    if ($o.bodyUse == "encoded") {
        fail("operation " + $o.name + " uses SOAP encoding, which the module does not write");
    }
    if ($o.style == "rpc") {
        if ($o.namespace == "") {
            fail("rpc operation " + $o.name + " has no soap:body namespace");
        }
        return operation($o.namespace, $o.name, $params, false);
    }
    if (len($o.input) != 1 or $o.input[0].element == "") {
        fail("the request of operation " + $o.name + " is not one schema element; " +
            "build the body with the xml builders");
    }
    def part as WsdlPart init $o.input[0];
    if ($part.elementNamespace == "") {
        fail("the request element " + $part.element + " of operation " + $o.name +
            " is in no namespace; build the body with the xml builders");
    }
    return operation($part.elementNamespace, $part.element, $params, $part.qualified);
}
