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
 * One name/value HTML attribute.
 * @field name {string} the attribute name
 * @field value {string} the attribute value (escaped on render)
 */
export def struct Attr {
    name as string,
    value as string
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
    return Attr{name: $name, value: $value};
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

# --- rendering (exported) ------------------------------------------

# isVoid reports whether a tag is an HTML5 void element (case-insensitive).
func isVoid(tag as string) {
    return lists.contains(VOID, strings.lower($tag));
}

# renderAttrs renders a leading-space-separated attribute list.
func renderAttrs(attrs as list of Attr) {
    def out as string init "";
    for (def a in $attrs) {
        $out = $out + " " + $a.name + "=\"" + escapeAttr($a.value) + "\"";
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
