# `compress` - byte-stream compression

Enable with `use compress;`. gzip, zlib, and raw DEFLATE - `bytes` in,
`bytes` out - plus a streaming compressor for large data. The algorithm
is a string argument, the same shape as [`hash`](hash.md)`.compute(b,
"sha-256")` and [`crc`](crc.md)`.compute(b, "crc32")`; the `pack` /
`unpack` verbs pair with [`archive`](archive.md)'s (byte streams here,
file bundles there). Distinct from [`encoding`](encoding.md), which is
*reversible representation* (hex / base64, which don't reduce
information); this is entropy-based *size reduction*. Backed by Go's
`compress/*`; works on both binaries.

## Surface

| Call                              | Returns           | Notes                                                          |
| --------------------------------- | ----------------- | -------------------------------------------------------------- |
| `compress.pack(b, algo [, level])`| `bytes`           | Compress; `algo` is `"gzip"` / `"zlib"` / `"deflate"`.         |
| `compress.unpack(b, algo [, maxBytes])` | `bytes`     | Decompress; same `algo`. Output is capped at `maxBytes` (default 256 MiB). |
| `compress.stream(algo [, level])` | `compress.Stream` | Start a streaming compressor.                                  |
| `compress.update(stream, b)`      | `null`            | Feed one chunk of input.                                       |
| `compress.finalize(stream)`       | `bytes`           | Close and return the full compressed output.                   |
| `compress.discard(stream)`        | `null`            | Drop a stream without flushing/returning output, releasing its state. Subsequent calls error. |

`level` is optional: `"fast"`, `"default"` (when omitted), or `"best"`.
Decompressing malformed input, or an unknown `algo`, is a positioned
runtime error (catchable with `try` / `catch`).

**Output cap.** A few hundred kilobytes of compressed input can expand to
gigabytes, so `unpack` stops as soon as the output passes a cap and raises a
catchable `Error{kind: "limit"}` naming it (the same kind
[`archive`](archive.md) uses). The default cap is 256 MiB (268435456 bytes);
reaching it can cost roughly twice that in peak memory while the output buffer
grows. Pass `maxBytes` (an `int`, at least `1`) to set your own: the size the
data declares, or a sane maximum, when the input is untrusted - a compressed
frame from a file, an HTTP body with `Content-Encoding: gzip`:

```jennifer
try {
    def body as bytes init compress.unpack($raw, "gzip", 16777216);   # 16 MiB
} catch (e) {
    # e.kind == "limit": the body expands past 16 MiB
}
```

`maxBytes` may also raise the cap above the default for trusted input.

The three algorithms:

- **`"gzip"`** (RFC 1952) - magic + CRC-32 + size. `gzip(1)`-compatible;
  the reliable HTTP `Content-Encoding: gzip`. Use for standalone files.
- **`"zlib"`** (RFC 1950) - a compact DEFLATE + Adler-32 wrapper. This is
  what HTTP `Content-Encoding: deflate` *officially* means (and it's the
  wrapper inside PNG, zip entries, etc.) - but that coding is
  inconsistently implemented in the wild, so prefer `"gzip"` for HTTP.
- **`"deflate"`** (RFC 1951) - raw DEFLATE, no framing. Smallest, when
  you supply your own framing. **Not** the HTTP `deflate` content-coding
  despite the name - that one is `"zlib"`.

## One-shot

```jennifer
use io;
use compress;
use convert;

def raw as bytes init convert.bytesFromString("hello hello hello world", "utf-8");
def packed as bytes init compress.pack($raw, "gzip", "best");
def back as bytes init compress.unpack($packed, "gzip");
io.printf("%t\n", convert.stringFromBytes($back, "utf-8") == "hello hello hello world");
```

## Streaming

Feed input in chunks without holding it all at once; the full compressed
result comes back at `finalize`:


```jennifer
def s as compress.Stream init compress.stream("gzip");
compress.update($s, chunkOne);
compress.update($s, chunkTwo);
def packed as bytes init compress.finalize($s);   # equals compress.pack of the concatenation
```

`compress.Stream` is a handle (like `hash.Stream`): copies share the
underlying state, and `finalize` consumes it - a second `finalize` or
`update` on the same handle errors.

## See also

- [encoding.md](encoding.md) - hex / base64 (representation, not compression).
- [hash.md](hash.md) - the same algo-string + streaming-handle shape.
- [archive.md](archive.md) - `pack` / `unpack` for file bundles (`"tar"` / `"zip"`).
