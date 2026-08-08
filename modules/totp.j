# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Time-based one-time passwords (RFC 6238 TOTP over RFC 4226 HOTP) - the
 * six-digit two-factor codes authenticator apps show. A shared **secret** (a
 * base32 string, the same one an app stores) plus the current time yields a
 * short numeric code that both sides compute independently. Built on
 * `hash.hmac` (HMAC-SHA1 by default; SHA-256 / SHA-512 optional), `encoding`
 * (base32 secrets), and `time` (the 30-second step); the dynamic-truncation
 * step uses `bytes` + bitwise operators. Pure `.j`, runs on both binaries.
 *
 * `generate` / `verify` read the clock; `generateAt` / `verifyAt` take an
 * explicit Unix time (deterministic - use them in tests). `verify` accepts a
 * +/-1-step clock-skew window. `uri` builds the `otpauth://` provisioning
 * string an authenticator app scans as a QR code.
 * @module totp
 * @example
 * def o as totp.Options;                      # zero-value = 6 digits, 30 s, SHA-1
 * def code as string init totp.generate("JBSWY3DPEHPK3PXP", $o);
 * def ok as bool init totp.verify("JBSWY3DPEHPK3PXP", $code, $o);
 */
use hash;
use crypto;
use encoding;
use time;
use strings;
use convert;
import "./uri.j" as urilib;

/**
 * The HMAC digest algorithm behind a TOTP code: `Sha1` (the default), `Sha256`,
 * or `Sha512`. Selected through `Options.algorithm`; the zero value is `Sha1`.
 */
export def enum Algorithm { Sha1, Sha256, Sha512 };

/**
 * TOTP parameters. A zero-value struct (`def o as totp.Options;`) means the
 * common defaults: 6 digits, a 30-second step, and HMAC-SHA1.
 * @field digits {int} the code length; 0 means 6
 * @field period {int} the time step in seconds; 0 means 30
 * @field algorithm {Algorithm} the HMAC digest: `totp.Algorithm.Sha1` (the zero-value default), `.Sha256`, or `.Sha512`
 */
export def struct Options {
    digits as int,
    period as int,
    algorithm as Algorithm
};

# --- option resolution ------------------------------------------------------

func digitsOf(opts as Options) {
    if ($opts.digits > 0) {
        return $opts.digits;
    }
    return 6;
}

func periodOf(opts as Options) {
    if ($opts.period > 0) {
        return $opts.period;
    }
    return 30;
}

# algorithmOf returns the wire name ("sha1" / "sha256" / "sha512") of an Options'
# digest, for hash.hmac and the otpauth URL. The zero-value Options defaults to
# Sha1 (the enum's first variant), so no "" sentinel is needed. A Security-style
# `match` over the Algorithm parameter is exhaustiveness-checked.
func algorithmOf(opts as Options) {
    match ($opts.algorithm) {
        when Sha1 { return "sha1"; }
        when Sha256 { return "sha256"; }
        when Sha512 { return "sha512"; }
    }
}

# --- core HOTP --------------------------------------------------------------

# powTen returns 10^n (n small; a plain int loop avoids float rounding).
func powTen(n as int) {
    def r as int init 1;
    def i as int init 0;
    while ($i < $n) {
        $r = $r * 10;
        $i = $i + 1;
    }
    return $r;
}

# padCode left-pads a numeric code with zeros to the requested width.
func padCode(code as int, digits as int) {
    def s as string init convert.toString($code);
    while (len($s) < $digits) {
        $s = "0" + $s;
    }
    return $s;
}

# decodeSecret turns a base32 secret (as authenticator apps store it) into key
# bytes. Spaces are ignored, letters upper-cased, and `=` padding is added to a
# multiple of 8 characters so an app's unpadded secret still decodes.
func decodeSecret(secret as string) {
    def s as string init strings.upper(strings.replace($secret, " ", ""));
    while (not (len($s) % 8 == 0)) {
        $s = $s + "=";
    }
    return encoding.fromText($s, "base32");
}

# counterBytes encodes a counter as an 8-byte big-endian value (RFC 4226).
func counterBytes(counter as int) {
    def out as bytes;
    def shift as int init 56;
    while ($shift >= 0) {
        $out[] = ($counter >> $shift) & 0xFF;
        $shift = $shift - 8;
    }
    return $out;
}

# hotpKey is the RFC 4226 HMAC-based one-time password for a counter: HMAC the
# counter with the key, then dynamic-truncate to a `digits`-wide number.
func hotpKey(key as bytes, counter as int, digits as int, algorithm as string) {
    def mac as bytes init hash.hmac($key, counterBytes($counter), $algorithm);
    def offset as int init $mac[len($mac) - 1] & 0x0F;
    def bin as int init ((($mac[$offset] & 0x7F) << 24) | ($mac[$offset + 1] << 16) | ($mac[$offset +
        2] << 8) | $mac[$offset + 3]);
    return padCode($bin % powTen($digits), $digits);
}

/**
 * The raw RFC 4226 HOTP: the six-digit HMAC-SHA1 code for an explicit counter,
 * the counter-based building block TOTP layers a clock over. Use it directly
 * for HOTP (event-based OTP) or to pin against the RFC 4226 test vectors; TOTP
 * itself is just this over `unixSeconds // period`.
 * @param secret {string} the base32 shared secret
 * @param counter {int} the RFC 4226 moving-factor counter
 * @return {string} the six-digit zero-padded numeric code
 */
export func hotp(secret as string, counter as int) {
    return hotpKey(decodeSecret($secret), $counter, 6, "sha1");
}

# --- generate / verify ------------------------------------------------------

/**
 * Compute the TOTP code for an explicit Unix time (seconds). Deterministic.
 * @param secret {string} the base32 shared secret
 * @param unixSeconds {int} the Unix time in seconds
 * @param opts {Options} digits / period / algorithm (zero-value = defaults)
 * @return {string} the zero-padded numeric code
 */
export func generateAt(secret as string, unixSeconds as int, opts as Options) {
    def counter as int init $unixSeconds // periodOf($opts);
    return hotpKey(decodeSecret($secret), $counter, digitsOf($opts), algorithmOf($opts));
}

# --- secret generation ------------------------------------------------------

/**
 * Generate a fresh random shared secret of an explicit byte length, as a
 * base32 string (unpadded, the authenticator format `decodeSecret` accepts).
 * Randomness is crypto-grade (`crypto.randBytes`), so the secret is
 * unguessable. Round-trips: the result works with `generate` / `verify`.
 * @param nbytes {int} the number of random bytes (RFC 6238 recommends >= 20 for SHA-1)
 * @return {string} the base32-encoded secret
 */
export func generateSecretN(nbytes as int) {
    if ($nbytes <= 0) {
        throw Error{
            kind: "totp",
            message: "totp: secret byte length must be positive",
            file: "",
            line: 0,
            col: 0
        };
    }
    def raw as bytes init crypto.randBytes($nbytes);
    return strings.replace(encoding.toText($raw, "base32"), "=", "");
}

/**
 * Generate a fresh random shared secret with the RFC 6238-recommended length
 * for SHA-1 TOTP (20 bytes / 160 bits), as an unpadded base32 string.
 * @return {string} the base32-encoded secret
 */
export func generateSecret() {
    return generateSecretN(20);
}

/**
 * Compute the TOTP code for the current time.
 * @param secret {string} the base32 shared secret
 * @param opts {Options} digits / period / algorithm (zero-value = defaults)
 * @return {string} the zero-padded numeric code
 */
export func generate(secret as string, opts as Options) {
    return generateAt($secret, time.unix(time.now()), $opts);
}

/**
 * Verify a code against an explicit Unix time, allowing a `window`-step skew on
 * each side of now (so `window` steps of clock drift in either direction still
 * pass). `window` 0 checks only the current step; `window` 1 is the +/-1-step
 * default `verifyAt` uses. Deterministic.
 * @param secret {string} the base32 shared secret
 * @param code {string} the code to check
 * @param unixSeconds {int} the Unix time in seconds
 * @param window {int} the number of period-steps to accept on each side of now
 * @param opts {Options} digits / period / algorithm (zero-value = defaults)
 * @return {bool} true if the code matches any step within the window
 */
export func verifyWindowAt(
    secret as string,
    code as string,
    unixSeconds as int,
    window as int,
    opts as Options) {
    def key as bytes init decodeSecret($secret);
    def digits as int init digitsOf($opts);
    def algorithm as string init algorithmOf($opts);
    def counter as int init $unixSeconds // periodOf($opts);
    def codeBytes as bytes init convert.bytesFromString($code, "utf-8");
    # Compare every window with a constant-time check and without an early
    # return: a plain `==` leaks, through response timing, how many leading
    # digits matched (and which window), which a network attacker can exploit
    # against a 2FA endpoint. crypto.hmacEqual runs in time independent of the
    # contents.
    def matched as bool init false;
    def step as int init -$window;
    while ($step <= $window) {
        def computed as bytes init convert.bytesFromString(
            hotpKey($key, $counter + $step, $digits, $algorithm),
            "utf-8");
        if (crypto.hmacEqual($computed, $codeBytes)) {
            $matched = true;
        }
        $step = $step + 1;
    }
    return $matched;
}

/**
 * Verify a code against the current time, allowing a `window`-step skew on each
 * side of now.
 * @param secret {string} the base32 shared secret
 * @param code {string} the code to check
 * @param window {int} the number of period-steps to accept on each side of now
 * @param opts {Options} digits / period / algorithm (zero-value = defaults)
 * @return {bool} true if the code matches any step within the window
 */
export func verifyWindow(secret as string, code as string, window as int, opts as Options) {
    return verifyWindowAt($secret, $code, time.unix(time.now()), $window, $opts);
}

/**
 * Verify a code against an explicit Unix time, allowing a +/-1-step skew (so a
 * code from the previous or next window still passes). Deterministic.
 * @param secret {string} the base32 shared secret
 * @param code {string} the code to check
 * @param unixSeconds {int} the Unix time in seconds
 * @param opts {Options} digits / period / algorithm (zero-value = defaults)
 * @return {bool} true if the code matches this step or an adjacent one
 */
export func verifyAt(secret as string, code as string, unixSeconds as int, opts as Options) {
    return verifyWindowAt($secret, $code, $unixSeconds, 1, $opts);
}

/**
 * Verify a code against the current time, allowing a +/-1-step skew.
 * @param secret {string} the base32 shared secret
 * @param code {string} the code to check
 * @param opts {Options} digits / period / algorithm (zero-value = defaults)
 * @return {bool} true if the code matches
 */
export func verify(secret as string, code as string, opts as Options) {
    return verifyAt($secret, $code, time.unix(time.now()), $opts);
}

# --- provisioning URI -------------------------------------------------------

# normalizedSecret canonicalizes a secret for the otpauth URI: strip spaces,
# upper-case, and drop `=` padding, so a secret an authenticator would accept
# (via decodeSecret) produces a scannable, matching URI.
func normalizedSecret(secret as string) {
    def s as string init strings.upper(strings.replace($secret, " ", ""));
    return strings.replace($s, "=", "");
}

/**
 * Build the `otpauth://totp/...` provisioning URI an authenticator app scans
 * (as a QR code) to enrol an account. The label is `issuer:account`.
 * @param issuer {string} the service name (e.g. "ACME")
 * @param account {string} the account name (e.g. "jane@acme.example")
 * @param secret {string} the base32 shared secret
 * @param opts {Options} digits / period / algorithm (zero-value = defaults)
 * @return {string} the otpauth provisioning URI
 */
export func uri(issuer as string, account as string, secret as string, opts as Options) {
    def label as string init urilib.encode($issuer) + ":" + urilib.encode($account);
    def query as string init "secret=" + normalizedSecret($secret) +
        "&issuer=" + urilib.encode($issuer) +
        "&algorithm=" + strings.upper(algorithmOf($opts)) +
        "&digits=" + convert.toString(digitsOf($opts)) +
        "&period=" + convert.toString(periodOf($opts));
    return "otpauth://totp/" + $label + "?" + $query;
}
