# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

/**
 * An IMAP4rev1 receive client (RFC 3501): tagged commands and untagged "*"
 * responses over the `net` system library, with plaintext / implicit TLS /
 * STARTTLS and auth by LOGIN, XOAUTH2, CRAM-MD5, or SCRAM-SHA-1 / SCRAM-SHA-256.
 * A useful reading subset - SELECT, SEARCH, FETCH the
 * whole message - not the full protocol. Retrieved messages come back as
 * strings for the `mime` module to parse. Uses `net`, so it needs the default
 * `jennifer` binary. A session is stateful: `connect`, `selectMailbox`,
 * `search` / `fetch`, `logout`. A "NO" / "BAD" completion throws a catchable
 * `Error` (kind "imap"). One fixed command tag is used, which is safe for this
 * synchronous client (one command in flight at a time). Message literals
 * (`{N}`) are framed over bytes by their byte count, so an 8-bit / multi-byte
 * UTF-8 literal is read byte-exact.
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
 * @field auth {string} the auth mechanism: "" (default - LOGIN), "auto" (probe CAPABILITY and pick the strongest mechanism, falling back to LOGIN), "xoauth2", "cram" (CRAM-MD5), "scram-sha-1", or "scram-sha-256"
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

# MAX_RESPONSE_BYTES caps a single accumulated response. A literal's `{N}` byte
# count is attacker-declarable, and a malicious / compromised server could also
# stream untagged lines that never reach the tagged completion; either grows the
# read buffer without bound, so crossing the limit fails with a catchable error.
def const MAX_RESPONSE_BYTES as int init 67108864;

# capResponse throws when an accumulated response has grown past the cap.
func capResponse(n as int) {
    if ($n > MAX_RESPONSE_BYTES) {
        throw Error{kind: "imap", message: "imap: response exceeds the " + convert.toString(MAX_RESPONSE_BYTES) + "-byte limit", file: "", line: 0, col: 0};
    }
    return;
}

# --- byte-buffer helpers -------------------------------------------
# IMAP literals (`{N}`) carry N raw bytes of arbitrary message content, so the
# reader frames over a byte buffer with a forward cursor: it never re-slices
# the buffer (which would be O(N^2) in the message size) and never decodes a
# partial chunk to a string (which would split a multi-byte sequence). The
# response text is decoded from bytes once, after the whole response is in hand.

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

# readResponse accumulates a full IMAP response - untagged lines, literals read
# by their byte count, then the tagged completion - and throws on NO / BAD. The
# consumed prefix of `buf` (bytes 0..pos) is exactly the response; it is decoded
# to a string once, at return.
func readResponse(conn as net.Conn, tag as string) {
    def buf as bytes;
    def pos as int init 0;
    def scanFrom as int init 0;
    while (true) {
        # Find the next CRLF at or after the cursor by indexing $buf in place -
        # handing the whole growing buffer to a helper each pass would deep-copy
        # it (value semantics) and make a many-line response O(n^2). The scan
        # resumes near the buffer end (1-byte overlap for a CRLF straddling a
        # read boundary), so it never rescans the whole buffer. When more data
        # is needed we read a chunk and `continue`, re-entering this same scan.
        def blen as int init len($buf);
        def nl as int init -1;
        def si as int init $scanFrom;
        if ($si < $pos) {
            $si = $pos;
        }
        while ($si + 1 < $blen and $nl < 0) {
            if ($buf[$si] == 13 and $buf[$si + 1] == 10) {
                $nl = $si;
            }
            $si = $si + 1;
        }
        if ($nl < 0) {
            net.setDeadline($conn, TIMEOUT_MS);
            def chunk as bytes init net.readBytes($conn, 512);
            if (len($chunk) == 0) {
                return convert.stringFromBytes(byteSlice($buf, 0, $pos), "utf-8");
            }
            def k as int init 0;
            while ($k < len($chunk)) {
                $buf[] = $chunk[$k];
                $k = $k + 1;
            }
            capResponse(len($buf));
            $scanFrom = $blen - 1;   # overlap 1 byte for a straddling CRLF
            continue;
        }
        def line as string init convert.stringFromBytes(byteSlice($buf, $pos, $nl), "utf-8");
        $pos = $nl + 2;
        $scanFrom = $pos;   # next line's scan starts at the new cursor
        # An unexpected `+ ...` continuation (e.g. a SASL error mid-AUTHENTICATE)
        # would otherwise read as an untagged line and the loop would block until
        # timeout. Answer with an empty line so the server sends its tagged NO.
        if (strings.startsWith($line, "+ ") or $line == "+") {
            net.writeBytes($conn, convert.bytesFromString("\r\n", "utf-8"));
            continue;
        }
        def litlen as int init literalLength($line);
        if ($litlen >= 0) {
            # A literal is exactly `litlen` bytes; read by byte count.
            while (len($buf) - $pos < $litlen) {
                net.setDeadline($conn, TIMEOUT_MS);
                def chunk as bytes init net.readBytes($conn, 512);
                if (len($chunk) == 0) {
                    return convert.stringFromBytes(byteSlice($buf, 0, $pos), "utf-8");
                }
                def k as int init 0;
                while ($k < len($chunk)) {
                    $buf[] = $chunk[$k];
                    $k = $k + 1;
                }
                capResponse(len($buf));
            }
            $pos = $pos + $litlen;
            $scanFrom = $pos;
            continue;
        }
        if (isTagged($line, $tag)) {
            expectTaggedOK($line, $tag);
            return convert.stringFromBytes(byteSlice($buf, 0, $pos), "utf-8");
        }
    }
    return convert.stringFromBytes(byteSlice($buf, 0, $pos), "utf-8");
}

# command sends a tagged command and returns the full response.
func command(conn as net.Conn, line as string) {
    net.writeBytes($conn, convert.bytesFromString(TAG + " " + $line + "\r\n", "utf-8"));
    return readResponse($conn, TAG);
}

# readLine reads one CRLF-terminated line. No literal handling: used for the
# greeting and the SASL AUTHENTICATE dialogue, where the server sends a single
# line and waits (so there is no over-read into the next line).
func readLine(conn as net.Conn) {
    def buf as bytes;
    def nl as int init -1;
    def scanFrom as int init 0;
    while ($nl < 0) {
        net.setDeadline($conn, TIMEOUT_MS);
        def chunk as bytes init net.readBytes($conn, 512);
        if (len($chunk) == 0) {
            $nl = len($buf);
            break;
        }
        def before as int init len($buf);
        def k as int init 0;
        while ($k < len($chunk)) {
            $buf[] = $chunk[$k];
            $k = $k + 1;
        }
        capResponse(len($buf));
        # Scan from the overlap point, indexing $buf in place (a helper taking
        # the whole buffer would deep-copy it each pass).
        def blen as int init len($buf);
        def si as int init $scanFrom;
        while ($si + 1 < $blen and $nl < 0) {
            if ($buf[$si] == 13 and $buf[$si + 1] == 10) {
                $nl = $si;
            }
            $si = $si + 1;
        }
        $scanFrom = $before - 1;
        if ($scanFrom < 0) {
            $scanFrom = 0;
        }
    }
    return convert.stringFromBytes(byteSlice($buf, 0, $nl), "utf-8");
}

# readGreeting consumes the untagged "* OK" server greeting.
func readGreeting(conn as net.Conn) {
    def line as string init readLine($conn);
    if (not strings.startsWith($line, "* OK")) {
        def msg as string init "greeting: " + strings.trim($line);
        throw Error{kind: "imap", message: $msg, file: "", line: 0, col: 0};
    }
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
 * Open a session: greeting, optional STARTTLS, then LOGIN (or XOAUTH2).
 * @param opts {Options} the connection and auth parameters
 * @return {Session} the open session
 * @throws {Error} on a bad greeting or a "NO" / "BAD" login completion (kind "imap")
 */
export func connect(opts as Options) {
    def conn as net.Conn init dial($opts);
    # A greeting / STARTTLS / auth failure must not leak the socket; on success
    # the caller owns the open connection. (The handle id survives net.startTLS.)
    errdefer net.close($conn);
    readGreeting($conn);
    if ($opts.security == "starttls") {
        command($conn, "STARTTLS");
        $conn = net.startTLS($conn, TIMEOUT_MS);
    }
    authenticate($conn, $opts);
    return Session{conn: $conn};
}

# writeLine sends one CRLF-terminated line.
func writeLine(conn as net.Conn, line as string) {
    net.writeBytes($conn, convert.bytesFromString($line + "\r\n", "utf-8"));
}

# imapChallenge extracts the base64 payload from a "+ <base64>" continuation.
func imapChallenge(line as string) {
    def t as string init strings.trim($line);
    if (strings.startsWith($t, "+ ")) {
        return strings.trim(strings.substring($t, 2, len($t)));
    }
    return "";
}

# requirePlus throws unless the line is a "+"/"+ ..." SASL continuation.
func requirePlus(line as string, ctx as string) {
    def t as string init strings.trim($line);
    if (not (strings.startsWith($t, "+ ") or $t == "+")) {
        throw Error{kind: "imap", message: $ctx + ": " + $t, file: "", line: 0, col: 0};
    }
}

# imapCapaMechs asks CAPABILITY and returns the mechanism names from its
# "AUTH=<mech>" capability tokens (or an empty list), so auth "" can pick the
# strongest.
func imapCapaMechs(conn as net.Conn) {
    def resp as string init command($conn, "CAPABILITY");
    def out as list of string init [];
    for (def tk in strings.split(strings.replace($resp, "\r\n", " "), " ")) {
        def t as string init strings.trim($tk);
        if (strings.startsWith(strings.upper($t), "AUTH=")) {
            $out[] = strings.substring($t, 5, len($t));
        }
    }
    return $out;
}

# authenticate runs the IMAP auth. "" is LOGIN; "auto" probes CAPABILITY and
# picks the strongest mechanism (falling back to LOGIN); an explicit opts.auth
# forces that mechanism.
func authenticate(conn as net.Conn, opts as Options) {
    def mech as string init $opts.auth;
    if ($mech == "auto") {
        $mech = sasl.negotiate(imapCapaMechs($conn));
    }
    if ($mech == "xoauth2") {
        command($conn, "AUTHENTICATE XOAUTH2 " + sasl.bearer($opts.user, $opts.pass));
        return;
    }
    if ($mech == "cram") {
        writeLine($conn, TAG + " AUTHENTICATE CRAM-MD5");
        def chal as string init readLine($conn);
        requirePlus($chal, "AUTHENTICATE CRAM-MD5");
        writeLine($conn, sasl.cram($opts.user, $opts.pass, imapChallenge($chal)));
        expectTaggedOK(readLine($conn), TAG);
        return;
    }
    if ($mech == "scram-sha-1" or $mech == "scram-sha-256") {
        scramAuth($conn, $opts, $mech);
        return;
    }
    command($conn, "LOGIN " + quoteArg($opts.user) + " " + quoteArg($opts.pass));
}

# scramAuth runs the SCRAM exchange (RFC 5802) over IMAP AUTHENTICATE in the
# non-initial-response form (the server sends an empty "+" first). The server
# signature on the server-final is verified before the session is accepted.
func scramAuth(conn as net.Conn, opts as Options, mech as string) {
    def algo as string init "sha256";
    def wire as string init "SCRAM-SHA-256";
    if ($mech == "scram-sha-1") {
        $algo = "sha1";
        $wire = "SCRAM-SHA-1";
    }
    def sc as sasl.Scram init sasl.scramStart($opts.user, $algo);
    writeLine($conn, TAG + " AUTHENTICATE " + $wire);
    requirePlus(readLine($conn), $wire + " initial");
    writeLine($conn, sasl.scramClientFirst($sc));
    def first as string init readLine($conn);
    requirePlus($first, $wire + " server-first");
    $sc = sasl.scramClientFinal($sc, imapChallenge($first), $opts.pass);
    writeLine($conn, sasl.scramFinalToken($sc));
    def final as string init readLine($conn);
    if (strings.startsWith(strings.trim($final), "+")) {
        if (not sasl.scramVerify($sc, imapChallenge($final))) {
            writeLine($conn, "*");
            throw Error{kind: "imap", message: $wire + ": server signature verification failed", file: "", line: 0, col: 0};
        }
        writeLine($conn, "");
        expectTaggedOK(readLine($conn), TAG);
        return;
    }
    expectTaggedOK($final, TAG);
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
    # The socket is shut even when the LOGOUT dialogue throws (a dead server
    # must not leak the fd).
    defer net.close($session.conn);
    command($session.conn, "LOGOUT");
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
