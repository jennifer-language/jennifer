# Editor support and AI-assisted coding

Two things make writing Jennifer outside this repo comfortable: syntax
highlighting in your editor, and a drop-in language reference so an AI coding
assistant can write correct Jennifer for you. Both ship in the repository.

## Editor syntax highlighting

Highlighting definitions live in
[`editors/`](https://github.com/mplx/jennifer-lang/tree/main/editors). Jennifer's
lexical rules are regular enough that highlighting is genuinely accurate - `$x`
is always a variable, `UPPER_CASE` a constant, `NS.name` a namespaced call, `#`
and `/* */` comments.

- **Vim / Neovim** - a true drop-in. Copy
  [`editors/vim/syntax/jennifer.vim`](https://github.com/mplx/jennifer-lang/blob/main/editors/vim/syntax/jennifer.vim)
  and
  [`editors/vim/ftdetect/jennifer.vim`](https://github.com/mplx/jennifer-lang/blob/main/editors/vim/ftdetect/jennifer.vim)
  into `~/.vim/` (or `~/.config/nvim/`); `.j` files are detected automatically.
- **VS Code / Sublime Text / Zed** - use the TextMate grammar
  [`editors/textmate/jennifer.tmLanguage.json`](https://github.com/mplx/jennifer-lang/blob/main/editors/textmate/jennifer.tmLanguage.json)
  (scope `source.jennifer`) from a thin language extension.
- **Static sites / blogs** - the
  [highlight.js definition](https://github.com/mplx/jennifer-lang/blob/main/editors/highlightjs/jennifer.js)
  registers a `jennifer` language.

Per-editor install steps are in
[`editors/README.md`](https://github.com/mplx/jennifer-lang/blob/main/editors/README.md).

One caveat: GitHub's Linguist assigns the `.j` extension to Objective-J, so
GitHub's web UI will not highlight Jennifer source as Jennifer. That is a
GitHub-side limitation; local editors and self-hosted sites are unaffected.

## AI-assisted coding with `JENNIFER.md`

Jennifer is new and small, so a general-purpose AI assistant has no built-in
knowledge of it and will otherwise guess (usually Python-with-dollar-signs).
[`JENNIFER.md`](https://github.com/mplx/jennifer-lang/blob/main/JENNIFER.md) is a
single, self-contained language reference written for exactly this: drop it into
your project and point your assistant at it.

```text
We're coding in Jennifer, a small interpreted language. Read JENNIFER.md
for the syntax and standard library, then let's build ...
```

It covers the lexical rules (the `$` sigil, letters-only identifiers,
`UPPER_CASE` constants), types, operators (including `/` being float division),
control flow, methods, concurrency, imports, the namespaced standard library,
and a checklist of the mistakes an assistant most often makes. It describes the
*language*, not the interpreter internals, and stays in sync with this spec.

It doubles as a human quick-reference. For the exhaustive per-function detail
behind it, see the [library reference](../libraries/index.md) and
[cheatsheet](../libraries/cheatsheet.md).
