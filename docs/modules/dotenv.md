# `dotenv` - `.env` configuration files

Import with `import "dotenv.j" as dotenv;`. Read the `KEY=VALUE` files that keep
configuration and secrets out of source, with layered environment profiles and
`${VAR}` interpolation. Over `fs` + `strings` + `os` + `path` + `regex` + `maps`;
pure `.j`, runs on **both** binaries.

```jennifer
import "dotenv.j" as dotenv;
use os;

# Single file:
def cfg as map of string to string init dotenv.parse("PORT=8080\nURL=\"http://h:${PORT}\"");
io.printf("%s\n", $cfg["URL"]);       # http://h:8080

# Layered, real-env-wins load (profile from JENNIFER_ENV):
dotenv.autoload(os.cwd());
```

Runnable: [`examples/modules/dotenv_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/dotenv_demo.j).

## Functions

### Single-file primitives

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `dotenv.parse(text)` | `map of string to string` | Parse `.env` text; touches nothing else. |
| `dotenv.read(path)` | `map of string to string` | Read and parse one file. |
| `dotenv.load(path)` | `map of string to string` | Read, parse, and `os.setEnv` each variable (**unconditional override**); returns the map. |

`load` overwrites whatever is already in the environment - it is the low-level
primitive. For a layered, real-env-wins load, use the cascade loaders below.

### Layered loaders

Merge, later overriding earlier and skipping absent files, from **one explicit
base directory** (no search, no walk-up):

```
.env  ->  .env.local  ->  .env.<profile>  ->  .env.<profile>.local
```

The `<profile>` comes from the `profile` argument (or `JENNIFER_ENV` for
`autoload`); an empty profile loads only the two base files (there is no
`.env.default`). At the command line, `jennifer run --env=prod app.j` sets
`JENNIFER_ENV=prod` before the program runs (identical to `JENNIFER_ENV=prod
jennifer run app.j`), so `--env` is the ergonomic way to pick the profile
`autoload` reads - see
[Run profiles](../user-guide/tooling.md#run-profiles---env).

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `dotenv.readCascade(dir, profile)` | `map of string to string` | The merged file layers. **No env mutation.** |
| `dotenv.resolve(dir, profile)` | `map of string to string` | The **effective** map: `readCascade` with the real OS env overlaid (real env wins). No env mutation. |
| `dotenv.loadCascade(dir, profile)` | `map of string to string` | `readCascade`, then `os.setEnv` **only keys not already set** in the real env. Returns the file map. |
| `dotenv.autoload(dir)` | `map of string to string` | `loadCascade(dir, JENNIFER_ENV)` - reads the `JENNIFER_ENV` variable for the profile. |

Value precedence, highest first: a **pre-existing OS env var** (never overwritten)
> `.env.<profile>.local` > `.env.<profile>` > `.env.local` > `.env`.

## Syntax

Each non-blank, non-comment line is a `KEY=VALUE` assignment:

```dotenv
# a comment line
PORT=8080
export NAME="ada lovelace"     # a leading `export` is ignored
GREETING='hi # not a comment'  # single quotes are literal
BANNER="line one
line two"                      # double quotes may span physical lines
URL="http://localhost:${PORT}" # ${VAR} interpolation
TOKEN=abc123        # trailing inline comment (unquoted values only)
EMPTY=
```

- **Comments** - a line starting with `#` is skipped; on an **unquoted** value a
  ` #` (space then hash) starts an inline comment. Inside quotes, `#` is literal.
- **`export`** - a leading `export ` prefix is stripped, so a file that doubles as
  a shell script parses the same.
- **Double quotes** expand the escapes `\n`, `\t`, `\r` (and `\"` for a literal
  quote) and **may span multiple physical lines** - the real newlines become part
  of the value. An unterminated double quote is a positioned `dotenv` error.
- **Single quotes** are fully literal - no escapes, no interpolation.
- **Unquoted** values are trimmed of surrounding whitespace.
- The value may contain `=` (only the first `=` splits the line); a line with no
  `=` or an empty key is ignored; a later duplicate key wins.

## Interpolation

`${VAR}` in an **unquoted or double-quoted** value is replaced with `VAR`'s value.
Resolution is **backward-reference only** - it looks up, in order:

1. a key already parsed (earlier in this file, or an earlier cascade layer),
2. the real OS environment,
3. the empty string.

Because a reference can only see keys defined *before* it, cycles are impossible
by construction (a forward reference simply resolves to `""`).

- Interpolation runs in **unquoted** and **double-quoted** values; a
  **single-quoted** value is fully literal (`'${X}'` stays `${X}`).
- A `${...}` whose contents are not a valid variable name is kept **literal**.
- There is **no** `$(...)` / backtick command substitution - those are plain
  characters. A `.env` value can never execute a command. This is a hard security
  line, not a missing feature.

## Security

`.env*` files are **trusted local input** - the developer controls the working
directory; dotenv is not a sandbox. Within that model:

- **Strict profile validation.** A profile label must match
  `^[A-Za-z0-9_-]{1,64}$` before it is spliced into `.env.<profile>`, so a
  `JENNIFER_ENV` from an untrusted upstream cannot path-traverse
  (`../../etc/x`, `prod/../y`). An invalid label is a `dotenv` error.
- **Fixed base directory - no search, no walk-up.** Loading from one explicit
  `dir` closes the file-hijack class: there is no "nearest `.env`" walk that an
  attacker could poison by planting a file in a parent directory. `.env.local`
  overriding `.env` is safe because they share that same developer-controlled
  directory.
- **Real env wins.** The cascade loaders (`resolve` / `loadCascade` / `autoload`)
  never let a committed file clobber a real environment variable - a deployment's
  real secret always beats a file value. (The single-file `load` still overrides
  unconditionally - use it only when that is what you want; reach for the cascade
  loaders otherwise.) To force-override from a cascade, use `readCascade` + your
  own `os.setEnv` loop.

> A variable set to the **empty string** in the real environment is treated as
> *unset* for "real env wins" (there is no `os.hasEnv`), so a file value can still
> fill it. Set a real value, not `""`, when you mean to override.

## Scope

- **No `$VAR` (unbraced) interpolation** - only `${VAR}`. A bare `$FOO` is literal.
- **Multi-line applies to double quotes only** - a single-quoted value is one
  line.

## See also

- [fs.md](../libraries/fs.md) - the file reads the loaders use.
- [os.md](../libraries/os.md) - `os.setEnv` / `os.getEnv` for the environment.
- [path.md](../libraries/path.md) - `path.join` builds the layer paths.
- [toml.md](../libraries/toml.md) - for richer, typed, nested configuration.
- [modules/index.md](index.md) - the module catalog and import rules.
