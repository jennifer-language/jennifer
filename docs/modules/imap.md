# `imap` - receive mail (IMAP client)

Import with `import "imap.j" as imap;`. An **IMAP4rev1** receive client (RFC
3501): tagged commands and untagged `*` responses over the `net` system
library, with plaintext / implicit-TLS / STARTTLS transport and `LOGIN` auth.
A useful **reading subset** - select a mailbox, search it, fetch whole
messages - not the full protocol. Retrieved messages come back as strings for
the [`mime`](mime.md) module to parse. Because it uses `net`, this module needs
the default **`jennifer`** binary.

```jennifer
import "imap.j" as imap;
import "mime.j" as mime;

def opts as imap.Options init imap.Options{host: "mail.example.com", port: 993,
    security: "tls", user: "me", pass: "secret"};
for (def raw in imap.fetchAll($opts, "INBOX")) {
    def msg as mime.Part init mime.parse($raw);
    io.printf("subject: %s\n", mime.headerValue($msg, "Subject"));
}
```

Runnable: [`examples/modules/imap_demo.j`](https://github.com/mplx/jennifer-lang/blob/main/examples/modules/imap_demo.j).

## Surface

A session is stateful: `connect`, `selectMailbox`, `search` / `fetch`,
`logout`. `fetchAll` wraps the common "read every message in a mailbox" case.

| Call / type                        | Notes                                                            |
| ---------------------------------- | ---------------------------------------------------------------- |
| `imap.Options`                     | `host`, `port`, `security`, `user`, `pass`.                      |
| `imap.Session`                     | A live session over one connection (from `connect`).             |
| `imap.connect(opts)`               | Open a session: greeting, optional STARTTLS, `LOGIN`.            |
| `imap.selectMailbox(session, name)`| `SELECT` a mailbox (e.g. `"INBOX"`); returns its message count.  |
| `imap.search(session)`             | `SEARCH ALL` - the sequence numbers in the selected mailbox (`list of int`). |
| `imap.fetch(session, n)`           | `FETCH n BODY.PEEK[]` - message `n` as a raw string, for `mime.parse`. |
| `imap.logout(session)`             | `LOGOUT` and close.                                              |
| `imap.fetchAll(opts, mailbox)`     | Connect, select, retrieve every message, log out; `list of string`. |

`Options.security` is `"none"` (143), `"tls"` (implicit TLS on connect, 993),
or `"starttls"`. `fetch` uses `BODY.PEEK[]`, so retrieving does **not** set the
`\Seen` flag.

## Tagged responses and literals

Two IMAP mechanics the client handles for you:

- **Tags.** Each command carries a tag and completes with a tagged
  `OK` / `NO` / `BAD` line; a `NO` / `BAD` throws a catchable `Error` (kind
  `"imap"`). The client uses one fixed tag, which is safe here because it is
  synchronous (one command in flight at a time).
- **Literals.** A `FETCH` body arrives as a `{N}` literal - a byte count
  followed by exactly `N` bytes - which the client reads by count rather than
  by line, so a message body containing blank lines or its own `)` is returned
  intact.

Certificate verification for `"tls"` / `"starttls"` is the `net` default.

## Testing

The pure protocol logic - tag detection, literal-length and literal
extraction, `EXISTS` / `SEARCH` parsing, `LOGIN` argument quoting, and tagged
`OK` / `NO` handling - is unit-tested in the overlay. The networked session
(tagged responses **and** literal reading) is covered end to end by an
in-process fake IMAP server in the Go test suite (`TestImapReceive`), so it
runs in CI without an external server.

## Out of scope

This is a reading subset, not full IMAP4rev1:

- **Commands.** `LOGIN` / `SELECT` / `SEARCH ALL` / `FETCH BODY.PEEK[]` /
  `LOGOUT`. No partial fetch, `STORE` (flag changes), `COPY`, `APPEND`,
  `EXPUNGE`, mailbox management, or `IDLE`.
- **Auth.** `LOGIN` only; SASL `AUTHENTICATE` (and `CRAM-MD5` / `SCRAM`) land
  with the `crypto` library.
- **Literals are read as 7-bit / ASCII.** MIME transfer encoding keeps mail
  bodies ASCII; raw 8-bit literals are not yet byte-exact.
- **ASCII host only (for now).** A non-ASCII (IDN) host throws rather than
  failing silently; IDNA support is a planned follow-on (M18.4.5).

## See also

- [mime.md](mime.md) - parse a fetched message (`mime.parse`).
- [pop.md](pop.md) - the simpler POP3 receive client; [smtp.md](smtp.md) - send.
- [net.md](../libraries/net.md) - the transport `imap` builds on.
- [modules/index.md](index.md) - the module catalog and import rules.
