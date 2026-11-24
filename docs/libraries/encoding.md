# `encoding` - introspection + text and character codecs

Enable with `use encoding;`. Four groups of functions:

1. **Introspection** - rune count vs byte count, and an ASCII test.
2. **Binary-to-text** (`toText` / `fromText`) - hex, base64,
   quoted-printable, and URI percent / form encoding, `bytes` to a printable
   `string` and back.
3. **Character codecs** (`encode` / `decode`) - a Jennifer `string` to and
   from bytes in a named encoding: UTF-8, UTF-16, UTF-32, and the
   single-byte legacy ones (ISO-8859-*, Windows-*, EBCDIC).
4. **Request parsers** (`parseForm` / `parseCookies` / `parseMultipart`) -
   form bodies, query strings, a Cookie header and multipart/form-data, with
   bounded memory.

Text becomes bytes, and bytes text, only here: `encoding.encode("utf-8",
s)` is how a string becomes `bytes`. `convert.bytesFromString` /
`convert.stringFromBytes` were removed in 0.28.0 (lint L302).

```jennifer
use io;
use encoding;

def src as bytes init encoding.encode("utf-8", "café");
io.printf("%s\n", encoding.toText("hex", $src));           # 636166c3a9
io.printf("%t\n", encoding.isAscii($src));                 # false
io.printf("%s\n", encoding.toText("hex", encoding.encode("iso-8859-1", "café")));  # 636166e9
```

## Introspection

| Call                   | Returns | Notes                                                              |
| ---------------------- | ------- | ------------------------------------------------------------------ |
| `encoding.isAscii(b)`  | `bool`  | True iff every byte is `< 0x80`. Empty bytes is true.              |
| `encoding.lenBytes(s)` | `int`   | Byte length of `s`'s UTF-8 encoding. Pair with `len(s)` (runes).   |
| `encoding.lenRunes(b)` | `int`   | Rune count of valid UTF-8 `bytes`; errors on invalid UTF-8.        |

## Binary-to-text: `toText` / `fromText`

One verb pair, the encoding named by a string. Reversible representation:
these grow or reshape the bytes without reducing information (for that, see
[`compress`](compress.md)).

| Call                        | Returns  | Notes                                    |
| --------------------------- | -------- | ---------------------------------------- |
| `encoding.toText(format, b)`   | `string` | Encode `bytes` as printable text.     |
| `encoding.fromText(format, s)` | `bytes`  | Decode back to `bytes`.               |

### Formats

| `format`             | Standard      | Notes                                                                         |
| -------------------- | ------------- | ----------------------------------------------------------------------------- |
| `"hex"`              | base-16       | Lowercase on encode; decode accepts upper and lower case (see below). Two chars per byte. |
| `"base32"`           | RFC 4648 §6   | Standard alphabet (`A`-`Z` `2`-`7`), `=` padding. Canonical only (see below).|
| `"base32-hex"`       | RFC 4648 §7   | Extended-hex alphabet (`0`-`9` `A`-`V`), `=` padding. Canonical only (see below).|
| `"base64"`           | RFC 4648 §4   | Standard alphabet (`+` `/`, `=` padding). Canonical only (see below).          |
| `"base64-url"`       | RFC 4648 §5   | URL / filename-safe alphabet (`-` `_`). Canonical only (see below).            |
| `"ascii85"`          | Adobe / btoa  | Base-85, the `!`..`u` alphabet, with `z` for an all-zero group.               |
| `"z85"`              | ZeroMQ RFC 32 | Base-85, a source-safe alphabet, no padding. Input must be a multiple of 4 bytes (decode input a multiple of 5 chars). |
| `"quoted-printable"` | RFC 2045      | MIME transfer encoding (see below).                                           |
| `"uri-percent"`      | RFC 3986      | RFC 3986 percent-encoding (see below). Space is `%20`.                         |
| `"uri-form"`         | WHATWG URL    | `application/x-www-form-urlencoded` (see below). Space is `+`.                 |

**hex decode accepts both cases**: hex from other tools is often uppercase
(MAC addresses, digests, RFC 4648's base16 alphabet), and case carries no
bits, unlike base64's padding bits, so it opens no second meaning.

**base32 / base64 decode accepts only the canonical spelling**: the text
`toText` would produce for those bytes. A line break anywhere, missing or
extra `=` padding, and unused bits set in the last character are errors, so
one byte string has exactly one accepted text and a token cannot be respelled
past a check that compares text. Line-wrapped input (a MIME body, a PEM block)
is unwrapped first, as the [`mime`](../modules/mime.md) module does:
`encoding.fromText("base64", strings.replace(strings.replace($wrapped, "\r", ""), "\n", ""))`.

**Quoted-printable** keeps printable ASCII literal, turns `=`, control, and
8-bit bytes into `=XX`, and soft-wraps lines to 76 columns with a trailing
`=`. Decode reverses it and round-trips `bytes`. Every `=` on decode is an
escape of two hex digits (upper or lower case, RFC 2045 6.7) or a soft line
break (`=`, optional spaces / tabs, then LF, CRLF or the end of the text);
anything else (`"a=ZZb"`, a trailing `"=4"`) is kind `"encoding"`.

**URI percent / form.** `"uri-percent"` is RFC 3986 percent-encoding: every
byte outside the unreserved set (`A`-`Z` `a`-`z` `0`-`9` `-` `.` `_` `~`)
becomes `%XX` (uppercase hex), and a space becomes `%20`. `"uri-form"` is the
`application/x-www-form-urlencoded` variant used for query strings and HTML form
bodies: identical, except a space becomes `+` (and a literal `+` therefore
encodes as `%2B`). Decode reverses each; a malformed `%` escape (`%`, `%2`,
`%zz`) is a positioned error. These are the byte-level codecs; the
[`uri`](../modules/uri.md) module builds on them for URL parsing, building, and
query strings (its `encode` / `decode` are `uri-percent`, `encodeForm` /
`decodeForm` and `buildQuery` / `parseQuery` are `uri-form`).

Format names are **exact** (case-sensitive, no `-` / `_` normalisation):
`"base64"` works; `"BASE64"` errors with the supported set listed.

A format string, rather than `encoding.hex()` / `encoding.base64()`, keeps
the codec-table shape shared with the rest of this library and with
[`hash`](hash.md) and [`crc`](crc.md).

## Character codecs: `encode` / `decode`

Convert a Jennifer `string` to and from bytes in a named encoding.

| Call                        | Returns          | Notes                                                       |
| --------------------------- | ---------------- | ----------------------------------------------------------- |
| `encoding.encode(codec, s)` | `bytes`          | Errors when a rune has no byte in the codec (e.g. `€` in ASCII). |
| `encoding.decode(codec, b)` | `string`         | Errors when a byte is undefined in the codec.               |
| `encoding.codecs()`         | `list of string` | Every registered codec name, in registration order.         |

### Unicode: UTF-8, UTF-16, UTF-32

| Codec | Encode writes | Decode reads |
| --- | --- | --- |
| `"utf-8"` | the string's bytes | valid UTF-8 only |
| `"utf-16le"`, `"utf-16be"` | that byte order, no byte-order mark | that byte order; a leading U+FEFF is a character |
| `"utf-16"` | big-endian, with a byte-order mark | the mark's order, big-endian without one (RFC 2781) |
| `"utf-32le"`, `"utf-32be"`, `"utf-32"` | as for UTF-16, four bytes per character | as for UTF-16 |

Decoding is strict: invalid UTF-8, an odd number of bytes for UTF-16 (not a
multiple of 4 for UTF-32), an unpaired surrogate, or a UTF-32 value that is
not a Unicode scalar value is an error that names the byte offset. Encoding
never fails, as a string holds only valid characters.

```jennifer
use encoding;
use io;

def wire as bytes init encoding.encode("utf-16le", "Grüße");
io.printf("%d bytes\n", len($wire));                       # 10 bytes
io.printf("%s\n", encoding.decode("utf-16le", $wire));     # Grüße
io.printf("%s\n", encoding.toText("hex", encoding.encode("utf-16", "A"))); # feff0041
```

### Single-byte codecs

Every codec below is single-byte: one byte maps to one rune. Bytes with no
assignment in a codec (some Windows code pages have a few) are a positioned
error on decode; runes with no byte are a positioned error on encode.

**ASCII and EBCDIC**

| Codec        | Notes                                                                    |
| ------------ | ------------------------------------------------------------------------ |
| `"ascii"`    | 7-bit US-ASCII. Rejects any byte or rune `>= 0x80`.                      |
| `"ebcdic"`   | IBM Code Page 1047 (the Open Systems Latin-1 EBCDIC variant).           |

**ISO/IEC 8859 (single-byte Latin and script families)**

| Codec           | Part      | Coverage                                            |
| --------------- | --------- | --------------------------------------------------- |
| `"iso-8859-1"`  | Latin-1   | Western European (identity `U+0000..U+00FF`).       |
| `"iso-8859-2"`  | Latin-2   | Central / Eastern European.                         |
| `"iso-8859-3"`  | Latin-3   | South European (Maltese, Esperanto).                |
| `"iso-8859-4"`  | Latin-4   | North European (Baltic).                            |
| `"iso-8859-5"`  | Cyrillic  | Latin / Cyrillic.                                   |
| `"iso-8859-6"`  | Arabic    | Latin / Arabic.                                     |
| `"iso-8859-7"`  | Greek     | Latin / Greek.                                      |
| `"iso-8859-8"`  | Hebrew    | Latin / Hebrew.                                     |
| `"iso-8859-9"`  | Latin-5   | Turkish.                                            |
| `"iso-8859-10"` | Latin-6   | Nordic.                                             |
| `"iso-8859-11"` | Thai      | Latin / Thai.                                       |
| `"iso-8859-13"` | Latin-7   | Baltic Rim.                                         |
| `"iso-8859-14"` | Latin-8   | Celtic.                                             |
| `"iso-8859-15"` | Latin-9   | Western European, Latin-1 with the euro sign at `0xA4` and Š š Ž ž Œ œ Ÿ. |
| `"iso-8859-16"` | Latin-10  | South-Eastern European.                             |

(There is no `iso-8859-12`; the draft was abandoned.)

**Windows code pages**

| Codec            | Coverage                                             |
| ---------------- | --------------------------------------------------- |
| `"windows-1250"` | Central European.                                   |
| `"windows-1251"` | Cyrillic.                                           |
| `"windows-1252"` | Western European - Latin-1 plus the `0x80..0x9F` "smart quotes" set (incl. the euro sign). Five positions (`0x81 0x8D 0x8F 0x90 0x9D`) are undefined. |
| `"windows-1253"` | Greek.                                              |
| `"windows-1254"` | Turkish.                                            |
| `"windows-1255"` | Hebrew.                                             |
| `"windows-1256"` | Arabic.                                             |
| `"windows-1257"` | Baltic.                                             |
| `"windows-1258"` | Vietnamese.                                         |
| `"macintosh"`    | Mac OS Roman (IANA `macintosh`), as classic Mac OS text, PDF's `MacRomanEncoding` and TrueType platform-1 names use it. `0xDB` is the euro sign (Apple's mapping since Mac OS 8.5, as WHATWG has it), `0xF0` the Apple logo `U+F8FF`; all 256 bytes are defined. |

### Codec names are exact

Codec names are **exact-match**: one canonical spelling, no case-folding,
separator-stripping, or IANA aliases. `"iso-8859-1"` works; `"latin-1"`,
`"ISO-8859-1"`, `"iso88591"`, and `"cp1252"` all error with the known set
listed (no lenient spelling hides a typo).
Map an external name (an HTTP `charset=ISO-8859-1`, say) to the canonical
form yourself before calling.

### How the tables are built

Only `ascii` (7-bit, with its own out-of-range errors) and `ebcdic`
(IBM-1047, not in the standard Unicode mapping path) are hand-written.
Every ISO-8859 and Windows single-byte codec, including `iso-8859-1` and
`windows-1252`, and `macintosh` (Apple's `ROMAN.TXT`, whose unlisted control
characters map to themselves) are **generated from the Unicode Consortium
mapping files**:

```sh
go generate ./internal/lib/encoding/   # writes codecs_gen.go
```

### Converting a text file

Jennifer strings are UTF-8 internally, so converting a legacy-encoded file
to UTF-8 is decode-then-write, with no separate "encode to UTF-8" step:

```jennifer
use fs;
use encoding;

def raw as bytes init fs.readBytes("legacy.txt");              # Windows-1252 bytes
def text as string init encoding.decode("windows-1252", $raw); # -> a UTF-8 string
fs.writeString("utf8.txt", $text);                             # written as UTF-8
```

The reverse (UTF-8 to a single-byte encoding) is `encoding.encode(codec,
text)`, which errors if a character has no byte in the target codec.

## Request parsers: `parseForm` / `parseCookies` / `parseMultipart`

The Go halves of `web.form` / `web.cookie`, `uri.parseQuery` and
`multipart.parse`. Untrusted request text is scanned in place and only the
results are allocated, so memory stays a small multiple of the input (a 3 MB
`&&&...` body takes a few MB, not the 3 GB a split in `.j` would), and the
result count is capped. Pure parsing: both binaries, no capability.

| Call                                    | Returns                        | Notes |
| --------------------------------------- | ------------------------------ | ----- |
| `encoding.parseForm(text)`              | `map of string to string`      | `application/x-www-form-urlencoded` / a query string. |
| `encoding.parseCookies(header)`         | `map of string to string`      | The cookies of a `Cookie` header, name to value. |
| `encoding.parseMultipart(contentType, body)` | `list of encoding.FormPart` | `multipart/form-data` (RFC 7578). |

**`parseForm`** splits on `&`, skips empty pairs, splits a pair at its first
`=` (none: the value is `""`), and decodes both halves: `+` is a space,
`%XX` a byte. A repeated key keeps its first position and its last value.
Every `%` needs two hex digits and each decoded half must be UTF-8, else kind
`"encoding"`; there is no lenient mode. More than 10000 non-empty pairs: kind
`"limit"`.

**`parseCookies`** splits the header on `;` and trims each piece; a piece
`name=value` (a non-empty name before its first `=`) gives `name` ->
`value`, as sent (no decoding). The first of a repeated name wins, and a
piece without a name is skipped. A missing cookie is a missing key (test with
`maps.has`). More than 1000 cookies: kind `"limit"`.

**`parseMultipart`** reads `boundary` from the `Content-Type` parameters
(quoted or bare, a whole name: not `xboundary`) and splits the body at
`CRLF--boundary`; the preamble and the epilogue after the close delimiter
`--boundary--` are dropped. Each part is `encoding.FormPart{name, filename,
contentType, data}`: `name` and `filename` from `Content-Disposition:
form-data` (quotes and `\` escapes honoured), `contentType` from the part's
`Content-Type` (`""` if none), `data` a copy of the part body. Kind
`"encoding"`: no boundary, no close delimiter, a line starting with the boundary
that is no delimiter, a part without its blank line, a header that is not
UTF-8 or a header line without a colon, no `form-data` disposition or no
`name`, an unterminated quoted parameter. Kind `"limit"`: more than 1000
parts, a part header over 16 KiB, a `name` or `filename` over 1 KiB.

Messages start with the function's name (`encoding.parseForm: invalid %
escape "%zz"`). `uri` and `multipart` rethrow an `"encoding"` error under
their own kind (`"uri"`, `"multipart"`) and pass `"limit"` on.

## Errors and memory

### Errors

Malformed input is kind `"encoding"`; misuse is `"runtime"`. Every error is
positioned at the call site.

| Kind | Raised for | Example message |
| ---- | ---------- | --------------- |
| `"encoding"` | text `fromText` cannot decode in its format: a bad character or length, non-canonical base64 / base32, a line break, a bad `%` escape | `encoding.fromText: percent decode: invalid % escape "%zz"` |
| `"encoding"` | a byte undefined in the codec on `decode` | `encoding.decode (windows-1252): byte 0x81 at position 0 has no mapping in windows-1252` |
| `"encoding"` | malformed Unicode on `decode` | `encoding.decode (utf-16be): high surrogate 0xD83D at byte 2 has no low surrogate`, `encoding.decode (utf-8): invalid UTF-8 at byte 1` |
| `"encoding"` | invalid UTF-8 to `lenRunes` | `encoding.lenRunes: input is not valid UTF-8` |
| `"encoding"` | a malformed form or multipart input (above) | `encoding.parseForm: invalid % escape "%zz"` |
| `"limit"` | a request parser's cap (above) | `encoding.parseForm: more than 10000 fields` |
| `"runtime"` | wrong arity | `encoding.encode expects 2 arguments (codec, string), got 1` |
| `"runtime"` | wrong argument type | `encoding.encode: second argument must be string, got bytes` |
| `"runtime"` | unknown format or codec (the message lists every name) | `encoding.encode: unknown codec "klingon"; known: "ascii", "iso-8859-1", ...` |
| `"runtime"` | a rune the codec cannot encode, `z85` input not a multiple of 4 bytes | `encoding.encode (ascii): rune U+00E9 at byte position 3 is outside ASCII (0x00..0x7F)` |

### Length and memory

`encode` / `decode` / `toText` / `fromText` are **one-shot**: each takes the
whole input and builds the whole output in memory, so both are held at once.
There is no fixed cap: a Jennifer `string` / `bytes` is a Go `string` /
`[]byte`, bounded only by available memory. For a file too large to hold
twice over: the single-byte codecs decode each byte independently, so
decoding can be split at any byte boundary. Read the file in fixed-size byte
chunks, `decode` each, and append to the output. (Encoding the other way must
split on rune boundaries, since a UTF-8 character can span several bytes.)

## See also

- [compress.md](compress.md) - size reduction (distinct from representation).
- [hash.md](hash.md), [crc.md](crc.md) - digests whose output you'll often
  `encoding.toText("hex", $digest)`.

## Academy talks

| Talk | What it shows |
| ---- | ------------- |
| [Jennifer 0.27: what's new, what's deprecated, what breaks](https://academy.jennifer-lang.dev/talks/mplx/release-0-27/) | text to bytes moved into encoding, UTF-16 and UTF-32 |
