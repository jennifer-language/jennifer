# `mqtt` - an MQTT 3.1.1 pub/sub client

Import with `import "mqtt.j" as mqtt;`. An **MQTT 3.1.1** publish/subscribe
client over the `net` system library - the same "protocol clients are modules,
`net` is the transport" line the other network clients follow. MQTT packets are
a 1-byte fixed header, a variable remaining-length integer, then a
length-prefixed payload; the module builds and parses them with Jennifer's
bitwise operators (`& | ^ ~ << >>`) and `bytes`. It does QoS 0 and QoS 1
publish/subscribe (a synchronous PUBACK handshake), retained messages, a
Last-Will registered in the CONNECT, and `reconnect` for session resumption.
Because it uses `net`, this module needs the default **`jennifer`** binary.

> **On `jennifer-tiny`:** "needs the default `jennifer` binary" refers to the
> **stock** tiny build, which ships without a network driver - not a TinyGo
> limitation. A `jennifer-tiny` rebuilt with a network stack runs this module
> too; see the
> [note on `net` and TinyGo](../technical/tinygo.md#net-on-tinygo-is-a-build-choice-not-a-hard-limit).

```jennifer
import "mqtt.j" as mqtt;

def c as mqtt.Client init mqtt.connect(mqtt.Options{host: "127.0.0.1",
    port: 1883, clientId: "demo", keepalive: 30, security: "none",
    username: "", password: ""});
$c = mqtt.subscribe($c, "sensors/temp");
mqtt.publish($c, "sensors/temp", "21.5");
def m as mqtt.Message init mqtt.receive($c);
io.printf("%s -> %s\n", $m.topic, convert.stringFromBytes($m.payload, "utf-8"));
mqtt.disconnect($c);
```

`subscribe` / `subscribeQos1` return an updated `Client` (they record the
subscription so `reconnect` can restore it), so reassign the result:
`$c = mqtt.subscribe($c, ...)`. A `Client` stays value-semantic - copies share
the socket handle but each carries its own settings and subscription list.

Runnable: [`examples/modules/mqtt_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/mqtt_demo.j).

## Surface

A client is stateful: `connect`, subscribe / publish / receive, `disconnect`.

| Call / type                          | Notes                                                                 |
| ------------------------------------ | --------------------------------------------------------------------- |
| `mqtt.Options`                       | `host`, `port`, `clientId`, `keepalive` (seconds), `security`, `username`, `password`. |
| `mqtt.Will`                          | A Last-Will: `topic` (`""` disables it), `payload` (bytes), `qos`, `retain`. |
| `mqtt.Subscription`                  | A tracked subscription: `topic`, `qos` (the granted QoS).             |
| `mqtt.Client`                        | A live connection (from `connect` / `connectWith`), carrying the settings and tracked subscriptions `reconnect` needs. |
| `mqtt.Message`                       | A received message: `topic` (string), `payload` (bytes).              |
| `mqtt.connect(opts)`                 | Open a connection (clean session, no will), send CONNECT, check the CONNACK. |
| `mqtt.connectWith(opts, will, cleanSession)` | Connect with an explicit Last-Will and clean-session flag.    |
| `mqtt.reconnect(client)`             | Re-dial, re-CONNECT with the stored settings, re-subscribe the tracked routes; returns a fresh `Client`. |
| `mqtt.subscribe(client, topic)`      | Subscribe at QoS 0, wait for the SUBACK, track it; returns an updated `Client`. |
| `mqtt.subscribeQos1(client, topic)`  | Subscribe requesting QoS 1 (parses the granted QoS), track it; returns an updated `Client`. |
| `mqtt.publish(client, topic, message)` | Publish a UTF-8 text message at QoS 0 (fire and forget).            |
| `mqtt.publishBytes(client, topic, payload)` | Publish a raw `bytes` payload at QoS 0.                        |
| `mqtt.publishRetain(client, topic, message, retain)` | Publish text at QoS 0 with an explicit retain flag.  |
| `mqtt.publishBytesRetain(client, topic, payload, retain)` | Publish `bytes` at QoS 0 with a retain flag.    |
| `mqtt.publishQos1(client, topic, payload, retain)` | Publish `bytes` at QoS 1 and block for the matching PUBACK. |
| `mqtt.receive(client)`               | Block until the next application message arrives; returns a `Message` (PUBACKs a QoS-1 PUBLISH first). |
| `mqtt.poll(client, timeoutMs)`       | Poll up to `timeoutMs` ms; returns a `list of Message` of length 0 or 1 (PUBACKs a QoS-1 PUBLISH first). |
| `mqtt.ping(client)`                  | Send a PINGREQ keepalive (fire and forget).                            |
| `mqtt.disconnect(client)`            | Send DISCONNECT and close.                                             |

`Options.security` is a [`transport.Security`](transport.md): `.None` (plaintext,
port 1883) or `.Tls` (implicit TLS, `mqtts`, port 8883); `.Starttls` is rejected
(MQTT has no in-band upgrade). Needs `import "transport.j" as transport;`.
`username` / `password` `""` omit the CONNECT
credentials. A non-empty `clientId` identifies the session to the broker.

## QoS 1: the PUBACK handshake

`publishQos1(client, topic, payload, retain)` sends a PUBLISH with QoS 1 and a
packet identifier, then blocks until the broker returns the matching **PUBACK**
(at-least-once delivery). If no PUBACK arrives within the wait window the
message is re-sent with the **DUP** (duplicate-delivery) flag set and the same
packet id, up to a fixed number of attempts; if the budget is exhausted it
throws an `Error` (kind `"mqtt"`). Because the call blocks for the PUBACK, only
one message is ever in flight, so a fixed non-zero packet id is safe and the
broker de-duplicates a re-sent PUBLISH by that id.

On the receive side, `subscribeQos1(client, topic)` asks the broker for QoS 1
and records the QoS it **granted** (parsed from the SUBACK - a broker may
downgrade to QoS 0, or reject with `0x80`). When a QoS-1 PUBLISH then arrives,
`receive` / `poll` send its PUBACK **before** returning the `Message`, so the
broker considers it delivered. QoS 0 is unchanged: `publish` / `publishBytes`
are fire-and-forget, and a QoS-0 PUBLISH is returned without any acknowledgement.

```jennifer
$c = mqtt.subscribeQos1($c, "commands/#");
mqtt.publishQos1($c, "commands/reboot",
    convert.bytesFromString("now", "utf-8"), false);   # blocks for the PUBACK
def m as mqtt.Message init mqtt.receive($c);            # PUBACKs it, then returns
```

## Retained messages and the Last-Will

Any publish takes a **retain** flag: `publishRetain` / `publishBytesRetain` at
QoS 0 and `publishQos1` at QoS 1. A retained message is stored by the broker and
delivered to every future subscriber of the topic at subscribe time; publishing
an **empty** payload with `retain: true` clears the retained message.

A **Last-Will** is a message the broker publishes on the client's behalf if the
connection drops without a clean `disconnect`. Register it in the CONNECT with
`connectWith(opts, will, cleanSession)`; a `Will` with an empty `topic` sets no
will (what plain `connect` does).

```jennifer
def will as mqtt.Will init mqtt.Will{topic: "clients/demo/status",
    payload: convert.bytesFromString("offline", "utf-8"), qos: 1, retain: true};
def c as mqtt.Client init mqtt.connectWith($opts, $will, false);  # persistent session
mqtt.publishRetain($c, "clients/demo/status", "online", true);   # clear on clean exit
```

## Reconnect and session resumption

`reconnect(client)` re-dials the broker, re-sends CONNECT with the client's
stored `opts` / `will` / `cleanSession`, and re-subscribes every subscription
the client tracked (via `subscribe` / `subscribeQos1`); it returns a **fresh
`Client`** the caller reassigns (`$c = mqtt.reconnect($c);`), best-effort
closing the stale socket.

Session resumption is the broker's job, gated by the clean-session flag. Connect
with `connectWith(opts, will, false)` (clean-session **false**) and the broker
keeps the session keyed by `clientId` across a drop - the retained subscriptions
and any queued QoS-1 messages - and reports it in the CONNACK's session-present
bit. The re-subscribe `reconnect` performs is idempotent in that case, and also
covers a clean-session (`true`) client, where the broker discarded the session
and the routes must be re-established from scratch. Reconnect timing (when to
give up and retry, backoff) is the caller's loop, the same division of labor as
keepalive.

## Single-threaded poll with timeout

Jennifer has no handler callbacks, so a subscriber drives its own loop. `poll`
arms a read deadline (via [`net.setDeadline`](../libraries/net.md)) so one flow
can wait for a message and, when idle, do other work - send a keepalive, check a
clock - without dedicating a `spawn`ed reader. It returns a list of zero or one
message: empty when nothing arrived in the window, one `Message` when a PUBLISH
was received. Non-PUBLISH control packets (a PINGRESP) are consumed and reported
as an empty poll.

```jennifer
def running as bool init true;
def ticks as int init 0;
while ($running) {
    def msgs as list of mqtt.Message init mqtt.poll($c, 1000);
    if (len($msgs) > 0) {
        def m as mqtt.Message init $msgs[0];
        io.printf("%s -> %s\n", $m.topic,
            convert.stringFromBytes($m.payload, "utf-8"));
    } else {
        $ticks = $ticks + 1;
        if ($ticks == 20) {    # ~20s idle
            mqtt.ping($c);     # keepalive; the PINGRESP is consumed by poll
            $ticks = 0;
        }
    }
}
```

`receive` is the blocking counterpart: it waits for the next PUBLISH with no
timeout, skipping any control packets in between.

Keepalive is the caller's job (call `ping` on your own cadence): the module
holds no mutable timing state - a `Client` is value-semantic, sharing only the
underlying socket handle across copies.

## Errors

`connect` / `connectWith` throw a catchable `Error` (kind `"mqtt"`) when the
broker refuses the connection (a non-zero CONNACK code) or does not answer with
a CONNACK; `subscribe` / `subscribeQos1` throw when the SUBACK reports failure
(`0x80`); `publishQos1` throws when no PUBACK arrives within the retry budget. A
connection that closes mid-packet throws `mqtt: connection closed mid-packet`. A
`poll` whose deadline elapses is **not** an error - it simply returns an empty
list.

## Testing

The pure packet logic - the remaining-length varint encode / decode, the
length-prefixed string framing, the CONNECT builder (including the Last-Will and
clean-session flags), the QoS-1 PUBLISH builder (packet id + DUP / retain
flags), the PUBACK build / parse, the packet-id extraction, and the SUBACK
granted-QoS parse - is unit-tested in the overlay (`modules/mqtt_test.j`). The
networked connect / subscribe / publish / receive / poll round-trip (QoS 0 and
QoS 1, retained, will, reconnect) is covered end to end by an in-process
MQTT-broker fake in the Go test suite (`TestMqttPubSub`), so it runs in CI
without a broker install.

## Out of scope

MQTT 3.1.1 with QoS 0 and QoS 1. Deferred until a workload needs them:

- **QoS 2** exactly-once (PUBREC / PUBREL / PUBCOMP with persistent packet-id
  state).
- **Multiple in-flight QoS-1 messages.** `publishQos1` is synchronous (one
  message in flight, a fixed packet id); a pipelined sender with a packet-id
  window is a larger design.
- **MQTT 5 properties.**

If full QoS with high-throughput processing ever makes the tree-walker the
bottleneck, a Go-backed engine (build-tag split like `net`) is the fallback -
but the pub/sub basics belong in a module.

## Timeouts and limits

The initial connect is bounded by a connection-establishment timeout, so a slow
or unreachable broker fails the dial instead of blocking it forever, and the
CONNECT and SUBSCRIBE handshakes carry a 30 s timeout, so a broker that accepts
the connection but never acknowledges fails instead of hanging. `publishQos1`
waits a bounded window for each PUBACK and retries a fixed number of times
before throwing. `poll(client,
ms)` already bounds how long it waits for a message; `receive` blocks until one
arrives. A single control packet is capped at **64 MiB**: the remaining-length
varint is attacker-declarable, so a broker declaring an enormous packet fails
with a catchable error rather than an unbounded allocation.

## See also

- [net.md](../libraries/net.md) - the transport `mqtt` builds on, including
  `net.setDeadline` for the poll loop.
- [idna.md](idna.md) - the other module doing bit-level `bytes` work (Punycode).
- [modules/index.md](index.md) - the module catalog and import rules.
