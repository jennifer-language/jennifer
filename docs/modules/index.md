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
`use`s: the pure-text modules run on either binary, while `smtp` `use`s `net`,
so it needs a build with a network stack.

A **`no (net)`** entry means the module needs `net`, which the **stock**
`jennifer-tiny` build ships without - not a TinyGo limitation. A `jennifer-tiny`
rebuilt with a network stack runs the net-backed modules too; see the
[note on `net` and TinyGo](../technical/tinygo.md#net-on-tinygo-is-a-build-choice-not-a-hard-limit).
Read "needs the default `jennifer` binary" throughout these docs as "needs a
build that includes a network stack" (the stock `jennifer` has one).

| Module                | Import with            | TinyGo | Contents                                                                                                                                    |
| --------------------- | ---------------------- | ------ | ------------------------------------------------------------------------------------------------------------------------------------------- |
| [`ansi`](ansi.md)     | `import "ansi.j";`     | full   | terminal styling as string wrappers. `color` / `bgColor` / `style` / `rgb` / `strip` plus per-colour and per-style shortcuts; TTY-aware.    |
| [`csv`](csv.md)       | `import "csv.j";`      | full   | RFC 4180 comma-separated values. `parse` / `format` (`*With` for any delimiter), `toRecords` / `fromRecords` for header-keyed maps; quoting-aware. |
| [`flatdb`](flatdb.md) | `import "flatdb.j";`   | full   | a file-backed JSON document store over `json` + `fs`: `open` a file into a value-semantic `DB`, query / edit by JSON Pointer (`get` / `has` / `keys` / `length` / `set` / `append` / `remove`), `save` with a crash-atomic temp+rename. Crash-atomic snapshotting of small data, not a database engine. |
| [`gotify`](gotify.md) | `import "gotify.j";`   | no (net) | push notifications to a [Gotify](https://gotify.net) server, on top of `http`: `push(cfg, title, message, priority)` POSTs the message form with the `X-Gotify-Key` header; value-semantic `Config` (url + token). |
| [`gpio`](gpio.md)     | `import "gpio.j";`     | full   | Raspberry-Pi (Linux SBC) GPIO over sysfs (`fs` is the whole backend): `setup(pin, "in"/"out")`, `write(pin, 0/1)`, `read(pin)`, `release(pin)`. Stateless, pin-keyed; `JENNIFER_GPIO_BASE` overrides the sysfs root (tests / mounts). Absent-platform errors are clear, not crashes. |
| [`htmlwriter`](htmlwriter.md) | `import "htmlwriter.j";` | full | build an HTML element tree and render escaped HTML5. `element` / `text` / `raw` / `attr` constructors, `render` / `renderAll`, `escape`; void-element aware. A writer, not a parser. |
| [`http`](http.md)     | `import "http.j";`     | no (net) | an HTTP/1.1 client over `net` (`https://` via TLS): method-agnostic `request` plus `get` / `post` / `put` / `patch` / `delete` / `head` / `options` -> `Response` (`status`, `headers`, `body`); Content-Length + chunked framing, `header` case-insensitive lookup. Redirects returned, not followed. |
| [`idna`](idna.md)     | `import "idna.j";`     | full   | internationalized domain names: `toAscii` / `toUnicode` over a Punycode (RFC 3492) core (`münchen.de` <-> `xn--mnchen-3ya.de`), plus `isAscii`. Used by the mail clients for hosts and envelope domains. |
| [`imap`](imap.md)     | `import "imap.j";`     | no (net) | receive mail (IMAP4rev1 client) over `net`: `LOGIN` / `SELECT` / `SEARCH` / `FETCH` (with literals) / `LOGOUT`. `connect` / `selectMailbox` / `search` / `fetch` / `logout`, plus `fetchAll`; messages parsed by `mime`. A reading subset. |
| [`markdown`](markdown.md) | `import "markdown.j";` | full | render a small CommonMark subset (headings, emphasis, links, lists, code, GFM tables) to HTML (through `htmlwriter`) and styled terminal text (through `ansi`) with `toHtml` / `toAnsi`, plus authoring helpers (`header` / `style` / `link` / `bullets` / `numbered` / `codeBlock` / `table`) that build Markdown, and `tablePretty` to align table source. |
| [`memcache`](memcache.md) | `import "memcache.j";` | no (net) | a memcached client (classic text protocol) over `net`: `set` / `add` / `get` / `delete` / `incr` / `decr` / `touch`, every store with a TTL. For caches, sessions, counters, and locks (a volatile store, not a system of record). |
| [`mime`](mime.md)     | `import "mime.j";`     | full   | build and parse MIME messages (RFC 5322 headers, multipart, quoted-printable / base64 transfer encodings, RFC 2047 encoded-words for non-ASCII headers). `text` / `attachment` / `multipart` / `encode` / `parse`; the foundation the mail clients build on. |
| [`oauth`](oauth.md)   | `import "oauth.j";`    | no (net) | a generic OAuth2 client (the get-a-token half) on `http` + `json`: Client Credentials, Refresh Token, and Device Authorization grants, `google` / `microsoft` presets, expiry + on-disk token store. Tokens feed `sasl` XOAUTH2 for mail. |
| [`pop`](pop.md)       | `import "pop.j";`      | no (net) | receive mail (POP3 client) over `net`: plaintext / STLS / implicit TLS, `USER` / `PASS`. `connect` / `stat` / `sizes` / `retrieve` / `deleteMessage` / `quit`, plus `fetchAll`; messages parsed by `mime`. |
| [`ratelimit`](ratelimit.md) | `import "ratelimit.j";` | no (net) | a fixed-window rate limiter on `memcache` (atomic `incr` + per-key TTL): `allow(mc, key, limit, window)` -> bool, `remaining(mc, key, limit)`. The window resets on its own when it expires. |
| [`redis`](redis.md)   | `import "redis.j";`    | no (net) | a Redis client speaking RESP2 over `net`: commands as RESP arrays, replies parsed into a `Reply`. Typed helpers `get` / `set` / `del` / `exists` / `incr` / `keys` / `ping`, plus a generic `command` for the rest. |
| [`resque`](resque.md) | `import "resque.j";`   | no (net) | background jobs on Redis, wire-compatible with Resque: `enqueue` onto named queues, `reserve` from a worker in priority order (`Job` = `queue` / `class` / `args`), `queueLength` / `queues` / `size` / `fail`. Interops with Ruby-resque / php-resque workers. Built on `redis` + `json`. |
| [`rest`](rest.md)     | `import "rest.j";`     | no (net) | an ergonomic REST layer over `http` + `json`: a value-semantic `Client` (base URL + headers) and `get` / `post` / `put` / `patch` / `delete` (+ `getJson` / `postJson` / ...). Base-URL joining, query strings, Bearer / Basic auth. |
| [`sasl`](sasl.md)     | `import "sasl.j";`     | full   | SASL auth encoders shared by the mail clients: `plain` / `loginUser` / `loginPass` / `bearer` (XOAUTH2, the "use a token" half of OAuth2). Pure base64, no networking. |
| [`semver`](semver.md) | `import "semver.j";`   | full   | strict Semantic Versioning 2.0.0. `parse` / `isValid` / `toString`, `compare` / `lt` / `eq` / `gt`, `isStable` / `isPrerelease`, `inc*`, `sort`; struct `Version`. |
| [`session`](session.md) | `import "session.j";` | no (net) | server-side sessions on `memcache`: a `map of string to string` under `sess:ID` with a sliding TTL. `create` / `load` / `save` / `touch` / `destroy`; UUID v4 IDs, base64-wrapped JSON values. Volatile (a cache, not a store of record). |
| [`smtp`](smtp.md)     | `import "smtp.j";`     | no (net) | send mail (SMTP client) over `net`: plaintext / STARTTLS / implicit TLS, `AUTH PLAIN`, `MAIL FROM` / `RCPT TO` / `DATA`. `smtp.send(opts, from, recipients, message)`; message built by `mime`. |
| [`web`](web.md)       | `import "web.j";`      | no (net) | a small HTTP framework over the `httpd` engine: register routes against handler methods by name (`web.get` / `post` / ...), `:param` capture, middleware, `web.Context` request / response helpers; `web.run` owns the accept loop. Dispatch by `meta.callMain`. Pairs with `jennifer serve`. |

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
