# Jennifer style guide

The recommended source style for Jennifer programs. `jennifer fmt` emits
exactly this shape, so code that follows it survives a `fmt` round-trip
unchanged. It is close to `gofmt`, `rustfmt` and PSR-12.

## Spacing

- **One space around every binary operator**: `$i = 1 + 2;`, not
  `$i=1+2;`. Applies to `+ - * / // % < > <= >= == != and or` and `=`.
- **Unary `-` hugs its operand**: `-5`, `-$x`, `-fact($n - 1)`. No space
  between the `-` and the value it negates.
- **Word-form unary operators take a space**: `not $ok`, never `not$ok`.
- **One space after `,` and `;` inside `for (...; ...; ...)`**, never
  before. `for (def i as int init 0; $i < 10; $i = $i + 1)`.
- **No space inside parentheses**: `io.printf("hi")`, not `io.printf( "hi" )`.
- **One space between a keyword and its `(`**: `if (cond)`, `while (cond)`,
  `for (...)`. Function calls don't get this space: `io.printf(...)`.
- **No space inside `[ ]` or `{ }` literals**: `[1, 2, 3]`,
  `{"a": 1, "b": 2}`, `Point{x: 1, y: 2}`, not `[ 1, 2, 3 ]` /
  `{ "a" : 1 }` / `Point{ x: 1 }`. List, map, and struct / enum literals
  are all tight (a struct literal also binds the brace to its type name:
  `Point{`, not `Point {`, see [Braces](#braces)). Empty literals are `[]`,
  `{}`, `Name{}`.
- **No space before `[`**: `$xs[0]`, not `$xs [0]`.
- **No space inside `[]` for the append form**: `$xs[] = item;`,
  never `$xs[ ] = item;` or `$xs [ ] = item;`.
- **One space after `:`** in map literals, never before: `{"a": 1}`,
  not `{"a" :1}` or `{"a":1}`.
- **No trailing whitespace** on any line.

## Indentation

- **4 spaces per level**, no tabs.
- One level per block - function body, `if`/`elseif`/`else` body, `while`
  body, `for` body.

## Braces

- **1TBS (one true brace style): opening brace on the same line**
  for *everything* - functions and control flow alike - separated by
  one space: `func fact(n as int) {`, `if ($x > 0) {`, `else {`.
- **`}` on its own line**, except for the `} else {` / `} elseif (...) {`
  cascade, where `else`/`elseif` continues on the same line as the
  preceding `}`.
- **`fmt` expands blocks across multiple lines**: the opening brace at
  end-of-line, each body statement on its own indented line, the closing
  brace on its own line. This applies to function bodies, control-flow
  blocks (`if / elseif / else`, `while`, `for`, `repeat`), `try { } catch
  (e) { }`, and `spawn { }` block expressions. Single-line blocks parse,
  but `fmt` expands them (`match` arms are the exception, below).
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
  `Point`, not a block) and no space inside it, like map and list literals.
  An empty literal is `Name{}`.
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
  conditional), so, as in C, Go, Rust, Swift and Kotlin, each `when` (and the
  `else`) begins its own line at the arm indent. An arm holding a **single short statement stays inline**
  (`when "idle" { begin(); }`); an arm with two or more statements, or one that
  overflows the line, expands one statement per indented line with its `}` on its
  own line. This is the one place `else` starts a line rather than cuddling a
  `}`: the `} else {` cuddle rule is for an `if`'s conditional tail, and a
  `match`'s `else` is a case-list arm, so the `when` column reads top to
  bottom. A `when` value list that overflows the line wraps with each
  continuation value aligned under the first:

  ```jennifer
  func step(state as string) {
      match ($state) {
          when "idle" { begin(); }
          when longEventName1(),
               longEventName2() {
              prepare();
              handle();
          }
          else { reject(); }
      }
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

  A source line break at one of these joiners is kept even when the line
  would fit under 100, so the shape above survives a `fmt` round-trip
  byte-for-byte; so is a break after an operand that ends in a trailing
  comment (the next operand keeps its continuation indent). A value moved to
  the line after `init`, or a call's lone argument moved to the line after its
  `(`, also keeps that break when joining it would pass 100 columns. What
  `fmt` will **not** guess at: a long argument list or a deeply nested call
  has no safe break point, so it stays on one line, and a single token longer
  than the limit (a long string literal, a URL) stays over it - break those
  by hand. `fmt` never *introduces* an over-long line, so its output does
  not add [`lint`](best-practices.md#lint-for-suspect-patterns-with-jennifer-lint) `L203` (`line-too-long`) findings that the
  source did not already have.

## Statements

- **Every statement ends with `;`**, including the last one in a block.
- **One statement per line**.
- **Blank lines separate logical groups** - imports from function
  definitions, functions from top-level code, distinct steps within a long
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

  The loop-local form is self-contained (the `for` line says everything
  about `i`), keeps the iterator out of the surrounding scope, and matches
  the for-each shape (`for (def x in $coll)`), which is *always*
  loop-local. Use the outer-scope form only when you need the iterator's
  value after the loop, for example to report which iteration triggered a
  `break`.
- **One concern per loop.** A body longer than a screen probably belongs
  in a helper function.

## Names

- **Variables, functions, parameters**: lowercase or `camelCase` if the name
  has multiple words.
- **Structs, enums, and enum variants**: `PascalCase` - `Point`, `Shape`,
  `Circle`. The interpreter does **not** enforce this (a name resolves by what
  it refers to, not by its case). It keeps types visually distinct from
  `camelCase` values and `UPPERCASE` constants, and avoids the one real
  collision: an
  all-`UPPERCASE` type name would be read as a *constant* in `Name.member`
  position, so never name a type in all caps.
- **Constants**: `UPPERCASE`, with `_` as a single word separator. The
  full rule is `[A-Z][A-Z0-9]*(_[A-Z][A-Z0-9]*)*`, up to 64 characters: one
  or more uppercase chunks joined by single `_`, each starting with a letter
  and optionally containing digits. Every `_` must be immediately followed by
  `[A-Z]` - no leading, trailing, or consecutive `_`. Examples: `MAX`,
  `MAX_RETRIES`, `HTTP_OK`, `SHA256`, `SCRAM_SHA256`. Lowercase letters are
  not allowed, and neither is a digit right after `_` (`AES_256`; write
  `AES256`).
- **Library names**: lowercase, single word where possible (`io`, `math`,
  `strings`, `meta`).

## Namespaced calls

Libraries are addressed by `prefix.name(...)`. The dot binds tight on
both sides:

- **No space around `.`**: `os.cwd()`, never `os . cwd()`.
- **The call parens hug the callee**: `os.cwd()`, not `os.cwd ()`.
- **`use lib as alias;` is one space on each side of `as`**:
  `use strings as str;`, never `use strings  as  str;`.

Aliasing a library frees its canonical name for ordinary use (you *could*
write `func os() { ... }` after `use os as o;`). Don't: the name reads as a
call into the library.

## Function signatures

These rules apply to every exported function, in the standard library and in
your own modules and decks, so all APIs read alike.

- **The subject comes first**: the data the call works on
  (`strings.indexOf(s, sub)`, `images.crop(img, x, y, w, h)`).
- **A name that picks the behaviour comes before the subject.** When a
  function takes an algorithm, format, codec, byte order, method or kind
  as a string, that name is the first argument: `hash.compute("sha256", b)`,
  `encoding.toText("hex", b)`, `compress.pack("gzip", b)`,
  `binary.readInt("le", b, 0, 4)`, `jwt.sign("HS256", claims, key)`. The
  name decides what the later arguments mean, and a literal in front is
  easy to read and to check.
- **A handle stays in front of everything.** A connection, stream or client
  that the call acts on is the receiver and comes first, followed by the
  name: `ldap.bindSasl(conn, algo, user, password)`; without a name it is
  first, `hash.update(stream, b)`.
- **Then the other parameters**: keys, nonces, offsets, options.
- **Optional arguments come last**: `compress.pack(algo, b[, level])`.
- **No plain name hides a fixed algorithm.** If a function could reasonably
  support more than one algorithm, take it as an argument instead of
  putting it in the function name (`hash.compute("sha256", b)`, not
  `sha256(b)`).

## Strings

- **Default to `"..."` (cooked)** for ordinary text: `"hello"`, `"line\nbreak"`.
  The two delimiters are **not** interchangeable: `'...'` is **raw** (no escape
  processing).
- **Reach for `'...'` (raw) when escapes would be noise**: a regex
  (`'\d+\.\d+'` over `"\\d+\\.\\d+"`), a Windows path, embedded JSON, or a
  multi-line block. A raw literal cannot contain a `'`, so a string with an
  apostrophe uses the cooked form (`"it's"`).
- **`fmt` preserves a literal's exact source spelling** - its delimiter, its
  escapes, and any line breaks inside it - so a raw or multi-line string
  survives a format untouched.
- A string literal of **either** form may span multiple lines (a newline
  between the quotes is part of the value); `fmt` keeps it as written.

### Interpolation slots

- **A cooked `"..."` interpolates `{expr}`**; a raw `'...'` never does. Prefer
  interpolation over `sprintf` concatenation when you are placing a value beside
  its label: `"user {$name} has {$count} items"` reads better than a `%s` / `%d`
  template whose arguments trail off the end.
- **Keep a slot trivial** - a variable (`{$x}`), a constant (`{MAX}`), a field or
  index access (`{$p.name}`, `{$xs[0]}`), or arithmetic (`{$a + $b}`).
- **Do not hide a call in a slot.** `"total {computeTotal($cart)}"` buries a
  function call - with its cost and any side effect - inside what looks like a
  string literal. Compute it first (`def total ...; "total {$total}"`). `lint`
  flags a call in a slot as **L204**.
- **A literal brace is `\{` / `\}`** in a cooked string; if a string is mostly
  literal braces (JSON, a `{{ }}` template), reach for a **raw** `'...'` string
  instead of escaping every brace.

## Comments

- `# line comment` for short notes that belong on or just above the
  thing they describe. The very first line may be a shebang
  (`#!/usr/bin/env -S jennifer run`); the lexer treats it as a comment.
- `/* block comment */` for longer commentary that doesn't fit one line.
  Block comments nest, so wrapping a chunk of code that already
  contains a block comment in another `/* ... */` works.
- **Inline block comments inside `(`, `[`, or after `.` get a space on
  the operand side.** `printf(/* note */ $x)`, not
  `printf(/* note */$x)`, an exception to the "no space inside `()`"
  rule because `*/$x` runs together. The comment hugs the opening
  delimiter (no space between `(` and `/*`). Same rule for
  `[/* note */ 0]` and `$obj./* doc */ field`.
- Comments explain *why*, not *what*.

## Doc comments

**Document every public `func`, `def struct`, and `def const` with a doc
comment**, and open a file with a module preamble. A doc comment is a block
comment that opens with exactly `/**` (a plain `/*` stays an ordinary
comment), sits immediately above the construct it documents, and holds a
summary line, an optional description, and `@`-tags. This is the format the
[`docblock`](../modules/docblock.md) module parses.

```jennifer
/**
 * Distance between two points.
 * A longer description can follow the summary line.
 * @param ax {float} first x coordinate
 * @param ay {float} first y coordinate
 * @return {float} the Euclidean distance
 * @since 0.9
 */
export func distance(ax as float, ay as float, bx as float, by as float) {
    # ...
}
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
  `@field` names against the real declaration and reports drift.

`jennifer fmt` preserves doc comments and keeps each on its own line above its
construct, so a formatted file is exactly what `docblock` expects to parse.

## SPDX header

Every committed `.j` file opens with a two-line [SPDX](https://spdx.dev/) header,
a machine-readable license tag and a copyright line, so license-scanning and
[REUSE](https://reuse.software/) tooling can attribute each file:

```jennifer
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
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
- In the interpreter's own repository the same header (written with `//`)
  tops every `.go` file; Markdown (`.md`) files carry **no** header.

`jennifer fmt` re-emits the header verbatim, as any leading comment block.

## Source file conventions

- **`.j` extension** for all Jennifer source. The interpreter rejects
  anything else, except an executable script with a shebang line run
  directly; `import` and `include` targets always need `.j`.
- **The two-line SPDX header** above tops every committed `.j` file.
- **`use` and `import` statements come first**, before any functions or
  top-level statements. Group `use` lines together, then `import` lines,
  then a blank line, then the rest of the program.
- **Blank line after a leading comment block.** If the file opens with
  a header comment (the SPDX license + copyright lines, an optional file
  description, a shebang), leave one blank line between the comment block
  and the first code line. Files that start directly with code (no header)
  start on line 1 - no leading blank.
- **Trailing newline** at end of file.

## Editor configuration

A `.editorconfig` file at your project root makes
[any editor with EditorConfig support](https://editorconfig.org/#download)
apply the spacing and file-encoding rules above:

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

It pins UTF-8 and LF line endings, as `jennifer fmt` emits. For a
repository with other languages, add a generic fallback as the first block:

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
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

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

`jennifer fmt` produces this output byte-for-byte from any equivalent input.
Comments, blank lines and a shebang on line 1 survive a `fmt` round-trip:
leading comments stay on the line above their statement, trailing comments
on the same line, and runs of blank lines collapse to one.
