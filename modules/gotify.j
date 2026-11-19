# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.25.0

/**
 * Push notifications to a Gotify server (https://gotify.net), on top of the
 * `http` client. Hold a value-semantic Config (server URL + application token)
 * and call push; it POSTs the message form to URL/message with the
 * X-Gotify-Key header, per Gotify's push-message contract. Needs the default
 * `jennifer` binary (uses `net` via `http`). The URL and token belong to the
 * caller - read them from the environment or a config file; never commit them.
 * A server URL other than `https://` (or `http://` to a loopback host) throws
 * `Error{kind: "gotify"}` before the token is sent, unless the Config sets
 * `allowInsecureHttp` (plain HTTP on a trusted LAN only).
 *
 * Deprecated: gotify moves out of the bundled modules into a deck, the
 * first-party sample deck https://github.com/jennifer-language/deck-gotify.
 * The bundled copy ships through the 0.27 releases and is removed from 0.28.0
 * (from the development line right after 0.27.0). Switch to the deck: declare
 * it in deck.toml, `jvc install`, and import it as
 * `import "@jennifer/gotify/" as gotify;` - the surface is the same.
 * @module gotify
 * @deprecated moved to the @jennifer/gotify deck; the bundled copy is removed in 0.28.0
 * @see https://github.com/jennifer-language/deck-gotify
 * @example
 * def g as gotify.Config init gotify.Config{url: "https://push.example.com", token: "tok"};
 * def r as http.Response init gotify.push($g, "Deploy", "build 1234 is live", 5);
 */
use json;
use regex;
use strings;
import "./http.j" as http;
import "./uri.j" as uri;

/**
 * A Gotify target: value-semantic, passed to each push (no module state).
 * @field url {string} the server URL, no trailing slash
 * @field token {string} the application token
 * @field allowInsecureHttp {bool} allow plain `http://` to a non-loopback host (default false)
 */
export def struct Config {
    url as string,
    token as string,
    allowInsecureHttp as bool
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

# formBody builds the Gotify message form (title / message / priority),
# form-encoding each value through the shared `uri` module.
func formBody(title as string, message as string, priority as int) {
    def body as string init "title=" + uri.encodeForm($title);
    $body = $body + "&message=" + uri.encodeForm($message);
    return $body + "&priority={$priority}";
}

# --- transport security --------------------------------------------

# isLoopback reports whether an http:// URL (scheme already stripped) names a
# loopback host: 127.0.0.0/8, [::1] or localhost. The host is cut out the way
# `http` does it: the authority ends at the first "/", "?" or "#", and the
# userinfo ends at the last "@", so http://127.0.0.1@evil.example is not one.
func isLoopback(rest as string) {
    def authority as string init $rest;
    for (def stop in ["/", "?", "#"]) {
        def i as int init strings.indexOf($authority, $stop);
        if ($i >= 0) {
            $authority = $authority[..$i];
        }
    }
    def at as int init strings.lastIndexOf($authority, "@");
    if ($at >= 0) {
        $authority = $authority[$at + 1..];
    }
    def host as string init $authority;
    if (strings.startsWith($host, "[")) {
        def rb as int init strings.indexOf($host, "]");
        if ($rb >= 0) {
            $host = $host[..$rb + 1];
        }
    } else {
        def colon as int init strings.indexOf($host, ":");
        if ($colon >= 0) {
            $host = $host[..$colon];
        }
    }
    $host = strings.lower($host);
    if ($host == "localhost" or $host == "[::1]") {
        return true;
    }
    return regex.matches('^127(\.(25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])){3}$', $host);
}

# checkUrl refuses a server URL that would send the X-Gotify-Key token in the
# clear: anything but https://, except http:// to a loopback host or with
# Config.allowInsecureHttp (a plain-HTTP homelab server). Kind "gotify",
# raised before any connection; the message leaves the URL out.
func checkUrl(cfg as Config) {
    def lower as string init strings.lower($cfg.url);
    if (strings.startsWith($lower, "https://")) {
        return;
    }
    if (strings.startsWith($lower, "http://") and
        ($cfg.allowInsecureHttp or isLoopback($cfg.url[7..]))) {
        return;
    }
    throw Error{
        kind: "gotify",
        message: "gotify: the server URL needs https:// (http:// only with allowInsecureHttp)"
    };
}

# --- push (exported) -----------------------------------------------

/**
 * Send a notification to the Gotify server.
 * @param cfg {Config} the server URL + token
 * @param title {string} the notification title
 * @param message {string} the notification body
 * @param priority {int} the Gotify priority (higher is more urgent)
 * @return {http.Response} 2xx on success; a bad token surfaces as a 4xx value
 * @throws {Error} kind "gotify" when the URL is not https:// and plain http:// is not allowed
 */
export func push(cfg as Config, title as string, message as string, priority as int) {
    checkUrl($cfg);
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
 * @throws {Error} kind "gotify" when the URL is not https:// and plain http:// is not allowed
 */
export func pushExtras(
    cfg as Config,
    title as string,
    message as string,
    priority as int,
    extras as Extras) {
    checkUrl($cfg);
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
