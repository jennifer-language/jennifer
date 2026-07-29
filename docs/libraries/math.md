# `math` - numeric functions and constants

Enable with `use math;`. A small set of frequently-needed numeric functions
plus the constants `math.PI` and `math.E`. The library is strict on undefined
inputs - anything that would produce `NaN` or `Infinity` in IEEE arithmetic
instead produces a positioned runtime error.

```jennifer
use io;
use math;

io.printf("%f\n", math.PI);                    # 3.141592653589793
io.printf("%d\n", math.abs(0 - 42));           # 42
io.printf("%d\n", math.min(3, 7));             # 3
io.printf("%f\n", math.sqrt(2));               # 1.4142135623730951
io.printf("%f\n", math.pow(2, 10));            # 1024.0
io.printf("%d\n", math.floor(3.7));            # 3
io.printf("%d\n", math.ceil(3.2));             # 4
io.printf("%d\n", math.round(2.5));            # 3 (half away from zero)
```

## Functions

| Call             | Returns          | Notes                                         |
| ---------------- | ---------------- | --------------------------------------------- |
| `math.abs(x)`    | same type as `x` | int → int, float → float; errors on `math.abs(MinInt64)` (no representable result) |
| `math.min(a, b)` | int or float     | int+int → int; mixed → float                  |
| `math.max(a, b)` | int or float     | same rule as `min`                            |
| `math.sqrt(x)`   | float            | errors on negative input                      |
| `math.pow(x, y)` | float            | errors if the result would be NaN or Infinity |
| `math.floor(x)`  | int              | toward `-∞`; accepts int (identity); errors if the result does not fit in a 64-bit int (NaN / Inf / out of range) |
| `math.ceil(x)`   | int              | toward `+∞`; same int-range error as `floor`  |
| `math.round(x)`  | int              | half-away-from-zero (`math.round(2.5)` = `3`); same int-range error as `floor` |

`min`/`max` follow the same numeric-promotion rule as `+`: same-type
arguments return that type; any `float` involved produces a `float`.

## Randomness

| Call                    | Returns | Notes                                          |
| ----------------------- | ------- | ---------------------------------------------- |
| `math.rand()`           | float   | uniform in `[0, 1)`                            |
| `math.randInt(lo, hi)`  | int     | uniform in `[lo, hi]` inclusive; errors if `lo > hi` |
| `math.randSeed(n)`      | null    | reseeds the shared source deterministically    |

The library keeps one shared pseudo-random source. It is seeded from OS
entropy at startup, so every run produces a different stream; call
`math.randSeed(n)` when you need reproducible output (tests, demos,
simulations). The same source drives `lists.shuffle`, so one `randSeed`
call makes both deterministic together. (`uuid` and `password` draw from
the [`crypto`](crypto.md) library's crypto-grade source instead, so
`randSeed` does not affect them.)

The source is **not cryptographically secure** - it is Go's `math/rand`,
seedable and predictable once observed. Don't derive secrets, tokens, keys,
or anything security-sensitive from it.

```jennifer
use io;
use math;

math.randSeed(42);                              # reproducible from here on
io.printf("%f\n", math.rand());                 # same value every run
io.printf("%d\n", math.randInt(1, 6));          # die roll, seeded
```

### For secure random numbers, use `crypto`

When the value must be unpredictable - a password, a token, a nonce, a
shuffle an adversary must not guess - use the [`crypto`](crypto.md)
library's crypto-grade source instead of `math`. `crypto.randInt(lo, hi)`
has the **same shape** as `math.randInt` (inclusive `[lo, hi]`), so it is a
drop-in replacement; it is unseedable by design (there is no
`crypto.randSeed`, since predictability is the thing you are avoiding):

```jennifer
use io;
use crypto;

io.printf("%d\n", crypto.randInt(1, 6));        # secure, unbiased die roll
def token as bytes init crypto.randBytes(32);   # 256 bits of secure entropy
```

If you specifically want `math`'s fast, `lists.shuffle`-sharing stream but
seeded **unpredictably** (so each run differs and the seed can't be guessed),
draw the seed from `crypto` once at startup:

```jennifer
use math;
use crypto;

# a full-width unpredictable seed from the crypto-grade source
math.randSeed(crypto.randInt(0 - 9223372036854775807, 9223372036854775807));

# math.rand / math.randInt / lists.shuffle now run off an unguessable seed -
# fast and non-reproducible, but still NOT cryptographically secure output.
```

Note the trade-off: seeding `math` from `crypto` only makes the *starting
point* unpredictable. The stream itself is still `math/rand`, so once enough
output is observed it remains predictable - for output that must stay secret,
use `crypto.randInt` / `crypto.randBytes` directly.

## Constants

| Name      | Kind  | Value                |
| --------- | ----- | -------------------- |
| `math.PI` | float | 3.141592653589793... |
| `math.E`  | float | 2.718281828459045... |

Constants are referenced through the `math.` namespace prefix like
every other library name; the bare identifiers `PI` and `E`
are not in scope. With `use math as m;` the alias takes over
(`m.PI`, `m.E`).

## Strictness

The library refuses to produce floating-point edge values:

- `math.sqrt(-1)` - undefined for negative input.
- `math.pow(0, -1)` - division-by-zero territory; result would be Infinity.
- `math.pow(-1, 0.5)` - would be NaN.

If a future use case needs the NaN/Infinity values, a `math.NAN` / `math.INF`
constant (or dedicated check functions) can be added later. For now Jennifer
treats them as errors at the boundary - consistent with how the language
already refuses to silently coerce types.

See also: [../user-guide/index.md](../user-guide/index.md), [../technical/interpreter.md](../technical/interpreter.md#builtins-and-libraries), [index.md](index.md).
