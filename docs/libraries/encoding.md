# `encoding` - introspection + text and character codecs

Enable with `use encoding;`. Three groups of functions:

1. **Introspection** - rune count vs byte count, and an ASCII test.
2. **Binary-to-text** (`toText` / `fromText`) - hex, base64, and
   quoted-printable, `bytes` to a printable `string` and back.
3. **Character codecs** (`encode` / `decode`) - a Jennifer `string` to and
   from a single-byte legacy encoding (ISO-8859-*, Windows-*, EBCDIC).

The cross-kind UTF-8 codec ships with [`convert`](convert.md)
(`convert.bytesFromString` / `convert.stringFromBytes`); this library is
where the rest of the codec proliferation lives.

```jennifer
use io;
use convert;
use encoding;

def src as bytes init convert.bytesFromString("café", "utf-8");
io.printf("%s\n", encoding.toText($src, "hex"));           # 636166c3a9
io.printf("%t\n", encoding.isAscii($src));                 # false
io.printf("%x\n", encoding.encode("café", "iso-8859-1"));  # 63 61 66 e9
```

## Introspection

| Call                   | Returns | Notes                                                              |
| ---------------------- | ------- | ------------------------------------------------------------------ |
| `encoding.isAscii(b)`  | `bool`  | True iff every byte is `< 0x80`. Empty bytes is true.              |
| `encoding.lenBytes(s)` | `int`   | Byte length of `s`'s UTF-8 encoding. Pair with `len(s)` (runes).   |
| `encoding.lenRunes(b)` | `int`   | Rune count of valid UTF-8 `bytes`; errors on invalid UTF-8.        |

## Binary-to-text: `toText` / `fromText`

One verb pair, the encoding named by a string. Reversible representation -
these grow or reshape the bytes, they don't reduce information (that's
[`compress`](compress.md)).

| Call                        | Returns  | Notes                                    |
| --------------------------- | -------- | ---------------------------------------- |
| `encoding.toText(b, format)`   | `string` | Encode `bytes` as printable text.     |
| `encoding.fromText(s, format)` | `bytes`  | Decode back to `bytes`.               |

### Formats

| `format`             | Standard      | Notes                                                                         |
| -------------------- | ------------- | ----------------------------------------------------------------------------- |
| `"hex"`              | base-16       | Lowercase on encode; decode accepts upper and lower case. Two chars per byte. |
| `"base32"`           | RFC 4648 §6   | Standard alphabet (`A`-`Z` `2`-`7`), `=` padding.                             |
| `"base32-hex"`       | RFC 4648 §7   | Extended-hex alphabet (`0`-`9` `A`-`V`), `=` padding.                         |
| `"base64"`           | RFC 4648 §4   | Standard alphabet (`+` `/`, `=` padding).                                      |
| `"base64-url"`       | RFC 4648 §5   | URL / filename-safe alphabet (`-` `_`).                                        |
| `"ascii85"`          | Adobe / btoa  | Base-85, the `!`..`u` alphabet, with `z` for an all-zero group.               |
| `"z85"`              | ZeroMQ RFC 32 | Base-85, a source-safe alphabet, no padding. Input must be a multiple of 4 bytes (decode input a multiple of 5 chars). |
| `"quoted-printable"` | RFC 2045      | MIME transfer encoding (see below).                                           |

**Quoted-printable** keeps printable ASCII literal, turns `=`, control, and
8-bit bytes into `=XX`, and soft-wraps lines to 76 columns with a trailing
`=`. Decode reverses it, tolerant of both CRLF and LF soft breaks, and
round-trips `bytes`.

Format names are **exact** (case-sensitive, no `-` / `_` normalisation) -
they're the library's own fixed set, not external standards with variant
spellings. `"base64"` works; `"BASE64"` errors with the supported set
listed.

> **Why a format string instead of `encoding.hex()` / `encoding.base64()`?**
> Jennifer's letters-only identifier rule rejects digits in method names, so
> `encoding.base64` won't parse; and the codec-table shape already matches
> the rest of this library plus [`convert`](convert.md), [`hash`](hash.md),
> and [`crc`](crc.md).

## Character codecs: `encode` / `decode`

Convert a Jennifer `string` to and from a named single-byte encoding.

| Call                        | Returns          | Notes                                                       |
| --------------------------- | ---------------- | ----------------------------------------------------------- |
| `encoding.encode(s, codec)` | `bytes`          | Errors when a rune has no byte in the codec (e.g. `€` in ASCII). |
| `encoding.decode(b, codec)` | `string`         | Errors when a byte is undefined in the codec.               |
| `encoding.codecs()`         | `list of string` | Every registered codec name, in registration order.         |

### Codec set

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

### Codec names are exact

Codec names are **exact-match** - the one canonical spelling only, no
case-folding, separator-stripping, or IANA aliases. `"iso-8859-1"` works;
`"latin-1"`, `"ISO-8859-1"`, `"iso88591"`, and `"cp1252"` all error with the
known set listed. This is deliberate strictness (stance #2, explicit over
implicit): a codec is named one way, and no lenient spelling hides a typo.
Map an external name (an HTTP `charset=ISO-8859-1`, say) to the canonical
form yourself before calling.

### How the tables are built

Only `ascii` (7-bit, with its own out-of-range errors) and `ebcdic`
(IBM-1047, not in the standard Unicode mapping path) are hand-written.
Every ISO-8859 and Windows single-byte codec - including `iso-8859-1` and
`windows-1252` - is **generated from the Unicode Consortium mapping files**,
so every table comes from one authoritative source rather than being
hand-transcribed:

```sh
go generate ./internal/lib/encoding/   # writes codecs_gen.go
```

### Converting a text file

Jennifer strings are UTF-8 internally, so converting a legacy-encoded file
to UTF-8 is just decode-then-write - there's no separate "encode to UTF-8"
step:

```jennifer
use fs;
use encoding;

def raw as bytes init fs.readBytes("legacy.txt");              # Windows-1252 bytes
def text as string init encoding.decode($raw, "windows-1252"); # -> a UTF-8 string
fs.writeString("utf8.txt", $text);                             # written as UTF-8
```

The reverse (UTF-8 to a single-byte encoding) is `encoding.encode(text,
codec)`, which errors if a character has no byte in the target codec.

### Length and memory

`encode` / `decode` / `toText` / `fromText` are **one-shot**: each takes the
whole input and builds the whole output in memory, so both are held at once.
There is no fixed cap - a Jennifer `string` / `bytes` is a Go `string` /
`[]byte`, bounded only by available memory. For a file too large to hold
twice over, note that the single-byte codecs decode each byte independently,
so decoding can be split at any byte boundary: read the file in fixed-size
byte chunks, `decode` each, and append to the output. (Encoding the other
way must instead split on rune boundaries, since a UTF-8 character can span
several bytes.)

## Errors

- Wrong arity: `encoding.encode expects 2 arguments (string, codec), got 1`.
- Wrong scalar type: `encoding.encode: first argument must be string, got bytes`.
- Unknown codec (the message lists every registered name):
  `encoding.encode: unknown codec "klingon"; known: "ascii", "iso-8859-1", ...`.
- Unrepresentable rune on encode:
  `encoding.encode (ascii): rune U+00E9 at byte position 3 is outside ASCII (0x00..0x7F)`.
- Undefined byte on decode:
  `encoding.decode (windows-1252): byte 0x81 at position 0 has no mapping in windows-1252`.
- Invalid UTF-8 to `lenRunes`: `encoding.lenRunes: input is not valid UTF-8`.

All errors are positioned at the call site.

## See also

- [convert.md](convert.md) - the UTF-8 pair (`bytesFromString` / `stringFromBytes`).
- [compress.md](compress.md) - size reduction (distinct from representation).
- [hash.md](hash.md), [crc.md](crc.md) - digests whose output you'll often
  `encoding.toText($digest, "hex")`.
