# `asn1` - ASN.1 BER/DER

The `asn1` library encodes and decodes [ASN.1](https://en.wikipedia.org/wiki/ASN.1)
(Abstract Syntax Notation One) in its BER / DER transfer encodings - the
byte-level foundation for LDAP, SNMP, and PKI formats. It is designed like the
other opaque-value libraries ([`json`](json.md) / [`toml`](toml.md) /
[`xml`](xml.md) / [`yaml`](yaml.md)): `decode` yields an opaque `asn1.Value` you
walk with `(node, pointer)` accessors, and there is no coercion into a typed
struct. Values are built with typed constructors and serialised with `encode`.

It is hand-rolled (Go's `encoding/asn1` is DER-only and reflect-bound), so it is
dependency-free and both binaries build it. `decode` accepts **BER** (indefinite
lengths, the alternative encodings LDAP / SNMP permit); `encode` always emits
canonical **DER** (definite lengths).

```jennifer
use asn1;
use convert;

# Build  SEQUENCE { INTEGER 42, OCTET STRING "hi", OID 1.2.840.113549 }
def msg as asn1.Value init asn1.sequence([
    asn1.integer(42),
    asn1.octetString(convert.bytesFromString("hi", "utf-8")),
    asn1.oid("1.2.840.113549")
]);
def wire as bytes init asn1.encode($msg);          # DER bytes

def back as asn1.Value init asn1.decode($wire);
asn1.asInt($back, "/0");                            # 42
asn1.asString($back, "/1");                         # "hi"
asn1.asOid($back, "/2");                            # "1.2.840.113549"
```

## The tree and the pointer

A decoded `asn1.Value` is a tree of elements. Each element has a **tag** (a
*class* - `universal`, `application`, `context`, or `private` - and a numeric
*tagNumber*), is either **primitive** (carrying content octets) or
**constructed** (carrying ordered child elements), and a `SEQUENCE` / `SET` is
the common constructed shape.

Accessors take the node and an optional **pointer** whose tokens are **child
indices**: `""` (or omitted) is the node itself, `"/0"` its first child, `"/2/1"`
the second child of its third child. (This is the child-index analogue of the
JSON Pointer the `json` family uses over map keys.)

## Reading

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `asn1.decode(bytes)`         | `asn1.Value` | Parse one BER element; trailing bytes are an error. |
| `asn1.typeOf(v[, ptr])`      | `string`     | A friendly type: the universal type name (`integer`, `octetString`, `sequence`, ...) or the class (`context` / `application` / `private`) for a non-universal tag. |
| `asn1.tagClass(v[, ptr])`    | `string`     | `universal` / `application` / `context` / `private`. |
| `asn1.tagNumber(v[, ptr])`   | `int`        | The numeric tag. |
| `asn1.isConstructed(v[, ptr])`| `bool`      | Whether the element holds children (vs. content). |
| `asn1.length(v[, ptr])`      | `int`        | Child count of a constructed element (a primitive is an error). |
| `asn1.get(v[, ptr])`         | `asn1.Value` | The addressed sub-element, as an `asn1.Value` (the walk stays opaque). |
| `asn1.has(v, ptr)`           | `bool`       | Whether the pointer resolves. |
| `asn1.asInt(v[, ptr])`       | `int`        | An `INTEGER` / `ENUMERATED` as `int` (must fit `int64`). |
| `asn1.asBool(v[, ptr])`      | `bool`       | A `BOOLEAN`. |
| `asn1.asString(v[, ptr])`    | `string`     | A string type (`UTF8String` / `PrintableString` / `IA5String`) or an `OCTET STRING`, as UTF-8 (invalid UTF-8 is an error - use `asBytes`). |
| `asn1.asBytes(v[, ptr])`     | `bytes`      | The raw content octets of any primitive element. |
| `asn1.asOid(v[, ptr])`       | `string`     | An `OBJECT IDENTIFIER` as a dotted string. |
| `asn1.isNull(v[, ptr])`      | `bool`       | Whether the element is `NULL`. |

## Building and encoding

| Call | Builds |
| ---- | ------ |
| `asn1.integer(n)` / `asn1.enumerated(n)` | `INTEGER` / `ENUMERATED` from an `int`. |
| `asn1.boolean(b)`                        | `BOOLEAN`. |
| `asn1.null()`                            | `NULL`. |
| `asn1.octetString(bytes)`                | `OCTET STRING`. |
| `asn1.utf8String(s)` / `asn1.printableString(s)` / `asn1.ia5String(s)` | the named string type. |
| `asn1.oid(dotted)`                       | `OBJECT IDENTIFIER` from a dotted string (`"1.3.6.1"`). |
| `asn1.sequence(items)` / `asn1.set(items)` | `SEQUENCE` / `SET` from a `list of asn1.Value`. |
| `asn1.tagged(class, tagNumber, value)`   | **EXPLICIT** tag: a constructed `[class tagNumber]` wrapping `value` as its single child (its own tag intact). |
| `asn1.retag(class, tagNumber, value)`    | **IMPLICIT** tag: `value` with its outer tag replaced by `[class tagNumber]`, content and constructed-ness kept. |
| `asn1.encode(v)`                         | The DER `bytes` of an `asn1.Value`. |

`class` is `"universal"`, `"application"`, `"context"`, or `"private"`. EXPLICIT
vs. IMPLICIT tagging is the ASN.1 distinction the two builders capture: `tagged`
nests the original element inside the new tag; `retag` overwrites the tag in
place (as SNMP PDUs and many LDAP fields are defined).

## Strictness and limits

Like the rest of the standard library, malformed input is a catchable error, not
a panic or a wrong answer: a truncated element, a length that runs past the
buffer, an indefinite length on a primitive, an unterminated indefinite element,
trailing bytes after the top-level value, and a leaf extractor called on the
wrong element type all raise a positioned error. Two resource guards convert a
crafted input into a catchable error rather than a crash: nesting is bounded (a
deeply nested value cannot overflow the Go stack), and the number of elements one
`decode` will materialise is capped (a flat "decode bomb" of millions of tiny
elements is rejected).

## Scope

BER decode / DER encode over the tree above is the whole of v1 - the enabler for
the [`ldap`](../modules/) and SNMP clients. It does not interpret schema
(no ASN.1-module compiler), and `asn1.asInt` is `int64`-bounded (read a larger
`INTEGER`, such as an SNMP `Counter64` near `2^64`, with `asBytes`).
