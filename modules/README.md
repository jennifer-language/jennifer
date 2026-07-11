# Jennifer modules

Jennifer-coded library modules (`.j` files) live here. Unlike the Go
system libraries (`internal/lib/*`, enabled with `use NAME;`), a module is
distributable Jennifer source, brought in with `import "NAME.j" as NAME;`.

Distribution packages install these to the system module directory
(`/usr/share/jennifer/modules/` by default; see `jennifer version -v`),
so `import "NAME.j";` resolves without a path. Local modules resolve with
`import "./NAME.j";`, and extra search directories are added with
`jennifer run -I DIR ...`.

## Available modules

- **`ansi.j`** - terminal styling as explicit string wrappers.
  `ansi.color(s, name)` / `ansi.bgColor(s, name)` / `ansi.style(s, name)`
  (bold / dim / italic / underline / reverse) / `ansi.rgb(s, r, g, b)`,
  `ansi.strip(s)` to remove escapes, plus per-colour and per-style
  shortcuts (`ansi.red(s)`, `ansi.bold(s)`, ...). Stateless and TTY-aware:
  styling suppresses itself when stdout is not a terminal or `NO_COLOR` is
  set, and is forced on by `FORCE_COLOR`. See
  [`examples/modules/ansi_demo.j`](../examples/modules/ansi_demo.j).
- **`csv.j`** - RFC 4180 comma-separated values: parse text into rows of
  string fields and format rows back to text, with a quoting-aware scanner.
  `csv.parse(s)` / `csv.format(rows)` (and `parseWith` / `formatWith` for any
  single-character delimiter, so TSV too), plus `csv.toRecords(rows)` /
  `csv.fromRecords(header, records)` for header-keyed `map of string to
  string` records. Pure Jennifer over `strings` and `maps`. See
  [`examples/modules/csv_demo.j`](../examples/modules/csv_demo.j).
- **`htmlwriter.j`** - build an HTML element tree and render it to escaped
  HTML5. `html.element(tag, attrs, children)` / `html.text(s)` / `html.raw(s)`
  / `html.attr(name, value)` constructors, `html.render(node)` /
  `html.renderAll(nodes)`, and `html.escape(s)`; text and attribute values are
  escaped automatically, void elements (`br`, `img`, ...) render without a
  closing tag. A writer, not a parser; pure Jennifer over `strings` and
  `lists`. See
  [`examples/modules/htmlwriter_demo.j`](../examples/modules/htmlwriter_demo.j).
- **`markdown.j`** - render a small CommonMark subset (headings, bold /
  italic, inline code, links, fenced code blocks, ordered / unordered lists)
  to HTML and to styled terminal text. `markdown.toHtml(md)` renders through
  the `htmlwriter` module (so escaping is automatic); `markdown.toAnsi(md)`
  renders through the `ansi` module. Also authors Markdown text with
  `markdown.header` / `style` / `link` / `bullets` / `numbered` / `codeBlock`,
  and `markdown.table(headings, aligns, rows)` for GFM tables.
  Pure Jennifer; the first module that imports sibling modules. See
  [`examples/modules/markdown_demo.j`](../examples/modules/markdown_demo.j).
- **`semver.j`** - strict Semantic Versioning 2.0.0: parse, compare, sort,
  and increment version numbers. `semver.parse(s)` / `isValid(s)` /
  `toString(v)`, `compare(a, b)` / `lt` / `eq` / `gt`, `isStable(v)` /
  `isPrerelease(v)`, `incMajor` / `incMinor` / `incPatch(v)`, and
  `sort(vs)`, over an exported `Version` struct. Pure Jennifer; parsing
  uses the canonical SemVer regex, precedence and sort are hand-written.
  See [`examples/modules/semver_demo.j`](../examples/modules/semver_demo.j).

Reference docs for each module live under
[`docs/modules/`](../docs/modules/index.md).
