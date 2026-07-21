// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package parser

// foldMinInt64 mirrors the interpreter's minInt64: the one int64 value with no
// positive image, so `MinInt64 // -1` and `MinInt64 * -1` are left unfolded.
const foldMinInt64 = -1 << 63

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
			case OpNeq:
				return &BoolLit{pos: ex.pos, Value: l != r}
			}
			return nil
		}
	}

	// Null equality with anything only folds when both sides are
	// null literals (matches the runtime's Value.Equal on null).
	if litIsNull(left) && litIsNull(right) {
		switch ex.Op {
		case OpEq:
			return &BoolLit{pos: ex.pos, Value: true}
		case OpNeq:
			return &BoolLit{pos: ex.pos, Value: false}
		}
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
		case OpNeq:
			return &BoolLit{pos: ex.pos, Value: li != ri}
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
	// A mixed int/float comparison must stay exact: promoting the int operand to
	// float64 here loses precision above 2^53 and would diverge from the runtime's
	// exact compareIntFloat (the language guarantees mixed comparison is exact, so
	// 9007199254740993 == 9007199254740992.0 is false). Leave it unfolded and let
	// the runtime decide. This guards only comparisons; mixed-int/float arithmetic
	// below stays folded, matching the language's int->float arithmetic promotion.
	if ex.Op.IsComparison() && lIsInt != rIsInt {
		return nil
	}
	// Remaining comparisons here are float/float, which are already exact.
	switch ex.Op {
	case OpEq:
		return &BoolLit{pos: ex.pos, Value: lf == rf}
	case OpNeq:
		return &BoolLit{pos: ex.pos, Value: lf != rf}
	case OpLt:
		return &BoolLit{pos: ex.pos, Value: lf < rf}
	case OpGt:
		return &BoolLit{pos: ex.pos, Value: lf > rf}
	case OpLe:
		return &BoolLit{pos: ex.pos, Value: lf <= rf}
	case OpGe:
		return &BoolLit{pos: ex.pos, Value: lf >= rf}
	}

	// Pure-int arithmetic + bit ops. Overflowing +, -, *, and MinInt64 // -1
	// are left unfolded (return nil) so the runtime raises the same positioned
	// error it would for non-literal operands (implementation-note 16).
	if lIsInt && rIsInt {
		switch ex.Op {
		case OpAdd:
			s := li + ri
			if (li > 0 && ri > 0 && s < 0) || (li < 0 && ri < 0 && s >= 0) {
				return nil
			}
			return &IntLit{pos: ex.pos, Value: s}
		case OpSub:
			d := li - ri
			if (li >= 0 && ri < 0 && d < 0) || (li < 0 && ri > 0 && d >= 0) {
				return nil
			}
			return &IntLit{pos: ex.pos, Value: d}
		case OpMul:
			if li == 0 || ri == 0 {
				return &IntLit{pos: ex.pos, Value: 0}
			}
			if (li == foldMinInt64 && ri == -1) || (ri == foldMinInt64 && li == -1) {
				return nil
			}
			p := li * ri
			if p/li != ri {
				return nil
			}
			return &IntLit{pos: ex.pos, Value: p}
		case OpFloorDiv:
			if ri == 0 || (li == foldMinInt64 && ri == -1) {
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
			// Floored, matching the runtime and `//`.
			r := li % ri
			if r != 0 && ((r < 0) != (ri < 0)) {
				r += ri
			}
			return &IntLit{pos: ex.pos, Value: r}
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
