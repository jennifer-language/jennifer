# `os` - operating-system glue

Enable with `use os;`. The `os` library is the first **namespaced**
library in Jennifer: every name lives behind the `os.` prefix
(`os.platform()`, `os.JENNIFER_OS`). Nothing here is reachable as a
bare identifier, so the library cannot collide with user methods or
other libraries' bare names.

```jennifer
use io;
use os;

io.printf("platform: %s\n", os.platform());
io.printf("os tag:   %s\n", os.JENNIFER_OS);
io.printf("line:     %s",   os.JENNIFER_LF);
```

Currently ships a deliberately tiny slice so the language can demonstrate
namespaced calls, namespaced constants, and `use os as ALIAS;`
aliasing end-to-end. The library expands in
[M15.1](../milestones.md#m151---os) (`os.args` and the rest of the
`JENNIFER_*` constants). Process exit is the language statement
`exit EXPR;`, not an `os` function.

## Functions

| Call              | Returns | Notes                                                                 |
| ----------------- | ------- | --------------------------------------------------------------------- |
| `os.platform()`   | string  | Operating-system name as reported by the runtime (`"linux"` today).   |
| `os.getEnv(name)` | string  | Reads an environment variable. Unset variables return `""`, no error. |

## Constants

| Name             | Kind   | Value                                                                                                   |
| ---------------- | ------ | ------------------------------------------------------------------------------------------------------- |
| `os.JENNIFER_LF` | string | Platform line ending. `"\n"` on Linux today; future cross-platform builds may emit `"\r\n"` on Windows. |
| `os.JENNIFER_OS` | string | OS tag; same value as `os.platform()`, available as a constant for compile-time-style branching.        |

See also: [../user-guide/index.md](../user-guide/index.md), [../user-guide/imports.md](../user-guide/imports.md), [../user-guide/style-guide.md](../user-guide/style-guide.md), [index.md](index.md).
