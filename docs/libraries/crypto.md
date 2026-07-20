# `crypto` - security primitives

Enable with `use crypto;`. The home for the security-sensitive
primitives that need a cryptographically secure random source or a
timing-safe operation: crypto-grade randomness, constant-time
comparison, and the two standard key-derivation functions.

Digests (MD5 / SHA-*) and the keyed-hash MAC (`hash.hmac`) live in
[`hash`](hash.md); non-cryptographic checksums live in [`crc`](crc.md).
`crypto` is the layer above them - the operations where predictability
or a timing side channel is a vulnerability.

Everything here is Go standard library only (`crypto/rand`,
`crypto/subtle`, `crypto/hkdf`, `crypto/pbkdf2`, `crypto/aes`,
`crypto/cipher`, `crypto/ed25519`), so the library adds no dependency and
works on **both binaries** (`jennifer` and `jennifer-tiny`). The one exception
is the RSA / ECDSA signature surface below (`crypto/rsa`, `crypto/ecdsa`,
`crypto/x509`), which is default-`jennifer` only - everything else runs on both.

```jennifer
use io;
use crypto;
use encoding;

def token as bytes init crypto.randBytes(32);
io.printf("session token: %s\n", encoding.toText($token, "base64-url"));
```

## Crypto-grade randomness

Unlike [`math`](math.md)'s `rand` family (fast, seedable, and therefore
**predictable** - a PRNG for simulations and sampling), these draw from
the operating system's cryptographically secure source. They are
**not** seedable: there is no reproducible sequence, by design.

| Call                   | Returns | Notes                                                        |
| ---------------------- | ------- | ------------------------------------------------------------ |
| `crypto.randBytes(n)`  | `bytes` | `n` secure random bytes (`0 <= n <= 64 MiB`; `0` yields empty bytes). |
| `crypto.randInt(lo, hi)` | `int` | A uniform int in the **inclusive** range `[lo, hi]`.        |

`crypto.randInt` has the same shape as [`math.randInt`](math.md) but is
unpredictable and unseedable - the drop-in when the value guards
something (a token, a nonce, a shuffle of secret material). It uses
rejection sampling, so the distribution is exactly uniform with no
modulo bias, across the full `int` range.

```jennifer
use crypto;

def dieRoll as int init crypto.randInt(1, 6);        # secure, unbiased
def key as bytes init crypto.randBytes(16);          # a 128-bit key
```

Since this library landed, [`uuid`](uuid.md) draws its v4 / v7
randomness here, so `uuid.generate("v4")` is unguessable and safe to
hand a client as a session id or bearer token.

## Constant-time comparison

`crypto.hmacEqual(a, b)` compares two `bytes` values for equality
**without an early-out**, so an attacker cannot recover a secret byte by
byte from response-timing differences. Use it to check a computed MAC
against a supplied one, rather than `==` (which short-circuits on the
first differing byte). Unequal-length inputs return `false`.

```jennifer
use crypto;
use hash;
use convert;

def key as bytes init convert.bytesFromString("secret", "utf-8");
def body as bytes init convert.bytesFromString("payload", "utf-8");
def expected as bytes init hash.hmac($key, $body, "sha256");
def supplied as bytes init requestSignature();      # from the caller

if (crypto.hmacEqual($expected, $supplied)) {
    # signature verified
}
```

## Key derivation

Two functions turn a secret into keying material. Both output `bytes`.

| Call                                          | Returns | Notes                                            |
| --------------------------------------------- | ------- | ------------------------------------------------ |
| `crypto.hkdf(secret, salt, info, length, algo)`     | `bytes` | HKDF (RFC 5869): expand a **high-entropy** secret into `length` bytes. `salt` / `info` may be empty bytes. |
| `crypto.pbkdf(password, salt, iterations, keyLen, algo)` | `bytes` | PBKDF2 (RFC 8018): stretch a **low-entropy** password over `iterations` rounds against `salt`. |

`algo` names the PRF hash: `"sha1"`, `"sha256"`, or `"sha512"` (the
`hash` library's names, minus `md5` - too weak to derive a key with). An
unknown algorithm is a positioned error. Use `"sha256"` unless you are
matching an external protocol: SCRAM-SHA-1 (MongoDB, XMPP) requires
`"sha1"`.

- **HKDF** derives one or more subkeys from material that is already
  strong (a shared secret, a master key). It is fast; do not use it to
  hash passwords.
- **PBKDF2** is for passwords: the `iterations` count makes each guess
  expensive, so pick the largest value the deployment can afford. The
  total work - `ceil(keyLen / hashLen) × iterations` - is capped so an
  untrusted parameter (e.g. a hostile SCRAM server's `i=`) cannot pin a
  core for days; a single-block key allows up to ~10⁸ iterations (worst
  case ~20 s), and a larger `keyLen` proportionally fewer. `keyLen` is
  also capped at 1 MiB, and `hkdf`'s `length` likewise - real key
  material is tens of bytes. It unblocks SASL SCRAM (see the
  [`sasl`](../modules/sasl.md) module) and password-based key wrapping.

```jennifer
use crypto;
use convert;
use encoding;

def password as bytes init convert.bytesFromString("correct horse", "utf-8");
def salt as bytes init crypto.randBytes(16);
def derived as bytes init crypto.pbkdf($password, $salt, 600000, 32, "sha256");
io.printf("derived key: %s\n", encoding.toText($derived, "hex"));
```

> **Why `pbkdf`, not `pbkdf2`?** A Jennifer method name is letters-only,
> so the "2" cannot appear - the same rule that makes uuid's version a
> string argument (`uuid.generate("v4")`). The scheme is still PBKDF2;
> only the name drops the digit.

### Password *hashing* is not here

Storing passwords for later verification wants a memory-hard function
(Argon2id, bcrypt, scrypt), which needs a dependency outside the
standard library. That is deliberately out of scope for `crypto`; use
PBKDF2 only where an interoperable KDF is required (e.g. SCRAM), not as
a general password store.

## Authenticated encryption

`crypto.encrypt` / `crypto.decrypt` are **AES-256-GCM**, an authenticated
(AEAD) cipher: the ciphertext carries a tag, so tampering is *detected*, not
silently decrypted into garbage. There is one algorithm and no mode / nonce
knobs - the footguns (ECB, IV reuse) are not expressible.

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `crypto.encrypt(key, plaintext)` | `bytes` | Seal. `key` must be exactly 32 bytes (AES-256). A fresh 12-byte nonce is generated and **prepended**: the result is `nonce \|\| ciphertext \|\| tag`, so you never handle (or reuse) a nonce. |
| `crypto.decrypt(key, box)` | `bytes` | Open. Splits the nonce, verifies the tag, returns the plaintext. A wrong key or a tampered box is a **catchable authentication error**. |

```jennifer
use crypto;
use convert;

def key as bytes init crypto.randBytes(32);          # keep this secret
def box as bytes init crypto.encrypt($key, convert.bytesFromString("secret", "utf-8"));
def back as bytes init crypto.decrypt($key, $box);   # throws if $box was tampered
```

The key is caller-managed - store it, or derive it from a password with
`crypto.pbkdf(..., 32, "sha256")`. Random 96-bit nonces can collide by the
birthday bound, so NIST's guidance (SP 800-38D) caps one key at **2^32
messages** (~4 billion) to keep that chance below 2^-32 - rotate the key before
then, which no ordinary workload approaches. Encrypting the same plaintext
twice yields different boxes (fresh nonce each time).

## Signatures

`crypto.sign` / `crypto.verify` are **Ed25519** - a modern signature scheme with
no parameters to choose. A keypair is a namespaced struct
`crypto.Keypair { public as bytes, private as bytes }`.

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `crypto.signKeypair()` | `crypto.Keypair` | A fresh keypair (32-byte `public`, 64-byte `private`). |
| `crypto.sign(private, message)` | `bytes` | A 64-byte signature over `message`. |
| `crypto.verify(public, message, signature)` | `bool` | `true` iff `signature` is `public`'s signature over `message`; `false` on any mismatch. A malformed key / signature length is a positioned error. |

```jennifer
use crypto;
use convert;

def kp as crypto.Keypair init crypto.signKeypair();
def msg as bytes init convert.bytesFromString("ship it", "utf-8");
def sig as bytes init crypto.sign($kp.private, $msg);
def ok as bool init crypto.verify($kp.public, $msg, $sig);   # true
```

Publish the `public` key; keep the `private` key secret. `verify` returning
`false` means the message, key, or signature does not match - it never throws for
a genuine mismatch, only for a wrong-length key or signature.

The random source draws from the same crypto-grade generator as `randBytes`.
All Go stdlib (`crypto/aes`, `crypto/cipher`, `crypto/ed25519`), so both binaries
carry it. Out of scope by design: password *hashing* (see above) and raw block
modes.

## Asymmetric signatures (RSA / ECDSA)

For interop with formats that mandate RSA or ECDSA - **JWT** `RS256` / `ES256`
foremost - `crypto` signs and verifies with **PEM-encoded** keys. These are the
one part of `crypto` that is **default-binary only**: they pull in `crypto/rsa`,
`crypto/ecdsa`, and `crypto/x509` (for PEM parsing), which are off the TinyGo
build, so on `jennifer-tiny` they raise a friendly "not available" error (the
same build-tag split as [`net`](net.md)). For a modern signature with no key
files, prefer Ed25519 (`crypto.sign`, above), which runs on both binaries.

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `crypto.rsaSign(privatePem, message, algo)` | `bytes` | RSASSA-PKCS#1 v1.5 signature. `privatePem` is a PKCS#1 or PKCS#8 PEM key (as `bytes`). |
| `crypto.rsaVerify(publicPem, message, signature, algo)` | `bool` | Verify against a PKIX / PKCS#1 public-key PEM. `false` on mismatch. |
| `crypto.ecdsaSign(privatePem, message, algo)` | `bytes` | ECDSA signature in the **JOSE R\|\|S** form (fixed-width, what JWT / WebCrypto use), not ASN.1 DER. `privatePem` is a SEC1 or PKCS#8 EC key. |
| `crypto.ecdsaVerify(publicPem, message, signature, algo)` | `bool` | Verify against a PKIX EC public-key PEM. A wrong-length signature is `false`, not an error. |

`algo` is the digest: `"sha256"` / `"sha384"` / `"sha512"` (JWT's 256 / 384 / 512
variants). The curve of an ECDSA key is taken from the key itself (P-256 for
`ES256`, and so on). A key that is not valid PEM, or not the key type the call
expects, is a positioned error; a genuine signature mismatch is `false`.

```jennifer
use crypto;
use fs;
use convert;

def priv as bytes init fs.readBytes("rsa_private.pem");
def pub as bytes init fs.readBytes("rsa_public.pem");
def msg as bytes init convert.bytesFromString("payload", "utf-8");
def sig as bytes init crypto.rsaSign($priv, $msg, "sha256");        # RS256
def ok as bool init crypto.rsaVerify($pub, $msg, $sig, "sha256");   # true
```

These are the primitives the [`jwt`](../modules/jwt.md) module's `RS*` / `ES*`
algorithms build on. Still out of scope: x509 *certificate* handling (chains,
SANs, expiry) - key parsing is all that is exposed.

## Key generation, CSR, and JWK

The rest of the asymmetric surface - what an [ACME](../modules/acme.md) client
needs to mint keys and request certificates. Same default-binary-only split.

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `crypto.rsaGenerateKey(bits)` | `bytes` | A fresh RSA private key as PKCS#8 PEM. `bits` is 2048, 3072, or 4096. |
| `crypto.ecGenerateKey(curve)` | `bytes` | A fresh EC private key as SEC1 PEM. `curve` is `"p256"`, `"p384"`, or `"p521"`. |
| `crypto.jwkPublic(privatePem)` | `string` | The **RFC 7638 canonical** public JWK JSON of the key (members sorted, no whitespace). |
| `crypto.csr(privatePem, domains)` | `bytes` | A DER PKCS#10 certificate-signing request over a `list of string` of domains (subject-alt DNS names; the first is the common name), signed with the key. |

`jwkPublic` is canonical, so its SHA-256 is the **JWK thumbprint** (RFC 7638) -
the value ACME challenges are keyed on:

```jennifer
use crypto;
use hash;
use encoding;

def key as bytes init crypto.ecGenerateKey("p256");
def jwk as string init crypto.jwkPublic($key);                # {"crv":"P-256","kty":"EC",...}
def thumbprint as bytes init hash.compute(convert.bytesFromString($jwk, "utf-8"), "sha256");
def csr as bytes init crypto.csr($key, ["example.com", "www.example.com"]);
```

## Errors

Every function validates argument kinds and counts and raises a
positioned runtime error on misuse (wrong type, negative `randBytes`
length, `lo > hi`, non-positive `iterations` / `keyLen` / `length`, a key
that is not 32 bytes for `encrypt` / `decrypt`, a wrong-length Ed25519 key or
signature). The secure random source is assumed always available on the
supported platform; an impossible failure aborts rather than returning weak
bytes.
