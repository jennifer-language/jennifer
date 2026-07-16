# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

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
 * Join rows into standard comma-delimited CSV.
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
