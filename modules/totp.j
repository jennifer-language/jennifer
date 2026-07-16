# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

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
use encoding;
use time;
use strings;
use convert;

/**
 * TOTP parameters. A zero-value struct (`def o as totp.Options;`) means the
 * common defaults: 6 digits, a 30-second step, and HMAC-SHA1.
 * @field digits {int} the code length; 0 means 6
 * @field period {int} the time step in seconds; 0 means 30
 * @field algorithm {string} the HMAC digest: "sha1" (default), "sha256", or "sha512"; "" means "sha1"
 */
export def struct Options {
    digits as int,
    period as int,
    algorithm as string
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

func algorithmOf(opts as Options) {
    if (not ($opts.algorithm == "")) {
        return $opts.algorithm;
    }
    return "sha1";
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

# hotp is the RFC 4226 HMAC-based one-time password for a counter: HMAC the
# counter with the key, then dynamic-truncate to a `digits`-wide number.
func hotp(key as bytes, counter as int, digits as int, algorithm as string) {
    def mac as bytes init hash.hmac($key, counterBytes($counter), $algorithm);
    def offset as int init $mac[len($mac) - 1] & 0x0F;
    def bin as int init ((($mac[$offset] & 0x7F) << 24) | ($mac[$offset + 1] << 16) | ($mac[$offset + 2] << 8) | $mac[$offset + 3]);
    return padCode($bin % powTen($digits), $digits);
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
    return hotp(decodeSecret($secret), $counter, digitsOf($opts), algorithmOf($opts));
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
 * Verify a code against an explicit Unix time, allowing a +/-1-step skew (so a
 * code from the previous or next window still passes). Deterministic.
 * @param secret {string} the base32 shared secret
 * @param code {string} the code to check
 * @param unixSeconds {int} the Unix time in seconds
 * @param opts {Options} digits / period / algorithm (zero-value = defaults)
 * @return {bool} true if the code matches this step or an adjacent one
 */
export func verifyAt(secret as string, code as string, unixSeconds as int, opts as Options) {
    def key as bytes init decodeSecret($secret);
    def digits as int init digitsOf($opts);
    def algorithm as string init algorithmOf($opts);
    def counter as int init $unixSeconds // periodOf($opts);
    def step as int init -1;
    while ($step <= 1) {
        if (hotp($key, $counter + $step, $digits, $algorithm) == $code) {
            return true;
        }
        $step = $step + 1;
    }
    return false;
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

# hexByte renders one byte as two uppercase hex digits.
func hexByte(b as int) {
    def digits as string init "0123456789ABCDEF";
    return strings.substring($digits, $b // 16, $b // 16 + 1) +
        strings.substring($digits, $b % 16, $b % 16 + 1);
}

# isUnreserved reports whether a byte is an RFC 3986 unreserved character.
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
    return ($b == 45 or $b == 46 or $b == 95 or $b == 126);
}

# urlEncode percent-encodes a string for use in a URI component.
func urlEncode(s as string) {
    def raw as bytes init convert.bytesFromString($s, "utf-8");
    def out as string init "";
    def i as int init 0;
    while ($i < len($raw)) {
        def b as int init $raw[$i];
        if (isUnreserved($b)) {
            $out = $out + convert.fromCodepoint($b);
        } else {
            $out = $out + "%" + hexByte($b);
        }
        $i = $i + 1;
    }
    return $out;
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
# normalizedSecret canonicalizes a secret for the otpauth URI: strip spaces,
# upper-case, and drop `=` padding, so a secret an authenticator would accept
# (via decodeSecret) produces a scannable, matching URI.
func normalizedSecret(secret as string) {
    def s as string init strings.upper(strings.replace($secret, " ", ""));
    return strings.replace($s, "=", "");
}

export func uri(issuer as string, account as string, secret as string, opts as Options) {
    def label as string init urlEncode($issuer) + ":" + urlEncode($account);
    def query as string init "secret=" + normalizedSecret($secret) +
        "&issuer=" + urlEncode($issuer) +
        "&algorithm=" + strings.upper(algorithmOf($opts)) +
        "&digits=" + convert.toString(digitsOf($opts)) +
        "&period=" + convert.toString(periodOf($opts));
    return "otpauth://totp/" + $label + "?" + $query;
}
