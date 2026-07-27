# Formatter (`cmd/jennifer/fmt.go`)

`jennifer fmt [-w] <file.j>...` rewrites Jennifer source into the one canonical
style defined in [../user-guide/style-guide.md](../user-guide/style-guide.md).
By default it prints the result to **stdout**; `-w` (or `--write`) rewrites the
named files in place instead:

```sh
jennifer fmt prog.j            # preview the formatted source on stdout
jennifer fmt prog.j > out.j    # or redirect it (pipe to `sponge`, an editor, ...)
jennifer fmt -w prog.j         # rewrite prog.j in place
jennifer fmt -w a.j b.j        # rewrite several named files in place
```

`-w` skips a file whose content is already canonical (no needless mtime churn)
and preserves each file's mode; it prints `formatted <path>` to stderr for each
file it changes. Sending several files to stdout without `-w` is refused as
ambiguous. There is exactly one write flag - `-w` / `--write`, no `-i` synonym -
and no style options: like `gofmt`, the formatter is opinionated by design, so a
whole codebase reads the same and diffs stay minimal.

### Selecting files: use the shell

`fmt` formats exactly the files you name; it does **no** globbing or directory
walking of its own (a directory argument is a usage error). Selecting files is
the shell's job - that is the one obvious way to do it, and it composes with
every other command. A filemask works flat, and recursively via `**`:

```sh
# bash / zsh
jennifer fmt -w src/*.j                 # flat: every .j in src/
shopt -s globstar                       # bash: enable ** (zsh has it on)
jennifer fmt -w src/**/*.j              # recursive: every .j under src/
jennifer fmt -w src/**/mikrotik*.j      # recursive filemask

# fish (** is recursive by default, no setup)
jennifer fmt -w src/**.j                # recursive: every .j under src/
jennifer fmt -w src/**/mikrotik*.j      # recursive filemask
```

For anything the glob can't express, pipe from `find`:
`find src -name '*.j' | xargs jennifer fmt -w`.

## Before / after

The formatter fixes spacing, indentation, brace placement, and statement
splitting in one pass, while leaving your intent (parentheses, comments,
imports) intact:

```jennifer
use io;import "helpers.j" as h;
# greet the world
def   x as int init 21 ;
if($x>0){io.printf("pos\n") ;}else{ io.printf("neg\n");}


def y as int init ($x + 1)*2;   # keep the parens
def z as int init -$x;
for(def i as int init 0;$i<3;$i=$i+1){io.printf("%d\n",$i);}
func add(a as int,b as int){return $a+$b;}
```

becomes:

```jennifer
use io;
import "helpers.j" as h;
# greet the world
def x as int init 21;
if ($x > 0) {
    io.printf("pos\n");
} else {
    io.printf("neg\n");
}

def y as int init ($x + 1) * 2; # keep the parens
def z as int init -$x;
for (def i as int init 0; $i < 3; $i = $i + 1) {
    io.printf("%d\n", $i);
}
func add(a as int, b as int) {
    return $a + $b;
}
```

## What `fmt` normalises

| Aspect          | Canonical form                                                                                                            |
| --------------- | ------------------------------------------------------------------------------------------------------------------------- |
| Statements      | one per line, each terminated by `;` (`use io;import ...;` splits onto two lines).                                         |
| Indentation     | 4 spaces per block level; a `}` dedents *before* it is written, so it lands at the outer level.                            |
| Operator spacing| a single space around binary operators (`$a + $b`, `$x > 0`); **none** around a unary `-` (`-$x`).                         |
| Punctuation     | one space after each `,` and after the `;`s in a `for` header; no space *before* a `;`.                                    |
| Blocks          | `{` follows its header with a space (`if ($c) {`); the body is indented; `}` sits on its own line.                         |
| `else` / `elseif` | cuddle the preceding brace on one line (`} else {`).                                                                     |
| `for` header    | the two `;` stay on the header line (`for (init; cond; step)`), not split across lines.                                    |
| Calls / params  | arguments and parameters get one space after each comma (`add(a as int, b as int)`, `f(a, b)`).                           |
| Strings         | re-quoted with double quotes and standard escapes (`quoteJenniferString`, mirroring the lexer's `readString`).            |
| Blank lines     | a run of blank lines collapses to a single one.                                                                           |

## What `fmt` deliberately preserves

Formatting is layout-only; it never rewrites meaning. Three things are kept
exactly as written:

| Kept as written               | Why                                                                                                                     |
| ----------------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| `import "file.j";` statements | `fmt` works on the token stream *before* preprocessing, so imports are re-emitted, not inlined - the opposite of a splice. |
| User-written parentheses      | `($x + 1) * 2` keeps its grouping; an AST-based formatter would erase parens the grammar makes redundant.                 |
| Comments (and blank lines)    | `#` and nesting `/* */` comments survive as trivia: a leading comment stays on its own line, a trailing one stays on the same line. |

## How it works

`fmt` is **token-level, not AST-level** - it walks the lexer's token stream
rather than the parsed tree. That choice is what makes the two preservation
guarantees above possible:

- **`import` survives.** The preprocessor consumes file imports before the
  parser sees them; an AST formatter would inline every one. The token walker
  sees `IMPORT` tokens unchanged and re-emits them.
- **User parens survive.** The AST records grouping only through nesting, so
  redundant parens vanish. A token walker preserves `LPAREN` / `RPAREN` exactly.

`formatTokens(tokens)` drives a small state machine (`fmtState`): for each token
it computes the separator (`writeSeparator` - none, a space, or a
newline-plus-indent) and then writes the token's canonical spelling
(`writeToken`). The key state fields:

| Field              | Role                                                                                                     |
| ------------------ | -------------------------------------------------------------------------------------------------------- |
| `indent`           | current block depth; bumps on `{`, drops on `}` (the closing brace dedents before it is written).        |
| `prevIsOperand`    | answers "is the next `-` binary or unary?" - flipped by `isOperandToken` after every emit.               |
| `prevIsUnaryMinus` | suppresses the right-side space after a `-` that was ruled unary, so `-$x` stays tight.                   |
| `insideForHeader`  | a small backward scan that lets the two `;`s inside `for (...; ...; ...)` stay on the same line.          |

Comments and blank lines flow through the same machine: the lexer emits them as
trivia tokens, and `emitTrivia` writes them in place without disturbing the
surrounding state (leading comments on their own line at the current indent,
trailing same-line comments inline, blank-line runs collapsed to one; block
comments may nest).

Part of the [CLI reference](cli.md).
