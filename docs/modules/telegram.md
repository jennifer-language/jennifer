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

Runnable: [`examples/modules/telegram_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/telegram_demo.j).

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
| `telegram.sendMessageWithKeyboard(bot, chatId, text, rows)` | `Message` | with an inline keyboard attached (see below) |
| `telegram.sendPhoto(bot, chatId, photo, caption)` | `Message` | send a photo by URL or file id, with an optional caption |
| `telegram.sendPhotoFile(bot, chatId, filePath, contentType)` | `Message` | upload a photo from a **local file** (multipart) |
| `telegram.sendDocumentFile(bot, chatId, filePath, contentType)` | `Message` | upload any document from a local file (multipart) |
| `telegram.sendChatAction(bot, chatId, action)` | `bool` | show activity (`"typing"`, `"upload_photo"`, ...) |
| `telegram.answerCallbackQuery(bot, callbackId, text)` | `bool` | acknowledge a button press (`text` = optional notification) |
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

## Inline keyboards

Attach a grid of buttons to a message. A `Button` carries either a `url` (opens
a link) or a `callbackData` (fires a `callback_query` back to the bot); build one
with `urlButton` / `inlineButton` rather than the raw literal. Rows are a
`list of list of Button` - each inner list is one row.

| Call | Returns | |
| ---- | ------- | - |
| `telegram.urlButton(text, url)` | `Button` | a button that opens `url` |
| `telegram.inlineButton(text, callbackData)` | `Button` | a button that sends `callbackData` back as a `callback_query` |
| `telegram.renderInlineKeyboard(rows)` | `string` | render `rows` to the `reply_markup` JSON (pure, no network) |

```jennifer
def rows as list of list of telegram.Button init [
    [ telegram.urlButton("Docs", "https://example.org"),
      telegram.inlineButton("Ping", "ping") ],
    [ telegram.inlineButton("Close", "close") ]
];
telegram.sendMessageWithKeyboard($bot, 12345678, "pick one", $rows);
```

`renderInlineKeyboard` is pure - it produces the exact JSON
`sendMessageWithKeyboard` sends as the `reply_markup` parameter (a URL button
emits `{"text":...,"url":...}`, a callback button `{"text":...,"callback_data":...}`),
so you can assert its shape without a network round-trip.

## Callback queries

When a user presses a callback button, Telegram delivers a `callback_query`
update. `getUpdates` sets `hasMessage` false for it (it is not a text message);
reach the raw JSON via a direct `http` call, then parse it:

| Call | Returns | |
| ---- | ------- | - |
| `telegram.parseCallbackQuery(update)` | `CallbackQuery` | parse a raw update `json.Value` (pure, no network) |
| `telegram.answerCallbackQuery(bot, callbackId, text)` | `bool` | ack the press (`text` = optional brief notification) |

```jennifer
def struct telegram.CallbackQuery {
    id as string,       # pass to answerCallbackQuery
    from as User,       # who pressed the button
    data as string,     # the button's callbackData
    messageId as int    # the message the button is attached to (0 if absent)
};
```

Always `answerCallbackQuery` a press, even with an empty `text` - otherwise the
client shows a spinner until it times out.

## Local-file upload

`sendPhoto` takes a URL or a Telegram file id; to upload a file from disk, use
`sendPhotoFile` / `sendDocumentFile`. They read the file with `fs.readBytes`,
build a `multipart/form-data` body via the [`multipart`](multipart.md) module (a
`chat_id` field plus the file part, named from the path's basename), and POST it.
Pass `contentType` as `""` to let Telegram sniff the format.

```jennifer
telegram.sendPhotoFile($bot, 12345678, "chart.png", "image/png");
telegram.sendDocumentFile($bot, 12345678, "report.pdf", "application/pdf");
```

`telegram.buildUpload(field, chatId, filename, contentType, data)` is the pure
builder underneath - given the file bytes it returns a `multipart.Built`
(`contentType` header + `body`), so the upload shape is testable with no disk or
network.

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

## Security: token redaction

The bot token is a secret that rides in the request URL **path**
(`baseUrl/bot<token>/<method>`). A transport error from the underlying `http`
layer can carry that URL - and thus the token - in its message, so this module
scrubs it: every error rethrown from an HTTP call passes through
`telegram.redactToken(s, token)`, which replaces the `bot<token>` path segment
with `bot<redacted>` and any bare token occurrence with `<redacted>`. A caught
`Error{kind: "telegram"}` (or a leaked log line) therefore never exposes the
token. `redactToken` is exported so you can apply the same scrubbing to your own
log output.

## Scope

- **Long-poll only**, no webhook receiver (that needs a public HTTPS server;
  compose [`web`](web.md) / [`httpd`](../libraries/httpd.md) yourself).
- **Text-centric updates.** `Update` surfaces the `message` (text) shape;
  `edited_message`, `channel_post`, and inline queries set `hasMessage` false -
  reach the raw JSON via a direct `http` call. A `callback_query` (a button
  press) is parsed by `parseCallbackQuery`.
- **UTF-8 upload bodies.** `sendPhotoFile` / `sendDocumentFile` send the
  multipart body through the text `http` client, so a file whose bytes are not
  valid UTF-8 is rejected on send; the pure `buildUpload` builder is byte-exact
  regardless. A byte-safe multipart send path is a follow-on.
- **No message editing or deletion** in this version.

## See also

- [http.md](http.md) - the HTTP client this module builds on.
- [slack.md](slack.md) / [discord.md](discord.md) / [gotify.md](gotify.md) - the
  one-shot notifier siblings.
- [modules/index.md](index.md) - the module catalog and import rules.
