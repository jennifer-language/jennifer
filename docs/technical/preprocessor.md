# Preprocessor (`internal/preproc`)

Sits between the lexer and the parser. It expands `include` file
splices and passes `use` (library) and `import` (module) statements
through unchanged, after checking them for common mistakes.

## Algorithm

1. Strip trivia tokens (comments, blank lines) so the recognizers can
   rely on adjacent tokens being meaningful.
2. Walk the token stream.
3. When `INCLUDE STRING SEMI` is found (a textual file splice):
   - Verify the string ends in `.j`.
   - Resolve the path (relative to the current file's directory, or
     absolute if it starts with `/`) to its canonical, symlink-free form.
   - Reject it if that path is already on the current include chain
     (circular include).
   - Read the file (refused above `limits.MaxSourceBytes`), check its own
     `# pragma-jennifer-*` requirement header, lex it (with file-tagged
     tokens) and preprocess it recursively. Each file is read and
     tokenized once per run (a per-path cache). Include recursion is
     capped at `limits.MaxNestingDepth` and the total spliced tokens at
     `maxSplicedTokens` (2,000,000), so an exponential include chain is a
     positioned error rather than an out-of-memory kill.
   - Splice the result (dropping the trailing `EOF`) at this point.
4. When `IMPORT ...` is found (a module import): pass the tokens through
   unchanged. `import` is a statement handled by the parser
   (`ModuleImportStmt`) and the interpreter's module loader, not a
   textual splice. Only the unquoted-path mistake is checked here
   (`validateModuleImport`).
5. When `USE IDENT SEMI` is found: pass it through unchanged. The parser
   turns it into an `ImportStmt` node.
6. Report common mistakes:

| Input                              | Error                                                                    |
| ---------------------------------- | ------------------------------------------------------------------------ |
| `include IDENT;` (library form)    | `` `include` is for files; use `use NAME;` for system libraries ``       |
| `include IDENT.j;` (unquoted file) | `` file splices take a string literal: `include "NAME.j";` ``             |
| `include "foo.go";`                | `` include path "foo.go" must end with `.j` ``                           |
| `import IDENT;` (library form)     | `` `import` takes a quoted module path (`import "NAME.j";`); for a system library use `use NAME;` `` |
| `import IDENT.j;` (unquoted path)  | `` module paths are quoted: `import "NAME.j";` ``                         |
| `use IDENT.j;` (file form)         | `` `use` is for system libraries; for files use `include "NAME.j";` `` |

## Edge cases

- An `include` path must literally end in `.j`.
- `include` paths may contain `/` for subdirectories. Absolute paths are
  accepted as-is; relative paths resolve from the including file's
  directory.
- Circular includes are detected by tracking the canonical paths on the
  current chain, so a symlink and its target count as one file. A module
  `import` cycle is detected separately, by the loader in
  `internal/interpreter`; the preprocessor never opens an imported
  module.
