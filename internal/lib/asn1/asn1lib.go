// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package asn1lib is the `asn1` library: ASN.1 BER decode / DER encode over an
// opaque `asn1.Value`, the byte-level foundation for the LDAP / SNMP clients
// (and, later, X.509 / PKCS surfaces). It is designed like the other
// opaque-value libraries (`json` / `toml` / `xml` / `yaml`): `decode` yields an
// `asn1.Value` (a KindObject) that callers walk with `(node, pointer)`
// accessors, and there is no coercion into a typed struct. Where those libraries
// address by JSON Pointer over map keys and list indices, an ASN.1 tree is a
// nest of ordered elements, so a pointer's tokens are child indices ("/0/2" is
// the first child's third child). Values are built with typed constructors
// (`integer`, `octetString`, `sequence`, `tagged`, ...) and serialised with
// `encode`.
//
// The byte codec is in asn1codec.go; this file is the Jennifer-facing surface.
package asn1lib

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// LibraryName is the namespace prefix (`asn1.`) and the `use` name.
const LibraryName = "asn1"

// Install registers the asn1 surface.
func Install(in *interpreter.Interpreter) {
	// An asn1.Value displays as a compact structural summary.
	in.RegisterNamespacedObject(LibraryName, "Value", func(inner interpreter.Value) string {
		if !isElem(inner) {
			return "<asn1.Value>"
		}
		var sb strings.Builder
		describe(&sb, inner, 0)
		return sb.String()
	})

	in.RegisterNamespaced(LibraryName, "decode", decodeFn)
	in.RegisterNamespaced(LibraryName, "encode", encodeFn)

	// Read accessors (node[, pointer]).
	in.RegisterNamespaced(LibraryName, "typeOf", typeOfFn)
	in.RegisterNamespaced(LibraryName, "tagClass", tagClassFn)
	in.RegisterNamespaced(LibraryName, "tagNumber", tagNumberFn)
	in.RegisterNamespaced(LibraryName, "isConstructed", isConstructedFn)
	in.RegisterNamespaced(LibraryName, "get", getFn)
	in.RegisterNamespaced(LibraryName, "has", hasFn)
	in.RegisterNamespaced(LibraryName, "length", lengthFn)
	in.RegisterNamespaced(LibraryName, "asInt", asIntFn)
	in.RegisterNamespaced(LibraryName, "asBool", asBoolFn)
	in.RegisterNamespaced(LibraryName, "asString", asStringFn)
	in.RegisterNamespaced(LibraryName, "asBytes", asBytesFn)
	in.RegisterNamespaced(LibraryName, "asOid", asOidFn)
	in.RegisterNamespaced(LibraryName, "isNull", isNullFn)

	// Build constructors + encode.
	in.RegisterNamespaced(LibraryName, "integer", integerFn)
	in.RegisterNamespaced(LibraryName, "enumerated", enumeratedFn)
	in.RegisterNamespaced(LibraryName, "boolean", booleanFn)
	in.RegisterNamespaced(LibraryName, "null", nullFn)
	in.RegisterNamespaced(LibraryName, "octetString", octetStringFn)
	in.RegisterNamespaced(LibraryName, "utf8String", utf8StringFn)
	in.RegisterNamespaced(LibraryName, "printableString", printableStringFn)
	in.RegisterNamespaced(LibraryName, "ia5String", ia5StringFn)
	in.RegisterNamespaced(LibraryName, "oid", oidFn)
	in.RegisterNamespaced(LibraryName, "sequence", sequenceFn)
	in.RegisterNamespaced(LibraryName, "set", setFn)
	in.RegisterNamespaced(LibraryName, "tagged", taggedFn)
	in.RegisterNamespaced(LibraryName, "retag", retagFn)
}

// wrap re-wraps an element tree as an asn1.Value so a walk stays opaque.
func wrap(elem interpreter.Value) interpreter.Value {
	return interpreter.ObjectVal(LibraryName, "Value", elem)
}

// takeElem unwraps an asn1.Value argument to its inner element tree.
func takeElem(fnName string, v interpreter.Value) (interpreter.Value, error) {
	inner, ok := v.AsObject(LibraryName, "Value")
	if !ok {
		return interpreter.Value{}, fmt.Errorf("%s: argument must be an asn1.Value, got %s", fnName, v.Kind)
	}
	if !isElem(inner) {
		return interpreter.Value{}, fmt.Errorf("%s: asn1.Value is empty", fnName)
	}
	return inner, nil
}

// nodeAt resolves the shared (node[, pointer]) shape to one element.
func nodeAt(fnName string, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return interpreter.Value{}, fmt.Errorf("%s expects 1 or 2 arguments (asn1.Value[, pointer]), got %d", fnName, len(args))
	}
	node, err := takeElem(fnName, args[0])
	if err != nil {
		return interpreter.Value{}, err
	}
	ptr := ""
	if len(args) == 2 {
		if args[1].Kind != interpreter.KindString {
			return interpreter.Value{}, fmt.Errorf("%s: pointer must be string, got %s", fnName, args[1].Kind)
		}
		ptr = args[1].Str
	}
	return resolvePointer(fnName, node, ptr)
}

// resolvePointer walks a child-index pointer ("/0/2") from node to a descendant.
func resolvePointer(fnName string, node interpreter.Value, ptr string) (interpreter.Value, error) {
	if ptr == "" {
		return node, nil
	}
	if ptr[0] != '/' {
		return interpreter.Value{}, fmt.Errorf("%s: pointer %q must be empty or start with '/'", fnName, ptr)
	}
	cur := node
	for _, tok := range strings.Split(ptr[1:], "/") {
		idx, err := strconv.Atoi(tok)
		if err != nil || idx < 0 {
			return interpreter.Value{}, fmt.Errorf("%s: pointer token %q is not a child index", fnName, tok)
		}
		if !elemConstructed(cur) {
			return interpreter.Value{}, fmt.Errorf("%s: cannot index into a primitive element", fnName)
		}
		ch := elemChildren(cur)
		if idx >= len(ch) {
			return interpreter.Value{}, fmt.Errorf("%s: child index %d out of range (%d children)", fnName, idx, len(ch))
		}
		cur = ch[idx]
	}
	return cur, nil
}

// requireUniversal checks that an element is a universal primitive of the given
// tag before a leaf extractor reads its content.
func requireUniversal(fnName string, node interpreter.Value, tag int, want string) error {
	if elemClass(node) != classUniversal || elemTag(node) != tag || elemConstructed(node) {
		return fmt.Errorf("%s: element is %s, not %s", fnName, typeName(elemClass(node), elemTag(node)), want)
	}
	return nil
}

// ---- decode / encode ----

func decodeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("asn1.decode expects 1 argument (bytes), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("asn1.decode: argument must be bytes, got %s", args[0].Kind)
	}
	elem, err := decodeTree(args[0].Bytes)
	if err != nil {
		return interpreter.Null(), err
	}
	return wrap(elem), nil
}

func encodeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("asn1.encode expects 1 argument (asn1.Value), got %d", len(args))
	}
	elem, err := takeElem("asn1.encode", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	out, err := encodeTree(elem)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.BytesVal(out), nil
}

// ---- read accessors ----

func typeOfFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("asn1.typeOf", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(typeName(elemClass(node), elemTag(node))), nil
}

func tagClassFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("asn1.tagClass", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(classString(elemClass(node))), nil
}

func tagNumberFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("asn1.tagNumber", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.IntVal(int64(elemTag(node))), nil
}

func isConstructedFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("asn1.isConstructed", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.BoolVal(elemConstructed(node)), nil
}

func getFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("asn1.get", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return wrap(node), nil
}

func hasFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("asn1.has expects 2 arguments (asn1.Value, pointer), got %d", len(args))
	}
	node, err := takeElem("asn1.has", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("asn1.has: pointer must be string, got %s", args[1].Kind)
	}
	_, rerr := resolvePointer("asn1.has", node, args[1].Str)
	return interpreter.BoolVal(rerr == nil), nil
}

func lengthFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("asn1.length", args)
	if err != nil {
		return interpreter.Null(), err
	}
	if !elemConstructed(node) {
		return interpreter.Null(), fmt.Errorf("asn1.length: element is primitive (%s), not a container", typeName(elemClass(node), elemTag(node)))
	}
	return interpreter.IntVal(int64(len(elemChildren(node)))), nil
}

func asIntFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("asn1.asInt", args)
	if err != nil {
		return interpreter.Null(), err
	}
	if elemClass(node) != classUniversal || elemConstructed(node) || (elemTag(node) != tagInteger && elemTag(node) != tagEnumerated) {
		return interpreter.Null(), fmt.Errorf("asn1.asInt: element is %s, not integer or enumerated", typeName(elemClass(node), elemTag(node)))
	}
	n, err := decodeIntContent(elemContent(node))
	if err != nil {
		return interpreter.Null(), fmt.Errorf("asn1.asInt: %v", err)
	}
	return interpreter.IntVal(n), nil
}

func asBoolFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("asn1.asBool", args)
	if err != nil {
		return interpreter.Null(), err
	}
	if err := requireUniversal("asn1.asBool", node, tagBoolean, "boolean"); err != nil {
		return interpreter.Null(), err
	}
	b := elemContent(node)
	if len(b) != 1 {
		return interpreter.Null(), fmt.Errorf("asn1.asBool: boolean content must be exactly one octet, got %d", len(b))
	}
	return interpreter.BoolVal(b[0] != 0), nil
}

func asStringFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("asn1.asString", args)
	if err != nil {
		return interpreter.Null(), err
	}
	if elemClass(node) != classUniversal || elemConstructed(node) {
		return interpreter.Null(), fmt.Errorf("asn1.asString: element is %s, not a string type", typeName(elemClass(node), elemTag(node)))
	}
	switch elemTag(node) {
	case tagUTF8String, tagPrintableString, tagIA5String, tagOctetString:
		// ok
	default:
		return interpreter.Null(), fmt.Errorf("asn1.asString: element is %s, not a string type", typeName(elemClass(node), elemTag(node)))
	}
	b := elemContent(node)
	if !utf8.Valid(b) {
		return interpreter.Null(), fmt.Errorf("asn1.asString: content is not valid UTF-8 (use asBytes for raw octets)")
	}
	return interpreter.StringVal(string(b)), nil
}

func asBytesFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("asn1.asBytes", args)
	if err != nil {
		return interpreter.Null(), err
	}
	if elemConstructed(node) {
		return interpreter.Null(), fmt.Errorf("asn1.asBytes: element is constructed (%s); it has children, not raw content", typeName(elemClass(node), elemTag(node)))
	}
	return interpreter.BytesVal(append([]byte(nil), elemContent(node)...)), nil
}

func asOidFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("asn1.asOid", args)
	if err != nil {
		return interpreter.Null(), err
	}
	if err := requireUniversal("asn1.asOid", node, tagOID, "oid"); err != nil {
		return interpreter.Null(), err
	}
	s, err := decodeOID(elemContent(node))
	if err != nil {
		return interpreter.Null(), fmt.Errorf("asn1.asOid: %v", err)
	}
	return interpreter.StringVal(s), nil
}

func isNullFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("asn1.isNull", args)
	if err != nil {
		return interpreter.Null(), err
	}
	isNull := elemClass(node) == classUniversal && elemTag(node) == tagNull && !elemConstructed(node)
	return interpreter.BoolVal(isNull), nil
}

// ---- build constructors ----

func oneInt(fnName string, args []interpreter.Value) (int64, error) {
	if len(args) != 1 {
		return 0, fmt.Errorf("%s expects 1 argument (int), got %d", fnName, len(args))
	}
	if args[0].Kind != interpreter.KindInt {
		return 0, fmt.Errorf("%s: argument must be int, got %s", fnName, args[0].Kind)
	}
	return args[0].Int, nil
}

func integerFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	n, err := oneInt("asn1.integer", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return wrap(makeElem(classUniversal, tagInteger, false, encodeIntContent(n), nil)), nil
}

func enumeratedFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	n, err := oneInt("asn1.enumerated", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return wrap(makeElem(classUniversal, tagEnumerated, false, encodeIntContent(n), nil)), nil
}

func booleanFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 || args[0].Kind != interpreter.KindBool {
		return interpreter.Null(), fmt.Errorf("asn1.boolean expects 1 bool argument")
	}
	b := byte(0)
	if args[0].Bool {
		b = 0xff
	}
	return wrap(makeElem(classUniversal, tagBoolean, false, []byte{b}, nil)), nil
}

func nullFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 0 {
		return interpreter.Null(), fmt.Errorf("asn1.null expects no arguments, got %d", len(args))
	}
	return wrap(makeElem(classUniversal, tagNull, false, nil, nil)), nil
}

func oneBytes(fnName string, args []interpreter.Value) ([]byte, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%s expects 1 argument (bytes), got %d", fnName, len(args))
	}
	if args[0].Kind != interpreter.KindBytes {
		return nil, fmt.Errorf("%s: argument must be bytes, got %s", fnName, args[0].Kind)
	}
	return append([]byte(nil), args[0].Bytes...), nil
}

func octetStringFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	b, err := oneBytes("asn1.octetString", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return wrap(makeElem(classUniversal, tagOctetString, false, b, nil)), nil
}

func oneString(fnName string, args []interpreter.Value) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("%s expects 1 argument (string), got %d", fnName, len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return "", fmt.Errorf("%s: argument must be string, got %s", fnName, args[0].Kind)
	}
	return args[0].Str, nil
}

func utf8StringFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	s, err := oneString("asn1.utf8String", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return wrap(makeElem(classUniversal, tagUTF8String, false, []byte(s), nil)), nil
}

func printableStringFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	s, err := oneString("asn1.printableString", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return wrap(makeElem(classUniversal, tagPrintableString, false, []byte(s), nil)), nil
}

func ia5StringFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	s, err := oneString("asn1.ia5String", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return wrap(makeElem(classUniversal, tagIA5String, false, []byte(s), nil)), nil
}

func oidFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	s, err := oneString("asn1.oid", args)
	if err != nil {
		return interpreter.Null(), err
	}
	content, err := encodeOID(s)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("asn1.oid: %v", err)
	}
	return wrap(makeElem(classUniversal, tagOID, false, content, nil)), nil
}

// childElems unwraps a Jennifer `list of asn1.Value` into element trees.
func childElems(fnName string, v interpreter.Value) ([]interpreter.Value, error) {
	if v.Kind != interpreter.KindList {
		return nil, fmt.Errorf("%s: argument must be a list of asn1.Value, got %s", fnName, v.Kind)
	}
	out := make([]interpreter.Value, len(v.List))
	for i, e := range v.List {
		elem, err := takeElem(fnName, e)
		if err != nil {
			return nil, fmt.Errorf("%s: element %d is not an asn1.Value", fnName, i)
		}
		out[i] = elem
	}
	return out, nil
}

func containerFn(fnName string, tag int, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("%s expects 1 argument (list of asn1.Value), got %d", fnName, len(args))
	}
	children, err := childElems(fnName, args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	return wrap(makeElem(classUniversal, tag, true, nil, children)), nil
}

func sequenceFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return containerFn("asn1.sequence", tagSequence, args)
}

func setFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return containerFn("asn1.set", tagSet, args)
}

// classAndTag reads the (class string, tag int, value) argument prefix shared by
// tagged / retag.
func classAndTag(fnName string, args []interpreter.Value) (int, int, interpreter.Value, error) {
	if len(args) != 3 {
		return 0, 0, interpreter.Value{}, fmt.Errorf("%s expects 3 arguments (class, tagNumber, value), got %d", fnName, len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return 0, 0, interpreter.Value{}, fmt.Errorf("%s: class must be a string, got %s", fnName, args[0].Kind)
	}
	class, err := className(args[0].Str)
	if err != nil {
		return 0, 0, interpreter.Value{}, fmt.Errorf("%s: %v", fnName, err)
	}
	if args[1].Kind != interpreter.KindInt || args[1].Int < 0 || args[1].Int > maxTagNumber {
		return 0, 0, interpreter.Value{}, fmt.Errorf("%s: tagNumber must be between 0 and %d", fnName, maxTagNumber)
	}
	inner, err := takeElem(fnName, args[2])
	if err != nil {
		return 0, 0, interpreter.Value{}, err
	}
	return class, int(args[1].Int), inner, nil
}

// taggedFn wraps a value in an EXPLICIT [class tagNumber] constructed element -
// the outer tag holds the value as its single child, its own tag intact.
func taggedFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	class, tag, inner, err := classAndTag("asn1.tagged", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return wrap(makeElem(class, tag, true, nil, []interpreter.Value{inner})), nil
}

// retagFn replaces a value's outer tag with [class tagNumber], keeping its
// content and constructed-ness - IMPLICIT tagging (as SNMP PDUs and many LDAP
// fields use).
func retagFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	class, tag, inner, err := classAndTag("asn1.retag", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return wrap(makeElem(class, tag, elemConstructed(inner), elemContent(inner), elemChildren(inner))), nil
}

// ---- display ----

// describe writes a compact structural summary of an element, bounded in depth
// and breadth so a large tree stays readable.
func describe(sb *strings.Builder, node interpreter.Value, depth int) {
	name := typeName(elemClass(node), elemTag(node))
	if elemClass(node) != classUniversal {
		name = fmt.Sprintf("%s[%d]", name, elemTag(node))
	}
	if elemConstructed(node) {
		sb.WriteString(name)
		sb.WriteString(" {")
		if depth >= 6 {
			sb.WriteString(" ... }")
			return
		}
		ch := elemChildren(node)
		for i, c := range ch {
			if i >= 16 {
				sb.WriteString(fmt.Sprintf(" ... %d more", len(ch)-i))
				break
			}
			if i > 0 {
				sb.WriteByte(',')
			}
			sb.WriteByte(' ')
			describe(sb, c, depth+1)
		}
		sb.WriteString(" }")
		return
	}
	// Primitive: name plus a short rendering of the content.
	sb.WriteString(name)
	switch {
	case elemClass(node) == classUniversal && elemTag(node) == tagNull:
		return
	case elemClass(node) == classUniversal && elemTag(node) == tagBoolean && len(elemContent(node)) == 1:
		sb.WriteString(fmt.Sprintf(" %t", elemContent(node)[0] != 0))
	case elemClass(node) == classUniversal && (elemTag(node) == tagInteger || elemTag(node) == tagEnumerated):
		if n, err := decodeIntContent(elemContent(node)); err == nil {
			sb.WriteString(fmt.Sprintf(" %d", n))
		}
	case elemClass(node) == classUniversal && elemTag(node) == tagOID:
		if s, err := decodeOID(elemContent(node)); err == nil {
			sb.WriteString(" " + s)
		}
	default:
		sb.WriteString(fmt.Sprintf(" (%d bytes)", len(elemContent(node))))
	}
}
