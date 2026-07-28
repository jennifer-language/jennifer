# `transport` - shared connection-security mode

Import with `import "transport.j" as transport;`. A single shared enum for the
connection **security mode** used by every network-client module that opens a
socket - `smtp`, `pop`, `imap`, `redis`, `amqp`, `mqtt`. One typed value instead
of a stringly-typed `security` field per module, so a typo is a compile-time
error, not a silent plaintext downgrade. Pure `.j` (just an enum + one helper);
runs on **both** binaries.

```jennifer
import "transport.j" as transport;
import "smtp.j" as smtp;

def o as smtp.Options init smtp.Options{
    host: "smtp.example.com", port: 587,
    security: transport.Security.Starttls,
    clientName: "me", user: "u", pass: "p", auth: "", allowInsecureAuth: false};
```

## The `Security` enum

```jennifer
export def enum transport.Security { None, Tls, Starttls };
```

| Variant | Meaning |
| ------- | ------- |
| `Security.None` | Plaintext - no encryption. |
| `Security.Tls` | Implicit TLS negotiated on connect (SMTPS / POP3S / IMAPS / rediss / AMQPS / mqtts). |
| `Security.Starttls` | Connect in plaintext, then upgrade in-band (SMTP STARTTLS, POP3 STLS, IMAP STARTTLS). |

The **zero value** is `Security.None` (the first variant), so a zero-value
`Options` is plaintext.

Only the protocols with an in-band upgrade command accept `Starttls`: `smtp`,
`pop`, and `imap`. `redis`, `amqp`, and `mqtt` have no such command and **reject**
`Security.Starttls` with a catchable `Error` (their `match` handles the variant
explicitly rather than silently treating it as plaintext) - use `Security.Tls`
for the TLS-on-connect port instead.

## Helper

| Call | Returns | |
| ---- | ------- | - |
| `transport.encrypted(s)` | `bool` | `true` for `Tls` / `Starttls`, `false` for `None` |

`encrypted` is the "is this connection protected?" test - e.g. `smtp` uses it to
refuse sending SASL credentials over a `None` connection unless
`allowInsecureAuth` is set.

## Why a separate module

The security mode is the one field shared verbatim across every socket client, so
it lives in one place rather than duplicated as six sibling enums (the
one-way-per-thing stance). Because Jennifer has no re-export, a program that
builds a client `Options` imports `transport` alongside the client module - the
small cost of a single shared type.

## See also

- [smtp.md](smtp.md) / [pop.md](pop.md) / [imap.md](imap.md) - the mail clients (support all three modes).
- [redis.md](redis.md) / [amqp.md](amqp.md) / [mqtt.md](mqtt.md) - the store / broker clients (None / Tls).
- [net.md](../libraries/net.md) - the TLS transport underneath.
- [modules/index.md](index.md) - the module catalog and import rules.
