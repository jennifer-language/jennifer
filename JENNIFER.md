# JENNIFER.md - the Jennifer language, for coding assistants

Drop this file into a project where you write **Jennifer** (`.j` files) and
point your AI coding assistant at it ("we code in Jennifer, see JENNIFER.md").
It is a self-contained reference to the language so an assistant with no prior
knowledge of Jennifer can write correct code. It describes the *language*, not
the interpreter's internals.

Jennifer is a small, interpreted language (tree-walking interpreter written in
Go/TinyGo). Source files use the `.j` extension. Run a program with
`jennifer run program.j`, start a REPL with `jennifer repl`.

**Full documentation** - guides, the complete library reference, and an
alphabetical cheatsheet of every builtin - is hosted at
<https://jennifer-lang.dev/>. If an assistant has web access, fetch
the exact signature of any function there. Source and issues:
<https://github.com/jennifer-language/jennifer>.

> This file mirrors the authoritative spec. If something here conflicts with the
> [hosted docs](https://jennifer-lang.dev/), the docs win - tell the
> maintainer.

---

## The 10 rules that trip people up

Read these first; they are where Jennifer differs from Python/JS/Go and where
an assistant usually guesses wrong:

1. **Variables are referenced with a `$` sigil: `$x`.** But the *declaration*
   uses a bare name: `def x as int init 5;` then use `$x`. Writing `def $x` is
   an error; using bare `x` in an expression is an error.
2. **Constants are referenced bare (no `$`): `MAX`.** They are `UPPER_CASE`.
   Reading one *with* `$` (`$MAX`) is a parse error - the sigil is for
   mutable variables only. A method may not share a name with a top-level
   variable or constant either (`def foo ...; func foo() {}` is rejected).
3. **Method calls are bare and take `()`: `greet()`.** The parser tells a call
   from a constant by the `(`.
4. **`/` is true division and always returns `float`** (like Python 3).
   `5 / 2 == 2.5`. Use `//` for integer/floor division: `5 // 2 == 2`. `%`
   is **floored** to match `//` (`-7 % 3 == 2`, `7 % -3 == -2`). Integer
   arithmetic that overflows `int64` is a runtime error (no silent wrap),
   and a mixed `int`/`float` comparison is exact (no lossy promotion).
5. **Identifiers are letters only, <= 64 chars.** No digits, no underscores in
   variable/method/parameter/library names. `myVar`, not `my_var` or `var2`.
   (Constants are the *only* names that take `_`: `MAX_RETRIES`.)
6. **Statements end with `;`.** Whitespace (including newlines) is
   insignificant everywhere.
7. **Comments are `#` (line) and `/* */` (block, nests).** Not `//` - that is
   the floor-division operator.
8. **No `++`, `--`, `+=`, or any compound assignment.** Only `$x = EXPR;`.
9. **Value semantics: assignment and argument passing copy.** `$b = $a;` then
   mutating `$b` never touches `$a`. Same for lists, maps, structs, bytes.
10. **Logical operators are words: `and`, `or`, `not`** (not `&&`/`||`/`!`).
    `&` `|` `^` `~` are the *bitwise* operators.

---

## Lexical basics

- **Identifiers** (variables, methods, parameters, library names): `[A-Za-z]`,
  <= 64 chars. No digits, no underscores.
- **Constant names**: uppercase chunks joined by single `_`:
  `[A-Z]+(_[A-Z]+)*`. Legal: `MAX`, `MAX_RETRIES`, `HTTP_OK`. Illegal: `_MAX`,
  `MAX_`, `MAX__INT`, `maxInt`.
- **`.j` import paths** are strings and may contain digits, `_`, `/`.
- A leading `#!` line is allowed (shebang): `#!/usr/bin/env -S jennifer run`.

## Types

Primitive: `null`, `int`, `float`, `string`, `bool`, `bytes`.
Compound: `list of T`, `map of K to V`, user `struct`s, `task of T` (a handle
to a `spawn`ed computation).

- **int** literals: `42`, `0xff`, `0o755`, `0b1010`, with `_` digit separators
  (`1_000_000`, `0xDEAD_BEEF`).
- **float** literals: need a `.`: `3.14`, `0.5` (and `_` separators).
- **string** literals: `"..."` or `'...'`; both parse escapes
  `\n \r \t \\ \" \' \0`.
- **bool**: `true`, `false`. **null**: `null`.
- **bytes** has no literal: build with `convert.bytesFromString(s, "utf-8")`
  or append into `def b as bytes;` with `$b[] = 65;`.
- **list** literals: `[1, 2, 3]`, `[]`. Lists are homogeneous (one element
  type).
- **map** literals: `{"a": 1, "b": 2}`, `{}`. Insertion-ordered.
- **struct** literals: `Point{ x: 1, y: 2 }` after
  `def struct Point { x as int, y as int };`. Every field must be named.

## Variables and constants

```jennifer
def x as int;                 # declare, zero value (0)
def y as int init 5;          # declare + initialize
def const MAX as int init 10; # constant, must be initialized, never reassigned
$x = 7;                       # assignment uses the $ sigil
```

- The name at the `def` site is bare (`def x`), never `def $x`.
- `const` is deep: a const list/map/struct rejects mutation at any depth.

## Operators

- Arithmetic: `+  -  *  /  //  %`. `/` is float division; `//` is floor.
- Unary `-` (negation). `+` also concatenates two strings.
- Comparison: `<  >  <=  >=  ==  !=` -> `bool`. `!=` is `not (a == b)`. There is
  no bare `!` (logical negation is the word `not`); a lone `!` is a lex error.
- Logical (words, short-circuit): `and`, `or`, `not`. Operands must be `bool`.
- Bitwise (int only): `&  |  ^  ~  <<  >>`.
- Mixed int/float arithmetic promotes to `float`.
- Precedence, low to high: `or` < `and` < `not` < comparison < `|` < `^` < `&`
  < shifts < `+ -` < `* / // %` < unary `- ~`. So `$x & 0xff == 0` parses as
  `($x & 0xff) == 0`.

## Control flow

```jennifer
if ($n > 0) { ... } elseif ($n < 0) { ... } else { ... }

while ($i < 10) { ... }

for (def i as int init 0; $i < 10; $i = $i + 1) { ... }   # C-style

for (def x in $xs) { ... }     # for-each over a list (elements)
for (def k in $m) { ... }      # for-each over a map (keys, insertion order)

repeat { ... } until ($done);  # post-test loop; body runs at least once

break;      # exit innermost loop
continue;   # next iteration
exit;       # terminate the whole program (exit 0); exit EXPR sets the code
```

Conditions must be `bool` (there is no truthiness). `break`/`continue` do not
cross a method-call or `spawn` boundary.

### Errors

```jennifer
use io;
try {
    throw Error{ kind: "bad", message: "nope", file: "", line: 0, col: 0 };
} catch (e) {
    io.printf("%s\n", $e.message);
}
```

`throw EXPR;` raises any value; convention is the auto-provided `Error` struct
`{kind, message, file, line, col}`. `catch` also catches the runtime errors
builtins raise (out-of-range, missing key, etc.), wrapped into `Error`.
`exit`/`return`/`break`/`continue` are control flow, not catchable.

### Cleanup with `defer`

```jennifer
use fs;
func write(path as string) {
    def f as fs.File init fs.open($path, "write");
    defer fs.close($f);         # runs when the block exits, however it exits
    fs.writeString($f, "data\n");
}
```

`defer CALL(args);` schedules a **call** (method / namespaced / module call - a
non-call is a parse error) to run when the **enclosing block** exits, on every
path (`return`/`break`/`continue`/`throw`/`exit`/fall-through), **LIFO**.
Arguments are evaluated at the `defer` line; the call runs at block exit.
Block-scoped (a `defer` in a loop body runs each iteration); does not cross a
method or `spawn` boundary. A deferred throw propagates and supersedes a pending
error (never an `exit`). There is no `finally`.

`errdefer CALL(args);` is the error-path variant: same form, same LIFO stack,
but the call runs **only when the block exits with a propagating error** (a
`throw` or a runtime error) - skipped on fall-through, `return`, `break`,
`continue`, and `exit`. It is the undo half of an acquire whose resource must
survive on success:

```jennifer
func connect(addr as string) {
    def c as net.Conn init net.connect($addr);
    errdefer net.close($c);     # a failed handshake closes; success keeps it open
    handshake($c);
    return Session{conn: $c};
}
```

## Methods

```jennifer
use io;

func greet(name as string) {
    io.printf("hi %s\n", $name);   # parameters referenced as $name
    return;                        # bare return -> null; or return EXPR;
}

greet("ada");
```

- Bare parameter names (`name as string`), referenced inside as `$name`.
- No return type is declared; the caller's `def x as T init f();` checks it.
- Methods are **top-level only** (not nested). Recursion works.
- Method bodies see global variables/constants. A method may not shadow a
  global name, nor share a name with a builtin from an imported library.
- A program has **no required entry point**: top-level statements run in order.

## Compound types: indexing and iteration

```jennifer
def xs as list of int init [1, 2, 3];
$xs[0];            # read -> 1
$xs[0] = 9;        # write
$xs[] = 4;         # append (write-only; lists and bytes only)

def m as map of string to int init {"a": 1};
$m["a"];           # read (missing key is an error - test with maps.has)
$m["b"] = 2;       # write

def p as Point init Point{ x: 1, y: 2 };
$p.x;              # field read
$p.x = 5;          # field write

$grid[i][j] = v;   # chains nest and mix [index] and .field
```

**Prefer `$xs[]` over `lists.push` in loops.** The `$xs[] = item;` append sugar
(lists and bytes) mutates in place via copy-on-write - amortized O(N) to append
N items. `$xs = lists.push($xs, item)` returns a *new* list each pass and copies
the whole list, so the same loop is O(N^2). Use `$xs[]` to build a list element
by element (a raster, a buffer, a big result set); use `lists.push` only when you
want a fresh list and keep the original.

`len(EXPR)` is a language built-in (not a library): rune count of a string,
element count of a list, entry count of a map, byte count of bytes.

## Concurrency

```jennifer
use task;
def t as task of int init spawn { return expensiveThing(); };
def result as int init task.wait($t);   # also poll / discard / waitAll / waitAny
```

`spawn { ... }` runs concurrently and evaluates to a `task of T`. It deep-copies
its enclosing scope at launch, so there are no shared-memory data races.

## Imports

```jennifer
use io;                 # enable a system library, addressed io.printf(...)
use strings as s;       # alias: only s.upper(...) works after this
include "helpers.j";    # textual splice of another .j file (preprocessor)
import "./util.j" as u; # load util.j as a module, addressed u.fn(...) / u.CONST
```

- `use NAME [as ALIAS];` - system library. Nothing auto-loads; every program
  states its imports. Aliasing is a rename (the canonical prefix stops working).
- `include "path.j";` - textual file splice (path is a string literal ending in
  `.j`, resolved relative to the including file).
- `import "PATH.j" [as NAME];` - **module** import (a real boundary, not a
  splice). Path forms: `./x.j` / `../x.j` local, `/x.j` absolute, bare `x.j`
  from the module search path. Loads once (run-once, cached), depth-first
  post-order; cycles error. Reach the module's surface as `NAME.fn(args)`,
  `NAME.CONST`, and `NAME.Struct` / `NAME.Struct{...}` (`NAME` is the `as`
  alias, else the file stem). A **module top level is declarations-only**:
  `def const`, `def struct`, `func`, `use`, `import` - no mutable `def`, no
  free-standing statements. `use` is not transitive across the boundary.
- `export` publishes a top-level `def const` / `def struct` / `func` from a
  module; unmarked names are private (reaching one from outside errors). A
  module struct type keeps its identity `(module, name)` at the consumer, so
  `def p as NAME.Struct init NAME.make();` type-checks and `a.Point` /
  `b.Point` are distinct. An exported struct/func may not expose a private
  struct. `export` is only valid in a module (a parse error in a `run`
  script). A co-located `MODULE_test.j` white-box overlay runs under
  `jennifer test`.

## Standard library (all namespaced, all opt-in via `use`)

Call as `LIB.name(...)`. Enable with `use LIB;` first. Highlights:

- **`io`** - `printf` / `sprintf` with verbs `%d %f %s %t %v %a` and
  `%verb[|key=value]` modifiers (`pad`, `align`, `base`, `prec`, `sign`,
  `group`, `case`, ...); `readLine`, `eof`, `readBytes`.
- **`convert`** - `toInt toFloat toString toBool`, `typeOf`, `objectType`,
  `bytesFromString` / `stringFromBytes` (utf-8). Note: the callees are
  `toInt` etc. because `int`/`float`/`string`/`bool`/`bytes` are reserved type
  keywords (they appear only after `as`).
- **`math`** - `abs min max sqrt pow floor ceil round rand randInt randSeed`;
  constants `PI`, `E`. Undefined results error (no NaN).
- **`strings`** - `upper lower contains startsWith endsWith indexOf trim
  replace repeat substring split chars join`. Rune-indexed.
- **`lists`** - `push pop first last head tail reverse sort contains concat
  slice shuffle range`. Non-mutating (they return new lists).
- **`maps`** - `keys values has delete merge`. `has` before a missing-key read.
- **`os`** - `getEnv`, `hasFlag`/`flag`, `isTerminal`, `run`/`spawn`;
  `catchSignal(name)`/`gotSignal(name)` to trap and poll a Unix signal
  (`"int"`/`"term"`/`"hup"`/`"usr2"`; cooperative, opt-in, for graceful shutdown;
  `"usr1"` reserved for `kill -USR1` interpreter diagnostics). Constants
  `PLATFORM ARCH EOL DIRSEP PATHSEP ARGS`.
- **`json`** - `encode`/`encodePretty`/`decode`. `decode` returns an opaque
  `json.Value` walked by JSON Pointer accessors (`get`/`asInt`/`asString`/
  `typeOf`/`has`/`keys`/`length`/...) and edited by non-mutating writers
  (`set`/`insert`/`append`/`remove`/`move`, `map()`/`list()`).
- **`toml`** - RFC-conformant TOML 1.0 `encode`/`encodePretty`/`decode` with
  the **same opaque-value, read / walk / write surface as `json`, name for
  name** (JSON Pointer addressing), plus `asDatetime` (backed by `time.Time`)
  for TOML's native date-times. The config format Jennifer ships (not INI).
- **`xml`** - `decode`/`encode`/`encodePretty` over an opaque `xml.Value`,
  designed like `json`/`toml` but an element tree (ordered attributes + children
  + mixed text). Read: `tag`/`text`/`attr`/`hasAttr`/`attrs`/`children`,
  `typeOf`; navigate with an XPath-style path (`name`, `name[k]` 1-based, `*`)
  via `get`/`findAll`/`has`; build with `element`/`setAttr`/`setText`/`append`.
  Entities + numeric refs decode; namespace prefixes kept verbatim.
- **`yaml`** - YAML 1.2 `decode`/`decodeAll`/`encode`/`encodePretty` over an
  opaque `yaml.Value`, the **same opaque-value read / walk / write surface as
  `json`/`toml`, name for name** (JSON Pointer addressing), plus
  `asDatetime`/`isDatetime` for timestamps and `isNull`. `decode` is one
  document (a multi-doc stream errors); `decodeAll` returns a `list of
  yaml.Value`. Anchors / aliases resolve by value and `<<` merge keys apply
  (own key wins). `encode` is flow (compact `{a: 1}`); `encodePretty` is block
  (readable). Backed by `gopkg.in/yaml.v3` (the one library with a Go
  dependency; TinyGo-clean).
- **`intl`** - internationalization: message catalogs + locale-aware
  translation. `intl.load(lang, catalog)` ingests a `map of string to string`
  (first language loaded is the default); `intl.setLocale(lang)` /
  `intl.locale()`; `intl.tr(key)` / `intl.tr(key, params)` translates with
  `{name}` placeholder interpolation (`{{`/`}}` escape a literal brace) and a
  fallback chain (current locale -> its base language -> default language -> the
  key itself, so a missing translation is visible). Named `intl` (letters-only,
  like JS `Intl`), not `i18n`; there is no ambient `_()`.
- **`httpd`** - HTTP/1.1 server engine over `net/http`. Pull loop (no handler
  callbacks): `httpd.listen(addr)` -> `Server`, then loop
  `httpd.accept($srv)` -> `Request` and `httpd.respond($req, status, body)`;
  request accessors `method`/`path`/`query`/`header`/`body`/`remoteAddr`, plus
  `setHeader`/`serveFile`/`serveDir`/`shutdown`. `spawn` several accept loops
  for a worker pool. Default binary only (`jennifer-tiny` stubs it).
- **`term`** - terminal control for interactive TUIs: `term.makeRaw(stream)` ->
  `term.State` and `term.restore(state)` (raw mode: unbuffered, no-echo input),
  `term.size(stream)` -> `term.Size{rows, cols}`, `term.readByte()` -> int (one
  raw byte from stdin, `-1` at EOF; bytes, not decoded keys). Over
  `golang.org/x/term`; default binary only (`jennifer-tiny` stubs it). Refused in
  the REPL. Output-only TUIs need only `ansi` + `os.isTerminal`.
- **Device I/O (Linux-only, default binary; stubs elsewhere and on
  `jennifer-tiny`):** **`serial`** - serial ports (`serial.open(path, baud)` ->
  `serial.Port`, `read`/`write`/`flush`/`close`, `openWith` for full termios
  config). **`spi`** - `spi.open(path)` -> `spi.Device`, `configure(dev, mode,
  speedHz)`, full-duplex `transfer(dev, bytes)`. **`iic`** - the I2C bus
  (`iic.open(path, addr)` -> `iic.Bus`, `read`/`write`/`readReg`/`writeReg`).
  **`gpio`** - `/dev/gpiochipN` lines, pin-keyed `setup`/`read`/`write`/`release`
  + `gpio.IN`/`gpio.OUT` (mirrors the sysfs `gpio` module).
- **`sql`** - relational-database client over `database/sql`: MySQL / MariaDB +
  PostgreSQL (pure-Go drivers; SQLite excluded). `sql.open(driver, dsn)` ->
  `Connection`, `query`/`exec` (target is a Connection or Tx), pull cursor
  `next` + typed `asInt`/`asFloat`/`asString`/`asBool`/`asBytes`/`isNull`,
  `begin`/`commit`/`rollback`, prepared statements. Values bind **only through
  placeholders** (no string interpolation -> injection-safe). Default `jennifer`
  binary only; `jennifer-tiny` stubs it.
- **`time`**, **`fs`**, **`net`**, **`regex`**, **`hash`**, **`crc`**,
  **`crypto`**, **`compress`**, **`archive`**, **`encoding`**, **`uuid`**,
  **`meta`**, **`testing`** - clock, files, sockets, RE2 regex, digests,
  checksums, security primitives (crypto-grade random `crypto.randBytes`/
  `randInt`, constant-time `crypto.hmacEqual`, key derivation `crypto.hkdf`/
  `crypto.pbkdf`, AES-256-GCM `crypto.encrypt`/`decrypt`, Ed25519
  `crypto.signKeypair`/`sign`/`verify`, PEM-key RSA / ECDSA
  `crypto.rsaSign`/`rsaVerify`/`ecdsaSign`/`ecdsaVerify` for JWT RS\* / ES\*, and
  key generation / CSR / JWK `crypto.rsaGenerateKey`/`ecGenerateKey`/`jwkPublic`/
  `csr` for ACME - the RSA / ECDSA parts default binary only), byte-stream + container compression, text/character codecs,
  UUIDs, interpreter identity, and test primitives.

For the exact signature of any function, see the hosted library reference -
the [cheatsheet](https://jennifer-lang.dev/libraries/cheatsheet.html)
(every builtin in one table) or the
[per-library pages](https://jennifer-lang.dev/libraries/index.html)
(e.g. `.../libraries/json.html`).

## Module library (Jennifer-coded, brought in with `import`)

Distributable `.j` modules that ship with Jennifer - ordinary Jennifer source
you can read, fork, or replace, distinct from the Go libraries above. Installed
to the system module dir, so `import "NAME.j";` resolves with no path (or
`import "./NAME.j" as NAME;` for a local copy); addressed `NAME.fn(...)` /
`NAME.Struct` like a library.

- **`acme`** - ACME (RFC 8555) client: obtain / renew TLS certificates from
  Let's Encrypt and compatible CAs. `acme.connect(directoryUrl, accountKey)` /
  `register(client, email)` an account, `order(client, domains)`,
  `authorization(client, authzUrl)` + `challenge(authz, kind)`, compute the
  HTTP-01 `keyAuthorization(client, token)` or DNS-01 `dnsRecord(client, token)`
  (both pure), `accept(client, challengeUrl)` + `pollAuthorization`, then
  `finalize(client, order, csr, ...)` with a `crypto.csr` and
  `downloadCertificate(client, order)`. Every request a JWS (`RS256` / `ES256`)
  over `http` + `json`; keys / CSR / JWK from `crypto`. Test against a CA
  **staging** endpoint first. Needs the default binary.
- **`ansi`** - terminal styling as string wrappers: `ansi.color(s, name)` /
  `bgColor` / `style(s, name)` (bold / dim / italic / underline / reverse) /
  `rgb` / `strip`, plus per-colour and per-style shortcuts (`ansi.red(s)`,
  `ansi.bold(s)`). TTY-aware: styling suppresses itself off a terminal or under
  `NO_COLOR`, and is forced on by `FORCE_COLOR`.
- **`csv`** - RFC 4180: `csv.parse(s)` / `format(rows)` (`parseWith` /
  `formatWith` for any single-character delimiter, e.g. TSV), plus `toRecords` /
  `fromRecords` for header-keyed `map of string to string`. Quoting-aware.
- **`docblock`** - the Jennifer doc-comment format (`/**` ... `*/` with a
  summary, description, and `@param`/`@field`/`@return`/`@throws`/`@since`/
  `@deprecated`/`@see`/`@example`/`@internal`/`@module` tags; types in `{...}`)
  and its parser. `docblock.parse(source)` -> a typed `FileDoc` tree (module
  preamble + per-construct docs + `Diagnostic`s for doc drift / orphans). Data,
  not rendering. Over `regex` + `strings`; both binaries.
- **`feed`** - RSS 2.0 and Atom 1.0 web syndication in one module (build and
  parse; format chosen on `build`, detected on `parse`). Value-semantic
  `feed.Feed { title, link, updated, entries }` of
  `feed.Entry { title, link, id, published, updated, summary, content }` with
  builders `feed.feed(title, link)` / `entry(title, link)` / `add(f, e)` /
  `feedUpdated(f, t)` / `entryId` / `entryPublished` / `entryUpdated` /
  `entrySummary` / `entryContent`; `feed.build(f, "rss"|"atom")` /
  `parse(text)` / `kind(text)`, and `feed.fetch(url)` over the `http` module.
  Over `xml` + `time` (build / parse on both binaries; `fetch` needs the default
  binary). Hardened for untrusted feeds (xml nesting cap, no billion-laughs,
  lenient dates, a 64 MiB body cap).
- **`flatdb`** - a file-backed JSON store over `json` + `fs`. `flatdb.open(path)`
  -> value-semantic `DB` (empty if the file is absent); query / edit by JSON
  Pointer (`get` / `has` / `keys` / `length`; the fresh-`DB`-returning `set` /
  `append` / `remove`); `flatdb.save(db)` writes back with a crash-atomic
  temp+`rename`. Values are `json.Value`s (`json.decode` for scalars). Not a
  database engine - crash-atomic snapshotting of small data. Both binaries.
- **`dotenv`** - read `.env` config files. `dotenv.parse(text)` /
  `dotenv.read(path)` -> `map of string to string`; `dotenv.load(path)` also sets
  each variable via `os.setEnv` (returns the map). Handles `#` comments
  (whole-line + inline on unquoted values), blank lines, a leading `export`,
  single quotes (literal) and double quotes (expand `\n` / `\t` / `\r`). No
  `${VAR}` interpolation. Over `fs` + `strings` + `os`; both binaries.
- **`cron`** - parse and evaluate cron expressions. `cron.parse(expr)` -> a
  `Schedule`; `cron.matches(schedule, t)` -> bool; `cron.next(schedule, after)` ->
  the next `time.Time` at or after a time (keeps its offset). Five fields
  (minute / hour / day-of-month / month / day-of-week 0-7) with `*` / `,` / `-`
  and `/n` steps; when both day fields are restricted a day matching either
  fires. A pure calculator over `time` (no clock) - a scheduler is your own
  `spawn` + `time.sleep` loop. Both binaries.
- **`htmlwriter`** - build an HTML element tree and render escaped HTML5:
  `html.element(tag, attrs, children)` / `text(s)` / `raw(s)` / `attr(n, v)`
  constructors, `render` / `renderAll`, `escape`. A writer, not a parser.
- **`tengine`** - a lightweight-CMS text template engine (a subset of Go
  `text/template`) rendered over a `json.Value` tree. `tengine.newSet()` ->
  `Set`; `tengine.add(set, name, src)` -> `Set` (extracting `{{ define }}`
  blocks); `tengine.render(set, entry, data)` -> string. Addressing: `.a.b`
  (current node), `$` / `$.a.b` (root), `$var` (variable). Actions: value output;
  `{{ if COND }}` / `{{ else if COND }}` / `{{ else }}` with the functions `eq` /
  `ne` / `lt` / `le` / `gt` / `ge` / `and` / `or` / `not` (parenthesised nesting);
  `{{ range .xs }}` / `{{ range $i, $e := .xs }}` and `{{ with }}`, each with
  `else`; `{{ $x := PIPE }}` variables; `{{ define }}` / `{{ template }}` /
  `{{ block }}` layout inheritance; `{{/* comments */}}`; `{{- -}}` trim markers;
  and output pipes `upper` / `lower` / `title` / `trim` / `html` / `urlize` /
  `default` / `truncate` / `join` / `len` / `printf`. Not auto-escaped (use the `html`
  pipe). Over `json` / `strings` / `lists` / `maps` / `convert`; both binaries.
- **`http`** - an HTTP/1.1 client over `net` (`https://` via TLS):
  `http.request(method, url, headers, body)` (method-agnostic; or
  `requestWith(..., timeoutMs)` for an explicit per-read idle timeout - `request`
  and the shortcuts use a 30 s default so a hung server can't block forever,
  `0` disables) plus
  `get(url, headers)` / `post(url, contentType, body, headers)` / `put` /
  `patch` / `delete` / `head` / `options` return an
  `http.Response` (`status` / `statusText` / lowercased `headers` / `body`);
  `http.header(resp, name)` reads a response header case-insensitively. Handles
  Content-Length and chunked framing; text (UTF-8) bodies. Redirects returned,
  not followed. **Default `jennifer` binary only** (`net`).
- **`gotify`** - push a notification to a [Gotify](https://gotify.net) server,
  on top of `http`: `gotify.push(cfg, title, message, priority)` POSTs the
  message form (`X-Gotify-Key` header) to `cfg.url + "/message"` and returns the
  `http.Response` (a bad token is a `4xx` value, not a crash). Value-semantic
  `gotify.Config{url, token}`, caller-supplied. **Default `jennifer` binary
  only** (`net`).
- **`gpio`** - Raspberry-Pi / Linux-SBC GPIO over sysfs (`fs` backend).
  Stateless, pin-keyed: `gpio.setup(pin, "in"/"out")` / `write(pin, 0/1)` /
  `read(pin)` / `release(pin)`. Root `/sys/class/gpio`, overridable via the
  `JENNIFER_GPIO_BASE` env var (`os.setEnv`, e.g. for a mock). Off a
  GPIO-capable host, calls throw `Error{kind: "gpio"}` clearly. Both binaries.
- **`rest`** - an ergonomic REST layer over `http` + `json`: a value-semantic
  `rest.Client{baseUrl, headers}` and `rest.get(c, path, query)` / `post(c,
  path, contentType, body)` / `put` / `patch` / `delete` -> `rest.Response`
  (`status` / `headers` / `body`), plus `getJson` (-> `json.Value`) / `postJson`
  / `putJson` / `patchJson`. Base-URL joining, percent-encoded query strings,
  `rest.bearer` / `rest.basic` / `rest.withHeader` for auth. A 4xx/5xx is a
  `Response` value, not a crash. **Default `jennifer` binary only** (`net`).
- **`oauth`** - a generic OAuth2 client (the *get-a-token* half; `sasl` is the
  *use-a-token* half) over `http` + `json`: `oauth.clientCredentials(cfg)` /
  `refresh(cfg, refreshToken)` / device flow `deviceStart(cfg)` -> `deviceWait(cfg,
  dev)` -> `oauth.Token`. `google` / `microsoft` `Config` presets, `isExpired` +
  `save` / `load` token store; tokens feed `sasl.bearer` for mail XOAUTH2.
  Throws `Error` (kind `"oauth"`) on a token-endpoint error. Auth-Code+PKCE / JWT
  assertion gated on `httpd` / `crypto`. **Default `jennifer` binary only**
  (`net`).
- **`web`** - a small HTTP framework over the `httpd` engine. Register routes
  against handler methods **by name** (`web.get($app, "/users/:id",
  "showUser")` / `post` / `put` / `patch` / `delete` / `route`); patterns take
  `:param` captures and a trailing `*rest` wildcard (`/files/*path`; `/*path` as
  an SPA fallback, registered last), plus
  `web.before` middleware and `web.notFound`; a handler is `func name(ctx as
  web.Context)`, dispatched by `meta.callMain`. `web.Context` helpers:
  `param` / `query` / `method` / `path` / `header` / `body` (bytes) / `bodyJson`
  / `form` / `formValue` / `remoteAddr`, and `text` / `html` / `sendJson` /
  `redirect` / `respond` / `setHeader` / `serveFile` / `serveDir` / `sendGzip`
  (gzip when the client accepts it). **Cookies:**
  `web.cookie($ctx, name)` / `web.setCookie($ctx, name, value, opts)` with a
  `web.CookieOptions` (`path` / `domain` / `maxAge` / `httpOnly` / `secure` /
  `sameSite`). **Sessions:** `web.sessionId($ctx, cookieName)` resolves or mints
  the session-id cookie (a new UUID + `HttpOnly` cookie on first use); `web`
  owns only that cookie, the session store stays the app's (e.g. `session` over
  `memcache`), so `web` forces no store dependency. **CORS:** `web.cors($app,
  opts)` with a `web.CorsOptions` (`allowOrigin` / `allowMethods` /
  `allowHeaders` / `allowCredentials` / `maxAge`) sets an app-wide policy - the
  serve loop adds the `Access-Control-*` headers and answers an `OPTIONS`
  preflight with `204`. **Caching:** `web.serveFile` already sets `ETag` /
  `Last-Modified` for static files; for a dynamic response `web.etag($ctx, tag)`
  sets the `ETag` and answers `304` on a matching `If-None-Match` (returns true
  so the handler stops). **Auth:** `web.basicAuth($ctx)` decodes
  `Authorization: Basic` into `web.BasicCredentials` (`user` / `password` /
  `present`) and `web.bearerToken($ctx)` extracts a bearer token; the credential
  check + `401` challenge stay app code (client-side auth is in `rest`; Digest is
  unsupported). **CSRF:** `web.csrfToken($ctx, secret)` mints an HMAC-signed
  double-submit token (into the `csrf` cookie) and `web.csrfCheck($ctx, secret)`
  validates it (the app owns the secret, opts in via middleware; stateless, no
  session store). `web.run($app, addr)`
  owns the accept loop (`web.serveOn($app, srv)` to hold the server handle).
  Run with `jennifer serve app.j [--watch]`. **Default `jennifer` binary
  only** (`net`).
- **`markdown`** - render a small CommonMark subset (headings, emphasis, links,
  lists, code, GFM tables) to HTML (`markdown.toHtml`, through `htmlwriter`) and
  styled terminal text (`toAnsi`, through `ansi`); author Markdown with
  `header` / `style` / `link` / `bullets` / `numbered` / `codeBlock` / `table`;
  align handcrafted table source with `tablePretty`.
- **`memcache`** - a memcached client over `net` (classic text protocol):
  `memcache.connect(opts)` -> `memcache.Session`, then `set(session, key, value,
  exptime)` / `add` (store-if-absent, `-> bool`) / `get(session, key)` (`""`
  when absent) / `delete` / `incr(session, key, delta)` (new value, `-1` when
  absent) / `decr` / `touch` / `quit`. Every store carries a TTL (`exptime`
  seconds). A volatile cache for sessions / counters / locks. Throws `Error`
  (kind `"memcache"`) on a protocol error. **Default `jennifer` binary only**
  (`net`).
- **`session`** - server-side sessions on the `memcache` module: a `map of
  string to string` under `sess:ID` with a sliding TTL. `session.create(mc,
  ttl)` -> id (UUID v4), `load(mc, id)` (empty map when absent / expired),
  `save(mc, id, data, ttl)`, `touch(mc, id, ttl)`, `destroy(mc, id)`. Threads
  `memcache` + `uuid` + `json`; data is stored base64-wrapped JSON so any UTF-8
  value round-trips. Volatile (a cache, not a store of record). **Default
  `jennifer` binary only** (`net`).
- **`ratelimit`** - a fixed-window rate limiter on the `memcache` module
  (atomic `incr` + per-key TTL): `ratelimit.allow(mc, key, limit, window)` ->
  bool records a hit and reports whether it is within `limit` for the current
  `window` seconds; `ratelimit.remaining(mc, key, limit)` -> int is the budget
  left. The counter is created with the window TTL on the first hit and expires
  on its own. **Default `jennifer` binary only** (`net`).
- **`mime`** - build and parse MIME messages (RFC 5322 headers, multipart,
  quoted-printable / base64 transfer encodings): `mime.text(contentType, body)` /
  `attachment` / `multipart(subtype, boundary, parts)` / `withHeader` build a
  `Part` tree, `mime.encode(part)` serializes it, `mime.parse(text)` reads it
  back, and `mime.headerValue` / `body` / `parts` / `contentType` / `address`
  read it. A non-ASCII `Subject` / display name is auto-encoded as an RFC 2047
  encoded-word on `encode` and decoded on `parse` (primitives `mime.encodeWord`
  / `decodeWord`). Bodies are text (UTF-8); no networking. The foundation the
  mail clients build on.
- **`sasl`** - SASL auth mechanisms shared by the mail clients: base64 encoders
  `sasl.plain(user, pass)`, `sasl.loginUser` / `loginPass`, `sasl.bearer(user,
  token)` (SASL XOAUTH2 - the "use a token" half of OAuth2, for Google /
  Microsoft 365; mail clients select it with `Options.auth = "xoauth2"`, token
  in `pass`), plus challenge-response `sasl.cram(user, pass, challenge)`
  (CRAM-MD5) and SCRAM: `sasl.scramStart(user, algo)` -> `scramClientFirst` ->
  `scramClientFinal(s, serverFirst, pass)` -> `scramFinalToken` -> `scramVerify`
  (SCRAM-SHA-1 / SCRAM-SHA-256, over `hash` + `crypto`). `sasl.negotiate(advertised)`
  picks the strongest of a server's advertised mechanisms (what the mail clients
  use for `auth: "auto"`; `auth: ""` keeps the plain default). No net; both binaries.
- **`screen`** - terminal user interfaces (an explicit `screen`, not a GUI).
  Output-only layer (both binaries): a value-semantic cell `screen.Buffer`
  (`screen.newScreen(rows, cols)`) drawn with `screen.text` / `textColor` /
  `box` / `fill` / `hline` / `vline` / `set`, ANSI control-sequence builders
  (`screen.clear` / `moveTo(x, y)` / `hideCursor` / `enterAlt` / ...), and a
  flicker-free `screen.render(buf)` / `diff(old, new)` paint loop that repaints
  only changed cells. Interactive layer (needs the `term` library, default
  binary): a pure `screen.decodeKey(seq)` -> `screen.Key{name, char}` (arrows,
  nav, F1-F12, ctrl / alt keys) plus `screen.nextKey` / `begin` / `end` / `size`
  over raw mode. Coordinates are 0-based (origin top-left); drawing past an edge
  is clipped, not an error.
- **`semver`** - strict SemVer 2.0.0 over a `Version` struct, package-registry-grade:
  `semver.parse(s)` / `isValid` / `toString`, `compare` / `lt` / `lte` / `eq` /
  `neq` / `gt` / `gte` / `diff`, `isStable` / `isPrerelease`, `incMajor` /
  `incMinor` / `incPatch`, `sort` / `rsort`; `coerce(s)` / `clean(s)` for loose
  tags; plus npm/Composer **range matching**: `satisfies(version, range)` (caret
  `^1.2.0`, tilde `~1.2`, comparators `>=1.0.0 <2.0.0`, OR `^1 || ^3`, hyphen
  `1.2.3 - 2.3.4`, x-ranges `1.x`; prereleases excluded unless a comparator pins
  one at the same major.minor.patch), `maxSatisfying` / `minSatisfying(versions,
  range)`, `minVersion(range)`, `validRange(range)`, and the solver algebra
  `intersects(a, b)` / `subset(inner, outer)` / `gtr` / `ltr` / `outside` / `simplifyRange(versions, range)`, all prerelease-precise.
- **`imap`** - receive mail (IMAP4rev1, RFC 3501) over `net`, a reading subset:
  `imap.connect(opts)` -> `imap.Session`, then `selectMailbox(session, name)`
  (count), `search(session)` (sequence numbers), `fetch(session, n)` (a whole
  message), `logout(session)`, plus `imap.fetchAll(opts, mailbox)`. Handles
  tagged responses and `{N}` literals; messages are strings for `mime.parse`.
  Throws `Error` (kind `"imap"`) on `NO` / `BAD`. **Default `jennifer` binary
  only** (`net`).
- **`idna`** - internationalized domain names: `idna.toAscii(domain)` /
  `idna.toUnicode(domain)` over a Punycode (RFC 3492) core
  (`münchen.de` <-> `xn--mnchen-3ya.de`), plus `idna.isAscii`. Pure Jennifer, no
  net (uses `convert.toCodepoint` / `fromCodepoint`). The mail clients encode
  hosts and SMTP envelope domains through it.
- **`pop`** - receive mail (POP3, RFC 1939) over `net`: `pop.connect(opts)` ->
  `pop.Session`, then `stat` / `count` / `sizes` / `retrieve(session, n)` /
  `deleteMessage(session, n)` / `quit`, plus `pop.fetchAll(opts)` (every
  message, no delete). Retrieved messages are strings for `mime.parse`. Auth per
  `Options.auth`: USER/PASS, APOP (`"apop"`), XOAUTH2, CRAM-MD5, SCRAM-SHA-1 /
  SCRAM-SHA-256, or `"auto"` (strongest offered). Named `pop` because a namespace
  is letters-only (no digit). **Default `jennifer` binary only** (`net`).
- **`redis`** - a Redis client speaking RESP2 over `net`: `redis.connect(opts)`
  -> `redis.Session`, then typed helpers `get` / `set(session, k, v)` /
  `del` / `exists` / `incr` / `decr` / `keys(session, pattern)` / `ping`, plus
  a generic `redis.command(session, args)` returning a raw `redis.Reply`
  (`kind` / `str` / `num` / `items`, walked like a `json.Value`) for any other
  command. `connect` does optional `AUTH` / `SELECT`; a `-ERR` reply throws
  `Error` (kind `"redis"`). **Default `jennifer` binary only** (`net`).
- **`resque`** - background jobs on Redis, wire-compatible with Resque:
  `resque.enqueue(session, queue, class, args)` schedules a job (JSON envelope
  `{"class","args"}` onto `resque:queue:NAME`, queue registered in the
  `resque:queues` set); `resque.reserve(session, queues)` -> `resque.Job`
  (`queue` / `class` / `args`) pops the next job from the first non-empty queue
  in priority order (empty `Job` when drained); plus `queueLength` / `queues` /
  `size` / `fail`. A Ruby-resque / php-resque worker can process the jobs and
  vice versa; the worker's `class`-dispatch loop is user code. `args` is a
  `list of string` (Ruby positional). Built on `redis` + `json`. **Default
  `jennifer` binary only** (`net`).
- **`smtp`** - send mail (SMTP client) over `net`: `smtp.send(opts, from,
  recipients, message)` runs the dialogue (EHLO, optional STARTTLS / implicit
  TLS via `smtp.Options.security`, SASL auth per `Options.auth` - PLAIN / LOGIN /
  XOAUTH2 / CRAM-MD5 / SCRAM-SHA-1 / SCRAM-SHA-256, `MAIL FROM` / `RCPT TO` /
  `DATA`), with `message` built by `mime`. Throws `Error` (kind `"smtp"`) on
  rejection. Uses `net`, so **default `jennifer` binary only**.
- **`totp`** - time-based one-time passwords (RFC 6238 over RFC 4226 HOTP), the
  two-factor codes authenticator apps show. `totp.generate(secret, opts)` /
  `verify(secret, code, opts)` read the clock (`verify` allows a +/-1-step skew);
  `generateAt` / `verifyAt` take an explicit Unix time (deterministic).
  `totp.uri(issuer, account, secret, opts)` builds the `otpauth://` provisioning
  string a QR code encodes. `secret` is base32; a zero-value `totp.Options` is 6
  digits / 30 s / SHA-1, else set `digits` / `period` / `algorithm` (`"sha256"` /
  `"sha512"`). `verify` compares codes constant-time (`crypto.hmacEqual`) so it
  leaks nothing via timing. Over `hash.hmac` + `crypto` + `encoding` + `time`;
  pure, both binaries.
- **`webhook`** - HMAC-signed webhooks (the GitHub `X-Hub-Signature-256`
  convention). `webhook.sign(payload, secret)` -> `"sha256=" + hex HMAC-SHA256`;
  `webhook.verify(payload, signature, secret)` -> bool (constant-time compare,
  never throws) - both pure, both binaries. `webhook.send(url, payload, secret)`
  POSTs the payload as `application/json` with the signature header and returns
  an `http.Response` (**default `jennifer` binary only**, over `http`). Sign /
  verify the raw body bytes, before any parsing. Over `hash.hmac` + `encoding`.
- **`bucket`** - an S3-compatible object-storage client (AWS S3 / MinIO /
  Cloudflare R2 / Backblaze B2), every request AWS Signature V4-signed.
  `bucket.connect(endpoint, region, accessKey, secretKey)` -> a `Client`, then
  `bucket.get` / `put` / `delete` / `listObjects(client, bucket[, key][, body])`
  each return an `http.Response`; `bucket.objectKeys(xml)` pulls the keys out of a
  `listObjects` body. Path-style addressing, configurable endpoint (one module,
  every store). The list op is `listObjects` (not `list`, a reserved keyword).
  `Client.timeout` (ms; `connect` defaults it to 30000, `0` disables) fails a
  hung endpoint instead of blocking forever. Over `hash.hmac` + `hash.compute` +
  `encoding` + `time` + `http`; **default `jennifer` binary only**.
- **`label`** - industrial label printing in a build / render / emit pipeline.
  Build a device-independent `label.Label` in millimetres: `label.new(w, h)`
  then value-semantic `text(label, x, y, opts, content)` (`label.TextOptions`:
  `height` mm / `points` / `rotation` 0-90-180-270 / `bold`) / `barcode(label, x,
  y, type, opts, data)` (linear code128 / ean13 / ean8 / itf / code39 / gs1-128,
  2D datamatrix / qr; ITF is padded to even length, GS1-128 takes `(AI)data`; a
  zero-value `label.BarcodeOptions` = defaults, else set `height` / `moduleWidth`
  (narrow element mm) / `ratio` (wide:narrow for ITF / Code 39) / `checkDigit`
  (`"mod10"` -> cab `+MOD10`) / `errorLevel` (`"L"`/`"M"`/`"Q"`/`"H"`) /
  `hideText`) / `box(label, x, y, w, h, thickness)` /
  `image(label, x, y, name)` (a pre-stored logo by reference) / `quantity(label,
  n)`. `render(label, device)` emits a selectable dialect; build the device with
  `label.zpl(dpi)` (Zebra, raster) or `label.cab()` / `label.cabWith(setup)` (cab
  JScript, mm-native). cab-only print-setup - the `J` job name, `H` heat/speed,
  `O` orientation, and `S` sensor + geometry lines - rides in a `label.CabSetup`
  vendor struct (ZPL ignores it). Build and render are pure (both binaries);
  `send(host, port, rendered)` writes the stream to a printer's raw `:9100` port
  (**default `jennifer` binary only**, `net`).
- **`prometheus`** - Prometheus metrics in two halves. **Exposition** (pure
  text, both binaries): `prometheus.counter(name, help)` / `gauge(name, help)`
  -> `prometheus.Metric`, `observe(metric, labels, value)` records a sample
  (upsert by label set, value-semantic), `render(metrics)` -> the text
  exposition format (`# HELP` / `# TYPE` / sample lines). Strict name / label
  validation and value / HELP escaping; an invalid name throws `Error` (kind
  `"prometheus"`). **Retrieval** (needs the default binary, over `http` +
  `json`): `query(base, promql)` (instant) / `queryRange(base, promql, start,
  end, step)` (range) -> `prometheus.Result` (`resultType` + `series` of
  `metric` label maps and `values` points).
- **`mqtt`** - an MQTT 3.1.1 pub/sub client over `net` (`mqtts` via TLS):
  `mqtt.connect(opts)` -> `mqtt.Client`, then `subscribe(client, topic)` /
  `publish(client, topic, message)` / `publishBytes(client, topic, payload)` at
  QoS 0, blocking `receive(client)` -> `mqtt.Message` (`topic` / `payload`) and
  single-threaded `poll(client, timeoutMs)` -> `list of Message` (0 or 1, via
  `net.setDeadline`), plus `ping` / `disconnect`. Binary packet framing (1-byte
  header, remaining-length varint, length-prefixed payload) is built with the
  bitwise operators and `bytes`; `connect` / `subscribe` throw `Error` (kind
  `"mqtt"`) on refusal. Basics-first (QoS 0; no retained / will / QoS 1-2).
  **Default `jennifer` binary only** (`net`).
- **`log`** - leveled, structured logging. A `log.Logger` carries a minimum
  level (`debug` < `info` < `warn` < `error`), a format (`text` / `logfmt` /
  `json`), and a sink; build one with `log.new(level, format)` (stdout) /
  `toStderr(level, format)` / `toFile(level, format, path)` /
  `toSyslog(level, address, app)`. `log.debug(logger, message, fields)` /
  `info` / `warn` / `error` (and `at(logger, level, message, fields)` for a
  runtime level) render one record - RFC 3339 timestamp, level, message, and a
  `map of string to string` of `fields` - and write it, dropping records below
  the logger's level; values with a space / quote / `=` are quoted in the text /
  logfmt forms. The syslog sink frames each record as an RFC 5424 datagram over
  UDP (facility `user`); console and file sinks work on both binaries, the
  **syslog sink needs the default `jennifer` binary** (`net`). Over `io` / `fs`
  + `json` + `strings` + `time` + `os` (+ `net` for syslog).
- **`ical`** - iCalendar (RFC 5545) build and parse. Build a value-semantic
  `ical.Calendar` of `ical.Event`s: `ical.calendar()` / `calendarWith(prodid)`,
  `event(uid, start, end, summary)` (dates are `time.Time`), then
  `describe(ev, text)` / `locate(ev, place)` / `add(cal, ev)` (each returns a
  fresh copy). `ical.encode(cal) -> string` renders a `VCALENDAR` of `VEVENT`s -
  CRLF lines, RFC 5545-escaped text (`\` `;` `,` newline), long lines folded at
  75 characters, `DTSTAMP` / `DTSTART` / `DTEND` as UTC `DATE-TIME` (`...Z`).
  `ical.parse(text) -> Calendar` unfolds, ignores property parameters, unescapes,
  skips a `VEVENT` with no `DTSTART`, and defaults a missing `DTEND` to the start,
  so `parse(encode(cal))` round-trips. `VEVENT`-only (no `RRULE` / `VALARM` /
  `TZID`; events are UTC). Pure text over `strings` / `lists` + `time`; **both
  binaries**.
- **`vcard`** - vCard (RFC 6350, vCard 4.0) contacts build and parse. Build a
  value-semantic `vcard.Card`: `vcard.card(formattedName)` then
  `withName(c, family, given)` / `withOrg(c, org, title)` / `addEmail(c, email)`
  / `addPhone(c, phone)` / `addAddress(c, vcard.address(street, locality, region,
  postalCode, country))` / `withUrl(c, url)` / `withNote(c, note)` (each returns
  a fresh copy). `vcard.encode(c) -> string` (one `VCARD`) / `encodeAll(cards)`
  writes `VERSION:4.0`, structured `N` / `ADR` / `ORG`, RFC 6350-escaped text,
  and 75-char folding; `vcard.parse(text) -> list of Card` reads one or many
  cards, ignoring property parameters (`;TYPE=work`), so `parse(encode(c))`
  round-trips. A contact subset (no `BDAY` / `PHOTO` / grouping / parameter
  round-trip). Shares the content-line codec with `ical`. Pure text over
  `strings` / `lists`; **both binaries**.
- **`jwt`** - JSON Web Tokens (RFC 7519). `jwt.sign(claims, key, alg)` /
  `verify(token, key, alg)` / `decode(token)` / `header(token)`, claims a
  `json.Value`. Ten algorithms - HMAC `HS256`/`384`/`512`, RSA
  `RS256`/`384`/`512`, ECDSA `ES256`/`384`/`512`, and `EdDSA` (Ed25519); the
  `key` is always `bytes` (HMAC secret / PEM / Ed25519). `verify` pins the
  **expected** algorithm (rejecting algorithm-confusion), enforces `exp` / `nbf`,
  and compares HMACs in constant time; `decode` / `header` read without
  verifying (never authorize on them). Over `crypto` + `hash` + `encoding` +
  `json` + `time`. HS\* / EdDSA on both binaries; RS\* / ES\* need the default
  binary. JWT auth is this module used as a `web.before` middleware, not a
  separate module.
- **`jsonl`** - JSON Lines (JSONL / NDJSON): newline-delimited JSON, one
  independent `json.Value` per line. `jsonl.encode(records)` writes one compact
  JSON value per line (each newline-terminated); `jsonl.decode(text) -> list of
  json.Value` parses each non-blank line (blank / whitespace lines skipped,
  trailing `\r` trimmed), so `decode(encode(rows))` round-trips. Whole-file
  `readFile` / `writeFile` / `appendFile` (append is the growing-log pattern),
  plus a streaming `jsonl.Reader` (`openReader` / `hasMore` / `readRecord` /
  `closeReader`) that reads one record at a time for files too large for memory
  (`readRecord` throws `Error{kind: "jsonl"}` at end - guard with `hasMore`). A
  thin framing layer over `json` + `fs`; **both binaries**.
- **`ipnet`** - IP addresses and CIDR networks, IPv4 and IPv6. `ipnet.parseAddress(s)
  -> Address` (dotted-quad or IPv6 with `::` compression + embedded IPv4),
  `toString(addr)` (canonical, RFC 5952 for IPv6), `version(addr)`,
  `equal(a, b)`; and `ipnet.parse(cidr) -> Network` (host bits zeroed),
  `contains(net, addr) -> bool` (version mismatch is false), `netmask(net)` /
  `broadcast(net)` -> `Address`, `networkString(net)`. An `ipnet.Address` holds
  its raw `octets as bytes` (4 or 16) and `version`; a `Network` pairs a base
  `addr` with a `prefix`. Bitwise subnet math for allow-lists and membership;
  malformed input throws `Error{kind: "ipnet"}`. Pure `.j` over `strings` +
  `convert`; **both binaries**.
- **`ntp`** - a simple SNTP network-time client (RFC 4330 / 5905). `ntp.query(host)
  -> Result` (port 123, 5s timeout) and `ntp.queryWith(address, timeoutMs) ->
  Result` query a server over UDP; `ntp.Result` carries `serverTime as time.Time`,
  `offset as time.Duration` (server minus local clock), and `delay as
  time.Duration` (round-trip). Packs / unpacks the 48-byte NTP packet with `bytes`
  + bitwise ops and converts the NTP epoch (seconds since 1900) through `time`; a
  lost reply times out via a UDP receive deadline (throws `Error{kind: "ntp"}`)
  rather than hanging. Query-only - it measures the offset, it does not discipline
  the clock or run as a daemon. **Default `jennifer` binary only** (`net`).
- **`pdfwriter`** - generate simple PDF documents (text / lines / rectangles) the
  way `htmlwriter` / `label` generate their formats. Value-semantic builders:
  `pdfwriter.document()`, `page(width, height)`, then `text(pg, x, y, font, size,
  str)` / `line(pg, fromX, fromY, toX, toY)` / `rect(pg, x, y, w, h, filled)` /
  `color(pg, red, green, blue)` (each returns a fresh `Page`), `addPage(doc, pg)`,
  and `render(doc) -> bytes`. Document metadata via `info(doc, key, value)` (the
  PDF Info dictionary - `Title` / `Author` / `Subject` / `Keywords` / `Creator` /
  `Producer`, which defaults to "Jennifer pdfwriter"; `pdfDate(t)` formats a PDF
  date). `render` writes the PDF 1.7 object / xref structure by hand with
  FlateDecode-compressed content streams (via `compress`). Standard-14 base fonts
  (an unknown font throws `Error{kind: "pdfwriter"}`); coordinates are PDF points
  (origin bottom-left, y up, ints), colour is 0-255 RGB. Output is **byte-identical**
  (no auto timestamp - opt in via `info` + `pdfDate`), so a rendered PDF is safe to
  assert against a golden; `qpdf`-clean. A writer, not a reader (no embedded fonts
  / images yet). Pure `.j` over `strings` / `lists` / `convert` / `compress` /
  `time`; **both binaries**.
- **`statsd`** - a fire-and-forget StatsD metrics client over UDP. `statsd.client(host)`
  (default port 8125) / `statsd.clientWith(address, prefix)` open a `Client`
  (`socket` + agent `address` + a metric-name `prefix`, "" for none; copies share
  the socket). The verbs each format one `[prefix.]name:value|type` line and send
  one datagram: `count(c, name, value)` / `increment(c, name)` / `decrement(c, name)`
  (counter `c`), `gauge(c, name, value)` (`g`), `timing(c, name, ms)` (`ms`),
  `set(c, name, value)` (`s`); `close(c)` closes the socket. The push counterpart to
  a pull-based scrape - UDP means no reply and no error when no agent is listening
  (metrics, not data you must not lose). Integer counter / gauge values; no sample
  rates or Datadog tags in this version. **Default `jennifer` binary only** (`net`).
- **`password`** - generate, validate, and score passwords against a policy schema.
  `password.schema()` is a strong default (16 chars, all four classes, min 1 each);
  copy-on-write builders `withLength(s, lo, hi)` / `withClasses(s, lo, up, dig, sym)`
  / `withMinimums(s, lo, up, dig, sym)` / `withSymbolSet(s, chars)` /
  `withoutAmbiguous(s)` each return a fresh `Schema`. `generate(schema) -> string`
  produces a conforming password (throws `Error{kind: "password"}` on an infeasible
  schema); `validate(schema, pw) -> Report{valid as bool, reasons as list of string}`
  checks length + per-class minimums (minimums, not a whitelist); `complexity(pw) ->
  Strength{length, classes, poolSize, entropy as float, label}` estimates bits
  (`length * log2(pool)`, banded very weak / weak / reasonable / strong / very strong).
  A disabled class overrides a leftover minimum. Randomness (character choice and
  the final shuffle) is `crypto`-grade, so a generated password is unpredictable
  and safe to mint as a real credential. Pure `.j` over `crypto` / `strings` /
  `convert`; **both binaries**.
- **`influxdb`** - an InfluxDB 1.x time-series client over the `http` module.
  `influxdb.client(url, db)` / `clientWith(url, db, user, password)` open a `Client`.
  Build a `Point` with value-semantic builders: `point(measurement)`, then
  `tag(p, k, v)` / `field(p, k, floatVal)` / `intField(p, k, intVal)` /
  `stringField(p, k, strVal)` / `boolField(p, k, boolVal)` / `at(p, unixNanos)` /
  `atTime(p, t)` (each returns a fresh `Point`; field types are held as pre-rendered
  line-protocol fragments so one point mixes types). `line(p) -> string` renders one
  line-protocol line (throws if no fields); `write(client, points)` posts a
  `list of Point` to `/write` (nanosecond precision, throws `Error{kind: "influxdb"}`
  on failure). `query(client, influxql) -> Result` runs InfluxQL and parses the
  tabular JSON into `Result{series as list of Series}`, each `Series{name, tags as
  map of string to string, columns as list of string, values as list of list of
  string}` with every cell stringified (convert numeric columns yourself). Automatic
  line-protocol escaping; Basic auth. Over `http` + `json` + `time` + `encoding`.
  **Default `jennifer` binary only** (`net`).
- **`slack`** - post to a Slack Incoming Webhook over the `http` module (sibling of
  `gotify` / `discord`). `slack.send(webhookUrl, text)` posts a plain `{"text": ...}`
  message. For a rich message, build a `Message` with `message()` then
  `text(m, s)` / `section(m, markdown)` / `header(m, heading)` / `divider(m)` (each
  returns a fresh `Message`; blocks held as pre-rendered Block Kit JSON fragments),
  and post it with `sendMessage(webhookUrl, m)` (`render(m) -> string` gives the JSON
  payload). All text is JSON-escaped for you. Both `send` / `sendMessage` return the
  `http.Response` (Slack answers 200 "ok"). Over `http` + `json`.
  **Default `jennifer` binary only** (`net`).
- **`discord`** - post to a Discord channel Webhook over the `http` module (sibling of
  `gotify` / `slack`). `discord.send(webhookUrl, content)` posts a plain
  `{"content": ...}` message. For a rich message, build a `Message` with `message()`
  then `content(m, s)` / `embed(m, title, description, color)` (each returns a fresh
  `Message`; `color` is a decimal RGB int; embeds held as pre-rendered JSON
  fragments), and post it with `sendMessage(webhookUrl, m)` (`render(m) -> string`
  gives the JSON). All text is JSON-escaped for you. `send` / `sendMessage` return the
  `http.Response` (Discord answers 204). Over `http` + `json`.
  **Default `jennifer` binary only** (`net`).
- **`telegram`** - a Telegram Bot API client over the `http` module + `json`.
  `telegram.bot(token)` (or `botWith(token, baseUrl)` for a self-hosted API server)
  gives a `Bot`. Send with `sendMessage(bot, chatId, text)` /
  `sendMessageWith(bot, chatId, text, parseMode)` / `sendPhoto(bot, chatId, photo,
  caption)` (photo by URL or file id) / `sendChatAction(bot, chatId, action)`, and
  `getMe(bot) -> User` checks the token. Each returns a parsed struct (`Message` has
  `messageId` / `chatId` / `text` / `date`); an API error `{"ok": false, ...}` throws
  `Error{kind: "telegram"}`. Receive with `getUpdates(bot, offset, timeout) -> list
  of Update` (long-poll `timeout` seconds); it is the stateful loop - advance `offset`
  to the last `updateId + 1` each pass, and check `Update.hasMessage` before reading
  `Update.message`. `chatId` is a 64-bit `int` (channel ids are large / negative).
  Params are form-encoded to `baseUrl/bot<token>/<method>`. **Default `jennifer`
  binary only** (`net`).
- **`websocket`** - an RFC 6455 WebSocket client over `net`. `websocket.connect(url)`
  (or `connectWith(url, timeoutMs)`) does the HTTP Upgrade handshake to a `ws://`
  (plain TCP) or `wss://` (TLS) URL and verifies the server's `Sec-WebSocket-Accept`
  (`base64(SHA1(key + GUID))`), returning a `Conn`. `send(c, text)` /
  `sendBytes(c, data)` write masked frames (client frames must be masked);
  `receive(c) -> Message{kind, text, data}` reads the next message, kind one of
  "text" / "binary" / "close" / "pong" - it transparently answers a ping with a pong
  and reassembles fragmented messages. `ping(c)` and `close(c)` (sends a close frame,
  shuts the socket). Frame length is auto-encoded in the 7 / 16 / 64-bit form; the
  mask and handshake nonce use `crypto`-grade random (RFC 6455 requires a strong
  entropy source for the mask).
  A protocol error or dropped connection throws `Error{kind: "websocket"}`. Client
  only (no server upgrade). **Default `jennifer` binary only** (`net`).
- **`amqp`** - an AMQP 0-9-1 client for RabbitMQ over `net` (the largest protocol
  module). `amqp.connect(amqp.options(host, user, password))` (tweak with
  `withPort` / `withVhost`) runs the full handshake (protocol header,
  `Connection.Start`/`Start-Ok` SASL PLAIN, `Tune`/`Tune-Ok`, `Open`/`Open-Ok`,
  `Channel.Open`) and returns a `Conn`. `declareQueue(c, name, durable) ->
  QueueInfo{name, messageCount, consumerCount}` (a classic queue) or
  `declareQuorumQueue(c, name)` (a replicated, always-durable quorum queue, via
  the `x-queue-type=quorum` argument); `publish(c, exchange, routingKey,
  bytesBody)` / `publishText(c, exchange, routingKey, text)` send method +
  content-header + body frames (exchange "" routes to a queue by name);
  `get(c, queue, autoAck) -> Message{empty, deliveryTag, exchange, routingKey, body}`
  pulls the next message with a synchronous `Basic.Get` (loop until `empty`);
  `ack(c, deliveryTag)`; `close(c)`. All integer / short-string / long-string /
  field-table / frame encoding is hand-built from `bytes` and the bitwise operators.
  Single channel, SASL PLAIN, no TLS, pull (not async delivery). A protocol error
  throws `Error{kind: "amqp"}`. **Default `jennifer` binary only** (`net`).
- **`multipart`** - build and parse `multipart/form-data` (RFC 7578), the file-upload
  counterpart to `mime`. `multipart.field(name, value)` and
  `multipart.file(name, filename, contentType, dataBytes)` build `Part{name,
  filename, contentType, data as bytes}` values; `multipart.build(parts)` (fresh
  random boundary) or `buildWith(parts, boundary)` returns `Built{contentType, body
  as bytes}` (POST `body` with header `Content-Type: contentType`);
  `multipart.parse(contentType, body) -> list of Part` reads it back. `text(part)`
  decodes a field value, `isFile(part)` tests for a filename. Bodies are `bytes` and
  the boundary is matched at `CRLF--boundary`, so binary file content round-trips
  intact. Pure `.j` over `strings` + `bytes`; **both binaries**.
- **`barcode`** - generate scannable barcodes / QR codes as images (the complement to
  `label`, which emits printer commands). `barcode.encode(data, symbology, opts) ->
  Symbol` encodes: `"qr"` (Reed-Solomon over GF(256), EC levels L/M/Q/H via
  `opts.ecLevel`, automatic version 1-10, data-mask scoring, byte mode - any UTF-8) or
  1D `"code128"` / `"code39"` / `"ean13"` / `"ean8"` / `"itf"`. Render a `Symbol` with
  `barcode.svg(sym, opts) -> string`, `barcode.png(sym, opts) -> bytes` (a monochrome
  PNG hand-encoded over `compress` + `crc`, no image library), `barcode.terminal(sym)
  -> string` (Unicode half-blocks, 2D only), or `barcode.matrix(sym) -> list of list
  of bool`. `barcode.defaults()` gives an `Options` (scale / height / quiet / ecLevel /
  foreground / background). The GF(256) / Reed-Solomon math is a private, `include`d
  `barcode_ecc.j`. Pure `.j`; **both binaries**.
- **`bloom`** - a Bloom filter (probabilistic set). `bloom.new(size, hashes) -> Filter`;
  `bloom.add(f, item)` / `bloom.addAll(f, items)` return a fresh filter (value-semantic,
  so `$f = bloom.add($f, x)`); `bloom.mightContain(f, item) -> bool` has no false
  negatives (a member always reports true) but possible false positives. Bits are
  packed into `bytes`; the k positions per item come from double-hashing one SHA-256
  digest (`pos_i = (h1 + i*h2) mod size`). Strings only. Over `hash` + `bytes`;
  **both binaries**.
- **`ringbuffer`** - a fixed-capacity ring buffer of strings (bounded FIFO,
  overwrite-oldest when full). `ringbuffer.new(capacity) -> RingBuffer`;
  `ringbuffer.push(rb, item)` appends (dropping the oldest at capacity),
  `ringbuffer.pop(rb)` removes the oldest - both return a fresh buffer (value-semantic),
  so read the oldest with `ringbuffer.first(rb)` before you `pop` it (a value-semantic
  pop can't return both the item and the new buffer). Plus `last` / `size` / `capacity`
  / `isEmpty` / `isFull` / `toList` (oldest-first). Strings only. Over `lists`;
  **both binaries**.
- **`mikrotik`** - a MikroTik RouterOS API client over `net` (the binary API, not
  SSH). `mikrotik.connect(mikrotik.options(host, user, password))` (or `optionsTLS`
  for api-ssl on 8729) logs in - plaintext for RouterOS 6.43+/v7, with an automatic
  MD5 challenge-response fallback for older routers - and returns a `Session`.
  `talk(s, command, attrs) -> list of map of string to string` sends a command
  (`/interface/print`) with a `map of string to string` of `=key=value` attributes and
  folds each `!re` reply sentence into a row map; `print(s, path)` is read sugar
  (`path + "/print"`); `run(s, command, attrs) -> string` is for add / set / remove
  and returns the `!done`'s `=ret=` (e.g. a new item id). The wire protocol is
  sentence-based (length-prefixed words, zero-length terminator) with RouterOS's
  variable-length length codec, hand-built from `bytes` + the bitwise operators. A
  `!trap` / `!fatal` reply throws `Error{kind: "mikrotik"}`. Over `net` + `hash` (MD5
  fallback) + `encoding`. **Default `jennifer` binary only** (`net`).

Full per-module reference: the hosted
[module docs](https://jennifer-lang.dev/modules/index.html).

## Two complete programs

Hello, with a helper and a loop:

```jennifer
use io;

func fib(n as int) {
    if ($n < 2) { return $n; }
    return fib($n - 1) + fib($n - 2);
}

for (def i as int init 0; $i < 10; $i = $i + 1) {
    io.printf("fib(%d) = %d\n", $i, fib($i));
}
```

Structs, a list, and JSON:

```jennifer
use io;
use json;

def struct User { name as string, age as int };

def users as list of User init [
    User{ name: "ada", age: 36 },
    User{ name: "bob", age: 41 },
];

for (def u in $users) {
    io.printf("%s is %d\n", $u.name, $u.age);
}

io.printf("%s\n", json.encode($users));   # [{"name":"ada","age":36},...]
```

## Common mistakes checklist (for the assistant)

- Referenced a variable without `$`? -> add it (`$x`, not `x`).
- Put `$` on a constant or a `def` name? -> remove it.
- Used `//` for a comment? -> that is floor division; use `#`.
- Expected `5 / 2` to be `2`? -> it is `2.5`; use `//`.
- Used `&&`/`||`/`!`? -> use `and`/`or`/`not`.
- Used a digit or `_` in a variable name? -> not allowed (only constants take
  `_`).
- Used `x += 1` or `x++`? -> write `$x = $x + 1;`.
- Called `int(x)` / `string(x)`? -> use `convert.toInt(x)` / `toString`.
- Read a map key that might be absent? -> guard with `maps.has($m, key)`.
- Forgot `use io;` before `io.printf`? -> every library must be imported.
- Expected a mutated copy to change the original? -> value semantics; it will
  not.
