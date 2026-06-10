# Jennifer style guide

This is the recommended source style for Jennifer programs. `jennifer fmt`
re-emits source in exactly this shape, so anything you write that matches
the spec will survive a `fmt` round-trip unchanged.

The guide is short on purpose: there are only a handful of rules, but they
are consistent. If you've used `gofmt`, `prettier`, `rustfmt`, or PSR-12,
nothing here will surprise you.

## Spacing

- **One space around every binary operator**: `$i = 1 + 2;`, not
  `$i=1+2;`. Applies to `+ - * / // % < > <= >= == and or` and `=`.
- **Unary `-` hugs its operand**: `-5`, `-$x`, `-fact($n - 1)`. No space
  between the `-` and the value it negates.
- **Word-form unary operators take a space**: `not $ok`, never `not$ok`.
  Same goes for any other keyword operator the language grows.
- **One space after `,` and `;` inside `for (...; ...; ...)`**, never
  before. `for (def i as int init 0; $i < 10; $i = $i + 1)`.
- **No space inside parentheses**: `printf("hi")`, not `printf( "hi" )`.
- **One space between a keyword and its `(`**: `if (cond)`, `while (cond)`,
  `for (...)`. Function calls don't get this space: `printf(...)`.
- **No space inside `[ ]` or `{ }` list/map literals**: `[1, 2, 3]`,
  `{"a": 1, "b": 2}`, not `[ 1, 2, 3 ]` or `{ "a" : 1 }`. Empty
  literals are `[]` and `{}`. Same rule as `()`.
- **No space before `[`**: `$xs[0]`, not `$xs [0]`. Index expressions
  hug their target.
- **No space inside `[]` for the M9 append form**: `$xs[] = item;`,
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

- **K&R: opening brace on the same line**, separated by one space:
  `func fact(n as int) {`, `if ($x > 0) {`, `else {`.
- **`}` on its own line**, except for the `} else {` / `} elseif (...) {`
  cascade, where `else`/`elseif` continues on the same line as the
  preceding `}`.
- **`fmt` always expands blocks across multiple lines** - the canonical
  form has the opening brace at end-of-line, each body statement on its
  own indented line, and the closing brace on its own line. Single-line
  blocks are still legal source (the parser accepts them), but `fmt`
  rewrites them to the expanded form for consistency.

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
      printf("%d\n", $i);
  }
  ```

  not

  ```jennifer
  def i as int;
  for ($i = 0; $i < 10; $i = $i + 1) {                # avoid
      printf("%d\n", $i);
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
  has multiple words. Identifiers are `[A-Za-z]{1,64}` - no digits, no
  underscores.
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
  otherwise need to escape.
- **Escape sequences are explicit**: `"\n"`, `"\t"`, `"\\"`. Don't rely
  on multi-line string literals - Jennifer doesn't have them.

## Comments

- `# line comment` for short notes that belong on or just above the
  thing they describe. The very first line may be a shebang
  (`#!/usr/bin/env -S jennifer run`); the lexer treats it as a comment.
- `/* block comment */` for longer commentary that doesn't fit one line.
  Block comments don't nest.
- Comments explain *why*, not *what*. The code already says what.

## Source file conventions

- **`.j` extension** for all Jennifer source. The interpreter rejects
  anything else.
- **One SPDX header at the top** of every committed `.j` file (the
  project uses `LGPL-3.0-only` - see existing examples).
- **`use` and `import` statements come first**, before any methods or
  top-level statements. Group `use` lines together, then `import` lines,
  then a blank line, then the rest of the program.
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

## Limitations

`jennifer fmt` (v1) operates on the lexer's token stream and re-emits
canonical source. As a consequence:

- **Comments are dropped.** The lexer strips `#` and `/* */` at scan
  time. Preserving them would require carrying comments as tokens, which
  is a future enhancement.
- **Blank lines aren't preserved or inserted.** The formatter packs
  statements one per line at the appropriate indent without inserting
  vertical whitespace between logical groups. You can still write blank
  lines in source, but they don't survive a `fmt` round-trip.

Both limitations are listed in `docs/milestones.md` for tracking.

## A complete example

```jennifer
use io;
func fact(n as int) {
    if ($n == 0) {
        return 1;
    }
    return $n * fact($n - 1);
}
for (def i as int init 0; $i <= 8; $i = $i + 1) {
    printf("%d! = %d\n", $i, fact($i));
}
```

Everything in this example follows the rules above: K&R braces, 4-space
indent, spaces around binary operators, double-quoted strings, expanded
blocks, no blank lines (a limitation - see above). `jennifer fmt` will
produce this output byte-for-byte from any equivalent input.

The SPDX header (`# SPDX-License-Identifier: ...`) and copyright comment
that the project's source files carry are stripped by `fmt` today; keep
them in version control via your normal workflow and re-apply after a
reformat until the comment-preservation enhancement lands.
