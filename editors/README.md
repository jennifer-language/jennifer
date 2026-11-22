# Editor support for Jennifer

Syntax-highlighting definitions for Jennifer (`.j`) source. Jennifer's lexical
rules are regular enough that highlighting is accurate: `$x` is always a
variable, `UPPER_CASE` a constant, `NS.name` a namespaced call, `#` / `/* */`
comments.

These files track the language spec (see [`../JENNIFER.md`](../JENNIFER.md) and
`docs/technical/grammar.md`). Jennifer is pre-1.0, so the grammar can still
change; if highlighting looks stale, check for an updated file here.

## Vim / Neovim (true drop-in)

Copy the two files into your runtime path:

```sh
mkdir -p ~/.vim/syntax ~/.vim/ftdetect        # Neovim: ~/.config/nvim/
cp vim/syntax/jennifer.vim   ~/.vim/syntax/
cp vim/ftdetect/jennifer.vim ~/.vim/ftdetect/
```

`.j` files are detected automatically. With a plugin manager, point it at the
`vim/` directory instead.

## VS Code / Sublime Text / JetBrains (TextMate grammar)

`textmate/jennifer.tmLanguage.json` is a TextMate grammar (scope
`source.jennifer`), consumed by any TextMate-compatible editor.

- **VS Code**: a ready-made minimal extension lives in [`vscode/`](vscode/) -
  copy or symlink that folder into your VS Code extensions directory (Linux /
  macOS / Windows steps in [`vscode/README.md`](vscode/README.md)), or open it and
  press <kbd>F5</kbd>. Its bundled `vscode/syntaxes/jennifer.tmLanguage.json` is a
  **byte-for-byte copy** of `textmate/jennifer.tmLanguage.json` (VS Code cannot
  load a grammar from outside the extension folder); when you change the TextMate
  grammar, copy it across so the two stay identical
  (`cp textmate/jennifer.tmLanguage.json vscode/syntaxes/`).
- **Sublime Text**: use the native `sublime/jennifer.sublime-syntax` (see
  below), or install a package that references this TextMate grammar.
- **JetBrains IDEs**: Settings, Editor, TextMate Bundles, add the `vscode/`
  folder.
- **Zed** and **Helix** read tree-sitter grammars only, not TextMate, so they
  have no Jennifer highlighting yet (see below).

## bat / Sublime Text (`.sublime-syntax`)

`sublime/jennifer.sublime-syntax` is a native Sublime Text syntax (scope
`source.jennifer`). Sublime Text reads it directly from a package; it is also
what [`bat`](https://github.com/sharkdp/bat) consumes.

`bat` compiles syntaxes into a per-user cache, so - unlike Vim/Neovim - it
cannot pick the file up from a system path automatically. Activate it once:

```sh
mkdir -p "$(bat --config-dir)/syntaxes"
cp sublime/jennifer.sublime-syntax "$(bat --config-dir)/syntaxes/"
bat cache --build
```

After that, `bat file.j` highlights as Jennifer (`bat --list-languages | grep
Jennifer` confirms it registered). The Jennifer packages ship this file at
`/usr/share/jennifer/syntaxes/jennifer.sublime-syntax`, so on a packaged
install copy it from there instead. `bat cache --clear` reverts.

## highlight.js (docs sites, blogs, static pages)

`highlightjs/jennifer.js` registers a `jennifer` language with
[highlight.js](https://highlightjs.org). Load it after highlight.js:

```html
<script src="highlight.min.js"></script>
<script src="jennifer.js"></script>
<script>hljs.highlightAll();</script>
```

Then fence code as ```` ```jennifer ````. It also works as a CommonJS module
(`require("./jennifer.js")`) for build pipelines.

## Chroma (Forgejo, Gitea, Hugo)

`chroma/jennifer.xml` is a lexer for [Chroma](https://github.com/alecthomas/chroma),
the Go highlighter behind Forgejo, Gitea and Hugo. It is a draft for an upstream
pull request: Chroma compiles its lexers in, so a server only highlights
Jennifer once its Chroma release carries the lexer. The PR adds the lexer, its
token test and three content-detection tests to a Chroma checkout, regenerates
the lexer table (`lexers/lexers_gen.go`) and adds `Jennifer` to the README's
language list:

```sh
cp chroma/jennifer.xml      <chroma>/lexers/embedded/
cp chroma/jennifer.actual   <chroma>/lexers/testdata/
cp chroma/jennifer.expected <chroma>/lexers/testdata/
cp chroma/analysis/*        <chroma>/lexers/testdata/analysis/
cd <chroma> && go generate ./lexers && go test ./...
```

After a lexer change, re-record the expected tokens with
`RECORD=true go test ./lexers -run 'TestLexers/Jennifer|TestLexersTextAnalyser'`
and copy the `.expected` files back here. The lexer uses the `words` rule
attribute of Chroma's `master` (v3); the `v2` branch, which Forgejo and Gitea
still build on, needs each `words="a b"` rule written as a
`pattern="\b(a|b)\b"` rule instead. Until a Forgejo release ships the lexer,
`[highlight.mapping]` `.j = php` in `app.ini` is a rough stand-in.

## Pygments (Sphinx, MkDocs, pygmentize)

`pygments/jennifer_lexer.py` is a lexer for
[Pygments](https://pygments.org), the Python highlighter behind Sphinx, MkDocs
and `pygmentize`, under BSD-2-Clause like Pygments itself. Pygments takes new
languages as plugins until they have a community of their own, so the lexer
ships as the `pygments-jennifer` package
([jennifer-language/pygments-jennifer](https://github.com/jennifer-language/pygments-jennifer)),
which registers it under the alias `jennifer` and the filename `*.j`. Its
`src/pygments_jennifer/lexer.py` is a byte-for-byte copy of the file here;
change it here and sync it across with that repository's `tools/sync.sh`.

To try the lexer without installing anything:

```sh
python -m pygments -x -l pygments/jennifer_lexer.py:JenniferLexer file.j
```

Two checks, both run from `pygments/`:

- `python golden.py` compares the tokens of `example.j` with `example.tokens`;
  after a deliberate change, `python golden.py --update` rewrites them.
- `python check.py DIR...` lexes every `.j` file under the given directories
  and compares the result with the interpreter's own lexer (`jennifer
  tokens`): no Error token, and every interpreter token starts a Pygments
  token of a matching class.

## A note on `.j` and GitHub

GitHub's Linguist assigns `.j` to Objective-J, so GitHub will not highlight
Jennifer source as Jennifer in the web UI. That is a GitHub-side limitation;
the definitions here work in local editors and self-hosted sites regardless.

## Contributing another editor

Emacs (`jennifer-mode.el`), a nano `.nanorc`, or a tree-sitter grammar (Zed,
Helix, Neovim) are all
welcome. Keep the token classes aligned with the files here:

- keywords, and the type keywords `int float string bool bytes list map task`,
  plus `channel` in `channel of T`;
- `$` variables, `UPPER_CASE` constants, `CamelCase` type names, and `name(`
  calls;
- `NS.name` calls: the prefix is a namespace (`task.` and `channel.` too), as is
  the name after `use` and an alias (`use strings as str;`);
- a keyword-spelled name is a name after a `.` (`strings.repeat`, `json.map`,
  `$e.to`) and in a field position (`to as Point`, `Written{bytes: 0}`);
- `#` comments, nested `/* */` comments, `/** */` docblocks with `@tags`, the
  `# pragma-jennifer-...:` and `# lint-disable:` / `# lint-disable-file:`
  directives, and a `#!... jennifer` first line for extensionless scripts;
- raw `'...'` strings (no escapes, no slots, may span lines) and cooked
  `"..."` strings with their escapes (`\n \r \t \\ \" \' \0 \{ \} \uXXXX
  \UXXXXXXXX`) and `{expr}` slots: a slot may hold a `{...}` map literal, so
  count braces, and strings of either kind;
- the numeric forms: `0x` / `0o` / `0b` integers, decimal integers and floats
  with an optional exponent, and `_` between digits.

Each definition here was checked against the interpreter's own lexer
(`jennifer tokens`) over every `.j` file in the workspace.
