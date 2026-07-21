// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package lint

import "jennifer-lang.dev/jennifer/internal/parser"

// scope.go carries a scope-aware traversal mirroring the resolver's frame
// model (internal/parser/resolver.go): a frame per method-params, block,
// for-header, for-each, catch, and spawn body; the try body runs in the
// enclosing frame. L101 (unused-local) and L104 (throw-non-error) both need
// binding visibility, so they share this walk with different hooks.

// binding is one name in scope. typ is its declared type (for L104);
// reportable marks a binding whose disuse L101 should flag (a local `def`,
// not a param / for-each iterator / catch var / spawn-local / global).
type binding struct {
	name       string
	typ        parser.Type
	def        parser.Node // node to anchor a diagnostic at
	used       bool
	reportable bool
}

// sframe is one scope frame. isGlobal marks the single top-level frame, whose
// bindings are program-global rather than local.
type sframe struct {
	vars     map[string]*binding
	order    []*binding // declaration order, for stable reporting
	isGlobal bool
}

// scoped drives the scope-aware traversal. Callers set the hooks they need;
// all are optional.
type scoped struct {
	stack      []*sframe
	spawnDepth int

	// onRef marks a read of a binding (used by L101). Fires only when the
	// name resolves to a binding in scope.
	onRef func(b *binding)
	// onThrow inspects a throw with a resolver for the current scope (L104).
	onThrow func(t *parser.ThrowStmt, resolve func(string) *binding)
	// onPop fires as a frame leaves scope, after its subtree is walked, so a
	// check can inspect never-used bindings (L101).
	onPop func(f *sframe)
}

func (s *scoped) push(isGlobal bool) *sframe {
	f := &sframe{vars: map[string]*binding{}, isGlobal: isGlobal}
	s.stack = append(s.stack, f)
	return f
}

func (s *scoped) pop() {
	f := s.stack[len(s.stack)-1]
	if s.onPop != nil {
		s.onPop(f)
	}
	s.stack = s.stack[:len(s.stack)-1]
}

func (s *scoped) top() *sframe { return s.stack[len(s.stack)-1] }

// declare adds a binding to the current frame. A duplicate name in the same
// frame (shouldn't happen in valid, non-shadowing programs) is overwritten so
// the traversal stays total.
func (s *scoped) declare(b *binding) {
	f := s.top()
	f.vars[b.name] = b
	f.order = append(f.order, b)
}

// resolve finds the innermost visible binding for name, or nil.
func (s *scoped) resolve(name string) *binding {
	for i := len(s.stack) - 1; i >= 0; i-- {
		if b, ok := s.stack[i].vars[name]; ok {
			return b
		}
	}
	return nil
}

// ref records a read of name.
func (s *scoped) ref(name string) {
	if b := s.resolve(name); b != nil {
		b.used = true
		if s.onRef != nil {
			s.onRef(b)
		}
	}
}

// program walks the whole program: a global frame holding top-level defs,
// kept on the stack while method bodies (which inherit globals) are walked.
func (s *scoped) program(p *parser.Program) {
	s.push(true)
	s.stmts(p.TopLevel)
	for _, m := range p.Methods {
		s.method(m)
	}
	s.pop()
}

// method walks one method: a params frame, then the body block (its own
// frame). Params are declared non-reportable (L101 targets `def`, not params).
func (s *scoped) method(m *parser.MethodDef) {
	s.push(false)
	for i := range m.Params {
		p := &m.Params[i]
		s.declare(&binding{name: p.Name, typ: p.Type})
	}
	s.block(m.Body)
	s.pop()
}

// block walks a *Block as a fresh frame.
func (s *scoped) block(b *parser.Block) {
	if b == nil {
		return
	}
	s.push(false)
	s.stmts(b.Stmts)
	s.pop()
}

// stmtsInEnclosing walks a statement list in the current frame (no push).
// Used for the try body, which the resolver keeps in the enclosing scope.
func (s *scoped) stmtsInEnclosing(ss []parser.Stmt) {
	s.stmts(ss)
}

func (s *scoped) stmts(ss []parser.Stmt) {
	for _, st := range ss {
		s.doStmt(st)
	}
}

func (s *scoped) doStmt(st parser.Stmt) {
	switch n := st.(type) {
	case *parser.DefineStmt:
		// Walk the initializer BEFORE declaring, so `def x init $x` can't
		// mark itself used (and matches left-to-right evaluation).
		s.doExpr(n.InitExpr)
		reportable := !s.top().isGlobal && s.spawnDepth == 0
		s.declare(&binding{name: n.VarName, typ: n.VarType, def: n, reportable: reportable})
	case *parser.AssignStmt:
		// A plain rebind is a write, not a read: it does not count as a use.
		s.doExpr(n.Value)
	case *parser.IndexAssignStmt:
		// $xs[i] = v reads the container to mutate it - a use of the base.
		s.doExpr(n.Target)
		s.doExpr(n.Value)
	case *parser.AppendStmt:
		s.doExpr(n.Target)
		s.doExpr(n.Value)
	case *parser.FieldAssignStmt:
		s.doExpr(n.Target)
		s.doExpr(n.Value)
	case *parser.IfStmt:
		s.doExpr(n.Cond)
		s.block(n.Then)
		for i := range n.ElseIfs {
			s.doExpr(n.ElseIfs[i])
			s.block(n.ElseIfBodies[i])
		}
		s.block(n.Else)
	case *parser.WhileStmt:
		s.doExpr(n.Cond)
		s.block(n.Body)
	case *parser.ForStmt:
		// The C-style for header gets its own frame (Init binds there),
		// then the body is a nested frame.
		s.push(false)
		s.doStmt(n.Init)
		s.doExpr(n.Cond)
		s.doStmt(n.Step)
		s.block(n.Body)
		s.pop()
	case *parser.ForEachStmt:
		// Iterator and body share one frame; the iterator is non-reportable.
		s.push(false)
		s.doExpr(n.Coll)
		s.declare(&binding{name: n.VarName})
		if n.Body != nil {
			s.stmts(n.Body.Stmts)
		}
		s.pop()
	case *parser.RepeatStmt:
		s.block(n.Body)
		s.doExpr(n.Cond)
	case *parser.ReturnStmt:
		s.doExpr(n.Value)
	case *parser.ExitStmt:
		s.doExpr(n.Code)
	case *parser.ThrowStmt:
		if s.onThrow != nil {
			s.onThrow(n, s.resolve)
		}
		s.doExpr(n.Value)
	case *parser.DeferStmt:
		// The deferred call's arguments are evaluated at the defer site, so a
		// `$var` in them is a real read in this scope - without this case the
		// canonical `defer fs.close($f);` would leave `f` looking unused (L101).
		s.doExpr(n.Call)
	case *parser.TryStmt:
		// Try body runs in the enclosing frame (resolver carve-out); catch
		// gets a fresh frame with the caught Error bound non-reportable.
		if n.Body != nil {
			s.stmtsInEnclosing(n.Body.Stmts)
		}
		s.push(false)
		s.declare(&binding{name: n.CatchName, typ: parser.StructType("Error")})
		if n.CatchBody != nil {
			s.stmts(n.CatchBody.Stmts)
		}
		s.pop()
	case *parser.ExprStmt:
		s.doExpr(n.Expr)
	case *parser.BreakStmt, *parser.ContinueStmt:
		// leaves
	}
}

func (s *scoped) doExpr(e parser.Expr) {
	if e == nil {
		return
	}
	switch n := e.(type) {
	case *parser.VarExpr:
		s.ref(n.Name)
	case *parser.ConstRefExpr:
		s.ref(n.Name)
	case *parser.ListLit:
		for _, el := range n.Elements {
			s.doExpr(el)
		}
	case *parser.MapLit:
		for i := range n.Keys {
			s.doExpr(n.Keys[i])
			s.doExpr(n.Values[i])
		}
	case *parser.StructLit:
		for _, f := range n.Fields {
			s.doExpr(f.Expr)
		}
	case *parser.IndexExpr:
		s.doExpr(n.Target)
		s.doExpr(n.Index)
	case *parser.FieldAccessExpr:
		s.doExpr(n.Target)
	case *parser.CallExpr:
		for _, a := range n.Args {
			s.doExpr(a)
		}
	case *parser.QualifiedCallExpr:
		for _, a := range n.Args {
			s.doExpr(a)
		}
	case *parser.LenExpr:
		s.doExpr(n.Operand)
	case *parser.SpawnExpr:
		// Spawn bodies get their own frame. References inside still mark
		// outer bindings used (the spawn captures the enclosing scope), but
		// bindings declared inside are non-reportable (spawnDepth > 0),
		// matching the resolver's spawn carve-out.
		s.spawnDepth++
		s.push(false)
		s.stmts(n.Body)
		s.pop()
		s.spawnDepth--
	case *parser.BinaryExpr:
		s.doExpr(n.Left)
		s.doExpr(n.Right)
	case *parser.UnaryExpr:
		s.doExpr(n.Operand)
		// literals and qualified const refs are leaves
	}
}
