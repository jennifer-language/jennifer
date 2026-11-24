// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build linux && !tinygo

package seriallib

import (
	"fmt"
	"io"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// parityDecoder undoes the marking the tty line discipline applies with PARMRK
// (and ISTRIP clear): a received 0xFF arrives as 0xFF 0xFF, and a byte received
// with a parity or framing error (or a break) as 0xFF 0x00 <byte>. Without
// PARMRK such a byte reads as 0x00, which a real NUL also is (M29.49).
//
// in holds decoded bytes not yet returned; errs the positions in `in` where a
// marked byte was dropped (ascending; an error sits before in[pos]); reported
// whether errs[0] has already been thrown. raw is an incomplete mark sequence
// left at the end of the last device read.
type parityDecoder struct {
	in       []byte
	errs     []int
	reported bool
	raw      []byte
}

// feed decodes chunk (appended to any incomplete mark from the last read).
func (d *parityDecoder) feed(chunk []byte) {
	buf := append(d.raw, chunk...)
	d.raw = nil
	i := 0
	for i < len(buf) {
		c := buf[i]
		if c != 0xFF {
			d.in = append(d.in, c)
			i++
			continue
		}
		if i+1 >= len(buf) {
			d.raw = append([]byte(nil), buf[i:]...)
			return
		}
		switch buf[i+1] {
		case 0xFF:
			d.in = append(d.in, 0xFF)
			i += 2
		case 0x00:
			if i+2 >= len(buf) {
				d.raw = append([]byte(nil), buf[i:]...)
				return
			}
			d.errs = append(d.errs, len(d.in))
			i += 3
		default:
			// Not a sequence the line discipline writes; keep the bytes.
			d.in = append(d.in, 0xFF)
			i++
		}
	}
}

// take returns up to n decoded bytes, or the offset of a parity error that a
// read of n bytes meets (errAt >= 0). ok is false when nothing is buffered and
// the device must be read. A read meets an error when fewer than n good bytes
// precede it; it throws once, and the next read returns the bytes before it.
func (d *parityDecoder) take(n int) (out []byte, errAt int, ok bool) {
	for len(d.errs) > 0 && d.errs[0] == 0 && d.reported {
		d.errs = d.errs[1:]
		d.reported = false
	}
	if len(d.errs) > 0 && d.errs[0] < n && !d.reported {
		d.reported = true
		return nil, d.errs[0], true
	}
	if len(d.in) == 0 {
		return nil, -1, false
	}
	k := min(n, len(d.in))
	if len(d.errs) > 0 {
		k = min(k, d.errs[0])
	}
	out = append([]byte(nil), d.in[:k]...)
	d.in = d.in[k:]
	for i := range d.errs {
		d.errs[i] -= k
	}
	return out, -1, true
}

// readMarked is serial.read on a port with parity on: it returns buffered
// decoded bytes first, and reads the device only when none are left.
func readMarked(ctx interpreter.BuiltinCtx, p *portState, n int) (Value, error) {
	if n == 0 {
		return interpreter.BytesVal([]byte{}), nil
	}
	p.rmu.Lock()
	defer p.rmu.Unlock()
	buf := make([]byte, 4096)
	for {
		out, errAt, ok := p.dec.take(n)
		if errAt >= 0 {
			return interpreter.Null(), interpreter.RaiseError("serial", fmt.Sprintf("serial.read: parity error at byte offset %d (the bytes before it come back from the next read)", errAt), ctx.File, ctx.Line, ctx.Col)
		}
		if ok {
			return interpreter.BytesVal(out), nil
		}
		stop := watchCancel(ctx, p)
		got, rerr := p.f.Read(buf)
		if stop() {
			p.mu.Lock()
			_ = p.f.SetReadDeadline(p.deadline)
			p.mu.Unlock()
			return interpreter.Null(), interpreter.ErrTaskCancelled()
		}
		if rerr == io.EOF || (rerr != nil && hungUp(rerr)) {
			return interpreter.Null(), interpreter.RaiseError("closed", "serial.read: port hung up (device unplugged or the other end closed)", ctx.File, ctx.Line, ctx.Col)
		}
		if rerr != nil {
			return interpreter.Null(), failure(ctx, "serial.read", "read", rerr)
		}
		p.dec.feed(buf[:got])
	}
}
