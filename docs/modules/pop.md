# `pop` - receive mail (POP3 client)

Import with `import "pop.j" as pop;`. A **POP3** receive client (RFC 1939):
the line-oriented status dialogue (`+OK` / `-ERR`) over the `net` system
library, with plaintext / implicit-TLS / STLS transport and auth by `USER` /
`PASS`, APOP, XOAUTH2, CRAM-MD5, or SCRAM-SHA-1 / SCRAM-SHA-256. Retrieved
messages come back as strings, ready for the
[`mime`](mime.md) module to parse. Because it uses `net`, this module needs
the default **`jennifer`** binary.

> **On `jennifer-tiny`:** "needs the default `jennifer` binary" refers to the
> **stock** tiny build, which ships without a network driver - not a TinyGo
> limitation. A `jennifer-tiny` rebuilt with a network stack runs this module
> too; see the
> [note on `net` and TinyGo](../technical/tinygo.md#net-on-tinygo-is-a-build-choice-not-a-hard-limit).

> The module is named `pop` (not `pop3`). It is POP
> version 3 - the only one in use - the same choice Ruby's `net/pop` makes.

```jennifer
use io;
import "pop.j" as pop;
import "mime.j" as mime;
import "transport.j" as transport;

def opts as pop.Options init pop.Options{host: "mail.example.com", port: 995,
    security: transport.Security.Tls, user: "me", pass: "secret"};
for (def raw in pop.fetchAll($opts)) {
    def msg as mime.Part init mime.parse($raw);
    io.printf("subject: %s\n", mime.headerValue($msg, "Subject"));
}
```

Runnable: [`examples/modules/pop_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/pop_demo.j).

## Surface

A session is stateful: `connect`, issue commands, `quit`. `fetchAll` wraps
the common "get every message" case.

| Call / type                       | Notes                                                        |
| --------------------------------- | ------------------------------------------------------------ |
| `pop.Options`                     | `host`, `port`, `security`, `user`, `pass`, `auth`, `allowInsecureAuth`. |
| `pop.Session`                     | A live session over one connection (from `connect`).         |
| `pop.Stat`                        | `count` and total `size`, from `stat`.                       |
| `pop.MessageId`                   | `number` (this session) + `id` (persistent UIDL id), from `uidl`. |
| `pop.MessageSize`                 | `number` (this session) + `size` in octets, from `listSizes`. |
| `pop.connect(opts)`               | Open a session: greet, optional STLS, then auth (`USER` / `PASS` by default). |
| `pop.stat(session)`               | Mailbox `Stat` (`STAT`).                                     |
| `pop.count(session)`              | Just the message count.                                      |
| `pop.listSizes(session)`          | `LIST` - each message's number and octet size -> `list of pop.MessageSize`. |
| `pop.sizes(session)`              | `LIST` sizes alone (`list of int`), in the order listed; once a message is marked deleted, a position is no longer its number - use `listSizes`. |
| `pop.retrieve(session, n)`        | Message `n` as a raw string (`RETR`), for `mime.parse`; a message that is not valid UTF-8 throws. |
| `pop.retrieveBytes(session, n)`   | Message `n` as its exact `bytes` (any 8-bit content).        |
| `pop.uidl(session)`               | `UIDL` - each message's stable id -> `list of pop.MessageId` (`number`, `id`). |
| `pop.uidlOne(session, n)`         | `UIDL n` - one message's stable id (string).                 |
| `pop.top(session, n, lines)`      | `TOP n lines` - the headers plus `lines` body lines (`0` = headers only), for a cheap preview. |
| `pop.topBytes(session, n, lines)` | `top` as exact `bytes`.                                      |
| `pop.deleteMessage(session, n)`   | Mark message `n` for deletion (`DELE`); removed at `quit`.   |
| `pop.reset(session)`              | `RSET` - unmark every message marked for deletion this session. |
| `pop.noop(session)`               | `NOOP` - keep the connection alive (server resets its idle timer). |
| `pop.quit(session)`               | End the session (commit deletions) and close.                |
| `pop.fetchAll(opts)`              | Connect, retrieve every message (no delete), quit; `list of string`. |

`Options.security` is a [`transport.Security`](transport.md): `.None` (plaintext,
port 110), `.Tls` (implicit TLS on connect, port 995), or `.Starttls` (STLS
upgrade on 110). Build it with `import "transport.j" as transport;`.

`USER` / `PASS` and XOAUTH2 send the password or token as it is, so over
`.None` they are **refused** (a `PopError` of kind `"pop"`, before anything is
sent) unless `allowInsecureAuth: true` is set. APOP, CRAM-MD5 and SCRAM never
send the password and need no opt-in.

## Retrieval and dot-stuffing

`retrieve`, `top`, `uidl` and `listSizes` read a **multi-line** response
terminated by a `.` line, and undo the byte-stuffing POP3 applies (a body line
that began with a `.` was sent doubled, e.g. `..sig` on the wire is `.sig` in
the message). Lines end in CRLF; a bare LF or CR inside a line is kept as it
is, so the message comes back byte for byte, without the CRLF that ends its
last line. `retrieve` returns it as a string, and a message that is not valid
UTF-8 (raw 8-bit Latin-1, say) throws a `PopError`; `retrieveBytes` /
`topBytes` return the exact `bytes` of any message:

```jennifer
# $opts as in the first example
def s as pop.Session init pop.connect($opts);
io.printf("%d messages\n", pop.count($s));
def raw as string init pop.retrieve($s, 1); # RFC 5322 message text
pop.deleteMessage($s, 1);                   # optional
pop.quit($s);                               # deletion commits here
```

A `-ERR` from the server throws a catchable `pop.PopError` (kind `"pop3"`) with
the refused `command`, the server's `text` and its RFC 2449 / 3206
`responseCode` (`AUTH`, `IN-USE`, `LOGIN-DELAY`, `SYS/TEMP`, `SYS/PERM`). A local
guard (a control character in an argument, plaintext credentials over `.None`)
is a `PopError` of kind `"pop"`. A reply that does not have the form its
command needs - a bare `+OK` to `STAT`, a `LIST` or `UIDL` line that is not
`<number> <size>` / `<number> <uid>`, a `UIDL n` answer for another message -
is a `PopError` of kind `"pop3"` too, never a default count or a skipped line.
A response over the cap is kind `"limit"`, and a server that closes the
connection before a reply line or a body is complete is kind `"closed"`.

Certificate verification for `.Tls` / `.Starttls` is the `net` default.

## Stable ids: download only what is new

A message `number` is only meaningful within the current session; the `UIDL`
`id` is **persistent** across sessions, so it is the key to leave-on-server /
skip-seen (POP3's `RETR` does not delete, so a mailbox can be polled without
consuming it). Keep the ids you have processed, and next time retrieve only the
`number`s whose `id` is new:

```jennifer
use maps;
def seen as map of string to bool init loadSeen();   # your persisted set
def s as pop.Session init pop.connect($opts);
for (def m in pop.uidl($s)) {
    if (not maps.has($seen, $m.id)) {
        def raw as string init pop.retrieve($s, $m.number);
        # ... process the new message ...
        $seen[$m.id] = true;
    }
}
pop.quit($s);   # no deleteMessage -> the mailbox is left intact
```

`top(session, n, 0)` fetches only the headers of message `n` - a cheap preview
(Subject / From / Date via `mime.parse`) before deciding whether to `retrieve`
the whole body. `reset` unmarks any pending `deleteMessage`s (abort a batch
before `quit` commits it); `noop` keeps an idle connection from timing out.
`UIDL` and `TOP` are widely but not universally supported - a server lacking one
answers `-ERR` (a catchable `"pop3"` error).

## Testing

The pure protocol logic - `+OK` detection, strict `STAT` / `LIST` / `UIDL`
parsing, and the multi-line dot-terminator / un-stuffing - is unit-tested in
the overlay, along with the plaintext-credential refusal, 8-bit bodies and a
server closing mid-reply.
The networked session is covered end to end by an in-process fake POP3 server
in the Go test suite (`TestPop3Receive`), so it runs in CI without an external
server.

## Out of scope

- **Receive only** (retrieve / delete). Sending is [`smtp`](smtp.md).
- **Auth**: `USER` / `PASS` (default), APOP (`auth: "apop"`, RFC 1939 - proves
  the password with MD5 of the greeting's timestamp, never sending it), or SASL
  `AUTH` - XOAUTH2 (`auth: "xoauth2"`, for Google / Microsoft 365), CRAM-MD5
  (`"cram"`), or SCRAM-SHA-1 / SCRAM-SHA-256 (`"scram-sha-1"` / `"scram-sha-256"`),
  via [`sasl`](sasl.md). SCRAM verifies the server signature in the server-final
  continuation; a `+OK` that arrives without one is refused (a `PopError`, kind
  `"pop3"`), since the server never proved it knows the password. `auth: "auto"` picks the strongest the server offers
  (SASL from `CAPA`, then APOP when the greeting carries a timestamp, else USER /
  PASS); `auth: ""` is plain USER / PASS.
- An internationalized (IDN) host is IDNA-encoded to its `xn--` form
  automatically (via [`idna`](idna.md)).

## Timeouts and limits

Reads carry a 30 s idle timeout (a deadline re-armed before each read), so a hung
server fails with a catchable error instead of blocking the caller forever. The
initial connect (and a STARTTLS handshake) is bounded by its own
connection-establishment timeout, so a slow or unreachable server fails the dial.
A single accumulated response is capped at **64 MiB**, so a server that streams a
status line or a dot-terminated body that never ends fails with a catchable error
of kind `"limit"` rather than growing the buffer without bound.

## See also

- [mime.md](mime.md) - parse a retrieved message (`mime.parse`).
- [smtp.md](smtp.md) - the send half of the mail suite.
- [net.md](../libraries/net.md) - the transport `pop` builds on.
- [modules/index.md](index.md) - the module catalog and import rules.

## API reference

Every exported function, struct and constant of `pop`, with its signature, generated
from the module's doc comments: [the `pop` API reference](../api/pop.md).
