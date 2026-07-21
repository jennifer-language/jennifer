# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

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
import "./http.j" as http;

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

func fail(msg as string) {
    throw Error{ kind: "telegram", message: "telegram: " + $msg, file: "", line: 0, col: 0 };
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
    return Bot{ token: $token, baseUrl: $baseUrl };
}

# --- form encoding (private) ------------------------------------------------

# isUnreserved reports whether a byte is an unreserved URL character.
func isUnreserved(b as int) {
    if ($b >= 65 and $b <= 90) {
        return true;
    }
    if ($b >= 97 and $b <= 122) {
        return true;
    }
    if ($b >= 48 and $b <= 57) {
        return true;
    }
    return $b == 45 or $b == 95 or $b == 46 or $b == 126;
}

# hexByte renders one byte as two uppercase hex digits.
func hexByte(b as int) {
    def digits as string init "0123456789ABCDEF";
    def hi as int init $b // 16;
    def lo as int init $b % 16;
    return strings.substring($digits, $hi, $hi + 1) + strings.substring($digits, $lo, $lo + 1);
}

# urlEncode percent-encodes a form value (unreserved stay, space -> +, else %XX).
func urlEncode(s as string) {
    def raw as bytes init convert.bytesFromString($s, "utf-8");
    def out as string init "";
    def i as int init 0;
    while ($i < len($raw)) {
        def b as int init $raw[$i];
        if (isUnreserved($b)) {
            $out = $out + convert.fromCodepoint($b);
        } elseif ($b == 32) {
            $out = $out + "+";
        } else {
            $out = $out + "%" + hexByte($b);
        }
        $i = $i + 1;
    }
    return $out;
}

# formEncode builds an application/x-www-form-urlencoded body from a string map
# (keys in insertion order).
func formEncode(params as map of string to string) {
    def out as string init "";
    def first as bool init true;
    for (def key in $params) {
        if (not $first) {
            $out = $out + "&";
        }
        $out = $out + urlEncode($key) + "=" + urlEncode($params[$key]);
        $first = false;
    }
    return $out;
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
    def headers as map of string to string init {"Content-Type": "application/x-www-form-urlencoded"};
    def resp as http.Response init http.requestWith("POST", $url, $headers, formEncode($params), $timeoutMs, 0);
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
    return User{ id: $id, isBot: $isBot, firstName: $firstName, username: $username };
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
    return Message{ messageId: $msgId, chatId: $chatId, text: $text, date: $date };
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
        $updates[] = Update{ updateId: $updateId, hasMessage: $hasMsg, message: $msg };
        $i = $i + 1;
    }
    return $updates;
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
