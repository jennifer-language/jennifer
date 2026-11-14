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
 * error, a reply that is malformed (a missing or mistyped field, an empty
 * token, a lifetime out of range), and a request that gets no usable reply (a
 * refused connection, DNS, TLS, a timeout, a reply over 1 MiB) surface as a
 * catchable `oauth.OAuthError` (kind "oauth"): a provider's answer carries
 * its `errorCode` (`invalid_grant`, `access_denied`, ...), `description` and
 * HTTP `status`. `devicePoll`, `exchangeCodeWith` and
 * `discoverMetadata` serve a server that relays sign-ins for its own clients.
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
def const IDLE_TIMEOUT_S as int init 15;
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
 * @field tls {http.TlsOptions} how the endpoints' certificates are checked: the zero value verifies against the system's roots; `caCert` (PEM) trusts a private CA instead, as for a self-hosted provider
 */
export def struct Config {
    tokenUrl as string,
    deviceUrl as string,
    authorizeUrl as string,
    clientId as string,
    clientSecret as string,
    scope as string,
    tls as http.TlsOptions
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

/**
 * The outcome of one device-flow poll, from `devicePoll`.
 * @field state {string} "ok" (`token` is set), "pending" (authorization_pending), "slow_down", or "failed" (any other answer: see `errorCode` and `status`)
 * @field token {Token} the issued token when `state` is "ok"
 * @field errorCode {string} the provider's `error` code ("" when it sent none, e.g. an HTML 5xx page)
 * @field description {string} the provider's `error_description`, or what was wrong with the reply ("" when none)
 * @field status {int} the HTTP status of the reply
 */
export def struct DevicePoll {
    state as string,
    token as Token,
    errorCode as string,
    description as string,
    status as int
};

/**
 * A provider's metadata, from `discoverMetadata`.
 * @field config {Config} the client settings, as `discover` returns them
 * @field userinfoUrl {string} the OpenID Connect UserInfo endpoint ("" when the provider names none), checked like the others
 * @field metadata {json.Value} the whole metadata document, for the fields the module does not read (`jwks_uri`, `end_session_endpoint`, ...)
 */
export def struct Discovery {
    config as Config,
    userinfoUrl as string,
    metadata as json.Value
};

# formBody renders a parameter map as a form-encoded request body
# (`application/x-www-form-urlencoded`, insertion order preserved) via the
# shared `uri` module.
func formBody(params as map of string to string) {
    return uri.buildQuery($params);
}

/**
 * A failed OAuth operation. Every error of kind `"oauth"` is one, so its fields
 * are safe to read once the kind matches. When a provider answered with an
 * OAuth error (RFC 6749 section 5.2, RFC 8628, or the authorization
 * redirect), `errorCode` is its code, such as `invalid_grant`,
 * `access_denied` or `expired_token`; otherwise it is `""` and the message
 * says what was wrong.
 * @field kind {string} always `"oauth"`
 * @field message {string} what went wrong, with the provider's code and description when it sent them
 * @field errorCode {string} the provider's `error`, "" when the failure is not a provider's OAuth error
 * @field description {string} the provider's `error_description`, "" when it sent none
 * @field status {int} the HTTP status of the provider's reply; 0 when no reply was involved (a redirect, a local check)
 * @example
 * try {
 *     $token = oauth.refresh($config, $token.refreshToken);
 * } catch (e) {
 *     if ($e.kind == "oauth" and $e.errorCode == "invalid_grant") {
 *         io.printf("the login expired; sign in again\n");
 *     } else {
 *         throw $e;
 *     }
 * }
 */
export def struct OAuthError {
    kind as string,
    message as string,
    errorCode as string,
    description as string,
    status as int
};

# --- token parsing (private, pure) ---------------------------------

func fail(message as string) {
    failWith($message, "", "", 0);
}

# failWith throws an OAuthError carrying a provider's error code, description
# and HTTP status.
func failWith(message as string, errorCode as string, description as string, status as int) {
    throw OAuthError{
        kind: "oauth",
        message: $message,
        errorCode: $errorCode,
        description: $description,
        status: $status
    };
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

# providerError throws the OAuthError of an OAuth2 error response (`error` +
# `error_description`, both strings), with the reply's HTTP status.
func providerError(node as json.Value, status as int) {
    if (json.typeOf($node, "/error") != "string") {
        fail("malformed error response: error is not a string");
    }
    def code as string init json.asString($node, "/error");
    def description as string init "";
    def msg as string init shown($code);
    if (json.has($node, "/error_description")) {
        if (json.typeOf($node, "/error_description") != "string") {
            fail("malformed error response: error_description is not a string");
        }
        $description = json.asString($node, "/error_description");
        $msg = $msg + ": " + shown($description);
    }
    failWith($msg, $code, $description, $status);
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
        failWith(
            "non-JSON response from the " + $endpoint + " (HTTP " + convert.toString($status) + ")",
            "",
            "",
            $status);
    }
    return $node;
}

# parseTokenBody parses a token-endpoint response, throwing on an OAuth2 error
# field, or on any other answer than 200.
func parseTokenBody(body as string, status as int, nowUnix as int) {
    def node as json.Value init decodeReply($body, $status, "token endpoint");
    if (json.typeOf($node) == "map" and json.has($node, "/error")) {
        providerError($node, $status);
    }
    if ($status != 200) {
        failWith("the token endpoint answered HTTP " + convert.toString($status), "", "", $status);
    }
    return tokenFromNode($node, $nowUnix);
}

# tokenRequest posts a token request and parses the answer.
func tokenRequest(url as string, params as map of string to string, tls as http.TlsOptions) {
    def r as http.Response init postForm($url, $params, $tls);
    return parseTokenBody($r.body, $r.status, nowUnix());
}

# pollOutcome classifies a device-poll reply (RFC 8628 section 3.5): a token,
# authorization_pending, slow_down, or anything else as "failed" with the
# provider's error code and the HTTP status. A JSON reply with a malformed
# `error` or a malformed token is an oauth error, as for any token reply.
func pollOutcome(body as string, status as int, nowUnix as int) {
    def none as Token init Token{
        accessToken: "",
        tokenType: "",
        refreshToken: "",
        scope: "",
        expiresAt: 0,
        idToken: "",
        extra: {}
    };
    def node as json.Value;
    try {
        $node = json.decode($body);
    } catch (e) {
        return DevicePoll{
            state: "failed",
            token: $none,
            errorCode: "",
            description: "the reply is not JSON",
            status: $status
        };
    }
    if (json.typeOf($node) == "map" and json.has($node, "/error")) {
        def code as string init textField($node, "token response", "error", true);
        if (not regex.matches('^[\x20-\x21\x23-\x5b\x5d-\x7e]{1,128}$', $code)) {
            fail("the token response's error is not an error code: " + shown($code));
        }
        def state as string init "failed";
        if ($code == "authorization_pending") {
            $state = "pending";
        } elseif ($code == "slow_down") {
            $state = "slow_down";
        }
        return DevicePoll{
            state: $state,
            token: $none,
            errorCode: $code,
            description: textField($node, "token response", "error_description", false),
            status: $status
        };
    }
    if ($status != 200) {
        return DevicePoll{
            state: "failed",
            token: $none,
            errorCode: "",
            description: "the reply has no error code",
            status: $status
        };
    }
    return DevicePoll{
        state: "ok",
        token: tokenFromNode($node, $nowUnix),
        errorCode: "",
        description: "",
        status: $status
    };
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
func postForm(url as string, params as map of string to string, tls as http.TlsOptions) {
    if (not endpointOk($url)) {
        fail("the endpoint must be an https URL, got " + shown($url));
    }
    return fetch(
        "POST",
        $url,
        {"Content-Type": "application/x-www-form-urlencoded", "Accept": "application/json"},
        formBody($params),
        $tls);
}

# fetch sends one request with the reply bounds: at most MAX_REPLY_BYTES of
# body, IDLE_TIMEOUT_S per read, REQUEST_TIMEOUT_S in all (a server sending a
# byte now and then cannot hold the caller; cancelling the request task ends its
# read and closes the socket). A breach, and any failure to get a reply (a
# refused connection, DNS, TLS, a malformed reply), is an oauth error. tls
# is the config's TlsOptions (the zero value verifies as usual).
func fetch(
    method as string,
    url as string,
    headers as map of string to string,
    body as string,
    tls as http.TlsOptions) {
    def t as task of http.Response init spawn {
        return http.requestWithTls(
            $method,
            $url,
            $headers,
            $body,
            IDLE_TIMEOUT_S * 1000,
            MAX_REPLY_BYTES,
            $tls);
    };
    def r as http.Response;
    try {
        $r = task.waitTimeout($t, REQUEST_TIMEOUT_S * 1000);
    } catch (e) {
        # The caller's own task was cancelled: that is not a network failure.
        if ($e.kind == "cancelled") {
            task.cancel($t);
            task.discard($t);
            throw $e;
        }
        if ($e.kind == "timeout" and strings.contains($e.message, "task.waitTimeout")) {
            # cancelling ends the request's reads and closes its socket
            task.cancel($t);
            task.discard($t);
            fail(shown($url) + " did not answer within " + convert.toString(REQUEST_TIMEOUT_S) +
                " s");
        }
        if ($e.kind == "timeout") {
            fail(shown($url) + " stalled for " + convert.toString(IDLE_TIMEOUT_S) + " s");
        }
        if ($e.kind == "limit") {
            fail("the reply from " + shown($url) + " is over " + convert.toString(MAX_REPLY_BYTES) +
                " bytes");
        }
        # a refused or reset connection, a DNS or TLS failure, a malformed reply
        fail("the request to " + shown($url) + " failed: " + shown($e.message));
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
 * @throws {OAuthError} kind "oauth" on a token-endpoint error
 */
export func clientCredentials(config as Config) {
    return tokenRequest($config.tokenUrl, clientParams($config), $config.tls);
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
 * @throws {OAuthError} kind "oauth" on a token-endpoint error
 */
export func refresh(config as Config, refreshToken as string) {
    def params as map of string to string init refreshParams($config, $refreshToken);
    def t as Token init tokenRequest($config.tokenUrl, $params, $config.tls);
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
 * @throws {OAuthError} kind "oauth" on a bad issuer / audience / lifetime / `extra` (jwt errors pass)
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
 * @throws {OAuthError} kind "oauth" on a token-endpoint error
 */
export func jwtBearer(config as Config, assertion as string) {
    def params as map of string to string init jwtBearerParams($config, $assertion);
    return tokenRequest($config.tokenUrl, $params, $config.tls);
}

/**
 * Begin the device flow: return the code + URL to show the user.
 * @param config {Config} the client settings
 * @return {DeviceAuth} the device-authorization handle
 * @throws {OAuthError} kind "oauth" on a device-endpoint error
 */
export func deviceStart(config as Config) {
    def params as map of string to string init {"client_id": $config.clientId};
    if ($config.scope != "") {
        $params["scope"] = $config.scope;
    }
    def r as http.Response init postForm($config.deviceUrl, $params, $config.tls);
    def node as json.Value init decodeReply($r.body, $r.status, "device endpoint");
    if (json.typeOf($node) == "map" and json.has($node, "/error")) {
        providerError($node, $r.status);
    }
    if ($r.status != 200) {
        failWith(
            "the device endpoint answered HTTP " + convert.toString($r.status),
            "",
            "",
            $r.status);
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
 * @throws {OAuthError} kind "oauth" if device authorization fails or expires, or the handle's interval is not 1 to 600 s or it has no expiry
 */
export func deviceWait(config as Config, deviceAuth as DeviceAuth) {
    def interval as int init $deviceAuth.interval;
    if ($interval <= 0 or $interval > MAX_POLL_INTERVAL) {
        fail("deviceWait: the interval must be from 1 to " + convert.toString(MAX_POLL_INTERVAL) +
            " s, got " + convert.toString($interval));
    }
    if ($deviceAuth.expiresAt <= 0) {
        fail("deviceWait: the handle has no expiry (expiresAt " +
            convert.toString($deviceAuth.expiresAt) + ")");
    }
    while (true) {
        # Stop polling once the device code has expired instead of looping
        # forever against a lenient endpoint that keeps returning pending.
        if (nowUnix() >= $deviceAuth.expiresAt) {
            fail("device authorization expired before approval");
        }
        def p as DevicePoll init devicePoll($config, $deviceAuth);
        if ($p.state == "ok") {
            return $p.token;
        }
        if ($p.state == "slow_down") {
            $interval = $interval + 5;
            if ($interval > MAX_POLL_INTERVAL) {
                $interval = MAX_POLL_INTERVAL;
            }
        } elseif ($p.state == "failed") {
            # A server fault without an error code (an HTML 5xx page) may pass;
            # anything else is the answer.
            if (not ($p.errorCode == "" and $p.status >= 500)) {
                failWith(
                    "device authorization failed: " + pollFailure($p),
                    $p.errorCode,
                    $p.description,
                    $p.status);
            }
        }
        def pause as int init $interval;
        def left as int init $deviceAuth.expiresAt - nowUnix();
        if ($left <= 0) {
            fail("device authorization expired before approval");
        }
        if ($left < $pause) {
            $pause = $left;
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

# pollFailure renders a failed poll for an error message: the error code and
# description, or what was wrong with the reply and its HTTP status.
func pollFailure(p as DevicePoll) {
    if ($p.errorCode != "") {
        def msg as string init shown($p.errorCode);
        if ($p.description != "") {
            $msg = $msg + ": " + shown($p.description);
        }
        return $msg;
    }
    return $p.description + " (HTTP " + convert.toString($p.status) + ")";
}

# devicePollParams builds a device-flow poll; the secret only when there is one
# (Google requires it, a public client has none).
func devicePollParams(config as Config, deviceAuth as DeviceAuth) {
    def params as map of string to string init {
        "grant_type": "urn:ietf:params:oauth:grant-type:device_code",
        "device_code": $deviceAuth.deviceCode,
        "client_id": $config.clientId
    };
    if ($config.clientSecret != "") {
        $params["client_secret"] = $config.clientSecret;
    }
    return $params;
}

/**
 * Poll the token endpoint once for a device authorization, without waiting:
 * for a server that relays the device flow to its own clients and polls on
 * their behalf, one poll per request. `deviceWait` is the loop around it. The
 * caller keeps to `interval` (wider after "slow_down") and stops at
 * `expiresAt`.
 * @param config {Config} the client settings
 * @param deviceAuth {DeviceAuth} the handle from `deviceStart` (or one rebuilt from its fields)
 * @return {DevicePoll} the outcome: "ok" with the token, "pending", "slow_down", or "failed" with the error code and HTTP status
 * @throws {OAuthError} kind "oauth" if the endpoint is not https, cannot be reached, or sends a malformed error or token
 */
export func devicePoll(config as Config, deviceAuth as DeviceAuth) {
    if ($deviceAuth.deviceCode == "") {
        fail("devicePoll: the handle has no device code");
    }
    def r as http.Response init postForm(
        $config.tokenUrl,
        devicePollParams($config, $deviceAuth),
        $config.tls);
    return pollOutcome($r.body, $r.status, nowUnix());
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
 * @throws {OAuthError} kind "oauth" if the issuer is not https, no metadata is found, or it is malformed or names another issuer
 */
export func discover(
    issuer as string,
    clientId as string,
    clientSecret as string,
    scope as string) {
    def tls as http.TlsOptions;
    return discoverMetadataTls($issuer, $clientId, $clientSecret, $scope, $tls).config;
}

/**
 * `discover` with TLS options: the metadata is fetched with them, and the
 * Config carries them to every later request, as for a provider whose
 * certificate a private CA signed.
 * @param issuer {string} the issuer URL
 * @param clientId {string} the OAuth2 client identifier
 * @param clientSecret {string} the client secret ("" for a public client)
 * @param scope {string} the space-separated requested scopes
 * @param tls {http.TlsOptions} how the certificates are checked
 * @return {Config} the client settings, `tls` included
 * @throws {OAuthError} kind "oauth" as `discover`
 * @example
 *   def tls as http.TlsOptions init http.TlsOptions{caCert: fs.readBytes("ca.pem")};
 *   def cfg as oauth.Config init oauth.discoverTls("https://sso.lan/realms/main",
 *       "id", "secret", "openid", $tls);
 */
export func discoverTls(
    issuer as string,
    clientId as string,
    clientSecret as string,
    scope as string,
    tls as http.TlsOptions) {
    return discoverMetadataTls($issuer, $clientId, $clientSecret, $scope, $tls).config;
}

/**
 * `discover`, also returning the OpenID Connect UserInfo endpoint (checked like
 * the others) and the whole metadata document, for a program that reads more
 * of it - who signed in, the signing keys. Keep the result to skip the fetch
 * on later sign-ins.
 * @param issuer {string} the issuer URL, e.g. "https://accounts.google.com"
 * @param clientId {string} the OAuth2 client identifier
 * @param clientSecret {string} the client secret ("" for a public client)
 * @param scope {string} the space-separated requested scopes
 * @return {Discovery} the client settings, the UserInfo URL and the document
 * @throws {OAuthError} kind "oauth" as `discover`
 */
export func discoverMetadata(
    issuer as string,
    clientId as string,
    clientSecret as string,
    scope as string) {
    def tls as http.TlsOptions;
    return discoverMetadataTls($issuer, $clientId, $clientSecret, $scope, $tls);
}

/**
 * `discoverMetadata` with TLS options, as `discoverTls`.
 * @param issuer {string} the issuer URL
 * @param clientId {string} the OAuth2 client identifier
 * @param clientSecret {string} the client secret ("" for a public client)
 * @param scope {string} the space-separated requested scopes
 * @param tls {http.TlsOptions} how the certificates are checked
 * @return {Discovery} the client settings (`tls` included), the UserInfo URL and the document
 * @throws {OAuthError} kind "oauth" as `discover`
 */
export func discoverMetadataTls(
    issuer as string,
    clientId as string,
    clientSecret as string,
    scope as string,
    tls as http.TlsOptions) {
    if (not endpointOk($issuer) or strings.contains($issuer, "?")) {
        fail("discover: the issuer must be an https URL without a query, got " + shown($issuer));
    }
    def found as bool init false;
    def body as string init "";
    def tried as list of string init [];
    for (def url in metadataUrls($issuer)) {
        def r as http.Response init fetch("GET", $url, {"Accept": "application/json"}, "", $tls);
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
    def cfg as Config init Config{
        tokenUrl: metadataEndpoint($node, "token_endpoint", true),
        deviceUrl: metadataEndpoint($node, "device_authorization_endpoint", false),
        authorizeUrl: metadataEndpoint($node, "authorization_endpoint", false),
        clientId: $clientId,
        clientSecret: $clientSecret,
        scope: $scope,
        tls: $tls
    };
    return Discovery{
        config: $cfg,
        userinfoUrl: metadataEndpoint($node, "userinfo_endpoint", false),
        metadata: $node
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
 * @throws {OAuthError} kind "oauth" on an empty client id or audience (jwt errors pass)
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
 * @throws {OAuthError} kind "oauth" on a token-endpoint error
 */
export func clientCredentialsJwt(config as Config, assertion as string) {
    def params as map of string to string init clientAssertionParams($config, $assertion);
    return tokenRequest($config.tokenUrl, $params, $config.tls);
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
    if ($scheme == "http") {
        # caught by awaitCode, which sees the decoded path: an escape there
        # would never match (and `%2F` has no single decoded form)
        return endpointOk($url) and not strings.contains($u.path, "%");
    }
    if ($scheme == "https") {
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
 * @throws {OAuthError} kind "oauth" if the config has no usable authorize URL or no client id, or the redirect URI is unusable
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
 * @throws {OAuthError} kind "oauth" as `authorize`, or if `extra` names a parameter `authorize` sets
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
        def description as string init "";
        if (maps.has($params, "error_description")) {
            $description = $params["error_description"];
            $why = $why + ": " + shown($description);
        }
        failWith("authorization refused: " + $why, $params["error"], $description, 0);
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
 * @throws {OAuthError} kind "oauth" on another redirect target, a state mismatch, a repeated parameter, a provider error (e.g. access_denied), or no code
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
 * @throws {OAuthError} kind "oauth" on a provider error (e.g. access_denied) or a redirect without a code
 */
export func awaitCode(req as AuthRequest, srv as httpd.Server) {
    def want as string init uri.parse($req.redirectUri).path;
    if (strings.contains($want, "%")) {
        fail("awaitCode: the redirect URI's path may not hold a %-escape, got " +
            shown($req.redirectUri));
    }
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
 * @throws {OAuthError} kind "oauth" on a token-endpoint error
 */
export func exchangeCode(config as Config, req as AuthRequest, code as string) {
    return exchangeCodeWith($config, $code, $req.verifier, $req.redirectUri);
}

/**
 * `exchangeCode` for a sign-in whose PKCE verifier and redirect were made
 * elsewhere: a server that trades the code its client caught, with the
 * client's verifier.
 * @param config {Config} the client settings (`tokenUrl`, `clientId`, optional `clientSecret`)
 * @param code {string} the authorization code
 * @param verifier {string} the PKCE code verifier (43 to 128 characters of `A-Z a-z 0-9 - . _ ~`, RFC 7636)
 * @param redirectUri {string} the redirect URI the sign-in used
 * @return {Token} the issued token
 * @throws {OAuthError} kind "oauth" on an empty code, a malformed verifier, an unusable redirect URI, or a token-endpoint error
 */
export func exchangeCodeWith(
    config as Config,
    code as string,
    verifier as string,
    redirectUri as string) {
    def params as map of string to string init codeParams($config, $code, $verifier, $redirectUri);
    return tokenRequest($config.tokenUrl, $params, $config.tls);
}

# codeParams builds the authorization-code token request, checking its parts.
func codeParams(config as Config, code as string, verifier as string, redirectUri as string) {
    if ($code == "") {
        fail("exchangeCode: the code is empty");
    }
    if (not regex.matches('^[A-Za-z0-9._~-]{43,128}$', $verifier)) {
        fail("exchangeCode: the code verifier must be 43 to 128 characters of A-Z a-z 0-9 - . _ ~");
    }
    if (not redirectOk($redirectUri)) {
        fail("exchangeCode: the redirect URI is not usable: " + shown($redirectUri));
    }
    def params as map of string to string init {
        "grant_type": "authorization_code",
        "code": $code,
        "redirect_uri": $redirectUri,
        "client_id": $config.clientId,
        "code_verifier": $verifier
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
 * (`0600`) file beside `path`, flushed to the disk, which then replaces
 * `path`: a crash or a power loss leaves the old file or the new one, and a
 * symbolic link at `path` is replaced, not followed. An
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
        writeSynced($tmp, json.encode($token));
        fs.rename($tmp, $path);
    } catch (e) {
        if (fs.exists($tmp)) {
            fs.remove($tmp);
        }
        throw $e;
    }
}

# writeSynced writes text to a file and flushes it to the disk before the
# rename in `save`, so a power loss leaves the old file or the new one, never
# an empty one.
func writeSynced(path as string, text as string) {
    def f as fs.File init fs.open($path, "write");
    defer fs.close($f);
    fs.writeString($f, $text);
    fs.sync($f);
}

/**
 * Read a token previously written by save. A file saved before 0.26.0 has no
 * `idToken` / `extra` and loads with them empty.
 * @param path {string} the file path to read
 * @return {Token} the loaded token
 * @throws {OAuthError} kind "oauth" if the file is not a saved token
 */
export func load(path as string) {
    def text as string init "";
    try {
        $text = fs.readString($path);
    } catch (e) {
        fail("cannot read the saved token file " + shown($path) + ": " + shown($e.message));
    }
    def node as json.Value;
    try {
        $node = json.decode($text);
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

# tokenFromFile reads the fields `save` writes; any missing or mistyped one, an
# empty access token or type, or a negative expiry is an error (load reports
# it).
func tokenFromFile(node as json.Value) {
    if (json.asString($node, "/accessToken") == "" or json.asString($node, "/tokenType") == "" or
        json.asInt($node, "/expiresAt") < 0) {
        fail("not a usable token");
    }
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
