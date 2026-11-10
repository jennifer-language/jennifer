# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.26.0

# A hand-rolled quoting-aware scanner: its parse core legitimately runs past the
# L201 statement-count limit. Every other lint check stays active.
# lint-disable-file: L201

/**
 * RFC 4180 comma-separated values: parse text into rows of fields and format
 * rows back into text. Pure Jennifer - no Go, no system library. The delimiter
 * is configurable, so the same code reads and writes TSV and other
 * single-character-separated formats. Parsing is strict: a quote inside an
 * unquoted field, text after a closing quote, an unclosed quote and records of
 * different widths are `csv` errors. Records separate on CRLF, LF or a bare CR.
 * format() quotes a field with `"` when it contains the delimiter, a quote or a
 * line break, doubles an embedded quote, and joins records with CRLF without a
 * trailing line break, so parse(format(rows)) returns the rows.
 * @module csv
 * @example
 * import "csv.j" as csv;
 * def rows as list of list of string init csv.parse("a,b\n1,\"x,y\"");
 * def recs as list of map of string to string init csv.toRecords($rows);
 */
use channel;
use fs;
use maps;
use strings;

# fail throws a catchable csv error.
func fail(msg as string) {
    throw Error{kind: "csv", message: "csv: " + $msg};
}

# failAt throws a csv error naming a position in the input.
func failAt(msg as string, line as int, col as int) {
    fail("{$msg} at line {$line}, column {$col}");
}

# checkWidth rejects a Dialect width other than -1 (any), 0 (the first
# record's) or a field count.
func checkWidth(width as int) {
    if ($width < -1) {
        fail("the width must be -1, 0 or a field count, got {$width}");
    }
}

# checkWidths fails on the first record whose field count differs from the
# expected one: `width` fields, the first record's when `width` is 0, any when
# it is -1.
func checkWidths(rows as list of list of string, width as int) {
    if ($width < 0 or len($rows) == 0) {
        return null;
    }
    def want as int init $width;
    if ($want == 0) {
        $want = len($rows[0]);
    }
    def n as int init 0;
    for (def row in $rows) {
        $n = $n + 1;
        def got as int init len($row);
        if ($got != $want) {
            fail("record {$n} has {$got} fields, expected {$want}");
        }
    }
    return null;
}

# checkChars rejects a delimiter or quote that is not exactly one character, is
# a line break, or is the same as the other: none of those can delimit fields.
func checkChars(delim as string, quote as string) {
    if (len($delim) != 1 or $delim == "\n" or $delim == "\r") {
        fail("the delimiter must be one character other than a line break, got '" + $delim + "'");
    }
    if (len($quote) != 1 or $quote == "\n" or $quote == "\r" or $quote == $delim) {
        fail("the quote must be one character other than a line break and the delimiter");
    }
}

# --- parse (exported) ----------------------------------------------

/**
 * Parse CSV text with a single-character delimiter and return its rows, each a
 * list of string fields. A quoted field may hold the delimiter, line breaks and
 * doubled quotes; a blank line is a record with one empty field; an empty input
 * yields no rows. Every record must have as many fields as the first.
 * @param s {string} the CSV text to parse
 * @param delim {string} the single-character field delimiter
 * @return {list of list of string} the parsed rows of fields
 * @throws {Error} kind `"csv"` on a bad delimiter, malformed quoting (with its
 *   line and column) or a record of a different width
 */
export func parseWith(s as string, delim as string) {
    return parseCore($s, dialect($delim), 1);
}

/**
 * Parse standard comma-delimited CSV (see parseWith).
 * @param s {string} the CSV text to parse
 * @return {list of list of string} the parsed rows of fields
 * @throws {Error} kind `"csv"` as parseWith
 */
export func parse(s as string) {
    return parseWith($s, ",");
}

# --- format (exported) ---------------------------------------------

/**
 * Join rows into text with a single-character delimiter, quoting each field as
 * needed. Records are separated by CRLF (RFC 4180) with no trailing line break.
 * A record that is one empty field is written `""`, so it reads back as a
 * record. Every row must have as many fields as the first, and at least one.
 *
 * **Not safe for a spreadsheet target.** A field beginning `= + - @` (or a tab /
 * CR) is a formula that Excel / Google Sheets will execute on open (CWE-1236). If
 * the output is opened as a spreadsheet, use `formatSafe` / `formatSafeWith`,
 * which neutralise such fields.
 * @param rows {list of list of string} the rows of fields to format
 * @param delim {string} the single-character field delimiter
 * @return {string} the formatted delimiter-separated text
 * @throws {Error} kind `"csv"` on a bad delimiter, a row with no fields, or rows
 *   of different widths
 */
export func formatWith(rows as list of list of string, delim as string) {
    return formatCore($rows, dialect($delim));
}

/**
 * Join rows into standard comma-delimited CSV. **Not safe for a spreadsheet
 * target** - use `formatSafe` when the output may be opened in Excel / Sheets
 * (see `formatWith` on the CWE-1236 formula-injection risk).
 * @param rows {list of list of string} the rows of fields to format
 * @return {string} the formatted comma-separated text
 * @throws {Error} kind `"csv"` as formatWith
 */
export func format(rows as list of list of string) {
    return formatWith($rows, ",");
}

# --- header records (exported) -------------------------------------

# checkHeader fails on a header that names a column twice, and returns its
# names as a set.
func checkHeader(header as list of string) {
    def seen as map of string to bool init {};
    for (def name in $header) {
        if (maps.has($seen, $name)) {
            fail("the header names the column '{$name}' twice");
        }
        $seen[$name] = true;
    }
    return $seen;
}

/**
 * Treat the first row as a header and map each later row into a
 * `map of string to string` keyed by the header names. An empty input yields
 * no records.
 * @param rows {list of list of string} the rows, with the first as the header
 * @return {list of map of string to string} one record per non-header row
 * @throws {Error} kind `"csv"` when the header names a column twice or a row
 *   does not have one field per header name
 */
export func toRecords(rows as list of list of string) {
    def records as list of map of string to string init [];
    if (len($rows) == 0) {
        return $records;
    }
    def header as list of string init $rows[0];
    checkHeader($header);
    def r as int init 1;
    while ($r < len($rows)) {
        def row as list of string init $rows[$r];
        def got as int init len($row);
        def want as int init len($header);
        if ($got != $want) {
            fail("row {$r + 1} has {$got} fields, the header has {$want}");
        }
        def rec as map of string to string init {};
        for (def c in 0..len($header)) {
            $rec[$header[$c]] = $row[$c];
        }
        $records[] = $rec;
        $r = $r + 1;
    }
    return $records;
}

/**
 * The inverse of toRecords: emit the header row followed by one row per record,
 * taking fields in header order. The explicit header fixes the column order,
 * which map iteration does not.
 * @param header {list of string} the column names, in output order
 * @param records {list of map of string to string} the records to emit
 * @return {list of list of string} the header row followed by one row per record
 * @throws {Error} kind `"csv"` when the header names a column twice, or a
 *   record lacks a header key or has a key the header does not name
 */
export func fromRecords(header as list of string, records as list of map of string to string) {
    def known as map of string to bool init checkHeader($header);
    def rows as list of list of string init [];
    $rows[] = $header;
    def n as int init 0;
    for (def rec in $records) {
        $n = $n + 1;
        for (def key in maps.keys($rec)) {
            if (not maps.has($known, $key)) {
                fail("record {$n} has the key '{$key}', which the header does not name");
            }
        }
        def row as list of string init [];
        for (def col in $header) {
            if (not maps.has($rec, $col)) {
                fail("record {$n} has no '{$col}' key");
            }
            $row[] = $rec[$col];
        }
        $rows[] = $row;
    }
    return $rows;
}

# --- formula-injection-safe format (exported) ----------------------

# sanitizeField neutralises a spreadsheet-injection payload (CWE-1236): a field
# whose FIRST character is one of `= + - @`, a tab, or a CR is prefixed with a
# single apostrophe, which every major spreadsheet treats as "this is text, do
# not evaluate". A field not starting with a dangerous character is returned
# unchanged. Pure and per-field, so it unit-tests directly.
func sanitizeField(s as string) {
    if (len($s) == 0) {
        return $s;
    }
    def first as string init $s[..1];
    if ($first == "=" or $first == "+" or $first == "-" or $first == "@" or $first == "\t" or
        $first == "\r") {
        return "'" + $s;
    }
    return $s;
}

/**
 * Like formatWith, but neutralises spreadsheet formula injection (CWE-1236)
 * before quoting: any field whose first character is `=`, `+`, `-`, `@`, a tab,
 * or a CR is prefixed with a single apostrophe so Excel / Google Sheets import
 * it as literal text instead of executing it. Use this - not formatWith - for
 * any CSV a spreadsheet will open.
 * @param rows {list of list of string} the rows of fields to format
 * @param delim {string} the single-character field delimiter
 * @return {string} the formatted, injection-neutralised text
 */
export func formatSafeWith(rows as list of list of string, delim as string) {
    def safe as list of list of string init [];
    for (def row in $rows) {
        def srow as list of string init [];
        for (def field in $row) {
            $srow[] = sanitizeField($field);
        }
        $safe[] = $srow;
    }
    return formatWith($safe, $delim);
}

/**
 * Injection-safe standard comma-delimited CSV (see formatSafeWith). The
 * export-to-spreadsheet path; prefer it over format for spreadsheet targets.
 * @param rows {list of list of string} the rows of fields to format
 * @return {string} the formatted, injection-neutralised comma-separated text
 */
export func formatSafe(rows as list of list of string) {
    return formatSafeWith($rows, ",");
}

# --- dialects (exported) -------------------------------------------

/**
 * A CSV dialect: the field delimiter, the quote character, an optional
 * comment-line prefix (a physical line whose record starts with it is skipped
 * on parse; "" disables comments), whether whitespace around fields is trimmed
 * on parse, and the record width. Build one with `dialect(delimiter)` for the
 * defaults.
 * @field delimiter {string} the single-character field delimiter
 * @field quote {string} the quote character (default `"`)
 * @field comment {string} the comment-line prefix, or "" for none
 * @field trim {bool} trim whitespace from unquoted fields, and allow spaces and
 *   tabs around a quoted field (its content stays verbatim)
 * @field width {int} the fields every record must have: 0 (default) as many as
 *   the first record, a count, or -1 for any number
 */
export def struct Dialect {
    delimiter as string,
    quote as string,
    comment as string,
    trim as bool,
    width as int
};

/**
 * A Dialect with the given delimiter and the defaults: quote `"`, no comment
 * prefix, no trimming, every record as wide as the first. Set the other fields
 * to customise.
 * @param delimiter {string} the single-character field delimiter
 * @return {Dialect} the dialect
 */
export func dialect(delimiter as string) {
    return Dialect{delimiter: $delimiter, quote: '"', comment: "", trim: false, width: 0};
}

# startsAt reports whether `chars` matches `prefix` starting at index i.
func startsAt(chars as list of string, i as int, prefix as string, n as int) {
    def p as list of string init strings.chars($prefix);
    def m as int init len($p);
    if ($m == 0 or $i + $m > $n) {
        return false;
    }
    return $chars[$i..$i + $m] == $p;
}

# flushField joins an accumulated field, trimming it when trimming is on and the
# field was not quoted (quoted fields keep their whitespace verbatim).
func flushField(field as list of string, trim as bool, quoted as bool) {
    def v as string init strings.join($field, "");
    if ($trim and not $quoted) {
        return strings.trim($v);
    }
    return $v;
}

# parseCore parses under a dialect; `line0` is the line number of the text's
# first line, for error positions. Physical lines are grouped into records by
# quote parity (an odd running count means a quoted field is still open - the
# same state the scanner reaches); a record whose fields are all plain or
# well-formed quoted ones is split natively, any other goes through
# scanRecords, which accepts it or names the fault. A comment line is honoured
# only where a record starts.
func parseCore(s as string, d as Dialect, line0 as int) {
    checkChars($d.delimiter, $d.quote);
    checkWidth($d.width);
    def delim as string init $d.delimiter;
    def quote as string init $d.quote;
    def comment as string init $d.comment;
    def trim as bool init $d.trim;
    def rows as list of list of string init [];
    if ($s == "") {
        return $rows;
    }
    # A bare CR (not part of CRLF) also ends a record, and a comment line, so a
    # text holding one is left to the scanner.
    if (strings.contains(strings.replace($s, "\r\n", ""), "\r")) {
        $rows = scanRecords($s, $d, $line0, false);
        checkWidths($rows, $d.width);
        return $rows;
    }
    def lines as list of string init strings.split($s, "\n");
    def last as int init len($lines);
    if ($lines[$last - 1] == "") {
        $last = $last - 1; # the text ended on a line break: no record after it
    }
    def i as int init 0;
    while ($i < $last) {
        def line as string init $lines[$i];
        if ($comment != "" and strings.startsWith($line, $comment)) {
            $i = $i + 1;
            continue;
        }
        def start as int init $i;
        def group as list of string init [$line];
        def quotes as int init strings.count($line, $quote);
        $i = $i + 1;
        while ($quotes % 2 == 1 and $i < $last) {
            $group[] = $lines[$i];
            $quotes = $quotes + strings.count($lines[$i], $quote);
            $i = $i + 1;
        }
        def text as string init strings.join($group, "\n");
        if (strings.endsWith($text, "\r") and $quotes % 2 == 0) {
            $text = $text[..len($text) - 1]; # CRLF
        }
        if (not strings.contains($text, "\r")) {
            def sp as FastSplit init splitRecord($text, $delim, $quote, $trim);
            if ($sp.ok) {
                $rows[] = $sp.fields;
                continue;
            }
        }
        $text = strings.join($group, "\n");
        if ($i < len($lines)) {
            $text = $text + "\n"; # the record ended on a line break
        }
        for (def r in scanRecords($text, $d, $line0 + $start, false)) {
            $rows[] = $r;
        }
    }
    checkWidths($rows, $d.width);
    return $rows;
}

# FastSplit is splitRecord's result: ok is false when the record needs the
# character scanner.
def struct FastSplit {
    ok as bool,
    fields as list of string
};

# splitRecord splits one record natively: split on the delimiter, rejoin
# segments while a quoted field is open (odd quote count), then unquote each
# field of the form "..." whose inner quotes are all doubled. Any other use of
# the quote character (one mid-field, text after a closing quote, spaces around
# a quoted field) is left to the scanner, which accepts or rejects it.
func splitRecord(text as string, delim as string, quote as string, trim as bool) {
    def out as list of string init [];
    def none as FastSplit init FastSplit{ok: false, fields: []};
    def segs as list of string init strings.split($text, $delim);
    def n as int init len($segs);
    def k as int init 0;
    while ($k < $n) {
        def field as string init $segs[$k];
        $k = $k + 1;
        if (not strings.contains($field, $quote)) {
            if ($trim) {
                $field = strings.trim($field);
            }
            $out[] = $field;
            continue;
        }
        def q as int init strings.count($field, $quote);
        def parts as list of string init [$field];
        while ($q % 2 == 1 and $k < $n) {
            $parts[] = $segs[$k];
            $q = $q + strings.count($segs[$k], $quote);
            $k = $k + 1;
        }
        if ($q % 2 == 1) {
            return $none; # an unclosed quote: the scanner reports it
        }
        def whole as string init strings.join($parts, $delim);
        if (len($whole) < 2 or not strings.startsWith($whole, $quote) or
            not strings.endsWith($whole, $quote)) {
            return $none;
        }
        def inner as string init $whole[1..len($whole) - 1];
        def pair as string init $quote + $quote;
        if (strings.contains(strings.replace($inner, $pair, ""), $quote)) {
            return $none;
        }
        $out[] = strings.replace($inner, $pair, $quote);
    }
    return FastSplit{ok: true, fields: $out};
}

# scanRecords is the character scanner for records the fast path cannot split:
# quoted fields (which may hold the delimiter, line breaks and doubled quotes)
# and bare-CR record separators. It enforces RFC 4180 quoting: a quote opens a
# field only at its start (after spaces and tabs when trimming), a closing
# quote is followed by the delimiter or a line break (or, when trimming, spaces
# and tabs first), and every quote is closed. `line0` numbers the first line
# for error positions. With `partial`, a quote still open at the end is not an
# error: the streaming reader checks a record before its end arrives.
func scanRecords(s as string, d as Dialect, line0 as int, partial as bool) {
    def delim as string init $d.delimiter;
    def quote as string init $d.quote;
    def comment as string init $d.comment;
    def trim as bool init $d.trim;
    def rows as list of list of string init [];
    def row as list of string init [];
    def field as list of string init [];
    def inQuotes as bool init false;
    def fieldStarted as bool init false;
    def rowStarted as bool init false;
    def fieldQuoted as bool init false;
    def closed as bool init false; # the field's closing quote has been read
    def blank as bool init true; # the field so far is only spaces and tabs
    def line as int init $line0;
    def lineStart as int init 0;
    def openLine as int init 0;
    def openCol as int init 0;
    def chars as list of string init strings.chars($s);
    def n as int init len($chars);
    def i as int init 0;
    while ($i < $n) {
        def c as string init $chars[$i];
        # A comment prefix is honoured only at the very start of a record and
        # outside quotes: skip the whole physical line, terminator included.
        if (not $inQuotes and not $fieldStarted and not $rowStarted and $comment != "" and
            startsAt($chars, $i, $comment, $n)) {
            while ($i < $n and $chars[$i] != "\n" and $chars[$i] != "\r") {
                $i = $i + 1;
            }
            def stop as int init $i;
            if ($i + 1 < $n and $chars[$i] == "\r" and $chars[$i + 1] == "\n") {
                $i = $i + 2;
            } elseif ($i < $n) {
                $i = $i + 1;
            }
            if ($i > $stop) {
                $line = $line + 1;
                $lineStart = $i;
            }
            continue;
        }
        if ($inQuotes and $c == $quote and $i + 1 < $n and $chars[$i + 1] == $quote) {
            $field[] = $quote;
            $i = $i + 2;
            continue;
        }
        if ($inQuotes and $c == $quote) {
            $inQuotes = false;
            $closed = true;
            $i = $i + 1;
            continue;
        }
        if ($inQuotes) {
            $field[] = $c;
            $i = $i + 1;
            if ($c == "\n" or ($c == "\r" and not ($i < $n and $chars[$i] == "\n"))) {
                $line = $line + 1;
                $lineStart = $i;
            }
            continue;
        }
        if ($c == $delim) {
            $row[] = flushField($field, $trim, $fieldQuoted);
            $field = [];
            $fieldStarted = false;
            $rowStarted = true;
            $fieldQuoted = false;
            $closed = false;
            $blank = true;
            $i = $i + 1;
            continue;
        }
        if ($c == "\n" or $c == "\r") {
            $row[] = flushField($field, $trim, $fieldQuoted);
            $rows[] = $row;
            $field = [];
            $row = [];
            $fieldStarted = false;
            $rowStarted = false;
            $fieldQuoted = false;
            $closed = false;
            $blank = true;
            if ($c == "\r" and $i + 1 < $n and $chars[$i + 1] == "\n") {
                $i = $i + 2;
            } else {
                $i = $i + 1;
            }
            $line = $line + 1;
            $lineStart = $i;
            continue;
        }
        if ($closed) {
            if ($trim and ($c == " " or $c == "\t")) {
                $i = $i + 1;
                continue;
            }
            failAt("text after a closing quote", $line, $i - $lineStart + 1);
        }
        if ($c == $quote) {
            if ($fieldStarted and not ($trim and $blank)) {
                failAt("a quote inside an unquoted field", $line, $i - $lineStart + 1);
            }
            $field = []; # spaces before the opening quote, when trimming
            $inQuotes = true;
            $fieldStarted = true;
            $rowStarted = true;
            $fieldQuoted = true;
            $openLine = $line;
            $openCol = $i - $lineStart + 1;
            $i = $i + 1;
            continue;
        }
        $field[] = $c;
        if ($c != " " and $c != "\t") {
            $blank = false;
        }
        $fieldStarted = true;
        $rowStarted = true;
        $i = $i + 1;
    }
    if ($inQuotes) {
        if ($partial) {
            return $rows;
        }
        fail("the quoted field opened at line {$openLine}, column {$openCol} is not closed");
    }
    if ($rowStarted or $fieldStarted) {
        $row[] = flushField($field, $trim, $fieldQuoted);
        $rows[] = $row;
    }
    return $rows;
}

/**
 * Parse CSV text under a Dialect: custom delimiter and quote, comment-line
 * skipping, optional trimming, and the record width. Quoted fields keep their
 * content (including whitespace and line breaks) verbatim.
 * @param text {string} the CSV text to parse
 * @param d {Dialect} the dialect
 * @return {list of list of string} the parsed rows of fields
 * @throws {Error} kind `"csv"` on a bad dialect, malformed quoting (with its
 *   line and column) or a record of the wrong width
 */
export func parseDialect(text as string, d as Dialect) {
    return parseCore($text, $d, 1);
}

# needsQuoteQ reports whether a field must be quoted to read back unchanged
# under the dialect: it holds the delimiter, the quote or a line break; it has
# surrounding whitespace that trimming would strip; or it is a record's first
# field and starts with the comment prefix.
func needsQuoteQ(f as string, d as Dialect, first as bool) {
    if (strings.contains($f, $d.delimiter) or strings.contains($f, $d.quote)) {
        return true;
    }
    if (strings.contains($f, "\n") or strings.contains($f, "\r")) {
        return true;
    }
    if ($d.trim and strings.trim($f) != $f) {
        return true;
    }
    return $first and $d.comment != "" and strings.startsWith($f, $d.comment);
}

func quoteFieldQ(f as string, d as Dialect, first as bool) {
    if (needsQuoteQ($f, $d, $first)) {
        return $d.quote + strings.replace($f, $d.quote, $d.quote + $d.quote) + $d.quote;
    }
    return $f;
}

# formatRecord formats one row as a record without its line break. `want` is
# the field count it must have (0 or -1: any), `n` its 1-based number for
# errors. A record needs at least one field, and a lone empty field is written
# quoted: unquoted it would be an empty line, which reads back as no record at
# the end of the text.
func formatRecord(row as list of string, d as Dialect, want as int, n as int) {
    if (len($row) == 0) {
        fail("row {$n} has no fields; a CSV record has at least one");
    }
    def got as int init len($row);
    if ($want > 0 and $got != $want) {
        fail("row {$n} has {$got} fields, expected {$want}");
    }
    if (len($row) == 1 and $row[0] == "") {
        return $d.quote + $d.quote;
    }
    def fields as list of string init [];
    def first as bool init true;
    for (def field in $row) {
        $fields[] = quoteFieldQ($field, $d, $first);
        $first = false;
    }
    return strings.join($fields, $d.delimiter);
}

# formatCore formats rows under a dialect so that parseDialect with the same
# dialect returns them: CRLF between records, widths checked as parsing checks
# them. Records are collected and joined once: an accumulating `+` over a large
# table would be O(N^2) in the output size.
func formatCore(rows as list of list of string, d as Dialect) {
    checkChars($d.delimiter, $d.quote);
    checkWidth($d.width);
    def want as int init $d.width;
    if ($want == 0 and len($rows) > 0) {
        $want = len($rows[0]);
    }
    def lines as list of string init [];
    def n as int init 0;
    for (def row in $rows) {
        $n = $n + 1;
        $lines[] = formatRecord($row, $d, $want, $n);
    }
    return strings.join($lines, "\r\n");
}

/**
 * Format rows under a Dialect (delimiter, quote, width), like formatWith: CRLF
 * between records, no trailing line break. A field that trimming would change
 * and a first field starting with the comment prefix are quoted, so
 * parseDialect with the same dialect returns the rows.
 * @param rows {list of list of string} the rows of fields to format
 * @param d {Dialect} the dialect
 * @return {string} the formatted text
 * @throws {Error} kind `"csv"` on a bad dialect, a row with no fields, or a row
 *   of the wrong width
 */
export func formatDialect(rows as list of list of string, d as Dialect) {
    return formatCore($rows, $d);
}

# --- streaming handles (exported) ----------------------------------

# ReadState is a streaming reader's state, held in a one-slot channel so every
# copy of the Reader shares it: text read from the file and not yet returned,
# whether the file is exhausted, the record width (0 until the first record),
# and the number of records read and the line the next one starts on.
def struct ReadState {
    buf as string,
    eof as bool,
    width as int,
    record as int,
    line as int
};

# The number of characters a reader takes from its file at a time.
def const READ_CHUNK as int init 4096;

/**
 * A streaming CSV reader over an open `fs.File`, for tables too large to hold in
 * memory. The file and the read state are handles shared by every copy of the
 * reader, so successive `readRow` calls advance the same stream. Build it with
 * `reader` or `readerWith`.
 * @field file {fs.File} the underlying open file handle
 * @field delim {string} the single-character field delimiter
 * @field state {channel of ReadState} the read buffer and position (internal)
 */
export def struct Reader {
    file as fs.File,
    delim as string,
    state as channel of ReadState
};

/**
 * Wrap an open, read-mode `fs.File` as a comma-delimited streaming reader.
 * @param file {fs.File} an open read-mode file handle
 * @return {Reader} the reader
 */
export func reader(file as fs.File) {
    return readerWith($file, ",");
}

/**
 * Wrap an open, read-mode `fs.File` as a streaming reader with a custom
 * single-character delimiter.
 * @param file {fs.File} an open read-mode file handle
 * @param delim {string} the single-character field delimiter
 * @return {Reader} the reader
 * @throws {Error} kind `"csv"` on a bad delimiter
 */
export func readerWith(file as fs.File, delim as string) {
    checkChars($delim, '"');
    def state as channel of ReadState init channel.make(1);
    channel.send($state, ReadState{buf: "", eof: false, width: 0, record: 0, line: 1});
    return Reader{file: $file, delim: $delim, state: $state};
}

# fill appends the file's next chunk to the buffer and notes its end.
func fill(file as fs.File, st as ReadState) {
    if ($st.eof) {
        return $st;
    }
    if (fs.eof($file)) {
        $st.eof = true;
        return $st;
    }
    def more as string init fs.readChars($file, READ_CHUNK);
    $st.buf = $st.buf + $more;
    if ($more == "" or fs.eof($file)) {
        $st.eof = true;
    }
    return $st;
}

# lineBreak returns the index of the first CR or LF in s at or after `from`, or
# -1.
func lineBreak(s as string, from as int) {
    def nl as int init strings.indexOf($s, "\n", $from);
    def cr as int init strings.indexOf($s, "\r", $from);
    if ($cr >= 0 and ($nl < 0 or $cr < $nl)) {
        return $cr;
    }
    return $nl;
}

# lineBreaks counts the line breaks in s: LF, CRLF and bare CR each count once.
func lineBreaks(s as string) {
    return strings.count($s, "\n") + strings.count($s, "\r") - strings.count($s, "\r\n");
}

# quotingBroken reports whether the start of a record already breaks the
# quoting rules, so the reader stops there instead of reading on for a closing
# quote that a stray one never gets.
func quotingBroken(text as string, delim as string, line0 as int) {
    try {
        scanRecords($text, dialect($delim), $line0, true);
    } catch (e) {
        return true;
    }
    return false;
}

# Take is takeRecord's result: the state after the record, the record's text
# without its line break, and whether a record was left at all.
def struct Take {
    st as ReadState,
    text as string,
    found as bool
};

# takeRecord cuts the next record off the buffer, reading more of the file
# until a line break outside quotes, or the end of the file, ends it. A CR at
# the end of the buffer waits for the next chunk, which may hold its LF.
func takeRecord(reader as Reader, st as ReadState) {
    def from as int init 0; # where the next line break is searched from
    def quotes as int init 0; # quotes before `from`
    while (true) {
        def pos as int init lineBreak($st.buf, $from);
        def size as int init len($st.buf);
        if ($pos < 0 or
            ($pos == $size - 1 and not $st.eof and
            strings.substring($st.buf, $pos) == "\r")) {
            if (not $st.eof) {
                $st = fill($reader.file, $st);
                continue;
            }
            if ($st.buf == "") {
                return Take{st: $st, text: "", found: false};
            }
            def rest as string init $st.buf;
            $st.buf = "";
            return Take{st: $st, text: $rest, found: true};
        }
        def q as int init strings.count(strings.substring($st.buf, $from, $pos), '"');
        $quotes = $quotes + $q;
        if ($quotes % 2 == 1 and
            ($q == 0 or
            not quotingBroken(strings.substring($st.buf, 0, $pos), $reader.delim, $st.line))) {
            $from = $pos + 1; # the line break is inside a quoted field
            continue;
        }
        def next as int init $pos + 1;
        if ($next < $size and strings.substring($st.buf, $pos, $next + 1) == "\r\n") {
            $next = $next + 1;
        }
        def text as string init strings.substring($st.buf, 0, $pos);
        $st.buf = strings.substring($st.buf, $next);
        return Take{st: $st, text: $text, found: true};
    }
}

# recordFields parses one record's text (no line break outside quotes); `line0`
# is the line it starts on. An empty text is a record with one empty field. A
# record without a CR goes to the native split first, as in parseCore.
func recordFields(text as string, delim as string, line0 as int) {
    if ($text == "") {
        return [""];
    }
    if (not strings.contains($text, "\r")) {
        def sp as FastSplit init splitRecord($text, $delim, '"', false);
        if ($sp.ok) {
            return $sp.fields;
        }
    }
    def rows as list of list of string init scanRecords($text, dialect($delim), $line0, false);
    if (len($rows) == 0) {
        return [""];
    }
    return $rows[0];
}

/**
 * Read the next record from the reader and return its fields. A quoted field
 * may span lines; CRLF, LF and a bare CR end a record, as in `parse`, and a
 * blank line is a record with one empty field `[""]`. Every record must have as
 * many fields as the first. Returns `[]` once no record is left (guard the
 * loop with `readerEof`). A malformed record is consumed before its error is
 * thrown, so a caller that catches it can read on.
 * @param reader {Reader} the reader
 * @return {list of string} the record's fields, or `[]` at the end
 * @throws {Error} kind `"csv"` on malformed quoting (with its line and column)
 *   or a record of a different width
 */
export func readRow(reader as Reader) {
    def st as ReadState init channel.recv($reader.state);
    def t as Take init Take{st: $st, text: "", found: false};
    try {
        $t = takeRecord($reader, $st);
    } catch (e) {
        channel.send($reader.state, $st);
        throw $e;
    }
    $st = $t.st;
    if (not $t.found) {
        channel.send($reader.state, $st);
        return [];
    }
    def line as int init $st.line;
    $st.record = $st.record + 1;
    $st.line = $st.line + lineBreaks($t.text) + 1;
    def fields as list of string init [""];
    def kind as string init "";
    def message as string init "";
    try {
        $fields = recordFields($t.text, $reader.delim, $line);
        if ($st.width == 0) {
            $st.width = len($fields);
        } elseif (len($fields) != $st.width) {
            def got as int init len($fields);
            fail("record {$st.record} has {$got} fields, expected {$st.width}");
        }
    } catch (e) {
        $kind = $e.kind;
        $message = $e.message;
    }
    channel.send($reader.state, $st);
    if ($kind != "") {
        throw Error{kind: $kind, message: $message};
    }
    return $fields;
}

/**
 * Whether the reader has no record left. Loop `while (not readerEof($r))`.
 * @param reader {Reader} the reader
 * @return {bool} true once the file is exhausted and every record returned
 */
export func readerEof(reader as Reader) {
    def st as ReadState init channel.recv($reader.state);
    if ($st.buf == "" and not $st.eof) {
        try {
            $st = fill($reader.file, $st);
        } catch (e) {
            channel.send($reader.state, $st);
            throw $e;
        }
    }
    channel.send($reader.state, $st);
    return $st.buf == "" and $st.eof;
}

/**
 * Close a streaming reader (closes the underlying file handle).
 * @param reader {Reader} the reader
 */
export func closeReader(reader as Reader) {
    fs.close($reader.file);
    return null;
}

# WriteState is a streaming writer's state, shared like ReadState: the record
# width (0 until the first record) and the number of records written.
def struct WriteState {
    width as int,
    rows as int
};

/**
 * A streaming CSV writer over an open `fs.File`. Each `writeRow` call appends one
 * quoted-as-needed record terminated by CRLF, so a large table is written
 * without building the whole text in memory. Build it with `writer` or
 * `writerWith`.
 * @field file {fs.File} the underlying open write/append-mode file handle
 * @field delim {string} the single-character field delimiter
 * @field state {channel of WriteState} the record width and count (internal)
 */
export def struct Writer {
    file as fs.File,
    delim as string,
    state as channel of WriteState
};

/**
 * Wrap an open write/append-mode `fs.File` as a comma-delimited streaming writer.
 * @param file {fs.File} an open write- or append-mode file handle
 * @return {Writer} the writer
 */
export func writer(file as fs.File) {
    return writerWith($file, ",");
}

/**
 * Wrap an open write/append-mode `fs.File` as a streaming writer with a custom
 * single-character delimiter.
 * @param file {fs.File} an open write- or append-mode file handle
 * @param delim {string} the single-character field delimiter
 * @return {Writer} the writer
 * @throws {Error} kind `"csv"` on a bad delimiter
 */
export func writerWith(file as fs.File, delim as string) {
    checkChars($delim, '"');
    def state as channel of WriteState init channel.make(1);
    channel.send($state, WriteState{width: 0, rows: 0});
    return Writer{file: $file, delim: $delim, state: $state};
}

/**
 * Append one record to the writer: the fields, quoted as needed, followed by
 * CRLF. A record that is one empty field is written `""`.
 * @param writer {Writer} the writer
 * @param fields {list of string} the record's fields
 * @throws {Error} kind `"csv"` when the record has no fields or not as many as
 *   the first; nothing is written then
 */
export func writeRow(writer as Writer, fields as list of string) {
    def ws as WriteState init channel.recv($writer.state);
    def text as string init "";
    try {
        $text = formatRecord($fields, dialect($writer.delim), $ws.width, $ws.rows + 1) + "\r\n";
        fs.writeString($writer.file, $text);
    } catch (e) {
        channel.send($writer.state, $ws);
        throw $e;
    }
    $ws.rows = $ws.rows + 1;
    if ($ws.width == 0) {
        $ws.width = len($fields);
    }
    channel.send($writer.state, $ws);
    return null;
}

/**
 * Close a streaming writer (closes the underlying file handle).
 * @param writer {Writer} the writer
 */
export func closeWriter(writer as Writer) {
    fs.close($writer.file);
    return null;
}
