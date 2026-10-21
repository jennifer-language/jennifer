// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// Package channellib implements Jennifer's `channel` library: CSP-style channels
// carrying `channel of T` values between goroutines (the counterpart to `spawn` /
// `task`). A channel is a shared handle - copies, including the spawn snapshot,
// refer to the one underlying Go channel - but the VALUES sent through it are
// deep-copied at the send site, so channels carry copies and the
// no-shared-mutable-state guarantee holds. That is why channels, not mutexes /
// atomics, are the coordination primitive: shared locks would violate value
// semantics and stay rejected.
//
// Surface: make / send / recv / close, plus a fan-in select and len / capacity.
//
// The Go package is named channellib so it doesn't collide with the `channel`
// type keyword if anyone imports it by short name.
package channellib

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// LibraryName is the namespace prefix and `use` name.
const LibraryName = "channel"

// Install registers the channel builtins.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespaced(LibraryName, "make", makeFn)
	in.RegisterNamespaced(LibraryName, "send", sendFn)
	in.RegisterNamespaced(LibraryName, "recv", recvFn)
	in.RegisterNamespaced(LibraryName, "close", closeFn)
	in.RegisterNamespaced(LibraryName, "select", selectFn)
	in.RegisterNamespaced(LibraryName, "len", lenFn)
	in.RegisterNamespaced(LibraryName, "capacity", capacityFn)
}

// extractChannel pulls the *ChannelState out of a KindChannel Value.
func extractChannel(fnName string, v interpreter.Value) (*interpreter.ChannelState, error) {
	if v.Kind != interpreter.KindChannel {
		return nil, fmt.Errorf("%s: argument must be a channel, got %s", fnName, v.Kind)
	}
	if v.Chan == nil {
		return nil, fmt.Errorf("%s: channel has no state (uninitialized; use channel.make)", fnName)
	}
	return v.Chan, nil
}

// makeFn: channel.make(capacity) -> a fresh channel. capacity 0 is unbuffered
// (send blocks until a receiver is ready); capacity n buffers up to n values. The
// element type is recorded when the result binds to a `channel of T`.
func makeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (retVal interpreter.Value, retErr error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("channel.make expects 1 argument (capacity), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("channel.make: capacity must be int, got %s", args[0].Kind)
	}
	capacity := args[0].Int
	if capacity < 0 {
		return interpreter.Null(), fmt.Errorf("channel.make: capacity must be >= 0, got %d", capacity)
	}
	// Cap the buffer a single channel can allocate. A buffered `chan Value`
	// commits its whole buffer eagerly (~272 bytes per slot), so an unbounded
	// capacity is a multi-gigabyte allocation that OOM-kills the process before Go
	// even reaches its makechan "size out of range" panic - a fatal, unrecoverable
	// failure. Bounding it here turns that into a catchable error, the same class
	// as limits.MaxRangeElements.
	if capacity > int64(limits.MaxChannelCapacity) {
		return interpreter.Null(), fmt.Errorf("channel.make: capacity %d exceeds the limit of %d", capacity, limits.MaxChannelCapacity)
	}
	// Belt-and-braces: the cap above keeps us well below the makechan panic
	// threshold, but recover any residual makechan panic so an unforeseen edge is
	// still a catchable error rather than an interpreter crash.
	defer func() {
		if r := recover(); r != nil {
			retVal, retErr = interpreter.Null(), fmt.Errorf("channel.make: capacity %d is too large", capacity)
		}
	}()
	state := &interpreter.ChannelState{
		Ch:       make(chan interpreter.Value, int(capacity)),
		Capacity: int(capacity),
	}
	return interpreter.ChannelVal(state), nil
}

// sendFn: channel.send(ch, value) -> null. The value is deep-copied in (value
// semantics: the receiver gets its own copy), then sent - blocking per the
// channel's capacity. Sending on a closed channel is a catchable error, not a Go
// panic (guarded by the closed flag plus a recover for the check-then-send race).
func sendFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (retVal interpreter.Value, retErr error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("channel.send expects 2 arguments (channel, value), got %d", len(args))
	}
	state, err := extractChannel("channel.send", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	// Validate the value against the channel's declared element type at the send
	// site, so a wrong-typed send fails here rather than deferring to the receiving
	// binding. ElemTyp is nil only for a channel not yet bound to a `channel of T`
	// (a fresh channel.make result), which stays unchecked - like a generic list.
	if et := state.ElemTyp.Load(); et != nil && !args[1].MatchesDeclared(*et) {
		return interpreter.Null(), fmt.Errorf("channel.send: value must be %s, got %s", *et, args[1].Kind)
	}
	if state.IsClosed() {
		return interpreter.Null(), fmt.Errorf("channel.send: send on a closed channel")
	}
	// A close between the check above and the send below would make the send
	// panic; recover turns that race into the same catchable error.
	defer func() {
		if r := recover(); r != nil {
			retVal, retErr = interpreter.Null(), fmt.Errorf("channel.send: send on a closed channel")
		}
	}()
	state.Send(args[1].Copy())
	return interpreter.Null(), nil
}

// recvFn: channel.recv(ch) -> T. Blocks until a value is available and returns it.
// On a channel that has been closed AND drained, it throws a catchable "receive on
// a closed channel" error - the drain idiom is `try { while (true) {
// process(channel.recv($ch)); } } catch (e) { }`.
func recvFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("channel.recv expects 1 argument (channel), got %d", len(args))
	}
	state, err := extractChannel("channel.recv", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	v, ok := <-state.Ch
	if !ok {
		return interpreter.Null(), fmt.Errorf("channel.recv: receive on a closed channel")
	}
	return v, nil
}

// closeFn: channel.close(ch) -> null. Closes the channel so a draining receiver
// sees the end. Closing an already-closed channel is a catchable error (not a Go
// panic). After close, further sends are catchable errors; buffered values still
// receive until drained.
func closeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("channel.close expects 1 argument (channel), got %d", len(args))
	}
	state, err := extractChannel("channel.close", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if !state.CloseOnce() {
		return interpreter.Null(), fmt.Errorf("channel.close: channel is already closed")
	}
	return interpreter.Null(), nil
}

// selectFn: channel.select(channels) -> T. A fan-in receive: blocks until any of
// the channels has a value and returns it (the next value from whichever channel
// is ready first). A closed channel is dropped from the wait set; when every
// channel is closed and drained, it throws a catchable "all channels are closed"
// error. Note this returns the received VALUE, not an index (a channel receive is
// destructive, and Jennifer has no multiple-return, so an index-plus-value select
// would need a follow-up shape); it is the merge / fan-in primitive.
func selectFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("channel.select expects 1 argument (list of channel), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindList {
		return interpreter.Null(), fmt.Errorf("channel.select: argument must be a list of channel, got %s", args[0].Kind)
	}
	states := make([]*interpreter.ChannelState, 0, len(args[0].List))
	for i, e := range args[0].List {
		s, err := extractChannel(fmt.Sprintf("channel.select: element %d", i), e)
		if err != nil {
			return interpreter.Null(), err
		}
		states = append(states, s)
	}
	if len(states) == 0 {
		return interpreter.Null(), fmt.Errorf("channel.select: the list is empty (no channels to select on)")
	}
	if v, ok := selectRecv(states); ok {
		return v, nil
	}
	return interpreter.Null(), fmt.Errorf("channel.select: all channels are closed")
}

// selectStart rotates the channel a select polls first, so a channel that always
// has a value ready cannot starve the ones after it in the list.
var selectStart atomic.Uint32

// selectRecv receives the next value from whichever channel in states is ready
// first, or reports ok=false once every channel is closed and drained (a closed
// channel is dropped from the wait set; the call keeps blocking on the open
// ones). There is no reflect.Select under TinyGo, so it registers one waker with
// every channel BEFORE polling them without blocking - a send or close the poll
// missed then pokes the waker (see interpreter/waitset.go). A sender that has
// poked but not yet parked on an unbuffered channel is not receivable yet;
// HasPendingSend covers that brief window with a short retry instead of a sleep
// on the waker.
func selectRecv(states []*interpreter.ChannelState) (interpreter.Value, bool) {
	waker := interpreter.NewWaker()
	for _, s := range states {
		s.Subscribe(waker)
	}
	defer func() {
		for _, s := range states {
			s.Unsubscribe(waker)
		}
	}()
	live := append([]*interpreter.ChannelState(nil), states...)
	spins := 0
	for len(live) > 0 {
		start := int(selectStart.Add(1) % uint32(len(live)))
		pending := false
		for k := 0; k < len(live); k++ {
			i := (start + k) % len(live)
			v, ok, ready := live[i].TryRecv()
			if !ready {
				pending = pending || live[i].HasPendingSend()
				continue
			}
			if ok {
				return v, true
			}
			// Closed and drained: drop it and rescan the rest.
			live = append(live[:i], live[i+1:]...)
			k = -1
			if len(live) == 0 {
				break
			}
			start = 0
		}
		if len(live) == 0 {
			break
		}
		if pending {
			// A sender is between its poke and parking on the channel: yield so it
			// can park, then poll again. Back off to a short sleep if it takes
			// longer than a few yields (it is on another thread).
			spins++
			if spins < 16 {
				runtime.Gosched()
			} else {
				time.Sleep(50 * time.Microsecond)
			}
			continue
		}
		spins = 0
		<-waker
	}
	return interpreter.Value{}, false
}

// lenFn: channel.len(ch) -> int. The number of values currently buffered (Go
// len(ch)).
func lenFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("channel.len expects 1 argument (channel), got %d", len(args))
	}
	state, err := extractChannel("channel.len", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.IntVal(int64(len(state.Ch))), nil
}

// capacityFn: channel.capacity(ch) -> int. The channel's buffer capacity (0 =
// unbuffered).
func capacityFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("channel.capacity expects 1 argument (channel), got %d", len(args))
	}
	state, err := extractChannel("channel.capacity", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.IntVal(int64(state.Capacity)), nil
}
