# `mime` - build and parse MIME messages

Import with `import "mime.j" as mime;`. Builds and parses MIME messages (RFC
5322 headers plus RFC 2045/2046 bodies) - the header-and-boundary structure
behind email. Pure Jennifer over `strings`, `convert`, and `encoding`; it
does **no networking**, so it runs on either binary and is the
message-structure foundation the `mail` protocol clients (SMTP / POP3 /
IMAP) build on.

```jennifer
use io;
import "mime.j" as mime;

def msg as mime.Part init mime.text("text/plain", "Hello, café.");
$msg = mime.withHeader($msg, "Subject", "Hi");
io.printf("%s", mime.encode($msg));

def back as mime.Part init mime.parse(mime.encode($msg));
io.printf("%s\n", mime.body($back));      # Hello, café.
```

Runnable: [`examples/modules/mime_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/mime_demo.j).

## The `Part` model

A message is a `Part` tree. A part is either a **leaf** (headers plus decoded
content) or a **multipart container** (headers plus child `parts` under a
`boundary`):

```jennifer
def struct Part {
    headers as list of Header, body as string, encoding as string,
    parts as list of Part, boundary as string, data as bytes
};
def struct Header { name as string, value as string };
```

Every leaf carries its raw decoded content as `data` (`bytes`), so **binary
attachments round-trip intact**. `body` is the text view: `parse` fills it for
text parts (`text/...`), decoding from the Content-Type `charset` (UTF-8 by
default; `iso-8859-*` / `windows-*` codepages are honoured, with a UTF-8
fallback for a charset the `encoding` library does not know), and leaves it `""`
for binary; `data` always holds the exact bytes. A text part whose bytes are not
valid in its declared charset is malformed (`parse` throws kind `"mime"`); a
charset with no codec reads as UTF-8 when the bytes are UTF-8, else `body` is
`""` and only `data` holds the content. `encode` applies the transfer
encoding and `parse` removes it, so you always read plain content.

## Surface

| Call                                    | Returns  | Notes                                                              |
| --------------------------------------- | -------- | ------------------------------------------------------------------ |
| `mime.text(contentType, body)`          | `Part`   | Leaf text part; 7bit if ASCII, else quoted-printable. Adds `charset=utf-8`. |
| `mime.attachment(filename, contentType, body)` | `Part` | Base64 leaf with a `Content-Disposition` filename (text `body`); a non-ASCII name is written RFC 2231 `filename*=`. |
| `mime.attachmentBytes(filename, contentType, data)` | `Part` | Base64 attachment from raw `bytes` (images, PDFs, any binary); non-ASCII name as `filename*=`. |
| `mime.multipart(subtype, boundary, parts)` | `Part` | Container (`multipart/subtype`) over one boundary.                 |
| `mime.withHeader(part, name, value)`    | `Part`   | Copy with a header set (case-insensitive replace, else append).    |
| `mime.encode(part)`                     | `string` | Serialize to a CRLF MIME message with transfer encodings applied.  |
| `mime.parse(text)`                      | `Part`   | Parse a message: unfold headers, split multipart, transfer-decode. |
| `mime.headerValue(part, name)`          | `string` | Header value (case-insensitive) or `""`.                           |
| `mime.body(part)`                       | `string` | A leaf's decoded text body (`""` for a binary part).               |
| `mime.data(part)`                       | `bytes`  | A leaf's raw decoded content (any type, incl. binary).             |
| `mime.parts(part)`                      | `list of Part` | A container's direct child parts.                           |
| `mime.contentType(part)`                | `string` | Media type without parameters (e.g. `text/plain`).                 |
| `mime.disposition(part)`                | `string` | `"attachment"` / `"inline"` / `""` (Content-Disposition).          |
| `mime.filename(part)`                   | `string` | `filename` (else Content-Type `name`), RFC 2231 (`filename*=`, continued) + RFC 2047 decoded, or `""`. |
| `mime.isAttachment(part)`               | `bool`   | Disposition is `attachment`, or a filename is set.                 |
| `mime.walk(part)`                       | `list of Part` | Every leaf part in the subtree, depth-first.                 |
| `mime.attachments(part)`                | `list of Part` | The attachment leaves (see `isAttachment`).                 |
| `mime.textBodies(part)`                 | `list of Part` | The `text/...` leaves that are not attachments (plain + html). |
| `mime.findParts(part, mediaType)`       | `list of Part` | Leaves whose media type equals `mediaType` (e.g. `text/html`). |
| `mime.address(name, email)`             | `string` | RFC 5322 mailbox: `email`, or `Name <email>` (name quoted, or RFC 2047-encoded when non-ASCII). |
| `mime.encodeWord(text)`                 | `string` | RFC 2047 UTF-8 base64 encoded-word(s), `=?UTF-8?B?...?=`, folded when long. |
| `mime.decodeWord(value)`                | `string` | Decode every encoded-word in a header value back to text (B and Q). |

## Encoding

`encode` produces a canonical message with **CRLF** line endings and picks the
transfer encoding automatically:

- **7bit** - `text` bodies that are pure ASCII pass through unencoded.
- **quoted-printable** - `text` bodies with non-ASCII are QP-encoded (e.g.
  `café` to `caf=C3=A9`).
- **base64** - `attachment` bodies are base64-encoded and folded at 76
  columns.

A multipart container writes its child parts between `--boundary` delimiters
and closes with `--boundary--`; nesting works (a part can itself be a
multipart).

```jennifer
use io;
import "mime.j" as mime;

def parts as list of mime.Part init [];
$parts[] = mime.text("text/plain", "plain");
$parts[] = mime.text("text/html", "<b>rich</b>");
def msg as mime.Part init mime.multipart("alternative", "b0", $parts);
io.printf("%s", mime.encode($msg));
```

The caller passes the boundary (it is not generated), so output is
deterministic and testable; pick a string that cannot appear in the content.

## Parsing

`parse` is the inverse: it splits the header block from the body at the first
blank line, unfolds continuation lines (a header value wrapped onto an
indented next line), reads `Content-Type` / `Content-Transfer-Encoding`, and
either splits a multipart body on its boundary (recursively) or
transfer-decodes a leaf. `encode` and `parse` round-trip (continuing the
example above):

```jennifer
def wire as string init mime.encode($msg);
def back as mime.Part init mime.parse($wire);
for (def part in mime.parts($back)) {
    io.printf("%s: %s\n", mime.contentType($part), mime.body($part));
}
```

A message whose structure cannot be read correctly is an error, not a guess.
`parse` throws `Error{kind: "mime"}` for:

- a header line without a colon, or a field name that is empty or holds a
  space or control character;
- a `multipart/*` type without a `boundary` parameter (or one longer than 70
  characters), and a multipart body that never reaches its close delimiter
  `--boundary--` (a truncated message) or holds no body part;
- a `base64` or `quoted-printable` body that is not valid in that encoding
  (QP: every `=` must start a hex escape or a soft line break);
- a text body whose bytes are not valid in its declared charset.

Multipart nesting deeper than 100 levels throws `Error{kind: "limit"}`. A body
part may have no header fields (it reads as a type-less text part), a delimiter
line may carry trailing whitespace, and the preamble and epilogue around the
delimiters are dropped. A transfer encoding other than base64 /
quoted-printable (`7bit`, `8bit`, `binary`, or an unknown label) is taken as
is.

## Extracting attachments and bodies

A received message is usually a `multipart/mixed` (bodies + attachments) whose
first child may itself be a `multipart/alternative` (the `text/plain` and
`text/html` views). The accessors flatten that tree:

```jennifer
use io;
use fs;
use path;
use strings;
import "mime.j" as mime;

func save(raw as string, dir as string) {
    def msg as mime.Part init mime.parse($raw); # or imap.fetchMessage($session, $uid)

    # Every readable body (both the plain and html alternatives).
    for (def part in mime.textBodies($msg)) {
        io.printf("[%s]\n%s\n", mime.contentType($part), mime.body($part));
    }

    # Save each attachment. data() is the raw bytes, filename() its name. The
    # name comes from the sender: keep only its last path element, so
    # "../../evil.txt" cannot write outside `dir`, and skip what is left empty.
    for (def att in mime.attachments($msg)) {
        def name as string init path.base(strings.replace(mime.filename($att), "\\", "/"));
        if ($name == "" or $name == "." or $name == ".." or $name == "/") {
            continue;
        }
        fs.writeNew(path.join($dir, $name), mime.data($att)); # never overwrites
    }
}
```

`walk` returns every leaf if you want to filter yourself; `findParts($msg,
"text/html")` picks a specific media type. `data` works on any leaf (text or
binary); `body` is the text convenience.

## Non-ASCII headers (RFC 2047 encoded-words)

Header values must be ASCII on the wire, so a non-ASCII `Subject` or display
name is carried as an RFC 2047 **encoded-word** (`=?UTF-8?B?...?=`), which
`encode` and `parse` handle:

- `encode` encodes a non-ASCII `Subject` / `Comments` value, and the
  display-name half of an address header (`From` / `To` / `Cc` / `Bcc` /
  `Reply-To` / `Sender`), leaving the `<addr>` untouched. A comma-separated
  address list is split on its top-level commas (a comma inside a quoted
  `"Last, First"` name is not a separator) and each mailbox's name is
  encoded on its own. Long values fold into several encoded-words split on
  rune boundaries.
- `parse` decodes those same headers back to plain text - so a fetched
  `Subject: =?UTF-8?B?QmVyaWNodCBhdXMgTcO8bmNoZW4=?=` reads back as
  `Bericht aus München`. A word that fails to decode is left verbatim, as RFC
  2047 says to.
- `mime.address("Jörg Müller", "j@x.de")` encodes the name for you.

```jennifer
use io;
import "mime.j" as mime;

def m as mime.Part init mime.withHeader(
    mime.text("text/plain", "hi"), "Subject", "Grüße aus München");
io.printf("%s", mime.encode($m));   # Subject: =?UTF-8?B?R3LDvMOfZSBhdXMgTcO8bmNoZW4=?=
```

`mime.encodeWord` / `mime.decodeWord` cover other headers (a custom header).
Both B (base64) and Q (quoted-printable) encoded-words decode; encoding always
emits B. `us-ascii` / `iso-8859-*` / `windows-*` charsets decode through
[`encoding`](../libraries/encoding.md); UTF-8 is the default.

## Out of scope

A message-structure foundation, not a full mail stack:

- **Legacy multibyte charsets.** A text part is decoded to `body` using its
  Content-Type `charset` for UTF-8 and every single-byte codepage the
  [`encoding`](../libraries/encoding.md) library ships (`us-ascii` /
  `iso-8859-*` / `windows-*`). A multibyte legacy charset (`shift_jis`,
  `gb2312`, `big5`, ...) has no `encoding` codec: `body` is a UTF-8 read when
  the bytes happen to be UTF-8, else `""`; the exact bytes always stay
  reachable via `data`.
- **Networking.** This module only shapes messages; sending / fetching them is
  the `mail` SMTP / POP3 / IMAP clients (built on `mime`).

## See also

- [encoding.md](../libraries/encoding.md) - `toText` / `fromText`
  (base64, quoted-printable), the transfer codecs `mime` delegates to.
- [strings.md](../libraries/strings.md) - the text operations the header and
  boundary handling build on.
- [modules/index.md](index.md) - the module catalog and import rules.

## Academy talks

| Talk | What it shows |
| ---- | ------------- |
| [Email with Jennifer: SMTP, IMAP and POP3](https://academy.jennifer-lang.dev/talks/mplx/email/) | composing multipart mail, encoded headers and parsing attachments |

## API reference

Every exported function, struct and constant of `mime`, with its signature, generated
from the module's doc comments: [the `mime` API reference](../api/mime.md).
