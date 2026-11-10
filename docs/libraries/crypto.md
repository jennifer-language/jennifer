# `crypto` - security primitives

Enable with `use crypto;`. Security-sensitive primitives that need a
cryptographically secure random source or a timing-safe operation:
crypto-grade randomness, constant-time comparison, key derivation,
authenticated encryption, signatures, and key agreement.

Digests (MD5 / SHA-*) and the keyed-hash MAC (`hash.hmac`) live in
[`hash`](hash.md); non-cryptographic checksums live in [`crc`](crc.md).
`crypto` is the layer above them - the operations where predictability
or a timing side channel is a vulnerability.

Everything here is Go standard library only (`crypto/rand`,
`crypto/subtle`, `crypto/hkdf`, `crypto/pbkdf2`, `crypto/aes`,
`crypto/cipher`, `crypto/ed25519`, `crypto/ecdh`), so the library adds no
dependency and works on **both binaries** (`jennifer` and `jennifer-tiny`). The
exceptions are default-`jennifer` only: the RSA / ECDSA, key-generation / CSR /
JWK surface (`crypto/rsa`, `crypto/ecdsa`, `crypto/x509`) and ML-KEM
(`crypto/mlkem`).

```jennifer
use io;
use crypto;
use encoding;

def token as bytes init crypto.randBytes(32);
io.printf("session token: %s\n", encoding.toText("base64-url", $token));
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

[`uuid`](uuid.md) draws its v4 / v7 randomness here, so `uuid.v4()` is
unguessable and safe to hand a client as a session id or bearer token.

## Constant-time comparison

Comparing a computed MAC or token against a supplied one is
[`hash.equal`](hash.md), next to `hash.hmac`: it has no early-out, so the
comparison does not leak how many leading bytes matched.

## Key derivation

Two functions turn a secret into keying material. Both output `bytes`.

| Call                                          | Returns | Notes                                            |
| --------------------------------------------- | ------- | ------------------------------------------------ |
| `crypto.hkdf(algo, secret, salt, info, length)`     | `bytes` | HKDF (RFC 5869): expand a **high-entropy** secret into `length` bytes. `salt` / `info` may be empty bytes. |
| `crypto.pbkdf2(algo, password, salt, iterations, keyLen)` | `bytes` | PBKDF2 (RFC 8018): stretch a **low-entropy** password over `iterations` rounds against `salt`. |

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
  core for days; a single-block key allows up to 10⁸ iterations (about
  15 s with sha256 and 45 s with sha512 on a 2020 desktop CPU; longer on
  small hardware), and a larger `keyLen` proportionally fewer. `keyLen`
  is also capped at 1 MiB; `hkdf`'s `length` is at most 255 x the hash
  size (8160 bytes for sha256). It unblocks SASL SCRAM (see the
  [`sasl`](../modules/sasl.md) module) and password-based key wrapping.

```jennifer
use io;
use crypto;
use convert;
use encoding;

def password as bytes init convert.bytesFromString("utf-8", "correct horse");
def salt as bytes init crypto.randBytes(16);
def derived as bytes init crypto.pbkdf2("sha256", $password, $salt, 600000, 32);
io.printf("derived key: %s\n", encoding.toText("hex", $derived));
```

### Password *hashing* is not here

Storing passwords for later verification wants a memory-hard function
(Argon2id, bcrypt, scrypt), which needs a dependency outside the
standard library. That is out of scope for `crypto`; use
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
def box as bytes init crypto.encrypt($key, convert.bytesFromString("utf-8", "secret"));
def back as bytes init crypto.decrypt($key, $box);   # throws if $box was tampered
```

The key is caller-managed - store it, or derive it from a password with
`crypto.pbkdf2("sha256", ..., 32)`. Random 96-bit nonces can collide by the
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
| `crypto.sign(private, message)` | `bytes` | A 64-byte signature over `message`. A `private` whose public half (bytes 32-63) does not belong to its seed is an error. |
| `crypto.verify(public, message, signature)` | `bool` | `true` iff `signature` is `public`'s signature over `message`; `false` on any mismatch. A malformed key / signature length is a positioned error. |

```jennifer
use crypto;
use convert;

def kp as crypto.Keypair init crypto.signKeypair();
def msg as bytes init convert.bytesFromString("utf-8", "ship it");
def sig as bytes init crypto.sign($kp.private, $msg);
def ok as bool init crypto.verify($kp.public, $msg, $sig);   # true
```

Publish the `public` key; keep the `private` key secret. `verify` returning
`false` means the message, key, or signature does not match; it throws only for
a wrong-length key or signature.

Keys and nonces come from the same crypto-grade generator as `randBytes`.
Out of scope: password *hashing* (see above) and raw block modes.

## Key agreement (X25519)

`crypto.x25519*` is **X25519** Diffie-Hellman (RFC 7748): two parties each make a
keypair, swap public keys, and each computes the same 32-byte shared secret from
its own private key and the other's public key - without the secret ever
crossing the wire. It is the exchange under Noise, age, WireGuard-style
handshakes, and the classical half of a hybrid with ML-KEM (below). Over Go's
`crypto/ecdh`; works on **both binaries**.

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `crypto.x25519Keypair()` | `crypto.Keypair` | A fresh keypair: 32-byte `public` (send it to the peer) and 32-byte `private` (keep it). |
| `crypto.x25519(private, peerPublic)` | `bytes` | The 32-byte shared secret. A low-order `peerPublic` (one that would force an all-zero secret, e.g. 32 zero bytes) is a catchable error, so a hostile peer cannot pin the secret to a known value. |
| `crypto.x25519Public(private)` | `bytes` | The public key of a stored private key. |

```jennifer
use crypto;
use binary;

def alice as crypto.Keypair init crypto.x25519Keypair();
def bob as crypto.Keypair init crypto.x25519Keypair();
# ... each side sends its public key to the other ...
def a as bytes init crypto.x25519($alice.private, $bob.public);
def b as bytes init crypto.x25519($bob.private, $alice.public);   # == $a

# The raw secret is curve output, not a key: derive one, binding both public keys.
def info as bytes init binary.concat($alice.public, $bob.public);
def key as bytes init crypto.hkdf("sha256", $a, $salt, $info, 32);
def box as bytes init crypto.encrypt($key, $plaintext);
```

X25519 on its own does not say **who** the peer is: anyone can send a public
key. Authenticate the exchange - sign the public keys with `crypto.sign`, or pin
the peer's public key in advance - or a man in the middle can run one exchange
with each side. An X25519 key is not an Ed25519 key; keep separate keypairs for
signing and for key agreement.

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
| `crypto.rsaSign(algo, message, privatePem)` | `bytes` | RSASSA-PKCS#1 v1.5 signature. `privatePem` is a PKCS#1 or PKCS#8 PEM key (as `bytes`). |
| `crypto.rsaVerify(algo, message, signature, publicPem)` | `bool` | Verify against a PKIX / PKCS#1 public-key PEM. `false` on mismatch. |
| `crypto.ecdsaSign(algo, message, privatePem)` | `bytes` | ECDSA signature in the **JOSE R\|\|S** form (fixed-width, what JWT / WebCrypto use), not ASN.1 DER. `privatePem` is a SEC1 or PKCS#8 EC key. |
| `crypto.ecdsaVerify(algo, message, signature, publicPem)` | `bool` | Verify against a PKIX EC public-key PEM. A wrong-length signature is `false`, not an error. |

`algo` is the digest: `"sha256"` / `"sha384"` / `"sha512"` (JWT's 256 / 384 / 512
variants). The curve of an ECDSA key is taken from the key itself (P-256 for
`ES256`, and so on). A key that is not valid PEM, or not the key type the call
expects, is a positioned error; a genuine signature mismatch is `false`. An RSA
key (PEM, JWK, or `csr` input) with a modulus over 16384 bits is an error
(`RSA modulus too large`): verify cost grows with the square of the size.

```jennifer
use crypto;
use fs;
use convert;

def priv as bytes init fs.readBytes("rsa_private.pem");
def pub as bytes init fs.readBytes("rsa_public.pem");
def msg as bytes init convert.bytesFromString("utf-8", "payload");
def sig as bytes init crypto.rsaSign("sha256", $msg, $priv);        # RS256
def ok as bool init crypto.rsaVerify("sha256", $msg, $sig, $pub);   # true
```

These are the primitives the [`jwt`](../modules/jwt.md) module's `RS*` / `ES*`
algorithms build on. Still out of scope: x509 *certificate* handling (chains,
SANs, expiry) - key parsing is all that is exposed.

## Key generation, CSR, and JWK

The rest of the asymmetric surface, used by an [ACME](../modules/acme.md) client
to mint keys and request certificates. Same default-binary-only split.

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `crypto.rsaGenerateKey(bits)` | `bytes` | A fresh RSA private key as PKCS#8 PEM. `bits` is 2048, 3072, or 4096. |
| `crypto.ecGenerateKey(curve)` | `bytes` | A fresh EC private key as SEC1 PEM. `curve` is `"p256"`, `"p384"`, or `"p521"`. |
| `crypto.jwkPublic(privatePem)` | `string` | The **RFC 7638 canonical** public JWK JSON of the key (members sorted, no whitespace). |
| `crypto.jwkToPem(jwkJson)` | `string` | The **inverse** of `jwkPublic`: parse a public JWK (an RSA `{kty, n, e}` or EC `{kty, crv, x, y}` key) and return a PKIX / SubjectPublicKeyInfo `PUBLIC KEY` PEM - exactly the shape `rsaVerify` / `ecdsaVerify` accept. Resolves a JWKS entry (by `kid`) to a verification key. Curves `P-256` / `P-384` / `P-521`; an off-curve point or malformed JWK is an error. |
| `crypto.csr(privatePem, domains)` | `bytes` | A DER PKCS#10 certificate-signing request over a `list of string` of domains (subject-alt DNS names; the first is the common name), signed with the key. |

`jwkPublic` is canonical, so its SHA-256 is the **JWK thumbprint** (RFC 7638) -
the value ACME challenges are keyed on:

```jennifer
use crypto;
use hash;
use convert;

def key as bytes init crypto.ecGenerateKey("p256");
def jwk as string init crypto.jwkPublic($key);                # {"crv":"P-256","kty":"EC",...}
def thumbprint as bytes init hash.compute("sha256", convert.bytesFromString("utf-8", $jwk));
def csr as bytes init crypto.csr($key, ["example.com", "www.example.com"]);
```

## EC-SRP (MikroTik MAC-Telnet / Winbox login)

`crypto.mtwei*` is the client side of the Elliptic-Curve Secure Remote Password
(EC-SRP) exchange RouterOS 6.43+ and all of v7 use for MAC-Telnet / Winbox login
(the modern replacement for the old MD5 salt). It is a dependency-free port
(`math/big` + `crypto/sha256`) of the reference algorithm over Curve25519 in
short-Weierstrass form, TinyGo-clean. The

[`mactelnet`](../modules/mactelnet.md) module drives these; you rarely call them
directly.

| Call | Returns | Notes |
| --- | --- | --- |
| `crypto.mtweiKeygen()` | `crypto.Keypair` | A client keypair: 32-byte `private` scalar, 33-byte compressed `public` key. |
| `crypto.mtweiId(user, password, salt)` | `bytes` | The 32-byte SRP validator `SHA256(salt + SHA256(user ":" password))`; `salt` is the router's 16-byte salt. |
| `crypto.mtweiClientKey(private, serverKey, clientKey, validator)` | `bytes` | The 32-byte authenticator sent as the login proof, from your `private`, the router's 33-byte `serverKey`, your 33-byte `clientKey`, and the `validator`. A `serverKey` off the curve, one crafted to collapse the shared point to infinity, or a `validator` that is zero modulo the group order is a catchable error, not a crash. |

The password itself never crosses the wire; only the derived proof does, and a
wrong password yields a proof the router will not match.

## Post-quantum key encapsulation (ML-KEM)

`crypto.mlkem*` is **ML-KEM-768** (NIST FIPS 203, formerly Kyber) - a
quantum-resistant **key encapsulation mechanism**. A KEM is not an encryptor: it
lets two parties agree on a fresh random 32-byte shared secret, which you then
run through `crypto.hkdf` and `crypto.encrypt` (AES-256-GCM). Over the Go
standard library's `crypto/mlkem`, so dependency-free; **default-binary only** -
`jennifer-tiny` returns a friendly error (the FIPS-140 / SHA-3 machinery is off
the TinyGo build), the same split as the RSA / ECDSA surface.

| Call | Returns | Notes |
| --- | --- | --- |
| `crypto.mlkemKeypair()` | `crypto.Keypair` | A fresh ML-KEM-768 keypair: 1184-byte `public` (the encapsulation key, publish it) and 64-byte `private` (the seed, keep it). |
| `crypto.mlkemEncapsulate(public)` | `crypto.Encapsulation` | Wrap a fresh shared secret under `public`: returns `ciphertext` (1088 bytes, send it to the key holder) and `sharedSecret` (32 bytes, keep it). |
| `crypto.mlkemDecapsulate(private, ciphertext)` | `bytes` | Recover the 32-byte shared secret from `ciphertext` with `private`. A ciphertext made for another key yields a different (useless) secret by design (implicit rejection), not an error; a malformed key / ciphertext length is a catchable error. |

```jennifer
use crypto;
use convert;

# Bob publishes a public key; Alice encapsulates a secret under it.
def kp as crypto.Keypair init crypto.mlkemKeypair();
def enc as crypto.Encapsulation init crypto.mlkemEncapsulate($kp.public);

# Bob decapsulates the ciphertext and gets the same secret.
def bob as bytes init crypto.mlkemDecapsulate($kp.private, $enc.ciphertext);
# hash.equal($enc.sharedSecret, $bob) is true.

# The shared secret is key-establishment material, not a key: derive one.
def key as bytes init crypto.hkdf("sha256", $bob, $salt, $info, 32);
def box as bytes init crypto.encrypt($key, $plaintext);        # AES-256-GCM
```

**Not a complete migration on its own.** Deployed post-quantum key agreement is
**hybrid** - ML-KEM combined with a classical exchange (X25519), so a break in
either scheme alone is survivable (this is what TLS 1.3's `X25519MLKEM768` does).
Use `mlkem*` as the post-quantum half and [`x25519`](#key-agreement-x25519) as
the classical one: concatenate both shared secrets and feed them through
`crypto.hkdf`, rather than shipping ML-KEM-only.

```jennifer
# The receiver publishes both public keys; the sender answers with an ML-KEM
# ciphertext and its own X25519 public key.
def pq as bytes init $enc.sharedSecret;                            # ML-KEM half
def classic as bytes init crypto.x25519($mine.private, $theirs);    # X25519 half
def key as bytes init crypto.hkdf("sha256", binary.concat($pq, $classic), $salt, $info, 32);
```

## Errors

Every function validates argument kinds and counts and raises a
positioned runtime error on misuse (wrong type, negative `randBytes`
length, `lo > hi`, non-positive `iterations` / `keyLen` / `length`, a key
that is not 32 bytes for `encrypt` / `decrypt`, a wrong-length Ed25519 key or
signature, a wrong-length X25519 key or a low-order X25519 peer key).

The functions that draw randomness (`randBytes`, `randInt`, `encrypt`'s nonce,
`mtweiKeygen`; and `uuid.v4` / `v7`, which share the source) raise a catchable
`secure random source unavailable` error if the operating system's random
source cannot be read. There is no weak fallback: the call fails rather than
return predictable bytes. The standard `jennifer` reads the `getrandom` system
call and does not hit this in practice; `jennifer-tiny` reads `/dev/urandom`,
which a minimal container or embedded image may lack (see
[TinyGo notes](../technical/tinygo.md)).
