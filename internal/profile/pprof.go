// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package profile

import (
	"compress/gzip"
	"io"
)

// pprof.go emits a pprof-compatible profile (gzipped protobuf) by hand,
// keeping the project's near-zero-dependency stance - no google/pprof import.
// The wire format is small enough to encode directly: the profile.proto
// fields this needs are ValueType, Sample, Location, Line, Function, and the
// string table. `go tool pprof` and https://www.speedscope.app/ consume the
// result, so we ship no renderer of our own.

// pbuf is a minimal protobuf writer.
type pbuf struct{ buf []byte }

func (p *pbuf) varint(x uint64) {
	for x >= 0x80 {
		p.buf = append(p.buf, byte(x)|0x80)
		x >>= 7
	}
	p.buf = append(p.buf, byte(x))
}

func (p *pbuf) tag(field, wire int) { p.varint(uint64(field)<<3 | uint64(wire)) }

func (p *pbuf) uint64Field(field int, v uint64) { p.tag(field, 0); p.varint(v) }

func (p *pbuf) int64Field(field int, v int64) { p.tag(field, 0); p.varint(uint64(v)) }

// msgField writes a length-delimited field (embedded message or packed bytes).
func (p *pbuf) msgField(field int, b []byte) {
	p.tag(field, 2)
	p.varint(uint64(len(b)))
	p.buf = append(p.buf, b...)
}

func (p *pbuf) stringField(field int, s string) {
	p.tag(field, 2)
	p.varint(uint64(len(s)))
	p.buf = append(p.buf, s...)
}

// packedUint64 / packedInt64 write proto3 packed repeated scalar fields.
func (p *pbuf) packedUint64(field int, vals []uint64) {
	var inner pbuf
	for _, v := range vals {
		inner.varint(v)
	}
	p.msgField(field, inner.buf)
}

func (p *pbuf) packedInt64(field int, vals []int64) {
	var inner pbuf
	for _, v := range vals {
		inner.varint(uint64(v))
	}
	p.msgField(field, inner.buf)
}

// stringTable interns strings for the pprof string_table (index 0 is "").
type stringTable struct {
	list []string
	idx  map[string]int64
}

func newStringTable() *stringTable {
	st := &stringTable{idx: map[string]int64{}}
	st.intern("")
	return st
}

func (st *stringTable) intern(s string) int64 {
	if i, ok := st.idx[s]; ok {
		return i
	}
	i := int64(len(st.list))
	st.list = append(st.list, s)
	st.idx[s] = i
	return i
}

// valueType builds a ValueType message: type (field 1) and unit (field 2),
// both string-table indices.
func valueType(typeIdx, unitIdx int64) []byte {
	var p pbuf
	p.int64Field(1, typeIdx)
	p.int64Field(2, unitIdx)
	return p.buf
}

// label builds a Label message: key (field 1) and str (field 2) string idxs.
func label(keyIdx, strIdx int64) []byte {
	var p pbuf
	p.int64Field(1, keyIdx)
	p.int64Field(2, strIdx)
	return p.buf
}

// Pprof writes a gzipped pprof Profile. For the statement profile the sample
// values are [hits, self_ns, cum_ns]; for --allocs they are [count] under an
// `alloc_objects` sample type (so `go tool pprof --alloc_objects` reads it),
// with a `kind` label distinguishing eager copies from spawn copies.
func (c *Collector) Pprof(w io.Writer) error {
	st := newStringTable()
	var prof pbuf

	// Sample types (field 1).
	if c.mode == ModeAllocs {
		prof.msgField(1, valueType(st.intern("alloc_objects"), st.intern("count")))
	} else {
		prof.msgField(1, valueType(st.intern("hits"), st.intern("count")))
		prof.msgField(1, valueType(st.intern("self"), st.intern("nanoseconds")))
		prof.msgField(1, valueType(st.intern("cum"), st.intern("nanoseconds")))
	}

	var funcID, locID uint64
	// addLoc appends a Function (field 5) and Location (field 4) for a
	// position and returns the location id.
	addLoc := func(file string, line int, name string) uint64 {
		funcID++
		var fn pbuf
		fn.uint64Field(1, funcID)
		fn.int64Field(2, st.intern(name)) // name
		fn.int64Field(4, st.intern(file)) // filename
		fn.int64Field(5, int64(line))     // start_line
		prof.msgField(5, fn.buf)

		var ln pbuf
		ln.uint64Field(1, funcID) // function_id
		ln.int64Field(2, int64(line))
		var loc pbuf
		locID++
		loc.uint64Field(1, locID) // id
		loc.msgField(4, ln.buf)   // line
		prof.msgField(4, loc.buf)
		return locID
	}

	if c.mode == ModeAllocs {
		emit := func(e *eventSample, kind string) {
			id := addLoc(e.file, e.line, e.pos())
			var s pbuf
			s.packedUint64(1, []uint64{id})
			s.packedInt64(2, []int64{e.count})
			s.msgField(3, label(st.intern("kind"), st.intern(kind)))
			prof.msgField(2, s.buf)
		}
		for _, e := range c.eventsSorted(c.eager) {
			emit(e, "eager")
		}
		for _, e := range c.eventsSorted(c.spawn) {
			emit(e, "spawn")
		}
	} else {
		var totalSelf int64
		for _, r := range c.stmtSorted() {
			id := addLoc(r.file, r.line, r.pos())
			var s pbuf
			s.packedUint64(1, []uint64{id})
			s.packedInt64(2, []int64{r.hits, int64(r.self), int64(r.cum)})
			prof.msgField(2, s.buf)
			totalSelf += int64(r.self)
		}
		prof.int64Field(10, totalSelf) // duration_nanos
	}

	// String table (field 6) - emitted last, after all interning.
	for _, s := range st.list {
		prof.stringField(6, s)
	}

	gz := gzip.NewWriter(w)
	if _, err := gz.Write(prof.buf); err != nil {
		return err
	}
	return gz.Close()
}
