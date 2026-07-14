# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

/**
 * An IMAP4rev1 receive client (RFC 3501): tagged commands and untagged "*"
 * responses over the `net` system library, with plaintext / implicit TLS /
 * STARTTLS and LOGIN auth. A useful reading subset - SELECT, SEARCH, FETCH the
 * whole message - not the full protocol. Retrieved messages come back as
 * strings for the `mime` module to parse. Uses `net`, so it needs the default
 * `jennifer` binary. A session is stateful: `connect`, `selectMailbox`,
 * `search` / `fetch`, `logout`. A "NO" / "BAD" completion throws a catchable
 * `Error` (kind "imap"). One fixed command tag is used, which is safe for this
 * synchronous client (one command in flight at a time). Message literals are
 * read assuming 7-bit / ASCII on the wire (MIME transfer encoding keeps mail
 * ASCII); raw 8-bit literals are not yet byte-exact.
 * @module imap
 * @example
 * import "imap.j" as imap;
 * import "mime.j" as mime;
 * def opts as imap.Options init imap.Options{host: "mail.example.com",
 *     port: 993, security: "tls", user: "me", pass: "secret"};
 * for (def raw in imap.fetchAll($opts, "INBOX")) {
 *     def msg as mime.Part init mime.parse($raw);
 *     io.printf("subject: %s\n", mime.headerValue($msg, "Subject"));
 * }
 */
use net;
use strings;
use convert;
use regex;
import "./sasl.j" as sasl;
import "./idna.j" as idna;

/**
 * The parameters for opening an IMAP session.
 * @field host {string} the server hostname
 * @field port {int} the server port (e.g. 993 for implicit TLS)
 * @field security {string} the transport, "" / "tls" (implicit) / "starttls"
 * @field user {string} the login username
 * @field pass {string} the login password, or the OAuth2 access token when auth is "xoauth2"
 * @field auth {string} the auth mechanism, "" (LOGIN) or "xoauth2" (SASL bearer via AUTHENTICATE)
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
 * An open IMAP session.
 * @field conn {net.Conn} the underlying connection
 */
export def struct Session {
    conn as net.Conn
};

# The single command tag. Distinctive so it never collides with response data.
def const TAG as string init "JEN";

# --- pure protocol helpers (private, unit-tested) ------------------

# quoteArg wraps a LOGIN argument as an IMAP quoted string, escaping `\` and `"`.
func quoteArg(s as string) {
    def esc as string init strings.replace($s, "\\", "\\\\");
    $esc = strings.replace($esc, "\"", "\\\"");
    return "\"" + $esc + "\"";
}

# isTagged reports whether a response line is the tagged completion.
func isTagged(line as string, tag as string) {
    return strings.startsWith($line, $tag + " ");
}

# literalLength returns N when a line ends with an IMAP literal marker `{N}`,
# else -1.
func literalLength(line as string) {
    def m as regex.Match init regex.find("\\{([0-9]+)\\}$", $line);
    if ($m.start < 0) {
        return -1;
    }
    return convert.toInt($m.groups[0]);
}

# extractLiteral returns the first `{N}`-introduced literal's content from a
# FETCH response (the message body), or "".
func extractLiteral(response as string) {
    def m as regex.Match init regex.find("\\{([0-9]+)\\}\r\n", $response);
    if ($m.start < 0) {
        return "";
    }
    def n as int init convert.toInt($m.groups[0]);
    return strings.substring($response, $m.end, $m.end + $n);
}

# parseExists returns the message count from an untagged "* N EXISTS" line.
func parseExists(response as string) {
    for (def line in strings.split($response, "\r\n")) {
        def m as regex.Match init regex.find("^\\* ([0-9]+) EXISTS", $line);
        if ($m.start >= 0) {
            return convert.toInt($m.groups[0]);
        }
    }
    return 0;
}

# searchLine finds the untagged "* SEARCH ..." line, or "".
func searchLine(response as string) {
    for (def line in strings.split($response, "\r\n")) {
        if (strings.startsWith($line, "* SEARCH")) {
            return $line;
        }
    }
    return "";
}

# parseSearch reads the message numbers from a SEARCH response.
func parseSearch(response as string) {
    def out as list of int init [];
    def line as string init searchLine($response);
    if (len($line) == 0) {
        return $out;
    }
    for (def tok in strings.split(strings.trim(strings.substring($line, 8)), " ")) {
        if (len(strings.trim($tok)) > 0) {
            $out[] = convert.toInt(strings.trim($tok));
        }
    }
    return $out;
}

# expectTaggedOK throws unless a tagged completion line reports "OK".
func expectTaggedOK(line as string, tag as string) {
    def rest as string init strings.substring($line, len($tag) + 1);
    if (not strings.startsWith($rest, "OK")) {
        throw Error{kind: "imap", message: strings.trim($line), file: "", line: 0, col: 0};
    }
}

# --- net dialogue (private) ----------------------------------------

# fillUntilCRLF reads from the connection until `buf` holds a CRLF (or EOF).
# The per-read idle timeout (ms), so a hung server fails instead of blocking
# forever. Re-armed before each read.
def const TIMEOUT_MS as int init 30000;

func fillUntilCRLF(conn as net.Conn, buf as string) {
    def b as string init $buf;
    while (strings.indexOf($b, "\r\n") < 0) {
        net.setDeadline($conn, TIMEOUT_MS);
        def chunk as bytes init net.readBytes($conn, 512);
        if (len($chunk) == 0) {
            return $b;
        }
        $b = $b + convert.stringFromBytes($chunk, "utf-8");
    }
    return $b;
}

# fillToLength reads until `buf` holds at least `n` characters (or EOF).
func fillToLength(conn as net.Conn, buf as string, n as int) {
    def b as string init $buf;
    while (len($b) < $n) {
        net.setDeadline($conn, TIMEOUT_MS);
        def chunk as bytes init net.readBytes($conn, 512);
        if (len($chunk) == 0) {
            return $b;
        }
        $b = $b + convert.stringFromBytes($chunk, "utf-8");
    }
    return $b;
}

# readResponse accumulates a full IMAP response - untagged lines, literals read
# by their byte count, then the tagged completion - and throws on NO / BAD.
func readResponse(conn as net.Conn, tag as string) {
    def resp as string init "";
    def buf as string init "";
    while (true) {
        $buf = fillUntilCRLF($conn, $buf);
        def nl as int init strings.indexOf($buf, "\r\n");
        if ($nl < 0) {
            return $resp;
        }
        def line as string init strings.substring($buf, 0, $nl);
        $buf = strings.substring($buf, $nl + 2);
        $resp = $resp + $line + "\r\n";
        def litlen as int init literalLength($line);
        if ($litlen >= 0) {
            $buf = fillToLength($conn, $buf, $litlen);
            $resp = $resp + strings.substring($buf, 0, $litlen);
            $buf = strings.substring($buf, $litlen);
            continue;
        }
        if (isTagged($line, $tag)) {
            expectTaggedOK($line, $tag);
            return $resp;
        }
    }
    return $resp;
}

# command sends a tagged command and returns the full response.
func command(conn as net.Conn, line as string) {
    net.writeBytes($conn, convert.bytesFromString(TAG + " " + $line + "\r\n", "utf-8"));
    return readResponse($conn, TAG);
}

# readGreeting consumes the untagged "* OK" server greeting.
func readGreeting(conn as net.Conn) {
    def buf as string init fillUntilCRLF($conn, "");
    def nl as int init strings.indexOf($buf, "\r\n");
    def line as string init $buf;
    if ($nl >= 0) {
        $line = strings.substring($buf, 0, $nl);
    }
    if (not strings.startsWith($line, "* OK")) {
        def msg as string init "greeting: " + strings.trim($line);
        throw Error{kind: "imap", message: $msg, file: "", line: 0, col: 0};
    }
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
 * Open a session: greeting, optional STARTTLS, then LOGIN (or XOAUTH2).
 * @param opts {Options} the connection and auth parameters
 * @return {Session} the open session
 * @throws {Error} on a bad greeting or a "NO" / "BAD" login completion (kind "imap")
 */
export func connect(opts as Options) {
    def conn as net.Conn init dial($opts);
    readGreeting($conn);
    if ($opts.security == "starttls") {
        command($conn, "STARTTLS");
        $conn = net.startTLS($conn);
    }
    if ($opts.auth == "xoauth2") {
        command($conn, "AUTHENTICATE XOAUTH2 " + sasl.bearer($opts.user, $opts.pass));
    } else {
        command($conn, "LOGIN " + quoteArg($opts.user) + " " + quoteArg($opts.pass));
    }
    return Session{conn: $conn};
}

/**
 * Select a mailbox (e.g. "INBOX") and return its message count.
 * @param session {Session} the open session
 * @param name {string} the mailbox name
 * @return {int} the number of messages in the mailbox
 * @throws {Error} on a "NO" / "BAD" completion (kind "imap")
 */
export func selectMailbox(session as Session, name as string) {
    return parseExists(command($session.conn, "SELECT " + quoteArg($name)));
}

/**
 * Return the sequence numbers of all messages in the selected mailbox.
 * @param session {Session} the open session
 * @return {list of int} the message sequence numbers
 * @throws {Error} on a "NO" / "BAD" completion (kind "imap")
 */
export func search(session as Session) {
    return parseSearch(command($session.conn, "SEARCH ALL"));
}

/**
 * Retrieve message `n` (its full body) as a raw string for mime.parse.
 * @param session {Session} the open session
 * @param n {int} the message sequence number
 * @return {string} the raw message body
 * @throws {Error} on a "NO" / "BAD" completion (kind "imap")
 */
export func fetch(session as Session, n as int) {
    def cmd as string init "FETCH " + convert.toString($n) + " BODY.PEEK[]";
    return extractLiteral(command($session.conn, $cmd));
}

/**
 * End the session and close the connection.
 * @param session {Session} the open session
 * @throws {Error} on a "NO" / "BAD" completion (kind "imap")
 */
export func logout(session as Session) {
    command($session.conn, "LOGOUT");
    net.close($session.conn);
}

/**
 * Connect, select `mailbox`, retrieve every message, and log out.
 * @param opts {Options} the connection and auth parameters
 * @param mailbox {string} the mailbox name
 * @return {list of string} the raw body of every message
 * @throws {Error} on a bad greeting or a "NO" / "BAD" completion (kind "imap")
 */
export func fetchAll(opts as Options, mailbox as string) {
    def session as Session init connect($opts);
    def n as int init selectMailbox($session, $mailbox);
    def msgs as list of string init [];
    def i as int init 1;
    while ($i <= $n) {
        $msgs[] = fetch($session, $i);
        $i = $i + 1;
    }
    logout($session);
    return $msgs;
}
