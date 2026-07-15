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
| [`amqp`](amqp.md)     | `import "amqp.j";`     | no (net) | an AMQP 0-9-1 client for RabbitMQ over `net`: `connect` runs the connection / channel handshake (SASL PLAIN), then `declareQueue` -> `QueueInfo`, `publish` / `publishText` (method + content-header + body frames), `get(c, queue, autoAck) -> Message` (synchronous `Basic.Get` pull), `ack`, `close`. Binary frame / method encoding hand-built with the bitwise ops. The largest protocol module. Needs the default binary. |
| [`ansi`](ansi.md)     | `import "ansi.j";`     | full   | terminal styling as string wrappers. `color` / `bgColor` / `style` / `rgb` / `strip` plus per-colour and per-style shortcuts; TTY-aware.    |
| [`bloom`](bloom.md)   | `import "bloom.j";`    | full   | a Bloom filter (probabilistic set): `new(size, hashes)`, `add` / `addAll`, `mightContain` - no false negatives, possible false positives. Bits packed in `bytes`; k positions from double-hashing one SHA-256 digest. Value-semantic. Over `hash`; both binaries. |
| [`bucket`](bucket.md) | `import "bucket.j";`   | no (net) | S3-compatible object storage (AWS S3 / MinIO / R2 / B2), AWS Signature V4-signed: `connect` -> `Client`, then `get` / `put` / `delete` / `listObjects` (+ `objectKeys`). Path-style; configurable endpoint. Over `hash.hmac` + `http` + `time`. |
| [`cron`](cron.md)     | `import "cron.j";`     | full   | cron schedules: `parse(expr)` -> `Schedule`, `matches(schedule, t)`, `next(schedule, after)` -> `time.Time`. Five fields with `*` / `,` / `-` / `/n`; the dom-OR-dow rule. A pure calculator over `time`. |
| [`csv`](csv.md)       | `import "csv.j";`      | full   | RFC 4180 comma-separated values. `parse` / `format` (`*With` for any delimiter), `toRecords` / `fromRecords` for header-keyed maps; quoting-aware. |
| [`discord`](discord.md) | `import "discord.j";`  | no (net) | post to a Discord channel Webhook on `http`: `send(webhookUrl, content)` for a plain message, or build a rich message with `message` / `content` / `embed(m, title, description, color)` and post it with `sendMessage`. `render` gives the JSON payload; strings JSON-escaped. Sibling of `slack` / `gotify`. Needs the default binary. |
| [`docblock`](docblock.md) | `import "docblock.j";` | full | Jennifer doc-comment format + parser. `docblock.parse(source)` -> a typed `FileDoc` (module preamble, per-construct `FuncDoc` / `StructDoc` / `ConstDoc`, tags, and `Diagnostic`s). Reports drift (a `@param` naming no real parameter, a parameter with no `@param`) and orphans; string-literal- and nesting-correct scanner. Data, not rendering. |
| [`dotenv`](dotenv.md) | `import "dotenv.j";`   | full   | `.env` config files: `parse(text)` / `read(path)` -> `map of string to string`, and `load(path)` (parse + `os.setEnv` each). Handles `#` comments, `export`, single / double quoting, inline comments. Over `fs` + `strings` + `os`. |
| [`flatdb`](flatdb.md) | `import "flatdb.j";`   | full   | a file-backed JSON document store over `json` + `fs`: `open` a file into a value-semantic `DB`, query / edit by JSON Pointer (`get` / `has` / `keys` / `length` / `set` / `append` / `remove`), `save` with a crash-atomic temp+rename. Crash-atomic snapshotting of small data, not a database engine. |
| [`gotify`](gotify.md) | `import "gotify.j";`   | no (net) | push notifications to a [Gotify](https://gotify.net) server, on top of `http`: `push(cfg, title, message, priority)` POSTs the message form with the `X-Gotify-Key` header; value-semantic `Config` (url + token). |
| [`gpio`](gpio.md)     | `import "gpio.j";`     | full   | Raspberry-Pi (Linux SBC) GPIO over sysfs (`fs` is the whole backend): `setup(pin, "in"/"out")`, `write(pin, 0/1)`, `read(pin)`, `release(pin)`. Stateless, pin-keyed; `JENNIFER_GPIO_BASE` overrides the sysfs root (tests / mounts). Absent-platform errors are clear, not crashes. |
| [`htmlwriter`](htmlwriter.md) | `import "htmlwriter.j";` | full | build an HTML element tree and render escaped HTML5. `element` / `text` / `raw` / `attr` constructors, `render` / `renderAll`, `escape`; void-element aware. A writer, not a parser. |
| [`http`](http.md)     | `import "http.j";`     | no (net) | an HTTP/1.1 client over `net` (`https://` via TLS): method-agnostic `request` plus `get` / `post` / `put` / `patch` / `delete` / `head` / `options` -> `Response` (`status`, `headers`, `body`); Content-Length + chunked framing, `header` case-insensitive lookup. Redirects returned, not followed. |
| [`ical`](ical.md)     | `import "ical.j";`     | full   | iCalendar (RFC 5545) build and parse: a `Calendar` of `Event`s encoded to a `VCALENDAR` of `VEVENT`s and parsed back. `calendar` / `event` / `describe` / `locate` / `add` value-semantic builders, `encode` / `parse`. `DTSTAMP` / `DTSTART` / `DTEND` go through `time` (UTC `DATE-TIME`); text values RFC 5545-escaped, long lines folded, so `parse(encode(cal))` round-trips. Pure text over `strings` / `lists` + `time`; both binaries. `VEVENT`-only (no `RRULE` / `VALARM` / `TZID`). |
| [`idna`](idna.md)     | `import "idna.j";`     | full   | internationalized domain names: `toAscii` / `toUnicode` over a Punycode (RFC 3492) core (`münchen.de` <-> `xn--mnchen-3ya.de`), plus `isAscii`. Used by the mail clients for hosts and envelope domains. |
| [`imap`](imap.md)     | `import "imap.j";`     | no (net) | receive mail (IMAP4rev1 client) over `net`: `LOGIN` / `SELECT` / `SEARCH` / `FETCH` (with literals) / `LOGOUT`. `connect` / `selectMailbox` / `search` / `fetch` / `logout`, plus `fetchAll`; messages parsed by `mime`. A reading subset. |
| [`influxdb`](influxdb.md) | `import "influxdb.j";` | no (net) | an InfluxDB 1.x time-series client on `http`: build line-protocol points with value-semantic builders (`point` / `tag` / `field` / `intField` / `stringField` / `boolField` / `at`, mixed field types via pre-rendered fragments), `line` / `write` them, and `query(client, influxql) -> Result` (a `list of Series` with stringified cells, prometheus-retrieval shape). Basic auth; nanosecond precision. Needs the default binary. |
| [`ipnet`](ipnet.md)   | `import "ipnet.j";`    | full   | IP addresses and CIDR networks, IPv4 and IPv6. `parseAddress` / `toString` (canonical, RFC 5952 for IPv6) / `version` / `equal`; `parse(cidr) -> Network` (host bits zeroed), `contains(net, addr)`, `netmask` / `broadcast` / `networkString`. Addresses held as raw `bytes` (4 / 16); bitwise subnet math for allow-lists. Pure `.j` over `strings` + `convert`; both binaries. |
| [`jsonl`](jsonl.md)   | `import "jsonl.j";`    | full   | JSON Lines (JSONL / NDJSON): newline-delimited JSON, one `json.Value` per line. `encode` / `decode` (compact JSON split / joined on `\n`, blank lines skipped, so `decode(encode(rows))` round-trips), whole-file `readFile` / `writeFile` / `appendFile`, and a streaming `Reader` (`openReader` / `hasMore` / `readRecord` / `closeReader`) for files too large to hold in memory. A thin framing layer over `json` + `fs`; both binaries. |
| [`label`](label.md)   | `import "label.j";`    | partial | industrial label printing in a build / render / emit pipeline. Build a device-independent `Label` in millimetres (`new` / `text` / `barcode` / `box` / `image` / `quantity`; barcodes code128 / ean13 / itf / code39 / gs1-128 / datamatrix / qr), `render(label, device)` to a selectable dialect (`"zpl"` Zebra, `"cab"` cab JScript), then emit - `send(host, port, rendered)` to a printer's raw `:9100` port. Build / render run on both binaries; `send` needs the default binary. |
| [`log`](log.md)       | `import "log.j";`      | partial | leveled, structured logging: a `Logger` carries a minimum level (`debug` < `info` < `warn` < `error`), a format (`text` / `logfmt` / `json`), and a sink; `debug` / `info` / `warn` / `error` (`at` for a runtime level) render one record - timestamp, level, message, `map of string to string` fields - and write it, dropping records below the level. Sinks `new` (stdout) / `toStderr` / `toFile` / `toSyslog` (RFC 5424 over UDP). Console + file work on both binaries; the syslog sink needs the default binary. |
| [`markdown`](markdown.md) | `import "markdown.j";` | full | render a small CommonMark subset (headings, emphasis, links, lists, code, GFM tables) to HTML (through `htmlwriter`) and styled terminal text (through `ansi`) with `toHtml` / `toAnsi`, plus authoring helpers (`header` / `style` / `link` / `bullets` / `numbered` / `codeBlock` / `table`) that build Markdown, and `tablePretty` to align table source. |
| [`memcache`](memcache.md) | `import "memcache.j";` | no (net) | a memcached client (classic text protocol) over `net`: `set` / `add` / `get` / `delete` / `incr` / `decr` / `touch`, every store with a TTL. For caches, sessions, counters, and locks (a volatile store, not a system of record). |
| [`mime`](mime.md)     | `import "mime.j";`     | full   | build and parse MIME messages (RFC 5322 headers, multipart, quoted-printable / base64 transfer encodings, RFC 2047 encoded-words for non-ASCII headers). `text` / `attachment` / `multipart` / `encode` / `parse`; the foundation the mail clients build on. |
| [`mqtt`](mqtt.md)     | `import "mqtt.j";`     | no (net) | an MQTT 3.1.1 pub/sub client over `net` (`mqtts` via TLS): `connect` -> `Client`, then `subscribe` / `publish` / `publishBytes` (QoS 0), blocking `receive` and single-threaded `poll(client, timeoutMs)` (via `net.setDeadline`), `ping`, `disconnect`. Binary packet framing built with bitwise ops + `bytes`. Basics-first (QoS 0). |
| [`multipart`](multipart.md) | `import "multipart.j";` | full | build and parse `multipart/form-data` (RFC 7578) - the file-upload counterpart to `mime`. `field` / `file` build `Part`s, `build` / `buildWith` -> `Built{contentType, body}` (fresh or fixed boundary), `parse(contentType, body) -> list of Part`; `text` / `isFile` read a part. Byte-level delimiter matching so binary bodies round-trip. Pairs with `web` / `http`. Pure `.j`; both binaries. |
| [`ntp`](ntp.md)       | `import "ntp.j";`      | no (net) | a simple SNTP network-time client (RFC 4330 / 5905) over UDP: `query(host)` / `queryWith(address, timeoutMs)` -> `Result` (`serverTime` + clock `offset` + round-trip `delay`). Packs / unpacks the 48-byte NTP packet with `bytes` + bitwise ops and converts the NTP epoch through `time`; a lost reply times out (not hangs). Query-only (no clock discipline / daemon). Needs the default binary. |
| [`oauth`](oauth.md)   | `import "oauth.j";`    | no (net) | a generic OAuth2 client (the get-a-token half) on `http` + `json`: Client Credentials, Refresh Token, and Device Authorization grants, `google` / `microsoft` presets, expiry + on-disk token store. Tokens feed `sasl` XOAUTH2 for mail. |
| [`password`](password.md) | `import "password.j";` | full | generate / validate / score passwords against a policy `Schema` (classes, length range, per-class minimums, symbol set, exclude-ambiguous). `schema` + `with*` builders, `generate -> string`, `validate -> Report{valid, reasons}`, `complexity -> Strength{length, classes, poolSize, entropy, label}` (bits = length * log2(pool)). Non-crypto RNG (like `uuid`; swaps to `crypto` later); pure `.j`, both binaries. |
| [`pdfwriter`](pdfwriter.md) | `import "pdfwriter.j";` | full | generate simple PDF documents (text / lines / rectangles) the way `htmlwriter` / `label` generate their formats: `document` / `page` / `text` / `line` / `rect` / `color` / `addPage` builders, `info` metadata (+ `pdfDate`), `render() -> bytes` writing the PDF object / xref structure by hand with FlateDecode content streams (via `compress`). Standard-14 fonts; points, 0-255 RGB. Byte-identical (no timestamps), `qpdf`-clean output - golden-test friendly; both binaries. A writer, not a reader (no embedded fonts / images yet). |
| [`pop`](pop.md)       | `import "pop.j";`      | no (net) | receive mail (POP3 client) over `net`: plaintext / STLS / implicit TLS, `USER` / `PASS`. `connect` / `stat` / `sizes` / `retrieve` / `deleteMessage` / `quit`, plus `fetchAll`; messages parsed by `mime`. |
| [`prometheus`](prometheus.md) | `import "prometheus.j";` | partial | metrics in two halves. **Exposition** (`counter` / `gauge` / `observe` / `render`) builds a metric set and renders the Prometheus text format - pure text, runs on both binaries. **Retrieval** (`query` / `queryRange` -> `Result`) is a read client for the HTTP query API over `http` + `json`, so it needs the default binary. Strict name / label validation and escaping. |
| [`ratelimit`](ratelimit.md) | `import "ratelimit.j";` | no (net) | a fixed-window rate limiter on `memcache` (atomic `incr` + per-key TTL): `allow(mc, key, limit, window)` -> bool, `remaining(mc, key, limit)`. The window resets on its own when it expires. |
| [`redis`](redis.md)   | `import "redis.j";`    | no (net) | a Redis client speaking RESP2 over `net`: commands as RESP arrays, replies parsed into a `Reply`. Typed helpers `get` / `set` / `del` / `exists` / `incr` / `keys` / `ping`, plus a generic `command` for the rest. |
| [`resque`](resque.md) | `import "resque.j";`   | no (net) | background jobs on Redis, wire-compatible with Resque: `enqueue` onto named queues, `reserve` from a worker in priority order (`Job` = `queue` / `class` / `args`), `queueLength` / `queues` / `size` / `fail`. Interops with Ruby-resque / php-resque workers. Built on `redis` + `json`. |
| [`rest`](rest.md)     | `import "rest.j";`     | no (net) | an ergonomic REST layer over `http` + `json`: a value-semantic `Client` (base URL + headers) and `get` / `post` / `put` / `patch` / `delete` (+ `getJson` / `postJson` / ...). Base-URL joining, query strings, Bearer / Basic auth. |
| [`ringbuffer`](ringbuffer.md) | `import "ringbuffer.j";` | full | a fixed-capacity ring buffer (bounded FIFO of strings, overwrite-oldest when full): `new(capacity)`, `push` / `pop`, `first` / `last` peek, `size` / `capacity` / `isEmpty` / `isFull` / `toList`. A sliding window of recent items. Value-semantic. Over `lists`; both binaries. |
| [`sasl`](sasl.md)     | `import "sasl.j";`     | full   | SASL auth encoders shared by the mail clients: `plain` / `loginUser` / `loginPass` / `bearer` (XOAUTH2, the "use a token" half of OAuth2). Pure base64, no networking. |
| [`semver`](semver.md) | `import "semver.j";`   | full   | strict Semantic Versioning 2.0.0, package-registry-grade. `parse` / `isValid` / `toString`, `compare` / `lt` / `lte` / `eq` / `neq` / `gt` / `gte` / `diff`, `isStable` / `isPrerelease`, `inc*`, `sort` / `rsort`; `coerce` / `clean` for loose tags; and npm/Composer **range matching** - `satisfies` (caret / tilde / comparators / `\|\|` / hyphen / x-ranges, prerelease-aware), `maxSatisfying` / `minSatisfying` / `minVersion` / `validRange`, plus solver algebra `intersects` / `subset` / `gtr` / `ltr` / `outside` / `simplifyRange` (prerelease-precise). Struct `Version`. |
| [`session`](session.md) | `import "session.j";` | no (net) | server-side sessions on `memcache`: a `map of string to string` under `sess:ID` with a sliding TTL. `create` / `load` / `save` / `touch` / `destroy`; UUID v4 IDs, base64-wrapped JSON values. Volatile (a cache, not a store of record). |
| [`slack`](slack.md)   | `import "slack.j";`    | no (net) | post to a Slack Incoming Webhook on `http`: `send(webhookUrl, text)` for a plain message, or build a Block Kit message with `message` / `text` / `section` / `header` / `divider` and post it with `sendMessage`. `render` gives the JSON payload; strings JSON-escaped. Sibling of `discord` / `gotify`. Needs the default binary. |
| [`smtp`](smtp.md)     | `import "smtp.j";`     | no (net) | send mail (SMTP client) over `net`: plaintext / STARTTLS / implicit TLS, `AUTH PLAIN`, `MAIL FROM` / `RCPT TO` / `DATA`. `smtp.send(opts, from, recipients, message)`; message built by `mime`. |
| [`statsd`](statsd.md) | `import "statsd.j";`   | no (net) | a fire-and-forget StatsD metrics client over UDP: `client` / `clientWith` -> `Client` (agent address + optional name prefix), then `count` / `increment` / `decrement` (counter `c`), `gauge` (`g`), `timing` (`ms`), `set` (`s`) each emit one `metric:value\|type` datagram. The push counterpart to a pull-based scrape; no reply, no error when no agent listens. No sample rates / tags yet. Needs the default binary. |
| [`telegram`](telegram.md) | `import "telegram.j";` | no (net) | a Telegram Bot API client on `http` + `json`: `bot` / `botWith` -> `Bot`, `getMe`, `sendMessage` / `sendMessageWith` (parse mode) / `sendPhoto` / `sendChatAction` -> `Message` / `bool`, and `getUpdates(bot, offset, timeout)` long-poll -> `list of Update` for a stateful receive loop. Form-encoded params, `{"ok":false}` throws. Needs the default binary. |
| [`tengine`](tengine.md) | `import "tengine.j";` | full   | a lightweight-CMS text template engine (a subset of Go `text/template`) over a `json.Value` tree: `newSet` / `add` / `render`. `.path` / `$` root / `$var`, `if` / `else if` with `eq` / `and` / `or` / `not`, `range` (with `$i, $e`) / `with` / `block`, `{{ $x := }}` variables, `define` / `template` layout inheritance, `{{- -}}` trim markers, and pipes `upper` / `lower` / `title` / `trim` / `html` / `urlize` / `default` / `truncate` / `join` / `len` / `printf`. |
| [`totp`](totp.md)     | `import "totp.j";`     | full   | time-based one-time passwords (RFC 6238 / 4226): `generate` / `verify` (+/-1-step skew) and `generateAt` / `verifyAt` (explicit time), plus `uri` for the `otpauth://` provisioning string. base32 secrets; SHA-1 / SHA-256 / SHA-512. Over `hash.hmac` + `encoding` + `time`. |
| [`vcard`](vcard.md)   | `import "vcard.j";`    | full   | vCard (RFC 6350, vCard 4.0) contacts build and parse: a `Card` of contact fields encoded to a `VCARD` and parsed back. `card` / `withName` / `withOrg` / `addEmail` / `addPhone` / `address` / `addAddress` / `withUrl` / `withNote` value-semantic builders, `encode` / `encodeAll` / `parse` (one or many cards). Structured `N` / `ADR` / `ORG`, RFC 6350 text escaping and 75-char line folding - shares the content-line codec with `ical`. Pure text over `strings` / `lists`; both binaries. A contact subset (no `BDAY` / `PHOTO` / parameter round-trip). |
| [`web`](web.md)       | `import "web.j";`      | no (net) | a small HTTP framework over the `httpd` engine: register routes against handler methods by name (`web.get` / `post` / ...), `:param` capture, middleware, `web.Context` request / response helpers; `web.run` owns the accept loop. Dispatch by `meta.callMain`. Pairs with `jennifer serve`. |
| [`webhook`](webhook.md) | `import "webhook.j";` | full (`send` net) | HMAC-signed webhooks (GitHub `X-Hub-Signature-256`): `sign(payload, secret)` / `verify(payload, signature, secret)` are pure (both binaries); `send(url, payload, secret)` POSTs the signed body via `http` (default binary). Over `hash.hmac` + `encoding` (hex). |
| [`websocket`](websocket.md) | `import "websocket.j";` | no (net) | an RFC 6455 WebSocket client over `net` (`ws://` / `wss://`): `connect` / `connectWith` do the HTTP Upgrade handshake (verifying the SHA-1 + base64 `Sec-WebSocket-Accept`), then `send` / `sendBytes` (masked frames) and `receive` -> `Message` (auto-pong, fragment reassembly), `ping` / `close`. Binary framing + masking with the bitwise ops over `hash` + `encoding` + `math`. Needs the default binary. |

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
