# Editor support and AI-assisted coding

Two things make writing Jennifer outside this repo comfortable: syntax
highlighting in your editor, and a drop-in language reference so an AI coding
assistant can write correct Jennifer for you. Both ship in the repository.

## Editor syntax highlighting

Highlighting definitions live in
[`editors/`](https://github.com/jennifer-language/jennifer/tree/main/editors). Jennifer's
lexical rules are regular enough that highlighting is genuinely accurate - `$x`
is always a variable, `UPPER_CASE` a constant, `NS.name` a namespaced call, `#`
and `/* */` comments.

- **Vim / Neovim** - a true drop-in. Copy
  [`editors/vim/syntax/jennifer.vim`](https://github.com/jennifer-language/jennifer/blob/main/editors/vim/syntax/jennifer.vim)
  and
  [`editors/vim/ftdetect/jennifer.vim`](https://github.com/jennifer-language/jennifer/blob/main/editors/vim/ftdetect/jennifer.vim)
  into `~/.vim/` (or `~/.config/nvim/`); `.j` files are detected automatically.
- **VS Code / Sublime Text / Zed** - use the TextMate grammar
  [`editors/textmate/jennifer.tmLanguage.json`](https://github.com/jennifer-language/jennifer/blob/main/editors/textmate/jennifer.tmLanguage.json)
  (scope `source.jennifer`) from a thin language extension.
- **`bat` / Sublime Text** - the native
  [`editors/sublime/jennifer.sublime-syntax`](https://github.com/jennifer-language/jennifer/blob/main/editors/sublime/jennifer.sublime-syntax).
  For `bat`, copy it into `$(bat --config-dir)/syntaxes/` and run
  `bat cache --build` (it caches syntaxes per user, so a system path can't
  auto-activate it).
- **Static sites / blogs** - the
  [highlight.js definition](https://github.com/jennifer-language/jennifer/blob/main/editors/highlightjs/jennifer.js)
  registers a `jennifer` language.

Per-editor install steps are in
[`editors/README.md`](https://github.com/jennifer-language/jennifer/blob/main/editors/README.md).

One caveat: GitHub's Linguist assigns the `.j` extension to Objective-J, so
GitHub's web UI will not highlight Jennifer source as Jennifer. That is a
GitHub-side limitation; local editors and self-hosted sites are unaffected.

## Jennifer as a shell filter

`jennifer run -` reads a program from stdin, so Jennifer slots into a pipe
like any other filter. A handy one is a `json-pretty` that reformats JSON
flowing through it. Save the program to a file (say
`~/.local/share/jennifer/json-pretty.j`):

```jennifer
use json;
use io;

def src as string init "";
while (not io.eof()) {
    $src = $src + io.readLine() + "\n";
}
io.printf("%s\n", json.encodePretty(json.decode($src)));
```

then alias it:

```sh
alias json-pretty='jennifer run ~/.local/share/jennifer/json-pretty.j'

echo '{"b":2,"a":1}' | json-pretty
curl -s https://api.example.com/thing | json-pretty
```

Swap `json` for any other decode / re-encode pair to get, for example, a
`pretty-xml`. A no-file variant that pipes the program itself through
`jennifer run -` is in the
[CLI reference](../technical/cli.md#shell-pipelines-and-aliases).

## AI-assisted coding with `JENNIFER.md`

Jennifer is new and small, so a general-purpose AI assistant has no built-in
knowledge of it and will otherwise guess (usually Python-with-dollar-signs).
[`JENNIFER.md`](https://github.com/jennifer-language/jennifer/blob/main/JENNIFER.md) is a
single, self-contained language reference written for exactly this: drop it into
your project and point your assistant at it.

```text
We're coding in Jennifer, a batteries-included interpreted language. Read
JENNIFER.md for the syntax and standard library, then let's build ...
```

It covers the lexical rules (the `$` sigil, letters-only identifiers,
`UPPER_CASE` constants), types, operators (including `/` being float division),
control flow, methods, concurrency, imports, the namespaced standard library,
and a checklist of the mistakes an assistant most often makes. It describes the
*language*, not the interpreter internals, and stays in sync with this spec.

It doubles as a human quick-reference. For the exhaustive per-function detail
behind it, see the [library reference](../libraries/index.md) and
[cheatsheet](../libraries/cheatsheet.md).
