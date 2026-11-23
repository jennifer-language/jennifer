# Best practices

Heuristics for Jennifer that reads well and ages well. None is enforced by
the language, but the alternative tends to bite later.

## Follow the style guide

Uniform source style is the biggest readability win: when every file uses
the same spacing, brace placement and naming, an unusual line stands out.
Personal styles cost review time and whitespace merge conflicts.

The [Style guide](style-guide.md) gives the rules (spacing, braces, naming,
literal layout); `jennifer fmt` makes any file conform. Run `fmt` on save,
or at least before every commit, so style drops out of review.

## Lint for suspect patterns with `jennifer lint`

`fmt` fixes how code *looks*; `jennifer lint` flags what it *does* that
is legal but probably wrong. Each check has a stable ID; the leading digit
is the group: **L0nn** source errors, **L1nn** correctness, **L2nn**
complexity and style, **L3nn** API lifecycle:

| ID     | Flags                                                                  |
| ------ | ---------------------------------------------------------------------- |
| `L001` | the source could not be tokenized (lex error)                          |
| `L002` | the source could not be parsed (parse error)                           |
| `L003` | an `include` could not be spliced (preprocess error)                   |
| `L004` | a malformed or unknown-ID `# lint-disable` comment                     |
| `L101` | a local variable declared but never used                               |
| `L102` | code after a `return` / `throw` / `exit` / `break` / `continue`        |
| `L103` | an empty `catch` block (an error caught and silently thrown away)      |
| `L104` | a `throw` of something that isn't an `Error` struct                    |
| `L105` | a condition that is always true or always false (`if ($x == $x)`, ...) |
| `L106` | a `use` / `import` whose namespace is never used                       |
| `L107` | an unqualified call to a name that is not a defined function (a typo'd function) |
| `L108` | a bare name read where nothing by that name is defined (a typo'd constant or function value) |
| `L109` | a call to a same-file function, an imported module's function, or a library builtin, with the wrong number of arguments |
| `L110` | a call argument of the wrong type (`strings.upper(5)`, a `bytes` passed to a `string` parameter) |
| `L111` | a value of the wrong type stored in a typed variable, field, or element (`def n as int init "5";`) |
| `L112` | a `ns.name` reference whose namespace has no `use` / `import`          |
| `L113` | a struct / enum-variant literal (same-file or an imported module's) with an unknown or missing field |
| `L114` | a `ns.name` its namespace does not have: a misspelt library function, constant or type (`time.millis(10)`, `math.PII`), or a module member that is not exported |
| `L115` | a `use NAME;` naming a library that does not exist (`use strngs;`)     |
| `L116` | a `{expr}` slot holding only a literal: `["{", "}"]` is one string, not two (write `['{', '}']`) |
| `L201` | a function with too many statements (default over 60)                  |
| `L202` | block nesting deeper than the limit (default over 4 - see below)       |
| `L203` | a source line longer than the 100-column limit                         |
| `L204` | a call (or other non-trivial expression) inside a `{expr}` string slot |
| `L205` | a function that takes, mutates, and returns a `list` / `map` (a quadratic copy-in / copy-out accumulator; not reported when every call in the file, recursive ones included, is `$v = f($v, ...)` inside a function outside a `try`, or passes a literal such as `[]`) |
| `L301` | use of a deprecated API, with what to use instead                      |
| `L302` | use of a removed API, with what to use instead                         |
| `L303` | a malformed / duplicate / unknown `# pragma-jennifer-*` header line    |

The **L0nn source errors are always on**. They report in whatever `--format` you ask for, so a
`--format=json` pipeline gets valid output even when a file is broken.
Everything else (`L1nn`/`L2nn`/`L3nn`) is selectable:

```sh
jennifer lint myprogram.j                 # human-readable, with source carets
jennifer lint --format=json app.j         # JSON array of findings, for editors/CI
jennifer lint --checks=!L201,!L202 app.j  # run everything except the two style checks
jennifer lint -I lib app.j                # resolve bare imports from lib/ too, as `run -I lib`
```

`lint` loads the modules a file `import`s - resolved the way `run` resolves them,
taking the same `--sysmoddir`, `-I` and `--vendor` flags - so a call into a
module with the wrong argument count (`L109`) or argument type (`L110`), a
literal of a module struct with a wrong field (`L113`), or a name the module does
not export (`L114`), is caught before the code runs. The type checks speak
only when a type is certain (a literal, a variable, parameter or field with a
declared type, a library call) and stay quiet about a user function's result,
since functions declare no return type.

The exit code follows `gofmt -l` / `shellcheck`: `0` clean, `1` when
there are findings (a source error counts), `2` only when the *invocation*
itself is broken (bad flags, unreadable file, bad `--checks`), so
`jennifer lint` works as a pre-commit or CI gate.

When a finding is deliberate, silence it in place rather than disabling the
check everywhere; the ID keeps the intent greppable. The
directive sits on the line the finding anchors to: usually the offending
statement, but the `func` line for `L201` and the block-introducer line
for `L202`:

```jennifer
try { risky(); } catch (e) { }   # lint-disable: L103
```

A directive is its own `#` comment, so a few lines have nowhere to put one:
the lines of a multi-line string, and the lines of a `/** ... */` doc
comment (text inside a doc comment is documentation, never a directive).
For those, a directive covers the whole token: put it after the line where a
multi-line string ends (`'; # lint-disable: L203`), or on its own line right
above a doc comment. A long `#` comment can end in its own directive
(`# long note ... # lint-disable: L203`). `L203` also leaves alone a comment
line that cannot be wrapped: a Markdown table row, or one whose overflow is a
single URL or `code span` too long to fit on a line of its own.

A `# lint-disable-file: L201, L202` at the top of a file silences those
IDs file-wide, and a `.jennifer-lint` file at your project root sets
defaults for every run (same `IDS` / `!IDS` format, one direction).
There is no blanket "disable everything": a directive always names the IDs
it turns off. A doubled marker (`## lint-disable: ...`) is a plain comment,
not a directive, for writing *about* a directive. `examples/linting.j`
demonstrates every check and its suppression.

An include fragment (`NAME.inc.j`, a file that is only ever `include`d) linted on
its own is not flagged for the functions, constants and namespaces it takes from
the including file; lint that file to check them.

## Document your public surface with doc comments

Put a `/** ... */` doc comment on every exported construct (each `func`,
`def struct`, `def enum`, `def const`) and an `@module` preamble on the file.
The [`docblock`](../modules/docblock.md) module parses them into structured
data, the source the API reference is generated from, and reports **drift**:
a `@param` naming no real parameter, a parameter with no `@param`, a `@field`
on the wrong construct. Check it in CI and the docs stay in sync with the code.

The shape is a **summary** (the first paragraph), an optional **description** (the
paragraphs after a blank line), then `@`-tags:

```jennifer
/**
 * Parse a duration like "1h30m" into seconds.
 *
 * Accepts `h` / `m` / `s` units in any order; a bare number is seconds. A malformed
 * string is a catchable Error, never a silent zero.
 * @param text {string} the duration to parse
 * @return {int} the total number of seconds
 * @throws {Error} kind "duration" on a malformed string
 */
export func parseDuration(text as string) {
    # ...
}
```

Two habits pay off:

- **Put a blank line between the summary and the description.** The summary is the
  first *paragraph*, not the first line, so an opening sentence may wrap across
  source lines. Without the blank line the whole opening block becomes the
  summary, one long teaser in the generated catalog.
- **Write types in Jennifer syntax, in braces** - `{int}`, `{list of string}`,
  `{map of string to Point}`, `{json.Value}`. There is no `any` pseudo-type; an
  opaque value documents as its named type.

See the [`docblock`](../modules/docblock.md) reference for the full tag set
(`@since` / `@deprecated` / `@example` / `@see` / `@internal` / ...) and every
diagnostic it reports.

## Why 4+ levels of nesting is a code smell

Nested collection types get unreadable fast. A four-level type holding
"per game, per player, per character, per inventory slot, the item name":

```jennifer
def saves as list of list of list of list of string init [
    [[["sword", "shield"], ["bow"]], [["dagger"]]],
    [[["staff", "amulet"]], [[], ["potion", "rope", "torch"]]]
];

# What does this even mean?
$saves[0][1][0][0] = "axe";
```

Three problems:

1. **No names for the dimensions.** Is index 2 "the character" or "the
   inventory slot"? Only the declaration, with its brackets counted, says.
2. **Bug-prone access.** `$saves[0][1][0][0]` is four indices that all
   look the same. An off-by-one or off-by-level error is an out-of-bounds
   runtime error at best; at worst it reads or modifies the wrong slot.
3. **Inflexible.** Adding a fifth dimension (per save slot, per
   timestamp, ...) means rewriting every access site in the program.

The standard fix is a struct (see [Structs](types-and-values.md#structs)).
Without a new type:

- **Wrap access in functions**: `getItem(save, player, character, slot)`
  reads better than four bare brackets and is one place to fix a bug.
- **Flatten with composite keys**: `map of string to string` keyed on
  `"save:0/player:1/char:0/slot:0"` trades index speed for name
  clarity. Better when the structure is sparse anyway.
- **Decompose into parallel simpler structures**: one list of save
  metadata, one map from save-id to inventory, etc.

Rule of thumb: **one level is normal, two is fine, three is uncommon,
four almost always means a missing abstraction.**

## Write for clarity first; tune the hot path separately

Factor code into small, well-named helpers and build data structures the obvious
way: the interpreter elides the copies that would make that expensive (a
read-only parameter is bound by alias, not deep-copied). The few places where the *shape* of the code decides whether a loop is linear or
quadratic - growing a string with `+` in a loop, calling a helper once per element
in a hot path - are collected in [Performance](performance.md), together with how
to find them with `jennifer profile`. Reach for those rewrites only where a profile
points; everywhere else, the readable version is the right version.
