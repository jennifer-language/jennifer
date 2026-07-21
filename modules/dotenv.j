# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Read `.env` configuration files - the `KEY=VALUE` lines that keep secrets and
 * settings out of source. `parse` turns text into a `map of string to string`,
 * `read` parses a file, and `load` parses a file and sets each variable in the
 * process environment (via `os.setEnv`). Handles `#` comments (whole-line and
 * inline), blank lines, a leading `export`, and single- / double-quoted values
 * (double quotes expand `\n` / `\t` escapes; single quotes are literal). Over
 * `fs` + `strings` + `os`; pure `.j`, both binaries.
 * @module dotenv
 * @example
 * def cfg as map of string to string init dotenv.parse("PORT=8080\n# note\nNAME=\"ada\"");
 * io.printf("%s\n", $cfg["PORT"]);          # 8080
 * dotenv.load(".env");                       # or set them in the environment
 */
use fs;
use strings;
use os;

# --- value parsing (private) ------------------------------------------------

# unescape maps the character after a backslash (inside a double-quoted value) to
# its literal; an unknown escape keeps the character as-is.
func unescape(c as string) {
    if ($c == "n") {
        return "\n";
    }
    if ($c == "t") {
        return "\t";
    }
    if ($c == "r") {
        return "\r";
    }
    return $c;
}

# unquoteDouble reads a double-quoted value from index 1, expanding escapes, and
# stops at the first unescaped closing quote.
func unquoteDouble(v as string) {
    def out as string init "";
    def i as int init 1;
    def n as int init len($v);
    while ($i < $n) {
        def ch as string init strings.substring($v, $i, $i + 1);
        if ($ch == "\"") {
            return $out;
        }
        if ($ch == "\\" and $i + 1 < $n) {
            $out = $out + unescape(strings.substring($v, $i + 1, $i + 2));
            $i = $i + 2;
        } else {
            $out = $out + $ch;
            $i = $i + 1;
        }
    }
    return $out;
}

# unquoteSingle reads a single-quoted value literally (no escapes) up to the next
# quote.
func unquoteSingle(v as string) {
    def rest as string init strings.substring($v, 1, len($v));
    def close as int init strings.indexOf($rest, "'");
    if ($close < 0) {
        return $rest;
    }
    return strings.substring($rest, 0, $close);
}

# stripInlineComment drops an unquoted value's trailing ` #...` comment.
func stripInlineComment(v as string) {
    def idx as int init strings.indexOf($v, " #");
    if ($idx < 0) {
        return $v;
    }
    return strings.trim(strings.substring($v, 0, $idx));
}

# parseValue turns the text after `=` into the value: single- or double-quoted,
# or a bare token with any inline comment stripped.
func parseValue(raw as string) {
    def v as string init strings.trim($raw);
    if (len($v) == 0) {
        return "";
    }
    def first as string init strings.substring($v, 0, 1);
    if ($first == "\"") {
        return unquoteDouble($v);
    }
    if ($first == "'") {
        return unquoteSingle($v);
    }
    return stripInlineComment($v);
}

# --- API (exported) ---------------------------------------------------------

/**
 * Parse `.env` text into a map. Blank lines and `#` comment lines are skipped, a
 * leading `export` is stripped, and each `KEY=VALUE` becomes an entry (later
 * duplicates win). A line with no `=` or an empty key is ignored.
 * @param text {string} the `.env` file contents
 * @return {map of string to string} the parsed variables
 */
export func parse(text as string) {
    def out as map of string to string init {};
    def lines as list of string init strings.split(strings.replace($text, "\r", ""), "\n");
    for (def raw in $lines) {
        def line as string init strings.trim($raw);
        if (len($line) == 0 or strings.startsWith($line, "#")) {
            continue;
        }
        if (strings.startsWith($line, "export ")) {
            $line = strings.trim(strings.substring($line, 7, len($line)));
        }
        def eq as int init strings.indexOf($line, "=");
        if ($eq < 0) {
            continue;
        }
        def key as string init strings.trim(strings.substring($line, 0, $eq));
        if (len($key) == 0) {
            continue;
        }
        $out[$key] = parseValue(strings.substring($line, $eq + 1, len($line)));
    }
    return $out;
}

/**
 * Read and parse a `.env` file, without touching the environment.
 * @param path {string} the file path
 * @return {map of string to string} the parsed variables
 * @throws {Error} on a filesystem error (a positioned `fs` error)
 */
export func read(path as string) {
    return parse(fs.readString($path));
}

/**
 * Read a `.env` file and set each variable in the process environment (via
 * `os.setEnv`), returning the parsed map.
 * @param path {string} the file path
 * @return {map of string to string} the variables that were set
 * @throws {Error} on a filesystem error, or an invalid variable name
 */
export func load(path as string) {
    def vars as map of string to string init read($path);
    for (def key in $vars) {
        os.setEnv($key, $vars[$key]);
    }
    return $vars;
}
