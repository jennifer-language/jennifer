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
mapping / validation tables on top, and it does no length checks and no bidi /
script-mixing validation.

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
