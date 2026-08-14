# Cheatsheet - all builtins at a glance

Alphabetical index of every standard-library function and constant. Use
it when you know the *name* and want to know *which library* and *how to
call it*; use each library's own page when you want to read about a
topic. Each row's library prefix links to the per-library doc.

The table covers what ships with the interpreter. New
entries land here at the same time as the per-library doc - it's a
flat lookup view, not authoritative.

## Functions

| Call                                                  | What it does                                                                                                                        |
| ----------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| [`archive`](archive.md)`.pack(entries, format)`      | Bundle a `list of archive.Entry` into `bytes`; `format` `"tar"`/`"zip"`/`"tar.gz"`.                                              |
| [`archive`](archive.md)`.unpack(b, format)`          | Read a bundle back into a `list of archive.Entry`.                                                                                 |
| [`asn1`](asn1.md)`.decode(b)` / `.encode(v)`         | Parse BER `bytes` into an opaque `asn1.Value` / serialise one back to DER `bytes`.                                                 |
| [`asn1`](asn1.md)`.typeOf/tagClass/tagNumber/isConstructed(v[, ptr])` | Element type name / class / numeric tag / constructed-ness; pointer tokens are child indices (`"/0/2"`).       |
| [`asn1`](asn1.md)`.get(v[, ptr])` / `.has(v, ptr)` / `.length(v[, ptr])` | Sub-element as `asn1.Value` / whether a pointer resolves / child count of a constructed node.               |
| [`asn1`](asn1.md)`.asInt/asBool/asString/asBytes/asOid/isNull(v[, ptr])` | Read a leaf: integer/enum, boolean, string type or octets, raw octets, dotted OID, or whether `NULL`.       |
| [`asn1`](asn1.md)`.integer/enumerated/boolean/null/octetString(...)` | Build a primitive element.                                                                                       |
| [`asn1`](asn1.md)`.utf8String/printableString/ia5String/oid(s)` | Build a string / OID element.                                                                                         |
| [`asn1`](asn1.md)`.sequence(items)` / `.set(items)`  | Build a constructed element from a `list of asn1.Value`.                                                                           |
| [`asn1`](asn1.md)`.tagged(class, n, v)` / `.retag(class, n, v)` | EXPLICIT (wrap) / IMPLICIT (overwrite tag) context/application tagging; `class` `"context"`/`"application"`/... |
| [`binary`](binary.md)`.concat(a, b)`                 | Join two `bytes` into a fresh `bytes` (O(len a + len b); avoid in an accumulation loop - use `net.readAll`/`readN`).                |
| [`binary`](binary.md)`.contains(haystack, needle)`   | Whether `needle` occurs in `bytes` `haystack` (boolean sibling of `indexOf`).                                                       |
| [`binary`](binary.md)`.endsWith(b, suffix)`          | True iff `bytes` `b` ends with `suffix`.                                                                                            |
| [`binary`](binary.md)`.indexOf(haystack, needle)`       | Byte index of the first `needle` in `haystack`; `-1` if absent, `0` for an empty needle. Native-speed scan.                        |
| [`binary`](binary.md)`.join(parts [, sep])`          | Concatenate a `list of bytes` into one `bytes` in O(n) (optional `bytes` `sep` between pieces). The byte-data `strings.join`.       |
| [`binary`](binary.md)`.slice(b, start [, end])`      | Half-open byte range `[start, end)`; `end` defaults to `len(b)`. Out-of-range / `start>end` errors.                                |
| [`binary`](binary.md)`.split(b, sep)`                | Split `bytes` on a non-empty `sep` -> `list of bytes` (e.g. a MIME body on its boundary, one Go pass).                             |
| [`binary`](binary.md)`.startsWith(b, prefix)`        | True iff `bytes` `b` begins with `prefix`.                                                                                         |
| [`channel`](channel.md)`.make(capacity)`             | New `channel of T`; capacity 0 = unbuffered, n = buffered. Type from the binding.                                                   |
| [`channel`](channel.md)`.send(ch, value)`            | Deep-copy `value` in and send (blocks per capacity); send on a closed channel is a catchable error.                                 |
| [`channel`](channel.md)`.recv(ch)`                   | Block and return the next value; throws (catchable) on a closed and drained channel.                                                |
| [`channel`](channel.md)`.close(ch)`                  | Close the channel; double-close is a catchable error. Buffered values still drain.                                                  |
| [`channel`](channel.md)`.select(chs)`                | Fan-in: next value from any open channel; throws when all are closed and drained.                                                   |
| [`channel`](channel.md)`.len(ch)` / `.capacity(ch)`  | Buffered value count / buffer capacity.                                                                                             |
| [`compress`](compress.md)`.discard(stream)`          | Drop a streaming compressor without returning output; releases its state.                                                          |
| [`compress`](compress.md)`.finalize(stream)`         | Close a streaming compressor; returns all compressed `bytes`.                                                                      |
| [`compress`](compress.md)`.pack(b, algo [, level])`  | Compress `bytes`; `algo` `"gzip"`/`"zlib"`/`"deflate"`, optional level `"fast"`/`"default"`/`"best"`.                                |
| [`compress`](compress.md)`.stream(algo [, level])`   | Start a streaming compressor -> `compress.Stream`.                                                                                 |
| [`compress`](compress.md)`.unpack(b, algo)`          | Decompress `bytes` with `algo`.                                                                                                    |
| [`compress`](compress.md)`.update(stream, b)`        | Feed one chunk into a streaming compressor.                                                                                        |
| [`convert`](convert.md)`.fromCodepoint(n)`            | One-rune string for Unicode code point `n` (whole range, 1-4 UTF-8 bytes); errors on out-of-range / surrogate.                      |
| [`convert`](convert.md)`.toBool(v)`                   | Canonical conversion to `bool` (`0`/`1`, `0.0`/`1.0`, `"true"`/`"false"`).                                                          |
| [`convert`](convert.md)`.toCodepoint(char)`           | Unicode code point (int) of a one-rune string; errors unless exactly one code point (not a grapheme cluster).                       |
| [`convert`](convert.md)`.toFloat(v)`                  | Convert to float (int→float, float identity, string parses, bool→1.0/0.0).                                                          |
| [`convert`](convert.md)`.toInt(v)`                    | Convert to int (float truncates toward zero, string parses, bool→1/0).                                                              |
| [`convert`](convert.md)`.toString(v)`                 | Convert to string (always succeeds; uses the value's display form).                                                                 |
| [`convert`](convert.md)`.typeOf(v)`                   | Runtime kind as string (`"int"`, `"float"`, `"string"`, `"bool"`, `"null"`, `"list"`, `"map"`, `"object"`).                         |
| [`convert`](convert.md)`.objectType(v)`               | Specific registered name of an opaque object (e.g. `"json.Value"`); errors on a non-object.                                         |
| [`crc`](crc.md)`.compute(b, algo)`                    | One-shot checksum. `algo` is `"crc32"` or `"crc64"`. Returns big-endian bytes (4 or 8).                                             |
| [`crc`](crc.md)`.discard($s)`                         | Drop a `crc.Stream` without computing its checksum; releases its state.                                                             |
| [`crc`](crc.md)`.finalize($s)`                        | Final checksum as big-endian bytes; consumes the handle.                                                                            |
| [`crc`](crc.md)`.stream(algo)`                        | Allocate a `crc.Stream` for `algo`; feed chunks via `crc.update` then close with `crc.finalize`.                                    |
| [`crc`](crc.md)`.update($s, $bytes)`                  | Feed one chunk into a `crc.Stream` (mutates by side effect).                                                                        |
| [`crypto`](crypto.md)`.encrypt(key, plaintext)` / `.decrypt(key, box)` | AES-256-GCM authenticated encryption. 32-byte `key`; a fresh nonce is prepended (`nonce\|\|ct\|\|tag`). decrypt of a tampered / wrong-key box is a catchable auth error. |
| [`crypto`](crypto.md)`.hkdf(secret, salt, info, length, algo)` | Derive `length` bytes from a high-entropy secret (HKDF, RFC 5869); `algo` `"sha1"`/`"sha256"`/`"sha512"`; `salt`/`info` may be empty.  |
| [`crypto`](crypto.md)`.hmacEqual(a, b)`               | Constant-time equality of two `bytes` (MAC comparison); unequal lengths return `false`.                                             |
| [`crypto`](crypto.md)`.signKeypair()` / `.sign($priv, msg)` / `.verify($pub, msg, sig)` | Ed25519. `signKeypair` -> `crypto.Keypair{public, private}`; sign -> 64-byte signature; verify -> bool (error only on a wrong-length key/sig). |
| [`crypto`](crypto.md)`.rsaSign($privPem, msg, algo)` / `.rsaVerify($pubPem, msg, sig, algo)` | RSASSA-PKCS#1 v1.5 over PEM keys (for JWT RS\*); `algo` `"sha256"`/`"sha384"`/`"sha512"`. **Default binary only.** |
| [`crypto`](crypto.md)`.ecdsaSign($privPem, msg, algo)` / `.ecdsaVerify($pubPem, msg, sig, algo)` | ECDSA over PEM keys, JOSE R\|\|S signature (for JWT ES\*); curve from the key. **Default binary only.** |
| [`crypto`](crypto.md)`.rsaGenerateKey(bits)` / `.ecGenerateKey(curve)` | Generate an RSA (2048/3072/4096) / EC (`"p256"`/`"p384"`/`"p521"`) private key as PEM. **Default binary only.** |
| [`crypto`](crypto.md)`.jwkPublic($privPem)` / `.jwkToPem($jwkJson)` / `.csr($privPem, domains)` | Canonical public JWK JSON (RFC 7638; SHA-256 it for the thumbprint); the inverse `jwkToPem` (public JWK -> `PUBLIC KEY` PEM for `rsaVerify` / `ecdsaVerify`, so a JWKS `kid` resolves to a key); DER PKCS#10 CSR over a `list of string` of domains. For ACME / JWT. **Default binary only.** |
| [`crypto`](crypto.md)`.pbkdf2(password, salt, iterations, keyLen, algo)` | Stretch a password into a `keyLen`-byte key (PBKDF2, RFC 8018); `algo` `"sha1"`/`"sha256"`/`"sha512"`.      |
| [`crypto`](crypto.md)`.randBytes(n)`                  | `n` crypto-grade random bytes (`n >= 0`).                                                                                           |
| [`crypto`](crypto.md)`.randInt(lo, hi)`               | Uniform crypto-grade int in the inclusive range `[lo, hi]` (rejection-sampled, unbiased; unseedable).                               |
| [`encoding`](encoding.md)`.codecs()`                  | Canonical character-codec names in registration order.                                                                              |
| [`encoding`](encoding.md)`.decode(b, codec)`          | Decode `bytes` from a character codec to a Jennifer string.                                                                         |
| [`encoding`](encoding.md)`.encode(s, codec)`          | Encode a Jennifer string into a character codec's bytes.                                                                            |
| [`encoding`](encoding.md)`.fromText(s, format)`       | Decode a binary-to-text format. `format`: `"hex"`, `"base32"`, `"base32-hex"`, `"base64"`, `"base64-url"`, `"ascii85"`, `"z85"`, `"quoted-printable"`, `"uri-percent"`, `"uri-form"`.                                                     |
| [`encoding`](encoding.md)`.isAscii(b)`                | True iff every byte in `b` is < 0x80.                                                                                               |
| [`encoding`](encoding.md)`.lenBytes(s)`               | UTF-8 byte length of `s` (pair with `len(s)` for rune count).                                                                       |
| [`encoding`](encoding.md)`.lenRunes(b)`               | Rune count of valid UTF-8 `bytes`; errors on invalid UTF-8.                                                                         |
| [`encoding`](encoding.md)`.toText(b, format)`         | Encode `bytes` as printable text. `format`: `"hex"`, `"base32"`, `"base32-hex"`, `"base64"`, `"base64-url"`, `"ascii85"`, `"z85"`, `"quoted-printable"`, `"uri-percent"`, `"uri-form"`.                                                   |
| [`fs`](fs.md)`.appendBytes(path, content)`            | Append `bytes` to `path`; creates the file if missing.                                                                              |
| [`fs`](fs.md)`.appendString(path, content)`           | Append UTF-8 `string` to `path`; creates the file if missing.                                                                       |
| [`fs`](fs.md)`.chmod(path, mode)`                     | Set permission bits (e.g. `0o600`); Unix/Linux. Rejected outside `[0, 0o7777]`.                                                     |
| [`fs`](fs.md)`.chown(path, uid, gid)`                 | Set owner / group (`-1` leaves unchanged); Unix/Linux, usually needs privilege.                                                    |
| [`fs`](fs.md)`.close($f)`                             | Close an `fs.File` handle; removes it from the registry.                                                                            |
| [`fs`](fs.md)`.eof($f)`                               | True iff the next read on `$f` would error or return partial. Sticky.                                                               |
| [`fs`](fs.md)`.exists(path)`                          | True if `path` resolves; permission errors still surface.                                                                           |
| [`fs`](fs.md)`.hasEvent($w)`                          | Whether an `fs.Watcher` has a queued change event (non-blocking peek).                                                              |
| [`fs`](fs.md)`.isDir(path)`                           | True iff `path` exists and is a directory.                                                                                          |
| [`fs`](fs.md)`.isFile(path)`                          | True iff `path` exists and is a regular file.                                                                                       |
| [`fs`](fs.md)`.list(path)`                            | Sorted entry names in `path`. Non-recursive; returns `list of string`.                                                              |
| [`fs`](fs.md)`.mkdir(path)`                           | Create a single directory; errors if any parent is missing.                                                                         |
| [`fs`](fs.md)`.mkdirAll(path)`                        | Create `path` and every missing parent (like `mkdir -p`).                                                                           |
| [`fs`](fs.md)`.next($w)`                              | Block for the next `fs.Event` (`path`, `kind`, `isDir`) from an `fs.Watcher`; errors when the watcher is closed.                    |
| [`fs`](fs.md)`.open(path, mode)`                      | Open `path` and return an `fs.File`. `mode`: `"read"`, `"write"`, `"append"`.                                                       |
| [`fs`](fs.md)`.readBytes(path)` / `.readBytes($f, n)` | Whole-file read (1 arg) or up to `n` bytes from handle (2 args). Partial + sticky-EOF on short handle reads.                        |
| [`fs`](fs.md)`.readChars($f, n)`                      | Up to `n` runes from handle, UTF-8 decoded. Partial + sticky-EOF on short reads.                                                    |
| [`fs`](fs.md)`.readLine($f)`                          | One line from handle, `\r\n` / `\n` stripped. Errors on EOF - check `fs.eof` first.                                                 |
| [`fs`](fs.md)`.readString(path)`                      | Whole file as UTF-8; invalid UTF-8 is a positioned runtime error.                                                                   |
| [`fs`](fs.md)`.realpath(path)`                        | Absolute, symlink-resolved canonical path (resolve `os.ARGS[0]` to find files beside the real script). Missing / unresolvable errors. |
| [`fs`](fs.md)`.remove(path)`                          | Delete one file or empty directory. Non-empty dir errors.                                                                           |
| [`fs`](fs.md)`.removeAll(path)`                       | Recursive delete. Explicit second verb (no-footguns stance).                                                                        |
| [`fs`](fs.md)`.rename(old, new)`                      | Same-filesystem rename; cross-fs is a boundary error.                                                                               |
| [`fs`](fs.md)`.stat(path)`                            | Returns `fs.Stat` (`path`, `size`, `isDir`, `mtimeNanos`, `mode`). Missing path errors.                                             |
| [`fs`](fs.md)`.sync($f)`                              | Flush a write/append handle's data to the storage device (fsync); handle stays open. The "safe to remove the stick" step.          |
| [`fs`](fs.md)`.walk(path)`                            | Depth-first, sorted, includes `path`. Returns `list of fs.Stat`. Skips symlinks.                                                    |
| [`fs`](fs.md)`.watch(path[, intervalMs])`             | Watch a file / dir (recursive) for changes -> `fs.Watcher`; pull with `fs.next` / `fs.hasEvent`, stop with `fs.close`. mtime polling. |
| [`fs`](fs.md)`.writeBytes(path, content)` / `.writeBytes($f, b)` | Whole-file overwrite (path form) or write via handle (fs.File form).                                                      |
| [`fs`](fs.md)`.writeString(path, content)` / `.writeString($f, s)` | Whole-file overwrite (path form) or write via handle (fs.File form).                                                    |
| [`fs`](fs.md)`.makeTempFile([dir[, prefix[, suffix]]])` | Create a unique empty file (atomic, `0600`); returns its path. `dir=""` = system temp; parent must exist. |
| [`fs`](fs.md)`.makeTempDir([dir[, prefix]])`          | Create a unique directory (atomic, `0700`); returns its path. Only the leaf is created (not `mkdir -p`).                            |
| [`gpio`](gpio.md)`.setup(pin, direction)`            | Request `pin` (0..63) with `gpio.IN` / `gpio.OUT` on the current chip. Linux only. |
| [`gpio`](gpio.md)`.read(pin)` / `.write(pin, value)` | Read a line (0/1) / drive an output line (0 or 1). |
| [`gpio`](gpio.md)`.release(pin)` / `.chip(path)`     | Free a requested line / select the gpiochip device (default `/dev/gpiochip0`). |
| [`hash`](hash.md)`.compute(b, algo)`                  | One-shot digest. `algo` is `"md5"`, `"sha1"`, `"sha256"`, `"sha384"`, or `"sha512"`. Returns raw bytes.                             |
| [`hash`](hash.md)`.hmac(key, message, algo)`          | Keyed-hash MAC (RFC 2104) over the same algorithms; raw bytes out. For JWT / TOTP / SigV4 / webhook signatures.                     |
| [`hash`](hash.md)`.equal(a, b)`                       | Constant-time equality of two `bytes` (for MAC / token / digest checks); `false` on a length or content difference.                |
| [`hash`](hash.md)`.discard($s)`                       | Drop a `hash.Stream` without computing its digest; releases its state.                                                             |
| [`hash`](hash.md)`.finalize($s)`                      | Final digest as bytes; consumes the handle (later calls error).                                                                     |
| [`hash`](hash.md)`.stream(algo)`                      | Allocate a `hash.Stream` for `algo`; feed chunks via `hash.update` then close with `hash.finalize`.                                 |
| [`hash`](hash.md)`.update($s, $bytes)`                | Feed one chunk into a `hash.Stream` (mutates by side effect).                                                                       |
| [`httpd`](httpd.md)`.listen(addr)` / `.listenTLS(addr, cert, key)` | Start an HTTP / HTTPS server -> `httpd.Server` (`":0"` = ephemeral port). Default binary only. |
| [`httpd`](httpd.md)`.address($srv)` / `.shutdown($srv)`  | Bound address of a server / graceful drain (unblocks parked `accept`).                                                            |
| [`httpd`](httpd.md)`.accept($srv)`                    | Block for the next request -> `httpd.Request` (the pull loop). Errors once the server is shut down.                                |
| [`httpd`](httpd.md)`.method($req)` / `.path($req)` / `.query($req, name)` / `.header($req, name)` / `.body($req)` / `.remoteAddr($req)` | Read the accepted request (`query` / `header` -> `""` if absent; `body` -> `bytes`). |
| [`httpd`](httpd.md)`.setHeader($req, name, value)` / `.respond($req, status, body)` | Set a response header / send the response once (`body` is string or bytes). |
| [`httpd`](httpd.md)`.serveFile($req, path)` / `.serveDir($req, root)` | Answer with a file / the file under `root` for the request path (`..` cannot escape `root`).                    |
| [`i2c`](i2c.md)`.open(path, addr)`                   | Open an I2C bus and select 7-bit slave `addr` -> `i2c.Bus`. Linux only. |
| [`i2c`](i2c.md)`.read($bus, n)` / `.write($bus, data)` | Read `n` raw bytes / write raw bytes to the selected slave. |
| [`i2c`](i2c.md)`.readReg($bus, reg, n)` / `.writeReg($bus, reg, data)` | Register read (set pointer, read back) / register write. |
| [`i2c`](i2c.md)`.close($bus)`                        | Close the bus. |
| [`intl`](intl.md)`.load(lang, catalog)`               | Merge a `map of string to string` into the catalog for `lang`; the first language loaded is the default.                            |
| [`intl`](intl.md)`.setLocale(lang)` / `.locale()`     | Set / read the current locale (e.g. `"de-AT"`; `locale()` is `""` until set).                                                       |
| [`intl`](intl.md)`.tr(key[, params])`                 | Translate `key` (fallback: locale -> base language -> default -> the key); `params` (a `map`) fills `%name%` placeholders (`%%` escapes a literal `%`). |
| [`io`](io.md)`.eof()`                                 | True if and only if the next `io.readLine()` would error. Pair with `while (not io.eof()) {...}`.                                   |
| [`io`](io.md)`.printf(format, args...)`               | Format-string write to stdout. Verbs: `%d %f %s %t %v %%`; per-verb `\|key=value` modifiers (`pad`, `prec`, `base`, `null=*`, ...). |
| [`io`](io.md)`.printf(value)`                         | Write a value's display form to stdout.                                                                                             |
| [`io`](io.md)`.eprintf(format, args...)`              | Like `printf`, but writes to **stderr** (diagnostics / logs that must not mix into stdout).                                        |
| [`io`](io.md)`.readLine()`                            | Read one line from stdin (trailing newline stripped). Errors at EOF - check `io.eof()` first.                                       |
| [`io`](io.md)`.readLine(prompt)`                      | Same as `io.readLine()` but writes `prompt` to stdout first.                                                                        |
| [`io`](io.md)`.readLines()`                           | Slurp all remaining stdin lines into a `list of string` (newlines stripped, LF/CRLF, OS-independent). Use the `readLine` loop for large streams. |
| [`io`](io.md)`.sprintf(format, args...)`              | Format-string version of `sprintf`. Same verbs and `\|key=value` modifiers as `printf`.                                             |
| [`io`](io.md)`.sprintf(value)`                        | Display-form of a value, returned as a string (doesn't write).                                                                      |
| `len(v)` *(language built-in)*                        | Structural length: rune count (string), element count (list), entry count (map), byte count (bytes).                                |
| [`json`](json.md)`.decode(s)`                         | Parse JSON text into an opaque `json.Value` handle (walk it with the accessors below).                                             |
| [`json`](json.md)`.encode(v)`                         | Compact JSON string for an encodable value (struct/map -> object, `bytes` -> base64, `json.Value` round-trips; task / non-string keys error). |
| [`json`](json.md)`.encodePretty(v)`                   | Like `encode`, 2-space indented.                                                                                                    |
| [`json`](json.md)`.typeOf(v[, ptr])`                  | JSON type at an optional JSON Pointer: `null` `bool` `int` `float` `string` `list` `map`.                                          |
| [`json`](json.md)`.get(v[, ptr])`                     | Sub-node at a JSON Pointer, as a `json.Value` (walk stays opaque; no pointer = the node itself).                                    |
| [`json`](json.md)`.has(v, ptr)`                       | Whether the JSON Pointer resolves to an existing node.                                                                              |
| [`json`](json.md)`.keys(v[, ptr])`                    | `list of string` keys of the addressed map, in document order.                                                                      |
| [`json`](json.md)`.length(v[, ptr])`                  | Element count of a list / entry count of a map at the pointer.                                                                      |
| [`json`](json.md)`.asInt(v[, ptr])` / `asFloat` / `asString` / `asBool` | Extract the addressed leaf as a typed value (strict; `asFloat` promotes an integral number).                    |
| [`json`](json.md)`.isNull(v[, ptr])`                  | Whether the addressed node is JSON `null`.                                                                                          |
| [`json`](json.md)`.map()` / `.list()`                 | A fresh empty JSON map / list `json.Value` - the explicit start of a document (writes never auto-vivify).                          |
| [`json`](json.md)`.set(v, ptr, val)`                  | Non-mutating: upsert a map key or replace an in-range list index; returns a new `json.Value`. Strict (no missing intermediates).   |
| [`json`](json.md)`.insert(v, ptr, val)`               | Insert into a list before index `ptr` (or `-` = at end); returns a new handle.                                                     |
| [`json`](json.md)`.append(v, ptr, val)`               | Push onto the list addressed by `ptr` (sugar for insert at `/.../-`).                                                              |
| [`json`](json.md)`.remove(v, ptr)`                    | Drop the map key or list element at `ptr`; returns a new handle.                                                                    |
| [`json`](json.md)`.move(v, from, to)`                 | Relocate the subtree at `from` to `to` (read, remove, then `set`).                                                                  |
| [`kv`](kv.md)`.open()`                                | A fresh in-memory `kv.Store` (reset each run).                                                                                      |
| [`kv`](kv.md)`.openFile(path)`                        | A `kv.Store` persisted to `path` (survives across runs; flush per mutation).                                                        |
| [`kv`](kv.md)`.set(store, key, value, ttl)`           | Store `value`, expiring in `ttl` seconds (`0` = never).                                                                            |
| [`kv`](kv.md)`.add(store, key, value, ttl)`           | Store only if absent; whether it stored (`bool`).                                                                                   |
| [`kv`](kv.md)`.get(store, key)`                       | The value, or `""` when absent / expired.                                                                                          |
| [`kv`](kv.md)`.has(store, key)`                       | Whether the key is present and unexpired (`bool`).                                                                                  |
| [`kv`](kv.md)`.delete(store, key)`                    | Remove the key; whether it existed (`bool`).                                                                                        |
| [`kv`](kv.md)`.touch(store, key, ttl)`                | Re-arm the expiry; whether it existed (`bool`).                                                                                     |
| [`kv`](kv.md)`.incr(store, key, delta)`               | Add signed `delta` to the numeric value; new value, or `-1` when absent (not created).                                             |
| [`kv`](kv.md)`.close(store)`                          | Drop the store and free its handle.                                                                                                 |
| [`linalg`](linalg.md)`.dot(a, b)` / `.distance(a, b)` | Dot product / Euclidean distance of vectors (`list of float`) -> `float`. Length-checked.                                          |
| [`linalg`](linalg.md)`.cross(a, b)` / `.normalize(v)` | 3-D cross product of length-3 vectors / the unit vector `v / norm(v)` (zero vector errors) -> `list of float`.                      |
| [`linalg`](linalg.md)`.norm(x)`                       | L2 norm of a vector, or the Frobenius norm of a matrix -> `float` (polymorphic).                                                    |
| [`linalg`](linalg.md)`.scale(x, s)` / `.add(a, b)` / `.sub(a, b)` | Scalar multiply / element-wise sum / difference of a vector **or** a matrix (same shape).                              |
| [`linalg`](linalg.md)`.matmul(a, b)`                  | General product: matrix*matrix -> matrix; matrix*vector or vector*matrix -> vector; vector*vector errors (use `dot`).               |
| [`linalg`](linalg.md)`.transpose(m)` / `.trace(m)`    | Transpose of a `list of list of float` / trace (diagonal sum, square) -> `float`.                                                   |
| [`linalg`](linalg.md)`.determinant(m)`                | Determinant of a square matrix -> `float` (singular is `0`).                                                                        |
| [`linalg`](linalg.md)`.inverse(m)` / `.solve(a, b)`   | Matrix inverse / solve `a x = b`; a singular matrix is a catchable error.                                                            |
| [`linalg`](linalg.md)`.identity(n)` / `.zeros(r, c)` / `.shape(m)` | The `n x n` identity / an `r x c` zero matrix / `[rows, cols]` of a matrix (`list of int`).                            |
| [`lists`](lists.md)`.all(xs, fn)`                     | True if the `func` `fn(x)` is true for every element (true for empty). Short-circuits.                                              |
| [`lists`](lists.md)`.any(xs, fn)`                     | True if the `func` `fn(x)` is true for any element. Short-circuits.                                                                 |
| [`lists`](lists.md)`.concat(a, b)`                    | New list with `a`'s elements followed by `b`'s.                                                                                     |
| [`lists`](lists.md)`.contains(xs, item)`              | True if `item` appears in `xs` (haystack, needle).                                                                                  |
| [`lists`](lists.md)`.filter(xs, fn)`                  | New list of the elements where the `func` `fn(x)` returns true.                                                                     |
| [`lists`](lists.md)`.find(xs, fn)`                    | First element where the `func` `fn(x)` is true; errors (catchable) if none.                                                          |
| [`lists`](lists.md)`.first(xs)`                       | Element at index 0. Empty input errors.                                                                                             |
| [`lists`](lists.md)`.head(xs, n)`                     | New list of the first `n` elements.                                                                                                 |
| [`lists`](lists.md)`.last(xs)`                        | Element at the last index. Empty input errors.                                                                                      |
| [`lists`](lists.md)`.map(xs, fn)`                     | New list of `fn(x)` for each element (`fn` a `func`); result type checked at the binding.                                            |
| [`lists`](lists.md)`.pop(xs)`                         | New list without the last element. Empty input errors.                                                                              |
| [`lists`](lists.md)`.push(xs, item)`                  | New list with `item` appended.                                                                                                      |
| [`lists`](lists.md)`.range(start, end[, step])`       | Half-open list of consecutive ints; `end` excluded; `step` must match direction.                                                    |
| [`lists`](lists.md)`.reduce(xs, fn, init)`            | Fold left with the `func` `fn(acc, x)`, starting from `init`.                                                                        |
| [`lists`](lists.md)`.reverse(xs)`                     | New list with elements reversed.                                                                                                    |
| [`lists`](lists.md)`.shuffle(xs)`                     | Fisher-Yates; respects `math.randSeed`. Non-mutating.                                                                               |
| [`lists`](lists.md)`.slice(xs, start[, end])`         | New sublist `[start, end)`; `end` defaults to `len(xs)`.                                                                            |
| [`lists`](lists.md)`.sort(xs)`                        | New ascending-sorted list. Numeric / string / bool elements; mixed errors.                                                          |
| [`lists`](lists.md)`.sortBy(xs, keyFn)`               | New list sorted ascending by the key the `func` `keyFn(x)` returns (a comparable scalar).                                            |
| [`lists`](lists.md)`.tail(xs, n)`                     | New list of the last `n` elements.                                                                                                  |
| [`maps`](maps.md)`.delete(m, key)`                    | New map without `key`. Missing key errors (strict at boundaries).                                                                   |
| [`maps`](maps.md)`.has(m, key)`                       | True if map `m` contains `key`. The non-erroring companion to `$m[key]`.                                                            |
| [`maps`](maps.md)`.keys(m)`                           | List of keys in insertion order.                                                                                                    |
| [`maps`](maps.md)`.merge(a, b)`                       | New map; `b`'s entries layered on top of `a`.                                                                                       |
| [`maps`](maps.md)`.values(m)`                         | List of values in insertion order.                                                                                                  |
| [`ml`](ml.md)`.linearRegression(X, y)` / `.ridge(X, y, alpha)` | Fit OLS / L2-ridge regression -> ml.Model (regressor). |
| [`ml`](ml.md)`.kNN(X, y, k)` / `.naiveBayes(X, y)` | Fit a k-NN / Gaussian naive-Bayes classifier -> ml.Model. |
| [`ml`](ml.md)`.logisticRegression(X, y [, lr [, epochs]])` | Fit a binary (0/1) logistic classifier (gradient descent) -> ml.Model. |
| [`ml`](ml.md)`.decisionTree(X, y [, maxDepth])` / `.randomForest(X, y [, nTrees [, maxDepth]])` | Fit a CART tree / bagged forest classifier -> ml.Model. |
| [`ml`](ml.md)`.kMeans(X, k [, maxIter])` | Fit k-means clustering (k-means++ seed) -> ml.Model. Uses `math.randSeed`. |
| [`ml`](ml.md)`.pca(X, nComponents)` | Fit PCA (covariance eigendecomposition) -> ml.Model (transform). |
| [`ml`](ml.md)`.standardScaler(X)` / `.minMaxScaler(X)` | Fit a z-score / [0,1] feature scaler -> ml.Model (transform). |
| [`ml`](ml.md)`.predict(model, X)` | Predicted labels (int, classifier/cluster) or values (float, regressor) per row. |
| [`ml`](ml.md)`.transform(model, X)` / `.predictProba(model, X)` | Transform features (scaler/PCA) / positive-class probability (logistic). |
| [`ml`](ml.md)`.free(model)` | Drop a fitted model from the registry to free memory. |
| [`ml`](ml.md)`.trainTestSplit(X, y, testFraction)` | Shuffle + split -> ml.Split{trainX,trainY,testX,testY}. Uses `math.randSeed`. |
| [`ml`](ml.md)`.kFold(nSamples, k)` | `k` contiguous CV folds -> list of ml.Fold{trainIdx, testIdx}. |
| [`ml`](ml.md)`.accuracy(yTrue, yPred)` | Fraction of exact matches. |
| [`ml`](ml.md)`.precision/recall/f1(yTrue, yPred [, positive])` | Binary classification metrics for the positive label (default 1). |
| [`ml`](ml.md)`.confusionMatrix(yTrue, yPred)` | ml.Confusion{labels, matrix} (rows=true, cols=predicted). |
| [`ml`](ml.md)`.rocAuc(yTrue, scores)` | Binary ROC-AUC from 0/1 labels and predicted scores (tie-aware). |
| [`ml`](ml.md)`.rmse/mae/r2(yTrue, yPred)` | Regression error metrics (r2 = coefficient of determination). |
| [`ml`](ml.md)`.lasso(X, y, alpha)` | Fit L1-regularized regression (coordinate descent; sparse coefficients) -> ml.Model. |
| [`ml`](ml.md)`.kNNRegressor(X, y, k)` / `.decisionTreeRegressor(X, y [, maxDepth])` | Regression variants of k-NN / CART (predict a continuous value). |
| [`ml`](ml.md)`.randomForestRegressor(X, y [, nTrees [, maxDepth]])` | Bagged regression trees -> ml.Model (regressor). |
| [`ml`](ml.md)`.coefficients(model)` / `.intercept(model)` | Read a linear/logistic model's learned weights / bias. |
| [`ml`](ml.md)`.centroids(model)` / `.components(model)` / `.explainedVariance(model)` | k-means centres / PCA axes / PCA per-component variance ratios. |
| [`ml`](ml.md)`.featureImportances(model)` | Gini feature importances of a tree / forest (sum to 1). |
| [`ml`](ml.md)`.polynomialFeatures(X, degree)` | Expand X to all monomials up to `degree` (+ bias); stateless. Degree in [1,8]. |
| [`ml`](ml.md)`.mse(yTrue, yPred)` / `.logLoss(yTrue, probas)` | Mean-squared error / binary cross-entropy. |
| [`math`](math.md)`.abs(x)`                            | Absolute value of `x` (int→int, float→float).                                                                                       |
| [`net`](net.md)`.accept($listener)`                   | Block until a client connects to `$listener`; return the new `net.Conn`.                                                            |
| [`net`](net.md)`.address($h)`                         | Polymorphic. Conn -> peer address; Listener / UDPSocket -> local bound address.                                                     |
| [`net`](net.md)`.close($h)`                           | Polymorphic. Closes a `net.Conn`, `net.Listener`, or `net.UDPSocket`.                                                               |
| [`net`](net.md)`.connect(address[, timeoutMs])`       | TCP client: dial `"host:port"` and return a `net.Conn`. Optional `timeoutMs` bounds connection establishment.                       |
| [`net`](net.md)`.connectTLS(address[, net.TLSOptions][, timeoutMs])` | TLS client: dial `"host:port"` + handshake, verifying the cert against the host. `net.TLSOptions` for caCert / skipVerify; optional `timeoutMs` bounds the dial + handshake. |
| [`net`](net.md)`.startTLS($conn[, net.TLSOptions][, timeoutMs])` | Upgrade an open plaintext `net.Conn` to TLS in place (STARTTLS); host reused from connect; same handle. Optional `timeoutMs` bounds the handshake. |
| [`net`](net.md)`.eof($conn)`                          | True iff the next read on `$conn` would return partial or fail. Sticky.                                                             |
| [`net`](net.md)`.listen(address)`                     | Bind TCP `"host:port"` (use `":0"` for ephemeral). Returns a `net.Listener`.                                                        |
| [`net`](net.md)`.listenUDP(address)`                  | Bind a UDP socket. Returns a `net.UDPSocket`; usable as both client and server.                                                     |
| [`net`](net.md)`.lookup(host)`                        | DNS: resolve `host` to a `list of string` IPs.                                                                                      |
| [`net`](net.md)`.readAll($conn [, maxBytes [, idleTimeoutMs]])` | Read to EOF, returning the whole stream as one `bytes` in a single Go loop (whole-body / object download). `maxBytes>0` caps (catchable), `idleTimeoutMs>0` re-arms a per-chunk read deadline. |
| [`net`](net.md)`.readBytes($conn, n)`                 | Read up to `n` bytes; blocks for at least one byte. Sticky-EOF on close.                                                            |
| [`net`](net.md)`.readN($conn, n [, idleTimeoutMs])`   | Read **exactly** `n` bytes for a length-prefixed frame; a close before `n` bytes is a catchable error, not a truncated return.      |
| [`net`](net.md)`.recvFrom($sock, n)`                  | Block for one UDP datagram, up to `n` bytes. Returns `net.Datagram{data, peer}`.                                                    |
| [`net`](net.md)`.reverseLookup(ip)`                   | Reverse DNS: IP address to a `list of string` of hostnames.                                                                         |
| [`net`](net.md)`.sendTo($sock, peer, bytes)`          | Send one UDP datagram to `peer` (`"host:port"`).                                                                                    |
| [`net`](net.md)`.setDeadline($conn, ms)`              | Arm a read/write deadline `ms` ms out (`0` clears). A read past it fails with a catchable `read timed out`.                          |
| [`net`](net.md)`.writeBytes($conn, bytes)`            | Blocking write of every byte to a `net.Conn`.                                                                                       |
| [`regex`](regex.md)`.escape(s)`                       | Escape RE2 metacharacters so `s` matches literally when used as a pattern.                                                          |
| [`regex`](regex.md)`.find(pattern, s)`                | First match as `regex.Match`; sentinel with `start=-1` if no match.                                                                 |
| [`regex`](regex.md)`.findAll(pattern, s)`             | Every non-overlapping match; returns `list of regex.Match`.                                                                         |
| [`regex`](regex.md)`.matches(pattern, s)`             | True iff `pattern` matches somewhere in `s`.                                                                                        |
| [`regex`](regex.md)`.replace(pattern, s, replacement)` | Replace every match. `$1`, `${name}` expand to captured groups; `$$` is a literal `$`.                                             |
| [`regex`](regex.md)`.split(pattern, s)`               | Split `s` at every match; returns `list of string`.                                                                                 |
| [`serial`](serial.md)`.open(path, baud)` / `.openWith(path, opts)` | Open a serial port (raw 8N1, or full `serial.Options`) -> `serial.Port`. Linux only. |
| [`serial`](serial.md)`.read($port, n)` / `.write($port, data)` | Read up to `n` bytes (blocks for >=1) / write bytes (-> count). |
| [`serial`](serial.md)`.flush($port)` / `.close($port)` | Discard buffered I/O / close the port. |
| [`spi`](spi.md)`.open(path)` / `.configure($dev, mode, speedHz)` | Open an SPI device -> `spi.Device` / set clock mode (0..3) and speed. Linux only. |
| [`spi`](spi.md)`.transfer($dev, data)` / `.close($dev)` | Full-duplex exchange (out and in together) / close the device. |
| [`sql`](sql.md)`.open(driver, dsn)` / `.close($c)`   | Open a MySQL / Postgres connection -> sql.Connection / close it. Default binary only. |
| [`sql`](sql.md)`.query(target, sql, params...)` / `.exec(target, sql, params...)` | Query -> sql.Rows / statement -> sql.Result{affected, lastId}. target = Connection or Tx; params bind through placeholders. |
| [`sql`](sql.md)`.next($rows)` / `.columns($rows)` / `.closeRows($rows)` | Advance the cursor (false at end) / column names / close early. |
| [`sql`](sql.md)`.asInt` / `.asFloat` / `.asString` / `.asBool` / `.asBytes($rows, col)` / `.isNull($rows, col)` | Read the current row's column (name or index), typed; a NULL column is an error (check isNull). |
| [`sql`](sql.md)`.begin($c)` / `.commit($tx)` / `.rollback($tx)` | Transaction: begin -> sql.Tx (a query/exec target), then commit / rollback. |
| [`sql`](sql.md)`.setQueryTimeout(ms)` | Set the client query/read deadline (ms); `0` disables it for a long cursor read. Default 30 000, process-wide. |
| [`sql`](sql.md)`.prepare($c, sql)` / `.queryStmt($s, ...)` / `.execStmt($s, ...)` / `.closeStmt($s)` | Prepared-statement lifecycle. |
| [`math`](math.md)`.acos(x)` | Arccosine (radians); errors outside [-1, 1]. |
| [`math`](math.md)`.acosh(x)` | Inverse hyperbolic cosine; errors for x < 1. |
| [`math`](math.md)`.asin(x)` | Arcsine (radians); errors outside [-1, 1]. |
| [`math`](math.md)`.asinh(x)` | Inverse hyperbolic sine. |
| [`math`](math.md)`.atan(x)` | Arctangent (radians). |
| [`math`](math.md)`.atan2(y, x)` | Two-argument arctangent, quadrant-aware (radians). |
| [`math`](math.md)`.atanh(x)` | Inverse hyperbolic tangent; errors outside (-1, 1). |
| [`math`](math.md)`.beta(a, b)` | Beta function B(a,b)=gamma(a)gamma(b)/gamma(a+b); a, b > 0. |
| [`math`](math.md)`.cbrt(x)` | Real cube root (handles negatives). |
| [`math`](math.md)`.ceil(x)` | Smallest int >= `x`. Accepts int (identity) or float. |
| [`math`](math.md)`.comb(n, k)` | Binomial coefficient nCr (exact int); k>n is 0; errors on overflow. |
| [`math`](math.md)`.cos(x)` | Cosine (radians). |
| [`math`](math.md)`.cosh(x)` | Hyperbolic cosine; errors on overflow. |
| [`math`](math.md)`.erf(x)` | Error function. |
| [`math`](math.md)`.erfc(x)` | Complementary error function (1 - erf). |
| [`math`](math.md)`.exp(x)` | e^x; errors on overflow. |
| [`math`](math.md)`.expm1(x)` | e^x - 1, accurate for small x. |
| [`math`](math.md)`.factorial(n)` | n! (exact int); errors on negative n or overflow (n > 20). |
| [`math`](math.md)`.floor(x)` | Largest int <= `x`. Accepts int (identity) or float. |
| [`math`](math.md)`.gamma(x)` | Gamma function; errors at the poles (0, negative ints) and on overflow. |
| [`math`](math.md)`.gcd(a, b)` | Greatest common divisor (non-negative; gcd(0,0)=0). |
| [`math`](math.md)`.hypot(x, y)` | sqrt(x^2 + y^2) without intermediate overflow. |
| [`math`](math.md)`.lbeta(a, b)` | Natural log of the beta function, ln B(a,b); a, b > 0. |
| [`math`](math.md)`.lcm(a, b)` | Least common multiple (lcm(x,0)=0); errors on overflow. |
| [`math`](math.md)`.lgamma(x)` | ln|gamma(x)|; errors at the poles. |
| [`math`](math.md)`.ln(x)` | Natural logarithm; errors for x <= 0. |
| [`math`](math.md)`.log(x, base)` | Logarithm of x to an arbitrary base; errors for non-positive x/base or base 1. |
| [`math`](math.md)`.log10(x)` | Base-10 logarithm; errors for x <= 0. |
| [`math`](math.md)`.log1p(x)` | ln(1 + x), accurate for small x; errors for x <= -1. |
| [`math`](math.md)`.log2(x)` | Base-2 logarithm; errors for x <= 0. |
| [`math`](math.md)`.max(a, b)` | Larger of two numbers; mixed int/float promotes to float. |
| [`math`](math.md)`.min(a, b)` | Smaller of two numbers; mixed int/float promotes to float. |
| [`math`](math.md)`.perm(n, k)` | k-permutations of n (nPr, exact int); k>n is 0; errors on overflow. |
| [`math`](math.md)`.pow(x, y)` | `x` raised to `y`; always float. Errors on NaN/Inf-producing inputs. |
| [`math`](math.md)`.rand()` | Float in `[0, 1)` from the shared seedable (non-crypto) source. |
| [`math`](math.md)`.randInt(lo, hi)` | Int in `[lo, hi]` inclusive; errors if `lo > hi`. |
| [`math`](math.md)`.randSeed(n)` | Reseed the shared source for reproducible runs (also drives `lists.shuffle`; `uuid` / `password` use `crypto` instead). |
| [`math`](math.md)`.regBetaI(x, a, b)` | Regularized incomplete beta I_x(a,b) in [0,1] (CDF engine for beta / t / F / binomial); 0<=x<=1, a,b>0. |
| [`math`](math.md)`.regGammaP(a, x)` | Regularized lower incomplete gamma P(a,x) in [0,1] (gamma / chi-square CDF); a>0, x>=0. |
| [`math`](math.md)`.regGammaQ(a, x)` | Regularized upper incomplete gamma Q(a,x) = 1 - P(a,x). |
| [`math`](math.md)`.round(x)` | Round to nearest int (half away from zero). |
| [`math`](math.md)`.sign(x)` | -1 / 0 / 1 with the sign of x, preserving its type (int -> int, float -> float). |
| [`math`](math.md)`.sin(x)` | Sine (radians). |
| [`math`](math.md)`.sinh(x)` | Hyperbolic sine; errors on overflow. |
| [`math`](math.md)`.sqrt(x)` | Square root; always float. Errors on negative input. |
| [`math`](math.md)`.tan(x)` | Tangent (radians). |
| [`math`](math.md)`.tanh(x)` | Hyperbolic tangent. |
| [`math`](math.md)`.trunc(x)` | Round toward zero -> int; int argument is the identity. |
| [`os`](os.md)`.flag(name)`                            | Value following `name` in `os.ARGS`, or `""` if absent / at end. Exact-match (no `--foo=bar` parsing).                              |
| [`os`](os.md)`.getEnv(name)`                          | Read environment variable `name`. Unset → empty string, no error.                                                                   |
| [`os`](os.md)`.setEnv(name, value)`                   | Set environment variable `name` for this process (and children it spawns). Invalid name errors.                                     |
| [`os`](os.md)`.hasFlag(name)`                         | True if `name` appears as an exact element of `os.ARGS`.                                                                            |
| [`os`](os.md)`.isTerminal(stream)`                    | Is `stream` (`"stdout"`/`"stderr"`/`"stdin"`) an interactive terminal? Pipe/file -> false.                                         |
| [`os`](os.md)`.cwd()`                                 | Absolute path of the current working directory.                                                                                     |
| [`os`](os.md)`.homeDir()`                             | Current user's home directory (`$HOME` / `%USERPROFILE%`).                                                                          |
| [`os`](os.md)`.tempDir()`                             | Temp-file directory (`$TMPDIR`/`/tmp`; `%TMP%` on Windows). Never errors.                                                          |
| [`os`](os.md)`.catchSignal(name)` / `.gotSignal(name)` | Opt into trapping a Unix signal (`"int"`/`"term"`/`"hup"`/`"usr2"`) / poll-and-clear whether it arrived. Cooperative; `"usr1"` reserved for `kill -USR1` diagnostics. |
| [`os`](os.md)`.kill(p)`                               | Send SIGTERM to spawned process `$p`.                                                                                               |
| [`os`](os.md)`.poll(p)`                               | True if spawned process `$p` has exited (a following `os.wait` returns immediately).                                                |
| [`os`](os.md)`.release(p)`                            | Drop a finished process handle from the registry (frees captured output); errors if `$p` still runs.                                |
| [`os`](os.md)`.run(argv)`                             | Blocking: run `argv` to completion, return `os.Result{exitCode, stdout, stderr}`.                                                   |
| [`os`](os.md)`.spawn(argv)`                           | Non-blocking: start `argv`, return `os.Process{pid}` handle.                                                                        |
| [`os`](os.md)`.wait(p)`                               | Block until spawned process `$p` exits; return `os.Result`. Idempotent.                                                             |
| [`path`](path.md)`.base(p)`                           | Last element of `p`. `""` -> `"."`, `"/"` -> `"/"`. OS-aware; not a filename sanitizer.                                             |
| [`path`](path.md)`.dir(p)`                            | All but the last element of `p`, cleaned. `"c.txt"` -> `"."`.                                                                       |
| [`path`](path.md)`.ext(p)`                            | File extension incl. the leading dot (`".txt"`), or `""`.                                                                           |
| [`path`](path.md)`.stem(p)`                           | Base name of `p` without its extension.                                                                                             |
| [`path`](path.md)`.join(a, b, ...)`                   | Join >= 1 elements with the separator and clean; empty elements dropped. Portable alternative to hardcoding `"/"`.                  |
| [`path`](path.md)`.clean(p)`                          | Shortest path equivalent to `p` (collapses `.`, `..`, repeated separators). `""` -> `"."`.                                          |
| [`path`](path.md)`.isAbs(p)`                          | Whether `p` is absolute (`bool`).                                                                                                   |
| [`path`](path.md)`.split(p)`                          | `[dir, file]` where `dir` keeps its trailing separator, so `dir + file == p`.                                                       |
| [`stats`](stats.md)`.mean(xs)` / `.median(xs)`        | Arithmetic mean / median of a `list of int`\|`float` -> `float`. Empty errors.                                                       |
| [`stats`](stats.md)`.variance(xs)` / `.stddev(xs)`    | Population variance / standard deviation -> `float`.                                                                                |
| [`stats`](stats.md)`.sampleVariance(xs)` / `.sampleStddev(xs)` | Sample (Bessel `n-1`) variance / stddev -> `float`. Needs >= 2 elements.                                                   |
| [`stats`](stats.md)`.mad(xs)`                         | Median absolute deviation - robust spread -> `float`.                                                                              |
| [`stats`](stats.md)`.skewness(xs)` / `.kurtosis(xs)`  | Shape: population skewness / excess kurtosis (normal -> 0) -> `float`. Non-constant, >= 2.                                          |
| [`stats`](stats.md)`.percentile(xs, p)`               | The `p`-th percentile (`p` in `[0,100]`, linear interpolation) -> `float`.                                                          |
| [`stats`](stats.md)`.quartiles(xs)` / `.iqr(xs)`      | `[Q1,Q2,Q3]` (`list of float`) / interquartile range `p75-p25` (`float`).                                                          |
| [`stats`](stats.md)`.min(xs)` / `.max(xs)` / `.range(xs)` | Smallest / largest element / spread `max-min`; all preserve int/float kind. Empty errors.                                       |
| [`stats`](stats.md)`.sum(xs)`                         | Sum: `int` for an all-int list (overflow-checked), `float` otherwise; `sum([])` is `0`.                                            |
| [`stats`](stats.md)`.geometricMean(xs)` / `.harmonicMean(xs)` | Geometric / harmonic mean -> `float`. Every element must be `> 0`.                                                          |
| [`stats`](stats.md)`.weightedMean(xs, ws)`            | Weighted mean `sum(x*w)/sum(w)` -> `float`. Equal length; weights not summing to 0.                                                |
| [`stats`](stats.md)`.zscore(xs)`                      | Each element standardized `(x-mean)/stddev` -> `list of float`. Constant list errors.                                              |
| [`stats`](stats.md)`.mode(xs)` (elem) / `.modes(xs)` (list) | Most frequent element / every tied modal value; both preserve kind.                                                          |
| [`stats`](stats.md)`.correlation(xs, ys)` / `.covariance(xs, ys)` | Pearson correlation (`[-1,1]`) / population covariance of two equal-length lists -> `float`.                            |
| [`stats`](stats.md)`.sampleCovariance(xs, ys)`        | Sample (`n-1`) covariance -> `float`. Needs >= 2 elements.                                                                         |
| [`stats`](stats.md)`.describe(xs)`                    | One-call `stats.Summary{count, min, q1, median, mean, q3, max, stddev}`.                                                           |
| [`stats`](stats.md)`.normalPdf/normalCdf/normalQuantile(p,mean,sd)` | Normal density / CDF / inverse-CDF; `sd>0`, quantile `p` in (0,1). |
| [`stats`](stats.md)`.normalSample(mean, sd)` | One Normal(mean, sd) draw (uses `math.randSeed`). |
| [`stats`](stats.md)`.tCdf(x, df)` / `.tQuantile(p, df)` / `.tPdf(x, df)` | Student's t CDF / inverse / density; `df>0`. |
| [`stats`](stats.md)`.chiSquareCdf(x, df)` / `.chiSquareQuantile(p, df)` | Chi-square CDF / inverse; `x>=0`, `df>0`. |
| [`stats`](stats.md)`.fCdf(x, df1, df2)` / `.fQuantile(p, df1, df2)` | F CDF / inverse; `x>=0`, dfs `>0`. |
| [`stats`](stats.md)`.binomialPmf(k, n, p)` / `.binomialCdf(k, n, p)` | Binomial mass / CDF; `k`,`n` ints, `0<=p<=1`. |
| [`stats`](stats.md)`.poissonPmf(k, lambda)` / `.poissonCdf(k, lambda)` | Poisson mass / CDF; `k` int, `lambda>0`. |
| [`stats`](stats.md)`.linearRegression(xs, ys)` | Simple OLS -> `stats.Regression{slope, intercept, r, r2, stdErr, pValue, n}`. |
| [`stats`](stats.md)`.multipleRegression(X, ys)` | Multiple OLS coefficients `[intercept, b1, ...]` (`list of float`). |
| [`stats`](stats.md)`.confidenceInterval(data, level)` | t-based CI for the mean -> `stats.Interval{lower, upper}`; `level` in (0,1). |
| [`stats`](stats.md)`.proportionCi(successes, n, level, method)` | Binomial-proportion CI; method `"wald"`/`"wilson"`/`"clopper-pearson"` -> `stats.Interval`. |
| [`stats`](stats.md)`.tTest(data, mu)` / `.tTest2(a, b)` | One-sample / two-sample (Welch) t-test -> `stats.Test{statistic, df1, df2, pValue}`. |
| [`stats`](stats.md)`.chiSquareTest(observed, expected)` | Chi-square goodness of fit -> `stats.Test`; expected counts `>0`. |
| [`stats`](stats.md)`.fTest(a, b)` / `.anova(groups)` | Two-sided F-test of variances / one-way ANOVA -> `stats.Test`. |
| [`stats`](stats.md)`.histogram(data, binEdges)` | Bin counts (Excel `FREQUENCY`); `k+1` ascending edges -> `k` counts (`list of int`). |
| [`strings`](strings.md)`.chars(s)`                    | Split `s` into a `list of string`, one entry per Unicode code point.                                                                |
| [`term`](term.md)`.makeRaw(stream)` / `.restore(state)` | Enter / leave raw mode (unbuffered, no-echo) on a terminal (`"stdin"`); `makeRaw` returns a single-use `term.State`. Non-terminal errors. |
| [`term`](term.md)`.size(stream)`                      | The terminal's `term.Size{rows, cols}` (query `"stdout"`).                                                                          |
| [`term`](term.md)`.readByte()`                        | Next raw byte from stdin (`0`-`255`), or `-1` at end of input. Bytes, not decoded keys. Refused in the REPL.                        |
| [`testing`](testing.md)`.assertContains(hay, needle)` | Throw `Error{kind:"assertion"}` unless hay contains needle: substring / list element / map key.                                     |
| [`testing`](testing.md)`.assertEqual(actual, expected)` | Throw unless deeply equal (lists / maps / structs compare by value).                                                              |
| [`testing`](testing.md)`.assertFalse(cond)`           | Throw unless `cond` (a bool) is false.                                                                                              |
| [`testing`](testing.md)`.assertNotEqual(actual, expected)` | Throw unless not deeply equal.                                                                                                 |
| [`testing`](testing.md)`.assertThrows(name, kind)`    | Throw unless the named zero-arg method throws an `Error` of that `kind`.                                                            |
| [`testing`](testing.md)`.assertTrue(cond)`            | Throw unless `cond` (a bool) is true.                                                                                              |
| [`testing`](testing.md)`.report(results, format)`     | Render results to `"text"`, `"tap"`, or `"junit"` (returns string).                                                                 |
| [`testing`](testing.md)`.reset()`                     | Clear the process-wide result accumulator.                                                                                          |
| [`testing`](testing.md)`.results()`                   | Snapshot of the accumulator as `list of testing.Result`.                                                                            |
| [`testing`](testing.md)`.run(name)`                   | Invoke a zero-arg user method by name; catch every failure mode into a `testing.Result`.                                            |
| [`testing`](testing.md)`.runWith(name, args)`         | Like `run`, binding the `args` list to the method's parameters (arity + type checked).                                             |
| [`strings`](strings.md)`.contains(s, sub)`            | True if `s` contains the substring `sub`.                                                                                           |
| [`strings`](strings.md)`.endsWith(s, suffix)`         | True if `s` ends with `suffix`.                                                                                                     |
| [`strings`](strings.md)`.fold(s)`                     | Remove common Latin diacritics for a sort / search key (`Österreich` -> `Osterreich`); case preserved. Not full Unicode collation. |
| [`strings`](strings.md)`.indexOf(s, sub)`             | Rune index of first `sub` in `s`, or `-1` if absent.                                                                                |
| [`strings`](strings.md)`.join(parts, sep)`            | Concatenate `list of string` `parts` separated by `sep`. Inverse of `strings.split`.                                                |
| [`strings`](strings.md)`.lower(s)`                    | Lowercase `s` (Unicode-aware).                                                                                                      |
| [`strings`](strings.md)`.repeat(s, n)`                | `n` non-negative copies of `s` concatenated.                                                                                        |
| [`strings`](strings.md)`.replace(s, old, new)`        | Replace **all** occurrences of `old` in `s` with `new`.                                                                             |
| [`strings`](strings.md)`.split(s, sep)`               | Split `s` on non-empty `sep`; returns `list of string`.                                                                             |
| [`strings`](strings.md)`.startsWith(s, prefix)`       | True if `s` starts with `prefix`.                                                                                                   |
| [`strings`](strings.md)`.substring(s, start)`         | Rune-indexed slice of `s` from `start` to end.                                                                                      |
| [`strings`](strings.md)`.substring(s, start, end)`    | Rune-indexed slice; **exclusive** `end`.                                                                                            |
| [`strings`](strings.md)`.trim(s)`                     | Strip leading and trailing Unicode whitespace.                                                                                      |
| [`strings`](strings.md)`.trimLeft(s)`                 | Strip leading whitespace.                                                                                                           |
| [`strings`](strings.md)`.trimRight(s)`                | Strip trailing whitespace.                                                                                                          |
| [`strings`](strings.md)`.upper(s)`                    | Uppercase `s` (Unicode-aware).                                                                                                      |
| [`task`](task.md)`.cancel($t)`                        | Request cooperative cancellation of `$t`; the body stops at its next loop checkpoint. Non-blocking.                                 |
| [`task`](task.md)`.cancelled()`                       | Non-raising poll: true if the current spawn body has been cancelled (false on main).                                                |
| [`task`](task.md)`.discard($t)`                       | Mark a `task of T` fire-and-forget; suppresses exit-time loud-fail. Returns null.                                                   |
| [`task`](task.md)`.poll($t)`                          | True if `$t` has finished (non-blocking).                                                                                           |
| [`task`](task.md)`.wait($t)`                          | Block until `$t` finishes; return its value or re-raise its error.                                                                  |
| [`task`](task.md)`.waitAll($ts)`                      | Block for all tasks in `$ts`; results in list order; re-raises the first error if any.                                              |
| [`task`](task.md)`.waitAny($ts)`                      | Block until any task in `$ts` is done; return its index.                                                                            |
| [`task`](task.md)`.waitAnyTimeout($ts, ms)`           | Like `waitAny`, but throws a catchable "timed out" error if none finishes within `ms`.                                              |
| [`task`](task.md)`.waitTimeout($t, ms)`               | Like `wait`, but throws a catchable "timed out" error if `$t` doesn't finish within `ms`.                                           |
| [`time`](time.md)`.add($t, $d)`                       | `time.Time` shifted by duration `$d`.                                                                                               |
| [`time`](time.md)`.after($a, $b)`                     | True if `$a` is strictly later than `$b`.                                                                                           |
| [`time`](time.md)`.before($a, $b)`                    | True if `$a` is strictly earlier than `$b`.                                                                                         |
| [`time`](time.md)`.day($t)`                           | Day of month, 1-31.                                                                                                                 |
| [`time`](time.md)`.equal($a, $b)`                     | True if `$a` and `$b` are the same UTC instant.                                                                                     |
| [`time`](time.md)`.format($t, layout)`                | Strftime-style format. Codes: `%Y %m %d %H %M %S %z %a %A %b %B %j %u %%`.                                                          |
| [`time`](time.md)`.fromHours(n)`                      | `time.Duration` of `n` hours.                                                                                                       |
| [`time`](time.md)`.fromIso(s)`                        | Parse RFC 3339; accepts `Z` or `+HH:MM`; optional fractional seconds.                                                               |
| [`time`](time.md)`.fromMilliseconds(n)`               | `time.Duration` of `n` milliseconds.                                                                                                |
| [`time`](time.md)`.fromMinutes(n)`                    | `time.Duration` of `n` minutes.                                                                                                     |
| [`time`](time.md)`.fromSeconds(n)`                    | `time.Duration` of `n` seconds.                                                                                                     |
| [`time`](time.md)`.fromUnix(seconds)`                 | `time.Time` at the given Unix second.                                                                                               |
| [`time`](time.md)`.fromUnixMillis(ms)`                | `time.Time` at the given Unix millisecond.                                                                                          |
| [`time`](time.md)`.fromUnixNanos(ns)`                 | `time.Time` at the given Unix nanosecond.                                                                                           |
| [`time`](time.md)`.hour($t)`                          | Hour 0-23.                                                                                                                          |
| [`time`](time.md)`.hours($d)`                         | Span as whole hours (int).                                                                                                          |
| [`time`](time.md)`.inZone($t, $z)`                    | Re-render `$t` in `$z`'s wall-clock; UTC instant is preserved.                                                                      |
| [`time`](time.md)`.iso($t)`                           | RFC 3339 string: `Z` for UTC, `+HH:MM` otherwise; fractional seconds when non-zero.                                                 |
| [`time`](time.md)`.local()`                           | Host's current `time.Zone` (name + offset).                                                                                         |
| [`time`](time.md)`.milliseconds($d)`                  | Span as whole milliseconds (int).                                                                                                   |
| [`time`](time.md)`.minute($t)`                        | Minute 0-59.                                                                                                                        |
| [`time`](time.md)`.minutes($d)`                       | Span as whole minutes (int).                                                                                                        |
| [`time`](time.md)`.month($t)`                         | Calendar month, January = 1.                                                                                                        |
| [`time`](time.md)`.nanosecond($t)`                    | Fractional second, 0-999_999_999.                                                                                                   |
| [`time`](time.md)`.now()`                             | Current instant in the host's local zone (`time.Time`).                                                                             |
| [`time`](time.md)`.parse(s, layout)`                  | Strict strftime-style parse. Same code set as format (`%j` / `%u` are format-only).                                                 |
| [`time`](time.md)`.second($t)`                        | Second 0-59.                                                                                                                        |
| [`time`](time.md)`.seconds($d)`                       | Span as whole seconds (int).                                                                                                        |
| [`time`](time.md)`.sleep($d)`                         | Block the running task for `$d`. Negative / zero returns immediately. Returns null.                                                 |
| [`time`](time.md)`.sub($a, $b)`                       | Signed `time.Duration` between two `time.Time` values.                                                                              |
| [`time`](time.md)`.unix($t)`                          | Unix-second instant of `$t` (int).                                                                                                  |
| [`time`](time.md)`.unixMillis($t)`                    | Unix-millisecond instant of `$t` (int).                                                                                             |
| [`time`](time.md)`.unixNanos($t)`                     | Unix-nanosecond instant of `$t` (int).                                                                                              |
| [`time`](time.md)`.utc()`                             | Current instant in UTC (`time.Time`).                                                                                               |
| [`time`](time.md)`.weekday($t)`                       | ISO 8601 weekday: Monday = 1 ... Sunday = 7.                                                                                        |
| [`time`](time.md)`.year($t)`                          | Calendar year (int).                                                                                                                |
| [`time`](time.md)`.zone(offset, name)`                | Build a `time.Zone` from an integer offset (seconds east of UTC) and a display name.                                                |
| [`toml`](toml.md)`.decode(s)`                         | Parse TOML text into an opaque `toml.Value` handle (walk it with the accessors below).                                             |
| [`toml`](toml.md)`.encode(v)` / `.encodePretty(v)`    | TOML string for a `toml.Value` (or native map / list / scalar); `encodePretty` blank-lines sections. Null value / non-table root errors. |
| [`toml`](toml.md)`.typeOf(v[, ptr])`                  | Node type at an optional JSON Pointer: `null` `bool` `int` `float` `string` `list` `map` `datetime`.                               |
| [`toml`](toml.md)`.get(v[, ptr])`                     | Sub-node at a JSON Pointer, as a `toml.Value` (walk stays opaque; no pointer = the node itself).                                    |
| [`toml`](toml.md)`.has(v, ptr)`                       | Whether the JSON Pointer resolves to an existing node.                                                                              |
| [`toml`](toml.md)`.keys(v[, ptr])` / `.length(v[, ptr])` | `list of string` table keys in document order / element count of a list or table.                                               |
| [`toml`](toml.md)`.asInt(v[, ptr])` / `asFloat` / `asString` / `asBool` | Extract the addressed leaf as a typed value (strict; `asFloat` promotes an int).                                 |
| [`toml`](toml.md)`.asDatetime(v[, ptr])` / `.isDatetime(v[, ptr])` | A date-time node as a `time.Time` (needs `use time;`) / whether the node is a date-time.                            |
| [`toml`](toml.md)`.map()` / `.list()`                 | A fresh empty table / array `toml.Value` - the explicit start of a document (writes never auto-vivify).                            |
| [`toml`](toml.md)`.set(v, ptr, val)` / `.insert` / `.append` / `.remove` / `.move` | Non-mutating edits by JSON Pointer; each returns a new `toml.Value` (strict / no missing intermediates). |
| [`uuid`](uuid.md)`.generate(v)`                       | New UUID string; `v` is `"v4"` (random) or `"v7"` (time-ordered).                                                                   |
| [`uuid`](uuid.md)`.isValid(s)`                        | Whether `s` is a well-formed UUID string.                                                                                           |
| [`uuid`](uuid.md)`.parse(s)`                          | The 16 `bytes` of a UUID string; errors on malformed input.                                                                         |
| [`uuid`](uuid.md)`.version(s)`                        | Version digit (4, 7, ...; 0 for NIL); errors on malformed input.                                                                     |
| [`xml`](xml.md)`.decode(s)`                           | Parse XML into an opaque `xml.Value` (root element); errors with line/column on malformed input.                                    |
| [`xml`](xml.md)`.encode(v)` / `.encodePretty(v)`      | Serialize an `xml.Value` (compact / 2-space indented).                                                                              |
| [`xml`](xml.md)`.typeOf(node)`                        | `"element"` or `"text"`.                                                                                                            |
| [`xml`](xml.md)`.tag(node)`                           | The element's tag name.                                                                                                             |
| [`xml`](xml.md)`.text(node)`                          | The element's concatenated direct character data (or a text node's string).                                                        |
| [`xml`](xml.md)`.attr(node, name)`                    | An attribute value; errors if absent. `xml.hasAttr(node, name)` tests presence.                                                     |
| [`xml`](xml.md)`.attrs(node)`                         | The attribute names (`list of string`, document order).                                                                            |
| [`xml`](xml.md)`.children(node)`                      | The element children (`list of xml.Value`; text excluded).                                                                         |
| [`xml`](xml.md)`.get(node, path)`                     | First element matching an XPath-style path (`name`/`name[k]`/`*`, `/`-separated); errors if none.                                   |
| [`xml`](xml.md)`.findAll(node, path)`                 | Every element matching the path (`list of xml.Value`). `xml.has(node, path)` -> bool.                                               |
| [`xml`](xml.md)`.element(name)`                       | A fresh empty element `xml.Value`.                                                                                                  |
| [`xml`](xml.md)`.setAttr(node, name, value)`          | The element with the attribute added/updated (fresh handle).                                                                        |
| [`xml`](xml.md)`.setText(node, s)`                    | The element with its children replaced by one text node (fresh handle).                                                            |
| [`xml`](xml.md)`.append(parent, child)`               | The parent element with `child` appended (fresh handle).                                                                            |
| [`yaml`](yaml.md)`.decode(s)` / `.decodeAll(s)`       | Parse one YAML document (multi-doc errors) / every document of a `---` stream (`list of yaml.Value`), into opaque handles.          |
| [`yaml`](yaml.md)`.encode(v)` / `.encodePretty(v)`    | YAML string for a `yaml.Value` (or native map / list / scalar): flow (compact) vs block (readable) style.                           |
| [`yaml`](yaml.md)`.typeOf(v[, ptr])`                  | Node type at an optional JSON Pointer: `null` `bool` `int` `float` `string` `bytes` `list` `map` `datetime`.                        |
| [`yaml`](yaml.md)`.get(v[, ptr])`                     | Sub-node at a JSON Pointer, as a `yaml.Value` (walk stays opaque; no pointer = the node itself).                                    |
| [`yaml`](yaml.md)`.has(v, ptr)`                       | Whether the JSON Pointer resolves to an existing node.                                                                              |
| [`yaml`](yaml.md)`.keys(v[, ptr])` / `.length(v[, ptr])` | `list of string` map keys in document order / element count of a list or map.                                                    |
| [`yaml`](yaml.md)`.asInt(v[, ptr])` / `asFloat` / `asString` / `asBool` / `isNull` | Extract the addressed leaf as a typed value (strict; `asFloat` promotes an int) / test for null.                    |
| [`yaml`](yaml.md)`.asDatetime(v[, ptr])` / `.isDatetime(v[, ptr])` | A timestamp node as a `time.Time` (needs `use time;`) / whether the node is a timestamp.                            |
| [`yaml`](yaml.md)`.map()` / `.list()`                 | A fresh empty mapping / sequence `yaml.Value` - the explicit start of a document (writes never auto-vivify).                        |
| [`yaml`](yaml.md)`.set(v, ptr, val)` / `.insert` / `.append` / `.remove` / `.move` | Non-mutating edits by JSON Pointer; each returns a new `yaml.Value` (strict / no missing intermediates). |

## Constants

| Name                                       | Type           | Value                                                                                            |
| ------------------------------------------ | -------------- | ------------------------------------------------------------------------------------------------ |
| [`math`](math.md)`.E`                      | `float`        | Euler's number, 2.718281828459045.                                                               |
| [`math`](math.md)`.PI`                     | `float`        | π, 3.141592653589793.                                                                            |
| [`math`](math.md)`.TAU`                    | `float`        | τ = 2π, the full-turn constant, 6.283185307179586. |
| [`meta`](meta.md)`.BUILD`                  | `string`       | Which Go toolchain compiled the interpreter: `"go"` / `"tinygo"`.                                |
| [`meta`](meta.md)`.VERSION`                | `string`       | The interpreter's build version (e.g. `"0.14.0"`).                                               |
| [`meta`](meta.md)`.SYSMODDIR`              | `string`       | Resolved system module directory (`--sysmoddir` > `JENNIFER_SYSMODDIR` > compile default).       |
| [`meta`](meta.md)`.CAPABILITIES`           | `list of string` | Host capabilities this build has, sorted: `["exec", "net", "sql"]` (standard) / `[]` (tiny).            |
| [`meta`](meta.md)`.hasCapability(name)`    | `bool`         | Whether the build includes capability `name` (`"net"` / `"exec"`); the `# pragma-jennifer-capability:` set. |
| [`meta`](meta.md)`.call(name, args...)`    | value          | Invoke a top-level method by runtime name (arity + types checked); errors / `exit` propagate.     |
| [`meta`](meta.md)`.defined(name)`          | `bool`         | Whether a top-level method `name` exists.                                                         |
| [`meta`](meta.md)`.callMain(name, args...)` / `.definedMain(name)` | value / `bool` | Like `call` / `defined` but against the **entry program's** methods (a module reaching its host's handlers). |
| [`os`](os.md)`.ARCH`                       | `string`       | CPU architecture: `"amd64"`, `"arm64"`, `"wasm"`, ...                                            |
| [`os`](os.md)`.ARGS`                       | list of string | Argv. Index 0 is the script path, the rest are user args.                                        |
| [`os`](os.md)`.DIRSEP`                     | `string`       | Path-component separator: `"/"` Unix, `"\\"` Windows.                                            |
| [`os`](os.md)`.EOL`                        | `string`       | Platform line ending. `"\n"` Unix-likes, `"\r\n"` Windows.                                       |
| [`os`](os.md)`.NCPU`                       | `int`          | Logical CPUs usable by the process (`runtime.NumCPU`). `1` on `jennifer-tiny` (single-thread scheduler). |
| [`os`](os.md)`.PATHSEP`                    | `string`       | PATH-list separator: `":"` Unix, `";"` Windows.                                                  |
| [`os`](os.md)`.PLATFORM`                   | `string`       | OS tag: `"linux"`, `"darwin"`, `"windows"`, ...                                                  |
| [`time`](time.md)`.PROGRAM_START`          | `time.Time`    | Captured the moment the time library installed; "since program launched" anchor.                 |
| [`time`](time.md)`.UTC`                    | `time.Zone`    | Canonical UTC: `Zone{offset: 0, name: "UTC"}`.                                                   |
| [`uuid`](uuid.md)`.NIL`                    | `string`       | The all-zero UUID `00000000-0000-0000-0000-000000000000`.                                        |

## Type-conversion calls

`int`, `float`, `string`, `bool` are also type keywords (used in `def x
as int`). The parser allows them in expression position **only** when
immediately followed by `(`, so `def x as int init convert.toInt("42");` works
but `def x as int init int;` errors. See
[convert.md](convert.md#notes-on-the-type-name-syntax) for the parser
detail.

## See also

- [index.md](index.md) - library catalog with code samples and the
  organizing principles.
- Per-library reference pages: [io.md](io.md), [convert.md](convert.md),
  [math.md](math.md), [strings.md](strings.md), [lists.md](lists.md),
  [maps.md](maps.md), [os.md](os.md), [meta.md](meta.md),
  [time.md](time.md), [hash.md](hash.md), [crc.md](crc.md),
  [encoding.md](encoding.md), [task.md](task.md), [fs.md](fs.md), [net.md](net.md), [regex.md](regex.md), [testing.md](testing.md).
- [../user-guide/imports.md](../user-guide/imports.md) - how to import a
  library in a Jennifer source file.
