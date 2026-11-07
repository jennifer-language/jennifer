// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// The BER decoder / DER encoder that backs the `asn1` library. It is
// hand-rolled (Go's stdlib `encoding/asn1` is DER-only and reflect-bound, so it
// covers neither the BER indefinite lengths LDAP / SNMP permit nor the opaque
// value model), which keeps the library dependency-free and TinyGo-clean.
//
// A decoded element is a native Go `elem` (class, tag, constructed, content for a
// primitive, children for a constructed element), the payload of an opaque
// `asn1.Value` (KindObject) so no Jennifer operator or field access can reach
// it. Elements are immutable once built and may be shared by several trees. The
// codec here converts between those element trees and the wire bytes; the
// accessors and constructors in asn1lib.go read and build them.
package asn1lib

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
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

// maxNodes bounds how many elements one decode will materialise, so a crafted
// input of many tiny (2-byte) elements is a catchable error rather than an
// amplified tree. An element costs about 100 bytes at the decode peak (the
// 56-byte elem, its share of the parent's children slice, primitive content),
// so the shared limits.MaxDecodedNodes bound keeps the tree to a few hundred MB
// on the default binary and a few tens of MB on jennifer-tiny - far past any
// real message (an SNMP PDU is dozens of elements, an LDAP PDU or X.509
// certificate a few thousand). The nesting depth is bounded separately by
// limits.MaxNestingDepth. A var only so a test can lower it.
var maxNodes = limits.MaxDecodedNodes

// elem is one ASN.1 element. content is set for a primitive element, children
// for a constructed one. An elem is never mutated after construction.
type elem struct {
	class       uint8
	constructed bool
	tag         int32
	content     []byte
	children    []*elem
}

// makeElem builds one element.
func makeElem(class, tag int, constructed bool, content []byte, children []*elem) *elem {
	return &elem{class: uint8(class), constructed: constructed, tag: int32(tag), content: content, children: children}
}

func elemClass(e *elem) int        { return int(e.class) }
func elemTag(e *elem) int          { return int(e.tag) }
func elemConstructed(e *elem) bool { return e.constructed }
func elemContent(e *elem) []byte   { return e.content }
func elemChildren(e *elem) []*elem { return e.children }

// EqualNative reports whether other holds a structurally equal tree (the
// language's `==` on two asn1.Value handles).
func (e *elem) EqualNative(other interpreter.NativeObject) bool {
	o, ok := other.(*elem)
	return ok && elemEqual(e, o, 0)
}

// elemEqual compares two trees; depth is bounded by the decode / build nesting
// limit, so the recursion is too.
func elemEqual(a, b *elem, depth int) bool {
	if a == b {
		return true
	}
	if a.class != b.class || a.tag != b.tag || a.constructed != b.constructed ||
		!bytes.Equal(a.content, b.content) || len(a.children) != len(b.children) {
		return false
	}
	if depth > limits.MaxNestingDepth {
		return false
	}
	for i := range a.children {
		if !elemEqual(a.children[i], b.children[i], depth+1) {
			return false
		}
	}
	return true
}

// DisplayNative renders a compact structural summary (see describe).
func (e *elem) DisplayNative() string {
	var sb strings.Builder
	describe(&sb, e, 0)
	return sb.String()
}

// ---- decode (BER) ----

// decodeTree parses exactly one top-level element from data. Trailing bytes are
// rejected (an ASN.1 value is a single element).
func decodeTree(data []byte) (*elem, error) {
	nodes := 0
	elem, rest, err := decodeOne(data, 0, &nodes)
	if err != nil {
		return nil, err
	}
	if len(rest) != 0 {
		return nil, fmt.Errorf("asn1.decode: %d trailing byte(s) after the top-level element", len(rest))
	}
	return elem, nil
}

// decodeOne parses one BER element from data, returning it and the unconsumed
// remainder. depth guards recursion; nodes counts materialised elements.
func decodeOne(data []byte, depth int, nodes *int) (*elem, []byte, error) {
	if depth > limits.MaxNestingDepth {
		return nil, nil, fmt.Errorf("asn1.decode: nesting deeper than the %d-level limit", limits.MaxNestingDepth)
	}
	*nodes++
	if *nodes > maxNodes {
		return nil, nil, fmt.Errorf("asn1.decode: more than %d elements (input rejected as a decode bomb)", maxNodes)
	}
	if len(data) < 2 {
		return nil, nil, fmt.Errorf("asn1.decode: truncated element (need at least an identifier and a length octet)")
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
				return nil, nil, fmt.Errorf("asn1.decode: truncated high-tag-number identifier")
			}
			c := data[i]
			i++
			t = t<<7 | uint64(c&0x7f)
			if t > maxTagNumber {
				return nil, nil, fmt.Errorf("asn1.decode: tag number too large")
			}
			if c&0x80 == 0 {
				break
			}
		}
		tag = int(t)
	}

	// Length octet(s).
	if i >= len(data) {
		return nil, nil, fmt.Errorf("asn1.decode: missing length octet")
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
				return nil, nil, fmt.Errorf("asn1.decode: length field of %d octets is unsupported", n)
			}
			// Accumulate in uint64 (width-independent, so a 32-bit int cannot
			// overflow), and bail the moment the length cannot fit the buffer -
			// which also keeps the int() conversion below safe on every platform.
			var length uint64
			for k := 0; k < n; k++ {
				if i >= len(data) {
					return nil, nil, fmt.Errorf("asn1.decode: truncated long-form length")
				}
				length = length<<8 | uint64(data[i])
				i++
				if length > uint64(len(data)) {
					return nil, nil, fmt.Errorf("asn1.decode: length exceeds the available data")
				}
			}
			contentLen = int(length)
		}
	}

	if indefinite {
		if !constructed {
			return nil, nil, fmt.Errorf("asn1.decode: indefinite length on a primitive element")
		}
		rest := data[i:]
		var children []*elem
		for {
			if len(rest) >= 2 && rest[0] == 0 && rest[1] == 0 {
				rest = rest[2:]
				break
			}
			if len(rest) == 0 {
				return nil, nil, fmt.Errorf("asn1.decode: unterminated indefinite-length element")
			}
			child, r, err := decodeOne(rest, depth+1, nodes)
			if err != nil {
				return nil, nil, err
			}
			children = append(children, child)
			rest = r
		}
		return makeElem(class, tag, true, nil, children), rest, nil
	}

	// Definite length: the content must fit in what remains.
	if contentLen > len(data)-i {
		return nil, nil, fmt.Errorf("asn1.decode: element length %d exceeds the %d available bytes", contentLen, len(data)-i)
	}
	content := data[i : i+contentLen]
	rest := data[i+contentLen:]

	if constructed {
		var children []*elem
		cdata := content
		for len(cdata) > 0 {
			child, r, err := decodeOne(cdata, depth+1, nodes)
			if err != nil {
				return nil, nil, err
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

// encodeTree serialises an element tree to DER definite lengths in two passes:
// measure computes every element's content length, then write emits into one
// buffer of the exact size, so each byte is copied once whatever the depth.
// SET children are emitted in ascending order of their encodings (X.690 11.6).
func encodeTree(e *elem) ([]byte, error) {
	enc := encoder{seen: map[*elem]measured{}}
	m, err := enc.measure(e, 0)
	if err != nil {
		return nil, err
	}
	if m.total > limits.MaxAllocBytes {
		return nil, fmt.Errorf("asn1.encode: encoding exceeds the %d-byte limit", limits.MaxAllocBytes)
	}
	return enc.write(make([]byte, 0, m.total), e), nil
}

// measured is one element's content length, full encoded size, and height (the
// depth of its deepest descendant below it).
type measured struct {
	content, total, height int
}

// encoder memoises measure per element: builders may share one elem between
// several parents, so a tree of n distinct elements is measured in O(n) even
// when its encoding repeats them.
type encoder struct {
	seen map[*elem]measured
}

// measure returns e's sizes. It rejects nesting past limits.MaxNestingDepth
// (also for a shared element first met higher up) and size overflow.
func (enc *encoder) measure(e *elem, depth int) (measured, error) {
	m, ok := enc.seen[e]
	if !ok {
		if depth > limits.MaxNestingDepth {
			return m, errEncodeDepth()
		}
		if e.constructed {
			for _, ch := range e.children {
				cm, err := enc.measure(ch, depth+1)
				if err != nil {
					return m, err
				}
				if m.content, ok = limits.AddInt(m.content, cm.total); !ok {
					return m, fmt.Errorf("asn1.encode: encoding size overflows")
				}
				m.height = max(m.height, cm.height+1)
			}
		} else {
			m.content = len(e.content)
		}
		if m.total, ok = limits.AddInt(headerLen(e, m.content), m.content); !ok {
			return m, fmt.Errorf("asn1.encode: encoding size overflows")
		}
		enc.seen[e] = m
	}
	if depth+m.height > limits.MaxNestingDepth {
		return m, errEncodeDepth()
	}
	return m, nil
}

func errEncodeDepth() error {
	return fmt.Errorf("asn1.encode: nesting deeper than the %d-level limit", limits.MaxNestingDepth)
}

// headerLen is the size of e's identifier and length octets for content length n.
func headerLen(e *elem, n int) int {
	h := 1
	if e.tag >= 0x1f {
		h += len(base128(uint64(e.tag)))
	}
	return h + len(encodeLength(n))
}

// write appends e's encoding to out; measure must have run on e.
func (enc *encoder) write(out []byte, e *elem) []byte {
	id := e.class << 6
	if e.constructed {
		id |= 0x20
	}
	if e.tag < 0x1f {
		out = append(out, id|byte(e.tag))
	} else {
		out = append(out, id|0x1f)
		out = append(out, base128(uint64(e.tag))...)
	}
	out = append(out, encodeLength(enc.seen[e].content)...)
	if !e.constructed {
		return append(out, e.content...)
	}
	if e.class == classUniversal && e.tag == tagSet && len(e.children) > 1 {
		parts := make([][]byte, len(e.children))
		for i, ch := range e.children {
			parts[i] = enc.write(nil, ch)
		}
		sort.Slice(parts, func(i, j int) bool { return bytes.Compare(parts[i], parts[j]) < 0 })
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}
	for _, ch := range e.children {
		out = enc.write(out, ch)
	}
	return out
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
