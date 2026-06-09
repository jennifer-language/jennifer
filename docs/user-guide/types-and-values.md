# Types and values

## Types

| Type                | Example literals             | Default | Notes                                     |
|---------------------|------------------------------|---------|-------------------------------------------|
| `int`               | `0`, `42`, `9001`            | `0`     | 64-bit signed                             |
| `float`             | `3.14`, `0.5`                | `0.0`   | 64-bit; promoted from int in mixed math   |
| `string`            | `"hello"`, `'single quotes'` | `""`    | Supports escape sequences                 |
| `bool`              | `true`, `false`              | `false` | Produced by comparison operators          |
| `null`              | `null`                       | `null`  | A type with a single value (the unit)     |
| `list of T`         | `[1, 2, 3]`                  | `[]`    | Ordered sequence; 0-indexed; mutable      |
| `map of K to V`     | `{"a": 1, "b": 2}`           | `{}`    | Key→value; insertion-ordered; mutable     |

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

| Escape | Meaning            |
|--------|--------------------|
| `\n`   | newline            |
| `\r`   | carriage return    |
| `\t`   | tab                |
| `\\`   | backslash          |
| `\"`   | double quote       |
| `\'`   | single quote       |
| `\0`   | null character     |

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
printf($x);              # use site - $ prefix
$x = 42;                 # assignment - $ prefix

def $x as int init 5;    # ERROR: drop the $ here
```

Constants don't use `$` anywhere (they're not mutable, so the sigil would have
no meaning):

```jennifer
def const MAX as int init 100;
printf(MAX);             # use site - bare name
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
printf("%d\n", $xs[0]);          # 10
$xs[1] = 99;                     # index write
printf("%d\n", len($xs));        # 3

# A map is a key->value lookup. Iteration is in insertion order.
def m as map of string to int init {"a": 1, "b": 2};
printf("%d\n", $m["a"]);         # 1
$m["c"] = 3;                     # adds new key
$m["a"] = 99;                    # updates existing

# Iterate a list's elements, or a map's keys.
for (def x in $xs) { printf("%d ", $x); }      printf("\n");
for (def k in $m) { printf("%s ", $k); }       printf("\n");
```

A few rules worth knowing up front:

- **Out-of-bounds list reads and writes are errors**, not silent
  no-ops. Same for reads of missing map keys - use
  [`has($m, key)`](../libraries/core.md) to test for presence first.
- **Lists and maps copy on assignment and on function-call binding.**
  `$ys = $xs;` makes an independent copy; mutating `$ys[0]` doesn't
  change `$xs`.
- **`const` is deep.** `def const NUMS as list of int init [1, 2, 3];`
  rejects both `$NUMS = ...` and `$NUMS[0] = ...`. Nested const
  lists/maps follow the same rule transitively.
- **Nesting works**: `list of list of int`,
  `map of string to list of int`, and so on. See
  [Nested lists and maps](#nested-lists-and-maps) below for the
  shape rules and when nesting gets too deep.
- **Empty literals require a declared type**: `[]` and `{}` are valid
  literals but the surrounding `def x as list of T` decides what they
  hold.

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

#### Why 4+ levels of nesting is a code smell

The same flexibility that lets `list of list of int` hold any shape gets
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
3. **Inflexible.** Adding a fifth dimension (per save slot, per timestamp,
   ...) means rewriting every access site in the program.

The standard fix is a struct or named record, which Jennifer doesn't have
yet (planned post-M10). Until then, options for the meantime:

- **Wrap access in methods**: `getItem(save, player, character, slot)`
  reads better than four bare brackets and gives you one place to fix a
  bug. Internally the function still walks the nested lists, but call
  sites are self-documenting.
- **Flatten with composite keys**: `map of string to string` keyed on
  `"save:0/player:1/char:0/slot:0"` trades index speed for name clarity.
  Better when the structure is sparse anyway.
- **Decompose into parallel simpler structures**: one list of save
  metadata, one map from save-id to inventory, etc.

As a rule of thumb: **one level is normal, two is fine, three is
uncommon, four is almost always a sign there's a missing abstraction.**
