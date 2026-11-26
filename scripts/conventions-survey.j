#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

/**
 * The conventions survey (roadmap M30.28, was M29.23): list every place where a
 * built-in library, a bundled module or a deck departs from a mechanically
 * checkable rule of docs/technical/conventions.md (stance 8).
 *
 * The built-in libraries are read from their signature tables in
 * internal/libsig (names and types, no parameter names); the modules and decks
 * through the docblock module, which gives parameter names and documented
 * return types. A deck is read from its src/ tree, topics included.
 *
 * Run from the repository root; each argument is a deck checkout to survey too:
 *   jennifer run scripts/conventions-survey.j [deck-dir...]
 *
 * It prints the findings as Markdown, one section per rule, and exits 0: the
 * list is a worklist, not a gate. A rule that becomes a gate moves into a test.
 */

use io;
use fs;
use path;
use strings;
use lists;
use maps;
use regex;
use os;
use convert;

import "../modules/docblock.j" as docblock;

def const LIBSIG as string init "internal/libsig";
def const MODDIR as string init "modules";

# Reserved words and type keywords: a library function may not be spelled
# like one, even where `alias.name` would parse.
def const KEYWORDS as list of string init [
    "and",
    "as",
    "break",
    "catch",
    "const",
    "continue",
    "def",
    "defer",
    "else",
    "elseif",
    "enum",
    "errdefer",
    "exit",
    "export",
    "false",
    "for",
    "func",
    "if",
    "import",
    "in",
    "include",
    "init",
    "len",
    "list",
    "map",
    "match",
    "not",
    "null",
    "of",
    "or",
    "repeat",
    "return",
    "spawn",
    "struct",
    "task",
    "throw",
    "to",
    "true",
    "try",
    "until",
    "use",
    "when",
    "while",
    "int",
    "float",
    "string",
    "bool",
    "bytes",
    "channel"
];

# The kinds any library may raise for a shared situation.
def const GENERIC_KINDS as list of string init [
    "limit",
    "timeout",
    "closed",
    "cancelled",
    "net",
    "value",
    "runtime"
];

# Names that look like a convention break but are not: each with its reason.
def const EXCEPTIONS as list of string init [
    # verb phrases, not `x` + With
    "strings.startsWith",
    "strings.endsWith",
    "binary.startsWith",
    "binary.endsWith",
    # an RFC 5545 recurrence interval is a count of periods, not a duration
    "ical.rule",
    # core names spelled like a keyword, kept (design-decisions.md)
    "lists.map",
    "json.map",
    "json.list",
    "toml.map",
    "toml.list",
    "yaml.map",
    "yaml.list",
    "fs.list",
    "channel.len",
    "os.spawn",
    "asn1.null",
    # kinds outside a module's own, kept (design-decisions.md)
    "mcp:jsonrpc",
    "pop:pop3",
    "s3presign:s3"
];

# A name ending in one of these is a duration and needs its unit (Ms / Sec).
def const DURATION_WORDS as string init '(?i)(timeout|delay|interval|ttl|deadline|backoff|period|wait)$';

/**
 * One exported function of a library, module or deck.
 * @field owner {string} the library, module or deck it belongs to
 * @field name {string} the function name
 * @field params {list of string} the parameter names ("" when not known)
 * @field types {list of string} the parameter types
 * @field returns {string} the return type ("" when not documented)
 * @field deprecated {string} the deprecation note, "" for a current name
 */
def struct Export {
    owner as string,
    name as string,
    params as list of string,
    types as list of string,
    returns as string,
    deprecated as string
};

/**
 * One departure from a rule.
 * @field rule {string} the rule's heading
 * @field where {string} owner.name, or a file
 * @field detail {string} what is wrong
 */
def struct Finding {
    rule as string,
    where as string,
    detail as string
};

# ---------- reading ----------

# libsigDeprecations reads the `deprecate("lib", "name", "instead", removed)`
# calls in internal/libsig/libsig.go: "lib.name" -> the note.
func libsigDeprecations() {
    def out as map of string to string init {};
    def src as string init fs.readString(path.join(LIBSIG, "libsig.go"));
    def removed as string init "";
    def r as regex.Match init regex.find('const removed = "([0-9.]+)"', $src);
    if ($r.start >= 0) {
        $removed = $r.groups[0];
    }
    for (def m in regex.findAll(
        'deprecate\("([a-z0-9]+)", "([A-Za-z0-9]+)", "([^"]*)", removed\)',
        $src)) {
        $out[$m.groups[0] + "." + $m.groups[1]] = "use " + $m.groups[2] + "; removed in " + $removed;
    }
    return $out;
}

# libsigExports reads every `add("lib", `...`)` table in internal/libsig.
func libsigExports() {
    def out as list of Export init [];
    def notes as map of string to string init libsigDeprecations();
    for (def file in fs.list(LIBSIG)) {
        if (not strings.endsWith($file, ".go") or strings.endsWith($file, "_test.go")) {
            continue;
        }
        def src as string init fs.readString(path.join(LIBSIG, $file));
        for (def block in regex.findAll('add\("([a-z0-9]+)", `([^`]*)`', $src)) {
            def lib as string init $block.groups[0];
            for (def line in strings.split($block.groups[1], "\n")) {
                def m as regex.Match init regex.find(
                    '^([A-Za-z][A-Za-z0-9]*)\((.*)\) (.+)$',
                    strings.trim($line));
                if ($m.start < 0) {
                    continue;
                }
                def types as list of string init [];
                if (strings.trim($m.groups[1]) != "") {
                    for (def t in strings.split($m.groups[1], ",")) {
                        $types[] = strings.trim($t);
                    }
                }
                def note as string init "";
                if (maps.has($notes, $lib + "." + $m.groups[0])) {
                    $note = $notes[$lib + "." + $m.groups[0]];
                }
                $out[] = Export{
                    owner: $lib,
                    name: $m.groups[0],
                    params: [],
                    types: $types,
                    returns: $m.groups[2],
                    deprecated: $note
                };
            }
        }
    }
    return $out;
}

# sourceFiles lists the .j files under dir that make up a module or deck,
# without test overlays.
func sourceFiles(dir as string) {
    def out as list of string init [];
    if (not fs.isDir($dir)) {
        return $out;
    }
    for (def st in fs.walk($dir)) {
        if (not $st.isDir and strings.endsWith($st.path, ".j") and
            not strings.endsWith($st.path, "_test.j")) {
            $out[] = $st.path;
        }
    }
    return $out;
}

# docExports reads the exported functions of one source file.
func docExports(owner as string, file as string) {
    def out as list of Export init [];
    def fd as docblock.FileDoc init docblock.parse(fs.readString($file));
    for (def f in $fd.funcs) {
        if (not $f.exported) {
            continue;
        }
        def names as list of string init [];
        def types as list of string init [];
        for (def p in $f.params) {
            $names[] = $p.name;
            $types[] = $p.type;
        }
        $out[] = Export{
            owner: $owner,
            name: $f.name,
            params: $names,
            types: $types,
            returns: $f.returns.type,
            deprecated: $f.deprecated
        };
    }
    return $out;
}

# structFields reads the exported structs' field names of one source file, as
# "owner.Struct.field".
func structFields(owner as string, file as string) {
    def out as list of string init [];
    def fd as docblock.FileDoc init docblock.parse(fs.readString($file));
    for (def s in $fd.structs) {
        if (not $s.exported) {
            continue;
        }
        for (def fld in $s.fields) {
            if (not strings.contains($fld.type, "Duration")) {
                $out[] = $owner + "." + $s.name + "." + $fld.name;
            }
        }
    }
    return $out;
}

# ---------- rules over exports ----------

func namesOf(exports as list of Export, owner as string) {
    def out as map of string to bool init {};
    for (def e in $exports) {
        if ($e.owner == $owner) {
            $out[$e.name] = true;
        }
    }
    return $out;
}

func exportRules(exports as list of Export) {
    def out as list of Finding init [];
    def byOwner as map of string to map of string to bool init {};
    for (def e in $exports) {
        if (not maps.has($byOwner, $e.owner)) {
            $byOwner[$e.owner] = namesOf($exports, $e.owner);
        }
    }
    def seen as map of string to bool init {};
    for (def e in $exports) {
        def where as string init $e.owner + "." + $e.name;
        # A deprecated name is the old half of a fix: listed until it is
        # removed, not held to the rules it was renamed for.
        if ($e.deprecated != "") {
            if (not maps.has($seen, $where)) {
                $out[] = Finding{
                    rule: "Deprecated names still shipped",
                    where: $where,
                    detail: $e.deprecated
                };
            }
            $seen[$where] = true;
            continue;
        }
        if (lists.contains(KEYWORDS, $e.name) and not lists.contains(EXCEPTIONS, $where)) {
            $out[] = Finding{
                rule: "Names spelled like a keyword",
                where: $where,
                detail: "rename it (`bitAnd`, `listObjects`)"
            };
        }
        if (regex.matches('^(is|has)[A-Z]', $e.name) and $e.returns != "" and $e.returns != "bool") {
            $out[] = Finding{
                rule: "Predicates return bool",
                where: $where,
                detail: "returns " + $e.returns
            };
        }
        def names as map of string to bool init $byOwner[$e.owner];
        def excepted as bool init lists.contains(EXCEPTIONS, $where);
        if (strings.endsWith($e.name, "With") and len($e.name) > 4 and
            not maps.has($seen, $where) and
            not $excepted) {
            def base as string init $e.name[..len($e.name) - 4];
            if (not maps.has($names, $base)) {
                $out[] = Finding{
                    rule: "`xWith` has its base form",
                    where: $where,
                    detail: "no `" + $base + "`"
                };
            }
        }
        for (def i in 0..len($e.params)) {
            def p as string init $e.params[$i];
            def typed as bool init $i < len($e.types) and strings.contains(
                $e.types[$i],
                "Duration");
            if (regex.matches(DURATION_WORDS, $p) and not $excepted and not $typed) {
                $out[] = Finding{
                    rule: "Durations name their unit",
                    where: $where,
                    detail: "parameter `" + $p + "`"
                };
            }
            if (strings.contains(strings.lower($p), "insecure") and
                not strings.startsWith($p, "allowInsecure")) {
                $out[] = Finding{
                    rule: "Security opt-outs say so",
                    where: $where,
                    detail: "parameter `" + $p + "`"
                };
            }
        }
        if (strings.contains(strings.lower($e.name), "insecure") and
            not strings.startsWith($e.name, "allowInsecure")) {
            $out[] = Finding{
                rule: "Security opt-outs say so",
                where: $where,
                detail: "function `" + $e.name +
                    "`; use `TLSOptions.skipVerify` or `allowInsecure...`"
            };
        }
        $seen[$where] = true;
    }
    return $out;
}

func fieldRules(fields as list of string) {
    def out as list of Finding init [];
    for (def f in $fields) {
        def parts as list of string init strings.split($f, ".");
        def name as string init lists.last($parts);
        if (regex.matches(DURATION_WORDS, $name)) {
            $out[] = Finding{rule: "Durations name their unit", where: $f, detail: "field"};
        }
        if (strings.contains(strings.lower($name), "insecure") and
            not strings.startsWith($name, "allowInsecure")) {
            $out[] = Finding{rule: "Security opt-outs say so", where: $f, detail: "field"};
        }
    }
    return $out;
}

# ---------- rules over source text ----------

# sourceRules looks at the throws and catches of one module or deck file.
func sourceRules(owner as string, ownKind as string, file as string) {
    def out as list of Finding init [];
    def src as string init fs.readString($file);
    def lines as list of string init strings.split($src, "\n");
    def own as list of string init [$ownKind];
    def kc as regex.Match init regex.find(
        'def const ERROR_KIND as string init "([a-z0-9-]+)"',
        $src);
    if ($kc.start >= 0) {
        $own[] = $kc.groups[0];
    }
    # Error literals, which may span lines: `Error{kind: "x"` or `XError{\n kind: "x"`.
    for (def m in regex.findAll('[A-Z][A-Za-z0-9]*Error\{\s*kind: "([a-z0-9-]+)"', $src)) {
        def kind as string init $m.groups[0];
        if (not lists.contains($own, $kind) and not lists.contains(GENERIC_KINDS, $kind) and
            not lists.contains(EXCEPTIONS, $owner + ":" + $kind)) {
            $out[] = Finding{
                rule: "A module throws its own kind or a generic one",
                where: $file + ":" + convert.toString(lineOf($src, $m.start)),
                detail: "kind `" + $kind + "` in " + $owner
            };
        }
    }
    def n as int init 0;
    for (def line in $lines) {
        $n = $n + 1;
        def at as string init $file + ":" + convert.toString($n);
        def trimmed as string init strings.trim($line);
        if (strings.startsWith($trimmed, "#") or strings.startsWith($trimmed, "*")) {
            continue;
        }
        def twice as regex.Match init regex.find('message: "([a-z0-9.]+): ([a-z0-9.]+): ', $line);
        if ($twice.start >= 0 and $twice.groups[0] == $twice.groups[1]) {
            $out[] = Finding{
                rule: "Messages name their source once",
                where: $at,
                detail: "`" + $twice.groups[0] + ": " + $twice.groups[1] + ": `"
            };
        }
        if (regex.matches(
            '\$(e|err|ex|caught)\.message\b\s*(==|!=)|strings\.(contains|startsWith|endsWith|indexOf)\(\$(e|err|ex|caught)\.message\b',
            $line)) {
            $out[] = Finding{
                rule: "No code matches on a message",
                where: $at,
                detail: strings.trim($line)
            };
        }
    }
    return $out;
}

# lineOf is the 1-based line of a rune offset in src.
func lineOf(src as string, offset as int) {
    return strings.count($src[..$offset], "\n") + 1;
}

# ownerOf is the module a file belongs to: `font_cff.inc.j` is part of `font`.
func ownerOf(file as string) {
    def base as string init path.base($file);
    def m as regex.Match init regex.find('^([a-z0-9]+)', $base);
    return $m.groups[0];
}

# ---------- driver ----------

func ownerOfDeck(dir as string) {
    def manifest as string init path.join($dir, "deck.toml");
    if (fs.isFile($manifest)) {
        def m as regex.Match init regex.find('(?m)^name = "(@[^"]+)"', fs.readString($manifest));
        if ($m.start >= 0) {
            return $m.groups[0];
        }
    }
    return "@?/" + slugOfDeck($dir);
}

# slugOfDeck is a deck's own error kind by convention: `deck-mikrotik` -> `mikrotik`.
func slugOfDeck(dir as string) {
    def base as string init path.base(path.clean($dir));
    if (strings.startsWith($base, "deck-")) {
        return $base[5..];
    }
    return $base;
}

func report(findings as list of Finding, exports as int, decks as list of string) {
    def rules as list of string init [];
    for (def f in $findings) {
        if (not lists.contains($rules, $f.rule)) {
            $rules[] = $f.rule;
        }
    }
    def deprecated as int init 0;
    for (def f in $findings) {
        if ($f.rule == "Deprecated names still shipped") {
            $deprecated = $deprecated + 1;
        }
    }
    io.printf("# Conventions survey\n\n");
    io.printf(
        "%d exported functions surveyed: %d findings, %d deprecated names still shipped.\n",
        $exports,
        len($findings) - $deprecated,
        $deprecated);
    io.printf("Generated by ");
    io.printf("`scripts/conventions-survey.j`; the rules are in [conventions](conventions.md).\n");
    io.printf("\nSurveyed: the built-in libraries (`internal/libsig`), the bundled modules");
    if (len($decks) == 0) {
        io.printf(".\n");
    } else {
        io.printf(" and %s.\n", strings.join($decks, ", "));
    }
    for (def rule in lists.sort($rules)) {
        def lines as list of string init [];
        for (def f in $findings) {
            if ($f.rule == $rule) {
                $lines[] = "- `" + $f.where + "`: " + $f.detail;
            }
        }
        io.printf("\n## %s (%d)\n\n", $rule, len($lines));
        for (def l in lists.sort($lines)) {
            io.printf("%s\n", $l);
        }
    }
}

func main() {
    def exports as list of Export init libsigExports();
    def findings as list of Finding init [];
    def fields as list of string init [];
    def decks as list of string init [];
    for (def file in fs.list(MODDIR)) {
        if (not strings.endsWith($file, ".j") or strings.endsWith($file, "_test.j")) {
            continue;
        }
        def owner as string init ownerOf($file);
        def full as string init path.join(MODDIR, $file);
        $exports = lists.concat($exports, docExports($owner, $full));
        $fields = lists.concat($fields, structFields($owner, $full));
        $findings = lists.concat($findings, sourceRules($owner, $owner, $full));
    }
    for (def i in 1..len(os.ARGS)) {
        def dir as string init os.ARGS[$i];
        def owner as string init ownerOfDeck($dir);
        $decks[] = "`" + $owner + "`";
        for (def file in sourceFiles(path.join($dir, "src"))) {
            $exports = lists.concat($exports, docExports($owner, $file));
            $fields = lists.concat($fields, structFields($owner, $file));
            $findings = lists.concat($findings, sourceRules($owner, slugOfDeck($dir), $file));
        }
    }
    $findings = lists.concat($findings, exportRules($exports));
    $findings = lists.concat($findings, fieldRules($fields));
    report($findings, len($exports), $decks);
}

main();
