# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * A Telegram Bot API client over the `http` client + `json`. Hold a `Bot`
 * (token + API base URL), send messages with `sendMessage` / `sendPhoto` /
 * `sendChatAction`, identify the bot with `getMe`, and poll for incoming
 * updates with `getUpdates` (long-poll). Larger than the one-shot notifiers
 * (`slack` / `discord` / `gotify`): `getUpdates` drives a stateful receive loop
 * where the caller advances the `offset` past each processed update.
 *
 * Needs the default `jennifer` binary (uses `net` via `http`). An API error
 * (`{"ok": false, ...}`) throws `Error{kind: "telegram"}`. The bot token is a
 * secret belonging to the caller - read it from the environment; never commit
 * it.
 * @module telegram
 * @example
 * import "telegram.j" as telegram;
 * def bot as telegram.Bot init telegram.bot("123456:ABC-DEF...");
 * def m as telegram.Message init telegram.sendMessage($bot, 12345678, "hello from Jennifer");
 * def updates as list of telegram.Update init telegram.getUpdates($bot, 0, 30);
 */
use json;
use strings;
use convert;
use lists;
use fs;
use path;
import "./http.j" as http;
import "./multipart.j" as multipart;
import "./uri.j" as uri;

# The public Telegram Bot API base (overridable for a self-hosted API server or
# tests via `botWith`).
def const DEFAULT_BASE as string init "https://api.telegram.org";
# Request timeout for the non-polling verbs, in milliseconds.
def const DEFAULT_TIMEOUT_MS as int init 30000;

/**
 * A bot: an API token and the API base URL.
 * @field token {string} the bot token from BotFather
 * @field baseUrl {string} the API base URL (no trailing slash)
 */
export def struct Bot {
    token as string,
    baseUrl as string
};

/**
 * A Telegram user (or bot).
 * @field id {int} the user id
 * @field isBot {bool} whether the user is a bot
 * @field firstName {string} the first name
 * @field username {string} the @username ("" if none)
 */
export def struct User {
    id as int,
    isBot as bool,
    firstName as string,
    username as string
};

/**
 * A message (the text-relevant fields).
 * @field messageId {int} the message id
 * @field chatId {int} the chat id the message belongs to
 * @field text {string} the message text ("" for non-text messages)
 * @field date {int} the send date as a Unix timestamp
 */
export def struct Message {
    messageId as int,
    chatId as int,
    text as string,
    date as int
};

/**
 * One polled update.
 * @field updateId {int} the update id (advance the poll `offset` to this + 1)
 * @field hasMessage {bool} whether this update carries a `message`
 * @field message {Message} the message (zero-valued when `hasMessage` is false)
 */
export def struct Update {
    updateId as int,
    hasMessage as bool,
    message as Message
};

/**
 * One inline-keyboard button. Exactly one action is set: a `url` (opens a link)
 * or a `callbackData` (fires a `callback_query` back to the bot). Build with
 * `urlButton` / `inlineButton` rather than the raw literal.
 * @field text {string} the button label
 * @field url {string} the URL to open ("" when this is a callback button)
 * @field callbackData {string} the callback payload ("" when this is a URL button)
 */
export def struct Button {
    text as string,
    url as string,
    callbackData as string
};

/**
 * An incoming button press (a Telegram `callback_query`). Acknowledge it with
 * `answerCallbackQuery` so the client stops its loading spinner.
 * @field id {string} the callback query id (pass to `answerCallbackQuery`)
 * @field from {User} the user who pressed the button
 * @field data {string} the button's `callbackData` ("" if none)
 * @field messageId {int} the id of the message the button is attached to (0 if absent)
 */
export def struct CallbackQuery {
    id as string,
    from as User,
    data as string,
    messageId as int
};

func fail(msg as string) {
    throw Error{kind: "telegram", message: "telegram: " + $msg, file: "", line: 0, col: 0};
}

/**
 * Redact a bot token from a string (an error message, a log line). The token is
 * a secret that rides in the request URL path (`/bot<TOKEN>/<method>`), so an
 * error carrying the URL would otherwise leak it. Replaces the `bot<TOKEN>` path
 * segment with `bot<redacted>` and any bare token occurrence with `<redacted>`.
 * Applied to every error message this module rethrows from the HTTP layer.
 * @param s {string} the string to scrub
 * @param token {string} the bot token to remove
 * @return {string} the string with the token removed
 */
export func redactToken(s as string, token as string) {
    def out as string init $s;
    if (len($token) > 0) {
        $out = strings.replace($out, "bot" + $token, "bot<redacted>");
        $out = strings.replace($out, $token, "<redacted>");
    }
    return $out;
}

# --- clients (exported) -----------------------------------------------------

/**
 * Create a bot against the public Telegram API.
 * @param token {string} the bot token
 * @return {Bot} a ready bot
 */
export func bot(token as string) {
    return botWith($token, DEFAULT_BASE);
}

/**
 * Create a bot against a specific API base URL (a self-hosted Bot API server,
 * or a test endpoint).
 * @param token {string} the bot token
 * @param baseUrl {string} the API base URL (no trailing slash)
 * @return {Bot} a ready bot
 */
export func botWith(token as string, baseUrl as string) {
    return Bot{token: $token, baseUrl: $baseUrl};
}

# --- form encoding (private) ------------------------------------------------

# formEncode builds an application/x-www-form-urlencoded body from a string map
# (keys in insertion order), via the shared `uri` module.
func formEncode(params as map of string to string) {
    return uri.buildQuery($params);
}

# --- request core (private) -------------------------------------------------

# checkResponse throws when the API reports `{"ok": false, ...}`.
func checkResponse(node as json.Value) {
    if (not json.asBool($node, "/ok")) {
        def desc as string init "request failed";
        if (json.has($node, "/description")) {
            $desc = json.asString($node, "/description");
        }
        fail($desc);
    }
}

# call POSTs a Bot API method with form params and returns the decoded response
# node (with a verified `ok: true`).
func call(b as Bot, method as string, params as map of string to string, timeoutMs as int) {
    def url as string init $b.baseUrl + "/bot" + $b.token + "/" + $method;
    def headers as map of string to string init {
        "Content-Type": "application/x-www-form-urlencoded"
    };
    # A transport error from the http layer can carry the request URL - and thus
    # the token - in its message. Rethrow it with the token redacted so a leaked
    # log or a caught Error never exposes the secret.
    def resp as http.Response;
    try {
        $resp = http.requestWith("POST", $url, $headers, formEncode($params), $timeoutMs, 0);
    } catch (e) {
        fail(redactToken($e.message, $b.token));
    }
    # A proxy error page (502 HTML, auth portal) isn't JSON: decode under a
    # guard and rethrow as a telegram-kind error rather than a raw json one.
    def node as json.Value;
    try {
        $node = json.decode($resp.body);
    } catch (e) {
        fail("non-JSON response (HTTP " + convert.toString($resp.status) + ")");
    }
    if (not json.has($node, "/ok")) {
        fail("malformed response: missing 'ok' field (HTTP " + convert.toString($resp.status) + ")");
    }
    checkResponse($node);
    return $node;
}

# callMultipart POSTs a `multipart/form-data` body (a file upload) and returns
# the decoded, ok-verified response node. Mirrors `call` but sends a built form
# instead of urlencoded params; the token is redacted from any rethrown
# transport error the same way.
func callMultipart(b as Bot, method as string, form as multipart.Built, timeoutMs as int) {
    def url as string init $b.baseUrl + "/bot" + $b.token + "/" + $method;
    def headers as map of string to string init {"Content-Type": $form.contentType};
    # Send the multipart body as raw bytes so a binary file (a photo, a document)
    # reaches the wire intact - a UTF-8 string round-trip would corrupt it.
    def resp as http.Response;
    try {
        $resp = http.requestRawBody("POST", $url, $headers, $form.body, $timeoutMs, 0);
    } catch (e) {
        fail(redactToken($e.message, $b.token));
    }
    def node as json.Value;
    try {
        $node = json.decode($resp.body);
    } catch (e) {
        fail("non-JSON response (HTTP " + convert.toString($resp.status) + ")");
    }
    if (not json.has($node, "/ok")) {
        fail("malformed response: missing 'ok' field (HTTP " + convert.toString($resp.status) + ")");
    }
    checkResponse($node);
    return $node;
}

# --- result parsing (private) -----------------------------------------------

# parseUser reads a User object at `base`.
func parseUser(node as json.Value, base as string) {
    def id as int init 0;
    if (json.has($node, $base + "/id")) {
        $id = json.asInt($node, $base + "/id");
    }
    def isBot as bool init false;
    if (json.has($node, $base + "/is_bot")) {
        $isBot = json.asBool($node, $base + "/is_bot");
    }
    def firstName as string init "";
    if (json.has($node, $base + "/first_name")) {
        $firstName = json.asString($node, $base + "/first_name");
    }
    def username as string init "";
    if (json.has($node, $base + "/username")) {
        $username = json.asString($node, $base + "/username");
    }
    return User{id: $id, isBot: $isBot, firstName: $firstName, username: $username};
}

# parseMessage reads a Message object at `base`.
func parseMessage(node as json.Value, base as string) {
    def msgId as int init 0;
    if (json.has($node, $base + "/message_id")) {
        $msgId = json.asInt($node, $base + "/message_id");
    }
    def chatId as int init 0;
    if (json.has($node, $base + "/chat/id")) {
        $chatId = json.asInt($node, $base + "/chat/id");
    }
    def text as string init "";
    if (json.has($node, $base + "/text")) {
        $text = json.asString($node, $base + "/text");
    }
    def date as int init 0;
    if (json.has($node, $base + "/date")) {
        $date = json.asInt($node, $base + "/date");
    }
    return Message{messageId: $msgId, chatId: $chatId, text: $text, date: $date};
}

# parseUpdates reads the `/result` array into a list of Update.
func parseUpdates(node as json.Value) {
    def updates as list of Update init [];
    def n as int init json.length($node, "/result");
    def i as int init 0;
    while ($i < $n) {
        def base as string init "/result/" + convert.toString($i);
        def updateId as int init json.asInt($node, $base + "/update_id");
        def hasMsg as bool init json.has($node, $base + "/message");
        def msg as Message;
        if ($hasMsg) {
            $msg = parseMessage($node, $base + "/message");
        }
        $updates[] = Update{updateId: $updateId, hasMessage: $hasMsg, message: $msg};
        $i = $i + 1;
    }
    return $updates;
}

/**
 * Parse an incoming update's `callback_query` into a `CallbackQuery`. The
 * `update` is the raw decoded JSON of one update object (a `json.Value`). Pure:
 * no network. Reads `/callback_query/{id,from,data,message/message_id}`; a field
 * that is absent lands as its zero value, and a field present but of the wrong
 * JSON type raises a `telegram` error (rather than a raw `json` one), so callers
 * catch malformed updates by the module's own error kind.
 * @param update {json.Value} the decoded update object
 * @return {CallbackQuery} the parsed callback query (zero-valued fields when absent)
 * @throws {Error} kind "telegram" when a present sub-field has the wrong type
 */
export func parseCallbackQuery(update as json.Value) {
    def base as string init "/callback_query";
    def id as string init "";
    def data as string init "";
    def from as User;
    def messageId as int init 0;
    # Wrap the field reads so a wrong-typed field surfaces as a telegram error,
    # matching how the request path wraps a json.decode failure.
    try {
        if (json.has($update, $base + "/id")) {
            $id = json.asString($update, $base + "/id");
        }
        if (json.has($update, $base + "/from")) {
            $from = parseUser($update, $base + "/from");
        }
        if (json.has($update, $base + "/data")) {
            $data = json.asString($update, $base + "/data");
        }
        if (json.has($update, $base + "/message/message_id")) {
            $messageId = json.asInt($update, $base + "/message/message_id");
        }
    } catch (e) {
        fail("parseCallbackQuery: malformed callback_query: " + $e.message);
    }
    return CallbackQuery{id: $id, from: $from, data: $data, messageId: $messageId};
}

# --- inline keyboards (exported) --------------------------------------------

/**
 * A URL button: pressing it opens `url` in the client.
 * @param text {string} the button label
 * @param url {string} the URL to open
 * @return {Button} the button
 */
export func urlButton(text as string, url as string) {
    return Button{text: $text, url: $url, callbackData: ""};
}

/**
 * A callback button: pressing it sends a `callback_query` carrying
 * `callbackData` back to the bot (observe it via `getUpdates` +
 * `parseCallbackQuery`, ack it with `answerCallbackQuery`).
 * @param text {string} the button label
 * @param callbackData {string} the callback payload (<= 64 bytes)
 * @return {Button} the button
 */
export func inlineButton(text as string, callbackData as string) {
    return Button{text: $text, url: "", callbackData: $callbackData};
}

/**
 * Render an inline-keyboard `reply_markup` to its JSON string. `rows` is a list
 * of rows, each row a list of `Button`s. Pure - the exact JSON `sendMessageWith`
 * variants attach as the `reply_markup` form parameter. A URL button emits
 * `{"text":...,"url":...}`; a callback button `{"text":...,"callback_data":...}`.
 * @param rows {list of list of Button} the keyboard rows
 * @return {string} the `reply_markup` JSON (a `{"inline_keyboard":[...]}` object)
 */
export func renderInlineKeyboard(rows as list of list of Button) {
    def markup as json.Value init json.map();
    $markup = json.set($markup, "/inline_keyboard", json.list());
    def r as int init 0;
    for (def row in $rows) {
        $markup = json.append($markup, "/inline_keyboard", json.list());
        def rowPtr as string init "/inline_keyboard/" + convert.toString($r);
        for (def btn in $row) {
            def b as json.Value init json.map();
            $b = json.set($b, "/text", $btn.text);
            if (len($btn.url) > 0) {
                $b = json.set($b, "/url", $btn.url);
            } else {
                $b = json.set($b, "/callback_data", $btn.callbackData);
            }
            $markup = json.append($markup, $rowPtr, $b);
        }
        $r = $r + 1;
    }
    return json.encode($markup);
}

# --- multipart upload builders (exported) -----------------------------------

/**
 * Assemble the `multipart/form-data` parts for a file upload: a `chat_id` field
 * plus the file itself under `field` (`"photo"` for `sendPhoto`, `"document"`
 * for `sendDocument`). Pure - takes the file bytes, so it is testable with no
 * disk or network. Feed the result's `contentType` / `body` to the API POST.
 * @param field {string} the file's form-field name ("photo" or "document")
 * @param chatId {int} the target chat id
 * @param filename {string} the file name reported to Telegram
 * @param contentType {string} the file's MIME type ("" lets Telegram sniff)
 * @param data {bytes} the file content
 * @return {multipart.Built} the built form (Content-Type header + body)
 */
export func buildUpload(
    field as string,
    chatId as int,
    filename as string,
    contentType as string,
    data as bytes) {
    def parts as list of multipart.Part init [
        multipart.field("chat_id", convert.toString($chatId)),
        multipart.file($field, $filename, $contentType, $data)
    ];
    return multipart.build($parts);
}

# --- API methods (exported) -------------------------------------------------

/**
 * Fetch the bot's own identity (a good connectivity / token check).
 * @param b {Bot} the bot
 * @return {User} the bot user
 * @throws {Error} kind "telegram" on an API error
 */
export func getMe(b as Bot) {
    def params as map of string to string init {};
    return parseUser(call($b, "getMe", $params, DEFAULT_TIMEOUT_MS), "/result");
}

/**
 * Send a text message to a chat.
 * @param b {Bot} the bot
 * @param chatId {int} the target chat id
 * @param text {string} the message text
 * @return {Message} the sent message
 * @throws {Error} kind "telegram" on an API error
 */
export func sendMessage(b as Bot, chatId as int, text as string) {
    def params as map of string to string init {};
    $params["chat_id"] = convert.toString($chatId);
    $params["text"] = $text;
    return parseMessage(call($b, "sendMessage", $params, DEFAULT_TIMEOUT_MS), "/result");
}

/**
 * Send a text message with a parse mode ("Markdown", "MarkdownV2", "HTML", or
 * "" for plain).
 * @param b {Bot} the bot
 * @param chatId {int} the target chat id
 * @param text {string} the message text
 * @param parseMode {string} the parse mode
 * @return {Message} the sent message
 * @throws {Error} kind "telegram" on an API error
 */
export func sendMessageWith(b as Bot, chatId as int, text as string, parseMode as string) {
    def params as map of string to string init {};
    $params["chat_id"] = convert.toString($chatId);
    $params["text"] = $text;
    if (len($parseMode) > 0) {
        $params["parse_mode"] = $parseMode;
    }
    return parseMessage(call($b, "sendMessage", $params, DEFAULT_TIMEOUT_MS), "/result");
}

/**
 * Send a photo by URL (or file id) with an optional caption.
 * @param b {Bot} the bot
 * @param chatId {int} the target chat id
 * @param photo {string} a photo URL or a Telegram file id
 * @param caption {string} the caption ("" for none)
 * @return {Message} the sent message
 * @throws {Error} kind "telegram" on an API error
 */
export func sendPhoto(b as Bot, chatId as int, photo as string, caption as string) {
    def params as map of string to string init {};
    $params["chat_id"] = convert.toString($chatId);
    $params["photo"] = $photo;
    if (len($caption) > 0) {
        $params["caption"] = $caption;
    }
    return parseMessage(call($b, "sendPhoto", $params, DEFAULT_TIMEOUT_MS), "/result");
}

/**
 * Send a chat action (e.g. "typing", "upload_photo") to show activity.
 * @param b {Bot} the bot
 * @param chatId {int} the target chat id
 * @param action {string} the action
 * @return {bool} true on success
 * @throws {Error} kind "telegram" on an API error
 */
export func sendChatAction(b as Bot, chatId as int, action as string) {
    def params as map of string to string init {};
    $params["chat_id"] = convert.toString($chatId);
    $params["action"] = $action;
    return json.asBool(call($b, "sendChatAction", $params, DEFAULT_TIMEOUT_MS), "/result");
}

/**
 * Send a text message with an inline keyboard attached. `rows` is a list of
 * button rows (see `urlButton` / `inlineButton`); it is rendered to the
 * `reply_markup` form parameter via `renderInlineKeyboard`.
 * @param b {Bot} the bot
 * @param chatId {int} the target chat id
 * @param text {string} the message text
 * @param rows {list of list of Button} the inline-keyboard rows
 * @return {Message} the sent message
 * @throws {Error} kind "telegram" on an API error
 */
export func sendMessageWithKeyboard(
    b as Bot,
    chatId as int,
    text as string,
    rows as list of list of Button) {
    def params as map of string to string init {};
    $params["chat_id"] = convert.toString($chatId);
    $params["text"] = $text;
    $params["reply_markup"] = renderInlineKeyboard($rows);
    return parseMessage(call($b, "sendMessage", $params, DEFAULT_TIMEOUT_MS), "/result");
}

/**
 * Acknowledge a button press (`answerCallbackQuery`). Stops the client's loading
 * spinner; the optional `text` shows a brief notification to the user.
 * @param b {Bot} the bot
 * @param callbackId {string} the `CallbackQuery.id` to answer
 * @param text {string} a short notification ("" for none)
 * @return {bool} true on success
 * @throws {Error} kind "telegram" on an API error
 */
export func answerCallbackQuery(b as Bot, callbackId as string, text as string) {
    def params as map of string to string init {};
    $params["callback_query_id"] = $callbackId;
    if (len($text) > 0) {
        $params["text"] = $text;
    }
    return json.asBool(call($b, "answerCallbackQuery", $params, DEFAULT_TIMEOUT_MS), "/result");
}

/**
 * Upload a photo from a local file path via `multipart/form-data`. Reads the
 * file with `fs.readBytes`, names the part from the path's basename, and POSTs
 * it as `sendPhoto`. The complement to `sendPhoto`, which takes a URL or file
 * id. `contentType` may be "" to let Telegram sniff the format.
 * @param b {Bot} the bot
 * @param chatId {int} the target chat id
 * @param filePath {string} the local file path
 * @param contentType {string} the file's MIME type ("" to let Telegram sniff)
 * @return {Message} the sent message
 * @throws {Error} kind "telegram" on an API error (token-redacted)
 */
export func sendPhotoFile(b as Bot, chatId as int, filePath as string, contentType as string) {
    def data as bytes init fs.readBytes($filePath);
    def form as multipart.Built init buildUpload(
        "photo",
        $chatId,
        path.base($filePath),
        $contentType,
        $data);
    return parseMessage(callMultipart($b, "sendPhoto", $form, DEFAULT_TIMEOUT_MS), "/result");
}

/**
 * Upload a document from a local file path via `multipart/form-data`. Like
 * `sendPhotoFile` but under the `document` field (any file type).
 * @param b {Bot} the bot
 * @param chatId {int} the target chat id
 * @param filePath {string} the local file path
 * @param contentType {string} the file's MIME type ("" to let Telegram sniff)
 * @return {Message} the sent message
 * @throws {Error} kind "telegram" on an API error (token-redacted)
 */
export func sendDocumentFile(b as Bot, chatId as int, filePath as string, contentType as string) {
    def data as bytes init fs.readBytes($filePath);
    def form as multipart.Built init buildUpload(
        "document",
        $chatId,
        path.base($filePath),
        $contentType,
        $data);
    return parseMessage(callMultipart($b, "sendDocument", $form, DEFAULT_TIMEOUT_MS), "/result");
}

/**
 * Long-poll for updates. Pass `offset` as the last processed `updateId + 1` (0
 * for the first call) and `timeout` as the long-poll wait in seconds; the HTTP
 * read is bounded a few seconds beyond that.
 * @param b {Bot} the bot
 * @param offset {int} the first update id to fetch (last processed + 1)
 * @param timeout {int} the long-poll timeout in seconds
 * @return {list of Update} the pending updates (empty when none arrived)
 * @throws {Error} kind "telegram" on an API error
 */
export func getUpdates(b as Bot, offset as int, timeout as int) {
    def params as map of string to string init {};
    $params["offset"] = convert.toString($offset);
    $params["timeout"] = convert.toString($timeout);
    return parseUpdates(call($b, "getUpdates", $params, ($timeout + 5) * 1000));
}
