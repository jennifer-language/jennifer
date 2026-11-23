# Editor support and AI-assisted coding

The repository ships syntax highlighting for common editors and a drop-in
language reference that lets an AI coding assistant write correct Jennifer.

## Editor syntax highlighting

Highlighting definitions live in
[`editors/`](https://github.com/jennifer-language/jennifer/tree/main/editors). The
lexical rules are regular, so highlighting is exact: `$x` is always a variable, `UPPER_CASE` a constant, `NS.name` a namespaced call, `#`
and `/* */` comments.

- **Vim / Neovim** - copy
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

GitHub's Linguist assigns the `.j` extension to Objective-J, so GitHub's web UI
does not highlight Jennifer source as Jennifer; local editors and self-hosted
sites are unaffected.

## Jennifer as a shell filter

A Jennifer program works as a filter in a pipe. For example, a `json-pretty`
that reformats the JSON flowing through it; save it to a file (say
`~/.local/share/jennifer/json-pretty.j`):

```jennifer
use io;
use json;
use strings;

def src as string init strings.join(io.readLines(), "\n");
io.printf("%s\n", json.encodePretty(json.decode($src)));
```

then alias it:

```sh
alias json-pretty='jennifer run ~/.local/share/jennifer/json-pretty.j'

echo '{"b":2,"a":1}' | json-pretty
curl -s https://api.example.com/thing | json-pretty
```

Swap `json` for another decode / re-encode pair to get, for example, a
`pretty-xml`. A no-file variant that pipes the program itself through
`jennifer run -` (which reads a program from stdin) is in the
[CLI reference](../technical/cli.md#shell-pipelines-and-aliases).

## Run profiles (`--env`)

`jennifer run --env=<profile> app.j` selects a **run profile** for a program -
`prod`, `dev`, `staging`, or whatever names you use. The flag only sets the
`JENNIFER_ENV` environment variable before the program runs, so

```sh
jennifer run --env=prod app.j
```

is identical to

```sh
JENNIFER_ENV=prod jennifer run app.j
```

The main consumer is the [`dotenv`](../modules/dotenv.md) module, whose
`autoload` / cascade loaders layer the `.env.<profile>` file on top of `.env`:
`--env=prod` loads `.env.prod`, and no profile loads only the base files.

The profile label must match `[A-Za-z0-9_-]`, 1 to 64 characters, so a hostile
value cannot point `.env.<profile>` at another path.
The flag goes **before** the script file (like `-I` and `--sysmoddir`); anything
after the file is passed to the program as its own arguments. An explicit
`--env=X` overrides any `JENNIFER_ENV` already in the environment.

## AI-assisted coding with `JENNIFER.md`

A general-purpose AI assistant has no built-in knowledge of Jennifer and
guesses (usually Python with dollar signs).
[`JENNIFER.md`](https://github.com/jennifer-language/jennifer/blob/main/JENNIFER.md) is a
self-contained language reference for it: drop it into your project and point
your assistant at it.

```text
We're coding in Jennifer, a batteries-included interpreted language. Read
JENNIFER.md for the syntax and standard library, then let's build ...
```

It covers the lexical rules (the `$` sigil, letters-only identifiers,
`UPPER_CASE` constants), types, operators (including `/` being float division),
control flow, functions, concurrency, imports, the namespaced standard library,
and a checklist of the mistakes an assistant most often makes. It describes the
*language*, not the interpreter internals. Its first lines tell the assistant how
to keep the copy in sync with the installed `jennifer version`.

It doubles as a quick reference for people. For per-function detail, see the
[library reference](../libraries/index.md) and
[cheatsheet](../libraries/cheatsheet.md).
