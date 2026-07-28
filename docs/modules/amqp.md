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
def struct amqp.Options { host as string, port as int, user as string, password as string, vhost as string, security as transport.Security };
```

`security` is a [`transport.Security`](transport.md): `.None` (plaintext AMQP, the
default) or `.Tls` (AMQPS - TLS on connect, verifying the broker certificate);
`.Starttls` is rejected (AMQP has no in-band upgrade). `amqp.options(...)` defaults it to
`.None`; set it to `transport.Security.Tls` on the returned `Options` for a broker that requires
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
| `amqp.publishWith(c, exchange, routingKey, body, props)` | | publish carrying message properties |

`declareQueue` returns `QueueInfo{name, messageCount, consumerCount}`.
`declareQuorumQueue` declares a **quorum queue** - a Raft-replicated, always-durable
queue type for high availability - by sending the `x-queue-type=quorum` argument.
Quorum queues cannot be server-named, so `name` must be non-empty, and there is no
`durable` flag (they are always durable). Do not also declare the same name as a
classic queue: a re-declare must use the same type.
`publish` sends the method frame, a content-header frame (body size), and a body
frame. Use exchange `""` (the default exchange) to route straight to a queue by
name via `routingKey`.

## Exchanges and bindings

Beyond the default exchange, declare a named exchange and bind a queue to it. The
exchange type selects the routing rule: `"direct"` (exact routing-key match),
`"topic"` (pattern match with `*` / `#`), or `"fanout"` (broadcast, routing key
ignored). Declared exchanges are durable.

```jennifer
amqp.declareExchange($c, "events", "topic");
amqp.declareQueue($c, "audit", true);
amqp.bindQueue($c, "audit", "events", "order.#");
amqp.publishText($c, "events", "order.created", "{}");
```

| Call | Returns | |
| ---- | ------- | - |
| `amqp.declareExchange(c, name, exType)` | | declare a durable exchange (`"direct"` / `"topic"` / `"fanout"`) |
| `amqp.bindQueue(c, queue, exchange, routingKey)` | | bind a queue to an exchange with a routing key / pattern |

## Message properties

`publishWith` attaches message properties to a publish. Build a `Properties` and
leave any field unset (`""` or `false`) to omit it from the wire:

```jennifer
def struct amqp.Properties {
    contentType as string,    # MIME type, e.g. "application/json"; "" omits it
    persistent as bool,       # true = delivery-mode 2 (survives a restart on a durable queue)
    correlationId as string,  # request / reply matching; "" omits it
    replyTo as string         # reply-to queue / routing key; "" omits it
};
```

```jennifer
def p as amqp.Properties init amqp.Properties{
    contentType: "application/json", persistent: true, correlationId: "req-42", replyTo: "replies"
};
amqp.publishWith($c, "", "jobs", convert.bytesFromString("{}", "utf-8"), $p);
```

`persistent` is the message-level durability flag (delivery-mode 2); combine it
with a `durable` queue so a message survives a broker restart.

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
| `amqp.nack(c, deliveryTag, requeue)` | | negatively acknowledge (`requeue` true re-queues, false drops / dead-letters) |

`nack` (Basic.Nack) rejects a delivered message: with `requeue` true the broker
puts it back for redelivery, with false it is dropped (or dead-lettered if the
queue is configured for it). It works for messages from both `get` and
`receiveDelivery`.

## Consuming (push)

`amqp.consume(c, queue, autoAck)` starts a **server-pushed** subscription
(`Basic.Consume`) and returns a broker-generated **consumer tag**. The broker then
streams messages; `amqp.receiveDelivery(c)` **blocks** for the next one
(`Basic.Deliver` + content-header + body frames) and returns a `Delivery`. There
is no callback API - the app calls `receiveDelivery` in a loop.

```jennifer
def struct amqp.Delivery {
    consumerTag as string,
    deliveryTag as int,    # pass to ack / nack
    redelivered as bool,   # the broker has delivered this before
    exchange as string,
    routingKey as string,
    body as bytes
};
```

Because `receiveDelivery` blocks, run the loop inside its own `spawn` so pushed
deliveries never stall the rest of the program (this is the cooperative
receive-loop convention - no handler callbacks re-enter the interpreter):

```jennifer
def tag as string init amqp.consume($c, "jobs", false);

def worker as task of null init spawn {
    def running as bool init true;
    while ($running) {
        def d as amqp.Delivery init amqp.receiveDelivery($c);
        # handle $d.body ...
        amqp.ack($c, $d.deliveryTag);
    }
    return;
};

# ... main program does other work; cancel when done ...
amqp.cancelConsume($c, $tag);
```

Use one `Conn` per consuming `spawn`: a `Conn` holds a single socket + channel,
and value semantics copy the handle but share the underlying `net.Conn`, so two
tasks reading the same connection would interleave frames. Open a second
connection for a concurrent consumer.

| Call | Returns | |
| ---- | ------- | - |
| `amqp.consume(c, queue, autoAck)` | `string` | start a subscription; returns the consumer tag |
| `amqp.receiveDelivery(c)` | `Delivery` | block for the next pushed message |
| `amqp.cancelConsume(c, consumerTag)` | | stop a subscription |

## Publisher confirms

`amqp.confirmSelect(c)` puts the channel into **publisher-confirm** mode
(`Confirm.Select`). After that, the broker confirms each publish;
`amqp.waitConfirm(c)` blocks for the next confirmation, returning `true` on
`Basic.Ack` (the broker took responsibility for the message) and `false` on
`Basic.Nack` (it could not). Call `waitConfirm` once per publish.

```jennifer
amqp.confirmSelect($c);
amqp.publishText($c, "", "jobs", "hello");
if (not amqp.waitConfirm($c)) {
    # broker could not accept the message - retry or fail
}
```

| Call | Returns | |
| ---- | ------- | - |
| `amqp.confirmSelect(c)` | | enable publisher confirms on the channel |
| `amqp.waitConfirm(c)` | `bool` | block for the next confirm (`true` ack / `false` nack) |

## Scope

- **Pull and push.** Receiving is either `Basic.Get` (one message per `get`
  call) or `Basic.Consume` + `receiveDelivery` (a server-pushed loop, run inside
  a `spawn`).
- **One channel.** A single channel (1) is opened per connection. For concurrent
  consumers open a second `Conn`.
- **Publisher confirms, no transactions.** `confirmSelect` + `waitConfirm` give
  per-message broker acknowledgement; AMQP transactions (`Tx`) are not
  implemented.
- **Message properties.** `publishWith` carries content-type, persistence
  (delivery-mode 2), correlation-id, and reply-to; other properties (headers,
  priority, timestamp, ...) are not exposed.
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
