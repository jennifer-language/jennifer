// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

// The BER decoder / DER encoder that backs the `asn1` library. It is
// hand-rolled (Go's stdlib `encoding/asn1` is DER-only and reflect-bound, so it
// covers neither the BER indefinite lengths LDAP / SNMP permit nor the opaque
// value model), which keeps the library dependency-free and TinyGo-clean.
//
// A decoded element is represented as an internal `KindStruct` value with a
// fixed schema - class, tag, constructed, content (bytes, primitive only),
// children (list, constructed only) - wrapped in an opaque `asn1.Value`
// (KindObject) so no Jennifer operator or field access can reach it. The codec
// here converts between those element trees and the wire bytes; the accessors
// and constructors in asn1lib.go read and build them.
package asn1lib

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// Tag classes (the top two bits of the identifier octet).
const (
	classUniversal   = 0
	classApplication = 1
	classContext     = 2
	classPrivate     = 3
)

// Universal tag numbers this library names and builds.
const (
	tagBoolean         = 1
	tagInteger         = 2
	tagBitString       = 3
	tagOctetString     = 4
	tagNull            = 5
	tagOID             = 6
	tagEnumerated      = 10
	tagUTF8String      = 12
	tagSequence        = 16
	tagSet             = 17
	tagPrintableString = 19
	tagIA5String       = 22
	tagUTCTime         = 23
	tagGeneralizedTime = 24
)

// elemType is the internal KindStruct name every element carries. It is never
// registered as a Jennifer type - the element only ever travels wrapped in the
// opaque `asn1.Value` - so it cannot collide with a user struct.
const elemType = "asn1.element"

// maxTagNumber caps a tag number both where decode reads a high-tag-number
// identifier (bounding the continuation-byte loop) and where a builder accepts
// one, so a value a program can build is a value decode can read back. It is far
// beyond any real tag (universal tags are < 40; application / context tags a
// handful) yet well short of the point the base-128 encoding grows unwieldy.
const maxTagNumber = 1 << 24

// maxOIDArc caps a single OID sub-identifier, enforced by both encodeOID and
// decodeOID so a value the library builds is always one it can read back. It is
// far past any real arc (the largest registered arcs are well under 2^32) yet
// leaves the base-128 accumulation comfortably inside uint64.
const maxOIDArc = 1<<57 - 1

// maxNodes bounds how many elements one decode will materialise. Each element is
// a five-field struct value (~1.5 KB once the Values are counted, not the 8 bytes
// its wire form suggests), and a crafted input of many tiny (2-byte) elements
// would otherwise turn a modest byte slice into gigabytes of tree - a decode-bomb
// amplification. At 200k the materialised tree stays near ~300 MB, the same
// single-operation memory ceiling MaxChannelCapacity / MaxMatrixElements accept,
// while leaving generous headroom over any legitimate ASN.1 message (an SNMP PDU
// is dozens of elements, an LDAP PDU or an X.509 certificate at most a few
// thousand). Beyond it, decode is a catchable error, not an OOM. The nesting
// depth is bounded separately by limits.MaxNestingDepth. It is a var (not a
// const) only so a test can lower it without materialising a real bomb.
var maxNodes = 200_000

// childListType is the placeholder element type stamped on an element's children
// list. The list only ever holds element structs and is never type-checked
// against it (it lives inside the opaque object), so the tag is immaterial.
var childListType = parser.PrimitiveType(parser.TypeNull)

// makeElem builds the internal struct value for one element.
func makeElem(class, tag int, constructed bool, content []byte, children []interpreter.Value) interpreter.Value {
	return interpreter.StructVal(elemType, []interpreter.StructField{
		{Name: "class", Value: interpreter.IntVal(int64(class))},
		{Name: "tag", Value: interpreter.IntVal(int64(tag))},
		{Name: "constructed", Value: interpreter.BoolVal(constructed)},
		{Name: "content", Value: interpreter.BytesVal(content)},
		{Name: "children", Value: interpreter.ListVal(childListType, children)},
	})
}

func elemField(v interpreter.Value, name string) interpreter.Value {
	for _, f := range v.Fields {
		if f.Name == name {
			return f.Value
		}
	}
	return interpreter.Null()
}

func elemClass(v interpreter.Value) int        { return int(elemField(v, "class").Int) }
func elemTag(v interpreter.Value) int          { return int(elemField(v, "tag").Int) }
func elemConstructed(v interpreter.Value) bool { return elemField(v, "constructed").Bool }
func elemContent(v interpreter.Value) []byte   { return elemField(v, "content").Bytes }
func elemChildren(v interpreter.Value) []interpreter.Value {
	return elemField(v, "children").List
}

// isElem reports whether v is one of our internal element structs.
func isElem(v interpreter.Value) bool {
	return v.Kind == interpreter.KindStruct && v.StructName == elemType
}

// ---- decode (BER) ----

// decodeTree parses exactly one top-level element from data. Trailing bytes are
// rejected (an ASN.1 value is a single element).
func decodeTree(data []byte) (interpreter.Value, error) {
	nodes := 0
	elem, rest, err := decodeOne(data, 0, &nodes)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(rest) != 0 {
		return interpreter.Null(), fmt.Errorf("asn1.decode: %d trailing byte(s) after the top-level element", len(rest))
	}
	return elem, nil
}

// decodeOne parses one BER element from data, returning it and the unconsumed
// remainder. depth guards recursion; nodes counts materialised elements.
func decodeOne(data []byte, depth int, nodes *int) (interpreter.Value, []byte, error) {
	if depth > limits.MaxNestingDepth {
		return interpreter.Null(), nil, fmt.Errorf("asn1.decode: nesting deeper than the %d-level limit", limits.MaxNestingDepth)
	}
	*nodes++
	if *nodes > maxNodes {
		return interpreter.Null(), nil, fmt.Errorf("asn1.decode: more than %d elements (input rejected as a decode bomb)", maxNodes)
	}
	if len(data) < 2 {
		return interpreter.Null(), nil, fmt.Errorf("asn1.decode: truncated element (need at least an identifier and a length octet)")
	}

	// Identifier octet(s).
	b0 := data[0]
	class := int(b0 >> 6)
	constructed := b0&0x20 != 0
	tag := int(b0 & 0x1f)
	i := 1
	if tag == 0x1f {
		// High-tag-number form: base-128 continuation octets. Accumulate in
		// uint64 (width-independent, so a 32-bit int cannot wrap) and cap *after*
		// each shift, so the accepted tag never exceeds maxTagNumber - keeping
		// decode symmetric with the builders, which reject the same bound.
		var t uint64
		for {
			if i >= len(data) {
				return interpreter.Null(), nil, fmt.Errorf("asn1.decode: truncated high-tag-number identifier")
			}
			c := data[i]
			i++
			t = t<<7 | uint64(c&0x7f)
			if t > maxTagNumber {
				return interpreter.Null(), nil, fmt.Errorf("asn1.decode: tag number too large")
			}
			if c&0x80 == 0 {
				break
			}
		}
		tag = int(t)
	}

	// Length octet(s).
	if i >= len(data) {
		return interpreter.Null(), nil, fmt.Errorf("asn1.decode: missing length octet")
	}
	l0 := data[i]
	i++
	indefinite := false
	contentLen := 0
	if l0&0x80 == 0 {
		contentLen = int(l0)
	} else {
		n := int(l0 & 0x7f)
		if n == 0 {
			indefinite = true
		} else {
			if n > 8 {
				return interpreter.Null(), nil, fmt.Errorf("asn1.decode: length field of %d octets is unsupported", n)
			}
			// Accumulate in uint64 (width-independent, so a 32-bit int cannot
			// overflow), and bail the moment the length cannot fit the buffer -
			// which also keeps the int() conversion below safe on every platform.
			var length uint64
			for k := 0; k < n; k++ {
				if i >= len(data) {
					return interpreter.Null(), nil, fmt.Errorf("asn1.decode: truncated long-form length")
				}
				length = length<<8 | uint64(data[i])
				i++
				if length > uint64(len(data)) {
					return interpreter.Null(), nil, fmt.Errorf("asn1.decode: length exceeds the available data")
				}
			}
			contentLen = int(length)
		}
	}

	if indefinite {
		if !constructed {
			return interpreter.Null(), nil, fmt.Errorf("asn1.decode: indefinite length on a primitive element")
		}
		rest := data[i:]
		var children []interpreter.Value
		for {
			if len(rest) >= 2 && rest[0] == 0 && rest[1] == 0 {
				rest = rest[2:]
				break
			}
			if len(rest) == 0 {
				return interpreter.Null(), nil, fmt.Errorf("asn1.decode: unterminated indefinite-length element")
			}
			child, r, err := decodeOne(rest, depth+1, nodes)
			if err != nil {
				return interpreter.Null(), nil, err
			}
			children = append(children, child)
			rest = r
		}
		return makeElem(class, tag, true, nil, children), rest, nil
	}

	// Definite length: the content must fit in what remains.
	if contentLen > len(data)-i {
		return interpreter.Null(), nil, fmt.Errorf("asn1.decode: element length %d exceeds the %d available bytes", contentLen, len(data)-i)
	}
	content := data[i : i+contentLen]
	rest := data[i+contentLen:]

	if constructed {
		var children []interpreter.Value
		cdata := content
		for len(cdata) > 0 {
			child, r, err := decodeOne(cdata, depth+1, nodes)
			if err != nil {
				return interpreter.Null(), nil, err
			}
			children = append(children, child)
			cdata = r
		}
		return makeElem(class, tag, true, nil, children), rest, nil
	}
	// Primitive: copy the content out of the input slice.
	return makeElem(class, tag, false, append([]byte(nil), content...), nil), rest, nil
}

// ---- encode (DER) ----

// encodeTree serialises an element tree to DER (definite lengths, canonical).
func encodeTree(v interpreter.Value) ([]byte, error) {
	return encodeElem(v, 0)
}

func encodeElem(v interpreter.Value, depth int) ([]byte, error) {
	if depth > limits.MaxNestingDepth {
		return nil, fmt.Errorf("asn1.encode: nesting deeper than the %d-level limit", limits.MaxNestingDepth)
	}
	class := elemClass(v)
	tag := elemTag(v)
	constructed := elemConstructed(v)

	var content []byte
	if constructed {
		for _, ch := range elemChildren(v) {
			cb, err := encodeElem(ch, depth+1)
			if err != nil {
				return nil, err
			}
			content = append(content, cb...)
		}
	} else {
		content = elemContent(v)
	}

	var out []byte
	id := byte(class << 6)
	if constructed {
		id |= 0x20
	}
	if tag < 0x1f {
		out = append(out, id|byte(tag))
	} else {
		out = append(out, id|0x1f)
		out = append(out, base128(uint64(tag))...)
	}
	out = append(out, encodeLength(len(content))...)
	out = append(out, content...)
	return out, nil
}

// encodeLength renders a definite length in DER short or long form.
func encodeLength(n int) []byte {
	if n < 0x80 {
		return []byte{byte(n)}
	}
	var tmp []byte
	for n > 0 {
		tmp = append([]byte{byte(n & 0xff)}, tmp...)
		n >>= 8
	}
	return append([]byte{0x80 | byte(len(tmp))}, tmp...)
}

// ---- primitive content codecs ----

// encodeIntContent renders n as a minimal two's-complement big-endian integer,
// the DER INTEGER / ENUMERATED content.
func encodeIntContent(n int64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(n))
	i := 0
	for i < 7 {
		// A leading 0x00 is redundant while the next byte stays positive; a
		// leading 0xff is redundant while the next byte stays negative.
		if b[i] == 0x00 && b[i+1]&0x80 == 0 {
			i++
		} else if b[i] == 0xff && b[i+1]&0x80 != 0 {
			i++
		} else {
			break
		}
	}
	return append([]byte(nil), b[i:]...)
}

// decodeIntContent reads a two's-complement big-endian INTEGER / ENUMERATED that
// fits in int64.
func decodeIntContent(b []byte) (int64, error) {
	if len(b) == 0 {
		return 0, fmt.Errorf("empty integer content")
	}
	if len(b) > 8 {
		return 0, fmt.Errorf("integer does not fit in a 64-bit int")
	}
	var n int64
	if b[0]&0x80 != 0 {
		n = -1 // sign-extend a negative value
	}
	for _, c := range b {
		n = n<<8 | int64(c)
	}
	return n, nil
}

// base128 renders v as base-128 with continuation bits (all but the last octet
// have the high bit set) - the encoding of an OID sub-identifier and of a
// high-form tag number.
func base128(v uint64) []byte {
	if v == 0 {
		return []byte{0}
	}
	var tmp []byte
	for v > 0 {
		tmp = append(tmp, byte(v&0x7f))
		v >>= 7
	}
	out := make([]byte, len(tmp))
	for i := range tmp {
		out[i] = tmp[len(tmp)-1-i]
		if i < len(tmp)-1 {
			out[i] |= 0x80
		}
	}
	return out
}

// encodeOID renders a dotted OID string ("1.3.6.1.2.1") as its content octets.
func encodeOID(dotted string) ([]byte, error) {
	parts := strings.Split(dotted, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("OID %q needs at least two arcs", dotted)
	}
	arcs := make([]uint64, len(parts))
	for i, p := range parts {
		a, err := strconv.ParseUint(p, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("OID arc %q is not a non-negative integer", p)
		}
		if a > maxOIDArc {
			return nil, fmt.Errorf("OID arc %d exceeds the maximum sub-identifier", a)
		}
		arcs[i] = a
	}
	if arcs[0] > 2 {
		return nil, fmt.Errorf("OID first arc must be 0, 1, or 2 (got %d)", arcs[0])
	}
	if arcs[0] < 2 && arcs[1] >= 40 {
		return nil, fmt.Errorf("OID second arc must be < 40 when the first is 0 or 1 (got %d)", arcs[1])
	}
	out := base128(arcs[0]*40 + arcs[1])
	for _, a := range arcs[2:] {
		out = append(out, base128(a)...)
	}
	return out, nil
}

// decodeOID renders OID content octets back to a dotted string.
func decodeOID(b []byte) (string, error) {
	if len(b) == 0 {
		return "", fmt.Errorf("empty OID content")
	}
	var arcs []uint64
	i := 0
	for i < len(b) {
		var v uint64
		for {
			if i >= len(b) {
				return "", fmt.Errorf("truncated OID sub-identifier")
			}
			c := b[i]
			i++
			if v > maxOIDArc {
				return "", fmt.Errorf("OID sub-identifier too large")
			}
			v = v<<7 | uint64(c&0x7f)
			if c&0x80 == 0 {
				break
			}
		}
		arcs = append(arcs, v)
	}
	first := arcs[0]
	var a0, a1 uint64
	if first < 80 {
		a0, a1 = first/40, first%40
	} else {
		a0, a1 = 2, first-80
	}
	var sb strings.Builder
	sb.WriteString(strconv.FormatUint(a0, 10))
	sb.WriteByte('.')
	sb.WriteString(strconv.FormatUint(a1, 10))
	for _, a := range arcs[1:] {
		sb.WriteByte('.')
		sb.WriteString(strconv.FormatUint(a, 10))
	}
	return sb.String(), nil
}

// typeName gives an element a friendly type string: the universal type name, or
// the class ("application" / "context" / "private") for a non-universal tag.
func typeName(class, tag int) string {
	if class != classUniversal {
		switch class {
		case classApplication:
			return "application"
		case classContext:
			return "context"
		case classPrivate:
			return "private"
		}
	}
	switch tag {
	case tagBoolean:
		return "boolean"
	case tagInteger:
		return "integer"
	case tagBitString:
		return "bitString"
	case tagOctetString:
		return "octetString"
	case tagNull:
		return "null"
	case tagOID:
		return "oid"
	case tagEnumerated:
		return "enumerated"
	case tagUTF8String:
		return "utf8String"
	case tagSequence:
		return "sequence"
	case tagSet:
		return "set"
	case tagPrintableString:
		return "printableString"
	case tagIA5String:
		return "ia5String"
	case tagUTCTime:
		return "utcTime"
	case tagGeneralizedTime:
		return "generalizedTime"
	}
	return "universal"
}

// className maps a class string ("context" / "application" / "private" /
// "universal") to its numeric class, for the tagged / retag constructors.
func className(s string) (int, error) {
	switch s {
	case "universal":
		return classUniversal, nil
	case "application":
		return classApplication, nil
	case "context":
		return classContext, nil
	case "private":
		return classPrivate, nil
	}
	return 0, fmt.Errorf("unknown tag class %q (want \"universal\", \"application\", \"context\", or \"private\")", s)
}

// classString is the inverse of className.
func classString(class int) string {
	switch class {
	case classUniversal:
		return "universal"
	case classApplication:
		return "application"
	case classContext:
		return "context"
	case classPrivate:
		return "private"
	}
	return "universal"
}
