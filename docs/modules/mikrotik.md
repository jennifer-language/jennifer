# `mikrotik` - RouterOS API client

Import with `import "mikrotik.j" as mikrotik;`. Connect to a MikroTik RouterOS
device over its **binary API** (not SSH) and run commands. The API is plain TCP
(8728) or api-ssl (8729 over TLS); its wire protocol is sentence-based - a
sentence is a run of length-prefixed words ending in a zero-length word. Built
on [`net`](../libraries/net.md) (+ TLS), with an MD5 fallback via `hash`. Needs
the default `jennifer` binary. A `!trap` / `!fatal` reply throws
`Error{kind: "mikrotik"}`.

```jennifer
import "mikrotik.j" as mikrotik;

def s as mikrotik.Session init mikrotik.connect(mikrotik.options("192.168.88.1", "admin", "secret"));
def ifaces as list of map of string to string init mikrotik.print($s, "/interface");
def id as string init mikrotik.run($s, "/ip/address/add", {});   # (with attrs)
mikrotik.close($s);
```

Runnable: [`examples/modules/mikrotik_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/mikrotik_demo.j).

## Why the API, not SSH

A real SSH client needs key exchange, host-key verification, and cipher / MAC
negotiation - the whole crypto surface plus a heavy dependency, against the
dependency-free, TinyGo-clean stance. The RouterOS API is the purpose-built,
crypto-optional door: plaintext auth over plain TCP, or confidentiality via
api-ssl (TLS), exactly like the mail clients.

## Connecting

Login is **plaintext** (`=name=` / `=password=`, RouterOS 6.43+ and all v7); for
pre-6.43 routers the client automatically falls back to the MD5
challenge-response (which only needs `hash.compute(b, "md5")`).

```jennifer
def struct mikrotik.Options { host as string, port as int, user as string, password as string, tls as bool };
def struct mikrotik.Session { socket as net.Conn };
```

| Call | Returns | |
| ---- | ------- | - |
| `mikrotik.options(host, user, password)` | `Options` | plain TCP, port 8728 |
| `mikrotik.optionsTLS(host, user, password)` | `Options` | api-ssl (TLS), port 8729 |
| `mikrotik.withPort(o, port)` | `Options` | copy with a different port |
| `mikrotik.connect(opts)` | `Session` | connect and log in |
| `mikrotik.close(s)` | | close the connection |

## Commands

A command is a menu path (`/interface/print`); attributes are a `map of string
to string` sent as `=key=value` words. Each `!re` reply sentence folds into one
row map.

| Call | Returns | |
| ---- | ------- | - |
| `mikrotik.talk(s, command, attrs)` | `list of map of string to string` | the general call - the `!re` reply rows |
| `mikrotik.talkQuery(s, command, attrs, queries)` | `list of map of string to string` | like `talk`, plus raw `?...` **query words** to filter rows on the router |
| `mikrotik.print(s, path)` | `list of map of string to string` | read sugar for `path + "/print"` |
| `mikrotik.printWhere(s, path, queries)` | `list of map of string to string` | read sugar: `path + "/print"` filtered by query words |
| `mikrotik.run(s, command, attrs)` | `string` | for add / set / remove - returns the `!done` `=ret=` (e.g. a new item id) |

```jennifer
# read
for (def iface in mikrotik.print($s, "/interface")) {
    # $iface["name"], $iface["type"], $iface["running"]
}

# add (run returns the new item's id)
def attrs as map of string to string init {};
$attrs["address"] = "10.0.0.1/24";
$attrs["interface"] = "ether1";
def newId as string init mikrotik.run($s, "/ip/address/add", $attrs);

# filtered read: query words run on the router (each starts with "?")
def ethers as list of map of string to string init
    mikrotik.printWhere($s, "/interface", ["?type=ether", "?running=true"]);
# talkQuery is the general form - attributes plus queries (e.g. trim the columns):
def cols as map of string to string init {};
$cols[".proplist"] = "name,type";
def named as list of map of string to string init
    mikrotik.talkQuery($s, "/interface/print", $cols, ["?type=ether"]);
```

Query words filter server-side (the router returns only matching rows), so they
are cheaper than fetching everything and filtering in Jennifer. Each is a raw
RouterOS query word starting with `?`: `?type=ether` (equals), `?disabled`
(property present), `?>mtu=1000` (greater), and the stack operators `?#!` / `?#&`
/ `?#|` for compound queries. A query word missing its leading `?` throws
`Error{kind: "mikrotik"}`.

## Scope

- **Binary API, v6 and v7.** The v7 REST API (HTTP + JSON) is a different,
  stateless shape and a possible second backend later; v1 ships the binary API.
- **Synchronous `talk`.** `.tag`-multiplexed concurrent commands are a follow-on -
  each call runs to its `!done` before the next. (Server-side **query words** are
  supported, via `talkQuery` / `printWhere`.)
- **`!trap` throws.** A command error surfaces as `Error{kind: "mikrotik"}`
  (the trailing `!done` is consumed first, so the session stays usable); a
  `!fatal` (connection closing) throws immediately.
- **String values.** Attributes and reply fields are strings, exactly as the API
  carries them - parse numbers / booleans yourself.

## See also

- [net.md](../libraries/net.md) - the TCP / TLS transport (+ `connectTLS` for
  api-ssl).
- [mqtt.md](mqtt.md) / [amqp.md](amqp.md) - the other hand-framed binary
  protocol clients.
- [modules/index.md](index.md) - the module catalog and import rules.
