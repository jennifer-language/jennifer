# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Push notifications to a Gotify server (https://gotify.net), on top of the
 * `http` client. Hold a value-semantic Config (server URL + application token)
 * and call push; it POSTs the message form to URL/message with the
 * X-Gotify-Key header, per Gotify's push-message contract. Needs the default
 * `jennifer` binary (uses `net` via `http`). The URL and token belong to the
 * caller - read them from the environment or a config file; never commit them.
 * @module gotify
 * @example
 * def g as gotify.Config init gotify.Config{url: "https://push.example.com", token: "tok"};
 * def r as http.Response init gotify.push($g, "Deploy", "build 1234 is live", 5);
 */
use strings;
use convert;
use json;
import "./http.j" as http;

/**
 * A Gotify target: value-semantic, passed to each push (no module state).
 * @field url {string} the server URL, no trailing slash
 * @field token {string} the application token
 */
export def struct Config {
    url as string,
    token as string
};

/**
 * Optional Gotify message extras (value-semantic, passed to `pushExtras`).
 * A zero `Extras` (markdown false, empty clickUrl) adds nothing, so a push
 * with it matches the plain `push`. Only the requested keys reach the wire.
 * @field markdown {bool} render the message body as markdown (sets the `client::display` content-type to `text/markdown`)
 * @field clickUrl {string} URL to open when the notification is tapped (sets `client::notification.click.url`); "" for none
 */
export def struct Extras {
    markdown as bool,
    clickUrl as string
};

# --- form encoding (private) ---------------------------------------

# isUnreserved reports whether a byte is an unreserved URL character
# (A-Z / a-z / 0-9 / - / _ / . / ~), which is left literal in a form value.
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

# urlEncode percent-encodes a string for an `application/x-www-form-urlencoded`
# value: unreserved bytes stay, a space becomes `+`, and every other byte
# becomes `%XX` (over the value's UTF-8 bytes).
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

# formBody builds the Gotify message form (title / message / priority).
func formBody(title as string, message as string, priority as int) {
    def body as string init "title=" + urlEncode($title);
    $body = $body + "&message=" + urlEncode($message);
    return $body + "&priority=" + convert.toString($priority);
}

# --- push (exported) -----------------------------------------------

/**
 * Send a notification to the Gotify server.
 * @param cfg {Config} the server URL + token
 * @param title {string} the notification title
 * @param message {string} the notification body
 * @param priority {int} the Gotify priority (higher is more urgent)
 * @return {http.Response} 2xx on success; a bad token surfaces as a 4xx value
 */
export func push(cfg as Config, title as string, message as string, priority as int) {
    def headers as map of string to string init {"X-Gotify-Key": $cfg.token};
    def body as string init formBody($title, $message, $priority);
    return http.post($cfg.url + "/message", "application/x-www-form-urlencoded", $body, $headers);
}

# --- JSON payload with extras (private builder) --------------------

# hasExtras reports whether an Extras carries any extra worth serializing.
func hasExtras(extras as Extras) {
    return $extras.markdown or len($extras.clickUrl) > 0;
}

# buildExtras builds the Gotify `extras` object for the requested options: the
# markdown content-type under `client::display`, and a click-to-open URL under
# `client::notification`. Each nested map is created before its child (json's
# strict, no-vivify write surface), and only the requested keys are present.
func buildExtras(extras as Extras) {
    def x as json.Value init json.map();
    if ($extras.markdown) {
        $x = json.set($x, "/client::display", json.map());
        $x = json.set($x, "/client::display/contentType", "text/markdown");
    }
    if (len($extras.clickUrl) > 0) {
        $x = json.set($x, "/client::notification", json.map());
        $x = json.set($x, "/client::notification/click", json.map());
        $x = json.set($x, "/client::notification/click/url", $extras.clickUrl);
    }
    return $x;
}

# jsonBody builds the Gotify JSON message body - title / message / priority,
# plus an `extras` object when any extra is set. The pure counterpart to
# `formBody`, used by the extras-carrying push variants (and unit-tested
# without a network call).
func jsonBody(title as string, message as string, priority as int, extras as Extras) {
    def payload as json.Value init json.map();
    $payload = json.set($payload, "/title", $title);
    $payload = json.set($payload, "/message", $message);
    $payload = json.set($payload, "/priority", $priority);
    if (hasExtras($extras)) {
        $payload = json.set($payload, "/extras", buildExtras($extras));
    }
    return json.encode($payload);
}

# --- push with extras (exported) -----------------------------------

/**
 * Send a notification carrying `extras` (markdown rendering and/or a click
 * URL). POSTs a JSON body (not the form) so the nested `extras` object reaches
 * Gotify; the plain `push` stays form-encoded and unchanged.
 * @param cfg {Config} the server URL + token
 * @param title {string} the notification title
 * @param message {string} the notification body
 * @param priority {int} the Gotify priority (higher is more urgent)
 * @param extras {Extras} the extras to attach (a zero Extras adds none)
 * @return {http.Response} 2xx on success; a bad token surfaces as a 4xx value
 */
export func pushExtras(
    cfg as Config,
    title as string,
    message as string,
    priority as int,
    extras as Extras) {
    def headers as map of string to string init {"X-Gotify-Key": $cfg.token};
    def body as string init jsonBody($title, $message, $priority, $extras);
    return http.post($cfg.url + "/message", "application/json", $body, $headers);
}

/**
 * Send a notification whose body renders as **markdown** (sets the
 * `client::display` content-type to `text/markdown`). Convenience over
 * `pushExtras` with `Extras{markdown: true}`.
 * @param cfg {Config} the server URL + token
 * @param title {string} the notification title
 * @param message {string} the markdown notification body
 * @param priority {int} the Gotify priority (higher is more urgent)
 * @return {http.Response} 2xx on success; a bad token surfaces as a 4xx value
 */
export func pushMarkdown(cfg as Config, title as string, message as string, priority as int) {
    return pushExtras($cfg, $title, $message, $priority, Extras{markdown: true, clickUrl: ""});
}

/**
 * Send a notification that opens `clickUrl` when tapped (sets
 * `client::notification.click.url`). Convenience over `pushExtras` with
 * `Extras{clickUrl: ...}`.
 * @param cfg {Config} the server URL + token
 * @param title {string} the notification title
 * @param message {string} the notification body
 * @param priority {int} the Gotify priority (higher is more urgent)
 * @param clickUrl {string} the URL to open when the notification is tapped
 * @return {http.Response} 2xx on success; a bad token surfaces as a 4xx value
 */
export func pushWith(
    cfg as Config,
    title as string,
    message as string,
    priority as int,
    clickUrl as string) {
    return pushExtras(
        $cfg,
        $title,
        $message,
        $priority,
        Extras{markdown: false, clickUrl: $clickUrl});
}
