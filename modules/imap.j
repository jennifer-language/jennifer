# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

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
use binary;
use time;
import "./sasl.j" as sasl;
import "./idna.j" as idna;
import "./mime.j" as mime;

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

# rejectControl throws if s contains a control byte. RFC 3501's QUOTED-CHAR
# excludes CR and LF, so a value carrying them cannot be sent as a quoted string
# at all - unchecked it splits the command line and injects IMAP commands (OM-006).
func rejectControl(s as string, what as string) {
    for (def c in strings.chars($s)) {
        def cp as int init convert.toCodepoint($c);
        if ($cp < 32 or $cp == 127) {
            throw Error{kind: "imap", message: $what + " contains a control character (IMAP command injection)", file: "", line: 0, col: 0};
        }
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
    # `n` is the literal's BYTE count, but the decoded response is a rune string,
    # so a rune-indexed slice would over-read a multi-byte body by (bytes - runes).
    # Re-encode the tail after the marker and take exactly `n` bytes (a valid
    # char boundary for a valid-UTF-8 message), so the body is returned byte-exact.
    def rest as bytes init convert.bytesFromString(strings.substring($response, $m.end, len($response)), "utf-8");
    def take as int init $n;
    if ($take > len($rest)) {
        $take = len($rest);
    }
    return convert.stringFromBytes(byteSlice($rest, 0, $take), "utf-8");
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
    return binary.slice($buf, $start, $end);
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
            # Reject an oversized declared literal before allocating for it: a
            # malicious server's `{2000000000}` must fail catchably, not force a
            # huge net.readN allocation.
            capResponse($litlen);
            # A literal is exactly `litlen` bytes. Read the remaining count in
            # one Go call (net.readN) instead of a per-byte accumulation; bytes
            # already read ahead into $buf count toward it. A peer that closes
            # mid-literal returns the partial response (as the old loop did).
            def need as int init $litlen - (len($buf) - $pos);
            if ($need > 0) {
                try {
                    $buf = binary.concat($buf, net.readN($conn, $need, TIMEOUT_MS));
                } catch (e) {
                    if (strings.contains($e.message, "closed after")) {
                        return convert.stringFromBytes(byteSlice($buf, 0, $pos), "utf-8");
                    }
                    throw $e;
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
    # Reject a control character in the whole command line at this single choke
    # point (every command passes through here), so a caller-supplied argument
    # interpolated raw - a folder name, a FETCH field list, a STORE flag list -
    # cannot inject a CR/LF and smuggle a second command (OM-006). Quoted args are
    # additionally control-checked before quoting; this is the backstop.
    rejectControl($line, "command");
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
 * Select a folder (e.g. "INBOX") and return its message count.
 * @param session {Session} the open session
 * @param name {string} the folder name
 * @return {int} the number of messages in the folder
 * @throws {Error} on a "NO" / "BAD" completion (kind "imap")
 */
export func selectFolder(session as Session, name as string) {
    return parseExists(command($session.conn, "SELECT " + quoteArg($name)));
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

# parseListLine parses one untagged "* LIST (flags) delim name" line into a
# Folder. Handles a quoted or atom name (the common cases); a literal-encoded
# name (rare - non-ASCII uses modified-UTF-7, which stays ASCII) is returned raw.
func parseListLine(line as string) {
    def m as regex.Match init regex.find("^\\* LIST \\(([^)]*)\\) (NIL|\"([^\"]*)\") (.+)$", $line);
    def flags as list of string init [];
    if ($m.start < 0) {
        return Folder{ name: "", delimiter: "", flags: $flags };
    }
    for (def f in strings.split(strings.trim($m.groups[0]), " ")) {
        if (len(strings.trim($f)) > 0) {
            $flags[] = strings.trim($f);
        }
    }
    def delim as string init "";
    if (not ($m.groups[1] == "NIL")) {
        $delim = $m.groups[2];
    }
    def name as string init strings.trim($m.groups[3]);
    if (strings.startsWith($name, "\"") and strings.endsWith($name, "\"") and len($name) >= 2) {
        $name = strings.substring($name, 1, len($name) - 1);
        $name = strings.replace($name, "\\\"", "\"");
        $name = strings.replace($name, "\\\\", "\\");
    }
    return Folder{ name: $name, delimiter: $delim, flags: $flags };
}

# parseList collects the Folder entries from a LIST response.
func parseList(resp as string) {
    def out as list of Folder init [];
    for (def line in strings.split($resp, "\r\n")) {
        if (strings.startsWith($line, "* LIST ")) {
            $out[] = parseListLine($line);
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
 * @throws {Error} on a "NO" / "BAD" completion (kind "imap")
 */
export func folders(session as Session, pattern as string) {
    return parseList(command($session.conn, "LIST \"\" " + quoteArg($pattern)));
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
    def s as Status init Status{ messages: 0, recent: 0, unseen: 0, uidnext: 0, uidvalidity: 0 };
    def m as regex.Match init regex.find("STATUS [^\r\n]*\\(([^()]*)\\)", $resp);
    if ($m.start < 0) {
        return $s;
    }
    def toks as list of string init strings.split(strings.trim($m.groups[0]), " ");
    def i as int init 0;
    while ($i + 1 < len($toks)) {
        def key as string init strings.upper(strings.trim($toks[$i]));
        def val as int init convert.toInt(strings.trim($toks[$i + 1]));
        if ($key == "MESSAGES") {
            $s.messages = $val;
        } elseif ($key == "RECENT") {
            $s.recent = $val;
        } elseif ($key == "UNSEEN") {
            $s.unseen = $val;
        } elseif ($key == "UIDNEXT") {
            $s.uidnext = $val;
        } elseif ($key == "UIDVALIDITY") {
            $s.uidvalidity = $val;
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
 * @throws {Error} on a "NO" / "BAD" completion (kind "imap")
 */
export func status(session as Session, folder as string) {
    return parseStatus(command($session.conn, "STATUS " + quoteArg($folder) + " (MESSAGES RECENT UNSEEN UIDNEXT UIDVALIDITY)"));
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
    return time.hour($t) > 0 or time.minute($t) > 0 or time.second($t) > 0 or time.nanosecond($t) > 0;
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
        throw Error{kind: "imap", message: "imap.search: `since` must be on or before `before` (the date range is inverted)", file: "", line: 0, col: 0};
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
        $toks[] = "LARGER " + convert.toString($c.largerThan);
    }
    if ($c.smallerThan > 0) {
        $toks[] = "SMALLER " + convert.toString($c.smallerThan);
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
    return len($c.subjectRegex) > 0 or len($c.fromRegex) > 0 or $c.hasAttachments
        or refineSinceNeeded($c) or refineBeforeNeeded($c);
}

# bodyStructureShowsAttachment applies the has-attachment heuristic to a raw
# BODYSTRUCTURE response: an attached part renders its disposition as the quoted
# atom "attachment". Structure metadata only, so no body was downloaded.
func bodyStructureShowsAttachment(resp as string) {
    return strings.contains(strings.lower($resp), "\"attachment\"");
}

# bodyStructureHasAttachment probes message n's structure (no body download).
func bodyStructureHasAttachment(session as Session, n as int) {
    def resp as string init command($session.conn, "FETCH " + convert.toString($n) + " BODYSTRUCTURE");
    return bodyStructureShowsAttachment($resp);
}

# parseInternalDate extracts and parses the INTERNALDATE from a FETCH response.
# IMAP renders it `"dd-Mon-yyyy HH:MM:SS +ZZZZ"`; a single-digit day is space-
# padded (` 5-Jan-...`), normalized here to a leading zero so the `%d` field parses.
func parseInternalDate(resp as string) {
    def m as regex.Match init regex.find("INTERNALDATE \"([^\"]*)\"", $resp);
    if ($m.start < 0) {
        throw Error{kind: "imap", message: "imap.search: no INTERNALDATE in FETCH response", file: "", line: 0, col: 0};
    }
    def raw as string init $m.groups[0];
    if (strings.startsWith($raw, " ")) {
        $raw = "0" + strings.substring($raw, 1, len($raw));
    }
    return time.parse($raw, "%d-%b-%Y %H:%M:%S %z");
}

# messageInternalDate fetches message n's INTERNALDATE (the arrival time SINCE /
# BEFORE filter on) as a time.Time, for the exact-instant date refinement.
func messageInternalDate(session as Session, n as int) {
    def resp as string init command($session.conn, "FETCH " + convert.toString($n) + " INTERNALDATE");
    return parseInternalDate($resp);
}

# matchesClientFilters applies the client-side criteria to one candidate message,
# fetching only the headers / structure each needs. `refineSince` / `refineBefore`
# are precomputed once by the caller (they depend only on the criteria, not on n).
func matchesClientFilters(session as Session, n as int, c as Criteria, refineSince as bool, refineBefore as bool) {
    if (len($c.subjectRegex) > 0 or len($c.fromRegex) > 0) {
        def part as mime.Part init mime.parse(fetchHeaders($session, $n, "SUBJECT FROM"));
        if (len($c.subjectRegex) > 0 and not regex.matches($c.subjectRegex, mime.headerValue($part, "Subject"))) {
            return false;
        }
        if (len($c.fromRegex) > 0 and not regex.matches($c.fromRegex, mime.headerValue($part, "From"))) {
            return false;
        }
    }
    # Exact-instant date refinement: the server SINCE / BEFORE filtered by calendar
    # day only, so when `since` / `before` carry a time-of-day, compare the
    # message's INTERNALDATE (the same clock SINCE / BEFORE use) to the full instant.
    if ($refineSince or $refineBefore) {
        def id as time.Time init messageInternalDate($session, $n);
        if ($refineSince and time.before($id, $c.since)) {
            return false;    # earlier than the inclusive lower bound
        }
        if ($refineBefore and not time.before($id, $c.before)) {
            return false;    # not strictly before the exclusive upper bound
        }
    }
    if ($c.hasAttachments and not bodyStructureHasAttachment($session, $n)) {
        return false;
    }
    return true;
}

/**
 * Return the sequence numbers of the messages in the selected folder matching
 * `criteria`. The server-side fields become one IMAP `SEARCH` (one round-trip,
 * no bodies); if any client-side condition is set - a `subjectRegex` / `fromRegex`,
 * `hasAttachments`, or a `since` / `before` carrying a time-of-day - the server's
 * candidates are then refined by fetching just their headers / structure /
 * INTERNALDATE. An empty `imap.criteria()` returns every message (`SEARCH ALL`).
 * @param session {Session} the open session
 * @param criteria {Criteria} the filter (build with `imap.criteria()` + fields)
 * @return {list of int} the matching message sequence numbers
 * @throws {Error} kind "imap" on a "NO" / "BAD" completion, an inverted date
 *   range (`since` after `before`), or a control character in a substring field
 */
export func search(session as Session, criteria as Criteria) {
    def nums as list of int init parseSearch(command($session.conn, buildSearchCommand($criteria)));
    if (not hasClientFilter($criteria)) {
        return $nums;
    }
    # These depend only on the criteria; compute once, not per candidate.
    def refineSince as bool init refineSinceNeeded($criteria);
    def refineBefore as bool init refineBeforeNeeded($criteria);
    def out as list of int init [];
    for (def n in $nums) {
        if (matchesClientFilters($session, $n, $criteria, $refineSince, $refineBefore)) {
            $out[] = $n;
        }
    }
    return $out;
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
 * Fetch message `n` and parse it into a `mime.Part` tree, ready for
 * `mime.attachments` / `mime.textBodies` / `mime.data`. Convenience for the
 * common `mime.parse(imap.fetch(...))` pattern; import `mime` too to walk the
 * result.
 * @param session {Session} the open session
 * @param n {int} the message sequence number
 * @return {mime.Part} the parsed message tree
 * @throws {Error} on a "NO" / "BAD" completion (kind "imap")
 */
export func fetchMessage(session as Session, n as int) {
    return mime.parse(fetch($session, $n));
}

/**
 * Retrieve only the named header fields of message `n` (space-separated, e.g.
 * "SUBJECT DATE") as a raw header block for `mime.parse` - far cheaper than
 * fetching the whole body when you only need a few headers.
 * @param session {Session} the open session
 * @param n {int} the message sequence number
 * @param fields {string} the space-separated header names, e.g. "SUBJECT DATE"
 * @return {string} the raw header block (the fields plus the terminating blank line)
 * @throws {Error} on a "NO" / "BAD" completion (kind "imap")
 */
export func fetchHeaders(session as Session, n as int, fields as string) {
    # `fields` is interpolated raw (not a quoted string), so it must not carry a
    # CR/LF or the `)]` that would close the fetch item and inject a command.
    rejectControl($fields, "IMAP header field list");
    def cmd as string init "FETCH " + convert.toString($n) + " BODY.PEEK[HEADER.FIELDS (" + $fields + ")]";
    return extractLiteral(command($session.conn, $cmd));
}

/**
 * Return the flags currently set on message `n` as a space-separated string
 * (e.g. "\\Seen $cl_1"), or "" when none. Useful to confirm a STORE actually
 * persisted: a server that does not allow a custom keyword answers OK but drops
 * it, so the keyword will be absent here.
 * @param session {Session} the open session
 * @param n {int} the message sequence number
 * @return {string} the flags, space-separated (no surrounding parentheses)
 * @throws {Error} on a "NO" / "BAD" completion (kind "imap")
 */
export func flags(session as Session, n as int) {
    def resp as string init command($session.conn, "FETCH " + convert.toString($n) + " (FLAGS)");
    def m as regex.Match init regex.find("FLAGS \\(([^)]*)\\)", $resp);
    if ($m.start < 0) {
        return "";
    }
    return strings.trim($m.groups[0]);
}

/**
 * Add IMAP flags / keywords to message `n` (STORE +FLAGS.SILENT). Use a keyword
 * like "$cl_1" to colour the message in Thunderbird, or the system flag
 * "\\Deleted" to mark it for `expunge`. The folder is selected read-write by
 * `selectFolder`, so the store is permitted.
 * @param session {Session} the open session
 * @param n {int} the message sequence number
 * @param flags {string} the space-separated flags, e.g. "$cl_1" or "\\Deleted"
 * @throws {Error} on a "NO" / "BAD" completion (kind "imap")
 */
export func addFlags(session as Session, n as int, flags as string) {
    command($session.conn, "STORE " + convert.toString($n) + " +FLAGS.SILENT (" + $flags + ")");
    return;
}

/**
 * Remove IMAP flags / keywords from message `n` (STORE -FLAGS.SILENT), the
 * inverse of addFlags. Removing a flag that is not set is a harmless no-op, e.g.
 * removeFlags(session, n, "$cl_1") clears that tag, "\\Deleted" un-marks a
 * pending delete.
 * @param session {Session} the open session
 * @param n {int} the message sequence number
 * @param flags {string} the space-separated flags to clear, e.g. "$cl_1"
 * @throws {Error} on a "NO" / "BAD" completion (kind "imap")
 */
export func removeFlags(session as Session, n as int, flags as string) {
    command($session.conn, "STORE " + convert.toString($n) + " -FLAGS.SILENT (" + $flags + ")");
    return;
}

/**
 * Create a folder (CREATE). A server answers "NO" if it already exists (often
 * with an "[ALREADYEXISTS]" response code), so wrap this in try / catch for a
 * create-if-missing.
 * @param session {Session} the open session
 * @param folder {string} the folder name to create
 * @throws {Error} on a "NO" / "BAD" completion (kind "imap"), including when the folder already exists
 */
export func createFolder(session as Session, folder as string) {
    command($session.conn, "CREATE " + quoteArg($folder));
    return;
}

/**
 * Copy message `n` into another folder (COPY). The source copy stays until it is
 * deleted, so the standard "move" is copy + addFlags(..., "\\Deleted") + expunge.
 * The destination folder must already exist - COPY to a missing one is a
 * "NO [TRYCREATE]" error.
 * @param session {Session} the open session
 * @param n {int} the message sequence number
 * @param folder {string} the destination folder name
 * @throws {Error} on a "NO" / "BAD" completion (kind "imap")
 */
export func copy(session as Session, n as int, folder as string) {
    command($session.conn, "COPY " + convert.toString($n) + " " + quoteArg($folder));
    return;
}

# appendLiteral runs the APPEND literal-continuation dialogue: send the command
# up to the `{N}` byte count, wait for the server's `+` continuation, then send
# the exact message bytes and read the tagged completion. `flagsPart` is "" or
# " (\Seen ...)"; the folder is quoted and the whole head is control-checked, so
# no argument can inject a command (the message body is an opaque literal, so its
# own CR/LF are data, not a command boundary).
func appendLiteral(session as Session, folder as string, flagsPart as string, message as string) {
    def raw as bytes init convert.bytesFromString($message, "utf-8");
    def head as string init TAG + " APPEND " + quoteArg($folder) + $flagsPart + " {" + convert.toString(len($raw)) + "}";
    rejectControl($head, "APPEND command");
    writeLine($session.conn, $head);
    def cont as string init readLine($session.conn);
    # A well-behaved server sends the `+` continuation or a tagged completion; skip
    # any untagged (`* ...`) status lines it interleaves first. Bounded so a
    # misbehaving server streaming untagged lines can't loop us forever.
    def guard as int init 0;
    while (strings.startsWith(strings.trim($cont), "*") and $guard < 32) {
        $cont = readLine($session.conn);
        $guard = $guard + 1;
    }
    if (not strings.startsWith(strings.trim($cont), "+")) {
        # The server refused before the literal (e.g. `NO [TRYCREATE]` for a
        # missing folder); surface its tagged completion as the error.
        expectTaggedOK($cont, TAG);
        return;
    }
    net.writeBytes($session.conn, $raw);
    net.writeBytes($session.conn, convert.bytesFromString("\r\n", "utf-8"));
    readResponse($session.conn, TAG);
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
 * @throws {Error} on a "NO" / "BAD" completion (kind "imap"), e.g. a missing folder
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
 * @throws {Error} on a "NO" / "BAD" completion (kind "imap")
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
 * @throws {Error} on a "NO" / "BAD" completion (kind "imap")
 */
export func expunge(session as Session) {
    command($session.conn, "EXPUNGE");
    return;
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
 * Connect, select `folder`, retrieve every message, and log out.
 * @param opts {Options} the connection and auth parameters
 * @param folder {string} the folder name
 * @return {list of string} the raw body of every message
 * @throws {Error} on a bad greeting or a "NO" / "BAD" completion (kind "imap")
 */
export func fetchAll(opts as Options, folder as string) {
    def session as Session init connect($opts);
    def n as int init selectFolder($session, $folder);
    def msgs as list of string init [];
    def i as int init 1;
    while ($i <= $n) {
        $msgs[] = fetch($session, $i);
        $i = $i + 1;
    }
    logout($session);
    return $msgs;
}
