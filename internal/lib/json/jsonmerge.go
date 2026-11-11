// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package jsonlib

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lib/doctree"
)

// mergePatchFn is json.mergePatch(target, patch): RFC 7386 JSON Merge Patch.
// Either argument may be a json.Value or any value json.set accepts (a
// Jennifer map, list, struct, scalar), so a patch can be written inline.
func mergePatchFn(args []interpreter.Value) (interpreter.Value, error) {
	const verb = "json.mergePatch"
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("%s expects 2 arguments (target, patch), got %d", verb, len(args))
	}
	target, err := lib.ToNode(args[0], false)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("%s: target: %v", verb, err)
	}
	patch, err := lib.ToNode(args[1], false)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("%s: patch: %v", verb, err)
	}
	out := mergePatch(target, patch)
	if err := doctree.CheckDepth(verb, 0, out); err != nil {
		return interpreter.Null(), err
	}
	if err := doctree.CheckSize(verb, out); err != nil {
		return interpreter.Null(), err
	}
	return wrap(out), nil
}

// mergePatch applies patch to target as RFC 7386 section 2 defines it:
//
//   - a patch that is not an object replaces the target outright (an array
//     is replaced whole, never merged element by element);
//   - an object patch merges into the target, which becomes an empty object
//     first when it is not one; a member whose patch value is null is
//     removed, any other member is merged recursively.
//
// A null in a patch therefore always means "remove": a merge patch cannot set
// a member to null (RFC 7386 section 1), use json.set for that. Only the
// objects the patch reaches are copied; everything else is shared. The
// recursion follows the patch, whose nesting is already bounded.
func mergePatch(target, patch node) node {
	if patch.Kind() != doctree.KindMap {
		return patch
	}
	var keys []string
	var elems []node
	if target.Kind() == doctree.KindMap {
		keys = append(keys, target.Keys()...)
		elems = append(elems, target.Elems()...)
	} else {
		target = doctree.Map(nil, nil)
	}
	n := len(keys)
	var drop []bool
	for i := 0; i < patch.Len(); i++ {
		k, v := patch.Key(i), patch.Elem(i)
		pos := target.Find(k)
		if v.Kind() == doctree.KindNull {
			if pos >= 0 {
				if drop == nil {
					drop = make([]bool, n)
				}
				drop[pos] = true
			}
			continue
		}
		if pos >= 0 {
			elems[pos] = mergePatch(elems[pos], v)
			continue
		}
		// A new member: merging into "absent" strips the patch's nulls.
		keys = append(keys, k)
		elems = append(elems, mergePatch(doctree.Null(), v))
	}
	if drop != nil {
		w := 0
		for i := range keys {
			if i < n && drop[i] {
				continue
			}
			keys[w], elems[w] = keys[i], elems[i]
			w++
		}
		keys, elems = keys[:w], elems[:w]
	}
	return doctree.Map(keys, elems)
}
