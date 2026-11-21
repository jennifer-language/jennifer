# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.27.0

/**
 * Build and parse `multipart/form-data` bodies (RFC 7578) - the file-upload
 * counterpart to `mime`'s email multipart. `build` assembles a list of `Part`s
 * (form fields and files) into a `(contentType, body)` pair ready to POST;
 * `parse` takes a `Content-Type` header and a body and returns the parts.
 * Bodies are `bytes`, so binary file content round-trips intact.
 *
 * Pairs with `web` / `httpd` for handling uploads and with `http` for sending
 * them. `parse` runs in Go (`encoding.parseMultipart`); runs on both binaries.
 * @module multipart
 * @example
 * use convert;
 * import "multipart.j" as multipart;
 * def parts as list of multipart.Part init [
 *     multipart.field("title", "hello"),
 *     multipart.file("doc", "a.txt", "text/plain", encoding.encode("utf-8", "hi"))
 * ];
 * def form as multipart.Built init multipart.build($parts);
 * # POST $form.body with Content-Type $form.contentType
 * def back as list of multipart.Part init multipart.parse($form.contentType, $form.body);
 */
use strings;
use regex;
use binary;
use crypto;
use encoding;

/**
 * One form part: a field or a file.
 * @field name {string} the field name
 * @field filename {string} the file name ("" for a plain field)
 * @field contentType {string} the part's Content-Type ("" for a plain field)
 * @field data {bytes} the part body
 */
export def struct Part {
    name as string,
    filename as string,
    contentType as string,
    data as bytes
};

/**
 * A built form body plus the matching Content-Type header (with the boundary).
 * @field contentType {string} the `multipart/form-data; boundary=...` value
 * @field body {bytes} the encoded body
 */
export def struct Built {
    contentType as string,
    body as bytes
};

func fail(msg as string) {
    throw Error{kind: "multipart", message: "multipart: " + $msg};
}

# BOUNDARY_RE is RFC 2046's boundary grammar: 1-70 characters from bchars, the
# last not a space. The `'` is concatenated in from a cooked string, since a raw
# string cannot hold one and a cooked one would read `{0,69}` as interpolation.
def const BOUNDARY_RE as string init "^[0-9A-Za-z" + "'" + '()+_,./:=? -]{0,69}[0-9A-Za-z' + "'" +
    '()+_,./:=?-]$';

# MAX_PARTS is the part cap `encoding.parseMultipart` enforces (kind "limit"):
# a body of many tiny parts costs far more time and memory than its size.
def const MAX_PARTS as int init 1000;

# --- part constructors (exported) -------------------------------------------

/**
 * A plain text form field.
 * @param name {string} the field name
 * @param value {string} the field value
 * @return {Part} the part
 */
export func field(name as string, value as string) {
    return Part{name: $name, filename: "", contentType: "", data: encoding.encode("utf-8", $value)};
}

/**
 * A file part.
 * @param name {string} the field name
 * @param filename {string} the file name
 * @param contentType {string} the file's Content-Type (e.g. "text/plain")
 * @param data {bytes} the file content
 * @return {Part} the part
 */
export func file(name as string, filename as string, contentType as string, data as bytes) {
    return Part{name: $name, filename: $filename, contentType: $contentType, data: $data};
}

/**
 * Decode a part's body as a UTF-8 string (for reading text field values).
 * @param p {Part} the part
 * @return {string} the decoded body
 */
export func text(p as Part) {
    return encoding.decode("utf-8", $p.data);
}

/**
 * Report whether a part is a file (has a filename).
 * @param p {Part} the part
 * @return {bool} true if the part carries a filename
 */
export func isFile(p as Part) {
    return len($p.filename) > 0;
}

# --- byte helpers (private) -------------------------------------------------

# --- build (exported) -------------------------------------------------------

# generateBoundary returns a fresh boundary token. The random suffix is drawn
# from the crypto source (not math's seedable RNG): a predictable boundary lets
# an attacker who influences part content embed a matching "--boundary"
# delimiter and inject or overwrite parts as seen by the receiver (RFC 7578
# requires an unguessable boundary when the payload is not fully sender-owned).
# 12 crypto bytes -> 24 hex chars, 96 bits of entropy.
func generateBoundary() {
    return "----JenniferFormBoundary" + encoding.toText("hex", crypto.randBytes(12));
}

# escapeParam makes a value safe inside a quoted Content-Disposition parameter:
# strip CR / LF (they would inject headers or a premature body separator) and
# backslash-escape `\` and `"` so a filename like `a".txt` cannot break out.
func escapeParam(s as string) {
    def clean as string init strings.replace(strings.replace($s, "\r", ""), "\n", "");
    $clean = strings.replace($clean, "\\", "\\\\");
    return strings.replace($clean, "\"", "\\\"");
}

# stripLines removes CR / LF from an unquoted header value (the part
# Content-Type) so it cannot inject an extra header line or a premature body
# separator. Unlike escapeParam it does not backslash-escape quotes, since a
# Content-Type is written unquoted.
func stripLines(s as string) {
    return strings.replace(strings.replace($s, "\r", ""), "\n", "");
}

/**
 * Build a form body with an explicit boundary (deterministic).
 * @param parts {list of Part} the parts
 * @param boundary {string} the boundary token (must not occur in any part body)
 * @return {Built} the Content-Type and encoded body
 */
export func buildWith(parts as list of Part, boundary as string) {
    # RFC 2046 5.1.1: 1-70 characters from a safe set, not ending in a space. A
    # CR / LF or quote would corrupt the Content-Type header.
    if (not regex.matches(BOUNDARY_RE, $boundary)) {
        fail("invalid boundary (RFC 2046: 1-70 of 0-9 A-Z a-z '()+_,-./:=? and space)");
    }
    def crlf as bytes init encoding.encode("utf-8", "\r\n");
    # A part holding the delimiter would be split there by the receiver.
    def delim as bytes init encoding.encode("utf-8", "--" + $boundary);
    def chunks as list of bytes init [];
    for (def p in $parts) {
        if (binary.contains($p.data, $delim)) {
            fail("a part's data contains the boundary; choose another");
        }
        def head as string init "--" + $boundary + "\r\n" +
            "Content-Disposition: form-data; name=\"" + escapeParam($p.name) + "\"";
        if (len($p.filename) > 0) {
            $head = $head + "; filename=\"" + escapeParam($p.filename) + "\"";
        }
        $head = $head + "\r\n";
        if (len($p.contentType) > 0) {
            $head = $head + "Content-Type: " + stripLines($p.contentType) + "\r\n";
        }
        $head = $head + "\r\n";
        # Append header bytes then data bytes into `body` in place: a by-value
        # putStr / appendBytes copies the whole growing body on every call
        # (O(parts^2 x size)); in-place append stays amortized O(total size).
        $chunks[] = encoding.encode("utf-8", $head);
        $chunks[] = $p.data;
        $chunks[] = $crlf;
    }
    $chunks[] = encoding.encode("utf-8", "--{$boundary}--\r\n");
    # A boundary that is not an RFC 2045 token (a space, or one of the tspecials
    # "(),/:=?" bchars allows) is only a valid parameter value quoted. No bchar
    # needs a backslash inside the quotes.
    def param as string init $boundary;
    if (not regex.matches("^[0-9A-Za-z'+_.-]+$", $boundary)) {
        $param = "\"" + $boundary + "\"";
    }
    return Built{
        contentType: "multipart/form-data; boundary=" + $param,
        body: binary.join($chunks)
    };
}

/**
 * Build a form body with a fresh random boundary.
 * @param parts {list of Part} the parts
 * @return {Built} the Content-Type and encoded body
 */
export func build(parts as list of Part) {
    return buildWith($parts, generateBoundary());
}

# --- parse (exported) -------------------------------------------------------

# parse runs in Go (`encoding.parseMultipart`): scanning a part header here as
# a list of characters made a 1 MiB header cost 1.6 GB. boundaryOf, paramOf,
# extractParam and extractHeader below are the .j reference of its rules, which
# the overlay pins.

# boundaryOf extracts the boundary token from a Content-Type header: the
# `boundary` parameter after the media type, matched as a whole parameter name
# (an `xboundary=` parameter is not it), or "" when absent.
func boundaryOf(contentType as string) {
    def semi as int init strings.indexOf($contentType, ";");
    if ($semi < 0) {
        return "";
    }
    return extractParam($contentType[$semi + 1..], "boundary");
}

# paramOf finds a `key="value"` (or bare `key=value`) parameter in a
# `;`-separated parameter list, matching the key only at a parameter boundary so
# `name` does not match inside `filename`, honoring quotes (a `;` inside a
# quoted value is not a separator) and `\"` / `\\` escapes. Returns [] when the
# key is absent, else [value]. An unterminated quoted value is malformed.
func paramOf(headers as string, key as string) {
    def target as string init strings.lower($key);
    def cs as list of string init strings.chars($headers);
    def n as int init len($cs);
    def i as int init 0;
    while ($i < $n) {
        while ($i < $n and ($cs[$i] == ";" or $cs[$i] == " " or $cs[$i] == "\t")) {
            $i = $i + 1;
        }
        def k as list of string init [];
        while ($i < $n and not ($cs[$i] == "=" or $cs[$i] == ";")) {
            $k[] = $cs[$i];
            $i = $i + 1;
        }
        if ($i < $n and $cs[$i] == "=") {
            $i = $i + 1;
            def v as list of string init [];
            if ($i < $n and $cs[$i] == "\"") {
                $i = $i + 1;
                while ($i < $n and not ($cs[$i] == "\"")) {
                    if ($cs[$i] == "\\" and $i + 1 < $n) {
                        $i = $i + 1;
                    }
                    $v[] = $cs[$i];
                    $i = $i + 1;
                }
                if ($i >= $n) {
                    fail("unterminated quoted parameter value");
                }
                $i = $i + 1;
            } else {
                while ($i < $n and not ($cs[$i] == ";")) {
                    $v[] = $cs[$i];
                    $i = $i + 1;
                }
            }
            if (strings.lower(strings.trim(strings.join($k, ""))) == $target) {
                return [strings.trim(strings.join($v, ""))];
            }
        }
    }
    return [];
}

# extractParam is paramOf's value, or "" when the key is absent.
func extractParam(headers as string, key as string) {
    def found as list of string init paramOf($headers, $key);
    if (len($found) == 0) {
        return "";
    }
    return $found[0];
}

# extractHeader pulls a `Name: value` header line's value (case-insensitive
# name), or "" when absent. A line without a colon is not a header field.
func extractHeader(headers as string, name as string) {
    def lines as list of string init strings.split($headers, "\r\n");
    for (def line in $lines) {
        def colon as int init strings.indexOf($line, ":");
        if ($colon < 0) {
            fail("malformed part header line (no colon)");
        }
        def k as string init strings.lower(strings.trim($line[..$colon]));
        if ($k == $name) {
            return strings.trim($line[$colon + 1..]);
        }
    }
    return "";
}

/**
 * Parse a `multipart/form-data` body into its parts.
 * @param contentType {string} the Content-Type header (carrying the boundary)
 * @param body {bytes} the encoded body
 * @return {list of Part} the parts (fields and files)
 * @throws {Error} kind "multipart" if the boundary is missing, a body part line
 *   starts with the delimiter, the close delimiter is missing, or a part is
 *   malformed (no header terminator, a header that is not UTF-8, a header line
 *   without a colon, no `Content-Disposition: form-data` with a `name`); kind
 *   "limit" past 1000 parts, a part header over 16 KiB, or a name / filename
 *   over 1 KiB
 */
export func parse(contentType as string, body as bytes) {
    def raw as list of encoding.FormPart init [];
    try {
        $raw = encoding.parseMultipart($contentType, $body);
    } catch (e) {
        if ($e.kind == "encoding") {
            fail($e.message);
        }
        throw $e;
    }
    def parts as list of Part init [];
    for (def p in $raw) {
        $parts[] = Part{
            name: $p.name,
            filename: $p.filename,
            contentType: $p.contentType,
            data: $p.data
        };
    }
    return $parts;
}
