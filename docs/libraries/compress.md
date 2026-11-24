# `compress` - byte-stream compression

Enable with `use compress;`. gzip, zlib, and raw DEFLATE (`bytes` in,
`bytes` out), plus a streaming compressor for large data. The algorithm
is a string argument and comes first, the same shape as
[`hash`](hash.md)`.compute("sha256", b)` and [`crc`](crc.md)`.compute("crc32", b)`; the `pack` /
`unpack` verbs pair with [`archive`](archive.md)'s (byte streams here,
file bundles there). Distinct from [`encoding`](encoding.md), which is
*reversible representation* (hex / base64, which don't reduce
information); this is entropy-based *size reduction*. Backed by Go's
`compress/*`; works on both binaries.

## Surface

| Call                              | Returns           | Notes                                                          |
| --------------------------------- | ----------------- | -------------------------------------------------------------- |
| `compress.pack(algo, b[, level])`| `bytes`           | Compress; `algo` is `"gzip"` / `"zlib"` / `"deflate"`.         |
| `compress.unpack(algo, b)`        | `bytes`           | Decompress; same `algo`. Output is capped at 256 MiB (32 MiB on `jennifer-tiny`). |
| `compress.unpackWith(algo, b, o)` | `bytes`           | Same, with the cap of a `compress.UnpackOptions{maxBytes}` (see below). |
| `compress.stream(algo [, level])` | `compress.Stream` | Start a streaming compressor.                                  |
| `compress.update(stream, b)`      | `null`            | Feed one chunk of input.                                       |
| `compress.finalize(stream)`       | `bytes`           | Close and return the full compressed output.                   |
| `compress.discard(stream)`        | `null`            | Drop a stream without flushing/returning output, releasing its state. On a finalized or discarded stream it does nothing, so `defer compress.discard($s);` is safe. |

`level` is optional: `"fast"`, `"default"` (when omitted), or `"best"`.

**Output cap.** A few hundred kilobytes of compressed input can expand to
gigabytes, so `unpack` stops as soon as the output passes a cap and raises a
catchable `Error{kind: "limit"}` naming it (the same kind
[`archive`](archive.md) uses). The default cap is 256 MiB (268435456 bytes;
32 MiB on `jennifer-tiny`). Reaching it can cost two to three times that in peak
memory: `gzip` sizes its output from the stream's size trailer, `zlib` and
`deflate` grow the buffer as data arrives, and binding the result copies it.
`compress.unpackWith` sets the cap per call: the size the data declares, or a
sane maximum, when the input is untrusted (a compressed frame from a file, an
HTTP body with `Content-Encoding: gzip`):

| `UnpackOptions.maxBytes` | Cap |
|---|---|
| `0` | the default (256 MiB; 32 MiB on `jennifer-tiny`) |
| `1` and up | that many bytes, also above the default for trusted input |
| `math.MAX_INT` | none: reads the whole stream |
| negative | a `"runtime"` error |

```jennifer
try {
    def body as bytes init compress.unpackWith("gzip", $raw, compress.UnpackOptions{maxBytes: 16777216});   # 16 MiB
} catch (e) {
    # e.kind == "limit": the body expands past 16 MiB
}
```

**After the stream.** `"zlib"` and `"deflate"` data is one stream: any byte
after its end (junk, padding, a second stream) is kind `"compress"`.
`"gzip"` data may hold several members (RFC 1952; `gzip -d` reads them, and
`cat a.gz b.gz` makes one): `unpack` returns the members' output joined, and
bytes after the last member that are not another member (zero padding
included) are kind `"compress"`.

| `algo` | Format | Use |
| ------ | ------ | --- |
| `"gzip"` | RFC 1952: magic + CRC-32 + size | `gzip(1)`-compatible; the reliable HTTP `Content-Encoding: gzip`. Standalone files. |
| `"zlib"` | RFC 1950: a compact DEFLATE + Adler-32 wrapper | What HTTP `Content-Encoding: deflate` officially means (and the wrapper inside PNG, zip entries, etc.), but that coding is inconsistently implemented in the wild, so prefer `"gzip"` for HTTP. |
| `"deflate"` | RFC 1951: raw DEFLATE, no framing | Smallest, when you supply your own framing. **Not** the HTTP `deflate` content-coding despite the name (that one is `"zlib"`). |

### Errors

| Kind | Raised for |
|---|---|
| `"compress"` | input `unpack` / `unpackWith` cannot decompress: a bad header, corrupt data, a CRC-32 / Adler-32 mismatch, a stream that ends early, any byte after a zlib or deflate stream, bytes after a gzip member that are not another member |
| `"limit"` | output past the cap, more than the open-stream cap |
| `"closed"` | `update` / `finalize` on a finalized or discarded stream (`compress.update: stream 3 has already been finalized or discarded, or never existed`) |
| `"runtime"` | misuse: a wrong argument, an unknown `algo` or `level`, a negative `maxBytes` |

### Deprecated

| Call | Instead | Removed |
|---|---|---|
| `compress.unpack(algo, b, maxBytes)` | `compress.unpackWith(algo, b, compress.UnpackOptions{maxBytes: ...})` | 0.29.0 |

The positional `maxBytes` still works and still refuses a value below 1.
Lint reports it as L301.

## One-shot

```jennifer
use encoding;
use io;
use compress;

def raw as bytes init encoding.encode("utf-8", "hello hello hello world");
def packed as bytes init compress.pack("gzip", $raw, "best");
def back as bytes init compress.unpack("gzip", $packed);
io.printf("%t\n", encoding.decode("utf-8", $back) == "hello hello hello world");
```

## Streaming

Feed input in chunks without holding it all at once; the full compressed
result comes back at `finalize`:

```jennifer
def s as compress.Stream init compress.stream("gzip");
compress.update($s, $chunkOne);
compress.update($s, $chunkTwo);
def packed as bytes init compress.finalize($s);   # equals compress.pack of the concatenation
```

`compress.Stream` is a handle (like `hash.Stream`): copies share the
underlying state, and `finalize` consumes it: a second `finalize` or
`update` on the same handle throws kind `"closed"`, a `discard` does nothing. Each open stream holds a compressor of up to
about 1 MiB, so at most 256 streams (32 on `jennifer-tiny`) are open at once;
`compress.stream` past that raises a catchable `Error{kind: "limit"}`.

## See also

- [encoding.md](encoding.md) - hex / base64 (representation, not compression).
- [hash.md](hash.md) - the same algo-string + streaming-handle shape.
- [archive.md](archive.md) - `pack` / `unpack` for file bundles (`"tar"` / `"zip"`).

## Academy talks

| Talk | What it shows |
| ---- | ------------- |
| [Jennifer facing hostile input](https://academy.jennifer-lang.dev/talks/mplx/security/) | a zip bomb stopped by a decompression size cap |
