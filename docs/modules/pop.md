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

> The module is named `pop` (not `pop3`): a Jennifer namespace is
> letters-only, so a digit in the name can't be a call prefix. It is POP
> version 3 - the only one in use - the same choice Ruby's `net/pop` makes.

```jennifer
import "pop.j" as pop;
import "mime.j" as mime;

def opts as pop.Options init pop.Options{host: "mail.example.com", port: 995,
    security: "tls", user: "me", pass: "secret"};
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
| `pop.Options`                     | `host`, `port`, `security`, `user`, `pass`.                  |
| `pop.Session`                     | A live session over one connection (from `connect`).         |
| `pop.Stat`                        | `count` and total `size`, from `stat`.                       |
| `pop.MessageId`                   | `number` (this session) + `id` (persistent UIDL id), from `uidl`. |
| `pop.connect(opts)`               | Open a session: greet, optional STLS, `USER` / `PASS`.       |
| `pop.stat(session)`               | Mailbox `Stat` (`STAT`).                                     |
| `pop.count(session)`              | Just the message count.                                      |
| `pop.sizes(session)`              | `list of int` - each message's octet size, in order (`LIST`). |
| `pop.retrieve(session, n)`        | Message `n` as a raw string (`RETR`), for `mime.parse`.      |
| `pop.uidl(session)`               | `UIDL` - each message's stable id -> `list of pop.MessageId` (`number`, `id`). |
| `pop.uidlOne(session, n)`         | `UIDL n` - one message's stable id (string).                 |
| `pop.top(session, n, lines)`      | `TOP n lines` - the headers plus `lines` body lines (`0` = headers only), for a cheap preview. |
| `pop.deleteMessage(session, n)`   | Mark message `n` for deletion (`DELE`); removed at `quit`.   |
| `pop.reset(session)`              | `RSET` - unmark every message marked for deletion this session. |
| `pop.noop(session)`               | `NOOP` - keep the connection alive (server resets its idle timer). |
| `pop.quit(session)`               | End the session (commit deletions) and close.                |
| `pop.fetchAll(opts)`              | Connect, retrieve every message (no delete), quit; `list of string`. |

`Options.security` is `"none"` (plaintext, port 110), `"tls"` (implicit TLS
on connect, port 995), or `"starttls"` (STLS upgrade on 110).

## Retrieval and dot-stuffing

`retrieve` and `sizes` read a **multi-line** response terminated by a `.`
line, and undo the byte-stuffing POP3 applies (a body line that began with a
`.` was sent doubled, e.g. `..sig` on the wire is `.sig` in the message), so
the string you get back is the exact message:

```jennifer
def s as pop.Session init pop.connect($opts);
io.printf("%d messages\n", pop.count($s));
def raw as string init pop.retrieve($s, 1);      # RFC 5322 message text
pop.deleteMessage($s, 1);                          # optional
pop.quit($s);                                      # deletion commits here
```

A `-ERR` from the server throws a catchable `Error` (kind `"pop3"`).

Certificate verification for `"tls"` / `"starttls"` is the `net` default.

## Stable ids: download only what is new

A message `number` is only meaningful within the current session; the `UIDL`
`id` is **persistent** across sessions, so it is the key to leave-on-server /
skip-seen (POP3's `RETR` does not delete, so a mailbox can be polled without
consuming it). Keep the ids you have processed, and next time retrieve only the
`number`s whose `id` is new:

```jennifer
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

The pure protocol logic - `+OK` detection, `STAT` parsing, `LIST` sizes, and
the multi-line dot-terminator / un-stuffing - is unit-tested in the overlay.
The networked session is covered end to end by an in-process fake POP3 server
in the Go test suite (`TestPop3Receive`), so it runs in CI without an external
server.

## Out of scope

- **Receive only** (retrieve / delete). Sending is [`smtp`](smtp.md).
- **Auth**: `USER` / `PASS` (default), APOP (`auth: "apop"`, RFC 1939 - proves
  the password with MD5 of the greeting's timestamp, never sending it), or SASL
  `AUTH` - XOAUTH2 (`auth: "xoauth2"`, for Google / Microsoft 365), CRAM-MD5
  (`"cram"`), or SCRAM-SHA-1 / SCRAM-SHA-256 (`"scram-sha-1"` / `"scram-sha-256"`),
  via [`sasl`](sasl.md). `auth: "auto"` picks the strongest the server offers
  (SASL from `CAPA`, then APOP when the greeting carries a timestamp, else USER /
  PASS); `auth: ""` is plain USER / PASS.
- **No `TOP` / `UIDL`.** Just `STAT` / `LIST` / `RETR` / `DELE`.
- An internationalized (IDN) host is IDNA-encoded to its `xn--` form
  automatically (via [`idna`](idna.md)).

## Timeouts and limits

Reads carry a 30 s idle timeout (a deadline re-armed before each read), so a hung
server fails with a catchable error instead of blocking the caller forever. The
initial connect (and a STARTTLS handshake) is bounded by its own
connection-establishment timeout, so a slow or unreachable server fails the dial.
A single accumulated response is capped at **64 MiB**, so a server that streams a
status line or a dot-terminated body that never ends fails with a catchable error
rather than growing the buffer without bound.

## See also

- [mime.md](mime.md) - parse a retrieved message (`mime.parse`).
- [smtp.md](smtp.md) - the send half of the mail suite.
- [net.md](../libraries/net.md) - the transport `pop` builds on.
- [modules/index.md](index.md) - the module catalog and import rules.
