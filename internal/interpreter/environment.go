// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package interpreter

import (
	"fmt"

	"github.com/mplx/jennifer-lang/internal/parser"
)

// Binding is one entry in an Environment frame: the current value plus the
// declared static type and whether it's a constant.
type Binding struct {
	Value    Value
	DeclType parser.Type
	IsConst  bool
}

// Environment is a lexically-scoped symbol table.
// Parent chains form the scope stack; Define adds to the current frame only.
// Lookups walk outward. The spec forbids shadowing, so Define returns an error
// if any visible parent already binds the name.
type Environment struct {
	parent *Environment
	vars   map[string]Binding
}

func NewEnvironment(parent *Environment) *Environment {
	return &Environment{
		parent: parent,
		vars:   make(map[string]Binding),
	}
}

// Define introduces a new binding in the current frame.
// Returns an error if the name already exists in this frame or any enclosing
// scope (spec: lower scopes may not overwrite existing bindings).
func (e *Environment) Define(name string, val Value, declType parser.Type, isConst bool) error {
	if e.existsInChain(name) {
		return fmt.Errorf("name %q is already defined in an enclosing scope", name)
	}
	e.vars[name] = Binding{Value: val, DeclType: declType, IsConst: isConst}
	return nil
}

// Assign updates an existing binding, walking up the parent chain to find it.
// Errors if the name is undefined, refers to a constant, or the new value's
// kind doesn't match the declared type.
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
