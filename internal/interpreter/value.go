// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package interpreter

import (
	"fmt"

	"github.com/mplx/jennifer-lang/internal/parser"
)

// ValueKind tags a runtime value's type. Future milestones add Float, Bool, Null.
type ValueKind int

const (
	KindNull ValueKind = iota
	KindInt
	KindString
)

func (k ValueKind) String() string {
	switch k {
	case KindNull:
		return "null"
	case KindInt:
		return "int"
	case KindString:
		return "string"
	}
	return "?"
}

// Value is a tagged union for all Jennifer runtime values.
// Using one concrete struct (rather than an interface hierarchy) keeps GC pressure
// low and avoids reflect - important when the binary is built with TinyGo.
type Value struct {
	Kind ValueKind
	Int  int64
	Str  string
}

func Null() Value           { return Value{Kind: KindNull} }
func IntVal(n int64) Value  { return Value{Kind: KindInt, Int: n} }
func StringVal(s string) Value { return Value{Kind: KindString, Str: s} }

// Display formats the value the way `printf` should render it.
func (v Value) Display() string {
	switch v.Kind {
	case KindNull:
		return "null"
	case KindInt:
		return fmt.Sprintf("%d", v.Int)
	case KindString:
		return v.Str
	}
	return "<unknown>"
}

// MatchesDeclared reports whether v's runtime kind matches a declared parser.Type.
func (v Value) MatchesDeclared(t parser.Type) bool {
	switch t {
	case parser.TypeInt:
		return v.Kind == KindInt
	case parser.TypeString:
		return v.Kind == KindString
	}
	return false
}
