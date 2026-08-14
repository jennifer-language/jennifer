# `bloom` - Bloom filter (probabilistic set)

Import with `import "bloom.j" as bloom;`. A compact, probabilistic set: `add`
records a string and `mightContain` tests membership with **no false negatives**
(a member always reports present) but possible **false positives** (a non-member
may report present, with a probability that grows as the filter fills). Trades a
little accuracy for a lot of space - ideal for "have I seen this before?" checks
over large sets. Pure `.j` over `hash` + `strings` + `binary`; runs on both binaries.

```jennifer
import "bloom.j" as bloom;

def f as bloom.Filter init bloom.new(1024, 4);
$f = bloom.add($f, "alice");
$f = bloom.add($f, "bob");
bloom.mightContain($f, "alice");   # true
bloom.mightContain($f, "carol");   # almost always false
```

Runnable: [`examples/modules/bloom_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/bloom_demo.j).

## Surface

```jennifer
def struct bloom.Filter { bits as bytes, size as int, hashes as int };
```

| Call | Returns | |
| ---- | ------- | - |
| `bloom.new(size, hashes)` | `Filter` | an empty filter of `size` bits and `hashes` hash functions (both >= 1) |
| `bloom.optimal(n, fpr)` | `Filter` | an empty filter sized for `n` expected items at target false-positive rate `fpr` (`n >= 1`, `0 < fpr < 1`) |
| `bloom.add(f, item)` | `Filter` | a fresh filter with `item` recorded |
| `bloom.addAll(f, items)` | `Filter` | a fresh filter with every item of a `list of string` recorded |
| `bloom.mightContain(f, item)` | `bool` | `true` if the item might be present, `false` if it is definitely absent |
| `bloom.serialize(f)` | `bytes` | encode a filter to bytes (4-byte `size`, 4-byte `hashes`, then the bit array) |
| `bloom.deserialize(b)` | `Filter` | reconstruct a filter from `serialize` output (identical filter, membership preserved) |
| `bloom.union(a, b)` | `Filter` | a fresh filter holding the members of both `a` and `b` (same `size` and `hashes` required) |
| `bloom.merge(a, b)` | `Filter` | alias for `bloom.union` |

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

**`bloom.optimal(n, fpr)` does this arithmetic for you** - it computes
`m = ceil(-(n * ln fpr) / (ln 2)^2)` and `k = round((m / n) * ln 2)` (clamped to
`k >= 1`) and returns an empty filter of that shape. So `bloom.optimal(1000,
0.01)` yields a `size = 9586`, `hashes = 7` filter with no manual sizing.

## Serialization and union

`bloom.serialize` encodes a filter to a self-describing `bytes` blob (a 4-byte
big-endian `size`, a 4-byte big-endian `hashes`, then the raw bit array), and
`bloom.deserialize` reverses it exactly - `serialize` then `deserialize`
round-trips to a byte-identical filter with membership preserved, so a filter can
be written to disk (`fs`) or sent over the wire and rebuilt later.

`bloom.union` (alias `bloom.merge`) bitwise-ORs two filters of the **same `size`
and `hashes`** into a fresh filter: an item present in either input is present in
the union. Filters that disagree on `size` or `hashes` cannot be combined and
throw an `Error{kind: "bloom"}`.

```jennifer
def a as bloom.Filter init bloom.addAll(bloom.new(2048, 5), ["alice", "bob"]);
def b as bloom.Filter init bloom.addAll(bloom.new(2048, 5), ["carol"]);
def both as bloom.Filter init bloom.union($a, $b);   # holds alice, bob, carol

def blob as bytes init bloom.serialize($both);
def restored as bloom.Filter init bloom.deserialize($blob);   # identical filter
```

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
