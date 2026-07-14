# `log` - leveled structured logging

Import with `import "log.j" as log;`. Leveled, **structured** logging: a
`log.Logger` carries a minimum level, an output format, and a sink;
`log.info(logger, message, fields)` (and the sibling levels) render one record -
a timestamp, the level, the message, and the caller's key/value `fields` - and
write it, dropping records below the logger's level.

```jennifer
import "log.j" as log;

def lg as log.Logger init log.new("info", "logfmt");
def f as map of string to string init {"user": "ada", "id": "42"};
log.info($lg, "user logged in", $f);
# time=2026-... level=info msg="user logged in" user=ada id=42
```

Runnable: [`examples/modules/log_demo.j`](https://github.com/mplx/jennifer-lang/blob/main/examples/modules/log_demo.j).

## Loggers

A `log.Logger` is value-semantic; build one with a constructor that fixes the
sink:

| Constructor | Sink |
| ----------- | ---- |
| `log.new(level, format)` | standard output |
| `log.toStderr(level, format)` | standard error |
| `log.toFile(level, format, path)` | append to a file (created if missing) |
| `log.toSyslog(level, address, app)` | an RFC 5424 syslog server over UDP |

`level` is the **minimum** level to emit - one of `"debug"` < `"info"` <
`"warn"` < `"error"` - so a logger at `"info"` silently drops `debug` records.
`format` is `"text"`, `"logfmt"`, or `"json"` (the syslog sink uses RFC 5424
framing and ignores it).

## Levels

Each level has a method taking the logger, a message, and a
`map of string to string` of fields (`{}` for none):

| Method | |
| ------ | - |
| `log.debug(logger, message, fields)` | verbose / development detail |
| `log.info(logger, message, fields)` | normal operation |
| `log.warn(logger, message, fields)` | something unexpected but handled |
| `log.error(logger, message, fields)` | a failure |
| `log.at(logger, level, message, fields)` | emit at a level chosen at runtime |

A record below the logger's level is dropped before rendering, so guarded
`log.debug` calls in hot paths cost only the level comparison.

## Formats

The same record - timestamp, level, message, fields - in each format:

| Format | Example |
| ------ | ------- |
| `text` | `2026-01-02T03:04:05Z INFO user logged in user=ada id=42` |
| `logfmt` | `time=2026-01-02T03:04:05Z level=info msg="user logged in" user=ada id=42` |
| `json` | `{"time":"2026-01-02T03:04:05Z","level":"info","msg":"user logged in","user":"ada","id":"42"}` |

Timestamps are RFC 3339 in UTC. A field value containing a space, a quote, or an
`=` is double-quoted (with inner quotes escaped) in the `text` / `logfmt` forms,
so pairs stay parseable; the `json` form is a proper object with the field keys
alongside `time` / `level` / `msg` (a field named `time` / `level` / `msg`
overrides the built-in key).

## Sinks and portability

- **`stdout` / `stderr`** (via `io.printf` / `io.eprintf`) and **`file`** (via
  `fs.appendString`) work on **both** binaries.
- **`syslog`** sends each record as an RFC 5424 datagram over **UDP** (`net`):
  `<PRI>1 TIMESTAMP HOST APP - - - MSG`, where the priority is facility `user`
  (1) times 8 plus the level's severity (`error`=3, `warn`=4, `info`=6,
  `debug`=7), the host is `$HOSTNAME` (or `-`), and the fields ride in the
  message as logfmt pairs. Because it uses `net`, the syslog sink needs the
  default **`jennifer`** binary - the module is therefore **partial** on
  `jennifer-tiny` (console and file logging work; the syslog sink returns the
  no-network error).

## Scope

- **String fields.** `fields` is a `map of string to string` - convert numbers /
  bools to strings at the call site (`convert.toString`). This keeps the record
  model simple and the value quoting predictable.
- **UDP syslog.** No TCP / TLS syslog transport, and no local `/dev/log` socket.
- **No global default logger.** Loggers are values you pass explicitly - no
  hidden singleton, no package-level state.

## See also

- [io.md](../libraries/io.md) - `printf` / `eprintf`, the stdout / stderr sinks.
- [fs.md](../libraries/fs.md) - `appendString`, the file sink.
- [net.md](../libraries/net.md) - the UDP transport the syslog sink uses.
- [modules/index.md](index.md) - the module catalog and import rules.
