// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package limits

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
)

// Size arithmetic and capped reads. A size computed from user or wire input
// (w*h, a count times a width, limit+1) must not wrap: a wrapped product slips
// under a cap check and reaches make() as a panic or a huge allocation, and a
// wrapped limit+1 turns "no limit" into "read nothing". The interpreter has no
// recover(), so both are fatal or silently wrong. The helpers below report
// overflow instead of wrapping, and the readers grow their buffer with the data
// that arrives instead of pre-allocating a requested size.

// ErrTooLarge is the sentinel every TooLargeError matches with errors.Is.
var ErrTooLarge = errors.New("data exceeds the size limit")

// TooLargeError reports input or output past a byte limit. Callers wrap it in
// a message naming the function.
type TooLargeError struct {
	Limit int64
}

func (e *TooLargeError) Error() string {
	return fmt.Sprintf("exceeds the %d-byte limit", e.Limit)
}

// Is makes errors.Is(err, ErrTooLarge) match.
func (e *TooLargeError) Is(target error) bool { return target == ErrTooLarge }

type signed interface{ ~int | ~int64 }

func addChecked[T signed](a, b T) (T, bool) {
	c := a + b
	if (b > 0 && c < a) || (b < 0 && c > a) {
		return 0, false
	}
	return c, true
}

func mulChecked[T signed](a, b T) (T, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	c := a * b
	// The sign test catches MinInt * -1, whose quotient check passes.
	if (c < 0) != ((a < 0) != (b < 0)) || c/b != a {
		return 0, false
	}
	return c, true
}

// AddInt64 returns a+b and whether it fit in int64.
func AddInt64(a, b int64) (int64, bool) { return addChecked(a, b) }

// MulInt64 returns a*b and whether it fit in int64.
func MulInt64(a, b int64) (int64, bool) { return mulChecked(a, b) }

// AddInt returns a+b and whether it fit in int (32 bits on jennifer-tiny).
func AddInt(a, b int) (int, bool) { return addChecked(a, b) }

// MulInt returns a*b and whether it fit in int (32 bits on jennifer-tiny).
func MulInt(a, b int) (int, bool) { return mulChecked(a, b) }

// LimitPlusOne returns limit+1 for a capped reader that reads one byte past
// the limit to detect overflow, saturating at MaxInt64 (a byte count no reader
// reaches) instead of wrapping to MinInt64. A negative limit gives 0.
func LimitPlusOne(limit int64) int64 {
	if limit < 0 {
		return 0
	}
	if limit == math.MaxInt64 {
		return limit
	}
	return limit + 1
}

// ReadAll reads r to EOF, keeping at most limit bytes. The buffer grows as
// data arrives, so a large limit costs nothing up front. More than limit bytes
// is a *TooLargeError (the data read is discarded). A negative limit is 0.
func ReadAll(r io.Reader, limit int64) ([]byte, error) {
	if limit < 0 {
		limit = 0
	}
	data, err := io.ReadAll(io.LimitReader(r, LimitPlusOne(limit)))
	if err != nil {
		return data, err
	}
	if int64(len(data)) > limit {
		return nil, &TooLargeError{Limit: limit}
	}
	return data, nil
}

// readNChunk is ReadN's first buffer size and growth floor.
const readNChunk = 64 << 10

// ReadN reads exactly n bytes from r. The buffer starts at min(n, 64 KiB) and
// doubles as data arrives (never past n), so a peer that announces a large n
// and then stalls holds only what it sent. before, if not nil, runs before each
// Read (to arm a deadline). n above MaxAllocBytes is a *TooLargeError before
// any read; a negative n is an error. A short stream returns the bytes read
// with io.ErrUnexpectedEOF (io.EOF when none arrived); another read error is
// returned with the bytes read so far.
func ReadN(r io.Reader, n int64, before func()) ([]byte, error) {
	if n < 0 {
		return nil, fmt.Errorf("negative length %d", n)
	}
	if n > MaxAllocBytes {
		return nil, &TooLargeError{Limit: MaxAllocBytes}
	}
	buf := make([]byte, 0, min(n, readNChunk))
	for int64(len(buf)) < n {
		if len(buf) == cap(buf) {
			nc := min(int64(cap(buf))*2, n)
			nb := make([]byte, len(buf), nc)
			copy(nb, buf)
			buf = nb
		}
		if before != nil {
			before()
		}
		got, err := r.Read(buf[len(buf):cap(buf)])
		buf = buf[:len(buf)+got]
		if int64(len(buf)) >= n {
			// A Reader may return the final bytes together with io.EOF.
			break
		}
		if err != nil {
			if errors.Is(err, io.EOF) && len(buf) > 0 {
				err = io.ErrUnexpectedEOF
			}
			return buf, err
		}
	}
	return buf, nil
}

// ReadDelim reads from br through the first delim, like bufio.Reader's
// ReadBytes, but keeps at most limit bytes (delim included): a longer run is a
// *TooLargeError after limit bytes are consumed, never an unbounded buffer. At
// EOF it returns the bytes read with io.EOF.
func ReadDelim(br *bufio.Reader, delim byte, limit int64) ([]byte, error) {
	var out []byte
	for {
		frag, err := br.ReadSlice(delim)
		if int64(len(out))+int64(len(frag)) > limit {
			return nil, &TooLargeError{Limit: limit}
		}
		out = append(out, frag...)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return out, err
	}
}
