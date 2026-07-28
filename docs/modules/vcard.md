# `vcard` - vCard (RFC 6350) contacts build and parse

Import with `import "vcard.j" as vcard;`. Build contact `Card`s and encode them
to **vCard 4.0** text (`VCARD` records), and parse that text back. The contacts
counterpart to [`ical`](ical.md): the two share the same content-line codec (TEXT
escaping, 75-character line folding). Pure Jennifer over `strings` / `lists` - no
Go engine, so it runs on **both** binaries.

```jennifer
import "vcard.j" as vcard;

def c as vcard.Card init vcard.card("Ada Lovelace");
$c = vcard.withName($c, "Lovelace", "Ada");
$c = vcard.addEmail($c, "ada@example.com");
def text as string init vcard.encode($c);   # BEGIN:VCARD ... END:VCARD
```

Runnable: [`examples/modules/vcard_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/vcard_demo.j).

## Types

Both structs have public fields (read them directly - `$c.emails`, `$c.family`);
the builder functions are the conventional way to construct them.

```jennifer
def struct vcard.Card {
    formattedName as string,          # FN (required by vCard 4.0)
    family as string, given as string,          # N family / given
    additional as string,             # N additional (middle) name(s)
    prefixes as string, suffixes as string,     # N honorific prefixes / suffixes
    nickname as string,               # NICKNAME ("" when unset)
    organization as string,           # ORG ("" when unset)
    title as string,                  # TITLE ("" when unset)
    emails as list of Typed,          # EMAIL (0..n, each with an optional TYPE)
    phones as list of Typed,          # TEL (0..n, each with an optional TYPE)
    addresses as list of Address,     # ADR (0..n)
    url as string,                    # URL ("" when unset)
    bday as string,                   # BDAY ("" when unset)
    photo as string,                  # PHOTO URI ("" when unset)
    categories as list of string,     # CATEGORIES tags
    note as string                    # NOTE ("" when unset)
};
def struct vcard.Typed { value as string, type as string };   # a value + optional TYPE ("work" / "home" / ...)
def struct vcard.Address {
    street as string, locality as string, region as string,
    postalCode as string, country as string, type as string
};
```

## Building

The builders are **value-semantic** - each returns a fresh copy and never
mutates its argument, so you thread them:

| Call | Returns | |
| ---- | ------- | - |
| `vcard.card(formattedName)` | `Card` | a card with just its `FN` display name |
| `vcard.withName(c, family, given)` | `Card` | set the `N` family / given |
| `vcard.withFullName(c, family, given, additional, prefixes, suffixes)` | `Card` | set all five `N` components |
| `vcard.withNickname(c, nickname)` | `Card` | set the `NICKNAME` |
| `vcard.withOrg(c, organization, title)` | `Card` | set `ORG` and `TITLE` |
| `vcard.addEmail(c, email)` / `addEmailTyped(c, email, type)` | `Card` | append an `EMAIL` (with optional `TYPE`) |
| `vcard.addPhone(c, phone)` / `addPhoneTyped(c, phone, type)` | `Card` | append a `TEL` (with optional `TYPE`) |
| `vcard.address(...)` / `addressTyped(..., type)` | `Address` | build an address (with optional `TYPE`) |
| `vcard.addAddress(c, address)` | `Card` | append an `ADR` |
| `vcard.withBday(c, bday)` / `withPhoto(c, uri)` | `Card` | set `BDAY` / `PHOTO` |
| `vcard.addCategory(c, tag)` | `Card` | append a `CATEGORIES` tag |
| `vcard.withUrl(c, url)` | `Card` | set the `URL` |
| `vcard.withNote(c, note)` | `Card` | set the `NOTE` |

```jennifer
def c as vcard.Card init vcard.card("Grace Hopper");
$c = vcard.withName($c, "Hopper", "Grace");
$c = vcard.withOrg($c, "US Navy", "Rear Admiral");
$c = vcard.addEmail($c, "grace@navy.mil");
$c = vcard.addAddress($c, vcard.address("1 Navy Way", "Arlington", "VA", "22202", "USA"));
```

## Encoding and parsing

| Call | Returns | |
| ---- | ------- | - |
| `vcard.encode(c)` | `string` | one card as a `VCARD` (CRLF-terminated) |
| `vcard.encodeAll(cards)` | `string` | many cards concatenated |
| `vcard.parse(text)` | `list of Card` | parse one or many `VCARD`s |

`parse(encode(card))` round-trips the data. `parse` always returns a **list** (a
vCard file may hold many cards, or none). `encode` writes `VERSION:4.0`, escapes
text values, folds long lines, and omits empty optional fields. `parse` unfolds
folded lines, ignores property parameters (the `;KEY=VALUE` after a name, e.g.
`EMAIL;TYPE=work`), reads the structured `N` / `ADR` values, and unescapes text.

## Structured values

`N` (name), `ADR` (address), and `ORG` are **structured**: components are
separated by `;`. `N` is `Family;Given;Additional;Prefix;Suffix` (this module
sets family and given); `ADR` is
`POBox;Extended;Street;Locality;Region;PostalCode;Country` (this module sets the
last five, leaving PO box and extended empty); `ORG` is
`Organization;Unit;...` (this module sets and reads the first component). A `;`
or `,` inside a component value is escaped, so it never splits the structure.

## Scope

- **A contact subset.** `FN` / `N` / `ORG` / `TITLE` / `EMAIL` / `TEL` / `ADR` /
  `URL` / `NOTE`. No `BDAY` / `PHOTO` / `GEO` / `TZ` / `KIND` / grouping, no
  parameter round-tripping (a `TYPE=work` on input is dropped, not preserved),
  and the `N` additional / prefix / suffix and `ADR` PO-box / extended components
  are not modelled. The escaping / folding / structured-value discipline is the
  reusable core.
- **vCard 4.0 output.** `encode` always writes `VERSION:4.0`; `parse` reads any
  version's properties (it does not enforce the version line).

## See also

- [ical.md](ical.md) - the calendar counterpart sharing the content-line codec.
- [strings.md](../libraries/strings.md) - the text surface the codec is built on.
- [modules/index.md](index.md) - the module catalog and import rules.
