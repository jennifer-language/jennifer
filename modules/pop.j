# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.27.0
# pragma-jennifer-capability: net

/**
 * A POP3 receive client (RFC 1939): the line-oriented status dialogue ("+OK" /
 * "-ERR") over the `net` system library, with plaintext, implicit TLS, or STLS,
 * and auth by USER / PASS, XOAUTH2, CRAM-MD5, or SCRAM-SHA-1 / SCRAM-SHA-256.
 * Retrieved messages come back as strings, ready for the
 * `mime` module to parse. Because it uses `net`, this module needs the default
 * `jennifer` binary (`jennifer-tiny` has no network stack). A session is
 * stateful: `connect`, then `stat` / `listSizes` / `retrieve` / `deleteMessage`,
 * then `quit`. `fetchAll` wraps the common "get every message" case. A server
 * "-ERR" throws a catchable `pop.PopError` (kind "pop3") with the refused
 * `command`, the server's `text` and its RFC 2449 / 3206 `responseCode`
 * (`AUTH`, `IN-USE`, `SYS/TEMP`, ...).
 * @module pop
 * @example
 * use io;
 * import "pop.j" as pop;
 * import "mime.j" as mime;
 * import "transport.j" as transport;
 * def opts as pop.Options init pop.Options{host: "mail.example.com",
 *     port: 995, security: transport.Security.Tls, user: "me", pass: "secret", auth: ""};
 * for (def raw in pop.fetchAll($opts)) {
 *     def msg as mime.Part init mime.parse($raw);
 *     io.printf("subject: %s\n", mime.headerValue($msg, "Subject"));
 * }
 */
use net;
use binary;
use strings;
use regex;
use convert;
use hash;
use encoding;
import "./idna.j" as idna;
import "./sasl.j" as sasl;
import "./transport.j" as transport;

/**
 * Connection settings. USER / PASS and XOAUTH2 send the password or token as
 * it is, so over `transport.Security.None` they are refused unless
 * `allowInsecureAuth` is set; APOP, CRAM-MD5 and SCRAM never send the password.
 * @field host {string} the server hostname
 * @field port {int} the server port (110 plaintext / STLS, 995 implicit TLS)
 * @field security {transport.Security} `transport.Security.None`, `.Tls`, or `.Starttls`
 * @field user {string} the account username
 * @field pass {string} the password (or access token for xoauth2)
 * @field auth {string} "" (default - USER / PASS), "auto" (pick the strongest mechanism the server offers, falling back to USER / PASS), "apop" (RFC 1939 APOP), "xoauth2" (`pass` holds the access token), "cram" (CRAM-MD5), "scram-sha-1", or "scram-sha-256"
 * @field allowInsecureAuth {bool} send USER / PASS or XOAUTH2 over an unencrypted (`.None`)
 *   connection (default false - the plaintext credentials are refused)
 */
export def struct Options {
    host as string,
    port as int,
    security as transport.Security,
    user as string,
    pass as string,
    auth as string,
    allowInsecureAuth as bool
};

/**
 * A live POP3 session over one connection.
 * @field conn {net.Conn} the underlying network connection
 */
export def struct Session {
    conn as net.Conn
};

/**
 * Mailbox totals from STAT.
 * @field count {int} the number of messages in the mailbox
 * @field size {int} the total mailbox size in octets
 */
export def struct Stat {
    count as int,
    size as int
};

/**
 * One message's UIDL entry: its message number and its server-assigned unique id.
 * The `id` is stable across sessions (unlike the `number`, which is per-session),
 * so it is the key for "download only what is new": remember the ids you have
 * seen, and on the next connection retrieve only the numbers whose id is new.
 * @field number {int} the 1-based message number this session
 * @field id {string} the persistent unique id (RFC 1939 UIDL)
 */
export def struct MessageId {
    number as int,
    id as string
};

/**
 * One message's LIST entry: its message number and its size.
 * @field number {int} the 1-based message number this session
 * @field size {int} the message size in octets
 */
export def struct MessageSize {
    number as int,
    size as int
};

# --- pure protocol helpers (private, unit-tested) ------------------

# A UIDL line is "<number> <uid>", the uid 1 to 70 printable ASCII characters
# (RFC 1939). A LIST scan listing and a STAT drop listing are numbers, and RFC
# 1939 allows any text after them.
def const UIDL_LINE as string init '^([0-9]{1,18}) ([\x21-\x7E]{1,70})$';
def const LIST_LINE as string init '^([0-9]{1,18}) ([0-9]{1,18})( .*)?$';
def const STAT_LINE as string init '^\+OK ([0-9]{1,18}) ([0-9]{1,18})( .*)?$';

# malformed is the PopError of a reply that does not have the form its command
# needs.
func malformed(ctx as string, line as string) {
    def e as PopError init popError($ctx + ": malformed reply: " + $line);
    $e.command = $ctx;
    return $e;
}

# bodyLines splits a multi-line body into its lines; an empty body has none.
func bodyLines(body as string) {
    if ($body == "") {
        def none as list of string init [];
        return $none;
    }
    return strings.split($body, "\r\n");
}

# parseUidl reads a UIDL multiline body ("<number> <uid>" per line) into a list
# of MessageId. A malformed line throws a PopError.
func parseUidl(body as string) {
    def out as list of MessageId init [];
    for (def line in bodyLines($body)) {
        def m as regex.Match init regex.find(UIDL_LINE, $line);
        if ($m.start < 0) {
            throw malformed("UIDL", $line);
        }
        $out[] = MessageId{number: convert.toInt($m.groups[0]), id: $m.groups[1]};
    }
    return $out;
}

# statusOK reports whether a reply line is a "+OK" status.
func statusOK(line as string) {
    return $line == "+OK" or strings.startsWith($line, "+OK ");
}

# stripCR drops a single trailing CR.
func stripCR(line as string) {
    if (strings.endsWith($line, "\r")) {
        return $line[..len($line) - 1];
    }
    return $line;
}

# parseStat reads "+OK <count> <size>" into a Stat; any other reply throws a
# PopError.
func parseStat(line as string) {
    def m as regex.Match init regex.find(STAT_LINE, $line);
    if ($m.start < 0) {
        throw malformed("STAT", $line);
    }
    return Stat{count: convert.toInt($m.groups[0]), size: convert.toInt($m.groups[1])};
}

# dotTerminated reports whether a multi-line body buffer already holds the
# terminating "." line (an empty body starts with it; otherwise it follows a
# CRLF).
func dotTerminated(s as string) {
    return strings.startsWith($s, ".\r\n") or strings.contains($s, "\r\n.\r\n");
}

# parseDotBytes extracts a multi-line body up to its "." terminator line,
# undoing the byte-stuffing (a body line that began with "." was sent doubled).
# Lines end in CRLF only, so the body comes back byte for byte: a bare LF or CR
# inside a line is kept. The line before the terminator loses its CRLF, which
# belongs to the terminator. Lines are collected and joined once (an
# accumulating concat would be O(N^2) in the body size). No terminator line
# means the body was cut off: kind "closed".
func parseDotBytes(rest as bytes) {
    def lines as list of bytes init [];
    def pos as int init 0;
    while (true) {
        def nl as int init binary.indexOf($rest, CRLF, $pos);
        if ($nl < 0) {
            closedEarly();
        }
        def line as bytes init $rest[$pos..$nl];
        if ($line == DOT) {
            return binary.join($lines, CRLF);
        }
        if (binary.startsWith($line, DOT)) {
            $line = $line[1..];
        }
        $lines[] = $line;
        $pos = $nl + 2;
    }
}

# parseDotBody is parseDotBytes for text.
func parseDotBody(rest as string) {
    return encoding.decode("utf-8", parseDotBytes(encoding.encode("utf-8", $rest)));
}

# parseList reads a LIST body ("<number> <size>" per line) into MessageSize
# pairs, in the order listed. A malformed line throws a PopError.
func parseList(body as string) {
    def out as list of MessageSize init [];
    for (def line in bodyLines($body)) {
        def m as regex.Match init regex.find(LIST_LINE, $line);
        if ($m.start < 0) {
            throw malformed("LIST", $line);
        }
        $out[] = MessageSize{
            number: convert.toInt($m.groups[0]),
            size: convert.toInt($m.groups[1])
        };
    }
    return $out;
}

# parseSizes reads a LIST body into the sizes alone, in the order listed.
func parseSizes(body as string) {
    def out as list of int init [];
    for (def e in parseList($body)) {
        $out[] = $e.size;
    }
    return $out;
}

/**
 * A failed POP3 operation. Every error of kind `"pop3"` or `"pop"` is one, so
 * its fields are safe to read once the kind matches.
 * @field kind {string} `"pop3"`; `"pop"` for a local guard (a control character in an
 *   argument, plaintext credentials refused over `.None`)
 * @field message {string} `<command>: -ERR ...`, or what went wrong
 * @field command {string} the command the server refused (`PASS`, `RETR`, ...); "" for another failure
 * @field responseCode {string} the bracketed response code of the `-ERR` (RFC 2449 / 3206: `AUTH`, `IN-USE`, `LOGIN-DELAY`, `SYS/TEMP`, `SYS/PERM`), "" when none
 * @field text {string} the server's text after `-ERR` and the code, "" for another failure
 * @example
 * try {
 *     def s as pop.Session init pop.connect($opts);
 * } catch (e) {
 *     if ($e.kind == "pop3" and $e.responseCode == "IN-USE") {
 *         io.printf("another client holds the mailbox\n");
 *     } else {
 *         throw $e;
 *     }
 * }
 */
export def struct PopError {
    kind as string,
    message as string,
    command as string,
    responseCode as string,
    text as string
};

# popError is the PopError of a failure that is no -ERR reply.
func popError(message as string) {
    return PopError{kind: "pop3", message: $message, command: "", responseCode: "", text: ""};
}

# guardError is the PopError of a local guard: a control character in an
# argument, or plaintext credentials over an unencrypted connection. It has
# always been kind "pop", not "pop3"; the kind is kept so
# existing handlers keep matching. The generic kinds are not guards: a
# response over the cap is kind "limit", a server closing mid-response
# "closed".
func guardError(message as string) {
    def e as PopError init popError($message);
    $e.kind = "pop";
    return $e;
}

# replyError reads a `-ERR [CODE] text` reply into a PopError.
func replyError(line as string, ctx as string) {
    def t as string init strings.trim($line);
    def err as PopError init popError($ctx + ": " + $t);
    $err.command = $ctx;
    def m as regex.Match init regex.find('^-ERR\s*(\[([A-Za-z0-9/-]+)[^\]]*\])?\s*(.*)$', $t);
    if ($m.start >= 0) {
        $err.responseCode = strings.upper($m.groups[1]);
        $err.text = $m.groups[2];
    }
    return $err;
}

func expectOK(line as string, ctx as string) {
    if (not statusOK($line)) {
        throw replyError($line, $ctx);
    }
}

# --- net dialogue (private) ----------------------------------------

# The per-read idle timeout (ms), so a hung server fails instead of blocking
# forever. Re-armed before each read.
def const TIMEOUT_MS as int init 30000;

# MAX_RESPONSE_BYTES caps a single accumulated response (a status line or a
# multiline body). A malicious / compromised server could stream a line or a
# dot-body that never terminates, growing the buffer without bound; crossing the
# limit fails the read with a catchable error instead.
def const MAX_RESPONSE_BYTES as int init 67108864;

# capResponse throws when an accumulated response has grown past the cap.
func capResponse(n as int) {
    if ($n > MAX_RESPONSE_BYTES) {
        throw Error{
            kind: "limit",
            message: "pop: response exceeds the {MAX_RESPONSE_BYTES}-byte limit"
        };
    }
    return;
}

# --- byte-buffer helpers -------------------------------------------
# POP3 message bodies carry arbitrary 8-bit / UTF-8 content framed by a "."
# terminator line, so the readers accumulate raw bytes and decode to a string
# only once the whole response is in hand. Decoding each 512-byte chunk
# separately would split a multi-byte sequence across a chunk boundary and
# corrupt the body.

def const LF as bytes init encoding.encode("utf-8", "\n");
def const CRLF as bytes init encoding.encode("utf-8", "\r\n");
def const DOT as bytes init encoding.encode("utf-8", ".");
def const DOT_CRLF as bytes init encoding.encode("utf-8", ".\r\n");
def const DOT_TERM as bytes init encoding.encode("utf-8", "\r\n.\r\n");

# readHead reads chunks until one carries an LF and returns everything read so
# far, joined once (a per-chunk concat would recopy the growing buffer). Only
# the newest chunk is searched: earlier ones held no LF. Returns what was read
# when the connection closes first; the caller finds the LF with binary.indexOf.
func readHead(conn as net.Conn) {
    def parts as list of bytes;
    def total as int init 0;
    while (true) {
        net.setReadDeadline($conn, TIMEOUT_MS);
        def chunk as bytes init net.readBytes($conn, 512);
        if (len($chunk) == 0) {
            break;
        }
        $parts[] = $chunk;
        $total = $total + len($chunk);
        capResponse($total);
        if (binary.indexOf($chunk, LF) >= 0) {
            break;
        }
    }
    return binary.join($parts);
}

# decodeText turns a response into a string; one that is not valid UTF-8
# throws a PopError (the bytes forms, retrieveBytes / topBytes, take any octet).
func decodeText(b as bytes, ctx as string) {
    try {
        return encoding.decode("utf-8", $b);
    } catch (e) {
        if ($e.kind != "encoding") {
            throw $e;
        }
    }
    def err as PopError init popError(
        $ctx + ": the response is not valid UTF-8 (use retrieveBytes / topBytes)");
    $err.command = $ctx;
    throw $err;
}

# readLine reads one CRLF-terminated status line (single-line responses do not
# over-read: the server sends the line and waits). A connection that closes
# before the line ends is kind "closed": a partial "+OK" is no reply.
func readLine(conn as net.Conn) {
    def buf as bytes init readHead($conn);
    def nl as int init binary.indexOf($buf, LF);
    if ($nl < 0) {
        closedEarly();
    }
    return stripCR(decodeText($buf[..$nl], "reply"));
}

# tail4 returns the last (up to) 4 bytes of b: enough to carry a partial
# "\r\n.\r\n" terminator over to the next read.
func tail4(b as bytes) {
    if (len($b) <= 4) {
        return $b;
    }
    return $b[len($b) - 4..];
}

# rejectControl throws if s carries a control byte. POP3 commands are CRLF
# terminated single lines, so a CR/LF in an argument (user, password) would inject
# extra commands (OM-006). Checked once here, the choke every command passes through.
func rejectControl(s as string, what as string) {
    for (def c in strings.chars($s)) {
        def cp as int init convert.toCodepoint($c);
        if ($cp < 32 or $cp == 127) {
            throw guardError($what + " contains a control character (POP3 command injection)");
        }
    }
    return;
}

# closedEarly throws the error for a server that closed mid-response: the
# generic kind "closed", not a PopError.
func closedEarly() {
    throw Error{kind: "closed", message: "pop: connection closed before the response completed"};
}

# command sends one line and returns the single-line status reply.
func command(conn as net.Conn, line as string) {
    rejectControl($line, "POP3 command");
    net.writeBytes($conn, encoding.encode("utf-8", $line + "\r\n"));
    return readLine($conn);
}

# readMultiline reads a multi-line response as text (readMultilineBytes,
# decoded as UTF-8).
func readMultiline(conn as net.Conn, ctx as string) {
    return decodeText(readMultilineBytes($conn, $ctx), $ctx);
}

# readMultilineBytes sends nothing; it reads a status line and, on "+OK", the
# dot-terminated body that follows, returning the un-stuffed body bytes. A
# "-ERR" status throws (no body follows, so it must not wait for a terminator).
func readMultilineBytes(conn as net.Conn, ctx as string) {
    def buf as bytes init readHead($conn);
    def nl as int init binary.indexOf($buf, LF);
    if ($nl < 0) {
        closedEarly();
    }
    expectOK(stripCR(decodeText($buf[..$nl], $ctx)), $ctx);
    def body as bytes init $buf[$nl + 1..];
    # The body is complete once it holds the terminating "\r\n.\r\n" (or starts
    # with a lone "." line = empty body). Chunks collect in a list and join once;
    # each new chunk is searched together with the previous 4 bytes, so a
    # terminator straddling a read boundary is still found and the scan stays
    # linear overall.
    def parts as list of bytes init [$body];
    def total as int init len($body);
    def found as bool init binary.startsWith($body, DOT_CRLF) or
        binary.indexOf($body, DOT_TERM) >= 0;
    def tail as bytes init tail4($body);
    while (not $found) {
        net.setReadDeadline($conn, TIMEOUT_MS);
        def chunk as bytes init net.readBytes($conn, 512);
        if (len($chunk) == 0) {
            # Closed before the terminator: the body is cut off, and must not
            # be returned as a whole message.
            closedEarly();
        }
        $parts[] = $chunk;
        $total = $total + len($chunk);
        capResponse($total);
        def window as bytes init binary.concat($tail, $chunk);
        $found = binary.indexOf($window, DOT_TERM) >= 0;
        if (not $found and $total - len($chunk) < 3) {
            # the body began shorter than ".\r\n": re-check its start
            $found = binary.startsWith(binary.join($parts), DOT_CRLF);
        }
        $tail = tail4($window);
    }
    return parseDotBytes(binary.join($parts));
}

func dial(opts as Options) {
    def addr as string init idna.toAscii($opts.host) + ":{$opts.port}";
    if ($opts.security == transport.Security.Tls) {
        return net.connectTLS($addr, TIMEOUT_MS);
    }
    return net.connect($addr, TIMEOUT_MS);
}

# --- session (exported) --------------------------------------------

/**
 * Open a session: greet, optional STLS upgrade, then USER / PASS auth.
 * @param opts {Options} the connection settings
 * @return {Session} a live authenticated session
 * @throws {PopError} kind "pop3" on a server "-ERR" reply, or a SCRAM "+OK" that skips the
 *   server-final (the server signature was never verified)
 */
export func connect(opts as Options) {
    def conn as net.Conn init dial($opts);
    # A greeting / STLS / auth failure must not leak the socket; on success the
    # caller owns the open connection. (The handle id survives net.startTLS.)
    errdefer net.close($conn);
    def greeting as string init readLine($conn);
    expectOK($greeting, "greeting");
    if ($opts.security == transport.Security.Starttls) {
        expectOK(command($conn, "STLS"), "STLS");
        $conn = net.startTLS($conn, TIMEOUT_MS);
    }
    authenticate($conn, $opts, $greeting);
    return Session{conn: $conn};
}

# apopTimestamp extracts the "<...>" timestamp banner from a POP3 greeting
# (RFC 1939), or "" when the server offers no APOP.
func apopTimestamp(greeting as string) {
    def lt as int init strings.indexOf($greeting, "<");
    if ($lt < 0) {
        return "";
    }
    def gt as int init strings.indexOf($greeting, ">");
    if ($gt < $lt) {
        return "";
    }
    return $greeting[$lt..$gt + 1];
}

# apopDigest is the lowercase-hex MD5 of the timestamp banner concatenated with
# the password - the APOP shared-secret proof (RFC 1939).
func apopDigest(stamp as string, pass as string) {
    return encoding.toText("hex", hash.compute("md5", encoding.encode("utf-8", $stamp + $pass)));
}

# apopAuth runs POP3 APOP (RFC 1939): the shared secret is proved by sending
# "APOP <user> <hex>", where <hex> is MD5(timestamp banner + password). The
# password itself never crosses the wire.
func apopAuth(conn as net.Conn, opts as Options, greeting as string) {
    def stamp as string init apopTimestamp($greeting);
    if ($stamp == "") {
        throw popError("APOP: the server greeting carries no timestamp");
    }
    expectOK(command($conn, "APOP " + $opts.user + " " + apopDigest($stamp, $opts.pass)), "APOP");
}

# popChallenge extracts the base64 payload from a "+ <base64>" SASL continuation.
func popChallenge(line as string) {
    def t as string init strings.trim($line);
    if (strings.startsWith($t, "+ ")) {
        return strings.trim($t[2..]);
    }
    return "";
}

# popCapaMechs asks the server for its SASL mechanisms via CAPA (RFC 2449),
# returning the names on the "SASL ..." capability line, or an empty list when
# the server has no CAPA / no SASL line (the caller then uses USER / PASS). Only
# a "-ERR" answer means no CAPA; a cap, a close or a timeout goes on.
func popCapaMechs(conn as net.Conn) {
    net.writeBytes($conn, encoding.encode("utf-8", "CAPA\r\n"));
    def body as string init "";
    try {
        $body = readMultiline($conn, "CAPA");
    } catch (e) {
        if ($e.kind != "pop3") {
            throw $e;
        }
        return [];
    }
    def out as list of string init [];
    for (def line in strings.split($body, "\n")) {
        def l as string init strings.trim($line);
        if (strings.startsWith(strings.upper($l), "SASL ")) {
            for (def tk in strings.split($l[5..], " ")) {
                if (not (strings.trim($tk) == "")) {
                    $out[] = strings.trim($tk);
                }
            }
        }
    }
    return $out;
}

# authenticate runs the POP3 auth exchange. "" is USER / PASS; "auto" probes
# CAPA and picks the strongest mechanism (a SASL mechanism, else APOP when the
# greeting offers it, else USER / PASS); an explicit opts.auth forces that
# mechanism ("apop" for APOP).
func authenticate(conn as net.Conn, opts as Options, greeting as string) {
    def mech as string init $opts.auth;
    if ($mech == "auto") {
        $mech = sasl.negotiate(popCapaMechs($conn));
        if ($mech == "" and not (apopTimestamp($greeting) == "")) {
            $mech = "apop";
        }
    }
    match ($mech) {
        when "apop" {
            apopAuth($conn, $opts, $greeting);
            return;
        }
        when "xoauth2" {
            refusePlaintext($opts, "XOAUTH2");
            expectOK(
                command($conn, "AUTH XOAUTH2 " + sasl.bearer($opts.user, $opts.pass)),
                "AUTH XOAUTH2");
            return;
        }
        when "cram" {
            def chal as string init command($conn, "AUTH CRAM-MD5");
            if (not strings.startsWith(strings.trim($chal), "+ ")) {
                throw popError("AUTH CRAM-MD5: " + strings.trim($chal));
            }
            expectOK(
                command($conn, sasl.cram($opts.user, $opts.pass, popChallenge($chal))),
                "CRAM-MD5 response");
            return;
        }
        when "scram-sha-1", "scram-sha-256" {
            scramAuth($conn, $opts, $mech);
            return;
        }
        else {
            refusePlaintext($opts, "USER / PASS");
            expectOK(command($conn, "USER " + $opts.user), "USER");
            expectOK(command($conn, "PASS " + $opts.pass), "PASS");
        }
    }
}

# refusePlaintext throws unless the connection is encrypted or the caller set
# allowInsecureAuth: USER / PASS and XOAUTH2 send the secret as it is.
func refusePlaintext(opts as Options, mech as string) {
    if (not transport.encrypted($opts.security) and not $opts.allowInsecureAuth) {
        throw guardError("pop: refusing to send " + $mech +
            " credentials over an unencrypted connection (security .None); " +
            "use .Tls or .Starttls, or set allowInsecureAuth to force");
    }
    return;
}

# scramAuth runs the SCRAM exchange (RFC 5802) over POP3: the server-first and
# server-final arrive as "+ ..." continuations; the client verifies the server
# signature before accepting. A "+OK" straight after the client-final skips
# the server-final, so the server never proved it knows the password (a MITM
# can do that): the session is refused.
func scramAuth(conn as net.Conn, opts as Options, mech as string) {
    def algo as string init "sha256";
    def wire as string init "SCRAM-SHA-256";
    if ($mech == "scram-sha-1") {
        $algo = "sha1";
        $wire = "SCRAM-SHA-1";
    }
    def sc as sasl.Scram init sasl.scramStart($algo, $opts.user);
    def first as string init command($conn, "AUTH " + $wire + " " + sasl.scramClientFirst($sc));
    if (not strings.startsWith(strings.trim($first), "+ ")) {
        throw popError($wire + " server-first: " + strings.trim($first));
    }
    $sc = sasl.scramClientFinal($sc, popChallenge($first), $opts.pass);
    def final as string init command($conn, sasl.scramFinalToken($sc));
    if (strings.startsWith(strings.trim($final), "+ ")) {
        if (not sasl.scramVerify($sc, popChallenge($final))) {
            command($conn, "*");
            throw popError($wire + ": server signature verification failed");
        }
        expectOK(command($conn, ""), $wire + " completion");
        return;
    }
    expectOK($final, $wire);
    throw popError($wire + ": server accepted without sending the server-final; " +
        "its signature was not verified, session refused");
}

/**
 * Return the mailbox message count and total size.
 * @param session {Session} the live session
 * @return {Stat} the mailbox totals
 * @throws {PopError} kind "pop3" on a server "-ERR" reply or a malformed STAT reply
 */
export func stat(session as Session) {
    def line as string init command($session.conn, "STAT");
    expectOK($line, "STAT");
    return parseStat($line);
}

/**
 * Return just the number of messages waiting.
 * @param session {Session} the live session
 * @return {int} the message count
 * @throws {PopError} kind "pop3" on a server "-ERR" reply
 */
export func count(session as Session) {
    return stat($session).count;
}

/**
 * Return the number and octet size of each message (LIST), in the order the
 * server lists them. A message marked for deletion this session is not listed,
 * so the numbers can have gaps: key on `number`, not on the list position.
 * @param session {Session} the live session
 * @return {list of MessageSize} the number and size of each message
 * @throws {PopError} kind "pop3" on a server "-ERR" reply or a malformed LIST line
 */
export func listSizes(session as Session) {
    net.writeBytes($session.conn, encoding.encode("utf-8", "LIST\r\n"));
    return parseList(readMultiline($session.conn, "LIST"));
}

/**
 * Return the octet size of each message (LIST), in the order listed, without
 * the message numbers. Once a message is marked for deletion this session it is
 * not listed, and a position no longer maps to a message number; use
 * `listSizes` for the number of each size.
 * @param session {Session} the live session
 * @return {list of int} the size in octets of each listed message
 * @throws {PopError} kind "pop3" on a server "-ERR" reply or a malformed LIST line
 */
export func sizes(session as Session) {
    def out as list of int init [];
    for (def e in listSizes($session)) {
        $out[] = $e.size;
    }
    return $out;
}

/**
 * Fetch message `n` as a raw message string (RETR), ready for `mime.parse`.
 * The text is the message as sent, dot-stuffing undone, without the final
 * CRLF. A message that is not valid UTF-8 (raw 8-bit Latin-1, say) throws; read
 * it with `retrieveBytes`.
 * @param session {Session} the live session
 * @param n {int} the 1-based message number
 * @return {string} the raw message text
 * @throws {PopError} kind "pop3" on a server "-ERR" reply or a message that is not valid UTF-8
 */
export func retrieve(session as Session, n as int) {
    return decodeText(retrieveBytes($session, $n), "RETR");
}

/**
 * Fetch message `n` as its exact bytes (RETR): any octet, dot-stuffing undone,
 * without the final CRLF.
 * @param session {Session} the live session
 * @param n {int} the 1-based message number
 * @return {bytes} the raw message
 * @throws {PopError} kind "pop3" on a server "-ERR" reply
 */
export func retrieveBytes(session as Session, n as int) {
    def cmd as string init "RETR {$n}";
    net.writeBytes($session.conn, encoding.encode("utf-8", $cmd + "\r\n"));
    return readMultilineBytes($session.conn, "RETR");
}

/**
 * Return the stable unique id of every message (UIDL), as `MessageId`
 * `{number, id}` pairs in message order. Unlike the per-session message number,
 * a UIDL `id` persists across sessions, so this is the key to leave-on-server /
 * skip-seen: keep the ids you have processed, and next time retrieve only the
 * `number`s whose `id` is new.
 * @param session {Session} the live session
 * @return {list of MessageId} the unique id of each message
 * @throws {PopError} kind "pop3" on a server "-ERR" reply (or no UIDL support) or a
 *   malformed UIDL line
 */
export func uidl(session as Session) {
    net.writeBytes($session.conn, encoding.encode("utf-8", "UIDL\r\n"));
    return parseUidl(readMultiline($session.conn, "UIDL"));
}

/**
 * Return the stable unique id of one message (UIDL n).
 * @param session {Session} the live session
 * @param n {int} the 1-based message number
 * @return {string} the message's persistent unique id
 * @throws {PopError} kind "pop3" on a server "-ERR" reply, or a reply that is not
 *   "+OK <n> <uid>" for this `n`
 */
export func uidlOne(session as Session, n as int) {
    def line as string init command($session.conn, "UIDL {$n}");
    expectOK($line, "UIDL");
    return parseUidlOne($line, $n);
}

# parseUidlOne reads "+OK <n> <uid>" (a UIDL id is printable ASCII with no
# spaces, RFC 1939); another form, or another message's number, throws.
func parseUidlOne(line as string, n as int) {
    if (not strings.startsWith($line, "+OK ")) {
        throw malformed("UIDL", $line);
    }
    def m as regex.Match init regex.find(UIDL_LINE, $line[4..]);
    if ($m.start < 0 or convert.toInt($m.groups[0]) != $n) {
        throw malformed("UIDL", $line);
    }
    return $m.groups[1];
}

/**
 * Preview message `n`: its full headers plus the first `lines` body lines (TOP).
 * `lines` = 0 fetches headers only - a cheap way to read Subject / From / Date
 * (parse the result with `mime.parse`) and decide whether to `retrieve` the whole
 * body. TOP is widely but not universally supported (a server lacking it answers
 * "-ERR").
 * @param session {Session} the live session
 * @param n {int} the 1-based message number
 * @param lines {int} how many leading body lines to include (0 = headers only)
 * @return {string} the header block plus the requested body lines
 * @throws {PopError} kind "pop3" on a server "-ERR" reply (or no TOP support), or a
 *   response that is not valid UTF-8
 */
export func top(session as Session, n as int, lines as int) {
    return decodeText(topBytes($session, $n, $lines), "TOP");
}

/**
 * `top` as exact bytes: any octet, dot-stuffing undone.
 * @param session {Session} the live session
 * @param n {int} the 1-based message number
 * @param lines {int} how many leading body lines to include (0 = headers only)
 * @return {bytes} the header block plus the requested body lines
 * @throws {PopError} kind "pop3" on a server "-ERR" reply (or no TOP support)
 */
export func topBytes(session as Session, n as int, lines as int) {
    def cmd as string init "TOP {$n} {$lines}";
    net.writeBytes($session.conn, encoding.encode("utf-8", $cmd + "\r\n"));
    return readMultilineBytes($session.conn, "TOP");
}

/**
 * Unmark every message marked for deletion this session (RSET), so a QUIT after
 * it deletes nothing. Use it to abort a batch of `deleteMessage` calls before
 * committing.
 * @param session {Session} the live session
 * @throws {PopError} kind "pop3" on a server "-ERR" reply
 */
export func reset(session as Session) {
    expectOK(command($session.conn, "RSET"), "RSET");
    return;
}

/**
 * Do nothing but keep the connection alive (NOOP) - the server resets its idle
 * timer and replies "+OK". Useful between long pauses in a session.
 * @param session {Session} the live session
 * @throws {PopError} kind "pop3" on a server "-ERR" reply
 */
export func noop(session as Session) {
    expectOK(command($session.conn, "NOOP"), "NOOP");
    return;
}

/**
 * Mark message `n` for deletion (DELE); it is removed at QUIT.
 * @param session {Session} the live session
 * @param n {int} the 1-based message number
 * @throws {PopError} kind "pop3" on a server "-ERR" reply
 */
export func deleteMessage(session as Session, n as int) {
    expectOK(command($session.conn, "DELE {$n}"), "DELE");
}

/**
 * End the session (committing any deletions) and close the connection.
 * @param session {Session} the live session
 */
export func quit(session as Session) {
    # The socket is shut even when the QUIT dialogue throws (a dead server
    # must not leak the fd).
    defer net.close($session.conn);
    command($session.conn, "QUIT");
}

/**
 * Connect, retrieve every message (without deleting), and quit.
 * @param opts {Options} the connection settings
 * @return {list of string} every message as a raw string, in message order
 * @throws {PopError} kind "pop3" on a server "-ERR" reply
 */
export func fetchAll(opts as Options) {
    def session as Session init connect($opts);
    def n as int init stat($session).count;
    def msgs as list of string init [];
    def i as int init 1;
    while ($i <= $n) {
        $msgs[] = retrieve($session, $i);
        $i = $i + 1;
    }
    quit($session);
    return $msgs;
}
