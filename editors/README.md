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

## VS Code / Sublime Text / Zed (TextMate grammar)

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
- **Zed**: reference the grammar from a language extension.

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

## A note on `.j` and GitHub

GitHub's Linguist assigns `.j` to Objective-J, so GitHub will not highlight
Jennifer source as Jennifer in the web UI. That is a GitHub-side limitation;
the definitions here work in local editors and self-hosted sites regardless.

## Contributing another editor

Emacs (`jennifer-mode.el`), a nano `.nanorc`, or a tree-sitter grammar are all
welcome. Keep the token classes aligned with the
files here: keywords, type keywords (`int float string bool bytes list map
task`), `$` variables, `UPPER_CASE` constants, `NS.name` calls, `#` / `/* */`
comments, and the numeric literal forms.
