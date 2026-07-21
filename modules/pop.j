# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * A POP3 receive client (RFC 1939): the line-oriented status dialogue ("+OK" /
 * "-ERR") over the `net` system library, with plaintext, implicit TLS, or STLS,
 * and auth by USER / PASS, XOAUTH2, CRAM-MD5, or SCRAM-SHA-1 / SCRAM-SHA-256.
 * Retrieved messages come back as strings, ready for the
 * `mime` module to parse. Because it uses `net`, this module needs the default
 * `jennifer` binary (`jennifer-tiny` has no network stack). A session is
 * stateful: `connect`, then `stat` / `sizes` / `retrieve` / `deleteMessage`,
 * then `quit`. `fetchAll` wraps the common "get every message" case. A server
 * "-ERR" throws a catchable `Error` (kind "pop3").
 * @module pop
 * @example
 * def opts as pop.Options init pop.Options{host: "mail.example.com",
 *     port: 995, security: "tls", user: "me", pass: "secret", auth: ""};
 * for (def raw in pop.fetchAll($opts)) {
 *     def msg as mime.Part init mime.parse($raw);
 *     io.printf("subject: %s\n", mime.headerValue($msg, "Subject"));
 * }
 */
use net;
use strings;
use convert;
use hash;
use encoding;
import "./idna.j" as idna;
import "./sasl.j" as sasl;

/**
 * Connection settings. `security` is "none", "tls" (implicit TLS on connect,
 * port 995), or "starttls" (STLS upgrade on port 110). `auth` is "" (USER /
 * PASS) or "xoauth2" (SASL bearer token, where `pass` holds the access token).
 * @field host {string} the server hostname
 * @field port {int} the server port (110 plaintext / STLS, 995 implicit TLS)
 * @field security {string} "none", "tls", or "starttls"
 * @field user {string} the account username
 * @field pass {string} the password (or access token for xoauth2)
 * @field auth {string} "" (default - USER / PASS), "auto" (pick the strongest mechanism the server offers, falling back to USER / PASS), "apop" (RFC 1939 APOP), "xoauth2", "cram" (CRAM-MD5), "scram-sha-1", or "scram-sha-256"
 */
export def struct Options {
    host as string,
    port as int,
    security as string,
    user as string,
    pass as string,
    auth as string
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

# --- pure protocol helpers (private, unit-tested) ------------------

# statusOK reports whether a reply line is a "+OK" status.
func statusOK(line as string) {
    return strings.startsWith($line, "+OK");
}

# stripCR drops a single trailing CR.
func stripCR(line as string) {
    if (strings.endsWith($line, "\r")) {
        return strings.substring($line, 0, len($line) - 1);
    }
    return $line;
}

# parseStat reads "+OK <count> <size>" into a Stat.
func parseStat(line as string) {
    def parts as list of string init strings.split(strings.trim($line), " ");
    def st as Stat init Stat{count: 0, size: 0};
    if (len($parts) >= 3) {
        $st.count = convert.toInt($parts[1]);
        $st.size = convert.toInt($parts[2]);
    }
    return $st;
}

# dotTerminated reports whether a multi-line body buffer already holds the
# terminating "." line (an empty body starts with it; otherwise it follows a
# CRLF).
func dotTerminated(s as string) {
    return strings.startsWith($s, ".\r\n") or strings.contains($s, "\r\n.\r\n");
}

# parseDotBody extracts a multi-line body up to its "." terminator, undoing the
# byte-stuffing (a body line that began with "." was sent doubled). Lines are
# collected and rejoined with CRLF once (an accumulating `+` would be O(N^2) in
# the body size).
func parseDotBody(rest as string) {
    def lines as list of string init [];
    for (def raw in strings.split($rest, "\n")) {
        def line as string init stripCR($raw);
        if ($line == ".") {
            return strings.join($lines, "\r\n");
        }
        def dl as string init $line;
        if (strings.startsWith($line, ".")) {
            $dl = strings.substring($line, 1, len($line));
        }
        $lines[] = $dl;
    }
    return strings.join($lines, "\r\n");
}

# parseSizes reads a LIST body ("num size" per line) into the sizes in order.
func parseSizes(body as string) {
    def out as list of int init [];
    for (def line in strings.split($body, "\r\n")) {
        def sp as int init strings.indexOf(strings.trim($line), " ");
        if ($sp >= 0) {
            $out[] = convert.toInt(strings.substring(strings.trim($line), $sp + 1));
        }
    }
    return $out;
}


func expectOK(line as string, ctx as string) {
    if (not statusOK($line)) {
        def msg as string init $ctx + ": " + strings.trim($line);
        throw Error{kind: "pop3", message: $msg, file: "", line: 0, col: 0};
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
        throw Error{kind: "pop", message: "pop: response exceeds the " + convert.toString(MAX_RESPONSE_BYTES) + "-byte limit", file: "", line: 0, col: 0};
    }
    return;
}

# --- byte-buffer helpers -------------------------------------------
# POP3 message bodies carry arbitrary 8-bit / UTF-8 content framed by a "."
# terminator line, so the readers accumulate raw bytes and decode to a string
# only once the whole response is in hand. Decoding each 512-byte chunk
# separately would split a multi-byte sequence across a chunk boundary and
# corrupt the body.

func emptyBytes() {
    def e as bytes;
    return $e;
}

# byteSlice returns buf[start:end] as a fresh bytes value.
func byteSlice(buf as bytes, start as int, end as int) {
    def out as bytes;
    def i as int init $start;
    while ($i < $end) {
        $out[] = $buf[$i];
        $i = $i + 1;
    }
    return $out;
}

# lfIndex returns the index of the first LF at or after `from`, or -1.
func lfIndex(buf as bytes, from as int) {
    def i as int init $from;
    def n as int init len($buf);
    while ($i < $n) {
        if ($buf[$i] == 10) {
            return $i;
        }
        $i = $i + 1;
    }
    return -1;
}


# readLine reads one CRLF-terminated status line (single-line responses do not
# over-read: the server sends the line and waits). The chunk is appended into
# the owning `buf` in place (amortised O(1) per byte); a by-value append helper
# would copy the whole growing buffer on every read.
func readLine(conn as net.Conn) {
    def buf as bytes;
    def nl as int init -1;
    while ($nl < 0) {
        net.setDeadline($conn, TIMEOUT_MS);
        def chunk as bytes init net.readBytes($conn, 512);
        if (len($chunk) == 0) {
            return stripCR(convert.stringFromBytes($buf, "utf-8"));
        }
        def i as int init 0;
        while ($i < len($chunk)) {
            $buf[] = $chunk[$i];
            $i = $i + 1;
        }
        capResponse(len($buf));
        $nl = lfIndex($buf, 0);
    }
    return stripCR(convert.stringFromBytes(byteSlice($buf, 0, $nl), "utf-8"));
}

# command sends one line and returns the single-line status reply.
func command(conn as net.Conn, line as string) {
    net.writeBytes($conn, convert.bytesFromString($line + "\r\n", "utf-8"));
    return readLine($conn);
}

# readMultiline sends nothing; it reads a status line and, on "+OK", the
# dot-terminated body that follows, returning the un-stuffed body. A "-ERR"
# status throws (no body follows, so it must not wait for a terminator).
func readMultiline(conn as net.Conn, ctx as string) {
    def buf as bytes;
    def nl as int init -1;
    while ($nl < 0) {
        net.setDeadline($conn, TIMEOUT_MS);
        def chunk as bytes init net.readBytes($conn, 512);
        if (len($chunk) == 0) {
            return "";
        }
        def i as int init 0;
        while ($i < len($chunk)) {
            $buf[] = $chunk[$i];
            $i = $i + 1;
        }
        capResponse(len($buf));
        $nl = lfIndex($buf, 0);
    }
    expectOK(stripCR(convert.stringFromBytes(byteSlice($buf, 0, $nl), "utf-8")), $ctx);
    def body as bytes init byteSlice($buf, $nl + 1, len($buf));
    def scanFrom as int init 0;
    while (true) {
        # Scan for the terminating "\r\n.\r\n" (or a lone "." first line = empty
        # body) by reading $body in place. Passing the whole growing body to a
        # helper each pass would deep-copy it (value semantics) and make a large
        # message O(n^2); indexing it directly here keeps the read loop O(n).
        def blen as int init len($body);
        def found as bool init $blen >= 3 and $body[0] == 46 and $body[1] == 13 and $body[2] == 10;
        def si as int init $scanFrom;
        if ($si < 0) {
            $si = 0;
        }
        while ($si + 4 < $blen and not $found) {
            if ($body[$si] == 13 and $body[$si + 1] == 10 and $body[$si + 2] == 46 and $body[$si + 3] == 13 and $body[$si + 4] == 10) {
                $found = true;
            }
            $si = $si + 1;
        }
        if ($found) {
            return parseDotBody(convert.stringFromBytes($body, "utf-8"));
        }
        net.setDeadline($conn, TIMEOUT_MS);
        def chunk as bytes init net.readBytes($conn, 512);
        if (len($chunk) == 0) {
            return parseDotBody(convert.stringFromBytes($body, "utf-8"));
        }
        # Rewind the scan a few bytes so a "\r\n.\r\n" straddling this read
        # boundary is still detected; the loop stays linear overall.
        def prev as int init len($body);
        def j as int init 0;
        while ($j < len($chunk)) {
            $body[] = $chunk[$j];
            $j = $j + 1;
        }
        capResponse(len($body));
        $scanFrom = prev - 4;
        if ($scanFrom < 0) {
            $scanFrom = 0;
        }
    }
    return parseDotBody(convert.stringFromBytes($body, "utf-8"));
}

func dial(opts as Options) {
    def addr as string init idna.toAscii($opts.host) + ":" + convert.toString($opts.port);
    if ($opts.security == "tls") {
        return net.connectTLS($addr, TIMEOUT_MS);
    }
    return net.connect($addr, TIMEOUT_MS);
}

# --- session (exported) --------------------------------------------

/**
 * Open a session: greet, optional STLS upgrade, then USER / PASS auth.
 * @param opts {Options} the connection settings
 * @return {Session} a live authenticated session
 * @throws {Error} kind "pop3" on a server "-ERR" reply
 */
export func connect(opts as Options) {
    def conn as net.Conn init dial($opts);
    # A greeting / STLS / auth failure must not leak the socket; on success the
    # caller owns the open connection. (The handle id survives net.startTLS.)
    errdefer net.close($conn);
    def greeting as string init readLine($conn);
    expectOK($greeting, "greeting");
    if ($opts.security == "starttls") {
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
    return strings.substring($greeting, $lt, $gt + 1);
}

# apopDigest is the lowercase-hex MD5 of the timestamp banner concatenated with
# the password - the APOP shared-secret proof (RFC 1939).
func apopDigest(stamp as string, pass as string) {
    return encoding.toText(hash.compute(convert.bytesFromString($stamp + $pass, "utf-8"), "md5"), "hex");
}

# apopAuth runs POP3 APOP (RFC 1939): the shared secret is proved by sending
# "APOP <user> <hex>", where <hex> is MD5(timestamp banner + password). The
# password itself never crosses the wire.
func apopAuth(conn as net.Conn, opts as Options, greeting as string) {
    def stamp as string init apopTimestamp($greeting);
    if ($stamp == "") {
        throw Error{kind: "pop3", message: "APOP: the server greeting carries no timestamp", file: "", line: 0, col: 0};
    }
    expectOK(command($conn, "APOP " + $opts.user + " " + apopDigest($stamp, $opts.pass)), "APOP");
}

# popChallenge extracts the base64 payload from a "+ <base64>" SASL continuation.
func popChallenge(line as string) {
    def t as string init strings.trim($line);
    if (strings.startsWith($t, "+ ")) {
        return strings.trim(strings.substring($t, 2, len($t)));
    }
    return "";
}

# popCapaMechs asks the server for its SASL mechanisms via CAPA (RFC 2449),
# returning the names on the "SASL ..." capability line, or an empty list when
# the server has no CAPA / no SASL line (the caller then uses USER / PASS).
func popCapaMechs(conn as net.Conn) {
    net.writeBytes($conn, convert.bytesFromString("CAPA\r\n", "utf-8"));
    def body as string init "";
    try {
        $body = readMultiline($conn, "CAPA");
    } catch (e) {
        return [];
    }
    def out as list of string init [];
    for (def line in strings.split($body, "\n")) {
        def l as string init strings.trim($line);
        if (strings.startsWith(strings.upper($l), "SASL ")) {
            for (def tk in strings.split(strings.substring($l, 5, len($l)), " ")) {
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
    if ($mech == "apop") {
        apopAuth($conn, $opts, $greeting);
        return;
    }
    if ($mech == "xoauth2") {
        expectOK(command($conn, "AUTH XOAUTH2 " + sasl.bearer($opts.user, $opts.pass)), "AUTH XOAUTH2");
        return;
    }
    if ($mech == "cram") {
        def chal as string init command($conn, "AUTH CRAM-MD5");
        if (not strings.startsWith(strings.trim($chal), "+ ")) {
            throw Error{kind: "pop3", message: "AUTH CRAM-MD5: " + strings.trim($chal), file: "", line: 0, col: 0};
        }
        expectOK(command($conn, sasl.cram($opts.user, $opts.pass, popChallenge($chal))), "CRAM-MD5 response");
        return;
    }
    if ($mech == "scram-sha-1" or $mech == "scram-sha-256") {
        scramAuth($conn, $opts, $mech);
        return;
    }
    expectOK(command($conn, "USER " + $opts.user), "USER");
    expectOK(command($conn, "PASS " + $opts.pass), "PASS");
}

# scramAuth runs the SCRAM exchange (RFC 5802) over POP3: the server-first and
# server-final arrive as "+ ..." continuations; the client verifies the server
# signature before accepting. A "+OK" after the client-final means the server
# concluded without a separate server-final line.
func scramAuth(conn as net.Conn, opts as Options, mech as string) {
    def algo as string init "sha256";
    def wire as string init "SCRAM-SHA-256";
    if ($mech == "scram-sha-1") {
        $algo = "sha1";
        $wire = "SCRAM-SHA-1";
    }
    def sc as sasl.Scram init sasl.scramStart($opts.user, $algo);
    def first as string init command($conn, "AUTH " + $wire + " " + sasl.scramClientFirst($sc));
    if (not strings.startsWith(strings.trim($first), "+ ")) {
        throw Error{kind: "pop3", message: $wire + " server-first: " + strings.trim($first), file: "", line: 0, col: 0};
    }
    $sc = sasl.scramClientFinal($sc, popChallenge($first), $opts.pass);
    def final as string init command($conn, sasl.scramFinalToken($sc));
    if (strings.startsWith(strings.trim($final), "+ ")) {
        if (not sasl.scramVerify($sc, popChallenge($final))) {
            command($conn, "*");
            throw Error{kind: "pop3", message: $wire + ": server signature verification failed", file: "", line: 0, col: 0};
        }
        expectOK(command($conn, ""), $wire + " completion");
        return;
    }
    expectOK($final, $wire);
}

/**
 * Return the mailbox message count and total size.
 * @param session {Session} the live session
 * @return {Stat} the mailbox totals
 * @throws {Error} kind "pop3" on a server "-ERR" reply
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
 * @throws {Error} kind "pop3" on a server "-ERR" reply
 */
export func count(session as Session) {
    return stat($session).count;
}

/**
 * Return the octet size of each message, in message order (LIST).
 * @param session {Session} the live session
 * @return {list of int} the size in octets of each message
 * @throws {Error} kind "pop3" on a server "-ERR" reply
 */
export func sizes(session as Session) {
    net.writeBytes($session.conn, convert.bytesFromString("LIST\r\n", "utf-8"));
    return parseSizes(readMultiline($session.conn, "LIST"));
}

/**
 * Fetch message `n` as a raw message string (RETR), ready for `mime.parse`.
 * @param session {Session} the live session
 * @param n {int} the 1-based message number
 * @return {string} the raw message text
 * @throws {Error} kind "pop3" on a server "-ERR" reply
 */
export func retrieve(session as Session, n as int) {
    def cmd as string init "RETR " + convert.toString($n);
    net.writeBytes($session.conn, convert.bytesFromString($cmd + "\r\n", "utf-8"));
    return readMultiline($session.conn, "RETR");
}

/**
 * Mark message `n` for deletion (DELE); it is removed at QUIT.
 * @param session {Session} the live session
 * @param n {int} the 1-based message number
 * @throws {Error} kind "pop3" on a server "-ERR" reply
 */
export func deleteMessage(session as Session, n as int) {
    expectOK(command($session.conn, "DELE " + convert.toString($n)), "DELE");
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
 * @throws {Error} kind "pop3" on a server "-ERR" reply
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
