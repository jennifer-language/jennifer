# `docblock` - Jennifer doc comments and their parser

`import "docblock.j" as docblock;`

Two things in one: the **blessed doc-comment format** for Jennifer source, and
`docblock.parse`, which reads source and returns that documentation as
structured, typed values. It produces *data*; it does not render (turning docs
into HTML is a separate consumer). Pure Jennifer over `regex` and `strings`;
runs on either binary.

## The format: Jennifer doc comments

A doc comment opens with **exactly `/**`** (a plain `/*` block comment stays
invisible) and closes with `*/`. It **immediately precedes** the construct it
documents - a `func`, a `def struct`, a `def enum`, or a `def const` - or, when it carries an
`@module` tag, it is the **file preamble**. Whether a construct is exported is
read from its `export` keyword, never a tag.

```jennifer
/**
 * Distance between two points.
 *
 * A longer description may follow, separated from the summary by a blank line.
 * @param ax {float} first x coordinate
 * @param ay {float} first y coordinate
 * @return {float} the Euclidean distance
 * @since 0.9
 */
export func distance(ax as float, ay as float, bx as float, by as float) { ... }
```

The body is a **summary** - the first paragraph, with wrapped lines joined, so it
can span several source lines - then an optional **description** (the remaining
paragraphs, separated from the summary by a blank line), then `@`-tags. Because the
split is by paragraph and not by line, a summary that wraps across source lines stays
one sentence rather than breaking at the line break; put a blank line where you want
the summary to end and the description to begin.

### Types are written in Jennifer syntax, in braces

Every `{...}` type is written verbatim in Jennifer's own syntax - `{int}`,
`{list of int}`, `{map of string to list of int}`, `{json.Value}`. There is no
`any` / `mixed` pseudo-type (Jennifer has no top type); an opaque value
documents as `json.Value` or a named struct. The braces make extraction
unambiguous.

### Tags

| Tag | On | Meaning |
| --- | -- | ------- |
| `@param name {type} desc` | func | One per parameter. |
| `@field name {type} desc` | struct | One per field. |
| `@return {type} desc` | func | The return value. |
| `@throws {type} desc` | func | An error the function may throw (repeatable). |
| `@since version` | any | When it was introduced. |
| `@deprecated [reason]` | any | Marks it deprecated. |
| `@see ref` | any | A cross-reference (repeatable). |
| `@example` | func | A runnable example; its body is the following lines up to the next tag. |
| `@internal` | any | Not part of the public surface. |
| `@module name` | preamble | Marks the file preamble (module doc). |
| `@author` / `@version` / `@license` | preamble | Module metadata. |

The reference layout is **`jennifer fmt` output** - fmt preserves doc comments
and normalises layout deterministically, so associating a comment with the
construct that follows it is reliable.

## Parsing: `docblock.parse(source)`

`docblock.parse(source as string) -> FileDoc` returns the whole document:

```jennifer
import "docblock.j" as docblock;
def doc as docblock.FileDoc init docblock.parse(source);
io.printf("%s\n", $doc.module.summary);
for (def f in $doc.funcs) {
    io.printf("%s (%d params)\n", $f.name, len($f.params));
}
```

### The result types

The result is **typed data, not a tag bag** - Jennifer has no sum types, so
heterogeneous collections are modelled as parallel typed lists plus fixed-field
structs. All are exported:

- `FileDoc { module, funcs, structs, enums, consts, diagnostics }`
- `ModuleDoc { summary, description, author, version, license, see }`
- `FuncDoc { name, exported, summary, description, params, returns, throws, examples, since, deprecated, see, internal }`
- `StructDoc { name, exported, summary, description, fields, since, deprecated, see, internal }`
- `EnumDoc { name, exported, summary, description, since, deprecated, see, internal }` (variants are described in prose, not tags, so there is no field list)
- `ConstDoc { name, exported, type, summary, description, since, deprecated, see, internal }`
- `ParamDoc { name, type, description }` (also used for a struct's `fields`)
- `ReturnDoc { type, description }`, `ThrowDoc { type, description }`
- `Diagnostic { severity, line, message }`

An absent value is its zero: a function with no `@return` has a `returns` whose
`type` is `""`; a file with no preamble has an empty `module`.

## Diagnostics: it reports, it never enforces

`docblock` never fails on a documentation error - problems come back as
`Diagnostic` values and the caller decides what is fatal. Three are reported:

- A **`@param` / `@field` that names nothing real** - the documented name is
  not a parameter / field of the construct.
- A **real parameter / field with no `@param` / `@field`** - docs drifting
  behind the code, the commonest doc bug. Names and counts are cross-checked
  against the actual declaration.
- An **orphaned** doc comment - one that precedes neither a documentable
  construct nor a preamble.

```
[warning] line 36: @param "typo" is not a param of drifted
[warning] line 36: param "real" of drifted has no @param
```

Whole-body analysis (e.g. "a `@return` on a function that never returns a
value") is **out of scope**: it needs the AST, and the module has only the
text. What earns its keep is the signature cross-check above.

## Scanner correctness

Comment boundaries are found by a character-level `/* */` depth scan that
**skips string literals** and `#` line comments and **nests `/* */`
correctly** - not a fragile "delimiter alone on its line" rule. So a `/**`
inside a string literal is not mistaken for a doc comment, and a nested
`/* ... */` inside a doc body does not close it early.

## Checking a file or tree

`scripts/docblock-check.sh` runs the parser over a `.j` file or a whole
directory and reports each file's doc coverage plus any diagnostics, exiting
non-zero when it finds a problem - a ready-made pre-commit / CI check:

```sh
scripts/docblock-check.sh modules/          # every .j under modules/
scripts/docblock-check.sh myapp.j           # a single file
# ok   modules/web.j  (1 module, 22 func, 3 struct, 0 enum, 0 const)
# WARN app.j  (1 diagnostic)
#        line 12: @param "nmae" is not a param of greet
```

It finds the interpreter via `JENNIFER=/path/to/jennifer`, else `./jennifer`
at the repo root, else `jennifer` on `PATH`. It exits `0` when every file is
clean and `1` when any file has diagnostics, so it drops straight into a
pre-commit hook.

### Enforced in CI

The project runs this check on every push and pull request (a "Check doc
comments" step in `.github/workflows/test.yml`), over both trees:

```sh
scripts/docblock-check.sh examples/
scripts/docblock-check.sh modules/
```

A drift diagnostic fails the build, so a doc comment can never silently fall
out of step with the code it documents - the same guarantee the linter and the
module test overlays give. A missing doc comment is *not* an error (docblock
reports drift, not absence); it flags a doc that is present but wrong, or one
that documents nothing.

## See also

- [`jennifer fmt`](../technical/cli_fmt.md) - the canonical layout normaliser.
- [`regex`](../libraries/regex.md) / [`strings`](../libraries/strings.md) - the
  libraries `docblock` is built on.
- [`html`](html.md) - render parsed docs to HTML (a separate
  consumer, not part of `docblock`).
