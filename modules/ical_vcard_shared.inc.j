# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.25.0
#
# ical_vcard_shared.inc.j - the shared "content line" codec for the text formats that
# descend from the vCard / iCalendar line grammar (RFC 5545 iCalendar, RFC 6350
# vCard): TEXT escaping (`\` `;` `,` and newline), 75-character line folding, and
# the name / value split. This file is spliced into ical.j and vcard.j via
# `include` and is not a standalone module: it declares no `use` of its own and
# relies on the including module's `use strings;` and `use convert;`.

# --- text escaping ----------------------------------------------------------

# escapeText applies the RFC 5545 / 6350 TEXT escaping: backslash first, then the
# structural `;` / `,`, then any line break to a literal `\n`.
func escapeText(v as string) {
    def s as string init strings.replace($v, "\\", "\\\\");
    $s = strings.replace($s, ";", "\\;");
    $s = strings.replace($s, ",", "\\,");
    $s = strings.replace($s, "\r\n", "\\n");
    $s = strings.replace($s, "\r", "\\n");
    $s = strings.replace($s, "\n", "\\n");
    return $s;
}

# unescapeText reverses escapeText with a single left-to-right scan, so an
# escaped backslash never re-triggers on the following character.
func unescapeText(v as string) {
    # Characters accumulate in a list and join once, so unescaping a long value
    # is linear - a growing `+` would re-copy the whole result per character.
    def out as list of string init [];
    def chars as list of string init strings.chars($v);
    def n as int init len($chars);
    def i as int init 0;
    while ($i < $n) {
        def c as string init $chars[$i];
        if ($c == "\\" and $i + 1 < $n) {
            def nx as string init $chars[$i + 1];
            if ($nx == "n" or $nx == "N") {
                $out[] = "\n";
            } elseif ($nx == "\\") {
                $out[] = "\\";
            } elseif ($nx == ";") {
                $out[] = ";";
            } elseif ($nx == ",") {
                $out[] = ",";
            } else {
                $out[] = $nx;
            }
            $i = $i + 2;
            continue;
        }
        $out[] = $c;
        $i = $i + 1;
    }
    return strings.join($out, "");
}

# --- line folding -----------------------------------------------------------

# fold breaks a content line longer than 75 OCTETS into CRLF + space
# continuations (RFC 5545 3.1 / RFC 6350 3.2). The limit is bytes, not runes -
# a rune-counted fold produces physical lines up to ~4x the octet limit that
# strict validators reject - and folds only on rune boundaries so a multi-byte
# character is never split. A continuation line's leading space counts toward
# its 75 octets, so it carries 74 content octets.
func fold(line as string) {
    if (len(convert.bytesFromString($line, "utf-8")) <= 75) {
        return $line;
    }
    # Each physical line's characters collect in a list joined once (a growing
    # `+` is quadratic on a long line - a vCard PHOTO is hundreds of KB), and a
    # character's UTF-8 length comes from its codepoint.
    def parts as list of string init [];
    def cur as list of string init [];
    def curBytes as int init 0;
    def limit as int init 75;
    for (def ch in strings.chars($line)) {
        def cp as int init convert.toCodepoint($ch);
        def chBytes as int init 4;
        if ($cp < 0x80) {
            $chBytes = 1;
        } elseif ($cp < 0x800) {
            $chBytes = 2;
        } elseif ($cp < 0x10000) {
            $chBytes = 3;
        }
        if ($curBytes + $chBytes > $limit) {
            $parts[] = strings.join($cur, "");
            $cur = [];
            $curBytes = 0;
            $limit = 74; # continuation lines: 1 octet is the leading space
        }
        $cur[] = $ch;
        $curBytes = $curBytes + $chBytes;
    }
    $parts[] = strings.join($cur, "");
    return strings.join($parts, "\r\n ");
}

# unfold removes line folds: a line break followed by a space or tab rejoins the
# continuation, for both CRLF and bare-LF input.
func unfold(text as string) {
    def s as string init strings.replace($text, "\r\n ", "");
    $s = strings.replace($s, "\r\n\t", "");
    $s = strings.replace($s, "\n ", "");
    $s = strings.replace($s, "\n\t", "");
    return $s;
}

# --- name / value split -----------------------------------------------------

# splitLines normalises CRLF / CR to LF and splits into physical lines.
func splitLines(text as string) {
    def s as string init strings.replace($text, "\r\n", "\n");
    $s = strings.replace($s, "\r", "\n");
    return strings.split($s, "\n");
}

# propName returns the upper-cased property name (the part before the first `;`
# parameter or the `:` value separator) of a content line's name section.
func propName(nameSection as string) {
    def semi as int init strings.indexOf($nameSection, ";");
    if ($semi >= 0) {
        return strings.upper($nameSection[..$semi]);
    }
    return strings.upper($nameSection);
}

# quoteParam encodes a parameter value. A double quote or a line break cannot
# appear in one at all (it would end the quoted string, or the line, and let the
# value inject parameters or properties), so they are caret-encoded per RFC 6868
# (`^` -> `^^`, line break -> `^n`, `"` -> `^'`); then a value holding `;`, `:` or
# `,` is wrapped in double quotes. `paramValue` reverses both.
func quoteParam(v as string) {
    def s as string init strings.replace($v, "^", "^^");
    $s = strings.replace($s, "\r\n", "^n");
    $s = strings.replace($s, "\r", "^n");
    $s = strings.replace($s, "\n", "^n");
    $s = strings.replace($s, "\"", "^'");
    if (strings.contains($s, ";") or strings.contains($s, ":") or strings.contains($s, ",")) {
        return "\"" + $s + "\"";
    }
    return $s;
}

# caretDecode reverses RFC 6868 parameter-value encoding: `^^` -> `^`, `^n` -> a
# newline, `^'` -> `"`; any other `^` is kept as is.
func caretDecode(v as string) {
    if (not strings.contains($v, "^")) {
        return $v;
    }
    def out as list of string init [];
    def cs as list of string init strings.chars($v);
    def n as int init len($cs);
    def i as int init 0;
    while ($i < $n) {
        if ($cs[$i] == "^" and $i + 1 < $n) {
            def nx as string init $cs[$i + 1];
            if ($nx == "^") {
                $out[] = "^";
                $i = $i + 2;
                continue;
            }
            if ($nx == "n") {
                $out[] = "\n";
                $i = $i + 2;
                continue;
            }
            if ($nx == "'") {
                $out[] = "\"";
                $i = $i + 2;
                continue;
            }
        }
        $out[] = $cs[$i];
        $i = $i + 1;
    }
    return strings.join($out, "");
}

# paramValue returns the value of the named parameter (case-insensitive name) in
# a content line's name section (`EMAIL;TYPE=work;PREF=1` -> "work" for "TYPE"),
# or "" when absent. A `;`-separated parameter list is walked honouring quotes; a
# quoted value has its surrounding quotes stripped, then RFC 6868 caret escapes
# are decoded. The name section is the
# part before the value colon.
func paramValue(nameSection as string, param as string) {
    def want as string init strings.upper($param);
    def cs as list of string init strings.chars($nameSection);
    def n as int init len($cs);
    # Skip the property name (up to the first ';').
    def i as int init 0;
    while ($i < $n and not ($cs[$i] == ";")) {
        $i = $i + 1;
    }
    while ($i < $n) {
        $i = $i + 1; # step past the ';'
        def keyStart as int init $i;
        while ($i < $n and not ($cs[$i] == "=") and not ($cs[$i] == ";")) {
            $i = $i + 1;
        }
        def key as string init strings.upper(strings.join($cs[$keyStart..$i], ""));
        def val as string init "";
        if ($i < $n and $cs[$i] == "=") {
            $i = $i + 1; # step past the '='
            def inQuote as bool init false;
            def vs as int init $i;
            while ($i < $n and (not ($cs[$i] == ";") or $inQuote)) {
                if ($cs[$i] == "\"") {
                    $inQuote = not $inQuote;
                }
                $i = $i + 1;
            }
            $val = strings.join($cs[$vs..$i], "");
            if (len($val) >= 2 and strings.startsWith($val, "\"") and strings.endsWith($val, "\"")) {
                $val = $val[1..len($val) - 1];
            }
        }
        if ($key == $want) {
            return caretDecode($val);
        }
    }
    return "";
}

# emitLine returns a folded `NAME:VALUE` content line. Callers append it
# directly (`$lines[] = emitLine(...)`), which mutates their own list in place -
# `emit`'s `lists.push` copied the whole growing line list on every call
# (O(L^2) over a large calendar / contact).
func emitLine(name as string, value as string) {
    # Strip CR / LF from the name section (which carries the parameters) and the
    # value: a bare CR / LF would end the line and inject a forged property into
    # the .ics / .vcf stream. A text value that needs a newline carries the `\n`
    # escape (escapeText), a parameter the `^n` one (quoteParam), so a legitimate
    # line never holds a bare CR / LF - only an injection attempt does.
    def line as string init strings.replace($name + ":" + $value, "\r", "");
    $line = strings.replace($line, "\n", "");
    return fold($line);
}
