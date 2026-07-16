# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

/**
 * A small text template engine for lightweight CMS-style rendering - a subset of
 * Go's `text/template` (the Go / Hugo style), evaluated directly over a
 * `json.Value` data tree. There is no compile step and no AST: the engine
 * re-scans block bodies as it renders, which is fine at page / CMS scale. Dotted
 * paths resolve as JSON Pointers against the current node (`.a.b` -> `/a/b`; `.`
 * is the current node); `$` is the root node, `$var` a variable; a missing key
 * renders as empty. Output is **not** auto-escaped (like `text/template`, not
 * `html/template`) - pipe untrusted text through `html`.
 *
 * Actions: value output; `if` / `else if` / `else` with the comparison / boolean
 * functions `eq` `ne` `lt` `le` `gt` `ge` `and` `or` `not` (parenthesised
 * nesting allowed); `range` (with an optional `$i, $e :=` index / element bind)
 * and `with`, each with an `else`; variable assignment `{{ $x := PIPE }}`;
 * `{{ define }}` / `{{ template }}` / `{{ block }}` layout inheritance;
 * `{{/* comments */}}`; `{{-` / `-}}` whitespace-trim markers; and output pipes
 * `upper` `lower` `title` `trim` `html` `urlize` `default` `truncate` `join`
 * `len`. Over the compiled-in `json` / `strings` / `lists` / `maps` / `convert`
 * libraries, so it runs on **both** binaries.
 * @module tengine
 * @example
 * def set as tengine.Set init tengine.newSet();
 * $set = tengine.add($set, "base", "<h1>{{ .title }}</h1>{{ template \"body\" . }}");
 * $set = tengine.add($set, "page", "{{ define \"body\" }}<p>{{ .msg | html }}</p>{{ end }}");
 * def out as string init tengine.render($set, "base", json.decode("{\"title\":\"Hi\",\"msg\":\"a<b\"}"));
 * # out == "<h1>Hi</h1><p>a&lt;b</p>"
 */
use json;
use strings;
use lists;
use maps;
use math;
use convert;

/**
 * A value-semantic set of named template sources (kept as two parallel lists).
 * Build it with `newSet`, populate it with `add`, and render an entry with
 * `render`.
 * @field names {list of string} the template names
 * @field sources {list of string} the template sources, positionally paired with `names`
 */
export def struct Set {
    names as list of string,
    sources as list of string
};

# The two halves of a control body, plus the source after the closing `{{ end }}`.
def struct BlockParts {
    thenPart as string,
    elsePart as string,
    remainder as string
};

# A parsed `"name" arg` tail of a template / block / define action.
def struct NameArg {
    name as string,
    arg as string
};

# One step of the recursive expression evaluator: a value and the token index
# just past what it consumed.
def struct Eval {
    value as json.Value,
    pos as int
};

# The `$i, $e :=` binding parsed off a `range` action (empty names = none).
def struct RangeVars {
    indexVar as string,
    elemVar as string,
    source as string
};

# The evolving state of a `{{ else }}` / `{{ else if }}` split inside takeBlock.
def struct ElseState {
    inElse as bool,
    opened as int,
    elsePart as string
};

# A parsed printf verb spec (`%[-][0][width][.prec]verb`) and the format position
# just past it.
def struct Verb {
    verb as string,
    width as int,
    leftAlign as bool,
    zeroPad as bool,
    prec as int,
    pos as int
};

# Nesting cap for exec's recursion: control blocks, template / block includes,
# and range bodies all re-enter exec one level deeper. A template that includes
# itself (directly or through a partner) would otherwise recurse until the
# interpreter's stack dies - a fatal crash, not a catchable error. 256 levels
# is far beyond any legitimate page.
def const MAX_NESTING as int init 256;

# --- set construction (exported) --------------------------------------------

/**
 * Create an empty template set.
 * @return {Set} a fresh, empty set
 */
export func newSet() {
    def names as list of string init [];
    def sources as list of string init [];
    return Set{ names: $names, sources: $sources };
}

# addRaw stores one name -> source pair, returning a new Set.
func addRaw(set as Set, name as string, src as string) {
    def s as Set init $set;
    $s.names = lists.push($s.names, $name);
    $s.sources = lists.push($s.sources, $src);
    return $s;
}

# setHas reports whether the set holds a template of the given name.
func setHas(set as Set, name as string) {
    def i as int init 0;
    while ($i < len($set.names)) {
        if ($set.names[$i] == $name) {
            return true;
        }
        $i = $i + 1;
    }
    return false;
}

# setGet returns the source of a named template, or "" if absent.
func setGet(set as Set, name as string) {
    def i as int init 0;
    while ($i < len($set.names)) {
        if ($set.names[$i] == $name) {
            return $set.sources[$i];
        }
        $i = $i + 1;
    }
    return "";
}

/**
 * Add a template source under `name`, returning a new Set. Any top-level
 * `{{ define "x" }}...{{ end }}` blocks are pulled out and registered as their
 * own entries `x`, so a page can define the sections its base layout pulls in.
 * Whitespace-trim markers (`{{-` / `-}}`) are applied as the source is stored.
 * @param set {Set} the set to extend
 * @param name {string} the template name
 * @param src {string} the template source text
 * @return {Set} a new Set with the template (and any `define`s) added
 * @throws {Error} kind "tengine" on an unterminated action
 */
export func add(set as Set, name as string, src as string) {
    def s as Set init $set;
    def residual as string init "";
    def rest as string init trimMarkers($src);
    while (true) {
        def i as int init strings.indexOf($rest, "{{");
        if ($i < 0) {
            $residual = $residual + $rest;
            break;
        }
        def pre as string init strings.substring($rest, 0, $i);
        def afterOpen as string init strings.substring($rest, $i + 2, len($rest));
        def j as int init closeActionIndex($afterOpen);
        if ($j < 0) {
            throw Error{ kind: "tengine", message: "tengine: unterminated action in " + $name, file: "", line: 0, col: 0 };
        }
        def action as string init strings.trim(strings.substring($afterOpen, 0, $j));
        def tail as string init strings.substring($afterOpen, $j + 2, len($afterOpen));
        if (actionKind($action) == "define") {
            $residual = $residual + $pre;
            def na as NameArg init parseNameArg($action);
            def bp as BlockParts init takeBlock($tail);
            $s = addRaw($s, $na.name, $bp.thenPart);
            $rest = $bp.remainder;
        } else {
            $residual = $residual + $pre + "{{" + $action + "}}";
            $rest = $tail;
        }
    }
    return addRaw($s, $name, $residual);
}

# --- rendering (exported) ---------------------------------------------------

/**
 * Render the named template against a `json.Value` data tree.
 * @param set {Set} the template set
 * @param entry {string} the name of the template to render
 * @param data {json.Value} the root data node
 * @return {string} the rendered output
 * @throws {Error} kind "tengine" if `entry` (or a referenced template) is absent
 */
export func render(set as Set, entry as string, data as json.Value) {
    if (not setHas($set, $entry)) {
        throw Error{ kind: "tengine", message: "tengine: no such template: " + $entry, file: "", line: 0, col: 0 };
    }
    return exec($set, setGet($set, $entry), $data, $data, emptyVars(), 0);
}

# exec renders a template source against the current node (`.`), the root (`$`),
# and the variable environment, returning the output. `depth` counts nesting
# levels (control blocks, template / block includes, range bodies) and trips
# MAX_NESTING so an include cycle throws instead of overflowing the stack.
func exec(set as Set, src as string, node as json.Value, root as json.Value, vars as map of string to json.Value, depth as int) {
    if ($depth > MAX_NESTING) {
        throw Error{ kind: "tengine", message: "tengine: nesting exceeds " + convert.toString(MAX_NESTING) + " levels (template include cycle?)", file: "", line: 0, col: 0 };
    }
    # Collect output fragments and join once: an accumulating `+` over a large
    # template (or a big rendered sub-block) is O(N^2) in the output size.
    def parts as list of string init [];
    def rest as string init $src;
    def env as map of string to json.Value init $vars;
    while (true) {
        def i as int init strings.indexOf($rest, "{{");
        if ($i < 0) {
            $parts[] = $rest;
            break;
        }
        $parts[] = strings.substring($rest, 0, $i);
        def afterOpen as string init strings.substring($rest, $i + 2, len($rest));
        def j as int init closeActionIndex($afterOpen);
        if ($j < 0) {
            throw Error{ kind: "tengine", message: "tengine: unterminated action", file: "", line: 0, col: 0 };
        }
        def action as string init strings.trim(strings.substring($afterOpen, 0, $j));
        def tail as string init strings.substring($afterOpen, $j + 2, len($afterOpen));
        def kind as string init actionKind($action);
        if ($kind == "if" or $kind == "range" or $kind == "with" or $kind == "block") {
            def bp as BlockParts init takeBlock($tail);
            $parts[] = execControl($set, $kind, $action, $bp, $node, $root, $env, $depth);
            $rest = $bp.remainder;
        } elseif ($kind == "assign") {
            $env = execAssign($action, $node, $root, $env);
            $rest = $tail;
        } elseif ($kind == "define") {
            $rest = takeBlock($tail).remainder;
        } elseif ($kind == "template") {
            def na as NameArg init parseNameArg($action);
            def argNode as json.Value init $node;
            if (len($na.arg) > 0) {
                $argNode = resolveTerm($na.arg, $node, $root, $env);
            }
            if (not setHas($set, $na.name)) {
                throw Error{ kind: "tengine", message: "tengine: no such template: " + $na.name, file: "", line: 0, col: 0 };
            }
            $parts[] = exec($set, setGet($set, $na.name), $argNode, $argNode, emptyVars(), $depth + 1);
            $rest = $tail;
        } elseif ($kind == "comment" or $kind == "end" or $kind == "else") {
            $rest = $tail;
        } else {
            $parts[] = evalOutput($action, $node, $root, $env);
            $rest = $tail;
        }
    }
    return strings.join($parts, "");
}

# execAssign handles `{{ $x := PIPELINE }}`, returning the updated environment.
func execAssign(action as string, node as json.Value, root as json.Value, vars as map of string to json.Value) {
    def eq as int init strings.indexOf($action, ":=");
    def name as string init stripDollar(strings.trim(strings.substring($action, 0, $eq)));
    def rhs as string init strings.trim(strings.substring($action, $eq + 2, len($action)));
    def env as map of string to json.Value init $vars;
    $env[$name] = evalPipeline($rhs, $node, $root, $env);
    return $env;
}

# execControl renders one control action (if / with / range / block), returning
# the produced output.
func execControl(set as Set, kind as string, action as string, bp as BlockParts, node as json.Value, root as json.Value, vars as map of string to json.Value, depth as int) {
    if ($kind == "if") {
        if (isTruthy(evalExprString(pipelineOf($action), $node, $root, $vars))) {
            return exec($set, $bp.thenPart, $node, $root, $vars, $depth + 1);
        }
        return exec($set, $bp.elsePart, $node, $root, $vars, $depth + 1);
    }
    if ($kind == "with") {
        def val as json.Value init evalExprString(pipelineOf($action), $node, $root, $vars);
        if (isTruthy($val)) {
            return exec($set, $bp.thenPart, $val, $root, $vars, $depth + 1);
        }
        return exec($set, $bp.elsePart, $node, $root, $vars, $depth + 1);
    }
    if ($kind == "range") {
        def rv as RangeVars init parseRange(pipelineOf($action));
        def val as json.Value init evalExprString($rv.source, $node, $root, $vars);
        return execRange($set, $val, $bp, $node, $root, $vars, $rv, $depth);
    }
    # block: render the set's named override if present, else the inline default
    def na as NameArg init parseNameArg($action);
    def argNode as json.Value init $node;
    if (len($na.arg) > 0) {
        $argNode = resolveTerm($na.arg, $node, $root, $vars);
    }
    if (setHas($set, $na.name)) {
        return exec($set, setGet($set, $na.name), $argNode, $argNode, emptyVars(), $depth + 1);
    }
    return exec($set, $bp.thenPart, $argNode, $argNode, emptyVars(), $depth + 1);
}

# execRange renders a range body once per element (list) or value (map, insertion
# order), rebinding `.` and any `$i, $e` bindings; an empty collection renders the
# else part.
func execRange(set as Set, val as json.Value, bp as BlockParts, node as json.Value, root as json.Value, vars as map of string to json.Value, rv as RangeVars, depth as int) {
    def t as string init json.typeOf($val);
    # Collect per-iteration fragments and join once: an accumulating `+` over a
    # large range is O(N^2) in the rendered output size.
    def parts as list of string init [];
    if ($t == "list") {
        def n as int init json.length($val);
        if ($n == 0) {
            return exec($set, $bp.elsePart, $node, $root, $vars, $depth + 1);
        }
        def i as int init 0;
        while ($i < $n) {
            def elem as json.Value init json.get($val, "/" + convert.toString($i));
            def env as map of string to json.Value init bindLoop($vars, $rv, numVal($i), $elem);
            $parts[] = exec($set, $bp.thenPart, $elem, $root, $env, $depth + 1);
            $i = $i + 1;
        }
        return strings.join($parts, "");
    }
    if ($t == "map") {
        def keys as list of string init json.keys($val);
        if (len($keys) == 0) {
            return exec($set, $bp.elsePart, $node, $root, $vars, $depth + 1);
        }
        for (def k in $keys) {
            def elem as json.Value init json.get($val, "/" + $k);
            def env as map of string to json.Value init bindLoop($vars, $rv, strVal($k), $elem);
            $parts[] = exec($set, $bp.thenPart, $elem, $root, $env, $depth + 1);
        }
        return strings.join($parts, "");
    }
    return exec($set, $bp.elsePart, $node, $root, $vars, $depth + 1);
}

# bindLoop returns a copy of the environment with the range's index / element
# variables bound (each optional).
func bindLoop(vars as map of string to json.Value, rv as RangeVars, key as json.Value, elem as json.Value) {
    def env as map of string to json.Value init $vars;
    if (len($rv.indexVar) > 0) {
        $env[$rv.indexVar] = $key;
    }
    if (len($rv.elemVar) > 0) {
        $env[$rv.elemVar] = $elem;
    }
    return $env;
}

# takeBlock scans forward from just after an opening keyword to its matching
# `{{ end }}`, splitting on a depth-0 `{{ else }}`. A depth-0 `{{ else if C }}`
# is desugared into a nested `{{ if C }}` inside the else part. Nested openers
# bump depth.
func takeBlock(src as string) {
    def depth as int init 0;
    def inElse as bool init false;
    def opened as int init 0;
    def thenPart as string init "";
    def elsePart as string init "";
    def rest as string init $src;
    while (true) {
        def i as int init strings.indexOf($rest, "{{");
        if ($i < 0) {
            throw Error{ kind: "tengine", message: "tengine: missing {{ end }}", file: "", line: 0, col: 0 };
        }
        def pre as string init strings.substring($rest, 0, $i);
        if ($inElse) {
            $elsePart = $elsePart + $pre;
        } else {
            $thenPart = $thenPart + $pre;
        }
        def afterOpen as string init strings.substring($rest, $i + 2, len($rest));
        def j as int init closeActionIndex($afterOpen);
        if ($j < 0) {
            throw Error{ kind: "tengine", message: "tengine: unterminated action", file: "", line: 0, col: 0 };
        }
        def action as string init strings.trim(strings.substring($afterOpen, 0, $j));
        def tail as string init strings.substring($afterOpen, $j + 2, len($afterOpen));
        def fw as string init firstWord($action);
        def literal as string init "{{" + $action + "}}";
        if ($fw == "if" or $fw == "range" or $fw == "with" or $fw == "block" or $fw == "define") {
            $depth = $depth + 1;
            if ($inElse) {
                $elsePart = $elsePart + $literal;
            } else {
                $thenPart = $thenPart + $literal;
            }
        } elseif ($action == "end") {
            if ($depth == 0) {
                return BlockParts{ thenPart: $thenPart, elsePart: repeatEnds($elsePart, $opened), remainder: $tail };
            }
            $depth = $depth - 1;
            if ($inElse) {
                $elsePart = $elsePart + $literal;
            } else {
                $thenPart = $thenPart + $literal;
            }
        } elseif ($fw == "else" and $depth == 0) {
            def es as ElseState init handleElse($action, $inElse, $opened, $elsePart);
            $inElse = $es.inElse;
            $opened = $es.opened;
            $elsePart = $es.elsePart;
        } else {
            if ($inElse) {
                $elsePart = $elsePart + $literal;
            } else {
                $thenPart = $thenPart + $literal;
            }
        }
        $rest = $tail;
    }
}

# repeatEnds appends `n` closing `{{end}}` actions (to balance desugared else-ifs).
func repeatEnds(s as string, n as int) {
    def out as string init $s;
    def k as int init 0;
    while ($k < $n) {
        $out = $out + "{{end}}";
        $k = $k + 1;
    }
    return $out;
}

# handleElse advances the else-split state for a depth-0 `{{ else }}` /
# `{{ else if C }}`: a plain else starts (or, in an else-if chain, appends) the
# else part; an else-if desugars to a nested `{{ if C }}` counted in `opened`.
func handleElse(action as string, inElse as bool, opened as int, elsePart as string) {
    def cond as string init strings.trim(afterFirstWord($action));
    if (len($cond) == 0) {
        if ($opened == 0 and not $inElse) {
            return ElseState{ inElse: true, opened: $opened, elsePart: $elsePart };
        }
        return ElseState{ inElse: $inElse, opened: $opened, elsePart: $elsePart + "{{else}}" };
    }
    if ($opened == 0 and not $inElse) {
        return ElseState{ inElse: true, opened: 1, elsePart: $elsePart + "{{" + $cond + "}}" };
    }
    return ElseState{ inElse: $inElse, opened: $opened + 1, elsePart: $elsePart + "{{else}}{{" + $cond + "}}" };
}

# --- expression evaluation (private) ----------------------------------------

# resolveTerm resolves one term to a json.Value: `.` / `.a.b` (against the node),
# `$` / `$.a.b` (root), `$var` / `$var.a.b` (variable), a "quoted" string, a
# number, `true` / `false`, or a bare word (a string).
func resolveTerm(term as string, node as json.Value, root as json.Value, vars as map of string to json.Value) {
    if ($term == ".") {
        return $node;
    }
    if ($term == "$") {
        return $root;
    }
    if (strings.startsWith($term, "$")) {
        def rest as string init strings.substring($term, 1, len($term));
        if (strings.startsWith($rest, ".")) {
            return lookup($root, $rest);
        }
        def dot as int init strings.indexOf($rest, ".");
        def vname as string init $rest;
        def sub as string init "";
        if ($dot >= 0) {
            $vname = strings.substring($rest, 0, $dot);
            $sub = strings.substring($rest, $dot, len($rest));
        }
        if (not maps.has($vars, $vname)) {
            return nullVal();
        }
        if (len($sub) == 0) {
            return $vars[$vname];
        }
        return lookup($vars[$vname], $sub);
    }
    if (strings.startsWith($term, ".")) {
        return lookup($node, $term);
    }
    if (strings.startsWith($term, "\"") or strings.startsWith($term, "'")) {
        return strVal(strings.substring($term, 1, len($term) - 1));
    }
    if ($term == "true") {
        return boolVal(true);
    }
    if ($term == "false") {
        return boolVal(false);
    }
    if (isNumber($term)) {
        return json.decode($term);
    }
    return strVal($term);
}

# lookup resolves a dotted path (".a.b") against a base value as a JSON Pointer.
func lookup(base as json.Value, dotted as string) {
    def ptr as string init strings.replace($dotted, ".", "/");
    if (json.has($base, $ptr)) {
        return json.get($base, $ptr);
    }
    return nullVal();
}

# tokenize splits an expression into terms, honoring quotes and parentheses.
# closeActionIndex returns the rune index within `s` (the text right after a
# `{{`) of the `}}` that ends the action, or -1 if it is unterminated. A `}}`
# inside a quoted string does not count; when the action is a `{{/* ... */}}`
# comment the scan runs to the `*/}}` terminator, so a `}}` inside the comment
# body is ignored too. This replaces a naive `indexOf("}}")` that cut on the
# first `}}` regardless of quoting or comments.
func closeActionIndex(s as string) {
    def cs as list of string init strings.chars($s);
    def n as int init len($cs);
    # Detect a comment: skip leading whitespace and an optional `-` trim marker,
    # then look for `/*`.
    def k as int init 0;
    while ($k < $n and ($cs[$k] == " " or $cs[$k] == "\t" or $cs[$k] == "\r" or $cs[$k] == "\n" or $cs[$k] == "-")) {
        $k = $k + 1;
    }
    if ($k + 1 < $n and $cs[$k] == "/" and $cs[$k + 1] == "*") {
        # Comment body: find `*/`, then the `}}` after it (allowing a trailing
        # `-` trim marker and whitespace in between).
        def i as int init $k + 2;
        while ($i + 1 < $n) {
            if ($cs[$i] == "*" and $cs[$i + 1] == "/") {
                def j as int init $i + 2;
                while ($j < $n and ($cs[$j] == " " or $cs[$j] == "\t" or $cs[$j] == "-")) {
                    $j = $j + 1;
                }
                if ($j + 1 < $n and $cs[$j] == "}" and $cs[$j + 1] == "}") {
                    return $j;
                }
            }
            $i = $i + 1;
        }
        return -1;
    }
    # Non-comment: the first top-level `}}` outside a quoted string.
    def quote as string init "";
    def i as int init 0;
    while ($i < $n) {
        def c as string init $cs[$i];
        if (len($quote) > 0) {
            if ($c == $quote) {
                $quote = "";
            }
            $i = $i + 1;
        } elseif ($c == "\"" or $c == "'") {
            $quote = $c;
            $i = $i + 1;
        } elseif ($c == "}" and $i + 1 < $n and $cs[$i + 1] == "}") {
            return $i;
        } else {
            $i = $i + 1;
        }
    }
    return -1;
}

# splitPipes splits a pipeline on top-level `|`, ignoring a `|` inside a quoted
# string or parentheses (so `printf "%s|%s"` and a literal `"a|b"` stay one
# stage). Replaces a naive `strings.split(pipeline, "|")`.
func splitPipes(s as string) {
    def out as list of string init [];
    def cur as string init "";
    def cs as list of string init strings.chars($s);
    def n as int init len($cs);
    def i as int init 0;
    def quote as string init "";
    def depth as int init 0;
    while ($i < $n) {
        def c as string init $cs[$i];
        if (len($quote) > 0) {
            $cur = $cur + $c;
            if ($c == $quote) {
                $quote = "";
            }
        } elseif ($c == "\"" or $c == "'") {
            $quote = $c;
            $cur = $cur + $c;
        } elseif ($c == "(") {
            $depth = $depth + 1;
            $cur = $cur + $c;
        } elseif ($c == ")") {
            if ($depth > 0) {
                $depth = $depth - 1;
            }
            $cur = $cur + $c;
        } elseif ($c == "|" and $depth == 0) {
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

func tokenize(expr as string) {
    def toks as list of string init [];
    def cur as string init "";
    def cs as list of string init strings.chars($expr);
    def i as int init 0;
    def n as int init len($cs);
    while ($i < $n) {
        def c as string init $cs[$i];
        if ($c == " " or $c == "\t") {
            if (len($cur) > 0) {
                $toks[] = $cur;
                $cur = "";
            }
            $i = $i + 1;
        } elseif ($c == "(" or $c == ")") {
            if (len($cur) > 0) {
                $toks[] = $cur;
                $cur = "";
            }
            $toks[] = $c;
            $i = $i + 1;
        } elseif ($c == "\"" or $c == "'") {
            if (len($cur) > 0) {
                $toks[] = $cur;
                $cur = "";
            }
            def q as string init $c;
            def s as string init $c;
            $i = $i + 1;
            while ($i < $n and not ($cs[$i] == $q)) {
                $s = $s + $cs[$i];
                $i = $i + 1;
            }
            if ($i < $n) {
                $s = $s + $q;
                $i = $i + 1;
            }
            $toks[] = $s;
        } else {
            $cur = $cur + $c;
            $i = $i + 1;
        }
    }
    if (len($cur) > 0) {
        $toks[] = $cur;
    }
    return $toks;
}

# isFunc reports whether a token names a built-in function usable in an
# expression (comparison / boolean, or printf).
func isFunc(name as string) {
    return $name == "eq" or $name == "ne" or $name == "lt" or $name == "le" or
        $name == "gt" or $name == "ge" or $name == "and" or $name == "or" or
        $name == "not" or $name == "printf";
}

# evalExpr evaluates a prefix expression starting at token `pos`, returning the
# value and the position just past it.
func evalExpr(toks as list of string, pos as int, node as json.Value, root as json.Value, vars as map of string to json.Value) {
    def tok as string init $toks[$pos];
    if ($tok == "(") {
        def inner as Eval init evalExpr($toks, $pos + 1, $node, $root, $vars);
        return Eval{ value: $inner.value, pos: $inner.pos + 1 };
    }
    if (isFunc($tok)) {
        return evalFunc($tok, $toks, $pos + 1, $node, $root, $vars);
    }
    return Eval{ value: resolveTerm($tok, $node, $root, $vars), pos: $pos + 1 };
}

# evalFunc applies a condition / boolean function to its argument expressions.
func evalFunc(name as string, toks as list of string, pos as int, node as json.Value, root as json.Value, vars as map of string to json.Value) {
    if ($name == "printf") {
        def parts as list of json.Value init [];
        def p as int init $pos;
        while ($p < len($toks) and not ($toks[$p] == ")")) {
            def a as Eval init evalExpr($toks, $p, $node, $root, $vars);
            $p = $a.pos;
            $parts[] = $a.value;
        }
        if (len($parts) == 0) {
            return Eval{ value: strVal(""), pos: $p };
        }
        return Eval{ value: strVal(sprintfValues(nodeToString($parts[0]), tailValues($parts))), pos: $p };
    }
    if ($name == "not") {
        def a as Eval init evalExpr($toks, $pos, $node, $root, $vars);
        return Eval{ value: boolVal(not isTruthy($a.value)), pos: $a.pos };
    }
    if ($name == "and" or $name == "or") {
        def acc as bool init ($name == "and");
        def p as int init $pos;
        while ($p < len($toks) and not ($toks[$p] == ")")) {
            def a as Eval init evalExpr($toks, $p, $node, $root, $vars);
            $p = $a.pos;
            if ($name == "and") {
                $acc = $acc and isTruthy($a.value);
            } else {
                $acc = $acc or isTruthy($a.value);
            }
        }
        return Eval{ value: boolVal($acc), pos: $p };
    }
    def a as Eval init evalExpr($toks, $pos, $node, $root, $vars);
    def b as Eval init evalExpr($toks, $a.pos, $node, $root, $vars);
    return Eval{ value: boolVal(compareValues($name, $a.value, $b.value)), pos: $b.pos };
}

# evalExprString evaluates a whole expression string to a value.
func evalExprString(expr as string, node as json.Value, root as json.Value, vars as map of string to json.Value) {
    def toks as list of string init tokenize($expr);
    if (len($toks) == 0) {
        return nullVal();
    }
    return evalExpr($toks, 0, $node, $root, $vars).value;
}

# compareValues applies a comparison function to two values.
func compareValues(op as string, a as json.Value, b as json.Value) {
    def equal as bool init valuesEqual($a, $b);
    if ($op == "eq") {
        return $equal;
    }
    if ($op == "ne") {
        return not $equal;
    }
    def order as int init compareOrder($a, $b);
    if ($op == "lt") {
        return $order < 0;
    }
    if ($op == "le") {
        return $order <= 0;
    }
    if ($op == "gt") {
        return $order > 0;
    }
    return $order >= 0;
}

# valuesEqual is type-aware equality: numbers compare numerically, strings and
# bools by value; mixed types are unequal.
func valuesEqual(a as json.Value, b as json.Value) {
    def ta as string init json.typeOf($a);
    def tb as string init json.typeOf($b);
    if (isNumeric($ta) and isNumeric($tb)) {
        return numOf($a) == numOf($b);
    }
    if ($ta == "string" and $tb == "string") {
        return json.asString($a) == json.asString($b);
    }
    if ($ta == "bool" and $tb == "bool") {
        return json.asBool($a) == json.asBool($b);
    }
    if ($ta == "null" and $tb == "null") {
        return true;
    }
    return false;
}

# compareOrder returns -1 / 0 / 1: numeric when both are numbers, else lexical.
func compareOrder(a as json.Value, b as json.Value) {
    if (isNumeric(json.typeOf($a)) and isNumeric(json.typeOf($b))) {
        def fa as float init numOf($a);
        def fb as float init numOf($b);
        if ($fa < $fb) {
            return -1;
        }
        if ($fa > $fb) {
            return 1;
        }
        return 0;
    }
    def sa as string init nodeToString($a);
    def sb as string init nodeToString($b);
    if ($sa < $sb) {
        return -1;
    }
    if ($sa > $sb) {
        return 1;
    }
    return 0;
}

# --- output pipelines (private) ---------------------------------------------

# evalPipeline evaluates a `term | func | func` pipeline to a value.
func evalPipeline(pipeline as string, node as json.Value, root as json.Value, vars as map of string to json.Value) {
    def segs as list of string init splitPipes($pipeline);
    def val as json.Value init evalExprString(strings.trim($segs[0]), $node, $root, $vars);
    def k as int init 1;
    while ($k < len($segs)) {
        $val = applyPipe(strings.trim($segs[$k]), $val, $node, $root, $vars);
        $k = $k + 1;
    }
    return $val;
}

# evalOutput evaluates a pipeline and renders it as output text.
func evalOutput(pipeline as string, node as json.Value, root as json.Value, vars as map of string to json.Value) {
    return nodeToString(evalPipeline($pipeline, $node, $root, $vars));
}

# applyPipe applies one pipe stage (a function name and optional argument terms)
# to a value, returning the transformed value.
func applyPipe(seg as string, val as json.Value, node as json.Value, root as json.Value, vars as map of string to json.Value) {
    def toks as list of string init tokenize($seg);
    def name as string init $toks[0];
    if ($name == "upper") {
        return strVal(strings.upper(nodeToString($val)));
    }
    if ($name == "lower") {
        return strVal(strings.lower(nodeToString($val)));
    }
    if ($name == "title") {
        return strVal(titleCase(nodeToString($val)));
    }
    if ($name == "trim") {
        return strVal(strings.trim(nodeToString($val)));
    }
    if ($name == "html") {
        return strVal(escapeHtml(nodeToString($val)));
    }
    if ($name == "urlize") {
        return strVal(urlize(nodeToString($val)));
    }
    if ($name == "default") {
        if (isTruthy($val)) {
            return $val;
        }
        return evalExpr($toks, 1, $node, $root, $vars).value;
    }
    if ($name == "truncate") {
        def limit as int init json.asInt(evalExpr($toks, 1, $node, $root, $vars).value);
        return strVal(truncateStr(nodeToString($val), $limit));
    }
    if ($name == "join") {
        def sep as string init nodeToString(evalExpr($toks, 1, $node, $root, $vars).value);
        return strVal(joinList($val, $sep));
    }
    if ($name == "len") {
        return numVal(lengthOf($val));
    }
    if ($name == "printf") {
        def fmtEval as Eval init evalExpr($toks, 1, $node, $root, $vars);
        def args as list of json.Value init [];
        def p as int init $fmtEval.pos;
        while ($p < len($toks)) {
            def a as Eval init evalExpr($toks, $p, $node, $root, $vars);
            $p = $a.pos;
            $args[] = $a.value;
        }
        $args[] = $val;
        return strVal(sprintfValues(nodeToString($fmtEval.value), $args));
    }
    throw Error{ kind: "tengine", message: "tengine: unknown pipe function: " + $name, file: "", line: 0, col: 0 };
}

# --- printf (private) -------------------------------------------------------

# sprintfValues formats `args` per a printf format string. Verbs: %s %v (string),
# %d (integer), %f (float), %t (bool), %% (literal). Flags `-` (left-align) and
# `0` (zero-pad), a width, and `.prec` (float decimals, or a string's max length).
func sprintfValues(format as string, args as list of json.Value) {
    def out as string init "";
    def cs as list of string init strings.chars($format);
    def i as int init 0;
    def n as int init len($cs);
    def ai as int init 0;
    while ($i < $n) {
        if (not ($cs[$i] == "%")) {
            $out = $out + $cs[$i];
            $i = $i + 1;
        } elseif ($i + 1 < $n and $cs[$i + 1] == "%") {
            $out = $out + "%";
            $i = $i + 2;
        } else {
            def spec as Verb init parseVerb($cs, $i + 1);
            $i = $spec.pos;
            def arg as json.Value init nullVal();
            if ($ai < len($args)) {
                $arg = $args[$ai];
                $ai = $ai + 1;
            }
            $out = $out + formatVerb($spec, $arg);
        }
    }
    return $out;
}

# parseVerb reads a verb spec from `cs` starting just after the `%`.
func parseVerb(cs as list of string, start as int) {
    def i as int init $start;
    def n as int init len($cs);
    def leftAlign as bool init false;
    def zeroPad as bool init false;
    while ($i < $n and ($cs[$i] == "-" or $cs[$i] == "0")) {
        if ($cs[$i] == "-") {
            $leftAlign = true;
        } else {
            $zeroPad = true;
        }
        $i = $i + 1;
    }
    def width as int init 0;
    while ($i < $n and isDigit($cs[$i])) {
        $width = $width * 10 + toDigit($cs[$i]);
        $i = $i + 1;
    }
    def prec as int init -1;
    if ($i < $n and $cs[$i] == ".") {
        $i = $i + 1;
        $prec = 0;
        while ($i < $n and isDigit($cs[$i])) {
            $prec = $prec * 10 + toDigit($cs[$i]);
            $i = $i + 1;
        }
    }
    def verb as string init "";
    if ($i < $n) {
        $verb = $cs[$i];
        $i = $i + 1;
    }
    return Verb{ verb: $verb, width: $width, leftAlign: $leftAlign, zeroPad: $zeroPad, prec: $prec, pos: $i };
}

# formatVerb renders one argument per a parsed verb spec.
func formatVerb(spec as Verb, arg as json.Value) {
    if ($spec.verb == "d") {
        return padNumber(convert.toString(intValueOf($arg)), $spec.width, $spec.leftAlign, $spec.zeroPad);
    }
    if ($spec.verb == "f") {
        def p as int init 6;
        if ($spec.prec >= 0) {
            $p = $spec.prec;
        }
        return padNumber(formatFloatStr(numOf($arg), $p), $spec.width, $spec.leftAlign, $spec.zeroPad);
    }
    if ($spec.verb == "t") {
        return padStr(boolStr(isTruthy($arg)), $spec.width, $spec.leftAlign);
    }
    def s as string init nodeToString($arg);
    if ($spec.prec >= 0 and len($s) > $spec.prec) {
        $s = strings.substring($s, 0, $spec.prec);
    }
    return padStr($s, $spec.width, $spec.leftAlign);
}

# padStr space-pads a string to `width` (right-aligned unless leftAlign).
func padStr(s as string, width as int, leftAlign as bool) {
    if (len($s) >= $width) {
        return $s;
    }
    def pad as string init strings.repeat(" ", $width - len($s));
    if ($leftAlign) {
        return $s + $pad;
    }
    return $pad + $s;
}

# padNumber pads a numeric string, honouring zero-pad (after any leading `-`).
func padNumber(s as string, width as int, leftAlign as bool, zeroPad as bool) {
    if (len($s) >= $width) {
        return $s;
    }
    def count as int init $width - len($s);
    if ($leftAlign) {
        return $s + strings.repeat(" ", $count);
    }
    if ($zeroPad) {
        if (strings.startsWith($s, "-")) {
            return "-" + strings.repeat("0", $count) + strings.substring($s, 1, len($s));
        }
        return strings.repeat("0", $count) + $s;
    }
    return strings.repeat(" ", $count) + $s;
}

# formatFloatStr renders a float with exactly `prec` decimal places.
func formatFloatStr(f as float, prec as int) {
    def neg as bool init $f < 0.0;
    def a as float init $f;
    if ($neg) {
        $a = 0.0 - $a;
    }
    def scale as int init 1;
    def k as int init 0;
    while ($k < $prec) {
        $scale = $scale * 10;
        $k = $k + 1;
    }
    def scaled as int init math.round($a * convert.toFloat($scale));
    def s as string init convert.toString($scaled // $scale);
    if ($prec > 0) {
        def frac as string init convert.toString($scaled % $scale);
        while (len($frac) < $prec) {
            $frac = "0" + $frac;
        }
        $s = $s + "." + $frac;
    }
    if ($neg) {
        return "-" + $s;
    }
    return $s;
}

# intValueOf coerces a value to an int for %d (numbers convert; else 0).
func intValueOf(v as json.Value) {
    def t as string init json.typeOf($v);
    if ($t == "int") {
        return json.asInt($v);
    }
    if ($t == "float") {
        return convert.toInt(json.asFloat($v));
    }
    if ($t == "string" and isNumber(json.asString($v))) {
        return convert.toInt(json.asString($v));
    }
    return 0;
}

func boolStr(b as bool) {
    if ($b) {
        return "true";
    }
    return "false";
}

func toDigit(c as string) {
    return strings.indexOf("0123456789", $c);
}

# tailValues returns a list's elements from index 1 onward.
func tailValues(vs as list of json.Value) {
    def out as list of json.Value init [];
    def i as int init 1;
    while ($i < len($vs)) {
        $out[] = $vs[$i];
        $i = $i + 1;
    }
    return $out;
}

# --- value helpers (private) ------------------------------------------------

# nodeToString renders a json.Value as output text (null -> "").
func nodeToString(v as json.Value) {
    def t as string init json.typeOf($v);
    if ($t == "string") {
        return json.asString($v);
    }
    if ($t == "int") {
        return convert.toString(json.asInt($v));
    }
    if ($t == "float") {
        return convert.toString(json.asFloat($v));
    }
    if ($t == "bool") {
        if (json.asBool($v)) {
            return "true";
        }
        return "false";
    }
    if ($t == "null") {
        return "";
    }
    return json.encode($v);
}

# isTruthy reports whether a value counts as true: non-null, non-empty string,
# non-zero number, true, or a non-empty list / map.
func isTruthy(v as json.Value) {
    def t as string init json.typeOf($v);
    if ($t == "null") {
        return false;
    }
    if ($t == "bool") {
        return json.asBool($v);
    }
    if ($t == "string") {
        return len(json.asString($v)) > 0;
    }
    if ($t == "int") {
        return not (json.asInt($v) == 0);
    }
    if ($t == "float") {
        return not (json.asFloat($v) == 0.0);
    }
    if ($t == "list" or $t == "map") {
        return json.length($v) > 0;
    }
    return false;
}

func isNumeric(t as string) {
    return $t == "int" or $t == "float";
}

func numOf(v as json.Value) {
    def t as string init json.typeOf($v);
    if ($t == "int") {
        return convert.toFloat(json.asInt($v));
    }
    if ($t == "float") {
        return json.asFloat($v);
    }
    if ($t == "string" and isNumber(json.asString($v))) {
        return convert.toFloat(json.asString($v));
    }
    return 0.0;
}

func lengthOf(v as json.Value) {
    def t as string init json.typeOf($v);
    if ($t == "string") {
        return len(json.asString($v));
    }
    if ($t == "list" or $t == "map") {
        return json.length($v);
    }
    return 0;
}

# truncateStr keeps the first `n` runes, appending "..." when it shortened.
func truncateStr(s as string, n as int) {
    def cs as list of string init strings.chars($s);
    if (len($cs) <= $n) {
        return $s;
    }
    def out as string init "";
    def i as int init 0;
    while ($i < $n) {
        $out = $out + $cs[$i];
        $i = $i + 1;
    }
    return $out + "...";
}

# joinList joins a list value's elements with `sep`; a non-list stringifies as-is.
func joinList(v as json.Value, sep as string) {
    if (not (json.typeOf($v) == "list")) {
        return nodeToString($v);
    }
    def out as string init "";
    def n as int init json.length($v);
    def i as int init 0;
    while ($i < $n) {
        if ($i > 0) {
            $out = $out + $sep;
        }
        $out = $out + nodeToString(json.get($v, "/" + convert.toString($i)));
        $i = $i + 1;
    }
    return $out;
}

# --- small helpers (private) ------------------------------------------------

# actionKind classifies an action by its leading token.
func actionKind(action as string) {
    if (strings.startsWith($action, "/*")) {
        return "comment";
    }
    if (strings.startsWith($action, "$") and strings.contains($action, ":=")) {
        return "assign";
    }
    def w as string init firstWord($action);
    if ($w == "if" or $w == "range" or $w == "with" or $w == "block" or $w == "define" or $w == "template") {
        return $w;
    }
    if ($action == "end") {
        return "end";
    }
    if ($w == "else") {
        return "else";
    }
    return "output";
}

func firstWord(s as string) {
    def i as int init strings.indexOf($s, " ");
    if ($i < 0) {
        return $s;
    }
    return strings.substring($s, 0, $i);
}

func afterFirstWord(s as string) {
    def i as int init strings.indexOf($s, " ");
    if ($i < 0) {
        return "";
    }
    return strings.substring($s, $i + 1, len($s));
}

func pipelineOf(action as string) {
    return strings.trim(afterFirstWord($action));
}

func stripDollar(s as string) {
    if (strings.startsWith($s, "$")) {
        return strings.substring($s, 1, len($s));
    }
    return $s;
}

# parseRange parses a range pipeline into its optional `$i, $e :=` binding and
# the source expression.
func parseRange(pipeline as string) {
    def assign as int init strings.indexOf($pipeline, ":=");
    if ($assign < 0) {
        return RangeVars{ indexVar: "", elemVar: "", source: $pipeline };
    }
    def lhs as string init strings.trim(strings.substring($pipeline, 0, $assign));
    def source as string init strings.trim(strings.substring($pipeline, $assign + 2, len($pipeline)));
    def comma as int init strings.indexOf($lhs, ",");
    if ($comma < 0) {
        return RangeVars{ indexVar: "", elemVar: stripDollar($lhs), source: $source };
    }
    def idx as string init stripDollar(strings.trim(strings.substring($lhs, 0, $comma)));
    def elem as string init stripDollar(strings.trim(strings.substring($lhs, $comma + 1, len($lhs))));
    return RangeVars{ indexVar: $idx, elemVar: $elem, source: $source };
}

# parseNameArg parses a `"name" arg` tail off an action like `template "body" .`.
func parseNameArg(action as string) {
    def rest as string init strings.trim(afterFirstWord($action));
    def q as string init strings.substring($rest, 0, 1);
    def afterQ as string init strings.substring($rest, 1, len($rest));
    def close as int init strings.indexOf($afterQ, $q);
    def name as string init strings.substring($afterQ, 0, $close);
    def arg as string init strings.trim(strings.substring($afterQ, $close + 1, len($afterQ)));
    return NameArg{ name: $name, arg: $arg };
}

# trimMarkers normalizes `{{-` / `-}}` whitespace-trim markers: `{{-` drops the
# whitespace before the action, `-}}` the whitespace after it. The returned
# source carries plain `{{ }}` actions.
func trimMarkers(src as string) {
    def out as string init "";
    def rest as string init $src;
    while (true) {
        def i as int init strings.indexOf($rest, "{{");
        if ($i < 0) {
            return $out + $rest;
        }
        def pre as string init strings.substring($rest, 0, $i);
        def afterOpen as string init strings.substring($rest, $i + 2, len($rest));
        def j as int init closeActionIndex($afterOpen);
        if ($j < 0) {
            throw Error{ kind: "tengine", message: "tengine: unterminated action", file: "", line: 0, col: 0 };
        }
        def inner as string init strings.substring($afterOpen, 0, $j);
        def tail as string init strings.substring($afterOpen, $j + 2, len($afterOpen));
        def trimLeft as bool init strings.startsWith($inner, "-");
        def trimRight as bool init strings.endsWith($inner, "-");
        def clean as string init $inner;
        if ($trimLeft) {
            $clean = strings.substring($clean, 1, len($clean));
        }
        if ($trimRight and len($clean) > 0) {
            $clean = strings.substring($clean, 0, len($clean) - 1);
        }
        if ($trimLeft) {
            $pre = strings.trimRight($pre);
        }
        $out = $out + $pre + "{{" + $clean + "}}";
        if ($trimRight) {
            $tail = strings.trimLeft($tail);
        }
        $rest = $tail;
    }
    return $out;
}

# escapeHtml replaces the five HTML-significant characters with entities.
func escapeHtml(s as string) {
    def r as string init strings.replace($s, "&", "&amp;");
    $r = strings.replace($r, "<", "&lt;");
    $r = strings.replace($r, ">", "&gt;");
    $r = strings.replace($r, "\"", "&quot;");
    $r = strings.replace($r, "'", "&#39;");
    return $r;
}

# titleCase upper-cases the first letter of each space-separated word.
func titleCase(s as string) {
    def out as string init "";
    def first as bool init true;
    for (def w in strings.split($s, " ")) {
        if (not $first) {
            $out = $out + " ";
        }
        $first = false;
        if (len($w) > 0) {
            $out = $out + strings.upper(strings.substring($w, 0, 1)) + strings.substring($w, 1, len($w));
        }
    }
    return $out;
}

# urlize slugifies a string: lower-case, alphanumerics kept, spaces / dashes /
# underscores collapsed to single dashes.
func urlize(s as string) {
    def out as string init "";
    for (def c in strings.chars(strings.lower($s))) {
        if (isAlnum($c)) {
            $out = $out + $c;
        } elseif ($c == " " or $c == "-" or $c == "_") {
            $out = $out + "-";
        }
    }
    while (strings.contains($out, "--")) {
        $out = strings.replace($out, "--", "-");
    }
    return $out;
}

func isAlnum(c as string) {
    return strings.contains("abcdefghijklmnopqrstuvwxyz0123456789", $c);
}

func isDigit(c as string) {
    return strings.contains("0123456789", $c);
}

# isNumber reports whether a term is an integer / float literal.
func isNumber(s as string) {
    if (len($s) == 0) {
        return false;
    }
    def cs as list of string init strings.chars($s);
    if (not (isDigit($cs[0]) or $cs[0] == "-")) {
        return false;
    }
    def i as int init 0;
    while ($i < len($cs)) {
        def c as string init $cs[$i];
        if (not (isDigit($c) or $c == "." or $c == "-")) {
            return false;
        }
        $i = $i + 1;
    }
    return true;
}

# strVal / numVal / boolVal / nullVal build json.Values for literals.
func strVal(s as string) {
    return json.decode(json.encode($s));
}

func numVal(n as int) {
    return json.decode(convert.toString($n));
}

func boolVal(b as bool) {
    if ($b) {
        return json.decode("true");
    }
    return json.decode("false");
}

func nullVal() {
    return json.decode("null");
}

func emptyVars() {
    def v as map of string to json.Value init {};
    return $v;
}
