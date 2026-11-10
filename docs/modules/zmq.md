# `zmq` - ZeroMQ sockets over ZMTP 3.1

Import with `import "zmq.j" as zmq;`. Talk to ZeroMQ peers - programs using
libzmq, pyzmq or another ZeroMQ binding - from Jennifer. The module speaks
**ZMTP 3.1** (ZeroMQ RFC 37), the wire protocol ZeroMQ uses over TCP, directly
over the [`net`](../libraries/net.md) library: no libzmq, no C, nothing to
install. It covers the classic socket types, multipart messages, and the NULL
and PLAIN security mechanisms.

```jennifer
import "zmq.j" as zmq;
use io;

def s as zmq.Socket init zmq.connect("REQ", "127.0.0.1:5555");
zmq.sendText($s, "hello");
io.printf("%s\n", zmq.text(zmq.receive($s)));
zmq.close($s);
```

Runnable: [`examples/modules/zmq_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/zmq_demo.j)
(request / reply and publish / subscribe, both ends in one program).

## How it differs from libzmq

libzmq runs a background I/O thread per context: a socket connects to and
binds many endpoints, queues messages, reconnects on its own, and balances or
fans out across peers. This module keeps Jennifer's explicit model instead:

- **One socket is one connection to one peer.** `connect` dials one peer;
  `listen` + `accept` take peers one at a time. To serve several clients,
  `accept` in a loop and `spawn` a task per socket.
- **Nothing happens in the background.** The program reads with `receive`
  (blocking) or `poll` (with a timeout). A peer's heartbeat PING is answered
  while a `receive` / `poll` is reading, so a peer with a heartbeat timeout
  (libzmq `ZMQ_HEARTBEAT_TIMEOUT`) disconnects a socket nobody reads for longer
  than that.
- **No reconnect, no send queue.** `send` writes to the connection and throws
  if it is gone; reconnecting is the program's choice.

## Socket types

| Type | Talks to | Sends | Receives |
| ---- | -------- | ----- | -------- |
| `"REQ"` | REP, ROUTER | `send` / `sendText` (an empty delimiter frame is added) | `receive` (the delimiter is removed) |
| `"REP"` | REQ, DEALER | `reply` to a received request | `receive`; routing frames go to `envelope` |
| `"DEALER"` | REP, DEALER, ROUTER | `send`, frames as given | `receive`, frames as sent |
| `"ROUTER"` | REQ, DEALER, ROUTER | `send` with the peer's routing id first, or `reply` | `receive`; `envelope` holds the peer's routing id |
| `"PUB"` | SUB | `send` | - |
| `"SUB"` | PUB | - | `receive` of messages a subscription matches |
| `"PUSH"` | PULL | `send` | - |
| `"PULL"` | PUSH | - | `receive` |
| `"PAIR"` | PAIR | `send` | `receive` |

The handshake checks the pairing: a `REQ` that reaches a `PULL` peer fails with
`Error{kind: "zmq"}` on both sides.

## Surface

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `zmq.defaultOptions()` | `Options` | NULL mechanism, no identity, 5 s timeout, 64 MiB message cap. |
| `zmq.connect(kind, address)` | `Socket` | Dial `"host:port"` (a `tcp://` prefix is accepted) and run the handshake. |
| `zmq.connectWith(kind, address, opts)` | `Socket` | The same, with options. |
| `zmq.listen(kind, address)` / `zmq.listenWith(kind, address, opts)` | `Listener` | Listen; `":0"` / `"127.0.0.1:0"` picks a free port. |
| `zmq.accept(listener)` | `Socket` | Wait for the next peer and run the handshake. A failed handshake closes that connection and throws; call `accept` again. |
| `zmq.address(listener)` | `string` | The bound `"host:port"`. |
| `zmq.send(sock, frames)` | `null` | Send a message, one frame per `list of bytes` element. |
| `zmq.sendText(sock, text)` | `null` | Send a one-frame UTF-8 message. |
| `zmq.reply(sock, request, frames)` | `null` | Answer a request on a REP or ROUTER socket (see below). |
| `zmq.receive(sock)` | `Message` | Block for the next message. |
| `zmq.poll(sock, timeoutMs)` | `list of Message` | Zero or one message: empty when nothing arrived in `timeoutMs`. |
| `zmq.subscribe(sock, prefix)` / `zmq.unsubscribe(sock, prefix)` | `Socket` | A SUB socket with the topic prefix added / removed; `""` matches everything. |
| `zmq.text(msg)` | `string` | The first frame as UTF-8. |
| `zmq.close(sock)` / `zmq.closeListener(listener)` | `null` | Close. |

### Types

- `zmq.Options { mechanism, username, password, identity, timeoutMs, maxMessageBytes }`
  - `mechanism` - `"NULL"` (or `""`) or `"PLAIN"`; both peers must use the same.
  - `username` / `password` - PLAIN credentials (see [PLAIN](#plain)).
  - `identity` (`bytes`) - the routing id a ROUTER peer sees for this REQ /
    DEALER / ROUTER socket; empty lets the ROUTER pick one. At most 255 bytes,
    not starting with a zero byte.
  - `timeoutMs` - the connect + handshake timeout, and how long the rest of a
    message may take once `poll` has seen its start (0 = 5000).
  - `maxMessageBytes` - the largest message accepted, all frames together (0 =
    64 MiB). A larger one throws `Error{kind: "limit"}`.
- `zmq.Socket` - the connection plus what the handshake learned: `kind`,
  `peerKind`, `peerIdentity`, `peerMinor` (the peer's ZMTP minor version), and a
  SUB socket's `subscriptions`. Copies share the connection.
- `zmq.Message { frames as list of bytes, envelope as list of bytes }` -
  `envelope` is set only on REP and ROUTER sockets.

Since a socket is a value, the calls that change it return the new one:
`$s = zmq.subscribe($s, "news.");`.

## Requests, replies and envelopes

A `REQ` socket adds an empty **delimiter frame** before each message, and a
`REP` socket keeps everything before the delimiter as the **envelope**: the
routing frames that ROUTER sockets in between add, so a reply finds its way back.
`receive` on REP returns the body in `frames` and those routing frames in
`envelope`; `reply` puts them back in front of the answer:

```jennifer
use convert;

def l as zmq.Listener init zmq.listen("REP", "0.0.0.0:5555");
def s as zmq.Socket init zmq.accept($l);
while (true) {
    def req as zmq.Message init zmq.receive($s);
    zmq.reply($s, $req, [convert.bytesFromString("utf-8", "ok: " + zmq.text($req))]);
}
```

On a `ROUTER` socket, `receive` puts the peer's routing id in `envelope` and the
frames exactly as the peer sent them in `frames` - for a REQ peer that includes
its empty delimiter. Send to the peer with `send($r, [routingId, ...])` or
`reply($r, $req, frames)`, adding the empty delimiter frame yourself when the
peer is a REQ. The routing id is the peer's `identity` option if it set one,
otherwise one the ROUTER makes up (a zero byte and four random bytes, as libzmq
does).

## Publish / subscribe

A SUB socket receives nothing until it subscribes. `subscribe` sends the
subscription to the publisher (a SUBSCRIBE command to a ZMTP 3.1 peer, a
subscription message to a 3.0 one) and `receive` keeps only messages whose first
frame starts with a subscribed prefix. Put the topic in the first frame and the
data after it:

```jennifer
def s as zmq.Socket init zmq.subscribe(zmq.connect("SUB", "127.0.0.1:5556"), "weather.");
def m as zmq.Message init zmq.receive($s);
# $m.frames[0] is the topic, $m.frames[1] the data
```

A Jennifer **PUB** socket sends every message to its peer; it does not read the
peer's subscriptions, so filtering happens at the subscriber. libzmq's SUB filters
on its side too, so this is correct with any peer - only bandwidth is spent on
messages nobody subscribed to.

## PLAIN

`"PLAIN"` sends a username and password during the handshake, **in clear
text**: use it only on a trusted network, or tunnel the connection (SSH, a VPN,
`stunnel`). CURVE, the encrypted mechanism, is not supported.

An empty `password` throws kind `zmq` when the socket is built: a PLAIN
listener with no password would accept anyone sending empty credentials.

- The **listening** side is the PLAIN server. `listenWith` with
  `mechanism: "PLAIN"` accepts a peer only if it sends exactly the options'
  `username` and `password` (compared in constant time); otherwise it answers
  with an ERROR and `accept` throws.
- The **connecting** side is the client and sends its options' credentials. A
  refusal throws `Error{kind: "zmq"}` with the server's reason.
- A **libzmq** PLAIN server (`ZMQ_PLAIN_SERVER`) checks passwords through a ZAP
  handler (ZeroMQ RFC 27). Without one installed in that process, it refuses
  every client, this module's included.

## Errors and limits

- `Error{kind: "zmq"}` - an unknown socket type, bad options, a handshake
  failure (not ZMTP, ZMTP below 3, a different mechanism, an incompatible socket
  type, refused credentials), a peer ERROR, or a call the socket type does not
  allow (`send` on SUB / PULL / REP, `receive` on PUB / PUSH).
- `Error{kind: "limit"}` - a message over `maxMessageBytes`, or with more than
  65536 frames, so a hostile peer cannot make the program allocate without bound.
- The `net` kinds pass through: `"timeout"` (no connection or handshake in
  `timeoutMs`), `"closed"` (the peer went away), `"runtime"`.

A REQ reply without the delimiter, or a REP request without one, is dropped, as
libzmq does.

## Not supported

CURVE encryption; the `ipc://`, `inproc://` and multicast transports; XPUB /
XSUB, STREAM and the draft socket types as this socket's own type; ZMTP 1.0 and
2.0 peers (libzmq before 4.0); socket options that only matter with libzmq's
queues (high-water marks, linger, reconnect intervals).

## Platforms

Pure Jennifer over `net`, `binary` and `crypto`. It needs the default `jennifer`
binary: the module declares the `net` capability, and the stock `jennifer-tiny`
has no network driver.

Tested against libzmq 4.3.5 in both directions: REQ / REP, PUB / SUB,
DEALER / ROUTER (with an identity), PUSH / PULL with multipart and long frames,
PLAIN (good and bad passwords, with a ZAP handler on the libzmq side), and
libzmq heartbeats.
