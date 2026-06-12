# Imports

Two keywords, two mechanisms:

```jennifer
use io;                   # library import - enables `io.printf`, `io.sprintf`, ...
include "helpers.j";      # textual file splice - pastes helpers.j here
```

A third keyword, `import`, is **reserved** for the planned module
system. Writing `import "x.j";` today produces a migration-hint
error pointing at `include`.

## Library imports

`use NAME;` enables a built-in library. Every library is
**namespaced** - every name lives behind the library's prefix
(`io.printf`, `math.sqrt`, `convert.toInt`). The only library exposing
bare-name globals is the auto-loaded `core` (which doesn't need `use`).
Each library has its own reference doc; the table below is the index.

| Library   | Enable with     | Contents                                                                                                                                                                                                | Reference                                       |
| --------- | --------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------- |
| `io`      | `use io;`       | `io.printf`, `io.sprintf`, `io.readLine`, `io.eof`, and the format-verb mini-language                                                                                                                   | [libraries/io.md](../libraries/io.md)           |
| `convert` | `use convert;`  | `convert.toInt`, `convert.toFloat`, `convert.toString`, `convert.toBool`, `convert.typeOf` - explicit casts                                                                                             | [libraries/convert.md](../libraries/convert.md) |
| `math`    | `use math;`     | `math.abs`, `math.min`, `math.max`, `math.sqrt`, `math.pow`, `math.floor`, `math.ceil`, `math.round`, `math.rand`, `math.randInt`, `math.randSeed`; constants `math.PI`, `math.E`                       | [libraries/math.md](../libraries/math.md)       |
| `strings` | `use strings;`  | `strings.upper`, `lower`, `contains`, `startsWith`, `endsWith`, `indexOf`, `trim`, `trimLeft`, `trimRight`, `replace`, `repeat`, `substring`, `split`, `chars`, `join`                                  | [libraries/strings.md](../libraries/strings.md) |
| `lists`   | `use lists;`    | `lists.push`, `pop`, `first`, `last`, `head`, `tail`, `reverse`, `sort`, `contains`, `concat`, `slice` - non-mutating helpers                                                                           | [libraries/lists.md](../libraries/lists.md)     |
| `maps`    | `use maps;`     | `maps.keys`, `values`, `has`, `delete`, `merge` - non-mutating helpers                                                                                                                                  | [libraries/maps.md](../libraries/maps.md)       |
| `os`      | `use os;`       | `os.platform`, `os.getEnv`, `os.JENNIFER_LF`, `os.JENNIFER_OS`                                                                                                                                          | [libraries/os.md](../libraries/os.md)           |
| `core`    | *(auto-loaded)* | `len`, `JENNIFER_VERSION`. Reachable as bare names only - no `core.len` / `core.JENNIFER_VERSION` form exists. The only library that exposes bare-name globals. Writing `use core;` is a runtime error. | [libraries/core.md](../libraries/core.md)       |

See [libraries/index.md](../libraries/index.md) for a fuller catalog
and the library-organization principles, or
[libraries/cheatsheet.md](../libraries/cheatsheet.md) for an
alphabetical lookup of every builtin function and constant.

Quick orientation - if you're reading top to bottom and just want a flavor:

```jennifer
use io;
use convert;
use math;

io.printf("%s\n", convert.typeOf(5 / 2));     # "float"   [convert]
io.printf("%d\n", math.floor(math.PI * 2.0)); # 6         [math + io]
io.printf("%s\n", convert.toString(true));      # "true"    [convert]
```

The per-library docs cover every function in detail along with error cases.

### Namespaced calls and aliasing

A qualified call is always `prefix.name(...)`; a qualified constant
is `prefix.NAME`. No spaces around the dot. The prefix is the library
name by default; an `as ALIAS` clause renames it at the use site:

```jennifer
use os;
use math as m;

io.printf("on %s\n", os.platform());      # canonical prefix
io.printf("pi=%f\n", m.PI);               # aliased prefix
```

#### Aliasing rules

- **Rename, not addition.** After `use os as o;` only `o.` resolves;
  `os.platform()` errors with a "did you mean `o`?" hint. Matches
  Python's `import foo as bar` shadowing of `foo`.
- **Canonical name freed.** The aliased canonical name (`os` above)
  is freed for ordinary identifier use - you *could* write
  `func os() { ... }` after `use os as o;`. The
  [style guide](style-guide.md#namespaced-calls) recommends against
  it - it reads as a library call at first glance and surprises the
  reader.
- **Without aliasing, the prefix is reserved.** Bare `use os;`
  reserves `os` as a namespace prefix for the rest of the program;
  `func os() {}` then errors with `shadows imported namespace 'os'`.
- **Repeating a `use` is a silent no-op in the REPL.** In a batch
  program a duplicate `use` is also accepted as a no-op unless the
  library exposes any global (only `core` does today); for a library
  with globals, a second `use NAME [as ALIAS];` errors with
  `library 'X' already in scope`. Pick one form per program.

#### `core` and global names

`core` is auto-loaded and is the only library that exposes its names
as bare globals. `len(...)` and `JENNIFER_VERSION` are reachable
without any prefix:

```jennifer
def n as int init len("hello");        # 5
io.printf("Jennifer %s\n", JENNIFER_VERSION);
```

There is **no** `core.len` / `core.JENNIFER_VERSION` qualified
form - `core` names are bare-only.

## File splices (`include`)

`include "PATH.j";` textually splices another `.j` source file at the
point of include. The path is a **string literal** that must end in
`.j`. Relative paths resolve from the directory of the file containing
the include; absolute paths and subdirectories work:

```jennifer
include "helpers.j";          # sibling file
include "subdir/utils.j";     # subdirectory
include "../shared/util.j";   # parent dir
include "/abs/path/lib.j";    # absolute path
```

File splices may appear anywhere a statement is allowed, including
inside a block:

```jennifer
use io;
include "helpers.j";          # ← spliced here; whatever helpers.j contains lands here
io.printf($helperValue);
```

Circular includes (file A includes file B, B includes A) are detected
and rejected with an error.

### `include` vs `import`

`include` does a textual splice with no module boundary - the spliced
file's top-level names land directly in the enclosing program's scope.
The `import` keyword is reserved for the planned module system, which
will add real modules with their own namespaces and explicit exports.
Writing `import "foo.j";` today is rejected with a migration-hint
pointing at `include`.

```
import "x.j";   → error: use `include "x.j";` for textual file
                  splicing; the `import` keyword is reserved for
                  the planned module system
include io;     → error: `include` is for files; use `use io;` for
                  system libraries
use foo.j;      → error: `use` is for system libraries; for files use
                  `include "foo.j";`
include foo.j;  → error: file splices take a string literal:
                  `include "foo.j";`
include "foo.go"; → error: include path "foo.go" must end with `.j`
```

Notes:

- The included file's contents must be valid where the include
  appears. A file containing a top-level `def` cannot be included
  inside a block (since definitions are only allowed at the top
  level).
