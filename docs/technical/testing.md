# Testing

| Package                | What it tests                                                                                                                                                                                                          |
| ---------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `internal/lexer`       | Token-by-token output for fixed inputs; error cases                                                                                                                                                                    |
| `internal/parser`      | AST shape via `Sprint`; precedence; error cases                                                                                                                                                                        |
| `internal/interpreter` | Full programs in-memory; stdout captured                                                                                                                                                                               |
| `internal/lib/io`      | `Install` registers `printf`/`sprintf`; arity and format errors                                                                                                                                                        |
| `internal/lib/core`    | `JENNIFER_VERSION` constant matches `version.Version`; explicit `use core;` is rejected (core is auto-loaded)                                                                                                          |
| `cmd/jennifer`         | Golden test that runs every `examples/*.j` and compares stdout to `examples/expected/*.txt`; REPL `inputComplete` helper; AST-JSON validity; formatter idempotence + behavior preservation; cross-file error reporting |

Run everything with `go test ./...`.
