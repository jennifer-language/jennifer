// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package interpreter

import (
	"fmt"

	"github.com/mplx/jennifer-lang/internal/parser"
)

// Binding is one entry in an Environment frame: the current value plus the
// declared static type and whether it's a constant. M16.5.2 adds Slot
// so name-based writes (Assign, execAppend / execIndexAssign /
// execFieldAssign via GetBinding + Assign) can mirror into the
// slot-indexed storage after finding the binding by name. -1 means
// "no slot mirror" (resolver-less path, REPL, tests).
type Binding struct {
	Value    Value
	DeclType parser.Type
	IsConst  bool
	Slot     int
}

// Environment is a lexically-scoped symbol table.
// Parent chains form the scope stack; Define adds to the current frame only.
// Lookups walk outward. The spec forbids shadowing, so Define returns an error
// if any visible parent already binds the name.
//
// M16.5.2: after Resolve() runs on the AST, every variable reference
// carries a (Depth, Slot) coordinate the runtime can use to skip the
// name-map walk. The slot-indexed storage lives in `slots` alongside
// the name map; DefineAt / GetAt / AssignAt operate on it. The name
// map remains as the fallback path for the REPL (which spans multiple
// resolver-less parses) and for any test that hand-builds AST
// fragments without slot annotations.
type Environment struct {
	parent *Environment
	vars   map[string]Binding
	slots  []Binding
}

func NewEnvironment(parent *Environment) *Environment {
	return &Environment{
		parent: parent,
		vars:   make(map[string]Binding),
	}
}

// NewEnvironmentSized creates an environment pre-sized to hold
// numSlots slot-indexed bindings, plus the fallback name map. Called
// from execBlock when Block.NumSlots is nonzero (i.e., the resolver
// pre-computed the slot count).
func NewEnvironmentSized(parent *Environment, numSlots int) *Environment {
	env := &Environment{
		parent: parent,
		vars:   make(map[string]Binding),
	}
	if numSlots > 0 {
		env.slots = make([]Binding, numSlots)
	}
	return env
}

// DefineAt records a binding at the given slot index in the current
// frame. Also mirrors into the name map so name-based lookups (REPL,
// GetBinding by name, tests) keep working. Returns the same no-shadow
// error as Define when the name collides with an enclosing binding,
// though for resolver-generated slot writes that check has already
// happened at parse time.
func (e *Environment) DefineAt(slot int, name string, val Value, declType parser.Type, isConst bool) error {
	if e.existsInChain(name) {
		return fmt.Errorf("name %q is already defined in an enclosing scope", name)
	}
	b := Binding{Value: val, DeclType: declType, IsConst: isConst, Slot: slot}
	e.vars[name] = b
	if slot >= 0 {
		if slot >= len(e.slots) {
			// Grow to fit. NumSlots hint from the resolver should
			// have made this unnecessary, but the safety net keeps
			// resolver-less code paths (REPL, some tests) working.
			grown := make([]Binding, slot+1)
			copy(grown, e.slots)
			e.slots = grown
		}
		e.slots[slot] = b
	}
	return nil
}

// GetAt reads a binding by (depth, slot). Walks depth parent pointers
// then indexes into slots. Returns the fallback name-based Get error
// on a bad address so the runtime error text stays uniform.
func (e *Environment) GetAt(depth, slot int, name string) (Value, error) {
	cur := e
	for d := 0; d < depth; d++ {
		if cur.parent == nil {
			return Value{}, fmt.Errorf("undefined variable %q", name)
		}
		cur = cur.parent
	}
	if slot < 0 || slot >= len(cur.slots) {
		// Slot outside range: fall back to name lookup at this depth
		// (covers method-body defs added at runtime by test paths).
		if b, ok := cur.vars[name]; ok {
			return b.Value, nil
		}
		return Value{}, fmt.Errorf("undefined variable %q", name)
	}
	return cur.slots[slot].Value, nil
}

// GetBindingAt is the metadata companion to GetAt.
func (e *Environment) GetBindingAt(depth, slot int, name string) (Binding, error) {
	cur := e
	for d := 0; d < depth; d++ {
		if cur.parent == nil {
			return Binding{}, fmt.Errorf("undefined %q", name)
		}
		cur = cur.parent
	}
	if slot < 0 || slot >= len(cur.slots) {
		if b, ok := cur.vars[name]; ok {
			return b, nil
		}
		return Binding{}, fmt.Errorf("undefined %q", name)
	}
	return cur.slots[slot], nil
}

// AssignAt writes a new value to the binding at (depth, slot). Const
// and type-mismatch checks match the name-based Assign path.
func (e *Environment) AssignAt(depth, slot int, name string, val Value) error {
	cur := e
	for d := 0; d < depth; d++ {
		if cur.parent == nil {
			return fmt.Errorf("undefined variable %q", name)
		}
		cur = cur.parent
	}
	if slot < 0 || slot >= len(cur.slots) {
		// Fall back to the name path.
		return e.Assign(name, val)
	}
	b := cur.slots[slot]
	if b.IsConst {
		return fmt.Errorf("cannot assign to constant %q", name)
	}
	if !val.MatchesDeclared(b.DeclType) {
		return fmt.Errorf("cannot assign %s to %s variable %q", val.Kind, b.DeclType, name)
	}
	b.Value = val
	cur.slots[slot] = b
	// Mirror into the name map so name-based reads see the update.
	cur.vars[name] = b
	return nil
}

// Define introduces a new binding in the current frame.
// Returns an error if the name already exists in this frame or any enclosing
// scope (spec: lower scopes may not overwrite existing bindings).
func (e *Environment) Define(name string, val Value, declType parser.Type, isConst bool) error {
	if e.existsInChain(name) {
		return fmt.Errorf("name %q is already defined in an enclosing scope", name)
	}
	e.vars[name] = Binding{Value: val, DeclType: declType, IsConst: isConst, Slot: -1}
	return nil
}

// Assign updates an existing binding, walking up the parent chain to find it.
// Errors if the name is undefined, refers to a constant, or the new value's
// kind doesn't match the declared type. M16.5.2: when the binding was
// installed via DefineAt (Slot >= 0), mirror the write into
// cur.slots[Slot] so a subsequent GetAt sees the update.
func (e *Environment) Assign(name string, val Value) error {
	for cur := e; cur != nil; cur = cur.parent {
		if b, ok := cur.vars[name]; ok {
			if b.IsConst {
				return fmt.Errorf("cannot assign to constant %q", name)
			}
			if !val.MatchesDeclared(b.DeclType) {
				return fmt.Errorf("cannot assign %s to %s variable %q", val.Kind, b.DeclType, name)
			}
			b.Value = val
			cur.vars[name] = b
			if b.Slot >= 0 && b.Slot < len(cur.slots) {
				cur.slots[b.Slot] = b
			}
			return nil
		}
	}
	return fmt.Errorf("undefined variable %q", name)
}

// Get looks up a name, walking outward.
func (e *Environment) Get(name string) (Value, error) {
	for cur := e; cur != nil; cur = cur.parent {
		if b, ok := cur.vars[name]; ok {
			return b.Value, nil
		}
	}
	return Value{}, fmt.Errorf("undefined variable %q", name)
}

// GetBinding looks up a binding (value + metadata) by name, walking outward.
// Used by callers that need to distinguish constants from variables.
func (e *Environment) GetBinding(name string) (Binding, error) {
	for cur := e; cur != nil; cur = cur.parent {
		if b, ok := cur.vars[name]; ok {
			return b, nil
		}
	}
	return Binding{}, fmt.Errorf("undefined %q", name)
}

func (e *Environment) existsInChain(name string) bool {
	for cur := e; cur != nil; cur = cur.parent {
		if _, ok := cur.vars[name]; ok {
			return true
		}
	}
	return false
}
