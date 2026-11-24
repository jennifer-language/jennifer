# Syntax

## Tokens and whitespace

**Whitespace (spaces, tabs, newlines) is not significant anywhere in
Jennifer.** The lexer skips it between tokens. Statements end with `;`,
blocks with a matching `}`, never with indentation or a line break. A
program written on one line or across many parses the same:

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
io.printf (
    "%d\n"
    ,
    $x
        +
            $x
)
;
```

The same holds for qualified (namespaced) calls. All three print the host
OS:

```jennifer
use io;
use os;

io.printf(os.PLATFORM);               # canonical - tight everywhere
io.printf( os.PLATFORM );             # ugly - spaces inside the call parens
io   .   printf   (   os . PLATFORM  );  # uglier - spaces around `.` too
```

Each `.` is an ordinary token boundary. **`jennifer fmt` rewrites all three
into the canonical form**; the
[style guide](style-guide.md#namespaced-calls)'s "no space around `.`" is a
style rule, not a syntax rule.

Consequences:

- `jennifer fmt` enforces style. The [style guide](style-guide.md)
  describes the canonical shape (one space around binary operators, no
  space inside `(` / `[` / `{`, 1TBS braces, tight `.` in qualified calls,
  ...) and `fmt` re-emits any well-formed source in that shape.
- The first line may be a `#!` shebang because `#` starts a line
  comment; everything from `#` to the next `\n` is whitespace to the
  parser.
- **Inside a string literal, whitespace *is* content.** A space, tab or
  newline between the quotes is part of the string value.

## Comments

```jennifer
# line comment - runs to end of line

/* block comment -
   can span multiple lines */
```

Block comments nest, so `/* ... */` can wrap code that already contains
one. Because `#` starts a line comment, the first line of a script may be a
Unix shebang:

```jennifer
#!/usr/bin/env -S jennifer run
use io;
io.printf("hi\n");
```

(`env -S` splits the rest of the line into arguments, which is how
`jennifer run` reaches the interpreter on Linux.)

`/usr/bin/env` is not universal: Android, some minimal containers and
userlands under another prefix have no `/usr/bin/env`, and an old `env`
lacks `-S`. There, name the interpreter by its absolute path; the kernel
passes one argument after it, so `run` fits:

```text
#!/opt/jennifer/bin/jennifer run
```

`jvc app install` writes such a launcher by itself when `/usr/bin/env` is
missing.

Source files normally must end in `.j`, but a file with a `#!` shebang first
line is exempt: `chmod +x` it and install it under a bare command name
(`/usr/local/bin/mytool`, no extension) and it runs as a normal command. Only the
entry point is exempt - files it `import`s or `include`s still need `.j`.

## Number literals

Decimal:

```text
42
1_000_000           # `_` is a visual digit separator
3.14
1_000.000_5         # the mantissa side of a float accepts `_` too
```

Non-decimal integer prefixes:

```text
0xff                # hex
0xDEAD_BEEF
0o755               # octal
0b1010_0110         # binary
```

All four bases produce ordinary `int` values. The `_` separator is allowed between digits but never
adjacent to the prefix, adjacent to another `_`, or at the start /
end of the digit run (`0x_ff`, `1__000`, `100_` are all lex errors).

## Identifiers

- Variable, function, parameter and library names start with a letter, then
  letters and digits: `[A-Za-z][A-Za-z0-9]*`, up to 64 characters. No
  underscores. `sha256`, `x2` and `toUtf8` are legal.
- **Constants** are uppercase chunks joined by single `_` separators:
  `[A-Z][A-Z0-9]*(_[A-Z][A-Z0-9]*)*`, up to 64 characters. Each chunk starts
  with an uppercase letter and may contain digits. `MAX`, `MAX_RETRIES`,
  `HTTP2`, `SHA256` and `SCRAM_SHA256` are legal; `_MAX`, `MAX_`, `MAX__INT`
  and `AES_256` are not.
- **Variable references** use a leading `$`: define `name`, refer to it as
  `$name`.
- **Constant references** are bare (no `$`).
- **Function calls** are bare and followed by `(...)`.
