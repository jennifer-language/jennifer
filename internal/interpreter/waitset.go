// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package interpreter

import (
	"errors"
	"sync"
	"time"
)

// waitSet is the wake-up list behind the multi-way waits (`task.waitAny`,
// `task.waitAnyTimeout`, `channel.select`). Those block on "any of N" Go
// channels, a dynamic shape a static `select` cannot express; `reflect.Select`
// can, but TinyGo leaves it unimplemented (a runtime panic), and a helper
// goroutine per case is both costly under jennifer-tiny (each goroutine gets the
// full `-stack-size` stack) and wrong for channels (a helper's receive consumes a
// value the select may not return).
//
// Instead a waiter registers one buffered wake-up channel with every source,
// polls each source without blocking, and only then sleeps on the wake-up
// channel. A source pokes its registered waiters after every state change (a
// task finishing, a channel send or close). Registering BEFORE polling is what
// rules out a lost wake-up: a change the poll missed happened after the
// registration, so its poke reaches the waiter.
type waitSet struct {
	mu      sync.Mutex
	waiters []chan struct{}
}

// add registers w. A waiter registered with the same source twice (a duplicate
// list element) is simply poked twice; the buffer of one absorbs it.
func (s *waitSet) add(w chan struct{}) {
	s.mu.Lock()
	s.waiters = append(s.waiters, w)
	s.mu.Unlock()
}

// remove drops one registration of w.
func (s *waitSet) remove(w chan struct{}) {
	s.mu.Lock()
	for i, x := range s.waiters {
		if x == w {
			last := len(s.waiters) - 1
			s.waiters[i] = s.waiters[last]
			s.waiters[last] = nil
			s.waiters = s.waiters[:last]
			break
		}
	}
	s.mu.Unlock()
}

// notify pokes every registered waiter without blocking: a wake-up channel has a
// buffer of one, so a waiter that already has a pending poke loses nothing.
func (s *waitSet) notify() {
	s.mu.Lock()
	for _, w := range s.waiters {
		select {
		case w <- struct{}{}:
		default:
		}
	}
	s.mu.Unlock()
}

// NewWaker returns a wake-up channel for Subscribe / Unsubscribe.
func NewWaker() chan struct{} { return make(chan struct{}, 1) }

// Subscribe registers w to be poked when the task finishes.
func (t *TaskState) Subscribe(w chan struct{}) { t.waiters.add(w) }

// Unsubscribe drops a registration made by Subscribe.
func (t *TaskState) Unsubscribe(w chan struct{}) { t.waiters.remove(w) }

// finish closes Done and wakes every multi-way waiter. Done is closed first, so
// a woken waiter's poll is guaranteed to see it.
func (t *TaskState) finish() {
	close(t.Done)
	t.waiters.notify()
}

// Subscribe registers w to be poked on every send to and on the close of the
// channel.
func (c *ChannelState) Subscribe(w chan struct{}) { c.waiters.add(w) }

// Unsubscribe drops a registration made by Subscribe.
func (c *ChannelState) Unsubscribe(w chan struct{}) { c.waiters.remove(w) }

// ErrChannelClosed is returned by Send on a closed channel (also when the
// channel is closed while the send is parked).
var ErrChannelClosed = errors.New("send on a closed channel")

// Send delivers v, blocking per the channel's capacity, and wakes multi-way
// waiters. It returns ErrChannelClosed when the channel is or becomes closed
// before the value is taken, and ErrTaskCancelled when cancel fires first (nil
// never fires). The pending count covers the one window a poke cannot: a sender
// that has poked but not yet parked on an unbuffered (or full) channel is not yet
// receivable, so a waiter whose poll finds nothing while HasPendingSend is true
// retries shortly instead of sleeping. Once the sender parks, a non-blocking
// receive takes the value directly.
func (c *ChannelState) Send(v Value, cancel <-chan struct{}) error {
	if c.closed.Load() {
		return ErrChannelClosed
	}
	c.pending.Add(1)
	defer c.pending.Add(-1)
	c.waiters.notify()
	select {
	case c.Ch <- v:
	case <-c.done:
		return ErrChannelClosed
	case <-cancel:
		return ErrTaskCancelled()
	}
	// A buffered send completes without a receiver; poke again so a waiter that
	// polled before the value landed picks it up.
	c.waiters.notify()
	return nil
}

// HasPendingSend reports whether a Send is in progress on the channel.
func (c *ChannelState) HasPendingSend() bool { return c.pending.Load() > 0 }

// TryRecv receives without blocking. ready is false when nothing is available
// (the channel is open and empty); otherwise ok mirrors a Go receive (false =
// closed and drained).
func (c *ChannelState) TryRecv() (v Value, ok, ready bool) {
	select {
	case v = <-c.Ch:
		return v, true, true
	default:
	}
	if !c.closed.Load() {
		return Value{}, false, false
	}
	// Closed: take a value a send completed just before the close, if any.
	select {
	case v = <-c.Ch:
		return v, true, true
	default:
		return Value{}, false, true
	}
}

// Recv blocks until a value is available (ok=true), the channel is closed and
// drained (ok=false), or cancel fires (ErrTaskCancelled; nil never fires).
// Buffered values are drained before a close is reported.
func (c *ChannelState) Recv(cancel <-chan struct{}) (v Value, ok bool, err error) {
	if v, ok, ready := c.TryRecv(); ready {
		return v, ok, nil
	}
	select {
	case v = <-c.Ch:
		return v, true, nil
	case <-c.done:
		v, ok, _ = c.TryRecv()
		return v, ok, nil
	case <-cancel:
		return Value{}, false, ErrTaskCancelled()
	}
}

// RecvTimeout is Recv bounded by timeout: timedOut is true when it fires first
// (no value taken: one that lands later stays in the channel for the next
// receive). A nil timeout never fires. A value already waiting, or a closed
// and drained channel, is reported without looking at the timeout, so an
// already-fired timer (0 ms) is a poll.
func (c *ChannelState) RecvTimeout(cancel <-chan struct{}, timeout <-chan time.Time) (v Value, ok, timedOut bool, err error) {
	if v, ok, ready := c.TryRecv(); ready {
		return v, ok, false, nil
	}
	select {
	case v = <-c.Ch:
		return v, true, false, nil
	case <-c.done:
		v, ok, _ = c.TryRecv()
		return v, ok, false, nil
	case <-cancel:
		return Value{}, false, false, ErrTaskCancelled()
	case <-timeout:
		return Value{}, false, true, nil
	}
}
