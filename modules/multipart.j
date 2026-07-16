# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

/**
 * Build and parse `multipart/form-data` bodies (RFC 7578) - the file-upload
 * counterpart to `mime`'s email multipart. `build` assembles a list of `Part`s
 * (form fields and files) into a `(contentType, body)` pair ready to POST;
 * `parse` takes a `Content-Type` header and a body and returns the parts.
 * Bodies are `bytes`, so binary file content round-trips intact.
 *
 * Pairs with `web` / `httpd` for handling uploads and with `http` for sending
 * them. Pure `.j` over `strings` + `bytes`; runs on both binaries.
 * @module multipart
 * @example
 * import "multipart.j" as multipart;
 * def parts as list of multipart.Part init [
 *     multipart.field("title", "hello"),
 *     multipart.file("doc", "a.txt", "text/plain", convert.bytesFromString("hi", "utf-8"))
 * ];
 * def form as multipart.Built init multipart.build($parts);
 * # POST $form.body with Content-Type $form.contentType
 * def back as list of multipart.Part init multipart.parse($form.contentType, $form.body);
 */
use strings;
use convert;
use lists;
use math;

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
    throw Error{ kind: "multipart", message: "multipart: " + $msg, file: "", line: 0, col: 0 };
}

# --- part constructors (exported) -------------------------------------------

/**
 * A plain text form field.
 * @param name {string} the field name
 * @param value {string} the field value
 * @return {Part} the part
 */
export func field(name as string, value as string) {
    return Part{ name: $name, filename: "", contentType: "", data: convert.bytesFromString($value, "utf-8") };
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
    return Part{ name: $name, filename: $filename, contentType: $contentType, data: $data };
}

/**
 * Decode a part's body as a UTF-8 string (for reading text field values).
 * @param p {Part} the part
 * @return {string} the decoded body
 */
export func text(p as Part) {
    return convert.stringFromBytes($p.data, "utf-8");
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

func emptyBytes() {
    def b as bytes;
    return $b;
}

func appendBytes(dst as bytes, src as bytes) {
    def i as int init 0;
    while ($i < len($src)) {
        $dst[] = $src[$i];
        $i = $i + 1;
    }
    return $dst;
}

func sliceBytes(src as bytes, start as int, end as int) {
    def out as bytes;
    def i as int init $start;
    while ($i < $end) {
        $out[] = $src[$i];
        $i = $i + 1;
    }
    return $out;
}

# putStr appends a string's UTF-8 bytes to a buffer.
func putStr(b as bytes, s as string) {
    return appendBytes($b, convert.bytesFromString($s, "utf-8"));
}

# indexOfBytes finds needle in hay at or after `from`, or -1.
func indexOfBytes(hay as bytes, needle as bytes, from as int) {
    def n as int init len($needle);
    if ($n == 0) {
        return $from;
    }
    def limit as int init len($hay) - $n;
    def i as int init $from;
    while ($i <= $limit) {
        def match as bool init true;
        def j as int init 0;
        while ($j < $n and $match) {
            if (not ($hay[$i + $j] == $needle[$j])) {
                $match = false;
            }
            $j = $j + 1;
        }
        if ($match) {
            return $i;
        }
        $i = $i + 1;
    }
    return -1;
}

# --- build (exported) -------------------------------------------------------

# generateBoundary returns a fresh, unlikely-to-collide boundary token.
func generateBoundary() {
    def digits as string init "0123456789abcdef";
    def out as string init "----JenniferFormBoundary";
    def i as int init 0;
    while ($i < 24) {
        def d as int init math.randInt(0, 15);
        $out = $out + strings.substring($digits, $d, $d + 1);
        $i = $i + 1;
    }
    return $out;
}

/**
 * Build a form body with an explicit boundary (deterministic).
 * @param parts {list of Part} the parts
 * @param boundary {string} the boundary token (must not occur in any part body)
 * @return {Built} the Content-Type and encoded body
 */
# escapeParam makes a value safe inside a quoted Content-Disposition parameter:
# strip CR / LF (they would inject headers or a premature body separator) and
# backslash-escape `\` and `"` so a filename like `a".txt` cannot break out.
func escapeParam(s as string) {
    def clean as string init strings.replace(strings.replace($s, "\r", ""), "\n", "");
    $clean = strings.replace($clean, "\\", "\\\\");
    return strings.replace($clean, "\"", "\\\"");
}

export func buildWith(parts as list of Part, boundary as string) {
    def body as bytes;
    for (def p in $parts) {
        def head as string init "--" + $boundary + "\r\n" +
            "Content-Disposition: form-data; name=\"" + escapeParam($p.name) + "\"";
        if (len($p.filename) > 0) {
            $head = $head + "; filename=\"" + escapeParam($p.filename) + "\"";
        }
        $head = $head + "\r\n";
        if (len($p.contentType) > 0) {
            $head = $head + "Content-Type: " + $p.contentType + "\r\n";
        }
        $head = $head + "\r\n";
        # Append header bytes then data bytes into `body` in place: a by-value
        # putStr / appendBytes copies the whole growing body on every call
        # (O(parts^2 x size)); in-place append stays amortized O(total size).
        def hb as bytes init convert.bytesFromString($head, "utf-8");
        def hi as int init 0;
        while ($hi < len($hb)) {
            $body[] = $hb[$hi];
            $hi = $hi + 1;
        }
        def di as int init 0;
        while ($di < len($p.data)) {
            $body[] = $p.data[$di];
            $di = $di + 1;
        }
        $body[] = 13;
        $body[] = 10;
    }
    def tail as bytes init convert.bytesFromString("--" + $boundary + "--\r\n", "utf-8");
    def ti as int init 0;
    while ($ti < len($tail)) {
        $body[] = $tail[$ti];
        $ti = $ti + 1;
    }
    return Built{ contentType: "multipart/form-data; boundary=" + $boundary, body: $body };
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

# boundaryOf extracts the boundary token from a Content-Type header.
func boundaryOf(contentType as string) {
    def lower as string init strings.lower($contentType);
    def idx as int init strings.indexOf($lower, "boundary=");
    if ($idx < 0) {
        return "";
    }
    def start as int init $idx + 9;   # len("boundary=")
    def rest as string init strings.substring($contentType, $start, len($contentType));
    if (strings.startsWith($rest, "\"")) {
        def inner as string init strings.substring($rest, 1, len($rest));
        def q as int init strings.indexOf($inner, "\"");
        if ($q >= 0) {
            return strings.substring($inner, 0, $q);
        }
        return $inner;
    }
    def semi as int init strings.indexOf($rest, ";");
    if ($semi >= 0) {
        return strings.trim(strings.substring($rest, 0, $semi));
    }
    return strings.trim($rest);
}

# extractParam pulls a `key="value"` (or bare `key=value`) parameter from a
# Content-Disposition header, matching the key only at a parameter boundary so
# `name` does not match inside `filename`, honoring quotes (a `;` inside a
# quoted value is not a separator) and `\"` / `\\` escapes.
func extractParam(headers as string, key as string) {
    def target as string init strings.lower($key);
    def cs as list of string init strings.chars($headers);
    def n as int init len($cs);
    def i as int init 0;
    while ($i < $n) {
        while ($i < $n and ($cs[$i] == ";" or $cs[$i] == " " or $cs[$i] == "\t")) {
            $i = $i + 1;
        }
        def k as string init "";
        while ($i < $n and not ($cs[$i] == "=" or $cs[$i] == ";")) {
            $k = $k + $cs[$i];
            $i = $i + 1;
        }
        if ($i < $n and $cs[$i] == "=") {
            $i = $i + 1;
            def v as string init "";
            if ($i < $n and $cs[$i] == "\"") {
                $i = $i + 1;
                while ($i < $n and not ($cs[$i] == "\"")) {
                    if ($cs[$i] == "\\" and $i + 1 < $n) {
                        $i = $i + 1;
                    }
                    $v = $v + $cs[$i];
                    $i = $i + 1;
                }
                if ($i < $n) {
                    $i = $i + 1;
                }
            } else {
                while ($i < $n and not ($cs[$i] == ";")) {
                    $v = $v + $cs[$i];
                    $i = $i + 1;
                }
            }
            if (strings.lower(strings.trim($k)) == $target) {
                return strings.trim($v);
            }
        }
    }
    return "";
}

# extractHeader pulls a `Name: value` header line's value (case-insensitive name).
func extractHeader(headers as string, name as string) {
    def lines as list of string init strings.split($headers, "\r\n");
    for (def line in $lines) {
        def colon as int init strings.indexOf($line, ":");
        if ($colon >= 0) {
            def k as string init strings.lower(strings.trim(strings.substring($line, 0, $colon)));
            if ($k == $name) {
                return strings.trim(strings.substring($line, $colon + 1, len($line)));
            }
        }
    }
    return "";
}

# parsePart decodes one part's bytes (headers + CRLFCRLF + data) into a Part.
func parsePart(pb as bytes) {
    def sep as bytes init convert.bytesFromString("\r\n\r\n", "utf-8");
    def hEnd as int init indexOfBytes($pb, $sep, 0);
    if ($hEnd < 0) {
        fail("malformed part: no header terminator");
    }
    def headerStr as string init convert.stringFromBytes(sliceBytes($pb, 0, $hEnd), "utf-8");
    def data as bytes init sliceBytes($pb, $hEnd + 4, len($pb));
    return Part{
        name: extractParam($headerStr, "name"),
        filename: extractParam($headerStr, "filename"),
        contentType: extractHeader($headerStr, "content-type"),
        data: $data
    };
}

/**
 * Parse a `multipart/form-data` body into its parts.
 * @param contentType {string} the Content-Type header (carrying the boundary)
 * @param body {bytes} the encoded body
 * @return {list of Part} the parts (fields and files)
 * @throws {Error} kind "multipart" if the boundary is missing or a part is malformed
 */
export func parse(contentType as string, body as bytes) {
    def boundary as string init boundaryOf($contentType);
    if (len($boundary) == 0) {
        fail("no boundary in Content-Type");
    }
    # Normalise by prepending CRLF so every delimiter is "\r\n--boundary"
    # (this avoids matching the token inside a part body).
    def work as bytes init putStr(emptyBytes(), "\r\n");
    $work = appendBytes($work, $body);
    def delim as bytes init convert.bytesFromString("\r\n--" + $boundary, "utf-8");
    def dlen as int init len($delim);
    def parts as list of Part init [];
    def pos as int init indexOfBytes($work, $delim, 0);
    def more as bool init $pos >= 0;
    while ($more) {
        def after as int init $pos + $dlen;
        if ($after + 2 <= len($work) and $work[$after] == 45 and $work[$after + 1] == 45) {
            $more = false;   # closing "--boundary--"
        } else {
            def partStart as int init $after + 2;   # skip the CRLF after the delimiter
            def nextPos as int init indexOfBytes($work, $delim, $partStart);
            if ($nextPos < 0) {
                $more = false;
            } else {
                $parts[] = parsePart(sliceBytes($work, $partStart, $nextPos));
                $pos = $nextPos;
            }
        }
    }
    return $parts;
}
