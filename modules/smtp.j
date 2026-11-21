# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.27.0
# pragma-jennifer-capability: net

/**
 * An SMTP send client: the line-oriented command/response dialogue of RFC 5321,
 * over the `net` system library (plaintext, implicit TLS, or STARTTLS) with
 * SASL AUTH: PLAIN, LOGIN, XOAUTH2, CRAM-MD5, and SCRAM-SHA-1 / SCRAM-SHA-256.
 * The message body is any string,
 * typically built by the `mime` module. Because it uses `net`, this module runs
 * on the default `jennifer` binary only (`jennifer-tiny` stubs the network
 * stack). `send` (the one-shot convenience) throws a catchable
 * `smtp.SmtpError` (kind "smtp") when the server rejects a command: its `code`
 * (4xx temporary, 5xx permanent), RFC 3463 `enhancedCode` and the `command`. To deliver a queue of messages over
 * a single TLS + auth handshake, `open` a persistent `Session`, `sendOn` each
 * message, and `close`. TLS certificate verification is the `net` default. An IDN
 * host or envelope domain is IDNA-encoded to its `xn--` form (via the `idna`
 * module); a non-ASCII address local part still throws (it needs SMTPUTF8).
 * @module smtp
 * @example
 * import "smtp.j" as smtp;
 * import "mime.j" as mime;
 * import "transport.j" as transport;
 * def opts as smtp.Options init smtp.Options{host: "mail.example.com",
 *     port: 587, security: transport.Security.Starttls, clientName: "me.example.com",
 *     user: "me@example.com", pass: "secret"};
 * def msg as mime.Part init mime.text("text/plain", "Hello!");
 * $msg = mime.withHeader($msg, "Subject", "Hi");
 * def rcpts as list of string init ["you@example.com"];
 * smtp.send($opts, "me@example.com", $rcpts, mime.encode($msg));
 */

use encoding;
use net;
use strings;
use convert;
use binary;
use regex;
import "./sasl.j" as sasl;
import "./idna.j" as idna;
import "./transport.j" as transport;

/**
 * Connection settings for an SMTP session.
 * @field host {string} the mail server hostname (IDNA-encoded when non-ASCII)
 * @field port {int} the server port (e.g. 25, 465, 587)
 * @field security {transport.Security} `transport.Security.None` (plaintext), `.Tls` (implicit TLS on connect), or `.Starttls` (upgrade after EHLO)
 * @field clientName {string} the EHLO identity (defaults to "localhost" when empty)
 * @field user {string} the SASL username (empty means no auth)
 * @field pass {string} the SASL password, or the OAuth2 access token for xoauth2
 * @field auth {string} the SASL mechanism: "" (default - no auth when `user` is empty, else PLAIN), "auto" (negotiate the strongest mechanism the server's EHLO advertises, falling back to PLAIN), "plain", "login", "xoauth2", "cram" (CRAM-MD5), "scram-sha-1", or "scram-sha-256"
 * @field allowInsecureAuth {bool} force SASL auth over an unencrypted ("none") connection (default false - credentials are refused over plaintext)
 */
export def struct Options {
    host as string,
    port as int,
    security as transport.Security,
    clientName as string,
    user as string,
    pass as string,
    auth as string,
    allowInsecureAuth as bool
};

/**
 * A failed SMTP exchange. Every error of kind `"smtp"` is one, so its fields are
 * safe to read once the kind matches. A server's rejection carries its reply
 * code: 4xx is temporary (`temporary` is true; retry later), 5xx permanent.
 * A connection that closes before a complete reply throws the generic kind
 * `"closed"` instead: the command's outcome is unknown.
 * @field kind {string} always `"smtp"`
 * @field message {string} `<command>: <code> <reply text>`, or what was refused locally
 * @field code {int} the server's reply code (such as 550); 0 for a local check
 * @field enhancedCode {string} the RFC 3463 enhanced status code at the start of the reply text (such as `5.1.1`), "" when there is none
 * @field command {string} the command the server rejected (`MAIL FROM`, `RCPT TO`, `DATA`, ...), "" for a local check
 * @field temporary {bool} true for a 4xx reply: the same message may be accepted later
 * @example
 * use io;
 * try {
 *     smtp.send($opts, "me@example.com", $rcpts, $body);
 * } catch (e) {
 *     if ($e.kind == "smtp" and $e.temporary) {
 *         io.printf("try again later: %s\n", $e.message);
 *     } else {
 *         throw $e;
 *     }
 * }
 */
export def struct SmtpError {
    kind as string,
    message as string,
    code as int,
    enhancedCode as string,
    command as string,
    temporary as bool
};

# localError is the SmtpError of a check made before or without a server reply.
func localError(message as string) {
    return SmtpError{
        kind: "smtp",
        message: $message,
        code: 0,
        enhancedCode: "",
        command: "",
        temporary: false
    };
}

# enhancedCodeOf reads the RFC 3463 code (`5.1.1`) after the reply code on the
# reply's last line, or "".
func enhancedCodeOf(text as string) {
    def lines as list of string init strings.split(strings.trim($text), "\n");
    def last as string init strings.trim($lines[len($lines) - 1]);
    def m as regex.Match init regex.find(
        '^[0-9]{3}[ -]([245]\.[0-9]{1,3}\.[0-9]{1,3})(\s|$)',
        $last);
    if ($m.start < 0) {
        return "";
    }
    return $m.groups[0];
}

# One parsed SMTP reply: its (final) status code and the raw reply text.
def struct Reply {
    code as int,
    text as string
};

# --- pure protocol helpers (private, unit-tested) ------------------

# parseCode reads the leading 3-digit status of a reply line, or 0.
func parseCode(line as string) {
    if (len($line) < 3) {
        return 0;
    }
    return convert.toInt($line[..3]);
}

# stripCR drops a single trailing CR.
func stripCR(line as string) {
    if (strings.endsWith($line, "\r")) {
        return $line[..len($line) - 1];
    }
    return $line;
}

# replyFinalCode returns the status code once `text` holds a complete reply
# (a terminated final line: "NNN" or "NNN " rather than a "NNN-" continuation),
# else -1. Multi-line replies use "-" after the code on every line but the last.
func replyFinalCode(text as string) {
    def lines as list of string init strings.split($text, "\n");
    def n as int init len($lines);
    def i as int init 0;
    while ($i < $n - 1) {
        def line as string init stripCR($lines[$i]);
        def spaced as bool init len($line) >= 4 and $line[3..4] == " ";
        def final as bool init len($line) == 3 or $spaced;
        if ($final) {
            return parseCode($line);
        }
        $i = $i + 1;
    }
    return -1;
}

# dotStuff prefixes an extra "." to any body line that begins with one, per the
# RFC 5321 transparency rule, so the "." end-of-data terminator is unambiguous.
func dotStuff(body as string) {
    # Collect the (possibly dot-stuffed) lines and join once: an accumulating
    # `+` over a multi-MB body would be O(N^2) before any network I/O.
    def lines as list of string init [];
    for (def line in strings.split($body, "\n")) {
        def l as string init $line;
        if (strings.startsWith($l, ".")) {
            $l = "." + $l;
        }
        $lines[] = $l;
    }
    return strings.join($lines, "\n");
}

# crlf normalises every line ending - CRLF, bare LF and bare CR - to CRLF, so
# dotStuff sees every line start: a server that ends lines on a bare CR or LF
# must not find an unstuffed "." line (SMTP smuggling).
func crlf(s as string) {
    def out as string init strings.replace($s, "\r\n", "\n");
    $out = strings.replace($out, "\r", "\n");
    return strings.replace($out, "\n", "\r\n");
}

# --- net dialogue (private) ----------------------------------------

func fail(reply as Reply, ctx as string) {
    def msg as string init "{$ctx}: {$reply.code}";
    $msg = $msg + " " + strings.trim($reply.text);
    throw SmtpError{
        kind: "smtp",
        message: $msg,
        code: $reply.code,
        enhancedCode: enhancedCodeOf($reply.text),
        command: $ctx,
        temporary: $reply.code >= 400 and $reply.code <= 499
    };
}

# expect throws unless the reply code falls in [lo, hi].
func expect(reply as Reply, lo as int, hi as int, ctx as string) {
    if ($reply.code < $lo or $reply.code > $hi) {
        fail($reply, $ctx);
    }
}

# rejectControl throws when `s` carries a control character (codepoint < 0x20, or
# DEL). CR (0x0D) and LF (0x0A) are ASCII, so `asciiOnly` lets them through - but
# on the SMTP wire a CR/LF in an address or the EHLO name injects arbitrary
# commands (a second RCPT TO, a DATA, ...). This is the guard that stops it.
func rejectControl(s as string, what as string) {
    if (regex.matches('[\x00-\x1F\x7F]', $s)) {
        throw localError($what + " contains a control character (SMTP command injection)");
    }
    return;
}

# asciiOnly returns `s` unchanged when it is ASCII, else throws: a non-ASCII
# local part (or a bare address with no domain) cannot be sent without SMTPUTF8,
# which is not yet supported.
func asciiOnly(s as string, what as string) {
    if (idna.isAscii($s)) {
        return $s;
    }
    def msg as string init $what + " is not ASCII (SMTPUTF8 not yet supported): " + $s;
    throw localError($msg);
}

# Envelope address grammar (RFC 5321 4.1.2): an unquoted local part is a
# Dot-string (atoms of atext joined by single dots), a quoted one a
# Quoted-string; the domain is LDH labels or an address literal. Anything else
# - above all `<`, `>` or a space - would let an address close its angle
# brackets and smuggle ESMTP parameters (`a@b> NOTIFY=NEVER`).
def const ATEXT as string init '[A-Za-z0-9!#$%&\x27*+/=?^_`{|}~-]+';
def const DOT_STRING as string init '^' + ATEXT + '(\.' + ATEXT + ')*$';
def const QUOTED_STRING as string init '^"([ !#-\[\]-~]|\\[ -~])*"$';
def const LABEL as string init '[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?';
def const LDH_DOMAIN as string init '^' + LABEL + '(\.' + LABEL + ')*$';
def const ADDRESS_LITERAL as string init '^\[[!-Z^-~]+\]$';

# asciiEnvelope makes an envelope address wire-safe: the domain is IDNA-encoded
# to its `xn--` form, the local part must already be ASCII, and both must match
# the RFC 5321 grammar.
func asciiEnvelope(addr as string) {
    rejectControl($addr, "envelope address");
    # RFC 5321 4.5.3.1.3: the forward/reverse path is at most 256 octets
    # including the angle brackets, so the bare address is <= 254.
    if (len($addr) == 0 or len($addr) > 254) {
        throw localError("envelope address must be 1 to 254 characters (RFC 5321): " + $addr);
    }
    # Split at the LAST '@': a quoted local part may itself contain '@', so
    # splitting at the first one would truncate the local part and mangle the
    # domain.
    def at as int init strings.lastIndexOf($addr, "@");
    if ($at < 0) {
        throw localError("envelope address is missing '@' (want local@domain, RFC 5321): " + $addr);
    }
    def local as string init $addr[..$at];
    def domain as string init $addr[$at + 1..];
    if (len($local) == 0) {
        throw localError("envelope address has an empty local part (RFC 5321): " + $addr);
    }
    if (len($domain) == 0) {
        throw localError("envelope address has an empty domain (RFC 5321): " + $addr);
    }
    def wireLocal as string init asciiOnly($local, "address local part");
    def dotted as bool init regex.matches(DOT_STRING, $wireLocal);
    if (not $dotted and not regex.matches(QUOTED_STRING, $wireLocal)) {
        throw localError(
            "envelope address local part is not a dot-string or quoted string (RFC 5321): " +
                $addr);
    }
    if (strings.startsWith($domain, "[")) {
        if (not regex.matches(ADDRESS_LITERAL, $domain)) {
            throw localError("envelope address has a malformed address literal: " + $addr);
        }
        return $wireLocal + "@" + $domain;
    }
    # An ASCII domain is checked before IDNA so a `>` or space is refused as an
    # smtp error; a Unicode one is checked once encoded.
    if (idna.isAscii($domain) and not regex.matches(LDH_DOMAIN, $domain)) {
        throw localError("envelope address domain is not a host name (RFC 5321): " + $addr);
    }
    def wireDomain as string init idna.toAscii($domain);
    if (not regex.matches(LDH_DOMAIN, $wireDomain)) {
        throw localError("envelope address domain is not a host name (RFC 5321): " + $addr);
    }
    return $wireLocal + "@" + $wireDomain;
}

# The per-read idle timeout (ms), so a hung server fails instead of blocking
# forever. Re-armed before each read.
def const TIMEOUT_MS as int init 30000;

# The connection-establishment timeout (ms): a slow / unreachable server (and
# the STARTTLS handshake) fails instead of blocking the dial forever.
def const CONNECT_TIMEOUT_MS as int init 30000;

# completeLines decodes the complete lines at the front of `raw` (everything
# through its last LF). A line feed never sits inside a multi-byte UTF-8
# sequence, so a character split across two reads stays in the undecoded rest.
func completeLines(raw as bytes) {
    def parts as list of bytes init binary.split($raw, LF);
    def n as int init len($raw) - len($parts[len($parts) - 1]);
    return encoding.decode("utf-8", $raw[0..$n]);
}

def const LF as bytes init encoding.encode("utf-8", "\n");

# readReply reads from the connection until a complete SMTP reply arrives. A
# connection that closes first - even after a partial final line such as
# "250 que" with no CRLF - throws kind "closed": the reply is not complete, so
# the command's outcome is unknown.
func readReply(conn as net.Conn) {
    # Raw chunks collect and join once: a per-read concat over a long reply is
    # O(n^2) (and re-decodes the growing buffer every read). The completion
    # check needs only the tail - a final reply line always ends the buffer -
    # so a bounded rolling tail of raw bytes is checked (bytes, so a UTF-8
    # character split across two reads is decoded only once whole), and the
    # full text is decoded once from the joined bytes when the reply completes.
    def pieces as list of bytes init [];
    def tail as bytes init binary.make(0);
    def total as int init 0;
    while (true) {
        def code as int init replyFinalCode(completeLines($tail));
        if ($code >= 0) {
            return Reply{code: $code, text: encoding.decode("utf-8", binary.join($pieces))};
        }
        net.setReadDeadline($conn, TIMEOUT_MS);
        def chunk as bytes init net.readBytes($conn, 512);
        if (len($chunk) == 0) {
            throw Error{kind: "closed", message: "smtp: connection closed before a complete reply"};
        }
        $pieces[] = $chunk;
        $tail = binary.concat($tail, $chunk);
        if (len($tail) > 2048) {
            # Trim from the front, through the first newline, so the tail still
            # starts on a complete line (a mid-line fragment could otherwise
            # read as a premature "final" code). The last, final reply line is
            # at the end, so it always survives a front-trim.
            def cut as int init binary.indexOf($tail, LF);
            if ($cut >= 0) {
                $tail = $tail[$cut + 1..];
            } else {
                $tail = binary.make(0);
            }
        }
        $total = $total + len($chunk);
        # A hostile server can stream an endless multi-line (`250-...`) reply; cap
        # the cumulative size so it fails typed instead of growing without bound.
        transport.checkReceiveSize($total, "smtp reply");
    }
    return Reply{code: -1, text: ""};
}

# command sends one line and returns the server's reply.
func command(conn as net.Conn, line as string) {
    net.writeBytes($conn, encoding.encode("utf-8", $line + "\r\n"));
    return readReply($conn);
}

func clientName(opts as Options) {
    if (len($opts.clientName) == 0) {
        return "localhost";
    }
    rejectControl($opts.clientName, "EHLO client name");
    return $opts.clientName;
}

# greet sends EHLO and returns the reply (its text carries the AUTH capability
# line); used after connect and after STARTTLS.
func greet(conn as net.Conn, opts as Options) {
    def reply as Reply init command($conn, "EHLO " + clientName($opts));
    expect($reply, 250, 259, "EHLO");
    return $reply;
}

# ehloMechs pulls the SASL mechanism names from an EHLO reply's "250[- ]AUTH
# <mech> <mech> ..." capability line (case-insensitive keyword).
func ehloMechs(text as string) {
    def out as list of string init [];
    for (def raw in strings.split($text, "\n")) {
        def line as string init strings.trim($raw);
        if (len($line) < 5) {
            continue;
        }
        def rest as string init $line[4..];
        if (strings.startsWith(strings.upper($rest), "AUTH ")) {
            for (def tk in strings.split($rest[5..], " ")) {
                if (not (strings.trim($tk) == "")) {
                    $out[] = strings.trim($tk);
                }
            }
        }
    }
    return $out;
}

# ehloAdvertises reports whether the EHLO reply lists `cap` as a capability (a
# "250[- ]CAP..." line, matched case-insensitively on the first keyword). Used
# to confirm the server actually offered STARTTLS before relying on it, so a
# MITM that strips the capability to force plaintext is caught (anti-downgrade).
func ehloAdvertises(text as string, cap as string) {
    def want as string init strings.upper($cap);
    for (def raw in strings.split($text, "\n")) {
        def line as string init strings.trim($raw);
        if (len($line) < 4) {
            continue;
        }
        def rest as string init strings.upper($line[4..]);
        def kw as string init $rest;
        def sp as int init strings.indexOf($rest, " ");
        if ($sp >= 0) {
            $kw = $rest[..$sp];
        }
        if ($kw == $want) {
            return true;
        }
    }
    return false;
}

# authenticate runs SASL per opts.auth: "" is PLAIN (when a user is set),
# "auto" picks the strongest mechanism the server's EHLO advertised (caps, else
# PLAIN), and an explicit token forces that mechanism.
func authenticate(conn as net.Conn, opts as Options, caps as string) {
    def mech as string init $opts.auth;
    if ($mech == "") {
        if (len($opts.user) == 0) {
            return;
        }
        $mech = "plain";
    } elseif ($mech == "auto") {
        if (len($opts.user) == 0) {
            return;
        }
        $mech = sasl.negotiate(ehloMechs($caps));
        if ($mech == "") {
            $mech = "plain";
        }
    }
    # A mechanism will run (the no-auth cases returned above). Refuse to hand
    # SASL credentials to the server over an unencrypted connection - "tls" and
    # a completed "starttls" upgrade are encrypted; "none" is plaintext.
    if (not transport.encrypted($opts.security) and not $opts.allowInsecureAuth) {
        throw localError("smtp: refusing to send SASL credentials over an unencrypted connection (security \"none\"); use \"tls\" or \"starttls\", or set allowInsecureAuth to force");
    }
    match ($mech) {
        when "plain" {
            def resp as string init "AUTH PLAIN " + sasl.plain($opts.user, $opts.pass);
            expect(command($conn, $resp), 235, 235, "AUTH PLAIN");
            return;
        }
        when "login" {
            expect(command($conn, "AUTH LOGIN"), 334, 334, "AUTH LOGIN");
            expect(command($conn, sasl.loginUser($opts.user)), 334, 334, "AUTH LOGIN user");
            expect(command($conn, sasl.loginPass($opts.pass)), 235, 235, "AUTH LOGIN pass");
            return;
        }
        when "xoauth2" {
            def resp as string init "AUTH XOAUTH2 " + sasl.bearer($opts.user, $opts.pass);
            expect(command($conn, $resp), 235, 235, "AUTH XOAUTH2");
            return;
        }
        when "cram" {
            def r as Reply init command($conn, "AUTH CRAM-MD5");
            expect($r, 334, 334, "AUTH CRAM-MD5");
            def resp as string init sasl.cram($opts.user, $opts.pass, saslChallenge($r.text));
            expect(command($conn, $resp), 235, 235, "CRAM-MD5 response");
            return;
        }
        when "scram-sha-1", "scram-sha-256" {
            scramAuth($conn, $opts, $mech);
            return;
        }
        else {
            def msg as string init "unknown auth mechanism: " + $mech;
            throw localError($msg);
        }
    }
}

# saslChallenge extracts the base64 payload from a SASL continuation reply
# ("334 <base64>"): the text after the status code, whitespace-trimmed.
func saslChallenge(text as string) {
    def t as string init strings.trim($text);
    def sp as int init strings.indexOf($t, " ");
    if ($sp < 0) {
        return "";
    }
    return strings.trim($t[$sp + 1..]);
}

# scramAuth runs the SCRAM exchange (RFC 5802): AUTH with the client-first,
# then client-final, then verify the server's signature off the server-final
# (a MITM without the password cannot forge it). The server-final rides on a
# 334 continuation (the client acks with an empty "=" response). A 235 straight
# after the client-final skips the server-final, so the server never proved it
# knows the password (a MITM can do that): the session is refused.
func scramAuth(conn as net.Conn, opts as Options, mech as string) {
    def algo as string init "sha256";
    def wire as string init "SCRAM-SHA-256";
    if ($mech == "scram-sha-1") {
        $algo = "sha1";
        $wire = "SCRAM-SHA-1";
    }
    def sc as sasl.Scram init sasl.scramStart($algo, $opts.user);
    def firstReply as Reply init command($conn, "AUTH " + $wire + " " + sasl.scramClientFirst($sc));
    expect($firstReply, 334, 334, $wire + " server-first");
    $sc = sasl.scramClientFinal($sc, saslChallenge($firstReply.text), $opts.pass);
    def finalReply as Reply init command($conn, sasl.scramFinalToken($sc));
    if ($finalReply.code == 334) {
        if (not sasl.scramVerify($sc, saslChallenge($finalReply.text))) {
            command($conn, "*");
            throw localError($wire + ": server signature verification failed");
        }
        expect(command($conn, "="), 235, 235, $wire + " completion");
        return;
    }
    expect($finalReply, 235, 235, $wire);
    throw localError($wire + ": server accepted without sending the server-final; " +
        "its signature was not verified, session refused");
}

func dial(opts as Options) {
    def host as string init idna.toAscii($opts.host);
    def addr as string init "{$host}:{$opts.port}";
    if ($opts.security == transport.Security.Tls) {
        return net.connectTLS($addr, CONNECT_TIMEOUT_MS);
    }
    return net.connect($addr, CONNECT_TIMEOUT_MS);
}

# --- persistent session (exported) ---------------------------------

/**
 * A live, authenticated SMTP connection: the handshake (greeting, EHLO,
 * STARTTLS, and AUTH) has run once, so `sendOn` can deliver many messages over
 * it. Value-semantic, but the `conn` handle is shared across copies (a
 * `net.Conn`), so the socket survives being passed to `sendOn`. Build with
 * `open`; retire with `close`. `close` cannot reach the caller's copy, so `open`
 * stays true afterwards; a `sendOn` on a closed session throws kind `"closed"`
 * (the socket is gone).
 * @field conn {net.Conn} the underlying (possibly TLS-upgraded) socket, shared across copies
 * @field open {bool} true for a session from `open`; `sendOn` refuses one built with false
 */
export def struct Session {
    conn as net.Conn,
    open as bool
};

/**
 * Open an authenticated SMTP session: connect per `opts.security`, read the
 * greeting, EHLO, upgrade with STARTTLS when asked (refusing a server that did
 * not advertise it - anti-downgrade), and authenticate when credentials are
 * present. The returned `Session` is ready for one or more `sendOn` calls, so a
 * queue of N messages pays this TLS + auth handshake once instead of N times.
 * @param opts {Options} the connection and auth settings
 * @return {Session} an authenticated, ready session
 * @throws {SmtpError} kind "smtp" if the connection, greeting, STARTTLS, or auth fails (including a
 *   SCRAM 235 that skips the server-final, whose signature was then never verified)
 * @throws {Error} kind "closed" when the server closes the connection before a complete reply
 */
export func open(opts as Options) {
    def conn as net.Conn init dial($opts);
    # If any handshake step throws, close the socket rather than leak it - `open`
    # never returns a half-built session. The handle id survives net.startTLS, so
    # the snapshot taken here closes the upgraded conn too.
    errdefer net.close($conn);
    expect(readReply($conn), 220, 220, "greeting");
    def caps as Reply init greet($conn, $opts);
    if ($opts.security == transport.Security.Starttls) {
        # Anti-downgrade: only issue STARTTLS if the server actually
        # advertised it. A MITM stripping the capability to keep the session
        # in plaintext is refused here rather than silently continuing.
        if (not ehloAdvertises($caps.text, "STARTTLS")) {
            throw localError("smtp: server did not advertise STARTTLS; refusing to continue (possible downgrade attack)");
        }
        expect(command($conn, "STARTTLS"), 220, 220, "STARTTLS");
        # The handle id survives net.startTLS (the upgrade swaps the registry
        # entry in place), so the Session keeps referring to the same conn.
        $conn = net.startTLS($conn, CONNECT_TIMEOUT_MS);
        $caps = greet($conn, $opts);
    }
    authenticate($conn, $opts, $caps.text);
    return Session{conn: $conn, open: true};
}

/**
 * Deliver one `message` (a full RFC 5322 message, e.g. from mime.encode) over an
 * open session: RSET, MAIL FROM / RCPT TO / DATA. The session is reusable
 * afterwards (call `sendOn` again for the next message; `close` when done).
 * @param session {Session} an open session (from `open`)
 * @param from {string} the envelope sender address
 * @param recipients {list of string} the envelope recipient addresses
 * @param message {string} the full RFC 5322 message body
 * @throws {SmtpError} kind "smtp" when the server rejects a command or an address is not a
 *   valid ASCII RFC 5321 address (or `open` is false)
 * @throws {Error} kind "closed" when the session was closed or the server closed the
 *   connection before a complete reply
 */
export func sendOn(
    session as Session,
    from as string,
    recipients as list of string,
    message as string) {
    if (not $session.open) {
        throw localError("smtp: session is closed");
    }
    # IDNA-encode envelope domains (and reject a non-ASCII local part) before
    # touching the wire.
    def sender as string init asciiEnvelope($from);
    def rcpts as list of string init [];
    for (def r in $recipients) {
        $rcpts[] = asciiEnvelope($r);
    }
    def conn as net.Conn init $session.conn;
    # Reset any prior transaction state so a reused session starts each message
    # clean (a previous aborted DATA cannot bleed into this one). Valid, and a
    # no-op, when no transaction is in progress.
    expect(command($conn, "RSET"), 250, 259, "RSET");
    expect(command($conn, "MAIL FROM:<" + $sender + ">"), 250, 259, "MAIL FROM");
    for (def rcpt in $rcpts) {
        expect(command($conn, "RCPT TO:<" + $rcpt + ">"), 250, 259, "RCPT TO");
    }
    expect(command($conn, "DATA"), 354, 354, "DATA");
    def payload as string init dotStuff(crlf($message)) + "\r\n.\r\n";
    net.writeBytes($conn, encoding.encode("utf-8", $payload));
    expect(readReply($conn), 250, 259, "end of DATA");
    return;
}

/**
 * Close a session: send QUIT (best-effort) and close the socket. Call it once;
 * the session's `open` field is not changed (value semantics), and a later
 * `sendOn` on it throws kind `"closed"`.
 * @param session {Session} the session to close
 */
export func close(session as Session) {
    if ($session.open) {
        # QUIT is courtesy: a dead connection must not turn cleanup into a throw,
        # so swallow its failure and close the socket regardless.
        try {
            command($session.conn, "QUIT");
        } catch (e) { # lint-disable: L103
        }
        net.close($session.conn);
    }
    return;
}

# --- send (exported) -----------------------------------------------

/**
 * Deliver `message` (a full RFC 5322 message, e.g. from mime.encode) to every
 * recipient, with `from` as the envelope sender - the one-shot convenience:
 * `open` a session, `sendOn` this one message, and `close`. To send several
 * messages over one handshake, call `open` / `sendOn` / `close` yourself.
 * @param opts {Options} the connection and auth settings
 * @param from {string} the envelope sender address
 * @param recipients {list of string} the envelope recipient addresses
 * @param message {string} the full RFC 5322 message body
 * @throws {SmtpError} kind "smtp" when the server rejects a command or an address is not a
 *   valid ASCII RFC 5321 address
 * @throws {Error} kind "closed" when the server closes the connection before a complete reply
 */
export func send(opts as Options, from as string, recipients as list of string, message as string) {
    def s as Session init open($opts);
    # Closed (QUIT + socket) however send exits, so a rejected command mid-DATA
    # does not leak the connection.
    defer close($s);
    sendOn($s, $from, $recipients, $message);
    return;
}
