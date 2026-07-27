# `webhook` - HMAC-signed webhooks

Import with `import "webhook.j" as webhook;`. Sign and verify webhook deliveries
the GitHub / Stripe way - the **`X-Hub-Signature-256`** convention. A sender
computes `sha256=<hex>`, the hex HMAC-SHA256 of the exact request body keyed by a
shared secret, and puts it in a header; the receiver recomputes it over the body
it got and compares, confirming the delivery is authentic and untampered.

`sign` / `verify` are pure and run on **both** binaries; `send` POSTs a signed
payload and needs the default **`jennifer`** binary (`net` via `http`).

```jennifer
import "webhook.j" as webhook;

def sig as string init webhook.sign("{\"event\":\"push\"}", "topsecret");
# -> "sha256=..."  (put this in the X-Hub-Signature-256 header)

def ok as bool init webhook.verify("{\"event\":\"push\"}", $sig, "topsecret");
```

Runnable: [`examples/modules/webhook_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/webhook_demo.j).

## Functions

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `webhook.sign(payload, secret)` | `string` | `sha256=` + hex HMAC-SHA256 of `payload` keyed by `secret`. |
| `webhook.verify(payload, signature, secret)` | `bool` | True if `signature` matches (constant-time compare). |
| `webhook.send(url, payload, secret)` | `http.Response` | POST `payload` to `url` with the signature header set. Needs the default binary. |

The signature covers the **raw body bytes** - sign and verify the exact string
you send / receive, before any parsing. A receiver that re-serializes the body
first can compute a different signature and reject a valid delivery.

`verify` uses a constant-time comparison, so a check does not leak via timing
how many leading characters of the signature were correct. It returns `false`
(never throws) for a wrong secret, a tampered payload, or a malformed signature.

## Timestamped, replay-protected schemes

The GitHub-style `sign` / `verify` above authenticate the body but carry no
timestamp, so a captured request can be replayed forever. The schemes below
fold a **unix timestamp** into the signed data and reject a delivery whose
timestamp drifts more than `toleranceSeconds` from `now` - the timestamp
tolerance **is** the replay defence. All of them constant-time compare, run on
**both** binaries, and are pure: pass `now` and `timestamp` as explicit `int`
unix seconds (a script reads the clock from `time` and passes it in), which
keeps the functions testable and deterministic.

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `webhook.stripeSign(secret, body, timestamp)` | `string` | Stripe `t=<ts>,v1=<hexsig>` header; signs `<ts>.<body>` HMAC-SHA256, hex. |
| `webhook.stripeVerify(secret, body, header, toleranceSeconds, now)` | `bool` | Parse `t` + the `v1=` sig(s), recompute, constant-time compare; true only if a sig matches **and** `\|now - t\| <= tolerance`. |
| `webhook.slackSign(secret, body, timestamp)` | `string` | Slack `v0=<hexsig>`; signs `"v0:" + <ts> + ":" + <body>` HMAC-SHA256, hex. |
| `webhook.slackVerify(secret, body, timestamp, signature, toleranceSeconds, now)` | `bool` | Recompute over the `X-Slack-Request-Timestamp` value; true only if it matches **and** the timestamp is fresh (e.g. 300s). |
| `webhook.timestampedSign(secret, body, timestamp, algo, encoding)` | `string` | Generic: sign `<ts>.<body>` with `algo` (`"sha1"`/`"sha256"`) and `encoding` (`"hex"`/`"base64"`). |
| `webhook.timestampedVerify(secret, body, timestamp, signature, algo, encoding, toleranceSeconds, now)` | `bool` | Constant-time compare + freshness check for the generic scheme. |

```jennifer
import "webhook.j" as webhook;
use time;

def body as string init "{\"event\":\"payment\"}";
def secret as string init "whsec_...";
def now as int init time.unix(time.now());

# Sign (sender side): stamp with the current time.
def header as string init webhook.stripeSign($secret, $body, $now);
# -> "t=...,v1=..."  (put this in the Stripe-Signature header)

# Verify (receiver side): recompute, constant-time compare, reject stale.
def ok as bool init webhook.stripeVerify($secret, $body, $header, 300, $now);
```

**Stripe** (`t=,v1=`): the signed payload is `<timestamp> + "." + <body>`,
HMAC-SHA256, hex. A header may carry several `v1=` values (a signing-secret
rotation); `stripeVerify` accepts the delivery if **any** matches and the `t`
value is fresh. A missing `t=` or `v0=`/`v1=` part, or a non-numeric timestamp,
yields `false` rather than throwing.

**Slack** (`v0=`): the base string is `"v0:" + <timestamp> + ":" + <body>`,
HMAC-SHA256, hex; the timestamp travels separately in
`X-Slack-Request-Timestamp` (parse it to an `int` and pass it in). Slack's own
guidance is a 5-minute (`300`) tolerance.

**Generic** (`timestampedSign` / `timestampedVerify`): the small composable core
- sign `<timestamp> + "." + <body>` with any supported digest and encoding, so
a GitHub-style `sha1`/`sha256` scheme or a base64 scheme is one call. Hex is
case-folded on the built-in schemes; base64 is compared exactly.

Every verify does a **constant-time** compare (via `crypto.hmacEqual`), so no
scheme leaks through timing how many leading bytes of the signature matched, and
the freshness window is checked independently so a replayed-but-correctly-signed
request is still rejected.

## Sending

`webhook.send` posts the payload as `application/json` with the
`X-Hub-Signature-256` header, and returns the receiver's `http.Response`
(status / headers / body). Reading the result needs `import "http.j"` for the
type:

```jennifer
import "webhook.j" as webhook;
import "http.j" as http;

def r as http.Response init webhook.send("https://example.com/hook",
    "{\"event\":\"push\"}", "topsecret");
io.printf("delivered: %d\n", $r.status);
```

A non-2xx status comes back as a value to branch on; a network failure throws a
positioned `http` / `net` error (wrap in `try` / `catch`).

## Notes and scope

- **SHA-256, `X-Hub-Signature-256`.** This is the modern GitHub convention. The
  legacy `X-Hub-Signature` (SHA-1) header is not emitted; sign with SHA-256.
- **The secret is the shared key** - store it like a password and distribute it
  over a secure channel. Randomness for a new secret is out of scope (use a
  cryptographic source; `math.rand*` is not crypto-grade).
- **Content type is `application/json`** for `send`. For a different body type,
  `sign` the payload yourself and post it with your own headers via `http`.

## See also

- [hash.md](../libraries/hash.md) - the `hmac` primitive the signature is built on.
- [http.md](http.md) - the client `send` posts through.
- [totp.md](totp.md) - the other `hash.hmac`-based module (2FA codes).
- [modules/index.md](index.md) - the module catalog and import rules.
