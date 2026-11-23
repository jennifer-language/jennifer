# `websocket` - RFC 6455 WebSocket client

Import with `import "websocket.j" as websocket;`. Open a WebSocket connection,
send and receive text / binary messages, and close it - a full RFC 6455 client
over [`net`](../libraries/net.md). `connect` performs the HTTP `Upgrade`
handshake and **verifies** the server's `Sec-WebSocket-Accept` (SHA-1 + base64
of the client key); `send` / `sendBytes` write masked frames (client-to-server
frames must be masked); `receive` reads the next message, answering pings with
pongs and reassembling fragmented messages. Needs the default `jennifer` binary.
A protocol error throws `Error{kind: "websocket"}`; a connection dropped
mid-frame is the generic kind `"closed"`.

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
| `websocket.connectOptions(url, options)` | `Conn` | handshake with extra headers, subprotocols, TLS trust and a timeout |
| `websocket.defaultOptions()` | `Options` | the zero `Options`, which is what `connect` uses |

A `Conn` has `socket` (the plain or TLS `net.Conn`), `timeoutMs` (the
per-receive timeout) and `protocol` (the subprotocol the server chose).

The handshake sends a random 16-byte `Sec-WebSocket-Key` and rejects the
connection (throws) unless the response is `101` with `Upgrade: websocket` and
`Connection: Upgrade`, its `Sec-WebSocket-Accept` matches
`base64(SHA1(key + GUID))`, and it picks no subprotocol or extension that was
not offered (RFC 6455 section 4.1).

A URL may name an IPv6 host in brackets (`ws://[::1]:8080/`) and carry a query
(`wss://host/live?room=1`); a fragment is dropped. One with no host, a port
outside 1 to 65535, whitespace in the path, or credentials (`user:pw@`) is
refused: send credentials as a header instead.

### Options

| Field | Type | |
| ----- | ---- | - |
| `timeoutMs` | `int` | per-receive timeout (0 = 30 s) |
| `headers` | `map of string to string` | extra handshake headers, such as an API key |
| `protocols` | `list of string` | subprotocols to offer (`Sec-WebSocket-Protocol`), preferred first |
| `caCert` | `bytes` | a PEM certificate to trust besides the system roots (`wss://`) |
| `skipVerify` | `bool` | accept any certificate (trusted LAN only: it exposes the connection to a man-in-the-middle) |

A GraphQL subscription server, for example, wants an API key and the
`graphql-transport-ws` subprotocol, and often runs on a self-signed
certificate:

```jennifer
use fs;
use os;
import "websocket.j" as websocket;

def o as websocket.Options init websocket.defaultOptions();
$o.headers = {"x-api-key": os.getEnv("API_KEY")};
$o.protocols = ["graphql-transport-ws"];
$o.caCert = fs.readBytes("server-ca.pem");
def ws as websocket.Conn init websocket.connectOptions("wss://tower.lan/graphql", $o);
if ($ws.protocol != "graphql-transport-ws") {
    websocket.close($ws);   # the server speaks none of the offered subprotocols
}
```

`Conn.protocol` is the subprotocol the server chose, or `""` when it chose
none; whether to carry on without one is the caller's call. A header name
that is not an HTTP token, a value with a control character, and the headers
the handshake writes itself (`Host`, `Upgrade`, `Connection`, the
`Sec-WebSocket-*` set) are refused before anything is sent.
`Sec-WebSocket-Extensions` is among them, because the module cannot decode
compressed frames.

## Sending and receiving

| Call | Returns | |
| ---- | ------- | - |
| `websocket.send(c, text)` | | send a text message |
| `websocket.sendBytes(c, data)` | | send a binary message |
| `websocket.ping(c)` | | send a ping |
| `websocket.receive(c)` | `Message` | read the next message |
| `websocket.close(c)` | | send a close frame and shut the socket |

Outgoing frames are masked with a fresh key (as the spec requires) and
length-encoded in the 7-bit, 16-bit, or 64-bit form. `receive` returns a
`Message`:

| Field | Type | |
| ----- | ---- | - |
| `kind` | `string` | `"text"`, `"binary"`, `"close"`, or `"pong"` |
| `text` | `string` | the decoded text (for a `"text"` message; `""` otherwise) |
| `data` | `bytes` | the raw payload bytes |

`receive` answers a **ping** with a pong and keeps reading, and reassembles a **fragmented** message (continuation frames) into one `Message`. A
**close** frame surfaces as `kind == "close"` - stop reading and `close` the
connection. Each `receive` is bounded by the connection's `timeoutMs`; a timeout
throws. A frame or reassembled message over 64 MiB throws kind `"limit"`. What
RFC 6455 tells a client to refuse throws `Error{kind: "websocket"}`: a masked
server frame, an unknown opcode, a reserved bit set
(an extension nobody negotiated), a control frame that is fragmented or
longer than 125 bytes, a continuation with no message to continue, a new
message inside a fragmented one, and a text message that is not valid UTF-8.

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
- **No permessage-deflate or other extensions.** Subprotocols are offered with
  `Options.protocols`; extensions are never offered, and a server that enables
  one anyway is refused.
- **`receive` blocks per message** up to `timeoutMs`; there is no non-blocking
  poll (drive one message at a time, or run `receive` in a `spawn`).
- **Bounded connect and message size.** The connect and opening handshake are
  bounded by a 10 s timeout (a slow or unreachable server fails the dial), and a
  received message is capped at 64 MiB - a peer that declares an oversized
  frame or reassembled message is refused with a catchable kind `"limit"`
  error, not an unbounded allocation.

## See also

- [net.md](../libraries/net.md) - the TCP / TLS transport this is built on.
- [http.md](http.md) - the HTTP/1.1 client (a different, request/response
  protocol over the same `net`).
- [modules/index.md](index.md) - the module catalog and import rules.

## API reference

Every exported function, struct and constant of `websocket`, with its signature, generated
from the module's doc comments: [the `websocket` API reference](../api/websocket.md).
