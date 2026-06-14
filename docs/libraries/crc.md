# `crc` - cyclic redundancy checks

Enable with `use crc;`. Computes CRC-32 (IEEE polynomial) and
CRC-64 (Go's `crc64.ECMA` polynomial) checksums over `bytes`,
matching the codec-table shape used by [`hash`](hash.md). Output
is the natural-width digest in big-endian byte order.

CRCs are designed to catch transport / storage corruption, not to
resist deliberate tampering. For content-addressing or signature
work, use [`hash`](hash.md) (MD5, SHA-1, SHA-256). The library
split keeps the difference visible at the import line.

```jennifer
use io;
use convert;
use crc;

def input as bytes init convert.bytesFromString("abc", "utf-8");
def sum as bytes init crc.compute($input, "crc32");
io.printf("crc32 checksum is %d bytes\n", len($sum));
```

## Algorithm selection

| Algo string | Output width | Polynomial                                    |
| ----------- | ------------ | --------------------------------------------- |
| `"crc32"`   | 4 bytes      | IEEE 802.3 (Go `crc32.IEEE`).                 |
| `"crc64"`   | 8 bytes      | Go `crc64.ECMA` (0xC96C5795D7870F42).         |

Note: the "CRC-64/XZ" vector you may see elsewhere
(`6c40df5f0b497347` for `"123456789"`) uses a different
polynomial. We ship Go's stdlib choice (`995dc9bbdf1939fa` for
the same input).

Output bytes are big-endian (network byte order). The convention
matches the natural display of `Sum()` from Go's CRC types and
removes any ambiguity at the call site.

Passing an unknown algorithm is a positioned runtime error:
`crc.compute: unknown algorithm "adler32"; known: "crc32", "crc64"`.

## One-shot

| Call                       | Returns | Notes                                                          |
| -------------------------- | ------- | -------------------------------------------------------------- |
| `crc.compute(b, algo)`     | `bytes` | Full checksum of the entire input. Big-endian.                 |

## Streaming

Feed chunks into a stream handle and finalize. Same shape as
[`hash`](hash.md):

```jennifer
use crc;
def s as crc.Stream init crc.stream("crc32");
crc.update($s, $chunkOne);
crc.update($s, $chunkTwo);
def sum as bytes init crc.finalize($s);
```

| Call                       | Returns      | Notes                                                              |
| -------------------------- | ------------ | ------------------------------------------------------------------ |
| `crc.stream(algo)`         | `crc.Stream` | Allocate a fresh handle for the named algorithm.                   |
| `crc.update($s, $bytes)`   | `null`       | Feed one chunk. Mutates the handle's state by side effect.         |
| `crc.finalize($s)`         | `bytes`      | Compute the checksum and consume the handle. Subsequent calls error. |

## Errors

- Wrong arity: `crc.compute expects 2 arguments (bytes, algo), got 1`.
- Wrong scalar type:
  `crc.compute: first argument must be bytes, got string`.
- Unknown algorithm:
  `crc.compute: unknown algorithm "adler32"; known: "crc32", "crc64"`.
- Reuse of a finalized stream:
  `crc.update: stream 3 has already been finalized or never existed`.

All errors are positioned at the call site.

## See also

- [hash.md](hash.md) - cryptographic-style digests (MD5, SHA-1, SHA-256).
- [milestones.md](../milestones.md) - M15.6 ships hash/crc.
