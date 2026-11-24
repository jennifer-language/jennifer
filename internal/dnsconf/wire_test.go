// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package dnsconf

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"reflect"
	"strings"
	"testing"
	"time"
)

// rr appends one resource record: owner (wire form), type, class IN, TTL 0,
// rdata.
func rr(b, owner []byte, rtype uint16, rdata []byte) []byte {
	b = append(b, owner...)
	b = binary.BigEndian.AppendUint16(b, rtype)
	b = binary.BigEndian.AppendUint16(b, classIN)
	b = binary.BigEndian.AppendUint32(b, 0)
	b = binary.BigEndian.AppendUint16(b, uint16(len(rdata)))
	return append(b, rdata...)
}

// replyMsg is a reply to id with the given question count (each the same
// question) and answer records.
func replyMsg(id uint16, qd int, qname string, qtype, qclass uint16, answers ...[]byte) []byte {
	b := make([]byte, headerLen)
	binary.BigEndian.PutUint16(b, id)
	binary.BigEndian.PutUint16(b[2:], 0x8180)
	binary.BigEndian.PutUint16(b[4:], uint16(qd))
	binary.BigEndian.PutUint16(b[6:], uint16(len(answers)))
	for i := 0; i < qd; i++ {
		b = appendName(b, qname)
		b = binary.BigEndian.AppendUint16(b, qtype)
		b = binary.BigEndian.AppendUint16(b, qclass)
	}
	for _, a := range answers {
		b = append(b, a...)
	}
	return b
}

// TestParseReplyChecksQuestionAndOwners: the reply must echo the question,
// and only records about the name asked (or its CNAME chain) count.
func TestParseReplyChecksQuestionAndOwners(t *testing.T) {
	ptr := []byte{0xC0, headerLen} // the question's name
	ip := func(s string) []byte { return netip.MustParseAddr(s).AsSlice() }
	a := func(owner []byte, s string) []byte { return rr(nil, owner, typeA, ip(s)) }

	for _, c := range []struct {
		name string
		msg  []byte
	}{
		{"no question", replyMsg(7, 0, "", 0, 0, a(appendName(nil, "x.test"), "192.0.2.1"))},
		{"two questions", replyMsg(7, 2, "x.test", typeA, classIN)},
		{"other name", replyMsg(7, 1, "y.test", typeA, classIN)},
		{"other type", replyMsg(7, 1, "x.test", typeAAAA, classIN)},
		{"other class", replyMsg(7, 1, "x.test", typeA, 3)},
		{"forward pointer", replyMsg(7, 1, "x.test", typeA, classIN, a([]byte{0xC0, 0xFF}, "192.0.2.1"))},
	} {
		if _, err := parseReply(c.msg, 7, "x.test", typeA); err == nil {
			t.Errorf("%s: accepted", c.name)
		}
	}

	// A record for another owner is ignored; the CNAME chain is followed,
	// through compressed names.
	msg := replyMsg(7, 1, "x.test", typeA, classIN)
	msg = rr(msg, ptr, typeA, ip("192.0.2.1"))
	msg = rr(msg, appendName(nil, "evil.test"), typeA, ip("203.0.113.66"))
	cnameAt := len(msg) + 12 // the CNAME's rdata: its target's name
	msg = rr(msg, ptr, typeCNAME, appendName(nil, "alias.test"))
	msg = rr(msg, []byte{0xC0, byte(cnameAt)}, typeA, ip("192.0.2.2"))
	binary.BigEndian.PutUint16(msg[6:], 4)
	got, err := parseReply(msg, 7, "X.Test.", typeA)
	if err != nil {
		t.Fatal(err)
	}
	var addrs []string
	for _, ad := range got.addrs {
		addrs = append(addrs, ad.String())
	}
	if want := []string{"192.0.2.1", "192.0.2.2"}; !reflect.DeepEqual(addrs, want) {
		t.Errorf("addrs %v, want %v", addrs, want)
	}
}

// TestHostsNameWithSearchDomain: a name the hosts file lists, with a search
// domain appended, is NXDOMAIN from the stub and never reaches the server, so
// Go falls through to the name as written.
func TestHostsNameWithSearchDomain(t *testing.T) {
	f := startFakeDNS(t)
	old := searchList
	searchList = func() []string { return []string{"corp.example", "lan"} }
	t.Cleanup(func() { searchList = old })
	h := parseHosts([]byte("10.9.9.9 myhost\n"))
	h.search = searchList()
	for _, name := range []string{"myhost.corp.example", "MyHost.lan"} {
		q := buildQuery(9, name, typeA)
		resp := answerQuery(q, []string{f.addr}, h, false, time.Now().Add(2*time.Second))
		if len(resp) < 4 || resp[3]&0xF != rcodeNXDomain {
			t.Errorf("%s: %x, want NXDOMAIN", name, resp)
		}
	}
	// Other names still go to the server.
	_ = answerQuery(buildQuery(9, "myhost.other.example", typeA), []string{f.addr}, h, false, time.Now().Add(2*time.Second))
	f.mu.Lock()
	defer f.mu.Unlock()
	if want := []string{"myhost.other.example"}; !reflect.DeepEqual(f.queries, want) {
		t.Errorf("the server saw %v, want %v", f.queries, want)
	}
	if got := parseSearch([]byte("search a.example B.example.\ndomain c.example\n")); !reflect.DeepEqual(got, []string{"c.example"}) {
		t.Errorf("last line wins: %v", got)
	}
	if got := parseSearch([]byte("domain c.example\nsearch a.example b.example # x\n")); !reflect.DeepEqual(got, []string{"a.example", "b.example"}) {
		t.Errorf("search: %v", got)
	}
}

// TestHostsAnswerBounded: names that are not valid DNS names are skipped,
// and an answer too large for one message is cut and marked truncated.
func TestHostsAnswerBounded(t *testing.T) {
	long := strings.Repeat("a", 64)
	h := parseHosts([]byte("10.0.0.1 a..b " + long + ".test ok.test\n"))
	if _, ok := h.byName["a..b"]; ok {
		t.Error("an empty label is skipped")
	}
	if _, ok := h.byName[long+".test"]; ok {
		t.Error("a 64-byte label is skipped")
	}
	if got := h.byAddr["1.0.0.10.in-addr.arpa"]; !reflect.DeepEqual(got, []string{"ok.test"}) {
		t.Errorf("PTR names %v", got)
	}

	var sb strings.Builder
	for i := 0; i < 3000; i++ {
		fmt.Fprintf(&sb, "2001:db8::%x big.test\n", i+1)
	}
	h = parseHosts([]byte(sb.String()))
	q := buildQuery(1, "big.test", typeAAAA)
	pq, err := parseQuestion(q)
	if err != nil {
		t.Fatal(err)
	}
	resp := fromHosts(q, pq, h)
	if len(resp) > maxMessage {
		t.Fatalf("reply of %d bytes", len(resp))
	}
	if resp[2]&0x02 == 0 {
		t.Error("a cut reply is marked truncated")
	}
	an := int(binary.BigEndian.Uint16(resp[6:]))
	if an == 0 || len(q)+an*28 != len(resp) {
		t.Errorf("%d answers in %d bytes", an, len(resp))
	}
}
