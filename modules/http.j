# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

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
 * decoded as UTF-8 (a binary body raises an error; use `requestBytes` /
 * `getBytes` for that). Chunked and Content-Length framing are both handled.
 * @module http
 * @example
 * def r as http.Response init http.get("http://example.com/", {});
 * io.printf("status %d\n%s\n", $r.status, $r.body);
 * def sent as http.Response init http.post("https://api.example.com/items",
 *     "application/json", "{\"name\":\"ada\"}", {"Authorization": "Bearer xyz"});
 */
use net;
use binary;
use strings;
use convert;
use maps;
use encoding;
use time;

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
 * `requestBytes` / `requestWithBytes` / `getBytes`. `headers` keys are
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
 * TLS options for an `https://` request, mirroring `net.TLSOptions`. The zero
 * value (`skipVerify` false, empty `caCert`) full-verifies the server
 * certificate against the URL host, which is what a plain `request` / verb
 * shortcut uses - so an `https://` call with no options behaves exactly as
 * before.
 * @field skipVerify {bool} accept *any* certificate (self-signed, wrong host, expired). Opt-in; disables authentication and exposes the connection to a man-in-the-middle. Use only for a trusted LAN endpoint you cannot give a proper CA.
 * @field caCert {bytes} a PEM certificate to trust in addition to the system roots, for a private CA or a pinned self-signed cert. The safer alternative to `skipVerify`: the server is still authenticated, just against this cert.
 */
export def struct TlsOptions {
    skipVerify as bool,
    caCert as bytes
};

# --- byte helpers (private) ----------------------------------------

# sliceBytes copies buf[start:end] into a new bytes value.
func sliceBytes(buf as bytes, start as int, end as int) {
    return binary.slice($buf, $start, $end);
}

# bytesToStr decodes buf[start:end] as UTF-8 text.
func bytesToStr(buf as bytes, start as int, end as int) {
    return convert.stringFromBytes(sliceBytes($buf, $start, $end), "utf-8");
}

# findCRLF returns the index of the next CRLF at or after `start`, or -1.
func findCRLF(buf as bytes, start as int) {
    def i as int init $start;
    def n as int init len($buf) - 1;
    while ($i < $n) {
        if ($buf[$i] == 13 and $buf[$i + 1] == 10) {
            return $i;
        }
        $i = $i + 1;
    }
    return -1;
}

# headerEnd returns the index of the CRLFCRLF separating headers from body, -1
# if not present.
func headerEnd(buf as bytes) {
    def n as int init len($buf) - 3;
    def i as int init 0;
    while ($i < $n) {
        if ($buf[$i] == 13 and $buf[$i + 1] == 10) {
            if ($buf[$i + 2] == 13 and $buf[$i + 3] == 10) {
                return $i;
            }
        }
        $i = $i + 1;
    }
    return -1;
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

# dechunk decodes a chunked transfer-encoded body (all bytes already read).
func dechunk(body as bytes) {
    def out as bytes;
    def pos as int init 0;
    def n as int init len($body);
    while ($pos < $n) {
        def lineEnd as int init findCRLF($body, $pos);
        if ($lineEnd < 0) {
            # A chunk-size line with no CRLF means the connection dropped
            # mid-stream: fail loudly rather than return a silent partial body.
            throw Error{
                kind: "http",
                message: "truncated body: chunk size line has no CRLF",
                file: "",
                line: 0,
                col: 0
            };
        }
        def size as int init hexToInt(strings.trim(bytesToStr($body, $pos, $lineEnd)));
        if ($size == 0) {
            return $out; # terminal 0-length chunk: complete
        }
        def dataStart as int init $lineEnd + 2;
        def dataEnd as int init $dataStart + $size;
        if ($dataEnd > $n) {
            throw Error{
                kind: "http",
                message: "truncated body: chunk declares more bytes than received",
                file: "",
                line: 0,
                col: 0
            };
        }
        def j as int init $dataStart;
        while ($j < $dataEnd) {
            $out[] = $body[$j];
            $j = $j + 1;
        }
        $pos = $dataEnd + 2;
    }
    # Ran out of input without ever seeing the terminal 0-length chunk.
    throw Error{
        kind: "http",
        message: "truncated body: missing terminal chunk",
        file: "",
        line: 0,
        col: 0
    };
}

# --- request / response (private) ----------------------------------

# parseUrl splits a URL into scheme / host / port / path (with query). Defaults:
# scheme "http", port 80 (443 for https), path "/".
func parseUrl(url as string) {
    def scheme as string init "http";
    def rest as string init $url;
    def si as int init strings.indexOf($url, "://");
    if ($si >= 0) {
        $scheme = strings.lower(strings.substring($url, 0, $si));
        $rest = strings.substring($url, $si + 3, len($url));
    }
    # A fragment (#...) is never sent to the server; strip it before anything
    # else so it cannot leak onto the wire.
    def hash as int init strings.indexOf($rest, "#");
    if ($hash >= 0) {
        $rest = strings.substring($rest, 0, $hash);
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
        $authority = strings.substring($rest, 0, $cut);
        def target as string init strings.substring($rest, $cut, len($rest));
        if (strings.startsWith($target, "?")) {
            $path = "/" + $target;
        } else {
            $path = $target;
        }
    }
    # Strip userinfo (user[:pass]@) at the LAST '@' - a password may contain '@'.
    def at as int init lastAt($authority);
    if ($at >= 0) {
        $authority = strings.substring($authority, $at + 1, len($authority));
    }
    def host as string init $authority;
    def port as int init 80;
    if ($scheme == "https") {
        $port = 443;
    }
    if (strings.startsWith($authority, "[")) {
        # Bracketed IPv6 literal: the colon inside the brackets is not the
        # port separator. Keep the brackets on the host so the Host header
        # and net.connect address stay well-formed (`[::1]:8080`).
        def rb as int init strings.indexOf($authority, "]");
        if ($rb >= 0) {
            $host = strings.substring($authority, 0, $rb + 1);
            def afterBr as string init strings.substring($authority, $rb + 1, len($authority));
            if (strings.startsWith($afterBr, ":")) {
                $port = convert.toInt(strings.substring($afterBr, 1, len($afterBr)));
            }
        }
    } else {
        def colon as int init strings.indexOf($authority, ":");
        if ($colon >= 0) {
            $host = strings.substring($authority, 0, $colon);
            $port = convert.toInt(strings.substring($authority, $colon + 1, len($authority)));
        }
    }
    return Url{scheme: $scheme, host: $host, port: $port, path: $path};
}

# lastAt returns the index of the last '@' in s, or -1.
func lastAt(s as string) {
    def cs as list of string init strings.chars($s);
    def found as int init -1;
    def i as int init 0;
    while ($i < len($cs)) {
        if ($cs[$i] == "@") {
            $found = $i;
        }
        $i = $i + 1;
    }
    return $found;
}

# hostHeader renders the Host header value (host, plus port when non-default).
func hostHeader(u as Url) {
    if ($u.scheme == "http" and $u.port == 80) {
        return $u.host;
    }
    if ($u.scheme == "https" and $u.port == 443) {
        return $u.host;
    }
    return $u.host + ":" + convert.toString($u.port);
}

# hasControlChar reports whether s contains CR, LF, or NUL - the bytes that let
# a caller-supplied value break out of its field and inject request lines.
func hasControlChar(s as string) {
    return strings.contains($s, "\r") or strings.contains($s, "\n") or strings.contains($s, "\0");
}

# rejectInjection throws if any part that goes onto the request line or a header
# carries CR / LF / NUL, which would otherwise smuggle extra headers or a whole
# second request (HTTP request splitting / header injection).
func rejectInjection(u as Url, headers as map of string to string) {
    if (hasControlChar($u.host) or hasControlChar($u.path)) {
        throw Error{
            kind: "http",
            message: "request target contains a control character (CR/LF/NUL)",
            file: "",
            line: 0,
            col: 0
        };
    }
    for (def k in $headers) {
        if (hasControlChar($k)) {
            throw Error{
                kind: "http",
                message: "header name contains a control character (CR/LF/NUL)",
                file: "",
                line: 0,
                col: 0
            };
        }
        if (hasControlChar($headers[$k])) {
            throw Error{
                kind: "http",
                message: "header value contains a control character (CR/LF/NUL): " + $k,
                file: "",
                line: 0,
                col: 0
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
            message: "request method contains a control character (CR/LF/NUL)",
            file: "",
            line: 0,
            col: 0
        };
    }
    def out as string init $method + " " + $u.path + " HTTP/1.1\r\n";
    $out = $out + "Host: " + hostHeader($u) + "\r\n";
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
        # reject and that enables request smuggling. Compared case-insensitively.
        def lk as string init strings.lower($k);
        if ($lk == "host" or $lk == "connection" or $lk == "content-length") {
            continue;
        }
        $out = $out + $k + ": " + $headers[$k] + "\r\n";
    }
    if ($contentLen > 0) {
        $out = $out + "Content-Length: " + convert.toString($contentLen) + "\r\n";
    }
    return $out + "\r\n";
}

func buildRequest(method as string, u as Url, headers as map of string to string, body as string) {
    def blen as int init 0;
    if (len($body) > 0) {
        $blen = len(convert.bytesFromString($body, "utf-8"));
    }
    return buildHead($method, $u, $headers, $blen) + $body;
}

# parseHeaders parses the header lines (after the status line) into a lowercased
# map.
func parseHeaders(lines as list of string) {
    def headers as map of string to string init {};
    def i as int init 1;
    while ($i < len($lines)) {
        def line as string init $lines[$i];
        def colon as int init strings.indexOf($line, ":");
        if ($colon > 0) {
            def raw as string init strings.substring($line, 0, $colon);
            def name as string init strings.lower(strings.trim($raw));
            def value as string init strings.trim(strings.substring($line, $colon + 1, len($line)));
            # Repeated headers (e.g. two Set-Cookie lines) would otherwise
            # last-wins-collapse, silently dropping all but one. Join them with
            # ", " so no value is lost (RFC 7230 comma-folding; a caller that
            # needs individual Set-Cookie values must re-split).
            if (maps.has($headers, $name)) {
                $headers[$name] = $headers[$name] + ", " + $value;
            } else {
                $headers[$name] = $value;
            }
        }
        $i = $i + 1;
    }
    return $headers;
}

# parseRaw parses a complete raw response (headers + body bytes) into a
# BytesResponse, de-chunking or trimming the body to its framed length. The body
# stays raw bytes - this is the byte-exact core both the text and binary paths
# build on; `parseResponse` decodes it to a string, the byte verbs keep it.
func parseRaw(raw as bytes) {
    def hend as int init headerEnd($raw);
    if ($hend < 0) {
        throw Error{
            kind: "http",
            message: "malformed response (no header terminator)",
            file: "",
            line: 0,
            col: 0
        };
    }
    def headerText as string init bytesToStr($raw, 0, $hend);
    def lines as list of string init strings.split($headerText, "\r\n");
    def statusLine as string init $lines[0];
    def protoEnd as int init strings.indexOf($statusLine, " ");
    def afterProto as string init strings.substring($statusLine, $protoEnd + 1, len($statusLine));
    # A status line may carry no reason phrase ("HTTP/1.1 200\r\n"), which every
    # client tolerates: when there is no space after the code, the whole
    # remainder is the code and the reason phrase is empty.
    def codeEnd as int init strings.indexOf($afterProto, " ");
    def codeStr as string init $afterProto;
    def statusText as string init "";
    if ($codeEnd >= 0) {
        $codeStr = strings.substring($afterProto, 0, $codeEnd);
        $statusText = strings.substring($afterProto, $codeEnd + 1, len($afterProto));
    }
    def status as int init convert.toInt($codeStr);
    def headers as map of string to string init parseHeaders($lines);
    def bodyBytes as bytes init sliceBytes($raw, $hend + 4, len($raw));
    if (isChunked($headers)) {
        $bodyBytes = dechunk($bodyBytes);
    } elseif (maps.has($headers, "content-length")) {
        def cl as int init convert.toInt($headers["content-length"]);
        if (len($bodyBytes) > $cl) {
            $bodyBytes = sliceBytes($bodyBytes, 0, $cl);
        }
    }
    return BytesResponse{
        status: $status,
        statusText: $statusText,
        headers: $headers,
        body: $bodyBytes
    };
}

# parseResponse parses a raw response into a text Response, decoding the body as
# UTF-8. Throws (strict, no replacement) on a non-UTF-8 body - use the byte verbs
# (`requestBytes` / `getBytes`) for binary payloads.
func parseResponse(raw as bytes) {
    def r as BytesResponse init parseRaw($raw);
    return Response{
        status: $r.status,
        statusText: $r.statusText,
        headers: $r.headers,
        body: convert.stringFromBytes($r.body, "utf-8")
    };
}

# isChunked reports whether the response uses chunked transfer-encoding.
func isChunked(headers as map of string to string) {
    if (not maps.has($headers, "transfer-encoding")) {
        return false;
    }
    return strings.contains(strings.lower($headers["transfer-encoding"]), "chunked");
}

# --- net (private) -------------------------------------------------

# The default per-read idle timeout: a read that stalls this long (a hung or
# unreachable server) fails with a "read timed out" error instead of blocking
# forever. Pass a different value via `requestWith`; 0 disables the timeout.
def const DEFAULT_TIMEOUT_MS as int init 30000;

# MAX_BODY_BYTES is the default response-body cap. The per-read deadline bounds
# a *stalled* peer, but a fast peer can stream unbounded data; without a size
# limit a hostile server (e.g. behind an untrusted feed URL) could drive the
# interpreter to OOM. 64 MiB is far past any API / feed / page response; a larger
# body is a catchable error, not a crash. A caller that genuinely needs a bigger
# (or unlimited) body passes an explicit `maxBytes` to `requestWith`.
def const MAX_BODY_BYTES as int init 67108864;

# readToEOF reads the whole connection (the server closes after the response
# because we send Connection: close). `timeoutMs` re-arms a read deadline before
# each read, so a stalled connection breaks with an error; 0 clears it.
# `maxBytes` caps the body: 0 uses the default (MAX_BODY_BYTES), a negative value
# is unlimited, a positive value is that exact ceiling.
#
# The whole body is read in one Go call (net.readAll), not a per-byte interpreted
# accumulation loop - so a large body (an object-storage download) runs at
# native speed instead of paying the tree-walker's per-byte cost.
func readToEOF(conn as net.Conn, timeoutMs as int, maxBytes as int) {
    def limit as int init $maxBytes;
    if ($limit == 0) {
        $limit = MAX_BODY_BYTES;
    }
    # net.readAll: maxBytes > 0 caps, <= 0 is unlimited; a negative $limit
    # (unlimited) maps to 0. idleTimeoutMs re-arms the per-read deadline.
    def capBytes as int init $limit;
    if ($capBytes < 0) {
        $capBytes = 0;
    }
    try {
        return net.readAll($conn, $capBytes, $timeoutMs);
    } catch (e) {
        # Re-tag the cap-exceeded error as kind "http" so callers catch it the
        # same way as before; re-raise anything else (timeout, I/O) unchanged.
        if (strings.contains($e.message, "exceeds the")) {
            throw Error{
                kind: "http",
                message: "http: response body exceeds " + convert.toString($limit) + " bytes",
                file: "",
                line: 0,
                col: 0
            };
        }
        throw $e;
    }
}

func dial(u as Url, tls as TlsOptions) {
    def addr as string init $u.host + ":" + convert.toString($u.port);
    if ($u.scheme == "https") {
        # A zero TlsOptions maps to a zero net.TLSOptions (verify on, no extra
        # CA), identical to the old no-options dial; skipVerify / caCert opt out.
        def opts as net.TLSOptions;
        $opts.skipVerify = $tls.skipVerify;
        $opts.caCert = $tls.caCert;
        return net.connectTLS($addr, $opts, DEFAULT_TIMEOUT_MS);
    }
    return net.connect($addr, DEFAULT_TIMEOUT_MS);
}

# --- API (exported) ------------------------------------------------

/**
 * Send one HTTP request with an explicit idle timeout and body cap, returning
 * the response. `timeoutMs` bounds each read (a stalled server fails rather than
 * hanging); 0 disables it. `maxBytes` caps the response body: 0 uses the 64 MiB
 * default (`MAX_BODY_BYTES`, which protects against an untrusted server
 * streaming to OOM), a negative value lifts the cap (for a trusted large
 * download), and a positive value sets an exact ceiling. `request` and the verb
 * shortcuts use `DEFAULT_TIMEOUT_MS` and the default cap.
 * @param method {string} the HTTP method (e.g. "GET", "POST")
 * @param url {string} the absolute request URL
 * @param headers {map of string to string} request headers ({} for none)
 * @param body {string} the request body ("" for no body)
 * @param timeoutMs {int} the per-read idle timeout in milliseconds (0 = none)
 * @param maxBytes {int} the response-body cap (0 = 64 MiB default, negative = unlimited, positive = exact)
 * @return {Response} the parsed response
 * @throws {Error} kind "http" if the response is malformed or exceeds `maxBytes`, or a "read timed out" error on timeout
 */
export func requestWith(
    method as string,
    url as string,
    headers as map of string to string,
    body as string,
    timeoutMs as int,
    maxBytes as int) {
    def t as TlsOptions; # zero value: full certificate verification (unchanged)
    return parseResponse(sendCore($method, $url, $headers, $body, $timeoutMs, $maxBytes, $t));
}

# sendCore is the single wire implementation - dial, send, read - returning the
# **raw response bytes** (head + framed body). The public variants differ only in
# which TlsOptions they hand it and whether they parse the result as a text
# `Response` or a byte-safe `BytesResponse`. An `http://` URL ignores `tls`.
func sendCore(
    method as string,
    url as string,
    headers as map of string to string,
    body as string,
    timeoutMs as int,
    maxBytes as int,
    tls as TlsOptions) {
    def u as Url init parseUrl($url);
    # Build (and validate) the request before opening a socket, so an injected
    # header / path throws without dialing and nothing malformed hits the wire.
    def wire as string init buildRequest($method, $u, $headers, $body);
    def conn as net.Conn init dial($u, $tls);
    # Close the socket exactly once whether the exchange succeeds or throws: a
    # read timeout or a parse error must not leak the connection (a poller
    # hitting timeouts would otherwise exhaust file descriptors).
    defer net.close($conn);
    if ($timeoutMs > 0) {
        net.setDeadline($conn, $timeoutMs); # covers the write and the first read
    }
    net.writeBytes($conn, convert.bytesFromString($wire, "utf-8"));
    return readToEOF($conn, $timeoutMs, $maxBytes);
}

# sendCoreRaw is sendCore for a **bytes** request body: it writes the ASCII head
# and the raw body as two separate socket writes, so an arbitrary-binary body (a
# multipart file upload, a protobuf) reaches the wire byte-for-byte instead of
# being mangled by a UTF-8 string round-trip. Returns the raw response bytes.
func sendCoreRaw(
    method as string,
    url as string,
    headers as map of string to string,
    body as bytes,
    timeoutMs as int,
    maxBytes as int,
    tls as TlsOptions) {
    def u as Url init parseUrl($url);
    def head as string init buildHead($method, $u, $headers, len($body));
    def conn as net.Conn init dial($u, $tls);
    defer net.close($conn);
    if ($timeoutMs > 0) {
        net.setDeadline($conn, $timeoutMs);
    }
    net.writeBytes($conn, convert.bytesFromString($head, "utf-8"));
    if (len($body) > 0) {
        net.writeBytes($conn, $body);
    }
    return readToEOF($conn, $timeoutMs, $maxBytes);
}

/**
 * Send one request with a **raw `bytes` body** and return the text `Response`.
 * Unlike `requestWith` (whose string body is UTF-8-encoded onto the wire), the
 * body here is written byte-for-byte, so a binary payload - a `multipart/form-data`
 * file upload, a protobuf - is transmitted intact. The response is still parsed
 * as text (use `requestWithBytes` if the *response* may be non-UTF-8).
 * @param method {string} the HTTP method (e.g. "POST")
 * @param url {string} the absolute request URL
 * @param headers {map of string to string} request headers (set your own Content-Type)
 * @param body {bytes} the raw request body
 * @param timeoutMs {int} the per-read idle timeout in milliseconds (0 = none)
 * @param maxBytes {int} the response-body cap (0 = 64 MiB default, negative = unlimited)
 * @return {Response} the parsed response
 * @throws {Error} kind "http" on a malformed response or a cap breach; "read timed out" on timeout
 */
export func requestRawBody(
    method as string,
    url as string,
    headers as map of string to string,
    body as bytes,
    timeoutMs as int,
    maxBytes as int) {
    def t as TlsOptions; # zero value: full certificate verification
    return parseResponse(sendCoreRaw($method, $url, $headers, $body, $timeoutMs, $maxBytes, $t));
}

/**
 * `requestRawBody` with explicit TLS options for an `https://` server (a
 * self-signed or private-CA host). For `http://` the options are ignored.
 * @param method {string} the HTTP method
 * @param url {string} the absolute request URL
 * @param headers {map of string to string} request headers
 * @param body {bytes} the raw request body
 * @param timeoutMs {int} the per-read idle timeout in milliseconds (0 = none)
 * @param maxBytes {int} the response-body cap (0 = default, negative = unlimited)
 * @param tls {TlsOptions} certificate-verification options for the TLS handshake
 * @return {Response} the parsed response
 * @throws {Error} kind "http" on a malformed response or a cap breach; "read timed out" on timeout
 */
export func requestRawBodyTls(
    method as string,
    url as string,
    headers as map of string to string,
    body as bytes,
    timeoutMs as int,
    maxBytes as int,
    tls as TlsOptions) {
    return parseResponse(sendCoreRaw($method, $url, $headers, $body, $timeoutMs, $maxBytes, $tls));
}

/**
 * Like `requestWith`, but with explicit TLS options for an `https://` URL (a
 * self-signed or private-CA server). For `http://` the options are ignored.
 * @param method {string} the HTTP method (e.g. "GET", "POST")
 * @param url {string} the absolute request URL
 * @param headers {map of string to string} request headers ({} for none)
 * @param body {string} the request body ("" for no body)
 * @param timeoutMs {int} the per-read idle timeout in milliseconds (0 = none)
 * @param maxBytes {int} the response-body cap (0 = 64 MiB default, negative = unlimited, positive = exact)
 * @param tls {TlsOptions} certificate-verification options for the TLS handshake
 * @return {Response} the parsed response
 * @throws {Error} kind "http" if the response is malformed or exceeds `maxBytes`, or a "read timed out" error on timeout
 */
export func requestWithTls(
    method as string,
    url as string,
    headers as map of string to string,
    body as string,
    timeoutMs as int,
    maxBytes as int,
    tls as TlsOptions) {
    return parseResponse(sendCore($method, $url, $headers, $body, $timeoutMs, $maxBytes, $tls));
}

/**
 * Send one request with explicit TLS options (default idle timeout and body
 * cap). The `https`-with-a-self-signed-cert shortcut over `requestWithTls`.
 * @param method {string} the HTTP method (e.g. "GET", "POST")
 * @param url {string} the absolute request URL
 * @param headers {map of string to string} request headers ({} for none)
 * @param body {string} the request body ("" for no body)
 * @param tls {TlsOptions} certificate-verification options for the TLS handshake
 * @return {Response} the parsed response
 * @throws {Error} kind "http" if the response is malformed, or a "read timed out" error on timeout
 */
export func requestTls(
    method as string,
    url as string,
    headers as map of string to string,
    body as string,
    tls as TlsOptions) {
    return requestWithTls($method, $url, $headers, $body, DEFAULT_TIMEOUT_MS, 0, $tls);
}

/**
 * Send one HTTP request and return the response (with the default idle timeout).
 * @param method {string} the HTTP method (e.g. "GET", "POST")
 * @param url {string} the absolute request URL
 * @param headers {map of string to string} request headers ({} for none)
 * @param body {string} the request body ("" for no body)
 * @return {Response} the parsed response
 * @throws {Error} kind "http" if the response is malformed, or a "read timed out" error on timeout
 */
export func request(
    method as string,
    url as string,
    headers as map of string to string,
    body as string) {
    return requestWith($method, $url, $headers, $body, DEFAULT_TIMEOUT_MS, 0);
}

/**
 * Send one request and return the body as **raw `bytes`** (a `BytesResponse`) -
 * the byte-safe path for downloading binary content (a `.tar.gz`, an image) that
 * a UTF-8 text `Response` cannot hold. Explicit per-read idle timeout and body
 * cap; pass a negative `maxBytes` for an unbounded download (a large release
 * archive), or `TlsOptions` for a self-signed / private-CA `https://` host.
 * @param method {string} the HTTP method (e.g. "GET")
 * @param url {string} the absolute request URL
 * @param headers {map of string to string} request headers ({} for none)
 * @param body {string} the request body ("" for no body)
 * @param timeoutMs {int} the per-read idle timeout in milliseconds (0 = none)
 * @param maxBytes {int} the response-body cap (0 = 64 MiB default, negative = unlimited, positive = exact)
 * @param tls {TlsOptions} certificate-verification options for the TLS handshake
 * @return {BytesResponse} the response with a raw bytes body
 * @throws {Error} kind "http" if the response is malformed or exceeds `maxBytes`, or a "read timed out" error on timeout
 */
export func requestWithBytes(
    method as string,
    url as string,
    headers as map of string to string,
    body as string,
    timeoutMs as int,
    maxBytes as int,
    tls as TlsOptions) {
    return parseRaw(sendCore($method, $url, $headers, $body, $timeoutMs, $maxBytes, $tls));
}

/**
 * Send one request and return a raw-bytes `BytesResponse` (default idle timeout,
 * default 64 MiB cap, full TLS verification). The binary counterpart to
 * `request`; for a large download or a self-signed host use `requestWithBytes`.
 * @param method {string} the HTTP method (e.g. "GET")
 * @param url {string} the absolute request URL
 * @param headers {map of string to string} request headers ({} for none)
 * @param body {string} the request body ("" for no body)
 * @return {BytesResponse} the response with a raw bytes body
 * @throws {Error} kind "http" if the response is malformed or exceeds the cap, or a "read timed out" error on timeout
 */
export func requestBytes(
    method as string,
    url as string,
    headers as map of string to string,
    body as string) {
    def t as TlsOptions; # zero value: full certificate verification
    return requestWithBytes($method, $url, $headers, $body, DEFAULT_TIMEOUT_MS, 0, $t);
}

/**
 * GET a URL and return its body as raw `bytes` (the download shortcut).
 * @param url {string} the absolute request URL
 * @param headers {map of string to string} request headers ({} for none)
 * @return {BytesResponse} the response with a raw bytes body
 */
export func getBytes(url as string, headers as map of string to string) {
    return requestBytes("GET", $url, $headers, "");
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
 * setting its field.
 * @field timeoutMs {int} per-read idle timeout (0 = 30s default, as `request`)
 * @field maxBytes {int} response-body cap (0 = 64 MiB default, negative = unlimited)
 * @field maxRedirects {int} how many 3xx redirects to follow (0 = none; the 3xx is returned)
 * @field maxRetries {int} how many times to retry a 429 / 5xx (0 = none), with exponential backoff honouring `Retry-After`
 * @field backoffMs {int} base backoff for the first retry (0 = 250ms), doubled each attempt and capped at 30s
 * @field tls {TlsOptions} TLS verification options for `https://` (zero = full verification)
 */
export def struct Options {
    timeoutMs as int,
    maxBytes as int,
    maxRedirects as int,
    maxRetries as int,
    backoffMs as int,
    tls as TlsOptions
};

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
    def creds as bytes init convert.bytesFromString($user + ":" + $pass, "utf-8");
    return "Basic " + encoding.toText($creds, "base64");
}

# effectiveTimeout maps a 0 (unset) options timeout onto the module default,
# matching what the one-shot verbs use.
func effectiveTimeout(timeoutMs as int) {
    if ($timeoutMs == 0) {
        return DEFAULT_TIMEOUT_MS;
    }
    return $timeoutMs;
}

# isAllDigits reports whether s is one or more ASCII digits (a numeric header).
func isAllDigits(s as string) {
    if (len($s) == 0) {
        return false;
    }
    for (def c in strings.chars($s)) {
        if (strings.indexOf("0123456789", $c) < 0) {
            return false;
        }
    }
    return true;
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
            def name as string init strings.lower(strings.trim(strings.substring($line, 0, $colon)));
            if ($name == "set-cookie") {
                $out[] = strings.trim(strings.substring($line, $colon + 1, len($line)));
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
        $pair = strings.substring($setCookie, 0, $semi);
    }
    def eq as int init strings.indexOf($pair, "=");
    if ($eq < 0) {
        return $jar;
    }
    def name as string init strings.trim(strings.substring($pair, 0, $eq));
    if (len($name) == 0) {
        return $jar;
    }
    def out as map of string to string init $jar;
    $out[$name] = strings.trim(strings.substring($pair, $eq + 1, len($pair)));
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
# raised to a numeric Retry-After (seconds) when the server sent a larger one,
# and clamped to MAX_BACKOFF_MS.
func retryDelayMs(resp as BytesResponse, attempt as int, baseMs as int) {
    def base as int init $baseMs;
    if ($base <= 0) {
        $base = DEFAULT_BACKOFF_MS;
    }
    def delay as int init $base;
    def k as int init 0;
    while ($k < $attempt) {
        $delay = $delay * 2;
        $k = $k + 1;
    }
    if (maps.has($resp.headers, "retry-after")) {
        def ra as string init strings.trim($resp.headers["retry-after"]);
        if (isAllDigits($ra)) {
            def raMs as int init convert.toInt($ra) * 1000;
            if ($raMs > $delay) {
                $delay = $raMs;
            }
        }
    }
    if ($delay > MAX_BACKOFF_MS) {
        $delay = MAX_BACKOFF_MS;
    }
    return $delay;
}

# originOf returns the "scheme://host[:port]" of a URL (its origin).
func originOf(url as string) {
    def u as Url init parseUrl($url);
    return $u.scheme + "://" + hostHeader($u);
}

# lastSlash returns the index of the last "/" in s, or -1.
func lastSlash(s as string) {
    def i as int init len($s) - 1;
    while ($i >= 0) {
        if (strings.substring($s, $i, $i + 1) == "/") {
            return $i;
        }
        $i = $i - 1;
    }
    return -1;
}

# resolveLocation resolves a Location value against the current URL: an absolute
# URL is used as-is, an absolute path attaches to the current origin, and a
# relative path resolves against the current path's directory.
func resolveLocation(curUrl as string, loc as string) {
    if (strings.contains($loc, "://")) {
        return $loc;
    }
    if (strings.startsWith($loc, "/")) {
        return originOf($curUrl) + $loc;
    }
    def u as Url init parseUrl($curUrl);
    def dir as string init $u.path;
    def cut as int init lastSlash($dir);
    if ($cut >= 0) {
        $dir = strings.substring($dir, 0, $cut + 1);
    } else {
        $dir = "/";
    }
    return originOf($curUrl) + $dir + $loc;
}

# sendOnce runs one request through the retry loop (no redirects): it dials,
# sends (Connection: close), and retries a 429 / 5xx up to opts.maxRetries with
# backoff. Returns the raw response bytes.
func sendOnce(
    method as string,
    url as string,
    headers as map of string to string,
    body as string,
    opts as Options) {
    def timeoutMs as int init effectiveTimeout($opts.timeoutMs);
    def attempt as int init 0;
    def raw as bytes;
    while (true) {
        $raw = sendCore($method, $url, $headers, $body, $timeoutMs, $opts.maxBytes, $opts.tls);
        def r as BytesResponse init parseRaw($raw);
        if (isRetryable($r.status) and $attempt < $opts.maxRetries) {
            time.sleep(time.fromMilliseconds(retryDelayMs($r, $attempt, $opts.backoffMs)));
            $attempt = $attempt + 1;
        } else {
            return $raw;
        }
    }
    return $raw;
}

/**
 * Send a request with a policy: follow up to `options.maxRedirects` 3xx
 * redirects, retry a 429 / 5xx up to `options.maxRetries` with exponential
 * backoff (honouring a numeric `Retry-After`), and carry cookies across the
 * redirect chain. A `303` (and a `301`/`302` on a `POST`) becomes a bodyless
 * `GET`; `307`/`308` preserve the method and body. With a zero `Options` it is a
 * one-shot request, exactly like `request`.
 * @param method {string} the HTTP method
 * @param url {string} the absolute request URL
 * @param headers {map of string to string} request headers ({} for none)
 * @param body {string} the request body ("" for none)
 * @param options {Options} the request policy
 * @return {Response} the final response (after any redirects / retries)
 * @throws {Error} kind "http" on a malformed response or a body-cap breach, "read timed out" on timeout
 */
export func send(
    method as string,
    url as string,
    headers as map of string to string,
    body as string,
    options as Options) {
    def curMethod as string init $method;
    def curUrl as string init $url;
    def curBody as string init $body;
    def redirectsLeft as int init $options.maxRedirects;
    def jar as map of string to string init {};
    while (true) {
        def raw as bytes init sendOnce(
            $curMethod,
            $curUrl,
            withCookies($headers, $jar),
            $curBody,
            $options);
        def r as BytesResponse init parseRaw($raw);
        for (def sc in setCookiesFromRaw($raw)) {
            $jar = jarAdd($jar, $sc);
        }
        if (isRedirect($r.status) and $redirectsLeft > 0 and maps.has($r.headers, "location")) {
            $curUrl = resolveLocation($curUrl, $r.headers["location"]);
            if ($r.status == 303 or
                (($r.status == 301 or $r.status == 302) and $curMethod == "POST")) {
                $curMethod = "GET";
                $curBody = "";
            }
            $redirectsLeft = $redirectsLeft - 1;
        } else {
            return Response{
                status: $r.status,
                statusText: $r.statusText,
                headers: $r.headers,
                body: convert.stringFromBytes($r.body, "utf-8")
            };
        }
    }
    return Response{status: 0, statusText: "", headers: {}, body: ""};
}

# --- framed read for keep-alive (private) --------------------------
# The one-shot path reads to EOF (the server sends Connection: close). A
# persistent connection never EOFs between responses, so a reused socket must be
# read one response at a time, framed by Content-Length or chunked encoding.

# readSock does one buffered read, re-arming the idle deadline first. Empty means
# EOF.
func readSock(conn as net.Conn, timeoutMs as int) {
    if ($timeoutMs > 0) {
        net.setDeadline($conn, $timeoutMs);
    }
    return net.readBytes($conn, 4096);
}

# capGuard throws when the accumulated size passes a positive cap (limit <= 0 is
# unlimited).
func capGuard(size as int, limit as int) {
    if ($limit > 0 and $size > $limit) {
        throw Error{
            kind: "http",
            message: "http: response body exceeds " + convert.toString($limit) + " bytes",
            file: "",
            line: 0,
            col: 0
        };
    }
}

# endsWithDoubleCRLF is the cheap gate before attempting a full dechunk: a
# complete chunked body ends with CRLFCRLF.
func endsWithDoubleCRLF(body as bytes) {
    def n as int init len($body);
    if ($n < 4) {
        return false;
    }
    return $body[$n - 4] == 13 and $body[$n - 3] == 10 and
        $body[$n - 2] == 13 and $body[$n - 1] == 10;
}

# chunkedComplete reports whether buf[bodyStart:] is a complete chunked body (its
# terminal 0-length chunk has arrived). It attempts a real dechunk only once the
# body ends in CRLFCRLF, so accumulation stays near-linear.
func chunkedComplete(buf as bytes, bodyStart as int) {
    def body as bytes init sliceBytes($buf, $bodyStart, len($buf));
    if (not endsWithDoubleCRLF($body)) {
        return false;
    }
    try {
        dechunk($body);
        return true;
    } catch (e) {
        return false;
    }
}

# readOneRaw reads exactly one response off a (possibly reused) connection and
# returns its raw bytes (status line + headers + body as framed), suitable for
# parseRaw. Frames by Content-Length or chunked encoding; a response with
# neither is read to EOF (and the caller must not reuse the socket).
func readOneRaw(conn as net.Conn, timeoutMs as int, maxBytes as int) {
    def limit as int init $maxBytes;
    if ($limit == 0) {
        $limit = MAX_BODY_BYTES;
    }
    def buf as bytes;
    def hend as int init -1;
    while (true) {
        $hend = headerEnd($buf);
        if ($hend >= 0) {
            break;
        }
        def chunk as bytes init readSock($conn, $timeoutMs);
        if (len($chunk) == 0) {
            throw Error{
                kind: "http",
                message: "connection closed before response headers",
                file: "",
                line: 0,
                col: 0
            };
        }
        $buf = binary.concat($buf, $chunk);
        capGuard(len($buf), $limit);
    }
    def headers as map of string to string init parseHeaders(strings.split(
        bytesToStr($buf, 0, $hend),
        "\r\n"));
    def bodyStart as int init $hend + 4;
    if (isChunked($headers)) {
        while (not chunkedComplete($buf, $bodyStart)) {
            def chunk as bytes init readSock($conn, $timeoutMs);
            if (len($chunk) == 0) {
                throw Error{
                    kind: "http",
                    message: "connection closed mid chunked body",
                    file: "",
                    line: 0,
                    col: 0
                };
            }
            $buf = binary.concat($buf, $chunk);
            capGuard(len($buf), $limit);
        }
        return $buf;
    }
    if (maps.has($headers, "content-length")) {
        def need as int init $bodyStart + convert.toInt($headers["content-length"]);
        capGuard($need, $limit);
        while (len($buf) < $need) {
            def chunk as bytes init readSock($conn, $timeoutMs);
            if (len($chunk) == 0) {
                throw Error{
                    kind: "http",
                    message: "connection closed before Content-Length bytes",
                    file: "",
                    line: 0,
                    col: 0
                };
            }
            $buf = binary.concat($buf, $chunk);
        }
        return sliceBytes($buf, 0, $need);
    }
    # No Content-Length, no chunked: the body runs to EOF, so the server has
    # closed the connection - read the rest and let the caller retire the socket.
    while (true) {
        def chunk as bytes init readSock($conn, $timeoutMs);
        if (len($chunk) == 0) {
            return $buf;
        }
        $buf = binary.concat($buf, $chunk);
        capGuard(len($buf), $limit);
    }
    return $buf;
}

# responseClosesConn reports whether a response ends the connection: an explicit
# `Connection: close`, or the framing-to-EOF case (no Content-Length, no
# chunked), which only terminates by the server closing the socket.
func responseClosesConn(headers as map of string to string) {
    if (maps.has($headers, "connection") and
        strings.lower($headers["connection"]) == "close") {
        return true;
    }
    return not isChunked($headers) and not maps.has($headers, "content-length");
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
 * @throws {Error} kind "http" (or a net error) if the connection cannot be opened
 */
export func connect(url as string, options as Options) {
    def u as Url init parseUrl($url);
    def conn as net.Conn init dial($u, $options.tls);
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
 * closed (by a prior `Connection: close` or a to-EOF response), it is transparently
 * reopened. Redirects are **not** followed here (that can cross origins and break
 * the socket) - use `send` for redirect-following; `exchange` returns the 3xx.
 * @param session {Session} the session (from `connect` or a prior `exchange`)
 * @param method {string} the HTTP method
 * @param path {string} the request path on the session's origin
 * @param headers {map of string to string} request headers ({} for none)
 * @param body {string} the request body ("" for none)
 * @return {Exchange} the response and the session to thread onward
 * @throws {Error} kind "http" on a malformed response or a cap breach, "read timed out" on timeout
 */
export func exchange(
    session as Session,
    method as string,
    path as string,
    headers as map of string to string,
    body as string) {
    def s as Session init $session;
    def u as Url init Url{scheme: $s.scheme, host: $s.host, port: $s.port, path: $path};
    if (not $s.open) {
        $s.conn = dial($u, $s.options.tls);
        $s.open = true;
    }
    def timeoutMs as int init effectiveTimeout($s.options.timeoutMs);
    def reqHeaders as map of string to string init withCookies($headers, $s.jar);
    def blen as int init 0;
    if (len($body) > 0) {
        $blen = len(convert.bytesFromString($body, "utf-8"));
    }
    def wire as string init buildHeadConn($method, $u, $reqHeaders, $blen, true) + $body;
    # Arm the deadline for this write: a deadline left over from the previous
    # exchange's last read would otherwise expire the write if time passed
    # between exchanges (setDeadline covers read and write). readSock re-arms it
    # before each read of the response.
    if ($timeoutMs > 0) {
        net.setDeadline($s.conn, $timeoutMs);
    }
    net.writeBytes($s.conn, convert.bytesFromString($wire, "utf-8"));
    def raw as bytes init readOneRaw($s.conn, $timeoutMs, $s.options.maxBytes);
    def r as BytesResponse init parseRaw($raw);
    for (def sc in setCookiesFromRaw($raw)) {
        $s.jar = jarAdd($s.jar, $sc);
    }
    if (responseClosesConn($r.headers)) {
        net.close($s.conn);
        $s.open = false;
    }
    return Exchange{
        response: Response{
            status: $r.status,
            statusText: $r.statusText,
            headers: $r.headers,
            body: convert.stringFromBytes($r.body, "utf-8")
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
        net.close($session.conn);
    }
    return;
}
