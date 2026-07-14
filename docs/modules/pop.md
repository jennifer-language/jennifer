# `pop` - receive mail (POP3 client)

Import with `import "pop.j" as pop;`. A **POP3** receive client (RFC 1939):
the line-oriented status dialogue (`+OK` / `-ERR`) over the `net` system
library, with plaintext / implicit-TLS / STLS transport and `USER` / `PASS`
auth. Retrieved messages come back as strings, ready for the
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

Runnable: [`examples/modules/pop_demo.j`](https://github.com/mplx/jennifer-lang/blob/main/examples/modules/pop_demo.j).

## Surface

A session is stateful: `connect`, issue commands, `quit`. `fetchAll` wraps
the common "get every message" case.

| Call / type                       | Notes                                                        |
| --------------------------------- | ------------------------------------------------------------ |
| `pop.Options`                     | `host`, `port`, `security`, `user`, `pass`.                  |
| `pop.Session`                     | A live session over one connection (from `connect`).         |
| `pop.Stat`                        | `count` and total `size`, from `stat`.                       |
| `pop.connect(opts)`               | Open a session: greet, optional STLS, `USER` / `PASS`.       |
| `pop.stat(session)`               | Mailbox `Stat` (`STAT`).                                     |
| `pop.count(session)`              | Just the message count.                                      |
| `pop.sizes(session)`              | `list of int` - each message's octet size, in order (`LIST`). |
| `pop.retrieve(session, n)`        | Message `n` as a raw string (`RETR`), for `mime.parse`.      |
| `pop.deleteMessage(session, n)`   | Mark message `n` for deletion (`DELE`); removed at `quit`.   |
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

## Testing

The pure protocol logic - `+OK` detection, `STAT` parsing, `LIST` sizes, and
the multi-line dot-terminator / un-stuffing - is unit-tested in the overlay.
The networked session is covered end to end by an in-process fake POP3 server
in the Go test suite (`TestPop3Receive`), so it runs in CI without an external
server.

## Out of scope

- **Receive only** (retrieve / delete). Sending is [`smtp`](smtp.md).
- **`USER` / `PASS`, or XOAUTH2** (`Options.auth = "xoauth2"`, via
  [`sasl`](sasl.md), for Google / Microsoft 365). `APOP` (MD5 challenge) and
  the SASL challenge-response mechanisms land with the `crypto` library.
- **No `TOP` / `UIDL`.** Just `STAT` / `LIST` / `RETR` / `DELE`.
- An internationalized (IDN) host is IDNA-encoded to its `xn--` form
  automatically (via [`idna`](idna.md)).

## Timeouts

Reads carry a 30 s idle timeout (a deadline re-armed before each read), so a hung
server fails with a catchable error instead of blocking the caller forever.

## See also

- [mime.md](mime.md) - parse a retrieved message (`mime.parse`).
- [smtp.md](smtp.md) - the send half of the mail suite.
- [net.md](../libraries/net.md) - the transport `pop` builds on.
- [modules/index.md](index.md) - the module catalog and import rules.
