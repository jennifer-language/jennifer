# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.27.0

/**
 * Internationalized domain names: convert a Unicode domain to its
 * ASCII-compatible (`xn--`) form and back, over a Punycode (RFC 3492) core. So
 * `münchen.de` goes on the wire as `xn--mnchen-3ya.de` (DNS, SMTP envelopes,
 * URL hosts are ASCII-only). Pure Jennifer over `strings`, `convert`, and
 * `encoding` - no networking, TinyGo-clean. This is the Punycode transformation
 * plus lowercasing, not full IDNA2008 (which layers nameprep / mapping tables
 * on top); it covers the common cases. It does reject what would make one name
 * look like or alias another: control, space and invisible characters, U-label
 * hyphen rules, and an `xn--` label that is not a canonical A-label of a
 * non-ASCII label. It needs `convert.toCodepoint` /
 * `convert.fromCodepoint` for the bootstring arithmetic on rune values.
 * @module idna
 * @example
 * use io;
 * import "idna.j" as idna;
 * io.printf("%s\n", idna.toAscii("münchen.de"));      # xn--mnchen-3ya.de
 * io.printf("%s\n", idna.toUnicode("xn--mnchen-3ya.de"));   # münchen.de
 */
use strings;
use convert;
use encoding;
use lists;

# RFC 3492 bootstring parameters for Punycode.
def const BASE as int init 36;
def const TMIN as int init 1;
def const TMAX as int init 26;
def const SKEW as int init 38;
def const DAMP as int init 700;
def const INITIAL_BIAS as int init 72;
def const INITIAL_N as int init 128;
# Largest int64, for the RFC 3492 section 6.4 overflow guards in the decoder: the
# accumulators are checked against this *before* each multiply, since the multiply
# itself would otherwise be a positioned int-overflow runtime error.
def const MAXINT as int init 9223372036854775807;

# --- small helpers (private) ---------------------------------------

func isAsciiStr(s as string) {
    return encoding.isAscii(encoding.encode("utf-8", $s));
}

func codePoints(s as string) {
    def out as list of int init [];
    for (def c in strings.chars($s)) {
        $out[] = convert.toCodepoint($c);
    }
    return $out;
}

func fromCodePoints(cps as list of int) {
    def out as list of string init [];
    for (def c in $cps) {
        $out[] = convert.fromCodepoint($c);
    }
    return strings.join($out, "");
}

# digitToChar maps a base-36 digit value to its Punycode character (0-25 to
# a-z, 26-35 to 0-9).
func digitToChar(d as int) {
    if ($d < 26) {
        return convert.fromCodepoint(97 + $d);
    }
    return convert.fromCodepoint(48 + $d - 26);
}

# charToDigit maps a Punycode character back to its digit value, or -1.
func charToDigit(c as string) {
    def code as int init convert.toCodepoint($c);
    if ($code >= 97 and $code <= 122) {
        return $code - 97;
    }
    if ($code >= 65 and $code <= 90) {
        return $code - 65;
    }
    if ($code >= 48 and $code <= 57) {
        return 26 + $code - 48;
    }
    return -1;
}

# threshold is the RFC 3492 per-position digit threshold `t`.
func threshold(k as int, bias as int) {
    if ($k <= $bias) {
        return TMIN;
    }
    if ($k >= $bias + TMAX) {
        return TMAX;
    }
    return $k - $bias;
}

# adapt is the RFC 3492 bias-adaptation function.
func adapt(delta as int, numpoints as int, firsttime as bool) {
    def d as int init $delta // 2;
    if ($firsttime) {
        $d = $delta // DAMP;
    }
    $d = $d + $d // $numpoints;
    def k as int init 0;
    while ($d > ((BASE - TMIN) * TMAX) // 2) {
        $d = $d // (BASE - TMIN);
        $k = $k + BASE;
    }
    return $k + (BASE - TMIN + 1) * $d // ($d + SKEW);
}

# --- Punycode encode / decode (private) ----------------------------

# encodeDigits emits the variable-length base-36 digits for one delta value.
func encodeDigits(delta as int, bias as int) {
    def out as string init "";
    def q as int init $delta;
    def k as int init BASE;
    while (true) {
        def t as int init threshold($k, $bias);
        if ($q < $t) {
            return $out + digitToChar($q);
        }
        $out = $out + digitToChar($t + (($q - $t) % (BASE - $t)));
        $q = ($q - $t) // (BASE - $t);
        $k = $k + BASE;
    }
    return $out;
}

# minGE returns the smallest code point in cps that is >= n (or a large
# sentinel if none).
func minGE(cps as list of int, n as int) {
    def m as int init 1114112;
    for (def c in $cps) {
        if ($c >= $n and $c < $m) {
            $m = $c;
        }
    }
    return $m;
}

# encodeLabel Punycode-encodes a label's code points (no `xn--` prefix).
# Output pieces collect and join once (per-append `+` is O(N^2) over the label).
func encodeLabel(cps as list of int) {
    def n as int init INITIAL_N;
    def delta as int init 0;
    def bias as int init INITIAL_BIAS;
    def out as list of string init [];
    def b as int init 0;
    for (def c in $cps) {
        if ($c < 128) {
            $out[] = convert.fromCodepoint($c);
            $b = $b + 1;
        }
    }
    def h as int init $b;
    if ($b > 0) {
        $out[] = "-";
    }
    while ($h < len($cps)) {
        def m as int init minGE($cps, $n);
        $delta = $delta + ($m - $n) * ($h + 1);
        $n = $m;
        for (def c in $cps) {
            if ($c < $n) {
                $delta = $delta + 1;
            }
            if ($c == $n) {
                $out[] = encodeDigits($delta, $bias);
                $bias = adapt($delta, $h + 1, $h == $b);
                $delta = 0;
                $h = $h + 1;
            }
        }
        $delta = $delta + 1;
        $n = $n + 1;
    }
    return strings.join($out, "");
}

# insertAt returns lst with val inserted at index pos.
func insertAt(lst as list of int, pos as int, val as int) {
    def out as list of int init [];
    def j as int init 0;
    for (def x in $lst) {
        if ($j == $pos) {
            $out[] = $val;
        }
        $out[] = $x;
        $j = $j + 1;
    }
    if ($pos >= len($lst)) {
        $out[] = $val;
    }
    return $out;
}

# decodeStep consumes the base-36 digits at cursor `ic`, returning the new
# integer `i` and cursor packed as a 2-element list [i, ic].
func decodeStep(cs as list of string, ic as int, i as int, bias as int) {
    def cur as int init $ic;
    def acc as int init $i;
    def w as int init 1;
    def k as int init BASE;
    def total as int init len($cs);
    while (true) {
        # A malformed ACE label can run the cursor off the end or carry a
        # character that is not a valid base-36 digit; reject both instead of
        # indexing out of bounds or accumulating a -1 digit.
        if ($cur >= $total) {
            throw Error{kind: "idna", message: "idna: truncated punycode label"};
        }
        def digit as int init charToDigit($cs[$cur]);
        if ($digit < 0) {
            throw Error{kind: "idna", message: "idna: invalid punycode digit '" + $cs[$cur] + "'"};
        }
        $cur = $cur + 1;
        # RFC 3492 section 6.4 overflow check: reject before `acc + digit * w` can
        # overflow int64 (which would surface as kind "runtime"). acc, digit and w
        # are all non-negative here.
        if ($w > 0 and $digit > (MAXINT - $acc) // $w) {
            throw Error{kind: "idna", message: "idna: punycode value overflow"};
        }
        $acc = $acc + $digit * $w;
        def t as int init threshold($k, $bias);
        if ($digit < $t) {
            def res as list of int init [];
            $res[] = $acc;
            $res[] = $cur;
            return $res;
        }
        def factor as int init BASE - $t;
        if ($factor > 0 and $w > MAXINT // $factor) {
            throw Error{kind: "idna", message: "idna: punycode value overflow"};
        }
        $w = $w * $factor;
        $k = $k + BASE;
    }
    return [];
}

# decodeLabel Punycode-decodes an ACE label (the part after `xn--`).
func decodeLabel(ace as string) {
    def output as list of int init [];
    def n as int init INITIAL_N;
    def i as int init 0;
    def bias as int init INITIAL_BIAS;
    def cs as list of string init strings.chars($ace);
    def total as int init len($cs);
    def basic as int init strings.lastIndexOf($ace, "-");
    def start as int init 0;
    if ($basic >= 0) {
        $output = codePoints($ace[..$basic]);
        $start = $basic + 1;
    }
    def ic as int init $start;
    while ($ic < $total) {
        def oldi as int init $i;
        def step as list of int init decodeStep($cs, $ic, $i, $bias);
        $i = $step[0];
        $ic = $step[1];
        def outLen as int init len($output) + 1;
        $bias = adapt($i - $oldi, $outLen, $oldi == 0);
        $n = $n + $i // $outLen;
        # A hostile label can drive n past the Unicode range; reject it before
        # it reaches fromCodepoint (a spoofing / garbage-output guard).
        if ($n > 1114111) {
            throw Error{kind: "idna", message: "idna: decoded code point out of range"};
        }
        $i = $i % $outLen;
        $output = insertAt($output, $i, $n);
        $i = $i + 1;
    }
    return fromCodePoints($output);
}

# --- domain conversion (exported) ----------------------------------

# isInvisible reports a code point that no label may carry: C0 / C1 controls,
# DEL, the space characters, and the zero-width, soft-hyphen, bidi and other
# format characters a spoofed name hides in (IDNA2008 DISALLOWED / CONTEXTJ;
# ZWNJ / ZWJ are rejected outright, the CONTEXTJ rules are not implemented).
func isInvisible(c as int) {
    if ($c <= 0x20 or ($c >= 0x7F and $c <= 0xA0) or $c == 0xAD) {
        return true;
    }
    if (lists.contains([0x034F, 0x061C, 0x115F, 0x1160, 0x1680, 0x180E], $c)) {
        return true;
    }
    if (($c >= 0x2000 and $c <= 0x200F) or ($c >= 0x2028 and $c <= 0x202F) or
        ($c >= 0x205F and $c <= 0x206F)) {
        return true;
    }
    if ($c == 0x3000 or $c == 0x3164 or ($c >= 0xFE00 and $c <= 0xFE0F) or $c == 0xFEFF or
        $c == 0xFFA0) {
        return true;
    }
    return ($c >= 0xFFF0 and $c <= 0xFFFF) or ($c >= 0xE0000 and $c <= 0xE0FFF);
}

# checkAsciiLabel rejects an all-ASCII label carrying a control character, DEL
# or a space. Other ASCII (`_dmarc`, `*`, an IPv6 host's `:`) passes, since
# DNS labels and the hosts callers hand in are not limited to letters, digits
# and hyphens.
func checkAsciiLabel(label as string) {
    for (def c in codePoints($label)) {
        if (isInvisible($c)) {
            throw Error{kind: "idna", message: "idna: label contains a control or space character"};
        }
    }
}

# checkULabel validates a U-label's code points (RFC 5891 sections 4.2.3,
# 5.4): no control, space or invisible format character, its ASCII part only
# letters, digits and hyphens, no leading or trailing hyphen, no `--` in
# positions 3-4, and no uppercase (a U-label is the lowercased form).
func checkULabel(cps as list of int) {
    def n as int init len($cps);
    if ($n == 0) {
        throw Error{kind: "idna", message: "idna: empty U-label"};
    }
    for (def c in $cps) {
        if (isInvisible($c)) {
            throw Error{
                kind: "idna",
                message: "idna: label contains a control, space or invisible character"
            };
        }
        if ($c < 128 and not (($c >= 97 and $c <= 122) or ($c >= 48 and $c <= 57) or $c == 45)) {
            throw Error{
                kind: "idna",
                message: "idna: U-label contains ASCII other than letters, digits and hyphens"
            };
        }
    }
    if ($cps[0] == 45 or $cps[$n - 1] == 45) {
        throw Error{kind: "idna", message: "idna: label begins or ends with a hyphen"};
    }
    if ($n >= 4 and $cps[2] == 45 and $cps[3] == 45) {
        throw Error{kind: "idna", message: "idna: U-label has hyphens in positions 3 and 4"};
    }
}

# aLabelToULabel decodes an `xn--` label (already lowercased) and checks it is
# a real A-label (RFC 5891 section 5.4): it decodes to a valid U-label with at
# least one non-ASCII code point, already lowercase, which encodes back to the
# same A-label. So `xn--abc-` (decodes to plain `abc`) and `xn--` (empty) are
# errors rather than aliases of an ASCII name.
func aLabelToULabel(low as string) {
    # An A-label is at most 63 octets (DNS); the decoder is quadratic in its
    # length, so a longer one is rejected rather than decoded.
    if (len($low) > 63) {
        throw Error{kind: "idna", message: "idna: A-label exceeds 63 octets"};
    }
    def ace as string init $low[4..];
    def ulabel as string init decodeLabel($ace);
    if (isAsciiStr($ulabel)) {
        throw Error{
            kind: "idna",
            message: "idna: A-label '" + $low + "' does not decode to a non-ASCII label"
        };
    }
    def cps as list of int init codePoints($ulabel);
    checkULabel($cps);
    if (strings.lower($ulabel) != $ulabel or encodeLabel($cps) != $ace) {
        throw Error{kind: "idna", message: "idna: A-label '" + $low + "' is not in canonical form"};
    }
    return $ulabel;
}

# labelToAscii returns a domain label's A-label: an all-ASCII label is
# lowercased and kept (an `xn--` one must be a valid A-label); a Unicode label
# is validated and Punycode-encoded with the `xn--` prefix.
func labelToAscii(label as string) {
    def lower as string init strings.lower($label);
    def alabel as string init $lower;
    if (isAsciiStr($lower)) {
        checkAsciiLabel($lower);
        if (strings.startsWith($lower, "xn--")) {
            aLabelToULabel($lower);
        }
    } else {
        # Every code point costs at least one A-label octet after the 4-octet
        # prefix, so more than 59 cannot fit 63 octets. Rejecting first also keeps
        # the quadratic encoder off an attacker-sized label.
        def cps as list of int init codePoints($lower);
        if (len($cps) > 59) {
            throw Error{kind: "idna", message: "idna: label too long for a 63-octet A-label"};
        }
        checkULabel($cps);
        $alabel = "xn--" + encodeLabel($cps);
    }
    # DNS labels are at most 63 octets; a longer A-label is invalid.
    if (len($alabel) > 63) {
        throw Error{kind: "idna", message: "idna: A-label exceeds 63 octets: " + $alabel};
    }
    return $alabel;
}

# normalizeDots maps the three Unicode full-stop variants (ideographic U+3002,
# fullwidth U+FF0E, halfwidth U+FF61) to an ASCII dot, so a domain written with
# them splits into labels correctly.
func normalizeDots(s as string) {
    def out as string init strings.replace($s, convert.fromCodepoint(0x3002), ".");
    $out = strings.replace($out, convert.fromCodepoint(0xFF0E), ".");
    return strings.replace($out, convert.fromCodepoint(0xFF61), ".");
}

# labelToUnicode reverses labelToAscii: an `xn--` label is Punycode-decoded,
# anything else is returned unchanged.
func labelToUnicode(label as string) {
    def low as string init strings.lower($label);
    if (strings.startsWith($low, "xn--")) {
        return aLabelToULabel($low);
    }
    if (isAsciiStr($label)) {
        checkAsciiLabel($label);
    } else {
        checkULabel(codePoints($low));
    }
    return $label;
}

/**
 * Convert a domain to its ASCII-compatible form, label by label.
 * @param domain {string} the (possibly Unicode) domain name
 * @return {string} the ASCII-compatible domain (Unicode labels get an `xn--` prefix)
 * @throws {Error} kind "idna" when a label exceeds 63 octets or the domain 253,
 * carries a control, space or invisible character, a Unicode label breaks the
 * U-label rules, or an `xn--` label is not a valid A-label
 */
export func toAscii(domain as string) {
    def out as list of string init [];
    for (def label in strings.split(normalizeDots($domain), ".")) {
        $out[] = labelToAscii($label);
    }
    def ascii as string init strings.join($out, ".");
    # A DNS name is at most 253 octets (255 on the wire, less the root and the
    # first length byte), not counting a trailing dot.
    def body as string init $ascii;
    if (strings.endsWith($body, ".")) {
        $body = $body[..len($body) - 1];
    }
    if (len($body) > 253) {
        throw Error{kind: "idna", message: "idna: domain exceeds 253 octets"};
    }
    return $ascii;
}

/**
 * Convert an ASCII-compatible domain back to Unicode, label by label.
 * @param domain {string} the ASCII-compatible domain name
 * @return {string} the Unicode domain (`xn--` labels get Punycode-decoded)
 * @throws {Error} kind "idna" on a malformed, non-canonical or over-long (> 63
 * octets) `xn--` label, one decoding to pure ASCII or nothing, or a label with
 * a control, space or invisible character
 */
export func toUnicode(domain as string) {
    def out as list of string init [];
    for (def label in strings.split($domain, ".")) {
        $out[] = labelToUnicode($label);
    }
    return strings.join($out, ".");
}

/**
 * Report whether a domain is already all-ASCII (needs no conversion).
 * @param domain {string} the domain name to test
 * @return {bool} true when every character is ASCII
 */
export func isAscii(domain as string) {
    return isAsciiStr($domain);
}
