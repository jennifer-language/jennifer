# `mqtt` - an MQTT 3.1.1 pub/sub client

Import with `import "mqtt.j" as mqtt;`. An **MQTT 3.1.1** publish/subscribe
client over the `net` system library - the same "protocol clients are modules,
`net` is the transport" line the other network clients follow. MQTT packets are
a 1-byte fixed header, a variable remaining-length integer, then a
length-prefixed payload; the module builds and parses them with Jennifer's
bitwise operators (`& | ^ ~ << >>`) and `bytes`. Because it uses `net`, this
module needs the default **`jennifer`** binary.

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
mqtt.subscribe($c, "sensors/temp");
mqtt.publish($c, "sensors/temp", "21.5");
def m as mqtt.Message init mqtt.receive($c);
io.printf("%s -> %s\n", $m.topic, convert.stringFromBytes($m.payload, "utf-8"));
mqtt.disconnect($c);
```

Runnable: [`examples/modules/mqtt_demo.j`](https://github.com/mplx/jennifer-lang/blob/main/examples/modules/mqtt_demo.j).

## Surface

A client is stateful: `connect`, subscribe / publish / receive, `disconnect`.

| Call / type                          | Notes                                                                 |
| ------------------------------------ | --------------------------------------------------------------------- |
| `mqtt.Options`                       | `host`, `port`, `clientId`, `keepalive` (seconds), `security`, `username`, `password`. |
| `mqtt.Client`                        | A live connection (from `connect`).                                   |
| `mqtt.Message`                       | A received message: `topic` (string), `payload` (bytes).              |
| `mqtt.connect(opts)`                 | Open a connection, send CONNECT, check the CONNACK return code.        |
| `mqtt.subscribe(client, topic)`      | Subscribe to a topic filter at QoS 0 and wait for the SUBACK.          |
| `mqtt.publish(client, topic, message)` | Publish a UTF-8 text message at QoS 0 (fire and forget).            |
| `mqtt.publishBytes(client, topic, payload)` | Publish a raw `bytes` payload at QoS 0.                        |
| `mqtt.receive(client)`               | Block until the next application message arrives; returns a `Message`. |
| `mqtt.poll(client, timeoutMs)`       | Poll up to `timeoutMs` ms; returns a `list of Message` of length 0 or 1. |
| `mqtt.ping(client)`                  | Send a PINGREQ keepalive (fire and forget).                            |
| `mqtt.disconnect(client)`            | Send DISCONNECT and close.                                             |

`Options.security` is `"none"` (plaintext, port 1883) or `"tls"` (implicit
TLS, `mqtts`, port 8883). `username` / `password` `""` omit the CONNECT
credentials. A non-empty `clientId` identifies the session to the broker.

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

`connect` throws a catchable `Error` (kind `"mqtt"`) when the broker refuses the
connection (a non-zero CONNACK code) or does not answer with a CONNACK;
`subscribe` throws when the SUBACK reports failure. A connection that closes
mid-packet throws `mqtt: connection closed mid-packet`. A `poll` whose deadline
elapses is **not** an error - it simply returns an empty list.

## Testing

The pure packet logic - the remaining-length varint encode / decode, the
length-prefixed string framing, the CONNECT builder, and the PUBLISH parser
(including the QoS>0 packet-id skip) - is unit-tested in the overlay
(`modules/mqtt_test.j`). The networked connect / subscribe / publish / receive /
poll round-trip is covered end to end by an in-process MQTT-broker fake in the
Go test suite (`TestMqttPubSub`), so it runs in CI without a broker install.

## Out of scope

Basics-first (MQTT 3.1.1, QoS 0). Deferred until a workload needs them:

- **QoS 1 / 2** handshakes (PUBACK / PUBREC / PUBREL / PUBCOMP with persistent
  packet-id state).
- **Retained messages and the will.**
- **Auto-reconnect / session resumption.**
- **MQTT 5 properties.**

If full QoS 1/2 with high-throughput processing ever makes the tree-walker the
bottleneck, a Go-backed engine (build-tag split like `net`) is the fallback -
but the pub/sub basics belong in a module.

## Timeouts

The CONNECT and SUBSCRIBE handshakes carry a 30 s timeout, so a broker that
accepts the connection but never acknowledges fails instead of hanging.
`poll(client, ms)` already bounds how long it waits for a message; `receive`
blocks until one arrives.

## See also

- [net.md](../libraries/net.md) - the transport `mqtt` builds on, including
  `net.setDeadline` for the poll loop.
- [idna.md](idna.md) - the other module doing bit-level `bytes` work (Punycode).
- [modules/index.md](index.md) - the module catalog and import rules.
