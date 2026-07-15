# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

/**
 * A Jennifer doc-comment parser. Read Jennifer source and return the
 * documentation embedded in it as structured, typed values. It produces data;
 * it does not render (turning docs into HTML is a separate consumer). A doc
 * comment opens with a doc-block marker, and its body is a summary line, an
 * optional description, and tag lines; it immediately precedes the construct it
 * documents (`func`, `def struct`, `def const`) or, when it carries a module
 * tag, is the file preamble. `export` is read from the construct keyword, not a
 * tag. Types are written verbatim in Jennifer syntax inside braces. The module
 * reports, never enforces: signature mismatches (a documented name that names
 * no real parameter, a parameter with no doc) and orphaned comments surface as
 * Diagnostic values, and the caller decides what is fatal.
 * @module docblock
 * @example
 * import "docblock.j" as db;
 * def doc as db.FileDoc init db.parse(fs.readString("mymod.j"));
 * io.printf("funcs=%d diagnostics=%d\n", len($doc.funcs), len($doc.diagnostics));
 */
use regex;
use strings;
use lists;

# ===== result types (all exported; a FileDoc is a tree of these) =====

/**
 * A reported documentation problem (a mismatch or an orphaned comment).
 * @field severity {string} the level, currently always "warning"
 * @field line {int} the source line the doc comment documents
 * @field message {string} the human-readable description of the problem
 */
export def struct Diagnostic { severity as string, line as int, message as string };
/**
 * One documented parameter or struct field.
 * @field name {string} the parameter or field name
 * @field type {string} the declared type, verbatim in Jennifer syntax
 * @field description {string} the prose description
 */
export def struct ParamDoc { name as string, type as string, description as string };
/**
 * A documented return value.
 * @field type {string} the returned type, verbatim in Jennifer syntax
 * @field description {string} the prose description
 */
export def struct ReturnDoc { type as string, description as string };
/**
 * A documented thrown error.
 * @field type {string} the thrown type, verbatim in Jennifer syntax
 * @field description {string} the prose description
 */
export def struct ThrowDoc { type as string, description as string };
/**
 * The module preamble documentation (the doc comment carrying a module tag).
 * @field summary {string} the one-line summary
 * @field description {string} the longer description below the summary
 * @field author {string} the documented author
 * @field version {string} the documented version
 * @field license {string} the documented license
 * @field see {list of string} the cross-references
 */
export def struct ModuleDoc { summary as string, description as string, author as string, version as string, license as string, see as list of string };
/**
 * The documentation of one constant.
 * @field name {string} the constant name
 * @field exported {bool} whether the constant is exported
 * @field type {string} the declared type, verbatim in Jennifer syntax
 * @field summary {string} the one-line summary
 * @field description {string} the longer description below the summary
 * @field since {string} the documented since-version
 * @field deprecated {string} the deprecation note, or "" if not deprecated
 * @field see {list of string} the cross-references
 * @field internal {bool} whether the constant is marked internal
 */
export def struct ConstDoc { name as string, exported as bool, type as string, summary as string, description as string, since as string, deprecated as string, see as list of string, internal as bool };
/**
 * The documentation of one struct.
 * @field name {string} the struct name
 * @field exported {bool} whether the struct is exported
 * @field summary {string} the one-line summary
 * @field description {string} the longer description below the summary
 * @field fields {list of ParamDoc} the documented fields
 * @field since {string} the documented since-version
 * @field deprecated {string} the deprecation note, or "" if not deprecated
 * @field see {list of string} the cross-references
 * @field internal {bool} whether the struct is marked internal
 */
export def struct StructDoc { name as string, exported as bool, summary as string, description as string, fields as list of ParamDoc, since as string, deprecated as string, see as list of string, internal as bool };
/**
 * The documentation of one method.
 * @field name {string} the method name
 * @field exported {bool} whether the method is exported
 * @field summary {string} the one-line summary
 * @field description {string} the longer description below the summary
 * @field params {list of ParamDoc} the documented parameters
 * @field returns {ReturnDoc} the documented return value
 * @field throws {list of ThrowDoc} the documented thrown errors
 * @field examples {list of string} the documented examples
 * @field since {string} the documented since-version
 * @field deprecated {string} the deprecation note, or "" if not deprecated
 * @field see {list of string} the cross-references
 * @field internal {bool} whether the method is marked internal
 */
export def struct FuncDoc { name as string, exported as bool, summary as string, description as string, params as list of ParamDoc, returns as ReturnDoc, throws as list of ThrowDoc, examples as list of string, since as string, deprecated as string, see as list of string, internal as bool };
/**
 * The full documentation extracted from one source file.
 * @field module {ModuleDoc} the module preamble documentation
 * @field funcs {list of FuncDoc} the documented methods
 * @field structs {list of StructDoc} the documented structs
 * @field consts {list of ConstDoc} the documented constants
 * @field diagnostics {list of Diagnostic} the reported documentation problems
 */
export def struct FileDoc { module as ModuleDoc, funcs as list of FuncDoc, structs as list of StructDoc, consts as list of ConstDoc, diagnostics as list of Diagnostic };

# ===== private intermediates =====

def struct RawDoc { body as string, after as int, line as int };
def struct Parsed {
    summary as string, description as string,
    params as list of ParamDoc, returns as ReturnDoc, throws as list of ThrowDoc,
    examples as list of string, see as list of string,
    since as string, deprecated as string, internal as bool,
    author as string, version as string, license as string, isModule as bool
};

# ===== the entry point =====

/**
 * Read Jennifer source and return a FileDoc: the module preamble, one doc per
 * func / struct / const, and any diagnostics (mismatched or orphaned doc
 * comments). It reports; it does not fail on a documentation error.
 * @param source {string} the Jennifer source text to parse
 * @return {FileDoc} the extracted documentation tree
 */
export func parse(source as string) {
    def raws as list of RawDoc init scanDocs($source);
    def module as ModuleDoc init ModuleDoc{ summary: "", description: "", author: "", version: "", license: "", see: [] };
    def funcs as list of FuncDoc init [];
    def structs as list of StructDoc init [];
    def consts as list of ConstDoc init [];
    def diags as list of Diagnostic init [];

    for (def raw in $raws) {
        def p as Parsed init parseBody($raw.body);
        if ($p.isModule) {
            $module = buildModule($p);
        } else {
            def tail as string init strings.trimLeft(strings.substring($source, $raw.after, len($source)));
            def fm as regex.Match init regex.find("^(export\\s+)?func\\s+([A-Za-z]+)\\s*\\(([^)]*)\\)", $tail);
            def sm as regex.Match init regex.find("^(export\\s+)?def\\s+struct\\s+([A-Za-z]+)\\s*\\{([^}]*)\\}", $tail);
            def cm as regex.Match init regex.find("^(export\\s+)?def\\s+const\\s+([A-Za-z][A-Za-z_]*)\\s+as\\s+([A-Za-z][A-Za-z. ]*?)\\s+init", $tail);
            if (not ($fm.start == -1)) {
                $funcs[] = buildFunc($p, $fm.groups[1], not ($fm.groups[0] == ""));
                $diags = lists.concat($diags, crossCheck("param", $fm.groups[1], $raw.line, $p.params, declNames($fm.groups[2])));
            } elseif (not ($sm.start == -1)) {
                $structs[] = buildStruct($p, $sm.groups[1], not ($sm.groups[0] == ""));
                $diags = lists.concat($diags, crossCheck("field", $sm.groups[1], $raw.line, $p.params, declNames($sm.groups[2])));
            } elseif (not ($cm.start == -1)) {
                $consts[] = buildConst($p, $cm.groups[1], not ($cm.groups[0] == ""), strings.trim($cm.groups[2]));
            } else {
                $diags[] = Diagnostic{ severity: "warning", line: $raw.line, message: "doc comment precedes no documentable construct (orphaned)" };
            }
        }
    }
    return FileDoc{ module: $module, funcs: $funcs, structs: $structs, consts: $consts, diagnostics: $diags };
}

# ===== struct builders (Parsed -> the public shapes) =====

func buildFunc(p as Parsed, name as string, exported as bool) {
    return FuncDoc{ name: $name, exported: $exported, summary: $p.summary, description: $p.description, params: $p.params, returns: $p.returns, throws: $p.throws, examples: $p.examples, since: $p.since, deprecated: $p.deprecated, see: $p.see, internal: $p.internal };
}

func buildStruct(p as Parsed, name as string, exported as bool) {
    return StructDoc{ name: $name, exported: $exported, summary: $p.summary, description: $p.description, fields: $p.params, since: $p.since, deprecated: $p.deprecated, see: $p.see, internal: $p.internal };
}

func buildConst(p as Parsed, name as string, exported as bool, ctype as string) {
    return ConstDoc{ name: $name, exported: $exported, type: $ctype, summary: $p.summary, description: $p.description, since: $p.since, deprecated: $p.deprecated, see: $p.see, internal: $p.internal };
}

func buildModule(p as Parsed) {
    return ModuleDoc{ summary: $p.summary, description: $p.description, author: $p.author, version: $p.version, license: $p.license, see: $p.see };
}

# ===== comment scanner =====

# scanDocs walks the source and returns each `/** ... */` doc comment with the
# byte offset just past its close and the source line of the construct. It skips
# string literals and `#` line comments (so a `/**` inside a string is not a doc
# comment) and nests `/* */` correctly.
func scanDocs(source as string) {
    def cs as list of string init strings.chars($source);
    def n as int init len($cs);
    def out as list of RawDoc init [];
    def i as int init 0;
    def line as int init 1;
    while ($i < $n) {
        def c as string init $cs[$i];
        if ($c == "\n") {
            $line = $line + 1;
            $i = $i + 1;
        } elseif ($c == "#") {
            while ($i < $n and not ($cs[$i] == "\n")) {
                $i = $i + 1;
            }
        } elseif ($c == "\"" or $c == "'") {
            def q as string init $c;
            $i = $i + 1;
            while ($i < $n and not ($cs[$i] == $q)) {
                if ($cs[$i] == "\\") {
                    $i = $i + 1;
                }
                if ($i < $n and $cs[$i] == "\n") {
                    $line = $line + 1;
                }
                $i = $i + 1;
            }
            $i = $i + 1;
        } elseif ($c == "/" and $i + 1 < $n and $cs[$i + 1] == "*") {
            def isDoc as bool init ($i + 2 < $n and $cs[$i + 2] == "*" and not ($i + 3 < $n and $cs[$i + 3] == "/"));
            def openLen as int init 2;
            if ($isDoc) {
                $openLen = 3;
            }
            def afterEnd as int init blockNestEnd($cs, $n, $i, $openLen);
            def span as string init strings.substring($source, $i, $afterEnd);
            def nl as int init countNewlines($span);
            if ($isDoc) {
                def body as string init strings.substring($source, $i + $openLen, $afterEnd - 2);
                $out[] = RawDoc{ body: $body, after: $afterEnd, line: $line + $nl };
            }
            $line = $line + $nl;
            $i = $afterEnd;
        } else {
            $i = $i + 1;
        }
    }
    return $out;
}

# blockNestEnd returns the index just past the `*/` that closes the block
# comment opened at `start` (openLen 2 for `/*`, 3 for `/**`), honouring nested
# `/* */`. Returns n on an unterminated comment.
func blockNestEnd(cs as list of string, n as int, start as int, openLen as int) {
    def i as int init $start + $openLen;
    def depth as int init 1;
    while ($i + 1 < $n) {
        if ($cs[$i] == "/" and $cs[$i + 1] == "*") {
            $depth = $depth + 1;
            $i = $i + 2;
        } elseif ($cs[$i] == "*" and $cs[$i + 1] == "/") {
            $depth = $depth - 1;
            $i = $i + 2;
            if ($depth == 0) {
                return $i;
            }
        } else {
            $i = $i + 1;
        }
    }
    return $n;
}

func countNewlines(s as string) {
    return len(strings.split($s, "\n")) - 1;
}

# ===== body / tag parser =====

# parseBody turns a doc comment body into a Parsed: summary (first line),
# description (up to the first tag), and the typed tag collections.
func parseBody(body as string) {
    def norm as string init strings.replace($body, "\r", "");
    def rawLines as list of string init strings.split($norm, "\n");
    def lines as list of string init [];
    for (def ln in $rawLines) {
        $lines[] = cleanLine($ln);
    }

    def summary as string init "";
    def descLines as list of string init [];
    def params as list of ParamDoc init [];
    def throws as list of ThrowDoc init [];
    def examples as list of string init [];
    def see as list of string init [];
    def returns as ReturnDoc init ReturnDoc{ type: "", description: "" };
    def since as string init "";
    def deprecated as string init "";
    def author as string init "";
    def version as string init "";
    def license as string init "";
    def internal as bool init false;
    def isModule as bool init false;

    def cnt as int init len($lines);
    def i as int init 0;
    while ($i < $cnt and $lines[$i] == "") {
        $i = $i + 1;
    }
    if ($i < $cnt) {
        $summary = $lines[$i];
        $i = $i + 1;
    }
    while ($i < $cnt and not isTag($lines[$i])) {
        $descLines[] = $lines[$i];
        $i = $i + 1;
    }
    while ($i < $cnt) {
        def line as string init $lines[$i];
        if (not isTag($line)) {
            $i = $i + 1;
        } else {
            def tm as regex.Match init regex.find("^@([A-Za-z]+)\\s*(.*)$", $line);
            if ($tm.start == -1) {
                $i = $i + 1;
            } else {
                def tag as string init $tm.groups[0];
                def rest as string init $tm.groups[1];
                if ($tag == "param" or $tag == "field") {
                    $params[] = parseParam($rest);
                    $i = $i + 1;
                } elseif ($tag == "return" or $tag == "returns") {
                    $returns = parseTyped($rest);
                    $i = $i + 1;
                } elseif ($tag == "throws") {
                    def t as ReturnDoc init parseTyped($rest);
                    $throws[] = ThrowDoc{ type: $t.type, description: $t.description };
                    $i = $i + 1;
                } elseif ($tag == "example") {
                    def exLines as list of string init [];
                    if (not ($rest == "")) {
                        $exLines[] = $rest;
                    }
                    def j as int init $i + 1;
                    while ($j < $cnt and not isTag($lines[$j])) {
                        $exLines[] = $lines[$j];
                        $j = $j + 1;
                    }
                    $examples[] = strings.trimRight(strings.join($exLines, "\n"));
                    $i = $j;
                } elseif ($tag == "since") {
                    $since = $rest;
                    $i = $i + 1;
                } elseif ($tag == "deprecated") {
                    $deprecated = $rest;
                    if ($rest == "") {
                        $deprecated = "deprecated";
                    }
                    $i = $i + 1;
                } elseif ($tag == "see") {
                    $see[] = $rest;
                    $i = $i + 1;
                } elseif ($tag == "internal") {
                    $internal = true;
                    $i = $i + 1;
                } elseif ($tag == "module") {
                    $isModule = true;
                    $i = $i + 1;
                } elseif ($tag == "author") {
                    $author = $rest;
                    $i = $i + 1;
                } elseif ($tag == "version") {
                    $version = $rest;
                    $i = $i + 1;
                } elseif ($tag == "license") {
                    $license = $rest;
                    $i = $i + 1;
                } else {
                    $i = $i + 1;
                }
            }
        }
    }
    return Parsed{ summary: $summary, description: strings.trim(strings.join($descLines, "\n")), params: $params, returns: $returns, throws: $throws, examples: $examples, see: $see, since: $since, deprecated: $deprecated, internal: $internal, author: $author, version: $version, license: $license, isModule: $isModule };
}

# cleanLine strips a line's leading doc decoration (optional whitespace + `*` +
# one space) and trailing whitespace.
func cleanLine(line as string) {
    return strings.trimRight(regex.replace("^[ \\t]*\\*?[ \\t]?", $line, ""));
}

func isTag(line as string) {
    return strings.startsWith($line, "@");
}

# parseParam parses `name {type} desc`.
func parseParam(rest as string) {
    def m as regex.Match init regex.find("^([A-Za-z]+)\\s+\\{([^}]*)\\}\\s*(.*)$", $rest);
    if ($m.start == -1) {
        return ParamDoc{ name: firstWord($rest), type: "", description: "" };
    }
    return ParamDoc{ name: $m.groups[0], type: strings.trim($m.groups[1]), description: strings.trim($m.groups[2]) };
}

# parseTyped parses `{type} desc` (for @return / @throws).
func parseTyped(rest as string) {
    def m as regex.Match init regex.find("^\\{([^}]*)\\}\\s*(.*)$", $rest);
    if ($m.start == -1) {
        return ReturnDoc{ type: "", description: strings.trim($rest) };
    }
    return ReturnDoc{ type: strings.trim($m.groups[0]), description: strings.trim($m.groups[1]) };
}

func firstWord(s as string) {
    def t as string init strings.trim($s);
    if ($t == "") {
        return "";
    }
    return strings.split($t, " ")[0];
}

# declNames splits a parameter / field signature ("a as int, b as string") into
# the declared names (["a", "b"]). Jennifer types carry no commas or parens, so
# the list splits cleanly.
func declNames(sig as string) {
    def out as list of string init [];
    def trimmed as string init strings.trim($sig);
    if ($trimmed == "") {
        return $out;
    }
    for (def part in strings.split($trimmed, ",")) {
        def pt as string init strings.trim($part);
        if (not ($pt == "")) {
            $out[] = firstWord($pt);
        }
    }
    return $out;
}

# crossCheck matches documented names against the real declaration, producing a
# Diagnostic for each documented name that is not real and each real name that
# is undocumented.
func crossCheck(kind as string, cname as string, line as int, docParams as list of ParamDoc, realNames as list of string) {
    def diags as list of Diagnostic init [];
    def docNames as list of string init [];
    for (def d in $docParams) {
        $docNames[] = $d.name;
    }
    for (def dn in $docNames) {
        if (not lists.contains($realNames, $dn)) {
            $diags[] = Diagnostic{ severity: "warning", line: $line, message: "@" + $kind + " \"" + $dn + "\" is not a " + $kind + " of " + $cname };
        }
    }
    for (def rn in $realNames) {
        if (not lists.contains($docNames, $rn)) {
            $diags[] = Diagnostic{ severity: "warning", line: $line, message: $kind + " \"" + $rn + "\" of " + $cname + " has no @" + $kind };
        }
    }
    return $diags;
}
