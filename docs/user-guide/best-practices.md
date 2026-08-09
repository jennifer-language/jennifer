# Best practices

Stylistic guidance for writing Jennifer that reads well, ages well, and
fits the way the language is shaped. Each entry is a heuristic, not a
hard rule - the language won't stop you, but the rule of thumb is
there because the alternative tends to bite later.

## Follow the style guide

The single biggest readability win is uniform source style across a
codebase. When every file uses the same spacing, brace placement, and
naming, the eye learns the shape of well-formed Jennifer and starts
spotting bugs from rhythm alone - a one-off indent or a stray space
becomes a signal. The reverse is also true: every codebase that
tolerates "personal style" eventually pays for it in review friction,
merge conflicts over whitespace, and reader-time spent on the wrong
question ("is this code unusual because it does something unusual, or
just because the author indents differently?"). Pick the agreed style
once, then stop thinking about it.

Jennifer ships its style as both a written spec and an enforcement
tool: read [Style guide](style-guide.md) for the canonical rules
(spacing, braces, naming, literal layout), then run `jennifer fmt` to
make any file conform. Running `fmt` on save - or at minimum before
every commit - is the cheapest habit you can adopt; it removes style
from the list of things you and your reviewers have to think about.

## Lint for suspect patterns with `jennifer lint`

`fmt` fixes how code *looks*; `jennifer lint` flags what it *does* that
is legal but probably wrong. It sits between the formatter and the
parser: the code parses and runs, but something is still worth a second
look. Each check has a stable ID, grouped by concern - the leading digit
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
| `L201` | a method with too many statements (default over 60)                    |
| `L202` | block nesting deeper than the limit (default over 4 - see below)       |
| `L203` | a source line longer than the 100-column limit                         |
| `L301` | use of a deprecated API (reserved, empty until an API is deprecated)   |
| `L302` | use of a removed API (e.g. an old `use core;`)                         |

The **L0nn source errors are always on** - you can't disable "the file
doesn't parse". They report in whatever `--format` you ask for, so a
`--format=json` pipeline gets valid output even when a file is broken.
Everything else (`L1nn`/`L2nn`/`L3nn`) is selectable:

```sh
jennifer lint myprogram.j                 # human-readable, with source carets
jennifer lint --format=json app.j         # JSON array of findings, for editors/CI
jennifer lint --checks=!L201,!L202 app.j  # run everything except the two style checks
```

The exit code follows `gofmt -l` / `shellcheck`: `0` clean, `1` when
there are findings (a source error counts), `2` only when the *invocation*
itself is broken (bad flags, unreadable file, bad `--checks`). That makes
`jennifer lint` a natural pre-commit or CI gate.

When a flag is a deliberate choice, silence it in place rather than
disabling the check everywhere - the ID keeps the intent greppable. The
directive sits on the line the finding anchors to: usually the offending
statement, but the `func` line for `L201` and the block-introducer line
for `L202`:

```jennifer
try { risky(); } catch (e) { }   # lint-disable: L103
```

A `# lint-disable-file: L201, L202` at the top of a file silences those
IDs file-wide, and a `.jennifer-lint` file at your project root sets
defaults for every run (same `IDS` / `!IDS` format, one direction).
There is no blanket "disable everything" - a directive always names the
IDs it turns off, so a reviewer can see exactly what was waved through. A
doubled marker (`## lint-disable: ...`) is just a comment, not a
directive - handy for writing *about* a directive. `examples/linting.j`
demonstrates every check and its suppression.

## Why 4+ levels of nesting is a code smell

The flexibility that lets `list of list of int` hold any shape gets
unreadable fast as you nest deeper. Here's a four-level type holding
"per game, per player, per character, per inventory slot, the item
name":

```jennifer
def saves as list of list of list of list of string init [
    [[["sword", "shield"], ["bow"]], [["dagger"]]],
    [[["staff", "amulet"]], [[], ["potion", "rope", "torch"]]]
];

# What does this even mean?
$saves[0][1][0][0] = "axe";
```

Three problems:

1. **No semantic names for the dimensions.** Is index 2 "the character"
   or "the inventory slot"? You can't tell without going back to read
   the declaration and counting brackets.
2. **Bug-prone access.** `$saves[0][1][0][0]` is four indices that all
   look the same. Off-by-one or off-by-level errors are silent until
   the program either panics or, worse, modifies the wrong slot.
3. **Inflexible.** Adding a fifth dimension (per save slot, per
   timestamp, ...) means rewriting every access site in the program.

The standard fix is a struct or named record (see
[Structs](types-and-values.md#structs)). Other options that work
without introducing a new type:

- **Wrap access in methods**: `getItem(save, player, character, slot)`
  reads better than four bare brackets and gives you one place to fix
  a bug. Internally the function still walks the nested lists, but
  call sites are self-documenting.
- **Flatten with composite keys**: `map of string to string` keyed on
  `"save:0/player:1/char:0/slot:0"` trades index speed for name
  clarity. Better when the structure is sparse anyway.
- **Decompose into parallel simpler structures**: one list of save
  metadata, one map from save-id to inventory, etc.

As a rule of thumb: **one level is normal, two is fine, three is
uncommon, four is almost always a sign there's a missing abstraction.**
