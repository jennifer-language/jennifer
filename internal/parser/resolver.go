// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package parser

import "fmt"

// M16.5.2: scope analyser that resolves every variable / constant
// reference to a (Depth, Slot) coordinate at parse time. Depth is the
// number of parent frames to walk from the innermost enclosing scope;
// Slot is the index in that frame's slot slice.
//
// Ships two side benefits alongside the runtime speedup:
//   - Undefined-variable and shadowing errors surface at parse time
//     rather than first-execution.
//   - Blocks that create a fresh runtime frame (Block, MethodDef body,
//     ForEachStmt body, TryStmt catch, SpawnExpr body) carry a
//     NumSlots hint so the interpreter can pre-size the slot slice.
//
// The Resolve() entry point is idempotent - calling it twice yields
// the same annotations. Tests that hand-build AST fragments can skip
// it; the interpreter falls back to name-based lookup when Depth/Slot
// are still the -1 sentinel.

// Resolve walks the program's AST and fills in (Depth, Slot)
// coordinates on VarExpr / ConstRefExpr / AssignStmt / DefineStmt /
// ForEachStmt / TryStmt / Block.NumSlots / Program.NumGlobals. A
// non-nil error is a positioned scope-analysis error: undefined
// reference, shadowing, or `throw`-related name misuse. The AST is
// mutated in place.
func Resolve(p *Program) error {
	r := &resolver{}
	return r.resolveProgram(p)
}

type resolver struct {
	// scopes is the active scope stack; scopes[0] is the outermost
	// (either globals when analysing top-level, or the callFrame
	// when analysing a method body - never both at once because
	// method calls jump directly to globals via effectiveGlobal
	// and don't inherit the caller's locals).
	scopes []*scopeFrame
	// methods records the names of every top-level `func` so bare
	// unqualified references can distinguish "call a method" from
	// "reference an undefined name." Populated during the hoist
	// pass; consulted by VarExpr / ConstRefExpr resolution when
	// the name doesn't match any slot on the scope stack.
	methods map[string]struct{}
	// structs records struct-decl names for the same reason:
	// `Point{...}` literals and `def x as Point;` don't require
	// a slot lookup but the resolver still needs to know they
	// exist for future type-check hooks.
	structs map[string]struct{}
}

type scopeFrame struct {
	// slots keyed by name -> (Slot, IsConst). Order is the order
	// defs were encountered; slot indices are dense from 0.
	slots map[string]slotInfo
	// count is the running slot allocator; also the final NumSlots
	// for the frame.
	count int
	// isRoot is true for the globals frame and for the callFrame
	// of a method being analysed. A ref that would need to walk
	// past a root frame is an undefined-name error (methods don't
	// close over the caller's locals; globals are the only thing
	// visible above a method's params).
	isRoot bool
}

type slotInfo struct {
	Slot    int
	IsConst bool
}

func (r *resolver) resolveProgram(p *Program) error {
	// Hoist method + struct names so their bodies can reference each
	// other in any order (mirrors the interpreter's Run() hoisting).
	r.methods = make(map[string]struct{}, len(p.Methods))
	for _, m := range p.Methods {
		r.methods[m.Name] = struct{}{}
	}
	r.structs = make(map[string]struct{}, len(p.Structs))
	for _, s := range p.Structs {
		r.structs[s.Name] = struct{}{}
	}
	// Struct definitions themselves declare no runtime bindings, so
	// they don't consume slots. StructDef's InitExpr side (if the
	// struct's field types reference names) doesn't currently exist
	// in the grammar - types are values, not expressions.

	// Top-level statements share the globals frame.
	globals := &scopeFrame{slots: map[string]slotInfo{}, isRoot: true}
	r.push(globals)
	for _, s := range p.TopLevel {
		if err := r.resolveStmt(s); err != nil {
			return err
		}
	}
	p.NumGlobals = globals.count
	r.pop()

	// Method bodies get their own scope chain rooted on the callFrame
	// (params in slot 0..N-1) with globals shadow-visible for
	// name-based reads.
	for _, m := range p.Methods {
		if err := r.resolveMethod(m, p.NumGlobals, globals); err != nil {
			return err
		}
	}

	return nil
}

// resolveMethod analyses one method body. The scope stack becomes
// [callFrame, ...body-nested-blocks]. globals is passed in as the
// pre-analysed top-level frame so global references from inside the
// method resolve correctly.
func (r *resolver) resolveMethod(m *MethodDef, numGlobals int, globals *scopeFrame) error {
	callFrame := &scopeFrame{slots: map[string]slotInfo{}, isRoot: true}
	for i, p := range m.Params {
		if _, ok := callFrame.slots[p.Name]; ok {
			return &ParseError{
				Msg:  fmt.Sprintf("parameter %q duplicated in method %q", p.Name, m.Name),
				File: p.File, Line: p.Line, Col: p.Col,
			}
		}
		callFrame.slots[p.Name] = slotInfo{Slot: i}
		callFrame.count = i + 1
	}

	// Rebase the resolver's scope stack: analysing a method body,
	// globals is at the base (for name-based reads only) with the
	// callFrame on top. Save/restore so top-level analysis state
	// isn't disturbed.
	saved := r.scopes
	r.scopes = []*scopeFrame{globals, callFrame}
	err := r.resolveBlock(m.Body)
	r.scopes = saved
	return err
}

// resolveBlock analyses a Block that will create a fresh runtime
// frame. Pushes a new scopeFrame, walks the statements, records the
// resulting slot count on the Block, then pops. Nested blocks nest
// this pattern.
func (r *resolver) resolveBlock(b *Block) error {
	frame := &scopeFrame{slots: map[string]slotInfo{}}
	r.push(frame)
	for _, s := range b.Stmts {
		if err := r.resolveStmt(s); err != nil {
			return err
		}
	}
	b.NumSlots = frame.count
	r.pop()
	return nil
}

func (r *resolver) push(f *scopeFrame) { r.scopes = append(r.scopes, f) }
func (r *resolver) pop()               { r.scopes = r.scopes[:len(r.scopes)-1] }

func (r *resolver) current() *scopeFrame { return r.scopes[len(r.scopes)-1] }

// define records a new binding in the current frame. Returns a
// positioned parse error if the name already binds in any visible
// enclosing scope (Jennifer forbids shadowing).
func (r *resolver) define(name string, isConst bool, file string, line, col int) (int, error) {
	if r.existsInChain(name) {
		return -1, &ParseError{
			Msg:  fmt.Sprintf("name %q is already defined in an enclosing scope", name),
			File: file, Line: line, Col: col,
		}
	}
	cur := r.current()
	idx := cur.count
	cur.slots[name] = slotInfo{Slot: idx, IsConst: isConst}
	cur.count++
	return idx, nil
}

// existsInChain walks the current scope stack looking for name, but
// stops at (and excludes from the walk) any frame above the innermost
// root frame - a callee's scope chain doesn't include the caller's
// locals. Method params + method body locals see globals; nested
// blocks see everything up to the method's own root.
func (r *resolver) existsInChain(name string) bool {
	for i := len(r.scopes) - 1; i >= 0; i-- {
		if _, ok := r.scopes[i].slots[name]; ok {
			return true
		}
		if r.scopes[i].isRoot {
			// Root frames terminate the walk downward (the frames
			// below a non-outermost root belong to enclosing
			// analysis contexts we shouldn't reach into).
			// The outermost frame (globals) is also root; walking
			// past it happens only when the loop's i reaches 0.
			break
		}
	}
	// Also check globals if we haven't already (the globals frame
	// is scopes[0] when we're analysing a method body; the loop
	// above may have short-circuited at the method's callFrame).
	if len(r.scopes) > 0 {
		if _, ok := r.scopes[0].slots[name]; ok {
			return true
		}
	}
	return false
}

// lookup finds name and returns (Depth, Slot, isConst). Depth is the
// number of parent-pointer walks from the innermost frame. Returns
// (-1, -1, false, false) when the name isn't in scope.
func (r *resolver) lookup(name string) (depth, slot int, isConst, ok bool) {
	// Walk innermost -> outermost.
	depth = 0
	for i := len(r.scopes) - 1; i >= 0; i-- {
		if info, hit := r.scopes[i].slots[name]; hit {
			return depth, info.Slot, info.IsConst, true
		}
		depth++
		if r.scopes[i].isRoot && i > 0 {
			// A root above globals (a method's callFrame) means
			// we've stopped seeing block-nested locals; the only
			// remaining visible frame is globals at scopes[0].
			// Skip anything strictly between i-1 and 0 (there's
			// nothing there in the current design, but the check
			// makes the depth math robust to future refactors).
			if _, hit := r.scopes[0].slots[name]; hit {
				return depth, r.scopes[0].slots[name].Slot, r.scopes[0].slots[name].IsConst, true
			}
			return -1, -1, false, false
		}
	}
	return -1, -1, false, false
}

// isMethod reports whether name is a top-level user method.
func (r *resolver) isMethod(name string) bool {
	_, ok := r.methods[name]
	return ok
}

func (r *resolver) resolveStmt(s Stmt) error {
	switch st := s.(type) {
	case *DefineStmt:
		return r.resolveDefine(st)
	case *AssignStmt:
		return r.resolveAssign(st)
	case *IndexAssignStmt:
		if err := r.resolveExpr(st.Target); err != nil {
			return err
		}
		return r.resolveExpr(st.Value)
	case *AppendStmt:
		if err := r.resolveExpr(st.Target); err != nil {
			return err
		}
		return r.resolveExpr(st.Value)
	case *FieldAssignStmt:
		if err := r.resolveExpr(st.Target); err != nil {
			return err
		}
		return r.resolveExpr(st.Value)
	case *IfStmt:
		if err := r.resolveExpr(st.Cond); err != nil {
			return err
		}
		if err := r.resolveBlock(st.Then); err != nil {
			return err
		}
		for i, c := range st.ElseIfs {
			if err := r.resolveExpr(c); err != nil {
				return err
			}
			if err := r.resolveBlock(st.ElseIfBodies[i]); err != nil {
				return err
			}
		}
		if st.Else != nil {
			return r.resolveBlock(st.Else)
		}
		return nil
	case *WhileStmt:
		if err := r.resolveExpr(st.Cond); err != nil {
			return err
		}
		return r.resolveBlock(st.Body)
	case *ForStmt:
		// C-style `for` header can introduce a fresh binding via
		// Init (usually a DefineStmt). Its scope covers Cond, Step,
		// and Body - so the header lives in the same fresh frame as
		// the body.
		frame := &scopeFrame{slots: map[string]slotInfo{}}
		r.push(frame)
		if st.Init != nil {
			if err := r.resolveStmt(st.Init); err != nil {
				r.pop()
				return err
			}
		}
		if st.Cond != nil {
			if err := r.resolveExpr(st.Cond); err != nil {
				r.pop()
				return err
			}
		}
		if st.Step != nil {
			if err := r.resolveStmt(st.Step); err != nil {
				r.pop()
				return err
			}
		}
		// The body block will push its own frame; that's fine, it
		// still finds the init var one frame up. Slot allocated in
		// the header stays in `frame`.
		if err := r.resolveBlock(st.Body); err != nil {
			r.pop()
			return err
		}
		st.Body.NumSlots += frame.count
		// The interpreter still creates just one fresh env per
		// for-header iteration; encoding the header slot into the
		// same NumSlots keeps runtime allocation aligned. The
		// header's Init DefineStmt got Slot 0..N-1 in `frame`;
		// the body block's own defs got Slot 0..M-1 in the body
		// frame. That's two frames at runtime; both are counted
		// via their respective NumSlots.
		frame.count = 0 // avoid double-count in tests / dumps
		r.pop()
		return nil
	case *ForEachStmt:
		if err := r.resolveExpr(st.Coll); err != nil {
			return err
		}
		// The iterator lives in a fresh per-iteration frame together
		// with any body-local defs. Push, allocate slot 0 for the
		// iterator, walk the body's stmts into the same frame.
		frame := &scopeFrame{slots: map[string]slotInfo{}}
		r.push(frame)
		if r.existsInChain(st.VarName) {
			r.pop()
			file, line, col := posFor(st)
			return &ParseError{
				Msg:  fmt.Sprintf("for-each iterator %q shadows an enclosing binding", st.VarName),
				File: file, Line: line, Col: col,
			}
		}
		frame.slots[st.VarName] = slotInfo{Slot: 0}
		frame.count = 1
		st.IterSlot = 0
		// Body stmts share this same frame (no fresh block); we walk
		// them directly instead of calling resolveBlock, then stamp
		// NumSlots on the block from the shared frame.
		for _, bs := range st.Body.Stmts {
			if err := r.resolveStmt(bs); err != nil {
				r.pop()
				return err
			}
		}
		st.Body.NumSlots = frame.count
		r.pop()
		return nil
	case *RepeatStmt:
		if err := r.resolveBlock(st.Body); err != nil {
			return err
		}
		return r.resolveExpr(st.Cond)
	case *ReturnStmt:
		if st.Value != nil {
			return r.resolveExpr(st.Value)
		}
		return nil
	case *ExitStmt:
		if st.Code != nil {
			return r.resolveExpr(st.Code)
		}
		return nil
	case *ThrowStmt:
		return r.resolveExpr(st.Value)
	case *TryStmt:
		// The try body runs in the enclosing env at runtime (execTry
		// calls execStmts(st.Body.Stmts, env) rather than execBlock).
		// The resolver walks its stmts directly in the current scope
		// to match. Body-local defs land in the enclosing frame.
		for _, bs := range st.Body.Stmts {
			if err := r.resolveStmt(bs); err != nil {
				return err
			}
		}
		st.Body.NumSlots = 0
		// Catch handler runs in a fresh runtime frame (catchEnv);
		// the caught value takes slot 0.
		frame := &scopeFrame{slots: map[string]slotInfo{}}
		r.push(frame)
		if r.existsInChain(st.CatchName) {
			r.pop()
			return &ParseError{
				Msg:  fmt.Sprintf("catch binding %q shadows an enclosing binding", st.CatchName),
				File: st.CatchFile, Line: st.CatchLine, Col: st.CatchCol,
			}
		}
		frame.slots[st.CatchName] = slotInfo{Slot: 0}
		frame.count = 1
		st.CatchSlot = 0
		for _, bs := range st.CatchBody.Stmts {
			if err := r.resolveStmt(bs); err != nil {
				r.pop()
				return err
			}
		}
		st.CatchBody.NumSlots = frame.count
		r.pop()
		return nil
	case *BreakStmt, *ContinueStmt:
		return nil
	case *ExprStmt:
		return r.resolveExpr(st.Expr)
	case *Block:
		return r.resolveBlock(st)
	case *ImportStmt, *StructDef, *MethodDef:
		// Structural declarations: no bindings introduced at the
		// resolver level (methods are hoisted; structs are types).
		return nil
	}
	return nil
}

func (r *resolver) resolveDefine(st *DefineStmt) error {
	// Init expression is evaluated BEFORE the name is in scope, so
	// resolve it first. This mirrors the interpreter's ordering and
	// makes `def x as int init $x + 1;` a proper "undefined $x" error
	// rather than a silent self-reference.
	if st.InitExpr != nil {
		if err := r.resolveExpr(st.InitExpr); err != nil {
			return err
		}
	}
	file, line, col := posFor(st)
	slot, err := r.define(st.VarName, st.IsConst, file, line, col)
	if err != nil {
		return err
	}
	st.Slot = slot
	return nil
}

func (r *resolver) resolveAssign(st *AssignStmt) error {
	if err := r.resolveExpr(st.Value); err != nil {
		return err
	}
	depth, slot, isConst, ok := r.lookup(st.VarName)
	if !ok {
		file, line, col := posFor(st)
		return &ParseError{
			Msg:  fmt.Sprintf("undefined variable %q", st.VarName),
			File: file, Line: line, Col: col,
		}
	}
	if isConst {
		file, line, col := posFor(st)
		return &ParseError{
			Msg:  fmt.Sprintf("cannot assign to constant %q", st.VarName),
			File: file, Line: line, Col: col,
		}
	}
	st.Depth = depth
	st.Slot = slot
	return nil
}

func (r *resolver) resolveExpr(e Expr) error {
	if e == nil {
		return nil
	}
	switch ex := e.(type) {
	case *VarExpr:
		depth, slot, _, ok := r.lookup(ex.Name)
		if !ok {
			file, line, col := posFor(ex)
			return &ParseError{
				Msg:  fmt.Sprintf("undefined variable %q", ex.Name),
				File: file, Line: line, Col: col,
			}
		}
		ex.Depth = depth
		ex.Slot = slot
		return nil
	case *ConstRefExpr:
		// A bare identifier in expression position: could be a
		// constant in scope OR a top-level method reference used
		// bare (which is a runtime error, but the parser doesn't
		// know until it sees the call).
		depth, slot, _, ok := r.lookup(ex.Name)
		if ok {
			ex.Depth = depth
			ex.Slot = slot
			return nil
		}
		// Not a slot binding. Might be a method name (bare method
		// reference is a runtime error today; leave it for the
		// interpreter's classifier). Might be an undefined name.
		// Defer to runtime for compatibility with existing tests
		// that expect "hint to use $" and similar error text.
		return nil
	case *CallExpr:
		for _, a := range ex.Args {
			if err := r.resolveExpr(a); err != nil {
				return err
			}
		}
		return nil
	case *QualifiedCallExpr:
		for _, a := range ex.Args {
			if err := r.resolveExpr(a); err != nil {
				return err
			}
		}
		return nil
	case *QualifiedConstRefExpr:
		return nil
	case *LenExpr:
		return r.resolveExpr(ex.Operand)
	case *SpawnExpr:
		// M16.5.2: spawn bodies are deliberately left unresolved.
		// The runtime's snapshotForSpawn produces a two-frame
		// duplex (globals-snap + locals-snap) that doesn't line up
		// with the resolver's single-frame view of "the enclosing
		// scope," and inventing depth arithmetic to reconcile the
		// two would be brittle. Spawn bodies fall back to
		// name-based lookup at runtime - not hot-loop territory,
		// so the perf regression is limited to coarse-grained
		// concurrency dispatch.
		return nil
	case *IndexExpr:
		if err := r.resolveExpr(ex.Target); err != nil {
			return err
		}
		return r.resolveExpr(ex.Index)
	case *FieldAccessExpr:
		return r.resolveExpr(ex.Target)
	case *ListLit:
		for _, el := range ex.Elements {
			if err := r.resolveExpr(el); err != nil {
				return err
			}
		}
		return nil
	case *MapLit:
		for i := range ex.Keys {
			if err := r.resolveExpr(ex.Keys[i]); err != nil {
				return err
			}
			if err := r.resolveExpr(ex.Values[i]); err != nil {
				return err
			}
		}
		return nil
	case *StructLit:
		for _, f := range ex.Fields {
			if err := r.resolveExpr(f.Expr); err != nil {
				return err
			}
		}
		return nil
	case *UnaryExpr:
		return r.resolveExpr(ex.Operand)
	case *BinaryExpr:
		if err := r.resolveExpr(ex.Left); err != nil {
			return err
		}
		return r.resolveExpr(ex.Right)
	case *IntLit, *FloatLit, *StringLit, *BoolLit, *NullLit:
		return nil
	}
	return nil
}

// posFor extracts the (file, line, col) triple from any node that
// carries a pos. Uses the pos.Filename / pos.Pos accessors so any
// AST node type works.
func posFor(n Node) (string, int, int) {
	line, col := n.Pos()
	return n.Filename(), line, col
}
