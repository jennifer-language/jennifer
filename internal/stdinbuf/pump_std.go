// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package stdinbuf

// pumpState is the pump goroutine's control: it starts with the first fill
// request, performs one fill per wake, and exits at the end of input, after a
// read error, or when a test resets the registry.
type pumpState struct {
	started bool
	wake    chan struct{} // capacity 1; r.filling keeps it to one pending wake
	quit    chan struct{}
}

// startFill hands the fill to the pump (starting it on first use) and reports
// false: the caller waits for the result instead of reading itself. Called
// with r.mu held and r.filling just set.
func (r *Reader) startFill() bool {
	if !r.started {
		r.started = true
		r.wake = make(chan struct{}, 1)
		r.quit = make(chan struct{})
		go r.pump(r.wake, r.quit)
	}
	r.wake <- struct{}{}
	return false
}

// pump performs the fills. A fill blocked in the source's Read stays blocked
// across a raw-mode switch: the terminal applies the new mode to the pending
// read, so the bytes it returns are the ones the next reader expects.
func (r *Reader) pump(wake, quit chan struct{}) {
	for {
		select {
		case <-quit:
			return
		case <-wake:
		}
		r.fillOnce()
		r.mu.Lock()
		done := r.err != nil
		r.mu.Unlock()
		if done {
			return
		}
	}
}

// stop ends an idle pump (ResetForTest); a later fill starts a new one.
// Called with r.mu held.
func (r *Reader) stop() {
	if r.started {
		close(r.quit)
		r.started = false
	}
}
