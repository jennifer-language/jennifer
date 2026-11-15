// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// Package stdinbuf is the one reader layer over the interpreter's input
// (stdin). Every library that reads it - `io` (readLine, readLines, readBytes,
// readChars, eof) and `term` (readByte, readByteTimeout) - takes bytes from the
// same buffer here, so the readers can be mixed in any order: each byte goes to
// exactly one read, in input order, and a `term` read that timed out loses
// nothing (the byte it was waiting for is the next byte any reader gets).
//
// One fill (a single Read of the source) is in flight at a time. In the default
// build a pump goroutine performs the fills, started by the first read that
// needs data (a program that never reads stdin never touches it), and it reads
// only on demand: when a reader finds the buffer short. So at most one chunk is
// read ahead, and a timed-out read leaves exactly that one fill pending. In the
// TinyGo build (pump_tinygo.go) the reader that needs data performs the fill
// itself; there a blocking read stops the whole cooperative scheduler, so a
// read with a timeout is not supported (`term` is stubbed there anyway).
//
// End of input and read errors are sticky: once the source reports one, every
// later read on either library sees it after the buffered bytes are drained.
package stdinbuf

import (
	"io"
	"sync"
	"time"
	"unicode/utf8"

	"jennifer-lang.dev/jennifer/internal/limits"
)

// chunkSize is the most one fill reads. A terminal in raw mode returns what is
// there (a key or one escape sequence); a pipe or file returns up to this much.
const chunkSize = 4096

// maxEmptyReads turns a source that keeps returning (0, nil) into
// io.ErrNoProgress, as bufio does, instead of spinning forever.
const maxEmptyReads = 100

// Reader is the shared buffered reader over one input source.
type Reader struct {
	src io.Reader

	mu      sync.Mutex
	buf     []byte // unread bytes are buf[off:]
	off     int
	err     error // sticky end of input or read error, after buf drains
	empties int   // consecutive (0, nil) reads
	filling bool  // a fill is in flight
	changed chan struct{}
	chunk   []byte // fill buffer; only the one in-flight fill uses it

	pumpState // build-specific (pump goroutine or none)
}

var (
	regMu   sync.Mutex
	readers = map[io.Reader]*Reader{}
)

// For returns the shared reader for an input source, creating it on first use.
// Every library must read the source through it, never directly. nil yields nil.
func For(src io.Reader) *Reader {
	if src == nil {
		return nil
	}
	regMu.Lock()
	defer regMu.Unlock()
	r := readers[src]
	if r == nil {
		r = &Reader{src: src, changed: make(chan struct{})}
		readers[src] = r
	}
	return r
}

// ResetForTest forgets every reader (and stops idle pumps), so a test can
// start from a clean slate.
func ResetForTest() {
	regMu.Lock()
	defer regMu.Unlock()
	for _, r := range readers {
		r.mu.Lock()
		r.stop()
		r.mu.Unlock()
	}
	readers = map[io.Reader]*Reader{}
}

// fillOnce performs one Read of the source and publishes the result. It runs
// with r.mu not held and r.filling set, so it is the only fill in flight.
func (r *Reader) fillOnce() {
	if r.chunk == nil {
		r.chunk = make([]byte, chunkSize)
	}
	n, err := r.src.Read(r.chunk)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.off > 0 {
		k := copy(r.buf, r.buf[r.off:])
		r.buf = r.buf[:k]
		r.off = 0
	}
	r.buf = append(r.buf, r.chunk[:n]...)
	if n == 0 && err == nil {
		r.empties++
		if r.empties >= maxEmptyReads {
			err = io.ErrNoProgress
		}
	} else {
		r.empties = 0
	}
	if err != nil && r.err == nil {
		r.err = err
	}
	r.filling = false
	close(r.changed)
	r.changed = make(chan struct{})
}

// await asks for one more fill (unless one is in flight) and waits for a fill
// to complete. Called and returns with r.mu held. A nil timer waits without
// limit; the result is false when the timer fired first, in which case the
// fill stays in flight and its bytes go to whichever read comes next.
func (r *Reader) await(timer <-chan time.Time) bool {
	if !r.filling {
		r.filling = true
		if r.startFill() {
			r.mu.Unlock()
			r.fillOnce()
			r.mu.Lock()
			return true
		}
	}
	ch := r.changed
	r.mu.Unlock()
	ok := true
	if timer == nil {
		<-ch
	} else {
		select {
		case <-ch:
		case <-timer:
			ok = false
		}
	}
	r.mu.Lock()
	return ok
}

func (r *Reader) avail() []byte { return r.buf[r.off:] }

func (r *Reader) consume(n int) {
	r.off += n
	if r.off == len(r.buf) {
		r.buf = r.buf[:0]
		r.off = 0
	}
}

// ReadByte returns the next byte, blocking until one arrives. At the end of
// input it returns io.EOF; after a read error, that error.
func (r *Reader) ReadByte() (byte, error) {
	b, _, err := r.readByte(nil)
	return b, err
}

// ReadByteTimeout is ReadByte giving up after d (0 polls): ok is false when
// no byte arrived in time. The fill it started stays in flight, so the byte
// is not lost: the next read of any kind returns it.
func (r *Reader) ReadByteTimeout(d time.Duration) (b byte, ok bool, err error) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	return r.readByte(timer.C)
}

func (r *Reader) readByte(timer <-chan time.Time) (byte, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for {
		if a := r.avail(); len(a) > 0 {
			r.consume(1)
			return a[0], true, nil
		}
		if r.err != nil {
			return 0, true, r.err
		}
		if !r.await(timer) {
			return 0, false, nil
		}
	}
}

// Peek waits until at least one byte is buffered and returns nil, or returns
// the sticky end-of-input / read error when none will come. Nothing is consumed.
func (r *Reader) Peek() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for {
		if len(r.avail()) > 0 {
			return nil
		}
		if r.err != nil {
			return r.err
		}
		r.await(nil)
	}
}

// ReadRune reads one UTF-8 encoded rune like bufio.Reader.ReadRune: an invalid
// byte (or a partial rune at the end of input) yields (utf8.RuneError, 1).
func (r *Reader) ReadRune() (rune, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for {
		a := r.avail()
		if len(a) > 0 && (utf8.FullRune(a) || r.err != nil) {
			ch, size := utf8.DecodeRune(a)
			r.consume(size)
			return ch, size, nil
		}
		if len(a) == 0 && r.err != nil {
			return 0, 0, r.err
		}
		r.await(nil)
	}
}

// ReadDelim reads through the next delim and returns the bytes including it.
// Without a delim before the end of input it returns what is left and the
// sticky error (io.EOF at a clean end). More than limit bytes is a
// *limits.TooLargeError; the bytes up to there are consumed.
func (r *Reader) ReadDelim(delim byte, limit int64) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []byte
	for {
		a := r.avail()
		take := len(a)
		found := false
		for i, c := range a {
			if c == delim {
				take, found = i+1, true
				break
			}
		}
		if int64(len(out))+int64(take) > limit {
			r.consume(take)
			return nil, &limits.TooLargeError{Limit: limit}
		}
		out = append(out, a[:take]...)
		r.consume(take)
		if found {
			return out, nil
		}
		if r.err != nil {
			return out, r.err
		}
		r.await(nil)
	}
}

// Read implements io.Reader: it returns the buffered bytes (up to len(p)),
// waiting for a fill only when none are buffered.
func (r *Reader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for {
		if a := r.avail(); len(a) > 0 {
			n := copy(p, a)
			r.consume(n)
			return n, nil
		}
		if r.err != nil {
			return 0, r.err
		}
		r.await(nil)
	}
}
