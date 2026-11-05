# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.25.0
#
# rst_refs.inc.j - the reference pass of the rst module: hyperlink targets and
# references, footnotes, citations and substitutions. Included (textual splice)
# into rst.j; it declares no `use` of its own and relies on the includer's
# imports and helpers. Private to the rst module.
#
# The block and inline parsers leave placeholders in the tree, because a
# reference can come before its target:
#
#   rst_ref     an inline reference: `lang` says which kind ("named", "anon",
#               "embedded", "embedded_anon", "alias", "footnote", "citation",
#               "subst"), `text` the (normalised) name, `url` an embedded URI,
#               `children` the link text, attrs `key` (its position, unique in
#               the document), `raw` (its source text) and `suffix` (a
#               substitution's `_` / `__`)
#   rst_target  a hyperlink target: `lang` "external" / "indirect" / "internal"
#               / "anon_external" / "anon_indirect", `text` the normalised
#               name, `title` the name as written, `url` the URI or refname
#   rst_subst   a substitution definition: `text` the name, `children` the
#               replacement
#   rst_itarget an inline target (_`text`): `text` the name as written
#
# and footnote / citation definitions carry attrs `rst-kind` ("manual",
# "auto", "autolabel", "symbol", "citation"), `rst-name` and `rst-key`.
#
# `collect` walks the tree once in document order and records all of that in a
# Refs value; `finishRefs` numbers footnotes and pairs anonymous references with
# anonymous targets; `resolveKids` rebuilds the tree with every placeholder
# replaced by a link, a note reference, a substitution or a diagnostic.

# The footnote symbols, in docutils' order. Past ten, a symbol repeats.
def const FOOTNOTE_SYMBOLS as list of string init [
    "*",
    "†",
    "‡",
    "§",
    "¶",
    "#",
    "♠",
    "♥",
    "♦",
    "♣"
];

# How deep substitutions may nest (a replacement that uses a substitution), and
# how long a chain of indirect targets may be, before it counts as a loop.
def const MAX_SUBST_DEPTH as int init 8;
def const MAX_INDIRECT as int init 20;

def struct FootDef {
    key as string,
    kind as string,
    name as string,
    line as int
};

def struct FootRef {
    key as string,
    name as string,
    line as int
};

def struct Refs {
    count as int,
    # explicit targets: normalised name -> URI, "#id", or "=" + a refname
    named as map of string to string,
    # implicit targets (section titles): normalised name -> "#id"
    implicit as map of string to string,
    anonTargets as list of string,
    anonRefs as list of string,
    footDefs as list of FootDef,
    footRefs as list of FootRef,
    citations as map of string to string,
    substs as map of string to list of doctree.Node,
    diags as list of Diagnostic,
    # filled in by finishRefs
    anonByKey as map of string to string,
    noteByKey as map of string to string,
    noteLabelByKey as map of string to string,
    refIdByKey as map of string to string,
    backrefByKey as map of string to string
};

func newRefs() {
    def r as Refs;
    return $r;
}

# normName is docutils' reference-name normalisation: case-insensitive, with
# runs of whitespace taken as one space.
func normName(s as string) {
    return regex.replace('\s+', strings.lower(strings.trim($s)), " ");
}

func refDiag(refs as Refs, severity as string, line as int, message as string) { # lint-disable: L205
    $refs.diags[] = Diagnostic{severity: $severity, line: $line, message: $message};
    return $refs;
}

func isPlaceholder(kind as string) {
    return $kind == "rst_target" or $kind == "rst_subst" or $kind == "rst_diag";
}

# --- collecting -------------------------------------------------------

# collect records the targets, references, notes and substitutions under a node
# in document order. Call it as `$refs = collect($refs, node);`.
func collect(refs as Refs, n as doctree.Node) { # lint-disable: L205
    # Internal targets (`.. _name:`) waiting for the element they point at.
    def pending as list of string init [];
    def pendingLines as list of int init [];
    for (def k in 0..len($n.children)) {
        def kind as string init $n.children[$k].kind;
        if ($kind == "rst_target" and $n.children[$k].lang == "internal") {
            $pending[] = $n.children[$k].title;
            $pendingLines[] = $n.children[$k].level;
            $refs.count = $refs.count + 1;
            continue;
        }
        if (len($pending) > 0 and not isPlaceholder($kind)) {
            def id as string init elementId($n.children[$k], $pending[0]);
            $refs = addInternal($refs, $pending, $pendingLines, $id);
            $pending = [];
            $pendingLines = [];
        }
        if (len($n.children[$k].children) == 0 and not strings.startsWith($kind, "rst_") and
            $kind != "footnote" and $kind != "citation") {
            # A plain leaf (text, code) holds nothing to collect.
            continue;
        }
        $refs = collectOne($refs, $n.children[$k]);
    }
    if (len($pending) > 0) {
        $refs = addInternal($refs, $pending, $pendingLines, slugOf($pending[0]));
    }
    return $refs;
}

# elementId is the id an internal target gives the element after it: the
# element's own id when it has one, else one made from the first target's name.
# resolveKids applies the same rule, so the two passes agree.
func elementId(node as doctree.Node, firstName as string) {
    if (maps.has($node.attrs, "id")) {
        return $node.attrs["id"];
    }
    return slugOf($firstName);
}

func addInternal(refs as Refs, names as list of string, lines as list of int, id as string) { # lint-disable: L205
    for (def k in 0..len($names)) {
        $refs = addNamed($refs, normName($names[$k]), "#" + $id, $lines[$k]);
    }
    return $refs;
}

func addNamed(refs as Refs, name as string, target as string, line as int) { # lint-disable: L205
    if (maps.has($refs.named, $name) and $refs.named[$name] != $target) {
        $refs = refDiag(
            $refs,
            "warning",
            $line,
            "Duplicate explicit target name: \"" + $name + "\".");
        return $refs;
    }
    $refs.named[$name] = $target;
    return $refs;
}

# collectOne records one node (and, through collect, everything under it).
func collectOne(refs as Refs, c as doctree.Node) { # lint-disable: L205
    match ($c.kind) {
        when "rst_target" {
            $refs.count = $refs.count + 1;
            $refs = addTarget($refs, $c);
            return $refs;
        }
        when "rst_subst" {
            $refs.count = $refs.count + 1;
            if (maps.has($refs.substs, $c.text)) {
                $refs = refDiag(
                    $refs,
                    "warning",
                    $c.level,
                    "Duplicate substitution definition name: \"" + $c.text + "\".");
            } else {
                $refs.substs[$c.text] = $c.children;
            }
        }
        when "rst_ref" {
            $refs.count = $refs.count + 1;
            $refs = addRef($refs, $c);
        }
        when "rst_itarget" {
            $refs.count = $refs.count + 1;
            $refs = addNamed($refs, normName($c.text), "#" + slugOf($c.text), $c.level);
        }
        when "heading" {
            def name as string init normName(doctree.text($c));
            if (maps.has($c.attrs, "id") and not maps.has($refs.implicit, $name)) {
                $refs.implicit[$name] = "#" + $c.attrs["id"];
            }
        }
        when "footnote", "citation" {
            if (maps.has($c.attrs, "rst-kind")) {
                $refs.count = $refs.count + 1;
                $refs = addNote($refs, $c);
            }
        }
    }
    if (len($c.children) > 0) {
        $refs = collect($refs, $c);
    }
    return $refs;
}

func addTarget(refs as Refs, t as doctree.Node) { # lint-disable: L205
    match ($t.lang) {
        when "external" { $refs = addNamed($refs, $t.text, $t.url, $t.level); }
        when "indirect" { $refs = addNamed($refs, $t.text, "=" + $t.url, $t.level); }
        when "anon_external" { $refs.anonTargets[] = $t.url; }
        when "anon_indirect" { $refs.anonTargets[] = "=" + $t.url; }
    }
    return $refs;
}

func addRef(refs as Refs, r as doctree.Node) { # lint-disable: L205
    match ($r.lang) {
        when "anon" { $refs.anonRefs[] = $r.attrs["key"]; }
        when "embedded" {
            # `text <uri>`_ also defines "text" as a target (with `__` it does not).
            $refs = addNamed($refs, $r.text, $r.url, $r.level);
        }
        when "footnote" {
            $refs.footRefs[] = FootRef{key: $r.attrs["key"], name: $r.text, line: $r.level};
        }
        when "subst" {
            if ($r.attrs["suffix"] == "__") {
                $refs.anonRefs[] = $r.attrs["key"];
            }
        }
    }
    return $refs;
}

func addNote(refs as Refs, c as doctree.Node) { # lint-disable: L205
    def kind as string init $c.attrs["rst-kind"];
    def name as string init $c.attrs["rst-name"];
    if ($kind == "citation") {
        def norm as string init normName($name);
        if (maps.has($refs.citations, $norm)) {
            $refs = refDiag(
                $refs,
                "warning",
                $c.level,
                "Duplicate citation label: \"" + $name + "\".");
        } else {
            $refs.citations[$norm] = "citation-" + slugOf($name);
        }
        return $refs;
    }
    $refs.footDefs[] = FootDef{key: $c.attrs["rst-key"], kind: $kind, name: $name, line: $c.level};
    return $refs;
}

# --- numbering and pairing -----------------------------------------------

func symbolLabel(k as int) {
    return strings.repeat(FOOTNOTE_SYMBOLS[$k % 10], $k // 10 + 1);
}

# nextFreeNumber is the smallest footnote number from `from` on that no
# footnote has taken yet.
func nextFreeNumber(used as map of string to bool, from as int) {
    def k as int init $from;
    while (maps.has($used, "{$k}")) {
        $k = $k + 1;
    }
    return $k;
}

# finishRefs numbers the footnotes the way docutils does - manual numbers stay,
# auto-numbered ones (`[#]`, `[#label]`) take the next free number in document
# order, symbol ones (`[*]`) the next symbol - then matches every footnote
# reference to its note, gives each reference an id and each note a link back to
# its first reference, and pairs anonymous references with anonymous targets in
# order. Call it as `$refs = finishRefs($refs);`.
func finishRefs(refs as Refs) { # lint-disable: L205
    def used as map of string to bool init {};
    for (def d in $refs.footDefs) {
        if ($d.kind == "manual") {
            $used[$d.name] = true;
        }
    }
    # label -> note id for numbered notes, "#name" -> label, and the anonymous
    # auto-numbered and symbol labels in order.
    def byLabel as map of string to string init {};
    def byAutoName as map of string to string init {};
    def anonAuto as list of string init [];
    def symbols as list of string init [];
    def next as int init 1;
    for (def d in $refs.footDefs) {
        def label as string init $d.name;
        def id as string init "";
        if ($d.kind == "symbol") {
            def nth as int init len($symbols) + 1;
            $label = symbolLabel($nth - 1);
            $id = "footnote-sym-{$nth}";
            $symbols[] = $label;
        } else {
            if ($d.kind != "manual") {
                $next = nextFreeNumber($used, $next);
                $label = "{$next}";
                $used[$label] = true;
            }
            $id = "footnote-" + $label;
            if (maps.has($byLabel, $label)) {
                $refs = refDiag(
                    $refs,
                    "warning",
                    $d.line,
                    "Duplicate footnote label: \"" + $label + "\".");
            }
            $byLabel[$label] = $id;
        }
        if ($d.kind == "auto") {
            $anonAuto[] = $label;
        }
        if ($d.kind == "autolabel") {
            $byAutoName[normName($d.name)] = $label;
        }
        $refs.noteLabelByKey[$d.key] = $label;
        $refs.noteByKey[$d.key] = $id;
    }
    $refs = pairFootRefs($refs, $byLabel, $byAutoName, $anonAuto, $symbols);
    $refs = pairAnonymous($refs);
    return $refs;
}

func pairFootRefs(
    refs as Refs,
    byLabel as map of string to string,
    byAutoName as map of string to string,
    anonAuto as list of string,
    symbols as list of string) { # lint-disable: L205
    def nAuto as int init 0;
    def nSym as int init 0;
    # note id -> id of its first reference
    def firstRef as map of string to string init {};
    for (def k in 0..len($refs.footRefs)) {
        def r as FootRef init $refs.footRefs[$k];
        def label as string init "";
        if ($r.name == "#") {
            if ($nAuto < len($anonAuto)) {
                $label = $anonAuto[$nAuto];
            }
            $nAuto = $nAuto + 1;
        } elseif ($r.name == "*") {
            if ($nSym < len($symbols)) {
                $label = $symbols[$nSym];
            }
            $nSym = $nSym + 1;
        } elseif (strings.startsWith($r.name, "#")) {
            def auto as string init normName($r.name[1..]);
            if (maps.has($byAutoName, $auto)) {
                $label = $byAutoName[$auto];
            }
        } elseif (maps.has($byLabel, $r.name)) {
            $label = $r.name;
        }
        if ($label == "") {
            $refs = refDiag(
                $refs,
                "error",
                $r.line,
                "Unknown footnote reference: \"[" + $r.name + "]_\".");
            continue;
        }
        def noteId as string init "footnote-" + $label;
        if ($r.name == "*") {
            $noteId = "footnote-sym-{$nSym}";
        }
        def refId as string init "footnote-reference-{$k + 1}";
        $refs.noteByKey[$r.key] = $noteId;
        $refs.noteLabelByKey[$r.key] = $label;
        $refs.refIdByKey[$r.key] = $refId;
        if (not maps.has($firstRef, $noteId)) {
            $firstRef[$noteId] = $refId;
        }
    }
    for (def d in $refs.footDefs) {
        def id as string init $refs.noteByKey[$d.key];
        if (maps.has($firstRef, $id)) {
            $refs.backrefByKey[$d.key] = $firstRef[$id];
        }
    }
    return $refs;
}

func pairAnonymous(refs as Refs) { # lint-disable: L205
    def nr as int init len($refs.anonRefs);
    def nt as int init len($refs.anonTargets);
    if ($nr != $nt) {
        $refs = refDiag(
            $refs,
            "error",
            0,
            "Anonymous hyperlink mismatch: {$nr} references but {$nt} targets.");
    }
    for (def k in 0..$nr) {
        if ($k < $nt) {
            $refs.anonByKey[$refs.anonRefs[$k]] = $refs.anonTargets[$k];
        }
    }
    return $refs;
}

# lookupName resolves a normalised reference name to a URI or "#id": explicit
# targets first, then section titles, following indirect targets. "" when the
# name is unknown or the chain loops.
func lookupName(refs as Refs, name as string) {
    def cur as string init $name;
    for (def hop in 0..MAX_INDIRECT) {
        def v as string init "";
        if (maps.has($refs.named, $cur)) {
            $v = $refs.named[$cur];
        } elseif (maps.has($refs.implicit, $cur)) {
            $v = $refs.implicit[$cur];
        } else {
            return "";
        }
        if (not strings.startsWith($v, "=")) {
            return $v;
        }
        $cur = $v[1..];
    }
    return "";
}

# --- resolving ------------------------------------------------------------

func linkNode(url as string, kids as list of doctree.Node) {
    def n as doctree.Node init withChildren("link", $kids);
    $n.url = $url;
    return $n;
}

func rawText(r as doctree.Node) {
    return textOf($r.attrs["raw"]);
}

# resolveKids rebuilds a child list with every placeholder resolved. It reads
# the old tree and builds a new one, so no subtree is copied more than once.
func resolveKids(kids as list of doctree.Node, refs as Refs, depth as int) {
    def out as list of doctree.Node init [];
    def pending as string init "";
    for (def k in 0..len($kids)) {
        def kind as string init $kids[$k].kind;
        if ($kind == "rst_target") {
            if ($kids[$k].lang == "internal" and $pending == "") {
                $pending = $kids[$k].title;
            }
            continue;
        }
        if ($kind == "rst_subst") {
            continue;
        }
        if ($kind == "rst_ref") {
            for (def x in resolveRef($kids[$k], $refs, $depth)) {
                $out[] = $x;
            }
            continue;
        }
        if ($kind == "rst_itarget") {
            def span as doctree.Node init withChildren("span", [textOf($kids[$k].text)]);
            $span.attrs = {"id": slugOf($kids[$k].text), "class": "target"};
            $out[] = $span;
            continue;
        }
        if ($pending == "" or $kind == "rst_diag") {
            # The common case appends the rebuilt node straight away: holding it
            # in a local first would copy the whole subtree.
            $out[] = resolveNode($kids[$k], $refs, $depth);
            continue;
        }
        def node as doctree.Node init resolveNode($kids[$k], $refs, $depth);
        if ($pending != "") {
            if (not maps.has($node.attrs, "id")) {
                $node.attrs["id"] = slugOf($pending);
            }
            $pending = "";
        }
        $out[] = $node;
    }
    if ($pending != "") {
        # A target with nothing after it still needs an anchor.
        def anchor as doctree.Node init mk("container");
        $anchor.attrs = {"id": slugOf($pending)};
        $out[] = $anchor;
    }
    return $out;
}

func resolveNode(n as doctree.Node, refs as Refs, depth as int) {
    if (len($n.children) == 0 and not maps.has($n.attrs, "rst-kind")) {
        return $n;
    }
    def attrs as map of string to string init $n.attrs;
    if (maps.has($attrs, "rst-kind")) {
        $attrs = noteAttrs($n, $refs);
    }
    return doctree.Node{
        kind: $n.kind,
        level: $n.level,
        text: $n.text,
        lang: $n.lang,
        ordered: $n.ordered,
        url: $n.url,
        title: $n.title,
        align: $n.align,
        attrs: $attrs,
        children: resolveKids($n.children, $refs, $depth)
    };
}

# noteAttrs gives a footnote or citation definition its final label, id and
# backlink, and drops the parser's bookkeeping attributes.
func noteAttrs(n as doctree.Node, refs as Refs) {
    def out as map of string to string init {};
    for (def k in maps.keys($n.attrs)) {
        if (not strings.startsWith($k, "rst-")) {
            $out[$k] = $n.attrs[$k];
        }
    }
    def key as string init $n.attrs["rst-key"];
    if ($n.kind == "citation") {
        $out["label"] = $n.attrs["rst-name"];
        $out["id"] = "citation-" + slugOf($n.attrs["rst-name"]);
        return $out;
    }
    $out["label"] = $refs.noteLabelByKey[$key];
    $out["id"] = $refs.noteByKey[$key];
    if (maps.has($refs.backrefByKey, $key)) {
        $out["backref"] = $refs.backrefByKey[$key];
    }
    return $out;
}

func resolveRef(r as doctree.Node, refs as Refs, depth as int) {
    match ($r.lang) {
        when "named", "alias" { return namedLink($r, $r.text, $refs); }
        when "embedded", "embedded_anon" { return [linkNode($r.url, $r.children)]; }
        when "anon" { return anonLink($r, $refs); }
        when "footnote" { return [noteRef($r, $refs)]; }
        when "citation" { return citationRef($r, $refs); }
        when "subst" { return substitute($r, $refs, $depth); }
    }
    return [rawText($r)];
}

func namedLink(r as doctree.Node, name as string, refs as Refs) {
    def url as string init lookupName($refs, $name);
    if ($url == "") {
        return [
            diagNode("error", $r.level, "Unknown target name: \"" + $name + "\"."),
            rawText($r)
        ];
    }
    return [linkNode($url, $r.children)];
}

func anonLink(r as doctree.Node, refs as Refs) {
    def key as string init $r.attrs["key"];
    if (not maps.has($refs.anonByKey, $key)) {
        # The mismatch is already reported once for the whole document.
        return [rawText($r)];
    }
    def target as string init $refs.anonByKey[$key];
    if (strings.startsWith($target, "=")) {
        return namedLink($r, $target[1..], $refs);
    }
    return [linkNode($target, $r.children)];
}

func noteRef(r as doctree.Node, refs as Refs) {
    def key as string init $r.attrs["key"];
    if (not maps.has($refs.refIdByKey, $key)) {
        return rawText($r);
    }
    def n as doctree.Node init mk("footnote_ref");
    $n.text = $refs.noteLabelByKey[$key];
    $n.url = "#" + $refs.noteByKey[$key];
    $n.attrs = {"id": $refs.refIdByKey[$key]};
    return $n;
}

func citationRef(r as doctree.Node, refs as Refs) {
    def norm as string init normName($r.text);
    if (not maps.has($refs.citations, $norm)) {
        return [
            diagNode("error", $r.level, "Unknown target name: \"" + $norm + "\"."),
            rawText($r)
        ];
    }
    def n as doctree.Node init mk("citation_ref");
    $n.text = $r.text;
    $n.url = "#" + $refs.citations[$norm];
    return [$n];
}

# substitute replaces a substitution reference with its definition (matched
# exactly, then case-insensitively), itself resolved, and links it when the
# reference ends in `_` / `__`.
func substitute(r as doctree.Node, refs as Refs, depth as int) {
    def name as string init $r.text;
    if (not maps.has($refs.substs, $name)) {
        $name = substByCase($refs, $r.text);
    }
    if ($name == "") {
        return [
            diagNode("error", $r.level, "Undefined substitution referenced: \"" + $r.text + "\"."),
            rawText($r)
        ];
    }
    if ($depth >= MAX_SUBST_DEPTH) {
        return [
            diagNode(
                "error",
                $r.level,
                "Circular substitution definition referenced: \"" + $r.text + "\"."),
            rawText($r)
        ];
    }
    def kids as list of doctree.Node init resolveKids($refs.substs[$name], $refs, $depth + 1);
    def suffix as string init $r.attrs["suffix"];
    if ($suffix == "_") {
        def url as string init lookupName($refs, normName($r.text));
        if ($url == "") {
            return [
                diagNode("error", $r.level, "Unknown target name: \"" + normName($r.text) + "\"."),
                withChildren("span", $kids)
            ];
        }
        return [linkNode($url, $kids)];
    }
    if ($suffix == "__" and maps.has($refs.anonByKey, $r.attrs["key"])) {
        def target as string init $refs.anonByKey[$r.attrs["key"]];
        if (strings.startsWith($target, "=")) {
            $target = lookupName($refs, $target[1..]);
        }
        return [linkNode($target, $kids)];
    }
    return $kids;
}

func substByCase(refs as Refs, name as string) {
    def want as string init strings.lower($name);
    for (def k in maps.keys($refs.substs)) {
        if (strings.lower($k) == $want) {
            return $k;
        }
    }
    return "";
}
