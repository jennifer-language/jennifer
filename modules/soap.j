# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.25.0

/**
 * SOAP 1.1 and 1.2 over HTTP: a client that calls an operation and returns its
 * result element, and a transport-agnostic server dispatcher.
 *
 * The **protocol layer** (`envelope`, `parse`, `Fault`, `parseFault`,
 * `faultEnvelope`) knows nothing about sockets. The **client** posts
 * envelopes over `http`; the **server** side, `handle`, turns a request body
 * into a reply body for any transport, such as an `httpd` loop. Payloads are
 * `xml.Value` elements: build them with `operation` or the `xml` builders and
 * read them with `values` or the `xml` accessors. No WSDL processing: take the
 * namespace, operation names and SOAPAction from the service's WSDL or its
 * documentation. Needs the default `jennifer` binary (`net` via `http`).
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

# The `action` parameter of a SOAP 1.2 Content-Type.
def const ACTION_PARAM_RE as string init '(?i);\s*action\s*=\s*("[^"]*"|[^;\s]+)';

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
 * @field detail {string} the detail's content: its XML when it holds elements,
 *   else its text; "" for none
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
 */
export def struct Client {
    endpoint as string,
    version as string,
    headers as map of string to string,
    soapHeaders as list of xml.Value,
    tls as http.TlsOptions,
    timeoutMs as int,
    maxBytes as int,
    digest as DigestCredentials
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

func fail(message as string) {
    throw Error{kind: ERROR_KIND, message: "soap: " + $message};
}

# plain cuts a string for an error message and replaces anything but printable
# ASCII, so a message stays one line and carries no terminal escape.
func plain(text as string) {
    def cut as string init $text;
    def more as string init "";
    if (len($text) > SHOWN) {
        $cut = strings.substring($text, 0, SHOWN);
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
 * @throws {Error} kind `"soap"` on an unknown version, or an action with a
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
        def m as regex.Match init regex.find(ACTION_PARAM_RE, $contentType);
        if ($m.start >= 0) {
            $a = $m.groups[0];
        }
    }
    if (len($a) >= 2 and strings.startsWith($a, "\"") and strings.endsWith($a, "\"")) {
        $a = strings.substring($a, 1, len($a) - 1);
    }
    return $a;
}

/**
 * Write an envelope.
 *
 * @param version {string} `1.1` or `1.2`
 * @param headers {list of xml.Value} header blocks; [] for no `Header`
 * @param body {list of xml.Value} the `Body`'s children, usually one
 * @return {string} the document, with an XML declaration
 * @throws {Error} kind `"soap"` on an unknown version
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
    # The regex can scan the whole text; most messages never reach it.
    def at as int init strings.indexOf($text, "<!DOCTYPE");
    if ($at >= 0 and regex.matches(DOCTYPE_RE, strings.substring($text, 0, $at + 9))) {
        fail("a SOAP message must not carry a DOCTYPE");
    }
    def root as xml.Value;
    try {
        $root = xml.decode($text);
    } catch (e) {
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
    for (def child in xml.children($root)) {
        $seen = $seen + 1;
        def local as string init xml.localName($child);
        if (xml.namespace($child) == $ns and $local == "Header" and $seen == 1) {
            $headers = xml.children($child);
        } elseif (xml.namespace($child) == $ns and $local == "Body") {
            $bodies = $bodies + 1;
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
    return Envelope{version: $version, headers: $headers, body: $body};
}

/**
 * Decode an envelope of either version.
 *
 * @param text {string} the document
 * @return {Envelope} the envelope
 * @throws {Error} kind `"soap"` on malformed XML, a DOCTYPE, a root that is
 *   not a 1.1 or 1.2 `Envelope`, or a missing or repeated `Body`
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
        $prefix = strings.substring($text, 0, $colon);
        $local = strings.substring($text, $colon + 1);
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
 * The content of a detail element: its children's XML, else its text.
 *
 * @param node {xml.Value} the `detail` / `Detail` element
 * @return {string} the content
 */
func detailOf(node as xml.Value) {
    def kids as list of xml.Value init xml.children($node);
    if (len($kids) == 0) {
        return strings.trim(xml.text($node));
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
 * @throws {Error} kind `"soap"` when the envelope is no fault, or a 1.2 fault
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
        if ($close < 2 or not regex.matches(NCNAME_RE, strings.substring($code, $close + 1))) {
            fail("a fault code is a name or \{namespace\}local, got " + shown($code));
        }
        if ($standard) {
            fail("a SOAP 1.2 fault code is one of " + strings.join(CODES12, ", ") +
                "; put " + shown($code) + " in the subcode");
        }
        def uri as string init strings.substring($code, 1, $close);
        return xml.setText(
            xml.setAttr($node, "xmlns:app", $uri),
            "app:" + strings.substring($code, $close + 1));
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
 * A detail element filled from `Fault.detail`: XML content is decoded and
 * appended, anything else becomes text.
 *
 * @param node {xml.Value} the empty detail element
 * @param detail {string} the content
 * @return {xml.Value} the filled element
 */
func withDetail(node as xml.Value, detail as string) {
    if (not strings.startsWith(strings.trim($detail), "<")) {
        return xml.setText($node, xmlSafe($detail));
    }
    def wrapper as xml.Value;
    try {
        $wrapper = xml.decode("<d>" + $detail + "</d>");
    } catch (e) {
        fail("a fault detail that starts with < is XML: " + plain($e.message));
    }
    def n as xml.Value init $node;
    for (def k in xml.children($wrapper)) {
        $n = xml.append($n, $k);
    }
    return $n;
}

/**
 * The `Fault` element of a version.
 *
 * @param version {string} `1.1` or `1.2`
 * @param f {Fault} the fault; `code` is required, `reason` should be set.
 *   Characters XML 1.0 does not allow are written as `?`
 * @return {xml.Value} the element, for an envelope's Body
 * @throws {Error} kind `"soap"` on an unknown version, a malformed code, a
 *   1.2 code that is not a standard one, or a detail starting with `<` that
 *   is not XML
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
            $out = xml.append($out, withDetail(xml.element("detail"), $f.detail));
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
        $out = xml.append($out, withDetail(xml.elementNS($ns, "soap:Detail"), $f.detail));
    }
    return $out;
}

/**
 * Write a fault envelope.
 *
 * @param version {string} `1.1` or `1.2`
 * @param f {Fault} the fault
 * @return {string} the document
 * @throws {Error} kind `"soap"` as `faultElement`
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
 * @throws {Error} kind `"soap"` on an empty namespace, a name that is not an
 *   XML name, or a value with a character XML 1.0 does not allow
 */
export func operation(
    ns as string,
    name as string,
    params as map of string to string,
    qualified as bool) {
    if ($ns == "") {
        fail("an operation has a namespace");
    }
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
 * @throws {Error} kind `"soap"` on a URL that is not http(s), or carries
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
        tls: http.TlsOptions{caCert: convert.bytesFromString("", "utf-8"), skipVerify: false},
        timeoutMs: DEFAULT_TIMEOUT_MS,
        maxBytes: DEFAULT_MAX_BYTES,
        digest: DigestCredentials{user: "", password: ""}
    };
}

/**
 * Speak SOAP 1.1 or 1.2.
 *
 * @param c {Client} the client
 * @param version {string} `1.1` or `1.2`
 * @return {Client} the changed copy
 * @throws {Error} kind `"soap"` on another version
 */
export func withVersion(c as Client, version as string) {
    namespaceOf($version);
    $c.version = $version;
    return $c;
}

/**
 * Send an HTTP header with every call.
 *
 * @param c {Client} the client
 * @param name {string} the header name
 * @param value {string} its value
 * @return {Client} the changed copy
 * @throws {Error} kind `"soap"` on a bad name, a value with a control
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
    $c.headers[$name] = $value;
    return $c;
}

/**
 * Authenticate with a user name and password (HTTP Basic). Over `http://`
 * they travel readable; use `https://`.
 *
 * @param c {Client} the client
 * @param user {string} the user name, without `:`
 * @param password {string} the password
 * @return {Client} the changed copy
 * @throws {Error} kind `"soap"` on a `:` in the name or a control character
 */
export func withBasicAuth(c as Client, user as string, password as string) {
    if (strings.contains($user, ":") or regex.matches(CONTROL_RE, $user + $password)) {
        fail("a Basic user name holds no :, and neither holds control characters");
    }
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
 * @throws {Error} kind `"soap"` on an empty token or one with a space or
 *   control character
 */
export func withToken(c as Client, token as string) {
    if ($token == "" or regex.matches('[\s\x00-\x1f\x7f]', $token)) {
        fail("a token is not empty and holds no spaces or control characters");
    }
    $c.headers["Authorization"] = "Bearer " + $token;
    $c.digest = DigestCredentials{user: "", password: ""};
    return $c;
}

/**
 * Authenticate with HTTP Digest (RFC 7616): each call goes out without
 * credentials and answers the endpoint's `401` challenge (`http.sendDigest`),
 * so it costs one extra round trip. Replaces Basic or bearer auth set before;
 * `withBasicAuth` / `withToken` replace it in turn.
 *
 * @param c {Client} the client
 * @param user {string} the user name
 * @param password {string} the password
 * @return {Client} the changed copy
 * @throws {Error} kind `"soap"` on an empty user name or a control character
 *   in it
 */
export func withDigestAuth(c as Client, user as string, password as string) {
    if ($user == "" or regex.matches(CONTROL_RE, $user)) {
        fail("a Digest user name is not empty and holds no control characters");
    }
    def kept as map of string to string init {};
    for (def name in $c.headers) {
        if (strings.lower($name) != "authorization") {
            $kept[$name] = $c.headers[$name];
        }
    }
    $c.headers = $kept;
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
 * @throws {Error} kind `"soap"` on a text node or an element without
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
 * Trust a certificate (PEM) in addition to the system's.
 *
 * @param c {Client} the client
 * @param pem {bytes} the certificate
 * @return {Client} the changed copy
 */
export func withCaCert(c as Client, pem as bytes) {
    $c.tls.caCert = $pem;
    $c.tls.skipVerify = false;
    return $c;
}

/**
 * Accept any certificate. The connection stays encrypted but is no longer
 * authenticated; prefer `withCaCert`.
 *
 * @param c {Client} the client
 * @return {Client} the changed copy
 */
export func withInsecure(c as Client) {
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
 * @throws {Error} kind `"soap"` on a transport error, a reply that is not an
 *   envelope (with the HTTP status when it is not 2xx), a non-fault reply in
 *   the other SOAP version, or a non-2xx status without a fault
 */
export func send(c as Client, action as string, body as xml.Value) {
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
        fail("POST " + plain(strings.split($c.endpoint, "?")[0]) + ": " + plain($e.message));
    }
    def ok as bool init $r.status >= 200 and $r.status < 300;
    if (strings.trim($r.body) == "") {
        if (not $ok) {
            fail("HTTP " + convert.toString($r.status) + " " + plain($r.statusText));
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
        if (not $ok) {
            fail("HTTP " + convert.toString($r.status) + " " + plain($r.statusText) +
                " without a SOAP reply");
        }
        throw $e;
    }
    if (isFault($env)) {
        return Reply{status: $r.status, envelope: $env, isFault: true, fault: parseFault($env)};
    }
    if ($env.version != $c.version) {
        fail("a SOAP " + $c.version + " call got a SOAP " + $env.version + " reply");
    }
    if (not $ok) {
        fail("HTTP " + convert.toString($r.status) + " " + plain($r.statusText) +
            " without a fault");
    }
    return Reply{
        status: $r.status,
        envelope: $env,
        isFault: false,
        fault: Fault{code: "", subcode: "", reason: "", actor: "", detail: ""}
    };
}

/**
 * Call an operation and return its result element.
 *
 * @param c {Client} the client
 * @param action {string} the SOAPAction URI from the WSDL; "" for none
 * @param body {xml.Value} the operation element (see `operation`)
 * @return {xml.Value} the reply's first Body child, e.g. `AddResponse`
 * @throws {Error} kind `"soap"` on a fault (`soap: fault CODE: REASON`; use
 *   `send` to read the `Fault`), an empty reply Body, or as `send`
 */
export func call(c as Client, action as string, body as xml.Value) {
    def r as Reply init send($c, $action, $body);
    if ($r.isFault) {
        fail("fault " + plain($r.fault.code) + ": " + plain($r.fault.reason));
    }
    if (len($r.envelope.body) == 0) {
        fail("the reply Body is empty");
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
 * Whether a header block must be understood here: `mustUnderstand` is `1` or
 * `true`, and the block targets this node, the ultimate receiver. That is a
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
    if ($flag != "1" and $flag != "true") {
        return false;
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
        if (mustUnderstand($block, $ns) and not lists.contains($understood, $name)) {
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
        if ($e.kind == FAULT_KIND) {
            return faultResponse($version, "Client", $e.message);
        }
        return faultResponse($version, "Server", "internal error");
    }
    return Response{
        status: 200,
        contentType: contentType($version, ""),
        body: envelope($version, [], [$result])
    };
}
