# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.28.0
# pragma-jennifer-capability: net

/**
 * An HTTP/1.1 client over the `net` system library. Build a request (method,
 * URL, headers, body), send it, and read the response back into a `Response`
 * (status, headers, body). `http://` connects in the clear; `https://`
 * connects with TLS (`net.connectTLS`). The one-shot verbs (`get` / `post` /
 * `request` / ...) use one connection per request (`Connection: close`);
 * `connect` / `exchange` reuse a persistent connection to one origin (keep-alive)
 * so a request loop pays a single handshake. `send` adds a request policy on top
 * of the one-shot path: automatic 3xx redirect-following, retry / backoff on
 * 429 / 5xx, and a cookie jar across the redirect chain. Because it uses `net`,
 * this module needs the default `jennifer` binary. A text `Response` body is
 * decoded as UTF-8 (a binary body raises an error; use `sendBytes` /
 * `getBytes` for that). Chunked and Content-Length framing are both handled.
 * @module http
 * @example
 * use io;
 * import "http.j" as http;
 * def r as http.Response init http.get("http://example.com/", {});
 * io.printf("status %d\n%s\n", $r.status, $r.body);
 * def sent as http.Response init http.post("https://api.example.com/items",
 *     "application/json", '{"name":"ada"}', {"Authorization": "Bearer xyz"});
 */
use net;
use binary;
use strings;
use convert;
use maps;
use lists;
use encoding;
use time;
use regex;
use hash;
use crypto;
use math;
import "./uri.j" as uri;

# A parsed request URL.
def struct Url {
    scheme as string,
    host as string,
    port as int,
    path as string
};

/**
 * An HTTP response. `headers` keys are lowercased (HTTP header names are
 * case-insensitive); use `http.header` for a case-insensitive read.
 * @field status {int} the numeric status code (e.g. 200, 404)
 * @field statusText {string} the reason phrase from the status line
 * @field headers {map of string to string} response headers, keys lowercased
 * @field body {string} the response body decoded as UTF-8 text
 */
export def struct Response {
    status as int,
    statusText as string,
    headers as map of string to string,
    body as string
};

/**
 * A response with a **raw `bytes` body** - the byte-safe counterpart to
 * `Response`, for downloading binary payloads (a `.tar.gz`, an image, any
 * non-text content) that a UTF-8 string body cannot hold. Returned by
 * `sendBytes` / `getBytes`. `headers` keys are
 * lowercased (read `$r.headers["content-type"]` directly, or with `http.header`
 * after wrapping - the map is shared shape with `Response`).
 * @field status {int} the numeric status code
 * @field statusText {string} the reason phrase from the status line
 * @field headers {map of string to string} response headers, keys lowercased
 * @field body {bytes} the response body, exactly as received (no decoding)
 */
export def struct BytesResponse {
    status as int,
    statusText as string,
    headers as map of string to string,
    body as bytes
};

/**
 * Deprecated TLS options for an `https://` request, the fields of
 * `net.TLSOptions`; set `Options.tlsOptions` (a `net.TLSOptions`) instead.
 * Removed in 0.29.0. The zero value full-verifies the server certificate
 * against the URL host and the system roots.
 * @field skipVerify {bool} accept *any* certificate (self-signed, wrong host, expired). Opt-in; disables authentication and exposes the connection to a man-in-the-middle. Use only for a trusted LAN endpoint you cannot give a proper CA.
 * @field caCert {bytes} a PEM certificate (or bundle) to trust **instead of** the system roots, for a private CA or a pinned self-signed cert. The safer alternative to `skipVerify`: the server is still authenticated, just against this cert.
 * @deprecated use `net.TLSOptions` in `Options.tlsOptions`; removed in 0.29.0
 */
export def struct TlsOptions {
    skipVerify as bool,
    caCert as bytes
};

# --- byte helpers (private) ----------------------------------------

# sliceBytes copies buf[start:end] into a new bytes value.
func sliceBytes(buf as bytes, start as int, end as int) {
    return $buf[$start..$end];
}

# bytesToStr decodes buf[start:end] - a response head or a chunk line - as
# UTF-8 text. A byte sequence that is not UTF-8 is a malformed response, kind
# "http" (decoding is the only thing that can fail here).
func bytesToStr(buf as bytes, start as int, end as int) {
    try {
        return encoding.decode("utf-8", sliceBytes($buf, $start, $end));
    } catch (e) {
        throw Error{kind: "http", message: "http: response head is not valid UTF-8"};
    }
}

# bodyText decodes a response body as UTF-8 for a text `Response`. A body that
# is not UTF-8 throws kind "http"; the byte verbs read such a body.
func bodyText(body as bytes) {
    try {
        return encoding.decode("utf-8", $body);
    } catch (e) {
        throw Error{
            kind: "http",
            message: "http: response body is not valid UTF-8 (use the byte verbs)"
        };
    }
}

def const CRLF as bytes init encoding.encode("utf-8", "\r\n");
def const CRLF2 as bytes init encoding.encode("utf-8", "\r\n\r\n");

# headerEnd returns the index of the CRLFCRLF separating headers from body, -1
# if not present.
func headerEnd(buf as bytes) {
    return binary.indexOf($buf, CRLF2);
}

# hexToInt parses a hex string (a chunk-size line), stopping at the first
# non-hex character (a chunk extension after ";").
func hexToInt(s as string) {
    def result as int init 0;
    for (def c in strings.chars(strings.lower($s))) {
        def d as int init strings.indexOf("0123456789abcdef", $c);
        if ($d < 0) {
            return $result;
        }
        $result = $result * 16 + $d;
    }
    return $result;
}

# chunkSize reads a chunk-size line: hex digits, then an optional ";"
# extension. Anything else is an error: read leniently, a garbled line would
# count as the terminal 0 chunk and cut the body short without a word.
func chunkSize(line as string) {
    def digits as string init $line;
    def semi as int init strings.indexOf($line, ";");
    if ($semi >= 0) {
        $digits = $line[..$semi];
    }
    $digits = strings.trim($digits);
    if (not regex.matches('^[0-9A-Fa-f]{1,15}$', $digits)) {
        throw Error{kind: "http", message: "malformed chunk size line"};
    }
    return hexToInt($digits);
}

# dechunk decodes a chunked transfer-encoded body (all bytes already read).
func dechunk(body as bytes) {
    def parts as list of bytes init [];
    def pos as int init 0;
    def n as int init len($body);
    while ($pos < $n) {
        def lineEnd as int init binary.indexOf($body, CRLF, $pos);
        if ($lineEnd < 0) {
            # A chunk-size line with no CRLF means the connection dropped
            # mid-stream: fail loudly rather than return a silent partial body.
            throw Error{kind: "http", message: "truncated body: chunk size line has no CRLF"};
        }
        def size as int init chunkSize(bytesToStr($body, $pos, $lineEnd));
        if ($size == 0) {
            return binary.join($parts); # terminal 0-length chunk: complete
        }
        def dataStart as int init $lineEnd + 2;
        def dataEnd as int init $dataStart + $size;
        if ($dataEnd + 2 > $n) {
            throw Error{
                kind: "http",
                message: "truncated body: chunk declares more bytes than received"
            };
        }
        if ($body[$dataEnd..$dataEnd + 2] != CRLF) {
            throw Error{kind: "http", message: "malformed chunk: data not followed by CRLF"};
        }
        $parts[] = $body[$dataStart..$dataEnd];
        $pos = $dataEnd + 2;
    }
    # Ran out of input without ever seeing the terminal 0-length chunk.
    throw Error{kind: "http", message: "truncated body: missing terminal chunk"};
}

# --- request / response (private) ----------------------------------

# parseUrl splits a URL into scheme / host / port / path (with query). The
# scheme must be http or https; defaults: port 80 (443 for https), path "/".
func parseUrl(url as string) {
    # Only http:// and https:// are spoken. A missing or unknown scheme is
    # refused rather than read as plain http: a typo (`htps://`) or a bare
    # `host/path` would otherwise send the request path, often the secret part
    # of a webhook / token URL, in the clear. The URL stays out of the message
    # for the same reason.
    def si as int init strings.indexOf($url, "://");
    if ($si < 0) {
        throw Error{kind: "http", message: "http: URL has no scheme (want http:// or https://)"};
    }
    def scheme as string init strings.lower($url[..$si]);
    if ($scheme != "http" and $scheme != "https") {
        throw Error{
            kind: "http",
            message: "http: unsupported URL scheme (want http:// or https://)"
        };
    }
    def rest as string init $url[$si + 3..];
    # A fragment (#...) is never sent to the server; strip it before anything
    # else so it cannot leak onto the wire.
    def hash as int init strings.indexOf($rest, "#");
    if ($hash >= 0) {
        $rest = $rest[..$hash];
    }
    def authority as string init $rest;
    def path as string init "/";
    # The authority ends at the first `/` or `?`. A query-only URL
    # (`http://host?q=1`, no path) is legal - the `?...` is the request target,
    # not part of the host, so cut on `?` too or it dials `host?q=1:80` (OM-020).
    def slash as int init strings.indexOf($rest, "/");
    def query as int init strings.indexOf($rest, "?");
    def cut as int init $slash;
    if ($query >= 0 and ($cut < 0 or $query < $cut)) {
        $cut = $query;
    }
    if ($cut >= 0) {
        $authority = $rest[..$cut];
        def target as string init $rest[$cut..];
        if (strings.startsWith($target, "?")) {
            $path = "/" + $target;
        } else {
            $path = $target;
        }
    }
    # Strip userinfo (user[:pass]@) at the LAST '@' - a password may contain '@'.
    def at as int init strings.lastIndexOf($authority, "@");
    if ($at >= 0) {
        $authority = $authority[$at + 1..];
    }
    def host as string init $authority;
    def port as int init 80;
    if ($scheme == "https") {
        $port = 443;
    }
    def portText as string init "";
    if (strings.startsWith($authority, "[")) {
        # Bracketed IPv6 literal: the colon inside the brackets is not the
        # port separator. Keep the brackets on the host so the Host header
        # and net.connect address stay well-formed (`[::1]:8080`).
        def rb as int init strings.indexOf($authority, "]");
        if ($rb < 0) {
            throw Error{kind: "http", message: "http: URL host has an unclosed \"[\""};
        }
        $host = $authority[..$rb + 1];
        def afterBr as string init $authority[$rb + 1..];
        if (strings.startsWith($afterBr, ":")) {
            $portText = $afterBr[1..];
        } elseif ($afterBr != "") {
            throw Error{kind: "http", message: "http: URL host is malformed"};
        }
    } else {
        def colon as int init strings.indexOf($authority, ":");
        if ($colon >= 0) {
            $host = $authority[..$colon];
            $portText = $authority[$colon + 1..];
        }
    }
    # An empty host would dial ":80", which Go reads as this machine.
    if ($host == "" or $host == "[]" or regex.matches('[\x00-\x20\x7f]', $host)) {
        throw Error{kind: "http", message: "http: URL has no host, or a malformed one"};
    }
    # "host:" is the default port (RFC 3986); otherwise 1 to 65535.
    if ($portText != "") {
        if (not regex.matches('^[0-9]{1,5}$', $portText) or convert.toInt($portText) < 1 or
            convert.toInt($portText) > 65535) {
            throw Error{kind: "http", message: "http: URL port is not a number from 1 to 65535"};
        }
        $port = convert.toInt($portText);
    }
    return Url{scheme: $scheme, host: $host, port: $port, path: $path};
}

# hostHeader renders the Host header value (host, plus port when non-default).
func hostHeader(u as Url) {
    if ($u.scheme == "http" and $u.port == 80) {
        return $u.host;
    }
    if ($u.scheme == "https" and $u.port == 443) {
        return $u.host;
    }
    return "{$u.host}:{$u.port}";
}

# chosenHost is the Host header to send: the caller's own `Host` when it names
# this URL's host and port - with the port written out, which some devices
# (Epson scanners) require even when it is the default - and `hostHeader`
# otherwise. Any other value is dropped, so a caller cannot aim the request at
# a different virtual host, and a header carried across a redirect to another
# origin falls back to the default.
func chosenHost(u as Url, headers as map of string to string) {
    def plain as string init hostHeader($u);
    for (def k in $headers) {
        if (strings.lower($k) == "host") {
            def v as string init strings.lower(strings.trim($headers[$k]));
            if ($v == strings.lower($plain) or $v == strings.lower("{$u.host}:{$u.port}")) {
                return strings.trim($headers[$k]);
            }
        }
    }
    return $plain;
}

# hasControlChar reports whether s contains CR, LF, or NUL - the bytes that let
# a caller-supplied value break out of its field and inject request lines.
func hasControlChar(s as string) {
    return strings.contains($s, "\r") or strings.contains($s, "\n") or strings.contains($s, "\0");
}

# TOKEN is an RFC 9110 token: what a method or a header name may hold.
def const TOKEN as string init "^[!#$%&'*+.^_`|~0-9A-Za-z-]+$";

# rejectInjection throws if any part that goes onto the request line or a header
# carries CR / LF / NUL, which would otherwise smuggle extra headers or a whole
# second request (HTTP request splitting / header injection). The request
# target may hold no whitespace or control character at all (a space would end
# it early on the request line), and must start with "/" or be "*".
func rejectInjection(u as Url, headers as map of string to string) {
    if (hasControlChar($u.host) or hasControlChar($u.path)) {
        throw Error{
            kind: "http",
            message: "request target contains a control character (CR/LF/NUL)"
        };
    }
    if (regex.matches('[\x00-\x20\x7f]', $u.path)) {
        throw Error{
            kind: "http",
            message: "request target contains whitespace or a control character (percent-encode it)"
        };
    }
    if (not strings.startsWith($u.path, "/") and $u.path != "*") {
        throw Error{kind: "http", message: "request target must start with \"/\""};
    }
    for (def k in $headers) {
        if (hasControlChar($k)) {
            throw Error{
                kind: "http",
                message: "header name contains a control character (CR/LF/NUL)"
            };
        }
        if (not regex.matches(TOKEN, $k)) {
            throw Error{kind: "http", message: "header name is not a valid token"};
        }
        if (hasControlChar($headers[$k])) {
            throw Error{
                kind: "http",
                message: "header value contains a control character (CR/LF/NUL): " + $k
            };
        }
    }
}

# buildRequest renders the full request text for a method / URL / headers / body.
# hasHeaderCI reports whether headers holds a key equal to lname (already
# lowercased), matching case-insensitively per RFC 7230.
func hasHeaderCI(headers as map of string to string, lname as string) {
    for (def k in $headers) {
        if (strings.lower($k) == $lname) {
            return true;
        }
    }
    return false;
}

# buildHead builds the request line + headers + the terminating blank line (no
# body). `contentLen` is the byte length of the body the caller will send (0 for
# none), emitted as Content-Length. The head is pure ASCII, so it is safe to
# UTF-8-encode for the wire even when the body that follows is raw binary.
func buildHead(method as string, u as Url, headers as map of string to string, contentLen as int) {
    return buildHeadConn($method, $u, $headers, $contentLen, false);
}

# buildHeadConn is buildHead with an explicit connection disposition: `keepAlive`
# true emits `Connection: keep-alive` (the persistent-session path reuses the
# socket), false emits `Connection: close` (the one-shot path, so the server ends
# the response with EOF). Everything else - injection checks, framing-header
# ownership, default User-Agent - is identical.
func buildHeadConn(
    method as string,
    u as Url,
    headers as map of string to string,
    contentLen as int,
    keepAlive as bool) {
    rejectInjection($u, $headers);
    # The method goes onto the request line verbatim, so a CR / LF / NUL in it
    # would smuggle extra headers or a second request just like the target does.
    if (hasControlChar($method)) {
        throw Error{
            kind: "http",
            message: "request method contains a control character (CR/LF/NUL)"
        };
    }
    if (not regex.matches(TOKEN, $method)) {
        throw Error{kind: "http", message: "request method is not a valid token"};
    }
    def out as string init $method + " " + $u.path + " HTTP/1.1\r\n";
    $out = $out + "Host: " + chosenHost($u, $headers) + "\r\n";
    def conndisp as string init "close";
    if ($keepAlive) {
        $conndisp = "keep-alive";
    }
    $out = $out + "Connection: " + $conndisp + "\r\n";
    if (not hasHeaderCI($headers, "user-agent")) {
        $out = $out + "User-Agent: jennifer-http\r\n";
    }
    for (def k in $headers) {
        # Skip the framing headers the client owns: re-emitting a caller's Host /
        # Connection / Content-Length yields a duplicate that strict servers
        # reject, and a Transfer-Encoding next to our Content-Length is the
        # classic request-smuggling pair. Compared case-insensitively.
        def lk as string init strings.lower($k);
        if ($lk == "host" or $lk == "connection" or $lk == "content-length" or
            $lk == "transfer-encoding") {
            continue;
        }
        $out = $out + $k + ": " + $headers[$k] + "\r\n";
    }
    # An empty POST / PUT / PATCH still says "Content-Length: 0": without it
    # some servers and proxies answer 411 Length Required.
    def um as string init strings.upper($method);
    if ($contentLen > 0 or $um == "POST" or $um == "PUT" or $um == "PATCH") {
        $out = $out + "Content-Length: {$contentLen}\r\n";
    }
    return $out + "\r\n";
}

func buildRequest(method as string, u as Url, headers as map of string to string, body as string) {
    def blen as int init 0;
    if (len($body) > 0) {
        $blen = len(encoding.encode("utf-8", $body));
    }
    return buildHead($method, $u, $headers, $blen) + $body;
}

# headerFail throws the error for a malformed response head.
func headerFail(what as string) {
    throw Error{kind: "http", message: "http: malformed response: " + $what};
}

# parseHeaders parses the header lines (after the status line) into a lowercased
# map. Every line must be `name: value` with a token name and the colon right
# after it (RFC 9112 5.1); a line without a colon, a space before the colon, a
# folded continuation line (obs-fold, RFC 9112 5.2) or a control character in a
# value is a malformed response, kind "http", never a dropped line.
func parseHeaders(lines as list of string) {
    def headers as map of string to string init {};
    def i as int init 1;
    while ($i < len($lines)) {
        def line as string init $lines[$i];
        if (strings.startsWith($line, " ") or strings.startsWith($line, "\t")) {
            headerFail("folded header line (obs-fold)");
        }
        def colon as int init strings.indexOf($line, ":");
        if ($colon < 0) {
            headerFail("header line without a colon");
        }
        def raw as string init $line[..$colon];
        if (not regex.matches(TOKEN, $raw)) {
            headerFail("header name is not a token");
        }
        def name as string init strings.lower($raw);
        def value as string init strings.trim($line[$colon + 1..]);
        if (regex.matches('[\x00-\x08\x0a-\x1f\x7f]', $value)) {
            headerFail("control character in a header value");
        }
        # Repeated headers (e.g. two Set-Cookie lines) would otherwise
        # last-wins-collapse, silently dropping all but one. Join them with
        # ", " so no value is lost (RFC 7230 comma-folding; a caller that
        # needs individual Set-Cookie values must re-split).
        if (maps.has($headers, $name)) {
            $headers[$name] = $headers[$name] + ", " + $value;
        } else {
            $headers[$name] = $value;
        }
        $i = $i + 1;
    }
    return $headers;
}

# A parsed status line.
def struct StatusLine {
    status as int,
    text as string
};

# parseStatusLine reads `HTTP/1.x SP code [SP reason]` (RFC 9112 4). The reason
# phrase may be absent ("HTTP/1.1 200"), which every client tolerates; any
# other protocol name or version, a code that is not three digits, or a
# control character in the reason is a malformed response, kind "http".
func parseStatusLine(line as string) {
    def m as regex.Match init regex.find('^HTTP/1\.[0-9] ([0-9]{3})(?: (.*))?$', $line);
    if ($m.start < 0 or regex.matches('[\x00-\x08\x0a-\x1f\x7f]', $line)) {
        headerFail("status line");
    }
    return StatusLine{status: convert.toInt($m.groups[0]), text: $m.groups[1]};
}

# parseRaw parses a complete raw response (headers + body bytes) into a
# BytesResponse, de-chunking or trimming the body to its framed length. The body
# stays raw bytes - this is the byte-exact core both the text and binary paths
# build on; `parseResponse` decodes it to a string, the byte verbs keep it.
func parseRaw(raw as bytes) {
    def hend as int init headerEnd($raw);
    if ($hend < 0) {
        throw Error{kind: "http", message: "malformed response (no header terminator)"};
    }
    def headerText as string init bytesToStr($raw, 0, $hend);
    def lines as list of string init strings.split($headerText, "\r\n");
    def sl as StatusLine init parseStatusLine($lines[0]);
    def headers as map of string to string init parseHeaders($lines);
    return BytesResponse{
        status: $sl.status,
        statusText: $sl.text,
        headers: $headers,
        body: bodyOf($raw, $hend + 4, $headers)
    };
}

# bodyOf cuts the body out of a raw response whose body starts at `start`,
# decoding chunked framing and stopping at the Content-Length. Each branch
# returns its one copy directly: assigning a large body to a variable copies it.
func bodyOf(raw as bytes, start as int, headers as map of string to string) {
    def end as int init len($raw);
    # An empty body is a bodiless response (HEAD / 204 / 304): a HEAD answer
    # may still advertise Transfer-Encoding: chunked for the GET it describes,
    # while a real chunked body always holds at least its terminating 0 chunk.
    if ($end > $start and isChunked($headers)) {
        return dechunk(sliceBytes($raw, $start, $end));
    }
    if (maps.has($headers, "content-length")) {
        def cl as int init contentLength($headers);
        if ($end - $start > $cl) {
            $end = $start + $cl;
        }
    }
    return sliceBytes($raw, $start, $end);
}

# parseResponse parses a raw response into a text Response, decoding the body as
# UTF-8. Throws (strict, no replacement) on a non-UTF-8 body - use the byte verbs
# (`sendBytes` / `getBytes`) for binary payloads.
func parseResponse(raw as bytes) {
    return textOf(parseRaw($raw));
}

# contentLength reads a response's Content-Length: digits, or a list of equal
# values (RFC 9110 8.6, what comma-folding two identical headers gives).
# Anything else, or two different lengths, is an error: the body framing
# cannot be trusted.
func contentLength(headers as map of string to string) {
    def values as list of string init strings.split($headers["content-length"], ",");
    def first as string init strings.trim($values[0]);
    for (def v in $values) {
        def t as string init strings.trim($v);
        if (not regex.matches('^[0-9]{1,15}$', $t) or $t != $first) {
            throw Error{kind: "http", message: "malformed or conflicting Content-Length"};
        }
    }
    return convert.toInt($first);
}

# isChunked reports whether the response uses chunked transfer-encoding. This
# client sends no TE header, so `chunked` is the only transfer coding a server
# may apply (RFC 9112 6.1, 7): any other coding (gzip, a chunked that is not
# last, chunked twice) cannot be decoded and is an error, as is a
# Transfer-Encoding next to a Content-Length (RFC 9112 6.3: a sign of response
# smuggling, to be handled as an error). Kind "http".
func isChunked(headers as map of string to string) {
    if (not maps.has($headers, "transfer-encoding")) {
        return false;
    }
    def codings as list of string init [];
    for (def c in strings.split($headers["transfer-encoding"], ",")) {
        $codings[] = strings.lower(strings.trim($c));
    }
    if ($codings != ["chunked"]) {
        headerFail("unsupported Transfer-Encoding (only chunked is accepted)");
    }
    if (maps.has($headers, "content-length")) {
        headerFail("both Transfer-Encoding and Content-Length");
    }
    return true;
}

# --- net (private) -------------------------------------------------

# The default idle timeout: a read that stalls this long (a hung or
# unreachable server) fails with a "read timed out" error instead of blocking
# forever, and so does a write that cannot hand the next WRITE_CHUNK_BYTES to the
# socket. `Options.timeoutMs` sets another; math.MAX_INT disables it.
def const DEFAULT_TIMEOUT_MS as int init 30000;

# A request is written in pieces of this size, the write deadline re-armed
# before each, so the timeout bounds a stalled send rather than the whole upload.
def const WRITE_CHUNK_BYTES as int init 16384;

# MAX_BODY_BYTES is the default response-body cap. The per-read deadline bounds
# a *stalled* peer, but a fast peer can stream unbounded data; without a size
# limit a hostile server (e.g. behind an untrusted feed URL) could drive the
# interpreter to OOM. 64 MiB is far past any API / feed / page response; a larger
# body is a catchable error, not a crash. A caller that genuinely needs a bigger
# (or unlimited) body sets `Options.maxBytes`. The cap
# counts body bytes only (the decoded data of a chunked body): the status line
# and headers have their own cap, MAX_HEADER_BYTES.
def const MAX_BODY_BYTES as int init 67108864;

# READ_PIECE_BYTES is the most one net.readN call reads of a Content-Length
# body; net.readN refuses more than 256 MiB a call.
def const READ_PIECE_BYTES as int init 67108864;

# MAX_HEADER_BYTES caps a response's status line and headers (each interim 1xx
# head too) whatever `maxBytes` says, so an unlimited body does not also mean
# unlimited headers. 1 MiB is far past real servers' heads (cookie-heavy ones
# run to tens of KiB). Under a body cap, a chunked body's framing - its size
# lines, CRLFs and trailers - may add max(cap, MAX_HEADER_BYTES) on top, so a
# server cannot hide a huge stream behind tiny chunks with long size lines.
def const MAX_HEADER_BYTES as int init 1048576;

# writeIdle writes b to conn in WRITE_CHUNK_BYTES pieces, re-arming the write
# deadline timeoutMs out before each one: an idle timeout, like the reads, so a
# large upload over a slow but live link is not cut off. 0 = no deadline.
func writeIdle(conn as net.Conn, b as bytes, timeoutMs as int) {
    def n as int init len($b);
    if ($timeoutMs <= 0) {
        net.writeBytes($conn, $b);
        return;
    }
    if ($n <= WRITE_CHUNK_BYTES) {
        net.setWriteDeadline($conn, $timeoutMs);
        net.writeBytes($conn, $b);
        return;
    }
    for (def off as int init 0; $off < $n; $off = $off + WRITE_CHUNK_BYTES) {
        def end as int init $off + WRITE_CHUNK_BYTES;
        if ($end > $n) {
            $end = $n;
        }
        net.setWriteDeadline($conn, $timeoutMs);
        net.writeBytes($conn, $b[$off..$end]);
    }
}

# dial opens the connection. A positive timeoutMs bounds the connect (and the
# TLS handshake) too, so a short timeout is not a 30 s wait on an unreachable
# host; 0 (no idle timeout) connects within net's default deadline (30 s).
func dial(u as Url, tls as net.TLSOptions, timeoutMs as int) {
    def addr as string init "{$u.host}:{$u.port}";
    def opts as net.ConnectOptions;
    if ($timeoutMs > 0) {
        $opts.timeoutMs = $timeoutMs;
    }
    if ($u.scheme == "https") {
        # The zero net.TLSOptions verifies against the system roots.
        $opts.tls = true;
        $opts.tlsOptions = $tls;
    }
    return net.connectWith($addr, $opts);
}

# --- API (exported) ------------------------------------------------

# sendCore is the single wire implementation - dial, send, read - returning the
# **raw response bytes** (head + framed body). A text body is UTF-8-encoded by
# its caller; a bytes body reaches the wire byte-for-byte. An `http://` URL
# ignores `lim.tls`.
func sendCore(
    method as string,
    url as string,
    headers as map of string to string,
    body as bytes,
    lim as Limits) {
    def u as Url init parseUrl($url);
    # Build (and validate) the request before opening a socket, so an injected
    # header / path throws without dialing and nothing malformed hits the wire.
    def head as bytes init encoding.encode("utf-8", buildHead($method, $u, $headers, len($body)));
    def conn as net.Conn init dial($u, $lim.tls, $lim.timeoutMs);
    # Close the socket exactly once whether the exchange succeeds or throws: a
    # read timeout or a parse error must not leak the connection (a poller
    # hitting timeouts would otherwise exhaust file descriptors).
    defer net.close($conn);
    # writeIdle re-arms the write deadline per chunk; readSock arms each response
    # read. A small body goes out with the head in one write; a large one is
    # written apart, not copied onto the head.
    if (len($body) <= WRITE_CHUNK_BYTES) {
        writeIdle($conn, binary.concat($head, $body), $lim.timeoutMs);
    } else {
        writeIdle($conn, $head, $lim.timeoutMs);
        writeIdle($conn, $body, $lim.timeoutMs);
    }
    # Read one framed response (Content-Length / chunked / read-to-EOF, per RFC
    # 9112 6.3), then let the deferred net.close retire the socket. A server that
    # ignores our Connection: close and holds the socket open (Cisco appliances,
    # some proxies) still terminates here as soon as the framing says the body is
    # complete, instead of blocking until the idle timeout.
    return readOneRaw($conn, $lim.timeoutMs, $lim.limit, $method);
}

# legacyOptions is the Options a deprecated positional call stands for. Its
# timeoutMs 0 meant "no idle timeout", which Options spells math.MAX_INT; a
# negative timeoutMs or maxBytes throws, as in Options.
func legacyOptions(timeoutMs as int, maxBytes as int, tls as TlsOptions) {
    def o as Options;
    $o.timeoutMs = $timeoutMs;
    if ($timeoutMs == 0) {
        $o.timeoutMs = math.MAX_INT;
    }
    $o.maxBytes = $maxBytes;
    $o.tls = $tls;
    return $o;
}

# textOf turns a BytesResponse into a text Response, decoding the body as
# UTF-8 (strict: a non-UTF-8 body throws kind "http").
func textOf(r as BytesResponse) {
    return Response{
        status: $r.status,
        statusText: $r.statusText,
        headers: $r.headers,
        body: bodyText($r.body)
    };
}

/**
 * Send one HTTP request with an explicit idle timeout and body cap, returning
 * the response. `timeoutMs` 0 is no idle timeout here (the connect then waits
 * up to 30 s); a negative `timeoutMs` or `maxBytes` throws.
 * @param method {string} the HTTP method (e.g. "GET", "POST")
 * @param url {string} the absolute request URL
 * @param headers {map of string to string} request headers ({} for none)
 * @param body {string} the request body ("" for no body)
 * @param timeoutMs {int} the idle timeout in milliseconds for each read and each 16 KiB written (0 = none)
 * @param maxBytes {int} the response-body cap (0 = 64 MiB default, `math.MAX_INT` = unlimited)
 * @return {Response} the parsed response
 * @throws {Error} as `send`
 * @deprecated use `http.send` with `http.Options{timeoutMs, maxBytes}`; removed in 0.29.0
 */
export func requestWith(
    method as string,
    url as string,
    headers as map of string to string,
    body as string,
    timeoutMs as int,
    maxBytes as int) {
    def t as TlsOptions;
    return send($method, $url, $headers, $body, legacyOptions($timeoutMs, $maxBytes, $t));
}

/**
 * Send one request with a **raw `bytes` body** and return the text `Response`.
 * @param method {string} the HTTP method (e.g. "POST")
 * @param url {string} the absolute request URL
 * @param headers {map of string to string} request headers (set your own Content-Type)
 * @param body {bytes} the raw request body
 * @param timeoutMs {int} the idle timeout in milliseconds for each read and each 16 KiB written (0 = none)
 * @param maxBytes {int} the response-body cap (0 = 64 MiB default, `math.MAX_INT` = unlimited)
 * @return {Response} the parsed response
 * @throws {Error} as `send`
 * @deprecated use `http.sendBytes` with `http.Options{timeoutMs, maxBytes}`; removed in 0.29.0
 */
export func requestRawBody(
    method as string,
    url as string,
    headers as map of string to string,
    body as bytes,
    timeoutMs as int,
    maxBytes as int) {
    def t as TlsOptions;
    return textOf(sendBytes(
        $method,
        $url,
        $headers,
        $body,
        legacyOptions($timeoutMs, $maxBytes, $t)));
}

/**
 * `requestRawBody` with explicit TLS options for an `https://` server.
 * @param method {string} the HTTP method
 * @param url {string} the absolute request URL
 * @param headers {map of string to string} request headers
 * @param body {bytes} the raw request body
 * @param timeoutMs {int} the idle timeout in milliseconds for each read and each 16 KiB written (0 = none)
 * @param maxBytes {int} the response-body cap (0 = 64 MiB default, `math.MAX_INT` = unlimited)
 * @param tls {TlsOptions} certificate-verification options for the TLS handshake
 * @return {Response} the parsed response
 * @throws {Error} as `send`
 * @deprecated use `http.sendBytes` with `http.Options{timeoutMs, maxBytes, tlsOptions}`;
 *   removed in 0.29.0
 */
export func requestRawBodyTls(
    method as string,
    url as string,
    headers as map of string to string,
    body as bytes,
    timeoutMs as int,
    maxBytes as int,
    tls as TlsOptions) {
    return textOf(sendBytes(
        $method,
        $url,
        $headers,
        $body,
        legacyOptions($timeoutMs, $maxBytes, $tls)));
}

/**
 * `requestWith` with explicit TLS options for an `https://` URL.
 * @param method {string} the HTTP method (e.g. "GET", "POST")
 * @param url {string} the absolute request URL
 * @param headers {map of string to string} request headers ({} for none)
 * @param body {string} the request body ("" for no body)
 * @param timeoutMs {int} the idle timeout in milliseconds for each read and each 16 KiB written (0 = none)
 * @param maxBytes {int} the response-body cap (0 = 64 MiB default, `math.MAX_INT` = unlimited)
 * @param tls {TlsOptions} certificate-verification options for the TLS handshake
 * @return {Response} the parsed response
 * @throws {Error} as `send`
 * @deprecated use `http.send` with `http.Options{timeoutMs, maxBytes, tlsOptions}`;
 *   removed in 0.29.0
 */
export func requestWithTls(
    method as string,
    url as string,
    headers as map of string to string,
    body as string,
    timeoutMs as int,
    maxBytes as int,
    tls as TlsOptions) {
    return send($method, $url, $headers, $body, legacyOptions($timeoutMs, $maxBytes, $tls));
}

/**
 * Send one request with explicit TLS options (default idle timeout and body
 * cap).
 * @param method {string} the HTTP method (e.g. "GET", "POST")
 * @param url {string} the absolute request URL
 * @param headers {map of string to string} request headers ({} for none)
 * @param body {string} the request body ("" for no body)
 * @param tls {TlsOptions} certificate-verification options for the TLS handshake
 * @return {Response} the parsed response
 * @throws {Error} as `send`
 * @deprecated use `http.send` with `http.Options{tlsOptions}`; removed in 0.29.0
 */
export func requestTls(
    method as string,
    url as string,
    headers as map of string to string,
    body as string,
    tls as TlsOptions) {
    def o as Options;
    $o.tls = $tls;
    return send($method, $url, $headers, $body, $o);
}

/**
 * Send one HTTP request and return the response (default idle timeout and body
 * cap, full TLS verification): `send` with zero `Options`.
 * @param method {string} the HTTP method (e.g. "GET", "POST")
 * @param url {string} the absolute request URL
 * @param headers {map of string to string} request headers ({} for none)
 * @param body {string} the request body ("" for no body)
 * @return {Response} the parsed response
 * @throws {Error} kind "http" if the response or the request is malformed, "limit" past a cap, "timeout", "closed" for a reply cut short, or a `net.NetError` (kind "net") when the network fails (see the module docs, "Errors")
 */
export func request(
    method as string,
    url as string,
    headers as map of string to string,
    body as string) {
    return send($method, $url, $headers, $body, defaultOptions());
}

/**
 * Send one request and return a raw-bytes `BytesResponse`, with explicit idle
 * timeout, body cap and TLS options.
 * @param method {string} the HTTP method (e.g. "GET")
 * @param url {string} the absolute request URL
 * @param headers {map of string to string} request headers ({} for none)
 * @param body {string} the request body ("" for no body)
 * @param timeoutMs {int} the idle timeout in milliseconds for each read and each 16 KiB written (0 = none)
 * @param maxBytes {int} the response-body cap (0 = 64 MiB default, `math.MAX_INT` = unlimited)
 * @param tls {TlsOptions} certificate-verification options for the TLS handshake
 * @return {BytesResponse} the response with a raw bytes body
 * @throws {Error} as `send`
 * @deprecated use `http.sendBytes` with `http.Options{timeoutMs, maxBytes, tlsOptions}`;
 *   removed in 0.29.0
 */
export func requestWithBytes(
    method as string,
    url as string,
    headers as map of string to string,
    body as string,
    timeoutMs as int,
    maxBytes as int,
    tls as TlsOptions) {
    return sendBytes(
        $method,
        $url,
        $headers,
        encoding.encode("utf-8", $body),
        legacyOptions($timeoutMs, $maxBytes, $tls));
}

/**
 * Send one request and return a raw-bytes `BytesResponse` (default idle timeout,
 * default 64 MiB cap, full TLS verification).
 * @param method {string} the HTTP method (e.g. "GET")
 * @param url {string} the absolute request URL
 * @param headers {map of string to string} request headers ({} for none)
 * @param body {string} the request body ("" for no body)
 * @return {BytesResponse} the response with a raw bytes body
 * @throws {Error} as `send`
 * @deprecated use `http.sendBytes` with zero `http.Options`; removed in 0.29.0
 */
export func requestBytes(
    method as string,
    url as string,
    headers as map of string to string,
    body as string) {
    return sendBytes($method, $url, $headers, encoding.encode("utf-8", $body), defaultOptions());
}

/**
 * GET a URL and return its body as raw `bytes` (the download shortcut):
 * `sendBytes` with zero `Options`.
 * @param url {string} the absolute request URL
 * @param headers {map of string to string} request headers ({} for none)
 * @return {BytesResponse} the response with a raw bytes body
 * @throws {Error} as `send`
 */
export func getBytes(url as string, headers as map of string to string) {
    def none as bytes;
    return sendBytes("GET", $url, $headers, $none, defaultOptions());
}

/**
 * Issue a GET request.
 * @param url {string} the absolute request URL
 * @param headers {map of string to string} request headers ({} for none)
 * @return {Response} the parsed response
 */
export func get(url as string, headers as map of string to string) {
    return request("GET", $url, $headers, "");
}

/**
 * Issue a POST request with `contentType` and `body`.
 * @param url {string} the absolute request URL
 * @param contentType {string} the Content-Type header value
 * @param body {string} the request body
 * @param headers {map of string to string} extra request headers ({} for none)
 * @return {Response} the parsed response
 */
export func post(
    url as string,
    contentType as string,
    body as string,
    headers as map of string to string) {
    def h as map of string to string init $headers;
    $h["Content-Type"] = $contentType;
    return request("POST", $url, $h, $body);
}

/**
 * Issue a PUT request with `contentType` and `body`.
 * @param url {string} the absolute request URL
 * @param contentType {string} the Content-Type header value
 * @param body {string} the request body
 * @param headers {map of string to string} extra request headers ({} for none)
 * @return {Response} the parsed response
 */
export func put(
    url as string,
    contentType as string,
    body as string,
    headers as map of string to string) {
    def h as map of string to string init $headers;
    $h["Content-Type"] = $contentType;
    return request("PUT", $url, $h, $body);
}

/**
 * Issue a PATCH request (a partial update) with `contentType` and `body`.
 * @param url {string} the absolute request URL
 * @param contentType {string} the Content-Type header value
 * @param body {string} the request body
 * @param headers {map of string to string} extra request headers ({} for none)
 * @return {Response} the parsed response
 */
export func patch(
    url as string,
    contentType as string,
    body as string,
    headers as map of string to string) {
    def h as map of string to string init $headers;
    $h["Content-Type"] = $contentType;
    return request("PATCH", $url, $h, $body);
}

/**
 * Issue a DELETE request.
 * @param url {string} the absolute request URL
 * @param headers {map of string to string} request headers ({} for none)
 * @return {Response} the parsed response
 */
export func delete(url as string, headers as map of string to string) {
    return request("DELETE", $url, $headers, "");
}

/**
 * Issue a HEAD request (status and headers, no body).
 * @param url {string} the absolute request URL
 * @param headers {map of string to string} request headers ({} for none)
 * @return {Response} the parsed response (empty body)
 */
export func head(url as string, headers as map of string to string) {
    return request("HEAD", $url, $headers, "");
}

/**
 * Issue an OPTIONS request (capability probe; read the `Allow` header).
 * @param url {string} the absolute request URL
 * @param headers {map of string to string} request headers ({} for none)
 * @return {Response} the parsed response
 */
export func options(url as string, headers as map of string to string) {
    return request("OPTIONS", $url, $headers, "");
}

/**
 * Read a response header by name, case-insensitively.
 * @param resp {Response} the response to read from
 * @param name {string} the header name (case-insensitive)
 * @return {string} the header value, or "" if absent
 */
export func header(resp as Response, name as string) {
    def key as string init strings.lower($name);
    if (maps.has($resp.headers, $key)) {
        return $resp.headers[$key];
    }
    return "";
}

/**
 * Reports whether a request to `url` would cross the network in the clear:
 * true for `http://` to any host but a loopback one (127.0.0.0/8, `[::1]`,
 * `localhost`), false for `https://` and loopback `http://`. The host is read
 * the way a request reads it (userinfo cut at the last `@`), so
 * `http://127.0.0.1@evil.example/` is remote. Modules that send credentials
 * use it to refuse plain HTTP unless their caller sets `allowInsecureHttp`.
 * @param url {string} an http:// or https:// URL
 * @return {bool} true when credentials sent to `url` would be readable on the wire
 * @throws {Error} kind "http" for a URL a request would refuse (scheme, host, port)
 * @example
 * http.isCleartextRemote("http://192.0.2.7/api");   # true
 * http.isCleartextRemote("http://127.0.0.1:8080/"); # false
 */
export func isCleartextRemote(url as string) {
    def u as Url init parseUrl($url);
    if ($u.scheme == "https") {
        return false;
    }
    def host as string init strings.lower($u.host);
    if ($host == "localhost" or $host == "[::1]") {
        return false;
    }
    return not regex.matches('^127(\.(25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])){3}$', $host);
}

# --- request policy + persistent sessions --------------------------

# The default first-retry backoff (doubled per attempt) and the ceiling any
# single backoff is clamped to, so an adversarial Retry-After can't park the
# program for hours.
def const DEFAULT_BACKOFF_MS as int init 250;
def const MAX_BACKOFF_MS as int init 30000;

/**
 * Per-request policy for `send` and a persistent `Session`. The zero value (from
 * `http.defaultOptions()` or `def o as http.Options;`) is the safe default: the standard
 * timeout and body cap, TLS fully verified, and **no** redirect-following or
 * retrying (so a bare `send` behaves like `request`). Opt into a behaviour by
 * setting its field. A negative number in any field throws `Error{kind: "http"}`.
 * @field timeoutMs {int} idle timeout for each read and each 16 KiB written, and the connect's deadline (0 = the 30 s default; `math.MAX_INT` = no idle timeout, the connect still giving up after 30 s)
 * @field maxBytes {int} response-body cap (0 = the 64 MiB default; `math.MAX_INT` = unlimited)
 * @field maxRedirects {int} how many 3xx redirects to follow (0 = none; the 3xx is returned)
 * @field maxRetries {int} how many times to retry a 429 / 5xx (0 = none), with exponential backoff honouring `Retry-After`; any method is retried, a `POST` too
 * @field backoffMs {int} base backoff for the first retry (0 = 250ms), doubled each attempt and capped at 30s
 * @field tls {TlsOptions} deprecated, removed in 0.29.0: use `tlsOptions`. Setting both throws `Error{kind: "http"}`
 * @field tlsOptions {net.TLSOptions} TLS verification options for `https://` (zero = full verification against the system roots)
 * @field allowCrossOriginRedirect {bool} keep every header and the cookies across a redirect to a different origin (default false = keep only content-negotiation, caching and range headers, and drop the cookies)
 * @field allowInsecureRedirect {bool} follow a redirect from `https://` to plain `http://` (default false = throw `Error{kind: "http"}` instead of sending the next request in the clear)
 */
export def struct Options {
    timeoutMs as int,
    maxBytes as int,
    maxRedirects as int,
    maxRetries as int,
    backoffMs as int,
    tls as TlsOptions,
    tlsOptions as net.TLSOptions,
    allowCrossOriginRedirect as bool,
    allowInsecureRedirect as bool
};

# Limits is an Options' timeout, body cap and TLS settings as the wire code
# takes them: `timeoutMs` 0 is no idle timeout, `limit` -1 no body cap.
def struct Limits {
    timeoutMs as int,
    limit as int,
    tls as net.TLSOptions
};

# optionFail throws the error for an Options field out of range.
func optionFail(field as string, value as int, meaning as string) {
    throw Error{
        kind: "http",
        message: "http: Options.{$field} must be >= 0 ({$meaning}), got {$value}"
    };
}

# limitsOf checks an Options (a negative number throws, so does setting both
# `tls` and `tlsOptions`) and resolves its timeout, cap and TLS settings:
# 0 is the default, math.MAX_INT none.
func limitsOf(o as Options) {
    if ($o.timeoutMs < 0) {
        optionFail("timeoutMs", $o.timeoutMs, "0 = the 30000 default, math.MAX_INT = none");
    }
    if ($o.maxBytes < 0) {
        optionFail("maxBytes", $o.maxBytes, "0 = the 64 MiB default, math.MAX_INT = unlimited");
    }
    if ($o.maxRedirects < 0) {
        optionFail("maxRedirects", $o.maxRedirects, "0 = none");
    }
    if ($o.maxRetries < 0) {
        optionFail("maxRetries", $o.maxRetries, "0 = none");
    }
    if ($o.backoffMs < 0) {
        optionFail("backoffMs", $o.backoffMs, "0 = the 250 default");
    }
    def lim as Limits;
    $lim.timeoutMs = $o.timeoutMs;
    if ($o.timeoutMs == 0) {
        $lim.timeoutMs = DEFAULT_TIMEOUT_MS;
    } elseif ($o.timeoutMs == math.MAX_INT) {
        $lim.timeoutMs = 0;
    }
    $lim.limit = $o.maxBytes;
    if ($o.maxBytes == 0) {
        $lim.limit = MAX_BODY_BYTES;
    } elseif ($o.maxBytes == math.MAX_INT) {
        $lim.limit = -1;
    }
    def oldSet as bool init $o.tls.skipVerify or len($o.tls.caCert) > 0;
    def newSet as bool init $o.tlsOptions.skipVerify or len($o.tlsOptions.caCert) > 0;
    if ($oldSet and $newSet) {
        throw Error{
            kind: "http",
            message: "http: Options.tls and Options.tlsOptions are both set; " +
                "set tlsOptions alone (tls is deprecated)"
        };
    }
    $lim.tls = $o.tlsOptions;
    if ($oldSet) {
        $lim.tls.skipVerify = $o.tls.skipVerify;
        $lim.tls.caCert = $o.tls.caCert;
    }
    return $lim;
}

/**
 * The zero `Options` (default timeout / cap, verified TLS, no redirects, no
 * retries), for inline use: `http.send(m, u, {}, "", http.defaultOptions())`.
 * Set fields on the result to opt into behaviour.
 * @return {Options} the default options
 */
export func defaultOptions() {
    def o as Options;
    return $o;
}

/**
 * Build an `Authorization` value for HTTP Basic auth (base64 of "user:password"),
 * to pass as a request header. Mirrors `rest.basic`.
 * @param user {string} the username
 * @param pass {string} the password
 * @return {string} the "Basic <base64>" header value
 */
export func basic(user as string, pass as string) {
    def creds as bytes init encoding.encode("utf-8", $user + ":" + $pass);
    return "Basic " + encoding.toText("base64", $creds);
}

# --- HTTP Digest authentication (RFC 7616, RFC 2617) -----------------

# One challenge of a `WWW-Authenticate` header: its scheme and its parameters
# (names lower-cased, quoted values unquoted).
def struct AuthChallenge {
    scheme as string,
    params as map of string to string
};

# The Digest algorithms answered, by their RFC 7616 names, with the `hash`
# algorithm each uses. Listed best first: the strongest offered is chosen.
def const DIGEST_ALGORITHMS as list of string init ["SHA-256", "SHA-256-SESS", "MD5", "MD5-SESS"];

# A control character: refused in anything written into the Authorization
# header, which a server could otherwise use to inject header lines.
def const DIGEST_CONTROL_RE as string init '[\x00-\x1f\x7f]';

func digestFail(message as string) {
    throw Error{kind: "http", message: "http: digest: " + $message};
}

# charAt is the one-character string at rune index i (strings are not
# indexable by position).
func charAt(text as string, i as int) {
    return $text[$i..$i + 1];
}

# A parameter value read from a challenge, and the position after it.
def struct ParamValue {
    value as string,
    next as int
};

# paramValue reads a quoted-string (backslash escapes undone) or a token from
# position i of a challenge header.
func paramValue(header as string, i as int) {
    def n as int init len($header);
    def k as int init $i;
    if ($k >= $n or charAt($header, $k) != "\"") {
        while ($k < $n and charAt($header, $k) != "," and charAt($header, $k) != " ") {
            $k = $k + 1;
        }
        return ParamValue{value: $header[$i..$k], next: $k};
    }
    $k = $k + 1;
    def parts as list of string init [];
    while ($k < $n and charAt($header, $k) != "\"") {
        if (charAt($header, $k) == "\\" and $k + 1 < $n) {
            $k = $k + 1;
        }
        $parts[] = charAt($header, $k);
        $k = $k + 1;
    }
    return ParamValue{value: strings.join($parts, ""), next: $k + 1};
}

/**
 * Split a `WWW-Authenticate` value into its challenges. Several challenges
 * may share one value (and do, when repeated headers are joined with ", "),
 * so the scan tells a new scheme from a parameter by the `=` after it; commas
 * inside quoted strings do not split.
 * @param header {string} the header value
 * @return {list of AuthChallenge} the challenges in order
 */
func authChallenges(header as string) {
    def out as list of AuthChallenge init [];
    def n as int init len($header);
    def i as int init 0;
    while ($i < $n) {
        while ($i < $n and
            (charAt($header, $i) == " " or charAt($header, $i) == "," or charAt($header, $i) == "\t")) {
            $i = $i + 1;
        }
        if ($i >= $n) {
            break;
        }
        def start as int init $i;
        while ($i < $n and not lists.contains([" ", ",", "=", "\t"], charAt($header, $i))) {
            $i = $i + 1;
        }
        def word as string init $header[$start..$i];
        def j as int init $i;
        while ($j < $n and (charAt($header, $j) == " " or charAt($header, $j) == "\t")) {
            $j = $j + 1;
        }
        if ($j < $n and charAt($header, $j) == "=" and len($out) > 0) {
            # A parameter of the current challenge (or a token68's padding).
            $i = $j + 1;
            while ($i < $n and (charAt($header, $i) == " " or charAt($header, $i) == "\t")) {
                $i = $i + 1;
            }
            def v as ParamValue init paramValue($header, $i);
            $i = $v.next;
            def value as string init $v.value;
            def last as int init len($out) - 1;
            $out[$last].params[strings.lower($word)] = $value;
        } elseif ($word != "") {
            $out[] = AuthChallenge{scheme: $word, params: {}};
        } else {
            $i = $i + 1;
        }
    }
    return $out;
}

/**
 * The Digest challenge to answer: the one with the strongest supported
 * algorithm (SHA-256 before MD5) whose `qop`, if any, offers `auth` or
 * `auth-int`.
 * @param header {string} the `WWW-Authenticate` value
 * @return {AuthChallenge} the challenge
 * @throws {Error} kind "http" when no Digest challenge can be answered
 */
func pickDigest(header as string) {
    def best as AuthChallenge init AuthChallenge{scheme: "", params: {}};
    def rank as int init len(DIGEST_ALGORITHMS);
    for (def c in authChallenges($header)) {
        if (strings.lower($c.scheme) != "digest" or not maps.has($c.params, "nonce")) {
            continue;
        }
        def alg as string init "MD5";
        if (maps.has($c.params, "algorithm")) {
            $alg = strings.upper($c.params["algorithm"]);
        }
        def r as int init lists.indexOf(DIGEST_ALGORITHMS, $alg);
        if ($r < 0 or $r >= $rank) {
            continue;
        }
        if (maps.has($c.params, "qop")) {
            def offered as list of string init [];
            for (def q in strings.split($c.params["qop"], ",")) {
                $offered[] = strings.lower(strings.trim($q));
            }
            if (not lists.contains($offered, "auth") and not lists.contains($offered, "auth-int")) {
                continue;
            }
        }
        $best = $c;
        $best.params["algorithm"] = $alg;
        $rank = $r;
    }
    if ($best.scheme == "") {
        digestFail("no Digest challenge with a supported algorithm (SHA-256, MD5) and qop");
    }
    return $best;
}

# digestHex is H(text): the lower-case hex digest of its UTF-8 bytes.
func digestHex(text as string, algorithm as string) {
    def alg as string init "md5";
    if (strings.startsWith($algorithm, "SHA-256")) {
        $alg = "sha256";
    }
    return encoding.toText("hex", hash.compute($alg, encoding.encode("utf-8", $text)));
}

# digestQuote writes a quoted-string, escaping `\` and `"`.
func digestQuote(text as string) {
    return "\"" + strings.replace(strings.replace($text, "\\", "\\\\"), "\"", "\\\"") + "\"";
}

/**
 * The Authorization value answering one Digest challenge, with a given client
 * nonce (`digestAuthorization` draws a random one).
 * @param c {AuthChallenge} the challenge, from `pickDigest`
 * @param method {string} the request method
 * @param target {string} the request target as sent (path and query)
 * @param user {string} the user name
 * @param password {string} the password
 * @param body {string} the request body (hashed for `qop=auth-int`)
 * @param cnonce {string} the client nonce
 * @return {string} the header value
 */
func digestAnswer(
    c as AuthChallenge,
    method as string,
    target as string,
    user as string,
    password as string,
    body as string,
    cnonce as string) {
    def realm as string init "";
    if (maps.has($c.params, "realm")) {
        $realm = $c.params["realm"];
    }
    def nonce as string init $c.params["nonce"];
    def opaque as string init "";
    if (maps.has($c.params, "opaque")) {
        $opaque = $c.params["opaque"];
    }
    # Every value written back into the header: a server-sent CR / LF here
    # would inject header lines into our request.
    if (regex.matches(DIGEST_CONTROL_RE, $realm + $nonce + $opaque + $user + $target)) {
        digestFail("a control character in the challenge, the user name or the target");
    }
    def alg as string init $c.params["algorithm"];
    def ha1 as string init digestHex($user + ":" + $realm + ":" + $password, $alg);
    if (strings.endsWith($alg, "-SESS")) {
        $ha1 = digestHex($ha1 + ":" + $nonce + ":" + $cnonce, $alg);
    }
    def qop as string init "";
    if (maps.has($c.params, "qop")) {
        def offered as list of string init [];
        for (def q in strings.split($c.params["qop"], ",")) {
            $offered[] = strings.lower(strings.trim($q));
        }
        $qop = "auth";
        if (not lists.contains($offered, "auth")) {
            $qop = "auth-int";
        }
    }
    def ha2 as string init digestHex($method + ":" + $target, $alg);
    if ($qop == "auth-int") {
        $ha2 = digestHex($method + ":" + $target + ":" + digestHex($body, $alg), $alg);
    }
    def nc as string init "00000001";
    def response as string init "";
    if ($qop == "") {
        # RFC 2069 form: a server that offers no qop.
        $response = digestHex($ha1 + ":" + $nonce + ":" + $ha2, $alg);
    } else {
        $response = digestHex(
            $ha1 + ":" + $nonce + ":" + $nc + ":" + $cnonce + ":" + $qop + ":" + $ha2,
            $alg);
    }
    def name as string init digestQuote($user);
    def userhash as bool init maps.has($c.params, "userhash") and
        strings.lower($c.params["userhash"]) == "true";
    if ($userhash) {
        $name = digestQuote(digestHex($user + ":" + $realm, $alg));
    }
    def parts as list of string init [
        "username=" + $name,
        "realm=" + digestQuote($realm),
        "uri=" + digestQuote($target),
        "algorithm=" + $alg,
        "nonce=" + digestQuote($nonce)
    ];
    if ($qop != "") {
        $parts[] = "nc=" + $nc;
        $parts[] = "cnonce=" + digestQuote($cnonce);
        $parts[] = "qop=" + $qop;
    }
    $parts[] = "response=" + digestQuote($response);
    if ($opaque != "") {
        $parts[] = "opaque=" + digestQuote($opaque);
    }
    if ($userhash) {
        $parts[] = "userhash=true";
    }
    return "Digest " + strings.join($parts, ", ");
}

/**
 * Build an `Authorization` value answering a Digest challenge (RFC 7616; the
 * RFC 2617 / 2069 forms too). Picks the strongest supported challenge
 * (SHA-256, then SHA-256-sess, MD5, MD5-sess), `qop=auth` when offered, else
 * `auth-int` (which hashes `body`), with a fresh random client nonce and
 * `nc=00000001`. For a request made with `sendBytes`, a `Session` or any
 * other call: `sendDigest` does the whole round trip for `send`.
 * @param challenge {string} the 401 response's `WWW-Authenticate` value
 * @param method {string} the method of the request to authenticate
 * @param url {string} its URL; the `uri` answered is its request target
 * @param user {string} the user name
 * @param password {string} the password
 * @param body {string} the request body ("" for none)
 * @return {string} the header value, "Digest username=..."
 * @throws {Error} kind "http" when no Digest challenge can be answered, or a
 *   value would put a control character into the header
 */
export func digestAuthorization(
    challenge as string,
    method as string,
    url as string,
    user as string,
    password as string,
    body as string) {
    def c as AuthChallenge init pickDigest($challenge);
    def cnonce as string init encoding.toText("hex", crypto.randBytes(16));
    return digestAnswer($c, $method, parseUrl($url).path, $user, $password, $body, $cnonce);
}

/**
 * `send` with HTTP Digest authentication: the request goes out without
 * credentials, and a `401` carrying a Digest challenge is answered once (and
 * once more when the server reports the nonce `stale`). Any other response,
 * a 401 offering only another scheme included, is returned as is.
 *
 * The challenge round follows no redirect, so the credentials answer the URL
 * given and no other origin; a 3xx there is returned to the caller. The
 * authenticated request then uses `options` in full (redirects, retries),
 * and `send` drops `Authorization` on a cross-origin hop. The body is sent
 * with both requests.
 * @param method {string} the HTTP method
 * @param url {string} the absolute request URL
 * @param headers {map of string to string} request headers ({} for none); an
 *   `Authorization` among them is replaced
 * @param body {string} the request body ("" for none)
 * @param options {Options} the request policy
 * @param user {string} the user name
 * @param password {string} the password
 * @return {Response} the final response
 * @throws {Error} kind "http" as `send`, or as `digestAuthorization`
 */
export func sendDigest(
    method as string,
    url as string,
    headers as map of string to string,
    body as string,
    options as Options,
    user as string,
    password as string) {
    def plain as map of string to string init {};
    for (def k in $headers) {
        if (strings.lower($k) != "authorization") {
            $plain[$k] = $headers[$k];
        }
    }
    def first as Options init $options;
    $first.maxRedirects = 0;
    def r as Response init send($method, $url, $plain, $body, $first);
    def tries as int init 0;
    while ($r.status == 401 and $tries < 2) {
        def challenge as string init header($r, "WWW-Authenticate");
        def c as AuthChallenge;
        try {
            $c = pickDigest($challenge);
        } catch (e) {
            # No Digest challenge (Basic only, or none): the 401 is the answer.
            return $r;
        }
        # A second round only for a stale nonce: a wrong password stays a 401.
        if ($tries == 1 and
            not (maps.has($c.params, "stale") and strings.lower($c.params["stale"]) == "true")) {
            return $r;
        }
        def authed as map of string to string init $plain;
        $authed["Authorization"] = digestAuthorization(
            $challenge,
            $method,
            $url,
            $user,
            $password,
            $body);
        $r = send($method, $url, $authed, $body, $options);
        $tries = $tries + 1;
    }
    return $r;
}

# isAllDigits reports whether s is one or more ASCII digits (a numeric header).
func isAllDigits(s as string) {
    return regex.matches('^[0-9]+$', $s);
}

# --- cookie jar (private) ------------------------------------------
# A deliberately small, session-scoped jar: cookie name -> value, no domain /
# path / expiry scoping. It preserves *multiple* Set-Cookie lines (read from the
# raw header block, before parseHeaders comma-folds them) so no cookie is lost,
# and replays them as one `Cookie` header. A full RFC 6265 store (attribute
# scoping, expiry) is a documented follow-up.

# setCookiesFromRaw returns each Set-Cookie line's value from a raw response's
# header block, un-folded (one entry per header line).
func setCookiesFromRaw(raw as bytes) {
    def out as list of string init [];
    def hend as int init headerEnd($raw);
    if ($hend < 0) {
        return $out;
    }
    def lines as list of string init strings.split(bytesToStr($raw, 0, $hend), "\r\n");
    for (def line in $lines) {
        def colon as int init strings.indexOf($line, ":");
        if ($colon > 0) {
            def name as string init strings.lower(strings.trim($line[..$colon]));
            if ($name == "set-cookie") {
                $out[] = strings.trim($line[$colon + 1..]);
            }
        }
    }
    return $out;
}

# jarAdd stores one Set-Cookie line's name=value (attributes after the first ";"
# are ignored), returning the updated jar.
func jarAdd(jar as map of string to string, setCookie as string) {
    def pair as string init $setCookie;
    def semi as int init strings.indexOf($setCookie, ";");
    if ($semi >= 0) {
        $pair = $setCookie[..$semi];
    }
    def eq as int init strings.indexOf($pair, "=");
    if ($eq < 0) {
        return $jar;
    }
    def name as string init strings.trim($pair[..$eq]);
    if (len($name) == 0) {
        return $jar;
    }
    def out as map of string to string init $jar;
    $out[$name] = strings.trim($pair[$eq + 1..]);
    return $out;
}

# jarHeader renders the jar as a "Cookie: a=1; b=2" value ("" when empty).
func jarHeader(jar as map of string to string) {
    def out as string init "";
    def first as bool init true;
    for (def k in $jar) {
        if (not $first) {
            $out = $out + "; ";
        }
        $out = $out + $k + "=" + $jar[$k];
        $first = false;
    }
    return $out;
}

# withCookies returns a copy of headers with a Cookie header set from the jar,
# unless the caller already supplied one (their value wins).
func withCookies(headers as map of string to string, jar as map of string to string) {
    def out as map of string to string init $headers;
    def cookieHdr as string init jarHeader($jar);
    if (len($cookieHdr) > 0 and not hasHeaderCI($out, "cookie")) {
        $out["Cookie"] = $cookieHdr;
    }
    return $out;
}

# --- redirect + retry (private) ------------------------------------

# isRedirect reports whether a status is one this client follows.
func isRedirect(status as int) {
    return $status == 301 or $status == 302 or $status == 303 or
        $status == 307 or $status == 308;
}

# isRetryable reports whether a status warrants a backoff-retry (429 or any 5xx).
func isRetryable(status as int) {
    return $status == 429 or ($status >= 500 and $status <= 599);
}

# retryDelayMs computes the backoff for `attempt` (0-based): base * 2^attempt,
# raised to the server's Retry-After (retryAfterMs) when it asked for longer,
# and clamped to MAX_BACKOFF_MS.
func retryDelayMs(resp as BytesResponse, attempt as int, baseMs as int) {
    def base as int init $baseMs;
    if ($base <= 0) {
        $base = DEFAULT_BACKOFF_MS;
    }
    def delay as int init $base;
    def k as int init 0;
    # Stop doubling at the ceiling: many attempts would overflow an int.
    while ($k < $attempt and $delay < MAX_BACKOFF_MS) {
        $delay = $delay * 2;
        $k = $k + 1;
    }
    if (maps.has($resp.headers, "retry-after")) {
        def raMs as int init retryAfterMs(strings.trim($resp.headers["retry-after"]));
        if ($raMs > $delay) {
            $delay = $raMs;
        }
    }
    if ($delay > MAX_BACKOFF_MS) {
        $delay = MAX_BACKOFF_MS;
    }
    return $delay;
}

# retryAfterMs reads a Retry-After value (RFC 9110 10.2.3) as a wait in
# milliseconds, at most MAX_BACKOFF_MS: delay-seconds, or an HTTP-date, which
# is a wait until then and 0 when it has passed. -1 when the value is
# malformed, so the caller treats it as absent. `time` holds dates up to the
# year 2262 only; a later one is past the cap anyway. Leading zeros do not
# count towards a delay's length: `0000001` is 1 s.
func retryAfterMs(value as string) {
    if (isAllDigits($value)) {
        def digits as string init regex.replace('^0+', $value, "");
        if ($digits == "") {
            return 0;
        }
        # A long number is past the ceiling anyway, and would overflow.
        if (len($digits) > 6) {
            return MAX_BACKOFF_MS;
        }
        return lowerOf(convert.toInt($digits) * 1000, MAX_BACKOFF_MS);
    }
    def stamp as string init httpDateStamp($value);
    if ($stamp == "") {
        return -1;
    }
    def year as int init convert.toInt($stamp[7..11]);
    if ($year < 1970) {
        return 0;
    }
    if ($year >= 2262) {
        return MAX_BACKOFF_MS;
    }
    def at as time.Time;
    try {
        $at = time.parse($stamp, "%d %b %Y %H:%M:%S");
    } catch (e) {
        # an impossible date (31 Feb) or a leap second: malformed
        return -1;
    }
    def ms as int init time.milliseconds(time.sub($at, time.now()));
    if ($ms <= 0) {
        return 0;
    }
    return lowerOf($ms, MAX_BACKOFF_MS);
}

# lowerOf returns the smaller of two ints.
func lowerOf(a as int, b as int) {
    if ($a < $b) {
        return $a;
    }
    return $b;
}

def const HTTP_MONTHS as string init '(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)';
def const HTTP_CLOCK as string init '([0-9]{2}:[0-9]{2}:[0-9]{2})';

# httpDateStamp reads an HTTP-date (RFC 9110 5.6.7) as "DD Mon YYYY HH:MM:SS"
# (GMT), or "" when it is none: the IMF-fixdate `Sun, 06 Nov 1994 08:49:37
# GMT`, and the two obsolete forms a recipient must accept, RFC 850's `Sunday,
# 06-Nov-94 08:49:37 GMT` and asctime's `Sun Nov  6 08:49:37 1994`. Names are
# case-sensitive, as the grammar says. A two-digit year more than 50 years
# ahead is the most recent past year with those digits.
func httpDateStamp(value as string) {
    def m as regex.Match init regex.find(
        '^(Mon|Tue|Wed|Thu|Fri|Sat|Sun), ([0-9]{2}) ' + HTTP_MONTHS + ' ([0-9]{4}) ' + HTTP_CLOCK +
            ' GMT$',
        $value);
    if ($m.start >= 0) {
        return $m.groups[1] + " " + $m.groups[2] + " " + $m.groups[3] + " " + $m.groups[4];
    }
    $m = regex.find(
        '^(Monday|Tuesday|Wednesday|Thursday|Friday|Saturday|Sunday), ([0-9]{2})-' + HTTP_MONTHS +
            '-([0-9]{2}) ' + HTTP_CLOCK + ' GMT$',
        $value);
    if ($m.start >= 0) {
        def now as int init time.year(time.utc());
        def year as int init $now - $now % 100 + convert.toInt($m.groups[3]);
        if ($year > $now + 50) {
            $year = $year - 100;
        } elseif ($year <= $now - 50) {
            $year = $year + 100;
        }
        return $m.groups[1] + " " + $m.groups[2] + " " + convert.toString($year) + " " +
            $m.groups[4];
    }
    $m = regex.find(
        '^(Mon|Tue|Wed|Thu|Fri|Sat|Sun) ' + HTTP_MONTHS + ' ( [1-9]|[0-9]{2}) ' + HTTP_CLOCK +
            ' ([0-9]{4})$',
        $value);
    if ($m.start >= 0) {
        def day as string init $m.groups[2];
        if (strings.startsWith($day, " ")) {
            $day = "0" + $day[1..];
        }
        return $day + " " + $m.groups[1] + " " + $m.groups[4] + " " + $m.groups[3];
    }
    return "";
}

# originOf returns the "scheme://host[:port]" of a URL (its origin).
func originOf(url as string) {
    def u as Url init parseUrl($url);
    return $u.scheme + "://" + hostHeader($u);
}

# resolveLocation resolves a Location value against the current URL per RFC
# 3986 section 5 (`uri.resolve`): an absolute URL is used as-is, `//host/p`
# keeps the current scheme, and a relative one (`../x`, `?page=2`) resolves
# against the current path, dot segments removed.
func resolveLocation(curUrl as string, loc as string) {
    return uri.resolve($curUrl, strings.trim($loc));
}

# isDowngrade reports whether a redirect leaves https for plain http.
func isDowngrade(curUrl as string, nextUrl as string) {
    def a as Url init parseUrl($curUrl);
    def b as Url init parseUrl($nextUrl);
    return $a.scheme == "https" and $b.scheme == "http";
}

# CROSS_ORIGIN_HEADERS are the only request headers (lowercase) that survive a
# redirect to a different origin: content negotiation, caching and range
# headers that carry nothing a server could abuse. Any other header may hold a
# secret (`PRIVATE-TOKEN`, `X-API-Key`, `api-key`, ...), and a caller-set `Host`
# names the old server, so both are dropped.
def const CROSS_ORIGIN_HEADERS as list of string init [
    "accept",
    "accept-encoding",
    "accept-language",
    "cache-control",
    "content-language",
    "content-type",
    "if-match",
    "if-modified-since",
    "if-none-match",
    "if-range",
    "if-unmodified-since",
    "pragma",
    "range",
    "user-agent"
];

# stripCredentialHeaders returns a copy of headers holding only those in
# CROSS_ORIGIN_HEADERS (case-insensitive). Applied before a cross-origin redirect
# hop so a hostile server cannot redirect a credentialed request to an attacker
# origin and harvest a token or cookie in whatever header it travels.
func stripCredentialHeaders(headers as map of string to string) {
    def out as map of string to string init {};
    for (def k in $headers) {
        if (lists.contains(CROSS_ORIGIN_HEADERS, strings.lower($k))) {
            $out[$k] = $headers[$k];
        }
    }
    return $out;
}

# sendOnce runs one request through the retry loop (no redirects): it dials,
# sends (Connection: close), and retries a 429 / 5xx up to opts.maxRetries with
# backoff. Returns the raw response bytes.
func sendOnce(
    method as string,
    url as string,
    headers as map of string to string,
    body as bytes,
    opts as Options,
    lim as Limits) {
    def attempt as int init 0;
    def raw as bytes;
    while (true) {
        $raw = sendCore($method, $url, $headers, $body, $lim);
        if ($attempt >= $opts.maxRetries) {
            return $raw;
        }
        def r as BytesResponse init parseRaw($raw);
        if (not isRetryable($r.status)) {
            return $raw;
        }
        time.sleep(time.fromMilliseconds(retryDelayMs($r, $attempt, $opts.backoffMs)));
        $attempt = $attempt + 1;
    }
    return $raw;
}

# sendPolicy is send / sendBytes: the request policy over a bytes body,
# returning the final response with its raw body. Without redirects or retries
# it is the bare one-shot request, parsed once.
func sendPolicy(
    method as string,
    url as string,
    headers as map of string to string,
    body as bytes,
    options as Options) {
    def lim as Limits init limitsOf($options);
    if ($options.maxRedirects == 0 and $options.maxRetries == 0) {
        return parseRaw(sendCore($method, $url, $headers, $body, $lim));
    }
    def curMethod as string init $method;
    def curUrl as string init $url;
    def curBody as bytes init $body;
    # curHeaders is sanitized (only CROSS_ORIGIN_HEADERS kept) on a cross-origin hop.
    def curHeaders as map of string to string init $headers;
    def redirectsLeft as int init $options.maxRedirects;
    def jar as map of string to string init {};
    while (true) {
        def raw as bytes init sendOnce(
            $curMethod,
            $curUrl,
            withCookies($curHeaders, $jar),
            $curBody,
            $options,
            $lim);
        def r as BytesResponse init parseRaw($raw);
        for (def sc in setCookiesFromRaw($raw)) {
            $jar = jarAdd($jar, $sc);
        }
        def follow as bool init isRedirect($r.status) and $redirectsLeft > 0 and
            maps.has($r.headers, "location");
        if (not $follow) {
            return $r;
        }
        def nextUrl as string init resolveLocation($curUrl, $r.headers["location"]);
        # An https -> http hop would send the next request (path, and the body
        # on a 307 / 308) unencrypted; refuse it unless the caller opted in.
        if (not $options.allowInsecureRedirect and isDowngrade($curUrl, $nextUrl)) {
            throw Error{kind: "http", message: "http: refused redirect from https:// to http://"};
        }
        # A redirect to a different origin drops every header that may carry a
        # credential and the cookie jar (whose entries are unscoped), unless the
        # caller explicitly opted into cross-origin credential forwarding.
        if (not $options.allowCrossOriginRedirect and originOf($nextUrl) != originOf($curUrl)) {
            $curHeaders = stripCredentialHeaders($curHeaders);
            $jar = {};
        }
        $curUrl = $nextUrl;
        if ($r.status == 303 or
            (($r.status == 301 or $r.status == 302) and $curMethod == "POST")) {
            $curMethod = "GET";
            $curBody = $curBody[0..0];
        }
        $redirectsLeft = $redirectsLeft - 1;
    }
    return BytesResponse{status: 0, statusText: "", headers: {}, body: $body[0..0]};
}

/**
 * Send a request with a policy: follow up to `options.maxRedirects` 3xx
 * redirects, retry a 429 / 5xx up to `options.maxRetries` with exponential
 * backoff (honouring `Retry-After`, seconds or an HTTP-date), and carry cookies across the
 * redirect chain. A `303` (and a `301`/`302` on a `POST`) becomes a bodyless
 * `GET`; `307`/`308` preserve the method and body. With a zero `Options` it is a
 * one-shot request, exactly like `request`.
 * @param method {string} the HTTP method
 * @param url {string} the absolute request URL
 * @param headers {map of string to string} request headers ({} for none)
 * @param body {string} the request body ("" for none), sent UTF-8-encoded
 * @param options {Options} the request policy
 * @return {Response} the final response (after any redirects / retries)
 * @throws {Error} kind "http" if the response or the request is malformed or `options` is out of range, "limit" past a cap, "timeout", "closed" for a reply cut short, or a `net.NetError` (kind "net") when the network fails (see the module docs, "Errors")
 */
export func send(
    method as string,
    url as string,
    headers as map of string to string,
    body as string,
    options as Options) {
    return textOf(sendPolicy($method, $url, $headers, encoding.encode("utf-8", $body), $options));
}

/**
 * `send` with a **raw `bytes` body** and a `BytesResponse`: the body goes onto
 * the wire byte-for-byte (a `multipart/form-data` file upload, a protobuf) and
 * the response body comes back undecoded (a `.tar.gz`, an image). Redirects,
 * retries and cookies as `send`.
 * @param method {string} the HTTP method
 * @param url {string} the absolute request URL
 * @param headers {map of string to string} request headers ({} for none; set your own Content-Type)
 * @param body {bytes} the raw request body (empty for none)
 * @param options {Options} the request policy
 * @return {BytesResponse} the final response, its body raw bytes
 * @throws {Error} as `send`
 * @example
 * def o as http.Options;
 * $o.maxBytes = math.MAX_INT;   # a large download: no body cap
 * def none as bytes;
 * def r as http.BytesResponse init http.sendBytes("GET", $url, {}, $none, $o);
 */
export func sendBytes(
    method as string,
    url as string,
    headers as map of string to string,
    body as bytes,
    options as Options) {
    return sendPolicy($method, $url, $headers, $body, $options);
}

# --- framed read for keep-alive (private) --------------------------
# The one-shot path reads to EOF (the server sends Connection: close). A
# persistent connection never EOFs between responses, so a reused socket must be
# read one response at a time, framed by Content-Length or chunked encoding.

# readSock does one buffered read, re-arming the idle deadline first. Empty means
# EOF.
func readSock(conn as net.Conn, timeoutMs as int) {
    armIdle($conn, $timeoutMs);
    return net.readBytes($conn, 4096);
}

# armIdle sets the connection's idle timeout to timeoutMs (0: none), so every
# read, a whole-body net.readN / net.readAll included, fails on a stall of
# timeoutMs while a slow but steady transfer runs on.
func armIdle(conn as net.Conn, timeoutMs as int) {
    if ($timeoutMs > 0) {
        net.setIdleTimeout($conn, $timeoutMs);
        return;
    }
    net.setIdleTimeout($conn, 0);
}

# capGuard throws when a body's size passes a positive cap (limit <= 0 is
# unlimited). A body of exactly `limit` bytes is within it.
func capGuard(size as int, limit as int) {
    if ($limit > 0 and $size > $limit) {
        throw Error{kind: "limit", message: "http: response body exceeds {$limit} bytes"};
    }
}

# chunkGuard checks a chunked body after a read: its declared data against the
# body cap, and its framing (`wire` bytes read past the head, less the data
# declared so far) against max(limit, MAX_HEADER_BYTES). An unlimited body gets
# no framing bound either: its data is unbounded already, and a long stream of
# small chunks has framing to match. Data declared but not yet received only
# lowers the framing figure, so the check is never stricter than the framing.
func chunkGuard(scan as ChunkScan, wire as int, limit as int) {
    if ($limit <= 0) {
        return;
    }
    capGuard($scan.data, $limit);
    def allowance as int init MAX_HEADER_BYTES;
    if ($limit > $allowance) {
        $allowance = $limit;
    }
    if ($wire - $scan.data > $allowance) {
        throw Error{kind: "limit", message: "http: chunked framing exceeds {$allowance} bytes"};
    }
}

# headGuard throws when a response head (status line and headers) passes
# MAX_HEADER_BYTES.
func headGuard(size as int) {
    if ($size > MAX_HEADER_BYTES) {
        throw Error{
            kind: "limit",
            message: "http: response headers exceed {MAX_HEADER_BYTES} bytes"
        };
    }
}

# LF ends a chunk-size or trailer line (its CR is trimmed with the line).
def const LF as bytes init encoding.encode("utf-8", "\n");

# MAX_CHUNK_LINE caps a chunk-size or trailer line, so a server cannot grow
# one without end.
def const MAX_CHUNK_LINE as int init 8192;

# MAX_INTERIM caps the 1xx responses skipped before the final one.
def const MAX_INTERIM as int init 32;

# ChunkScan follows a chunked body as it arrives, so the reader knows when the
# terminal chunk and its trailers are complete without re-reading what came
# before. `mode` is 0 while reading a size line, 1 inside chunk data (plus its
# CRLF), 2 in the trailers and 3 when done. `data` sums the chunk sizes declared
# so far: the decoded body length the cap is checked against, known as soon as
# a size line arrives, before its data is read.
def struct ChunkScan {
    mode as int,
    remaining as int,
    line as bytes,
    data as int
};

# scanChunks advances a ChunkScan over `b` from offset `from`. Data is skipped
# by arithmetic, and only size and trailer lines are looked at, so a read costs
# its own length. The final `dechunk` checks the framing again in full.
func scanChunks(st as ChunkScan, b as bytes, from as int) {
    def s as ChunkScan init $st;
    def i as int init $from;
    def n as int init len($b);
    while ($i < $n and $s.mode != 3) {
        if ($s.mode == 1) {
            def take as int init $n - $i;
            if ($take > $s.remaining) {
                $take = $s.remaining;
            }
            $s.remaining = $s.remaining - $take;
            $i = $i + $take;
            if ($s.remaining == 0) {
                $s.mode = 0;
            }
        } else {
            def lf as int init binary.indexOf($b, LF, $i);
            if ($lf < 0) {
                $s.line = binary.concat($s.line, $b[$i..$n]);
                if (len($s.line) > MAX_CHUNK_LINE) {
                    throw Error{kind: "limit", message: "chunk size or trailer line too long"};
                }
                $i = $n;
            } else {
                def lineBytes as bytes init binary.concat($s.line, $b[$i..$lf]);
                def text as string init bytesToStr($lineBytes, 0, len($lineBytes));
                $s.line = $b[0..0];
                $i = $lf + 1;
                if (strings.endsWith($text, "\r")) {
                    $text = $text[..len($text) - 1];
                }
                if ($s.mode == 0) {
                    def size as int init chunkSize($text);
                    if ($size == 0) {
                        $s.mode = 2;
                    } else {
                        $s.mode = 1;
                        $s.remaining = $size + 2;
                        $s.data = $s.data + $size;
                    }
                } elseif ($text == "") {
                    $s.mode = 3;
                }
            }
        }
    }
    return $s;
}

# statusOf reads the status code from a response head ("HTTP/1.1 304 Not
# Modified\r\n..."), throwing kind "http" for a malformed status line, so a
# garbled head is refused before its headers decide how the body is read.
func statusOf(head as string) {
    return parseStatusLine(strings.split($head, "\r\n")[0]).status;
}

# hasNoBody reports whether a response carries no body whatever its headers say
# (RFC 9110 6.4.1 / 9.3.2): the answer to a HEAD request, and every 1xx / 204 /
# 304. Its Content-Length (or Transfer-Encoding) describes the body a GET would
# have returned, so reading that many bytes would block on bytes that never come.
func hasNoBody(method as string, status as int) {
    return strings.upper($method) == "HEAD" or ($status >= 100 and $status < 200) or
        $status == 204 or $status == 304;
}

# readOneRaw reads exactly one response off a (possibly reused) connection and
# returns its raw bytes (status line + headers + body as framed), suitable for
# parseRaw. Frames by Content-Length or chunked encoding; a response with
# neither is read to EOF (and the caller must not reuse the socket). `method` is
# the request's: the answer to HEAD - like a 204 / 304 - has no body, so only its
# head is read. An interim 1xx response (100 Continue, 103 Early Hints) is
# skipped and the final response read in its place; 101 Switching Protocols is
# final, returned as-is.
func readOneRaw(conn as net.Conn, timeoutMs as int, maxBytes as int, method as string) {
    def limit as int init $maxBytes;
    if ($limit == 0) {
        $limit = MAX_BODY_BYTES;
    }
    def buf as bytes;
    def hend as int init -1;
    def status as int init 0;
    def interim as int init 0;
    while (true) {
        $hend = headerEnd($buf);
        if ($hend >= 0) {
            headGuard($hend + 4);
            $status = statusOf(bytesToStr($buf, 0, $hend));
            if ($status >= 100 and $status < 200 and $status != 101) {
                # Interim response: drop its head (keeping any bytes of the
                # final response already read) and look for the next head.
                $interim = $interim + 1;
                if ($interim > MAX_INTERIM) {
                    throw Error{kind: "limit", message: "too many interim (1xx) responses"};
                }
                $buf = sliceBytes($buf, $hend + 4, len($buf));
                continue;
            }
            break;
        }
        # Any byte read so far, an interim response's too, is part of the reply.
        def replied as bool init len($buf) > 0 or $interim > 0;
        def chunk as bytes;
        try {
            $chunk = readSock($conn, $timeoutMs);
        } catch (e) {
            if ($e.kind == "net") {
                throw withFacts($e, true, $replied);
            }
            throw $e;
        }
        if (len($chunk) == 0) {
            if (not $replied) {
                # Closed with no answer at all: a transport failure, told apart
                # from a reply cut short (kind "closed") by the NetError's facts.
                throw net.NetError{
                    kind: "net",
                    message: "connection closed before response headers",
                    op: "read",
                    reason: "closed",
                    sent: true,
                    replied: false
                };
            }
            throw Error{kind: "closed", message: "connection closed before response headers"};
        }
        $buf = binary.concat($buf, $chunk);
        # No head end yet: everything read is head (or the start of one), so a
        # buffer past the cap holds a head past it. A head that does end within
        # the cap is checked above, once its end is known.
        if (headerEnd($buf) < 0) {
            headGuard(len($buf));
        }
    }
    try {
        return readBody($conn, $buf, $hend, $status, $timeoutMs, $limit, $method);
    } catch (e) {
        if ($e.kind == "net") {
            throw withFacts($e, true, true);
        }
        throw $e;
    }
}

# withFacts returns a NetError with the exchange facts set: whether the whole
# request was written (`sent`) and whether any byte of the reply arrived
# (`replied`). Message, op and reason stay as net set them.
func withFacts(e as net.NetError, sent as bool, replied as bool) {
    def out as net.NetError init $e;
    $out.sent = $sent;
    $out.replied = $replied;
    return $out;
}

# readBody reads the body of the response whose head ends at `hend` in `buf`
# and returns the raw response (head and framed body), as readOneRaw does.
func readBody(
    conn as net.Conn,
    buf as bytes,
    hend as int,
    status as int,
    timeoutMs as int,
    limit as int,
    method as string) {
    def bodyStart as int init $hend + 4;
    if (hasNoBody($method, $status)) {
        return sliceBytes($buf, 0, $bodyStart);
    }
    def headers as map of string to string init parseHeaders(strings.split(
        bytesToStr($buf, 0, $hend),
        "\r\n"));
    if (isChunked($headers)) {
        return readChunked($conn, $buf, $bodyStart, $limit, $timeoutMs);
    }
    if (maps.has($headers, "content-length")) {
        def length as int init contentLength($headers);
        capGuard($length, $limit);
        def need as int init $bodyStart + $length;
        # The remaining body length is known: one net.readN reads it in a single
        # Go loop, so a large body is one read + one concat instead of a
        # per-chunk re-copy of the whole accumulating buffer (O(N^2)).
        if (len($buf) < $need) {
            # The idle timeout re-arms the read deadline per internal read (as
            # the readSock loop does). Returned as it is, with no store or cut: the concat is exactly the
            # response, and a body is copied once here instead of three times.
            def remaining as int init $need - len($buf);
            armIdle($conn, $timeoutMs);
            if ($remaining <= READ_PIECE_BYTES) {
                return binary.concat($buf, net.readN($conn, $remaining));
            }
            # net.readN refuses more than 256 MiB a call, so a larger body (an
            # uncapped download) is read in pieces and joined once.
            def pieces as list of bytes init [$buf];
            while ($remaining > 0) {
                def n as int init lowerOf($remaining, READ_PIECE_BYTES);
                $pieces[] = net.readN($conn, $n);
                $remaining = $remaining - $n;
            }
            return binary.join($pieces);
        }
        if (len($buf) == $need) {
            return $buf;
        }
        return sliceBytes($buf, 0, $need);
    }
    # No Content-Length, no chunked: the body runs to the server's close.
    return readToClose($conn, $buf, $bodyStart, $limit, $timeoutMs);
}

# readChunked reads the rest of a chunked response whose head (and whatever
# followed it in the same reads) is `buf`, the body starting at `bodyStart`.
func readChunked(conn as net.Conn, buf as bytes, bodyStart as int, limit as int, timeoutMs as int) {
    # Collect the wire chunks in a list and join once (binary.join, O(n)): a
    # per-read re-copy of the accumulating body is O(N^2) over a large chunked
    # body. A ChunkScan follows the framing read by read, so the end is known
    # exactly, trailers included. The cap counts the decoded data (the declared
    # chunk sizes), checked as each size line arrives; the framing around it is
    # bounded separately (chunkGuard), so the wire total stays within a constant
    # factor of the cap.
    def pieces as list of bytes init [$buf];
    def wire as int init len($buf) - $bodyStart;
    def scan as ChunkScan init ChunkScan{mode: 0, remaining: 0, line: $buf[0..0], data: 0};
    $scan = scanChunks($scan, $buf, $bodyStart);
    chunkGuard($scan, $wire, $limit);
    while ($scan.mode != 3) {
        def chunk as bytes init readSock($conn, $timeoutMs);
        if (len($chunk) == 0) {
            throw Error{kind: "closed", message: "connection closed mid chunked body"};
        }
        $pieces[] = $chunk;
        $wire = $wire + len($chunk);
        $scan = scanChunks($scan, $chunk, 0);
        chunkGuard($scan, $wire, $limit);
    }
    return binary.join($pieces);
}

# readToClose reads the rest of a response framed by the server closing the
# connection, whose head (and any body bytes read with it) is `buf`.
func readToClose(conn as net.Conn, buf as bytes, bodyStart as int, limit as int, timeoutMs as int) {
    # The body is framed by the server closing the connection (RFC 9112 6.3
    # rule 7), so the caller retires the socket after. Read the remainder in one
    # native net.readAll rather than an interpreted per-4KB accumulate loop, so a
    # large unframed body (a big getBytes download) runs at native speed. The cap
    # is what is left of the body budget after any body bytes over-read while
    # reading the headers; net.readAll enforces it. For net.readAll a negative cap
    # is unlimited and 0 its own 256 MiB default, so an unlimited $limit passes
    # -1, and a budget already used up exactly passes 1 (one more byte is then one
    # too many, caught below).
    def rest as int init -1;
    def readCap as int init -1;
    if ($limit > 0) {
        capGuard(len($buf) - $bodyStart, $limit);
        $rest = $limit - (len($buf) - $bodyStart);
        $readCap = $rest;
        if ($readCap == 0) {
            $readCap = 1;
        }
    }
    armIdle($conn, $timeoutMs);
    try {
        def tail as bytes init net.readAll($conn, $readCap);
        if ($rest >= 0 and len($tail) > $rest) {
            throw Error{kind: "limit", message: "http: response body exceeds {$limit} bytes"};
        }
        return binary.concat($buf, $tail);
    } catch (e) {
        # net.readAll's cap (kind "limit") is reported as this body's cap, like
        # the other framing paths; anything else (timeout, I/O) is re-raised
        # unchanged.
        if ($e.kind == "limit") {
            throw Error{kind: "limit", message: "http: response body exceeds {$limit} bytes"};
        }
        throw $e;
    }
}

# responseClosesConn reports whether a response ends the connection (RFC 9112
# 9.3): a `close` among its Connection options (a list such as `keep-alive,
# close` too), an HTTP/1.0 response without `keep-alive`, or the
# framing-to-EOF case (no Content-Length, no chunked), which only terminates
# by the server closing the socket.
func responseClosesConn(headers as map of string to string, http10 as bool) {
    def options as list of string init [];
    if (maps.has($headers, "connection")) {
        for (def o in strings.split($headers["connection"], ",")) {
            $options[] = strings.lower(strings.trim($o));
        }
    }
    if (lists.contains($options, "close")) {
        return true;
    }
    if ($http10 and not lists.contains($options, "keep-alive")) {
        return true;
    }
    return not isChunked($headers) and not maps.has($headers, "content-length");
}

# IDEMPOTENT_METHODS are the methods RFC 9110 9.2.2 defines as idempotent:
# sending one twice has the effect of sending it once.
def const IDEMPOTENT_METHODS as list of string init [
    "GET",
    "HEAD",
    "OPTIONS",
    "TRACE",
    "PUT",
    "DELETE"
];

# isReplayable reports whether a request may be sent again after a reused
# connection failed under it, when the server may already have acted on it
# (RFC 9112 9.3.1: only an idempotent request, or one the caller marked with an
# Idempotency-Key header).
func isReplayable(method as string, headers as map of string to string) {
    return lists.contains(IDEMPOTENT_METHODS, strings.upper($method)) or
        hasHeaderCI($headers, "idempotency-key");
}

# isStaleClose reports whether a read failed because the peer closed or reset
# the connection before any byte of its reply: what a keep-alive socket the
# server timed out looks like. A timeout, or a reply cut short, is not one.
func isStaleClose(ne as net.NetError) {
    return not $ne.replied and ($ne.reason == "closed" or $ne.reason == "reset");
}

# closeQuietly closes a socket that may already be closed.
func closeQuietly(conn as net.Conn) {
    try {
        net.close($conn);
    } catch (e) {
        if ($e.kind != "closed") {
            throw $e;
        }
    }
}

# --- persistent session (exported) ---------------------------------

/**
 * A persistent HTTP connection to one origin (scheme + host + port), holding the
 * reused socket, a cookie jar, and the request options. Value-semantic, but the
 * `conn` handle is shared across copies (a `net.Conn`), so the live socket
 * survives being threaded through `exchange`. Build with `connect`; retire with
 * `close`.
 * @field conn {net.Conn} the underlying socket (shared across copies)
 * @field scheme {string} "http" or "https"
 * @field host {string} the origin host
 * @field port {int} the origin port
 * @field open {bool} whether `conn` is currently usable (false after the server closed it)
 * @field jar {map of string to string} the accumulated cookies (name -> value)
 * @field options {Options} the request options (timeout, cap, tls)
 */
export def struct Session {
    conn as net.Conn,
    scheme as string,
    host as string,
    port as int,
    open as bool,
    jar as map of string to string,
    options as Options
};

/**
 * The result of an `exchange`: the response, and the updated session to thread
 * into the next `exchange` (it carries the reused socket and any new cookies).
 * @field response {Response} the response
 * @field session {Session} the session to use for the next request
 */
export def struct Exchange {
    response as Response,
    session as Session
};

/**
 * Open a persistent connection to the origin of `url` (its scheme / host / port;
 * the path is ignored - `exchange` supplies per-request paths). The socket is
 * reused across `exchange` calls to the same origin, so a request loop pays one
 * handshake instead of N.
 * @param url {string} a URL whose origin to connect to
 * @param options {Options} the request options (timeout, cap, tls)
 * @return {Session} an open session
 * @throws {Error} kind "http" for a malformed URL, or a `net.NetError` (kind "net") or "timeout" if the connection cannot be opened
 */
export func connect(url as string, options as Options) {
    def u as Url init parseUrl($url);
    def lim as Limits init limitsOf($options);
    def conn as net.Conn init dial($u, $lim.tls, $lim.timeoutMs);
    def emptyJar as map of string to string init {};
    return Session{
        conn: $conn,
        scheme: $u.scheme,
        host: $u.host,
        port: $u.port,
        open: true,
        jar: $emptyJar,
        options: $options
    };
}

/**
 * Send one request over a session's reused connection and return the response
 * plus the updated session (reassign it: `def x as http.Exchange init
 * http.exchange($s, ...); $s = $x.session;`). `path` is a request target on the
 * session's origin (e.g. "/items?page=2"). Cookies from the response are folded
 * into the session jar and replayed on later requests. If the connection was
 * closed (by a prior `Connection: close` or a to-EOF response, or by an
 * `exchange` that failed partway), it is transparently reopened. When a reused
 * socket turns out stale (the server closed or reset it before any byte of the
 * answer), the request is sent once more on a new connection if that is safe:
 * an idempotent method (GET, HEAD, OPTIONS, TRACE, PUT, DELETE) or an
 * `Idempotency-Key` header; otherwise the `net.NetError` is thrown. Redirects are **not** followed here (that can cross origins and break
 * the socket) - use `send` for redirect-following; `exchange` returns the 3xx.
 * @param session {Session} the session (from `connect` or a prior `exchange`)
 * @param method {string} the HTTP method
 * @param path {string} the request path on the session's origin
 * @param headers {map of string to string} request headers ({} for none)
 * @param body {string} the request body ("" for none)
 * @return {Exchange} the response and the session to thread onward
 * @throws {Error} kind "http" if the response or the request is malformed, "limit" past a cap, "timeout", "closed" for a reply cut short, or a `net.NetError` (kind "net") when the network fails (see the module docs, "Errors")
 */
export func exchange(
    session as Session,
    method as string,
    path as string,
    headers as map of string to string,
    body as string) {
    def s as Session init $session;
    def u as Url init Url{scheme: $s.scheme, host: $s.host, port: $s.port, path: $path};
    # reused: the socket was not dialed by this call, so the server may have
    # closed it while it sat idle (a keep-alive timeout).
    def reused as bool init $s.open;
    def lim as Limits init limitsOf($s.options);
    if (not $s.open) {
        $s.conn = dial($u, $lim.tls, $lim.timeoutMs);
        $s.open = true;
    }
    def timeoutMs as int init $lim.timeoutMs;
    def reqHeaders as map of string to string init withCookies($headers, $s.jar);
    def blen as int init 0;
    if (len($body) > 0) {
        $blen = len(encoding.encode("utf-8", $body));
    }
    def wire as bytes init encoding.encode(
        "utf-8",
        buildHeadConn($method, $u, $reqHeaders, $blen, true) + $body);
    def replayable as bool init isReplayable($method, $headers);
    # writeIdle arms the write deadline per chunk; readSock re-arms the read
    # deadline before each read of the response. Read and write deadlines are
    # independent, so a prior read's deadline can never expire this write.
    try {
        writeIdle($s.conn, $wire, $timeoutMs);
    } catch (e) {
        # "closed": a socket an earlier failed exchange closed (see below), so
        # nothing was sent; "net" on a reused socket: the server dropped it while
        # idle, and the request is resent only when replaying it is safe.
        if ($e.kind != "closed" and not ($e.kind == "net" and $reused and $replayable)) {
            throw $e;
        }
        closeQuietly($s.conn);
        $s.conn = dial($u, $lim.tls, $lim.timeoutMs);
        $reused = false;
        writeIdle($s.conn, $wire, $timeoutMs);
    }
    def raw as bytes;
    try {
        $raw = readOneRaw($s.conn, $timeoutMs, $lim.limit, $method);
    } catch (e) {
        # A timeout or bad framing leaves the rest of this response on the
        # socket, where the next exchange would read it as its own answer.
        # Close it; the next exchange on this session dials a new one.
        closeQuietly($s.conn);
        def stale as bool init false;
        if ($reused and $replayable and $e.kind == "net") {
            def ne as net.NetError init $e;
            $stale = isStaleClose($ne);
        }
        if (not $stale) {
            throw $e;
        }
        # The reused socket was closed or reset before a single byte of the
        # reply: the server dropped an idle keep-alive connection, racing this
        # request. Resend once on a new connection (RFC 9112 9.3.1).
        $s.conn = dial($u, $lim.tls, $lim.timeoutMs);
        try {
            writeIdle($s.conn, $wire, $timeoutMs);
            $raw = readOneRaw($s.conn, $timeoutMs, $lim.limit, $method);
        } catch (e2) {
            closeQuietly($s.conn);
            throw $e2;
        }
    }
    def r as BytesResponse init parseRaw($raw);
    for (def sc in setCookiesFromRaw($raw)) {
        $s.jar = jarAdd($s.jar, $sc);
    }
    def http10 as bool init strings.startsWith(
        bytesToStr($raw, 0, lowerOf(len($raw), 8)),
        "HTTP/1.0");
    if (responseClosesConn($r.headers, $http10)) {
        net.close($s.conn);
        $s.open = false;
    }
    return Exchange{
        response: Response{
            status: $r.status,
            statusText: $r.statusText,
            headers: $r.headers,
            body: bodyText($r.body)
        },
        session: $s
    };
}

/**
 * Close a session's connection. Idempotent-safe to call once; do not use the
 * session afterwards.
 * @param session {Session} the session to close
 */
export func close(session as Session) {
    if ($session.open) {
        try {
            net.close($session.conn);
        } catch (e) {
            # Already closed by a failed exchange.
            if ($e.kind != "closed") {
                throw $e;
            }
        }
    }
    return;
}
