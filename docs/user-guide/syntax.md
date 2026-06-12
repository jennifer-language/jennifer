# Syntax

## Tokens and whitespace

**Whitespace (spaces, tabs, newlines) is not significant anywhere in
Jennifer.** The lexer skips it between tokens and never reads it from
inside one. Statements are terminated by `;`, blocks by matched `{` /
`}` - never by indentation or line breaks. The same program can be
written across many lines or jammed onto one, and it parses the same:

```jennifer
# canonical form
use io;
def x as int init 21;
io.printf("%d\n", $x + $x);
```

```jennifer
# all three statements on one line - same program
use io; def x as int init 21; io.printf("%d\n", $x + $x);
```

```jennifer
# split across many lines - also the same program
use   io
;
def
    x
        as
            int
                init
                    21 ;
printf (
    "%d\n"
    ,
    $x
        +
            $x
)
;
```

The same flexibility applies to qualified (namespaced) calls.
All three of these print the host OS and are accepted by the parser:

```jennifer
use io;
use os;

io.printf(os.JENNIFER_OS);            # canonical - tight everywhere
io.printf( os.JENNIFER_OS );          # ugly - spaces inside the call parens
printf   (   os     . JENNIFER_OS  );   # uglier - spaces around `.` too
```

The third form parses fine because the `.` between `os` and
`JENNIFER_OS` is just another token boundary - the lexer doesn't
care that there are spaces around it. **`jennifer fmt` rewrites all
three into the canonical form**, so you'll only ever see the first
one after a format pass. The
[style guide](style-guide.md#namespaced-calls) makes "no space
around `.`" explicit, but it's a style rule, not a syntax rule.

A few practical consequences worth knowing up front:

- `jennifer fmt` is the enforcement layer for style. The
  [style guide](style-guide.md) describes the canonical shape (one
  space around binary operators, no space inside `(` / `[` / `{`,
  1TBS braces, tight `.` in qualified calls, ...) and `fmt` re-emits
  any well-formed source in exactly that shape. You're never
  *required* to write canonical form; you just won't see anything
  else after `fmt`.
- The first line may be a `#!` shebang because `#` starts a line
  comment; everything from `#` to the next `\n` is whitespace as far
  as the parser is concerned.
- **Inside a string literal, whitespace *is* literal content.** A
  space or tab between the quotes is part of the string value; an
  actual newline between the quotes is a literal newline in the
  value (though the conventional spelling is the `\n` escape -
  multi-line literals work but aren't the canonical form).

Indentation and blank lines never change the meaning of a program;
they only change how it reads.

## Comments

```jennifer
# line comment - runs to end of line

/* block comment -
   can span multiple lines */
```

Block comments don't nest. Because `#` starts a line comment, the first
line of a script may be a Unix shebang and the interpreter will skip it:

```jennifer
#!/usr/bin/env -S jennifer run
use io;
io.printf("hi\n");
```

(`env -S` splits the rest of the line into arguments, which is how
`jennifer run` reaches the interpreter on Linux.)

## Number literals

Decimal:

```jennifer
42
1_000_000           # `_` is a visual digit separator
3.14
1_000.000_5         # the mantissa side of a float accepts `_` too
```

Non-decimal integer prefixes:

```jennifer
0xff                # hex
0xDEAD_BEEF
0o755               # octal
0b1010_0110         # binary
```

All four bases produce ordinary `int` values - same kind, same
operators. The `_` separator is allowed between digits but never
adjacent to the prefix, adjacent to another `_`, or at the start /
end of the digit run (`0x_ff`, `1__000`, `100_` are all lex errors).

## Identifiers

- Variable, method, parameter and library names are letters only: `[A-Za-z]`,
  up to 64 characters. No digits or underscores.
- **Constants** are uppercase chunks joined by single `_` separators:
  `[A-Z]+(_[A-Z]+)*`, up to 64 characters. Every `_` must be immediately
  followed by another uppercase letter. `MAX`, `MAX_RETRIES`, `HTTP_OK`,
  and `A_B_C` are all legal; `_MAX`, `MAX_`, and `MAX__INT` are not.
- **Variable references** use a leading `$`: define `name`, refer to it as
  `$name`.
- **Constant references** are bare (no `$`).
- **Method calls** are bare and followed by `(...)`.
