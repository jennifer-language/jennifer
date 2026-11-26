# Conventions

The rules behind [stance 8](../design-stances.md), "one convention
everywhere": the same situation looks and behaves the same in every built-in
library and bundled module. Decks are outside the project (the `@jennifer`
decks moved to `@deckyard` in M29.55); for them the catalogue is a recommendation, and a deck on
the shared CI layer gets its checks with the next sync. A reader who has learnt one
library should be able to guess the next one; a reader who finds two libraries
doing the same thing differently has found a bug.

Each rule says how it is checked. "Gate" means a check that fails CI today;
"survey" means the conventions survey (roadmap M30.28, was M29.23), which
lists every deviation and is turning the rest into checks. A deliberate exception is
recorded in [design-decisions.md](design-decisions.md) and named here.

## Names

| Rule | Example | Checked by |
|---|---|---|
| Functions and parameters are `camelCase`, constants `UPPER_SNAKE`, types `CamelCase`; no other underscores. | `readSecretBytes`, `MAX_RETRIES`, `HttpError` | the parser |
| No name spelled like a keyword, even where `alias.name` would parse. Eleven core names are recorded exceptions (`lists.map`, `json.map`, `fs.list`, ...; see [design-decisions](design-decisions.md#core-names-spelled-like-a-keyword)). | `bitAnd`, `listObjects`, not `and`, `list` | survey |
| A predicate starts with `is` or `has` and returns `bool`; nothing else does. | `fs.isFile`, `maps.has` | survey |
| A variant that takes more control is the base name plus `With`, taking one options struct; the base form is exactly `xWith` with zero options. Tuning parameters go in that struct, not in trailing optional arguments. | `connect` / `connectWith(addr, Options)` | survey |
| A variant that returns or takes `bytes` instead of `string` is the base name plus `Bytes` (or a `String` / `Bytes` pair). | `fetch` / `fetchBytes`, `readString` / `readBytes` | review |
| A quantity carries its unit in its name: durations in milliseconds as `...Ms`; seconds as `...Sec` only where the protocol counts seconds; sizes as `...Bytes`. Never a bare `timeout`. A value typed `time.Duration` carries its unit in the type. | `timeoutMs`, `ttlSec`, `maxBytes` | survey |
| An option that weakens security says so: `allowInsecure...` for a flag, `skipVerify` for TLS certificate checks, and nothing shorter. | `allowInsecureAuth`, `allowInsecureHttp`, `TLSOptions.skipVerify` | survey |

## Arguments and values

| Rule | Example | Checked by |
|---|---|---|
| Argument order: a handle first, then an algorithm / format / codec name, then the subject, then other parameters, optional ones last. | `crypto.encrypt(algo, plaintext, key, nonce, aad)`, `fs.readBytes($f, n)` | survey |
| An algorithm, format or codec is a string argument, not a separate function per choice; an unknown name is an error listing the known ones, a weak one (offered for interop only, never as a default) tagged `(weak)` ([design decision](design-decisions.md#weak-algorithms-by-name-never-by-default)). | `hash.compute("sha256", b)` | survey |
| Builders are value-semantic: they return the updated value, which the caller threads. | `$f = feed.add($f, $e);` | the language |
| A result that holds no operating-system resource is an immutable value the garbage collector frees, not a handle into a registry with a `free`. | `json.Value`, `ml.Model` | review |
| The zero value of an options struct is the safe default. `0` may mean "the default" only where the docs say so; a negative count or duration throws. | `def o as totp.Options;` | survey |
| A wait has one of three shapes: an options field (`timeoutMs`) on a call that takes options, `xTimeout(ms)` for a blocking primitive, stream settings on a stream (a deadline, `setReadDeadline`, bounds the whole exchange; an idle timeout, `setIdleTimeout`, bounds a stall; with both, the first to pass fires). An options field's `0` is its default and `math.MAX_INT` none; a stream setting's `0` is none. An `xTimeout`'s `ms` is the wait itself: `0` checks once (polls), and on `"timeout"` the work keeps running. A negative or oversized value throws `"runtime"` in the built-in libraries, the module's kind in modules. `time.sleep` takes a `time.Duration` (a length, not a bound): `0` returns at once, negative throws. | `net.connectWith(a, net.ConnectOptions{timeoutMs: 5000})`, `sql.openWith(d, dsn, sql.Options{queryTimeoutMs: 5000})`, `channel.recvTimeout`, `channel.selectTimeout`, `task.waitAllTimeout`, `os.waitTimeout`, `net.setReadDeadline`, `net.setIdleTimeout` | review |
| Decimal rounding is half to even on the shortest decimal that reads back as the value, everywhere a number is rounded for output. | `%f\|prec`, `intl.formatNumber`, `decimal.round`'s default | per-library tests |
| Text that comes from outside is returned as `string` only after UTF-8 validation (the error names the byte offset); binary data has a `bytes` form. | `fs.readString`, `os.runBytes`, `os.getEnv` / `getEnvBytes`, `os.ARGS` / `argsBytes()` | survey |
| A search that finds nothing returns a "not found" value where its result type has one, and throws where it cannot (an element of any type has none to spare). | `lists.indexOf` -1, `regex.find`'s `Match` with `start` -1; `lists.find` throws | review |
| A decoder accepts only canonical text where the encoding has spare bits, which would give one value two spellings (base32 / base64 padding bits, line breaks). Hex accepts either case: case carries no bits, and hex from other tools is often uppercase (MAC addresses, digests, RFC 4648's base16 alphabet). Encoders write lowercase hex. | `fromText("base64", ...)` refuses non-canonical text; `fromText("hex", "DEADBEEF")` | per-library tests |

## Errors

The kinds are defined in the control-flow guide's
["Error kinds"](../user-guide/control-flow.md#error-kinds); these are the rules
for raising them.

| Rule | Example | Checked by |
|---|---|---|
| Bad input data (decoding, parsing, decompressing, decrypting, verifying) throws the library's or module's own kind (`mcp`, `pop` and `s3presign` are [recorded exceptions](design-decisions.md#error-kinds-outside-a-modules-own)). Misuse (wrong type, unknown algorithm name, wrong key length) is `"runtime"` in the built-in libraries, the module's kind elsewhere. | `"json"`, `"crypto"`, `"totp"` | survey; built-ins: each library's data kind in `internal/libsig` and its kind test |
| A situation every library shares has its generic kind: a reached cap `"limit"`, a deadline `"timeout"`, a closed handle or peer `"closed"`, a cancelled task `"cancelled"`, a network failure `"net"` (`net.NetError`). | `fs.readSecretBytes` over 1 MiB is `"limit"` | per-library tests |
| An error that carries a server's status or codes is one exported struct for every throw of its kind (`kind` + `message` + fields), so its fields are safe to read once the kind matches. | `rest.HttpError`, `smtp.SmtpError`, `sql.SqlError` | lint L104; survey |
| A wrapper that rewraps what it caught passes the generic kinds on unchanged (`throw $e;`). | | survey |
| A message is for people: it names its source once (`lib.fn:` in the built-ins, `module:` or `module.fn:` in modules and decks, never twice as in `statfiles: statfiles: ...`), the path where there is one, what was wrong and, where there is one, the fix. No caller ever matches on it. | `fs.readSecretBytes: key has group or other permissions (mode 0644); run chmod 600 key` | survey |
| A handle once closed (or consumed: a finalized stream, an answered request, a shut-down server) throws `"closed"` on any use but `close`, and a second `close` does nothing, so a `defer` and an explicit close may both run. | `fs.close($f); fs.close($f);` is fine; `fs.readLine($f)` after it throws `"closed"` | per-library tests |
| A kind, once released, keeps its name; changing a caller-visible kind is a breaking change. | | review |

## Limits and security defaults

| Rule | Example | Checked by |
|---|---|---|
| Every reader of outside data has a default cap on size, depth or count, which the caller can lower; reaching it is `"limit"`. | parser nesting caps, `net.readAll` | survey |
| A caller-set cap is a field of an options struct taken by an `xWith` variant, never a positional argument: `0` is the default, a negative value throws (`"runtime"` in the built-in libraries, the module's kind in modules), `math.MAX_INT` is unlimited. A decoder's depth / node caps only lower the default, and a value above it throws `"runtime"`; `ml`'s hyper-parameters are cost caps, and a value past the ceiling is `"limit"`. | `compress.unpackWith(f, b, compress.UnpackOptions{maxBytes: math.MAX_INT})`, `toml.decodeWith(s, toml.DecodeOptions{maxDepth: 32})` | review |
| Every network call has a default deadline, which the caller can change (`timeoutMs`). | | survey |
| Credentials need TLS unless the caller opts out with `allowInsecureAuth` / `allowInsecureHttp` / `allowInsecure` (`sql.Options`); a loopback host or Unix socket is exempt. | `smtp`, `imap`, `webdav`, `sql` | survey |
| Secrets are compared with `hash.equal` (constant time), drawn from `crypto` randomness, and read from files with `fs.readSecretBytes`. | | survey |
| Nothing malformed is guessed at: no lenient mode, no silent default, no clamping of a bad option. | | review |
| A file-backed store refuses a lost update: a write checks the file is the one it last read or wrote and throws rather than overwrite another writer's change. | `flatdb.save`, `kv.openFile`'s flush | per-library tests |
| A per-task allocation budget is opt-in and set on the task by a positional setter (`task.setAllocBudget(bytes)`, `web.allocBudget($app, bytes)`), not an options field: > 0 sets, `math.MAX_INT` lifts, 0 / negative throws the library's kind. An overrun is `"limit"`; `web` answers it 500, unlike a cap's 413, because the server declines rather than the request being too large. | `task`, `web` | `allocbudget_test.go`, `web_allocbudget_test.go` |

## Documentation

| Rule | Checked by |
|---|---|
| Every export has a docblock; every built-in function has its signature in `internal/libsig` and an entry in its `docs/libraries` page. | lint, `jvc publish`, the deck gate |
| Every page names the error kinds its functions throw, and its caps. | survey |
| A deck's exports are listed in its cheatsheet, and the calls in its docs are checked against the code. | the deck gate (`tools/check-docs.sh`) |
| A deprecated library function is marked in `internal/libsig` (`deprecate`), a deprecated module or deck export by `@deprecated <what to use>; removed in X` in its docblock; lint reports each call (L301) with that text, and the release after removes it (L302). | lint L301 / L302 |

## Known deviations

The survey writes them to [conventions-findings.md](conventions-findings.md),
one line per deviation, so the list shrinking is visible in its history. From
the repository root:

```sh
jennifer run scripts/conventions-survey.j > docs/technical/conventions-findings.md
```

Not yet surveyed mechanically: argument order, the kind of each data error in
modules (the built-ins' are tested since M29.21), caps without `"limit"`, an
`xWith` that takes something other than one options struct, and the
documentation rules.
