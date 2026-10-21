# File map

Where each piece of the codebase lives, grouped by area. The 6-stage pipeline
(lexer -> preproc -> parser -> resolver -> interpreter -> libraries) is described
in [interpreter.md](interpreter.md); this page is the file-level index.

## CLI (`cmd/jennifer/`)

| File | Description |
| ---- | ----------- |
| `main.go` | CLI + source-context error formatting; argv forwarding to the user program. |
| `repl.go` | Interactive REPL loop (TTY + bufio fallback). |
| `lineedit.go` | Raw-mode line editor (cursor keys, word ops). |
| `history.go` | In-memory REPL history ring buffer. |
| `dump.go` | `tokens` and `ast` subcommands. |
| `astjson.go` | Hand-rolled AST -> JSON emitter. |
| `fmt.go` | Token-level source formatter; comment + blank-line preservation. |
| `lint.go` | `lint` subcommand: pipeline + config + suppression + JSON / human / GitHub rendering. |
| `profile.go` | `profile` subcommand: table / pprof / trace output. |
| `test.go` | `test` subcommand: `test*` discovery, `setUp` / `tearDown`, `--isolated` subprocess mode, text / TAP / JUnit reports. |
| `coverage.go` | `test --coverage`: the executable-statement walk (coverage denominator) and the text / JSON coverage report. |
| `serve.go` | `serve` subcommand: run a program, `--watch` re-runs it in a child process on any change to the entry file. |
| `highlight.go` | ANSI token colouring of the REPL input line (categories mirror the `editors/` definitions). |
| `diag_unix.go` / `diag_other.go` | `kill -USR1` interpreter diagnostics dump (Unix); no-op elsewhere. |
| `termrestore_unix.go` / `termrestore_other.go` | Restore a raw-mode terminal on a terminating signal (Unix); no-op elsewhere. |
| `devtools_tinygo.go` | TinyGo build: run-only stubs for `tokens` / `ast` / `fmt` / `lint` / `profile` / `test`, plus a `serve` stub (no network stack). |

### CLI tests

| File | Description |
| ---- | ----------- |
| `examples_test.go` | Golden-file integration test over top-level `examples/*.j` (skips files without an `expected/`). |
| `module_overlay_test.go` | Runs every `modules/*_test.j` overlay under `jennifer test`. |
| `repl_test.go` | REPL `inputComplete` unit tests. |
| `lineedit_test.go` | Line-editor state-machine tests. |
| `dump_test.go` | AST-JSON validity and token-name tests. |
| `fmt_test.go` | Formatter idempotence + behaviour + comment-preservation tests. |
| `cross_file_error_test.go` | Cross-file error reporting tests. |
| `highlight_test.go` | REPL input-line colouring tests (text preserved byte for byte, categories, lex-error fallback). |
| `smtp_send_test.go` / `pop_recv_test.go` / `imap_recv_test.go` / `mail_xoauth2_test.go` | Mail-module integration tests: `smtp` / `pop` / `imap` / XOAUTH2 against in-process fake servers. |
| `redis_test.go` / `resque_test.go` | `redis` / `resque` integration tests against an in-process RESP server. |
| `memcache_test.go` / `session_test.go` / `ratelimit_test.go` | `memcache` and its reference modules against an in-process memcached fake. |
| `http_test.go` / `gotify_test.go` / `rest_test.go` / `oauth_test.go` | `http` client + `gotify` / `rest` / `oauth` against an in-process `net/http` server. |
| other `*_test.go` | Further per-module integration tests (`web_test.go`, `mqtt_test.go`, `ldap_test.go`, ...) and CLI / interpreter-boundary tests (`lint_test.go`, `coverage_test.go`, `vendor_test.go`, `concurrent_dispatch_test.go`, `reentry_depth_test.go`, `funcvalue_home_test.go`, ...). |

## Lexer, preprocessor, module resolution (`internal/`)

| File | Description |
| ---- | ----------- |
| `lexer/token.go` | Token type definitions (incl. trivia tokens). |
| `lexer/lexer.go` | Scanner with optional file tagging; nested block comments. |
| `lexer/lexer_test.go` | Lexer tests. |
| `preproc/preproc.go` | File-import preprocessor; trivia stripping. |
| `preproc/preproc_test.go` | Preprocessor tests. |
| `module/resolve.go` | Module path classification + resolution. |
| `module/sysmoddir.go` | System module dir: CLI / env / compile precedence + validation. |
| `module/resolve_test.go` / `module/sysmoddir_test.go` | Resolver + sysmoddir tests. |
| `reqcheck/reqcheck.go` | Requirement-header (`# pragma-jennifer-*:`) validation, shared by every first read of a source file. |
| `capabilities/capabilities.go`, `capabilities/capabilities_std.go`, `capabilities/capabilities_tiny.go` | The build's host-capability set (`exec` / `net` / `sql` standard, empty tiny), behind the capability header check and `meta.CAPABILITIES`. |

## Parser (`internal/parser/`)

| File | Description |
| ---- | ----------- |
| `ast.go` | AST node types + `Sprint` (incl. namespaced struct types). |
| `parser.go` | Recursive-descent parser; namespaced struct literals. |
| `resolver.go` | Parse-time scope / slot resolution; undefined-variable + shadowing errors; enum `match` patterns + exhaustiveness; borrowable-parameter / local and move-on-last-use marking. |
| `fold.go` | Parse-time constant folding of literal-only subtrees. |
| `ownership.go` | Ownership analysis run from `Resolve`: which functions return solely owned values (`ReturnsOwned`) and which `$v = f($v, ...)` calls move their argument (`MoveArg`). |
| `parser_test.go` / `fold_test.go` / `ownership_test.go` / `borrow_test.go` | Parser, constant-folding (incl. "runtime error stays runtime"), ownership, and borrow-analysis tests. |

## Interpreter (`internal/interpreter/`)

| File | Description |
| ---- | ----------- |
| `value.go` | Runtime `Value` tagged union (incl. struct + namespaced-struct + `KindObject` tags; an object's payload is an `ObjectBox`: a wrapped `Value` tree or a library-native `NativeObject`). |
| `environment.go` | Scoped symbol table (name map + slot slice). |
| `interpreter.go` | Tree-walking evaluator; namespaced-struct + object registration and resolution. |
| `module.go` | Module loading: the run-scoped module registry, sub-interpreters, export checks, cross-boundary struct retagging. |
| `globalsafe.go` | `GlobalSafe` escape analysis that widens read-only-parameter borrow to functions proven to mutate no global. |
| `spawncapture.go` | Spawn-capture analysis: the globals each `spawn` body reads, so the snapshot copies only those. |
| `argsnap.go` | Argument snapshots: marks call arguments that alias a global a later argument may mutate, so they are copied (never borrowed). |
| `intfast.go` | In-place int / bool fast path for operands, conditions and assignments; falls back to the general evaluator on anything it does not handle exactly. |
| `structlit_check.go` | `checkStructLits`: load-time field-set check of every struct / enum-variant literal. |
| `waitset.go` | Register-then-poll wait set behind `task.waitAny` / `waitAnyTimeout` / `channel.select` (no `reflect.Select`). |
| `profiling.go` | The optional `Profiler` interface and `eagerCopy` hook. |
| `diag_runtime_std.go` / `diag_runtime_tiny.go` | Runtime snapshot (goroutines, memory) for the `kill -USR1` diagnostics dump. |
| `interpreter_test.go` | End-to-end interpreter tests. |
| `ownership_test.go` | Value semantics on every path of the owned-result and moved-argument copy elisions, across a module boundary too. |
| `interactive_test.go` | REPL `EvalInteractive` tests. |
| `namespace_test.go` | Namespaced builtins + constants tests. |
| `namespaced_struct_test.go` | Namespaced struct tests (synthetic `widgets` lib). |
| `structs_test.go` | User-defined struct tests. |
| `trycatch_test.go` | `try` / `catch` / `throw` tests. |
| `spawn_test.go` | `spawn` / `task of T` runtime + registry tests. |
| `control_flow_test.go` | `break` / `continue` / `repeat` / `exit` tests. |
| `collections_test.go` | Lists, maps, iteration tests. |
| `compound_test.go` | Nested list-of-list / map-of-list / chained-index tests. |
| `bytes_test.go` | Bytes type + non-decimal literals + bit-op tests. |
| `callbyname_test.go` | `CallByName` / `CallByNameWith` dispatch + `RaiseError` classification tests. |
| `value_alias_test.go` | Value-semantics alias-stress tests (eager deep copies; no two live bindings share a compound backing). |
| `collection_typing_test.go` | Generic collection validated entry-by-entry against the declared element type at def-init + assign. |

## Standard libraries (`internal/lib/`)

| File | Description |
| ---- | ----------- |
| `io/iolib.go`, `io/format.go`, `io/input.go` | `io`: entrypoint; `printf` / `sprintf` + verb-modifier mini-language; `readLine` / `readBytes` / `readChars` / `eof`. |
| `convert/convert.go` | `convert`: `toInt` / `toFloat` / `toString` / `toBool` / `typeOf` / `objectType` + UTF-8 codecs + codepoint bridges. |
| `math/mathlib.go`, `math/elementary.go`, `math/special.go` | `math`: `abs` / `min` / `max` / `sqrt` / `pow` / `floor` / `ceil` / `round` / `rand*`; `PI` / `E`; elementary and special functions. |
| `stats/statslib.go`, `stats/distributions.go`, `stats/inference.go` | `stats`: descriptive statistics over `list of int` / `list of float`, distributions, inference. |
| `linalg/linalglib.go` | `linalg`: vectors (`list of float`) and matrices (`list of list of float`). |
| `ml/mllib.go` (+ `classify.go`, `cluster.go`, `decomp.go`, `introspect.go`, `metrics.go`, `preprocess.go`, `regression.go`, `tree.go`) | `ml`: machine-learning primitives. |
| `binary/binarylib.go` | `binary`: bulk `bytes` operations (`concat` / `slice` / `indexOf` / `split` / ...). |
| `path/pathlib.go` | `path`: OS-aware path manipulation over `path/filepath` (no disk I/O). |
| `strings/stringslib.go` | `strings`: case, predicates, slicing, `split` / `chars` / `join`. |
| `lists/listslib.go` | `lists`: `push` / `pop` / `first` / `last` / `head` / `tail` / `reverse` / `sort` / `contains` / `indexOf` / `mismatch` / `concat` / `slice` / `shuffle` / `range`, plus the higher-order `map` / `filter` / `reduce` / `find` / `any` / `all` / `sortBy`. |
| `maps/mapslib.go` | `maps`: `keys` / `values` / `has` / `delete` / `merge`. |
| `os/oslib.go`, `os/exec.go`, `os/signal_unix.go`, `os/signal_other.go`, `os/isterminal_std.go`, `os/isterminal_tinygo.go` | `os`: `PLATFORM` / `ARCH` / `EOL` / `DIRSEP` / `PATHSEP` / `ARGS` + `getEnv` / `hasFlag` / `flag` / `isTerminal`; external-program execution (`run` / `spawn` / `wait` / `poll` / `kill` / `release`); `catchSignal` / `gotSignal` (Unix); `isTerminal` per build. |
| `meta/metalib.go` | `meta`: `VERSION` / `BUILD` / `SYSMODDIR` / `CAPABILITIES` (interpreter self-identity) + `hasCapability` / `call` / `defined` / `callMain` / `definedMain`. |
| `time/timelib.go`, `time/zone.go`, `time/format.go` | `time`: `Time` / `Duration`, Unix + calendar + arithmetic; fixed-offset `Zone`, `UTC` / `local` / `inZone`; strftime `format` / `parse` + ISO round-trip. |
| `hash/hashlib.go` | `hash`: MD5 / SHA-1 / SHA-256 / SHA-384 / SHA-512 one-shot `compute`, `hmac`, constant-time `equal`, + streaming via a codec table. |
| `crc/crclib.go` | `crc`: CRC-32 / CRC-64 (big-endian) one-shot + streaming. |
| `compress/compresslib.go` | `compress`: `pack` / `unpack` (gzip / zlib / deflate) + streaming (`compress.Stream`); optional level. |
| `asn1/asn1lib.go`, `asn1/asn1codec.go` | `asn1`: BER decode / DER encode over an opaque `asn1.Value`, typed constructors + accessors. |
| `archive/archivelib.go` | `archive`: tar / zip / tar.gz `pack` / `unpack` over `bytes` (`archive.Entry`); no `fs` dependency. |
| `encoding/encodinglib.go`, `encoding/codecs.go`, `encoding/codecs_gen.go`, `encoding/gen_codecs.go` | `encoding`: introspection + `toText` / `fromText` + `encode` / `decode`; hand-written `ascii` / `ebcdic` + generated ISO-8859-N / Windows-125N tables (`codecs_gen.go` is generated, do not edit; `gen_codecs.go` is the `go:build ignore` codegen). |
| `doctree/node.go`, `doctree/edit.go`, `doctree/build.go`, `doctree/lib.go` | The compact document tree shared by `json` / `toml` / `yaml`: a 40-byte `Node` per value (map members in document order, a sorted key index above 8 members), about 8x smaller than the same document as `Value`s; JSON Pointer addressing and the copy-the-spine write verbs (`edit.go`); a `Builder` that grows containers in place for TOML's reopened tables, then seals them (`build.go`); and `Lib`, which registers the opaque `NAME.Value` type (a `NativeObject` payload) and the shared accessor + write builtins, with per-library options for `isNull` and date-times (`lib.go`). |
| `json/jsonlib.go`, `json/jsondecode.go`, `json/jsonnode.go` | `json`: RFC 8259 `encode` / `encodePretty` + `Value` emitter; recursive-descent decoder into a `doctree` tree (unescaped strings sliced from the input), positioned errors. |
| `task/tasklib.go` | `task`: `wait` / `poll` / `discard` / `waitAll` / `waitAny`, `cancel` / `cancelled`, `waitTimeout` / `waitAnyTimeout` over `task of T` handles. |
| `channel/channellib.go` | `channel`: `make` / `send` / `recv` / `close` / `select` / `len` / `capacity` over `channel of T`. |
| `kv/kvlib.go` | `kv`: in-process key/value store with per-key TTL; a run-scoped registry on the host interpreter. |
| `fs/fslib.go`, `fs/handles.go`, `fs/temp.go`, `fs/watch.go` | `fs`: one-shot read / write / append + metadata + dir ops; buffered `fs.File` handles; `makeTempFile` / `makeTempDir`; the polling `fs.Watcher`. |
| `net/netlib.go`, `net/netlib_std.go`, `net/netlib_tinygo.go` (+ `netlib_broadcast_*.go`, `netlib_binddevice_*.go`) | `net`: shared install / dispatch; `!tinygo` full TCP / TLS / UDP / DNS; `tinygo` friendly-error stubs; per-OS `setBroadcast` / `bindToDevice`. |
| `regex/regexlib.go` | `regex`: `matches` / `find` / `findAll` / `replace` / `split` / `escape` + `regex.Match` + 128-entry LRU pattern cache. |
| `testing/testinglib.go`, `testing/assertions.go` | `testing`: `run` / `runWith` / `results` / `reset` / `report` (text / TAP / JUnit) + exit interception; the seven `assert*` builtins. |
| `uuid/uuidlib.go` | `uuid`: `v4` / `v7` + `parse` / `isValid` / `version` + `NIL` (RFC 9562). |
| `crypto/cryptolib.go` | `crypto`: crypto-grade random (`randBytes` / `randInt`), constant-time `hmacEqual`, key derivation (`hkdf` / `pbkdf2`), AEAD `encrypt` / `decrypt` (AES-256-GCM), Ed25519 `signKeypair` / `sign` / `verify`; registers the asymmetric `rsa*` / `ecdsa*` surface. |
| `crypto/cryptolib_asym_std.go`, `crypto/cryptolib_asym_tiny.go` | `crypto` RSA / ECDSA over PEM keys (`rsaSign` / `rsaVerify` / `ecdsaSign` / `ecdsaVerify`, for JWT RS\* / ES\*): `!tinygo` real over `crypto/rsa` + `crypto/ecdsa` + `crypto/x509`; `tinygo` friendly-error stub (build-tag split like `net`). |
| `crypto/cryptolib_acme_std.go`, `crypto/cryptolib_acme_tiny.go` | `crypto` key generation, CSR, and canonical JWK (`rsaGenerateKey` / `ecGenerateKey` / `jwkPublic` / `jwkToPem` / `csr`, for the `acme` module): same `!tinygo` real / `tinygo` stub split. |
| `crypto/cryptolib_pqc_std.go`, `crypto/cryptolib_pqc_tiny.go` | `crypto` ML-KEM-768 (`mlkemKeypair` / `mlkemEncapsulate` / `mlkemDecapsulate`): same `!tinygo` real / `tinygo` stub split. |
| `crypto/mtwei.go` | `crypto` EC-SRP over Curve25519 (`mtweiKeygen` / `mtweiId` / `mtweiClientKey`) for the `mactelnet` module, over `math/big`. |
| `intl/intllib.go` | `intl`: locale catalogs - `load(lang, catalog)`, `setLocale` / `locale`, `tr(key[, params])` with `%name%` interpolation. |
| `term/termlib.go`, `term/termlib_std.go`, `term/termlib_tinygo.go` | `term`: raw mode (`makeRaw` / `restore` -> `term.State`), `size`, raw byte reads; `!tinygo` real termios (+ `RestoreAll` for the CLI signal path), `tinygo` stubs. |
| `httpd/httpdlib.go`, `httpd/httpdlib_std.go`, `httpd/httpdlib_tinygo.go`, `httpd/httpdlib_memlimit.go` | `httpd`: HTTP/1.1 server engine over `net/http` (TLS, graceful shutdown) - pull loop `listen` / `accept` / `respond` + request accessors; the cgroup-aware buffer budget (`httpdlib_memlimit.go`); `tinygo` stubs. |
| `xml/xmllib.go`, `xml/xmldecode.go`, `xml/xmlaccess.go`, `xml/xmlnode.go` | `xml`: `decode` / `encode` / `encodePretty` over an opaque `xml.Value` element tree (ordered attributes, children, mixed text); `xmlnode.go` is the compact native tree (a `NativeObject` payload, names and plain text sliced from the input). |
| `yaml/yamllib.go`, `yaml/yamldecode.go`, `yaml/yamlencode.go` | `yaml`: YAML 1.2 `decode` / `decodeAll` / `encode` / `encodePretty` over an opaque `yaml.Value` (on `gopkg.in/yaml.v3`, converted into a `doctree` tree with bytes and timestamp nodes); the read / walk / write surface is `doctree`'s. |
| `toml/tomllib.go`, `toml/tomldecode.go` | `toml`: TOML 1.0 `encode` / `encodePretty` / `decode` into a `doctree` tree (tables grown with a `doctree.Builder`, date-times as datetime nodes tagged with their form); the read / walk / write surface, plus `asDatetime`, is `doctree`'s. |
| `devio/devio.go` | Shared plumbing for the handle-based libraries (`serial` / `spi` / `i2c` / `gpio`, also `sql`): integer-handle structs + typed positional-argument extraction. Not a Jennifer library itself (no namespace, no Install). |
| `serial/seriallib.go`, `serial/seriallib_linux.go`, `serial/seriallib_other.go` | `serial`: termios serial-port I/O - `open` / `openWith` -> `serial.Port`, `read` / `write` / `flush` / `close`. Linux-only; stubs elsewhere. |
| `spi/spilib.go`, `spi/spilib_linux.go`, `spi/spilib_other.go` | `spi`: SPI devices - `open` -> `spi.Device`, `configure(mode, speedHz)`, full-duplex `transfer`, `close`. Linux-only; stubs elsewhere. |
| `i2c/i2clib.go`, `i2c/i2clib_linux.go`, `i2c/i2clib_other.go` | `i2c`: the I2C bus - `open(path, addr)` -> `i2c.Bus`, `read` / `write` / `readReg` / `writeReg`, `close`. Linux-only; stubs elsewhere. |
| `gpio/gpiolib.go`, `gpio/gpiolib_linux.go`, `gpio/gpiolib_other.go` | `gpio`: `/dev/gpiochipN` GPIO v2 lines - pin-keyed `setup` / `read` / `write` / `release` + `IN` / `OUT`. Linux-only; stubs elsewhere. |
| `sql/sqllib.go`, `sql/sqllib_std.go`, `sql/sqllib_tiny.go` | `sql`: MySQL / MariaDB + PostgreSQL client over `database/sql` (placeholder-only binding, pull cursor + typed accessors, transactions, prepared statements); `!tinygo` imports the drivers, `tinygo` stubs so the driver trees never compile there. |
| `*/…_test.go` | Each library has a co-located `_test.go` with its unit tests (canonical vectors, round-trips, boundary errors). |

## Tooling internals

| File | Description |
| ---- | ----------- |
| `internal/stdlib/stdlib.go` | `InstallAll`: the single registration point for every system library; `run` / `repl` / `profile` / `test` all call it (the seam for build-time selection). |
| `internal/lint/lint.go` | `lint` checks: `Diagnostic` + grouped-ID registry (L0nn source / L1nn correctness / L2nn style / L3nn lifecycle) + `KnownIDs` / selectable / `Catalog`. |
| `internal/lint/run.go` | Check driver + threshold `Config` + source-error diagnostic builder. |
| `internal/lint/checks.go` | The individual check implementations. |
| `internal/lint/scope.go` | Scope-aware traversal for binding-visibility checks (L101 unused-local, L104 throw-non-error, and the typed hooks L110 / L111 use). |
| `internal/lint/types.go` | The static typer behind L110 / L111: the type an expression certainly has, struct identity normalised to the linted file's terms, and the runtime-faithful mismatch test. |
| `internal/lint/typecheck.go` | L110 argument-type and L111 binding-type checks over the typer. |
| `internal/lint/members.go` | L114 unknown-member: a `ns.name` checked against its library's signature table and registered constants / types, or its module's exported surface, with a "did you mean" hint. L115 unknown-library: each `use NAME;` checked against the installed library names. |
| `internal/libsig/` | Static signature table of every library builtin (arity, argument types, return type), one file per library; read by `lint` (L109 / L110 / L111). `internal/stdlib/libsig_test.go` keeps it in lock-step with `InstallAll`. |
| `internal/lint/walk.go` | Flat AST walker for node-shape checks (L102 / L103 / L105 / L106 / L107 / L108 / L109 / L112 / L113 / L114 / L201 / L204). |
| `internal/lint/config.go` | `--checks` / `.jennifer-lint` selection parsing (selectable IDs only). |
| `internal/lint/suppress.go` | `# lint-disable` directive parsing + application (a line directive also covers a multi-line string / block comment ending on its line, or a docblock right below a lone directive; a line comment may end in one); a malformed / unknown-ID directive becomes an L004 finding. |
| `internal/profile/profile.go` | `profile` collector: per-position hit counts + wall-clock (+ optional alloc counting). |
| `internal/profile/render.go` | Table (statements, max call depth, allocs) + Chrome-trace renderers. |
| `internal/profile/pprof.go` | Gzipped-pprof output (hand-encoded, zero-dependency). |

## Shared limits

| File | Description |
| ---- | ----------- |
| `internal/limits/nesting_std.go` | `MaxNestingDepth = 1000` + `MaxCallDepth = 10000` (`!tinygo`): the recursive-descent nesting cap (language parser + json / toml / xml decoders) and the `evalCall` function-call-depth cap on the default binary. |
| `internal/limits/nesting_tiny.go` | `MaxNestingDepth = 64` + `MaxCallDepth = 120` (`tinygo`): the same caps lowered to stay below `jennifer-tiny`'s fixed 4 MB stack. See `docs/technical/tinygo.md`. |

## Version & build

| File | Description |
| ---- | ----------- |
| `internal/version/version.go` | Default `Version = "dev"`. |
| `internal/version/version_gen.go` | GENERATED by `scripts/gen-version.sh` (gitignored). |
| `scripts/version.sh` | Computes the version string from git state. |
| `scripts/gen-version.sh` | Writes `version_gen.go` before each build. |
| `scripts/gen-module-docs.j` | Jennifer program that renders the per-module API reference (`docs/api/`) + the `docs/SUMMARY.md` "Module API Reference" region from each module's doc comments, via the `docblock` module. Run at publish time by `docs.yml` before `grimoire build --pdf` (not committed); `test.yml` runs it as a smoke-check. |
| `docs/api/` | GENERATED by `scripts/gen-module-docs.j` at docs-build time (gitignored, not committed). One API page per shipped module plus `index.md`; the committed `SUMMARY.md` keeps an empty `api:start`/`api:end` region the generator fills. |
| `scripts/gen-coverage-docs.sh` | Shell generator for the test-coverage report: `go test -cover` (system libraries) + `jennifer test --coverage` (modules) into a Markdown page. Run at docs-build time by `docs.yml` after building the interpreter; excluded from the PDF. |
| `docs/technical/coverage.md` | GENERATED by `scripts/gen-coverage-docs.sh` at docs-build time (gitignored, not committed). Library + module statement-coverage tables; the committed `SUMMARY.md` keeps an empty `coverage:start`/`coverage:end` region the generator fills. |
| `Makefile` | `build` (both binaries) / `build-tinygo` / `build-go` / `test` / `clean` / `gen-version` / `version` / `docker`. |

## Jennifer-coded modules (`modules/`)

Distributable `.j` modules brought in with `import`. Each `X.j` ships a
co-located `X_test.j` white-box overlay (run under `jennifer test`) and a
`examples/modules/X_demo.j` demo; a reference doc lives under
[`docs/modules/`](../modules/index.md), which (with `modules/README.md`)
is the full catalog; the table below is a representative selection.

| Module | Description |
| ------ | ----------- |
| `ansi.j` | Terminal styling as string wrappers (color / style / rgb / strip); TTY-aware. |
| `csv.j` | RFC 4180 CSV parse / format (+ any delimiter) + header-keyed records. |
| `semver.j` | Strict SemVer 2.0.0 parse / compare / sort / increment. |
| `html.j` | Build an HTML element tree and render escaped HTML5. |
| `markdown.j` | CommonMark subset -> HTML / ANSI, plus Markdown authoring helpers. |
| `mime.j` | Build + parse MIME messages (RFC 5322 / 2045, RFC 2047 encoded-words). |
| `smtp.j` / `pop.j` / `imap.j` | Mail clients over `net`: send (SMTP), receive (POP3 / IMAP4rev1). |
| `sasl.j` | SASL auth encoders (`plain` / `login` / `bearer` XOAUTH2), pure base64. |
| `idna.j` | Internationalized domain names (Punycode `toAscii` / `toUnicode`). |
| `redis.j` | Redis RESP2 client over `net`. |
| `resque.j` | Resque-wire-compatible background jobs (on `redis`). |
| `memcache.j` | memcached text-protocol client over `net`. |
| `session.j` | Server-side sessions (on `memcache` + `uuid` + `json`). |
| `ratelimit.j` | Fixed-window rate limiter (on `memcache`). |
| `http.j` | HTTP/1.1 client over `net` (`https` via TLS). |
| `gotify.j` | Gotify push notifications (on `http`). |
| `rest.j` | Ergonomic REST layer (on `http` + `json`). |
| `oauth.j` | OAuth2 client - get-a-token grants (on `http` + `json`; feeds `sasl`). |

## Examples

| Path | Description |
| ---- | ----------- |
| `examples/*.j` | Example programs. |
| `examples/expected/*.txt` | Expected stdout per example (no entry == not in the golden suite). |
| `examples/with_import/` | Subdirectory demonstrating file imports. |
| `examples/showcase/` | Helpers spliced by `showcase.j`. |
| `examples/modules/*_demo.j` | Runnable demo per shipped module (smoke-run in CI; not golden-checked). |
