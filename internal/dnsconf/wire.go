// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package dnsconf

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"strings"
)

// The few pieces of the DNS wire format (RFC 1035) the stub and Query need:
// read a query's question, write an answer from a hosts file, build an A /
// AAAA query and pick the addresses out of a reply. Everything else is
// forwarded to a real server byte for byte.

const (
	typeA    = 1
	typePTR  = 12
	typeAAAA = 28
	classIN  = 1

	rcodeOK       = 0
	rcodeFormErr  = 1
	rcodeServFail = 2
	rcodeNXDomain = 3

	headerLen = 12
	maxName   = 255
)

var errMalformed = errors.New("malformed DNS message")

// question is the single question of a query.
type question struct {
	id     uint16
	flags  uint16
	name   string // lower case, no trailing dot ("" is the root)
	qtype  uint16
	qclass uint16
	end    int // offset just past the question
}

// readName reads an uncompressed name at off (a query's question never uses
// compression) and returns it in lower case without the trailing dot.
func readName(msg []byte, off int) (string, int, error) {
	var b strings.Builder
	total := 0
	for {
		if off >= len(msg) {
			return "", 0, errMalformed
		}
		l := int(msg[off])
		off++
		if l == 0 {
			return b.String(), off, nil
		}
		if l > 63 || off+l > len(msg) {
			return "", 0, errMalformed
		}
		total += l + 1
		if total > maxName {
			return "", 0, errMalformed
		}
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(strings.ToLower(string(msg[off : off+l])))
		off += l
	}
}

// skipName steps over a possibly compressed name in a reply.
func skipName(msg []byte, off int) (int, error) {
	for {
		if off >= len(msg) {
			return 0, errMalformed
		}
		l := int(msg[off])
		switch {
		case l == 0:
			return off + 1, nil
		case l&0xC0 == 0xC0:
			if off+2 > len(msg) {
				return 0, errMalformed
			}
			return off + 2, nil
		case l > 63:
			return 0, errMalformed
		}
		off += 1 + l
	}
}

// parseQuestion reads the header and the one question of a query.
func parseQuestion(msg []byte) (question, error) {
	if len(msg) < headerLen {
		return question{}, errMalformed
	}
	q := question{id: binary.BigEndian.Uint16(msg), flags: binary.BigEndian.Uint16(msg[2:])}
	if q.flags&0x8000 != 0 || binary.BigEndian.Uint16(msg[4:]) != 1 {
		return q, errMalformed // a reply, or not exactly one question
	}
	name, off, err := readName(msg, headerLen)
	if err != nil || off+4 > len(msg) {
		return q, errMalformed
	}
	q.name = name
	q.qtype = binary.BigEndian.Uint16(msg[off:])
	q.qclass = binary.BigEndian.Uint16(msg[off+2:])
	q.end = off + 4
	return q, nil
}

// appendName writes a name in wire form, uncompressed.
func appendName(b []byte, name string) []byte {
	name = strings.TrimSuffix(name, ".")
	if name != "" {
		for _, label := range strings.Split(name, ".") {
			b = append(b, byte(len(label)))
			b = append(b, label...)
		}
	}
	return append(b, 0)
}

// reply builds an authoritative answer to query q: the question echoed, then
// one record per rdata (each pointing back at the question's name). The
// query's additional records (an EDNS0 OPT) are not echoed.
func reply(query []byte, q question, rcode int, rtype uint16, rdatas [][]byte) []byte {
	const ttl = 0 // read from a file on every query: nothing to cache
	out := make([]byte, headerLen, headerLen+len(query)+len(rdatas)*32)
	binary.BigEndian.PutUint16(out, q.id)
	// QR, the query's opcode, AA, the query's RD, RA, then the rcode.
	flags := uint16(0x8000) | q.flags&0x7800 | 0x0400 | q.flags&0x0100 | 0x0080 | uint16(rcode&0xF)
	binary.BigEndian.PutUint16(out[2:], flags)
	qd := 0
	if q.end > headerLen {
		qd = 1
		out = append(out, query[headerLen:q.end]...)
	}
	binary.BigEndian.PutUint16(out[4:], uint16(qd))
	binary.BigEndian.PutUint16(out[6:], uint16(len(rdatas)))
	for _, rd := range rdatas {
		out = append(out, 0xC0, headerLen) // the question's name
		out = binary.BigEndian.AppendUint16(out, rtype)
		out = binary.BigEndian.AppendUint16(out, classIN)
		out = binary.BigEndian.AppendUint32(out, ttl)
		out = binary.BigEndian.AppendUint16(out, uint16(len(rd)))
		out = append(out, rd...)
	}
	return out
}

// buildQuery is a recursive query for name / qtype.
func buildQuery(id uint16, name string, qtype uint16) []byte {
	b := make([]byte, headerLen, headerLen+len(name)+6)
	binary.BigEndian.PutUint16(b, id)
	binary.BigEndian.PutUint16(b[2:], 0x0100) // RD
	binary.BigEndian.PutUint16(b[4:], 1)
	b = appendName(b, name)
	b = binary.BigEndian.AppendUint16(b, qtype)
	return binary.BigEndian.AppendUint16(b, classIN)
}

// answer is what parseReply found in a reply to one of our queries.
type answer struct {
	rcode     int
	truncated bool
	addrs     []netip.Addr
}

// parseReply checks a reply against the query (id, question) and collects the
// A / AAAA records of the answer section; a CNAME chain's records are all in
// that section, so their owners are not followed.
func parseReply(msg []byte, id uint16, name string, qtype uint16) (answer, error) {
	if len(msg) < headerLen || binary.BigEndian.Uint16(msg) != id {
		return answer{}, errMalformed
	}
	flags := binary.BigEndian.Uint16(msg[2:])
	if flags&0x8000 == 0 {
		return answer{}, errMalformed
	}
	a := answer{rcode: int(flags & 0xF), truncated: flags&0x0200 != 0}
	qd := int(binary.BigEndian.Uint16(msg[4:]))
	an := int(binary.BigEndian.Uint16(msg[6:]))
	off := headerLen
	for i := 0; i < qd; i++ {
		qn, next, err := readName(msg, off)
		if err != nil || next+4 > len(msg) {
			return answer{}, errMalformed
		}
		if qn != strings.ToLower(strings.TrimSuffix(name, ".")) || binary.BigEndian.Uint16(msg[next:]) != qtype {
			return answer{}, errMalformed
		}
		off = next + 4
	}
	for i := 0; i < an; i++ {
		next, err := skipName(msg, off)
		if err != nil || next+10 > len(msg) {
			return answer{}, errMalformed
		}
		rtype := binary.BigEndian.Uint16(msg[next:])
		rclass := binary.BigEndian.Uint16(msg[next+2:])
		rdlen := int(binary.BigEndian.Uint16(msg[next+8:]))
		rd := next + 10
		if rd+rdlen > len(msg) {
			return answer{}, errMalformed
		}
		if rclass == classIN && rtype == qtype {
			if addr, ok := netip.AddrFromSlice(msg[rd : rd+rdlen]); ok &&
				((qtype == typeA && rdlen == 4) || (qtype == typeAAAA && rdlen == 16)) {
				a.addrs = append(a.addrs, addr)
			}
		}
		off = rd + rdlen
	}
	return a, nil
}
