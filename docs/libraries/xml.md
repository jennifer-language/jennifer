# `xml` - XML encode / decode

Enable with `use xml;`. Parses and serializes XML over an opaque
`xml.Value`, designed like [`json`](json.md) and [`toml`](toml.md):
`decode` returns a handle you walk with accessors, and a small build
surface constructs a tree to `encode`. Hand-rolled (no reflect-bound
`encoding/xml`), so it runs on **both binaries**.

XML is a tree of **elements**, each with a tag name, ordered
**attributes**, and ordered **children** mixing elements and text.
Navigation uses a small **XPath-style path** dialect instead of JSON
Pointer.

```jennifer
use io;
use xml;

def root as xml.Value init xml.decode(
    "<library><book id=\"1\"><title>Go</title></book></library>");
io.printf("%s\n", xml.tag($root));                         # library
io.printf("%s\n", xml.attr(xml.get($root, "book"), "id")); # 1
io.printf("%s\n", xml.text(xml.get($root, "book/title"))); # Go
```

## Decode / encode

| Call                    | Returns     | Notes                                          |
| ----------------------- | ----------- | ---------------------------------------------- |
| `xml.decode(s [, ns])`  | `xml.Value` | Parse a document into the root element. Errors (with line / column) on malformed XML. `ns` (a `map of string to string`, prefix to namespace URI) puts bindings in scope at the root, for a fragment cut out of a larger document (see [Namespaces](#namespaces)). |
| `xml.encode(v)`         | `string`    | Serialize compactly.                           |
| `xml.encodePretty(v)`   | `string`    | Serialize with 2-space indentation.            |

`decode` handles elements, attributes, text, `<![CDATA[...]]>` (as text),
comments, processing instructions, an XML declaration, and a `DOCTYPE`
(the last four are skipped, not kept). The five predefined entities
(`&lt; &gt; &amp; &apos; &quot;`) and numeric character references
(`&#65;`, `&#x41;`) are decoded; an unknown entity is an error. Names keep
their prefix as written (`kml:Document`), and `decode` also resolves each one
to its namespace URI; a document that is not namespace-well-formed (an
undeclared prefix, say) is an error. See [Namespaces](#namespaces).

`encodePretty` indents element-only content, but an element that contains
any text child is emitted inline so its character data stays byte-exact.

## Reading

Every accessor takes an `xml.Value`. `xml.get` / `xml.findAll` / `xml.has`
take a path as a second argument and an optional prefix map as a third (see
below).

| Call                    | Returns             | Notes                                                    |
| ----------------------- | ------------------- | -------------------------------------------------------- |
| `xml.typeOf(node)`      | `string`            | `"element"` or `"text"`.                                 |
| `xml.tag(node)`         | `string`            | The element's tag name as written (with prefix, if any). |
| `xml.localName(node)`   | `string`            | The tag name without its prefix (`Document` for `kml:Document`). |
| `xml.namespace(node)`   | `string`            | The element's namespace URI; `""` for none.              |
| `xml.lookupPrefix(node, prefix)` | `string`   | The namespace URI bound to `prefix` at this element (`""` for the default namespace when none is declared); errors when the prefix is unbound. Works on a sub-tree too. |
| `xml.text(node)`        | `string`            | The concatenated character data of the element's direct text children (or a text node's own string). |
| `xml.attr(node, name)`  | `string`            | An attribute's value; errors if the attribute is absent. |
| `xml.hasAttr(node, name)` | `bool`            | Whether the attribute is present.                        |
| `xml.attrNS(node, uri, local)` | `string`     | The attribute named (`uri`, `local`), whatever its prefix; `uri` `""` reads an unprefixed attribute. Errors if absent. |
| `xml.hasAttrNS(node, uri, local)` | `bool`    | Whether that attribute is present.                       |
| `xml.attrs(node)`       | `list of string`    | The attribute names, in document order.                  |
| `xml.children(node)`    | `list of xml.Value` | The element children (text children are excluded).       |
| `xml.get(node, path [, ns])` | `xml.Value`    | The first element matching the path; errors if none.     |
| `xml.findAll(node, path [, ns])` | `list of xml.Value` | Every element matching the path (empty if none). |
| `xml.has(node, path [, ns])` | `bool`         | Whether the path matches at least one element.           |

## Path dialect

A path is `/`-separated steps, evaluated against the element children of
the node (a leading `/` is optional; `""` addresses the node itself):

| Step       | Matches                                                        |
| ---------- | ------------------------------------------------------------- |
| `name`     | every element child with that tag.                            |
| `name[k]`  | the **1-based** `k`-th element child with that tag.           |
| `*`        | every element child, whatever its tag.                        |
| `*:local`  | every element child with that local name, in any namespace (or none). |

Without a prefix map a `name` step matches the tag **as written**, prefix
included. With one, it matches by namespace instead - see
[Namespaces](#namespaces).

`xml.findAll($root, "book")` returns all `book` children;
`xml.get($root, "book[2]/title")` drills to the second book's title.
Attributes and text are read with `xml.attr` / `xml.text` after
navigating, not addressed inside the path. A malformed path is an error;
a well-formed path that matches nothing is an empty `findAll` (and a false
`has`, and an error from `get`).

## Building

The build surface is non-mutating: each call returns a fresh `xml.Value`
(`$v = xml.setAttr($v, ...)`), as in `json` / `toml`:

| Call                        | Returns     | Notes                                               |
| --------------------------- | ----------- | --------------------------------------------------- |
| `xml.element(name)`         | `xml.Value` | A new empty element. Namespace-unaware: its name is used as written and `encode` adds no declaration for it. |
| `xml.elementNS(uri, qname)` | `xml.Value` | A new empty element in namespace `uri` (`""` for none); `qname` may carry a prefix (`kml:Placemark`). |
| `xml.setAttr(node, name, value)` | `xml.Value` | The element with the attribute added or updated. |
| `xml.setAttrNS(node, uri, qname, value)` | `xml.Value` | The element with the attribute (`uri`, local part of `qname`) added or updated. A namespaced attribute needs a prefix. |
| `xml.setText(node, s)`      | `xml.Value` | The element with its children replaced by one text node. |
| `xml.append(parent, child)` | `xml.Value` | The parent with `child` (an `xml.Value` element) appended. |

```jennifer
use xml;

def note as xml.Value init xml.element("note");
$note = xml.setAttr($note, "type", "info");
$note = xml.setText($note, "hi & bye");
def page as xml.Value init xml.append(xml.element("page"), $note);
# <page><note type="info">hi &amp; bye</note></page>
```

## Namespaces

[Namespaces in XML](https://www.w3.org/TR/xml-names/) name an element or
attribute by a **namespace URI plus a local name**; the prefix a document uses
is only a local shorthand. `<kml xmlns="http://www.opengis.net/kml/2.2">` and
`<k:kml xmlns:k="http://www.opengis.net/kml/2.2">` hold the same elements, so
code that matches on the written tag (`xml.get($root, "Document")`) reads one
and misses the other. Match by namespace instead:

```jennifer
use xml;

def const KML as string init "http://www.opengis.net/kml/2.2";
# The same path reads both forms; "g" is your prefix, not the document's.
def docs as list of xml.Value init xml.findAll($root, "g:Document/g:Placemark", {"g": KML});
```

- **Resolution.** `decode` resolves every name against the `xmlns` /
  `xmlns:p` declarations in scope and keeps the result on the node, so a
  sub-tree from `get` / `findAll` / `children` still answers `namespace` and
  `lookupPrefix`. An unprefixed **element** takes the default namespace; an
  unprefixed **attribute** is in no namespace. `xmlns=""` undeclares the
  default. `xml:` is bound to `http://www.w3.org/XML/1998/namespace` without a
  declaration (`xml.attrNS($el, "http://www.w3.org/XML/1998/namespace", "lang")`
  reads `xml:lang`).
- **Strict.** A prefix with no declaration in scope, binding the reserved
  `xml` / `xmlns` prefixes or their namespaces otherwise, `xmlns:p=""`, a name
  with more than one `:`, and two attributes that resolve to the same
  (namespace, local name) are decode errors, with line and column.
- **Paths with a prefix map.** `get` / `findAll` / `has` take an optional
  `map of string to string` from prefix to URI, as XPath and lxml do. With it,
  `p:name` matches elements in `ns["p"]` with local name `name` (a prefix not in
  the map is an error), and an unprefixed `name` matches the namespace under the
  `""` key, or no namespace when `""` is absent. Without a map, paths match the
  written names exactly as before. `*:name` matches a local name in any
  namespace either way, a shortcut when you do not care which.
- **Fragments.** A piece cut out of a larger document (`<gpx:trkpt>...` without
  the root's `xmlns:gpx`) has an undeclared prefix. Pass the root's bindings to
  `decode` as a prefix map: `xml.decode($frag, {"gpx": GPX})`.
- **Encoding.** A decoded document encodes back byte for byte: its own `xmlns`
  attributes are kept. A sub-tree encoded on its own, and a tree built with
  `elementNS` / `setAttrNS`, gets an `xmlns` declaration exactly where an
  element needs a binding that is not in scope yet, so the output decodes to the
  same names. Elements built with `xml.element` are left as written.
- **Equality.** Two elements with the same written name in different
  namespaces are not equal.

## Notes and limits

- **`convert.typeOf`** reports `"object"`; **`convert.objectType`** reports
  `"xml.Value"`. A bare `$v` at the REPL (or `%v`) shows the compact XML.
- **Comments, PIs, the prolog, and the DOCTYPE are dropped** on decode, so
  a decode / encode round-trip is faithful for the element tree but not for
  those. The one root element is required (XML has exactly one).
- **Richer XPath** (predicates, `@attr` / `text()` path steps) is not
  supported; navigate to an element, then read attributes and text with the
  accessors.
- **Element nesting is capped** on decode, encode and `xml.append`: a deeper
  document (or an `append` that would build one) raises a catchable error instead
  of overflowing the stack. The limit is shared by every parser in the
  toolchain and is set per binary: 1000 levels on the default `jennifer` (Go's
  growable stack), 64 on `jennifer-tiny` (its fixed 4 MB stack). No entity is expanded
  beyond the five predefined names and numeric references, so there is no
  entity-expansion ("billion laughs") blow-up, and nothing is fetched
  externally; an unknown or external entity is an error.
- **Node count is capped** on decode: a document with more than about two
  million elements, attributes and text runs (a quarter million on
  `jennifer-tiny`) raises a catchable error.
- **Memory**: a decoded `xml.Value` is a compact element tree (name, ordered
  attributes, ordered children); names, and text and attribute values with no
  entity to decode, share the input string. A document takes from about its
  text size (text-heavy markup) up to roughly 15 times it (many small
  elements) in memory; each element and attribute also holds its namespace
  URI (a slice of the input as well). Copies of a handle,
  and the sub-trees `get` / `findAll` / `children` hand out, share the tree
  instead of copying it. A sub-tree handle therefore keeps the whole document's
  text in memory, while `tag` / `text` / `attr` / `attrs` return copies: to keep
  a few values of a large document, read them out instead of holding a node.

## See also

- [json.md](json.md) / [toml.md](toml.md) - the sibling opaque-handle
  formats with the same read / write shape.
- [cheatsheet.md](cheatsheet.md) - every builtin at a glance.
