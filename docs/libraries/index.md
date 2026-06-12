# Jennifer libraries

Jennifer's standard library is split into topic-based libraries. Each
is enabled explicitly with `use NAME;`; nothing is auto-loaded except
`core`. This page catalogs every library that ships with the
interpreter today and links to the reference doc for each.

> **Looking for one specific function?** See the
> [cheatsheet](cheatsheet.md) - alphabetical list of every builtin
> with its library and a one-line description.

| Library   | Enable with     | Contents                                                                                                                                                                                       | Reference                |
| --------- | --------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------ |
| `io`      | `use io;`       | `io.printf`, `io.sprintf`, `io.readLine`, `io.eof`, plus the format-verb mini-language                                                                                                         | [io.md](io.md)           |
| `convert` | `use convert;`  | `convert.toInt`, `convert.toFloat`, `convert.toString`, `convert.toBool`, `convert.typeOf` - explicit casts; canonical-only `toBool` conversion                                                | [convert.md](convert.md) |
| `math`    | `use math;`     | `math.abs`, `min`, `max`, `sqrt`, `pow`, `floor`, `ceil`, `round`, `rand`, `randInt`, `randSeed`; constants `math.PI`, `math.E`                                                                | [math.md](math.md)       |
| `strings` | `use strings;`  | `strings.upper`, `lower`, `contains`, `startsWith`, `endsWith`, `indexOf`, `trim`, `trimLeft`, `trimRight`, `replace`, `repeat`, `substring`, `split`, `chars`, `join`. `len` lives in `core`. | [strings.md](strings.md) |
| `lists`   | `use lists;`    | `lists.push`, `pop`, `first`, `last`, `head`, `tail`, `reverse`, `sort`, `contains`, `concat`, `slice` - all return a new list.                                                                | [lists.md](lists.md)     |
| `maps`    | `use maps;`     | `maps.keys`, `values`, `has`, `delete`, `merge` - all return a new map / list / bool.                                                                                                          | [maps.md](maps.md)       |
| `os`      | `use os;`       | `os.platform`, `os.getEnv`, `os.JENNIFER_LF`, `os.JENNIFER_OS`                                                                                                                                 | [os.md](os.md)           |
| `core`    | *(auto-loaded)* | `len` (polymorphic over string/list/map), `JENNIFER_VERSION`. The only library that ships bare-name globals; **no** namespaced form (`core.len`) is published, by design.                      | [core.md](core.md)       |

A quick taste:

```jennifer
use io;
use math;
use strings;

io.printf("Jennifer %s\n", JENNIFER_VERSION);   # auto-loaded from core
io.printf("pi is roughly %f\n", math.PI);
io.printf("math.sqrt(2) = %f\n", math.sqrt(2));
io.printf("upper: %s\n", strings.upper("hello"));
io.printf("len: %d\n", len("hello"));           # auto-loaded from core
```

## Namespace-first registration

After M10 every library is **namespaced**: each name is reachable as
`lib.name(...)` (call) or `lib.NAME` (constant). The library's name
doubles as the namespace prefix at the use site. There is no longer
a separate "flat library" category.

The only library that publishes bare-name **globals** is
auto-loaded `core`. Its two globals - `len(...)` and
`JENNIFER_VERSION` - are polymorphic structural primitives that
earn the exemption from the "every call carries its library name"
rule. There is **no** `core.len` / `core.JENNIFER_VERSION`
qualified form: shipping the same name two ways would violate
stance #1 ("one way per thing"). `core` is the only library where
the exposure is asymmetric, and its asymmetry is the whole point -
the auto-loaded library exists precisely so its names can stay
short.

**Rule for library authors:** new libraries always use
`RegisterNamespaced` / `RegisterNamespacedConst`. The
`RegisterGlobal` / `RegisterGlobalConst` family is reserved for
polymorphic structural primitives that genuinely span types; the bar
is intentionally high.

Aliasing (`use lib as alias;`) is a **rename**, not an addition:
after the alias the canonical name no longer resolves at call sites
(it errors with a "did you mean *alias*?" hint). The canonical name
is also freed for use as an ordinary identifier, just like Python's
`import foo as bar`. A library that exposes any global (today: only
`core`) cannot be `use`d twice in the same batch program; that's the
M10 alias-with-globals rule, in practice inert because `core` is
auto-loaded.

## How libraries are organized

The standard library favors many small, focused libraries over a few
large ones. The organizing principle, captured for future extensions:

- Touches I/O (stdin/stdout/files/network/clock) -> `io` (which can
  later split into `fs`, `net`, `time` as it grows).
- Pure value transformation across kinds -> `convert`.
- Pure numeric -> `math` (includes the non-crypto random helpers).
- String manipulation -> `strings`.
- List manipulation -> `lists`.
- Map manipulation -> `maps`.
- Operating-system glue (env, args, host info) -> `os`.
- Polymorphic structural primitives spanning types (`len`,
  `JENNIFER_VERSION`) -> auto-loaded `core` with `RegisterGlobal`.
  Reserve carefully; this is the only escape hatch from the "every
  call carries its library name" rule.
- A genuinely new topic with **five or more** functions / constants
  -> a new library. Fewer than five names fold into the most-related
  existing library (M10 raised the threshold from "3+"; the
  non-crypto random helpers were the first case the new rule
  caught - they live under `math.rand*` rather than getting their
  own library).
- A single function with no clear topic -> the most-related existing
  library.

## Naming convention

Library names look mixed at first glance - `strings` is plural but
`math` is singular. The rule:

- **Plural for count nouns**: when the library operates on instances of
  something you can have multiples of. `strings`, `lists`, `maps`,
  `bytes`, `files`.
- **Singular for mass nouns and conceptual wholes**: `math`, `core`,
  `time` (planned), `regex` (planned).
- **Bare verb when the library is named for what it does**, not what
  it touches: `convert`.
- **Idiomatic abbreviations are fine**: `os`, `fs`, `net`, `regex`.

Three practical constraints reinforce the count/mass rule:

1. Type keywords are reserved. `string`, `int`, `float`, `bool`,
   `list`, `map`, `null` cannot be library names because they
   tokenize as type tokens, not IDENTs. The plural form (`strings`,
   `lists`, `maps`) sidesteps this naturally.
2. The rule matches Go's stdlib: `strings` and `bytes` are plural;
   `math`, `io`, `os` are singular. Since the interpreter is written
   in Go, the convention transfers cleanly to library author
   intuition.
3. Within a library, function names are lowercase / camelCase
   (`upper`, `startsWith`, `typeOf`). Constants are uppercase
   (`PI`, `E`, `JENNIFER_VERSION`).

For implementation notes on how libraries register themselves with the
interpreter (`RegisterNamespaced`, `RegisterGlobal`, the `use`-gated
lookup), see
[../technical/interpreter.md > Builtins and libraries](../technical/interpreter.md#builtins-and-libraries).

For canonical terminology (library vs module, function vs method,
list vs array, ...), see [../glossary.md](../glossary.md). This page
uses the terms in that table.
