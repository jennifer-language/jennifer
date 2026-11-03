# `decimal` - exact decimal arithmetic

Enable with `use decimal;`. Base-10 numbers that are **exact**: `0.1 + 0.2` is
`0.3`, a price times a quantity is the right number of cents, and nothing rounds
unless the program says how. A `float` cannot do that - most decimal fractions
have no exact binary form - which is why money, invoices, tax and measurements
with a fixed number of places belong in a `decimal.Decimal`.

```jennifer
use io;
use decimal;

def net as decimal.Decimal init decimal.mul("19.99", 3);              # 59.97
def tax as decimal.Decimal init decimal.round(decimal.mul($net, "0.20"), 2);
io.printf("%v\n", decimal.add($net, $tax));                             # 71.96
```

Runnable: [`examples/decimal.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/decimal.j).

A value is an opaque `decimal.Decimal`: an arbitrary-size integer coefficient and
a **scale**, the number of digits after the point. `12.340` has scale 3. Values
are immutable; every call returns a new one. Backed by Go's `math/big`; works on
both binaries.

## Operands

Every function that takes a number accepts a `decimal.Decimal`, an `int`, or a
**decimal string** (`"12.34"`, `"-0.5"`, `"1e3"`), so literals need no wrapping:
`decimal.mul($price, 3)`, `decimal.add($a, "0.05")`. A `float` is refused as an
operand: convert it explicitly with `decimal.from`, so a binary rounding error
never slips in unnoticed.

A decimal string is `[+-]digits[.digits]`, or `[+-].digits`, optionally followed
by an exponent `e` / `E` `[+-]digits`. No spaces, digit separators, `NaN` or
infinities.

## Surface

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `decimal.from(x)` | `Decimal` | From a decimal string, an `int`, or a `float`. A float goes in by its **shortest** decimal form, the one `%v` prints: `from(0.1)` is `0.1`, not `0.1000000000000000055511151231257827`. |
| `decimal.add(a, b)` / `decimal.sub(a, b)` | `Decimal` | Exact. The result's scale is the larger of the two. |
| `decimal.mul(a, b)` | `Decimal` | Exact. The scales add: `10.00 * 1.2` is `12.000`. |
| `decimal.div(a, b[, scale[, mode]])` | `Decimal` | See [Division](#division). |
| `decimal.round(d, scale[, mode])` | `Decimal` | Exactly `scale` places: rounded by `mode` (default `"half-even"`) when that drops digits, padded with zeros when it adds them (`round("1.5", 2)` is `1.50`). |
| `decimal.neg(d)` / `decimal.abs(d)` | `Decimal` | Negation / absolute value. |
| `decimal.normalize(d)` | `Decimal` | Drop trailing zeros after the point: `12.3400` -> `12.34`. |
| `decimal.pow(d, n)` | `Decimal` | `d` to the power of an `int` `n >= 0`, exactly. |
| `decimal.sum(xs)` | `Decimal` | The sum of a list of operands; `0` for an empty list. |
| `decimal.compare(a, b)` | `int` | `-1`, `0` or `1`. The ordering operators do not apply to a `Decimal`; use this. |
| `decimal.sign(d)` | `int` | `-1`, `0` or `1`. |
| `decimal.scale(d)` | `int` | The number of digits after the point. |
| `decimal.isInteger(d)` | `bool` | No fractional part (`3.000` is an integer). |
| `decimal.toString(d)` | `string` | Plain notation, never an exponent - the same text `%v` prints. |
| `decimal.toInt(d)` | `int` | Only an integral value that fits an `int`; anything else is an error (round first). |
| `decimal.toFloat(d)` | `float` | The nearest `float`. A value too large for a float is an error. |

## Equality, ordering, display

`==` compares **values**: `decimal.from("1.0") == decimal.from("1.00")` is
`true`. The scale is still kept and shown - `%v` of `1.00` prints `1.00` - and
`decimal.scale` tells the two apart when the number of places matters. For
ordering use `decimal.compare(a, b) < 0`; `<` / `>` on a `Decimal` are a
positioned error, like on any opaque value.

`%v`, `convert.toString` and the REPL print plain notation with exactly the
value's scale: `12.340`, `-0.05`, `1267650600228229401496703205376`. An
uninitialised `def d as decimal.Decimal;` is `0`.

## Division

Addition, subtraction and multiplication are always exact. Division is not -
`1 / 3` has no finite decimal form - so `div` makes the choice explicit:

- **`div(a, b)`** (no scale) returns the exact quotient when it has a finite
  decimal form, and throws `Error{kind: "decimal"}` otherwise. `div(1, 8)` is
  `0.125`; `div(1, 3)` throws. The result keeps the operands' scale difference
  (`a`'s scale minus `b`'s, at least 0) and gains places only where the quotient
  needs them, as Java's `BigDecimal` and Python's `decimal` do: `7.50 / 2.5` is
  `3.0`, `1 / 4` is `0.25`.
- **`div(a, b, scale[, mode])`** returns the quotient at exactly `scale` places,
  rounded by `mode` (default `"half-even"`): `div(10, 3, 4)` is `3.3333`,
  `div(2, 3, 2, "up")` is `0.67`.

Division by zero is `Error{kind: "decimal"}` either way.

## Rounding modes

`round` and `div` take an optional mode; **`"half-even"`** is the default.

| Mode | Rule | `2.5` | `-2.5` | `1.6` | `-1.1` |
| ---- | ---- | ----- | ------ | ----- | ------ |
| `"half-even"` | to nearest; a tie goes to the even neighbour (banker's rounding, no upward bias) | `2` | `-2` | `2` | `-1` |
| `"half-up"` | to nearest; a tie goes away from zero (schoolbook rounding) | `3` | `-3` | `2` | `-1` |
| `"half-down"` | to nearest; a tie goes toward zero | `2` | `-2` | `2` | `-1` |
| `"up"` | away from zero | `3` | `-3` | `2` | `-2` |
| `"down"` | toward zero (truncate) | `2` | `-2` | `1` | `-1` |
| `"ceiling"` | toward positive infinity | `3` | `-2` | `2` | `-1` |
| `"floor"` | toward negative infinity | `2` | `-3` | `1` | `-2` |

Rounding happens on the exact decimal value, so `round("2.675", 2)` is `2.68`
(half-even keeps the even 8) - unlike a float, where `2.675` is really
`2.67499999...`.

## Big integers

There is no separate big-integer type: a `Decimal` with scale 0 is one.
`decimal.pow(2, 100)` is `1267650600228229401496703205376`, and `add`, `sub`,
`mul`, `compare` and `isInteger` work past the `int` range. `toInt` converts
back when the value fits.

## Limits and errors

- `Error{kind: "decimal"}` - a malformed decimal string, division by zero, a
  scale-less division with no finite result, `toInt` of a fractional or
  too-large value, `toFloat` past the float range.
- `Error{kind: "limit"}` - a result with more than 100,000 digits (coefficient
  or scale), from `pow`, `mul` or a parsed exponent such as `"1e999999999"`, so a
  computation cannot exhaust memory.
- An argument of the wrong kind (a `float` or `bool` operand, an unknown
  rounding mode, a negative scale) is a positioned runtime error.

All are catchable with `try` / `catch`.

## Not here

Square roots, logarithms and other irrational functions (no finite decimal
result; use `math` on a `float`); a global precision or rounding context (every
rounding is stated at the call); locale-aware formatting with digit grouping or
a decimal comma (an open `printf` question). Currency amounts with per-currency
minor units, allocation and conversion are planned as a `money` deck on top of
this library.
