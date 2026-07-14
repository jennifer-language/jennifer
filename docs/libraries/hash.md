# `hash` - cryptographic-style digests

Enable with `use hash;`. Computes fixed-size digests over `bytes`
using common cryptographic-style algorithms (MD5, SHA-1, SHA-256).
Output is raw `bytes`; users hex- or base64-encode through the
`encoding` library when they need a string representation.

For non-cryptographic checksums (transport integrity rather than
content addressing), use [`crc`](crc.md) instead. The split makes
the semantic difference visible at the import line.

```jennifer
use io;
use convert;
use hash;

def input as bytes init convert.bytesFromString("abc", "utf-8");
def digest as bytes init hash.compute($input, "sha256");
io.printf("sha256 digest is %d bytes\n", len($digest));
```

## Algorithm selection

The library uses the codec-table shape - one verb per category,
with the algorithm passed as a string. The shape mirrors
`convert.bytesFromString(s, "utf-8")` and
`encoding.encode(s, codec)`. Algorithm names are lowercase.

| Algo string | Output width | Notes                                                            |
| ----------- | ------------ | ---------------------------------------------------------------- |
| `"md5"`     | 16 bytes     | Broken for collision resistance; useful for integrity / caching. |
| `"sha1"`    | 20 bytes     | Broken for collision resistance; still common in legacy formats. |
| `"sha256"`  | 32 bytes     | The default choice for new code.                                 |
| `"sha512"`  | 64 bytes     | Wider SHA-2 digest; used by some HMAC / TOTP variants.           |

Passing an unknown algorithm is a positioned runtime error that
lists the supported set:
`hash.compute: unknown digest algorithm "md4"; known: "md5", "sha1", "sha256", "sha512"`.

## One-shot

| Call                          | Returns | Notes                                  |
| ----------------------------- | ------- | -------------------------------------- |
| `hash.compute(b, algo)`       | `bytes` | Full digest of the entire input.       |
| `hash.hmac(key, message, algo)` | `bytes` | Keyed-hash MAC (RFC 2104) over the same algorithms. |

## HMAC

`hash.hmac(key, message, algo)` computes the keyed-hash message authentication
code (RFC 2104) - the primitive behind JWT (HS256), TOTP, AWS SigV4, and webhook
signatures. `key` and `message` are `bytes`; the result is the raw MAC as
`bytes` (hex / base64 via [`encoding`](encoding.md), matching `compute`).

```jennifer
use hash;
use convert;
use encoding;

def key as bytes init convert.bytesFromString("secret", "utf-8");
def msg as bytes init convert.bytesFromString("payload", "utf-8");
def mac as bytes init hash.hmac($key, $msg, "sha256");
io.printf("%s\n", encoding.toText($mac, "hex"));
```

To **verify**, recompute the MAC over the same message and compare it to the
received one (comparing the full digests, not a prefix).

## Streaming

For inputs that don't fit comfortably in memory (files, large
network reads), feed chunks into a stream handle and finalize:

```jennifer
use hash;
def s as hash.Stream init hash.stream("sha256");
hash.update($s, $chunkOne);
hash.update($s, $chunkTwo);
def digest as bytes init hash.finalize($s);
```

| Call                          | Returns       | Notes                                                              |
| ----------------------------- | ------------- | ------------------------------------------------------------------ |
| `hash.stream(algo)`           | `hash.Stream` | Allocate a fresh handle for the named algorithm.                   |
| `hash.update($s, $bytes)`     | `null`        | Feed one chunk. Mutates the handle's state by side effect.         |
| `hash.finalize($s)`           | `bytes`       | Compute the digest and consume the handle. Subsequent calls error. |

`hash.Stream` carries an integer `id` field that indexes into a
Go-side map of live state. Users should not read or mutate the
field; pass the struct around as an opaque token.

## Composing through `convert` and (future) `encoding`

No convenience wrappers like `hash.md5String(s)` ship. Stance #1
"one way per thing": strings become bytes through
`convert.bytesFromString`, the digest stays as bytes, and the user
hex-encodes through `encoding.hex`. The example
[`examples/hash.j`](https://github.com/mplx/jennifer-lang/blob/main/examples/hash.j) carries a tiny inline
`bytesToHex` helper for printing until `encoding` ships.

## Errors

- Wrong arity: `hash.compute expects 2 arguments (bytes, algo), got 1`.
- Wrong scalar type:
  `hash.compute: first argument must be bytes, got string`.
- Unknown algorithm:
  `hash.compute: unknown digest algorithm "md4"; known: "md5", "sha1", "sha256"`.
- Reuse of a finalized stream:
  `hash.update: stream 3 has already been finalized or never existed`.

All errors are positioned at the call site.

## See also

- [crc.md](crc.md) - non-cryptographic checksums (CRC-32, CRC-64).
- [milestones.md](../milestones.md) - hash/crc, `encoding` for
  hex/base64 round-trips, and key-based crypto.
