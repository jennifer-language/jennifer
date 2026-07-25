# `sasl` - SASL authentication encoders

Import with `import "sasl.j" as sasl;`. The SASL authentication mechanisms
shared by the mail clients (`smtp` / `pop` / `imap`). These format the client
tokens; the protocol clients run the mechanism-specific wire dialogue around
them. The simple encoders are pure base64; the challenge-response mechanisms
(`cram`, SCRAM) key off the `hash` / `crypto` libraries. **No networking**, and
every dependency is TinyGo-clean, so the module runs on either binary.

```jennifer
import "sasl.j" as sasl;

def p as string init sasl.plain("me@example.com", "secret");     # SASL PLAIN
def b as string init sasl.bearer("me@gmail.com", accessToken);   # SASL XOAUTH2
```

Runnable: [`examples/modules/sasl_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/sasl_demo.j)
(and it is exercised end to end by the mail clients' demos, e.g.
[`smtp_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/smtp_demo.j)).

## Surface

| Call                        | Returns  | Notes                                                             |
| --------------------------- | -------- | ----------------------------------------------------------------- |
| `sasl.plain(user, pass)`    | `string` | SASL PLAIN: base64 of `"\0user\0pass"`.                           |
| `sasl.loginUser(user)`      | `string` | SASL LOGIN step 1: base64 of the username.                        |
| `sasl.loginPass(pass)`      | `string` | SASL LOGIN step 2: base64 of the password.                        |
| `sasl.bearer(user, token)`  | `string` | SASL XOAUTH2: an OAuth2 bearer-token response.                     |
| `sasl.cram(user, pass, challenge)` | `string` | CRAM-MD5 (RFC 2195): one-step HMAC-MD5 response to a base64 challenge. |
| `sasl.scramStart(user, algo)` | `Scram` | Begin a SCRAM exchange; `algo` `"sha1"` or `"sha256"`. Crypto-grade nonce. |
| `sasl.scramClientFirst(s)`  | `string` | The base64 client-first message to send.                          |
| `sasl.scramClientFinal(s, serverFirst, pass)` | `Scram` | Process server-first; derive the proof + expected server signature. |
| `sasl.scramFinalToken(s)`   | `string` | The base64 client-final message to send.                          |
| `sasl.scramVerify(s, serverFinal)` | `bool` | Verify the server's signature (constant-time); reject on `false`. |
| `sasl.negotiate(advertised)` | `string` | Pick the strongest password mechanism from a server's advertised list, as an `auth` token (see below). |

## Mechanism negotiation

`sasl.negotiate(advertised)` takes the mechanism names a server advertises (a
`list of string`, any case) and returns the strongest one this module supports
as a mail-client `auth` token: `"scram-sha-256"` > `"scram-sha-1"` > `"cram"`,
or `""` when the server offers none (the caller then uses its protocol default -
PLAIN / LOGIN / USER-PASS). XOAUTH2 is never auto-selected (it needs a bearer
token, not a password). The mail clients call this when `Options.auth` is
`"auto"`, feeding it the mechanisms parsed from the SMTP `EHLO`, POP3 `CAPA`, or
IMAP `CAPABILITY` advertisement (`""` keeps their plain default).

## SCRAM (RFC 5802 / RFC 7677)

SCRAM is a salted challenge-response: the password never crosses the wire, and
both sides prove knowledge of it. The exchange threads a value-semantic `Scram`
handle through four calls (each returns an updated copy), so a client drives it
against the SASL `AUTHENTICATE` / `AUTH` loop:

```jennifer
def s as sasl.Scram init sasl.scramStart("user", "sha256");
# 1. send sasl.scramClientFirst($s); read the server-first reply
$s = sasl.scramClientFinal($s, serverFirst, "pencil");
# 2. send sasl.scramFinalToken($s); read the server-final reply
if (not sasl.scramVerify($s, serverFinal)) {
    # the server does not know the password - abort, do not trust the session
}
```

`scramClientFinal` runs PBKDF2 in the mechanism's hash (`crypto.pbkdf2` with
`"sha1"` / `"sha256"`), so SCRAM-SHA-1 (MongoDB, XMPP) and SCRAM-SHA-256
(PostgreSQL, modern mail) both work. `scramVerify` compares the server
signature with `crypto.hmacEqual` (constant-time). The wire strings are the
base64 SASL tokens; the raw SCRAM messages are inside.

`sasl.cram(user, pass, challenge)` is the simpler one-step CRAM-MD5: the server
sends a base64 challenge, the client answers `base64("user " + hex(HMAC-MD5))`.
It is named `cram`, not `cramMd5`, for the letters-only method-name rule.

## XOAUTH2 (the "use a token" half of OAuth2)

`sasl.bearer(user, token)` builds the SASL **XOAUTH2** initial response -
`base64("user=" user 0x01 "auth=Bearer " token 0x01 0x01)` - which is how
**Google** and **Microsoft 365** authenticate mail now that both have retired
password auth. The mail clients accept it via `Options.auth = "xoauth2"` (with
the access token in `pass`):

```jennifer
def opts as smtp.Options init smtp.Options{host: "smtp.gmail.com", port: 587,
    security: "starttls", clientName: "me.example", user: "me@gmail.com",
    pass: accessToken, auth: "xoauth2"};
smtp.send($opts, "me@gmail.com", ["you@example.com"], $message);
```

The function is named `bearer`, not `xoauth2`, because a Jennifer method name
is letters-only (no digit); the wire mechanism name `"XOAUTH2"` is a string the
client sends. Getting the **token itself** is the other half of OAuth2 - the
job of the generic `oauth` client (planned, M18.7.3).

## Out of scope

- **Encoders, not transport.** `sasl` formats tokens and drives the SCRAM state
  machine; the SMTP `AUTH`, IMAP `AUTHENTICATE`, and POP3 `AUTH` line dialogue
  lives in the respective clients.
- **No SASLprep.** Usernames / passwords are used as given (only SCRAM's `=` /
  `,` username escaping is applied); normalize non-ASCII credentials yourself.

## See also

- [smtp.md](smtp.md) / [pop.md](pop.md) / [imap.md](imap.md) - the mail
  clients that consume these encoders.
- [crypto.md](../libraries/crypto.md) - the KDF / constant-time compare SCRAM
  builds on; [encoding.md](../libraries/encoding.md) - the base64 codec.
- [modules/index.md](index.md) - the module catalog and import rules.
