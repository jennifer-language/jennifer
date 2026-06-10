# Jennifer libraries

Jennifer's standard library is split into topic-based modules. Each is
enabled explicitly with `use NAME;`; nothing is auto-loaded. This page
catalogs every library that ships with the interpreter today and links
to the reference doc for each.

> **Looking for one specific function?** See the
> [cheatsheet](cheatsheet.md) - alphabetical list of every builtin
> with its library and a one-line description.

| Library   | Enable with     | Contents                                                                                                                                   | Reference                  |
|-----------|-----------------|--------------------------------------------------------------------------------------------------------------------------------------------|----------------------------|
| `io`      | `use io;`       | `printf`, `sprintf`, and a `%d %f %s %t %v %%` format-verb mini-language                                                                   | [io.md](io.md)             |
| `convert` | `use convert;`  | `int`, `float`, `string`, `bool`, `typeOf` - explicit casts; canonical-only `bool` conversion                                              | [convert.md](convert.md)   |
| `math`    | `use math;`     | `abs`, `min`, `max`, `sqrt`, `pow`, `floor`, `ceil`, `round`; constants `PI`, `E`                                                          | [math.md](math.md)         |
| `strings` | `use strings;`  | `upper`, `lower`, `contains`, `startsWith`, `endsWith`, `indexOf`, `trim`, `trimLeft`, `trimRight`, `replace`, `repeat`, `substring`, `split`, `chars`, `join` (`len` lives in [`core`](core.md)) | [strings.md](strings.md)   |
| `lists`   | `use lists;`    | **Namespaced** (`lists.push`, `pop`, `first`, `last`, `head`, `tail`, `reverse`, `sort`, `contains`, `concat`, `slice`). All return a new list.        | [lists.md](lists.md)       |
| `maps`    | `use maps;`     | **Namespaced** (`maps.keys`, `values`, `has`, `delete`, `merge`). All return a new map / list / bool.                                                  | [maps.md](maps.md)         |
| `os`      | `use os;`       | `os.platform`, `os.getEnv`, `os.JENNIFER_LF`, `os.JENNIFER_OS`) | [os.md](os.md) |
| `core`    | *(auto-loaded)* | `len` (polymorphic over string/list/map), `JENNIFER_VERSION`. Pre-imported by the interpreter; writing `use core;` is a runtime error.                | [core.md](core.md)         |

A quick taste:

```jennifer
use io;
use math;
use strings;

printf("Jennifer %s\n", JENNIFER_VERSION);   # auto-loaded from core
printf("pi is roughly %f\n", PI);
printf("sqrt(2) = %f\n", sqrt(2));
printf("upper: %s\n", upper("hello"));
printf("len: %d\n", len("hello"));           # auto-loaded from core
```

## Flat vs namespaced libraries

Jennifer ships two flavours of library, distinguished at registration
time:

- **Essential / flat libraries** (`io`, `convert`, `math`, `strings`,
  auto-loaded `core`). Their builtins are bare names: `printf(...)`,
  `upper(...)`, `PI`. Five small libraries with carefully chosen names,
  collision risk is low, and the bare form keeps everyday programs
  readable.
- **Domain / namespaced libraries**.
  Builtins are addressed as `lib.name(...)` / `lib.NAME`. The
  namespace prevents a domain library from polluting the bare-name
  pool and lets two libraries safely register a common verb
  (`net.parse` and `regex.parse` would not collide).

**Rule for library authors:** if your library could ship a name that
another library might also want (`parse`, `read`, `encode`, `len`),
use the namespaced API. If your library is genuinely essential and
its names are unmistakable (`printf`, `sqrt`), the flat API is fine
- but the bar is high; in doubt, choose namespaced.

Aliasing (`use lib as alias;`) is a **rename**, not an addition:
after the alias the canonical name no longer resolves at call sites
(it errors with a "did you mean *alias*?" hint). The canonical name
is also freed for use as an ordinary identifier in user code, just
like Python's `import foo as bar`.

## How libraries are organized

The standard library favors many small, focused modules over a few large
ones. The organizing principle, captured for future extensions:

- Touches I/O (stdin/stdout/files/network/clock) -> `io` (which can later
  split into `fs`, `net`, `time` as it grows).
- Pure value transformation across kinds -> `convert`.
- Pure numeric -> `math`.
- String manipulation -> `strings`.
- Interpreter introspection (version, host, build info) -> auto-loaded
  `core` (reserve carefully; this is the only escape hatch from the
  "nothing for free" rule).
- A genuinely new topic with three or more functions -> a new library.
- A single function with no clear topic -> the most-related existing
  library.

## Naming convention

Library names look mixed at first glance - `strings` is plural but
`math` is singular. The rule:

- **Plural for count nouns**: when the library operates on instances of
  something you can have multiples of. `strings`, `lists` (planned),
  `maps` (planned), `bytes`, `files`.
- **Singular for mass nouns and conceptual wholes**: `math`, `core`,
  `time` (planned), `regex` (planned).
- **Bare verb when the library is named for what it does**, not what
  it touches: `convert`.
- **Idiomatic abbreviations are fine**: `os`, `fs`, `net`, `regex`.

Three practical constraints reinforce the count/mass rule:

1. Type keywords are reserved. `string`, `int`, `float`, `bool`, `list`,
   `map`, `null` cannot be library names because they tokenize as type
   tokens, not IDENTs. The plural form (`strings`, `lists`, `maps`)
   sidesteps this naturally.
2. The rule matches Go's stdlib: `strings` and `bytes` are plural;
   `math`, `io`, `os` are singular. Since the interpreter is written in
   Go, the convention transfers cleanly to library author intuition.
3. Within a library, function names are lowercase / camelCase
   (`upper`, `startsWith`, `typeOf`). Constants are uppercase
   (`PI`, `E`, `JENNIFER_VERSION`).

For implementation notes on how libraries register themselves with the
interpreter (`Register`, `RegisterConst`, the `use`-gated lookup),
see [../technical/interpreter.md > Builtins and libraries](../technical/interpreter.md#builtins-and-libraries).
