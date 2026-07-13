// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package parser

// Constant folding at parse time.
//
// Runs from inside Resolve() as a post-step on BinaryExpr and
// UnaryExpr: after the resolver has walked the operands, tryFold*
// checks whether they're compile-time literals (either bare
// IntLit / FloatLit / StringLit / BoolLit / NullLit nodes, OR
// nested BinaryExpr / UnaryExpr whose own Folded field was already
// filled). If yes, the operator is applied at parse time and the
// result stamped as `Folded` on the AST node.
//
// The interpreter's evalBinary / evalUnary consult Folded first
// and short-circuit to its value, skipping the operand walk and
// the op-switch dispatch. Chain-folding works because tryFold
// transitively unwraps `Folded` fields on its operands - so
// `((1+2)*3)+4` collapses to a single IntLit at Resolve time.
//
// Operations that would error at parse time (division by zero,
// shift-count negative, mixed-type comparison, unknown op) leave
// Folded nil so the runtime hits the same error at the same
// source position it would have hit without folding. This keeps
// error semantics unchanged.

// asLit unwraps a BinaryExpr / UnaryExpr's Folded field if
// present, and returns the terminal literal node when the
// argument is (or resolves to) a literal. Returns nil when the
// expression isn't a literal at parse time.
func asLit(e Expr) Expr {
	for {
		switch ex := e.(type) {
		case *BinaryExpr:
			if ex.Folded == nil {
				return nil
			}
			e = ex.Folded
		case *UnaryExpr:
			if ex.Folded == nil {
				return nil
			}
			e = ex.Folded
		case *IntLit, *FloatLit, *StringLit, *BoolLit, *NullLit:
			return e
		default:
			return nil
		}
	}
}

// litInt / litFloat / litString / litBool return the payload of a
// literal Expr along with a "was it a literal of this kind" flag.
// Callers cascade through litInt then litFloat when a numeric
// operation is being folded.
func litInt(e Expr) (int64, bool) {
	if lit, ok := e.(*IntLit); ok {
		return lit.Value, true
	}
	return 0, false
}

func litFloat(e Expr) (float64, bool) {
	if lit, ok := e.(*FloatLit); ok {
		return lit.Value, true
	}
	return 0, false
}

func litString(e Expr) (string, bool) {
	if lit, ok := e.(*StringLit); ok {
		return lit.Value, true
	}
	return "", false
}

func litBool(e Expr) (bool, bool) {
	if lit, ok := e.(*BoolLit); ok {
		return lit.Value, true
	}
	return false, false
}

func litIsNull(e Expr) bool {
	_, ok := e.(*NullLit)
	return ok
}

// tryFoldUnary returns a literal Expr for `op operand` when the
// operand is a compile-time literal and the operation is
// well-defined for it. Returns nil otherwise (runtime evaluates
// as usual).
func tryFoldUnary(ex *UnaryExpr) Expr {
	operand := asLit(ex.Operand)
	if operand == nil {
		return nil
	}
	switch ex.Op {
	case OpNeg:
		if v, ok := litInt(operand); ok {
			return &IntLit{pos: ex.pos, Value: -v}
		}
		if v, ok := litFloat(operand); ok {
			return &FloatLit{pos: ex.pos, Value: -v}
		}
	case OpNot:
		if v, ok := litBool(operand); ok {
			return &BoolLit{pos: ex.pos, Value: !v}
		}
	case OpBitNot:
		if v, ok := litInt(operand); ok {
			return &IntLit{pos: ex.pos, Value: ^v}
		}
	}
	return nil
}

// tryFoldBinary returns a literal Expr for `left op right` when
// both operands are compile-time literals and the operation is
// well-defined (no division by zero, no unsupported type combo).
// Returns nil otherwise.
func tryFoldBinary(ex *BinaryExpr) Expr {
	left := asLit(ex.Left)
	right := asLit(ex.Right)
	if left == nil || right == nil {
		return nil
	}

	// String concat with `+`.
	if ex.Op == OpAdd {
		if l, ok := litString(left); ok {
			if r, ok := litString(right); ok {
				return &StringLit{pos: ex.pos, Value: l + r}
			}
		}
	}

	// Bool logical / equality.
	if l, ok := litBool(left); ok {
		if r, ok := litBool(right); ok {
			switch ex.Op {
			case OpAnd:
				return &BoolLit{pos: ex.pos, Value: l && r}
			case OpOr:
				return &BoolLit{pos: ex.pos, Value: l || r}
			case OpEq:
				return &BoolLit{pos: ex.pos, Value: l == r}
			}
			return nil
		}
	}

	// Null equality with anything only folds when both sides are
	// null literals (matches the runtime's Value.Equal on null).
	if litIsNull(left) && litIsNull(right) && ex.Op == OpEq {
		return &BoolLit{pos: ex.pos, Value: true}
	}

	// Numeric ops. Extract both sides as float first (int auto-
	// promotes), then dispatch to the int-int fast path when both
	// were ints AND the op is exact-int (`/` is float per Python 3
	// semantics; `%` is int-only).
	li, lIsInt := litInt(left)
	ri, rIsInt := litInt(right)
	var lf, rf float64
	var lIsNum, rIsNum bool
	if lIsInt {
		lf, lIsNum = float64(li), true
	} else if v, ok := litFloat(left); ok {
		lf, lIsNum = v, true
	}
	if rIsInt {
		rf, rIsNum = float64(ri), true
	} else if v, ok := litFloat(right); ok {
		rf, rIsNum = v, true
	}
	if !lIsNum || !rIsNum {
		return nil
	}

	// Comparisons produce bool regardless of int/float. When both sides are
	// ints, compare the exact int64 values - promoting to float64 loses
	// precision above 2^53 and would diverge from the runtime's exact int
	// comparison (e.g. 9007199254740993 == 9007199254740992).
	if lIsInt && rIsInt {
		switch ex.Op {
		case OpEq:
			return &BoolLit{pos: ex.pos, Value: li == ri}
		case OpLt:
			return &BoolLit{pos: ex.pos, Value: li < ri}
		case OpGt:
			return &BoolLit{pos: ex.pos, Value: li > ri}
		case OpLe:
			return &BoolLit{pos: ex.pos, Value: li <= ri}
		case OpGe:
			return &BoolLit{pos: ex.pos, Value: li >= ri}
		}
	}
	switch ex.Op {
	case OpEq:
		return &BoolLit{pos: ex.pos, Value: lf == rf}
	case OpLt:
		return &BoolLit{pos: ex.pos, Value: lf < rf}
	case OpGt:
		return &BoolLit{pos: ex.pos, Value: lf > rf}
	case OpLe:
		return &BoolLit{pos: ex.pos, Value: lf <= rf}
	case OpGe:
		return &BoolLit{pos: ex.pos, Value: lf >= rf}
	}

	// Pure-int arithmetic + bit ops.
	if lIsInt && rIsInt {
		switch ex.Op {
		case OpAdd:
			return &IntLit{pos: ex.pos, Value: li + ri}
		case OpSub:
			return &IntLit{pos: ex.pos, Value: li - ri}
		case OpMul:
			return &IntLit{pos: ex.pos, Value: li * ri}
		case OpFloorDiv:
			if ri == 0 {
				return nil
			}
			q := li / ri
			if (li%ri != 0) && ((li < 0) != (ri < 0)) {
				q--
			}
			return &IntLit{pos: ex.pos, Value: q}
		case OpMod:
			if ri == 0 {
				return nil
			}
			return &IntLit{pos: ex.pos, Value: li % ri}
		case OpBitAnd:
			return &IntLit{pos: ex.pos, Value: li & ri}
		case OpBitOr:
			return &IntLit{pos: ex.pos, Value: li | ri}
		case OpBitXor:
			return &IntLit{pos: ex.pos, Value: li ^ ri}
		case OpShl, OpShr:
			if ri < 0 {
				return nil
			}
			if ri >= 64 {
				if ex.Op == OpShl {
					return &IntLit{pos: ex.pos, Value: 0}
				}
				if li < 0 {
					return &IntLit{pos: ex.pos, Value: -1}
				}
				return &IntLit{pos: ex.pos, Value: 0}
			}
			if ex.Op == OpShl {
				return &IntLit{pos: ex.pos, Value: li << uint(ri)}
			}
			return &IntLit{pos: ex.pos, Value: li >> uint(ri)}
		}
	}

	// Mixed / pure-float arithmetic (Python 3: `/` always float).
	switch ex.Op {
	case OpAdd:
		return &FloatLit{pos: ex.pos, Value: lf + rf}
	case OpSub:
		return &FloatLit{pos: ex.pos, Value: lf - rf}
	case OpMul:
		return &FloatLit{pos: ex.pos, Value: lf * rf}
	case OpDiv:
		if rf == 0 {
			return nil
		}
		return &FloatLit{pos: ex.pos, Value: lf / rf}
	}
	return nil
}
