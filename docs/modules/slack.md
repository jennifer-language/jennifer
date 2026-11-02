# `slack` - Slack Incoming Webhook client

> **Deprecated - moving to a deck.** `slack` leaves the bundled modules and
> continues as [`deck-slack`](https://github.com/jennifer-language/deck-slack).
>
> - **0.27.x** is the last release line that bundles `slack.j`.
> - **Removed in 0.28.0.** The development line drops it right after 0.27.0 is
>   released, so `0.27.x+dev` builds no longer have it either.
>
> To migrate, add the deck to your project's `deck.toml`, run `jvc install`, and
> change the import; the surface (`send`, `message`, `section`, `header`, `sendMessage`, ...) stays the same:
>
> ```jennifer
> import "@jennifer/slack/" as slack;   # was: import "slack.j" as slack;
> ```

Import with `import "slack.j" as slack;`. Post messages to a Slack channel
through an [Incoming Webhook](https://api.slack.com/messaging/webhooks), on top
of the [`http`](http.md) module - a sibling of [`gotify`](gotify.md) and
[`discord`](discord.md). Needs the default `jennifer` binary. The webhook URL is
a secret: read it from the environment or a config file, never commit it.

```jennifer
import "slack.j" as slack;

slack.send("https://hooks.slack.com/services/T/B/xxx", "deploy finished");

def m as slack.Message init slack.section(
    slack.header(slack.message(), "Deploy"), "*build 1234* is live");
slack.sendMessage("https://hooks.slack.com/services/T/B/xxx", $m);
```

Runnable: [`examples/modules/slack_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/slack_demo.j).

## Plain messages

`slack.send(webhookUrl, text)` posts `{"text": text}` (the text is Slack
mrkdwn) and returns the [`http.Response`](http.md) - Slack answers `200` with
body `ok` on success. The webhook's configured channel receives the message.

## Rich messages (Block Kit)

Build a message from [Block Kit](https://api.slack.com/block-kit) blocks with
value-semantic builders - each returns a fresh `Message`, so they chain - then
post it with `sendMessage` (or inspect the JSON with `render`).

```jennifer
def struct slack.Message {
    text as string,           # top-level fallback / notification text ("" to omit)
    blocks as list of string  # pre-rendered block JSON fragments
};
```

| Call | Returns | |
| ---- | ------- | - |
| `slack.message()` | `Message` | start an empty message |
| `slack.text(m, text)` | `Message` | set the fallback / notification text |
| `slack.section(m, markdown)` | `Message` | append a section block (mrkdwn) |
| `slack.header(m, heading)` | `Message` | append a header block (plain text) |
| `slack.contextBlock(m, text)` | `Message` | append a context block (small muted mrkdwn) |
| `slack.fieldsSection(m, fields)` | `Message` | append a section laid out as two-column fields |
| `slack.button(text, url)` | `string` | build a URL button fragment for `actionsBlock` |
| `slack.actionsBlock(m, buttons)` | `Message` | append an actions block of buttons |
| `slack.divider(m)` | `Message` | append a divider block |
| `slack.render(m)` | `string` | render the JSON payload |
| `slack.sendMessage(webhookUrl, m)` | `http.Response` | post the built message |

Text passed to any builder is JSON-escaped for you (via the `json` library), so
quotes and newlines cannot break the payload. JSON escaping does **not**
neutralise Slack's own markup: Slack decodes the JSON before it reads mrkdwn, so
untrusted text (a commit message, a user name) containing `<!channel>`,
`<!here>` or `<@U123>` still notifies people. This module has no escape helper;
the [`deck-slack`](https://github.com/jennifer-language/deck-slack) deck's
`slack.escape(text)` does it. The fallback `text` is
shown in notifications and by clients that do not render blocks - set it even
when you use blocks.

`fieldsSection` takes a `list of string` of mrkdwn snippets (Slack packs them
into a two-column grid - a common convention is `"*Key:*\nvalue"` per field).
For an actions block, build each button with `slack.button(text, url)` and pass
the list to `actionsBlock`:

```jennifer
def m as slack.Message init slack.message();
$m = slack.header($m, "Deploy");
$m = slack.fieldsSection($m, ["*Env:*\nprod", "*Build:*\n1234"]);
$m = slack.actionsBlock($m, [
    slack.button("View build", "https://ci.example.com/build/1234"),
    slack.button("Logs", "https://ci.example.com/build/1234/logs")
]);
$m = slack.contextBlock($m, "posted by ci-bot");
```

## Scope

- **Incoming Webhooks**, not the Web API - no bot tokens, `chat.postMessage`,
  threads, reactions, or file uploads. The channel is fixed by the webhook.
- **A subset of Block Kit** - `section` / `header` / `context` / `fields` /
  `actions` (URL buttons) / `divider`. Accessories, images, and non-button
  interactive elements are not built here (compose the JSON yourself and post
  via [`http`](http.md) if you need them).
- **No retry / rate-limit handling** - a non-2xx is returned as the response
  value for the caller to inspect, not thrown.

## See also

- [http.md](http.md) - the HTTP client this module builds on.
- [discord.md](discord.md) / [gotify.md](gotify.md) - the sibling notifiers.
- [modules/index.md](index.md) - the module catalog and import rules.
