# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Post messages to a Discord channel through a channel Webhook, on top of the
 * `http` client - a sibling of `gotify` / `slack`. `send(webhookUrl, content)`
 * posts a plain message; the `message` builder assembles a richer payload from
 * embeds (`embed`) and `sendMessage` posts it. Needs the default `jennifer`
 * binary (uses `net` via `http`). The webhook URL is a secret belonging to the
 * caller - read it from the environment or a config file; never commit it.
 * @module discord
 * @example
 * import "discord.j" as discord;
 * discord.send("https://discord.com/api/webhooks/1/xxx", "deploy finished");
 * def m as discord.Message init discord.embed(
 *     discord.content(discord.message(), "heads up"), "Deploy", "build 1234 is live", 3066993);
 * discord.sendMessage("https://discord.com/api/webhooks/1/xxx", $m);
 */
use json;
use lists;
use strings;
use convert;
import "./http.j" as http;

/**
 * A rich message under construction: a top-level `content` string plus a list
 * of pre-rendered embed JSON fragments.
 * @field content {string} the message content ("" to omit; embeds must then be present)
 * @field embeds {list of string} pre-rendered embed JSON fragments
 */
export def struct Message {
    content as string,
    embeds as list of string
};

# post sends a JSON payload to a Discord webhook URL.
func post(webhookUrl as string, payload as string) {
    def headers as map of string to string init {};
    return http.post($webhookUrl, "application/json", $payload, $headers);
}

/**
 * Post a plain-text message to a Discord Webhook.
 * @param webhookUrl {string} the channel Webhook URL
 * @param content {string} the message content (Discord markdown)
 * @return {http.Response} Discord answers 204 No Content on success
 */
export func send(webhookUrl as string, content as string) {
    def payload as map of string to string init {"content": $content};
    return post($webhookUrl, json.encode($payload));
}

# --- rich-message builder (exported) ----------------------------------------

/**
 * Start an empty rich message (no content, no embeds).
 * @return {Message} a fresh message
 */
export func message() {
    def embeds as list of string init [];
    return Message{ content: "", embeds: $embeds };
}

/**
 * Set the top-level message content. Returns a fresh message.
 * @param m {Message} the message
 * @param content {string} the content text
 * @return {Message} a message with the content set
 */
export func content(m as Message, content as string) {
    def out as Message init $m;
    $out.content = $content;
    return $out;
}

/**
 * Add an embed (a titled, coloured card). `color` is a decimal RGB integer
 * (e.g. 3066993 for green). At least one of `title` / `description` should be
 * non-empty. Returns a fresh message.
 * @param m {Message} the message
 * @param title {string} the embed title
 * @param description {string} the embed body (Discord markdown)
 * @param color {int} the left-bar colour as a decimal RGB integer
 * @return {Message} a message with the embed appended
 */
export func embed(m as Message, title as string, description as string, color as int) {
    def out as Message init $m;
    def e as string init "{\"title\":" + json.encode($title) +
        ",\"description\":" + json.encode($description) +
        ",\"color\":" + convert.toString($color) + "}";
    $out.embeds = lists.push($out.embeds, $e);
    return $out;
}

/**
 * Render a message to its JSON payload string.
 * @param m {Message} the message
 * @return {string} the JSON payload Discord expects
 */
export func render(m as Message) {
    def parts as list of string init [];
    if (len($m.content) > 0) {
        $parts[] = "\"content\":" + json.encode($m.content);
    }
    if (len($m.embeds) > 0) {
        $parts[] = "\"embeds\":[" + strings.join($m.embeds, ",") + "]";
    }
    return "{" + strings.join($parts, ",") + "}";
}

/**
 * Post a built rich message to a Discord Webhook.
 * @param webhookUrl {string} the channel Webhook URL
 * @param m {Message} the message to send
 * @return {http.Response} Discord answers 204 No Content on success
 */
export func sendMessage(webhookUrl as string, m as Message) {
    return post($webhookUrl, render($m));
}
