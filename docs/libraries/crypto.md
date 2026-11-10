# `crypto` - security primitives

Enable with `use crypto;`. Security-sensitive primitives that need a
cryptographically secure random source or a timing-safe operation:
crypto-grade randomness, key derivation, encryption, signatures, key
agreement and key encapsulation.

Digests (MD5 / SHA-*), the keyed-hash MAC (`hash.hmac`) and the
constant-time compare (`hash.equal`) live in [`hash`](hash.md);
non-cryptographic checksums live in [`crc`](crc.md). `crypto` is the layer
above them - the operations where predictability or a timing side channel
is a vulnerability.

**One function per operation, the algorithm its first argument.** No
function name hides a fixed algorithm: `crypto.sign("ed25519", ...)`,
`crypto.sign("rsa-sha256", ...)`, `crypto.keypair("p256")`. A new algorithm
is a new name, never a new function. The operations:

| Operation | Functions | Algorithms |
| --- | --- | --- |
| [Encryption](#encryption) | `encrypt` / `decrypt` | `"aes-gcm"` |
| [Block cipher modes](#block-cipher-modes) (interop only) | `blockEncrypt` / `blockDecrypt` | `"aes-ecb-nopad"`, `"aes-ecb-pkcs7"`, `"aes-cbc-nopad"`, `"aes-cbc-pkcs7"`, `"aes-ctr"`, `"aes-cfb"` |
| [Signatures](#signatures) | `sign` / `verify` | `"ed25519"`, `"rsa-sha256"` / `384` / `512`, `"ecdsa-sha256"` / `384` / `512` |
| [Keys](#keys) | `keypair` / `publicKey` | `"ed25519"`, `"x25519"`, `"mlkem768"`, `"rsa2048"` / `3072` / `4096`, `"p256"` / `"p384"` / `"p521"` |
| [Key agreement](#key-agreement) | `agree` | `"x25519"` |
| [Key encapsulation](#key-encapsulation-ml-kem) | `encapsulate` / `decapsulate` | `"mlkem768"` |
| [Key wrapping](#key-wrapping-rsa-oaep) | `wrap` / `unwrap` | `"rsa-oaep-sha1"`, `"rsa-oaep-sha256"` |

An unknown name is an error that lists the known ones.

Everything here is Go standard library only, so the library adds no
dependency. AES, Ed25519 and X25519 work on **both binaries** (`jennifer`
and `jennifer-tiny`). RSA and ECDSA (`crypto/x509`), RSA-OAEP and ML-KEM
(`crypto/mlkem`) are default-`jennifer` only: on `jennifer-tiny` those
algorithm names are an error naming the algorithm, as are `jwkPublic`,
`jwkToPem` and `csr`.

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

## Encryption

`crypto.encrypt` / `crypto.decrypt` with `"aes-gcm"` are **AES-GCM**, an
authenticated (AEAD) cipher: the ciphertext carries a tag, so tampering is
*detected*, not silently decrypted into garbage. This is the one to use for
your own data.

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `crypto.encrypt(algo, plaintext, key, nonce, aad)` | `bytes` | The ciphertext with the 16-byte tag appended. `key` is 16, 24 or 32 bytes (AES-128 / 192 / 256), `nonce` 12 bytes, `aad` (data authenticated but not encrypted, such as a header) may be empty. |
| `crypto.decrypt(algo, box, key, nonce, aad)` | `bytes` | The plaintext. A wrong key, nonce or aad, or a tampered box, is one catchable **authentication error**. |

**The nonce must never repeat under one key.** Reusing one exposes the XOR of
the two plaintexts and lets an attacker forge messages. The usual way: a fresh
random nonce per message, stored in front of the box.

```jennifer
use crypto;
use convert;
use binary;

def key as bytes init crypto.randBytes(32);          # keep this secret
def none as bytes init convert.bytesFromString("utf-8", "");

# Seal: a fresh nonce per message, kept in front of the ciphertext.
def nonce as bytes init crypto.randBytes(12);
def sealed as bytes init crypto.encrypt("aes-gcm", convert.bytesFromString("utf-8", "secret"), $key, $nonce, $none);
def box as bytes init binary.concat($nonce, $sealed);

# Open: split the nonce off again. Throws if anything was changed.
def back as bytes init crypto.decrypt("aes-gcm", $box[12..len($box)], $key, $box[0..12], $none);
```

Random 96-bit nonces can collide by the birthday bound, so NIST's guidance (SP
800-38D) caps one key at **2^32 messages** (~4 billion) - rotate the key before
then. The key is caller-managed: store it, or derive it from a password with
`crypto.pbkdf2("sha256", ..., 32)`. The
[`@deckyard/sealbox`](https://registry.jennifer-lang.dev/) deck wraps this
pattern (and formats such as Fernet) in a one-call `seal` / `open`.

## Block cipher modes

**For a protocol or file format you don't control. For your own data use
`encrypt`.** `crypto.blockEncrypt` / `crypto.blockDecrypt` are the NIST SP
800-38A modes with **no authentication**: a changed ciphertext decrypts to
changed plaintext (or a padding error) without anything noticing.

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `crypto.blockEncrypt(algo, data, key, iv)` | `bytes` | `key` 16, 24 or 32 bytes; `iv` 16 bytes, and `null` for ECB. |
| `crypto.blockDecrypt(algo, data, key, iv)` | `bytes` | The inverse. |

| `algo` | Mode | Padding |
| --- | --- | --- |
| `"aes-ecb-nopad"` / `"aes-ecb-pkcs7"` | ECB, each block alone (identical blocks give identical output) | none / PKCS#7 |
| `"aes-cbc-nopad"` / `"aes-cbc-pkcs7"` | CBC | none / PKCS#7 |
| `"aes-ctr"` | CTR (a stream: any length) | none |
| `"aes-cfb"` | CFB-128 (a stream: any length) | none |

The padding is part of the name, so it is always spelled out. With `-nopad`,
data that is not a multiple of 16 bytes is an error; a PKCS#7 padding that does
not check out on decrypt is an error (`bad padding`). An IV for ECB, or none
for the other modes, is an error.

To authenticate block-mode data, **encrypt then MAC**: compute
[`hash.hmac`](hash.md) over the IV and ciphertext with a second key, and check
it with `hash.equal` **before** decrypting. Checking first is what keeps a
padding oracle out: an attacker who can tell a padding error from a MAC error
can decrypt CBC ciphertext byte by byte. Formats MAC different inputs (Fernet
covers `version || timestamp || iv || ciphertext`), so there is no fixed
function for it; this is the shape:

```jennifer
def want as bytes init hash.hmac("sha256", binary.concat($iv, $ct), $macKey);
if (not hash.equal($want, $mac)) {
    throw Error{kind: "crypto", message: "authentication failed"};
}
def plain as bytes init crypto.blockDecrypt("aes-cbc-pkcs7", $ct, $encKey, $iv);
```

## Signatures

`crypto.sign` / `crypto.verify`, the algorithm first:

| `algo` | Keys | Signature |
| --- | --- | --- |
| `"ed25519"` | raw bytes: 64-byte private, 32-byte public | 64 bytes |
| `"rsa-sha256"` / `"rsa-sha384"` / `"rsa-sha512"` | PEM (private PKCS#1 or PKCS#8, public PKIX or PKCS#1) | RSASSA-PKCS#1 v1.5 |
| `"ecdsa-sha256"` / `"ecdsa-sha384"` / `"ecdsa-sha512"` | PEM (private SEC1 or PKCS#8, public PKIX); the curve comes from the key | JOSE `R \|\| S`, fixed width (what JWT and WebCrypto use), not ASN.1 DER |

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `crypto.sign(algo, message, privateKey)` | `bytes` | The signature. |
| `crypto.verify(algo, message, signature, publicKey)` | `bool` | `false` on any mismatch, a wrong-length signature included. A key that is malformed or of the wrong type for `algo` is an error. |

```jennifer
use crypto;
use convert;

def kp as crypto.Keypair init crypto.keypair("ed25519");
def msg as bytes init convert.bytesFromString("utf-8", "ship it");
def sig as bytes init crypto.sign("ed25519", $msg, $kp.private);
def ok as bool init crypto.verify("ed25519", $msg, $sig, $kp.public);   # true
```

Publish the public key; keep the private key secret. An Ed25519 private key
whose public half (bytes 32-63) does not belong to its seed is refused: two
signatures of one message under such a key would reveal the private scalar. An
RSA key with a modulus over 16384 bits is an error (`RSA modulus too large`),
since verify cost grows with the square of the size. These are what the
[`jwt`](../modules/jwt.md) module's `EdDSA`, `RS*` and `ES*` algorithms build on.

## Keys

`crypto.keypair(algo)` makes a fresh `crypto.Keypair { public as bytes, private
as bytes }`; `crypto.publicKey(algo, privateKey)` gives the public key of a
stored private key, in the same form. Each algorithm keeps the form its
ecosystem uses:

| `algo` | `private` | `public` | Used by |
| --- | --- | --- | --- |
| `"ed25519"` | 64 raw bytes (seed and public half) | 32 raw bytes | `sign` / `verify` |
| `"x25519"` | 32 raw bytes | 32 raw bytes | `agree` |
| `"mlkem768"` | 64 raw bytes (the seed) | 1184 raw bytes | `encapsulate` / `decapsulate` |
| `"rsa2048"` / `"rsa3072"` / `"rsa4096"` | PKCS#8 PEM | PKIX PEM | `sign` (`rsa-*`), `wrap` / `unwrap` |
| `"p256"` / `"p384"` / `"p521"` | SEC1 PEM | PKIX PEM | `sign` (`ecdsa-*`) |

`publicKey` checks the key against the name: a 2048-bit key for `"rsa3072"`,
or a P-256 key for `"p384"`, is an error. An Ed25519 key is not an X25519 key;
keep separate keypairs for signing and for key agreement.

Three more functions serve an [ACME](../modules/acme.md) client (default binary
only):

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `crypto.jwkPublic(privatePem)` | `string` | The **RFC 7638 canonical** public JWK JSON of an RSA or EC key (members sorted, no whitespace). |
| `crypto.jwkToPem(jwkJson)` | `string` | The inverse: a public JWK (RSA `{kty, n, e}` or EC `{kty, crv, x, y}`) as a PKIX `PUBLIC KEY` PEM, the form `verify` takes. Resolves a JWKS entry (by `kid`) to a verification key. Curves `P-256` / `P-384` / `P-521`; an off-curve point or malformed JWK is an error. |
| `crypto.csr(privatePem, domains)` | `bytes` | A DER PKCS#10 certificate-signing request over a `list of string` of domains (subject-alt DNS names; the first is the common name), signed with the key. |

`jwkPublic` is canonical, so its SHA-256 is the **JWK thumbprint** (RFC 7638) -
the value ACME challenges are keyed on:

```jennifer
use crypto;
use hash;
use convert;

def key as crypto.Keypair init crypto.keypair("p256");
def jwk as string init crypto.jwkPublic($key.private);       # {"crv":"P-256","kty":"EC",...}
def thumbprint as bytes init hash.compute("sha256", convert.bytesFromString("utf-8", $jwk));
def csr as bytes init crypto.csr($key.private, ["example.com", "www.example.com"]);
```

## Key agreement

`crypto.agree("x25519", privateKey, peerPublic)` is **X25519** Diffie-Hellman
(RFC 7748): two parties each make a keypair, swap public keys, and each
computes the same 32-byte shared secret from its own private key and the
other's public key, without the secret crossing the wire. It is the exchange
under Noise, age and WireGuard-style handshakes, and the classical half of a
hybrid with ML-KEM (below). A low-order peer key (one that would force an
all-zero secret, such as 32 zero bytes) is an error, so a hostile peer cannot
pin the secret to a known value.

```jennifer
use crypto;
use binary;

def alice as crypto.Keypair init crypto.keypair("x25519");
def bob as crypto.Keypair init crypto.keypair("x25519");
# ... each side sends its public key to the other ...
def a as bytes init crypto.agree("x25519", $alice.private, $bob.public);
def b as bytes init crypto.agree("x25519", $bob.private, $alice.public);   # == $a

# The raw secret is curve output, not a key: derive one, binding both public keys.
def info as bytes init binary.concat($alice.public, $bob.public);
def key as bytes init crypto.hkdf("sha256", $a, $salt, $info, 32);
```

X25519 on its own does not say **who** the peer is: anyone can send a public
key. Authenticate the exchange - sign the public keys, or pin the peer's public
key in advance - or a man in the middle can run one exchange with each side.

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

## Key encapsulation (ML-KEM)

`crypto.encapsulate` / `crypto.decapsulate` with `"mlkem768"` are **ML-KEM-768**
(NIST FIPS 203, formerly Kyber), a quantum-resistant **key encapsulation
mechanism**. A KEM is not an encryptor: it gives two parties a fresh random
32-byte shared secret, which you then run through `crypto.hkdf` and use with
`crypto.encrypt`. Default binary only.

| Call | Returns | Notes |
| --- | --- | --- |
| `crypto.encapsulate(algo, publicKey)` | `crypto.Encapsulation` | A fresh shared secret wrapped under `publicKey`: `ciphertext` (1088 bytes, send it to the key holder) and `sharedSecret` (32 bytes, keep it). |
| `crypto.decapsulate(algo, ciphertext, privateKey)` | `bytes` | The 32-byte shared secret. A ciphertext made for another key yields a different, useless secret by design (implicit rejection), not an error; a malformed key or ciphertext length is an error. |

```jennifer
use crypto;

# Bob publishes a public key; Alice encapsulates a secret under it.
def kp as crypto.Keypair init crypto.keypair("mlkem768");
def enc as crypto.Encapsulation init crypto.encapsulate("mlkem768", $kp.public);

# Bob decapsulates the ciphertext and gets the same secret.
def bob as bytes init crypto.decapsulate("mlkem768", $enc.ciphertext, $kp.private);

# The shared secret is key-establishment material, not a key: derive one.
def key as bytes init crypto.hkdf("sha256", $bob, $salt, $info, 32);
```

**Not a complete migration on its own.** Deployed post-quantum key agreement is
**hybrid** - ML-KEM combined with a classical exchange (X25519), so a break in
either scheme alone is survivable (this is what TLS 1.3's `X25519MLKEM768`
does). Concatenate both shared secrets and feed them through `crypto.hkdf`:

```jennifer
def pq as bytes init $enc.sharedSecret;                                  # ML-KEM half
def classic as bytes init crypto.agree("x25519", $mine.private, $theirs); # X25519 half
def key as bytes init crypto.hkdf("sha256", binary.concat($pq, $classic), $salt, $info, 32);
```

## Key wrapping (RSA-OAEP)

`crypto.wrap` / `crypto.unwrap` are **RSAES-OAEP**: the usual way to encrypt a
symmetric key for the holder of an RSA key. Default binary only.

| Call | Returns | Notes |
| --- | --- | --- |
| `crypto.wrap(algo, plaintext, publicKey)` | `bytes` | `publicKey` is a PKIX or PKCS#1 PEM. The plaintext must be short (up to the key size minus 2 x hash size minus 2 bytes: 190 bytes for a 2048-bit key with SHA-256). |
| `crypto.unwrap(algo, ciphertext, privateKey)` | `bytes` | The plaintext. Every failure is the same `decryption failed` error, so the error tells an attacker nothing. |

`algo` is `"rsa-oaep-sha256"`, or `"rsa-oaep-sha1"` for formats that require it
(MGF1 with the same hash, empty label). SHA-1 is accepted here only, for
interop: OAEP does not rely on its collision resistance.

## Errors

Every function checks its argument kinds and count, and raises a positioned
runtime error on misuse: a wrong type, an unknown algorithm name (the message
lists the known ones), a negative `randBytes` length, `lo > hi`, a
non-positive `iterations` / `keyLen` / `length`, a key, nonce or IV of the
wrong size, an IV for ECB, unaligned data for a `-nopad` mode, a bad padding,
a key of the wrong form or type for the algorithm, or a low-order X25519 peer
key. A failed `decrypt` or `unwrap` is one error whatever the cause.

The functions that draw randomness (`randBytes`, `randInt`, `keypair`, `sign`
with ECDSA, `encapsulate`, `wrap`, `mtweiKeygen`; and `uuid.v4` / `v7`, which
share the source) raise a catchable `secure random source unavailable` error
if the operating system's random source cannot be read. There is no weak
fallback: the call fails rather than return predictable bytes. The standard
`jennifer` reads the `getrandom` system call and does not hit this in practice;
`jennifer-tiny` reads `/dev/urandom`, which a minimal container or embedded
image may lack (see [TinyGo notes](../technical/tinygo.md)).
