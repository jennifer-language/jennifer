# `crc` - cyclic redundancy checks

Enable with `use crc;`. Computes CRC-32 (IEEE polynomial) and
CRC-64/XZ checksums over `bytes`,
matching the codec-table shape used by [`hash`](hash.md). Output
is the natural-width digest in big-endian byte order.

CRCs are designed to catch transport / storage corruption, not to
resist deliberate tampering. For content addressing or signatures
use [`hash`](hash.md) (`"sha256"` or wider). The library
split keeps the difference visible at the import line.

```jennifer
use io;
use convert;
use crc;

def input as bytes init convert.bytesFromString("utf-8", "abc");
def sum as bytes init crc.compute("crc32", $input);
io.printf("crc32 checksum is %d bytes\n", len($sum));
```

## Algorithm selection

| Algo string | Output width | Polynomial                                    |
| ----------- | ------------ | --------------------------------------------- |
| `"crc32"`   | 4 bytes      | IEEE 802.3 (CRC-32/ISO-HDLC).                 |
| `"crc64"`   | 8 bytes      | CRC-64/XZ (Go crc64.ECMA table).              |

`"crc64"` is CRC-64/XZ (also called CRC-64/GO-ECMA): ECMA-182 polynomial,
reflected, init and xorout all ones; check value `995dc9bbdf1939fa` for
`"123456789"`. CRC-64/ECMA-182 (`6c40df5f0b497347`) is a different
parameterisation and is not provided.

Output bytes are big-endian (network byte order), matching the display
of `Sum()` from Go's CRC types.


Passing an unknown algorithm is a positioned runtime error:
`crc.compute: unknown algorithm "adler32"; known: "crc32", "crc64"`.

## One-shot

| Call                       | Returns | Notes                                                          |
| -------------------------- | ------- | -------------------------------------------------------------- |
| `crc.compute(algo, b)`     | `bytes` | Full checksum of the entire input. Big-endian.                 |

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
| `crc.discard($s)`          | `null`       | Drop a stream without computing its checksum, releasing its state. Subsequent calls error. |

At most 4096 streams are open at once; each needs a matching `finalize` or
`discard`.

## Errors

- Wrong arity: `crc.compute expects 2 arguments (algo, bytes), got 1`.
- Wrong scalar type:
  `crc.compute: second argument must be bytes, got string`.
- Unknown algorithm:
  `crc.compute: unknown algorithm "adler32"; known: "crc32", "crc64"`.
- Reuse of a finalized stream:
  `crc.update: stream 3 has already been finalized or never existed`.
- Leaked streams:
  `crc.stream: too many open streams (limit 4096); each needs a matching crc.finalize or discard`.

All errors are positioned at the call site.

## See also

- [hash.md](hash.md) - cryptographic-style digests (MD5, SHA-1, SHA-2).
- [encoding.md](encoding.md) - hex / base64 rendering of a checksum.
