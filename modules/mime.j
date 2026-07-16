# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

/**
 * Build and parse MIME messages (RFC 5322 headers + RFC 2045/2046 bodies): a
 * header-and-boundary structure that is exactly the text orchestration a `.j`
 * module does well. The transfer codecs (base64, quoted-printable) are delegated
 * to the `encoding` system library. This is the message-structure foundation the
 * mail clients (SMTP / POP3 / IMAP) build on; it does no networking itself. A
 * `Part` is a leaf (headers + a decoded-text `body` with a transfer `encoding`)
 * or a multipart container (headers + child `parts` + a `boundary`); bodies are
 * held decoded as text, encode applies the transfer encoding and parse removes
 * it. RFC 2047 encoded-words are applied automatically on encode and decoded on
 * parse, with `encodeWord` / `decodeWord` exposed for manual use. Content is
 * text (UTF-8); binary attachments (a `bytes` body) are not yet handled.
 * @module mime
 * @example
 * import "mime.j" as mime;
 * def msg as mime.Part init mime.text("text/plain", "Hello, monde: café.");
 * $msg = mime.withHeader($msg, "Subject", "Hi");
 * io.printf("%s", mime.encode($msg));
 * def back as mime.Part init mime.parse(mime.encode($msg));
 */

use strings;
use convert;
use encoding;
use regex;

/**
 * A single header field.
 * @field name {string} the field name (e.g. "Subject")
 * @field value {string} the field value
 */
export def struct Header {
    name as string,
    value as string
};

/**
 * A MIME part: a leaf (`body` + `encoding`, `parts` empty) or a multipart
 * container (`parts` + `boundary`, `body` empty).
 * @field headers {list of Header} the part's header fields
 * @field body {string} the decoded text body of a leaf part
 * @field encoding {string} the transfer encoding ("7bit", "base64", "quoted-printable")
 * @field parts {list of Part} the child parts of a multipart container
 * @field boundary {string} the multipart boundary delimiter
 */
export def struct Part {
    headers as list of Header,
    body as string,
    encoding as string,
    parts as list of Part,
    boundary as string
};

# --- small text helpers (private) ----------------------------------

func mkHeader(name as string, value as string) {
    return Header{name: $name, value: $value};
}

# crlf normalises any line endings to CRLF (canonical for a MIME message).
func crlf(s as string) {
    def out as string init strings.replace($s, "\r\n", "\n");
    return strings.replace($out, "\n", "\r\n");
}

# stripCR drops a single trailing CR (a line split off a CRLF stream).
func stripCR(line as string) {
    if (strings.endsWith($line, "\r")) {
        return strings.substring($line, 0, len($line) - 1);
    }
    return $line;
}

# stripWS removes all spaces and line breaks (base64 tolerates none on decode).
func stripWS(s as string) {
    def out as string init strings.replace($s, "\r", "");
    $out = strings.replace($out, "\n", "");
    return strings.replace($out, " ", "");
}

# wrapLines folds a long unbroken string (base64) into 76-column CRLF lines.
# The input is ASCII base64, so slicing by rune index equals slicing by column;
# collecting the 76-char lines and joining once keeps a large attachment linear
# (a per-character `+` was O(N^2)).
func wrapLines(s as string) {
    def n as int init len($s);
    def lines as list of string init [];
    def i as int init 0;
    while ($i < $n) {
        def end as int init $i + 76;
        if ($end > $n) {
            $end = $n;
        }
        $lines[] = strings.substring($s, $i, $end);
        $i = $end;
    }
    return strings.join($lines, "\r\n");
}

# --- transfer encoding (private) -----------------------------------

func encodeBody(body as string, enc as string) {
    def b as bytes init convert.bytesFromString($body, "utf-8");
    if ($enc == "base64") {
        return wrapLines(encoding.toText($b, "base64"));
    }
    if ($enc == "quoted-printable") {
        return crlf(encoding.toText($b, "quoted-printable"));
    }
    return crlf($body);
}

func decodeBody(raw as string, enc as string) {
    if ($enc == "base64") {
        return convert.stringFromBytes(encoding.fromText(stripWS($raw), "base64"), "utf-8");
    }
    if ($enc == "quoted-printable") {
        return convert.stringFromBytes(encoding.fromText($raw, "quoted-printable"), "utf-8");
    }
    return $raw;
}

# --- header lookup (private) ---------------------------------------

# findHeader returns the value of the named header (case-insensitive) or "".
func findHeader(headers as list of Header, name as string) {
    def lname as string init strings.lower($name);
    for (def h in $headers) {
        if (strings.lower($h.name) == $lname) {
            return $h.value;
        }
    }
    return "";
}

# setHeaderIn returns the header list with `name` set (replaced if present,
# else appended).
func setHeaderIn(headers as list of Header, name as string, value as string) {
    def out as list of Header init [];
    def lname as string init strings.lower($name);
    def found as bool init false;
    for (def h in $headers) {
        if (strings.lower($h.name) == $lname) {
            $out[] = mkHeader($h.name, $value);
            $found = true;
        } else {
            $out[] = $h;
        }
    }
    if (not $found) {
        $out[] = mkHeader($name, $value);
    }
    return $out;
}

# --- content-type parsing (private) --------------------------------

# typeOnly is the media type without parameters ("text/plain" from
# "text/plain; charset=utf-8").
func typeOnly(ct as string) {
    def semi as int init strings.indexOf($ct, ";");
    if ($semi >= 0) {
        return strings.trim(strings.substring($ct, 0, $semi));
    }
    return strings.trim($ct);
}

# extractBoundary reads the `boundary=` parameter of a Content-Type, quoted or
# bare, or "" when absent.
func extractBoundary(ct as string) {
    def idx as int init strings.indexOf(strings.lower($ct), "boundary=");
    if ($idx < 0) {
        return "";
    }
    def rest as string init strings.trim(strings.substring($ct, $idx + 9, len($ct)));
    if (strings.startsWith($rest, "\"")) {
        def tail as string init strings.substring($rest, 1, len($rest));
        def end as int init strings.indexOf($tail, "\"");
        if ($end >= 0) {
            return strings.substring($tail, 0, $end);
        }
        return "";
    }
    def semi as int init strings.indexOf($rest, ";");
    if ($semi >= 0) {
        return strings.trim(strings.substring($rest, 0, $semi));
    }
    return $rest;
}

# --- header parsing (private) --------------------------------------

# splitHeaderLine parses one unfolded `Name: value` line.
func splitHeaderLine(line as string) {
    def idx as int init strings.indexOf($line, ":");
    if ($idx < 0) {
        return mkHeader(strings.trim($line), "");
    }
    def name as string init strings.trim(strings.substring($line, 0, $idx));
    def value as string init strings.trim(strings.substring($line, $idx + 1, len($line)));
    return mkHeader($name, $value);
}

# parseHeaders parses a header block, unfolding continuation lines (those that
# begin with whitespace) into the field above.
func parseHeaders(text as string) {
    def hs as list of Header init [];
    def cur as string init "";
    for (def line in strings.split($text, "\n")) {
        def l as string init stripCR($line);
        def folds as bool init strings.startsWith($l, " ") or strings.startsWith($l, "\t");
        if (len($l) > 0 and $folds) {
            $cur = $cur + " " + strings.trim($l);
        } else {
            if (len($cur) > 0) {
                $hs[] = splitHeaderLine($cur);
            }
            $cur = $l;
        }
    }
    if (len($cur) > 0) {
        $hs[] = splitHeaderLine($cur);
    }
    return $hs;
}

# parseMultipart splits a multipart body on its boundary and parses each
# body-part; the preamble before the first boundary and epilogue after the
# close delimiter are dropped.
func parseMultipart(body as string, boundary as string) {
    def delim as string init "--" + $boundary;
    def close as string init $delim + "--";
    def parts as list of Part init [];
    def cur as list of string init [];
    def collecting as bool init false;
    for (def line in strings.split($body, "\n")) {
        def t as string init stripCR($line);
        if ($t == $delim or $t == $close) {
            if ($collecting) {
                $parts[] = parse(strings.join($cur, "\n"));
            }
            $cur = [];
            $collecting = not ($t == $close);
        } else {
            if ($collecting) {
                $cur[] = $line;
            }
        }
    }
    return $parts;
}

# --- RFC 2047 encoded-words (exported + private) -------------------

# isAsciiText reports whether `s` is pure ASCII (no encoded-word needed).
func isAsciiText(s as string) {
    return encoding.isAscii(convert.bytesFromString($s, "utf-8"));
}

# isBlank reports whether `s` is empty or only linear whitespace.
func isBlank(s as string) {
    for (def ch in strings.chars($s)) {
        if (not ($ch == " " or $ch == "\t" or $ch == "\r" or $ch == "\n")) {
            return false;
        }
    }
    return true;
}

# encodeWordChunk wraps one rune-safe slice as a single UTF-8 B encoded-word.
func encodeWordChunk(s as string) {
    def b as bytes init convert.bytesFromString($s, "utf-8");
    return "=?UTF-8?B?" + encoding.toText($b, "base64") + "?=";
}

/**
 * Render `text` as one or more RFC 2047 UTF-8 base64 encoded-words, each kept
 * under the 75-character limit (split on rune boundaries, never mid-character)
 * and folded with CRLF + space when more than one is needed.
 * @param text {string} the text to encode
 * @return {string} the encoded-word sequence
 */
export func encodeWord(text as string) {
    def words as list of string init [];
    def chunk as string init "";
    def chunkBytes as int init 0;
    for (def ch in strings.chars($text)) {
        def w as int init len(convert.bytesFromString($ch, "utf-8"));
        if ($chunkBytes + $w > 45 and $chunkBytes > 0) {
            $words[] = encodeWordChunk($chunk);
            $chunk = "";
            $chunkBytes = 0;
        }
        $chunk = $chunk + $ch;
        $chunkBytes = $chunkBytes + $w;
    }
    $words[] = encodeWordChunk($chunk);
    return strings.join($words, "\r\n ");
}

# hexDigit maps one hex character to its value (0..15), or -1.
func hexDigit(c as string) {
    return strings.indexOf("0123456789abcdef", strings.lower($c));
}

# decodeQ decodes RFC 2047 "Q" text ("_"=space, "=XX"=byte) to bytes.
func decodeQ(text as string) {
    def out as bytes;
    def cs as list of string init strings.chars($text);
    def n as int init len($cs);
    def i as int init 0;
    while ($i < $n) {
        def ch as string init $cs[$i];
        if ($ch == "_") {
            $out[] = 32;
            $i = $i + 1;
        } elseif ($ch == "=" and $i + 2 < $n) {
            $out[] = hexDigit($cs[$i + 1]) * 16 + hexDigit($cs[$i + 2]);
            $i = $i + 3;
        } else {
            $out[] = convert.toCodepoint($ch);
            $i = $i + 1;
        }
    }
    return $out;
}

# decodeCharset turns decoded bytes into a string per the encoded-word charset.
func decodeCharset(raw as bytes, charset as string) {
    if ($charset == "utf-8" or $charset == "utf8") {
        return convert.stringFromBytes($raw, "utf-8");
    }
    if ($charset == "us-ascii" or $charset == "ascii") {
        return encoding.decode($raw, "ascii");
    }
    return encoding.decode($raw, $charset);
}

# decodeOneWord decodes a single encoded-word's (charset, encoding, text) triple.
func decodeOneWord(charset as string, enc as string, text as string) {
    def up as string init strings.upper($enc);
    def raw as bytes;
    if ($up == "B") {
        $raw = encoding.fromText(stripWS($text), "base64");
    } else {
        $raw = decodeQ($text);
    }
    return decodeCharset($raw, strings.lower($charset));
}

/**
 * Decode every RFC 2047 encoded-word in `value`, dropping the linear whitespace
 * that separates two adjacent encoded-words (as a reader should). A word that
 * fails to decode is left verbatim so parse never crashes.
 * @param value {string} the header value possibly carrying encoded-words
 * @return {string} the decoded text
 */
export func decodeWord(value as string) {
    def pat as string init "=\\?([^?]+)\\?([BbQq])\\?([^?]*)\\?=";
    def ms as list of regex.Match init regex.findAll($pat, $value);
    if (len($ms) == 0) {
        return $value;
    }
    def out as string init "";
    def prevEnd as int init 0;
    def prevWord as bool init false;
    for (def m in $ms) {
        def between as string init strings.substring($value, $prevEnd, $m.start);
        if (not ($prevWord and isBlank($between))) {
            $out = $out + $between;
        }
        def decoded as string init strings.substring($value, $m.start, $m.end);
        try {
            $decoded = decodeOneWord($m.groups[0], $m.groups[1], $m.groups[2]);
        } catch (e) {
            $decoded = strings.substring($value, $m.start, $m.end);
        }
        $out = $out + $decoded;
        $prevEnd = $m.end;
        $prevWord = true;
    }
    return $out + strings.substring($value, $prevEnd, len($value));
}

# isEncodedWordHeader reports whether a header carries encoded-words (decode on
# parse) - the unstructured fields and the display-name of address fields.
func isEncodedWordHeader(name as string) {
    def n as string init strings.lower($name);
    if ($n == "subject" or $n == "comments") {
        return true;
    }
    return isAddressHeader($n);
}

# isAddressHeader reports whether a header holds mailbox addresses.
func isAddressHeader(name as string) {
    def n as string init strings.lower($name);
    if ($n == "from" or $n == "to" or $n == "cc") {
        return true;
    }
    return $n == "bcc" or $n == "reply-to" or $n == "sender";
}

# autoDecodeHeaders decodes encoded-words in the header fields that may carry
# them, leaving structured fields (Message-ID, Date, Content-*) untouched.
func autoDecodeHeaders(headers as list of Header) {
    def out as list of Header init [];
    for (def h in $headers) {
        if (isEncodedWordHeader($h.name)) {
            $out[] = mkHeader($h.name, decodeWord($h.value));
        } else {
            $out[] = $h;
        }
    }
    return $out;
}

# unquote strips a surrounding quoted-string and unescapes it.
func unquote(s as string) {
    if (len($s) >= 2 and strings.startsWith($s, "\"") and strings.endsWith($s, "\"")) {
        def inner as string init strings.substring($s, 1, len($s) - 1);
        return strings.replace($inner, "\\\"", "\"");
    }
    return $s;
}

# splitTopLevelCommas splits an address-list value on commas that are outside a
# quoted display name (a comma inside `"Last, First"` is not a separator).
func splitTopLevelCommas(s as string) {
    def out as list of string init [];
    def cur as string init "";
    def cs as list of string init strings.chars($s);
    def i as int init 0;
    def inQuote as bool init false;
    while ($i < len($cs)) {
        def c as string init $cs[$i];
        if ($c == "\"") {
            $inQuote = not $inQuote;
            $cur = $cur + $c;
        } elseif ($c == "," and not $inQuote) {
            $out[] = $cur;
            $cur = "";
        } else {
            $cur = $cur + $c;
        }
        $i = $i + 1;
    }
    $out[] = $cur;
    return $out;
}

# encodeOneAddress encodes a non-ASCII display name in a single `Name <addr>`
# mailbox (or a bare non-ASCII phrase), leaving the address alone.
func encodeOneAddress(value as string) {
    def lt as int init strings.indexOf($value, "<");
    if ($lt < 0) {
        if (isAsciiText($value)) {
            return $value;
        }
        return encodeWord(unquote($value));
    }
    def phrase as string init strings.trim(strings.substring($value, 0, $lt));
    if (isAsciiText($phrase)) {
        return $value;
    }
    def addr as string init strings.substring($value, $lt, len($value));
    return encodeWord(unquote($phrase)) + " " + $addr;
}

# encodeAddressHeader encodes each mailbox of an address-list header, so an
# international display name survives even in a multi-address To / Cc / From
# (splitting on top-level commas rather than bailing out on the first comma,
# which used to serialize the whole value as raw 8-bit).
func encodeAddressHeader(value as string) {
    def encoded as list of string init [];
    for (def part in splitTopLevelCommas($value)) {
        $encoded[] = encodeOneAddress(strings.trim($part));
    }
    return strings.join($encoded, ", ");
}

# encodeHeaderValue applies encoded-word encoding to a non-ASCII header value
# that may carry one; ASCII values and structured fields pass through unchanged.
func encodeHeaderValue(name as string, value as string) {
    if (isAsciiText($value)) {
        return $value;
    }
    def n as string init strings.lower($name);
    if ($n == "subject" or $n == "comments") {
        return encodeWord($value);
    }
    if (isAddressHeader($n)) {
        return encodeAddressHeader($value);
    }
    return $value;
}

# --- building (exported) -------------------------------------------

/**
 * Build a leaf text part; the body is sent 7bit when ASCII, else
 * quoted-printable. `charset=utf-8` is appended to the content type.
 * @param contentType {string} the media type (e.g. "text/plain")
 * @param body {string} the decoded text body
 * @return {Part} the leaf text part
 */
export func text(contentType as string, body as string) {
    def enc as string init "7bit";
    if (not encoding.isAscii(convert.bytesFromString($body, "utf-8"))) {
        $enc = "quoted-printable";
    }
    def hs as list of Header init [];
    $hs[] = mkHeader("Content-Type", $contentType + "; charset=utf-8");
    $hs[] = mkHeader("Content-Transfer-Encoding", $enc);
    return Part{headers: $hs, body: $body, encoding: $enc, parts: [], boundary: ""};
}

/**
 * Build a base64 leaf part with a filename. `body` is text content (UTF-8);
 * binary bodies are not yet supported.
 * @param filename {string} the attachment filename
 * @param contentType {string} the media type
 * @param body {string} the text content to attach
 * @return {Part} the attachment part
 */
export func attachment(filename as string, contentType as string, body as string) {
    def hs as list of Header init [];
    $hs[] = mkHeader("Content-Type", $contentType);
    $hs[] = mkHeader("Content-Transfer-Encoding", "base64");
    $hs[] = mkHeader("Content-Disposition", "attachment; filename=\"" + $filename + "\"");
    return Part{headers: $hs, body: $body, encoding: "base64", parts: [], boundary: ""};
}

/**
 * Build a container part; every child ships under one `boundary`.
 * @param subtype {string} the multipart subtype (e.g. "mixed", "alternative")
 * @param boundary {string} the boundary delimiter separating children
 * @param parts {list of Part} the child parts
 * @return {Part} the multipart container part
 */
export func multipart(subtype as string, boundary as string, parts as list of Part) {
    def hs as list of Header init [];
    $hs[] = mkHeader("Content-Type", "multipart/" + $subtype + "; boundary=\"" + $boundary + "\"");
    return Part{headers: $hs, body: "", encoding: "", parts: $parts, boundary: $boundary};
}

/**
 * Return a copy of the part with `name` set (replaced or appended).
 * @param part {Part} the part to copy
 * @param name {string} the header name to set
 * @param value {string} the header value
 * @return {Part} the updated copy
 */
export func withHeader(part as Part, name as string, value as string) {
    def np as Part init $part;
    $np.headers = setHeaderIn($part.headers, $name, $value);
    return $np;
}

# --- encoding (exported) -------------------------------------------

func encodeMultipart(p as Part) {
    def out as string init "";
    for (def sub in $p.parts) {
        $out = $out + "--" + $p.boundary + "\r\n" + encode($sub) + "\r\n";
    }
    return $out + "--" + $p.boundary + "--\r\n";
}

/**
 * Serialize a part (and its subtree) to a MIME message string with CRLF line
 * endings and the declared transfer encodings applied.
 * @param part {Part} the part to serialize
 * @return {string} the encoded MIME message
 */
export func encode(part as Part) {
    def out as string init "";
    for (def h in $part.headers) {
        $out = $out + $h.name + ": " + encodeHeaderValue($h.name, $h.value) + "\r\n";
    }
    $out = $out + "\r\n";
    if (len($part.boundary) > 0) {
        return $out + encodeMultipart($part);
    }
    return $out + encodeBody($part.body, $part.encoding);
}

# --- parsing (exported) --------------------------------------------

/**
 * Read a MIME message string into a Part tree: headers are unfolded, a multipart
 * body is split on its boundary (recursively), and a leaf body is
 * transfer-decoded.
 * @param text {string} the MIME message text
 * @return {Part} the parsed part tree
 */
export func parse(text as string) {
    def norm as string init strings.replace($text, "\r\n", "\n");
    def idx as int init strings.indexOf($norm, "\n\n");
    def headerText as string init $norm;
    def bodyText as string init "";
    if ($idx >= 0) {
        $headerText = strings.substring($norm, 0, $idx);
        $bodyText = strings.substring($norm, $idx + 2, len($norm));
    }
    def hs as list of Header init autoDecodeHeaders(parseHeaders($headerText));
    def boundary as string init extractBoundary(findHeader($hs, "Content-Type"));
    if (len($boundary) > 0) {
        def kids as list of Part init parseMultipart($bodyText, $boundary);
        return Part{headers: $hs, body: "", encoding: "", parts: $kids, boundary: $boundary};
    }
    def enc as string init findHeader($hs, "Content-Transfer-Encoding");
    if (len($enc) == 0) {
        $enc = "7bit";
    }
    return Part{headers: $hs, body: decodeBody($bodyText, $enc), encoding: $enc,
        parts: [], boundary: ""};
}

# --- accessors + helpers (exported) --------------------------------

/**
 * Return a part's header value (case-insensitive) or "".
 * @param part {Part} the part to read
 * @param name {string} the header name
 * @return {string} the header value, or "" when absent
 */
export func headerValue(part as Part, name as string) {
    return findHeader($part.headers, $name);
}

/**
 * Return a leaf part's decoded text body.
 * @param part {Part} the leaf part
 * @return {string} the decoded body text
 */
export func body(part as Part) {
    return $part.body;
}

/**
 * Return a multipart container's child parts.
 * @param part {Part} the multipart container
 * @return {list of Part} the child parts
 */
export func parts(part as Part) {
    return $part.parts;
}

/**
 * Return a part's media type without parameters (e.g. "text/plain").
 * @param part {Part} the part to read
 * @return {string} the media type
 */
export func contentType(part as Part) {
    return typeOnly(findHeader($part.headers, "Content-Type"));
}

# needsQuoting reports whether a display name must be a quoted-string.
func needsQuoting(s as string) {
    if (strings.contains($s, ",") or strings.contains($s, "<")) {
        return true;
    }
    return strings.contains($s, "@") or strings.contains($s, "\"");
}

/**
 * Format an RFC 5322 mailbox: `email` alone, or `Name <email>` with the name
 * quoted when it carries a special character.
 * @param name {string} the display name (empty for a bare address)
 * @param email {string} the email address
 * @return {string} the formatted mailbox
 */
export func address(name as string, email as string) {
    if (len($name) == 0) {
        return $email;
    }
    if (not isAsciiText($name)) {
        return encodeWord($name) + " <" + $email + ">";
    }
    def display as string init $name;
    if (needsQuoting($name)) {
        $display = "\"" + strings.replace($name, "\"", "\\\"") + "\"";
    }
    return $display + " <" + $email + ">";
}
