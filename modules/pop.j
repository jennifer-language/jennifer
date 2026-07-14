# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

/**
 * A POP3 receive client (RFC 1939): the line-oriented status dialogue ("+OK" /
 * "-ERR") over the `net` system library, with plaintext, implicit TLS, or STLS,
 * and USER / PASS auth. Retrieved messages come back as strings, ready for the
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
 * @field auth {string} "" for USER / PASS or "xoauth2" for SASL bearer
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
# rejoined with CRLF.
func parseDotBody(rest as string) {
    def out as string init "";
    def first as bool init true;
    for (def raw in strings.split($rest, "\n")) {
        def line as string init stripCR($raw);
        if ($line == ".") {
            return $out;
        }
        def dl as string init $line;
        if (strings.startsWith($line, ".")) {
            $dl = strings.substring($line, 1, len($line));
        }
        if (not $first) {
            $out = $out + "\r\n";
        }
        $first = false;
        $out = $out + $dl;
    }
    return $out;
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

# readLine reads one CRLF-terminated status line (single-line responses do not
# over-read: the server sends the line and waits).
func readLine(conn as net.Conn) {
    def buf as string init "";
    while (strings.indexOf($buf, "\n") < 0) {
        net.setDeadline($conn, TIMEOUT_MS);
        def chunk as bytes init net.readBytes($conn, 512);
        if (len($chunk) == 0) {
            return stripCR($buf);
        }
        $buf = $buf + convert.stringFromBytes($chunk, "utf-8");
    }
    def nl as int init strings.indexOf($buf, "\n");
    return stripCR(strings.substring($buf, 0, $nl));
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
    def buf as string init "";
    while (strings.indexOf($buf, "\n") < 0) {
        net.setDeadline($conn, TIMEOUT_MS);
        def chunk as bytes init net.readBytes($conn, 512);
        if (len($chunk) == 0) {
            return "";
        }
        $buf = $buf + convert.stringFromBytes($chunk, "utf-8");
    }
    def nl as int init strings.indexOf($buf, "\n");
    expectOK(stripCR(strings.substring($buf, 0, $nl)), $ctx);
    def rest as string init strings.substring($buf, $nl + 1);
    while (not dotTerminated($rest)) {
        net.setDeadline($conn, TIMEOUT_MS);
        def chunk as bytes init net.readBytes($conn, 512);
        if (len($chunk) == 0) {
            return parseDotBody($rest);
        }
        $rest = $rest + convert.stringFromBytes($chunk, "utf-8");
    }
    return parseDotBody($rest);
}

func dial(opts as Options) {
    def addr as string init idna.toAscii($opts.host) + ":" + convert.toString($opts.port);
    if ($opts.security == "tls") {
        return net.connectTLS($addr);
    }
    return net.connect($addr);
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
    expectOK(readLine($conn), "greeting");
    if ($opts.security == "starttls") {
        expectOK(command($conn, "STLS"), "STLS");
        $conn = net.startTLS($conn);
    }
    if ($opts.auth == "xoauth2") {
        def resp as string init "AUTH XOAUTH2 " + sasl.bearer($opts.user, $opts.pass);
        expectOK(command($conn, $resp), "AUTH XOAUTH2");
    } else {
        expectOK(command($conn, "USER " + $opts.user), "USER");
        expectOK(command($conn, "PASS " + $opts.pass), "PASS");
    }
    return Session{conn: $conn};
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
    command($session.conn, "QUIT");
    net.close($session.conn);
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
