// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package asn1lib

import (
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func TestInstallRegistersEveryAsn1Builtin(t *testing.T) {
	in := interpreter.New()
	Install(in)
	for _, name := range []string{
		"decode", "encode", "typeOf", "get", "has", "asBool", "isNull",
		"isConstructed", "length", "printableString", "ia5String", "set", "sequence",
	} {
		if in.LookupNamespacedBuiltin("asn1", name) == nil {
			t.Errorf("asn1.%s is not registered", name)
		}
	}
}

// TestAsn1AccessorsAndConstructors builds a SEQUENCE mixing several primitive
// and constructed elements, round-trips it through DER, and walks it with the
// accessors the codec tests leave uncovered (typeOf / isConstructed / get / has
// / asBool / isNull), plus the printableString / ia5String / set constructors.
func TestAsn1AccessorsAndConstructors(t *testing.T) {
	seq := mustBuild(t, sequenceFn, interpreter.ListVal(childListType, []interpreter.Value{
		mustBuild(t, booleanFn, interpreter.BoolVal(true)),
		mustBuild(t, printableStringFn, interpreter.StringVal("Hi There")), // PrintableString charset
		mustBuild(t, ia5StringFn, interpreter.StringVal("a@b.com")),        // IA5 allows '@'
		mustBuild(t, nullFn),
		mustBuild(t, setFn, interpreter.ListVal(childListType, []interpreter.Value{
			mustBuild(t, integerFn, interpreter.IntVal(1)),
			mustBuild(t, integerFn, interpreter.IntVal(2)),
		})),
	}))
	node := decode(t, der(t, seq)) // round-trip through DER so we walk a decoded tree

	// typeOf returns a non-empty type name for the root and a child.
	if v, err := call(typeOfFn, node); err != nil || v.Kind != interpreter.KindString || v.Str == "" {
		t.Errorf("typeOf(root) = %+v, err %v", v, err)
	}
	if v, err := call(typeOfFn, node, interpreter.StringVal("/3")); err != nil || v.Str == "" {
		t.Errorf("typeOf(/3) = %+v, err %v", v, err)
	}

	// isConstructed: the sequence is constructed; the boolean child is primitive.
	if v, _ := call(isConstructedFn, node); !v.Bool {
		t.Error("isConstructed(root) should be true")
	}
	if v, _ := call(isConstructedFn, node, interpreter.StringVal("/0")); v.Bool {
		t.Error("isConstructed(boolean child) should be false")
	}

	// get fetches a child as an asn1.Value (opaque object).
	if child, err := call(getFn, node, interpreter.StringVal("/0")); err != nil || child.Kind != interpreter.KindObject {
		t.Fatalf("get(/0) = %+v, err %v; want an object", child, err)
	}

	// asBool on the boolean child.
	if v, err := call(asBoolFn, node, interpreter.StringVal("/0")); err != nil || !v.Bool {
		t.Errorf("asBool(/0) = %+v, err %v; want true", v, err)
	}

	// isNull: true at the NULL child, false at the boolean.
	if v, _ := call(isNullFn, node, interpreter.StringVal("/3")); !v.Bool {
		t.Error("isNull(/3) should be true")
	}
	if v, _ := call(isNullFn, node, interpreter.StringVal("/0")); v.Bool {
		t.Error("isNull(/0) should be false")
	}

	// has: an existing pointer vs an out-of-range one.
	if v, _ := call(hasFn, node, interpreter.StringVal("/2")); !v.Bool {
		t.Error("has(/2) should be true")
	}
	if v, _ := call(hasFn, node, interpreter.StringVal("/99")); v.Bool {
		t.Error("has(/99) should be false")
	}

	// The SET child is a constructed container of length 2.
	if v, err := call(lengthFn, node, interpreter.StringVal("/4")); err != nil || v.Int != 2 {
		t.Errorf("length(/4) = %+v, err %v; want 2", v, err)
	}

	// The string children read back through asString.
	if v, err := call(asStringFn, node, interpreter.StringVal("/1")); err != nil || v.Str != "Hi There" {
		t.Errorf("asString(/1) = %+v, err %v; want \"Hi There\"", v, err)
	}
	if v, err := call(asStringFn, node, interpreter.StringVal("/2")); err != nil || v.Str != "a@b.com" {
		t.Errorf("asString(/2) = %+v, err %v; want \"a@b.com\"", v, err)
	}
}
