// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package interpreter

import "jennifer-lang.dev/jennifer/internal/parser"

// Integer fast path. A Value is a large struct, and the general evaluator moves
// one in and out of every node: each operand of `$n % $d == 0` is returned as a
// whole Value, then both are passed by value to evalArithmetic / evalComparison.
// The standard Go compiler copies all of that; on a tight integer loop the
// copies, not the arithmetic, are most of the time. intOperand reads an int
// operand straight from its literal or binding and folds simple int arithmetic
// over such operands, with no Value in between.
//
// It is pure (no side effects, no error, no depth bump - it never recurses into
// a call) and gives up (ok = false) on anything it does not handle exactly: a
// non-int value, an unresolved slot, an out-of-range index, an overflow, a zero
// divisor. The caller then runs the general path, which re-evaluates the same
// side-effect-free operands and raises the same positioned errors as before, so
// the fast path changes no observable behaviour.

// slotBinding returns a pointer to the binding in a resolved slot, or nil when
// the address does not resolve through the slot path (GetAt's name-map fallback
// case, left to the general path).
func (e *Environment) slotBinding(depth, slot int) *Binding {
	cur := e
	for d := 0; d < depth; d++ {
		if cur.parent == nil {
			return nil
		}
		cur = cur.parent
	}
	if slot < 0 || slot >= len(cur.slots) {
		return nil
	}
	return &cur.slots[slot]
}

// varValue returns a pointer to a resolved variable's value, or nil.
func varValue(ex *parser.VarExpr, env *Environment) *Value {
	if ex.Slot < 0 {
		return nil
	}
	if b := env.slotBinding(ex.Depth, ex.Slot); b != nil {
		return &b.Value
	}
	return nil
}

// intOperand evaluates e as an int when that needs no general evaluation.
func intOperand(e parser.Expr, env *Environment) (int64, bool) {
	switch ex := e.(type) {
	case *parser.IntLit:
		return ex.Value, true
	case *parser.VarExpr:
		if v := varValue(ex, env); v != nil && v.Kind == KindInt {
			return v.Int, true
		}
	case *parser.IndexExpr:
		t, ok := ex.Target.(*parser.VarExpr)
		if !ok {
			return 0, false
		}
		parent := varValue(t, env)
		if parent == nil {
			return 0, false
		}
		idx, ok := intOperand(ex.Index, env)
		if !ok || idx < 0 {
			return 0, false
		}
		switch parent.Kind {
		case KindBytes:
			if idx < int64(len(parent.Bytes)) {
				return int64(parent.Bytes[idx]), true
			}
		case KindList:
			if idx < int64(len(parent.List)) {
				if el := &parent.List[idx]; el.Kind == KindInt {
					return el.Int, true
				}
			}
		}
	case *parser.BinaryExpr:
		if ex.Folded != nil {
			return intOperand(ex.Folded, env)
		}
		switch ex.Op {
		case parser.OpAdd, parser.OpSub, parser.OpMul, parser.OpFloorDiv, parser.OpMod:
		default:
			return 0, false
		}
		a, ok := intOperand(ex.Left, env)
		if !ok {
			return 0, false
		}
		b, ok := intOperand(ex.Right, env)
		if !ok {
			return 0, false
		}
		return intArith(ex.Op, a, b)
	}
	return 0, false
}

// intArith applies an arithmetic operator to two ints with evalArithmetic's
// semantics (floored `//` and `%`), reporting ok = false wherever evalArithmetic
// would raise an error.
func intArith(op parser.BinaryOp, a, b int64) (int64, bool) {
	switch op {
	case parser.OpAdd:
		s, ovf := addOverflow(a, b)
		return s, !ovf
	case parser.OpSub:
		d, ovf := subOverflow(a, b)
		return d, !ovf
	case parser.OpMul:
		p, ovf := mulOverflow(a, b)
		return p, !ovf
	case parser.OpFloorDiv:
		if b == 0 || (a == minInt64 && b == -1) {
			return 0, false
		}
		q := a / b
		if (a%b != 0) && ((a < 0) != (b < 0)) {
			q--
		}
		return q, true
	case parser.OpMod:
		if b == 0 {
			return 0, false
		}
		return flooredMod(a, b), true
	}
	return 0, false
}

// intCompare applies a comparison operator to two ints. Every comparison is
// defined on two ints, so it never fails for a comparison op.
func intCompare(op parser.BinaryOp, a, b int64) (bool, bool) {
	switch op {
	case parser.OpLt:
		return a < b, true
	case parser.OpGt:
		return a > b, true
	case parser.OpLe:
		return a <= b, true
	case parser.OpGe:
		return a >= b, true
	case parser.OpEq:
		return a == b, true
	case parser.OpNeq:
		return a != b, true
	}
	return false, false
}

// fastCondition evaluates e as a bool when it is a bool literal, a bool
// variable, an int comparison whose operands intOperand handles, or `and` / `or`
// / `not` over those. `and` / `or` short-circuit exactly as evalLogical does: a
// right operand the left one decides is never looked at.
func fastCondition(e parser.Expr, env *Environment) (bool, bool) {
	switch ex := e.(type) {
	case *parser.BoolLit:
		return ex.Value, true
	case *parser.VarExpr:
		if v := varValue(ex, env); v != nil && v.Kind == KindBool {
			return v.Bool, true
		}
	case *parser.UnaryExpr:
		if ex.Folded != nil {
			return fastCondition(ex.Folded, env)
		}
		if ex.Op == parser.OpNot {
			if r, ok := fastCondition(ex.Operand, env); ok {
				return !r, true
			}
		}
	case *parser.BinaryExpr:
		if ex.Folded != nil {
			return fastCondition(ex.Folded, env)
		}
		switch {
		case ex.Op == parser.OpAnd || ex.Op == parser.OpOr:
			l, ok := fastCondition(ex.Left, env)
			if !ok {
				return false, false
			}
			if l == (ex.Op == parser.OpOr) {
				return l, true
			}
			return fastCondition(ex.Right, env)
		case ex.Op.IsComparison():
			l, ok := intOperand(ex.Left, env)
			if !ok {
				return false, false
			}
			r, ok := intOperand(ex.Right, env)
			if !ok {
				return false, false
			}
			return intCompare(ex.Op, l, r)
		}
	}
	return false, false
}

// fastAssign performs `$v = e;` in place when $v is a resolved, non-const int or
// bool variable and e is an operand the fast path evaluates. Reports whether it
// did; when it did not, nothing was written and execAssign runs in full.
func fastAssign(st *parser.AssignStmt, env *Environment) bool {
	if st.Slot < 0 {
		return false
	}
	b := env.slotBinding(st.Depth, st.Slot)
	if b == nil || b.IsConst {
		return false
	}
	v := &b.Value
	switch {
	case b.DeclType.Kind == parser.TypeInt && v.Kind == KindInt:
		if n, ok := intOperand(st.Value, env); ok {
			v.Int = n
			return true
		}
	case b.DeclType.Kind == parser.TypeBool && v.Kind == KindBool:
		if r, ok := fastCondition(st.Value, env); ok {
			v.Bool = r
			return true
		}
	}
	return false
}
