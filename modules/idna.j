# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

/**
 * Internationalized domain names: convert a Unicode domain to its
 * ASCII-compatible (`xn--`) form and back, over a Punycode (RFC 3492) core. So
 * `münchen.de` goes on the wire as `xn--mnchen-3ya.de` (DNS, SMTP envelopes,
 * URL hosts are ASCII-only). Pure Jennifer over `strings`, `convert`, and
 * `encoding` - no networking, TinyGo-clean. This is the Punycode transformation
 * plus lowercasing, not full IDNA2008 (which layers nameprep / mapping tables
 * on top); it covers the common cases. It needs `convert.toCodepoint` /
 * `convert.fromCodepoint` for the bootstring arithmetic on rune values.
 * @module idna
 * @example
 * import "idna.j" as idna;
 * io.printf("%s\n", idna.toAscii("münchen.de"));      # xn--mnchen-3ya.de
 * io.printf("%s\n", idna.toUnicode("xn--mnchen-3ya.de"));   # münchen.de
 */
use strings;
use convert;
use encoding;

# RFC 3492 bootstring parameters for Punycode.
def const BASE as int init 36;
def const TMIN as int init 1;
def const TMAX as int init 26;
def const SKEW as int init 38;
def const DAMP as int init 700;
def const INITIAL_BIAS as int init 72;
def const INITIAL_N as int init 128;

# --- small helpers (private) ---------------------------------------

func isAsciiStr(s as string) {
    return encoding.isAscii(convert.bytesFromString($s, "utf-8"));
}

func codePoints(s as string) {
    def out as list of int init [];
    for (def c in strings.chars($s)) {
        $out[] = convert.toCodepoint($c);
    }
    return $out;
}

func fromCodePoints(cps as list of int) {
    def out as string init "";
    for (def c in $cps) {
        $out = $out + convert.fromCodepoint($c);
    }
    return $out;
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
func encodeLabel(cps as list of int) {
    def n as int init INITIAL_N;
    def delta as int init 0;
    def bias as int init INITIAL_BIAS;
    def out as string init "";
    def b as int init 0;
    for (def c in $cps) {
        if ($c < 128) {
            $out = $out + convert.fromCodepoint($c);
            $b = $b + 1;
        }
    }
    def h as int init $b;
    if ($b > 0) {
        $out = $out + "-";
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
                $out = $out + encodeDigits($delta, $bias);
                $bias = adapt($delta, $h + 1, $h == $b);
                $delta = 0;
                $h = $h + 1;
            }
        }
        $delta = $delta + 1;
        $n = $n + 1;
    }
    return $out;
}

# lastDash returns the index of the last "-" in cs, or -1.
func lastDash(cs as list of string) {
    def idx as int init -1;
    def j as int init 0;
    for (def c in $cs) {
        if ($c == "-") {
            $idx = $j;
        }
        $j = $j + 1;
    }
    return $idx;
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
            throw Error{kind: "idna", message: "idna: truncated punycode label", file: "", line: 0, col: 0};
        }
        def digit as int init charToDigit($cs[$cur]);
        if ($digit < 0) {
            throw Error{kind: "idna", message: "idna: invalid punycode digit '" + $cs[$cur] + "'", file: "", line: 0, col: 0};
        }
        $cur = $cur + 1;
        $acc = $acc + $digit * $w;
        def t as int init threshold($k, $bias);
        if ($digit < $t) {
            def res as list of int init [];
            $res[] = $acc;
            $res[] = $cur;
            return $res;
        }
        $w = $w * (BASE - $t);
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
    def basic as int init lastDash($cs);
    def start as int init 0;
    if ($basic >= 0) {
        def j as int init 0;
        while ($j < $basic) {
            $output[] = convert.toCodepoint($cs[$j]);
            $j = $j + 1;
        }
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
            throw Error{kind: "idna", message: "idna: decoded code point out of range", file: "", line: 0, col: 0};
        }
        $i = $i % $outLen;
        $output = insertAt($output, $i, $n);
        $i = $i + 1;
    }
    return fromCodePoints($output);
}

# --- domain conversion (exported) ----------------------------------

# labelToAscii returns a domain label's A-label: an all-ASCII label is
# lowercased and kept; a Unicode label is Punycode-encoded with the `xn--`
# prefix.
func labelToAscii(label as string) {
    def lower as string init strings.lower($label);
    if (isAsciiStr($lower)) {
        return $lower;
    }
    return "xn--" + encodeLabel(codePoints($lower));
}

# labelToUnicode reverses labelToAscii: an `xn--` label is Punycode-decoded,
# anything else is returned unchanged.
func labelToUnicode(label as string) {
    def low as string init strings.lower($label);
    if (strings.startsWith($low, "xn--")) {
        return decodeLabel(strings.substring($low, 4, len($low)));
    }
    return $label;
}

/**
 * Convert a domain to its ASCII-compatible form, label by label.
 * @param domain {string} the (possibly Unicode) domain name
 * @return {string} the ASCII-compatible domain (Unicode labels get an `xn--` prefix)
 */
export func toAscii(domain as string) {
    def out as list of string init [];
    for (def label in strings.split($domain, ".")) {
        $out[] = labelToAscii($label);
    }
    return strings.join($out, ".");
}

/**
 * Convert an ASCII-compatible domain back to Unicode, label by label.
 * @param domain {string} the ASCII-compatible domain name
 * @return {string} the Unicode domain (`xn--` labels get Punycode-decoded)
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
