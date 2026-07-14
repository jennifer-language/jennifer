# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

/**
 * An SMTP send client: the line-oriented command/response dialogue of RFC 5321,
 * over the `net` system library (plaintext, implicit TLS, or STARTTLS) with
 * SASL AUTH PLAIN (LOGIN / XOAUTH2 to follow). The message body is any string,
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
 * @field auth {string} the SASL mechanism: "" (auto - PLAIN when `user` is set, else no auth), "plain", "login", or "xoauth2"
 */
export def struct Options {
    host as string,
    port as int,
    security as string,
    clientName as string,
    user as string,
    pass as string,
    auth as string
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
    def out as string init "";
    def first as bool init true;
    for (def line in strings.split($body, "\n")) {
        if (not $first) {
            $out = $out + "\n";
        }
        $first = false;
        def l as string init $line;
        if (strings.startsWith($l, ".")) {
            $l = "." + $l;
        }
        $out = $out + $l;
    }
    return $out;
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
    def at as int init strings.indexOf($addr, "@");
    if ($at < 0) {
        return asciiOnly($addr, "address");
    }
    def local as string init strings.substring($addr, 0, $at);
    def domain as string init strings.substring($addr, $at + 1, len($addr));
    return asciiOnly($local, "address local part") + "@" + idna.toAscii($domain);
}

# The per-read idle timeout (ms), so a hung server fails instead of blocking
# forever. Re-armed before each read.
def const TIMEOUT_MS as int init 30000;

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
    return $opts.clientName;
}

# greet sends EHLO and expects a 2xx; used after connect and after STARTTLS.
func greet(conn as net.Conn, opts as Options) {
    expect(command($conn, "EHLO " + clientName($opts)), 250, 259, "EHLO");
}

# authenticate runs SASL PLAIN when credentials are set.
func authenticate(conn as net.Conn, opts as Options) {
    def mech as string init $opts.auth;
    if (len($mech) == 0) {
        if (len($opts.user) == 0) {
            return;
        }
        $mech = "plain";
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
    def msg as string init "unknown auth mechanism: " + $mech;
    throw Error{kind: "smtp", message: $msg, file: "", line: 0, col: 0};
}

func dial(opts as Options) {
    def addr as string init idna.toAscii($opts.host) + ":" + convert.toString($opts.port);
    if ($opts.security == "tls") {
        return net.connectTLS($addr);
    }
    return net.connect($addr);
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
    expect(readReply($conn), 220, 220, "greeting");
    greet($conn, $opts);
    if ($opts.security == "starttls") {
        expect(command($conn, "STARTTLS"), 220, 220, "STARTTLS");
        $conn = net.startTLS($conn);
        greet($conn, $opts);
    }
    authenticate($conn, $opts);
    expect(command($conn, "MAIL FROM:<" + $sender + ">"), 250, 259, "MAIL FROM");
    for (def rcpt in $rcpts) {
        expect(command($conn, "RCPT TO:<" + $rcpt + ">"), 250, 259, "RCPT TO");
    }
    expect(command($conn, "DATA"), 354, 354, "DATA");
    def payload as string init dotStuff(crlf($message)) + "\r\n.\r\n";
    net.writeBytes($conn, convert.bytesFromString($payload, "utf-8"));
    expect(readReply($conn), 250, 259, "end of DATA");
    command($conn, "QUIT");
    net.close($conn);
}
