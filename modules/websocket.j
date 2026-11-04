# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.25.0
# pragma-jennifer-capability: net

/**
 * An RFC 6455 WebSocket client over `net`. `connect` performs the HTTP
 * `Upgrade` handshake (verifying the server's `Sec-WebSocket-Accept`, which is
 * SHA-1 + base64 of the client key), then `send` / `sendBytes` write masked
 * text / binary frames and `receive` reads the next message, transparently
 * answering pings with pongs and reassembling fragmented messages. `close`
 * sends a close frame and shuts the socket. `ws://` is plain TCP, `wss://` is
 * TLS.
 *
 * `connectOptions` takes an `Options`: extra handshake headers (an API key or
 * a token), the subprotocols to offer (`graphql-transport-ws`), and TLS trust
 * for a self-signed or private-CA server. The subprotocol the server chose is
 * `Conn.protocol`.
 *
 * Client-to-server frames are masked (required by the spec); server frames are
 * not. Needs the default `jennifer` binary (`net`); a protocol error or a
 * dropped connection throws `Error{kind: "websocket"}`. A server-side upgrade
 * is out of scope (it needs an `httpd` connection-hijack hook).
 * @module websocket
 * @example
 * import "websocket.j" as websocket;
 * def ws as websocket.Conn init websocket.connect("ws://echo.example.com/");
 * websocket.send($ws, "hello");
 * def m as websocket.Message init websocket.receive($ws);   # m.kind == "text", m.text == "hello"
 * websocket.close($ws);
 */
use net;
use binary;
use strings;
use convert;
use hash;
use encoding;
use crypto;
use regex;
use lists;

# The RFC 6455 handshake GUID, concatenated with the client key before SHA-1.
def const WS_MAGIC as string init "258EAFA5-E914-47DA-95CA-C5AB0DC85B11";
def const HANDSHAKE_TIMEOUT_MS as int init 10000;
# Default per-receive read timeout in milliseconds.
def const DEFAULT_TIMEOUT_MS as int init 30000;

# MAX_MESSAGE_BYTES caps a received message. A malicious peer can declare a
# 64-bit frame length (opcode 127) or stream unbounded continuation fragments
# (fin = 0); without a cap either drives the interpreter to OOM. A frame or a
# reassembled message larger than this is refused (a 1009 close where possible).
def const MAX_MESSAGE_BYTES as int init 67108864;

# Frame opcodes.
def const OP_CONT as int init 0;
def const OP_TEXT as int init 1;
def const OP_BINARY as int init 2;
def const OP_CLOSE as int init 8;
def const OP_PING as int init 9;
def const OP_PONG as int init 10;

/**
 * An open WebSocket connection.
 * @field socket {net.Conn} the underlying (plain or TLS) connection
 * @field timeoutMs {int} the per-receive read timeout in milliseconds
 * @field protocol {string} the subprotocol the server chose from those offered, "" for none
 */
export def struct Conn {
    socket as net.Conn,
    timeoutMs as int,
    protocol as string
};

/**
 * How `connectOptions` opens a connection. The zero value (from
 * `websocket.defaultOptions()`) is what `connect` does: the 30 s receive
 * timeout, no extra headers, no subprotocol, and TLS verified against the
 * system roots.
 * @field timeoutMs {int} the per-receive read timeout in milliseconds (0 = 30 s)
 * @field headers {map of string to string} extra headers for the handshake request, such as an API key
 * @field protocols {list of string} the subprotocols to offer, in order of preference (`Sec-WebSocket-Protocol`)
 * @field caCert {bytes} a PEM certificate to trust besides the system roots, for a private CA or a self-signed `wss://` server
 * @field skipVerify {bool} accept any certificate; for a trusted LAN endpoint only, it exposes the connection to a man-in-the-middle
 */
export def struct Options {
    timeoutMs as int,
    headers as map of string to string,
    protocols as list of string,
    caCert as bytes,
    skipVerify as bool
};

# TOKEN is an RFC 9110 token: a header name, or a subprotocol name.
def const TOKEN as string init "^[!#$%&'*+.^_`|~0-9A-Za-z-]+$";

# OWNED are the handshake headers the client writes itself. A caller's copy
# would duplicate them, and Sec-WebSocket-Extensions would ask for a framing
# (compression) this module cannot decode.
def const OWNED as list of string init [
    "host",
    "upgrade",
    "connection",
    "sec-websocket-key",
    "sec-websocket-version",
    "sec-websocket-protocol",
    "sec-websocket-extensions",
    "content-length",
    "transfer-encoding"
];

/**
 * A received message.
 * @field kind {string} "text", "binary", "close", or "pong"
 * @field text {string} the decoded text (for a "text" message; "" otherwise)
 * @field data {bytes} the raw payload bytes
 */
export def struct Message {
    kind as string,
    text as string,
    data as bytes
};

# A parsed ws:// / wss:// target (private).
def struct Target {
    secure as bool,
    host as string,
    port as int,
    path as string
};

# A decoded frame (private).
def struct Frame {
    opcode as int,
    fin as int,
    payload as bytes
};

func fail(msg as string) {
    throw Error{kind: "websocket", message: "websocket: " + $msg};
}

# --- byte helpers (private) -------------------------------------------------

# readN reads exactly n bytes, looping over the "up to n" net.readBytes. The
# common case - a single read that satisfies the whole request (a frame header,
# or a body the peer wrote in one go) - is returned as-is, skipping any copy.
# Only a short read falls back to collecting the chunks and joining them once
# (binary.join, a native Go copy); appending byte by byte would feed every byte
# of a large frame through the interpreter, and `$out = binary.concat($out, $chunk)`
# would recopy the whole growing buffer on every short read (O(n^2)).
func readN(socket as net.Conn, n as int) {
    if ($n <= 0) {
        def e as bytes;
        return $e;
    }
    def chunk as bytes init net.readBytes($socket, $n);
    if (len($chunk) == 0) {
        fail("connection closed mid-frame");
    }
    if (len($chunk) == $n) {
        return $chunk;
    }
    def parts as list of bytes init [$chunk];
    def total as int init len($chunk);
    while ($total < $n) {
        def more as bytes init net.readBytes($socket, $n - $total);
        if (len($more) == 0) {
            fail("connection closed mid-frame");
        }
        $parts[] = $more;
        $total = $total + len($more);
    }
    return binary.join($parts);
}

# randomBytes returns n crypto-grade random bytes. RFC 6455 (section 5.3)
# requires the frame masking key be "derived from a strong source of entropy"
# and hard to predict frame to frame (it is what stops a malicious script from
# steering the on-the-wire bytes to poison an intermediary cache); the
# handshake nonce (Sec-WebSocket-Key) is a random value too. `crypto` satisfies
# both - `math`'s seedable RNG would not.
func randomBytes(n as int) {
    return crypto.randBytes($n);
}

# --- handshake (private) ----------------------------------------------------

# parseUrl splits a ws:// or wss:// URL into a Target.
func parseUrl(url as string) {
    # The host and path are written verbatim into the GET / Host lines of the
    # upgrade request, so a CR / LF / NUL in the URL would smuggle arbitrary HTTP
    # headers (request splitting) into the handshake.
    if (strings.contains($url, "\r") or strings.contains($url, "\n") or strings.contains(
        $url,
        "\0")) {
        fail("URL contains a control character (CR/LF/NUL)");
    }
    def secure as bool init false;
    def rest as string init $url;
    if (strings.startsWith($url, "wss://")) {
        $secure = true;
        $rest = $url[6..];
    } elseif (strings.startsWith($url, "ws://")) {
        $rest = $url[5..];
    } else {
        fail("URL must start with ws:// or wss://");
    }
    # A fragment never goes on the wire (RFC 6455 forbids one).
    def hash as int init strings.indexOf($rest, "#");
    if ($hash >= 0) {
        $rest = $rest[..$hash];
    }
    # The authority ends at the first "/" or "?": "ws://h?x" has the target
    # "/?x", not the host "h?x".
    def path as string init "/";
    def hostport as string init $rest;
    def cut as int init strings.indexOf($rest, "/");
    def q as int init strings.indexOf($rest, "?");
    if ($q >= 0 and ($cut < 0 or $q < $cut)) {
        $cut = $q;
    }
    if ($cut >= 0) {
        $hostport = $rest[..$cut];
        $path = $rest[$cut..];
        if (strings.startsWith($path, "?")) {
            $path = "/" + $path;
        }
    }
    if (strings.contains($hostport, "@")) {
        fail("URL must not hold credentials; send them as a header (Options.headers)");
    }
    def host as string init $hostport;
    def portText as string init "";
    if (strings.startsWith($hostport, "[")) {
        # A bracketed IPv6 literal keeps its brackets, as the Host header and
        # the dial address both want them.
        def rb as int init strings.indexOf($hostport, "]");
        if ($rb < 0) {
            fail("URL host has an unclosed \"[\"");
        }
        $host = $hostport[..$rb + 1];
        def after as string init $hostport[$rb + 1..];
        if (strings.startsWith($after, ":")) {
            $portText = $after[1..];
        } elseif ($after != "") {
            fail("URL host is malformed");
        }
    } else {
        def colon as int init strings.indexOf($hostport, ":");
        if ($colon >= 0) {
            $host = $hostport[..$colon];
            $portText = $hostport[$colon + 1..];
        }
    }
    if ($host == "" or $host == "[]" or regex.matches('[\x00-\x20\x7f]', $host)) {
        fail("URL has no host, or a malformed one");
    }
    if (regex.matches('[\x00-\x20\x7f]', $path)) {
        fail("URL path contains whitespace or a control character (percent-encode it)");
    }
    def port as int init 80;
    if ($secure) {
        $port = 443;
    }
    if ($portText != "") {
        if (not regex.matches('^[0-9]{1,5}$', $portText) or convert.toInt($portText) < 1 or
            convert.toInt($portText) > 65535) {
            fail("URL port is not a number from 1 to 65535");
        }
        $port = convert.toInt($portText);
    }
    return Target{secure: $secure, host: $host, port: $port, path: $path};
}

# makeKey returns a fresh base64 Sec-WebSocket-Key (16 random bytes).
func makeKey() {
    return encoding.toText(randomBytes(16), "base64");
}

# acceptFor computes the expected Sec-WebSocket-Accept for a client key:
# base64(SHA1(key + magic)). hash.compute returns the raw 20 digest bytes.
func acceptFor(key as string) {
    def concat as bytes init convert.bytesFromString($key + WS_MAGIC, "utf-8");
    def raw as bytes init hash.compute($concat, "sha1");
    return encoding.toText($raw, "base64");
}

# readHandshakeResponse reads bytes until the CRLFCRLF header terminator.
# It reads a byte at a time (the terminator must not be over-read into the first
# frame, and readN blocks for exactly its count), but accumulates into a byte
# buffer and decodes once - a growing `string +` per byte would be O(N^2).
func readHandshakeResponse(socket as net.Conn) {
    def buf as bytes;
    def done as bool init false;
    repeat {
        def one as bytes init readN($socket, 1);
        $buf[] = $one[0];
        def m as int init len($buf);
        # `\r\n\r\n` is 13,10,13,10.
        if ($m >= 4 and $buf[$m - 1] == 10 and $buf[$m - 2] == 13 and $buf[$m - 3] == 10 and
            $buf[$m - 4] == 13) {
            $done = true;
        }
        if ($m > 8192) {
            fail("handshake response too large");
        }
    } until ($done);
    return convert.stringFromBytes($buf, "utf-8");
}

# statusCodeOf extracts the numeric status code (the token after the first
# space) from an HTTP status line, so a code check can't be fooled by "101"
# appearing elsewhere on the line.
func statusCodeOf(statusLine as string) {
    def sp as int init strings.indexOf($statusLine, " ");
    if ($sp < 0) {
        return "";
    }
    def rest as string init $statusLine[$sp + 1..];
    def spTwo as int init strings.indexOf($rest, " ");
    if ($spTwo < 0) {
        return strings.trim($rest);
    }
    return $rest[..$spTwo];
}

# headerValue finds a header's value (case-insensitive name) in the response
# lines, preserving the value's original case.
func headerValue(lines as list of string, name as string) {
    for (def i in 1..len($lines)) {
        def line as string init $lines[$i];
        def colon as int init strings.indexOf($line, ":");
        if ($colon >= 0) {
            def key as string init strings.lower(strings.trim($line[..$colon]));
            if ($key == $name) {
                return strings.trim($line[$colon + 1..]);
            }
        }
    }
    return "";
}

# checkOptions refuses extra headers and subprotocols that would break the
# handshake: a name that is not a token, a value with CR / LF / NUL (request
# splitting), or a header the client writes itself (OWNED).
func checkOptions(o as Options) {
    for (def name in $o.headers) {
        if (not regex.matches(TOKEN, $name)) {
            fail("header name is not a valid token");
        }
        if (regex.matches('[\x00-\x08\x0a-\x1f\x7f]', $o.headers[$name])) {
            fail("header " + $name + " value contains a control character");
        }
        if (lists.contains(OWNED, strings.lower($name))) {
            fail("header " + $name + " is set by the client itself");
        }
    }
    for (def p in $o.protocols) {
        if (not regex.matches(TOKEN, $p)) {
            fail("subprotocol name is not a valid token");
        }
    }
}

# hasToken reports whether a comma-separated header value holds token
# (case-insensitive), as "Connection: keep-alive, Upgrade" holds "upgrade".
func hasToken(value as string, token as string) {
    for (def part in strings.split($value, ",")) {
        if (strings.lower(strings.trim($part)) == $token) {
            return true;
        }
    }
    return false;
}

# handshake sends the Upgrade request, checks the response as RFC 6455
# section 4.1 asks (101, Upgrade, Connection, the accept key, no subprotocol
# or extension that was not offered), and returns the chosen subprotocol.
func handshake(socket as net.Conn, t as Target, key as string, o as Options) {
    def hostHeader as string init $t.host;
    if ((not $t.secure and not ($t.port == 80)) or ($t.secure and not ($t.port == 443))) {
        $hostHeader = "{$t.host}:{$t.port}";
    }
    def req as string init "GET " + $t.path + " HTTP/1.1\r\n" +
        "Host: " + $hostHeader + "\r\n" +
        "Upgrade: websocket\r\n" +
        "Connection: Upgrade\r\n" +
        "Sec-WebSocket-Key: " + $key + "\r\n" +
        "Sec-WebSocket-Version: 13\r\n";
    if (len($o.protocols) > 0) {
        $req = $req + "Sec-WebSocket-Protocol: " + strings.join($o.protocols, ", ") + "\r\n";
    }
    for (def name in $o.headers) {
        $req = $req + $name + ": " + $o.headers[$name] + "\r\n";
    }
    net.writeBytes($socket, convert.bytesFromString($req + "\r\n", "utf-8"));
    def resp as string init readHandshakeResponse($socket);
    def lines as list of string init strings.split($resp, "\r\n");
    if (not (statusCodeOf($lines[0]) == "101")) {
        fail("handshake rejected: " + regex.replace('[\x00-\x1f\x7f]', $lines[0], " "));
    }
    if (strings.lower(headerValue($lines, "upgrade")) != "websocket" or
        not hasToken(headerValue($lines, "connection"), "upgrade")) {
        fail("handshake response is not a WebSocket upgrade");
    }
    if (not (headerValue($lines, "sec-websocket-accept") == acceptFor($key))) {
        fail("invalid Sec-WebSocket-Accept");
    }
    if (headerValue($lines, "sec-websocket-extensions") != "") {
        fail("server enabled an extension that was not offered");
    }
    def chosen as string init headerValue($lines, "sec-websocket-protocol");
    if ($chosen != "" and not lists.contains($o.protocols, $chosen)) {
        fail("server chose a subprotocol that was not offered");
    }
    return $chosen;
}

# --- framing (private) ------------------------------------------------------

# encodeFrameMasked builds one client frame (FIN set, masked) with an explicit
# mask - the deterministic core `encodeFrame` wraps with a random mask.
func encodeFrameMasked(opcode as int, payload as bytes, mask as bytes) {
    def frame as bytes;
    $frame[] = 0x80 | $opcode;
    def n as int init len($payload);
    if ($n < 126) {
        $frame[] = 0x80 | $n;
    } elseif ($n < 65536) {
        $frame[] = 0x80 | 126;
        $frame[] = ($n >> 8) & 0xff;
        $frame[] = $n & 0xff;
    } else {
        $frame[] = 0x80 | 127;
        def s as int init 56;
        while ($s >= 0) {
            $frame[] = ($n >> $s) & 0xff;
            $s = $s - 8;
        }
    }
    $frame = binary.concat($frame, $mask);
    def j as int init 0;
    while ($j < $n) {
        # `& 3` is `% 4` (the mask is exactly 4 bytes) without the per-byte
        # modulo; the masked payload has to be built byte by byte regardless.
        $frame[] = $payload[$j] ^ $mask[$j & 3];
        $j = $j + 1;
    }
    return $frame;
}

# encodeFrame builds one client frame with a fresh random mask.
func encodeFrame(opcode as int, payload as bytes) {
    return encodeFrameMasked($opcode, $payload, randomBytes(4));
}

# readFrame reads and decodes one server frame off the socket; a server frame
# is never masked.
func readFrame(socket as net.Conn) {
    def h as bytes init readN($socket, 2);
    # RSV1-3 belong to an extension (RSV1 is compression), and none was
    # negotiated: a frame with one set cannot be read correctly.
    if (($h[0] & 0x70) != 0) {
        fail("frame has reserved bits set (an extension that was not negotiated)");
    }
    def opcode as int init $h[0] & 0x0f;
    def fin as int init ($h[0] >> 7) & 1;
    def masked as int init ($h[1] >> 7) & 1;
    def n as int init $h[1] & 0x7f;
    # RFC 6455 section 5: a client fails the connection on a masked server
    # frame, an opcode it does not know, and a control frame that is
    # fragmented or longer than 125 bytes.
    if ($masked == 1) {
        fail("server frame is masked");
    }
    if (($opcode > OP_BINARY and $opcode < OP_CLOSE) or $opcode > OP_PONG) {
        fail("frame has an unknown opcode {$opcode}");
    }
    if ($opcode >= OP_CLOSE and ($fin == 0 or $n > 125)) {
        fail("control frame is fragmented or longer than 125 bytes");
    }
    if ($n == 126) {
        def e as bytes init readN($socket, 2);
        $n = ($e[0] << 8) | $e[1];
    } elseif ($n == 127) {
        def e as bytes init readN($socket, 8);
        # Anything with a high byte set is far past the cap, and shifting it
        # in would overflow an int.
        if ($e[0] != 0 or $e[1] != 0 or $e[2] != 0 or $e[3] != 0) {
            fail("frame declares more than the {MAX_MESSAGE_BYTES}-byte limit");
        }
        $n = 0;
        for (def i in 0..8) {
            $n = ($n << 8) | $e[$i];
        }
    }
    if ($n > MAX_MESSAGE_BYTES) {
        throw Error{
            kind: "websocket",
            message: "websocket: frame declares {$n} bytes, over the {MAX_MESSAGE_BYTES}-byte limit"
        };
    }
    def payload as bytes;
    if ($n > 0) {
        $payload = readN($socket, $n);
    }
    return Frame{opcode: $opcode, fin: $fin, payload: $payload};
}

# --- API (exported) ---------------------------------------------------------

/**
 * Open a WebSocket connection to a ws:// or wss:// URL (default receive
 * timeout).
 * @param url {string} the ws:// or wss:// URL
 * @return {Conn} the open connection
 * @throws {Error} kind "websocket" on a failed handshake
 */
export func connect(url as string) {
    return connectWith($url, DEFAULT_TIMEOUT_MS);
}

/**
 * Open a WebSocket connection with an explicit per-receive read timeout.
 * @param url {string} the ws:// or wss:// URL
 * @param timeoutMs {int} the per-receive read timeout in milliseconds
 * @return {Conn} the open connection
 * @throws {Error} kind "websocket" on a failed handshake
 */
export func connectWith(url as string, timeoutMs as int) {
    def o as Options init defaultOptions();
    $o.timeoutMs = $timeoutMs;
    return connectOptions($url, $o);
}

/**
 * The zero `Options`: what `connect` does. Set fields on the result.
 * @return {Options} the default options
 */
export func defaultOptions() {
    def o as Options;
    return $o;
}

/**
 * Open a WebSocket connection with `Options`: extra handshake headers, the
 * subprotocols to offer, TLS trust and the receive timeout. The subprotocol
 * the server picked is `protocol` on the returned `Conn` ("" when it picked
 * none; whether that is acceptable is the caller's call).
 * @param url {string} the ws:// or wss:// URL
 * @param options {Options} how to connect
 * @return {Conn} the open connection
 * @throws {Error} kind "websocket" on a bad URL or option, or a failed handshake
 */
export func connectOptions(url as string, options as Options) {
    def t as Target init parseUrl($url);
    checkOptions($options);
    def timeoutMs as int init $options.timeoutMs;
    if ($timeoutMs == 0) {
        $timeoutMs = DEFAULT_TIMEOUT_MS;
    }
    def addr as string init "{$t.host}:{$t.port}";
    def socket as net.Conn;
    if ($t.secure) {
        def tls as net.TLSOptions;
        $tls.skipVerify = $options.skipVerify;
        $tls.caCert = $options.caCert;
        $socket = net.connectTLS($addr, $tls, HANDSHAKE_TIMEOUT_MS);
    } else {
        $socket = net.connect($addr, HANDSHAKE_TIMEOUT_MS);
    }
    # A failed HTTP upgrade must not leak the socket; on success the caller
    # owns the open connection.
    errdefer net.close($socket);
    # The handshake both writes the upgrade request and reads the response, so
    # bound each direction.
    net.setWriteDeadline($socket, HANDSHAKE_TIMEOUT_MS);
    net.setReadDeadline($socket, HANDSHAKE_TIMEOUT_MS);
    def chosen as string init handshake($socket, $t, makeKey(), $options);
    net.setWriteDeadline($socket, 0);
    net.setReadDeadline($socket, 0);
    return Conn{socket: $socket, timeoutMs: $timeoutMs, protocol: $chosen};
}

# sendFrame writes one masked frame, bounded by the connection timeout.
func sendFrame(c as Conn, opcode as int, payload as bytes) {
    net.setWriteDeadline($c.socket, $c.timeoutMs);
    net.writeBytes($c.socket, encodeFrame($opcode, $payload));
    net.setWriteDeadline($c.socket, 0);
}

/**
 * Send a text message.
 * @param c {Conn} the connection
 * @param text {string} the message text
 */
export func send(c as Conn, text as string) {
    sendFrame($c, OP_TEXT, convert.bytesFromString($text, "utf-8"));
}

/**
 * Send a binary message.
 * @param c {Conn} the connection
 * @param data {bytes} the message payload
 */
export func sendBytes(c as Conn, data as bytes) {
    sendFrame($c, OP_BINARY, $data);
}

/**
 * Send a ping (empty payload).
 * @param c {Conn} the connection
 */
export func ping(c as Conn) {
    def empty as bytes;
    sendFrame($c, OP_PING, $empty);
}

/**
 * Receive the next message. Pings are answered with pongs and skipped;
 * fragmented text / binary messages are reassembled. A close frame is returned
 * as kind "close".
 * @param c {Conn} the connection
 * @return {Message} the next message
 * @throws {Error} kind "websocket" on a protocol error or dropped connection
 */
export func receive(c as Conn) {
    net.setReadDeadline($c.socket, $c.timeoutMs);
    # Data fragments collect in a list and join once (binary.join, a native Go
    # copy) when the message completes; accLen tracks the reassembled size.
    def fragments as list of bytes init [];
    def accLen as int init 0;
    def dataOpcode as int init OP_TEXT;
    # `started` marks that a data message is being reassembled, so a control
    # frame (ping / pong) interleaved between fragments does not abandon the
    # partial message: RFC 6455 allows control frames between data fragments.
    def started as bool init false;
    def done as bool init false;
    def control as Message;
    def isControl as bool init false;
    repeat {
        def f as Frame init readFrame($c.socket);
        if ($f.opcode == OP_PING) {
            sendFrame($c, OP_PONG, $f.payload);
        } elseif ($f.opcode == OP_PONG) {
            # A pong interleaved in a fragmented message is ignored so
            # reassembly continues; only a standalone pong surfaces as a
            # message.
            if (not $started) {
                $control = Message{kind: "pong", text: "", data: $f.payload};
                $isControl = true;
                $done = true;
            }
        } elseif ($f.opcode == OP_CLOSE) {
            $control = Message{kind: "close", text: "", data: $f.payload};
            $isControl = true;
            $done = true;
        } else {
            # A continuation must follow a started message, and a new message
            # must not start inside one.
            if ($f.opcode == OP_CONT and not $started) {
                fail("continuation frame without a message to continue");
            }
            if ($f.opcode != OP_CONT and $started) {
                fail("new message started inside a fragmented one");
            }
            $started = true;
            if (not ($f.opcode == OP_CONT)) {
                $dataOpcode = $f.opcode;
            }
            $fragments[] = $f.payload;
            $accLen = $accLen + len($f.payload);
            if ($accLen > MAX_MESSAGE_BYTES) {
                # Refuse the oversized message: send a 1009 (message too big)
                # close and abort, rather than growing memory unbounded.
                def code as bytes;
                $code[] = 3;
                $code[] = 241;
                sendFrame($c, OP_CLOSE, $code);
                throw Error{
                    kind: "websocket",
                    message: "websocket: reassembled message exceeds the " +
                        "{MAX_MESSAGE_BYTES}-byte limit"
                };
            }
            if ($f.fin == 1) {
                $done = true;
            }
        }
    } until ($done);
    net.setReadDeadline($c.socket, 0);
    if ($isControl) {
        return $control;
    }
    def acc as bytes init binary.join($fragments);
    if ($dataOpcode == OP_TEXT) {
        def text as string;
        try {
            $text = convert.stringFromBytes($acc, "utf-8");
        } catch (e) {
            fail("text message is not valid UTF-8");
        }
        return Message{kind: "text", text: $text, data: $acc};
    }
    return Message{kind: "binary", text: "", data: $acc};
}

/**
 * Send a close frame and shut the connection.
 * @param c {Conn} the connection
 */
export func close(c as Conn) {
    # The socket is shut even when the close-frame write throws (a dead peer
    # must not leak the fd).
    defer net.close($c.socket);
    def empty as bytes;
    sendFrame($c, OP_CLOSE, $empty);
}
