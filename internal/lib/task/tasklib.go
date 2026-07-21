// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package tasklib implements Jennifer's `task` library:
// wait, poll, discard, waitAll, waitAny - the five user-facing
// operations on `task of T` values produced by `spawn { ... }` blocks.
//
// `task.wait` blocks on the underlying done channel and returns the
// stored result (or re-raises the stored error so `try`/`catch` can
// catch it). `task.discard` marks the task observed so the exit-time
// loud-fail skips it - the explicit fire-and-forget escape hatch.
// `waitAll` and `waitAny` cover the two most common multi-task
// patterns (parallel map-and-collect, first-to-finish).
//
// The Go package is named tasklib so it doesn't collide with the
// `task` type keyword if anyone imports the package by short name.
package tasklib

import (
	"fmt"
	"reflect"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// LibraryName is the namespace prefix and `use` name.
const LibraryName = "task"

// Install registers the five task builtins.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespaced(LibraryName, "wait", makeWait(in))
	in.RegisterNamespaced(LibraryName, "poll", pollFn)
	in.RegisterNamespaced(LibraryName, "discard", makeDiscard(in))
	in.RegisterNamespaced(LibraryName, "waitAll", makeWaitAll(in))
	in.RegisterNamespaced(LibraryName, "waitAny", waitAnyFn)
}

// extractTask pulls the *TaskState out of a KindTask Value. Errors
// out at the call boundary if the argument isn't a task.
func extractTask(fnName string, v interpreter.Value) (*interpreter.TaskState, error) {
	if v.Kind != interpreter.KindTask {
		return nil, fmt.Errorf("%s: argument must be a task, got %s", fnName, v.Kind)
	}
	if v.Task == nil {
		return nil, fmt.Errorf("%s: task has no state (internal error)", fnName)
	}
	return v.Task, nil
}

// makeWait closes over the interpreter so MarkObserved can flip the
// observed bit after a successful wait or before re-raising the
// task's stored error. The closure pattern parallels what `time`
// does for its clock hook and `oslib` does for its process state.
func makeWait(in *interpreter.Interpreter) interpreter.Builtin {
	return func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		if len(args) != 1 {
			return interpreter.Null(), fmt.Errorf("task.wait expects 1 argument (task), got %d", len(args))
		}
		state, err := extractTask("task.wait", args[0])
		if err != nil {
			return interpreter.Null(), err
		}
		<-state.Done
		// Mark observed in both branches - the spec says wait counts
		// as an observation whether it returns the value or re-raises
		// the error, because the parent saw the outcome either way.
		in.MarkObserved(state)
		if state.Err != nil {
			return interpreter.Null(), state.Err
		}
		return state.Result, nil
	}
}

// pollFn is the non-blocking completion check. Marks observed only
// implicitly via wait/discard later; a true poll result is read-only.
func pollFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("task.poll expects 1 argument (task), got %d", len(args))
	}
	state, err := extractTask("task.poll", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.BoolVal(state.IsDone()), nil
}

// makeDiscard turns a task into fire-and-forget: marks it observed so
// the exit-time loud-fail scan skips it. Doesn't block on completion;
// the spawned goroutine runs to its own end.
func makeDiscard(in *interpreter.Interpreter) interpreter.Builtin {
	return func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		if len(args) != 1 {
			return interpreter.Null(), fmt.Errorf("task.discard expects 1 argument (task), got %d", len(args))
		}
		state, err := extractTask("task.discard", args[0])
		if err != nil {
			return interpreter.Null(), err
		}
		in.MarkObserved(state)
		return interpreter.Null(), nil
	}
}

// extractTaskList walks a list-of-task argument, validating each
// element is a KindTask. Returns the slice of *TaskState plus the
// element type carried on the input list (the `T` in `list of task
// of T`) so waitAll can stamp the right type on its output list.
func extractTaskList(fnName string, v interpreter.Value) ([]*interpreter.TaskState, *parser.Type, error) {
	if v.Kind != interpreter.KindList {
		return nil, nil, fmt.Errorf("%s: argument must be a list of task, got %s", fnName, v.Kind)
	}
	out := make([]*interpreter.TaskState, len(v.List))
	for i, e := range v.List {
		state, err := extractTask(fmt.Sprintf("%s: element %d", fnName, i), e)
		if err != nil {
			return nil, nil, err
		}
		out[i] = state
	}
	// The input's declared element type is `task of T`; pull T out
	// so waitAll's returned list carries the right type.
	var innerT *parser.Type
	if v.ElemTyp != nil && v.ElemTyp.Kind == parser.TypeTask && v.ElemTyp.Element != nil {
		innerT = v.ElemTyp.Element
	}
	return out, innerT, nil
}

// makeWaitAll waits for every task in the list, marks each observed,
// and returns a list of results. The first error encountered (in list
// order) is re-raised after every other task has been drained, so the
// exit-time loud-fail doesn't fire on the survivors.
func makeWaitAll(in *interpreter.Interpreter) interpreter.Builtin {
	return func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		if len(args) != 1 {
			return interpreter.Null(), fmt.Errorf("task.waitAll expects 1 argument (list of task), got %d", len(args))
		}
		states, innerT, err := extractTaskList("task.waitAll", args[0])
		if err != nil {
			return interpreter.Null(), err
		}
		results := make([]interpreter.Value, 0, len(states))
		var firstErr error
		for _, state := range states {
			<-state.Done
			in.MarkObserved(state)
			if state.Err != nil && firstErr == nil {
				firstErr = state.Err
			}
			if state.Err == nil {
				results = append(results, state.Result)
			}
		}
		if firstErr != nil {
			return interpreter.Null(), firstErr
		}
		// When the task list carries a recorded element type (`list of
		// task of T`), stamp the results as `list of T`. When it does not
		// (a bare list-literal argument, whose ElemTyp is nil), return a
		// GENERIC list - not `list of int` - so the binding site validates
		// each result against the caller's declared type instead of
		// relabeling strings (etc.) as ints via the recorded-type fast path.
		if innerT != nil {
			return interpreter.ListVal(*innerT, results), nil
		}
		return interpreter.Value{Kind: interpreter.KindList, List: results}, nil
	}
}

// waitAnyFn blocks until any task in the list completes, returning
// its zero-based index. The caller is expected to follow up with
// task.wait on the returned index to observe the result; this builtin
// itself does NOT mark observed (the user picks which task to
// observe based on the returned index, the others continue and may
// also be waited on or hit the exit-time loud-fail).
func waitAnyFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("task.waitAny expects 1 argument (list of task), got %d", len(args))
	}
	states, _, err := extractTaskList("task.waitAny", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if len(states) == 0 {
		return interpreter.Null(), fmt.Errorf("task.waitAny: list is empty (no tasks to wait on)")
	}
	// reflect.Select over each task's Done channel. The chosen index
	// is the position in the input list.
	cases := make([]reflect.SelectCase, len(states))
	for i, s := range states {
		cases[i] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(s.Done)}
	}
	chosen, _, _ := reflect.Select(cases)
	return interpreter.IntVal(int64(chosen)), nil
}
