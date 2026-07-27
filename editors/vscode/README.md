# Jennifer for VS Code

Syntax highlighting for the [Jennifer](https://jennifer-lang.dev/) programming
language (`.j` source files). A minimal, declarative extension: it contributes a
`jennifer` language and a TextMate grammar (scope `source.jennifer`) - no
activation code, no dependencies.

It highlights the whole lexical surface: `#` and nested `/* */` comments, single-
and double-quoted strings with escapes, every numeric form (`0xFF`, `0o755`,
`0b1010`, `1_000`, `3.14`), keywords and type keywords, `$variables`,
`UPPER_CASE` constants, `NS.name` namespaced calls, and operators.

## Install

Copy (or symlink) this folder into your VS Code extensions directory, then reload
the window. The commands per OS:

### Linux

```sh
cp -r editors/vscode ~/.vscode/extensions/jennifer-0.1.0
#   or, to track the repo, symlink instead:
ln -s "$(pwd)/editors/vscode" ~/.vscode/extensions/jennifer-0.1.0
```

### macOS

Same as Linux - the extensions folder is also `~/.vscode/extensions`:

```sh
cp -R editors/vscode ~/.vscode/extensions/jennifer-0.1.0
#   or symlink:
ln -s "$(pwd)/editors/vscode" ~/.vscode/extensions/jennifer-0.1.0
```

### Windows

The extensions folder is `%USERPROFILE%\.vscode\extensions`. In **PowerShell**:

```powershell
# copy
Copy-Item -Recurse editors\vscode "$env:USERPROFILE\.vscode\extensions\jennifer-0.1.0"

# or, to track the repo, a symlink (needs an elevated shell or Developer Mode)
New-Item -ItemType SymbolicLink `
  -Path   "$env:USERPROFILE\.vscode\extensions\jennifer-0.1.0" `
  -Target (Resolve-Path editors\vscode)
```

From **`cmd.exe`** instead:

```bat
xcopy /E /I editors\vscode "%USERPROFILE%\.vscode\extensions\jennifer-0.1.0"
:: or a symlink (run the prompt as Administrator):
mklink /D "%USERPROFILE%\.vscode\extensions\jennifer-0.1.0" "%CD%\editors\vscode"
```

On **WSL**, treat it as Linux above for the Linux-side VS Code, or use the
packaged `.vsix` below for the Windows-side VS Code.

### After installing

Restart VS Code (or run **Developer: Reload Window** from the Command Palette).
Open any `.j` file and it highlights as Jennifer; a file whose first line is a
`#!/usr/bin/env -S jennifer run` shebang is detected even without the `.j`
extension.

## Develop / preview

Open this folder in VS Code and press <kbd>F5</kbd> to launch an Extension
Development Host with the extension loaded.

## Package a `.vsix` (optional)

Needs [`vsce`](https://github.com/microsoft/vscode-vsce). Works the same on every
OS:

```sh
cd editors/vscode
npx @vscode/vsce package        # -> jennifer-0.1.0.vsix
code --install-extension jennifer-0.1.0.vsix
```

The grammar is bundled in `syntaxes/`, so the `.vsix` is self-contained.

## Files

- `package.json` - the extension manifest (language + grammar contributions).
- `language-configuration.json` - comments, brackets, and auto-closing pairs.
- `syntaxes/jennifer.tmLanguage.json` - the TextMate grammar. This is a copy of
  [`../textmate/jennifer.tmLanguage.json`](../textmate/jennifer.tmLanguage.json),
  the shared source of truth; keep the two identical (see the note in
  [`../README.md`](../README.md)).

The grammar tracks the language spec ([`JENNIFER.md`](../../JENNIFER.md),
`docs/technical/grammar.md`). Jennifer is pre-1.0, so if highlighting looks
stale, check for an updated grammar here.
