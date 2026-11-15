// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build tinygo

package stdinbuf

// pumpState is empty under TinyGo: there is no pump goroutine. The tiny build
// runs a cooperative scheduler (-scheduler=tasks), where a goroutine blocked in
// a read of stdin blocks every goroutine, so a pump could not let a timed read
// give up. The reader that needs data performs the fill itself; a timeout is
// not honoured (ReadByteTimeout blocks like ReadByte).
type pumpState struct{}

// startFill reports true: the caller performs the fill inline.
func (r *Reader) startFill() bool { return true }

func (r *Reader) stop() {}
