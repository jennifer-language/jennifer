# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
# pragma-jennifer-version: >=0.24.0

/**
 * Build an HTML element tree and render it to a correctly escaped HTML5 string.
 * Pure Jennifer over `strings` and `lists` - a writer, not a parser, so it has
 * no dependency on an XML parser: serialization is a handful of string
 * operations. The shared output layer any HTML-emitting consumer reuses (a
 * Markdown renderer, a documentation generator, a view layer). Text nodes are
 * escaped on render (`&` `<` `>`); attribute values also escape `"`; a `raw`
 * node passes through verbatim for already-trusted markup. Void elements (`br`,
 * `img`, ...) render without a closing tag and drop children.
 * @module html
 * @example
 * import "html.j" as html;
 * def kids as list of html.Node init [];
 * $kids[] = html.text("hi & bye");
 * def p as html.Node init html.element("p", [], $kids);
 * io.printf("%s\n", html.render($p));   # <p>hi &amp; bye</p>
 */

use strings;
use lists;
use convert;

/**
 * The kind of an HTML node: `Element` (a tag with attributes and children),
 * `Text` (escaped text content), or `Raw` (verbatim, already-trusted markup).
 */
export def enum NodeKind { Element, Text, Raw };

/**
 * A node is one of three kinds, tagged by `kind`: "element" (tag + attrs +
 * children), "text" (escaped content), or "raw" (verbatim content). The
 * constructors below are the intended way to build one.
 * @field kind {NodeKind} the node kind (`Element`, `Text`, or `Raw`)
 * @field tag {string} the element tag name (element nodes only)
 * @field attrs {list of Attr} the element's attributes (element nodes only)
 * @field children {list of Node} the element's child nodes (element nodes only)
 * @field text {string} the content of a text or raw node
 */
export def struct Node {
    kind as NodeKind,
    tag as string,
    attrs as list of Attr,
    children as list of Node,
    text as string
};

/**
 * One HTML attribute. A normal attribute has a name and a value and renders as
 * `name="value"`; a boolean attribute (`boolean: true`, built by `boolAttr`)
 * carries no value and renders as the bare name (`disabled`, `checked`, ...).
 * @field name {string} the attribute name
 * @field value {string} the attribute value (escaped on render; empty for a boolean attribute)
 * @field boolean {bool} true for a valueless boolean attribute (rendered as the bare name)
 */
export def struct Attr {
    name as string,
    value as string,
    boolean as bool
};

# The HTML5 void elements: they never have a closing tag or children.
def const VOID as list of string init [
    "area",
    "base",
    "br",
    "col",
    "embed",
    "hr",
    "img",
    "input",
    "link",
    "meta",
    "param",
    "source",
    "track",
    "wbr"
]; # lint-disable: L203

# --- constructors (exported) ---------------------------------------

/**
 * Build an element node from a tag, its attributes, and its children. Pass `[]`
 * for either when there are none.
 * @param tag {string} the element tag name
 * @param attrs {list of Attr} the element's attributes
 * @param children {list of Node} the element's child nodes
 * @return {Node} the element node
 */
export func element(tag as string, attrs as list of Attr, children as list of Node) {
    checkName($tag, "tag");
    return Node{kind: NodeKind.Element, tag: $tag, attrs: $attrs, children: $children, text: ""};
}

/**
 * Build a text node; its content is HTML-escaped on render.
 * @param s {string} the text content
 * @return {Node} the text node
 */
export func text(s as string) {
    return Node{kind: NodeKind.Text, tag: "", attrs: [], children: [], text: $s};
}

/**
 * Build a node whose content is emitted verbatim - for already-trusted markup
 * only, since it is not escaped.
 * @param s {string} the verbatim markup
 * @return {Node} the raw node
 */
export func raw(s as string) {
    return Node{kind: NodeKind.Raw, tag: "", attrs: [], children: [], text: $s};
}

/**
 * Build one name/value attribute for an element.
 * @param name {string} the attribute name
 * @param value {string} the attribute value
 * @return {Attr} the attribute
 */
export func attr(name as string, value as string) {
    checkName($name, "attribute");
    return Attr{name: $name, value: $value, boolean: false};
}

/**
 * Build a boolean (valueless) HTML attribute - `disabled`, `checked`,
 * `selected`, `required`, `readonly`, `multiple`, `autofocus`, and the like.
 * It renders as the bare name (`<input disabled>`), not `name=""`. The name is
 * validated exactly as `attr` validates it.
 * @param name {string} the attribute name
 * @return {Attr} the boolean attribute
 */
export func boolAttr(name as string) {
    checkName($name, "attribute");
    return Attr{name: $name, value: "", boolean: true};
}

# --- escaping (private + one public helper) ------------------------

/**
 * Return s with the text-context HTML metacharacters replaced by entities (`&`
 * first, so an escaped entity is not re-escaped). Public because escaping a bare
 * string for HTML text is useful without building a node.
 * @param s {string} the string to escape
 * @return {string} the escaped string
 */
export func escape(s as string) {
    def out as string init strings.replace($s, "&", "&amp;");
    $out = strings.replace($out, "<", "&lt;");
    $out = strings.replace($out, ">", "&gt;");
    return $out;
}

# escapeAttr additionally escapes the double quote, since attribute values are
# rendered inside double quotes.
func escapeAttr(s as string) {
    def out as string init escape($s);
    return strings.replace($out, '"', "&quot;");
}

# --- name + URL safety (validation) --------------------------------

# validName reports whether s is a safe HTML tag or attribute name: a letter,
# then letters, digits, or '-'. This is the `^[A-Za-z][A-Za-z0-9-]*$` shape.
# A name outside it (a space, '>', '=', '/', a quote) is how a data-driven tag
# or attribute forges markup - `attr("x onclick=alert(1)", "y")` would emit a
# live event handler - so both constructors reject it (OM-011).
func validName(s as string) {
    def raw as bytes init convert.bytesFromString($s, "utf-8");
    if (len($raw) == 0) {
        return false;
    }
    def i as int init 0;
    while ($i < len($raw)) {
        def b as int init $raw[$i];
        def alpha as bool init ($b >= 65 and $b <= 90) or ($b >= 97 and $b <= 122);
        def digit as bool init $b >= 48 and $b <= 57;
        if ($i == 0) {
            if (not $alpha) {
                return false;
            }
        } else {
            if (not ($alpha or $digit or $b == 45)) {
                return false;
            }
        }
        $i = $i + 1;
    }
    return true;
}

# checkName throws when a tag / attribute name is not validName.
func checkName(name as string, what as string) {
    if (not validName($name)) {
        throw Error{
            kind: "html",
            message: "html: illegal " + $what + " name (must match [A-Za-z][A-Za-z0-9-]*): " +
                $name,
            file: "",
            line: 0,
            col: 0
        };
    }
    return;
}

/**
 * Return a URL safe to place in an `href` / `src` attribute, or `"#"` if its
 * scheme is not one of `http` / `https` / `mailto`. Whitespace and control
 * characters are ignored while reading the scheme (so `"java\tscript:..."` is
 * still caught), and a relative reference (no scheme) is returned unchanged.
 * This is the anti-XSS gate for building links from untrusted input.
 * @param url {string} the URL to check
 * @return {string} the URL if its scheme is allowed, else `"#"`
 */
export func safeUrl(url as string) {
    # Iterate by rune (not byte): a non-ASCII URL must not error, and the scheme
    # we test for is ASCII anyway. Drop whitespace / control characters
    # (codepoint <= 32) while reading the scheme so "java\tscript:" is still caught.
    def cs as list of string init strings.chars($url);
    def probe as string init "";
    def i as int init 0;
    while ($i < len($cs)) {
        if (convert.toCodepoint($cs[$i]) > 32) {
            $probe = $probe + $cs[$i];
        }
        $i = $i + 1;
    }
    if (len($probe) == 0) {
        return $url;
    }
    # The scheme is the run before the first ':', but only if that ':' comes
    # before any '/', '?', or '#' (else there is no scheme and the reference is
    # relative, hence safe).
    def scheme as string init "";
    def hasScheme as bool init false;
    def j as int init 0;
    def pn as int init len($probe);
    while ($j < $pn) {
        def ch as string init strings.substring($probe, $j, $j + 1);
        if ($ch == ":") {
            $hasScheme = true;
            break;
        }
        if ($ch == "/" or $ch == "?" or $ch == "#") {
            break;
        }
        $scheme = $scheme + $ch;
        $j = $j + 1;
    }
    if (not $hasScheme) {
        return $url;
    }
    def low as string init strings.lower($scheme);
    if ($low == "http" or $low == "https" or $low == "mailto") {
        return $url;
    }
    return "#";
}

# --- rendering (exported) ------------------------------------------

# isVoid reports whether a tag is an HTML5 void element (case-insensitive).
func isVoid(tag as string) {
    return lists.contains(VOID, strings.lower($tag));
}

# renderAttrs renders a leading-space-separated attribute list.
func renderAttrs(attrs as list of Attr) {
    def out as string init "";
    for (def a in $attrs) {
        if ($a.boolean) {
            $out = $out + " " + $a.name;
        } else {
            $out = $out + " " + $a.name + "=\"" + escapeAttr($a.value) + "\"";
        }
    }
    return $out;
}

/**
 * Serialize a node and its subtree to an HTML5 string.
 * @param node {Node} the node to render
 * @return {string} the rendered HTML
 */
export func render(node as Node) {
    match ($node.kind) {
        when Text {
            return escape($node.text);
        }
        when Raw {
            return $node.text;
        }
        when Element {
            def open as string init "<" + $node.tag + renderAttrs($node.attrs);
            if (isVoid($node.tag)) {
                return $open + ">";
            }
            # Collect the pieces and join once. Growing a string with `+` per
            # child is O(output^2), so a node with many children (a paragraph
            # full of links) would otherwise be quadratic in its rendered size.
            def parts as list of string init [$open + ">"];
            for (def child in $node.children) {
                $parts[] = render($child);
            }
            $parts[] = "</" + $node.tag + ">";
            return strings.join($parts, "");
        }
    }
}

/**
 * Serialize a list of sibling nodes (a fragment) in order.
 * @param nodes {list of Node} the sibling nodes
 * @return {string} the rendered HTML fragment
 */
export func renderAll(nodes as list of Node) {
    def parts as list of string init [];
    for (def n in $nodes) {
        $parts[] = render($n);
    }
    return strings.join($parts, "");
}

# ============================================================================
# Parsing (exported) - a tolerant HTML reader.
#
# `parse` produces the SAME `Node` tree the builders above create, so a parsed
# document can be walked (read `.tag` / `.attrs` / `.children` / `.text` directly,
# or query with `get` / `findAll`), edited, and re-rendered with `render` -
# build and parse round-trip through one model. Tolerant by design: void elements,
# self-closing tags, unquoted attributes, mismatched nesting, comments, DOCTYPE,
# and `script` / `style` raw text are handled rather than rejected. A depth and a
# node budget turn a malicious document into a catchable "html" error.
# ============================================================================

def const MAX_PARSE_DEPTH as int init 512;
def const MAX_PARSE_NODES as int init 200000;

# Frame is a partially-built element on the parse stack: the tree is built
# bottom-up, so on close a frame is folded into a Node and appended to its parent.
def struct Frame {
    tag as string,
    attrs as list of Attr,
    children as list of Node
};

# The Scan* structs carry a sub-scan's result plus the new cursor, so the
# closure-free parser can thread the scan position through return values.
def struct ScanTag { tag as string, attrs as list of Attr, selfClose as bool, i as int };
def struct ScanEnd { tag as string, i as int };
def struct ScanText { text as string, i as int };

# A parsed selector step: a tag name (or "*"), and a 1-based index (0 = all).
def struct Step { name as string, index as int };

func isSpace(ch as string) {
    return $ch == " " or $ch == "\t" or $ch == "\n" or $ch == "\r";
}

# matchLit reports whether the literal `lit` occurs at index i of cs.
func matchLit(cs as list of string, i as int, lit as string) {
    def ls as list of string init strings.chars($lit);
    def m as int init len($ls);
    if ($i + $m > len($cs)) {
        return false;
    }
    def k as int init 0;
    while ($k < $m) {
        if ($cs[$i + $k] != $ls[$k]) {
            return false;
        }
        $k = $k + 1;
    }
    return true;
}

# findLit returns the index of the next occurrence of `lit` at or after `from`,
# or len(cs) if there is none.
func findLit(cs as list of string, from as int, lit as string) {
    def i as int init $from;
    def n as int init len($cs);
    while ($i < $n) {
        if (matchLit($cs, $i, $lit)) {
            return $i;
        }
        $i = $i + 1;
    }
    return $n;
}

# joinRange returns cs[a..b) joined into a string.
func joinRange(cs as list of string, a as int, b as int) {
    def parts as list of string init [];
    def k as int init $a;
    while ($k < $b) {
        $parts[] = $cs[$k];
        $k = $k + 1;
    }
    return strings.join($parts, "");
}

func isVoidTag(tag as string) {
    return lists.contains(VOID, $tag);
}

/**
 * Decode the common HTML entities in a string - `&lt;` / `&gt;` / `&quot;` / `&#39;` /
 * `&apos;` / `&amp;` to their characters - the inverse of `escape`. `&amp;` is decoded
 * last, so `&amp;lt;` yields the literal `&lt;`, not `<`. A string with no `&` is
 * returned unchanged.
 *
 * Scope is deliberately the metacharacter entities only: the ones any standard
 * text-context escaper (including this module's `escape` / `escapeAttr`) emits, so a
 * round-trip is exact. It is not a general HTML5 entity decoder - the ~2000 named
 * references for authoring (`&nbsp;` / `&copy;` / `&mdash;` / ...) and numeric refs
 * (`&#8212;`) are out of scope, since decoding those means shipping the full named
 * table plus numeric parsing, not the metacharacter round-trip this serves.
 * @param s {string} the text to decode
 * @return {string} the decoded text
 */
export func unescape(s as string) {
    if (not strings.contains($s, "&")) {
        return $s;
    }
    def out as string init strings.replace($s, "&lt;", "<");
    $out = strings.replace($out, "&gt;", ">");
    $out = strings.replace($out, "&quot;", '"');
    $out = strings.replace($out, "&#39;", "'");
    $out = strings.replace($out, "&apos;", "'");
    $out = strings.replace($out, "&amp;", "&");
    return $out;
}

func failParse(msg as string) {
    throw Error{kind: "html", message: "html: " + $msg, file: "", line: 0, col: 0};
}

# readText reads a text run from i up to the next "<" (or EOF).
func readText(cs as list of string, i as int, n as int) {
    def j as int init $i;
    while ($j < $n and $cs[$j] != "<") {
        $j = $j + 1;
    }
    return ScanText{text: joinRange($cs, $i, $j), i: $j};
}

# readEndTag reads a "</tag>" from i (the char after "</"): the tag name.
func readEndTag(cs as list of string, i as int, n as int) {
    def j as int init $i;
    while ($j < $n and $cs[$j] != ">") {
        $j = $j + 1;
    }
    def tag as string init strings.lower(strings.trim(joinRange($cs, $i, $j)));
    if ($j < $n) {
        $j = $j + 1;
    }
    return ScanEnd{tag: $tag, i: $j};
}

# readStartTag reads a start tag from i (the char after "<"): the tag name, its
# attributes (quoted, unquoted, or valueless), and whether it self-closes.
func readStartTag(cs as list of string, i as int, n as int) {
    def j as int init $i;
    while ($j < $n and not (isSpace($cs[$j]) or $cs[$j] == ">" or $cs[$j] == "/")) {
        $j = $j + 1;
    }
    def tag as string init strings.lower(joinRange($cs, $i, $j));
    def attrs as list of Attr init [];
    def selfClose as bool init false;
    def done as bool init false;
    while ($j < $n and not $done) {
        while ($j < $n and isSpace($cs[$j])) {
            $j = $j + 1;
        }
        if ($j >= $n) {
            $done = true;
        } elseif ($cs[$j] == ">") {
            $j = $j + 1;
            $done = true;
        } elseif ($cs[$j] == "/") {
            $selfClose = true;
            $j = $j + 1;
        } else {
            def anStart as int init $j;
            while ($j < $n and not (isSpace($cs[$j]) or $cs[$j] == "=" or $cs[$j] == ">" or $cs[$j] == "/")) {
                $j = $j + 1;
            }
            def aname as string init strings.lower(joinRange($cs, $anStart, $j));
            while ($j < $n and isSpace($cs[$j])) {
                $j = $j + 1;
            }
            def avalue as string init "";
            def isBool as bool init true;
            if ($j < $n and $cs[$j] == "=") {
                $isBool = false;
                $j = $j + 1;
                while ($j < $n and isSpace($cs[$j])) {
                    $j = $j + 1;
                }
                if ($j < $n and ($cs[$j] == '"' or $cs[$j] == "'")) {
                    def q as string init $cs[$j];
                    $j = $j + 1;
                    def vs as int init $j;
                    while ($j < $n and $cs[$j] != $q) {
                        $j = $j + 1;
                    }
                    $avalue = unescape(joinRange($cs, $vs, $j));
                    if ($j < $n) {
                        $j = $j + 1;
                    }
                } else {
                    def vs2 as int init $j;
                    while ($j < $n and not (isSpace($cs[$j]) or $cs[$j] == ">")) {
                        $j = $j + 1;
                    }
                    $avalue = unescape(joinRange($cs, $vs2, $j));
                }
            }
            if (len($aname) > 0) {
                $attrs[] = Attr{name: $aname, value: $avalue, boolean: $isBool};
            }
        }
    }
    return ScanTag{tag: $tag, attrs: $attrs, selfClose: $selfClose, i: $j};
}

# addChild appends a node to the top frame's children (a read-modify-write, since
# a chained append `$stack[top].children[] = ...` is not supported).
func addChild(stack as list of Frame, child as Node) {
    def top as int init len($stack) - 1;
    def kids as list of Node init $stack[$top].children;
    $kids[] = $child;
    $stack[$top].children = $kids;
    return $stack;
}

# closeTag closes the nearest open frame matching `name` (an empty name closes just
# the top frame, for EOF). Mismatched nesting is tolerated: frames above the match
# are folded closed too, and an end tag with no open match is ignored.
func closeTag(stack as list of Frame, name as string) {
    def top as int init len($stack) - 1;
    if ($top < 1) {
        return $stack;
    }
    def target as int init -1;
    if ($name == "") {
        $target = $top;
    } else {
        def k as int init $top;
        while ($k >= 1) {
            if ($stack[$k].tag == $name) {
                $target = $k;
                $k = 0;
            } else {
                $k = $k - 1;
            }
        }
    }
    if ($target < 0) {
        return $stack;
    }
    def s as list of Frame init $stack;
    while (len($s) - 1 >= $target) {
        def t2 as int init len($s) - 1;
        def f as Frame init $s[$t2];
        def node as Node init Node{kind: NodeKind.Element, tag: $f.tag, attrs: $f.attrs, children: $f.children, text: ""};
        $s = lists.slice($s, 0, $t2);
        $s = addChild($s, $node);
    }
    return $s;
}

/**
 * Parse an HTML string into a `Node` tree. The result is a synthetic `#root`
 * element whose `children` are the document's top-level nodes; walk it with
 * `.children` / `get` / `findAll`, and re-serialize any node with `render`.
 * Tolerant of real-world HTML (void / self-closing tags, unquoted attributes,
 * mismatched nesting, comments, DOCTYPE, `script` / `style` raw text).
 * @param src {string} the HTML source
 * @return {Node} the `#root` element containing the parsed top-level nodes
 * @throws {Error} kind "html" if the document exceeds the depth or node budget
 */
export func parse(src as string) {
    def cs as list of string init strings.chars($src);
    def n as int init len($cs);
    def stack as list of Frame init [];
    $stack[] = Frame{tag: "#root", attrs: [], children: []};
    def budget as int init 0;
    def i as int init 0;
    while ($i < $n) {
        if ($cs[$i] == "<" and matchLit($cs, $i, "<!--")) {
            $i = findLit($cs, $i + 4, "-->");
            if ($i < $n) {
                $i = $i + 3;
            }
        } elseif ($cs[$i] == "<" and $i + 1 < $n and $cs[$i + 1] == "!") {
            $i = findLit($cs, $i + 2, ">");
            if ($i < $n) {
                $i = $i + 1;
            }
        } elseif ($cs[$i] == "<" and $i + 1 < $n and $cs[$i + 1] == "/") {
            def et as ScanEnd init readEndTag($cs, $i + 2, $n);
            $i = $et.i;
            $stack = closeTag($stack, $et.tag);
        } elseif ($cs[$i] == "<" and $i + 1 < $n and validName($cs[$i + 1])) {
            def st as ScanTag init readStartTag($cs, $i + 1, $n);
            $i = $st.i;
            $budget = $budget + 1;
            if ($budget > MAX_PARSE_NODES) {
                failParse("document exceeds the node budget");
            }
            if ($st.selfClose or isVoidTag($st.tag)) {
                $stack = addChild($stack, Node{kind: NodeKind.Element, tag: $st.tag, attrs: $st.attrs, children: [], text: ""});
            } elseif ($st.tag == "script" or $st.tag == "style") {
                def close as string init "</" + $st.tag;
                def end as int init findLit($cs, $i, $close);
                def body as list of Node init [];
                def rawText as string init joinRange($cs, $i, $end);
                if (len($rawText) > 0) {
                    $body[] = Node{kind: NodeKind.Raw, tag: "", attrs: [], children: [], text: $rawText};
                }
                $stack = addChild($stack, Node{kind: NodeKind.Element, tag: $st.tag, attrs: $st.attrs, children: $body, text: ""});
                $i = findLit($cs, $end, ">");
                if ($i < $n) {
                    $i = $i + 1;
                }
            } else {
                if (len($stack) + 1 > MAX_PARSE_DEPTH) {
                    failParse("document exceeds the nesting depth");
                }
                $stack[] = Frame{tag: $st.tag, attrs: $st.attrs, children: []};
            }
        } else {
            def tx as ScanText init readText($cs, $i, $n);
            def content as string init $tx.text;
            if ($tx.i == $i) {
                # a stray "<" that begins no tag: consume it as text
                $content = "<";
                $i = $i + 1;
            } else {
                $i = $tx.i;
            }
            if (len($content) > 0) {
                $budget = $budget + 1;
                if ($budget > MAX_PARSE_NODES) {
                    failParse("document exceeds the node budget");
                }
                $stack = addChild($stack, Node{kind: NodeKind.Text, tag: "", attrs: [], children: [], text: unescape($content)});
            }
        }
    }
    while (len($stack) > 1) {
        $stack = closeTag($stack, "");
    }
    def root as Frame init $stack[0];
    return Node{kind: NodeKind.Element, tag: "#root", attrs: [], children: $root.children, text: ""};
}

# --- node queries (exported) --------------------------------------------------

/**
 * The value of an element's attribute by name, or `""` if it has no such
 * attribute. (Read `.attrs` directly for the full list.)
 * @param node {Node} the element node
 * @param name {string} the attribute name
 * @return {string} the attribute value, or ""
 */
export func attrOf(node as Node, name as string) {
    for (def a in $node.attrs) {
        if ($a.name == $name) {
            return $a.value;
        }
    }
    return "";
}

/**
 * Whether an element has an attribute by name (including a valueless boolean one).
 * @param node {Node} the element node
 * @param name {string} the attribute name
 * @return {bool} true if present
 */
export func hasAttr(node as Node, name as string) {
    for (def a in $node.attrs) {
        if ($a.name == $name) {
            return true;
        }
    }
    return false;
}

# parseSteps splits an XPath-ish selector ("body/ul/li[2]") into steps.
func parseSteps(selector as string) {
    def steps as list of Step init [];
    for (def raw in strings.split($selector, "/")) {
        def s as string init strings.trim($raw);
        if (len($s) > 0) {
            def name as string init $s;
            def idx as int init 0;
            def lb as int init strings.indexOf($s, "[");
            if ($lb >= 0 and strings.endsWith($s, "]")) {
                $name = strings.substring($s, 0, $lb);
                def inner as string init strings.substring($s, $lb + 1, len($s) - 1);
                try {
                    $idx = convert.toInt($inner);
                } catch (e) {
                    $idx = 0;
                }
            }
            $steps[] = Step{name: strings.lower($name), index: $idx};
        }
    }
    return $steps;
}

/**
 * Every element matching an XPath-ish `selector` relative to `node`: `/`-separated
 * steps, each a tag name, `*` (any element), or `name[k]` (the k-th such child,
 * 1-based). Steps match **direct element children**; text nodes are skipped.
 * @param node {Node} the node to search under
 * @param selector {string} the selector path (e.g. "body/ul/li")
 * @return {list of Node} the matching element nodes (empty if none)
 */
export func findAll(node as Node, selector as string) {
    def frontier as list of Node init [$node];
    for (def step in parseSteps($selector)) {
        def next as list of Node init [];
        for (def parent in $frontier) {
            def matchNum as int init 0;
            for (def child in $parent.children) {
                def isEl as bool init $child.kind == NodeKind.Element;
                if ($isEl and ($step.name == "*" or $child.tag == $step.name)) {
                    $matchNum = $matchNum + 1;
                    if ($step.index == 0 or $step.index == $matchNum) {
                        $next[] = $child;
                    }
                }
            }
        }
        $frontier = $next;
    }
    return $frontier;
}

/**
 * The first element matching `selector` (see `findAll`), or an empty element node
 * (`.tag == ""`) if there is no match.
 * @param node {Node} the node to search under
 * @param selector {string} the selector path
 * @return {Node} the first match, or an empty element node
 */
export func get(node as Node, selector as string) {
    def all as list of Node init findAll($node, $selector);
    if (len($all) > 0) {
        return $all[0];
    }
    return Node{kind: NodeKind.Element, tag: "", attrs: [], children: [], text: ""};
}

/**
 * Whether any element matches `selector` (see `findAll`).
 * @param node {Node} the node to search under
 * @param selector {string} the selector path
 * @return {bool} true if at least one element matches
 */
export func has(node as Node, selector as string) {
    return len(findAll($node, $selector)) > 0;
}
