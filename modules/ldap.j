# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
# pragma-jennifer-version: >=0.24.0

/**
 * An LDAP v3 client and a lightweight, read-only directory server (RFC 4511),
 * over TCP with the wire messages built and parsed as ASN.1 BER (the `asn1`
 * library) on the `net` transport. The client binds (simple or SASL SCRAM),
 * searches with RFC 4515 string filters, follows paged results, and unbinds;
 * LDAPS and StartTLS reuse `net`'s TLS. The server answers simple bind and
 * search for an in-memory directory you build in a few lines - enough to back
 * any LDAP client (an authentication portal such as Authelia, or an app that
 * authenticates its users against LDAP): it validates `userPassword` on bind and
 * evaluates search filters against the entries so a client can find users and
 * read their groups.
 *
 * The server is read-only over the LDAP protocol (it answers bind and search
 * but rejects add / modify / delete on the wire), yet the in-memory directory is
 * fully mutable from your own code: after building it you can addEntry /
 * modifyEntry / deleteEntry / setAttribute at runtime, so a web interface or a
 * database sync can update the directory a running server is serving. The
 * Directory is shared mutable state (a kv-backed store shared across the serve
 * loop's connection tasks), so a change is visible to the live server
 * immediately. No schema enforcement, no ACLs.
 *
 * @module ldap
 * @example
 * import "ldap.j" as ldap;
 * import "transport.j" as transport;
 * use io;
 * def c as ldap.Conn init ldap.connect("localhost:389", transport.Security.None);
 * ldap.bind($c, "cn=admin,dc=example,dc=org", "secret");
 * def hits as list of ldap.Entry init ldap.search($c,
 *     "ou=people,dc=example,dc=org", ldap.SCOPE_SUB,
 *     ldap.parseFilter("(uid=alice)"), ["cn", "mail"]);
 * for (def e in $hits) { io.printf("%s -> %s\n", $e.dn, ldap.firstValue($e, "mail")); }
 * ldap.unbind($c);
 */
use asn1;
use net;
use io;
use convert;
use strings;
use math;
use encoding;
use hash;
use crypto;
use binary;
use kv;
use json;

import "./sasl.j" as sasl;
import "./transport.j" as transport;

# --- constants ---

/** Search scope: the base entry only. */
export def const SCOPE_BASE as int init 0;
/** Search scope: the base's immediate children. */
export def const SCOPE_ONE as int init 1;
/** Search scope: the base and its whole subtree. */
export def const SCOPE_SUB as int init 2;

/** LDAP result code: success. */
export def const SUCCESS as int init 0;
/** LDAP result code: invalid credentials (a failed bind). */
export def const INVALID_CREDENTIALS as int init 49;

def const DEFAULT_PORT as int init 389;
def const DEFAULT_TLS_PORT as int init 636;
def const DEFAULT_TIMEOUT as int init 5000;

# Internal result codes used by the server and SASL bind.
def const R_PROTOCOL_ERROR as int init 2;
def const R_AUTH_METHOD_NOT_SUPPORTED as int init 7;
def const R_SASL_BIND_IN_PROGRESS as int init 14;

# protocolOp application tags (RFC 4511 section 4.2 onward).
def const APP_BIND_REQ as int init 0;
def const APP_BIND_RESP as int init 1;
def const APP_UNBIND as int init 2;
def const APP_SEARCH_REQ as int init 3;
def const APP_SEARCH_ENTRY as int init 4;
def const APP_SEARCH_DONE as int init 5;
def const APP_SEARCH_REF as int init 19;
def const APP_EXT_REQ as int init 23;
def const APP_EXT_RESP as int init 24;

def const DEREF_NEVER as int init 0;
def const LDAP_VERSION as int init 3;

/** modify() change type: add the given values to the attribute. */
export def const MOD_ADD as int init 0;
/** modify() change type: delete the given values (or the whole attribute if none). */
export def const MOD_DELETE as int init 1;
/** modify() change type: replace the attribute with the given values. */
export def const MOD_REPLACE as int init 2;

# RFC 3062 password-modify extended-operation OID.
def const PASSWD_MODIFY_OID as string init "1.3.6.1.4.1.4203.1.11.1";

# Iteration count for the pbkdf2 password schemes (a deliberately slow KDF).
def const PBKDF2_ITERATIONS as int init 100000;

# StartTLS + paged-results control OIDs.
def const STARTTLS_OID as string init "1.3.6.1.4.1.1466.20037";
def const PAGED_OID as string init "1.2.840.113556.1.4.319";

# A server connection uses an idle read deadline: it waits this long for the
# next request, then drops the connection - so a stalled or slow-loris peer
# cannot park a goroutine (and its read buffer) forever.
def const SERVER_IDLE_MS as int init 60000;

# Largest LDAP message the codec will read, on both the client and the server.
# Caps the up-front net.readN allocation a hostile length header can force
# (16 MiB is well above any real bind / search message).
def const MAX_MSG_BYTES as int init 16777216;

# Upper bound on a stored PBKDF2 iteration count the server will honour on bind,
# so a hostile stored userPassword cannot stall the bind handler for minutes.
def const MAX_PBKDF2_ITERATIONS as int init 10000000;

# The single kv key under which a Directory stores its whole entry set.
def const DIR_KEY as string init "entries";

# --- structs ---

/**
 * An open LDAP connection.
 * @field handle {net.Conn} the underlying TCP (or TLS) connection
 * @field timeoutMs {int} the per-read timeout in ms (0 blocks with no deadline)
 */
export def struct Conn {
    handle as net.Conn,
    timeoutMs as int
};

/**
 * One attribute of a directory entry.
 * @field name {string} the attribute type (e.g. "mail", "objectClass")
 * @field values {list of string} its values (LDAP attributes are multi-valued)
 */
export def struct Attribute {
    name as string,
    values as list of string
};

/**
 * A directory entry.
 * @field dn {string} the distinguished name
 * @field attributes {list of ldap.Attribute} the entry's attributes
 */
export def struct Entry {
    dn as string,
    attributes as list of Attribute
};

/**
 * The outcome of a bind (or any LDAPResult).
 * @field code {int} the result code (ldap.SUCCESS, or ldap.INVALID_CREDENTIALS on a bad bind)
 * @field matchedDn {string} the matched DN the server reports, if any
 * @field message {string} the server's diagnostic message
 */
export def struct Result {
    code as int,
    matchedDn as string,
    message as string
};

/**
 * One change in a modify request.
 * @field operation {int} ldap.MOD_ADD / MOD_DELETE / MOD_REPLACE
 * @field name {string} the attribute to change
 * @field values {list of string} the values (empty deletes the whole attribute for MOD_DELETE)
 */
export def struct Change {
    operation as int,
    name as string,
    values as list of string
};

/**
 * A mutable in-memory directory the server answers for. It is backed by a
 * shared kv store, so edits made with addEntry / modifyEntry / deleteEntry /
 * setAttribute are visible to a running server across its connection tasks.
 * @field store {kv.Store} the shared backing store holding every entry
 */
export def struct Directory {
    store as kv.Store
};

# Internal: a filter parse result threads the consumed position back to the caller.
def struct FilterAt {
    filter as asn1.Value,
    pos as int
};

# Internal: the entries plus the paged-results cookie from one search exchange.
def struct PagedState {
    entries as list of Entry,
    cookie as bytes
};

# --- small helpers ---

func fail(msg as string) {
    throw Error{kind: "ldap", message: "ldap: " + $msg, file: "", line: 0, col: 0};
}

# oct wraps a UTF-8 string as an ASN.1 OCTET STRING (asn1.octetString takes bytes).
func oct(s as string) {
    return asn1.octetString(convert.bytesFromString($s, "utf-8"));
}

# primStr reads a primitive node's content as a UTF-8 string. Used for
# context-tagged primitives (present filter, substring parts, SASL creds) that
# asn1.asString rejects because their universal string tag was replaced.
func primStr(node as asn1.Value) {
    return convert.stringFromBytes(asn1.asBytes($node), "utf-8");
}

func emptyBytes() {
    def b as bytes;
    return $b;
}

func nextId() {
    return math.randInt(1, 2000000000);
}

func ensurePort(address as string, defPort as int) {
    if (strings.contains($address, ":")) {
        return $address;
    }
    return $address + ":" + convert.toString($defPort);
}

# normDn canonicalises a DN for comparison: lowercase, trimmed RDNs.
func normDn(dn as string) {
    def parts as list of string init strings.split($dn, ",");
    def out as list of string;
    for (def p in $parts) {
        $out[] = strings.lower(strings.trim($p));
    }
    return strings.join($out, ",");
}

# compareStr returns -1 / 0 / 1 ordering two strings lexicographically (the
# string operators compare by UTF-8 byte, matching the old hand-rolled loop).
func compareStr(a as string, b as string) {
    if ($a < $b) {
        return -1;
    }
    if ($a > $b) {
        return 1;
    }
    return 0;
}

# --- entry accessors ---

/**
 * values returns an entry's values for an attribute (case-insensitive name), or an empty list.
 * @param entry {Entry} the entry to read
 * @param name {string} the attribute name, matched case-insensitively
 * @return {list of string} the attribute's values, or an empty list if absent
 */
export func values(entry as Entry, name as string) {
    for (def a in $entry.attributes) {
        if (strings.lower($a.name) == strings.lower($name)) {
            return $a.values;
        }
    }
    def empty as list of string;
    return $empty;
}

/**
 * firstValue returns an entry's first value for an attribute, or "" if absent.
 * @param entry {Entry} the entry to read
 * @param name {string} the attribute name, matched case-insensitively
 * @return {string} the first value, or "" if the attribute has none
 */
export func firstValue(entry as Entry, name as string) {
    def vs as list of string init values($entry, $name);
    if (len($vs) == 0) {
        return "";
    }
    return $vs[0];
}

# --- filter constructors (RFC 4511 Filter, all context-tagged) ---

/**
 * equals builds an equality filter: (attr=value).
 * @param attr {string} the attribute name
 * @param value {string} the value to match exactly
 * @return {asn1.Value} the encoded filter, for search
 */
export func equals(attr as string, value as string) {
    return asn1.retag("context", 3, asn1.sequence([oct($attr), oct($value)]));
}

/**
 * greaterOrEqual builds an (attr>=value) filter.
 * @param attr {string} the attribute name
 * @param value {string} the lower bound to compare against
 * @return {asn1.Value} the encoded filter, for search
 */
export func greaterOrEqual(attr as string, value as string) {
    return asn1.retag("context", 5, asn1.sequence([oct($attr), oct($value)]));
}

/**
 * lessOrEqual builds an (attr<=value) filter.
 * @param attr {string} the attribute name
 * @param value {string} the upper bound to compare against
 * @return {asn1.Value} the encoded filter, for search
 */
export func lessOrEqual(attr as string, value as string) {
    return asn1.retag("context", 6, asn1.sequence([oct($attr), oct($value)]));
}

/**
 * approx builds an approximate-match (attr~=value) filter; the server treats it as equality.
 * @param attr {string} the attribute name
 * @param value {string} the value to match approximately
 * @return {asn1.Value} the encoded filter, for search
 */
export func approx(attr as string, value as string) {
    return asn1.retag("context", 8, asn1.sequence([oct($attr), oct($value)]));
}

/**
 * present builds a presence filter: (attr=*).
 * @param attr {string} the attribute that must be present
 * @return {asn1.Value} the encoded filter, for search
 */
export func present(attr as string) {
    return asn1.retag("context", 7, oct($attr));
}

/**
 * substrings builds a substring filter, e.g. (cn=a*b*c): initial "a", any ["b"],
 * final "c". Pass "" for an absent initial or final and an empty list for no
 * interior parts.
 * @param attr {string} the attribute name
 * @param initial {string} the required leading substring, or "" for none
 * @param anyParts {list of string} interior substrings in order, or empty for none
 * @param final {string} the required trailing substring, or "" for none
 * @return {asn1.Value} the encoded filter, for search
 */
export func substrings(
    attr as string,
    initial as string,
    anyParts as list of string,
    final as string) {
    def subs as list of asn1.Value;
    if ($initial != "") {
        $subs[] = asn1.retag("context", 0, oct($initial));
    }
    for (def a in $anyParts) {
        $subs[] = asn1.retag("context", 1, oct($a));
    }
    if ($final != "") {
        $subs[] = asn1.retag("context", 2, oct($final));
    }
    return asn1.retag("context", 4, asn1.sequence([oct($attr), asn1.sequence($subs)]));
}

/**
 * allOf builds a conjunction: (&(f1)(f2)...).
 * @param filters {list of asn1.Value} the sub-filters that must all match
 * @return {asn1.Value} the combined filter, for search
 */
export func allOf(filters as list of asn1.Value) {
    return asn1.retag("context", 0, asn1.sequence($filters));
}

/**
 * anyOf builds a disjunction: (|(f1)(f2)...).
 * @param filters {list of asn1.Value} the sub-filters, any of which may match
 * @return {asn1.Value} the combined filter, for search
 */
export func anyOf(filters as list of asn1.Value) {
    return asn1.retag("context", 1, asn1.sequence($filters));
}

/**
 * negate builds a negation: (!(f)).
 * @param f {asn1.Value} the sub-filter to invert
 * @return {asn1.Value} the negated filter, for search
 */
export func negate(f as asn1.Value) {
    return asn1.tagged("context", 2, $f);
}

# --- RFC 4515 string filter parser ---

/**
 * parseFilter compiles an RFC 4515 filter string (e.g. "(&(a=1)(b=2))") into a filter value.
 * @param s {string} the RFC 4515 filter text
 * @return {asn1.Value} the compiled filter, for search
 * @throws {Error} on a malformed filter string
 */
export func parseFilter(s as string) {
    def r as FilterAt init parseFilterAt($s, 0);
    return $r.filter;
}

func charAt(s as string, i as int) {
    if ($i >= len($s)) {
        return "";
    }
    return strings.substring($s, $i, $i + 1);
}

func indexFrom(s as string, sub as string, from as int) {
    def rest as string init strings.substring($s, $from, len($s));
    def idx as int init strings.indexOf($rest, $sub);
    if ($idx < 0) {
        return -1;
    }
    return $from + $idx;
}

func parseFilterAt(s as string, pos as int) {
    if (charAt($s, $pos) != "(") {
        fail("filter must start with '(' at position " + convert.toString($pos));
    }
    def i as int init $pos + 1;
    def c as string init charAt($s, $i);
    if ($c == "&" or $c == "|") {
        def subs as list of asn1.Value;
        $i = $i + 1;
        while (charAt($s, $i) == "(") {
            def r as FilterAt init parseFilterAt($s, $i);
            $subs[] = $r.filter;
            $i = $r.pos;
        }
        if (charAt($s, $i) != ")") {
            fail("unterminated filter group");
        }
        $i = $i + 1;
        if ($c == "&") {
            return FilterAt{filter: allOf($subs), pos: $i};
        }
        return FilterAt{filter: anyOf($subs), pos: $i};
    }
    if ($c == "!") {
        def r as FilterAt init parseFilterAt($s, $i + 1);
        if (charAt($s, $r.pos) != ")") {
            fail("unterminated NOT filter");
        }
        return FilterAt{filter: negate($r.filter), pos: $r.pos + 1};
    }
    def close as int init indexFrom($s, ")", $i);
    if ($close < 0) {
        fail("unterminated filter item");
    }
    def item as string init strings.substring($s, $i, $close);
    return FilterAt{filter: parseItem($item), pos: $close + 1};
}

func parseItem(item as string) {
    def eq as int init strings.indexOf($item, "=");
    if ($eq < 0) {
        fail("filter item missing '=': " + $item);
    }
    def prev as string init "";
    if ($eq > 0) {
        $prev = strings.substring($item, $eq - 1, $eq);
    }
    def attr as string;
    def op as string;
    def value as string;
    if ($prev == ">" or $prev == "<" or $prev == "~") {
        $attr = strings.substring($item, 0, $eq - 1);
        $op = $prev;
        $value = strings.substring($item, $eq + 1, len($item));
    } else {
        $attr = strings.substring($item, 0, $eq);
        $op = "=";
        $value = strings.substring($item, $eq + 1, len($item));
    }
    if ($op == ">") {
        return greaterOrEqual($attr, $value);
    }
    if ($op == "<") {
        return lessOrEqual($attr, $value);
    }
    if ($op == "~") {
        return approx($attr, $value);
    }
    if ($value == "*") {
        return present($attr);
    }
    if (strings.contains($value, "*")) {
        return parseSubstrings($attr, $value);
    }
    return equals($attr, $value);
}

func parseSubstrings(attr as string, value as string) {
    def parts as list of string init strings.split($value, "*");
    def n as int init len($parts);
    def initial as string init $parts[0];
    def final as string init $parts[$n - 1];
    def anyParts as list of string;
    def k as int init 1;
    while ($k < $n - 1) {
        if ($parts[$k] != "") {
            $anyParts[] = $parts[$k];
        }
        $k = $k + 1;
    }
    return substrings($attr, $initial, $anyParts, $final);
}

# --- message framing (shared by client and server) ---

func encodeMessage(id as int, op as asn1.Value, controls as list of asn1.Value) {
    def elems as list of asn1.Value init [asn1.integer($id), $op];
    if (len($controls) > 0) {
        $elems[] = asn1.retag("context", 0, asn1.sequence($controls));
    }
    return asn1.encode(asn1.sequence($elems));
}

# readMessage reads one complete BER LDAPMessage off the stream (tag + length +
# content) and decodes it. A client conn (timeoutMs > 0) bounds each read; a
# server conn (0) blocks for the next request.
func readMessage(conn as Conn) {
    if ($conn.timeoutMs > 0) {
        net.setDeadline($conn.handle, $conn.timeoutMs);
    }
    def header as bytes init net.readN($conn.handle, 2);
    def lenByte as int init $header[1];
    def contentLen as int init 0;
    def full as bytes init $header;
    if ($lenByte >= 128) {
        def num as int init $lenByte - 128;
        if ($num == 0 or $num > 4) {
            fail("invalid BER length");
        }
        def extra as bytes init net.readN($conn.handle, $num);
        def bi as int init 0;
        while ($bi < $num) {
            $contentLen = $contentLen * 256 + $extra[$bi];
            $bi = $bi + 1;
        }
        $full = binary.concat($full, $extra);
    } else {
        $contentLen = $lenByte;
    }
    if ($contentLen > MAX_MSG_BYTES) {
        fail("message too large: " + convert.toString($contentLen) + " bytes (limit " +
            convert.toString(MAX_MSG_BYTES) + ")");
    }
    if ($contentLen > 0) {
        $full = binary.concat($full, net.readN($conn.handle, $contentLen));
    }
    return asn1.decode($full);
}

func sendOp(conn as Conn, id as int, op as asn1.Value, controls as list of asn1.Value) {
    net.setDeadline($conn.handle, $conn.timeoutMs);
    net.writeBytes($conn.handle, encodeMessage($id, $op, $controls));
}

func checkId(resp as asn1.Value, id as int) {
    if (asn1.asInt($resp, "/0") != $id) {
        fail("response messageID mismatch");
    }
}

func parseResult(protoOp as asn1.Value) {
    return Result{
        code: asn1.asInt($protoOp, "/0"),
        matchedDn: asn1.asString($protoOp, "/1"),
        message: asn1.asString($protoOp, "/2")
    };
}

func resultText(r as Result) {
    return "code " + convert.toString($r.code) + " (" + $r.message + ")";
}

# --- client: connect / TLS / close ---

/**
 * connect opens an LDAP connection. security selects the transport:
 * transport.Security.None is plaintext (port defaults to 389),
 * transport.Security.Tls is LDAPS / implicit TLS (port defaults to 636), and
 * transport.Security.Starttls connects in plaintext then upgrades in-band via
 * the StartTLS extended operation.
 * @param address {string} the server "host:port" (port defaults to 389, or 636 for Tls)
 * @param security {transport.Security} the transport: None, Tls, or Starttls
 * @return {Conn} the open connection
 * @throws {Error} on a connection or StartTLS failure
 */
export func connect(address as string, security as transport.Security) {
    if ($security == transport.Security.Tls) {
        return Conn{
            handle: net.connectTLS(ensurePort($address, DEFAULT_TLS_PORT)),
            timeoutMs: DEFAULT_TIMEOUT
        };
    }
    def c as Conn init Conn{
        handle: net.connect(ensurePort($address, DEFAULT_PORT)),
        timeoutMs: DEFAULT_TIMEOUT
    };
    if ($security == transport.Security.Starttls) {
        return startTls($c);
    }
    return $c;
}

/**
 * startTls upgrades a plaintext connection to TLS via the StartTLS extended
 * operation, then hands the same handle back (now encrypted).
 * @param conn {Conn} an open plaintext connection from connect
 * @return {Conn} the same connection, now encrypted
 * @throws {Error} if the server refuses StartTLS
 */
export func startTls(conn as Conn) {
    def id as int init nextId();
    def op as asn1.Value init asn1.retag(
        "application",
        APP_EXT_REQ,
        asn1.sequence([asn1.retag("context", 0, oct(STARTTLS_OID))]));
    sendOp($conn, $id, $op, []);
    def resp as asn1.Value init readMessage($conn);
    def code as int init asn1.asInt(asn1.get($resp, "/1"), "/0");
    if ($code != SUCCESS) {
        fail("StartTLS refused (code " + convert.toString($code) + ")");
    }
    net.startTLS($conn.handle);
    return $conn;
}

/**
 * close closes the connection without sending an unbind.
 * @param conn {Conn} the connection to close
 */
export func close(conn as Conn) {
    net.close($conn.handle);
}

/**
 * unbind sends an unbind request and closes the connection.
 * @param conn {Conn} the connection to unbind and close
 */
export func unbind(conn as Conn) {
    def id as int init nextId();
    net.setDeadline($conn.handle, $conn.timeoutMs);
    net.writeBytes(
        $conn.handle,
        encodeMessage($id, asn1.retag("application", APP_UNBIND, asn1.null()), []));
    net.close($conn.handle);
}

# --- client: bind ---

/**
 * bind performs a simple bind and returns the Result (check .code against
 * ldap.SUCCESS / ldap.INVALID_CREDENTIALS). An empty dn and password is an
 * anonymous bind.
 * @param conn {Conn} an open connection
 * @param dn {string} the bind DN ("" for anonymous)
 * @param password {string} the password ("" for anonymous)
 * @return {Result} the bind result (check .code)
 * @throws {Error} on a transport or protocol error
 */
export func bind(conn as Conn, dn as string, password as string) {
    def id as int init nextId();
    sendOp($conn, $id, encodeBindRequest($dn, $password), []);
    def resp as asn1.Value init readMessage($conn);
    checkId($resp, $id);
    return parseResult(asn1.get($resp, "/1"));
}

func encodeBindRequest(dn as string, password as string) {
    return asn1.retag(
        "application",
        APP_BIND_REQ,
        asn1.sequence([
            asn1.integer(LDAP_VERSION),
            oct($dn),
            asn1.retag("context", 0, oct($password))
        ]));
}

/**
 * bindSasl performs a SASL SCRAM bind (algo "sha1" -> SCRAM-SHA-1, "sha256" ->
 * SCRAM-SHA-256), verifying the server signature. Returns the Result on success
 * or throws on a protocol / verification failure.
 * @param conn {Conn} an open connection
 * @param user {string} the SCRAM authentication username
 * @param password {string} the user's password
 * @param algo {string} the SCRAM hash: "sha1" or "sha256"
 * @return {Result} the bind result on success
 * @throws {Error} on a protocol or server-signature-verification failure
 */
export func bindSasl(conn as Conn, user as string, password as string, algo as string) {
    def mech as string init "SCRAM-SHA-256";
    if (strings.lower($algo) == "sha1") {
        $mech = "SCRAM-SHA-1";
    }
    def s as sasl.Scram init sasl.scramStart($user, $algo);
    def id1 as int init nextId();
    sendOp(
        $conn,
        $id1,
        encodeSaslBind($mech, convert.bytesFromString(sasl.scramClientFirst($s), "utf-8")),
        []);
    def resp1 as asn1.Value init readMessage($conn);
    checkId($resp1, $id1);
    def op1 as asn1.Value init asn1.get($resp1, "/1");
    def r1 as Result init parseResult($op1);
    if ($r1.code != R_SASL_BIND_IN_PROGRESS) {
        fail("SASL bind step 1 failed: " + resultText($r1));
    }
    def serverFirst as string init convert.stringFromBytes(serverSaslCreds($op1), "utf-8");
    $s = sasl.scramClientFinal($s, $serverFirst, $password);
    def id2 as int init nextId();
    sendOp(
        $conn,
        $id2,
        encodeSaslBind($mech, convert.bytesFromString(sasl.scramFinalToken($s), "utf-8")),
        []);
    def resp2 as asn1.Value init readMessage($conn);
    checkId($resp2, $id2);
    def op2 as asn1.Value init asn1.get($resp2, "/1");
    def r2 as Result init parseResult($op2);
    if ($r2.code != SUCCESS) {
        fail("SASL bind failed: " + resultText($r2));
    }
    if (not sasl.scramVerify($s, convert.stringFromBytes(serverSaslCreds($op2), "utf-8"))) {
        fail("SASL server signature verification failed");
    }
    return $r2;
}

func encodeSaslBind(mech as string, creds as bytes) {
    return asn1.retag(
        "application",
        APP_BIND_REQ,
        asn1.sequence([
            asn1.integer(LDAP_VERSION),
            oct(""),
            asn1.retag("context", 3, asn1.sequence([oct($mech), asn1.octetString($creds)]))
        ]));
}

func serverSaslCreds(op as asn1.Value) {
    def n as int init asn1.length($op);
    def i as int init 3;
    while ($i < $n) {
        def c as asn1.Value init asn1.get($op, "/" + convert.toString($i));
        if (asn1.tagClass($c) == "context" and asn1.tagNumber($c) == 7) {
            return asn1.asBytes($c);
        }
        $i = $i + 1;
    }
    return emptyBytes();
}

# --- client: search ---

/**
 * search runs a search and returns the matching entries. scope is one of
 * ldap.SCOPE_BASE / SCOPE_ONE / SCOPE_SUB; filter comes from parseFilter or the
 * filter constructors; attributes is the list to return (empty = all user attributes).
 * @param conn {Conn} an open, bound connection
 * @param baseDn {string} the search base DN
 * @param scope {int} ldap.SCOPE_BASE, SCOPE_ONE, or SCOPE_SUB
 * @param filter {asn1.Value} the filter, from parseFilter or a filter constructor
 * @param attributes {list of string} the attributes to return (empty = all user attributes)
 * @return {list of Entry} the matching entries
 * @throws {Error} on a transport or protocol error
 */
export func search(
    conn as Conn,
    baseDn as string,
    scope as int,
    filter as asn1.Value,
    attributes as list of string) {
    def id as int init nextId();
    sendOp($conn, $id, encodeSearchRequest($baseDn, $scope, $filter, $attributes), []);
    def state as PagedState init readEntries($conn, $id);
    return $state.entries;
}

/**
 * searchPaged runs a search in pages of pageSize via the simple-paged-results control.
 * @param conn {Conn} an open, bound connection
 * @param baseDn {string} the search base DN
 * @param scope {int} ldap.SCOPE_BASE, SCOPE_ONE, or SCOPE_SUB
 * @param filter {asn1.Value} the filter, from parseFilter or a filter constructor
 * @param attributes {list of string} the attributes to return (empty = all user attributes)
 * @param pageSize {int} the maximum entries per page
 * @return {list of Entry} all matching entries across every page
 * @throws {Error} on a transport or protocol error
 */
export func searchPaged(
    conn as Conn,
    baseDn as string,
    scope as int,
    filter as asn1.Value,
    attributes as list of string,
    pageSize as int) {
    def all as list of Entry;
    def cookie as bytes init emptyBytes();
    def more as bool init true;
    while ($more) {
        def id as int init nextId();
        sendOp(
            $conn,
            $id,
            encodeSearchRequest($baseDn, $scope, $filter, $attributes),
            [pagedControl($pageSize, $cookie)]);
        def state as PagedState init readEntries($conn, $id);
        for (def e in $state.entries) {
            $all[] = $e;
        }
        $cookie = $state.cookie;
        if (len($cookie) == 0) {
            $more = false;
        }
    }
    return $all;
}

func encodeSearchRequest(
    baseDn as string,
    scope as int,
    filter as asn1.Value,
    attributes as list of string) {
    def attrElems as list of asn1.Value;
    for (def a in $attributes) {
        $attrElems[] = oct($a);
    }
    return asn1.retag(
        "application",
        APP_SEARCH_REQ,
        asn1.sequence([
            oct($baseDn),
            asn1.enumerated($scope),
            asn1.enumerated(DEREF_NEVER),
            asn1.integer(0),
            asn1.integer(0),
            asn1.boolean(false),
            $filter,
            asn1.sequence($attrElems)
        ]));
}

func readEntries(conn as Conn, id as int) {
    def entries as list of Entry;
    def cookie as bytes init emptyBytes();
    def done as bool init false;
    while (not $done) {
        def resp as asn1.Value init readMessage($conn);
        checkId($resp, $id);
        def protoOp as asn1.Value init asn1.get($resp, "/1");
        def tag as int init asn1.tagNumber($protoOp);
        if ($tag == APP_SEARCH_ENTRY) {
            $entries[] = parseEntry($protoOp);
        } elseif ($tag == APP_SEARCH_DONE) {
            def r as Result init parseResult($protoOp);
            if ($r.code != SUCCESS) {
                fail("search failed: " + resultText($r));
            }
            $cookie = extractCookie($resp);
            $done = true;
        } elseif ($tag == APP_SEARCH_REF) {
            # a SearchResultReference (a continuation referral) is ignored; keep reading.
        } else {
            fail("unexpected response tag " + convert.toString($tag));
        }
    }
    return PagedState{entries: $entries, cookie: $cookie};
}

func parseEntry(protoOp as asn1.Value) {
    def dn as string init asn1.asString($protoOp, "/0");
    def attrsNode as asn1.Value init asn1.get($protoOp, "/1");
    def attrs as list of Attribute;
    def n as int init asn1.length($attrsNode);
    def i as int init 0;
    while ($i < $n) {
        def a as asn1.Value init asn1.get($attrsNode, "/" + convert.toString($i));
        def valsNode as asn1.Value init asn1.get($a, "/1");
        def vals as list of string;
        def m as int init asn1.length($valsNode);
        def j as int init 0;
        while ($j < $m) {
            $vals[] = valueString($valsNode, "/" + convert.toString($j));
            $j = $j + 1;
        }
        $attrs[] = Attribute{name: asn1.asString($a, "/0"), values: $vals};
        $i = $i + 1;
    }
    return Entry{dn: $dn, attributes: $attrs};
}

# valueString decodes an attribute value. LDAP values are octets: a UTF-8 value
# comes back as its text, but a binary value (Active Directory objectGUID /
# objectSid, userCertificate, jpegPhoto) is not valid UTF-8, so it is returned
# base64-encoded (the LDIF convention) instead of throwing. Decode such a value
# with encoding.fromText(value, "base64").
func valueString(node as asn1.Value, ptr as string) {
    def raw as bytes init asn1.asBytes($node, $ptr);
    try {
        return convert.stringFromBytes($raw, "utf-8");
    } catch (e) {
        return encoding.toText($raw, "base64");
    }
}

func pagedControl(size as int, cookie as bytes) {
    def value as asn1.Value init asn1.sequence([asn1.integer($size), asn1.octetString($cookie)]);
    def cv as bytes init asn1.encode($value);
    return asn1.sequence([oct(PAGED_OID), asn1.boolean(false), asn1.octetString($cv)]);
}

func extractCookie(resp as asn1.Value) {
    if (asn1.length($resp) < 3) {
        return emptyBytes();
    }
    def controls as asn1.Value init asn1.get($resp, "/2");
    if (asn1.tagClass($controls) != "context" or asn1.tagNumber($controls) != 0) {
        return emptyBytes();
    }
    def cn as int init asn1.length($controls);
    def i as int init 0;
    while ($i < $cn) {
        def ctrl as asn1.Value init asn1.get($controls, "/" + convert.toString($i));
        if (asn1.asString($ctrl, "/0") == PAGED_OID) {
            def cv as bytes init asn1.asBytes(
                $ctrl,
                "/" + convert.toString(asn1.length($ctrl) - 1));
            return asn1.asBytes(asn1.decode($cv), "/1");
        }
        $i = $i + 1;
    }
    return emptyBytes();
}

# --- client: write operations ---

func doWrite(conn as Conn, op as asn1.Value) {
    def id as int init nextId();
    sendOp($conn, $id, $op, []);
    def resp as asn1.Value init readMessage($conn);
    checkId($resp, $id);
    return parseResult(asn1.get($resp, "/1"));
}

/**
 * add creates an entry from a DN and an attribute map, returning the Result (check .code).
 * @param conn {Conn} an open, bound connection
 * @param dn {string} the DN of the entry to create
 * @param attrs {map of string to list of string} the entry's attributes (name -> values)
 * @return {Result} the operation result (check .code)
 * @throws {Error} on a transport or protocol error
 */
export func add(conn as Conn, dn as string, attrs as map of string to list of string) {
    return doWrite($conn, encodeAddRequest($dn, $attrs));
}

func encodeAddRequest(dn as string, attrs as map of string to list of string) {
    def attrElems as list of asn1.Value;
    for (def k in $attrs) {
        def valElems as list of asn1.Value;
        for (def v in $attrs[$k]) {
            $valElems[] = oct($v);
        }
        $attrElems[] = asn1.sequence([oct($k), asn1.set($valElems)]);
    }
    return asn1.retag("application", 8, asn1.sequence([oct($dn), asn1.sequence($attrElems)]));
}

/**
 * change builds one modify change (operation is ldap.MOD_ADD / MOD_DELETE / MOD_REPLACE).
 * @param operation {int} ldap.MOD_ADD, MOD_DELETE, or MOD_REPLACE
 * @param name {string} the attribute to change
 * @param values {list of string} the values for the change (empty deletes the attribute)
 * @return {Change} the change, for modify
 */
export func change(operation as int, name as string, values as list of string) {
    return Change{operation: $operation, name: $name, values: $values};
}

/**
 * modify applies a list of changes to an entry, returning the Result.
 * @param conn {Conn} an open, bound connection
 * @param dn {string} the DN of the entry to modify
 * @param changes {list of Change} the changes to apply, from change
 * @return {Result} the operation result (check .code)
 * @throws {Error} on a transport or protocol error
 */
export func modify(conn as Conn, dn as string, changes as list of Change) {
    return doWrite($conn, encodeModifyRequest($dn, $changes));
}

func encodeModifyRequest(dn as string, changes as list of Change) {
    def changeElems as list of asn1.Value;
    for (def ch in $changes) {
        def valElems as list of asn1.Value;
        for (def v in $ch.values) {
            $valElems[] = oct($v);
        }
        def modification as asn1.Value init asn1.sequence([oct($ch.name), asn1.set($valElems)]);
        $changeElems[] = asn1.sequence([asn1.enumerated($ch.operation), $modification]);
    }
    return asn1.retag("application", 6, asn1.sequence([oct($dn), asn1.sequence($changeElems)]));
}

/**
 * delete removes an entry by DN, returning the Result.
 * @param conn {Conn} an open, bound connection
 * @param dn {string} the DN of the entry to remove
 * @return {Result} the operation result (check .code)
 * @throws {Error} on a transport or protocol error
 */
export func delete(conn as Conn, dn as string) {
    return doWrite($conn, encodeDeleteRequest($dn));
}

func encodeDeleteRequest(dn as string) {
    return asn1.retag("application", 10, oct($dn));
}

/**
 * modifyDn renames or moves an entry: newRdn is the new relative DN,
 * deleteOldRdn drops the old RDN attribute, and newSuperior ("" to keep the
 * parent) moves the entry under a new parent DN.
 * @param conn {Conn} an open, bound connection
 * @param dn {string} the DN of the entry to rename or move
 * @param newRdn {string} the new relative DN (e.g. "cn=bob")
 * @param deleteOldRdn {bool} whether to drop the old RDN attribute value
 * @param newSuperior {string} the new parent DN, or "" to keep the current parent
 * @return {Result} the operation result (check .code)
 * @throws {Error} on a transport or protocol error
 */
export func modifyDn(
    conn as Conn,
    dn as string,
    newRdn as string,
    deleteOldRdn as bool,
    newSuperior as string) {
    return doWrite($conn, encodeModifyDnRequest($dn, $newRdn, $deleteOldRdn, $newSuperior));
}

func encodeModifyDnRequest(
    dn as string,
    newRdn as string,
    deleteOldRdn as bool,
    newSuperior as string) {
    def elems as list of asn1.Value init [oct($dn), oct($newRdn), asn1.boolean($deleteOldRdn)];
    if ($newSuperior != "") {
        $elems[] = asn1.retag("context", 0, oct($newSuperior));
    }
    return asn1.retag("application", 12, asn1.sequence($elems));
}

/**
 * passwordModify changes a password via the RFC 3062 extended operation. Pass
 * userDn "" for the currently-bound user, oldPassword "" to skip the old-password
 * check (an administrative reset), and newPassword "" to have the server generate
 * one. Returns the Result.
 * @param conn {Conn} an open, bound connection
 * @param userDn {string} the target user DN, or "" for the currently-bound user
 * @param oldPassword {string} the current password, or "" to skip the check (admin reset)
 * @param newPassword {string} the new password, or "" to let the server generate one
 * @return {Result} the operation result (check .code)
 * @throws {Error} on a transport or protocol error
 */
export func passwordModify(
    conn as Conn,
    userDn as string,
    oldPassword as string,
    newPassword as string) {
    return doWrite($conn, encodePasswordModifyRequest($userDn, $oldPassword, $newPassword));
}

func encodePasswordModifyRequest(userDn as string, oldPassword as string, newPassword as string) {
    def parts as list of asn1.Value;
    if ($userDn != "") {
        $parts[] = asn1.retag("context", 0, oct($userDn));
    }
    if ($oldPassword != "") {
        $parts[] = asn1.retag("context", 1, oct($oldPassword));
    }
    if ($newPassword != "") {
        $parts[] = asn1.retag("context", 2, oct($newPassword));
    }
    def reqValue as bytes init asn1.encode(asn1.sequence($parts));
    return asn1.retag(
        "application",
        APP_EXT_REQ,
        asn1.sequence([
            asn1.retag("context", 0, oct(PASSWD_MODIFY_OID)),
            asn1.retag("context", 1, asn1.octetString($reqValue))
        ]));
}

# --- server: directory construction ---

/**
 * entry builds a directory entry from a DN and an attribute map (name -> values).
 * @param dn {string} the entry's distinguished name
 * @param attrs {map of string to list of string} the entry's attributes (name -> values)
 * @return {Entry} the constructed entry
 */
export func entry(dn as string, attrs as map of string to list of string) {
    def out as list of Attribute;
    for (def k in $attrs) {
        $out[] = Attribute{name: $k, values: $attrs[$k]};
    }
    return Entry{dn: $dn, attributes: $out};
}

/**
 * group builds a groupOfNames entry (objectClass + cn from the DN's RDN + member) from members.
 * @param dn {string} the group's distinguished name (its RDN value becomes cn)
 * @param members {list of string} the member DNs
 * @return {Entry} the constructed groupOfNames entry
 */
export func group(dn as string, members as list of string) {
    def attrs as map of string to list of string;
    def oc as list of string init ["groupOfNames"];
    $attrs["objectClass"] = $oc;
    def cn as string init rdnValue($dn);
    if ($cn != "") {
        $attrs["cn"] = [$cn];
    }
    $attrs["member"] = $members;
    return entry($dn, $attrs);
}

# rdnValue extracts the first RDN's value from a DN ("cn=staff,ou=groups" -> "staff").
func rdnValue(dn as string) {
    def firstRdn as string init $dn;
    def comma as int init strings.indexOf($dn, ",");
    if ($comma >= 0) {
        $firstRdn = strings.substring($dn, 0, $comma);
    }
    def eq as int init strings.indexOf($firstRdn, "=");
    if ($eq < 0) {
        return "";
    }
    return strings.substring($firstRdn, $eq + 1, len($firstRdn));
}

/**
 * directory builds a mutable in-memory directory the server answers for - the
 * lightweight, ephemeral default. Use openDirectory for one that persists.
 * @param entries {list of Entry} the initial entries to seed the directory with
 * @return {Directory} the in-memory directory
 */
export func directory(entries as list of Entry) {
    def s as kv.Store init kv.open();
    kv.set($s, DIR_KEY, json.encode($entries), 0);
    return Directory{store: $s};
}

/**
 * openDirectory opens a file-backed directory that persists across restarts (a
 * kv.openFile store at path). A new file starts empty; an existing one restores
 * its entries. Seed a fresh store on first run by checking listEntries and
 * addEntry-ing when it is empty. Edits (addEntry / modifyEntry / ...) persist.
 * @param path {string} the backing file path for the kv store
 * @return {Directory} the file-backed directory
 */
export func openDirectory(path as string) {
    def s as kv.Store init kv.openFile($path);
    if (not kv.has($s, DIR_KEY)) {
        def empty as list of Entry;
        kv.set($s, DIR_KEY, json.encode($empty), 0);
    }
    return Directory{store: $s};
}

# loadEntries / saveEntries move the whole entry set through the shared store.
func loadEntries(dir as Directory) {
    return decodeEntries(kv.get($dir.store, DIR_KEY));
}

func saveEntries(dir as Directory, entries as list of Entry) {
    kv.set($dir.store, DIR_KEY, json.encode($entries), 0);
}

func decodeEntries(s as string) {
    def root as json.Value init json.decode($s);
    def n as int init json.length($root);
    def out as list of Entry;
    def i as int init 0;
    while ($i < $n) {
        def ptr as string init "/" + convert.toString($i);
        def an as int init json.length(json.get($root, $ptr + "/attributes"));
        def attrs as list of Attribute;
        def j as int init 0;
        while ($j < $an) {
            def ap as string init $ptr + "/attributes/" + convert.toString($j);
            def vn as int init json.length(json.get($root, $ap + "/values"));
            def vals as list of string;
            def k as int init 0;
            while ($k < $vn) {
                $vals[] = json.asString($root, $ap + "/values/" + convert.toString($k));
                $k = $k + 1;
            }
            $attrs[] = Attribute{name: json.asString($root, $ap + "/name"), values: $vals};
            $j = $j + 1;
        }
        $out[] = Entry{dn: json.asString($root, $ptr + "/dn"), attributes: $attrs};
        $i = $i + 1;
    }
    return $out;
}

/**
 * listEntries returns a snapshot of every entry currently in the directory.
 * @param dir {Directory} the directory to read
 * @return {list of Entry} a snapshot of all current entries
 */
export func listEntries(dir as Directory) {
    return loadEntries($dir);
}

/**
 * getEntry returns the entry with the given DN, or an empty Entry (dn == "") if absent.
 * @param dir {Directory} the directory to read
 * @param dn {string} the DN to look up (matched case-insensitively)
 * @return {Entry} the matching entry, or an empty Entry (dn == "") if absent
 */
export func getEntry(dir as Directory, dn as string) {
    return findEntry($dir, $dn);
}

/**
 * hasEntry reports whether an entry with the given DN exists.
 * @param dir {Directory} the directory to read
 * @param dn {string} the DN to test (matched case-insensitively)
 * @return {bool} true if an entry with that DN exists
 */
export func hasEntry(dir as Directory, dn as string) {
    return findEntry($dir, $dn).dn != "";
}

/**
 * addEntry inserts an entry, replacing any existing entry with the same DN.
 * @param dir {Directory} the directory to modify
 * @param e {Entry} the entry to insert (or replace a same-DN entry with)
 */
export func addEntry(dir as Directory, e as Entry) {
    def entries as list of Entry init loadEntries($dir);
    def out as list of Entry;
    def target as string init normDn($e.dn);
    def replaced as bool init false;
    for (def x in $entries) {
        if (normDn($x.dn) == $target) {
            $out[] = $e;
            $replaced = true;
        } else {
            $out[] = $x;
        }
    }
    if (not $replaced) {
        $out[] = $e;
    }
    saveEntries($dir, $out);
}

/**
 * modifyEntry replaces the entry with the same DN (an alias for addEntry).
 * @param dir {Directory} the directory to modify
 * @param e {Entry} the replacement entry, keyed by its DN
 */
export func modifyEntry(dir as Directory, e as Entry) {
    addEntry($dir, $e);
}

/**
 * deleteEntry removes the entry with the given DN if present.
 * @param dir {Directory} the directory to modify
 * @param dn {string} the DN of the entry to remove (matched case-insensitively)
 */
export func deleteEntry(dir as Directory, dn as string) {
    def entries as list of Entry init loadEntries($dir);
    def out as list of Entry;
    def target as string init normDn($dn);
    for (def x in $entries) {
        if (normDn($x.dn) != $target) {
            $out[] = $x;
        }
    }
    saveEntries($dir, $out);
}

/**
 * setAttribute creates or replaces one attribute's values on an existing entry.
 * @param dir {Directory} the directory to modify
 * @param dn {string} the DN of the entry to update
 * @param name {string} the attribute to create or replace
 * @param vals {list of string} the new values for the attribute
 * @throws {Error} if no entry with dn exists
 */
export func setAttribute(dir as Directory, dn as string, name as string, vals as list of string) {
    def e as Entry init findEntry($dir, $dn);
    if ($e.dn == "") {
        fail("setAttribute: no entry " + $dn);
    }
    def attrs as list of Attribute;
    def found as bool init false;
    for (def a in $e.attributes) {
        if (strings.lower($a.name) == strings.lower($name)) {
            $attrs[] = Attribute{name: $name, values: $vals};
            $found = true;
        } else {
            $attrs[] = $a;
        }
    }
    if (not $found) {
        $attrs[] = Attribute{name: $name, values: $vals};
    }
    modifyEntry($dir, Entry{dn: $e.dn, attributes: $attrs});
}

/**
 * password hashes a plaintext password into an LDAP userPassword value. scheme
 * is "plain", "sha", "sha256", "ssha", "ssha256" (salted variants use a random
 * 8-byte salt), "pbkdf2" (PBKDF2-SHA256), or "pbkdf2-sha512". Store the result
 * as an entry's userPassword.
 * @param plain {string} the plaintext password to hash
 * @param scheme {string} one of plain / sha / sha256 / ssha / ssha256 / pbkdf2 / pbkdf2-sha512
 * @return {string} the encoded userPassword value (with its scheme prefix)
 * @throws {Error} on an unknown scheme
 */
export func password(plain as string, scheme as string) {
    def s as string init strings.lower($scheme);
    if ($s == "plain") {
        return $plain;
    }
    if ($s == "sha") {
        return '{SHA}' +
            encoding.toText(
            hash.compute(convert.bytesFromString($plain, "utf-8"), "sha1"),
            "base64");
    }
    if ($s == "sha256") {
        return '{SHA256}' +
            encoding.toText(
            hash.compute(convert.bytesFromString($plain, "utf-8"), "sha256"),
            "base64");
    }
    if ($s == "ssha") {
        def salt as bytes init crypto.randBytes(8);
        return '{SSHA}' +
            encoding.toText(
            binary.concat(
                hash.compute(
                    binary.concat(convert.bytesFromString($plain, "utf-8"), $salt),
                    "sha1"),
                $salt),
            "base64");
    }
    if ($s == "ssha256") {
        def salt as bytes init crypto.randBytes(8);
        return '{SSHA256}' +
            encoding.toText(
            binary.concat(
                hash.compute(
                    binary.concat(convert.bytesFromString($plain, "utf-8"), $salt),
                    "sha256"),
                $salt),
            "base64");
    }
    if ($s == "pbkdf2") {
        return pbkdf2Password($plain, "sha256", 32, "SHA256");
    }
    if ($s == "pbkdf2-sha512") {
        return pbkdf2Password($plain, "sha512", 64, "SHA512");
    }
    fail("unknown password scheme '" + $scheme +
        "' (use plain/sha/sha256/ssha/ssha256/pbkdf2/pbkdf2-sha512)");
}

# pbkdf2Password derives a slow-KDF userPassword value in the OpenLDAP-style
# {PBKDF2-<ALGO>}$iterations$salt$hash format (salt and hash base64-encoded).
func pbkdf2Password(plain as string, algo as string, keyLen as int, label as string) {
    def salt as bytes init crypto.randBytes(16);
    def dk as bytes init crypto.pbkdf2(
        convert.bytesFromString($plain, "utf-8"),
        $salt,
        PBKDF2_ITERATIONS,
        $keyLen,
        $algo);
    return '{PBKDF2-' + $label + '}$' + convert.toString(PBKDF2_ITERATIONS) +
        "$" + encoding.toText($salt, "base64") +
        "$" + encoding.toText($dk, "base64");
}

func verifyPassword(stored as string, supplied as string) {
    # Never throw on a malformed stored value (bad base64, a non-numeric pbkdf2
    # iterations field, ...): a bad hash is a failed verification, not a server
    # error that should unwind the request handler.
    try {
        return verifyPasswordInner($stored, $supplied);
    } catch (e) {
        return false;
    }
}

func verifyPasswordInner(stored as string, supplied as string) {
    if ($stored == "") {
        return false;
    }
    if (strings.startsWith($stored, '{SSHA256}')) {
        return checkSalted($stored, 9, "sha256", 32, $supplied);
    }
    if (strings.startsWith($stored, '{SSHA}')) {
        return checkSalted($stored, 6, "sha1", 20, $supplied);
    }
    if (strings.startsWith($stored, '{SHA256}')) {
        return checkSimple($stored, 8, "sha256", $supplied);
    }
    if (strings.startsWith($stored, '{SHA}')) {
        return checkSimple($stored, 5, "sha1", $supplied);
    }
    if (strings.startsWith($stored, '{PBKDF2-')) {
        return checkPbkdf2($stored, $supplied);
    }
    return crypto.hmacEqual(
        convert.bytesFromString($stored, "utf-8"),
        convert.bytesFromString($supplied, "utf-8"));
}

# checkPbkdf2 verifies a {PBKDF2-<ALGO>}$iterations$salt$hash value by re-deriving
# the key with the stored algorithm / iterations / salt and comparing constant-time.
func checkPbkdf2(stored as string, supplied as string) {
    def close as int init strings.indexOf($stored, '}');
    if ($close < 0) {
        return false;
    }
    def algo as string init pbkdf2Algo(strings.substring($stored, 8, $close));
    if ($algo == "") {
        return false;
    }
    def parts as list of string init strings.split(
        strings.substring($stored, $close + 1, len($stored)),
        "$");
    if (len($parts) != 4) {
        return false;
    }
    def iters as int init convert.toInt($parts[1]);
    if ($iters < 1 or $iters > MAX_PBKDF2_ITERATIONS) {
        return false;
    }
    def salt as bytes init encoding.fromText($parts[2], "base64");
    def want as bytes init encoding.fromText($parts[3], "base64");
    if (len($want) < 16 or len($want) > 128) {
        return false;
    }
    def dk as bytes init crypto.pbkdf2(
        convert.bytesFromString($supplied, "utf-8"),
        $salt,
        $iters,
        len($want),
        $algo);
    return crypto.hmacEqual($dk, $want);
}

# pbkdf2Algo maps a {PBKDF2-<label>} label to a crypto.pbkdf2 algorithm name.
func pbkdf2Algo(label as string) {
    def u as string init strings.upper($label);
    if ($u == "SHA256") {
        return "sha256";
    }
    if ($u == "SHA512") {
        return "sha512";
    }
    if ($u == "SHA1") {
        return "sha1";
    }
    return "";
}

func checkSalted(
    stored as string,
    prefixLen as int,
    algo as string,
    digestLen as int,
    supplied as string) {
    def raw as bytes init encoding.fromText(
        strings.substring($stored, $prefixLen, len($stored)),
        "base64");
    if (len($raw) < $digestLen) {
        return false;
    }
    def digest as bytes init binary.slice($raw, 0, $digestLen);
    def salt as bytes init binary.slice($raw, $digestLen, len($raw));
    def calc as bytes init hash.compute(
        binary.concat(convert.bytesFromString($supplied, "utf-8"), $salt),
        $algo);
    return crypto.hmacEqual($calc, $digest);
}

func checkSimple(stored as string, prefixLen as int, algo as string, supplied as string) {
    def digest as bytes init encoding.fromText(
        strings.substring($stored, $prefixLen, len($stored)),
        "base64");
    def calc as bytes init hash.compute(convert.bytesFromString($supplied, "utf-8"), $algo);
    return crypto.hmacEqual($calc, $digest);
}

# --- server: serve loop ---

/**
 * listen opens a listening socket for the server ("host:port"; port defaults to 389).
 * @param address {string} the listen address "host:port" (port defaults to 389)
 * @return {net.Listener} the open listener, for serveOn
 * @throws {Error} if the socket cannot be opened
 */
export func listen(address as string) {
    return net.listen(ensurePort($address, DEFAULT_PORT));
}

/**
 * serve binds address and serves dir until the process ends (blocks).
 * @param dir {Directory} the directory to answer queries from
 * @param address {string} the listen address "host:port" (port defaults to 389)
 * @throws {Error} if the socket cannot be opened
 */
export func serve(dir as Directory, address as string) {
    serveOn($dir, listen($address));
}

/**
 * serveOn serves dir on an already-open listener, one spawn per connection,
 * until the listener is closed (which ends the accept loop). Close the listener
 * from another task for a graceful stop.
 * @param dir {Directory} the directory to answer queries from
 * @param listener {net.Listener} an open listener from listen
 */
export func serveOn(dir as Directory, listener as net.Listener) {
    def running as bool init true;
    while ($running) {
        try {
            def conn as net.Conn init net.accept($listener);
            spawn {
                handleConn($dir, Conn{handle: $conn, timeoutMs: SERVER_IDLE_MS});
            };
        } catch (e) {
            $running = false;
        }
    }
}

func handleConn(dir as Directory, conn as Conn) {
    # defer guarantees the socket closes on every exit path - including a throw
    # unwinding out of a handler - so a faulting request cannot leak the conn.
    defer net.close($conn.handle);
    def open as bool init true;
    while ($open) {
        def req as asn1.Value;
        def gotMsg as bool init false;
        try {
            $req = readMessage($conn);
            $gotMsg = true;
        } catch (e) {
            $open = false;
        }
        if ($gotMsg) {
            # A faulting handler (malformed request, decode-bomb trip, ...) is
            # caught here: log it and close this connection, rather than
            # unwinding out of the spawn silently and leaking the socket.
            try {
                $open = dispatch($dir, $conn, $req);
            } catch (e) {
                io.eprintf("ldap: dropping connection after request error: %s\n", $e.message);
                $open = false;
            }
        }
    }
}

# dispatch handles one decoded request and returns whether to keep the
# connection open (false ends the session: unbind, an unknown op, or an error).
func dispatch(dir as Directory, conn as Conn, req as asn1.Value) {
    def id as int init asn1.asInt($req, "/0");
    def op as asn1.Value init asn1.get($req, "/1");
    def tag as int init asn1.tagNumber($op);
    if ($tag == APP_BIND_REQ) {
        net.writeBytes($conn.handle, respondBind($dir, $op, $id));
        return true;
    }
    if ($tag == APP_SEARCH_REQ) {
        respondSearch($dir, $op, $id, $conn);
        return true;
    }
    if ($tag == APP_EXT_REQ) {
        net.writeBytes($conn.handle, encodeExtResponse($id, R_PROTOCOL_ERROR));
        return true;
    }
    return false;
}

func respondBind(dir as Directory, op as asn1.Value, id as int) {
    def dn as string init asn1.asString($op, "/1");
    def auth as asn1.Value init asn1.get($op, "/2");
    if (asn1.tagNumber($auth) != 0) {
        return encodeBindResponse(
            $id,
            R_AUTH_METHOD_NOT_SUPPORTED,
            "",
            "only simple bind is supported");
    }
    def supplied as string init primStr($auth);
    if ($dn == "" and $supplied == "") {
        return encodeBindResponse($id, SUCCESS, "", "");
    }
    def e as Entry init findEntry($dir, $dn);
    if ($e.dn != "" and verifyPassword(firstValue($e, "userPassword"), $supplied)) {
        return encodeBindResponse($id, SUCCESS, $dn, "");
    }
    return encodeBindResponse($id, INVALID_CREDENTIALS, "", "invalid credentials");
}

func respondSearch(dir as Directory, op as asn1.Value, id as int, conn as Conn) {
    def baseDn as string init asn1.asString($op, "/0");
    def scope as int init asn1.asInt($op, "/1");
    def filter as asn1.Value init asn1.get($op, "/6");
    def reqAttrs as list of string init decodeAttrList($op);
    for (def e in loadEntries($dir)) {
        if (inScope($e.dn, $baseDn, $scope) and evalFilter($filter, $e)) {
            net.writeBytes($conn.handle, encodeSearchEntry($id, $e, $reqAttrs));
        }
    }
    net.writeBytes($conn.handle, encodeSearchDone($id, SUCCESS));
}

func decodeAttrList(op as asn1.Value) {
    def node as asn1.Value init asn1.get($op, "/7");
    def n as int init asn1.length($node);
    def out as list of string;
    def i as int init 0;
    while ($i < $n) {
        $out[] = asn1.asString($node, "/" + convert.toString($i));
        $i = $i + 1;
    }
    return $out;
}

func findEntry(dir as Directory, dn as string) {
    def target as string init normDn($dn);
    for (def e in loadEntries($dir)) {
        if (normDn($e.dn) == $target) {
            return $e;
        }
    }
    def empty as Entry;
    return $empty;
}

func inScope(dn as string, base as string, scope as int) {
    def d as string init normDn($dn);
    def b as string init normDn($base);
    if ($scope == SCOPE_BASE) {
        return $d == $b;
    }
    if ($scope == SCOPE_SUB) {
        if ($b == "") {
            return true;
        }
        return $d == $b or strings.endsWith($d, "," + $b);
    }
    if ($scope == SCOPE_ONE) {
        if ($b == "" or not strings.endsWith($d, "," + $b)) {
            return false;
        }
        def rdn as string init strings.substring($d, 0, len($d) - len($b) - 1);
        return not strings.contains($rdn, ",");
    }
    return false;
}

# --- server: filter evaluation ---

func evalFilter(f as asn1.Value, e as Entry) {
    if (asn1.tagClass($f) != "context") {
        return false;
    }
    def num as int init asn1.tagNumber($f);
    if ($num == 0) {
        def n as int init asn1.length($f);
        def i as int init 0;
        while ($i < $n) {
            if (not evalFilter(asn1.get($f, "/" + convert.toString($i)), $e)) {
                return false;
            }
            $i = $i + 1;
        }
        return true;
    }
    if ($num == 1) {
        def n as int init asn1.length($f);
        def i as int init 0;
        while ($i < $n) {
            if (evalFilter(asn1.get($f, "/" + convert.toString($i)), $e)) {
                return true;
            }
            $i = $i + 1;
        }
        return false;
    }
    if ($num == 2) {
        return not evalFilter(asn1.get($f, "/0"), $e);
    }
    if ($num == 3 or $num == 8) {
        return matchEquality($f, $e);
    }
    if ($num == 4) {
        return matchSubstrings($f, $e);
    }
    if ($num == 5) {
        return matchOrder($f, $e, true);
    }
    if ($num == 6) {
        return matchOrder($f, $e, false);
    }
    if ($num == 7) {
        return len(values($e, primStr($f))) > 0;
    }
    return false;
}

func matchEquality(f as asn1.Value, e as Entry) {
    def name as string init asn1.asString($f, "/0");
    def val as string init strings.lower(asn1.asString($f, "/1"));
    for (def v in values($e, $name)) {
        if (strings.lower($v) == $val) {
            return true;
        }
    }
    return false;
}

func matchOrder(f as asn1.Value, e as Entry, greater as bool) {
    def name as string init asn1.asString($f, "/0");
    def val as string init strings.lower(asn1.asString($f, "/1"));
    for (def v in values($e, $name)) {
        def cmp as int init compareStr(strings.lower($v), $val);
        if ($greater and $cmp >= 0) {
            return true;
        }
        if (not $greater and $cmp <= 0) {
            return true;
        }
    }
    return false;
}

func matchSubstrings(f as asn1.Value, e as Entry) {
    def name as string init asn1.asString($f, "/0");
    def comps as asn1.Value init asn1.get($f, "/1");
    def cn as int init asn1.length($comps);
    def initial as string init "";
    def final as string init "";
    def anyParts as list of string;
    def i as int init 0;
    while ($i < $cn) {
        def c as asn1.Value init asn1.get($comps, "/" + convert.toString($i));
        def t as int init asn1.tagNumber($c);
        if ($t == 0) {
            $initial = primStr($c);
        } elseif ($t == 1) {
            $anyParts[] = primStr($c);
        } elseif ($t == 2) {
            $final = primStr($c);
        }
        $i = $i + 1;
    }
    for (def v in values($e, $name)) {
        if (substrMatch(
            strings.lower($v),
            strings.lower($initial),
            $anyParts,
            strings.lower($final))) {
            return true;
        }
    }
    return false;
}

func substrMatch(v as string, initial as string, anyParts as list of string, final as string) {
    def rest as string init $v;
    if ($initial != "") {
        if (not strings.startsWith($rest, $initial)) {
            return false;
        }
        $rest = strings.substring($rest, len($initial), len($rest));
    }
    if ($final != "") {
        if (not strings.endsWith($rest, $final)) {
            return false;
        }
        $rest = strings.substring($rest, 0, len($rest) - len($final));
    }
    for (def a in $anyParts) {
        def al as string init strings.lower($a);
        def idx as int init strings.indexOf($rest, $al);
        if ($idx < 0) {
            return false;
        }
        $rest = strings.substring($rest, $idx + len($al), len($rest));
    }
    return true;
}

# --- server: response encoders ---

func encodeBindResponse(id as int, code as int, matchedDn as string, msg as string) {
    return encodeMessage(
        $id,
        asn1.retag(
            "application",
            APP_BIND_RESP,
            asn1.sequence([asn1.enumerated($code), oct($matchedDn), oct($msg)])),
        []);
}

func encodeSearchEntry(id as int, e as Entry, reqAttrs as list of string) {
    def attrElems as list of asn1.Value;
    for (def a in $e.attributes) {
        if (attrRequested($a.name, $reqAttrs)) {
            def valElems as list of asn1.Value;
            for (def v in $a.values) {
                $valElems[] = oct($v);
            }
            $attrElems[] = asn1.sequence([oct($a.name), asn1.set($valElems)]);
        }
    }
    return encodeMessage(
        $id,
        asn1.retag(
            "application",
            APP_SEARCH_ENTRY,
            asn1.sequence([oct($e.dn), asn1.sequence($attrElems)])),
        []);
}

func attrRequested(name as string, reqAttrs as list of string) {
    def isPw as bool init strings.lower($name) == "userpassword";
    if (len($reqAttrs) == 0) {
        return not $isPw;
    }
    for (def r in $reqAttrs) {
        def rl as string init strings.lower($r);
        if ($rl == "*" and not $isPw) {
            return true;
        }
        if ($rl == strings.lower($name)) {
            return true;
        }
    }
    return false;
}

func encodeSearchDone(id as int, code as int) {
    return encodeMessage(
        $id,
        asn1.retag(
            "application",
            APP_SEARCH_DONE,
            asn1.sequence([asn1.enumerated($code), oct(""), oct("")])),
        []);
}

func encodeExtResponse(id as int, code as int) {
    return encodeMessage(
        $id,
        asn1.retag(
            "application",
            APP_EXT_RESP,
            asn1.sequence([asn1.enumerated($code), oct(""), oct("")])),
        []);
}
