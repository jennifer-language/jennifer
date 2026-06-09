# Control flow

## Operators

| Operator             | Meaning                                                  |
|----------------------|----------------------------------------------------------|
| `+`                  | addition (`int`/`float`); also concatenation on `string` |
| `-`, `*`             | subtraction, multiplication (`int`/`float`)              |
| `/`                  | **true division - always returns `float`**               |
| `//`                 | floor (integer) division; `int // int -> int`            |
| `%`                  | modulo (`int` only)                                      |
| unary `-`            | numeric negation (`int`/`float`)                         |
| `<`, `>`, `<=`, `>=` | numeric comparison; result is `bool`                     |
| `==`                 | equality; same-kind plus `int`/`float` promotion; `bool` |
| `and`, `or`          | logical; both operands `bool`; short-circuit             |
| `not`                | unary logical negation; operand `bool`                   |

**Division has two operators.** `/` always returns a `float` (Python 3
style). `//` returns the floor, keeping the type when both operands are
ints:

```jennifer
5 / 2          # 2.5 (float)
5 // 2         # 2   (int)
5.0 / 2.0      # 2.5 (float)
5.7 // 2.0     # 2.0 (float - floor of a float division)
```

So `def x as int init 5 / 2;` is rejected (right side is float). Use
`5 // 2` for an int result, or `def x as float init 5 / 2;`.

(Line comments are `#`, freeing `//` for the Python-3 floor-division
operator. The `#` choice also lets Jennifer files start with a shebang:
`#!/usr/bin/env -S jennifer run`.)

Precedence (low to high): `or`, `and`, `not`, comparison, additive (`+`, `-`),
multiplicative (`*`, `/`, `//`, `%`), unary `-`. Use parentheses to override:
`(1 + 2) * 3`. Examples that follow the rules:

```jennifer
not 1 == 2                  # not (1 == 2) -> true
1 > 0 and 2 > 1             # true
true or false and false     # true or (false and false) -> true
-3 + 10                     # (-3) + 10 -> 7
-3 * 2                      # (-3) * 2 -> -6
```

**`and` and `or` short-circuit.** The right operand is only evaluated when
the left doesn't already decide the result. That matters when the right side
has side effects:

```jennifer
def gate as bool init false;
def result as bool init $gate and expensive();   # expensive() not called
```

Mixed `int`/`float` arithmetic promotes the int to float and the result is a
float (`3 + 0.5` -> `3.5`). **`/` always returns `float`**, even with two
int operands (`5 / 2` is `2.5`, not `2`). Use `//` when you want an
integer quotient: `5 // 2` is `2`. This is Python-3 division, not C/Java
division.

## Conditionals and loops

```jennifer
if ($n == 0) {
    printf("zero");
} elseif ($n < 10) {
    printf("small");
} else {
    printf("large");
}

while ($i < 5) {
    $i = $i + 1;
}

for (def i as int init 0; $i < 10; $i = $i + 1) {
    printf($i);
}

# for-each over a list or map.
for (def x in $xs) {
    printf("%d ", $x);
}
for (def k in $m) {
    printf("%s=%d ", $k, $m[$k]);
}
```

Conditions in `if`, `elseif`, `while`, and `for` **must be `bool`** - there
is no implicit truthiness. Use a comparison (`$x == 0`) to get a bool.
For-each (`for (def x in $coll)`) doesn't take a condition - it walks
the whole collection.

### Loop variable scope

C-style `for` opens its own scope. **Where you `def` the iterator
variable decides whether you can still see it after the loop.**

```jennifer
# Loop-local: declare inside the for-init. The iterator lives only for
# the duration of the loop.
for (def i as int init 0; $i < 10; $i = $i + 1) {
    printf("%d\n", $i);
}
printf("%d\n", $i);   # ERROR: `i` not in scope here
```

```jennifer
# Outer-scope: declare in the surrounding scope, assign in the for-init.
# The variable survives past the loop and holds the value that made the
# condition false (10 here).
def i as int;
for ($i = 0; $i < 10; $i = $i + 1) {
    printf("%d\n", $i);
}
printf("%d\n", $i);   # ok - prints 10
```

The loop-local form is the [recommended style](style-guide.md#loops);
reach for the outer-scope form only when you actually need to inspect
the iterator after the loop ends. For-each (`for (def x in $coll)`)
is always loop-local - the iteration variable lives in a fresh scope
each pass through the loop and is gone once the loop exits.
