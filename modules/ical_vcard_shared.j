# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
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
    def out as string init "";
    def chars as list of string init strings.chars($v);
    def n as int init len($chars);
    def i as int init 0;
    while ($i < $n) {
        def c as string init $chars[$i];
        if ($c == "\\" and $i + 1 < $n) {
            def nx as string init $chars[$i + 1];
            if ($nx == "n" or $nx == "N") {
                $out = $out + "\n";
            } elseif ($nx == "\\") {
                $out = $out + "\\";
            } elseif ($nx == ";") {
                $out = $out + ";";
            } elseif ($nx == ",") {
                $out = $out + ",";
            } else {
                $out = $out + $nx;
            }
            $i = $i + 2;
            continue;
        }
        $out = $out + $c;
        $i = $i + 1;
    }
    return $out;
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
            $limit = 74;   # continuation lines: 1 octet is the leading space
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

# emitLine returns a folded `NAME:VALUE` content line. Callers append it
# directly (`$lines[] = emitLine(...)`), which mutates their own list in place -
# `emit`'s `lists.push` copied the whole growing line list on every call
# (O(L^2) over a large calendar / contact).
func emitLine(name as string, value as string) {
    return fold($name + ":" + $value);
}
