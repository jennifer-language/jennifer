// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package interpreter

import (
	"strconv"
	"strings"

	"github.com/mplx/jennifer-lang/internal/parser"
)

// DisplayFloat formats a float64 for human output. Unlike `strconv.FormatFloat`
// with verb 'g', it guarantees the result is recognisable as a float: if the
// shortest round-trip representation has no `.`, `e`, or `E`, we append `.0`.
// So `5.0` prints as "5.0", not "5" - the value's *type* stays visible even
// when its value happens to be a whole number.
func DisplayFloat(f float64) string {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

// ValueKind tags a runtime value's type.
type ValueKind int

const (
	KindNull ValueKind = iota
	KindInt
	KindFloat
	KindString
	KindBool
)

func (k ValueKind) String() string {
	switch k {
	case KindNull:
		return "null"
	case KindInt:
		return "int"
	case KindFloat:
		return "float"
	case KindString:
		return "string"
	case KindBool:
		return "bool"
	}
	return "?"
}

// Value is a tagged union for all Jennifer runtime values.
// One concrete struct rather than a Go interface hierarchy keeps GC pressure
// low and avoids reflect - important when the binary is built with TinyGo.
type Value struct {
	Kind  ValueKind
	Int   int64
	Float float64
	Str   string
	Bool  bool
}

func Null() Value              { return Value{Kind: KindNull} }
func IntVal(n int64) Value     { return Value{Kind: KindInt, Int: n} }
func FloatVal(f float64) Value { return Value{Kind: KindFloat, Float: f} }
func StringVal(s string) Value { return Value{Kind: KindString, Str: s} }
func BoolVal(b bool) Value     { return Value{Kind: KindBool, Bool: b} }

// ZeroFor returns the zero value for a declared parser.Type. Used when a
// variable is defined without an `init` clause.
func ZeroFor(t parser.Type) Value {
	switch t {
	case parser.TypeInt:
		return IntVal(0)
	case parser.TypeFloat:
		return FloatVal(0)
	case parser.TypeString:
		return StringVal("")
	case parser.TypeBool:
		return BoolVal(false)
	case parser.TypeNull:
		return Null()
	}
	return Null()
}

// Display formats the value the way `printf` should render it.
func (v Value) Display() string {
	switch v.Kind {
	case KindNull:
		return "null"
	case KindInt:
		return strconv.FormatInt(v.Int, 10)
	case KindFloat:
		return DisplayFloat(v.Float)
	case KindString:
		return v.Str
	case KindBool:
		if v.Bool {
			return "true"
		}
		return "false"
	}
	return "<unknown>"
}

// MatchesDeclared reports whether v's runtime kind matches a declared parser.Type.
// `null` is the only value accepted for parser.TypeNull and is also accepted
// anywhere (a null can stand in for any typed slot) - see [[design-null]] notes
// in CLAUDE.md if added later. For M2 we keep it strict to surface bugs early.
func (v Value) MatchesDeclared(t parser.Type) bool {
	switch t {
	case parser.TypeInt:
		return v.Kind == KindInt
	case parser.TypeFloat:
		return v.Kind == KindFloat
	case parser.TypeString:
		return v.Kind == KindString
	case parser.TypeBool:
		return v.Kind == KindBool
	case parser.TypeNull:
		return v.Kind == KindNull
	}
	return false
}

// AsFloat returns the value as float64 if it's int or float; ok=false otherwise.
// Used by arithmetic and comparison to implement int->float promotion.
func (v Value) AsFloat() (float64, bool) {
	switch v.Kind {
	case KindInt:
		return float64(v.Int), true
	case KindFloat:
		return v.Float, true
	}
	return 0, false
}

// Equal implements Jennifer's `==` semantics. Same-kind: deep equal. Mixed
// int/float: compared as floats (per the int->float promotion rule). Any
// other cross-kind pairing is not equal.
func (v Value) Equal(o Value) bool {
	if v.Kind == o.Kind {
		switch v.Kind {
		case KindNull:
			return true
		case KindInt:
			return v.Int == o.Int
		case KindFloat:
			return v.Float == o.Float
		case KindString:
			return v.Str == o.Str
		case KindBool:
			return v.Bool == o.Bool
		}
	}
	// numeric promotion
	if a, ok := v.AsFloat(); ok {
		if b, ok := o.AsFloat(); ok {
			return a == b
		}
	}
	return false
}
