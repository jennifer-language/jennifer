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
| `L104` | throw-non-error             | warning  | a `throw` whose value isn't statically an `Error`: an `Error{...}` literal, an `Error`-typed variable / parameter / catch binding, or a call to a same-file function that always returns one (every `return` is one of these or such a call, and the body cannot fall off its end) |
| `L105` | constant-condition          | warning  | `if (true)`, `while (true)` with no escape, `if ($x == $x)`, ...   |
| `L106` | unused-import               | warning  | a `use` / `import` whose namespace is never referenced (call, constant, value, or type)  |
| `L107` | undefined-call              | warning  | an unqualified `foo(...)` whose name is not a defined function (the call analogue of L002; namespaced / `$f()` calls excluded) |
| `L108` | undefined-name              | warning  | an unqualified bare name read where nothing by that name is defined anywhere: a constant (`MISSING_CONSTANT`), or a function used as a value (`Table{ greet: greetingOf }`); a bare variable / parameter name gets a "did you mean `$x`?" hint. The bare-name analogue of L107; `ns.NAME` and `match` arm heads / pattern binders excluded |
| `L109` | call-arity                  | warning  | a call with the wrong argument count to a same-file function (exact: no default / variadic params), an imported module's exported function (`alias.fn(...)`), or a library builtin (optional / variadic counts from its `libsig` signature); func-value calls excluded |
| `L110` | argument-type               | warning  | a call argument whose static type certainly differs from the parameter's - a same-file function's, an imported module's exported function's, or a library builtin's signature; arguments of unknown type (a user function's result, a func-value call) are skipped |
| `L111` | binding-type                | warning  | a value whose static type certainly differs from the declared type of where it is stored: a `def ... init`, a `$x = ...`, a struct field write or literal field, a list / map element write, a `$xs[] = ...` append |
| `L112` | undeclared-namespace        | warning  | a `ns.name` reference whose namespace has no `use` / `import` (the inverse of L106; local enum / struct type prefixes excluded) |
| `L113` | struct-literal-fields       | warning  | a struct / enum-variant literal - of a same-file type, or an imported module's exported one (`alias.T{...}`, `alias.E.V{...}`) - naming a field the type lacks, leaving one out, or naming an unknown variant (library types are checked at load by `run`) |
| `L114` | unknown-member              | warning  | a `ns.name` naming a function, constant, or type that its library does not register (`time.millis(10)`, `math.PII`, `def t as time.Tme`) or its imported module does not export (unknown, or private to it), with a "did you mean" hint for a near miss; a surface that is not available is skipped |
| `L115` | unknown-library             | warning  | a `use NAME;` naming a library the build does not install (`use strngs;`, `use core;`), with a "did you mean" hint for a near miss; skipped when no library list is supplied |
| `L201` | function-too-long           | info     | function body over the statement threshold (default 60)              |
| `L202` | nesting-too-deep            | info     | block nesting over the depth threshold (default 4)                 |
| `L203` | line-too-long               | info     | a source line over the column limit (default 100); a comment line that cannot wrap (a Markdown table row, or overflow that is one URL / code span too long for a line of its own) is exempt |
| `L204` | interpolation-slot-too-complex | info  | a call or other non-trivial expression inside a `{expr}` string slot |
| `L205` | accumulator-parameter       | info     | a function that takes, mutates, and returns a `list` / `map` parameter, or grows a struct / `bytes` parameter with `[] =` and returns it (an O(n^2) copy-in / copy-out accumulator, unless every call is `$v = f($v, ...)` on a function local outside a `try`); or one that copies a parameter into a local (`def out as T init $p`), grows the local with `[] =`, and returns it (O(n^2) in a loop however it is called). A struct whose field is only set (`$c.timeout = $ms`) is the builder idiom and is not flagged. |
| `L301` | deprecation                 | warning  | reserved family, empty until an API is deprecated                  |
| `L302` | removed-api                 | warning  | reserved family, empty until a removal warrants a dev-time notice  |
| `L303` | invalid-requirement-header  | warning  | a malformed / duplicate / unknown `# pragma-jennifer-*` directive    |

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
checks that match node shapes (L102/L103/L106/L107/L108/L109/L112/L113/L114/L201/L204/L105 - L106 and
L114 also walk declared types, which the node walker does not visit, to catch a
namespace used only in a type annotation), and a
scope-aware traversal (`scope.go`) mirroring the resolver's frame model
for the checks that need binding visibility (L101/L104/L110/L111). Both descend
into `SpawnExpr.Body`, which the resolver deliberately skips: a read
inside a spawn still marks an outer local used, but a *declaration*
inside a spawn is left unreported (the resolver's spawn carve-out, which
the linter inherits). The checks in `internal/lint` run on the parsed AST
and track their own bindings and declared types; they do not read the
resolver's `(Depth, Slot)` annotations. The CLI does run `parser.Resolve`
first (`lintComputeDiags`), so an undefined name, a shadowing, or an enum
`match` error that `run` would refuse surfaces as an `L002` source finding
rather than a clean lint. L202 and L205 walk statements with their own
small recursions (nesting depth, accumulator scan) instead of either
traversal.

**Format-honest errors.** A lex / preprocess / parse failure is not a
stderr bail-out: `lintComputeDiags` turns it into an `L0nn` source
finding that renders in whatever `--format` was asked for, so a
`--format=json` pipeline always receives valid output saying why the file
couldn't be checked. `stripPositionPrefix` peels the `FILE:LINE:COL:`
that the pipeline errors embed, since the finding carries those as
fields.

**Module-overlay splice.** Linting a `MODULE_test.j` white-box overlay
splices the sibling `MODULE.j` in front of the test file before parsing -
the same splice `jennifer test` applies (`overlayBaseFor` /
`tokenizeForSplice` / `spliceTokens`) - so the scope analysis
`lintComputeDiags` runs (`parser.Resolve`: undefined names, shadowing,
enum-match binding and exhaustiveness) sees the module's private consts /
structs / enums / functions. Without it the resolver reports false positives
the test run never hits: e.g. a `when Variant(bind)` arm over a
module-private enum goes unrecognised, so the bound name reads as an
undefined variable (`L002`). Findings that land in the spliced base module
(or its own includes) are dropped from the overlay's report - they belong
to `lint MODULE.j`, so a module's findings are never double-counted. A file
that is not a `*_test.j` overlay, or whose sibling module is absent, is
linted unchanged.

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
them to findings by `file`/`line`. A directive must be its own `#` comment
token, so a line inside a multi-line token cannot carry one; a line directive
therefore also covers every line of a multi-line string or block comment
that **ends** on its line (`'; # lint-disable: L203`), and a directive
standing alone on its line covers the whole block comment (a `/** */`
docblock) that **starts** on the next line. Text inside a docblock is never a
directive (it is documentation, rendered by the docs tooling), so it is not
reported as `L004` either. A line comment may **end** in a directive
(`# long note # lint-disable: L203`, since a comment has no room for a second
comment token); that embedded form counts only when it parses as a line directive, so prose
that mentions a directive stays an ordinary comment rather than an `L004`, and
an embedded `lint-disable-file` is not honoured (a file-wide directive belongs
at the file head). A malformed or unknown-ID directive is
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

**Imported modules.** `lint` resolves each `import` exactly as `run` does -
relative to the linted file's directory, then the system module dir and any
`-I DIR`, `@` decks under the vendor root - taking the same `--sysmoddir DIR`,
`-I DIR` (repeatable) and `--vendor DIR` flags. Each resolved module is parsed
(`loadModuleProgram`, with its own requirement header checked) and handed to the
checks in `Config.Modules`, keyed by the prefix the import binds, so `L109`,
`L110`, `L111`, `L113` and `L114` check a call, literal or type into it against
its **exported** surface (a module's own struct type reads as `alias.T` from the
importing file, a library type it names through its own `use` as the canonical
`lib.T`). A
module that does not resolve or parse is left out silently - `run` reports that -
so the checks stay false-positive-free; an unexported or unknown member of a
loaded module is `L114`. CI passes
`--sysmoddir modules` so the bare imports of the shipped examples resolve.

**Static types (L110 / L111).** `types.go` is a deliberately small static
typer: it names the type an expression *certainly* has - a literal, a reference
to a binding with a declared type (parameter, `def`, constant, a for-each
iterator over a typed list / map), a field / index / slice of one, an operator
over typed operands (`int + int` is int, a float operand makes float, `/` is
float, a comparison is bool), a struct or enum literal, a range (`list of int`),
or a library call whose signature names a single return type - and answers
"unknown" for everything else, above all a user function's result (functions declare
no return type) and a func-value call. An unknown type never produces a finding.
Types compare as the runtime's `MatchesDeclared` does (strict kinds: an `int` is
not a `float`; struct identity by name; a stamped collection's element type must
match), so a finding is a call or store the runtime would reject; a list / map
literal is checked element by element, the way the runtime validates a generic
collection. Library builtins are the one exception to element checking: a
builtin validates a collection's elements one by one, so an empty `list of int`
passes a builtin that wants strings, and a library parameter's element type is
compared only against literal elements. Struct identity is normalised to the
linted file's terms (no prefix for a same-file type, the canonical library name
for a library type whatever alias it is `use`d under, the import prefix for a
module type); a name the typer cannot place makes the type unknown. The CLI
supplies the library's registered struct shapes and constant types
(`Config.LibStructs` / `LibConsts`, read once from an interpreter with every
library installed), so a library struct's fields (`$size.rows`) and constants
(`math.PI`) are typed too.

**Library signatures (`internal/libsig`).** Builtins validate their own
arguments at runtime and carry no static signature, so the arity, argument
types and return type of every builtin live in a declarative side table: one
file per library, each a text block of `name(T1, T2?, T3...) R` lines in
Jennifer's type syntax (`any` for any value, `A|B` for a union, `?` optional,
`...` variadic; a builtin that dispatches on its first argument gets one line
per shape). Registration stays a name and a function, and the table compiles to
strings the tiny binary never links (only `lint` reads it). A signature is never
stricter than its builtin: lint reports only when no line accepts a call, so an
uncertain parameter is `any` and a return whose kind varies is `any` or a union
(which lint treats as unknown); a list return names its element type only when
the builtin stamps one. `TestLibsigCoversInstalledBuiltins`
(`internal/stdlib`) keeps the table in lock-step with `InstallAll`: every line
parses, every registered builtin has a line, every line names a registered
builtin, and every library type it mentions is registered. **A new builtin needs
its `libsig` line.** Because the table is complete, `L114` reads it as
the list of a library's functions: a call to a name with no line is unknown.
A library's constants and types come from the interpreter's registered tables
(`Config.LibConsts` / `LibStructs`, filled by the CLI).

**Include fragments.** A file named `NAME.inc.j` is an include fragment: a part
that is only ever `include`d, free to call functions and read constants and
namespaces its includer defines. Linted on its own (`jennifer lint modules/*.j`
picks the fragments up), the undefined-name checks would flag every such borrowed
name, so the CLI drops `L107`, `L108` and `L112` for a fragment and prints one
stderr note pointing at the including file; every other check still runs. Linting
the including file covers the fragment fully, since its tokens are spliced in and
findings anchor to the fragment's own lines.

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
