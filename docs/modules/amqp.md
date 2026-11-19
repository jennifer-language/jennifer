# `amqp` - AMQP 0-9-1 client (RabbitMQ)

Import with `import "amqp.j" as amqp;`. A client for RabbitMQ and compatible
AMQP 0-9-1 brokers over [`net`](../libraries/net.md): connect, declare a queue,
publish messages, and pull them back. The binary frame and method encoding is
built by hand from `bytes` and the bitwise operators. Needs the default
`jennifer` binary. A protocol error or a broker error throws an
`amqp.AmqpError` (kind `"amqp"`); a broker error (a
`Connection.Close` or `Channel.Close`, e.g. a refused login or no such queue)
carries its reply code and text in the message,
`amqp: broker closed the channel: 404 NOT_FOUND - no queue 'jobs'`, and as
fields: `scope` (`"channel"` / `"connection"`), `replyCode`, `replyText`, and
the `classId` / `methodId` of the method it refused (all empty or 0 for other
failures). A mandatory message the broker returned is an `AmqpError` with
`scope` `"return"` (see [Mandatory publishing](#mandatory-publishing)). A socket failure (refused, reset) is a `net.NetError` (kind
`"net"`), a stall kind `"timeout"`, and a connection the broker drops
mid-frame kind `"closed"` (`connection closed after N of M bytes`). After a
broker `Close` the connection is unusable; open a new one.

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

**Bad credentials.** `Start-Ok` advertises the `authentication_failure_close`
client capability, so RabbitMQ answers a wrong user or password with
`Connection.Close` 403 ACCESS_REFUSED, and `connect` throws it as an
`AmqpError` with `scope` `"connection"` and `replyCode` 403
(`amqp: broker closed the connection: 403 ACCESS_REFUSED - Login was refused
using authentication mechanism PLAIN. ...`). A user without access to the
virtual host gets the same 403 at `Connection.Open`. A broker that does not
support the capability just drops the socket after `Start-Ok`, which is kind
`"closed"` (`connection closed after 0 of 1 bytes`).

```jennifer
try {
    def c as amqp.Conn init amqp.connect(amqp.options("localhost", "app", $password));
    # ... use $c, then amqp.close($c);
} catch (e) {
    if ($e.kind == "amqp" and $e.replyCode == 403) {
        io.printf("login refused\n");
    } else {
        throw $e;
    }
}
```

The dial has a 30 s timeout, and every read has a 30 s idle timeout (a
`timeout` error) except `receiveDelivery`'s wait for the next push, so a broker
that stalls mid-reply fails instead of hanging the program. Queue, exchange,
routing-key and consumer-tag names are AMQP short strings: over 255 bytes is an
error, raised before anything is sent.

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
| `amqp.options(host, user, password)` | `Options` | defaults: port 5672, vhost "/", security `transport.Security.None` |
| `amqp.withPort(o, port)` | `Options` | copy with a different port |
| `amqp.withVhost(o, vhost)` | `Options` | copy with a different virtual host |
| `amqp.connect(opts)` | `Conn` | connect and open a channel |
| `amqp.close(c)` | | `Connection.Close` and shut the socket |

`close` reads up to the broker's `Close-Ok`, discarding what the broker sent
before it saw the `Close`: publisher confirms nobody waited for, returned
messages, and deliveries to a consumer still active (unacked ones go back to
their queue). Any other method in their place is still a protocol error.

## Queues and publishing

| Call | Returns | |
| ---- | ------- | - |
| `amqp.declareQueue(c, name, durable)` | `QueueInfo` | declare a classic queue (`""` name = server-generated); `durable` survives a restart |
| `amqp.declareQuorumQueue(c, name)` | `QueueInfo` | declare a quorum queue (`x-queue-type=quorum`); always durable, name required |
| `amqp.publish(c, exchange, routingKey, body)` | | publish a `bytes` body |
| `amqp.publishText(c, exchange, routingKey, text)` | | publish a UTF-8 string |
| `amqp.publishWith(c, exchange, routingKey, body, props)` | | publish carrying message properties |
| `amqp.publishMandatory(c, exchange, routingKey, body, props)` | | publish with the mandatory flag: an unroutable message comes back (use in confirm mode) |

`declareQueue` returns `QueueInfo{name, messageCount, consumerCount}`.
`declareQuorumQueue` declares a **quorum queue** (Raft-replicated, always
durable, for high availability) by sending the `x-queue-type=quorum` argument.
Quorum queues cannot be server-named, so `name` must be non-empty, and there is no
`durable` flag. Do not also declare the same name as a
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
amqp.publishWith($c, "", "jobs", convert.bytesFromString("utf-8", "{}"), $p);
```

`persistent` is the message-level durability flag (delivery-mode 2); combine it
with a `durable` queue so a message survives a broker restart.

## Consuming (pull)

`amqp.get(c, queue, autoAck)` pulls the next message with `Basic.Get`
(a synchronous pull). Call it in a loop until
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

**Prefetch.** Without a limit the broker pushes every ready message to the
consumer at once: one consumer takes the whole queue, other workers on the same
queue get nothing, and a consumer that crashes holds every message unacked until
its connection drops. `amqp.qos(c, prefetchCount)` (`Basic.Qos`) caps how many
unacknowledged deliveries the broker sends each consumer started on the channel
afterwards, so call it before `consume`. With `amqp.qos($c, 1)` the broker
sends the next message only after the previous one is acked or nacked; a
larger count trades fairness for throughput. `0` removes the limit, and the
count is 0 to 65535 (anything else is an `AmqpError` before anything is sent).
It has no effect with `autoAck` true (nothing stays unacked) or on `get`.

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

Because `receiveDelivery` blocks, run the loop inside its own `spawn` so it
does not stall the rest of the program:

```jennifer
def worker as task of null init spawn {
    amqp.qos($c, 1); # one unacked message at a time
    def tag as string init amqp.consume($c, "jobs", false);
    def running as bool init true;
    while ($running) {
        def d as amqp.Delivery init amqp.receiveDelivery($c);
        # handle $d.body ...; set $running = false to stop
        amqp.ack($c, $d.deliveryTag);
    }
    amqp.cancelConsume($c, $tag);
    return;
};
```

While a consumer is active, only the task reading its deliveries may use the
`Conn`, and only for `receiveDelivery`, `ack`, `nack` and `cancelConsume`. A
`Conn` is one socket and one channel; value semantics copy the handle but share
the `net.Conn`, so a second task reading it would take frames meant for the
first, and a delivery arriving where a reply was expected is a protocol error.
Open a second `Conn` for publishing or for another consumer. A delivery already
in flight when `cancelConsume` is sent can arrive before `Cancel-Ok`, which is
also a protocol error; with `autoAck` false the broker redelivers it later.

| Call | Returns | |
| ---- | ------- | - |
| `amqp.qos(c, prefetchCount)` | | limit unacked deliveries per consumer (call before `consume`; `0` = no limit) |
| `amqp.consume(c, queue, autoAck)` | `string` | start a subscription; returns the consumer tag |
| `amqp.receiveDelivery(c)` | `Delivery` | block for the next pushed message |
| `amqp.cancelConsume(c, consumerTag)` | | stop a subscription |

## Publisher confirms

`amqp.confirmSelect(c)` puts the channel into **publisher-confirm** mode
(`Confirm.Select`). After that, the broker confirms each publish;
`amqp.waitConfirm(c)` blocks for the next confirmation, returning `true` on
`Basic.Ack` (the broker took responsibility for the message) and `false` on
`Basic.Nack` (it could not). Call `waitConfirm` once per publish: each call
reports the oldest publish not yet reported, so publishing a batch and then
calling `waitConfirm` once for each message works as well as alternating them.

```jennifer
amqp.confirmSelect($c);
amqp.publishText($c, "", "jobs", "hello");
if (not amqp.waitConfirm($c)) {
    # broker could not accept the message - retry or fail
}
```

The broker may confirm several publishes with one `Basic.Ack` or `Basic.Nack`
(its `multiple` flag: every publish up to that delivery tag). The `Conn` counts
publishes and confirmed delivery tags, in state every copy of it shares, so
`waitConfirm` still returns one result per publish, and a nack covering a
publish is reported as `false` for exactly that publish. A confirm for a tag
never published or already confirmed is a protocol error, as is `waitConfirm`
before `confirmSelect` or with no publish outstanding.

**A skipped `waitConfirm`** leaves its confirm unread on the connection. The
next `waitConfirm` reports that older publish, not the latest one, and any other
call that reads a reply (`declareQueue`, `get`, `consume`, ...) fails with
`amqp: expected Queue.Declare-Ok, got class 60 method 80 (a pending publisher
confirm: call waitConfirm once per publish)`. Only `close` discards unread
confirms.

### Mandatory publishing

A message that no binding matches is dropped by the broker, and in confirm
mode it is still confirmed (`waitConfirm` returns `true`): the broker did take
responsibility for it. `amqp.publishMandatory(c, exchange, routingKey, body,
props)` sets the `mandatory` flag, and the broker returns such a message
(`Basic.Return`) instead of dropping it. The return arrives before the
publish's confirm, and is matched to the oldest unconfirmed `publishMandatory`
with the same exchange and routing key. The `waitConfirm` call for that publish
throws an `AmqpError` with
`scope` `"return"`, `replyCode` 312 (NO_ROUTE), `replyText`, and `classId` 60 /
`methodId` 40 (`Basic.Publish`). The channel stays usable.

```jennifer
def none as amqp.Properties;
amqp.confirmSelect($c);
amqp.publishMandatory($c, "events", "order.created", $body, $none);
try {
    amqp.waitConfirm($c);
} catch (e) {
    if ($e.kind == "amqp" and $e.scope == "return") {
        # no queue is bound for order.created ($e.replyCode 312)
    } else {
        throw $e;
    }
}
```

Use `publishMandatory` in confirm mode only. Without confirms nothing waits for
the return, and the next call that reads a reply fails on it (`... got class 60
method 50 (a returned mandatory message: ...)`).

| Call | Returns | |
| ---- | ------- | - |
| `amqp.confirmSelect(c)` | | enable publisher confirms on the channel |
| `amqp.waitConfirm(c)` | `bool` | block for the next confirm (`true` ack / `false` nack); throws a returned mandatory message |
| `amqp.publishMandatory(c, exchange, routingKey, body, props)` | | publish that the broker returns when no queue takes it |

## Scope

- **Pull and push.** Receiving is either `Basic.Get` (one message per `get`
  call) or `Basic.Consume` + `receiveDelivery` (a server-pushed loop, run inside
  a `spawn`).
- **One channel.** A single channel (1) is opened per connection. For concurrent
  consumers open a second `Conn`.
- **Publisher confirms, no transactions.** `confirmSelect` + `waitConfirm` give
  per-message broker acknowledgement, and `publishMandatory` surfaces
  unroutable messages; AMQP transactions (`Tx`) and the `immediate` flag are
  not implemented.
- **Prefetch per consumer.** `qos` sets the prefetch count with `global`
  false; a byte limit (`prefetch-size`) is not exposed.
- **Message properties.** `publishWith` carries content-type, persistence
  (delivery-mode 2), correlation-id, and reply-to; other properties (headers,
  priority, timestamp, ...) are not exposed.
- **SASL PLAIN only.** TLS (`amqps`) is available via `Options.security =
  transport.Security.Tls`; without it, use a trusted network or a local broker.
- **The largest protocol module.** If the tree-walker ever becomes the
  bottleneck for high-throughput messaging, this is a candidate to reimplement
  as a Go library.

## See also

- [net.md](../libraries/net.md) - the TCP transport this is built on.
- [mqtt.md](mqtt.md) / [redis.md](redis.md) - the other binary-protocol clients
  over `net`.
- [modules/index.md](index.md) - the module catalog and import rules.
