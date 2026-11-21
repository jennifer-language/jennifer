# Conventions

The rules behind [stance 8](../design-stances.md), "one convention
everywhere": the same situation looks and behaves the same in every built-in
library, bundled module and `@jennifer/*` deck. Decks of other scopes are
outside the project; for them the catalogue is a recommendation, and a deck on
the shared CI layer gets its checks with the next sync. A reader who has learnt one
library should be able to guess the next one; a reader who finds two libraries
doing the same thing differently has found a bug.

Each rule says how it is checked. "Gate" means a check that fails CI today;
"survey" means the conventions survey (roadmap M29.23), which lists every
deviation and is turning the rest into checks. A deliberate exception is
recorded in [design-decisions.md](design-decisions.md) and named here.

## Names

| Rule | Example | Checked by |
|---|---|---|
| Functions and parameters are `camelCase`, constants `UPPER_SNAKE`, types `CamelCase`; no other underscores. | `readSecretBytes`, `MAX_RETRIES`, `HttpError` | the parser |
| No name spelled like a keyword, even where `alias.name` would parse. Eleven core names are recorded exceptions (`lists.map`, `json.map`, `fs.list`, ...; see [design-decisions](design-decisions.md#core-names-spelled-like-a-keyword)). | `bitAnd`, `listObjects`, not `and`, `list` | survey |
| A predicate starts with `is` or `has` and returns `bool`; nothing else does. | `fs.isFile`, `maps.has` | survey |
| A variant that takes more control is the base name plus `With`, taking one options struct; the base form is exactly `xWith` with zero options. | `connect` / `connectWith(addr, Options)` | survey |
| A variant that returns or takes `bytes` instead of `string` is the base name plus `Bytes` (or a `String` / `Bytes` pair). | `fetch` / `fetchBytes`, `readString` / `readBytes` | review |
| A quantity carries its unit in its name: durations in milliseconds as `...Ms`; seconds as `...Sec` only where the protocol counts seconds; sizes as `...Bytes`. Never a bare `timeout`. A value typed `time.Duration` carries its unit in the type. | `timeoutMs`, `ttlSec`, `maxBytes` | survey |
| An option that weakens security says so: `allowInsecure...` for a flag, `skipVerify` for TLS certificate checks, and nothing shorter. | `allowInsecureAuth`, `allowInsecureHttp`, `TLSOptions.skipVerify` | survey |

## Arguments and values

| Rule | Example | Checked by |
|---|---|---|
| Argument order: a handle first, then an algorithm / format / codec name, then the subject, then other parameters, optional ones last. | `crypto.encrypt(algo, plaintext, key, nonce, aad)`, `fs.readBytes($f, n)` | survey |
| An algorithm, format or codec is a string argument, not a separate function per choice; an unknown name is an error listing the known ones. | `hash.compute("sha256", b)` | survey |
| Builders are value-semantic: they return the updated value, which the caller threads. | `$f = feed.add($f, $e);` | the language |
| The zero value of an options struct is the safe default. `0` may mean "the default" only where the docs say so; a negative count or duration throws. | `def o as totp.Options;` | survey |
| Text that comes from outside is returned as `string` only after UTF-8 validation (the error names the byte offset); binary data has a `bytes` form. | `fs.readString`, `os.runBytes` | survey |

## Errors

The kinds are defined in the control-flow guide's
["Error kinds"](../user-guide/control-flow.md#error-kinds); these are the rules
for raising them.

| Rule | Example | Checked by |
|---|---|---|
| Bad input data (decoding, parsing, decompressing, decrypting, verifying) throws the library's or module's own kind (`mcp` and `pop` are [recorded exceptions](design-decisions.md#two-error-kinds-outside-a-modules-own)). Misuse (wrong type, unknown algorithm name, wrong key length) is `"runtime"` in the built-in libraries, the module's kind elsewhere. | `"json"`, `"crypto"`, `"totp"` | survey; built-ins pending M29.21 |
| A situation every library shares has its generic kind: a reached cap `"limit"`, a deadline `"timeout"`, a closed handle or peer `"closed"`, a cancelled task `"cancelled"`, a network failure `"net"` (`net.NetError`). | `fs.readSecretBytes` over 1 MiB is `"limit"` | per-library tests |
| An error that carries a server's status or codes is one exported struct for every throw of its kind (`kind` + `message` + fields), so its fields are safe to read once the kind matches. | `rest.HttpError`, `smtp.SmtpError` | lint L104; survey |
| A wrapper that rewraps what it caught passes the generic kinds on unchanged (`throw $e;`). | | survey |
| A message is for people: it names its source once (`lib.fn:` in the built-ins, `module:` or `module.fn:` in modules and decks, never twice as in `statfiles: statfiles: ...`), the path where there is one, what was wrong and, where there is one, the fix. No caller ever matches on it. | `fs.readSecretBytes: key has group or other permissions (mode 0644); run chmod 600 key` | survey |
| A kind, once released, keeps its name; changing a caller-visible kind is a breaking change. | | review |

## Limits and security defaults

| Rule | Example | Checked by |
|---|---|---|
| Every reader of outside data has a default cap on size, depth or count, which the caller can lower; reaching it is `"limit"`. | parser nesting caps, `net.readAll` | survey |
| Every network call has a default deadline, which the caller can change (`timeoutMs`). | | survey |
| Credentials need TLS unless the caller opts out with `allowInsecureAuth` / `allowInsecureHttp`. | `smtp`, `imap`, `webdav` | survey |
| Secrets are compared with `hash.equal` (constant time), drawn from `crypto` randomness, and read from files with `fs.readSecretBytes`. | | survey |
| Nothing malformed is guessed at: no lenient mode, no silent default, no clamping of a bad option. | | review |

## Documentation

| Rule | Checked by |
|---|---|
| Every export has a docblock; every built-in function has its signature in `internal/libsig` and an entry in its `docs/libraries` page. | lint, `jvc publish`, the deck gate |
| Every page names the error kinds its functions throw, and its caps. | survey |
| A deck's exports are listed in its cheatsheet, and the calls in its docs are checked against the code. | the deck gate (`tools/check-docs.sh`) |

## Known deviations

The survey writes them to [conventions-findings.md](conventions-findings.md),
one line per deviation, so the list shrinking is visible in its history. From
the repository root of a workspace with the decks checked out beside it:

```sh
jennifer run scripts/conventions-survey.j ../decks/deck-discord ... \
    > docs/technical/conventions-findings.md
```

It names every `@jennifer/*` deck surveyed. Not yet surveyed mechanically:
argument order, the kind of each data error (pending the per-library kind
tests of M29.21), caps without `"limit"`, and the documentation rules.
