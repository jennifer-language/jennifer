# `bloom` - Bloom filter (probabilistic set)

Import with `import "bloom.j" as bloom;`. A compact, probabilistic set: `add`
records a string and `mightContain` tests membership with **no false negatives**
(a member always reports present) but possible **false positives** (a non-member
may report present, with a probability that grows as the filter fills). Trades a
little accuracy for a lot of space - ideal for "have I seen this before?" checks
over large sets. Pure `.j` over `hash` + `bytes`; runs on both binaries.

```jennifer
import "bloom.j" as bloom;

def f as bloom.Filter init bloom.new(1024, 4);
$f = bloom.add($f, "alice");
$f = bloom.add($f, "bob");
bloom.mightContain($f, "alice");   # true
bloom.mightContain($f, "carol");   # almost always false
```

Runnable: [`examples/modules/bloom_demo.j`](https://github.com/mplx/jennifer-lang/blob/main/examples/modules/bloom_demo.j).

## Surface

```jennifer
def struct bloom.Filter { bits as bytes, size as int, hashes as int };
```

| Call | Returns | |
| ---- | ------- | - |
| `bloom.new(size, hashes)` | `Filter` | an empty filter of `size` bits and `hashes` hash functions (both >= 1) |
| `bloom.add(f, item)` | `Filter` | a fresh filter with `item` recorded |
| `bloom.addAll(f, items)` | `Filter` | a fresh filter with every item of a `list of string` recorded |
| `bloom.mightContain(f, item)` | `bool` | `true` if the item might be present, `false` if it is definitely absent |

The `hashes` bit positions per item come from **double-hashing one SHA-256
digest** - `pos_i = (h1 + i*h2) mod size`, where `h1` / `h2` are the first two
32-bit words of the digest - so one hash yields all *k* positions.

## Choosing size and hashes

Bigger `size` and a well-chosen `hashes` lower the false-positive rate. Rules of
thumb for `n` expected items at a target false-positive probability `p`:

- **bits** `m ~= -n * ln(p) / (ln 2)^2` (about `9.6 * n` bits for `p = 1%`,
  `14.4 * n` for `p = 0.1%`).
- **hashes** `k ~= (m / n) * ln 2` (about 7 for `p = 1%`).

So for 10000 items at 1%: `size ~= 96000`, `hashes = 7`.

## Scope

- **Add and test only** - a standard Bloom filter cannot remove an item or count
  occurrences (removal needs a counting Bloom filter; membership only, here).
- **Value-semantic `add`.** Each `add` copies the bit array and returns a fresh
  filter, so chain adds (`$f = bloom.add($f, x)`); it does not mutate in place.
  For many inserts this copies the array each time - fine for typical set sizes,
  not for millions of adds into a huge filter in a tight loop.
- **Strings only.** Hash other values through `convert.toString` or a `json` /
  `encoding` representation first.
- **Non-crypto use.** The filter is a set membership structure, not a security
  primitive.

## See also

- [hash.md](../libraries/hash.md) - the SHA-256 the positions derive from.
- [ringbuffer.md](ringbuffer.md) - the sibling data-structure module.
- [modules/index.md](index.md) - the module catalog and import rules.
