# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
# pragma-jennifer-version: >=0.24.0

# A hand-rolled quoting-aware scanner: its parse core legitimately runs past the
# L201 statement-count limit. Every other lint check stays active.
# lint-disable-file: L201

/**
 * RFC 4180 comma-separated values: parse text into rows of fields and format
 * rows back into text, with a quoting-aware hand-written scanner. Pure Jennifer
 * - no Go, no system library. The delimiter is configurable, so the same code
 * reads and writes TSV and other single-character-separated formats. Records
 * separate on LF or CRLF (a bare CR outside quotes also ends a record); a field
 * is quoted with `"` when it contains the delimiter, a quote, or a newline, and
 * an embedded quote doubles to `""`. format() joins records with LF and adds no
 * trailing newline, so parse(format(rows)) round-trips the data.
 * @module csv
 * @example
 * import "csv.j" as csv;
 * def rows as list of list of string init csv.parse("a,b\n1,\"x,y\"");
 * def recs as list of map of string to string init csv.toRecords($rows);
 */
use strings;
use maps;
use fs;

# --- quoting helpers (private) -------------------------------------

# needsQuote reports whether a field must be wrapped in quotes: it carries the
# delimiter, a quote, or a line break.
func needsQuote(f as string, delim as string) {
    if (strings.contains($f, $delim)) {
        return true;
    }
    if (strings.contains($f, '"')) {
        return true;
    }
    if (strings.contains($f, "\n") or strings.contains($f, "\r")) {
        return true;
    }
    return false;
}

# quoteField returns a field ready to write: wrapped in quotes with any
# embedded quote doubled when quoting is required, otherwise unchanged.
func quoteField(f as string, delim as string) {
    if (needsQuote($f, $delim)) {
        return '"' + strings.replace($f, '"', '""') + '"';
    }
    return $f;
}

# --- parse (exported) ----------------------------------------------

/**
 * Scan CSV text with a single-character delimiter and return its rows, each a
 * list of string fields. A quoted field may span the delimiter, newlines, and
 * doubled quotes; an empty input yields no rows.
 * @param s {string} the CSV text to parse
 * @param delim {string} the single-character field delimiter
 * @return {list of list of string} the parsed rows of fields
 */
export func parseWith(s as string, delim as string) {
    def rows as list of list of string init [];
    def row as list of string init [];
    # Accumulate a field's characters in a list and join once at each flush: an
    # accumulating `+` would be O(field_len^2) on a large quoted field.
    def field as list of string init [];
    def inQuotes as bool init false;
    def fieldStarted as bool init false;
    def rowStarted as bool init false;
    def chars as list of string init strings.chars($s);
    def n as int init len($chars);
    def i as int init 0;
    # Flat guard-and-continue dispatch keeps the scanner readable and shallow.
    while ($i < $n) {
        def c as string init $chars[$i];
        # Inside a quoted field: a doubled quote is a literal, a lone quote
        # closes, anything else is content (delimiters and newlines included).
        if ($inQuotes and $c == '"' and $i + 1 < $n and $chars[$i + 1] == '"') {
            $field[] = '"';
            $i = $i + 2;
            continue;
        }
        if ($inQuotes and $c == '"') {
            $inQuotes = false;
            $i = $i + 1;
            continue;
        }
        if ($inQuotes) {
            $field[] = $c;
            $i = $i + 1;
            continue;
        }
        # Outside quotes.
        if ($c == '"') {
            $inQuotes = true;
            $fieldStarted = true;
            $rowStarted = true;
            $i = $i + 1;
            continue;
        }
        if ($c == $delim) {
            $row[] = strings.join($field, "");
            $field = [];
            $fieldStarted = false;
            $rowStarted = true;
            $i = $i + 1;
            continue;
        }
        if ($c == "\n" or $c == "\r") {
            $row[] = strings.join($field, "");
            $rows[] = $row;
            $field = [];
            $row = [];
            $fieldStarted = false;
            $rowStarted = false;
            # Consume the LF of a CRLF pair as one record separator.
            if ($c == "\r" and $i + 1 < $n and $chars[$i + 1] == "\n") {
                $i = $i + 2;
            } else {
                $i = $i + 1;
            }
            continue;
        }
        $field[] = $c;
        $fieldStarted = true;
        $rowStarted = true;
        $i = $i + 1;
    }
    # Flush the final record unless the text ended exactly on a separator.
    if ($rowStarted or $fieldStarted) {
        $row[] = strings.join($field, "");
        $rows[] = $row;
    }
    return $rows;
}

/**
 * Scan standard comma-delimited CSV.
 * @param s {string} the CSV text to parse
 * @return {list of list of string} the parsed rows of fields
 */
export func parse(s as string) {
    return parseWith($s, ",");
}

# --- format (exported) ---------------------------------------------

/**
 * Join rows into text with a single-character delimiter, quoting each field as
 * needed. Records are separated by LF with no trailing newline.
 *
 * **Not safe for a spreadsheet target.** A field beginning `= + - @` (or a tab /
 * CR) is a formula that Excel / Google Sheets will execute on open (CWE-1236). If
 * the output is opened as a spreadsheet, use `formatSafe` / `formatSafeWith`,
 * which neutralise such fields.
 * @param rows {list of list of string} the rows of fields to format
 * @param delim {string} the single-character field delimiter
 * @return {string} the formatted delimiter-separated text
 */
export func formatWith(rows as list of list of string, delim as string) {
    # Build each row's fields and each row into lists, joining once per level:
    # an accumulating `+` over a large table would be O(N^2) in the output size.
    def lines as list of string init [];
    for (def row in $rows) {
        def fields as list of string init [];
        for (def field in $row) {
            $fields[] = quoteField($field, $delim);
        }
        $lines[] = strings.join($fields, $delim);
    }
    return strings.join($lines, "\n");
}

/**
 * Join rows into standard comma-delimited CSV. **Not safe for a spreadsheet
 * target** - use `formatSafe` when the output may be opened in Excel / Sheets
 * (see `formatWith` on the CWE-1236 formula-injection risk).
 * @param rows {list of list of string} the rows of fields to format
 * @return {string} the formatted comma-separated text
 */
export func format(rows as list of list of string) {
    return formatWith($rows, ",");
}

# --- header records (exported) -------------------------------------

/**
 * Treat the first row as a header and map each later row into a
 * `map of string to string` keyed by the header names. Every record carries
 * every header key (a short row fills missing fields with ""); fields past the
 * header width are dropped. An empty input yields no records.
 * @param rows {list of list of string} the rows, with the first as the header
 * @return {list of map of string to string} one record per non-header row
 */
export func toRecords(rows as list of list of string) {
    def records as list of map of string to string init [];
    if (len($rows) == 0) {
        return $records;
    }
    def header as list of string init $rows[0];
    def r as int init 1;
    while ($r < len($rows)) {
        def row as list of string init $rows[$r];
        def rec as map of string to string init {};
        def c as int init 0;
        while ($c < len($header)) {
            if ($c < len($row)) {
                $rec[$header[$c]] = $row[$c];
            } else {
                $rec[$header[$c]] = "";
            }
            $c = $c + 1;
        }
        $records[] = $rec;
        $r = $r + 1;
    }
    return $records;
}

/**
 * The inverse of toRecords: emit the header row followed by one row per record,
 * taking fields in header order (a key absent from a record writes ""). The
 * explicit header fixes the column order, which map iteration does not.
 * @param header {list of string} the column names, in output order
 * @param records {list of map of string to string} the records to emit
 * @return {list of list of string} the header row followed by one row per record
 */
export func fromRecords(header as list of string, records as list of map of string to string) {
    def rows as list of list of string init [];
    $rows[] = $header;
    for (def rec in $records) {
        def row as list of string init [];
        for (def col in $header) {
            if (maps.has($rec, $col)) {
                $row[] = $rec[$col];
            } else {
                $row[] = "";
            }
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
    def first as string init strings.substring($s, 0, 1);
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
 * on parse; "" disables comments), and whether unquoted fields are whitespace-
 * trimmed on parse. Build one with `dialect(delimiter)` for the common defaults.
 * @field delimiter {string} the single-character field delimiter
 * @field quote {string} the quote character (default `"`)
 * @field comment {string} the comment-line prefix, or "" for none
 * @field trim {bool} trim leading/trailing whitespace from unquoted fields
 */
export def struct Dialect {
    delimiter as string,
    quote as string,
    comment as string,
    trim as bool
};

/**
 * A Dialect with the given delimiter and the standard defaults: quote `"`, no
 * comment prefix, no trimming. Set the other fields to customise.
 * @param delimiter {string} the single-character field delimiter
 * @return {Dialect} the dialect
 */
export func dialect(delimiter as string) {
    return Dialect{delimiter: $delimiter, quote: '"', comment: "", trim: false};
}

# startsAt reports whether `chars` matches `prefix` starting at index i.
func startsAt(chars as list of string, i as int, prefix as string, n as int) {
    def p as list of string init strings.chars($prefix);
    def m as int init len($p);
    if ($m == 0 or $i + $m > $n) {
        return false;
    }
    def k as int init 0;
    while ($k < $m) {
        if ($chars[$i + $k] != $p[$k]) {
            return false;
        }
        $k = $k + 1;
    }
    return true;
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

# parseCore is the dialect-aware scanner: parametrised on delimiter, quote,
# comment prefix, and trimming. It mirrors parseWith's structure with the extra
# dialect handling.
func parseCore(s as string, delim as string, quote as string, comment as string, trim as bool) {
    def rows as list of list of string init [];
    def row as list of string init [];
    def field as list of string init [];
    def inQuotes as bool init false;
    def fieldStarted as bool init false;
    def rowStarted as bool init false;
    def fieldQuoted as bool init false;
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
            if ($i < $n and $chars[$i] == "\r" and $i + 1 < $n and $chars[$i + 1] == "\n") {
                $i = $i + 2;
            } elseif ($i < $n) {
                $i = $i + 1;
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
            $i = $i + 1;
            continue;
        }
        if ($inQuotes) {
            $field[] = $c;
            $i = $i + 1;
            continue;
        }
        if ($c == $quote) {
            $inQuotes = true;
            $fieldStarted = true;
            $rowStarted = true;
            $fieldQuoted = true;
            $i = $i + 1;
            continue;
        }
        if ($c == $delim) {
            $row[] = flushField($field, $trim, $fieldQuoted);
            $field = [];
            $fieldStarted = false;
            $rowStarted = true;
            $fieldQuoted = false;
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
            if ($c == "\r" and $i + 1 < $n and $chars[$i + 1] == "\n") {
                $i = $i + 2;
            } else {
                $i = $i + 1;
            }
            continue;
        }
        $field[] = $c;
        $fieldStarted = true;
        $rowStarted = true;
        $i = $i + 1;
    }
    if ($rowStarted or $fieldStarted) {
        $row[] = flushField($field, $trim, $fieldQuoted);
        $rows[] = $row;
    }
    return $rows;
}

/**
 * Parse CSV text under a Dialect: custom delimiter and quote, comment-line
 * skipping, and optional unquoted-field trimming. Quoted fields keep their
 * content (including whitespace and embedded newlines) verbatim.
 * @param text {string} the CSV text to parse
 * @param d {Dialect} the dialect controlling delimiter/quote/comment/trim
 * @return {list of list of string} the parsed rows of fields
 */
export func parseDialect(text as string, d as Dialect) {
    return parseCore($text, $d.delimiter, $d.quote, $d.comment, $d.trim);
}

# needsQuoteQ / quoteFieldQ are the dialect-quote-aware counterparts of the
# private needsQuote / quoteField helpers.
func needsQuoteQ(f as string, delim as string, quote as string) {
    if (strings.contains($f, $delim)) {
        return true;
    }
    if (strings.contains($f, $quote)) {
        return true;
    }
    if (strings.contains($f, "\n") or strings.contains($f, "\r")) {
        return true;
    }
    return false;
}

func quoteFieldQ(f as string, delim as string, quote as string) {
    if (needsQuoteQ($f, $delim, $quote)) {
        return $quote + strings.replace($f, $quote, $quote + $quote) + $quote;
    }
    return $f;
}

/**
 * Format rows under a Dialect (custom delimiter and quote character). Records
 * are joined with LF and no trailing newline, like formatWith. The comment and
 * trim fields are parse-only and ignored here.
 * @param rows {list of list of string} the rows of fields to format
 * @param d {Dialect} the dialect controlling delimiter/quote
 * @return {string} the formatted text
 */
export func formatDialect(rows as list of list of string, d as Dialect) {
    def lines as list of string init [];
    for (def row in $rows) {
        def fields as list of string init [];
        for (def field in $row) {
            $fields[] = quoteFieldQ($field, $d.delimiter, $d.quote);
        }
        $lines[] = strings.join($fields, $d.delimiter);
    }
    return strings.join($lines, "\n");
}

# --- streaming handles (exported) ----------------------------------

/**
 * A streaming CSV reader over an open `fs.File`, for tables too large to hold in
 * memory. The wrapped file is a handle - it shares its read position across
 * value copies - so successive `readRow` calls advance the same stream.
 * @field file {fs.File} the underlying open file handle
 * @field delim {string} the single-character field delimiter
 */
export def struct Reader {
    file as fs.File,
    delim as string
};

/**
 * Wrap an open, read-mode `fs.File` as a comma-delimited streaming reader.
 * @param file {fs.File} an open read-mode file handle
 * @return {Reader} the reader
 */
export func reader(file as fs.File) {
    return Reader{file: $file, delim: ","};
}

/**
 * Wrap an open, read-mode `fs.File` as a streaming reader with a custom
 * single-character delimiter.
 * @param file {fs.File} an open read-mode file handle
 * @param delim {string} the single-character field delimiter
 * @return {Reader} the reader
 */
export func readerWith(file as fs.File, delim as string) {
    return Reader{file: $file, delim: $delim};
}

# quoteCount returns the number of `"` characters in a string, used to detect a
# record that is still open inside a quoted field across physical lines.
func quoteCount(s as string) {
    return len(strings.split($s, '"')) - 1;
}

/**
 * Read one complete CSV record from the reader, returning its fields. A record
 * whose quoted field spans physical lines is read to completion (further lines
 * are pulled until the quotes balance). Returns the empty list `[]` at
 * end-of-file (guard the loop with `readerEof`) and for a blank line.
 * @param reader {Reader} the reader
 * @return {list of string} the record's fields, or `[]` at EOF / on a blank line
 */
export func readRow(reader as Reader) {
    if (fs.eof($reader.file)) {
        return [];
    }
    def first as string init fs.readLine($reader.file);
    def lines as list of string init [$first];
    # An odd running `"` count means we are still inside a quoted field (a doubled
    # quote "" stays even). Count only each newly-read line's quotes into a running
    # total and accumulate the lines, then join once - so assembling a field that
    # spans many physical lines stays O(field size), not O(field size^2) from
    # re-scanning / re-concatenating the whole growing buffer each iteration.
    def quotes as int init quoteCount($first);
    while (($quotes % 2) == 1 and not fs.eof($reader.file)) {
        def next as string init fs.readLine($reader.file);
        $lines[] = $next;
        $quotes = $quotes + quoteCount($next);
    }
    def buffer as string init strings.join($lines, "\n");
    def rows as list of list of string init parseWith($buffer, $reader.delim);
    if (len($rows) == 0) {
        return [];
    }
    return $rows[0];
}

/**
 * Whether the reader has reached end-of-file. Loop `while (not readerEof($r))`.
 * @param reader {Reader} the reader
 * @return {bool} true once no further record remains
 */
export func readerEof(reader as Reader) {
    return fs.eof($reader.file);
}

/**
 * Close a streaming reader (closes the underlying file handle).
 * @param reader {Reader} the reader
 */
export func closeReader(reader as Reader) {
    fs.close($reader.file);
    return null;
}

/**
 * A streaming CSV writer over an open `fs.File`. Each `writeRow` call appends one
 * quoted-as-needed record terminated by LF, so a large table is written without
 * building the whole text in memory.
 * @field file {fs.File} the underlying open write/append-mode file handle
 * @field delim {string} the single-character field delimiter
 */
export def struct Writer {
    file as fs.File,
    delim as string
};

/**
 * Wrap an open write/append-mode `fs.File` as a comma-delimited streaming writer.
 * @param file {fs.File} an open write- or append-mode file handle
 * @return {Writer} the writer
 */
export func writer(file as fs.File) {
    return Writer{file: $file, delim: ","};
}

/**
 * Wrap an open write/append-mode `fs.File` as a streaming writer with a custom
 * single-character delimiter.
 * @param file {fs.File} an open write- or append-mode file handle
 * @param delim {string} the single-character field delimiter
 * @return {Writer} the writer
 */
export func writerWith(file as fs.File, delim as string) {
    return Writer{file: $file, delim: $delim};
}

/**
 * Append one record to the writer: the fields, quoted as needed, followed by LF.
 * @param writer {Writer} the writer
 * @param fields {list of string} the record's fields
 */
export func writeRow(writer as Writer, fields as list of string) {
    def one as list of list of string init [];
    $one[] = $fields;
    fs.writeString($writer.file, formatWith($one, $writer.delim) + "\n");
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
