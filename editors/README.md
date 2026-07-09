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

- **VS Code**: create a minimal language extension whose
  `contributes.grammars` points at this file, mapping `source.jennifer` to the
  `.j` extension. (`yo code` scaffolds one; drop the grammar in and set
  `"language": "jennifer"`, `"extensions": [".j"]`.)
- **Sublime Text**: install a package that references the grammar, or convert
  it to a `.sublime-syntax`.
- **Zed**: reference the grammar from a language extension.

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

Emacs (`jennifer-mode.el`), a Sublime `.sublime-syntax`, a nano `.nanorc`, or a
tree-sitter grammar are all welcome. Keep the token classes aligned with the
files here: keywords, type keywords (`int float string bool bytes list map
task`), `$` variables, `UPPER_CASE` constants, `NS.name` calls, `#` / `/* */`
comments, and the numeric literal forms.
