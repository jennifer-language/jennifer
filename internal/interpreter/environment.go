// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package interpreter

import "fmt"

// Environment is a lexically-scoped symbol table.
// Parent chains form the scope stack; Define adds to the current frame only.
// Lookups walk outward. The spec forbids shadowing, so Define returns an error
// if any visible parent already binds the name.
type Environment struct {
	parent *Environment
	vars   map[string]Value
	consts map[string]bool // names in this frame that are constants
}

func NewEnvironment(parent *Environment) *Environment {
	return &Environment{
		parent: parent,
		vars:   make(map[string]Value),
		consts: make(map[string]bool),
	}
}

// Define introduces a new binding in the current frame.
// Returns an error if the name already exists in this frame or any enclosing scope
// (spec: lower scopes may not overwrite existing bindings).
func (e *Environment) Define(name string, val Value, isConst bool) error {
	if e.existsInChain(name) {
		return fmt.Errorf("name %q is already defined in an enclosing scope", name)
	}
	e.vars[name] = val
	if isConst {
		e.consts[name] = true
	}
	return nil
}

// Assign updates an existing binding, walking up the parent chain to find it.
// Errors if the name is undefined or refers to a constant.
func (e *Environment) Assign(name string, val Value) error {
	for cur := e; cur != nil; cur = cur.parent {
		if _, ok := cur.vars[name]; ok {
			if cur.consts[name] {
				return fmt.Errorf("cannot assign to constant %q", name)
			}
			cur.vars[name] = val
			return nil
		}
	}
	return fmt.Errorf("undefined variable %q", name)
}

// Get looks up a name, walking outward.
func (e *Environment) Get(name string) (Value, error) {
	for cur := e; cur != nil; cur = cur.parent {
		if v, ok := cur.vars[name]; ok {
			return v, nil
		}
	}
	return Value{}, fmt.Errorf("undefined variable %q", name)
}

func (e *Environment) existsInChain(name string) bool {
	for cur := e; cur != nil; cur = cur.parent {
		if _, ok := cur.vars[name]; ok {
			return true
		}
	}
	return false
}
