# Types and values

## Types

| Type            | Example literals                                    | Default          | Notes                                                                                                                  |
| --------------- | --------------------------------------------------- | ---------------- | ---------------------------------------------------------------------------------------------------------------------- |
| `int`           | `42`, `0xff`, `0o755`, `0b1010_0110`, `1_000`       | `0`              | 64-bit signed; `_` may separate digits                                                                                 |
| `float`         | `3.14`, `0.5`, `1_000.000_5`                        | `0.0`            | 64-bit; promoted from int in mixed math                                                                                |
| `string`        | `"hello"`, `'single quotes'`                        | `""`             | Supports escape sequences                                                                                              |
| `bool`          | `true`, `false`                                     | `false`          | Produced by comparison operators                                                                                       |
| `null`          | `null`                                              | `null`           | A type with a single value (the unit)                                                                                  |
| `bytes`         | *(no literal)*                                      | empty            | Mutable byte sequence; element = `int` in `[0, 255]`; built via `convert.bytesFromString` or grown with `$b[] = byte;` |
| `list of T`     | `[1, 2, 3]`                                         | `[]`             | Ordered sequence; 0-indexed; mutable                                                                                   |
| `map of K to V` | `{"a": 1, "b": 2}`                                  | `{}`             | Key→value; insertion-ordered; mutable                                                                                  |
| user struct     | `Point{x: 1, y: 2}` (after `def struct Point ...;`) | every field zero | Named fixed set of typed fields; see [Structs](#structs)                                                               |
| `task of T`     | *(no literal - produced by `spawn { ... }`)*         | *(cannot be defaulted; must be initialised)* | Handle to a concurrent computation; observed via the [`task`](../libraries/task.md) library. See [Concurrency](concurrency.md)     |

The **Default** column is the value an uninitialized variable receives
(`def x as int;` produces `0`). For compound types the default is an
empty container of the declared element / key / value type, not `null`.

Lists and maps are compound types - they hold other Jennifer values.
Nesting works: `list of list of int`, `map of string to list of int`,
etc. Both are **value-typed**: `$ys = $xs;` makes an independent copy,
function parameters bind by copy, and `const` is deep (you cannot
mutate the contents of a `const` list or map).

Note: Jennifer's `list` is an array-backed sequence (Go slice
underneath), not a Lisp linked list. You get O(1) random access via
`$xs[i]`, but no O(1) prepend.

### String escape sequences

Both `"..."` and `'...'` are valid string delimiters. The following escapes
are recognized:

| Escape | Meaning         |
| ------ | --------------- |
| `\n`   | newline         |
| `\r`   | carriage return |
| `\t`   | tab             |
| `\\`   | backslash       |
| `\"`   | double quote    |
| `\'`   | single quote    |
| `\0`   | null character  |

## Variables and constants

```jennifer
def name as int init 5;            # declare and initialize
def count as int;                  # declare with the zero value of int (0)
def const MAX as int init 100;     # constant: uppercase name, init required
```

Uninitialized variables get the **default value** of their declared
type (see the [Types](#types) table).

**`init` accepts any expression of the declared type**, not just
literals. Arithmetic, comparisons, function calls, and index reads all
work as long as the result kind matches:

```jennifer
def half as float init 5 / 2;                # 2.5 (arithmetic)
def isZero as bool init 1 == 0;              # false (comparison)
def winner as string init decide($a, $b);    # whatever decide() returns
def first as int init $xs[0];                # element read
```

The same goes for `def const NAME` - the `init` expression is evaluated
once at declaration time and the result is frozen.

**At the def site, names are bare identifiers (no `$`).** The `$` sigil is
reserved for use-site references that read or assign a variable. So:

```jennifer
def x as int init 5;     # def site - bare name
io.printf($x);              # use site - $ prefix
$x = 42;                 # assignment - $ prefix

def $x as int init 5;    # ERROR: drop the $ here
```

Constants don't use `$` anywhere (they're not mutable, so the sigil would have
no meaning):

```jennifer
def const MAX as int init 100;
io.printf(MAX);             # use site - bare name
MAX = 200;               # ERROR: cannot assign to constant
```

**Constant names must be UPPERCASE.** The full rule is
`[A-Z]+(_[A-Z]+)*`: one or more uppercase chunks joined by single
underscores. `MAX`, `MAX_RETRIES`, `HTTP_OK`, and `A_B_C` are all
legal; `max`, `Max`, `_MAX`, `MAX_`, and `MAX__INT` are not. The
uppercase-only rule is what tells the parser at use sites that a bare
identifier is a constant reference, not a variable that forgot its
`$`. Constants also **require an `init` expression** - there is no
"declare-then-set" form (`def const X as int;` is rejected).

Assignment uses `=`:

```jennifer
def x as int init 0;
$x = 42;          # ok
$x = "string";    # error: cannot assign string to int variable
```

## Scoping

- Each `{...}` block introduces a new scope.
- A binding is visible from its `def` to the end of the enclosing block, and
  is inherited by inner blocks.
- Inner scopes can **read** outer bindings but **cannot redefine** a name
  already in scope (no shadowing). The interpreter rejects shadowing at
  runtime.
- A `for` loop opens a private scope wrapping `init`/`cond`/`step`/body, so
  the loop variable does not leak out.
- Constants follow the same scoping rules and reject any later assignment.

## Lists and maps

Two compound types let you hold collections of values.

```jennifer
use io;

# A list is an ordered, 0-indexed, mutable sequence.
def xs as list of int init [10, 20, 30];
io.printf("%d\n", $xs[0]);          # 10
$xs[1] = 99;                     # index write
io.printf("%d\n", len($xs));        # 3

# A map is a key->value lookup. Iteration is in insertion order.
def m as map of string to int init {"a": 1, "b": 2};
io.printf("%d\n", $m["a"]);         # 1
$m["c"] = 3;                     # adds new key
$m["a"] = 99;                    # updates existing

# Iterate a list's elements, or a map's keys.
for (def x in $xs) { io.printf("%d ", $x); }      io.printf("\n");
for (def k in $m) { io.printf("%s ", $k); }       io.printf("\n");
```

A few rules worth knowing up front:

- **Out-of-bounds list reads and writes are errors**, not silent
  no-ops. Same for reads of missing map keys - use
  [`maps.has($m, key)`](../libraries/maps.md) to test for presence first.
- **Lists and maps copy on assignment and on function-call binding.**
  `$ys = $xs;` makes an independent copy; mutating `$ys[0]` doesn't
  change `$xs`.
- **`const` is deep.** `def const NUMS as list of int init [1, 2, 3];`
  rejects both `$NUMS = ...` and `$NUMS[0] = ...`. Nested const
  lists/maps follow the same rule transitively.
- **Nesting works**: `list of list of int`,
  `map of string to list of int`, and so on. See
  [Nested lists and maps](#nested-lists-and-maps) below for the
  shape rules; [best practices](best-practices.md#why-4-levels-of-nesting-is-a-code-smell)
  has guidance on when nesting gets too deep.
- **Empty literals require a declared type**: `[]` and `{}` are valid
  literals but the surrounding `def x as list of T` decides what they
  hold.

### The `$xs[]` append sugar

For the common "build a list by appending" pattern, `$xs[] = item;`
writes to the position just past the end of the list:

```jennifer
def xs as list of int init [];
$xs[] = 10;
$xs[] = 20;
$xs[] = 30;
# $xs is now [10, 20, 30]
```

It's equivalent to `$xs = lists.push($xs, item);`; use whichever
reads better in context.

Rules:

- **Write-only.** `$xs[]` is only meaningful as a write target. Any
  read context (`io.printf($xs[])`, `def y init $xs[] + 1`) is a
  parse error.
- **Lists and bytes only.** `$m[] = ...;` on a map errors at runtime;
  maps have no "end-of" position.
- **Type-checked.** The value is checked against the list's declared
  element type, same as `$xs[i] = item;`.
- **`const` is still deep.** `$NUMS[] = ...;` on a `def const` list
  errors with the usual "cannot mutate contents of constant" message.

### Nested lists and maps

Compound types nest by repeating the keyword. `list of list of int` is a
list whose elements are themselves lists of ints; `map of string to list
of int` is a map whose values are lists of ints. There's no depth cap -
the parser will recurse as far as you nest.

#### The "different dimensions, same type" gotcha

Coming from C or Java, you might expect `int[3][3]` to mean "a 3×3 grid -
exactly nine ints, fixed shape". **Jennifer does not work that way.**

The declared type only fixes *what each level holds*, not *how many
elements are at each level*. So all of these are the same `list of list of
int` type:

```jennifer
# 2×2 grid - two rows of two columns
def gridA as list of list of int init [[1, 2], [3, 4]];

# 3×3 grid - three rows of three columns
def gridB as list of list of int init [[0, 0, 0], [0, 0, 0], [0, 0, 0]];

# Jagged - rows have different lengths
def gridC as list of list of int init [[1], [2, 3], [4, 5, 6]];

# Empty - zero rows
def gridD as list of list of int init [];
```

Same declared type, four very different shapes. At runtime each list
just knows its own length; reading `$gridA[2]` is an out-of-bounds error
(only indices 0 and 1 exist), reading `$gridC[2][2]` works (the third
row has three elements), but `$gridC[0][2]` is out of bounds (the
first row has only one element). **`len($gridC[i])` is the only way to
ask "how wide is this particular row?"**

If you need a strict shape, enforce it in code:

```jennifer
func makeGrid(size as int) {
    def out as list of list of int init [];
    for (def i as int init 0; $i < $size; $i = $i + 1) {
        def row as list of int init [];
        for (def j as int init 0; $j < $size; $j = $j + 1) {
            $row[] = 0;
        }
        $out[] = $row;
    }
    return $out;
}
```

When nesting gets deep enough that you're counting brackets, it's
usually time to reach for a struct or another abstraction - see
[best practices](best-practices.md#why-4-levels-of-nesting-is-a-code-smell)
for the heuristics.

## Bytes

`bytes` is a **mutable byte sequence**. It looks and acts a lot like a
`list of int`, with two important specialisations:

- Each element is constrained to `int` in `[0, 255]`. A write outside
  that range is a positioned runtime error.
- Indexing returns the byte as an `int` (you can't get a one-byte
  `bytes` slice via `$b[i]` - it's the integer value of the byte).

```jennifer
use io;
use convert;

# Constructing - bytes has no literal form. Either decode a string,
# or start empty and append.
def from_string as bytes init convert.bytesFromString("Hello", "utf-8");
def grown as bytes;
$grown[] = 0x48;
$grown[] = 0x69;

io.printf("from_string: %v\n", $from_string);  # bytes[48 65 6c 6c 6f]
io.printf("grown:       %v\n", $grown);        # bytes[48 69]
io.printf("len:         %d\n", len($from_string));  # 5

# Reading - $b[i] is the byte's value as int.
io.printf("first byte:  %d (= 0x%d|base=16)\n", $from_string[0], $from_string[0]);

# Writing - same int-in-range rule.
$from_string[0] = 0x68;       # lowercase h
io.printf("after edit:  %v\n", $from_string);

# Round-trip back to string.
def s as string init convert.stringFromBytes($from_string, "utf-8");
io.printf("string back: %s\n", $s);
```

### Why `bytes` is its own type (not just `list of int`)

The range constraint is the point. A `list of int` can hold any
64-bit signed integer; `bytes` can only hold a byte. The runtime
enforces this on every write so I/O, hashing, encoding, and
crypto code can rely on it. Trying to write `$b[i] = 256;` is a
positioned runtime error, not a silent truncation.

### Value semantics, just like lists and maps

```jennifer
def src as bytes init convert.bytesFromString("Hi", "utf-8");
def dst as bytes init $src;
$dst[0] = 0x78;            # mutates only dst
# $src is still bytes[48 69]
```

Function parameters bind by copy too, so a `func mutate(b as bytes)`
that writes into `$b` doesn't leak back to its caller. `const` is
deep: `def const B as bytes init ...;` rejects both `$B = ...` and
`$B[i] = ...`.

### The `$b[] = byte;` append form

Bytes share the append sugar with lists:

```jennifer
def buf as bytes;
$buf[] = 0x48;
$buf[] = 0x69;
# buf is now bytes[48 69]
```

The byte you append must be an `int` in `[0, 255]`.

### Codecs and rune vs byte counts

- `convert.bytesFromString(s, codec)` and
  `convert.stringFromBytes(b, codec)` are the canonical bridges.
  Only `"utf-8"` is supported today; a future `encoding` library
  will add the rest.
- `stringFromBytes` is **strict at boundaries**: invalid UTF-8 input
  is a runtime error, not a silent replacement character.
- `len($b)` returns the **byte count**; `len($s)` on a string returns
  the **rune count**. They will disagree for any non-ASCII input.
- `io.readBytes(n) -> bytes` reads `n` bytes from stdin;
  `io.readChars(n) -> string` reads `n` Unicode code points (1-4
  bytes each, decoded from UTF-8). See
  [libraries/io.md](../libraries/io.md) for details.

## Structs

A **struct** names a fixed set of typed fields. Use a struct whenever a
multi-value bundle would otherwise be a map keyed by string literals -
the fields are checked at construction time, the field names are part
of the type, and reading `$p.x` is faster and clearer than indexing a
map by `"x"`.

A struct is defined once at the top level and reused everywhere:

```jennifer
def struct Point { x as int, y as int };
def struct Line { from as Point, to as Point };
```

The shape is `def struct Name { field as type, field as type, ... };`.
The struct name follows the identifier rule (letters only, up to 64
characters); field names follow the same rule. The trailing `;` is
required (every statement ends in one).

### Constructing, reading, writing

```jennifer
# Construct - every field must be named at the literal.
def p as Point init Point{ x: 3, y: 4 };

# Read.
io.printf("%d %d\n", $p.x, $p.y);    # 3 4

# Write.
$p.x = 30;
```

The struct literal `Point{ x: 3, y: 4 }` requires **every** field; a
missing field is a positioned error, and so is an unknown one (`z: 5`
on a `Point`). Field order in the literal is free - the runtime stores
each field at its declaration position regardless.

`def p as Point;` (no `init`) gives every field its declared zero,
recursing through nested struct fields. So `def L as Line;` produces
`Line{from: Point{x: 0, y: 0}, to: Point{x: 0, y: 0}}` without any
extra ceremony.

### Nested structs and chained access

A struct's field can itself be a struct, a list, a map, or any
combination. Reads and writes chain through `.field` and `[index]` in
whatever order makes sense:

```jennifer
def L as Line init Line{ from: Point{ x: 0, y: 0 }, to: Point{ x: 10, y: 20 } };

io.printf("%d\n", $L.to.x);    # 10  - field after field

$L.from.x = 5;                  # write through the chain
```

A struct field that's a list works the same way: `$bag.items[0] = 99;`
descends through the `.items` field and writes into the list at index 0.

### Value semantics

Like lists, maps, and bytes, structs are **value-typed**:

```jennifer
def p as Point init Point{ x: 1, y: 2 };
def q as Point init $p;     # independent copy
$q.y = 99;
# $p is still Point{x: 1, y: 2}; $q is Point{x: 1, y: 99}.
```

Function parameter binding copies too, so `func translate(pt as Point, dx as int)`
that writes into `$pt` doesn't leak back to the caller.

### `const` is deep

`def const ORIGIN as Point init Point{ x: 0, y: 0 };` rejects both
`$ORIGIN = ...` (rebinding) and `$ORIGIN.x = ...` (content mutation),
including writes that descend through nested struct fields. Same rule
as lists and maps - the value behind a `const` is frozen at every
depth.

### Strict at boundaries

- **Unknown struct type at declaration**: `def x as Widget;` when no
  `def struct Widget` exists is a positioned runtime error
  ("unknown struct type").
- **Missing or unknown field at the literal**: positioned errors that
  point at the offending position.
- **Field type mismatch on write**: `$p.x = "hi";` on `x as int`
  errors with the declared type and the actual value's kind.
- **Field access on a non-struct value**: `$xs.foo` where `$xs` is a
  list errors with "field access `.foo` requires a struct, got list".
