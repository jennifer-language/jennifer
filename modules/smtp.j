# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * An SMTP send client: the line-oriented command/response dialogue of RFC 5321,
 * over the `net` system library (plaintext, implicit TLS, or STARTTLS) with
 * SASL AUTH: PLAIN, LOGIN, XOAUTH2, CRAM-MD5, and SCRAM-SHA-1 / SCRAM-SHA-256.
 * The message body is any string,
 * typically built by the `mime` module. Because it uses `net`, this module runs
 * on the default `jennifer` binary only (`jennifer-tiny` stubs the network
 * stack). `send` throws a catchable `Error` (kind "smtp") when the server
 * rejects a command. TLS certificate verification is the `net` default. An IDN
 * host or envelope domain is IDNA-encoded to its `xn--` form (via the `idna`
 * module); a non-ASCII address local part still throws (it needs SMTPUTF8).
 * @module smtp
 * @example
 * def opts as smtp.Options init smtp.Options{host: "mail.example.com",
 *     port: 587, security: "starttls", clientName: "me.example.com",
 *     user: "me@example.com", pass: "secret"};
 * def msg as mime.Part init mime.text("text/plain", "Hello!");
 * $msg = mime.withHeader($msg, "Subject", "Hi");
 * def rcpts as list of string init ["you@example.com"];
 * smtp.send($opts, "me@example.com", $rcpts, mime.encode($msg));
 */

use net;
use strings;
use convert;
import "./sasl.j" as sasl;
import "./idna.j" as idna;

/**
 * Connection settings for an SMTP session.
 * @field host {string} the mail server hostname (IDNA-encoded when non-ASCII)
 * @field port {int} the server port (e.g. 25, 465, 587)
 * @field security {string} "none" (plaintext), "tls" (implicit TLS on connect), or "starttls" (upgrade after EHLO)
 * @field clientName {string} the EHLO identity (defaults to "localhost" when empty)
 * @field user {string} the SASL username (empty means no auth)
 * @field pass {string} the SASL password, or the OAuth2 access token for xoauth2
 * @field auth {string} the SASL mechanism: "" (default - no auth when `user` is empty, else PLAIN), "auto" (negotiate the strongest mechanism the server's EHLO advertises, falling back to PLAIN), "plain", "login", "xoauth2", "cram" (CRAM-MD5), "scram-sha-1", or "scram-sha-256"
 * @field allowInsecureAuth {bool} force SASL auth over an unencrypted ("none") connection (default false - credentials are refused over plaintext)
 */
export def struct Options {
    host as string,
    port as int,
    security as string,
    clientName as string,
    user as string,
    pass as string,
    auth as string,
    allowInsecureAuth as bool
};

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
    return convert.toInt(strings.substring($line, 0, 3));
}

# stripCR drops a single trailing CR.
func stripCR(line as string) {
    if (strings.endsWith($line, "\r")) {
        return strings.substring($line, 0, len($line) - 1);
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
        def spaced as bool init len($line) >= 4 and strings.substring($line, 3, 4) == " ";
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

# crlf normalises any line endings to CRLF.
func crlf(s as string) {
    def out as string init strings.replace($s, "\r\n", "\n");
    return strings.replace($out, "\n", "\r\n");
}

# --- net dialogue (private) ----------------------------------------

func fail(reply as Reply, ctx as string) {
    def msg as string init $ctx + ": " + convert.toString($reply.code);
    $msg = $msg + " " + strings.trim($reply.text);
    throw Error{kind: "smtp", message: $msg, file: "", line: 0, col: 0};
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
    for (def c in strings.chars($s)) {
        def cp as int init convert.toCodepoint($c);
        if ($cp < 32 or $cp == 127) {
            throw Error{kind: "smtp", message: $what + " contains a control character (SMTP command injection)", file: "", line: 0, col: 0};
        }
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
    throw Error{kind: "smtp", message: $msg, file: "", line: 0, col: 0};
}

# asciiEnvelope makes an envelope address wire-safe: the domain is IDNA-encoded
# to its `xn--` form, the local part must already be ASCII.
func asciiEnvelope(addr as string) {
    rejectControl($addr, "envelope address");
    # RFC 5321 4.5.3.1.3: the forward/reverse path is at most 256 octets
    # including the angle brackets, so the bare address is <= 254.
    if (len($addr) == 0 or len($addr) > 254) {
        throw Error{kind: "smtp", message: "envelope address must be 1 to 254 characters (RFC 5321): " + $addr, file: "", line: 0, col: 0};
    }
    # Split at the LAST '@': a quoted local part may itself contain '@', so
    # splitting at the first one would truncate the local part and mangle the
    # domain.
    def at as int init -1;
    def cs as list of string init strings.chars($addr);
    def ci as int init 0;
    while ($ci < len($cs)) {
        if ($cs[$ci] == "@") {
            $at = $ci;
        }
        $ci = $ci + 1;
    }
    if ($at < 0) {
        throw Error{kind: "smtp", message: "envelope address is missing '@' (want local@domain, RFC 5321): " + $addr, file: "", line: 0, col: 0};
    }
    def local as string init strings.substring($addr, 0, $at);
    def domain as string init strings.substring($addr, $at + 1, len($addr));
    if (len($local) == 0) {
        throw Error{kind: "smtp", message: "envelope address has an empty local part (RFC 5321): " + $addr, file: "", line: 0, col: 0};
    }
    if (len($domain) == 0) {
        throw Error{kind: "smtp", message: "envelope address has an empty domain (RFC 5321): " + $addr, file: "", line: 0, col: 0};
    }
    return asciiOnly($local, "address local part") + "@" + idna.toAscii($domain);
}

# The per-read idle timeout (ms), so a hung server fails instead of blocking
# forever. Re-armed before each read.
def const TIMEOUT_MS as int init 30000;

# The connection-establishment timeout (ms): a slow / unreachable server (and
# the STARTTLS handshake) fails instead of blocking the dial forever.
def const CONNECT_TIMEOUT_MS as int init 30000;

# readReply reads from the connection until a complete SMTP reply arrives.
func readReply(conn as net.Conn) {
    def buf as string init "";
    while (true) {
        def code as int init replyFinalCode($buf);
        if ($code >= 0) {
            return Reply{code: $code, text: $buf};
        }
        net.setDeadline($conn, TIMEOUT_MS);
        def chunk as bytes init net.readBytes($conn, 512);
        if (len($chunk) == 0) {
            return Reply{code: replyFinalCode($buf + "\n"), text: $buf};
        }
        $buf = $buf + convert.stringFromBytes($chunk, "utf-8");
    }
    return Reply{code: -1, text: $buf};
}

# command sends one line and returns the server's reply.
func command(conn as net.Conn, line as string) {
    net.writeBytes($conn, convert.bytesFromString($line + "\r\n", "utf-8"));
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
        def rest as string init strings.substring($line, 4, len($line));
        if (strings.startsWith(strings.upper($rest), "AUTH ")) {
            for (def tk in strings.split(strings.substring($rest, 5, len($rest)), " ")) {
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
        def rest as string init strings.upper(strings.substring($line, 4, len($line)));
        def kw as string init $rest;
        def sp as int init strings.indexOf($rest, " ");
        if ($sp >= 0) {
            $kw = strings.substring($rest, 0, $sp);
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
    if ($opts.security == "none" and not $opts.allowInsecureAuth) {
        throw Error{kind: "smtp", message: "smtp: refusing to send SASL credentials over an unencrypted connection (security \"none\"); use \"tls\" or \"starttls\", or set allowInsecureAuth to force", file: "", line: 0, col: 0};
    }
    if ($mech == "plain") {
        def resp as string init "AUTH PLAIN " + sasl.plain($opts.user, $opts.pass);
        expect(command($conn, $resp), 235, 235, "AUTH PLAIN");
        return;
    }
    if ($mech == "login") {
        expect(command($conn, "AUTH LOGIN"), 334, 334, "AUTH LOGIN");
        expect(command($conn, sasl.loginUser($opts.user)), 334, 334, "AUTH LOGIN user");
        expect(command($conn, sasl.loginPass($opts.pass)), 235, 235, "AUTH LOGIN pass");
        return;
    }
    if ($mech == "xoauth2") {
        def resp as string init "AUTH XOAUTH2 " + sasl.bearer($opts.user, $opts.pass);
        expect(command($conn, $resp), 235, 235, "AUTH XOAUTH2");
        return;
    }
    if ($mech == "cram") {
        def r as Reply init command($conn, "AUTH CRAM-MD5");
        expect($r, 334, 334, "AUTH CRAM-MD5");
        def resp as string init sasl.cram($opts.user, $opts.pass, saslChallenge($r.text));
        expect(command($conn, $resp), 235, 235, "CRAM-MD5 response");
        return;
    }
    if ($mech == "scram-sha-1" or $mech == "scram-sha-256") {
        scramAuth($conn, $opts, $mech);
        return;
    }
    def msg as string init "unknown auth mechanism: " + $mech;
    throw Error{kind: "smtp", message: $msg, file: "", line: 0, col: 0};
}

# saslChallenge extracts the base64 payload from a SASL continuation reply
# ("334 <base64>"): the text after the status code, whitespace-trimmed.
func saslChallenge(text as string) {
    def t as string init strings.trim($text);
    def sp as int init strings.indexOf($t, " ");
    if ($sp < 0) {
        return "";
    }
    return strings.trim(strings.substring($t, $sp + 1, len($t)));
}

# scramAuth runs the SCRAM exchange (RFC 5802): AUTH with the client-first,
# then client-final, then verify the server's signature off the server-final
# (a MITM without the password cannot forge it). The server-final rides on a
# 334 continuation (the client acks with an empty "=" response); a server that
# instead concludes with 235 has already accepted us.
func scramAuth(conn as net.Conn, opts as Options, mech as string) {
    def algo as string init "sha256";
    def wire as string init "SCRAM-SHA-256";
    if ($mech == "scram-sha-1") {
        $algo = "sha1";
        $wire = "SCRAM-SHA-1";
    }
    def sc as sasl.Scram init sasl.scramStart($opts.user, $algo);
    def firstReply as Reply init command($conn, "AUTH " + $wire + " " + sasl.scramClientFirst($sc));
    expect($firstReply, 334, 334, $wire + " server-first");
    $sc = sasl.scramClientFinal($sc, saslChallenge($firstReply.text), $opts.pass);
    def finalReply as Reply init command($conn, sasl.scramFinalToken($sc));
    if ($finalReply.code == 334) {
        if (not sasl.scramVerify($sc, saslChallenge($finalReply.text))) {
            command($conn, "*");
            throw Error{kind: "smtp", message: $wire + ": server signature verification failed", file: "", line: 0, col: 0};
        }
        expect(command($conn, "="), 235, 235, $wire + " completion");
        return;
    }
    expect($finalReply, 235, 235, $wire);
}

func dial(opts as Options) {
    def addr as string init idna.toAscii($opts.host) + ":" + convert.toString($opts.port);
    if ($opts.security == "tls") {
        return net.connectTLS($addr, CONNECT_TIMEOUT_MS);
    }
    return net.connect($addr, CONNECT_TIMEOUT_MS);
}

# --- send (exported) -----------------------------------------------

/**
 * Deliver `message` (a full RFC 5322 message, e.g. from mime.encode) to every
 * recipient, with `from` as the envelope sender. Opens the connection per
 * `opts.security`, greets, upgrades with STARTTLS when asked, authenticates when
 * credentials are present, then runs MAIL FROM / RCPT TO / DATA / QUIT.
 * @param opts {Options} the connection and auth settings
 * @param from {string} the envelope sender address
 * @param recipients {list of string} the envelope recipient addresses
 * @param message {string} the full RFC 5322 message body
 * @throws {Error} kind "smtp" when the server rejects a command or an address is not ASCII-safe
 */
export func send(opts as Options, from as string, recipients as list of string,
        message as string) {
    # IDNA-encode envelope domains (and reject a non-ASCII local part) before
    # opening a connection.
    def sender as string init asciiEnvelope($from);
    def rcpts as list of string init [];
    for (def r in $recipients) {
        $rcpts[] = asciiEnvelope($r);
    }
    def conn as net.Conn init dial($opts);
    # Closed however send exits, so a rejected command mid-dialogue does not
    # leak the socket. The handle id survives net.startTLS (the upgrade swaps
    # the registry entry in place), so this also closes the TLS connection.
    defer net.close($conn);
    expect(readReply($conn), 220, 220, "greeting");
    def caps as Reply init greet($conn, $opts);
    if ($opts.security == "starttls") {
        # Anti-downgrade: only issue STARTTLS if the server actually advertised
        # it. A MITM stripping the capability to keep the session in plaintext
        # is refused here rather than silently continuing unencrypted.
        if (not ehloAdvertises($caps.text, "STARTTLS")) {
            throw Error{kind: "smtp", message: "smtp: server did not advertise STARTTLS; refusing to continue (possible downgrade attack)", file: "", line: 0, col: 0};
        }
        expect(command($conn, "STARTTLS"), 220, 220, "STARTTLS");
        $conn = net.startTLS($conn, CONNECT_TIMEOUT_MS);
        $caps = greet($conn, $opts);
    }
    authenticate($conn, $opts, $caps.text);
    expect(command($conn, "MAIL FROM:<" + $sender + ">"), 250, 259, "MAIL FROM");
    for (def rcpt in $rcpts) {
        expect(command($conn, "RCPT TO:<" + $rcpt + ">"), 250, 259, "RCPT TO");
    }
    expect(command($conn, "DATA"), 354, 354, "DATA");
    def payload as string init dotStuff(crlf($message)) + "\r\n.\r\n";
    net.writeBytes($conn, convert.bytesFromString($payload, "utf-8"));
    expect(readReply($conn), 250, 259, "end of DATA");
    command($conn, "QUIT");
}
