# `imap` - receive mail (IMAP client)

Import with `import "imap.j" as imap;`. An **IMAP4rev1** receive client (RFC
3501): tagged commands and untagged `*` responses over the `net` system
library, with plaintext / implicit-TLS / STARTTLS transport and auth by `LOGIN`,
XOAUTH2, CRAM-MD5, or SCRAM-SHA-1 / SCRAM-SHA-256.
A practical subset - not the full protocol, but covering both **reading and
basic folder management**: select a folder, search it with criteria, fetch
whole messages or named headers, set/clear flags, copy or atomically **move**
messages (RFC 6851), create a folder, and delete (mark `\Deleted` + expunge).
It is not read-only. Retrieved messages come back as strings for the
[`mime`](mime.md) module to parse. Because it uses `net`, this module needs the
default **`jennifer`** binary.

> **On `jennifer-tiny`:** "needs the default `jennifer` binary" refers to the
> **stock** tiny build, which ships without a network driver - not a TinyGo
> limitation. A `jennifer-tiny` rebuilt with a network stack runs this module
> too; see the
> [note on `net` and TinyGo](../technical/tinygo.md#net-on-tinygo-is-a-build-choice-not-a-hard-limit).

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

Runnable: [`examples/modules/imap_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/imap_demo.j).

## Surface

A session is stateful: `connect`, `selectFolder`, `search` / `fetch` /
`fetchHeaders`, optional `addFlags` / `expunge`, `logout`. `fetchAll` wraps the
common "read every message in a folder" case. Messages are addressed by their
stable **UID** throughout (see [UIDs](#uids-stable-identifiers) below) - `search`
returns UIDs, and every message verb takes one.

| Call / type                          | Notes                                                            |
| ------------------------------------ | ---------------------------------------------------------------- |
| `imap.Options`                       | `host`, `port`, `security`, `user`, `pass`, `auth`.              |
| `imap.Session`                       | A live session over one connection (from `connect`).             |
| `imap.connect(opts)`                 | Open a session: greeting, optional STARTTLS, `LOGIN` / SASL.     |
| `imap.folders(session, pattern)`     | `LIST` the folders matching `pattern` (`"*"` = all) -> `list of imap.Folder` (`name`, `delimiter`, `flags`). |
| `imap.status(session, folder)`       | `STATUS` counts for a folder without selecting it -> `imap.Status` (`messages`, `recent`, `unseen`, `uidnext`, `uidvalidity`). |
| `imap.selectFolder(session, name)`  | `SELECT` a folder (e.g. `"INBOX"`); returns its message count.  |
| `imap.Criteria`                      | A search filter (all fields optional; zero = match all). See [Searching](#searching). |
| `imap.criteria()`                    | An empty `Criteria` (matches all). Set fields, then pass to `search`. |
| `imap.search(session, criteria)`     | The **UIDs** matching `criteria` (`list of int`, via `UID SEARCH`). `imap.criteria()` = every message. |
| `imap.fetch(session, uid)`           | `UID FETCH uid BODY.PEEK[]` - the message as a raw string, for `mime.parse`. |
| `imap.fetchMessage(session, uid)`    | Fetch the message and parse it into a `mime.Part` tree (import `mime` too, then use `mime.attachments` / `mime.textBodies`). |
| `imap.fetchHeaders(session, uid, flds)`| `UID FETCH uid BODY.PEEK[HEADER.FIELDS (flds)]` - only the named headers (e.g. `"SUBJECT DATE"`), cheaper than the whole body. |
| `imap.fetchPartial(session, uid, offset, length)` | `UID FETCH uid BODY.PEEK[]<offset.length>` - a byte range of the body, to pull a large message in bounded chunks. |
| `imap.flags(session, uid)`           | `UID FETCH uid (FLAGS)` - the flags set on the message as a space-separated string (confirm a `STORE` persisted). |
| `imap.addFlags(session, uid, flags)` | `UID STORE uid +FLAGS.SILENT (flags)` - add keywords / flags, e.g. `"$cl_1"` (Thunderbird tag colour) or `"\Deleted"`. A server that disallows a keyword answers OK but drops it - verify with `flags`. |
| `imap.removeFlags(session, uid, flags)`| `UID STORE uid -FLAGS.SILENT (flags)` - clear keywords / flags (inverse of `addFlags`); removing an unset flag is a no-op. |
| `imap.createFolder(session, name)`  | `CREATE name` - make a folder; errors if it already exists, so `try`/`catch` for a create-if-missing. |
| `imap.copy(session, uid, folder)`   | `UID COPY uid folder` - copy the message into another (existing) folder. A "move" is `copy` + `addFlags(..., "\Deleted")` + `expunge`, or the atomic `move`. |
| `imap.move(session, uid, folder)`    | `UID MOVE uid folder` (RFC 6851) - copy + delete + expunge in one atomic step (no manual `\Deleted` + `EXPUNGE`). |
| `imap.append(session, folder, msg)` | `APPEND` - upload a full RFC 5322 message into `folder` (e.g. save to Sent). |
| `imap.appendWith(session, folder, flags, msg)` | `APPEND` with initial flags, e.g. `"\Seen"` for Sent or `"\Draft"` for a draft. |
| `imap.expunge(session)`              | `EXPUNGE` - permanently remove all `\Deleted` messages in the selected folder. |
| `imap.logout(session)`               | `LOGOUT` and close.                                              |
| `imap.fetchAll(opts, folder)`       | Connect, select, retrieve every message, log out; `list of string`. |
| `imap.Notification`                  | One server push during IDLE: `kind` (`"exists"` / `"expunge"` / `"recent"` / `""`), `number`. |
| `imap.supportsIdle(session)`         | `CAPABILITY` gate - true when the server advertises IDLE (RFC 2177). |
| `imap.idle(session)`                 | Enter IDLE (`IDLE` -> `+ idling`); the server now pushes mailbox changes. |
| `imap.receiveNotification(session)` | Block for the next push -> `imap.Notification` (empty sentinel when IDLE ends). |
| `imap.pollNotification(session, timeoutMs)` | Like `receiveNotification`, but wait at most `timeoutMs` ms (`net.setDeadline`), then the empty sentinel. |
| `imap.done(session)`                 | Leave IDLE (`DONE` + tagged completion), back to command mode.   |

`Options.security` is `"none"` (143), `"tls"` (implicit TLS on connect, 993),
or `"starttls"`. `fetch` uses `BODY.PEEK[]`, so retrieving does **not** set the
`\Seen` flag. An internationalized (IDN) host is IDNA-encoded to its `xn--` form
automatically (via [`idna`](idna.md)).

## Authentication

`connect` authenticates according to `Options.auth`; the non-LOGIN mechanisms go
through the [`sasl`](sasl.md) module:

| `auth`          | Mechanism |
| --------------- | --------- |
| `""` (default)  | Plain `LOGIN` (username + `pass`). |
| `"xoauth2"`     | `AUTHENTICATE XOAUTH2` - pass the OAuth2 **access token** as `pass` (Gmail / Microsoft 365). |
| `"cram"`        | `AUTHENTICATE CRAM-MD5` - challenge-response, no cleartext password. |
| `"scram-sha-1"` / `"scram-sha-256"` | `AUTHENTICATE SCRAM-*` - salted challenge-response (non-initial-response form); the server signature is verified. |
| `"auto"`        | Probe `CAPABILITY` and pick the strongest mechanism offered, falling back to `LOGIN`. |

Use `security: "tls"` / `"starttls"` so `LOGIN` / XOAUTH2 credentials never go
over the wire in the clear.

## Browsing folders

`folders(session, pattern)` runs `LIST` and returns the matching folders as
`imap.Folder` values (`name`, `delimiter`, `flags`); `status(session, folder)`
returns a folder's counts (`messages` / `unseen` / `recent` / `uidnext` /
`uidvalidity`) **without** selecting it - ideal for a folder tree with unread
badges. (IMAP calls these "mailboxes"; the module uses the everyday term
*folder*.)

```jennifer
def s as imap.Session init imap.connect($opts);
for (def f in imap.folders($s, "*")) {          # every folder, at any depth
    def st as imap.Status init imap.status($s, $f.name);
    io.printf("%s (%d unread)\n", $f.name, $st.unseen);
}
```

Use `"%"` instead of `"*"` for the top level only, or a prefix like `Archive/*`
to scope to a subtree. `flags` carries IMAP attributes such as `\HasChildren`
(build a tree from the `delimiter`) and `\Noselect` (a container you cannot
`selectFolder`). Non-ASCII folder names arrive in IMAP's modified-UTF-7 form.

## Searching

`search(session, criteria)` returns the **UIDs** of the messages in the
selected folder that match a `Criteria` (via `UID SEARCH`), so you fetch only the
mail you want instead of pulling the whole folder. Build one from `imap.criteria()` (which
matches everything) and set the fields you need - a zero-value `Criteria` is the
old `SEARCH ALL`.

The fields split by **where they run**:

- **Server-side** - mapped straight to one IMAP `SEARCH` (a single round-trip, no
  message bodies downloaded), and all ANDed together:
  - `subject` / `from` / `to` / `text` - case-insensitive **substring** match
    (`text` covers headers + body).
  - `since` / `before` - an inclusive-since / exclusive-before range as standard
    `time.Time` values (a zero-value time, the default, ignores the bound). Set
    them straight from `time` - `$c.since = time.now()`, or a shifted time for
    "the last week". IMAP `SEARCH` filters by calendar **day**, so a bound at
    midnight is a pure server-side search; a bound that carries a **time-of-day**
    is transparently refined to the exact instant on the client (against each
    candidate's `INTERNALDATE`, the arrival clock `SEARCH` uses), so a sub-day
    range like "since 14:30 today" just works - no extra call.
  - `seen` / `unseen` / `flagged` / `answered` - flag state.
  - `largerThan` / `smallerThan` - size in bytes.
- **Client-side** - applied only to the messages the server-side search returns,
  by fetching just their headers or structure (never full bodies):
  - `subjectRegex` / `fromRegex` - an [RE2](regex.md) pattern matched against the
    decoded `Subject` / `From` header (what IMAP's substring search can't do).
  - `hasAttachments` - keep only messages whose `BODYSTRUCTURE` shows an
    attachment. This is a **heuristic** (it looks for an `attachment`
    content-disposition and downloads no body); an unusual message may fool it.

```jennifer
import "imap.j" as imap;
import "mime.j" as mime;
use time;

def s as imap.Session init imap.connect($opts);
imap.selectFolder($s, "INBOX");

# Unseen invoices from billing since the start of the year, with an attachment.
def c as imap.Criteria init imap.criteria();
$c.from = "billing@";                                        # server-side substring
$c.since = time.fromIso("2026-01-01T00:00:00Z");             # server-side date (a time.Time)
$c.unseen = true;                                            # server-side flag
$c.subjectRegex = "INV-2026-[0-9]+";                         # client-side regex on Subject
$c.hasAttachments = true;                                    # client-side structure check

for (def uid in imap.search($s, $c)) {
    def msg as mime.Part init imap.fetchMessage($s, $uid);   # fetch only the matches
    io.printf("%s\n", mime.headerValue($msg, "Subject"));
}
```

The server-side fields (`from`, `since`, `unseen`) become one `SEARCH`; only
the handful of messages it returns are then fetched to apply the `subjectRegex`
and `hasAttachments` filters, so an inbox of thousands costs one search plus a
few header/structure fetches - not a full download.

Search strings are safe to build from data: substrings are sent as quoted,
control-checked IMAP strings, dates come from `time.Time` values, and sizes are
integers, so no criteria field can inject an IMAP command. An inverted range
(`since` set after `before`) is a catchable `Error` (kind `"imap"`) rather than a
silent empty result; `since == before` is a valid (empty) range.

Not yet covered: non-ASCII search strings over `SEARCH` (use `subjectRegex` /
`fromRegex` meanwhile).

## UIDs: stable identifiers

Every message verb addresses by **UID**, never by a sequence number. A sequence
number is only valid within the current selection - an `EXPUNGE` renumbers every
message after the removed one, so a number captured earlier can silently point at
the wrong message. A UID is stable: it keeps naming the same message across
expunges and across sessions (paired with the folder's `UIDVALIDITY` from
`status`). So there is one addressing scheme, and it is the correct one:

```jennifer
def uids as list of int init imap.search($s, imap.criteria());  # stable UIDs
def body as string init imap.fetch($s, $uids[0]);               # fetch by UID
```

This is the correct basis for "process only what is new since last run": record
the UIDs (and `UIDVALIDITY`) you have handled, then next session `search` and
skip the ones you have already seen - immune to the renumbering a sequence-number
loop would trip over. Sequence numbers appear only where the *server* emits them
as data - the count from `selectFolder`, and the numbers in IDLE `EXISTS` /
`EXPUNGE` pushes - never as an addressing input.

## Managing messages

Reading is only half of it - the module also **modifies** the folder. Given a
UID (from `search`):

**Flag a message** (mark read, tag, star). Flags are a space-separated string;
system flags start with `\`, keywords don't. A server that disallows a keyword
answers `OK` but silently drops it, so read it back with `flags` if it matters:

```jennifer
def uid as int init imap.search($s, imap.criteria())[0];
imap.addFlags($s, $uid, "\\Seen");            # mark read
imap.addFlags($s, $uid, "$cl_1");             # add a keyword/tag (Thunderbird colour label)
imap.removeFlags($s, $uid, "\\Flagged");      # unstar it
io.printf("now: %s\n", imap.flags($s, $uid)); # e.g. "\Seen $cl_1"
```

**Move** a message. The RFC 6851 `MOVE` does copy + delete + expunge atomically:

```jennifer
imap.createFolder($s, "Archive");        # once; errors if it already exists (try/catch)
imap.move($s, $uid, "Archive");          # atomic: no manual \Deleted + expunge
```

`MOVE` is widely but not universally supported; a server lacking it answers
`BAD`, so fall back to the classic copy + `\Deleted` + `expunge` under a
`try`/`catch`:

```jennifer
imap.copy($s, $uid, "Archive");           # copy into the target folder
imap.addFlags($s, $uid, "\\Deleted");     # then mark the original deleted
imap.expunge($s);                         # permanently remove every \Deleted message
```

**Large bodies** can be pulled in ranges with `fetchPartial(session, uid, offset,
length)`, which issues `UID FETCH ... BODY.PEEK[]<offset.length>` so a big
message is retrieved in bounded chunks instead of one huge literal.

**Save a message** into a folder with `append` (e.g. keep a copy in Sent after
sending, or store a draft). The message is a full RFC 5322 string - headers, a
blank line, then the body - built however you like (the `mime` module helps).
`appendWith` sets initial flags:

```jennifer
imap.append($s, "Sent", $rawMessage);              # arrives unflagged
imap.appendWith($s, "Drafts", "\\Draft", $draft);  # marked \Draft
```

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

## IDLE (server push)

Instead of re-polling `STATUS` on a timer, `IDLE` (RFC 2177) lets the server
**push** mailbox changes as they happen - new mail, an expunge, a recent-count
change. The idiom is a cooperative read loop: **idle -> receive / poll -> done**.
There are no callbacks; you read pushes with a blocking `receiveNotification` (or
a timeout-bounded `pollNotification`) and, when you want to stop, break out with
`done`. It is the same M23.1 streaming / server-push shape the other read loops
share, so wrapping the loop in a `spawn` runs push handling beside the rest of
your program.

Each push arrives as an `imap.Notification`: `kind` is `"exists"` (the mailbox
now holds this many messages - new mail), `"expunge"` (the message at this
sequence number was removed), or `"recent"` (recent-count change); `number` is
the count / sequence number it carried. A `kind` of `""` is the **idle-gap
sentinel** - `pollNotification` timed out, or IDLE ended - so it never blocks a
poll loop.

```jennifer
import "imap.j" as imap;

def s as imap.Session init imap.connect($opts);
imap.selectFolder($s, "INBOX");
if (not imap.supportsIdle($s)) {
    io.printf("server has no IDLE - fall back to a STATUS poll\n");
    exit 1;
}

imap.idle($s);                                  # enter IDLE; server now pushes
def n as imap.Notification init imap.receiveNotification($s);   # blocks for the next push
if ($n.kind == "exists") {
    io.printf("new mail: the mailbox now has %d messages\n", $n.number);
}
imap.done($s);                                  # leave IDLE, back to command mode
# Ordinary commands work again. A push carries a sequence number, not a UID, so
# resolve the new mail's stable UID with a search before fetching it.
def uids as list of int init imap.search($s, imap.criteria());
def latest as string init imap.fetch($s, $uids[len($uids) - 1]);
```

`pollNotification($s, timeoutMs)` is the non-blocking variant - it returns the
next push if one arrives within `timeoutMs`, otherwise the empty sentinel, so a
loop can do other work between checks:

```jennifer
imap.idle($s);
while (running()) {
    def n as imap.Notification init imap.pollNotification($s, 1000);   # wait up to 1 s
    if ($n.kind == "exists") {
        handleNewMail($s, $n.number);
    }
    doOtherWork();
}
imap.done($s);
```

A server **drops an idle session after about 29 minutes** (RFC 2177 advises
re-issuing well before that), so a long-lived client must periodically `done`
and `idle` again - break IDLE, then re-enter it - rather than assume the same
IDLE stays live forever. Always pair an `idle` with a `done` before running any
other command: while idling, the connection is dedicated to the push stream.

## Out of scope

A practical subset of IMAP4rev1, not the whole protocol. What it does **not**
cover:

- **Commands.** No `RENAME` / `DELETE` of *folders* (only `CREATE` / `LIST` /
  `STATUS`). `IDLE` is supported (see [IDLE](#idle-server-push)) for the `EXISTS`
  / `EXPUNGE` / `RECENT` pushes (whose numbers are sequence numbers - the one
  place they surface). Message operations address by **UID**; `SEARCH` sends
  ASCII-only criteria (no `CHARSET UTF-8`).
- **Auth.** The supported mechanisms are in [Authentication](#authentication);
  *not* covered are GSSAPI / Kerberos, NTLM, client-certificate auth, and OAuth2
  token **acquisition** / refresh - obtain the access token yourself (e.g. via
  the [`oauth`](oauth.md) module) and pass it as `pass` with `auth: "xoauth2"`.
- **Extensions.** Only the IMAP4rev1 core - no `SORT` / `THREAD`, `CONDSTORE` /
  `QRESYNC`, `COMPRESS`, `METADATA`, quota, or ACL support.

(Message literals are read by byte count, so a raw 8-bit / multi-byte body **is**
returned byte-exact - see [Tagged responses and literals](#tagged-responses-and-literals).)

## Timeouts and limits

Reads carry a 30 s idle timeout (a deadline re-armed before each read), so a hung
server fails with a catchable error instead of blocking the caller forever. The
initial connect (and a STARTTLS handshake) is bounded by its own
connection-establishment timeout, so a slow or unreachable server fails the dial.
A single accumulated response is capped at **64 MiB**: a literal's `{N}` byte
count is attacker-declarable, and a server can also stream untagged lines that
never reach the tagged completion, so either fails with a catchable error rather
than an unbounded allocation.

## See also

- [mime.md](mime.md) - parse a fetched message (`imap.fetchMessage` /
  `mime.parse`) and pull out attachments (`mime.attachments` / `mime.data`) and
  text bodies (`mime.textBodies`).
- [pop.md](pop.md) - the simpler POP3 receive client; [smtp.md](smtp.md) - send.
- [net.md](../libraries/net.md) - the transport `imap` builds on.
- [modules/index.md](index.md) - the module catalog and import rules.
