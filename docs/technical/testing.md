# Testing

Ordered by the pipeline, then libraries, then dev tooling and the CLI.

| Package                | What it tests                                                                                                                                                                                                          |
| ---------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `internal/lexer`       | Token-by-token output for fixed inputs; trivia (comments, blank lines); error cases                                                                                                                                    |
| `internal/preproc`     | `include` file splicing and path resolution; circular-include detection; trivia handling                                                                                                                               |
| `internal/parser`      | AST shape via `Sprint`; operator precedence; the resolver's scope / slot pass with shadowing + undefined-variable errors; constant folding; parse error cases                                                          |
| `internal/interpreter` | Full programs in-memory with stdout captured; value-semantics aliasing (eager deep copies) under `-race`; `CallByName` / `CallByNameWith` dispatch and `RaiseError` classification                                      |
| `internal/lib/convert` | `toInt` / `toFloat` / `toString` / `toBool` (canonical-only, radix forms); `typeOf` / `objectType`; `bytesFromString` / `stringFromBytes`                                                                              |
| `internal/lib/crc`     | CRC-32 / CRC-64 checksums over `bytes`; codec-table lookups and aliases                                                                                                                                                |
| `internal/lib/encoding`| `toText` / `fromText` (hex, base64); charset `encode` / `decode`; `isAscii` and length introspection                                                                                                                   |
| `internal/lib/fs`      | Whole-file read / write / append; metadata (`stat`); directory ops; buffered `File` handles, against temp dirs                                                                                                         |
| `internal/lib/hash`    | MD5 / SHA-1 / SHA-256 one-shot `compute` and streaming `update` / `finalize`                                                                                                                                           |
| `internal/lib/io`      | `Install` registers `printf` / `sprintf`; format verbs and the modifier grammar; arity and format errors                                                                                                               |
| `internal/lib/lists`   | `push` / `pop` / `sort` / `reverse` / `slice` / `concat` / `range`; non-mutating (value) semantics                                                                                                                     |
| `internal/lib/maps`    | `keys` / `values` / `has` / `delete` / `merge`; insertion order; missing-key errors                                                                                                                                    |
| `internal/lib/math`    | `min` / `max` / `sqrt` / `pow`; seeded `rand` / `randInt` determinism; strict errors for unrepresentable or undefined results; special functions                                                                       |
| `internal/lib/meta`    | `meta.VERSION` matches `version.Version`; `meta.BUILD` matches the compiler tag                                                                                                                                        |
| `internal/lib/net`     | TCP / UDP loopback round-trips and DNS lookups (build-tag gated; the TinyGo stub returns friendly errors)                                                                                                              |
| `internal/lib/os`      | env / args / flag helpers; external-process `run` / `spawn` / `wait` / `poll` / `kill`                                                                                                                                 |
| `internal/lib/regex`   | RE2 `matches` / `find` / `findAll` / `replace` / `split`; positional + named captures; the pattern cache                                                                                                               |
| `internal/lib/strings` | `upper` / `lower`, `contains` / `startsWith` / `endsWith`, rune-indexed `indexOf`; case folding                                                                                                                        |
| `internal/lib/task`    | `spawn` observation - `wait` / `poll` / `discard` / `waitAll` / `waitAny` - under `-race`                                                                                                                               |
| `internal/lib/testing` | Assertion vocabulary (`assert*`) throwing `Error{kind:"assertion"}`; `run` / `runWith` failure classification into `testing.Result`; text / TAP / JUnit report rendering                                                |
| `internal/lib/time`    | Constructors / accessors / arithmetic round-trip; ISO weekday remapping; deterministic via a `nowFunc` package-var override                                                                                            |
| `internal/lint`        | The `L0nn` / `L1nn` / `L2nn` / `L3nn` check catalog; `# lint-disable` suppression; `--checks` / `.jennifer-lint` selection and unknown-ID rejection                                                                    |
| `internal/profile`     | Collector aggregation (self / cumulative, hit counts); table, Chrome-trace, and pprof rendering (gzip + string-table checks)                                                                                           |
| `cmd/jennifer`         | Golden test that runs every `examples/*.j` and compares stdout to `examples/expected/*.txt`; REPL `inputComplete` helper; AST-JSON validity; formatter idempotence + behavior preservation; cross-file error reporting |

The table lists representative packages; every `internal/lib/*` library
carries its own `_test.go`. `internal/libsig` has none of its own: its
signature table is checked against the installed builtins by
`TestLibsigCoversInstalledBuiltins` in `internal/stdlib`.
`internal/version` is generated code (`version_gen.go`), verified
indirectly through `internal/lib/meta`.

Run everything with `go test ./...`. Concurrency-touching packages
(`internal/interpreter`, `internal/lib/task`) should also run under
`go test -race ./...`.
