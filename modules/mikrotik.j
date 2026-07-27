# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * A MikroTik RouterOS API client over `net` (not SSH). Connect to a router's
 * binary API (plain TCP 8728, or api-ssl 8729 over TLS), log in, and run
 * commands. The wire protocol is sentence-based: a sentence is a run of
 * length-prefixed words ending in a zero-length word; `talk` sends a command
 * sentence (`/interface/print`) with `=key=value` attribute words and folds
 * each `!re` reply sentence into a row map. `talkQuery` / `printWhere` add `?...`
 * query words to filter rows on the router. `print` is read sugar, `run` is for
 * mutating commands (returning the `!done`'s `=ret=`, e.g. a new item id).
 *
 * Login is plaintext (`=name=` / `=password=`, RouterOS 6.43+ and all v7), with
 * an automatic MD5 challenge-response fallback for pre-6.43 routers. A `!trap`
 * or `!fatal` reply throws `Error{kind: "mikrotik"}`. Needs the default
 * `jennifer` binary (`net`). Over `net` + `hash` (MD5 fallback) + `encoding` +
 * the bitwise operators.
 *
 * SECURITY: `mikrotik.options` uses **cleartext** TCP (port 8728), so the
 * credentials and every command are readable on the wire. Use `mikrotik.optionsTLS`
 * (api-ssl, port 8729) on any untrusted network. A `CONNECT_TIMEOUT_MS` deadline
 * bounds the dial and every read, and a server-declared word length is capped at
 * `MAX_WORD_BYTES` (64 MiB), so a wedged or hostile router cannot hang or OOM the
 * program.
 * @module mikrotik
 * @example
 * import "mikrotik.j" as mikrotik;
 * def s as mikrotik.Session init mikrotik.connect(mikrotik.options("192.168.88.1", "admin", "secret"));
 * def ifaces as list of map of string to string init mikrotik.print($s, "/interface");
 * mikrotik.close($s);
 */
use net;
use strings;
use convert;
use lists;
use maps;
use hash;
use encoding;
use uuid;

/**
 * Connection options.
 * @field host {string} the router host
 * @field port {int} the API port (8728 plain, 8729 api-ssl)
 * @field user {string} the API username
 * @field password {string} the API password
 * @field tls {bool} whether to use api-ssl (TLS)
 */
export def struct Options {
    host as string,
    port as int,
    user as string,
    password as string,
    tls as bool
};

/**
 * An open, logged-in API session.
 * @field socket {net.Conn} the underlying (plain or TLS) connection
 */
export def struct Session {
    socket as net.Conn
};

# One command's collected reply (private): the `!re` rows and the `!done` ret.
def struct Reply {
    rows as list of map of string to string,
    ret as string
};

func fail(msg as string) {
    throw Error{kind: "mikrotik", message: "mikrotik: " + $msg, file: "", line: 0, col: 0};
}

# Network timeout (ms) applied to the dial and to every read, so a blackholed or
# mid-sentence router fails instead of hanging the program forever (the RouterOS
# API has no framing that would otherwise bound a read).
def const CONNECT_TIMEOUT_MS as int init 30000;

# Cap on a server-declared word length (64 MiB, the tree-wide house rule). The
# API length prefix is up to a 32-bit value read straight off the wire; without
# this a hostile or MITM'd router (plaintext by default) declaring ~4 GiB would
# drive `readN` to OOM the recover-less interpreter.
def const MAX_WORD_BYTES as int init 67108864;

# checkWordLen rejects a server-declared word length outside [0, MAX_WORD_BYTES]
# before it is used to size a read.
func checkWordLen(n as int) {
    if ($n < 0 or $n > MAX_WORD_BYTES) {
        fail("word length out of range: " + convert.toString($n));
    }
    return;
}

# --- options (exported) -----------------------------------------------------

/**
 * Plain-TCP options (port 8728). SECURITY: this transport is **cleartext** - the
 * login exchange and every command travel unencrypted, so an on-path attacker
 * reads the router's admin credentials. Prefer `optionsTLS` (api-ssl, port 8729)
 * on any network you do not fully trust; use `options` only on a loopback or an
 * otherwise-secured link.
 * @param host {string} the router host
 * @param user {string} the API username
 * @param password {string} the API password
 * @return {Options} the options
 */
export func options(host as string, user as string, password as string) {
    return Options{host: $host, port: 8728, user: $user, password: $password, tls: false};
}

/**
 * api-ssl (TLS) options (port 8729).
 * @param host {string} the router host
 * @param user {string} the API username
 * @param password {string} the API password
 * @return {Options} the options
 */
export func optionsTLS(host as string, user as string, password as string) {
    return Options{host: $host, port: 8729, user: $user, password: $password, tls: true};
}

/**
 * Copy options with a different port.
 * @param o {Options} the options
 * @param port {int} the port
 * @return {Options} a fresh options
 */
export func withPort(o as Options, port as int) {
    def out as Options init $o;
    $out.port = $port;
    return $out;
}

# --- byte + word codec (private) --------------------------------------------

func appendBytes(dst as bytes, src as bytes) {
    def i as int init 0;
    while ($i < len($src)) {
        $dst[] = $src[$i];
        $i = $i + 1;
    }
    return $dst;
}

func readN(socket as net.Conn, n as int) {
    def out as bytes;
    # Clear the read deadline on every exit path (normal return, and the throw
    # from a mid-sentence close), so the bounded read window armed below never
    # leaks a stale deadline to a later read/write on this socket - which would
    # otherwise inherit it and spuriously time out. `defer` captures $socket now
    # and runs `net.setDeadline($socket, 0)` when this block exits (0 clears it).
    defer net.setDeadline($socket, 0);
    while (len($out) < $n) {
        net.setDeadline($socket, CONNECT_TIMEOUT_MS);
        def chunk as bytes init net.readBytes($socket, $n - len($out));
        if (len($chunk) == 0) {
            fail("connection closed mid-sentence");
        }
        # Append in place to keep the read accumulation O(N), not O(N^2).
        def k as int init 0;
        while ($k < len($chunk)) {
            $out[] = $chunk[$k];
            $k = $k + 1;
        }
    }
    return $out;
}

# encodeLen encodes a word length with RouterOS's variable-length scheme.
func encodeLen(n as int) {
    def b as bytes;
    if ($n < 0x80) {
        $b[] = $n;
    } elseif ($n < 0x4000) {
        $b[] = ($n >> 8) | 0x80;
        $b[] = $n & 0xff;
    } elseif ($n < 0x200000) {
        $b[] = ($n >> 16) | 0xc0;
        $b[] = ($n >> 8) & 0xff;
        $b[] = $n & 0xff;
    } elseif ($n < 0x10000000) {
        $b[] = ($n >> 24) | 0xe0;
        $b[] = ($n >> 16) & 0xff;
        $b[] = ($n >> 8) & 0xff;
        $b[] = $n & 0xff;
    } else {
        $b[] = 0xf0;
        $b[] = ($n >> 24) & 0xff;
        $b[] = ($n >> 16) & 0xff;
        $b[] = ($n >> 8) & 0xff;
        $b[] = $n & 0xff;
    }
    return $b;
}

# lenPrefixSize returns how many bytes a length prefix occupies, from its first
# byte.
func lenPrefixSize(head as int) {
    if (($head & 0x80) == 0) {
        return 1;
    }
    if (($head & 0xc0) == 0x80) {
        return 2;
    }
    if (($head & 0xe0) == 0xc0) {
        return 3;
    }
    if (($head & 0xf0) == 0xe0) {
        return 4;
    }
    return 5;
}

# decodeLen decodes the length prefix that starts at `off` in `buf`.
func decodeLen(buf as bytes, off as int) {
    def head as int init $buf[$off];
    def size as int init lenPrefixSize($head);
    if ($size == 1) {
        return $head;
    }
    if ($size == 2) {
        return (($head & 0x3f) << 8) | $buf[$off + 1];
    }
    if ($size == 3) {
        return (($head & 0x1f) << 16) | ($buf[$off + 1] << 8) | $buf[$off + 2];
    }
    if ($size == 4) {
        return (($head & 0x0f) << 24) | ($buf[$off + 1] << 16) | ($buf[$off + 2] << 8) | $buf[$off +
            3];
    }
    return ($buf[$off + 1] << 24) | ($buf[$off + 2] << 16) | ($buf[$off + 3] << 8) | $buf[$off + 4];
}

# readLen reads and decodes a word length prefix off the socket.
func readLen(socket as net.Conn) {
    def buf as bytes init readN($socket, 1);
    def size as int init lenPrefixSize($buf[0]);
    if ($size > 1) {
        $buf = appendBytes($buf, readN($socket, $size - 1));
    }
    return decodeLen($buf, 0);
}

# writeWord writes one length-prefixed word.
func writeWord(socket as net.Conn, word as string) {
    def raw as bytes init convert.bytesFromString($word, "utf-8");
    def out as bytes init encodeLen(len($raw));
    net.writeBytes($socket, appendBytes($out, $raw));
}

# writeSentence writes a run of words plus the zero-length terminator.
func writeSentence(socket as net.Conn, words as list of string) {
    for (def w in $words) {
        writeWord($socket, $w);
    }
    net.writeBytes($socket, encodeLen(0));
}

# readWord reads one word ("" for the zero-length sentence terminator).
func readWord(socket as net.Conn) {
    def n as int init readLen($socket);
    if ($n == 0) {
        return "";
    }
    checkWordLen($n);
    return convert.stringFromBytes(readN($socket, $n), "utf-8");
}

# readSentence reads words until the zero-length terminator.
func readSentence(socket as net.Conn) {
    def words as list of string init [];
    def done as bool init false;
    repeat {
        def w as string init readWord($socket);
        if (len($w) == 0) {
            $done = true;
        } else {
            $words[] = $w;
        }
    } until ($done);
    return $words;
}

# --- sentence <-> row mapping (private) -------------------------------------

# parseFields folds a sentence's `=key=value` words (after the reply word) into
# a map.
func parseFields(sentence as list of string) {
    def fields as map of string to string init {};
    def i as int init 1;
    while ($i < len($sentence)) {
        def w as string init $sentence[$i];
        if (strings.startsWith($w, "=")) {
            def rest as string init strings.substring($w, 1, len($w));
            def eq as int init strings.indexOf($rest, "=");
            if ($eq >= 0) {
                $fields[strings.substring($rest, 0, $eq)] = strings.substring(
                    $rest,
                    $eq + 1,
                    len($rest));
            }
        }
        $i = $i + 1;
    }
    return $fields;
}

# parseTag returns the value of a reply sentence's `.tag=<id>` word (the API tag
# word, distinct from the `=key=value` attribute words `parseFields` folds), or
# "" when the sentence carries no tag. Used to correlate a pushed reply back to
# the command that started it.
func parseTag(sentence as list of string) {
    for (def w in $sentence) {
        if (strings.startsWith($w, ".tag=")) {
            return strings.substring($w, 5, len($w));
        }
    }
    return "";
}

# buildWords turns a command plus attribute and query words into a word list.
# Attribute words are `=key=value`; query words are raw RouterOS query words -
# each starts with `?` (e.g. `?type=ether`, `?>mtu=1000`, or a `?#&` / `?#|` /
# `?#!` stack operator) - and are appended verbatim, so the full query grammar
# stays reachable. A query word missing its leading `?` is a caller error.
func buildWords(command as string, attrs as map of string to string, queries as list of string) {
    def words as list of string init [$command];
    for (def key in $attrs) {
        $words[] = "=" + $key + "=" + $attrs[$key];
    }
    for (def q in $queries) {
        if (not strings.startsWith($q, "?")) {
            fail("query word must start with '?': " + $q);
        }
        $words[] = $q;
    }
    return $words;
}

# buildTaggedWords is buildWords plus a trailing `.tag=<tag>` API word, so the
# router echoes the tag on every reply to this command and a later `/cancel` can
# name it. The command word stays first; the tag word (like every API word) may
# follow in any order.
func buildTaggedWords(
    command as string,
    attrs as map of string to string,
    queries as list of string,
    tag as string) {
    def words as list of string init buildWords($command, $attrs, $queries);
    $words[] = ".tag=" + $tag;
    return $words;
}

# buildCancelWords builds a `/cancel` sentence naming the tag to stop:
# ["/cancel", "=tag=<tag>"]. Sent to interrupt a streaming command started with
# that tag; the router answers the streaming command with `!trap` (category
# "interrupted") then `!done`.
func buildCancelWords(tag as string) {
    return ["/cancel", "=tag=" + $tag];
}

# resolveTag returns the caller's tag when non-empty, else an auto-generated,
# collision-free one (a UUID, drawn from crypto-grade randomness).
func resolveTag(tag as string) {
    if (len($tag) == 0) {
        return uuid.v4();
    }
    return $tag;
}

# collectReply reads reply sentences until `!done`, collecting `!re` rows and the
# `!done` ret; a `!trap` / `!fatal` throws. Shared by the untagged `exchange` and
# the tagged `talkTagged` (both bounded, one-shot commands; a streaming command
# never reaches `!done` and is read with `receiveReply` instead).
func collectReply(session as Session) {
    def rows as list of map of string to string init [];
    def ret as string init "";
    def trapMsg as string init "";
    def trapped as bool init false;
    def done as bool init false;
    repeat {
        def sentence as list of string init readSentence($session.socket);
        if (len($sentence) == 0) {
            $done = true;
        } else {
            def reply as string init $sentence[0];
            def fields as map of string to string init parseFields($sentence);
            if ($reply == "!re") {
                $rows[] = $fields;
            } elseif ($reply == "!done") {
                if (maps.has($fields, "ret")) {
                    $ret = $fields["ret"];
                }
                $done = true;
            } elseif ($reply == "!trap") {
                # A trap is followed by a !done; record it and keep reading so
                # the trailing !done is consumed before we raise.
                $trapped = true;
                if (maps.has($fields, "message")) {
                    $trapMsg = $fields["message"];
                }
            } elseif ($reply == "!fatal") {
                # A fatal closes the connection - no !done follows, so raise now.
                def fmsg as string init "connection error";
                if (maps.has($fields, "message")) {
                    $fmsg = $fields["message"];
                }
                fail("!fatal: " + $fmsg);
            }
        }
    } until ($done);
    if ($trapped) {
        def msg as string init "command failed";
        if (len($trapMsg) > 0) {
            $msg = $trapMsg;
        }
        fail("!trap: " + $msg);
    }
    return Reply{rows: $rows, ret: $ret};
}

# exchange sends a command and collects its reply to `!done`.
func exchange(
    session as Session,
    command as string,
    attrs as map of string to string,
    queries as list of string) {
    writeSentence($session.socket, buildWords($command, $attrs, $queries));
    return collectReply($session);
}

# --- login (private) --------------------------------------------------------

# challengeResponse builds the pre-6.43 challenge response: "00" + hex(MD5(0x00 +
# password + challengeBytes)).
func challengeResponse(password as string, challengeHex as string) {
    def input as bytes;
    $input[] = 0;
    $input = appendBytes($input, convert.bytesFromString($password, "utf-8"));
    $input = appendBytes($input, encoding.fromText($challengeHex, "hex"));
    return "00" + encoding.toText(hash.compute($input, "md5"), "hex");
}

func login(session as Session, user as string, password as string) {
    def attrs as map of string to string init {};
    $attrs["name"] = $user;
    $attrs["password"] = $password;
    def r as Reply init exchange($session, "/login", $attrs, []);
    # An old (pre-6.43) router answers with a challenge in =ret= instead of
    # logging in; complete the MD5 challenge-response.
    if (len($r.ret) > 0) {
        def resp as map of string to string init {};
        $resp["name"] = $user;
        $resp["response"] = challengeResponse($password, $r.ret);
        exchange($session, "/login", $resp, []);
    }
}

# --- API (exported) ---------------------------------------------------------

/**
 * Connect to a router and log in.
 * @param opts {Options} the connection options
 * @return {Session} the logged-in session
 * @throws {Error} kind "mikrotik" on a login failure
 */
export func connect(opts as Options) {
    def addr as string init $opts.host + ":" + convert.toString($opts.port);
    def socket as net.Conn;
    if ($opts.tls) {
        $socket = net.connectTLS($addr, CONNECT_TIMEOUT_MS);
    } else {
        $socket = net.connect($addr, CONNECT_TIMEOUT_MS);
    }
    def session as Session init Session{socket: $socket};
    # A refused login must not leak the socket; on success the caller owns the
    # open session.
    errdefer net.close($socket);
    login($session, $opts.user, $opts.password);
    return $session;
}

/**
 * Run a command with attribute words and return the `!re` reply rows (each a
 * `=key=value` map). The general call - reads, adds, sets, and removes all go
 * through it.
 * @param session {Session} the session
 * @param command {string} the API command (e.g. "/interface/print")
 * @param attrs {map of string to string} attribute words ({} for none)
 * @return {list of map of string to string} the reply rows
 * @throws {Error} kind "mikrotik" on a !trap / !fatal reply
 */
export func talk(session as Session, command as string, attrs as map of string to string) {
    return exchange($session, $command, $attrs, []).rows;
}

/**
 * Like `talk`, but also sends RouterOS **query words** to filter the reply on the
 * router. Each query word is raw and starts with `?`: `?type=ether` (property
 * equals), `?disabled` (property present), `?>mtu=1000` (greater), plus the stack
 * operators `?#!` / `?#&` / `?#|` for compound queries. Attribute words still
 * apply (e.g. `=.proplist=name,type` to trim the columns). The general filtered
 * read the higher-level helpers build on.
 * @param session {Session} the session
 * @param command {string} the API command (e.g. "/interface/print")
 * @param attrs {map of string to string} attribute words ({} for none)
 * @param queries {list of string} raw query words, each starting with "?"
 * @return {list of map of string to string} the matching reply rows
 * @throws {Error} kind "mikrotik" on a bad query word or a !trap / !fatal reply
 */
export func talkQuery(
    session as Session,
    command as string,
    attrs as map of string to string,
    queries as list of string) {
    return exchange($session, $command, $attrs, $queries).rows;
}

/**
 * Read sugar: run `path + "/print"` with no attributes.
 * @param session {Session} the session
 * @param path {string} the menu path (e.g. "/interface")
 * @return {list of map of string to string} the reply rows
 * @throws {Error} kind "mikrotik" on a !trap / !fatal reply
 */
export func print(session as Session, path as string) {
    def none as map of string to string init {};
    return exchange($session, $path + "/print", $none, []).rows;
}

/**
 * Read sugar: run `path + "/print"` filtered by query words (no attributes) -
 * `printWhere($s, "/interface", ["?type=ether"])` returns only the ether ports.
 * @param session {Session} the session
 * @param path {string} the menu path (e.g. "/interface")
 * @param queries {list of string} raw query words, each starting with "?"
 * @return {list of map of string to string} the matching reply rows
 * @throws {Error} kind "mikrotik" on a bad query word or a !trap / !fatal reply
 */
export func printWhere(session as Session, path as string, queries as list of string) {
    def none as map of string to string init {};
    return exchange($session, $path + "/print", $none, $queries).rows;
}

/**
 * Run a mutating command (add / set / remove) and return the `!done` `=ret=`
 * value (e.g. the new item's id for `add`; "" when there is none).
 * @param session {Session} the session
 * @param command {string} the API command (e.g. "/ip/address/add")
 * @param attrs {map of string to string} attribute words
 * @return {string} the `=ret=` value, or ""
 * @throws {Error} kind "mikrotik" on a !trap / !fatal reply
 */
export func run(session as Session, command as string, attrs as map of string to string) {
    return exchange($session, $command, $attrs, []).ret;
}

# --- tagging + streaming (exported) -----------------------------------------

/**
 * Like `talk`, but attaches a `.tag=<id>` word so the router echoes the tag on
 * every reply, letting a caller correlate this command's replies. Pass `tag: ""`
 * to auto-generate a unique tag. The reply is still collected to `!done` (a
 * one-shot command); for a command that pushes replies over time use `listen` +
 * `receiveReply`.
 * @param session {Session} the session
 * @param command {string} the API command (e.g. "/interface/print")
 * @param attrs {map of string to string} attribute words ({} for none)
 * @param tag {string} the correlation tag, or "" to auto-generate one
 * @return {list of map of string to string} the reply rows
 * @throws {Error} kind "mikrotik" on a !trap / !fatal reply
 */
export func talkTagged(
    session as Session,
    command as string,
    attrs as map of string to string,
    tag as string) {
    def t as string init resolveTag($tag);
    def noq as list of string init [];
    writeSentence($session.socket, buildTaggedWords($command, $attrs, $noq, $t));
    return collectReply($session).rows;
}

/**
 * Start a **streaming** command (one that pushes `!re` sentences over time, e.g.
 * "/interface/monitor" or "/ping") and return its auto-generated tag. Does NOT
 * read a reply - the stream has no `!done` until it is stopped. Pull each pushed
 * reply with `receiveReply($s, tag)` (typically from a `spawn`ed loop), and stop
 * it with `cancel($s, tag)`.
 * @param session {Session} the session
 * @param command {string} the streaming API command
 * @param params {map of string to string} attribute words for the command ({} for none)
 * @return {string} the tag naming this stream (feed it to receiveReply / cancel)
 */
export func listen(session as Session, command as string, params as map of string to string) {
    def tag as string init uuid.v4();
    def noq as list of string init [];
    writeSentence($session.socket, buildTaggedWords($command, $params, $noq, $tag));
    return $tag;
}

/**
 * Block until the next pushed reply for `tag` arrives and return it as a
 * `=key=value` field map. Returns an **empty map** when the stream has ended (a
 * `!done` for the tag - e.g. after `cancel`), which is the loop's stop signal.
 * Replies carrying a different tag are skipped (they belong to another stream on
 * the same session). A `cancel`-induced `!trap` (category "interrupted") is
 * absorbed; any other `!trap` / `!fatal` throws.
 * @param session {Session} the session
 * @param tag {string} the stream tag returned by `listen`
 * @return {map of string to string} the next reply's fields, or {} at stream end
 * @throws {Error} kind "mikrotik" on a genuine !trap / !fatal reply
 */
export func receiveReply(session as Session, tag as string) {
    def result as map of string to string init {};
    def found as bool init false;
    def ended as bool init false;
    repeat {
        def sentence as list of string init readSentence($session.socket);
        def reply as string init "";
        if (len($sentence) > 0) {
            $reply = $sentence[0];
        }
        def rtag as string init parseTag($sentence);
        def mine as bool init (len($tag) == 0 or $rtag == $tag);
        if ($reply == "!re") {
            if ($mine) {
                $result = parseFields($sentence);
                $found = true;
            }
        } elseif ($reply == "!done") {
            if ($mine) {
                $ended = true;
            }
        } elseif ($reply == "!trap") {
            def fields as map of string to string init parseFields($sentence);
            def cat as string init "";
            if (maps.has($fields, "category")) {
                $cat = $fields["category"];
            }
            # A cancel interrupts the streaming command with a trap; it is normal
            # end-of-stream, not an error, so keep reading for the trailing !done.
            if ($cat != "interrupted") {
                def m as string init "command failed";
                if (maps.has($fields, "message")) {
                    $m = $fields["message"];
                }
                fail("!trap: " + $m);
            }
        } elseif ($reply == "!fatal") {
            def ff as map of string to string init parseFields($sentence);
            def fm as string init "connection error";
            if (maps.has($ff, "message")) {
                $fm = $ff["message"];
            }
            fail("!fatal: " + $fm);
        }
    } until ($found or $ended);
    return $result;
}

/**
 * Stop a streaming command started with `listen` by issuing `/cancel` naming its
 * tag. Fire-and-forget: the resulting `!trap` (interrupted) and `!done` for the
 * stream, plus the `/cancel`'s own reply, are drained by the `receiveReply` loop
 * (which then returns {} and lets the loop exit).
 * @param session {Session} the session
 * @param tag {string} the stream tag returned by `listen`
 */
export func cancel(session as Session, tag as string) {
    writeSentence($session.socket, buildCancelWords($tag));
}

/**
 * Close the session's connection.
 * @param session {Session} the session
 */
export func close(session as Session) {
    net.close($session.socket);
}
