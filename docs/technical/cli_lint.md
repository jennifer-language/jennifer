# Linter (`cmd/jennifer/lint.go`, `internal/lint`)

`jennifer lint <file.j>` reports patterns that are compile-legal but
stylistically or semantically suspect - the slot between `fmt` (which
normalises lexical shape) and the parser (which rejects the outright
illegal). The checks live in `internal/lint`; the subcommand in
`cmd/jennifer/lint.go` wraps them with file I/O, config resolution, and
output rendering.

The check set is grouped by concern, each check with a stable ID so
suppression and configuration stay portable and greppable. The leading
digit is the group: **L0nn** source errors (the file doesn't parse, or a
directive is malformed), **L1nn** correctness, **L2nn** complexity and
style, **L3nn** API lifecycle.

| ID     | Check                       | Severity | Flags                                                              |
| ------ | --------------------------- | -------- | ------------------------------------------------------------------ |
| `L001` | lex-error                   | error    | the source could not be tokenized                                  |
| `L002` | parse-error                 | error    | the source could not be parsed                                     |
| `L003` | preproc-error               | error    | an `include` could not be spliced                                  |
| `L004` | invalid-directive           | error    | a malformed or unknown-ID `# lint-disable` comment                 |
| `L101` | unused-local                | warning  | a local `def` binding never read (skips spawn-body declarations)   |
| `L102` | dead-code-after-terminator  | warning  | a statement after `return`/`throw`/`exit`/`break`/`continue`       |
| `L103` | empty-catch                 | warning  | a `catch` block with no body                                       |
| `L104` | throw-non-error             | warning  | a `throw` whose value isn't statically an `Error`                  |
| `L105` | constant-condition          | warning  | `if (true)`, `while (true)` with no escape, `if ($x == $x)`, ...   |
| `L106` | unused-import               | warning  | a `use` / `import` whose namespace is never referenced (call, constant, value, or type)  |
| `L201` | method-too-long             | info     | method body over the statement threshold (default 60)              |
| `L202` | nesting-too-deep            | info     | block nesting over the depth threshold (default 4)                 |
| `L203` | line-too-long               | info     | a source line over the column limit (default 100)                  |
| `L301` | deprecation                 | warning  | reserved family, empty until an API is deprecated                  |
| `L302` | removed-api                 | warning  | use of a removed API (e.g. `use core;`)                            |

The **L0nn source errors are always on and not user-selectable**: they
are produced by the pipeline (lex / preprocess / parse) or the
suppression pass (`L004`), not by an AST walk, so `--checks` can't enable
or exclude them and they carry a nil `run`. `registry` in `lint.go` marks
every other check `selectable`; `selectableIDs()` is what `--checks`
resolves against, `KnownIDs()` (all IDs) is what directive/config
validation checks against. Adding a check takes the next free number in
its group; a new group (say `L4nn` for a portability family) is a new
leading digit.

**Traversal.** The parser exposes no generic visitor, so `internal/lint`
carries two: a flat `walker` (`walk.go`) with list/stmt/expr hooks for
checks that match node shapes (L102/L103/L106/L201/L202/L105 - L106 also
walks declared types, which the node walker does not visit, to catch a
namespace used only in a type annotation), and a
scope-aware traversal (`scope.go`) mirroring the resolver's frame model
for the checks that need binding visibility (L101/L104). Both descend
into `SpawnExpr.Body`, which the resolver deliberately skips: a read
inside a spawn still marks an outer local used, but a *declaration*
inside a spawn is left unreported (the resolver's spawn carve-out, which
the linter inherits). The linter runs on the parsed AST alone - it does
not call `parser.Resolve`, so it can lint code that would fail
resolution, and it tracks its own bindings and declared types.

**Format-honest errors.** A lex / preprocess / parse failure is not a
stderr bail-out: `lintComputeDiags` turns it into an `L0nn` source
finding that renders in whatever `--format` was asked for, so a
`--format=json` pipeline always receives valid output saying why the file
couldn't be checked. `stripPositionPrefix` peels the `FILE:LINE:COL:`
that the pipeline errors embed, since the finding carries those as
fields.

**Severity and exit code.** A finding at or above `SeverityFloor`
(warning) makes the run exit 1; an info-only run exits 0. Exit 2 is
reserved for an invocation failure with no source position - bad flags,
unreadable file, or a bad `--checks` / `.jennifer-lint` - which prints to
stderr. A source error (`L0nn`, severity error) is a finding, so it exits
1, not 2. Same triaging shape as `gofmt -l` / `shellcheck`.

**Suppression.** `# lint-disable: L101` (trailing) silences an ID on that
line; `# lint-disable-file: L101, L102` silences file-wide. There is no
blanket disable-all - a directive names IDs, on the line the finding
anchors to (the `func` line for `L201`, the block-introducer line for
`L202`). Because the parser strips comments, `applySuppressions` reads
directives off the raw `lexer.TokenizeWithFile` stream and correlates
them to findings by `file`/`line`. A malformed or unknown-ID directive is
**continue-and-report**: it becomes an `L004` finding, suppresses nothing
(so the finding it meant to silence still surfaces), and the run keeps
going. A doubled marker (`## lint-disable: ...`) is an ordinary comment,
not a directive.

Selection and suppression are orthogonal layers, and suppression always
wins locally: `--checks` gates which checks *run*, then
`applySuppressions` filters the findings they produced. So
`--checks=L203` with a `# lint-disable: L203` on one line runs L203
everywhere but silences that one line - suppression can only ever remove
findings, never add them, so there is no conflict to resolve.

**Configuration.** `--checks=IDS` (per run) or a `.jennifer-lint` file at
the tree root (per project) select checks with one `IDS` / `!IDS`
direction - all includes ("run only these") or all excludes ("run
everything except"); mixing is an error. Unknown IDs are always an error;
naming an always-on `L0nn` source error in `--checks` is rejected too.
Error messages are terse - `unknown check ID "L999"`, no catalog dump;
`jennifer lint --help` lists the catalog. `--format=human|json|github`
picks the output shape: positioned carets (reusing `printErrorContextTo`),
a JSON array of `{id,file,line,col,message,severity}` objects, or GitHub
Actions annotations. **Multi-file `--format=json` aggregates every file's
findings into one array** (a stream of per-file `[...]` documents would
not be valid JSON); human and github stream per file.

**TinyGo.** The subcommand is build-tag split: `lint.go` (`!tinygo`)
carries the real implementation and is the only importer of
`internal/lint`, so the whole AST-walking machinery stays out of the
`jennifer-tiny` binary; `devtools_tinygo.go` (`tinygo`) stubs `runLint`
to a friendly pointer at the default `jennifer` binary, alongside the
other dev-tool stubs, mirroring the `os.run` / `net` pattern.


Part of the [CLI reference](cli.md).
