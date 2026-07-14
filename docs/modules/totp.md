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

Runnable: [`examples/modules/totp_demo.j`](https://github.com/mplx/jennifer-lang/blob/main/examples/modules/totp_demo.j).

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
| `totp.generate(secret, opts)` | `string` | The code for the current time. |
| `totp.generateAt(secret, unixSeconds, opts)` | `string` | The code for an explicit Unix time. Deterministic - use it in tests. |
| `totp.verify(secret, code, opts)` | `bool` | True if `code` is valid for the current time. |
| `totp.verifyAt(secret, code, unixSeconds, opts)` | `bool` | True if `code` is valid for an explicit Unix time. Deterministic. |
| `totp.uri(issuer, account, secret, opts)` | `string` | The `otpauth://totp/...` provisioning URI (what a QR code encodes). |

`verify` / `verifyAt` accept a **+/-1-step skew window**: a code from the
immediately previous or next time step still passes, so a small clock drift
between the two sides does not reject a legitimate code. A code two or more
steps away is rejected.

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

- The secret is the shared key: store it like a password, and transmit the
  provisioning URI over a secure channel only.
- Randomness for a *new* secret is out of scope here - generate one from a
  cryptographic source and base32-encode it with `encoding.toText(bytes,
  "base32")`. (`math.rand*` is **not** crypto-grade; that lands with the future
  `crypto` library.)
- SHA-1 is the default because authenticator apps default to it; it is a safe
  choice for HMAC despite being broken for collision resistance.

## See also

- [hash.md](../libraries/hash.md) - the `hmac` primitive TOTP is built on.
- [encoding.md](../libraries/encoding.md) - base32 for secrets.
- [time.md](../libraries/time.md) - the clock the step counter uses.
- [modules/index.md](index.md) - the module catalog and import rules.
