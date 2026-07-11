# Imports

Three keywords, three mechanisms:

```jennifer
use io;                   # library import - enables `io.printf`, `io.sprintf`, ...
include "helpers.j";      # textual file splice - pastes helpers.j here
import "./config.j";      # module import - loads config.j as its own module
```

`use` and `include` operate at the textual level; `import` is the
module system - a real module boundary with run-once initialisation. A
module loads and initialises before the code that imports it (see
[Module imports](#module-imports) below).

## Library imports

`use NAME;` enables a built-in library. Every library is
**namespaced** - every name lives behind the library's prefix
(`io.printf`, `math.sqrt`, `convert.toInt`). Nothing auto-loads;
every program states its imports. Each library has its own
reference doc; the table below is the index.

`len(EXPR)` is a language built-in (not a library) - polymorphic
over string / list / map / bytes; no import needed.

| Library   | Enable with    | Contents                                                                                                                                                                          | Reference                                       |
| --------- | -------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------- |
| `io`      | `use io;`      | `io.printf`, `io.sprintf`, `io.readLine`, `io.eof`, and the format-verb mini-language                                                                                             | [libraries/io.md](../libraries/io.md)           |
| `convert` | `use convert;` | `convert.toInt`, `convert.toFloat`, `convert.toString`, `convert.toBool`, `convert.typeOf` - explicit casts                                                                       | [libraries/convert.md](../libraries/convert.md) |
| `math`    | `use math;`    | `math.abs`, `math.min`, `math.max`, `math.sqrt`, `math.pow`, `math.floor`, `math.ceil`, `math.round`, `math.rand`, `math.randInt`, `math.randSeed`; constants `math.PI`, `math.E` | [libraries/math.md](../libraries/math.md)       |
| `strings` | `use strings;` | `strings.upper`, `lower`, `contains`, `startsWith`, `endsWith`, `indexOf`, `trim`, `trimLeft`, `trimRight`, `replace`, `repeat`, `substring`, `split`, `chars`, `join`            | [libraries/strings.md](../libraries/strings.md) |
| `json`    | `use json;`    | RFC 8259 `json.encode`/`encodePretty`/`decode`; structs and `map of string to V` map to objects, `bytes` to base64, integral numbers decode to `int` else `float`; decode yields generic values (no map-to-struct coercion) | [libraries/json.md](../libraries/json.md)       |
| `toml`    | `use toml;`    | RFC-conformant TOML 1.0 `toml.encode`/`encodePretty`/`decode`; same opaque `toml.Value` + read / walk / write surface as `json` (JSON Pointer), plus `toml.asDatetime` (backed by `time.Time`) for date-times; the config format Jennifer ships, not INI | [libraries/toml.md](../libraries/toml.md)       |
| `lists`   | `use lists;`   | `lists.push`, `pop`, `first`, `last`, `head`, `tail`, `reverse`, `sort`, `contains`, `concat`, `slice`, `shuffle`, `range` - non-mutating helpers                                 | [libraries/lists.md](../libraries/lists.md)     |
| `maps`    | `use maps;`    | `maps.keys`, `values`, `has`, `delete`, `merge` - non-mutating helpers                                                                                                            | [libraries/maps.md](../libraries/maps.md)       |
| `os`      | `use os;`      | `os.getEnv`, `os.hasFlag`, `os.flag`, `os.run`/`spawn`/`wait`/`poll`/`kill`; constants `os.PLATFORM`, `os.ARCH`, `os.EOL`, `os.DIRSEP`, `os.PATHSEP`, `os.ARGS`                    | [libraries/os.md](../libraries/os.md)           |
| `meta`    | `use meta;`    | `meta.VERSION`, `meta.BUILD` - interpreter-self-identity constants                                                                                                                | [libraries/meta.md](../libraries/meta.md)       |
| `time`    | `use time;`    | instants, durations, calendar accessors, fixed-offset zones, strftime format/parse, ISO; structs `time.Time`, `time.Duration`, `time.Zone`; constant `time.UTC`                                                       | [libraries/time.md](../libraries/time.md)       |
| `hash`    | `use hash;`    | `hash.compute(b, algo)` for `"md5"`/`"sha1"`/`"sha256"`; streaming via `hash.stream`/`update`/`finalize`; struct `hash.Stream`                                                                                              | [libraries/hash.md](../libraries/hash.md)       |
| `archive` | `use archive;` | tar / zip containers over `bytes` (no `fs`): `archive.pack`/`unpack` with format `"tar"`/`"zip"`/`"tar.gz"`; bundle is a `list of archive.Entry` `{name, data, mode, mtime}`                                        | [libraries/archive.md](../libraries/archive.md)   |
| `compress`| `use compress;` | byte-stream compression: `pack`/`unpack` for `"gzip"`/`"zlib"`/`"deflate"` (optional `"fast"`/`"default"`/`"best"` level) + streaming (`compress.stream`/`update`/`finalize`); struct `compress.Stream`                | [libraries/compress.md](../libraries/compress.md) |
| `crc`     | `use crc;`     | `crc.compute(b, algo)` for `"crc32"`/`"crc64"` (big-endian bytes); streaming via `crc.stream`/`update`/`finalize`; struct `crc.Stream`                                                                                     | [libraries/crc.md](../libraries/crc.md)         |
| `encoding`| `use encoding;` | `isAscii`/`lenBytes`/`lenRunes` introspection; `toText`/`fromText` for `"hex"`/`"base64"`/`"base64-url"`; `encode`/`decode` for character codecs `"ascii"`/`"iso-8859-1"`/`"windows-1252"`/`"ebcdic"`                       | [libraries/encoding.md](../libraries/encoding.md) |
| `task`    | `use task;`    | observe / join `task of T` handles from `spawn { ... }`. `task.wait`, `task.poll`, `task.discard`, `task.waitAll`, `task.waitAny`                                          | [libraries/task.md](../libraries/task.md)       |
| `fs`      | `use fs;`      | filesystem I/O. Whole-file read/write/append (`readString`/`readBytes`/`writeString`/`writeBytes`/`appendString`/`appendBytes`), metadata (`exists`/`isFile`/`isDir`/`stat`), directory ops (`mkdir`/`mkdirAll`/`remove`/`removeAll`/`rename`/`list`/`walk`), buffered handles (`open`/`readLine`/`readChars`/`readBytes`/`writeString`/`writeBytes`/`eof`/`close`); structs `fs.Stat`, `fs.File` | [libraries/fs.md](../libraries/fs.md) |
| `net`     | `use net;`     | TCP + UDP sockets and DNS. TCP `connect`/`listen`/`accept`/`readBytes`/`writeBytes`/`eof`, UDP `listenUDP`/`sendTo`/`recvFrom`, DNS `lookup`/`reverseLookup`, polymorphic `close`/`address`; structs `net.Conn`, `net.Listener`, `net.UDPSocket`, `net.Datagram`. Supported on the default `jennifer` binary; the constrained `jennifer-tiny` returns friendly errors from every entry point | [libraries/net.md](../libraries/net.md) |
| `regex`   | `use regex;`   | regular expressions over `string` (RE2 syntax). `matches`/`find`/`findAll`/`replace`/`split`/`escape` + `regex.Match` with positional and named captures. Implicit LRU cache for compiled patterns; rune-index offsets in matches | [libraries/regex.md](../libraries/regex.md) |
| `testing` | `use testing;` | test-runner primitives. `run`/`results`/`reset`/`report` + `testing.Result`. Catches runtime errors, throws, and `exit` inside test bodies. Three report formats: `"text"`, `"tap"`, `"junit"`. Foundation for the `.j`-side testing framework | [libraries/testing.md](../libraries/testing.md) |
| `uuid`    | `use uuid;`    | RFC 9562 UUIDs. `uuid.generate("v4")` / `generate("v7")` + `parse`/`isValid`/`version` + constant `NIL`. Version tag is a string arg; RNG is `math`'s seedable source (not crypto-grade)                          | [libraries/uuid.md](../libraries/uuid.md)       |

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
  program a duplicate `use` is accepted as a no-op too. Pick one
  form per program.

#### `len` is a language built-in

`len(EXPR)` is not a library function - it's a reserved keyword
and a language primary expression. Polymorphic over string / list /
map / bytes; no `use` statement needed:

```jennifer
def n as int init len("hello");        # 5 (rune count)
def m as int init len([1, 2, 3]);      # 3 (element count)
```

`len` used to live in an auto-loaded `core` library;
`use core;` now errors with a migration hint. Build-version
constants moved from `core` to `meta`
(`use meta;` then `meta.VERSION`, `meta.BUILD`).

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

## Module imports

`import "PATH.j" [as NAME];` loads another `.j` file as a **module** - a
real boundary, not a textual splice. Where an `include` pastes tokens
into the current scope, an `import` runs the referenced file as its own
program: its top level executes once, in its own scope, before the
importing file's body runs.

```jennifer
use io;
import "./config.j";   # config.j initialises before this line returns
import "./db.j";       # db.j (which may itself import config.j) initialises next

io.printf("app: running\n");
```

The path decides where the module is found by its leading token:

- **Local** - `import "./util.j";` or `import "../shared.j";` resolves
  relative to the importing file's directory.
- **Absolute** - `import "/opt/pkg/m.j";` (leading `/`) is an absolute
  filesystem path.
- **Module** - `import "util.j";` (no `./`, no `/`) is looked up on the
  module search path: the system module directory first, then each
  `-I DIR` passed on the command line. The importing file's own
  directory is **not** consulted for this form. A `/` anywhere but the
  front is an ordinary subdirectory (`import "sub/util.j";` works in
  every form).

Guarantees:

- **Run-once.** A module's top level runs exactly once per program,
  cached by its resolved absolute path. Importing it again returns the
  same initialised module without re-running it.
- **Post-order init.** Imports initialise depth-first: a module is fully
  initialised before any module that imports it. If `main` imports `db`
  and `db` imports `config`, the order is `config`, then `db`, then
  `main`'s body.
- **Acyclic.** An import cycle (`a` imports `b` imports `a`) is a
  load-time error that names every edge in the loop.
- **Load errors are not catchable.** A parse error or a `throw` during a
  module's initialisation fails the program at load. An `import` is a
  declaration, not an expression, so it cannot appear inside a
  `try`/`catch` - wrapping one is itself a parse error.

**A module publishes with `export`; unmarked names are private.** Put
`export` in front of a top-level `def const`, `def struct`, or `func` to
make it reachable through the importer's alias. A forgotten `export` stays
internal (the safe default) - reaching a private name from outside is a
positioned "not exported from module" error. (`export` is meaningful only
in a module: it is a parse error in a `jennifer run` script.)

```jennifer
# config.j
use convert;
export def const MAXCONN as int init 16;   # public
def const NAME as string init "jdb";        # private (no export)
export func describe() {                     # public
    return NAME + " x" + convert.toString(MAXCONN);
}
```

Reaching a module's surface:

- **`alias.fn(args)`** calls an exported function - the arguments are
  evaluated in your code, but the body runs in the module's own scope
  (its constants and other functions, plus whatever *it* imported - `use`
  is not transitive, so the module's `use net;` does not give you `net.*`).
- **`alias.CONST`** reads an exported constant.
- **`alias.Struct`** names an exported struct type, and **`alias.Struct{...}`**
  builds one. A module struct keeps its identity `(module, name)` across the
  boundary, so you can hold one (`def p as alias.Struct init alias.make();`),
  read its fields, and pass it back - while `a.Point` and `b.Point` from two
  modules stay distinct types. An exported struct or function signature may
  not expose a *private* struct (a referential-closure error at the `export`).
- A module top level is **declarations-only**: `def const`, `def struct`,
  `func`, `use`, `import` - no mutable `def` and no free-standing
  statements. A `def const` initializer may still call a private `func`.

```jennifer
use io;
import "./config.j" as config;

io.printf("%s\n", config.describe());   # calls an exported function
io.printf("%d\n", config.MAXCONN);      # reads an exported constant
```

Testing: a co-located `MODULE_test.j` is a white-box overlay - `jennifer
test MODULE_test.j` splices `MODULE.j` in first, so its `test*` methods
reach the module's private names by bare identifier. For black-box tests,
`import` the module and exercise its exported surface. See the runnable
[`examples/modules/`](https://github.com/mplx/jennifer-lang/tree/main/examples/modules)
chain.

### `include` vs `import`

Both read another `.j` file, but they differ at the boundary:

- `include` does a **textual splice** with no module boundary - the
  spliced file's top-level names land directly in the enclosing
  program's scope, and the same file spliced twice contributes its
  definitions twice. Use it to share snippets within one program.
- `import` loads a **module** - separate scope, run-once, post-order
  init. Use it to compose independent files.

Mixing the keywords up produces a positioned, actionable error:

```
include io;     → error: `include` is for files; use `use io;` for
                  system libraries
use foo.j;      → error: `use` is for system libraries; for files use
                  `include "foo.j";`
include foo.j;  → error: file splices take a string literal:
                  `include "foo.j";`
include "foo.go"; → error: include path "foo.go" must end with `.j`
import foo;     → error: `import` takes a quoted module path
                  (`import "foo.j";`); for a system library use `use foo;`
```

Notes:

- The included file's contents must be valid where the include
  appears. A file containing a top-level `def` cannot be included
  inside a block (since definitions are only allowed at the top
  level).
