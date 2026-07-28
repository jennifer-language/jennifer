// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package parser

import "fmt"

// Scope analyser that resolves every variable / constant
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
	// methods records the top-level `func` names + their MethodDef
	// pointers so bare unqualified references can distinguish
	// "call a method" from "reference an undefined name," and so
	// CallExpr.Method pre-resolution has a target to
	// point at. Populated during the hoist pass; consulted by
	// VarExpr / ConstRefExpr resolution when the name doesn't
	// match any slot on the scope stack, and by CallExpr
	// resolution to fill in the pre-resolved method pointer.
	methods map[string]*MethodDef
	// structs records struct-decl names for the same reason:
	// `Point{...}` literals and `def x as Point;` don't require
	// a slot lookup but the resolver still needs to know they
	// exist for future type-check hooks.
	structs map[string]*StructDef
	// enums records enum declarations by name so a `match` over an
	// enum-typed subject can resolve bare variant patterns and check
	// exhaustiveness at parse time.
	enums map[string]*EnumDef
	// prog is the program being resolved, so a deferred enum match can
	// register itself on PendingEnumMatches for the interpreter to validate
	// after modules load.
	prog *Program
}

// namedTypeOf returns a pointer to t when t names a struct / enum type, else
// nil. The pointer aliases the AST node's own Type, so a later pass that stamps
// a module type's (stem, path) identity in place (the interpreter's
// resolveDeclaredTypesOnce) is visible through it - which is what lets a
// deferred enum match be validated against the right module type.
func namedTypeOf(t *Type) *Type {
	if t == nil || t.Kind != TypeStruct {
		return nil
	}
	return t
}

// isLocalNamed reports whether t names a type declared in *this* file: no
// namespace and no module path. A module or library type carries one of those,
// and must never be matched against a same-named local declaration - that is
// how a local `def enum Shape` used to hijack a `match` over a module's
// `sh.Shape` (wrong exhaustiveness, wrong payload shape).
func isLocalNamed(t *Type) bool {
	return t != nil && t.Kind == TypeStruct && t.StructNS == "" && t.ModPath == ""
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
	// DeclType points at the binding's declared named type (a struct or enum
	// type), or nil for a primitive / compound / untyped binding. Recorded for
	// `def`s and method params so a `match` subject that is a variable or
	// parameter of an enum type drives enum-pattern resolution and
	// exhaustiveness. It is a pointer into the AST so the full identity
	// (namespace + module path) stays visible, including whatever a later
	// stamping pass fills in. Foreach / catch bindings stay untyped.
	DeclType *Type
}

func (r *resolver) resolveProgram(p *Program) error {
	r.prog = p
	// Resolve is idempotent, so start the deferred-match list empty rather than
	// appending a second copy of every entry on a re-resolve.
	p.PendingEnumMatches = nil
	// Hoist method + struct names so their bodies can reference each
	// other in any order (mirrors the interpreter's Run() hoisting).
	r.methods = make(map[string]*MethodDef, len(p.Methods))
	for _, m := range p.Methods {
		r.methods[m.Name] = m
	}
	r.structs = make(map[string]*StructDef, len(p.Structs))
	for _, s := range p.Structs {
		r.structs[s.Name] = s
	}
	r.enums = make(map[string]*EnumDef, len(p.Enums))
	for _, e := range p.Enums {
		r.enums[e.Name] = e
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

	// A method may not share its name with a top-level variable / constant:
	// the no-shadowing rule applies both directions (a bare `foo` reference
	// would be ambiguous between the global binding and the method).
	for _, m := range p.Methods {
		if _, clash := globals.slots[m.Name]; clash {
			file, line, col := posFor(m)
			return &ParseError{
				Msg:  fmt.Sprintf("method %q shares its name with a top-level variable or constant", m.Name),
				File: file, Line: line, Col: col,
			}
		}
	}

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
		callFrame.slots[p.Name] = slotInfo{Slot: i, DeclType: namedTypeOf(&m.Params[i].Type)}
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
func (r *resolver) define(name string, isConst bool, declType *Type, file string, line, col int) (int, error) {
	if r.existsInChain(name) {
		return -1, &ParseError{
			Msg:  fmt.Sprintf("name %q is already defined in an enclosing scope", name),
			File: file, Line: line, Col: col,
		}
	}
	cur := r.current()
	idx := cur.count
	cur.slots[name] = slotInfo{Slot: idx, IsConst: isConst, DeclType: declType}
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

// resolveTarget resolves a mutation lvalue (the target of an index / append /
// field assignment). It mirrors resolveExpr but lets the root VarExpr be a
// constant: the runtime raises the clearer "cannot mutate constant (const is
// deep)" error there, so the `$CONST`-read rejection does not preempt it. Index
// sub-expressions are ordinary reads and resolve through resolveExpr.
func (r *resolver) resolveTarget(e Expr) error {
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
	case *IndexExpr:
		if err := r.resolveTarget(ex.Target); err != nil {
			return err
		}
		return r.resolveExpr(ex.Index)
	case *FieldAccessExpr:
		return r.resolveTarget(ex.Target)
	}
	return r.resolveExpr(e)
}

// bindingType returns the declared named type of an in-scope binding, or nil if
// absent or untyped. Mirrors lookup's scope walk.
func (r *resolver) bindingType(name string) *Type {
	for i := len(r.scopes) - 1; i >= 0; i-- {
		if info, hit := r.scopes[i].slots[name]; hit {
			return info.DeclType
		}
		if r.scopes[i].isRoot && i > 0 {
			if info, hit := r.scopes[0].slots[name]; hit {
				return info.DeclType
			}
			return nil
		}
	}
	return nil
}

// subjectType returns the declared named type of a match subject when it is
// statically knowable: a variable / constant (from its binding) or a field
// access into a *local* struct (from the struct's field type, recursively).
// nil for anything else (a method result has no declared type). Field access
// only descends through a local struct: a module struct's definition is not
// resolver-visible, and resolving it against a same-named local one would pick
// the wrong field types.
func (r *resolver) subjectType(e Expr) *Type {
	switch ex := e.(type) {
	case *VarExpr:
		return r.bindingType(ex.Name)
	case *ConstRefExpr:
		return r.bindingType(ex.Name)
	case *FieldAccessExpr:
		parent := r.subjectType(ex.Target)
		if !isLocalNamed(parent) {
			return nil
		}
		if sd, ok := r.structs[parent.StructName]; ok && sd != nil {
			for fi := range sd.Fields {
				if sd.Fields[fi].Name == ex.Field {
					return namedTypeOf(&sd.Fields[fi].Type)
				}
			}
		}
	}
	return nil
}

// rejectBinderPatternOnUntypedSubject errors when a value-match arm head is a
// `Variant(bind)` destructuring pattern whose name is a known enum variant - a
// pattern the resolver cannot serve because the subject's enum type is not
// statically known. Payload-less heads are left alone (the runtime fallback
// handles them). Returns nil when the head is not such a pattern.
func (r *resolver) rejectBinderPatternOnUntypedSubject(v Expr) error {
	call, ok := v.(*CallExpr)
	if !ok || len(call.Args) != 1 {
		return nil
	}
	if _, ok := call.Args[0].(*ConstRefExpr); !ok {
		return nil
	}
	for _, ed := range r.enums {
		for vi := range ed.Variants {
			if ed.Variants[vi].Name == call.Callee {
				file, line, col := posFor(v)
				return &ParseError{
					Msg:  fmt.Sprintf("`when %s(...)` destructures enum %s, but this `match` subject is not statically an enum type; assign the subject to a variable first (`def s as %s init ...;` then `match ($s)`)", call.Callee, ed.Name, ed.Name),
					File: file, Line: line, Col: col,
				}
			}
		}
	}
	return nil
}

// localEnumOf returns the locally-declared EnumDef t names, or nil. Only a type
// with no namespace and no module path can name a local enum - a `mod.Shape`
// subject must never resolve to a same-named local `def enum Shape`.
func (r *resolver) localEnumOf(t *Type) *EnumDef {
	if !isLocalNamed(t) {
		return nil
	}
	return r.enums[t.StructName]
}

// deferrableEnumType reports whether t names a type this file cannot see the
// declaration of - a module or library type. A `match` over such a subject may
// still be an enum pattern match; the resolver rewrites the arms so the binder
// gets a slot, and records the match on Program.PendingEnumMatches for the
// interpreter to validate once the module is loaded.
func deferrableEnumType(t *Type) bool {
	return t != nil && t.Kind == TypeStruct && (t.StructNS != "" || t.ModPath != "")
}

// checkSpawnEnums applies enum-match checking inside a `spawn` body. Spawn
// bodies are deliberately not slot-resolved (see resolveExpr's SpawnExpr case),
// which used to mean they escaped enum checking altogether - a non-exhaustive
// match inside a spawn simply fell through at runtime, with none of the
// compile-time guarantee the feature exists to give. This walk is types-only: it
// allocates no slots and rewrites no references, so the runtime's name-based
// lookup for spawn bodies is untouched. It only tracks declared types well
// enough to find each match's subject enum, tag the arms as variant patterns
// (leaving BindSlot at -1 so the payload binds by name), and run the same checks
// the batch path runs. A match whose arms are not all variant-shaped is left
// alone, so an ordinary value match inside a spawn keeps working.
func (r *resolver) checkSpawnEnums(body []Stmt) error {
	if len(body) == 0 {
		return nil
	}
	// types is a scope stack of declared types layered over the enclosing
	// resolver scopes, which a spawn body captures by value.
	types := []map[string]*Type{{}}
	lookup := func(name string) *Type {
		for i := len(types) - 1; i >= 0; i-- {
			if t, hit := types[i][name]; hit {
				return t
			}
		}
		return r.bindingType(name)
	}
	var subjectType func(e Expr) *Type
	subjectType = func(e Expr) *Type {
		switch ex := e.(type) {
		case *VarExpr:
			return lookup(ex.Name)
		case *ConstRefExpr:
			return lookup(ex.Name)
		case *FieldAccessExpr:
			parent := subjectType(ex.Target)
			if !isLocalNamed(parent) {
				return nil
			}
			if sd, ok := r.structs[parent.StructName]; ok && sd != nil {
				for fi := range sd.Fields {
					if sd.Fields[fi].Name == ex.Field {
						return namedTypeOf(&sd.Fields[fi].Type)
					}
				}
			}
		}
		return nil
	}
	var walkStmts func(ss []Stmt) error
	var walkBlock func(bl *Block) error
	var walkExpr func(e Expr) error
	// tag rewrites the arms of a variant-shaped match into patterns without
	// touching slots, so both the runtime fast path and checkEnumArms can read
	// them. Values is left in place: nothing consumes it once Variant is set.
	// tag rewrites every arm head into (Variant, Bind). It reports ok=false when
	// some arm is not pattern-shaped at all, in which case the match is left as
	// an ordinary value match.
	tag := func(st *MatchStmt) (bool, error) {
		for ai := range st.Arms {
			if len(st.Arms[ai].Values) != 1 {
				return false, nil
			}
			variant, bind, ok, perr := r.patternExpr(st.Arms[ai].Values[0])
			if perr != nil {
				return false, perr
			}
			if !ok {
				return false, nil
			}
			st.Arms[ai].Variant = variant
			st.Arms[ai].Bind = bind
		}
		return true, nil
	}
	walkMatch := func(st *MatchStmt) error {
		if len(st.Arms) == 0 {
			return nil
		}
		t := subjectType(st.Subject)
		// A locally declared enum subject is known to be an enum, so any
		// pattern-shaped head is a variant - exactly as the batch path reads it.
		// That matters for variants spelled like constants (`when A`), which the
		// conservative shape test below deliberately does not claim.
		if ed := r.localEnumOf(t); ed != nil {
			ok, err := tag(st)
			if err != nil || !ok {
				return err
			}
			return checkEnumMatchArms(st, ed, ed.Name)
		}
		// A module type cannot be confirmed to be an enum from here, so only
		// commit when every head is unambiguously a variant pattern; the
		// interpreter finishes the check once the module is loaded.
		if deferrableEnumType(t) && allArmsVariantShaped(st.Arms) {
			ok, err := tag(st)
			if err != nil || !ok {
				return err
			}
			st.EnumType = t
			if r.prog != nil {
				r.prog.PendingEnumMatches = append(r.prog.PendingEnumMatches, st)
			}
		}
		return nil
	}
	walkExpr = func(e Expr) error {
		if sp, ok := e.(*SpawnExpr); ok {
			return r.checkSpawnEnums(sp.Body)
		}
		return nil
	}
	walkBlock = func(bl *Block) error {
		if bl == nil {
			return nil
		}
		types = append(types, map[string]*Type{})
		err := walkStmts(bl.Stmts)
		types = types[:len(types)-1]
		return err
	}
	walkStmts = func(ss []Stmt) error {
		for _, s := range ss {
			switch st := s.(type) {
			case *DefineStmt:
				if t := namedTypeOf(&st.VarType); t != nil {
					types[len(types)-1][st.VarName] = t
				}
				if err := walkExpr(st.InitExpr); err != nil {
					return err
				}
			case *MatchStmt:
				if err := walkMatch(st); err != nil {
					return err
				}
				for ai := range st.Arms {
					if err := walkBlock(st.Arms[ai].Body); err != nil {
						return err
					}
				}
				if err := walkBlock(st.Else); err != nil {
					return err
				}
			case *Block:
				if err := walkBlock(st); err != nil {
					return err
				}
			case *IfStmt:
				if err := walkBlock(st.Then); err != nil {
					return err
				}
				for idx := range st.ElseIfBodies {
					if err := walkBlock(st.ElseIfBodies[idx]); err != nil {
						return err
					}
				}
				if err := walkBlock(st.Else); err != nil {
					return err
				}
			case *WhileStmt:
				if err := walkBlock(st.Body); err != nil {
					return err
				}
			case *RepeatStmt:
				if err := walkBlock(st.Body); err != nil {
					return err
				}
			case *ForStmt:
				types = append(types, map[string]*Type{})
				if d, ok := st.Init.(*DefineStmt); ok {
					if t := namedTypeOf(&d.VarType); t != nil {
						types[len(types)-1][d.VarName] = t
					}
				}
				err := walkBlock(st.Body)
				types = types[:len(types)-1]
				if err != nil {
					return err
				}
			case *ForEachStmt:
				if err := walkBlock(st.Body); err != nil {
					return err
				}
			case *TryStmt:
				if err := walkBlock(st.Body); err != nil {
					return err
				}
				if err := walkBlock(st.CatchBody); err != nil {
					return err
				}
			case *ExprStmt:
				if err := walkExpr(st.Expr); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walkStmts(body)
}

// allArmsVariantShaped reports whether every arm of a match is a single
// variant-shaped head. A match that mixes in a value arm stays a value match,
// so deferring can never reinterpret one.
func allArmsVariantShaped(arms []MatchArm) bool {
	for i := range arms {
		if len(arms[i].Values) != 1 || !variantShapedArm(arms[i].Values[0]) {
			return false
		}
	}
	return true
}

// variantShapedArm reports whether a `when` head is unambiguously an enum
// variant pattern rather than a value to compare against: `Variant(bind)`, or a
// bare `Variant` whose name is not spelled like a constant. The constant
// spelling is what keeps `when SOME_CONST` a value arm, so deferring a match
// cannot silently reinterpret an existing value comparison.
func variantShapedArm(e Expr) bool {
	switch ex := e.(type) {
	case *ConstRefExpr:
		return !isValidConstName(ex.Name)
	case *CallExpr:
		if len(ex.Args) != 1 || isValidConstName(ex.Callee) {
			return false
		}
		_, isName := ex.Args[0].(*ConstRefExpr)
		return isName
	}
	return false
}

// patternExpr interprets a `when` arm head as an enum-variant pattern. It
// accepts `Variant` (payload-less, a ConstRefExpr) and `Variant(bind)` (a
// CallExpr with one bare-name argument), returning the variant name and binder
// ("" if none). ok is false when the head is not pattern-shaped; a non-nil
// error is a positioned misuse (e.g. a `$bind` sigil where a bare name belongs).
func (r *resolver) patternExpr(e Expr) (variant, bind string, ok bool, err error) {
	switch ex := e.(type) {
	case *ConstRefExpr:
		return ex.Name, "", true, nil
	case *CallExpr:
		if len(ex.Args) == 0 {
			// `Variant()` - no binder; write the bare `Variant` form.
			file, line, col := posFor(ex)
			return "", "", false, &ParseError{Msg: fmt.Sprintf("payload-less variant pattern is written `when %s`, without `()`", ex.Callee), File: file, Line: line, Col: col}
		}
		if len(ex.Args) != 1 {
			file, line, col := posFor(ex)
			return "", "", false, &ParseError{Msg: fmt.Sprintf("variant pattern `%s(...)` binds exactly one payload name", ex.Callee), File: file, Line: line, Col: col}
		}
		bindRef, isName := ex.Args[0].(*ConstRefExpr)
		if !isName {
			file, line, col := posFor(ex.Args[0])
			return "", "", false, &ParseError{Msg: "a variant pattern binder is a bare name (write `when Circle(c)`, not `$c`)", File: file, Line: line, Col: col}
		}
		return ex.Callee, bindRef.Name, true, nil
	}
	return "", "", false, nil
}

// resolveEnumMatch resolves a pattern match over an enum subject: each arm head
// must be a distinct variant of ed, a binder gets its own arm-frame slot, and
// the arm set must be exhaustive unless an `else` is present.
func (r *resolver) resolveEnumMatch(st *MatchStmt, ed *EnumDef) error {
	variantByName := make(map[string]*EnumVariant, len(ed.Variants))
	for i := range ed.Variants {
		variantByName[ed.Variants[i].Name] = &ed.Variants[i]
	}
	covered := make(map[string]bool, len(ed.Variants))
	for ai := range st.Arms {
		arm := &st.Arms[ai]
		afile, aline, acol := posFor(arm)
		if len(arm.Values) != 1 {
			return &ParseError{Msg: fmt.Sprintf("a `match` arm on enum %s matches one variant pattern (no comma-separated value list)", ed.Name), File: afile, Line: aline, Col: acol}
		}
		variant, bind, ok, perr := r.patternExpr(arm.Values[0])
		if perr != nil {
			return perr
		}
		if !ok {
			return &ParseError{Msg: fmt.Sprintf("`match` on enum %s expects variant patterns (`when Circle(c)` or `when Empty`)", ed.Name), File: afile, Line: aline, Col: acol}
		}
		vdef, isVariant := variantByName[variant]
		if !isVariant {
			return &ParseError{Msg: fmt.Sprintf("%q is not a variant of enum %s", variant, ed.Name), File: afile, Line: aline, Col: acol}
		}
		if covered[variant] {
			return &ParseError{Msg: fmt.Sprintf("variant %s is covered more than once in this `match`", variant), File: afile, Line: aline, Col: acol}
		}
		covered[variant] = true
		if bind != "" && len(vdef.Fields) == 0 {
			return &ParseError{Msg: fmt.Sprintf("variant %s has no payload to bind (write `when %s`)", variant, variant), File: afile, Line: aline, Col: acol}
		}
		arm.Variant = variant
		arm.Bind = bind
		// Values is left in place, not cleared: the runtime reads Variant first
		// (an arm with Variant set never consults Values), and keeping the
		// original head lets a second Resolve re-derive the same (Variant, Bind)
		// - so Resolve stays idempotent even for a pattern match.
		// The arm body runs in a fresh frame; the binder (if any) is slot 0.
		frame := &scopeFrame{slots: map[string]slotInfo{}}
		r.push(frame)
		arm.BindSlot = -1
		if bind != "" {
			slot, derr := r.define(bind, false, nil, afile, aline, acol)
			if derr != nil {
				r.pop()
				return derr
			}
			arm.BindSlot = slot
		}
		for _, s := range arm.Body.Stmts {
			if err := r.resolveStmt(s); err != nil {
				r.pop()
				return err
			}
		}
		arm.Body.NumSlots = frame.count
		r.pop()
	}
	if st.Else != nil {
		return r.resolveBlock(st.Else)
	}
	var missing []string
	for i := range ed.Variants {
		if !covered[ed.Variants[i].Name] {
			missing = append(missing, ed.Variants[i].Name)
		}
	}
	if len(missing) > 0 {
		file, line, col := posFor(st)
		return &ParseError{Msg: fmt.Sprintf("`match` on enum %s is not exhaustive: missing %s (cover every variant or add an `else`)", ed.Name, joinNames(missing)), File: file, Line: line, Col: col}
	}
	return nil
}

// resolveDeferredEnumMatch handles a `match` whose subject is a module / library
// type this file cannot see the declaration of. The arms are rewritten into
// variant patterns exactly as for a local enum - so a binder gets its slot and
// the arm body can read `$c` - but nothing is validated here: the variant names,
// duplicate coverage, exhaustiveness, and even whether the type is an enum at
// all are checked by the interpreter once the module is loaded, driven by
// Program.PendingEnumMatches. Only called when every arm is variant-shaped.
func (r *resolver) resolveDeferredEnumMatch(st *MatchStmt, t *Type) error {
	for ai := range st.Arms {
		arm := &st.Arms[ai]
		afile, aline, acol := posFor(arm)
		if len(arm.Values) != 1 {
			return &ParseError{Msg: fmt.Sprintf("a `match` arm on %s matches one variant pattern (no comma-separated value list)", t.String()), File: afile, Line: aline, Col: acol}
		}
		variant, bind, ok, perr := r.patternExpr(arm.Values[0])
		if perr != nil {
			return perr
		}
		if !ok {
			return &ParseError{Msg: fmt.Sprintf("`match` on %s expects variant patterns (`when Circle(c)` or `when Empty`)", t.String()), File: afile, Line: aline, Col: acol}
		}
		arm.Variant = variant
		arm.Bind = bind
		// Values kept (not cleared) for idempotency - see resolveEnumMatch.
		frame := &scopeFrame{slots: map[string]slotInfo{}}
		r.push(frame)
		arm.BindSlot = -1
		if bind != "" {
			slot, derr := r.define(bind, false, nil, afile, aline, acol)
			if derr != nil {
				r.pop()
				return derr
			}
			arm.BindSlot = slot
		}
		for _, s := range arm.Body.Stmts {
			if err := r.resolveStmt(s); err != nil {
				r.pop()
				return err
			}
		}
		arm.Body.NumSlots = frame.count
		r.pop()
	}
	st.EnumType = t
	if r.prog != nil {
		r.prog.PendingEnumMatches = append(r.prog.PendingEnumMatches, st)
	}
	if st.Else != nil {
		return r.resolveBlock(st.Else)
	}
	return nil
}

// checkEnumMatchArms validates an already-tagged pattern match against its enum
// without touching slots: every arm names a real variant, none is covered twice,
// a binder only appears where there is a payload, and the arms are exhaustive
// unless an `else` is present. Used for spawn bodies, which are tagged but not
// slot-resolved; the batch path interleaves the same checks with slot
// allocation in resolveEnumMatch.
func checkEnumMatchArms(st *MatchStmt, ed *EnumDef, typeName string) error {
	variantByName := make(map[string]*EnumVariant, len(ed.Variants))
	for vi := range ed.Variants {
		variantByName[ed.Variants[vi].Name] = &ed.Variants[vi]
	}
	covered := make(map[string]bool, len(ed.Variants))
	for ai := range st.Arms {
		arm := &st.Arms[ai]
		file, line, col := posFor(arm)
		vdef, isVariant := variantByName[arm.Variant]
		if !isVariant {
			return &ParseError{Msg: fmt.Sprintf("%q is not a variant of enum %s", arm.Variant, typeName), File: file, Line: line, Col: col}
		}
		if covered[arm.Variant] {
			return &ParseError{Msg: fmt.Sprintf("variant %s is covered more than once in this `match`", arm.Variant), File: file, Line: line, Col: col}
		}
		covered[arm.Variant] = true
		if arm.Bind != "" && len(vdef.Fields) == 0 {
			return &ParseError{Msg: fmt.Sprintf("variant %s has no payload to bind (write `when %s`)", arm.Variant, arm.Variant), File: file, Line: line, Col: col}
		}
	}
	if st.Else != nil {
		return nil
	}
	var missing []string
	for vi := range ed.Variants {
		if !covered[ed.Variants[vi].Name] {
			missing = append(missing, ed.Variants[vi].Name)
		}
	}
	if len(missing) > 0 {
		file, line, col := posFor(st)
		return &ParseError{Msg: fmt.Sprintf("`match` on enum %s is not exhaustive: missing %s (cover every variant or add an `else`)", typeName, joinNames(missing)), File: file, Line: line, Col: col}
	}
	return nil
}

// joinNames renders a variant-name list for an exhaustiveness error.
func joinNames(names []string) string {
	out := ""
	for i, n := range names {
		if i > 0 {
			out += ", "
		}
		out += n
	}
	return out
}

func (r *resolver) resolveStmt(s Stmt) error {
	switch st := s.(type) {
	case *DefineStmt:
		return r.resolveDefine(st)
	case *AssignStmt:
		return r.resolveAssign(st)
	case *IndexAssignStmt:
		if err := r.resolveTarget(st.Target); err != nil {
			return err
		}
		return r.resolveExpr(st.Value)
	case *AppendStmt:
		if err := r.resolveTarget(st.Target); err != nil {
			return err
		}
		return r.resolveExpr(st.Value)
	case *FieldAssignStmt:
		if err := r.resolveTarget(st.Target); err != nil {
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
	case *MatchStmt:
		if err := r.resolveExpr(st.Subject); err != nil {
			return err
		}
		// If the subject is a variable / parameter / constant of a locally
		// declared enum type, this is a pattern match: arm heads are bare variant
		// patterns, resolved against the enum, and coverage is checked for
		// exhaustiveness right here.
		subjTyp := r.subjectType(st.Subject)
		if ed := r.localEnumOf(subjTyp); ed != nil {
			return r.resolveEnumMatch(st, ed)
		}
		// The subject names a module / library type whose declaration this file
		// cannot see. When every arm is unambiguously variant-shaped, rewrite them
		// as patterns (so a binder gets a slot and the body can read it) and defer
		// validation - variant names, duplicates, exhaustiveness, and whether the
		// type is an enum at all - to the interpreter, after the module loads.
		if deferrableEnumType(subjTyp) && len(st.Arms) > 0 && allArmsVariantShaped(st.Arms) {
			return r.resolveDeferredEnumMatch(st, subjTyp)
		}
		// The subject is not a statically-known named type (a method result, say).
		// Resolve arms as value arms; a bare variant-name head resolves name-based
		// and the runtime match fallback recognizes it as a pattern against the
		// actual enum value (so `match (mk())` works, just without resolve-time
		// exhaustiveness).
		for ai := range st.Arms {
			// A `when Variant(bind)` head over a non-statically-enum subject is an
			// unambiguous destructuring intent the resolver cannot serve (the
			// binder needs the subject's type). Error clearly here rather than
			// letting the body's `$bind` reference fail as "undefined variable".
			if len(st.Arms[ai].Values) == 1 {
				if err := r.rejectBinderPatternOnUntypedSubject(st.Arms[ai].Values[0]); err != nil {
					return err
				}
			}
			for _, v := range st.Arms[ai].Values {
				if err := r.resolveExpr(v); err != nil {
					return err
				}
			}
			st.Arms[ai].BindSlot = -1
			if err := r.resolveBlock(st.Arms[ai].Body); err != nil {
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
		// The header's Init `def`s live in `frame` (the loop's own header
		// frame at runtime); the body block's defs live in the body frame.
		// Record the header count separately so the interpreter pre-sizes the
		// header frame and leaves Body.NumSlots as just the body's own slots -
		// otherwise every iteration's body frame is oversized by the header
		// count.
		st.HeaderSlots = frame.count
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
	case *DeferStmt:
		// The deferred call's arguments are evaluated at the defer site, so its
		// callee + args resolve exactly like a normal call expression here.
		return r.resolveExpr(st.Call)
	case *TryStmt:
		// The try body is its own block scope (its own runtime frame at
		// execTry). Its `def`s do not leak into the enclosing frame, so a
		// `def` skipped by a throw is out of scope afterward - a later
		// reference is an undefined-variable error, not a zeroed null read.
		bodyFrame := &scopeFrame{slots: map[string]slotInfo{}}
		r.push(bodyFrame)
		for _, bs := range st.Body.Stmts {
			if err := r.resolveStmt(bs); err != nil {
				r.pop()
				return err
			}
		}
		st.Body.NumSlots = bodyFrame.count
		r.pop()
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
	slot, err := r.define(st.VarName, st.IsConst, namedTypeOf(&st.VarType), file, line, col)
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
		depth, slot, isConst, ok := r.lookup(ex.Name)
		if !ok {
			file, line, col := posFor(ex)
			return &ParseError{
				Msg:  fmt.Sprintf("undefined variable %q", ex.Name),
				File: file, Line: line, Col: col,
			}
		}
		// `$NAME` where NAME is a constant: the `$` sigil is for mutable
		// variables only. Constants are referenced bare.
		if isConst {
			file, line, col := posFor(ex)
			return &ParseError{
				Msg:  fmt.Sprintf("constant %q is referenced with `$`; drop the sigil (constants are referenced bare)", ex.Name),
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
		// Pre-resolve the callee to a method pointer when
		// it names a hoisted top-level user method. Builtins stay
		// nil - the interpreter dispatches those through the
		// namespaced / global registries which check `use`
		// activation state at runtime.
		if m, ok := r.methods[ex.Callee]; ok {
			ex.Method = m
		}
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
		// Spawn bodies are deliberately left unresolved.
		// The runtime's snapshotForSpawn produces a two-frame
		// duplex (globals-snap + locals-snap) that doesn't line up
		// with the resolver's single-frame view of "the enclosing
		// scope," and inventing depth arithmetic to reconcile the
		// two would be brittle. Spawn bodies fall back to
		// name-based lookup at runtime - not hot-loop territory,
		// so the perf regression is limited to coarse-grained
		// concurrency dispatch.
		//
		// Skipping *slot* analysis must not skip semantic analysis: an enum
		// `match` inside a spawn still has to be exhaustive. checkSpawnEnums
		// walks the body types-only - no slots, no reference rewrites - and
		// applies the same enum checks the batch path applies.
		return r.checkSpawnEnums(ex.Body)
	case *IndexExpr:
		if err := r.resolveExpr(ex.Target); err != nil {
			return err
		}
		return r.resolveExpr(ex.Index)
	case *RangeExpr:
		if err := r.resolveExpr(ex.Lo); err != nil {
			return err
		}
		return r.resolveExpr(ex.Hi)
	case *SliceExpr:
		if err := r.resolveExpr(ex.Target); err != nil {
			return err
		}
		if ex.Lo != nil {
			if err := r.resolveExpr(ex.Lo); err != nil {
				return err
			}
		}
		if ex.Hi != nil {
			return r.resolveExpr(ex.Hi)
		}
		return nil
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
		if err := r.resolveExpr(ex.Operand); err != nil {
			return err
		}
		// Attempt constant folding once the operand is
		// resolved. tryFoldUnary returns nil when the operand isn't
		// a compile-time literal.
		ex.Folded = tryFoldUnary(ex)
		return nil
	case *BinaryExpr:
		if err := r.resolveExpr(ex.Left); err != nil {
			return err
		}
		if err := r.resolveExpr(ex.Right); err != nil {
			return err
		}
		// Same fold pass as UnaryExpr.
		ex.Folded = tryFoldBinary(ex)
		return nil
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
