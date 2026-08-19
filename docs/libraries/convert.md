# `convert` - explicit type conversion

Enable with `use convert;`. Provides one-argument conversion functions plus
`typeOf` for runtime kind introspection. Every function either returns the
converted value or produces a positioned runtime error.

```jennifer
use io;
use convert;

def n as int init convert.toInt("42");        # parse string -> 42
def f as float init convert.toFloat(5);       # int -> 5.0
def s as string init convert.toString(3.14);  # any -> "3.14"
def b as bool init convert.toBool(0);         # 0 -> false

io.printf("%s\n", convert.typeOf(5 / 2));      # "float" (after Python 3 / change)
io.printf("%s\n", convert.typeOf(5 // 2));     # "int"
```

## Behavior summary

| Call                                       | Source kinds                | Behavior                                                              |
| ------------------------------------------ | --------------------------- | --------------------------------------------------------------------- |
| `convert.toInt(v)`                         | int / float / string / bool | identity / truncate / parse / `true`=1, `false`=0                     |
| `convert.toFloat(v)`                       | int / float / string / bool | convert / identity / parse / `true`=1.0, `false`=0.0                  |
| `convert.toString(v)`                      | any                         | always succeeds; uses the value's display form                        |
| `convert.toBool(v)`                        | bool / int / float / string | identity / canonical only (`0`/`1`, `0.0`/`1.0`, `"true"`/`"false"`)  |
| `convert.typeOf(v)`                        | any                         | returns the kind as a string: `"int"`, `"float"`, ..., `"list"`, `"map"`, `"object"` |
| `convert.objectType(v)`                    | object                      | specific registered name of an opaque object, e.g. `"json.Value"`; errors on a non-object |
| `convert.bytesFromString(s, codec)` | (string, string)            | string to bytes; `"utf-8"` only (other codecs live in `encoding`)                            |
| `convert.stringFromBytes(b, codec)` | (bytes, string)             | bytes to string; `"utf-8"` only; invalid UTF-8 is an error |
| `convert.toCodepoint(char)`         | string                      | Unicode code point (int) of a **one-rune** string; errors unless the argument is exactly one code point |
| `convert.fromCodepoint(n)`          | int                         | the **one-rune** string for code point `n`; errors on a negative / out-of-range / surrogate value |

`bytesFromString` / `stringFromBytes` are the **UTF-8** cross-kind pair
(`string` to `bytes` and back) - the one codec every program needs. Every
other character encoding (ISO-8859-*, Windows-*, EBCDIC) and the
binary-to-text codecs (hex, base64, quoted-printable) live in
[`encoding`](encoding.md): `encoding.encode` / `decode` and
`encoding.toText` / `fromText`.

### `toCodepoint` / `fromCodepoint`: code point, not "character"

These convert between a single **code point** (a Unicode scalar value, an
`int`) and the **one-rune string** that holds it - the primitive a Unicode
algorithm needs (Punycode digits, a `\x01` control byte, an escape). Two
details matter here:

- **The whole range works, not just ASCII or the BMP.** `fromCodepoint`
  accepts any scalar value `0`..`0x10FFFF` (hex literals are fine:
  `fromCodepoint(0x20AC)` is `€`, `fromCodepoint(0x1F602)` is `😂`). The
  result is *one rune* whose UTF-8 encoding is **1 to 4 bytes** - a code point
  is not a byte. Only a negative value, one above `0x10FFFF`, or a UTF-16
  surrogate (`0xD800`..`0xDFFF`) errors.
- **"One rune" is not "one character a reader sees."** A user-perceived
  character - a *grapheme cluster* - can be several code points: a base plus
  combining marks (`e` + U+0301 = `é`), an NFD-decomposed accent, an emoji ZWJ
  sequence (`👨‍👩‍👧`). Each of those code points round-trips individually, but
  `toCodepoint` takes **exactly one** code point, so it rejects a multi-rune
  cluster the same way it rejects `"ab"`. `len`, `strings.chars`, and
  `strings.substring` are all **rune-indexed** too (see
  [strings.md](strings.md)); Jennifer has no grapheme-cluster API. The tell:
  precomposed `é` (U+00E9) is `len` 1 and `toCodepoint` gives 233; decomposed
  `é` (`e` + U+0301) is `len` 2 and `toCodepoint` errors.

## Errors

- `convert.toInt("abc")` - parse failure (string doesn't represent a valid integer).
- `convert.toInt(null)` - no conversion defined.
- `convert.toInt(f)` for a NaN, +/-Infinity, or out-of-int64-range float - the value has no representable integer (truncation would be garbage).
- `convert.toBool("maybe")` - strings: only `"true"` and `"false"` accepted.
- `convert.toBool(123)`, `convert.toBool(-1)` - ints: only `0` and `1` accepted.
- `convert.toBool(1.5)` - floats: only `0.0` and `1.0` accepted.
- `convert.stringFromBytes(b, "utf-8")` on bytes that aren't valid
  UTF-8 - strict at boundaries; no silent replacement characters.
- `convert.bytesFromString(s, "iso-8859-1")` or any non-`"utf-8"`
  codec name - rejected as unsupported; use
  [`encoding`](encoding.md)`.encode` / `decode` for those.
- Arity errors (too many or too few arguments).

For "any nonzero counts as true" semantics, write the comparison explicitly:

```jennifer
def b as bool init $count != 0;     // not convert.toBool($count)
```

This matches the strict-conditions rule everywhere else in Jennifer - if you
want to project an arbitrary value into a bool, state the criterion.

## Notes on the verb naming

The convert library's four conversion callees are named `toInt`,
`toFloat`, `toString`, `toBool` (not `int` / `float` / `string` /
`bool`) so they don't collide with the type keywords - the parser
keeps those reserved for declarations (`def x as int ...`). The
`to`-prefixed verb also reads as English at the call site:
`convert.toInt("42")` says "convert to int." `typeOf` is a normal
identifier and is not a type keyword.

Writing the bare form (`int("42")`, `string(42)`, ...) is
a parse error directing you at the `convert.to*` form.

## Objects: `typeOf` vs `objectType`

Some libraries hand back an **opaque object** - a value that carries data
but exposes it only through that library's own accessors, not through
operators or `[index]` / `.field`. The first is
[`json.Value`](json.md) (from `json.decode`). For any such value
`convert.typeOf` returns the generic `"object"`, and `convert.objectType`
returns the specific registered name so you can tell one object family
from another:

```jennifer
def doc as json.Value init json.decode("{}");
io.printf("%s\n", convert.typeOf($doc));       # object
io.printf("%s\n", convert.objectType($doc));   # json.Value
```

`convert.objectType` errors on a non-object, so guard with
`convert.typeOf(v) == "object"` first if the kind is unknown.

See also: [encoding.md](encoding.md) (all other character and binary-to-text codecs), [../user-guide/index.md](../user-guide/index.md), [../technical/interpreter.md](../technical/interpreter.md#builtins-and-libraries), [index.md](index.md).
