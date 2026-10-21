# Preprocessor (`internal/preproc`)

Sits between the lexer and the parser. Its job is to expand `include`
file splices and pass `use` (library) and `import` (module) statements
through unchanged (after checking them for common mistakes).

## Algorithm

1. Strip trivia tokens (comments, blank lines) so the recognizers can
   rely on adjacent tokens being meaningful.
2. Walk the token stream.
3. When `INCLUDE STRING SEMI` is found (a textual file splice):
   - Verify the string ends in `.j`.
   - Resolve the path (relative to the current file's directory, or
     absolute if it starts with `/`).
   - Reject if the path was already visited up the include chain
     (circular include).
   - Read the file (refused above `limits.MaxSourceBytes`), check its own
     `# pragma-jennifer-*` requirement header, lex it (with file-tagged
     tokens), recursively preprocess it. Each file is read and tokenized
     once per run (a per-path cache), include recursion is capped at
     `limits.MaxNestingDepth`, and the total spliced tokens at
     `maxSplicedTokens`, so an exponential include chain is a positioned
     error rather than an out-of-memory kill.
   - Splice the result (dropping the trailing `EOF`) at this point.
4. When `IMPORT ...` is found (a module import): pass the tokens through
   unchanged. `import` is a real statement handled by the parser
   (`ModuleImportStmt`) and interpreter (the module loader), not a
   textual splice. The one thing checked here is the common unquoted
   mistake (`validateModuleImport`).
5. When `USE IDENT SEMI` is found: pass through unchanged. The parser
   turns it into an `ImportStmt` node.
6. Helpful errors for common mistakes:
   - `include IDENT;` (library form with `include`) -> "use `use NAME;`
     for system libraries".
   - `include IDENT.j;` (unquoted file form) -> "file splices take a
     string literal".
   - `include "foo.go";` (wrong extension) -> "include path must end
     with `.j`".
   - `import IDENT;` (library form with `import`) -> "`import` takes a
     quoted module path; for a system library use `use NAME;`".
   - `import IDENT.j;` (unquoted module path) -> "module paths are
     quoted: `import \"name.j\";`".
   - `use IDENT.j;` (file form with `use`) -> "for files use
     `include \"name.j\";`".

## Edge cases

- An `include` path must literally end in `.j`. `include "foo.go";` is
  rejected.
- `include` paths may contain `/` for subdirectories. Absolute paths are
  accepted as-is; relative paths resolve from the including file's
  directory.
- Circular includes are detected by tracking absolute paths visited
  along the current chain. (A module `import` cycle is detected
  separately, by the loader in `internal/interpreter`; the preprocessor
  never opens an imported module - it only forwards its tokens.)
