# `discord` - Discord Webhook client

Import with `import "discord.j" as discord;`. Post messages to a Discord channel
through a [channel Webhook](https://discord.com/developers/docs/resources/webhook),
on top of the [`http`](http.md) module - a sibling of [`gotify`](gotify.md) and
[`slack`](slack.md). Needs the default `jennifer` binary. The webhook URL is a
secret: read it from the environment or a config file, never commit it.

```jennifer
import "discord.j" as discord;

discord.send("https://discord.com/api/webhooks/1/xxx", "deploy finished");

def m as discord.Message init discord.embed(
    discord.content(discord.message(), "heads up"),
    "Deploy", "build 1234 is live", 3066993);
discord.sendMessage("https://discord.com/api/webhooks/1/xxx", $m);
```

Runnable: [`examples/modules/discord_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/discord_demo.j).

## Plain messages

`discord.send(webhookUrl, content)` posts `{"content": content}` (the content is
Discord markdown) and returns the [`http.Response`](http.md) - Discord answers
`204 No Content` on success.

## Rich messages (embeds)

Build a message from embeds with value-semantic builders - each returns a fresh
`Message`, so they chain - then post it with `sendMessage` (or inspect the JSON
with `render`).

```jennifer
def struct discord.Message {
    content as string,       # top-level content ("" to omit; embeds must then be present)
    embeds as list of Embed, # the message embeds
    username as string,      # "" or a per-message username override
    avatar as string         # "" or a per-message avatar image URL override
};

def struct discord.Embed {
    title as string,
    description as string,
    color as int,
    fields as list of string, # pre-rendered field JSON fragments
    footer as string,         # "" or a pre-rendered footer JSON object
    author as string          # "" or a pre-rendered author JSON object
};
```

| Call | Returns | |
| ---- | ------- | - |
| `discord.message()` | `Message` | start an empty message |
| `discord.content(m, content)` | `Message` | set the top-level content |
| `discord.username(m, name)` | `Message` | override the webhook's display name |
| `discord.avatar(m, url)` | `Message` | override the webhook's avatar image |
| `discord.embed(m, title, description, color)` | `Message` | append an embed |
| `discord.embedField(m, name, value, inline)` | `Message` | add a field to the latest embed |
| `discord.embedFooter(m, text)` | `Message` | set the footer of the latest embed |
| `discord.embedAuthor(m, name)` | `Message` | set the author of the latest embed |
| `discord.render(m)` | `string` | render the JSON payload |
| `discord.sendMessage(webhookUrl, m)` | `http.Response` | post the built message |

`color` is the embed's left-bar colour as a **decimal** RGB integer (e.g.
`3066993` for green, `0xFF0000` = `16711680` for red). At least one of `title` /
`description` should be non-empty. All strings are JSON-escaped for you, so
quotes and newlines are safe. `content` and `embeds` are each emitted only when
present, so a plain `send`, an embeds-only message, and a content-plus-embeds
message are all valid.

### Embed fields, footer, and author

`embedField` / `embedFooter` / `embedAuthor` decorate the **most recent** embed
(the one last appended by `embed`), so build the embed first, then add to it.
Each throws an `Error{kind: "discord"}` if the message has no embed yet.
`embedField` appends to the embed's `fields` array (`inline` lets Discord pack
several fields side by side); `embedFooter` sets `footer.text` and `embedAuthor`
sets `author.name`.

```jennifer
def m as discord.Message init discord.embed(discord.message(), "Deploy", "v1234", 3066993);
$m = discord.embedField($m, "Env", "production", true);
$m = discord.embedField($m, "Duration", "42s", true);
$m = discord.embedFooter($m, "ci bot");
$m = discord.embedAuthor($m, "release-pipeline");
# render($m) ->
# {"embeds":[{"title":"Deploy","description":"v1234","color":3066993,
#   "fields":[{"name":"Env","value":"production","inline":true},
#             {"name":"Duration","value":"42s","inline":true}],
#   "footer":{"text":"ci bot"},"author":{"name":"release-pipeline"}}]}
```

### Webhook identity override

`username` and `avatar` set the top-level `username` / `avatar_url` on the
payload, so a single webhook can post under different names and images.

```jennifer
def m as discord.Message init discord.content(discord.message(), "backup complete");
$m = discord.username($m, "backup-bot");
$m = discord.avatar($m, "https://example.com/backup.png");
# render($m) -> {"username":"backup-bot","avatar_url":"https://example.com/backup.png","content":"backup complete"}
```

## Scope

- **Channel Webhooks**, not the bot API - no bot token, gateway, slash
  commands, threads, reactions, or attachments. The channel is fixed by the
  webhook.
- **A subset of embeds** - title, description, color, fields, footer, and
  author, plus the `username` / `avatar_url` identity overrides. Embed
  thumbnail, image, and timestamp are not built here (compose the JSON yourself
  and post via [`http`](http.md) if you need them).
- **No retry / rate-limit handling** - a non-2xx is returned as the response
  value for the caller to inspect, not thrown.

## See also

- [http.md](http.md) - the HTTP client this module builds on.
- [slack.md](slack.md) / [gotify.md](gotify.md) - the sibling notifiers.
- [modules/index.md](index.md) - the module catalog and import rules.
