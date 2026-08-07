# `xml` - XML encode / decode

Enable with `use xml;`. Parses and serializes XML over an opaque
`xml.Value`, designed exactly like [`json`](json.md) and
[`toml`](toml.md): `decode` returns a handle you walk with accessors, and
a small build surface constructs a tree to `encode`. Hand-rolled (no
`encoding/xml`, which is reflect-bound), so it runs on **both binaries**.

Unlike JSON's map/list/scalar tree, XML is a tree of **elements** - each
with a tag name, ordered **attributes**, and ordered **children** that mix
elements and text. The accessors reflect that shape, and navigation uses a
small **XPath-style path** dialect in place of JSON Pointer.

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
| `xml.decode(s)`         | `xml.Value` | Parse a document into the root element. Errors (with line / column) on malformed XML. |
| `xml.encode(v)`         | `string`    | Serialize compactly.                           |
| `xml.encodePretty(v)`   | `string`    | Serialize with 2-space indentation.            |

`decode` handles elements, attributes, text, `<![CDATA[...]]>` (as text),
comments, processing instructions, an XML declaration, and a `DOCTYPE`
(the last four are skipped, not kept). The five predefined entities
(`&lt; &gt; &amp; &apos; &quot;`) and numeric character references
(`&#65;`, `&#x41;`) are decoded; an unknown entity is an error. Namespace
prefixes are kept verbatim in the name (`ns:tag`), and `xmlns` declarations
are ordinary attributes.

`encodePretty` indents element-only content, but an element that contains
any text child is emitted inline so its character data stays byte-exact.

## Reading

Every accessor takes an `xml.Value`. `xml.get` / `xml.findAll` / `xml.has`
take a path as a second argument (see below).

| Call                    | Returns             | Notes                                                    |
| ----------------------- | ------------------- | -------------------------------------------------------- |
| `xml.typeOf(node)`      | `string`            | `"element"` or `"text"`.                                 |
| `xml.tag(node)`         | `string`            | The element's tag name (with prefix, if any).            |
| `xml.text(node)`        | `string`            | The concatenated character data of the element's direct text children (or a text node's own string). |
| `xml.attr(node, name)`  | `string`            | An attribute's value; errors if the attribute is absent. |
| `xml.hasAttr(node, name)` | `bool`            | Whether the attribute is present.                        |
| `xml.attrs(node)`       | `list of string`    | The attribute names, in document order.                  |
| `xml.children(node)`    | `list of xml.Value` | The element children (text children are excluded).       |
| `xml.get(node, path)`   | `xml.Value`         | The first element matching the path; errors if none.     |
| `xml.findAll(node, path)` | `list of xml.Value` | Every element matching the path (empty if none).       |
| `xml.has(node, path)`   | `bool`              | Whether the path matches at least one element.           |

## Path dialect

A path is `/`-separated steps, evaluated against the element children of
the node (a leading `/` is optional; `""` addresses the node itself):

| Step       | Matches                                                        |
| ---------- | ------------------------------------------------------------- |
| `name`     | every element child with that tag.                            |
| `name[k]`  | the **1-based** `k`-th element child with that tag.           |
| `*`        | every element child, whatever its tag.                        |

`xml.findAll($root, "book")` returns all `book` children;
`xml.get($root, "book[2]/title")` drills to the second book's title.
Attributes and text are read with `xml.attr` / `xml.text` after
navigating, not addressed inside the path. A malformed path is an error;
a well-formed path that matches nothing is an empty `findAll` (and a false
`has`, and an error from `get`).

## Building

The build surface is non-mutating - each call returns a fresh `xml.Value`,
the same `$v = xml.set(...)` idiom as `json` / `toml`:

| Call                        | Returns     | Notes                                               |
| --------------------------- | ----------- | --------------------------------------------------- |
| `xml.element(name)`         | `xml.Value` | A new empty element.                                |
| `xml.setAttr(node, name, value)` | `xml.Value` | The element with the attribute added or updated. |
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

## Notes and limits

- **`convert.typeOf`** reports `"object"`; **`convert.objectType`** reports
  `"xml.Value"`. A bare `$v` at the REPL (or `%v`) shows the compact XML.
- **Comments, PIs, the prolog, and the DOCTYPE are dropped** on decode, so
  a decode / encode round-trip is faithful for the element tree but not for
  those. The one root element is required (XML has exactly one).
- **Namespaces are lexical**: prefixes are kept as written, not resolved to
  their `xmlns` URIs. Full namespace resolution and richer XPath (predicates,
  `@attr` / `text()` path steps) are future extensions.
- **Element nesting is capped** on both decode and encode: a deeper document
  (or a tree built that deep with `xml.append`) raises a catchable error rather
  than overflowing the stack. The limit is shared by every parser in the
  toolchain and is set per binary: 1000 levels on the default `jennifer` (Go's
  growable stack), 64 on `jennifer-tiny` (its fixed 4 MB stack). No entity is expanded
  beyond the five predefined names and numeric references, so there is no
  entity-expansion ("billion laughs") blow-up, and nothing is fetched
  externally - an unknown or external entity is simply an error.

## See also

- [json.md](json.md) / [toml.md](toml.md) - the sibling opaque-handle
  formats with the same read / write shape.
- [cheatsheet.md](cheatsheet.md) - every builtin at a glance.
