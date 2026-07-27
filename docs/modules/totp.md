# `totp` - time-based one-time passwords

Import with `import "totp.j" as totp;`. Generate and verify **TOTP** codes
([RFC 6238](https://www.rfc-editor.org/rfc/rfc6238) over
[RFC 4226](https://www.rfc-editor.org/rfc/rfc4226) HOTP) - the six-digit
two-factor codes an authenticator app shows. Both sides share a **secret** (a
base32 string) and, from the current time, compute the same short numeric code
independently. Pure `.j`; runs on **both** binaries.

Built on `hash.hmac` (HMAC-SHA1 by default; SHA-256 / SHA-512 optional),
`encoding` (base32 secrets), and `time` (the 30-second step); the
dynamic-truncation step uses `bytes` + bitwise operators.

```jennifer
import "totp.j" as totp;

def o as totp.Options;                             # zero-value: 6 digits, 30 s, SHA-1
def code as string init totp.generate("JBSWY3DPEHPK3PXP", $o);
def ok as bool init totp.verify("JBSWY3DPEHPK3PXP", $code, $o);
```

Runnable: [`examples/modules/totp_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/totp_demo.j).

## Options

A `totp.Options` carries the parameters; a zero-value struct
(`def o as totp.Options;`) means the common authenticator defaults.

| Field | Effect |
| ----- | ------ |
| `digits` (int) | Code length; `0` means 6. |
| `period` (int) | Time step in seconds; `0` means 30. |
| `algorithm` (string) | HMAC digest: `"sha1"` (default), `"sha256"`, or `"sha512"`; `""` means `"sha1"`. |

The `secret` is a **base32** string - the same value an authenticator app
stores. Spaces are ignored, letters are upper-cased, and missing `=` padding is
supplied, so an app's grouped, unpadded secret (`JBSW Y3DP EHPK 3PXP`) decodes
fine.

## Functions

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `totp.generateSecret()` | `string` | A fresh, crypto-grade random secret at the RFC 6238-recommended 20-byte / 160-bit length, as an unpadded base32 string. |
| `totp.generateSecretN(nbytes)` | `string` | Same, with an explicit byte length (`nbytes` must be positive). |
| `totp.hotp(secret, counter)` | `string` | The raw RFC 4226 HOTP: the six-digit HMAC-SHA1 code for an explicit counter (the building block TOTP layers a clock over). |
| `totp.generate(secret, opts)` | `string` | The code for the current time. |
| `totp.generateAt(secret, unixSeconds, opts)` | `string` | The code for an explicit Unix time. Deterministic - use it in tests. |
| `totp.verify(secret, code, opts)` | `bool` | True if `code` is valid for the current time (+/-1-step skew). |
| `totp.verifyAt(secret, code, unixSeconds, opts)` | `bool` | True if `code` is valid for an explicit Unix time (+/-1-step skew). Deterministic. |
| `totp.verifyWindow(secret, code, window, opts)` | `bool` | Like `verify`, but accepts a `window`-step skew on each side of now. |
| `totp.verifyWindowAt(secret, code, unixSeconds, window, opts)` | `bool` | Like `verifyAt`, with an explicit `window`. Deterministic. |
| `totp.uri(issuer, account, secret, opts)` | `string` | The `otpauth://totp/...` provisioning URI (what a QR code encodes). |

`verify` / `verifyAt` accept a **+/-1-step skew window**: a code from the
immediately previous or next time step still passes, so a small clock drift
between the two sides does not reject a legitimate code. A code two or more
steps away is rejected.

Use `verifyWindow` / `verifyWindowAt` to set the tolerance explicitly: `window`
is the number of period-steps accepted on each side of now, so `window` 0 checks
only the current step and `window` 2 tolerates up to a minute of drift (at the
default 30-second period). Widen it only as far as the drift you actually need -
every extra step is another valid code an attacker may guess.

### Generating a secret

`totp.generateSecret()` draws 20 bytes (the RFC 6238-recommended length for
SHA-1 TOTP) from a cryptographic source (`crypto.randBytes`) and returns them as
an unpadded base32 string - the format an authenticator app stores. It
round-trips: the result works directly with `generate` / `verify`. Use
`generateSecretN(nbytes)` for a different key length.

```jennifer
def secret as string init totp.generateSecret();   # e.g. "JBSWY3DPEHPK3PXP..."
def code as string init totp.generate($secret, $o); # both sides now agree
```

### HOTP

`totp.hotp(secret, counter)` is the counter-based RFC 4226 one-time password
TOTP is built from (six digits, HMAC-SHA1). Use it directly for event-based
HOTP, or to pin against the RFC 4226 Appendix D test vectors; TOTP is exactly
this over `unixSeconds // period`.

`generate` / `verify` read the host clock via `time`; `generateAt` / `verifyAt`
take the time as an argument, which is what makes them deterministic (and what
the RFC 6238 Appendix B test vectors pin the module against).

## Provisioning URI

`totp.uri` builds the string an authenticator app enrols by scanning a QR code.
The label is `issuer:account`, and the issuer / account are percent-encoded:

```jennifer
totp.uri("ACME Corp", "jane@acme.example", "JBSWY3DPEHPK3PXP", $o)
# otpauth://totp/ACME%20Corp:jane%40acme.example?secret=JBSWY3DPEHPK3PXP&issuer=ACME%20Corp&algorithm=SHA1&digits=6&period=30
```

Render that URI as a QR code (any QR generator) and the app is enrolled; the app
and `totp.verify` then agree on the code for each 30-second window.

## Security notes

> **The caller is the verifier, and RFC 6238 section 5.2 puts two duties on the
> verifier that this module does *not* perform for you.** This module only
> *computes and compares* codes; it keeps no state across calls. You MUST, in
> your own code:
>
> 1. **Prevent replay (single-use).** A code stays valid for the whole
>    validity window (a step, plus the skew window). Record each code that
>    successfully authenticates and reject a second use of the *same* code
>    within that window - otherwise an attacker who observes one code can reuse
>    it until it expires.
> 2. **Rate-limit verification attempts.** Cap the number of failed
>    verifications per secret (and lock out or back off after a threshold) so an
>    attacker cannot brute-force the small numeric code space. `verify` is
>    already constant-time per attempt, but that does not bound *how many*
>    attempts an attacker gets - only you can.
>
> Both are stateful policy that belongs to your application (a store of used
> codes, an attempt counter); the module has no place to keep it and never sees
> your request flow.

- The secret is the shared key: store it like a password, and transmit the
  provisioning URI over a secure channel only.
- Generate a *new* secret with `totp.generateSecret()` (crypto-grade random,
  RFC 6238-recommended 20 bytes, base32-encoded) rather than rolling your own -
  never use `math.rand*`, which is not crypto-grade.
- Keep the skew window as narrow as the real clock drift requires: each extra
  step (`verifyWindow`) is another simultaneously-valid code.
- SHA-1 is the default because authenticator apps default to it; it is a safe
  choice for HMAC despite being broken for collision resistance.

## See also

- [hash.md](../libraries/hash.md) - the `hmac` primitive TOTP is built on.
- [encoding.md](../libraries/encoding.md) - base32 for secrets.
- [time.md](../libraries/time.md) - the clock the step counter uses.
- [modules/index.md](index.md) - the module catalog and import rules.
