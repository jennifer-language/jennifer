# `tengine` - a text template engine

Import with `import "tengine.j" as tengine;`. A text template engine for
lightweight-CMS-style rendering - a subset of Go's `text/template` (the Go / Hugo
style) - evaluated directly over a `json.Value` data tree. There is no compile
step and no AST: the engine re-scans block bodies as it renders, which is fine at
page / CMS scale. It uses only the compiled-in `json` / `strings` / `lists` /
`maps` / `convert` libraries, so it runs on **both** binaries.

```jennifer
import "tengine.j" as tengine;
use json;

def set as tengine.Set init tengine.newSet();
$set = tengine.add($set, "base", "<h1>{{ .title }}</h1>{{ template \"body\" . }}");
$set = tengine.add($set, "page", "{{ define \"body\" }}<p>{{ .msg | html }}</p>{{ end }}");
def out as string init tengine.render($set, "base",
    json.decode("{\"title\":\"Hi\",\"msg\":\"a<b\"}"));
# out == "<h1>Hi</h1><p>a&lt;b</p>"
```

Runnable: [`examples/modules/tengine_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/tengine_demo.j).

## Security: output is not auto-escaped

`tengine` is modelled on Go's `text/template`, **not** `html/template`: it does
**no automatic HTML escaping**. Whatever a value contains is emitted verbatim,
`<script>` and all. Rendering untrusted data (a form field, a URL parameter, a
database row a user controls) straight into an HTML page is therefore a
cross-site-scripting (XSS) hole.

Escape every untrusted value that reaches HTML yourself, with the built-in
`html` pipe:

```jennifer
# SAFE: the user-controlled field is escaped before it lands in the page.
$set = tengine.add($set, "page", "<p>{{ .comment | html }}</p>");

# UNSAFE: .comment is emitted raw - a comment of "<script>...</script>" runs.
$set = tengine.add($set, "page", "<p>{{ .comment }}</p>");
```

The `html` pipe escapes `&`, `<`, `>`, `'`, and `"`. For attribute values and
URLs the same caution applies: build links through `html.safeUrl` (which
rejects `javascript:` and other dangerous schemes) rather than interpolating a
raw URL into an `href`. When a template mixes trusted markup with untrusted
data, escape at the boundary - the engine cannot tell the two apart for you.

## Data model

Templates render against a **`json.Value`** node (use `json.decode` to build
one). Inside a template three things address data:

- **`.`** - the *current* node. `{{ . }}` outputs it; `{{ .a.b }}` reads `/a/b`
  from it (a dotted path is a JSON Pointer). `.` is rebound by `with`, `range`,
  and `template`.
- **`$`** - the *root* node passed to `render` (or to the enclosing `template`
  call). `{{ $.site.title }}` reaches the top-level data even from inside a loop.
- **`$name`** - a variable (see [Variables](#variables)).

A missing key renders as **empty**, not an error. Output is **not auto-escaped**
(this mirrors `text/template`, not `html/template`): pipe untrusted values
through `html` in an HTML context.

**Truthiness** (for `if` / `with` and the range `else`): a value is true when it
is non-null, a non-empty string, a non-zero number, `true`, or a non-empty list
or map.

## API

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `tengine.newSet()` | `Set` | A fresh, empty template set. |
| `tengine.add(set, name, src)` | `Set` | Register `src` under `name`; any `{{ define }}` blocks become their own entries. Value-semantic (returns a new `Set`). |
| `tengine.render(set, entry, data)` | `string` | Render `entry` against a `json.Value`. |

`Set` is value-semantic, so build a page per request by `add`ing the base layout,
the partials, and the page (which `{{ define }}`s the sections the base pulls
in), then `render` the base.

## Actions

| Action | Effect |
| ------ | ------ |
| `{{ .a.b }}` / `{{ . }}` / `{{ $.x }}` / `{{ $var }}` | Output a value (see [pipes](#pipes)). |
| `{{ if COND }} A {{ else if COND }} B {{ else }} C {{ end }}` | Conditional with any number of `else if`s and an optional final `else`. |
| `{{ range .items }} A {{ else }} B {{ end }}` | Render `A` once per element (of a list) or value (of a map, insertion order), rebinding `.` each time; `B` for an empty collection. |
| `{{ range $i, $e := .items }} ... {{ end }}` | As above, also binding `$i` (index / key) and `$e` (element). `{{ range $e := .items }}` binds just `$e`. |
| `{{ with .x }} A {{ else }} B {{ end }}` | Rebind `.` to `.x` for `A` when truthy, else `B`. |
| `{{ $x := PIPE }}` | Assign a variable (produces no output). |
| `{{ define "name" }} ... {{ end }}` | Define a named template (collected when the source is `add`ed). |
| `{{ template "name" . }}` | Render a named template with the given node as `.` (and `$`). |
| `{{ block "name" . }} default {{ end }}` | Render the set's `name` template if one exists, otherwise the inline default. |
| `{{/* comment */}}` | Dropped from the output. |

### Conditionals

A condition is truthiness of a value, or a call to a comparison / boolean
function. Functions are prefix, and arguments may be parenthesised to nest:

| Function | Meaning |
| -------- | ------- |
| `eq a b` / `ne a b` | Equal / not equal (numbers compare numerically, strings and bools by value). |
| `lt` / `le` / `gt` / `ge` | Ordered comparison (numeric when both are numbers, else lexical). |
| `and x y ...` / `or x y ...` | Boolean and / or over any number of truthy tests. |
| `not x` | Boolean negation. |

```
{{ if eq .type "post" }}...{{ end }}
{{ if and .published (not .draft) }}...{{ end }}
{{ if or (eq .section "blog") (eq .section "news") }}...{{ end }}
{{ if gt .count 0 }}{{ .count }} items{{ else }}empty{{ end }}
```

Arguments are terms: `.path`, `$`, `$.path`, `$var`, a `"string"`, a number, or
`true` / `false`.

### Variables

`{{ $name := PIPE }}` binds a variable to a pipeline's value; `{{ $name }}` reads
it, and `{{ $name.a.b }}` indexes into it. A variable is visible from its
assignment to the end of the current template (and inside the blocks it
encloses) - the usual way to keep the root, or a computed value, reachable inside
a `range`:

```
{{ $site := .site }}
{{ range .posts }}
  <a href="{{ $site.baseUrl }}/{{ .slug }}">{{ .title }}</a>
{{ end }}
```

`range $i, $e := .items` is the same mechanism: it binds the index / key and the
element per iteration.

### Pipes

An output or assignment value may be piped through one or more functions:
`{{ .title | trim | title }}`, `{{ .tags | join ", " }}`.

| Pipe | Effect |
| ---- | ------ |
| `upper` / `lower` | Case conversion. |
| `title` | Upper-case the first letter of each word. |
| `trim` | Strip surrounding whitespace. |
| `html` | Escape `& < > " '` as HTML entities. |
| `urlize` | Slugify: lower-case, keep alphanumerics, collapse spaces / dashes / underscores to single dashes. |
| `default X` | The value if it is truthy, otherwise `X` (a fallback for optional fields). |
| `truncate N` | The first `N` characters, with `...` appended when it shortened (excerpts). |
| `join SEP` | Join a list's elements into a string with `SEP` between them. |
| `len` | The length: characters of a string, or elements of a list / map. |
| `printf FORMAT` | Format the piped value per `FORMAT` (see below). |

An unknown pipe throws a catchable `Error` (kind `"tengine"`).

Nesting (control blocks, `template` / `block` includes, `range` bodies) is
capped at 256 levels; past the cap the engine throws a catchable `Error`
(kind `"tengine"`) instead of recursing without bound, so a template that
includes itself - directly or through a partner - fails cleanly even when
templates come from untrusted authors.

### `printf`

`printf` formats values with a `text/template`-style format string. It works both
as a **function** - `{{ printf "%s: %d" .name .count }}` - and as a **pipe**,
where the piped value is the last argument: `{{ .n | printf "%02d" }}` renders
`07`. Verbs: `%s` / `%v` (string), `%d` (integer), `%f` (float), `%t` (bool), and
`%%` (a literal `%`). Each verb accepts the flags `-` (left-align) and `0`
(zero-pad), a width, and a `.precision` (decimal places for `%f`, or a maximum
length for `%s`):

```
{{ printf "%.2f" .price }}     -> 3.50
{{ printf "%-10s|" .name }}    -> "Ada       |"
{{ range $i, $t := .items }}{{ printf "%02d. %s\n" $i $t.title }}{{ end }}
```

(This is a self-contained subset, not the full Go / `io.printf` verb set - no
`%x` / `%e` / `%q`, and no `*` dynamic width.)

### Whitespace-trim markers

A leading `{{-` trims the whitespace immediately **before** the action; a
trailing `-}}` trims the whitespace **after** it - the same as Go. This keeps
generated output tidy without cramming the template onto one line:

```
<ul>
{{- range .items }}
  <li>{{ . }}</li>
{{- end }}
</ul>
```

renders `<ul>\n  <li>a</li>\n  <li>b</li>\n</ul>` for `items = ["a", "b"]`.

## Layout inheritance

The `define` / `template` / `block` trio gives Go/Hugo-style layout inheritance.
A base layout names the holes; a page fills them:

```jennifer
def set as tengine.Set init tengine.newSet();
$set = tengine.add($set, "base",
    "<html><body>{{ block \"content\" . }}<p>default</p>{{ end }}</body></html>");
$set = tengine.add($set, "page",
    "{{ define \"content\" }}<h1>{{ .heading }}</h1>{{ end }}");
def out as string init tengine.render($set, "base", json.decode("{\"heading\":\"Hello\"}"));
# <html><body><h1>Hello</h1></body></html>
```

`{{ block "x" . }}...{{ end }}` renders the set's `x` if a page defined one, else
its own inline body - the override hook. `add` pulls each `{{ define }}` out into
its own named entry, so the order you `add` pages does not matter. Each
`template` / `block` invocation starts a fresh variable scope, and `$` is reset
to the node it was called with.

## Scope

A focused subset - enough to drive a lightweight CMS (list and single pages,
menus, conditional sections, excerpts), not a general programming language:

- **Pipe / function arguments are literal or path terms** (`default "x"`,
  `truncate 20`, `printf "%02d" .n`); no `*` dynamic widths.
- **No user-defined functions** and no method calls - the built-in comparison /
  boolean / `printf` functions and pipes are the whole vocabulary.
- **Fixed data source.** The data tree is the `json.Value` you pass; there is no
  file inclusion or front-matter parsing (compose the tree yourself, e.g. with
  `toml` / `json` for front matter and `markdown` for bodies).
- **No auto-escaping.** Escaping is explicit via the `html` pipe (a
  `text/template`, not an `html/template`).

Adding a pipe or a comparison function is one branch in the engine.

## See also

- [json.md](../libraries/json.md) - the `json.Value` data tree templates render over.
- [markdown.md](markdown.md) - render Markdown bodies to HTML for a template.
- [toml.md](../libraries/toml.md) - parse front matter / config into a data tree.
- [html.md](html.md) - build an HTML tree programmatically instead of from text.
- [modules/index.md](index.md) - the module catalog and import rules.
