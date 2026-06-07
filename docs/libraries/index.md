# Jennifer libraries

Jennifer's standard library is split into topic-based modules. Each is
enabled explicitly with `use NAME;`; nothing is auto-loaded. This page
catalogs every library that ships with the interpreter today and links
to the reference doc for each.

| Library   | Enable with     | Contents                                                                                                                                   | Reference                  |
|-----------|-----------------|--------------------------------------------------------------------------------------------------------------------------------------------|----------------------------|
| `io`      | `use io;`       | `printf`, `sprintf`, and a `%d %f %s %t %v %%` format-verb mini-language                                                                   | [io.md](io.md)             |
| `convert` | `use convert;`  | `int`, `float`, `string`, `bool`, `typeOf` - explicit casts; canonical-only `bool` conversion                                              | [convert.md](convert.md)   |
| `math`    | `use math;`     | `abs`, `min`, `max`, `sqrt`, `pow`, `floor`, `ceil`, `round`; constants `PI`, `E`                                                          | [math.md](math.md)         |
| `strings` | `use strings;`  | `len`, `upper`, `lower`, `contains`, `startsWith`, `endsWith`, `indexOf`, `trim`, `trimLeft`, `trimRight`, `replace`, `repeat`, `substring` | [strings.md](strings.md)   |
| `core`    | *(auto-loaded)* | `JENNIFER_VERSION` - the interpreter's build version string. Pre-imported; writing `use core;` is a runtime error.                          | [core.md](core.md)         |

A quick taste:

```jennifer
use io;
use math;
use strings;

printf("Jennifer %s\n", JENNIFER_VERSION);   // auto-loaded from core
printf("pi is roughly %f\n", PI);
printf("sqrt(2) = %f\n", sqrt(2));
printf("upper: %s\n", upper("hello"));
```

## How libraries are organized

The standard library favors many small, focused modules over a few large
ones. The organizing principle, captured for future extensions:

- Touches I/O (stdin/stdout/files/network/clock) -> `io` (which can later
  split into `fs`, `net`, `time` as it grows).
- Pure value transformation across kinds -> `convert`.
- Pure numeric -> `math`.
- String manipulation -> `strings`.
- Interpreter introspection (version, host, build info) -> `meta`.
- A genuinely new topic with three or more functions -> a new library.
- A single function with no clear topic -> the most-related existing
  library.

For implementation notes on how libraries register themselves with the
interpreter (`Register`, `RegisterConst`, the `use`-gated lookup),
see [../technical/interpreter.md > Builtins and libraries](../technical/interpreter.md#builtins-and-libraries).
