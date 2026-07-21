# `websocket` - RFC 6455 WebSocket client

Import with `import "websocket.j" as websocket;`. Open a WebSocket connection,
send and receive text / binary messages, and close it - a full RFC 6455 client
over [`net`](../libraries/net.md). `connect` performs the HTTP `Upgrade`
handshake and **verifies** the server's `Sec-WebSocket-Accept` (SHA-1 + base64
of the client key); `send` / `sendBytes` write masked frames (client-to-server
frames must be masked); `receive` reads the next message, answering pings with
pongs and reassembling fragmented messages. Needs the default `jennifer` binary.
A protocol error or dropped connection throws `Error{kind: "websocket"}`.

```jennifer
import "websocket.j" as websocket;

def ws as websocket.Conn init websocket.connect("wss://ws.postman-echo.com/raw");
websocket.send($ws, "hello");
def m as websocket.Message init websocket.receive($ws);   # m.kind "text", m.text "hello"
websocket.close($ws);
```

Runnable: [`examples/modules/websocket_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/websocket_demo.j).

## Connecting

`ws://` is a plain TCP connection; `wss://` is TLS (`net.connectTLS`). The port
defaults to 80 / 443 and may be overridden in the URL (`ws://host:9001/path`).

| Call | Returns | |
| ---- | ------- | - |
| `websocket.connect(url)` | `Conn` | handshake with the default receive timeout (30 s) |
| `websocket.connectWith(url, timeoutMs)` | `Conn` | handshake with an explicit per-receive timeout |

```jennifer
def struct websocket.Conn { socket as net.Conn, timeoutMs as int };
```

The handshake sends a random 16-byte `Sec-WebSocket-Key` and rejects the
connection (throws) unless the response is `101` and its
`Sec-WebSocket-Accept` matches `base64(SHA1(key + GUID))`.

## Sending and receiving

| Call | Returns | |
| ---- | ------- | - |
| `websocket.send(c, text)` | | send a text message |
| `websocket.sendBytes(c, data)` | | send a binary message |
| `websocket.ping(c)` | | send a ping |
| `websocket.receive(c)` | `Message` | read the next message |
| `websocket.close(c)` | | send a close frame and shut the socket |

Outgoing frames are masked with a fresh key (as the spec requires) and length-
encoded in the 7-bit, 16-bit, or 64-bit form automatically. `receive` returns a
`Message`:

```jennifer
def struct websocket.Message {
    kind as string,   # "text", "binary", "close", or "pong"
    text as string,   # decoded text (for a "text" message; "" otherwise)
    data as bytes     # the raw payload bytes
};
```

`receive` transparently answers a **ping** with a pong and keeps reading, and
reassembles a **fragmented** message (continuation frames) into one `Message`. A
**close** frame surfaces as `kind == "close"` - stop reading and `close` the
connection. Each `receive` is bounded by the connection's `timeoutMs`; a timeout
throws.

```jennifer
def m as websocket.Message init websocket.receive($ws);
if ($m.kind == "close") {
    websocket.close($ws);
} elseif ($m.kind == "text") {
    # handle $m.text
}
```

## Scope

- **Client only.** A server-side upgrade would need an `httpd`
  connection-hijack hook (a separate, larger piece).
- **Crypto-grade masking key / nonce.** The 4-byte frame mask and the handshake
  nonce come from the [`crypto`](../libraries/crypto.md) library, as RFC 6455
  (section 5.3) requires: the mask must be "derived from a strong source of
  entropy" and hard to predict frame to frame.
- **No permessage-deflate / extensions, no subprotocol negotiation.** The
  `Sec-WebSocket-Protocol` / `-Extensions` headers are not sent.
- **`receive` blocks per message** up to `timeoutMs`; there is no non-blocking
  poll (drive one message at a time, or run `receive` in a `spawn`).
- **Bounded connect and message size.** The opening handshake is bounded by a
  connection-establishment timeout (a slow or unreachable server fails the dial),
  and a received message is capped at 64 MiB - a peer that declares an oversized
  frame or reassembled message is refused with a catchable error, not an
  unbounded allocation.

## See also

- [net.md](../libraries/net.md) - the TCP / TLS transport this is built on.
- [http.md](http.md) - the HTTP/1.1 client (a different, request/response
  protocol over the same `net`).
- [modules/index.md](index.md) - the module catalog and import rules.
