# Jennifer modules

A **module** is distributable Jennifer source - a `.j` file whose
`export`ed names you bring in with `import`, the same call shape as a
system library:

```jennifer
import "ansi.j" as ansi;
io.printf("%s\n", ansi.bold(ansi.red("error")));
```

Modules are **not** the Go system libraries. A library
(`use NAME;` - see [../libraries/index.md](../libraries/index.md)) is
compiled into the interpreter binary; a module is ordinary Jennifer code
that ships as a file, so you can read it, fork it, or write your own. The
modules listed here are the reference set that ships with Jennifer under
`modules/`; the mechanism itself (`import` / `export`, resolution,
run-once init) is documented in the
[Imports guide](../user-guide/imports.md).

## How a module resolves

`import` picks the resolution mode from the leading token of the path:

- `import "./util.j" as u;` (or `../`) - **local**, relative to the
  importing file's directory.
- `import "/opt/m.j" as m;` - **absolute** path.
- `import "ansi.j" as ansi;` (no `./`, no `/`) - **module** lookup
  through the search path: the system module directory first (see
  `jennifer version -v` or `meta.SYSMODDIR`), then any `-I DIR` passed to
  `jennifer run`. The importing file's own directory is never consulted
  in this mode.

Distribution packages install the shipped modules to the system module
directory (`/usr/share/jennifer/modules/` by default), so
`import "ansi.j";` resolves with no path. The `as ALIAS` clause is
optional - without it the module is addressed by its file stem
(`import "ansi.j";` then `ansi.red(...)`).

## Available modules

The **TinyGo** column reports whether the module runs on the constrained
`jennifer-tiny` binary. A module is only as portable as the libraries it
`use`s: the pure-text modules run on either binary, while `smtp` `use`s `net`
(no TinyGo network stack), so it needs the default `jennifer`.

| Module                | Import with            | TinyGo | Contents                                                                                                                                    |
| --------------------- | ---------------------- | ------ | ------------------------------------------------------------------------------------------------------------------------------------------- |
| [`ansi`](ansi.md)     | `import "ansi.j";`     | full   | terminal styling as string wrappers. `color` / `bgColor` / `style` / `rgb` / `strip` plus per-colour and per-style shortcuts; TTY-aware.    |
| [`csv`](csv.md)       | `import "csv.j";`      | full   | RFC 4180 comma-separated values. `parse` / `format` (`*With` for any delimiter), `toRecords` / `fromRecords` for header-keyed maps; quoting-aware. |
| [`htmlwriter`](htmlwriter.md) | `import "htmlwriter.j";` | full | build an HTML element tree and render escaped HTML5. `element` / `text` / `raw` / `attr` constructors, `render` / `renderAll`, `escape`; void-element aware. A writer, not a parser. |
| [`idna`](idna.md)     | `import "idna.j";`     | full   | internationalized domain names: `toAscii` / `toUnicode` over a Punycode (RFC 3492) core (`münchen.de` <-> `xn--mnchen-3ya.de`), plus `isAscii`. Used by the mail clients for hosts and envelope domains. |
| [`imap`](imap.md)     | `import "imap.j";`     | no (net) | receive mail (IMAP4rev1 client) over `net`: `LOGIN` / `SELECT` / `SEARCH` / `FETCH` (with literals) / `LOGOUT`. `connect` / `selectMailbox` / `search` / `fetch` / `logout`, plus `fetchAll`; messages parsed by `mime`. A reading subset. |
| [`markdown`](markdown.md) | `import "markdown.j";` | full | render a small CommonMark subset (headings, emphasis, links, lists, code, GFM tables) to HTML (through `htmlwriter`) and styled terminal text (through `ansi`) with `toHtml` / `toAnsi`, plus authoring helpers (`header` / `style` / `link` / `bullets` / `numbered` / `codeBlock` / `table`) that build Markdown, and `tablePretty` to align table source. |
| [`mime`](mime.md)     | `import "mime.j";`     | full   | build and parse MIME messages (RFC 5322 headers, multipart, quoted-printable / base64 transfer encodings). `text` / `attachment` / `multipart` / `encode` / `parse`; the foundation the mail clients build on. |
| [`pop`](pop.md)       | `import "pop.j";`      | no (net) | receive mail (POP3 client) over `net`: plaintext / STLS / implicit TLS, `USER` / `PASS`. `connect` / `stat` / `sizes` / `retrieve` / `deleteMessage` / `quit`, plus `fetchAll`; messages parsed by `mime`. |
| [`redis`](redis.md)   | `import "redis.j";`    | no (net) | a Redis client speaking RESP2 over `net`: commands as RESP arrays, replies parsed into a `Reply`. Typed helpers `get` / `set` / `del` / `exists` / `incr` / `keys` / `ping`, plus a generic `command` for the rest. |
| [`sasl`](sasl.md)     | `import "sasl.j";`     | full   | SASL auth encoders shared by the mail clients: `plain` / `loginUser` / `loginPass` / `bearer` (XOAUTH2, the "use a token" half of OAuth2). Pure base64, no networking. |
| [`semver`](semver.md) | `import "semver.j";`   | full   | strict Semantic Versioning 2.0.0. `parse` / `isValid` / `toString`, `compare` / `lt` / `eq` / `gt`, `isStable` / `isPrerelease`, `inc*`, `sort`; struct `Version`. |
| [`smtp`](smtp.md)     | `import "smtp.j";`     | no (net) | send mail (SMTP client) over `net`: plaintext / STARTTLS / implicit TLS, `AUTH PLAIN`, `MAIL FROM` / `RCPT TO` / `DATA`. `smtp.send(opts, from, recipients, message)`; message built by `mime`. |

## Writing your own

A module is a declarations-only file: its top level permits only
`def const`, `def struct`, `func`, `use`, and `import` - no mutable
module state and no free-standing statements. Prefix a top-level
`func` / `def struct` / `def const` with `export` to publish it; unmarked
names stay module-private. Each file states its own `use` imports
(`use` is not transitive across a module boundary).

Every module that ships in this repository carries a co-located
white-box test overlay (`NAME_test.j`) run with `jennifer test`, and a
runnable demo under `examples/modules/`. See
[`modules/README.md`](https://github.com/mplx/jennifer-lang/blob/main/modules/README.md)
for the contributor checklist.

## See also

- [Imports guide](../user-guide/imports.md) - `use` vs `include` vs
  `import`, resolution rules, and the module boundary in depth.
- [Libraries catalog](../libraries/index.md) - the Go system libraries a
  module builds on.
