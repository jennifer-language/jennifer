# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

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
