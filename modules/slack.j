# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

/**
 * Post messages to a Slack channel through an Incoming Webhook, on top of the
 * `http` client - a sibling of `gotify` / `discord`. `send(webhookUrl, text)`
 * posts a plain message; the `message` builder assembles a richer payload from
 * Slack Block Kit blocks (`section` / `header` / `divider`) and `sendMessage`
 * posts it. Needs the default `jennifer` binary (uses `net` via `http`). The
 * webhook URL is a secret belonging to the caller - read it from the
 * environment or a config file; never commit it.
 * @module slack
 * @example
 * import "slack.j" as slack;
 * slack.send("https://hooks.slack.com/services/T/B/xxx", "deploy finished");
 * def m as slack.Message init slack.section(
 *     slack.header(slack.message(), "Deploy"), "*build 1234* is live");
 * slack.sendMessage("https://hooks.slack.com/services/T/B/xxx", $m);
 */
use json;
use lists;
use strings;
import "./http.j" as http;

/**
 * A rich message under construction: a fallback text (shown in notifications)
 * plus a list of pre-rendered Block Kit block JSON fragments.
 * @field text {string} the top-level fallback / notification text ("" to omit)
 * @field blocks {list of string} pre-rendered block JSON fragments
 */
export def struct Message {
    text as string,
    blocks as list of string
};

# post sends a JSON payload to a Slack webhook URL.
func post(webhookUrl as string, payload as string) {
    def headers as map of string to string init {};
    return http.post($webhookUrl, "application/json", $payload, $headers);
}

/**
 * Post a plain-text message to a Slack Incoming Webhook.
 * @param webhookUrl {string} the Incoming Webhook URL
 * @param text {string} the message text (Slack mrkdwn)
 * @return {http.Response} Slack answers 200 with body "ok" on success
 */
export func send(webhookUrl as string, text as string) {
    def payload as map of string to string init {"text": $text};
    return post($webhookUrl, json.encode($payload));
}

# --- rich-message builder (exported) ----------------------------------------

/**
 * Start an empty rich message (no fallback text, no blocks).
 * @return {Message} a fresh message
 */
export func message() {
    def blocks as list of string init [];
    return Message{ text: "", blocks: $blocks };
}

/**
 * Set the top-level fallback text (shown in notifications and by clients that
 * do not render blocks). Returns a fresh message.
 * @param m {Message} the message
 * @param text {string} the fallback text
 * @return {Message} a message with the fallback text set
 */
export func text(m as Message, text as string) {
    def out as Message init $m;
    $out.text = $text;
    return $out;
}

/**
 * Add a section block (Slack mrkdwn). Returns a fresh message.
 * @param m {Message} the message
 * @param markdown {string} the section body (mrkdwn)
 * @return {Message} a message with the section appended
 */
export func section(m as Message, markdown as string) {
    def out as Message init $m;
    def block as string init "{\"type\":\"section\",\"text\":{\"type\":\"mrkdwn\",\"text\":" + json.encode($markdown) + "}}";
    $out.blocks = lists.push($out.blocks, $block);
    return $out;
}

/**
 * Add a header block (plain text, bold and large). Returns a fresh message.
 * @param m {Message} the message
 * @param heading {string} the header text (plain text)
 * @return {Message} a message with the header appended
 */
export func header(m as Message, heading as string) {
    def out as Message init $m;
    def block as string init "{\"type\":\"header\",\"text\":{\"type\":\"plain_text\",\"text\":" + json.encode($heading) + "}}";
    $out.blocks = lists.push($out.blocks, $block);
    return $out;
}

/**
 * Add a divider block (a horizontal rule). Returns a fresh message.
 * @param m {Message} the message
 * @return {Message} a message with the divider appended
 */
export func divider(m as Message) {
    def out as Message init $m;
    $out.blocks = lists.push($out.blocks, "{\"type\":\"divider\"}");
    return $out;
}

/**
 * Render a message to its JSON payload string.
 * @param m {Message} the message
 * @return {string} the JSON payload Slack expects
 */
export func render(m as Message) {
    def parts as list of string init [];
    if (len($m.text) > 0) {
        $parts[] = "\"text\":" + json.encode($m.text);
    }
    if (len($m.blocks) > 0) {
        $parts[] = "\"blocks\":[" + strings.join($m.blocks, ",") + "]";
    }
    return "{" + strings.join($parts, ",") + "}";
}

/**
 * Post a built rich message to a Slack Incoming Webhook.
 * @param webhookUrl {string} the Incoming Webhook URL
 * @param m {Message} the message to send
 * @return {http.Response} Slack answers 200 with body "ok" on success
 */
export func sendMessage(webhookUrl as string, m as Message) {
    return post($webhookUrl, render($m));
}
