# `htmlwriter` - build and render an HTML tree

Import with `import "htmlwriter.j" as html;`. Assembles an HTML element tree
and renders it to a correctly-escaped HTML5 string. Pure Jennifer over
`strings` and `lists`, so it runs on either binary. It is a **writer, not a
parser** - serialization is a handful of string operations, so it has no
dependency on an XML parser. It is the shared output layer an HTML-emitting
consumer reuses (a Markdown renderer, a documentation generator, a view
layer).

```jennifer
use io;
import "htmlwriter.j" as html;

def kids as list of html.Node init [];
$kids[] = html.text("hi & bye");
def p as html.Node init html.element("p", [], $kids);
io.printf("%s\n", html.render($p));          # <p>hi &amp; bye</p>
```

Runnable: [`examples/modules/htmlwriter_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/htmlwriter_demo.j).

## The node model

An HTML tree is `Node` values. A node is one of three kinds, tagged by its
`kind` field, and is built with a constructor rather than a literal:

| Kind        | Built with                       | Renders as                                   |
| ----------- | -------------------------------- | -------------------------------------------- |
| `"element"` | `element(tag, attrs, children)`  | `<tag ...>children</tag>` (or a void tag)    |
| `"text"`    | `text(s)`                        | `s`, HTML-escaped                            |
| `"raw"`     | `raw(s)`                         | `s`, verbatim (already-trusted markup only)  |

```jennifer
export def struct Node {
    kind as string, tag as string,
    attrs as list of Attr, children as list of Node, text as string
};
export def struct Attr { name as string, value as string, boolean as bool };
```

Children are supplied to `element` as a `list of Node` you build first (the
append sugar does not chain into a struct field, so build the list in a
variable, then pass it). Attributes are a `list of Attr` built with `attr`
(a normal `name="value"` attribute) or `boolAttr` (a valueless boolean
attribute).

## Surface

| Call                                | Returns  | Notes                                                              |
| ----------------------------------- | -------- | ----------------------------------------------------------------- |
| `html.element(tag, attrs, children)`| `Node`   | An element node. Pass `[]` for no attributes or no children.      |
| `html.text(s)`                      | `Node`   | A text node; `s` is HTML-escaped on render.                       |
| `html.raw(s)`                       | `Node`   | A verbatim node; `s` is **not** escaped. Trusted markup only.     |
| `html.attr(name, value)`            | `Attr`   | One attribute; `value` is escaped in attribute context on render. |
| `html.boolAttr(name)`               | `Attr`   | A boolean (valueless) attribute; renders as the bare name (`disabled`). |
| `html.render(node)`                 | `string` | Serialize a node and its subtree to HTML5.                        |
| `html.renderAll(nodes)`             | `string` | Serialize a `list of Node` fragment in order.                     |
| `html.escape(s)`                    | `string` | HTML-escape a bare string for text context (the helper `render` uses). |
| `html.safeUrl(url)`                 | `string` | The URL if its scheme is `http` / `https` / `mailto`, else `"#"` (anti-XSS `href`/`src` gate). |

## Escaping

Escaping is automatic and context-aware, so a value is escaped exactly
once:

- **Text nodes** escape `&`, `<`, `>` (with `&` first, so an existing entity
  is not double-escaped).
- **Attribute values** additionally escape `"`, since they render inside
  double quotes.
- **`raw` nodes** are emitted verbatim - the escape hatch for markup you
  have already produced (an SVG blob, a rendered sub-tree). Only pass
  trusted content.

```jennifer
def a as list of html.Attr init [];
$a[] = html.attr("title", "a \"b\" <c>");
io.printf("%s\n", html.render(html.element("span", $a, [])));
# <span title="a &quot;b&quot; &lt;c&gt;"></span>
```

`html.escape(s)` exposes the text-context escaper on its own, for when you
need an escaped string without building a node.

## Boolean attributes

HTML boolean attributes - `disabled`, `checked`, `selected`, `required`,
`readonly`, `multiple`, `autofocus`, and the like - carry no value: their
presence alone is the truth. Build one with `html.boolAttr(name)` and it
renders as the bare name, not `name=""`. Normal and boolean attributes mix
freely on one element, in the order you append them; a boolean-attribute
name is validated exactly like an `attr` name.

```jennifer
def a as list of html.Attr init [];
$a[] = html.attr("type", "checkbox");
$a[] = html.boolAttr("checked");
$a[] = html.boolAttr("required");
io.printf("%s\n", html.render(html.element("input", $a, [])));
# <input type="checkbox" checked required>
```

## Safe tag / attribute names

`html.element` and `html.attr` validate the tag / attribute name against
`[A-Za-z][A-Za-z0-9-]*` and throw an `Error` of kind `"htmlwriter"` on anything
else. A name is markup structure, not data, so unlike a value it cannot be
escaped: `html.attr("x onclick=alert(1)", "y")` or a tag built from an untrusted
string would otherwise inject a live attribute or element. Legal names
(`"data-id"`, `"h1"`, `"aria-label"`) build as before; a name carrying a space,
`>`, `=`, `/`, or a quote is rejected at construction.

## Safe URLs in links

`html.safeUrl(url)` returns the URL when its scheme is `http`, `https`, or
`mailto`, and `"#"` otherwise, so a `javascript:` (or `data:`) URL built from
untrusted input cannot become a live `href` / `src`. Whitespace and control
characters are ignored while reading the scheme, so `"java\tscript:..."` is
still caught; a relative reference (no scheme) is returned unchanged.

```jennifer
$a[] = html.attr("href", html.safeUrl($userUrl));   # "#" if $userUrl is javascript:...
```

## Void elements

The HTML5 void elements - `area base br col embed hr img input link meta
param source track wbr` - render with no closing tag, and any children
passed to them are dropped (they cannot have content). The tag is matched
case-insensitively.

```jennifer
def a as list of html.Attr init [];
$a[] = html.attr("src", "logo.png");
io.printf("%s\n", html.render(html.element("img", $a, [])));   # <img src="logo.png">
```

## Fragments

`render` serializes a single node; `renderAll` serializes a list of sibling
nodes with no wrapping element - a document fragment:

```jennifer
def parts as list of html.Node init [];
$parts[] = html.element("h1", [], heading);
$parts[] = html.element("hr", [], []);
io.printf("%s\n", html.renderAll($parts));
```

## Out of scope

This module **writes** HTML; it does not parse it (parsing arbitrary HTML
is a separate, much larger job and would want the `xml` system library).
There is no pretty-printing / indentation pass - output is compact, which
round-trips and diffs predictably; wrap it in your own formatter if you
need indented source. A `<!DOCTYPE html>` prologue is not emitted; prepend
`html.raw("<!DOCTYPE html>")` (or a literal string) when you need a full
document.

## See also

- [strings.md](../libraries/strings.md) - `replace` / `lower`, which the
  escaping and void-element check build on.
- [lists.md](../libraries/lists.md) - `contains`, used for the void-element
  lookup.
- [modules/index.md](index.md) - the module catalog and import rules.
