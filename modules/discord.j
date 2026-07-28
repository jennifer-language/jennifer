# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Post messages to a Discord channel through a channel Webhook, on top of the
 * `http` client - a sibling of `gotify` / `slack`. `send(webhookUrl, content)`
 * posts a plain message; the `message` builder assembles a richer payload from
 * embeds (`embed`, plus `embedField` / `embedFooter` / `embedAuthor`) and
 * optional webhook identity overrides (`username` / `avatar`), and
 * `sendMessage` posts it. Needs the default `jennifer` binary (uses `net` via
 * `http`). The webhook URL is a secret belonging to the caller - read it from
 * the environment or a config file; never commit it.
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
 * One embed under construction: the title / description / colour card plus the
 * optional field list, footer, and author. The `fields` entries and the
 * `footer` / `author` strings are pre-rendered JSON fragments ("" to omit).
 * @field title {string} the embed title
 * @field description {string} the embed body (Discord markdown)
 * @field color {int} the left-bar colour as a decimal RGB integer
 * @field fields {list of string} pre-rendered field JSON fragments
 * @field footer {string} "" or a pre-rendered footer JSON object
 * @field author {string} "" or a pre-rendered author JSON object
 */
export def struct Embed {
    title as string,
    description as string,
    color as int,
    fields as list of string,
    footer as string,
    author as string
};

/**
 * A rich message under construction: a top-level `content` string, a list of
 * embeds, and optional webhook-identity overrides (`username` / `avatar`, each
 * "" to omit).
 * @field content {string} the message content ("" to omit; embeds must then be present)
 * @field embeds {list of Embed} the message embeds
 * @field username {string} "" or a per-message username override
 * @field avatar {string} "" or a per-message avatar image URL override
 */
export def struct Message {
    content as string,
    embeds as list of Embed,
    username as string,
    avatar as string
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
 * Start an empty rich message (no content, no embeds, default identity).
 * @return {Message} a fresh message
 */
export func message() {
    def embeds as list of Embed init [];
    return Message{content: "", embeds: $embeds, username: "", avatar: ""};
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
 * Set a per-message username override (the name Discord shows instead of the
 * webhook's configured name). Returns a fresh message.
 * @param m {Message} the message
 * @param name {string} the display name to show
 * @return {Message} a message with the username override set
 */
export func username(m as Message, name as string) {
    def out as Message init $m;
    $out.username = $name;
    return $out;
}

/**
 * Set a per-message avatar override (the image Discord shows instead of the
 * webhook's configured avatar). Returns a fresh message.
 * @param m {Message} the message
 * @param url {string} the avatar image URL
 * @return {Message} a message with the avatar override set
 */
export func avatar(m as Message, url as string) {
    def out as Message init $m;
    $out.avatar = $url;
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
    def fields as list of string init [];
    def e as Embed init Embed{
        title: $title,
        description: $description,
        color: $color,
        fields: $fields,
        footer: "",
        author: ""
    };
    $out.embeds = lists.push($out.embeds, $e);
    return $out;
}

# lastEmbedIndex returns the index of the most recent embed, or throws if there
# is none to modify.
func lastEmbedIndex(m as Message) {
    if (len($m.embeds) == 0) {
        throw Error{
            kind: "discord",
            message: "discord: no embed to modify; add one with embed(...) first",
            file: "",
            line: 0,
            col: 0
        };
    }
    return len($m.embeds) - 1;
}

/**
 * Add a field (a name / value pair, optionally inline) to the most recent
 * embed. Throws if the message has no embed yet. Returns a fresh message.
 * @param m {Message} the message
 * @param name {string} the field name (bold heading)
 * @param value {string} the field value (Discord markdown)
 * @param inline {bool} true to let Discord pack this field beside others
 * @return {Message} a message whose latest embed gained the field
 */
export func embedField(m as Message, name as string, value as string, inline as bool) {
    def out as Message init $m;
    def idx as int init lastEmbedIndex($out);
    def e as Embed init $out.embeds[$idx];
    def f as string init "{\"name\":" + json.encode($name) +
        ",\"value\":" + json.encode($value) +
        ",\"inline\":" + convert.toString($inline) + "}";
    $e.fields = lists.push($e.fields, $f);
    $out.embeds[$idx] = $e;
    return $out;
}

/**
 * Set the footer text on the most recent embed. Throws if the message has no
 * embed yet. Returns a fresh message.
 * @param m {Message} the message
 * @param text {string} the footer text
 * @return {Message} a message whose latest embed gained the footer
 */
export func embedFooter(m as Message, text as string) {
    def out as Message init $m;
    def idx as int init lastEmbedIndex($out);
    def e as Embed init $out.embeds[$idx];
    $e.footer = "{\"text\":" + json.encode($text) + "}";
    $out.embeds[$idx] = $e;
    return $out;
}

/**
 * Set the author name on the most recent embed. Throws if the message has no
 * embed yet. Returns a fresh message.
 * @param m {Message} the message
 * @param name {string} the author name
 * @return {Message} a message whose latest embed gained the author
 */
export func embedAuthor(m as Message, name as string) {
    def out as Message init $m;
    def idx as int init lastEmbedIndex($out);
    def e as Embed init $out.embeds[$idx];
    $e.author = "{\"name\":" + json.encode($name) + "}";
    $out.embeds[$idx] = $e;
    return $out;
}

# renderEmbed renders one embed to its JSON object string.
func renderEmbed(e as Embed) {
    def parts as list of string init [];
    $parts[] = "\"title\":" + json.encode($e.title);
    $parts[] = "\"description\":" + json.encode($e.description);
    $parts[] = "\"color\":" + convert.toString($e.color);
    if (len($e.fields) > 0) {
        $parts[] = "\"fields\":[" + strings.join($e.fields, ",") + "]";
    }
    if (len($e.footer) > 0) {
        $parts[] = "\"footer\":" + $e.footer;
    }
    if (len($e.author) > 0) {
        $parts[] = "\"author\":" + $e.author;
    }
    return "{" + strings.join($parts, ",") + "}";
}

/**
 * Render a message to its JSON payload string.
 * @param m {Message} the message
 * @return {string} the JSON payload Discord expects
 */
export func render(m as Message) {
    def parts as list of string init [];
    if (len($m.username) > 0) {
        $parts[] = "\"username\":" + json.encode($m.username);
    }
    if (len($m.avatar) > 0) {
        $parts[] = "\"avatar_url\":" + json.encode($m.avatar);
    }
    if (len($m.content) > 0) {
        $parts[] = "\"content\":" + json.encode($m.content);
    }
    if (len($m.embeds) > 0) {
        def rendered as list of string init [];
        for (def e in $m.embeds) {
            $rendered[] = renderEmbed($e);
        }
        $parts[] = "\"embeds\":[" + strings.join($rendered, ",") + "]";
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
