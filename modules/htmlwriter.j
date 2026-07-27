# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Build an HTML element tree and render it to a correctly escaped HTML5 string.
 * Pure Jennifer over `strings` and `lists` - a writer, not a parser, so it has
 * no dependency on an XML parser: serialization is a handful of string
 * operations. The shared output layer any HTML-emitting consumer reuses (a
 * Markdown renderer, a documentation generator, a view layer). Text nodes are
 * escaped on render (`&` `<` `>`); attribute values also escape `"`; a `raw`
 * node passes through verbatim for already-trusted markup. Void elements (`br`,
 * `img`, ...) render without a closing tag and drop children.
 * @module htmlwriter
 * @example
 * import "htmlwriter.j" as html;
 * def kids as list of html.Node init [];
 * $kids[] = html.text("hi & bye");
 * def p as html.Node init html.element("p", [], $kids);
 * io.printf("%s\n", html.render($p));   # <p>hi &amp; bye</p>
 */

use strings;
use lists;
use convert;

/**
 * A node is one of three kinds, tagged by `kind`: "element" (tag + attrs +
 * children), "text" (escaped content), or "raw" (verbatim content). The
 * constructors below are the intended way to build one.
 * @field kind {string} the node kind ("element", "text", or "raw")
 * @field tag {string} the element tag name (element nodes only)
 * @field attrs {list of Attr} the element's attributes (element nodes only)
 * @field children {list of Node} the element's child nodes (element nodes only)
 * @field text {string} the content of a text or raw node
 */
export def struct Node {
    kind as string,
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
def const VOID as list of string init ["area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr"];  # lint-disable: L203

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
    return Node{kind: "element", tag: $tag, attrs: $attrs, children: $children, text: ""};
}

/**
 * Build a text node; its content is HTML-escaped on render.
 * @param s {string} the text content
 * @return {Node} the text node
 */
export func text(s as string) {
    return Node{kind: "text", tag: "", attrs: [], children: [], text: $s};
}

/**
 * Build a node whose content is emitted verbatim - for already-trusted markup
 * only, since it is not escaped.
 * @param s {string} the verbatim markup
 * @return {Node} the raw node
 */
export func raw(s as string) {
    return Node{kind: "raw", tag: "", attrs: [], children: [], text: $s};
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
        throw Error{kind: "htmlwriter", message: "htmlwriter: illegal " + $what + " name (must match [A-Za-z][A-Za-z0-9-]*): " + $name, file: "", line: 0, col: 0};
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
    if ($node.kind == "text") {
        return escape($node.text);
    }
    if ($node.kind == "raw") {
        return $node.text;
    }
    def open as string init "<" + $node.tag + renderAttrs($node.attrs);
    if (isVoid($node.tag)) {
        return $open + ">";
    }
    # Collect the pieces and join once. Growing a string with `+` per child is
    # O(output^2), so a node with many children (a paragraph full of links)
    # would otherwise be quadratic in its rendered size.
    def parts as list of string init [$open + ">"];
    for (def child in $node.children) {
        $parts[] = render($child);
    }
    $parts[] = "</" + $node.tag + ">";
    return strings.join($parts, "");
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
