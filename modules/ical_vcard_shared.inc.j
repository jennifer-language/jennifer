# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
# pragma-jennifer-version: >=0.25.0
#
# ical_vcard_shared.j - the shared "content line" codec for the text formats that
# descend from the vCard / iCalendar line grammar (RFC 5545 iCalendar, RFC 6350
# vCard): TEXT escaping (`\` `;` `,` and newline), 75-character line folding, and
# the name / value split. This file is spliced into ical.j and vcard.j via
# `include` and is not a standalone module: it declares no `use` of its own and
# relies on the including module's `use strings;` and `use lists;`.

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
    def parts as list of string init [];
    def cur as string init "";
    def curBytes as int init 0;
    def limit as int init 75;
    for (def ch in strings.chars($line)) {
        def chBytes as int init len(convert.bytesFromString($ch, "utf-8"));
        if ($curBytes + $chBytes > $limit) {
            $parts[] = $cur;
            $cur = "";
            $curBytes = 0;
            $limit = 74; # continuation lines: 1 octet is the leading space
        }
        $cur = $cur + $ch;
        $curBytes = $curBytes + $chBytes;
    }
    $parts[] = $cur;
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
        return strings.upper(strings.substring($nameSection, 0, $semi));
    }
    return strings.upper($nameSection);
}

# quoteParam wraps a parameter value in double quotes when it contains a `;`,
# `:`, or `,` - the characters that would otherwise be read as a parameter /
# value separator (RFC 5545 / 6350 require a quoted-string for these). A safe
# token is returned unchanged. `paramValue` strips the quotes back off on parse.
func quoteParam(v as string) {
    if (strings.contains($v, ";") or strings.contains($v, ":") or strings.contains($v, ",")) {
        return "\"" + $v + "\"";
    }
    return $v;
}

# paramValue returns the value of the named parameter (case-insensitive name) in
# a content line's name section (`EMAIL;TYPE=work;PREF=1` -> "work" for "TYPE"),
# or "" when absent. A `;`-separated parameter list is walked honouring quotes,
# and a quoted value has its surrounding quotes stripped. The name section is the
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
        def key as string init strings.upper(strings.join(lists.slice($cs, $keyStart, $i), ""));
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
            $val = strings.join(lists.slice($cs, $vs, $i), "");
            if (len($val) >= 2 and strings.startsWith($val, "\"") and strings.endsWith($val, "\"")) {
                $val = strings.substring($val, 1, len($val) - 1);
            }
        }
        if ($key == $want) {
            return $val;
        }
    }
    return "";
}

# emitLine returns a folded `NAME:VALUE` content line. Callers append it
# directly (`$lines[] = emitLine(...)`), which mutates their own list in place -
# `emit`'s `lists.push` copied the whole growing line list on every call
# (O(L^2) over a large calendar / contact).
func emitLine(name as string, value as string) {
    # Strip CR / LF from the value: a bare CR / LF would terminate the folded line
    # and inject a forged property into the .ics / .vcf stream. A text value that
    # needs a literal newline is represented as the `\n` escape via escapeText, so a
    # legitimate value never carries a bare CR / LF - only an injection attempt does.
    def v as string init strings.replace($value, "\r", "");
    $v = strings.replace($v, "\n", "");
    return fold($name + ":" + $v);
}
