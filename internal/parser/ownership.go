// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package parser

// Ownership analysis: two copy elisions for the value-semantics idiom
// `$v = f($v, ...)`, where f mutates its parameter and returns it.
//
// Every store site deep-copies so that no two live bindings share a compound
// backing. That invariant makes most method locals the sole owners of their
// values, and two copies redundant:
//
//   - A method whose every return yields a solely owned value (ReturnsOwned)
//     hands its caller a value nothing else references once the frame exits,
//     so the caller stores the result without copying it again.
//   - In `$v = f(..., $v, ...)` the old value of an owned local `$v` is dead
//     after the call (MoveArg): replaced when the call returns, and gone with
//     the frame when it throws - unless a `try` in the same method could catch
//     the error and read `$v`, so the elision is not made inside one. The
//     parameter then binds the value by move, without the copy.
//
// Together they make a mutate-and-return helper O(1) per call instead of
// O(size of the value). Both analyses work on names: the no-shadowing rule
// makes a name inside a method body refer to one parameter or local throughout
// (sibling blocks may reuse a name, so a name counts as owned only if every
// binding of it is), and never to a global.

// markOwnership sets MethodDef.ReturnsOwned and the MoveArg of eligible calls in
// every method of p. Run after the methods are resolved, since it reads the
// Borrow flags. Idempotent: every flag is recomputed.
func markOwnership(p *Program) {
	for _, m := range p.Methods {
		m.ReturnsOwned = true
	}
	owned := make(map[*MethodDef]map[string]bool, len(p.Methods))
	// Greatest fixpoint: assume every method returns owned values, then clear
	// the flag on any method with a return that is not owned under the current
	// assumptions, until nothing changes. Sound for recursion: an actual
	// returned value comes from a finite chain of calls that ends in a return
	// that is owned on its own terms.
	for changed := true; changed; {
		changed = false
		for _, m := range p.Methods {
			owned[m] = ownedLocals(m)
			if !m.ReturnsOwned {
				continue
			}
			for _, e := range returnValues(m.Body.Stmts, nil) {
				if !ownedExpr(e, owned[m]) {
					m.ReturnsOwned = false
					changed = true
					break
				}
			}
		}
	}
	for _, m := range p.Methods {
		owned[m] = ownedLocals(m)
		markMoveArgs(m.Body.Stmts, owned[m], false)
	}
}

// ownedLocals returns the parameter and local names of m whose bindings solely
// own their values: a parameter that is copied in (not borrowed), and a mutable
// `def` stored by copy - one not borrowed, or one whose initializer is a fresh
// literal or an owning call, which the bind site stores ahead of a borrow. It
// reads ReturnsOwned, so markOwnership recomputes it as those flags settle. A for-each iterator, a catch name and a match
// binder may hold a value that shares its backing, a constant is left out to
// stay conservative, and a name bound more than one way is not owned.
func ownedLocals(m *MethodDef) map[string]bool {
	owned := map[string]bool{}
	not := map[string]bool{}
	for _, p := range m.Params {
		if p.Borrow {
			not[p.Name] = true
		} else {
			owned[p.Name] = true
		}
	}
	walkStmtTree(m.Body.Stmts, func(s Stmt) {
		switch st := s.(type) {
		case *DefineStmt:
			if st.IsConst || (st.Borrow && !freshOrOwnedInit(st.InitExpr)) {
				not[st.VarName] = true
			} else {
				owned[st.VarName] = true
			}
		case *ForEachStmt:
			not[st.VarName] = true
		case *TryStmt:
			not[st.CatchName] = true
		case *MatchStmt:
			for _, arm := range st.Arms {
				if arm.Bind != "" {
					not[arm.Bind] = true
				}
			}
		}
	})
	for n := range not {
		delete(owned, n)
	}
	return owned
}

// freshOrOwnedInit mirrors the bind site's first case: a fresh literal or a call
// to an owning method is stored without a copy and without a borrow.
func freshOrOwnedInit(e Expr) bool {
	if freshLiteralRHS(e) {
		return true
	}
	c, ok := e.(*CallExpr)
	return ok && c.Method != nil && c.Method.ReturnsOwned
}

// returnValues collects the value of every `return` in stmts (nil for a bare
// `return;`). It does not descend into expressions, so a `return` inside a
// spawn body - which returns from the task, not the method - is not seen.
func returnValues(stmts []Stmt, out []Expr) []Expr {
	walkStmtTree(stmts, func(s Stmt) {
		if r, ok := s.(*ReturnStmt); ok {
			out = append(out, r.Value)
		}
	})
	return out
}

// ownedExpr reports whether the value of e, returned from a method whose owned
// locals are owned, is referenced by nothing else once the frame exits.
func ownedExpr(e Expr, owned map[string]bool) bool {
	switch n := e.(type) {
	case nil:
		return true // bare return: null
	case *IntLit, *FloatLit, *StringLit, *BoolLit, *NullLit, *InterpStringExpr,
		*BinaryExpr, *UnaryExpr, *LenExpr:
		return true // a scalar or a new string
	case *ListLit, *MapLit, *StructLit, *RangeExpr, *SliceExpr:
		return true // a new container; its evaluator copies every element in
	case *VarExpr:
		return owned[n.Name]
	case *IndexExpr, *FieldAccessExpr:
		return isOwnedRoot(e, owned)
	case *CallExpr:
		return n.Method != nil && n.Method.ReturnsOwned
	}
	return false
}

// isOwnedRoot reports whether e is a chain of field / index reads rooted at an
// owned local: a part of a value the frame solely owns, so solely owned too
// once the frame exits.
func isOwnedRoot(e Expr, owned map[string]bool) bool {
	switch n := e.(type) {
	case *VarExpr:
		return owned[n.Name]
	case *IndexExpr:
		return isOwnedRoot(n.Target, owned)
	case *FieldAccessExpr:
		return isOwnedRoot(n.Target, owned)
	}
	return false
}

// markMoveArgs sets MoveArg on each `$v = f(..., $v, ...)` in stmts whose `$v`
// is an owned local, named exactly once in the right-hand side, as a direct
// argument. inTry is true inside a `try` body, where the elision is not made: a
// catch could read `$v` after the callee mutated the moved value.
func markMoveArgs(stmts []Stmt, owned map[string]bool, inTry bool) {
	for _, s := range stmts {
		switch st := s.(type) {
		case *AssignStmt:
			markMoveAssign(st, owned, inTry)
		case *IfStmt:
			markMoveArgs(st.Then.Stmts, owned, inTry)
			for _, b := range st.ElseIfBodies {
				markMoveArgs(b.Stmts, owned, inTry)
			}
			if st.Else != nil {
				markMoveArgs(st.Else.Stmts, owned, inTry)
			}
		case *MatchStmt:
			for _, arm := range st.Arms {
				markMoveArgs(arm.Body.Stmts, owned, inTry)
			}
			if st.Else != nil {
				markMoveArgs(st.Else.Stmts, owned, inTry)
			}
		case *WhileStmt:
			markMoveArgs(st.Body.Stmts, owned, inTry)
		case *ForStmt:
			if st.Init != nil {
				markMoveArgs([]Stmt{st.Init}, owned, inTry)
			}
			if st.Step != nil {
				markMoveArgs([]Stmt{st.Step}, owned, inTry)
			}
			markMoveArgs(st.Body.Stmts, owned, inTry)
		case *ForEachStmt:
			markMoveArgs(st.Body.Stmts, owned, inTry)
		case *RepeatStmt:
			markMoveArgs(st.Body.Stmts, owned, inTry)
		case *TryStmt:
			markMoveArgs(st.Body.Stmts, owned, true)
			markMoveArgs(st.CatchBody.Stmts, owned, inTry)
		case *Block:
			markMoveArgs(st.Stmts, owned, inTry)
		}
	}
}

func markMoveAssign(st *AssignStmt, owned map[string]bool, inTry bool) {
	var args []Expr
	var slot *int
	switch c := st.Value.(type) {
	case *CallExpr:
		c.MoveArg = 0
		args, slot = c.Args, &c.MoveArg
	case *QualifiedCallExpr:
		c.MoveArg = 0
		args, slot = c.Args, &c.MoveArg
	default:
		return
	}
	if inTry || !owned[st.VarName] {
		return
	}
	if n, ok := countVarRefs(st.Value, st.VarName); !ok || n != 1 {
		return
	}
	for idx, a := range args {
		if v, ok := a.(*VarExpr); ok && v.Name == st.VarName {
			*slot = idx + 1
			return
		}
	}
}

// countVarRefs counts the `$name` references in e. ok is false when e holds a
// node the count does not know, so a caller fails closed.
func countVarRefs(e Expr, name string) (n int, ok bool) {
	ok = true
	var walk func(Expr)
	walk = func(e Expr) {
		switch x := e.(type) {
		case nil, *IntLit, *FloatLit, *StringLit, *BoolLit, *NullLit,
			*ConstRefExpr, *QualifiedConstRefExpr, *PreEval:
		case *VarExpr:
			if x.Name == name {
				n++
			}
		case *BinaryExpr:
			walk(x.Left)
			walk(x.Right)
		case *UnaryExpr:
			walk(x.Operand)
		case *LenExpr:
			walk(x.Operand)
		case *IndexExpr:
			walk(x.Target)
			walk(x.Index)
		case *SliceExpr:
			walk(x.Target)
			walk(x.Lo)
			walk(x.Hi)
		case *RangeExpr:
			walk(x.Lo)
			walk(x.Hi)
		case *FieldAccessExpr:
			walk(x.Target)
		case *ListLit:
			for _, el := range x.Elements {
				walk(el)
			}
		case *MapLit:
			for k := range x.Keys {
				walk(x.Keys[k])
				walk(x.Values[k])
			}
		case *StructLit:
			for _, f := range x.Fields {
				walk(f.Expr)
			}
		case *InterpStringExpr:
			for _, p := range x.Parts {
				walk(p.Expr)
			}
		case *CallExpr:
			for _, a := range x.Args {
				walk(a)
			}
		case *QualifiedCallExpr:
			for _, a := range x.Args {
				walk(a)
			}
		case *CallValueExpr:
			walk(x.Callee)
			for _, a := range x.Args {
				walk(a)
			}
		default:
			ok = false // a spawn body, or a node this count does not know
		}
	}
	walk(e)
	return n, ok
}

// walkStmtTree calls visit on every statement in stmts and in the blocks nested
// in them, without descending into expressions (so not into spawn bodies).
func walkStmtTree(stmts []Stmt, visit func(Stmt)) {
	for _, s := range stmts {
		visit(s)
		switch st := s.(type) {
		case *IfStmt:
			walkStmtTree(st.Then.Stmts, visit)
			for _, b := range st.ElseIfBodies {
				walkStmtTree(b.Stmts, visit)
			}
			if st.Else != nil {
				walkStmtTree(st.Else.Stmts, visit)
			}
		case *MatchStmt:
			for _, arm := range st.Arms {
				walkStmtTree(arm.Body.Stmts, visit)
			}
			if st.Else != nil {
				walkStmtTree(st.Else.Stmts, visit)
			}
		case *WhileStmt:
			walkStmtTree(st.Body.Stmts, visit)
		case *ForStmt:
			if st.Init != nil {
				walkStmtTree([]Stmt{st.Init}, visit)
			}
			if st.Step != nil {
				walkStmtTree([]Stmt{st.Step}, visit)
			}
			walkStmtTree(st.Body.Stmts, visit)
		case *ForEachStmt:
			walkStmtTree(st.Body.Stmts, visit)
		case *RepeatStmt:
			walkStmtTree(st.Body.Stmts, visit)
		case *TryStmt:
			walkStmtTree(st.Body.Stmts, visit)
			walkStmtTree(st.CatchBody.Stmts, visit)
		case *Block:
			walkStmtTree(st.Stmts, visit)
		}
	}
}
