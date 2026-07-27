# `gotify` - push notifications to a Gotify server

Import with `import "gotify.j" as gotify;`. A tiny module on top of the
[`http`](http.md) client that pushes a notification to a
[Gotify](https://gotify.net) server. Hold a value-semantic `Config` (server URL
+ application token) and call `push`. Because it builds on `http` (which uses
`net`), this module needs the default **`jennifer`** binary.

> **On `jennifer-tiny`:** "needs the default `jennifer` binary" refers to the
> **stock** tiny build, which ships without a network driver - not a TinyGo
> limitation. A `jennifer-tiny` rebuilt with a network stack runs this module
> too; see the
> [note on `net` and TinyGo](../technical/tinygo.md#net-on-tinygo-is-a-build-choice-not-a-hard-limit).

```jennifer
import "gotify.j" as gotify;
import "http.j" as http;

def g as gotify.Config init gotify.Config{url: "https://push.example.com",
    token: "AqB3cD..."};
def r as http.Response init gotify.push($g, "Deploy", "build 1234 is live", 5);
io.printf("pushed -> %d\n", $r.status);      # 200 on success
```

Runnable: [`examples/modules/gotify_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/gotify_demo.j).

## Surface

| Call / type                                            | Notes                                                            |
| ------------------------------------------------------ | ---------------------------------------------------------------- |
| `gotify.Config`                                        | `url` (server, no trailing slash) and `token` (application key). |
| `gotify.Extras`                                        | `markdown` (bool) and `clickUrl` (string) - optional message extras. |
| `gotify.push(cfg, title, message, priority)`           | POST the message form; returns the `http.Response`.              |
| `gotify.pushMarkdown(cfg, title, message, priority)`   | Render the body as markdown; returns the `http.Response`.        |
| `gotify.pushWith(cfg, title, message, priority, url)`  | Open `url` when the notification is tapped; returns the `http.Response`. |
| `gotify.pushExtras(cfg, title, message, priority, ex)` | Attach an arbitrary `Extras` (markdown and/or click URL).        |

`push` POSTs `title` / `message` / `priority` as
`application/x-www-form-urlencoded` to `cfg.url + "/message"` with an
`X-Gotify-Key: cfg.token` header - Gotify's
[push-message](https://gotify.net/docs/pushmsg) contract, where
[priority](https://gotify.net/docs/priority) is a plain int (0 lowest, higher is
more urgent). It returns the raw `http.Response`, so the caller checks
`.status`: a `200` on success, and a bad token comes back as a `4xx` **value**,
not a crash.

## Message extras (markdown, click action)

Gotify's [extras](https://gotify.net/docs/msgextras) let a message carry
client hints. Because extras are a nested object, the extras-carrying variants
POST a **JSON** body (`application/json`) instead of the form - the plain
`push` stays form-encoded and unchanged. Two extras are supported:

- **markdown** - `pushMarkdown` (or `Extras{markdown: true}`) sets
  `extras["client::display"]["contentType"] = "text/markdown"`, so the body
  renders as markdown.
- **click action** - `pushWith(..., clickUrl)` (or `Extras{clickUrl: url}`)
  sets `extras["client::notification"]["click"]["url"] = clickUrl`, so tapping
  the notification opens that URL.

Combine both with `pushExtras` and an `Extras` holding both fields. A zero
`Extras` (markdown false, empty `clickUrl`) adds no `extras` object, so
`pushExtras` with it matches a plain `push` (over JSON).

```jennifer
import "gotify.j" as gotify;

def g as gotify.Config init gotify.Config{url: "https://push.example.com", token: "tok"};

# markdown body
def a as http.Response init gotify.pushMarkdown($g, "Report", "# Done\n- built\n- shipped", 5);

# tap-to-open URL
def b as http.Response init gotify.pushWith($g, "Deploy", "build 1234 is live", 5,
    "https://ci.example.com/builds/1234");

# both at once
def ex as gotify.Extras init gotify.Extras{markdown: true, clickUrl: "https://ci.example.com/builds/1234"};
def c as http.Response init gotify.pushExtras($g, "Deploy", "**build 1234** is live", 5, $ex);
```

The JSON body `pushExtras` builds for the combined case is:

```json
{"title":"Deploy","message":"done","priority":8,"extras":{"client::display":{"contentType":"text/markdown"},"client::notification":{"click":{"url":"https://x.example/go"}}}}
```

## Stateless by design

There is no `init()` that stashes the URL and token - a module has no mutable
state. The caller holds the value-semantic `Config` and passes it to each
`push`, the same shape as the `time` / `hash` structs. The URL and token are
**yours to supply and never commit**: read them from the environment or a config
file. The demo reads `GOTIFY_URL` / `GOTIFY_TOKEN` from the environment; the docs
use placeholders.

## See also

- [http.md](http.md) - the client `gotify` posts through.
- [modules/index.md](index.md) - the module catalog and import rules.
