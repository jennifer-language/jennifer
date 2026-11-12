# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.26.0

/**
 * A generic OAuth2 client: the get-a-token half of OAuth2 (the use-a-token half
 * is `sasl` XOAUTH2). Acquires and refreshes access tokens against any OAuth2
 * token endpoint, over `http` + `json`. Ships five flows:
 * Client Credentials (a service authenticating as itself, with a secret or a
 * signed client assertion), Refresh Token (trade a refresh token for a fresh
 * access token), the Device Authorization Grant (the CLI-friendly flow: show
 * the user a URL + code, poll the token endpoint until they approve),
 * Authorization Code with PKCE (the browser sign-in, its redirect caught on a
 * loopback `httpd` server), and the JWT Bearer grant (RFC 7523: a signed
 * assertion, built by `jwtAssertion`, traded for a token - how service
 * accounts authenticate). `discover` reads a provider's endpoints from its
 * metadata document. The module is provider-neutral: presets with a provider's
 * endpoints, and Google service-account key files, are in the
 * `@jennifer/oauthext` deck. Because it builds on `http` (which uses
 * `net`), this module needs the default `jennifer` binary. A token-endpoint
 * error, and a reply that is malformed (a missing or mistyped field, an empty
 * token, a non-positive lifetime), surfaces as a catchable `Error` (kind
 * "oauth").
 * @module oauth
 * @example
 * def cfg as oauth.Config init oauth.discover("https://accounts.google.com",
 *     "id", "secret", "https://mail.google.com/");
 * def srv as httpd.Server init httpd.listen("127.0.0.1:0");
 * def req as oauth.AuthRequest init oauth.authorize($cfg,
 *     "http://" + httpd.address($srv) + "/callback");
 * io.printf("open %s\n", $req.url);
 * def tok as oauth.Token init oauth.exchangeCode($cfg, $req, oauth.awaitCode($req, $srv));
 */
use time;
use json;
use fs;
use convert;
use crypto;
use encoding;
use hash;
use httpd;
use lists;
use maps;
use regex;
use strings;
use task;
import "./http.j" as http;
import "./uri.j" as uri;
import "./jwt.j" as jwt;

# The RFC 7523 grant type, and the registered claims jwtAssertion sets itself
# (so `extra` may not carry them).
def const JWT_BEARER as string init "urn:ietf:params:oauth:grant-type:jwt-bearer";
def const ASSERTION_CLAIMS as list of string init ["iss", "sub", "aud", "iat", "exp"];
# A client assertion (RFC 7523 section 2.2): its type, and how long it lives.
def const CLIENT_ASSERTION_TYPE as string init "urn:ietf:params:oauth:client-assertion-type:jwt-bearer";
def const CLIENT_ASSERTION_LIFETIME as int init 300;
# The token-response fields Token has a field for; the rest go to `extra`.
def const TOKEN_FIELDS as list of string init [
    "access_token",
    "token_type",
    "refresh_token",
    "scope",
    "expires_in",
    "id_token"
];
# Control characters, kept out of error messages.
def const CONTROL as string init '[\x00-\x1f\x7f-\x9f]';
# Control characters and whitespace, which no URL holds.
def const SPACE_OR_CONTROL as string init '[\x00-\x20\x7f-\x9f]';
# The hosts an endpoint may be plain http on: a test server or an emulator on
# this machine.
def const LOOPBACK as list of string init ["127.0.0.1", "localhost", "[::1]"];
# The bounds on a reply from an endpoint: its body (a token reply is a few KiB;
# a bigger one is refused before it is decoded), how long one read may stall,
# and how long the whole request may take.
def const MAX_REPLY_BYTES as int init 1048576;
def const IDLE_TIMEOUT_MS as int init 15000;
def const REQUEST_TIMEOUT_S as int init 60;
# The largest lifetimes accepted from a server: a token's (ten years), a device
# code's (a day), and the device-poll interval (ten minutes).
def const MAX_TOKEN_LIFETIME as int init 315360000;
def const MAX_DEVICE_LIFETIME as int init 86400;
def const MAX_POLL_INTERVAL as int init 600;
# The longest user code shown to a user (RFC 8628 suggests about eight
# characters) and verification URI accepted.
def const MAX_USER_CODE as int init 64;
def const MAX_VERIFICATION_URI as int init 2048;

/**
 * The OAuth2 client settings for one provider / application.
 * @field tokenUrl {string} the token endpoint URL
 * @field deviceUrl {string} the device-authorization endpoint URL ("" if the provider has none)
 * @field authorizeUrl {string} the authorization endpoint URL, for `authorize` ("" if the provider has none)
 * @field clientId {string} the OAuth2 client identifier
 * @field clientSecret {string} the OAuth2 client secret
 * @field scope {string} the space-separated requested scopes
 */
export def struct Config {
    tokenUrl as string,
    deviceUrl as string,
    authorizeUrl as string,
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
 * @field idToken {string} the OpenID Connect ID token ("" if none)
 * @field extra {map of string to string} the response's other top-level string, number and boolean fields, as text (e.g. Salesforce's `instance_url`)
 */
export def struct Token {
    accessToken as string,
    tokenType as string,
    refreshToken as string,
    scope as string,
    expiresAt as int,
    idToken as string,
    extra as map of string to string
};

/**
 * One authorization-code sign-in in progress, from `authorize`: send the user
 * to `url`, then catch the redirect (`awaitCode` or `codeFromRedirect`) and
 * trade the code with `exchangeCode`.
 * @field url {string} the authorization URL to open in the user's browser
 * @field state {string} the random value the redirect must carry back
 * @field verifier {string} the PKCE code verifier, sent with the code
 * @field redirectUri {string} the redirect URI registered with the provider
 */
export def struct AuthRequest {
    url as string,
    state as string,
    verifier as string,
    redirectUri as string
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

# formBody renders a parameter map as a form-encoded request body
# (`application/x-www-form-urlencoded`, insertion order preserved) via the
# shared `uri` module.
func formBody(params as map of string to string) {
    return uri.buildQuery($params);
}

# --- token parsing (private, pure) ---------------------------------

func fail(message as string) {
    throw Error{kind: "oauth", message: $message};
}

# shown makes text from a server or a caller safe for an error message: at most
# 200 characters, control characters as `?`.
func shown(s as string) {
    def t as string init $s;
    if (len($t) > 200) {
        $t = strings.substring($t, 0, 200) + "...";
    }
    return regex.replace(CONTROL, $t, "?");
}

# errorMessage renders an OAuth2 error response (`error` + `error_description`,
# both strings).
func errorMessage(node as json.Value) {
    if (json.typeOf($node, "/error") != "string") {
        fail("malformed error response: error is not a string");
    }
    def msg as string init shown(json.asString($node, "/error"));
    if (json.has($node, "/error_description")) {
        if (json.typeOf($node, "/error_description") != "string") {
            fail("malformed error response: error_description is not a string");
        }
        return $msg + ": " + shown(json.asString($node, "/error_description"));
    }
    return $msg;
}

# textField reads a string field of a response: "" when absent and optional;
# missing when required, or not a string, is an error.
func textField(node as json.Value, what as string, name as string, required as bool) {
    if (not json.has($node, "/" + $name)) {
        if ($required) {
            fail("the " + $what + " has no " + $name);
        }
        return "";
    }
    if (json.typeOf($node, "/" + $name) != "string") {
        fail("the " + $what + "'s " + $name + " is not a string");
    }
    return json.asString($node, "/" + $name);
}

# positiveField reads an optional integer field (seconds): `absent` when the
# field is missing, an error when it is not a whole number from 1 to `most`.
func positiveField(node as json.Value, what as string, name as string, absent as int, most as int) {
    if (not json.has($node, "/" + $name)) {
        return $absent;
    }
    if (json.typeOf($node, "/" + $name) != "int" or json.asInt($node, "/" + $name) <= 0 or
        json.asInt($node, "/" + $name) > $most) {
        fail("the " + $what + "'s " + $name + " must be a whole number of seconds from 1 to " +
            convert.toString($most) + ", got " + shown(json.encode(json.get($node, "/" + $name))));
    }
    return json.asInt($node, "/" + $name);
}

# tokenFromNode builds a Token from a decoded success response; `nowUnix` fixes
# the clock so the expiry computation is testable. `access_token` and
# `token_type` are required and not empty (RFC 6749 section 5.1), every field
# must have its type, and `expires_in` must be from 1 s to ten years.
func tokenFromNode(node as json.Value, nowUnix as int) {
    if (json.typeOf($node) != "map") {
        fail("the token response is not a JSON object");
    }
    def accessToken as string init textField($node, "token response", "access_token", true);
    if ($accessToken == "") {
        fail("the token response's access_token is empty");
    }
    def tokenType as string init textField($node, "token response", "token_type", true);
    if ($tokenType == "") {
        fail("the token response's token_type is empty");
    }
    def expiresAt as int init 0;
    def secs as int init positiveField(
        $node,
        "token response",
        "expires_in",
        0,
        MAX_TOKEN_LIFETIME);
    if ($secs > 0) {
        $expiresAt = $nowUnix + $secs;
    }
    return Token{
        accessToken: $accessToken,
        tokenType: $tokenType,
        refreshToken: textField($node, "token response", "refresh_token", false),
        scope: textField($node, "token response", "scope", false),
        expiresAt: $expiresAt,
        idToken: textField($node, "token response", "id_token", false),
        extra: extraFields($node)
    };
}

# extraFields keeps a token response's other top-level fields: strings as they
# are, numbers and booleans as their JSON text. Nested objects, lists and nulls
# are left out.
func extraFields(node as json.Value) {
    def out as map of string to string init {};
    if (json.typeOf($node) != "map") {
        return $out;
    }
    for (def k in json.keys($node)) {
        if (lists.contains(TOKEN_FIELDS, $k)) {
            continue;
        }
        def p as string init pointer($k);
        match (json.typeOf($node, $p)) {
            when "string" { $out[$k] = json.asString($node, $p); }
            when "int", "float", "bool" { $out[$k] = json.encode(json.get($node, $p)); }
        }
    }
    return $out;
}

# pointer is the JSON pointer to a top-level key (`~` and `/` escaped).
func pointer(key as string) {
    return "/" + strings.replace(strings.replace($key, "~", "~0"), "/", "~1");
}

# decodeReply decodes an endpoint's JSON body, mapping a non-JSON response (a
# 502 HTML page) to an oauth-kind error, with the status, rather than a raw
# json one.
func decodeReply(body as string, status as int, endpoint as string) {
    def node as json.Value;
    try {
        $node = json.decode($body);
    } catch (e) {
        fail("non-JSON response from the " + $endpoint + " (HTTP " + convert.toString($status) + ")");
    }
    return $node;
}

# parseTokenBody parses a token-endpoint response, throwing on an OAuth2 error
# field, or on any other answer than 200.
func parseTokenBody(body as string, status as int, nowUnix as int) {
    def node as json.Value init decodeReply($body, $status, "token endpoint");
    if (json.typeOf($node) == "map" and json.has($node, "/error")) {
        fail(errorMessage($node));
    }
    if ($status != 200) {
        fail("the token endpoint answered HTTP " + convert.toString($status));
    }
    return tokenFromNode($node, $nowUnix);
}

# tokenRequest posts a token request and parses the answer.
func tokenRequest(url as string, params as map of string to string) {
    def r as http.Response init postForm($url, $params);
    return parseTokenBody($r.body, $r.status, nowUnix());
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
    if (json.typeOf($node) == "map" and json.has($node, "/error")) {
        if (json.typeOf($node, "/error") != "string") {
            return "invalid_response";
        }
        return shown(json.asString($node, "/error"));
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

# --- JWT bearer (private, pure) ------------------------------------

# assertionClaims builds the RFC 7523 claim set: `iss`, `sub` (left out when
# empty), `aud`, `iat` = now, `exp` = now + lifetime, then the caller's `extra`
# claims. `nowUnix` fixes the clock so the claims are testable.
func assertionClaims(
    issuer as string,
    subject as string,
    audience as string,
    lifetime as int,
    extra as json.Value,
    nowUnix as int) {
    if ($issuer == "" or $audience == "") {
        fail("jwtAssertion: issuer and audience must not be empty");
    }
    if ($lifetime <= 0) {
        fail("jwtAssertion: lifetime must be positive, got " + convert.toString($lifetime));
    }
    if (json.typeOf($extra) != "map") {
        fail("jwtAssertion: extra claims must be a JSON map");
    }
    def claims as json.Value init json.map();
    $claims = json.set($claims, "/iss", $issuer);
    if ($subject != "") {
        $claims = json.set($claims, "/sub", $subject);
    }
    $claims = json.set($claims, "/aud", $audience);
    $claims = json.set($claims, "/iat", $nowUnix);
    $claims = json.set($claims, "/exp", $nowUnix + $lifetime);
    for (def k in json.keys($extra)) {
        if (lists.contains(ASSERTION_CLAIMS, $k)) {
            fail("jwtAssertion: extra claims may not set \"" + $k + "\"");
        }
        $claims = json.set($claims, pointer($k), json.get($extra, pointer($k)));
    }
    return $claims;
}

# jwtBearerParams builds the RFC 7523 token request: the grant type and the
# assertion, plus `scope` and client authentication only when the config has
# them (a service-account assertion authenticates the client by itself).
func jwtBearerParams(config as Config, assertion as string) {
    def params as map of string to string init {"grant_type": JWT_BEARER, "assertion": $assertion};
    if ($config.scope != "") {
        $params["scope"] = $config.scope;
    }
    if ($config.clientId != "") {
        $params["client_id"] = $config.clientId;
    }
    if ($config.clientSecret != "") {
        $params["client_secret"] = $config.clientSecret;
    }
    return $params;
}

# --- net (private) -------------------------------------------------

# Token endpoints answer JSON, but some (GitHub's) only when asked to. The
# endpoint must be https (http only on this machine): the request carries the
# client secret, a refresh token or a code.
func postForm(url as string, params as map of string to string) {
    if (not endpointOk($url)) {
        fail("the endpoint must be an https URL, got " + shown($url));
    }
    return fetch(
        "POST",
        $url,
        {"Content-Type": "application/x-www-form-urlencoded", "Accept": "application/json"},
        formBody($params));
}

# fetch sends one request with the reply bounds: at most MAX_REPLY_BYTES of
# body, IDLE_TIMEOUT_MS per read, REQUEST_TIMEOUT_S in all (a server sending a
# byte now and then cannot hold the caller). A breach is an oauth error.
func fetch(method as string, url as string, headers as map of string to string, body as string) {
    def t as task of http.Response init spawn {
        return http.requestWith($method, $url, $headers, $body, IDLE_TIMEOUT_MS, MAX_REPLY_BYTES);
    };
    def r as http.Response;
    try {
        $r = task.waitTimeout($t, REQUEST_TIMEOUT_S * 1000);
    } catch (e) {
        if ($e.kind == "timeout") {
            task.cancel($t);
            task.discard($t);
            fail(shown($url) + " did not answer within " + convert.toString(REQUEST_TIMEOUT_S) +
                " s");
        }
        if ($e.kind == "http" and strings.contains($e.message, " exceeds ")) {
            fail("the reply from " + shown($url) + " is over " + convert.toString(MAX_REPLY_BYTES) +
                " bytes");
        }
        throw $e;
    }
    return $r;
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
    return tokenRequest($config.tokenUrl, clientParams($config));
}

# clientParams builds the client-credentials request; the secret and the scope
# only when the config has them (an empty `scope=` is refused by some servers).
func clientParams(config as Config) {
    def params as map of string to string init {
        "grant_type": "client_credentials",
        "client_id": $config.clientId
    };
    if ($config.clientSecret != "") {
        $params["client_secret"] = $config.clientSecret;
    }
    if ($config.scope != "") {
        $params["scope"] = $config.scope;
    }
    return $params;
}

# refreshParams builds the refresh request; the secret only when there is one
# (a public client has none, and Entra ID refuses an empty one).
func refreshParams(config as Config, refreshToken as string) {
    def params as map of string to string init {
        "grant_type": "refresh_token",
        "refresh_token": $refreshToken,
        "client_id": $config.clientId
    };
    if ($config.clientSecret != "") {
        $params["client_secret"] = $config.clientSecret;
    }
    return $params;
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
    def params as map of string to string init refreshParams($config, $refreshToken);
    def t as Token init tokenRequest($config.tokenUrl, $params);
    if (len($t.refreshToken) == 0) {
        $t.refreshToken = $refreshToken;
    }
    return $t;
}

/**
 * Build a signed JWT assertion for the JWT Bearer grant (RFC 7523). The claims
 * are `iss` = issuer, `sub` = subject (left out when ""), `aud` = audience
 * (usually the token endpoint URL), `iat` = now, and `exp` = now + lifetime,
 * plus any `extra` claims the provider wants (a `scope`, a `jti`, ...). `extra`
 * may not set the five claims above. Providers bound the lifetime differently
 * (often 1 to 60 minutes), so it is the caller's choice.
 * @param alg {string} the JOSE algorithm (e.g. "RS256", "ES256")
 * @param issuer {string} the `iss` claim (the client or service-account id)
 * @param subject {string} the `sub` claim ("" to leave it out)
 * @param audience {string} the `aud` claim
 * @param lifetime {int} seconds from now until the assertion expires (> 0)
 * @param extra {json.Value} additional claims, a JSON map (`json.map()` for none)
 * @param key {bytes} the signing key: a PEM private key for RS* / ES*, an HMAC secret for HS*
 * @param kid {string} the key id for the JWT header ("" for none)
 * @return {string} the compact signed assertion
 * @throws {Error} kind "oauth" on a bad issuer / audience / lifetime / `extra` (jwt errors pass)
 */
export func jwtAssertion(
    alg as string,
    issuer as string,
    subject as string,
    audience as string,
    lifetime as int,
    extra as json.Value,
    key as bytes,
    kid as string) {
    def claims as json.Value init assertionClaims(
        $issuer,
        $subject,
        $audience,
        $lifetime,
        $extra,
        nowUnix());
    return jwt.signWithKid($alg, $claims, $key, $kid);
}

/**
 * Trade a signed assertion (from `jwtAssertion`) for an access token with the
 * JWT Bearer grant (RFC 7523). Sends `config.scope` when set, and the client id
 * / secret only when the config has them - a service-account assertion
 * authenticates the client by itself, so those are usually "". `deviceUrl` is
 * not used.
 * @param config {Config} the client settings (`tokenUrl`, optional `scope` / client credentials)
 * @param assertion {string} the signed JWT assertion
 * @return {Token} the issued access token
 * @throws {Error} kind "oauth" on a token-endpoint error
 */
export func jwtBearer(config as Config, assertion as string) {
    def params as map of string to string init jwtBearerParams($config, $assertion);
    return tokenRequest($config.tokenUrl, $params);
}

/**
 * Begin the device flow: return the code + URL to show the user.
 * @param config {Config} the client settings
 * @return {DeviceAuth} the device-authorization handle
 * @throws {Error} kind "oauth" on a device-endpoint error
 */
export func deviceStart(config as Config) {
    def params as map of string to string init {"client_id": $config.clientId};
    if ($config.scope != "") {
        $params["scope"] = $config.scope;
    }
    def r as http.Response init postForm($config.deviceUrl, $params);
    def node as json.Value init decodeReply($r.body, $r.status, "device endpoint");
    if (json.typeOf($node) == "map" and json.has($node, "/error")) {
        fail(errorMessage($node));
    }
    if ($r.status != 200) {
        fail("the device endpoint answered HTTP " + convert.toString($r.status));
    }
    return deviceFromNode($node, nowUnix());
}

# deviceFromNode builds a DeviceAuth from a device-authorization response
# (RFC 8628 section 3.2): `device_code`, `user_code` and the verification URI
# (Google names it `verification_url`) are required strings; the user code is
# at most MAX_USER_CODE characters without control characters, the URI an https
# endpoint; `interval` (at most ten minutes) and `expires_in` (at most a day)
# are 5 s and 15 minutes when absent.
func deviceFromNode(node as json.Value, nowUnix as int) {
    if (json.typeOf($node) != "map") {
        fail("the device response is not a JSON object");
    }
    def what as string init "device response";
    def vuri as string init "";
    if (json.has($node, "/verification_uri")) {
        $vuri = textField($node, $what, "verification_uri", true);
    } else {
        $vuri = textField($node, $what, "verification_url", true);
    }
    def deviceCode as string init textField($node, $what, "device_code", true);
    def userCode as string init textField($node, $what, "user_code", true);
    if ($deviceCode == "" or $userCode == "" or $vuri == "") {
        fail("the device response has an empty device_code, user_code or verification URI");
    }
    if (len($userCode) > MAX_USER_CODE or regex.matches(CONTROL, $userCode)) {
        fail("the device response's user_code is over " + convert.toString(MAX_USER_CODE) +
            " characters or holds a control character: " + shown($userCode));
    }
    if (len($vuri) > MAX_VERIFICATION_URI or not endpointOk($vuri)) {
        fail("the device response's verification URI is not an https URL: " + shown($vuri));
    }
    return DeviceAuth{
        deviceCode: $deviceCode,
        userCode: $userCode,
        verificationUri: $vuri,
        interval: positiveField($node, $what, "interval", 5, MAX_POLL_INTERVAL),
        expiresAt: $nowUnix + positiveField($node, $what, "expires_in", 900, MAX_DEVICE_LIFETIME)
    };
}

/**
 * Poll the token endpoint until the user approves (or a terminal error),
 * returning the token. Sleeps `interval` seconds between polls, backing off on
 * `slow_down` (to at most ten minutes), and never past the code's expiry.
 * @param config {Config} the client settings
 * @param deviceAuth {DeviceAuth} the handle from `deviceStart`
 * @return {Token} the issued access token
 * @throws {Error} kind "oauth" if device authorization fails or expires, or the handle's interval is not 1 to 600 s
 */
export func deviceWait(config as Config, deviceAuth as DeviceAuth) {
    def interval as int init $deviceAuth.interval;
    if ($interval <= 0 or $interval > MAX_POLL_INTERVAL) {
        fail("deviceWait: the interval must be from 1 to " + convert.toString(MAX_POLL_INTERVAL) +
            " s, got " + convert.toString($interval));
    }
    while (true) {
        # Stop polling once the device code has expired instead of looping
        # forever against a lenient endpoint that keeps returning pending.
        if ($deviceAuth.expiresAt > 0 and nowUnix() >= $deviceAuth.expiresAt) {
            fail("device authorization expired before approval");
        }
        def params as map of string to string init {
            "grant_type": "urn:ietf:params:oauth:grant-type:device_code",
            "device_code": $deviceAuth.deviceCode,
            "client_id": $config.clientId
        };
        # Google's token endpoint requires the client secret in the device
        # polling request; include it when configured (public clients leave it
        # empty).
        if (not ($config.clientSecret == "")) {
            $params["client_secret"] = $config.clientSecret;
        }
        def r as http.Response init postForm($config.tokenUrl, $params);
        def state as string init pollState($r.body);
        if ($state == "success") {
            return parseTokenBody($r.body, $r.status, nowUnix());
        }
        if ($state == "slow_down") {
            $interval = $interval + 5;
            if ($interval > MAX_POLL_INTERVAL) {
                $interval = MAX_POLL_INTERVAL;
            }
        } elseif (not ($state == "authorization_pending")) {
            fail("device authorization failed: " + $state);
        }
        def pause as int init $interval;
        if ($deviceAuth.expiresAt > 0) {
            def left as int init $deviceAuth.expiresAt - nowUnix();
            if ($left <= 0) {
                fail("device authorization expired before approval");
            }
            if ($left < $pause) {
                $pause = $left;
            }
        }
        time.sleep(time.fromSeconds($pause));
    }
    return Token{
        accessToken: "",
        tokenType: "",
        refreshToken: "",
        scope: "",
        expiresAt: 0,
        idToken: "",
        extra: {}
    };
}

/**
 * Report whether a token has expired (30s skew buffer).
 * @param token {Token} the token to check
 * @return {bool} true if the token is at or past its expiry
 */
export func isExpired(token as Token) {
    return tokenExpired($token.expiresAt, nowUnix());
}

# --- discovery (exported) -------------------------------------------

# authority is the part of an absolute URL between `://` and the path, query
# or fragment ("" for a URL without `://`).
func authority(url as string) {
    def at as int init strings.indexOf($url, "://");
    if ($at < 0) {
        return "";
    }
    def rest as string init strings.substring($url, $at + 3);
    for (def stop in ["/", "?", "#"]) {
        def k as int init strings.indexOf($rest, $stop);
        if ($k >= 0) {
            $rest = strings.substring($rest, 0, $k);
        }
    }
    return $rest;
}

# endpointOk reports whether a URL is an endpoint: https, or plain http on this
# machine; a host, no login part (`user@`), no fragment, no space or control
# character.
func endpointOk(url as string) {
    if (regex.matches(SPACE_OR_CONTROL, $url) or strings.contains($url, "#") or
        strings.contains(authority($url), "@")) {
        return false;
    }
    def u as uri.Uri;
    try {
        $u = uri.parse($url);
    } catch (e) {
        return false;
    }
    if ($u.host == "" or $u.user != "" or not regex.matches('^[0-9]*$', $u.port)) {
        return false;
    }
    def scheme as string init strings.lower($u.scheme);
    if ($scheme == "https") {
        return true;
    }
    return $scheme == "http" and lists.contains(LOOPBACK, strings.lower($u.host));
}

# metadataUrls lists where an issuer's metadata may be: the OpenID Connect
# location (the issuer + `/.well-known/openid-configuration`), then RFC 8414's
# (`/.well-known/oauth-authorization-server` between the host and the path).
func metadataUrls(issuer as string) {
    def base as string init $issuer;
    while (strings.endsWith($base, "/")) {
        $base = strings.substring($base, 0, len($base) - 1);
    }
    def scheme as int init strings.indexOf($base, "://") + 3;
    def slash as int init strings.indexOf($base, "/", $scheme);
    def origin as string init $base;
    def path as string init "";
    if ($slash >= 0) {
        $origin = strings.substring($base, 0, $slash);
        $path = strings.substring($base, $slash);
    }
    return [
        $base + "/.well-known/openid-configuration",
        $origin + "/.well-known/oauth-authorization-server" + $path
    ];
}

# metadataEndpoint reads an endpoint from a metadata document: "" when absent
# and optional, else it must be an https (or loopback http) URL.
func metadataEndpoint(node as json.Value, name as string, required as bool) {
    if (not json.has($node, "/" + $name)) {
        if ($required) {
            fail("discover: the metadata has no " + $name);
        }
        return "";
    }
    if (json.typeOf($node, "/" + $name) != "string") {
        fail("discover: " + $name + " is not a string");
    }
    def url as string init json.asString($node, "/" + $name);
    if (not endpointOk($url)) {
        fail("discover: " + $name + " must be an https URL, got " + shown($url));
    }
    return $url;
}

/**
 * Read a provider's endpoints from its metadata document (OpenID Connect
 * Discovery, then RFC 8414) and return a Config for them. The document must
 * name exactly this `issuer`, and every endpoint must be https (plain http
 * only on this machine, for a test server). `deviceUrl` / `authorizeUrl` are
 * "" when the provider offers no such endpoint.
 * @param issuer {string} the issuer URL, e.g. "https://accounts.google.com"
 * @param clientId {string} the OAuth2 client identifier
 * @param clientSecret {string} the client secret ("" for a public client)
 * @param scope {string} the space-separated requested scopes
 * @return {Config} the client settings
 * @throws {Error} kind "oauth" if the issuer is not https, no metadata is found, or it is malformed or names another issuer
 */
export func discover(
    issuer as string,
    clientId as string,
    clientSecret as string,
    scope as string) {
    if (not endpointOk($issuer) or strings.contains($issuer, "?")) {
        fail("discover: the issuer must be an https URL without a query, got " + shown($issuer));
    }
    def found as bool init false;
    def body as string init "";
    def tried as list of string init [];
    for (def url in metadataUrls($issuer)) {
        def r as http.Response init fetch("GET", $url, {"Accept": "application/json"}, "");
        if ($r.status == 200) {
            $found = true;
            $body = $r.body;
            break;
        }
        $tried[] = $url + " (HTTP " + convert.toString($r.status) + ")";
    }
    if (not $found) {
        fail("discover: no metadata at " + shown(strings.join($tried, " or ")));
    }
    def node as json.Value;
    try {
        $node = json.decode($body);
    } catch (e) {
        fail("discover: the metadata is not JSON");
    }
    if (json.typeOf($node) != "map") {
        fail("discover: the metadata is not a JSON object");
    }
    if (json.typeOf($node, "/issuer") != "string" or json.asString($node, "/issuer") != $issuer) {
        fail("discover: the metadata does not name the issuer " + shown($issuer));
    }
    return Config{
        tokenUrl: metadataEndpoint($node, "token_endpoint", true),
        deviceUrl: metadataEndpoint($node, "device_authorization_endpoint", false),
        authorizeUrl: metadataEndpoint($node, "authorization_endpoint", false),
        clientId: $clientId,
        clientSecret: $clientSecret,
        scope: $scope
    };
}

# --- client assertions (exported) ------------------------------------

/**
 * Build a client assertion (RFC 7523 section 2.2, OpenID Connect
 * `private_key_jwt`): the client proves who it is with a JWT signed by its own
 * key instead of a secret. `iss` and `sub` are the client id, `aud` the token
 * endpoint (`audience`), a random `jti`, valid for five minutes.
 * @param alg {string} the JOSE algorithm (e.g. "RS256", "ES256")
 * @param clientId {string} the client identifier
 * @param audience {string} the `aud` claim, usually the token endpoint URL
 * @param key {bytes} the signing key (PEM private key for RS* / ES*)
 * @param kid {string} the key id for the JWT header ("" for none)
 * @return {string} the compact signed assertion
 * @throws {Error} kind "oauth" on an empty client id or audience (jwt errors pass)
 */
export func clientAssertion(
    alg as string,
    clientId as string,
    audience as string,
    key as bytes,
    kid as string) {
    if ($clientId == "") {
        fail("clientAssertion: the client id must not be empty");
    }
    def extra as json.Value init json.set(json.map(), "/jti", base64url(crypto.randBytes(16)));
    return jwtAssertion(
        $alg,
        $clientId,
        $clientId,
        $audience,
        CLIENT_ASSERTION_LIFETIME,
        $extra,
        $key,
        $kid);
}

# clientAssertionParams builds a client-credentials request that authenticates
# with an assertion instead of a secret.
func clientAssertionParams(config as Config, assertion as string) {
    def params as map of string to string init {
        "grant_type": "client_credentials",
        "client_id": $config.clientId,
        "client_assertion_type": CLIENT_ASSERTION_TYPE,
        "client_assertion": $assertion
    };
    if ($config.scope != "") {
        $params["scope"] = $config.scope;
    }
    return $params;
}

/**
 * The Client Credentials grant, the client authenticating with a signed
 * assertion (from `clientAssertion`) instead of `clientSecret`, which is not
 * sent.
 * @param config {Config} the client settings (`tokenUrl`, `clientId`, optional `scope`)
 * @param assertion {string} the signed client assertion
 * @return {Token} the issued access token
 * @throws {Error} kind "oauth" on a token-endpoint error
 */
export func clientCredentialsJwt(config as Config, assertion as string) {
    def params as map of string to string init clientAssertionParams($config, $assertion);
    return tokenRequest($config.tokenUrl, $params);
}

# --- authorization code + PKCE (exported) ----------------------------

# base64url encodes bytes without padding (RFC 7636 / RFC 7515).
func base64url(b as bytes) {
    return strings.replace(encoding.toText("base64-url", $b), "=", "");
}

# challengeOf is the S256 PKCE challenge for a verifier.
func challengeOf(verifier as string) {
    return base64url(hash.compute("sha256", convert.bytesFromString("utf-8", $verifier)));
}

# The parameters `authorize` sets itself, which `authorizeWith`'s extras may not.
def const AUTHORIZE_PARAMS as list of string init [
    "response_type",
    "client_id",
    "redirect_uri",
    "scope",
    "state",
    "code_challenge",
    "code_challenge_method"
];

# redirectOk reports whether a redirect URI is usable: https, http on this
# machine (RFC 8252 section 7.3), or a private-use scheme named like a reverse
# domain (`com.example.app:/cb`, section 7.1); absolute, no fragment, no space.
func redirectOk(url as string) {
    if (regex.matches(SPACE_OR_CONTROL, $url) or strings.contains($url, "#")) {
        return false;
    }
    def u as uri.Uri;
    try {
        $u = uri.parse($url);
    } catch (e) {
        return false;
    }
    def scheme as string init strings.lower($u.scheme);
    if ($scheme == "http" or $scheme == "https") {
        return endpointOk($url);
    }
    return regex.matches('^[a-z][a-z0-9+-]*(\.[a-z][a-z0-9+-]*)+$', $scheme) and $u.user == "";
}

/**
 * Start an authorization-code sign-in with PKCE (RFC 7636, S256): a fresh
 * random `state` and code verifier, and the URL to open in the user's browser.
 * The provider sends the browser back to `redirectUri` with a code: catch it
 * with `awaitCode` (a loopback `httpd` server, `http://127.0.0.1:<port>/...`,
 * as RFC 8252 advises for programs on the user's machine) or read it from the
 * redirected URL with `codeFromRedirect`, then call `exchangeCode`. The
 * authorization URL must be https (http only on this machine); the redirect
 * URI https, http on this machine, or a private-use scheme like
 * `com.example.app:/callback`.
 * @param config {Config} the client settings (`authorizeUrl`, `clientId`, `scope`)
 * @param redirectUri {string} the redirect URI registered with the provider
 * @return {AuthRequest} the sign-in in progress
 * @throws {Error} kind "oauth" if the config has no usable authorize URL or no client id, or the redirect URI is unusable
 */
export func authorize(config as Config, redirectUri as string) {
    return authorizeWith($config, $redirectUri, {});
}

/**
 * `authorize`, with more parameters for the authorization URL: a provider's
 * own (`access_type=offline` and `prompt=consent` for a Google refresh token)
 * or OpenID Connect's (`nonce`, `login_hint`, `prompt`). They may not replace
 * the ones `authorize` sets (`response_type`, `client_id`, `redirect_uri`,
 * `scope`, `state`, `code_challenge`, `code_challenge_method`).
 * @param config {Config} the client settings (`authorizeUrl`, `clientId`, `scope`)
 * @param redirectUri {string} the redirect URI registered with the provider
 * @param extra {map of string to string} the added parameters
 * @return {AuthRequest} the sign-in in progress
 * @throws {Error} kind "oauth" as `authorize`, or if `extra` names a parameter `authorize` sets
 */
export func authorizeWith(
    config as Config,
    redirectUri as string,
    extra as map of string to string) {
    if ($config.authorizeUrl == "") {
        fail("authorize: the config has no authorizeUrl");
    }
    if (not endpointOk($config.authorizeUrl)) {
        fail("authorize: the authorizeUrl must be https, got " + shown($config.authorizeUrl));
    }
    if ($config.clientId == "") {
        fail("authorize: the config has no clientId");
    }
    if (not redirectOk($redirectUri)) {
        fail("authorize: the redirect URI must be https, http on this machine, or a scheme like " +
            "com.example.app:/callback, without a fragment, got " + shown($redirectUri));
    }
    for (def k in maps.keys($extra)) {
        if (lists.contains(AUTHORIZE_PARAMS, $k)) {
            fail("authorize: an extra parameter may not set " + shown($k));
        }
    }
    def state as string init base64url(crypto.randBytes(16));
    def verifier as string init base64url(crypto.randBytes(32));
    def params as map of string to string init {
        "response_type": "code",
        "client_id": $config.clientId,
        "redirect_uri": $redirectUri
    };
    if ($config.scope != "") {
        $params["scope"] = $config.scope;
    }
    $params["state"] = $state;
    $params["code_challenge"] = challengeOf($verifier);
    $params["code_challenge_method"] = "S256";
    for (def k in maps.keys($extra)) {
        $params[$k] = $extra[$k];
    }
    def sep as string init "?";
    if (strings.contains($config.authorizeUrl, "?")) {
        $sep = "&";
    }
    return AuthRequest{
        url: $config.authorizeUrl + $sep + formBody($params),
        state: $state,
        verifier: $verifier,
        redirectUri: $redirectUri
    };
}

# queryParams decodes a redirect's query, refusing a parameter that repeats
# (RFC 6749 section 3.1) or a malformed escape.
func queryParams(q as string) {
    def out as map of string to string init {};
    if ($q == "") {
        return $out;
    }
    for (def pair in strings.split($q, "&")) {
        if ($pair == "") {
            continue;
        }
        def one as map of string to string;
        try {
            $one = uri.parseQuery($pair);
        } catch (e) {
            fail("authorization: the redirect's query is malformed");
        }
        for (def k in maps.keys($one)) {
            if (maps.has($out, $k)) {
                fail("authorization: the redirect repeats " + shown($k));
            }
            $out[$k] = $one[$k];
        }
    }
    return $out;
}

# codeFrom checks a redirect's parameters against the request: the state must
# match, a provider error is raised, and the code is returned.
func codeFrom(req as AuthRequest, params as map of string to string) {
    if (not maps.has($params, "state") or $params["state"] != $req.state) {
        fail("authorization: the redirect's state does not match the request");
    }
    if (maps.has($params, "error")) {
        def why as string init shown($params["error"]);
        if (maps.has($params, "error_description")) {
            $why = $why + ": " + shown($params["error_description"]);
        }
        fail("authorization refused: " + $why);
    }
    if (not maps.has($params, "code") or $params["code"] == "") {
        fail("authorization: the redirect carries no code");
    }
    return $params["code"];
}

# sameEndpoint reports whether a redirected URL goes to the redirect URI: the
# same scheme, host and port (case-insensitive) and the same path.
func sameEndpoint(redirected as string, redirectUri as string) {
    def a as uri.Uri;
    def b as uri.Uri;
    try {
        $a = uri.parse($redirected);
        $b = uri.parse($redirectUri);
    } catch (e) {
        return false;
    }
    def pa as string init $a.path;
    def pb as string init $b.path;
    if ($pa == "") {
        $pa = "/";
    }
    if ($pb == "") {
        $pb = "/";
    }
    return strings.lower($a.scheme) == strings.lower($b.scheme) and
        strings.lower($a.host) == strings.lower($b.host) and $a.port == $b.port and $pa == $pb and
        $a.user == "";
}

/**
 * The code from the URL the provider redirected the browser to, or just its
 * query string, checked against the request: a URL must go to the request's
 * redirect URI, the `state` must match, and no parameter may repeat.
 * @param req {AuthRequest} the sign-in, from `authorize`
 * @param redirected {string} the redirected URL, or its query
 * @return {string} the authorization code
 * @throws {Error} kind "oauth" on another redirect target, a state mismatch, a repeated parameter, a provider error (e.g. access_denied), or no code
 */
export func codeFromRedirect(req as AuthRequest, redirected as string) {
    def q as string init $redirected;
    def hashAt as int init strings.indexOf($q, "#");
    if ($hashAt >= 0) {
        $q = strings.substring($q, 0, $hashAt);
    }
    def qm as int init strings.indexOf($q, "?");
    if ($qm >= 0) {
        def target as string init strings.substring($q, 0, $qm);
        if ($target != "" and not sameEndpoint($target, $req.redirectUri)) {
            fail("authorization: the redirect goes to " + shown($target) +
                ", not to the redirect URI");
        }
        $q = strings.substring($q, $qm + 1);
    }
    return codeFrom($req, queryParams($q));
}

/**
 * Wait on a loopback `httpd` server for the provider's redirect, answer the
 * browser with a short page, and return the code. Requests for another path,
 * carrying another `state`, or with a malformed or repeated parameter are
 * answered and ignored, however many arrive. It blocks until the redirect
 * arrives: to bound the wait, run it in a task with `task.waitTimeout` and
 * `httpd.shutdown` the server on a timeout (the wait then throws).
 * @param req {AuthRequest} the sign-in, from `authorize`; its redirect URI points at `srv`
 * @param srv {httpd.Server} the listening server
 * @return {string} the authorization code
 * @throws {Error} kind "oauth" on a provider error (e.g. access_denied) or a redirect without a code
 */
export func awaitCode(req as AuthRequest, srv as httpd.Server) {
    def want as string init uri.parse($req.redirectUri).path;
    if ($want == "") {
        $want = "/";
    }
    while (true) {
        def r as httpd.Request init httpd.accept($srv);
        httpd.setHeader($r, "Content-Type", "text/plain; charset=utf-8");
        # the browser would keep the connection open and hold up a shutdown
        httpd.setHeader($r, "Connection", "close");
        if (httpd.path($r) != $want) {
            httpd.respond($r, 404, "not found\n");
            continue;
        }
        def params as map of string to string;
        try {
            $params = queryParams(httpd.rawQuery($r));
        } catch (e) {
            httpd.respond($r, 400, "malformed redirect\n");
            continue;
        }
        if (not maps.has($params, "state") or $params["state"] != $req.state) {
            httpd.respond($r, 404, "not found\n");
            continue;
        }
        def code as string init "";
        try {
            $code = codeFrom($req, $params);
        } catch (e) {
            httpd.respond($r, 400, "Sign-in failed. You can close this window.\n");
            throw $e;
        }
        httpd.respond($r, 200, "Signed in. You can close this window.\n");
        return $code;
    }
    return "";
}

/**
 * Trade an authorization code for a token, sending the request's PKCE code
 * verifier and redirect URI, and the client secret when the config has one.
 * @param config {Config} the client settings (`tokenUrl`, `clientId`, optional `clientSecret`)
 * @param req {AuthRequest} the sign-in, from `authorize`
 * @param code {string} the code from `awaitCode` or `codeFromRedirect`
 * @return {Token} the issued token
 * @throws {Error} kind "oauth" on a token-endpoint error
 */
export func exchangeCode(config as Config, req as AuthRequest, code as string) {
    def params as map of string to string init codeParams($config, $req, $code);
    return tokenRequest($config.tokenUrl, $params);
}

# codeParams builds the authorization-code token request.
func codeParams(config as Config, req as AuthRequest, code as string) {
    def params as map of string to string init {
        "grant_type": "authorization_code",
        "code": $code,
        "redirect_uri": $req.redirectUri,
        "client_id": $config.clientId,
        "code_verifier": $req.verifier
    };
    if ($config.clientSecret != "") {
        $params["client_secret"] = $config.clientSecret;
    }
    return $params;
}

# --- token store (exported) ----------------------------------------

/**
 * Write a token to a file as JSON (its own field shape, round-trips with load;
 * absolute `expiresAt` is preserved). The token goes to a new owner-only
 * (`0600`) file beside `path`, which then replaces `path`: a crash leaves the
 * old file whole, and a symbolic link at `path` is replaced, not followed. An
 * OAuth token is a bearer credential - the file-permission model `gh` / `aws`
 * use for their token stores.
 * @param path {string} the file path to write
 * @param token {Token} the token to persist
 */
export func save(path as string, token as Token) {
    # The new file is made owner-only *before* the token is written into it,
    # and created exclusively, so nothing else can be at that name.
    def tmp as string init $path + ".tmp-" + base64url(crypto.randBytes(9));
    fs.writeNew($tmp, "");
    try {
        fs.chmod($tmp, 0o600);
        fs.writeString($tmp, json.encode($token));
        fs.rename($tmp, $path);
    } catch (e) {
        if (fs.exists($tmp)) {
            fs.remove($tmp);
        }
        throw $e;
    }
}

/**
 * Read a token previously written by save. A file saved before 0.26.0 has no
 * `idToken` / `extra` and loads with them empty.
 * @param path {string} the file path to read
 * @return {Token} the loaded token
 * @throws {Error} kind "oauth" if the file is not a saved token
 */
export func load(path as string) {
    def node as json.Value;
    try {
        $node = json.decode(fs.readString($path));
    } catch (e) {
        fail("saved token file is not valid JSON: " + shown($path));
    }
    def t as Token;
    try {
        $t = tokenFromFile($node);
    } catch (e) {
        fail("saved token file is malformed: " + shown($path));
    }
    return $t;
}

# tokenFromFile reads the fields `save` writes; any missing or mistyped one is
# an error (load reports it).
func tokenFromFile(node as json.Value) {
    def idToken as string init "";
    if (json.has($node, "/idToken")) {
        $idToken = json.asString($node, "/idToken");
    }
    def extra as map of string to string init {};
    if (json.has($node, "/extra")) {
        for (def k in json.keys($node, "/extra")) {
            $extra[$k] = json.asString($node, "/extra" + pointer($k));
        }
    }
    return Token{
        accessToken: json.asString($node, "/accessToken"),
        tokenType: json.asString($node, "/tokenType"),
        refreshToken: json.asString($node, "/refreshToken"),
        scope: json.asString($node, "/scope"),
        expiresAt: json.asInt($node, "/expiresAt"),
        idToken: $idToken,
        extra: $extra
    };
}
