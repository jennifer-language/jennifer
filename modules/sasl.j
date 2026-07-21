# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The SASL authentication mechanisms shared by the mail clients (`smtp` / `pop`
 * / `imap`). These format the client tokens; the protocol clients run the
 * mechanism-specific wire dialogue (SMTP `AUTH`, IMAP `AUTHENTICATE`, POP3
 * `AUTH`) around them. No networking; TinyGo-clean, so it runs on both binaries.
 *
 * - **Simple encoders** (`plain`, `loginUser` / `loginPass`, `bearer`) - one
 *   base64 token per step. `bearer` builds the SASL XOAUTH2 response from an
 *   OAuth2 bearer token (named `bearer`, not `xoauth2`, because a Jennifer
 *   method name is letters-only; "XOAUTH2" is the string the client sends).
 * - **Challenge-response** (`cram`, and the `SCRAM` family) - keyed by the
 *   `hash` / `crypto` libraries. `cram` (CRAM-MD5, RFC 2195) answers a server
 *   challenge in one step. SCRAM is a multi-step exchange over a `Scram` handle:
 *   `scramStart` ->
 *   `scramClientFirst` (send), receive server-first -> `scramClientFinal` +
 *   `scramFinalToken` (send), receive server-final -> `scramVerify` (checks the
 *   server signature, so a MITM without the password is caught). Mechanism
 *   "sha1" is SCRAM-SHA-1, "sha256" is SCRAM-SHA-256.
 * @module sasl
 * @example
 * import "sasl.j" as sasl;
 * def token as string init sasl.plain("me@example.com", "secret");
 * def xo as string init sasl.bearer("me@gmail.com", accessToken);
 */
use convert;
use encoding;
use strings;
use hash;
use crypto;

# baseEncode is base64 of a string's UTF-8 bytes.
func baseEncode(s as string) {
    return encoding.toText(convert.bytesFromString($s, "utf-8"), "base64");
}

# ctrlA returns the SASL control-A (0x01) separator. Jennifer has no `\x01`
# string escape, so it is built from a one-byte `bytes`.
func ctrlA() {
    def b as bytes;
    $b[] = 1;
    return convert.stringFromBytes($b, "utf-8");
}

/**
 * Build the SASL PLAIN response: base64 of "\0user\0pass".
 * @param user {string} the login username
 * @param pass {string} the login password
 * @return {string} the base64-encoded PLAIN token
 */
export func plain(user as string, pass as string) {
    return baseEncode("\0" + $user + "\0" + $pass);
}

/**
 * Build the username step of SASL LOGIN (a server sends the "Username:" prompt).
 * @param user {string} the login username
 * @return {string} the base64-encoded username
 */
export func loginUser(user as string) {
    return baseEncode($user);
}

/**
 * Build the password step of SASL LOGIN (a server sends the "Password:" prompt).
 * @param pass {string} the login password
 * @return {string} the base64-encoded password
 */
export func loginPass(pass as string) {
    return baseEncode($pass);
}

/**
 * Build the SASL XOAUTH2 response for an OAuth2 bearer token:
 * base64("user=" user 0x01 "auth=Bearer " token 0x01 0x01). This is how Google
 * and Microsoft 365 authenticate mail (both have retired password auth).
 * @param user {string} the account username
 * @param token {string} the OAuth2 bearer access token
 * @return {string} the base64-encoded XOAUTH2 response
 */
export func bearer(user as string, token as string) {
    def sep as string init ctrlA();
    def raw as string init "user=" + $user + $sep + "auth=Bearer " + $token + $sep + $sep;
    return baseEncode($raw);
}

# --- mechanism negotiation --------------------------------------------------

# hasMech reports whether `name` is among `mechs`, compared case-insensitively.
func hasMech(mechs as list of string, name as string) {
    for (def m in $mechs) {
        if (strings.upper($m) == $name) {
            return true;
        }
    }
    return false;
}

/**
 * Choose the strongest password mechanism this module implements from the list
 * a server advertises, returned as the `auth` token a mail client uses:
 * "scram-sha-256" > "scram-sha-1" > "cram", or "" when the server offers none
 * (the caller then falls back to its protocol default - PLAIN / LOGIN /
 * USER-PASS). XOAUTH2 is never auto-selected: it needs a bearer token, not a
 * password.
 * @param advertised {list of string} the mechanism names the server advertises (any case)
 * @return {string} the chosen auth token, or "" for the protocol default
 */
export func negotiate(advertised as list of string) {
    if (hasMech($advertised, "SCRAM-SHA-256")) {
        return "scram-sha-256";
    }
    if (hasMech($advertised, "SCRAM-SHA-1")) {
        return "scram-sha-1";
    }
    if (hasMech($advertised, "CRAM-MD5")) {
        return "cram";
    }
    return "";
}

# --- CRAM-MD5 ---------------------------------------------------------------

/**
 * Build the CRAM-MD5 response (RFC 2195): base64 of "user <hex>", where <hex>
 * is the lowercase HMAC-MD5 of the server's (base64-encoded) challenge keyed by
 * the password. A one-step challenge-response mechanism. Named `cram` (not
 * `cramMd5`) because a Jennifer method name is letters-only; the wire mechanism
 * is "CRAM-MD5".
 * @param user {string} the login username
 * @param pass {string} the login password
 * @param challenge {string} the server's base64-encoded challenge
 * @return {string} the base64-encoded CRAM-MD5 response
 */
export func cram(user as string, pass as string, challenge as string) {
    def raw as bytes init encoding.fromText($challenge, "base64");
    def mac as bytes init hash.hmac(convert.bytesFromString($pass, "utf-8"), $raw, "md5");
    return baseEncode($user + " " + encoding.toText($mac, "hex"));
}

# --- SCRAM (RFC 5802 / RFC 7677) --------------------------------------------

/**
 * The state of an in-progress SCRAM exchange, threaded through the four calls.
 * Value-semantic: each step returns an updated copy.
 * @field algo {string} the hash mechanism: "sha1" (SCRAM-SHA-1) or "sha256" (SCRAM-SHA-256)
 * @field clientNonce {string} the client-generated nonce
 * @field clientFirstBare {string} the client-first message without the gs2 header
 * @field clientFinal {string} the client-final message (set by scramClientFinal)
 * @field serverSig {string} the base64 ServerSignature to expect (set by scramClientFinal)
 */
export def struct Scram {
    algo as string,
    clientNonce as string,
    clientFirstBare as string,
    clientFinal as string,
    serverSig as string
};

# baseDecode decodes a base64 wire token back to its text.
func baseDecode(s as string) {
    return convert.stringFromBytes(encoding.fromText($s, "base64"), "utf-8");
}

# scramEscape encodes the two characters SCRAM reserves in a username: `=` and
# `,` become `=3D` and `=2C` (RFC 5802 saslname).
func scramEscape(s as string) {
    def out as string init strings.replace($s, "=", "=3D");
    return strings.replace($out, ",", "=2C");
}

# hmacStr is HMAC(key, message) over a string message, in the SCRAM hash.
func hmacStr(key as bytes, msg as string, algo as string) {
    return hash.hmac($key, convert.bytesFromString($msg, "utf-8"), $algo);
}

# xorBytes returns the byte-wise XOR of two equal-length byte strings.
func xorBytes(a as bytes, b as bytes) {
    def out as bytes;
    def i as int init 0;
    while ($i < len($a)) {
        $out[] = $a[$i] ^ $b[$i];
        $i = $i + 1;
    }
    return $out;
}

# scramAttr returns the value of attribute `key` in a comma-separated SCRAM
# message ("k=value,..."), or "" when absent.
func scramAttr(msg as string, key as string) {
    def prefix as string init $key + "=";
    for (def p in strings.split($msg, ",")) {
        if (strings.startsWith($p, $prefix)) {
            return strings.substring($p, len($prefix), len($p));
        }
    }
    return "";
}

# hashLen is the digest length in bytes for a SCRAM hash mechanism.
func hashLen(algo as string) {
    if ($algo == "sha256") {
        return 32;
    }
    if ($algo == "sha512") {
        return 64;
    }
    return 20;
}

# scramStartNonce builds the SCRAM state with an explicit client nonce; the
# public scramStart supplies a crypto-grade one. Split out so a white-box test
# can pin the RFC's nonce and check the exact wire bytes.
func scramStartNonce(user as string, algo as string, nonce as string) {
    def bare as string init "n=" + scramEscape($user) + ",r=" + $nonce;
    return Scram{ algo: $algo, clientNonce: $nonce, clientFirstBare: $bare,
        clientFinal: "", serverSig: "" };
}

/**
 * Begin a SCRAM exchange: generate a crypto-grade client nonce and build the
 * initial state. `algo` selects the mechanism ("sha1" or "sha256").
 * @param user {string} the login username
 * @param algo {string} "sha1" (SCRAM-SHA-1) or "sha256" (SCRAM-SHA-256)
 * @return {Scram} the exchange state; pass it to scramClientFirst
 */
export func scramStart(user as string, algo as string) {
    return scramStartNonce($user, $algo, encoding.toText(crypto.randBytes(18), "base64"));
}

/**
 * The base64 client-first message to send to the server (SASL initial response).
 * @param s {Scram} the exchange state from scramStart
 * @return {string} the base64-encoded client-first message
 */
export func scramClientFirst(s as Scram) {
    return baseEncode("n,," + $s.clientFirstBare);
}

/**
 * Process the server-first message and compute the client-final message. Derives
 * the salted password (PBKDF2 in the mechanism's hash), the client proof, and
 * the expected server signature; the returned state carries both. Throws
 * `Error{kind: "sasl"}` if the server's nonce does not extend the client nonce.
 * @param s {Scram} the exchange state
 * @param serverFirst {string} the base64 server-first message
 * @param password {string} the login password
 * @return {Scram} the updated state; pass it to scramFinalToken and scramVerify
 */
export func scramClientFinal(s as Scram, serverFirst as string, password as string) {
    def sf as string init baseDecode($serverFirst);
    def rnonce as string init scramAttr($sf, "r");
    if (not strings.startsWith($rnonce, $s.clientNonce)) {
        throw Error{ kind: "sasl", message: "sasl: server nonce does not extend the client nonce", file: "", line: 0, col: 0 };
    }
    def salt as bytes init encoding.fromText(scramAttr($sf, "s"), "base64");
    def iters as int init convert.toInt(scramAttr($sf, "i"));
    def salted as bytes init crypto.pbkdf(convert.bytesFromString($password, "utf-8"), $salt, $iters, hashLen($s.algo), $s.algo);
    def clientKey as bytes init hmacStr($salted, "Client Key", $s.algo);
    def storedKey as bytes init hash.compute($clientKey, $s.algo);
    def finalNoProof as string init "c=biws,r=" + $rnonce;
    def authMessage as string init $s.clientFirstBare + "," + $sf + "," + $finalNoProof;
    def clientSig as bytes init hmacStr($storedKey, $authMessage, $s.algo);
    def proof as bytes init xorBytes($clientKey, $clientSig);
    def serverKey as bytes init hmacStr($salted, "Server Key", $s.algo);
    def serverSig as bytes init hmacStr($serverKey, $authMessage, $s.algo);
    def out as Scram init $s;
    $out.clientFinal = $finalNoProof + ",p=" + encoding.toText($proof, "base64");
    $out.serverSig = encoding.toText($serverSig, "base64");
    return $out;
}

/**
 * The base64 client-final message to send (after scramClientFinal).
 * @param s {Scram} the state returned by scramClientFinal
 * @return {string} the base64-encoded client-final message
 */
export func scramFinalToken(s as Scram) {
    return baseEncode($s.clientFinal);
}

/**
 * Verify the server-final message: the server's signature must match the one
 * derived in scramClientFinal (constant-time), proving the server also knows the
 * password. Reject the session on false.
 * @param s {Scram} the state returned by scramClientFinal
 * @param serverFinal {string} the base64 server-final message
 * @return {bool} true if the server signature verifies
 */
export func scramVerify(s as Scram, serverFinal as string) {
    def v as string init scramAttr(baseDecode($serverFinal), "v");
    return crypto.hmacEqual(convert.bytesFromString($v, "utf-8"),
        convert.bytesFromString($s.serverSig, "utf-8"));
}
