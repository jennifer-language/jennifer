# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

/**
 * A generic OAuth2 client: the get-a-token half of OAuth2 (the use-a-token half
 * is `sasl` XOAUTH2). Acquires and refreshes access tokens against any OAuth2
 * token endpoint, over `http` + `json`. Ships the flows that need no extra
 * dependencies: Client Credentials (a service authenticating as itself),
 * Refresh Token (trade a refresh token for a fresh access token), and the
 * Device Authorization Grant (the CLI-friendly flow: show the user a URL +
 * code, poll the token endpoint until they approve). Authorization Code + PKCE
 * and the service-account JWT assertion need a local redirect server and
 * crypto-grade signing, so they land later. Provider presets for Google and
 * Microsoft 365 fill in the endpoints. Because it builds on `http` (which uses
 * `net`), this module needs the default `jennifer` binary. A token-endpoint
 * error surfaces as a catchable `Error` (kind "oauth").
 * @module oauth
 * @example
 * def cfg as oauth.Config init oauth.google("id", "secret",
 *     "https://mail.google.com/");
 * def dev as oauth.DeviceAuth init oauth.deviceStart($cfg);
 * io.printf("visit %s and enter %s\n", $dev.verificationUri, $dev.userCode);
 * def tok as oauth.Token init oauth.deviceWait($cfg, $dev);
 */
use strings;
use convert;
use time;
use json;
use fs;
import "./http.j" as http;

/**
 * The OAuth2 client settings for one provider / application.
 * @field tokenUrl {string} the token endpoint URL
 * @field deviceUrl {string} the device-authorization endpoint URL
 * @field clientId {string} the OAuth2 client identifier
 * @field clientSecret {string} the OAuth2 client secret
 * @field scope {string} the space-separated requested scopes
 */
export def struct Config {
    tokenUrl as string,
    deviceUrl as string,
    clientId as string,
    clientSecret as string,
    scope as string
};

/**
 * An issued token. `expiresAt` is a Unix timestamp (seconds; 0 = no known
 * expiry).
 * @field accessToken {string} the bearer access token
 * @field tokenType {string} the token type (e.g. "Bearer")
 * @field refreshToken {string} the refresh token ("" if none)
 * @field scope {string} the scopes granted with this token
 * @field expiresAt {int} Unix expiry timestamp in seconds (0 = unknown)
 */
export def struct Token {
    accessToken as string,
    tokenType as string,
    refreshToken as string,
    scope as string,
    expiresAt as int
};

/**
 * A device-authorization handle: show the user `verificationUri` + `userCode`,
 * then `deviceWait` polls with `deviceCode` every `interval` seconds.
 * @field deviceCode {string} the code polled at the token endpoint
 * @field userCode {string} the code the user types at the verification URL
 * @field verificationUri {string} the URL to show the user
 * @field interval {int} seconds to wait between polls
 * @field expiresAt {int} Unix timestamp when the device code expires
 */
export def struct DeviceAuth {
    deviceCode as string,
    userCode as string,
    verificationUri as string,
    interval as int,
    expiresAt as int
};

# --- form encoding (private, pure) ---------------------------------

func hexByte(b as int) {
    def digits as string init "0123456789ABCDEF";
    return strings.substring($digits, $b // 16, $b // 16 + 1) +
        strings.substring($digits, $b % 16, $b % 16 + 1);
}

# urlEncode percent-encodes a value for an `application/x-www-form-urlencoded`
# body (unreserved bytes literal, space to `+`, else `%XX` over UTF-8).
func urlEncode(s as string) {
    def raw as bytes init convert.bytesFromString($s, "utf-8");
    def out as string init "";
    def i as int init 0;
    while ($i < len($raw)) {
        def b as int init $raw[$i];
        def keep as bool init ($b >= 65 and $b <= 90) or ($b >= 97 and $b <= 122);
        $keep = $keep or ($b >= 48 and $b <= 57);
        $keep = $keep or $b == 45 or $b == 95 or $b == 46 or $b == 126;
        if ($keep) {
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

# formBody renders a parameter map as a form-encoded request body.
func formBody(params as map of string to string) {
    def out as string init "";
    def first as bool init true;
    for (def k in $params) {
        if (not $first) {
            $out = $out + "&";
        }
        $out = $out + urlEncode($k) + "=" + urlEncode($params[$k]);
        $first = false;
    }
    return $out;
}

# --- token parsing (private, pure) ---------------------------------

func fail(message as string) {
    throw Error{kind: "oauth", message: $message, file: "", line: 0, col: 0};
}

# errorMessage renders an OAuth2 error response (`error` + `error_description`).
func errorMessage(node as json.Value) {
    def msg as string init json.asString($node, "/error");
    if (json.has($node, "/error_description")) {
        return $msg + ": " + json.asString($node, "/error_description");
    }
    return $msg;
}

# tokenFromNode builds a Token from a decoded success response; `nowUnix` fixes
# the clock so the expiry computation is testable.
func tokenFromNode(node as json.Value, nowUnix as int) {
    def expiresAt as int init 0;
    if (json.has($node, "/expires_in")) {
        def secs as int init json.asInt($node, "/expires_in");
        if ($secs > 0) {
            $expiresAt = $nowUnix + $secs;
        }
    }
    def tokenType as string init "Bearer";
    if (json.has($node, "/token_type")) {
        $tokenType = json.asString($node, "/token_type");
    }
    def refreshToken as string init "";
    if (json.has($node, "/refresh_token")) {
        $refreshToken = json.asString($node, "/refresh_token");
    }
    def scope as string init "";
    if (json.has($node, "/scope")) {
        $scope = json.asString($node, "/scope");
    }
    return Token{accessToken: json.asString($node, "/access_token"),
        tokenType: $tokenType, refreshToken: $refreshToken, scope: $scope,
        expiresAt: $expiresAt};
}

# decodeToken decodes a token-endpoint body, mapping a non-JSON response (a 502
# HTML page during polling) to an oauth-kind error rather than a raw json one.
func decodeToken(body as string) {
    def node as json.Value;
    try {
        $node = json.decode($body);
    } catch (e) {
        fail("non-JSON response from the token endpoint");
    }
    return $node;
}

# parseTokenBody parses a token-endpoint response body, throwing on an OAuth2
# error field.
func parseTokenBody(body as string, nowUnix as int) {
    def node as json.Value init decodeToken($body);
    if (json.has($node, "/error")) {
        fail(errorMessage($node));
    }
    return tokenFromNode($node, $nowUnix);
}

# pollState classifies a device-poll response: "success" or the error code
# ("authorization_pending" / "slow_down" / a terminal error). A non-JSON body
# during polling is treated as retryable ("slow_down") rather than fatal.
func pollState(body as string) {
    def node as json.Value;
    try {
        $node = json.decode($body);
    } catch (e) {
        return "slow_down";
    }
    if (json.has($node, "/error")) {
        return json.asString($node, "/error");
    }
    return "success";
}

# tokenExpired reports whether an `expiresAt` timestamp is past (with a 30s
# skew buffer); a 0 timestamp means the expiry is unknown, so not expired.
func tokenExpired(expiresAt as int, nowUnix as int) {
    if ($expiresAt == 0) {
        return false;
    }
    return $nowUnix + 30 >= $expiresAt;
}

# --- net (private) -------------------------------------------------

func postForm(url as string, params as map of string to string) {
    return http.post($url, "application/x-www-form-urlencoded", formBody($params), {});
}

func nowUnix() {
    return time.unix(time.now());
}

# --- grants (exported) ---------------------------------------------

/**
 * Acquire a token for the client itself (no user).
 * @param config {Config} the client settings
 * @return {Token} the issued access token
 * @throws {Error} kind "oauth" on a token-endpoint error
 */
export func clientCredentials(config as Config) {
    def params as map of string to string init {"grant_type": "client_credentials",
        "client_id": $config.clientId, "client_secret": $config.clientSecret,
        "scope": $config.scope};
    return parseTokenBody(postForm($config.tokenUrl, $params).body, nowUnix());
}

/**
 * Trade a refresh token for a new access token, preserving the refresh token
 * when the server omits it from the reply.
 * @param config {Config} the client settings
 * @param refreshToken {string} the refresh token to redeem
 * @return {Token} the new access token
 * @throws {Error} kind "oauth" on a token-endpoint error
 */
export func refresh(config as Config, refreshToken as string) {
    def params as map of string to string init {"grant_type": "refresh_token",
        "refresh_token": $refreshToken, "client_id": $config.clientId,
        "client_secret": $config.clientSecret};
    def t as Token init parseTokenBody(postForm($config.tokenUrl, $params).body, nowUnix());
    if (len($t.refreshToken) == 0) {
        $t.refreshToken = $refreshToken;
    }
    return $t;
}

/**
 * Begin the device flow: return the code + URL to show the user.
 * @param config {Config} the client settings
 * @return {DeviceAuth} the device-authorization handle
 * @throws {Error} kind "oauth" on a device-endpoint error
 */
export func deviceStart(config as Config) {
    def params as map of string to string init {"client_id": $config.clientId,
        "scope": $config.scope};
    def node as json.Value init json.decode(postForm($config.deviceUrl, $params).body);
    if (json.has($node, "/error")) {
        fail(errorMessage($node));
    }
    def vuri as string init "";
    if (json.has($node, "/verification_uri")) {
        $vuri = json.asString($node, "/verification_uri");
    } elseif (json.has($node, "/verification_url")) {
        $vuri = json.asString($node, "/verification_url");
    }
    def interval as int init 5;
    if (json.has($node, "/interval")) {
        $interval = json.asInt($node, "/interval");
    }
    def expiresAt as int init nowUnix() + 900;
    if (json.has($node, "/expires_in")) {
        $expiresAt = nowUnix() + json.asInt($node, "/expires_in");
    }
    return DeviceAuth{deviceCode: json.asString($node, "/device_code"),
        userCode: json.asString($node, "/user_code"), verificationUri: $vuri,
        interval: $interval, expiresAt: $expiresAt};
}

/**
 * Poll the token endpoint until the user approves (or a terminal error),
 * returning the token. Sleeps `interval` seconds between polls, backing off on
 * `slow_down`.
 * @param config {Config} the client settings
 * @param deviceAuth {DeviceAuth} the handle from `deviceStart`
 * @return {Token} the issued access token
 * @throws {Error} kind "oauth" if device authorization fails
 */
export func deviceWait(config as Config, deviceAuth as DeviceAuth) {
    def interval as int init $deviceAuth.interval;
    while (true) {
        # Stop polling once the device code has expired instead of looping
        # forever against a lenient endpoint that keeps returning pending.
        if ($deviceAuth.expiresAt > 0 and nowUnix() >= $deviceAuth.expiresAt) {
            fail("device authorization expired before approval");
        }
        def params as map of string to string init {
            "grant_type": "urn:ietf:params:oauth:grant-type:device_code",
            "device_code": $deviceAuth.deviceCode, "client_id": $config.clientId};
        # Google's token endpoint requires the client secret in the device
        # polling request; include it when configured (public clients leave it
        # empty).
        if (not ($config.clientSecret == "")) {
            $params["client_secret"] = $config.clientSecret;
        }
        def body as string init postForm($config.tokenUrl, $params).body;
        def state as string init pollState($body);
        if ($state == "success") {
            return parseTokenBody($body, nowUnix());
        }
        if ($state == "slow_down") {
            $interval = $interval + 5;
        } elseif (not ($state == "authorization_pending")) {
            fail("device authorization failed: " + $state);
        }
        time.sleep(time.fromSeconds($interval));
    }
    return Token{accessToken: "", tokenType: "", refreshToken: "", scope: "", expiresAt: 0};
}

/**
 * Report whether a token has expired (30s skew buffer).
 * @param token {Token} the token to check
 * @return {bool} true if the token is at or past its expiry
 */
export func isExpired(token as Token) {
    return tokenExpired($token.expiresAt, nowUnix());
}

# --- provider presets (exported) -----------------------------------

/**
 * Return a Config for Google's OAuth2 endpoints.
 * @param clientId {string} the OAuth2 client identifier
 * @param clientSecret {string} the OAuth2 client secret
 * @param scope {string} the space-separated requested scopes
 * @return {Config} a Config wired to Google's endpoints
 */
export func google(clientId as string, clientSecret as string, scope as string) {
    return Config{tokenUrl: "https://oauth2.googleapis.com/token",
        deviceUrl: "https://oauth2.googleapis.com/device/code",
        clientId: $clientId, clientSecret: $clientSecret, scope: $scope};
}

/**
 * Return a Config for a Microsoft 365 / Entra tenant's endpoints.
 * @param tenant {string} the tenant identifier (or "common")
 * @param clientId {string} the OAuth2 client identifier
 * @param clientSecret {string} the OAuth2 client secret
 * @param scope {string} the space-separated requested scopes
 * @return {Config} a Config wired to the tenant's endpoints
 */
export func microsoft(tenant as string, clientId as string, clientSecret as string,
    scope as string) {
    def base as string init "https://login.microsoftonline.com/" + $tenant + "/oauth2/v2.0";
    return Config{tokenUrl: $base + "/token", deviceUrl: $base + "/devicecode",
        clientId: $clientId, clientSecret: $clientSecret, scope: $scope};
}

# --- token store (exported) ----------------------------------------

/**
 * Write a token to a file as JSON (its own field shape, round-trips with load;
 * absolute `expiresAt` is preserved). The file is tightened to owner-only
 * (`0600`) after writing, since an OAuth token is a bearer credential - the same
 * file-permission model `gh` / `aws` use for their token stores, so it is not
 * left world-readable at the write verbs' default `0644`.
 * @param path {string} the file path to write
 * @param token {Token} the token to persist
 */
export func save(path as string, token as Token) {
    fs.writeString($path, json.encode($token));
    fs.chmod($path, 0o600);
}

/**
 * Read a token previously written by save.
 * @param path {string} the file path to read
 * @return {Token} the loaded token
 */
export func load(path as string) {
    def node as json.Value init json.decode(fs.readString($path));
    return Token{accessToken: json.asString($node, "/accessToken"),
        tokenType: json.asString($node, "/tokenType"),
        refreshToken: json.asString($node, "/refreshToken"),
        scope: json.asString($node, "/scope"),
        expiresAt: json.asInt($node, "/expiresAt")};
}
