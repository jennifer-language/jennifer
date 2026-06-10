# Imports

Two keywords, two mechanisms:

```jennifer
use io;                  # library import - enables `io` library (printf, sprintf)
import "helpers.j";      # file import - splices helpers.j here
```

## Library imports

`use NAME;` enables a built-in module. Standard functions live in
topic-based libraries; nothing is auto-loaded except the special `core`
library. Each library has its own reference doc; the table below is the
index.

| Library   | Enable with      | Contents                                                                                                  | Reference                                  |
|-----------|------------------|-----------------------------------------------------------------------------------------------------------|--------------------------------------------|
| `io`      | `use io;`        | `printf`, `sprintf` and the format-verb mini-language                                                     | [libraries/io.md](../libraries/io.md)       |
| `convert` | `use convert;`   | `int`, `float`, `string`, `bool`, `typeOf` - explicit casts                                               | [libraries/convert.md](../libraries/convert.md) |
| `math`    | `use math;`      | `abs`, `min`, `max`, `sqrt`, `pow`, `floor`, `ceil`, `round`; constants `PI`, `E`                         | [libraries/math.md](../libraries/math.md)   |
| `strings` | `use strings;`   | `upper`, `lower`, `contains`, `startsWith`, `endsWith`, `indexOf`, `trim`, `trimLeft`, `trimRight`, `replace`, `repeat`, `substring`, `split`, `chars`, `join` | [libraries/strings.md](../libraries/strings.md) |
| `lists`   | `use lists;`     | **Namespaced**; non-mutating list helpers (`push`, `pop`, `first`, `last`, `head`, `tail`, `reverse`, `sort`, `contains`, `concat`, `slice`) | [libraries/lists.md](../libraries/lists.md) |
| `maps`    | `use maps;`      | **Namespaced**; non-mutating map helpers (`keys`, `values`, `delete`, `merge`)                                                              | [libraries/maps.md](../libraries/maps.md)   |
| `os`      | `use os;`        | `os.platform`, `os.getEnv`, `os.JENNIFER_LF`, `os.JENNIFER_OS` | [libraries/os.md](../libraries/os.md)       |
| `core`    | *(auto-loaded)*  | `len`, `JENNIFER_VERSION`. No `use` needed; writing `use core;` is a runtime error.                       | [libraries/core.md](../libraries/core.md)   |

See [libraries/index.md](../libraries/index.md) for a fuller catalog and
the library-organization principles, or
[libraries/cheatsheet.md](../libraries/cheatsheet.md) for an
alphabetical lookup of every builtin function and constant.

Quick orientation - if you're reading top to bottom and just want a flavor:

```jennifer
use io;
use convert;
use math;

printf("%s\n", typeOf(5 / 2));         # "float"      [convert]
printf("%d\n", floor(PI * 2.0));       # 6            [math + io]
printf("%s\n", string(true));          # "true"       [convert]
```

The per-library docs cover every function in detail along with error cases.

### Namespaced libraries and aliasing

Jennifer ships two flavours of library. Flat libraries (`io`,
`convert`, `math`, `strings`, the auto-loaded `core`) register
their names at the top level - you call `printf(...)`, `upper(...)`,
reference `PI`. Namespaced libraries register their names behind a
prefix - the prefix is the library's name:

```jennifer
use io;
use os;

printf("on %s\n", os.platform());     # qualified call
printf("OS:  %s\n", os.JENNIFER_OS);  # qualified constant
```

A qualified reference is always `prefix.name(...)` for a call or
`prefix.NAME` for a constant. No spaces around the dot.

#### `use NAME as ALIAS;` aliasing

The optional `as ALIAS` clause renames the namespace at the use
site:

```jennifer
use os as o;

printf("on %s\n", o.platform());      # only o. resolves
printf("on %s\n", os.platform());     # ERROR: did you mean `o`?
```

**Aliasing is only available for namespaced libraries.** It would
be meaningless for a flat library - there's no prefix to rename -
so the interpreter rejects it explicitly:

```jennifer
use math as m;
# ERROR: library "math" has no namespaced builtins;
#        `as m` aliasing is meaningless here
```

If you find yourself wanting to alias a flat library, you're
probably trying to solve a name collision that doesn't actually
exist (flat builtins don't ship duplicate names) - drop the `as`
and use the canonical form.

The rest of the aliasing semantics for namespaced libraries:

- **Rename, not addition.** After `use os as o;` only `o.`
  resolves; `os.foo()` errors with a "did you mean `o`?" hint.
  Matches Python's `import foo as bar` shadowing of `foo`.
- **Canonical name freed.** The aliased canonical name (`os`
  above) is freed for ordinary identifier use - you *could* write
  `func os() { ... }` after `use os as o;`. The
  [style guide](style-guide.md#namespaced-calls) recommends
  against it - it reads as a library call at first glance and
  surprises the reader.
- **Without aliasing, the prefix is reserved.** Bare `use os;`
  reserves `os` as a namespace prefix for the rest of the program;
  `func os() {}` then errors with `shadows imported namespace 'os'`.
- **Repeating an `use` is a silent no-op** in both batch mode and
  the REPL - useful so re-running the same input doesn't produce a
  redefinition error. Pick one form (`use os;` or `use os as o;`)
  per program and stick with it.

## File imports

`import "PATH.j";` textually includes another `.j` source file at the
point of import. The path is a **string literal** that must end in `.j`.
Relative paths resolve from the directory of the file containing the
import; absolute paths and subdirectories work:

```jennifer
import "helpers.j";          # sibling file
import "subdir/utils.j";     # subdirectory
import "../shared/util.j";   # parent dir
import "/abs/path/lib.j";    # absolute path
```

File imports may appear anywhere a statement is allowed, including inside a
block:

```jennifer
use io;
import "helpers.j";          # ← spliced here; whatever helpers.j contains lands here
printf($helper_value);
```

Circular imports (file A imports file B, B imports A) are detected and
rejected with an error.

Mixing the keywords produces a helpful error:

```
import io;           → error: use `use io;` for system libraries
use foo.j;           → error: use `import "foo.j";` for files
import foo.j;        → error: file imports take a string literal: `import "foo.j";`
```

Notes:

- The imported file's contents must be valid where the import appears. A file
  containing a top-level `def` cannot be imported inside a block (since
  definitions are only allowed at the top level).
