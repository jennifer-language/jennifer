# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
# pragma-jennifer-version: >=0.24.0
# pragma-jennifer-capability: net

/**
 * An SNMP v1 / v2c client and agent (RFC 1157 / RFC 3416), over UDP with
 * community-string authentication. The client queries an agent with GET /
 * GETNEXT / SET / walk; the agent (server) answers those for a MIB you supply -
 * a hardware simulator, or a way to expose an app's metrics over SNMP. The wire
 * messages are ASN.1 BER, built and parsed with the `asn1` library; the
 * transport is `net` UDP. No SNMPv3 / USM (that is the security model a later
 * tier would add).
 *
 * @module snmp
 * @example
 * import "snmp.j" as snmp;
 * use io;
 * def c as snmp.Client init snmp.client("127.0.0.1", "public");
 * def vbs as list of snmp.Varbind init snmp.get($c, ["1.3.6.1.2.1.1.1.0"]);
 * io.printf("%s = %s\n", $vbs[0].oid, $vbs[0].value);
 */
use asn1;
use net;
use channel;
use convert;
use encoding;
use strings;
use math;

# --- constants ---

/** SNMP version 1. */
export def const VERSION1 as int init 0;
/** SNMP version 2c. */
export def const VERSION2C as int init 1;

def const DEFAULT_PORT as int init 161;
def const DEFAULT_TIMEOUT as int init 2000;
def const DEFAULT_RETRIES as int init 1;

# PDU tags (context class, IMPLICIT).
def const PDU_GET as int init 0;
def const PDU_GETNEXT as int init 1;
def const PDU_RESPONSE as int init 2;
def const PDU_SET as int init 3;

def const MAX_WALK as int init 100000;

# How often the agent wakes from a blocked receive to check its shutdown channel.
def const SHUTDOWN_POLL_MS as int init 250;

# --- structs ---

/**
 * A configured SNMP agent endpoint.
 * @field address {string} the agent as "host:port"
 * @field community {string} the community string (the v1 / v2c credential)
 * @field version {int} the protocol version (snmp.VERSION1 or snmp.VERSION2C)
 * @field timeoutMs {int} the per-attempt receive timeout, in milliseconds
 * @field retries {int} the number of extra attempts after the first
 */
export def struct Client {
    address as string,
    community as string,
    version as int,
    timeoutMs as int,
    retries as int
};

/**
 * One variable binding: an OID and its value. On a returned binding, `type` is
 * the SNMP value type ("integer", "octetString", "oid", "null", "counter32",
 * "gauge32", "timeTicks", "ipAddress", "counter64", "opaque", or the exception
 * "noSuchObject" / "noSuchInstance" / "endOfMibView"); `value` is a string
 * rendering, and `number` holds the integer for a numeric type (0 otherwise).
 * @field oid {string} the object identifier, dotted
 * @field type {string} the value's SNMP type name
 * @field value {string} the value rendered as a string
 * @field number {int} the integer value for a numeric type (0 otherwise)
 */
export def struct Varbind {
    oid as string,
    type as string,
    value as string,
    number as int
};

/**
 * An SNMP agent (server): a community, a protocol version, and a MIB - the set
 * of bindings it answers for. Build one with snmp.agent and serve it with
 * snmp.serve.
 * @field community {string} the community string it accepts (others are dropped)
 * @field version {int} snmp.VERSION1 or snmp.VERSION2C
 * @field bindings {list of Varbind} the MIB, one binding per OID it serves
 */
export def struct Agent {
    community as string,
    version as int,
    bindings as list of Varbind
};

# The parsed decode of a message (request or response), module-private.
def struct Message {
    version as int,
    community as string,
    pduTag as int,
    requestId as int,
    errorStatus as int,
    errorIndex as int,
    varbinds as list of Varbind
};

# The result of handling one request: the reply bytes (empty = drop, send
# nothing) and the MIB after any SET mutation.
def struct ServeResult {
    reply as bytes,
    mib as list of Varbind
};

# --- errors (private) ---

func fail(msg as string) {
    throw Error{kind: "snmp", message: $msg, file: "", line: 0, col: 0};
}

# --- clients + varbind builders (exported) ---

/**
 * Construct a client for an agent, using UDP port 161, SNMP v2c, a 2s timeout,
 * and one retry.
 * @param host {string} the agent host or IP
 * @param community {string} the community string
 * @return {Client} the configured client
 */
export func client(host as string, community as string) {
    return clientWith($host + ":" + convert.toString(DEFAULT_PORT), $community, VERSION2C, DEFAULT_TIMEOUT, DEFAULT_RETRIES);
}

/**
 * Construct a client with full control over the endpoint and timing.
 * @param address {string} the agent as "host:port"
 * @param community {string} the community string
 * @param version {int} snmp.VERSION1 or snmp.VERSION2C
 * @param timeoutMs {int} the per-attempt receive timeout, in milliseconds
 * @param retries {int} the number of extra attempts after the first
 * @return {Client} the configured client
 */
export func clientWith(address as string, community as string, version as int, timeoutMs as int, retries as int) {
    return Client{address: $address, community: $community, version: $version, timeoutMs: $timeoutMs, retries: $retries};
}

/**
 * Build an integer-valued binding for snmp.set.
 * @param oid {string} the object identifier, dotted
 * @param n {int} the integer value
 * @return {Varbind} the binding
 */
export func intVar(oid as string, n as int) {
    return Varbind{oid: $oid, type: "integer", value: convert.toString($n), number: $n};
}

/**
 * Build an octet-string-valued binding for snmp.set.
 * @param oid {string} the object identifier, dotted
 * @param s {string} the string value
 * @return {Varbind} the binding
 */
export func stringVar(oid as string, s as string) {
    return Varbind{oid: $oid, type: "octetString", value: $s, number: 0};
}

/**
 * Build an OID-valued binding for snmp.set.
 * @param oid {string} the object identifier, dotted
 * @param target {string} the OID value, dotted
 * @return {Varbind} the binding
 */
export func oidVar(oid as string, target as string) {
    return Varbind{oid: $oid, type: "oid", value: $target, number: 0};
}

/**
 * Build a binding of any SNMP type (for a SET, or a MIB entry served by an
 * agent). `type` is a value type name (see Varbind); `number` is used for the
 * numeric types, `value` for the string / oid / ipAddress types.
 * @param oid {string} the object identifier, dotted
 * @param type {string} the SNMP value type name
 * @param value {string} the string / oid / ipAddress rendering
 * @param number {int} the integer for a numeric type
 * @return {Varbind} the binding
 */
export func varbind(oid as string, type as string, value as string, number as int) {
    return Varbind{oid: $oid, type: $type, value: $value, number: $number};
}

# --- requests (exported) ---

/**
 * GET the values of one or more OIDs.
 * @param c {Client} the agent client
 * @param oids {list of string} the OIDs to fetch
 * @return {list of Varbind} the returned bindings
 * @throws snmp if the agent reports an error, or does not answer
 */
export func get(c as Client, oids as list of string) {
    return request($c, PDU_GET, oidsToVarbinds($oids));
}

/**
 * GETNEXT: fetch the binding lexically following each OID (the walk primitive).
 * @param c {Client} the agent client
 * @param oids {list of string} the starting OIDs
 * @return {list of Varbind} the returned bindings
 * @throws snmp if the agent reports an error, or does not answer
 */
export func getNext(c as Client, oids as list of string) {
    return request($c, PDU_GETNEXT, oidsToVarbinds($oids));
}

/**
 * SET one or more bindings (build them with intVar / stringVar / oidVar).
 * @param c {Client} the agent client
 * @param varbinds {list of Varbind} the bindings to write
 * @return {list of Varbind} the agent's echoed bindings
 * @throws snmp if the agent reports an error, or does not answer
 */
export func set(c as Client, varbinds as list of Varbind) {
    return request($c, PDU_SET, $varbinds);
}

/**
 * Walk a subtree: repeated GETNEXT from rootOid until the returned OID leaves
 * the subtree or the agent signals endOfMibView.
 * @param c {Client} the agent client
 * @param rootOid {string} the subtree root OID, dotted
 * @return {list of Varbind} every binding under the subtree, in order
 * @throws snmp if the agent reports an error, does not answer, or does not advance
 */
export func walk(c as Client, rootOid as string) {
    def results as list of Varbind;
    def current as string init $rootOid;
    def guard as int init 0;
    while (true) {
        $guard = $guard + 1;
        if ($guard > MAX_WALK) {
            fail("walk exceeded " + convert.toString(MAX_WALK) + " steps (agent not advancing?)");
        }
        def vbs as list of Varbind init getNext($c, [$current]);
        if (len($vbs) == 0) {
            break;
        }
        def vb as Varbind init $vbs[0];
        if ($vb.type == "endOfMibView") {
            break;
        }
        if (not inSubtree($vb.oid, $rootOid)) {
            break;
        }
        # A well-behaved agent returns a strictly greater OID each step. If it
        # does not advance (a buggy or hostile agent), stop rather than loop.
        if (compareOid($vb.oid, $current) <= 0) {
            break;
        }
        $results[] = $vb;
        $current = $vb.oid;
    }
    return $results;
}

# --- agent / server (exported) ---

/**
 * Build an agent (server) that answers GET / GETNEXT / SET for a MIB. Build the
 * bindings with `varbind` / `intVar` / `stringVar` / `oidVar`; each is one OID
 * the agent serves.
 * @param community {string} the community string the agent accepts
 * @param version {int} snmp.VERSION1 or snmp.VERSION2C
 * @param bindings {list of Varbind} the MIB
 * @return {Agent} the configured agent
 */
export func agent(community as string, version as int, bindings as list of Varbind) {
    return Agent{community: $community, version: $version, bindings: $bindings};
}

/**
 * Bind `address` ("host:port", e.g. ":161") and serve requests forever. Blocks;
 * run it as the program's main loop, or in a `spawn`. A request with the wrong
 * community, or a malformed datagram, is dropped silently (as a real agent does).
 * To serve with a shutdown control, bind the socket yourself and use serveOn.
 * @param a {Agent} the agent to serve
 * @param address {string} the UDP bind address
 * @throws snmp if the address cannot be bound
 */
export func serve(a as Agent, address as string) {
    def socket as net.UDPSocket init net.listenUDP($address);
    defer net.close($socket);
    def never as channel of bool init channel.make(1);
    serveOn($a, $socket, $never);
}

/**
 * Serve requests on an already-bound UDP socket until a shutdown is signalled.
 * Useful for embedding an agent beside a client in one program (bind first, then
 * `spawn` this, so there is no bind race). To stop it, `channel.send($stop,
 * true)` (a capacity >= 1 channel); the loop returns within SHUTDOWN_POLL_MS.
 * `task.wait` the spawned handle afterwards for a graceful join. The caller owns
 * the socket's lifetime; a request with the wrong community or a malformed
 * datagram is dropped silently.
 * @param a {Agent} the agent to serve
 * @param socket {net.UDPSocket} a bound UDP socket
 * @param stop {channel of bool} a value on this channel stops the loop
 */
export func serveOn(a as Agent, socket as net.UDPSocket, stop as channel of bool) {
    def mib as list of Varbind init $a.bindings;
    while (true) {
        if (channel.len($stop) > 0) {
            return;
        }
        # Bound the wait so the loop periodically re-checks the shutdown channel;
        # a real datagram still returns immediately, so request latency is
        # unaffected.
        net.setDeadline($socket, SHUTDOWN_POLL_MS);
        def dg as net.Datagram;
        try {
            $dg = net.recvFrom($socket, 65535);
        } catch (e) {
            # A deadline timeout is expected (idle) - loop and re-check the
            # shutdown channel. Any other read error means the socket is gone;
            # stop serving.
            if (not isTimeout($e)) {
                return;
            }
            continue;
        }
        try {
            def r as ServeResult init handleRequest($mib, $a, $dg.data);
            $mib = $r.mib;
            if (len($r.reply) > 0) {
                net.sendTo($socket, $dg.peer, $r.reply);
            }
        } catch (e) {  # lint-disable: L103
            # A malformed request (or a transient send error) is intentionally
            # dropped, like a real agent - keep serving the next datagram.
        }
    }
}

# isTimeout reports whether a caught error is a receive-deadline timeout (as
# opposed to a real socket failure).
func isTimeout(e as Error) {
    return strings.contains($e.message, "timeout") or strings.contains($e.message, "timed out");
}

# --- agent request handling (private) ---

func emptyReply() {
    def b as bytes;
    return $b;
}

# handleRequest decodes one request and produces the reply bytes plus the MIB
# after any SET. A wrong community or an unsupported PDU yields an empty reply.
func handleRequest(mib as list of Varbind, a as Agent, data as bytes) {
    def req as Message init decodeMessage($data);
    if ($req.community != $a.community) {
        return ServeResult{reply: emptyReply(), mib: $mib};
    }
    # Serve only versions up to the one configured: a v2c agent answers v1 and
    # v2c, a v1 agent answers only v1. A higher-version request is dropped (the
    # agent does not speak it), the response echoes the request's version.
    if ($req.version > $a.version) {
        return ServeResult{reply: emptyReply(), mib: $mib};
    }
    match ($req.pduTag) {
        when PDU_GET {
            return respondGet($mib, $req);
        }
        when PDU_GETNEXT {
            return respondGetNext($mib, $req);
        }
        when PDU_SET {
            return respondSet($mib, $req);
        }
        else {
            return ServeResult{reply: emptyReply(), mib: $mib};
        }
    }
}

func respondGet(mib as list of Varbind, req as Message) {
    def out as list of Varbind;
    def i as int init 0;
    while ($i < len($req.varbinds)) {
        def oid as string init $req.varbinds[$i].oid;
        def found as int init findEntry($mib, $oid);
        if ($found < 0) {
            if ($req.version == VERSION1) {
                return buildResult($mib, $req, 2, $i + 1, $req.varbinds);
            }
            $out[] = Varbind{oid: $oid, type: "noSuchObject", value: "", number: 0};
        } else {
            $out[] = $mib[$found];
        }
        $i = $i + 1;
    }
    return buildResult($mib, $req, 0, 0, $out);
}

func respondGetNext(mib as list of Varbind, req as Message) {
    def out as list of Varbind;
    def i as int init 0;
    while ($i < len($req.varbinds)) {
        def nxt as int init nextEntry($mib, $req.varbinds[$i].oid);
        if ($nxt < 0) {
            if ($req.version == VERSION1) {
                return buildResult($mib, $req, 2, $i + 1, $req.varbinds);
            }
            $out[] = Varbind{oid: $req.varbinds[$i].oid, type: "endOfMibView", value: "", number: 0};
        } else {
            $out[] = $mib[$nxt];
        }
        $i = $i + 1;
    }
    return buildResult($mib, $req, 0, 0, $out);
}

func respondSet(mib as list of Varbind, req as Message) {
    def updated as list of Varbind init $mib;
    def i as int init 0;
    while ($i < len($req.varbinds)) {
        def rvb as Varbind init $req.varbinds[$i];
        def found as int init findEntry($updated, $rvb.oid);
        if ($found < 0) {
            # Unknown / non-writable OID: error, leave the MIB unchanged.
            return buildResult($mib, $req, 2, $i + 1, $req.varbinds);
        }
        $updated[$found] = $rvb;
        $i = $i + 1;
    }
    return buildResult($updated, $req, 0, 0, $req.varbinds);
}

# buildResult encodes a Response PDU echoing the request-id and pairs it with the
# resulting MIB.
func buildResult(mib as list of Varbind, req as Message, errStatus as int, errIndex as int, out as list of Varbind) {
    def reply as bytes init encodeMessage($req.version, $req.community, PDU_RESPONSE, $req.requestId, $errStatus, $errIndex, $out, false);
    return ServeResult{reply: $reply, mib: $mib};
}

# findEntry returns the index of the exact-OID binding, or -1.
func findEntry(mib as list of Varbind, oid as string) {
    def i as int init 0;
    while ($i < len($mib)) {
        if ($mib[$i].oid == $oid) {
            return $i;
        }
        $i = $i + 1;
    }
    return -1;
}

# nextEntry returns the index of the binding with the smallest OID strictly
# greater than target (GETNEXT), or -1 if none (end of MIB).
func nextEntry(mib as list of Varbind, target as string) {
    def best as int init -1;
    def i as int init 0;
    while ($i < len($mib)) {
        if (compareOid($mib[$i].oid, $target) > 0) {
            if ($best < 0 or compareOid($mib[$i].oid, $mib[$best].oid) < 0) {
                $best = $i;
            }
        }
        $i = $i + 1;
    }
    return $best;
}

# compareOid orders two dotted OIDs by numeric component (not string), so
# 1.3.6.1.2.1.1.10.0 sorts after 1.3.6.1.2.1.1.2.0. Returns -1 / 0 / 1.
func compareOid(a as string, b as string) {
    def pa as list of string init strings.split($a, ".");
    def pb as list of string init strings.split($b, ".");
    def i as int init 0;
    while ($i < len($pa) and $i < len($pb)) {
        def na as int init convert.toInt($pa[$i]);
        def nb as int init convert.toInt($pb[$i]);
        if ($na < $nb) {
            return -1;
        }
        if ($na > $nb) {
            return 1;
        }
        $i = $i + 1;
    }
    if (len($pa) < len($pb)) {
        return -1;
    }
    if (len($pa) > len($pb)) {
        return 1;
    }
    return 0;
}

# --- transport (private) ---

func request(c as Client, pduTag as int, vbs as list of Varbind) {
    def requestId as int init math.randInt(1, 2147483647);
    def nullValues as bool init ($pduTag != PDU_SET);
    # A malformed OID or value in a binding surfaces as an asn1 error while
    # encoding; normalise it to one snmp-kind error like the decode path.
    def payload as bytes;
    try {
        $payload = encodeMessage($c.version, $c.community, $pduTag, $requestId, 0, 0, $vbs, $nullValues);
    } catch (e) {
        if ($e.kind == "snmp") {
            throw $e;
        }
        fail("cannot build SNMP request: " + $e.message);
    }
    # The transport (bind / send / receive) can raise a raw net error, e.g. an
    # unresolvable address on sendTo; normalise those to one snmp-kind error too,
    # so a caller filtering on kind == "snmp" catches every client failure.
    def reply as bytes;
    try {
        $reply = exchange($c, $payload);
    } catch (e) {
        if ($e.kind == "snmp") {
            throw $e;
        }
        fail("SNMP transport error to " + $c.address + ": " + $e.message);
    }
    def resp as Message init decodeMessage($reply);
    if ($resp.requestId != $requestId) {
        fail("response request-id mismatch (sent " + convert.toString($requestId) + ", got " + convert.toString($resp.requestId) + ")");
    }
    if ($resp.errorStatus != 0) {
        fail("agent returned error-status " + convert.toString($resp.errorStatus) + " at index " + convert.toString($resp.errorIndex));
    }
    return $resp.varbinds;
}

func exchange(c as Client, payload as bytes) {
    def sock as net.UDPSocket init net.listenUDP(":0");
    defer net.close($sock);
    def attempt as int init 0;
    while ($attempt <= $c.retries) {
        net.setDeadline($sock, $c.timeoutMs);
        net.sendTo($sock, $c.address, $payload);
        try {
            def dg as net.Datagram init net.recvFrom($sock, 65535);
            return $dg.data;
        } catch (e) {
            $attempt = $attempt + 1;
        }
    }
    fail("no response from " + $c.address + " after " + convert.toString($c.retries + 1) + " attempt(s)");
}

# --- message codec (private, pure) ---

func oidsToVarbinds(oids as list of string) {
    def vbs as list of Varbind;
    for (def o in $oids) {
        $vbs[] = Varbind{oid: $o, type: "null", value: "", number: 0};
    }
    return $vbs;
}

# encodeMessage builds a complete SNMP message. nullValues true encodes each
# binding's value as NULL (a GET / GETNEXT request); false encodes the actual
# typed value (a SET, or a simulated response).
func encodeMessage(version as int, community as string, pduTag as int, requestId as int, errStatus as int, errIndex as int, vbs as list of Varbind, nullValues as bool) {
    def bindings as list of asn1.Value;
    for (def vb in $vbs) {
        def valElem as asn1.Value;
        if ($nullValues) {
            $valElem = asn1.null();
        } else {
            $valElem = encodeValue($vb);
        }
        $bindings[] = asn1.sequence([asn1.oid($vb.oid), $valElem]);
    }
    def pduBody as asn1.Value init asn1.sequence([
        asn1.integer($requestId),
        asn1.integer($errStatus),
        asn1.integer($errIndex),
        asn1.sequence($bindings)
    ]);
    def message as asn1.Value init asn1.sequence([
        asn1.integer($version),
        asn1.octetString(convert.bytesFromString($community, "utf-8")),
        asn1.retag("context", $pduTag, $pduBody)
    ]);
    return asn1.encode($message);
}

# encodeValue renders a Varbind's typed value as an asn1 element.
func encodeValue(vb as Varbind) {
    match ($vb.type) {
        when "null" {
            return asn1.null();
        }
        when "integer" {
            return asn1.integer($vb.number);
        }
        when "octetString" {
            return asn1.octetString(convert.bytesFromString($vb.value, "utf-8"));
        }
        when "oid" {
            return asn1.oid($vb.value);
        }
        when "counter32" {
            return asn1.retag("application", 1, asn1.integer($vb.number));
        }
        when "gauge32" {
            return asn1.retag("application", 2, asn1.integer($vb.number));
        }
        when "timeTicks" {
            return asn1.retag("application", 3, asn1.integer($vb.number));
        }
        when "counter64" {
            return asn1.retag("application", 6, asn1.integer($vb.number));
        }
        when "ipAddress" {
            return asn1.retag("application", 0, asn1.octetString(parseIp($vb.value)));
        }
        when "opaque" {
            return asn1.retag("application", 4, asn1.octetString(convert.bytesFromString($vb.value, "utf-8")));
        }
        when "noSuchObject" {
            return asn1.retag("context", 0, asn1.null());
        }
        when "noSuchInstance" {
            return asn1.retag("context", 1, asn1.null());
        }
        when "endOfMibView" {
            return asn1.retag("context", 2, asn1.null());
        }
        else {
            fail("cannot encode SNMP value of type '" + $vb.type + "'");
        }
    }
}

# decodeMessage parses a response message into its fields and bindings, reporting
# any malformed input (including a raw ASN.1 error) as a single snmp-kind error.
func decodeMessage(data as bytes) {
    try {
        return parseMessage($data);
    } catch (e) {
        if ($e.kind == "snmp") {
            throw $e;
        }
        fail("malformed SNMP response: " + $e.message);
    }
}

func parseMessage(data as bytes) {
    def tree as asn1.Value init asn1.decode($data);
    if (asn1.typeOf($tree) != "sequence" or asn1.length($tree) != 3) {
        fail("malformed SNMP message (expected a 3-field SEQUENCE)");
    }
    def pdu as asn1.Value init asn1.get($tree, "/2");
    if (asn1.tagClass($pdu) != "context" or asn1.length($pdu) != 4) {
        fail("malformed SNMP PDU");
    }
    def vblist as asn1.Value init asn1.get($pdu, "/3");
    def n as int init asn1.length($vblist);
    def vbs as list of Varbind;
    def i as int init 0;
    while ($i < $n) {
        def vb as asn1.Value init asn1.get($vblist, "/" + convert.toString($i));
        if (asn1.length($vb) != 2) {
            fail("malformed variable binding");
        }
        $vbs[] = decodeValue(asn1.asOid($vb, "/0"), asn1.get($vb, "/1"));
        $i = $i + 1;
    }
    return Message{
        version: asn1.asInt($tree, "/0"),
        community: asn1.asString($tree, "/1"),
        pduTag: asn1.tagNumber($pdu),
        requestId: asn1.asInt($pdu, "/0"),
        errorStatus: asn1.asInt($pdu, "/1"),
        errorIndex: asn1.asInt($pdu, "/2"),
        varbinds: $vbs
    };
}

# decodeValue renders one asn1 value element as a typed Varbind.
func decodeValue(oid as string, v as asn1.Value) {
    def cls as string init asn1.tagClass($v);
    def num as int init asn1.tagNumber($v);
    if ($cls == "universal") {
        match ($num) {
            when 2 {
                def n as int init asn1.asInt($v);
                return Varbind{oid: $oid, type: "integer", value: convert.toString($n), number: $n};
            }
            when 4 {
                return octetVarbind($oid, "octetString", asn1.asBytes($v));
            }
            when 6 {
                return Varbind{oid: $oid, type: "oid", value: asn1.asOid($v), number: 0};
            }
            when 5 {
                return Varbind{oid: $oid, type: "null", value: "", number: 0};
            }
            else {
                return octetVarbind($oid, asn1.typeOf($v), asn1.asBytes($v));
            }
        }
    } elseif ($cls == "application") {
        match ($num) {
            when 0 {
                return Varbind{oid: $oid, type: "ipAddress", value: renderIp(asn1.asBytes($v)), number: 0};
            }
            when 1 {
                return unsignedVarbind($oid, "counter32", asn1.asBytes($v));
            }
            when 2 {
                return unsignedVarbind($oid, "gauge32", asn1.asBytes($v));
            }
            when 3 {
                return unsignedVarbind($oid, "timeTicks", asn1.asBytes($v));
            }
            when 6 {
                return unsignedVarbind($oid, "counter64", asn1.asBytes($v));
            }
            when 4 {
                return octetVarbind($oid, "opaque", asn1.asBytes($v));
            }
            else {
                return octetVarbind($oid, "application", asn1.asBytes($v));
            }
        }
    } elseif ($cls == "context") {
        match ($num) {
            when 0 {
                return Varbind{oid: $oid, type: "noSuchObject", value: "", number: 0};
            }
            when 1 {
                return Varbind{oid: $oid, type: "noSuchInstance", value: "", number: 0};
            }
            when 2 {
                return Varbind{oid: $oid, type: "endOfMibView", value: "", number: 0};
            }
            else {
                return Varbind{oid: $oid, type: "context", value: "", number: 0};
            }
        }
    }
    return octetVarbind($oid, "unknown", asn1.asBytes($v));
}

# unsignedVarbind folds big-endian content into a non-negative integer. An
# unsigned value that does not fit a signed int64 (a Counter64 >= 2^63, which
# needs > 8 content octets or sets bit 63) would fold to a wrong negative number,
# so it is rendered as a "0x" hex string with number 0 instead - the value stays
# inspectable and is never silently corrupted.
func unsignedVarbind(oid as string, type as string, b as bytes) {
    if (len($b) <= 8) {
        def n as int init 0;
        def i as int init 0;
        while ($i < len($b)) {
            $n = ($n << 8) | $b[$i];
            $i = $i + 1;
        }
        if ($n >= 0) {
            return Varbind{oid: $oid, type: $type, value: convert.toString($n), number: $n};
        }
    }
    return Varbind{oid: $oid, type: $type, value: "0x" + encoding.toText($b, "hex"), number: 0};
}

# octetVarbind renders content as text if it is all printable ASCII, else hex.
func octetVarbind(oid as string, type as string, b as bytes) {
    return Varbind{oid: $oid, type: $type, value: renderOctets($b), number: 0};
}

func renderOctets(b as bytes) {
    if (isPrintable($b)) {
        return convert.stringFromBytes($b, "utf-8");
    }
    return "0x" + encoding.toText($b, "hex");
}

func isPrintable(b as bytes) {
    def i as int init 0;
    while ($i < len($b)) {
        def c as int init $b[$i];
        if ($c < 32 or $c > 126) {
            return false;
        }
        $i = $i + 1;
    }
    return true;
}

func renderIp(b as bytes) {
    if (len($b) != 4) {
        return "0x" + encoding.toText($b, "hex");
    }
    return convert.toString($b[0]) + "." + convert.toString($b[1]) + "." + convert.toString($b[2]) + "." + convert.toString($b[3]);
}

func parseIp(s as string) {
    def parts as list of string init strings.split($s, ".");
    if (len($parts) != 4) {
        fail("invalid IPv4 address: " + $s);
    }
    def b as bytes;
    for (def p in $parts) {
        def n as int init convert.toInt($p);
        if ($n < 0 or $n > 255) {
            fail("invalid IPv4 octet in: " + $s);
        }
        $b[] = $n;
    }
    return $b;
}

func inSubtree(oid as string, root as string) {
    return $oid == $root or strings.startsWith($oid, $root + ".");
}
