# `uuid` - generate and parse UUIDs

Enable with `use uuid;`. RFC 9562 UUIDs: version 4 (random) and version 7
(time-ordered), the `8-4-4-4-12` hex form, and parse / validate helpers.
Self-contained and TinyGo-clean.

## Surface

| Call                  | Returns  | Notes                                                                 |
| --------------------- | -------- | --------------------------------------------------------------------- |
| `uuid.v4()`           | `string` | A new random (version 4) UUID.                                        |
| `uuid.v7()`           | `string` | A new time-ordered (version 7) UUID.                                 |
| `uuid.parse(s)`       | `bytes`  | The 16 bytes of a well-formed UUID string; errors on malformed input. |
| `uuid.isValid(s)`     | `bool`   | Whether `s` is a well-formed UUID string.                             |
| `uuid.version(s)`     | `int`    | The version digit (4, 7, ...; `0` for `uuid.NIL`); errors on malformed input. |
| `uuid.NIL`            | `string` | The all-zero UUID `"00000000-0000-0000-0000-000000000000"`.           |

```jennifer
use io;
use uuid;

def id as string init uuid.v4();
io.printf("%s (v%d)\n", $id, uuid.version($id));   # e.g. 524f1d03-...-042736d40bd9 (v4)

if (uuid.isValid($id)) {
    def raw as bytes init uuid.parse($id);         # 16 bytes
}
```

## v4 vs v7

- **`uuid.v4()`** - fully random. Use for opaque identifiers with no ordering.
- **`uuid.v7()`** - a 48-bit millisecond timestamp in the leading bytes, random
  after. Two v7s created later sort lexically after earlier ones, so they
  make good database keys (index locality) while staying globally unique.

```jennifer
def a as string init uuid.v7();
# ... later ...
def b as string init uuid.v7();
# $a < $b  (string comparison reflects creation order)
```

## Randomness

v4/v7 randomness comes from the [`crypto`](crypto.md) library's
crypto-grade source, so a generated UUID is **unguessable** - safe to
use directly as a security token, session key, or bearer id. It is not
seedable (crypto randomness has no reproducible sequence); for a fast,
seedable, deliberately reproducible identifier stream, draw from
[`math`](math.md) instead.

## See also

- [crypto.md](crypto.md) - the crypto-grade random source behind v4 / v7.
- [cheatsheet.md](cheatsheet.md) - every builtin at a glance.
