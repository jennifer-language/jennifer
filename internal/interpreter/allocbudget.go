// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package interpreter

import (
	"fmt"
	"math"
	"sync/atomic"
)

// AllocBudget is a task's allocation budget (task.setAllocBudget): a counter of
// the bytes the task has allocated as values grew, and the limit past which the
// next charged operation raises a catchable Error{kind: "limit"}. Go cannot
// measure one goroutine's live memory, so the count is cumulative: it grows with
// every eager copy, append, growing index write, field write, string
// concatenation, literal and builtin result, and never shrinks.
//
// A budget hangs off the goroutine's CallChain, so it follows the chain into
// another module's functions and func values, and costs a goroutine without one
// a single nil check per charged operation. A task spawned by a task with a
// budget shares the same *AllocBudget (a request's helper tasks count toward the
// request), so the fields are atomic. A task that sets a budget while it runs
// under an inherited one gets a budget of its own nested in it: its charges
// count toward both, and either limit raises (a per-request budget under a
// server that has one).
type AllocBudget struct {
	// parent is the budget this one is nested in (nil for an outermost one);
	// every charge also counts there.
	parent *AllocBudget
	// owner is the call chain (the task) that set the budget. A later
	// task.setAllocBudget from the same task changes this budget's limit; one
	// from a task that inherited it nests a new budget.
	owner *CallChain
	used  atomic.Int64
	// limit is the configured limit (math.MaxInt64: unlimited), reported in the
	// error.
	limit atomic.Int64
	// trip is the count past which the next check raises: the limit, moved up by
	// allocRecoveryBytes past the count at each raise so the code that catches
	// the error (log it, answer a request) can run, and a task that keeps
	// allocating raises again.
	trip atomic.Int64
}

// allocRecoveryBytes is how far past the count a raise moves the next trip
// point: room for the handler that catches the error, small enough that a task
// catching and carrying on raises again soon.
const allocRecoveryBytes = 1 << 20

// allocSample is how many elements of a compound the estimate looks at to
// extrapolate the size of the next level (allocEstimate).
const allocSample = 8

// SetLimit sets the budget's limit; the count is kept. A limit of math.MaxInt64
// lifts it. A count already past the new limit raises at the next charge.
func (b *AllocBudget) SetLimit(limit int64) {
	b.limit.Store(limit)
	b.trip.Store(limit)
}

// Used returns the bytes counted so far.
func (b *AllocBudget) Used() int64 { return b.used.Load() }

// Limit returns the configured limit (math.MaxInt64 when lifted).
func (b *AllocBudget) Limit() int64 { return b.limit.Load() }

// add counts n bytes, here and in every enclosing budget, without checking the
// limits; the next check raises.
func (b *AllocBudget) add(n int64) {
	if n <= 0 {
		return
	}
	for ; b != nil; b = b.parent {
		b.used.Add(n)
	}
}

// check raises the error of the innermost budget (this one or an enclosing one)
// whose count has passed its trip point, else returns nil.
func (b *AllocBudget) check() *runtimeError {
	for ; b != nil; b = b.parent {
		if e := b.checkOne(); e != nil {
			return e
		}
	}
	return nil
}

// checkOne raises the budget error when the count has passed the trip point, and
// moves the trip point allocRecoveryBytes past the count. Of tasks sharing the
// budget that see the same crossing, only the one that moves the point raises.
func (b *AllocBudget) checkOne() *runtimeError {
	u := b.used.Load()
	t := b.trip.Load()
	if u <= t {
		return nil
	}
	next := int64(math.MaxInt64)
	if u < math.MaxInt64-allocRecoveryBytes {
		next = u + allocRecoveryBytes
	}
	if !b.trip.CompareAndSwap(t, next) {
		return nil
	}
	return &runtimeError{
		Kind: "limit",
		Msg:  fmt.Sprintf("task allocation budget exceeded: %d bytes allocated, over the %d-byte budget set by task.setAllocBudget", u, b.limit.Load()),
	}
}

// chargeAt counts n bytes and returns the positioned budget error if the count
// has passed the limit, else nil. An operation that allocated nothing (a
// builtin returning a scalar) is not checked, so a task.setAllocBudget lowering
// the limit below the count does not raise at its own call.
func (b *AllocBudget) chargeAt(n int64, at positioned) error {
	if n <= 0 {
		return nil
	}
	b.add(n)
	if e := b.check(); e != nil {
		e.File, e.Line, e.Col = posOf(at)
		return e
	}
	return nil
}

// allocBudget returns the budget the frame's task counts against, or nil.
func (e *Environment) allocBudget() *AllocBudget {
	if e.depth == nil {
		return nil
	}
	return e.depth.budget
}

// allocEstimate estimates the bytes a builtin allocated to build its result v:
// its own backing (one Value cell per list element and struct / enum field, two
// per map entry, the bytes of a bytes value and of strings) plus the next
// level, extrapolated from the first allocSample elements. Bounded per call (at
// most allocSample elements read), so charging a huge result costs the same as
// a small one. Deeper levels are not counted: a builtin may return a value it
// did not build (a stored element), which a full walk would charge again at
// O(size) per call. An eager copy is charged in full instead (copyBytes).
func allocEstimate(v Value) int64 {
	switch v.Kind {
	case KindString:
		return int64(len(v.Str()))
	case KindBytes:
		return int64(len(v.Bytes()))
	case KindList:
		l := v.List()
		n := len(l)
		if n == 0 {
			return 0
		}
		k := min(n, allocSample)
		var s int64
		for _, e := range l[:k] {
			s += elemBytes(e, true)
		}
		return valueCellBytes*int64(n) + s/int64(k)*int64(n)
	case KindMap:
		m := v.Map()
		n := len(m)
		if n == 0 {
			return 0
		}
		k := min(n, allocSample)
		var s int64
		for _, e := range m[:k] {
			s += elemBytes(e.Key, true) + elemBytes(e.Value, true)
		}
		return 2*valueCellBytes*int64(n) + s/int64(k)*int64(n)
	case KindStruct, KindEnum:
		f := v.Fields()
		n := len(f)
		if n == 0 {
			return 0
		}
		k := min(n, allocSample)
		var s int64
		for _, e := range f[:k] {
			s += elemBytes(e.Value, true)
		}
		return valueCellBytes*int64(n) + s/int64(k)*int64(n)
	}
	return 0
}

// elemBytes is the one-level size of an element for allocEstimate: its own
// backing, without looking inside it.
func elemBytes(v Value, withStrings bool) int64 {
	switch v.Kind {
	case KindString:
		if withStrings {
			return int64(len(v.Str()))
		}
		return 0
	case KindBytes:
		return int64(len(v.Bytes()))
	case KindList:
		return valueCellBytes * int64(len(v.List()))
	case KindMap:
		return 2 * valueCellBytes * int64(len(v.Map()))
	case KindStruct, KindEnum:
		return valueCellBytes * int64(len(v.Fields()))
	}
	return 0
}

// SetAllocBudget sets the calling task's allocation budget to limit bytes
// (math.MaxInt64: unlimited). The first call creates the budget, with a count of
// zero, nested in the budget the task inherited from its spawner if any; a later
// one changes its limit and keeps the count. A task spawned afterwards shares
// it. limit must be positive; the caller validates it.
func (c BuiltinCtx) SetAllocBudget(limit int64) error {
	if c.Depth == nil {
		return fmt.Errorf("no task context to set a budget on")
	}
	cur := c.Depth.budget
	if cur != nil && cur.owner == c.Depth {
		cur.SetLimit(limit)
		return nil
	}
	b := &AllocBudget{parent: cur, owner: c.Depth}
	b.SetLimit(limit)
	c.Depth.budget = b
	return nil
}

// AllocBudget returns the calling task's allocation budget, or nil if it has
// none.
func (c BuiltinCtx) AllocBudget() *AllocBudget {
	if c.Depth == nil {
		return nil
	}
	return c.Depth.budget
}
