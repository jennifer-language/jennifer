# File map

```
cmd/jennifer/main.go                       CLI + source-context error formatting; argv forwarding to user program
cmd/jennifer/repl.go                       Interactive REPL loop (TTY + bufio fallback)
cmd/jennifer/lineedit.go                   Raw-mode line editor (cursor keys, word ops)
cmd/jennifer/history.go                    In-memory REPL history ring buffer
cmd/jennifer/dump.go                       `tokens` and `ast` subcommands
cmd/jennifer/astjson.go                    Hand-rolled AST -> JSON emitter
cmd/jennifer/fmt.go                        Token-level source formatter; comment + blank-line preservation
cmd/jennifer/lint.go                       `lint` subcommand: pipeline + config + suppression + JSON/human/github rendering (format-honest L0nn source errors, single JSON array across files)
cmd/jennifer/profile.go                    `profile` subcommand: table / pprof / trace output
cmd/jennifer/test.go                       `test` subcommand: test* discovery, setUp/tearDown, --isolated subprocess mode, text/TAP/JUnit reports
cmd/jennifer/devtools_tinygo.go            tinygo build: run-only stubs for tokens/ast/fmt/lint/profile/test
cmd/jennifer/examples_test.go              Golden-file integration test (skips files without expected/)
cmd/jennifer/repl_test.go                  REPL inputComplete unit tests
cmd/jennifer/lineedit_test.go              Line-editor state-machine tests
cmd/jennifer/dump_test.go                  AST-JSON validity and token-name tests
cmd/jennifer/fmt_test.go                   Formatter idempotence + behaviour + comment-preservation tests
cmd/jennifer/cross_file_error_test.go      Cross-file error reporting tests

internal/lexer/token.go                    Token type definitions (incl. trivia tokens)
internal/lexer/lexer.go                    Scanner with optional file tagging; nested block comments
internal/lexer/lexer_test.go               Lexer tests

internal/preproc/preproc.go                File-import preprocessor; trivia stripping
internal/preproc/preproc_test.go           Preprocessor tests

internal/parser/ast.go                     AST node types + Sprint (incl. namespaced struct types)
internal/parser/parser.go                  Recursive-descent parser; namespaced struct literals
internal/parser/resolver.go                parse-time scope/slot resolution; undefined-variable + shadowing errors
internal/parser/fold.go                    parse-time constant folding of literal-only subtrees
internal/parser/parser_test.go             Parser tests
internal/parser/fold_test.go               constant-folding tests (incl. runtime-error-stays-runtime)

internal/interpreter/value.go              Runtime Value tagged union (incl. struct + namespaced-struct tag)
internal/interpreter/environment.go        Scoped symbol table
internal/interpreter/interpreter.go        Tree-walking evaluator; namespaced-struct registration + resolution
internal/interpreter/interpreter_test.go   End-to-end interpreter tests
internal/interpreter/interactive_test.go   REPL EvalInteractive tests
internal/interpreter/namespace_test.go     Namespaced builtins + constants tests
internal/interpreter/namespaced_struct_test.go  namespaced struct tests (synthetic `widgets` lib)
internal/interpreter/structs_test.go       user-defined struct tests
internal/interpreter/trycatch_test.go      try/catch/throw tests
internal/interpreter/spawn_test.go         spawn / task-of-T runtime + registry tests
internal/interpreter/control_flow_test.go  break/continue/repeat/exit tests
internal/interpreter/collections_test.go   Lists, maps, iteration tests
internal/interpreter/compound_test.go      Nested list-of-list / map-of-list / chained-index tests
internal/interpreter/bytes_test.go         Bytes type + non-decimal literals + bit-op tests
internal/interpreter/callbyname_test.go    CallByName / CallByNameWith dispatch + RaiseError classification tests
internal/interpreter/value_alias_test.go   shared-marker COW alias-stress tests
internal/interpreter/collection_typing_test.gogeneric-collection (literal / decode) validated entry-by-entry against declared element type at def-init + assign

internal/lib/io/iolib.go                   `io` library entrypoint
internal/lib/io/format.go                  printf / sprintf + verb-modifier mini-language
internal/lib/io/input.go                   io.readLine / readBytes / readChars / eof
internal/lib/io/iolib_test.go              io library unit tests
internal/lib/io/input_test.go              stdin-input tests
internal/lib/convert/convert.go            `convert` library: toInt/toFloat/toString/toBool/typeOf + UTF-8 codecs
internal/lib/math/mathlib.go               `math` library: abs/min/max/sqrt/pow/floor/ceil/round/rand*; PI/E
internal/lib/strings/stringslib.go         `strings` library: case, predicates, slicing, split/chars/join
internal/lib/lists/listslib.go             `lists` library: push/pop/first/last/head/tail/reverse/sort/contains/concat/slice/shuffle/range
internal/lib/lists/listslib_test.go        lists library unit tests
internal/lib/maps/mapslib.go               `maps` library: keys/values/has/delete/merge
internal/lib/maps/mapslib_test.go          maps library unit tests
internal/lib/os/oslib.go                   `os` library: PLATFORM/ARCH/EOL/DIRSEP/PATHSEP/ARGS + getEnv/hasFlag/flag
internal/lib/os/oslib_test.go              os library unit tests
internal/lib/os/exec.go                    external-program execution (run/spawn/wait/poll/kill)
internal/lib/os/exec_test.go               exec tests (skip on non-Linux; TinyGo-gated at runtime)
internal/lib/meta/metalib.go               `meta` library: VERSION / BUILD (interpreter-self-identity)
internal/lib/meta/metalib_test.go          meta library unit tests
internal/lib/time/timelib.go               `time` library entrypoint + base surface (Time/Duration, Unix, calendar, arithmetic)
internal/lib/time/zone.go                  fixed-offset Zone struct, time.UTC, time.local, time.inZone
internal/lib/time/format.go                strftime format / parse + ISO round-trip
internal/lib/time/timelib_test.go          base surface unit tests
internal/lib/time/format_test.go           unit tests (format/parse/zone/UTC/local)
internal/lib/hash/hashlib.go               `hash` library: md5/sha1/sha256 one-shot + streaming via codec-table
internal/lib/hash/hashlib_test.go          hash library unit tests (canonical vectors + stream/one-shot equality)
internal/lib/crc/crclib.go                 `crc` library: crc32/crc64 (big-endian) one-shot + streaming
internal/lib/crc/crclib_test.go            crc library unit tests
internal/lib/compress/compresslib.go       `compress` library: pack/unpack (gzip/zlib/deflate) + streaming (compress.Stream handle); optional fast/default/best level
internal/lib/compress/compresslib_test.go  compress unit tests (round-trips, gzip magic + shrink, levels, streaming matches one-shot, errors)
internal/lib/archive/archivelib.go         `archive` library: tar/zip/tar.gz pack/unpack over bytes (archive.Entry struct); no fs dependency
internal/lib/archive/archivelib_test.go    archive unit tests (round-trips tar/zip/tar.gz, mode/mtime, default mode, gzip magic, empty, errors)
internal/lib/encoding/encodinglib.go       `encoding` library: introspection + toText/fromText + encode/decode dispatch
internal/lib/encoding/codecs.go            character codec tables for ascii / latin-1 / windows-1252 / ebcdic (IBM-1047)
internal/lib/encoding/encodinglib_test.go  encoding library unit tests
internal/lib/json/jsonlib.go               `json` library: RFC 8259 encode/encodePretty + Value->JSON emitter (structs/maps->object, bytes->base64)
internal/lib/json/jsondecode.go            `json` library: recursive-descent decoder -> generic Values (int/float split, positioned line/col errors)
internal/lib/json/jsonlib_test.go          json library unit tests (encode/decode, int/float split, escapes/surrogates, positioned errors, round-trips)
internal/lib/task/tasklib.go               `task` library: wait/poll/discard/waitAll/waitAny over `task of T` handles
internal/lib/task/tasklib_test.go          task library unit tests (wait/discard/loud-fail/waitAll/waitAny + boundary checks)
internal/lib/fs/fslib.go                   `fs` library: one-shot ops (read/write/append), metadata (exists/isFile/isDir/stat), dir ops (mkdir/mkdirAll/remove/removeAll/rename/list/walk), fs.Stat struct
internal/lib/fs/handles.go                 `fs` library: fs.File handle registry, open/close/readLine/readChars/readBytes/writeString/writeBytes/eof
internal/lib/fs/fslib_test.go              fs library unit tests (t.TempDir()-based; one-shot ops, metadata, dir ops, handles, spawn+fs composition)
internal/lib/net/netlib.go                 `net` library: shared - Install, struct registration, polymorphic close/address dispatch, arg-boundary helpers
internal/lib/net/netlib_std.go             `net` library: !tinygo build - full TCP/TLS/UDP/DNS implementation
internal/lib/net/netlib_tinygo.go          `net` library: tinygo build - friendly-error stubs pointing at the default `jennifer` binary
internal/lib/net/netlib_test.go            net library unit tests (!tinygo; loopback with :0 ephemeral ports; TCP round-trip, UDP round-trip, DNS, polymorphic close, use-after-close)
internal/lib/net/netlib_tls_test.go        net TLS tests (!tinygo; loopback self-signed cert; connectTLS round-trip, bad-cert reject, skipVerify, startTLS upgrade)
internal/lib/regex/regexlib.go             `regex` library: matches/find/findAll/replace/split/escape + regex.Match struct + 128-entry LRU pattern cache
internal/lib/regex/regexlib_test.go        regex library unit tests (predicate, positional + named groups, rune-index offsets, LRU behaviour under load, invalid pattern boundary)
internal/lib/testing/testinglib.go         `testing` library: run/runWith/results/reset/report + testing.Result struct + three renderers (text / TAP / JUnit XML) + exit interception
internal/lib/testing/testinglib_test.go    testing library unit tests (pass/fail/runtime/exit paths, accumulator, each of the three report formats, boundary errors)
internal/lib/testing/assertions.go         `testing` assertions: assertEqual/assertNotEqual/assertTrue/assertFalse/assertContains/assertThrows (throw Error{kind:"assertion"} at the call site)
internal/lib/testing/assertions_test.go    assertion + runWith unit tests
internal/lib/uuid/uuidlib.go               `uuid` library: generate("v4"/"v7") + parse/isValid/version + NIL; RFC 9562, hand-formatted hex, math shared RNG
internal/lib/uuid/uuidlib_test.go          uuid library unit tests (v4/v7 version+variant bits, v7 time-ordering via nowFunc, parse round-trip + invalids, dispatch)

internal/stdlib/stdlib.go                  InstallAll: single registration point for every system library; run/repl/profile/test all call it (seam for build-time selection)

internal/lint/lint.go                      `lint` checks: Diagnostic + grouped-ID registry (L0nn source / L1nn correctness / L2nn style / L3nn lifecycle) + KnownIDs/selectable/Catalog
internal/lint/run.go                       Check driver + threshold Config + source-error diagnostic builder
internal/lint/checks.go                    the individual check implementations
internal/lint/scope.go                     scope-aware traversal for binding-visibility checks (L101 unused-local, L104 throw-non-error)
internal/lint/walk.go                      flat AST walker for node-shape checks (L102/L103/L105/L201/L202)
internal/lint/config.go                    --checks / .jennifer-lint selection parsing (selectable IDs only)
internal/lint/suppress.go                  # lint-disable directive parsing + application; malformed/unknown-ID directive -> L004 finding
internal/lint/lint_test.go                 lint unit tests
internal/profile/profile.go                `profile` collector: per-position hit counts + wall-clock (+ optional alloc counting)
internal/profile/render.go                 table + Chrome-trace renderers
internal/profile/pprof.go                  pprof.gz output
internal/profile/profile_test.go           profile unit tests

internal/version/version.go                Default Version = "dev"
internal/version/version_gen.go            GENERATED by scripts/gen-version.sh (gitignored)
scripts/version.sh                         Computes the version string from git
scripts/gen-version.sh                     Writes version_gen.go before each build
Makefile                                   build (both binaries) / build-tinygo / build-go / test / clean / version

examples/*.j                               Example programs
examples/expected/*.txt                    Expected stdout per example (no entry == not in golden suite)
examples/with_import/                      Subdirectory demonstrating file imports
examples/showcase/                         Helpers spliced by showcase.j
```
