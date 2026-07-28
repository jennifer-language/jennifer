# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Read `.env` configuration files - the `KEY=VALUE` lines that keep secrets and
 * settings out of source - with layered profiles and `${VAR}` interpolation.
 *
 * Single-file primitives: `parse` turns text into a `map of string to string`,
 * `read` parses a file, and `load` parses a file and sets each variable in the
 * process environment (via `os.setEnv`, unconditional override). Layered loaders:
 * `readCascade` / `resolve` / `loadCascade` / `autoload` merge
 * `.env` -> `.env.local` -> `.env.<profile>` -> `.env.<profile>.local` from one
 * explicit directory, later overriding earlier, with a **real OS env var always
 * winning** over a file value (a committed file cannot clobber a deployment
 * secret). The profile comes from `JENNIFER_ENV` (empty = base files only).
 *
 * Values handle `#` comments (whole-line and inline on unquoted values), blank
 * lines, a leading `export`, single-quoted (fully literal) and double-quoted
 * (expand `\n` / `\t` / `\r`, may span multiple physical lines) values, and
 * `${VAR}` interpolation in unquoted and double-quoted values (backward-reference
 * only: earlier keys -> real OS env -> empty; single quotes never interpolate;
 * no `$(...)` / backtick command substitution). Over `fs` + `strings` + `os` +
 * `path` + `regex` + `maps`; pure `.j`, both binaries.
 * @module dotenv
 * @example
 * def cfg as map of string to string init dotenv.parse("PORT=8080\nURL=\"http://h:${PORT}\"");
 * io.printf("%s\n", $cfg["URL"]);            # http://h:8080
 * dotenv.autoload(os.cwd());                  # layered load, real env wins
 */
use fs;
use strings;
use os;
use convert;
use path;
use regex;
use maps;

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

# closingDoubleIndex returns the index of the first unescaped closing `"` in a
# value that opens with `"` at index 0 (scanning from index 1), or -1 if the
# quote is not closed - the signal that a double-quoted value spans more physical
# lines. A `\"` is an escaped quote, not a close.
func closingDoubleIndex(s as string) {
    def i as int init 1;
    def n as int init len($s);
    while ($i < $n) {
        def ch as string init strings.substring($s, $i, $i + 1);
        if ($ch == "\\" and $i + 1 < $n) {
            $i = $i + 2;
            continue;
        }
        if ($ch == "\"") {
            return $i;
        }
        $i = $i + 1;
    }
    return -1;
}

# unquoteDouble reads a double-quoted value from index 1, expanding escapes, and
# stops at the first unescaped closing quote. Embedded real newlines (a multi-line
# value) are ordinary characters and pass through.
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

# unquoteSingle reads a single-quoted value literally (no escapes, no
# interpolation) up to the next quote.
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

# indexOfFrom finds sub in s at or after index `from`, or -1.
func indexOfFrom(s as string, sub as string, from as int) {
    def rest as string init strings.substring($s, $from, len($s));
    def idx as int init strings.indexOf($rest, $sub);
    if ($idx < 0) {
        return -1;
    }
    return $from + $idx;
}

# resolveVar looks a `${VAR}` reference up: an earlier-parsed key wins, then the
# real OS environment, then the empty string (os.getEnv returns "" when unset).
func resolveVar(name as string, acc as map of string to string) {
    if (maps.has($acc, $name)) {
        return $acc[$name];
    }
    return os.getEnv($name);
}

# interpolate expands `${VAR}` references in a value. VAR must be a valid env name
# (a malformed `${...}` is kept literal). Backward-reference only - it resolves
# against `acc` (the keys parsed so far, this file plus earlier cascade files) and
# the real OS env, so a reference to a not-yet-defined key yields "" and cycles are
# impossible. A lone `$` (or `$(` / backtick) is a plain character: there is no
# command substitution, by design.
func interpolate(value as string, acc as map of string to string) {
    def out as string init "";
    def i as int init 0;
    def n as int init len($value);
    while ($i < $n) {
        def ch as string init strings.substring($value, $i, $i + 1);
        if ($ch == "$" and $i + 1 < $n and strings.substring($value, $i + 1, $i + 2) == "{") {
            def close as int init indexOfFrom($value, "}", $i + 2);
            if ($close < 0) {
                $out = $out + $ch;
                $i = $i + 1;
                continue;
            }
            def name as string init strings.substring($value, $i + 2, $close);
            if (validEnvName($name)) {
                $out = $out + resolveVar($name, $acc);
            } else {
                # Not a valid reference - keep the whole `${...}` literal.
                $out = $out + strings.substring($value, $i, $close + 1);
            }
            $i = $close + 1;
            continue;
        }
        $out = $out + $ch;
        $i = $i + 1;
    }
    return $out;
}

# parseValueInterp turns the (already-trimmed) text after `=` into the value:
# double-quoted (escapes + interpolation), single-quoted (fully literal), or a
# bare token (inline comment stripped, then interpolation).
func parseValueInterp(v as string, acc as map of string to string) {
    if (len($v) == 0) {
        return "";
    }
    def first as string init strings.substring($v, 0, 1);
    if ($first == "\"") {
        return interpolate(unquoteDouble($v), $acc);
    }
    if ($first == "'") {
        return unquoteSingle($v);
    }
    return interpolate(stripInlineComment($v), $acc);
}

# parseValue is the back-compatible single-value parser: trim, then parse with no
# interpolation base (a `${VAR}` still consults the real OS env).
func parseValue(raw as string) {
    def empty as map of string to string init {};
    return parseValueInterp(strings.trim($raw), $empty);
}

# validEnvName reports whether name is a POSIX-shaped environment variable name:
# a letter or `_`, then letters, digits, or `_`. A malformed key reaching
# os.setEnv could set the process environment in surprising ways (OM-021).
func validEnvName(name as string) {
    def raw as bytes init convert.bytesFromString($name, "utf-8");
    if (len($raw) == 0) {
        return false;
    }
    def i as int init 0;
    while ($i < len($raw)) {
        def b as int init $raw[$i];
        def alpha as bool init ($b >= 65 and $b <= 90) or ($b >= 97 and $b <= 122) or $b == 95;
        def digit as bool init $b >= 48 and $b <= 57;
        if ($i == 0) {
            if (not $alpha) {
                return false;
            }
        } else {
            if (not ($alpha or $digit)) {
                return false;
            }
        }
        $i = $i + 1;
    }
    return true;
}

# validProfile reports whether a profile label is safe to splice into a
# `.env.<profile>` filename. The strict shape (letters, digits, `_`, `-`, 1-64
# chars) blocks path traversal - a `JENNIFER_ENV` from an untrusted upstream
# cannot become `../../etc/x` or `prod/../y`. path.base is not a sanitizer, so
# this is validated up front.
func validProfile(name as string) {
    return regex.matches("^[A-Za-z0-9_-]{1,64}$", $name);
}

# --- single-file API (exported) ---------------------------------------------

/**
 * Parse `.env` text into a map. Blank lines and `#` comment lines are skipped, a
 * leading `export` is stripped, and each `KEY=VALUE` becomes an entry (later
 * duplicates win). Double-quoted values may span multiple physical lines;
 * `${VAR}` references are interpolated (earlier keys -> real OS env -> ""). A
 * line with no `=` or an empty key is ignored.
 * @param text {string} the `.env` file contents
 * @return {map of string to string} the parsed variables
 * @throws {Error} kind "dotenv" on an unterminated multi-line double-quoted value
 */
export func parse(text as string) {
    def empty as map of string to string init {};
    return parseWithBase($text, $empty);
}

# parseWithBase is parse with a starting map (earlier cascade files), so a
# `${VAR}` in a later file resolves against the keys already loaded. The returned
# map is `base` overlaid by this text's keys (later wins). Interpolation is
# backward-reference: each value sees only the keys parsed before it.
func parseWithBase(text as string, base as map of string to string) {
    def out as map of string to string init $base;
    def clean as string init strings.replace($text, "\r", "");
    def lines as list of string init strings.split($clean, "\n");
    def i as int init 0;
    def n as int init len($lines);
    while ($i < $n) {
        def line as string init strings.trim($lines[$i]);
        if (len($line) == 0 or strings.startsWith($line, "#")) {
            $i = $i + 1;
            continue;
        }
        if (strings.startsWith($line, "export ")) {
            $line = strings.trim(strings.substring($line, 7, len($line)));
        }
        def eq as int init strings.indexOf($line, "=");
        if ($eq < 0) {
            $i = $i + 1;
            continue;
        }
        def key as string init strings.trim(strings.substring($line, 0, $eq));
        if (len($key) == 0) {
            $i = $i + 1;
            continue;
        }
        def tv as string init strings.trim(strings.substring($line, $eq + 1, len($line)));
        # A double-quoted value with no closing quote on this line spans further
        # physical lines: accumulate them (real newlines and all) until the close.
        if (len($tv) > 0 and strings.startsWith($tv, "\"") and closingDoubleIndex($tv) < 0) {
            def acc as string init $tv;
            def closed as bool init false;
            def j as int init $i + 1;
            while ($j < $n) {
                $acc = $acc + "\n" + $lines[$j];
                if (closingDoubleIndex($acc) >= 0) {
                    $closed = true;
                    $i = $j;
                    break;
                }
                $j = $j + 1;
            }
            if (not $closed) {
                throw Error{
                    kind: "dotenv",
                    message: "dotenv: unterminated double-quoted value for key `" + $key + "`",
                    file: "",
                    line: $i + 1,
                    col: 0
                };
            }
            $out[$key] = interpolate(unquoteDouble($acc), $out);
            $i = $i + 1;
            continue;
        }
        $out[$key] = parseValueInterp($tv, $out);
        $i = $i + 1;
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
 * `os.setEnv`, **unconditional override**), returning the parsed map. This is the
 * low-level primitive; for a real-env-wins layered load use `loadCascade` /
 * `autoload`.
 * @param path {string} the file path
 * @return {map of string to string} the variables that were set
 * @throws {Error} on a filesystem error, or an invalid variable name
 */
export func load(path as string) {
    def vars as map of string to string init read($path);
    for (def key in $vars) {
        if (not validEnvName($key)) {
            throw Error{
                kind: "dotenv",
                message: "dotenv.load: invalid environment variable name (must be letters/digits/`_`, not starting with a digit): " +
                    $key,
                file: "",
                line: 0,
                col: 0
            };
        }
        os.setEnv($key, $vars[$key]);
    }
    return $vars;
}

# --- layered / cascade API (exported) ---------------------------------------

# cascadeFiles is the ordered layer list for a profile (later overrides earlier):
# base files always, then the two profile files when a profile is set.
func cascadeFiles(profile as string) {
    def files as list of string init [".env", ".env.local"];
    if (len($profile) > 0) {
        $files[] = ".env." + $profile;
        $files[] = ".env." + $profile + ".local";
    }
    return $files;
}

/**
 * Merge the `.env` layers from one directory into a map, **without touching the
 * environment**. Reads, later overriding earlier and skipping absent files:
 * `.env` -> `.env.local` -> `.env.<profile>` -> `.env.<profile>.local`. A `${VAR}`
 * in a later file resolves against keys from earlier files. An empty `profile`
 * loads only the base files (there is no `.env.default`).
 * @param dir {string} the single base directory (no search / no walk-up)
 * @param profile {string} the profile label, or "" for base files only
 * @return {map of string to string} the merged file variables
 * @throws {Error} kind "dotenv" on an invalid profile label; an absent layer is
 *   skipped (not an error) but an unreadable one raises the `fs` error
 */
export func readCascade(dir as string, profile as string) {
    if (len($profile) > 0 and not validProfile($profile)) {
        throw Error{
            kind: "dotenv",
            message: "dotenv: invalid profile name (must match [A-Za-z0-9_-] and be 1-64 chars): " +
                $profile,
            file: "",
            line: 0,
            col: 0
        };
    }
    def acc as map of string to string init {};
    for (def name in cascadeFiles($profile)) {
        def p as string init path.join($dir, $name);
        if (fs.exists($p)) {
            $acc = parseWithBase(fs.readString($p), $acc);
        }
    }
    return $acc;
}

/**
 * The **effective** configuration map: `readCascade` overlaid by the real OS
 * environment, so a real env var always wins over a file value. The result holds
 * exactly the file map's keys (each taking the OS-env value when that variable is
 * set), for a program that reads a config map instead of calling `os.getEnv`.
 * Touches nothing in the environment. A variable set to the **empty string** in
 * the environment counts as unset (there is no `os.hasEnv`), so a file value
 * still fills it.
 * @param dir {string} the base directory
 * @param profile {string} the profile label, or "" for base files only
 * @return {map of string to string} the effective values (real env wins)
 */
export func resolve(dir as string, profile as string) {
    def m as map of string to string init readCascade($dir, $profile);
    def out as map of string to string init {};
    for (def key in $m) {
        def real as string init os.getEnv($key);
        if (len($real) > 0) {
            $out[$key] = $real;
        } else {
            $out[$key] = $m[$key];
        }
    }
    return $out;
}

/**
 * Merge the `.env` layers (`readCascade`) then `os.setEnv` each variable **only
 * when it is not already set** in the real environment - a real env var is never
 * clobbered by a committed file. A variable set to the **empty string** counts as
 * unset (there is no `os.hasEnv`), so a file value still fills it. Returns the
 * file map (all keys, whether or not they were set). To force-override instead,
 * use `readCascade` + your own `os.setEnv` loop.
 * @param dir {string} the base directory
 * @param profile {string} the profile label, or "" for base files only
 * @return {map of string to string} the merged file variables
 * @throws {Error} kind "dotenv" on an invalid profile label or variable name
 */
export func loadCascade(dir as string, profile as string) {
    def m as map of string to string init readCascade($dir, $profile);
    for (def key in $m) {
        if (not validEnvName($key)) {
            throw Error{
                kind: "dotenv",
                message: "dotenv.loadCascade: invalid environment variable name (must be letters/digits/`_`, not starting with a digit): " +
                    $key,
                file: "",
                line: 0,
                col: 0
            };
        }
        if (len(os.getEnv($key)) == 0) {
            os.setEnv($key, $m[$key]);
        }
    }
    return $m;
}

/**
 * Convenience: `loadCascade(dir, JENNIFER_ENV)` - the layered, real-env-wins load
 * with the profile taken from the `JENNIFER_ENV` environment variable (empty =
 * base files only). The one env var this module reads to pick a profile.
 * @param dir {string} the base directory (usually `os.cwd()`)
 * @return {map of string to string} the merged file variables
 * @throws {Error} kind "dotenv" on an invalid profile label or variable name
 */
export func autoload(dir as string) {
    return loadCascade($dir, os.getEnv("JENNIFER_ENV"));
}
