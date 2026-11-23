# `idna` - internationalized domain names

Import with `import "idna.j" as idna;`. Converts an internationalized domain
name between its Unicode form and its ASCII-compatible (`xn--`) encoding, over
a Punycode (RFC 3492) core: `münchen.de` goes on the wire as
`xn--mnchen-3ya.de` (DNS, SMTP envelopes, and URL hosts are ASCII-only). Pure
Jennifer over `strings`, `convert`, and `encoding`; no networking, TinyGo-clean.

```jennifer
use io;
import "idna.j" as idna;

io.printf("%s\n", idna.toAscii("münchen.de"));            # xn--mnchen-3ya.de
io.printf("%s\n", idna.toUnicode("xn--mnchen-3ya.de"));   # münchen.de
```

Runnable: [`examples/modules/idna_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/idna_demo.j).

## Surface

| Call                   | Returns  | Notes                                                              |
| ---------------------- | -------- | ------------------------------------------------------------------ |
| `idna.toAscii(domain)` | `string` | Domain to its ASCII form; a Unicode label becomes `xn--...`, an ASCII label is lowercased. |
| `idna.toUnicode(domain)` | `string` | The inverse; an `xn--` label is decoded, others pass through.     |
| `idna.isAscii(domain)` | `bool`   | Whether the domain is already all-ASCII (needs no conversion).     |

Both conversions work label by label (splitting on `.`), so a mixed domain
like `sub.münchen.example` converts only the label that needs it. `toAscii`
lowercases (IDNA case-folding); `toAscii(toUnicode(x))` round-trips a domain.

## What it is (and isn't)

The `xn--` transformation is Punycode (RFC 3492), a bootstring encoding that
packs the non-ASCII code points of a label into an ASCII string. This module is
that transformation plus lowercasing, enough for the common cases (European
accents, most scripts). It is not full IDNA2008, which layers nameprep /
mapping / validation tables on top, and it does no bidi / script-mixing
validation. It does enforce the DNS lengths: an A-label over 63 octets (on
either side of the conversion) or a domain over 253 is an `Error{kind: "idna"}`.

Malformed labels are errors too (`kind "idna"`), on both sides:

- An `xn--` label must be a real A-label (RFC 5891): it decodes to a label with
  at least one non-ASCII code point, already lowercase, that encodes back to
  the same A-label. `xn--abc-.com` (decodes to plain `abc`), `xn--.de` (decodes
  to nothing) and a truncated or non-canonical encoding are rejected instead of
  aliasing another name. `toAscii` checks an `xn--` label it is handed the same
  way.
- No label may carry a control character, DEL, a space, or an invisible format
  character: zero-width space / joiners (U+200B-U+200D), soft hyphen, bidi
  controls, variation selectors, U+FEFF and the like. ZWNJ / ZWJ are allowed by
  IDNA2008 in some contexts (CONTEXTJ); those rules are not implemented, so they
  are always rejected.
- A Unicode label (U-label) holds no ASCII besides letters, digits and hyphens,
  does not begin or end with a hyphen, and has no `--` in positions 3-4.
- An all-ASCII label is otherwise passed through lowercased: `_` (as in
  `_dmarc`), `*` or an IPv6 literal's `:` are not refused, since DNS names and
  the hosts the callers hand in are not limited to letters, digits and hyphens.

No UTS #46 mapping is done: a fullwidth `ｅｘａｍｐｌｅ` is encoded as it
is, not mapped to `example`.

The bootstring arithmetic works on rune **code-point integers**, which the
[`convert`](../libraries/convert.md) library provides via
`convert.toCodepoint(char)` / `convert.fromCodepoint(n)`.

## Used by the mail suite

The mail clients call `idna.toAscii` on the connection host and on the domain
part of each SMTP envelope address, so an internationalized recipient
(`user@münchen.de`) is delivered instead of throwing. A non-ASCII local part
(before the `@`) still errors; it needs SMTPUTF8 (RFC 6531), not yet supported.
Also usable for URL hosts, DNS tooling, or anywhere an IDN meets an ASCII-only
protocol.

## See also

- [convert.md](../libraries/convert.md) - `toCodepoint` / `fromCodepoint`, the
  rune / code-point pair the bootstring arithmetic uses.
- [smtp.md](smtp.md) / [pop.md](pop.md) / [imap.md](imap.md) - the mail clients
  that IDNA-encode their host and envelope domains.
- [modules/index.md](index.md) - the module catalog and import rules.

## API reference

Every exported function, struct and constant of `idna`, with its signature, generated
from the module's doc comments: [the `idna` API reference](../api/idna.md).
