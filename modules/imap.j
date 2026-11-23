# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.28.0
# pragma-jennifer-capability: net

/**
 * An IMAP4rev1 client (RFC 3501): tagged commands and untagged "*"
 * responses over the `net` system library, with plaintext / implicit TLS /
 * STARTTLS and auth by LOGIN, XOAUTH2, CRAM-MD5, or SCRAM-SHA-1 / SCRAM-SHA-256.
 * A practical subset covering both reading and basic folder management, not the
 * full protocol: SELECT a folder, SEARCH it (filtered), FETCH whole messages or
 * named headers, and manage messages - STORE flags, COPY, CREATE a folder,
 * mark + EXPUNGE (so, delete and move). It is not read-only.
 * Retrieved messages come back as strings for the `mime` module to parse. Uses
 * `net`, so it needs the default `jennifer` binary. A session is stateful:
 * `connect`, `selectFolder`, `search` / `fetch` / `fetchHeaders`, optional
 * `addFlags` / `expunge`, `logout`. A "NO" / "BAD" completion throws a catchable
 * `imap.ImapError` (kind "imap") with the `status`, the RFC 5530 `responseCode`
 * (`AUTHENTICATIONFAILED`, `TRYCREATE`, `OVERQUOTA`, ...) and the server's `text`;
 * so does a reply the module cannot read (a FETCH that names no such UID, a
 * SELECT without its EXISTS count, a malformed LIST / STATUS / SEARCH line). A
 * peer that closes mid-dialogue is kind "closed". One fixed command tag is used,
 * which is safe for this synchronous client (one command in flight at a time).
 * Message literals (`{N}`) are framed over bytes by their byte count, so a
 * multi-byte UTF-8 body is read byte-exact; `fetchBytes` returns any body (8-bit
 * charsets included) as raw bytes. LOGIN / XOAUTH2 credentials are refused over
 * `transport.Security.None` unless `allowInsecureAuth` is set.
 * @module imap
 * @example
 * use io;
 * import "imap.j" as imap;
 * import "mime.j" as mime;
 * import "transport.j" as transport;
 * def opts as imap.Options init imap.Options{host: "mail.example.com",
 *     port: 993, security: transport.Security.Tls, user: "me", pass: "secret"};
 * for (def raw in imap.fetchAll($opts, "INBOX")) {
 *     def msg as mime.Part init mime.parse($raw);
 *     io.printf("subject: %s\n", mime.headerValue($msg, "Subject"));
 * }
 */
use encoding;
use net;
use strings;
use convert;
use regex;
use binary;
use time;
use channel;
use math;
import "./sasl.j" as sasl;
import "./idna.j" as idna;
import "./mime.j" as mime;
import "./transport.j" as transport;

/**
 * The parameters for opening an IMAP session.
 * @field host {string} the server hostname
 * @field port {int} the server port (e.g. 993 for implicit TLS)
 * @field security {transport.Security} the transport: `transport.Security.None` (plaintext) / `.Tls` (implicit) / `.Starttls`
 * @field user {string} the login username
 * @field pass {string} the login password, or the OAuth2 access token when auth is "xoauth2"
 * @field auth {string} the auth mechanism: "" (default - LOGIN), "auto" (probe CAPABILITY and pick the strongest mechanism, falling back to LOGIN), "xoauth2", "cram" (CRAM-MD5), "scram-sha-1", or "scram-sha-256"
 * @field allowInsecureAuth {bool} send LOGIN / XOAUTH2 credentials over an unencrypted (`transport.Security.None`) connection (default false - they are refused; CRAM-MD5 and SCRAM need no opt-in)
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
 * An open IMAP session.
 * @field conn {net.Conn} the underlying connection
 * @field rx {channel of bytes} the bytes read past the last line or response, shared by every copy of the session (internal)
 */
export def struct Session {
    conn as net.Conn,
    rx as channel of bytes
};

# The single command tag. Distinctive so it never collides with response data.
def const TAG as string init "JEN";

# --- pure protocol helpers (private, unit-tested) ------------------

# rejectControl throws if s contains a control byte. RFC 3501's QUOTED-CHAR
# excludes CR and LF, so a value carrying them cannot be sent as a quoted string
# at all - unchecked it splits the command line and injects IMAP commands (OM-006).
# A single RE2 character-class scan replaces a per-rune loop (one call, no char
# list), and it runs on every command, so this is on the command hot path.
func rejectControl(s as string, what as string) {
    if (regex.matches('[\x00-\x1f\x7f]', $s)) {
        throw imapError($what + " contains a control character (IMAP command injection)");
    }
    return;
}

# quoteArg wraps a LOGIN argument as an IMAP quoted string, escaping `\` and `"`.
# Control characters (which a quoted string cannot represent) are rejected first.
func quoteArg(s as string) {
    rejectControl($s, "IMAP argument");
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
    def m as regex.Match init regex.find('\{([0-9]+)\}$', $line);
    if ($m.start < 0) {
        return -1;
    }
    return convert.toInt($m.groups[0]);
}

# Untagged is one untagged response of a raw response: its lines joined with
# every literal left out (the `{N}` markers stay in the text), and the literals
# in order.
def struct Untagged {
    text as string,
    literals as list of bytes
};

# untaggedResponses splits a raw response into its untagged responses, each
# literal framed by its byte count. A literal's content is never decoded, so an
# 8-bit body stays byte-exact; the lines around it must be UTF-8.
func untaggedResponses(raw as bytes) {
    def out as list of Untagged init [];
    def cur as Untagged init Untagged{text: "", literals: []};
    def inside as bool init false;
    def pos as int init 0;
    while ($pos < len($raw)) {
        def nl as int init binary.indexOf($raw, CRLF, $pos);
        if ($nl < 0) {
            break;
        }
        def line as string init decodeText($raw[$pos..$nl]);
        $pos = $nl + 2;
        if ($inside) {
            $cur.text = $cur.text + $line;
        } elseif (strings.startsWith($line, "* ")) {
            $cur = Untagged{text: $line, literals: []};
            $inside = true;
        } elseif (isContinuation($line)) {
            continue;
        }
        def n as int init literalLength($line);
        if ($n >= 0) {
            if ($n > len($raw) - $pos) {
                closedEarly();
            }
            if ($inside) {
                $cur.literals[] = $raw[$pos..$pos + $n];
            }
            $pos = $pos + $n;
            continue;
        }
        if ($inside) {
            $out[] = $cur;
            $inside = false;
        }
    }
    return $out;
}

# fetchResponses returns the untagged FETCH responses of a raw response that
# carry `UID <uid>`. An unsolicited FETCH for another message (a flag change a
# second client made) is left out.
func fetchResponses(raw as bytes, uid as int) {
    def out as list of Untagged init [];
    for (def u in untaggedResponses($raw)) {
        if (regex.matches('(?i)^\* [0-9]+ FETCH \(', $u.text) and
            regex.matches("(?i)[( ]UID {$uid}[ )]", $u.text)) {
            $out[] = $u;
        }
    }
    return $out;
}

# noSuchUid is the error for a FETCH whose response names no message `uid`:
# the server answers OK with no FETCH for a UID that does not exist.
func noSuchUid(uid as int) {
    return imapError("imap: no message with UID {$uid} in the selected folder");
}

# fetchBody returns the body section of `uid`'s FETCH response as bytes: its
# literal, or a quoted string / NIL (an empty section). A response without one
# throws.
func fetchBody(raw as bytes, uid as int) {
    for (def u in fetchResponses($raw, $uid)) {
        if (len($u.literals) > 0) {
            return $u.literals[0];
        }
        def m as regex.Match init regex.find(
            '(?i)BODY\[[^\]]*\](<[0-9]+>)? (NIL|"((?:[^"\\]|\\.)*)")',
            $u.text);
        if ($m.start >= 0) {
            if (strings.upper($m.groups[1]) == "NIL") {
                return emptyBytes();
            }
            return encoding.encode("utf-8", unquote($m.groups[2]));
        }
    }
    throw noSuchUid($uid);
}

# unquote undoes an IMAP quoted string's `\\` and `\"` escapes (the quotes are
# already stripped).
func unquote(s as string) {
    if (not strings.contains($s, "\\")) {
        return $s;
    }
    def out as list of string init [];
    def i as int init 0;
    while ($i < len($s)) {
        def c as string init $s[$i..$i + 1];
        if ($c == "\\" and $i + 1 < len($s)) {
            $i = $i + 1;
            $c = $s[$i..$i + 1];
        }
        $out[] = $c;
        $i = $i + 1;
    }
    return strings.join($out, "");
}

# parseExists returns the message count from the untagged "* N EXISTS" line a
# SELECT response must carry.
func parseExists(response as string) {
    for (def line in strings.split($response, "\r\n")) {
        def m as regex.Match init regex.find("^\\* ([0-9]+) EXISTS", $line);
        if ($m.start >= 0) {
            return convert.toInt($m.groups[0]);
        }
    }
    throw imapError("imap: the SELECT response has no EXISTS count");
}

# parseSearch reads the message numbers from the "* SEARCH" line a SEARCH
# response must carry (empty when nothing matched).
func parseSearch(response as string) {
    def line as string init "";
    for (def l in strings.split($response, "\r\n")) {
        if ($l == "* SEARCH" or strings.startsWith($l, "* SEARCH ")) {
            $line = $l;
            break;
        }
    }
    if (len($line) == 0) {
        throw imapError("imap: the SEARCH response has no SEARCH line");
    }
    def out as list of int init [];
    for (def tok in strings.split($line[8..], " ")) {
        if (len($tok) == 0) {
            continue;
        }
        if (not regex.matches("^[0-9]+$", $tok)) {
            throw imapError("imap: malformed SEARCH response: " + $line);
        }
        $out[] = convert.toInt($tok);
    }
    return $out;
}

/**
 * A failed IMAP operation. Every error of kind `"imap"` is one, so its fields
 * are safe to read once the kind matches.
 * @field kind {string} always `"imap"`
 * @field message {string} the server's completion line, or what went wrong
 * @field status {string} `"NO"` (the command failed) or `"BAD"` (the server did not accept it) for a tagged completion; "" for another failure
 * @field responseCode {string} the bracketed response code's name (RFC 5530: `AUTHENTICATIONFAILED`, `TRYCREATE`, `NONEXISTENT`, `OVERQUOTA`, `UNAVAILABLE`, ...), "" when none
 * @field text {string} the server's human-readable text after the code, "" for another failure
 * @example
 * try {
 *     $s = imap.connect($opts);
 * } catch (e) {
 *     if ($e.kind == "imap" and $e.responseCode == "AUTHENTICATIONFAILED") {
 *         io.printf("wrong user name or password\n");
 *     } else {
 *         throw $e;
 *     }
 * }
 */
export def struct ImapError {
    kind as string,
    message as string,
    status as string,
    responseCode as string,
    text as string
};

# imapError is the ImapError of a failure that is no tagged completion.
func imapError(message as string) {
    return ImapError{kind: "imap", message: $message, status: "", responseCode: "", text: ""};
}

# completionError reads a tagged NO / BAD completion (`TAG NO [CODE args] text`)
# into an ImapError.
func completionError(line as string, tag as string) {
    def rest as string init strings.trim($line[len($tag) + 1..]);
    def m as regex.Match init regex.find(
        '^(NO|BAD)\b\s*(\[([A-Za-z0-9.-]+)[^\]]*\])?\s*(.*)$',
        $rest);
    def err as ImapError init imapError(strings.trim($line));
    if ($m.start >= 0) {
        $err.status = $m.groups[0];
        $err.responseCode = strings.upper($m.groups[2]);
        $err.text = $m.groups[3];
    }
    return $err;
}

# expectTaggedOK throws unless a tagged completion line reports "OK".
func expectTaggedOK(line as string, tag as string) {
    def rest as string init $line[len($tag) + 1..];
    if (not strings.startsWith($rest, "OK")) {
        throw completionError($line, $tag);
    }
}

# --- net dialogue (private) ----------------------------------------

# The per-read idle timeout (ms), so a hung server fails instead of blocking
# forever. Re-armed before each read.
def const TIMEOUT_MS as int init 30000;

# CRLF is the IMAP line terminator, as bytes for the framing scans.
def const CRLF as bytes init encoding.encode("utf-8", "\r\n");

# MAX_RESPONSE_BYTES caps a single accumulated response. A literal's `{N}` byte
# count is attacker-declarable, and a malicious / compromised server could also
# stream untagged lines that never reach the tagged completion; either grows the
# read buffer without bound, so crossing the limit fails with a catchable error.
def const MAX_RESPONSE_BYTES as int init 67108864;

# capResponse throws when an accumulated response has grown past the cap.
func capResponse(n as int) {
    if ($n > MAX_RESPONSE_BYTES) {
        throw Error{
            kind: "limit",
            message: "imap: response exceeds the {MAX_RESPONSE_BYTES}-byte limit"
        };
    }
    return;
}

# --- the per-session read buffer -----------------------------------
# One socket read can return more than the line or response asked for: a push
# behind the "+ idling" continuation, a tagged completion behind the last
# untagged line. Those bytes belong to the next read, so they stay in the
# session. A Session is a value, copied on every call, so they cannot sit in a
# plain field: `rx` is a 1-slot channel every copy shares. A reader takes the
# buffer out, consumes from its front, and puts the rest back on every exit (a
# throw too); while it holds the buffer, any other reader of the session waits.
#
# IMAP literals (`{N}`) carry N raw bytes of arbitrary message content, so the
# reader frames over bytes with a forward cursor: it never re-slices the buffer
# per line (which would be O(N^2) in the response size) and never decodes a
# partial chunk to a string (which would split a multi-byte sequence).

func emptyBytes() {
    def e as bytes;
    return $e;
}

# byteSlice returns buf[start:end] as a fresh bytes value.
func byteSlice(buf as bytes, start as int, end as int) {
    return $buf[$start..$end];
}

# READ_CHUNK is the most bytes one socket read asks for.
def const READ_CHUNK as int init 4096;

# newSession wraps an open connection with an empty read buffer.
func newSession(conn as net.Conn) {
    def rx as channel of bytes init channel.make(1);
    channel.send($rx, emptyBytes());
    return Session{conn: $conn, rx: $rx};
}

# decodeText decodes response text (never a literal's content) as UTF-8.
func decodeText(b as bytes) {
    try {
        return encoding.decode("utf-8", $b);
    } catch (e) {
        throw imapError("imap: the server sent a response line that is not valid UTF-8");
    }
}

# readChunk reads the next bytes from the socket. A closed peer throws kind
# "closed", or with `eofOk` returns empty bytes. With `gap` the caller armed the
# deadline, and reaching it returns empty bytes (an idle gap is a value, not a
# throw); without it the per-read timeout is re-armed and reaching it throws
# kind "timeout".
func readChunk(conn as net.Conn, gap as bool, eofOk as bool) {
    if (not $gap) {
        net.setReadDeadline($conn, TIMEOUT_MS);
    }
    def chunk as bytes;
    try {
        $chunk = net.readBytes($conn, READ_CHUNK);
    } catch (e) {
        if ($gap and $e.kind == "timeout") {
            return emptyBytes();
        }
        if ($eofOk and $e.kind == "closed") {
            return emptyBytes();
        }
        throw $e;
    }
    if (len($chunk) == 0 and not $eofOk) {
        closedEarly();
    }
    return $chunk;
}

# Split is a response cut from the front of the session buffer: its raw bytes,
# the bytes after it, and its tagged completion line ("" when the peer closed
# first, which only LOGOUT accepts).
def struct Split {
    raw as bytes,
    rest as bytes,
    line as string
};

# readResponse accumulates a full IMAP response - untagged lines, literals read
# by their byte count, then the tagged completion - and returns it as raw bytes;
# a NO / BAD completion throws. Bytes after the completion stay in the session
# buffer, so a NO leaves the session usable. A connection that closes before the
# tagged completion throws kind "closed" (a message cut off mid-literal must not
# pass for a whole one), unless `eofOk` - LOGOUT, where a server may close right
# after its BYE. After any other failure the stream is out of step, so the
# buffer is dropped.
func readResponse(session as Session, tag as string, eofOk as bool) {
    def buf as bytes init channel.recv($session.rx);
    def r as Split;
    try {
        $r = scanResponse($session, $buf, $tag, $eofOk);
    } catch (e) {
        channel.send($session.rx, emptyBytes());
        throw $e;
    }
    channel.send($session.rx, $r.rest);
    if (len($r.line) > 0) {
        expectTaggedOK($r.line, $tag);
    }
    return $r.raw;
}

# scanResponse is readResponse's reader, starting from the buffered bytes `buf`.
func scanResponse(session as Session, buf as bytes, tag as string, eofOk as bool) {
    def pos as int init 0;
    def scanFrom as int init 0;
    while (true) {
        # Find the next CRLF at or after the cursor. The scan resumes near the
        # buffer end (1-byte overlap for a CRLF straddling a read boundary), so
        # it never rescans the whole buffer.
        def blen as int init len($buf);
        def nl as int init binary.indexOf($buf, CRLF, math.max($scanFrom, $pos));
        if ($nl < 0) {
            def chunk as bytes init readChunk($session.conn, false, $eofOk);
            if (len($chunk) == 0) {
                return Split{raw: byteSlice($buf, 0, $pos), rest: emptyBytes(), line: ""};
            }
            if ($blen == 0) {
                $buf = $chunk;
            }
            def k as int init 0;
            while ($blen > 0 and $k < len($chunk)) {
                $buf[] = $chunk[$k];
                $k = $k + 1;
            }
            capResponse(len($buf));
            $scanFrom = $blen - 1; # overlap 1 byte for a straddling CRLF
            continue;
        }
        def line as string init decodeText(byteSlice($buf, $pos, $nl));
        $pos = $nl + 2;
        $scanFrom = $pos;
        # An unexpected `+ ...` continuation (e.g. a SASL error mid-AUTHENTICATE)
        # would otherwise read as an untagged line and the loop would block until
        # timeout. Answer with an empty line so the server sends its tagged NO.
        if (isContinuation($line)) {
            net.writeBytes($session.conn, CRLF);
            continue;
        }
        if (isTagged($line, $tag)) {
            return Split{
                raw: byteSlice($buf, 0, $pos),
                rest: byteSlice($buf, $pos, len($buf)),
                line: $line
            };
        }
        def litlen as int init literalLength($line);
        if ($litlen < 0) {
            continue;
        }
        # Reject an oversized declared literal before allocating for it: a
        # malicious server's `{2000000000}` must fail catchably, not force a huge
        # net.readN allocation. A literal is exactly `litlen` bytes, read in one
        # Go call; bytes already in the buffer count toward it. A peer closing
        # mid-literal is net's kind "closed", passed on.
        capResponse($litlen);
        def need as int init $litlen - (len($buf) - $pos);
        if ($need > 0) {
            $buf = binary.concat($buf, readExact($session.conn, $need, TIMEOUT_MS));
            capResponse(len($buf));
        }
        $pos = $pos + $litlen;
        $scanFrom = $pos;
    }
    return Split{raw: emptyBytes(), rest: emptyBytes(), line: ""};
}

# closedEarly throws the error for a connection closed before the server's
# reply was complete: the generic kind "closed", not an ImapError.
func closedEarly() {
    throw Error{kind: "closed", message: "imap: connection closed before the command completed"};
}

# command sends a tagged command and returns the full response as text.
func command(session as Session, line as string) {
    return decodeText(commandRaw($session, $line));
}

# commandRaw sends a tagged command and returns the full response as raw bytes
# (for a FETCH, whose literals may hold any bytes).
func commandRaw(session as Session, line as string) {
    # Reject a control character in the whole command line at this single choke
    # point (every command passes through here), so a caller-supplied argument
    # interpolated raw - a folder name, a FETCH field list, a STORE flag list -
    # cannot inject a CR/LF and smuggle a second command (OM-006). Quoted args are
    # additionally control-checked before quoting; this is the backstop.
    rejectControl($line, "command");
    net.writeBytes($session.conn, encoding.encode("utf-8", TAG + " " + $line + "\r\n"));
    return readResponse($session, TAG, strings.upper($line) == "LOGOUT");
}

# LineRead is one line from readLineFrom; `gap` reports that the caller's
# deadline passed first (the line is then "").
def struct LineRead {
    line as string,
    gap as bool,
    rest as bytes
};

# readLineFrom reads one CRLF-terminated line (no literal handling) and leaves
# the bytes after it in the session buffer. With `gap`, the caller has armed the
# read deadline and reaching it returns a gap, the partial line kept for the next
# read; without it every read re-arms the timeout and reaching it throws. After a
# failure the stream is out of step, so the buffer is dropped.
func readLineFrom(session as Session, gap as bool) {
    def buf as bytes init channel.recv($session.rx);
    def r as LineRead;
    try {
        $r = scanLine($session, $buf, $gap);
    } catch (e) {
        channel.send($session.rx, emptyBytes());
        throw $e;
    }
    channel.send($session.rx, $r.rest);
    return $r;
}

# scanLine is readLineFrom's reader, starting from the buffered bytes `buf`.
func scanLine(session as Session, buf as bytes, gap as bool) {
    def scanFrom as int init 0;
    while (true) {
        def nl as int init binary.indexOf($buf, CRLF, $scanFrom);
        if ($nl >= 0) {
            return LineRead{
                line: decodeText(byteSlice($buf, 0, $nl)),
                gap: false,
                rest: byteSlice($buf, $nl + 2, len($buf))
            };
        }
        def blen as int init len($buf);
        $scanFrom = math.max($blen - 1, 0);
        def chunk as bytes init readChunk($session.conn, $gap, false);
        if (len($chunk) == 0) {
            return LineRead{line: "", gap: true, rest: $buf};
        }
        if ($blen == 0) {
            $buf = $chunk;
        }
        def k as int init 0;
        while ($blen > 0 and $k < len($chunk)) {
            $buf[] = $chunk[$k];
            $k = $k + 1;
        }
        capResponse(len($buf));
    }
    return LineRead{line: "", gap: true, rest: $buf};
}

# readLine reads one CRLF-terminated line: the greeting, a SASL step, the APPEND
# and IDLE continuations.
# readExact reads exactly `n` bytes in one native call; `timeoutMs` is the idle
# timeout, re-armed per underlying read, so it bounds a stall rather than the
# whole literal.
func readExact(conn as net.Conn, n as int, timeoutMs as int) {
    net.setReadDeadline($conn, 0);
    net.setIdleTimeout($conn, $timeoutMs);
    defer net.setIdleTimeout($conn, 0);
    return net.readN($conn, $n);
}

func readLine(session as Session) {
    return readLineFrom($session, false).line;
}

# skipLiteral consumes the `n`-byte literal that follows a line read by
# readLineFrom.
func skipLiteral(session as Session, n as int) {
    capResponse($n);
    def buf as bytes init channel.recv($session.rx);
    if (len($buf) >= $n) {
        channel.send($session.rx, byteSlice($buf, $n, len($buf)));
        return;
    }
    try {
        readExact($session.conn, $n - len($buf), TIMEOUT_MS);
    } catch (e) {
        channel.send($session.rx, emptyBytes());
        throw $e;
    }
    channel.send($session.rx, emptyBytes());
    return;
}

# unshiftLines puts lines back in front of the session buffer, so the next read
# returns them first.
func unshiftLines(session as Session, lines as list of string) {
    def front as bytes init encoding.encode("utf-8", strings.join($lines, "\r\n") + "\r\n");
    channel.send($session.rx, binary.concat($front, channel.recv($session.rx)));
    return;
}

# readStep reads the server's answer to a command that waits for a "+"
# continuation (AUTHENTICATE steps, APPEND): the continuation or the tagged
# completion. Untagged lines in between (a CAPABILITY or EXISTS update) are
# skipped, with their literals; anything else is a protocol error.
func readStep(session as Session, what as string) {
    def seen as int init 0;
    while (true) {
        def line as string init readLine($session);
        if (isContinuation($line) or isTagged($line, TAG)) {
            return $line;
        }
        if (not strings.startsWith($line, "* ")) {
            throw imapError("imap: unexpected line during " + $what + ": " + strings.trim($line));
        }
        def n as int init literalLength($line);
        if ($n >= 0) {
            skipLiteral($session, $n);
            $seen = $seen + $n;
        }
        $seen = $seen + len($line) + 2;
        capResponse($seen);
    }
    return "";
}

# readGreeting consumes the untagged "* OK" server greeting.
func readGreeting(session as Session) {
    def line as string init readLine($session);
    if (not strings.startsWith($line, "* OK")) {
        def msg as string init "greeting: " + strings.trim($line);
        throw imapError($msg);
    }
}

func dial(opts as Options) {
    def addr as string init idna.toAscii($opts.host) + ":{$opts.port}";
    if ($opts.security == transport.Security.Tls) {
        return net.connectWith($addr, net.ConnectOptions{timeoutMs: TIMEOUT_MS, tls: true});
    }
    return net.connectWith($addr, net.ConnectOptions{timeoutMs: TIMEOUT_MS});
}

# --- session (exported) --------------------------------------------

/**
 * Open a session: greeting, optional STARTTLS, then LOGIN (or the `auth`
 * mechanism). Over `transport.Security.None` the cleartext mechanisms (LOGIN,
 * XOAUTH2) are refused unless `allowInsecureAuth` is set.
 * @param opts {Options} the connection and auth parameters
 * @return {Session} the open session
 * @throws {ImapError} on a bad greeting, a "NO" / "BAD" login completion, LOGIN / XOAUTH2 over an
 *   unencrypted connection without `allowInsecureAuth`, bytes the server sent after its STARTTLS
 *   reply, or a SCRAM tagged OK that skips the server-final (its signature was then never
 *   verified; kind "imap")
 */
export func connect(opts as Options) {
    def conn as net.Conn init dial($opts);
    # A greeting / STARTTLS / auth failure must not leak the socket; on success
    # the caller owns the open connection. (The handle id survives net.startTLS.)
    errdefer net.close($conn);
    def session as Session init newSession($conn);
    readGreeting($session);
    if ($opts.security == transport.Security.Starttls) {
        command($session, "STARTTLS");
        # Bytes after the STARTTLS reply arrived in plaintext; read later as if
        # they came over TLS they would be an injected response.
        if (len(channel.recv($session.rx)) > 0) {
            throw imapError("imap: data after the STARTTLS reply (possible injection)");
        }
        $conn = net.startTLSWith($conn, net.ConnectOptions{timeoutMs: TIMEOUT_MS});
        $session = newSession($conn);
    }
    authenticate($session, $opts);
    return $session;
}

# writeLine sends one CRLF-terminated line.
func writeLine(conn as net.Conn, line as string) {
    net.writeBytes($conn, encoding.encode("utf-8", $line + "\r\n"));
}

# imapChallenge extracts the base64 payload from a "+ <base64>" continuation.
func imapChallenge(line as string) {
    def t as string init strings.trim($line);
    if (strings.startsWith($t, "+ ")) {
        return strings.trim($t[2..]);
    }
    return "";
}

# requirePlus throws unless the line is a "+"/"+ ..." SASL continuation: a
# tagged NO / BAD as its ImapError, anything else as a protocol error.
func requirePlus(line as string, ctx as string) {
    if (isContinuation($line)) {
        return;
    }
    if (isTagged($line, TAG)) {
        expectTaggedOK($line, TAG);
    }
    throw imapError($ctx + ": " + strings.trim($line));
}

# imapCapaMechs asks CAPABILITY and returns the mechanism names from its
# "AUTH=<mech>" capability tokens (or an empty list), so auth "" can pick the
# strongest.
func imapCapaMechs(session as Session) {
    def resp as string init command($session, "CAPABILITY");
    def out as list of string init [];
    for (def tk in strings.split(strings.replace($resp, "\r\n", " "), " ")) {
        def t as string init strings.trim($tk);
        if (strings.startsWith(strings.upper($t), "AUTH=")) {
            $out[] = $t[5..];
        }
    }
    return $out;
}

# authenticate runs the IMAP auth. "" is LOGIN; "auto" probes CAPABILITY and
# picks the strongest mechanism (falling back to LOGIN); an explicit opts.auth
# forces that mechanism. LOGIN and XOAUTH2 send the secret itself, so over a
# plaintext connection they are refused unless allowInsecureAuth (CRAM-MD5 and
# SCRAM never send the password).
func authenticate(session as Session, opts as Options) {
    def mech as string init $opts.auth;
    if ($mech == "auto") {
        $mech = sasl.negotiate(imapCapaMechs($session));
    }
    def cleartext as bool init not ($mech == "cram" or $mech == "scram-sha-1" or
        $mech == "scram-sha-256");
    if ($cleartext and not transport.encrypted($opts.security) and not $opts.allowInsecureAuth) {
        throw imapError("imap: refusing to send LOGIN / XOAUTH2 credentials over an unencrypted " +
            "connection (security None); use Tls or Starttls, or set allowInsecureAuth to force");
    }
    match ($mech) {
        when "xoauth2" {
            command($session, "AUTHENTICATE XOAUTH2 " + sasl.bearer($opts.user, $opts.pass));
            return;
        }
        when "cram" {
            writeLine($session.conn, TAG + " AUTHENTICATE CRAM-MD5");
            def chal as string init readStep($session, "AUTHENTICATE CRAM-MD5");
            requirePlus($chal, "AUTHENTICATE CRAM-MD5");
            writeLine($session.conn, sasl.cram($opts.user, $opts.pass, imapChallenge($chal)));
            readResponse($session, TAG, false);
            return;
        }
        when "scram-sha-1", "scram-sha-256" {
            scramAuth($session, $opts, $mech);
            return;
        }
        else { command($session, "LOGIN " + quoteArg($opts.user) + " " + quoteArg($opts.pass)); }
    }
}

# scramAuth runs the SCRAM exchange (RFC 5802) over IMAP AUTHENTICATE in the
# non-initial-response form (the server sends an empty "+" first). The server
# signature on the server-final is verified before the session is accepted; a
# tagged OK straight after the client-final skips the server-final, so the
# server never proved it knows the password (a MITM can do that): the session
# is refused.
func scramAuth(session as Session, opts as Options, mech as string) {
    def algo as string init "sha256";
    def wire as string init "SCRAM-SHA-256";
    if ($mech == "scram-sha-1") {
        $algo = "sha1";
        $wire = "SCRAM-SHA-1";
    }
    def what as string init "AUTHENTICATE " + $wire;
    def sc as sasl.Scram init sasl.scramStart($algo, $opts.user);
    writeLine($session.conn, TAG + " " + $what);
    requirePlus(readStep($session, $what), $wire + " initial");
    writeLine($session.conn, sasl.scramClientFirst($sc));
    def first as string init readStep($session, $what);
    requirePlus($first, $wire + " server-first");
    $sc = sasl.scramClientFinal($sc, imapChallenge($first), $opts.pass);
    writeLine($session.conn, sasl.scramFinalToken($sc));
    def final as string init readStep($session, $what);
    if (isContinuation($final)) {
        if (not sasl.scramVerify($sc, imapChallenge($final))) {
            writeLine($session.conn, "*");
            throw imapError($wire + ": server signature verification failed");
        }
        writeLine($session.conn, "");
        readResponse($session, TAG, false);
        return;
    }
    expectTaggedOK($final, TAG);
    throw imapError($wire + ": server accepted without sending the server-final; " +
        "its signature was not verified, session refused");
}

/**
 * Select a folder (e.g. "INBOX") and return its message count.
 * @param session {Session} the open session
 * @param name {string} the folder name
 * @return {int} the number of messages in the folder
 * @throws {ImapError} on a "NO" / "BAD" completion or a reply without its EXISTS count (kind "imap")
 */
export func selectFolder(session as Session, name as string) {
    return parseExists(command($session, "SELECT " + quoteArg($name)));
}

/**
 * One folder as reported by `list`.
 * @field name {string} the folder name (ASCII, or modified-UTF-7 for non-ASCII names)
 * @field delimiter {string} the hierarchy separator (e.g. "/" or "."), "" for a flat namespace
 * @field flags {list of string} the folder attribute flags (e.g. "\HasChildren", "\Noselect")
 */
export def struct Folder {
    name as string,
    delimiter as string,
    flags as list of string
};

# parseListLine parses one untagged "* LIST (flags) delim name" response into
# a Folder. The name is an atom, a quoted string, or a literal (`{N}`, its
# content in `literals`); a line that is none of these throws.
func parseListLine(line as string, literals as list of bytes) {
    def m as regex.Match init regex.find(
        '^\* LIST \(([^)]*)\) (NIL|"((?:[^"\\]|\\.)*)") (.+)$',
        $line);
    if ($m.start < 0) {
        throw imapError("imap: malformed LIST response: " + $line);
    }
    def flags as list of string init [];
    for (def f in strings.split(strings.trim($m.groups[0]), " ")) {
        if (len($f) > 0) {
            $flags[] = $f;
        }
    }
    def delim as string init "";
    if (not (strings.upper($m.groups[1]) == "NIL")) {
        $delim = unquote($m.groups[2]);
    }
    def name as string init $m.groups[3];
    if (regex.matches('^\{[0-9]+\}$', $name)) {
        if (len($literals) != 1) {
            throw imapError("imap: malformed LIST response: " + $line);
        }
        $name = decodeText($literals[0]);
    } elseif (strings.startsWith($name, "\"")) {
        if (not regex.matches('^"(?:[^"\\]|\\.)*"$', $name)) {
            throw imapError("imap: malformed LIST response: " + $line);
        }
        $name = unquote($name[1..len($name) - 1]);
    } elseif (strings.contains($name, " ")) {
        throw imapError("imap: malformed LIST response: " + $line);
    }
    return Folder{name: $name, delimiter: $delim, flags: $flags};
}

# parseList collects the Folder entries from a raw LIST response.
func parseList(raw as bytes) {
    def out as list of Folder init [];
    for (def u in untaggedResponses($raw)) {
        if (strings.startsWith(strings.upper($u.text), "* LIST ")) {
            $out[] = parseListLine($u.text, $u.literals);
        }
    }
    return $out;
}

/**
 * List the folders matching `pattern` (the IMAP `LIST "" pattern`). Use `"*"`
 * for every folder at any depth, `"%"` for the top level only, or scope to a
 * subtree with a prefix (all of Archive is the pattern `Archive/` then `*`). The
 * reference is empty, so put any prefix in the pattern. (Named `folders`, not
 * `list`, since `list` is a reserved type keyword.)
 * @param session {Session} the open session
 * @param pattern {string} the folder pattern (`*` = any depth, `%` = one level)
 * @return {list of Folder} the matching folders (name, delimiter, flags)
 * @throws {ImapError} on a "NO" / "BAD" completion or a malformed LIST line (kind "imap")
 */
export func folders(session as Session, pattern as string) {
    return parseList(commandRaw($session, "LIST \"\" " + quoteArg($pattern)));
}

/**
 * Folder status counts, as reported by `status`. A field the server did not
 * return stays 0.
 * @field messages {int} total messages (MESSAGES)
 * @field recent {int} messages with the `\Recent` flag (RECENT)
 * @field unseen {int} messages without `\Seen` (UNSEEN)
 * @field uidnext {int} the UID that will be assigned to the next message (UIDNEXT)
 * @field uidvalidity {int} the folder's UID-validity value (UIDVALIDITY)
 */
export def struct Status {
    messages as int,
    recent as int,
    unseen as int,
    uidnext as int,
    uidvalidity as int
};

# parseStatus reads the "* STATUS folder (KEY val KEY val ...)" line into a
# Status. The items may arrive in any order; only requested keys are set. The
# items are the trailing parenthesized group: `[^\r\n]*` stays on the STATUS line
# and (leftmost-longest) lands the capture on the last `(...)`, and `[^()]` keeps
# it to the innermost group, so a folder name containing a `(` can't be mistaken
# for the item list.
func parseStatus(resp as string) {
    def s as Status init Status{messages: 0, recent: 0, unseen: 0, uidnext: 0, uidvalidity: 0};
    def m as regex.Match init regex.find('(?mi)^\* STATUS [^\r\n]*\(([^()]*)\)', $resp);
    if ($m.start < 0) {
        throw imapError("imap: the STATUS response has no STATUS line");
    }
    def toks as list of string init [];
    for (def t in strings.split(strings.trim($m.groups[0]), " ")) {
        if (len($t) > 0) {
            $toks[] = $t;
        }
    }
    if (len($toks) % 2 != 0) {
        throw imapError("imap: malformed STATUS response: " + $m.groups[0]);
    }
    def i as int init 0;
    while ($i + 1 < len($toks)) {
        def key as string init strings.upper($toks[$i]);
        if (not regex.matches("^[0-9]+$", $toks[$i + 1])) {
            throw imapError("imap: malformed STATUS response: " + $m.groups[0]);
        }
        def val as int init convert.toInt($toks[$i + 1]);
        match ($key) {
            when "MESSAGES" { $s.messages = $val; }
            when "RECENT" { $s.recent = $val; }
            when "UNSEEN" { $s.unseen = $val; }
            when "UIDNEXT" { $s.uidnext = $val; }
            when "UIDVALIDITY" { $s.uidvalidity = $val; }
        }
        $i = $i + 2;
    }
    return $s;
}

/**
 * Query a folder's counts without selecting it (`STATUS`) - message / unseen /
 * recent totals plus UIDNEXT / UIDVALIDITY. Handy for a folder badge or a "new
 * mail?" poll on a folder other than the selected one.
 * @param session {Session} the open session
 * @param folder {string} the folder name
 * @return {Status} the folder counts
 * @throws {ImapError} on a "NO" / "BAD" completion or a missing / malformed STATUS line (kind "imap")
 */
export func status(session as Session, folder as string) {
    return parseStatus(command(
        $session,
        "STATUS " + quoteArg($folder) + " (MESSAGES RECENT UNSEEN UIDNEXT UIDVALIDITY)"));
}

/**
 * A message-search filter. Every field is optional; a zero-value `Criteria`
 * (from `imap.criteria()`) matches all messages, so `search($s, imap.criteria())`
 * is the old `SEARCH ALL`. Fields split into two groups by where they run:
 *
 * Server-side (mapped straight to one IMAP `SEARCH`, one round-trip, no bodies
 * downloaded; all conditions are ANDed): `subject` / `from` / `to` / `text` are
 * case-insensitive **substring** matches (`text` covers headers + body);
 * `since` / `before` are an inclusive-since / exclusive-before range as
 * `time.Time` values (a zero-value time - the default - ignores the bound). IMAP
 * `SEARCH` filters by calendar **day**, so a bound at midnight is a pure
 * server-side search; a bound that carries a **time-of-day** is additionally
 * refined to the exact instant client-side (against each candidate's
 * `INTERNALDATE`, the arrival clock `SEARCH` itself uses), so sub-day ranges just
 * work. `seen` / `unseen` / `flagged` / `answered` filter on flags; `largerThan`
 * / `smallerThan` filter on size in bytes.
 *
 * Client-side (applied only to the messages the server-side search returns, by
 * fetching just their headers or structure - never full bodies): `subjectRegex`
 * / `fromRegex` are RE2 patterns matched against the decoded `Subject` / `From`
 * header; `hasAttachments` keeps only messages whose `BODYSTRUCTURE` shows an
 * attachment (a heuristic - it looks for an `attachment` content-disposition,
 * without downloading the body).
 *
 * @field subject {string} `SUBJECT` substring ("" ignores it)
 * @field from {string} `FROM` substring
 * @field to {string} `TO` substring
 * @field text {string} `TEXT` substring (whole message)
 * @field since {time.Time} only messages on/after this instant (a zero time ignores it)
 * @field before {time.Time} only messages strictly before this instant (a zero time ignores it)
 * @field seen {bool} only `\Seen` messages
 * @field unseen {bool} only unseen messages
 * @field flagged {bool} only `\Flagged` messages
 * @field answered {bool} only `\Answered` messages
 * @field largerThan {int} `LARGER` than n bytes (0 ignores it)
 * @field smallerThan {int} `SMALLER` than n bytes (0 ignores it)
 * @field subjectRegex {string} RE2 pattern on the `Subject` header (client-side, "" ignores it)
 * @field fromRegex {string} RE2 pattern on the `From` header (client-side)
 * @field hasAttachments {bool} keep only messages with an attachment (client-side heuristic)
 */
export def struct Criteria {
    subject as string,
    from as string,
    to as string,
    text as string,
    since as time.Time,
    before as time.Time,
    seen as bool,
    unseen as bool,
    flagged as bool,
    answered as bool,
    largerThan as int,
    smallerThan as int,
    subjectRegex as string,
    fromRegex as string,
    hasAttachments as bool
};

/**
 * An empty `Criteria` (matches all messages). Set the fields you want to filter
 * on, then pass it to `search`.
 * @return {Criteria} a zero-value criteria
 */
export func criteria() {
    def c as Criteria;
    return $c;
}

# timeSet reports whether a Criteria date field was set (a non-zero time.Time).
# The zero value (an unset field) is the all-zero struct - which the Unix epoch
# also happens to be, so a `since`/`before` of exactly 1970-01-01T00:00:00Z reads
# as unset (harmless: no mail predates the epoch).
func timeSet(t as time.Time) {
    def zero as time.Time;
    return not ($t == $zero);
}

# hasTimeOfDay reports whether t carries a non-midnight wall-clock time. IMAP
# SEARCH dates are day-granular, so a time-of-day means the day-level server
# filter must be refined to the exact instant on the client (like a regex field).
func hasTimeOfDay(t as time.Time) {
    return time.hour($t) > 0 or time.minute($t) > 0 or time.second($t) > 0 or
        time.nanosecond($t) > 0;
}

# buildSearchCommand renders the server-side fields of a Criteria into one IMAP
# SEARCH command (its conditions ANDed). Substrings go through quoteArg (control-
# checked + escaped + quoted); dates are rendered from time.Time (a controlled
# dd-Mon-yyyy, no user-supplied string); sizes are ints; flags are fixed tokens -
# so no field can inject a command. No server-side field set -> "SEARCH ALL".
func buildSearchCommand(c as Criteria) {
    def toks as list of string init [];
    if (len($c.subject) > 0) {
        $toks[] = "SUBJECT " + quoteArg($c.subject);
    }
    if (len($c.from) > 0) {
        $toks[] = "FROM " + quoteArg($c.from);
    }
    if (len($c.to) > 0) {
        $toks[] = "TO " + quoteArg($c.to);
    }
    if (len($c.text) > 0) {
        $toks[] = "TEXT " + quoteArg($c.text);
    }
    # A zero-value time.Time (the default) means the bound is unset; only a date
    # the caller actually set becomes a SINCE / BEFORE token. IMAP dates are
    # day-granular (the time-of-day is dropped here); if the caller gave a
    # time-of-day it is refined client-side in matchesClientFilters.
    def hasSince as bool init timeSet($c.since);
    def hasBefore as bool init timeSet($c.before);
    # An inverted range (`since` strictly after `before`) matches nothing and is
    # almost always a mistake - reject it as a catchable error rather than
    # silently returning no messages. `since == before` (empty range) is allowed.
    if ($hasSince and $hasBefore and time.before($c.before, $c.since)) {
        throw imapError("imap.search: `since` must be on or before `before` (the date range is inverted)");
    }
    if ($hasSince) {
        # SINCE is inclusive on the calendar date, already a superset of an
        # instant-precise lower bound, so no widening is needed here.
        $toks[] = "SINCE " + time.format($c.since, "%d-%b-%Y");
    }
    if ($hasBefore) {
        # BEFORE is exclusive on the calendar date. When `before` carries a
        # time-of-day, widen to the next day so the server keeps that day's
        # earlier messages for the client-side pass to refine (a midnight
        # `before` is exact and needs no widening).
        def upper as time.Time init $c.before;
        if (hasTimeOfDay($c.before)) {
            $upper = time.add($c.before, time.fromHours(24));
        }
        $toks[] = "BEFORE " + time.format($upper, "%d-%b-%Y");
    }
    if ($c.seen) {
        $toks[] = "SEEN";
    }
    if ($c.unseen) {
        $toks[] = "UNSEEN";
    }
    if ($c.flagged) {
        $toks[] = "FLAGGED";
    }
    if ($c.answered) {
        $toks[] = "ANSWERED";
    }
    if ($c.largerThan > 0) {
        $toks[] = "LARGER {$c.largerThan}";
    }
    if ($c.smallerThan > 0) {
        $toks[] = "SMALLER {$c.smallerThan}";
    }
    if (len($toks) == 0) {
        return "SEARCH ALL";
    }
    return "SEARCH " + strings.join($toks, " ");
}

# refineSinceNeeded / refineBeforeNeeded report whether a set `since` / `before`
# carries a time-of-day the day-granular server search couldn't apply, so the
# exact instant must be checked client-side.
func refineSinceNeeded(c as Criteria) {
    return timeSet($c.since) and hasTimeOfDay($c.since);
}
func refineBeforeNeeded(c as Criteria) {
    return timeSet($c.before) and hasTimeOfDay($c.before);
}

# hasClientFilter reports whether any field needs a client-side post-filter pass.
func hasClientFilter(c as Criteria) {
    return len($c.subjectRegex) > 0 or len($c.fromRegex) > 0 or $c.hasAttachments or
        refineSinceNeeded($c) or refineBeforeNeeded($c);
}

# bodyStructureShowsAttachment applies the has-attachment heuristic to a raw
# BODYSTRUCTURE response: an attached part renders its disposition as the quoted
# atom "attachment". Structure metadata only, so no body was downloaded.
func bodyStructureShowsAttachment(resp as string) {
    return strings.contains(strings.lower($resp), "\"attachment\"");
}

# The whole module addresses messages by their stable UID (UID FETCH / STORE /
# COPY / MOVE / SEARCH), never by volatile sequence numbers: a UID keeps naming
# the same message across an expunge, so a captured id is safe to reuse within
# and across sessions. Sequence numbers appear only where the server emits them
# as data (the selectFolder count, IDLE EXISTS / EXPUNGE numbers), never as an
# addressing input.

# bodyStructureHasAttachment probes a message's structure (no body download).
func bodyStructureHasAttachment(session as Session, uid as int) {
    def raw as bytes init commandRaw($session, "UID FETCH {$uid} BODYSTRUCTURE");
    for (def u in fetchResponses($raw, $uid)) {
        if (regex.matches("(?i)BODYSTRUCTURE", $u.text)) {
            return bodyStructureShowsAttachment($u.text);
        }
    }
    throw noSuchUid($uid);
}

# parseInternalDate extracts and parses the INTERNALDATE from a FETCH response.
# IMAP renders it `"dd-Mon-yyyy HH:MM:SS +ZZZZ"`; a single-digit day is space-
# padded (` 5-Jan-...`), normalized here to a leading zero so the `%d` field parses.
func parseInternalDate(resp as string) {
    def m as regex.Match init regex.find("INTERNALDATE \"([^\"]*)\"", $resp);
    if ($m.start < 0) {
        throw imapError("imap.search: no INTERNALDATE in FETCH response");
    }
    def raw as string init $m.groups[0];
    if (strings.startsWith($raw, " ")) {
        $raw = "0" + $raw[1..];
    }
    return time.parse($raw, "%d-%b-%Y %H:%M:%S %z");
}

# messageInternalDate fetches a message's INTERNALDATE (the arrival time SINCE /
# BEFORE filter on) as a time.Time, for the exact-instant date refinement.
func messageInternalDate(session as Session, uid as int) {
    def raw as bytes init commandRaw($session, "UID FETCH {$uid} INTERNALDATE");
    for (def u in fetchResponses($raw, $uid)) {
        if (regex.matches("(?i)INTERNALDATE", $u.text)) {
            return parseInternalDate($u.text);
        }
    }
    throw noSuchUid($uid);
}

# matchesClientFilters applies the client-side criteria to one candidate message
# (addressed by UID), fetching only the headers / structure each needs.
# `refineSince` / `refineBefore` are precomputed once by the caller (they depend
# only on the criteria, not on the message).
func matchesClientFilters(
    session as Session,
    uid as int,
    c as Criteria,
    refineSince as bool,
    refineBefore as bool) {
    if (len($c.subjectRegex) > 0 or len($c.fromRegex) > 0) {
        def part as mime.Part init mime.parse(fetchHeaders($session, $uid, "SUBJECT FROM"));
        if (len($c.subjectRegex) > 0 and
            not regex.matches($c.subjectRegex, mime.headerValue($part, "Subject"))) {
            return false;
        }
        if (len($c.fromRegex) > 0 and
            not regex.matches($c.fromRegex, mime.headerValue($part, "From"))) {
            return false;
        }
    }
    # Exact-instant date refinement: the server SINCE / BEFORE filtered by calendar
    # day only, so when `since` / `before` carry a time-of-day, compare the
    # message's INTERNALDATE (the same clock SINCE / BEFORE use) to the full instant.
    if ($refineSince or $refineBefore) {
        def id as time.Time init messageInternalDate($session, $uid);
        if ($refineSince and time.before($id, $c.since)) {
            return false; # earlier than the inclusive lower bound
        }
        if ($refineBefore and not time.before($id, $c.before)) {
            return false; # not strictly before the exclusive upper bound
        }
    }
    if ($c.hasAttachments and not bodyStructureHasAttachment($session, $uid)) {
        return false;
    }
    return true;
}

/**
 * Return the **UIDs** of the messages in the selected folder matching `criteria`
 * (`UID SEARCH`). A UID is stable across an expunge - it keeps addressing the
 * same message after others are removed - so the returned ids are the correct
 * input to every other verb here and the basis for "fetch only what is new since
 * last run". The server-side fields become one round-trip (no bodies); if any
 * client-side condition is set - a `subjectRegex` / `fromRegex`, `hasAttachments`,
 * or a `since` / `before` carrying a time-of-day - the candidates are then refined
 * by fetching just their headers / structure / INTERNALDATE. An empty
 * `imap.criteria()` returns every message (`UID SEARCH ALL`).
 * @param session {Session} the open session
 * @param criteria {Criteria} the filter (build with `imap.criteria()` + fields)
 * @return {list of int} the matching message UIDs
 * @throws {ImapError} kind "imap" on a "NO" / "BAD" completion, an inverted date
 *   range (`since` after `before`), or a control character in a substring field
 */
export func search(session as Session, criteria as Criteria) {
    def uids as list of int init parseSearch(command(
        $session,
        "UID " + buildSearchCommand($criteria)));
    if (not hasClientFilter($criteria)) {
        return $uids;
    }
    # These depend only on the criteria; compute once, not per candidate.
    def refineSince as bool init refineSinceNeeded($criteria);
    def refineBefore as bool init refineBeforeNeeded($criteria);
    def out as list of int init [];
    for (def uid in $uids) {
        if (matchesClientFilters($session, $uid, $criteria, $refineSince, $refineBefore)) {
            $out[] = $uid;
        }
    }
    return $out;
}

# bodyText decodes a fetched body section, which must be UTF-8 to be a string.
func bodyText(body as bytes, uid as int, bytesVariant as string) {
    try {
        return encoding.decode("utf-8", $body);
    } catch (e) {
        throw imapError("imap: the body of message UID {$uid} is not valid UTF-8; use imap." +
            $bytesVariant + " for its raw bytes");
    }
}

/**
 * Retrieve a message (its full body) as a raw string for `mime.parse`, addressed
 * by its stable UID (`UID FETCH ... BODY.PEEK[]`). Get a UID from `search`. A
 * message that is not valid UTF-8 (an 8-bit body in a legacy charset) throws;
 * read it with `fetchBytes`.
 * @param session {Session} the open session
 * @param uid {int} the message UID
 * @return {string} the raw message body
 * @throws {ImapError} on a "NO" / "BAD" completion, no message with that UID, or a body that is not
 *   valid UTF-8 (kind "imap")
 */
export func fetch(session as Session, uid as int) {
    return bodyText(fetchBytes($session, $uid), $uid, "fetchBytes");
}

/**
 * Retrieve a message (its full body) byte-exact, whatever its charset or
 * transfer encoding (`UID FETCH ... BODY.PEEK[]`).
 * @param session {Session} the open session
 * @param uid {int} the message UID
 * @return {bytes} the raw message
 * @throws {ImapError} on a "NO" / "BAD" completion or no message with that UID (kind "imap")
 */
export func fetchBytes(session as Session, uid as int) {
    return fetchBody(commandRaw($session, "UID FETCH {$uid} BODY.PEEK[]"), $uid);
}

/**
 * Retrieve a **byte range** of a message's body (`UID FETCH ...
 * BODY.PEEK[]<offset.length>`), so a large message can be pulled in bounded
 * chunks instead of one huge literal. `offset` is the 0-based start, `length` the
 * number of octets (the server returns fewer at end-of-body). Both must be >= 0.
 * A range that is not valid UTF-8 - an 8-bit body, or a cut through a multi-byte
 * character - throws; read ranges with `fetchPartialBytes`.
 * @param session {Session} the open session
 * @param uid {int} the message UID
 * @param offset {int} the 0-based byte offset into the body
 * @param length {int} the number of octets to retrieve
 * @return {string} the requested byte range of the body
 * @throws {ImapError} kind "imap" on a bad range, a "NO" / "BAD" completion, no message with that
 *   UID, or a range that is not valid UTF-8
 */
export func fetchPartial(session as Session, uid as int, offset as int, length as int) {
    return bodyText(fetchPartialBytes($session, $uid, $offset, $length), $uid, "fetchPartialBytes");
}

/**
 * Like `fetchPartial`, but returns the byte range as raw bytes, so chunks can be
 * joined byte-exact whatever they cut through.
 * @param session {Session} the open session
 * @param uid {int} the message UID
 * @param offset {int} the 0-based byte offset into the body
 * @param length {int} the number of octets to retrieve
 * @return {bytes} the requested byte range of the body
 * @throws {ImapError} kind "imap" on a bad range, a "NO" / "BAD" completion, or no message with that
 *   UID
 */
export func fetchPartialBytes(session as Session, uid as int, offset as int, length as int) {
    if ($offset < 0 or $length < 0) {
        throw imapError("imap.fetchPartial: offset and length must be >= 0");
    }
    def cmd as string init "UID FETCH {$uid} BODY.PEEK[]<{$offset}.{$length}>";
    return fetchBody(commandRaw($session, $cmd), $uid);
}

/**
 * Fetch a message by UID and parse it into a `mime.Part` tree, ready for
 * `mime.attachments` / `mime.textBodies` / `mime.data`. Convenience for the
 * common `mime.parse(imap.fetch(...))` pattern; import `mime` too to walk the
 * result.
 * @param session {Session} the open session
 * @param uid {int} the message UID
 * @return {mime.Part} the parsed message tree
 * @throws {ImapError} as `fetch` (kind "imap")
 */
export func fetchMessage(session as Session, uid as int) {
    return mime.parse(fetch($session, $uid));
}

/**
 * Retrieve only the named header fields of a message (space-separated, e.g.
 * "SUBJECT DATE") as a raw header block for `mime.parse` - far cheaper than
 * fetching the whole body when you only need a few headers. Addressed by UID.
 * @param session {Session} the open session
 * @param uid {int} the message UID
 * @param fields {string} the space-separated header names, e.g. "SUBJECT DATE"
 * @return {string} the raw header block (the fields plus the terminating blank line)
 * @throws {ImapError} on a "NO" / "BAD" completion, no message with that UID, or headers that are
 *   not valid UTF-8 (kind "imap")
 */
export func fetchHeaders(session as Session, uid as int, fields as string) {
    # `fields` is interpolated raw (not a quoted string), so it must not carry a
    # CR/LF or the `)]` that would close the fetch item and inject a command.
    rejectControl($fields, "IMAP header field list");
    if (strings.contains($fields, ")") or strings.contains($fields, "]")) {
        throw imapError("imap.fetchHeaders: a header field list cannot contain `)` or `]`");
    }
    def cmd as string init "UID FETCH {$uid} BODY.PEEK[HEADER.FIELDS ({$fields})]";
    return bodyText(fetchBody(commandRaw($session, $cmd), $uid), $uid, "fetchBytes");
}

/**
 * Return the flags currently set on a message (by UID) as a space-separated
 * string (e.g. "\\Seen $cl_1"), or "" when none. Useful to confirm a STORE
 * actually persisted: a server that does not allow a custom keyword answers OK
 * but drops it, so the keyword will be absent here. Only the FETCH response for
 * this UID counts, not an unsolicited one for another message.
 * @param session {Session} the open session
 * @param uid {int} the message UID
 * @return {string} the flags, space-separated (no surrounding parentheses)
 * @throws {ImapError} on a "NO" / "BAD" completion or no message with that UID (kind "imap")
 */
export func flags(session as Session, uid as int) {
    def raw as bytes init commandRaw($session, "UID FETCH {$uid} (FLAGS)");
    for (def u in fetchResponses($raw, $uid)) {
        def m as regex.Match init regex.find('(?i)[( ]FLAGS \(([^)]*)\)', $u.text);
        if ($m.start >= 0) {
            return strings.trim($m.groups[0]);
        }
    }
    throw noSuchUid($uid);
}

# storeFlags runs UID STORE +/-FLAGS.SILENT (shared by addFlags / removeFlags).
func storeFlags(session as Session, uid as int, sign as string, flags as string) {
    command($session, "UID STORE {$uid} {$sign}FLAGS.SILENT ({$flags})");
    return;
}

/**
 * Add IMAP flags / keywords to a message (`UID STORE +FLAGS.SILENT`). Use a
 * keyword like "$cl_1" to colour the message in Thunderbird, or the system flag
 * "\\Deleted" to mark it for `expunge`. The folder is selected read-write by
 * `selectFolder`, so the store is permitted.
 * @param session {Session} the open session
 * @param uid {int} the message UID
 * @param flags {string} the space-separated flags, e.g. "$cl_1" or "\\Deleted"
 * @throws {ImapError} on a "NO" / "BAD" completion (kind "imap")
 */
export func addFlags(session as Session, uid as int, flags as string) {
    storeFlags($session, $uid, "+", $flags);
    return;
}

/**
 * Remove IMAP flags / keywords from a message (`UID STORE -FLAGS.SILENT`), the
 * inverse of addFlags. Removing a flag that is not set is a harmless no-op, e.g.
 * removeFlags(session, uid, "$cl_1") clears that tag, "\\Deleted" un-marks a
 * pending delete.
 * @param session {Session} the open session
 * @param uid {int} the message UID
 * @param flags {string} the space-separated flags to clear, e.g. "$cl_1"
 * @throws {ImapError} on a "NO" / "BAD" completion (kind "imap")
 */
export func removeFlags(session as Session, uid as int, flags as string) {
    storeFlags($session, $uid, "-", $flags);
    return;
}

/**
 * Create a folder (CREATE). A server answers "NO" if it already exists (often
 * with an "[ALREADYEXISTS]" response code), so wrap this in try / catch for a
 * create-if-missing.
 * @param session {Session} the open session
 * @param folder {string} the folder name to create
 * @throws {ImapError} on a "NO" / "BAD" completion (kind "imap"), including when the folder already exists
 */
export func createFolder(session as Session, folder as string) {
    command($session, "CREATE " + quoteArg($folder));
    return;
}

/**
 * Copy a message (by UID) into another folder (`UID COPY`). The source copy stays
 * until it is deleted, so the standard "move" is copy + addFlags(..., "\\Deleted")
 * + expunge - or the atomic `move` below. The destination folder must already
 * exist - COPY to a missing one is a "NO [TRYCREATE]" error.
 * @param session {Session} the open session
 * @param uid {int} the message UID
 * @param folder {string} the destination folder name
 * @throws {ImapError} on a "NO" / "BAD" completion (kind "imap")
 */
export func copy(session as Session, uid as int, folder as string) {
    command($session, "UID COPY {$uid} " + quoteArg($folder));
    return;
}

/**
 * Move a message (by UID) into another folder in one atomic step (`UID MOVE`,
 * RFC 6851) - the server copies then removes and expunges the source, so unlike
 * the copy + `\\Deleted` + expunge dance there is no window with a duplicate and
 * no manual expunge. The destination folder must already exist. `MOVE` is widely
 * but not universally supported; a server lacking it answers "BAD", so fall back
 * to `copy` + `addFlags(..., "\\Deleted")` + `expunge` when catching that.
 * @param session {Session} the open session
 * @param uid {int} the message UID
 * @param folder {string} the destination folder name
 * @throws {ImapError} on a "NO" / "BAD" completion (kind "imap"), including an unsupported MOVE
 */
export func move(session as Session, uid as int, folder as string) {
    command($session, "UID MOVE {$uid} " + quoteArg($folder));
    return;
}

# appendLiteral runs the APPEND literal-continuation dialogue: send the command
# up to the `{N}` byte count, wait for the server's `+` continuation, then send
# the exact message bytes and read the tagged completion. `flagsPart` is "" or
# " (\Seen ...)"; the folder is quoted and the whole head is control-checked, so
# no argument can inject a command (the message body is an opaque literal, so its
# own CR/LF are data, not a command boundary).
func appendLiteral(session as Session, folder as string, flagsPart as string, message as string) {
    def raw as bytes init encoding.encode("utf-8", $message);
    def head as string init TAG + " APPEND " + quoteArg($folder) + $flagsPart + ' {' +
        convert.toString(len($raw)) + '}';
    rejectControl($head, "APPEND command");
    writeLine($session.conn, $head);
    # The server answers with the `+` continuation or a tagged completion,
    # possibly after untagged status lines, which readStep skips.
    def cont as string init readStep($session, "APPEND");
    if (not isContinuation($cont)) {
        # The server refused before the literal (e.g. `NO [TRYCREATE]` for a
        # missing folder); surface its tagged completion as the error.
        expectTaggedOK($cont, TAG);
        throw imapError("imap: APPEND completed before the message was sent: " + $cont);
    }
    net.writeBytes($session.conn, $raw);
    net.writeBytes($session.conn, CRLF);
    readResponse($session, TAG, false);
    return;
}

/**
 * Upload `message` (a full RFC 5322 message - headers, a blank line, then the
 * body, e.g. built with the `mime` module) into `folder` (APPEND). The folder
 * must already exist. Use this to save a copy to "Sent" after sending, or to
 * store a message a client composed. The message is sent as a byte-counted
 * literal, so any 8-bit / multi-byte content is uploaded exactly.
 * @param session {Session} the open session
 * @param folder {string} the destination folder name
 * @param message {string} the full RFC 5322 message
 * @throws {ImapError} on a "NO" / "BAD" completion (kind "imap"), e.g. a missing folder
 */
export func append(session as Session, folder as string, message as string) {
    appendLiteral($session, $folder, "", $message);
    return;
}

/**
 * Like `append`, but sets initial flags on the stored message (APPEND with a
 * flag list) - e.g. `"\\Seen"` for a copy to Sent (already read) or `"\\Draft"`
 * for a saved draft. `flags` is a space-separated flag string, as `addFlags`.
 * @param session {Session} the open session
 * @param folder {string} the destination folder name
 * @param flags {string} the space-separated initial flags, e.g. "\\Seen"
 * @param message {string} the full RFC 5322 message
 * @throws {ImapError} on a "NO" / "BAD" completion (kind "imap")
 */
export func appendWith(session as Session, folder as string, flags as string, message as string) {
    appendLiteral($session, $folder, " (" + $flags + ")", $message);
    return;
}

/**
 * Permanently remove every message flagged "\\Deleted" in the selected folder
 * (EXPUNGE). Sequence numbers shift as messages are removed, so mark all the
 * target messages with `addFlags(..., "\\Deleted")` first and call this once.
 * @param session {Session} the open session
 * @throws {ImapError} on a "NO" / "BAD" completion (kind "imap")
 */
export func expunge(session as Session) {
    command($session, "EXPUNGE");
    return;
}

/**
 * End the session and close the connection.
 * @param session {Session} the open session
 * @throws {ImapError} on a "NO" / "BAD" completion (kind "imap")
 */
export func logout(session as Session) {
    # The socket is shut even when the LOGOUT dialogue throws (a dead server
    # must not leak the fd).
    defer net.close($session.conn);
    command($session, "LOGOUT");
}

/**
 * Connect, select `folder`, retrieve every message, and log out.
 * @param opts {Options} the connection and auth parameters
 * @param folder {string} the folder name
 * @return {list of string} the raw body of every message
 * @throws {ImapError} on a bad greeting or a "NO" / "BAD" completion (kind "imap")
 */
export func fetchAll(opts as Options, folder as string) {
    def session as Session init connect($opts);
    selectFolder($session, $folder);
    def msgs as list of string init [];
    # search(criteria()) is UID SEARCH ALL, so each id is a stable UID for fetch.
    for (def uid in search($session, criteria())) {
        $msgs[] = fetch($session, $uid);
    }
    logout($session);
    return $msgs;
}

# --- IDLE / server push (RFC 2177) ---------------------------------
# A cooperative read loop rather than callbacks: enter IDLE, then block in
# receiveNotification (or poll with a timeout) for the server's untagged pushes,
# and break out with done. The app wraps the loop in its own `spawn` when it
# wants push handling to run alongside other work. This is the
# streaming / server-push shape shared with the other read loops.

/**
 * One server-pushed mailbox change delivered during IDLE (RFC 2177). A `kind` of
 * "" is the idle-gap sentinel (`imap.pollNotification` timed out, or the server ended IDLE)
 * carrying no push.
 * @field kind {string} the push kind: "exists" (new message count), "expunge" (a message was removed), "recent" (recent-count change), or "" (no push / idle gap)
 * @field number {int} the sequence number or count the push carried (0 for the sentinel)
 */
export def struct Notification {
    kind as string,
    number as int
};

# noNotification is the idle-gap sentinel: a poll timed out or IDLE ended, so no
# push is available. The caller tests `len($n.kind) == 0`.
func noNotification() {
    return Notification{kind: "", number: 0};
}

# parseNotification turns one untagged line into a typed Notification: a
# "* N EXISTS" / "* N EXPUNGE" / "* N RECENT" push becomes {kind, number}; any
# other line (a tagged completion, a "* SEARCH", an unmodeled untagged response)
# becomes the empty sentinel. Pure: unit-tested without a server.
func parseNotification(line as string) {
    def m as regex.Match init regex.find("(?i)^\\* ([0-9]+) (EXISTS|EXPUNGE|RECENT)", $line);
    if ($m.start < 0) {
        return noNotification();
    }
    return Notification{kind: strings.lower($m.groups[1]), number: convert.toInt($m.groups[0])};
}

# isContinuation reports whether a line is a "+"/"+ ..." command continuation -
# the server's "+ idling" reply that means IDLE is active and it will now push.
# Pure: unit-tested without a server.
func isContinuation(line as string) {
    def t as string init strings.trim($line);
    return strings.startsWith($t, "+ ") or $t == "+";
}

# hasCapability reports whether a CAPABILITY response advertises `token`
# (case-insensitive, whole-token). Pure: unit-tested without a server.
func hasCapability(resp as string, token as string) {
    def want as string init strings.upper($token);
    for (def tk in strings.split(strings.replace($resp, "\r\n", " "), " ")) {
        if (strings.upper(strings.trim($tk)) == $want) {
            return true;
        }
    }
    return false;
}

/**
 * Report whether the server advertises the IDLE extension (RFC 2177) in its
 * `CAPABILITY` reply. Call before `idle`: a server without IDLE answers the
 * `IDLE` command with a tagged "NO"/"BAD", which `idle` surfaces as an `Error`.
 * @param session {Session} the open session
 * @return {bool} true when IDLE is advertised
 * @throws {ImapError} on a "NO" / "BAD" completion (kind "imap")
 */
export func supportsIdle(session as Session) {
    return hasCapability(command($session, "CAPABILITY"), "IDLE");
}

# readNotification is the shared receive core: under one read deadline
# (`timeoutMs`, 0 = block), read lines until a modeled push arrives (returned
# typed) or there is nothing to return - the deadline passed, or the tagged
# completion that ends IDLE - in which case the empty sentinel is returned. An
# untagged line the module does not model (e.g. "* n FETCH") is skipped, with
# any literal it carries. A partial line stays buffered for the next call, and
# a peer that closes throws kind "closed".
func readNotification(session as Session, timeoutMs as int) {
    # Clear the read deadline on every exit path so a timed-out poll does not leave
    # one armed for a later read on this connection (e.g. `done`'s tagged reply).
    defer net.setReadDeadline($session.conn, 0);
    net.setReadDeadline($session.conn, $timeoutMs);
    while (true) {
        def got as LineRead init readLineFrom($session, true);
        if ($got.gap) {
            return noNotification();
        }
        def n as Notification init parseNotification($got.line);
        if (len($n.kind) > 0) {
            return $n;
        }
        if (isTagged($got.line, TAG)) {
            return noNotification();
        }
        def lit as int init literalLength($got.line);
        if ($lit >= 0) {
            skipLiteral($session, $lit);
            net.setReadDeadline($session.conn, $timeoutMs);
        }
    }
    return noNotification();
}

/**
 * Enter IDLE (RFC 2177): send `IDLE` and wait for the server's "+ idling"
 * continuation, leaving the session in the idling state so the server pushes
 * mailbox changes (call `supportsIdle` first - a server that lacks the extension
 * answers with a tagged "NO"/"BAD", surfaced here as an `Error`). Follow with
 * `receiveNotification` / `pollNotification` to read pushes, then `done` to
 * return to command mode. A server drops an idle session after about 29 minutes,
 * so a long-lived client must periodically `done` and `idle` again.
 * @param session {Session} the open session
 * @throws {ImapError} kind "imap" when the server does not enter IDLE (e.g. a tagged "NO"/"BAD")
 */
export func idle(session as Session) {
    writeLine($session.conn, TAG + " IDLE");
    # A push the server sends before its "+ idling" continuation is kept for
    # receiveNotification / pollNotification.
    def early as list of string init [];
    def seen as int init 0;
    while (true) {
        def line as string init readLine($session);
        if (isContinuation($line)) {
            break;
        }
        if (not strings.startsWith($line, "* ")) {
            # Not the "+ idling" continuation - surface the server's tagged
            # completion (a "NO"/"BAD" throws; anything else is unexpected).
            if (isTagged($line, TAG)) {
                expectTaggedOK($line, TAG);
            }
            throw imapError("imap.idle: server did not enter IDLE: " + strings.trim($line));
        }
        def lit as int init literalLength($line);
        if ($lit >= 0) {
            skipLiteral($session, $lit);
            $seen = $seen + $lit;
        } else {
            $early[] = $line;
        }
        $seen = $seen + len($line) + 2;
        capResponse($seen);
    }
    if (len($early) > 0) {
        unshiftLines($session, $early);
    }
    return;
}

/**
 * Block until the next server push arrives while idling and return it as a typed
 * `Notification` ("exists" new-mail / "expunge" / "recent"). Returns the empty
 * sentinel (`kind == ""`) only if the server ends IDLE with its tagged
 * completion. No callbacks - wrap this in a loop, and in a `spawn` to run it
 * beside other work. Requires an `idle` first.
 * @param session {Session} the idling session
 * @return {Notification} the next push, or the empty sentinel when IDLE ends
 * @throws {Error} kind "closed" when the peer closes the connection; a read failure is passed on
 */
export func receiveNotification(session as Session) {
    return readNotification($session, 0);
}

/**
 * Like `receiveNotification`, but wait at most `timeoutMs` milliseconds (armed
 * with `net.setReadDeadline`): return the next push if one arrives in time, else the
 * empty sentinel (`kind == ""`) so a poll loop can do other work between checks.
 * Requires an `idle` first.
 * @param session {Session} the idling session
 * @param timeoutMs {int} the maximum time to wait, in milliseconds
 * @return {Notification} the next push, or the empty sentinel on a timeout
 * @throws {Error} kind "closed" when the peer closes the connection; a read failure other than the
 *   timeout is passed on
 */
export func pollNotification(session as Session, timeoutMs as int) {
    return readNotification($session, $timeoutMs);
}

/**
 * Leave IDLE: send `DONE`, drain any pending pushes, and read the tagged
 * completion of the `IDLE` command, returning the session to command mode so
 * ordinary calls (`fetch`, `search`, ...) work again. Pair every `idle` with a
 * `done`.
 * @param session {Session} the idling session
 * @throws {ImapError} on a "NO" / "BAD" completion (kind "imap")
 */
export func done(session as Session) {
    writeLine($session.conn, "DONE");
    # After DONE the server may flush buffered untagged pushes before the tagged
    # completion of the IDLE command; readResponse reads through them (under the
    # response cap) to that tagged line.
    readResponse($session, TAG, false);
    return;
}
