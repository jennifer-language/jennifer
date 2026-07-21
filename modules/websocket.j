# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

/**
 * An RFC 6455 WebSocket client over `net`. `connect` performs the HTTP
 * `Upgrade` handshake (verifying the server's `Sec-WebSocket-Accept`, which is
 * SHA-1 + base64 of the client key), then `send` / `sendBytes` write masked
 * text / binary frames and `receive` reads the next message, transparently
 * answering pings with pongs and reassembling fragmented messages. `close`
 * sends a close frame and shuts the socket. `ws://` is plain TCP, `wss://` is
 * TLS.
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
use strings;
use convert;
use hash;
use encoding;
use crypto;

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
 */
export def struct Conn {
    socket as net.Conn,
    timeoutMs as int
};

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
    throw Error{ kind: "websocket", message: "websocket: " + $msg, file: "", line: 0, col: 0 };
}

# --- byte helpers (private) -------------------------------------------------

# appendBytes copies src onto the end of dst and returns dst.
func appendBytes(dst as bytes, src as bytes) {
    def i as int init 0;
    while ($i < len($src)) {
        $dst[] = $src[$i];
        $i = $i + 1;
    }
    return $dst;
}

# readN reads exactly n bytes, looping over the "up to n" net.readBytes. The
# chunk is appended into the owning `out` in place (amortised O(1) per byte);
# `$out = appendBytes($out, $chunk)` would copy the whole growing buffer on
# every short read (gigabytes of memcpy for a large frame read in small chunks).
func readN(socket as net.Conn, n as int) {
    def out as bytes;
    while (len($out) < $n) {
        def chunk as bytes init net.readBytes($socket, $n - len($out));
        if (len($chunk) == 0) {
            fail("connection closed mid-frame");
        }
        def i as int init 0;
        while ($i < len($chunk)) {
            $out[] = $chunk[$i];
            $i = $i + 1;
        }
    }
    return $out;
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
    if (strings.contains($url, "\r") or strings.contains($url, "\n") or strings.contains($url, "\0")) {
        fail("URL contains a control character (CR/LF/NUL)");
    }
    def secure as bool init false;
    def rest as string init $url;
    if (strings.startsWith($url, "wss://")) {
        $secure = true;
        $rest = strings.substring($url, 6, len($url));
    } elseif (strings.startsWith($url, "ws://")) {
        $rest = strings.substring($url, 5, len($url));
    } else {
        fail("URL must start with ws:// or wss://");
    }
    def path as string init "/";
    def hostport as string init $rest;
    def slash as int init strings.indexOf($rest, "/");
    if ($slash >= 0) {
        $hostport = strings.substring($rest, 0, $slash);
        $path = strings.substring($rest, $slash, len($rest));
    }
    def host as string init $hostport;
    def port as int init 80;
    if ($secure) {
        $port = 443;
    }
    def colon as int init strings.indexOf($hostport, ":");
    if ($colon >= 0) {
        $host = strings.substring($hostport, 0, $colon);
        $port = convert.toInt(strings.substring($hostport, $colon + 1, len($hostport)));
    }
    return Target{ secure: $secure, host: $host, port: $port, path: $path };
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
        if ($m >= 4 and $buf[$m - 1] == 10 and $buf[$m - 2] == 13 and $buf[$m - 3] == 10 and $buf[$m - 4] == 13) {
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
    def rest as string init strings.substring($statusLine, $sp + 1, len($statusLine));
    def spTwo as int init strings.indexOf($rest, " ");
    if ($spTwo < 0) {
        return strings.trim($rest);
    }
    return strings.substring($rest, 0, $spTwo);
}

# headerValue finds a header's value (case-insensitive name) in the response
# lines, preserving the value's original case.
func headerValue(lines as list of string, name as string) {
    def i as int init 1;
    while ($i < len($lines)) {
        def line as string init $lines[$i];
        def colon as int init strings.indexOf($line, ":");
        if ($colon >= 0) {
            def key as string init strings.lower(strings.trim(strings.substring($line, 0, $colon)));
            if ($key == $name) {
                return strings.trim(strings.substring($line, $colon + 1, len($line)));
            }
        }
        $i = $i + 1;
    }
    return "";
}

# handshake sends the Upgrade request and verifies the 101 + accept key.
func handshake(socket as net.Conn, t as Target, key as string) {
    def hostHeader as string init $t.host;
    if ((not $t.secure and not ($t.port == 80)) or ($t.secure and not ($t.port == 443))) {
        $hostHeader = $t.host + ":" + convert.toString($t.port);
    }
    def req as string init "GET " + $t.path + " HTTP/1.1\r\n" +
        "Host: " + $hostHeader + "\r\n" +
        "Upgrade: websocket\r\n" +
        "Connection: Upgrade\r\n" +
        "Sec-WebSocket-Key: " + $key + "\r\n" +
        "Sec-WebSocket-Version: 13\r\n\r\n";
    net.writeBytes($socket, convert.bytesFromString($req, "utf-8"));
    def resp as string init readHandshakeResponse($socket);
    def lines as list of string init strings.split($resp, "\r\n");
    if (not (statusCodeOf($lines[0]) == "101")) {
        fail("handshake rejected: " + $lines[0]);
    }
    if (not (headerValue($lines, "sec-websocket-accept") == acceptFor($key))) {
        fail("invalid Sec-WebSocket-Accept");
    }
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
    $frame = appendBytes($frame, $mask);
    def j as int init 0;
    while ($j < $n) {
        $frame[] = $payload[$j] ^ $mask[$j % 4];
        $j = $j + 1;
    }
    return $frame;
}

# encodeFrame builds one client frame with a fresh random mask.
func encodeFrame(opcode as int, payload as bytes) {
    return encodeFrameMasked($opcode, $payload, randomBytes(4));
}

# readFrame reads and decodes one (unmasked, server) frame off the socket.
func readFrame(socket as net.Conn) {
    def h as bytes init readN($socket, 2);
    def opcode as int init $h[0] & 0x0f;
    def fin as int init ($h[0] >> 7) & 1;
    def masked as int init ($h[1] >> 7) & 1;
    def n as int init $h[1] & 0x7f;
    if ($n == 126) {
        def e as bytes init readN($socket, 2);
        $n = ($e[0] << 8) | $e[1];
    } elseif ($n == 127) {
        def e as bytes init readN($socket, 8);
        $n = 0;
        def i as int init 0;
        while ($i < 8) {
            $n = ($n << 8) | $e[$i];
            $i = $i + 1;
        }
    }
    def mkey as bytes;
    if ($masked == 1) {
        $mkey = readN($socket, 4);
    }
    if ($n > MAX_MESSAGE_BYTES) {
        throw Error{kind: "websocket", message: "websocket: frame declares " + convert.toString($n) + " bytes, over the " + convert.toString(MAX_MESSAGE_BYTES) + "-byte limit", file: "", line: 0, col: 0};
    }
    def payload as bytes;
    if ($n > 0) {
        $payload = readN($socket, $n);
    }
    if ($masked == 1) {
        def j as int init 0;
        while ($j < $n) {
            $payload[$j] = $payload[$j] ^ $mkey[$j % 4];
            $j = $j + 1;
        }
    }
    return Frame{ opcode: $opcode, fin: $fin, payload: $payload };
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
    def t as Target init parseUrl($url);
    def addr as string init $t.host + ":" + convert.toString($t.port);
    def socket as net.Conn;
    if ($t.secure) {
        $socket = net.connectTLS($addr, HANDSHAKE_TIMEOUT_MS);
    } else {
        $socket = net.connect($addr, HANDSHAKE_TIMEOUT_MS);
    }
    # A failed HTTP upgrade must not leak the socket; on success the caller
    # owns the open connection.
    errdefer net.close($socket);
    net.setDeadline($socket, HANDSHAKE_TIMEOUT_MS);
    handshake($socket, $t, makeKey());
    net.setDeadline($socket, 0);
    return Conn{ socket: $socket, timeoutMs: $timeoutMs };
}

# sendFrame writes one masked frame, bounded by the connection timeout.
func sendFrame(c as Conn, opcode as int, payload as bytes) {
    net.setDeadline($c.socket, $c.timeoutMs);
    net.writeBytes($c.socket, encodeFrame($opcode, $payload));
    net.setDeadline($c.socket, 0);
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
    net.setDeadline($c.socket, $c.timeoutMs);
    def acc as bytes;
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
            net.writeBytes($c.socket, encodeFrame(OP_PONG, $f.payload));
        } elseif ($f.opcode == OP_PONG) {
            # A pong interleaved in a fragmented message is ignored so
            # reassembly continues; only a standalone pong surfaces as a
            # message.
            if (not $started) {
                $control = Message{ kind: "pong", text: "", data: $f.payload };
                $isControl = true;
                $done = true;
            }
        } elseif ($f.opcode == OP_CLOSE) {
            $control = Message{ kind: "close", text: "", data: $f.payload };
            $isControl = true;
            $done = true;
        } else {
            $started = true;
            if (not ($f.opcode == OP_CONT)) {
                $dataOpcode = $f.opcode;
            }
            # Append fragment payload into `acc` in place (a by-value
            # appendBytes copies the whole reassembly buffer per fragment).
            def bi as int init 0;
            while ($bi < len($f.payload)) {
                $acc[] = $f.payload[$bi];
                $bi = $bi + 1;
            }
            if (len($acc) > MAX_MESSAGE_BYTES) {
                # Refuse the oversized message: send a 1009 (message too big)
                # close and abort, rather than growing memory unbounded.
                def code as bytes;
                $code[] = 3;
                $code[] = 241;
                net.writeBytes($c.socket, encodeFrame(OP_CLOSE, $code));
                throw Error{kind: "websocket", message: "websocket: reassembled message exceeds the " + convert.toString(MAX_MESSAGE_BYTES) + "-byte limit", file: "", line: 0, col: 0};
            }
            if ($f.fin == 1) {
                $done = true;
            }
        }
    } until ($done);
    net.setDeadline($c.socket, 0);
    if ($isControl) {
        return $control;
    }
    if ($dataOpcode == OP_TEXT) {
        return Message{ kind: "text", text: convert.stringFromBytes($acc, "utf-8"), data: $acc };
    }
    return Message{ kind: "binary", text: "", data: $acc };
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
