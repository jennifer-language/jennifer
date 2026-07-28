# Jennifer style guide

This is the recommended source style for Jennifer programs. `jennifer fmt`
re-emits source in exactly this shape, so anything you write that matches
the spec will survive a `fmt` round-trip unchanged.

The guide is short on purpose: there are only a handful of rules, but they
are consistent. If you've used `gofmt`, `prettier`, `rustfmt`, or PSR-12,
nothing here will surprise you.

## Spacing

- **One space around every binary operator**: `$i = 1 + 2;`, not
  `$i=1+2;`. Applies to `+ - * / // % < > <= >= == != and or` and `=`.
- **Unary `-` hugs its operand**: `-5`, `-$x`, `-fact($n - 1)`. No space
  between the `-` and the value it negates.
- **Word-form unary operators take a space**: `not $ok`, never `not$ok`.
  Same goes for any other keyword operator the language grows.
- **One space after `,` and `;` inside `for (...; ...; ...)`**, never
  before. `for (def i as int init 0; $i < 10; $i = $i + 1)`.
- **No space inside parentheses**: `io.printf("hi")`, not `io.printf( "hi" )`.
- **One space between a keyword and its `(`**: `if (cond)`, `while (cond)`,
  `for (...)`. Function calls don't get this space: `io.printf(...)`.
- **No space inside `[ ]` or `{ }` literals**: `[1, 2, 3]`,
  `{"a": 1, "b": 2}`, `Point{x: 1, y: 2}`, not `[ 1, 2, 3 ]` /
  `{ "a" : 1 }` / `Point{ x: 1 }`. This is uniform - list, map, and
  struct / enum literals are all tight (a struct literal also binds
  the brace to its type name: `Point{`, not `Point {`, see
  [Braces](#braces)). Empty literals are `[]`, `{}`, `Name{}`. Same
  rule as `()`.
- **No space before `[`**: `$xs[0]`, not `$xs [0]`. Index expressions
  hug their target.
- **No space inside `[]` for the append form**: `$xs[] = item;`,
  never `$xs[ ] = item;` or `$xs [ ] = item;`. Same rule as `$xs[0]`
  hugs its target and its brackets.
- **One space after `:`** in map literals, never before: `{"a": 1}`,
  not `{"a" :1}` or `{"a":1}`.
- **No trailing whitespace** on any line.

## Indentation

- **4 spaces per level**, no tabs. Re-indenting on `}` always lands you
  back on a multiple of 4.
- One level per block - method body, `if`/`elseif`/`else` body, `while`
  body, `for` body.

## Braces

- **1TBS (one true brace style): opening brace on the same line**
  for *everything* - methods and control flow alike - separated by
  one space: `func fact(n as int) {`, `if ($x > 0) {`, `else {`.
  (Jennifer uses the uniformly-same-line variant that Java, Go,
  Rust, and the Linux kernel also use.)
- **`}` on its own line**, except for the `} else {` / `} elseif (...) {`
  cascade, where `else`/`elseif` continues on the same line as the
  preceding `}`.
- **`fmt` always expands blocks across multiple lines** - the canonical
  form has the opening brace at end-of-line, each body statement on its
  own indented line, and the closing brace on its own line. Applies
  uniformly to method bodies, control-flow blocks (`if / elseif / else`,
  `while`, `for`, `repeat`, `match`), `try { } catch (e) { }`, and
  `spawn { }` block expressions. Single-line blocks are still legal source
  (the parser accepts them), but `fmt` rewrites them to the expanded form
  for consistency.
- **Struct and enum declarations expand to one member per line.**
  `def struct Point { x as int, y as int };` reflows to

  ```jennifer
  def struct Point {
      x as int,
      y as int
  };
  ```

  and `def enum Shape { Circle, Square };` reflows the same way, one variant
  per line. A payload-carrying variant keeps its payload tight on the variant's
  line (`Circle{r as int},` - the brace binds to the variant name, like a
  struct literal):

  ```jennifer
  def enum Shape {
      Circle{r as int},
      Square{side as int},
      Empty
  };
  ```


- **Struct / enum literals bind the brace to the type name, tight**:
  `Point{x: 1, y: 2}` - no space before `{` (so the brace reads as bound to
  `Point`, not a block) and no space inside it. Bodies are tight just like
  map and list literals (`{k: v}`, `[1, 2]`) - one uniform rule, no
  exceptions. An empty literal is `Name{}`.
- **Long literals wrap to one element per line.** A struct / enum / map /
  list literal stays inline while it fits; once its single-line form would
  pass the 100-column limit (or a struct / map literal has more than a
  handful of members), `fmt` breaks it with the opening bracket at
  end-of-line, one element per indented line, and the closing bracket on
  its own line:

  ```jennifer
  def rule as FirewallRule init FirewallRule{
      chain: "forward",
      action: "drop",
      comment: "block inbound"
  };
  ```

  Each container decides independently, so a wrapped list of short maps
  keeps each map inline on its own line. A list of scalars wraps on width
  only (a long row of short numbers reads fine on one line).
- **Tail keywords cuddle the preceding `}`.** `} else { ... }`,
  `} elseif (cond) { ... }`, `} catch (e) { ... }`, and
  `} until (cond);` all keep the trailing keyword on the same line as
  the closing brace. `};` (a struct-decl terminator) also cuddles.
- **`match` arms each start on their own line - they do *not* cuddle.** A
  `match` is a flat list of peer arms (a `switch` / `case`, not a nested
  conditional), so - like `switch`/`case`/`when` in C, Go, Rust, Swift, Kotlin,
  Ruby, and Python - each `when` (and the `else`) begins its own line at the arm
  indent. An arm holding a **single short statement stays inline**
  (`when "idle" { begin(); }`); an arm with two or more statements, or one that
  overflows the line, expands one statement per indented line with its `}` on its
  own line. This is the one place `else` starts a line rather than cuddling a
  `}`: the `} else {` cuddle rule is for an `if`'s conditional tail, and a
  `match`'s `else` is a case-list arm, not that. Reading down the `when` column
  is the point. A `when` value list that overflows the line wraps with each
  continuation value aligned under the first:

  ```jennifer
  match ($state) {
      when "idle" { begin(); }
      when longEventName1(),
           longEventName2() {
          prepare();
          handle();
      }
      else { reject(); }
  }
  ```

## Line length

- **Soft limit: 100 columns.** `fmt` keeps lines under 100 columns where it
  can without changing meaning. It has two levers: it **wraps a literal**
  (struct / enum / map / list) to one element per line (see
  [Braces](#braces)), and it **breaks a binary-operator chain** (`+`, `and`,
  `or`) after the operator. The operator break fills the line first, then
  hangs the operator at end-of-line and indents the continuation one level
  deeper - so it breaks at the last joiner whose next operand would still fit,
  not the first one past the limit:

  ```jennifer
  def body as string init "line one\r\n" +
      "line two\r\n" +
      "line three\r\n";
  ```

  Source-level line breaks at these joiners are also preserved even when the
  line would fit under 100 - so the shape above survives a `fmt` round-trip
  byte-for-byte. What `fmt` will **not** guess at: a long argument list or a
  deeply nested call has no safe break point, so it stays on one line, and a
  single token longer than the limit (a long string literal, a URL) stays
  over it - break those by hand. Because `fmt` never *introduces* an
  over-long line, its output does not add [`lint`](tooling.md) `L203`
  (`line-too-long`) findings that the source did not already have.

## Statements

- **Every statement ends with `;`** - no exceptions, including the last
  statement in a block.
- **One statement per line**. Don't chain multiple statements with `;`
  on a single line.
- **Blank lines separate logical groups** - imports from method
  definitions, methods from top-level code, distinct steps within a long
  block. Never more than one consecutive blank line.

## Loops

- **Declare the iterator variable inside the `for` init**, not in the
  surrounding scope. The variable's lifetime should match the loop's:

  ```jennifer
  for (def i as int init 0; $i < 10; $i = $i + 1) {   # preferred
      io.printf("%d\n", $i);
  }
  ```

  not

  ```jennifer
  def i as int;
  for ($i = 0; $i < 10; $i = $i + 1) {                # avoid
      io.printf("%d\n", $i);
  }
  ```

  The loop-local form is self-contained (reading the `for` line tells
  you everything about `i`), keeps the iterator out of the surrounding
  scope, and matches the for-each shape (`for (def x in $coll)`) which
  is *always* loop-local. The outer-scope form is only justified when
  you genuinely need the iterator's value after the loop ends - for
  example, to report which iteration triggered a break in a future
  language version that adds `break`. Use it deliberately, not by
  habit.
- **One concern per loop.** If the body is more than a screen,
  consider whether the work belongs in a helper method called from
  inside the loop.

## Names

- **Variables, methods, parameters**: lowercase or `camelCase` if the name
  has multiple words.
- **Structs, enums, and enum variants**: `PascalCase` - `Point`, `Shape`,
  `Circle`. The interpreter does **not** enforce this (a name resolves by what
  it refers to, not by how it is capitalized), so it is a convention, not a
  rule. Following it keeps types visually distinct from `camelCase` values and
  `UPPERCASE` constants, and sidesteps the one real collision: an
  all-`UPPERCASE` type name would be read as a *constant* in `Name.member`
  position, so never name a type in all caps.
- **Constants**: `UPPERCASE`, with `_` as a single word separator. The
  full rule is `[A-Z]+(_[A-Z]+)*`, up to 64 characters: one or more
  uppercase chunks joined by single `_`. Every `_` must be immediately
  followed by `[A-Z]` - no leading, trailing, or consecutive `_`.
  Examples: `MAX`, `MAX_RETRIES`, `HTTP_OK`, `A_B_C`. Digits and
  lowercase letters are not allowed.
- **Library names**: lowercase, single word where possible (`io`, `math`,
  `strings`, `meta`).

## Namespaced calls

Domain libraries are addressed by `prefix.name(...)`. The dot binds
tight on both sides, like a method call's `(`:

- **No space around `.`**: `os.platform()`, never `os . platform()`.
- **The call parens still hug the callee**: `os.platform()`, not
  `os.platform ()`.
- **`use lib as alias;` is one space on each side of `as`**:
  `use bio as b;`, never `use bio  as  b;`.

When you alias a library, the canonical name is freed for ordinary
identifier use (e.g. you *could* write `func os() { ... }` after
`use os as o;`). Don't. Reusing a library's canonical name reads as
"this is a call into the library" at first glance, then surprises
the reader when it isn't - keep the canonical name out of the
user-method pool even when aliasing has technically freed it.

## Strings

- **Prefer double quotes**: `"hello"` over `'hello'`. Both forms parse
  escape sequences the same way, but mixing styles in one file reads as
  noise. Use single quotes only when the string contains a `"` you'd
  otherwise need to escape. This is a human guideline: **`fmt` preserves a
  string literal's exact source spelling** - its quotes, its escape choices,
  and any line breaks inside it - rather than rewriting them, so a
  deliberately single-quoted or multi-line string survives a format
  untouched (a formatter must not silently alter a literal's value or shape).
- **Escape sequences are explicit**: `"\n"`, `"\t"`, `"\\"`. A string
  literal *may* span multiple lines (a raw newline between the quotes is
  part of the value), which is handy for an embedded template or sample; `fmt`
  keeps it as written rather than collapsing it to `\n`.

## Comments

- `# line comment` for short notes that belong on or just above the
  thing they describe. The very first line may be a shebang
  (`#!/usr/bin/env -S jennifer run`); the lexer treats it as a comment.
- `/* block comment */` for longer commentary that doesn't fit one line.
  Block comments nest, so wrapping a chunk of code that already
  contains a block comment in another `/* ... */` works.
- **Inline block comments inside `(`, `[`, or after `.` get a space on
  the operand side.** `printf(/* note */ $x)`, not
  `printf(/* note */$x)`. This is a deliberate exception to the
  "no space inside `()`" rule above: `*/$x` runs together visually
  and is harder to read than the spaced form. The comment hugs the
  opening delimiter (no space between `(` and `/*`) so only the
  operand side picks up the space. Same rule for `[/* note */ 0]`
  and `$obj./* doc */ field`.
- Comments explain *why*, not *what*. The code already says what.

## Doc comments

**Document every public `func`, `def struct`, and `def const` with a doc
comment**, and open a file with a module preamble. A doc comment is a block
comment that opens with exactly `/**` (a plain `/*` stays an ordinary
comment), sits immediately above the construct it documents, and holds a
summary line, an optional description, and `@`-tags. This is the format the
[`docblock`](../modules/docblock.md) module parses, so your docs are
machine-readable, not just prose.

```jennifer
/**
 * Distance between two points.
 * A longer description can follow the summary line.
 * @param ax {float} first x coordinate
 * @param ay {float} first y coordinate
 * @return {float} the Euclidean distance
 * @since 0.9
 */
export func distance(ax as float, ay as float, bx as float, by as float) { ... }
```

- **Types go in `{ }`, in Jennifer's own syntax**: `{int}`, `{list of int}`,
  `{map of string to int}`, `{json.Value}`. There is no `any` - an opaque
  value documents as `json.Value` or a named struct.
- **`export` is read from the keyword**, never a tag - don't write `@public`.
- **Tags**: `@param name {type} desc` (functions) and `@field name {type} desc`
  (structs), one per parameter / field; `@return {type} desc`;
  `@throws {type} desc`; and the universals `@since`, `@deprecated [reason]`,
  `@see`, `@example`, `@internal`.
- **A file preamble** is a doc comment carrying `@module name`, plus optional
  `@author`, `@version`, `@license`. It goes at the top, after the SPDX header.
- **Keep doc names in step with the code.** `docblock` cross-checks `@param` /
  `@field` names against the real declaration and reports drift, so a stale doc
  is a caught bug, not a silent one.

`jennifer fmt` preserves doc comments and keeps each on its own line above its
construct, so a formatted file is exactly what `docblock` expects to parse.

## SPDX header

Every committed `.j` file opens with a two-line [SPDX](https://spdx.dev/) header -
a machine-readable license tag and a copyright line - so license-scanning and
[REUSE](https://reuse.software/) tooling can attribute each file without parsing
prose:

```jennifer
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
```

- **`SPDX-License-Identifier`** is the [SPDX license
  expression](https://spdx.org/licenses/) for the file. The Jennifer project
  itself uses `LGPL-3.0-only`; your own program carries whatever license you
  ship under.
- **`SPDX-FileCopyrightText`** is the copyright notice, kept behind its SPDX tag
  (`SPDX-FileCopyrightText: Copyright (C) <year> <holder> <email>`) and placed
  **directly under** the license line - one blank line then separates the header
  from the first code line.
- Both are `#` line comments. If the file is executable it may open with a
  `#!/usr/bin/env -S jennifer run` shebang on **line 1**, with the two SPDX lines
  immediately below it.
- The identical two-line header (written with `//`) tops every `.go` file in the
  interpreter, so the license/copyright form is uniform across the whole repo.
  Markdown (`.md`) files carry **no** header.

`jennifer fmt` treats the header as an ordinary leading comment block and
re-emits it verbatim - it never rewrites, reorders, or drops your SPDX lines.

## Source file conventions

- **`.j` extension** for all Jennifer source. The interpreter rejects
  anything else.
- **The two-line SPDX header** above tops every committed `.j` file.
- **`use` and `import` statements come first**, before any methods or
  top-level statements. Group `use` lines together, then `import` lines,
  then a blank line, then the rest of the program.
- **Blank line after a leading comment block.** If the file opens with
  a header comment (the SPDX license + copyright lines, an optional file
  description, a shebang), leave one blank line between the comment block
  and the first code line. Files that start directly with code (no header)
  start on line 1 - no leading blank.
- **Trailing newline** at end of file.

## Editor configuration

Drop the following into a `.editorconfig` file at your project root and
[any editor with EditorConfig support](https://editorconfig.org/#download)
will enforce the spacing and file-encoding rules above automatically:

```ini
# .editorconfig
root = true

[*.j]
indent_style = space
indent_size = 4
end_of_line = lf
charset = utf-8
trim_trailing_whitespace = true
insert_final_newline = true
```

That covers the indentation rule (4 spaces, no tabs), trailing-whitespace
and final-newline conventions, and pins UTF-8 + LF line endings so
collaborators on different OSes don't accidentally introduce CRLF
diffs. `jennifer fmt` re-emits source in the same shape, so the
EditorConfig settings and the formatter never disagree.

If you keep `.j` files alongside other languages in one repository, add
a generic fallback as the first block so plain text files don't drift
either:

```ini
[*]
end_of_line = lf
charset = utf-8
trim_trailing_whitespace = true
insert_final_newline = true
```

## A complete example

```jennifer
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

use io;

/**
 * Factorial of a non-negative integer.
 * @param n {int} the operand (assumed >= 0)
 * @return {int} n!
 */
func fact(n as int) {
    if ($n == 0) {
        return 1;
    }
    return $n * fact($n - 1);
}

for (def i as int init 0; $i <= 8; $i = $i + 1) {
    io.printf("%d! = %d\n", $i, fact($i));
}
```

Everything in this example follows the rules above: the two-line SPDX header,
1TBS braces, 4-space indent, spaces around binary operators, double-quoted
strings, expanded blocks, and a doc comment on the `func`. `jennifer fmt` will
produce this output byte-for-byte from any equivalent input.

Comments, blank lines, and a shebang on line 1 all survive a `fmt`
round-trip. The two SPDX header lines (`# SPDX-License-Identifier: ...` and
`# SPDX-FileCopyrightText: ...`) and any inline `# why` notes you keep
alongside the code are re-emitted at their original positions: leading
comments stay on the line above their attached statement, trailing same-line
comments stay on the same line, and runs of blank lines collapse to one
(matching the "never more than one consecutive blank line" rule).
