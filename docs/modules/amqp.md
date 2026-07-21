# `amqp` - AMQP 0-9-1 client (RabbitMQ)

Import with `import "amqp.j" as amqp;`. A client for RabbitMQ and compatible
AMQP 0-9-1 brokers over [`net`](../libraries/net.md): connect, declare a queue,
publish messages, and pull them back. The binary frame and method encoding is
built by hand from `bytes` and the bitwise operators - the largest protocol
module in the library. Needs the default `jennifer` binary. A protocol error or
dropped connection throws `Error{kind: "amqp"}`.

```jennifer
import "amqp.j" as amqp;

def c as amqp.Conn init amqp.connect(amqp.options("localhost", "guest", "guest"));
amqp.declareQueue($c, "jobs", true);
amqp.publishText($c, "", "jobs", "hello");

def m as amqp.Message init amqp.get($c, "jobs", false);
if (not $m.empty) {
    amqp.ack($c, $m.deliveryTag);
}
amqp.close($c);
```

Runnable: [`examples/modules/amqp_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/amqp_demo.j).

## Connecting

`connect` runs the full handshake - protocol header, `Connection.Start` /
`Start-Ok` (SASL **PLAIN** auth), `Tune` / `Tune-Ok` (heartbeats disabled),
`Open` / `Open-Ok`, then `Channel.Open` - and returns a `Conn` on a single
channel.

The dial is bounded by a connection-establishment timeout, so a slow or
unreachable broker fails with a catchable error instead of blocking forever.

```jennifer
def struct amqp.Options { host as string, port as int, user as string, password as string, vhost as string, security as string };
```

`security` is `"none"` (plaintext AMQP, the default) or `"tls"` (AMQPS - TLS on
connect, verifying the broker certificate). `amqp.options(...)` defaults it to
`"none"`; set it to `"tls"` on the returned `Options` for a broker that requires
TLS so credentials do not cross the wire in the clear.

| Call | Returns | |
| ---- | ------- | - |
| `amqp.options(host, user, password)` | `Options` | defaults: port 5672, vhost "/", security "none" |
| `amqp.withPort(o, port)` | `Options` | copy with a different port |
| `amqp.withVhost(o, vhost)` | `Options` | copy with a different virtual host |
| `amqp.connect(opts)` | `Conn` | connect and open a channel |
| `amqp.close(c)` | | `Connection.Close` and shut the socket |

## Queues and publishing

| Call | Returns | |
| ---- | ------- | - |
| `amqp.declareQueue(c, name, durable)` | `QueueInfo` | declare a classic queue (`""` name = server-generated); `durable` survives a restart |
| `amqp.declareQuorumQueue(c, name)` | `QueueInfo` | declare a quorum queue (`x-queue-type=quorum`); always durable, name required |
| `amqp.publish(c, exchange, routingKey, body)` | | publish a `bytes` body |
| `amqp.publishText(c, exchange, routingKey, text)` | | publish a UTF-8 string |

`declareQueue` returns `QueueInfo{name, messageCount, consumerCount}`.
`declareQuorumQueue` declares a **quorum queue** - a Raft-replicated, always-durable
queue type for high availability - by sending the `x-queue-type=quorum` argument.
Quorum queues cannot be server-named, so `name` must be non-empty, and there is no
`durable` flag (they are always durable). Do not also declare the same name as a
classic queue: a re-declare must use the same type.
`publish` sends the method frame, a content-header frame (body size), and a body
frame. Use exchange `""` (the default exchange) to route straight to a queue by
name via `routingKey`.

## Consuming (pull)

`amqp.get(c, queue, autoAck)` pulls the next message with `Basic.Get` - a
**synchronous pull**, not an async delivery loop. Call it in a loop until
`Message.empty` is true; `ack` each message (unless `autoAck`).

```jennifer
def struct amqp.Message {
    empty as bool,        # true when the queue was empty (other fields zero)
    deliveryTag as int,   # pass to ack
    exchange as string,
    routingKey as string,
    body as bytes
};
```

```jennifer
def more as bool init true;
repeat {
    def m as amqp.Message init amqp.get($c, "jobs", false);
    if ($m.empty) {
        $more = false;
    } else {
        # handle $m.body
        amqp.ack($c, $m.deliveryTag);
    }
} until (not $more);
```

| Call | Returns | |
| ---- | ------- | - |
| `amqp.get(c, queue, autoAck)` | `Message` | pull the next message (`empty` true when none) |
| `amqp.ack(c, deliveryTag)` | | acknowledge a delivered message |

## Scope

- **Pull, not push.** Receiving is `Basic.Get` (one message per call);
  streaming `Basic.Consume` with server-pushed `Basic.Deliver` (an async loop)
  is a follow-on.
- **One channel, no publisher confirms / transactions.** A single channel (1)
  is opened; `publish` is fire-and-forget (no `Confirm.Select`).
- **No message properties.** Publishes carry an empty property set (no
  content-type, headers, or persistence flag on the message itself - queue
  durability is set at `declareQueue`).
- **SASL PLAIN only.** TLS (`amqps`) is available via `Options.security =
  "tls"`; without it, use a trusted network or a local broker.
- **The largest protocol module.** If the tree-walker ever becomes the
  bottleneck for high-throughput messaging, this is a candidate to reimplement
  as a Go library.

## See also

- [net.md](../libraries/net.md) - the TCP transport this is built on.
- [mqtt.md](mqtt.md) / [redis.md](redis.md) - the other binary-protocol clients
  over `net`.
- [modules/index.md](index.md) - the module catalog and import rules.
