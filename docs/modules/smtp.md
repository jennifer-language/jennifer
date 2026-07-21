# `smtp` - send mail (SMTP client)

Import with `import "smtp.j" as smtp;`. An SMTP **send** client: the
line-oriented command/response dialogue of RFC 5321 over the `net` system
library, with plaintext / implicit-TLS / STARTTLS transport and SASL `AUTH`
(PLAIN / LOGIN / XOAUTH2 / CRAM-MD5 / SCRAM-SHA-1 / SCRAM-SHA-256). The message
body is any string, typically built by the
[`mime`](mime.md) module. Because it uses `net`, this module needs the
default **`jennifer`** binary; on the stock `jennifer-tiny` a send raises a
friendly error.

> **On `jennifer-tiny`:** "needs the default `jennifer` binary" refers to the
> **stock** tiny build, which ships without a network driver - not a TinyGo
> limitation. A `jennifer-tiny` rebuilt with a network stack runs this module
> too; see the
> [note on `net` and TinyGo](../technical/tinygo.md#net-on-tinygo-is-a-build-choice-not-a-hard-limit).

```jennifer
import "smtp.j" as smtp;
import "mime.j" as mime;

def msg as mime.Part init mime.text("text/plain", "Hello!");
$msg = mime.withHeader($msg, "Subject", "Hi");

def opts as smtp.Options init smtp.Options{host: "mail.example.com", port: 587,
    security: "starttls", clientName: "me.example.com",
    user: "me@example.com", pass: "secret"};
smtp.send($opts, "me@example.com", ["you@example.com"], mime.encode($msg));
```

Runnable: [`examples/modules/smtp_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/smtp_demo.j).

## Surface

| Call / type                              | Notes                                                              |
| ---------------------------------------- | ------------------------------------------------------------------ |
| `smtp.Options`                           | `host`, `port`, `security`, `clientName`, `user`, `pass`, `auth`, `allowInsecureAuth`. |
| `smtp.send(opts, from, recipients, message)` | Deliver `message` to every recipient; throws on a server rejection. |

`Options` fields:

| Field        | Notes                                                                  |
| ------------ | ---------------------------------------------------------------------- |
| `host`       | Server hostname.                                                       |
| `port`       | Server port (25 / 587 plaintext or STARTTLS, 465 implicit TLS).        |
| `security`   | `"none"` (plaintext), `"starttls"` (upgrade after EHLO), `"tls"` (implicit TLS on connect). |
| `clientName` | The `EHLO` identity; defaults to `"localhost"` when empty.             |
| `auth`       | SASL mechanism: `""` (none when `user` is empty, else PLAIN), `"auto"` (negotiate the strongest mechanism `EHLO` advertises, falling back to PLAIN), `"plain"`, `"login"`, `"xoauth2"`, `"cram"` (CRAM-MD5), `"scram-sha-1"`, or `"scram-sha-256"`. |
| `user`       | SASL username; `""` with `auth: ""` skips authentication.              |
| `pass`       | SASL password.                                                         |
| `allowInsecureAuth` | Force SASL `AUTH` over an unencrypted (`security: "none"`) connection. Default `false`: credentials are **refused** over plaintext (use `"tls"` / `"starttls"`, or set this to force). |

## What `send` does

One call runs the whole delivery, throwing a catchable `Error` (kind
`"smtp"`) the moment the server rejects a step:

1. Connect per `security` (`net.connect`, or `net.connectTLS` for `"tls"`),
   bounded by a connection-establishment timeout.
2. Read the `220` greeting, send `EHLO`.
3. For `"starttls"`: confirm the `EHLO` response actually advertised `STARTTLS`
   (an anti-downgrade check - a MITM that strips the capability to keep the
   session in plaintext is refused), then `STARTTLS`, `net.startTLS`, and a
   second `EHLO`.
4. Authenticate per `auth` (via the [`sasl`](sasl.md) mechanisms): `AUTH PLAIN`,
   the `AUTH LOGIN` two-step, `AUTH XOAUTH2` (an OAuth2 bearer token in `pass` -
   how Google / Microsoft 365 authenticate), `AUTH CRAM-MD5`, or the SCRAM
   challenge-response (`AUTH SCRAM-SHA-1` / `SCRAM-SHA-256`, whose server
   signature the client verifies).
5. `MAIL FROM:<from>`, one `RCPT TO:<r>` per recipient, `DATA`.
6. Send the message (CRLF-normalised and dot-stuffed) ended by `<CRLF>.<CRLF>`.
7. `QUIT` and close.

The `from` / `recipients` are the **envelope** (who the server routes to),
separate from the `From:` / `To:` header lines in the message - set both. Each
envelope address is validated before the connection opens: control characters
and CRLF (command injection) are rejected, and the address must be a well-formed
`local@domain` per RFC 5321 (a missing `@`, an empty local part or domain, or a
length over 254 throws).

When `auth` runs over an unencrypted (`security: "none"`) connection, `send`
**refuses** to hand SASL credentials to the server unless
`allowInsecureAuth: true` is set - so a password does not cross the wire in the
clear by accident. `"tls"` and a completed `"starttls"` upgrade are encrypted
and unaffected.

Certificate verification for `"tls"` / `"starttls"` is the `net` default (on;
see [net.md](../libraries/net.md) for the opt-out).

## Errors

A rejection at any step throws `Error{kind: "smtp", message: "..."}` carrying
the step and the server's reply, so wrap untrusted sends in `try` / `catch`:

```jennifer
try {
    smtp.send($opts, $from, $rcpts, $wire);
} catch (e) {
    io.printf("send failed: %s\n", $e.message);
}
```

A connection failure (host down, port blocked) surfaces as the underlying
`net` error through the same path.

## Testing

The pure protocol logic - reply-code parsing (including multi-line `250-`
continuations), `AUTH PLAIN` base64, and dot-stuffing - is unit-tested in the
overlay. The networked `send` path is covered end to end by an in-process
fake SMTP server in the Go test suite (so it runs in CI without an external
server); a live send against a real daemon is the demo's job.

## Out of scope

- **Send only.** Receiving is POP3 / IMAP (later sub-milestones); this module
  does not fetch mail.
- **Auth mechanisms**: PLAIN, LOGIN, XOAUTH2, CRAM-MD5, and SCRAM-SHA-1 /
  SCRAM-SHA-256 (via [`sasl`](sasl.md)). `auth: "auto"` selects the strongest
  the server's `EHLO` advertises. Server-side SASLprep is not applied.
- **No connection reuse / pipelining.** `send` opens, delivers, and closes one
  connection per call.
- **Non-ASCII local parts only.** An internationalized **domain** in the host
  or an envelope address (`user@münchen.de`) is IDNA-encoded to its `xn--`
  form automatically (via [`idna`](idna.md)). A non-ASCII **local part**
  (before the `@`) still **throws** - it needs SMTPUTF8 (RFC 6531), a later
  step - rather than sending a misrouted address.

## Timeouts

Reads carry a 30 s idle timeout (a deadline re-armed before each read), so a hung
server fails with a catchable error instead of blocking the caller forever. The
initial connect and the STARTTLS handshake are bounded by their own
connection-establishment timeout, so a slow or unreachable server fails the dial
rather than blocking it indefinitely.

## See also

- [mime.md](mime.md) - build the `message` (headers, multipart, encodings).
- [net.md](../libraries/net.md) - the transport (`connect` / `connectTLS` /
  `startTLS`) and TLS options `smtp` builds on.
- [modules/index.md](index.md) - the module catalog and import rules.
