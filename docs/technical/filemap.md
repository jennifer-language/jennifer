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
| `devtools_tinygo.go` | TinyGo build: run-only stubs for `tokens` / `ast` / `fmt` / `lint` / `profile` / `test`. |

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
| `highlight_test.go` | Docs-site highlight-def generation test. |
| `smtp_send_test.go` / `pop_recv_test.go` / `imap_recv_test.go` / `mail_xoauth2_test.go` | Mail-module integration tests: `smtp` / `pop` / `imap` / XOAUTH2 against in-process fake servers. |
| `redis_test.go` / `resque_test.go` | `redis` / `resque` integration tests against an in-process RESP server. |
| `memcache_test.go` / `session_test.go` / `ratelimit_test.go` | `memcache` and its reference modules against an in-process memcached fake. |
| `http_test.go` / `gotify_test.go` / `rest_test.go` / `oauth_test.go` | `http` client + `gotify` / `rest` / `oauth` against an in-process `net/http` server. |

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
| `module/resolve_test.go` | Resolver + sysmoddir tests. |

## Parser (`internal/parser/`)

| File | Description |
| ---- | ----------- |
| `ast.go` | AST node types + `Sprint` (incl. namespaced struct types). |
| `parser.go` | Recursive-descent parser; namespaced struct literals. |
| `resolver.go` | Parse-time scope / slot resolution; undefined-variable + shadowing errors. |
| `fold.go` | Parse-time constant folding of literal-only subtrees. |
| `parser_test.go` / `fold_test.go` | Parser and constant-folding tests (incl. "runtime error stays runtime"). |

## Interpreter (`internal/interpreter/`)

| File | Description |
| ---- | ----------- |
| `value.go` | Runtime `Value` tagged union (incl. struct + namespaced-struct + `KindObject` tags). |
| `environment.go` | Scoped symbol table (name map + slot slice). |
| `interpreter.go` | Tree-walking evaluator; namespaced-struct + object registration and resolution. |
| `interpreter_test.go` | End-to-end interpreter tests. |
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
| `value_alias_test.go` | Shared-marker COW alias-stress tests. |
| `collection_typing_test.go` | Generic collection validated entry-by-entry against the declared element type at def-init + assign. |

## Standard libraries (`internal/lib/`)

| File | Description |
| ---- | ----------- |
| `io/iolib.go`, `io/format.go`, `io/input.go` | `io`: entrypoint; `printf` / `sprintf` + verb-modifier mini-language; `readLine` / `readBytes` / `readChars` / `eof`. |
| `convert/convert.go` | `convert`: `toInt` / `toFloat` / `toString` / `toBool` / `typeOf` / `objectType` + UTF-8 codecs + codepoint bridges. |
| `math/mathlib.go` | `math`: `abs` / `min` / `max` / `sqrt` / `pow` / `floor` / `ceil` / `round` / `rand*`; `PI` / `E`. |
| `strings/stringslib.go` | `strings`: case, predicates, slicing, `split` / `chars` / `join`. |
| `lists/listslib.go` | `lists`: `push` / `pop` / `first` / `last` / `head` / `tail` / `reverse` / `sort` / `contains` / `concat` / `slice` / `shuffle` / `range`. |
| `maps/mapslib.go` | `maps`: `keys` / `values` / `has` / `delete` / `merge`. |
| `os/oslib.go`, `os/exec.go` | `os`: `PLATFORM` / `ARCH` / `EOL` / `DIRSEP` / `PATHSEP` / `ARGS` + `getEnv` / `hasFlag` / `flag` / `isTerminal`; external-program execution (`run` / `spawn` / `wait` / `poll` / `kill`). |
| `meta/metalib.go` | `meta`: `VERSION` / `BUILD` / `SYSMODDIR` (interpreter self-identity). |
| `time/timelib.go`, `time/zone.go`, `time/format.go` | `time`: `Time` / `Duration`, Unix + calendar + arithmetic; fixed-offset `Zone`, `UTC` / `local` / `inZone`; strftime `format` / `parse` + ISO round-trip. |
| `hash/hashlib.go` | `hash`: MD5 / SHA-1 / SHA-256 / SHA-384 / SHA-512 one-shot + streaming via a codec table. |
| `crc/crclib.go` | `crc`: CRC-32 / CRC-64 (big-endian) one-shot + streaming. |
| `compress/compresslib.go` | `compress`: `pack` / `unpack` (gzip / zlib / deflate) + streaming (`compress.Stream`); optional level. |
| `archive/archivelib.go` | `archive`: tar / zip / tar.gz `pack` / `unpack` over `bytes` (`archive.Entry`); no `fs` dependency. |
| `encoding/encodinglib.go`, `encoding/codecs.go`, `encoding/codecs_gen.go`, `encoding/gen_codecs.go` | `encoding`: introspection + `toText` / `fromText` + `encode` / `decode`; hand-written `ascii` / `ebcdic` + generated ISO-8859-N / Windows-125N tables (`codecs_gen.go` is generated, do not edit; `gen_codecs.go` is the `go:build ignore` codegen). |
| `json/jsonlib.go`, `json/jsondecode.go` | `json`: RFC 8259 `encode` / `encodePretty` + `Value` emitter; recursive-descent decoder -> opaque `json.Value` (`KindObject`), positioned errors. |
| `task/tasklib.go` | `task`: `wait` / `poll` / `discard` / `waitAll` / `waitAny` over `task of T` handles. |
| `fs/fslib.go`, `fs/handles.go` | `fs`: one-shot read / write / append + metadata + dir ops; buffered `fs.File` handles. |
| `net/netlib.go`, `net/netlib_std.go`, `net/netlib_tinygo.go` | `net`: shared install / dispatch; `!tinygo` full TCP / TLS / UDP / DNS; `tinygo` friendly-error stubs. |
| `regex/regexlib.go` | `regex`: `matches` / `find` / `findAll` / `replace` / `split` / `escape` + `regex.Match` + 128-entry LRU pattern cache. |
| `testing/testinglib.go`, `testing/assertions.go` | `testing`: `run` / `runWith` / `results` / `reset` / `report` (text / TAP / JUnit) + exit interception; the six `assert*` builtins. |
| `uuid/uuidlib.go` | `uuid`: `generate("v4"/"v7")` + `parse` / `isValid` / `version` + `NIL` (RFC 9562). |
| `crypto/cryptolib.go` | `crypto`: crypto-grade random (`randBytes` / `randInt`), constant-time `hmacEqual`, key derivation (`hkdf` / `pbkdf`), AEAD `encrypt` / `decrypt` (AES-256-GCM), Ed25519 `signKeypair` / `sign` / `verify`; registers the asymmetric `rsa*` / `ecdsa*` surface. |
| `crypto/cryptolib_asym_std.go`, `crypto/cryptolib_asym_tiny.go` | `crypto` RSA / ECDSA over PEM keys (`rsaSign` / `rsaVerify` / `ecdsaSign` / `ecdsaVerify`, for JWT RS\* / ES\*): `!tinygo` real over `crypto/rsa` + `crypto/ecdsa` + `crypto/x509`; `tinygo` friendly-error stub (build-tag split like `net`). |
| `crypto/cryptolib_acme_std.go`, `crypto/cryptolib_acme_tiny.go` | `crypto` key generation, CSR, and canonical JWK (`rsaGenerateKey` / `ecGenerateKey` / `jwkPublic` / `csr`, for the `acme` module): same `!tinygo` real / `tinygo` stub split. |
| `intl/intllib.go` | `intl`: locale catalogs - `load(lang, catalog)`, `setLocale` / `locale`, `tr(key[, params])` with `{name}` interpolation. |
| `term/termlib.go`, `term/termlib_std.go`, `term/termlib_tinygo.go` | `term`: raw mode (`makeRaw` / `restore` -> `term.State`), `size`, raw byte reads; `!tinygo` real termios (+ `RestoreAll` for the CLI signal path), `tinygo` stubs. |
| `httpd/httpdlib.go`, `httpd/httpdlib_std.go`, `httpd/httpdlib_tinygo.go` | `httpd`: HTTP/1.1 server engine over `net/http` (TLS, graceful shutdown) - pull loop `listen` / `accept` / `respond` + request accessors; `tinygo` stubs. |
| `xml/xmllib.go`, `xml/xmldecode.go`, `xml/xmlaccess.go` | `xml`: `decode` / `encode` / `encodePretty` over an opaque `xml.Value` element tree (ordered attributes, children, mixed text). |
| `yaml/yamllib.go`, `yaml/yamldecode.go`, `yaml/yamlencode.go`, `yaml/yamlaccess.go`, `yaml/yamlwrite.go` | `yaml`: YAML 1.2 `decode` / `decodeAll` / `encode` / `encodePretty` over an opaque `yaml.Value` (on `gopkg.in/yaml.v3`); same read / walk / write surface as `json`. |
| `toml/tomllib.go`, `toml/tomldecode.go`, `toml/tomlaccess.go`, `toml/tomlwrite.go` | `toml`: TOML 1.0 `encode` / `encodePretty` / `decode`, same `Value` walk / write surface as `json`, plus `asDatetime`. |
| `devio/devio.go` | Shared plumbing for the handle-based libraries (`serial` / `spi` / `iic` / `gpio`, also `sql`): integer-handle structs + typed positional-argument extraction. Not a Jennifer library itself (no namespace, no Install). |
| `serial/seriallib.go`, `serial/seriallib_linux.go`, `serial/seriallib_other.go` | `serial`: termios serial-port I/O - `open` / `openWith` -> `serial.Port`, `read` / `write` / `flush` / `close`. Linux-only; stubs elsewhere. |
| `spi/spilib.go`, `spi/spilib_linux.go`, `spi/spilib_other.go` | `spi`: SPI devices - `open` -> `spi.Device`, `configure(mode, speedHz)`, full-duplex `transfer`, `close`. Linux-only; stubs elsewhere. |
| `iic/iiclib.go`, `iic/iiclib_linux.go`, `iic/iiclib_other.go` | `iic`: the I2C bus - `open(path, addr)` -> `iic.Bus`, `read` / `write` / `readReg` / `writeReg`, `close`. Linux-only; stubs elsewhere. |
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
| `internal/lint/scope.go` | Scope-aware traversal for binding-visibility checks (L101 unused-local, L104 throw-non-error). |
| `internal/lint/walk.go` | Flat AST walker for node-shape checks (L102 / L103 / L105 / L201 / L202). |
| `internal/lint/config.go` | `--checks` / `.jennifer-lint` selection parsing (selectable IDs only). |
| `internal/lint/suppress.go` | `# lint-disable` directive parsing + application; a malformed / unknown-ID directive becomes an L004 finding. |
| `internal/profile/profile.go` | `profile` collector: per-position hit counts + wall-clock (+ optional alloc counting). |
| `internal/profile/render.go` | Table + Chrome-trace renderers. |
| `internal/profile/pprof.go` | Gzipped-pprof output (hand-encoded, zero-dependency). |

## Shared limits

| File | Description |
| ---- | ----------- |
| `internal/limits/nesting_std.go` | `MaxNestingDepth = 1000` (`!tinygo`): the recursive-descent nesting cap enforced by the language parser and the json / toml / xml decoders on the default binary. |
| `internal/limits/nesting_tiny.go` | `MaxNestingDepth = 64` (`tinygo`): the same cap lowered to stay below `jennifer-tiny`'s fixed 2 MB stack. See `docs/technical/tinygo.md`. |

## Version & build

| File | Description |
| ---- | ----------- |
| `internal/version/version.go` | Default `Version = "dev"`. |
| `internal/version/version_gen.go` | GENERATED by `scripts/gen-version.sh` (gitignored). |
| `scripts/version.sh` | Computes the version string from git state. |
| `scripts/gen-version.sh` | Writes `version_gen.go` before each build. |
| `Makefile` | `build` (both binaries) / `build-tinygo` / `build-go` / `test` / `clean` / `version`. |

## Jennifer-coded modules (`modules/`)

Distributable `.j` modules brought in with `import`. Each `X.j` ships a
co-located `X_test.j` white-box overlay (run under `jennifer test`) and a
`examples/modules/X_demo.j` demo; a reference doc lives under
[`docs/modules/`](../modules/index.md).

| Module | Description |
| ------ | ----------- |
| `ansi.j` | Terminal styling as string wrappers (color / style / rgb / strip); TTY-aware. |
| `csv.j` | RFC 4180 CSV parse / format (+ any delimiter) + header-keyed records. |
| `semver.j` | Strict SemVer 2.0.0 parse / compare / sort / increment. |
| `htmlwriter.j` | Build an HTML element tree and render escaped HTML5. |
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
