# `telegram` - Telegram Bot API client

Import with `import "telegram.j" as telegram;`. Drive a Telegram bot over the
[Bot API](https://core.telegram.org/bots/api): send messages and photos,
identify the bot, and long-poll for incoming updates. Built on the
[`http`](http.md) module + `json`, so it needs the default `jennifer` binary.
Larger than the one-shot notifiers ([`slack`](slack.md) / [`discord`](discord.md)
/ [`gotify`](gotify.md)) - `getUpdates` drives a stateful receive loop. An API
error (`{"ok": false, ...}`) throws `Error{kind: "telegram"}`.

```jennifer
import "telegram.j" as telegram;

def bot as telegram.Bot init telegram.bot("123456:ABC-DEF...");   # token from @BotFather
telegram.sendMessage($bot, 12345678, "hello from Jennifer");

def updates as list of telegram.Update init telegram.getUpdates($bot, 0, 30);
```

Runnable: [`examples/modules/telegram_demo.j`](https://github.com/mplx/jennifer-lang/blob/main/examples/modules/telegram_demo.j).

## The bot

```jennifer
def struct telegram.Bot { token as string, baseUrl as string };
```

| Call | Returns | |
| ---- | ------- | - |
| `telegram.bot(token)` | `Bot` | a bot against the public Telegram API |
| `telegram.botWith(token, baseUrl)` | `Bot` | a bot against a custom API base (a self-hosted Bot API server, or a test endpoint) |

The token is a secret - read it from the environment, never commit it. Every
call POSTs `application/x-www-form-urlencoded` params to
`baseUrl/bot<token>/<method>` and verifies the `{"ok": true}` envelope,
throwing on an API error with the server's `description`.

## Sending

| Call | Returns | |
| ---- | ------- | - |
| `telegram.sendMessage(bot, chatId, text)` | `Message` | send a text message |
| `telegram.sendMessageWith(bot, chatId, text, parseMode)` | `Message` | with a parse mode (`"Markdown"`, `"MarkdownV2"`, `"HTML"`, or `""`) |
| `telegram.sendPhoto(bot, chatId, photo, caption)` | `Message` | send a photo by URL or file id, with an optional caption |
| `telegram.sendChatAction(bot, chatId, action)` | `bool` | show activity (`"typing"`, `"upload_photo"`, ...) |
| `telegram.getMe(bot)` | `User` | the bot's own identity (a good token / connectivity check) |

`chatId` is an integer (Telegram user, group, or channel id - channel ids are
large and negative, which fits Jennifer's 64-bit `int`). A returned `Message`
carries the text-relevant fields:

```jennifer
def struct telegram.Message {
    messageId as int,   # the message id
    chatId as int,      # the chat it belongs to
    text as string,     # the text ("" for non-text messages)
    date as int         # send time as a Unix timestamp
};
def struct telegram.User {
    id as int, isBot as bool, firstName as string, username as string
};
```

## Receiving (long-poll)

`telegram.getUpdates(bot, offset, timeout)` long-polls for pending updates.
Pass `offset` as the last processed `updateId + 1` (`0` on the first call) and
`timeout` as the wait in seconds; the HTTP read is bounded a few seconds beyond
that. Returns a `list of Update`:

```jennifer
def struct telegram.Update {
    updateId as int,       # advance the next poll offset to this + 1
    hasMessage as bool,    # whether this update carries a text-message
    message as Message     # the message (zero-valued when hasMessage is false)
};
```

The receive-loop pattern - fetch, process, advance the offset past each update:

```jennifer
def offset as int init 0;
def updates as list of telegram.Update init telegram.getUpdates($bot, $offset, 30);
for (def u in $updates) {
    $offset = $u.updateId + 1;
    if ($u.hasMessage and len($u.message.text) > 0) {
        telegram.sendMessage($bot, $u.message.chatId, "echo: " + $u.message.text);
    }
}
# next loop: telegram.getUpdates($bot, $offset, 30)
```

## Scope

- **Long-poll only**, no webhook receiver (that needs a public HTTPS server;
  compose [`web`](web.md) / [`httpd`](../libraries/httpd.md) yourself).
- **Text-centric updates.** `Update` surfaces the `message` (text) shape;
  `edited_message`, `channel_post`, `callback_query`, and inline queries set
  `hasMessage` false - reach the raw JSON via a direct `http` call if you need
  them.
- **No multipart upload.** `sendPhoto` takes a URL or file id, not a local file
  (multipart `multipart/form-data` upload is a follow-on).
- **No inline keyboards / reply markup, no message editing or deletion** in this
  version.

## See also

- [http.md](http.md) - the HTTP client this module builds on.
- [slack.md](slack.md) / [discord.md](discord.md) / [gotify.md](gotify.md) - the
  one-shot notifier siblings.
- [modules/index.md](index.md) - the module catalog and import rules.
